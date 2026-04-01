// Copyright (c) 2025 The TWINS Core developers
// Distributed under the MIT software license

package p2p

import (
	"fmt"
	"sync"

	"github.com/twins-dev/twins-core/pkg/types"
)

// ConsensusStrategy defines how consensus is calculated
type ConsensusStrategy int

const (
	StrategyOutboundOnly ConsensusStrategy = iota // Only outbound peers (default, Sybil-resistant)
	StrategyAll                                    // All healthy peers
)

// Consensus calculation constants
const (
	// HeightTolerance defines the maximum block difference for peers to be considered
	// "in agreement" with the consensus height. This accounts for normal network
	// propagation delays where peers may be several blocks apart near the chain tip.
	HeightTolerance uint32 = 10

	// OutlierReportThreshold defines the block difference beyond which a peer is
	// flagged as an outlier in reporting (potential fork or malicious peer).
	OutlierReportThreshold uint32 = 1000

	// ForkDetectionThreshold defines the maximum block difference where chains are
	// considered "just syncing" vs potentially forked.
	ForkDetectionThreshold uint32 = 100

	// DefaultMinClusterSize is the minimum number of peers that must agree on a height
	// (within ±HeightTolerance) for it to be accepted as consensus. This replaces the
	// old percentage-based thresholds which failed when many peers were still syncing.
	DefaultMinClusterSize = 3
)

// String returns the string representation of a consensus strategy
func (cs ConsensusStrategy) String() string {
	switch cs {
	case StrategyOutboundOnly:
		return "OUTBOUND_ONLY"
	case StrategyAll:
		return "ALL"
	default:
		return "UNKNOWN"
	}
}

// ConsensusValidator validates network consensus to prevent sync from malicious/forked peers
type ConsensusValidator struct {
	healthTracker *PeerHealthTracker

	// Configuration
	minClusterSize  int               // Minimum peers in ±HeightTolerance window to accept consensus
	defaultStrategy ConsensusStrategy // Default strategy to use

	mu sync.RWMutex
}

// ConsensusResult contains the result of consensus calculation
type ConsensusResult struct {
	Height     uint32            // Consensus height (highest with ≥minClusterSize peers agreeing)
	Confidence float64           // Percentage of peers agreeing (0.0-1.0, informational)
	PeerCount  int               // Total peers considered
	Outliers   []string          // Peers significantly off consensus
	Strategy   ConsensusStrategy // Strategy used to calculate consensus
}

// NewConsensusValidator creates a new consensus validator with default configuration
func NewConsensusValidator(healthTracker *PeerHealthTracker) *ConsensusValidator {
	return &ConsensusValidator{
		healthTracker:   healthTracker,
		minClusterSize:  DefaultMinClusterSize,
		defaultStrategy: StrategyOutboundOnly,
	}
}

// SetMinClusterSize sets the minimum peers required in a cluster for consensus
func (cv *ConsensusValidator) SetMinClusterSize(size int) {
	cv.mu.Lock()
	defer cv.mu.Unlock()
	cv.minClusterSize = size
}

// SetDefaultStrategy sets the default consensus calculation strategy.
// Valid strategies: StrategyOutboundOnly (default, Sybil-resistant), StrategyAll.
func (cv *ConsensusValidator) SetDefaultStrategy(strategy ConsensusStrategy) {
	cv.mu.Lock()
	defer cv.mu.Unlock()
	cv.defaultStrategy = strategy
}

// CalculateConsensusHeight determines the consensus height using outbound peers only.
// Uses "top cluster" algorithm: finds the highest height where ≥minClusterSize peers
// agree within ±HeightTolerance blocks. Peers at lower heights are treated as
// "still syncing" rather than "disagreeing", which handles the common case where
// most peers haven't caught up yet.
func (cv *ConsensusValidator) CalculateConsensusHeight() (*ConsensusResult, error) {
	return cv.CalculateConsensusHeightWithStrategy(StrategyOutboundOnly)
}

// detectOutliers identifies peers significantly off consensus
func (cv *ConsensusValidator) detectOutliers(peerHeights map[string]uint32, consensusHeight uint32) []string {
	outliers := make([]string, 0)
	for addr, height := range peerHeights {
		if absDiff(height, consensusHeight) > OutlierReportThreshold {
			outliers = append(outliers, addr)
		}
	}

	return outliers
}

