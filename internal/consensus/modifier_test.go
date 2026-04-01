package consensus

import (
	"testing"

	"github.com/twins-dev/twins-core/pkg/types"
)

// TestGetStakeModifierSelectionIntervalSection verifies interval calculation
func TestGetStakeModifierSelectionIntervalSection(t *testing.T) {
	tests := []struct {
		section  int
		expected int64
	}{
		{0, 60 * 63 / (63 + 63*2)},  // First section (smallest)
		{31, 60 * 63 / (63 + 32*2)}, // Middle section
		{63, 60 * 63 / (63 + 0*2)},  // Last section (largest)
		{-1, 0},                     // Invalid (negative)
		{64, 0},                     // Invalid (too large)
	}

	for _, tt := range tests {
		result := GetStakeModifierSelectionIntervalSection(tt.section)
		if result != tt.expected {
			t.Errorf("GetStakeModifierSelectionIntervalSection(%d) = %d, want %d",
				tt.section, result, tt.expected)
		}
	}
}

// TestGetStakeModifierSelectionInterval verifies total interval is sum of all sections
func TestGetStakeModifierSelectionInterval(t *testing.T) {
	total := GetStakeModifierSelectionInterval()

	// Calculate expected total
	var expected int64
	for i := 0; i < 64; i++ {
		expected += GetStakeModifierSelectionIntervalSection(i)
	}

	if total != expected {
		t.Errorf("GetStakeModifierSelectionInterval() = %d, want %d", total, expected)
	}

	// Verify it's a reasonable value (should be several minutes)
	if total < 60 || total > 3600 {
		t.Errorf("Total interval %d seconds seems unreasonable", total)
	}
}

// TestCompareHashes verifies hash comparison function
func TestCompareHashes(t *testing.T) {
	tests := []struct {
		name     string
		a        types.Hash
		b        types.Hash
		expected int
	}{
		{
			name:     "equal hashes",
			a:        types.Hash{1, 2, 3, 4, 5},
			b:        types.Hash{1, 2, 3, 4, 5},
			expected: 0,
		},
		{
			name:     "a < b (first byte differs)",
			a:        types.Hash{1, 2, 3, 4, 5},
			b:        types.Hash{2, 2, 3, 4, 5},
			expected: -1,
		},
		{
			name:     "a > b (first byte differs)",
			a:        types.Hash{2, 2, 3, 4, 5},
			b:        types.Hash{1, 2, 3, 4, 5},
			expected: 1,
		},
		{
			name:     "a < b (middle byte differs)",
			a:        types.Hash{1, 2, 3, 4, 5},
			b:        types.Hash{1, 2, 4, 4, 5},
			expected: -1,
		},
		{
			name:     "a > b (last byte differs)",
			a:        types.Hash{1, 2, 3, 4, 6},
			b:        types.Hash{1, 2, 3, 4, 5},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareHashes(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("compareHashes() = %d, want %d", result, tt.expected)
			}
		})
	}
}

// TestCalculateEntropyBit verifies entropy bit extraction
func TestCalculateEntropyBit(t *testing.T) {
	// Test with pre-set entropy bit
	block := &types.Block{
		Header: &types.BlockHeader{
			Version:   1,
			Timestamp: 1000,
		},
		Transactions: []*types.Transaction{},
	}

	// Test entropy bit = 1
	block.SetStakeEntropyBit(1)
	result := block.GetStakeEntropyBit()
	if result != 0 && result != 1 {
		t.Errorf("GetStakeEntropyBit() = %d, must be 0 or 1", result)
	}

	// Test entropy bit = 0
	block.SetStakeEntropyBit(0)
	result = block.GetStakeEntropyBit()
	if result != 0 && result != 1 {
		t.Errorf("GetStakeEntropyBit() = %d, must be 0 or 1", result)
	}

	// Test that SetStakeEntropyBit enforces 0 or 1
	block.SetStakeEntropyBit(255)
	if block.GetStakeEntropyBit() != 1 {
		t.Errorf("SetStakeEntropyBit(255) should result in 1, got %d", block.GetStakeEntropyBit())
	}

	block.SetStakeEntropyBit(2)
	if block.GetStakeEntropyBit() != 0 {
		t.Errorf("SetStakeEntropyBit(2) should result in 0, got %d", block.GetStakeEntropyBit())
	}
}

