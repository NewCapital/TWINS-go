// Copyright (c) 2025 The TWINS Core developers
// Distributed under the MIT software license

//go:build integration

package p2p

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

// TestBootstrapPhase tests bootstrap phase timing and peer discovery
func TestBootstrapPhase(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)
	consensusValidator := NewConsensusValidator(healthTracker)
	logger := logrus.New().WithField("test", "bootstrap")

	stateMachine := NewSyncStateMachine(peerList, healthTracker, consensusValidator, logger)

	// Initially should be in BOOTSTRAP state
	if stateMachine.GetState() != StateBootstrap {
		t.Errorf("expected initial state BOOTSTRAP, got %v", stateMachine.GetState())
	}

	// Add some peers during bootstrap
	for i := 0; i < 3; i++ {
		addr := "peer" + string(rune('0'+i))
		healthTracker.RecordPeerDiscovered(addr, 1000, false, TierBronze, true)
	}

	// Rebuild peer list
	peerList.Rebuild(healthTracker.GetAllPeers())

	// Should have 3 peers
	if len(peerList.GetAllPeers()) != 3 {
		t.Errorf("expected 3 peers after bootstrap, got %d", len(peerList.GetAllPeers()))
	}

	// Transition to SYNC_DECISION
	err := stateMachine.Transition(StateSyncDecision)
	if err != nil {
		t.Fatalf("failed to transition to SYNC_DECISION: %v", err)
	}

	if stateMachine.GetState() != StateSyncDecision {
		t.Errorf("expected state SYNC_DECISION after transition, got %v", stateMachine.GetState())
	}
}

// TestPeerRotationOnError tests peer rotation when errors occur
func TestPeerRotationOnError(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)

	// Add 5 peers
	for i := 0; i < 5; i++ {
		addr := "peer" + string(rune('0'+i))
		healthTracker.RecordPeerDiscovered(addr, 1000, false, TierBronze, true)
	}

	peerList.Rebuild(healthTracker.GetAllPeers())

	// Get first peer
	peer1, err := peerList.Next()
	if err != nil {
		t.Fatalf("failed to get first peer: %v", err)
	}

	// Record error for first peer
	healthTracker.RecordError(peer1, ErrorTypeInvalidBlock)
	healthTracker.RecordError(peer1, ErrorTypeInvalidBlock)
	healthTracker.RecordError(peer1, ErrorTypeInvalidBlock) // 3 * 2.0 = 6.0 > 5.0 threshold

	// Get next peer - should skip the unhealthy one
	peer2, err := peerList.Next()
	if err != nil {
		t.Fatalf("failed to get second peer: %v", err)
	}

	// Should not be the same as the unhealthy peer
	if peer2 == peer1 {
		t.Error("rotation should skip unhealthy peer but got same peer")
	}

	// Verify peer1 is unhealthy
	if healthTracker.IsHealthy(peer1) {
		t.Error("peer1 should be unhealthy after errors")
	}

	// Verify peer2 is healthy
	if !healthTracker.IsHealthy(peer2) {
		t.Error("peer2 should be healthy")
	}
}

// TestPeerRotationOnBatchComplete tests rotation after batch completion
func TestPeerRotationOnBatchComplete(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)

	// Add 3 peers
	for i := 0; i < 3; i++ {
		addr := "peer" + string(rune('0'+i))
		healthTracker.RecordPeerDiscovered(addr, 1000, false, TierBronze, true)
	}

	peerList.Rebuild(healthTracker.GetAllPeers())

	// Simulate batch completion: get peer, process blocks, record success, get next
	peer1, _ := peerList.Next()
	healthTracker.RecordSuccess(peer1, 10, 10000, time.Second)

	peer2, _ := peerList.Next()
	if peer2 == peer1 {
		// This is OK in small peer sets with circular rotation
		t.Log("Got same peer again (circular rotation)")
	}

	// After a full round, should have rotated through all peers
	seenPeers := make(map[string]bool)
	for i := 0; i < 6; i++ { // 2 full rounds
		peer, err := peerList.Next()
		if err != nil {
			t.Fatalf("failed to get peer in round: %v", err)
		}
		seenPeers[peer] = true
	}

	// Should have seen all 3 peers
	if len(seenPeers) != 3 {
		t.Errorf("expected to see 3 different peers, got %d", len(seenPeers))
	}

	// Should have completed 2 rounds
	if peerList.GetRoundCount() != 2 {
		t.Errorf("expected 2 completed rounds, got %d", peerList.GetRoundCount())
	}
}

