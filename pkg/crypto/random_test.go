package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestSecureRandom(t *testing.T) {
	// Test various lengths
	lengths := []int{1, 8, 16, 32, 64, 128, 256, 512, 1024}

	for _, length := range lengths {
		data, err := SecureRandom(length)
		if err != nil {
			t.Fatalf("SecureRandom(%d) failed: %v", length, err)
		}

		if len(data) != length {
			t.Errorf("Expected %d bytes, got %d", length, len(data))
		}
	}

	// Test that consecutive calls produce different results
	data1, err := SecureRandom(32)
	if err != nil {
		t.Fatalf("SecureRandom failed: %v", err)
	}

	data2, err := SecureRandom(32)
	if err != nil {
		t.Fatalf("SecureRandom failed: %v", err)
	}

	if bytes.Equal(data1, data2) {
		t.Error("SecureRandom should produce different results")
	}

	// Test invalid inputs
	_, err = SecureRandom(0)
	if err == nil {
		t.Error("SecureRandom should fail with zero length")
	}

	_, err = SecureRandom(-1)
	if err == nil {
		t.Error("SecureRandom should fail with negative length")
	}

	_, err = SecureRandom(2 * 1024 * 1024) // 2MB
	if err == nil {
		t.Error("SecureRandom should fail with excessive length")
	}
}

func TestGenerateSalt(t *testing.T) {
	// Test various salt lengths
	lengths := []int{8, 16, 32, 64, 128}

	for _, length := range lengths {
		salt, err := GenerateSalt(length)
		if err != nil {
			t.Fatalf("GenerateSalt(%d) failed: %v", length, err)
		}

		if len(salt) != length {
			t.Errorf("Expected salt length %d, got %d", length, len(salt))
		}
	}

	// Test that salts are different
	salt1, err := GenerateSalt(32)
	if err != nil {
		t.Fatalf("GenerateSalt failed: %v", err)
	}

	salt2, err := GenerateSalt(32)
	if err != nil {
		t.Fatalf("GenerateSalt failed: %v", err)
	}

	if bytes.Equal(salt1, salt2) {
		t.Error("GenerateSalt should produce different salts")
	}

	// Test invalid lengths
	_, err = GenerateSalt(7) // Less than 8
	if err == nil {
		t.Error("GenerateSalt should fail with length < 8")
	}

	_, err = GenerateSalt(300) // Greater than 256
	if err == nil {
		t.Error("GenerateSalt should fail with length > 256")
	}
}

func TestGenerateNonce(t *testing.T) {
	// Test nonce generation
	nonce1, err := GenerateNonce()
	if err != nil {
		t.Fatalf("GenerateNonce failed: %v", err)
	}

	nonce2, err := GenerateNonce()
	if err != nil {
		t.Fatalf("GenerateNonce failed: %v", err)
	}

	// Nonces should be different (extremely unlikely to be same)
	if nonce1 == nonce2 {
		t.Error("GenerateNonce produced identical nonces (very unlikely)")
	}

	// Test multiple nonces
	nonces := make(map[uint64]bool)
	for i := 0; i < 1000; i++ {
		nonce, err := GenerateNonce()
		if err != nil {
			t.Fatalf("GenerateNonce failed: %v", err)
		}

		if nonces[nonce] {
			t.Error("GenerateNonce produced duplicate nonce")
		}
		nonces[nonce] = true
	}
}

func TestGenerateSecureToken(t *testing.T) {
	// Test various token lengths
	lengths := []int{8, 16, 32, 64, 128, 256}

	for _, length := range lengths {
		token, err := GenerateSecureToken(length)
		if err != nil {
			t.Fatalf("GenerateSecureToken(%d) failed: %v", length, err)
		}

		if len(token) != length {
			t.Errorf("Expected token length %d, got %d", length, len(token))
		}

		// Check that token is hex
		for _, c := range token {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("Token contains non-hex character: %c", c)
			}
		}
	}

	// Test that tokens are different
	token1, err := GenerateSecureToken(32)
	if err != nil {
		t.Fatalf("GenerateSecureToken failed: %v", err)
	}

	token2, err := GenerateSecureToken(32)
	if err != nil {
		t.Fatalf("GenerateSecureToken failed: %v", err)
	}

	if token1 == token2 {
		t.Error("GenerateSecureToken should produce different tokens")
	}

	// Test invalid lengths
	_, err = GenerateSecureToken(0)
	if err == nil {
		t.Error("GenerateSecureToken should fail with zero length")
	}

	_, err = GenerateSecureToken(1000) // Too large
	if err == nil {
		t.Error("GenerateSecureToken should fail with excessive length")
	}
}

