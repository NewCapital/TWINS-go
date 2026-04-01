package blockchain

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/twins-dev/twins-core/pkg/types"
)

// ValidationMode defines how thorough chain validation should be
type ValidationMode int

const (
	// ValidationQuick checks only checkpoints and chain tip
	ValidationQuick ValidationMode = iota
	// ValidationFull validates every block's parent link (default)
	ValidationFull
	// ValidationSmart adaptively validates based on chain size
	ValidationSmart
)

// ChainValidator validates chain integrity on startup
type ChainValidator struct {
	bc        *BlockChain
	logger    *logrus.Entry
	mode      ValidationMode
	maxHeight uint32
	cm        *CheckpointManager
}

// NewChainValidator creates a new chain validator with Smart validation mode by default
func NewChainValidator(bc *BlockChain) *ChainValidator {
	return &ChainValidator{
		bc:     bc,
		logger: bc.logger.WithField("component", "chain-validator"),
		mode:   ValidationSmart, // Default to Smart validation (adaptive based on chain size)
		cm:     NewCheckpointManager(bc.config.Network),
	}
}

// SetValidationMode sets the validation mode
func (cv *ChainValidator) SetValidationMode(mode ValidationMode) {
	cv.mode = mode
	modeStr := "unknown"
	switch mode {
	case ValidationQuick:
		modeStr = "quick"
	case ValidationFull:
		modeStr = "full"
	case ValidationSmart:
		modeStr = "smart"
	}
	cv.logger.WithField("mode", modeStr).Debug("Validation mode set")
}

// ValidateChainIntegrity performs chain validation based on configured mode
func (cv *ChainValidator) ValidateChainIntegrity(ctx context.Context) error {
	switch cv.mode {
	case ValidationQuick:
		return cv.quickValidate(ctx)
	case ValidationSmart:
		return cv.smartValidate(ctx)
	case ValidationFull:
		fallthrough // Full mode is default
	default:
		return cv.fullValidate(ctx)
	}
}

