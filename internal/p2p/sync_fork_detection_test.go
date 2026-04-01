package p2p

import (
	"net"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/twins-dev/twins-core/pkg/types"
)

// TestSubnetKey tests the /24 subnet key extraction
func TestSubnetKey(t *testing.T) {
	tests := []struct {
		name     string
		ip       net.IP
		expected string
	}{
		{"IPv4 standard", net.ParseIP("192.168.1.100"), "192.168.1.0"},
		{"IPv4 different host", net.ParseIP("192.168.1.200"), "192.168.1.0"},
		{"IPv4 different subnet", net.ParseIP("10.0.0.1"), "10.0.0.0"},
		{"IPv6 mapped v4", net.ParseIP("::ffff:192.168.1.1"), "192.168.1.0"},
		{"IPv6 pure", net.ParseIP("2001:db8::1"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := subnetKey(tt.ip)
			if result != tt.expected {
				t.Errorf("subnetKey(%v) = %q, want %q", tt.ip, result, tt.expected)
			}
		})
	}
}

// TestAnalyzeForkResults_AllAgree tests when all peers agree with us
func TestAnalyzeForkResults_AllAgree(t *testing.T) {
	bs := &BlockchainSyncer{
		logger: testLogger(),
	}

	ourHeight := uint32(1000)
	ourTip := types.Hash{0xAA}

	results := []forkCheckResult{
		{peerAddr: "peer1", tipHeight: 1000, tipHash: ourTip},
		{peerAddr: "peer2", tipHeight: 1000, tipHash: ourTip},
		{peerAddr: "peer3", tipHeight: 1000, tipHash: ourTip},
	}

	// Should not panic or trigger recovery — all agree
	bs.analyzeForkResults(results, ourHeight, ourTip)
}

// TestAnalyzeForkResults_MinorityDisagree tests when minority disagrees (no fork)
func TestAnalyzeForkResults_MinorityDisagree(t *testing.T) {
	bs := &BlockchainSyncer{
		logger: testLogger(),
	}

	ourHeight := uint32(1000)
	ourTip := types.Hash{0xAA}
	otherTip := types.Hash{0xBB}

	results := []forkCheckResult{
		{peerAddr: "peer1", tipHeight: 1000, tipHash: ourTip},
		{peerAddr: "peer2", tipHeight: 1000, tipHash: ourTip},
		{peerAddr: "peer3", tipHeight: 1000, tipHash: otherTip},
		{peerAddr: "peer4", tipHeight: 1000, tipHash: ourTip},
	}

	// 1 out of 4 disagrees — not a majority, no recovery
	bs.analyzeForkResults(results, ourHeight, ourTip)
}

// TestAnalyzeForkResults_PeerAheadSameChain tests peers ahead on same chain.
// With nil blockchain, locatorSharesChain returns false (can't verify blocks),
// so this tests that the code doesn't panic and handles nil safely.
// Full integration testing of locatorSharesChain requires a real blockchain.
func TestAnalyzeForkResults_PeerAheadSameChain(t *testing.T) {
	bs := &BlockchainSyncer{
		logger: testLogger(),
		// blockchain is nil — locatorSharesChain will return false
	}

	ourHeight := uint32(1000)
	ourTip := types.Hash{0xAA}

	// Peers at same height agreeing with us — no fork regardless of blockchain
	results := []forkCheckResult{
		{peerAddr: "peer1", tipHeight: 1000, tipHash: ourTip},
		{peerAddr: "peer2", tipHeight: 1000, tipHash: ourTip},
		{peerAddr: "peer3", tipHeight: 1000, tipHash: ourTip},
	}

	// Should not panic — all agree at same height
	bs.analyzeForkResults(results, ourHeight, ourTip)
}

// TestLocatorSharesChain_NilBlockchain tests nil safety
func TestLocatorSharesChain_NilBlockchain(t *testing.T) {
	bs := &BlockchainSyncer{}
	locator := []types.Hash{{0x01}, {0x02}}
	if bs.locatorSharesChain(locator) {
		t.Error("expected false with nil blockchain")
	}
	if bs.locatorSharesChain(nil) {
		t.Error("expected false with nil locator")
	}
}

// TestAnalyzeForkResults_InsufficientVoters tests not enough peers at our height
func TestAnalyzeForkResults_InsufficientVoters(t *testing.T) {
	bs := &BlockchainSyncer{
		logger: testLogger(),
	}

	ourHeight := uint32(1000)
	ourTip := types.Hash{0xAA}

	// All peers are below our height — not enough voters
	results := []forkCheckResult{
		{peerAddr: "peer1", tipHeight: 990},
		{peerAddr: "peer2", tipHeight: 995},
		{peerAddr: "peer3", tipHeight: 998},
	}

	bs.analyzeForkResults(results, ourHeight, ourTip)
}

// TestSubnetDiversityLimit tests that candidates are limited per /24 subnet
func TestSubnetDiversityLimit(t *testing.T) {
	counts := make(map[string]int)

	// Simulate 5 peers from same /24
	ips := []net.IP{
		net.ParseIP("10.0.1.1"),
		net.ParseIP("10.0.1.2"),
		net.ParseIP("10.0.1.3"),
		net.ParseIP("10.0.1.4"),
		net.ParseIP("10.0.1.5"),
	}

	accepted := 0
	for _, ip := range ips {
		subnet := subnetKey(ip)
		if counts[subnet] >= forkSubnetMaxPerSubnet {
			continue
		}
		counts[subnet]++
		accepted++
	}

	if accepted != forkSubnetMaxPerSubnet {
		t.Errorf("expected %d peers accepted from same subnet, got %d", forkSubnetMaxPerSubnet, accepted)
	}
}

// testLogger creates a minimal logger for tests
func testLogger() *logrus.Entry {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	return logger.WithField("test", "fork_detection")
}
