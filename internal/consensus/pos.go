package consensus

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/twins-dev/twins-core/internal/storage"
	"github.com/twins-dev/twins-core/pkg/types"
)

// BlockchainInterface defines methods needed from blockchain for consensus validation
type BlockchainInterface interface {
	GetBlockHeight(hash types.Hash) (uint32, error)
	GetBestHeight() (uint32, error)
	GetBlock(hash types.Hash) (*types.Block, error)
	GetBlockByHeight(height uint32) (*types.Block, error)
	GetUTXO(outpoint types.Outpoint) (*types.UTXO, error)
	GetStakeModifier(blockHash types.Hash) (uint64, error)
	IsInitialBlockDownload() bool
	GetCheckpointManager() types.CheckpointManager
	// Batch-aware transaction lookups (for intra-batch validation)
	GetTransaction(hash types.Hash) (*types.Transaction, error)
	GetTransactionBlock(hash types.Hash) (*types.Block, error)
	// PoS metadata for stake modifier checksum chaining
	GetBlockWithPoSMetadata(hash types.Hash) (*types.Block, error)
	// Block processing (validates, stores to DB, updates UTXOs)
	ProcessBlock(block *types.Block) error
}

// ProofOfStake implements the TWINS PoS consensus mechanism
type ProofOfStake struct {
	storage    storage.Storage
	blockchain BlockchainInterface // For batch heights cache access
	params     *types.ChainParams
	logger     *logrus.Entry

	// Block validation
	validator *BlockValidator

	// Caches for performance (Go 1.25 optimized)
	modifierCache *ModifierCache
	targetCache   *TargetCache

	// Concurrent processing coordination
	validationPool *sync.Pool
	mu             sync.RWMutex

	// Event system for consensus updates
	subscribers map[EventType][]chan Event
	subMu       sync.RWMutex

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	// Staking control
	stakingActive atomic.Bool
	stakingStop   chan struct{}
	stakingWg     sync.WaitGroup

	// Staking worker and wallet integration
	// CRITICAL: Staking operates INDEPENDENTLY of masternode sync status.
	// This breaks the circular dependency that caused the legacy chain deadlock.
	wallet            StakingWalletInterface
	stakingWorker     *StakingWorker
	blockBuilder      *BlockBuilder
	consensusProvider ConsensusHeightProvider     // For network consensus height validation
	paymentValidator  *MasternodePaymentValidator // For masternode/dev fund outputs (legacy: FillBlockPayee)
	blockBroadcaster  func(*types.Block)          // For broadcasting staked blocks to P2P network
	mempool           MempoolInterface            // For including mempool transactions in staked blocks

	// Statistics
	stats *Stats
}

// PoSConsensus is an alias for ProofOfStake for compatibility
type PoSConsensus = ProofOfStake

// StakeValidationResult contains the result of stake validation
type StakeValidationResult struct {
	IsValid       bool
	StakeWeight   int64
	CoinAge       int64
	Target        *big.Int
	ProofHash     types.Hash
	KernelHash    types.Hash
	Modifier      uint64
	TargetSpacing time.Duration
}

// ValidationError represents PoS validation errors with detailed context
type ValidationError struct {
	Code    string
	Message string
	Height  uint32
	Hash    types.Hash
	Cause   error
}

func (e *ValidationError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *ValidationError) WithCause(cause error) *ValidationError {
	return &ValidationError{
		Code:    e.Code,
		Message: e.Message,
		Height:  e.Height,
		Hash:    e.Hash,
		Cause:   cause,
	}
}

// PoS specific validation errors
var (
	ErrPosInvalidStakeAge    = &ValidationError{Code: "INVALID_STAKE_AGE", Message: "stake age below minimum required"}
	ErrPosInsufficientWeight = &ValidationError{Code: "INSUFFICIENT_WEIGHT", Message: "insufficient stake weight"}
	ErrPosInvalidModifier    = &ValidationError{Code: "INVALID_MODIFIER", Message: "invalid stake modifier"}
	ErrPosTargetNotMet       = &ValidationError{Code: "TARGET_NOT_MET", Message: "stake target not met"}
	ErrPosInvalidTimestamp   = &ValidationError{Code: "INVALID_TIMESTAMP", Message: "block timestamp violation"}
	ErrPosDuplicateStake     = &ValidationError{Code: "DUPLICATE_STAKE", Message: "duplicate stake input"}
	ErrPosStakeNotMatured    = &ValidationError{Code: "STAKE_NOT_MATURED", Message: "stake not sufficiently matured"}
	ErrPosStakeAmountTooLow  = &ValidationError{Code: "STAKE_AMOUNT_TOO_LOW", Message: "stake amount below minimum required"}
	ErrTimeTooOld            = &ValidationError{Code: "TIME_TOO_OLD", Message: "block timestamp before median time past"}
	ErrTimeTooNew            = &ValidationError{Code: "TIME_TOO_NEW", Message: "block timestamp too far in future"}
)

// PoS algorithm constants (TWINS specific)
// Note: Timing parameters (StakeMinAge, TargetSpacing, etc.) are now in ChainParams
const (
	// NOTE: StakeMaxAge removed - C++ has no maximum stake age limit (kernel.cpp)
	// Old coins have no cap on age, they stake at full value weight

	// Validation parameters (non-timing)
	MedianTimeSpan        = 11         // Number of blocks for median time
	MinStakeConfirmations = uint32(60) // Minimum confirmations for stake input

	// Performance parameters
	MaxCacheSize      = 10000 // Maximum cache entries
	ValidationWorkers = 4     // Concurrent validation workers
)

// ValidationMode defines the level of PoS validation to perform
type ValidationMode int

const (
	// ValidationModeFull performs complete PoS validation (normal operation)
	// Validates: kernel hash, block signature, stake age
	ValidationModeFull ValidationMode = iota

	// ValidationModeIBD performs simplified validation during Initial Block Download
	// Validates: kernel hash only (skips signature and stake age checks)
	ValidationModeIBD

	// ValidationModeCheckpoint skips PoS validation entirely for checkpoint blocks
	// Trusts: hardcoded checkpoint hash
	ValidationModeCheckpoint
)

