package crypto

import (
	"bytes"
	"testing"
)

func TestHash256(t *testing.T) {
	data := []byte("test data for SHA-256")
	hash := Hash256(data)

	if len(hash) != 32 {
		t.Errorf("Hash256 should return 32 bytes, got %d", len(hash))
	}

	// Test consistency
	hash2 := Hash256(data)
	if !bytes.Equal(hash, hash2) {
		t.Error("Hash256 should be consistent")
	}

	// Test different data produces different hash
	differentData := []byte("different data for SHA-256")
	differentHash := Hash256(differentData)
	if bytes.Equal(hash, differentHash) {
		t.Error("Different data should produce different hash")
	}
}

func TestHash160(t *testing.T) {
	data := []byte("test data for RIPEMD-160")
	hash := Hash160(data)

	if len(hash) != 20 {
		t.Errorf("Hash160 should return 20 bytes, got %d", len(hash))
	}

	// Test consistency
	hash2 := Hash160(data)
	if !bytes.Equal(hash, hash2) {
		t.Error("Hash160 should be consistent")
	}

	// Test against known Bitcoin address generation
	// This should work with standard Bitcoin public keys
	if CompareHashes(hash, make([]byte, 20)) {
		t.Error("Hash160 should not return all zeros for non-zero input")
	}
}

func TestDoubleHash256(t *testing.T) {
	data := []byte("test data for double SHA-256")
	doubleHash := DoubleHash256(data)

	if len(doubleHash) != 32 {
		t.Errorf("DoubleHash256 should return 32 bytes, got %d", len(doubleHash))
	}

	// Test consistency
	doubleHash2 := DoubleHash256(data)
	if !bytes.Equal(doubleHash, doubleHash2) {
		t.Error("DoubleHash256 should be consistent")
	}

	// Test that double hash differs from single hash
	singleHash := Hash256(data)
	if bytes.Equal(doubleHash, singleHash) {
		t.Error("Double hash should differ from single hash")
	}

	// Test against manual double hashing
	firstHash := Hash256(data)
	expectedDoubleHash := Hash256(firstHash)
	if !bytes.Equal(doubleHash, expectedDoubleHash) {
		t.Error("DoubleHash256 should match manual double hashing")
	}
}

func TestHash256SHA3(t *testing.T) {
	data := []byte("test data for SHA-3")
	hash := Hash256SHA3(data)

	if len(hash) != 32 {
		t.Errorf("Hash256SHA3 should return 32 bytes, got %d", len(hash))
	}

	// Test consistency
	hash2 := Hash256SHA3(data)
	if !bytes.Equal(hash, hash2) {
		t.Error("Hash256SHA3 should be consistent")
	}

	// Test that SHA-3 differs from SHA-256
	sha256Hash := Hash256(data)
	if bytes.Equal(hash, sha256Hash) {
		t.Error("SHA-3 hash should differ from SHA-256 hash")
	}
}

func TestHMACShA256(t *testing.T) {
	key := []byte("secret key for HMAC")
	data := []byte("data to authenticate")
	hmac := HMACSHA256(key, data)

	if len(hmac) != 32 {
		t.Errorf("HMACSHA256 should return 32 bytes, got %d", len(hmac))
	}

	// Test consistency
	hmac2 := HMACSHA256(key, data)
	if !bytes.Equal(hmac, hmac2) {
		t.Error("HMACSHA256 should be consistent")
	}

	// Test with different key produces different HMAC
	differentKey := []byte("different secret key")
	differentHMAC := HMACSHA256(differentKey, data)
	if bytes.Equal(hmac, differentHMAC) {
		t.Error("Different key should produce different HMAC")
	}

	// Test with different data produces different HMAC
	differentData := []byte("different data to authenticate")
	differentDataHMAC := HMACSHA256(key, differentData)
	if bytes.Equal(hmac, differentDataHMAC) {
		t.Error("Different data should produce different HMAC")
	}
}

func TestMerkleHash(t *testing.T) {
	left := Hash256([]byte("left branch"))
	right := Hash256([]byte("right branch"))

	merkleHash := MerkleHash(left, right)

	if len(merkleHash) != 32 {
		t.Errorf("MerkleHash should return 32 bytes, got %d", len(merkleHash))
	}

	// Test consistency
	merkleHash2 := MerkleHash(left, right)
	if !bytes.Equal(merkleHash, merkleHash2) {
		t.Error("MerkleHash should be consistent")
	}

	// Test with swapped inputs produces different hash
	swappedHash := MerkleHash(right, left)
	if bytes.Equal(merkleHash, swappedHash) {
		t.Error("Swapped inputs should produce different merkle hash")
	}

	// Test with padded inputs
	shortLeft := []byte{1, 2, 3}
	shortRight := []byte{4, 5, 6}
	paddedHash := MerkleHash(shortLeft, shortRight)
	if len(paddedHash) != 32 {
		t.Errorf("MerkleHash with short inputs should return 32 bytes, got %d", len(paddedHash))
	}
}

