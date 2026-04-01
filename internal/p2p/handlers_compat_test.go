package p2p

import (
	"bytes"
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/twins-dev/twins-core/pkg/types"
)

func TestHandleAlertMessage(t *testing.T) {
	// Create test server
	server := &Server{
		logger: logrus.WithField("test", "alert"),
		params: types.MainnetParams(),
	}

	// Create test peer
	peer := &Peer{
		logger: logrus.NewEntry(logrus.New()),
		addr:   &NetAddress{IP: net.ParseIP("127.0.0.1"), Port: 8333},
	}

	// Create alert message
	msg := &Message{
		Command: [12]byte{'a', 'l', 'e', 'r', 't'},
		Payload: []byte("test alert"),
	}

	// Handle alert - should not crash and should log
	server.handleAlertMessage(peer, msg)

	// Alert should be ignored (no action taken)
	assert.NotNil(t, server)
}

func TestHandleFilterLoadMessage(t *testing.T) {
	server := &Server{
		logger: logrus.WithField("test", "filterload"),
		params: types.MainnetParams(),
	}

	peer := &Peer{
		logger: logrus.NewEntry(logrus.New()),
		addr:   &NetAddress{IP: net.ParseIP("127.0.0.1"), Port: 8333},
		quit:   make(chan struct{}),
	}

	// Valid filterload payload: varbytes filter + nHashFuncs + nTweak + nFlags.
	filterData := []byte{0x01, 0x02, 0x03, 0x04}
	payload := make([]byte, 0, 1+len(filterData)+9)
	payload = append(payload, byte(len(filterData)))
	payload = append(payload, filterData...)
	payload = binary.LittleEndian.AppendUint32(payload, 2)      // nHashFuncs
	payload = binary.LittleEndian.AppendUint32(payload, 0x1234) // nTweak
	payload = append(payload, 0x00)                             // nFlags

	msg := &Message{
		Command: [12]byte{'f', 'i', 'l', 't', 'e', 'r', 'l', 'o', 'a', 'd'},
		Payload: payload,
	}

	server.handleFilterLoadMessage(peer, msg)

	// Check filter was loaded
	peer.mu.RLock()
	defer peer.mu.RUnlock()
	assert.NotNil(t, peer.bloomFilter)
	assert.True(t, peer.bloomFilter.loaded)
	assert.Equal(t, filterData, peer.bloomFilter.data)
	assert.Equal(t, uint32(2), peer.bloomFilter.hashFuncs)
	assert.Equal(t, uint32(0x1234), peer.bloomFilter.tweak)
}

func TestHandleFilterAddMessage(t *testing.T) {
	server := &Server{
		logger: logrus.WithField("test", "filteradd"),
		params: types.MainnetParams(),
	}

	peer := &Peer{
		logger: logrus.NewEntry(logrus.New()),
		addr:   &NetAddress{IP: net.ParseIP("127.0.0.1"), Port: 8333},
		bloomFilter: &BloomFilter{
			data:      []byte{0x00, 0x00, 0x00, 0x00},
			loaded:    true,
			elements:  0,
			hashFuncs: 2,
			tweak:     1,
		},
	}

	elem := []byte{0x03, 0x04}
	payload := append([]byte{byte(len(elem))}, elem...)
	msg := &Message{
		Command: [12]byte{'f', 'i', 'l', 't', 'e', 'r', 'a', 'd', 'd'},
		Payload: payload,
	}

	server.handleFilterAddMessage(peer, msg)

	// Check element count increased
	peer.mu.RLock()
	defer peer.mu.RUnlock()
	assert.Equal(t, 1, peer.bloomFilter.elements)
}

func TestHandleFilterClearMessage(t *testing.T) {
	server := &Server{
		logger: logrus.WithField("test", "filterclear"),
		params: types.MainnetParams(),
	}

	peer := &Peer{
		logger: logrus.NewEntry(logrus.New()),
		addr:   &NetAddress{IP: net.ParseIP("127.0.0.1"), Port: 8333},
		bloomFilter: &BloomFilter{
			data:     []byte{0x01, 0x02},
			loaded:   true,
			elements: 5,
		},
	}

	msg := &Message{
		Command: [12]byte{'f', 'i', 'l', 't', 'e', 'r', 'c', 'l', 'e', 'a', 'r'},
		Payload: []byte{},
	}

	server.handleFilterClearMessage(peer, msg)

	// Check filter was cleared
	peer.mu.RLock()
	defer peer.mu.RUnlock()
	assert.Nil(t, peer.bloomFilter)
}

