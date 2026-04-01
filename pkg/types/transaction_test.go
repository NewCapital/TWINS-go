package types

import (
	"testing"
)

func TestTransactionHash(t *testing.T) {
	tx := &Transaction{
		Version: 1,
		Inputs: []*TxInput{
			{
				PreviousOutput: Outpoint{Hash: ZeroHash, Index: 0xffffffff},
				ScriptSig:      []byte{},
				Sequence:       0xffffffff,
			},
		},
		Outputs: []*TxOutput{
			{
				Value:        5000000000,
				ScriptPubKey: []byte{0x76, 0xa9, 0x14},
			},
		},
		LockTime: 0,
	}

	hash1 := tx.Hash()
	hash2 := tx.Hash()

	if !hash1.IsEqual(hash2) {
		t.Error("Same transaction should produce same hash")
	}

	if hash1.IsZero() {
		t.Error("Transaction hash should not be zero")
	}

	// Change a field and verify hash changes
	tx.Version = 2
	hash3 := tx.Hash()

	if hash1.IsEqual(hash3) {
		t.Error("Different transactions should produce different hashes")
	}
}

func TestIsCoinbase(t *testing.T) {
	// Create a coinbase transaction
	coinbaseTx := &Transaction{
		Version: 1,
		Inputs: []*TxInput{
			{
				PreviousOutput: Outpoint{Hash: ZeroHash, Index: 0xffffffff},
				ScriptSig:      []byte{0x03, 0x51, 0x0e, 0x00},
				Sequence:       0xffffffff,
			},
		},
		Outputs: []*TxOutput{
			{
				Value:        5000000000,
				ScriptPubKey: []byte{0x76, 0xa9, 0x14},
			},
		},
		LockTime: 0,
	}

	if !coinbaseTx.IsCoinbase() {
		t.Error("Transaction should be identified as coinbase")
	}

	// Create a regular transaction
	regularTx := &Transaction{
		Version: 1,
		Inputs: []*TxInput{
			{
				PreviousOutput: Outpoint{Hash: NewHash([]byte("previous transaction")), Index: 0},
				ScriptSig:      []byte{0x47, 0x30, 0x44},
				Sequence:       0xffffffff,
			},
		},
		Outputs: []*TxOutput{
			{
				Value:        1000000000,
				ScriptPubKey: []byte{0x76, 0xa9, 0x14},
			},
		},
		LockTime: 0,
	}

	if regularTx.IsCoinbase() {
		t.Error("Regular transaction should not be identified as coinbase")
	}

	// Test empty inputs
	emptyTx := &Transaction{
		Version:  1,
		Inputs:   []*TxInput{},
		Outputs:  []*TxOutput{},
		LockTime: 0,
	}

	if emptyTx.IsCoinbase() {
		t.Error("Transaction with no inputs should not be coinbase")
	}

	// Test multiple inputs
	multiInputTx := &Transaction{
		Version: 1,
		Inputs: []*TxInput{
			{PreviousOutput: Outpoint{Hash: ZeroHash, Index: 0xffffffff}, ScriptSig: []byte{}, Sequence: 0xffffffff},
			{PreviousOutput: Outpoint{Hash: ZeroHash, Index: 0}, ScriptSig: []byte{}, Sequence: 0xffffffff},
		},
		Outputs:  []*TxOutput{},
		LockTime: 0,
	}

	if multiInputTx.IsCoinbase() {
		t.Error("Transaction with multiple inputs should not be coinbase")
	}
}

func TestTransactionSerializeSize(t *testing.T) {
	tx := &Transaction{
		Version: 1,
		Inputs: []*TxInput{
			{
				PreviousOutput: Outpoint{Hash: ZeroHash, Index: 0},
				ScriptSig:      []byte{0x01, 0x02, 0x03, 0x04, 0x05},
				Sequence:       0xffffffff,
			},
		},
		Outputs: []*TxOutput{
			{
				Value:        5000000000,
				ScriptPubKey: []byte{0x76, 0xa9, 0x14, 0x88, 0xac},
			},
		},
		LockTime: 0,
	}

	size := tx.SerializeSize()

	// Calculate expected size manually using CompactSize varints
	expectedSize := 4 + // Version
		1 + // Input count (varint, 1 input < 253)
		32 + // PrevTxHash
		4 + // Index
		1 + // ScriptSig length (varint, 5 bytes < 253)
		5 + // ScriptSig data
		4 + // Sequence
		1 + // Output count (varint, 1 output < 253)
		8 + // Value
		1 + // ScriptPubKey length (varint, 5 bytes < 253)
		5 + // ScriptPubKey data
		4 // LockTime
	// Total: 70 bytes

	if size != expectedSize {
		t.Errorf("Expected serialize size %d, got %d", expectedSize, size)
	}

	// Test transaction with no inputs/outputs
	emptyTx := &Transaction{
		Version:  1,
		Inputs:   []*TxInput{},
		Outputs:  []*TxOutput{},
		LockTime: 0,
	}

	emptySize := emptyTx.SerializeSize()
	// Version(4) + InputCount varint(1) + OutputCount varint(1) + LockTime(4) = 10
	expectedEmptySize := 4 + 1 + 1 + 4

	if emptySize != expectedEmptySize {
		t.Errorf("Expected empty transaction size %d, got %d", expectedEmptySize, emptySize)
	}
}