// ValidateLocalChain checks if our local chain matches network consensus
// Returns true if we're on the majority chain, false if we need to reorg
func (cv *ConsensusValidator) ValidateLocalChain(localHeight uint32, localTip types.Hash,
	consensusHeight uint32, consensusTip types.Hash) (bool, error) {

	cv.mu.RLock()
	defer cv.mu.RUnlock()

	// If heights match and hashes match, we're on consensus chain
	if localHeight == consensusHeight && localTip == consensusTip {
		return true, nil
	}

	// If we're within ForkDetectionThreshold blocks, we're probably just behind (not forked)
	heightDiff := absDiff(localHeight, consensusHeight)

	if heightDiff <= ForkDetectionThreshold {
		// Small difference, likely just syncing
		return localTip == consensusTip, nil
	}

	// Significant height difference - we're likely forked
	return false, fmt.Errorf("chain fork detected: local=%d/%s consensus=%d/%s",
		localHeight, localTip.String()[:8], consensusHeight, consensusTip.String()[:8])
}

// GetPeersByHeight returns peers grouped by their reported height
func (cv *ConsensusValidator) GetPeersByHeight() map[uint32][]string {
	cv.mu.RLock()
	defer cv.mu.RUnlock()

	result := make(map[uint32][]string)
	allPeers := cv.healthTracker.GetAllPeers()

	for addr, stats := range allPeers {
		if cv.healthTracker.IsHealthy(addr) {
			result[stats.TipHeight] = append(result[stats.TipHeight], addr)
		}
	}

	return result
}

// RequiresReorg determines if a reorg is needed based on consensus
func (cv *ConsensusValidator) RequiresReorg(localHeight uint32, consensusHeight uint32) bool {
	// If we're more than ForkDetectionThreshold blocks different, likely need reorg
	return absDiff(localHeight, consensusHeight) > ForkDetectionThreshold
}

// GetConsensusConfidence returns the confidence level of current consensus
// Returns 0.0-1.0 representing the percentage of peers agreeing
func (cv *ConsensusValidator) GetConsensusConfidence() float64 {
	result, err := cv.CalculateConsensusHeight()
	if err != nil {
		return 0.0
	}
	return result.Confidence
}

// IsConsensusReliable checks if we have enough peers for reliable consensus
func (cv *ConsensusValidator) IsConsensusReliable() bool {
	cv.mu.RLock()
	minCluster := cv.minClusterSize
	cv.mu.RUnlock()

	allPeers := cv.healthTracker.GetAllPeers()
	healthyCount := 0

	for addr := range allPeers {
		if cv.healthTracker.IsHealthy(addr) {
			healthyCount++
		}
	}

	return healthyCount >= minCluster
}

// GetOutboundPeers returns heights of only outbound peers
func (cv *ConsensusValidator) GetOutboundPeers() map[string]uint32 {
	cv.mu.RLock()
	defer cv.mu.RUnlock()

	outboundHeights := make(map[string]uint32)
	allPeers := cv.healthTracker.GetAllPeers()

	for addr, stats := range allPeers {
		if cv.healthTracker.IsHealthy(addr) && stats.IsOutbound {
			outboundHeights[addr] = stats.TipHeight
		}
	}

	return outboundHeights
}

