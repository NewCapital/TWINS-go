package p2p

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test utilities

// setupTestDiscovery creates a test peer discovery instance
func setupTestDiscovery(t *testing.T) *PeerDiscovery {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests

	tmpDir := t.TempDir()

	pd := NewPeerDiscovery(DiscoveryConfig{
		Logger:         logger,
		Network:        "testnet",
		Seeds:          nil,
		DNSSeeds:       nil,
		MaxPeers:       100,
		DataDir:        tmpDir,
		DNSSeedEnabled: true,
	})

	return pd
}

// createTestAddress creates a test NetAddress
func createTestAddress(ipPort string) *NetAddress {
	host, port, err := net.SplitHostPort(ipPort)
	if err != nil {
		panic(err)
	}

	ip := net.ParseIP(host)
	if ip == nil {
		panic("invalid IP: " + host)
	}

	var portNum uint16
	if _, err := net.LookupPort("tcp", port); err == nil {
		portNum = 37817 // default port
	}

	return &NetAddress{
		Time:     uint32(time.Now().Unix()),
		Services: SFNodeNetwork,
		IP:       ip,
		Port:     portNum,
	}
}

// Unit Tests

func TestMarkAttempt(t *testing.T) {
	pd := setupTestDiscovery(t)
	addr := createTestAddress("192.168.1.100:37817")

	// Add address first
	known := &KnownAddress{
		Addr:     addr,
		LastSeen: time.Now(),
		Services: SFNodeNetwork,
	}
	pd.AddAddress(known, nil, SourceSeed)

	// Mark multiple attempts
	for i := 0; i < 5; i++ {
		pd.MarkAttempt(addr)
	}

	// Verify counter incremented
	pd.addrMu.RLock()
	knownAddr := pd.addresses[addr.String()]
	pd.addrMu.RUnlock()

	assert.NotNil(t, knownAddr)
	assert.Equal(t, int32(5), knownAddr.Attempts)
	assert.False(t, knownAddr.LastAttempt.IsZero())
}

func TestMarkSuccess(t *testing.T) {
	pd := setupTestDiscovery(t)
	addr := createTestAddress("192.168.1.100:37817")

	// Add address
	known := &KnownAddress{
		Addr:     addr,
		LastSeen: time.Now(),
		Services: SFNodeNetwork,
	}
	pd.AddAddress(known, nil, SourceSeed)

	// Mark as successful
	pd.MarkSuccess(addr)

	// Verify success tracking
	pd.addrMu.RLock()
	knownAddr := pd.addresses[addr.String()]
	pd.addrMu.RUnlock()

	assert.NotNil(t, knownAddr)
	assert.False(t, knownAddr.LastSuccess.IsZero(), "LastSuccess should be set")
	assert.False(t, knownAddr.IsBad, "Address should not be marked as bad after success")
}

func TestMarkBad(t *testing.T) {
	pd := setupTestDiscovery(t)
	addr := createTestAddress("192.168.1.100:37817")

	// Add address
	known := &KnownAddress{
		Addr:     addr,
		LastSeen: time.Now(),
		Services: SFNodeNetwork,
	}
	pd.AddAddress(known, nil, SourceSeed)

	// Mark as bad
	pd.MarkBad(addr, "test failure")

	// Verify bad flag
	pd.addrMu.RLock()
	knownAddr := pd.addresses[addr.String()]
	pd.addrMu.RUnlock()

	assert.NotNil(t, knownAddr)
	assert.True(t, knownAddr.IsBad)
}

func TestMarkFailureThreshold(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	pd := NewPeerDiscovery(DiscoveryConfig{Logger: logger, Network: "testnet", MaxPeers: 100, DataDir: t.TempDir(), DNSSeedEnabled: true})

	addr := createTestAddress("192.168.1.1:37817")

	// Add address
	known := &KnownAddress{
		Addr:     addr,
		LastSeen: time.Now(),
		Services: SFNodeNetwork,
	}
	pd.AddAddress(known, nil, SourceSeed)

	// Mark failures below threshold - should NOT be marked bad
	for i := 0; i < FailureThreshold-1; i++ {
		pd.MarkAttempt(addr)
		pd.MarkFailure(addr, "connection timeout")
	}

	pd.addrMu.RLock()
	knownAddr := pd.addresses[addr.String()]
	pd.addrMu.RUnlock()

	assert.NotNil(t, knownAddr)
	assert.False(t, knownAddr.IsBad, "Should not be marked bad before threshold")
	assert.Equal(t, int32(FailureThreshold-1), knownAddr.Attempts)

	// One more failure should trigger bad marking
	pd.MarkAttempt(addr)
	pd.MarkFailure(addr, "connection timeout")

	pd.addrMu.RLock()
	knownAddr = pd.addresses[addr.String()]
	pd.addrMu.RUnlock()

	assert.True(t, knownAddr.IsBad, "Should be marked bad after threshold")
	assert.Equal(t, int32(FailureThreshold), knownAddr.Attempts)
}

