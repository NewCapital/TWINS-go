package p2p

import (
	"encoding/binary"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockConn implements net.Conn for testing
type MockConn struct {
	readData      []byte
	writeData     []byte
	readIndex     int
	closed        bool
	readDeadline  time.Time
	writeDeadline time.Time
	mu            sync.Mutex
}

func NewMockConn() *MockConn {
	return &MockConn{
		readData:  make([]byte, 0),
		writeData: make([]byte, 0),
	}
}

func (mc *MockConn) Read(b []byte) (n int, err error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.closed {
		return 0, net.ErrClosed
	}

	available := len(mc.readData) - mc.readIndex
	if available == 0 {
		return 0, nil // No data available
	}

	n = copy(b, mc.readData[mc.readIndex:])
	mc.readIndex += n
	return n, nil
}

func (mc *MockConn) Write(b []byte) (n int, err error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.closed {
		return 0, net.ErrClosed
	}

	mc.writeData = append(mc.writeData, b...)
	return len(b), nil
}

func (mc *MockConn) Close() error {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.closed = true
	return nil
}

func (mc *MockConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 18333}
}

func (mc *MockConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("192.168.1.100"), Port: 18333}
}

func (mc *MockConn) SetDeadline(t time.Time) error {
	return nil
}

func (mc *MockConn) SetReadDeadline(t time.Time) error {
	mc.readDeadline = t
	return nil
}

func (mc *MockConn) SetWriteDeadline(t time.Time) error {
	mc.writeDeadline = t
	return nil
}

func (mc *MockConn) SetReadData(data []byte) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.readData = data
	mc.readIndex = 0
}

func (mc *MockConn) GetWrittenData() []byte {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	result := make([]byte, len(mc.writeData))
	copy(result, mc.writeData)
	return result
}

func TestNewPeer(t *testing.T) {
	conn := NewMockConn()
	logger := logrus.New()

	peer := NewPeer(conn, true, MagicToBytes(MainNetMagic), logger)
	require.NotNil(t, peer)

	assert.True(t, peer.inbound)
	assert.False(t, peer.IsConnected())
	assert.False(t, peer.IsHandshakeComplete())
	assert.NotNil(t, peer.GetAddress())
}

func TestPeerConnect(t *testing.T) {
	// This test would require a real network connection
	// For now, test the error cases
	_, err := Connect("invalid-address", MagicToBytes(MainNetMagic), logrus.New())
	assert.Error(t, err)

	_, err = Connect("127.0.0.1:99999", MagicToBytes(MainNetMagic), logrus.New())
	assert.Error(t, err) // Should fail to connect
}

func TestPeerSendMessage(t *testing.T) {
	conn := NewMockConn()
	logger := logrus.New()
	peer := NewPeer(conn, false, MagicToBytes(MainNetMagic), logger)

	// SendMessage should work on a new peer (pre-handshake messages like version/verack
	// must be sendable before connected flag is set).
	msg := NewMessage(MsgPing, []byte{1, 2, 3, 4, 5, 6, 7, 8}, MagicToBytes(MainNetMagic))
	err := peer.SendMessage(msg)
	assert.NoError(t, err)

	// After stopping, SendMessage should fail
	peer.Stop()
	err = peer.SendMessage(msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "shutting down")
}

func TestPeerSendMessageSync(t *testing.T) {
	conn := NewMockConn()
	logger := logrus.New()
	peer := NewPeer(conn, false, MagicToBytes(MainNetMagic), logger)

	// After stopping, sync message should fail
	peer.Stop()
	msg := NewMessage(MsgPing, []byte{1, 2, 3, 4, 5, 6, 7, 8}, MagicToBytes(MainNetMagic))
	err := peer.SendMessageSync(msg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "shutting down")
}

