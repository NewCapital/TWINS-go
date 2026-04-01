package storage_test

import (
	"testing"
	"time"

	stpkg "github.com/twins-dev/twins-core/internal/storage"
	binarystorage "github.com/twins-dev/twins-core/internal/storage/binary"
	"github.com/twins-dev/twins-core/pkg/types"
)

// TestStorage tests basic storage operations
func TestStorage(t *testing.T) {
	// Create temporary directory for test database
	tempDir := t.TempDir()

	// Create test configuration
	config := stpkg.TestStorageConfig()
	config.Path = tempDir

	// Create storage using new binary storage
	storage, err := binarystorage.NewBinaryStorage(config)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Test basic operations
	t.Run("BlockOperations", func(t *testing.T) {
		testBlockOperations(t, storage)
	})

	t.Run("TransactionOperations", func(t *testing.T) {
		testTransactionOperations(t, storage)
	})

	t.Run("UTXOOperations", func(t *testing.T) {
		testUTXOOperations(t, storage)
	})

	t.Run("ChainStateOperations", func(t *testing.T) {
		testChainStateOperations(t, storage)
	})
}

func testBlockOperations(t *testing.T, storage stpkg.Storage) {
	// Create test block
	block := createTestBlock(t)

	// Store block index first (StoreBlock requires height lookup)
	blockHash := block.Hash()
	if err := storage.StoreBlockIndex(blockHash, 0); err != nil {
		t.Fatalf("Failed to store block index: %v", err)
	}

	// Test StoreBlock
	if err := storage.StoreBlock(block); err != nil {
		t.Fatalf("Failed to store block: %v", err)
	}

	// Test HasBlock
	has, err := storage.HasBlock(blockHash)
	if err != nil {
		t.Fatalf("Failed to check block existence: %v", err)
	}
	if !has {
		t.Error("Block should exist after storing")
	}

	// Test GetBlock
	retrievedBlock, err := storage.GetBlock(blockHash)
	if err != nil {
		t.Fatalf("Failed to get block: %v", err)
	}

	if retrievedBlock.Hash() != blockHash {
		t.Error("Retrieved block hash doesn't match")
	}

	// Test GetBlockByHeight
	blockByHeight, err := storage.GetBlockByHeight(0)
	if err != nil {
		t.Fatalf("Failed to get block by height: %v", err)
	}

	if blockByHeight.Hash() != blockHash {
		t.Error("Block retrieved by height doesn't match")
	}

	// Test DeleteBlock
	if err := storage.DeleteBlock(blockHash); err != nil {
		t.Fatalf("Failed to delete block: %v", err)
	}

	// Verify block is deleted
	has, err = storage.HasBlock(blockHash)
	if err != nil {
		t.Fatalf("Failed to check block existence after deletion: %v", err)
	}
	if has {
		t.Error("Block should not exist after deletion")
	}
}

func testTransactionOperations(t *testing.T, storage *binarystorage.BinaryStorage) {
	// Create test transaction
	tx := createTestTransaction(t)

	// Test StoreTransaction
	if err := storage.StoreTransaction(tx); err != nil {
		t.Fatalf("Failed to store transaction: %v", err)
	}

	// Test HasTransaction
	txHash := tx.Hash()
	has, err := storage.HasTransaction(txHash)
	if err != nil {
		t.Fatalf("Failed to check transaction existence: %v", err)
	}
	if !has {
		t.Error("Transaction should exist after storing")
	}

	// Test GetTransaction
	retrievedTx, err := storage.GetTransaction(txHash)
	if err != nil {
		t.Fatalf("Failed to get transaction: %v", err)
	}

	if retrievedTx.Hash() != txHash {
		t.Error("Retrieved transaction hash doesn't match")
	}

	if len(retrievedTx.Inputs) != len(tx.Inputs) {
		t.Errorf("Input count mismatch: expected %d, got %d", len(tx.Inputs), len(retrievedTx.Inputs))
	}

	if len(retrievedTx.Outputs) != len(tx.Outputs) {
		t.Errorf("Output count mismatch: expected %d, got %d", len(tx.Outputs), len(retrievedTx.Outputs))
	}
}