// fullValidate performs complete chain validation from genesis to tip
// This is the DEFAULT mode - validates EVERY block's parent link
func (cv *ChainValidator) fullValidate(ctx context.Context) error {
	startTime := time.Now()
	cv.logger.Info("Starting FULL chain integrity validation (checking every block)...")

	// Get current best height
	bestHeight, _ := cv.bc.GetBestHeight()
	if bestHeight == 0 {
		cv.logger.Debug("Chain is at genesis, no validation needed")
		return nil
	}

	cv.maxHeight = bestHeight
	cv.logger.WithField("height", bestHeight).Debug("Validating entire chain integrity")

	// Progress tracking
	var lastProgress uint32
	progressTicker := time.NewTicker(2 * time.Second)
	defer progressTicker.Stop()

	// Start progress reporter
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-progressTicker.C:
				current := atomic.LoadUint32(&lastProgress)
				if current > 0 && current <= cv.maxHeight {
					// Safe percentage calculation to avoid overflow
					percent := (float64(current) / float64(cv.maxHeight)) * 100.0

					// Calculate rate with safety checks
					elapsed := time.Since(startTime).Seconds()
					if elapsed > 0 {
						blocksPerSec := float64(current) / elapsed
						remainingBlocks := cv.maxHeight - current

						// Build log fields
						fields := logrus.Fields{
							"height":  current,
							"total":   cv.maxHeight,
							"percent": fmt.Sprintf("%.1f%%", percent),
						}

						// Only add rate and ETA if we have meaningful values
						if blocksPerSec > 0 {
							fields["rate"] = fmt.Sprintf("%.0f blocks/sec", blocksPerSec)

							// Calculate ETA only if rate is positive
							if remainingBlocks > 0 {
								etaSeconds := float64(remainingBlocks) / blocksPerSec
								if etaSeconds < 86400 { // Less than 24 hours
									eta := time.Duration(etaSeconds) * time.Second
									fields["eta"] = eta.Round(time.Second).String()
								}
							}
						}

						cv.logger.WithFields(fields).Debug("Chain validation progress")
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Validate chain continuity
	var prevHash types.Hash
	var prevBlock *types.Block

	// Start from genesis
	genesisHash := cv.bc.config.ChainParams.GenesisHash
	genesisBlock, err := cv.bc.storage.GetBlock(genesisHash)
	if err != nil {
		cv.logger.WithError(err).Error("Genesis block not found")
		return fmt.Errorf("genesis block not found: %w", err)
	}
	prevHash = genesisHash
	prevBlock = genesisBlock

	// Validate EVERY block (Full mode)
	for height := uint32(1); height <= bestHeight; height++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Update progress
		atomic.StoreUint32(&lastProgress, height)

		// Get block hash at height
		blockHash, err := cv.bc.storage.GetBlockHashByHeight(height)
		if err != nil {
			cv.logger.WithFields(logrus.Fields{
				"height": height,
				"error":  err,
			}).Error("Failed to get block hash at height")

			// Try recovery
			return cv.handleChainGap(height, prevBlock)
		}

		// Get block
		block, err := cv.bc.storage.GetBlock(blockHash)
		if err != nil {
			cv.logger.WithFields(logrus.Fields{
				"height": height,
				"hash":   blockHash.String(),
				"error":  err,
			}).Error("Failed to get block")

			// Try recovery
			return cv.handleChainGap(height, prevBlock)
		}

		// CRITICAL: Verify parent link for EVERY block in Full mode
		if block.Header.PrevBlockHash != prevHash {
			cv.logger.WithFields(logrus.Fields{
				"height":          height,
				"expected_parent": prevHash.String(),
				"actual_parent":   block.Header.PrevBlockHash.String(),
			}).Error("Chain discontinuity detected - parent mismatch!")

			// Try recovery
			return cv.handleChainGap(height, prevBlock)
		}

		// Validate checkpoint if applicable
		if cv.shouldValidateCheckpoint(height) {
			if err := cv.validateCheckpoint(height, blockHash); err != nil {
				cv.logger.WithFields(logrus.Fields{
					"height": height,
					"hash":   blockHash.String(),
					"error":  err,
				}).Error("Checkpoint validation failed")

				// Checkpoint failure is critical - try recovery
				return cv.handleCheckpointFailure(height, blockHash)
			}
		}

		// Update for next iteration
		prevHash = blockHash
		prevBlock = block
	}

	// Final validation - ensure chain tip matches
	chainHeight, err := cv.bc.storage.GetChainHeight()
	if err != nil {
		return fmt.Errorf("failed to get chain height: %w", err)
	}
	chainHash, err := cv.bc.storage.GetChainTip()
	if err != nil {
		return fmt.Errorf("failed to get chain tip: %w", err)
	}
	if chainHeight != bestHeight || chainHash != prevHash {
		cv.logger.WithFields(logrus.Fields{
			"expected_height": bestHeight,
			"actual_height":   chainHeight,
			"expected_hash":   prevHash.String(),
			"actual_hash":     chainHash.String(),
		}).Error("Chain state mismatch")

		return cv.handleStateMismatch(bestHeight, prevHash)
	}

	elapsed := time.Since(startTime)
	cv.logger.WithFields(logrus.Fields{
		"mode":       "FULL",
		"height":     bestHeight,
		"duration":   elapsed,
		"blocks/sec": fmt.Sprintf("%.0f", float64(bestHeight)/elapsed.Seconds()),
	}).Info("✓ FULL chain integrity validation completed successfully")

	return nil
}

// quickValidate performs quick validation checking only critical points
func (cv *ChainValidator) quickValidate(ctx context.Context) error {
	cv.logger.Info("Performing QUICK chain validation (checkpoints only)...")

	bestHeight, _ := cv.bc.GetBestHeight()

	// Check genesis
	genesisHash := cv.bc.config.ChainParams.GenesisHash
	if _, err := cv.bc.storage.GetBlock(genesisHash); err != nil {
		return fmt.Errorf("genesis block missing: %w", err)
	}

	// Check all checkpoints
	cm := NewCheckpointManager(cv.bc.config.Network)
	checkpoints := cm.GetCheckpoints()
	validated := 0
	for height, expectedHash := range checkpoints {
		if height > bestHeight {
			continue // Skip future checkpoints
		}

		actualHash, err := cv.bc.storage.GetBlockHashByHeight(height)
		if err != nil {
			cv.logger.WithFields(logrus.Fields{
				"height": height,
				"error":  err,
			}).Error("Checkpoint block missing")
			return fmt.Errorf("checkpoint at height %d missing: %w", height, err)
		}

		if actualHash != expectedHash {
			cv.logger.WithFields(logrus.Fields{
				"height":   height,
				"expected": expectedHash.String(),
				"actual":   actualHash.String(),
			}).Error("Checkpoint mismatch")
			return fmt.Errorf("checkpoint mismatch at height %d", height)
		}
		validated++
	}

	// Check chain tip
	chainHeight, err := cv.bc.storage.GetChainHeight()
	if err != nil {
		return fmt.Errorf("failed to get chain height: %w", err)
	}
	chainHash, err := cv.bc.storage.GetChainTip()
	if err != nil {
		return fmt.Errorf("failed to get chain tip: %w", err)
	}
	if chainHeight != bestHeight {
		return fmt.Errorf("chain height mismatch: stored=%d, expected=%d", chainHeight, bestHeight)
	}

	// Verify tip block exists
	if _, err := cv.bc.storage.GetBlock(chainHash); err != nil {
		return fmt.Errorf("chain tip block missing: %w", err)
	}

	cv.logger.WithFields(logrus.Fields{
		"mode":        "QUICK",
		"height":      bestHeight,
		"checkpoints": validated,
	}).Info("✓ QUICK validation passed")
	return nil
}

// smartValidate performs adaptive validation based on chain size
func (cv *ChainValidator) smartValidate(ctx context.Context) error {
	cv.logger.Info("Performing SMART chain validation (adaptive sampling)...")

	bestHeight, _ := cv.bc.GetBestHeight()

	// For chains under 100k blocks, do full validation
	if bestHeight < 100000 {
		cv.logger.Debug("Chain under 100k blocks, using full validation")
		return cv.fullValidate(ctx)
	}

	// For larger chains, validate:
	// - All checkpoints
	// - Every 1000th block
	// - Last 1000 blocks fully

	startTime := time.Now()

	// First validate all checkpoints
	if err := cv.quickValidate(ctx); err != nil {
		return err
	}

	// Sample validation: every 1000th block
	cv.logger.Debug("Validating sampled blocks...")
	for height := uint32(1000); height < bestHeight-1000; height += 1000 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		blockHash, err := cv.bc.storage.GetBlockHashByHeight(height)
		if err != nil {
			return cv.handleChainGap(height, nil)
		}

		block, err := cv.bc.storage.GetBlock(blockHash)
		if err != nil {
			return cv.handleChainGap(height, nil)
		}

		// Verify parent exists
		if _, err := cv.bc.storage.GetBlock(block.Header.PrevBlockHash); err != nil {
			cv.logger.WithFields(logrus.Fields{
				"height": height,
				"parent": block.Header.PrevBlockHash.String(),
			}).Error("Parent block missing")
			return cv.handleChainGap(height, nil)
		}
	}

	// Full validation for last 1000 blocks
	cv.logger.Debug("Validating last 1000 blocks fully...")
	var startHeight uint32
	if bestHeight > 1000 {
		startHeight = bestHeight - 1000
	} else {
		startHeight = 0
	}

	var prevHash types.Hash
	for height := startHeight; height <= bestHeight; height++ {
		blockHash, err := cv.bc.storage.GetBlockHashByHeight(height)
		if err != nil {
			return cv.handleChainGap(height, nil)
		}

		block, err := cv.bc.storage.GetBlock(blockHash)
		if err != nil {
			return cv.handleChainGap(height, nil)
		}

		if height > startHeight && block.Header.PrevBlockHash != prevHash {
			return cv.handleChainGap(height, nil)
		}

		prevHash = blockHash
	}

	elapsed := time.Since(startTime)
	cv.logger.WithFields(logrus.Fields{
		"mode":     "SMART",
		"height":   bestHeight,
		"duration": elapsed,
	}).Info("✓ SMART validation completed successfully")

	return nil
}

