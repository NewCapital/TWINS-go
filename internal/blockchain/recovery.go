package blockchain

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/twins-dev/twins-core/pkg/types"
)

// RecoveryState represents the current state of the recovery process
type RecoveryState int

const (
	StateNormal RecoveryState = iota
	StateInconsistencyDetected
	StateRollingBack
	StateRecovering
	StateRecoveryFailed
)

func (s RecoveryState) String() string {
	switch s {
	case StateNormal:
		return "normal"
	case StateInconsistencyDetected:
		return "inconsistency_detected"
	case StateRollingBack:
		return "rolling_back"
	case StateRecovering:
		return "recovering"
	case StateRecoveryFailed:
		return "recovery_failed"
	default:
		return "unknown"
	}
}

// RecoveryMetrics tracks recovery statistics
type RecoveryMetrics struct {
	ForkDetections   int
	RecoveryAttempts int
	RecoverySuccess  int
	RecoveryFailures int
	LastForkHeight   uint32
	LastRecoveryTime time.Time
}

// RecoveryManager handles automatic recovery from forks and inconsistencies
type RecoveryManager struct {
	blockchain         *BlockChain
	mu                 sync.RWMutex // Protects state and attempts from concurrent access
	state              RecoveryState
	attempts           int
	maxAttempts        int
	staleForkRollbacks uint32 // consecutive stale fork rollbacks for progressive depth
	metrics            RecoveryMetrics
	logger             *logrus.Entry
}

// NewRecoveryManager creates a new recovery manager
func NewRecoveryManager(blockchain *BlockChain) *RecoveryManager {
	return &RecoveryManager{
		blockchain:  blockchain,
		state:       StateNormal,
		maxAttempts: 3,
		logger:      logrus.WithField("component", "recovery_manager"),
	}
}

// GetState returns the current recovery state
func (rm *RecoveryManager) GetState() RecoveryState {
	return rm.state
}

// GetMetrics returns recovery metrics
func (rm *RecoveryManager) GetMetrics() RecoveryMetrics {
	return rm.metrics
}

// FindForkPoint finds where the chain diverged from the correct chain
func (rm *RecoveryManager) FindForkPoint(errorHeight uint32) (uint32, types.Hash, error) {
	rm.logger.WithField("error_height", errorHeight).Debug("Finding fork point...")

	// Start from the error height and walk backwards
	currentHeight := errorHeight

	// Don't go below genesis
	minHeight := uint32(0)

	// If we have checkpoints, use the nearest one as a lower bound
	if rm.blockchain.checkpoints != nil {
		checkpointHeight, _, found := rm.blockchain.checkpoints.GetNearestCheckpoint(errorHeight)
		if found {
			minHeight = checkpointHeight
			rm.logger.WithField("checkpoint_height", minHeight).Debug("Using checkpoint as lower bound")
		}
	}

	// Walk backwards looking for a valid block
	for currentHeight > minHeight {
		// Get block at this height
		block, err := rm.blockchain.GetBlockByHeight(currentHeight)
		if err != nil {
			// Block doesn't exist at this height, continue backwards
			currentHeight--
			continue
		}

		blockHash := block.Hash()

		// Check if this is a checkpoint height
		if rm.blockchain.checkpoints != nil && rm.blockchain.checkpoints.IsCheckpointHeight(currentHeight) {
			// Validate against checkpoint
			if err := rm.blockchain.checkpoints.ValidateCheckpoint(currentHeight, blockHash); err == nil {
				// This is a valid checkpoint, fork must be after this
				rm.logger.WithFields(logrus.Fields{
					"fork_point": currentHeight,
					"hash":       blockHash.String(),
				}).Debug("Found fork point at checkpoint")
				return currentHeight, blockHash, nil
			}
		}

		// Check if we can validate the parent chain from here
		if rm.canValidateChainFrom(currentHeight) {
			rm.logger.WithFields(logrus.Fields{
				"fork_point": currentHeight,
				"hash":       blockHash.String(),
			}).Debug("Found valid chain segment")
			return currentHeight, blockHash, nil
		}

		currentHeight--
	}

	// If we get here, use the minimum height as fork point
	if minHeight > 0 {
		block, err := rm.blockchain.GetBlockByHeight(minHeight)
		if err != nil {
			return 0, types.Hash{}, fmt.Errorf("failed to get checkpoint block at height %d: %w", minHeight, err)
		}
		return minHeight, block.Hash(), nil
	}

	// Last resort: go back to genesis
	return 0, types.Hash{}, nil
}

