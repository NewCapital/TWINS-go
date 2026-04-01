// Copyright (c) 2026 The TWINS Core developers
// Distributed under the MIT software license

package p2p

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	blockchainpkg "github.com/twins-dev/twins-core/internal/blockchain"
	"github.com/twins-dev/twins-core/internal/config"
	"github.com/twins-dev/twins-core/pkg/types"
)

// newTestServerWithSyncConfig creates a minimal Server with the given SyncConfig for testing.
func newTestServerWithSyncConfig(syncCfg config.SyncConfig) *Server {
	cfg := config.DefaultConfig()
	cfg.Sync = syncCfg
	return &Server{
		config: cfg,
	}
}

// TestSyncConfigIBDThreshold verifies ibdThreshold is read from server config.
func TestSyncConfigIBDThreshold(t *testing.T) {
	srv := newTestServerWithSyncConfig(config.SyncConfig{
		IBDThreshold:        10000,
		BootstrapMinPeers:   4,
		BootstrapMaxWait:    120,
		ProgressLogInterval: 10,
	})

	syncer := NewBlockchainSyncer(nil, nil, types.DefaultChainParams(), logrus.NewEntry(logrus.StandardLogger()), srv)

	if syncer.ibdThreshold != 10000 {
		t.Errorf("expected ibdThreshold=10000, got %d", syncer.ibdThreshold)
	}
	if syncer.stateMachine.ibdThreshold != 10000 {
		t.Errorf("expected stateMachine.ibdThreshold=10000, got %d", syncer.stateMachine.ibdThreshold)
	}
}

// TestSyncConfigIBDThresholdDefault verifies default ibdThreshold when config is zero.
func TestSyncConfigIBDThresholdDefault(t *testing.T) {
	srv := newTestServerWithSyncConfig(config.SyncConfig{
		BootstrapMinPeers:   4,
		BootstrapMaxWait:    120,
		ProgressLogInterval: 10,
	})

	syncer := NewBlockchainSyncer(nil, nil, types.DefaultChainParams(), logrus.NewEntry(logrus.StandardLogger()), srv)

	if syncer.ibdThreshold != blockchainpkg.DefaultIBDThreshold {
		t.Errorf("expected ibdThreshold=%d, got %d", blockchainpkg.DefaultIBDThreshold, syncer.ibdThreshold)
	}
}

// TestSyncConfigConsensusStrategy verifies consensusStrategy is applied.
func TestSyncConfigConsensusStrategy(t *testing.T) {
	tests := []struct {
		name     string
		strategy string
		expected ConsensusStrategy
	}{
		{"outbound_only", "outbound_only", StrategyOutboundOnly},
		{"all", "all", StrategyAll},
		{"empty defaults to outbound_only", "", StrategyOutboundOnly},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newTestServerWithSyncConfig(config.SyncConfig{
				ConsensusStrategy:   tc.strategy,
				BootstrapMinPeers:   4,
				BootstrapMaxWait:    120,
				ProgressLogInterval: 10,
			})

			syncer := NewBlockchainSyncer(nil, nil, types.DefaultChainParams(), logrus.NewEntry(logrus.StandardLogger()), srv)

			syncer.consensusValidator.mu.RLock()
			actual := syncer.consensusValidator.defaultStrategy
			syncer.consensusValidator.mu.RUnlock()

			if actual != tc.expected {
				t.Errorf("expected strategy=%v, got %v", tc.expected, actual)
			}
		})
	}
}

// TestSyncConfigMaxSyncPeers verifies maxSyncPeers is applied to SyncPeerList.
func TestSyncConfigMaxSyncPeers(t *testing.T) {
	srv := newTestServerWithSyncConfig(config.SyncConfig{
		MaxSyncPeers:        50,
		BootstrapMinPeers:   4,
		BootstrapMaxWait:    120,
		ProgressLogInterval: 10,
	})

	syncer := NewBlockchainSyncer(nil, nil, types.DefaultChainParams(), logrus.NewEntry(logrus.StandardLogger()), srv)

	syncer.peerList.mu.RLock()
	actual := syncer.peerList.maxPeers
	syncer.peerList.mu.RUnlock()

	if actual != 50 {
		t.Errorf("expected maxPeers=50, got %d", actual)
	}
}

// TestSyncConfigMaxSyncPeersDefault verifies default maxSyncPeers.
func TestSyncConfigMaxSyncPeersDefault(t *testing.T) {
	srv := newTestServerWithSyncConfig(config.SyncConfig{
		BootstrapMinPeers:   4,
		BootstrapMaxWait:    120,
		ProgressLogInterval: 10,
	})

	syncer := NewBlockchainSyncer(nil, nil, types.DefaultChainParams(), logrus.NewEntry(logrus.StandardLogger()), srv)

	syncer.peerList.mu.RLock()
	actual := syncer.peerList.maxPeers
	syncer.peerList.mu.RUnlock()

	if actual != DefaultMaxSyncPeers {
		t.Errorf("expected maxPeers=%d, got %d", DefaultMaxSyncPeers, actual)
	}
}

