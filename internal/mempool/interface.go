package mempool

import (
	"time"

	"github.com/twins-dev/twins-core/pkg/types"
)

// Mempool defines the interface for the transaction memory pool
type Mempool interface {
	// Transaction operations
	AddTransaction(tx *types.Transaction) error
	RemoveTransaction(hash types.Hash) error
	HasTransaction(hash types.Hash) bool
	GetTransaction(hash types.Hash) (*types.Transaction, error)

	// Batch operations
	GetTransactions(maxCount int) []*types.Transaction
	GetTransactionsForBlock(maxSize uint32, maxCount int) []*types.Transaction
	RemoveTransactions(hashes []types.Hash) error

	// Priority and fee operations
	GetHighPriorityTransactions(count int) []*types.Transaction
	GetTransactionsByFeeRate(minFeeRate int64, maxCount int) []*types.Transaction
	UpdatePriority(hash types.Hash, priorityDelta float64, feeDelta int64) error

	// Query operations
	Count() int
	Size() uint64
	GetStats() *Stats

	// Block cleanup
	RemoveConfirmedTransactions(block *types.Block)

	// Orphan management
	GetOrphanTransactions() []*types.Transaction
	RemoveOrphanTransaction(hash types.Hash) error

	// Notification
	SetOnTransaction(fn func(*types.Transaction))
	SetOnRemoveTransaction(fn func(types.Hash))

	// Lifecycle
	Start() error
	Stop() error
	Clear() error
}

// Stats contains mempool statistics
type Stats struct {
	// Size metrics
	TransactionCount int
	OrphanCount      int
	TotalSize        uint64
	TotalFees        int64

	// Rate metrics
	AddedLast1Min    int
	AddedLast5Min    int
	RemovedLast1Min  int
	RemovedLast5Min  int

	// Performance metrics
	AverageAddTime      time.Duration
	AverageValidateTime time.Duration
	RejectedCount       uint64
	EvictedCount        uint64

	// Fee metrics
	MinFeeRate    int64
	MedianFeeRate int64
	MaxFeeRate    int64
}

// Config contains mempool configuration
type Config struct {
	// Size limits
	MaxSize           uint64 // Maximum total size in bytes
	MaxTransactions   int    // Maximum number of transactions
	MaxOrphans        int    // Maximum orphan transactions
	MaxTransactionAge time.Duration

	// Fee requirements
	MinRelayFee     int64 // Minimum fee to relay (satoshis per KB)
	MinMempoolFee   int64 // Minimum fee to stay in mempool
	DynamicFeeFloor int64 // Dynamic fee floor

	// Performance settings
	ValidationWorkers int
	ExpiryInterval    time.Duration
	CleanupInterval   time.Duration

	// Rate limiting
	MaxTxsPerPeer     int
	MaxTxsPerSecond   int
	BanDuration       time.Duration
	MaxRejectionsRate int

	// Dependencies
	Blockchain  interface{} // blockchain.Blockchain
	Consensus   interface{} // consensus.Engine
	ChainParams *types.ChainParams
	UTXOSet     interface{} // UTXOSet interface for querying UTXOs
}

// DefaultConfig returns default mempool configuration
func DefaultConfig() *Config {
	return &Config{
		MaxSize:           300 * 1024 * 1024, // 300MB
		MaxTransactions:   50000,
		MaxOrphans:        1000,
		MaxTransactionAge: 24 * time.Hour,

		MinRelayFee:     1000, // 1000 satoshis per KB
		MinMempoolFee:   1000,
		DynamicFeeFloor: 1000,

		ValidationWorkers: 4,
		ExpiryInterval:    5 * time.Minute,
		CleanupInterval:   1 * time.Minute,

		MaxTxsPerPeer:     1000,
		MaxTxsPerSecond:   100,
		BanDuration:       10 * time.Minute,
		MaxRejectionsRate: 10,
	}
}

// TxEntry represents a transaction in the mempool
type TxEntry struct {
	Tx            *types.Transaction
	Hash          types.Hash
	Size          uint32
	Fee           int64
	FeeRate       int64
	Time          time.Time
	Height        uint32
	Priority      float64
	Dependencies  []types.Hash
	Descendants   []types.Hash
	SpendsPending bool
}

// TxPriority represents transaction priority
type TxPriority int

const (
	TxPriorityLow TxPriority = iota
	TxPriorityNormal
	TxPriorityHigh
	TxPriorityUrgent
)

// RejectCode represents reasons for transaction rejection
type RejectCode int

const (
	RejectInvalid RejectCode = iota
	RejectDuplicate
	RejectInsufficientFee
	RejectNonStandard
	RejectDust
	RejectTooLarge
	RejectRateLimited
	RejectPoolFull
	RejectOrphan
	RejectConflict
	RejectMalformed
	RejectExpired
)

func (r RejectCode) String() string {
	switch r {
	case RejectInvalid:
		return "invalid"
	case RejectDuplicate:
		return "duplicate"
	case RejectInsufficientFee:
		return "insufficient-fee"
	case RejectNonStandard:
		return "non-standard"
	case RejectDust:
		return "dust"
	case RejectTooLarge:
		return "too-large"
	case RejectRateLimited:
		return "rate-limited"
	case RejectPoolFull:
		return "pool-full"
	case RejectOrphan:
		return "orphan"
	case RejectConflict:
		return "conflict"
	case RejectMalformed:
		return "malformed"
	case RejectExpired:
		return "expired"
	default:
		return "unknown"
	}
}

// MempoolError represents a mempool error
type MempoolError struct {
	Code    RejectCode
	Message string
	TxHash  types.Hash
}

func (e *MempoolError) Error() string {
	return e.Message
}

// NewMempoolError creates a new mempool error
func NewMempoolError(code RejectCode, message string, txHash types.Hash) *MempoolError {
	return &MempoolError{
		Code:    code,
		Message: message,
		TxHash:  txHash,
	}
}