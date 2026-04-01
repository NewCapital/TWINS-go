package masternode

import (
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/twins-dev/twins-core/internal/consensus"
	"github.com/twins-dev/twins-core/pkg/crypto"
	"github.com/twins-dev/twins-core/pkg/types"
)

// Ping validation constants.
// PingTimeTolerance and PingSpacingGrace are int64 (not time.Duration) because they
// participate directly in Unix timestamp arithmetic (sigTime comparisons).
const (
	PingTimeTolerance     int64  = 3600 // 1 hour - max clock drift for ping signatures
	PingSpacingGrace      int64  = 60   // 1 minute - grace period subtracted from MinPingSeconds for spacing check
	PingBlockHashMaxDepth uint32 = 24   // Max block depth for valid ping block hashes
	PingBadSignatureDoS          = 33   // DoS score for invalid ping signatures
)

// PingProcessResult contains the result of ping processing
type PingProcessResult struct {
	Accepted   bool   // Whether the ping was accepted
	ShouldSkip bool   // If true, skip but don't punish (e.g., duplicate, too early)
	DoS        int    // Denial-of-service punishment score (0 = don't punish)
	Error      string // Error message if not accepted
	Relay      bool   // Whether to relay this ping to peers
}

// BlockHashValidator validates block hashes for ping messages
type BlockHashValidator interface {
	// GetBlockHeightByHash returns the height of a block given its hash
	// Returns error if block not found
	GetBlockHeightByHash(hash types.Hash) (uint32, error)
	// GetBestHeight returns the current chain tip height
	GetBestHeight() (uint32, error)
}

// PingManager handles masternode ping/pong protocol
type PingManager struct {
	manager          *Manager
	logger           *logrus.Entry
	pingInterval     time.Duration
	pingTimeout      time.Duration
	broadcastFunc    func(*MasternodePing) error // Callback for broadcasting pings to P2P network
	blockValidator   BlockHashValidator          // For validating ping block hashes
	seenPings        map[types.Hash]int64        // Map of ping hash -> sigTime for deduplication
	seenPingsMu      sync.RWMutex                // Protects seenPings map
	maxSeenPingsAge  int64                       // Max age in seconds for seenPings entries
}

// NewPingManager creates a new ping manager
func NewPingManager(manager *Manager, logger *logrus.Logger) *PingManager {
	if logger == nil {
		logger = logrus.New()
	}

	return &PingManager{
		manager:         manager,
		logger:          logger.WithField("component", "masternode-ping"),
		pingInterval:    5 * time.Minute, // LEGACY COMPATIBILITY: matches MASTERNODE_PING_SECONDS (5*60)
		pingTimeout:     time.Duration(ExpirationSeconds) * time.Second, // 7200s = 2 hours, matches legacy MASTERNODE_EXPIRATION_SECONDS
		seenPings:       make(map[types.Hash]int64),
		maxSeenPingsAge: PingTimeTolerance, // matches sigTime tolerance
	}
}

// SetBlockValidator sets the block hash validator for ping verification
func (pm *PingManager) SetBlockValidator(validator BlockHashValidator) {
	pm.blockValidator = validator
}

// CreatePing creates a ping message for a masternode
func (pm *PingManager) CreatePing(
	outpoint types.Outpoint,
	blockHash types.Hash,
	privateKey *crypto.PrivateKey,
	sentinelVersion string,
) (*MasternodePing, error) {
	// Check masternode exists
	_, err := pm.manager.GetMasternode(outpoint)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMasternodeNotFound, err)
	}

	// Create ping message
	// LEGACY COMPATIBILITY: Use GetAdjustedTime() for sigTime (masternode.cpp:86,399,419,716,777,787)
	ping := &MasternodePing{
		OutPoint:        outpoint,
		BlockHash:       blockHash,
		SigTime:         int64(consensus.GetAdjustedTime()),
		SentinelPing:    sentinelVersion != "",
		SentinelVersion: sentinelVersion,
	}

	// Sign the ping
	signature, err := pm.signPing(ping, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign ping: %w", err)
	}

	ping.Signature = signature

	return ping, nil
}

