package crypto

import (
	"crypto"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
)

// TWINSMessageSigner implements Go 1.25's crypto.MessageSigner interface
type TWINSMessageSigner struct {
	privateKey *btcec.PrivateKey
	mutex      sync.RWMutex
}

// MasternodeSigner provides masternode-specific signing capabilities
type MasternodeSigner struct {
	signer *TWINSMessageSigner
	nodeID string
}

// BatchSigner allows efficient batch signing operations
type BatchSigner struct {
	signer *TWINSMessageSigner
	mutex  sync.Mutex
}

// MultiSigValidator validates multi-signature schemes
type MultiSigValidator struct {
	threshold  int
	publicKeys []*PublicKey
}

// SignerOpts implements crypto.SignerOpts for TWINS-specific options
type SignerOpts struct {
	hash         crypto.Hash
	masternodeID string
}

// NewTWINSMessageSigner creates a new message signer from a private key
func NewTWINSMessageSigner(privateKey *PrivateKey) *TWINSMessageSigner {
	return &TWINSMessageSigner{
		privateKey: privateKey.key,
	}
}

// Public returns the public key corresponding to the private key
func (s *TWINSMessageSigner) Public() crypto.PublicKey {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.privateKey.PubKey()
}

// Sign implements crypto.Signer interface
// Returns standard 64-byte R||S signature (not compact format)
func (s *TWINSMessageSigner) Sign(reader io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if len(digest) != 32 {
		return nil, fmt.Errorf("digest must be 32 bytes for SHA-256")
	}

	// Sign the digest using ECDSA
	sig := ecdsa.Sign(s.privateKey, digest)

	// Serialize to DER format (standard Bitcoin signature format)
	// This will be parsed by ParseSignatureFromBytes which handles both DER and R||S
	sigBytes := sig.Serialize()

	return sigBytes, nil
}

// SignMessage implements Go 1.25's crypto.MessageSigner interface
func (s *TWINSMessageSigner) SignMessage(message []byte, opts crypto.SignerOpts) ([]byte, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Hash the message first
	hasher := opts.HashFunc().New()
	if _, err := hasher.Write(message); err != nil {
		return nil, fmt.Errorf("failed to hash message: %v", err)
	}
	digest := hasher.Sum(nil)

	return s.Sign(rand.Reader, digest, opts)
}

// GetPublicKey returns the public key as a TWINS PublicKey type
func (s *TWINSMessageSigner) GetPublicKey() *PublicKey {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return &PublicKey{key: s.privateKey.PubKey()}
}

// NewMasternodeSigner creates a new masternode signer
func NewMasternodeSigner(privateKey *PrivateKey, nodeID string) *MasternodeSigner {
	return &MasternodeSigner{
		signer: NewTWINSMessageSigner(privateKey),
		nodeID: nodeID,
	}
}

// SignMasternodeMessage signs a message with masternode-specific formatting
func (ms *MasternodeSigner) SignMasternodeMessage(message []byte) ([]byte, error) {
	// Prepend masternode ID to message for unique signing
	masternodeMessage := append([]byte(ms.nodeID), message...)

	opts := &SignerOpts{hash: crypto.SHA256, masternodeID: ms.nodeID}
	return ms.signer.SignMessage(masternodeMessage, opts)
}

// GetNodeID returns the masternode ID
func (ms *MasternodeSigner) GetNodeID() string {
	return ms.nodeID
}

// GetPublicKey returns the masternode's public key
func (ms *MasternodeSigner) GetPublicKey() *PublicKey {
	return ms.signer.GetPublicKey()
}

// VerifyMasternodeSignature verifies a masternode signature
func VerifyMasternodeSignature(publicKey *PublicKey, nodeID string, message []byte, signature []byte) bool {
	// Recreate the signed message
	masternodeMessage := append([]byte(nodeID), message...)

	// Parse signature (DER or 64-byte R||S format)
	sig, err := ParseSignatureFromBytes(signature)
	if err != nil {
		return false
	}

	// Hash the message
	digest := Hash256(masternodeMessage)

	return publicKey.Verify(digest, sig)
}

// NewBatchSigner creates a new batch signer
func NewBatchSigner(privateKey *PrivateKey) *BatchSigner {
	return &BatchSigner{
		signer: NewTWINSMessageSigner(privateKey),
	}
}

// SignBatch efficiently signs multiple messages in a batch
func (bs *BatchSigner) SignBatch(messages [][]byte) ([][]byte, error) {
	bs.mutex.Lock()
	defer bs.mutex.Unlock()

	signatures := make([][]byte, len(messages))
	opts := &SignerOpts{hash: crypto.SHA256}

	for i, message := range messages {
		sig, err := bs.signer.SignMessage(message, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to sign message %d: %v", i, err)
		}
		signatures[i] = sig
	}

	return signatures, nil
}

