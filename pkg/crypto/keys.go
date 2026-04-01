package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
)

// PrivateKey represents a secp256k1 private key (Bitcoin-compatible)
type PrivateKey struct {
	key *btcec.PrivateKey
}

// PublicKey represents a secp256k1 public key (Bitcoin-compatible)
type PublicKey struct {
	key *btcec.PublicKey
}

// KeyPair represents an ECDSA key pair
type KeyPair struct {
	Private *PrivateKey
	Public  *PublicKey
}

// Ed25519KeyPair represents an Ed25519 key pair
type Ed25519KeyPair struct {
	Private ed25519.PrivateKey
	Public  ed25519.PublicKey
}

// NewPublicKeyFromBTCEC creates a PublicKey wrapper from a btcec.PublicKey
func NewPublicKeyFromBTCEC(pubKey *btcec.PublicKey) *PublicKey {
	return &PublicKey{key: pubKey}
}

// NewPrivateKeyFromBTCEC creates a PrivateKey wrapper from a btcec.PrivateKey
func NewPrivateKeyFromBTCEC(privKey *btcec.PrivateKey) *PrivateKey {
	return &PrivateKey{key: privKey}
}

// GenerateKeyPair generates a new secp256k1 key pair for Bitcoin/TWINS compatibility
func GenerateKeyPair() (*KeyPair, error) {
	// Use secp256k1 curve (Bitcoin standard)
	privateKey, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate secp256k1 key pair: %v", err)
	}

	return &KeyPair{
		Private: &PrivateKey{key: privateKey},
		Public:  &PublicKey{key: privateKey.PubKey()},
	}, nil
}

// GenerateKeyPairFromSeed generates a deterministic secp256k1 key pair from a seed
func GenerateKeyPairFromSeed(seed []byte) (*KeyPair, error) {
	if len(seed) < 32 {
		return nil, fmt.Errorf("seed must be at least 32 bytes")
	}

	// Use seed to generate deterministic private key
	hash := sha256.Sum256(seed)

	// Create private key from seed (btcec handles curve validation)
	privateKey, _ := btcec.PrivKeyFromBytes(hash[:])

	return &KeyPair{
		Private: &PrivateKey{key: privateKey},
		Public:  &PublicKey{key: privateKey.PubKey()},
	}, nil
}

// GenerateEd25519KeyPair generates a new Ed25519 key pair
func GenerateEd25519KeyPair() (*Ed25519KeyPair, error) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Ed25519 key pair: %v", err)
	}

	return &Ed25519KeyPair{
		Private: private,
		Public:  public,
	}, nil
}

// Bytes returns the private key as bytes (32 bytes)
func (pk *PrivateKey) Bytes() []byte {
	return pk.key.Serialize()
}

// Hex returns the private key as a hex string
func (pk *PrivateKey) Hex() string {
	return hex.EncodeToString(pk.Bytes())
}

// Sign signs a message hash using the private key (secp256k1)
func (pk *PrivateKey) Sign(hash []byte) (*Signature, error) {
	// Use btcec/v2/ecdsa compact signing (includes recovery ID)
	compactSig := ecdsa.SignCompact(pk.key, hash, true)

	// Compact format: [recovery_id] + [r] + [s] (65 bytes total)
	// Skip first byte (recovery ID) and parse R and S
	if len(compactSig) != 65 {
		return nil, fmt.Errorf("invalid compact signature length: %d", len(compactSig))
	}

	r := new(big.Int).SetBytes(compactSig[1:33])
	s := new(big.Int).SetBytes(compactSig[33:65])

	return &Signature{R: r, S: s}, nil
}

// PublicKey returns the corresponding public key
func (pk *PrivateKey) PublicKey() *PublicKey {
	return &PublicKey{key: pk.key.PubKey()}
}

// Bytes returns the public key as uncompressed bytes (65 bytes)
func (pub *PublicKey) Bytes() []byte {
	// btcec provides SerializeUncompressed which returns 65 bytes (0x04 + x + y)
	return pub.key.SerializeUncompressed()
}

