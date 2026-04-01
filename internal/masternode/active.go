// Copyright (c) 2018-2025 The TWINS developers
// Distributed under the MIT/X11 software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package masternode

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/twins-dev/twins-core/internal/consensus"
	"github.com/twins-dev/twins-core/internal/masternode/debug"
	"github.com/twins-dev/twins-core/pkg/crypto"
	"github.com/twins-dev/twins-core/pkg/types"
)

// ActiveStatus represents the status of the active masternode
type ActiveStatus int

const (
	ActiveInitial       ActiveStatus = iota // Initial state
	ActiveSyncInProcess                     // Waiting for blockchain sync
	ActiveInputTooNew                       // Collateral has insufficient confirmations
	ActiveNotCapable                        // Not capable of running as MN
	ActiveStarted                           // Successfully started
)

func (s ActiveStatus) String() string {
	switch s {
	case ActiveInitial:
		return "Node just started, not yet activated"
	case ActiveSyncInProcess:
		return "Sync in progress. Must wait until sync is complete to start Masternode"
	case ActiveInputTooNew:
		return fmt.Sprintf("Masternode input must have at least %d confirmations", MinConfirmations)
	case ActiveNotCapable:
		return "Not capable masternode"
	case ActiveStarted:
		return "Masternode successfully started"
	default:
		return "unknown"
	}
}

// MinConfirmations is the minimum confirmations required for collateral
const MinConfirmations = 15

// PingInterval is the interval between masternode pings
// Legacy: MASTERNODE_PING_SECONDS (5 * 60) = 5 minutes
// See legacy/src/masternode.h:21 and obfuscation.cpp:2305
// ManageStatus() is called every MASTERNODE_PING_SECONDS seconds
const PingInterval = 5 * time.Minute

// MaxVotedHeightsHistory is the maximum number of voted block heights to keep
// in memory. Older entries are cleaned up to prevent memory leaks.
const MaxVotedHeightsHistory = 100

// ActiveMasternode manages the local masternode state
type ActiveMasternode struct {
	// Identity
	PubKeyMasternode *crypto.PublicKey // Masternode operator key
	Vin              types.Outpoint    // Collateral outpoint
	ServiceAddr      net.Addr          // External address

	// State
	Status           ActiveStatus
	NotCapableReason string
	LastPing         time.Time

	// Keys
	privateKey *crypto.PrivateKey // Masternode private key for signing

	// Dependencies (set via SetDependencies)
	manager    *Manager
	confFile   *MasternodeConfFile
	isMainnet  bool
	isSynced   func() bool
	getBalance func() int64
	blockchain BlockchainReader // For getting recent block hashes

	// Wallet interface for auto-collateral and locking (set via SetWallet)
	wallet WalletInterface

	// Address discovery for auto-detecting external IP (set via SetAddressDiscovery)
	// LEGACY COMPATIBILITY: Matches C++ GetLocal() from activemasternode.cpp:61-69
	addressDiscovery AddressDiscovery

	// Vote tracking - prevents double-voting for winner votes at the same height
	votedHeights map[uint32]bool

	// Debug event collector (nil when disabled)
	debugCollector atomic.Pointer[debug.Collector]

	// Auto-management loop control
	// LEGACY COMPATIBILITY: C++ calls ManageStatus() every MASTERNODE_PING_SECONDS (300s)
	// from obfuscation.cpp:2305: if (c % MASTERNODE_PING_SECONDS == 1) activeMasternode.ManageStatus();
	stopCh    chan struct{}
	runningWg sync.WaitGroup
	running   bool

	mu sync.RWMutex
}

// WalletInterface defines wallet operations needed for masternode management
// This interface allows the masternode package to interact with the wallet
// without directly depending on the wallet package
type WalletInterface interface {
	// IsLocked returns true if the wallet is encrypted and locked
	IsLocked() bool

	// GetUnspentOutputs returns all unspent transaction outputs in the wallet
	GetUnspentOutputs() ([]CollateralUTXO, error)

	// LockCoin locks a specific UTXO to prevent it from being spent
	LockCoin(outpoint types.Outpoint) error

	// UnlockCoin unlocks a previously locked UTXO
	UnlockCoin(outpoint types.Outpoint) error

	// GetPrivateKey returns the private key for a given public key hash
	// Used to sign masternode broadcasts with the collateral key
	GetPrivateKey(pubKeyHash []byte) (*crypto.PrivateKey, error)
}

// AddressDiscovery interface for external IP address discovery
// LEGACY COMPATIBILITY: Matches C++ GetLocal() functionality
// C++ Reference: activemasternode.cpp:61-69
type AddressDiscovery interface {
	// GetExternalIP returns the discovered external IP address
	// Returns nil if no external IP has been discovered
	GetExternalIP() net.IP

	// GetMappedPort returns the external port (from UPnP mapping)
	// Returns 0 if no port mapping exists
	GetMappedPort() int
}

// CollateralUTXO represents a UTXO that could be used as masternode collateral
type CollateralUTXO struct {
	Outpoint      types.Outpoint
	Amount        int64
	Confirmations int32
	PubKeyHash    []byte // Public key hash for retrieving private key
}

// NewActiveMasternode creates a new active masternode instance
func NewActiveMasternode() *ActiveMasternode {
	return &ActiveMasternode{
		Status:       ActiveInitial,
		votedHeights: make(map[uint32]bool),
		stopCh:       make(chan struct{}),
	}
}