// NewProofOfStake creates a new PoS consensus engine
func NewProofOfStake(storage storage.Storage, params *types.ChainParams, logger *logrus.Logger) *ProofOfStake {
	pos := &ProofOfStake{
		storage: storage,
		params:  params,
		logger:  logger.WithField("component", "pos_consensus"),

		// Initialize caches with Go 1.25 performance optimizations
		// blockchain will be set later via SetBlockchain()
		modifierCache: NewModifierCache(MaxCacheSize, nil, storage, params),
		targetCache:   NewTargetCache(MaxCacheSize),

		// Pool for validation contexts to reduce GC pressure
		validationPool: &sync.Pool{
			New: func() interface{} {
				return &ValidationContext{}
			},
		},
	}

	// Create block validator
	pos.validator = NewBlockValidator(pos, storage, params)

	pos.logger.Debug("PoS consensus engine initialized")
	return pos
}

// GetBlockValidator returns the block validator instance
func (pos *ProofOfStake) GetBlockValidator() *BlockValidator {
	return pos.validator
}

// SetBlockchain sets the blockchain interface for batch heights cache access
func (pos *ProofOfStake) SetBlockchain(blockchain BlockchainInterface) {
	pos.blockchain = blockchain
	// Update modifier cache with blockchain reference
	if pos.modifierCache != nil {
		pos.modifierCache.blockchain = blockchain
	}
}

// ValidateBlock performs comprehensive PoS validation on a block
func (pos *ProofOfStake) ValidateBlock(block *types.Block) error {
	if block == nil {
		return ErrInvalidBlock.WithCause(errors.New("block is nil"))
	}

	pos.logger.WithFields(logrus.Fields{
		"hash": block.Header.Hash().String(),
	}).Debug("Starting PoS block validation")

	// Get validation context from pool
	ctx := pos.validationPool.Get().(*ValidationContext)
	defer func() {
		ctx.Reset()
		pos.validationPool.Put(ctx)
	}()

	// Initialize validation context
	if err := pos.prepareValidationContext(ctx, block); err != nil {
		return &ValidationError{
			Code:    "CONTEXT_PREPARATION_FAILED",
			Message: "failed to prepare validation context",
			Hash:    block.Header.Hash(),
			Cause:   err,
		}
	}

	// Perform comprehensive validation using the stored validator
	ctx.Flags = ValidateAll
	return pos.validator.ValidateBlock(ctx)
}

// ValidateBlockForBatch performs block validation optimized for batch processing
// Skips UTXO lookup and script verification since MarkUTXOSpent will verify UTXO existence
// This is critical for intra-batch UTXO dependencies where UTXO created in block N
// is spent in block N+1 within the same uncommitted batch
func (pos *ProofOfStake) ValidateBlockForBatch(block *types.Block) error {
	if block == nil {
		return ErrInvalidBlock.WithCause(errors.New("block is nil"))
	}

	pos.logger.WithFields(logrus.Fields{
		"hash": block.Header.Hash().String(),
	}).Debug("Starting batch-optimized block validation")

	// Get validation context from pool
	ctx := pos.validationPool.Get().(*ValidationContext)
	defer func() {
		ctx.Reset()
		pos.validationPool.Put(ctx)
	}()

	// Initialize validation context
	if err := pos.prepareValidationContext(ctx, block); err != nil {
		return &ValidationError{
			Code:    "CONTEXT_PREPARATION_FAILED",
			Message: "failed to prepare validation context",
			Hash:    block.Header.Hash(),
			Cause:   err,
		}
	}

	// Validate with SkipInputValidation flag - UTXO checks done by MarkUTXOSpent
	ctx.Flags = ValidateAll | SkipInputValidation
	return pos.validator.ValidateBlock(ctx)
}

// ValidateProofOfStake validates the PoS proof for a block using three-tier validation strategy
// DEPRECATED: Use ValidateProofOfStakeWithHeight when height is known from ValidationContext
// This method looks up height via GetBlockHeight which fails for unindexed blocks
func (pos *ProofOfStake) ValidateProofOfStake(block *types.Block) (*StakeValidationResult, error) {
	// Get block height from blockchain (may fail for unindexed blocks)
	if pos.blockchain == nil {
		return &StakeValidationResult{}, &ValidationError{
			Code:    "BLOCKCHAIN_NOT_SET",
			Message: "blockchain interface not initialized",
			Hash:    block.Header.Hash(),
		}
	}

	blockHash := block.Header.Hash()
	blockHeight, err := pos.blockchain.GetBlockHeight(blockHash)
	if err != nil {
		return &StakeValidationResult{}, &ValidationError{
			Code:    "BLOCK_HEIGHT_ERROR",
			Message: "failed to get block height - use ValidateProofOfStakeWithHeight for unindexed blocks",
			Hash:    blockHash,
			Cause:   err,
		}
	}

	return pos.ValidateProofOfStakeWithHeight(block, blockHeight)
}