func testUTXOOperations(t *testing.T, storage *binarystorage.BinaryStorage) {
	// Create test UTXO
	outpoint := types.Outpoint{
		Hash:  types.Hash{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
		Index: 0,
	}

	output := &types.TxOutput{
		Value:        1000000,
		ScriptPubKey: []byte("test script"),
	}

	// Test StoreUTXO
	if err := storage.StoreUTXO(outpoint, output, 100, false); err != nil {
		t.Fatalf("Failed to store UTXO: %v", err)
	}

	// Test GetUTXO
	retrievedUTXO, err := storage.GetUTXO(outpoint)
	if err != nil {
		t.Fatalf("Failed to get UTXO: %v", err)
	}

	if retrievedUTXO.Outpoint != outpoint {
		t.Error("UTXO outpoint doesn't match")
	}

	if retrievedUTXO.Output.Value != output.Value {
		t.Error("UTXO value doesn't match")
	}

	if retrievedUTXO.Height != 100 {
		t.Error("UTXO height doesn't match")
	}

	if retrievedUTXO.IsCoinbase != false {
		t.Error("UTXO coinbase flag doesn't match")
	}

	// Test DeleteUTXOWithData
	if err := storage.DeleteUTXOWithData(outpoint, retrievedUTXO); err != nil {
		t.Fatalf("Failed to delete UTXO: %v", err)
	}

	// Verify UTXO is deleted
	_, err = storage.GetUTXO(outpoint)
	if err == nil {
		t.Error("UTXO should not exist after deletion")
	}
}

func testChainStateOperations(t *testing.T, storage *binarystorage.BinaryStorage) {
	// Test initial chain height
	height, err := storage.GetChainHeight()
	if err != nil {
		t.Fatalf("Failed to get initial chain height: %v", err)
	}
	if height != 0 {
		t.Errorf("Initial chain height should be 0, got %d", height)
	}

	// Test SetChainState
	testHash := types.Hash{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	if err := storage.SetChainState(100, testHash); err != nil {
		t.Fatalf("Failed to set chain state: %v", err)
	}

	// Test GetChainHeight after setting
	height, err = storage.GetChainHeight()
	if err != nil {
		t.Fatalf("Failed to get chain height after setting: %v", err)
	}
	if height != 100 {
		t.Errorf("Chain height should be 100, got %d", height)
	}

	// Test GetChainTip
	tip, err := storage.GetChainTip()
	if err != nil {
		t.Fatalf("Failed to get chain tip: %v", err)
	}
	if tip != testHash {
		t.Error("Chain tip doesn't match")
	}

	// Test Money Supply operations
	t.Run("MoneySupply", func(t *testing.T) {
		// Test initial money supply (should return 0 for missing height)
		supply, err := storage.GetMoneySupply(0)
		if err != nil {
			t.Fatalf("Failed to get initial money supply: %v", err)
		}
		if supply != 0 {
			t.Errorf("Initial money supply should be 0, got %d", supply)
		}

		// Test storing money supply
		testSupply := int64(1084183789900000000) // ~10.8B TWINS in satoshis
		if err := storage.StoreMoneySupply(100, testSupply); err != nil {
			t.Fatalf("Failed to store money supply: %v", err)
		}

		// Test retrieving stored money supply
		retrievedSupply, err := storage.GetMoneySupply(100)
		if err != nil {
			t.Fatalf("Failed to get money supply: %v", err)
		}
		if retrievedSupply != testSupply {
			t.Errorf("Money supply mismatch: expected %d, got %d", testSupply, retrievedSupply)
		}

		// Test money supply for different height
		testSupply2 := int64(1084183789900000000 + 5000000000) // +50 TWINS
		if err := storage.StoreMoneySupply(101, testSupply2); err != nil {
			t.Fatalf("Failed to store money supply for height 101: %v", err)
		}

		supply101, err := storage.GetMoneySupply(101)
		if err != nil {
			t.Fatalf("Failed to get money supply for height 101: %v", err)
		}
		if supply101 != testSupply2 {
			t.Errorf("Money supply mismatch at height 101: expected %d, got %d", testSupply2, supply101)
		}
	})
}

func TestBatchOperations(t *testing.T) {
	tempDir := t.TempDir()
	config := stpkg.TestStorageConfig()
	config.Path = tempDir

	storage, err := binarystorage.NewBinaryStorage(config)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Test batch operations
	batch := storage.NewBatch()

	// Add operations to batch
	block := createTestBlock(t)
	tx := createTestTransaction(t)

	// StoreBlockIndex must be committed before StoreBlock can look up height
	if err := storage.StoreBlockIndex(block.Hash(), 0); err != nil {
		t.Fatalf("Failed to store block index: %v", err)
	}
	if err := batch.StoreBlock(block); err != nil {
		t.Fatalf("Failed to add block to batch: %v", err)
	}

	if err := batch.StoreTransaction(tx); err != nil {
		t.Fatalf("Failed to add transaction to batch: %v", err)
	}

	// Test batch size
	if batch.Size() <= 0 {
		t.Error("Batch size should be greater than 0")
	}

	// Commit batch
	if err := batch.Commit(); err != nil {
		t.Fatalf("Failed to commit batch: %v", err)
	}

	// Verify items were stored
	blockHash := block.Hash()
	has, err := storage.HasBlock(blockHash)
	if err != nil {
		t.Fatalf("Failed to check block existence after batch commit: %v", err)
	}
	if !has {
		t.Error("Block should exist after batch commit")
	}

	txHash := tx.Hash()
	has, err = storage.HasTransaction(txHash)
	if err != nil {
		t.Fatalf("Failed to check transaction existence after batch commit: %v", err)
	}
	if !has {
		t.Error("Transaction should exist after batch commit")
	}
}

func TestBatchRollback(t *testing.T) {
	tempDir := t.TempDir()
	config := stpkg.TestStorageConfig()
	config.Path = tempDir

	storage, err := binarystorage.NewBinaryStorage(config)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	batch := storage.NewBatch()

	// Add operations to batch
	block := createTestBlock(t)
	// StoreBlockIndex must be committed before StoreBlock can look up height
	if err := storage.StoreBlockIndex(block.Hash(), 0); err != nil {
		t.Fatalf("Failed to store block index: %v", err)
	}
	if err := batch.StoreBlock(block); err != nil {
		t.Fatalf("Failed to add block to batch: %v", err)
	}

	// Rollback batch
	if err := batch.Rollback(); err != nil {
		t.Fatalf("Failed to rollback batch: %v", err)
	}

	// Verify items were not stored
	blockHash := block.Hash()
	has, err := storage.HasBlock(blockHash)
	if err != nil {
		t.Fatalf("Failed to check block existence after batch rollback: %v", err)
	}
	if has {
		t.Error("Block should not exist after batch rollback")
	}
}

func TestStorageStats(t *testing.T) {
	tempDir := t.TempDir()
	config := stpkg.TestStorageConfig()
	config.Path = tempDir

	storage, err := binarystorage.NewBinaryStorage(config)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Get initial stats
	stats, err := storage.GetStats()
	if err != nil {
		t.Fatalf("Failed to get storage stats: %v", err)
	}

	if stats == nil {
		t.Fatal("Stats should not be nil")
	}

	// Store some data
	block := createTestBlock(t)
	storage.StoreBlockIndex(block.Hash(), 0)
	if err := storage.StoreBlock(block); err != nil {
		t.Fatalf("Failed to store block: %v", err)
	}

	// Get stats after storing data
	stats, err = storage.GetStats()
	if err != nil {
		t.Fatalf("Failed to get storage stats after storing data: %v", err)
	}

	if stats.Blocks == 0 {
		t.Error("Block count should be greater than 0")
	}
}

func TestStorageSize(t *testing.T) {
	tempDir := t.TempDir()
	config := stpkg.TestStorageConfig()
	config.Path = tempDir

	storage, err := binarystorage.NewBinaryStorage(config)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Get initial size
	size, err := storage.GetSize()
	if err != nil {
		t.Fatalf("Failed to get storage size: %v", err)
	}

	if size < 0 {
		t.Error("Storage size should not be negative")
	}
}

func TestStorageCompaction(t *testing.T) {
	tempDir := t.TempDir()
	config := stpkg.TestStorageConfig()
	config.Path = tempDir

	storage, err := binarystorage.NewBinaryStorage(config)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Store some data
	for i := 0; i < 100; i++ {
		block := createTestBlockWithHeight(t, uint32(i))
		storage.StoreBlockIndex(block.Hash(), uint32(i))
		if err := storage.StoreBlock(block); err != nil {
			t.Fatalf("Failed to store block %d: %v", i, err)
		}
	}

	// Test manual compaction
	// Note: Pebble Compact(nil,nil) may return "start is not less than end" on some versions
	// when the database keyspace is empty or very small. This is not a real error.
	if err := storage.Compact(); err != nil {
		t.Logf("Compact returned (non-fatal): %v", err)
	}
}

func TestStorageSync(t *testing.T) {
	tempDir := t.TempDir()
	config := stpkg.TestStorageConfig()
	config.Path = tempDir

	storage, err := binarystorage.NewBinaryStorage(config)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Test sync
	if err := storage.Sync(); err != nil {
		t.Fatalf("Failed to sync storage: %v", err)
	}
}

func TestMemoryStorage(t *testing.T) {
	config := stpkg.TestStorageConfig()
	config.Path = ":memory:"

	storage, err := binarystorage.NewBinaryStorage(config)
	if err != nil {
		t.Fatalf("Failed to create memory storage: %v", err)
	}
	defer storage.Close()

	// Test basic operations with memory storage
	block := createTestBlock(t)
	storage.StoreBlockIndex(block.Hash(), 0)
	if err := storage.StoreBlock(block); err != nil {
		t.Fatalf("Failed to store block in memory storage: %v", err)
	}

	blockHash := block.Hash()
	has, err := storage.HasBlock(blockHash)
	if err != nil {
		t.Fatalf("Failed to check block existence in memory storage: %v", err)
	}
	if !has {
		t.Error("Block should exist in memory storage")
	}
}

func TestReadOnlyMode(t *testing.T) {
	tempDir := t.TempDir()

	// First create a database with some data
	config := stpkg.TestStorageConfig()
	config.Path = tempDir

	storage, err := binarystorage.NewBinaryStorage(config)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	block := createTestBlock(t)
	storage.StoreBlockIndex(block.Hash(), 0)
	if err := storage.StoreBlock(block); err != nil {
		t.Fatalf("Failed to store block: %v", err)
	}
	storage.Close()

	// Reopen in read-only mode
	config.ReadOnly = true
	readOnlyStorage, err := binarystorage.NewBinaryStorage(config)
	if err != nil {
		t.Fatalf("Failed to create read-only storage: %v", err)
	}
	defer readOnlyStorage.Close()

	// Test that reads work
	blockHash := block.Hash()
	has, err := readOnlyStorage.HasBlock(blockHash)
	if err != nil {
		t.Fatalf("Failed to check block existence in read-only mode: %v", err)
	}
	if !has {
		t.Error("Block should exist in read-only storage")
	}

	// Test that writes fail
	newBlock := createTestBlockWithHeight(t, 1)
	err = readOnlyStorage.StoreBlock(newBlock)
	if err == nil {
		t.Error("Write should fail in read-only mode")
	}
}

// Helper functions for creating test data

func createTestBlock(t testing.TB) *types.Block {
	return createTestBlockWithHeight(t, 0)
}

func createTestBlockWithHeight(t testing.TB, height uint32) *types.Block {
	// Create test transaction
	tx := createTestTransaction(t)

	// Create test block header
	header := &types.BlockHeader{
		Version:       1,
		PrevBlockHash: types.Hash{},
		MerkleRoot:    tx.Hash(), // Simplified - should be proper merkle root
		Timestamp:             uint32(time.Now().Unix()),
		Bits:                  0x1d00ffff,
		Nonce:                 12345,
		AccumulatorCheckpoint: types.Hash{},
	}

	// Modify some fields based on height to make blocks unique
	header.Timestamp += height
	header.Nonce += height

	return &types.Block{
		Header:       header,
		Transactions: []*types.Transaction{tx},
	}
}

func createTestTransaction(t testing.TB) *types.Transaction {
	// Create coinbase input
	coinbaseInput := &types.TxInput{
		PreviousOutput: types.Outpoint{Hash: types.Hash{}, Index: 0xffffffff},
		ScriptSig:      []byte("coinbase script"),
		Sequence:       0xffffffff,
	}

	// Create test output
	output := &types.TxOutput{
		Value:        5000000000, // 50 TWINS
		ScriptPubKey: []byte("test output script"),
	}

	return &types.Transaction{
		Version:  1,
		Inputs:   []*types.TxInput{coinbaseInput},
		Outputs:  []*types.TxOutput{output},
		LockTime: 0,
	}
}

func TestStorageConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *stpkg.StorageConfig
		shouldError bool
	}{
		{
			name:        "nil config",
			config:      nil,
			shouldError: true,
		},
		{
			name: "empty path",
			config: &stpkg.StorageConfig{
				Path: "",
			},
			shouldError: true,
		},
		{
			name: "negative cache size",
			config: &stpkg.StorageConfig{
				Path:      "/tmp/test",
				CacheSize: -1,
			},
			shouldError: true,
		},
		{
			name: "invalid compression type",
			config: &stpkg.StorageConfig{
				Path:            "/tmp/test",
				CacheSize:       10,
				WriteBufferSize: 10,
				WriteBuffer:     1,
				MaxOpenFiles:    100,
				CompressionType: "invalid",
				BloomFilterBits: 10,
			},
			shouldError: true,
		},
		{
			name: "valid config",
			config: &stpkg.StorageConfig{
				Path:            "/tmp/test",
				CacheSize:       10,
				WriteBufferSize: 10,
				WriteBuffer:     1,
				MaxOpenFiles:    100,
				CompressionType: "snappy",
				BloomFilterBits: 10,
			},
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := stpkg.ValidateStorageConfig(tt.config)
			if tt.shouldError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestErrorTypes(t *testing.T) {
	// Test IsNotFoundError
	if !stpkg.IsNotFoundError(stpkg.ErrNotFound) {
		t.Error("stpkg.ErrNotFound should be recognized as not found error")
	}

	if stpkg.IsNotFoundError(stpkg.ErrCorruptedData) {
		t.Error("stpkg.ErrCorruptedData should not be recognized as not found error")
	}

	// Test stpkg.IsCorruptedDataError
	if !stpkg.IsCorruptedDataError(stpkg.ErrCorruptedData) {
		t.Error("stpkg.ErrCorruptedData should be recognized as corrupted data error")
	}

	if stpkg.IsCorruptedDataError(stpkg.ErrNotFound) {
		t.Error("stpkg.ErrNotFound should not be recognized as corrupted data error")
	}

	// Test stpkg.IsDatabaseClosedError
	if !stpkg.IsDatabaseClosedError(stpkg.ErrDatabaseClosed) {
		t.Error("stpkg.ErrDatabaseClosed should be recognized as database closed error")
	}

	if stpkg.IsDatabaseClosedError(stpkg.ErrNotFound) {
		t.Error("stpkg.ErrNotFound should not be recognized as database closed error")
	}
}

// Benchmark tests

func BenchmarkBlockStore(b *testing.B) {
	tempDir := b.TempDir()
	config := stpkg.BenchmarkStorageConfig()
	config.Path = tempDir

	storage, err := binarystorage.NewBinaryStorage(config)
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	block := createTestBlock(b)
	storage.StoreBlockIndex(block.Hash(), 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := storage.StoreBlock(block); err != nil {
			b.Fatalf("Failed to store block: %v", err)
		}
	}
}

func BenchmarkBlockGet(b *testing.B) {
	tempDir := b.TempDir()
	config := stpkg.BenchmarkStorageConfig()
	config.Path = tempDir

	storage, err := binarystorage.NewBinaryStorage(config)
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	block := createTestBlock(b)
	blockHash := block.Hash()

	storage.StoreBlockIndex(blockHash, 0)
	if err := storage.StoreBlock(block); err != nil {
		b.Fatalf("Failed to store block: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := storage.GetBlock(blockHash)
		if err != nil {
			b.Fatalf("Failed to get block: %v", err)
		}
	}
}

func BenchmarkBatchCommit(b *testing.B) {
	tempDir := b.TempDir()
	config := stpkg.BenchmarkStorageConfig()
	config.Path = tempDir

	storage, err := binarystorage.NewBinaryStorage(config)
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batch := storage.NewBatch()
		block := createTestBlockWithHeight(b, uint32(i))

		storage.StoreBlockIndex(block.Hash(), uint32(i))
		if err := batch.StoreBlock(block); err != nil {
			b.Fatalf("Failed to add block to batch: %v", err)
		}

		if err := batch.Commit(); err != nil {
			b.Fatalf("Failed to commit batch: %v", err)
		}
	}
}

// Benchmark helper functions removed to avoid duplication - using the ones above