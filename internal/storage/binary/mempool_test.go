package binary

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twins-dev/twins-core/internal/storage"
	"github.com/twins-dev/twins-core/pkg/types"
)

// TestMempoolTransactionStorage tests the separate mempool transaction storage
func TestMempoolTransactionStorage(t *testing.T) {
	// Create temporary database
	tmpDir, err := os.MkdirTemp("", "mempool-test-*")
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

	// Create a test transaction
	tx := &types.Transaction{
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
				Value: 100 * 1e8, // 100 coins
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

	txHash := tx.Hash()

	// Test 1: Store mempool transaction
	err = stor.StoreTransaction(tx)
	require.NoError(t, err, "Should store mempool transaction")

	// Test 2: Retrieve mempool transaction
	retrievedTx, err := stor.GetTransaction(txHash)
	require.NoError(t, err, "Should retrieve mempool transaction")
	assert.Equal(t, txHash, retrievedTx.Hash(), "Transaction hashes should match")
	assert.Equal(t, tx.Version, retrievedTx.Version, "Transaction version should match")
	assert.Len(t, retrievedTx.Inputs, 1, "Should have correct number of inputs")
	assert.Len(t, retrievedTx.Outputs, 1, "Should have correct number of outputs")

	// Test 3: Check transaction exists
	exists, err := stor.HasTransaction(txHash)
	require.NoError(t, err, "Should check if transaction exists")
	assert.True(t, exists, "Transaction should exist in mempool")

	// Test 4: Delete mempool transaction
	err = stor.DeleteMempoolTransaction(txHash)
	require.NoError(t, err, "Should delete mempool transaction")

	// Test 5: Verify transaction no longer exists
	_, err = stor.GetTransaction(txHash)
	assert.Error(t, err, "Should not find deleted transaction")

	exists, err = stor.HasTransaction(txHash)
	require.NoError(t, err, "Should check if transaction exists")
	assert.False(t, exists, "Transaction should not exist after deletion")

	// Test 6: Deleting non-existent transaction should not error
	err = stor.DeleteMempoolTransaction(txHash)
	assert.NoError(t, err, "Deleting non-existent mempool transaction should not error")
}

// TestMempoolAndBlockchainTransactionCoexistence tests that blockchain and mempool
// transactions are stored separately and don't interfere with each other
func TestMempoolAndBlockchainTransactionCoexistence(t *testing.T) {
	// Create temporary database
	tmpDir, err := os.MkdirTemp("", "coexist-test-*")
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

	// Create a test transaction
	tx := &types.Transaction{
		Version: 1,
		Inputs: []*types.TxInput{
			{
				PreviousOutput: types.Outpoint{
					Hash:  types.Hash{0x04, 0x05, 0x06},
					Index: 0,
				},
				ScriptSig: []byte{0x48, 0x31},
				Sequence:  0xffffffff,
			},
		},
		Outputs: []*types.TxOutput{
			{
				Value: 50 * 1e8, // 50 coins
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

	txHash := tx.Hash()

	// Step 1: Store transaction in mempool
	err = stor.StoreTransaction(tx)
	require.NoError(t, err, "Should store in mempool")

	// Step 2: Verify it's in mempool
	mempoolTx, err := stor.GetTransaction(txHash)
	require.NoError(t, err, "Should find in mempool")
	assert.Equal(t, txHash, mempoolTx.Hash())

	// Step 3: Simulate including transaction in a block
	// First create a block with the transaction
	block := &types.Block{
		Header: &types.BlockHeader{
			Version:       1,
			PrevBlockHash: types.Hash{0xff, 0xfe},
			MerkleRoot:    types.Hash{0xaa, 0xbb},
			Timestamp:     1234567890,
			Bits:          0x1d00ffff,
			Nonce:         54321,
		},
		Transactions: []*types.Transaction{tx},
	}

	// Store block with transaction
	batch := stor.NewBatch()
	binaryBatch := batch.(*BinaryBatch)
	err = binaryBatch.StoreBlockWithHeight(block, 100)
	require.NoError(t, err)
	err = binaryBatch.StoreBlockIndex(block.Hash(), 100)
	require.NoError(t, err)
	err = batch.Commit()
	require.NoError(t, err)

	// Step 4: Delete from mempool (simulating confirmation)
	err = stor.DeleteMempoolTransaction(txHash)
	require.NoError(t, err, "Should delete from mempool")

	// Step 5: Verify transaction is still accessible (now from blockchain)
	blockchainTx, err := stor.GetTransaction(txHash)
	require.NoError(t, err, "Should find in blockchain")
	assert.Equal(t, txHash, blockchainTx.Hash())

	// Step 6: Verify it exists
	exists, err := stor.HasTransaction(txHash)
	require.NoError(t, err)
	assert.True(t, exists, "Transaction should exist in blockchain")
}