// canValidateChainFrom checks if we have a valid chain starting from a height
func (rm *RecoveryManager) canValidateChainFrom(height uint32) bool {
	// Try to validate a small segment of the chain
	segmentSize := uint32(10)
	if height < segmentSize {
		segmentSize = height
	}

	fromHeight := height - segmentSize
	err := rm.blockchain.ValidateChainSegment(fromHeight, height)
	return err == nil
}

// findCheckpointBeforeMismatch finds the nearest valid checkpoint before a failed checkpoint
func (rm *RecoveryManager) findCheckpointBeforeMismatch(failedCheckpoint uint32) (uint32, types.Hash, error) {
	rm.logger.WithField("failed_checkpoint", failedCheckpoint).Debug("Finding safe checkpoint before mismatch...")

	// Get all checkpoints
	checkpoints := rm.blockchain.checkpoints.GetCheckpoints()

	// Find all checkpoints before the failed one
	var validCheckpoints []uint32
	for height := range checkpoints {
		if height < failedCheckpoint {
			validCheckpoints = append(validCheckpoints, height)
		}
	}

	if len(validCheckpoints) == 0 {
		// No checkpoint before this, go back to genesis
		rm.logger.Debug("No checkpoint before failed one, using genesis")
		return 0, types.Hash{}, nil
	}

	// Find the highest checkpoint before the failed one
	var highestValid uint32
	for _, height := range validCheckpoints {
		if height > highestValid {
			highestValid = height
		}
	}

	// Verify this checkpoint is actually valid in our chain
	block, err := rm.blockchain.GetBlockByHeight(highestValid)
	if err != nil {
		// Can't find this checkpoint block, try the one before
		rm.logger.WithError(err).WithField("height", highestValid).Warn("Checkpoint block not found, trying earlier checkpoint")

		// Remove this height and try again
		var earlierCheckpoints []uint32
		for _, h := range validCheckpoints {
			if h < highestValid {
				earlierCheckpoints = append(earlierCheckpoints, h)
			}
		}

		if len(earlierCheckpoints) == 0 {
			return 0, types.Hash{}, nil // Back to genesis
		}

		// Try the next highest
		highestValid = 0
		for _, h := range earlierCheckpoints {
			if h > highestValid {
				highestValid = h
			}
		}

		block, err = rm.blockchain.GetBlockByHeight(highestValid)
		if err != nil {
			// Still can't find it, go to genesis
			rm.logger.WithError(err).Warn("Cannot find any valid checkpoint, using genesis")
			return 0, types.Hash{}, nil
		}
	}

	blockHash := block.Hash()
	expectedHash := checkpoints[highestValid]

	// Verify this checkpoint matches
	if blockHash != expectedHash {
		rm.logger.WithFields(logrus.Fields{
			"height":   highestValid,
			"expected": expectedHash.String(),
			"actual":   blockHash.String(),
		}).Warn("Safe checkpoint also mismatches, going to genesis")
		return 0, types.Hash{}, nil
	}

	rm.logger.WithFields(logrus.Fields{
		"height": highestValid,
		"hash":   blockHash.String(),
	}).Debug("Found valid checkpoint for recovery")

	return highestValid, blockHash, nil
}

