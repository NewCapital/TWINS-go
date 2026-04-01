package blockchain

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/twins-dev/twins-core/internal/storage"
	"github.com/twins-dev/twins-core/internal/storage/binary"
	"github.com/twins-dev/twins-core/pkg/types"
)

// indexBlockTransactions indexes all transactions in a block.
// precomputedOutputs contains spent output data collected by applyBlockToBatch
// (from MarkUTXOSpent results), eliminating redundant GetTransaction DB lookups.
// Pass nil to fall back to DB lookups (e.g. during reindex without applyBlockToBatch).
func (bc *BlockChain) indexBlockTransactions(block *types.Block, height uint32, batch storage.Batch, precomputedOutputs map[types.Outpoint]*types.TxOutput) error {
	blockHash := block.Hash()

	for txIndex, tx := range block.Transactions {
		txHash := tx.Hash()

		// Index by address (for wallet functionality)
		// ALWAYS index - database must write all data
		if err := bc.indexTransactionAddresses(tx, height, uint32(txIndex), batch, blockHash, precomputedOutputs); err != nil {
			// Log but don't fail - address indexing is not critical for block processing
			bc.logger.WithError(err).WithFields(map[string]interface{}{
				"txhash": txHash.String(),
				"height": height,
				"txIdx":  txIndex,
			}).Warn("Failed to index transaction addresses")
		}
	}
	return nil
}

// getSpentOutputsForTx retrieves spent outputs for a transaction.
// precomputedOutputs is checked first (populated by applyBlockToBatch from MarkUTXOSpent).
// Falls back to DB lookup via GetTransaction when precomputedOutputs is nil or missing an entry
// (e.g. during reindex).
func (bc *BlockChain) getSpentOutputsForTx(tx *types.Transaction, precomputedOutputs map[types.Outpoint]*types.TxOutput) ([]storage.SpentOutput, error) {
	if tx.IsCoinbase() {
		return nil, nil // Coinbase has no inputs
	}

	spentOutputs := make([]storage.SpentOutput, 0, len(tx.Inputs))
	for _, input := range tx.Inputs {
		// Fast path: use precomputed output from applyBlockToBatch
		if precomputedOutputs != nil {
			if output, ok := precomputedOutputs[input.PreviousOutput]; ok {
				spentOutputs = append(spentOutputs, storage.SpentOutput{
					Outpoint: input.PreviousOutput,
					Output:   output,
				})
				continue
			}
		}

		// Slow path: fall back to DB lookup (used during reindex or if precomputed data missing)
		prevTx, err := bc.storage.GetTransaction(input.PreviousOutput.Hash)
		if err != nil {
			// Transaction not found - skip this input
			// This can happen if transaction index is not built yet
			continue
		}

		// Validate output index
		if input.PreviousOutput.Index >= uint32(len(prevTx.Outputs)) {
			continue
		}

		// Get the spent output
		prevOutput := prevTx.Outputs[input.PreviousOutput.Index]

		spentOutputs = append(spentOutputs, storage.SpentOutput{
			Outpoint: input.PreviousOutput,
			Output:   prevOutput,
		})
	}

	return spentOutputs, nil
}

