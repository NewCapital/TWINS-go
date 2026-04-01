package masternode

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/twins-dev/twins-core/pkg/types"
)

// mockSyncPeerRequester implements SyncPeerRequester for testing
type mockSyncPeerRequester struct {
	sporkRequests  int
	listRequests   int
	winnerRequests int
	connectedPeers int
	// dseg response simulation: how many peers sent/skipped
	dsegSent    int
	dsegSkipped int
}

func (m *mockSyncPeerRequester) RequestSporks() error { m.sporkRequests++; return nil }
func (m *mockSyncPeerRequester) RequestMasternodeList() (int, int, error) {
	m.listRequests++
	return m.dsegSent, m.dsegSkipped, nil
}
func (m *mockSyncPeerRequester) RequestMasternodeWinners(int) (int, int, error) {
	m.winnerRequests++
	return m.dsegSent, m.dsegSkipped, nil
}
func (m *mockSyncPeerRequester) GetConnectedPeerCount() int         { return m.connectedPeers }

// mockBlockchainSyncer implements BlockchainSyncer for testing
type mockBlockchainSyncer struct{}

func (m *mockBlockchainSyncer) GetBestHeight() (uint32, error)                      { return 100000, nil }
func (m *mockBlockchainSyncer) GetBlockByHeight(height uint32) (*types.Block, error) { return nil, nil }
func (m *mockBlockchainSyncer) IsInitialBlockDownload() bool                         { return false }

// mockNetworkSyncStatus implements NetworkSyncStatusProvider for testing
type mockNetworkSyncStatus struct{ synced bool }

func (m *mockNetworkSyncStatus) IsSynced() bool { return m.synced }

func TestProcessSyncList_AllPeersSkipped_WithCache_Advances(t *testing.T) {
	sm := NewSyncManager(logrus.StandardLogger())
	// All peers already asked (sent=0, skipped=5) — simulates quick restart
	peer := &mockSyncPeerRequester{connectedPeers: 5, dsegSent: 0, dsegSkipped: 5}
	sm.SetPeerRequester(peer)
	sm.SetNetworkSyncStatus(&mockNetworkSyncStatus{synced: true})

	// Fresh cache with masternodes
	sm.NotifyCacheLoaded(time.Now().Add(-10*time.Minute), 500)

	sm.mu.Lock()
	sm.currentState = SyncList
	sm.requestAttempt = 0
	sm.blockchainSynced = true
	sm.mu.Unlock()

	// All peers skipped + cache exists → should advance immediately to SyncMNW
	sm.Process()

	sm.mu.RLock()
	state := sm.currentState
	sm.mu.RUnlock()

	if state != SyncMNW {
		t.Errorf("expected state SyncMNW when all peers skipped with cache, got %s", state.String())
	}

	// dseg was still attempted (per-peer check happens inside RequestMasternodeList)
	if peer.listRequests != 1 {
		t.Errorf("expected 1 list request, got %d", peer.listRequests)
	}
}

func TestProcessSyncList_SomePeersSent_StaysInSyncList(t *testing.T) {
	sm := NewSyncManager(logrus.StandardLogger())
	// Some peers sent, some skipped — new peers available
	peer := &mockSyncPeerRequester{connectedPeers: 5, dsegSent: 2, dsegSkipped: 3}
	sm.SetPeerRequester(peer)
	sm.SetNetworkSyncStatus(&mockNetworkSyncStatus{synced: true})

	sm.NotifyCacheLoaded(time.Now().Add(-10*time.Minute), 500)

	sm.mu.Lock()
	sm.currentState = SyncList
	sm.requestAttempt = 0
	sm.blockchainSynced = true
	sm.mu.Unlock()

	// Some peers got dseg → should stay in SyncList waiting for responses
	sm.Process()

	sm.mu.RLock()
	state := sm.currentState
	sm.mu.RUnlock()

	if state != SyncList {
		t.Errorf("expected state SyncList when some peers sent dseg, got %s", state.String())
	}
}

