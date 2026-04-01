// Package blockchain implements the core blockchain state management and block processing
// for TWINS cryptocurrency. It provides:
//
// - Block processing pipeline with checkpoint validation (process.go:194-213)
// - Automatic fork detection and recovery system (recovery.go)
// - Chain integrity validation with adaptive modes (chain_validator.go)
// - Unified batch block processing for high-performance sync (unified_processor.go)
// - Typed error system for proper error handling (errors.go)
// - Wallet notification support via WalletNotifier interface (blockchain.go:70-73)
//
// Key Components:
//
// BlockChain - Main blockchain state manager with batch-aware caches
// RecoveryManager - Automatic fork detection and recovery
// CheckpointManager - Hardcoded mainnet checkpoints for fork prevention
// ChainValidator - Adaptive chain integrity validation (Quick/Smart/Full modes)
//
// Critical Patterns:
//
// All blocks are validated against hardcoded checkpoints before acceptance.
// Fork detection triggers automatic recovery to last known good checkpoint.
// ChainValidator runs adaptive validation on daemon startup (chain_validator.go).
// Parent-child relationships are cryptographically verified during processing.
// Wallet receives block notifications for transaction scanning (SetWallet).
// Single blocks at height <= bestHeight are rejected (anti-reorg Variant 3).
//
// Concurrency Model:
//
// bestHeight is atomic.Uint32 for lock-free reads from RPC/GUI/P2P threads.
// bestBlock and bestHash writes are protected by mu.Lock().
// processingMu serializes all block connection paths (sync, staking, RPC).
// Internal callers use processBatchUnifiedLocked to avoid deadlock.
//
// Error Handling:
//
// Use typed errors with errors.Is() for proper error detection:
//   - ErrBlockExists: Block already in chain (skip processing)
//   - ErrParentNotFound: Parent block missing (fatal, triggers recovery)
//   - ErrCheckpointFailed: Checkpoint validation failed (peer on wrong fork)
//   - ErrInvalidBlock: Block validation failed (consensus/crypto violation)
//   - ErrSequencingGap: Batch sequencing issue (not peer's fault)
//   - ErrHeightNotAdvancing: Block rejected (height <= best, not stored)
//   - ErrUTXONotFound: UTXO missing during validation (enables recovery)
//
// Integration:
//
// The blockchain package integrates with:
//   - consensus package for PoS validation
//   - storage package for persistent state
//   - p2p package for block synchronization
//   - wallet package for transaction scanning (optional)
//
// Startup Sequence:
//
// ChainValidator runs during daemon startup (cmd/twinsd/startup_improved.go:106-121).
// Uses Smart validation mode by default (adaptive based on chain size).
// Automatically triggers recovery on chain integrity issues.
//
// See Also:
//   - internal/CLAUDE.md for architecture overview and recent changes
//   - chain_validator.go for validation modes and auto-recovery
//   - recovery.go for fork detection and recovery details
//   - process.go for block processing pipeline
package blockchain

import (
	"context"
	"encoding/binary"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/sirupsen/logrus"
	"github.com/twins-dev/twins-core/internal/consensus"
	"github.com/twins-dev/twins-core/internal/storage"
	binaryStorage "github.com/twins-dev/twins-core/internal/storage/binary"
	"github.com/twins-dev/twins-core/pkg/types"
)

const (
	// DefaultIBDThreshold is the default number of blocks behind network tip
	// that determines when we're in Initial Block Download mode.
	// When gap >= IBDThreshold: Fast IBD (checkpoint validation only)
	// When gap < IBDThreshold: Full validation (consensus.ValidateBlock)
	// This value can be overridden via config (sync.ibdThreshold).
	DefaultIBDThreshold = 5000

	// IBDThreshold is kept as an alias for DefaultIBDThreshold for backward compatibility.
	// New code should use bc.GetIBDThreshold() or DefaultIBDThreshold.
	IBDThreshold = DefaultIBDThreshold

	// MinSyncPeers is the minimum number of connected peers required
	// to establish reliable network consensus. With fewer peers,
	// the node cannot determine if it is synced and will report as
	// not synced / in IBD mode. This prevents false sync detection
	// when connected to too few (potentially malicious) peers.
	MinSyncPeers = 3
)

// WalletNotifier is an interface for notifying wallet about new blocks and block disconnections
type WalletNotifier interface {
	NotifyBlocks(blocks []*types.Block) error
	NotifyBlockDisconnected(block *types.Block) error
}

// MasternodeNotifier is an interface for notifying masternode manager about new blocks
// This enables the masternode system to create and relay winner votes when blocks are connected.
// Legacy: Called CMasternodePayments::ProcessBlock() in masternode-payments.cpp
type MasternodeNotifier interface {
	// ProcessBlockForWinner processes a new block and creates/relays a winner vote if applicable
	// Called for each block after it's connected to the chain
	// Returns the created vote (if any) or nil if no vote was created
	ProcessBlockForWinner(currentHeight uint32) (interface{}, error)
}

// MempoolNotifier is an interface for notifying the mempool about block connections.
// Used to remove confirmed and conflicting transactions when blocks are connected.
type MempoolNotifier interface {
	RemoveConfirmedTransactions(block *types.Block)
}

// BlockAnnouncementNotifier is an interface for tracking which peers announced blocks
// This enables updating peer heights when blocks are actually saved to chain,
// solving the problem where only the peer who delivered the block gets height updated.
// Implementation lives in P2P layer (PeerHealthTracker).
type BlockAnnouncementNotifier interface {
	// NotifyBlocksProcessed notifies that blocks have been saved to the chain
	// Called after batch.Commit() in unified_processor.go
	// Implementation should update heights for all peers who announced these blocks
	NotifyBlocksProcessed(blocks []*types.Block, heights map[types.Hash]uint32)
}

// BlockChain represents the main blockchain structure
type BlockChain struct {
	config      *Config
	storage     storage.Storage
	baseStorage storage.Storage // Unwrapped storage for direct DB access (reindex operations)
	consensus   consensus.Engine
	logger      *logrus.Entry

	// Configurable IBD threshold (from sync.ibdThreshold config)
	ibdThreshold uint32

	// Chain state (modified only under processingMu)
	bestBlock    *types.Block
	bestHeight   atomic.Uint32 // atomic for lock-free reads from RPC/GUI
	bestHash     types.Hash
	mu           sync.RWMutex // Protects bestBlock and bestHash reads

	// Single-writer enforcement for all block connection paths (sync, staking, RPC).
	processingMu sync.Mutex

	// Network consensus (from P2P layer) for dynamic validation
	networkConsensusHeight uint32
	networkPeerCount       int32 // Connected peer count from P2P layer
	consensusMu            sync.RWMutex

	// Orphan management
	orphans   map[types.Hash]*types.Block
	orphansMu sync.RWMutex

	// Block request callback for requesting missing parent blocks
	onRequestBlock func(types.Hash)

	// Block index for quick lookups
	blockIndex map[types.Hash]*BlockNode
	indexMu    sync.RWMutex

	// Known blocks cache: hash → height for O(1) existence and height checks.
	// Populated from storage at startup via IterateHashToHeight, updated on
	// block connect/disconnect.  Eliminates Pebble DB lookups in the P2P inv
	// handler hot path (HasBlock / GetBlockHeight).
	knownBlocks   map[types.Hash]uint32
	knownBlocksMu sync.RWMutex

	// Checkpoint validation
	checkpoints *CheckpointManager

	// Recovery manager for automatic fork recovery
	recoveryManager *RecoveryManager

	// Shutdown coordination
	stopChan chan struct{}
	wg       sync.WaitGroup

	// Statistics
	stats   BlockchainStats
	statsMu sync.RWMutex

	// Batch processing caches (active only during processBatchUnified)
	batchCacheMu     sync.RWMutex
	batchBlocks      map[types.Hash]*types.Block
	batchHeights     map[types.Hash]uint32
	batchHashes      map[uint32]types.Hash
	batchModifiers   map[types.Hash]uint64 // Stake modifiers computed during batch
	batchMoneySupply map[uint32]int64      // Money supply per height during batch

	// Wallet notification (optional, set via SetWallet)
	wallet WalletNotifier

	// Mempool notification (optional, set via SetMempool)
	// Used to remove confirmed/conflicting transactions when blocks are connected
	mempool MempoolNotifier

	// Masternode notification (optional, set via SetMasternodeManager)
	// Used to trigger winner vote creation when blocks are connected
	masternodeManager MasternodeNotifier

	// Block announcement notification (optional, set via SetBlockAnnouncementNotifier)
	// Used to update peer heights when blocks are saved to chain
	blockAnnouncementNotifier BlockAnnouncementNotifier
}

