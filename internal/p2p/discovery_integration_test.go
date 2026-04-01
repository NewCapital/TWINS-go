//go:build integration
// +build integration

package p2p

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration test utilities

func setupIntegrationDiscovery(t *testing.T, seeds []string, dnsSeeds []string) *PeerDiscovery {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel) // Reduce noise

	tmpDir := t.TempDir()

	pd := NewPeerDiscovery(DiscoveryConfig{
		Logger:         logger,
		Network:        "testnet",
		Seeds:          seeds,
		DNSSeeds:       dnsSeeds,
		MaxPeers:       100,
		DataDir:        tmpDir,
		DNSSeedEnabled: true,
	})

	return pd
}

func createIntegrationAddress(ip string, port uint16) *NetAddress {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		panic("invalid IP: " + ip)
	}

	return &NetAddress{
		Time:     uint32(time.Now().Unix()),
		Services: SFNodeNetwork,
		IP:       parsedIP,
		Port:     port,
	}
}

// Integration Tests

func TestColdStartBootstrap(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create discovery with seed addresses (simulating DNS seeds)
	seeds := []string{
		"192.168.1.100:37817",
		"192.168.1.101:37817",
		"192.168.1.102:37817",
		"10.0.1.50:37817",
		"10.0.1.51:37817",
	}

	pd := setupIntegrationDiscovery(t, seeds, nil)

	// Verify starts with empty address book (cold start)
	pd.addrMu.RLock()
	initialCount := len(pd.addresses)
	pd.addrMu.RUnlock()
	assert.Equal(t, 0, initialCount, "Should start with empty addresses")

	// Start discovery
	pd.Start()
	defer pd.Stop()

	// Give time for seed loading
	time.Sleep(100 * time.Millisecond)

	// Verify seed addresses were loaded
	pd.addrMu.RLock()
	finalCount := len(pd.addresses)
	pd.addrMu.RUnlock()

	assert.GreaterOrEqual(t, finalCount, len(seeds), "Should have loaded seed addresses")

	// Verify addresses are in the new bucket (not tried yet)
	pd.addrMu.RLock()
	newCount := len(pd.new)
	pd.addrMu.RUnlock()

	assert.Equal(t, finalCount, newCount, "All addresses should be in new bucket on cold start")
}

func TestWarmStartBootstrap(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	// Phase 1: Create discovery and populate with addresses
	pd1 := NewPeerDiscovery(DiscoveryConfig{Logger: logger, Network: "testnet", MaxPeers: 100, DataDir: tmpDir, DNSSeedEnabled: true})

	addresses := make([]*NetAddress, 100)
	for i := 0; i < 100; i++ {
		addr := createIntegrationAddress(fmt.Sprintf("192.168.%d.%d", i/256, i%256), 37817)
		addresses[i] = addr

		known := &KnownAddress{
			Addr:     addr,
			LastSeen: time.Now(),
			Services: SFNodeNetwork,
		}

		// Mark some as successful (so they go to tried bucket)
		if i%3 == 0 {
			known.LastSuccess = time.Now().Add(-1 * time.Hour)
		}

		pd1.AddAddress(known, nil, SourceDNS)
	}

	// Save addresses
	pd1.addrMu.RLock()
	addrsToSave := make(map[string]*KnownAddress, len(pd1.addresses))
	for k, v := range pd1.addresses {
		addrsToSave[k] = v
	}
	pd1.addrMu.RUnlock()

	err := pd1.addrDB.Save(addrsToSave)
	require.NoError(t, err)

	// Phase 2: Create new discovery (warm start)
	startTime := time.Now()
	pd2 := NewPeerDiscovery(DiscoveryConfig{Logger: logger, Network: "testnet", MaxPeers: 100, DataDir: tmpDir, DNSSeedEnabled: true})
	loadDuration := time.Since(startTime)

	// Verify fast load (< 5 seconds target, should be much faster)
	assert.Less(t, loadDuration, 5*time.Second, "Should load quickly on warm start")
	assert.Less(t, loadDuration, 100*time.Millisecond, "Should load in under 100ms")

	// Verify addresses were loaded
	pd2.addrMu.RLock()
	loadedCount := len(pd2.addresses)
	triedCount := len(pd2.tried)
	newCount := len(pd2.new)
	pd2.addrMu.RUnlock()

	assert.Equal(t, 100, loadedCount, "Should have loaded all addresses")
	assert.Greater(t, triedCount, 0, "Should have some tried addresses")
	assert.Greater(t, newCount, 0, "Should have some new addresses")
	assert.Equal(t, loadedCount, triedCount+newCount, "All addresses should be in tried or new bucket")
}