// TestAllPeersUnhealthyRecovery tests recovery when all peers become unhealthy
func TestAllPeersUnhealthyRecovery(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)

	// Add 3 peers
	for i := 0; i < 3; i++ {
		addr := "peer" + string(rune('0'+i))
		healthTracker.RecordPeerDiscovered(addr, 1000, false, TierBronze, true)
	}

	peerList.Rebuild(healthTracker.GetAllPeers())

	// Make all peers unhealthy
	for _, peer := range peerList.GetAllPeers() {
		for j := 0; j < 3; j++ {
			healthTracker.RecordError(peer, ErrorTypeInvalidBlock)
		}
	}

	// Trying to get next peer should fail
	_, err := peerList.Next()
	if err == nil {
		t.Error("expected error when all peers are unhealthy")
	}

	// Simulate recovery: add new healthy peer
	healthTracker.RecordPeerDiscovered("recovery_peer", 1000, false, TierBronze, true)
	peerList.Rebuild(healthTracker.GetAllPeers())

	// Should be able to get the new peer
	peer, err := peerList.Next()
	if err != nil {
		t.Fatalf("failed to get peer after recovery: %v", err)
	}

	if peer != "recovery_peer" {
		t.Errorf("expected to get recovery_peer, got %s", peer)
	}
}

// TestHeightReEvaluationEvery3Rounds tests that consensus height is re-evaluated every 3 rounds
func TestHeightReEvaluationEvery3Rounds(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)
	consensusValidator := NewConsensusValidator(healthTracker)
	logger := logrus.New().WithField("test", "reevaluation")

	stateMachine := NewSyncStateMachine(peerList, healthTracker, consensusValidator, logger)

	// Add 5 peers with initial height 1000
	for i := 0; i < 5; i++ {
		addr := "peer" + string(rune('0'+i))
		healthTracker.RecordPeerDiscovered(addr, 1000, false, TierBronze, true)
	}

	peerList.Rebuild(healthTracker.GetAllPeers())

	// Complete 3 rounds
	for round := 0; round < 3; round++ {
		for i := 0; i < 5; i++ {
			_, err := peerList.Next()
			if err != nil {
				t.Fatalf("failed to get peer in round %d: %v", round, err)
			}
		}
	}

	// Should have completed 3 rounds
	if peerList.GetRoundCount() != 3 {
		t.Errorf("expected 3 rounds, got %d", peerList.GetRoundCount())
	}

	// Now update peer heights
	allPeers := peerList.GetAllPeers()
	for _, peer := range allPeers {
		healthTracker.UpdateTipHeight(peer, 1500)
	}

	// Trigger re-evaluation (in real system, this happens automatically)
	result, err := consensusValidator.CalculateConsensusHeight()
	if err != nil {
		t.Fatalf("failed to calculate consensus height: %v", err)
	}

	if result.Height != 1500 {
		t.Errorf("expected consensus height 1500 after re-evaluation, got %d", result.Height)
	}

	// State machine should detect the need for sync
	currentHeight := uint32(1000)
	targetState, err := stateMachine.EvaluateSyncNeeded(currentHeight, result.Height)
	if err != nil {
		t.Fatalf("failed to evaluate sync needed: %v", err)
	}

	// 500 blocks behind should trigger REGULAR_SYNC
	if targetState != StateRegularSync {
		t.Errorf("expected REGULAR_SYNC state for 500 blocks behind, got %v", targetState)
	}
}

