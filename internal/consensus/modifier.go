package consensus

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/twins-dev/twins-core/internal/storage"
	"github.com/twins-dev/twins-core/pkg/types"
)

// Modifier calculation constants (from legacy kernel.h)
const (
	ModifierInterval       = 60    // Seconds between modifier changes (mainnet and testnet)
	ModifierIntervalRatio  = 3     // Ratio between last and first group intervals
	MaxModifierSearchDepth = 10080 // Maximum blocks to search backwards (~1 week at 60s)
)

// Block1Modifier is the stake modifier for block 1 (first block after genesis)
// Legacy C++ kernel.cpp:162 uses: nStakeModifier = uint64_t("stakemodifier")
// This casts the string POINTER address to uint64, not the string content.
// The actual value depends on the compiled binary and was extracted from
// a synced legacy database. The value is now hardcoded as a constant.
const Block1Modifier = uint64(0x000055e2ab932345)

// ModifierCache provides thread-safe caching for stake modifiers
type ModifierCache struct {
	cache      map[types.Hash]*StakeModifier
	mu         sync.RWMutex
	maxSize    int
	hits       uint64
	misses     uint64
	blockchain BlockchainInterface // Access to blockchain for modifier calculation
	storage    storage.Storage     // Access to storage for block data and modifier persistence
	params     *types.ChainParams  // Chain parameters
}

// ModifierCheckpoint represents a hardcoded checkpoint for modifier validation
type ModifierCheckpoint struct {
	Height   uint32
	Hash     types.Hash
	Modifier uint64
	Checksum uint32
}

// Hardcoded modifier checkpoints for TWINS network
var ModifierCheckpoints = []ModifierCheckpoint{
	{Height: 1, Modifier: Block1Modifier}, // First block after genesis
	// Additional checkpoints should be added from mainnet for validation:
	// - Every 10,000 blocks is recommended
	// - Critical fork points should always have checkpoints
}

// NewModifierCache creates a new stake modifier cache
func NewModifierCache(maxSize int, blockchain BlockchainInterface, storage storage.Storage, params *types.ChainParams) *ModifierCache {
	return &ModifierCache{
		cache:      make(map[types.Hash]*StakeModifier),
		maxSize:    maxSize,
		blockchain: blockchain,
		storage:    storage,
		params:     params,
	}
}

// GetStakeModifier retrieves the stake modifier for a given block hash
func (mc *ModifierCache) GetStakeModifier(blockHash types.Hash) (uint64, error) {
	return mc.GetStakeModifierWithBatch(blockHash, nil)
}

// GetStakeModifierWithBatch retrieves the stake modifier for a given block hash
// If batch is provided, stores computed modifier in batch (for atomic commit)
// If batch is nil, stores directly to storage (backward compatibility)
func (mc *ModifierCache) GetStakeModifierWithBatch(blockHash types.Hash, batch storage.Batch) (uint64, error) {
	mc.mu.RLock()
	if modifier, exists := mc.cache[blockHash]; exists {
		mc.hits++
		mc.mu.RUnlock()
		return modifier.Modifier, nil
	}
	mc.misses++
	mc.mu.RUnlock()

	// Check storage first
	if mc.storage != nil {
		if modifier, err := mc.storage.GetStakeModifier(blockHash); err == nil {
			// Cache the result
			mc.mu.Lock()
			mc.cache[blockHash] = &StakeModifier{
				Modifier:  modifier,
				BlockHash: blockHash,
			}
			mc.mu.Unlock()
			return modifier, nil
		}
	}

	// Check blockchain batch cache (captures uncommitted modifiers)
	if mc.blockchain != nil {
		if modifier, err := mc.blockchain.GetStakeModifier(blockHash); err == nil {
			mc.mu.Lock()
			mc.cache[blockHash] = &StakeModifier{
				Modifier:  modifier,
				BlockHash: blockHash,
			}
			mc.mu.Unlock()
			return modifier, nil
		}
	}

	// Cache miss - need to compute modifier
	return mc.computeAndCacheModifierWithBatch(blockHash, batch)
}

// computeAndCacheModifier computes and caches a stake modifier (backward compatibility wrapper)
func (mc *ModifierCache) computeAndCacheModifier(blockHash types.Hash) (uint64, error) {
	return mc.computeAndCacheModifierWithBatch(blockHash, nil)
}

