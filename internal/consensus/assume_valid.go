package consensus

import (
	"fmt"
	"math/big"

	"github.com/twins-dev/twins-core/pkg/types"
)

// HashToBig converts a hash to a big.Int for comparison with difficulty target
func HashToBig(hash types.Hash) *big.Int {
	// Hash bytes are in little-endian, but big.Int expects big-endian
	bytes := hash.Bytes()
	// Reverse bytes for big-endian
	for i, j := 0, len(bytes)-1; i < j; i, j = i+1, j-1 {
		bytes[i], bytes[j] = bytes[j], bytes[i]
	}
	return new(big.Int).SetBytes(bytes)
}

// ShouldAssumeValid returns true if we should skip expensive validation for this block
// during initial block download. This significantly speeds up initial sync.
//
// DYNAMIC VALIDATION: The actual validation decision is made in blockchain.go
// based on block depth from current tip (last 50 blocks = full validation).
// This function is kept for backward compatibility but always returns true during IBD.
func ShouldAssumeValid(params *types.ChainParams, height uint32, hash types.Hash) bool {
	// Always allow minimal validation during IBD
	// Blockchain layer decides which blocks to fully validate based on depth
	return true
}

// ValidateMinimal performs only essential validation during initial sync
// This includes:
// - Block structure validity
// - Proof of Work verification
// - Timestamp sanity checks
// - Parent block exists
// - Checkpoint verification (if at checkpoint height)
func ValidateMinimal(block *types.Block, prevHeader *types.BlockHeader, height uint32, params *types.ChainParams) error {
	if block == nil || block.Header == nil {
		return fmt.Errorf("invalid block structure")
	}

	header := block.Header

	// 1. Verify parent exists (chain continuity)
	if height > 0 {
		if prevHeader == nil {
			return fmt.Errorf("missing parent block")
		}

		// Check parent hash matches
		var expectedPrevHash types.Hash
		if height == 1 {
			expectedPrevHash = params.GenesisHash
		} else {
			expectedPrevHash = prevHeader.Hash()
		}

		if header.PrevBlockHash != expectedPrevHash {
			return fmt.Errorf("parent hash mismatch: got %s, expected %s",
				header.PrevBlockHash.String(), expectedPrevHash.String())
		}
	}

	// 2. Verify Proof of Work meets minimum difficulty
	target := GetTargetFromBits(header.Bits)
	maxTarget := params.PowLimitBig
	if target.Cmp(maxTarget) > 0 {
		return fmt.Errorf("target exceeds maximum allowed")
	}

	// For PoW blocks, verify the hash meets the target
	if height <= params.LastPOWBlock {
		blockHash := block.Hash()
		hashBig := HashToBig(blockHash)
		if hashBig.Cmp(target) > 0 {
			return fmt.Errorf("block hash doesn't meet target difficulty")
		}
	}

	// 3. Timestamp sanity checks
	// Must not be too far in the future
	now := GetAdjustedTime()
	maxFutureTime := now + 7200 // 2 hours for PoW blocks
	if block.IsProofOfStake() {
		maxFutureTime = now + 180 // 3 minutes for PoS blocks
	}

	if header.Timestamp > maxFutureTime {
		return fmt.Errorf("block timestamp too far in future: %d > %d",
			header.Timestamp, maxFutureTime)
	}

	// During initial sync, be very lenient with timestamps
	// Legacy chain has some blocks with weird timestamps due to time drift
	if height > 0 && prevHeader != nil {
		// Only reject if timestamp is way too far back (more than 2 hours)
		maxDrift := uint32(7200) // 2 hours
		if header.Timestamp+maxDrift < prevHeader.Timestamp {
			return fmt.Errorf("block timestamp too far before parent: %d < %d - %d",
				header.Timestamp, prevHeader.Timestamp, maxDrift)
		}
	}

	// 4. Checkpoint validation is now handled by blockchain layer
	// The blockchain.ProcessBlock() method validates against checkpoints

	// 5. Basic transaction structure validation
	if len(block.Transactions) == 0 {
		return fmt.Errorf("block has no transactions")
	}

	// First transaction must be coinbase (or coinstake for PoS)
	firstTx := block.Transactions[0]
	if height > params.LastPOWBlock {
		// PoS block - must have coinstake as second transaction
		if len(block.Transactions) < 2 {
			return fmt.Errorf("PoS block missing coinstake transaction")
		}
		if !block.Transactions[1].IsCoinStake() {
			return fmt.Errorf("second transaction is not coinstake in PoS block")
		}
	}

	if !firstTx.IsCoinbase() {
		return fmt.Errorf("first transaction is not coinbase")
	}

	// 6. Merkle root validation (cheap, ensures transaction integrity)
	calculatedMerkleRoot := types.CalculateMerkleRoot(block.Transactions)
	if header.MerkleRoot != calculatedMerkleRoot {
		return fmt.Errorf("merkle root mismatch: got %s, calculated %s",
			header.MerkleRoot.String(), calculatedMerkleRoot.String())
	}

	// Skip expensive validations:
	// - Script signature verification
	// - Full UTXO validation
	// - Stake kernel verification (for PoS)
	// - Transaction fee validation
	// - Masternode payment validation
	// - Coinbase maturity validation

	return nil
}

// GetValidationLevel returns whether to use minimal or full validation
func GetValidationLevel(bc interface{ IsInitialBlockDownload() bool }, params *types.ChainParams, height uint32, hash types.Hash) string {
	// During initial block download
	if bc.IsInitialBlockDownload() {
		// Use assume-valid if configured
		if ShouldAssumeValid(params, height, hash) {
			return "minimal"
		}
	}

	// After IBD or for blocks after assume-valid height
	return "full"
}