// SignBatchConcurrent signs multiple messages concurrently
func (bs *BatchSigner) SignBatchConcurrent(messages [][]byte) ([][]byte, error) {
	signatures := make([][]byte, len(messages))
	errors := make([]error, len(messages))

	var wg sync.WaitGroup
	opts := &SignerOpts{hash: crypto.SHA256}

	for i, message := range messages {
		wg.Add(1)
		go func(index int, msg []byte) {
			defer wg.Done()

			sig, err := bs.signer.SignMessage(msg, opts)
			if err != nil {
				errors[index] = err
				return
			}
			signatures[index] = sig
		}(i, message)
	}

	wg.Wait()

	// Check for errors
	for i, err := range errors {
		if err != nil {
			return nil, fmt.Errorf("failed to sign message %d: %v", i, err)
		}
	}

	return signatures, nil
}

// NewMultiSigValidator creates a new multi-signature validator
func NewMultiSigValidator(threshold int, publicKeys []*PublicKey) *MultiSigValidator {
	return &MultiSigValidator{
		threshold:  threshold,
		publicKeys: publicKeys,
	}
}

// ValidateMultiSig validates a multi-signature against a message
func (msv *MultiSigValidator) ValidateMultiSig(message []byte, signatures [][]byte) bool {
	if len(signatures) < msv.threshold {
		return false
	}

	digest := Hash256(message)
	validSignatures := 0

	// Check each signature against each public key
	for _, sigBytes := range signatures {
		sig, err := ParseSignatureFromBytes(sigBytes)
		if err != nil {
			continue
		}

		for _, pubKey := range msv.publicKeys {
			if pubKey.Verify(digest, sig) {
				validSignatures++
				break // Only count each signature once
			}
		}
	}

	return validSignatures >= msv.threshold
}

// ValidatePartialMultiSig validates signatures from specific signers
func (msv *MultiSigValidator) ValidatePartialMultiSig(message []byte, signatures [][]byte, signerIndices []int) bool {
	if len(signatures) != len(signerIndices) {
		return false
	}

	if len(signatures) < msv.threshold {
		return false
	}

	digest := Hash256(message)
	validSignatures := 0

	for i, sigBytes := range signatures {
		if signerIndices[i] >= len(msv.publicKeys) {
			continue
		}

		sig, err := ParseSignatureFromBytes(sigBytes)
		if err != nil {
			continue
		}

		pubKey := msv.publicKeys[signerIndices[i]]
		if pubKey.Verify(digest, sig) {
			validSignatures++
		}
	}

	return validSignatures >= msv.threshold
}

// GetThreshold returns the multi-signature threshold
func (msv *MultiSigValidator) GetThreshold() int {
	return msv.threshold
}

// GetPublicKeys returns the public keys used for validation
func (msv *MultiSigValidator) GetPublicKeys() []*PublicKey {
	return msv.publicKeys
}

// AddPublicKey adds a public key to the validator
func (msv *MultiSigValidator) AddPublicKey(publicKey *PublicKey) {
	msv.publicKeys = append(msv.publicKeys, publicKey)
}

// RemovePublicKey removes a public key from the validator
func (msv *MultiSigValidator) RemovePublicKey(index int) error {
	if index < 0 || index >= len(msv.publicKeys) {
		return errors.New("invalid public key index")
	}

	msv.publicKeys = append(msv.publicKeys[:index], msv.publicKeys[index+1:]...)
	return nil
}

// SignerOpts implementation

func (so *SignerOpts) HashFunc() crypto.Hash {
	return so.hash
}

// NewSignerOpts creates new signer options
func NewSignerOpts(hashFunc crypto.Hash) *SignerOpts {
	return &SignerOpts{hash: hashFunc}
}

// NewMasternodeSignerOpts creates new masternode-specific signer options
func NewMasternodeSignerOpts(hashFunc crypto.Hash, masternodeID string) *SignerOpts {
	return &SignerOpts{
		hash:         hashFunc,
		masternodeID: masternodeID,
	}
}

// GetMasternodeID returns the masternode ID from options
func (so *SignerOpts) GetMasternodeID() string {
	return so.masternodeID
}

// Utility functions for signature verification

// VerifyMessageSignature verifies a message signature using a public key
// This function double-hashes the message (Hash256) before verification
func VerifyMessageSignature(publicKey *PublicKey, message []byte, signature []byte) bool {
	// Handle 65-byte compact signatures by stripping recovery byte
	if len(signature) == 65 {
		signature = signature[1:] // Strip recovery byte, use R||S
	}

	sig, err := ParseSignatureFromBytes(signature)
	if err != nil {
		return false
	}

	digest := Hash256(message)
	return publicKey.Verify(digest, sig)
}

// VerifySignature verifies a signature against a pre-computed hash
// Accepts both DER and 64-byte R||S formats for flexibility.
// WARNING: For block signatures, use VerifyDERSignature instead for legacy compatibility.
func VerifySignature(publicKey *PublicKey, hash []byte, signature []byte) bool {
	sig, err := ParseSignatureFromBytes(signature)
	if err != nil {
		return false
	}

	return publicKey.Verify(hash, sig)
}

