package mempool

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/twins-dev/twins-core/pkg/types"
)

// AddTransactionFromPeer adds a transaction from a specific peer with rate limiting
func (mp *TxMempool) AddTransactionFromPeer(tx *types.Transaction, peerID string) error {
	// Check peer rate limits
	if err := mp.checkPeerRateLimit(peerID); err != nil {
		return err
	}

	// Check if peer is banned
	if mp.isPeerBanned(peerID) {
		return NewMempoolError(RejectRateLimited,
			fmt.Sprintf("peer %s is banned", peerID),
			tx.Hash())
	}

	// Try to add transaction
	err := mp.AddTransaction(tx)
	if err != nil {
		// Track rejection
		mp.trackPeerRejection(peerID, tx.Hash())
		return err
	}

	// Track successful add
	mp.trackPeerTransaction(peerID)

	return nil
}

// checkPeerRateLimit checks if a peer exceeds rate limits
func (mp *TxMempool) checkPeerRateLimit(peerID string) error {
	mp.peerStatsMu.Lock()
	defer mp.peerStatsMu.Unlock()

	stats, exists := mp.peerStats[peerID]
	if !exists {
		// Initialize peer stats
		mp.peerStats[peerID] = &PeerStats{
			TxCount:    0,
			LastTxTime: time.Now(),
		}
		return nil
	}

	// Check total transaction limit
	if stats.TxCount >= mp.config.MaxTxsPerPeer {
		return NewMempoolError(RejectRateLimited,
			fmt.Sprintf("peer %s exceeded max transactions (%d)", peerID, mp.config.MaxTxsPerPeer),
			types.ZeroHash)
	}

	// Check per-second rate limit
	now := time.Now()
	timeSinceLastTx := now.Sub(stats.LastTxTime)
	minInterval := time.Second / time.Duration(mp.config.MaxTxsPerSecond)

	if timeSinceLastTx < minInterval {
		return NewMempoolError(RejectRateLimited,
			fmt.Sprintf("peer %s sending too fast", peerID),
			types.ZeroHash)
	}

	return nil
}

// isPeerBanned checks if a peer is currently banned.
// Uses write lock because expired bans are cleared as a side effect.
func (mp *TxMempool) isPeerBanned(peerID string) bool {
	mp.peerStatsMu.Lock()
	defer mp.peerStatsMu.Unlock()

	stats, exists := mp.peerStats[peerID]
	if !exists {
		return false
	}

	if stats.Banned && time.Now().Before(stats.BanExpiry) {
		return true
	}

	// Unban if expiry passed
	if stats.Banned {
		stats.Banned = false
	}

	return false
}

// trackPeerTransaction tracks a successful transaction from a peer
func (mp *TxMempool) trackPeerTransaction(peerID string) {
	mp.peerStatsMu.Lock()
	defer mp.peerStatsMu.Unlock()

	stats, exists := mp.peerStats[peerID]
	if !exists {
		mp.peerStats[peerID] = &PeerStats{
			TxCount:    1,
			LastTxTime: time.Now(),
		}
		return
	}

	stats.TxCount++
	stats.LastTxTime = time.Now()
}

// trackPeerRejection tracks a rejected transaction from a peer
func (mp *TxMempool) trackPeerRejection(peerID string, txHash types.Hash) {
	mp.peerStatsMu.Lock()
	defer mp.peerStatsMu.Unlock()

	stats, exists := mp.peerStats[peerID]
	if !exists {
		mp.peerStats[peerID] = &PeerStats{
			Rejections:   1,
			LastRejected: time.Now(),
		}
		return
	}

	stats.Rejections++
	stats.LastRejected = time.Now()

	// Check if we should ban this peer
	recentRejectionRate := stats.Rejections
	if recentRejectionRate > mp.config.MaxRejectionsRate {
		mp.banPeer(peerID, stats)
	}
}

// banPeer bans a peer for the configured duration
func (mp *TxMempool) banPeer(peerID string, stats *PeerStats) {
	stats.Banned = true
	stats.BanExpiry = time.Now().Add(mp.config.BanDuration)

	mp.logger.WithFields(logrus.Fields{
		"peer":       peerID,
		"rejections": stats.Rejections,
		"duration":   mp.config.BanDuration,
	}).Warn("Peer banned for excessive rejections")
}

