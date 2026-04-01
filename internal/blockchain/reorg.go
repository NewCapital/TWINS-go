package blockchain

import (
	"fmt"

	"github.com/twins-dev/twins-core/pkg/types"
)

// ReorganizeChain handles blockchain reorganization from an external caller.
// Acquires processingMu — must NOT be called while the lock is already held.
func (bc *BlockChain) ReorganizeChain(newTipHash types.Hash) error {
	bc.processingMu.Lock()
	defer bc.processingMu.Unlock()
	return bc.reorganizeChainLocked(newTipHash)
}

// reorganizeChainLocked is the internal reorganization implementation.
// Caller MUST hold processingMu.
func (bc *BlockChain) reorganizeChainLocked(newTipHash types.Hash) error {
	bc.logger.WithFields(map[string]interface{}{
		"old_tip": bc.bestHash.String(),
		"new_tip": newTipHash.String(),
	}).Info("Starting chain reorganization")

	// Find the fork point
	forkPoint, oldBlocks, newBlocks, err := bc.findForkPoint(newTipHash)
	if err != nil {
		return fmt.Errorf("failed to find fork point: %w", err)
	}

	bc.logger.WithFields(map[string]interface{}{
		"fork_point":  forkPoint.String(),
		"disconnect":  len(oldBlocks),
		"connect":     len(newBlocks),
	}).Debug("Fork point found")

	// Check reorganization depth
	if len(oldBlocks) > int(bc.config.MaxReorgDepth) {
		return fmt.Errorf("reorganization too deep: %d blocks", len(oldBlocks))
	}

	// Disconnect old blocks
	for i := len(oldBlocks) - 1; i >= 0; i-- {
		if err := bc.disconnectBlock(oldBlocks[i]); err != nil {
			return fmt.Errorf("failed to disconnect block %s: %w",
				oldBlocks[i].Hash().String(), err)
		}
	}

	// Connect new blocks
	for _, block := range newBlocks {
		if err := bc.processBatchUnifiedLocked([]*types.Block{block}); err != nil {
			// Reorg failed, try to restore old chain
			bc.logger.WithError(err).Error("Failed to connect block during reorg, attempting rollback")
			bc.attemptRollback(oldBlocks)
			return fmt.Errorf("failed to connect block %s: %w",
				block.Hash().String(), err)
		}
	}

	bc.logger.WithFields(map[string]interface{}{
		"new_height": bc.bestHeight.Load(),
		"new_tip":    bc.bestHash.String(),
	}).Info("Chain reorganization completed")

	return nil
}

// findForkPoint finds the common ancestor between current chain and new chain
func (bc *BlockChain) findForkPoint(newTipHash types.Hash) (types.Hash, []*types.Block, []*types.Block, error) {
	// Build path from new tip to genesis
	newChain := make([]*types.Block, 0)
	currentHash := newTipHash

	for !currentHash.IsZero() {
		block, err := bc.storage.GetBlock(currentHash)
		if err != nil {
			return types.ZeroHash, nil, nil, fmt.Errorf("failed to get block %s: %w",
				currentHash.String(), err)
		}

		newChain = append([]*types.Block{block}, newChain...)
		currentHash = block.Header.PrevBlockHash

		// Check if this block is on the main chain
		isOnMain, err := bc.IsOnMainChain(block.Hash())
		if err != nil {
			return types.ZeroHash, nil, nil, err
		}

		if isOnMain {
			// Found fork point
			forkPoint := block.Hash()

			// Get blocks to disconnect from old chain
			oldChain := make([]*types.Block, 0)
			currentHash = bc.bestHash

			for currentHash != forkPoint {
				block, err := bc.storage.GetBlock(currentHash)
				if err != nil {
					return types.ZeroHash, nil, nil, err
				}
				oldChain = append(oldChain, block)
				currentHash = block.Header.PrevBlockHash
			}

			// Remove fork point from new chain
			newChain = newChain[1:]

			return forkPoint, oldChain, newChain, nil
		}
	}

	return types.ZeroHash, nil, nil, fmt.Errorf("no common ancestor found")
}

