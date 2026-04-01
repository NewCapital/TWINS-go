package consensus

import (
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/twins-dev/twins-core/internal/storage"
	"github.com/twins-dev/twins-core/pkg/types"
)

// TargetCalculator handles PoS difficulty adjustment and target calculation
type TargetCalculator struct {
	params     *types.ChainParams
	cache      *TargetCache
	storage    storage.Storage
	blockchain BlockchainInterface // For batch-aware block lookups during sync
}

// TargetCache provides thread-safe caching for difficulty targets
type TargetCache struct {
	targets map[types.Hash]*big.Int
	mu      sync.RWMutex
	maxSize int
	hits    uint64
	misses  uint64
}

// Target calculation constants for TWINS PoS
const (
	// PoS difficulty adjustment parameters (legacy ppcoin-style retargeting)
	// After LAST_POW_BLOCK (400), retarget EVERY block with exponential moving average
	TargetTimespan               = 40 * 60 // 40 minutes
	TargetSpacingSeconds         = 2 * 60  // 2 minutes per block (after block 19500)
	TargetSpacingSecondsOld      = 60      // 1 minute per block (before block 19500)
	DifficultyAdjustmentInterval = 1       // Adjust every block for PoS
	BlockHeightTargetSpacingChange = 19500 // Height where target spacing changed from 60s to 120s

	// Limits on difficulty adjustment
	MaxDifficultyAdjustmentRatio = 4 // Max 4x change per adjustment
	MinDifficultyAdjustmentRatio = 4 // Max 4x change per adjustment (inverse)

	// Compact target representation limits
	MaxBits = 0x207fffff // Maximum difficulty (minimum target)
	MinBits = 0x1d00ffff // Minimum difficulty (maximum target)
)

// Well-known target values
var (
	// Maximum target (minimum difficulty) for PoW
	// Legacy: bnProofOfWorkLimit = ~uint256(0) >> 20 (chainparams.cpp:220)
	// This means 256 - 20 = 236 bits set, starting from bit 0
	MaxTargetPoW = new(big.Int).SetBytes([]byte{
		0x00, 0x00, 0x0f, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	})

	// Maximum target (minimum difficulty) for PoS
	MaxTargetPoS = new(big.Int).SetBytes([]byte{
		0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	})

	// Minimum target (maximum difficulty) for PoS
	MinTargetPoS = new(big.Int).SetBytes([]byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff,
	})

	// Genesis block target
	GenesisTarget = new(big.Int).SetBytes([]byte{
		0x00, 0x00, 0x0f, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	})
)

// NewTargetCalculator creates a new target calculator
// blockchain parameter is optional but required for batch-aware lookups during sync
func NewTargetCalculator(params *types.ChainParams, cache *TargetCache, storage storage.Storage, blockchain BlockchainInterface) *TargetCalculator {
	if cache == nil {
		cache = NewTargetCache(MaxCacheSize)
	}

	return &TargetCalculator{
		params:     params,
		cache:      cache,
		storage:    storage,
		blockchain: blockchain,
	}
}

// NewTargetCache creates a new target cache
func NewTargetCache(maxSize int) *TargetCache {
	return &TargetCache{
		targets: make(map[types.Hash]*big.Int),
		maxSize: maxSize,
	}
}

