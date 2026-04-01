package masternode

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/twins-dev/twins-core/internal/masternode/debug"
	"github.com/twins-dev/twins-core/internal/spork"
	"github.com/twins-dev/twins-core/pkg/types"
)

// Sync state constants matching legacy CMasternodeSync
// See legacy/src/masternode-sync.h
const (
	SyncInitial  SyncState = 0   // Initial state, waiting to start
	SyncSporks   SyncState = 1   // Syncing sporks
	SyncList     SyncState = 2   // Syncing masternode list
	SyncMNW      SyncState = 3   // Syncing masternode winners
	SyncBudget   SyncState = 4   // Syncing budget (skipped - budget disabled via SPORK_13)
	SyncFailed   SyncState = 998 // Sync failed
	SyncFinished SyncState = 999 // Sync complete
)

// Sync timing constants from legacy
const (
	SyncTimeout   = 10 * time.Second // Timeout between sync attempts (increased from 5s to allow mnb responses)
	// SyncThreshold is the minimum number of peer confirmations required before
	// advancing to the next sync stage. Value 2 matches legacy C++ MASTERNODE_SYNC_THRESHOLD
	// and ensures we've heard from at least 2 peers to avoid single-source bias.
	SyncThreshold = 2
)

// SleepDetectionThreshold is the max seconds between Process() calls before
// assuming the client was asleep and resetting sync state.
// Legacy: if (GetTime() - lastProcess > 60 * 60)
const SleepDetectionThreshold int64 = 3600

// SyncState represents the current masternode sync state
type SyncState int

// String returns a human-readable sync state name
func (s SyncState) String() string {
	switch s {
	case SyncInitial:
		return "INITIAL"
	case SyncSporks:
		return "SPORKS"
	case SyncList:
		return "LIST"
	case SyncMNW:
		return "MNW"
	case SyncBudget:
		return "BUDGET"
	case SyncFailed:
		return "FAILED"
	case SyncFinished:
		return "FINISHED"
	default:
		return "UNKNOWN"
	}
}

// SyncManager manages masternode synchronization state
// Implements the CMasternodeSync state machine from legacy C++
//
// Lock ordering: mu protects all state fields. Methods that read state use RLock,
// methods that modify state use Lock. The processLoop goroutine calls Process()
// which acquires Lock internally.
//
// Protected fields (require mu):
//   - currentState, requestAttempt, assetSyncStarted
//   - seenMNBroadcasts, seenMNWinners
//   - lastMasternodeList, lastMasternodeWinner
//   - sumMasternodeList, sumMasternodeWinner, countMasternodeList, countMasternodeWinner
//   - lastFailure, failureCount
//   - blockchain, peerRequester, getMasternodeCount
//   - lastProcess, blockchainSynced
//   - ctx, cancelFunc, running
type SyncManager struct {
	mu sync.RWMutex

	// Current sync state (protected by mu)
	currentState     SyncState
	requestAttempt   int   // Number of attempts at current sync stage
	assetSyncStarted int64 // Unix timestamp when current stage started

	// Tracking maps for deduplication (like mapSeenSyncMNB, mapSeenSyncMNW)
	seenMNBroadcasts map[types.Hash]int // hash -> confirmation count
	seenMNWinners    map[types.Hash]int // hash -> confirmation count

	// Last activity timestamps
	lastMasternodeList   int64
	lastMasternodeWinner int64

	// Counters for sync progress
	sumMasternodeList     int
	sumMasternodeWinner   int
	countMasternodeList   int
	countMasternodeWinner int

	// Failure tracking
	lastFailure  int64
	failureCount int

	// Blockchain interface for IsBlockchainSynced check
	blockchain BlockchainSyncer

	// P2P interface for sending sync requests
	peerRequester SyncPeerRequester

	// Masternode count getter for mnget requests
	getMasternodeCount func() int

	// Last process time for sleep/wake detection
	lastProcess int64

	// Cached blockchain synced state
	blockchainSynced bool

	// hadMasternodes tracks whether CountEnabled() > 0 was ever observed in FINISHED state.
	// Used to prevent infinite reset loops on networks with no valid masternodes.
	//
	// Re-arming: the flag is set to true each time getMasternodeCount() > 0 is observed
	// while in SyncFinished. After a legitimate reset (masternodes expired → resync →
	// resetLocked clears the flag), the node re-arms automatically the next time masternodes
	// become available. If a resync completes with 0 masternodes the flag stays false and
	// no further automatic resync is triggered until at least one masternode is observed.
	hadMasternodes bool

	// Network sync status provider (P2P server)
	networkSyncStatus NetworkSyncStatusProvider

	// Process loop control
	ctx        context.Context
	cancelFunc context.CancelFunc
	running    bool
	wg         sync.WaitGroup // WaitGroup for goroutine cleanup

	// Spork manager for SPORK_8 check
	sporkManager SporkInterface

	// Debug event collector (nil when disabled)
	debugCollector atomic.Pointer[debug.Collector]

	// Cache freshness for quick-restart sync skip (push model to avoid lock ordering)
	// Set by NotifyCacheLoaded after LoadCache completes, outside Manager.mu
	cacheLoadedAt    time.Time // file modification time of mncache.dat
	cacheLoadedCount int       // masternodes loaded from cache

	// Per-peer fulfilled request tracking (like legacy pnode->HasFulfilledRequest)
	// Cleared on sync reset, tracks which peers have been asked for each sync type.
	// Maps peer address -> unix timestamp when we asked (for 3h cooldown matching).
	// Persisted to mncache.dat via WeAskedForList/WeAskedForEntry fields.
	fulfilledMNSync  map[string]int64 // peer address -> unix timestamp when asked for mnsync (LIST)
	fulfilledMNWSync map[string]int64 // peer address -> unix timestamp when asked for mnwsync (MNW)
	// peerSSCResponses tracks the latest ssc count per peer address for the current SyncList
	// visit. Entries are overwritten on repeated ssc messages from the same peer. The map is
	// scoped to a single SyncList visit — it is only cleared by resetLocked(), which is called
	// on every path that re-enters SyncList (full reset, SyncFailed recovery). Do not read
	// this map outside of the SyncList case in ProcessSyncStatusCount.
	peerSSCResponses map[string]int

	logger *logrus.Entry
}