func TestPeerDiversification(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pd := setupIntegrationDiscovery(t, nil, nil)

	// Add 50 addresses in same /16 network (192.168.x.x)
	for i := 0; i < 50; i++ {
		addr := createIntegrationAddress(fmt.Sprintf("192.168.1.%d", i+1), 37817)
		known := &KnownAddress{
			Addr:     addr,
			LastSeen: time.Now(),
			Services: SFNodeNetwork,
		}
		pd.AddAddress(known, nil, SourceDNS)
	}

	// Add 50 addresses in different /16 networks
	for i := 0; i < 50; i++ {
		addr := createIntegrationAddress(fmt.Sprintf("10.%d.1.1", i), 37817)
		known := &KnownAddress{
			Addr:     addr,
			LastSeen: time.Now(),
			Services: SFNodeNetwork,
		}
		pd.AddAddress(known, nil, SourceDNS)
	}

	// Get addresses and check diversity
	selectedAddrs := pd.GetAddresses(20)

	// Count network groups
	groups := make(map[string]int)
	for _, addr := range selectedAddrs {
		group := getNetworkGroup(addr.IP)
		groups[group]++
	}

	// With 20 addresses selected and good diversity, we should have multiple groups
	// Not all from same group (which would indicate clustering)
	assert.GreaterOrEqual(t, len(groups), 2, "Should have addresses from multiple network groups")

	// The largest group should not dominate (shouldn't have more than 50% of addresses)
	maxGroupSize := 0
	for _, count := range groups {
		if count > maxGroupSize {
			maxGroupSize = count
		}
	}
	assert.LessOrEqual(t, maxGroupSize, len(selectedAddrs)/2+1,
		"No single network group should dominate selection")
}

func TestFailedPeerEviction(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pd := setupIntegrationDiscovery(t, nil, nil)

	// Add an address
	addr := createIntegrationAddress("192.168.1.100", 37817)
	known := &KnownAddress{
		Addr:     addr,
		LastSeen: time.Now(),
		Services: SFNodeNetwork,
	}
	pd.AddAddress(known, nil, SourceDNS)

	// Mark it as failed multiple times
	for i := 0; i < 15; i++ {
		pd.MarkAttempt(addr)
	}

	// Verify address is still there but has high attempt count
	pd.addrMu.RLock()
	knownAddr := pd.addresses[addr.String()]
	pd.addrMu.RUnlock()

	require.NotNil(t, knownAddr)
	assert.Equal(t, int32(15), knownAddr.Attempts)

	// Run cleanup
	pd.cleanupAddresses()

	// Verify address was removed (>10 attempts with no success)
	pd.addrMu.RLock()
	_, exists := pd.addresses[addr.String()]
	pd.addrMu.RUnlock()

	assert.False(t, exists, "Address with many failures should be removed during cleanup")
}

func TestStaleAddressRemoval(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pd := setupIntegrationDiscovery(t, nil, nil)

	// Add an old address (> AddressTimeout)
	oldAddr := createIntegrationAddress("192.168.1.100", 37817)
	oldKnown := &KnownAddress{
		Addr:     oldAddr,
		LastSeen: time.Now().Add(-8 * 24 * time.Hour), // 8 days old
		Services: SFNodeNetwork,
	}
	pd.AddAddress(oldKnown, nil, SourceDNS)

	// Add a recent address
	recentAddr := createIntegrationAddress("192.168.1.101", 37817)
	recentKnown := &KnownAddress{
		Addr:     recentAddr,
		LastSeen: time.Now(),
		Services: SFNodeNetwork,
	}
	pd.AddAddress(recentKnown, nil, SourceDNS)

	// Run cleanup
	pd.cleanupAddresses()

	// Verify old address was removed
	pd.addrMu.RLock()
	_, oldExists := pd.addresses[oldAddr.String()]
	_, recentExists := pd.addresses[recentAddr.String()]
	pd.addrMu.RUnlock()

	assert.False(t, oldExists, "Stale address should be removed")
	assert.True(t, recentExists, "Recent address should remain")
}