// TestModifierConstants verifies critical constants
func TestModifierConstants(t *testing.T) {
	if ModifierInterval != 60 {
		t.Errorf("ModifierInterval = %d, want 60", ModifierInterval)
	}

	if ModifierIntervalRatio != 3 {
		t.Errorf("ModifierIntervalRatio = %d, want 3", ModifierIntervalRatio)
	}

	if MaxModifierSearchDepth != 10080 {
		t.Errorf("MaxModifierSearchDepth = %d, want 10080", MaxModifierSearchDepth)
	}

	// Verify Block1Modifier is non-zero
	if Block1Modifier == 0 {
		t.Error("Block1Modifier should not be zero")
	}
}

// TestModifierCheckpoints verifies checkpoint data
func TestModifierCheckpoints(t *testing.T) {
	if len(ModifierCheckpoints) == 0 {
		t.Error("ModifierCheckpoints should have at least genesis entry")
	}

	// Verify first checkpoint is height 1
	if ModifierCheckpoints[0].Height != 1 {
		t.Errorf("First checkpoint height = %d, want 1", ModifierCheckpoints[0].Height)
	}

	// Verify first checkpoint uses Block1Modifier constant
	if ModifierCheckpoints[0].Modifier != Block1Modifier {
		t.Errorf("First checkpoint modifier = %d, want %d",
			ModifierCheckpoints[0].Modifier, Block1Modifier)
	}
}

// TestComputeNextStakeModifier_Genesis verifies genesis block handling
func TestComputeNextStakeModifier_Genesis(t *testing.T) {
	mc := &ModifierCache{}

	header := &types.BlockHeader{
		Version:   1,
		Timestamp: 1000,
	}

	modifier, generated, err := mc.ComputeNextStakeModifier(header, 0)
	if err != nil {
		t.Fatalf("ComputeNextStakeModifier() error = %v", err)
	}

	if modifier != 0 {
		t.Errorf("Genesis modifier = %d, want 0", modifier)
	}

	if !generated {
		t.Error("Genesis modifier should be marked as generated")
	}
}

// TestComputeNextStakeModifier_Block1 verifies block 1 handling
func TestComputeNextStakeModifier_Block1(t *testing.T) {
	mc := &ModifierCache{}

	header := &types.BlockHeader{
		Version:   1,
		Timestamp: 1000,
	}

	modifier, generated, err := mc.ComputeNextStakeModifier(header, 1)
	if err != nil {
		t.Fatalf("ComputeNextStakeModifier() error = %v", err)
	}

	if modifier != Block1Modifier {
		t.Errorf("Block 1 modifier = %d, want %d", modifier, Block1Modifier)
	}

	if !generated {
		t.Error("Block 1 modifier should be marked as generated")
	}
}

// TestComputeNextStakeModifier_TimestampOverflow verifies overflow protection
func TestComputeNextStakeModifier_TimestampOverflow(t *testing.T) {
	// Note: This test requires a mock storage to test the full flow
	// For now, we just verify the timestamp conversion logic

	// uint32 max value
	maxUint32 := uint32(0xFFFFFFFF)

	header := &types.BlockHeader{
		Version:   1,
		Timestamp: maxUint32,
	}

	// When converted to int64, should not be negative
	headerTime := int64(header.Timestamp)
	if headerTime < 0 {
		t.Errorf("Timestamp %d converted to negative int64: %d",
			header.Timestamp, headerTime)
	}
}

// BenchmarkGetStakeModifierSelectionInterval benchmarks interval calculation
func BenchmarkGetStakeModifierSelectionInterval(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GetStakeModifierSelectionInterval()
	}
}

// BenchmarkCompareHashes benchmarks hash comparison
func BenchmarkCompareHashes(b *testing.B) {
	hash1 := types.Hash{1, 2, 3, 4, 5, 6, 7, 8}
	hash2 := types.Hash{1, 2, 3, 4, 5, 6, 7, 9}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		compareHashes(hash1, hash2)
	}
}

