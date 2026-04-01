package consensus

import (
	"context"
	"time"

	"github.com/twins-dev/twins-core/pkg/types"
)

// Engine defines the consensus interface for TWINS PoS
type Engine interface {
	// Start begins the consensus engine with the given context
	Start(ctx context.Context) error

	// Stop gracefully shuts down the consensus engine
	Stop() error

	// ValidateBlock validates a block according to PoS consensus rules
	ValidateBlock(block *types.Block) error

	// ValidateBlockForBatch validates a block optimized for batch processing
	// Skips UTXO lookup and script verification since MarkUTXOSpent verifies UTXO existence
	ValidateBlockForBatch(block *types.Block) error

	// GetNextBlockTime returns when the next block can be mined
	GetNextBlockTime() time.Time

	// CanStake returns whether the node can participate in staking
	CanStake() bool

	// IsStaking returns whether the consensus engine is actively staking
	IsStaking() bool

	// GetNetworkStakeWeight returns the estimated total network stake weight
	GetNetworkStakeWeight() int64

	// StartStaking begins the staking process (requires wallet with mature coins)
	StartStaking() error

	// StopStaking halts the staking process
	StopStaking() error

	// GetStakeModifier returns the current stake modifier
	GetStakeModifier() uint64

	// ComputeAndStoreModifier computes and stores stake modifier for a block in the given batch
	// This should be called during block processing to ensure modifiers are persisted
	// Returns: modifier value, whether it was generated (vs inherited), error
	ComputeAndStoreModifier(block *types.Block, height uint32, batch interface{}) (uint64, bool, error)

	// Subscribe returns a channel for consensus events
	Subscribe(eventType EventType) <-chan Event

	// Unsubscribe removes event subscription
	Unsubscribe(eventType EventType, ch <-chan Event)

	// GetStats returns current consensus statistics
	GetStats() *Stats

	// SetConsensusProvider sets the network consensus height provider for staking sync validation.
	// This should be called after P2P layer is initialized.
	SetConsensusProvider(provider ConsensusHeightProvider)

	// SetBlockBroadcaster sets the callback to broadcast staked blocks to the P2P network.
	// This should be called after P2P layer is initialized.
	SetBlockBroadcaster(broadcaster func(*types.Block))

	// SetWallet sets the wallet interface for staking operations.
	// Must be called before StartStaking().
	SetWallet(wallet StakingWalletInterface)
}

// EventType represents different types of consensus events
type EventType int

const (
	EventBlockValidated EventType = iota
	EventBlockProcessed
	EventStakeFound
	EventConsensusUpdate
	EventError
)

// Event represents a consensus event
type Event struct {
	Type      EventType
	Block     *types.Block
	Error     error
	Data      interface{}
	Timestamp time.Time
}

// Stats contains consensus engine statistics
type Stats struct {
	BlocksValidated uint64
	BlocksProcessed uint64
	StakingActive   bool
	LastBlockTime   time.Time
	NextStakeTime   time.Time
	StakeModifier   uint64
	ActiveStakers   int
	NetworkWeight   uint64
	MyWeight        uint64
	StakeReward     uint64
	Difficulty      uint32
	ValidationTime  time.Duration
	ProcessingTime  time.Duration
}

// StakeValidator validates staking operations
type StakeValidator interface {
	// ValidateStake validates a stake transaction
	ValidateStake(stake *types.Transaction, block *types.Block) error

	// CalculateStakeReward calculates the reward for a stake
	CalculateStakeReward(stake *types.Transaction, blockTime time.Time) uint64

	// CheckStakeAge verifies minimum stake age requirement
	CheckStakeAge(stake *types.Transaction, blockTime time.Time) bool

	// GetStakeModifier calculates the stake modifier for given block
	GetStakeModifier(prevBlock *types.Block) uint64
}

// BlockProducer handles block creation for PoS
type BlockProducer interface {
	// CreateBlock creates a new block with given transactions
	CreateBlock(txs []*types.Transaction, stakeTx *types.Transaction) (*types.Block, error)

	// SignBlock signs a block with the staker's key
	SignBlock(block *types.Block, privateKey []byte) error

	// FindStake searches for valid stake kernel
	FindStake(ctx context.Context) (*types.Transaction, error)

	// EstimateStakeTime estimates time until next successful stake
	EstimateStakeTime() time.Duration
}

// NetworkState provides access to network-wide consensus state
type NetworkState interface {
	// GetNetworkWeight returns the total network staking weight
	GetNetworkWeight() uint64

	// GetActiveStakers returns the number of active stakers
	GetActiveStakers() int

	// GetLastBlock returns the last block in the chain
	GetLastBlock() *types.Block

	// GetBlockByHeight returns block at specific height
	GetBlockByHeight(height uint32) (*types.Block, error)

	// GetChainTip returns the current chain tip hash and height
	GetChainTip() (types.Hash, uint32, error)

	// UpdateChainState updates the consensus view of chain state
	UpdateChainState(block *types.Block) error
}

// Config contains consensus engine configuration
type Config struct {
	// PoS parameters
	StakeMinAge      time.Duration // Minimum age for stake to be valid
	StakeModifier    time.Duration // Stake modifier calculation interval
	BlockTime        time.Duration // Target time between blocks
	MaxBlockSize     int           // Maximum block size in bytes
	MinStakeAmount   uint64        // Minimum amount required to stake
	StakeReward      float64       // Annual staking reward percentage
	DifficultyWindow int           // Number of blocks for difficulty adjustment

	// Performance settings
	ValidationWorkers int // Number of worker goroutines for validation
	EventBufferSize   int // Size of event channel buffers

	// Network settings
	NetworkID string // Network identifier (mainnet, testnet, regtest)
}

// Error types for consensus operations
var (
	ErrInvalidBlock     = NewConsensusError("INVALID_BLOCK", "block validation failed")
	ErrInvalidStake     = NewConsensusError("INVALID_STAKE", "stake validation failed")
	ErrInsufficientAge  = NewConsensusError("INSUFFICIENT_AGE", "stake age below minimum")
	ErrInvalidModifier  = NewConsensusError("INVALID_MODIFIER", "invalid stake modifier")
	ErrConsensusFailure = NewConsensusError("CONSENSUS_FAILURE", "consensus operation failed")
)

// ConsensusError represents consensus-specific errors
type ConsensusError struct {
	Code    string
	Message string
	Cause   error
}

func (e *ConsensusError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func NewConsensusError(code, message string) *ConsensusError {
	return &ConsensusError{
		Code:    code,
		Message: message,
	}
}

func (e *ConsensusError) WithCause(cause error) *ConsensusError {
	e.Cause = cause
	return e
}
