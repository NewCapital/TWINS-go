package blockchain

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twins-dev/twins-core/internal/consensus"
	"github.com/twins-dev/twins-core/internal/storage"
	"github.com/twins-dev/twins-core/internal/storage/binary"
	"github.com/twins-dev/twins-core/pkg/types"
)

func createTestBlockchain(t *testing.T) *BlockChain {
	// Create test storage
	storageConfig := storage.TestStorageConfig()
	store, err := binary.NewBinaryStorage(storageConfig)
	require.NoError(t, err)

	// Create test consensus
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel) // Reduce noise in tests
	pos := consensus.NewProofOfStake(store, types.DefaultChainParams(), logger)

	// Create blockchain
	config := DefaultConfig()
	config.Storage = store
	config.Consensus = pos
	config.ChainParams = types.DefaultChainParams()

	bc, err := New(config)
	require.NoError(t, err)

	return bc
}

func createTestBlock(height uint32, prevHash types.Hash) *types.Block {
	block := &types.Block{
		Header: &types.BlockHeader{
			Version:       1,
			PrevBlockHash: prevHash,
			Timestamp:     1640995200 + height*120,
			Bits:          0x1d00ffff,
			Nonce:         height,
		},
		Transactions: []*types.Transaction{
			createCoinbaseTx(height),
		},
	}
	// Compute valid merkle root from transactions
	block.Header.MerkleRoot = block.CalculateMerkleRoot()
	return block
}

func createCoinbaseTx(height uint32) *types.Transaction {
	return &types.Transaction{
		Version: 1,
		Inputs: []*types.TxInput{
			{
				PreviousOutput: types.Outpoint{
					Hash:  types.ZeroHash,
					Index: 0xffffffff,
				},
				ScriptSig: []byte{byte(height)},
				Sequence:  0xffffffff,
			},
		},
		Outputs: []*types.TxOutput{
			{
				Value:        5000000000, // 50 TWINS
				ScriptPubKey: []byte("coinbase output"),
			},
		},
		LockTime: 0,
	}
}

func TestNew(t *testing.T) {
	bc := createTestBlockchain(t)
	assert.NotNil(t, bc)
	assert.NotNil(t, bc.storage)
	assert.NotNil(t, bc.consensus)
	assert.NotNil(t, bc.logger)
}

func TestNewWithNilConfig(t *testing.T) {
	_, err := New(nil)
	assert.Error(t, err)
}

func TestNewWithoutStorage(t *testing.T) {
	config := DefaultConfig()
	config.Consensus = &struct{}{}

	_, err := New(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "storage")
}

func TestNewWithoutConsensus(t *testing.T) {
	storageConfig := storage.TestStorageConfig()
	store, err := binary.NewBinaryStorage(storageConfig)
	require.NoError(t, err)

	config := DefaultConfig()
	config.Storage = store

	_, err = New(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "consensus")
}

func TestStartStop(t *testing.T) {
	bc := createTestBlockchain(t)

	err := bc.Start()
	assert.NoError(t, err)

	err = bc.Stop()
	assert.NoError(t, err)
}

func TestGetBestBlock_Empty(t *testing.T) {
	bc := createTestBlockchain(t)

	_, err := bc.GetBestBlock()
	assert.Error(t, err)
}

func TestGetBestHeight(t *testing.T) {
	bc := createTestBlockchain(t)

	height, err := bc.GetBestHeight()
	assert.NoError(t, err)
	assert.Equal(t, uint32(0), height)
}

func TestGetBestBlockHash(t *testing.T) {
	bc := createTestBlockchain(t)

	hash, err := bc.GetBestBlockHash()
	assert.NoError(t, err)
	assert.Equal(t, types.ZeroHash, hash)
}

func TestProcessBlock_NilBlock(t *testing.T) {
	bc := createTestBlockchain(t)

	err := bc.ProcessBlock(nil)
	assert.Error(t, err)
}

func TestConnectBlock(t *testing.T) {
	t.Skip("ConnectBlock delegates to processBatchUnified which requires valid PoW/PoS — needs integration test with real block mining")
}

func TestAddOrphan(t *testing.T) {
	bc := createTestBlockchain(t)

	block := createTestBlock(5, types.NewHash([]byte("unknown")))

	err := bc.addOrphan(block)
	assert.NoError(t, err)

	orphans := bc.GetOrphanBlocks()
	assert.Len(t, orphans, 1)
	assert.Equal(t, block.Hash(), orphans[0].Hash())
}

func TestAddOrphan_MaxLimit(t *testing.T) {
	bc := createTestBlockchain(t)
	bc.config.MaxOrphans = 2

	// Add three orphans
	for i := uint32(0); i < 3; i++ {
		block := createTestBlock(i, types.NewHash([]byte{byte(i)}))
		err := bc.addOrphan(block)
		assert.NoError(t, err)
	}

	// Should only have MaxOrphans
	orphans := bc.GetOrphanBlocks()
	assert.LessOrEqual(t, len(orphans), bc.config.MaxOrphans)
}

