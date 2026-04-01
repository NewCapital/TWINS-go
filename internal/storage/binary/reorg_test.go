package binary

import (
	"encoding/binary"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twins-dev/twins-core/internal/storage"
	"github.com/twins-dev/twins-core/pkg/types"
)

// TestDeleteBlockWithSync tests that DeleteBlock properly cleans up all indexes and calls Sync
func TestDeleteBlockWithSync(t *testing.T) {
	// Create temporary database
	tmpDir, err := os.MkdirTemp("", "reorg-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Initialize storage
	config := &storage.StorageConfig{
		Path:            tmpDir,
		CacheSize:       1 << 20, // 1MB
		WriteBuffer:     2,       // Minimum for Pebble
		MaxOpenFiles:    10,      // Minimum required
		CompressionType: "snappy", // Default compression
	}
	stor, err := NewBinaryStorage(config)
	require.NoError(t, err)
	defer stor.Close()

	// Create a test block with transactions
	block := createTestBlock()
	height := uint32(100)

	// Store block with height
	batch := stor.NewBatch()
	binaryBatch := batch.(*BinaryBatch)

	// IMPORTANT: Store block index FIRST (for height lookup)
	err = binaryBatch.StoreBlockIndex(block.Hash(), height)
	require.NoError(t, err)
	err = binaryBatch.StoreBlockWithHeight(block, height)
	require.NoError(t, err)

	// Store some UTXOs from the block
	for i, tx := range block.Transactions {
		for j, output := range tx.Outputs {
			outpoint := types.Outpoint{
				Hash:  tx.Hash(),
				Index: uint32(j),
			}
			err = binaryBatch.StoreUTXO(outpoint, output, height, i == 0) // First tx is coinbase
			require.NoError(t, err)
		}
	}

	// Commit the batch
	err = batch.Commit()
	require.NoError(t, err)

	// Verify block exists
	storedBlock, err := stor.GetBlock(block.Hash())
	require.NoError(t, err)
	assert.Equal(t, block.Hash(), storedBlock.Hash())

	// Verify transactions exist
	for _, tx := range block.Transactions {
		storedTx, err := stor.GetTransaction(tx.Hash())
		require.NoError(t, err)
		assert.Equal(t, tx.Hash(), storedTx.Hash())
	}

	// Delete the block
	err = stor.DeleteBlock(block.Hash())
	require.NoError(t, err)

	// Verify block is deleted
	_, err = stor.GetBlock(block.Hash())
	assert.Error(t, err)

	// Verify transactions are deleted
	for _, tx := range block.Transactions {
		_, err = stor.GetTransaction(tx.Hash())
		assert.Error(t, err, "Transaction %x should be deleted", tx.Hash())
	}

	// Verify height index is deleted
	_, err = stor.GetBlockHeight(block.Hash())
	assert.Error(t, err)

	// Verify reverse height index is deleted
	_, err = stor.GetBlockHashByHeight(height)
	assert.Error(t, err)

	// Verify address history is cleaned up
	// We can't directly check this without exposing internal methods,
	// but we can verify no crash occurs on subsequent operations

	// Add a new block at the same height to verify no conflicts
	newBlock := createTestBlock()
	batch = stor.NewBatch()
	binaryBatch = batch.(*BinaryBatch)
	err = binaryBatch.StoreBlockWithHeight(newBlock, height)
	require.NoError(t, err)
	err = binaryBatch.StoreBlockIndex(newBlock.Hash(), height)
	require.NoError(t, err)
	err = batch.Commit()
	require.NoError(t, err)

	// Verify new block is stored correctly
	storedNewBlock, err := stor.GetBlock(newBlock.Hash())
	require.NoError(t, err)
	assert.Equal(t, newBlock.Hash(), storedNewBlock.Hash())
}

// TestDeleteBlockDuringReorg simulates a reorg scenario
func TestDeleteBlockDuringReorg(t *testing.T) {
	// Create temporary database
	tmpDir, err := os.MkdirTemp("", "reorg-scenario-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Initialize storage
	config := &storage.StorageConfig{
		Path:            tmpDir,
		CacheSize:       1 << 20, // 1MB
		WriteBuffer:     2,       // Minimum for Pebble
		MaxOpenFiles:    10,      // Minimum required
		CompressionType: "snappy", // Default compression
	}
	stor, err := NewBinaryStorage(config)
	require.NoError(t, err)
	defer stor.Close()

	// Create a chain of 3 blocks with unique content
	blocks := make([]*types.Block, 3)
	for i := range blocks {
		block := createTestBlock()
		// Make each block unique by changing the merkle root
		block.Header.MerkleRoot = types.Hash{byte(i), 0xaa, 0xbb}
		// Make timestamp unique
		block.Header.Timestamp = uint32(1234567890 + i)
		// Make transactions unique by modifying output values
		for j, tx := range block.Transactions {
			if len(tx.Outputs) > 0 {
				// Make the output value unique per block
				tx.Outputs[0].Value = int64((50 + i*10 + j) * 1e8)
			}
		}
		blocks[i] = block
	}

	// Store all blocks
	for i, block := range blocks {
		height := uint32(100 + i)
		batch := stor.NewBatch()
		binaryBatch := batch.(*BinaryBatch)

		// IMPORTANT: Store block index FIRST (for height lookup)
		err = binaryBatch.StoreBlockIndex(block.Hash(), height)
		require.NoError(t, err)
		err = binaryBatch.StoreBlockWithHeight(block, height)
		require.NoError(t, err)

		// Store address indexes for transactions
		blockHash := block.Hash()
		for txIdx, tx := range block.Transactions {
			// Simulate address indexing
			addressBinary := []byte{0x00} // network byte
			addressBinary = append(addressBinary, make([]byte, 20)...) // dummy address hash
			err = binaryBatch.IndexTransactionByAddress(addressBinary, tx.Hash(), height, uint32(txIdx), 100000000, false, blockHash)
			require.NoError(t, err)
		}

		err = batch.Commit()
		require.NoError(t, err)
	}

	// Simulate reorg: Delete blocks 2 and 3 (heights 101 and 102)
	for i := 2; i >= 1; i-- {
		err = stor.DeleteBlock(blocks[i].Hash())
		require.NoError(t, err)

		// Verify block is deleted
		_, err = stor.GetBlock(blocks[i].Hash())
		assert.Error(t, err, "Block at index %d should be deleted", i)
	}

	// Verify first block still exists
	block0, err := stor.GetBlock(blocks[0].Hash())
	require.NoError(t, err)
	assert.Equal(t, blocks[0].Hash(), block0.Hash())

	// Store new blocks at heights 101 and 102 (simulating new chain)
	newBlocks := make([]*types.Block, 2)
	for i := range newBlocks {
		newBlocks[i] = createTestBlock()
		height := uint32(101 + i)

		batch := stor.NewBatch()
		binaryBatch := batch.(*BinaryBatch)
		err = binaryBatch.StoreBlockWithHeight(newBlocks[i], height)
		require.NoError(t, err)
		err = binaryBatch.StoreBlockIndex(newBlocks[i].Hash(), height)
		require.NoError(t, err)
		err = batch.Commit()
		require.NoError(t, err)
	}

	// Verify new blocks are stored correctly
	for i, block := range newBlocks {
		height := uint32(101 + i)
		storedBlock, err := stor.GetBlock(block.Hash())
		require.NoError(t, err)
		assert.Equal(t, block.Hash(), storedBlock.Hash())

		// Verify height mapping is correct
		hash, err := stor.GetBlockHashByHeight(height)
		require.NoError(t, err)
		assert.Equal(t, block.Hash(), hash)
	}
}


// createUniqueTestBlock creates a test block with unique transactions based on the seed value.
func createUniqueTestBlock(seed byte) *types.Block {
	coinbaseTx := &types.Transaction{
		Version: 1,
		Inputs:  []*types.TxInput{},
		Outputs: []*types.TxOutput{
			{
				Value: int64(50+seed) * 1e8,
				ScriptPubKey: []byte{
					0x76, 0xa9, 0x14,
					seed, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09,
					0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13,
					0x88, 0xac,
				},
			},
		},
		LockTime: 0,
	}

	regularTx := &types.Transaction{
		Version: 1,
		Inputs: []*types.TxInput{
			{
				PreviousOutput: types.Outpoint{
					Hash:  types.Hash{seed, 0x02, 0x03},
					Index: 0,
				},
				ScriptSig: []byte{0x48, 0x30, seed},
				Sequence:  0xffffffff,
			},
		},
		Outputs: []*types.TxOutput{
			{
				Value: int64(25+seed) * 1e8,
				ScriptPubKey: []byte{
					0xa9, 0x14,
					seed, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x29,
					0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f, 0x30, 0x31, 0x32, 0x33,
					0x87,
				},
			},
		},
		LockTime: 0,
	}

	return &types.Block{
		Header: &types.BlockHeader{
			Version:       1,
			PrevBlockHash: types.Hash{0xff, seed},
			MerkleRoot:    types.Hash{seed, 0xbb},
			Timestamp:     uint32(1234567890 + int(seed)),
			Bits:          0x1d00ffff,
			Nonce:         uint32(12345 + int(seed)),
		},
		Transactions: []*types.Transaction{coinbaseTx, regularTx},
	}
}

// TestCleanOrphanedBlocks tests that orphaned hash→height entries from fork blocks
// are fully cleaned up after rollback.
func TestCleanOrphanedBlocks(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "orphan-cleanup-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	config := &storage.StorageConfig{
		Path:            tmpDir,
		CacheSize:       1 << 20,
		WriteBuffer:     2,
		MaxOpenFiles:    10,
		CompressionType: "snappy",
	}
	stor, err := NewBinaryStorage(config)
	require.NoError(t, err)
	defer stor.Close()

	// Store a "good" block at height 100 (unique seed 0)
	goodBlock := createUniqueTestBlock(0)
	{
		batch := stor.NewBatch()
		bb := batch.(*BinaryBatch)
		require.NoError(t, bb.StoreBlockIndex(goodBlock.Hash(), 100))
		require.NoError(t, bb.StoreBlockWithHeight(goodBlock, 100))
		require.NoError(t, batch.Commit())
	}

	// Store chain blocks at heights 101 and 102 (seeds 1, 2)
	chainBlocks := make([]*types.Block, 2)
	for i := range chainBlocks {
		b := createUniqueTestBlock(byte(i + 1))
		chainBlocks[i] = b
		height := uint32(101 + i)

		batch := stor.NewBatch()
		bb := batch.(*BinaryBatch)
		require.NoError(t, bb.StoreBlockIndex(b.Hash(), height))
		require.NoError(t, bb.StoreBlockWithHeight(b, height))
		require.NoError(t, batch.Commit())
	}

	// Simulate orphaned fork blocks at heights 101 and 102 (seeds 10, 11)
	forkBlocks := make([]*types.Block, 2)
	for i := range forkBlocks {
		b := createUniqueTestBlock(byte(i + 10))
		forkBlocks[i] = b
		height := uint32(101 + i)

		batch := stor.NewBatch()
		bb := batch.(*BinaryBatch)
		require.NoError(t, bb.StoreBlockWithHeight(b, height))
		require.NoError(t, bb.StoreBlockIndex(b.Hash(), height))
		require.NoError(t, batch.Commit())

		// Restore height→hash to point to chain block
		restoreBatch := stor.NewBatch()
		restoreBB := restoreBatch.(*BinaryBatch)
		require.NoError(t, restoreBB.StoreBlockIndex(chainBlocks[i].Hash(), height))
		require.NoError(t, restoreBatch.Commit())
	}

	// Verify inconsistency: fork blocks have hash→height entries
	for _, fb := range forkBlocks {
		h, err := stor.GetBlockHeight(fb.Hash())
		require.NoError(t, err, "fork block hash→height should exist")
		assert.True(t, h > 100)
	}

	// Verify chain blocks still have correct height→hash
	for i, cb := range chainBlocks {
		hash, err := stor.GetBlockHashByHeight(uint32(101 + i))
		require.NoError(t, err)
		assert.Equal(t, cb.Hash(), hash, "height→hash should point to chain block")
	}

	// Run CleanOrphanedBlocks with maxValidHeight=100
	cleaned, err := stor.CleanOrphanedBlocks(100)
	require.NoError(t, err)
	assert.Equal(t, 4, cleaned) // 2 chain + 2 fork

	// Verify fork blocks' hash→height entries are gone
	for _, fb := range forkBlocks {
		_, err := stor.GetBlockHeight(fb.Hash())
		assert.Error(t, err, "fork block hash→height should be deleted")
	}

	// Verify chain blocks' hash→height entries are also gone
	for _, cb := range chainBlocks {
		_, err := stor.GetBlockHeight(cb.Hash())
		assert.Error(t, err, "chain block hash→height should be deleted")
	}

	// Verify fork blocks' data is gone
	for _, fb := range forkBlocks {
		_, err := stor.GetBlock(fb.Hash())
		assert.Error(t, err, "fork block data should be deleted")
	}

	// Verify good block at height 100 is untouched
	_, err = stor.GetBlock(goodBlock.Hash())
	require.NoError(t, err, "good block should still exist")
	h, err := stor.GetBlockHeight(goodBlock.Hash())
	require.NoError(t, err)
	assert.Equal(t, uint32(100), h)
}

// TestCleanOrphanedBlocksPreservesHeightToHash verifies that height→hash entries
// pointing to correct blocks are NOT deleted when cleaning orphans.
func TestCleanOrphanedBlocksPreservesHeightToHash(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "orphan-preserve-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	config := &storage.StorageConfig{
		Path:            tmpDir,
		CacheSize:       1 << 20,
		WriteBuffer:     2,
		MaxOpenFiles:    10,
		CompressionType: "snappy",
	}
	stor, err := NewBinaryStorage(config)
	require.NoError(t, err)
	defer stor.Close()

	// Store a block at height 100 (the rollback target block — should survive)
	targetBlock := createUniqueTestBlock(20)
	{
		batch := stor.NewBatch()
		bb := batch.(*BinaryBatch)
		require.NoError(t, bb.StoreBlockIndex(targetBlock.Hash(), 100))
		require.NoError(t, bb.StoreBlockWithHeight(targetBlock, 100))
		require.NoError(t, batch.Commit())
	}

	// Store a chain block at height 101
	chainBlock := createUniqueTestBlock(21)
	{
		batch := stor.NewBatch()
		bb := batch.(*BinaryBatch)
		require.NoError(t, bb.StoreBlockIndex(chainBlock.Hash(), 101))
		require.NoError(t, bb.StoreBlockWithHeight(chainBlock, 101))
		require.NoError(t, batch.Commit())
	}

	// Store an orphan fork block at height 101 (hash→height only, no block data, height→hash points to chainBlock)
	forkBlock := createUniqueTestBlock(30)
	{
		batch := stor.db.NewBatch()
		heightBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(heightBytes, 101)
		require.NoError(t, batch.Set(HashToHeightKey(forkBlock.Hash()), heightBytes, nil))
		require.NoError(t, batch.Commit(nil))
	}

	// Verify setup: height→hash(101) points to chainBlock, NOT forkBlock
	hash, err := stor.GetBlockHashByHeight(101)
	require.NoError(t, err)
	assert.Equal(t, chainBlock.Hash(), hash)

	// Run cleanup — maxValidHeight=100 means height 101 is above target
	cleaned, err := stor.CleanOrphanedBlocks(100)
	require.NoError(t, err)
	assert.Equal(t, 2, cleaned) // chainBlock + forkBlock hash→height entries

	// Fork block hash→height should be gone
	_, err = stor.GetBlockHeight(forkBlock.Hash())
	assert.Error(t, err)

	// Target block at height 100 should be fully intact
	_, err = stor.GetBlock(targetBlock.Hash())
	require.NoError(t, err)
	h, err := stor.GetBlockHeight(targetBlock.Hash())
	require.NoError(t, err)
	assert.Equal(t, uint32(100), h)
}