// ValidateProofOfStakeWithHeight validates the PoS proof for a block with pre-computed height
// This is the preferred method when height is known from ValidationContext (prevHeight + 1)
// Matches legacy C++ which uses pindexPrev->nHeight + 1 for new block validation
//
// NOTE: zPoS (Zerocoin PoS) is intentionally NOT implemented.
// SPORK_16_ZEROCOIN_MAINTENANCE_MODE is permanently enabled on TWINS mainnet,
// which disables all zerocoin operations including zPoS staking.
// Legacy references: kernel.cpp:371-391, blocksignature.cpp:71-74
func (pos *ProofOfStake) ValidateProofOfStakeWithHeight(block *types.Block, blockHeight uint32) (*StakeValidationResult, error) {
	result := &StakeValidationResult{}

	// STEP 1: Basic structural validation (no blockchain interface needed)
	// Extract stake transaction (coinstake) from block
	// Coinstake is at index 1 (index 0 is coinbase)
	if len(block.Transactions) < 2 {
		return result, ErrInvalidBlock.WithCause(errors.New("PoS block must have at least coinbase and coinstake"))
	}

	coinstake := block.Transactions[1]
	if len(coinstake.Inputs) < 1 {
		return result, ErrInvalidBlock.WithCause(errors.New("coinstake has no inputs"))
	}

	// STEP 2: Verify blockchain interface is available for UTXO lookups
	if pos.blockchain == nil {
		return result, &ValidationError{
			Code:    "BLOCKCHAIN_NOT_SET",
			Message: "blockchain interface not initialized",
			Hash:    block.Header.Hash(),
		}
	}

	// NETWORK RECOVERY EXEMPTION: Skip PoS validation for blocks 907996-908007
	// Legacy C++ main.cpp:4320-4322 explicitly skips CheckProofOfStake for these blocks:
	//   if (pindexPrev->nHeight < 907995 || pindexPrev->nHeight > 908006) { ... }
	// This was a recovery hack to allow the network to heal from a fork in August 2022.
	// Block 908000 (checkpoint f9f9d700...) is part of this exempted range.
	if blockHeight >= 907996 && blockHeight <= 908007 {
		pos.logger.WithFields(logrus.Fields{
			"height": blockHeight,
			"hash":   block.Header.Hash().String(),
		}).Debug("Skipping PoS validation for network recovery block (legacy exemption)")
		result.IsValid = true
		return result, nil
	}

	// Determine validation mode (for logging and signature checks only)
	// CRITICAL: Stake age and min amount are ALWAYS validated per C++ kernel.cpp:306-319
	validationMode := pos.determineValidationMode(block, blockHeight)

	pos.logger.WithFields(logrus.Fields{
		"height": blockHeight,
		"mode":   validationMode,
	}).Debug("Determining PoS validation mode")

	// NOTE: Removed TIER 1 checkpoint bypass - C++ always validates stake age + min amount
	// Legacy kernel.cpp:306-319 performs these checks unconditionally

	// Get stake input information (includes MinStakeAmount check)
	stakeInput, err := pos.getStakeInput(coinstake.Inputs[0], blockHeight)
	if err != nil {
		return result, &ValidationError{
			Code:    "STAKE_INPUT_ERROR",
			Message: "failed to get stake input",
			Height:  blockHeight,
			Hash:    block.Header.Hash(),
			Cause:   err,
		}
	}

	// Calculate coin age (needed for weight calculation and logging)
	coinAge := stakeInput.GetCoinAge(block.Header.Timestamp)

	// NOTE: C++ does NOT validate min stake age at consensus level!
	// The check in kernel.cpp:311-313 is inside CheckStakeKernelHash which is called
	// from wallet's Stake() function during stake CREATION, not block VALIDATION.
	// Legacy CheckProofOfStake() in kernel.cpp:361-418 only validates the kernel hash.
	// Min age enforcement at consensus would cause chain forks on valid legacy blocks.
	// Wallet-level SelectStakeCoins() enforces min age during stake selection.

	// Calculate stake weight using block timestamp (not wall clock)
	stakeWeight := stakeInput.GetWeight(pos.params, block.Header.Timestamp)
	if stakeWeight <= 0 {
		return result, &ValidationError{
			Code:    "INSUFFICIENT_WEIGHT",
			Message: "stake weight is zero or negative",
			Height:  blockHeight,
			Hash:    block.Header.Hash(),
		}
	}

	// Get stake modifier for kernel hash validation using GetKernelStakeModifier
	// CRITICAL: This is NOT the same as getting the modifier directly from the UTXO block!
	//
	// Legacy algorithm (kernel.cpp:255-283):
	// 1. Start at UTXO source block (stakeInput.BlockHeight)
	// 2. Move FORWARD ~20 minutes (selection interval)
	// 3. Use the modifier from the LAST block in that interval
	//
	// This ensures the modifier is deterministic but unpredictable at UTXO creation time.
	stakeInputBlock, err := pos.blockchain.GetBlockByHeight(stakeInput.BlockHeight)
	if err != nil {
		return result, fmt.Errorf("failed to get block for stake input at height %d: %w", stakeInput.BlockHeight, err)
	}

	modifier, modHeight, modTime, err := pos.modifierCache.GetKernelStakeModifier(stakeInputBlock.Hash())
	if err != nil {
		return result, ErrPosInvalidModifier.WithCause(err)
	}

	// Debug logging for stake validation
	pos.logger.WithFields(logrus.Fields{
		"block_height":       blockHeight,
		"stake_input_height": stakeInput.BlockHeight,
		"stake_input_block":  stakeInputBlock.Hash().String(),
		"modifier":           modifier,
		"modifier_height":    modHeight,
		"modifier_time":      modTime,
		"stake_value":        stakeInput.Value,
		"stake_tx_hash":      stakeInput.TxHash.String(),
		"stake_index":        stakeInput.Index,
		"stake_block_time":   stakeInput.BlockTime,
		"new_block_time":     block.Header.Timestamp,
	}).Debug("Stake validation context")

	// Get target from block's difficulty bits (network difficulty from difficulty adjustment)
	// CRITICAL: Target comes from block.Header.Bits, NOT computed from weight
	// The weight is applied as a multiplier in CheckStakeKernelHash (kernel.go:93)
	target := GetTargetFromBits(block.Header.Bits)

	// ALWAYS validate kernel hash meets difficulty target (CRITICAL SECURITY CHECK)
	kernelValid, kernelHash := CheckStakeKernelHash(
		modifier,
		stakeInput,
		block.Header.Timestamp,
		target,
		pos.params,
	)

	// Fill result
	result.StakeWeight = stakeWeight
	result.CoinAge = coinAge
	result.Target = target
	result.KernelHash = kernelHash
	result.ProofHash = block.Header.Hash()
	result.Modifier = modifier
	result.TargetSpacing = pos.params.TargetSpacing

	// REJECT blocks with invalid kernel hash (no exceptions)
	if !kernelValid {
		result.IsValid = false
		pos.logger.WithFields(logrus.Fields{
			"height":       blockHeight,
			"hash":         block.Header.Hash().String(),
			"bits":         fmt.Sprintf("0x%08x", block.Header.Bits),
			"kernel_hash":  kernelHash.String(),
			"target":       target.Text(16),
			"target_len":   len(target.Text(16)),
			"modifier":     modifier,
			"stake_weight": stakeWeight,
		}).Warn("Stake kernel validation failed: kernel hash does not meet target")
		return result, nil
	}

	// NOTE: Removed IBD/checkpoint tiered validation bypass
	// C++ kernel.cpp:306-319 ALWAYS validates stake age and min amount
	// Stake age check is now performed above, before kernel hash calculation

	// Note: Block signature validation is performed separately in BlockValidator.ValidateBlock()
	// via validateBlockSignature() in validation.go. This separation ensures that PoS kernel
	// validation and cryptographic signature verification are independent validation steps.

	result.IsValid = true
	pos.logger.WithFields(logrus.Fields{
		"height":       blockHeight,
		"stake_weight": stakeWeight,
		"coin_age":     coinAge,
		"kernel_hash":  kernelHash.String(),
		"target":       target.Text(16),
	}).Debug("PoS validation successful")

	return result, nil
}