// signPing signs a ping message using compact signature format (65 bytes)
// This matches legacy CMasternodePing::Sign() which uses obfuScationSigner.SignMessage
func (pm *PingManager) signPing(ping *MasternodePing, privateKey *crypto.PrivateKey) ([]byte, error) {
	// Create string message matching legacy format
	// Legacy: vin.ToString() + blockHash.ToString() + std::to_string(sigTime)
	message := ping.getSignatureMessage()

	// Sign using compact signature format (65 bytes with Bitcoin message magic)
	// This matches legacy obfuScationSigner.SignMessage which uses CKey::SignCompact
	signature, err := crypto.SignCompact(privateKey, message)
	if err != nil {
		return nil, fmt.Errorf("failed to sign message: %w", err)
	}

	return signature, nil
}

// ProcessPing processes an incoming ping message (legacy simple interface)
// Returns error if ping is invalid. For full control, use CheckAndUpdatePing instead.
// LEGACY COMPATIBILITY: Uses GetAdjustedTime() for network-synchronized time validation
func (pm *PingManager) ProcessPing(ping *MasternodePing) error {
	// Use network-adjusted time like legacy (masternode.cpp uses GetAdjustedTime)
	adjustedTime := int64(consensus.GetAdjustedTime())
	result := pm.CheckAndUpdatePing(ping, adjustedTime, false, false)
	if !result.Accepted {
		return fmt.Errorf("ping rejected: %s", result.Error)
	}
	return nil
}

// CheckAndUpdatePing performs full ping validation matching legacy CMasternodePing::CheckAndUpdate()
// Parameters:
//   - ping: the ping message to validate
//   - currentTime: current Unix timestamp (GetAdjustedTime in legacy)
//   - requireEnabled: if true, reject pings from non-enabled masternodes
//   - checkSigTimeOnly: if true, only validate sigTime and signature (used for broadcast validation)
//
// Returns: PingProcessResult with acceptance status, DoS score, and relay flag
func (pm *PingManager) CheckAndUpdatePing(ping *MasternodePing, currentTime int64, requireEnabled bool, checkSigTimeOnly bool) PingProcessResult {
	if result := pm.validatePingSigTime(ping, currentTime); !result.Accepted {
		return result
	}

	if result := pm.checkDuplicatePing(ping); result.ShouldSkip {
		return result
	}

	if checkSigTimeOnly {
		return pm.validatePingSigOnly(ping)
	}

	mn, result := pm.findAndValidateMasternode(ping, requireEnabled)
	if mn == nil {
		return result
	}

	if result := pm.validatePingSpacing(ping, mn); !result.Accepted {
		return result
	}

	if result := pm.validatePingSignature(ping, mn); !result.Accepted {
		return result
	}

	if result := pm.validatePingBlockHash(ping); !result.Accepted {
		return result
	}

	result = pm.applyPingUpdate(ping, mn)
	return result
}

// validatePingSigTime checks sigTime is within PingTimeTolerance of current time.
// Legacy: sigTime > GetAdjustedTime() + 60*60 and sigTime <= GetAdjustedTime() - 60*60
func (pm *PingManager) validatePingSigTime(ping *MasternodePing, currentTime int64) PingProcessResult {
	if ping.SigTime > currentTime+PingTimeTolerance {
		return PingProcessResult{
			Accepted: false,
			DoS:      1,
			Error:    fmt.Sprintf("signature rejected, too far into the future: sigTime=%d, now=%d", ping.SigTime, currentTime),
		}
	}

	if ping.SigTime <= currentTime-PingTimeTolerance {
		return PingProcessResult{
			Accepted: false,
			DoS:      1,
			Error:    fmt.Sprintf("signature rejected, too far into the past: sigTime=%d, now=%d", ping.SigTime, currentTime),
		}
	}

	return PingProcessResult{Accepted: true}
}

