package rpc

import (
	"fmt"
	"time"

	"github.com/twins-dev/twins-core/internal/masternode"
	"github.com/twins-dev/twins-core/pkg/types"
)

// MasternodeAdapter adapts masternode.Manager to rpc.MasternodeInterface
type MasternodeAdapter struct {
	manager        *masternode.Manager
	paymentTracker *masternode.PaymentTracker
}

// NewMasternodeAdapter creates a new masternode adapter
func NewMasternodeAdapter(manager *masternode.Manager) *MasternodeAdapter {
	return &MasternodeAdapter{manager: manager}
}

// SetPaymentTracker sets the payment tracker for LastPaid lookups.
func (a *MasternodeAdapter) SetPaymentTracker(tracker *masternode.PaymentTracker) {
	a.paymentTracker = tracker
}

// GetMasternodeCount returns enabled, total, and stable masternode counts
// CRITICAL FIX: Use CountEnabled which properly refreshes masternode status
// before counting. The old code read stored status without refreshing, causing
// masternodes to appear as PreEnabled even when they should be Enabled.
// Legacy C++ calls mn.Check() inside CountEnabled loop (masternodeman.cpp:384)
func (a *MasternodeAdapter) GetMasternodeCount() (int, int, int) {
	total := a.manager.GetMasternodeCount()

	// Use CountEnabled(-1) which:
	// 1. Calls UpdateStatus for each masternode to refresh status
	// 2. Uses minimum protocol version from spork
	// 3. Matches legacy CMasternodeMan::CountEnabled() behavior
	enabled := a.manager.CountEnabled(-1)

	// Stable count uses GetStableCount() which excludes newly activated nodes
	// (nodes younger than MN_WINNER_MINIMUM_AGE = 8000 seconds)
	stable := a.manager.GetStableCount()

	return enabled, total, stable
}

// GetMasternodeList returns list of masternodes
// CRITICAL FIX: Refresh status before filtering to match legacy behavior
// Legacy C++ calls mn.Check() before reading status (masternodeman.cpp)
func (a *MasternodeAdapter) GetMasternodeList(filter string) []MasternodeInfo {
	masternodes := a.manager.GetMasternodes()
	list := make([]MasternodeInfo, 0, len(masternodes))
	currentTime := time.Now()
	expireTime := time.Duration(masternode.ExpirationSeconds) * time.Second

	for _, mn := range masternodes {
		// CRITICAL FIX: Refresh status before reading
		// Legacy: mn.Check() is called before status check
		mn.UpdateStatus(currentTime, expireTime)

		// Use payment tracker for LastPaid if available, fall back to mn.LastPaid
		lastPaidTime := max(mn.LastPaid.Unix(), 0)
		if a.paymentTracker != nil {
			if stats := a.paymentTracker.GetStatsByScript(mn.GetPayeeScript()); stats != nil {
				lastPaidTime = stats.LastPaid.Unix()
			}
		}

		info := MasternodeInfo{
			TxHash:          mn.OutPoint.Hash.String(),
			OutputIndex:     int(mn.OutPoint.Index),
			Status:          mn.Status.String(),
			Protocol:        int(mn.Protocol),
			Payee:           mn.Addr.String(),
			LastSeen:        mn.LastSeen.Unix(),
			ActiveSeconds:   activeSeconds(mn.ActiveSince), // Live-incrementing duration since activation
			LastPaidTime:    lastPaidTime,
			LastPaidBlock:   int64(mn.BlockHeight),
			IP:              mn.Addr.String(),
			Tier:            mn.Tier.String(),
			Rank:            mn.Rank,
			Addr:            mn.Addr.String(),
			Version:         int(mn.Protocol),
			CollateralAmount: float64(mn.Collateral) / 1e8,
		}

		// Apply filter
		if filter == "" || filter == "all" {
			list = append(list, info)
		} else if filter == "enabled" && mn.Status == masternode.StatusEnabled {
			list = append(list, info)
		} else if filter == "qualify" && mn.Status == masternode.StatusEnabled {
			list = append(list, info)
		}
	}

	return list
}