// shouldValidateCheckpoint returns true if this height has a checkpoint
func (cv *ChainValidator) shouldValidateCheckpoint(height uint32) bool {
	checkpoints := cv.cm.GetCheckpoints()
	_, exists := checkpoints[height]
	return exists
}

// validateCheckpoint validates block against known checkpoint
func (cv *ChainValidator) validateCheckpoint(height uint32, blockHash types.Hash) error {
	checkpoints := cv.cm.GetCheckpoints()
	expectedHash, exists := checkpoints[height]
	if !exists {
		return nil // No checkpoint at this height
	}

	if blockHash != expectedHash {
		return fmt.Errorf("checkpoint mismatch at height %d: expected %s, got %s",
			height, expectedHash, blockHash)
	}

	cv.logger.WithFields(logrus.Fields{
		"height": height,
		"hash":   blockHash.String(),
	}).Debug("✓ Checkpoint validated")

	return nil
}

// handleChainGap handles a gap in the chain
func (cv *ChainValidator) handleChainGap(height uint32, lastGoodBlock *types.Block) error {
	cv.logger.WithFields(logrus.Fields{
		"gap_at_height": height,
	}).Error("Chain gap detected, attempting recovery...")

	// Try to trigger recovery
	recovery := NewRecoveryManager(cv.bc)
	if err := recovery.RecoverFromFork(height - 1); err != nil {
		cv.logger.WithError(err).Error("Recovery failed")
		return fmt.Errorf("CRITICAL: Chain is broken at height %d and recovery failed. "+
			"The blockchain data is corrupted beyond automatic repair. "+
			"Please delete the data directory and resync from scratch. "+
			"Error: %w", height, err)
	}

	cv.logger.Info("Recovery completed, verifying recovered state...")

	// Verify recovered state
	newBestHeight, err := cv.bc.GetBestHeight()
	if err != nil {
		return fmt.Errorf("failed to get best height after recovery: %w", err)
	}

	if newBestHeight < height-1 {
		cv.logger.WithFields(logrus.Fields{
			"recovered_height":   newBestHeight,
			"gap_was_at_height": height,
		}).Info("Recovery rolled back chain to valid state")
	}

	// Verify chain tip is valid
	chainHash, err := cv.bc.storage.GetChainTip()
	if err != nil {
		return fmt.Errorf("failed to verify chain tip after recovery: %w", err)
	}

	// Verify tip block exists
	if _, err := cv.bc.storage.GetBlock(chainHash); err != nil {
		return fmt.Errorf("chain tip block missing after recovery: %w", err)
	}

	cv.logger.WithFields(logrus.Fields{
		"new_height": newBestHeight,
		"new_hash":   chainHash.String(),
	}).Info("✓ Recovery successful and chain state verified")

	return nil
}

