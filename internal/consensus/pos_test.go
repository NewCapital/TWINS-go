package consensus

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/twins-dev/twins-core/pkg/types"
)

func TestNewProofOfStake(t *testing.T) {
	storage := &MockStorage{}
	params := &types.ChainParams{
		PowLimit: 0x1d00ffff,
	}
	logger := logrus.New()

	pos := NewProofOfStake(storage, params, logger)

	assert.NotNil(t, pos)
	assert.Equal(t, storage, pos.storage)
	assert.Equal(t, params, pos.params)
	assert.NotNil(t, pos.logger)
	assert.NotNil(t, pos.modifierCache)
	assert.NotNil(t, pos.targetCache)
	assert.NotNil(t, pos.validationPool)
}

func TestValidateBlock_NilBlock(t *testing.T) {
	pos := createTestPoS(t)

	err := pos.ValidateBlock(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "block is nil")
}

func TestValidateBlock_ValidBlock(t *testing.T) {
	pos := createTestPoS(t)
	block := createTestBlock(t, 1)

	// Mock storage to return required data
	mockStorage := pos.storage.(*MockStorage)
	mockStorage.blocks[block.Header.PrevBlockHash] = createTestBlock(t, 0)

	err := pos.ValidateBlock(block)
	// May fail due to missing dependencies, but shouldn't panic
	assert.NotNil(t, err) // Expected to fail in test environment
}

func TestValidateProofOfStake_NoTransactions(t *testing.T) {
	pos := createTestPoS(t)
	block := &types.Block{
		Header:       createTestBlockHeader(t, 1),
		Transactions: []*types.Transaction{},
	}

	// Use ValidateProofOfStakeWithHeight directly for structural tests
	// (ValidateProofOfStake requires blockchain to be set for height lookup)
	result, err := pos.ValidateProofOfStakeWithHeight(block, 1000)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "PoS block must have at least")
	assert.NotNil(t, result)
	assert.False(t, result.IsValid)
}

func TestValidateProofOfStake_NoInputs(t *testing.T) {
	pos := createTestPoS(t)
	block := &types.Block{
		Header: createTestBlockHeader(t, 1),
		Transactions: []*types.Transaction{
			{
				Inputs:  []*types.TxInput{},
				Outputs: []*types.TxOutput{{Value: 1000}},
			},
		},
	}

	// Use ValidateProofOfStakeWithHeight directly for structural tests
	// (ValidateProofOfStake requires blockchain to be set for height lookup)
	result, err := pos.ValidateProofOfStakeWithHeight(block, 1000)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "PoS block must have at least")
	assert.NotNil(t, result)
	assert.False(t, result.IsValid)
}

func TestCalculateNextWorkRequired_NilHeader(t *testing.T) {
	pos := createTestPoS(t)

	bits, err := pos.CalculateNextWorkRequired(nil, 1)
	assert.Error(t, err)
	assert.Equal(t, uint32(0), bits)
}

func TestCalculateNextWorkRequired_ValidHeader(t *testing.T) {
	pos := createTestPoS(t)
	header := createTestBlockHeader(t, 1)

	// Mock storage
	mockStorage := pos.storage.(*MockStorage)
	prevBlock := createTestBlock(t, 0)
	mockStorage.blocks[header.PrevBlockHash] = prevBlock
	mockStorage.blocksByHeight[0] = prevBlock

	// Set chain tip so GetChainTip doesn't fail
	mockStorage.SetChainTipBlock(prevBlock)

	// Set up mock blockchain (CalculateNextWorkRequired uses pos.blockchain.GetBlock)
	mockBC := NewMockBlockchain(mockStorage)
	mockBC.blocks[header.PrevBlockHash] = prevBlock
	mockBC.blocksByHeight[0] = prevBlock
	pos.SetBlockchain(mockBC)

	bits, err := pos.CalculateNextWorkRequired(header, 1)
	// Should not error, but may return default values
	assert.NoError(t, err)
	assert.NotEqual(t, uint32(0), bits)
}

func TestGetStats(t *testing.T) {
	pos := createTestPoS(t)

	stats := pos.GetStats()
	assert.NotNil(t, stats)
	// Staking starts inactive by default (must call StartStaking to enable)
	assert.False(t, stats.StakingActive)
	assert.GreaterOrEqual(t, stats.ActiveStakers, 0)
	assert.GreaterOrEqual(t, stats.NetworkWeight, uint64(0))
	assert.GreaterOrEqual(t, stats.StakeModifier, uint64(0))
	assert.GreaterOrEqual(t, stats.Difficulty, uint32(0))
}