// findActualCorruptionPoint searches backwards from current height to find where chain is broken
func (rm *RecoveryManager) findActualCorruptionPoint(fromHeight uint32) uint32 {
	rm.logger.WithField("from_height", fromHeight).Debug("Searching for actual corruption point...")

	// Search backwards to find missing blocks or chain discontinuities
	// Must be large enough to cover typical corruption scenarios (100 blocks was too small)
	chunkSize := uint32(1000)
	currentHeight := fromHeight

	// Don't search too far back (max 10,000 blocks)
	maxSearchDepth := uint32(10000)
	minHeight := fromHeight
	if minHeight > maxSearchDepth {
		minHeight = fromHeight - maxSearchDepth
	} else {
		minHeight = 0
	}

	blocksChecked := 0
	missingBlocks := []uint32{}

	for currentHeight > minHeight {
		// Check if block exists at this height
		block, err := rm.blockchain.GetBlockByHeight(currentHeight)
		if err != nil {
			// Block missing at this height
			missingBlocks = append(missingBlocks, currentHeight)
			rm.logger.WithField("height", currentHeight).Debug("Block missing")
		} else {
			// Block exists, check if parent exists and chain is continuous (unless genesis)
			if currentHeight > 0 {
				parentHash := block.Header.PrevBlockHash

				// Check 1: Parent block exists
				_, parentErr := rm.blockchain.GetBlock(parentHash)
				if parentErr != nil {
					// Parent missing - this is a corruption point
					rm.logger.WithFields(logrus.Fields{
						"height":      currentHeight,
						"parent_hash": parentHash.String(),
					}).Debug("Found corruption: parent block missing")
					return currentHeight - 1 // Return parent height as corruption point
				}

				// Check 2: Parent hash matches block at expected height (chain continuity)
				// This catches cases where PrevBlockHash points to a valid but WRONG block
				expectedParent, expectedErr := rm.blockchain.GetBlockByHeight(currentHeight - 1)
				if expectedErr != nil {
					// Cannot get block at expected parent height - chain is broken
					rm.logger.WithFields(logrus.Fields{
						"height": currentHeight,
						"error":  expectedErr,
					}).Debug("Found corruption: expected parent block missing at height")
					return currentHeight - 1 // Return parent height as corruption point
				}
				expectedHash := expectedParent.Hash()
				if parentHash != expectedHash {
					// Parent hash mismatch - chain is discontinuous
					rm.logger.WithFields(logrus.Fields{
						"height":          currentHeight,
						"expected_parent": expectedHash.String(),
						"actual_parent":   parentHash.String(),
					}).Debug("Found corruption: parent hash mismatch (chain discontinuity)")
					return currentHeight - 1 // Return parent height as corruption point
				}
			}

			// If we found a gap (missing blocks followed by existing blocks)
			if len(missingBlocks) > 0 {
				// The highest missing block is our corruption point
				highestMissing := missingBlocks[0]
				rm.logger.WithFields(logrus.Fields{
					"corruption_height": highestMissing,
					"gap_size":         len(missingBlocks),
				}).Debug("Found gap in chain")
				return highestMissing
			}
		}

		blocksChecked++
		if blocksChecked%1000 == 0 {
			rm.logger.WithField("checked", blocksChecked).Debug("Corruption search progress")
		}

		// Move to previous block
		currentHeight--

		// Stop if we've checked enough blocks without finding corruption
		if blocksChecked > int(chunkSize) && len(missingBlocks) == 0 {
			// No corruption found in recent blocks
			break
		}
	}

	// Check if we found any missing blocks
	if len(missingBlocks) > 0 {
		// Return the lowest missing block as corruption point
		lowestMissing := missingBlocks[len(missingBlocks)-1]
		rm.logger.WithField("corruption_height", lowestMissing).Debug("Found corruption at height")
		return lowestMissing
	}

	rm.logger.Debug("No corruption found in search range")
	return 0 // No corruption found
}

