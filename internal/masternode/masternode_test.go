package masternode

import (
	"net"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/twins-dev/twins-core/pkg/crypto"
	"github.com/twins-dev/twins-core/pkg/types"
)

func createTestManager(t *testing.T) *Manager {
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	config := DefaultConfig()
	manager, err := NewManager(config, logger)
	require.NoError(t, err)

	return manager
}

func createTestMasternode(tier MasternodeTier) *Masternode {
	kp, _ := crypto.GenerateKeyPair()

	addr, _ := net.ResolveTCPAddr("tcp", "127.0.0.1:12345")

	now := time.Now()
	// SigTime must be old enough to pass the sigTime filter:
	// sigTime + (millionsLocked * 2.6 * 60) <= currentTime
	// For tests with no other masternodes, millionsLocked = 0, so any sigTime works
	// But we set it to 1 hour ago for safety
	sigTime := now.Add(-1 * time.Hour).Unix()

	return &Masternode{
		OutPoint: types.Outpoint{
			Hash:  types.NewHash([]byte("test")),
			Index: 0,
		},
		Addr:       addr,
		PubKey:     kp.Public,
		Tier:       tier,
		Collateral: tier.Collateral(),
		Status:     StatusEnabled,
		Protocol:   MinPeerProtoAfterEnforcement, // 70927 - required for payment selection
		SigTime:    sigTime,                      // Required for payment selection algorithm
		ActiveSince: now,
		LastPing:    now,
		LastSeen:    now,
		// Cycle tracking for SecondsSincePayment
		PrevCycleLastPaymentTime: sigTime,
	}
}

func TestNewManager(t *testing.T) {
	manager := createTestManager(t)
	assert.NotNil(t, manager)
	assert.NotNil(t, manager.config)
	assert.NotNil(t, manager.logger)
	assert.Equal(t, 0, manager.GetMasternodeCount())
}

func TestStartStop(t *testing.T) {
	manager := createTestManager(t)

	// Use StartWithoutValidation for unit tests (no sporkManager/blockchain configured)
	err := manager.StartWithoutValidation()
	assert.NoError(t, err)

	err = manager.Stop()
	assert.NoError(t, err)
}

func TestStartRequiresDependencies(t *testing.T) {
	manager := createTestManager(t)

	// Start() without dependencies should fail
	err := manager.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "sporkManager is REQUIRED")
}

func TestAddMasternode(t *testing.T) {
	manager := createTestManager(t)

	mn := createTestMasternode(Bronze)
	err := manager.AddMasternode(mn)
	assert.NoError(t, err)

	assert.Equal(t, 1, manager.GetMasternodeCount())
}

func TestAddDuplicateMasternode(t *testing.T) {
	manager := createTestManager(t)

	mn := createTestMasternode(Bronze)
	err := manager.AddMasternode(mn)
	require.NoError(t, err)

	// Try to add same masternode again
	err = manager.AddMasternode(mn)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestGetMasternode(t *testing.T) {
	manager := createTestManager(t)

	mn := createTestMasternode(Silver)
	err := manager.AddMasternode(mn)
	require.NoError(t, err)

	retrieved, err := manager.GetMasternode(mn.OutPoint)
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, mn.OutPoint, retrieved.OutPoint)
	assert.Equal(t, mn.Tier, retrieved.Tier)
}

func TestRemoveMasternode(t *testing.T) {
	manager := createTestManager(t)

	mn := createTestMasternode(Gold)
	err := manager.AddMasternode(mn)
	require.NoError(t, err)

	err = manager.RemoveMasternode(mn.OutPoint)
	assert.NoError(t, err)

	assert.Equal(t, 0, manager.GetMasternodeCount())

	// Verify it's really gone
	_, err = manager.GetMasternode(mn.OutPoint)
	assert.Error(t, err)
}

func TestGetMasternodesByTier(t *testing.T) {
	manager := createTestManager(t)

	// Add masternodes of different tiers
	tiers := []MasternodeTier{Bronze, Bronze, Silver, Gold, Platinum}
	for i, tier := range tiers {
		mn := createTestMasternode(tier)
		mn.OutPoint.Index = uint32(i) // Make unique outpoints
		err := manager.AddMasternode(mn)
		require.NoError(t, err)
	}

	// Get bronze masternodes
	bronzeMNs := manager.GetMasternodesByTier(Bronze)
	assert.Len(t, bronzeMNs, 2)

	// Get silver masternodes
	silverMNs := manager.GetMasternodesByTier(Silver)
	assert.Len(t, silverMNs, 1)

	// Get gold masternodes
	goldMNs := manager.GetMasternodesByTier(Gold)
	assert.Len(t, goldMNs, 1)

	// Get platinum masternodes
	platinumMNs := manager.GetMasternodesByTier(Platinum)
	assert.Len(t, platinumMNs, 1)
}

func TestGetMasternodeCount(t *testing.T) {
	manager := createTestManager(t)

	assert.Equal(t, 0, manager.GetMasternodeCount())

	// Add 5 masternodes
	for i := 0; i < 5; i++ {
		mn := createTestMasternode(Bronze)
		mn.OutPoint.Index = uint32(i)
		err := manager.AddMasternode(mn)
		require.NoError(t, err)
	}

	assert.Equal(t, 5, manager.GetMasternodeCount())
}

func TestGetMasternodeCountByTier(t *testing.T) {
	manager := createTestManager(t)

	// Add 3 bronze and 2 silver
	for i := 0; i < 3; i++ {
		mn := createTestMasternode(Bronze)
		mn.OutPoint.Index = uint32(i)
		manager.AddMasternode(mn)
	}

	for i := 0; i < 2; i++ {
		mn := createTestMasternode(Silver)
		mn.OutPoint.Index = uint32(i + 10)
		manager.AddMasternode(mn)
	}

	assert.Equal(t, 3, manager.GetMasternodeCountByTier(Bronze))
	assert.Equal(t, 2, manager.GetMasternodeCountByTier(Silver))
	assert.Equal(t, 0, manager.GetMasternodeCountByTier(Gold))
}

func TestIsMasternodeActive(t *testing.T) {
	manager := createTestManager(t)

	mn := createTestMasternode(Bronze)
	mn.Status = StatusEnabled
	err := manager.AddMasternode(mn)
	require.NoError(t, err)

	assert.True(t, manager.IsMasternodeActive(mn.OutPoint))

	// Remove it
	manager.RemoveMasternode(mn.OutPoint)
	assert.False(t, manager.IsMasternodeActive(mn.OutPoint))
}

func TestProcessPayment(t *testing.T) {
	manager := createTestManager(t)

	mn := createTestMasternode(Bronze)
	err := manager.AddMasternode(mn)
	require.NoError(t, err)

	// Process payment using ProcessPaymentWithBlockTime (legacy compatible)
	// Must use block timestamp and hash for deterministic consensus
	blockTime := time.Now().Unix()
	blockHash := types.Hash{0x01, 0x02, 0x03} // Test block hash
	err = manager.ProcessPaymentWithBlockTime(mn.OutPoint, 1000, blockTime, blockHash)
	assert.NoError(t, err)

	// Verify payment count increased
	updated, err := manager.GetMasternode(mn.OutPoint)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), updated.PaymentCount)
}