// determineValidationMode determines which validation mode to use for a block
func (pos *ProofOfStake) determineValidationMode(block *types.Block, blockHeight uint32) ValidationMode {
	// Check if blockchain interface is available for advanced checks
	if pos.blockchain == nil {
		// Without blockchain interface, use full validation (safest default)
		return ValidationModeFull
	}

	// TIER 1: Check if this is a checkpoint block
	checkpointMgr := pos.blockchain.GetCheckpointManager()
	if checkpointMgr != nil && checkpointMgr.IsCheckpointHeight(blockHeight) {
		// Verify the block hash matches the checkpoint
		expectedHash, exists := checkpointMgr.GetCheckpoint(blockHeight)
		if exists && expectedHash == block.Header.Hash() {
			return ValidationModeCheckpoint
		}
		// Block at checkpoint height but wrong hash - use full validation to reject it
	}

	// TIER 2: Check if we're in IBD mode
	if pos.blockchain.IsInitialBlockDownload() {
		return ValidationModeIBD
	}

	// TIER 3: Normal operation - full validation
	return ValidationModeFull
}


// CalculateNextWorkRequired calculates the next difficulty target
// height is the height of the block being validated (not the current chain height)
func (pos *ProofOfStake) CalculateNextWorkRequired(header *types.BlockHeader, height uint32) (uint32, error) {
	if header == nil {
		return 0, errors.New("header is nil")
	}

	// Get previous block for difficulty calculation
	// Use blockchain (batch-aware) instead of storage directly, because during
	// batch processing the previous block may only exist in the batch cache
	prevBlock, err := pos.blockchain.GetBlock(header.PrevBlockHash)
	if err != nil {
		return 0, &ValidationError{
			Code:    "PREV_BLOCK_NOT_FOUND",
			Message: "failed to get previous block",
			Height:  height,
			Hash:    header.Hash(),
			Cause:   err,
		}
	}

	// Use target calculator with the provided height (not storage height which may be stale during sync)
	// Pass blockchain for batch-aware lookups - blocks may be in batch cache during sync
	calculator := NewTargetCalculator(pos.params, pos.targetCache, pos.storage, pos.blockchain)
	nextTarget, err := calculator.CalculateNextTarget(prevBlock.Header, header, height)
	if err != nil {
		return 0, err
	}

	return GetBitsFromTarget(nextTarget), nil
}

// getStakeInput retrieves and validates stake input information
// Uses blockchain interface for batch-aware lookups during IBD
func (pos *ProofOfStake) getStakeInput(input *types.TxInput, blockHeight uint32) (*StakeInput, error) {
	// Get the transaction output being spent
	// Use blockchain interface if available (batch-aware), otherwise fall back to storage
	var prevTx *types.Transaction
	var err error
	if pos.blockchain != nil {
		prevTx, err = pos.blockchain.GetTransaction(input.PreviousOutput.Hash)
	} else {
		prevTx, err = pos.storage.GetTransaction(input.PreviousOutput.Hash)
	}
	if err != nil {
		return nil, err
	}

	if input.PreviousOutput.Index >= uint32(len(prevTx.Outputs)) {
		return nil, errors.New("output index out of range")
	}

	output := prevTx.Outputs[input.PreviousOutput.Index]

	// Get the block containing the previous transaction
	// Use blockchain interface if available (batch-aware), otherwise fall back to storage
	var txBlock *types.Block
	if pos.blockchain != nil {
		txBlock, err = pos.blockchain.GetTransactionBlock(input.PreviousOutput.Hash)
	} else {
		txBlock, err = pos.storage.GetBlockContainingTx(input.PreviousOutput.Hash)
	}
	if err != nil {
		return nil, err
	}

	// Get block height - use blockchain interface for batch-aware lookup
	var txBlockHeight uint32
	if pos.blockchain != nil {
		txBlockHeight, err = pos.blockchain.GetBlockHeight(txBlock.Header.Hash())
	} else {
		txBlockHeight, err = pos.storage.GetBlockHeight(txBlock.Header.Hash())
	}
	if err != nil {
		return nil, err
	}

	// NOTE: C++ does NOT enforce min confirmations or min stake amount at consensus level!
	// These checks are done at WALLET level (SelectStakeCoins) during stake creation.
	// Legacy blocks with "low" inputs are VALID at consensus - removing these checks
	// to match C++ behavior and prevent chain forks.
	//
	// The OUTPUT amount check (vout[1].Value >= MinStakeAmount) is done separately
	// in validation.go with proper spork + height gating per main.cpp:3975-3979.

	// Create stake input
	stakeInput := NewStakeInput(
		input.PreviousOutput,
		output,
		txBlockHeight,
		txBlock.Header.Timestamp,
	)

	return stakeInput, nil
}

