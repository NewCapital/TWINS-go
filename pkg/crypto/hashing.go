package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"hash"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/ripemd160"
	"golang.org/x/crypto/scrypt"
	"golang.org/x/crypto/sha3"
)

// Hash160Hasher provides SHA-256 followed by RIPEMD-160 hashing (Bitcoin standard)
type Hash160Hasher struct {
	sha256Hash hash.Hash
}

// NewHash160Hasher creates a new Hash160 hasher
func NewHash160Hasher() *Hash160Hasher {
	return &Hash160Hasher{
		sha256Hash: sha256.New(),
	}
}

// Hash256 performs SHA-256 hashing
func Hash256(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
}

// Hash160 performs SHA-256 followed by RIPEMD-160 (Bitcoin address hashing)
func Hash160(data []byte) []byte {
	// First SHA-256
	sha256Hash := sha256.Sum256(data)

	// Then RIPEMD-160
	ripemd := ripemd160.New()
	ripemd.Write(sha256Hash[:])
	return ripemd.Sum(nil)
}

// DoubleHash256 performs double SHA-256 hashing (Bitcoin standard)
func DoubleHash256(data []byte) []byte {
	first := sha256.Sum256(data)
	second := sha256.Sum256(first[:])
	return second[:]
}

// Hash256SHA3 performs SHA-3 hashing for future use
func Hash256SHA3(data []byte) []byte {
	hash := sha3.Sum256(data)
	return hash[:]
}

// Hash512SHA3 performs SHA-3-512 hashing
func Hash512SHA3(data []byte) []byte {
	hash := sha3.Sum512(data)
	return hash[:]
}

// HMACSHA256 performs HMAC-SHA256 authenticated hashing
func HMACSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

// MerkleHash combines two hashes for merkle tree operations
func MerkleHash(left, right []byte) []byte {
	if len(left) != 32 || len(right) != 32 {
		// Pad or truncate to 32 bytes if necessary
		if len(left) < 32 {
			padded := make([]byte, 32)
			copy(padded[32-len(left):], left)
			left = padded
		} else if len(left) > 32 {
			left = left[:32]
		}

		if len(right) < 32 {
			padded := make([]byte, 32)
			copy(padded[32-len(right):], right)
			right = padded
		} else if len(right) > 32 {
			right = right[:32]
		}
	}

	combined := make([]byte, 64)
	copy(combined[:32], left)
	copy(combined[32:], right)

	return DoubleHash256(combined)
}

// ChecksumHash generates a checksum hash for addresses
func ChecksumHash(data []byte) []byte {
	hash := DoubleHash256(data)
	return hash[:4] // First 4 bytes as checksum
}

// Hash methods for the Hash160Hasher

// Write implements hash.Hash interface
func (h *Hash160Hasher) Write(p []byte) (n int, err error) {
	return h.sha256Hash.Write(p)
}

// Sum implements hash.Hash interface
func (h *Hash160Hasher) Sum(b []byte) []byte {
	// Get SHA-256 result
	sha256Result := h.sha256Hash.Sum(nil)

	// Apply RIPEMD-160
	ripemd := ripemd160.New()
	ripemd.Write(sha256Result)
	result := ripemd.Sum(nil)

	if b == nil {
		return result
	}
	return append(b, result...)
}

// Reset implements hash.Hash interface
func (h *Hash160Hasher) Reset() {
	h.sha256Hash.Reset()
}

// Size implements hash.Hash interface
func (h *Hash160Hasher) Size() int {
	return ripemd160.Size // 20 bytes
}

// BlockSize implements hash.Hash interface
func (h *Hash160Hasher) BlockSize() int {
	return h.sha256Hash.BlockSize()
}

// Advanced hashing functions

// PBKDF2SHA256 derives a key from a password using PBKDF2 with SHA-256
func PBKDF2SHA256(password, salt []byte, iterations, keyLength int) []byte {
	return pbkdf2(sha256.New, password, salt, iterations, keyLength)
}

// ScryptHash derives a key using Scrypt (memory-hard function)
func ScryptHash(password, salt []byte, N, r, p, keyLength int) ([]byte, error) {
	// Use golang.org/x/crypto/scrypt for proper implementation
	// This is critical for wallet key derivation compatibility with legacy
	return scrypt.Key(password, salt, N, r, p, keyLength)
}

// Blake2bHash performs BLAKE2b hashing (optional, for future use)
func Blake2bHash(data []byte) []byte {
	// Use golang.org/x/crypto/blake2b for proper BLAKE2b hashing
	hash := blake2b.Sum256(data)
	return hash[:]
}

// Specialized hashing for blockchain operations