// indexTransactionAddresses indexes transaction by involved addresses.
// precomputedOutputs provides spent output data from applyBlockToBatch to avoid DB lookups.
// This enables wallet-related RPC calls like listreceivedbyaddress.
func (bc *BlockChain) indexTransactionAddresses(tx *types.Transaction, height uint32, txIndex uint32, batch storage.Batch, blockHash types.Hash, precomputedOutputs map[types.Outpoint]*types.TxOutput) error {
	txHash := tx.Hash()

	spentValues := make(map[string]int64, len(tx.Inputs))

	// Index inputs (spending addresses)
	if !tx.IsCoinbase() {
		spentOutputs, err := bc.getSpentOutputsForTx(tx, precomputedOutputs)
		if err != nil {
			return fmt.Errorf("failed to get spent outputs: %w", err)
		}

		for _, spentOutput := range spentOutputs {
			if spentOutput.Output != nil {
				scriptType, scriptHash := binary.AnalyzeScript(spentOutput.Output.ScriptPubKey)
				addressBinary := binary.ScriptHashToAddressBinary(scriptType, scriptHash, bc.config.IsTestNet())
				if addressBinary != nil {
					spentValues[string(binary.AddressHistoryKey(scriptHash, height, txHash, uint16(txIndex)))] = spentOutput.Output.Value
					if err := batch.IndexTransactionByAddress(addressBinary, txHash, height, txIndex, -spentOutput.Output.Value, true, blockHash); err != nil {
						return fmt.Errorf("failed to index input address: %w", err)
					}
				}
			}
		}
	}

	// Index outputs (receiving addresses)
	for _, output := range tx.Outputs {
		scriptType, scriptHash := binary.AnalyzeScript(output.ScriptPubKey)
		addressBinary := binary.ScriptHashToAddressBinary(scriptType, scriptHash, bc.config.IsTestNet())
		if addressBinary != nil {
			key := string(binary.AddressHistoryKey(scriptHash, height, txHash, uint16(txIndex)))
			value := output.Value
			if spentValues[key] != 0 {
				value -= spentValues[key]
			}
			if err := batch.IndexTransactionByAddress(addressBinary, txHash, height, txIndex, value, false, blockHash); err != nil {
				return fmt.Errorf("failed to index output address: %w", err)
			}
		}
		// else: skip non-addressable scripts (coinstake empty marker, PoS coinbase
		// marker, OP_RETURN 0x6a, Zerocoin/Sigma privacy scripts 0xc1-0xc4)
	}

	return nil
}

// calculateBlockHeight calculates block height from parent
// Reads directly from storage after parent block is committed
func (bc *BlockChain) calculateBlockHeight(block *types.Block) (uint32, error) {
	// Genesis block
	if block.Header.PrevBlockHash.IsZero() {
		return 0, nil
	}

	// Read from storage
	parentHeight, err := bc.storage.GetBlockHeight(block.Header.PrevBlockHash)
	if err != nil {
		return 0, fmt.Errorf("%w: parent %s not found", ErrParentNotFound, block.Header.PrevBlockHash.String())
	}

	return parentHeight + 1, nil
}

// processBatchUnified is the unified batch processing path.
// Handles both single blocks and multiple blocks.
// CONCURRENCY: Acquires processingMu to serialize all external callers (sync, staking, RPC).
// Internal callers that already hold the lock (e.g. reorganizeChain) must use
// processBatchUnifiedLocked directly to avoid deadlock.
func (bc *BlockChain) processBatchUnified(blocks []*types.Block) error {
	bc.processingMu.Lock()
	defer bc.processingMu.Unlock()
	return bc.processBatchUnifiedLocked(blocks)
}

// processBatchUnifiedLocked is the lock-free inner implementation.
// Caller MUST hold processingMu.
func (bc *BlockChain) processBatchUnifiedLocked(blocks []*types.Block) error {
	if len(blocks) == 0 {
		return nil
	}

	// Track batch performance
	batchStart := time.Now()
	defer func() {
		duration := time.Since(batchStart)
		blocksPerSec := float64(len(blocks)) / duration.Seconds()
		bc.logger.WithFields(map[string]interface{}{
			"blocks":         len(blocks),
			"duration_ms":    duration.Milliseconds(),
			"blocks_per_sec": int(blocksPerSec),
			"ms_per_block":   duration.Milliseconds() / int64(len(blocks)),
		}).Debug("Batch processing complete")
	}()

	// Batch size optimization for IBD
	if len(blocks) > MaxBatchSizeForOptimalPerformance {
		bc.logger.WithFields(map[string]interface{}{
			"requested": len(blocks),
			"max":       MaxBatchSizeForOptimalPerformance,
		}).Warn("Batch size exceeds maximum, processing in chunks")
		// Process in chunks - use internal method to avoid recursive atomic guard
		for i := 0; i < len(blocks); i += MaxBatchSizeForOptimalPerformance {
			end := i + MaxBatchSizeForOptimalPerformance
			if end > len(blocks) {
				end = len(blocks)
			}
			if err := bc.processBatchInternal(blocks[i:end]); err != nil {
				return fmt.Errorf("chunk %d-%d failed: %w", i, end, err)
			}
		}
		return nil
	}

	return bc.processBatchInternal(blocks)
}

