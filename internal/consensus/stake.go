package consensus

import (
	"math/big"
	"time"

	"github.com/twins-dev/twins-core/pkg/types"
)

// StakeInput represents a transaction output used for staking
type StakeInput struct {
	TxHash      types.Hash // Hash of the transaction containing the output
	Index       uint32     // Index of the output in the transaction
	Value       int64      // Value of the output in satoshis
	Age         int64      // Age of the output in seconds
	BlockHeight uint32     // Height of the block containing the transaction
	BlockTime   uint32     // Timestamp of the block containing the transaction
}

// StakeModifier represents a stake modifier used in PoS calculations
type StakeModifier struct {
	Modifier     uint64     // The actual modifier value
	Height       uint32     // Block height where this modifier was calculated
	Time         uint32     // Timestamp when this modifier was calculated
	PrevModifier uint64     // Previous modifier for validation
	BlockHash    types.Hash // Hash of the block this modifier belongs to
}

// StakeParams contains parameters for stake calculations
type StakeParams struct {
	MinStakeAge      time.Duration // Minimum age for stake to be valid
	TargetSpacing    time.Duration // Target time between blocks
	CoinbaseMaturity uint32        // Minimum confirmations for coinbase outputs
	// NOTE: MaxStakeAge removed - C++ has no maximum stake age limit (kernel.cpp)
}

// NewStakeInput creates a new stake input from transaction output information
func NewStakeInput(outpoint types.Outpoint, output *types.TxOutput, blockHeight uint32, blockTime uint32) *StakeInput {
	return &StakeInput{
		TxHash:      outpoint.Hash,
		Index:       outpoint.Index,
		Value:       output.Value,
		Age:         0, // Will be calculated dynamically
		BlockHeight: blockHeight,
		BlockTime:   blockTime,
	}
}

// GetWeight calculates the stake weight based on value only
// Matches legacy kernel.cpp: bnCoinDayWeight = uint256(nValueIn) / 100
// The params and currentTime are kept for interface compatibility but unused
func (si *StakeInput) GetWeight(params *types.ChainParams, currentTime uint32) int64 {
	if si.Value <= 0 {
		return 0
	}

	// Legacy formula: nValueIn / 100
	// This is used directly in StakeTargetHit comparison
	return si.Value / 100
}

// GetCoinAge calculates the coin age in seconds for a given current time
func (si *StakeInput) GetCoinAge(currentTime uint32) int64 {
	if currentTime <= si.BlockTime {
		return 0
	}
	return int64(currentTime - si.BlockTime)
}

// IsMatured checks if the stake input has sufficient confirmations
func (si *StakeInput) IsMatured(currentHeight, minConfirmations uint32) bool {
	if currentHeight < si.BlockHeight {
		return false
	}

	confirmations := currentHeight - si.BlockHeight
	return confirmations >= minConfirmations
}

// IsStakeAge checks if the stake input meets minimum age requirement
func (si *StakeInput) IsStakeAge(currentTime uint32, minAge time.Duration) bool {
	coinAge := si.GetCoinAge(currentTime)
	return coinAge >= int64(minAge.Seconds())
}

// GetOutpoint returns the outpoint for this stake input
func (si *StakeInput) GetOutpoint() types.Outpoint {
	return types.Outpoint{
		Hash:  si.TxHash,
		Index: si.Index,
	}
}

// CalculateStakeTarget calculates the PoS target based on stake weight
// Target = MaxTarget / StakeWeight
func CalculateStakeTarget(weight int64, targetSpacing time.Duration) *big.Int {
	if weight <= 0 {
		// Return maximum possible target (impossible to meet)
		return new(big.Int).SetBytes([]byte{
			0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		})
	}

	// Maximum target for PoS (easier than PoW)
	maxTarget := new(big.Int).SetBytes([]byte{
		0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	})

	// Apply weight scaling
	weightBig := big.NewInt(weight)
	target := new(big.Int).Div(maxTarget, weightBig)

	// Ensure minimum difficulty
	minTarget := new(big.Int).SetBytes([]byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	})

	if target.Cmp(minTarget) > 0 {
		return minTarget
	}

	return target
}

// ValidateStakeAmount checks if a stake amount meets minimum requirements
func ValidateStakeAmount(amount int64, minStake int64) bool {
	return amount >= minStake && amount > 0
}

