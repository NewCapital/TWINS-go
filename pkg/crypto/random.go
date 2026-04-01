package crypto

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
)

// SecureRandomGenerator provides cryptographically secure random number generation
type SecureRandomGenerator struct {
	reader io.Reader
	mutex  sync.Mutex
}

// DefaultGenerator is the default secure random generator
var DefaultGenerator = &SecureRandomGenerator{
	reader: rand.Reader,
}

// SecureRandom generates cryptographically secure random bytes
func SecureRandom(length int) ([]byte, error) {
	return DefaultGenerator.SecureRandom(length)
}

// SecureRandom generates cryptographically secure random bytes using this generator
func (g *SecureRandomGenerator) SecureRandom(length int) ([]byte, error) {
	if length <= 0 {
		return nil, fmt.Errorf("length must be positive, got %d", length)
	}

	if length > 1024*1024 { // 1MB limit for safety
		return nil, fmt.Errorf("length too large: %d bytes (max 1MB)", length)
	}

	bytes := make([]byte, length)

	g.mutex.Lock()
	defer g.mutex.Unlock()

	n, err := io.ReadFull(g.reader, bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to read random bytes: %v", err)
	}

	if n != length {
		return nil, fmt.Errorf("insufficient random bytes: expected %d, got %d", length, n)
	}

	return bytes, nil
}

// SecureRandomReader returns a cryptographically secure random reader
func SecureRandomReader() io.Reader {
	return rand.Reader
}

// SecureRandomReader returns this generator's random reader
func (g *SecureRandomGenerator) SecureRandomReader() io.Reader {
	return g.reader
}

// GenerateSalt generates a random salt for cryptographic operations
func GenerateSalt(length int) ([]byte, error) {
	if length < 8 {
		return nil, fmt.Errorf("salt length should be at least 8 bytes, got %d", length)
	}

	if length > 256 {
		return nil, fmt.Errorf("salt length too large: %d bytes (max 256)", length)
	}

	return SecureRandom(length)
}

// GenerateSalt generates a random salt using this generator
func (g *SecureRandomGenerator) GenerateSalt(length int) ([]byte, error) {
	if length < 8 {
		return nil, fmt.Errorf("salt length should be at least 8 bytes, got %d", length)
	}

	if length > 256 {
		return nil, fmt.Errorf("salt length too large: %d bytes (max 256)", length)
	}

	return g.SecureRandom(length)
}

// GenerateNonce generates a random nonce (number used once)
func GenerateNonce() (uint64, error) {
	return DefaultGenerator.GenerateNonce()
}

// GenerateNonce generates a random nonce using this generator
func (g *SecureRandomGenerator) GenerateNonce() (uint64, error) {
	bytes, err := g.SecureRandom(8)
	if err != nil {
		return 0, fmt.Errorf("failed to generate nonce: %v", err)
	}

	// Convert bytes to uint64 (little-endian)
	nonce := uint64(bytes[0]) |
		uint64(bytes[1])<<8 |
		uint64(bytes[2])<<16 |
		uint64(bytes[3])<<24 |
		uint64(bytes[4])<<32 |
		uint64(bytes[5])<<40 |
		uint64(bytes[6])<<48 |
		uint64(bytes[7])<<56

	return nonce, nil
}

// GenerateSecureToken generates a cryptographically secure token string
func GenerateSecureToken(length int) (string, error) {
	return DefaultGenerator.GenerateSecureToken(length)
}

// GenerateSecureToken generates a secure token using this generator
func (g *SecureRandomGenerator) GenerateSecureToken(length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("token length must be positive, got %d", length)
	}

	if length > 512 {
		return "", fmt.Errorf("token length too large: %d (max 512)", length)
	}

	// Generate random bytes (half the desired hex string length)
	byteLength := (length + 1) / 2
	bytes, err := g.SecureRandom(byteLength)
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %v", err)
	}

	// Convert to hex string
	token := hex.EncodeToString(bytes)

	// Truncate to exact length if needed
	if len(token) > length {
		token = token[:length]
	}

	return token, nil
}