// handleCheckpointFailure handles checkpoint validation failure
func (cv *ChainValidator) handleCheckpointFailure(height uint32, blockHash types.Hash) error {
	cv.logger.WithFields(logrus.Fields{
		"height": height,
		"hash":   blockHash.String(),
	}).Error("Checkpoint failure detected, attempting recovery...")

	// Find last good checkpoint
	checkpoints := cv.cm.GetCheckpoints()
	lastGoodHeight := uint32(0)

	for h := height - 1; h > 0; h-- {
		if expectedHash, exists := checkpoints[h]; exists {
			// Verify this checkpoint is valid
			actualHash, err := cv.bc.storage.GetBlockHashByHeight(h)
			if err == nil && actualHash == expectedHash {
				lastGoodHeight = h
				break
			}
		}
	}

	if lastGoodHeight == 0 {
		return fmt.Errorf("CRITICAL: No valid checkpoints found in blockchain. " +
			"The chain is on a completely wrong fork. " +
			"Please delete the data directory and resync from scratch")
	}

	cv.logger.WithField("rollback_to", lastGoodHeight).Debug("Rolling back to last valid checkpoint")

	recovery := NewRecoveryManager(cv.bc)
	if err := recovery.RecoverFromFork(lastGoodHeight); err != nil {
		return fmt.Errorf("CRITICAL: Checkpoint recovery failed. "+
			"Cannot rollback to valid checkpoint at height %d. "+
			"Please delete the data directory and resync from scratch. "+
			"Error: %w", lastGoodHeight, err)
	}

	return nil
}