// GetMasternodeStatus returns status of a specific masternode
func (a *MasternodeAdapter) GetMasternodeStatus(outpoint types.Outpoint) (*MasternodeStatus, error) {
	mn, err := a.manager.GetMasternode(outpoint)
	if err != nil {
		return nil, err
	}

	// Use payment tracker for LastPaid if available, fall back to mn.LastPaid
	lastPaidTime := max(mn.LastPaid.Unix(), 0)
	if a.paymentTracker != nil {
		if stats := a.paymentTracker.GetStatsByScript(mn.GetPayeeScript()); stats != nil {
			lastPaidTime = stats.LastPaid.Unix()
		}
	}

	status := &MasternodeStatus{
		Outpoint:        outpoint,
		Service:         mn.Addr.String(),
		Payee:           mn.Addr.String(),
		Status:          mn.Status.String(),
		ProtocolVersion: int(mn.Protocol),
		LastSeen:        mn.LastSeen.Unix(),
		ActiveSeconds:   activeSeconds(mn.ActiveSince), // Live-incrementing duration since activation
		LastPaidTime:    lastPaidTime,
		LastPaidBlock:   int64(mn.BlockHeight),
		Tier:            int(mn.Tier),
	}

	return status, nil
}

// activeSeconds computes live-incrementing active time as duration since activation.
// Guards against zero-value times and negative results from clock skew.
func activeSeconds(activeSince time.Time) int64 {
	if activeSince.IsZero() {
		return 0
	}
	v := time.Now().Unix() - activeSince.Unix()
	if v < 0 {
		return 0
	}
	return v
}

// GetMasternodeWinners returns masternode payment winners
func (a *MasternodeAdapter) GetMasternodeWinners(blocks int, filter string) []MasternodeWinner {
	// Get payment queue to determine winners
	winners := []MasternodeWinner{}
	masternodes := a.manager.GetPaymentQueue()

	// Limit to requested block count
	if blocks > len(masternodes) {
		blocks = len(masternodes)
	}

	for i := 0; i < blocks && i < len(masternodes); i++ {
		mn := masternodes[i]
		winner := MasternodeWinner{
			Height:  i, // Sequential position in queue
			Payee:   mn.Addr.String(),
			Votes:   1, // Single vote for now
		}
		winners = append(winners, winner)
	}

	return winners
}

// GetMasternodeScores returns masternode scores
func (a *MasternodeAdapter) GetMasternodeScores(blocks int) []MasternodeScore {
	scores := []MasternodeScore{}
	masternodes := a.manager.GetMasternodes()

	for outpoint, mn := range masternodes {
		score := MasternodeScore{
			Outpoint: outpoint,
			// Use ScoreCompact (float64) for RPC/JSON - mn.Score is now types.Hash (32 bytes)
			// ScoreCompact contains the first 8 bytes of the hash as float64 for display/API
			Score: int64(mn.ScoreCompact),
		}
		scores = append(scores, score)
	}

	return scores
}

// GetMasternodes returns all masternodes
func (a *MasternodeAdapter) GetMasternodes() map[types.Outpoint]*masternode.Masternode {
	return a.manager.GetMasternodes()
}

// GetMasternodeInfo returns information about a specific masternode
func (a *MasternodeAdapter) GetMasternodeInfo(outpoint types.Outpoint) (*masternode.MasternodeInfo, error) {
	return a.manager.GetMasternodeInfo(outpoint)
}

// GetMasternodeCountByTier returns count of masternodes by tier
func (a *MasternodeAdapter) GetMasternodeCountByTier(tier masternode.MasternodeTier) int {
	return a.manager.GetMasternodeCountByTier(tier)
}

