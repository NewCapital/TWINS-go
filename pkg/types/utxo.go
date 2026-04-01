package types

import (
	"bytes"
	"fmt"
	"sync"
)

// Outpoint represents a specific output in a transaction
type Outpoint struct {
	Hash  Hash   // Transaction hash
	Index uint32 // Output index within the transaction
}

// UTXO represents an unspent transaction output
type UTXO struct {
	Outpoint   Outpoint  // Reference to the transaction output
	Output     *TxOutput // The actual output data
	Height     uint32    // Block height where this UTXO was created
	IsCoinbase bool      // Whether this UTXO comes from a coinbase transaction

	// Spending metadata (0/empty = unspent)
	SpendingHeight uint32 // Block height where spent (0 = unspent)
	SpendingTxHash Hash   // Transaction hash that spent this UTXO (empty = unspent)
}

// UTXOSet manages a collection of unspent transaction outputs
type UTXOSet struct {
	utxos map[Outpoint]*UTXO
	mutex sync.RWMutex // Protects concurrent access to the UTXO set
}

// NewUTXOSet creates a new empty UTXO set
func NewUTXOSet() *UTXOSet {
	return &UTXOSet{
		utxos: make(map[Outpoint]*UTXO),
	}
}

// String returns a string representation of the outpoint
func (op Outpoint) String() string {
	return fmt.Sprintf("%s:%d", op.Hash.String(), op.Index)
}

// Bytes returns the byte representation of the outpoint
func (op Outpoint) Bytes() []byte {
	result := make([]byte, 36)
	copy(result[:32], op.Hash[:])
	result[32] = byte(op.Index)
	result[33] = byte(op.Index >> 8)
	result[34] = byte(op.Index >> 16)
	result[35] = byte(op.Index >> 24)
	return result
}

// String returns a string representation of the UTXO
func (u *UTXO) String() string {
	coinbaseStr := ""
	if u.IsCoinbase {
		coinbaseStr = " (coinbase)"
	}
	return fmt.Sprintf("%s: %d satoshis at height %d%s",
		u.Outpoint.String(), u.Output.Value, u.Height, coinbaseStr)
}

// IsUnspent returns true if this UTXO has not been spent
func (u *UTXO) IsUnspent() bool {
	return u.SpendingHeight == 0
}

// IsSpent returns true if this UTXO has been spent
func (u *UTXO) IsSpent() bool {
	return u.SpendingHeight != 0
}

// IsSpendable checks if the UTXO can be spent given the current height and minimum confirmations
func (u *UTXO) IsSpendable(currentHeight, minConfirmations uint32) bool {
	if currentHeight < u.Height {
		return false // UTXO is from a future block
	}

	confirmations := currentHeight - u.Height

	// Coinbase outputs require more confirmations (typically 100)
	if u.IsCoinbase {
		return confirmations >= 100
	}

	return confirmations >= minConfirmations
}

// Add adds a new UTXO to the set
func (us *UTXOSet) Add(utxo *UTXO) {
	us.mutex.Lock()
	defer us.mutex.Unlock()
	us.utxos[utxo.Outpoint] = utxo
}

// Remove removes a UTXO from the set
func (us *UTXOSet) Remove(outpoint Outpoint) {
	us.mutex.Lock()
	defer us.mutex.Unlock()
	delete(us.utxos, outpoint)
}

// Get retrieves a UTXO from the set
func (us *UTXOSet) Get(outpoint Outpoint) (*UTXO, bool) {
	us.mutex.RLock()
	defer us.mutex.RUnlock()
	utxo, exists := us.utxos[outpoint]
	return utxo, exists
}

// GetBalance calculates the total spendable balance for a given script
func (us *UTXOSet) GetBalance(scriptPubKey []byte, currentHeight, minConfirmations uint32) int64 {
	us.mutex.RLock()
	defer us.mutex.RUnlock()

	var balance int64
	for _, utxo := range us.utxos {
		// Check if this UTXO belongs to the script and is spendable
		if bytes.Equal(utxo.Output.ScriptPubKey, scriptPubKey) &&
			utxo.IsSpendable(currentHeight, minConfirmations) {
			balance += utxo.Output.Value
		}
	}
	return balance
}

// GetUTXOs returns all UTXOs that match the given script
func (us *UTXOSet) GetUTXOs(scriptPubKey []byte) []*UTXO {
	us.mutex.RLock()
	defer us.mutex.RUnlock()

	var utxos []*UTXO
	for _, utxo := range us.utxos {
		if bytes.Equal(utxo.Output.ScriptPubKey, scriptPubKey) {
			utxos = append(utxos, utxo)
		}
	}
	return utxos
}

// GetSpendableUTXOs returns all spendable UTXOs for a given script
func (us *UTXOSet) GetSpendableUTXOs(scriptPubKey []byte, currentHeight, minConfirmations uint32) []*UTXO {
	us.mutex.RLock()
	defer us.mutex.RUnlock()

	var utxos []*UTXO
	for _, utxo := range us.utxos {
		if bytes.Equal(utxo.Output.ScriptPubKey, scriptPubKey) &&
			utxo.IsSpendable(currentHeight, minConfirmations) {
			utxos = append(utxos, utxo)
		}
	}
	return utxos
}

// Size returns the number of UTXOs in the set
func (us *UTXOSet) Size() int {
	us.mutex.RLock()
	defer us.mutex.RUnlock()
	return len(us.utxos)
}

// TotalValue returns the total value of all UTXOs in the set
func (us *UTXOSet) TotalValue() int64 {
	us.mutex.RLock()
	defer us.mutex.RUnlock()

	var total int64
	for _, utxo := range us.utxos {
		total += utxo.Output.Value
	}
	return total
}
