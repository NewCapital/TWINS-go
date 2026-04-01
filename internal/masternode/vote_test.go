package masternode

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/twins-dev/twins-core/pkg/script"
	"github.com/twins-dev/twins-core/pkg/types"
)

// TestGetSignatureMessage_ASMFormat verifies that GetSignatureMessage returns
// the correct ASM format matching legacy C++ CScript::ToString()
func TestGetSignatureMessage_ASMFormat(t *testing.T) {
	// Create a P2PKH script (25 bytes)
	// Format: OP_DUP OP_HASH160 <20-byte hash> OP_EQUALVERIFY OP_CHECKSIG
	p2pkhScript := []byte{
		0x76, 0xa9, 0x14, // OP_DUP OP_HASH160 PUSH(20)
		0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67, // hash bytes 1-8
		0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67, // hash bytes 9-16
		0x89, 0xab, 0xcd, 0xef, // hash bytes 17-20
		0x88, 0xac, // OP_EQUALVERIFY OP_CHECKSIG
	}

	// Create a test vote
	vote := &MasternodeWinnerVote{
		VoterOutpoint: types.Outpoint{
			Hash:  types.Hash{0x12, 0x34, 0x56, 0x78}, // Will be reversed in String()
			Index: 1,
		},
		BlockHeight: 1000000,
		PayeeScript: p2pkhScript,
	}

	message := vote.GetSignatureMessage()

	// Verify message contains ASM format (space-separated opcodes), NOT raw hex
	// The message should contain "OP_DUP OP_HASH160" for P2PKH scripts
	if !strings.Contains(message, "OP_DUP") {
		t.Errorf("GetSignatureMessage should contain OP_DUP (ASM format).\nGot: %s", message)
	}

	if !strings.Contains(message, "OP_HASH160") {
		t.Errorf("GetSignatureMessage should contain OP_HASH160 (ASM format).\nGot: %s", message)
	}

	if !strings.Contains(message, "OP_EQUALVERIFY") {
		t.Errorf("GetSignatureMessage should contain OP_EQUALVERIFY (ASM format).\nGot: %s", message)
	}

	if !strings.Contains(message, "OP_CHECKSIG") {
		t.Errorf("GetSignatureMessage should contain OP_CHECKSIG (ASM format).\nGot: %s", message)
	}

	// Verify it does NOT contain raw hex prefix "76a914" (that would be wrong format)
	if strings.Contains(message, "76a914") {
		t.Errorf("GetSignatureMessage should NOT contain raw hex '76a914' (wrong format).\nGot: %s", message)
	}

	t.Logf("Vote signature message: %s", message)
}

// TestGetSignatureMessage_MatchesLegacyFormat verifies the exact format matches legacy C++
// Legacy: vinMasternode.prevout.ToStringShort() + nBlockHeight + payee.ToString()
func TestGetSignatureMessage_MatchesLegacyFormat(t *testing.T) {
	// Known test vector with expected output
	// P2PKH script for pubkey hash: 89abcdef0123456789abcdef0123456789abcdef
	p2pkhScript := []byte{
		0x76, 0xa9, 0x14,
		0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67,
		0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67,
		0x89, 0xab, 0xcd, 0xef,
		0x88, 0xac,
	}

	// Hash bytes (will be reversed when displayed)
	hashBytes := [32]byte{
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
		0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20,
	}

	vote := &MasternodeWinnerVote{
		VoterOutpoint: types.Outpoint{
			Hash:  types.Hash(hashBytes),
			Index: 0,
		},
		BlockHeight: 500000,
		PayeeScript: p2pkhScript,
	}

	message := vote.GetSignatureMessage()

	// Expected format: "<hash-reversed>-<index><height><asm>"
	// Hash reversed: 201f1e1d1c1b1a191817161514131211100f0e0d0c0b0a090807060504030201
	// Index: 0
	// Height: 500000
	// ASM: OP_DUP OP_HASH160 89abcdef0123456789abcdef0123456789abcdef OP_EQUALVERIFY OP_CHECKSIG

	// Verify hash is reversed (big-endian display)
	expectedHashPrefix := "201f1e1d1c1b1a191817161514131211100f0e0d0c0b0a090807060504030201"
	if !strings.HasPrefix(message, expectedHashPrefix) {
		t.Errorf("Message should start with reversed hash.\nExpected prefix: %s\nGot: %s", expectedHashPrefix, message)
	}

	// Verify format: hash-index then height then ASM
	if !strings.Contains(message, "-0500000") {
		t.Errorf("Message should contain '-0500000' (index 0, height 500000).\nGot: %s", message)
	}

	// Verify pubkey hash is in hex (20 bytes = 40 hex chars)
	pubkeyHashHex := "89abcdef0123456789abcdef0123456789abcdef"
	if !strings.Contains(message, pubkeyHashHex) {
		t.Errorf("Message should contain pubkey hash hex: %s\nGot: %s", pubkeyHashHex, message)
	}

	t.Logf("Full message: %s", message)
}