func TestPermanentAddressProtection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pd := setupIntegrationDiscovery(t, nil, nil)

	// Add permanent address that's old and has failures
	permAddr := createIntegrationAddress("192.168.1.100", 37817)
	permKnown := &KnownAddress{
		Addr:      permAddr,
		LastSeen:  time.Now().Add(-30 * 24 * time.Hour), // 30 days old
		Services:  SFNodeNetwork,
		Attempts:  20, // Many failures
		Permanent: true,
	}
	pd.AddAddress(permKnown, nil, SourceConfig)

	// Add non-permanent address with same characteristics
	tempAddr := createIntegrationAddress("192.168.1.101", 37817)
	tempKnown := &KnownAddress{
		Addr:      tempAddr,
		LastSeen:  time.Now().Add(-30 * 24 * time.Hour),
		Services:  SFNodeNetwork,
		Attempts:  20,
		Permanent: false,
	}
	pd.AddAddress(tempKnown, nil, SourceDNS)

	// Run cleanup
	pd.cleanupAddresses()

	// Verify permanent address was NOT removed, but temporary was
	pd.addrMu.RLock()
	_, permExists := pd.addresses[permAddr.String()]
	_, tempExists := pd.addresses[tempAddr.String()]
	pd.addrMu.RUnlock()

	assert.True(t, permExists, "Permanent address should never be removed")
	assert.False(t, tempExists, "Temporary address with failures should be removed")
}

func TestAddressSourceTracking(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pd := setupIntegrationDiscovery(t, nil, nil)

	// Add addresses from different sources
	sourceAddr := createIntegrationAddress("10.0.1.1", 37817)

	addr1 := createIntegrationAddress("192.168.1.100", 37817)
	known1 := &KnownAddress{
		Addr:     addr1,
		LastSeen: time.Now(),
		Services: SFNodeNetwork,
	}
	pd.AddAddress(known1, sourceAddr, SourcePeer)

	addr2 := createIntegrationAddress("192.168.1.101", 37817)
	known2 := &KnownAddress{
		Addr:     addr2,
		LastSeen: time.Now(),
		Services: SFNodeNetwork,
	}
	pd.AddAddress(known2, nil, SourceDNS)

	// Verify source tracking
	pd.addrMu.RLock()
	stored1 := pd.addresses[addr1.String()]
	stored2 := pd.addresses[addr2.String()]
	pd.addrMu.RUnlock()

	assert.NotNil(t, stored1.Source, "Address from peer should have source")
	assert.Equal(t, sourceAddr.String(), stored1.Source.String())
	assert.Nil(t, stored2.Source, "Address from DNS should have no specific source")
}

func TestMasternodePrioritization(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	pd := setupIntegrationDiscovery(t, nil, nil)

	// Add regular nodes
	for i := 0; i < 50; i++ {
		addr := createIntegrationAddress(fmt.Sprintf("192.168.1.%d", i+1), 37817)
		known := &KnownAddress{
			Addr:        addr,
			LastSeen:    time.Now(),
			LastSuccess: time.Now().Add(-1 * time.Hour),
			Services:    SFNodeNetwork,
		}
		pd.AddAddress(known, nil, SourceDNS)
	}

	// Add masternodes
	for i := 0; i < 10; i++ {
		addr := createIntegrationAddress(fmt.Sprintf("10.0.1.%d", i+1), 37817)
		known := &KnownAddress{
			Addr:        addr,
			LastSeen:    time.Now(),
			LastSuccess: time.Now().Add(-1 * time.Hour),
			Services:    SFNodeNetwork | SFNodeMasternode,
		}
		pd.AddAddress(known, nil, SourceDNS)
	}

	// Get addresses multiple times and count selections
	masternodeSelections := 0
	totalSelections := 0

	for i := 0; i < 10; i++ {
		selectedAddrs := pd.GetAddresses(10)
		for _, addr := range selectedAddrs {
			totalSelections++
			// Check if this is a masternode (10.0.1.x range)
			if addr.IP.String()[:6] == "10.0.1" {
				masternodeSelections++
			}
		}
	}

	// With 10 masternodes and 50 regular nodes (16.7% masternodes),
	// and 5x selection boost, masternodes should be selected more than their proportion
	masternodeRate := float64(masternodeSelections) / float64(totalSelections)
	expectedRate := 0.167 // Base rate without boost

	assert.Greater(t, masternodeRate, expectedRate,
		"Masternodes should be selected more often than their proportion due to priority boost")
}