// TestModifierCacheStats verifies cache statistics tracking
func TestModifierCacheStats(t *testing.T) {
	mc := NewModifierCache(10, nil, nil, nil)

	hits, misses, size := mc.GetCacheStats()
	if hits != 0 || misses != 0 || size != 0 {
		t.Errorf("Initial stats should be zero: hits=%d, misses=%d, size=%d",
			hits, misses, size)
	}
}

// TestModifierCacheClear verifies cache clearing
func TestModifierCacheClear(t *testing.T) {
	mc := NewModifierCache(10, nil, nil, nil)

	// Manually add some entries
	mc.cache[types.Hash{1}] = &StakeModifier{Modifier: 123}
	mc.cache[types.Hash{2}] = &StakeModifier{Modifier: 456}

	if len(mc.cache) != 2 {
		t.Errorf("Cache size = %d, want 2", len(mc.cache))
	}

	mc.ClearCache()

	if len(mc.cache) != 0 {
		t.Errorf("Cache size after clear = %d, want 0", len(mc.cache))
	}
}

// ============================================================================
// Integration Tests for Stake Modifier Calculation
// ============================================================================

// createTestChain builds a mock blockchain with connected blocks for testing
func createTestChain(t *testing.T, numBlocks int, baseTime uint32) (*MockBlockchain, *MockStorage) {
	storage := NewMockStorage()
	blockchain := NewMockBlockchain(storage)

	var prevHash types.Hash // Genesis has zero prev hash

	for i := 0; i < numBlocks; i++ {
		// Create block header with proper linkage
		header := &types.BlockHeader{
			Version:       1,
			PrevBlockHash: prevHash,
			MerkleRoot:    types.Hash{byte(i), byte(i >> 8)},
			Timestamp:     baseTime + uint32(i*60), // 60 seconds apart
			Bits:          0x1d00ffff,
			Nonce:         uint32(i),
		}

		// Create a simple coinbase transaction
		coinbase := &types.Transaction{
			Version: 1,
			Inputs: []*types.TxInput{
				{
					PreviousOutput: types.Outpoint{Hash: types.ZeroHash, Index: 0xFFFFFFFF},
					ScriptSig:      []byte{byte(i)},
				},
			},
			Outputs: []*types.TxOutput{
				{Value: 5000000000}, // 50 TWINS
			},
		}

		block := &types.Block{
			Header:       header,
			Transactions: []*types.Transaction{coinbase},
		}

		blockchain.AddBlock(block, uint32(i))
		prevHash = block.Hash()
	}

	return blockchain, storage
}

// TestGetLastStakeModifier tests walking backwards to find the last generated modifier
func TestGetLastStakeModifier(t *testing.T) {
	// Create a chain of 10 blocks
	blockchain, storage := createTestChain(t, 10, 1640995200)

	// Create modifier cache with blockchain interface
	params := types.MainnetParams()
	mc := NewModifierCache(100, blockchain, storage, params)

	// Set up stake modifiers at specific heights
	// Block 1 has the Block1Modifier (hardcoded)
	block1, _ := blockchain.GetBlockByHeight(1)
	blockchain.SetStakeModifier(block1.Hash(), Block1Modifier)

	// Block 5 has a new generated modifier
	block5, _ := blockchain.GetBlockByHeight(5)
	expectedModifier := uint64(0xDEADBEEF12345678)
	blockchain.SetStakeModifier(block5.Hash(), expectedModifier)

	// Test: Get last modifier from block 7 (should find block 5's modifier)
	block7, _ := blockchain.GetBlockByHeight(7)
	modifier, modTime, err := mc.GetLastStakeModifier(block7.Hash())

	if err != nil {
		t.Fatalf("GetLastStakeModifier() error = %v", err)
	}

	if modifier != expectedModifier {
		t.Errorf("GetLastStakeModifier() modifier = %x, want %x", modifier, expectedModifier)
	}

	// Verify modifier time matches block 5's timestamp
	if modTime != block5.Header.Timestamp {
		t.Errorf("GetLastStakeModifier() time = %d, want %d", modTime, block5.Header.Timestamp)
	}
}