// BlockNode represents a node in the block tree
type BlockNode struct {
	Hash      types.Hash
	Height    uint32
	Work      *types.BigInt
	Parent    *BlockNode
	Block     *types.Block
	Status    BlockStatus
	Timestamp uint32
}

// BlockStatus represents the validation status of a block
type BlockStatus int

const (
	BlockStatusNone BlockStatus = iota
	BlockStatusHeaderValid
	BlockStatusValid
	BlockStatusInvalid
	BlockStatusConnected
)

// New creates a new blockchain instance
func New(config *Config) (*BlockChain, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if config.Storage == nil {
		return nil, fmt.Errorf("storage is required")
	}

	if config.Consensus == nil {
		return nil, fmt.Errorf("consensus is required")
	}

	if config.ChainParams == nil {
		config.ChainParams = types.DefaultChainParams()
	}

	// Wrap storage with cache if enabled
	var finalStorage storage.Storage
	baseStorage := config.Storage.(storage.Storage)

	if config.EnableUTXOCache {
		cacheConfig := &storage.CachedStorageConfig{
			MaxUTXOCacheEntries: config.MaxUTXOCacheEntries,
		}
		cachedStorage, err := storage.NewCachedStorage(baseStorage, cacheConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create cached storage: %w", err)
		}
		finalStorage = cachedStorage
		logrus.WithField("cache_entries", config.MaxUTXOCacheEntries).Debug("UTXO cache enabled")
	} else {
		finalStorage = baseStorage
		logrus.Debug("UTXO cache disabled")
	}

	// Resolve IBD threshold: config value > 0 takes precedence, else default
	ibdThresh := config.IBDThreshold
	if ibdThresh == 0 {
		ibdThresh = DefaultIBDThreshold
	}

	bc := &BlockChain{
		config:       config,
		storage:      finalStorage,
		baseStorage:  baseStorage, // Store unwrapped storage for direct DB access
		consensus:    config.Consensus.(consensus.Engine),
		logger:       logrus.WithField("component", "blockchain"),
		ibdThreshold: ibdThresh,
		orphans:      make(map[types.Hash]*types.Block),
		blockIndex:   make(map[types.Hash]*BlockNode),
		knownBlocks:  make(map[types.Hash]uint32),
		checkpoints:  NewCheckpointManager(config.Network),
		stopChan:     make(chan struct{}),
	}

	// Initialize recovery manager for automatic fork recovery
	bc.recoveryManager = NewRecoveryManager(bc)
	bc.logger.Debug("Recovery manager initialized")

	// Initialize chain state
	if err := bc.loadChainState(); err != nil {
		return nil, fmt.Errorf("failed to load chain state: %w", err)
	}

	// Chain validation is now handled by ChainValidator in the improved startup sequence
	// The new validator provides adaptive validation (Smart mode) that scales better
	// for large chains and validates more thoroughly than the old limited validation

	// Set blockchain reference in consensus for batch heights cache access
	if posConsensus, ok := bc.consensus.(*consensus.ProofOfStake); ok {
		posConsensus.SetBlockchain(bc)
	}

	bc.logger.WithFields(logrus.Fields{
		"height":    bc.bestHeight.Load(),
		"best_hash": bc.bestHash.String(),
		"network":   config.Network,
	}).Info("Blockchain initialized")

	return bc, nil
}

// Start starts the blockchain processing
func (bc *BlockChain) Start() error {
	bc.logger.Info("Starting blockchain")

	// Validate our chain against checkpoints on startup
	// This detects if we're on a fork before attempting to sync
	if err := bc.ValidateChainCheckpoints(); err != nil {
		bc.logger.WithError(err).Error("Blockchain failed checkpoint validation - we are on a fork!")
		// Don't fail startup, but log the error prominently
		// The operator needs to decide whether to continue or resync
		bc.logger.Error("⚠️  WARNING: Your blockchain is on a fork! Consider resyncing from genesis or a checkpoint.")
	}

	return nil
}

// SetWallet sets the wallet notifier for blockchain events
func (bc *BlockChain) SetWallet(wallet WalletNotifier) {
	bc.wallet = wallet
}

// SetMempool sets the mempool notifier for block connection events.
// When set, confirmed and conflicting transactions are removed from the mempool
// after every block connection (both locally-staked and P2P-received blocks).
func (bc *BlockChain) SetMempool(mempool MempoolNotifier) {
	bc.mempool = mempool
}

// SetMasternodeManager sets the masternode notifier for blockchain events
// This enables automatic winner vote creation when blocks are connected.
// Legacy: Equivalent to hooking CMasternodePayments::ProcessBlock() to block connection
func (bc *BlockChain) SetMasternodeManager(mn MasternodeNotifier) {
	bc.masternodeManager = mn
}

// SetBlockAnnouncementNotifier sets the block announcement notifier
// This enables updating peer heights when blocks are saved to chain
func (bc *BlockChain) SetBlockAnnouncementNotifier(notifier BlockAnnouncementNotifier) {
	bc.blockAnnouncementNotifier = notifier
}

// Stop stops the blockchain
func (bc *BlockChain) Stop() error {
	bc.logger.Info("Stopping blockchain")

	close(bc.stopChan)
	bc.wg.Wait()

	// Save final state
	if err := bc.saveChainState(); err != nil {
		bc.logger.WithError(err).Error("Failed to save chain state")
	}

	return nil
}

// Sync flushes blockchain state to storage
func (bc *BlockChain) Sync() error {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	return bc.storage.Sync()
}

// GetBestBlock returns the current best block
func (bc *BlockChain) GetBestBlock() (*types.Block, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	if bc.bestBlock == nil {
		return nil, fmt.Errorf("no best block")
	}

	return bc.bestBlock, nil
}

// GetBestHeight returns the current blockchain height
func (bc *BlockChain) GetBestHeight() (uint32, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	return bc.bestHeight.Load(), nil
}

// GetBestBlockHash returns the hash of the best block
func (bc *BlockChain) GetBestBlockHash() (types.Hash, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	return bc.bestHash, nil
}

// ProcessBlockBatch processes multiple blocks in a single batch transaction
// This is much more efficient for syncing as it reduces database I/O
func (bc *BlockChain) ProcessBlockBatch(blocks []*types.Block) error {
	// Use unified batch processing for all blocks
	return bc.processBatchUnified(blocks)
}

// SetNetworkConsensusHeight updates the network consensus height from P2P layer
// This is used for dynamic validation - blocks within IBDThreshold of network consensus
// get full validation, while older blocks use minimal validation for faster sync
func (bc *BlockChain) SetNetworkConsensusHeight(height uint32) {
	bc.consensusMu.Lock()
	defer bc.consensusMu.Unlock()

	oldHeight := bc.networkConsensusHeight
	bc.networkConsensusHeight = height

	// Only log when height changes significantly (avoid spam)
	if height > oldHeight && (height-oldHeight >= 10 || oldHeight == 0) {
		gap := uint32(0)
		localHeight := bc.bestHeight.Load()
		if height > localHeight {
			gap = height - localHeight
		}

		bc.logger.WithFields(logrus.Fields{
			"consensus_height": height,
			"local_height":     bc.bestHeight.Load(),
			"blocks_behind":    gap,
			"ibd_mode":         gap >= bc.ibdThreshold,
		}).Debug("Network consensus height updated")
	}
}

// GetNetworkConsensusHeight returns the current network consensus height.
func (bc *BlockChain) GetNetworkConsensusHeight() uint32 {
	bc.consensusMu.RLock()
	defer bc.consensusMu.RUnlock()
	return bc.networkConsensusHeight
}

// SetNetworkPeerCount updates the connected peer count from P2P layer.
// Used by sync detection: with fewer than MinSyncPeers peers,
// the node cannot reliably determine sync status.
func (bc *BlockChain) SetNetworkPeerCount(count int32) {
	bc.consensusMu.Lock()
	defer bc.consensusMu.Unlock()
	bc.networkPeerCount = count
}

// GetCheckpointManager returns the checkpoint manager for validation
func (bc *BlockChain) GetCheckpointManager() types.CheckpointManager {
	return bc.checkpoints
}