func TestMarkFailureDecay(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	pd := NewPeerDiscovery(DiscoveryConfig{Logger: logger, Network: "testnet", MaxPeers: 100, DataDir: t.TempDir(), DNSSeedEnabled: true})

	addr := createTestAddress("192.168.1.1:37817")

	// Add address
	known := &KnownAddress{
		Addr:     addr,
		LastSeen: time.Now(),
		Services: SFNodeNetwork,
	}
	pd.AddAddress(known, nil, SourceSeed)

	// Mark two failures
	pd.MarkAttempt(addr)
	pd.MarkAttempt(addr)

	pd.addrMu.RLock()
	assert.Equal(t, int32(2), pd.addresses[addr.String()].Attempts)
	lastAttempt := pd.addresses[addr.String()].LastAttempt
	pd.addrMu.RUnlock()

	// Simulate decay period by manually setting LastAttempt beyond FailureDecayDuration (24h)
	pd.addrMu.Lock()
	pd.addresses[addr.String()].LastAttempt = time.Now().Add(-25 * time.Hour)
	pd.addrMu.Unlock()

	// Next attempt should reset counter
	pd.MarkAttempt(addr)

	pd.addrMu.RLock()
	knownAddr := pd.addresses[addr.String()]
	pd.addrMu.RUnlock()

	assert.Equal(t, int32(1), knownAddr.Attempts, "Attempts should reset after decay period")
	assert.True(t, knownAddr.LastAttempt.After(lastAttempt))
}

func TestKnownAddressClone(t *testing.T) {
	addr := createTestAddress("192.168.1.1:37817")
	source := createTestAddress("192.168.1.100:37817")

	original := &KnownAddress{
		Addr:        addr,
		Source:      source,
		Attempts:    5,
		LastAttempt: time.Now(),
		LastSuccess: time.Now().Add(-1 * time.Hour),
		LastSeen:    time.Now(),
		Services:    SFNodeNetwork | SFNodeMasternode,
		IsBad:       true,
		Permanent:   false,
	}

	// Clone the address
	clone := original.Clone()

	// Verify all fields are copied
	assert.NotNil(t, clone)
	assert.Equal(t, original.Attempts, clone.Attempts)
	assert.Equal(t, original.LastAttempt, clone.LastAttempt)
	assert.Equal(t, original.LastSuccess, clone.LastSuccess)
	assert.Equal(t, original.LastSeen, clone.LastSeen)
	assert.Equal(t, original.Services, clone.Services)
	assert.Equal(t, original.IsBad, clone.IsBad)
	assert.Equal(t, original.Permanent, clone.Permanent)

	// Verify deep copy - modifying clone shouldn't affect original
	assert.NotSame(t, original, clone)
	assert.NotSame(t, original.Addr, clone.Addr)
	assert.NotSame(t, original.Source, clone.Source)
	// Note: net.IP is []byte, can't use NotSame (requires pointers).
	// Deep copy verification for IP slices is done below by modifying clone and checking original.

	// Modify clone and verify original is unchanged
	clone.Attempts = 10
	clone.IsBad = false
	clone.Addr.Port = 9999

	// ParseIP returns IPv4-mapped IPv6, so IPv4 bytes are at index 12-15
	ipv4Offset := len(clone.Addr.IP) - 4
	clone.Addr.IP[ipv4Offset] = 10  // Change 192 to 10
	clone.Source.IP[ipv4Offset] = 10

	assert.Equal(t, int32(5), original.Attempts)
	assert.True(t, original.IsBad)
	assert.Equal(t, uint16(37817), original.Addr.Port)
	assert.Equal(t, byte(192), original.Addr.IP[ipv4Offset])
	assert.Equal(t, byte(192), original.Source.IP[ipv4Offset])
}