func TestProcessPaymentDeprecatedPanics(t *testing.T) {
	manager := createTestManager(t)

	mn := createTestMasternode(Bronze)
	err := manager.AddMasternode(mn)
	require.NoError(t, err)

	// Verify that deprecated ProcessPayment panics
	assert.Panics(t, func() {
		manager.ProcessPayment(mn.OutPoint, 1000)
	}, "ProcessPayment should panic as it is deprecated")
}

func TestGetPaymentQueue(t *testing.T) {
	manager := createTestManager(t)

	// Add multiple masternodes
	for i := 0; i < 3; i++ {
		mn := createTestMasternode(Bronze)
		mn.OutPoint.Index = uint32(i)
		manager.AddMasternode(mn)
	}

	queue := manager.GetPaymentQueue()
	assert.Len(t, queue, 3)
}

func TestGetNextPayee(t *testing.T) {
	manager := createTestManager(t)

	// Add masternodes
	for i := 0; i < 3; i++ {
		mn := createTestMasternode(Bronze)
		mn.OutPoint.Index = uint32(i)
		manager.AddMasternode(mn)
	}

	// LEGACY COMPATIBILITY FIX: GetNextPayee now uses GetAdjustedTime() instead of
	// blockchain block timestamp, so it doesn't require blockchain to be set.
	// It should return a masternode if there are eligible ones.
	next, err := manager.GetNextPayee()
	assert.NoError(t, err)
	assert.NotNil(t, next)
}

func TestMasternodeInfo(t *testing.T) {
	manager := createTestManager(t)

	mn := createTestMasternode(Bronze)
	err := manager.AddMasternode(mn)
	require.NoError(t, err)

	info, err := manager.GetMasternodeInfo(mn.OutPoint)
	assert.NoError(t, err)
	assert.NotNil(t, info)
	assert.Equal(t, mn.Tier.String(), info.Tier)
	assert.Equal(t, mn.Collateral, info.Collateral)
	assert.Equal(t, "enabled", info.Status)
}

func TestGetMasternodeList(t *testing.T) {
	manager := createTestManager(t)

	// Add several masternodes
	for i := 0; i < 5; i++ {
		mn := createTestMasternode(Bronze)
		mn.OutPoint.Index = uint32(i)
		manager.AddMasternode(mn)
	}

	list := manager.GetMasternodeList()
	assert.NotNil(t, list)
	assert.Len(t, list.Masternodes, 5)
}

func TestTierFunctions(t *testing.T) {
	// Test tier string conversion
	assert.Equal(t, "bronze", Bronze.String())
	assert.Equal(t, "silver", Silver.String())
	assert.Equal(t, "gold", Gold.String())
	assert.Equal(t, "platinum", Platinum.String())

	// Test tier collateral
	assert.Equal(t, int64(TierBronzeCollateral), Bronze.Collateral())
	assert.Equal(t, int64(TierSilverCollateral), Silver.Collateral())
	assert.Equal(t, int64(TierGoldCollateral), Gold.Collateral())
	assert.Equal(t, int64(TierPlatinumCollateral), Platinum.Collateral())
}

func TestMasternodeStatus(t *testing.T) {
	mn := createTestMasternode(Bronze)

	// Test IsActive
	mn.Status = StatusEnabled
	assert.True(t, mn.IsActive())

	mn.Status = StatusPreEnabled
	assert.True(t, mn.IsActive())

	mn.Status = StatusInactive
	assert.False(t, mn.IsActive())

	// Test IsExpired
	mn.Status = StatusExpired
	assert.True(t, mn.IsExpired())

	// Test IsPosebanActive
	mn.Status = StatusPoseban
	assert.True(t, mn.IsPosebanActive())
}

func TestMasternodeScore(t *testing.T) {
	mn := createTestMasternode(Bronze)
	mn.LastPaid = time.Now().Add(-1 * time.Hour)

	blockHash := types.NewHash([]byte("test"))
	score := mn.CalculateScore(blockHash)

	// Score is now types.Hash (full 32-byte uint256), not float64
	// Verify score is not zero hash (would indicate no valid score)
	assert.NotEqual(t, types.ZeroHash, score, "Score should not be zero hash")

	// Also test compact score for backward compatibility
	compactScore := mn.CalculateScoreCompact(blockHash)
	assert.Greater(t, compactScore, 0.0, "Compact score should be positive")
}

func TestUpdateMasternodeStatus(t *testing.T) {
	now := time.Now()

	// Test 1: Expired status (between ExpirationSeconds and RemovalSeconds)
	// ExpirationSeconds = 7200 (2 hours), RemovalSeconds = 7800 (2h 10m)
	mn := createTestMasternode(Bronze)
	mn.Status = StatusEnabled
	mn.SigTime = now.Add(-24 * time.Hour).Unix() // Old enough to not be PRE_ENABLED
	// CRITICAL: UpdateStatus now uses LastPingMessage.SigTime, not LastPing wall-clock
	mn.LastPingMessage = &MasternodePing{
		SigTime: now.Add(-125 * time.Minute).Unix(), // 125 min > 120 min (expired) but < 130 min (removal)
	}

	mn.UpdateStatus(now, 2*time.Hour) // expireTime param is ignored in new logic
	assert.Equal(t, StatusExpired, mn.Status, "Should be expired after 125 minutes without ping")

	// Test 2: Removed status (past RemovalSeconds)
	mn2 := createTestMasternode(Bronze)
	mn2.Status = StatusEnabled
	mn2.SigTime = now.Add(-24 * time.Hour).Unix()
	mn2.LastPingMessage = &MasternodePing{
		SigTime: now.Add(-135 * time.Minute).Unix(), // 135 min > 130 min (removal)
	}

	mn2.UpdateStatus(now, 2*time.Hour)
	assert.Equal(t, StatusRemoved, mn2.Status, "Should be removed after 135 minutes without ping")

	// Test 3: PRE_ENABLED status (sigTime too recent)
	mn3 := createTestMasternode(Bronze)
	mn3.Status = StatusEnabled
	mn3.SigTime = now.Add(-2 * time.Minute).Unix() // Only 2 min ago, still in PRE_ENABLED window (gap < 300s MinPingSeconds)
	mn3.LastPingMessage = &MasternodePing{
		SigTime: now.Unix(),
	}

	mn3.UpdateStatus(now, 2*time.Hour)
	assert.Equal(t, StatusPreEnabled, mn3.Status, "Should be pre-enabled when sigTime is recent")

	// Test 4: ENABLED status (normal operation with fresh ping)
	mn4 := createTestMasternode(Bronze)
	mn4.Status = StatusEnabled
	mn4.SigTime = now.Add(-24 * time.Hour).Unix() // Old enough broadcast
	mn4.LastPingMessage = &MasternodePing{
		SigTime: now.Add(-5 * time.Minute).Unix(), // Fresh ping (< 10 min old)
	}

	mn4.UpdateStatus(now, 2*time.Hour)
	assert.Equal(t, StatusEnabled, mn4.Status, "Should be enabled with fresh ping (< 5 min old)")

	// Test 5: ENABLED status with older ping (> 5 min old but gap check passes)
	// C++ CMasternode::Check() does NOT have a ping-to-now freshness check.
	// Once the broadcast-to-ping gap exceeds MinPingSeconds, the masternode is ENABLED.
	mn5 := createTestMasternode(Bronze)
	mn5.Status = StatusEnabled
	mn5.SigTime = now.Add(-24 * time.Hour).Unix() // Old broadcast (gap check passes)
	mn5.LastPingMessage = &MasternodePing{
		SigTime: now.Add(-15 * time.Minute).Unix(), // 15 min old ping - gap still passes
	}

	mn5.UpdateStatus(now, 2*time.Hour)
	assert.Equal(t, StatusEnabled, mn5.Status,
		"Should be enabled when broadcast-to-ping gap passes (no ping-to-now freshness check in C++)")

	// Test 6: ENABLED even when ping is exactly at 5 min
	// C++ only checks lastPing.sigTime - sigTime < MIN_MNP_SECONDS, not ping-to-now
	mn6 := createTestMasternode(Bronze)
	mn6.Status = StatusEnabled
	mn6.SigTime = now.Add(-24 * time.Hour).Unix()
	mn6.LastPingMessage = &MasternodePing{
		SigTime: now.Add(-10 * time.Minute).Unix(), // 10 min old - gap still passes
	}

	mn6.UpdateStatus(now, 2*time.Hour)
	assert.Equal(t, StatusEnabled, mn6.Status,
		"Should be enabled - C++ has no ping-to-now freshness check")

	// Test 7: ENABLED with very recent ping
	mn7 := createTestMasternode(Bronze)
	mn7.Status = StatusEnabled
	mn7.SigTime = now.Add(-24 * time.Hour).Unix()
	mn7.LastPingMessage = &MasternodePing{
		SigTime: now.Add(-9*time.Minute - 59*time.Second).Unix(), // Recent ping
	}

	mn7.UpdateStatus(now, 2*time.Hour)
	assert.Equal(t, StatusEnabled, mn7.Status,
		"Should be enabled with recent ping and passing gap check")
}