// HashTransaction creates a hash for a transaction
func HashTransaction(version uint32, inputs, outputs, lockTime []byte) []byte {
	hasher := sha256.New()

	// Write version
	versionBytes := make([]byte, 4)
	versionBytes[0] = byte(version)
	versionBytes[1] = byte(version >> 8)
	versionBytes[2] = byte(version >> 16)
	versionBytes[3] = byte(version >> 24)
	hasher.Write(versionBytes)

	// Write inputs
	hasher.Write(inputs)

	// Write outputs
	hasher.Write(outputs)

	// Write lock time
	hasher.Write(lockTime)

	// Double hash
	first := hasher.Sum(nil)
	return Hash256(first)
}

// HashBlock creates a hash for a block header
func HashBlock(version uint32, prevHash, merkleRoot []byte, timestamp, bits, nonce uint32) []byte {
	hasher := sha256.New()

	// Write version
	writeUint32(hasher, version)

	// Write previous hash
	hasher.Write(prevHash)

	// Write merkle root
	hasher.Write(merkleRoot)

	// Write timestamp
	writeUint32(hasher, timestamp)

	// Write bits
	writeUint32(hasher, bits)

	// Write nonce
	writeUint32(hasher, nonce)

	// Double hash
	first := hasher.Sum(nil)
	return Hash256(first)
}

// HashStakeModifier creates a hash for stake modifier calculation
func HashStakeModifier(prevModifier uint64, timestamp uint32, blockHash []byte) []byte {
	hasher := sha256.New()

	// Write previous modifier
	writeUint64(hasher, prevModifier)

	// Write timestamp
	writeUint32(hasher, timestamp)

	// Write block hash
	hasher.Write(blockHash)

	return Hash256(hasher.Sum(nil))
}

// Utility functions for hashing operations

// CompareHashes compares two hash values for equality
func CompareHashes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

// IsZeroHash checks if a hash is all zeros
func IsZeroHash(hash []byte) bool {
	for _, b := range hash {
		if b != 0 {
			return false
		}
	}
	return true
}

// ReverseHash reverses a hash (for Bitcoin compatibility)
func ReverseHash(hash []byte) []byte {
	reversed := make([]byte, len(hash))
	for i, b := range hash {
		reversed[len(hash)-1-i] = b
	}
	return reversed
}

// Helper functions

func writeUint32(w hash.Hash, value uint32) {
	bytes := make([]byte, 4)
	bytes[0] = byte(value)
	bytes[1] = byte(value >> 8)
	bytes[2] = byte(value >> 16)
	bytes[3] = byte(value >> 24)
	w.Write(bytes)
}

func writeUint64(w hash.Hash, value uint64) {
	bytes := make([]byte, 8)
	bytes[0] = byte(value)
	bytes[1] = byte(value >> 8)
	bytes[2] = byte(value >> 16)
	bytes[3] = byte(value >> 24)
	bytes[4] = byte(value >> 32)
	bytes[5] = byte(value >> 40)
	bytes[6] = byte(value >> 48)
	bytes[7] = byte(value >> 56)
	w.Write(bytes)
}

// PBKDF2 implementation (simplified)
func pbkdf2(h func() hash.Hash, password, salt []byte, iter, keyLen int) []byte {
	prf := hmac.New(h, password)
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen

	var buf [4]byte
	dk := make([]byte, 0, numBlocks*hashLen)

	for block := 1; block <= numBlocks; block++ {
		// INT(i)
		buf[0] = byte(block >> 24)
		buf[1] = byte(block >> 16)
		buf[2] = byte(block >> 8)
		buf[3] = byte(block)

		prf.Reset()
		prf.Write(salt)
		prf.Write(buf[:4])
		u := prf.Sum(nil)

		out := make([]byte, len(u))
		copy(out, u)

		for i := 1; i < iter; i++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(u[:0])
			for j, v := range u {
				out[j] ^= v
			}
		}

		dk = append(dk, out...)
	}

	return dk[:keyLen]
}

// Streaming hasher for large data

// StreamingHasher allows hashing of large amounts of data in chunks
type StreamingHasher struct {
	hasher hash.Hash
}

// NewStreamingHasher creates a new streaming hasher
func NewStreamingHasher() *StreamingHasher {
	return &StreamingHasher{
		hasher: sha256.New(),
	}
}

// Write adds data to the hash
func (sh *StreamingHasher) Write(data []byte) (int, error) {
	return sh.hasher.Write(data)
}

// Sum finalizes the hash and returns the result
func (sh *StreamingHasher) Sum() []byte {
	return sh.hasher.Sum(nil)
}

// SumDouble finalizes the hash with double hashing
func (sh *StreamingHasher) SumDouble() []byte {
	first := sh.hasher.Sum(nil)
	return Hash256(first)
}

// Reset resets the hasher for reuse
func (sh *StreamingHasher) Reset() {
	sh.hasher.Reset()
}