// BlockchainSyncer provides blockchain state queries needed by the sync manager.
// Implemented by the blockchain layer to let SyncManager determine whether the
// local chain is fully downloaded (IBD complete) and to look up blocks by height.
type BlockchainSyncer interface {
	// GetBestHeight returns the height of the local chain tip.
	GetBestHeight() (uint32, error)
	// GetBlockByHeight returns the block at the given height.
	GetBlockByHeight(height uint32) (*types.Block, error)
	// IsInitialBlockDownload returns true while the node is still downloading the chain.
	IsInitialBlockDownload() bool
}

// NetworkSyncStatusProvider reports whether the node is synced with network peers.
// Implemented by the P2P server which compares local height to peer consensus height.
type NetworkSyncStatusProvider interface {
	// IsSynced returns true when the local chain height is close to the
	// consensus height reported by connected peers.
	IsSynced() bool
}

// SyncPeerRequester sends masternode sync requests to connected peers.
// Implemented by the P2P server to let SyncManager drive the sync protocol.
type SyncPeerRequester interface {
	// RequestSporks sends getsporks to connected peers
	RequestSporks() error
	// RequestMasternodeList sends dseg (DsegUpdate) to request masternode list
	// Returns (sent, skipped, error) where sent = peers we sent dseg to,
	// skipped = peers already asked within cooldown period
	RequestMasternodeList() (sent int, skipped int, err error)
	// RequestMasternodeWinners sends mnget to request masternode winners
	// Returns (sent, skipped, error) like RequestMasternodeList
	RequestMasternodeWinners(mnCount int) (sent int, skipped int, err error)
	// GetConnectedPeerCount returns number of connected peers
	GetConnectedPeerCount() int
}

// NewSyncManager creates a new masternode sync manager
func NewSyncManager(logger *logrus.Logger) *SyncManager {
	if logger == nil {
		logger = logrus.New()
	}

	sm := &SyncManager{
		currentState:     SyncInitial,
		seenMNBroadcasts: make(map[types.Hash]int),
		seenMNWinners:    make(map[types.Hash]int),
		assetSyncStarted: time.Now().Unix(),
		lastProcess:      time.Now().Unix(),
		fulfilledMNSync:  make(map[string]int64),
		fulfilledMNWSync: make(map[string]int64),
		peerSSCResponses: make(map[string]int),
		logger:           logger.WithField("component", "masternode-sync"),
	}

	return sm
}

// SetBlockchain sets the blockchain interface for sync status checks
func (sm *SyncManager) SetBlockchain(bc BlockchainSyncer) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.blockchain = bc
}

// SetNetworkSyncStatus sets the network sync status provider (P2P server)
// This is used by IsBlockchainSynced to check if we're synced with peers
func (sm *SyncManager) SetNetworkSyncStatus(provider NetworkSyncStatusProvider) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.networkSyncStatus = provider
}

// IsSynced returns true if masternode sync is complete
// Legacy: RequestedMasternodeAssets == MASTERNODE_SYNC_FINISHED
func (sm *SyncManager) IsSynced() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.currentState == SyncFinished
}

// IsMasternodeListSynced returns true if masternode list sync is complete
// Legacy: RequestedMasternodeAssets > MASTERNODE_SYNC_LIST
func (sm *SyncManager) IsMasternodeListSynced() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.currentState > SyncList
}

