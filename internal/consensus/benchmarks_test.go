package consensus

import (
	"crypto/sha256"
	"math/big"
	"testing"
	"time"

	"github.com/twins-dev/twins-core/pkg/types"
)

// Benchmark tests specifically targeting Go 1.25 performance improvements

func BenchmarkStakeValidation(b *testing.B) {
	pos := createTestPoS(&testing.T{})
	block := createValidTestBlock(&testing.T{}, 1)

	// Setup mock storage
	mockStorage := pos.storage.(*MockStorage)
	setupMockStorageForBlock(mockStorage, block)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		result, _ := pos.ValidateProofOfStake(block)
		_ = result
	}
}

func BenchmarkStakeValidationParallel(b *testing.B) {
	pos := createTestPoS(&testing.T{})
	block := createValidTestBlock(&testing.T{}, 1)

	// Setup mock storage
	mockStorage := pos.storage.(*MockStorage)
	setupMockStorageForBlock(mockStorage, block)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			result, _ := pos.ValidateProofOfStake(block)
			_ = result
		}
	})
}

func BenchmarkModifierCalculation(b *testing.B) {
	storage := NewMockStorage()
	params := types.MainnetParams()
	cache := NewModifierCache(1000, nil, storage, params)
	header := createTestBlockHeader(&testing.T{}, 100)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _, _ = cache.ComputeNextStakeModifier(header, uint32(i))
	}
}

func BenchmarkModifierCalculationParallel(b *testing.B) {
	storage := NewMockStorage()
	params := types.MainnetParams()
	cache := NewModifierCache(1000, nil, storage, params)
	header := createTestBlockHeader(&testing.T{}, 100)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := uint32(0)
		for pb.Next() {
			_, _, _ = cache.ComputeNextStakeModifier(header, i)
			i++
		}
	})
}

func BenchmarkTargetCalculation(b *testing.B) {
	calculator := NewTargetCalculator(&types.ChainParams{}, NewTargetCache(1000), nil, nil)
	prevHeader := createTestBlockHeader(&testing.T{}, 99)
	currentHeader := createTestBlockHeader(&testing.T{}, 100)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = calculator.CalculateNextTarget(prevHeader, currentHeader, uint32(i))
	}
}

func BenchmarkTargetCalculationParallel(b *testing.B) {
	calculator := NewTargetCalculator(&types.ChainParams{}, NewTargetCache(1000), nil, nil)
	prevHeader := createTestBlockHeader(&testing.T{}, 99)
	currentHeader := createTestBlockHeader(&testing.T{}, 100)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		height := uint32(1)
		for pb.Next() {
			_, _ = calculator.CalculateNextTarget(prevHeader, currentHeader, height)
			height++
		}
	})
}

func BenchmarkKernelHash(b *testing.B) {
	stakeInput := &StakeInput{
		TxHash:    types.Hash{1, 2, 3, 4, 5},
		Index:     0,
		Value:     1000000000,
		BlockTime: 1640995200,
	}
	modifier := uint64(0x12345678)
	blockTime := uint32(1640995200 + 3600)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = ComputeStakeKernelHash(stakeInput, modifier, blockTime)
	}
}

func BenchmarkKernelHashParallel(b *testing.B) {
	stakeInput := &StakeInput{
		TxHash:    types.Hash{1, 2, 3, 4, 5},
		Index:     0,
		Value:     1000000000,
		BlockTime: 1640995200,
	}
	modifier := uint64(0x12345678)
	blockTime := uint32(1640995200 + 3600)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = ComputeStakeKernelHash(stakeInput, modifier, blockTime)
		}
	})
}

func BenchmarkKernelValidation(b *testing.B) {
	stakeInput := &StakeInput{
		TxHash:    types.Hash{1, 2, 3, 4, 5},
		Index:     0,
		Value:     1000000000,
		BlockTime: 1640995200,
	}
	modifier := uint64(0x12345678)
	blockTime := uint32(1640995200 + 3600)
	target := big.NewInt(1000000000000000)

	b.ResetTimer()
	b.ReportAllocs()

	params := &types.ChainParams{}
	for i := 0; i < b.N; i++ {
		_, _ = CheckStakeKernelHash(modifier, stakeInput, blockTime, target, params)
	}
}