// StartAutoManagement starts the periodic ManageStatus loop
// LEGACY COMPATIBILITY: This replicates C++ behavior from obfuscation.cpp:2305
// where ManageStatus() is called every MASTERNODE_PING_SECONDS (300s = 5 min)
func (am *ActiveMasternode) StartAutoManagement() {
	am.mu.Lock()
	if am.running {
		am.mu.Unlock()
		return
	}
	am.running = true
	// Reinitialize stopCh if it was closed
	if am.stopCh == nil {
		am.stopCh = make(chan struct{})
	}
	am.mu.Unlock()

	am.runningWg.Add(1)
	go am.autoManagementLoop()
}

// StopAutoManagement stops the periodic ManageStatus loop
func (am *ActiveMasternode) StopAutoManagement() {
	am.mu.Lock()
	if !am.running {
		am.mu.Unlock()
		return
	}
	am.running = false
	close(am.stopCh)
	am.mu.Unlock()

	am.runningWg.Wait()

	// Prepare for potential restart
	am.mu.Lock()
	am.stopCh = make(chan struct{})
	am.mu.Unlock()
}

// IsAutoManagementRunning returns true if the auto-management loop is running
func (am *ActiveMasternode) IsAutoManagementRunning() bool {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.running
}

// autoManagementLoop is the internal goroutine that periodically calls ManageStatus
func (am *ActiveMasternode) autoManagementLoop() {
	defer am.runningWg.Done()

	// Use PingInterval (5 minutes) for the loop - matches legacy MASTERNODE_PING_SECONDS
	ticker := time.NewTicker(PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Call ManageStatus to handle state transitions and send pings
			if err := am.ManageStatus(); err != nil {
				logrus.WithError(err).Warn("Active masternode ManageStatus error")
			}
		case <-am.stopCh:
			return
		}
	}
}

// SetDependencies sets the dependencies for the active masternode
func (am *ActiveMasternode) SetDependencies(manager *Manager, confFile *MasternodeConfFile, isMainnet bool) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.manager = manager
	am.confFile = confFile
	am.isMainnet = isMainnet
}

// SetSyncChecker sets the function to check if blockchain is synced
func (am *ActiveMasternode) SetSyncChecker(fn func() bool) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.isSynced = fn
}

// SetBalanceGetter sets the function to get wallet balance
func (am *ActiveMasternode) SetBalanceGetter(fn func() int64) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.getBalance = fn
}

// SetBlockchain sets the blockchain reader for getting recent block hashes
func (am *ActiveMasternode) SetBlockchain(bc BlockchainReader) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.blockchain = bc
}

// SetWallet sets the wallet interface for auto-collateral and locking
func (am *ActiveMasternode) SetWallet(w WalletInterface) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.wallet = w
}

// SetAddressDiscovery sets the address discovery interface for external IP auto-detection
// LEGACY COMPATIBILITY: Matches C++ GetLocal() from activemasternode.cpp:61-69
func (am *ActiveMasternode) SetAddressDiscovery(ad AddressDiscovery) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.addressDiscovery = ad
}

// SetVin sets the collateral outpoint for the active masternode
func (am *ActiveMasternode) SetVin(outpoint types.Outpoint) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.Vin = outpoint
}

// getRecentBlockHash returns a recent block hash for ping messages
// Legacy C++ uses tip - 12 (PingBlockDepth) to ensure the block is confirmed
// Returns error if blockchain not available - caller should NOT proceed with ZeroHash
// ZeroHash broadcasts/pings will be rejected by other nodes
func (am *ActiveMasternode) getRecentBlockHash() (types.Hash, error) {
	if am.blockchain == nil {
		return types.ZeroHash, fmt.Errorf("blockchain not available")
	}

	height, err := am.blockchain.GetBestHeight()
	if err != nil {
		return types.ZeroHash, fmt.Errorf("failed to get best height: %w", err)
	}

	if height < PingBlockDepth {
		return types.ZeroHash, fmt.Errorf("chain too short: height %d < %d", height, PingBlockDepth)
	}

	targetHeight := height - PingBlockDepth
	block, err := am.blockchain.GetBlockByHeight(targetHeight)
	if err != nil {
		return types.ZeroHash, fmt.Errorf("failed to get block at height %d: %w", targetHeight, err)
	}
	if block == nil {
		return types.ZeroHash, fmt.Errorf("block at height %d is nil", targetHeight)
	}

	return block.Hash(), nil
}

// Initialize sets up the active masternode with the given private key and service address
func (am *ActiveMasternode) Initialize(privKeyWIF string, serviceAddr string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	// Parse private key from WIF
	privKey, err := crypto.DecodeWIF(privKeyWIF)
	if err != nil {
		return fmt.Errorf("invalid masternode private key: %w", err)
	}
	am.privateKey = privKey
	am.PubKeyMasternode = privKey.PublicKey()

	// Parse service address
	addr, err := net.ResolveTCPAddr("tcp", serviceAddr)
	if err != nil {
		return fmt.Errorf("invalid service address: %w", err)
	}
	am.ServiceAddr = addr

	// Validate port
	port := addr.Port
	if err := ValidatePort(port, am.isMainnet); err != nil {
		return err
	}

	return nil
}

// GetStatus returns the current status string
func (am *ActiveMasternode) GetStatus() string {
	am.mu.RLock()
	defer am.mu.RUnlock()

	if am.Status == ActiveNotCapable && am.NotCapableReason != "" {
		return "Not capable masternode: " + am.NotCapableReason
	}
	return am.Status.String()
}

// IsStarted returns true if the masternode is successfully started
func (am *ActiveMasternode) IsStarted() bool {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.Status == ActiveStarted
}