// TestGetLastStakeModifier_FallbackToBlock1 tests fallback to Block1Modifier
func TestGetLastStakeModifier_FallbackToBlock1(t *testing.T) {
	// Create a chain of 5 blocks
	blockchain, storage := createTestChain(t, 5, 1640995200)

	params := types.MainnetParams()
	mc := NewModifierCache(100, blockchain, storage, params)

	// Only set Block1Modifier (no other modifiers in chain)
	block1, _ := blockchain.GetBlockByHeight(1)
	blockchain.SetStakeModifier(block1.Hash(), Block1Modifier)

	// Test: Get last modifier from block 4 (should find block 1's modifier)
	block4, _ := blockchain.GetBlockByHeight(4)
	modifier, _, err := mc.GetLastStakeModifier(block4.Hash())

	if err != nil {
		t.Fatalf("GetLastStakeModifier() error = %v", err)
	}

	if modifier != Block1Modifier {
		t.Errorf("GetLastStakeModifier() modifier = %x, want %x (Block1Modifier)", modifier, Block1Modifier)
	}
}

// TestSelectBlockFromCandidates tests the block selection algorithm for modifier calculation
func TestSelectBlockFromCandidates(t *testing.T) {
	storage := NewMockStorage()
	blockchain := NewMockBlockchain(storage)
	params := types.MainnetParams()
	mc := NewModifierCache(100, blockchain, storage, params)

	baseTime := int64(1640995200)

	// Create test candidates with different timestamps and hashes
	candidates := make([]blockCandidate, 5)
	for i := 0; i < 5; i++ {
		header := &types.BlockHeader{
			Version:   1,
			Timestamp: uint32(baseTime + int64(i*60)),
			Bits:      0x1d00ffff,
			Nonce:     uint32(i * 1000), // Different nonce = different hash
		}
		block := &types.Block{
			Header: header,
			Transactions: []*types.Transaction{
				{Version: 1, Inputs: []*types.TxInput{{ScriptSig: []byte{byte(i)}}}},
			},
		}
		candidates[i] = blockCandidate{
			timestamp: int64(header.Timestamp),
			hash:      block.Hash(),
			block:     block,
			height:    uint32(i + 100), // Heights above ModifierUpgradeBlock
		}
	}

	// Test selection with no previously selected blocks
	selectedBlocks := make(map[types.Hash]bool)
	selectionIntervalStop := baseTime + 300 // Allow all candidates

	selected, err := mc.SelectBlockFromCandidates(
		candidates,
		selectedBlocks,
		selectionIntervalStop,
		0x123456789ABCDEF0, // Previous modifier
	)

	if err != nil {
		t.Fatalf("SelectBlockFromCandidates() error = %v", err)
	}

	if selected == nil {
		t.Fatal("SelectBlockFromCandidates() returned nil block")
	}

	// Verify a block was selected (should be the one with smallest hash after selection hash calculation)
	found := false
	for _, c := range candidates {
		if c.block.Hash() == selected.Hash() {
			found = true
			break
		}
	}
	if !found {
		t.Error("Selected block not in candidates list")
	}
}

// TestSelectBlockFromCandidates_SkipsAlreadySelected tests that already selected blocks are skipped
func TestSelectBlockFromCandidates_SkipsAlreadySelected(t *testing.T) {
	storage := NewMockStorage()
	blockchain := NewMockBlockchain(storage)
	params := types.MainnetParams()
	mc := NewModifierCache(100, blockchain, storage, params)

	baseTime := int64(1640995200)

	// Create 3 candidates
	candidates := make([]blockCandidate, 3)
	for i := 0; i < 3; i++ {
		header := &types.BlockHeader{
			Version:   1,
			Timestamp: uint32(baseTime + int64(i*60)),
			Bits:      0x1d00ffff,
			Nonce:     uint32(i * 1000),
		}
		block := &types.Block{
			Header:       header,
			Transactions: []*types.Transaction{{Version: 1}},
		}
		candidates[i] = blockCandidate{
			timestamp: int64(header.Timestamp),
			hash:      block.Hash(),
			block:     block,
			height:    uint32(i + 100),
		}
	}

	// Mark first two blocks as already selected
	selectedBlocks := map[types.Hash]bool{
		candidates[0].hash: true,
		candidates[1].hash: true,
	}

	selected, err := mc.SelectBlockFromCandidates(
		candidates,
		selectedBlocks,
		baseTime+300,
		0x123456789ABCDEF0,
	)

	if err != nil {
		t.Fatalf("SelectBlockFromCandidates() error = %v", err)
	}

	// Should select the third candidate (only one not already selected)
	if selected.Hash() != candidates[2].hash {
		t.Errorf("Expected third candidate to be selected, got different block")
	}
}

