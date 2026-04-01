package consensus

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
	"math/big"
	"time"

	"github.com/twins-dev/twins-core/pkg/types"
)

// CheckStakeKernelHash validates a stake kernel against the difficulty target
// This is the core PoS validation function that determines if a stake input
// successfully creates a valid block at a given timestamp
func CheckStakeKernelHash(modifier uint64, stakeInput *StakeInput, blockTime uint32, target *big.Int, params *types.ChainParams) (bool, types.Hash) {
	// Calculate the stake kernel hash
	kernelHash := ComputeStakeKernelHash(stakeInput, modifier, blockTime)

	// Check if kernel hash meets the target (use blockTime for weight calculation)
	stakeWeight := stakeInput.GetWeight(params, blockTime)
	isValid := StakeTargetHit(kernelHash, stakeWeight, target)

	return isValid, kernelHash
}

// ComputeStakeKernelHash calculates the stake kernel hash for PoS validation
// Matches legacy kernel.cpp CheckStake() exactly:
// KernelHash = Hash(nStakeModifier << nTimeBlockFrom << ssUniqueID << nTimeTx)
// where ssUniqueID = prevout hash + prevout index
func ComputeStakeKernelHash(stakeInput *StakeInput, modifier uint64, blockTime uint32) types.Hash {
	hasher := sha256.New()

	// 1. Serialize stake modifier (8 bytes, little endian) - matches nStakeModifier
	modifierBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(modifierBytes, modifier)
	hasher.Write(modifierBytes)

	// 2. Serialize block time from stake input (4 bytes, little endian) - matches nTimeBlockFrom
	// This is the time of the block that created the stake input UTXO
	txTimeBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(txTimeBytes, stakeInput.BlockTime)
	hasher.Write(txTimeBytes)

	// 3. Serialize ssUniqueID (prevout index + hash) - matches legacy CDataStream ssUniqueID
	// CRITICAL: Legacy order is INDEX then HASH (stakeinput.cpp:246 - ss << nPosition << txFrom.GetHash())
	// First the output index (4 bytes, little endian)
	indexBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(indexBytes, stakeInput.Index)
	hasher.Write(indexBytes)
	// Then the transaction hash (32 bytes)
	// CRITICAL: Both legacy C++ uint256 and our Hash type store bytes in little-endian format.
	// Legacy CDataStream serializes uint256 raw bytes as-is, so we use Hash bytes directly.
	// NO REVERSE needed - both are in the same byte order (little-endian internal storage).
	hasher.Write(stakeInput.TxHash[:])

	// 4. Serialize nTimeTx (4 bytes, little endian) - matches nTimeTx (the new block time)
	blockTimeBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(blockTimeBytes, blockTime)
	hasher.Write(blockTimeBytes)

	// First SHA256
	firstHash := hasher.Sum(nil)

	// Second SHA256 (double hash) - Hash() in legacy is double SHA256
	secondHasher := sha256.New()
	secondHasher.Write(firstHash)
	finalHash := secondHasher.Sum(nil)

	// Convert to Hash type - copy SHA256 output directly without reversal
	// SHA256 produces big-endian bytes which is the format needed for comparison
	// Legacy C++ also compares uint256 as big-endian (pn[WIDTH-1] to pn[0])
	var kernelHash types.Hash
	copy(kernelHash[:], finalHash)

	return kernelHash
}

// StakeTargetHit checks if a kernel hash meets the stake target
// The kernel hash is treated as a big integer and compared against the target
func StakeTargetHit(kernelHash types.Hash, stakeWeight int64, target *big.Int) bool {
	if stakeWeight <= 0 || target == nil || target.Sign() <= 0 {
		return false
	}

	// Convert kernel hash to big.Int for numerical comparison
	// CRITICAL: Legacy C++ stores SHA256 output directly into uint256 pn[] array,
	// where pn[0] is the LEAST significant word. This means the first bytes of
	// SHA256 output become the LEAST significant part of the number.
	// To match this, we need to reverse the bytes before SetBytes (which expects big-endian).
	reversedHash := kernelHash.Reverse()
	kernelBig := new(big.Int).SetBytes(reversedHash[:])

	// Adjust target based on stake weight
	// EffectiveTarget = Target * StakeWeight
	effectiveTarget := new(big.Int).Mul(target, big.NewInt(stakeWeight))

	// Check if kernel hash is less than effective target
	return kernelBig.Cmp(effectiveTarget) < 0
}