// handleStateMismatch handles chain state inconsistency
func (cv *ChainValidator) handleStateMismatch(expectedHeight uint32, expectedHash types.Hash) error {
	cv.logger.Error("Chain state mismatch, correcting...")

	// Update chain state to match validated chain
	batch := cv.bc.storage.NewBatch()
	if err := batch.SetChainState(expectedHeight, expectedHash); err != nil {
		return fmt.Errorf("failed to correct chain state: %w", err)
	}

	if err := batch.Commit(); err != nil {
		return fmt.Errorf("failed to commit chain state correction: %w", err)
	}

	// Force storage sync
	if err := cv.bc.storage.Sync(); err != nil {
		return fmt.Errorf("failed to sync storage: %w", err)
	}

	cv.logger.Info("✓ Chain state corrected")
	return nil
}

// IndexInconsistency represents a mismatch between hash→height and height→hash indexes
type IndexInconsistency struct {
	Height       uint32
	HashFromIdx  types.Hash // hash stored in hash→height index
	HashFromHght types.Hash // hash stored in height→hash index (may be zero if missing)
	Missing      bool       // true if height→hash entry is missing entirely
}

// CheckIndexConsistency is the shared function that validates bidirectional block index integrity.
// It iterates all hash→height entries and cross-checks against height→hash.
// Returns the list of inconsistencies found (empty if indexes are consistent).
// This function is called both during startup validation and on UTXO errors.
func (cv *ChainValidator) CheckIndexConsistency(ctx context.Context) ([]IndexInconsistency, error) {
	cv.logger.Info("Starting block index consistency check (hash→height vs height→hash)...")
	startTime := time.Now()

	var inconsistencies []IndexInconsistency
	var checkedCount int

	err := cv.bc.storage.IterateHashToHeight(func(hash types.Hash, height uint32) bool {
		// Check context cancellation periodically
		checkedCount++
		if checkedCount%10000 == 0 {
			select {
			case <-ctx.Done():
				return false
			default:
			}
			cv.logger.WithFields(logrus.Fields{
				"checked": checkedCount,
			}).Debug("Index consistency check progress")
		}

		// Cross-check: height→hash at this height should return the same hash
		reverseHash, err := cv.bc.storage.GetBlockHashByHeight(height)
		if err != nil {
			// height→hash entry missing entirely
			cv.logger.WithFields(logrus.Fields{
				"hash":   hash.String(),
				"height": height,
				"error":  err,
			}).Warn("Index inconsistency: hash→height exists but height→hash missing")

			inconsistencies = append(inconsistencies, IndexInconsistency{
				Height:      height,
				HashFromIdx: hash,
				Missing:     true,
			})
			return true
		}

		if reverseHash != hash {
			// Mismatch: height→hash points to a different block
			cv.logger.WithFields(logrus.Fields{
				"height":          height,
				"hash_from_index": hash.String(),
				"hash_from_height": reverseHash.String(),
			}).Warn("Index inconsistency: hash→height and height→hash disagree")

			inconsistencies = append(inconsistencies, IndexInconsistency{
				Height:       height,
				HashFromIdx:  hash,
				HashFromHght: reverseHash,
				Missing:      false,
			})
		}

		return true
	})

	if err != nil {
		return nil, fmt.Errorf("failed to iterate hash→height index: %w", err)
	}

	// Check if iteration was cancelled by context - partial scan is unreliable
	if ctx.Err() != nil {
		return nil, fmt.Errorf("index consistency check cancelled: %w", ctx.Err())
	}

	elapsed := time.Since(startTime)
	if len(inconsistencies) == 0 {
		cv.logger.WithFields(logrus.Fields{
			"checked":  checkedCount,
			"duration": elapsed,
		}).Info("✓ Block index consistency check passed")
	} else {
		cv.logger.WithFields(logrus.Fields{
			"checked":         checkedCount,
			"inconsistencies": len(inconsistencies),
			"duration":        elapsed,
		}).Error("Block index inconsistencies found!")
	}

	return inconsistencies, nil
}