// RecoverFromFork attempts to recover from a detected fork
func (rm *RecoveryManager) RecoverFromFork(detectedHeight uint32) error {
	// Acquire lock for thread-safe state updates (prevents race condition)
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.logger.WithField("height", detectedHeight).Warn("Starting automatic fork recovery...")

	// Update state
	rm.state = StateInconsistencyDetected
	rm.metrics.ForkDetections++
	rm.metrics.LastForkHeight = detectedHeight

	// Check if we've exceeded max attempts (atomic check-and-increment)
	if rm.attempts >= rm.maxAttempts {
		rm.state = StateRecoveryFailed
		rm.metrics.RecoveryFailures++
		return fmt.Errorf("exceeded maximum recovery attempts (%d)", rm.maxAttempts)
	}

	rm.attempts++
	rm.metrics.RecoveryAttempts++

	// Get current chain height
	currentHeight, _ := rm.blockchain.GetBestHeight()

	// CRITICAL: Check if this is a checkpoint mismatch
	// Checkpoint mismatches mean we have wrong blocks, not missing blocks
	if rm.blockchain.checkpoints != nil && rm.blockchain.checkpoints.IsCheckpointHeight(detectedHeight) {
		rm.logger.WithField("checkpoint_height", detectedHeight).Debug("Detected checkpoint mismatch, finding safe rollback point...")

		// Find the nearest valid checkpoint BEFORE the failed one
		forkHeight, forkHash, err := rm.findCheckpointBeforeMismatch(detectedHeight)
		if err != nil {
			rm.state = StateRecoveryFailed
			rm.metrics.RecoveryFailures++
			return fmt.Errorf("failed to find safe checkpoint: %w", err)
		}

		rm.logger.WithFields(logrus.Fields{
			"rollback_to": forkHeight,
			"safe_checkpoint": forkHash.String(),
		}).Debug("Found safe checkpoint for recovery")

		// Rollback to the safe checkpoint
		if err := rm.RollbackToHeight(forkHeight); err != nil {
			rm.state = StateRecoveryFailed
			rm.metrics.RecoveryFailures++
			return fmt.Errorf("rollback to checkpoint %d failed: %w", forkHeight, err)
		}

		// Clean up wrong fork blocks
		rm.state = StateRecovering
		if err := rm.CleanOrphanedData(forkHeight, currentHeight); err != nil {
			rm.logger.WithError(err).Warn("Failed to clean orphaned data")
		}

		// Reset blockchain state
		if err := rm.ResetChainState(forkHeight, forkHash); err != nil {
			rm.state = StateRecoveryFailed
			rm.metrics.RecoveryFailures++
			return fmt.Errorf("failed to reset chain state: %w", err)
		}

		// Success
		rm.state = StateNormal
		rm.attempts = 0
		rm.staleForkRollbacks = 0
		rm.metrics.RecoverySuccess++
		rm.metrics.LastRecoveryTime = time.Now()

		rm.logger.WithFields(logrus.Fields{
			"rollback_to":    forkHeight,
			"removed_blocks": currentHeight - forkHeight,
		}).Info("✓ Checkpoint mismatch recovery successful")

		return nil
	}

	// Check for internal corruption vs stale fork.
	// This covers all non-checkpoint cases:
	// - Tip sequencing: detectedHeight near currentHeight
	// - Sync gap: detectedHeight far ahead (blocks between don't exist in storage,
	//   so FindForkPoint would return currentHeight and RollbackToHeight would fail)
	// In both cases, findActualCorruptionPoint determines the right recovery path.
	{
		rm.logger.WithFields(logrus.Fields{
			"detected_height": detectedHeight,
			"current_height":  currentHeight,
		}).Debug("Finding actual corruption point...")

		// Find where the chain is actually broken
		actualCorruptionHeight := rm.findActualCorruptionPoint(currentHeight)
		if actualCorruptionHeight == 0 {
			// No internal corruption found. This likely means we're on a stale
			// fork that is internally consistent but diverges from the network's
			// main chain. All peers send blocks whose parents we don't have
			// because our tip block has a different hash than the network's.
			// Roll back from the tip to allow the getblocks locator to find
			// the correct common ancestor on the next sync attempt.
			// Progressive depth: each consecutive stale fork rollback goes
			// deeper to handle forks that diverged more than 10 blocks back.
			rm.staleForkRollbacks++
			staleForkRollbackDepth := uint32(10) * rm.staleForkRollbacks
			if staleForkRollbackDepth > 500 {
				staleForkRollbackDepth = 500
			}
			if currentHeight > staleForkRollbackDepth {
				rollbackTo := currentHeight - staleForkRollbackDepth
				rm.logger.WithFields(logrus.Fields{
					"current_height":   currentHeight,
					"rollback_to":      rollbackTo,
					"depth":            staleForkRollbackDepth,
					"consecutive_fork": rm.staleForkRollbacks,
				}).Warn("No corruption found - performing stale fork rollback from tip")

				rm.state = StateRollingBack
				if err := rm.RollbackToHeight(rollbackTo); err != nil {
					rm.state = StateRecoveryFailed
					rm.metrics.RecoveryFailures++
					return fmt.Errorf("stale fork rollback failed: %w", err)
				}

				// Note: CleanOrphanedData is not needed here because
				// RollbackToHeight → disconnectBlock already deletes
				// all block data, indexes, and transactions for each
				// disconnected block.

				block, err := rm.blockchain.GetBlockByHeight(rollbackTo)
				if err != nil {
					rm.state = StateRecoveryFailed
					rm.metrics.RecoveryFailures++
					return fmt.Errorf("failed to get block at rollback height %d: %w", rollbackTo, err)
				}

				if err := rm.ResetChainState(rollbackTo, block.Hash()); err != nil {
					rm.state = StateRecoveryFailed
					rm.metrics.RecoveryFailures++
					return fmt.Errorf("failed to reset chain state: %w", err)
				}

				rm.state = StateNormal
				rm.attempts = 0
				rm.metrics.RecoverySuccess++
				rm.metrics.LastRecoveryTime = time.Now()

				rm.logger.WithFields(logrus.Fields{
					"rollback_to":      rollbackTo,
					"removed_blocks":   staleForkRollbackDepth,
					"consecutive_fork": rm.staleForkRollbacks,
				}).Info("✓ Stale fork rollback successful")

				return nil
			}

			// Chain too short for rollback - don't reset attempts so
			// maxAttempts safety net remains functional
			rm.state = StateNormal
			return fmt.Errorf("chain too short for stale fork rollback (height %d)", currentHeight)
		}

		// Use the actual corruption point for recovery
		detectedHeight = actualCorruptionHeight
		rm.logger.WithField("actual_corruption_height", actualCorruptionHeight).
			Info("Found actual corruption point")
	}

	// 1. Find the fork point
	forkHeight, forkHash, err := rm.FindForkPoint(detectedHeight)
	if err != nil {
		rm.state = StateRecoveryFailed
		rm.metrics.RecoveryFailures++
		return fmt.Errorf("failed to find fork point: %w", err)
	}

	rm.logger.WithFields(logrus.Fields{
		"fork_height": forkHeight,
		"fork_hash":   forkHash.String(),
	}).Info("Found fork point, initiating rollback...")

	// 2. Update state to rolling back
	rm.state = StateRollingBack

	// 3. Rollback to the fork point
	if err := rm.RollbackToHeight(forkHeight); err != nil {
		rm.state = StateRecoveryFailed
		rm.metrics.RecoveryFailures++
		return fmt.Errorf("rollback to height %d failed: %w", forkHeight, err)
	}

	// 4. Clean up orphaned data
	rm.state = StateRecovering
	if err := rm.CleanOrphanedData(forkHeight, detectedHeight); err != nil {
		// Non-fatal, log but continue
		rm.logger.WithError(err).Warn("Failed to clean orphaned data")
	}

	// 5. Reset blockchain state
	if err := rm.ResetChainState(forkHeight, forkHash); err != nil {
		rm.state = StateRecoveryFailed
		rm.metrics.RecoveryFailures++
		return fmt.Errorf("failed to reset chain state: %w", err)
	}

	// 6. Success
	rm.state = StateNormal
	rm.attempts = 0              // Reset attempts on success
	rm.staleForkRollbacks = 0    // Reset progressive depth counter
	rm.metrics.RecoverySuccess++
	rm.metrics.LastRecoveryTime = time.Now()

	rm.logger.WithFields(logrus.Fields{
		"rollback_to":    forkHeight,
		"removed_blocks": detectedHeight - forkHeight,
	}).Info("✓ Fork recovery successful")

	return nil
}

