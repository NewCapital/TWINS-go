package p2p

import (
	"net"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPeers_NoProtocolFallback(t *testing.T) {
	logger := logrus.New()
	server := &Server{
		logger: logger.WithField("test", "no-protocol-fallback"),
	}

	peer := NewPeer(NewMockConn(), true, MagicToBytes(MainNetMagic), logger)
	peer.connected.Store(true)
	server.peers.Store(peer.GetAddress().String(), peer)

	peers := server.GetPeers()
	require.Len(t, peers, 1)
	assert.Equal(t, int32(0), peers[0].ProtocolVersion)
}

func TestAddPeer_HandshakeTimeoutQueuesDisconnect(t *testing.T) {
	logger := logrus.New()
	server := &Server{
		logger:       logger.WithField("test", "handshake-timeout"),
		donePeers:    make(chan *Peer, 1),
		quit:         make(chan struct{}),
		handshakeTTL: 25 * time.Millisecond,
	}

	peer := NewPeer(NewMockConn(), true, MagicToBytes(MainNetMagic), logger)
	peer.connected.Store(true)

	server.addPeer(peer)

	select {
	case disconnected := <-server.donePeers:
		assert.Same(t, peer, disconnected)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected peer to be queued for disconnect on handshake timeout")
	}

	close(server.quit)
}

func TestReadLoop_VersionRoutedToHandshakeChan(t *testing.T) {
	logger := logrus.New()
	server := &Server{
		msgChan:       make(chan *PeerMessage, 1),
		handshakeChan: make(chan *PeerMessage, 10),
		addrChan:      make(chan *PeerMessage, 1),
		donePeers:     make(chan *Peer, 1),
		quit:          make(chan struct{}),
	}

	// Fill msgChan to verify version does NOT go there.
	server.msgChan <- &PeerMessage{
		Peer:    nil,
		Message: NewMessage(MsgPing, make([]byte, 8), MagicToBytes(MainNetMagic)),
	}

	mockConn := NewMockConn()
	peer := NewPeer(mockConn, true, MagicToBytes(MainNetMagic), logger)

	versionPayload, err := SerializeVersionMessage(&VersionMessage{
		Version:     ProtocolVersion,
		Services:    SFNodeNetwork,
		Timestamp:   time.Now().Unix(),
		AddrRecv:    NetAddress{IP: net.ParseIP("127.0.0.1"), Port: 8333},
		AddrFrom:    NetAddress{IP: net.ParseIP("127.0.0.1"), Port: 8333},
		Nonce:       1,
		UserAgent:   "/test/",
		StartHeight: 1,
		Relay:       true,
	})
	require.NoError(t, err)

	versionMsg := NewMessage(MsgVersion, versionPayload, MagicToBytes(MainNetMagic))
	wire, err := versionMsg.Serialize()
	require.NoError(t, err)
	mockConn.SetReadData(wire)

	peer.wg.Add(1)
	go peer.readLoop(server)

	// Version should arrive on handshakeChan, NOT msgChan.
	select {
	case queued := <-server.handshakeChan:
		require.NotNil(t, queued)
		require.NotNil(t, queued.Message)
		assert.Equal(t, string(MsgVersion), queued.Message.GetCommand())
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected version message on handshakeChan")
	}

	// msgChan should still have only the prefilled ping message.
	select {
	case msg := <-server.msgChan:
		assert.Equal(t, string(MsgPing), msg.Message.GetCommand())
	default:
		t.Fatal("expected prefilled ping in msgChan")
	}

	peer.Stop()
	close(server.quit)
}
