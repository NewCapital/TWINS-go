// Copyright (c) 2025 The TWINS Core developers
// Distributed under the MIT software license

package p2p

import (
	"sync"
	"sync/atomic"

	"github.com/sirupsen/logrus"
)

// MasternodeTier represents the tier of a masternode
type MasternodeTier uint32

const (
	TierNone     MasternodeTier = 0
	TierBronze   MasternodeTier = 1  // 1M TWINS
	TierSilver   MasternodeTier = 2  // 5M TWINS
	TierGold     MasternodeTier = 3  // 20M TWINS
	TierPlatinum MasternodeTier = 4  // 100M TWINS
)

// Service flags for masternode identification
const (
	ServiceFlagMasternode       ServiceFlag = 1 << 5  // Generic masternode flag
	ServiceFlagMasternodeBronze ServiceFlag = 1 << 6  // Bronze tier masternode
	ServiceFlagMasternodeSilver ServiceFlag = 1 << 7  // Silver tier masternode
	ServiceFlagMasternodeGold   ServiceFlag = 1 << 8  // Gold tier masternode
	ServiceFlagMasternodePlat   ServiceFlag = 1 << 9  // Platinum tier masternode
)

// MasternodePeerManager manages masternode-aware peer connections
type MasternodePeerManager struct {
	logger *logrus.Entry
	mu     sync.RWMutex

	// Masternode peer tracking
	masternodesByTier map[MasternodeTier][]*Peer
	masternodeCount   map[MasternodeTier]int

	// Configuration
	minMasternodeConnections int
	preferMasternodes       bool
	masternodePriority      map[MasternodeTier]int // Priority score for each tier

	// Statistics
	totalMasternodeConnections atomic.Uint64
	syncFromMasternodes       atomic.Uint64
	masternodeDiscoveries     atomic.Uint64
}

// NewMasternodePeerManager creates a new masternode peer manager
func NewMasternodePeerManager(logger *logrus.Logger) *MasternodePeerManager {
	return &MasternodePeerManager{
		logger:                   logger.WithField("component", "masternode-peers"),
		masternodesByTier:        make(map[MasternodeTier][]*Peer),
		masternodeCount:          make(map[MasternodeTier]int),
		minMasternodeConnections: 3,
		preferMasternodes:        true,
		masternodePriority: map[MasternodeTier]int{
			TierPlatinum: 100,
			TierGold:     75,
			TierSilver:   50,
			TierBronze:   25,
			TierNone:     0,
		},
	}
}

// GetMasternodeTier determines the masternode tier from service flags
func (mpm *MasternodePeerManager) GetMasternodeTier(services ServiceFlag) MasternodeTier {
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
	if services&ServiceFlagMasternode != 0 {
		return TierBronze // Default to bronze if generic flag is set
	}
	return TierNone
}

// RegisterPeer registers a peer and tracks if it's a masternode
func (mpm *MasternodePeerManager) RegisterPeer(peer *Peer) {
	mpm.mu.Lock()
	defer mpm.mu.Unlock()

	tier := mpm.GetMasternodeTier(peer.services)
	if tier == TierNone {
		return // Not a masternode
	}

	// Add to tier tracking
	mpm.masternodesByTier[tier] = append(mpm.masternodesByTier[tier], peer)
	mpm.masternodeCount[tier]++
	mpm.totalMasternodeConnections.Add(1)
	mpm.masternodeDiscoveries.Add(1)

	mpm.logger.WithFields(logrus.Fields{
		"peer":     peer.GetAddress().String(),
		"tier":     mpm.tierToString(tier),
		"services": peer.services,
	}).Debug("🏆 Discovered masternode peer")
}

// UnregisterPeer removes a peer from masternode tracking
func (mpm *MasternodePeerManager) UnregisterPeer(peer *Peer) {
	mpm.mu.Lock()
	defer mpm.mu.Unlock()

	tier := mpm.GetMasternodeTier(peer.services)
	if tier == TierNone {
		return
	}

	// Remove from tier list
	if peers, exists := mpm.masternodesByTier[tier]; exists {
		for i, p := range peers {
			if p == peer {
				// Remove peer from slice
				mpm.masternodesByTier[tier] = append(peers[:i], peers[i+1:]...)
				mpm.masternodeCount[tier]--
				break
			}
		}
	}

	mpm.logger.WithFields(logrus.Fields{
		"peer": peer.GetAddress().String(),
		"tier": mpm.tierToString(tier),
	}).Debug("Removed masternode peer")
}

// GetBestMasternodes returns the best masternode peers for sync
func (mpm *MasternodePeerManager) GetBestMasternodes(count int) []*Peer {
	mpm.mu.RLock()
	defer mpm.mu.RUnlock()

	result := make([]*Peer, 0, count)

	// Prioritize by tier (Platinum > Gold > Silver > Bronze)
	tiers := []MasternodeTier{TierPlatinum, TierGold, TierSilver, TierBronze}

	for _, tier := range tiers {
		if peers, exists := mpm.masternodesByTier[tier]; exists {
			for _, peer := range peers {
				if peer.IsConnected() && peer.IsHandshakeComplete() {
					result = append(result, peer)
					if len(result) >= count {
						return result
					}
				}
			}
		}
	}

	return result
}