// IsBlockchainSynced checks if the blockchain is sufficiently synced
// Uses P2P sync status (current_height vs peer_consensus_height) instead of tip timestamp
func (sm *SyncManager) IsBlockchainSynced() bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now().Unix()

	// If last call was more than SleepDetectionThreshold ago, reset (client was in sleep mode)
	if now-sm.lastProcess > SleepDetectionThreshold {
		sm.resetLocked()
		sm.blockchainSynced = false
	}
	sm.lastProcess = now

	if sm.blockchainSynced {
		return true
	}

	if sm.blockchain != nil && sm.blockchain.IsInitialBlockDownload() {
		return false
	}

	// Use network sync status provider if available (preferred method)
	if sm.networkSyncStatus != nil {
		if sm.networkSyncStatus.IsSynced() {
			sm.blockchainSynced = true
			sm.logger.Info("Blockchain is synced (network consensus)")
			return true
		}
		return false
	}

	// Fallback: no network sync provider
	if sm.blockchain == nil {
		return false
	}

	height, err := sm.blockchain.GetBestHeight()
	if err != nil || height == 0 {
		return false
	}

	sm.logger.Warn("No network sync status provider, cannot determine sync state")
	return false
}

// GetSyncState returns the current sync state
func (sm *SyncManager) GetSyncState() SyncState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.currentState
}

// GetSyncStatus returns a human-readable sync status string
// Legacy: CMasternodeSync::GetSyncStatus()
func (sm *SyncManager) GetSyncStatus() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	switch sm.currentState {
	case SyncInitial:
		return "Synchronization pending..."
	case SyncSporks:
		return "Synchronizing sporks..."
	case SyncList:
		return "Synchronizing masternodes..."
	case SyncMNW:
		return "Synchronizing masternode winners..."
	case SyncBudget:
		return "Synchronizing budgets..."
	case SyncFailed:
		return "Synchronization failed"
	case SyncFinished:
		return "Synchronization finished"
	default:
		return "Unknown sync state"
	}
}

// Reset resets the sync state to initial
// Legacy: CMasternodeSync::Reset()
func (sm *SyncManager) Reset() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.resetLocked()
}

// resetLocked performs reset without holding lock (caller must hold lock)
func (sm *SyncManager) resetLocked() {
	sm.lastMasternodeList = 0
	sm.lastMasternodeWinner = 0
	sm.seenMNBroadcasts = make(map[types.Hash]int)
	sm.seenMNWinners = make(map[types.Hash]int)
	sm.lastFailure = 0
	sm.failureCount = 0
	sm.sumMasternodeList = 0
	sm.sumMasternodeWinner = 0
	sm.countMasternodeList = 0
	sm.countMasternodeWinner = 0
	sm.currentState = SyncInitial
	sm.requestAttempt = 0
	sm.assetSyncStarted = time.Now().Unix()
	sm.blockchainSynced = false
	sm.hadMasternodes = false

	// Clear per-peer fulfilled request tracking (like legacy ClearFulfilledRequest)
	sm.fulfilledMNSync = make(map[string]int64)
	sm.fulfilledMNWSync = make(map[string]int64)
	sm.peerSSCResponses = make(map[string]int)

	sm.logger.Debug("Masternode sync state reset")
}

// GetNextAsset advances to the next sync stage
// Legacy: CMasternodeSync::GetNextAsset()
func (sm *SyncManager) GetNextAsset() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.getNextAssetLocked()
}

// getNextAssetLocked advances state without holding lock
func (sm *SyncManager) getNextAssetLocked() {
	prevState := sm.currentState

	switch sm.currentState {
	case SyncInitial, SyncFailed:
		sm.currentState = SyncSporks
	case SyncSporks:
		sm.currentState = SyncList
	case SyncList:
		sm.currentState = SyncMNW
	case SyncMNW:
		// Skip budget sync (budget system disabled via SPORK_13)
		// Go directly to finished
		sm.currentState = SyncFinished
		sm.logger.Info("Masternode sync has finished")
	case SyncBudget:
		// In case we somehow got to budget state
		sm.currentState = SyncFinished
		sm.logger.Info("Masternode sync has finished")
	}

	sm.requestAttempt = 0
	sm.assetSyncStarted = time.Now().Unix()

	if prevState != sm.currentState {
		sm.logger.WithFields(logrus.Fields{
			"prev_state": prevState.String(),
			"new_state":  sm.currentState.String(),
		}).Debug("Sync state advanced")

		// Emit debug event on state transition
		if dc := sm.debugCollector.Load(); dc != nil && dc.IsEnabled() {
			dc.EmitSync("sync_state_change", "local", fmt.Sprintf("Sync state: %s → %s", prevState.String(), sm.currentState.String()), map[string]any{
				"prev_state": prevState.String(),
				"new_state":  sm.currentState.String(),
				"attempt":    sm.requestAttempt,
			})
		}
	}
}

// AddedMasternodeList records a received masternode broadcast
// Legacy: CMasternodeSync::AddedMasternodeList()
func (sm *SyncManager) AddedMasternodeList(hash types.Hash) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	count, exists := sm.seenMNBroadcasts[hash]
	if exists {
		if count < SyncThreshold {
			sm.lastMasternodeList = time.Now().Unix()
			sm.seenMNBroadcasts[hash] = count + 1
		}
	} else {
		sm.lastMasternodeList = time.Now().Unix()
		sm.seenMNBroadcasts[hash] = 1
	}
}

