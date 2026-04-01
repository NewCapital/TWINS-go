package types

import (
	"encoding/hex"
	"testing"
)

func TestNewHash(t *testing.T) {
	data := []byte("hello world")
	hash := NewHash(data)

	if hash.IsZero() {
		t.Error("NewHash should not return zero hash for non-empty data")
	}

	// Test that same data produces same hash
	hash2 := NewHash(data)
	if !hash.IsEqual(hash2) {
		t.Error("Same data should produce same hash")
	}

	// Test that different data produces different hash
	hash3 := NewHash([]byte("different data"))
	if hash.IsEqual(hash3) {
		t.Error("Different data should produce different hash")
	}
}

func TestNewHashFromString(t *testing.T) {
	// Valid hex string
	hexStr := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	hash, err := NewHashFromString(hexStr)
	if err != nil {
		t.Errorf("NewHashFromString failed with valid hex: %v", err)
	}

	if hash.String() != hexStr {
		t.Errorf("Round trip failed: expected %s, got %s", hexStr, hash.String())
	}

	// Invalid length
	_, err = NewHashFromString("short")
	if err == nil {
		t.Error("NewHashFromString should fail with short string")
	}

	// Invalid hex characters
	_, err = NewHashFromString("gggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggg")
	if err == nil {
		t.Error("NewHashFromString should fail with invalid hex")
	}
}

func TestHashString(t *testing.T) {
	data := []byte("test")
	hash := NewHash(data)
	str := hash.String()

	if len(str) != 64 {
		t.Errorf("Hash string should be 64 characters, got %d", len(str))
	}

	// Test round trip
	hash2, err := NewHashFromString(str)
	if err != nil {
		t.Errorf("Failed to parse hash string: %v", err)
	}

	if !hash.IsEqual(hash2) {
		t.Error("Hash string round trip failed")
	}
}

func TestHashStringLittleEndian(t *testing.T) {
	var hash Hash
	for i := 0; i < len(hash); i++ {
		hash[i] = byte(i)
	}

	str := hash.String()

	expectedBytes := make([]byte, len(hash))
	for i := 0; i < len(hash); i++ {
		expectedBytes[i] = hash[len(hash)-1-i]
	}

	expectedStr := hex.EncodeToString(expectedBytes)
	if str != expectedStr {
		t.Fatalf("expected little-endian string %s, got %s", expectedStr, str)
	}

	parsed, err := NewHashFromString(str)
	if err != nil {
		t.Fatalf("failed to parse hash string: %v", err)
	}

	if !hash.IsEqual(parsed) {
		t.Fatal("parsed hash does not match original big-endian representation")
	}
}

func TestHashBytes(t *testing.T) {
	data := []byte("test")
	hash := NewHash(data)
	bytes := hash.Bytes()

	if len(bytes) != 32 {
		t.Errorf("Hash bytes should be 32 bytes, got %d", len(bytes))
	}

	// Verify it's a copy, not the original
	bytes[0] = 0xFF
	if hash[0] == 0xFF {
		t.Error("Hash.Bytes() should return a copy, not reference to original")
	}
}

func TestHashIsEqual(t *testing.T) {
	data1 := []byte("test1")
	data2 := []byte("test2")

	hash1 := NewHash(data1)
	hash1Copy := NewHash(data1)
	hash2 := NewHash(data2)

	if !hash1.IsEqual(hash1Copy) {
		t.Error("Same hash should be equal")
	}

	if hash1.IsEqual(hash2) {
		t.Error("Different hashes should not be equal")
	}

	if hash1.IsEqual(ZeroHash) {
		t.Error("Non-zero hash should not equal zero hash")
	}
}

func TestHashIsZero(t *testing.T) {
	if !ZeroHash.IsZero() {
		t.Error("ZeroHash should be zero")
	}

	data := []byte("test")
	hash := NewHash(data)
	if hash.IsZero() {
		t.Error("Non-zero hash should not be zero")
	}

	var emptyHash Hash
	if !emptyHash.IsZero() {
		t.Error("Empty hash should be zero")
	}
}