// computeAndCacheModifierWithBatch computes and caches a stake modifier
// If batch is provided, stores modifier in batch for atomic commit
// If batch is nil, stores directly to storage (backward compatibility)
func (mc *ModifierCache) computeAndCacheModifierWithBatch(blockHash types.Hash, batch storage.Batch) (uint64, error) {
	// Quick check with read lock
	mc.mu.RLock()
	if modifier, exists := mc.cache[blockHash]; exists {
		mc.mu.RUnlock()
		return modifier.Modifier, nil
	}
	mc.mu.RUnlock()

	// Compute without holding lock (I/O operations)
	// Prefer blockchain interface to leverage batch caches during unified processing.
	var block *types.Block
	var err error
	if mc.blockchain != nil {
		block, err = mc.blockchain.GetBlock(blockHash)
	} else {
		block, err = mc.storage.GetBlock(blockHash)
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get block: %w", err)
	}

	var height uint32
	if mc.blockchain != nil {
		height, err = mc.blockchain.GetBlockHeight(blockHash)
	} else {
		height, err = mc.storage.GetBlockHeight(blockHash)
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get block height: %w", err)
	}

	// Compute the modifier using the legacy algorithm
	modifier, generated, err := mc.ComputeNextStakeModifier(block.Header, height)
	if err != nil {
		return 0, err
	}

	// Store in database if modifier was generated
	if generated {
		if batch != nil {
			// Store in batch for atomic commit (preferred during block processing)
			if err := batch.StoreStakeModifier(blockHash, modifier); err != nil {
				return 0, fmt.Errorf("failed to store stake modifier in batch: %w", err)
			}
		} else if mc.storage != nil {
			shouldPersist := true
			// During unified batch processing, block data may only exist in the blockchain
			// batch cache (uncommitted). Avoid writing such modifiers directly to storage
			// outside the active batch, otherwise a failed batch can leave stale modifiers.
			if mc.blockchain != nil {
				if exists, err := mc.storage.HasBlock(blockHash); err == nil && !exists {
					shouldPersist = false
				}
			}
			if shouldPersist {
				// Fallback to direct storage (backward compatibility for non-batch operations)
				if err := mc.storage.StoreStakeModifier(blockHash, modifier); err != nil {
					return 0, fmt.Errorf("failed to store stake modifier: %w", err)
				}
			}
		}
	}

	// Now lock only to update cache
	mc.mu.Lock()
	defer mc.mu.Unlock()

	// Double-check cache again in case another goroutine added it
	if existing, exists := mc.cache[blockHash]; exists {
		return existing.Modifier, nil
	}

	// Store in cache
	stakeModifier := &StakeModifier{
		Modifier:  modifier,
		BlockHash: blockHash,
		Height:    height,
		Time:      block.Header.Timestamp,
	}

	// Add to cache with eviction if necessary
	if len(mc.cache) >= mc.maxSize {
		mc.evictOldest()
	}
	mc.cache[blockHash] = stakeModifier

	return modifier, nil
}

// GetLastStakeModifier walks backwards from given block to find last generated modifier
// Matches legacy kernel.cpp:44-55
func (mc *ModifierCache) GetLastStakeModifier(blockHash types.Hash) (uint64, uint32, error) {
	if blockHash == types.ZeroHash {
		return 0, 0, errors.New("null block hash")
	}

	// CRITICAL: Blockchain interface must be initialized via SetBlockchain()
	// Using storage directly bypasses batch cache and can cause race conditions
	if mc.blockchain == nil {
		return 0, 0, errors.New("blockchain interface not initialized - call SetBlockchain() first")
	}

	currentHash := blockHash
	for depth := 0; depth < MaxModifierSearchDepth; depth++ {
		// Check if this block has a stored modifier (indicates it was generated here)
		// Use blockchain.GetStakeModifier() to check batch cache first, then storage
		modifier, err := mc.blockchain.GetStakeModifier(currentHash)

		if err == nil {
			// Found a stored modifier - this block generated it
			// Get block to retrieve timestamp
			block, err := mc.blockchain.GetBlock(currentHash)
			if err != nil {
				return 0, 0, fmt.Errorf("failed to get block at depth %d: %w", depth, err)
			}
			return modifier, block.Header.Timestamp, nil
		}

		// No modifier found, move to previous block
		block, err := mc.blockchain.GetBlock(currentHash)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to get block at depth %d: %w", depth, err)
		}

		// Check if reached genesis block
		if block.Header.PrevBlockHash == types.ZeroHash {
			// Reached genesis block - return modifier for first block
			return Block1Modifier, 0, nil
		}

		currentHash = block.Header.PrevBlockHash
	}

	return 0, 0, fmt.Errorf("modifier search exceeded maximum depth (%d blocks)", MaxModifierSearchDepth)
}