// AddedMasternodeWinner records a received masternode winner vote
// Legacy: CMasternodeSync::AddedMasternodeWinner()
func (sm *SyncManager) AddedMasternodeWinner(hash types.Hash) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	count, exists := sm.seenMNWinners[hash]
	if exists {
		if count < SyncThreshold {
			sm.lastMasternodeWinner = time.Now().Unix()
			sm.seenMNWinners[hash] = count + 1
		}
	} else {
		sm.lastMasternodeWinner = time.Now().Unix()
		sm.seenMNWinners[hash] = 1
	}
}

// ProcessSyncStatusCount handles sync status count messages (ssc)
// Legacy: CMasternodeSync::ProcessMessage() for "ssc" command
// Updated to track per-peer responses for better sync advancement decisions
func (sm *SyncManager) ProcessSyncStatusCount(peerAddr string, itemID int, count int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.currentState >= SyncFinished {
		return
	}

	switch SyncState(itemID) {
	case SyncList:
		if SyncState(itemID) != sm.currentState {
			return
		}
		// Track per-peer response only while in LIST phase — entries are only
		// meaningful here and resetLocked() clears the map before re-entry.
		if peerAddr != "" {
			// Store the maximum count observed from this peer. A peer may retransmit
			// ssc with a lower count due to list churn or a race; keeping the maximum
			// ensures a peer cannot "downgrade" from a non-zero count to zero, which
			// would otherwise cause a spurious fast-path advance on networks that do
			// have masternodes.
			if existing, ok := sm.peerSSCResponses[peerAddr]; !ok || count > existing {
				sm.peerSSCResponses[peerAddr] = count
			}
		}
		// Note: sumMasternodeList and countMasternodeList are incremented before the
		// fast-path check below, so they reflect all ssc messages received, including
		// the triggering message when the fast-path fires.
		sm.sumMasternodeList += count
		sm.countMasternodeList++

		// Fast-path: if enough distinct peers have all responded with 0 masternodes,
		// the network is empty — advance immediately rather than waiting for the 20s
		// quiescence window. Uses peerSSCResponses (keyed by peer address) rather than
		// countMasternodeList so that a single peer sending repeated ssc(0) messages
		// cannot satisfy the threshold alone. The map stores the maximum count per peer,
		// so a peer that previously reported non-zero masternodes will not be counted as
		// zero even if it retransmits with a lower count.
		// Checked BEFORE updating lastMasternodeList so we skip the quiescence
		// timestamp update entirely when we are about to advance anyway.
		if len(sm.peerSSCResponses) >= SyncThreshold {
			allZero := true
			for _, cnt := range sm.peerSSCResponses {
				if cnt > 0 {
					allZero = false
					break
				}
			}
			if allZero {
				sm.logger.Info("All ssc responses indicate 0 masternodes, advancing from LIST")
				sm.getNextAssetLocked()
				sm.logger.WithFields(logrus.Fields{
					"peer":    peerAddr,
					"item_id": itemID,
					"count":   count,
				}).Debug("Processed sync status count (fast-path advance)")
				return // state has advanced; skip quiescence timestamp update
			}
		}

		// Update lastMasternodeList on ssc responses so the 20s quiescence window
		// in processSyncList() can fire.
		// NOTE: The C++ ssc handler does NOT update lastMasternodeList — it only
		// accumulates sum/count. In C++, lastMasternodeList is always fed by
		// AddedMasternodeList() MNB broadcasts. On a 0-masternode network no MNBs
		// arrive, so lastMasternodeList stays 0 and the quiescence path never fires.
		// Updating it here from ssc responses enables the 20s timer even when
		// broadcasts won't come (deliberate divergence from C++ to fix the stuck case).
		if sm.requestAttempt >= SyncThreshold {
			sm.lastMasternodeList = time.Now().Unix()
		}

	case SyncMNW:
		if SyncState(itemID) != sm.currentState {
			return
		}
		sm.sumMasternodeWinner += count
		sm.countMasternodeWinner++
	}

	sm.logger.WithFields(logrus.Fields{
		"peer":    peerAddr,
		"item_id": itemID,
		"count":   count,
	}).Debug("Processed sync status count")
}

// fulfilledCooldown is the duration during which a peer is considered "already asked".
// Matches legacy MASTERNODES_DSEG_SECONDS (3 hours).
const fulfilledCooldown = 3 * time.Hour

