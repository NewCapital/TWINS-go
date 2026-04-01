package p2p

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPeerMetadataTracking verifies that connection attempts and successes are tracked
func TestPeerMetadataTracking(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)
	logger.SetLevel(logrus.DebugLevel)

	tmpDir := t.TempDir()

	// Create discovery with temp directory
	pd := NewPeerDiscovery(DiscoveryConfig{Logger: logger, Network: "testnet", Seeds: []string{}, DNSSeeds: []string{}, MaxPeers: 10, DataDir: tmpDir, DNSSeedEnabled: true})

	// Add a test address
	addr := &NetAddress{
		IP:       net.ParseIP("192.168.1.1"),
		Port:     37817,
		Services: SFNodeNetwork,
	}

	known := &KnownAddress{
		Addr:     addr,
		Services: SFNodeNetwork,
		LastSeen: time.Now(),
	}

	pd.AddAddress(known, addr, SourceDNS)

	// Verify initial state - should have zero timestamps
	pd.addrMu.RLock()
	knownAddr := pd.addresses[addr.String()]
	pd.addrMu.RUnlock()

	require.NotNil(t, knownAddr)
	assert.True(t, knownAddr.LastAttempt.IsZero(), "LastAttempt should be zero initially")
	assert.True(t, knownAddr.LastSuccess.IsZero(), "LastSuccess should be zero initially")
	assert.Equal(t, int32(0), knownAddr.Attempts, "Attempts should be 0 initially")

	// Mark connection attempt
	pd.MarkAttempt(addr)

	// Verify attempt was recorded
	pd.addrMu.RLock()
	knownAddr = pd.addresses[addr.String()]
	pd.addrMu.RUnlock()

	assert.False(t, knownAddr.LastAttempt.IsZero(), "LastAttempt should be set after MarkAttempt")
	assert.True(t, knownAddr.LastSuccess.IsZero(), "LastSuccess should still be zero")
	assert.Equal(t, int32(1), knownAddr.Attempts, "Attempts should be 1 after first attempt")

	lastAttempt := knownAddr.LastAttempt

	// Mark another attempt
	time.Sleep(10 * time.Millisecond)
	pd.MarkAttempt(addr)

	pd.addrMu.RLock()
	knownAddr = pd.addresses[addr.String()]
	pd.addrMu.RUnlock()

	assert.True(t, knownAddr.LastAttempt.After(lastAttempt), "LastAttempt should be updated")
	assert.Equal(t, int32(2), knownAddr.Attempts, "Attempts should be 2 after second attempt")

	// Mark successful connection
	pd.MarkSuccess(addr)

	pd.addrMu.RLock()
	knownAddr = pd.addresses[addr.String()]
	pd.addrMu.RUnlock()

	assert.False(t, knownAddr.LastSuccess.IsZero(), "LastSuccess should be set after MarkSuccess")
	assert.Equal(t, int32(0), knownAddr.Attempts, "Attempts should reset to 0 after success")

	// Save and reload to verify persistence
	pd.addrMu.RLock()
	addresses := make(map[string]*KnownAddress)
	for k, v := range pd.addresses {
		addresses[k] = v.Clone()
	}
	pd.addrMu.RUnlock()

	err := pd.addrDB.Save(addresses)
	require.NoError(t, err, "Save should succeed")

	// Verify file was created in correct location
	peersFile := filepath.Join(tmpDir, "peers.json")
	assert.FileExists(t, peersFile, "peers.json should exist in dataDir")

	// Load addresses from disk
	loaded, err := pd.addrDB.Load()
	require.NoError(t, err, "Load should succeed")

	loadedKnown := loaded[addr.String()]
	require.NotNil(t, loadedKnown, "Address should be loaded from disk")

	// Verify metadata persisted correctly
	assert.False(t, loadedKnown.LastAttempt.IsZero(), "Loaded LastAttempt should not be zero")
	assert.False(t, loadedKnown.LastSuccess.IsZero(), "Loaded LastSuccess should not be zero")
	assert.Equal(t, int32(0), loadedKnown.Attempts, "Loaded Attempts should be 0")

	t.Logf("✓ Metadata tracking working correctly:")
	t.Logf("  - LastAttempt: %v", loadedKnown.LastAttempt)
	t.Logf("  - LastSuccess: %v", loadedKnown.LastSuccess)
	t.Logf("  - Attempts: %d", loadedKnown.Attempts)
	t.Logf("  - File location: %s", peersFile)
}
