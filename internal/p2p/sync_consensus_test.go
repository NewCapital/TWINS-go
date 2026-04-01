// Copyright (c) 2025 The TWINS Core developers
// Distributed under the MIT software license

package p2p

import (
	"testing"

	"github.com/twins-dev/twins-core/pkg/types"
)

// TestCalculateConsensusHeight tests consensus height calculation with top-cluster algorithm
func TestCalculateConsensusHeight(t *testing.T) {

	tests := []struct {
		name           string
		peerCount      int
		heights        map[string]uint32
		expectedHeight uint32
		expectError    bool
		minConfidence  float64
	}{
		{
			name:           "4 peers, 3 in top cluster",
			peerCount:      4,
			heights:        map[string]uint32{"p1": 100, "p2": 100, "p3": 100, "p4": 90},
			expectedHeight: 100, // 3 peers at 100, p4 at 90 is within ±10 → 4 in window
			expectError:    false,
			minConfidence:  1.0,
		},
		{
			// Sliding window picks highest viable height (1010) where all peers agree within ±10
			name:           "7 peers all within tolerance - picks highest",
			peerCount:      7,
			heights:        map[string]uint32{"p1": 1000, "p2": 1000, "p3": 1000, "p4": 1000, "p5": 1000, "p6": 1000, "p7": 1010},
			expectedHeight: 1010, // Sliding window returns highest viable height
			expectError:    false,
			minConfidence:  1.0, // All 7 within ±10 of 1010
		},
		{
			// Heights: 1000×4, 990, 980, 970
			// From 1000: window [990,1010] → 1000×4 + 990 = 5 peers ≥ 3 → consensus at 1000
			name:           "7 peers with spread heights - top cluster at highest viable",
			peerCount:      7,
			heights:        map[string]uint32{"p1": 1000, "p2": 1000, "p3": 1000, "p4": 1000, "p5": 990, "p6": 980, "p7": 970},
			expectedHeight: 1000, // Highest height where ≥3 peers agree within ±10
			expectError:    false,
			minConfidence:  0.71, // 5/7 = 71.4%
		},
		{
			name:           "3 peers unanimous",
			peerCount:      3,
			heights:        map[string]uint32{"p1": 500, "p2": 500, "p3": 500},
			expectedHeight: 500,
			expectError:    false,
			minConfidence:  1.0,
		},
		{
			name:        "no peers",
			peerCount:   0,
			heights:     map[string]uint32{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create health tracker and add peers
			healthTracker := NewPeerHealthTracker()
			for addr, height := range tt.heights {
				healthTracker.RecordPeerDiscovered(addr, height, false, TierBronze, true)
			}

			// Create consensus validator
			cv := NewConsensusValidator(healthTracker)

			// Calculate consensus
			result, err := cv.CalculateConsensusHeight()

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result.Height != tt.expectedHeight {
				t.Errorf("expected height %d, got %d", tt.expectedHeight, result.Height)
			}

			if result.Confidence < tt.minConfidence {
				t.Errorf("expected confidence >= %.2f, got %.2f", tt.minConfidence, result.Confidence)
			}

			if result.PeerCount != tt.peerCount {
				t.Errorf("expected peer count %d, got %d", tt.peerCount, result.PeerCount)
			}
		})
	}
}

// TestOutlierDetection tests outlier detection
func TestOutlierDetection(t *testing.T) {
	healthTracker := NewPeerHealthTracker()

	// Add peers with various heights (6 good peers + 2 outliers = 8 total)
	healthTracker.RecordPeerDiscovered("p1", 10000, false, TierBronze, true)
	healthTracker.RecordPeerDiscovered("p2", 10000, false, TierBronze, true)
	healthTracker.RecordPeerDiscovered("p3", 10000, false, TierBronze, true)
	healthTracker.RecordPeerDiscovered("p4", 10000, false, TierBronze, true)
	healthTracker.RecordPeerDiscovered("p5", 10000, false, TierBronze, true)
	healthTracker.RecordPeerDiscovered("p6", 10000, false, TierBronze, true)
	healthTracker.RecordPeerDiscovered("outlier1", 15000, false, TierBronze, true) // >1000 blocks ahead
	healthTracker.RecordPeerDiscovered("outlier2", 8000, false, TierBronze, true)  // >1000 blocks behind

	cv := NewConsensusValidator(healthTracker)
	result, err := cv.CalculateConsensusHeight()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Height != 10000 {
		t.Errorf("expected consensus height 10000, got %d", result.Height)
	}

	if len(result.Outliers) != 2 {
		t.Errorf("expected 2 outliers, got %d", len(result.Outliers))
	}

	// Check outliers are correctly identified
	outlierMap := make(map[string]bool)
	for _, addr := range result.Outliers {
		outlierMap[addr] = true
	}

	if !outlierMap["outlier1"] {
		t.Error("outlier1 not detected")
	}
	if !outlierMap["outlier2"] {
		t.Error("outlier2 not detected")
	}
}