func TestGetChainTips_Empty(t *testing.T) {
	bc := createTestBlockchain(t)

	tips, err := bc.GetChainTips()
	assert.NoError(t, err)
	assert.NotNil(t, tips)
}

func TestGetStats(t *testing.T) {
	bc := createTestBlockchain(t)

	stats := bc.GetStats()
	assert.NotNil(t, stats)
	assert.Equal(t, uint32(0), stats.Height)
}

func TestUpdateStats(t *testing.T) {
	bc := createTestBlockchain(t)

	block := createTestBlock(1, types.ZeroHash)

	bc.mu.Lock()
	bc.bestHeight.Store(1)
	bc.bestHash = block.Hash()
	bc.mu.Unlock()

	bc.updateStats(block)

	stats := bc.GetStats()
	assert.Equal(t, uint32(1), stats.Height)
	assert.Equal(t, block.Hash(), stats.BestBlockHash)
	assert.GreaterOrEqual(t, stats.Transactions, uint64(1))
}

func TestHasBlock(t *testing.T) {
	bc := createTestBlockchain(t)

	block := createTestBlock(1, types.ZeroHash)

	// Should not have block initially
	has, err := bc.HasBlock(block.Hash())
	assert.NoError(t, err)
	assert.False(t, has)

	// Store index first (StoreBlock requires height lookup)
	err = bc.storage.StoreBlockIndex(block.Hash(), 1)
	require.NoError(t, err)
	err = bc.storage.StoreBlock(block)
	require.NoError(t, err)

	// Should have block now
	has, err = bc.HasBlock(block.Hash())
	assert.NoError(t, err)
	assert.True(t, has)
}

func TestGetBlock(t *testing.T) {
	bc := createTestBlockchain(t)

	block := createTestBlock(1, types.ZeroHash)

	// Store index first (StoreBlock requires height lookup)
	err := bc.storage.StoreBlockIndex(block.Hash(), 1)
	require.NoError(t, err)
	err = bc.storage.StoreBlock(block)
	require.NoError(t, err)

	// Retrieve block
	retrieved, err := bc.GetBlock(block.Hash())
	assert.NoError(t, err)
	assert.Equal(t, block.Hash(), retrieved.Hash())
}

func TestGetBlockByHeight(t *testing.T) {
	bc := createTestBlockchain(t)

	// GetBlockByHeight with no blocks should return error
	_, err := bc.GetBlockByHeight(1)
	assert.Error(t, err)
}

func TestIsOnMainChain(t *testing.T) {
	bc := createTestBlockchain(t)

	block := createTestBlock(1, types.ZeroHash)

	// Initially should not be on main chain
	isMain, err := bc.IsOnMainChain(block.Hash())
	assert.NoError(t, err)
	assert.False(t, isMain)

	// Update index to mark as connected
	bc.updateBlockIndex(block, 1, BlockStatusConnected)

	// Now should be on main chain
	isMain, err = bc.IsOnMainChain(block.Hash())
	assert.NoError(t, err)
	assert.True(t, isMain)
}

func TestGetChainWork(t *testing.T) {
	bc := createTestBlockchain(t)

	work, err := bc.GetChainWork()
	assert.NoError(t, err)
	assert.NotNil(t, work)
	assert.Equal(t, int64(0), work.Int64())
}

func TestUpdateBlockIndex(t *testing.T) {
	bc := createTestBlockchain(t)

	block := createTestBlock(1, types.ZeroHash)

	bc.updateBlockIndex(block, 1, BlockStatusValid)

	bc.indexMu.RLock()
	node, exists := bc.blockIndex[block.Hash()]
	bc.indexMu.RUnlock()

	assert.True(t, exists)
	assert.Equal(t, block.Hash(), node.Hash)
	assert.Equal(t, uint32(1), node.Height)
	assert.Equal(t, BlockStatusValid, node.Status)
}

func TestBlockStatus_String(t *testing.T) {
	assert.Equal(t, "active", ChainTipActive.String())
	assert.Equal(t, "orphan", ChainTipOrphan.String())
	assert.Equal(t, "valid-headers", ChainTipValidHeaders.String())
	assert.Equal(t, "valid-fork", ChainTipValidFork.String())
	assert.Equal(t, "invalid", ChainTipInvalid.String())
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	assert.NotNil(t, config)
	assert.Equal(t, 100, config.MaxOrphans)
	assert.Equal(t, uint32(100000), config.MaxReorgDepth)
	assert.True(t, config.EnableAddressIndex)
}

// Benchmark tests
func BenchmarkGetBestHeight(b *testing.B) {
	bc := createTestBlockchain(&testing.T{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = bc.GetBestHeight()
	}
}

func BenchmarkGetBestBlockHash(b *testing.B) {
	bc := createTestBlockchain(&testing.T{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = bc.GetBestBlockHash()
	}
}

func BenchmarkUpdateBlockIndex(b *testing.B) {
	bc := createTestBlockchain(&testing.T{})
	block := createTestBlock(1, types.ZeroHash)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bc.updateBlockIndex(block, 1, BlockStatusValid)
	}
}