func TestProcessSyncList_AllPeersSkipped_NoCache_StaysInSyncList(t *testing.T) {
	sm := NewSyncManager(logrus.StandardLogger())
	// All peers skipped but no cache data — can't advance
	peer := &mockSyncPeerRequester{connectedPeers: 5, dsegSent: 0, dsegSkipped: 5}
	sm.SetPeerRequester(peer)
	sm.SetNetworkSyncStatus(&mockNetworkSyncStatus{synced: true})

	// Empty cache (0 masternodes)
	sm.NotifyCacheLoaded(time.Now().Add(-10*time.Minute), 0)

	sm.mu.Lock()
	sm.currentState = SyncList
	sm.requestAttempt = 0
	sm.blockchainSynced = true
	sm.mu.Unlock()

	sm.Process()

	sm.mu.RLock()
	state := sm.currentState
	sm.mu.RUnlock()

	if state != SyncList {
		t.Errorf("expected state SyncList with empty cache, got %s", state.String())
	}
}

func TestProcessSyncList_NoCacheNotification_SendsDseg(t *testing.T) {
	sm := NewSyncManager(logrus.StandardLogger())
	// Fresh start, no cache — peers get dseg
	peer := &mockSyncPeerRequester{connectedPeers: 5, dsegSent: 5, dsegSkipped: 0}
	sm.SetPeerRequester(peer)
	sm.SetNetworkSyncStatus(&mockNetworkSyncStatus{synced: true})

	// No NotifyCacheLoaded called (zero values)

	sm.mu.Lock()
	sm.currentState = SyncList
	sm.requestAttempt = 0
	sm.blockchainSynced = true
	sm.mu.Unlock()

	sm.Process()

	sm.mu.RLock()
	state := sm.currentState
	sm.mu.RUnlock()

	if state != SyncList {
		t.Errorf("expected state SyncList without cache notification, got %s", state.String())
	}

	if peer.listRequests != 1 {
		t.Errorf("expected 1 list request, got %d", peer.listRequests)
	}
}

func TestProcessSyncList_StaleCache_SendsDseg(t *testing.T) {
	sm := NewSyncManager(logrus.StandardLogger())
	// Stale cache, peers get dseg (cooldowns expired so all peers accept)
	peer := &mockSyncPeerRequester{connectedPeers: 5, dsegSent: 5, dsegSkipped: 0}
	sm.SetPeerRequester(peer)
	sm.SetNetworkSyncStatus(&mockNetworkSyncStatus{synced: true})

	// Stale cache (4 hours old)
	sm.NotifyCacheLoaded(time.Now().Add(-4*time.Hour), 500)

	sm.mu.Lock()
	sm.currentState = SyncList
	sm.requestAttempt = 0
	sm.blockchainSynced = true
	sm.mu.Unlock()

	sm.Process()

	sm.mu.RLock()
	state := sm.currentState
	sm.mu.RUnlock()

	// Sent to peers, waiting for responses
	if state != SyncList {
		t.Errorf("expected state SyncList with stale cache, got %s", state.String())
	}

	if peer.listRequests == 0 {
		t.Error("expected list request with stale cache, got 0")
	}
}

func TestGetCacheInfo_ReturnsMetadata(t *testing.T) {
	cfg := DefaultConfig()
	m, err := NewManager(cfg, logrus.StandardLogger())
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Initially zero values
	loadedAt, count := m.GetCacheInfo()
	if !loadedAt.IsZero() {
		t.Error("expected zero loadedAt initially")
	}
	if count != 0 {
		t.Errorf("expected 0 count initially, got %d", count)
	}

	// Set values directly for testing
	now := time.Now()
	m.mu.Lock()
	m.cacheLoadedAt = now
	m.cacheLoadedCount = 42
	m.mu.Unlock()

	loadedAt, count = m.GetCacheInfo()
	if !loadedAt.Equal(now) {
		t.Errorf("expected loadedAt %v, got %v", now, loadedAt)
	}
	if count != 42 {
		t.Errorf("expected count 42, got %d", count)
	}
}

func TestProcessSyncStatusCount_UpdatesLastMasternodeList(t *testing.T) {
	sm := NewSyncManager(logrus.StandardLogger())
	sm.mu.Lock()
	sm.currentState = SyncList
	sm.requestAttempt = SyncThreshold // meets the update condition
	sm.lastMasternodeList = 0
	sm.blockchainSynced = true
	sm.mu.Unlock()

	before := time.Now().Unix()
	sm.ProcessSyncStatusCount("10.0.0.1:37817", int(SyncList), 5) // count > 0 so no fast-path
	after := time.Now().Unix()

	sm.mu.RLock()
	ts := sm.lastMasternodeList
	sm.mu.RUnlock()

	if ts < before || ts > after {
		t.Errorf("lastMasternodeList not updated: got %d, want in [%d, %d]", ts, before, after)
	}
}