// CalculateNextTarget calculates the next difficulty target based on recent block times
// For PoW blocks (≤400): Uses DarkGravity v3 algorithm (24-block average)
// For PoS blocks (>400): Uses ppcoin-style per-block retargeting
func (tc *TargetCalculator) CalculateNextTarget(prevHeader, currentHeader *types.BlockHeader, currentHeight uint32) (*big.Int, error) {
	if prevHeader == nil || currentHeader == nil {
		return nil, errors.New("headers cannot be nil")
	}

	// Genesis block uses predefined target
	if currentHeight == 0 {
		return new(big.Int).Set(GenesisTarget), nil
	}

	// Use hardcoded LastPOWBlock value (from ChainParams.LastPOWBlock = 400)
	const LastPOWBlock = 400

	// Use DarkGravity v3 for PoW blocks (height <= 400)
	if currentHeight <= LastPOWBlock {
		return tc.calculateDarkGravityV3(currentHeight)
	}

	// PoS blocks use ppcoin-style retargeting
	// Get target spacing based on block height
	nTargetSpacing := int64(TargetSpacingSeconds)
	if currentHeight < BlockHeightTargetSpacingChange {
		nTargetSpacing = int64(TargetSpacingSecondsOld)
	}

	// Calculate actual spacing (time between this block and previous block)
	// This is the key difference from Bitcoin - we retarget EVERY block
	nActualSpacing := int64(prevHeader.Timestamp) - int64(currentHeader.Timestamp)
	if currentHeight > 1 {
		// Need to get the block before prevHeader to calculate actual spacing
		prevPrevBlock, err := tc.getBlockAtHeight(currentHeight - 2)
		if err == nil && prevPrevBlock != nil {
			nActualSpacing = int64(prevHeader.Timestamp) - int64(prevPrevBlock.Header.Timestamp)
		}
	}

	// Negative spacing is invalid, clamp to 1 second
	if nActualSpacing < 0 {
		nActualSpacing = 1
	}

	// Get current target from previous block
	bnNew := GetTargetFromBits(prevHeader.Bits)

	// ppcoin: target change every block
	// ppcoin: retarget with exponential moving toward target spacing
	// Formula: bnNew *= ((nInterval - 1) * nTargetSpacing + 2 * nActualSpacing) / ((nInterval + 1) * nTargetSpacing)
	nTargetTimespan := int64(TargetTimespan)
	nInterval := nTargetTimespan / nTargetSpacing

	// Calculate numerator: (nInterval - 1) * nTargetSpacing + 2 * nActualSpacing
	numerator := big.NewInt((nInterval - 1) * nTargetSpacing)
	numerator.Add(numerator, big.NewInt(nActualSpacing))
	numerator.Add(numerator, big.NewInt(nActualSpacing)) // Add twice for 2 * nActualSpacing

	// Calculate denominator: (nInterval + 1) * nTargetSpacing
	denominator := big.NewInt((nInterval + 1) * nTargetSpacing)

	// Apply formula: bnNew = bnNew * numerator / denominator
	bnNew.Mul(bnNew, numerator)
	bnNew.Div(bnNew, denominator)

	// Apply target limits (bnTargetLimit = ~uint256(0) >> 24)
	if bnNew.Sign() <= 0 || bnNew.Cmp(MaxTargetPoS) > 0 {
		bnNew.Set(MaxTargetPoS)
	}

	return bnNew, nil
}

// calculateDarkGravityV3 implements DarkGravity v3 difficulty algorithm for PoW blocks
// This matches legacy pow.cpp lines 70-115 (DarkGravity v3 by Evan Duffield)
func (tc *TargetCalculator) calculateDarkGravityV3(currentHeight uint32) (*big.Int, error) {
	const PastBlocksMin = 24
	const PastBlocksMax = 24

	// If we don't have enough blocks, return max difficulty (easiest)
	// Legacy check: BlockLastSolved->nHeight < PastBlocksMin (previous block height, not current)
	// For block #1, previous block (genesis) has height 0, so 0 < 24 → return ProofOfWorkLimit
	prevHeight := currentHeight - 1
	if currentHeight == 0 || prevHeight < PastBlocksMin {
		return new(big.Int).Set(MaxTargetPoW), nil
	}

	var CountBlocks int64
	var PastDifficultyAverage *big.Int
	var PastDifficultyAveragePrev *big.Int
	var nActualTimespan int64
	var LastBlockTime int64

	// Iterate through past blocks to calculate average difficulty
	for i := uint32(1); i <= PastBlocksMax && (currentHeight-i) > 0; i++ {
		block, err := tc.getBlockAtHeight(currentHeight - i)
		if err != nil || block == nil {
			break
		}

		CountBlocks++

		// Calculate difficulty average (first PastBlocksMin blocks)
		if CountBlocks <= PastBlocksMin {
			target := GetTargetFromBits(block.Header.Bits)

			if CountBlocks == 1 {
				PastDifficultyAverage = new(big.Int).Set(target)
			} else {
				// PastDifficultyAverage = ((PastDifficultyAveragePrev * CountBlocks) + target) / (CountBlocks + 1)
				temp := new(big.Int).Mul(PastDifficultyAveragePrev, big.NewInt(CountBlocks))
				temp.Add(temp, target)
				PastDifficultyAverage = new(big.Int).Div(temp, big.NewInt(CountBlocks+1))
			}
			PastDifficultyAveragePrev = new(big.Int).Set(PastDifficultyAverage)
		}

		// Calculate actual timespan
		if LastBlockTime > 0 {
			diff := LastBlockTime - int64(block.Header.Timestamp)
			nActualTimespan += diff
		}
		LastBlockTime = int64(block.Header.Timestamp)
	}

	if PastDifficultyAverage == nil {
		return new(big.Int).Set(MaxTargetPoW), nil
	}

	bnNew := new(big.Int).Set(PastDifficultyAverage)

	// Calculate target timespan
	// Use TargetSpacingSecondsOld (60s) for PoW blocks
	targetTimespan := CountBlocks * int64(TargetSpacingSecondsOld)

	// Limit adjustment (no more than 3x in either direction)
	if nActualTimespan < targetTimespan/3 {
		nActualTimespan = targetTimespan / 3
	}
	if nActualTimespan > targetTimespan*3 {
		nActualTimespan = targetTimespan * 3
	}

	// Retarget: bnNew = bnNew * nActualTimespan / targetTimespan
	bnNew.Mul(bnNew, big.NewInt(nActualTimespan))
	bnNew.Div(bnNew, big.NewInt(targetTimespan))

	// Limit to max PoW target
	if bnNew.Cmp(MaxTargetPoW) > 0 {
		bnNew.Set(MaxTargetPoW)
	}

	return bnNew, nil
}