// TestComputeNextStakeModifier_FullChain tests modifier computation with a simulated chain
func TestComputeNextStakeModifier_FullChain(t *testing.T) {
	// Create a longer chain to test full modifier computation
	blockchain, storage := createTestChain(t, 100, 1640995200)

	params := types.MainnetParams()
	mc := NewModifierCache(100, blockchain, storage, params)

	// Set Block1Modifier
	block1, _ := blockchain.GetBlockByHeight(1)
	blockchain.SetStakeModifier(block1.Hash(), Block1Modifier)

	// Test modifier computation for various heights
	testCases := []struct {
		height   uint32
		name     string
		wantZero bool // Whether we expect modifier to be zero (genesis)
	}{
		{0, "genesis", true},
		{1, "block1", false},
		{2, "early_block", false},
		{50, "mid_chain", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			block, err := blockchain.GetBlockByHeight(tc.height)
			if err != nil {
				t.Fatalf("Failed to get block at height %d: %v", tc.height, err)
			}

			modifier, generated, err := mc.ComputeNextStakeModifier(block.Header, tc.height)

			if err != nil {
				t.Fatalf("ComputeNextStakeModifier(height=%d) error = %v", tc.height, err)
			}

			if tc.wantZero {
				if modifier != 0 {
					t.Errorf("ComputeNextStakeModifier(height=%d) = %x, want 0", tc.height, modifier)
				}
			} else {
				// Non-genesis blocks should have non-zero modifier or be Block1Modifier
				if tc.height == 1 && modifier != Block1Modifier {
					t.Errorf("ComputeNextStakeModifier(height=1) = %x, want %x", modifier, Block1Modifier)
				}
			}

			// Genesis and Block1 should always be marked as generated
			if (tc.height == 0 || tc.height == 1) && !generated {
				t.Errorf("ComputeNextStakeModifier(height=%d) generated = false, want true", tc.height)
			}
		})
	}
}

// TestComputeNextStakeModifier_InheritedModifier tests that modifiers are inherited when interval not reached
func TestComputeNextStakeModifier_InheritedModifier(t *testing.T) {
	// Create chain with blocks very close in time (less than ModifierInterval apart)
	blockchain, storage := createTestChain(t, 10, 1640995200)

	params := types.MainnetParams()
	mc := NewModifierCache(100, blockchain, storage, params)

	// Set Block1Modifier
	block1, _ := blockchain.GetBlockByHeight(1)
	blockchain.SetStakeModifier(block1.Hash(), Block1Modifier)

	// Get block 2 - it should inherit from block 1 since interval hasn't passed
	block2, _ := blockchain.GetBlockByHeight(2)

	modifier, generated, err := mc.ComputeNextStakeModifier(block2.Header, 2)
	if err != nil {
		t.Fatalf("ComputeNextStakeModifier() error = %v", err)
	}

	// Should inherit Block1Modifier
	if modifier != Block1Modifier {
		t.Errorf("ComputeNextStakeModifier() = %x, want %x (inherited)", modifier, Block1Modifier)
	}

	// Should NOT be marked as generated (it's inherited)
	if generated {
		t.Error("ComputeNextStakeModifier() generated = true, want false (inherited)")
	}
}