// IsMasternodeActive checks if a masternode is active
func (a *MasternodeAdapter) IsMasternodeActive(outpoint types.Outpoint) bool {
	return a.manager.IsMasternodeActive(outpoint)
}

// GetNextPayee returns the next masternode to be paid
func (a *MasternodeAdapter) GetNextPayee() (*masternode.Masternode, error) {
	return a.manager.GetNextPayee()
}

// GetNextPaymentWinner returns the masternode winner for a specific block
// Delegates to Manager.GetNextMasternodeInQueueForPayment which uses the proper
// legacy algorithm with UpdateStatusWithUTXO (equivalent to mn.Check()).
func (a *MasternodeAdapter) GetNextPaymentWinner(blockHeight uint32, blockHash types.Hash) (*masternode.Masternode, error) {
	// Use the proper legacy algorithm from Manager
	// This correctly calls UpdateStatusWithUTXO on each masternode before eligibility check
	mn, count := a.manager.GetNextMasternodeInQueueForPayment(blockHeight, true)
	if mn == nil {
		return nil, fmt.Errorf("no eligible masternodes for payment (checked %d)", count)
	}
	return mn, nil
}

// ProcessBroadcast processes a masternode broadcast with optional origin peer exclusion.
func (a *MasternodeAdapter) ProcessBroadcast(broadcast *masternode.MasternodeBroadcast, originAddr string) error {
	return a.manager.ProcessBroadcast(broadcast, originAddr)
}

// StartMasternode starts a masternode
func (a *MasternodeAdapter) StartMasternode(alias string, lockWallet bool) (string, error) {
	// Starting a masternode requires:
	// 1. Reading masternode.conf for the alias
	// 2. Creating and signing a MasternodeBroadcast
	// 3. Broadcasting to the network
	// This is typically done through RPC handler, not here
	return "", fmt.Errorf("masternode start not implemented - use createmasternodebroadcast and relaymasternodebroadcast")
}

// StartMasternodeMany starts multiple masternodes
func (a *MasternodeAdapter) StartMasternodeMany() (int, int) {
	// Would iterate through masternode.conf and start all configured masternodes
	// Returns (successful, failed) counts
	return 0, 0
}

// CreateMasternodeBroadcast creates a masternode broadcast message
func (a *MasternodeAdapter) CreateMasternodeBroadcast(alias string) (string, error) {
	// Creating a broadcast requires:
	// 1. Loading masternode configuration
	// 2. Creating MasternodeBroadcast struct
	// 3. Signing with masternode key
	// 4. Serializing to hex
	// This is handled in the RPC handler layer
	return "", fmt.Errorf("broadcast creation not implemented - use RPC handler")
}

// RelayMasternodeBroadcast relays a masternode broadcast
func (a *MasternodeAdapter) RelayMasternodeBroadcast(hex string) error {
	// Relaying requires:
	// 1. Deserializing broadcast from hex
	// 2. Processing through manager
	// 3. Broadcasting to P2P network
	// This is handled in the RPC handler layer
	return fmt.Errorf("broadcast relay not implemented - use RPC handler")
}

// ResetSync resets the masternode sync state
func (a *MasternodeAdapter) ResetSync() {
	a.manager.ResetSync()
}

// ActiveMasternodeAdapter adapts masternode.ActiveMasternode to rpc.ActiveMasternodeInterface
type ActiveMasternodeAdapter struct {
	active *masternode.ActiveMasternode
}

// NewActiveMasternodeAdapter creates a new active masternode adapter
func NewActiveMasternodeAdapter(active *masternode.ActiveMasternode) *ActiveMasternodeAdapter {
	return &ActiveMasternodeAdapter{active: active}
}

// GetStatus returns the current status string
func (a *ActiveMasternodeAdapter) GetStatus() string {
	return a.active.GetStatus()
}

