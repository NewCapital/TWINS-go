package types

import (
	"testing"
	"time"
)

func TestBlockHeaderHash(t *testing.T) {
	header := &BlockHeader{
		Version:               1,
		PrevBlockHash:         ZeroHash,
		MerkleRoot:            ZeroHash,
		Timestamp:             uint32(time.Now().Unix()),
		Bits:                  0x1d00ffff,
		Nonce:                 12345,
		AccumulatorCheckpoint: ZeroHash,
	}

	hash1 := header.Hash()
	hash2 := header.Hash()

	if !hash1.IsEqual(hash2) {
		t.Error("Same header should produce same hash")
	}

	if hash1.IsZero() {
		t.Error("Header hash should not be zero")
	}

	// Change a field and verify hash changes
	header.Nonce = 54321
	hash3 := header.Hash()

	if hash1.IsEqual(hash3) {
		t.Error("Different headers should produce different hashes")
	}
}

func TestBlockHash(t *testing.T) {
	header := &BlockHeader{
		Version:       1,
		PrevBlockHash: ZeroHash,
		MerkleRoot:            ZeroHash,
		Timestamp:             uint32(time.Now().Unix()),
		Bits:                  0x1d00ffff,
		Nonce:                 12345,
		AccumulatorCheckpoint: ZeroHash,
	}

	block := &Block{
		Header:       header,
		Transactions: []*Transaction{},
	}

	blockHash := block.Hash()
	headerHash := header.Hash()

	if !blockHash.IsEqual(headerHash) {
		t.Error("Block hash should equal header hash")
	}
}

func TestCalculateMerkleRoot(t *testing.T) {
	// Test empty block
	block := &Block{
		Header:       &BlockHeader{},
		Transactions: []*Transaction{},
	}

	merkleRoot := block.CalculateMerkleRoot()
	if !merkleRoot.IsZero() {
		t.Error("Empty block should have zero merkle root")
	}

	// Test single transaction
	tx := &Transaction{
		Version:  1,
		Inputs:   []*TxInput{},
		Outputs:  []*TxOutput{},
		LockTime: 0,
	}

	block.Transactions = []*Transaction{tx}
	merkleRoot = block.CalculateMerkleRoot()
	expectedRoot := tx.Hash()

	if !merkleRoot.IsEqual(expectedRoot) {
		t.Error("Single transaction merkle root should equal transaction hash")
	}

	// Test two transactions
	tx2 := &Transaction{
		Version:  2,
		Inputs:   []*TxInput{},
		Outputs:  []*TxOutput{},
		LockTime: 1,
	}

	block.Transactions = []*Transaction{tx, tx2}
	merkleRoot = block.CalculateMerkleRoot()

	if merkleRoot.IsZero() {
		t.Error("Two transaction merkle root should not be zero")
	}

	// Test that merkle root is deterministic
	merkleRoot2 := block.CalculateMerkleRoot()
	if !merkleRoot.IsEqual(merkleRoot2) {
		t.Error("Merkle root calculation should be deterministic")
	}

	// Test odd number of transactions (3)
	tx3 := &Transaction{
		Version:  3,
		Inputs:   []*TxInput{},
		Outputs:  []*TxOutput{},
		LockTime: 2,
	}

	block.Transactions = []*Transaction{tx, tx2, tx3}
	merkleRoot3 := block.CalculateMerkleRoot()

	if merkleRoot3.IsZero() {
		t.Error("Three transaction merkle root should not be zero")
	}
}

func TestBlockHeaderSerializeSize(t *testing.T) {
	// Test version 0-3 (without accumulator checkpoint)
	header := &BlockHeader{Version: 1}
	size := header.SerializeSize()
	expectedSize := 4 + 32 + 32 + 4 + 4 + 4 // 80 bytes
	if size != expectedSize {
		t.Errorf("BlockHeader (v1) serialize size should be %d, got %d", expectedSize, size)
	}

	// Test version > 3 (with accumulator checkpoint)
	headerV4 := &BlockHeader{Version: 4}
	sizeV4 := headerV4.SerializeSize()
	expectedSizeV4 := 4 + 32 + 32 + 4 + 4 + 4 + 32 // 112 bytes
	if sizeV4 != expectedSizeV4 {
		t.Errorf("BlockHeader (v4) serialize size should be %d, got %d", expectedSizeV4, sizeV4)
	}
}

func TestBlockSerializeSize(t *testing.T) {
	header := &BlockHeader{}

	// Create a simple transaction
	tx := &Transaction{
		Version: 1,
		Inputs: []*TxInput{
			{
				PreviousOutput: Outpoint{Hash: ZeroHash, Index: 0},
				ScriptSig:      []byte{0x01, 0x02, 0x03},
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

	block := &Block{
		Header:       header,
		Transactions: []*Transaction{tx},
	}

	size := block.SerializeSize()
	headerSize := header.SerializeSize()

	if size <= headerSize {
		t.Error("Block size should be larger than header size")
	}

	// Size should include transaction data
	if size < headerSize+tx.SerializeSize() {
		t.Error("Block size should include transaction sizes")
	}
}

func TestMerkleTreeProperties(t *testing.T) {
	// Create multiple transactions
	var transactions []*Transaction
	for i := 0; i < 8; i++ {
		tx := &Transaction{
			Version:  uint32(i + 1),
			Inputs:   []*TxInput{},
			Outputs:  []*TxOutput{},
			LockTime: uint32(i),
		}
		transactions = append(transactions, tx)
	}

	block := &Block{
		Header:       &BlockHeader{},
		Transactions: transactions,
	}

	merkleRoot := block.CalculateMerkleRoot()

	// Test that changing any transaction changes the merkle root
	originalTx := transactions[0]
	modifiedTx := &Transaction{
		Version:  999,
		Inputs:   []*TxInput{},
		Outputs:  []*TxOutput{},
		LockTime: 999,
	}

	block.Transactions[0] = modifiedTx
	newMerkleRoot := block.CalculateMerkleRoot()

	if merkleRoot.IsEqual(newMerkleRoot) {
		t.Error("Changing a transaction should change the merkle root")
	}

	// Restore original transaction
	block.Transactions[0] = originalTx
	restoredMerkleRoot := block.CalculateMerkleRoot()

	if !merkleRoot.IsEqual(restoredMerkleRoot) {
		t.Error("Restoring original transaction should restore merkle root")
	}
}

func BenchmarkBlockHeaderHash(b *testing.B) {
	header := &BlockHeader{
		Version:       1,
		PrevBlockHash: ZeroHash,
		MerkleRoot:            ZeroHash,
		Timestamp:             uint32(time.Now().Unix()),
		Bits:                  0x1d00ffff,
		Nonce:                 12345,
		AccumulatorCheckpoint: ZeroHash,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = header.Hash()
	}
}

func BenchmarkCalculateMerkleRoot(b *testing.B) {
	// Create 1000 transactions
	var transactions []*Transaction
	for i := 0; i < 1000; i++ {
		tx := &Transaction{
			Version:  uint32(i + 1),
			Inputs:   []*TxInput{},
			Outputs:  []*TxOutput{},
			LockTime: uint32(i),
		}
		transactions = append(transactions, tx)
	}

	block := &Block{
		Header:       &BlockHeader{},
		Transactions: transactions,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = block.CalculateMerkleRoot()
	}
}