// RollbackToHeight rolls back the blockchain to a specific height
func (rm *RecoveryManager) RollbackToHeight(height uint32) error {
	rm.logger.WithField("target_height", height).Info("Rolling back blockchain...")

	// Get current height
	currentHeight, _ := rm.blockchain.GetBestHeight()
	if currentHeight == height {
		rm.logger.WithField("height", height).Debug("Already at target height, nothing to roll back")
		return nil
	}
	if currentHeight < height {
		return fmt.Errorf("current height %d is below rollback target %d", currentHeight, height)
	}

	// Check if rollback depth exceeds maximum
	rollbackDepth := currentHeight - height
	if rollbackDepth > rm.blockchain.config.MaxReorgDepth {
		return fmt.Errorf("rollback depth %d exceeds maximum %d", rollbackDepth, rm.blockchain.config.MaxReorgDepth)
	}

	// Get the block at target height
	targetBlock, err := rm.blockchain.GetBlockByHeight(height)
	if err != nil {
		return fmt.Errorf("failed to get block at height %d: %w", height, err)
	}

	rm.logger.WithFields(logrus.Fields{
		"from_height": currentHeight,
		"to_height":   height,
		"blocks":      rollbackDepth,
	}).Debug("Starting block-by-block rollback (ensures UTXO restoration)...")

	// Hold processingMu for the entire rollback to prevent concurrent
	// block connections from staking/RPC during chain state modification.
	rm.blockchain.processingMu.Lock()
	defer rm.blockchain.processingMu.Unlock()

	// ALWAYS use block-by-block disconnect to ensure UTXO integrity
	// Previous "fast batch rollback" skipped UTXO restoration which was incorrect
	progressInterval := uint32(100)
	if rollbackDepth > 10000 {
		progressInterval = 1000
	}

	for h := currentHeight; h > height; h-- {
		block, err := rm.blockchain.GetBlockByHeight(h)
		if err != nil {
			rm.logger.WithError(err).WithField("height", h).Warn("Failed to get block for disconnect")
			continue
		}

		if err := rm.blockchain.disconnectBlock(block); err != nil {
			rm.logger.WithError(err).WithField("height", h).Error("Failed to disconnect block")
			return fmt.Errorf("failed to disconnect block at height %d: %w", h, err)
		}

		// Progress reporting for large rollbacks
		blocksProcessed := currentHeight - h
		if blocksProcessed%progressInterval == 0 && blocksProcessed > 0 {
			remaining := h - height
			percent := float64(blocksProcessed) * 100.0 / float64(rollbackDepth)
			rm.logger.WithFields(logrus.Fields{
				"height":    h,
				"remaining": remaining,
				"progress":  fmt.Sprintf("%.1f%%", percent),
			}).Debug("Rollback progress")
		}
	}

	// Clean orphaned hash→height entries left by fork blocks at heights above target.
	// disconnectBlock only removes the index for the block found via height→hash,
	// leaving hash→height entries from competing fork blocks as orphans.
	orphansCleaned, err := rm.blockchain.storage.CleanOrphanedBlocks(height)
	if err != nil {
		rm.logger.WithError(err).Warn("Failed to clean orphaned blocks after rollback")
	} else if orphansCleaned > 0 {
		rm.logger.WithField("orphans_cleaned", orphansCleaned).Info("Cleaned orphaned fork block entries after rollback")
	}

	// Clean in-memory knownBlocks cache of entries above target height
	rm.blockchain.cleanKnownBlocksAboveHeight(height)

	// Update blockchain state
	rm.blockchain.mu.Lock()
	rm.blockchain.bestHeight.Store(height)
	rm.blockchain.bestHash = targetBlock.Hash()
	rm.blockchain.bestBlock = targetBlock
	rm.blockchain.mu.Unlock()

	// Update storage chain tip
	if err := rm.blockchain.storage.SetChainState(height, targetBlock.Hash()); err != nil {
		return fmt.Errorf("failed to update chain state: %w", err)
	}

	rm.logger.WithFields(logrus.Fields{
		"from_height":     currentHeight,
		"to_height":       height,
		"rolled_back":     currentHeight - height,
		"orphans_cleaned": orphansCleaned,
	}).Info("Blockchain rolled back successfully")

	return nil
}