// GenerateSecureString generates a random string using a custom alphabet
func GenerateSecureString(length int, alphabet string) (string, error) {
	return DefaultGenerator.GenerateSecureString(length, alphabet)
}

// GenerateSecureString generates a random string using this generator
func (g *SecureRandomGenerator) GenerateSecureString(length int, alphabet string) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("string length must be positive, got %d", length)
	}

	if length > 1024 {
		return "", fmt.Errorf("string length too large: %d (max 1024)", length)
	}

	if len(alphabet) == 0 {
		return "", fmt.Errorf("alphabet cannot be empty")
	}

	if len(alphabet) > 256 {
		return "", fmt.Errorf("alphabet too large: %d characters (max 256)", len(alphabet))
	}

	bytes, err := g.SecureRandom(length)
	if err != nil {
		return "", fmt.Errorf("failed to generate random string: %v", err)
	}

	result := make([]byte, length)
	for i, b := range bytes {
		result[i] = alphabet[int(b)%len(alphabet)]
	}

	return string(result), nil
}

// Predefined alphabets for secure string generation
const (
	AlphabetNumeric      = "0123456789"
	AlphabetAlpha        = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	AlphabetAlphaNumeric = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	AlphabetBase58       = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	AlphabetHex          = "0123456789abcdef"
	AlphabetSafe         = "23456789abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ" // Excludes ambiguous chars
)

// GenerateAlphaNumericToken generates an alphanumeric token
func GenerateAlphaNumericToken(length int) (string, error) {
	return DefaultGenerator.GenerateSecureString(length, AlphabetAlphaNumeric)
}

// GenerateBase58Token generates a Base58 token (Bitcoin-like)
func GenerateBase58Token(length int) (string, error) {
	return DefaultGenerator.GenerateSecureString(length, AlphabetBase58)
}

// GenerateSafeToken generates a token using safe characters (no ambiguous chars)
func GenerateSafeToken(length int) (string, error) {
	return DefaultGenerator.GenerateSecureString(length, AlphabetSafe)
}

// Secure random number generation for specific ranges

// SecureRandomInt generates a secure random integer in range [0, max)
func SecureRandomInt(max int) (int, error) {
	return DefaultGenerator.SecureRandomInt(max)
}

// SecureRandomInt generates a secure random integer using this generator
func (g *SecureRandomGenerator) SecureRandomInt(max int) (int, error) {
	if max <= 0 {
		return 0, fmt.Errorf("max must be positive, got %d", max)
	}

	if max == 1 {
		return 0, nil
	}

	// Calculate the number of bytes needed
	byteCount := 1
	temp := max - 1
	for temp > 255 {
		temp >>= 8
		byteCount++
	}

	if byteCount > 8 {
		return 0, fmt.Errorf("max too large: %d", max)
	}

	// Generate uniform random number in range
	for {
		bytes, err := g.SecureRandom(byteCount)
		if err != nil {
			return 0, fmt.Errorf("failed to generate random int: %v", err)
		}

		// Convert bytes to integer
		result := 0
		for i, b := range bytes {
			result |= int(b) << (8 * i)
		}

		// Check if result is in our range (avoids modulo bias)
		limit := (1 << (8 * byteCount)) / max * max
		if result < limit {
			return result % max, nil
		}

		// If result >= limit, try again to avoid bias
	}
}

// SecureRandomRange generates a secure random integer in range [min, max]
func SecureRandomRange(min, max int) (int, error) {
	return DefaultGenerator.SecureRandomRange(min, max)
}

// SecureRandomRange generates a secure random integer using this generator
func (g *SecureRandomGenerator) SecureRandomRange(min, max int) (int, error) {
	if min > max {
		return 0, fmt.Errorf("min (%d) must be <= max (%d)", min, max)
	}

	if min == max {
		return min, nil
	}

	rangeSize := max - min + 1
	result, err := g.SecureRandomInt(rangeSize)
	if err != nil {
		return 0, err
	}

	return min + result, nil
}

// Entropy testing and validation