// CalculateConsensusHeightWithStrategy calculates consensus using a specific strategy.
//
// Algorithm ("top cluster"):
//  1. Collect healthy peer heights based on strategy (outbound-only or all)
//  2. For each unique height from highest to lowest, count peers within ±HeightTolerance
//  3. First height with ≥minClusterSize peers in window → accepted as consensus
//  4. Peers at lower heights are "still syncing", not "disagreeing"
//
// This replaces the old percentage-based thresholds (66%/75%/51%) which failed when
// connected to many peers where most hadn't finished syncing yet. With the top-cluster
// approach, even 3 synced peers among 20 unsynced ones produce a valid consensus.
//
// Security: the consensus height is only a sync TARGET, not a block acceptance mechanism.
// Every block is cryptographically validated regardless. Wrong consensus height → blocks
// fail validation → recovery system rolls back. The StrategyOutboundOnly default provides
// additional Sybil resistance since we choose which peers to connect to outbound.
func (cv *ConsensusValidator) CalculateConsensusHeightWithStrategy(strategy ConsensusStrategy) (*ConsensusResult, error) {
	cv.mu.RLock()
	minCluster := cv.minClusterSize
	cv.mu.RUnlock()

	var peerHeights map[string]uint32

	// Step 1: Select peers based on strategy
	switch strategy {
	case StrategyOutboundOnly:
		allPeers := cv.healthTracker.GetAllPeers()
		peerHeights = make(map[string]uint32)
		for addr, stats := range allPeers {
			if cv.healthTracker.IsHealthy(addr) && stats.IsOutbound {
				peerHeights[addr] = stats.TipHeight
			}
		}

	case StrategyAll:
		allPeers := cv.healthTracker.GetAllPeers()
		peerHeights = make(map[string]uint32)
		for addr, stats := range allPeers {
			if cv.healthTracker.IsHealthy(addr) {
				peerHeights[addr] = stats.TipHeight
			}
		}

	default:
		return nil, fmt.Errorf("unknown consensus strategy: %v", strategy)
	}

	if len(peerHeights) == 0 {
		return nil, fmt.Errorf("no peers available for consensus (strategy: %s)", strategy.String())
	}

	// Step 2: Find highest height where ≥minClusterSize peers agree within ±HeightTolerance
	totalPeers := len(peerHeights)
	optimalHeight, clusterCount, found := findOptimalConsensusHeight(peerHeights, HeightTolerance, minCluster)
	if !found {
		return nil, fmt.Errorf("insufficient peer agreement: no height has %d+ peers within ±%d blocks (%d peers, strategy: %s)",
			minCluster, HeightTolerance, totalPeers, strategy.String())
	}

	confidence := float64(clusterCount) / float64(totalPeers)

	// Step 3: Identify outliers for reporting
	outliers := cv.detectOutliers(peerHeights, optimalHeight)

	return &ConsensusResult{
		Height:     optimalHeight,
		Confidence: confidence,
		PeerCount:  totalPeers,
		Outliers:   outliers,
		Strategy:   strategy,
	}, nil
}

// GetConsensusHeightInfo implements consensus.ConsensusHeightProvider interface.
// This adapter method allows StakingWorker to use ConsensusValidator without import cycles.
// Returns simple values instead of struct to avoid type compatibility issues across packages.
func (cv *ConsensusValidator) GetConsensusHeightInfo() (height uint32, confidence float64, peerCount int, err error) {
	result, err := cv.CalculateConsensusHeight()
	if err != nil {
		return 0, 0, 0, err
	}

	return result.Height, result.Confidence, result.PeerCount, nil
}

// absDiff returns the absolute difference between two uint32 values
func absDiff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}

// collectUniqueHeightsSorted returns unique peer heights sorted in descending order
func collectUniqueHeightsSorted(peerHeights map[string]uint32) []uint32 {
	seen := make(map[uint32]bool)
	heights := make([]uint32, 0)

	for _, h := range peerHeights {
		if !seen[h] {
			seen[h] = true
			heights = append(heights, h)
		}
	}

	// Sort descending (highest first)
	for i := 0; i < len(heights); i++ {
		for j := i + 1; j < len(heights); j++ {
			if heights[j] > heights[i] {
				heights[i], heights[j] = heights[j], heights[i]
			}
		}
	}

	return heights
}

// countPeersInWindow counts how many peers have heights within [center-tolerance, center+tolerance]
func countPeersInWindow(peerHeights map[string]uint32, center uint32, tolerance uint32) int {
	count := 0
	for _, h := range peerHeights {
		if absDiff(h, center) <= tolerance {
			count++
		}
	}
	return count
}

// findOptimalConsensusHeight finds the highest height where at least minCount peers
// are within ±tolerance blocks. Returns the height, the count of agreeing peers, and
// whether a valid cluster was found.
func findOptimalConsensusHeight(peerHeights map[string]uint32, tolerance uint32, minCount int) (uint32, int, bool) {
	if len(peerHeights) == 0 {
		return 0, 0, false
	}

	candidates := collectUniqueHeightsSorted(peerHeights)

	// Try each unique height from highest to lowest
	for _, candidate := range candidates {
		count := countPeersInWindow(peerHeights, candidate, tolerance)
		if count >= minCount {
			return candidate, count, true
		}
	}

	return 0, 0, false
}
