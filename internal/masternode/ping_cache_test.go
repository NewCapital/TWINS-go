package masternode

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twins-dev/twins-core/pkg/types"
)

func TestGetPingByHash_UsesSeenPingPayloadCache(t *testing.T) {
	manager, err := NewManager(DefaultConfig(), logrus.New())
	require.NoError(t, err)

	ping := &MasternodePing{
		OutPoint: types.Outpoint{
			Hash:  types.Hash{0xAA, 0xBB},
			Index: 3,
		},
		BlockHash: types.Hash{0xCC},
		SigTime:   time.Now().Unix(),
		Signature: []byte{0x01, 0x02},
	}
	hash := ping.GetHash()

	manager.seenPingsMu.Lock()
	manager.seenPings[hash] = ping.SigTime
	manager.seenPingMessages[hash] = ping
	manager.seenPingsMu.Unlock()

	got := manager.GetPingByHash(hash)
	require.NotNil(t, got)
	assert.Equal(t, hash, got.GetHash())
}

func TestGetPingByHash_FallsBackToMasternodeLastPing(t *testing.T) {
	manager, err := NewManager(DefaultConfig(), logrus.New())
	require.NoError(t, err)

	ping := &MasternodePing{
		OutPoint: types.Outpoint{
			Hash:  types.Hash{0x10, 0x20},
			Index: 9,
		},
		BlockHash: types.Hash{0x30},
		SigTime:   time.Now().Unix(),
		Signature: []byte{0x03, 0x04},
	}
	hash := ping.GetHash()

	mn := &Masternode{
		OutPoint:        ping.OutPoint,
		LastPingMessage: ping,
	}

	manager.mu.Lock()
	manager.masternodes[mn.OutPoint] = mn
	manager.mu.Unlock()

	got := manager.GetPingByHash(hash)
	require.NotNil(t, got)
	assert.Equal(t, hash, got.GetHash())
}

func TestCleanSeenPings_RemovesPayloadCacheEntries(t *testing.T) {
	manager, err := NewManager(DefaultConfig(), logrus.New())
	require.NoError(t, err)

	manager.maxSeenPingsAge = 1
	oldSigTime := time.Now().Unix() - 100

	ping := &MasternodePing{
		OutPoint: types.Outpoint{
			Hash:  types.Hash{0xFE},
			Index: 1,
		},
		BlockHash: types.Hash{0xEE},
		SigTime:   oldSigTime,
		Signature: []byte{0x05},
	}
	hash := ping.GetHash()

	manager.seenPingsMu.Lock()
	manager.seenPings[hash] = oldSigTime
	manager.seenPingMessages[hash] = ping
	manager.seenPingsMu.Unlock()

	manager.CleanSeenPings()

	manager.seenPingsMu.RLock()
	_, hasTime := manager.seenPings[hash]
	_, hasPayload := manager.seenPingMessages[hash]
	manager.seenPingsMu.RUnlock()

	assert.False(t, hasTime)
	assert.False(t, hasPayload)
}

func TestProcessPing_InvalidPingDoesNotStorePayload(t *testing.T) {
	manager, err := NewManager(DefaultConfig(), logrus.New())
	require.NoError(t, err)
	manager.syncManager = nil // Bypass IBD gate for direct ProcessPing validation path.

	ping := &MasternodePing{
		OutPoint: types.Outpoint{
			Hash:  types.Hash{0xBA, 0xAD},
			Index: 42,
		},
		BlockHash: types.Hash{0x99},
		// Unknown outpoint ensures validation fails after dedup marker insertion.
		SigTime:   time.Now().Unix(),
		Signature: []byte{0x01},
	}
	hash := ping.GetHash()

	err = manager.ProcessPing(ping, "")
	require.Error(t, err)

	manager.seenPingsMu.RLock()
	_, hasSeenTime := manager.seenPings[hash]
	_, hasPayload := manager.seenPingMessages[hash]
	manager.seenPingsMu.RUnlock()

	assert.True(t, hasSeenTime, "dedup marker should remain for invalid ping hash")
	assert.False(t, hasPayload, "invalid ping payload should not be retained")
}

func TestHasSeenPing_UsesPayloadCacheWhenSeenPingsMissing(t *testing.T) {
	manager, err := NewManager(DefaultConfig(), logrus.New())
	require.NoError(t, err)

	ping := &MasternodePing{
		OutPoint: types.Outpoint{
			Hash:  types.Hash{0x44, 0x55},
			Index: 2,
		},
		BlockHash: types.Hash{0x66},
		SigTime:   time.Now().Unix(),
		Signature: []byte{0x01},
	}
	hash := ping.GetHash()

	manager.seenPingsMu.Lock()
	manager.seenPingMessages[hash] = ping
	manager.seenPingsMu.Unlock()

	assert.True(t, manager.HasSeenPing(hash), "payload cache should suppress redundant getdata")
}