// prepareValidationContext sets up the validation context for a block
func (pos *ProofOfStake) prepareValidationContext(ctx *ValidationContext, block *types.Block) error {
	ctx.Block = block

	// Get previous block to determine correct height
	// This ensures orphan/side-chain blocks use prevHeight + 1 (matches legacy)
	if !block.Header.PrevBlockHash.IsZero() {
		// Get previous block WITH PoS metadata - needed for stake modifier checksum chaining
		// Use blockchain interface if available (for batch cache with already-populated metadata)
		// Otherwise fall back to storage
		var prevBlock *types.Block
		var err error
		if pos.blockchain != nil {
			// GetBlockWithPoSMetadata loads stakeModifierChecksum and hashProofOfStake
			// These are required by computeStakeModifierChecksum for proper chaining
			prevBlock, err = pos.blockchain.GetBlockWithPoSMetadata(block.Header.PrevBlockHash)
		} else {
			prevBlock, err = pos.storage.GetBlock(block.Header.PrevBlockHash)
		}
		if err != nil {
			pos.logger.WithFields(logrus.Fields{
				"hash":  block.Header.PrevBlockHash.String(),
				"error": err.Error(),
			}).Error("Failed to get previous block GetBlockWithPoSMetadata")
			return err
		}
		ctx.PrevBlock = prevBlock

		// Get previous block height - use blockchain interface if available (for batch cache)
		// Otherwise fall back to storage
		var prevHeight uint32
		if pos.blockchain != nil {
			prevHeight, err = pos.blockchain.GetBlockHeight(block.Header.PrevBlockHash)
		} else {
			prevHeight, err = pos.storage.GetBlockHeight(block.Header.PrevBlockHash)
		}
		if err != nil {
			pos.logger.WithFields(logrus.Fields{
				"hash":  block.Header.PrevBlockHash.String(),
				"error": err.Error(),
			}).Error("Failed to get previous block GetBlockHeight")
			return err
		}
		// Height is previous block height + 1 (works for main chain and orphans)
		ctx.Height = prevHeight + 1
	} else {
		// Genesis block - height 0
		ctx.Height = 0
	}

	// Calculate median time
	medianTime, err := pos.calculateMedianTime(ctx.Height)
	if err != nil {
		pos.logger.WithFields(logrus.Fields{
			"hash":  block.Header.PrevBlockHash.String(),
			"error": err.Error(),
		}).Error("Failed to calculateMedianTime")
		return err
	}
	ctx.MedianTime = medianTime

	return nil
}

// calculateMedianTime calculates the median time of the last MedianTimeSpan blocks
func (pos *ProofOfStake) calculateMedianTime(height uint32) (uint32, error) {
	if height == 0 {
		return 0, nil
	}

	// Get timestamps from last MedianTimeSpan blocks
	timestamps := make([]uint32, 0, MedianTimeSpan)

	for i := 0; i < MedianTimeSpan && height > uint32(i); i++ {
		blockHeight := height - uint32(i) - 1
		var block *types.Block
		var err error
		if pos.blockchain != nil {
			block, err = pos.blockchain.GetBlockByHeight(blockHeight)
		} else {
			block, err = pos.storage.GetBlockByHeight(blockHeight)
		}
		if err != nil {
			if i == 0 {
				return 0, err // Must have at least the previous block
			}
			break
		}
		timestamps = append(timestamps, block.Header.Timestamp)
	}

	if len(timestamps) == 0 {
		return 0, nil
	}

	// Sort and return median
	return getMedianTimestamp(timestamps), nil
}

// GetStats returns current PoS consensus statistics
func (pos *ProofOfStake) GetStats() *Stats {
	pos.mu.RLock()
	defer pos.mu.RUnlock()

	return &Stats{
		StakingActive: pos.stakingActive.Load(),
		ActiveStakers: pos.getActiveStakerCount(),
		NetworkWeight: pos.getNetworkStakeWeight(),
		StakeModifier: pos.getCurrentStakeModifier(),
		Difficulty:    pos.getCurrentDifficulty(),
	}
}

// IsStaking returns whether the consensus engine is actively staking
func (pos *ProofOfStake) IsStaking() bool {
	return pos.stakingActive.Load()
}

// GetNetworkStakeWeight returns the estimated total network stake weight
func (pos *ProofOfStake) GetNetworkStakeWeight() int64 {
	return int64(pos.getNetworkStakeWeight())
}

// SetWallet sets the wallet interface for staking operations.
// Must be called before StartStaking().
func (pos *ProofOfStake) SetWallet(wallet StakingWalletInterface) {
	pos.mu.Lock()
	defer pos.mu.Unlock()
	pos.wallet = wallet

	// Initialize block builder if not already done
	if pos.blockBuilder == nil && pos.blockchain != nil {
		pos.blockBuilder = NewBlockBuilder(pos.blockchain, pos.params, logrus.StandardLogger())
	}

	pos.logger.Debug("Wallet set for staking")
}

// StartStaking begins the staking process.
//
// CRITICAL ARCHITECTURE DECISION:
// This function does NOT check masternode sync status.
// Staking operates INDEPENDENTLY to prevent the deadlock that killed
// the legacy C++ chain. See: legacy/src/masternode-sync.cpp:56
//
// Prerequisites checked:
// 1. Wallet must be set and unlocked
// 2. Chain must not be in IBD (height-based, NOT time-based)
//
// NOT checked (intentionally):
// - Masternode sync status
// - Time since last block (no time-based staleness checks)
func (pos *ProofOfStake) StartStaking() error {
	// Use CompareAndSwap to atomically check and set - prevents race condition
	if !pos.stakingActive.CompareAndSwap(false, true) {
		return errors.New("staking is already active")
	}

	// Check wallet is set
	pos.mu.RLock()
	wallet := pos.wallet
	pos.mu.RUnlock()

	if wallet == nil {
		pos.stakingActive.Store(false)
		return errors.New("wallet not set - call SetWallet() first")
	}

	// Check wallet is unlocked
	if wallet.IsLocked() {
		pos.stakingActive.Store(false)
		return errors.New("wallet is locked - unlock wallet before staking")
	}

	// NOTE: No IBD check here. The staking worker's own canStake() method
	// checks isAtConsensusHeight() every tick (1s), which handles IBD/sync
	// state dynamically. A one-time IBD gate at startup is harmful because
	// StartStaking() is called in Phase 5 before P2P peers have connected,
	// so IsInitialBlockDownload() returns true (peerCount < MinSyncPeers)
	// and the staking worker is never created — with no retry mechanism.

	// CRITICAL: NO masternode sync check here!
	// This is intentional - see architecture decision above.

	// Create stop channel under mutex protection
	pos.mu.Lock()
	pos.stakingStop = make(chan struct{})

	// Initialize block builder if needed
	if pos.blockBuilder == nil {
		pos.blockBuilder = NewBlockBuilder(pos.blockchain, pos.params, logrus.StandardLogger())
	}

	// Create and start staking worker
	pos.stakingWorker = NewStakingWorker(
		pos,
		wallet,
		pos.blockchain,
		pos.blockBuilder,
		pos.params,
		logrus.StandardLogger(),
		nil, // Use default config
	)

	// Inject consensus provider if available (for network sync validation)
	if pos.consensusProvider != nil {
		pos.stakingWorker.SetConsensusProvider(pos.consensusProvider)
	}

	// Inject payment validator if available (for masternode/dev fund outputs)
	// Legacy: wallet.cpp:3337 - FillBlockPayee(txNew, nMinFee, true, stakeInput->IsZTWINS())
	if pos.paymentValidator != nil {
		pos.stakingWorker.SetPaymentValidator(pos.paymentValidator)
	}

	// Inject block broadcaster if available (for P2P relay of staked blocks)
	if pos.blockBroadcaster != nil {
		pos.stakingWorker.SetBlockBroadcaster(pos.blockBroadcaster)
	}

	// Inject mempool if available (for including pending transactions in staked blocks)
	if pos.mempool != nil {
		pos.stakingWorker.SetMempool(pos.mempool)
	}
	pos.mu.Unlock()

	// Start the worker with context
	if err := pos.stakingWorker.Start(pos.ctx); err != nil {
		pos.stakingActive.Store(false)
		return fmt.Errorf("failed to start staking worker: %w", err)
	}

	pos.logger.Info("Staking enabled - worker started")
	return nil
}