// TestStateTransitionsIBDToSynced tests full state transition sequence
func TestStateTransitionsIBDToSynced(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)
	consensusValidator := NewConsensusValidator(healthTracker)
	logger := logrus.New().WithField("test", "transitions")

	stateMachine := NewSyncStateMachine(peerList, healthTracker, consensusValidator, logger)

	// Add peers
	for i := 0; i < 5; i++ {
		addr := "peer" + string(rune('0'+i))
		healthTracker.RecordPeerDiscovered(addr, 10000, false, TierBronze, true)
	}

	peerList.Rebuild(healthTracker.GetAllPeers())

	// State flow: BOOTSTRAP → SYNC_DECISION → IBD → REGULAR_SYNC → SYNCED

	// Step 1: BOOTSTRAP → SYNC_DECISION
	if err := stateMachine.Transition(StateSyncDecision); err != nil {
		t.Fatalf("failed to transition to SYNC_DECISION: %v", err)
	}

	// Step 2: Evaluate sync needed (far behind = IBD)
	currentHeight := uint32(0)
	consensusHeight := uint32(10000)
	targetState, _ := stateMachine.EvaluateSyncNeeded(currentHeight, consensusHeight)

	if targetState != StateIBD {
		t.Errorf("expected IBD for 10000 blocks behind, got %v", targetState)
	}

	// Step 3: SYNC_DECISION → IBD
	if err := stateMachine.Transition(StateIBD); err != nil {
		t.Fatalf("failed to transition to IBD: %v", err)
	}

	// Simulate catching up
	currentHeight = 9000

	// Step 4: Re-evaluate (close = REGULAR_SYNC)
	targetState, _ = stateMachine.EvaluateSyncNeeded(currentHeight, consensusHeight)
	if targetState != StateRegularSync {
		t.Errorf("expected REGULAR_SYNC for 1000 blocks behind, got %v", targetState)
	}

	// Step 5: IBD → REGULAR_SYNC
	if err := stateMachine.Transition(StateRegularSync); err != nil {
		t.Fatalf("failed to transition to REGULAR_SYNC: %v", err)
	}

	// Simulate final catch-up
	currentHeight = 10000

	// Step 6: Re-evaluate (synced = SYNCED)
	targetState, _ = stateMachine.EvaluateSyncNeeded(currentHeight, consensusHeight)
	if targetState != StateSynced {
		t.Errorf("expected SYNCED when caught up, got %v", targetState)
	}

	// Step 7: REGULAR_SYNC → SYNCED
	if err := stateMachine.Transition(StateSynced); err != nil {
		t.Fatalf("failed to transition to SYNCED: %v", err)
	}

	if stateMachine.GetState() != StateSynced {
		t.Errorf("expected final state SYNCED, got %v", stateMachine.GetState())
	}
}

// TestReorgHandling tests reorg detection and handling
func TestReorgHandling(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)
	consensusValidator := NewConsensusValidator(healthTracker)
	logger := logrus.New().WithField("test", "reorg")

	stateMachine := NewSyncStateMachine(peerList, healthTracker, consensusValidator, logger)

	// First reorg should auto-execute
	autoExecute, err := stateMachine.HandleReorg()
	if err != nil {
		t.Fatalf("failed to handle first reorg: %v", err)
	}

	if !autoExecute {
		t.Error("first reorg should auto-execute")
	}

	// Second reorg within window should pause (returns error)
	autoExecute, err = stateMachine.HandleReorg()
	if err == nil {
		t.Error("expected error when sync is paused for manual review")
	}

	// autoExecute should be false when error is returned
	if autoExecute {
		t.Error("second reorg within window should not auto-execute")
	}

	// Confirm and resume
	stateMachine.ResumeAfterReorg()

	// Wait for window to expire (simulate with time manipulation)
	stateMachine.lastReorgTime = time.Now().Add(-2 * time.Hour)

	// Third reorg after window should auto-execute again
	autoExecute, err = stateMachine.HandleReorg()
	if err != nil {
		t.Fatalf("failed to handle third reorg: %v", err)
	}

	if !autoExecute {
		t.Error("reorg after window expiry should auto-execute")
	}
}