func TestGenerateSecureString(t *testing.T) {
	length := 32
	alphabet := AlphabetAlphaNumeric

	str, err := GenerateSecureString(length, alphabet)
	if err != nil {
		t.Fatalf("GenerateSecureString failed: %v", err)
	}

	if len(str) != length {
		t.Errorf("Expected string length %d, got %d", length, len(str))
	}

	// Check that all characters are from alphabet
	for _, c := range str {
		found := false
		for _, a := range alphabet {
			if c == a {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("String contains character not in alphabet: %c", c)
		}
	}

	// Test different alphabets
	alphabets := []string{
		AlphabetNumeric,
		AlphabetAlpha,
		AlphabetAlphaNumeric,
		AlphabetBase58,
		AlphabetHex,
		AlphabetSafe,
	}

	for _, alph := range alphabets {
		str, err := GenerateSecureString(16, alph)
		if err != nil {
			t.Fatalf("GenerateSecureString with alphabet %s failed: %v", alph[:10], err)
		}

		if len(str) != 16 {
			t.Error("Generated string has wrong length")
		}
	}

	// Test invalid inputs
	_, err = GenerateSecureString(0, alphabet)
	if err == nil {
		t.Error("GenerateSecureString should fail with zero length")
	}

	_, err = GenerateSecureString(10, "")
	if err == nil {
		t.Error("GenerateSecureString should fail with empty alphabet")
	}
}

func TestPredefinedTokenGenerators(t *testing.T) {
	length := 20

	// Test alphanumeric token
	token, err := GenerateAlphaNumericToken(length)
	if err != nil {
		t.Fatalf("GenerateAlphaNumericToken failed: %v", err)
	}
	if len(token) != length {
		t.Error("AlphaNumeric token has wrong length")
	}

	// Test Base58 token
	base58Token, err := GenerateBase58Token(length)
	if err != nil {
		t.Fatalf("GenerateBase58Token failed: %v", err)
	}
	if len(base58Token) != length {
		t.Error("Base58 token has wrong length")
	}

	// Test safe token
	safeToken, err := GenerateSafeToken(length)
	if err != nil {
		t.Fatalf("GenerateSafeToken failed: %v", err)
	}
	if len(safeToken) != length {
		t.Error("Safe token has wrong length")
	}

	// Tokens should be different
	if token == base58Token || token == safeToken || base58Token == safeToken {
		t.Error("Different token types should produce different results")
	}
}

func TestSecureRandomInt(t *testing.T) {
	// Test various ranges
	maxValues := []int{2, 10, 100, 1000, 10000}

	for _, max := range maxValues {
		for i := 0; i < 100; i++ { // Test multiple times
			val, err := SecureRandomInt(max)
			if err != nil {
				t.Fatalf("SecureRandomInt(%d) failed: %v", max, err)
			}

			if val < 0 || val >= max {
				t.Errorf("SecureRandomInt(%d) returned %d, should be in [0, %d)", max, val, max)
			}
		}
	}

	// Test invalid inputs
	_, err := SecureRandomInt(0)
	if err == nil {
		t.Error("SecureRandomInt should fail with zero max")
	}

	_, err = SecureRandomInt(-1)
	if err == nil {
		t.Error("SecureRandomInt should fail with negative max")
	}

	// Test max = 1 (should always return 0)
	for i := 0; i < 10; i++ {
		val, err := SecureRandomInt(1)
		if err != nil {
			t.Fatalf("SecureRandomInt(1) failed: %v", err)
		}
		if val != 0 {
			t.Errorf("SecureRandomInt(1) should always return 0, got %d", val)
		}
	}
}

func TestSecureRandomRange(t *testing.T) {
	// Test various ranges
	testRanges := []struct {
		min, max int
	}{
		{0, 10},
		{5, 15},
		{-10, 10},
		{100, 200},
	}

	for _, tr := range testRanges {
		for i := 0; i < 100; i++ { // Test multiple times
			val, err := SecureRandomRange(tr.min, tr.max)
			if err != nil {
				t.Fatalf("SecureRandomRange(%d, %d) failed: %v", tr.min, tr.max, err)
			}

			if val < tr.min || val > tr.max {
				t.Errorf("SecureRandomRange(%d, %d) returned %d, should be in [%d, %d]",
					tr.min, tr.max, val, tr.min, tr.max)
			}
		}
	}

	// Test equal min and max
	val, err := SecureRandomRange(5, 5)
	if err != nil {
		t.Fatalf("SecureRandomRange(5, 5) failed: %v", err)
	}
	if val != 5 {
		t.Errorf("SecureRandomRange(5, 5) should return 5, got %d", val)
	}

	// Test invalid range
	_, err = SecureRandomRange(10, 5)
	if err == nil {
		t.Error("SecureRandomRange should fail with min > max")
	}
}

func TestRandomnessQuality(t *testing.T) {
	// Generate random data for testing
	data, err := SecureRandom(1000)
	if err != nil {
		t.Fatalf("SecureRandom failed: %v", err)
	}

	// Test randomness
	result := TestRandomness(data)
	if result == nil {
		t.Fatal("TestRandomness returned nil")
	}

	// The result should generally be valid for good random data
	// But we won't enforce it strictly as random data can occasionally fail tests
	if !result.Valid {
		t.Logf("Randomness test failed (this can happen occasionally): %s", result.Message)
	}

	// Test with empty data
	emptyResult := TestRandomness([]byte{})
	if emptyResult.Valid {
		t.Error("TestRandomness should fail with empty data")
	}

	// Test with obviously bad data (all zeros)
	badData := make([]byte, 1000)
	badResult := TestRandomness(badData)
	if badResult.Valid {
		t.Error("TestRandomness should fail with all-zero data")
	}
}

func TestSecureRandomGenerator(t *testing.T) {
	// Create custom generator
	generator := NewSecureRandomGenerator(rand.Reader)

	// Test all methods work with custom generator
	data, err := generator.SecureRandom(32)
	if err != nil {
		t.Fatalf("Custom generator SecureRandom failed: %v", err)
	}
	if len(data) != 32 {
		t.Error("Custom generator returned wrong data length")
	}

	salt, err := generator.GenerateSalt(16)
	if err != nil {
		t.Fatalf("Custom generator GenerateSalt failed: %v", err)
	}
	if len(salt) != 16 {
		t.Error("Custom generator returned wrong salt length")
	}

	nonce, err := generator.GenerateNonce()
	if err != nil {
		t.Fatalf("Custom generator GenerateNonce failed: %v", err)
	}
	if nonce == 0 {
		t.Log("Custom generator nonce is zero (unlikely but possible)")
	}

	token, err := generator.GenerateSecureToken(24)
	if err != nil {
		t.Fatalf("Custom generator GenerateSecureToken failed: %v", err)
	}
	if len(token) != 24 {
		t.Error("Custom generator returned wrong token length")
	}

	// Test that custom generator produces different results from default
	defaultData, err := SecureRandom(32)
	if err != nil {
		t.Fatalf("Default SecureRandom failed: %v", err)
	}

	customData, err := generator.SecureRandom(32)
	if err != nil {
		t.Fatalf("Custom SecureRandom failed: %v", err)
	}

	if bytes.Equal(defaultData, customData) {
		t.Log("Default and custom generators produced same data (unlikely but possible)")
	}
}

func TestSeedAndEntropyGeneration(t *testing.T) {
	// Test seed generation
	seed, err := GenerateSeed()
	if err != nil {
		t.Fatalf("GenerateSeed failed: %v", err)
	}

	if len(seed) != 32 {
		t.Errorf("Seed should be 32 bytes, got %d", len(seed))
	}

	// Test entropy generation
	entropy128, err := GenerateEntropy(128)
	if err != nil {
		t.Fatalf("GenerateEntropy(128) failed: %v", err)
	}

	if len(entropy128) != 16 { // 128 bits = 16 bytes
		t.Errorf("128-bit entropy should be 16 bytes, got %d", len(entropy128))
	}

	entropy256, err := GenerateEntropy(256)
	if err != nil {
		t.Fatalf("GenerateEntropy(256) failed: %v", err)
	}

	if len(entropy256) != 32 { // 256 bits = 32 bytes
		t.Errorf("256-bit entropy should be 32 bytes, got %d", len(entropy256))
	}

	// Test invalid entropy requests
	_, err = GenerateEntropy(127) // Not divisible by 8
	if err == nil {
		t.Error("GenerateEntropy should fail with bits not divisible by 8")
	}

	_, err = GenerateEntropy(0)
	if err == nil {
		t.Error("GenerateEntropy should fail with zero bits")
	}

	_, err = GenerateEntropy(5000) // Too large
	if err == nil {
		t.Error("GenerateEntropy should fail with excessive bits")
	}
}

func BenchmarkSecureRandom(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := SecureRandom(32)
		if err != nil {
			b.Fatalf("SecureRandom failed: %v", err)
		}
	}
}

func BenchmarkGenerateNonce(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GenerateNonce()
		if err != nil {
			b.Fatalf("GenerateNonce failed: %v", err)
		}
	}
}

func BenchmarkGenerateSecureToken(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GenerateSecureToken(32)
		if err != nil {
			b.Fatalf("GenerateSecureToken failed: %v", err)
		}
	}
}

func BenchmarkSecureRandomInt(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := SecureRandomInt(1000)
		if err != nil {
			b.Fatalf("SecureRandomInt failed: %v", err)
		}
	}
}