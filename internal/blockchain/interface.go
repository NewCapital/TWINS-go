package blockchain

import (
	"github.com/twins-dev/twins-core/pkg/types"
)

// Blockchain defines the interface for blockchain operations
type Blockchain interface {
	// Chain state
	GetBestBlock() (*types.Block, error)
	GetBestHeight() (uint32, error)
	GetBestBlockHash() (types.Hash, error)

	// Block operations
	GetBlock(hash types.Hash) (*types.Block, error)
	GetBlockByHeight(height uint32) (*types.Block, error)
	GetBlockHash(height uint32) (types.Hash, error)
	HasBlock(hash types.Hash) (bool, error)

	// Block processing
	ProcessBlock(block *types.Block) error
	ProcessBlockBatch(blocks []*types.Block) error // Process multiple blocks in batch
	ConnectBlock(block *types.Block) error
	DisconnectBlock(block *types.Block) error

	// Chain queries
	GetBlockHeight(hash types.Hash) (uint32, error)
	IsOnMainChain(hash types.Hash) (bool, error)
	GetChainWork() (*types.BigInt, error)
	GetMoneySupply(height uint32) (int64, error)

	// Transaction queries
	GetTransaction(hash types.Hash) (*types.Transaction, error)
	GetTransactionBlock(hash types.Hash) (*types.Block, error)

	// UTXO operations
	GetUTXO(outpoint types.Outpoint) (*types.UTXO, error)
	GetUTXOsByAddress(address string) ([]*types.UTXO, error)

	// Chain management
	GetOrphanBlocks() []*types.Block
	GetChainTips() ([]ChainTip, error)
	IsInitialBlockDownload() bool

	// Network consensus (for dynamic validation)
	SetNetworkConsensusHeight(height uint32)
	GetNetworkConsensusHeight() uint32
	SetNetworkPeerCount(count int32)

	// Checkpoint validation
	GetCheckpointManager() types.CheckpointManager
	ValidateChainSegment(fromHeight, toHeight uint32) error

	// Block invalidation and reconsideration
	InvalidateBlock(hash types.Hash) error
	ReconsiderBlock(hash types.Hash) error
	AddCheckpoint(height uint32, hash types.Hash) error

	// Wallet notification
	SetWallet(wallet WalletNotifier)

	// Lifecycle
	Start() error
	Stop() error
	Sync() error
}

// ChainTip represents a blockchain tip
type ChainTip struct {
	Height uint32
	Hash   types.Hash
	Work   *types.BigInt
	Status ChainTipStatus
}

// ChainTipStatus represents the status of a chain tip
type ChainTipStatus int

const (
	ChainTipActive ChainTipStatus = iota
	ChainTipOrphan
	ChainTipValidHeaders
	ChainTipValidFork
	ChainTipInvalid
)

func (s ChainTipStatus) String() string {
	switch s {
	case ChainTipActive:
		return "active"
	case ChainTipOrphan:
		return "orphan"
	case ChainTipValidHeaders:
		return "valid-headers"
	case ChainTipValidFork:
		return "valid-fork"
	case ChainTipInvalid:
		return "invalid"
	default:
		return "unknown"
	}
}

// BlockchainStats contains blockchain statistics
type BlockchainStats struct {
	Height          uint32
	BestBlockHash   types.Hash
	Difficulty      float64
	MedianTime      uint32
	ChainWork       *types.BigInt
	Blocks          uint32
	Transactions    uint64
	UTXOs           uint64
	OrphanBlocks    int
	VerifiedBlocks  uint32
	ValidationSpeed float64 // Blocks per second
}

// Config contains blockchain configuration
type Config struct {
	// Chain parameters
	ChainParams *types.ChainParams

	// Dependencies
	Storage   interface{} // storage.Storage interface
	Consensus interface{} // consensus.Engine interface

	// Performance settings
	MaxOrphans         int
	BlockCacheSize     int
	MaxReorgDepth      uint32
	CheckpointInterval uint32

	// UTXO Cache settings
	EnableUTXOCache     bool // Enable UTXO caching for performance
	MaxUTXOCacheEntries int  // Maximum number of UTXO entries to cache

	// Batch Processing settings
	EnableBatchProcessing bool // Enable batch processing for sync
	MaxBatchSize          int  // Maximum blocks per batch
	BatchTimeoutMs        int  // Batch timeout in milliseconds

	// Network settings
	Network string

	// Indexing options - ALWAYS enabled, database must write all data
	EnableAddressIndex bool

	// Sync thresholds
	IBDThreshold uint32 // Blocks behind to trigger IBD mode (0 = use default 5000)
}

// DefaultConfig returns default blockchain configuration
func DefaultConfig() *Config {
	return &Config{
		MaxOrphans:         100,
		BlockCacheSize:     1000,
		MaxReorgDepth:      100000, // Increased to handle deep fork recovery
		CheckpointInterval: 1000,
		EnableUTXOCache:       true,
		MaxUTXOCacheEntries:   100000, // ~100MB for typical UTXOs
		EnableBatchProcessing: true,
		MaxBatchSize:          500, // Process up to 100 blocks per batch
		BatchTimeoutMs:        100, // 100ms timeout for batches
		EnableAddressIndex:    true,
		Network:               "mainnet",
		IBDThreshold:          DefaultIBDThreshold,
	}
}

// IsTestNet returns true if the network is testnet
func (c *Config) IsTestNet() bool {
	return c.Network == "testnet" || c.Network == "test"
}