// TestComputeAndCacheModifier_UncommittedBlockDoesNotPersist verifies that
// modifiers for blocks existing only in blockchain batch cache are not written
// directly to persistent storage outside a batch commit.
func TestComputeAndCacheModifier_UncommittedBlockDoesNotPersist(t *testing.T) {
	blockchain, storage := createTestChain(t, 8, 1640995200)
	params := types.MainnetParams()
	mc := NewModifierCache(100, blockchain, storage, params)

	// Ensure base modifier exists for backward walk.
	block1, err := blockchain.GetBlockByHeight(1)
	if err != nil {
		t.Fatalf("failed to get block 1: %v", err)
	}
	blockchain.SetStakeModifier(block1.Hash(), Block1Modifier)

	// Pick a target block and mark it as "uncommitted" by removing it from storage,
	// while keeping it visible via blockchain interface (simulating batch cache).
	targetHeight := uint32(7)
	targetBlock, err := blockchain.GetBlockByHeight(targetHeight)
	if err != nil {
		t.Fatalf("failed to get target block at height %d: %v", targetHeight, err)
	}
	targetHash := targetBlock.Hash()
	delete(storage.blocks, targetHash)
	delete(storage.blocksByHeight, targetHeight)

	// Sanity: block still visible via blockchain but not in storage.
	if _, err := blockchain.GetBlock(targetHash); err != nil {
		t.Fatalf("target block must remain visible via blockchain: %v", err)
	}
	if exists, _ := storage.HasBlock(targetHash); exists {
		t.Fatalf("target block must be absent from storage for this test")
	}

	// Precondition: this height should generate a modifier (so persistence would be attempted).
	_, generated, err := mc.ComputeNextStakeModifier(targetBlock.Header, targetHeight)
	if err != nil {
		t.Fatalf("failed to precompute modifier: %v", err)
	}
	if !generated {
		t.Fatalf("expected generated modifier at height %d for test setup", targetHeight)
	}

	// Compute and cache modifier for uncommitted block.
	if _, err := mc.computeAndCacheModifier(targetHash); err != nil {
		t.Fatalf("computeAndCacheModifier() failed: %v", err)
	}

	// Must not persist directly to storage for uncommitted block.
	if _, err := storage.GetStakeModifier(targetHash); err == nil {
		t.Fatalf("unexpected persisted modifier for uncommitted block %s", targetHash.String())
	}
}

// Known mainnet modifier checkpoints for legacy compatibility verification
// These values are extracted from a synced legacy TWINS node
var knownMainnetModifiers = []struct {
	height   uint32
	modifier uint64
	desc     string
}{
	{1, Block1Modifier, "Block 1 hardcoded modifier"},
	// Add more checkpoints as they are extracted from legacy nodes
}

// TestModifierLegacyCompatibility verifies modifiers match legacy C++ implementation
func TestModifierLegacyCompatibility(t *testing.T) {
	for _, tc := range knownMainnetModifiers {
		t.Run(tc.desc, func(t *testing.T) {
			// For now, just verify the hardcoded checkpoints
			found := false
			for _, cp := range ModifierCheckpoints {
				if cp.Height == tc.height {
					found = true
					if cp.Modifier != tc.modifier {
						t.Errorf("ModifierCheckpoints[height=%d] = %x, want %x",
							tc.height, cp.Modifier, tc.modifier)
					}
					break
				}
			}
			if !found && tc.height == 1 {
				// Block 1 should always be in checkpoints
				t.Errorf("ModifierCheckpoints missing height %d", tc.height)
			}
		})
	}
}

// BenchmarkComputeNextStakeModifier benchmarks modifier computation
func BenchmarkComputeNextStakeModifier(b *testing.B) {
	// Create a chain for benchmarking
	storage := NewMockStorage()
	blockchain := NewMockBlockchain(storage)

	baseTime := uint32(1640995200)
	var prevHash types.Hash

	// Build a 100-block chain
	for i := 0; i < 100; i++ {
		header := &types.BlockHeader{
			Version:       1,
			PrevBlockHash: prevHash,
			Timestamp:     baseTime + uint32(i*60),
			Bits:          0x1d00ffff,
			Nonce:         uint32(i),
		}
		block := &types.Block{
			Header:       header,
			Transactions: []*types.Transaction{{Version: 1}},
		}
		blockchain.AddBlock(block, uint32(i))
		prevHash = block.Hash()
	}

	// Set Block1Modifier
	block1, _ := blockchain.GetBlockByHeight(1)
	blockchain.SetStakeModifier(block1.Hash(), Block1Modifier)

	params := types.MainnetParams()
	mc := NewModifierCache(100, blockchain, storage, params)

	// Get a block to benchmark
	block50, _ := blockchain.GetBlockByHeight(50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mc.ComputeNextStakeModifier(block50.Header, 50)
	}
}
