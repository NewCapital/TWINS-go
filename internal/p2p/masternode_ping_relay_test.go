package p2p

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twins-dev/twins-core/internal/masternode"
	"github.com/twins-dev/twins-core/pkg/types"
)

func TestBroadcastMasternodePing_SendsInventoryAnnouncement(t *testing.T) {
	logger := logrus.New()
	peer := NewPeer(NewMockConn(), false, MagicToBytes(MainNetMagic), logger)
	peer.connected.Store(true)
	peer.handshake.Store(true)

	server := &Server{
		params: &types.ChainParams{
			NetMagicBytes: MagicToBytes(MainNetMagic),
		},
		logger: logger.WithField("test", "mnp-inv-relay"),
	}
	server.peers.Store(peer.GetAddress().String(), peer)

	ping := &masternode.MasternodePing{
		OutPoint: types.Outpoint{
			Hash:  types.Hash{0x01, 0x02, 0x03},
			Index: 7,
		},
		BlockHash: types.Hash{0x09, 0x08, 0x07},
		SigTime:   time.Now().Unix(),
		Signature: []byte{0x11, 0x22},
	}
	wantHash := ping.GetHash()

	err := server.BroadcastMasternodePing(ping)
	require.NoError(t, err)

	select {
	case out := <-peer.writeQueue:
		require.NotNil(t, out.message)
		assert.Equal(t, string(MsgInv), out.message.GetCommand())

		payload := out.message.Payload
		require.GreaterOrEqual(t, len(payload), 37) // count(1) + type(4) + hash(32)
		assert.Equal(t, byte(1), payload[0], "inv count should be 1")

		invType := binary.LittleEndian.Uint32(payload[1:5])
		assert.Equal(t, uint32(InvTypeMasternodePing), invType)

		var gotHash types.Hash
		copy(gotHash[:], payload[5:37])
		assert.Equal(t, wantHash, gotHash)
	case <-time.After(1 * time.Second):
		t.Fatal("expected inventory message to be queued")
	}
}
