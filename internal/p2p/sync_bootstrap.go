// Copyright (c) 2025 The TWINS Core developers
// Distributed under the MIT software license

package p2p

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// BootstrapManager manages the bootstrap phase where we discover and evaluate peers
// before starting blockchain sync
type BootstrapManager struct {
	// Configuration
	targetPeers int           // Target number of peers (default: 7)
	minWait     time.Duration // Minimum wait time (default: 10s)
	maxWait     time.Duration // Maximum wait timeout (default: 30s)
	logger      *logrus.Entry

	// State
	discoveredPeers map[string]*BootstrapPeerInfo // Peers discovered during bootstrap
	masternodes     map[string]MasternodeTier     // Masternode addresses and tiers
	startTime       time.Time                     // Bootstrap start time
	active          bool                          // Is bootstrap currently active
	completed       bool                          // Has bootstrap completed

	// Synchronization
	completeChan chan struct{} // Signals bootstrap completion
	stopChan     chan struct{} // Signals bootstrap should stop early
	mu           sync.RWMutex
}

// BootstrapPeerInfo contains information about a peer discovered during bootstrap
type BootstrapPeerInfo struct {
	Address      string
	Height       uint32
	IsMasternode bool
	Tier         MasternodeTier
	Version      uint32
	Services     ServiceFlag
	UserAgent    string
	DiscoveredAt time.Time
}

// NewBootstrapManager creates a new bootstrap manager
func NewBootstrapManager(targetPeers int, minWait, maxWait time.Duration, logger *logrus.Entry) *BootstrapManager {
	return &BootstrapManager{
		targetPeers:     targetPeers,
		minWait:         minWait,
		maxWait:         maxWait,
		logger:          logger,
		discoveredPeers: make(map[string]*BootstrapPeerInfo),
		masternodes:     make(map[string]MasternodeTier),
		completeChan:    make(chan struct{}),
	}
}

// Start begins the bootstrap phase
func (bm *BootstrapManager) Start() {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if bm.active {
		return
	}

	bm.active = true
	bm.completed = false
	bm.startTime = time.Now()
	bm.discoveredPeers = make(map[string]*BootstrapPeerInfo)
	bm.masternodes = make(map[string]MasternodeTier)
	bm.completeChan = make(chan struct{})
	bm.stopChan = make(chan struct{})

	bm.logger.WithFields(logrus.Fields{
		"target_peers": bm.targetPeers,
		"min_wait":     bm.minWait,
		"max_wait":     bm.maxWait,
	}).Info("Starting bootstrap phase")

	// Start bootstrap timer in background
	go bm.runBootstrap()
}

// runBootstrap runs the bootstrap timer and completion logic
func (bm *BootstrapManager) runBootstrap() {
	minTimer := time.NewTimer(bm.minWait)
	maxTimer := time.NewTimer(bm.maxWait)
	checkTicker := time.NewTicker(1 * time.Second)
	defer minTimer.Stop()
	defer maxTimer.Stop()
	defer checkTicker.Stop()

	minWaitDone := false

	for {
		select {
		case <-bm.stopChan:
			bm.logger.Info("Bootstrap stopped by shutdown signal")
			bm.complete()
			return

		case <-minTimer.C:
			minWaitDone = true
			bm.logger.Debug("Bootstrap minimum wait completed")

			// Check if we have enough peers
			if bm.hasEnoughPeers() {
				bm.complete()
				return
			}

		case <-maxTimer.C:
			bm.logger.WithField("peer_count", bm.PeerCount()).
				Warn("Bootstrap maximum wait timeout reached")
			bm.complete()
			return

		case <-checkTicker.C:
			// After min wait, check if we reached target
			if minWaitDone && bm.hasEnoughPeers() {
				bm.complete()
				return
			}
		}
	}
}

// Stop signals the bootstrap manager to stop early (for graceful shutdown)
func (bm *BootstrapManager) Stop() {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if !bm.active || bm.completed {
		return
	}

	// Signal the runBootstrap goroutine to stop
	close(bm.stopChan)
}

// complete marks bootstrap as complete and signals waiting goroutines
func (bm *BootstrapManager) complete() {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if bm.completed {
		return
	}

	bm.completed = true
	bm.active = false
	duration := time.Since(bm.startTime)

	bm.logger.WithFields(logrus.Fields{
		"peer_count":       len(bm.discoveredPeers),
		"masternode_count": len(bm.masternodes),
		"duration":         duration,
	}).Info("Bootstrap phase completed")

	close(bm.completeChan)
}