// TestRandomness performs basic randomness tests on generated data
func TestRandomness(data []byte) *RandomnessResult {
	if len(data) == 0 {
		return &RandomnessResult{
			Valid:   false,
			Message: "No data provided",
		}
	}

	// Chi-square test for uniform distribution
	chiSquare := calculateChiSquare(data)

	// Frequency test
	freq := calculateFrequency(data)

	// Run test
	runs := calculateRuns(data)

	// Simple thresholds (in production, use proper statistical tests)
	valid := true
	var messages []string

	if chiSquare > 300 { // Simplified threshold
		valid = false
		messages = append(messages, fmt.Sprintf("Chi-square test failed: %.2f", chiSquare))
	}

	if freq < 0.45 || freq > 0.55 { // Should be close to 0.5 for uniform bits
		valid = false
		messages = append(messages, fmt.Sprintf("Frequency test failed: %.3f", freq))
	}

	if runs < len(data)/4 || runs > 3*len(data)/4 { // Simplified run test
		valid = false
		messages = append(messages, fmt.Sprintf("Run test failed: %d runs", runs))
	}

	message := "Randomness tests passed"
	if !valid {
		message = fmt.Sprintf("Randomness tests failed: %v", messages)
	}

	return &RandomnessResult{
		Valid:     valid,
		Message:   message,
		ChiSquare: chiSquare,
		Frequency: freq,
		Runs:      runs,
	}
}

// RandomnessResult contains the results of randomness testing
type RandomnessResult struct {
	Valid     bool
	Message   string
	ChiSquare float64
	Frequency float64
	Runs      int
}

// Helper functions for randomness testing

func calculateChiSquare(data []byte) float64 {
	if len(data) == 0 {
		return 0
	}

	// Count frequency of each byte value
	counts := make([]int, 256)
	for _, b := range data {
		counts[b]++
	}

	// Expected frequency
	expected := float64(len(data)) / 256.0

	// Calculate chi-square statistic
	chiSquare := 0.0
	for _, count := range counts {
		diff := float64(count) - expected
		chiSquare += (diff * diff) / expected
	}

	return chiSquare
}

func calculateFrequency(data []byte) float64 {
	if len(data) == 0 {
		return 0
	}

	ones := 0
	for _, b := range data {
		for i := 0; i < 8; i++ {
			if (b>>i)&1 == 1 {
				ones++
			}
		}
	}

	totalBits := len(data) * 8
	return float64(ones) / float64(totalBits)
}

func calculateRuns(data []byte) int {
	if len(data) == 0 {
		return 0
	}

	runs := 1
	for i := 1; i < len(data); i++ {
		if data[i] != data[i-1] {
			runs++
		}
	}

	return runs
}

// Seed generation for deterministic operations

// GenerateSeed generates a seed suitable for deterministic operations
func GenerateSeed() ([]byte, error) {
	return DefaultGenerator.GenerateSeed()
}

// GenerateSeed generates a seed using this generator
func (g *SecureRandomGenerator) GenerateSeed() ([]byte, error) {
	return g.SecureRandom(32) // 256-bit seed
}

// GenerateEntropy generates high-entropy bytes for cryptographic operations
func GenerateEntropy(bits int) ([]byte, error) {
	return DefaultGenerator.GenerateEntropy(bits)
}

// GenerateEntropy generates high-entropy bytes using this generator
func (g *SecureRandomGenerator) GenerateEntropy(bits int) ([]byte, error) {
	if bits <= 0 || bits%8 != 0 {
		return nil, fmt.Errorf("entropy bits must be positive and divisible by 8, got %d", bits)
	}

	if bits > 4096 { // 512 bytes max
		return nil, fmt.Errorf("entropy bits too large: %d (max 4096)", bits)
	}

	bytes := bits / 8
	return g.SecureRandom(bytes)
}

// NewSecureRandomGenerator creates a new secure random generator with custom reader
func NewSecureRandomGenerator(reader io.Reader) *SecureRandomGenerator {
	return &SecureRandomGenerator{
		reader: reader,
	}
}

// SetRandomReader sets a custom random reader for the default generator
func SetRandomReader(reader io.Reader) {
	DefaultGenerator.mutex.Lock()
	defer DefaultGenerator.mutex.Unlock()
	DefaultGenerator.reader = reader
}