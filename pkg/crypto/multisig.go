package crypto

import (
	"encoding/hex"
	"errors"
	"fmt"
)

// Script opcodes for multisig
const (
	OP_0             = 0x00
	OP_1             = 0x51
	OP_2             = 0x52
	OP_3             = 0x53
	OP_4             = 0x54
	OP_5             = 0x55
	OP_6             = 0x56
	OP_7             = 0x57
	OP_8             = 0x58
	OP_9             = 0x59
	OP_10            = 0x5a
	OP_11            = 0x5b
	OP_12            = 0x5c
	OP_13            = 0x5d
	OP_14            = 0x5e
	OP_15            = 0x5f
	OP_16            = 0x60
	OP_PUSHDATA1     = 0x4c
	OP_PUSHDATA2     = 0x4d
	OP_PUSHDATA4     = 0x4e
	OP_CHECKMULTISIG = 0xae
)

// MultisigInfo contains information about a created multisig address
type MultisigInfo struct {
	Address      string // P2SH address
	RedeemScript string // Hex-encoded redeem script
}

// CreateMultisigAddress creates a multisig P2SH address from public keys
// nrequired: number of signatures required (1-15)
// keys: array of hex-encoded public keys (33 or 65 bytes)
// netID: network ID for address encoding
//
// NOTE: This function accepts ONLY hex-encoded public keys, not addresses.
// To create multisig from addresses, use wallet.AddMultisigAddress() which
// can look up public keys from the wallet.
func CreateMultisigAddress(nrequired int, keys []string, netID byte) (*MultisigInfo, error) {
	// Validate parameters
	if nrequired < 1 {
		return nil, errors.New("nrequired must be at least 1")
	}
	if nrequired > 15 {
		return nil, errors.New("nrequired cannot exceed 15 (Bitcoin script limitation)")
	}
	if len(keys) < 2 || len(keys) > 15 {
		return nil, errors.New("must have between 2 and 15 keys")
	}
	if nrequired > len(keys) {
		return nil, fmt.Errorf("nrequired (%d) cannot exceed number of keys (%d)", nrequired, len(keys))
	}

	// Parse and validate public keys
	pubKeys := make([][]byte, 0, len(keys))
	for i, key := range keys {
		// Parse as hex-encoded public key
		pubKeyBytes, err := hex.DecodeString(key)
		if err != nil {
			return nil, fmt.Errorf("key %d: invalid hex encoding: %v", i, err)
		}

		// Validate public key format
		// Compressed: 33 bytes with 02/03 prefix
		// Uncompressed: 65 bytes with 04 prefix
		if len(pubKeyBytes) == 33 {
			if pubKeyBytes[0] != 0x02 && pubKeyBytes[0] != 0x03 {
				return nil, fmt.Errorf("key %d: invalid compressed public key prefix (expected 02 or 03)", i)
			}
		} else if len(pubKeyBytes) == 65 {
			if pubKeyBytes[0] != 0x04 {
				return nil, fmt.Errorf("key %d: invalid uncompressed public key prefix (expected 04)", i)
			}
		} else {
			return nil, fmt.Errorf("key %d: invalid public key length %d (expected 33 or 65)", i, len(pubKeyBytes))
		}

		// Store full public key (NOT hash)
		pubKeys = append(pubKeys, pubKeyBytes)
	}

	// Create multisig redeem script using FULL public keys
	redeemScript := createMultisigRedeemScript(nrequired, pubKeys)

	// Create P2SH address from redeem script
	scriptAddr := NewScriptAddress(redeemScript, netID)

	// Encode redeem script as hex
	redeemScriptHex := fmt.Sprintf("%x", redeemScript)

	return &MultisigInfo{
		Address:      scriptAddr.String(),
		RedeemScript: redeemScriptHex,
	}, nil
}

// createMultisigRedeemScript creates a multisig redeem script
// Bitcoin P2SH Multisig Format:
//   <nrequired> <pubkey1> <pubkey2> ... <npubkeys> OP_CHECKMULTISIG
//
// Example 2-of-3:
//   OP_2 <33-byte-pubkey1> <33-byte-pubkey2> <33-byte-pubkey3> OP_3 OP_CHECKMULTISIG
//
// The script hash (Hash160) of this redeem script becomes the P2SH address.
// To spend: provide <sig1> <sig2> <redeemScript>
//
// CRITICAL: OP_CHECKMULTISIG requires FULL public keys (33 or 65 bytes),
// NOT Hash160 (20 bytes). Using hashes will make funds permanently unspendable.
func createMultisigRedeemScript(nrequired int, pubKeys [][]byte) []byte {
	script := make([]byte, 0, 256)

	// Push nrequired (OP_1 through OP_16)
	script = append(script, encodeNumber(nrequired))

	// Push each FULL public key
	for _, pk := range pubKeys {
		// For keys <= 75 bytes, use direct push opcode
		// (All valid pubkeys are 33 or 65 bytes, so this always applies)
		script = append(script, byte(len(pk))) // Push length opcode
		script = append(script, pk...)         // Push full public key bytes
	}

	// Push total number of keys (OP_1 through OP_16)
	script = append(script, encodeNumber(len(pubKeys)))

	// OP_CHECKMULTISIG
	script = append(script, OP_CHECKMULTISIG)

	return script
}

// encodeNumber encodes a small integer (1-16) as an opcode
func encodeNumber(n int) byte {
	if n >= 1 && n <= 16 {
		return byte(OP_1 + n - 1)
	}
	// Should not happen with validation
	return OP_0
}