// ValidateChainCheckpoints validates our blockchain against known checkpoints
// This detects if we're on a fork by checking that blocks at checkpoint heights
// have the expected hashes
func (bc *BlockChain) ValidateChainCheckpoints() error {
	if bc.checkpoints == nil {
		bc.logger.Debug("No checkpoints configured, skipping validation")
		return nil
	}

	bc.mu.RLock()
	defer bc.mu.RUnlock()

	validatedCount := 0
	failedCount := 0

	// Iterate through all checkpoints and validate them
	for height := uint32(0); height <= bc.checkpoints.GetLastCheckpointHeight(); height++ {
		expectedHash, isCheckpoint := bc.checkpoints.GetCheckpoint(height)
		if !isCheckpoint {
			continue
		}

		// Skip checkpoints beyond our current height
		if height > bc.bestHeight.Load() {
			continue
		}

		// Get the block hash at this height
		blockHash, err := bc.storage.GetBlockHashByHeight(height)
		if err != nil {
			// Block doesn't exist at this height - could be we haven't synced that far
			bc.logger.WithFields(logrus.Fields{
				"height": height,
				"error":  err.Error(),
			}).Debug("Cannot validate checkpoint - block not found")
			continue
		}

		// Validate the checkpoint
		if blockHash != expectedHash {
			failedCount++
			bc.logger.WithFields(logrus.Fields{
				"height":   height,
				"expected": expectedHash.String(),
				"actual":   blockHash.String(),
			}).Error("❌ Checkpoint validation FAILED - blockchain is on wrong fork!")

			// Return error on first failed checkpoint
			return fmt.Errorf("checkpoint mismatch at height %d: expected %s, got %s",
				height, expectedHash.String(), blockHash.String())
		}

		validatedCount++
		bc.logger.WithFields(logrus.Fields{
			"height": height,
			"hash":   blockHash.String(),
		}).Debug("✓ Checkpoint validated successfully")
	}

	bc.logger.WithFields(logrus.Fields{
		"validated": validatedCount,
		"failed":    failedCount,
		"total":     bc.checkpoints.GetLastCheckpointHeight(),
	}).Debug("Blockchain checkpoint validation complete")

	return nil
}

// ValidateChainSegment validates a segment of the chain by walking backwards
// This is used for periodic validation during sync to detect forks early
func (bc *BlockChain) ValidateChainSegment(fromHeight, toHeight uint32) error {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	if toHeight <= fromHeight {
		return fmt.Errorf("invalid height range: from %d to %d", fromHeight, toHeight)
	}

	bc.logger.WithFields(logrus.Fields{
		"from_height": fromHeight,
		"to_height":   toHeight,
	}).Debug("Validating chain segment")

	// Get the block hash at the end height
	currentHash, err := bc.storage.GetBlockHashByHeight(toHeight)
	if err != nil {
		return fmt.Errorf("cannot get block hash at height %d: %w", toHeight, err)
	}

	currentHeight := toHeight
	validatedCount := 0
	checkpointsValidated := 0

	// Walk backwards through the segment
	for currentHeight > fromHeight {
		// Get the block
		block, err := bc.storage.GetBlock(currentHash)
		if err != nil {
			return fmt.Errorf("chain broken at height %d: missing block %s", currentHeight, currentHash.String())
		}

		// Validate against checkpoint if this is a checkpoint height
		if bc.checkpoints != nil && bc.checkpoints.IsCheckpointHeight(currentHeight) {
			if err := bc.checkpoints.ValidateCheckpoint(currentHeight, currentHash); err != nil {
				bc.logger.WithFields(logrus.Fields{
					"height": currentHeight,
					"hash":   currentHash.String(),
					"error":  err.Error(),
				}).Error("Chain segment failed checkpoint validation!")
				return fmt.Errorf("checkpoint mismatch at height %d: %w", currentHeight, err)
			}
			checkpointsValidated++
		}

		// Move to parent
		currentHash = block.Header.PrevBlockHash
		currentHeight--
		validatedCount++

		// Log progress every 10,000 blocks
		if validatedCount%10000 == 0 {
			bc.logger.WithFields(logrus.Fields{
				"validated": validatedCount,
				"current":   currentHeight,
			}).Debug("Chain validation progress")
		}
	}

	bc.logger.WithFields(logrus.Fields{
		"blocks_validated":      validatedCount,
		"checkpoints_validated": checkpointsValidated,
		"from_height":           fromHeight,
		"to_height":             toHeight,
	}).Debug("✓ Chain segment validation successful")

	return nil
}

// GetBlock retrieves a block by hash from storage
// Only returns committed blocks - uncommitted blocks in batch cache are not visible to external callers
func (bc *BlockChain) GetBlock(hash types.Hash) (*types.Block, error) {
	// Check batch cache first for intra-batch dependencies
	// CRITICAL: Hold lock for entire operation to prevent TOCTOU race condition
	bc.batchCacheMu.RLock()
	defer bc.batchCacheMu.RUnlock()

	if bc.batchBlocks != nil {
		if block, ok := bc.batchBlocks[hash]; ok {
			return block, nil
		}
	}

	// Fall back to storage for committed blocks (lock still held)
	return bc.storage.GetBlock(hash)
}

// GetBlockWithPoSMetadata retrieves a block and loads its PoS metadata (checksum, proofHash)
// This is needed for stake modifier checksum chaining where the previous block's
// checksum and proofHash are required to compute the current block's checksum.
// For blocks in batch cache, PoS metadata is already populated during validation.
// For committed blocks, metadata is loaded from storage.
func (bc *BlockChain) GetBlockWithPoSMetadata(hash types.Hash) (*types.Block, error) {
	// Check batch cache first - blocks in cache already have PoS metadata set
	bc.batchCacheMu.RLock()
	if bc.batchBlocks != nil {
		if block, ok := bc.batchBlocks[hash]; ok {
			bc.batchCacheMu.RUnlock()
			return block, nil
		}
	}
	bc.batchCacheMu.RUnlock()

	// Get block from storage
	block, err := bc.storage.GetBlock(hash)
	if err != nil {
		return nil, err
	}

	// Load PoS metadata from storage and populate block fields
	// These fields are needed for checksum chaining in computeStakeModifierChecksum
	checksum, proofHash, err := bc.storage.GetBlockPoSMetadata(hash)
	if err == nil {
		// Successfully loaded metadata - set in block
		block.SetStakeModifierChecksum(checksum)
		block.SetHashProofOfStake(proofHash)
	} else if err != pebble.ErrNotFound {
		// Log unexpected errors but don't fail - old blocks won't have metadata
		bc.logger.WithField("hash", hash.String()).WithError(err).Warn("Failed to load PoS metadata")
	}
	// If metadata not found (old blocks before this feature), fields remain zero
	// which is correct for genesis and early PoW blocks

	// Also load stake modifier if available
	modifier, err := bc.storage.GetStakeModifier(hash)
	if err == nil {
		block.SetStakeModifier(modifier, false)
	}

	return block, nil
}

// GetBlockByHeight retrieves a block by height from storage
// Only returns committed blocks - uncommitted blocks in batch cache are not visible to external callers
func (bc *BlockChain) GetBlockByHeight(height uint32) (*types.Block, error) {
	// Check batch cache first for intra-batch dependencies
	bc.batchCacheMu.RLock()
	if bc.batchHashes != nil {
		if hash, ok := bc.batchHashes[height]; ok {
			bc.batchCacheMu.RUnlock()
			// Get block from batch cache using hash
			return bc.GetBlock(hash)
		}
	}
	bc.batchCacheMu.RUnlock()

	// Fall back to storage for committed blocks
	return bc.storage.GetBlockByHeight(height)
}

// GetUTXO retrieves a UTXO by outpoint from storage
// GetUTXO returns a committed UTXO from storage
// Note: Intra-batch UTXO lookups use IndexedBatch directly, not this method
func (bc *BlockChain) GetUTXO(outpoint types.Outpoint) (*types.UTXO, error) {
	return bc.storage.GetUTXO(outpoint)
}

// GetStakeModifier retrieves a stake modifier by block hash
// Checks batch cache first for uncommitted modifiers, then falls back to storage
func (bc *BlockChain) GetStakeModifier(blockHash types.Hash) (uint64, error) {
	// Check batch cache first for intra-batch dependencies
	// CRITICAL: Hold lock for entire operation to prevent TOCTOU race condition
	bc.batchCacheMu.RLock()
	defer bc.batchCacheMu.RUnlock()

	if bc.batchModifiers != nil {
		if modifier, ok := bc.batchModifiers[blockHash]; ok {
			return modifier, nil
		}
	}

	// Fall back to storage for committed modifiers (lock still held)
	return bc.storage.GetStakeModifier(blockHash)
}