// StopStaking halts the staking process
func (pos *ProofOfStake) StopStaking() error {
	if !pos.stakingActive.Load() {
		return errors.New("staking is not active")
	}

	pos.stakingActive.Store(false)

	// Stop the staking worker
	pos.mu.Lock()
	worker := pos.stakingWorker
	if pos.stakingStop != nil {
		close(pos.stakingStop)
		pos.stakingStop = nil
	}
	pos.mu.Unlock()

	// Stop worker if it exists
	if worker != nil {
		if err := worker.Stop(); err != nil {
			pos.logger.WithError(err).Warn("Error stopping staking worker")
		}
	}

	// Wait for any remaining goroutines
	pos.stakingWg.Wait()

	pos.mu.Lock()
	pos.stakingWorker = nil
	pos.mu.Unlock()

	pos.logger.Info("Staking disabled")

	return nil
}

// SetConsensusProvider sets the consensus height provider for network sync validation.
// This should be called after P2P layer is initialized to avoid import cycles.
// The provider is used by StakingWorker to verify local chain is at network consensus.
func (pos *ProofOfStake) SetConsensusProvider(provider ConsensusHeightProvider) {
	pos.mu.Lock()
	defer pos.mu.Unlock()

	pos.consensusProvider = provider

	// If staking worker already exists, update it too
	if pos.stakingWorker != nil {
		pos.stakingWorker.SetConsensusProvider(provider)
	}

	pos.logger.Debug("Consensus height provider configured for PoS")
}

// SetPaymentValidator sets the masternode payment validator for block rewards.
// This should be called after masternode manager is initialized.
// Without this, staking blocks will not include masternode/dev fund outputs (legacy: FillBlockPayee).
func (pos *ProofOfStake) SetPaymentValidator(validator *MasternodePaymentValidator) {
	pos.mu.Lock()
	defer pos.mu.Unlock()

	pos.paymentValidator = validator

	// If staking worker already exists, update it too
	if pos.stakingWorker != nil {
		pos.stakingWorker.SetPaymentValidator(validator)
	}

	pos.logger.Debug("Payment validator configured for PoS staking")
}

// SetBlockBroadcaster sets the callback to broadcast staked blocks to the P2P network.
// This should be called after P2P layer is initialized to avoid import cycles.
// The callback is typically p2p.Server.RelayBlock.
func (pos *ProofOfStake) SetBlockBroadcaster(broadcaster func(*types.Block)) {
	pos.mu.Lock()
	defer pos.mu.Unlock()

	pos.blockBroadcaster = broadcaster

	// If staking worker already exists, update it too
	if pos.stakingWorker != nil {
		pos.stakingWorker.SetBlockBroadcaster(broadcaster)
	}

	pos.logger.Debug("Block broadcaster configured for PoS staking")
}

// SetMempool sets the mempool for including pending transactions in staked blocks.
// This should be called after mempool is initialized.
// Without this, staked blocks will only contain the coinstake transaction.
func (pos *ProofOfStake) SetMempool(mp MempoolInterface) {
	pos.mu.Lock()
	defer pos.mu.Unlock()

	pos.mempool = mp

	// If staking worker already exists, update it too
	if pos.stakingWorker != nil {
		pos.stakingWorker.SetMempool(mp)
	}

	pos.logger.Debug("Mempool configured for transaction inclusion in staked blocks")
}

// GetStakingStats returns current staking statistics.
func (pos *ProofOfStake) GetStakingStats() *StakingWorkerStats {
	pos.mu.RLock()
	worker := pos.stakingWorker
	pos.mu.RUnlock()

	if worker == nil {
		return &StakingWorkerStats{
			IsRunning: false,
		}
	}

	stats := worker.GetStats()
	return &stats
}

// recentBlockWindow is the number of recent blocks analyzed for staking statistics.
const recentBlockWindow = 100

// getRecentBlocks returns the last n blocks from the chain.
func (pos *ProofOfStake) getRecentBlocks(n uint32) []*types.Block {
	height, err := pos.storage.GetChainHeight()
	if err != nil || height == 0 {
		return nil
	}

	startHeight := uint32(0)
	if height > n {
		startHeight = height - n
	}

	blocks := make([]*types.Block, 0, n)
	for blockHeight := startHeight; blockHeight <= height; blockHeight++ {
		block, err := pos.storage.GetBlockByHeight(blockHeight)
		if err != nil {
			continue
		}
		blocks = append(blocks, block)
	}
	return blocks
}