// HasFulfilledRequest checks if a peer has been asked for a sync request type within the cooldown window.
// Legacy: pnode->HasFulfilledRequest("mnsync") / pnode->HasFulfilledRequest("mnwsync")
func (sm *SyncManager) HasFulfilledRequest(peerAddr string, requestType string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	now := time.Now().Unix()
	cooldownSec := int64(fulfilledCooldown.Seconds())

	switch requestType {
	case "mnsync":
		ts, ok := sm.fulfilledMNSync[peerAddr]
		if !ok {
			return false
		}
		return now-ts < cooldownSec
	case "mnwsync":
		ts, ok := sm.fulfilledMNWSync[peerAddr]
		if !ok {
			return false
		}
		return now-ts < cooldownSec
	default:
		return false
	}
}

// FulfilledRequest marks a peer as having been asked for a sync request type.
// Stores the current unix timestamp for cooldown-based expiry.
// Legacy: pnode->FulfilledRequest("mnsync") / pnode->FulfilledRequest("mnwsync")
func (sm *SyncManager) FulfilledRequest(peerAddr string, requestType string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now().Unix()
	switch requestType {
	case "mnsync":
		sm.fulfilledMNSync[peerAddr] = now
	case "mnwsync":
		sm.fulfilledMNWSync[peerAddr] = now
	}

	sm.logger.WithFields(logrus.Fields{
		"peer":         peerAddr,
		"request_type": requestType,
	}).Debug("Marked peer as fulfilled for sync request")
}

// GetFulfilledPeerCount returns the number of peers that have been asked for a request type
func (sm *SyncManager) GetFulfilledPeerCount(requestType string) int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	switch requestType {
	case "mnsync":
		return len(sm.fulfilledMNSync)
	case "mnwsync":
		return len(sm.fulfilledMNWSync)
	default:
		return 0
	}
}

// GetSSCResponseCount returns the number of peers that have responded with ssc messages
func (sm *SyncManager) GetSSCResponseCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.peerSSCResponses)
}

// GetFulfilledMaps returns copies of the fulfilled request maps for cache persistence.
// The returned maps are safe to use without holding the SyncManager lock.
func (sm *SyncManager) GetFulfilledMaps() (mnsync map[string]int64, mnwsync map[string]int64) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	mnsync = make(map[string]int64, len(sm.fulfilledMNSync))
	for k, v := range sm.fulfilledMNSync {
		mnsync[k] = v
	}
	mnwsync = make(map[string]int64, len(sm.fulfilledMNWSync))
	for k, v := range sm.fulfilledMNWSync {
		mnwsync[k] = v
	}
	return mnsync, mnwsync
}

// SetFulfilledMaps restores fulfilled request maps from cache.
// Only mnsync (dseg) entries are restored because C++ peers persist dseg rate limits
// in mncache.dat (mAskedUsForMasternodeList, 3h cooldown).
// mnwsync (mnget) entries are NOT restored because C++ peers use per-connection
// tracking (pfrom->HasFulfilledRequest("mnget")) which resets on reconnect —
// restoring these would incorrectly block us from asking peers who would respond.
// Only entries within the cooldown window are imported; expired entries are dropped.
func (sm *SyncManager) SetFulfilledMaps(mnsync map[string]int64, mnwsync map[string]int64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now().Unix()
	cooldownSec := int64(fulfilledCooldown.Seconds())

	for k, v := range mnsync {
		if now-v < cooldownSec {
			sm.fulfilledMNSync[k] = v
		}
	}
	// mnwsync intentionally not restored — see comment above
	_ = mnwsync
}

// SetSynced forces the sync state to finished
// Used for fast-sync or when sync is not needed (e.g., after IBD)
func (sm *SyncManager) SetSynced() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.currentState = SyncFinished
	sm.blockchainSynced = true
	sm.logger.Info("Masternode sync marked as finished")
}

// SetFailed marks sync as failed
func (sm *SyncManager) SetFailed() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.currentState = SyncFailed
	sm.lastFailure = time.Now().Unix()
	sm.failureCount++

	sm.logger.WithField("failure_count", sm.failureCount).Warn("Masternode sync failed")
}

// ShouldRetry returns true if sync should be retried after failure
// Legacy: if (RequestedMasternodeAssets == MASTERNODE_SYNC_FAILED && lastFailure + (1 * 60) < GetTime())
func (sm *SyncManager) ShouldRetry() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if sm.currentState != SyncFailed {
		return false
	}

	// Retry after 1 minute
	return time.Now().Unix()-sm.lastFailure > 60
}

// IncrementAttempt increments the request attempt counter
func (sm *SyncManager) IncrementAttempt() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.requestAttempt++
}

// GetAttempt returns the current request attempt count
func (sm *SyncManager) GetAttempt() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.requestAttempt
}

// ShouldAdvanceFromList checks if we should advance from LIST sync stage
// Legacy logic: if (lastMasternodeList > 0 && lastMasternodeList < GetTime() - MASTERNODE_SYNC_TIMEOUT * 2 && RequestedMasternodeAttempt >= MASTERNODE_SYNC_THRESHOLD)
func (sm *SyncManager) ShouldAdvanceFromList() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if sm.currentState != SyncList {
		return false
	}

	now := time.Now().Unix()
	timeout := int64(SyncTimeout.Seconds() * 2)

	// Haven't received anything in a while, and we've tried enough times
	if sm.lastMasternodeList > 0 &&
		sm.lastMasternodeList < now-timeout &&
		sm.requestAttempt >= SyncThreshold {
		return true
	}

	return false
}

