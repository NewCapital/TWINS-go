package consensus

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/twins-dev/twins-core/pkg/types"
)

func TestNewStakeInput(t *testing.T) {
	outpoint := types.Outpoint{
		Hash:  types.Hash{1, 2, 3},
		Index: 42,
	}
	output := &types.TxOutput{
		Value:        5000000000, // 50 TWINS
		ScriptPubKey: []byte("test script"),
	}
	blockHeight := uint32(1000)
	blockTime := uint32(1640995200)

	stakeInput := NewStakeInput(outpoint, output, blockHeight, blockTime)

	assert.NotNil(t, stakeInput)
	assert.Equal(t, outpoint.Hash, stakeInput.TxHash)
	assert.Equal(t, outpoint.Index, stakeInput.Index)
	assert.Equal(t, output.Value, stakeInput.Value)
	assert.Equal(t, blockHeight, stakeInput.BlockHeight)
	assert.Equal(t, blockTime, stakeInput.BlockTime)
	assert.Equal(t, int64(0), stakeInput.Age)
}

func TestStakeInput_GetCoinAge(t *testing.T) {
	stakeInput := &StakeInput{
		BlockTime: 1640995200, // Base time
	}

	tests := []struct {
		name        string
		currentTime uint32
		expectedAge int64
	}{
		{
			name:        "same time",
			currentTime: 1640995200,
			expectedAge: 0,
		},
		{
			name:        "1 hour later",
			currentTime: 1640995200 + 3600,
			expectedAge: 3600,
		},
		{
			name:        "1 day later",
			currentTime: 1640995200 + 86400,
			expectedAge: 86400,
		},
		{
			name:        "earlier time",
			currentTime: 1640995200 - 1000,
			expectedAge: 0, // Should not be negative
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			age := stakeInput.GetCoinAge(tt.currentTime)
			assert.Equal(t, tt.expectedAge, age)
		})
	}
}

func TestStakeInput_GetWeight(t *testing.T) {
	baseTime := uint32(1640995200)

	tests := []struct {
		name           string
		value          int64
		blockTime      uint32
		currentTime    uint32
		expectedWeight int64
	}{
		{
			name:           "zero value",
			value:          0,
			blockTime:      baseTime,
			currentTime:    baseTime + 86400, // 1 day later
			expectedWeight: 0,
		},
		{
			name:           "negative value",
			value:          -1000,
			blockTime:      baseTime,
			currentTime:    baseTime + 86400,
			expectedWeight: 0,
		},
		{
			name:           "valid stake",
			value:          5000000000, // 50 TWINS
			blockTime:      baseTime,
			currentTime:    baseTime + 86400, // 1 day
			expectedWeight: 5000000000 / 100, // Legacy formula: nValueIn / 100
		},
		{
			name:           "max age cap",
			value:          1000000000, // 10 TWINS
			blockTime:      baseTime,
			currentTime:    baseTime + uint32((30 * 24 * time.Hour).Seconds()) + 86400, // Beyond max age (30 days)
			expectedWeight: 1000000000 / 100, // Legacy formula: nValueIn / 100 (age doesn't matter)
		},
	}

	params := types.MainnetParams()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stakeInput := &StakeInput{
				Value:     tt.value,
				BlockTime: tt.blockTime,
			}

			weight := stakeInput.GetWeight(params, tt.currentTime)

			// Check that weight matches expected value
			assert.Equal(t, tt.expectedWeight, weight)
		})
	}
}