// HasBlock checks if a block exists.
// Checks the in-memory knownBlocks cache first (O(1)) before falling back
// to Pebble storage.  This eliminates DB lookups in the P2P inv handler.
func (bc *BlockChain) HasBlock(hash types.Hash) (bool, error) {
	bc.knownBlocksMu.RLock()
	_, ok := bc.knownBlocks[hash]
	bc.knownBlocksMu.RUnlock()
	if ok {
		return true, nil
	}
	return bc.storage.HasBlock(hash)
}

// GetBlockHeight returns the height of a block by its hash.
// Checks in-memory caches (knownBlocks, then batchHeights) before falling
// back to Pebble storage.
func (bc *BlockChain) GetBlockHeight(hash types.Hash) (uint32, error) {
	// Check knownBlocks cache first (covers all committed blocks)
	bc.knownBlocksMu.RLock()
	if height, ok := bc.knownBlocks[hash]; ok {
		bc.knownBlocksMu.RUnlock()
		return height, nil
	}
	bc.knownBlocksMu.RUnlock()

	// Check batch cache for intra-batch dependencies
	bc.batchCacheMu.RLock()
	if bc.batchHeights != nil {
		if height, ok := bc.batchHeights[hash]; ok {
			bc.batchCacheMu.RUnlock()
			return height, nil
		}
	}
	bc.batchCacheMu.RUnlock()

	// Fall back to storage for committed blocks
	return bc.storage.GetBlockHeight(hash)
}

// IsOnMainChain checks if a block is on the main chain
func (bc *BlockChain) IsOnMainChain(hash types.Hash) (bool, error) {
	bc.indexMu.RLock()
	defer bc.indexMu.RUnlock()

	node, exists := bc.blockIndex[hash]
	if !exists {
		return false, nil
	}

	return node.Status == BlockStatusConnected, nil
}

// GetChainWork returns the total chain work
func (bc *BlockChain) GetChainWork() (*types.BigInt, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	bc.indexMu.RLock()
	defer bc.indexMu.RUnlock()

	if bc.bestBlock == nil {
		return types.NewBigInt(0), nil
	}

	node, exists := bc.blockIndex[bc.bestHash]
	if !exists {
		return types.NewBigInt(0), nil
	}

	return node.Work, nil
}

// GetMoneySupply retrieves the money supply at a given height from storage
func (bc *BlockChain) GetMoneySupply(height uint32) (int64, error) {
	return bc.storage.GetMoneySupply(height)
}

// GetTransaction retrieves a transaction by hash
// Batch-aware: checks blocks in current batch before storage
func (bc *BlockChain) GetTransaction(hash types.Hash) (*types.Transaction, error) {
	// Check batch cache first for intra-batch dependencies
	bc.batchCacheMu.RLock()
	if bc.batchBlocks != nil {
		for _, block := range bc.batchBlocks {
			for _, tx := range block.Transactions {
				if tx.Hash() == hash {
					bc.batchCacheMu.RUnlock()
					return tx, nil
				}
			}
		}
	}
	bc.batchCacheMu.RUnlock()

	return bc.storage.GetTransaction(hash)
}

// GetTransactionBlock retrieves the block containing a transaction
// Batch-aware: checks blocks in current batch before storage
func (bc *BlockChain) GetTransactionBlock(hash types.Hash) (*types.Block, error) {
	// Check batch cache first for intra-batch dependencies
	bc.batchCacheMu.RLock()
	if bc.batchBlocks != nil {
		for _, block := range bc.batchBlocks {
			for _, tx := range block.Transactions {
				if tx.Hash() == hash {
					bc.batchCacheMu.RUnlock()
					return block, nil
				}
			}
		}
	}
	bc.batchCacheMu.RUnlock()

	return bc.storage.GetBlockContainingTx(hash)
}

// GetUTXOsByAddress retrieves all UTXOs for an address
func (bc *BlockChain) GetUTXOsByAddress(address string) ([]*types.UTXO, error) {
	return bc.storage.GetUTXOsByAddress(address)
}

// GetOrphanBlocks returns all orphan blocks
func (bc *BlockChain) GetOrphanBlocks() []*types.Block {
	bc.orphansMu.RLock()
	defer bc.orphansMu.RUnlock()

	orphans := make([]*types.Block, 0, len(bc.orphans))
	for _, block := range bc.orphans {
		orphans = append(orphans, block)
	}

	return orphans
}

// GetChainTips returns all known chain tips
func (bc *BlockChain) GetChainTips() ([]ChainTip, error) {
	bc.indexMu.RLock()
	defer bc.indexMu.RUnlock()

	tips := make([]ChainTip, 0)

	// Find all leaf nodes in the block tree
	for hash, node := range bc.blockIndex {
		isLeaf := true
		for _, other := range bc.blockIndex {
			if other.Parent != nil && other.Parent.Hash == hash {
				isLeaf = false
				break
			}
		}

		if isLeaf {
			status := ChainTipOrphan
			if node.Status == BlockStatusConnected {
				status = ChainTipActive
			} else if node.Status == BlockStatusValid {
				status = ChainTipValidFork
			}

			tips = append(tips, ChainTip{
				Height: node.Height,
				Hash:   node.Hash,
				Work:   node.Work,
				Status: status,
			})
		}
	}

	return tips, nil
}

// loadChainState loads the blockchain state from storage
func (bc *BlockChain) loadChainState() error {
	// CRITICAL: Always check if genesis block exists first
	// Genesis must exist before we can load any chain state
	if bc.config == nil || bc.config.ChainParams == nil {
		return fmt.Errorf("blockchain config or chain params not initialized")
	}

	genesisHash := bc.config.ChainParams.GenesisHash
	hasGenesis, err := bc.storage.HasBlock(genesisHash)
	if err != nil {
		bc.logger.WithError(err).Warn("Failed to check for genesis block, assuming not present")
		hasGenesis = false
	}

	if !hasGenesis {
		// Genesis block doesn't exist - initialize it regardless of chain tip state
		bc.logger.WithField("genesis_hash", genesisHash.String()).Info("Genesis block not found in storage, initializing")
		if err := bc.initializeGenesisBlock(); err != nil {
			return fmt.Errorf("failed to initialize genesis: %w", err)
		}
		return nil
	}

	// Genesis exists, now check chain tip
	tipHash, err := bc.storage.GetChainTip()
	if err != nil {
		// No chain tip set but genesis exists - set genesis as tip
		bc.logger.Debug("No chain tip found but genesis exists, setting genesis as tip")
		bc.bestBlock, _ = bc.storage.GetBlock(genesisHash)
		bc.bestHash = genesisHash
		bc.bestHeight.Store(0)

		// Save genesis as tip
		if err := bc.storage.SetChainState(0, genesisHash); err != nil {
			bc.logger.WithError(err).Warn("Failed to set genesis chain state")
		}

		return nil
	}

	// Check if tip hash is zero (shouldn't happen if genesis exists, but handle it)
	if tipHash.IsZero() {
		bc.logger.Debug("Chain tip is zero but genesis exists, setting genesis as tip")
		bc.bestBlock, _ = bc.storage.GetBlock(genesisHash)
		bc.bestHash = genesisHash
		bc.bestHeight.Store(0)
		return nil
	}

	// Load best block
	bestBlock, err := bc.storage.GetBlock(tipHash)
	if err != nil {
		// Chain tip exists but block not found
		// This can happen during initial sync or if database is corrupt

		// Get the height that was stored
		height, heightErr := bc.storage.GetChainHeight()
		if heightErr != nil {
			height = 0 // Default to 0 if no height stored
		}

		// CRITICAL: If height is claimed to be 0, the hash MUST be genesis
		if height == 0 && tipHash != bc.config.ChainParams.GenesisHash {
			bc.logger.WithFields(logrus.Fields{
				"tip_hash":     tipHash.String(),
				"genesis_hash": bc.config.ChainParams.GenesisHash.String(),
			}).Warn("Chain tip at height 0 doesn't match genesis - reinitializing")

			// Reinitialize genesis
			return bc.initializeGenesisBlock()
		}

		bc.logger.WithFields(logrus.Fields{
			"tip_hash": tipHash.String(),
			"height":   height,
		}).Warn("Chain tip set but block not found - waiting for block from network")
		bc.bestBlock = nil
		bc.bestHash = tipHash
		bc.bestHeight.Store(height)
		return nil
	}

	// Get height from chain state (authoritative source).
	// Do NOT use GetBlockHeight(tipHash) here - that reads from hash→height index
	// which may be missing after index inconsistency recovery.
	height, err := bc.storage.GetChainHeight()
	if err != nil {
		bc.logger.WithError(err).Warn("Failed to read chain height from chain state, assuming genesis")
		height = 0
	}

	bc.bestBlock = bestBlock
	bc.bestHash = tipHash
	bc.bestHeight.Store(height)

	// Build block index
	if err := bc.buildBlockIndex(); err != nil {
		return fmt.Errorf("failed to build block index: %w", err)
	}

	bc.logger.WithFields(logrus.Fields{
		"height": height,
		"hash":   tipHash.String(),
	}).Info("Chain state loaded")

	return nil
}