// ShouldAdvanceFromMNW checks if we should advance from MNW sync stage
func (sm *SyncManager) ShouldAdvanceFromMNW() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if sm.currentState != SyncMNW {
		return false
	}

	now := time.Now().Unix()
	timeout := int64(SyncTimeout.Seconds() * 2)

	// Haven't received anything in a while, and we've tried enough times
	if sm.lastMasternodeWinner > 0 &&
		sm.lastMasternodeWinner < now-timeout &&
		sm.requestAttempt >= SyncThreshold {
		return true
	}

	return false
}

// GetAssetSyncStarted returns when the current sync stage started
func (sm *SyncManager) GetAssetSyncStarted() int64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.assetSyncStarted
}

// HasTimedOut checks if current sync stage has timed out
// Legacy: if (lastMasternodeList == 0 && (RequestedMasternodeAttempt >= MASTERNODE_SYNC_THRESHOLD * 3 || GetTime() - nAssetSyncStarted > MASTERNODE_SYNC_TIMEOUT * 5))
func (sm *SyncManager) HasTimedOut() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	now := time.Now().Unix()
	longTimeout := int64(SyncTimeout.Seconds() * 5)

	// For list sync
	if sm.currentState == SyncList && sm.lastMasternodeList == 0 {
		if sm.requestAttempt >= SyncThreshold*3 || now-sm.assetSyncStarted > longTimeout {
			return true
		}
	}

	// For winner sync
	if sm.currentState == SyncMNW && sm.lastMasternodeWinner == 0 {
		if sm.requestAttempt >= SyncThreshold*3 || now-sm.assetSyncStarted > longTimeout {
			return true
		}
	}

	return false
}

// SetPeerRequester sets the P2P interface for sending sync requests
func (sm *SyncManager) SetPeerRequester(pr SyncPeerRequester) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.peerRequester = pr
}

// SetMasternodeCountGetter sets the function to get current masternode count
// This is needed for mnget requests which send the count
func (sm *SyncManager) SetMasternodeCountGetter(getter func() int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.getMasternodeCount = getter
}

// NotifyCacheLoaded records cache freshness after LoadCache completes.
// IMPORTANT: Must be called OUTSIDE Manager.mu to avoid lock ordering deadlock
// (SyncManager.mu → Manager.mu path exists via getMasternodeCount).
func (sm *SyncManager) NotifyCacheLoaded(loadedAt time.Time, count int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.cacheLoadedAt = loadedAt
	sm.cacheLoadedCount = count
	sm.logger.WithFields(logrus.Fields{
		"cached_count": count,
		"cache_age":    time.Since(loadedAt).Round(time.Second),
	}).Debug("Cache freshness recorded for sync skip")
}

// SetSporkManager sets the spork manager for SPORK_8 check during sync timeout handling
// Legacy: CMasternodeSync::Process() checks IsSporkActive(SPORK_8_MASTERNODE_PAYMENT_ENFORCEMENT)
func (sm *SyncManager) SetSporkManager(sporkMgr SporkInterface) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.sporkManager = sporkMgr
}

// StartProcessLoop starts the background goroutine that periodically processes sync state
// This implements the legacy CMasternodeSync::Process() tick loop from obfuscation.cpp:2298
// Legacy behavior: Process() is called every second, but only acts every MASTERNODE_SYNC_TIMEOUT (5) seconds
func (sm *SyncManager) StartProcessLoop() {
	sm.mu.Lock()
	if sm.running {
		sm.mu.Unlock()
		return
	}
	sm.ctx, sm.cancelFunc = context.WithCancel(context.Background())
	sm.running = true
	sm.wg.Add(1)
	sm.mu.Unlock()

	go sm.processLoop()
	sm.logger.Info("Masternode sync process loop started")
}

// StopProcessLoop stops the background process loop and waits for goroutine to exit
func (sm *SyncManager) StopProcessLoop() {
	sm.mu.Lock()
	if !sm.running {
		sm.mu.Unlock()
		return
	}

	if sm.cancelFunc != nil {
		sm.cancelFunc()
	}
	sm.running = false
	sm.mu.Unlock()

	// Wait for goroutine to exit (outside of lock to avoid deadlock)
	sm.wg.Wait()
	sm.logger.Info("Masternode sync process loop stopped")
}

// processLoop runs the periodic sync process
// Legacy: CMasternodeSync::Process() called every ~1 second, acts every MASTERNODE_SYNC_TIMEOUT
func (sm *SyncManager) processLoop() {
	defer sm.wg.Done()

	// Legacy uses a tick counter, we use a ticker at SyncTimeout interval
	ticker := time.NewTicker(SyncTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-sm.ctx.Done():
			return
		case <-ticker.C:
			sm.Process()
		}
	}
}

