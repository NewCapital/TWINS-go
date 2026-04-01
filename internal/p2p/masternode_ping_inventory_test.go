package p2p

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/twins-dev/twins-core/internal/masternode"
	"github.com/twins-dev/twins-core/pkg/types"
)

type stubMasternodeManager struct {
	seenPingHashes     map[types.Hash]bool
	getPingByHashCalls int
	processPingCalls   int
}

var _ MasternodeManager = (*stubMasternodeManager)(nil)

func (s *stubMasternodeManager) GetPublicKey(outpoint types.Outpoint) ([]byte, error) {
	return nil, nil
}

func (s *stubMasternodeManager) IsActive(outpoint types.Outpoint, height uint32) bool {
	return false
}

func (s *stubMasternodeManager) GetTier(outpoint types.Outpoint) (uint8, error) {
	return 0, nil
}

func (s *stubMasternodeManager) GetPaymentQueuePosition(outpoint types.Outpoint, height uint32) (int, error) {
	return 0, nil
}

func (s *stubMasternodeManager) GetLastPaidBlock(outpoint types.Outpoint) (uint32, error) {
	return 0, nil
}

func (s *stubMasternodeManager) ProcessBroadcast(mnb *masternode.MasternodeBroadcast, originAddr string) error {
	return nil
}

func (s *stubMasternodeManager) ProcessPing(mnp *masternode.MasternodePing, peerAddr string) error {
	s.processPingCalls++
	return nil
}

func (s *stubMasternodeManager) GetMasternodeList() *masternode.MasternodeList {
	return nil
}

func (s *stubMasternodeManager) GetBroadcastByHash(hash types.Hash) (*masternode.MasternodeBroadcast, error) {
	return nil, nil
}

func (s *stubMasternodeManager) CountEnabled(protocolVersion int32) int {
	return 0
}

func (s *stubMasternodeManager) StoreWinnerVote(voterOutpoint types.Outpoint, blockHeight uint32, payeeScript, signature []byte) {
}

func (s *stubMasternodeManager) GetMinMasternodePaymentsProto() int32 {
	return 0
}

func (s *stubMasternodeManager) GetMasternodeRank(outpoint types.Outpoint, blockHeight uint32, minProtocol int32, filterTier bool) int {
	return 0
}

func (s *stubMasternodeManager) GetMasternode(outpoint types.Outpoint) (*masternode.Masternode, error) {
	return nil, nil
}

func (s *stubMasternodeManager) AddedMasternodeWinner(hash types.Hash) {
}

func (s *stubMasternodeManager) ProcessSyncStatusCount(peerAddr string, syncType int, count int) {
}

func (s *stubMasternodeManager) HasFulfilledRequest(peerAddr string, requestType string) bool {
	return false
}

func (s *stubMasternodeManager) FulfilledRequest(peerAddr string, requestType string) {
}

func (s *stubMasternodeManager) MarkBroadcastSeen(mnb *masternode.MasternodeBroadcast) {
}

func (s *stubMasternodeManager) GetPingByHash(hash types.Hash) *masternode.MasternodePing {
	s.getPingByHashCalls++
	return nil
}

func (s *stubMasternodeManager) GetPeerAddresses() []string {
	return nil
}

// Extra fast-path method used via type assertion in processMasternodePingInventory.
func (s *stubMasternodeManager) HasSeenPing(hash types.Hash) bool {
	return s.seenPingHashes[hash]
}

func TestProcessMasternodePingInventory_SeenHashSkipsLookupAndRequest(t *testing.T) {
	logger := logrus.New()
	peer := NewPeer(NewMockConn(), false, MagicToBytes(MainNetMagic), logger)

	knownHash := types.Hash{0xAB, 0xCD}
	mnMgr := &stubMasternodeManager{
		seenPingHashes: map[types.Hash]bool{
			knownHash: true,
		},
	}

	server := &Server{
		logger:    logger.WithField("test", "mnp-inv-seen-skip"),
		mnManager: mnMgr,
	}

	inv := InventoryVector{
		Type: InvTypeMasternodePing,
		Hash: knownHash,
	}
	got := server.processMasternodePingInventory(peer, inv, false)

	assert.Nil(t, got, "known ping hash should not be requested again")
	assert.Equal(t, 0, mnMgr.getPingByHashCalls, "hot-path should not call slow payload lookup")
}

func TestProcessMasternodePingInventory_UnknownHashRequestsData(t *testing.T) {
	logger := logrus.New()
	peer := NewPeer(NewMockConn(), false, MagicToBytes(MainNetMagic), logger)

	unknownHash := types.Hash{0x01, 0x02}
	mnMgr := &stubMasternodeManager{
		seenPingHashes: map[types.Hash]bool{},
	}

	server := &Server{
		logger:    logger.WithField("test", "mnp-inv-unknown-request"),
		mnManager: mnMgr,
	}

	inv := InventoryVector{
		Type: InvTypeMasternodePing,
		Hash: unknownHash,
	}
	got := server.processMasternodePingInventory(peer, inv, false)

	if assert.NotNil(t, got, "unknown ping hash should be requested") {
		assert.Equal(t, inv.Hash, got.Hash)
		assert.Equal(t, inv.Type, got.Type)
	}
	assert.Equal(t, 0, mnMgr.getPingByHashCalls, "with fast-path support we should not fallback to slow scan")
}

func TestHandleMasternodePing_InvalidSignatureLengthRejectedEarly(t *testing.T) {
	logger := logrus.New()
	peer := NewPeer(NewMockConn(), false, MagicToBytes(MainNetMagic), logger)
	mnMgr := &stubMasternodeManager{
		seenPingHashes: map[types.Hash]bool{},
	}
	server := &Server{
		logger:    logger.WithField("test", "mnp-invalid-siglen"),
		mnManager: mnMgr,
	}

	ping := &masternode.MasternodePing{
		OutPoint: types.Outpoint{
			Hash:  types.Hash{0x11, 0x22},
			Index: 1,
		},
		BlockHash: types.Hash{0x33},
		SigTime:   1,
		Signature: []byte{0xAA, 0xBB}, // invalid compact signature length
	}
	payload, err := SerializeMasternodePing(ping)
	if !assert.NoError(t, err) {
		return
	}
	msg := NewMessage(MsgMNPing, payload, MagicToBytes(MainNetMagic))

	server.handleMasternodePing(peer, msg)

	assert.Equal(t, 0, mnMgr.processPingCalls, "invalid signature length should be rejected before ProcessPing")
}