// GetStakeModifierSelectionIntervalSection calculates interval for a specific section (0-63)
// Matches legacy kernel.cpp:58-63
func GetStakeModifierSelectionIntervalSection(section int) int64 {
	if section < 0 || section >= 64 {
		return 0
	}
	// Formula: MODIFIER_INTERVAL * 63 / (63 + ((63 - section) * (RATIO - 1)))
	return int64(ModifierInterval) * 63 / (63 + int64((63-section)*(ModifierIntervalRatio-1)))
}

// GetStakeModifierSelectionInterval returns total selection interval (sum of all 64 sections)
// Matches legacy kernel.cpp:66-73
func GetStakeModifierSelectionInterval() int64 {
	var interval int64
	for section := 0; section < 64; section++ {
		interval += GetStakeModifierSelectionIntervalSection(section)
	}
	return interval
}

// blockCandidate represents a candidate block for modifier selection
type blockCandidate struct {
	timestamp int64
	hash      types.Hash
	block     *types.Block
	height    uint32 // Block height (needed for Modifier V2 upgrade check)
}

// SelectBlockFromCandidates selects a block from candidates for a given round
// IMPORTANT: Candidates MUST be sorted by timestamp (ascending) before calling
// Matches legacy kernel.cpp:78-136
// CRITICAL: Modifier V2 flag is checked ONCE for the FIRST candidate only (legacy kernel.cpp:98-102)
// The fFirstRun pattern means we use the first (lowest) block's height to determine V2 for all candidates
func (mc *ModifierCache) SelectBlockFromCandidates(
	candidates []blockCandidate,
	selectedBlocks map[types.Hash]bool,
	selectionIntervalStop int64,
	prevModifier uint64,
) (*types.Block, error) {
	var bestBlock *types.Block
	var bestHash types.Hash
	selected := false

	// CRITICAL FIX: Determine ModifierV2 flag ONCE based on the FIRST candidate
	// Legacy kernel.cpp:98-102:
	//   if (fFirstRun){
	//       fModifierV2 = pindex->nHeight >= Params().ModifierUpgradeBlock();
	//       fFirstRun = false;
	//   }
	// This means the first (lowest height) candidate in the sorted list determines V2 for ALL candidates
	useModifierV2 := false
	if len(candidates) > 0 && mc.params != nil {
		useModifierV2 = candidates[0].height >= mc.params.ModifierUpgradeBlock
	}

	for _, candidate := range candidates {
		// Skip if already selected
		if selectedBlocks[candidate.hash] {
			continue
		}

		// Stop if past selection interval
		if selected && int64(candidate.block.Header.Timestamp) > selectionIntervalStop {
			break
		}

		// Compute selection hash (legacy lines 108-112)
		var hashProof types.Hash
		if useModifierV2 {
			// Modifier V2: Use actual block hash for ALL blocks (PoS and PoW)
			hashProof = candidate.hash
		} else {
			// Modifier V1: Use zero hash for PoS, actual hash for PoW
			if candidate.block.IsProofOfStake() {
				hashProof = types.ZeroHash
			} else {
				hashProof = candidate.hash
			}
		}

		// CRITICAL FIX: Use DOUBLE SHA256 (Bitcoin's Hash256), not single SHA256
		// Legacy kernel.cpp:116: uint256 hashSelection = Hash(ss.begin(), ss.end());
		// Bitcoin's Hash() function is double SHA256
		var buf [40]byte // 32 bytes hash + 8 bytes modifier
		copy(buf[:32], hashProof[:])
		binary.LittleEndian.PutUint64(buf[32:], prevModifier)

		// First SHA256
		first := sha256.Sum256(buf[:])
		// Second SHA256 (double hash)
		second := sha256.Sum256(first[:])

		var hashSelection types.Hash
		copy(hashSelection[:], second[:])

		// PoS blocks favored: divide by 2^32 (right shift by 32 bits)
		// Legacy kernel.cpp:121-122: if (pindex->IsProofOfStake()) hashSelection >>= 32;
		if candidate.block.IsProofOfStake() {
			// Right shift the hash by 32 bits (4 bytes)
			// In little-endian, shifting right moves higher bytes to lower positions
			shifted := make([]byte, 32)
			// Copy bytes 4-31 to positions 0-27 (equivalent to >>= 32 on little-endian uint256)
			for i := 0; i < 32-4; i++ {
				shifted[i] = hashSelection[i+4]
			}
			// Last 4 bytes remain zero
			copy(hashSelection[:], shifted)
		}

		// Select block with smallest hash
		if !selected || compareHashes(hashSelection, bestHash) < 0 {
			bestHash = hashSelection
			bestBlock = candidate.block
			selected = true
		}
	}

	if !selected {
		return nil, errors.New("no block selected from candidates")
	}

	return bestBlock, nil
}