// ShouldPreferMasternode determines if we should prefer a masternode for sync
func (mpm *MasternodePeerManager) ShouldPreferMasternode(peer *Peer) bool {
	if !mpm.preferMasternodes {
		return false
	}

	tier := mpm.GetMasternodeTier(peer.services)
	return tier != TierNone
}

// GetMasternodePriority returns the priority score for a peer
func (mpm *MasternodePeerManager) GetMasternodePriority(peer *Peer) int {
	tier := mpm.GetMasternodeTier(peer.services)
	return mpm.masternodePriority[tier]
}

// GetMasternodeCount returns the number of connected masternodes by tier
func (mpm *MasternodePeerManager) GetMasternodeCount() map[MasternodeTier]int {
	mpm.mu.RLock()
	defer mpm.mu.RUnlock()

	result := make(map[MasternodeTier]int)
	for tier, count := range mpm.masternodeCount {
		result[tier] = count
	}
	return result
}

// NeedMoreMasternodes checks if we need more masternode connections
func (mpm *MasternodePeerManager) NeedMoreMasternodes() bool {
	mpm.mu.RLock()
	defer mpm.mu.RUnlock()

	totalMasternodes := 0
	for _, count := range mpm.masternodeCount {
		totalMasternodes += count
	}

	return totalMasternodes < mpm.minMasternodeConnections
}

// GetStatistics returns masternode peer statistics
func (mpm *MasternodePeerManager) GetStatistics() map[string]interface{} {
	mpm.mu.RLock()
	defer mpm.mu.RUnlock()

	stats := map[string]interface{}{
		"total_masternode_connections": mpm.totalMasternodeConnections.Load(),
		"sync_from_masternodes":        mpm.syncFromMasternodes.Load(),
		"masternode_discoveries":       mpm.masternodeDiscoveries.Load(),
		"masternode_counts":            mpm.masternodeCount,
		"prefer_masternodes":           mpm.preferMasternodes,
	}

	// Add tier-specific stats
	for tier, peers := range mpm.masternodesByTier {
		tierName := mpm.tierToString(tier)
		stats[tierName+"_count"] = len(peers)
	}

	return stats
}

// tierToString converts a tier to a string representation
func (mpm *MasternodePeerManager) tierToString(tier MasternodeTier) string {
	switch tier {
	case TierPlatinum:
		return "Platinum"
	case TierGold:
		return "Gold"
	case TierSilver:
		return "Silver"
	case TierBronze:
		return "Bronze"
	default:
		return "None"
	}
}

// MasternodeDNSSeeds returns DNS seeds that are known to be masternodes
func GetMasternodeDNSSeeds(network string) []string {
	switch network {
	case "mainnet":
		return []string{
			"masternode1.twins.dev",
			"masternode2.twins.dev",
			"masternode3.twins.dev",
			"mn-seed1.twins.network",
			"mn-seed2.twins.network",
		}
	case "testnet":
		return []string{
			"testnet-mn1.twins.dev",
			"testnet-mn2.twins.dev",
		}
	default:
		return []string{}
	}
}

// GetMasternodeBootstrapAddresses returns hardcoded masternode addresses for bootstrap
func GetMasternodeBootstrapAddresses(network string) []string {
	switch network {
	case "mainnet":
		return []string{
			"45.32.220.182:37817",   // Known Platinum masternode
			"144.202.86.130:37817",  // Known Gold masternode
			"45.77.227.147:37817",   // Known Gold masternode
			"207.148.18.151:37817",  // Known Silver masternode
			"95.179.236.171:37817",  // Known Silver masternode
		}
	case "testnet":
		return []string{
			"testnet1.twins.dev:37819",
			"testnet2.twins.dev:37819",
		}
	default:
		return []string{}
	}
}

// ScorePeerForSync calculates a sync priority score for a peer
func (mpm *MasternodePeerManager) ScorePeerForSync(peer *Peer, qualityScore int32) int32 {
	// Start with quality score
	score := qualityScore

	// Add masternode tier bonus
	tier := mpm.GetMasternodeTier(peer.services)
	tierBonus := int32(mpm.masternodePriority[tier])

	// Weight the tier bonus (20% of total score potential)
	score = score + (tierBonus * 20 / 100)

	// Boost score if we're low on masternodes
	if mpm.NeedMoreMasternodes() && tier != TierNone {
		score += 10
	}

	// Track that we're syncing from a masternode
	if tier != TierNone {
		mpm.syncFromMasternodes.Add(1)
	}

	return score
}

// UpdatePeerDiscovery modifies peer discovery to prefer masternodes
func (mpm *MasternodePeerManager) UpdatePeerDiscovery(server *Server) {
	// Add masternode DNS seeds
	masternodeSeeds := GetMasternodeDNSSeeds(server.params.Name)
	for _, seed := range masternodeSeeds {
		server.logger.WithField("seed", seed).Debug("🌐 Adding masternode DNS seed")
		// This would be handled by the DNS resolver in the server
	}

	// Add bootstrap masternode addresses
	bootstrapAddrs := GetMasternodeBootstrapAddresses(server.params.Name)
	for _, addr := range bootstrapAddrs {
		if err := server.ConnectNode(addr); err != nil {
			server.logger.WithError(err).WithField("addr", addr).
				Warn("Failed to connect to bootstrap masternode")
		} else {
			server.logger.WithField("addr", addr).
				Info("🏆 Connecting to bootstrap masternode")
		}
	}
}