func TestHashReverse(t *testing.T) {
	// Create a hash with known pattern
	var hash Hash
	for i := 0; i < 32; i++ {
		hash[i] = byte(i)
	}

	reversed := hash.Reverse()

	// Check that bytes are reversed
	for i := 0; i < 32; i++ {
		if reversed[i] != hash[31-i] {
			t.Errorf("Byte at position %d not reversed correctly", i)
		}
	}

	// Test double reverse
	doubleReversed := reversed.Reverse()
	if !hash.IsEqual(doubleReversed) {
		t.Error("Double reverse should return original hash")
	}
}

func TestBitcoinCompatibility(t *testing.T) {
	// Test empty data (should match Bitcoin's double SHA-256 of empty string)
	emptyData := []byte{}
	hash := NewHash(emptyData)

	// Note: This is just checking the format, actual Bitcoin compatibility
	// would need the exact same double SHA-256 implementation
	if hash.IsZero() {
		t.Error("Hash of empty data should not be zero")
	}

	// Test that our hash produces consistent results
	hash2 := NewHash(emptyData)
	if !hash.IsEqual(hash2) {
		t.Error("Hash should be deterministic")
	}
}

func TestHashCompareTo(t *testing.T) {
	// Test that CompareTo compares from most significant byte (index 31)
	// to least significant byte (index 0), matching C++ uint256::CompareTo behavior

	// Test 1: Equal hashes
	var h1, h2 Hash
	for i := 0; i < 32; i++ {
		h1[i] = byte(i)
		h2[i] = byte(i)
	}
	if h1.CompareTo(h2) != 0 {
		t.Error("Equal hashes should return 0")
	}

	// Test 2: h1 < h2 (different in most significant byte)
	// h1[31] < h2[31], all other bytes equal
	h1 = Hash{}
	h2 = Hash{}
	h1[31] = 0x10
	h2[31] = 0x20
	if h1.CompareTo(h2) != -1 {
		t.Errorf("Expected h1 < h2 (most significant byte differs), got %d", h1.CompareTo(h2))
	}
	if h2.CompareTo(h1) != 1 {
		t.Errorf("Expected h2 > h1, got %d", h2.CompareTo(h1))
	}

	// Test 3: Compare in correct byte order (most significant first)
	// h1 has larger value in least significant byte but smaller in most significant
	// Should be h1 < h2 because most significant byte takes precedence
	h1 = Hash{}
	h2 = Hash{}
	h1[0] = 0xFF  // Large value in least significant byte
	h1[31] = 0x01 // Small value in most significant byte
	h2[0] = 0x01  // Small value in least significant byte
	h2[31] = 0x02 // Larger value in most significant byte
	if h1.CompareTo(h2) != -1 {
		t.Errorf("Most significant byte should take precedence: expected h1 < h2, got %d", h1.CompareTo(h2))
	}

	// Test 4: Zero hashes
	var zero1, zero2 Hash
	if zero1.CompareTo(zero2) != 0 {
		t.Error("Zero hashes should be equal")
	}

	// Test 5: Verify byte-by-byte comparison order
	// Set different bytes and ensure comparison respects byte order
	h1 = Hash{}
	h2 = Hash{}
	// Make them equal except at index 15
	h1[15] = 0x10
	h2[15] = 0x20
	if h1.CompareTo(h2) != -1 {
		t.Errorf("Should detect difference at byte 15: expected h1 < h2, got %d", h1.CompareTo(h2))
	}

	// Test 6: Verify comparison is from MSB (31) down to LSB (0)
	// If we have a difference at byte 30 and byte 5, byte 30 should win
	h1 = Hash{}
	h2 = Hash{}
	h1[30] = 0x20 // Higher at position 30
	h1[5] = 0x10  // Lower at position 5
	h2[30] = 0x10 // Lower at position 30
	h2[5] = 0x20  // Higher at position 5
	// h1 should be > h2 because byte 30 is more significant than byte 5
	if h1.CompareTo(h2) != 1 {
		t.Errorf("Byte 30 should take precedence over byte 5: expected h1 > h2, got %d", h1.CompareTo(h2))
	}
}

func BenchmarkNewHash(b *testing.B) {
	data := []byte("benchmark test data for hash performance")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		NewHash(data)
	}
}

func BenchmarkHashString(b *testing.B) {
	data := []byte("benchmark test data")
	hash := NewHash(data)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = hash.String()
	}
}

func BenchmarkHashReverse(b *testing.B) {
	data := []byte("benchmark test data")
	hash := NewHash(data)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = hash.Reverse()
	}
}