// ComputeStakeKernelHashV2 is an alternative kernel hash calculation
// This version includes additional data for enhanced security (if needed for upgrades)
func ComputeStakeKernelHashV2(stakeInput *StakeInput, modifier uint64, blockTime uint32, extraData []byte) types.Hash {
	hasher := sha256.New()

	// Standard kernel data
	modifierBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(modifierBytes, modifier)
	hasher.Write(modifierBytes)

	txTimeBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(txTimeBytes, stakeInput.BlockTime)
	hasher.Write(txTimeBytes)

	hasher.Write(stakeInput.TxHash.Bytes())

	indexBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(indexBytes, stakeInput.Index)
	hasher.Write(indexBytes)

	blockTimeBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(blockTimeBytes, blockTime)
	hasher.Write(blockTimeBytes)

	// Additional data (stake value, previous block hash, etc.)
	if len(extraData) > 0 {
		hasher.Write(extraData)
	}

	// Include stake value for additional entropy
	valueBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(valueBytes, uint64(stakeInput.Value))
	hasher.Write(valueBytes)

	// Double SHA256
	firstHash := hasher.Sum(nil)
	secondHasher := sha256.New()
	secondHasher.Write(firstHash)
	finalHash := secondHasher.Sum(nil)

	var kernelHash types.Hash
	copy(kernelHash[:], finalHash)

	return kernelHash
}

// ValidateStakeKernel performs comprehensive validation of a stake kernel
func ValidateStakeKernel(stakeInput *StakeInput, modifier uint64, blockTime uint32, target *big.Int, params *types.ChainParams) error {
	// Validate inputs
	if stakeInput == nil {
		return ErrPosInvalidStakeAge.WithCause(errors.New("stake input is nil"))
	}

	if target == nil || target.Sign() <= 0 {
		return ErrPosTargetNotMet.WithCause(errors.New("invalid target"))
	}

	// Check stake age (use chain params)
	coinAge := stakeInput.GetCoinAge(blockTime)
	if coinAge < int64(params.StakeMinAge.Seconds()) {
		return ErrPosInvalidStakeAge
	}

	// Check stake weight (use blockTime for calculation)
	stakeWeight := stakeInput.GetWeight(params, blockTime)
	if stakeWeight <= 0 {
		return ErrPosInsufficientWeight
	}

	// Validate timing
	if blockTime <= stakeInput.BlockTime {
		return ErrPosInvalidTimestamp.WithCause(errors.New("block time must be after stake time"))
	}

	// Check kernel hash against target
	isValid, _ := CheckStakeKernelHash(modifier, stakeInput, blockTime, target, params)
	if !isValid {
		return ErrPosTargetNotMet
	}

	return nil
}

// FindValidStakeKernel searches for a valid stake kernel within a time range
// This is used by miners/stakers to find a valid proof
func FindValidStakeKernel(stakeInput *StakeInput, modifier uint64, startTime, endTime uint32, target *big.Int, params *types.ChainParams) (uint32, types.Hash, error) {
	if stakeInput == nil || target == nil || startTime >= endTime {
		return 0, types.Hash{}, errors.New("invalid parameters")
	}

	// Search through the time range
	for blockTime := startTime; blockTime <= endTime; blockTime++ {
		isValid, kernelHash := CheckStakeKernelHash(modifier, stakeInput, blockTime, target, params)
		if isValid {
			return blockTime, kernelHash, nil
		}
	}

	return 0, types.Hash{}, errors.New("no valid kernel found in time range")
}

// GetKernelDifficulty calculates the difficulty achieved by a kernel hash
func GetKernelDifficulty(kernelHash types.Hash, stakeWeight int64) *big.Int {
	if stakeWeight <= 0 {
		return big.NewInt(0)
	}

	// Convert kernel hash to big.Int
	kernelBytes := kernelHash.Bytes()
	reversedBytes := make([]byte, len(kernelBytes))
	for i, b := range kernelBytes {
		reversedBytes[len(kernelBytes)-1-i] = b
	}

	kernelBig := new(big.Int).SetBytes(reversedBytes)
	if kernelBig.Sign() == 0 {
		return big.NewInt(0)
	}

	// Difficulty = MaxTarget * StakeWeight / KernelHash
	maxTarget := GetMaximumTarget()
	difficulty := new(big.Int).Mul(maxTarget, big.NewInt(stakeWeight))
	difficulty.Div(difficulty, kernelBig)

	return difficulty
}

