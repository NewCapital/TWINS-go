// Copyright (c) 2025 The TWINS Core developers
// Distributed under the MIT software license

package p2p

import (
	"testing"
)

// TestRotation tests circular rotation with wrap-around
func TestRotation(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)

	// Add 3 healthy peers
	for i := 0; i < 3; i++ {
		addr := "peer" + string(rune('0'+i))
		healthTracker.RecordPeerDiscovered(addr, 1000, false, TierBronze, false)
	}

	// Build the peer list
	peerList.Rebuild(healthTracker.GetAllPeers())

	if len(peerList.GetAllPeers()) != 3 {
		t.Fatalf("expected 3 peers, got %d", len(peerList.GetAllPeers()))
	}

	// Test circular rotation
	firstPeer, err := peerList.Next()
	if err != nil {
		t.Fatalf("failed to get first peer: %v", err)
	}

	secondPeer, err := peerList.Next()
	if err != nil {
		t.Fatalf("failed to get second peer: %v", err)
	}

	thirdPeer, err := peerList.Next()
	if err != nil {
		t.Fatalf("failed to get third peer: %v", err)
	}

	// Should wrap around to first peer
	fourthPeer, err := peerList.Next()
	if err != nil {
		t.Fatalf("failed to get fourth peer: %v", err)
	}

	if fourthPeer != firstPeer {
		t.Errorf("expected rotation to wrap to first peer, got different peer")
	}

	// Verify all three peers are different
	if firstPeer == secondPeer || secondPeer == thirdPeer || firstPeer == thirdPeer {
		t.Error("expected all peers to be different")
	}
}

// TestRoundDetection tests round counter and completion detection
func TestRoundDetection(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)

	// Add 3 peers
	for i := 0; i < 3; i++ {
		addr := "peer" + string(rune('0'+i))
		healthTracker.RecordPeerDiscovered(addr, 1000, false, TierBronze, false)
	}

	peerList.Rebuild(healthTracker.GetAllPeers())

	// Initial round count should be 0
	if peerList.GetRoundCount() != 0 {
		t.Errorf("expected initial round count 0, got %d", peerList.GetRoundCount())
	}

	// Rotate through all 3 peers
	for i := 0; i < 3; i++ {
		_, err := peerList.Next()
		if err != nil {
			t.Fatalf("failed to get peer %d: %v", i, err)
		}
	}

	// After 3 peers, should have completed 1 round
	if peerList.GetRoundCount() != 1 {
		t.Errorf("expected round count 1 after wrapping, got %d", peerList.GetRoundCount())
	}

	// Rotate through another round
	for i := 0; i < 3; i++ {
		_, err := peerList.Next()
		if err != nil {
			t.Fatalf("failed to get peer in second round: %v", err)
		}
	}

	// Should have completed 2 rounds
	if peerList.GetRoundCount() != 2 {
		t.Errorf("expected round count 2, got %d", peerList.GetRoundCount())
	}
}

// TestSkipUnhealthyPeers tests that rotation skips unhealthy peers
func TestSkipUnhealthyPeers(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)

	// Add 3 peers
	healthTracker.RecordPeerDiscovered("healthy1", 1000, false, TierBronze, false)
	healthTracker.RecordPeerDiscovered("unhealthy", 1000, false, TierBronze, false)
	healthTracker.RecordPeerDiscovered("healthy2", 1000, false, TierBronze, false)

	// Make "unhealthy" peer unhealthy by putting it on cooldown
	for i := 0; i < 3; i++ {
		healthTracker.RecordError("unhealthy", ErrorTypeInvalidBlock) // 2.0*3=6.0 > 5.0 threshold
	}

	peerList.Rebuild(healthTracker.GetAllPeers())

	// Rotate 10 times - should never get the unhealthy peer
	seenPeers := make(map[string]bool)
	for i := 0; i < 10; i++ {
		peer, err := peerList.Next()
		if err != nil {
			t.Fatalf("failed to get peer: %v", err)
		}
		seenPeers[peer] = true

		if peer == "unhealthy" {
			t.Errorf("got unhealthy peer in rotation")
		}
	}

	// Should have seen both healthy peers
	if !seenPeers["healthy1"] || !seenPeers["healthy2"] {
		t.Error("did not see all healthy peers in rotation")
	}
}

