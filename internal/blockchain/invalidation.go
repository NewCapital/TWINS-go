package blockchain

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/twins-dev/twins-core/pkg/types"
)

// InvalidateBlock marks a block and all its descendants as invalid
// This triggers a chain reorganization if the block is on the main chain
func (bc *BlockChain) InvalidateBlock(hash types.Hash) error {
	bc.logger.WithField("hash", hash.String()).Warn("InvalidateBlock: Marking block as invalid")

	// Check if block exists
	_, err := bc.GetBlock(hash)
	if err != nil {
		return fmt.Errorf("block not found: %w", err)
	}

	// Get block height
	height, err := bc.GetBlockHeight(hash)
	if err != nil {
		return fmt.Errorf("failed to get block height: %w", err)
	}

	// NOTE: For full atomicity, batch operations would be needed. Currently,
	// if MarkBlockInvalid succeeds but markDescendantsInvalid fails, we could
	// have partial state. This is acceptable because:
	// 1. InvalidateBlock is called rarely (manual intervention or fork recovery)
	// 2. The partial state is recoverable - unmarked descendants will be invalid
	//    when their parent is checked during validation
	// 3. Adding MarkBlockInvalid to Batch interface requires storage layer changes

	// Mark block as invalid in storage
	if err := bc.storage.MarkBlockInvalid(hash); err != nil {
		return fmt.Errorf("failed to mark block invalid: %w", err)
	}

	// Update block index status
	bc.indexMu.Lock()
	if node, exists := bc.blockIndex[hash]; exists {
		node.Status = BlockStatusInvalid
	}
	bc.indexMu.Unlock()

	// Mark all descendant blocks as invalid
	// Note: This performs multiple storage writes. If it fails partway through,
	// some descendants may be marked invalid while others are not.
	if err := bc.markDescendantsInvalid(hash); err != nil {
		// This is a critical error - we've marked the parent invalid but failed
		// to mark all descendants. The chain state is now inconsistent.
		bc.logger.WithError(err).Error("CRITICAL: Failed to mark all descendants invalid - chain state may be inconsistent")
		return fmt.Errorf("failed to mark descendants invalid: %w", err)
	}

	// Check if this block is on the main chain
	isMainChain, err := bc.IsOnMainChain(hash)
	if err != nil {
		return fmt.Errorf("failed to check if block is on main chain: %w", err)
	}

	if isMainChain {
		bc.logger.WithFields(logrus.Fields{
			"hash":   hash.String(),
			"height": height,
		}).Info("Invalid block is on main chain, triggering reorganization")

		// Find the best valid ancestor
		bestValidHeight, bestValidHash, err := bc.findBestValidAncestor(hash)
		if err != nil {
			return fmt.Errorf("failed to find best valid ancestor: %w", err)
		}

		// Trigger reorganization by rolling back to best valid ancestor
		if bc.recoveryManager != nil {
			bc.logger.WithFields(logrus.Fields{
				"rollback_to_height": bestValidHeight,
				"rollback_to_hash":   bestValidHash.String(),
			}).Info("Rolling back to best valid ancestor")

			// Use recovery manager to handle the reorganization
			// Note: RollbackToHeight handles everything including:
			// - Disconnecting blocks
			// - Updating UTXO set
			// - Updating in-memory chain state
			// - Persisting new chain state to storage
			if err := bc.recoveryManager.RollbackToHeight(bestValidHeight); err != nil {
				return fmt.Errorf("failed to rollback chain: %w", err)
			}

			// No need to call ResetChainState - RollbackToHeight already updated storage
		} else {
			// Fallback: manual reorganization if recovery manager not available
			if err := bc.reorganizeToValidChain(bestValidHeight, bestValidHash); err != nil {
				return fmt.Errorf("failed to reorganize chain: %w", err)
			}
		}

		bc.logger.WithFields(logrus.Fields{
			"new_height": bestValidHeight,
			"new_tip":    bestValidHash.String(),
		}).Info("Chain reorganized to best valid chain")
	}

	bc.logger.WithField("hash", hash.String()).Info("Block successfully invalidated")
	return nil
}