// RecoverFromCorruptBlock handles recovery when a block's header exists but transactions are missing.
// The block was at chain tip, so processBatchUnified already applied all UTXO changes (marking
// inputs as spent, creating output UTXOs). We must roll back these UTXO changes in addition to
// removing the block data, otherwise re-syncing the block will fail with double-spend errors.
//
// Uses DeleteCorruptBlock which reads tx hashes from the compact block header and performs
// height-based UTXO scanning to find and revert all affected entries without needing
// full transaction bodies.
func (rm *RecoveryManager) RecoverFromCorruptBlock(corruptHeight uint32) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.logger.WithField("corrupt_height", corruptHeight).Warn("Recovering from corrupt block (missing transactions)...")

	// Update metrics
	rm.state = StateInconsistencyDetected
	rm.metrics.ForkDetections++
	rm.metrics.LastForkHeight = corruptHeight

	// Get the corrupt block's hash
	corruptHash, err := rm.blockchain.storage.GetBlockHashByHeight(corruptHeight)
	if err != nil {
		rm.state = StateRecoveryFailed
		rm.metrics.RecoveryFailures++
		return fmt.Errorf("failed to get corrupt block hash at height %d: %w", corruptHeight, err)
	}

	// Guard against genesis block - cannot roll back below height 0
	if corruptHeight == 0 {
		rm.state = StateRecoveryFailed
		rm.metrics.RecoveryFailures++
		return fmt.Errorf("cannot recover corrupt genesis block at height 0")
	}

	// Get parent hash from compact block (doesn't require loading transactions)
	parentHash, err := rm.blockchain.storage.GetBlockParentHash(corruptHash)
	if err != nil {
		rm.state = StateRecoveryFailed
		rm.metrics.RecoveryFailures++
		return fmt.Errorf("failed to get parent hash for corrupt block: %w", err)
	}

	parentHeight := corruptHeight - 1

	rm.logger.WithFields(logrus.Fields{
		"corrupt_hash":  corruptHash.String(),
		"parent_hash":   parentHash.String(),
		"parent_height": parentHeight,
	}).Debug("Found parent block for corrupt block recovery")

	// Update state to rolling back
	rm.state = StateRollingBack

	// Hold processingMu for the entire chain state modification to prevent
	// concurrent block connections from staking/RPC during recovery.
	rm.blockchain.processingMu.Lock()
	defer rm.blockchain.processingMu.Unlock()

	// Update chain state to parent block FIRST (so if anything fails below,
	// the chain tip is already correct and re-sync can be attempted)
	if err := rm.blockchain.storage.SetChainState(parentHeight, parentHash); err != nil {
		rm.state = StateRecoveryFailed
		rm.metrics.RecoveryFailures++
		return fmt.Errorf("failed to update chain state: %w", err)
	}

	// Delete the corrupt block with full UTXO rollback
	// This handles: unspending spent UTXOs, deleting created UTXOs, cleaning up
	// block data, indexes, address history, and any partial transaction entries
	rm.state = StateRecovering
	unspent, deleted, err := rm.blockchain.storage.DeleteCorruptBlock(corruptHash, corruptHeight)
	if err != nil {
		rm.logger.WithError(err).Error("Failed to delete corrupt block with UTXO rollback")
		rm.state = StateRecoveryFailed
		rm.metrics.RecoveryFailures++
		return fmt.Errorf("failed to delete corrupt block: %w", err)
	}

	rm.logger.WithFields(logrus.Fields{
		"unspent_utxos": unspent,
		"deleted_utxos": deleted,
	}).Info("UTXO rollback completed for corrupt block")

	// Update in-memory blockchain state
	// Note: We load parent block to update bestBlock, but if parent is also corrupt
	// the next sync attempt will trigger another recovery
	parentBlock, err := rm.blockchain.storage.GetBlock(parentHash)
	if err != nil {
		rm.logger.WithError(err).Warn("Failed to load parent block for in-memory state update")
		// Set minimal state - sync will reload properly
		rm.blockchain.mu.Lock()
		rm.blockchain.bestHeight.Store(parentHeight)
		rm.blockchain.bestHash = parentHash
		rm.blockchain.bestBlock = nil
		rm.blockchain.mu.Unlock()
	} else {
		rm.blockchain.mu.Lock()
		rm.blockchain.bestHeight.Store(parentHeight)
		rm.blockchain.bestHash = parentHash
		rm.blockchain.bestBlock = parentBlock
		rm.blockchain.mu.Unlock()
	}

	// Clean up block index
	rm.blockchain.indexMu.Lock()
	delete(rm.blockchain.blockIndex, corruptHash)
	rm.blockchain.indexMu.Unlock()

	// Success
	rm.state = StateNormal
	rm.attempts = 0
	rm.staleForkRollbacks = 0
	rm.metrics.RecoverySuccess++
	rm.metrics.LastRecoveryTime = time.Now()

	rm.logger.WithFields(logrus.Fields{
		"removed_block": corruptHash.String(),
		"new_tip":       parentHash.String(),
		"new_height":    parentHeight,
	}).Info("Corrupt block recovery successful")

	return nil
}

