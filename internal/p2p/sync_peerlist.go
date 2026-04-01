package p2p

import (
	"errors"
	"math/rand"
	"sync"
	"time"
)

// DefaultMaxSyncPeers is the default cap on the sync rotation list size.
const DefaultMaxSyncPeers = 20

// SyncPeerList manages a circular rotation queue of peers for sync
type SyncPeerList struct {
	peers         []string           // Peer addresses in rotation order
	currentIndex  int                // Current position in rotation
	roundCount    int                // Number of complete rounds
	maxPeers      int                // Cap on rotation list size (from sync.maxSyncPeers config)
	healthTracker *PeerHealthTracker // Reference to health tracker
	mu            sync.RWMutex
}

// NewSyncPeerList creates a new sync peer list
func NewSyncPeerList(healthTracker *PeerHealthTracker) *SyncPeerList {
	return &SyncPeerList{
		peers:         make([]string, 0),
		currentIndex:  0,
		roundCount:    0,
		maxPeers:      DefaultMaxSyncPeers,
		healthTracker: healthTracker,
	}
}

// SetMaxPeers sets the maximum number of peers in the rotation list.
func (spl *SyncPeerList) SetMaxPeers(max int) {
	spl.mu.Lock()
	defer spl.mu.Unlock()
	if max > 0 {
		spl.maxPeers = max
	}
}

// Next returns the next peer in rotation, skipping unhealthy ones
func (spl *SyncPeerList) Next() (string, error) {
	spl.mu.Lock()
	defer spl.mu.Unlock()

	if len(spl.peers) == 0 {
		return "", errors.New("no peers in list")
	}

	attempts := 0

	// Try to find a healthy peer
	for attempts < len(spl.peers) {
		peer := spl.peers[spl.currentIndex]

		// Move to next index
		spl.currentIndex = (spl.currentIndex + 1) % len(spl.peers)

		// Detect round completion (wrapped back to 0)
		if spl.currentIndex == 0 {
			spl.roundCount++
		}

		// Check if peer is healthy
		if spl.healthTracker.IsHealthy(peer) {
			return peer, nil
		}

		attempts++
	}

	return "", errors.New("no healthy peers available")
}

// CompletedRound returns true if we just completed a full round
func (spl *SyncPeerList) CompletedRound() bool {
	spl.mu.RLock()
	defer spl.mu.RUnlock()

	return spl.currentIndex == 0 && spl.roundCount > 0
}

// GetRoundCount returns the number of completed rounds
func (spl *SyncPeerList) GetRoundCount() int {
	spl.mu.RLock()
	defer spl.mu.RUnlock()

	return spl.roundCount
}

// GetAllPeers returns a copy of the peer list
func (spl *SyncPeerList) GetAllPeers() []string {
	spl.mu.RLock()
	defer spl.mu.RUnlock()

	result := make([]string, len(spl.peers))
	copy(result, spl.peers)
	return result
}

// Rebuild creates a new random peer list with masternode duplication
func (spl *SyncPeerList) Rebuild(allPeers map[string]*PeerHealthStats) {
	spl.mu.Lock()
	defer spl.mu.Unlock()

	var masternodes []string
	var regularPeers []string

	// Separate masternodes and regular peers
	now := time.Now()
	for addr, stats := range allPeers {
		// Skip peers on cooldown
		if !stats.CooldownUntil.IsZero() && now.Before(stats.CooldownUntil) {
			continue // Peer is in cooldown period, skip it
		}

		// Check if peer is healthy (or recently off cooldown)
		if stats.IsMasternode {
			// Duplicate masternodes based on tier
			copies := spl.getMasternodeCopies(stats.Tier)
			for i := 0; i < copies; i++ {
				masternodes = append(masternodes, addr)
			}
		} else {
			regularPeers = append(regularPeers, addr)
		}
	}

	// Randomize both lists independently
	rand.Shuffle(len(masternodes), func(i, j int) {
		masternodes[i], masternodes[j] = masternodes[j], masternodes[i]
	})
	rand.Shuffle(len(regularPeers), func(i, j int) {
		regularPeers[i], regularPeers[j] = regularPeers[j], regularPeers[i]
	})

	// Merge: sprinkle masternodes throughout regular peers
	newList := spl.sprinkleMasternodes(masternodes, regularPeers)

	// Cap at maxPeers (from sync.maxSyncPeers config, default 20)
	if len(newList) > spl.maxPeers {
		newList = newList[:spl.maxPeers]
	}

	spl.peers = newList
	spl.currentIndex = 0
	spl.roundCount = 0
}

