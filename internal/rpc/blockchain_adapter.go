package rpc

import (
	"fmt"

	"github.com/twins-dev/twins-core/internal/blockchain"
	"github.com/twins-dev/twins-core/pkg/types"
)

// BlockchainAdapter adapts blockchain.Blockchain to rpc.BlockchainInterface
type BlockchainAdapter struct {
	bc blockchain.Blockchain
}

// NewBlockchainAdapter creates a new blockchain adapter
func NewBlockchainAdapter(bc blockchain.Blockchain) *BlockchainAdapter {
	return &BlockchainAdapter{bc: bc}
}

// Block retrieval
func (a *BlockchainAdapter) GetBestBlock() (*types.Block, error) {
	return a.bc.GetBestBlock()
}

func (a *BlockchainAdapter) GetBestHeight() (uint32, error) {
	return a.bc.GetBestHeight()
}

func (a *BlockchainAdapter) GetBestBlockHash() (types.Hash, error) {
	return a.bc.GetBestBlockHash()
}

func (a *BlockchainAdapter) GetBlock(hash types.Hash) (*types.Block, error) {
	return a.bc.GetBlock(hash)
}

func (a *BlockchainAdapter) GetBlockByHeight(height uint32) (*types.Block, error) {
	return a.bc.GetBlockByHeight(height)
}

func (a *BlockchainAdapter) GetBlockHash(height uint32) (types.Hash, error) {
	// Delegate to the blockchain's GetBlockHash which handles genesis correctly
	return a.bc.GetBlockHash(height)
}

func (a *BlockchainAdapter) GetBlockHeight() (uint32, error) {
	return a.bc.GetBestHeight()
}

func (a *BlockchainAdapter) GetBlockHeightByHash(hash types.Hash) (uint32, error) {
	return a.bc.GetBlockHeight(hash)
}

// Chain info
func (a *BlockchainAdapter) GetChainWork() (string, error) {
	work, err := a.bc.GetChainWork()
	if err != nil {
		return "0", err
	}
	return work.String(), nil
}

func (a *BlockchainAdapter) GetDifficulty() (float64, error) {
	// Calculate from best block bits using TWINS PoS parameters
	bestBlock, err := a.bc.GetBestBlock()
	if err != nil {
		return 1.0, err
	}

	return calculateDifficultyFromBits(bestBlock.Header.Bits), nil
}

// calculateDifficultyFromBits converts compact bits to difficulty value
// Uses Bitcoin-style formula for RPC display compatibility with legacy implementation.
// Reference: legacy/src/rpc/blockchain.cpp:94-120 GetDifficulty()
func calculateDifficultyFromBits(bits uint32) float64 {
	if bits == 0 {
		return 0
	}

	// Extract exponent (nShift) from high byte
	nShift := int((bits >> 24) & 0xff)

	// Extract mantissa from lower 3 bytes
	mantissa := bits & 0x00ffffff
	if mantissa == 0 {
		return 0
	}

	// Bitcoin-style difficulty formula:
	// Base difficulty = 0x0000ffff / mantissa
	// Then adjust for exponent difference from reference exponent 29
	dDiff := float64(0x0000ffff) / float64(mantissa)

	// Adjust for exponent: multiply by 256 for each step below 29,
	// divide by 256 for each step above 29
	for nShift < 29 {
		dDiff *= 256.0
		nShift++
	}
	for nShift > 29 {
		dDiff /= 256.0
		nShift--
	}

	return dDiff
}

func (a *BlockchainAdapter) GetChainTips() ([]ChainTip, error) {
	tips, err := a.bc.GetChainTips()
	if err != nil {
		return nil, err
	}

	result := make([]ChainTip, len(tips))
	for i, tip := range tips {
		status := "unknown"
		switch tip.Status {
		case blockchain.ChainTipActive:
			status = "active"
		case blockchain.ChainTipOrphan:
			status = "orphan"
		case blockchain.ChainTipValidHeaders:
			status = "valid-headers"
		}

		// Calculate branch length (blocks from main chain fork point)
		branchLen := 0
		if status != "active" {
			// For non-active tips, calculate distance from main chain
			// This would require walking back to find the fork point
			// For now, approximate based on height difference from best height
			bestHeight, err := a.bc.GetBestHeight()
			if err == nil && tip.Height < bestHeight {
				branchLen = int(bestHeight - tip.Height)
			}
		}

		result[i] = ChainTip{
			Height:    int64(tip.Height),
			Hash:      tip.Hash.String(),
			BranchLen: branchLen,
			Status:    status,
		}
	}
	return result, nil
}

