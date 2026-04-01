package masternode

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adjustedtime "github.com/twins-dev/twins-core/internal/time"
	"github.com/twins-dev/twins-core/pkg/crypto"
	"github.com/twins-dev/twins-core/pkg/types"
)

// createTestMasternodeForPing creates a test masternode with proper collateral for ping tests
func createTestMasternodeForPing(keyPair *crypto.KeyPair) *Masternode {
	now := time.Now()
	sigTime := now.Add(-1 * time.Hour).Unix()
	addr, _ := net.ResolveTCPAddr("tcp", "127.0.0.1:12345")

	return &Masternode{
		OutPoint: types.Outpoint{
			Hash:  types.Hash{0x01},
			Index: 0,
		},
		Addr:       addr,
		PubKey:     keyPair.Public,
		Tier:       Bronze,
		Collateral: Bronze.Collateral(),
		Status:     StatusEnabled,
		Protocol:   ActiveProtocolVersion,
		SigTime:    sigTime,
		ActiveSince: now,
		LastPing:    now.Add(-time.Hour), // Long ago to allow pings
		LastSeen:    now,
	}
}

// TestProcessPingDeduplication tests that duplicate pings are rejected
func TestProcessPingDeduplication(t *testing.T) {
	manager := createTestManager(t)

	// Mark sync as complete so ProcessPing doesn't return early
	manager.GetSyncManager().SetSynced()

	// Create a test masternode with proper keys
	keyPair, err := crypto.GenerateKeyPair()
	require.NoError(t, err)

	mn := createTestMasternodeForPing(keyPair)

	// Add masternode to manager
	err = manager.AddMasternode(mn)
	require.NoError(t, err)

	// Create a valid ping
	currentTime := adjustedtime.GetAdjustedUnix()
	ping := &MasternodePing{
		OutPoint:  mn.OutPoint,
		BlockHash: types.Hash{0x02}, // Dummy block hash
		SigTime:   currentTime,
	}

	// Sign the ping
	signature, err := crypto.SignCompact(keyPair.Private, ping.getSignatureMessage())
	require.NoError(t, err)
	ping.Signature = signature

	// First ping should succeed
	err = manager.ProcessPing(ping, "")
	assert.NoError(t, err, "First ping should be accepted")

	// Same ping should be silently accepted (deduplication) but not processed again
	err = manager.ProcessPing(ping, "")
	assert.NoError(t, err, "Duplicate ping should be silently accepted")

	// Verify seenPings contains the ping hash
	pingHash := ping.GetHash()
	manager.seenPingsMu.RLock()
	_, seen := manager.seenPings[pingHash]
	manager.seenPingsMu.RUnlock()
	assert.True(t, seen, "Ping hash should be in seenPings map")
}