// TestMasternodePriorityRotation tests that masternodes get priority in rotation
func TestMasternodePriorityRotation(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)

	// Add 2 regular peers and 1 gold masternode
	healthTracker.RecordPeerDiscovered("regular1", 1000, false, TierNone, true)
	healthTracker.RecordPeerDiscovered("regular2", 1000, false, TierNone, true)
	healthTracker.RecordPeerDiscovered("gold_mn", 1000, true, TierGold, true)

	peerList.Rebuild(healthTracker.GetAllPeers())

	peers := peerList.GetAllPeers()

	// Count occurrences
	counts := make(map[string]int)
	for _, peer := range peers {
		counts[peer]++
	}

	// Gold masternode should appear 6 times (tier duplication)
	if counts["gold_mn"] != 6 {
		t.Errorf("expected gold masternode to appear 6 times, got %d", counts["gold_mn"])
	}

	// Regular peers should appear 1 time each
	if counts["regular1"] != 1 {
		t.Errorf("expected regular1 to appear 1 time, got %d", counts["regular1"])
	}
	if counts["regular2"] != 1 {
		t.Errorf("expected regular2 to appear 1 time, got %d", counts["regular2"])
	}

	// Total should be 8 (2 regular + 6 masternode)
	if len(peers) != 8 {
		t.Errorf("expected 8 total peers, got %d", len(peers))
	}

	// Verify masternode appears more frequently in rotation
	mnCount := 0
	regularCount := 0
	for i := 0; i < 24; i++ { // 3 full rounds
		peer, _ := peerList.Next()
		if peer == "gold_mn" {
			mnCount++
		} else {
			regularCount++
		}
	}

	// Masternode should appear ~18 times (6/8 of 24)
	if mnCount < 15 || mnCount > 21 {
		t.Errorf("expected masternode to appear ~18 times, got %d", mnCount)
	}
}

// TestConsensusFailureRecovery tests recovery when consensus cannot be reached
func TestConsensusFailureRecovery(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	consensusValidator := NewConsensusValidator(healthTracker)

	// Add peers with conflicting heights (no clear majority)
	healthTracker.RecordPeerDiscovered("peer1", 1000, false, TierBronze, true)
	healthTracker.RecordPeerDiscovered("peer2", 2500, false, TierBronze, true)
	healthTracker.RecordPeerDiscovered("peer3", 3000, false, TierBronze, true)

	// Should fail to reach consensus
	_, err := consensusValidator.CalculateConsensusHeight()
	if err == nil {
		t.Error("expected consensus failure with conflicting heights")
	}

	// Add more peers agreeing on one height
	for i := 0; i < 5; i++ {
		addr := "consensus_peer" + string(rune('0'+i))
		healthTracker.RecordPeerDiscovered(addr, 1000, false, TierBronze, true)
	}

	// Should now reach consensus on 1000
	result, err := consensusValidator.CalculateConsensusHeight()
	if err != nil {
		t.Fatalf("failed to reach consensus after adding agreeing peers: %v", err)
	}

	if result.Height != 1000 {
		t.Errorf("expected consensus height 1000, got %d", result.Height)
	}

	// Should have identified the outliers
	if len(result.Outliers) < 2 {
		t.Errorf("expected at least 2 outliers, got %d", len(result.Outliers))
	}
}