// initializeGenesisBlock creates and stores the genesis block for the network
func (bc *BlockChain) initializeGenesisBlock() error {
	// Create genesis block for the configured network
	var genesis *types.Block
	switch bc.config.Network {
	case "mainnet":
		genesis = types.MainnetGenesisBlock()
	case "testnet":
		genesis = types.TestnetGenesisBlock()
	case "regtest":
		genesis = types.RegtestGenesisBlock()
	default:
		genesis = types.MainnetGenesisBlock()
	}

	if genesis == nil {
		return fmt.Errorf("failed to create genesis block for network %s", bc.config.Network)
	}

	genesisHash := genesis.Hash()
	bc.logger.WithFields(logrus.Fields{
		"network": bc.config.Network,
		"hash":    genesisHash.String(),
	}).Info("Initializing genesis block")

	// Store genesis block using ConnectBlock (which handles all indexing)
	if err := bc.ConnectBlock(genesis); err != nil {
		return fmt.Errorf("failed to connect genesis block: %w", err)
	}

	bc.logger.WithFields(logrus.Fields{
		"hash":   genesisHash.String(),
		"height": 0,
	}).Info("Genesis block initialized and stored")

	// Verify height index was created
	if _, err := bc.storage.GetBlockHashByHeight(0); err != nil {
		bc.logger.WithError(err).Warn("Genesis height index not found, manually creating")
		if err := bc.storage.StoreBlockIndex(genesisHash, 0); err != nil {
			return fmt.Errorf("failed to store genesis index: %w", err)
		}
		bc.logger.Debug("Genesis height index created successfully")
	}

	return nil
}

// saveChainState saves the blockchain state to storage
func (bc *BlockChain) saveChainState() error {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	return bc.storage.SetChainState(bc.bestHeight.Load(), bc.bestHash)
}

// buildBlockIndex initialises the in-memory block index with the current chain tip
// and populates the knownBlocks cache by iterating all hash-to-height entries from storage.
func (bc *BlockChain) buildBlockIndex() error {
	bc.indexMu.Lock()
	defer bc.indexMu.Unlock()

	if bc.bestBlock != nil {
		node := &BlockNode{
			Hash:      bc.bestHash,
			Height:    bc.bestHeight.Load(),
			Work:      types.NewBigInt(int64(bc.bestHeight.Load())), // Simplified
			Block:     bc.bestBlock,
			Status:    BlockStatusConnected,
			Timestamp: bc.bestBlock.Header.Timestamp,
		}
		bc.blockIndex[bc.bestHash] = node
	}

	// Populate knownBlocks cache from storage for O(1) HasBlock/GetBlockHeight.
	// At 1.6M blocks × 36 bytes ≈ 58MB RAM — acceptable trade-off for
	// eliminating all Pebble lookups in the P2P inv handler hot path.
	bc.knownBlocksMu.Lock()
	defer bc.knownBlocksMu.Unlock()

	var count int
	err := bc.storage.IterateHashToHeight(func(hash types.Hash, height uint32) bool {
		bc.knownBlocks[hash] = height
		count++
		return true
	})
	if err != nil {
		bc.logger.WithError(err).Warn("Failed to populate knownBlocks cache from storage")
		// Non-fatal: HasBlock/GetBlockHeight will fall back to storage
	} else {
		bc.logger.WithField("count", count).Info("Populated knownBlocks cache")
	}

	return nil
}

// cleanKnownBlocksAboveHeight removes all entries from the knownBlocks in-memory
// cache that have a height above maxValidHeight. Called after rollback to keep
// the cache consistent with storage after CleanOrphanedBlocks removes orphaned entries.
func (bc *BlockChain) cleanKnownBlocksAboveHeight(maxValidHeight uint32) {
	bc.knownBlocksMu.Lock()
	defer bc.knownBlocksMu.Unlock()

	removed := 0
	for hash, height := range bc.knownBlocks {
		if height > maxValidHeight {
			delete(bc.knownBlocks, hash)
			removed++
		}
	}

	if removed > 0 {
		bc.logger.WithFields(logrus.Fields{
			"removed":          removed,
			"max_valid_height": maxValidHeight,
		}).Debug("Cleaned knownBlocks cache entries above rollback target")
	}
}

// Additional methods to satisfy RPC BlockchainInterface

// GetBlockHash returns the hash of a block at a specific height
func (bc *BlockChain) GetBlockHash(height uint32) (types.Hash, error) {
	// Special case for genesis block - always return the canonical Quark hash
	if height == 0 {
		return bc.config.ChainParams.GenesisHash, nil
	}

	block, err := bc.GetBlockByHeight(height)
	if err != nil {
		return types.Hash{}, err
	}
	return block.Hash(), nil
}

// GetBlockCount returns the current block count as int64
func (bc *BlockChain) GetBlockCount() (int64, error) {
	height, err := bc.GetBestHeight()
	return int64(height), err
}

// GetDifficulty returns the current difficulty
func (bc *BlockChain) GetDifficulty() (float64, error) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	if bc.bestBlock == nil {
		return 1.0, nil
	}

	// Calculate difficulty from bits
	bits := bc.bestBlock.Header.Bits
	// Bitcoin genesis difficulty target (bnProofOfWorkLimit): 0x00000000FFFF << 208
	maxTarget := float64(0x00000000FFFF0000000000000000000000000000000000000000000000000000)

	exponent := bits >> 24
	mantissa := bits & 0xffffff

	var target float64
	if exponent <= 3 {
		divisor := uint(8 * (3 - exponent))
		target = float64(mantissa) / float64(uint64(1)<<divisor)
	} else {
		multiplier := uint(8 * (exponent - 3))
		target = float64(mantissa) * float64(uint64(1)<<multiplier)
	}

	if target <= 0 {
		return 0, nil
	}

	return maxTarget / target, nil
}

// ValidateBlock validates a block using the consensus engine
func (bc *BlockChain) ValidateBlock(block *types.Block) error {
	return bc.consensus.ValidateBlock(block)
}

// GetRawTransaction returns a transaction as serialized bytes
func (bc *BlockChain) GetRawTransaction(hash types.Hash) ([]byte, error) {
	tx, err := bc.GetTransaction(hash)
	if err != nil {
		return nil, err
	}
	return tx.Serialize()
}

// SetBlockRequestCallback sets the callback function for requesting missing blocks
func (bc *BlockChain) SetBlockRequestCallback(cb func(types.Hash)) {
	bc.onRequestBlock = cb
}

// GetChainParams returns the chain parameters
func (bc *BlockChain) GetChainParams() *types.ChainParams {
	return bc.config.ChainParams
}

// IsInitialBlockDownload returns whether we're still in IBD mode
// IBD mode uses checkpoint-based validation for speed
// After IBD (gap < IBDThreshold), we switch to full consensus validation
// IsInitialBlockDownload checks if we're in initial block download
func (bc *BlockChain) IsInitialBlockDownload() bool {
	if bc.bestBlock == nil {
		return true
	}

	bc.consensusMu.RLock()
	networkHeight := bc.networkConsensusHeight
	peerCount := bc.networkPeerCount
	bc.consensusMu.RUnlock()

	// Not enough peers to establish reliable consensus
	if peerCount < MinSyncPeers {
		return true
	}

	// If consensus height is not set yet, we're in bootstrap/early IBD
	if networkHeight == 0 {
		return true
	}

	// Calculate gap between our height and network consensus
	var gap uint32
	localH := bc.bestHeight.Load()
	if networkHeight > localH {
		gap = networkHeight - localH
	}

	// IBD threshold determines validation mode
	// gap >= ibdThreshold: Fast IBD mode (checkpoint validation only)
	// gap < ibdThreshold: Full validation mode (consensus.ValidateBlock)
	return gap >= bc.ibdThreshold
}