// CompressedBytes returns the public key as compressed bytes (33 bytes)
func (pub *PublicKey) CompressedBytes() []byte {
	// btcec provides SerializeCompressed which returns 33 bytes (0x02/0x03 + x)
	return pub.key.SerializeCompressed()
}

// SerializeCompressed returns the public key as compressed bytes (33 bytes)
// Alias for CompressedBytes() for Bitcoin/PIVX/TWINS compatibility
func (pub *PublicKey) SerializeCompressed() []byte {
	return pub.CompressedBytes()
}

// Hex returns the public key as a hex string
func (pub *PublicKey) Hex() string {
	return hex.EncodeToString(pub.Bytes())
}

// CompressedHex returns the compressed public key as a hex string
func (pub *PublicKey) CompressedHex() string {
	return hex.EncodeToString(pub.CompressedBytes())
}

// Verify verifies a signature against a message hash (secp256k1)
func (pub *PublicKey) Verify(hash []byte, sig *Signature) bool {
	// Convert to ModNScalar for btcec/v2
	var r, s btcec.ModNScalar
	r.SetByteSlice(sig.R.Bytes())
	s.SetByteSlice(sig.S.Bytes())

	// Create ecdsa signature from R and S
	signature := ecdsa.NewSignature(&r, &s)

	return signature.Verify(hash, pub.key)
}

// IsEqual checks if two public keys are equal
func (pub *PublicKey) IsEqual(other *PublicKey) bool {
	return pub.key.IsEqual(other.key)
}

// ParsePrivateKeyFromBytes parses a secp256k1 private key from bytes
func ParsePrivateKeyFromBytes(data []byte) (*PrivateKey, error) {
	if len(data) != 32 {
		return nil, fmt.Errorf("private key must be 32 bytes")
	}

	// btcec handles validation
	privateKey, _ := btcec.PrivKeyFromBytes(data)
	return &PrivateKey{key: privateKey}, nil
}

// ParsePrivateKeyFromHex parses a private key from a hex string
func ParsePrivateKeyFromHex(hexStr string) (*PrivateKey, error) {
	data, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("invalid hex string: %v", err)
	}

	return ParsePrivateKeyFromBytes(data)
}

// ParsePublicKeyFromBytes parses a secp256k1 public key from bytes
func ParsePublicKeyFromBytes(data []byte) (*PublicKey, error) {
	// btcec handles both compressed (33 bytes) and uncompressed (65 bytes) formats
	pubKey, err := btcec.ParsePubKey(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %v", err)
	}

	return &PublicKey{key: pubKey}, nil
}

// ParsePublicKeyFromHex parses a public key from a hex string
func ParsePublicKeyFromHex(hexStr string) (*PublicKey, error) {
	data, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("invalid hex string: %v", err)
	}

	return ParsePublicKeyFromBytes(data)
}

// IsValidPublicKey validates that a public key is a valid secp256k1 point
// Returns true if the key is valid (on curve), false otherwise
// Supports both compressed (33 bytes) and uncompressed (65 bytes) formats
func IsValidPublicKey(pubKeyBytes []byte) bool {
	_, err := btcec.ParsePubKey(pubKeyBytes)
	return err == nil
}

// PubKey is an alias for PublicKey() for convenience and legacy compatibility
func (pk *PrivateKey) PubKey() *PublicKey {
	return pk.PublicKey()
}

// DecodeWIF decodes a WIF (Wallet Import Format) private key string
// WIF format: base58check encoding of [1 byte prefix][32 bytes key][optional 1 byte compression flag][4 bytes checksum]
// For TWINS: mainnet prefix is 66 (0x42), testnet prefix is 237 (0xED)
func DecodeWIF(wifStr string) (*PrivateKey, error) {
	// Decode base58check
	decoded, err := Base58CheckDecode(wifStr)
	if err != nil {
		return nil, fmt.Errorf("invalid WIF encoding: %w", err)
	}

	// Check length: 1 byte version + 32 bytes key + optional 1 byte compression flag
	if len(decoded) != 33 && len(decoded) != 34 {
		return nil, fmt.Errorf("invalid WIF length: expected 33 or 34, got %d", len(decoded))
	}

	// Extract private key bytes (skip version byte)
	keyBytes := decoded[1:33]

	return ParsePrivateKeyFromBytes(keyBytes)
}