func TestPeerHandshake(t *testing.T) {
	conn := NewMockConn()
	logger := logrus.New()
	peer := NewPeer(conn, true, MagicToBytes(MainNetMagic), logger)

	// Create a version message
	version := &VersionMessage{
		Version:     ProtocolVersion,
		Services:    SFNodeNetwork,
		Timestamp:   time.Now().Unix(),
		UserAgent:   "/TWINS-Go:1.0.0/",
		StartHeight: 12345,
		Relay:       true,
	}

	// Test handshake completion
	assert.False(t, peer.IsHandshakeComplete())
	peer.SetHandshakeComplete(version)
	assert.True(t, peer.IsHandshakeComplete())

	// Test version retrieval
	retrievedVersion := peer.GetVersion()
	require.NotNil(t, retrievedVersion)
	assert.Equal(t, version.Version, retrievedVersion.Version)
	assert.Equal(t, version.Services, retrievedVersion.Services)
	assert.Equal(t, version.UserAgent, retrievedVersion.UserAgent)
	assert.Equal(t, version.StartHeight, retrievedVersion.StartHeight)
}

func TestPeerPingPong(t *testing.T) {
	conn := NewMockConn()
	logger := logrus.New()
	peer := NewPeer(conn, false, MagicToBytes(MainNetMagic), logger)

	// Test pong handling
	nonce := uint64(0x123456789abcdef0)
	peer.pingNonce.Store(nonce)

	err := peer.HandlePong(nonce, 0)
	assert.NoError(t, err)

	// Check that last pong time was updated
	lastPong := peer.lastPong.Load()
	assert.True(t, lastPong > 0)

	// Test wrong nonce
	err = peer.HandlePong(nonce+1, 0)
	assert.NoError(t, err) // Still no error, but warns in logs

	// Test Protocol 70928: pong with height updates peerHeight
	err = peer.HandlePong(nonce, 500000)
	assert.NoError(t, err)
	assert.Equal(t, uint32(500000), peer.GetPeerHeight())
}

func TestPeerStats(t *testing.T) {
	conn := NewMockConn()
	logger := logrus.New()
	peer := NewPeer(conn, true, MagicToBytes(MainNetMagic), logger)

	// Set up version
	version := &VersionMessage{
		Version:     ProtocolVersion,
		Services:    SFNodeNetwork | SFNodeBloom,
		UserAgent:   "/TWINS-Go:1.0.0/",
		StartHeight: 54321,
	}
	peer.SetHandshakeComplete(version)

	// Update some statistics
	peer.bytesReceived.Store(1024)
	peer.bytesSent.Store(2048)
	peer.messageCount.Store(10)

	stats := peer.GetStats()
	require.NotNil(t, stats)

	assert.True(t, peer.inbound == stats.Inbound)
	assert.Equal(t, version.Services, stats.Services)
	assert.Equal(t, version.UserAgent, stats.UserAgent)
	assert.Equal(t, version.Version, stats.ProtocolVersion)
	assert.Equal(t, version.StartHeight, stats.StartHeight)
	assert.Equal(t, uint64(1024), stats.BytesReceived)
	assert.Equal(t, uint64(2048), stats.BytesSent)
	assert.Equal(t, uint32(10), stats.MessagesSent)
}

func TestPeerPersistent(t *testing.T) {
	conn := NewMockConn()
	logger := logrus.New()
	peer := NewPeer(conn, false, MagicToBytes(MainNetMagic), logger)

	assert.False(t, peer.IsPersistent())

	peer.SetPersistent(true)
	assert.True(t, peer.IsPersistent())

	peer.SetPersistent(false)
	assert.False(t, peer.IsPersistent())
}

func TestPeerString(t *testing.T) {
	conn := NewMockConn()
	logger := logrus.New()

	// Test inbound peer
	inboundPeer := NewPeer(conn, true, MagicToBytes(MainNetMagic), logger)
	str := inboundPeer.String()
	assert.Contains(t, str, "inbound")
	assert.Contains(t, str, "192.168.1.100:18333")

	// Test outbound peer
	outboundPeer := NewPeer(conn, false, MagicToBytes(MainNetMagic), logger)
	str = outboundPeer.String()
	assert.Contains(t, str, "outbound")
	assert.Contains(t, str, "192.168.1.100:18333")
}