// ComputeNextStakeModifier calculates the next stake modifier using legacy algorithm
// Matches legacy kernel.cpp:151-249
// CRITICAL: Legacy uses pindexPrev (previous block) for all timestamp checks,
// so we must load the previous block's timestamp, NOT use the current header's timestamp.
func (mc *ModifierCache) ComputeNextStakeModifier(header *types.BlockHeader, height uint32) (uint64, bool, error) {
	if header == nil {
		return 0, false, errors.New("header is nil")
	}

	// Genesis block modifier
	if height == 0 {
		return 0, true, nil
	}

	// First block after genesis - uses hardcoded modifier from legacy kernel.cpp:162
	// Legacy: nStakeModifier = uint64_t("stakemodifier") = 0x646f6d656b617473
	if height == 1 {
		return Block1Modifier, true, nil
	}

	// CRITICAL FIX: Load previous block to get its timestamp
	// Legacy kernel.cpp:184 uses pindexPrev->GetBlockTime(), NOT the current block's time
	var prevBlock *types.Block
	var err error
	if mc.blockchain != nil {
		prevBlock, err = mc.blockchain.GetBlock(header.PrevBlockHash)
	} else {
		prevBlock, err = mc.storage.GetBlock(header.PrevBlockHash)
	}
	if err != nil {
		return 0, false, fmt.Errorf("failed to get previous block: %w", err)
	}
	prevBlockTime := int64(prevBlock.Header.Timestamp)

	// Get last stake modifier and its generation time
	prevModifier, modifierTime, err := mc.GetLastStakeModifier(header.PrevBlockHash)
	if err != nil {
		return 0, false, fmt.Errorf("unable to get last modifier: %w", err)
	}

	modTime := int64(modifierTime)
	if modTime < 0 {
		return 0, false, fmt.Errorf("modifier timestamp overflow: %d", modifierTime)
	}

	// CRITICAL FIX: Check interval using PREVIOUS block's timestamp, not current
	// Legacy kernel.cpp:184:
	//   if (nModifierTime / getIntervalVersion(fTestNet) >= pindexPrev->GetBlockTime() / getIntervalVersion(fTestNet))
	//       return true;
	if modTime/ModifierInterval >= prevBlockTime/ModifierInterval {
		return prevModifier, false, nil
	}

	// CRITICAL FIX: Use PREVIOUS block's timestamp for selection interval calculation
	// Legacy kernel.cpp:191:
	//   int64_t nSelectionIntervalStart = (pindexPrev->GetBlockTime() / getIntervalVersion(fTestNet)) * getIntervalVersion(fTestNet) - nSelectionInterval;
	selectionInterval := GetStakeModifierSelectionInterval()
	selectionIntervalStart := (prevBlockTime / ModifierInterval * ModifierInterval) - selectionInterval

	var candidates []blockCandidate
	currentHash := header.PrevBlockHash

	// Walk backwards collecting candidates
	for {
		// Use blockchain.GetBlock() if available (checks batch cache)
		// Otherwise fall back to storage.GetBlock() (direct storage access)
		var block *types.Block
		var err error
		if mc.blockchain != nil {
			block, err = mc.blockchain.GetBlock(currentHash)
		} else {
			block, err = mc.storage.GetBlock(currentHash)
		}
		if err != nil {
			break
		}

		if int64(block.Header.Timestamp) < selectionIntervalStart {
			break
		}

		// Get height for this candidate (needed for Modifier V2 check)
		var candidateHeight uint32
		if mc.blockchain != nil {
			candidateHeight, err = mc.blockchain.GetBlockHeight(currentHash)
		} else {
			candidateHeight, err = mc.storage.GetBlockHeight(currentHash)
		}
		if err != nil {
			// If we can't get height, skip this candidate
			break
		}

		candidates = append(candidates, blockCandidate{
			timestamp: int64(block.Header.Timestamp),
			hash:      block.Hash(),
			block:     block,
			height:    candidateHeight,
		})

		if block.Header.PrevBlockHash == types.ZeroHash {
			break
		}
		currentHash = block.Header.PrevBlockHash
	}

	// Check if we have any candidates
	if len(candidates) == 0 {
		return 0, false, errors.New("no candidate blocks for modifier selection")
	}

	// Reverse to get chronological order, then sort by (timestamp, hash)
	// CRITICAL: Legacy kernel.cpp uses map<pair<int64_t, uint256>, CBlockIndex*> which
	// sorts by timestamp first, then by hash when timestamps are equal.
	// This is important when multiple blocks have the same timestamp.
	for i, j := 0, len(candidates)-1; i < j; i, j = i+1, j-1 {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].timestamp != candidates[j].timestamp {
			return candidates[i].timestamp < candidates[j].timestamp
		}
		// When timestamps are equal, sort by hash (ascending)
		return compareHashes(candidates[i].hash, candidates[j].hash) < 0
	})

	// Select 64 blocks to generate stake modifier
	var newModifier uint64
	selectedBlocks := make(map[types.Hash]bool)
	selectionIntervalStop := selectionIntervalStart

	rounds := 64
	if len(candidates) < 64 {
		rounds = len(candidates)
	}

	for round := 0; round < rounds; round++ {
		// Add interval section for this round
		selectionIntervalStop += GetStakeModifierSelectionIntervalSection(round)

		// Select block from candidates
		selectedBlock, err := mc.SelectBlockFromCandidates(
			candidates,
			selectedBlocks,
			selectionIntervalStop,
			prevModifier,
		)
		if err != nil {
			return 0, false, fmt.Errorf("unable to select block at round %d: %w", round, err)
		}

		// Calculate entropy bit from selected block
		entropyBit := mc.calculateEntropyBit(selectedBlock)

		// Set the bit in the new modifier
		if entropyBit == 1 {
			newModifier |= (uint64(1) << uint(round))
		}

		// Mark block as selected
		selectedBlocks[selectedBlock.Hash()] = true
	}

	return newModifier, true, nil
}