// processBatchInternal is the actual batch processing logic without atomic guard
// Called by processBatchUnified which handles the atomic guard
func (bc *BlockChain) processBatchInternal(blocks []*types.Block) error {
	if len(blocks) == 0 {
		return nil
	}

	bc.logger.WithField("blocks", len(blocks)).Debug("Entering processBatchUnified")

	// Filter out blocks that already exist in the chain
	// Duplicate blocks can only be at the start of the batch due to sequential sync
	skippedCount := 0
	for _, block := range blocks {
		has, err := bc.hasBlock(block.Hash())
		if err != nil {
			return fmt.Errorf("failed to check block existence: %w", err)
		}
		if has {
			skippedCount++
		} else {
			break // Stop at first new block
		}
	}

	if skippedCount > 0 {
		bc.logger.WithFields(map[string]interface{}{
			"total":   len(blocks),
			"skipped": skippedCount,
			"new":     len(blocks) - skippedCount,
		}).Debug("Filtered existing blocks from batch")
	}

	if skippedCount == len(blocks) {
		bc.logger.Debug("All blocks in batch already exist, signaling for fork detection")
		return ErrAllBlocksExist
	}

	// Process only new blocks
	blocks = blocks[skippedCount:]

	// Sync storage at the end or on error (for durability)
	var syncNeeded bool
	defer func() {
		if syncNeeded {
			if err := bc.storage.Sync(); err != nil {
				bc.logger.WithError(err).Error("Failed to sync storage in defer")
			}
		}
	}()

	// For single block processing: perform validation checks
	// (batch processing from P2P layer already validated blocks)
	if len(blocks) == 1 {
		block := blocks[0]
		blockHash := block.Hash()

		// Check if block already exists in storage
		has, err := bc.hasBlock(blockHash)
		if err != nil {
			syncNeeded = true
			return fmt.Errorf("failed to check block existence: %w", err)
		}
		if has {
			// Check if this block is already our best block
			isBest := bc.bestHash == blockHash

			if isBest {
				bc.logger.WithField("block", blockHash.String()).Debug("Block already exists and is best block")
				return ErrBlockExists
			}

			// Block exists but might not be connected to main chain
			// This can happen if we received it as orphan before
			// Try to see if we need reorg
			bc.logger.WithField("block", blockHash.String()).Debug("Block already exists, checking if reorg needed")

			// Get the block from storage to check its status
			storedBlock, err := bc.GetBlock(blockHash)
			if err != nil {
				syncNeeded = true
				return fmt.Errorf("failed to get stored block: %w", err)
			}

			// Check if reorganization is needed
			needsReorg, err := bc.CheckReorgNeeded(storedBlock)
			if err != nil {
				bc.logger.WithError(err).Warn("Failed to check reorg needed")
				return ErrBlockExists
			}

			if needsReorg {
				bc.logger.WithField("block", blockHash.String()).Info("Triggering reorganization for existing block")
				// Call locked version — processingMu is already held by us.
				if err := bc.reorganizeChainLocked(blockHash); err != nil {
					bc.logger.WithError(err).Error("Chain reorganization failed")
					syncNeeded = true
					return fmt.Errorf("reorganization failed: %w", err)
				}
			}

			return ErrBlockExists
		}

		// Check if block already exists in orphan pool
		bc.orphansMu.RLock()
		_, isOrphan := bc.orphans[blockHash]
		bc.orphansMu.RUnlock()
		if isOrphan {
			bc.logger.WithField("block", blockHash.String()).Debug("Block already in orphan pool")
			return nil
		}

		// Check if parent exists (atomic with block connection due to held lock)
		// This prevents race where parent batch commits between our check and block processing
		// Skip parent check for genesis block (zero hash parent)
		if !block.Header.PrevBlockHash.IsZero() {
			hasParent, err := bc.hasBlock(block.Header.PrevBlockHash)
			if err != nil {
				syncNeeded = true
				return fmt.Errorf("failed to check parent existence: %w", err)
			}

			if !hasParent {
				// Parent not found - this should trigger orphan handling
				bc.logger.WithFields(map[string]interface{}{
					"block":  blockHash.String(),
					"parent": block.Header.PrevBlockHash.String(),
				}).Info("Block parent not found")
				return fmt.Errorf("%w: parent %s for block %s", ErrParentNotFound,
					block.Header.PrevBlockHash.String(), blockHash.String())
			}

			// Variant 3: Reject single blocks that would land at or below our current tip.
			// This prevents stale-fork peers from triggering unwanted chain reorganizations.
			// If we are genuinely on the wrong fork, consensus (proto 70928 majority inv)
			// will trigger proper batch-based re-sync, not single-block reorg.
			parentHeight, err := bc.storage.GetBlockHeight(block.Header.PrevBlockHash)
			if err == nil {
				blockHeight := parentHeight + 1
				if blockHeight <= bc.bestHeight.Load() {
					bc.logger.WithFields(map[string]interface{}{
						"block":        blockHash.String(),
						"block_height": blockHeight,
						"best_height":  bc.bestHeight.Load(),
					}).Debug("Rejecting block: height not advancing beyond current tip")
					return ErrHeightNotAdvancing
				}
			}
		}
	}

	// Check IBD status (for validation level)
	isIBD := bc.IsInitialBlockDownload()

	// Pre-validate batch integrity
	if err := bc.preValidateBatch(blocks); err != nil {
		return fmt.Errorf("batch pre-validation failed: %w", err)
	}

	// Initialize batch caches for intra-batch dependencies
	// Note: UTXOs don't need caching - IndexedBatch supports Get() for uncommitted data
	bc.batchCacheMu.Lock()
	bc.batchBlocks = make(map[types.Hash]*types.Block)
	bc.batchHeights = make(map[types.Hash]uint32)
	bc.batchHashes = make(map[uint32]types.Hash)
	bc.batchModifiers = make(map[types.Hash]uint64)
	bc.batchMoneySupply = make(map[uint32]int64)
	bc.batchCacheMu.Unlock()

	// Clear batch caches after processing
	defer func() {
		bc.batchCacheMu.Lock()
		bc.batchBlocks = nil
		bc.batchHeights = nil
		bc.batchHashes = nil
		bc.batchModifiers = nil
		bc.batchMoneySupply = nil
		bc.batchCacheMu.Unlock()
	}()

	// Open single batch for all blocks
	batch := bc.storage.NewBatch()

	// Process all blocks in single batch
	var height uint32
	for i, block := range blocks {
		blockHash := block.Hash()

		// Calculate height: first block from storage, others increment
		if i == 0 {
			var err error
			height, err = bc.calculateBlockHeight(block)
			if err != nil {
				syncNeeded = true
				return fmt.Errorf("block 0 height calculation failed: %w", err)
			}
		} else {
			height++
		}

		// Store height and hash in batch cache for intra-batch dependencies
		bc.batchCacheMu.Lock()
		bc.batchHeights[blockHash] = height
		bc.batchHashes[height] = blockHash
		bc.batchCacheMu.Unlock()

		// Validate checkpoint
		if bc.checkpoints != nil {
			if err := bc.checkpoints.ValidateCheckpoint(height, blockHash); err != nil {
				syncNeeded = true
				return fmt.Errorf("%w at height %d: %v", ErrCheckpointFailed, height, err)
			}
		}

		// Store block in batch cache before validation (for GetBlock during validation)
		bc.batchCacheMu.Lock()
		bc.batchBlocks[blockHash] = block
		bc.batchCacheMu.Unlock()

		// Compute and store stake modifier for every block during processing
		// Done before validation so modifiers are available to later blocks in the same batch
		if bc.consensus != nil {
			modifier, generated, err := bc.consensus.ComputeAndStoreModifier(block, height, batch)
			if err != nil {
				// Log warning but don't fail - modifier can be recomputed later if needed
				bc.logger.WithError(err).WithFields(map[string]interface{}{
					"height": height,
					"hash":   blockHash.String(),
				}).Warn("Failed to compute stake modifier")
			} else if generated {
				// CRITICAL: Only cache generated modifiers (not inherited ones)
				// This matches the storage behavior and maintains legacy consensus compatibility
				bc.batchCacheMu.Lock()
				if bc.batchModifiers != nil {
					bc.batchModifiers[blockHash] = modifier
				}
				bc.batchCacheMu.Unlock()
			}
		}

		// Validate block - choose validation method based on context:
		// 1. Single block after IBD: Full validation (ValidateBlock) - all UTXOs are in committed storage
		// 2. Batch processing (IBD or multi-block): ValidateBlockForBatch - skips UTXO lookup
		//    because intra-batch UTXOs may not be committed yet (MarkUTXOSpent will verify)
		// 3. IBD simplified: Skip most validation for speed, rely on checkpoints
		isSingleBlockAfterIBD := !isIBD && len(blocks) == 1
		if isSingleBlockAfterIBD {
			// Full validation for single blocks after sync - all UTXOs are in storage
			if err := bc.consensus.ValidateBlock(block); err != nil {
				syncNeeded = true
				return fmt.Errorf("block validation failed at height %d %s: %w", height, block.Hash().String(), err)
			}
		} else if !isIBD || (height > ValidationHeightThreshold && height%ValidationFrequency == 0) {
			// Batch validation - skips UTXO lookup for intra-batch dependencies
			if err := bc.consensus.ValidateBlockForBatch(block); err != nil {
				syncNeeded = true
				return fmt.Errorf("block validation failed at height %d %s: %w", height, block.Hash().String(), err)
			}
		} else {
			// IBD simplified validation - rely on checkpoints for security
			if err := bc.validateBlockSimplified(block, height); err != nil {
				syncNeeded = true
				return fmt.Errorf("simplified validation failed at height %d: %w", height, err)
			}
		}

		// Apply UTXO changes and store block.
		// spentOutputs captures data from MarkUTXOSpent to avoid redundant GetTransaction
		// calls in the indexing path (~18% CPU savings during sync).
		spentOutputs, err := bc.applyBlockToBatch(block, height, batch)
		if err != nil {
			syncNeeded = true
			return fmt.Errorf("batch[%d] height %d (hash %s) apply failed: %w", i, height, block.Hash().String(), err)
		}

		// Index transactions using precomputed spent outputs
		if err := bc.indexBlockTransactions(block, height, batch, spentOutputs); err != nil {
			syncNeeded = true
			return fmt.Errorf("block %d indexing failed: %w", i, err)
		}
	}

	// Commit entire batch after all blocks processed
	if err := batch.Commit(); err != nil {
		if rollbackErr := batch.Rollback(); rollbackErr != nil {
			bc.logger.WithError(rollbackErr).Error("Failed to rollback after commit error")
		}
		syncNeeded = true
		return fmt.Errorf("batch commit failed: %w", err)
	}

	// Sync storage to flush indices
	if err := bc.storage.Sync(); err != nil {
		return fmt.Errorf("storage sync failed: %w", err)
	}

	// Notify wallet about new blocks
	if bc.wallet != nil {
		if err := bc.wallet.NotifyBlocks(blocks); err != nil {
			bc.logger.WithError(err).Warn("Failed to notify wallet about blocks")
			// Don't fail the batch processing, just log the error
		}
	}

	// Remove confirmed and conflicting transactions from mempool.
	// This handles both locally-staked and P2P-received blocks.
	if bc.mempool != nil {
		for _, block := range blocks {
			bc.mempool.RemoveConfirmedTransactions(block)
		}
	}

	// Notify masternode manager about new blocks for winner vote creation
	// Legacy: CMasternodePayments::ProcessBlock() called after block connection
	// This triggers automatic winner vote creation and relay for active masternodes
	if bc.masternodeManager != nil {
		for _, block := range blocks {
			bc.batchCacheMu.RLock()
			blockHeight := bc.batchHeights[block.Hash()]
			bc.batchCacheMu.RUnlock()

			// ProcessBlockForWinner creates and relays winner vote if we're an active masternode
			// It calculates vote for height + 10 blocks ahead (WinnerVoteBlocksAhead)
			if _, err := bc.masternodeManager.ProcessBlockForWinner(blockHeight); err != nil {
				bc.logger.WithError(err).WithField("height", blockHeight).Warn("Failed to process block for winner vote")
				// Don't fail batch processing - winner voting is non-critical
			}
		}
	}

	// Notify block announcement tracker to update peer heights
	// This updates heights for ALL peers who announced these blocks, not just the one who delivered them
	if bc.blockAnnouncementNotifier != nil && len(blocks) > 0 {
		// Build heights map from batch cache
		bc.batchCacheMu.RLock()
		heights := make(map[types.Hash]uint32, len(blocks))
		for _, block := range blocks {
			heights[block.Hash()] = bc.batchHeights[block.Hash()]
		}
		bc.batchCacheMu.RUnlock()

		bc.blockAnnouncementNotifier.NotifyBlocksProcessed(blocks, heights)
	}

	// Update in-memory state after successful commit.
	// Only advance the tip if the last block is higher than current best.
	if len(blocks) > 0 {
		lastBlock := blocks[len(blocks)-1]

		bc.batchCacheMu.RLock()
		lastHeight := bc.batchHeights[lastBlock.Hash()]
		bc.batchCacheMu.RUnlock()

		tipAdvanced := lastHeight > bc.bestHeight.Load()
		if tipAdvanced {
			bc.mu.Lock()
			bc.bestBlock = lastBlock
			bc.bestHash = lastBlock.Hash()
			bc.bestHeight.Store(lastHeight)
			bc.mu.Unlock()
		}

		// Update block index for all blocks; stats only when tip advanced.
		for _, block := range blocks {
			bc.batchCacheMu.RLock()
			height := bc.batchHeights[block.Hash()]
			bc.batchCacheMu.RUnlock()
			bc.updateBlockIndex(block, height, BlockStatusConnected)
			if tipAdvanced {
				bc.updateStats(block)
			}
		}
	}

	// Clean up mempool for all blocks
	if binaryStorage, ok := bc.storage.(*binary.BinaryStorage); ok {
		for _, block := range blocks {
			for _, tx := range block.Transactions {
				binaryStorage.DeleteMempoolTransaction(tx.Hash())
			}
		}
	}

	return nil
}

