package binary

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/twins-dev/twins-core/pkg/crypto"
)

func TestAnalyzeScript_P2PKH(t *testing.T) {
	// P2PKH script: OP_DUP OP_HASH160 <20 bytes> OP_EQUALVERIFY OP_CHECKSIG
	pubKeyHash, _ := hex.DecodeString("89abcdefabbaabbaabbaabbaabbaabbaabbaabba")
	script := make([]byte, 25)
	script[0] = 0x76 // OP_DUP
	script[1] = 0xa9 // OP_HASH160
	script[2] = 0x14 // Push 20 bytes
	copy(script[3:23], pubKeyHash)
	script[23] = 0x88 // OP_EQUALVERIFY
	script[24] = 0xac // OP_CHECKSIG

	scriptType, scriptHash := AnalyzeScript(script)

	assert.Equal(t, ScriptTypeP2PKH, scriptType)
	assert.Equal(t, pubKeyHash, scriptHash[:])
}

func TestAnalyzeScript_P2SH(t *testing.T) {
	// P2SH script: OP_HASH160 <20 bytes> OP_EQUAL
	scriptHash160, _ := hex.DecodeString("89abcdefabbaabbaabbaabbaabbaabbaabbaabba")
	script := make([]byte, 23)
	script[0] = 0xa9 // OP_HASH160
	script[1] = 0x14 // Push 20 bytes
	copy(script[2:22], scriptHash160)
	script[22] = 0x87 // OP_EQUAL

	scriptType, scriptHash := AnalyzeScript(script)

	assert.Equal(t, ScriptTypeP2SH, scriptType)
	assert.Equal(t, scriptHash160, scriptHash[:])
}

func TestAnalyzeScript_P2PK_Compressed(t *testing.T) {
	// P2PK script with compressed pubkey: <33 bytes> OP_CHECKSIG
	// Compressed pubkey starts with 0x02 or 0x03
	compressedPubKey, _ := hex.DecodeString("0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798")

	script := make([]byte, 35)
	script[0] = 0x21 // Push 33 bytes
	copy(script[1:34], compressedPubKey)
	script[34] = 0xac // OP_CHECKSIG

	scriptType, scriptHash := AnalyzeScript(script)

	// Should extract Hash160 of the public key
	expectedHash := crypto.Hash160(compressedPubKey)

	assert.Equal(t, ScriptTypeP2PK, scriptType)
	assert.Equal(t, expectedHash, scriptHash[:])
}

func TestAnalyzeScript_P2PK_Uncompressed(t *testing.T) {
	// P2PK script with uncompressed pubkey: <65 bytes> OP_CHECKSIG
	// Uncompressed pubkey starts with 0x04
	uncompressedPubKey, _ := hex.DecodeString("0479be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798483ada7726a3c4655da4fbfc0e1108a8fd17b448a68554199c47d08ffb10d4b8")

	script := make([]byte, 67)
	script[0] = 0x41 // Push 65 bytes
	copy(script[1:66], uncompressedPubKey)
	script[66] = 0xac // OP_CHECKSIG

	scriptType, scriptHash := AnalyzeScript(script)

	// Should extract Hash160 of the public key
	expectedHash := crypto.Hash160(uncompressedPubKey)

	assert.Equal(t, ScriptTypeP2PK, scriptType)
	assert.Equal(t, expectedHash, scriptHash[:])
}

func TestAnalyzeScript_Unknown(t *testing.T) {
	// Invalid script
	script := []byte{0x00, 0x01, 0x02}

	scriptType, scriptHash := AnalyzeScript(script)

	assert.Equal(t, ScriptTypeUnknown, scriptType)
	assert.Equal(t, [20]byte{}, scriptHash)
}

func TestScriptHashToAddressBinary_P2PKH_Mainnet(t *testing.T) {
	pubKeyHash, _ := hex.DecodeString("89abcdefabbaabbaabbaabbaabbaabbaabbaabba")
	var scriptHash [20]byte
	copy(scriptHash[:], pubKeyHash)

	addressBinary := ScriptHashToAddressBinary(ScriptTypeP2PKH, scriptHash, false)

	assert.NotNil(t, addressBinary)
	assert.Equal(t, 21, len(addressBinary))
	assert.Equal(t, byte(0x49), addressBinary[0]) // TWINS mainnet P2PKH prefix
	assert.Equal(t, pubKeyHash, addressBinary[1:])
}

func TestScriptHashToAddressBinary_P2PKH_Testnet(t *testing.T) {
	pubKeyHash, _ := hex.DecodeString("89abcdefabbaabbaabbaabbaabbaabbaabbaabba")
	var scriptHash [20]byte
	copy(scriptHash[:], pubKeyHash)

	addressBinary := ScriptHashToAddressBinary(ScriptTypeP2PKH, scriptHash, true)

	assert.NotNil(t, addressBinary)
	assert.Equal(t, 21, len(addressBinary))
	assert.Equal(t, byte(0x6F), addressBinary[0]) // TWINS testnet P2PKH prefix
	assert.Equal(t, pubKeyHash, addressBinary[1:])
}