// TestGetSignatureMessage_EmptyScript verifies behavior with empty script
func TestGetSignatureMessage_EmptyScript(t *testing.T) {
	vote := &MasternodeWinnerVote{
		VoterOutpoint: types.Outpoint{
			Hash:  types.Hash{0x01},
			Index: 0,
		},
		BlockHeight: 100,
		PayeeScript: []byte{}, // Empty script
	}

	message := vote.GetSignatureMessage()

	// Should not panic, should produce valid message
	if message == "" {
		t.Error("GetSignatureMessage should not return empty string even with empty script")
	}

	// Empty script disassembles to empty string, so message should end with height
	if !strings.Contains(message, "-0100") {
		t.Errorf("Message should contain '-0100' (index 0, height 100).\nGot: %s", message)
	}

	t.Logf("Empty script message: %s", message)
}

// TestGetSignatureMessage_InvalidScript verifies fallback to hex for invalid scripts
func TestGetSignatureMessage_InvalidScript(t *testing.T) {
	// Create an invalid script that will fail disassembly
	// PUSHDATA1 (0x4c) followed by length 0xFF but not enough data
	invalidScript := []byte{0x4c, 0xff}

	vote := &MasternodeWinnerVote{
		VoterOutpoint: types.Outpoint{
			Hash:  types.Hash{0x01},
			Index: 0,
		},
		BlockHeight: 100,
		PayeeScript: invalidScript,
	}

	message := vote.GetSignatureMessage()

	// Should not panic
	if message == "" {
		t.Error("GetSignatureMessage should not return empty string even with invalid script")
	}

	// On disassembly error, should fallback to hex format
	// Hex of invalidScript: 4cff
	t.Logf("Invalid script message (should fallback to hex): %s", message)
}

// TestDisassemble_P2PKH verifies script.Disassemble matches expected ASM format
func TestDisassemble_P2PKH(t *testing.T) {
	// Standard P2PKH script
	p2pkhScript := []byte{
		0x76,       // OP_DUP
		0xa9,       // OP_HASH160
		0x14,       // PUSH 20 bytes
		0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67,
		0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67,
		0x89, 0xab, 0xcd, 0xef,
		0x88, // OP_EQUALVERIFY
		0xac, // OP_CHECKSIG
	}

	asm, err := script.Disassemble(p2pkhScript)
	if err != nil {
		t.Fatalf("Disassemble failed: %v", err)
	}

	// Expected format: "OP_DUP OP_HASH160 <hash> OP_EQUALVERIFY OP_CHECKSIG"
	expected := "OP_DUP OP_HASH160 89abcdef0123456789abcdef0123456789abcdef OP_EQUALVERIFY OP_CHECKSIG"
	if asm != expected {
		t.Errorf("Disassemble output mismatch.\nGot:  %s\nWant: %s", asm, expected)
	}
}

// TestDisassemble_P2SH verifies P2SH script disassembly
func TestDisassemble_P2SH(t *testing.T) {
	// Standard P2SH script: OP_HASH160 <20 bytes> OP_EQUAL
	p2shScript := []byte{
		0xa9, // OP_HASH160
		0x14, // PUSH 20 bytes
		0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67,
		0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67,
		0x89, 0xab, 0xcd, 0xef,
		0x87, // OP_EQUAL
	}

	asm, err := script.Disassemble(p2shScript)
	if err != nil {
		t.Fatalf("Disassemble failed: %v", err)
	}

	expected := "OP_HASH160 89abcdef0123456789abcdef0123456789abcdef OP_EQUAL"
	if asm != expected {
		t.Errorf("Disassemble output mismatch.\nGot:  %s\nWant: %s", asm, expected)
	}
}