// OnPeerDiscovered is called when a peer sends a version message
func (bm *BootstrapManager) OnPeerDiscovered(address string, height uint32, version uint32,
	services ServiceFlag, userAgent string) {

	bm.mu.Lock()
	defer bm.mu.Unlock()

	if !bm.active {
		return
	}

	// Check if this is a masternode
	isMasternode := services&SFNodeMasternode != 0
	tier := TierBronze // Default tier

	// Determine masternode tier from services flags
	if isMasternode {
		tier = bm.detectMasternodeTier(services)
		bm.masternodes[address] = tier
	}

	// Record peer info
	bm.discoveredPeers[address] = &BootstrapPeerInfo{
		Address:      address,
		Height:       height,
		IsMasternode: isMasternode,
		Tier:         tier,
		Version:      version,
		Services:     services,
		UserAgent:    userAgent,
		DiscoveredAt: time.Now(),
	}

	bm.logger.WithFields(logrus.Fields{
		"address":      address,
		"height":       height,
		"masternode":   isMasternode,
		"tier":         tier,
		"peer_count":   len(bm.discoveredPeers),
		"target_peers": bm.targetPeers,
	}).Debug("Peer discovered during bootstrap")
}

// detectMasternodeTier determines masternode tier from service flags
func (bm *BootstrapManager) detectMasternodeTier(services ServiceFlag) MasternodeTier {
	// Check for tier-specific service flags
	if services&ServiceFlagMasternodePlat != 0 {
		return TierPlatinum
	}
	if services&ServiceFlagMasternodeGold != 0 {
		return TierGold
	}
	if services&ServiceFlagMasternodeSilver != 0 {
		return TierSilver
	}
	if services&ServiceFlagMasternodeBronze != 0 {
		return TierBronze
	}
	// Default to Bronze if masternode flag is set but no tier specified
	return TierBronze
}

// hasEnoughPeers checks if we've reached the target peer count
func (bm *BootstrapManager) hasEnoughPeers() bool {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	return len(bm.discoveredPeers) >= bm.targetPeers
}

// IsActive returns true if bootstrap is currently running
func (bm *BootstrapManager) IsActive() bool {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	return bm.active
}

// IsCompleted returns true if bootstrap has completed
func (bm *BootstrapManager) IsCompleted() bool {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	return bm.completed
}

// Done returns a channel that is closed when bootstrap completes
func (bm *BootstrapManager) Done() <-chan struct{} {
	return bm.completeChan
}

// PeerCount returns the number of peers discovered
func (bm *BootstrapManager) PeerCount() int {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	return len(bm.discoveredPeers)
}

// MasternodeCount returns the number of masternodes discovered
func (bm *BootstrapManager) MasternodeCount() int {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	return len(bm.masternodes)
}

// GetDiscoveredPeers returns a copy of all discovered peers
func (bm *BootstrapManager) GetDiscoveredPeers() map[string]*BootstrapPeerInfo {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	result := make(map[string]*BootstrapPeerInfo)
	for addr, info := range bm.discoveredPeers {
		// Create a copy
		infoCopy := *info
		result[addr] = &infoCopy
	}
	return result
}

// GetMasternodes returns a copy of discovered masternodes and their tiers
func (bm *BootstrapManager) GetMasternodes() map[string]MasternodeTier {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	result := make(map[string]MasternodeTier)
	for addr, tier := range bm.masternodes {
		result[addr] = tier
	}
	return result
}

// GetStats returns bootstrap statistics
func (bm *BootstrapManager) GetStats() BootstrapStats {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	var duration time.Duration
	if !bm.startTime.IsZero() {
		if bm.completed {
			duration = time.Since(bm.startTime)
		} else {
			duration = time.Since(bm.startTime)
		}
	}

	return BootstrapStats{
		Active:          bm.active,
		Completed:       bm.completed,
		PeerCount:       len(bm.discoveredPeers),
		MasternodeCount: len(bm.masternodes),
		TargetPeers:     bm.targetPeers,
		Duration:        duration,
	}
}

// BootstrapStats contains bootstrap statistics
type BootstrapStats struct {
	Active          bool
	Completed       bool
	PeerCount       int
	MasternodeCount int
	TargetPeers     int
	Duration        time.Duration
}

// Reset resets the bootstrap manager for a new bootstrap phase
func (bm *BootstrapManager) Reset() {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	bm.active = false
	bm.completed = false
	bm.discoveredPeers = make(map[string]*BootstrapPeerInfo)
	bm.masternodes = make(map[string]MasternodeTier)
	bm.startTime = time.Time{}
}

// TargetPeers returns the target number of peers for bootstrap
func (bm *BootstrapManager) TargetPeers() int {
	return bm.targetPeers
}

// MaxWait returns the maximum wait time for bootstrap before emergency fallback
func (bm *BootstrapManager) MaxWait() time.Duration {
	return bm.maxWait
}