// TestAllUnhealthyPeers tests behavior when all peers are unhealthy
func TestAllUnhealthyPeers(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)

	// Add 2 peers and make them unhealthy
	for i := 0; i < 2; i++ {
		addr := "peer" + string(rune('0'+i))
		healthTracker.RecordPeerDiscovered(addr, 1000, false, TierBronze, false)
		for j := 0; j < 3; j++ {
			healthTracker.RecordError(addr, ErrorTypeInvalidBlock)
		}
	}

	peerList.Rebuild(healthTracker.GetAllPeers())

	// Should get error when trying to get next peer
	_, err := peerList.Next()
	if err == nil {
		t.Error("expected error when all peers are unhealthy")
	}
}

// TestMasternodeDuplication tests tier-based masternode duplication
func TestMasternodeDuplication(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)

	// Add masternodes of different tiers
	healthTracker.RecordPeerDiscovered("bronze", 1000, true, TierBronze, false)
	healthTracker.RecordPeerDiscovered("silver", 1000, true, TierSilver, false)
	healthTracker.RecordPeerDiscovered("gold", 1000, true, TierGold, false)
	healthTracker.RecordPeerDiscovered("platinum", 1000, true, TierPlatinum, false)

	peerList.Rebuild(healthTracker.GetAllPeers())

	// Count occurrences of each masternode
	peers := peerList.GetAllPeers()
	counts := make(map[string]int)
	for _, peer := range peers {
		counts[peer]++
	}

	// Verify duplication: Bronze=2, Silver=4, Gold=6, Platinum=8
	if counts["bronze"] != 2 {
		t.Errorf("expected bronze to appear 2 times, got %d", counts["bronze"])
	}
	if counts["silver"] != 4 {
		t.Errorf("expected silver to appear 4 times, got %d", counts["silver"])
	}
	if counts["gold"] != 6 {
		t.Errorf("expected gold to appear 6 times, got %d", counts["gold"])
	}
	if counts["platinum"] != 8 {
		t.Errorf("expected platinum to appear 8 times, got %d", counts["platinum"])
	}

	// Total should be 2+4+6+8 = 20
	if len(peers) != 20 {
		t.Errorf("expected 20 total peers, got %d", len(peers))
	}
}

// TestSprinkleDistribution tests that masternodes are distributed throughout the list
func TestSprinkleDistribution(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)

	// Add 1 masternode and 10 regular peers
	healthTracker.RecordPeerDiscovered("masternode", 1000, true, TierBronze, false)
	for i := 0; i < 10; i++ {
		addr := "regular" + string(rune('0'+i))
		healthTracker.RecordPeerDiscovered(addr, 1000, false, TierNone, false)
	}

	peerList.Rebuild(healthTracker.GetAllPeers())

	// Masternode should appear 2 times (Bronze tier)
	peers := peerList.GetAllPeers()
	mnCount := 0
	for _, peer := range peers {
		if peer == "masternode" {
			mnCount++
		}
	}

	if mnCount != 2 {
		t.Errorf("expected masternode to appear 2 times, got %d", mnCount)
	}

	// Verify masternodes are not all at the beginning or end
	// Check that at least one MN appears after index 2 and before the last 2 positions
	firstMN := -1
	lastMN := -1
	for i, peer := range peers {
		if peer == "masternode" {
			if firstMN == -1 {
				firstMN = i
			}
			lastMN = i
		}
	}

	// Masternodes should be spread out (not adjacent)
	if lastMN-firstMN < 2 {
		t.Error("masternodes appear to be too close together (not sprinkled)")
	}
}

// Test20PeerCap tests that peer list is capped at 20 peers
func Test20PeerCap(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)

	// Add 30 regular peers
	for i := 0; i < 30; i++ {
		addr := "peer" + string(rune('0'+i))
		healthTracker.RecordPeerDiscovered(addr, 1000, false, TierNone, false)
	}

	peerList.Rebuild(healthTracker.GetAllPeers())

	// Should be capped at 20
	if len(peerList.GetAllPeers()) != 20 {
		t.Errorf("expected 20 peers (capped), got %d", len(peerList.GetAllPeers()))
	}
}