// applyBlockToBatch applies a block's UTXO changes to the batch and returns
// spent output data keyed by outpoint. The returned map enables indexBlockTransactions
// to skip redundant DB lookups — MarkUTXOSpent already reads the UTXO, so we
// capture the output (Value + ScriptPubKey) here instead of re-fetching via GetTransaction.
// Uses batch-level UTXO cache to handle UTXOs created and spent within same batch of blocks
// Uses the new simplified storage schema from Phase 1
// Also calculates and stores the money supply incrementally
func (bc *BlockChain) applyBlockToBatch(block *types.Block, height uint32, batch storage.Batch) (map[types.Outpoint]*types.TxOutput, error) {
	blockHash := block.Hash()

	// Store block index FIRST (required for lookups)
	if err := batch.StoreBlockIndex(blockHash, height); err != nil {
		return nil, fmt.Errorf("failed to store block index: %w", err)
	}

	// Store block data with height (compact format + direct transaction storage)
	// StoreBlockWithHeight handles both compact block AND direct transaction storage internally
	if err := batch.StoreBlockWithHeight(block, height); err != nil {
		return nil, fmt.Errorf("failed to store block: %w", err)
	}

	// Track value in/out for money supply calculation
	// MoneySupply = PrevSupply + TotalOutputs - TotalInputs
	var totalValueIn int64
	var totalValueOut int64

	// Collect spent outputs for use by indexBlockTransactions.
	// This avoids re-reading the same UTXO data via expensive GetTransaction calls.
	spentOutputs := make(map[types.Outpoint]*types.TxOutput, 32)

	// Process transactions
	for _, tx := range block.Transactions {
		txHash := tx.Hash()

		// Process inputs (spend UTXOs)
		if !tx.IsCoinbase() {
			for _, input := range tx.Inputs {
				// Mark UTXO as spent and get its data (mark-as-spent model for faster block disconnect)
				// IndexedBatch supports Get() so it can read UTXOs created earlier in this batch
				utxo, err := batch.MarkUTXOSpent(input.PreviousOutput, height, txHash)
				if err != nil {
					// Check for fork duplicate spend signal from storage layer
					if forkHeight := parseForkDuplicateSpendHeight(err); forkHeight > 0 {
						return nil, &ErrForkDuplicateSpend{
							ForkHeight: forkHeight,
							TxHash:     txHash.String(),
							Outpoint:   fmt.Sprintf("%s:%d", input.PreviousOutput.Hash.String(), input.PreviousOutput.Index),
						}
					}
					return nil, fmt.Errorf("failed to mark UTXO as spent (height %d, block %s): %w", height, blockHash.String(), err)
				}

				// Track input value for money supply
				totalValueIn += utxo.Output.Value

				// Capture spent output for address indexing (eliminates GetTransaction in indexing path)
				spentOutputs[input.PreviousOutput] = utxo.Output
			}
		}

		// Create new UTXOs
		for outIdx, output := range tx.Outputs {
			outpoint := types.Outpoint{
				Hash:  txHash,
				Index: uint32(outIdx),
			}

			isCoinbase := tx.IsCoinbase() || tx.IsCoinStake()

			// Track output value for money supply
			totalValueOut += output.Value

			// Store UTXO with new schema (includes script hash for address indexing)
			// IndexedBatch makes this UTXO readable immediately for subsequent transactions
			if err := batch.StoreUTXO(outpoint, output, height, isCoinbase); err != nil {
				return nil, fmt.Errorf("failed to store UTXO: %w", err)
			}
		}
	}

	// Calculate and store money supply
	// Formula: moneySupply = prevSupply + valueOut - valueIn
	var prevSupply int64
	if height > 0 {
		// First check batch cache (for blocks in same batch)
		var foundInCache bool
		bc.batchCacheMu.RLock()
		if bc.batchMoneySupply != nil {
			if cached, ok := bc.batchMoneySupply[height-1]; ok {
				prevSupply = cached
				foundInCache = true
			}
		}
		bc.batchCacheMu.RUnlock()

		// Fall back to storage
		if !foundInCache {
			var err error
			prevSupply, err = bc.storage.GetMoneySupply(height - 1)
			if err != nil {
				// Log warning but don't fail - we might be processing first blocks
				bc.logger.WithError(err).WithField("height", height-1).Debug("Could not get previous money supply")
			}
		}
	}
	moneySupply := prevSupply + totalValueOut - totalValueIn

	// Store in batch cache for subsequent blocks in this batch
	bc.batchCacheMu.Lock()
	if bc.batchMoneySupply != nil {
		bc.batchMoneySupply[height] = moneySupply
	}
	bc.batchCacheMu.Unlock()

	if err := batch.StoreMoneySupply(height, moneySupply); err != nil {
		return nil, fmt.Errorf("failed to store money supply: %w", err)
	}

	// Store PoS metadata (stakeModifierChecksum + hashProofOfStake) for checksum chaining
	checksum := block.GetStakeModifierChecksum()
	proofHash := block.GetHashProofOfStake()
	if err := batch.StoreBlockPoSMetadata(blockHash, checksum, proofHash); err != nil {
		return nil, fmt.Errorf("failed to store PoS metadata: %w", err)
	}

	// Update chain tip only if this block advances the chain.
	// Side-chain blocks are stored but do not move the tip backward.
	if height > bc.bestHeight.Load() {
		if err := batch.SetChainState(height, blockHash); err != nil {
			return nil, err
		}
	}

	return spentOutputs, nil
}