// GetVin returns the collateral outpoint
func (am *ActiveMasternode) GetVin() types.Outpoint {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.Vin
}

// GetServiceAddr returns the service address
func (am *ActiveMasternode) GetServiceAddr() net.Addr {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.ServiceAddr
}

// GetPubKeyMasternode returns the masternode public key
func (am *ActiveMasternode) GetPubKeyMasternode() *crypto.PublicKey {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.PubKeyMasternode
}

// HasVotedForHeight checks if we already voted for a specific block height
// Used to prevent double-voting for winner votes
func (am *ActiveMasternode) HasVotedForHeight(height uint32) bool {
	if am == nil {
		return false
	}

	am.mu.RLock()
	defer am.mu.RUnlock()

	if am.votedHeights == nil {
		return false
	}
	return am.votedHeights[height]
}

// MarkVotedForHeight marks that we've voted for a specific block height
// Called after successfully broadcasting a winner vote
func (am *ActiveMasternode) MarkVotedForHeight(height uint32) {
	if am == nil {
		return
	}

	am.mu.Lock()
	defer am.mu.Unlock()

	if am.votedHeights == nil {
		am.votedHeights = make(map[uint32]bool)
	}
	am.votedHeights[height] = true

	// Cleanup old entries to prevent memory leak
	if len(am.votedHeights) > MaxVotedHeightsHistory {
		minHeight := height - MaxVotedHeightsHistory
		for h := range am.votedHeights {
			if h < minHeight {
				delete(am.votedHeights, h)
			}
		}
	}
}

// GetPrivateKey returns the masternode private key for signing
// Used for signing winner votes
func (am *ActiveMasternode) GetPrivateKey() *crypto.PrivateKey {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.privateKey
}

// ManageStatus manages the masternode status (called periodically)
// This is the main loop that handles state transitions
// Legacy: CActiveMasternode::ManageStatus() in activemasternode.cpp:18-136
func (am *ActiveMasternode) ManageStatus() error {
	am.mu.Lock()
	defer am.mu.Unlock()

	prevStatus := am.Status

	// Check if sync is complete
	// Legacy: masternodeSync.IsBlockchainSynced() check
	if am.isSynced != nil && !am.isSynced() {
		if prevStatus != ActiveSyncInProcess {
			logrus.WithField("prev_status", prevStatus.String()).
				Warn("Active masternode pausing pings - blockchain not synced")
		}
		am.Status = ActiveSyncInProcess
		am.emitStateChange(prevStatus)
		return nil
	}

	// Reset from sync state
	if am.Status == ActiveSyncInProcess {
		logrus.Info("Active masternode resuming - blockchain synced")
		am.Status = ActiveInitial
	}

	// If already started, just send pings
	// Legacy: SendMasternodePing(errorMessage) at line 133
	if am.Status == ActiveStarted {
		err := am.sendPingLocked()
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"outpoint": am.Vin.String(),
				"error":    err.Error(),
			}).Warn("Active masternode ping failed")
		}
		if dc := am.debugCollector.Load(); dc != nil && dc.IsEnabled() {
			summary := "Active MN ping sent"
			if err != nil {
				summary = fmt.Sprintf("Active MN ping failed: %v", err)
			}
			dc.EmitActive("active_ping_sent", "local", summary, map[string]any{
				"success": err == nil,
			})
		}
		return err
	}

	// Try to auto-start from the masternode network list
	if am.tryAutoStartFromNetworkLocked() {
		logrus.WithField("outpoint", am.Vin.String()).
			Info("Active masternode auto-started from network list")
		am.emitStateChange(prevStatus)
		return nil
	}

	// Set default not capable state
	am.Status = ActiveNotCapable
	am.NotCapableReason = ""

	// Validate all prerequisites (wallet, balance, address, key, port)
	if reason := am.validatePrerequisitesLocked(); reason != "" {
		if am.NotCapableReason != reason {
			logrus.WithField("reason", reason).Warn("Active masternode not capable")
		}
		am.NotCapableReason = reason
		am.emitStateChange(prevStatus)
		return nil
	}

	// Attempt collateral activation
	err := am.activateWithCollateralLocked()
	am.emitStateChange(prevStatus)
	return err
}

// emitStateChange emits a debug event when the active masternode status changes.
// Caller must hold am.mu.
func (am *ActiveMasternode) emitStateChange(prevStatus ActiveStatus) {
	if am.Status == prevStatus {
		return
	}
	if dc := am.debugCollector.Load(); dc != nil && dc.IsEnabled() {
		dc.EmitActive("active_state_change", "local", fmt.Sprintf("Active MN state: %s → %s", prevStatus.String(), am.Status.String()), map[string]any{
			"prev_status": prevStatus.String(),
			"new_status":  am.Status.String(),
			"reason":      am.NotCapableReason,
		})
	}
}

// tryAutoStartFromNetworkLocked checks if this node exists in the masternode list
// and auto-starts if found with matching protocol version.
// Returns true if auto-started successfully.
// Legacy: mnodeman.Find(pubKeyMasternode) at line 37-41
func (am *ActiveMasternode) tryAutoStartFromNetworkLocked() bool {
	if am.manager == nil || am.PubKeyMasternode == nil {
		return false
	}

	mn := am.manager.FindByPubKey(am.PubKeyMasternode)
	if mn == nil || !mn.IsActive() {
		return false
	}

	// CRITICAL: Verify protocol version matches before autostart
	// Legacy: if (pmn->IsEnabled() && pmn->protocolVersion == PROTOCOL_VERSION) EnableHotColdMasterNode
	mn.mu.RLock()
	mnProtocol := mn.Protocol
	mn.mu.RUnlock()

	if mnProtocol != ActiveProtocolVersion {
		logrus.WithFields(logrus.Fields{
			"mn_protocol":       mnProtocol,
			"required_protocol": ActiveProtocolVersion,
		}).Warn("Found masternode in list but protocol version mismatch, skipping autostart")
		return false
	}

	am.Vin = mn.OutPoint
	am.Status = ActiveStarted
	return true
}