// TestSyncConfigBatchTimeout verifies batchTimeout is applied as syncStallTimeout.
func TestSyncConfigBatchTimeout(t *testing.T) {
	srv := newTestServerWithSyncConfig(config.SyncConfig{
		BatchTimeout:        30,
		BootstrapMinPeers:   4,
		BootstrapMaxWait:    120,
		ProgressLogInterval: 10,
	})

	syncer := NewBlockchainSyncer(nil, nil, types.DefaultChainParams(), logrus.NewEntry(logrus.StandardLogger()), srv)

	if syncer.syncStallTimeout != 30*time.Second {
		t.Errorf("expected syncStallTimeout=30s, got %v", syncer.syncStallTimeout)
	}
}

// TestSyncConfigReorgWindow verifies reorgWindow is applied to state machine.
func TestSyncConfigReorgWindow(t *testing.T) {
	srv := newTestServerWithSyncConfig(config.SyncConfig{
		ReorgWindow:         7200,
		BootstrapMinPeers:   4,
		BootstrapMaxWait:    120,
		ProgressLogInterval: 10,
	})

	syncer := NewBlockchainSyncer(nil, nil, types.DefaultChainParams(), logrus.NewEntry(logrus.StandardLogger()), srv)

	syncer.stateMachine.mu.RLock()
	actual := syncer.stateMachine.reorgWindow
	syncer.stateMachine.mu.RUnlock()

	if actual != 7200*time.Second {
		t.Errorf("expected reorgWindow=7200s, got %v", actual)
	}
}

// TestSyncConfigMaxAutoReorgs verifies maxAutoReorgs is applied to state machine.
func TestSyncConfigMaxAutoReorgs(t *testing.T) {
	srv := newTestServerWithSyncConfig(config.SyncConfig{
		MaxAutoReorgs:       5,
		BootstrapMinPeers:   4,
		BootstrapMaxWait:    120,
		ProgressLogInterval: 10,
	})

	syncer := NewBlockchainSyncer(nil, nil, types.DefaultChainParams(), logrus.NewEntry(logrus.StandardLogger()), srv)

	syncer.stateMachine.mu.RLock()
	actual := syncer.stateMachine.maxAutoReorgs
	syncer.stateMachine.mu.RUnlock()

	if actual != 5 {
		t.Errorf("expected maxAutoReorgs=5, got %d", actual)
	}
}

// TestSyncConfigProgressLogInterval verifies progressLogInterval is applied.
func TestSyncConfigProgressLogInterval(t *testing.T) {
	srv := newTestServerWithSyncConfig(config.SyncConfig{
		ProgressLogInterval: 30,
		BootstrapMinPeers:   4,
		BootstrapMaxWait:    120,
	})

	syncer := NewBlockchainSyncer(nil, nil, types.DefaultChainParams(), logrus.NewEntry(logrus.StandardLogger()), srv)

	if syncer.progressLogInterval != 30*time.Second {
		t.Errorf("expected progressLogInterval=30s, got %v", syncer.progressLogInterval)
	}
}

// TestSyncConfigNilServer verifies graceful handling when server is nil.
func TestSyncConfigNilServer(t *testing.T) {
	syncer := NewBlockchainSyncer(nil, nil, types.DefaultChainParams(), logrus.NewEntry(logrus.StandardLogger()), nil)

	// Should use defaults without panicking
	if syncer.ibdThreshold != blockchainpkg.DefaultIBDThreshold {
		t.Errorf("expected default ibdThreshold=%d, got %d", blockchainpkg.DefaultIBDThreshold, syncer.ibdThreshold)
	}
	if syncer.progressLogInterval != 10*time.Second {
		t.Errorf("expected default progressLogInterval=10s, got %v", syncer.progressLogInterval)
	}
}

// TestSyncPeerListMaxPeersRebuild verifies Rebuild respects maxPeers cap.
func TestSyncPeerListMaxPeersRebuild(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)
	peerList.SetMaxPeers(5)

	// Add 10 peers
	for i := 0; i < 10; i++ {
		addr := "peer" + string(rune('A'+i))
		healthTracker.RecordPeerDiscovered(addr, 1000, false, TierBronze, false)
	}

	peerList.Rebuild(healthTracker.GetAllPeers())

	if len(peerList.GetAllPeers()) > 5 {
		t.Errorf("expected at most 5 peers after rebuild, got %d", len(peerList.GetAllPeers()))
	}
}