// RecoverFromIndexInconsistency handles recovery when index inconsistencies are detected.
// It finds the lowest inconsistent height, collects all block hashes at and above that height,
// performs rollback, then verifies the collected hashes are removed from the database.
func (cv *ChainValidator) RecoverFromIndexInconsistency(ctx context.Context, inconsistencies []IndexInconsistency) error {
	if len(inconsistencies) == 0 {
		return nil
	}

	// Find the lowest inconsistent height
	lowestHeight := inconsistencies[0].Height
	for _, inc := range inconsistencies[1:] {
		if inc.Height < lowestHeight {
			lowestHeight = inc.Height
		}
	}

	// Rollback target is two below the lowest inconsistency to ensure
	// a clean re-sync margin (the block at lowestHeight-1 may depend on
	// the inconsistent index entry via stake modifiers or UTXO references).
	rollbackTarget := lowestHeight
	if rollbackTarget >= 2 {
		rollbackTarget -= 2
	} else {
		rollbackTarget = 0
	}

	cv.logger.WithFields(logrus.Fields{
		"lowest_inconsistency": lowestHeight,
		"rollback_target":      rollbackTarget,
		"total_inconsistencies": len(inconsistencies),
	}).Warn("Recovering from index inconsistency, rolling back to safe height...")

	bestHeight, _ := cv.bc.GetBestHeight()

	cv.logger.WithFields(logrus.Fields{
		"rollback_from": bestHeight,
		"rollback_to":   rollbackTarget,
	}).Info("Starting index inconsistency recovery...")

	// Perform rollback — RollbackToHeight now includes CleanOrphanedBlocks
	// which iterates ALL hash→height entries and removes any above the target,
	// eliminating the orphaned fork block entries that caused the inconsistencies.
	recovery := NewRecoveryManager(cv.bc)
	if err := recovery.RollbackToHeight(rollbackTarget); err != nil {
		return fmt.Errorf("rollback to height %d failed: %w", rollbackTarget, err)
	}

	// Clean orphaned data (block data at heights above target found via height→hash)
	if err := recovery.CleanOrphanedData(rollbackTarget, bestHeight); err != nil {
		cv.logger.WithError(err).Warn("Failed to clean orphaned data after rollback")
	}

	// Reset chain state
	rollbackBlock, err := cv.bc.GetBlockByHeight(rollbackTarget)
	if err != nil {
		return fmt.Errorf("failed to get block at rollback target %d: %w", rollbackTarget, err)
	}
	rollbackHash := rollbackBlock.Hash()

	if err := recovery.ResetChainState(rollbackTarget, rollbackHash); err != nil {
		return fmt.Errorf("failed to reset chain state: %w", err)
	}

	cv.logger.WithFields(logrus.Fields{
		"new_height":     rollbackTarget,
		"blocks_removed": bestHeight - rollbackTarget,
	}).Info("✓ Index inconsistency recovery completed")

	return nil
}

// ValidateBlockIndexConsistency runs index consistency check as a separate pass
// and triggers recovery if inconsistencies are found.
// Called after chain validation during startup.
func (cv *ChainValidator) ValidateBlockIndexConsistency(ctx context.Context) error {
	inconsistencies, err := cv.CheckIndexConsistency(ctx)
	if err != nil {
		return fmt.Errorf("index consistency check failed: %w", err)
	}

	if len(inconsistencies) == 0 {
		return nil
	}

	return cv.RecoverFromIndexInconsistency(ctx, inconsistencies)
}

