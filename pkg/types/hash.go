package types

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
)

// Hash represents a 256-bit hash value compatible with Bitcoin protocol
type Hash [32]byte

// ZeroHash is a hash filled with zeros
var ZeroHash = Hash{}

// NewHash creates a hash from the given data using SHA-256 double hashing
func NewHash(data []byte) Hash {
	first := sha256.Sum256(data)
	second := sha256.Sum256(first[:])
	return second
}

// NewHashFromString creates a hash from a hex string representation
// Interprets the input in display order (most significant byte first) and stores the hash using the internal little-endian layout
func NewHashFromString(s string) (Hash, error) {
	if len(s) != 64 {
		return ZeroHash, fmt.Errorf("hash string must be exactly 64 characters, got %d", len(s))
	}

	bytes, err := hex.DecodeString(s)
	if err != nil {
		return ZeroHash, fmt.Errorf("invalid hex string: %v", err)
	}

	var hash Hash
	for i := 0; i < len(hash); i++ {
		hash[i] = bytes[len(bytes)-1-i]
	}

	return hash, nil
}

// String returns the hex string representation of the hash
// Outputs the canonical display order (most significant byte first)
func (h Hash) String() string {
	reversed := h.Reverse()
	return hex.EncodeToString(reversed[:])
}

// Bytes returns a byte slice copy of the hash
func (h Hash) Bytes() []byte {
	bytes := make([]byte, 32)
	copy(bytes, h[:])
	return bytes
}

// IsEqual returns true if the hash equals the other hash
func (h Hash) IsEqual(other Hash) bool {
	return h == other
}

// IsZero returns true if the hash is all zeros
func (h Hash) IsZero() bool {
	return h == ZeroHash
}

// Reverse returns a new hash with reversed byte order (Bitcoin compatibility)
func (h Hash) Reverse() Hash {
	var reversed Hash
	for i := 0; i < 32; i++ {
		reversed[i] = h[31-i]
	}
	return reversed
}

// ToBig converts the hash to a big.Int for numerical comparisons (e.g., difficulty checks)
func (h Hash) ToBig() *big.Int {
	// Convert hash bytes to big.Int (treating as little-endian)
	// We need to reverse for big.Int which expects big-endian
	reversed := h.Reverse()
	return new(big.Int).SetBytes(reversed[:])
}

// GetCompact converts the hash to Bitcoin's compact format (used for difficulty targets).
// This matches legacy uint256::GetCompact(bool fNegative) from uint256.cpp:294-315.
// The compact format is: [1 byte exponent][3 bytes mantissa]
// For masternode ranking, fNegative is always false.
// Returns a uint32 that can be compared as int64 for ranking purposes.
func (h Hash) GetCompact() uint32 {
	// Count significant bits (from most significant byte)
	// Legacy: bits() returns position of highest set bit
	nBits := h.bits()
	nSize := (nBits + 7) / 8

	var nCompact uint32
	if nSize <= 3 {
		// Shift the low 64 bits left to fit in 3 bytes
		nCompact = uint32(h.getLow64() << (8 * (3 - nSize)))
	} else {
		// Shift right to get the most significant 3 bytes
		shifted := h.shiftRight(8 * (nSize - 3))
		nCompact = uint32(shifted.getLow64())
	}

	// If the 0x00800000 bit (sign bit in compact format) is set,
	// divide mantissa by 256 and increase exponent
	if nCompact&0x00800000 != 0 {
		nCompact >>= 8
		nSize++
	}

	// Combine exponent and mantissa
	// Clear any bits above mantissa
	nCompact &= 0x007fffff
	nCompact |= uint32(nSize) << 24

	return nCompact
}

// bits returns the position of the highest set bit (1-indexed), or 0 if the hash is zero.
// Legacy: base_uint::bits() from uint256.cpp
func (h Hash) bits() int {
	// Hash is stored in little-endian: h[0] is least significant, h[31] is most significant
	for i := 31; i >= 0; i-- {
		if h[i] != 0 {
			// Found the most significant non-zero byte
			// Count bits in this byte
			b := h[i]
			nBits := 0
			for b != 0 {
				nBits++
				b >>= 1
			}
			return (i * 8) + nBits
		}
	}
	return 0
}

// getLow64 returns the lowest 64 bits of the hash as uint64.
// Legacy: GetLow64() - interprets bytes 0-7 as little-endian uint64
func (h Hash) getLow64() uint64 {
	return uint64(h[0]) |
		uint64(h[1])<<8 |
		uint64(h[2])<<16 |
		uint64(h[3])<<24 |
		uint64(h[4])<<32 |
		uint64(h[5])<<40 |
		uint64(h[6])<<48 |
		uint64(h[7])<<56
}

// CompareTo compares two hashes numerically, matching C++ uint256::CompareTo behavior.
// Returns -1 if h < other, 0 if h == other, 1 if h > other.
// CRITICAL: This compares from most significant byte to least significant byte,
// which matches C++ uint256 comparison (comparing from WIDTH-1 down to 0).
// Hash is stored little-endian: h[0] = least significant, h[31] = most significant.
// Legacy: base_uint<256>::CompareTo from uint256.cpp:118-127
func (h Hash) CompareTo(other Hash) int {
	// Compare from most significant byte (index 31) to least significant (index 0)
	for i := 31; i >= 0; i-- {
		if h[i] < other[i] {
			return -1
		}
		if h[i] > other[i] {
			return 1
		}
	}
	return 0
}

// shiftRight returns a new hash shifted right by n bits.
// Legacy: operator>>= from uint256.cpp
func (h Hash) shiftRight(n int) Hash {
	var result Hash
	k := n / 8      // number of whole bytes to shift
	shift := n % 8  // remaining bits to shift

	for i := 0; i < 32; i++ {
		srcIdx := i + k
		if srcIdx < 32 {
			result[i] = h[srcIdx] >> shift
		}
		if srcIdx+1 < 32 && shift != 0 {
			result[i] |= h[srcIdx+1] << (8 - shift)
		}
	}
	return result
}