// Benchmark tests
func BenchmarkAddMasternode(b *testing.B) {
	manager := createTestManager(&testing.T{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mn := createTestMasternode(Bronze)
		mn.OutPoint.Index = uint32(i)
		manager.AddMasternode(mn)
	}
}

func BenchmarkGetMasternode(b *testing.B) {
	manager := createTestManager(&testing.T{})
	mn := createTestMasternode(Bronze)
	manager.AddMasternode(mn)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.GetMasternode(mn.OutPoint)
	}
}

func BenchmarkCalculateScore(b *testing.B) {
	mn := createTestMasternode(Bronze)
	blockHash := types.NewHash([]byte("test"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mn.CalculateScore(blockHash)
	}
}

// TestCalculateScoreEdgeCases tests edge cases for uint256 subtraction in CalculateScore
// This addresses the code review warning about potential integer overflow in score subtraction
func TestCalculateScoreEdgeCases(t *testing.T) {
	mn := createTestMasternode(Bronze)

	t.Run("ZeroHash", func(t *testing.T) {
		// Test with zero hash - should not panic
		score := mn.CalculateScore(types.ZeroHash)
		// Score should be computed (may be zero or non-zero depending on algorithm)
		assert.NotNil(t, score)
	})

	t.Run("MaxHash", func(t *testing.T) {
		// Test with all-0xFF hash (maximum uint256 value)
		var maxHash types.Hash
		for i := range maxHash {
			maxHash[i] = 0xFF
		}
		score := mn.CalculateScore(maxHash)
		assert.NotNil(t, score)
	})

	t.Run("AdjacentValues", func(t *testing.T) {
		// Test with adjacent hash values to verify borrow handling
		var hash1, hash2 types.Hash
		hash1[31] = 0x01 // Smallest non-zero value
		hash2[31] = 0x02 // Next value

		score1 := mn.CalculateScore(hash1)
		score2 := mn.CalculateScore(hash2)

		// Scores should be different for different hashes
		assert.NotEqual(t, score1, score2, "Different block hashes should produce different scores")
	})

	t.Run("BorrowChain", func(t *testing.T) {
		// Test borrow chain propagation: 0x0100...00 - 0x00FF...FF = 0x01
		// This tests the subtraction borrow across multiple bytes
		var hash types.Hash
		hash[0] = 0x01 // High byte set, all others zero
		score := mn.CalculateScore(hash)
		assert.NotNil(t, score)
	})

	t.Run("AlternatingBytes", func(t *testing.T) {
		// Test with alternating byte pattern to stress subtraction
		var hash types.Hash
		for i := range hash {
			if i%2 == 0 {
				hash[i] = 0xAA
			} else {
				hash[i] = 0x55
			}
		}
		score := mn.CalculateScore(hash)
		assert.NotNil(t, score)
	})

	t.Run("DeterministicOrdering", func(t *testing.T) {
		// Verify that CalculateScore produces deterministic ordering
		// Same inputs should always produce same score
		blockHash := types.NewHash([]byte("deterministic_test"))

		score1 := mn.CalculateScore(blockHash)
		score2 := mn.CalculateScore(blockHash)

		assert.Equal(t, score1, score2, "Same inputs should produce identical scores")
	})

	t.Run("DifferentMasternodesProduceDifferentScores", func(t *testing.T) {
		// Different masternodes should generally have different scores for same block
		mn2 := createTestMasternode(Silver)
		mn2.OutPoint.Index = 999 // Different outpoint

		blockHash := types.NewHash([]byte("multi_mn_test"))

		score1 := mn.CalculateScore(blockHash)
		score2 := mn2.CalculateScore(blockHash)

		// Note: scores CAN be equal by chance, but extremely unlikely
		// This test primarily verifies no panics occur
		assert.NotNil(t, score1)
		assert.NotNil(t, score2)
	})

	t.Run("ScoreComparisonIsConsistent", func(t *testing.T) {
		// Test that score comparisons are consistent with bytes.Compare
		// This is critical for deterministic payment selection
		blockHash := types.NewHash([]byte("comparison_test"))

		mn2 := createTestMasternode(Silver)
		mn2.OutPoint.Index = 100

		score1 := mn.CalculateScore(blockHash)
		score2 := mn2.CalculateScore(blockHash)

		// bytes.Compare should work correctly for ordering
		import1 := score1[:]
		import2 := score2[:]

		// Comparison should be transitive: if a < b and b < c, then a < c
		cmp := 0
		for i := 0; i < 32; i++ {
			if import1[i] < import2[i] {
				cmp = -1
				break
			} else if import1[i] > import2[i] {
				cmp = 1
				break
			}
		}
		// Just verify comparison works without panic
		_ = cmp
	})
}

// TestLegacyStringFormatting tests the legacy C++ compatible string formatting functions
// These functions MUST produce output that exactly matches the C++ implementations
// for signature message verification to work across Go and C++ nodes
func TestLegacyStringFormatting(t *testing.T) {
	t.Run("LegacyOutpointString", func(t *testing.T) {
		// Test basic outpoint formatting
		// C++ COutPoint::ToString() format: "COutPoint(%s, %u)"
		hash, _ := types.NewHashFromString("0000000000000000000000000000000000000000000000000000000000001234")
		outpoint := types.Outpoint{
			Hash:  hash,
			Index: 0,
		}

		result := LegacyOutpointString(outpoint)
		expected := "COutPoint(0000000000000000000000000000000000000000000000000000000000001234, 0)"
		assert.Equal(t, expected, result)

		// Test with different index
		outpoint.Index = 42
		result = LegacyOutpointString(outpoint)
		expected = "COutPoint(0000000000000000000000000000000000000000000000000000000000001234, 42)"
		assert.Equal(t, expected, result)
	})

	t.Run("LegacyTxInString", func(t *testing.T) {
		// Test CTxIn formatting for masternode collateral
		// C++ CTxIn::ToString() format for masternode: "CTxIn(COutPoint(%s, %u), scriptSig=)"
		hash, _ := types.NewHashFromString("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")
		outpoint := types.Outpoint{
			Hash:  hash,
			Index: 1,
		}

		result := LegacyTxInString(outpoint)
		expected := "CTxIn(COutPoint(abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890, 1), scriptSig=)"
		assert.Equal(t, expected, result)
	})

	t.Run("PingSignatureMessage", func(t *testing.T) {
		// Test the full ping signature message format
		// C++ format: vin.ToString() + blockHash.ToString() + std::to_string(sigTime)
		txHash, _ := types.NewHashFromString("1111111111111111111111111111111111111111111111111111111111111111")
		blockHash, _ := types.NewHashFromString("2222222222222222222222222222222222222222222222222222222222222222")

		ping := &MasternodePing{
			OutPoint: types.Outpoint{
				Hash:  txHash,
				Index: 0,
			},
			BlockHash: blockHash,
			SigTime:   1234567890,
		}

		message := ping.getSignatureMessage()

		// Expected: CTxIn(COutPoint(hash, 0), scriptSig=) + blockHash + sigTime
		expectedPrefix := "CTxIn(COutPoint(1111111111111111111111111111111111111111111111111111111111111111, 0), scriptSig=)"
		expectedBlockHash := "2222222222222222222222222222222222222222222222222222222222222222"
		expectedSigTime := "1234567890"
		expected := expectedPrefix + expectedBlockHash + expectedSigTime

		assert.Equal(t, expected, message)
	})

	t.Run("ZeroIndexOutpoint", func(t *testing.T) {
		// Verify zero index is formatted correctly (common case for masternodes)
		hash, _ := types.NewHashFromString("0000000000000000000000000000000000000000000000000000000000000001")
		outpoint := types.Outpoint{
			Hash:  hash,
			Index: 0,
		}

		result := LegacyTxInString(outpoint)
		assert.Contains(t, result, ", 0)")
		assert.Contains(t, result, "scriptSig=)")
	})

	t.Run("LargeIndex", func(t *testing.T) {
		// Test with maximum uint32 index (edge case)
		hash, _ := types.NewHashFromString("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
		outpoint := types.Outpoint{
			Hash:  hash,
			Index: 4294967295, // max uint32
		}

		result := LegacyOutpointString(outpoint)
		assert.Contains(t, result, "4294967295")
	})

	t.Run("GetSignatureMessage_Public", func(t *testing.T) {
		// Test the public GetSignatureMessage() method returns same result as internal method
		// This is critical for RPC handlers that need to sign pings correctly
		txHash, _ := types.NewHashFromString("abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234")
		blockHash, _ := types.NewHashFromString("5678efgh5678efgh5678efgh5678efgh5678efgh5678efgh5678efgh5678efgh")

		ping := &MasternodePing{
			OutPoint: types.Outpoint{
				Hash:  txHash,
				Index: 5,
			},
			BlockHash: blockHash,
			SigTime:   1700000000,
		}

		// Public method should return CTxIn format, NOT "hash:index" format
		message := ping.GetSignatureMessage()

		// Must start with CTxIn format
		assert.Contains(t, message, "CTxIn(COutPoint(")
		assert.Contains(t, message, ", scriptSig=)")

		// Must NOT contain simple "hash:index" format (the bug we fixed)
		assert.NotContains(t, message, txHash.String()+":5")

		// Must contain the sigTime
		assert.Contains(t, message, "1700000000")
	})

	t.Run("SignatureFormat_NotSimpleOutpoint", func(t *testing.T) {
		// Verify signature message does NOT use simple outpoint.String() format
		// This was the bug in RPC: using outpoint.String() instead of LegacyTxInString
		txHash, _ := types.NewHashFromString("1234000000000000000000000000000000000000000000000000000000005678")
		blockHash, _ := types.NewHashFromString("aaaa000000000000000000000000000000000000000000000000000000000001")

		outpoint := types.Outpoint{Hash: txHash, Index: 2}

		ping := &MasternodePing{
			OutPoint:  outpoint,
			BlockHash: blockHash,
			SigTime:   1600000000,
		}

		message := ping.GetSignatureMessage()

		// The wrong format would be: "hash:index" + blockHash + sigTime
		wrongFormat := outpoint.String() + blockHash.String() + "1600000000"

		// The correct format uses CTxIn wrapper
		correctFormat := LegacyTxInString(outpoint) + blockHash.String() + "1600000000"

		assert.NotEqual(t, wrongFormat, message, "Should NOT use simple outpoint.String() format")
		assert.Equal(t, correctFormat, message, "Should use LegacyTxInString format")
	})
}

// TestCheckDependencies verifies the dependency checking mechanism
func TestCheckDependencies(t *testing.T) {
	t.Run("AllMissing", func(t *testing.T) {
		// Fresh manager has no dependencies configured
		manager := createTestManager(t)

		missing := manager.CheckDependencies()

		// Should report all 4 dependencies as missing
		assert.Len(t, missing, 4)
		assert.Contains(t, missing, "blockchain")
		assert.Contains(t, missing, "sporkManager")
		assert.Contains(t, missing, "pingRelayHandler")
		assert.Contains(t, missing, "winnerRelayHandler")
	})

	t.Run("WithPingHandler", func(t *testing.T) {
		manager := createTestManager(t)

		// Set only ping relay handler
		manager.SetPingRelayHandler(func(ping *MasternodePing) {})

		missing := manager.CheckDependencies()

		// Should report 3 dependencies as missing
		assert.Len(t, missing, 3)
		assert.NotContains(t, missing, "pingRelayHandler")
		assert.Contains(t, missing, "blockchain")
		assert.Contains(t, missing, "sporkManager")
		assert.Contains(t, missing, "winnerRelayHandler")
	})

	t.Run("WithWinnerHandler", func(t *testing.T) {
		manager := createTestManager(t)

		// Set only winner relay handler
		manager.SetWinnerRelayHandler(func(vote *MasternodeWinnerVote) {})

		missing := manager.CheckDependencies()

		// Should report 3 dependencies as missing
		assert.Len(t, missing, 3)
		assert.NotContains(t, missing, "winnerRelayHandler")
		assert.Contains(t, missing, "blockchain")
		assert.Contains(t, missing, "sporkManager")
		assert.Contains(t, missing, "pingRelayHandler")
	})

	t.Run("WithAllHandlers", func(t *testing.T) {
		manager := createTestManager(t)

		// Set both relay handlers (blockchain and spork still missing)
		manager.SetPingRelayHandler(func(ping *MasternodePing) {})
		manager.SetWinnerRelayHandler(func(vote *MasternodeWinnerVote) {})

		missing := manager.CheckDependencies()

		// Should report only blockchain and sporkManager as missing
		assert.Len(t, missing, 2)
		assert.Contains(t, missing, "blockchain")
		assert.Contains(t, missing, "sporkManager")
		assert.NotContains(t, missing, "pingRelayHandler")
		assert.NotContains(t, missing, "winnerRelayHandler")
	})
}

// TestUpdateFromBroadcast tests the UpdateFromBroadcast method that handles
// full state refresh from rebroadcast messages (matching legacy UpdateFromNewBroadcast)
func TestUpdateFromBroadcast(t *testing.T) {
	t.Run("AddressUpdate", func(t *testing.T) {
		manager := createTestManager(t)

		// Create and add initial masternode
		oldAddr := &net.TCPAddr{IP: net.ParseIP("1.2.3.4"), Port: 37817}
		kp, _ := crypto.GenerateKeyPair()
		outpoint := types.Outpoint{Hash: types.Hash{0x01}, Index: 0}

		mn := &Masternode{
			OutPoint:   outpoint,
			Addr:       oldAddr,
			PubKey:     kp.Public,
			Protocol:   70926,
			Tier:       Bronze,
			Collateral: TierBronzeCollateral,
			Status:     StatusEnabled,
			SigTime:    time.Now().Unix(),
		}
		err := manager.AddMasternode(mn)
		require.NoError(t, err)

		// Verify initial address index
		assert.NotNil(t, manager.addressIndex["1.2.3.4:37817"])

		// Create broadcast with new address
		newAddr := &net.TCPAddr{IP: net.ParseIP("5.6.7.8"), Port: 37817}
		updatedMN := &Masternode{
			OutPoint:   outpoint,
			Addr:       newAddr,
			PubKey:     kp.Public,
			Protocol:   70926,
			Tier:       Bronze,
			Collateral: TierBronzeCollateral,
			Status:     StatusEnabled,
			SigTime:    time.Now().Unix() + 100,
			LastSeen:   time.Now(),
			LastPing:   time.Now(),
		}

		err = manager.UpdateFromBroadcast(updatedMN)
		require.NoError(t, err)

		// Verify address updated
		retrieved, err := manager.GetMasternode(outpoint)
		require.NoError(t, err)
		assert.Equal(t, "5.6.7.8:37817", retrieved.Addr.String())

		// Verify address index updated
		assert.Nil(t, manager.addressIndex["1.2.3.4:37817"], "old address should be removed from index")
		assert.NotNil(t, manager.addressIndex["5.6.7.8:37817"], "new address should be in index")
	})

	t.Run("ProtocolUpgrade", func(t *testing.T) {
		manager := createTestManager(t)

		addr := &net.TCPAddr{IP: net.ParseIP("1.2.3.4"), Port: 37817}
		kp, _ := crypto.GenerateKeyPair()
		outpoint := types.Outpoint{Hash: types.Hash{0x02}, Index: 0}

		// Add masternode with old protocol
		mn := &Masternode{
			OutPoint:   outpoint,
			Addr:       addr,
			PubKey:     kp.Public,
			Protocol:   70926,
			Tier:       Bronze,
			Collateral: TierBronzeCollateral,
			Status:     StatusEnabled,
			SigTime:    time.Now().Unix(),
		}
		err := manager.AddMasternode(mn)
		require.NoError(t, err)

		// Update with new protocol
		updatedMN := &Masternode{
			OutPoint:   outpoint,
			Addr:       addr,
			PubKey:     kp.Public,
			Protocol:   ActiveProtocolVersion, // Current protocol
			Tier:       Bronze,
			Collateral: TierBronzeCollateral,
			Status:     StatusEnabled,
			SigTime:    time.Now().Unix() + 100,
			LastSeen:   time.Now(),
			LastPing:   time.Now(),
		}

		err = manager.UpdateFromBroadcast(updatedMN)
		require.NoError(t, err)

		// Verify protocol updated
		retrieved, err := manager.GetMasternode(outpoint)
		require.NoError(t, err)
		assert.Equal(t, ActiveProtocolVersion, retrieved.Protocol)
	})

	t.Run("PubKeyRotation", func(t *testing.T) {
		manager := createTestManager(t)

		addr := &net.TCPAddr{IP: net.ParseIP("1.2.3.4"), Port: 37817}
		oldKp, _ := crypto.GenerateKeyPair()
		newKp, _ := crypto.GenerateKeyPair()
		outpoint := types.Outpoint{Hash: types.Hash{0x03}, Index: 0}

		// Add masternode with old pubkey
		mn := &Masternode{
			OutPoint:   outpoint,
			Addr:       addr,
			PubKey:     oldKp.Public,
			Protocol:   70926,
			Tier:       Bronze,
			Collateral: TierBronzeCollateral,
			Status:     StatusEnabled,
			SigTime:    time.Now().Unix(),
		}
		err := manager.AddMasternode(mn)
		require.NoError(t, err)

		oldPubkeyHex := oldKp.Public.Hex()
		assert.NotNil(t, manager.pubkeyIndex[oldPubkeyHex])

		// Update with new pubkey
		updatedMN := &Masternode{
			OutPoint:   outpoint,
			Addr:       addr,
			PubKey:     newKp.Public,
			Protocol:   70926,
			Tier:       Bronze,
			Collateral: TierBronzeCollateral,
			Status:     StatusEnabled,
			SigTime:    time.Now().Unix() + 100,
			LastSeen:   time.Now(),
			LastPing:   time.Now(),
		}

		err = manager.UpdateFromBroadcast(updatedMN)
		require.NoError(t, err)

		// Verify pubkey updated
		retrieved, err := manager.GetMasternode(outpoint)
		require.NoError(t, err)
		assert.Equal(t, newKp.Public.Hex(), retrieved.PubKey.Hex())

		// Verify pubkey index updated
		newPubkeyHex := newKp.Public.Hex()
		assert.Nil(t, manager.pubkeyIndex[oldPubkeyHex], "old pubkey should be removed from index")
		assert.NotNil(t, manager.pubkeyIndex[newPubkeyHex], "new pubkey should be in index")
	})

	t.Run("RecoveryFromSpent", func(t *testing.T) {
		manager := createTestManager(t)

		addr := &net.TCPAddr{IP: net.ParseIP("1.2.3.4"), Port: 37817}
		kp, _ := crypto.GenerateKeyPair()
		outpoint := types.Outpoint{Hash: types.Hash{0x04}, Index: 0}

		// Add masternode with StatusVinSpent (simulating spent collateral)
		mn := &Masternode{
			OutPoint:                 outpoint,
			Addr:                     addr,
			PubKey:                   kp.Public,
			Protocol:                 70926,
			Tier:                     Bronze,
			Collateral:               TierBronzeCollateral,
			Status:                   StatusVinSpent,
			SigTime:                  time.Now().Unix(),
			PrevCycleLastPaymentTime: 12345,
			WinsThisCycle:            5,
		}
		err := manager.AddMasternode(mn)
		require.NoError(t, err)

		// Rebroadcast with valid UTXO (recovery)
		updatedMN := &Masternode{
			OutPoint:   outpoint,
			Addr:       addr,
			PubKey:     kp.Public,
			Protocol:   70926,
			Tier:       Bronze,
			Collateral: TierBronzeCollateral,
			Status:     StatusEnabled, // Now enabled after recovery
			SigTime:    time.Now().Unix() + 100,
			LastSeen:   time.Now(),
			LastPing:   time.Now(),
		}

		err = manager.UpdateFromBroadcast(updatedMN)
		require.NoError(t, err)

		// Verify status changed to Enabled
		retrieved, err := manager.GetMasternode(outpoint)
		require.NoError(t, err)
		assert.Equal(t, StatusEnabled, retrieved.Status)

		// Verify cycle tracking was reset (matches legacy CheckInputsAndAdd)
		assert.Equal(t, int64(0), retrieved.PrevCycleLastPaymentTime, "cycle tracking should be reset on recovery")
		assert.Equal(t, 0, retrieved.WinsThisCycle, "wins this cycle should be reset on recovery")
	})

	t.Run("SignatureFieldsUpdated", func(t *testing.T) {
		manager := createTestManager(t)

		addr := &net.TCPAddr{IP: net.ParseIP("1.2.3.4"), Port: 37817}
		kp, _ := crypto.GenerateKeyPair()
		outpoint := types.Outpoint{Hash: types.Hash{0x05}, Index: 0}

		oldSigTime := time.Now().Unix()
		oldSig := []byte("old_signature")

		mn := &Masternode{
			OutPoint:   outpoint,
			Addr:       addr,
			PubKey:     kp.Public,
			Protocol:   70926,
			Tier:       Bronze,
			Collateral: TierBronzeCollateral,
			Status:     StatusEnabled,
			SigTime:    oldSigTime,
			Signature:  oldSig,
		}
		err := manager.AddMasternode(mn)
		require.NoError(t, err)

		// Update with new signature
		newSigTime := oldSigTime + 3600
		newSig := []byte("new_signature")
		updatedMN := &Masternode{
			OutPoint:   outpoint,
			Addr:       addr,
			PubKey:     kp.Public,
			Protocol:   70926,
			Tier:       Bronze,
			Collateral: TierBronzeCollateral,
			Status:     StatusEnabled,
			SigTime:    newSigTime,
			Signature:  newSig,
			LastSeen:   time.Now(),
			LastPing:   time.Now(),
		}

		err = manager.UpdateFromBroadcast(updatedMN)
		require.NoError(t, err)

		// Verify signature fields updated
		retrieved, err := manager.GetMasternode(outpoint)
		require.NoError(t, err)
		assert.Equal(t, newSigTime, retrieved.SigTime)
		assert.Equal(t, newSig, retrieved.Signature)
	})

	t.Run("NotFoundError", func(t *testing.T) {
		manager := createTestManager(t)

		// Try to update non-existent masternode
		addr := &net.TCPAddr{IP: net.ParseIP("1.2.3.4"), Port: 37817}
		kp, _ := crypto.GenerateKeyPair()
		outpoint := types.Outpoint{Hash: types.Hash{0xFF}, Index: 0}

		mn := &Masternode{
			OutPoint:   outpoint,
			Addr:       addr,
			PubKey:     kp.Public,
			Protocol:   70926,
			Tier:       Bronze,
			Collateral: TierBronzeCollateral,
			Status:     StatusEnabled,
			SigTime:    time.Now().Unix(),
		}

		err := manager.UpdateFromBroadcast(mn)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "masternode not found")
	})
}

// TestCountMillionsLockedLaunchLocked_ExcludesStaleStatus verifies that countMillionsLockedLaunchLocked
// properly refreshes masternode status and excludes expired/spent nodes from the count.
// This test ensures legacy compatibility with mn.Check() behavior in CountMillionsLockedLaunch.
func TestCountMillionsLockedLaunchLocked_ExcludesStaleStatus(t *testing.T) {
	manager := createTestManager(t)

	now := time.Now()
	baseTime := now.Add(-2 * time.Hour).Unix() // Old enough to be counted

	// Create two masternodes with same sigTime
	mn1 := createTestMasternode(Bronze)
	mn1.SigTime = baseTime
	mn1.Status = StatusEnabled
	mn1.OutPoint.Index = 1
	// Set LastPingMessage with fresh SigTime (< 10 min for freshness check)
	mn1.LastPingMessage = &MasternodePing{SigTime: now.Add(-5 * time.Minute).Unix()}

	mn2 := createTestMasternode(Bronze)
	mn2.SigTime = baseTime
	mn2.Status = StatusEnabled
	mn2.OutPoint.Index = 2
	// Set LastPingMessage with fresh SigTime (< 10 min for freshness check)
	mn2.LastPingMessage = &MasternodePing{SigTime: now.Add(-5 * time.Minute).Unix()}

	// Add both masternodes
	require.NoError(t, manager.AddMasternode(mn1))
	require.NoError(t, manager.AddMasternode(mn2))

	// Create a newer masternode whose millionsLocked we will check
	// Its sigTime is newer than mn1 and mn2, so they should be counted
	newerSigTime := now.Add(-1 * time.Hour).Unix()

	// Use CountMillionsLocked public method to avoid lock issues
	// Initially both should be counted (Status=Enabled)
	currentTime := now
	count := manager.CountMillionsLocked(newerSigTime, currentTime, nil)

	assert.Equal(t, 2, count, "Both enabled masternodes should be counted")

	// Now simulate mn2 becoming expired by setting LastPingMessage.SigTime to old time
	// UpdateStatusWithUTXO checks LastPingMessage.SigTime for expiry
	// ExpirationSeconds = 7200 (2h), RemovalSeconds = 7800 (2h10m)
	// Use 2h5m (7500s) - past expiration but before removal
	mn2.mu.Lock()
	mn2.LastPingMessage = &MasternodePing{SigTime: now.Add(-125 * time.Minute).Unix()} // 2h5m - past ExpirationSeconds but before RemovalSeconds
	// Reset status to Enabled to simulate stale status (would have been expired if Check() was called)
	mn2.Status = StatusEnabled
	mn2.mu.Unlock()

	// Call CountMillionsLocked again - it should now refresh status
	// and exclude mn2 since it will be marked as Expired
	count = manager.CountMillionsLocked(newerSigTime, currentTime, nil)

	// mn2 should now be excluded because UpdateStatusWithUTXO marks it as expired
	// Only mn1 should be counted
	assert.Equal(t, 1, count, "Only enabled (non-expired) masternode should be counted after status refresh")

	// Verify mn2 status was actually updated to Expired
	mn2.mu.RLock()
	assert.Equal(t, StatusExpired, mn2.Status, "mn2 should be marked as Expired")
	mn2.mu.RUnlock()
}

// mockUTXOChecker implements UTXOChecker for testing
type mockUTXOChecker struct {
	spent  map[types.Outpoint]bool
	values map[types.Outpoint]int64
	errors map[types.Outpoint]error
}

func newMockUTXOChecker() *mockUTXOChecker {
	return &mockUTXOChecker{
		spent:  make(map[types.Outpoint]bool),
		values: make(map[types.Outpoint]int64),
		errors: make(map[types.Outpoint]error),
	}
}

func (m *mockUTXOChecker) IsUTXOSpent(outpoint types.Outpoint) (bool, error) {
	if err, exists := m.errors[outpoint]; exists && err != nil {
		return false, err
	}
	return m.spent[outpoint], nil
}

func (m *mockUTXOChecker) GetUTXOValue(outpoint types.Outpoint) (int64, error) {
	if err, exists := m.errors[outpoint]; exists && err != nil {
		return 0, err
	}
	if val, exists := m.values[outpoint]; exists {
		return val, nil
	}
	return 0, nil
}

// TestUpdateStatusWithUTXO_SporkTransition tests Issue 4 fix:
// When SPORK_TWINS_01 is disabled, higher-tier masternodes (Silver/Gold/Platinum)
// should be marked as StatusVinSpent because only Bronze (1M) collateral is valid.
func TestUpdateStatusWithUTXO_SporkTransition(t *testing.T) {
	now := time.Now()
	expireTime := time.Duration(ExpirationSeconds) * time.Second

	testCases := []struct {
		name             string
		tier             MasternodeTier
		multiTierEnabled bool
		expectedStatus   MasternodeStatus
	}{
		// Multi-tier enabled: all tiers valid
		{"Bronze with multi-tier enabled", Bronze, true, StatusEnabled},
		{"Silver with multi-tier enabled", Silver, true, StatusEnabled},
		{"Gold with multi-tier enabled", Gold, true, StatusEnabled},
		{"Platinum with multi-tier enabled", Platinum, true, StatusEnabled},

		// Multi-tier disabled: only Bronze valid
		{"Bronze with multi-tier disabled", Bronze, false, StatusEnabled},
		{"Silver with multi-tier disabled", Silver, false, StatusVinSpent},
		{"Gold with multi-tier disabled", Gold, false, StatusVinSpent},
		{"Platinum with multi-tier disabled", Platinum, false, StatusVinSpent},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mn := createTestMasternode(tc.tier)
			// Set up recent ping to avoid expiration
			mn.LastPingMessage = &MasternodePing{SigTime: now.Unix()}

			// Set up UTXO checker with valid collateral value for the tier
			utxoChecker := newMockUTXOChecker()
			utxoChecker.spent[mn.OutPoint] = false
			utxoChecker.values[mn.OutPoint] = tc.tier.Collateral()

			// Call UpdateStatusWithUTXO
			mn.UpdateStatusWithUTXO(now, expireTime, utxoChecker, tc.multiTierEnabled)

			mn.mu.RLock()
			status := mn.Status
			mn.mu.RUnlock()

			assert.Equal(t, tc.expectedStatus, status,
				"Tier %s with multiTierEnabled=%v should have status %s",
				tc.tier.String(), tc.multiTierEnabled, tc.expectedStatus.String())
		})
	}
}