// validatePrerequisitesLocked checks all prerequisites for masternode operation.
// Returns an empty string if all checks pass, or the reason string if not capable.
// Caller must hold am.mu.
func (am *ActiveMasternode) validatePrerequisitesLocked() string {
	// Check wallet locked status
	// Legacy: pwalletMain->IsLocked() at line 49-53
	if am.wallet != nil && am.wallet.IsLocked() {
		return "Wallet is locked"
	}

	// Check wallet balance (for hot wallet detection)
	// Legacy: pwalletMain->GetBalance() == 0 at line 55-59
	if am.getBalance != nil && am.getBalance() == 0 {
		return "Hot node, waiting for remote activation"
	}

	// Check if service address is set, auto-detect if not configured
	// LEGACY COMPATIBILITY: Matches C++ GetLocal() from activemasternode.cpp:61-69
	if am.ServiceAddr == nil {
		detectedAddr := am.detectExternalAddressLocked()
		if detectedAddr == nil {
			return "Can't detect external address. Please use masternodeaddr configuration option."
		}
		am.ServiceAddr = detectedAddr
		logrus.WithField("address", detectedAddr.String()).Debug("Auto-detected external masternode address via GetLocal equivalent")
	}

	// Check if private key is set
	if am.privateKey == nil {
		return "Masternode private key not configured"
	}

	// Port validation
	// Legacy: CMasternodeBroadcast::CheckDefaultPort() at line 71-73
	if tcpAddr, ok := am.ServiceAddr.(*net.TCPAddr); ok {
		if err := ValidatePort(tcpAddr.Port, am.isMainnet); err != nil {
			return err.Error()
		}
	}

	// Check inbound connection to our address (port accessibility)
	// Legacy: ConnectNode((CAddress)service, NULL, false) at line 77-83
	if err := am.checkPortAccessibility(); err != nil {
		return fmt.Sprintf("Could not connect to %s: %v", am.ServiceAddr.String(), err)
	}

	return ""
}

// activateWithCollateralLocked attempts to find collateral and activate the masternode.
// Handles wallet-based activation, hot node fallback, and restart scenarios.
// Caller must hold am.mu.
func (am *ActiveMasternode) activateWithCollateralLocked() error {
	// Auto-find collateral UTXO if wallet is available
	// Legacy: GetMasterNodeVin() at line 89
	if am.wallet != nil && am.Vin.Hash == (types.Hash{}) {
		collateral, collateralKey, err := am.findCollateralUTXOLocked()
		if err != nil {
			am.NotCapableReason = "Could not find suitable coins"
			return nil
		}

		// Check input age (confirmations)
		// Legacy: GetInputAge(vin) < MASTERNODE_MIN_CONFIRMATIONS at line 90-95
		if collateral.Confirmations < MinConfirmations {
			am.Status = ActiveInputTooNew
			am.NotCapableReason = fmt.Sprintf("Masternode input must have at least %d confirmations, has %d",
				MinConfirmations, collateral.Confirmations)
			return nil
		}

		// Lock the collateral coin in wallet
		// Legacy: pwalletMain->LockCoin(vin.prevout) at line 98
		if err := am.wallet.LockCoin(collateral.Outpoint); err != nil {
			am.NotCapableReason = fmt.Sprintf("Failed to lock collateral: %v", err)
			return nil
		}

		am.Vin = collateral.Outpoint

		// Create and relay broadcast
		// Legacy: CreateBroadcast() and mnb.Relay() at line 110-119
		broadcast, err := am.createBroadcastLocked(collateralKey)
		if err != nil {
			am.NotCapableReason = fmt.Sprintf("Error creating broadcast: %v", err)
			return nil
		}

		// Process and relay broadcast (no origin peer for local broadcasts)
		if am.manager != nil {
			if err := am.manager.ProcessBroadcast(broadcast, ""); err != nil && !errors.Is(err, ErrBroadcastAlreadySeen) {
				am.NotCapableReason = fmt.Sprintf("Error on broadcast: %v", err)
				return nil
			}
		}

		am.Status = ActiveStarted
		// LEGACY COMPATIBILITY: Use network-adjusted time like legacy C++
		am.LastPing = consensus.GetAdjustedTimeAsTime()
		return nil
	}

	// Hot node case: No wallet available - waiting for remote activation from cold wallet
	if am.wallet == nil {
		am.NotCapableReason = "Hot node, waiting for remote activation"
		return nil
	}

	// Fallback: wallet available but Vin already set (shouldn't happen normally)
	if am.Vin.Hash != (types.Hash{}) {
		am.NotCapableReason = "Collateral configured, waiting for network sync or broadcast"
		return nil
	}

	// Wallet available but no suitable collateral found
	am.NotCapableReason = "Could not find suitable coins"
	return nil
}

// checkPortAccessibility verifies that our masternode port is reachable
// Legacy: ConnectNode((CAddress)service, NULL, false) in activemasternode.cpp:77-83
func (am *ActiveMasternode) checkPortAccessibility() error {
	if am.ServiceAddr == nil {
		return fmt.Errorf("service address not set")
	}

	// Try to connect to our own address with a short timeout
	conn, err := net.DialTimeout("tcp", am.ServiceAddr.String(), 5*time.Second)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}

