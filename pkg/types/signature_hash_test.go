package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"testing"
)

// TestSignatureHash_ByteLevelMatchesLegacyFormat hand-constructs the exact
// serialization that legacy C++ CTransactionSignatureSerializer produces
// (legacy/src/script/interpreter.cpp:983-1078) for a fixed 2-in 1-out tx,
// then compares the double-SHA256 against Transaction.SignatureHash. If
// SignatureHash diverges in even one byte from the legacy spec, this test
// will fail and pinpoint the issue.
//
// Fixed tx layout:
//   version     = 1
//   inputs[0]   = prevout(hash=aaaa...aa, index=1) + sequence=0xfffffffe
//   inputs[1]   = prevout(hash=bbbb...bb, index=0) + sequence=0xffffffff
//   outputs[0]  = value=10_000, P2PKH with hash = 0x11 * 20
//   locktime    = 0
//
// scriptPubKey passed to sighash (P2PKH, 25 bytes):
//   76 a9 14 <20 bytes 0x22> 88 ac
func TestSignatureHash_ByteLevelMatchesLegacyFormat(t *testing.T) {
	var in1Hash Hash
	for i := range in1Hash {
		in1Hash[i] = 0xaa
	}
	var in2Hash Hash
	for i := range in2Hash {
		in2Hash[i] = 0xbb
	}

	pkh := bytes.Repeat([]byte{0x22}, 20)
	scriptPubKey := append([]byte{0x76, 0xa9, 0x14}, pkh...)
	scriptPubKey = append(scriptPubKey, 0x88, 0xac)

	outScriptHash := bytes.Repeat([]byte{0x11}, 20)
	outScript := append([]byte{0x76, 0xa9, 0x14}, outScriptHash...)
	outScript = append(outScript, 0x88, 0xac)

	tx := &Transaction{
		Version: 1,
		Inputs: []*TxInput{
			{
				PreviousOutput: Outpoint{Hash: in1Hash, Index: 1},
				Sequence:       0xfffffffe,
			},
			{
				PreviousOutput: Outpoint{Hash: in2Hash, Index: 0},
				Sequence:       0xffffffff,
			},
		},
		Outputs: []*TxOutput{
			{Value: 10_000, ScriptPubKey: outScript},
		},
		LockTime: 0,
	}

	// Build expected serialization byte-by-byte per CTransactionSignatureSerializer.
	// Sign input[0] with scriptPubKey (SIGHASH_ALL).
	var buf bytes.Buffer

	// version (LE32)
	_ = binary.Write(&buf, binary.LittleEndian, uint32(1))

	// input count (varint)
	_ = WriteCompactSize(&buf, 2)

	// input[0]: full scriptPubKey (cleaned — no OP_CODESEPARATOR in this script)
	buf.Write(in1Hash[:])
	_ = binary.Write(&buf, binary.LittleEndian, uint32(1))
	_ = WriteCompactSize(&buf, uint64(len(scriptPubKey)))
	buf.Write(scriptPubKey)
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0xfffffffe))

	// input[1]: empty script
	buf.Write(in2Hash[:])
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0))
	_ = WriteCompactSize(&buf, 0)
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0xffffffff))

	// output count (varint)
	_ = WriteCompactSize(&buf, 1)

	// output[0]
	_ = binary.Write(&buf, binary.LittleEndian, int64(10_000))
	_ = WriteCompactSize(&buf, uint64(len(outScript)))
	buf.Write(outScript)

	// locktime (LE32)
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0))

	// sighash type (LE32) — appended to preimage by Transaction.SignatureHash
	_ = binary.Write(&buf, binary.LittleEndian, uint32(SigHashAll))

	// Double-SHA256 the expected serialization.
	first := sha256.Sum256(buf.Bytes())
	expected := sha256.Sum256(first[:])

	got := tx.SignatureHash(0, scriptPubKey, SigHashAll)

	if got != Hash(expected) {
		t.Fatalf("SignatureHash diverges from hand-built legacy serialization:\n  expected: %x\n  got:      %x\n  preimage len: %d",
			expected, got, buf.Len())
	}
}
