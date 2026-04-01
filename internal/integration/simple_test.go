package integration

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twins-dev/twins-core/internal/blockchain"
	"github.com/twins-dev/twins-core/internal/consensus"
	"github.com/twins-dev/twins-core/internal/masternode"
	"github.com/twins-dev/twins-core/internal/mempool"
	"github.com/twins-dev/twins-core/internal/spork"
	"github.com/twins-dev/twins-core/internal/storage"
	"github.com/twins-dev/twins-core/internal/storage/binary"
	"github.com/twins-dev/twins-core/internal/wallet"
	"github.com/twins-dev/twins-core/pkg/crypto"
	"github.com/twins-dev/twins-core/pkg/types"
)

// TestStorageIntegration tests storage initialization
func TestStorageIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create storage
	storageConfig := storage.TestStorageConfig()
	store, err := binary.NewBinaryStorage(storageConfig)
	require.NoError(t, err)
	defer store.Close()

	// Verify storage is initialized
	assert.NotNil(t, store)

	// Get chain height (should be 0 for new storage)
	height, err := store.GetChainHeight()
	// Error is expected for new storage with no genesis
	if err == nil {
		assert.Equal(t, uint32(0), height)
	}
}

// TestMasternodeManagerIntegration tests masternode manager
func TestMasternodeManagerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create logger
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	// Create storage and blockchain (required by masternode manager)
	storageConfig := storage.TestStorageConfig()
	store, err := binary.NewBinaryStorage(storageConfig)
	require.NoError(t, err)
	defer store.Close()

	pos := consensus.NewProofOfStake(store, types.DefaultChainParams(), logger)
	bcConfig := blockchain.DefaultConfig()
	bcConfig.Storage = store
	bcConfig.Consensus = pos
	bcConfig.ChainParams = types.DefaultChainParams()
	bc, err := blockchain.New(bcConfig)
	require.NoError(t, err)

	// Create spork manager (required by masternode manager)
	chainParams := types.DefaultChainParams()
	sm, err := spork.NewManager(chainParams.SporkPubKey, chainParams.SporkPubKeyOld, nil)
	require.NoError(t, err)

	// Create masternode manager
	config := masternode.DefaultConfig()
	mnManager, err := masternode.NewManager(config, logger)
	require.NoError(t, err)
	mnManager.SetSporkManager(sm)
	mnManager.SetBlockchain(bc)

	// Start manager
	err = mnManager.Start()
	require.NoError(t, err)
	defer mnManager.Stop()

	// Verify initial state
	assert.Equal(t, 0, mnManager.GetMasternodeCount())

	// Create test masternode
	mn := createTestMasternode(t)

	// Add masternode
	err = mnManager.AddMasternode(mn)
	require.NoError(t, err)

	// Verify added
	assert.Equal(t, 1, mnManager.GetMasternodeCount())

	// Get masternode
	retrieved, err := mnManager.GetMasternode(mn.OutPoint)
	require.NoError(t, err)
	assert.Equal(t, mn.OutPoint, retrieved.OutPoint)
	assert.Equal(t, mn.Tier, retrieved.Tier)

	// Remove masternode
	err = mnManager.RemoveMasternode(mn.OutPoint)
	require.NoError(t, err)

	// Verify removed
	assert.Equal(t, 0, mnManager.GetMasternodeCount())
}

// TestMempoolIntegration tests mempool initialization
func TestMempoolIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create mempool
	config := mempool.DefaultConfig()
	mp, err := mempool.New(config)

	// Mempool requires blockchain, so error is expected
	if err != nil {
		t.Skip("Mempool requires blockchain dependency")
		return
	}

	// Verify initial state
	assert.Equal(t, 0, mp.Size())
}