// detectExternalAddressLocked attempts to auto-detect the external IP address
// LEGACY COMPATIBILITY: Matches C++ GetLocal() from activemasternode.cpp:61-69
// C++ implementation uses net.GetLocal(service) which queries peer-discovered addresses
//
// Detection methods (in priority order):
// 1. UPnP-discovered external IP (via AddressDiscovery interface)
// 2. First non-loopback, non-private IPv4 address from local interfaces
//
// Must hold am.mu lock
func (am *ActiveMasternode) detectExternalAddressLocked() net.Addr {
	// Default port based on network
	defaultPort := MainnetDefaultPort
	if !am.isMainnet {
		defaultPort = TestnetDefaultPort
	}

	// Method 1: Try UPnP/AddressDiscovery if available
	if am.addressDiscovery != nil {
		externalIP := am.addressDiscovery.GetExternalIP()
		if externalIP != nil && !externalIP.IsLoopback() && !externalIP.IsPrivate() {
			port := am.addressDiscovery.GetMappedPort()
			if port == 0 {
				port = defaultPort
			}
			return &net.TCPAddr{IP: externalIP, Port: port}
		}
	}

	// Method 2: Try local network interfaces for public IP
	// This matches legacy behavior when UPnP is unavailable
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	for _, iface := range ifaces {
		// Skip down interfaces and loopback
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// Only accept IPv4, non-loopback, non-private addresses
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}

			// Check if it's a public IP (not private range)
			if !ip.IsPrivate() {
				return &net.TCPAddr{IP: ip, Port: defaultPort}
			}
		}
	}

	return nil
}

// findCollateralUTXOLocked finds a suitable collateral UTXO from the wallet
// Legacy: GetMasterNodeVin() in activemasternode.cpp
// Returns the collateral UTXO and the private key needed to sign broadcasts
// Must hold am.mu lock
func (am *ActiveMasternode) findCollateralUTXOLocked() (*CollateralUTXO, *crypto.PrivateKey, error) {
	if am.wallet == nil {
		return nil, nil, fmt.Errorf("wallet not available")
	}

	utxos, err := am.wallet.GetUnspentOutputs()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get unspent outputs: %w", err)
	}

	// Valid collateral amounts for each tier
	validCollaterals := []int64{
		TierPlatinumCollateral, // 100M TWINS - check highest first
		TierGoldCollateral,     // 20M TWINS
		TierSilverCollateral,   // 5M TWINS
		TierBronzeCollateral,   // 1M TWINS
	}

	// Find exact collateral match (legacy behavior)
	for _, utxo := range utxos {
		for _, collateralAmount := range validCollaterals {
			if utxo.Amount == collateralAmount {
				// Found valid collateral, get private key
				privKey, err := am.wallet.GetPrivateKey(utxo.PubKeyHash)
				if err != nil {
					continue // Try next UTXO if we can't get the key
				}
				return &utxo, privKey, nil
			}
		}
	}

	return nil, nil, fmt.Errorf("no suitable collateral found")
}

// Start attempts to start the masternode with the given collateral
func (am *ActiveMasternode) Start(collateralTx types.Hash, collateralIdx uint32, collateralKey *crypto.PrivateKey) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	if am.privateKey == nil {
		return fmt.Errorf("masternode private key not initialized")
	}

	if am.ServiceAddr == nil {
		return fmt.Errorf("service address not initialized")
	}

	// Create the outpoint
	am.Vin = types.Outpoint{
		Hash:  collateralTx,
		Index: collateralIdx,
	}

	// Create broadcast message
	broadcast, err := am.createBroadcastLocked(collateralKey)
	if err != nil {
		am.Status = ActiveNotCapable
		am.NotCapableReason = err.Error()
		return err
	}

	// Add to manager and relay (no origin peer for local broadcasts)
	if am.manager != nil {
		if err := am.manager.ProcessBroadcast(broadcast, ""); err != nil && !errors.Is(err, ErrBroadcastAlreadySeen) {
			am.Status = ActiveNotCapable
			am.NotCapableReason = err.Error()
			return err
		}
	}

	am.Status = ActiveStarted
	// LEGACY COMPATIBILITY: Use network-adjusted time like legacy C++
	am.LastPing = consensus.GetAdjustedTimeAsTime()
	return nil
}

// Stop stops the active masternode
func (am *ActiveMasternode) Stop() {
	am.mu.Lock()
	defer am.mu.Unlock()

	am.Status = ActiveInitial
	am.NotCapableReason = ""
	am.Vin = types.Outpoint{}
}