func TestProcessSyncStatusCount_NoUpdateBelowThreshold(t *testing.T) {
	sm := NewSyncManager(logrus.StandardLogger())
	sm.mu.Lock()
	sm.currentState = SyncList
	sm.requestAttempt = SyncThreshold - 1 // below threshold — no update
	sm.lastMasternodeList = 0
	sm.blockchainSynced = true
	sm.mu.Unlock()

	sm.ProcessSyncStatusCount("10.0.0.1:37817", int(SyncList), 5)

	sm.mu.RLock()
	ts := sm.lastMasternodeList
	sm.mu.RUnlock()

	if ts != 0 {
		t.Errorf("lastMasternodeList should not be updated below threshold, got %d", ts)
	}
}

func TestProcessSyncStatusCount_AdvancesWhenAllPeersReturnZero(t *testing.T) {
	sm := NewSyncManager(logrus.StandardLogger())
	peer := &mockSyncPeerRequester{connectedPeers: 3, dsegSent: 1, dsegSkipped: 0}
	sm.SetPeerRequester(peer)
	sm.mu.Lock()
	sm.currentState = SyncList
	sm.requestAttempt = SyncThreshold
	sm.blockchainSynced = true
	sm.mu.Unlock()

	// Two peers both respond with 0 masternodes (SyncThreshold = 2)
	sm.ProcessSyncStatusCount("10.0.0.1:37817", int(SyncList), 0)
	sm.ProcessSyncStatusCount("10.0.0.2:37817", int(SyncList), 0)

	sm.mu.RLock()
	state := sm.currentState
	sm.mu.RUnlock()

	if state != SyncMNW {
		t.Errorf("expected SyncMNW after all peers return 0 masternodes, got %s", state.String())
	}
}

func TestProcessSyncStatusCount_StaysInListWhenSomePeersReturnNonZero(t *testing.T) {
	sm := NewSyncManager(logrus.StandardLogger())
	sm.mu.Lock()
	sm.currentState = SyncList
	sm.requestAttempt = SyncThreshold
	sm.blockchainSynced = true
	sm.mu.Unlock()

	// One peer returns 0, one returns 2 — sumMasternodeList > 0, no fast-path
	sm.ProcessSyncStatusCount("10.0.0.1:37817", int(SyncList), 0)
	sm.ProcessSyncStatusCount("10.0.0.2:37817", int(SyncList), 2)

	sm.mu.RLock()
	state := sm.currentState
	sm.mu.RUnlock()

	if state != SyncList {
		t.Errorf("expected SyncList when some peers return masternodes, got %s", state.String())
	}
}

func TestProcessSyncStatusCount_StaysInListBelowPeerThreshold(t *testing.T) {
	sm := NewSyncManager(logrus.StandardLogger())
	sm.mu.Lock()
	sm.currentState = SyncList
	sm.requestAttempt = SyncThreshold
	sm.blockchainSynced = true
	sm.mu.Unlock()

	// Only 1 peer responds with 0 — below SyncThreshold (2), no fast-path
	sm.ProcessSyncStatusCount("10.0.0.1:37817", int(SyncList), 0)

	sm.mu.RLock()
	state := sm.currentState
	sm.mu.RUnlock()

	if state != SyncList {
		t.Errorf("expected SyncList with only 1 peer response, got %s", state.String())
	}
}

func TestProcessSyncStatusCount_SinglePeerRepeated_DoesNotAdvance(t *testing.T) {
	sm := NewSyncManager(logrus.StandardLogger())
	sm.mu.Lock()
	sm.currentState = SyncList
	sm.requestAttempt = SyncThreshold
	sm.blockchainSynced = true
	sm.mu.Unlock()

	// Same peer sends SyncThreshold ssc(0) messages — should NOT satisfy the threshold
	// because peerSSCResponses is keyed by address (only 1 distinct peer).
	for i := 0; i < SyncThreshold; i++ {
		sm.ProcessSyncStatusCount("10.0.0.1:37817", int(SyncList), 0)
	}

	sm.mu.RLock()
	state := sm.currentState
	sm.mu.RUnlock()

	if state != SyncList {
		t.Errorf("single peer repeated ssc(0) should not advance state, got %s", state.String())
	}
}