// calculateEntropyBit calculates the entropy bit (0 or 1) for a block
// Matches legacy chain.h:347-354 GetStakeEntropyBit()
// Uses LSB of block hash for ALL blocks (both PoW and PoS)
func (mc *ModifierCache) calculateEntropyBit(block *types.Block) uint8 {
	// Legacy: nEntropyBit = ((GetBlockHash().Get64()) & 1)
	// CHash256::Finalize() writes SHA256 output directly to uint256 memory.
	// uint256::Get64() returns pn[0] | (pn[1] << 32), where pn[0] contains
	// the first 4 bytes of SHA256 output.
	// So Get64() & 1 extracts the LSB of the first byte of SHA256 output.
	//
	// Our NewHash() copies SHA256 output directly to Hash[0..31], so
	// Hash[0] = first byte of SHA256 = same as what legacy reads.
	hash := block.Hash()
	return hash[0] & 1
}

// compareHashes compares two hashes, returns -1 if a < b, 0 if equal, 1 if a > b
// Matches legacy C++ uint256::CompareTo() which compares uint32 words from MSB to LSB.
//
// Legacy CompareTo (uint256.cpp):
//
//	for (int i = WIDTH - 1; i >= 0; i--) {  // WIDTH=8 for uint256
//	    if (pn[i] < b.pn[i]) return -1;
//	    if (pn[i] > b.pn[i]) return 1;
//	}
//
// CRITICAL: Legacy compares uint32 words, not individual bytes!
// pn[7] contains bytes 28-31 as a little-endian uint32.
// We must compare uint32 words in the same order to match legacy behavior.
func compareHashes(a, b types.Hash) int {
	// Compare 8 uint32 words from pn[7] (bytes 28-31) down to pn[0] (bytes 0-3)
	for i := 7; i >= 0; i-- {
		offset := i * 4
		// Read uint32 as little-endian (matching how pn[] is stored)
		aWord := uint32(a[offset]) | uint32(a[offset+1])<<8 | uint32(a[offset+2])<<16 | uint32(a[offset+3])<<24
		bWord := uint32(b[offset]) | uint32(b[offset+1])<<8 | uint32(b[offset+2])<<16 | uint32(b[offset+3])<<24
		if aWord < bWord {
			return -1
		}
		if aWord > bWord {
			return 1
		}
	}
	return 0
}

