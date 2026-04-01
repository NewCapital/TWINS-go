package consensus

import (
	"github.com/twins-dev/twins-core/pkg/types"
)

// BlockIndex represents a block in the blockchain index
// Used for tracking block metadata and chain state
type BlockIndex struct {
	// Block header fields
	Hash        types.Hash
	Version     int32
	Height      uint32
	Timestamp   uint32
	Bits        uint32
	Nonce       uint32
	MerkleRoot  types.Hash

	// Chain linkage
	PrevIndex *BlockIndex
	PrevHash  types.Hash

	// Validation status
	Status uint32

	// PoS specific fields
	IsProofOfStake bool
	StakeModifier  uint64
	HashProof      types.Hash // For PoS blocks

	// Statistics
	ChainWork    types.Hash // Total work in chain up to this block
	TxCount      uint32      // Number of transactions
	Size         uint32      // Block size in bytes
	TotalFees    int64       // Total fees in block
}

// BlockStatus flags
const (
	BlockStatusValidHeader     = 1 << 0  // Header validated
	BlockStatusValidTree       = 1 << 1  // All parents validated
	BlockStatusValidChain      = 1 << 2  // Added to main chain
	BlockStatusValidScripts    = 1 << 3  // Scripts/signatures validated
	BlockStatusHaveData        = 1 << 4  // Full block data available
	BlockStatusHaveUndo        = 1 << 5  // Undo data available
	BlockStatusFailed          = 1 << 6  // Block failed validation
	BlockStatusFailedChild     = 1 << 7  // Descendant of failed block
	BlockStatusOptInRBF        = 1 << 8  // Contains opt-in RBF transactions
)

// NewBlockIndex creates a new block index from a block
func NewBlockIndex(block *types.Block) *BlockIndex {
	index := &BlockIndex{
		Hash:           block.Hash(),
		Version:        int32(block.Header.Version),
		Height:         0, // Set by caller
		Timestamp:      block.Header.Timestamp,
		Bits:           block.Header.Bits,
		Nonce:          block.Header.Nonce,
		MerkleRoot:     block.Header.MerkleRoot,
		PrevHash:       block.Header.PrevBlockHash,
		IsProofOfStake: block.IsProofOfStake(),
		Status:         BlockStatusValidHeader,
		TxCount:        uint32(len(block.Transactions)),
	}

	// Calculate block size
	index.Size = uint32(block.SerializeSize())

	// For PoS blocks, extract proof hash
	if index.IsProofOfStake && len(block.Signature) > 0 {
		// The hash proof is typically embedded in the block signature
		// This would need proper implementation based on TWINS PoS specifics
		index.HashProof = types.NewHash(block.Signature)
	}

	return index
}

// GetAncestor returns the ancestor at the given height
func (bi *BlockIndex) GetAncestor(height uint32) *BlockIndex {
	if height > bi.Height {
		return nil
	}

	index := bi
	for index != nil && index.Height > height {
		index = index.PrevIndex
	}

	return index
}

// IsValid returns true if the block has been validated
func (bi *BlockIndex) IsValid() bool {
	return bi.Status&BlockStatusFailed == 0
}

// HaveData returns true if we have the full block data
func (bi *BlockIndex) HaveData() bool {
	return bi.Status&BlockStatusHaveData != 0
}

// IsInMainChain returns true if this block is in the main chain
func (bi *BlockIndex) IsInMainChain() bool {
	return bi.Status&BlockStatusValidChain != 0
}

// GetMedianTimePast returns the median timestamp of the last 11 blocks
func (bi *BlockIndex) GetMedianTimePast() uint32 {
	medianTimeSpan := 11
	timestamps := make([]uint32, 0, medianTimeSpan)

	index := bi
	for i := 0; i < medianTimeSpan && index != nil; i++ {
		timestamps = append(timestamps, index.Timestamp)
		index = index.PrevIndex
	}

	// If we don't have enough blocks, return the oldest we have
	if len(timestamps) == 0 {
		return 0
	}
	if len(timestamps) == 1 {
		return timestamps[0]
	}

	// Sort timestamps
	for i := 0; i < len(timestamps); i++ {
		for j := i + 1; j < len(timestamps); j++ {
			if timestamps[i] > timestamps[j] {
				timestamps[i], timestamps[j] = timestamps[j], timestamps[i]
			}
		}
	}

	// Return median
	return timestamps[len(timestamps)/2]
}