// Process performs one iteration of the sync state machine
// Implements legacy CMasternodeSync::Process() from masternode-sync.cpp:234-405
func (sm *SyncManager) Process() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Already synced - check if we lost all masternodes and need to resync
	if sm.currentState == SyncFinished {
		// Legacy: if (mnodeman.CountEnabled() == 0) Reset()
		// Guard: only reset if we previously had masternodes. On networks with no valid
		// masternodes CountEnabled() is permanently 0, and resetting just creates an
		// infinite INITIAL→SPORKS→LIST→MNW→FINISHED→INITIAL loop every ~3 minutes.
		if sm.getMasternodeCount != nil {
			count := sm.getMasternodeCount()
			if count > 0 {
				sm.hadMasternodes = true
			} else if sm.hadMasternodes {
				sm.logger.Warn("Lost all masternodes, resyncing")
				sm.resetLocked()
			}
		}
		return
	}

	// Handle failed state - retry after 1 minute
	// Legacy: if (RequestedMasternodeAssets == MASTERNODE_SYNC_FAILED && lastFailure + (1 * 60) < GetTime())
	if sm.currentState == SyncFailed {
		elapsed := time.Now().Unix() - sm.lastFailure
		if elapsed > 60 {
			sm.logger.Debug("Retrying masternode sync after failure")
			sm.resetLocked()
		}
		return
	}

	sm.logger.WithFields(logrus.Fields{
		"state":   sm.currentState.String(),
		"attempt": sm.requestAttempt,
	}).Debug("Processing masternode sync")

	// Initial state - advance to sporks
	if sm.currentState == SyncInitial {
		sm.getNextAssetLocked()
	}

	// Wait for blockchain sync before proceeding past sporks
	// Legacy: if (!IsBlockchainSynced() && RequestedMasternodeAssets > MASTERNODE_SYNC_SPORKS) return
	if sm.currentState > SyncSporks && !sm.blockchainSynced {
		// Check network sync status (uses peer consensus height, not tip timestamp)
		if sm.networkSyncStatus != nil && sm.networkSyncStatus.IsSynced() {
			sm.blockchainSynced = true
			sm.logger.Debug("Blockchain is synced, continuing masternode sync")
		} else if sm.blockchain != nil && sm.blockchain.IsInitialBlockDownload() {
			// Still in IBD, wait
			sm.logger.Debug("Waiting for blockchain sync before continuing masternode sync")
			return
		} else if sm.networkSyncStatus == nil {
			// No network status provider - shouldn't happen in normal operation
			sm.logger.Debug("Waiting for blockchain sync before continuing masternode sync")
			return
		} else {
			// Network says not synced yet
			sm.logger.Debug("Waiting for blockchain sync before continuing masternode sync")
			return
		}
	}

	// Check if we have peer requester configured
	if sm.peerRequester == nil {
		sm.logger.Debug("No peer requester configured, skipping sync process")
		return
	}

	// Check if we have any connected peers
	if sm.peerRequester.GetConnectedPeerCount() == 0 {
		sm.logger.Debug("No connected peers, skipping sync process")
		return
	}

	// Process based on current state
	switch sm.currentState {
	case SyncSporks:
		sm.processSyncSporks()

	case SyncList:
		sm.processSyncList()

	case SyncMNW:
		sm.processSyncMNW()
	}
}

// processSyncSporks handles the SYNC_SPORKS state
// Legacy: lines 287-296 of masternode-sync.cpp
func (sm *SyncManager) processSyncSporks() {
	// Request sporks from peers
	if err := sm.peerRequester.RequestSporks(); err != nil {
		sm.logger.WithError(err).Debug("Failed to request sporks")
	}

	// After 2 attempts, advance to next stage
	// Legacy: if (RequestedMasternodeAttempt >= 2) GetNextAsset()
	if sm.requestAttempt >= 2 {
		sm.getNextAssetLocked()
		return
	}

	sm.requestAttempt++
}