func TestKnownAddressCloneNil(t *testing.T) {
	var nilAddr *KnownAddress
	clone := nilAddr.Clone()
	assert.Nil(t, clone)
}

func TestAddressPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Create discovery and add addresses
	pd1 := NewPeerDiscovery(DiscoveryConfig{Logger: logger, Network: "testnet", MaxPeers: 100, DataDir: tmpDir, DNSSeedEnabled: true})

	addr1 := createTestAddress("192.168.1.100:37817")
	addr2 := createTestAddress("192.168.1.101:37817")

	known1 := &KnownAddress{
		Addr:     addr1,
		LastSeen: time.Now(),
		Services: SFNodeNetwork,
	}
	known2 := &KnownAddress{
		Addr:     addr2,
		LastSeen: time.Now(),
		Services: SFNodeNetwork,
	}

	pd1.AddAddress(known1, nil, SourceSeed)
	pd1.AddAddress(known2, nil, SourceDNS)
	pd1.MarkSuccess(addr1)

	// Save addresses
	pd1.addrMu.RLock()
	addresses := make(map[string]*KnownAddress, len(pd1.addresses))
	for k, v := range pd1.addresses {
		addresses[k] = v
	}
	pd1.addrMu.RUnlock()

	err := pd1.addrDB.Save(addresses)
	require.NoError(t, err)

	// Create new discovery and load
	pd2 := NewPeerDiscovery(DiscoveryConfig{Logger: logger, Network: "testnet", MaxPeers: 100, DataDir: tmpDir, DNSSeedEnabled: true})

	// Verify addresses loaded
	pd2.addrMu.RLock()
	defer pd2.addrMu.RUnlock()

	assert.Contains(t, pd2.addresses, addr1.String())
	assert.Contains(t, pd2.addresses, addr2.String())

	// Verify addr1 is in tried bucket (had success)
	assert.Contains(t, pd2.tried, addr1.String())
	assert.False(t, pd2.addresses[addr1.String()].LastSuccess.IsZero())

	// Verify addr2 is in new bucket (no success)
	assert.Contains(t, pd2.new, addr2.String())
}

func TestNetworkGroupFiltering(t *testing.T) {
	// Test /16 grouping for IPv4
	addr1 := net.ParseIP("192.168.1.1")
	addr2 := net.ParseIP("192.168.1.2")
	addr3 := net.ParseIP("10.0.1.1")
	addr4 := net.ParseIP("172.16.1.1")

	group1 := getNetworkGroup(addr1)
	group2 := getNetworkGroup(addr2)
	group3 := getNetworkGroup(addr3)
	group4 := getNetworkGroup(addr4)

	// Same /16 network
	assert.Equal(t, group1, group2, "192.168.1.1 and 192.168.1.2 should be in same /16")

	// Different /16 networks
	assert.NotEqual(t, group1, group3, "192.168.x.x and 10.0.x.x should be in different /16")
	assert.NotEqual(t, group1, group4, "192.168.x.x and 172.16.x.x should be in different /16")
	assert.NotEqual(t, group3, group4, "10.0.x.x and 172.16.x.x should be in different /16")

	// Verify format
	assert.Equal(t, "192.168.0.0/16", group1)
	assert.Equal(t, "192.168.0.0/16", group2)
	assert.Equal(t, "10.0.0.0/16", group3)
	assert.Equal(t, "172.16.0.0/16", group4)
}

func TestPriorityCalculation(t *testing.T) {
	pd := setupTestDiscovery(t)

	// Recent successful connection should have high priority
	known1 := &KnownAddress{
		Addr:        createTestAddress("192.168.1.100:37817"),
		LastSuccess: time.Now().Add(-1 * time.Hour),
		LastSeen:    time.Now(),
		Services:    SFNodeNetwork,
		Attempts:    1,
	}

	// Old address with no success should have low priority
	known2 := &KnownAddress{
		Addr:     createTestAddress("192.168.1.101:37817"),
		LastSeen: time.Now().Add(-30 * 24 * time.Hour), // 30 days old
		Services: SFNodeNetwork,
		Attempts: 5,
	}

	// Masternode should have higher priority
	known3 := &KnownAddress{
		Addr:        createTestAddress("192.168.1.102:37817"),
		LastSuccess: time.Now().Add(-1 * time.Hour),
		LastSeen:    time.Now(),
		Services:    SFNodeNetwork | SFNodeMasternode,
		Attempts:    1,
	}

	priority1 := pd.calculateAddressPriority(known1)
	priority2 := pd.calculateAddressPriority(known2)
	priority3 := pd.calculateAddressPriority(known3)

	assert.Greater(t, priority1, priority2, "Recent successful address should have higher priority than old address")
	assert.Greater(t, priority3, priority1, "Masternode should have higher priority than regular node")
}

