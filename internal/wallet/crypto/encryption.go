// Package crypto provides wallet encryption compatible with legacy TWINS C++ client
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"fmt"

	"golang.org/x/crypto/scrypt"
)

var (
	// ErrInvalidKeySize indicates an invalid encryption key size
	ErrInvalidKeySize = errors.New("invalid key size, must be 32 bytes")
	// ErrInvalidIVSize indicates an invalid IV size
	ErrInvalidIVSize = errors.New("invalid IV size, must be 16 bytes")
	// ErrInvalidPadding indicates invalid PKCS#7 padding
	ErrInvalidPadding = errors.New("invalid padding")
	// ErrDecryptionFailed indicates decryption failed
	ErrDecryptionFailed = errors.New("decryption failed")
)

// Derivation methods matching legacy C++ CMasterKey
const (
	DerivationMethodEVPSHA512 = 0 // EVP_sha512 (legacy)
	DerivationMethodScrypt    = 1 // scrypt (newer)
)

// Default parameters
const (
	DefaultEVPIterations = 25000 // Default iterations for EVP_sha512
	DefaultScryptN       = 32768 // scrypt N parameter (CPU/memory cost)
	DefaultScryptR       = 8     // scrypt r parameter (block size)
	DefaultScryptP       = 1     // scrypt p parameter (parallelization)
	KeySize              = 32    // AES-256 key size
	IVSize               = 16    // AES block size
	SaltSize             = 8     // Salt size for key derivation
)

// DeriveKeyEVPSHA512 derives a key/IV pair using EVP_sha512 (legacy method)
// Matches OpenSSL's EVP_BytesToKey(passphrase, salt, iterations, SHA512)
func DeriveKeyEVPSHA512(passphrase []byte, salt []byte, iterations uint32) ([]byte, []byte, error) {
	if len(salt) != SaltSize {
		return nil, nil, fmt.Errorf("salt must be %d bytes", SaltSize)
	}

	if iterations == 0 {
		iterations = 1
	}

	required := KeySize + IVSize
	result := make([]byte, 0, ((required+sha512.Size-1)/sha512.Size)*sha512.Size)
	var prev []byte

	for len(result) < required {
		// d_i = SHA512^iterations(d_{i-1} || passphrase || salt)
		h := sha512.New()
		if len(prev) > 0 {
			h.Write(prev)
		}
		h.Write(passphrase)
		h.Write(salt)
		digest := h.Sum(nil)

		for i := uint32(1); i < iterations; i++ {
			sum := sha512.Sum512(digest)
			digest = sum[:]
		}

		result = append(result, digest...)
		prev = digest
	}

	key := make([]byte, KeySize)
	iv := make([]byte, IVSize)
	copy(key, result[:KeySize])
	copy(iv, result[KeySize:KeySize+IVSize])

	return key, iv, nil
}

// DeriveKeyScrypt derives a key/IV pair using scrypt
func DeriveKeyScrypt(passphrase []byte, salt []byte, N, r, p int) ([]byte, []byte, error) {
	if len(salt) != SaltSize {
		return nil, nil, fmt.Errorf("salt must be %d bytes", SaltSize)
	}

	derivedLen := KeySize + IVSize
	material, err := scrypt.Key(passphrase, salt, N, r, p, derivedLen)
	if err != nil {
		return nil, nil, fmt.Errorf("scrypt derivation failed: %w", err)
	}

	key := make([]byte, KeySize)
	iv := make([]byte, IVSize)
	copy(key, material[:KeySize])
	copy(iv, material[KeySize:derivedLen])

	return key, iv, nil
}

// GenerateIV generates an IV from a public key using double-SHA256
// This matches the C++ wallet's IV generation: SHA256(SHA256(pubkey))
func GenerateIV(pubkey []byte) []byte {
	// First SHA256
	hash1 := sha256.Sum256(pubkey)
	// Second SHA256
	hash2 := sha256.Sum256(hash1[:])
	// Return first 16 bytes for AES IV
	return hash2[:IVSize]
}