// evictOldest removes the oldest entry from cache (simplified LRU)
func (mc *ModifierCache) evictOldest() {
	// Simplified eviction - just remove first entry
	// Full implementation would use proper LRU tracking
	for hash := range mc.cache {
		delete(mc.cache, hash)
		break
	}
}

// GetCacheStats returns cache performance statistics
func (mc *ModifierCache) GetCacheStats() (hits, misses uint64, size int) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.hits, mc.misses, len(mc.cache)
}

// ClearCache clears all cached modifiers
func (mc *ModifierCache) ClearCache() {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.cache = make(map[types.Hash]*StakeModifier)
}

// GetModifierChecksum calculates a checksum for modifier validation
func (mc *ModifierCache) GetModifierChecksum(header *types.BlockHeader, height uint32) uint32 {
	if header == nil {
		return 0
	}

	// Simplified checksum calculation
	hasher := sha256.New()
	hasher.Write(header.Hash().Bytes())
	binary.Write(hasher, binary.LittleEndian, height)

	hash := hasher.Sum(nil)
	return binary.LittleEndian.Uint32(hash[:4])
}

// ValidateModifierCheckpoints validates modifier against known checkpoints
func (mc *ModifierCache) ValidateModifierCheckpoints(height uint32, checksum uint32) bool {
	for _, checkpoint := range ModifierCheckpoints {
		if checkpoint.Height == height {
			return checkpoint.Checksum == checksum
		}
	}

	// If no checkpoint exists for this height, validation passes
	return true
}