// UnbanPeer manually unbans a peer
func (mp *TxMempool) UnbanPeer(peerID string) error {
	mp.peerStatsMu.Lock()
	defer mp.peerStatsMu.Unlock()

	stats, exists := mp.peerStats[peerID]
	if !exists {
		return fmt.Errorf("peer not found")
	}

	stats.Banned = false
	stats.BanExpiry = time.Time{}
	stats.Rejections = 0

	mp.logger.WithField("peer", peerID).Info("Peer unbanned")

	return nil
}

// GetPeerStats returns statistics for a specific peer
func (mp *TxMempool) GetPeerStats(peerID string) (*PeerStats, error) {
	mp.peerStatsMu.RLock()
	defer mp.peerStatsMu.RUnlock()

	stats, exists := mp.peerStats[peerID]
	if !exists {
		return nil, fmt.Errorf("peer not found")
	}

	// Return a copy
	statsCopy := *stats
	return &statsCopy, nil
}

// GetBannedPeers returns a list of currently banned peers
func (mp *TxMempool) GetBannedPeers() []string {
	mp.peerStatsMu.RLock()
	defer mp.peerStatsMu.RUnlock()

	banned := make([]string, 0)
	now := time.Now()

	for peerID, stats := range mp.peerStats {
		if stats.Banned && now.Before(stats.BanExpiry) {
			banned = append(banned, peerID)
		}
	}

	return banned
}

// ResetPeerStats resets statistics for all peers
func (mp *TxMempool) ResetPeerStats() {
	mp.peerStatsMu.Lock()
	defer mp.peerStatsMu.Unlock()

	mp.peerStats = make(map[string]*PeerStats)

	mp.logger.Debug("Peer statistics reset")
}

// CleanPeerStats removes stale peer statistics
func (mp *TxMempool) CleanPeerStats() {
	mp.peerStatsMu.Lock()
	defer mp.peerStatsMu.Unlock()

	staleThreshold := time.Now().Add(-1 * time.Hour)
	removed := 0

	for peerID, stats := range mp.peerStats {
		// Remove if not banned and hasn't sent transactions recently
		if !stats.Banned && stats.LastTxTime.Before(staleThreshold) {
			delete(mp.peerStats, peerID)
			removed++
		}
	}

	if removed > 0 {
		mp.logger.WithField("count", removed).Debug("Cleaned stale peer statistics")
	}
}

// UpdatePeerTxLimit updates the transaction limit for a specific peer
func (mp *TxMempool) UpdatePeerTxLimit(peerID string, newLimit int) error {
	mp.peerStatsMu.Lock()
	defer mp.peerStatsMu.Unlock()

	stats, exists := mp.peerStats[peerID]
	if !exists {
		return fmt.Errorf("peer not found")
	}

	// Store old limit for logging
	oldCount := stats.TxCount

	// Reset count if new limit is higher
	if newLimit > mp.config.MaxTxsPerPeer {
		stats.TxCount = 0
	}

	mp.logger.WithFields(logrus.Fields{
		"peer":      peerID,
		"old_count": oldCount,
		"new_limit": newLimit,
	}).Debug("Updated peer transaction limit")

	return nil
}

// GetRateLimitConfig returns the current rate limit configuration
func (mp *TxMempool) GetRateLimitConfig() logrus.Fields {
	return logrus.Fields{
		"max_txs_per_peer":      mp.config.MaxTxsPerPeer,
		"max_txs_per_second":    mp.config.MaxTxsPerSecond,
		"ban_duration":          mp.config.BanDuration,
		"max_rejections_rate":   mp.config.MaxRejectionsRate,
	}
}

// SetRateLimitConfig updates rate limiting configuration
func (mp *TxMempool) SetRateLimitConfig(maxTxsPerPeer, maxTxsPerSecond, maxRejectionsRate int, banDuration time.Duration) {
	mp.config.MaxTxsPerPeer = maxTxsPerPeer
	mp.config.MaxTxsPerSecond = maxTxsPerSecond
	mp.config.MaxRejectionsRate = maxRejectionsRate
	mp.config.BanDuration = banDuration

	mp.logger.WithFields(logrus.Fields{
		"max_txs_per_peer":    maxTxsPerPeer,
		"max_txs_per_second":  maxTxsPerSecond,
		"max_rejections_rate": maxRejectionsRate,
		"ban_duration":        banDuration,
	}).Info("Rate limit configuration updated")
}

// IsRateLimited checks if adding a transaction would trigger rate limiting
func (mp *TxMempool) IsRateLimited(peerID string) bool {
	return mp.checkPeerRateLimit(peerID) != nil || mp.isPeerBanned(peerID)
}