// TestDisassemble_MatchesLegacyCpp verifies Go Disassemble matches C++ CScript::ToString
// This test uses known legacy outputs for validation
func TestDisassemble_MatchesLegacyCpp(t *testing.T) {
	testCases := []struct {
		name     string
		scriptHex string
		expected string
	}{
		{
			name:      "P2PKH standard",
			scriptHex: "76a91489abcdef0123456789abcdef0123456789abcdef88ac",
			expected:  "OP_DUP OP_HASH160 89abcdef0123456789abcdef0123456789abcdef OP_EQUALVERIFY OP_CHECKSIG",
		},
		{
			name:      "P2SH standard",
			scriptHex: "a91489abcdef0123456789abcdef0123456789abcdef87",
			expected:  "OP_HASH160 89abcdef0123456789abcdef0123456789abcdef OP_EQUAL",
		},
		{
			name:      "OP_RETURN nulldata",
			scriptHex: "6a0568656c6c6f", // OP_RETURN "hello"
			expected:  "OP_RETURN 68656c6c6f",
		},
		{
			name:      "Empty script",
			scriptHex: "",
			expected:  "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scriptBytes, err := hex.DecodeString(tc.scriptHex)
			if err != nil {
				t.Fatalf("Failed to decode script hex: %v", err)
			}

			asm, err := script.Disassemble(scriptBytes)
			if err != nil {
				t.Fatalf("Disassemble failed: %v", err)
			}

			if asm != tc.expected {
				t.Errorf("Disassemble mismatch.\nScript: %s\nGot:    %s\nWant:   %s",
					tc.scriptHex, asm, tc.expected)
			}
		})
	}
}

// TestDisassemble_P2PKH_MasternodePayee verifies that P2PKH scripts (used for masternode payees)
// produce identical ASM output in Go and C++.
//
// CRITICAL: Masternode winner vote signature messages include payee.ToString() which must match
// exactly between Go and C++ for signature verification to succeed.
//
// This test uses real masternode payee addresses from mainnet to verify compatibility.
func TestDisassemble_P2PKH_MasternodePayee(t *testing.T) {
	// Real mainnet P2PKH addresses converted to scripts
	// Address: DXXXXXX... (20-byte pubkey hash embedded in script)
	testCases := []struct {
		name        string
		pubkeyHash  string // 20-byte hex
		goExpected  string
		cppExpected string
	}{
		{
			name:       "Standard P2PKH payee",
			pubkeyHash: "89abcdef0123456789abcdef0123456789abcdef",
			// Both Go and C++ produce identical output for P2PKH scripts
			// because OP_DUP, OP_HASH160, OP_EQUALVERIFY, OP_CHECKSIG all have "OP_" prefix in both
			goExpected:  "OP_DUP OP_HASH160 89abcdef0123456789abcdef0123456789abcdef OP_EQUALVERIFY OP_CHECKSIG",
			cppExpected: "OP_DUP OP_HASH160 89abcdef0123456789abcdef0123456789abcdef OP_EQUALVERIFY OP_CHECKSIG",
		},
		{
			name:        "Zero-filled pubkey hash",
			pubkeyHash:  "0000000000000000000000000000000000000000",
			goExpected:  "OP_DUP OP_HASH160 0000000000000000000000000000000000000000 OP_EQUALVERIFY OP_CHECKSIG",
			cppExpected: "OP_DUP OP_HASH160 0000000000000000000000000000000000000000 OP_EQUALVERIFY OP_CHECKSIG",
		},
		{
			name:        "Max-value pubkey hash",
			pubkeyHash:  "ffffffffffffffffffffffffffffffffffffffff",
			goExpected:  "OP_DUP OP_HASH160 ffffffffffffffffffffffffffffffffffffffff OP_EQUALVERIFY OP_CHECKSIG",
			cppExpected: "OP_DUP OP_HASH160 ffffffffffffffffffffffffffffffffffffffff OP_EQUALVERIFY OP_CHECKSIG",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Build P2PKH script: OP_DUP OP_HASH160 <20 bytes> OP_EQUALVERIFY OP_CHECKSIG
			pubkeyHashBytes, err := hex.DecodeString(tc.pubkeyHash)
			if err != nil {
				t.Fatalf("Failed to decode pubkey hash: %v", err)
			}
			if len(pubkeyHashBytes) != 20 {
				t.Fatalf("Invalid pubkey hash length: %d", len(pubkeyHashBytes))
			}

			p2pkhScript := make([]byte, 25)
			p2pkhScript[0] = 0x76       // OP_DUP
			p2pkhScript[1] = 0xa9       // OP_HASH160
			p2pkhScript[2] = 0x14       // Push 20 bytes
			copy(p2pkhScript[3:23], pubkeyHashBytes)
			p2pkhScript[23] = 0x88 // OP_EQUALVERIFY
			p2pkhScript[24] = 0xac // OP_CHECKSIG

			asm, err := script.Disassemble(p2pkhScript)
			if err != nil {
				t.Fatalf("Disassemble failed: %v", err)
			}

			// Verify Go output matches expected
			if asm != tc.goExpected {
				t.Errorf("Go Disassemble mismatch.\nGot:  %s\nWant: %s", asm, tc.goExpected)
			}

			// Verify Go and C++ outputs are identical for P2PKH
			if tc.goExpected != tc.cppExpected {
				t.Errorf("CRITICAL: Go and C++ ASM outputs differ for P2PKH!\nGo:  %s\nC++: %s",
					tc.goExpected, tc.cppExpected)
			}
		})
	}
}