// EncryptAES256CBC encrypts data using AES-256-CBC with PKCS#7 padding
// This matches the legacy C++ wallet encryption
func EncryptAES256CBC(key, iv, plaintext []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKeySize
	}
	if len(iv) != IVSize {
		return nil, ErrInvalidIVSize
	}

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Apply PKCS#7 padding
	paddedPlaintext := pkcs7Pad(plaintext, aes.BlockSize)

	// Create CBC encrypter
	ciphertext := make([]byte, len(paddedPlaintext))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, paddedPlaintext)

	return ciphertext, nil
}

// DecryptAES256CBC decrypts data using AES-256-CBC and removes PKCS#7 padding
func DecryptAES256CBC(key, iv, ciphertext []byte) ([]byte, error) {
	// Support both AES-128 (16 bytes) and AES-256 (32 bytes) for legacy compatibility
	if len(key) != 16 && len(key) != KeySize {
		return nil, ErrInvalidKeySize
	}
	if len(iv) != IVSize {
		return nil, ErrInvalidIVSize
	}
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext length must be multiple of block size")
	}

	// Create AES cipher (automatically selects AES-128 or AES-256 based on key length)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create CBC decrypter
	plaintext := make([]byte, len(ciphertext))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(plaintext, ciphertext)

	// Remove PKCS#7 padding
	unpaddedPlaintext, err := pkcs7Unpad(plaintext, aes.BlockSize)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return unpaddedPlaintext, nil
}

// DecryptWithoutPaddingRemoval decrypts data without removing padding (for debugging)
func DecryptWithoutPaddingRemoval(key, iv, ciphertext []byte) ([]byte, error) {
	// Support both AES-128 (16 bytes) and AES-256 (32 bytes) for legacy compatibility
	if len(key) != 16 && len(key) != KeySize {
		return nil, ErrInvalidKeySize
	}
	if len(iv) != IVSize {
		return nil, ErrInvalidIVSize
	}
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext length must be multiple of block size")
	}

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create CBC decrypter
	plaintext := make([]byte, len(ciphertext))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(plaintext, ciphertext)

	return plaintext, nil
}

// pkcs7Pad applies PKCS#7 padding
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padText := make([]byte, padding)
	for i := range padText {
		padText[i] = byte(padding)
	}
	return append(data, padText...)
}

// pkcs7Unpad removes PKCS#7 padding using constant-time verification
func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 {
		return nil, ErrInvalidPadding
	}

	padding := int(data[len(data)-1])
	if padding > blockSize || padding == 0 {
		return nil, ErrInvalidPadding
	}

	// Verify padding in constant time (prevents timing attacks)
	var invalid byte
	for i := len(data) - padding; i < len(data); i++ {
		// XOR with expected value - will be 0 if correct, non-zero if incorrect
		invalid |= data[i] ^ byte(padding)
	}

	// Check if any byte was incorrect (constant time)
	if invalid != 0 {
		return nil, ErrInvalidPadding
	}

	return data[:len(data)-padding], nil
}

// MasterKey represents a wallet master encryption key
// Compatible with legacy C++ CMasterKey
type MasterKey struct {
	EncryptedKey              []byte // Encrypted 32-byte key
	Salt                      []byte // 8-byte salt
	DerivationMethod          uint32 // 0=EVP_sha512, 1=scrypt
	DeriveIterations          uint32 // Iteration count
	OtherDerivationParameters []byte // Additional params (for scrypt)
}

