// Copyright (c) 2018-2025 The TWINS developers
// Distributed under the MIT/X11 software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package masternode

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/twins-dev/twins-core/pkg/types"
)

func TestPaymentVotesPersistence(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "payment_cache_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Network magic for mainnet
	networkMagic := []byte{0x2f, 0x1c, 0xd3, 0x0a}

	// Create manager with some scheduled payments
	config := DefaultConfig()
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel) // Suppress info logs in tests
	m, err := NewManager(config, logger)
	require.NoError(t, err)

	// Add some scheduled payments
	testPayments := map[uint32][]byte{
		100000: []byte{0x76, 0xa9, 0x14, 0x01, 0x02, 0x03}, // P2PKH script prefix
		100001: []byte{0x76, 0xa9, 0x14, 0x04, 0x05, 0x06},
		100002: []byte{0x76, 0xa9, 0x14, 0x07, 0x08, 0x09},
	}

	m.mu.Lock()
	for height, script := range testPayments {
		m.scheduledPayments[height] = script
	}
	m.mu.Unlock()

	// Save to disk
	err = m.SavePaymentVotes(tmpDir, networkMagic)
	require.NoError(t, err)

	// Verify file was created
	cachePath := filepath.Join(tmpDir, "mnpayments.dat")
	_, err = os.Stat(cachePath)
	require.NoError(t, err, "mnpayments.dat should exist")

	// Create a new manager and load the data
	m2, err := NewManager(config, logger)
	require.NoError(t, err)

	err = m2.LoadPaymentVotes(tmpDir, networkMagic)
	require.NoError(t, err)

	// Verify loaded data matches original
	m2.mu.RLock()
	defer m2.mu.RUnlock()

	assert.Equal(t, len(testPayments), len(m2.scheduledPayments), "Should have same number of scheduled payments")

	for height, expectedScript := range testPayments {
		actualScript, exists := m2.scheduledPayments[height]
		assert.True(t, exists, "Height %d should exist", height)
		assert.Equal(t, expectedScript, actualScript, "Script at height %d should match", height)
	}
}

func TestPaymentVotesPersistence_EmptyData(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "payment_cache_empty_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	networkMagic := []byte{0x2f, 0x1c, 0xd3, 0x0a}

	config := DefaultConfig()
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)
	m, err := NewManager(config, logger)
	require.NoError(t, err)

	// Save empty data
	err = m.SavePaymentVotes(tmpDir, networkMagic)
	require.NoError(t, err)

	// Load into new manager
	m2, err := NewManager(config, logger)
	require.NoError(t, err)

	err = m2.LoadPaymentVotes(tmpDir, networkMagic)
	require.NoError(t, err)

	m2.mu.RLock()
	assert.Equal(t, 0, len(m2.scheduledPayments), "Should have no scheduled payments")
	m2.mu.RUnlock()
}

func TestPaymentVotesPersistence_FileNotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "payment_cache_notfound_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	networkMagic := []byte{0x2f, 0x1c, 0xd3, 0x0a}

	config := DefaultConfig()
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)
	m, err := NewManager(config, logger)
	require.NoError(t, err)

	// Try to load non-existent file
	err = m.LoadPaymentVotes(tmpDir, networkMagic)
	assert.ErrorIs(t, err, ErrPaymentCacheNotFound)
}

func TestPaymentVotesPersistence_WrongNetwork(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "payment_cache_network_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	mainnetMagic := []byte{0x2f, 0x1c, 0xd3, 0x0a}
	testnetMagic := []byte{0xe5, 0xba, 0xc5, 0xb6}

	config := DefaultConfig()
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)
	m, err := NewManager(config, logger)
	require.NoError(t, err)

	// Add some data
	m.mu.Lock()
	m.scheduledPayments[100000] = []byte{0x76, 0xa9, 0x14, 0x01, 0x02, 0x03}
	m.mu.Unlock()

	// Save with mainnet magic
	err = m.SavePaymentVotes(tmpDir, mainnetMagic)
	require.NoError(t, err)

	// Try to load with testnet magic - should fail
	m2, err := NewManager(config, logger)
	require.NoError(t, err)

	err = m2.LoadPaymentVotes(tmpDir, testnetMagic)
	assert.ErrorIs(t, err, ErrPaymentCacheInvalidNetwork)
}

func TestPaymentVotesPersistence_CorruptedFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "payment_cache_corrupt_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	networkMagic := []byte{0x2f, 0x1c, 0xd3, 0x0a}

	// Create a corrupted cache file
	cachePath := filepath.Join(tmpDir, "mnpayments.dat")
	err = os.WriteFile(cachePath, []byte("this is not valid cache data plus some padding for checksum"), 0644)
	require.NoError(t, err)

	config := DefaultConfig()
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)
	m, err := NewManager(config, logger)
	require.NoError(t, err)

	err = m.LoadPaymentVotes(tmpDir, networkMagic)
	assert.Error(t, err, "Should fail on corrupted file")
}