// GetIBDThreshold returns the configured IBD threshold.
// Used by P2P sync layer to match blockchain's IBD detection.
func (bc *BlockChain) GetIBDThreshold() uint32 {
	return bc.ibdThreshold
}

// GetVerificationProgress returns the verification progress (0.0 to 1.0)
func (bc *BlockChain) GetVerificationProgress() float64 {
	if bc.IsInitialBlockDownload() {
		// Calculate progress based on current height vs expected chain length
		currentHeight, err := bc.GetBestHeight()
		if err != nil {
			return 0.0
		}

		// Estimate target height based on genesis time and block interval
		// This is an approximation - checkpoints would be more accurate
		genesisTime := bc.config.ChainParams.GenesisTime
		currentTime := time.Now().Unix()
		elapsedTime := currentTime - genesisTime
		targetSpacing := int64(bc.config.ChainParams.TargetSpacing.Seconds())

		if targetSpacing > 0 && elapsedTime > 0 {
			estimatedHeight := uint32(elapsedTime / targetSpacing)
			if estimatedHeight > 0 {
				progress := float64(currentHeight) / float64(estimatedHeight)
				if progress > 1.0 {
					progress = 1.0
				}
				return progress
			}
		}

		return 0.0
	}
	return 1.0
}

// GetUTXOSet returns the entire UTXO set as a map.
// Not implemented: use GetUTXO() for individual lookups instead.
func (bc *BlockChain) GetUTXOSet() (map[types.Outpoint]*types.TxOutput, error) {
	return nil, fmt.Errorf("GetUTXOSet not implemented: use GetUTXO for individual lookups")
}

// GetBlockReward returns the block reward for a given height
// DEPRECATED: This function previously used an incorrect halving formula.
// Use consensus.GetBlockValue() for correct TWINS reward schedule.
// This function now delegates to the consensus module.
func (bc *BlockChain) GetBlockReward(height uint32) int64 {
	// Delegate to consensus module's correct implementation
	// The correct TWINS reward schedule is height-based, not halving-based.
	// See internal/consensus/validation.go:GetBlockValue() for the full schedule.
	return getBlockValueFromConsensus(height)
}

// getBlockValueFromConsensus returns the correct TWINS block reward schedule.
// This is a copy of consensus.GetBlockValue() to avoid import cycles.
// The canonical implementation is in internal/consensus/validation.go
func getBlockValueFromConsensus(height uint32) int64 {
	const COIN = 100000000 // 1 TWINS = 100000000 satoshis

	// First block with initial pre-mine
	if height == 1 {
		return 6000000 * COIN // 6 million TWINS premine
	}

	// Legacy TWINS reward schedule
	if height < 711111 {
		return int64(15220.70 * COIN)
	}

	// Phased reduction schedule
	switch {
	case height < 716666:
		return 8000 * COIN
	case height < 722222:
		return 4000 * COIN
	case height < 727777:
		return 2000 * COIN
	case height < 733333:
		return 1000 * COIN
	case height < 738888:
		return 500 * COIN
	case height < 744444:
		return 250 * COIN
	case height < 750000:
		return 125 * COIN
	case height < 755555:
		return 60 * COIN
	case height < 761111:
		return 30 * COIN
	case height < 766666:
		return 15 * COIN
	case height < 772222:
		return 8 * COIN
	case height < 777777:
		return 4 * COIN
	case height < 910000:
		return 2 * COIN
	case height < 6569605:
		return 100 * COIN
	default:
		return 0 // No more rewards after block 6569605
	}
}

// GetMaxBlockFees returns the maximum fees allowed in a block
func (bc *BlockChain) GetMaxBlockFees() int64 {
	const COIN = 100000000       // 1 TWINS = 100,000,000 satoshis
	const maxBlockFeeTWINS = 1000000 // 1M TWINS cap
	return int64(maxBlockFeeTWINS) * COIN
}

// GetMedianTimePast returns the median timestamp of the last 11 blocks
func (bc *BlockChain) GetMedianTimePast(height uint32) (time.Time, error) {
	// Bitcoin uses median of last 11 blocks
	const medianTimeSpan = 11

	timestamps := make([]int64, 0, medianTimeSpan)

	currentHeight := height
	for i := 0; i < medianTimeSpan && currentHeight > 0; i++ {
		block, err := bc.GetBlockByHeight(currentHeight)
		if err != nil {
			// If we can't get enough blocks, use what we have
			break
		}
		timestamps = append(timestamps, int64(block.Header.Timestamp))
		currentHeight--
	}

	if len(timestamps) == 0 {
		return time.Time{}, fmt.Errorf("no blocks available for median time calculation")
	}

	// Sort timestamps
	sort.Slice(timestamps, func(i, j int) bool {
		return timestamps[i] < timestamps[j]
	})

	// Get median
	var medianTime int64
	if len(timestamps)%2 == 0 {
		// Even number of elements, average the two middle ones
		mid := len(timestamps) / 2
		medianTime = (timestamps[mid-1] + timestamps[mid]) / 2
	} else {
		// Odd number of elements, take the middle one
		medianTime = timestamps[len(timestamps)/2]
	}

	return time.Unix(medianTime, 0), nil
}

// TriggerRecovery attempts automatic recovery from a detected fork or inconsistency
func (bc *BlockChain) TriggerRecovery(errorHeight uint32, err error) error {
	// Skip recovery during IBD - errors should result in re-requesting blocks
	// Recovery is only meaningful when we're synced and receive conflicting blocks
	if bc.IsInitialBlockDownload() {
		bc.logger.WithFields(logrus.Fields{
			"error_height": errorHeight,
			"error":        err.Error(),
		}).Debug("Skipping fork recovery during IBD - will re-request blocks")
		return nil // Not an error, just skip recovery
	}

	if bc.recoveryManager == nil {
		return fmt.Errorf("recovery manager not initialized")
	}

	// Check if recovery should be attempted for this error
	if !bc.recoveryManager.ShouldRecover(err) {
		return fmt.Errorf("error not recoverable: %w", err)
	}

	// Try to extract the actual error height from checkpoint validation errors
	actualErrorHeight := errorHeight
	errStr := err.Error()
	if strings.Contains(errStr, "checkpoint validation failed at height") ||
		strings.Contains(errStr, "checkpoint mismatch at height") {
		// Parse the height from error message like "checkpoint validation failed at height 280000"
		if height := extractHeightFromError(errStr); height > 0 {
			actualErrorHeight = height
			bc.logger.WithFields(logrus.Fields{
				"extracted_height": actualErrorHeight,
				"provided_height":  errorHeight,
			}).Debug("Extracted checkpoint height from error message")
		}
	}

	bc.logger.WithFields(logrus.Fields{
		"error_height": actualErrorHeight,
		"error":        err.Error(),
	}).Warn("Triggering automatic recovery...")

	// Attempt recovery
	if recoveryErr := bc.recoveryManager.RecoverFromFork(actualErrorHeight); recoveryErr != nil {
		bc.logger.WithError(recoveryErr).Error("Automatic recovery failed")
		return fmt.Errorf("recovery failed: %w", recoveryErr)
	}

	bc.logger.Info("✓ Automatic recovery successful")
	return nil
}