// getMasternodeCopies returns the number of times a masternode should appear based on tier
func (spl *SyncPeerList) getMasternodeCopies(tier MasternodeTier) int {
	switch tier {
	case TierPlatinum:
		return 8
	case TierGold:
		return 6
	case TierSilver:
		return 4
	case TierBronze:
		return 2
	default:
		return 1
	}
}

// sprinkleMasternodes interleaves masternodes throughout regular peers
func (spl *SyncPeerList) sprinkleMasternodes(masternodes, regularPeers []string) []string {
	if len(masternodes) == 0 {
		return regularPeers
	}

	if len(regularPeers) == 0 {
		return masternodes
	}

	// Calculate distribution ratio
	ratio := len(regularPeers) / len(masternodes)
	if ratio == 0 {
		ratio = 1
	}

	result := make([]string, 0, len(masternodes)+len(regularPeers))
	mnIndex := 0
	regIndex := 0

	// Interleave: add masternode, then 'ratio' regular peers
	for mnIndex < len(masternodes) || regIndex < len(regularPeers) {
		// Add masternode
		if mnIndex < len(masternodes) {
			result = append(result, masternodes[mnIndex])
			mnIndex++
		}

		// Add 'ratio' regular peers
		for i := 0; i < ratio && regIndex < len(regularPeers); i++ {
			result = append(result, regularPeers[regIndex])
			regIndex++
		}
	}

	return result
}

// AddPeer appends a peer to the rotation list without reshuffling.
// If the peer is a masternode, it gets tier-based duplication.
// This is used for incremental updates when a new peer completes handshake.
func (spl *SyncPeerList) AddPeer(addr string, stats *PeerHealthStats) {
	spl.mu.Lock()
	defer spl.mu.Unlock()

	if stats == nil {
		return
	}

	// Skip peers on cooldown
	if !stats.CooldownUntil.IsZero() && time.Now().Before(stats.CooldownUntil) {
		return
	}

	// Check if already in list
	for _, p := range spl.peers {
		if p == addr {
			return
		}
	}

	// Determine how many copies to add
	copies := 1
	if stats.IsMasternode {
		copies = spl.getMasternodeCopies(stats.Tier)
	}

	for i := 0; i < copies; i++ {
		spl.peers = append(spl.peers, addr)
	}
}

// RemovePeer removes all occurrences of a peer from the rotation list
// and adjusts currentIndex to maintain rotation position.
func (spl *SyncPeerList) RemovePeer(addr string) {
	spl.mu.Lock()
	defer spl.mu.Unlock()

	newPeers := make([]string, 0, len(spl.peers))
	removed := 0
	for i, p := range spl.peers {
		if p == addr {
			if i <= spl.currentIndex {
				removed++
			}
			continue
		}
		newPeers = append(newPeers, p)
	}

	spl.peers = newPeers
	spl.currentIndex -= removed

	// Clamp index
	if len(spl.peers) == 0 {
		spl.currentIndex = 0
	} else if spl.currentIndex < 0 {
		spl.currentIndex = 0
	} else if spl.currentIndex >= len(spl.peers) {
		spl.currentIndex = 0
	}
}

// Size returns the number of peers in the list
func (spl *SyncPeerList) Size() int {
	spl.mu.RLock()
	defer spl.mu.RUnlock()

	return len(spl.peers)
}

// Reset clears the list and resets counters
func (spl *SyncPeerList) Reset() {
	spl.mu.Lock()
	defer spl.mu.Unlock()

	spl.peers = make([]string, 0)
	spl.currentIndex = 0
	spl.roundCount = 0
}

// ShouldRebuild returns true if the list should be rebuilt
func (spl *SyncPeerList) ShouldRebuild() bool {
	spl.mu.RLock()
	defer spl.mu.RUnlock()

	// Rebuild every 3 complete rounds
	return spl.roundCount > 0 && spl.roundCount%3 == 0
}

// GetCurrentIndex returns the current rotation index
func (spl *SyncPeerList) GetCurrentIndex() int {
	spl.mu.RLock()
	defer spl.mu.RUnlock()

	return spl.currentIndex
}