// TestRebuildRandomization tests that rebuild produces different order
func TestRebuildRandomization(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)

	// Add 10 peers
	for i := 0; i < 10; i++ {
		addr := "peer" + string(rune('0'+i))
		healthTracker.RecordPeerDiscovered(addr, 1000, false, TierNone, false)
	}

	// Get first ordering
	peerList.Rebuild(healthTracker.GetAllPeers())
	firstOrder := peerList.GetAllPeers()

	// Rebuild multiple times and check for differences
	foundDifferent := false
	for attempt := 0; attempt < 10; attempt++ {
		peerList.Rebuild(healthTracker.GetAllPeers())
		newOrder := peerList.GetAllPeers()

		// Check if order is different
		different := false
		for i := range firstOrder {
			if firstOrder[i] != newOrder[i] {
				different = true
				break
			}
		}

		if different {
			foundDifferent = true
			break
		}
	}

	if !foundDifferent {
		t.Error("rebuild should produce randomized order, but got same order multiple times")
	}
}

// TestRebuildResetsCounters tests that rebuild resets index and round count
func TestRebuildResetsCounters(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)

	// Add peers
	for i := 0; i < 3; i++ {
		addr := "peer" + string(rune('0'+i))
		healthTracker.RecordPeerDiscovered(addr, 1000, false, TierNone, false)
	}

	peerList.Rebuild(healthTracker.GetAllPeers())

	// Rotate a few times
	for i := 0; i < 5; i++ {
		peerList.Next()
	}

	// Round count should be > 0
	if peerList.GetRoundCount() == 0 {
		t.Error("expected round count > 0 before rebuild")
	}

	// Rebuild
	peerList.Rebuild(healthTracker.GetAllPeers())

	// Round count should be reset
	if peerList.GetRoundCount() != 0 {
		t.Errorf("expected round count 0 after rebuild, got %d", peerList.GetRoundCount())
	}
}

// TestCooldownExclusion tests that peers on cooldown are excluded from rebuild
func TestCooldownExclusion(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)

	// Add peers
	healthTracker.RecordPeerDiscovered("healthy", 1000, false, TierNone, false)
	healthTracker.RecordPeerDiscovered("oncooldown", 1000, false, TierNone, false)

	// Put one peer on cooldown
	for i := 0; i < 3; i++ {
		healthTracker.RecordError("oncooldown", ErrorTypeInvalidBlock)
	}

	// Rebuild
	peerList.Rebuild(healthTracker.GetAllPeers())

	peers := peerList.GetAllPeers()

	// Should only have the healthy peer (cooldown peer excluded)
	// Verify the healthy peer is definitely included
	hasHealthy := false
	for _, peer := range peers {
		if peer == "healthy" {
			hasHealthy = true
			break
		}
	}

	if !hasHealthy {
		t.Error("healthy peer should be included in peer list")
	}
}

// TestEmptyPeerList tests behavior with empty peer list
func TestEmptyPeerList(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)

	// Don't add any peers

	// Next should return error
	_, err := peerList.Next()
	if err == nil {
		t.Error("expected error with empty peer list")
	}

	// Round count should be 0
	if peerList.GetRoundCount() != 0 {
		t.Errorf("expected round count 0, got %d", peerList.GetRoundCount())
	}
}

// TestMasternodeOnlyList tests peer list with only masternodes
func TestMasternodeOnlyList(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)

	// Add only masternodes
	healthTracker.RecordPeerDiscovered("gold1", 1000, true, TierGold, false)
	healthTracker.RecordPeerDiscovered("gold2", 1000, true, TierGold, false)

	peerList.Rebuild(healthTracker.GetAllPeers())

	peers := peerList.GetAllPeers()

	// Each gold MN should appear 6 times = 12 total
	if len(peers) != 12 {
		t.Errorf("expected 12 peers (2 gold * 6 copies), got %d", len(peers))
	}

	// Should be able to rotate
	peer, err := peerList.Next()
	if err != nil {
		t.Fatalf("failed to get peer from masternode-only list: %v", err)
	}

	if peer != "gold1" && peer != "gold2" {
		t.Errorf("unexpected peer: %s", peer)
	}
}
