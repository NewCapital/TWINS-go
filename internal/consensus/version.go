package consensus

import (
	"fmt"

	"github.com/twins-dev/twins-core/pkg/types"
)

// Block version constants
const (
	// Block versions
	BlockVersionPoW    = 3  // Maximum PoW block version
	BlockVersionPoSv1  = 4  // First PoS block version
	BlockVersionCurrent = 5 // Current block version (supports CLTV)

	// BIP enforcement heights (mainnet)
	BIP66HeightMainnet = 891730 // Height after which strict DER signatures are required

	// Version enforcement majority
	RejectBlockOutdatedMajority = 950 // Reject blocks if 950 of last 1000 are newer version
	EnforceBlockUpgradeMajority = 750 // Enforce rules if 750 of last 1000 are new version
)

// GetRequiredBlockVersion returns the minimum required block version at a given height
func GetRequiredBlockVersion(height uint32, params *types.ChainParams) int32 {
	// For PoS blocks, require at least version 4
	// Version 5 adds CLTV support but isn't strictly required
	return BlockVersionPoSv1
}

// ValidateBlockVersion checks if a block version is acceptable at a given height
func ValidateBlockVersion(block *types.Block, prevIndex *BlockIndex, params *types.ChainParams) error {
	// Check if this is a PoS block
	if block.IsProofOfStake() {
		// PoS blocks must be at least version 4
		if block.Header.Version < BlockVersionPoSv1 {
			return &ValidationError{
				Code:    "BAD_VERSION",
				Message: fmt.Sprintf("proof-of-stake block version too old: got %d, minimum %d",
					block.Header.Version, BlockVersionPoSv1),
				Hash: block.Hash(),
			}
		}
	} else {
		// PoW blocks can be version 1-3
		// But reject old versions when supermajority has upgraded
		if prevIndex != nil {
			// Check version 1 blocks
			if block.Header.Version < 2 && IsSuperMajority(2, prevIndex, RejectBlockOutdatedMajority) {
				return &ValidationError{
					Code:    "OBSOLETE",
					Message: "rejected version 1 block",
					Hash:    block.Hash(),
				}
			}

			// Check version 2 blocks
			if block.Header.Version < 3 && IsSuperMajority(3, prevIndex, RejectBlockOutdatedMajority) {
				return &ValidationError{
					Code:    "OBSOLETE",
					Message: "rejected version 2 block",
					Hash:    block.Hash(),
				}
			}

			// Check version 4 blocks (should be 5+ for new blocks)
			if block.Header.Version < 5 && IsSuperMajority(5, prevIndex, RejectBlockOutdatedMajority) {
				return &ValidationError{
					Code:    "OBSOLETE",
					Message: "rejected version 4 block",
					Hash:    block.Hash(),
				}
			}
		}
	}

	return nil
}

// IsSuperMajority determines if a super-majority of blocks have a minimum version
// Checks if nRequired out of the last 1000 blocks have version >= minVersion
func IsSuperMajority(minVersion int32, startIndex *BlockIndex, nRequired int) bool {
	if startIndex == nil {
		return false
	}

	// Count blocks with version >= minVersion in the last 1000 blocks
	nFound := 0
	nToCheck := 1000
	index := startIndex

	for i := 0; i < nToCheck && index != nil; i++ {
		if index.Version >= minVersion {
			nFound++
		}
		index = index.PrevIndex
	}

	return nFound >= nRequired
}

// ShouldEnforceBIP66 returns true if BIP66 (strict DER signatures) should be enforced at this height
func ShouldEnforceBIP66(height uint32, params *types.ChainParams) bool {
	if params.Name == "mainnet" {
		return height >= BIP66HeightMainnet
	}
	// For testnet/regtest, always enforce
	return true
}

// ValidateBlockVersionContext performs version-specific contextual validation
func ValidateBlockVersionContext(block *types.Block, prevIndex *BlockIndex, params *types.ChainParams) error {
	height := uint32(0)
	if prevIndex != nil {
		height = prevIndex.Height + 1
	}

	// Version 2+ blocks must have height in coinbase
	if block.Header.Version >= 2 && prevIndex != nil {
		// Check if supermajority enforces this rule
		if IsSuperMajority(2, prevIndex, EnforceBlockUpgradeMajority) {
			// Coinbase scriptSig must start with serialized height
			if len(block.Transactions) == 0 {
				return &ValidationError{
					Code:    "BAD_COINBASE",
					Message: "block has no transactions",
					Hash:    block.Hash(),
					Height:  height,
				}
			}

			coinbase := block.Transactions[0]
			if !coinbase.IsCoinbase() {
				return &ValidationError{
					Code:    "BAD_COINBASE",
					Message: "first transaction is not coinbase",
					Hash:    block.Hash(),
					Height:  height,
				}
			}

			// Check that coinbase starts with height
			// This is serialized as a compact int in the scriptSig
			if len(coinbase.Inputs[0].ScriptSig) < 1 {
				return &ValidationError{
					Code:    "BAD_COINBASE",
					Message: "coinbase scriptSig too short for height",
					Hash:    block.Hash(),
					Height:  height,
				}
			}

			// For now, we'll do a simple check that scriptSig is not empty
			// Full height serialization validation would require script parsing
			// which can be added later when script package is implemented
		}
	}

	return nil
}