// EncodeWIF encodes a private key to WIF (Wallet Import Format) string
// compressed: if true, indicates the corresponding public key should be compressed
// version: network version byte (66/0x42 for TWINS mainnet, 237/0xED for testnet)
func (pk *PrivateKey) EncodeWIF(version byte, compressed bool) string {
	// Build payload: version + key + optional compression flag
	payload := make([]byte, 0, 34)
	payload = append(payload, version)
	payload = append(payload, pk.Bytes()...)
	if compressed {
		payload = append(payload, 0x01)
	}

	return Base58CheckEncode(payload)
}

// SignCompactHash creates a compact recoverable signature (65 bytes) from a raw hash
// Unlike SignCompact (which applies message magic), this signs the hash directly
// Used for masternode ping/broadcast signatures
func SignCompactHash(pk *PrivateKey, hash []byte) ([]byte, error) {
	// Use btcec/v2/ecdsa compact signing
	compactSig := ecdsa.SignCompact(pk.key, hash, true)

	// Verify length
	if len(compactSig) != 65 {
		return nil, fmt.Errorf("unexpected compact signature length: %d", len(compactSig))
	}

	return compactSig, nil
}

// RecoverPublicKeyFromHash recovers the public key from a compact signature and raw hash
func RecoverPublicKeyFromHash(signature []byte, hash []byte) (*PublicKey, error) {
	if len(signature) != 65 {
		return nil, fmt.Errorf("invalid compact signature length: expected 65, got %d", len(signature))
	}

	pubKey, _, err := ecdsa.RecoverCompact(signature, hash)
	if err != nil {
		return nil, fmt.Errorf("failed to recover public key: %w", err)
	}

	return &PublicKey{key: pubKey}, nil
}

// VerifyCompactHash verifies a compact signature against a raw hash and public key
func VerifyCompactHash(pubKey *PublicKey, signature []byte, hash []byte) bool {
	if len(signature) != 65 {
		return false
	}

	// Recover public key from signature
	recoveredPubKey, _, err := ecdsa.RecoverCompact(signature, hash)
	if err != nil {
		return false
	}

	// Compare recovered key with expected key
	return recoveredPubKey.IsEqual(pubKey.key)
}

// Signature represents an ECDSA signature
type Signature struct {
	R *big.Int
	S *big.Int
}

// Bytes returns the signature as DER-encoded bytes (Bitcoin/legacy compatible).
// DER format: 0x30 [total-length] 0x02 [R-length] [R] 0x02 [S-length] [S]
// This is the standard Bitcoin signature format expected by legacy C++ nodes.
func (sig *Signature) Bytes() []byte {
	// Get R and S bytes, stripping leading zeros
	rBytes := sig.R.Bytes()
	sBytes := sig.S.Bytes()

	// Add leading 0x00 if high bit is set (DER requires positive integers)
	if len(rBytes) > 0 && rBytes[0]&0x80 != 0 {
		rBytes = append([]byte{0x00}, rBytes...)
	}
	if len(sBytes) > 0 && sBytes[0]&0x80 != 0 {
		sBytes = append([]byte{0x00}, sBytes...)
	}

	// Handle empty R or S (shouldn't happen, but be safe)
	if len(rBytes) == 0 {
		rBytes = []byte{0x00}
	}
	if len(sBytes) == 0 {
		sBytes = []byte{0x00}
	}

	// Build DER structure
	// 0x30 = SEQUENCE, 0x02 = INTEGER
	totalLen := 2 + len(rBytes) + 2 + len(sBytes) // R tag+len + R + S tag+len + S
	result := make([]byte, 0, 2+totalLen)

	// SEQUENCE header
	result = append(result, 0x30)
	result = append(result, byte(totalLen))

	// R INTEGER
	result = append(result, 0x02)
	result = append(result, byte(len(rBytes)))
	result = append(result, rBytes...)

	// S INTEGER
	result = append(result, 0x02)
	result = append(result, byte(len(sBytes)))
	result = append(result, sBytes...)

	return result
}