// Helper functions for statistics (simplified implementations)
func (pos *ProofOfStake) getActiveStakerCount() int {
	blocks := pos.getRecentBlocks(recentBlockWindow)

	// Count unique stakers from recent blocks
	stakers := make(map[string]bool)
	for _, block := range blocks {
		// Extract staker address from coinstake transaction
		if len(block.Transactions) > 1 {
			coinstake := block.Transactions[1]
			if len(coinstake.Outputs) > 1 {
				// Use first non-empty output as staker identifier
				for _, output := range coinstake.Outputs {
					if output.Value > 0 {
						stakers[string(output.ScriptPubKey)] = true
						break
					}
				}
			}
		}
	}

	return len(stakers)
}

func (pos *ProofOfStake) getNetworkStakeWeight() uint64 {
	blocks := pos.getRecentBlocks(recentBlockWindow)

	// Use stake.go helper function to calculate network weight
	weight := CalculateNetworkStakeWeight(blocks, pos.params)
	if weight < 0 {
		return 0
	}

	return uint64(weight)
}

// CalculateKernelHash computes the hash for a stake kernel
func (pos *ProofOfStake) CalculateKernelHash(kernel *types.StakeKernel) (types.Hash, error) {
	// Create StakeInput from kernel
	stakeInput := &StakeInput{
		TxHash:    kernel.PrevOut.Hash,
		Index:     kernel.PrevOut.Index,
		Value:     int64(kernel.StakeValue),
		BlockTime: kernel.PrevBlockTime,
	}

	// Use existing ComputeStakeKernelHash function
	hash := ComputeStakeKernelHash(stakeInput, kernel.StakeModifier, kernel.Timestamp)
	return hash, nil
}

// ValidateStakeKernel validates a stake kernel against target difficulty
func (pos *ProofOfStake) ValidateStakeKernel(kernel *types.StakeKernel, targetBits uint32) error {
	// Convert target bits to big int
	target := types.CompactToBig(targetBits)

	// Create StakeInput from kernel
	stakeInput := &StakeInput{
		TxHash:    kernel.PrevOut.Hash,
		Index:     kernel.PrevOut.Index,
		Value:     int64(kernel.StakeValue),
		BlockTime: kernel.PrevBlockTime,
	}

	// Use existing ValidateStakeKernel function
	return ValidateStakeKernel(stakeInput, kernel.StakeModifier, kernel.Timestamp, target, pos.params)
}

func (pos *ProofOfStake) getCurrentStakeModifier() uint64 {
	// Get current chain tip hash
	tipHash, err := pos.storage.GetChainTip()
	if err != nil {
		return 0
	}

	// Get tip block
	tipBlock, err := pos.storage.GetBlock(tipHash)
	if err != nil {
		return 0
	}

	// Get block height
	height, err := pos.storage.GetBlockHeight(tipHash)
	if err != nil {
		return 0
	}

	// Compute modifier using the modifier cache
	computedModifier, _, err := pos.modifierCache.ComputeNextStakeModifier(tipBlock.Header, height)
	if err != nil {
		return 0
	}

	return computedModifier
}

func (pos *ProofOfStake) getCurrentDifficulty() uint32 {
	// Get current chain tip hash
	tipHash, err := pos.storage.GetChainTip()
	if err != nil {
		return pos.params.PowLimit
	}

	// Get tip block header
	tipBlock, err := pos.storage.GetBlock(tipHash)
	if err != nil {
		return pos.params.PowLimit
	}

	// Return the bits (compact difficulty representation) from tip header
	return tipBlock.Header.Bits
}

// Engine interface methods

// Start begins the consensus engine with the given context
func (pos *ProofOfStake) Start(ctx context.Context) error {
	pos.logger.Info("Starting PoS consensus engine")

	// Initialize context for lifecycle management
	pos.ctx, pos.cancel = context.WithCancel(ctx)
	pos.done = make(chan struct{})

	// Initialize event subscribers map
	pos.subMu.Lock()
	pos.subscribers = make(map[EventType][]chan Event)
	pos.subMu.Unlock()

	// Initialize statistics
	pos.stats = &Stats{
		StakingActive: false,
		StakeModifier: pos.getCurrentStakeModifier(),
	}

	// Start background goroutine for periodic tasks
	go pos.runBackgroundTasks()

	pos.logger.Debug("PoS consensus engine started successfully")
	return nil
}

// Stop gracefully shuts down the consensus engine
func (pos *ProofOfStake) Stop() error {
	pos.logger.Debug("Stopping PoS consensus engine")

	// Stop staking worker FIRST - must finish before storage closes.
	// StopStaking blocks until the worker's current tryStake() iteration
	// completes and the goroutine exits. Without this, the worker can
	// panic accessing Pebble after Storage.Close() in node shutdown.
	if pos.stakingActive.Load() {
		if err := pos.StopStaking(); err != nil {
			pos.logger.WithError(err).Warn("Error stopping staking during shutdown")
		}
	}

	// Signal cancellation
	if pos.cancel != nil {
		pos.cancel()
	}

	// Wait for background tasks to finish
	if pos.done != nil {
		select {
		case <-pos.done:
			pos.logger.Debug("Background tasks stopped")
		case <-time.After(5 * time.Second):
			pos.logger.Warn("Timeout waiting for background tasks to stop")
		}
	}

	// Close all subscriber channels
	pos.subMu.Lock()
	for eventType, channels := range pos.subscribers {
		for _, ch := range channels {
			close(ch)
		}
		delete(pos.subscribers, eventType)
	}
	pos.subMu.Unlock()

	pos.logger.Debug("PoS consensus engine stopped successfully")
	return nil
}

// GetNextBlockTime returns when the next block can be mined
func (pos *ProofOfStake) GetNextBlockTime() time.Time {
	// Get current chain tip
	tipBlock, err := pos.storage.GetChainTip()
	if err != nil {
		// If no tip available, return default spacing from now (use chain params)
		return time.Now().Add(pos.params.TargetSpacing)
	}

	block, err := pos.storage.GetBlock(tipBlock)
	if err != nil {
		return time.Now().Add(pos.params.TargetSpacing)
	}

	// Calculate next block time based on last block timestamp + target spacing
	lastBlockTime := time.Unix(int64(block.Header.Timestamp), 0)
	nextBlockTime := lastBlockTime.Add(pos.params.TargetSpacing)

	// If next block time is in the past, return now + target spacing
	if nextBlockTime.Before(time.Now()) {
		return time.Now().Add(pos.params.TargetSpacing)
	}

	return nextBlockTime
}