// ReconsiderBlock removes the invalid status from a block and its ancestors
// This may trigger a chain reorganization if the reconsidered blocks form a better chain
func (bc *BlockChain) ReconsiderBlock(hash types.Hash) error {
	bc.logger.WithField("hash", hash.String()).Debug("ReconsiderBlock: Removing invalid status")

	// Check if block exists
	_, err := bc.GetBlock(hash)
	if err != nil {
		return fmt.Errorf("block not found: %w", err)
	}

	// Check if block is actually marked invalid
	isInvalid, err := bc.storage.IsBlockInvalid(hash)
	if err != nil {
		return fmt.Errorf("failed to check invalid status: %w", err)
	}

	if !isInvalid {
		bc.logger.WithField("hash", hash.String()).Debug("Block is not marked invalid, nothing to reconsider")
		return nil
	}

	// Remove invalid status from storage
	if err := bc.storage.RemoveBlockInvalid(hash); err != nil {
		return fmt.Errorf("failed to remove invalid status: %w", err)
	}

	// Update block index status
	bc.indexMu.Lock()
	if node, exists := bc.blockIndex[hash]; exists {
		node.Status = BlockStatusValid
	}
	bc.indexMu.Unlock()

	// Remove invalid status from all ancestors that were only invalid due to this block
	if err := bc.reconsiderAncestors(hash); err != nil {
		bc.logger.WithError(err).Warn("Failed to reconsider some ancestors")
	}

	// Re-evaluate the best chain considering the reconsidered blocks
	newBestHeight, newBestHash, reorganized, err := bc.findBestChain()
	if err != nil {
		return fmt.Errorf("failed to find best chain: %w", err)
	}

	if reorganized {
		bc.logger.WithFields(logrus.Fields{
			"new_height": newBestHeight,
			"new_tip":    newBestHash.String(),
			"old_height": bc.bestHeight.Load(),
			"old_tip":    bc.bestHash.String(),
		}).Info("Reconsidered block resulted in new best chain")

		// Perform chain reorganization
		if err := bc.reorganizeToValidChain(newBestHeight, newBestHash); err != nil {
			return fmt.Errorf("failed to reorganize to new best chain: %w", err)
		}
	}

	bc.logger.WithField("hash", hash.String()).Info("Block successfully reconsidered")
	return nil
}

// AddCheckpoint adds a dynamic checkpoint at the specified height
func (bc *BlockChain) AddCheckpoint(height uint32, hash types.Hash) error {
	bc.logger.WithFields(logrus.Fields{
		"height": height,
		"hash":   hash.String(),
	}).Debug("AddCheckpoint: Adding dynamic checkpoint")

	// Verify block exists at specified height
	blockAtHeight, err := bc.GetBlockByHeight(height)
	if err != nil {
		return fmt.Errorf("no block at height %d: %w", height, err)
	}

	// Verify hash matches
	if blockAtHeight.Hash() != hash {
		return fmt.Errorf("block at height %d has hash %s, not %s",
			height, blockAtHeight.Hash().String(), hash.String())
	}

	// Add to storage
	if err := bc.storage.AddDynamicCheckpoint(height, hash); err != nil {
		return fmt.Errorf("failed to store checkpoint: %w", err)
	}

	// Add to checkpoint manager if available
	if bc.checkpoints != nil {
		bc.checkpoints.AddDynamicCheckpoint(height, hash)
	}

	// Optionally validate chain up to this checkpoint
	currentHeight, _ := bc.GetBestHeight()
	if height <= currentHeight {
		bc.logger.WithFields(logrus.Fields{
			"checkpoint_height": height,
			"current_height":    currentHeight,
		}).Debug("Validating chain up to new checkpoint")

		// Validate that our chain matches the checkpoint
		isValid := bc.validateChainAtCheckpoint(height, hash)
		if !isValid {
			bc.logger.WithFields(logrus.Fields{
				"height": height,
				"hash":   hash.String(),
			}).Error("Current chain violates new checkpoint, reorganization may be needed")

			// Remove the checkpoint since our chain doesn't match
			bc.storage.RemoveDynamicCheckpoint(height)
			return fmt.Errorf("current chain does not match checkpoint at height %d", height)
		}
	}

	bc.logger.WithFields(logrus.Fields{
		"height": height,
		"hash":   hash.String(),
	}).Info("Dynamic checkpoint successfully added")

	return nil
}