// parseForkDuplicateSpendHeight extracts the fork height from a storage-layer
// "fork duplicate spend" error. Returns 0 if the error is not a fork signal.
// The storage layer encodes the fork height as [fork_height=N] in the message.
func parseForkDuplicateSpendHeight(err error) uint32 {
	msg := err.Error()
	const marker = "[fork_height="
	idx := strings.Index(msg, marker)
	if idx < 0 {
		return 0
	}
	start := idx + len(marker)
	end := strings.Index(msg[start:], "]")
	if end < 0 {
		return 0
	}
	h, parseErr := strconv.ParseUint(msg[start:start+end], 10, 32)
	if parseErr != nil {
		return 0
	}
	return uint32(h)
}

// validateBlockSimplified performs minimal validation during IBD
// Phase 3: Optimized validation for fast sync
func (bc *BlockChain) validateBlockSimplified(block *types.Block, height uint32) error {
	// Basic structure validation
	if block == nil || block.Header == nil {
		return fmt.Errorf("invalid block structure")
	}

	// Timestamp validation - not too far in future
	// CRITICAL: Legacy C++ (main.cpp:3921) uses different limits for PoS vs PoW:
	// - PoS blocks: 180 seconds (3 minutes)
	// - PoW blocks: 7200 seconds (2 hours)
	var maxFutureDrift time.Duration
	if block.IsProofOfStake() {
		maxFutureDrift = 180 * time.Second // 3 minutes for PoS
	} else {
		maxFutureDrift = 7200 * time.Second // 2 hours for PoW
	}
	maxFutureTime := time.Now().Add(maxFutureDrift)
	blockTime := time.Unix(int64(block.Header.Timestamp), 0)
	if blockTime.After(maxFutureTime) {
		return fmt.Errorf("block timestamp too far in future: %v (max drift: %v)", blockTime, maxFutureDrift)
	}

	// Basic PoW validation (only for PoW blocks, not PoS)
	// TWINS switched from PoW to PoS at block 400
	const LastPOWBlock = 400
	blockHash := block.Hash()

	// Only validate PoW difficulty for PoW blocks (height <= 400)
	// PoS blocks (height > 400) use different consensus mechanism
	if height <= LastPOWBlock {
		target := types.CompactToBig(block.Header.Bits)
		if blockHash.ToBig().Cmp(target) > 0 {
			return fmt.Errorf("block hash doesn't meet difficulty target")
		}
	}

	// Transaction count validation
	if len(block.Transactions) == 0 {
		return fmt.Errorf("block has no transactions")
	}

	// Coinbase/coinstake validation
	if !block.Transactions[0].IsCoinbase() && !block.Transactions[0].IsCoinStake() {
		return fmt.Errorf("first transaction must be coinbase or coinstake")
	}

	// Skip expensive script validation during IBD
	// Skip signature validation during IBD
	// Skip full UTXO validation during IBD

	bc.logger.WithFields(map[string]interface{}{
		"height": height,
		"hash":   blockHash.String(),
	}).Trace("Simplified validation passed")

	return nil
}