func TestHandleMemPoolMessage(t *testing.T) {
	server := &Server{
		logger: logrus.WithField("test", "mempool"),
		params: types.MainnetParams(),
	}

	// Mock peer with send capability
	peer := &Peer{
		logger:     logrus.NewEntry(logrus.New()),
		addr:       &NetAddress{IP: net.ParseIP("127.0.0.1"), Port: 8333},
		conn:       &mockConn{},
		writeQueue: make(chan outgoingMessage, 10),
		magic:      server.params.NetMagicBytes,
	}
	peer.connected.Store(true)

	drainQueue := func() []MessageType {
		var types []MessageType
		for {
			select {
			case out := <-peer.writeQueue:
				cmd := strings.TrimRight(string(out.message.Command[:]), "\x00")
				types = append(types, MessageType(cmd))
			default:
				return types
			}
		}
	}

	msg := &Message{
		Command: [12]byte{'m', 'e', 'm', 'p', 'o', 'o', 'l'},
		Payload: []byte{},
	}

	// Ensure queue empty before test
	_ = drainQueue()

	// Test without bloom filter (regular client)
	server.handleMemPoolMessage(peer, msg)
	sentMessages := drainQueue()
	assert.Contains(t, sentMessages, MsgInv)

	// Test with bloom filter (SPV client)
	_ = drainQueue()
	peer.bloomFilter = &BloomFilter{loaded: true}
	server.handleMemPoolMessage(peer, msg)
	sentMessages = drainQueue()
	assert.Contains(t, sentMessages, MsgInv)
}

func TestHandleBudgetMessages(t *testing.T) {
	server := &Server{
		logger: logrus.WithField("test", "budget"),
		params: types.MainnetParams(),
	}

	peer := &Peer{
		logger: logrus.NewEntry(logrus.New()),
		addr:   &NetAddress{IP: net.ParseIP("127.0.0.1"), Port: 8333},
	}

	// Test mnfinal message
	msg := &Message{
		Command: [12]byte{'m', 'n', 'f', 'i', 'n', 'a', 'l'},
		Payload: []byte{0x01, 0x02},
	}
	server.handleMNFinalMessage(peer, msg)

	// Test fbvote message
	msg = &Message{
		Command: [12]byte{'f', 'b', 'v', 'o', 't', 'e'},
		Payload: []byte{0x03, 0x04},
	}
	server.handleFBVoteMessage(peer, msg)

	// These should be acknowledged but not processed
	assert.NotNil(t, server)
}

func TestMatchesBloomFilter(t *testing.T) {
	server := &Server{
		logger: logrus.WithField("test", "bloom"),
	}

	// Test with no filter
	tx := &types.Transaction{
		Version: 1,
	}
	assert.True(t, server.matchesBloomFilter(tx, nil))

	// Test with unloaded filter
	filter := &BloomFilter{loaded: false}
	assert.True(t, server.matchesBloomFilter(tx, filter))

	// Test with loaded filter and no matches.
	filter = &BloomFilter{loaded: true, data: make([]byte, 8), hashFuncs: 2, tweak: 1}
	assert.False(t, server.matchesBloomFilter(tx, filter))

	// Test with loaded filter matching tx hash.
	txHash := tx.Hash()
	filter.add(txHash[:])
	assert.True(t, server.matchesBloomFilter(tx, filter))
}

func TestSendFilteredBlock(t *testing.T) {
	server := &Server{
		logger: logrus.WithField("test", "filtered"),
		params: types.MainnetParams(),
	}

	// Create test block
	block := &types.Block{
		Header: &types.BlockHeader{
			Version:   5,
			Timestamp: 1234567890,
		},
		Transactions: []*types.Transaction{
			{Version: 1},
			{Version: 1},
		},
	}

	// Mock peer
	peer := &Peer{
		logger:     logrus.NewEntry(logrus.New()),
		addr:       &NetAddress{IP: net.ParseIP("127.0.0.1"), Port: 8333},
		conn:       &mockConn{},
		writeQueue: make(chan outgoingMessage, 10),
		magic:      server.params.NetMagicBytes,
	}
	peer.connected.Store(true)

	drainQueue := func() []MessageType {
		var types []MessageType
		for {
			select {
			case out := <-peer.writeQueue:
				cmd := strings.TrimRight(string(out.message.Command[:]), "\x00")
				types = append(types, MessageType(cmd))
			default:
				return types
			}
		}
	}

	// Test without filter - should send regular block
	_ = drainQueue()
	err := server.sendFilteredBlock(peer, block)
	assert.NoError(t, err)
	sentTypes := drainQueue()
	assert.Contains(t, sentTypes, MsgBlock)

	// Test with filter - should send merkle block
	_ = drainQueue()
	peer.bloomFilter = &BloomFilter{
		loaded:    true,
		data:      make([]byte, 8),
		hashFuncs: 2,
		tweak:     1,
	}
	err = server.sendFilteredBlock(peer, block)
	assert.NoError(t, err)
	sentTypes = drainQueue()
	assert.Contains(t, sentTypes, MsgMerkleBlock)
}

// Mock connection for testing
type mockConn struct {
	bytes.Buffer
}

func (m *mockConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8333}
}

func (m *mockConn) Close() error {
	return nil
}

func (m *mockConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345}
}

func (m *mockConn) SetDeadline(t time.Time) error {
	return nil
}

func (m *mockConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *mockConn) SetWriteDeadline(t time.Time) error {
	return nil
}