// RawBytes returns the signature as 64-byte R||S format (non-DER).
// Use this when compact format is needed instead of DER.
func (sig *Signature) RawBytes() []byte {
	result := make([]byte, 64)

	rBytes := sig.R.Bytes()
	sBytes := sig.S.Bytes()

	// Pad to 32 bytes if necessary
	copy(result[32-len(rBytes):32], rBytes)
	copy(result[64-len(sBytes):64], sBytes)

	return result
}

// Hex returns the signature as a hex string
func (sig *Signature) Hex() string {
	return hex.EncodeToString(sig.Bytes())
}

// ParseSignatureFromBytes parses a signature from bytes
// Supports both 64-byte raw R||S format and DER-encoded signatures
func ParseSignatureFromBytes(data []byte) (*Signature, error) {
	// Try DER format first (legacy TWINS format)
	// DER signatures can be 70-72 bytes typically, or even shorter
	if len(data) > 8 && data[0] == 0x30 {
		// Validate it's a valid DER signature first
		_, err := ecdsa.ParseDERSignature(data)
		if err == nil {
			// Extract R and S from DER bytes manually since btcec fields are private
			// DER format: 0x30 [total-length] 0x02 [R-length] [R] 0x02 [S-length] [S]
			pos := 2
			if data[1] > 0x80 {
				// Long form length
				pos = 2 + int(data[1]&0x7f)
			}

			// Read R
			if pos >= len(data) || data[pos] != 0x02 {
				return nil, fmt.Errorf("invalid DER signature: missing R integer tag")
			}
			pos++
			rLen := int(data[pos])
			pos++
			if pos+rLen > len(data) {
				return nil, fmt.Errorf("invalid DER signature: R length exceeds data")
			}
			rBytes := data[pos : pos+rLen]
			pos += rLen

			// Read S
			if pos >= len(data) || data[pos] != 0x02 {
				return nil, fmt.Errorf("invalid DER signature: missing S integer tag")
			}
			pos++
			sLen := int(data[pos])
			pos++
			if pos+sLen > len(data) {
				return nil, fmt.Errorf("invalid DER signature: S length exceeds data")
			}
			sBytes := data[pos : pos+sLen]

			// Convert to big.Int
			r := new(big.Int).SetBytes(rBytes)
			s := new(big.Int).SetBytes(sBytes)

			return &Signature{R: r, S: s}, nil
		}
	}

	// Fall back to raw 64-byte R||S format
	if len(data) == 64 {
		r := new(big.Int).SetBytes(data[:32])
		s := new(big.Int).SetBytes(data[32:64])
		return &Signature{R: r, S: s}, nil
	}

	return nil, fmt.Errorf("invalid signature format: length=%d, not valid DER or 64-byte R||S", len(data))
}

// ParseSignatureFromHex parses a signature from a hex string
func ParseSignatureFromHex(hexStr string) (*Signature, error) {
	data, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("invalid hex string: %v", err)
	}

	return ParseSignatureFromBytes(data)
}

// Ed25519 specific methods

// Bytes returns the Ed25519 private key as bytes
func (kp *Ed25519KeyPair) PrivateBytes() []byte {
	return kp.Private
}

// PublicBytes returns the Ed25519 public key as bytes
func (kp *Ed25519KeyPair) PublicBytes() []byte {
	return kp.Public
}

// Sign signs a message using Ed25519
func (kp *Ed25519KeyPair) Sign(message []byte) []byte {
	return ed25519.Sign(kp.Private, message)
}

// Verify verifies an Ed25519 signature
func (kp *Ed25519KeyPair) Verify(message, signature []byte) bool {
	return ed25519.Verify(kp.Public, message, signature)
}

// VerifyEd25519 verifies an Ed25519 signature with a public key
func VerifyEd25519(publicKey, message, signature []byte) bool {
	if len(publicKey) != ed25519.PublicKeySize {
		return false
	}
	if len(signature) != ed25519.SignatureSize {
		return false
	}

	return ed25519.Verify(publicKey, message, signature)
}