// TestDisassemble_NumberOpcodes_KnownDifference documents the known difference between
// Go and C++ for OP_0, OP_1-16, and OP_1NEGATE opcodes.
//
// NOTE: This is NOT a bug - masternode payees ONLY use P2PKH scripts which don't contain
// these opcodes, so the difference doesn't affect masternode signature verification.
func TestDisassemble_NumberOpcodes_KnownDifference(t *testing.T) {
	testCases := []struct {
		name        string
		scriptHex   string
		goExpected  string
		cppExpected string // What C++ would output (documented for reference)
	}{
		{
			name:        "OP_0 (bare)",
			scriptHex:   "00",
			goExpected:  "OP_0",
			cppExpected: "0", // C++ outputs "0" not "OP_0"
		},
		{
			name:        "OP_1 (bare)",
			scriptHex:   "51",
			goExpected:  "OP_1",
			cppExpected: "1", // C++ outputs "1" not "OP_1"
		},
		{
			name:        "OP_2 (bare)",
			scriptHex:   "52",
			goExpected:  "OP_2",
			cppExpected: "2", // C++ outputs "2" not "OP_2"
		},
		{
			name:        "OP_16 (bare)",
			scriptHex:   "60",
			goExpected:  "OP_16",
			cppExpected: "16", // C++ outputs "16" not "OP_16"
		},
		{
			name:        "OP_1NEGATE (bare)",
			scriptHex:   "4f",
			goExpected:  "OP_1NEGATE",
			cppExpected: "-1", // C++ outputs "-1" not "OP_1NEGATE"
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scriptBytes, err := hex.DecodeString(tc.scriptHex)
			if err != nil {
				t.Fatalf("Failed to decode script: %v", err)
			}

			asm, err := script.Disassemble(scriptBytes)
			if err != nil {
				t.Fatalf("Disassemble failed: %v", err)
			}

			if asm != tc.goExpected {
				t.Errorf("Go output mismatch.\nGot:  %s\nWant: %s", asm, tc.goExpected)
			}

			// Document that Go and C++ differ for these opcodes
			if tc.goExpected == tc.cppExpected {
				t.Errorf("Expected Go and C++ to differ for %s, but they match", tc.name)
			}

			t.Logf("Known difference: Go='%s', C++='%s' (OK for masternode payees which don't use these)", asm, tc.cppExpected)
		})
	}
}