// processSyncList handles the SYNC_LIST state
// Legacy: lines 299-329 of masternode-sync.cpp
func (sm *SyncManager) processSyncList() {
	// Always attempt dseg to peers. Per-peer fulfilled request tracking (with persisted
	// timestamps) handles skipping peers we already asked within the 3-hour cooldown.
	// If all connected peers were already asked and we have cache data, advance immediately.
	// NOTE: Uses local fields (set by NotifyCacheLoaded) to avoid lock ordering deadlock
	// with Manager.mu — SyncManager.mu → Manager.mu path exists via getMasternodeCount.
	now := time.Now().Unix()
	timeout := int64(SyncTimeout.Seconds() * 2)

	// Check if we should advance - haven't received anything in a while and tried enough
	if sm.lastMasternodeList > 0 &&
		sm.lastMasternodeList < now-timeout &&
		sm.requestAttempt >= SyncThreshold {
		sm.getNextAssetLocked()
		return
	}

	// Check for timeout - never received anything
	longTimeout := int64(SyncTimeout.Seconds() * 5)
	if sm.lastMasternodeList == 0 &&
		(sm.requestAttempt >= SyncThreshold*3 || now-sm.assetSyncStarted > longTimeout) {
		if sm.sporkManager != nil && sm.sporkManager.IsActive(spork.SporkMasternodePaymentEnforcement) {
			sm.logger.Warn("Masternode list sync failed (SPORK_8 active), will retry later")
			sm.currentState = SyncFailed
			sm.requestAttempt = 0
			sm.lastFailure = time.Now().Unix()
			sm.failureCount++
		} else {
			sm.logger.Warn("Masternode list sync timed out, advancing to next stage")
			sm.getNextAssetLocked()
		}
		return
	}

	// Don't spam requests if we've tried too many times
	if sm.requestAttempt >= SyncThreshold*3 {
		return
	}

	// Request masternode list from peers (dseg)
	// IMPORTANT: Release lock before calling peerRequester to avoid deadlock
	// The P2P server may call back into SyncManager (HasFulfilledRequest, FulfilledRequest)
	pr := sm.peerRequester
	sm.mu.Unlock()
	sent, skipped, err := pr.RequestMasternodeList()
	sm.mu.Lock()

	if err != nil {
		sm.logger.WithError(err).Debug("Failed to request masternode list")
	}

	// If all peers were skipped (already asked within cooldown) and we have fresh
	// cache data, advance immediately — no point waiting for responses that won't come.
	if sent == 0 && skipped > 0 && sm.cacheLoadedCount > 0 {
		sm.logger.WithFields(logrus.Fields{
			"skipped_peers": skipped,
			"cached_count":  sm.cacheLoadedCount,
		}).Debug("All peers in fulfilled cooldown, advancing with cached masternode list")
		sm.getNextAssetLocked()
		return
	}

	sm.requestAttempt++
}

// processSyncMNW handles the SYNC_MNW state
// Legacy: lines 331-365 of masternode-sync.cpp
func (sm *SyncManager) processSyncMNW() {
	now := time.Now().Unix()
	timeout := int64(SyncTimeout.Seconds() * 2)

	// Check if we should advance - haven't received anything in a while and tried enough
	if sm.lastMasternodeWinner > 0 &&
		sm.lastMasternodeWinner < now-timeout &&
		sm.requestAttempt >= SyncThreshold {
		sm.getNextAssetLocked()
		return
	}

	// Check for timeout - never received anything
	longTimeout := int64(SyncTimeout.Seconds() * 5)
	if sm.lastMasternodeWinner == 0 &&
		(sm.requestAttempt >= SyncThreshold*3 || now-sm.assetSyncStarted > longTimeout) {
		// LEGACY COMPATIBILITY: Check SPORK_8 for masternode payment enforcement
		// Legacy: masternode-sync.cpp:341-352
		// When SPORK_8 is active, set FAILED state instead of advancing without data
		// This prevents nodes from operating without proper winner data
		if sm.sporkManager != nil && sm.sporkManager.IsActive(spork.SporkMasternodePaymentEnforcement) {
			sm.logger.Warn("Masternode winner sync failed (SPORK_8 active), will retry later")
			sm.currentState = SyncFailed
			sm.requestAttempt = 0
			sm.lastFailure = time.Now().Unix()
			sm.failureCount++
		} else {
			sm.logger.Warn("Masternode winner sync timed out, advancing to next stage")
			sm.getNextAssetLocked()
		}
		return
	}

	// Don't spam requests if we've tried too many times
	if sm.requestAttempt >= SyncThreshold*3 {
		return
	}

	// Get masternode count for mnget request
	mnCount := 0
	if sm.getMasternodeCount != nil {
		mnCount = sm.getMasternodeCount()
	}

	// Request masternode winners from peers (mnget)
	// IMPORTANT: Release lock before calling peerRequester to avoid deadlock
	// The P2P server may call back into SyncManager (HasFulfilledRequest, FulfilledRequest)
	pr := sm.peerRequester
	sm.mu.Unlock()
	_, _, err := pr.RequestMasternodeWinners(mnCount)
	sm.mu.Lock()

	if err != nil {
		sm.logger.WithError(err).Debug("Failed to request masternode winners")
	}

	// NOTE: Unlike dseg (processSyncList), we do NOT advance immediately when all peers
	// are skipped. For dseg, C++ peers persist a 3h rate limit (mAskedUsForMasternodeList)
	// so skipped peers genuinely won't respond. For mnget, peers are only skipped because
	// WE marked them as fulfilled on a previous attempt in this same sync cycle — they ARE
	// responding, we just need to wait for the mnw messages to arrive.
	// The normal timeout logic (SyncThreshold attempts / longTimeout) handles advancement.

	sm.requestAttempt++
}