func BenchmarkBatchKernelValidation(b *testing.B) {
	// Create multiple stake inputs for batch validation
	inputs := make([]*StakeInput, 10)
	for i := 0; i < 10; i++ {
		inputs[i] = &StakeInput{
			TxHash:    types.Hash{byte(i), 2, 3, 4, 5},
			Index:     uint32(i),
			Value:     1000000000 + int64(i)*100000000,
			BlockTime: 1640995200 - uint32(i)*3600,
		}
	}

	modifier := uint64(0x12345678)
	blockTime := uint32(1640995200 + 3600)
	target := big.NewInt(1000000000000000)

	b.ResetTimer()
	b.ReportAllocs()

	params := &types.ChainParams{}
	for i := 0; i < b.N; i++ {
		_, _ = BatchValidateKernels(inputs, modifier, blockTime, target, params)
	}
}

func BenchmarkCryptoOperations(b *testing.B) {
	// Benchmark Go 1.25 enhanced crypto performance
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		hasher := sha256.New()
		hasher.Write(data)
		_ = hasher.Sum(nil)
	}
}

func BenchmarkCryptoOperationsParallel(b *testing.B) {
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			hasher := sha256.New()
			hasher.Write(data)
			_ = hasher.Sum(nil)
		}
	})
}

func BenchmarkBigIntOperations(b *testing.B) {
	// Benchmark improved big integer arithmetic in Go 1.25
	x := new(big.Int).SetBytes([]byte{
		0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0,
		0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0,
		0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0,
		0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0,
	})
	y := new(big.Int).SetBytes([]byte{
		0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10,
		0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10,
		0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10,
		0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10,
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		result := new(big.Int)
		result.Mul(x, y)
		result.Div(result, y)
		_ = result
	}
}

func BenchmarkMemoryAllocation(b *testing.B) {
	// Test Go 1.25 memory allocation improvements
	b.ResetTimer()
	b.ReportAllocs()

	params := &types.ChainParams{}
	for i := 0; i < b.N; i++ {
		// Simulate stake validation memory pattern
		stakeInputs := make([]*StakeInput, 100)
		for j := 0; j < 100; j++ {
			stakeInputs[j] = &StakeInput{
				TxHash:    types.Hash{byte(j)},
				Index:     uint32(j),
				Value:     int64(j * 1000000),
				BlockTime: uint32(1640995200 + j*3600),
			}
		}

		// Process and discard (use current time as approximation for benchmark)
		currentTime := uint32(time.Now().Unix())
		for _, input := range stakeInputs {
			_ = input.GetWeight(params, currentTime)
		}
	}
}

func BenchmarkConcurrentValidation(b *testing.B) {
	// Benchmark concurrent block validation (Go 1.25 improvements)
	pos := createTestPoS(&testing.T{})
	blocks := make([]*types.Block, 10)

	for i := 0; i < 10; i++ {
		blocks[i] = createValidTestBlock(&testing.T{}, uint32(i+1))
	}

	// Setup mock storage
	mockStorage := pos.storage.(*MockStorage)
	for _, block := range blocks {
		setupMockStorageForBlock(mockStorage, block)
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			for _, block := range blocks {
				_, _ = pos.ValidateProofOfStake(block)
			}
		}
	})
}

func BenchmarkCachePerformance(b *testing.B) {
	// Test cache performance with Go 1.25 improvements
	storage := NewMockStorage()
	params := types.MainnetParams()
	modifierCache := NewModifierCache(10000, nil, storage, params)
	targetCache := NewTargetCache(10000)

	// Pre-populate caches
	for i := 0; i < 1000; i++ {
		hash := types.Hash{byte(i), byte(i >> 8)}
		modifier := &StakeModifier{
			Modifier:  uint64(i),
			Height:    uint32(i),
			BlockHash: hash,
		}

		modifierCache.cache[hash] = modifier
		targetCache.targets[hash] = big.NewInt(int64(i * 1000000))
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Simulate cache access pattern
			for i := 0; i < 100; i++ {
				hash := types.Hash{byte(i % 1000), byte((i % 1000) >> 8)}
				_, _ = modifierCache.GetStakeModifier(hash)

				// Simulate cache miss and computation
				if i%10 == 0 {
					_, _ = modifierCache.computeAndCacheModifier(types.Hash{byte(i + 1000)})
				}
			}
		}
	})
}