// checkDuplicatePing checks if we've already seen this ping (deduplication).
// Must happen before spacing check because duplicate pings have the same sigTime.
func (pm *PingManager) checkDuplicatePing(ping *MasternodePing) PingProcessResult {
	pingHash := ping.GetHash()
	pm.seenPingsMu.RLock()
	_, seen := pm.seenPings[pingHash]
	pm.seenPingsMu.RUnlock()

	if seen {
		return PingProcessResult{
			Accepted:   true,
			ShouldSkip: true,
			Relay:      false,
		}
	}

	return PingProcessResult{Accepted: true}
}

// validatePingSigOnly verifies signature without full validation (used for broadcast validation).
// Legacy: if(fCheckSigTimeOnly) { return VerifySignature(pmn->pubKeyMasternode, nDos); }
func (pm *PingManager) validatePingSigOnly(ping *MasternodePing) PingProcessResult {
	mn, err := pm.manager.GetMasternode(ping.OutPoint)
	if err != nil {
		return PingProcessResult{Accepted: true}
	}

	if err := ping.Verify(mn.PubKey); err != nil {
		return PingProcessResult{
			Accepted: false,
			DoS:      PingBadSignatureDoS,
			Error:    fmt.Sprintf("bad ping signature: %v", err),
		}
	}

	return PingProcessResult{Accepted: true}
}

// findAndValidateMasternode looks up the masternode and checks protocol version and enabled status.
// Returns the masternode and a result; if mn is nil the result should be returned by the caller.
func (pm *PingManager) findAndValidateMasternode(ping *MasternodePing, requireEnabled bool) (*Masternode, PingProcessResult) {
	mn, err := pm.manager.GetMasternode(ping.OutPoint)
	if err != nil {
		return nil, PingProcessResult{
			Accepted: false,
			Error:    fmt.Sprintf("couldn't find compatible masternode: %v", err),
		}
	}

	mn.mu.RLock()
	protocol := mn.Protocol
	status := mn.Status
	mn.mu.RUnlock()

	if protocol < pm.manager.getMinMasternodePaymentsProto() {
		return nil, PingProcessResult{
			Accepted: false,
			Error:    fmt.Sprintf("masternode protocol %d below minimum %d", protocol, pm.manager.getMinMasternodePaymentsProto()),
		}
	}

	if requireEnabled && status != StatusEnabled {
		return nil, PingProcessResult{
			Accepted:   false,
			ShouldSkip: true,
			Error:      "masternode is not enabled",
		}
	}

	return mn, PingProcessResult{Accepted: true}
}

// validatePingSpacing ensures enough time has elapsed since the last ping.
// Legacy: if (!pmn->IsPingedWithin(MASTERNODE_MIN_MNP_SECONDS - 60, sigTime))
func (pm *PingManager) validatePingSpacing(ping *MasternodePing, mn *Masternode) PingProcessResult {
	mn.mu.RLock()
	lastPingSigTime := int64(0)
	if mn.LastPingMessage != nil {
		lastPingSigTime = mn.LastPingMessage.SigTime
	}
	mn.mu.RUnlock()

	pingSpacingThreshold := MinPingSeconds - PingSpacingGrace
	if lastPingSigTime > 0 && (ping.SigTime-lastPingSigTime) < pingSpacingThreshold {
		return PingProcessResult{
			Accepted:   false,
			ShouldSkip: true,
			Error:      fmt.Sprintf("ping arrived too early: %d seconds since last (need %d)", ping.SigTime-lastPingSigTime, pingSpacingThreshold),
		}
	}

	return PingProcessResult{Accepted: true}
}

// validatePingSignature verifies the ping's cryptographic signature.
func (pm *PingManager) validatePingSignature(ping *MasternodePing, mn *Masternode) PingProcessResult {
	mn.mu.RLock()
	pubKey := mn.PubKey
	mn.mu.RUnlock()

	if err := ping.Verify(pubKey); err != nil {
		return PingProcessResult{
			Accepted: false,
			DoS:      PingBadSignatureDoS,
			Error:    fmt.Sprintf("bad ping signature: %v", err),
		}
	}

	return PingProcessResult{Accepted: true}
}

