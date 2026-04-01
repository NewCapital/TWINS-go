package p2p

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/twins-dev/twins-core/internal/config"
	"github.com/twins-dev/twins-core/pkg/types"
)

func testChainParams() *types.ChainParams {
	return &types.ChainParams{
		Name:          "testnet",
		DefaultPort:   37819,
		NetMagicBytes: [4]byte{0xe5, 0xba, 0xc5, 0xb6},
	}
}

func TestNewServer_MaxInboundFromMaxPeers(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	cfg := config.DefaultConfig()
	cfg.Network.MaxPeers = 50
	cfg.Network.MaxOutboundPeers = 10

	s := NewServer(cfg, testChainParams(), logger)

	// maxInbound = MaxPeers - MaxOutboundPeers = 50 - 10 = 40
	assert.Equal(t, int32(40), s.maxInbound)
	assert.Equal(t, int32(10), s.maxOutbound)
}

func TestNewServer_MaxInboundDefault(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	cfg := config.DefaultConfig()
	// MaxPeers defaults to 125, MaxOutboundPeers defaults to 16

	s := NewServer(cfg, testChainParams(), logger)

	assert.Equal(t, int32(125-16), s.maxInbound) // 109
	assert.Equal(t, int32(16), s.maxOutbound)
}

func TestNewServer_MaxInboundFloorZero(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	cfg := config.DefaultConfig()
	cfg.Network.MaxPeers = 5
	cfg.Network.MaxOutboundPeers = 10 // More outbound than total

	s := NewServer(cfg, testChainParams(), logger)

	// Outbound capped to maxPeers (legacy: min(MAX_OUTBOUND, nMaxConnections), net.cpp:1722)
	// maxOutbound = min(10, 5) = 5, maxInbound = 5 - 5 = 0
	assert.Equal(t, int32(0), s.maxInbound)
	assert.Equal(t, int32(5), s.maxOutbound)
}

func TestNewServer_MaxPeersCapsOutbound(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	cfg := config.DefaultConfig()
	cfg.Network.MaxPeers = 5
	// MaxOutboundPeers defaults to 16

	s := NewServer(cfg, testChainParams(), logger)

	// maxOutbound capped: min(16, 5) = 5, maxInbound = 5 - 5 = 0
	assert.Equal(t, int32(5), s.maxOutbound)
	assert.Equal(t, int32(0), s.maxInbound)
}

func TestServer_GetDialTimeout(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	cfg := config.DefaultConfig()
	cfg.Network.Timeout = 15 // 15 seconds

	s := NewServer(cfg, testChainParams(), logger)

	assert.Equal(t, 15*time.Second, s.getDialTimeout())
}

func TestServer_GetDialTimeoutDefault(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	cfg := config.DefaultConfig()
	cfg.Network.Timeout = 0 // Not set

	s := NewServer(cfg, testChainParams(), logger)

	assert.Equal(t, 30*time.Second, s.getDialTimeout())
}

func TestServer_GetPingInterval(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	cfg := config.DefaultConfig()
	cfg.Network.KeepAlive = 60 // 60 seconds

	s := NewServer(cfg, testChainParams(), logger)

	assert.Equal(t, 60*time.Second, s.getPingInterval())
}

func TestServer_GetPingIntervalDefault(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	cfg := config.DefaultConfig()
	cfg.Network.KeepAlive = 0 // Not set

	s := NewServer(cfg, testChainParams(), logger)

	assert.Equal(t, PingInterval, s.getPingInterval())
}

func TestServer_ApplyConfigToPeer(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	cfg := config.DefaultConfig()
	cfg.Network.Timeout = 10
	cfg.Network.KeepAlive = 45

	s := NewServer(cfg, testChainParams(), logger)

	peer := &Peer{}
	s.applyConfigToPeer(peer)

	assert.Equal(t, 45*time.Second, peer.pingInterval)
	assert.Equal(t, 10*time.Second, peer.dialTimeout)
}

func TestPeer_GetPingIntervalDefault(t *testing.T) {
	peer := &Peer{} // No pingInterval set (zero value)
	assert.Equal(t, PingInterval, peer.getPingInterval())
}

func TestPeer_GetPingIntervalConfigured(t *testing.T) {
	peer := &Peer{pingInterval: 90 * time.Second}
	assert.Equal(t, 90*time.Second, peer.getPingInterval())
}

func TestPeer_GetDialTimeoutDefault(t *testing.T) {
	peer := &Peer{} // No dialTimeout set (zero value)
	assert.Equal(t, HandshakeTimeout, peer.getDialTimeout())
}

func TestPeer_GetDialTimeoutConfigured(t *testing.T) {
	peer := &Peer{dialTimeout: 20 * time.Second}
	assert.Equal(t, 20*time.Second, peer.getDialTimeout())
}

func TestGetStats_ListenPort(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	cfg := config.DefaultConfig()
	s := NewServer(cfg, testChainParams(), logger)

	// No listener yet — ListenPort should be 0
	stats := s.GetStats()
	assert.Equal(t, uint16(0), stats.ListenPort)

	// Simulate a local address with a custom port
	s.localAddr = &NetAddress{
		IP:   []byte{0, 0, 0, 0},
		Port: 38000,
	}
	stats = s.GetStats()
	assert.Equal(t, uint16(38000), stats.ListenPort)
	assert.Contains(t, stats.LocalAddress, "38000")
}

func TestNewServer_ExternalIPNotSetByDefault(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	cfg := config.DefaultConfig()
	// ExternalIP defaults to ""

	s := NewServer(cfg, testChainParams(), logger)

	assert.Nil(t, s.getExternalIP())
}
