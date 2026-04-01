package p2p

import (
	"net"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twins-dev/twins-core/pkg/types"
)

func testHash(seed byte) types.Hash {
	var h types.Hash
	h[0] = seed
	return h
}

func TestTxRelayCache_PutGetAndExpire(t *testing.T) {
	cache := newTxRelayCache()
	hash := testHash(1)
	payload := []byte{1, 2, 3, 4}
	now := time.Now()

	cache.put(hash, payload, now)

	got, ok := cache.get(hash, now.Add(2*time.Second))
	require.True(t, ok)
	assert.Equal(t, payload, got)

	_, ok = cache.get(hash, now.Add(TxRelayCacheTTL+time.Second))
	assert.False(t, ok)
}

func TestPeerTxRelayState_QueueDedupAndKnownSuppression(t *testing.T) {
	state := newPeerTxRelayState()
	h1 := testHash(1)
	h2 := testHash(2)

	assert.True(t, state.enqueue(h1))
	assert.False(t, state.enqueue(h1), "duplicate hash should not be queued twice")

	batch := state.popBatch(10)
	require.Len(t, batch, 1)
	assert.Equal(t, h1, batch[0])

	// Until explicitly marked known, hash can be queued again.
	assert.True(t, state.enqueue(h1))
	state.markBatchKnown([]types.Hash{h1})
	// Once marked known, it should be suppressed.
	assert.False(t, state.enqueue(h1))
	assert.True(t, state.enqueue(h2))
}

func TestAllowMemPoolRequest_PerPeerRateLimit(t *testing.T) {
	logger := logrus.New()
	server := &Server{
		logger: logger.WithField("test", "mempool-rate-limit"),
	}
	peer := NewPeer(NewMockConn(), true, MagicToBytes(MainNetMagic), logger)
	peer.SetHandshakeComplete(&VersionMessage{
		Version:   ProtocolVersion,
		Relay:     true,
		AddrRecv:  NetAddress{IP: net.ParseIP("127.0.0.1"), Port: 18333},
		AddrFrom:  NetAddress{IP: net.ParseIP("127.0.0.1"), Port: 18333},
		UserAgent: "/test/",
	})

	now := time.Now()
	assert.True(t, server.allowMemPoolRequest(peer, now))
	assert.False(t, server.allowMemPoolRequest(peer, now.Add(10*time.Second)))
	assert.True(t, server.allowMemPoolRequest(peer, now.Add(TxMemPoolRequestMinInterval+time.Second)))
}