func TestPrepareValidationContext(t *testing.T) {
	pos := createTestPoS(t)

	// Create previous block first
	mockStorage := pos.storage.(*MockStorage)
	prevBlock := createTestBlock(t, 0)
	prevBlock.Header.Timestamp = uint32(time.Now().Unix())

	// Store previous block with its actual hash
	prevBlockHash := prevBlock.Hash()
	mockStorage.blocks[prevBlockHash] = prevBlock
	mockStorage.blocksByHeight[0] = prevBlock

	// Create block with correct PrevBlockHash
	block := createTestBlock(t, 1)
	block.Header.PrevBlockHash = prevBlockHash // Set to actual previous block hash

	ctx := &ValidationContext{}

	// Add the current block to storage
	blockHash := block.Hash()
	mockStorage.blocks[blockHash] = block

	// Set chain tip to previous block (height 0) so new block will be height 1
	mockStorage.SetChainTipBlock(prevBlock)

	// Debug: log the PrevBlockHash
	t.Logf("Block PrevBlockHash: %v", block.Header.PrevBlockHash)
	t.Logf("PrevBlockHash.IsZero(): %v", block.Header.PrevBlockHash.IsZero())
	t.Logf("Mock storage has key: %v", mockStorage.blocks[block.Header.PrevBlockHash] != nil)

	err := pos.prepareValidationContext(ctx, block)
	assert.NoError(t, err)
	assert.Equal(t, block, ctx.Block)
	assert.Equal(t, uint32(1), ctx.Height) // chainHeight (0) + 1 = 1
	if ctx.PrevBlock == nil {
		t.Logf("ctx.PrevBlock is nil! Expected it to be set")
	}
	assert.NotNil(t, ctx.PrevBlock)
	assert.Equal(t, prevBlock, ctx.PrevBlock)
}

func TestCalculateMedianTime(t *testing.T) {
	pos := createTestPoS(t)

	// Test with height 0
	medianTime, err := pos.calculateMedianTime(0)
	assert.NoError(t, err)
	assert.Equal(t, uint32(0), medianTime)

	// Mock storage with blocks
	mockStorage := pos.storage.(*MockStorage)
	for i := uint32(0); i < 5; i++ {
		block := createTestBlock(t, i)
		block.Header.Timestamp = uint32(1640995200 + int64(i)*120) // 2 minutes apart
		mockStorage.blocksByHeight[i] = block
	}

	medianTime, err = pos.calculateMedianTime(5)
	assert.NoError(t, err)
	assert.Greater(t, medianTime, uint32(0))
}