// GetKernelStakeModifier retrieves the stake modifier to use for kernel hash validation
// Implements legacy kernel.cpp:255-283 GetKernelStakeModifier()
//
// CRITICAL: This is NOT the same as GetStakeModifier()!
// - GetStakeModifier() returns the modifier stored FOR a block
// - GetKernelStakeModifier() returns the modifier to USE WHEN VALIDATING a stake from that block
//
// Algorithm (from legacy kernel.cpp:255-283):
// 1. Start at the block where the UTXO was created (hashBlockFrom)
// 2. Calculate selection interval (GetStakeModifierSelectionInterval ~= 20 minutes)
// 3. Move FORWARD from that block until time exceeds block_time + selection_interval
// 4. Return the modifier from the LAST block in that range
//
// This ensures the modifier used for validation is deterministic but not predictable at UTXO creation time.
func (mc *ModifierCache) GetKernelStakeModifier(utxoBlockHash types.Hash) (uint64, uint32, uint32, error) {
	// Get the block where the UTXO was created
	var utxoBlock *types.Block
	var err error
	if mc.blockchain != nil {
		utxoBlock, err = mc.blockchain.GetBlock(utxoBlockHash)
	} else {
		utxoBlock, err = mc.storage.GetBlock(utxoBlockHash)
	}
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to get UTXO source block: %w", err)
	}

	var utxoHeight uint32
	if mc.blockchain != nil {
		utxoHeight, err = mc.blockchain.GetBlockHeight(utxoBlockHash)
	} else {
		// Fallback: search storage (less efficient)
		for h := uint32(0); h < 10000000; h++ {
			hash, err := mc.storage.GetBlockHashByHeight(h)
			if err != nil {
				break
			}
			if hash == utxoBlockHash {
				utxoHeight = h
				break
			}
		}
	}

	// Calculate selection interval
	// This is the sum of 64 intervals with varying lengths (kernel.cpp:66-73)
	selectionInterval := mc.getStakeModifierSelectionInterval()

	// Find the last block within the selection interval
	// Loop forward from UTXO block until time exceeds utxo_time + selection_interval
	currentHeight := utxoHeight
	currentTime := utxoBlock.Header.Timestamp
	targetTime := utxoBlock.Header.Timestamp + selectionInterval

	var modifierHeight uint32
	var modifierTime uint32
	var modifier uint64

	modifierHeight = utxoHeight
	modifierTime = utxoBlock.Header.Timestamp

	// Loop forward, updating modifier from each block that has one
	// CRITICAL: Loop condition uses modifierTime (last PoS block time), not currentTime
	// This matches legacy kernel.cpp:268 which checks nStakeModifierTime in the loop condition
	// nStakeModifierTime is ONLY updated for PoS blocks (line 278)
	// This means the loop continues past the interval if we hit PoW blocks,
	// and only stops when a PoS block's time >= utxo_time + interval
	for modifierTime < targetTime {
		nextHeight := currentHeight + 1

		// Get next block
		var nextBlock *types.Block
		if mc.blockchain != nil {
			nextBlock, err = mc.blockchain.GetBlockByHeight(nextHeight)
			if err != nil {
				break
			}
		} else {
			nextHash, err := mc.storage.GetBlockHashByHeight(nextHeight)
			if err != nil {
				break // Reached end of chain
			}
			nextBlock, err = mc.storage.GetBlock(nextHash)
			if err != nil {
				break
			}
		}

		if nextBlock == nil {
			break // Reached end of chain
		}

		currentHeight = nextHeight
		currentTime = nextBlock.Header.Timestamp

		// Check if this block generated a stake modifier
		// CRITICAL: Check blockchain batch cache and storage for presence of modifier
		// Do NOT call mc.GetStakeModifier() as that would compute/cache modifiers recursively
		// Only blocks that generated a modifier have one stored (inherited modifiers are not stored)
		nextBlockHash := nextBlock.Hash()
		hasModifier := false
		if mc.blockchain != nil {
			// blockchain.GetStakeModifier checks batch cache first, then storage
			if _, err := mc.blockchain.GetStakeModifier(nextBlockHash); err == nil {
				hasModifier = true
			}
		} else if mc.storage != nil {
			// Fallback to storage directly
			if _, err := mc.storage.GetStakeModifier(nextBlockHash); err == nil {
				hasModifier = true
			}
		}

		if hasModifier {
			// This block generated a new modifier, update selection
			modifierHeight = currentHeight
			modifierTime = currentTime
		}
	}

	// Get the modifier from the LAST block we looped through (currentHeight)
	// This matches legacy kernel.cpp:281 which returns pindex->nStakeModifier
	// where pindex is the last block in the loop (the one that caused loop exit)
	// NOT the last PoS block (modifierHeight)!
	var lastBlockHash types.Hash
	if mc.blockchain != nil {
		lastBlock, err := mc.blockchain.GetBlockByHeight(currentHeight)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("failed to get last block at height %d: %w", currentHeight, err)
		}
		lastBlockHash = lastBlock.Hash()
	} else {
		lastBlockHash, err = mc.storage.GetBlockHashByHeight(currentHeight)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("failed to get last block hash at height %d: %w", currentHeight, err)
		}
	}

	// CRITICAL: Use computeAndCacheModifier, NOT GetStakeModifier!
	// In legacy, every block has nStakeModifier field containing the ACTIVE modifier at that block.
	// Our storage only stores modifiers for blocks where generated=true.
	// computeAndCacheModifier will compute the active modifier for ANY block (whether generated or inherited).
	modifier, err = mc.computeAndCacheModifier(lastBlockHash)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to compute stake modifier for block %d: %w", currentHeight, err)
	}

	// Return the last PoS block's height and time for informational purposes
	// (matches legacy nStakeModifierHeight and nStakeModifierTime which track last PoS)
	// but use modifier from last block in loop (currentHeight)
	return modifier, modifierHeight, modifierTime, nil
}

// getStakeModifierSelectionInterval calculates the total selection interval
// Implements legacy kernel.cpp:66-73 GetStakeModifierSelectionInterval()
// Returns interval in seconds (~1200 seconds = ~20 minutes for mainnet)
func (mc *ModifierCache) getStakeModifierSelectionInterval() uint32 {
	var selectionInterval uint32 = 0

	// Sum of 64 sections with increasing lengths (kernel.cpp:58-63, 66-73)
	for section := 0; section < 64; section++ {
		// Each section length follows: interval * 63 / (63 + (63 - section) * (ratio - 1))
		// Where interval = 60 (MODIFIER_INTERVAL) and ratio = 3 (MODIFIER_INTERVAL_RATIO)
		// This creates a geometric progression from shortest (section 0) to longest (section 63)
		sectionLength := uint32(ModifierInterval) * 63 / uint32(63+(63-section)*(ModifierIntervalRatio-1))
		selectionInterval += sectionLength
	}

	return selectionInterval
}