// GetTargetFromBits converts compact bits representation to target
func GetTargetFromBits(bits uint32) *big.Int {
	if bits == 0 {
		return big.NewInt(0)
	}

	// Extract exponent and mantissa
	exponent := int(bits >> 24)
	mantissa := bits & 0x7fffff

	// Handle sign bit (should not be set for valid targets)
	if bits&0x800000 != 0 {
		return big.NewInt(0) // Invalid target
	}

	// Convert to big.Int
	target := big.NewInt(int64(mantissa))

	if exponent <= 3 {
		// Shift right if exponent is small
		target.Rsh(target, uint(8*(3-exponent)))
	} else {
		// Shift left if exponent is large
		target.Lsh(target, uint(8*(exponent-3)))
	}

	return target
}

// GetBitsFromTarget converts target to compact bits representation
func GetBitsFromTarget(target *big.Int) uint32 {
	if target == nil || target.Sign() <= 0 {
		return 0
	}

	// Get target as bytes (big endian)
	targetBytes := target.Bytes()
	if len(targetBytes) == 0 {
		return 0
	}

	// Find the most significant byte
	exponent := len(targetBytes)
	mantissa := uint32(0)

	if exponent <= 3 {
		// Shift left to get 3 bytes
		mantissa = uint32(targetBytes[0])
		if len(targetBytes) > 1 {
			mantissa |= uint32(targetBytes[1]) << 8
		}
		if len(targetBytes) > 2 {
			mantissa |= uint32(targetBytes[2]) << 16
		}
		mantissa <<= uint(8 * (3 - exponent))
	} else {
		// Take the most significant 3 bytes
		mantissa = uint32(targetBytes[0]) << 16
		mantissa |= uint32(targetBytes[1]) << 8
		mantissa |= uint32(targetBytes[2])
	}

	// Ensure mantissa is valid (high bit should not be set)
	if mantissa&0x800000 != 0 {
		mantissa >>= 8
		exponent++
	}

	// Combine exponent and mantissa
	bits := uint32(exponent)<<24 | mantissa

	return bits
}

// GetBlockProof calculates the proof amount for a block header
func GetBlockProof(header *types.BlockHeader) *big.Int {
	if header == nil {
		return big.NewInt(0)
	}

	target := GetTargetFromBits(header.Bits)
	if target.Sign() <= 0 {
		return big.NewInt(0)
	}

	// Proof = (2^256 - 1) / (target + 1)
	// Simplified to: MaxTarget / target
	maxTarget := new(big.Int).Lsh(big.NewInt(1), 256)
	maxTarget.Sub(maxTarget, big.NewInt(1))

	proof := new(big.Int).Div(maxTarget, target)
	return proof
}

// CheckProofOfWork validates proof of work matching legacy pow.cpp:118-138
// Returns error for invalid PoW, nil for valid PoW
func CheckProofOfWork(blockHash types.Hash, bits uint32, powLimit *big.Int) error {
	// Convert compact bits to target
	target := GetTargetFromBits(bits)

	// Check for negative target (sign bit set in mantissa)
	// Legacy: bnTarget.SetCompact checks fNegative flag
	if bits&0x800000 != 0 {
		return fmt.Errorf("proof of work failed: negative target (bits: 0x%x)", bits)
	}

	// Check for zero target
	if target.Sign() == 0 {
		return fmt.Errorf("proof of work failed: zero target (bits: 0x%x)", bits)
	}

	// Check for overflow (target > max allowed)
	// Legacy: bnTarget > Params().ProofOfWorkLimit()
	if target.Cmp(powLimit) > 0 {
		return fmt.Errorf("proof of work failed: target exceeds limit (bits: 0x%x)", bits)
	}

	// Check proof of work matches claimed amount
	// Legacy: if (hash > bnTarget) return error("CheckProofOfWork() : hash doesn't match nBits")
	hashBig := blockHash.ToBig()
	if hashBig.Cmp(target) > 0 {
		return fmt.Errorf("proof of work failed: hash %s > target %s (bits: 0x%x)",
			blockHash.String(),
			target.Text(16),
			bits)
	}

	return nil
}

