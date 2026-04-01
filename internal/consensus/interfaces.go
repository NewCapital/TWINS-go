package consensus

import (
	"github.com/twins-dev/twins-core/pkg/crypto"
	"github.com/twins-dev/twins-core/pkg/types"
)

// StakingWalletInterface defines wallet operations needed for staking.
// This interface allows the consensus engine to interact with the wallet
// for staking operations without creating a circular dependency.
//
// CRITICAL ARCHITECTURE DECISION:
// The staking system uses this interface to decouple wallet from consensus.
// This enables staking to operate independently of masternode sync status,
// preventing the deadlock that killed the legacy C++ chain.
type StakingWalletInterface interface {
	// IsLocked returns true if the wallet is encrypted and locked.
	// Staking requires an unlocked wallet to sign blocks.
	IsLocked() bool

	// GetStakeableUTXOs returns UTXOs eligible for staking.
	// Eligibility criteria:
	// - Minimum confirmations (MinStakeConfirmations = 60)
	// - Minimum coin age (StakeMinAge from ChainParams)
	// - Coinbase/coinstake outputs need CoinbaseMaturity confirmations
	// - Not locked for other purposes
	GetStakeableUTXOs(chainHeight uint32, chainTime uint32) ([]*StakeableUTXO, error)

	// CreateCoinstakeTx builds the coinstake transaction for a valid stake.
	// The coinstake structure:
	// - Input[0]: stake UTXO being spent (with signature)
	// - Output[0]: empty marker (value=0, empty script) - required for IsCoinStake()
	// - Output[1]: stake return + reward (to staker's address)
	// - Output[2+]: masternode payments, dev fund, etc.
	// CreateCoinstakeTx builds the coinstake transaction structure WITHOUT signing.
	// The transaction is returned unsigned - call SignCoinstakeTx after adding
	// masternode/dev outputs to sign it.
	// Legacy: wallet.cpp:3251-3332 creates structure, FillBlockPayee adds outputs,
	// then CreateTxIn signs (line 3341).
	CreateCoinstakeTx(
		stakeUTXO *StakeableUTXO,
		blockReward int64,
		blockTime uint32,
	) (*types.Transaction, error)

	// SignCoinstakeTx signs an unsigned coinstake transaction.
	// Must be called AFTER all outputs are added (masternode, dev fund, etc.)
	// Legacy: wallet.cpp:3341 - stakeInput->CreateTxIn(this, in, hashTxOut)
	SignCoinstakeTx(tx *types.Transaction, stakeUTXO *StakeableUTXO) error

	// GetPrivateKeyForAddress returns the private key for signing blocks.
	// The address should be the P2PKH address from the stake input.
	GetPrivateKeyForAddress(address string) (*crypto.PrivateKey, error)

	// SignMessageBytes signs arbitrary data with the key for the given address.
	// Returns raw DER-encoded signature bytes for block signature creation.
	SignMessageBytes(address string, message []byte) ([]byte, error)
}

// StakeableUTXO represents a UTXO that is eligible for staking.
// This contains all information needed to attempt staking with this UTXO.
type StakeableUTXO struct {
	// Outpoint identifies the UTXO
	Outpoint types.Outpoint

	// Value in satoshis
	Amount int64

	// Address that owns this UTXO (Base58 encoded)
	Address string

	// Block information for coin age calculation
	BlockHeight uint32
	BlockTime   uint32

	// Derived values
	Confirmations uint32
	CoinAge       int64 // in seconds

	// Script information for transaction building
	ScriptPubKey []byte
}

// ToStakeInput converts a StakeableUTXO to a StakeInput for kernel validation.
func (su *StakeableUTXO) ToStakeInput() *StakeInput {
	return &StakeInput{
		TxHash:      su.Outpoint.Hash,
		Index:       su.Outpoint.Index,
		Value:       su.Amount,
		BlockHeight: su.BlockHeight,
		BlockTime:   su.BlockTime,
	}
}

// MempoolInterface defines mempool operations needed for block building.
type MempoolInterface interface {
	// GetTransactionsForBlock returns transactions to include in a new block.
	// The transactions should be sorted by fee rate (highest first).
	// maxSize is the maximum total size in bytes.
	// maxCount is the maximum number of transactions.
	GetTransactionsForBlock(maxSize uint32, maxCount int) []*types.Transaction

	// RemoveTransaction removes a transaction from the mempool.
	// Called after a block containing the transaction is accepted.
	RemoveTransaction(txHash types.Hash) error

	// RemoveConfirmedTransactions removes all transactions confirmed in a block,
	// plus any conflicting transactions whose inputs overlap with block transactions.
	RemoveConfirmedTransactions(block *types.Block)
}

// BlockSubmitter defines the interface for submitting newly created blocks.
type BlockSubmitter interface {
	// ProcessBlock validates and adds a block to the chain.
	// Returns nil if the block was accepted.
	ProcessBlock(block *types.Block) error

	// BroadcastBlock sends the block to connected peers.
	BroadcastBlock(block *types.Block) error
}