func TestChecksumHash(t *testing.T) {
	data := []byte("test data for checksum")
	checksum := ChecksumHash(data)

	if len(checksum) != 4 {
		t.Errorf("ChecksumHash should return 4 bytes, got %d", len(checksum))
	}

	// Test consistency
	checksum2 := ChecksumHash(data)
	if !bytes.Equal(checksum, checksum2) {
		t.Error("ChecksumHash should be consistent")
	}

	// Test different data produces different checksum
	differentData := []byte("different data for checksum")
	differentChecksum := ChecksumHash(differentData)
	if bytes.Equal(checksum, differentChecksum) {
		t.Error("Different data should produce different checksum")
	}

	// Verify checksum is first 4 bytes of double hash
	doubleHash := DoubleHash256(data)
	expectedChecksum := doubleHash[:4]
	if !bytes.Equal(checksum, expectedChecksum) {
		t.Error("ChecksumHash should match first 4 bytes of double hash")
	}
}

func TestHash160Hasher(t *testing.T) {
	hasher := NewHash160Hasher()

	// Test Size and BlockSize
	if hasher.Size() != 20 {
		t.Errorf("Hash160Hasher Size should be 20, got %d", hasher.Size())
	}

	if hasher.BlockSize() <= 0 {
		t.Error("Hash160Hasher BlockSize should be positive")
	}

	// Test Write and Sum
	data := []byte("test data for Hash160Hasher")
	n, err := hasher.Write(data)
	if err != nil {
		t.Fatalf("Hash160Hasher Write failed: %v", err)
	}

	if n != len(data) {
		t.Errorf("Hash160Hasher Write should return %d, got %d", len(data), n)
	}

	hash := hasher.Sum(nil)
	if len(hash) != 20 {
		t.Errorf("Hash160Hasher Sum should return 20 bytes, got %d", len(hash))
	}

	// Test against direct Hash160 function
	expectedHash := Hash160(data)
	hasher.Reset()
	hasher.Write(data)
	hasherResult := hasher.Sum(nil)

	if !bytes.Equal(expectedHash, hasherResult) {
		t.Error("Hash160Hasher result should match Hash160 function")
	}

	// Test Reset
	hasher.Reset()
	hasher.Write([]byte("different data"))
	resetHash := hasher.Sum(nil)
	if bytes.Equal(hash, resetHash) {
		t.Error("Hash160Hasher Reset should clear state")
	}
}

func TestStreamingHasher(t *testing.T) {
	hasher := NewStreamingHasher()

	data1 := []byte("first chunk of data")
	data2 := []byte("second chunk of data")

	// Test Write
	n1, err := hasher.Write(data1)
	if err != nil {
		t.Fatalf("StreamingHasher Write failed: %v", err)
	}
	if n1 != len(data1) {
		t.Errorf("StreamingHasher Write should return %d, got %d", len(data1), n1)
	}

	n2, err := hasher.Write(data2)
	if err != nil {
		t.Fatalf("StreamingHasher Write failed: %v", err)
	}
	if n2 != len(data2) {
		t.Errorf("StreamingHasher Write should return %d, got %d", len(data2), n2)
	}

	// Test Sum
	hash := hasher.Sum()
	if len(hash) != 32 {
		t.Errorf("StreamingHasher Sum should return 32 bytes, got %d", len(hash))
	}

	// Test SumDouble
	hasher.Reset()
	hasher.Write(data1)
	hasher.Write(data2)
	doubleHash := hasher.SumDouble()
	if len(doubleHash) != 32 {
		t.Errorf("StreamingHasher SumDouble should return 32 bytes, got %d", len(doubleHash))
	}

	// Test that SumDouble differs from Sum
	if bytes.Equal(hash, doubleHash) {
		t.Error("SumDouble should differ from Sum")
	}

	// Test against direct hashing
	combinedData := append(data1, data2...)
	expectedHash := Hash256(combinedData)
	if !bytes.Equal(hash, expectedHash) {
		t.Error("StreamingHasher should match direct hashing")
	}

	expectedDoubleHash := DoubleHash256(combinedData)
	if !bytes.Equal(doubleHash, expectedDoubleHash) {
		t.Error("StreamingHasher SumDouble should match direct double hashing")
	}
}

