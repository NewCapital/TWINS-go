package blockchain

import (
	"time"

	"github.com/twins-dev/twins-core/pkg/types"
)

// ChainStatus provides informational chain health metrics.
// CRITICAL ARCHITECTURE DECISION: IsStale is INFORMATIONAL ONLY.
// It must NEVER be used to block any operation (staking, validation, etc).
// This is a direct lesson from the legacy C++ deadlock that killed the chain.
// See: legacy/src/masternode-sync.cpp:56 for the bug we're avoiding.
type ChainStatus struct {
	// Chain tip information
	Height       uint32
	BestHash     types.Hash
	BestTime     time.Time
	Difficulty   uint32
	StakeWeight  uint64

	// Sync status
	IsSyncing        bool    // Currently syncing blocks
	IsSynced         bool    // Caught up with network
	SyncProgress     float64 // 0.0 to 1.0 sync progress
	NetworkHeight    uint32  // Network consensus height
	BlocksBehind     uint32  // How many blocks behind network

	// Staleness indicators (INFORMATIONAL ONLY - never blocks operations)
	// WARNING: Do NOT use these fields to gate any functionality!
	IsStale          bool          // Last block > StaleThreshold old
	StaleReason      string        // Human-readable reason if stale
	TimeSinceLastBlock time.Duration // Time since last block

	// Peer information
	PeerCount        int
	ConnectedPeers   int

	// Performance metrics
	LastBlockProcessTime time.Duration
	AverageBlockTime     time.Duration
}

const (
	// StaleThreshold is the duration after which a chain is considered stale.
	// This is INFORMATIONAL ONLY and must NEVER block any operations.
	// Legacy used 1 hour which caused the deadlock - we use 2 hours for monitoring only.
	StaleThreshold = 2 * time.Hour
)

// GetChainStatus returns current chain health metrics.
// All fields are informational only and should never be used to block operations.
func (bc *BlockChain) GetChainStatus() *ChainStatus {
	bc.mu.RLock()
	bestBlock := bc.bestBlock
	bestHeight := bc.bestHeight.Load()
	bestHash := bc.bestHash
	bc.mu.RUnlock()

	bc.consensusMu.RLock()
	networkHeight := bc.networkConsensusHeight
	peerCount := bc.networkPeerCount
	bc.consensusMu.RUnlock()

	status := &ChainStatus{
		Height:        bestHeight,
		BestHash:      bestHash,
		NetworkHeight: networkHeight,
	}

	// Calculate blocks behind
	if networkHeight > bestHeight {
		status.BlocksBehind = networkHeight - bestHeight
	}

	// Set sync progress
	if networkHeight > 0 {
		status.SyncProgress = float64(bestHeight) / float64(networkHeight)
		if status.SyncProgress > 1.0 {
			status.SyncProgress = 1.0
		}
	}

	// Populate peer information
	status.PeerCount = int(peerCount)
	status.ConnectedPeers = int(peerCount)

	// Determine sync state (height-based, NOT time-based)
	// Requires MinSyncPeers connected peers for reliable consensus
	if peerCount < MinSyncPeers || networkHeight == 0 {
		status.IsSyncing = false
		status.IsSynced = false
	} else {
		status.IsSyncing = status.BlocksBehind >= bc.ibdThreshold
		status.IsSynced = !status.IsSyncing
	}

	// Get best block time and calculate staleness (INFORMATIONAL ONLY)
	if bestBlock != nil {
		status.BestTime = time.Unix(int64(bestBlock.Header.Timestamp), 0)
		status.Difficulty = bestBlock.Header.Bits
		status.TimeSinceLastBlock = time.Since(status.BestTime)

		// Staleness is INFORMATIONAL ONLY - never blocks any operation
		if status.TimeSinceLastBlock > StaleThreshold {
			status.IsStale = true
			status.StaleReason = "last block is older than 2 hours"
		}
	} else {
		status.IsStale = true
		status.StaleReason = "no blocks in chain"
	}

	// Note: Performance metrics like LastBlockProcessTime and AverageBlockTime
	// would require additional tracking infrastructure not yet implemented.
	// These fields remain at zero values for now.

	return status
}

// GetSyncProgress returns the current sync progress as a percentage (0-100).
// This is a convenience method for RPC calls.
func (bc *BlockChain) GetSyncProgress() float64 {
	status := bc.GetChainStatus()
	return status.SyncProgress * 100
}