// TestAddPeerIncremental tests that AddPeer adds peers without reshuffling
func TestAddPeerIncremental(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)

	// Add 3 initial peers via Rebuild
	for i := 0; i < 3; i++ {
		addr := "peer" + string(rune('0'+i))
		healthTracker.RecordPeerDiscovered(addr, 1000, false, TierBronze, true)
	}
	peerList.Rebuild(healthTracker.GetAllPeers())

	initialSize := peerList.Size()

	// Advance rotation to index 2
	peerList.Next()
	peerList.Next()
	savedIndex := peerList.GetCurrentIndex()

	// Add a new peer incrementally
	healthTracker.RecordPeerDiscovered("new_peer", 1000, false, TierNone, true)
	stats := healthTracker.GetPeerStats("new_peer")
	peerList.AddPeer("new_peer", stats)

	// Size should increase by 1
	if peerList.Size() != initialSize+1 {
		t.Errorf("expected size %d after AddPeer, got %d", initialSize+1, peerList.Size())
	}

	// currentIndex should be preserved
	if peerList.GetCurrentIndex() != savedIndex {
		t.Errorf("expected index %d preserved after AddPeer, got %d", savedIndex, peerList.GetCurrentIndex())
	}

	// New peer should be reachable via rotation
	found := false
	for i := 0; i < peerList.Size()+1; i++ {
		peer, err := peerList.Next()
		if err != nil {
			t.Fatalf("failed to get peer: %v", err)
		}
		if peer == "new_peer" {
			found = true
			break
		}
	}
	if !found {
		t.Error("new_peer should be reachable via rotation after AddPeer")
	}
}

// TestAddPeerMasternode tests that AddPeer duplicates masternodes by tier
func TestAddPeerMasternode(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)

	// Start with empty list
	peerList.Rebuild(healthTracker.GetAllPeers())

	// Add a gold masternode incrementally
	healthTracker.RecordPeerDiscovered("gold_mn", 1000, true, TierGold, true)
	stats := healthTracker.GetPeerStats("gold_mn")
	peerList.AddPeer("gold_mn", stats)

	// Gold masternode should appear 6 times
	if peerList.Size() != 6 {
		t.Errorf("expected 6 entries for gold masternode, got %d", peerList.Size())
	}
}

// TestAddPeerDuplicate tests that AddPeer does not add duplicates
func TestAddPeerDuplicate(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)

	healthTracker.RecordPeerDiscovered("peer0", 1000, false, TierNone, true)
	stats := healthTracker.GetPeerStats("peer0")

	peerList.AddPeer("peer0", stats)
	sizeBefore := peerList.Size()

	// Adding same peer again should be a no-op
	peerList.AddPeer("peer0", stats)
	if peerList.Size() != sizeBefore {
		t.Errorf("duplicate AddPeer should not increase size, got %d (was %d)", peerList.Size(), sizeBefore)
	}
}

// TestRemovePeerAdjustsIndex tests that RemovePeer adjusts rotation index
func TestRemovePeerAdjustsIndex(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)

	// Add 5 peers
	for i := 0; i < 5; i++ {
		addr := "peer" + string(rune('0'+i))
		healthTracker.RecordPeerDiscovered(addr, 1000, false, TierBronze, true)
	}
	peerList.Rebuild(healthTracker.GetAllPeers())

	// Advance to index 3
	peerList.Next()
	peerList.Next()
	peerList.Next()
	indexBefore := peerList.GetCurrentIndex()

	// Get the peer at index 0 (before current position)
	allPeers := peerList.GetAllPeers()
	peerToRemove := allPeers[0]

	// Remove peer before current index
	peerList.RemovePeer(peerToRemove)

	// Index should shift down by 1
	if peerList.GetCurrentIndex() != indexBefore-1 {
		t.Errorf("expected index %d after removing earlier peer, got %d", indexBefore-1, peerList.GetCurrentIndex())
	}

	// Size should decrease
	if peerList.Size() != 4 {
		t.Errorf("expected 4 peers after removal, got %d", peerList.Size())
	}

	// Removed peer should not be in list
	for _, p := range peerList.GetAllPeers() {
		if p == peerToRemove {
			t.Errorf("removed peer %s should not be in list", peerToRemove)
		}
	}
}

// TestRemovePeerEmptyList tests RemovePeer on empty list
func TestRemovePeerEmptyList(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	peerList := NewSyncPeerList(healthTracker)

	// Should not panic
	peerList.RemovePeer("nonexistent")

	if peerList.Size() != 0 {
		t.Errorf("expected empty list, got size %d", peerList.Size())
	}
}