// Helper methods

// markDescendantsInvalid marks all blocks that descend from the given block as invalid
func (bc *BlockChain) markDescendantsInvalid(parentHash types.Hash) error {
	bc.indexMu.Lock()
	defer bc.indexMu.Unlock()

	// Find all blocks that have this block as an ancestor
	// Process synchronously to avoid race conditions
	for hash, node := range bc.blockIndex {
		if bc.isDescendantOf(hash, parentHash) {
			// Mark as invalid in storage
			if err := bc.storage.MarkBlockInvalid(hash); err != nil {
				bc.logger.WithError(err).Errorf("Failed to mark block %s invalid", hash.String())
				return fmt.Errorf("failed to mark %s invalid: %w", hash.String(), err)
			}

			// Update status in index
			node.Status = BlockStatusInvalid
		}
	}

	return nil
}

// isDescendantOf checks if blockHash is a descendant of ancestorHash
func (bc *BlockChain) isDescendantOf(blockHash, ancestorHash types.Hash) bool {
	// Note: indexMu should already be locked by caller

	node, exists := bc.blockIndex[blockHash]
	if !exists {
		return false
	}

	// Walk up the parent chain
	for node != nil {
		if node.Hash == ancestorHash {
			return true
		}
		node = node.Parent
	}

	return false
}

// findBestValidAncestor finds the highest valid ancestor of the given block
func (bc *BlockChain) findBestValidAncestor(hash types.Hash) (uint32, types.Hash, error) {
	block, err := bc.GetBlock(hash)
	if err != nil {
		return 0, types.Hash{}, err
	}

	// Track visited blocks to detect cycles
	visited := make(map[types.Hash]bool)
	visited[hash] = true

	// Walk back through ancestors until we find a valid one
	currentHash := block.Header.PrevBlockHash
	for !currentHash.IsZero() {
		// Check for cycles
		if visited[currentHash] {
			return 0, types.Hash{}, fmt.Errorf("cycle detected in block chain at %s", currentHash.String())
		}
		visited[currentHash] = true

		// Check if this block is invalid
		isInvalid, err := bc.storage.IsBlockInvalid(currentHash)
		if err != nil {
			return 0, types.Hash{}, err
		}

		if !isInvalid {
			// Found a valid ancestor
			height, err := bc.GetBlockHeight(currentHash)
			if err != nil {
				return 0, types.Hash{}, err
			}
			return height, currentHash, nil
		}

		// Move to parent
		ancestorBlock, err := bc.GetBlock(currentHash)
		if err != nil {
			return 0, types.Hash{}, err
		}
		currentHash = ancestorBlock.Header.PrevBlockHash
	}

	// If we get here, we've walked back to genesis
	return 0, bc.getGenesisHash(), nil
}

// reconsiderAncestors removes invalid status from ancestors if appropriate
func (bc *BlockChain) reconsiderAncestors(hash types.Hash) error {
	block, err := bc.GetBlock(hash)
	if err != nil {
		return err
	}

	// Walk up the parent chain
	currentHash := block.Header.PrevBlockHash
	for !currentHash.IsZero() {
		// Check if parent is marked invalid
		isInvalid, err := bc.storage.IsBlockInvalid(currentHash)
		if err != nil {
			return err
		}

		if !isInvalid {
			// Parent is not invalid, we're done
			break
		}

		// Check if parent has any other invalid children
		// Use write lock since we may modify the index
		hasOtherInvalidChildren := false
		bc.indexMu.Lock()
		for _, node := range bc.blockIndex {
			if node.Parent != nil && node.Parent.Hash == currentHash {
				// This is a child of currentHash
				if node.Hash != hash {
					// Check if this other child is invalid
					isOtherChildInvalid, err := bc.storage.IsBlockInvalid(node.Hash)
					if err != nil {
						bc.logger.WithError(err).Warn("Failed to check invalid status for child block")
						// Conservative: assume it's invalid if we can't check
						hasOtherInvalidChildren = true
						break
					}
					if isOtherChildInvalid {
						hasOtherInvalidChildren = true
						break
					}
				}
			}
		}

		// While we have the lock, update the index if needed
		if !hasOtherInvalidChildren {
			// Remove invalid status from parent in storage first
			if err := bc.storage.RemoveBlockInvalid(currentHash); err != nil {
				bc.indexMu.Unlock()
				return err
			}

			// Update index status while lock is held
			if node, exists := bc.blockIndex[currentHash]; exists {
				node.Status = BlockStatusValid
			}
		}
		bc.indexMu.Unlock()

		// Move to next parent
		parentBlock, err := bc.GetBlock(currentHash)
		if err != nil {
			return err
		}
		currentHash = parentBlock.Header.PrevBlockHash
	}

	return nil
}