func TestCleanupAddresses(t *testing.T) {
	pd := setupTestDiscovery(t)

	// Add old address (should be cleaned up)
	oldAddr := createTestAddress("192.168.1.100:37817")
	known1 := &KnownAddress{
		Addr:     oldAddr,
		LastSeen: time.Now().Add(-8 * 24 * time.Hour), // 8 days old (> AddressTimeout)
		Services: SFNodeNetwork,
	}
	pd.AddAddress(known1, nil, SourceDNS)

	// Add bad address with many failures (should be cleaned up)
	badAddr := createTestAddress("192.168.1.101:37817")
	known2 := &KnownAddress{
		Addr:     badAddr,
		LastSeen: time.Now(),
		Services: SFNodeNetwork,
		Attempts: 15, // > 10 attempts
	}
	pd.AddAddress(known2, nil, SourceDNS)

	// Add recent successful address (should NOT be cleaned up)
	goodAddr := createTestAddress("192.168.1.102:37817")
	known3 := &KnownAddress{
		Addr:        goodAddr,
		LastSeen:    time.Now(),
		LastSuccess: time.Now().Add(-1 * time.Hour),
		Services:    SFNodeNetwork,
		Attempts:    2,
	}
	pd.AddAddress(known3, nil, SourceSeed)

	// Add permanent address (should NOT be cleaned up even if old)
	permAddr := createTestAddress("192.168.1.103:37817")
	known4 := &KnownAddress{
		Addr:      permAddr,
		LastSeen:  time.Now().Add(-30 * 24 * time.Hour), // 30 days old
		Services:  SFNodeNetwork,
		Permanent: true,
	}
	pd.AddAddress(known4, nil, SourceConfig)

	initialCount := len(pd.addresses)

	// Run cleanup
	pd.cleanupAddresses()

	// Verify cleanup results
	pd.addrMu.RLock()
	defer pd.addrMu.RUnlock()

	assert.NotContains(t, pd.addresses, oldAddr.String(), "Old address should be removed")
	assert.NotContains(t, pd.addresses, badAddr.String(), "Bad address with many failures should be removed")
	assert.Contains(t, pd.addresses, goodAddr.String(), "Recent successful address should remain")
	assert.Contains(t, pd.addresses, permAddr.String(), "Permanent address should remain even if old")

	assert.Less(t, len(pd.addresses), initialCount, "Cleanup should have removed some addresses")
}

func TestAddAddress(t *testing.T) {
	pd := setupTestDiscovery(t)

	addr := createTestAddress("192.168.1.100:37817")
	known := &KnownAddress{
		Addr:     addr,
		LastSeen: time.Now(),
		Services: SFNodeNetwork,
	}

	// Add address
	added := pd.AddAddress(known, nil, SourceDNS)
	assert.True(t, added, "First add should succeed")

	// Verify address was added
	pd.addrMu.RLock()
	storedAddr, exists := pd.addresses[addr.String()]
	_, inNew := pd.new[addr.String()]
	pd.addrMu.RUnlock()

	assert.True(t, exists, "Address should exist in addresses map")
	assert.True(t, inNew, "New address should be in new bucket")
	assert.NotNil(t, storedAddr)

	// Try to add same address again (should update, not add)
	added = pd.AddAddress(known, nil, SourcePeer)
	assert.False(t, added, "Adding duplicate address should return false")
}

func TestGetAddresses(t *testing.T) {
	pd := setupTestDiscovery(t)

	// Add multiple addresses
	for i := 1; i <= 10; i++ {
		addr := createTestAddress(fmt.Sprintf("192.168.1.%d:37817", i))
		known := &KnownAddress{
			Addr:     addr,
			LastSeen: time.Now(),
			Services: SFNodeNetwork,
		}
		pd.AddAddress(known, nil, SourceDNS)
	}

	// Get addresses
	addresses := pd.GetAddresses(5)

	assert.Len(t, addresses, 5, "Should return requested number of addresses")

	// Verify no duplicates
	seen := make(map[string]bool)
	for _, addr := range addresses {
		addrStr := addr.String()
		assert.False(t, seen[addrStr], "Should not return duplicate addresses")
		seen[addrStr] = true
	}
}