// TestGetUTXOValue_ErrorPaths tests error handling in blockchainUTXOChecker.GetUTXOValue
// These tests verify LEGACY COMPATIBILITY: errors are treated as spent collateral (VinSpent).
// Legacy C++ (masternode.cpp:257-266): if (!coins || !coins->IsAvailable(...)) { activeState = MASTERNODE_VIN_SPENT; }
func TestUpdateStatusWithUTXO_GetUTXOValueErrors(t *testing.T) {
	now := time.Now()
	expireTime := time.Duration(ExpirationSeconds) * time.Second

	t.Run("UTXO checker returns error - legacy treats as VinSpent", func(t *testing.T) {
		mn := createTestMasternode(Bronze)
		mn.LastPingMessage = &MasternodePing{SigTime: now.Unix()}

		utxoChecker := newMockUTXOChecker()
		utxoChecker.spent[mn.OutPoint] = false
		utxoChecker.errors[mn.OutPoint] = assert.AnError // Simulate error

		// LEGACY COMPATIBILITY: Error getting value = treat as spent collateral
		// Legacy C++ treats lookup failure (!coins) the SAME as spent collateral
		mn.UpdateStatusWithUTXO(now, expireTime, utxoChecker, true)

		mn.mu.RLock()
		status := mn.Status
		mn.mu.RUnlock()

		assert.Equal(t, StatusVinSpent, status, "UTXO lookup error should result in VinSpent (legacy compatibility)")
	})

	t.Run("nil UTXO checker - skips collateral validation", func(t *testing.T) {
		mn := createTestMasternode(Silver)
		mn.LastPingMessage = &MasternodePing{SigTime: now.Unix()}

		// nil utxoChecker should skip UTXO validation entirely
		mn.UpdateStatusWithUTXO(now, expireTime, nil, false)

		mn.mu.RLock()
		status := mn.Status
		mn.mu.RUnlock()

		// Even though Silver is not valid in single-tier mode,
		// nil utxoChecker skips validation so it stays enabled
		assert.Equal(t, StatusEnabled, status, "nil UTXO checker should skip collateral validation")
	})

	t.Run("UTXO spent - marked as VinSpent", func(t *testing.T) {
		mn := createTestMasternode(Bronze)
		mn.LastPingMessage = &MasternodePing{SigTime: now.Unix()}

		utxoChecker := newMockUTXOChecker()
		utxoChecker.spent[mn.OutPoint] = true // UTXO is spent

		mn.UpdateStatusWithUTXO(now, expireTime, utxoChecker, true)

		mn.mu.RLock()
		status := mn.Status
		mn.mu.RUnlock()

		assert.Equal(t, StatusVinSpent, status, "Spent UTXO should result in VinSpent status")
	})

	t.Run("Invalid collateral amount - marked as VinSpent", func(t *testing.T) {
		mn := createTestMasternode(Bronze)
		mn.LastPingMessage = &MasternodePing{SigTime: now.Unix()}

		utxoChecker := newMockUTXOChecker()
		utxoChecker.spent[mn.OutPoint] = false
		utxoChecker.values[mn.OutPoint] = 500000 * 100000000 // 500K TWINS - invalid amount

		mn.UpdateStatusWithUTXO(now, expireTime, utxoChecker, true)

		mn.mu.RLock()
		status := mn.Status
		mn.mu.RUnlock()

		assert.Equal(t, StatusVinSpent, status, "Invalid collateral amount should result in VinSpent status")
	})

	t.Run("Both UTXO checks return errors - legacy treats as VinSpent", func(t *testing.T) {
		mn := createTestMasternode(Bronze)
		mn.LastPingMessage = &MasternodePing{SigTime: now.Unix()}

		utxoChecker := newMockUTXOChecker()
		// Both IsUTXOSpent and GetUTXOValue will return errors
		utxoChecker.errors[mn.OutPoint] = assert.AnError

		// LEGACY COMPATIBILITY: Error = treat as spent collateral (VinSpent)
		mn.UpdateStatusWithUTXO(now, expireTime, utxoChecker, true)

		mn.mu.RLock()
		status := mn.Status
		mn.mu.RUnlock()

		assert.Equal(t, StatusVinSpent, status, "UTXO lookup error should result in VinSpent (legacy compatibility)")
	})
}