// createBroadcastLocked creates a masternode broadcast message (must hold lock)
// SECURITY: Captures all required state atomically at function start to prevent
// race conditions if state changes during external calls (like getRecentBlockHash).
func (am *ActiveMasternode) createBroadcastLocked(collateralKey *crypto.PrivateKey) (*MasternodeBroadcast, error) {
	if collateralKey == nil {
		return nil, fmt.Errorf("collateral key required")
	}

	// CRITICAL: Capture all required state atomically at function start
	// This prevents race conditions if state changes during external calls
	vin := am.Vin
	serviceAddr := am.ServiceAddr
	privateKey := am.privateKey
	pubKeyMasternode := am.PubKeyMasternode

	// Validate captured state before proceeding
	if vin.Hash == (types.Hash{}) {
		return nil, fmt.Errorf("collateral outpoint not set")
	}
	if serviceAddr == nil {
		return nil, fmt.Errorf("service address not set")
	}
	if privateKey == nil {
		return nil, fmt.Errorf("masternode private key not set")
	}
	if pubKeyMasternode == nil {
		return nil, fmt.Errorf("masternode public key not set")
	}

	// LEGACY COMPATIBILITY: Use GetAdjustedTime() like legacy C++ (activemasternode.cpp:206,308)
	sigTime := consensus.GetAdjustedTimeUnix()

	// Get recent block hash (tip - 12) - MUST be valid, not ZeroHash
	// Legacy C++ requires valid block hash for broadcast/ping validation:
	// - Ping must reference a block within last 24 blocks (MASTERNODE_PING_SECONDS / BLOCK_TIME)
	// - Broadcasts with ZeroHash are rejected by network peers immediately
	// - Ensures masternode has synced blockchain before announcing
	blockHash, err := am.getRecentBlockHash()
	if err != nil {
		return nil, fmt.Errorf("cannot create broadcast: blockchain not synced (need tip-12 block hash, got error: %w). Ensure node is fully synced before starting masternode", err)
	}

	// Create ping first with recent block hash (using captured state)
	ping := &MasternodePing{
		OutPoint:  vin,
		BlockHash: blockHash,
		SigTime:   sigTime,
	}

	// Sign ping with masternode key (using captured privateKey)
	pingMsg := ping.getSignatureMessage()
	pingSig, err := crypto.SignCompact(privateKey, pingMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to sign ping: %w", err)
	}
	ping.Signature = pingSig

	// Create broadcast (using all captured state)
	broadcast := &MasternodeBroadcast{
		OutPoint:         vin,
		Addr:             serviceAddr,
		PubKeyCollateral: collateralKey.PublicKey(),
		PubKeyMasternode: pubKeyMasternode,
		SigTime:          sigTime,
		Protocol:         ActiveProtocolVersion, // Must match legacy (70928)
		LastPing:         ping,
	}

	// Sign broadcast with collateral key
	broadcastMsg := broadcast.getNewSignatureMessage()
	broadcastSig, err := crypto.SignCompact(collateralKey, broadcastMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to sign broadcast: %w", err)
	}
	broadcast.Signature = broadcastSig

	return broadcast, nil
}

// sendPingLocked sends a masternode ping (must hold lock)
// SECURITY: Captures all required state atomically at function start to prevent
// race conditions if state changes during external calls.
func (am *ActiveMasternode) sendPingLocked() error {
	// Check if enough time has passed since last ping (local state)
	timeSinceLastPing := time.Since(am.LastPing)
	if timeSinceLastPing < PingInterval {
		logrus.WithField("since_last_s", int(timeSinceLastPing.Seconds())).
			Debug("sendPingLocked: skipping, too early (local state)")
		return nil // Too early
	}

	// LEGACY COMPAT: Also check masternode's stored ping sigTime
	// C++ Reference: activemasternode.cpp:182-185
	// Legacy checks pmn->IsPingedWithin(MASTERNODE_PING_SECONDS) before sending
	// This prevents sending redundant pings if network already has a recent one
	if am.manager == nil {
		logrus.Warn("sendPingLocked: manager not configured - skipping network timing checks")
	}
	if am.manager != nil {
		mn, err := am.manager.GetMasternode(am.Vin)
		if err != nil {
			// LEGACY COMPATIBILITY: Match C++ SendMasternodePing() behavior (activemasternode.cpp:230-234)
			// C++ sets status = ACTIVE_MASTERNODE_NOT_CAPABLE when Find(vin) returns NULL,
			// causing the next ManageStatus() call to re-enter the registration path.
			am.Status = ActiveNotCapable

			// Determine recovery strategy based on wallet capability:
			// - Cold wallet (has collateral): clear Vin to trigger re-discovery and fresh broadcast
			// - Hot node (no collateral): keep Vin, wait for broadcast from cold wallet or dseg
			canSelfBroadcast := am.wallet != nil && am.getBalance != nil && am.getBalance() > 0
			if canSelfBroadcast {
				logrus.WithFields(logrus.Fields{
					"outpoint": am.Vin.String(),
					"error":    err.Error(),
				}).Warn("Active masternode not found in list - will re-broadcast on next cycle")
				am.NotCapableReason = "Masternode removed from list, will re-broadcast"
				am.Vin = types.Outpoint{}
			} else {
				logrus.WithFields(logrus.Fields{
					"outpoint": am.Vin.String(),
					"error":    err.Error(),
				}).Warn("Active masternode not found in list - hot node waiting for remote re-broadcast")
				am.NotCapableReason = "Masternode removed from list, waiting for remote re-broadcast"
			}
			return fmt.Errorf("local masternode not in list: %w", err)
		}
		if mn != nil {
			mn.mu.RLock()
			var lastPingSigTime int64
			if mn.LastPingMessage != nil {
				lastPingSigTime = mn.LastPingMessage.SigTime
			} else {
				// Fallback to broadcast SigTime if no ping yet
				lastPingSigTime = mn.SigTime
			}
			mn.mu.RUnlock()

			pingInterval := int64(PingInterval.Seconds())
			currentTime := consensus.GetAdjustedTimeUnix()
			if currentTime-lastPingSigTime < pingInterval {
				logrus.WithFields(logrus.Fields{
					"since_last_ping_s": currentTime - lastPingSigTime,
					"interval_s":        pingInterval,
				}).Debug("sendPingLocked: skipping, too early (network state)")
				return nil // Too early based on network state
			}
		}
	}

	// CRITICAL: Capture all required state atomically at function start
	vin := am.Vin
	privateKey := am.privateKey

	// Validate captured state
	if privateKey == nil {
		return fmt.Errorf("masternode private key not set")
	}
	if vin.Hash == (types.Hash{}) {
		return fmt.Errorf("collateral outpoint not set")
	}

	// Get recent block hash (tip - 12) - MUST be valid, not ZeroHash
	// Legacy C++ requires valid block hash for ping validation:
	// - Ping must reference a block within last 24 blocks (MASTERNODE_PING_SECONDS / BLOCK_TIME)
	// - Pings with ZeroHash are rejected by network peers immediately
	// - Ensures masternode maintains blockchain sync while active
	blockHash, err := am.getRecentBlockHash()
	if err != nil {
		return fmt.Errorf("cannot send ping: blockchain not synced (need tip-12 block hash, got error: %w). Check node sync status", err)
	}

	// Create ping with recent block hash (using captured state)
	// LEGACY COMPATIBILITY: Use GetAdjustedTime() like legacy C++
	sigTime := consensus.GetAdjustedTimeUnix()
	ping := &MasternodePing{
		OutPoint:  vin,
		BlockHash: blockHash,
		SigTime:   sigTime,
	}

	// Sign ping (using captured privateKey)
	pingMsg := ping.getSignatureMessage()
	sig, err := crypto.SignCompact(privateKey, pingMsg)
	if err != nil {
		return fmt.Errorf("failed to sign ping: %w", err)
	}
	ping.Signature = sig

	// Update in manager
	if am.manager != nil {
		if err := am.manager.ProcessPing(ping, "local"); err != nil {
			return fmt.Errorf("ProcessPing rejected local ping: %w", err)
		}
	}

	// LEGACY COMPATIBILITY: Use network-adjusted time like legacy C++
	am.LastPing = consensus.GetAdjustedTimeAsTime()
	logrus.WithField("outpoint", vin.String()).Info("Active masternode ping sent successfully")
	return nil
}