// validatePingBlockHash checks that the ping's block hash is recent enough.
// Legacy: if ((*mi).second->nHeight < chainActive.Height() - 24)
func (pm *PingManager) validatePingBlockHash(ping *MasternodePing) PingProcessResult {
	if pm.blockValidator == nil {
		return PingProcessResult{Accepted: true}
	}

	blockHeight, err := pm.blockValidator.GetBlockHeightByHash(ping.BlockHash)
	if err != nil {
		pm.logger.WithFields(logrus.Fields{
			"outpoint":  ping.OutPoint.String(),
			"blockhash": ping.BlockHash.String(),
		}).Debug("Ping block hash is unknown, might be out of sync")
		return PingProcessResult{
			Accepted:   false,
			ShouldSkip: true,
			Error:      "ping block hash is unknown",
		}
	}

	bestHeight, err := pm.blockValidator.GetBestHeight()
	if err == nil && bestHeight > PingBlockHashMaxDepth && blockHeight < bestHeight-PingBlockHashMaxDepth {
		pm.logger.WithFields(logrus.Fields{
			"outpoint":    ping.OutPoint.String(),
			"blockhash":   ping.BlockHash.String(),
			"blockheight": blockHeight,
			"bestheight":  bestHeight,
		}).Debug("Ping block hash is too old")
		return PingProcessResult{
			Accepted:   false,
			ShouldSkip: true,
			Error:      fmt.Sprintf("ping block hash is too old: height %d, need >= %d", blockHeight, bestHeight-PingBlockHashMaxDepth),
		}
	}

	return PingProcessResult{Accepted: true}
}

// applyPingUpdate updates masternode state after all validations pass and records the ping.
func (pm *PingManager) applyPingUpdate(ping *MasternodePing, mn *Masternode) PingProcessResult {
	// Use ping's sigTime instead of time.Now() for deterministic status checks
	sigTimeT := time.Unix(ping.SigTime, 0)
	mn.mu.Lock()
	mn.LastPing = sigTimeT
	mn.LastSeen = sigTimeT
	mn.LastPingMessage = ping

	if ping.SentinelPing {
		mn.SentinelPing = sigTimeT
		mn.SentinelVersion = ping.SentinelVersion
	}
	mn.mu.Unlock()

	mn.UpdateStatus(consensus.GetAdjustedTimeAsTime(), pm.pingTimeout)

	mn.mu.RLock()
	newStatus := mn.Status
	mn.mu.RUnlock()

	if newStatus != StatusEnabled && newStatus != StatusPreEnabled {
		return PingProcessResult{
			Accepted:   false,
			ShouldSkip: true,
			Error:      fmt.Sprintf("masternode status is %s after update", newStatus.String()),
		}
	}

	pingHash := ping.GetHash()
	pm.seenPingsMu.Lock()
	pm.seenPings[pingHash] = ping.SigTime
	pm.seenPingsMu.Unlock()

	pm.logger.WithFields(logrus.Fields{
		"outpoint":  ping.OutPoint.String(),
		"blockhash": ping.BlockHash.String(),
		"sentinel":  ping.SentinelPing,
	}).Debug("Masternode ping accepted")

	return PingProcessResult{
		Accepted: true,
		Relay:    true,
	}
}

// CleanSeenPings removes old entries from seenPings map
// Should be called periodically to prevent memory growth
func (pm *PingManager) CleanSeenPings() {
	pm.seenPingsMu.Lock()
	defer pm.seenPingsMu.Unlock()

	// LEGACY COMPATIBILITY: Use network-adjusted time like legacy C++
	currentTime := consensus.GetAdjustedTimeUnix()
	for hash, sigTime := range pm.seenPings {
		if currentTime-sigTime > pm.maxSeenPingsAge {
			delete(pm.seenPings, hash)
		}
	}
}

// verifyPing verifies a ping message signature
func (pm *PingManager) verifyPing(ping *MasternodePing, pubKey *crypto.PublicKey) error {
	// Use the ping's Verify method which properly handles compact signatures from legacy nodes
	return ping.Verify(pubKey)
}