func TestAddressDBCorruption(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Write corrupted JSON file
	dbPath := filepath.Join(tmpDir, "peers.json")
	err := os.WriteFile(dbPath, []byte("corrupted json{{{"), 0644)
	require.NoError(t, err)

	// Create discovery - should handle corruption gracefully
	pd := NewPeerDiscovery(DiscoveryConfig{Logger: logger, Network: "testnet", MaxPeers: 100, DataDir: tmpDir, DNSSeedEnabled: true})

	// Should start with empty address book
	pd.addrMu.RLock()
	count := len(pd.addresses)
	pd.addrMu.RUnlock()

	assert.Equal(t, 0, count, "Should start with empty addresses after load failure")
}

func TestWeightedSelection(t *testing.T) {
	pd := setupTestDiscovery(t)

	// Create pool with different quality addresses
	pool := make(map[string]*KnownAddress)

	// Recent successful address
	addr1 := createTestAddress("192.168.1.100:37817")
	pool[addr1.String()] = &KnownAddress{
		Addr:        addr1,
		LastSuccess: time.Now().Add(-1 * time.Hour),
		LastSeen:    time.Now(),
		Services:    SFNodeNetwork,
		Attempts:    1,
	}

	// Address with many failures
	addr2 := createTestAddress("192.168.1.101:37817")
	pool[addr2.String()] = &KnownAddress{
		Addr:     addr2,
		LastSeen: time.Now(),
		Services: SFNodeNetwork,
		Attempts: 8,
	}

	// Masternode address
	addr3 := createTestAddress("192.168.1.102:37817")
	pool[addr3.String()] = &KnownAddress{
		Addr:        addr3,
		LastSuccess: time.Now().Add(-30 * time.Minute),
		LastSeen:    time.Now(),
		Services:    SFNodeNetwork | SFNodeMasternode,
		Attempts:    1,
	}

	// Run selection many times and track results (large sample for statistical reliability)
	selections := make(map[string]int)
	for i := 0; i < 10000; i++ {
		selected := pd.selectWeighted(pool)
		if selected != nil {
			selections[selected.Addr.String()]++
		}
	}

	// Masternode should be selected most often (5x multiplier)
	// Address with failures should be selected least often
	assert.Greater(t, selections[addr3.String()], selections[addr1.String()],
		"Masternode should be selected more often than regular node")
	assert.Greater(t, selections[addr1.String()], selections[addr2.String()],
		"Recent successful address should be selected more than failed address")
}

func TestCalculateChance(t *testing.T) {
	pd := setupTestDiscovery(t)

	// Test recent attempt penalty
	recentAttempt := &KnownAddress{
		Addr:        createTestAddress("192.168.1.100:37817"),
		LastAttempt: time.Now().Add(-5 * time.Minute), // 5 minutes ago
		LastSeen:    time.Now(),
		Services:    SFNodeNetwork,
		Attempts:    0,
	}
	recentChance := pd.calculateChance(recentAttempt)
	assert.Less(t, recentChance, 0.05, "Recent attempt should have very low chance")

	// Test failure penalty
	manyFailures := &KnownAddress{
		Addr:        createTestAddress("192.168.1.101:37817"),
		LastAttempt: time.Now().Add(-1 * time.Hour),
		LastSeen:    time.Now(),
		Services:    SFNodeNetwork,
		Attempts:    5,
	}
	failureChance := pd.calculateChance(manyFailures)
	assert.Less(t, failureChance, 0.5, "Many failures should reduce chance significantly")

	// Test masternode boost
	masternode := &KnownAddress{
		Addr:        createTestAddress("192.168.1.102:37817"),
		LastAttempt: time.Now().Add(-1 * time.Hour),
		LastSeen:    time.Now(),
		Services:    SFNodeNetwork | SFNodeMasternode,
		Attempts:    0,
	}
	masternodeChance := pd.calculateChance(masternode)

	regularNode := &KnownAddress{
		Addr:        createTestAddress("192.168.1.103:37817"),
		LastAttempt: time.Now().Add(-1 * time.Hour),
		LastSeen:    time.Now(),
		Services:    SFNodeNetwork,
		Attempts:    0,
	}
	regularChance := pd.calculateChance(regularNode)

	assert.Greater(t, masternodeChance, regularChance*4.0,
		"Masternode should have at least 4x higher chance than regular node")
}