// TestProcessPingSigTimeValidation tests sigTime ±1 hour window
func TestProcessPingSigTimeValidation(t *testing.T) {
	manager := createTestManager(t)

	// Mark sync as complete so ProcessPing doesn't return early
	manager.GetSyncManager().SetSynced()

	// Create test masternode
	keyPair, err := crypto.GenerateKeyPair()
	require.NoError(t, err)

	mn := createTestMasternodeForPing(keyPair)
	err = manager.AddMasternode(mn)
	require.NoError(t, err)

	currentTime := adjustedtime.GetAdjustedUnix()

	tests := []struct {
		name        string
		sigTime     int64
		shouldError bool
		errorMsg    string
	}{
		{
			name:        "Valid sigTime (current)",
			sigTime:     currentTime,
			shouldError: false,
		},
		{
			name:        "Valid sigTime (30 min future)",
			sigTime:     currentTime + 1800,
			shouldError: false,
		},
		{
			name:        "Valid sigTime (30 min past)",
			sigTime:     currentTime - 1800,
			shouldError: false,
		},
		{
			name:        "Invalid sigTime (>1 hour future)",
			sigTime:     currentTime + 3601,
			shouldError: true,
			errorMsg:    "too far in the future",
		},
		{
			name:        "Invalid sigTime (>1 hour past)",
			sigTime:     currentTime - 3601,
			shouldError: true,
			errorMsg:    "too far in the past",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear seen pings for each test
			manager.seenPingsMu.Lock()
			manager.seenPings = make(map[types.Hash]int64)
			manager.seenPingsMu.Unlock()

			// Reset LastPingMessage to nil so spacing check doesn't interfere
			mn.mu.Lock()
			mn.LastPingMessage = nil
			mn.mu.Unlock()

			ping := &MasternodePing{
				OutPoint:  mn.OutPoint,
				BlockHash: types.Hash{0x02},
				SigTime:   tt.sigTime,
			}

			signature, err := crypto.SignCompact(keyPair.Private, ping.getSignatureMessage())
			require.NoError(t, err)
			ping.Signature = signature

			err = manager.ProcessPing(ping, "")
			if tt.shouldError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestProcessPingSpacingRule tests minimum time between pings (240 seconds = MinPingSeconds - PingSpacingGrace)
func TestProcessPingSpacingRule(t *testing.T) {
	manager := createTestManager(t)

	// Mark sync as complete so ProcessPing doesn't return early
	manager.GetSyncManager().SetSynced()

	// Create test masternode
	keyPair, err := crypto.GenerateKeyPair()
	require.NoError(t, err)

	currentTime := adjustedtime.GetAdjustedUnix()

	// Create initial ping message that was "received" earlier
	lastPing := &MasternodePing{
		OutPoint:  types.Outpoint{Hash: types.Hash{0x01}, Index: 0},
		BlockHash: types.Hash{0x02},
		SigTime:   currentTime - 300, // 5 minutes ago
	}

	mn := createTestMasternodeForPing(keyPair)
	mn.LastPing = time.Unix(lastPing.SigTime, 0)
	mn.LastPingMessage = lastPing

	err = manager.AddMasternode(mn)
	require.NoError(t, err)

	tests := []struct {
		name        string
		sigTime     int64
		shouldError bool
		errorMsg    string
	}{
		{
			name:        "Ping too soon (100 seconds since last)",
			sigTime:     lastPing.SigTime + 100,
			shouldError: true,
			errorMsg:    "ping too soon",
		},
		{
			name:        "Ping too soon (239 seconds since last)",
			sigTime:     lastPing.SigTime + 239,
			shouldError: true,
			errorMsg:    "ping too soon",
		},
		{
			name:        "Ping at minimum spacing (240 seconds)",
			sigTime:     lastPing.SigTime + 240,
			shouldError: false,
		},
		{
			name:        "Ping well after minimum (5 min)",
			sigTime:     lastPing.SigTime + 300,
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear seen pings for each test
			manager.seenPingsMu.Lock()
			manager.seenPings = make(map[types.Hash]int64)
			manager.seenPingsMu.Unlock()

			// Reset last ping message to original state
			mn.mu.Lock()
			mn.LastPingMessage = lastPing
			mn.mu.Unlock()

			ping := &MasternodePing{
				OutPoint:  mn.OutPoint,
				BlockHash: types.Hash{0x03}, // Different hash to avoid dedup
				SigTime:   tt.sigTime,
			}

			signature, err := crypto.SignCompact(keyPair.Private, ping.getSignatureMessage())
			require.NoError(t, err)
			ping.Signature = signature

			err = manager.ProcessPing(ping, "")
			if tt.shouldError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestCleanSeenPings tests TTL-based cleanup of seen pings
func TestCleanSeenPings(t *testing.T) {
	manager := createTestManager(t)

	// Set a short max age for testing
	manager.maxSeenPingsAge = 100 // 100 seconds

	currentTime := adjustedtime.GetAdjustedUnix()

	// Add some old and new ping entries
	manager.seenPingsMu.Lock()
	manager.seenPings[types.Hash{0x01}] = currentTime - 200 // Old (200 seconds ago)
	manager.seenPings[types.Hash{0x02}] = currentTime - 150 // Old (150 seconds ago)
	manager.seenPings[types.Hash{0x03}] = currentTime - 50  // Recent (50 seconds ago)
	manager.seenPings[types.Hash{0x04}] = currentTime       // Current
	manager.seenPingsMu.Unlock()

	// Verify we have 4 entries
	manager.seenPingsMu.RLock()
	assert.Equal(t, 4, len(manager.seenPings))
	manager.seenPingsMu.RUnlock()

	// Run cleanup
	manager.CleanSeenPings()

	// Verify old entries were removed
	manager.seenPingsMu.RLock()
	assert.Equal(t, 2, len(manager.seenPings), "Should have removed 2 old entries")
	_, has03 := manager.seenPings[types.Hash{0x03}]
	_, has04 := manager.seenPings[types.Hash{0x04}]
	manager.seenPingsMu.RUnlock()

	assert.True(t, has03, "Recent entry 0x03 should still exist")
	assert.True(t, has04, "Current entry 0x04 should still exist")
}

// TestPingManagerCheckAndUpdatePing tests PingManager.CheckAndUpdatePing
func TestPingManagerCheckAndUpdatePing(t *testing.T) {
	manager := createTestManager(t)
	pingManager := NewPingManager(manager, nil)

	// Create test masternode
	keyPair, err := crypto.GenerateKeyPair()
	require.NoError(t, err)

	mn := createTestMasternodeForPing(keyPair)
	err = manager.AddMasternode(mn)
	require.NoError(t, err)

	currentTime := adjustedtime.GetAdjustedUnix()

	t.Run("Valid ping is accepted and relayed", func(t *testing.T) {
		ping := &MasternodePing{
			OutPoint:  mn.OutPoint,
			BlockHash: types.Hash{0x02},
			SigTime:   currentTime,
		}
		signature, err := crypto.SignCompact(keyPair.Private, ping.getSignatureMessage())
		require.NoError(t, err)
		ping.Signature = signature

		result := pingManager.CheckAndUpdatePing(ping, currentTime, false, false)
		assert.True(t, result.Accepted, "Valid ping should be accepted")
		assert.True(t, result.Relay, "Valid ping should be relayed")
		assert.Equal(t, 0, result.DoS, "Valid ping should not trigger DoS")
	})

	t.Run("Duplicate ping is accepted but not relayed", func(t *testing.T) {
		// Clear seenPings in the pingManager for this test
		pingManager.seenPingsMu.Lock()
		pingManager.seenPings = make(map[types.Hash]int64)
		pingManager.seenPingsMu.Unlock()

		// Reset LastPingMessage so spacing check doesn't interfere
		mn.mu.Lock()
		mn.LastPingMessage = nil
		mn.mu.Unlock()

		ping := &MasternodePing{
			OutPoint:  mn.OutPoint,
			BlockHash: types.Hash{0x05}, // Different hash
			SigTime:   currentTime,
		}
		signature, err := crypto.SignCompact(keyPair.Private, ping.getSignatureMessage())
		require.NoError(t, err)
		ping.Signature = signature

		// First ping accepted
		result1 := pingManager.CheckAndUpdatePing(ping, currentTime, false, false)
		require.True(t, result1.Accepted, "First ping should be accepted")

		// Second (duplicate) should be accepted but not relayed
		result := pingManager.CheckAndUpdatePing(ping, currentTime, false, false)
		assert.True(t, result.Accepted, "Duplicate ping should be accepted")
		assert.False(t, result.Relay, "Duplicate ping should NOT be relayed")
		assert.True(t, result.ShouldSkip, "Duplicate ping should be skipped")
	})

	t.Run("SigTime too far in future triggers DoS", func(t *testing.T) {
		ping := &MasternodePing{
			OutPoint:  mn.OutPoint,
			BlockHash: types.Hash{0x03},
			SigTime:   currentTime + 3700, // More than 1 hour
		}
		signature, err := crypto.SignCompact(keyPair.Private, ping.getSignatureMessage())
		require.NoError(t, err)
		ping.Signature = signature

		result := pingManager.CheckAndUpdatePing(ping, currentTime, false, false)
		assert.False(t, result.Accepted, "Future ping should be rejected")
		assert.Equal(t, 1, result.DoS, "Future ping should trigger DoS=1")
		assert.Contains(t, result.Error, "too far into the future")
	})
}