// TriggerRecoveryForCorruptBlock attempts automatic recovery from a corrupt block
// Unlike TriggerRecovery, this method works even during IBD because corrupt blocks
// (header exists but transactions missing) cannot be fixed by re-requesting from peers.
// The block data in storage is incomplete and needs rollback + re-sync.
//
// This uses RecoverFromCorruptBlock which directly rolls back to height-1 without
// the complex findActualCorruptionPoint logic that doesn't detect missing transactions.
func (bc *BlockChain) TriggerRecoveryForCorruptBlock(errorHeight uint32, err error) error {
	// NOTE: No IBD skip here - corrupt blocks need recovery even during sync
	// This is different from TriggerRecovery which skips IBD assuming re-request will fix it

	if bc.recoveryManager == nil {
		return fmt.Errorf("recovery manager not initialized")
	}

	// Check if recovery should be attempted for this error
	if !bc.recoveryManager.ShouldRecover(err) {
		return fmt.Errorf("error not recoverable: %w", err)
	}

	bc.logger.WithFields(logrus.Fields{
		"error_height": errorHeight,
		"error":        err.Error(),
	}).Warn("Triggering recovery for corrupt block (missing transactions)...")

	// Use direct corrupt block recovery - simpler and more reliable than RecoverFromFork
	// RecoverFromFork's findActualCorruptionPoint doesn't detect missing transactions
	if recoveryErr := bc.recoveryManager.RecoverFromCorruptBlock(errorHeight); recoveryErr != nil {
		bc.logger.WithError(recoveryErr).Error("Automatic recovery failed for corrupt block")
		return fmt.Errorf("recovery failed: %w", recoveryErr)
	}

	bc.logger.Info("✓ Automatic recovery successful for corrupt block")
	return nil
}

// extractHeightFromError attempts to parse a height value from an error message
func extractHeightFromError(errStr string) uint32 {
	// Look for patterns like "at height 280000" or "height 280000"
	patterns := []string{
		"at height ",
		"height ",
	}

	for _, pattern := range patterns {
		if idx := strings.Index(errStr, pattern); idx != -1 {
			// Start after the pattern
			start := idx + len(pattern)
			// Find the end (next non-digit character)
			end := start
			for end < len(errStr) && errStr[end] >= '0' && errStr[end] <= '9' {
				end++
			}
			if end > start {
				// Parse the number
				heightStr := errStr[start:end]
				if height, err := strconv.ParseUint(heightStr, 10, 32); err == nil {
					return uint32(height)
				}
			}
		}
	}
	return 0
}

// GetRecoveryManager returns the recovery manager
func (bc *BlockChain) GetRecoveryManager() *RecoveryManager {
	return bc.recoveryManager
}

// TriggerRecoveryForIndexInconsistency runs a full chain index consistency check
// and performs recovery if inconsistencies are found. This is triggered when
// "failed to mark UTXO as spent" errors occur during block processing, as they
// may indicate block index corruption (hash→height vs height→hash mismatch).
// Unlike TriggerRecovery which rolls back a specific height, this checks the
// entire chain's index integrity first.
//
// During IBD, UTXO errors are more commonly caused by batch sequencing than actual
// index corruption, so we skip the expensive index scan and fall back to standard recovery.
func (bc *BlockChain) TriggerRecoveryForIndexInconsistency(ctx context.Context, triggerErr error) error {
	// During IBD, skip expensive full index scan - fall back to standard recovery
	// which itself will skip during IBD (re-request from peers instead)
	if bc.IsInitialBlockDownload() {
		bc.logger.WithError(triggerErr).Debug("Skipping index consistency check during IBD - falling back to standard recovery")
		currentHeight, _ := bc.GetBestHeight()
		return bc.TriggerRecovery(currentHeight, triggerErr)
	}

	bc.logger.WithError(triggerErr).Warn("UTXO error detected - running full chain index consistency check...")

	validator := NewChainValidator(bc)
	inconsistencies, err := validator.CheckIndexConsistency(ctx)
	if err != nil {
		bc.logger.WithError(err).Error("Index consistency check failed")
		return fmt.Errorf("index consistency check failed: %w", err)
	}

	if len(inconsistencies) == 0 {
		bc.logger.Info("No index inconsistencies found - falling back to standard recovery")
		// No index issues found, fall back to standard height-based recovery
		currentHeight, _ := bc.GetBestHeight()
		return bc.TriggerRecovery(currentHeight, triggerErr)
	}

	// Index inconsistencies found - recover
	bc.logger.WithField("inconsistencies", len(inconsistencies)).Warn("Index inconsistencies detected, recovering...")
	if err := validator.RecoverFromIndexInconsistency(ctx, inconsistencies); err != nil {
		return fmt.Errorf("index inconsistency recovery failed: %w", err)
	}

	bc.logger.Info("✓ Index inconsistency recovery completed")
	return nil
}

// TriggerRecoveryForRepeatedValidationFailure performs a targeted index consistency
// check around a specific height and recovers if inconsistencies are found.
// Called by the P2P sync layer when the same block fails validation from multiple
// peers, indicating local index corruption rather than a peer problem.
func (bc *BlockChain) TriggerRecoveryForRepeatedValidationFailure(ctx context.Context, failHeight uint32) error {
	bc.logger.WithField("fail_height", failHeight).Warn("Repeated validation failure detected - running targeted index consistency check...")

	validator := NewChainValidator(bc)

	// Check a small radius around the failing height.
	// Index corruption typically affects a small range.
	const reactiveCheckRadius = uint32(10)
	inconsistencies, err := validator.CheckIndexConsistencyAroundHeight(ctx, failHeight, reactiveCheckRadius)
	if err != nil {
		bc.logger.WithError(err).Error("Targeted index consistency check failed")
		return fmt.Errorf("targeted index consistency check failed: %w", err)
	}

	if len(inconsistencies) == 0 {
		// No index issues found in the targeted range. Run a full scan as fallback
		// since the corruption might be further from the failing height.
		// Note: This runs even during IBD because reactive recovery only triggers
		// after 3+ unique peers fail on the same block, ruling out batch sequencing.
		bc.logger.Info("No targeted inconsistencies found, running full index scan...")
		inconsistencies, err = validator.CheckIndexConsistency(ctx)
		if err != nil {
			bc.logger.WithError(err).Error("Full index consistency check failed")
			return fmt.Errorf("full index consistency check failed: %w", err)
		}
	}

	if len(inconsistencies) == 0 {
		// No index inconsistencies found. This may indicate a stale fork rather
		// than index corruption. Fall back to RecoverFromFork which will perform
		// a rollback from the tip if the chain is internally consistent.
		// Note: Runs during IBD too — reactive recovery requires 3+ unique peers
		// to fail on the same block, which rules out batch sequencing issues.
		bc.logger.Warn("No index inconsistencies found after reactive check - attempting stale fork recovery")
		if bc.recoveryManager != nil {
			if recoveryErr := bc.recoveryManager.RecoverFromFork(failHeight); recoveryErr != nil {
				bc.logger.WithError(recoveryErr).Error("Stale fork recovery also failed after reactive check")
				return fmt.Errorf("no index inconsistencies and fork recovery failed at height %d: %w", failHeight, recoveryErr)
			}
			bc.logger.Info("✓ Stale fork recovery successful after reactive check")
			return nil
		}
		return fmt.Errorf("no index inconsistencies found at height %d", failHeight)
	}

	// Index inconsistencies found - recover
	bc.logger.WithFields(logrus.Fields{
		"inconsistencies": len(inconsistencies),
		"fail_height":     failHeight,
	}).Warn("Index inconsistencies detected near failing height, recovering...")

	if err := validator.RecoverFromIndexInconsistency(ctx, inconsistencies); err != nil {
		return fmt.Errorf("index inconsistency recovery failed: %w", err)
	}

	bc.logger.Info("✓ Reactive index inconsistency recovery completed")
	return nil
}

// FlushUTXOCache clears the UTXO LRU cache, forcing subsequent reads to go to storage.
// Called by the P2P sync layer after stall detection to eliminate stale cache entries
// that may cause repeated validation failures across sync restarts.
func (bc *BlockChain) FlushUTXOCache() {
	if cs, ok := bc.storage.(*storage.CachedStorage); ok {
		cs.FlushCache()
		bc.logger.Info("UTXO cache flushed due to sync stall recovery")
	}
}

// RebuildAddressUTXOIndexResult contains the result of address UTXO index rebuild
type RebuildAddressUTXOIndexResult struct {
	UTXOsProcessed uint64
	IndexesCreated uint64
	Duration       time.Duration
	Error          error
}