// getTargetFromCache retrieves target from cache or computes it
func (tc *TargetCalculator) getTargetFromCache(hash types.Hash, compute func() *big.Int) *big.Int {
	tc.cache.mu.RLock()
	if target, exists := tc.cache.targets[hash]; exists {
		tc.cache.hits++
		tc.cache.mu.RUnlock()
		return new(big.Int).Set(target)
	}
	tc.cache.misses++
	tc.cache.mu.RUnlock()

	// Compute and cache
	target := compute()
	if target != nil {
		tc.cache.mu.Lock()
		if len(tc.cache.targets) >= tc.cache.maxSize {
			tc.cache.evictOldest()
		}
		tc.cache.targets[hash] = new(big.Int).Set(target)
		tc.cache.mu.Unlock()
	}

	return target
}

// evictOldest removes oldest entry from cache
func (tc *TargetCache) evictOldest() {
	// Simplified eviction - remove first entry found
	for hash := range tc.targets {
		delete(tc.targets, hash)
		break
	}
}

// GetCacheStats returns cache performance statistics
func (tc *TargetCache) GetCacheStats() (hits, misses uint64, size int) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.hits, tc.misses, len(tc.targets)
}

// ClearCache clears all cached targets
func (tc *TargetCache) ClearCache() {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.targets = make(map[types.Hash]*big.Int)
}

// getBlockAtHeight retrieves block at specific height
// Uses blockchain interface if available (for batch-aware lookups during sync),
// otherwise falls back to storage
func (tc *TargetCalculator) getBlockAtHeight(height uint32) (*types.Block, error) {
	// Use blockchain if available - this is batch-aware and can access
	// blocks that are in the batch cache but not yet committed to storage
	if tc.blockchain != nil {
		return tc.blockchain.GetBlockByHeight(height)
	}

	// Fallback to storage (for tests or when blockchain not set)
	if tc.storage == nil {
		return nil, errors.New("storage not initialized")
	}

	// Get block hash at this height from the chain index
	blockHash, err := tc.storage.GetBlockHashByHeight(height)
	if err != nil {
		return nil, err
	}

	// Fetch the actual block from storage
	block, err := tc.storage.GetBlock(blockHash)
	if err != nil {
		return nil, err
	}

	return block, nil
}

// CalculateDifficulty calculates difficulty from target
func CalculateDifficulty(target *big.Int) *big.Int {
	if target == nil || target.Sign() <= 0 {
		return big.NewInt(0)
	}

	// Difficulty = MaxTarget / target
	difficulty := new(big.Int).Div(MaxTargetPoS, target)
	return difficulty
}

// GetDifficultyFromBits converts bits to difficulty value
func GetDifficultyFromBits(bits uint32) *big.Int {
	target := GetTargetFromBits(bits)
	return CalculateDifficulty(target)
}

// ValidateTargetBounds checks if target is within acceptable bounds
func ValidateTargetBounds(target *big.Int) error {
	if target == nil {
		return errors.New("target is nil")
	}

	if target.Sign() <= 0 {
		return errors.New("target must be positive")
	}

	if target.Cmp(MaxTargetPoS) > 0 {
		return errors.New("target exceeds maximum allowed")
	}

	if target.Cmp(MinTargetPoS) < 0 {
		return errors.New("target below minimum allowed")
	}

	return nil
}

// ValidateBits checks if bits representation is valid
func ValidateBits(bits uint32) error {
	if bits == 0 {
		return errors.New("bits cannot be zero")
	}

	// Check for sign bit (should not be set)
	if bits&0x800000 != 0 {
		return errors.New("invalid bits: sign bit set")
	}

	// Convert to target and validate
	target := GetTargetFromBits(bits)
	return ValidateTargetBounds(target)
}

// CompareTargets compares two targets (-1: t1 < t2, 0: t1 == t2, 1: t1 > t2)
func CompareTargets(t1, t2 *big.Int) int {
	if t1 == nil && t2 == nil {
		return 0
	}
	if t1 == nil {
		return -1
	}
	if t2 == nil {
		return 1
	}
	return t1.Cmp(t2)
}

// GetMinimumTarget returns the minimum allowed target
func GetMinimumTarget() *big.Int {
	return new(big.Int).Set(MinTargetPoS)
}

// GetMaximumTarget returns the maximum allowed target
func GetMaximumTarget() *big.Int {
	return new(big.Int).Set(MaxTargetPoS)
}

// IsValidDifficultyTransition checks if difficulty change is reasonable
func IsValidDifficultyTransition(oldTarget, newTarget *big.Int) bool {
	if oldTarget == nil || newTarget == nil {
		return false
	}

	// Calculate ratio of change
	ratio := new(big.Int).Div(newTarget, oldTarget)

	// Should not change by more than MaxDifficultyAdjustmentRatio
	maxRatio := big.NewInt(MaxDifficultyAdjustmentRatio)
	minRatio := new(big.Int).Div(big.NewInt(1), maxRatio)

	return ratio.Cmp(maxRatio) <= 0 && ratio.Cmp(minRatio) >= 0
}