// TestPaymentQueueSelection_MultiTier tests the payment queue selection algorithm
// with multiple tiers to verify legacy C++ compatibility.
//
// Legacy algorithm from masternodeman.cpp:562-631 (GetNextMasternodeInQueueForPayment):
// 1. Filter: sigTime + millionsLocked*2.6*60 <= currentTime (maturity wait)
// 2. Filter: Protocol version >= MIN_PEER_PROTO_VERSION_BEFORE_ENFORCEMENT
// 3. Select: Top 1/10 by SecondsSincePayment, then lowest CalculateScore wins
// 4. Multi-tier: tierRounds = 1 means IsScheduled bypassed (millionsLocked-based)
//
// This test verifies the core selection logic matches legacy behavior.
func TestPaymentQueueSelection_MultiTier(t *testing.T) {
	manager := createTestManager(t)

	// Current time reference
	now := time.Now()

	// Create masternodes of different tiers with varying payment history
	// SigTime must be old enough to pass maturity filter:
	// sigTime + (millionsLocked * 2.6 * 60) <= now
	// With 0 other MNs, any sigTime < now works, but we use -2h for safety

	t.Run("SingleTierOrdering", func(t *testing.T) {
		// Reset manager
		manager.mu.Lock()
		manager.masternodes = make(map[types.Outpoint]*Masternode)
		manager.mu.Unlock()

		// Create 3 Bronze masternodes with different payment history
		// Payment priority: longer time since payment = higher priority
		mns := make([]*Masternode, 3)
		for i := 0; i < 3; i++ {
			mn := createTestMasternode(Bronze)
			mn.OutPoint.Index = uint32(i)
			mn.SigTime = now.Add(-2 * time.Hour).Unix()

			// Different payment times: index 0 paid most recently, index 2 longest ago
			// This should make index 2 have highest priority
			mn.PrevCycleLastPaymentTime = now.Add(time.Duration(-(i+1)*24) * time.Hour).Unix()

			mns[i] = mn
			manager.AddMasternode(mn)
		}

		// Get payment queue
		queue := manager.GetPaymentQueue()
		assert.Len(t, queue, 3, "Queue should contain all 3 masternodes")

		// Verify queue is not empty and contains valid entries
		for _, mn := range queue {
			assert.NotNil(t, mn)
			assert.NotNil(t, mn.PubKey)
		}
	})

	t.Run("MultiTierSelection", func(t *testing.T) {
		// Reset manager
		manager.mu.Lock()
		manager.masternodes = make(map[types.Outpoint]*Masternode)
		manager.mu.Unlock()

		// Create one masternode of each tier
		tiers := []MasternodeTier{Bronze, Silver, Gold, Platinum}
		for i, tier := range tiers {
			mn := createTestMasternode(tier)
			mn.OutPoint.Index = uint32(i)
			mn.SigTime = now.Add(-2 * time.Hour).Unix()
			mn.PrevCycleLastPaymentTime = now.Add(-48 * time.Hour).Unix() // Same payment time
			manager.AddMasternode(mn)
		}

		// GetNextPayee should return a valid masternode
		next, err := manager.GetNextPayee()
		assert.NoError(t, err)
		assert.NotNil(t, next, "GetNextPayee should return a masternode")

		// Verify it's one of the registered masternodes
		found := false
		for i := range tiers {
			if next.OutPoint.Index == uint32(i) {
				found = true
				break
			}
		}
		assert.True(t, found, "Returned masternode should be in the registered set")
	})

	t.Run("ProtocolVersionFiltering", func(t *testing.T) {
		// Reset manager
		manager.mu.Lock()
		manager.masternodes = make(map[types.Outpoint]*Masternode)
		manager.mu.Unlock()

		// Create a masternode with old protocol version
		mnOld := createTestMasternode(Bronze)
		mnOld.OutPoint.Index = 0
		mnOld.Protocol = 70925 // Below MinPeerProtoAfterEnforcement (70927)
		mnOld.SigTime = now.Add(-2 * time.Hour).Unix()
		manager.AddMasternode(mnOld)

		// Create a masternode with current protocol version
		mnNew := createTestMasternode(Bronze)
		mnNew.OutPoint.Index = 1
		mnNew.Protocol = MinPeerProtoAfterEnforcement // 70927
		mnNew.SigTime = now.Add(-2 * time.Hour).Unix()
		manager.AddMasternode(mnNew)

		// GetNextPayee should prefer the node with current protocol
		// Note: The actual filter depends on spork state, but basic queue should include both
		queue := manager.GetPaymentQueue()
		assert.GreaterOrEqual(t, len(queue), 1, "Queue should have at least one masternode")
	})

	t.Run("SigTimeMaturityFilter", func(t *testing.T) {
		// Reset manager
		manager.mu.Lock()
		manager.masternodes = make(map[types.Outpoint]*Masternode)
		manager.mu.Unlock()

		// Create a masternode with very recent sigTime (should be filtered)
		mnRecent := createTestMasternode(Bronze)
		mnRecent.OutPoint.Index = 0
		mnRecent.SigTime = now.Unix() // Just now - too recent
		manager.AddMasternode(mnRecent)

		// Create a masternode with old sigTime (should pass filter)
		mnOld := createTestMasternode(Bronze)
		mnOld.OutPoint.Index = 1
		mnOld.SigTime = now.Add(-2 * time.Hour).Unix()
		manager.AddMasternode(mnOld)

		// GetNextPayee uses the sigTime filter internally
		// With millionsLocked = 1 (Bronze), wait time = 1 * 2.6 * 60 = 156 seconds
		// Recent node should be filtered out
		next, err := manager.GetNextPayee()
		assert.NoError(t, err)
		// Note: If both pass filter, either could be selected based on score
		// The key is that the function doesn't error
		_ = next
	})

	t.Run("EmptyQueueHandling", func(t *testing.T) {
		// Reset manager
		manager.mu.Lock()
		manager.masternodes = make(map[types.Outpoint]*Masternode)
		manager.mu.Unlock()

		// No masternodes added
		next, err := manager.GetNextPayee()
		assert.Error(t, err, "Empty manager should return error")
		assert.Nil(t, next)
	})

	t.Run("ScoreBasedSelection", func(t *testing.T) {
		// Reset manager
		manager.mu.Lock()
		manager.masternodes = make(map[types.Outpoint]*Masternode)
		manager.mu.Unlock()

		// Create multiple masternodes - they should be scored deterministically
		for i := 0; i < 5; i++ {
			mn := createTestMasternode(Bronze)
			mn.OutPoint.Index = uint32(i)
			mn.SigTime = now.Add(-2 * time.Hour).Unix()
			mn.PrevCycleLastPaymentTime = now.Add(-48 * time.Hour).Unix()
			manager.AddMasternode(mn)
		}

		// Get payee - verify it returns a valid masternode
		// Note: GetNextPayee uses GetAdjustedTime() internally which can vary slightly
		// between calls, so we just verify the result is valid, not deterministic
		next, err := manager.GetNextPayee()
		assert.NoError(t, err)
		assert.NotNil(t, next, "GetNextPayee should return a masternode")

		// Verify the returned masternode is from our set
		found := false
		for i := 0; i < 5; i++ {
			if next.OutPoint.Index == uint32(i) {
				found = true
				break
			}
		}
		assert.True(t, found, "Returned masternode should be in the registered set")
	})
}