func TestScriptHashToAddressBinary_P2SH_Mainnet(t *testing.T) {
	scriptHash160, _ := hex.DecodeString("89abcdefabbaabbaabbaabbaabbaabbaabbaabba")
	var scriptHash [20]byte
	copy(scriptHash[:], scriptHash160)

	addressBinary := ScriptHashToAddressBinary(ScriptTypeP2SH, scriptHash, false)

	assert.NotNil(t, addressBinary)
	assert.Equal(t, 21, len(addressBinary))
	assert.Equal(t, byte(0x53), addressBinary[0]) // TWINS mainnet P2SH prefix
	assert.Equal(t, scriptHash160, addressBinary[1:])
}

func TestScriptHashToAddressBinary_P2PK_Mainnet(t *testing.T) {
	// P2PK should use P2PKH prefix (same as P2PKH)
	pubKeyHash, _ := hex.DecodeString("89abcdefabbaabbaabbaabbaabbaabbaabbaabba")
	var scriptHash [20]byte
	copy(scriptHash[:], pubKeyHash)

	addressBinary := ScriptHashToAddressBinary(ScriptTypeP2PK, scriptHash, false)

	assert.NotNil(t, addressBinary)
	assert.Equal(t, 21, len(addressBinary))
	assert.Equal(t, byte(0x49), addressBinary[0]) // TWINS mainnet P2PKH prefix (same as P2PKH)
	assert.Equal(t, pubKeyHash, addressBinary[1:])
}

func TestScriptHashToAddressBinary_Unknown(t *testing.T) {
	var scriptHash [20]byte

	addressBinary := ScriptHashToAddressBinary(ScriptTypeUnknown, scriptHash, false)

	assert.Nil(t, addressBinary)
}

func TestScriptHashToAddressBinary_EmptyHash(t *testing.T) {
	var emptyHash [20]byte

	addressBinary := ScriptHashToAddressBinary(ScriptTypeP2PKH, emptyHash, false)

	assert.Nil(t, addressBinary)
}

// Edge case tests for invalid public keys

func TestAnalyzeScript_P2PK_InvalidCompressedKey(t *testing.T) {
	// Invalid compressed key - all zeros (not a valid curve point)
	invalidPubKey := make([]byte, 33)
	invalidPubKey[0] = 0x02 // Valid prefix
	// Rest is zeros - not a valid secp256k1 point

	script := make([]byte, 35)
	script[0] = 0x21 // Push 33 bytes
	copy(script[1:34], invalidPubKey)
	script[34] = 0xac // OP_CHECKSIG

	scriptType, scriptHash := AnalyzeScript(script)

	// Should reject invalid public key and return Unknown
	assert.Equal(t, ScriptTypeUnknown, scriptType)
	assert.Equal(t, [20]byte{}, scriptHash)
}

func TestAnalyzeScript_P2PK_InvalidUncompressedKey(t *testing.T) {
	// Invalid uncompressed key - all zeros (not a valid curve point)
	invalidPubKey := make([]byte, 65)
	invalidPubKey[0] = 0x04 // Valid prefix
	// Rest is zeros - not a valid secp256k1 point

	script := make([]byte, 67)
	script[0] = 0x41 // Push 65 bytes
	copy(script[1:66], invalidPubKey)
	script[66] = 0xac // OP_CHECKSIG

	scriptType, scriptHash := AnalyzeScript(script)

	// Should reject invalid public key and return Unknown
	assert.Equal(t, ScriptTypeUnknown, scriptType)
	assert.Equal(t, [20]byte{}, scriptHash)
}

func TestAnalyzeScript_P2PK_WrongLengthPrefix(t *testing.T) {
	// Valid pubkey but wrong length prefix
	validPubKey, _ := hex.DecodeString("0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798")

	script := make([]byte, 35)
	script[0] = 0x20 // Wrong: should be 0x21 for 33 bytes
	copy(script[1:34], validPubKey)
	script[34] = 0xac

	scriptType, _ := AnalyzeScript(script)

	assert.Equal(t, ScriptTypeUnknown, scriptType)
}

func TestAnalyzeScript_P2PK_InvalidPrefix(t *testing.T) {
	// Compressed pubkey with invalid prefix (not 0x02 or 0x03)
	invalidPubKey := make([]byte, 33)
	invalidPubKey[0] = 0x05 // Invalid: should be 0x02 or 0x03

	script := make([]byte, 35)
	script[0] = 0x21
	copy(script[1:34], invalidPubKey)
	script[34] = 0xac

	scriptType, _ := AnalyzeScript(script)

	assert.Equal(t, ScriptTypeUnknown, scriptType)
}

func TestAnalyzeScript_P2PK_MissingOpCheckSig(t *testing.T) {
	// Valid pubkey but missing OP_CHECKSIG
	validPubKey, _ := hex.DecodeString("0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798")

	script := make([]byte, 35)
	script[0] = 0x21
	copy(script[1:34], validPubKey)
	script[34] = 0x00 // Wrong: should be 0xac (OP_CHECKSIG)

	scriptType, _ := AnalyzeScript(script)

	assert.Equal(t, ScriptTypeUnknown, scriptType)
}