// findBestChain finds the best valid chain considering all blocks
func (bc *BlockChain) findBestChain() (uint32, types.Hash, bool, error) {
	bc.indexMu.RLock()
	defer bc.indexMu.RUnlock()

	var bestNode *BlockNode
	var bestWork *types.BigInt

	// Find the valid chain tip with most work
	for hash, node := range bc.blockIndex {
		// Skip invalid blocks
		isInvalid, _ := bc.storage.IsBlockInvalid(hash)
		if isInvalid {
			continue
		}

		// Check if this chain has more work
		if bestWork == nil || node.Work.Cmp(bestWork) > 0 {
			bestNode = node
			bestWork = node.Work
		}
	}

	if bestNode == nil {
		return 0, types.Hash{}, false, fmt.Errorf("no valid chain found")
	}

	// Check if this is different from current best
	reorganized := bestNode.Hash != bc.bestHash

	return bestNode.Height, bestNode.Hash, reorganized, nil
}

// reorganizeToValidChain performs a chain reorganization to a valid chain.
// NOTE: Fallback only — used when recovery manager is not available.
// Does NOT handle UTXO set updates; chain state may be inconsistent after.
func (bc *BlockChain) reorganizeToValidChain(newHeight uint32, newHash types.Hash) error {
	bc.logger.Warn("Using fallback reorganization without UTXO updates - chain state may be inconsistent")

	bc.processingMu.Lock()
	defer bc.processingMu.Unlock()

	// Fetch block before acquiring mu (GetBlock may take mu.RLock internally)
	newBest, err := bc.GetBlock(newHash)
	if err != nil {
		return err
	}

	bc.mu.Lock()
	bc.bestBlock = newBest
	bc.bestHeight.Store(newHeight)
	bc.bestHash = newHash
	bc.mu.Unlock()

	// Update chain state in storage
	if err := bc.storage.SetChainState(newHeight, newHash); err != nil {
		return err
	}

	// WARNING: UTXO set is NOT updated here. This could cause transaction
	// validation failures until the node is restarted and resyncs.
	bc.logger.Error("UTXO set not updated during reorganization - manual resync may be required")

	return nil
}


// validateChainAtCheckpoint validates that the chain matches a checkpoint
func (bc *BlockChain) validateChainAtCheckpoint(height uint32, expectedHash types.Hash) bool {
	block, err := bc.GetBlockByHeight(height)
	if err != nil {
		return false
	}

	return block.Hash() == expectedHash
}

// getGenesisHash returns the genesis block hash
func (bc *BlockChain) getGenesisHash() types.Hash {
	// This should be cached or retrieved from chain params
	genesisBlock, _ := bc.GetBlockByHeight(0)
	if genesisBlock != nil {
		return genesisBlock.Hash()
	}
	return types.Hash{}
}

// AddDynamicCheckpoint adds a dynamic checkpoint to the checkpoint manager
func (cm *CheckpointManager) AddDynamicCheckpoint(height uint32, hash types.Hash) {
	if cm.checkpoints == nil {
		cm.checkpoints = make(map[uint32]types.Hash)
	}
	cm.checkpoints[height] = hash
	if height > cm.lastHeight {
		cm.lastHeight = height
	}
}