func BenchmarkCompleteStakeSearch(b *testing.B) {
	// Benchmark complete stake search operation
	stakeInput := &StakeInput{
		TxHash:    types.Hash{1, 2, 3, 4, 5},
		Index:     0,
		Value:     5000000000, // 50 TWINS
		BlockTime: 1640995200,
	}
	modifier := uint64(0x12345678)
	startTime := uint32(1640995200 + 8*3600) // 8 hours later (valid age)
	endTime := startTime + 3600              // 1 hour search window
	target := big.NewInt(100000000000000)    // Reasonable target

	b.ResetTimer()
	b.ReportAllocs()

	params := &types.ChainParams{}
	for i := 0; i < b.N; i++ {
		_, _, _ = FindValidStakeKernel(stakeInput, modifier, startTime, endTime, target, params)
	}
}

// Comparison benchmarks to show Go 1.25 improvements vs theoretical Go 1.24

func BenchmarkLegacyStyleOperations(b *testing.B) {
	// Simulate older style operations for comparison
	data := make([][]byte, 100)
	for i := range data {
		data[i] = make([]byte, 32)
		for j := range data[i] {
			data[i][j] = byte((i + j) % 256)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Simulate less efficient memory usage pattern
		results := make([]*big.Int, len(data))
		for j, d := range data {
			hasher := sha256.New()
			hasher.Write(d)
			hash := hasher.Sum(nil)
			results[j] = new(big.Int).SetBytes(hash)
		}

		// Force some GC pressure
		_ = results
	}
}

func BenchmarkOptimizedGo25Operations(b *testing.B) {
	// Optimized operations leveraging Go 1.25 improvements
	data := make([][]byte, 100)
	for i := range data {
		data[i] = make([]byte, 32)
		for j := range data[i] {
			data[i][j] = byte((i + j) % 256)
		}
	}

	// Pre-allocate to reduce GC pressure
	results := make([]*big.Int, 100)
	for i := range results {
		results[i] = new(big.Int)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// More efficient memory reuse pattern
		for j, d := range data {
			hasher := sha256.New()
			hasher.Write(d)
			hash := hasher.Sum(nil)
			results[j].SetBytes(hash)
		}
	}
}

// Helper functions for benchmarks

func createValidTestBlock(t *testing.T, height uint32) *types.Block {
	block := createTestBlock(t, height)

	// Make it more realistic for benchmarking
	for i := 0; i < 5; i++ {
		tx := &types.Transaction{
			Version: 1,
			Inputs: []*types.TxInput{
				{
					PreviousOutput: types.Outpoint{
						Hash:  types.Hash{byte(height), byte(i)},
						Index: uint32(i),
					},
					ScriptSig: make([]byte, 70), // Realistic script size
				},
			},
			Outputs: []*types.TxOutput{
				{
					Value:        int64((i + 1) * 100000000),
					ScriptPubKey: make([]byte, 25), // Realistic script size
				},
			},
		}
		block.Transactions = append(block.Transactions, tx)
	}

	return block
}

func setupMockStorageForBlock(storage *MockStorage, block *types.Block) {
	// Add the block
	storage.AddTestBlock(block)

	// Simplified test setup without height dependencies
	prevBlock := createTestBlock(&testing.T{}, 0)
	prevBlock.Header.Timestamp = block.Header.Timestamp - 120 // 2 minutes earlier
	storage.AddTestBlock(prevBlock)
	storage.blocksByHeight[0] = prevBlock
	storage.blocksByHeight[1] = block

	// Add transactions that the stake input might reference
	for _, tx := range block.Transactions {
		for _, _ = range tx.Inputs {
			// Create the previous transaction
			prevTx := &types.Transaction{
				Version: 1,
				Outputs: []*types.TxOutput{
					{Value: 1000000000, ScriptPubKey: []byte("prev output")},
				},
			}
			storage.AddTestTransaction(prevTx, block)
		}
	}
}