// NewMasterKeyEVP creates a master key using EVP_sha512 derivation
func NewMasterKeyEVP(passphrase []byte, iterations uint32) (*MasterKey, []byte, error) {
	// Generate random salt
	salt := make([]byte, SaltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, nil, fmt.Errorf("failed to generate random salt: %w", err)
	}

	// Generate random encryption key
	encryptionKey := make([]byte, KeySize)
	if _, err := rand.Read(encryptionKey); err != nil {
		return nil, nil, fmt.Errorf("failed to generate random encryption key: %w", err)
	}

	// Derive key from passphrase
	derivedKey, _, err := DeriveKeyEVPSHA512(passphrase, salt, iterations)
	if err != nil {
		return nil, nil, err
	}

	// Encrypt the encryption key with derived key
	iv := make([]byte, IVSize)
	if _, err := rand.Read(iv); err != nil {
		return nil, nil, fmt.Errorf("failed to generate random IV: %w", err)
	}

	encryptedKey, err := EncryptAES256CBC(derivedKey, iv, encryptionKey)
	if err != nil {
		return nil, nil, err
	}

	mk := &MasterKey{
		EncryptedKey:     append(iv, encryptedKey...), // Prepend IV
		Salt:             salt,
		DerivationMethod: DerivationMethodEVPSHA512,
		DeriveIterations: iterations,
	}

	return mk, encryptionKey, nil
}

// NewMasterKeyScrypt creates a master key using scrypt derivation
func NewMasterKeyScrypt(passphrase []byte, N, r, p int) (*MasterKey, []byte, error) {
	// Generate random salt
	salt := make([]byte, SaltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, nil, fmt.Errorf("failed to generate random salt: %w", err)
	}

	// Generate random encryption key
	encryptionKey := make([]byte, KeySize)
	if _, err := rand.Read(encryptionKey); err != nil {
		return nil, nil, fmt.Errorf("failed to generate random encryption key: %w", err)
	}

	// Derive key from passphrase
	derivedKey, _, err := DeriveKeyScrypt(passphrase, salt, N, r, p)
	if err != nil {
		return nil, nil, err
	}

	// Encrypt the encryption key with derived key
	iv := make([]byte, IVSize)
	if _, err := rand.Read(iv); err != nil {
		return nil, nil, fmt.Errorf("failed to generate random IV: %w", err)
	}

	encryptedKey, err := EncryptAES256CBC(derivedKey, iv, encryptionKey)
	if err != nil {
		return nil, nil, err
	}

	// Encode scrypt parameters
	params := encodeScryptParams(N, r, p)

	mk := &MasterKey{
		EncryptedKey:              append(iv, encryptedKey...), // Prepend IV
		Salt:                      salt,
		DerivationMethod:          DerivationMethodScrypt,
		DeriveIterations:          1, // Not used for scrypt
		OtherDerivationParameters: params,
	}

	return mk, encryptionKey, nil
}

// WrapEncryptionKeyScrypt wraps an existing encryption key with a new passphrase using scrypt.
// Unlike NewMasterKeyScrypt which generates a fresh random key, this re-wraps the same key
// so that all data encrypted with it remains valid after a passphrase change.
func WrapEncryptionKeyScrypt(encryptionKey, passphrase []byte, N, r, p int) (*MasterKey, error) {
	if len(encryptionKey) == 0 {
		return nil, fmt.Errorf("encryption key cannot be empty")
	}

	// Generate random salt
	salt := make([]byte, SaltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("failed to generate random salt: %w", err)
	}

	// Derive key from new passphrase
	derivedKey, _, err := DeriveKeyScrypt(passphrase, salt, N, r, p)
	if err != nil {
		return nil, err
	}
	defer func() {
		for i := range derivedKey {
			derivedKey[i] = 0
		}
	}()

	// Encrypt the existing encryption key with new derived key
	iv := make([]byte, IVSize)
	if _, err := rand.Read(iv); err != nil {
		return nil, fmt.Errorf("failed to generate random IV: %w", err)
	}

	encryptedKey, err := EncryptAES256CBC(derivedKey, iv, encryptionKey)
	if err != nil {
		return nil, err
	}

	params := encodeScryptParams(N, r, p)

	mk := &MasterKey{
		EncryptedKey:              append(iv, encryptedKey...), // Prepend IV
		Salt:                      salt,
		DerivationMethod:          DerivationMethodScrypt,
		DeriveIterations:          1, // Not used for scrypt
		OtherDerivationParameters: params,
	}

	return mk, nil
}