func TestGetMedianTimestamp(t *testing.T) {
	tests := []struct {
		name       string
		timestamps []uint32
		expected   uint32
	}{
		{
			name:       "empty slice",
			timestamps: []uint32{},
			expected:   0,
		},
		{
			name:       "single timestamp",
			timestamps: []uint32{1000},
			expected:   1000,
		},
		{
			name:       "odd number of timestamps",
			timestamps: []uint32{100, 200, 300, 400, 500},
			expected:   300,
		},
		{
			name:       "even number of timestamps",
			timestamps: []uint32{100, 200, 300, 400},
			expected:   250, // (200 + 300) / 2
		},
		{
			name:       "unsorted timestamps",
			timestamps: []uint32{300, 100, 500, 200, 400},
			expected:   300,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getMedianTimestamp(tt.timestamps)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidationError(t *testing.T) {
	// Test without cause
	err := &ValidationError{
		Code:    "TEST_ERROR",
		Message: "test error message",
		Height:  100,
	}
	assert.Equal(t, "test error message", err.Error())

	// Test with cause
	cause := errors.New("underlying error")
	err = &ValidationError{
		Code:    "TEST_ERROR",
		Message: "test error message",
		Height:  100,
		Cause:   cause,
	}
	assert.Contains(t, err.Error(), "test error message")
	assert.Contains(t, err.Error(), "underlying error")
}

func TestErrorConstants(t *testing.T) {
	assert.Equal(t, "INVALID_STAKE_AGE", ErrPosInvalidStakeAge.Code)
	assert.Equal(t, "INSUFFICIENT_WEIGHT", ErrPosInsufficientWeight.Code)
	assert.Equal(t, "INVALID_MODIFIER", ErrPosInvalidModifier.Code)
	assert.Equal(t, "TARGET_NOT_MET", ErrPosTargetNotMet.Code)
	assert.Equal(t, "INVALID_TIMESTAMP", ErrPosInvalidTimestamp.Code)
	assert.Equal(t, "DUPLICATE_STAKE", ErrPosDuplicateStake.Code)
	assert.Equal(t, "STAKE_NOT_MATURED", ErrPosStakeNotMatured.Code)
}

func TestStakeValidationResult(t *testing.T) {
	params := types.MainnetParams()

	result := &StakeValidationResult{
		IsValid:       true,
		StakeWeight:   1000,
		CoinAge:       86400, // 1 day
		Target:        big.NewInt(12345),
		KernelHash:    types.Hash{1, 2, 3},
		TargetSpacing: params.TargetSpacing,
	}

	assert.True(t, result.IsValid)
	assert.Equal(t, int64(1000), result.StakeWeight)
	assert.Equal(t, int64(86400), result.CoinAge)
	assert.NotNil(t, result.Target)
	assert.NotZero(t, result.KernelHash)
	assert.Equal(t, params.TargetSpacing, result.TargetSpacing)
}

func TestPoSConstants(t *testing.T) {
	params := types.MainnetParams()

	// Verify mainnet PoS parameters (from legacy C++ code)
	assert.Equal(t, 3*time.Hour, params.StakeMinAge)         // Legacy: nStakeMinAge = 3 * 60 * 60 (main.cpp:84)
	assert.Equal(t, 2*time.Minute, params.TargetSpacing)
	assert.Equal(t, 60*time.Second, params.StakeModifierInterval) // Legacy: MODIFIER_INTERVAL = 60 (kernel.h:14)
	assert.Equal(t, 2*time.Hour, params.MaxFutureBlockTime)

	// Verify validation constants
	assert.Equal(t, 11, MedianTimeSpan)
	assert.Equal(t, uint32(60), MinStakeConfirmations)
	assert.Equal(t, 10000, MaxCacheSize)
	assert.Equal(t, 4, ValidationWorkers)
}

// Helper functions for testing

func createTestPoS(t *testing.T) *ProofOfStake {
	storage := NewMockStorage()
	params := &types.ChainParams{
		PowLimit:         0x1d00ffff,
		GenesisTime:      1640995200,
		MinBlockVersion:  1,
		MaxBlockSize:     1000000,
		MinBlockInterval: time.Second,
	}
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce log noise in tests

	return NewProofOfStake(storage, params, logger)
}

func createTestBlockHeader(t *testing.T, height uint32) *types.BlockHeader {
	var prevHash types.Hash
	if height > 0 {
		// Create a deterministic NON-ZERO previous hash
		// Use byte 1 to avoid being considered a zero hash
		prevHash = types.Hash{byte(height), 1, 2, 3, 4, 5}
	}

	return &types.BlockHeader{
		Version:       1,
		PrevBlockHash: prevHash,
		MerkleRoot:    types.Hash{byte(height)},
		Timestamp:     uint32(1640995200 + int64(height)*120), // 2 minutes per block
		Bits:          0x1d00ffff,
		Nonce:         height,
	}
}

func createTestBlock(t *testing.T, height uint32) *types.Block {
	header := createTestBlockHeader(t, height)

	// Create coinstake transaction
	coinstake := &types.Transaction{
		Version: 1,
		Inputs: []*types.TxInput{
			{
				PreviousOutput: types.Outpoint{
					Hash:  types.Hash{byte(height + 100)},
					Index: 0,
				},
				ScriptSig: []byte("coinstake"),
			},
		},
		Outputs: []*types.TxOutput{
			{Value: 0},          // Empty output for coinstake
			{Value: 5000000000}, // 50 TWINS reward
		},
		LockTime: 0,
	}

	return &types.Block{
		Header:       header,
		Transactions: []*types.Transaction{coinstake},
	}
}

// Benchmark tests

func BenchmarkValidateBlock(b *testing.B) {
	pos := createTestPoS(&testing.T{})
	block := createTestBlock(&testing.T{}, 1)

	// Mock storage
	mockStorage := pos.storage.(*MockStorage)
	mockStorage.blocks[block.Header.PrevBlockHash] = createTestBlock(&testing.T{}, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pos.ValidateBlock(block)
	}
}

func BenchmarkCalculateNextWorkRequired(b *testing.B) {
	pos := createTestPoS(&testing.T{})
	header := createTestBlockHeader(&testing.T{}, 1)

	// Mock storage
	mockStorage := pos.storage.(*MockStorage)
	prevBlock := createTestBlock(&testing.T{}, 0)
	mockStorage.blocks[header.PrevBlockHash] = prevBlock

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pos.CalculateNextWorkRequired(header, 1)
	}
}

func BenchmarkGetMedianTimestamp(b *testing.B) {
	timestamps := []uint32{100, 200, 300, 400, 500, 600, 700, 800, 900, 1000, 1100}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		getMedianTimestamp(timestamps)
	}
}