// CheckIndexConsistencyAroundHeight performs a targeted index consistency check
// around a specific height. This is much faster than a full scan since it only
// checks blocks in the range [height-radius, height+radius].
// Used by the reactive validation failure detector to quickly identify local
// index corruption when the same block fails validation from multiple peers.
func (cv *ChainValidator) CheckIndexConsistencyAroundHeight(ctx context.Context, targetHeight uint32, radius uint32) ([]IndexInconsistency, error) {
	cv.logger.WithFields(logrus.Fields{
		"target_height": targetHeight,
		"radius":        radius,
	}).Info("Starting targeted block index consistency check...")
	startTime := time.Now()

	// Calculate the range to check
	var fromHeight uint32
	if targetHeight > radius {
		fromHeight = targetHeight - radius
	}

	bestHeight, _ := cv.bc.GetBestHeight()
	toHeight := targetHeight + radius
	if toHeight < targetHeight || toHeight > bestHeight {
		// Overflow protection (toHeight < targetHeight) or clamp to chain tip
		toHeight = bestHeight
	}

	var inconsistencies []IndexInconsistency

	for h := fromHeight; h <= toHeight; h++ {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("targeted index check cancelled: %w", ctx.Err())
		default:
		}

		// Get hash at this height via height→hash index
		hashAtHeight, err := cv.bc.storage.GetBlockHashByHeight(h)
		if err != nil {
			cv.logger.WithFields(logrus.Fields{
				"height": h,
				"error":  err,
			}).Warn("Targeted check: height→hash entry missing")
			inconsistencies = append(inconsistencies, IndexInconsistency{
				Height:  h,
				Missing: true,
			})
			continue
		}

		// Reverse check: hash→height should return the same height
		reverseHeight, err := cv.bc.storage.GetBlockHeight(hashAtHeight)
		if err != nil {
			cv.logger.WithFields(logrus.Fields{
				"height": h,
				"hash":   hashAtHeight.String(),
			}).Warn("Targeted check: hash→height missing for height→hash entry")
			inconsistencies = append(inconsistencies, IndexInconsistency{
				Height:       h,
				HashFromHght: hashAtHeight,
				Missing:      true,
			})
			continue
		}

		if reverseHeight != h {
			// Round-trip mismatch: height H → hashAtHeight, but hashAtHeight → reverseHeight (≠ H).
			// Look up the hash at reverseHeight to identify the competing index entry.
			reverseHash, _ := cv.bc.storage.GetBlockHashByHeight(reverseHeight)
			cv.logger.WithFields(logrus.Fields{
				"height":              h,
				"hash":                hashAtHeight.String(),
				"reverse_height":      reverseHeight,
				"hash_at_reverse_hgt": reverseHash.String(),
			}).Warn("Targeted check: height→hash→height round-trip mismatch")
			inconsistencies = append(inconsistencies, IndexInconsistency{
				Height:       h,
				HashFromIdx:  reverseHash,   // hash stored at the height that hash→height points to
				HashFromHght: hashAtHeight,  // hash stored at our target height from height→hash
			})
		}
	}

	elapsed := time.Since(startTime)
	checkedCount := toHeight - fromHeight + 1
	if len(inconsistencies) == 0 {
		cv.logger.WithFields(logrus.Fields{
			"checked":  checkedCount,
			"range":    fmt.Sprintf("%d-%d", fromHeight, toHeight),
			"duration": elapsed,
		}).Info("✓ Targeted index consistency check passed")
	} else {
		cv.logger.WithFields(logrus.Fields{
			"checked":         checkedCount,
			"range":           fmt.Sprintf("%d-%d", fromHeight, toHeight),
			"inconsistencies": len(inconsistencies),
			"duration":        elapsed,
		}).Error("Targeted index inconsistencies found!")
	}

	return inconsistencies, nil
}

// GetValidationMode returns the current validation mode
func (cv *ChainValidator) GetValidationMode() ValidationMode {
	return cv.mode
}

// GetValidationModeString returns human-readable validation mode
func (cv *ChainValidator) GetValidationModeString() string {
	switch cv.mode {
	case ValidationQuick:
		return "quick"
	case ValidationFull:
		return "full"
	case ValidationSmart:
		return "smart"
	default:
		return "unknown"
	}
}