// TestValidateLocalChain tests local chain validation
func TestValidateLocalChain(t *testing.T) {
	healthTracker := NewPeerHealthTracker()
	cv := NewConsensusValidator(healthTracker)

	localHash := types.Hash{}
	copy(localHash[:], []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
	consensusHash := types.Hash{}
	copy(consensusHash[:], []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2})

	tests := []struct {
		name            string
		localHeight     uint32
		localTip        types.Hash
		consensusHeight uint32
		consensusTip    types.Hash
		expectValid     bool
	}{
		{
			name:            "exact match",
			localHeight:     1000,
			localTip:        localHash,
			consensusHeight: 1000,
			consensusTip:    localHash,
			expectValid:     true,
		},
		{
			name:            "small height diff same hash",
			localHeight:     1000,
			localTip:        localHash,
			consensusHeight: 1010,
			consensusTip:    localHash,
			expectValid:     true,
		},
		{
			name:            "small height diff different hash",
			localHeight:     1000,
			localTip:        localHash,
			consensusHeight: 1010,
			consensusTip:    consensusHash,
			expectValid:     false,
		},
		{
			name:            "large height diff - forked",
			localHeight:     1000,
			localTip:        localHash,
			consensusHeight: 1200,
			consensusTip:    consensusHash,
			expectValid:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, _ := cv.ValidateLocalChain(tt.localHeight, tt.localTip, tt.consensusHeight, tt.consensusTip)
			if valid != tt.expectValid {
				t.Errorf("expected valid=%v, got %v", tt.expectValid, valid)
			}
		})
	}
}

// TestTopClusterConsensus tests the core top-cluster algorithm behavior:
// consensus requires ≥3 peers in a ±10 block window, regardless of total peer count
func TestTopClusterConsensus(t *testing.T) {

	tests := []struct {
		name       string
		totalPeers int
		agreePeers int // peers at height 1000 (rest at 900, outside ±10 window)
		shouldPass bool
	}{
		// Core rule: need ≥3 peers in cluster
		{"3 peers, 3 agree", 3, 3, true},
		{"4 peers, 3 agree", 4, 3, true},
		{"4 peers, 4 agree", 4, 4, true},

		// <3 agree and dissenting peers scattered (no cluster anywhere)
		{"3 peers, 2 agree", 3, 2, false},
		{"4 peers, 2 agree", 4, 2, false},
		{"5 peers, 2 agree", 5, 2, false},

		// Key improvement: works with many unsynced peers
		{"10 peers, 3 agree (top cluster)", 10, 3, true},
		{"10 peers, 5 agree", 10, 5, true},
		{"20 peers, 3 agree (top cluster)", 20, 3, true},
		{"20 peers, 5 agree", 20, 5, true},
		{"50 peers, 3 agree (top cluster)", 50, 3, true},

		// Still fails if <3 agree and dissenting peers don't cluster
		{"10 peers, 2 agree scattered", 10, 2, false},
		{"20 peers, 2 agree scattered", 20, 2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			healthTracker := NewPeerHealthTracker()

			// Add peers with consensus height (1000)
			for i := 0; i < tt.agreePeers; i++ {
				addr := "agree" + string(rune('0'+i))
				healthTracker.RecordPeerDiscovered(addr, 1000, false, TierBronze, true)
			}

			// Add dissenting peers at scattered heights (each 100 blocks apart,
			// well away from 1000, so they can't form clusters)
			for i := 0; i < tt.totalPeers-tt.agreePeers; i++ {
				addr := "dissent" + string(rune('0'+i))
				healthTracker.RecordPeerDiscovered(addr, uint32(5000+i*100), false, TierBronze, true)
			}

			cv := NewConsensusValidator(healthTracker)
			result, err := cv.CalculateConsensusHeight()

			if tt.shouldPass && err != nil {
				t.Errorf("expected pass but got error: %v", err)
			}
			if !tt.shouldPass && err == nil {
				t.Errorf("expected fail but got no error")
			}
			if tt.shouldPass && err == nil && result.Height != 1000 {
				t.Errorf("expected consensus height 1000, got %d", result.Height)
			}
		})
	}
}

// TestConsensusWithUnhealthyPeers tests that unhealthy peers are excluded
func TestConsensusWithUnhealthyPeers(t *testing.T) {
	healthTracker := NewPeerHealthTracker()

	// Add healthy peers with height 1000
	for i := 0; i < 5; i++ {
		addr := "healthy" + string(rune('0'+i))
		healthTracker.RecordPeerDiscovered(addr, 1000, false, TierBronze, true)
	}

	// Add unhealthy peer with different height
	healthTracker.RecordPeerDiscovered("unhealthy", 500, false, TierBronze, true)
	// Put it on cooldown by recording errors
	for i := 0; i < 6; i++ {
		healthTracker.RecordError("unhealthy", ErrorTypeTimeout)
	}

	cv := NewConsensusValidator(healthTracker)
	result, err := cv.CalculateConsensusHeight()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Height != 1000 {
		t.Errorf("expected consensus height 1000, got %d (unhealthy peer should be excluded)", result.Height)
	}

	if result.PeerCount != 5 {
		t.Errorf("expected 5 peers (unhealthy excluded), got %d", result.PeerCount)
	}
}

