package masternode

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twins-dev/twins-core/pkg/types"
)

func TestCacheStatusDemotionOnSave(t *testing.T) {
	// Test that ENABLED masternodes are saved as PRE_ENABLED
	// This ensures they must receive a fresh ping after cache load

	// Create temporary directory for cache
	tmpDir, err := os.MkdirTemp("", "mncache-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create manager and add masternodes with different statuses
	manager := createTestManager(t)

	// Create ENABLED masternode with ping gap < MinPingSeconds (300s)
	// After save (ENABLED -> PRE_ENABLED) and load + recalculation,
	// it should stay PRE_ENABLED because gap < MinPingSeconds.
	// If demotion didn't happen, it would stay ENABLED.
	mnEnabled := createTestMasternode(Bronze)
	mnEnabled.Status = StatusEnabled
	mnEnabled.OutPoint.Index = 1
	// Set sigTime to 2 minutes ago (gap will be < 300s MinPingSeconds)
	sigTime := time.Now().Add(-2 * time.Minute).Unix()
	mnEnabled.SigTime = sigTime
	// Set ping 1 second after sigTime (gap = 1 second < 300s)
	mnEnabled.LastPingMessage = &MasternodePing{
		OutPoint:  mnEnabled.OutPoint,
		BlockHash: types.Hash{},
		SigTime:   sigTime + 1,
	}
	require.NoError(t, manager.AddMasternode(mnEnabled))

	// Add PRE_ENABLED masternode (should stay PRE_ENABLED after recalculation)
	mnPreEnabled := createTestMasternode(Bronze)
	mnPreEnabled.Status = StatusPreEnabled
	mnPreEnabled.OutPoint.Index = 2
	mnPreEnabled.SigTime = sigTime
	mnPreEnabled.LastPingMessage = &MasternodePing{
		OutPoint:  mnPreEnabled.OutPoint,
		BlockHash: types.Hash{},
		SigTime:   sigTime + 1,
	}
	require.NoError(t, manager.AddMasternode(mnPreEnabled))

	// Add EXPIRED masternode with old ping (should stay EXPIRED after recalculation)
	mnExpired := createTestMasternode(Bronze)
	mnExpired.Status = StatusExpired
	mnExpired.OutPoint.Index = 3
	oldTime := time.Now().Add(-3 * time.Hour).Unix()
	mnExpired.SigTime = oldTime
	mnExpired.LastPingMessage = &MasternodePing{
		OutPoint:  mnExpired.OutPoint,
		BlockHash: types.Hash{},
		SigTime:   oldTime,
	}
	require.NoError(t, manager.AddMasternode(mnExpired))

	// Save cache
	networkMagic := []byte{0x01, 0x02, 0x03, 0x04}
	err = manager.SaveCache(tmpDir, networkMagic)
	require.NoError(t, err)

	// Verify cache file was created
	cachePath := filepath.Join(tmpDir, "mncache.dat")
	_, err = os.Stat(cachePath)
	require.NoError(t, err)

	// Create new manager and load cache
	manager2 := createTestManager(t)
	err = manager2.LoadCache(tmpDir, networkMagic)
	require.NoError(t, err)

	// Verify loaded count
	assert.Equal(t, 3, manager2.GetMasternodeCount())

	// Verify status demotion worked:
	// The ENABLED masternode was saved as PRE_ENABLED.
	// No recalculation happens after load, so it stays PRE_ENABLED
	// until a new ping is received from the network.
	mnLoaded, err := manager2.GetMasternode(mnEnabled.OutPoint)
	require.NoError(t, err)
	assert.Equal(t, StatusPreEnabled, mnLoaded.Status,
		"ENABLED masternode should be PRE_ENABLED after save+load (demotion on save)")

	// Verify PRE_ENABLED stays PRE_ENABLED
	mnPreLoaded, err := manager2.GetMasternode(mnPreEnabled.OutPoint)
	require.NoError(t, err)
	assert.Equal(t, StatusPreEnabled, mnPreLoaded.Status,
		"PRE_ENABLED masternode should stay PRE_ENABLED")

	// Verify EXPIRED stays EXPIRED (no recalculation after load)
	mnExpiredLoaded, err := manager2.GetMasternode(mnExpired.OutPoint)
	require.NoError(t, err)
	assert.Equal(t, StatusExpired, mnExpiredLoaded.Status,
		"EXPIRED masternode should stay EXPIRED after load (no recalculation)")
}

func TestCacheStatusPreservedAfterLoad(t *testing.T) {
	// Test that status is preserved after cache load (no recalculation)
	// Masternodes must receive a fresh ping to change status

	// Create temporary directory for cache
	tmpDir, err := os.MkdirTemp("", "mncache-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	manager := createTestManager(t)

	// Create masternode with old ping - status should be preserved, not recalculated
	mnOld := createTestMasternode(Bronze)
	mnOld.Status = StatusPreEnabled
	mnOld.OutPoint.Index = 1
	// Set ping time to be very old (>2 hours ago)
	oldTime := time.Now().Add(-3 * time.Hour).Unix()
	mnOld.SigTime = oldTime
	mnOld.LastPingMessage = &MasternodePing{
		OutPoint:  mnOld.OutPoint,
		BlockHash: types.Hash{},
		SigTime:   oldTime,
	}
	require.NoError(t, manager.AddMasternode(mnOld))

	// Save cache
	networkMagic := []byte{0x01, 0x02, 0x03, 0x04}
	err = manager.SaveCache(tmpDir, networkMagic)
	require.NoError(t, err)

	// Create new manager and load cache
	manager2 := createTestManager(t)
	err = manager2.LoadCache(tmpDir, networkMagic)
	require.NoError(t, err)

	// Status should be preserved as PRE_ENABLED (no recalculation)
	// Even though ping is old, we don't recalculate on load
	mn, err := manager2.GetMasternode(mnOld.OutPoint)
	require.NoError(t, err)
	require.NotNil(t, mn)

	assert.Equal(t, StatusPreEnabled, mn.Status,
		"Status should be preserved after load, not recalculated")
}

func TestCacheEnabledDemotedToPreEnabled(t *testing.T) {
	// Test that ENABLED masternodes are saved as PRE_ENABLED and stay that way
	// They must receive a fresh ping from the network to become ENABLED again

	// Create temporary directory for cache
	tmpDir, err := os.MkdirTemp("", "mncache-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	manager := createTestManager(t)

	// Create ENABLED masternode with recent ping
	mnFresh := createTestMasternode(Bronze)
	mnFresh.Status = StatusEnabled // Will be saved as PRE_ENABLED
	mnFresh.OutPoint.Index = 1
	sigTime := time.Now().Add(-15 * time.Minute).Unix()
	mnFresh.SigTime = sigTime
	recentTime := time.Now().Add(-5 * time.Second).Unix()
	mnFresh.LastPingMessage = &MasternodePing{
		OutPoint:  mnFresh.OutPoint,
		BlockHash: types.Hash{},
		SigTime:   recentTime,
	}
	require.NoError(t, manager.AddMasternode(mnFresh))

	// Verify it's ENABLED before save
	assert.Equal(t, StatusEnabled, mnFresh.Status)

	// Save cache (ENABLED -> PRE_ENABLED demotion)
	networkMagic := []byte{0x01, 0x02, 0x03, 0x04}
	err = manager.SaveCache(tmpDir, networkMagic)
	require.NoError(t, err)

	// Create new manager and load cache
	manager2 := createTestManager(t)
	err = manager2.LoadCache(tmpDir, networkMagic)
	require.NoError(t, err)

	// After load, should be PRE_ENABLED (demoted on save, no recalculation)
	// Even though ping data would allow ENABLED, we don't recalculate
	mn, err := manager2.GetMasternode(mnFresh.OutPoint)
	require.NoError(t, err)
	require.NotNil(t, mn)

	assert.Equal(t, StatusPreEnabled, mn.Status,
		"ENABLED masternode should be PRE_ENABLED after save+load (must receive new ping)")
}

func TestCacheExpiredMasternodePingRecovery(t *testing.T) {
	// Test that EXPIRED masternode loaded from cache can recover
	// when it receives a fresh ping (simulated via UpdateStatus)

	tmpDir, err := os.MkdirTemp("", "mncache-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	manager := createTestManager(t)

	// Create EXPIRED masternode
	mnExpired := createTestMasternode(Bronze)
	mnExpired.Status = StatusExpired
	mnExpired.OutPoint.Index = 1
	// Old ping data (expired)
	oldTime := time.Now().Add(-3 * time.Hour).Unix()
	mnExpired.SigTime = oldTime
	mnExpired.LastPingMessage = &MasternodePing{
		OutPoint:  mnExpired.OutPoint,
		BlockHash: types.Hash{},
		SigTime:   oldTime,
	}
	require.NoError(t, manager.AddMasternode(mnExpired))

	// Save cache (EXPIRED stays EXPIRED, no demotion)
	networkMagic := []byte{0x01, 0x02, 0x03, 0x04}
	err = manager.SaveCache(tmpDir, networkMagic)
	require.NoError(t, err)

	// Load into new manager
	manager2 := createTestManager(t)
	err = manager2.LoadCache(tmpDir, networkMagic)
	require.NoError(t, err)

	// Verify loaded as EXPIRED (preserved, no recalculation)
	mn, err := manager2.GetMasternode(mnExpired.OutPoint)
	require.NoError(t, err)
	assert.Equal(t, StatusExpired, mn.Status, "Should load as EXPIRED")

	// Simulate receiving a fresh ping by updating the ping data
	// and calling UpdateStatus (this is what ProcessPing does)
	freshSigTime := time.Now().Add(-15 * time.Minute).Unix()
	freshPingTime := time.Now().Add(-5 * time.Second).Unix()
	mn.SigTime = freshSigTime
	mn.LastPingMessage = &MasternodePing{
		OutPoint:  mn.OutPoint,
		BlockHash: types.Hash{},
		SigTime:   freshPingTime,
	}

	// Call UpdateStatus (simulating what ProcessPing would do)
	mn.UpdateStatus(time.Now(), DefaultConfig().ExpireTime)

	// Should recover to ENABLED (fresh ping, gap > MinPingSeconds)
	assert.Equal(t, StatusEnabled, mn.Status,
		"EXPIRED masternode should recover to ENABLED after receiving fresh ping")
}

func TestCacheRoundTrip(t *testing.T) {
	// Test basic cache save/load round trip preserves masternode data

	tmpDir, err := os.MkdirTemp("", "mncache-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	manager := createTestManager(t)

	// Add masternodes of different tiers
	for i, tier := range []MasternodeTier{Bronze, Silver, Gold, Platinum} {
		mn := createTestMasternode(tier)
		mn.OutPoint.Index = uint32(i)
		// Set recent ping to ensure they stay ENABLED after recalculation
		// Gap between sigTime and ping must be > MinPingSeconds (300)
		recentTime := time.Now().Add(-5 * time.Second).Unix()
		mn.SigTime = time.Now().Add(-15 * time.Minute).Unix()
		mn.LastPingMessage = &MasternodePing{
			OutPoint:  mn.OutPoint,
			BlockHash: types.Hash{},
			SigTime:   recentTime,
		}
		require.NoError(t, manager.AddMasternode(mn))
	}

	assert.Equal(t, 4, manager.GetMasternodeCount())

	// Save cache
	networkMagic := []byte{0x01, 0x02, 0x03, 0x04}
	err = manager.SaveCache(tmpDir, networkMagic)
	require.NoError(t, err)

	// Load into new manager
	manager2 := createTestManager(t)
	err = manager2.LoadCache(tmpDir, networkMagic)
	require.NoError(t, err)

	// Verify all masternodes were loaded
	assert.Equal(t, 4, manager2.GetMasternodeCount())
}

func TestCacheWrongNetworkMagic(t *testing.T) {
	// Test that cache load fails with wrong network magic

	tmpDir, err := os.MkdirTemp("", "mncache-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	manager := createTestManager(t)
	mn := createTestMasternode(Bronze)
	require.NoError(t, manager.AddMasternode(mn))

	// Save with one magic
	saveMagic := []byte{0x01, 0x02, 0x03, 0x04}
	err = manager.SaveCache(tmpDir, saveMagic)
	require.NoError(t, err)

	// Load with different magic should fail
	manager2 := createTestManager(t)
	loadMagic := []byte{0x05, 0x06, 0x07, 0x08}
	err = manager2.LoadCache(tmpDir, loadMagic)
	assert.ErrorIs(t, err, ErrCacheInvalidNetwork)
}

func TestCacheFileNotFound(t *testing.T) {
	// Test that loading non-existent cache returns proper error

	tmpDir, err := os.MkdirTemp("", "mncache-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	manager := createTestManager(t)
	networkMagic := []byte{0x01, 0x02, 0x03, 0x04}

	err = manager.LoadCache(tmpDir, networkMagic)
	assert.ErrorIs(t, err, ErrCacheFileNotFound)
}

func TestCacheCorruptedChecksum(t *testing.T) {
	// Test that corrupted cache is detected

	tmpDir, err := os.MkdirTemp("", "mncache-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	manager := createTestManager(t)
	mn := createTestMasternode(Bronze)
	require.NoError(t, manager.AddMasternode(mn))

	// Save valid cache
	networkMagic := []byte{0x01, 0x02, 0x03, 0x04}
	err = manager.SaveCache(tmpDir, networkMagic)
	require.NoError(t, err)

	// Corrupt the cache file
	cachePath := filepath.Join(tmpDir, "mncache.dat")
	data, err := os.ReadFile(cachePath)
	require.NoError(t, err)
	// Flip some bits in the middle
	if len(data) > 50 {
		data[50] ^= 0xFF
	}
	err = os.WriteFile(cachePath, data, 0644)
	require.NoError(t, err)

	// Load should fail with checksum error
	manager2 := createTestManager(t)
	err = manager2.LoadCache(tmpDir, networkMagic)
	assert.ErrorIs(t, err, ErrCacheCorrupted)
}