// Unlock decrypts the master key using a passphrase
func (mk *MasterKey) Unlock(passphrase []byte) ([]byte, error) {
	var derivedKey []byte
	var derivedIV []byte
	var err error

	// Ensure derived key is securely zeroed after use
	defer func() {
		if derivedKey != nil {
			for i := range derivedKey {
				derivedKey[i] = 0
			}
		}
	}()

	// Derive key based on method
	switch mk.DerivationMethod {
	case DerivationMethodEVPSHA512:
		derivedKey, derivedIV, err = DeriveKeyEVPSHA512(passphrase, mk.Salt, mk.DeriveIterations)
	case DerivationMethodScrypt:
		N, r, p := decodeScryptParams(mk.OtherDerivationParameters)
		derivedKey, derivedIV, err = DeriveKeyScrypt(passphrase, mk.Salt, N, r, p)
	default:
		return nil, fmt.Errorf("unsupported derivation method: %d", mk.DerivationMethod)
	}

	if err != nil {
		return nil, err
	}

	if len(mk.EncryptedKey) == 0 {
		return nil, fmt.Errorf("encrypted key missing")
	}

	var iv []byte
	ciphertext := mk.EncryptedKey

	if len(mk.EncryptedKey) == 48 {
		// Legacy layout: ciphertext only, IV derived from passphrase
		if len(derivedIV) < IVSize {
			return nil, fmt.Errorf("derived IV too short")
		}
		iv = derivedIV[:IVSize]
	} else {
		if len(mk.EncryptedKey) < IVSize {
			return nil, fmt.Errorf("encrypted key too short")
		}
		iv = mk.EncryptedKey[:IVSize]
		ciphertext = mk.EncryptedKey[IVSize:]
	}

	// Decrypt the encryption key
	encryptionKey, err := DecryptAES256CBC(derivedKey, iv, ciphertext)
	if err != nil {
		return nil, fmt.Errorf("failed to unlock master key: %w", err)
	}

	// Legacy wallets use 16-byte AES-128 key, modern wallets use 32-byte AES-256 key
	// Both are valid and supported
	return encryptionKey, nil
}

// encodeScryptParams encodes scrypt parameters into bytes
func encodeScryptParams(N, r, p int) []byte {
	params := make([]byte, 12)
	// N (4 bytes, little-endian)
	params[0] = byte(N)
	params[1] = byte(N >> 8)
	params[2] = byte(N >> 16)
	params[3] = byte(N >> 24)
	// r (4 bytes, little-endian)
	params[4] = byte(r)
	params[5] = byte(r >> 8)
	params[6] = byte(r >> 16)
	params[7] = byte(r >> 24)
	// p (4 bytes, little-endian)
	params[8] = byte(p)
	params[9] = byte(p >> 8)
	params[10] = byte(p >> 16)
	params[11] = byte(p >> 24)
	return params
}

// decodeScryptParams decodes scrypt parameters from bytes
func decodeScryptParams(params []byte) (N, r, p int) {
	if len(params) < 12 {
		return DefaultScryptN, DefaultScryptR, DefaultScryptP
	}
	N = int(params[0]) | int(params[1])<<8 | int(params[2])<<16 | int(params[3])<<24
	r = int(params[4]) | int(params[5])<<8 | int(params[6])<<16 | int(params[7])<<24
	p = int(params[8]) | int(params[9])<<8 | int(params[10])<<16 | int(params[11])<<24
	return
}

// EncryptSecret encrypts data using AES-256-CBC with a custom IV
// This matches the C++ wallet's EncryptSecret function
func EncryptSecret(key []byte, plaintext []byte, iv []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKeySize
	}
	if len(iv) != IVSize {
		return nil, ErrInvalidIVSize
	}

	return EncryptAES256CBC(key, iv, plaintext)
}

// DecryptSecret decrypts data using AES-256-CBC with a custom IV
// This matches the C++ wallet's DecryptSecret function
func DecryptSecret(key []byte, ciphertext []byte, iv []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKeySize
	}
	if len(iv) != IVSize {
		return nil, ErrInvalidIVSize
	}

	return DecryptAES256CBC(key, iv, ciphertext)
}