// CleanOrphanedData removes blocks and data above the rollback height
func (rm *RecoveryManager) CleanOrphanedData(goodHeight, badHeight uint32) error {
	rm.logger.WithFields(logrus.Fields{
		"good_height": goodHeight,
		"bad_height":  badHeight,
	}).Debug("Cleaning orphaned data...")

	cleanedBlocks := 0

	// Delete blocks between goodHeight+1 and badHeight
	for height := goodHeight + 1; height <= badHeight; height++ {
		// Get block at this height (if it exists)
		block, err := rm.blockchain.GetBlockByHeight(height)
		if err != nil {
			continue // Block doesn't exist, skip
		}

		// Delete the block from storage
		if err := rm.blockchain.storage.DeleteBlock(block.Hash()); err != nil {
			rm.logger.WithError(err).WithField("height", height).Warn("Failed to delete orphaned block")
			continue
		}

		cleanedBlocks++

		// Log progress every 1000 blocks
		if cleanedBlocks%1000 == 0 {
			rm.logger.WithField("cleaned", cleanedBlocks).Debug("Cleanup progress")
		}
	}

	// Force a storage sync to ensure deletions are persisted
	if err := rm.blockchain.storage.Sync(); err != nil {
		rm.logger.WithError(err).Warn("Failed to sync storage after cleanup")
	}

	// Compact the database to reclaim space
	if err := rm.blockchain.storage.Compact(); err != nil {
		rm.logger.WithError(err).Warn("Failed to compact storage after cleanup")
	}

	rm.logger.WithField("cleaned_blocks", cleanedBlocks).Info("✓ Orphaned data cleanup complete")

	return nil
}