// VerifyDERSignature verifies a DER-encoded signature (strict - no R||S fallback).
// Use this for block signatures where legacy C++ compatibility is required.
// Legacy C++ nodes only accept DER format, so Go nodes must also be strict
// to maintain consensus compatibility.
// Matches legacy: CPubKey::Verify() in pubkey.cpp which expects DER format.
func VerifyDERSignature(publicKey *PublicKey, hash []byte, signature []byte) bool {
	// Reject obviously invalid signatures
	if len(signature) < 8 {
		return false
	}

	// MUST start with DER SEQUENCE tag (0x30)
	if signature[0] != 0x30 {
		return false
	}

	// Parse as strict DER format using btcec
	sig, err := ecdsa.ParseDERSignature(signature)
	if err != nil {
		return false
	}

	// Verify signature
	return sig.Verify(hash, publicKey.key)
}

// CreateMessageSignature creates a signature for a message
func CreateMessageSignature(privateKey *PrivateKey, message []byte) ([]byte, error) {
	signer := NewTWINSMessageSigner(privateKey)
	opts := NewSignerOpts(crypto.SHA256)
	return signer.SignMessage(message, opts)
}

// RecoverPublicKeyFromSignature attempts to recover the public key from a compact signature
// Signature must be 65 bytes: [recovery_id (1 byte)][r (32 bytes)][s (32 bytes)]
// This is used for masternode and spork message verification
func RecoverPublicKeyFromSignature(message []byte, signature []byte) (*PublicKey, error) {
	if len(signature) != 65 {
		return nil, fmt.Errorf("invalid signature length: expected 65 bytes, got %d", len(signature))
	}

	// Hash the message (double SHA256 for Bitcoin compatibility)
	messageHash := Hash256(Hash256(message))

	// Extract recovery ID (first byte) and signature (remaining 64 bytes)
	recoveryID := signature[0]
	if recoveryID >= 4 {
		// Adjust for compressed key flag (27-30 for uncompressed, 31-34 for compressed)
		if recoveryID >= 31 && recoveryID <= 34 {
			recoveryID -= 31
		} else if recoveryID >= 27 && recoveryID <= 30 {
			recoveryID -= 27
		} else {
			return nil, fmt.Errorf("invalid recovery ID: %d", recoveryID)
		}
	}

	// Recover the public key using btcec's RecoverCompact
	// This handles the full 65-byte compact signature format
	pubKey, _, err := ecdsa.RecoverCompact(signature, messageHash)
	if err != nil {
		return nil, fmt.Errorf("failed to recover public key: %w", err)
	}

	return &PublicKey{key: pubKey}, nil
}

// ValidateSignatureFormat validates that a signature has the correct format
func ValidateSignatureFormat(signature []byte) error {
	// Accept 64-byte (R||S), 65-byte (compact with recovery), and DER signatures
	sigLen := len(signature)

	// Check for standard formats
	if sigLen == 64 || sigLen == 65 {
		return nil
	}

	// Check for DER format (typically 70-72 bytes, but can vary)
	if sigLen > 8 && signature[0] == 0x30 {
		// Validate DER structure
		_, err := ecdsa.ParseDERSignature(signature)
		if err == nil {
			return nil
		}
		return fmt.Errorf("invalid DER signature: %v", err)
	}

	return fmt.Errorf("signature must be 64 bytes (R||S), 65 bytes (compact), or valid DER format, got %d bytes", sigLen)
}

// IsValidSignature checks if a signature is valid without verifying against a message
func IsValidSignature(signature []byte) bool {
	return ValidateSignatureFormat(signature) == nil
}

// ParsePublicKeyHex parses a hex-encoded public key
func ParsePublicKeyHex(hexKey string) (*btcec.PublicKey, error) {
	pubKey, err := ParsePublicKeyFromHex(hexKey)
	if err != nil {
		return nil, err
	}
	return pubKey.key, nil
}

// PublicKeyToHex converts a public key to hex string
func PublicKeyToHex(pubKey *btcec.PublicKey) string {
	pk := &PublicKey{key: pubKey}
	return pk.Hex()
}

// GenerateKey generates a new secp256k1 private key
func GenerateKey() (*btcec.PrivateKey, error) {
	kp, err := GenerateKeyPair()
	if err != nil {
		return nil, err
	}
	return kp.Private.key, nil
}

// SignMessage signs a message with compact 65-byte signature format
func SignMessage(privKey *btcec.PrivateKey, message []byte) ([]byte, error) {
	pk := &PrivateKey{key: privKey}
	signer := NewTWINSMessageSigner(pk)
	opts := NewSignerOpts(crypto.SHA256)

	sig, err := signer.SignMessage(message, opts)
	if err != nil {
		return nil, err
	}

	// TWINSMessageSigner.Sign() now returns 65-byte compact signature with proper recovery byte
	return sig, nil
}

// VerifyRawCompactSignature verifies a compact signature against a public key and message
func VerifyRawCompactSignature(pubKey *btcec.PublicKey, message []byte, signature []byte) bool {
	if len(signature) == 65 {
		// Remove recovery byte if present
		signature = signature[:64]
	}

	pk := &PublicKey{key: pubKey}
	return VerifyMessageSignature(pk, message, signature)
}