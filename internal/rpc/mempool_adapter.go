package rpc

import (
	"fmt"
	"time"

	"github.com/twins-dev/twins-core/internal/blockchain"
	"github.com/twins-dev/twins-core/internal/mempool"
	"github.com/twins-dev/twins-core/pkg/types"
)

// MempoolAdapter adapts mempool.TxMempool to rpc.MempoolInterface
type MempoolAdapter struct {
	mp         *mempool.TxMempool
	blockchain blockchain.Blockchain
}

// NewMempoolAdapter creates a new mempool adapter
func NewMempoolAdapter(mp *mempool.TxMempool, bc blockchain.Blockchain) *MempoolAdapter {
	return &MempoolAdapter{
		mp:         mp,
		blockchain: bc,
	}
}

// AddTransaction adds a transaction to the mempool
func (a *MempoolAdapter) AddTransaction(tx *types.Transaction) error {
	return a.mp.AddTransaction(tx)
}

// GetTransaction retrieves a transaction from the mempool
func (a *MempoolAdapter) GetTransaction(hash types.Hash) (*types.Transaction, bool) {
	tx, err := a.mp.GetTransaction(hash)
	if err != nil {
		return nil, false
	}
	return tx, true
}

// GetRawMempool returns all transaction hashes in the mempool
func (a *MempoolAdapter) GetRawMempool() []types.Hash {
	txs := a.mp.GetTransactions(0) // 0 = unlimited
	hashes := make([]types.Hash, len(txs))
	for i, tx := range txs {
		hashes[i] = tx.Hash()
	}
	return hashes
}

// GetMempoolInfo returns mempool information
func (a *MempoolAdapter) GetMempoolInfo() MempoolInfo {
	stats := a.mp.GetStats()
	return MempoolInfo{
		Size:  stats.TransactionCount,
		Bytes: stats.TotalSize,
		Usage: stats.TotalSize,
		MaxMempool: 300 * 1024 * 1024, // 300MB default
		MempoolMinFee: 0.0001,
	}
}

// GetMempoolEntry returns detailed information about a mempool entry
func (a *MempoolAdapter) GetMempoolEntry(hash types.Hash) (*MempoolEntry, error) {
	tx, err := a.mp.GetTransaction(hash)
	if err != nil {
		return nil, err
	}

	// Calculate size
	txData, _ := tx.Serialize()
	size := len(txData)

	// Calculate fee (would need UTXO data to be accurate)
	fee := int64(0)

	// Get current block height from blockchain
	height := uint32(0)
	if a.blockchain != nil {
		if h, err := a.blockchain.GetBestHeight(); err == nil {
			height = h
		}
	}

	// Track transaction add time (current time)
	addTime := time.Now().Unix()

	entry := &MempoolEntry{
		Size:            size,
		Fee:             fee,
		ModifiedFee:     fee,
		Time:            addTime,
		Height:          int64(height),
		DescendantCount: 1,
		DescendantSize:  size,
		DescendantFees:  fee,
		AncestorCount:   1,
		AncestorSize:    size,
		AncestorFees:    fee,
		Depends:         []string{},
	}

	return entry, nil
}

// RemoveTransaction removes a transaction from the mempool
func (a *MempoolAdapter) RemoveTransaction(hash types.Hash) {
	a.mp.RemoveTransaction(hash)
}

// HasTransaction checks if a transaction exists in the mempool
func (a *MempoolAdapter) HasTransaction(hash types.Hash) bool {
	return a.mp.HasTransaction(hash)
}

// GetTransactions returns all transactions in the mempool
func (a *MempoolAdapter) GetTransactions() []*types.Transaction {
	return a.mp.GetTransactions(0) // 0 = unlimited
}

// GetMempoolAncestors returns ancestors of a transaction in the mempool
func (a *MempoolAdapter) GetMempoolAncestors(hash types.Hash) ([]types.Hash, error) {
	// Get the transaction from mempool
	tx, err := a.mp.GetTransaction(hash)
	if err != nil {
		return nil, err
	}

	// Find all ancestor transactions by walking inputs
	ancestors := make([]types.Hash, 0)
	visited := make(map[types.Hash]bool)

	var findAncestors func(*types.Transaction)
	findAncestors = func(t *types.Transaction) {
		for _, input := range t.Inputs {
			prevHash := input.PreviousOutput.Hash
			if visited[prevHash] {
				continue
			}
			visited[prevHash] = true

			// Check if ancestor is in mempool
			if a.mp.HasTransaction(prevHash) {
				ancestors = append(ancestors, prevHash)
				if parentTx, err := a.mp.GetTransaction(prevHash); err == nil {
					findAncestors(parentTx)
				}
			}
		}
	}

	findAncestors(tx)
	return ancestors, nil
}

// GetMempoolDescendants returns descendants of a transaction in the mempool
func (a *MempoolAdapter) GetMempoolDescendants(hash types.Hash) ([]types.Hash, error) {
	// Get all transactions in mempool
	allTxs := a.mp.GetTransactions(0)

	// Find descendants by checking which transactions spend outputs of this tx
	descendants := make([]types.Hash, 0)
	visited := make(map[types.Hash]bool)
	toCheck := []types.Hash{hash}

	for len(toCheck) > 0 {
		currentHash := toCheck[0]
		toCheck = toCheck[1:]

		if visited[currentHash] {
			continue
		}
		visited[currentHash] = true

		// Check all mempool transactions for inputs spending this transaction
		for _, tx := range allTxs {
			txHash := tx.Hash()
			if visited[txHash] {
				continue
			}

			for _, input := range tx.Inputs {
				if input.PreviousOutput.Hash == currentHash {
					descendants = append(descendants, txHash)
					toCheck = append(toCheck, txHash)
					break
				}
			}
		}
	}

	return descendants, nil
}

// ValidateTransaction validates a transaction without adding to mempool.
// Note: The mempool package does not expose a read-only validation path,
// so this returns an error to prevent accidental mempool mutation.
// Callers needing validation should use AddTransaction directly.
func (a *MempoolAdapter) ValidateTransaction(tx *types.Transaction) error {
	return fmt.Errorf("read-only validation not supported; use AddTransaction for validation-and-insert")
}

// GetMempoolSize returns the number of transactions in the mempool
func (a *MempoolAdapter) GetMempoolSize() int {
	return a.mp.Count()
}

// GetMempoolBytes returns the total size of the mempool in bytes
func (a *MempoolAdapter) GetMempoolBytes() uint64 {
	return a.mp.Size()
}

// GetStats returns mempool statistics
func (a *MempoolAdapter) GetStats() interface{} {
	return a.mp.GetStats()
}

// UpdatePriority updates the priority and fee delta for a transaction
func (a *MempoolAdapter) UpdatePriority(hash types.Hash, priorityDelta float64, feeDelta int64) error {
	return a.mp.UpdatePriority(hash, priorityDelta, feeDelta)
}