func TestSignatureHash(t *testing.T) {
	tx := &Transaction{
		Version: 1,
		Inputs: []*TxInput{
			{
				PreviousOutput: Outpoint{Hash: NewHash([]byte("input1")), Index: 0},
				ScriptSig:      []byte{0x47, 0x30, 0x44},
				Sequence:       0xffffffff,
			},
			{
				PreviousOutput: Outpoint{Hash: NewHash([]byte("input2")), Index: 1},
				ScriptSig:      []byte{0x48, 0x31, 0x45},
				Sequence:       0xffffffff,
			},
		},
		Outputs: []*TxOutput{
			{
				Value:        1000000000,
				ScriptPubKey: []byte{0x76, 0xa9, 0x14},
			},
		},
		LockTime: 0,
	}

	scriptPubKey := []byte{0x76, 0xa9, 0x14, 0x88, 0xac}

	// Test signature hash for first input
	sigHash1 := tx.SignatureHash(0, scriptPubKey, SigHashAll)
	if sigHash1.IsZero() {
		t.Error("Signature hash should not be zero")
	}

	// Test signature hash for second input
	sigHash2 := tx.SignatureHash(1, scriptPubKey, SigHashAll)
	if sigHash2.IsZero() {
		t.Error("Signature hash should not be zero")
	}

	// Different inputs should produce different signature hashes
	if sigHash1.IsEqual(sigHash2) {
		t.Error("Different inputs should produce different signature hashes")
	}

	// Test same input produces same signature hash
	sigHash1_2 := tx.SignatureHash(0, scriptPubKey, SigHashAll)
	if !sigHash1.IsEqual(sigHash1_2) {
		t.Error("Same input should produce same signature hash")
	}

	// Test out of bounds input index
	sigHashInvalid := tx.SignatureHash(999, scriptPubKey, SigHashAll)
	if !sigHashInvalid.IsZero() {
		t.Error("Invalid input index should return zero hash")
	}

	// Test different scriptPubKey produces different hash
	differentScript := []byte{0x51}
	sigHashDifferent := tx.SignatureHash(0, differentScript, SigHashAll)
	if sigHash1.IsEqual(sigHashDifferent) {
		t.Error("Different scriptPubKey should produce different signature hash")
	}
}

func TestTransactionInputOutput(t *testing.T) {
	// Test TxInput creation
	input := &TxInput{
		PreviousOutput: Outpoint{Hash: NewHash([]byte("previous")), Index: 5},
		ScriptSig:      []byte{0x01, 0x02, 0x03},
		Sequence:       0xfffffffe,
	}

	if input.PreviousOutput.Hash.IsZero() {
		t.Error("TxInput should have non-zero previous transaction hash")
	}

	if input.PreviousOutput.Index != 5 {
		t.Errorf("Expected index 5, got %d", input.PreviousOutput.Index)
	}

	// Test TxOutput creation
	output := &TxOutput{
		Value:        2500000000,
		ScriptPubKey: []byte{0x76, 0xa9, 0x14, 0x88, 0xac},
	}

	if output.Value != 2500000000 {
		t.Errorf("Expected value 2500000000, got %d", output.Value)
	}

	if len(output.ScriptPubKey) != 5 {
		t.Errorf("Expected ScriptPubKey length 5, got %d", len(output.ScriptPubKey))
	}
}

func TestTransactionValidation(t *testing.T) {
	// Test transaction with valid structure
	validTx := &Transaction{
		Version: 1,
		Inputs: []*TxInput{
			{
				PreviousOutput: Outpoint{Hash: NewHash([]byte("valid input")), Index: 0},
				ScriptSig:      []byte{0x47, 0x30, 0x44},
				Sequence:       0xffffffff,
			},
		},
		Outputs: []*TxOutput{
			{
				Value:        1000000000,
				ScriptPubKey: []byte{0x76, 0xa9, 0x14},
			},
		},
		LockTime: 0,
	}

	// Should be able to calculate hash without errors
	hash := validTx.Hash()
	if hash.IsZero() {
		t.Error("Valid transaction should have non-zero hash")
	}

	// Should be able to calculate serialize size
	size := validTx.SerializeSize()
	if size <= 0 {
		t.Error("Valid transaction should have positive serialize size")
	}
}

func BenchmarkTransactionHash(b *testing.B) {
	tx := &Transaction{
		Version: 1,
		Inputs: []*TxInput{
			{
				PreviousOutput: Outpoint{Hash: NewHash([]byte("benchmark input")), Index: 0},
				ScriptSig:      []byte{0x47, 0x30, 0x44, 0x02, 0x20},
				Sequence:       0xffffffff,
			},
		},
		Outputs: []*TxOutput{
			{
				Value:        1000000000,
				ScriptPubKey: []byte{0x76, 0xa9, 0x14, 0x88, 0xac},
			},
		},
		LockTime: 0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tx.Hash()
	}
}

func BenchmarkSignatureHash(b *testing.B) {
	tx := &Transaction{
		Version: 1,
		Inputs: []*TxInput{
			{
				PreviousOutput: Outpoint{Hash: NewHash([]byte("benchmark input")), Index: 0},
				ScriptSig:      []byte{0x47, 0x30, 0x44, 0x02, 0x20},
				Sequence:       0xffffffff,
			},
		},
		Outputs: []*TxOutput{
			{
				Value:        1000000000,
				ScriptPubKey: []byte{0x76, 0xa9, 0x14, 0x88, 0xac},
			},
		},
		LockTime: 0,
	}

	scriptPubKey := []byte{0x76, 0xa9, 0x14, 0x88, 0xac}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tx.SignatureHash(0, scriptPubKey, SigHashAll)
	}
}