// attemptRollback attempts to rollback a failed reorganization
func (bc *BlockChain) attemptRollback(blocks []*types.Block) {
	bc.logger.Warn("Attempting to rollback failed reorganization")

	// Try to reconnect old blocks
	for _, block := range blocks {
		if err := bc.processBatchUnifiedLocked([]*types.Block{block}); err != nil {
			bc.logger.WithError(err).WithField("block", block.Hash().String()).
				Error("Failed to rollback block - blockchain may be in inconsistent state")
		}
	}
}

// CheckReorgNeeded checks if a reorganization is needed for a new block
func (bc *BlockChain) CheckReorgNeeded(block *types.Block) (bool, error) {
	bc.indexMu.RLock()
	defer bc.indexMu.RUnlock()

	blockHash := block.Hash()

	// Get the new block's node
	newNode, exists := bc.blockIndex[blockHash]
	if !exists {
		return false, fmt.Errorf("block not in index")
	}

	// Get current tip node
	currentNode, exists := bc.blockIndex[bc.bestHash]
	if !exists {
		// No current tip, this is the first block
		return true, nil
	}

	// Compare work
	if newNode.Work.Int64() > currentNode.Work.Int64() {
		return true, nil
	}

	return false, nil
}

// ValidateReorg validates that a reorganization is safe
func (bc *BlockChain) ValidateReorg(newTipHash types.Hash) error {
	// Check if new tip exists
	newTip, err := bc.storage.GetBlock(newTipHash)
	if err != nil {
		return fmt.Errorf("new tip block not found: %w", err)
	}

	// Validate the new tip
	if err := bc.consensus.ValidateBlock(newTip); err != nil {
		return fmt.Errorf("new tip validation failed: %w", err)
	}

	// Check reorg depth
	forkPoint, oldBlocks, _, err := bc.findForkPoint(newTipHash)
	if err != nil {
		return err
	}

	if len(oldBlocks) > int(bc.config.MaxReorgDepth) {
		return fmt.Errorf("reorg depth %d exceeds maximum %d",
			len(oldBlocks), bc.config.MaxReorgDepth)
	}

	bc.logger.WithFields(map[string]interface{}{
		"fork_point": forkPoint.String(),
		"depth":      len(oldBlocks),
	}).Debug("Reorganization validated")

	return nil
}

// GetReorgInfo returns information about a potential reorganization
func (bc *BlockChain) GetReorgInfo(newTipHash types.Hash) (*ReorgInfo, error) {
	forkPoint, oldBlocks, newBlocks, err := bc.findForkPoint(newTipHash)
	if err != nil {
		return nil, err
	}

	info := &ReorgInfo{
		ForkPoint:    forkPoint,
		OldTip:       bc.bestHash,
		NewTip:       newTipHash,
		OldHeight:    bc.bestHeight.Load(),
		BlocksToDisconnect: len(oldBlocks),
		BlocksToConnect:    len(newBlocks),
	}

	// Calculate new height
	newHeight, err := bc.storage.GetBlockHeight(forkPoint)
	if err != nil {
		return nil, err
	}
	info.NewHeight = newHeight + uint32(len(newBlocks))

	return info, nil
}

// ReorgInfo contains information about a reorganization
type ReorgInfo struct {
	ForkPoint          types.Hash
	OldTip             types.Hash
	NewTip             types.Hash
	OldHeight          uint32
	NewHeight          uint32
	BlocksToDisconnect int
	BlocksToConnect    int
}

func (r *ReorgInfo) String() string {
	return fmt.Sprintf("Reorg: fork=%s old=%s->%d new=%s->%d disconnect=%d connect=%d",
		r.ForkPoint.String()[:8],
		r.OldTip.String()[:8],
		r.OldHeight,
		r.NewTip.String()[:8],
		r.NewHeight,
		r.BlocksToDisconnect,
		r.BlocksToConnect,
	)
}