// TestHeightTolerance tests that peers within ±HeightTolerance (10 blocks) are counted as agreeing
func TestHeightTolerance(t *testing.T) {
	tests := []struct {
		name           string
		heights        map[string]uint32
		expectedHeight uint32
		minConfidence  float64
		expectError    bool
	}{
		{
			// Original issue: 5 peers at 1660380, 3 at 1660381, 1 outlier at 1545299
			// Sliding window picks 1660381 (highest) where 8/9 agree within ±10
			name: "real-world scenario - peers 1 block apart",
			heights: map[string]uint32{
				"p1": 1660380, "p2": 1660380, "p3": 1660380, "p4": 1660380, "p5": 1660380,
				"p6": 1660381, "p7": 1660381, "p8": 1660381,
				"outlier": 1545299,
			},
			expectedHeight: 1660381, // Sliding window picks highest viable
			minConfidence:  0.88,    // 8/9 = 88.9%
			expectError:    false,
		},
		{
			// Peers within ±10 blocks should all count as agreeing
			// Sliding window picks highest (1002) where all agree
			name: "peers within tolerance range",
			heights: map[string]uint32{
				"p1": 1000, "p2": 1000, "p3": 1000,
				"p4": 1001, "p5": 1002,
			},
			expectedHeight: 1002, // Sliding window picks highest
			minConfidence:  1.0,  // All 5 within tolerance
			expectError:    false,
		},
		{
			// Peers at exactly ±10 blocks from center should all count
			// Heights: 990, 1000(×4), 1010 - all within ±10 of 1000
			name: "peers at boundary of tolerance (±10)",
			heights: map[string]uint32{
				"p1": 1000, "p2": 1000, "p3": 1000, "p4": 1000,
				"p5": 990,  // Exactly -10 from 1000
				"p6": 1010, // Exactly +10 from 1000
			},
			// From 1010: [1000,1020] = 1000(×4)+1010 = 5/6 → returns 1010 (≥3)
			expectedHeight: 1010,
			minConfidence:  0.83, // 5/6 peers within tolerance of highest viable
			expectError:    false,
		},
		{
			// 5 at 1000, 1 at 985, 1 at 1015 — the ±15 peers are outside tolerance of each other
			// but 5 at 1000 form a cluster of ≥3 → passes with top-cluster algorithm
			name: "5 peers at same height, 2 outside tolerance",
			heights: map[string]uint32{
				"p1": 1000, "p2": 1000, "p3": 1000, "p4": 1000, "p5": 1000,
				"p6": 985,  // 15 blocks away - outside tolerance
				"p7": 1015, // 15 blocks away - outside tolerance
			},
			expectedHeight: 1000, // 5 peers form cluster at 1000
			minConfidence:  0.71, // 5/7 = 71.4%
			expectError:    false,
		},
		{
			// User's actual issue case: peers spread across 11 blocks
			// Heights: 1660973, 1660977(×3), 1660983(×2), 1660984(×4)
			// With tolerance=10, sliding window should find consensus
			name: "user issue - peers spread 11 blocks",
			heights: map[string]uint32{
				"p1": 1660984, "p2": 1660984, "p3": 1660984, "p4": 1660984,
				"p5": 1660983, "p6": 1660983,
				"p7": 1660977, "p8": 1660977, "p9": 1660977,
				"p10": 1660973,
			},
			expectedHeight: 1660984, // Highest where ≥3 agree
			minConfidence:  0.90,    // 9/10 within ±10 of 1660984
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			healthTracker := NewPeerHealthTracker()
			for addr, height := range tt.heights {
				healthTracker.RecordPeerDiscovered(addr, height, false, TierBronze, true)
			}

			cv := NewConsensusValidator(healthTracker)
			result, err := cv.CalculateConsensusHeight()

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result.Height != tt.expectedHeight {
				t.Errorf("expected height %d, got %d", tt.expectedHeight, result.Height)
			}

			if result.Confidence < tt.minConfidence {
				t.Errorf("expected confidence >= %.2f, got %.2f", tt.minConfidence, result.Confidence)
			}
		})
	}
}

// TestAbsDiff tests the absolute difference helper function
func TestAbsDiff(t *testing.T) {
	tests := []struct {
		a, b     uint32
		expected uint32
	}{
		{10, 5, 5},
		{5, 10, 5},
		{0, 0, 0},
		{100, 100, 0},
		{1000000, 999998, 2},
	}

	for _, tt := range tests {
		result := absDiff(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("absDiff(%d, %d) = %d, expected %d", tt.a, tt.b, result, tt.expected)
		}
	}
}