func TestPaymentVotesPersistence_InvalidNetworkMagicLength(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "payment_cache_magic_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	invalidMagic := []byte{0x2f, 0x1c, 0xd3} // Only 3 bytes

	config := DefaultConfig()
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)
	m, err := NewManager(config, logger)
	require.NoError(t, err)

	err = m.SavePaymentVotes(tmpDir, invalidMagic)
	assert.Error(t, err, "Should fail with invalid magic length")

	err = m.LoadPaymentVotes(tmpDir, invalidMagic)
	assert.Error(t, err, "Should fail with invalid magic length")
}

func TestPaymentVotesPersistence_LargeDataSet(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "payment_cache_large_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	networkMagic := []byte{0x2f, 0x1c, 0xd3, 0x0a}

	config := DefaultConfig()
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)
	m, err := NewManager(config, logger)
	require.NoError(t, err)

	// Add a large number of scheduled payments (simulating long-running node)
	numPayments := 1000
	m.mu.Lock()
	for i := 0; i < numPayments; i++ {
		height := uint32(100000 + i)
		script := make([]byte, 25) // Typical P2PKH script length
		script[0] = 0x76           // OP_DUP
		script[1] = 0xa9           // OP_HASH160
		script[2] = 0x14           // Push 20 bytes
		// Fill with height-dependent data for uniqueness
		script[3] = byte(i)
		script[4] = byte(i >> 8)
		m.scheduledPayments[height] = script
	}
	m.mu.Unlock()

	// Save
	err = m.SavePaymentVotes(tmpDir, networkMagic)
	require.NoError(t, err)

	// Load
	m2, err := NewManager(config, logger)
	require.NoError(t, err)

	err = m2.LoadPaymentVotes(tmpDir, networkMagic)
	require.NoError(t, err)

	m2.mu.RLock()
	assert.Equal(t, numPayments, len(m2.scheduledPayments), "Should load all %d payments", numPayments)
	m2.mu.RUnlock()
}

func TestPaymentVotesPersistence_AtomicWrite(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "payment_cache_atomic_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	networkMagic := []byte{0x2f, 0x1c, 0xd3, 0x0a}

	config := DefaultConfig()
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)
	m, err := NewManager(config, logger)
	require.NoError(t, err)

	// Add initial data
	m.mu.Lock()
	m.scheduledPayments[100000] = []byte{0x76, 0xa9, 0x14, 0x01, 0x02, 0x03}
	m.mu.Unlock()

	// Save
	err = m.SavePaymentVotes(tmpDir, networkMagic)
	require.NoError(t, err)

	// Verify .tmp file doesn't exist (atomic rename completed)
	tmpPath := filepath.Join(tmpDir, "mnpayments.dat.tmp")
	_, err = os.Stat(tmpPath)
	assert.True(t, os.IsNotExist(err), "Temp file should not exist after successful save")
}

func TestPaymentCacheData_Serialization(t *testing.T) {
	// Test that block payees serialize and deserialize correctly
	data := &PaymentCacheData{
		WinnerVotes: make(map[types.Hash]*PaymentWinnerCacheEntry),
		BlockPayees: map[uint32]*BlockPayeesCacheEntry{
			100000: {
				BlockHeight: 100000,
				Payees: []*PayeeCacheEntry{
					{ScriptPubKey: []byte{0x76, 0xa9, 0x14}, Votes: 5},
					{ScriptPubKey: []byte{0xa9, 0x14, 0x00}, Votes: 3},
				},
			},
		},
	}

	// Serialize
	var buf bytes.Buffer
	err := serializePaymentCacheData(&buf, data)
	require.NoError(t, err)

	// Deserialize
	reader := bytes.NewReader(buf.Bytes())
	loadedData, err := deserializePaymentCacheData(reader)
	require.NoError(t, err)

	// Verify
	assert.Equal(t, len(data.BlockPayees), len(loadedData.BlockPayees))
	assert.Equal(t, data.BlockPayees[100000].BlockHeight, loadedData.BlockPayees[100000].BlockHeight)
	assert.Equal(t, len(data.BlockPayees[100000].Payees), len(loadedData.BlockPayees[100000].Payees))
	assert.Equal(t, data.BlockPayees[100000].Payees[0].Votes, loadedData.BlockPayees[100000].Payees[0].Votes)
}