// CalculateStakeReward calculates the PoS staking reward
// This is a simplified version - full implementation would consider
// network parameters, stake duration, and other factors
func CalculateStakeReward(stakeAmount int64, coinAge int64, annualRate float64) int64 {
	if stakeAmount <= 0 || coinAge <= 0 || annualRate <= 0 {
		return 0
	}

	// Annual rate as basis points (e.g., 5% = 500)
	rateBasisPoints := int64(annualRate * 100)

	// Calculate reward based on amount, age, and rate
	// Reward = (StakeAmount * CoinAge * AnnualRate) / (365 * 24 * 3600 * 10000)
	secondsInYear := int64(365 * 24 * 3600)

	reward := (stakeAmount * coinAge * rateBasisPoints) / (secondsInYear * 10000)

	// Apply minimum and maximum reward limits
	const (
		MinStakeReward = 1         // Minimum 1 satoshi
		MaxStakeReward = 100000000 // Maximum 1 TWINS
	)

	if reward < MinStakeReward {
		return MinStakeReward
	}
	if reward > MaxStakeReward {
		return MaxStakeReward
	}

	return reward
}

// GetStakeParams returns the stake parameters for TWINS PoS
func GetStakeParams(params *types.ChainParams) *StakeParams {
	return &StakeParams{
		MinStakeAge:      params.StakeMinAge,
		TargetSpacing:    params.TargetSpacing,
		CoinbaseMaturity: MinStakeConfirmations,
		// NOTE: MaxStakeAge removed - C++ has no maximum stake age limit
	}
}

// StakeInputSorter provides sorting functionality for stake inputs
type StakeInputSorter []*StakeInput

func (s StakeInputSorter) Len() int {
	return len(s)
}

func (s StakeInputSorter) Less(i, j int) bool {
	// Sort by weight (descending), then by age (descending)
	// Note: This sorter is deprecated - use sortStakeInputs function instead
	// For compatibility, use BlockTime as currentTime (stored in StakeInput)
	params := types.MainnetParams() // Temporary workaround
	weightI := s[i].GetWeight(params, s[i].BlockTime)
	weightJ := s[j].GetWeight(params, s[j].BlockTime)

	if weightI != weightJ {
		return weightI > weightJ
	}

	return s[i].Age > s[j].Age
}

func (s StakeInputSorter) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// SelectBestStakeInputs selects the best stake inputs for staking
// based on weight and age criteria
func SelectBestStakeInputs(inputs []*StakeInput, maxInputs int) []*StakeInput {
	if len(inputs) == 0 {
		return nil
	}

	// Create a copy to avoid modifying original slice
	sortedInputs := make([]*StakeInput, len(inputs))
	copy(sortedInputs, inputs)

	// Sort by weight and age
	sorter := StakeInputSorter(sortedInputs)

	// Simple selection sort for small arrays (more efficient for small datasets)
	n := len(sorter)
	for i := 0; i < n-1; i++ {
		maxIdx := i
		for j := i + 1; j < n; j++ {
			if sorter.Less(j, maxIdx) {
				maxIdx = j
			}
		}
		if maxIdx != i {
			sorter.Swap(i, maxIdx)
		}
	}

	// Return up to maxInputs best inputs
	if maxInputs > 0 && maxInputs < len(sortedInputs) {
		return sortedInputs[:maxInputs]
	}

	return sortedInputs
}

// CalculateNetworkStakeWeight estimates the total network stake weight
// This is a simplified calculation - full implementation would analyze
// recent staking activity across the network
func CalculateNetworkStakeWeight(recentBlocks []*types.Block, params *types.ChainParams) int64 {
	if len(recentBlocks) == 0 {
		return 0
	}

	totalWeight := int64(0)
	validBlocks := 0

	for _, block := range recentBlocks {
		if len(block.Transactions) < 2 {
			continue
		}

		coinstake := block.Transactions[1]
		if len(coinstake.Inputs) < 1 {
			continue
		}

		// Estimate weight from coinstake (simplified)
		stakeValue := int64(0)
		for _, output := range coinstake.Outputs {
			stakeValue += output.Value
		}

		if stakeValue > 0 {
			// Legacy weight formula: nValueIn / 100
			estimatedWeight := stakeValue / 100
			totalWeight += estimatedWeight
			validBlocks++
		}
	}

	if validBlocks == 0 {
		return 0
	}

	// Return average weight scaled by expected network size
	return totalWeight / int64(validBlocks)
}

// IsValidStakeTime checks if the block timestamp is valid for staking
func IsValidStakeTime(blockTime uint32, prevBlockTime uint32, medianTime uint32) bool {
	// Block must be after previous block
	if blockTime <= prevBlockTime {
		return false
	}

	// Block must be after median time of last 11 blocks
	if blockTime <= medianTime {
		return false
	}

	// NOTE: MaxFutureBlockTime check removed - should be passed as param
	// Block cannot be too far in the future (2 hours by default)
	// This will be validated at a higher level with chain params

	return true
}