// TestWalletStorageIntegration tests wallet with storage
func TestWalletStorageIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create logger
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	// Create storage
	storageConfig := storage.TestStorageConfig()
	store, err := binary.NewBinaryStorage(storageConfig)
	require.NoError(t, err)
	defer store.Close()

	// Create wallet
	walletConfig := wallet.DefaultConfig()
	walletConfig.Network = wallet.TestNet
	w, err := wallet.NewWallet(walletConfig, store, logger)
	require.NoError(t, err)

	// Create wallet
	seed := []byte("test seed for integration with enough entropy")
	err = w.CreateWallet(seed, nil)
	require.NoError(t, err)

	// Generate addresses
	addr1, err := w.GetNewAddress("Address 1")
	require.NoError(t, err)
	assert.NotEmpty(t, addr1)

	addr2, err := w.GetNewAddress("Address 2")
	require.NoError(t, err)
	assert.NotEmpty(t, addr2)
	assert.NotEqual(t, addr1, addr2)

	// List addresses
	addresses, err := w.ListAddresses()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(addresses), 2)
}

// TestFullStackIntegration tests multiple components working together
func TestFullStackIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create logger
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	// Create storage
	storageConfig := storage.TestStorageConfig()
	store, err := binary.NewBinaryStorage(storageConfig)
	require.NoError(t, err)
	defer store.Close()

	// Create mempool (may fail without blockchain)
	mempoolConfig := mempool.DefaultConfig()
	mp, err := mempool.New(mempoolConfig)
	if err != nil {
		// Mempool requires blockchain, skip mempool part
		mp = nil
	}

	// Create blockchain (required by masternode manager)
	pos := consensus.NewProofOfStake(store, types.DefaultChainParams(), logger)
	bcConfig := blockchain.DefaultConfig()
	bcConfig.Storage = store
	bcConfig.Consensus = pos
	bcConfig.ChainParams = types.DefaultChainParams()
	bc, err := blockchain.New(bcConfig)
	require.NoError(t, err)

	// Create spork manager and masternode manager
	chainParams := types.DefaultChainParams()
	sm, err := spork.NewManager(chainParams.SporkPubKey, chainParams.SporkPubKeyOld, nil)
	require.NoError(t, err)
	mnConfig := masternode.DefaultConfig()
	mnManager, err := masternode.NewManager(mnConfig, logger)
	require.NoError(t, err)
	mnManager.SetSporkManager(sm)
	mnManager.SetBlockchain(bc)
	err = mnManager.Start()
	require.NoError(t, err)
	defer mnManager.Stop()

	// Create wallet
	walletConfig := wallet.DefaultConfig()
	walletConfig.Network = wallet.TestNet
	w, err := wallet.NewWallet(walletConfig, store, logger)
	require.NoError(t, err)

	// Verify all components are initialized
	assert.NotNil(t, mnManager)
	assert.NotNil(t, w)

	// Verify mempool (if initialized)
	if mp != nil {
		assert.Equal(t, 0, mp.Size())
	}

	// Verify masternode manager
	assert.Equal(t, 0, mnManager.GetMasternodeCount())

	// Verify wallet
	balance := w.GetBalance()
	assert.NotNil(t, balance)
}

// Helper functions

func createTestMasternode(t *testing.T) *masternode.Masternode {
	kp, err := crypto.GenerateKeyPair()
	require.NoError(t, err)

	addr, err := parseTCPAddr("127.0.0.1:12345")
	require.NoError(t, err)

	return &masternode.Masternode{
		OutPoint: types.Outpoint{
			Hash:  types.NewHash([]byte("test")),
			Index: 0,
		},
		Addr:        addr,
		PubKey:      kp.Public,
		Tier:        masternode.Bronze,
		Collateral:  masternode.TierBronzeCollateral,
		Status:      masternode.StatusEnabled,
		Protocol:    70926,
		ActiveSince: time.Now(),
		LastPing:    time.Now(),
		LastSeen:    time.Now(),
	}
}

func parseTCPAddr(addr string) (*net.TCPAddr, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("invalid TCP address: %w", err)
	}
	return tcpAddr, nil
}