// SendPing manually sends a masternode ping
func (am *ActiveMasternode) SendPing() error {
	am.mu.Lock()
	defer am.mu.Unlock()

	if am.Status != ActiveStarted {
		return fmt.Errorf("masternode is not started")
	}

	return am.sendPingLocked()
}

// EnableHotColdMasterNode enables hot/cold masternode mode from masternode.conf
// Hot mode: Local wallet has collateral key, can auto-start
// Cold mode: Remote wallet controls, MN just runs with operator key
// Legacy: CActiveMasternode::EnableHotColdMasterNode() in activemasternode.cpp
func (am *ActiveMasternode) EnableHotColdMasterNode(entry *MasternodeEntry, collateralKey *crypto.PrivateKey) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	// Parse masternode private key from entry
	mnPrivKey, err := crypto.DecodeWIF(entry.PrivKey)
	if err != nil {
		am.Status = ActiveNotCapable
		am.NotCapableReason = fmt.Sprintf("invalid masternode private key: %v", err)
		return fmt.Errorf("invalid masternode private key for %s: %w", entry.Alias, err)
	}

	// Parse service address
	addr, err := net.ResolveTCPAddr("tcp", entry.IP)
	if err != nil {
		am.Status = ActiveNotCapable
		am.NotCapableReason = fmt.Sprintf("invalid service address: %v", err)
		return fmt.Errorf("invalid service address for %s: %w", entry.Alias, err)
	}

	// Validate port
	port := addr.Port
	if err := ValidatePort(port, am.isMainnet); err != nil {
		am.Status = ActiveNotCapable
		am.NotCapableReason = err.Error()
		return err
	}

	// Set identity
	am.privateKey = mnPrivKey
	am.PubKeyMasternode = mnPrivKey.PublicKey()
	am.ServiceAddr = addr
	am.Vin = entry.GetOutpoint()

	// If collateral key is provided, we're in hot mode - auto-broadcast
	if collateralKey != nil {
		broadcast, err := am.createBroadcastFromEntryLocked(entry, collateralKey)
		if err != nil {
			am.Status = ActiveNotCapable
			am.NotCapableReason = err.Error()
			return err
		}

		// Process and relay broadcast (no origin peer for local broadcasts)
		if am.manager != nil {
			if err := am.manager.ProcessBroadcast(broadcast, ""); err != nil && !errors.Is(err, ErrBroadcastAlreadySeen) {
				am.Status = ActiveNotCapable
				am.NotCapableReason = err.Error()
				return err
			}
		}

		am.Status = ActiveStarted
		// LEGACY COMPATIBILITY: Use network-adjusted time like legacy C++
		am.LastPing = consensus.GetAdjustedTimeAsTime()
		return nil
	}

	// Cold mode - waiting for remote activation
	am.Status = ActiveNotCapable
	am.NotCapableReason = "Hot node, waiting for remote activation"
	return nil
}

// EnableHotColdMasterNodeRemote enables hot-cold mode from a received broadcast
// This is the simple version called from ProcessBroadcast when we receive a broadcast
// matching our masternode key - enables remote activation from cold wallet.
// Legacy: CActiveMasternode::EnableHotColdMasterNode(CTxIn& newVin, CService& newService)
// at activemasternode.cpp:468-481
func (am *ActiveMasternode) EnableHotColdMasterNodeRemote(outpoint types.Outpoint, addr net.Addr) bool {
	am.mu.Lock()
	defer am.mu.Unlock()

	// Legacy: if (!fMasterNode) return false;
	// We always return true if we have a private key configured
	if am.privateKey == nil {
		return false
	}

	am.Status = ActiveStarted

	// The values below are needed for signing mnping messages going forward
	am.Vin = outpoint
	am.ServiceAddr = addr

	logrus.Info("Hot-cold masternode enabled! You may shut down the cold daemon.")

	return true
}