// preValidateBatch performs preliminary validation on a batch of blocks
// Phase 3: Ensures batch integrity before expensive operations
func (bc *BlockChain) preValidateBatch(blocks []*types.Block) error {
	if len(blocks) == 0 {
		return nil
	}

	// Validate first block connects to our chain
	firstBlock := blocks[0]
	if !firstBlock.Header.PrevBlockHash.IsZero() {
		// Check parent exists (unless genesis)
		hasParent, err := bc.hasBlock(firstBlock.Header.PrevBlockHash)
		if err != nil {
			return fmt.Errorf("failed to check parent: %w", err)
		}
		if !hasParent {
			return fmt.Errorf("first block parent %s not found", firstBlock.Header.PrevBlockHash.String())
		}
	}

	// Validate chain continuity within batch
	for i := 1; i < len(blocks); i++ {
		if blocks[i].Header.PrevBlockHash != blocks[i-1].Hash() {
			return fmt.Errorf("batch discontinuity at index %d: expected parent %s, got %s",
				i, blocks[i-1].Hash().String(), blocks[i].Header.PrevBlockHash.String())
		}
	}

	// Check for duplicate blocks
	seen := make(map[types.Hash]int)
	for i, block := range blocks {
		hash := block.Hash()
		if prevIdx, exists := seen[hash]; exists {
			return fmt.Errorf("duplicate block %s at indices %d and %d", hash.String(), prevIdx, i)
		}
		seen[hash] = i
	}

	// Validate basic block structure
	for i, block := range blocks {
		// Check block has transactions
		if len(block.Transactions) == 0 {
			return fmt.Errorf("block %d has no transactions", i)
		}

		// First transaction must be coinbase
		if !block.Transactions[0].IsCoinbase() && !block.Transactions[0].IsCoinStake() {
			return fmt.Errorf("block %d first tx is not coinbase/coinstake", i)
		}

		// Validate merkle root
		calculatedMerkle := block.CalculateMerkleRoot()
		if calculatedMerkle != block.Header.MerkleRoot {
			return fmt.Errorf("block %d merkle root mismatch: expected %s, calculated %s",
				i, block.Header.MerkleRoot.String(), calculatedMerkle.String())
		}
	}

	return nil
}