// BroadcastPing broadcasts a ping message to the network
func (pm *PingManager) BroadcastPing(ping *MasternodePing) error {
	// Process locally first
	if err := pm.ProcessPing(ping); err != nil {
		return fmt.Errorf("failed to process ping locally: %w", err)
	}

	// Broadcast to P2P network if handler is set
	if pm.broadcastFunc != nil {
		if err := pm.broadcastFunc(ping); err != nil {
			pm.logger.WithError(err).Warn("Failed to broadcast ping to P2P network")
			// Don't return error - local processing succeeded
		}
	}

	return nil
}

// SetBroadcastHandler sets the handler for broadcasting pings to P2P network
func (pm *PingManager) SetBroadcastHandler(handler func(*MasternodePing) error) {
	pm.broadcastFunc = handler
}

// GetPingStatus returns the ping status for all masternodes
func (pm *PingManager) GetPingStatus() []*PingStatus {
	pm.manager.mu.RLock()
	defer pm.manager.mu.RUnlock()

	var status []*PingStatus
	// LEGACY COMPATIBILITY: Use network-adjusted time like legacy C++
	currentTime := consensus.GetAdjustedTimeAsTime()

	for _, mn := range pm.manager.masternodes {
		mn.mu.RLock()

		timeSinceLastPing := currentTime.Sub(mn.LastPing)
		isHealthy := timeSinceLastPing < pm.pingTimeout

		ps := &PingStatus{
			OutPoint:          mn.OutPoint,
			LastPing:          mn.LastPing,
			LastSeen:          mn.LastSeen,
			TimeSinceLastPing: timeSinceLastPing,
			IsHealthy:         isHealthy,
			Status:            mn.Status,
			SentinelPing:      mn.SentinelPing,
			SentinelVersion:   mn.SentinelVersion,
		}

		mn.mu.RUnlock()
		status = append(status, ps)
	}

	return status
}

// GetMasternodePingInfo returns detailed ping information for a masternode
func (pm *PingManager) GetMasternodePingInfo(outpoint types.Outpoint) (*PingInfo, error) {
	mn, err := pm.manager.GetMasternode(outpoint)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMasternodeNotFound, err)
	}

	mn.mu.RLock()
	defer mn.mu.RUnlock()

	// LEGACY COMPATIBILITY: Use network-adjusted time like legacy C++
	currentTime := consensus.GetAdjustedTimeAsTime()
	timeSinceLastPing := currentTime.Sub(mn.LastPing)
	isHealthy := timeSinceLastPing < pm.pingTimeout

	info := &PingInfo{
		OutPoint:          outpoint,
		LastPing:          mn.LastPing,
		LastSeen:          mn.LastSeen,
		TimeSinceLastPing: timeSinceLastPing,
		IsHealthy:         isHealthy,
		PingInterval:      pm.pingInterval,
		PingTimeout:       pm.pingTimeout,
		Status:            mn.Status,
		SentinelEnabled:   mn.SentinelVersion != "",
		SentinelPing:      mn.SentinelPing,
		SentinelVersion:   mn.SentinelVersion,
	}

	return info, nil
}

// PingStatus represents the ping status of a masternode
type PingStatus struct {
	OutPoint          types.Outpoint
	LastPing          time.Time
	LastSeen          time.Time
	TimeSinceLastPing time.Duration
	IsHealthy         bool
	Status            MasternodeStatus
	SentinelPing      time.Time
	SentinelVersion   string
}

// PingInfo contains detailed ping information for a masternode
type PingInfo struct {
	OutPoint          types.Outpoint
	LastPing          time.Time
	LastSeen          time.Time
	TimeSinceLastPing time.Duration
	IsHealthy         bool
	PingInterval      time.Duration
	PingTimeout       time.Duration
	Status            MasternodeStatus
	SentinelEnabled   bool
	SentinelPing      time.Time
	SentinelVersion   string
}

// PingStatistics contains network-wide ping statistics
type PingStatistics struct {
	TotalMasternodes  int
	HealthyNodes      int
	UnhealthyNodes    int
	ExpiredNodes      int
	AveragePingTime   time.Duration
	SentinelEnabled   int
}