func (a *BlockchainAdapter) GetBlockCount() (int64, error) {
	height, err := a.bc.GetBestHeight()
	return int64(height), err
}

// Validation
func (a *BlockchainAdapter) ValidateBlock(block *types.Block) error {
	// Blockchain doesn't have ValidateBlock, use ProcessBlock logic
	return a.bc.ProcessBlock(block)
}

func (a *BlockchainAdapter) ProcessBlock(block *types.Block) error {
	return a.bc.ProcessBlock(block)
}

// UTXO operations
func (a *BlockchainAdapter) GetUTXO(outpoint types.Outpoint) (*types.TxOutput, error) {
	utxo, err := a.bc.GetUTXO(outpoint)
	if err != nil {
		return nil, err
	}
	if utxo == nil {
		return nil, nil
	}
	return utxo.Output, nil
}

func (a *BlockchainAdapter) GetUTXOSet() (map[types.Outpoint]*types.TxOutput, error) {
	return nil, fmt.Errorf("GetUTXOSet not implemented: use GetUTXO for individual lookups")
}

// Transaction operations
func (a *BlockchainAdapter) GetTransaction(hash types.Hash) (*types.Transaction, error) {
	return a.bc.GetTransaction(hash)
}

func (a *BlockchainAdapter) GetTransactionBlock(hash types.Hash) (*types.Block, error) {
	return a.bc.GetTransactionBlock(hash)
}

func (a *BlockchainAdapter) GetRawTransaction(hash types.Hash) ([]byte, error) {
	tx, err := a.bc.GetTransaction(hash)
	if err != nil {
		return nil, err
	}
	return tx.Serialize()
}

// Chain state
func (a *BlockchainAdapter) GetChainParams() *types.ChainParams {
	// This is not available in blockchain interface, return default
	return types.MainnetParams()
}

func (a *BlockchainAdapter) IsInitialBlockDownload() bool {
	// Not available in blockchain interface, return false
	return false
}

func (a *BlockchainAdapter) GetVerificationProgress() float64 {
	// Not available in blockchain interface, return 1.0
	return 1.0
}

func (a *BlockchainAdapter) GetMoneySupply(height uint32) (int64, error) {
	// Use type assertion to access GetMoneySupply from storage via blockchain
	if supplier, ok := a.bc.(interface {
		GetMoneySupply(height uint32) (int64, error)
	}); ok {
		return supplier.GetMoneySupply(height)
	}
	return 0, fmt.Errorf("GetMoneySupply not supported")
}

// InvalidateBlock marks a block as invalid
func (a *BlockchainAdapter) InvalidateBlock(hash types.Hash) error {
	if invalidator, ok := a.bc.(interface {
		InvalidateBlock(hash types.Hash) error
	}); ok {
		return invalidator.InvalidateBlock(hash)
	}
	return fmt.Errorf("InvalidateBlock not supported")
}

// ReconsiderBlock removes invalid status from a block
func (a *BlockchainAdapter) ReconsiderBlock(hash types.Hash) error {
	if reconsiderer, ok := a.bc.(interface {
		ReconsiderBlock(hash types.Hash) error
	}); ok {
		return reconsiderer.ReconsiderBlock(hash)
	}
	return fmt.Errorf("ReconsiderBlock not supported")
}

// AddCheckpoint adds a dynamic checkpoint
func (a *BlockchainAdapter) AddCheckpoint(height uint32, hash types.Hash) error {
	if checkpointer, ok := a.bc.(interface {
		AddCheckpoint(height uint32, hash types.Hash) error
	}); ok {
		return checkpointer.AddCheckpoint(height, hash)
	}
	return fmt.Errorf("AddCheckpoint not supported")
}
