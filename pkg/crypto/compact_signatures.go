// Copyright (c) 2025 The TWINS Core developers
// Distributed under the MIT software license

package crypto

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"

	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// Bitcoin message magic used in legacy TWINS (inherited from DASH/PIVX)
const MessageMagic = "DarkNet Signed Message:\n"

// SignCompact creates a compact signature (65 bytes) compatible with Bitcoin/legacy TWINS
// Format: [recovery_id(1 byte)][r(32 bytes)][s(32 bytes)]
// The message is prefixed with Bitcoin message magic before hashing
func SignCompact(privKey *PrivateKey, message string) ([]byte, error) {
	// Create message hash with Bitcoin magic prefix (matches legacy signing)
	messageHash := hashMessageForSigning(message)

	// Sign using ecdsa's compact signature (includes recovery ID)
	// privKey.key is already a *btcec.PrivateKey
	sig := ecdsa.SignCompact(privKey.key, messageHash, true) // true = compressed pubkey

	return sig, nil
}

// VerifyCompactSignature verifies a compact signature (65 bytes) against a public key
// Returns true if signature is valid and recovers to the expected public key
func VerifyCompactSignature(pubKey *PublicKey, message string, signature []byte) (bool, error) {
	if len(signature) != 65 {
		return false, fmt.Errorf("compact signature must be 65 bytes, got %d", len(signature))
	}

	// Recover public key from compact signature
	recoveredPubKey, err := RecoverCompactSignature(message, signature)
	if err != nil {
		return false, fmt.Errorf("failed to recover public key: %w", err)
	}

	// Compare recovered key with expected key by comparing the serialized public keys
	return bytes.Equal(recoveredPubKey.Bytes(), pubKey.Bytes()), nil
}

// RecoverCompactSignature recovers the public key from a compact signature
// Returns the public key that created the signature
func RecoverCompactSignature(message string, signature []byte) (*PublicKey, error) {
	if len(signature) != 65 {
		return nil, fmt.Errorf("compact signature must be 65 bytes, got %d", len(signature))
	}

	// Create message hash with Bitcoin magic prefix (matches legacy signing)
	messageHash := hashMessageForSigning(message)

	// Recover public key from signature
	recoveredPubKey, _, err := ecdsa.RecoverCompact(signature, messageHash)
	if err != nil {
		return nil, fmt.Errorf("failed to recover public key: %w", err)
	}

	// Convert to our PublicKey type (already btcec/v2.PublicKey)
	return &PublicKey{key: recoveredPubKey}, nil
}

// writeCompactSize writes a Bitcoin compact size (varint) to the writer
// This matches Bitcoin Core's WriteCompactSize and C++ CHashWriter serialization
func writeCompactSize(w io.Writer, val uint64) error {
	if val < 0xfd {
		// 1 byte: 0x00-0xfc
		_, err := w.Write([]byte{byte(val)})
		return err
	} else if val <= 0xffff {
		// 3 bytes: 0xfd + 2 byte uint16
		if _, err := w.Write([]byte{0xfd}); err != nil {
			return err
		}
		return binary.Write(w, binary.LittleEndian, uint16(val))
	} else if val <= 0xffffffff {
		// 5 bytes: 0xfe + 4 byte uint32
		if _, err := w.Write([]byte{0xfe}); err != nil {
			return err
		}
		return binary.Write(w, binary.LittleEndian, uint32(val))
	} else {
		// 9 bytes: 0xff + 8 byte uint64
		if _, err := w.Write([]byte{0xff}); err != nil {
			return err
		}
		return binary.Write(w, binary.LittleEndian, val)
	}
}

// hashMessageForSigning creates the message hash using Bitcoin message magic
// This matches the legacy TWINS signing format (Bitcoin/DASH/PIVX compatible):
//   SHA256(SHA256(varint(len(MessageMagic)) + MessageMagic + varint(len(message)) + message))
//
// The C++ code uses CHashWriter which serializes strings with their length prefix:
//   CHashWriter ss(SER_GETHASH, 0);
//   ss << strMessageMagic;  // Writes: varint(len) + data
//   ss << strMessage;       // Writes: varint(len) + data
func hashMessageForSigning(message string) []byte {
	// Build message with varint-prefixed strings (matches C++ CHashWriter << string)
	var buf bytes.Buffer

	// Write MessageMagic with varint length prefix
	writeCompactSize(&buf, uint64(len(MessageMagic)))
	buf.WriteString(MessageMagic)

	// Write message with varint length prefix
	writeCompactSize(&buf, uint64(len(message)))
	buf.WriteString(message)

	// Double SHA256 (Bitcoin standard) using chainhash
	return chainhash.DoubleHashB(buf.Bytes())
}

// SignCompactBytes is a convenience function that signs raw bytes
// by converting them to a string first
func SignCompactBytes(privKey *PrivateKey, data []byte) ([]byte, error) {
	return SignCompact(privKey, string(data))
}

// VerifyCompactSignatureBytes is a convenience function that verifies
// a signature on raw bytes by converting them to a string first
func VerifyCompactSignatureBytes(pubKey *PublicKey, data []byte, signature []byte) (bool, error) {
	return VerifyCompactSignature(pubKey, string(data), signature)
}

// ParseCompactSignature validates and parses a compact signature
// Returns the recovery ID, R, and S values
func ParseCompactSignature(signature []byte) (recoveryID byte, r, s *big.Int, err error) {
	if len(signature) != 65 {
		return 0, nil, nil, fmt.Errorf("compact signature must be 65 bytes, got %d", len(signature))
	}

	recoveryID = signature[0]
	r = new(big.Int).SetBytes(signature[1:33])
	s = new(big.Int).SetBytes(signature[33:65])

	return recoveryID, r, s, nil
}