// RebuildAddressUTXOIndex rebuilds the AddressUTXO index by iterating through all UTXOs
// This is much faster than reindexing block-by-block since we only need to read UTXOs
func (bc *BlockChain) RebuildAddressUTXOIndex() <-chan RebuildAddressUTXOIndexResult {
	resultChan := make(chan RebuildAddressUTXOIndexResult, 1)

	go func() {
		defer close(resultChan)

		startTime := time.Now()
		bc.logger.Info("Starting AddressUTXO index rebuild...")

		// Get the underlying Pebble database from baseStorage (unwrapped)
		binStorage, ok := bc.baseStorage.(*binaryStorage.BinaryStorage)
		if !ok {
			resultChan <- RebuildAddressUTXOIndexResult{
				Error: fmt.Errorf("base storage is not BinaryStorage, cannot rebuild index (type: %T)", bc.baseStorage),
			}
			return
		}

		db := binStorage.GetDB()

		// Step 1: Delete old AddressUTXO index entries
		bc.logger.Debug("Deleting old AddressUTXO index entries...")
		if err := bc.deleteAddressUTXOIndex(db); err != nil {
			resultChan <- RebuildAddressUTXOIndexResult{
				Error: fmt.Errorf("failed to delete old index: %w", err),
			}
			return
		}

		// Step 2: Iterate through all UTXOs and rebuild index
		bc.logger.Debug("Rebuilding AddressUTXO index from UTXOs...")

		utxoPrefix := []byte{binaryStorage.PrefixUTXOExist}
		utxoUpperBound := []byte{binaryStorage.PrefixUTXOExist + 1}

		iter, err := db.NewIter(&pebble.IterOptions{
			LowerBound: utxoPrefix,
			UpperBound: utxoUpperBound,
		})
		if err != nil {
			resultChan <- RebuildAddressUTXOIndexResult{
				Error: fmt.Errorf("failed to create UTXO iterator: %w", err),
			}
			return
		}
		defer iter.Close()

		batch := db.NewBatch()
		var utxosProcessed, indexesCreated uint64
		const batchSize = 1000

		for iter.First(); iter.Valid(); iter.Next() {
			utxosProcessed++

			// Parse UTXO key to get outpoint
			utxoKey := iter.Key()
			if len(utxoKey) < 1+32+2 { // 35 bytes: prefix(1) + txHash(32) + index(2)
				bc.logger.Warnf("Invalid UTXO key length: %d", len(utxoKey))
				continue
			}

			// Extract txHash and index from key immediately (before iter.Next())
			// Format: 0x03 + txHash(32) + index(2)
			var txHash types.Hash
			copy(txHash[:], utxoKey[1:33])
			index := uint32(binary.LittleEndian.Uint16(utxoKey[33:35])) // Read uint16, cast to uint32

			// Parse UTXO data to get scriptPubKey and height
			utxoData := iter.Value()
			if len(utxoData) < 8+2+4+1 { // Minimum 15 bytes: amount(8) + scriptLen(2) + height(4) + isCoinbase(1)
				bc.logger.Warnf("Invalid UTXO data length: %d for tx %s:%d", len(utxoData), txHash, index)
				continue
			}

			// UTXO format: amount(8) + scriptPubKeyLen(2) + scriptPubKey + height(4) + isCoinbase(1)
			scriptPubKeyLen := binary.LittleEndian.Uint16(utxoData[8:10]) // Read 2 bytes, not 4
			if len(utxoData) < int(10+scriptPubKeyLen+4+1) {              // Adjusted offsets
				bc.logger.Warnf("UTXO data too short for scriptPubKey: %d", len(utxoData))
				continue
			}

			// Make defensive copy of scriptPubKey before iterator advances
			scriptPubKey := make([]byte, scriptPubKeyLen)
			copy(scriptPubKey, utxoData[10:10+scriptPubKeyLen]) // Start at offset 10, not 12
			height := binary.LittleEndian.Uint32(utxoData[10+scriptPubKeyLen : 10+scriptPubKeyLen+4])

			// Analyze script to extract address hash
			scriptType, scriptData := binaryStorage.AnalyzeScript(scriptPubKey)

			// Only create index for known script types
			if scriptType == binaryStorage.ScriptTypeUnknown || scriptData == [20]byte{} {
				continue
			}

			// Create AddressUTXO index key
			// Format: 0x05 + scriptHash(20) + height(4) + txHash(32) + index(2)
			addrKey := make([]byte, 59)
			addrKey[0] = binaryStorage.PrefixAddressUTXO
			copy(addrKey[1:21], scriptData[:])
			binary.LittleEndian.PutUint32(addrKey[21:25], height)
			copy(addrKey[25:57], txHash[:])
			binary.LittleEndian.PutUint16(addrKey[57:59], uint16(index))

			// Store with empty value (key contains all info)
			if err := batch.Set(addrKey, []byte{}, nil); err != nil {
				resultChan <- RebuildAddressUTXOIndexResult{
					UTXOsProcessed: utxosProcessed,
					IndexesCreated: indexesCreated,
					Duration:       time.Since(startTime),
					Error:          fmt.Errorf("failed to set AddressUTXO index: %w", err),
				}
				return
			}
			indexesCreated++

			// Commit batch periodically
			if utxosProcessed%batchSize == 0 {
				if err := batch.Commit(pebble.Sync); err != nil {
					resultChan <- RebuildAddressUTXOIndexResult{
						UTXOsProcessed: utxosProcessed,
						IndexesCreated: indexesCreated,
						Duration:       time.Since(startTime),
						Error:          fmt.Errorf("failed to commit batch at %d UTXOs: %w", utxosProcessed, err),
					}
					return
				}
				batch = db.NewBatch()

				// Log progress every 10,000 UTXOs
				if utxosProcessed%10000 == 0 {
					elapsed := time.Since(startTime)
					rate := float64(utxosProcessed) / elapsed.Seconds()
					bc.logger.WithFields(logrus.Fields{
						"utxos":   utxosProcessed,
						"indexes": indexesCreated,
						"rate":    fmt.Sprintf("%.0f UTXO/sec", rate),
						"elapsed": elapsed.Round(time.Second),
					}).Debug("AddressUTXO index rebuild progress")
				}
			}
		}

		// Commit final batch
		if batch.Count() > 0 {
			if err := batch.Commit(pebble.Sync); err != nil {
				resultChan <- RebuildAddressUTXOIndexResult{
					UTXOsProcessed: utxosProcessed,
					IndexesCreated: indexesCreated,
					Duration:       time.Since(startTime),
					Error:          fmt.Errorf("failed to commit final batch: %w", err),
				}
				return
			}
		}

		// Check for iteration errors
		if err := iter.Error(); err != nil {
			resultChan <- RebuildAddressUTXOIndexResult{
				UTXOsProcessed: utxosProcessed,
				IndexesCreated: indexesCreated,
				Duration:       time.Since(startTime),
				Error:          fmt.Errorf("UTXO iteration error: %w", err),
			}
			return
		}

		duration := time.Since(startTime)
		rate := float64(utxosProcessed) / duration.Seconds()

		bc.logger.WithFields(logrus.Fields{
			"utxos":    utxosProcessed,
			"indexes":  indexesCreated,
			"rate":     fmt.Sprintf("%.0f UTXO/sec", rate),
			"duration": duration.Round(time.Second),
		}).Info("✓ AddressUTXO index rebuild completed successfully")

		resultChan <- RebuildAddressUTXOIndexResult{
			UTXOsProcessed: utxosProcessed,
			IndexesCreated: indexesCreated,
			Duration:       duration,
			Error:          nil,
		}
	}()

	return resultChan
}

// deleteAddressUTXOIndex deletes all AddressUTXO index entries
func (bc *BlockChain) deleteAddressUTXOIndex(db *pebble.DB) error {
	prefix := []byte{binaryStorage.PrefixAddressUTXO}
	upperBound := []byte{binaryStorage.PrefixAddressUTXO + 1}

	iter, err := db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: upperBound,
	})
	if err != nil {
		return fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close()

	batch := db.NewBatch()
	var count uint64
	const batchSize = 10000

	for iter.First(); iter.Valid(); iter.Next() {
		if err := batch.Delete(iter.Key(), nil); err != nil {
			return fmt.Errorf("failed to delete key: %w", err)
		}
		count++

		if count%batchSize == 0 {
			if err := batch.Commit(pebble.Sync); err != nil {
				return fmt.Errorf("failed to commit delete batch: %w", err)
			}
			batch = db.NewBatch()
			bc.logger.Debugf("Deleted %d old AddressUTXO entries", count)
		}
	}

	// Commit final batch
	if batch.Count() > 0 {
		if err := batch.Commit(pebble.Sync); err != nil {
			return fmt.Errorf("failed to commit final delete batch: %w", err)
		}
	}

	if err := iter.Error(); err != nil {
		return fmt.Errorf("iterator error during deletion: %w", err)
	}

	bc.logger.Debugf("Deleted %d old AddressUTXO index entries", count)
	return nil
}