func TestUtilityFunctions(t *testing.T) {
	hash1 := Hash256([]byte("test data 1"))
	hash2 := Hash256([]byte("test data 2"))
	hash1Copy := Hash256([]byte("test data 1"))

	// Test CompareHashes
	if !CompareHashes(hash1, hash1Copy) {
		t.Error("CompareHashes should return true for identical hashes")
	}

	if CompareHashes(hash1, hash2) {
		t.Error("CompareHashes should return false for different hashes")
	}

	if CompareHashes(hash1, hash1[:20]) {
		t.Error("CompareHashes should return false for different length hashes")
	}

	// Test IsZeroHash
	zeroHash := make([]byte, 32)
	if !IsZeroHash(zeroHash) {
		t.Error("IsZeroHash should return true for all-zero hash")
	}

	if IsZeroHash(hash1) {
		t.Error("IsZeroHash should return false for non-zero hash")
	}

	// Test ReverseHash
	reversed := ReverseHash(hash1)
	if len(reversed) != len(hash1) {
		t.Errorf("ReverseHash should return same length, got %d vs %d", len(reversed), len(hash1))
	}

	doubleReversed := ReverseHash(reversed)
	if !bytes.Equal(hash1, doubleReversed) {
		t.Error("Double reverse should return original hash")
	}

	// Check that reverse actually reverses
	for i, b := range hash1 {
		if reversed[len(reversed)-1-i] != b {
			t.Error("ReverseHash doesn't properly reverse bytes")
			break
		}
	}
}

func TestSpecializedHashingFunctions(t *testing.T) {
	// Test PBKDF2SHA256
	password := []byte("test password")
	salt := []byte("test salt")
	iterations := 1000
	keyLength := 32

	derivedKey := PBKDF2SHA256(password, salt, iterations, keyLength)
	if len(derivedKey) != keyLength {
		t.Errorf("PBKDF2SHA256 should return %d bytes, got %d", keyLength, len(derivedKey))
	}

	// Test consistency
	derivedKey2 := PBKDF2SHA256(password, salt, iterations, keyLength)
	if !bytes.Equal(derivedKey, derivedKey2) {
		t.Error("PBKDF2SHA256 should be consistent")
	}

	// Test different password produces different key
	differentPassword := []byte("different password")
	differentKey := PBKDF2SHA256(differentPassword, salt, iterations, keyLength)
	if bytes.Equal(derivedKey, differentKey) {
		t.Error("Different password should produce different key")
	}

	// Test ScryptHash (placeholder implementation)
	scryptKey, err := ScryptHash(password, salt, 16384, 8, 1, 32)
	if err != nil {
		t.Errorf("ScryptHash failed: %v", err)
	}
	if len(scryptKey) != 32 {
		t.Errorf("ScryptHash should return 32 bytes, got %d", len(scryptKey))
	}

	// Test Blake2bHash (placeholder implementation)
	blake2bHash := Blake2bHash([]byte("test data"))
	if len(blake2bHash) != 32 {
		t.Errorf("Blake2bHash should return 32 bytes, got %d", len(blake2bHash))
	}
}

func BenchmarkHash256(b *testing.B) {
	data := []byte("benchmark data for SHA-256 hashing performance testing")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Hash256(data)
	}
}

func BenchmarkHash160(b *testing.B) {
	data := []byte("benchmark data for Hash160 performance testing")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Hash160(data)
	}
}

func BenchmarkDoubleHash256(b *testing.B) {
	data := []byte("benchmark data for double SHA-256 performance testing")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = DoubleHash256(data)
	}
}

func BenchmarkHMACShA256(b *testing.B) {
	key := []byte("benchmark key for HMAC-SHA256")
	data := []byte("benchmark data for HMAC-SHA256 performance testing")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = HMACSHA256(key, data)
	}
}

func BenchmarkMerkleHash(b *testing.B) {
	left := Hash256([]byte("left branch for merkle hash benchmark"))
	right := Hash256([]byte("right branch for merkle hash benchmark"))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = MerkleHash(left, right)
	}
}

func BenchmarkStreamingHasher(b *testing.B) {
	data := []byte("benchmark data for streaming hasher performance")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hasher := NewStreamingHasher()
		hasher.Write(data)
		_ = hasher.Sum()
	}
}

func BenchmarkPBKDF2SHA256(b *testing.B) {
	password := []byte("benchmark password")
	salt := []byte("benchmark salt")
	iterations := 1000
	keyLength := 32

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = PBKDF2SHA256(password, salt, iterations, keyLength)
	}
}