// IsStarted returns true if the masternode is successfully started
func (a *ActiveMasternodeAdapter) IsStarted() bool {
	return a.active.IsStarted()
}

// Initialize sets up the active masternode with the given private key and service address
func (a *ActiveMasternodeAdapter) Initialize(privKeyWIF string, serviceAddr string) error {
	return a.active.Initialize(privKeyWIF, serviceAddr)
}

// Start attempts to start the masternode with the given collateral
func (a *ActiveMasternodeAdapter) Start(collateralTx types.Hash, collateralIdx uint32, collateralKey interface{}) error {
	// Type assert collateralKey to *crypto.PrivateKey
	// The interface{} is used to avoid circular imports
	if collateralKey == nil {
		return fmt.Errorf("collateral key is required")
	}
	// Note: The actual type assertion happens in the RPC handler which has access to crypto package
	return fmt.Errorf("Start must be called through RPC handler with proper key type")
}

// Stop stops the active masternode
func (a *ActiveMasternodeAdapter) Stop() {
	a.active.Stop()
}

// ManageStatus manages the masternode status (called periodically)
func (a *ActiveMasternodeAdapter) ManageStatus() error {
	return a.active.ManageStatus()
}

// SetSyncChecker sets the function to check if blockchain is synced
func (a *ActiveMasternodeAdapter) SetSyncChecker(fn func() bool) {
	a.active.SetSyncChecker(fn)
}

// SetBalanceGetter sets the function to get wallet balance
func (a *ActiveMasternodeAdapter) SetBalanceGetter(fn func() int64) {
	a.active.SetBalanceGetter(fn)
}

// GetActiveMasternode returns the underlying ActiveMasternode for direct access
func (a *ActiveMasternodeAdapter) GetActiveMasternode() *masternode.ActiveMasternode {
	return a.active
}

// MasternodeConfAdapter adapts masternode.MasternodeConfFile to rpc.MasternodeConfInterface
type MasternodeConfAdapter struct {
	confFile *masternode.MasternodeConfFile
}

// NewMasternodeConfAdapter creates a new masternode.conf adapter
func NewMasternodeConfAdapter(confFile *masternode.MasternodeConfFile) *MasternodeConfAdapter {
	return &MasternodeConfAdapter{confFile: confFile}
}

// Read reads and parses the masternode.conf file
func (a *MasternodeConfAdapter) Read() error {
	return a.confFile.Read()
}

// GetEntries returns all masternode entries converted to RPC type
func (a *MasternodeConfAdapter) GetEntries() []*MasternodeConfEntry {
	entries := a.confFile.GetEntries()
	result := make([]*MasternodeConfEntry, len(entries))
	for i, e := range entries {
		result[i] = &MasternodeConfEntry{
			Alias:           e.Alias,
			IP:              e.IP,
			PrivKey:         e.PrivKey,
			TxHash:          e.TxHash,
			OutputIndex:     e.OutputIndex,
			DonationAddress: e.DonationAddress,
			DonationPercent: e.DonationPercent,
		}
	}
	return result
}

// GetEntry returns a masternode entry by alias
func (a *MasternodeConfAdapter) GetEntry(alias string) *MasternodeConfEntry {
	e := a.confFile.GetEntry(alias)
	if e == nil {
		return nil
	}
	return &MasternodeConfEntry{
		Alias:           e.Alias,
		IP:              e.IP,
		PrivKey:         e.PrivKey,
		TxHash:          e.TxHash,
		OutputIndex:     e.OutputIndex,
		DonationAddress: e.DonationAddress,
		DonationPercent: e.DonationPercent,
	}
}

// GetCount returns the number of entries
func (a *MasternodeConfAdapter) GetCount() int {
	return a.confFile.GetCount()
}

// GetConfFile returns the underlying MasternodeConfFile for direct access
func (a *MasternodeConfAdapter) GetConfFile() *masternode.MasternodeConfFile {
	return a.confFile
}