// CanStake returns whether the node can participate in staking.
// Delegates to IsStaking which checks the actual staking worker state.
func (pos *ProofOfStake) CanStake() bool {
	return pos.IsStaking()
}

// GetStakeModifier returns the current stake modifier
func (pos *ProofOfStake) GetStakeModifier() uint64 {
	return pos.getCurrentStakeModifier()
}

// ComputeAndStoreModifier computes and stores stake modifier for a block in the given batch
// This should be called during block processing to ensure modifiers are persisted atomically
// Returns the computed modifier value and whether it was generated (vs inherited from previous block)
func (pos *ProofOfStake) ComputeAndStoreModifier(block *types.Block, height uint32, batch interface{}) (uint64, bool, error) {
	if block == nil {
		return 0, false, fmt.Errorf("cannot compute modifier for nil block")
	}

	// Cast batch to storage.Batch (may be nil for backward compatibility)
	var storageBatch storage.Batch
	if batch != nil {
		var ok bool
		storageBatch, ok = batch.(storage.Batch)
		if !ok {
			return 0, false, fmt.Errorf("batch parameter is not of type storage.Batch")
		}
	}

	// Get block hash
	blockHash := block.Header.Hash()

	// Compute modifier using the algorithm
	modifier, generated, err := pos.modifierCache.ComputeNextStakeModifier(block.Header, height)
	if err != nil {
		return 0, false, fmt.Errorf("failed to compute stake modifier for block %s at height %d: %w",
			blockHash.String(), height, err)
	}

	// Attach modifier to block for in-memory consumers
	block.SetStakeModifier(modifier, generated)

	// CRITICAL: Only persist modifiers when they are actually generated (not inherited)
	// This maintains compatibility with legacy C++ consensus where only generated modifiers
	// are stored. GetLastStakeModifier will search backwards for the last generated modifier.
	// Storing all modifiers would break the modifier generation timeline and cause consensus fork.
	if generated {
		switch {
		case storageBatch != nil:
			if err := storageBatch.StoreStakeModifier(blockHash, modifier); err != nil {
				return 0, false, fmt.Errorf("failed to store stake modifier in batch: %w", err)
			}
		case pos.storage != nil:
			if err := pos.storage.StoreStakeModifier(blockHash, modifier); err != nil {
				return 0, false, fmt.Errorf("failed to store stake modifier: %w", err)
			}
		}
	}

	pos.logger.WithFields(logrus.Fields{
		"height":    height,
		"hash":      blockHash.String(),
		"modifier":  fmt.Sprintf("0x%016x", modifier),
		"generated": generated,
	}).Debug("Computed stake modifier")

	return modifier, generated, nil
}

// Subscribe returns a channel for consensus events
func (pos *ProofOfStake) Subscribe(eventType EventType) <-chan Event {
	ch := make(chan Event, 10)

	pos.subMu.Lock()
	defer pos.subMu.Unlock()

	if pos.subscribers == nil {
		pos.subscribers = make(map[EventType][]chan Event)
	}

	pos.subscribers[eventType] = append(pos.subscribers[eventType], ch)

	pos.logger.WithField("event_type", eventType).Debug("New subscriber registered")

	return ch
}

// Unsubscribe removes event subscription
func (pos *ProofOfStake) Unsubscribe(eventType EventType, ch <-chan Event) {
	pos.subMu.Lock()
	defer pos.subMu.Unlock()

	channels, exists := pos.subscribers[eventType]
	if !exists {
		return
	}

	// Find and remove the channel
	for i, subscriber := range channels {
		if subscriber == ch {
			// Remove from slice
			pos.subscribers[eventType] = append(channels[:i], channels[i+1:]...)
			// Close the channel
			close(subscriber)
			pos.logger.WithField("event_type", eventType).Debug("Subscriber unregistered")
			break
		}
	}

	// Clean up empty event type entries
	if len(pos.subscribers[eventType]) == 0 {
		delete(pos.subscribers, eventType)
	}
}

// Utility functions

// emitEvent sends an event to all subscribers of that event type
func (pos *ProofOfStake) emitEvent(event Event) {
	pos.subMu.RLock()
	defer pos.subMu.RUnlock()

	channels, exists := pos.subscribers[event.Type]
	if !exists || len(channels) == 0 {
		return
	}

	// Send to all subscribers (non-blocking)
	for _, ch := range channels {
		select {
		case ch <- event:
		default:
			// Channel full, skip this subscriber
			pos.logger.Debug("Event channel full, dropping event")
		}
	}
}

// runBackgroundTasks handles periodic consensus tasks
func (pos *ProofOfStake) runBackgroundTasks() {
	defer close(pos.done)

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-pos.ctx.Done():
			pos.logger.Debug("Background tasks context cancelled")
			return

		case <-ticker.C:
			// Periodic tasks like updating statistics
			pos.mu.Lock()
			if pos.stats != nil {
				pos.stats.StakeModifier = pos.getCurrentStakeModifier()
				pos.stats.NextStakeTime = pos.GetNextBlockTime()
			}
			pos.mu.Unlock()
		}
	}
}

// getMedianTimestamp returns the median of a slice of timestamps
func getMedianTimestamp(timestamps []uint32) uint32 {
	n := len(timestamps)
	if n == 0 {
		return 0
	}

	// Simple selection sort for small arrays
	for i := 0; i < n-1; i++ {
		minIdx := i
		for j := i + 1; j < n; j++ {
			if timestamps[j] < timestamps[minIdx] {
				minIdx = j
			}
		}
		timestamps[i], timestamps[minIdx] = timestamps[minIdx], timestamps[i]
	}

	if n%2 == 1 {
		return timestamps[n/2]
	}
	return (timestamps[n/2-1] + timestamps[n/2]) / 2
}