// TestCleanOrphanedBlocksNoOrphans verifies the no-op case
func TestCleanOrphanedBlocksNoOrphans(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "orphan-noop-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	config := &storage.StorageConfig{
		Path:            tmpDir,
		CacheSize:       1 << 20,
		WriteBuffer:     2,
		MaxOpenFiles:    10,
		CompressionType: "snappy",
	}
	stor, err := NewBinaryStorage(config)
	require.NoError(t, err)
	defer stor.Close()

	// Store a block at height 50
	block := createUniqueTestBlock(40)
	{
		batch := stor.NewBatch()
		bb := batch.(*BinaryBatch)
		require.NoError(t, bb.StoreBlockIndex(block.Hash(), 50))
		require.NoError(t, bb.StoreBlockWithHeight(block, 50))
		require.NoError(t, batch.Commit())
	}

	// Clean with maxValidHeight=100 — nothing should be cleaned
	cleaned, err := stor.CleanOrphanedBlocks(100)
	require.NoError(t, err)
	assert.Equal(t, 0, cleaned)

	// Block should still exist
	_, err = stor.GetBlock(block.Hash())
	require.NoError(t, err)
}

// createTestBlock creates a test block with some transactions
func createTestBlock() *types.Block {
	// Create coinbase transaction
	coinbaseTx := &types.Transaction{
		Version: 1,
		Inputs:  []*types.TxInput{},
		Outputs: []*types.TxOutput{
			{
				Value: 50 * 1e8, // 50 coins
				ScriptPubKey: []byte{
					0x76, 0xa9, 0x14, // OP_DUP OP_HASH160 PUSH(20)
					0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09,
					0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13,
					0x88, 0xac, // OP_EQUALVERIFY OP_CHECKSIG
				},
			},
		},
		LockTime: 0,
	}

	// Create regular transaction
	regularTx := &types.Transaction{
		Version: 1,
		Inputs: []*types.TxInput{
			{
				PreviousOutput: types.Outpoint{
					Hash:  types.Hash{0x01, 0x02, 0x03},
					Index: 0,
				},
				ScriptSig: []byte{0x48, 0x30}, // Dummy signature
				Sequence:  0xffffffff,
			},
		},
		Outputs: []*types.TxOutput{
			{
				Value: 25 * 1e8, // 25 coins
				ScriptPubKey: []byte{
					0xa9, 0x14, // OP_HASH160 PUSH(20)
					0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x29,
					0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f, 0x30, 0x31, 0x32, 0x33,
					0x87, // OP_EQUAL
				},
			},
		},
		LockTime: 0,
	}

	return &types.Block{
		Header: &types.BlockHeader{
			Version:       1,
			PrevBlockHash: types.Hash{0xff, 0xff},
			MerkleRoot:    types.Hash{0xaa, 0xbb},
			Timestamp:     1234567890,
			Bits:          0x1d00ffff,
			Nonce:         12345,
		},
		Transactions: []*types.Transaction{coinbaseTx, regularTx},
	}
}