// EstimateStakeTime estimates expected time to find a valid stake
// This helps stakers understand their expected staking frequency
func EstimateStakeTime(stakeWeight int64, networkWeight uint64, targetSpacing time.Duration) time.Duration {
	if stakeWeight <= 0 || networkWeight == 0 {
		return time.Duration(0)
	}

	// Expected time = TargetSpacing * (NetworkWeight / StakeWeight)
	ratio := float64(networkWeight) / float64(stakeWeight)
	expectedSeconds := float64(targetSpacing.Seconds()) * ratio

	return time.Duration(expectedSeconds * float64(time.Second))
}

// BatchValidateKernels validates multiple stake kernels efficiently
// This is useful for validating multiple stakes in a single operation
func BatchValidateKernels(stakeInputs []*StakeInput, modifier uint64, blockTime uint32, target *big.Int, params *types.ChainParams) ([]bool, []types.Hash) {
	if len(stakeInputs) == 0 {
		return nil, nil
	}

	results := make([]bool, len(stakeInputs))
	hashes := make([]types.Hash, len(stakeInputs))

	// Validate each kernel
	for i, stakeInput := range stakeInputs {
		if stakeInput == nil {
			results[i] = false
			continue
		}

		isValid, kernelHash := CheckStakeKernelHash(modifier, stakeInput, blockTime, target, params)
		results[i] = isValid
		hashes[i] = kernelHash
	}

	return results, hashes
}

// GetKernelEntropy calculates entropy of a kernel hash for randomness analysis
func GetKernelEntropy(kernelHash types.Hash) float64 {
	// Simple entropy calculation based on bit distribution
	hashBytes := kernelHash.Bytes()
	bitCounts := make([]int, 2)

	for _, b := range hashBytes {
		for i := 0; i < 8; i++ {
			bit := (b >> uint(i)) & 1
			bitCounts[bit]++
		}
	}

	totalBits := len(hashBytes) * 8
	if totalBits == 0 {
		return 0.0
	}

	// Calculate Shannon entropy
	entropy := 0.0
	for _, count := range bitCounts {
		if count > 0 {
			p := float64(count) / float64(totalBits)
			entropy -= p * math.Log2(p)
		}
	}

	return entropy
}

// SerializeKernelData creates serialized data for kernel hash calculation
// LEGACY COMPLIANCE: Must match ComputeStakeKernelHash order exactly
// Order: modifier (8) → txTime (4) → index (4) → hash (32) → blockTime (4)
// Legacy: ss << nStakeModifier << nTimeBlockFrom << ssUniqueID << nTimeTx
// where ssUniqueID = nPosition << txFrom.GetHash() (index then hash)
func SerializeKernelData(stakeInput *StakeInput, modifier uint64, blockTime uint32) []byte {
	var result []byte

	// 1. Stake modifier (8 bytes)
	modifierBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(modifierBytes, modifier)
	result = append(result, modifierBytes...)

	// 2. Block time from stake input (4 bytes) - nTimeBlockFrom
	txTimeBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(txTimeBytes, stakeInput.BlockTime)
	result = append(result, txTimeBytes...)

	// 3. ssUniqueID: INDEX first, then HASH (legacy stakeinput.cpp:246)
	// Transaction index (4 bytes)
	indexBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(indexBytes, stakeInput.Index)
	result = append(result, indexBytes...)

	// Transaction hash (32 bytes)
	result = append(result, stakeInput.TxHash[:]...)

	// 4. New block time (4 bytes) - nTimeTx
	blockTimeBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(blockTimeBytes, blockTime)
	result = append(result, blockTimeBytes...)

	return result
}

// VerifyKernelHash verifies a precomputed kernel hash
func VerifyKernelHash(stakeInput *StakeInput, modifier uint64, blockTime uint32, expectedHash types.Hash) bool {
	calculatedHash := ComputeStakeKernelHash(stakeInput, modifier, blockTime)
	return calculatedHash == expectedHash
}

// GetStakeSearchRange calculates the time range for stake searching
func GetStakeSearchRange(currentTime uint32, futureLimit time.Duration) (uint32, uint32) {
	startTime := currentTime
	endTime := currentTime + uint32(futureLimit.Seconds())
	return startTime, endTime
}