// createBroadcastFromEntryLocked creates a broadcast from conf entry (must hold lock)
func (am *ActiveMasternode) createBroadcastFromEntryLocked(entry *MasternodeEntry, collateralKey *crypto.PrivateKey) (*MasternodeBroadcast, error) {
	// Parse masternode private key from entry
	mnPrivKey, err := crypto.DecodeWIF(entry.PrivKey)
	if err != nil {
		return nil, fmt.Errorf("invalid masternode private key: %w", err)
	}

	// Parse service address
	addr, err := net.ResolveTCPAddr("tcp", entry.IP)
	if err != nil {
		return nil, fmt.Errorf("invalid service address: %w", err)
	}

	// LEGACY COMPATIBILITY: Use GetAdjustedTime() like legacy C++
	sigTime := consensus.GetAdjustedTimeUnix()

	// Get recent block hash (tip - 12) - MUST be valid, not ZeroHash
	// Legacy C++ requires valid block hash for broadcast/ping validation:
	// - Ping must reference a block within last 24 blocks (MASTERNODE_PING_SECONDS / BLOCK_TIME)
	// - Broadcasts with ZeroHash are rejected by network peers immediately
	blockHash, err := am.getRecentBlockHash()
	if err != nil {
		return nil, fmt.Errorf("cannot create broadcast: blockchain not synced (need tip-12 block hash, got error: %w). Ensure node is fully synced before starting masternode", err)
	}

	// Create ping with recent block hash
	ping := &MasternodePing{
		OutPoint:  entry.GetOutpoint(),
		BlockHash: blockHash,
		SigTime:   sigTime,
	}

	// Sign ping with masternode key
	pingMsg := ping.getSignatureMessage()
	pingSig, err := crypto.SignCompact(mnPrivKey, pingMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to sign ping: %w", err)
	}
	ping.Signature = pingSig

	// Create broadcast
	broadcast := &MasternodeBroadcast{
		OutPoint:         entry.GetOutpoint(),
		Addr:             addr,
		PubKeyCollateral: collateralKey.PublicKey(),
		PubKeyMasternode: mnPrivKey.PublicKey(),
		SigTime:          sigTime,
		Protocol:         ActiveProtocolVersion,
		LastPing:         ping,
	}

	// Sign broadcast with collateral key
	broadcastMsg := broadcast.getNewSignatureMessage()
	broadcastSig, err := crypto.SignCompact(collateralKey, broadcastMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to sign broadcast: %w", err)
	}
	broadcast.Signature = broadcastSig

	return broadcast, nil
}

// CreateBroadcastFromConf creates a broadcast for a remote masternode from masternode.conf
func (am *ActiveMasternode) CreateBroadcastFromConf(entry *MasternodeEntry, collateralKey *crypto.PrivateKey) (*MasternodeBroadcast, error) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	// Parse masternode private key from entry
	mnPrivKey, err := crypto.DecodeWIF(entry.PrivKey)
	if err != nil {
		return nil, fmt.Errorf("invalid masternode private key for %s: %w", entry.Alias, err)
	}

	// Parse service address
	addr, err := net.ResolveTCPAddr("tcp", entry.IP)
	if err != nil {
		return nil, fmt.Errorf("invalid service address for %s: %w", entry.Alias, err)
	}

	// LEGACY COMPATIBILITY: Use GetAdjustedTime() like legacy C++
	sigTime := consensus.GetAdjustedTimeUnix()

	// Get recent block hash (tip - 12) - MUST be valid, not ZeroHash
	// Legacy C++ requires valid block hash for ping validation:
	// - Ping must reference a block within last 24 blocks (MASTERNODE_PING_SECONDS / BLOCK_TIME)
	// - Broadcasts with ZeroHash are rejected by network peers immediately
	blockHash, err := am.getRecentBlockHash()
	if err != nil {
		return nil, fmt.Errorf("cannot create broadcast for %s: blockchain not synced (need tip-12 block hash, got error: %w). Ensure node is fully synced", entry.Alias, err)
	}

	// Create ping with recent block hash
	ping := &MasternodePing{
		OutPoint:  entry.GetOutpoint(),
		BlockHash: blockHash,
		SigTime:   sigTime,
	}

	// Sign ping with masternode key
	pingMsg := ping.getSignatureMessage()
	pingSig, err := crypto.SignCompact(mnPrivKey, pingMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to sign ping: %w", err)
	}
	ping.Signature = pingSig

	// Create broadcast
	broadcast := &MasternodeBroadcast{
		OutPoint:         entry.GetOutpoint(),
		Addr:             addr,
		PubKeyCollateral: collateralKey.PublicKey(),
		PubKeyMasternode: mnPrivKey.PublicKey(),
		SigTime:          sigTime,
		Protocol:         ActiveProtocolVersion, // Must match legacy (70928)
		LastPing:         ping,
	}

	// Sign broadcast with collateral key
	broadcastMsg := broadcast.getNewSignatureMessage()
	broadcastSig, err := crypto.SignCompact(collateralKey, broadcastMsg)
	if err != nil {
		return nil, fmt.Errorf("failed to sign broadcast: %w", err)
	}
	broadcast.Signature = broadcastSig

	return broadcast, nil
}