// ResetChainState resets the blockchain state after rollback
func (rm *RecoveryManager) ResetChainState(height uint32, hash types.Hash) error {
	rm.logger.Debug("Resetting chain state...")

	// Clear the in-memory block index for blocks above the rollback height
	rm.blockchain.indexMu.Lock()
	for blockHash, node := range rm.blockchain.blockIndex {
		if node.Height > height {
			delete(rm.blockchain.blockIndex, blockHash)
		}
	}
	rm.blockchain.indexMu.Unlock()

	// Reset orphan blocks
	rm.blockchain.orphansMu.Lock()
	rm.blockchain.orphans = make(map[types.Hash]*types.Block)
	rm.blockchain.orphansMu.Unlock()

	// Clear any cached validation state (if UTXO cache exists)
	// Note: UTXO cache implementation would go here if available

	rm.logger.Info("✓ Chain state reset complete")

	return nil
}

// ShouldRecover determines if automatic recovery should be attempted
func (rm *RecoveryManager) ShouldRecover(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Check for specific error patterns that indicate recovery is needed
	recoveryPatterns := []string{
		"parent block",
		"batch sequencing error",
		"not found in index",
		"checkpoint validation failed",
		"checkpoint mismatch",
		"chain validation failed",
		"transaction not found",          // Corrupt block: header exists but transactions missing
		"failed to mark UTXO as spent",   // Index inconsistency or UTXO corruption
		"already spent",                  // UTXO double-spend from index mismatch
	}

	for _, pattern := range recoveryPatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