func TestPeerServices(t *testing.T) {
	conn := NewMockConn()
	logger := logrus.New()
	peer := NewPeer(conn, false, MagicToBytes(MainNetMagic), logger)

	// Initially no services
	services := peer.GetServices()
	assert.Equal(t, ServiceFlag(0), services)

	// Set services through version
	version := &VersionMessage{
		Services: SFNodeNetwork | SFNodeMasternode,
	}
	peer.SetHandshakeComplete(version)

	services = peer.GetServices()
	assert.Equal(t, SFNodeNetwork|SFNodeMasternode, services)
}

func TestPeerMessageSerialization(t *testing.T) {
	// Test ping message serialization
	ping := &PingMessage{Nonce: 0x123456789abcdef0}

	conn := NewMockConn()
	logger := logrus.New()
	peer := NewPeer(conn, false, MagicToBytes(MainNetMagic), logger)

	payload, err := peer.serializePing(ping)
	require.NoError(t, err)
	assert.Equal(t, 8, len(payload))

	// Verify the nonce is correctly encoded
	nonce := uint64(payload[0]) |
		uint64(payload[1])<<8 |
		uint64(payload[2])<<16 |
		uint64(payload[3])<<24 |
		uint64(payload[4])<<32 |
		uint64(payload[5])<<40 |
		uint64(payload[6])<<48 |
		uint64(payload[7])<<56

	assert.Equal(t, ping.Nonce, nonce)
}

func TestPeerMagic(t *testing.T) {
	conn := NewMockConn()
	logger := logrus.New()
	peer := NewPeer(conn, false, MagicToBytes(MainNetMagic), logger)

	magic := peer.getMagic()
	// Convert [4]byte back to uint32 for comparison
	magicUint32 := binary.LittleEndian.Uint32(magic[:])
	assert.Equal(t, uint32(MainNetMagic), magicUint32)
}

// Test concurrent access to peer
func TestPeerConcurrentAccess(t *testing.T) {
	conn := NewMockConn()
	logger := logrus.New()
	peer := NewPeer(conn, false, MagicToBytes(MainNetMagic), logger)

	var wg sync.WaitGroup
	numGoroutines := 100

	// Test concurrent stats access
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			stats := peer.GetStats()
			assert.NotNil(t, stats)
		}()
	}
	wg.Wait()

	// Test concurrent service access
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			services := peer.GetServices()
			_ = services // Just access it
		}()
	}
	wg.Wait()

	// Test concurrent version access
	version := &VersionMessage{
		Version:   ProtocolVersion,
		Services:  SFNodeNetwork,
		UserAgent: "/TWINS-Go:1.0.0/",
	}
	peer.SetHandshakeComplete(version)

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			v := peer.GetVersion()
			if v != nil {
				assert.Equal(t, int32(ProtocolVersion), v.Version)
			}
		}()
	}
	wg.Wait()
}

func BenchmarkPeerStats(b *testing.B) {
	conn := NewMockConn()
	logger := logrus.New()
	peer := NewPeer(conn, true, MagicToBytes(MainNetMagic), logger)

	version := &VersionMessage{
		Version:     ProtocolVersion,
		Services:    SFNodeNetwork,
		UserAgent:   "/TWINS-Go:1.0.0/",
		StartHeight: 12345,
	}
	peer.SetHandshakeComplete(version)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stats := peer.GetStats()
		_ = stats
	}
}

func BenchmarkPeerSendMessage(b *testing.B) {
	conn := NewMockConn()
	logger := logrus.New()
	peer := NewPeer(conn, false, MagicToBytes(MainNetMagic), logger)

	// Set as connected for benchmark
	peer.connected.Store(true)

	msg := NewMessage(MsgPing, []byte{1, 2, 3, 4, 5, 6, 7, 8}, MagicToBytes(MainNetMagic))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := peer.SendMessage(msg)
		if err != nil {
			// Queue might be full, that's ok for benchmark
			continue
		}
	}
}

func BenchmarkPingPongHandling(b *testing.B) {
	conn := NewMockConn()
	logger := logrus.New()
	peer := NewPeer(conn, false, MagicToBytes(MainNetMagic), logger)

	nonce := uint64(0x123456789abcdef0)
	peer.pingNonce.Store(nonce)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := peer.HandlePong(nonce, 0)
		if err != nil {
			b.Fatal(err)
		}
	}
}