func TestStakeInput_IsMatured(t *testing.T) {
	stakeInput := &StakeInput{
		BlockHeight: 1000,
	}

	tests := []struct {
		name             string
		currentHeight    uint32
		minConfirmations uint32
		expected         bool
	}{
		{
			name:             "not enough confirmations",
			currentHeight:    1050,
			minConfirmations: 60,
			expected:         false,
		},
		{
			name:             "exact confirmations",
			currentHeight:    1060,
			minConfirmations: 60,
			expected:         true,
		},
		{
			name:             "more than enough confirmations",
			currentHeight:    1100,
			minConfirmations: 60,
			expected:         true,
		},
		{
			name:             "current height less than stake height",
			currentHeight:    900,
			minConfirmations: 60,
			expected:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stakeInput.IsMatured(tt.currentHeight, tt.minConfirmations)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStakeInput_IsStakeAge(t *testing.T) {
	baseTime := uint32(1640995200)
	stakeInput := &StakeInput{
		BlockTime: baseTime,
	}

	tests := []struct {
		name        string
		currentTime uint32
		minAge      time.Duration
		expected    bool
	}{
		{
			name:        "not enough age",
			currentTime: baseTime + 3600, // 1 hour
			minAge:      8 * time.Hour,
			expected:    false,
		},
		{
			name:        "exact min age",
			currentTime: baseTime + uint32((8 * time.Hour).Seconds()),
			minAge:      8 * time.Hour,
			expected:    true,
		},
		{
			name:        "more than min age",
			currentTime: baseTime + uint32((24 * time.Hour).Seconds()),
			minAge:      8 * time.Hour,
			expected:    true,
		},
		{
			name:        "same time",
			currentTime: baseTime,
			minAge:      time.Hour,
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stakeInput.IsStakeAge(tt.currentTime, tt.minAge)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStakeInput_GetOutpoint(t *testing.T) {
	expectedHash := types.Hash{1, 2, 3, 4, 5}
	expectedIndex := uint32(42)

	stakeInput := &StakeInput{
		TxHash: expectedHash,
		Index:  expectedIndex,
	}

	outpoint := stakeInput.GetOutpoint()
	assert.Equal(t, expectedHash, outpoint.Hash)
	assert.Equal(t, expectedIndex, outpoint.Index)
}

func TestCalculateStakeTarget(t *testing.T) {
	params := types.MainnetParams()

	tests := []struct {
		name           string
		weight         int64
		targetSpacing  time.Duration
		expectedResult string // We'll check if it's reasonable rather than exact
	}{
		{
			name:           "zero weight",
			weight:         0,
			targetSpacing:  params.TargetSpacing,
			expectedResult: "max", // Should return maximum target
		},
		{
			name:           "negative weight",
			weight:         -100,
			targetSpacing:  params.TargetSpacing,
			expectedResult: "max",
		},
		{
			name:           "small weight",
			weight:         1000,
			targetSpacing:  params.TargetSpacing,
			expectedResult: "large", // Should be a large target
		},
		{
			name:           "large weight",
			weight:         1000000000, // 10 TWINS equivalent
			targetSpacing:  params.TargetSpacing,
			expectedResult: "small", // Should be a smaller target
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := CalculateStakeTarget(tt.weight, tt.targetSpacing)
			assert.NotNil(t, target)

			switch tt.expectedResult {
			case "max":
				// Should be maximum target for impossible to meet
				assert.True(t, target.BitLen() > 200) // Very large number
			case "large":
				assert.True(t, target.Sign() > 0)
				assert.True(t, target.BitLen() > 100)
			case "small":
				assert.True(t, target.Sign() > 0)
				// Should be smaller than large weight case
			}
		})
	}
}

func TestValidateStakeAmount(t *testing.T) {
	tests := []struct {
		name     string
		amount   int64
		minStake int64
		expected bool
	}{
		{
			name:     "valid amount",
			amount:   1000000000, // 10 TWINS
			minStake: 500000000,  // 5 TWINS
			expected: true,
		},
		{
			name:     "exact minimum",
			amount:   500000000,
			minStake: 500000000,
			expected: true,
		},
		{
			name:     "below minimum",
			amount:   400000000,
			minStake: 500000000,
			expected: false,
		},
		{
			name:     "zero amount",
			amount:   0,
			minStake: 500000000,
			expected: false,
		},
		{
			name:     "negative amount",
			amount:   -1000000000,
			minStake: 500000000,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateStakeAmount(tt.amount, tt.minStake)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateStakeReward(t *testing.T) {
	tests := []struct {
		name          string
		stakeAmount   int64
		coinAge       int64
		annualRate    float64
		expectedRange []int64 // [min, max] for reasonable range
	}{
		{
			name:          "zero inputs",
			stakeAmount:   0,
			coinAge:       86400,
			annualRate:    5.0,
			expectedRange: []int64{0, 0},
		},
		{
			name:          "normal stake",
			stakeAmount:   1000000000,            // 10 TWINS
			coinAge:       86400,                 // 1 day
			annualRate:    5.0,                   // 5% annual
			expectedRange: []int64{1, 100000000}, // Should be reasonable
		},
		{
			name:          "large stake",
			stakeAmount:   10000000000, // 100 TWINS
			coinAge:       86400 * 30,  // 30 days
			annualRate:    5.0,
			expectedRange: []int64{1, 100000000}, // Capped at max
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reward := CalculateStakeReward(tt.stakeAmount, tt.coinAge, tt.annualRate)
			assert.GreaterOrEqual(t, reward, tt.expectedRange[0])
			assert.LessOrEqual(t, reward, tt.expectedRange[1])
		})
	}
}

func TestGetStakeParams(t *testing.T) {
	params := GetStakeParams(types.MainnetParams())
	assert.NotNil(t, params)
	assert.Equal(t, 3*time.Hour, params.MinStakeAge) // Legacy: nStakeMinAge = 3 * 60 * 60
	// NOTE: MaxStakeAge removed - C++ has no maximum stake age limit (kernel.cpp)
	assert.Equal(t, 2*time.Minute, params.TargetSpacing)
	assert.Equal(t, MinStakeConfirmations, params.CoinbaseMaturity)
}

func TestSelectBestStakeInputs(t *testing.T) {
	// Create test stake inputs with different weights
	inputs := []*StakeInput{
		{Value: 1000000000, BlockTime: 1640995200, Age: 86400},           // 10 TWINS, 1 day
		{Value: 5000000000, BlockTime: 1640995200 - 172800, Age: 172800}, // 50 TWINS, 2 days
		{Value: 2000000000, BlockTime: 1640995200 - 86400, Age: 86400},   // 20 TWINS, 1 day
	}

	tests := []struct {
		name      string
		inputs    []*StakeInput
		maxInputs int
		expected  int // Expected number of inputs returned
	}{
		{
			name:      "empty inputs",
			inputs:    []*StakeInput{},
			maxInputs: 5,
			expected:  0,
		},
		{
			name:      "select all",
			inputs:    inputs,
			maxInputs: 0, // 0 means no limit
			expected:  3,
		},
		{
			name:      "select top 2",
			inputs:    inputs,
			maxInputs: 2,
			expected:  2,
		},
		{
			name:      "select more than available",
			inputs:    inputs,
			maxInputs: 10,
			expected:  3,
		},
	}

	params := types.MainnetParams()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SelectBestStakeInputs(tt.inputs, tt.maxInputs)
			assert.Len(t, result, tt.expected)

			// Check that result is sorted by weight (best first)
			// Note: SelectBestStakeInputs internally uses BlockTime as currentTime for sorting
			for i := 1; i < len(result); i++ {
				weightPrev := result[i-1].GetWeight(params, result[i-1].BlockTime)
				weightCurr := result[i].GetWeight(params, result[i].BlockTime)
				assert.GreaterOrEqual(t, weightPrev, weightCurr, "Inputs should be sorted by weight (descending)")
			}
		})
	}
}

func TestIsValidStakeTime(t *testing.T) {
	now := uint32(time.Now().Unix())
	baseTime := now - 300         // 5 minutes ago
	prevTime := baseTime - 120    // 2 minutes before baseTime
	medianTime := baseTime - 1200 // 20 minutes before baseTime

	tests := []struct {
		name        string
		blockTime   uint32
		prevTime    uint32
		medianTime  uint32
		expected    bool
		description string
	}{
		{
			name:        "valid time",
			blockTime:   baseTime,
			prevTime:    prevTime,
			medianTime:  medianTime,
			expected:    true,
			description: "normal valid time",
		},
		{
			name:        "same as previous",
			blockTime:   prevTime,
			prevTime:    prevTime,
			medianTime:  medianTime,
			expected:    false,
			description: "must be after previous block",
		},
		{
			name:        "before previous",
			blockTime:   prevTime - 60,
			prevTime:    prevTime,
			medianTime:  medianTime,
			expected:    false,
			description: "cannot be before previous block",
		},
		{
			name:        "same as median",
			blockTime:   medianTime,
			prevTime:    prevTime,
			medianTime:  medianTime,
			expected:    false,
			description: "must be after median time",
		},
		// Note: MaxFutureBlockTime validation removed from IsValidStakeTime()
		// This check is now performed at a higher level with chain params
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidStakeTime(tt.blockTime, tt.prevTime, tt.medianTime)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

func TestCalculateNetworkStakeWeight(t *testing.T) {
	params := types.MainnetParams()

	// Create test blocks
	blocks := []*types.Block{
		createTestBlockWithValue(t, 0, 1000000000), // 10 TWINS
		createTestBlockWithValue(t, 1, 2000000000), // 20 TWINS
		createTestBlockWithValue(t, 2, 5000000000), // 50 TWINS
	}

	tests := []struct {
		name     string
		blocks   []*types.Block
		expected string // "zero" or "positive"
	}{
		{
			name:     "empty blocks",
			blocks:   []*types.Block{},
			expected: "zero",
		},
		{
			name:     "valid blocks",
			blocks:   blocks,
			expected: "positive",
		},
		{
			name:     "blocks without transactions",
			blocks:   []*types.Block{{Header: &types.BlockHeader{}, Transactions: []*types.Transaction{}}},
			expected: "zero",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			weight := CalculateNetworkStakeWeight(tt.blocks, params)
			if tt.expected == "zero" {
				assert.Equal(t, int64(0), weight)
			} else {
				assert.Greater(t, weight, int64(0))
			}
		})
	}
}

// Helper function for testing
func createTestBlockWithValue(t *testing.T, height uint32, stakeValue int64) *types.Block {
	coinbase := &types.Transaction{
		Inputs:  []*types.TxInput{},
		Outputs: []*types.TxOutput{{Value: 0}},
	}

	coinstake := &types.Transaction{
		Inputs: []*types.TxInput{
			{PreviousOutput: types.Outpoint{Hash: types.Hash{byte(height)}, Index: 0}},
		},
		Outputs: []*types.TxOutput{
			{Value: 0},          // Empty output
			{Value: stakeValue}, // Stake value
		},
	}

	return &types.Block{
		Header: &types.BlockHeader{
			Timestamp: 1640995200 + uint32(height)*120,
		},
		Transactions: []*types.Transaction{coinbase, coinstake},
	}
}

// Benchmark tests for stake calculations
func BenchmarkStakeInput_GetWeight(b *testing.B) {
	params := types.MainnetParams()
	stakeInput := &StakeInput{
		Value:     5000000000,
		BlockTime: 1640995200,
	}
	currentTime := uint32(1640995200 + 86400) // Current time for weight calculation

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stakeInput.GetWeight(params, currentTime)
	}
}

func BenchmarkCalculateStakeTarget(b *testing.B) {
	params := types.MainnetParams()
	weight := int64(1000000)
	spacing := params.TargetSpacing

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalculateStakeTarget(weight, spacing)
	}
}

func BenchmarkSelectBestStakeInputs(b *testing.B) {
	inputs := make([]*StakeInput, 100)
	for i := 0; i < 100; i++ {
		inputs[i] = &StakeInput{
			Value:     int64(1000000000 + i*100000000),
			BlockTime: 1640995200 - uint32(i*3600),
			Age:       int64(i * 3600),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SelectBestStakeInputs(inputs, 10)
	}
}
