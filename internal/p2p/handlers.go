package p2p

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	mrand "math/rand"
	"net"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/twins-dev/twins-core/internal/mempool"
	"github.com/twins-dev/twins-core/pkg/types"
)

// LocalHeightLag defines the maximum number of blocks an outbound peer can be
// behind our local chain height before being disconnected and banned at connection time.
// Peers more than this many blocks behind our known-good local chain are
// clearly stale/stuck and waste connection slots.
const LocalHeightLag uint32 = 1000

// Helper function to convert MessageType to [12]byte command
func commandToBytes(cmd MessageType) [12]byte {
	var result [12]byte
	copy(result[:], []byte(cmd))
	return result
}

// Helper function to get magic bytes directly from params
// Note: We use the NetMagicBytes directly to avoid any endianness confusion
func (s *Server) getMagicBytes() [4]byte {
	return s.params.NetMagicBytes
}

// =============================================================================
// Inventory Message Processing Helpers
// =============================================================================

// inventoryStats holds categorized counts of inventory items for logging and filtering.
type inventoryStats struct {
	firstBlockHash string
	lastBlockHash  string
	blockCount     int
	txCount        int
	mnCount        int
	sporkCount     int
	otherCount     int
	total          int
}

// countInventoryTypes categorizes and counts inventory items by type.
// It also records block announcements to the health tracker for peer height updates.
func (s *Server) countInventoryTypes(peer *Peer, invList []InventoryVector) *inventoryStats {
	stats := &inventoryStats{total: len(invList)}

	for _, inv := range invList {
		switch inv.Type {
		case InvTypeBlock:
			if stats.firstBlockHash == "" {
				stats.firstBlockHash = inv.Hash.String()
			}
			stats.lastBlockHash = inv.Hash.String()
			stats.blockCount++
			// Update peer's best known height from this block hash
			s.UpdateBlockAvailability(peer, inv.Hash)
			// Record block announcement for height update when block is saved
			if s.syncer != nil {
				if healthTracker := s.syncer.GetHealthTracker(); healthTracker != nil {
					healthTracker.RecordBlockAnnouncement(peer.GetAddress().String(), inv.Hash)
				}
			}
		case InvTypeTx:
			stats.txCount++
		case InvTypeMN:
			stats.mnCount++
		case InvTypeSpork:
			stats.sporkCount++
		default:
			stats.otherCount++
		}
	}

	return stats
}

// shouldIgnoreInventoryDuringIBD returns true if the inventory should be ignored during IBD.
// During IBD, we ignore non-block inventory and small block broadcasts (unsolicited announcements).
func (s *Server) shouldIgnoreInventoryDuringIBD(peer *Peer, stats *inventoryStats, isIBD bool) bool {
	if !isIBD {
		return false
	}

	// Ignore non-block inventory during IBD
	if stats.blockCount == 0 {
		s.logger.WithFields(logrus.Fields{
			"peer":        peer.GetAddress().String(),
			"txs":         stats.txCount,
			"masternodes": stats.mnCount,
			"sporks":      stats.sporkCount,
		}).Debug("Ignoring non-block inventory during IBD")
		return true
	}

	// Ignore small block broadcasts during IBD (unsolicited announcements)
	// Response to our getblocks request typically has 500 blocks
	if stats.blockCount < 10 {
		s.logger.WithFields(logrus.Fields{
			"peer":   peer.GetAddress().String(),
			"blocks": stats.blockCount,
		}).Debug("Ignoring small block broadcast during IBD (unsolicited)")
		return true
	}

	return false
}

// logInventoryReceived logs detailed information about received inventory.
func (s *Server) logInventoryReceived(peer *Peer, invList []InventoryVector, stats *inventoryStats, syncStatus string) {
	s.logger.WithFields(logrus.Fields{
		"first_block": stats.firstBlockHash,
		"last_block":  stats.lastBlockHash,
		"peer":        peer.GetAddress().String(),
		"total":       stats.total,
		"blocks":      stats.blockCount,
		"txs":         stats.txCount,
		"masternodes": stats.mnCount,
		"sporks":      stats.sporkCount,
		"other":       stats.otherCount,
	}).Debugf("Received inventory%s", syncStatus)

	// Log first few items for debugging
	for i, inv := range invList {
		if i >= 3 {
			break
		}
		typeName := invTypeName(inv.Type)
		s.logger.WithFields(logrus.Fields{
			"index": i,
			"type":  fmt.Sprintf("%s(%d)", typeName, inv.Type),
			"hash":  inv.Hash.String(),
		}).Debug("Inventory item")
	}
}

// invTypeName returns a human-readable name for an inventory type.
func invTypeName(invType InvType) string {
	switch invType {
	case InvTypeBlock:
		return "block"
	case InvTypeTx:
		return "tx"
	case InvTypeMN:
		return "masternode"
	case InvTypeSpork:
		return "spork"
	case InvTypeMasternodePing:
		return "mnping"
	case InvTypeMasternodeWinner:
		return "mnwinner"
	default:
		return "unknown"
	}
}

// routeInventoryToBatchSync attempts to route inventory to the batch syncer.
// Returns true if successfully routed, false if should continue with normal processing.
func (s *Server) routeInventoryToBatchSync(peer *Peer, invList []InventoryVector, stats *inventoryStats) bool {
	if s.syncer == nil {
		return false
	}

	batchInProgress := s.syncer.IsBatchInProgress()
	isSyncing := s.syncer.IsSyncing()

	s.logger.WithFields(logrus.Fields{
		"batch_in_progress": batchInProgress,
		"is_syncing":        isSyncing,
		"block_count":       stats.blockCount,
		"tx_count":          stats.txCount,
		"peer":              peer.GetAddress().String(),
	}).Debug("Inventory routing check")

	if !batchInProgress {
		s.logger.WithFields(logrus.Fields{
			"reason": "batch_not_in_progress",
			"blocks": stats.blockCount,
		}).Debug("Not routing to batch - batch not in progress")
		return false
	}

	s.logger.WithFields(logrus.Fields{
		"peer":   peer.GetAddress().String(),
		"blocks": stats.blockCount,
		"txs":    stats.txCount,
	}).Debug("Received inventory - routing to batch sync")

	if s.syncer.RouteInventoryToBatch(invList) {
		s.logger.Debug("Successfully routed inventory to batch")
		return true
	}

	s.logger.Warn("Failed to route INV to batch, processing normally")
	return false
}

// buildInventoryRequestList processes inventory items and builds a list of items to request via getdata.
func (s *Server) buildInventoryRequestList(peer *Peer, invList []InventoryVector, isIBD bool) []InventoryVector {
	var requestItems []InventoryVector

	for _, inv := range invList {
		if item := s.processInventoryItem(peer, inv, isIBD); item != nil {
			requestItems = append(requestItems, *item)
		}
	}

	return requestItems
}

// processInventoryItem processes a single inventory item and returns it if we should request it.
// Returns nil if we already have the item or should skip it.
func (s *Server) processInventoryItem(peer *Peer, inv InventoryVector, isIBD bool) *InventoryVector {
	switch inv.Type {
	case InvTypeTx:
		return s.processTransactionInventory(inv, isIBD)
	case InvTypeBlock:
		return s.processBlockInventory(peer, inv)
	case InvTypeMN:
		return s.processMasternodeInventory(peer, inv, isIBD)
	case InvTypeSpork:
		return s.processSporkInventory(peer, inv)
	case InvTypeMasternodePing:
		return s.processMasternodePingInventory(peer, inv, isIBD)
	case InvTypeMasternodeWinner:
		return s.processMasternodeWinnerInventory(peer, inv, isIBD)
	default:
		s.logger.WithFields(logrus.Fields{
			"peer": peer.GetAddress().String(),
			"type": inv.Type,
			"hash": inv.Hash.String(),
		}).Debug("Unknown inventory type")
		return nil
	}
}

func (s *Server) processTransactionInventory(inv InventoryVector, isIBD bool) *InventoryVector {
	if isIBD {
		return nil // Skip transactions during IBD
	}
	if s.mempool != nil && !s.mempool.HasTransaction(inv.Hash) {
		return &inv
	}
	return nil
}

func (s *Server) processBlockInventory(peer *Peer, inv InventoryVector) *InventoryVector {
	if bs := s.syncer; bs != nil && bs.IsSyncing() {
		s.logger.WithFields(logrus.Fields{
			"hash":    inv.Hash.String(),
			"syncing": true,
		}).Debug("Requesting block during sync")
		return &inv
	}

	if s.blockchain == nil {
		return nil
	}

	has, err := s.blockchain.HasBlock(inv.Hash)
	if err != nil || has {
		return nil
	}

	// Check if we already requested this block (deduplication with stale retry)
	s.pendingBlockRequestsMu.RLock()
	requestTime, alreadyRequested := s.pendingBlockRequests[inv.Hash]
	s.pendingBlockRequestsMu.RUnlock()

	if alreadyRequested && time.Since(requestTime) < 30*time.Second {
		return nil
	}

	// Mark as requested
	s.pendingBlockRequestsMu.Lock()
	s.pendingBlockRequests[inv.Hash] = time.Now()
	s.pendingBlockRequestsMu.Unlock()

	return &inv
}

func (s *Server) processMasternodeInventory(peer *Peer, inv InventoryVector, isIBD bool) *InventoryVector {
	if isIBD {
		return nil // Skip during IBD
	}

	s.logger.WithFields(logrus.Fields{
		"peer": peer.GetAddress().String(),
		"hash": inv.Hash.String(),
	}).Debug("Received masternode inventory")

	if s.mnManager != nil {
		if _, err := s.mnManager.GetBroadcastByHash(inv.Hash); err == nil {
			return nil // Already have it
		}
	}

	s.logger.WithField("hash", inv.Hash.String()).Debug("Requesting masternode broadcast")
	return &inv
}

func (s *Server) processSporkInventory(peer *Peer, inv InventoryVector) *InventoryVector {
	s.logger.WithFields(logrus.Fields{
		"peer": peer.GetAddress().String(),
		"hash": inv.Hash.String(),
	}).Debug("Received spork inventory")
	return &inv // Always request sporks
}

func (s *Server) processMasternodePingInventory(peer *Peer, inv InventoryVector, isIBD bool) *InventoryVector {
	if isIBD {
		return nil // Skip during IBD
	}

	if s.mnManager != nil {
		type seenPingChecker interface {
			HasSeenPing(hash types.Hash) bool
		}
		if checker, ok := s.mnManager.(seenPingChecker); ok {
			if checker.HasSeenPing(inv.Hash) {
				return nil // Already know this ping hash, no need to request again
			}
		} else if ping := s.mnManager.GetPingByHash(inv.Hash); ping != nil {
			return nil // Already have this ping payload, no need to request again
		}
	}

	s.logger.WithFields(logrus.Fields{
		"peer": peer.GetAddress().String(),
		"hash": inv.Hash.String(),
	}).Debug("Received masternode ping inventory")
	return &inv // Always request pings - they update masternode status
}

func (s *Server) processMasternodeWinnerInventory(peer *Peer, inv InventoryVector, isIBD bool) *InventoryVector {
	if isIBD {
		return nil // Skip during IBD
	}

	s.logger.WithFields(logrus.Fields{
		"peer": peer.GetAddress().String(),
		"hash": inv.Hash.String(),
	}).Debug("Received masternode winner inventory")
	return &inv // Request winner votes for payment consensus
}

// sendGetDataForInventory sends a getdata message for the requested inventory items.
func (s *Server) sendGetDataForInventory(peer *Peer, requestItems []InventoryVector) {
	if len(requestItems) == 0 {
		return
	}

	// Count what we're requesting for logging
	reqBlockCount, reqTxCount, reqOtherCount := 0, 0, 0
	for _, inv := range requestItems {
		switch inv.Type {
		case InvTypeBlock:
			reqBlockCount++
		case InvTypeTx:
			reqTxCount++
		default:
			reqOtherCount++
		}
	}

	s.logger.WithFields(logrus.Fields{
		"peer":   peer.GetAddress().String(),
		"total":  len(requestItems),
		"blocks": reqBlockCount,
		"txs":    reqTxCount,
		"other":  reqOtherCount,
	}).Debug("Sending getdata request")

	getDataMsg := &GetDataMessage{InvList: requestItems}
	payload, err := s.serializeGetDataMessage(getDataMsg)
	if err != nil {
		s.logger.WithError(err).WithField("peer", peer.GetAddress().String()).
			Error("Failed to serialize getdata message")
		return
	}

	getDataPkt := NewMessage(MsgGetData, payload, s.params.NetMagicBytes)
	if err := peer.SendMessage(getDataPkt); err != nil {
		s.logger.WithError(err).WithField("peer", peer.GetAddress().String()).
			Error("Failed to send getdata message")
		return
	}

	s.logger.WithFields(logrus.Fields{
		"peer":  peer.GetAddress().String(),
		"count": len(requestItems),
	}).Debug("Successfully sent getdata message")
}

// =============================================================================
// Message handlers for the P2P server
// =============================================================================

// Message handlers for the P2P server
// These handlers process specific protocol messages from peers

// handleVersionMessage processes a version message from a peer
func (s *Server) handleVersionMessage(peer *Peer, msg *Message) {
	if peer.IsHandshakeComplete() {
		s.logger.WithField("peer", peer.GetAddress().String()).
			Warn("Received version message from peer with completed handshake")
		s.Misbehaving(peer, 1, "duplicate version message")
		return
	}

	// Deserialize version message
	version, err := DeserializeVersionMessage(msg.Payload)
	if err != nil {
		s.logger.WithError(err).WithField("peer", peer.GetAddress().String()).
			Error("Failed to deserialize version message")
		select {
		case s.banPeers <- banRequest{peer: peer, reason: "failed to deserialize version message"}:
		case <-s.quit:
		}
		return
	}

	// Validate protocol version against minimum accepted version.
	// Protocol 70928: Accept peers >= MinPeerProtocol (70927) for backward compatibility.
	if version.Version < MinPeerProtocol {
		s.logger.WithFields(logrus.Fields{
			"peer":         peer.GetAddress().String(),
			"peer_version": version.Version,
			"min_version":  MinPeerProtocol,
		}).Warn("Peer protocol version too old")
		select {
		case s.donePeers <- peer:
		case <-s.quit:
		}
		return
	}

	// Self-connection detection via nonce.
	// If the remote sent a nonce that we generated, both sides are this node.
	// Legacy: main.cpp:5760-5766 — if (IsLocalHost(addrMe) || nNonce == nLocalHostNonce)
	if s.isSelfNonce(version.Nonce) {
		s.logger.WithField("peer", peer.GetAddress().String()).
			Warn("Self-connection detected via nonce, disconnecting")
		select {
		case s.donePeers <- peer:
		case <-s.quit:
		}
		return
	}

	// Learn our external IP from what outbound peers see as AddrRecv.
	// Only trust outbound peers (we initiated the connection) to prevent spoofing.
	if !peer.inbound && s.getExternalIP() == nil {
		if version.AddrRecv.IP != nil && !version.AddrRecv.IP.IsUnspecified() &&
			!version.AddrRecv.IP.IsLoopback() && version.AddrRecv.IsRoutable() {
			s.SetExternalIP(version.AddrRecv.IP)
			s.logger.WithField("external_ip", version.AddrRecv.IP.String()).
				Info("Learned external IP from outbound peer")
		}
	}

	// Store peer version info (but do NOT mark handshake complete yet).
	// The handshake flag is set after version+verack are queued to writeQueue,
	// preventing concurrent broadcasts from reaching the TCP stream before
	// the handshake messages.
	peer.SetPeerVersion(version)

	// Detect masternode tier from service flags
	isMasternode := (version.Services & ServiceFlagMasternode) != 0
	tier := TierNone
	if version.Services&ServiceFlagMasternodePlat != 0 {
		tier = TierPlatinum
	} else if version.Services&ServiceFlagMasternodeGold != 0 {
		tier = TierGold
	} else if version.Services&ServiceFlagMasternodeSilver != 0 {
		tier = TierSilver
	} else if version.Services&ServiceFlagMasternodeBronze != 0 {
		tier = TierBronze
	}

	// Notify bootstrap manager if active
	if s.bootstrap != nil && s.bootstrap.IsActive() {
		s.bootstrap.OnPeerDiscovered(
			peer.GetAddress().String(),
			uint32(version.StartHeight),
			uint32(version.Version),
			version.Services,
			version.UserAgent,
		)
	}

	// Record in health tracker
	if s.healthTracker != nil {
		s.healthTracker.RecordPeerDiscovered(
			peer.GetAddress().String(),
			uint32(version.StartHeight),
			isMasternode,
			tier,
			!peer.inbound, // isOutbound = !inbound
		)
	}

	// Early disconnect+ban for outbound peers with stale height.
	// If an outbound peer reports a height significantly below our local chain,
	// it's clearly stuck/dead and wastes a connection slot. Ban immediately
	// to prevent reconnection via peer discovery.
	if !peer.inbound {
		if localHeight, err := s.blockchain.GetBestHeight(); err == nil && localHeight > LocalHeightLag {
			peerHeight := uint32(version.StartHeight)
			minAcceptableHeight := localHeight - LocalHeightLag
			if peerHeight < minAcceptableHeight {
				s.logger.WithFields(logrus.Fields{
					"peer":         peer.GetAddress().String(),
					"peer_height":  peerHeight,
					"local_height": localHeight,
					"min_height":   minAcceptableHeight,
					"lag":          localHeight - peerHeight,
				}).Warn("Banning stale outbound peer far below local chain height")
				select {
				case s.banPeers <- banRequest{peer: peer, reason: "stale peer far below local chain height"}:
				case <-s.quit:
				}
				return
			}
		}
	}

	// Add peer time data for network time adjustment (matches legacy)
	// Use peer's timestamp to calculate network time offset
	// Security: Validate timestamp is reasonable to prevent time manipulation attacks
	if version.Timestamp > 0 && s.onPeerTime != nil {
		localTime := time.Now().Unix()
		peerTime := version.Timestamp
		offset := peerTime - localTime

		// Reject obviously wrong timestamps (>2 hours off)
		// Legacy: main.cpp:5865 validates before AddTimeData
		if offset > MaxClockOffsetSeconds || offset < -MaxClockOffsetSeconds {
			s.logger.WithFields(logrus.Fields{
				"peer":         peer.GetAddress().String(),
				"peer_time":    peerTime,
				"local_time":   localTime,
				"offset_hours": offset / 3600,
			}).Warn("Rejecting peer with unreasonable timestamp for time adjustment")
		} else {
			s.onPeerTime(peer.GetAddress().String(), uint32(version.Timestamp))
		}
	}

	// For inbound connections, send our version BEFORE verack
	// Bitcoin protocol requires: receive version -> send version -> send verack
	// Legacy C++ code expects to receive our version before our verack
	if peer.inbound {
		ourVersion := s.createVersionMessage(peer.GetAddress())
		s.registerSentNonce(ourVersion.Nonce)
		peer.localNonce.Store(ourVersion.Nonce)
		versionPayload, err := SerializeVersionMessage(ourVersion)
		if err != nil {
			s.logger.WithError(err).WithField("peer", peer.GetAddress().String()).
				Error("Failed to serialize version message")
			select {
			case s.donePeers <- peer:
			case <-s.quit:
			}
			return
		}

		versionMsg := NewMessage(MsgVersion, versionPayload, s.params.NetMagicBytes)
		if err := peer.SendMessage(versionMsg); err != nil {
			s.logger.WithError(err).WithField("peer", peer.GetAddress().String()).
				Error("Failed to send version message")
			select {
			case s.donePeers <- peer:
			case <-s.quit:
			}
			return
		}
	}

	// Send verack to acknowledge their version message
	// This MUST come AFTER we send our version for inbound connections
	verackMsg := NewMessage(MsgVerAck, nil, s.params.NetMagicBytes)
	if err := peer.SendMessage(verackMsg); err != nil {
		s.logger.WithError(err).WithField("peer", peer.GetAddress().String()).
			Error("Failed to send verack message")
		select {
		case s.donePeers <- peer:
		case <-s.quit:
		}
		return
	}

	// Mark handshake complete AFTER version+verack are queued to writeQueue.
	// This prevents the race where a concurrent broadcast (RelayBlock, BroadcastTransaction,
	// etc.) sees IsHandshakeComplete()==true and pushes a message into writeQueue BEFORE
	// our version/verack, causing the remote to receive a non-handshake message first.
	peer.MarkHandshakeComplete()

	s.logger.WithFields(logrus.Fields{
		"peer":         peer.GetAddress().String(),
		"version":      version.Version,
		"services":     version.Services,
		"user_agent":   version.UserAgent,
		"start_height": version.StartHeight,
	}).Debug("Peer handshake completed")
}

// handleVerAckMessage processes a verack message from a peer.
// Verack must arrive AFTER version in the Bitcoin protocol handshake.
// If version was never received, this is a protocol violation.
func (s *Server) handleVerAckMessage(peer *Peer, msg *Message) {
	// Verack without prior version is a protocol violation — disconnect.
	// Normal flow: version → verack. A peer sending verack first is buggy or malicious.
	if peer.GetVersion() == nil {
		s.logger.WithField("peer", peer.GetAddress().String()).
			Warn("Received verack before version, disconnecting")
		select {
		case s.donePeers <- peer:
		case <-s.quit:
		}
		return
	}

	s.logger.WithField("peer", peer.GetAddress().String()).
		Debug("Received verack from peer")

	// Send getaddr message to request peer addresses
	// NOTE: Only send to outbound peers (peers we connected to)
	// Legacy behavior: main.cpp:5820-5836 only sends getaddr when !pfrom->fInbound
	// This prevents both sides from waiting for each other's addr response
	if !peer.inbound {
		getAddrMsg := NewMessage(MsgGetAddr, []byte{}, s.params.NetMagicBytes)
		if err := peer.SendMessage(getAddrMsg); err != nil {
			s.logger.WithError(err).WithField("peer", peer.GetAddress().String()).
				Warn("Failed to send getaddr message")
		} else {
			peer.fGetAddr.Store(true) // Matches legacy main.cpp:5837
			s.logger.WithField("peer", peer.GetAddress().String()).
				Debug("Sent getaddr request to peer")
		}
	}

	// Send getsporks message to request active sporks
	getSporksMsg := NewMessage(MsgGetSporks, []byte{}, s.params.NetMagicBytes)
	if err := peer.SendMessage(getSporksMsg); err != nil {
		s.logger.WithError(err).WithField("peer", peer.GetAddress().String()).
			Warn("Failed to send getsporks message")
	} else {
		s.logger.WithField("peer", peer.GetAddress().String()).
			Debug("Sent getsporks request to peer")
	}

	// Legacy C++ only sends own address via PushAddress during version handshake for
	// outbound peers (main.cpp:5820-5831), NOT 1000 from the address book.
	// The getaddr request (sent above) is the proper mechanism for address exchange.
	// Proactively dumping 1000 addresses here caused addr message flooding.

	// Notify the syncer about this peer for health tracking
	if s.syncer != nil {
		s.syncer.OnPeerDiscovered(peer)
	}

	// Notify bootstrap manager about peer discovery
	if s.bootstrap != nil && !s.bootstrap.IsCompleted() {
		version := peer.GetVersion()
		if version != nil {
			s.bootstrap.OnPeerDiscovered(
				peer.GetAddress().String(),
				uint32(version.StartHeight),
				uint32(version.Version),
				version.Services,
				version.UserAgent,
			)

			s.logger.WithFields(logrus.Fields{
				"peer":   peer.GetAddress().String(),
				"height": version.StartHeight,
			}).Debug("Notified bootstrap manager of peer discovery")
		}
	}

	// Note: Sync is no longer initiated immediately after handshake.
	// Instead, it starts after bootstrap phase completes in server.go Start()
}

// handlePingMessage processes a ping message from a peer.
// Protocol 70928+: ping is 12 bytes (nonce + height). Legacy: 8 bytes (nonce only).
// Pong response mirrors the sender's format: 12 bytes if we support 70928, 8 bytes otherwise.
func (s *Server) handlePingMessage(peer *Peer, msg *Message) {
	var nonce uint64
	var peerHeight uint32

	switch len(msg.Payload) {
	case 12:
		// Protocol 70928: Nonce(8) + Height(4)
		nonce = binary.LittleEndian.Uint64(msg.Payload[0:8])
		peerHeight = binary.LittleEndian.Uint32(msg.Payload[8:12])
		// Update peer's reported height
		peer.SetPeerHeight(peerHeight)
		// Update health tracker with new height
		if s.healthTracker != nil {
			s.healthTracker.UpdateBestKnownHeight(peer.GetAddress().String(), peerHeight)
		}
	case 8:
		// Legacy: Nonce(8) only
		nonce = binary.LittleEndian.Uint64(msg.Payload)
	default:
		s.logger.WithField("peer", peer.GetAddress().String()).
			Warn("Invalid ping message size")
		return
	}

	// Build version-gated pong response.
	// If the peer supports 70928, include our height; otherwise send legacy 8-byte pong.
	var pongPayload []byte
	if peer.SupportsProto70928() {
		pongPayload = make([]byte, 12)
		binary.LittleEndian.PutUint64(pongPayload[0:8], nonce)
		// Include our current chain height
		if bestHeight, err := s.blockchain.GetBestHeight(); err == nil {
			binary.LittleEndian.PutUint32(pongPayload[8:12], bestHeight)
		}
	} else {
		pongPayload = make([]byte, 8)
		binary.LittleEndian.PutUint64(pongPayload, nonce)
	}

	pongMsg := NewMessage(MsgPong, pongPayload, s.params.NetMagicBytes)
	if err := peer.SendMessage(pongMsg); err != nil {
		s.logger.WithError(err).WithField("peer", peer.GetAddress().String()).
			Error("Failed to send pong message")
		return
	}

	s.logger.WithFields(logrus.Fields{
		"peer":        peer.GetAddress().String(),
		"nonce":       nonce,
		"peer_height": peerHeight,
	}).Debug("Sent pong response")
}

// Note: handlePongMessage removed - pong messages are now handled directly in peer.readLoop()
// to avoid msgChan bottleneck during sync (see peer.go:529-541)

// handleAddrMessage processes an addr message from a peer
func (s *Server) handleAddrMessage(peer *Peer, msg *Message) {
	peerAddr := peer.GetAddress().String() // cache once — hot in profiling

	addrMsg, err := s.deserializeAddrMessage(msg.Payload)
	if err != nil {
		s.logger.WithError(err).WithField("peer", peerAddr).
			Error("Failed to deserialize addr message")
		return
	}

	if len(addrMsg.Addresses) > MaxAddrMessages {
		s.logger.WithFields(logrus.Fields{
			"peer":  peerAddr,
			"count": len(addrMsg.Addresses),
			"max":   MaxAddrMessages,
		}).Warn("Received addr message with too many addresses")
		s.Misbehaving(peer, 20, "oversized addr message")
		return
	}

	// Per-peer addr message rate limiting: max MaxAddrMsgPerWindow messages per AddrMsgWindow seconds.
	// Matches getblocks/getheaders rate limiting pattern (handlers.go:1943-1960).
	now := time.Now()
	nowUnix := now.Unix()
	windowEnd := peer.addrMsgWindowEnd.Load()
	if nowUnix >= windowEnd {
		// Window expired — start new window
		peer.addrMsgCount.Store(1)
		peer.addrMsgWindowEnd.Store(nowUnix + AddrMsgWindow)
	} else {
		count := peer.addrMsgCount.Add(1)
		if int(count) > MaxAddrMsgPerWindow {
			s.Misbehaving(peer, 20, "addr message flood")
			return
		}
	}

	s.logger.WithFields(logrus.Fields{
		"peer":  peerAddr,
		"count": len(addrMsg.Addresses),
	}).Debug("Received peer addresses")

	// Process addresses and add to address manager
	if s.discovery != nil {
		peerNA := peer.GetAddress()
		for _, addr := range addrMsg.Addresses {
			if s.isSelfAddress(&addr) {
				continue
			}

			// Apply time penalty before storing — matches C++ addrman.cpp:263-264, 289
			// CAddrMan::Add_() applies nTimePenalty (2 hours) to prevent addresses from
			// appearing artificially fresh after relay. This limits the effective relay
			// distance to ~1 hop since penalized timestamps fail the 10-minute freshness check.
			penalizedTime := int64(addr.Time) - AddrTimePenalty
			if penalizedTime < 0 {
				penalizedTime = 0
			}

			known := &KnownAddress{
				Addr:     &addr,
				Source:   peerNA,
				LastSeen: time.Unix(penalizedTime, 0),
				Services: addr.Services,
			}

			s.discovery.AddAddress(known, peerNA, SourcePeer)
		}
	}

	// Relay fresh addresses to other peers (addresses received in last 10 minutes)
	// Legacy main.cpp:5914 guards:
	// 1. Don't relay solicited getaddr responses (fGetAddr == true)
	// 2. Don't relay large addr batches (> 10) — only small organic updates get relayed
	// 3. Per-address: must be fresh (< 10 min) and routable (no RFC1918/loopback)
	// 4. Relay dedup: skip addresses already relayed within AddrRelayDedupSec (new)
	if s.discovery != nil && len(addrMsg.Addresses) > 0 &&
		!peer.fGetAddr.Load() && len(addrMsg.Addresses) <= 10 {
		freshAddrs := make([]NetAddress, 0)
		for _, addr := range addrMsg.Addresses {
			addrTime := time.Unix(int64(addr.Time), 0)
			if now.Sub(addrTime) < 10*time.Minute && addr.IsRoutable() {
				// Relay dedup: skip if we already relayed this address recently
				addrKey := addr.String()
				if s.isAddrRecentlyRelayed(addrKey, nowUnix) {
					continue
				}
				freshAddrs = append(freshAddrs, addr)
			}
		}

		// Relay to other peers and mark as relayed
		if len(freshAddrs) > 0 {
			s.relayAddresses(freshAddrs, peer)
			s.markAddrsRelayed(freshAddrs, nowUnix)
		}
	}

	// Clear fGetAddr after processing a non-full response (matches legacy main.cpp:5946-5947)
	if len(addrMsg.Addresses) < 1000 {
		peer.fGetAddr.Store(false)
	}
}

// isAddrRecentlyRelayed checks if an address was relayed within the dedup window.
func (s *Server) isAddrRecentlyRelayed(addrKey string, nowUnix int64) bool {
	s.addrRelayDedupMu.RLock()
	expiry, exists := s.addrRelayDedup[addrKey]
	s.addrRelayDedupMu.RUnlock()
	return exists && nowUnix < expiry
}

// markAddrsRelayed records addresses in the relay dedup cache with expiry.
// Also performs lazy cleanup of expired entries when the cache grows large.
func (s *Server) markAddrsRelayed(addrs []NetAddress, nowUnix int64) {
	expiry := nowUnix + AddrRelayDedupSec
	s.addrRelayDedupMu.Lock()
	for _, addr := range addrs {
		s.addrRelayDedup[addr.String()] = expiry
	}
	// Lazy cleanup: when cache exceeds 10K entries, remove expired ones
	if len(s.addrRelayDedup) > 10000 {
		for key, exp := range s.addrRelayDedup {
			if nowUnix >= exp {
				delete(s.addrRelayDedup, key)
			}
		}
	}
	s.addrRelayDedupMu.Unlock()
}

// handleGetAddrMessage processes a getaddr message from a peer
func (s *Server) handleGetAddrMessage(peer *Peer, msg *Message) {
	s.logger.WithField("peer", peer.GetAddress().String()).
		Debug("Received getaddr request")

	// SECURITY: Only respond to inbound peers (match legacy behavior)
	// This prevents address database harvesting and network topology mapping attacks
	if !peer.inbound {
		s.logger.WithField("peer", peer.GetAddress().String()).
			Debug("Ignoring getaddr from outbound peer (security policy)")
		return
	}

	// Respond with known peer addresses
	if s.discovery != nil {
		// Get up to 1000 addresses from our address manager
		addresses := s.discovery.GetAddresses(1000)
		if len(addresses) > 0 {
			s.sendAddrMessage(peer, addresses)
			s.logger.WithFields(logrus.Fields{
				"peer":  peer.GetAddress().String(),
				"count": len(addresses),
			}).Debug("Sent addresses to peer")
		}
	}
}

// UpdateBlockAvailability updates peer's best known block height based on announced block hash.
// This is called for each block hash in INV messages to track peer's chain tip.
// Mirrors Bitcoin Core's UpdateBlockAvailability() in main.cpp.
func (s *Server) UpdateBlockAvailability(peer *Peer, blockHash types.Hash) {
	if s.blockchain == nil || s.syncer == nil {
		return
	}

	healthTracker := s.syncer.GetHealthTracker()
	if healthTracker == nil {
		return
	}

	// Look up the block in our blockchain
	height, err := s.blockchain.GetBlockHeight(blockHash)
	if err != nil {
		// Block not found in our chain - this is expected for blocks we don't have yet
		// Legacy stores this as hashLastUnknownBlock for later processing
		// For now, we skip unknown blocks - they'll be processed when received
		return
	}

	// Update peer's best known height if this block is higher
	peerAddr := peer.GetAddress().String()
	healthTracker.UpdateBestKnownHeight(peerAddr, height)
}

// handleInvMessage processes an inv message from a peer.
// Decomposed into focused helper functions for maintainability.
func (s *Server) handleInvMessage(peer *Peer, msg *Message) {
	// Step 1: Deserialize and validate (pass peer for 70928 extended inv format)
	invMsg, invHeights, err := s.deserializeInvMessage(msg.Payload, peer)
	if err != nil {
		s.logger.WithError(err).WithField("peer", peer.GetAddress().String()).
			Error("Failed to deserialize inv message")
		return
	}

	// Protocol 70928: use heights from inv to update peer's best known height
	if invHeights != nil && s.healthTracker != nil {
		peerAddr := peer.GetAddress().String()
		for i, iv := range invMsg.InvList {
			if iv.Type == InvTypeBlock && invHeights[i] > 0 {
				s.healthTracker.UpdateBestKnownHeight(peerAddr, invHeights[i])
			}
		}
	}

	if len(invMsg.InvList) > MaxInvMessages {
		s.logger.WithFields(logrus.Fields{
			"peer":  peer.GetAddress().String(),
			"count": len(invMsg.InvList),
			"max":   MaxInvMessages,
		}).Warn("Received inv message with too many items")
		s.Misbehaving(peer, 20, "oversized inv message")
		return
	}

	// Check send buffer overflow before processing inventory.
	// If the peer's write queue is nearly full, they are causing us to buffer
	// excessive outbound data. Legacy: main.cpp:5992 — Misbehaving(50).
	if queueLen, queueCap := len(peer.writeQueue), cap(peer.writeQueue); queueCap > 0 && queueLen > queueCap*9/10 {
		s.logger.WithFields(logrus.Fields{
			"peer":      peer.GetAddress().String(),
			"queue_len": queueLen,
			"queue_cap": queueCap,
		}).Warn("Send buffer overflow during inv processing")
		s.Misbehaving(peer, 50, "send buffer overflow")
		return
	}

	// Step 2: Count and categorize inventory types
	stats := s.countInventoryTypes(peer, invMsg.InvList)

	// Step 3: Check IBD status and filter if needed
	isIBD := s.blockchain != nil && s.blockchain.IsInitialBlockDownload()
	if s.shouldIgnoreInventoryDuringIBD(peer, stats, isIBD) {
		return
	}

	// Step 4: Build sync status string and log inventory
	syncStatus := s.buildSyncStatusString()
	s.logInventoryReceived(peer, invMsg.InvList, stats, syncStatus)

	// Step 5: Try to route to batch sync if in progress
	if s.routeInventoryToBatchSync(peer, invMsg.InvList, stats) {
		return // Batch sync will handle this
	}

	// Step 6: Notify syncer about inventory for sync continuation tracking
	if s.syncer != nil {
		s.syncer.AnnounceInventory(peer, invMsg.InvList)
	}

	// Step 7: Build list of items to request
	requestItems := s.buildInventoryRequestList(peer, invMsg.InvList, isIBD)

	// Step 8: Send getdata request if we have items
	if len(requestItems) > 0 {
		s.sendGetDataForInventory(peer, requestItems)
	} else {
		s.logger.WithFields(logrus.Fields{
			"peer":      peer.GetAddress().String(),
			"inv_count": len(invMsg.InvList),
		}).Debug("No items to request from inventory")
	}
}

// buildSyncStatusString returns a status string for logging (e.g., " SYNCING IBD").
func (s *Server) buildSyncStatusString() string {
	syncStatus := ""
	if s.syncer != nil && s.syncer.IsSyncing() {
		syncStatus = " SYNCING"
	}
	if s.blockchain != nil && s.blockchain.IsInitialBlockDownload() {
		syncStatus += " IBD"
	}
	return syncStatus
}

// handleGetDataMessage processes a getdata message from a peer
func (s *Server) handleGetDataMessage(peer *Peer, msg *Message) {
	getDataMsg, err := s.deserializeGetDataMessage(msg.Payload)
	if err != nil {
		s.logger.WithError(err).WithField("peer", peer.GetAddress().String()).
			Error("Failed to deserialize getdata message")
		return
	}

	// Hard limit: reject getdata exceeding protocol maximum (legacy: MAX_INV_SZ).
	// Check immediately after deserialization, before iterating the list,
	// to avoid unnecessary CPU/memory work on oversized messages.
	if len(getDataMsg.InvList) > MaxInvMessages {
		s.logger.WithFields(logrus.Fields{
			"peer":  peer.GetAddress().String(),
			"count": len(getDataMsg.InvList),
			"max":   MaxInvMessages,
		}).Warn("Received getdata message exceeding protocol limit")
		s.Misbehaving(peer, 20, "oversized getdata message")
		return
	}

	// Count requested types for detailed logging
	reqBlockCount := 0
	reqTxCount := 0
	reqOtherCount := 0
	blockHashes := []types.Hash{}

	for _, inv := range getDataMsg.InvList {
		switch inv.Type {
		case InvTypeBlock:
			reqBlockCount++
			blockHashes = append(blockHashes, inv.Hash)
		case InvTypeTx:
			reqTxCount++
		default:
			reqOtherCount++
		}
	}

	s.logger.WithFields(logrus.Fields{
		"peer":   peer.GetAddress().String(),
		"total":  len(getDataMsg.InvList),
		"blocks": reqBlockCount,
		"txs":    reqTxCount,
		"other":  reqOtherCount,
	}).Debug("Peer requesting data from us")

	// Soft warning: large but legal getdata requests
	maxBatchSize := s.config.Network.MaxGetDataBatchSize
	if maxBatchSize == 0 {
		maxBatchSize = DefaultMaxBatchSize // Default fallback
	}
	if len(getDataMsg.InvList) > maxBatchSize {
		s.logger.WithFields(logrus.Fields{
			"peer":  peer.GetAddress().String(),
			"count": len(getDataMsg.InvList),
		}).Warn("Large getdata request, may indicate DoS attempt")
	}

	// Process block requests
	if len(blockHashes) > 0 {
		// Check if any requested block matches hashContinue (pipelining trigger)
		hashContinue := peer.GetHashContinue()
		triggerPipelining := false
		if hashContinue != nil {
			for _, hash := range blockHashes {
				if hash == *hashContinue {
					triggerPipelining = true
					break
				}
			}
		}

		// Process ALL block requests asynchronously to avoid blocking
		// the messageHandler goroutine with Pebble DB reads.
		// Previously only batches > 50 were async, but even small batches
		// (e.g. 10 blocks × 5ms/read = 50ms) contribute to msgChan backpressure.
		go s.sendBlocksBatchWithPipelining(peer, blockHashes, triggerPipelining)
	}

	// Process other requests immediately (they're typically small)
	for _, inv := range getDataMsg.InvList {
		switch inv.Type {
		case InvTypeTx:
			s.handleTxRequest(peer, inv.Hash)

		case InvTypeMN:
			s.handleMasternodeRequest(peer, inv.Hash)

		case InvTypeSpork:
			s.handleSporkRequest(peer, inv.Hash)

		case InvTypeMasternodePing:
			s.handleMasternodePingRequest(peer, inv.Hash)

		case InvTypeMasternodeWinner:
			s.handleMasternodeWinnerRequest(peer, inv.Hash)

		default:
			s.logger.WithFields(logrus.Fields{
				"peer": peer.GetAddress().String(),
				"type": inv.Type,
				"hash": inv.Hash.String(),
			}).Debug("Unsupported getdata request type")
		}
	}
}

// sendBlocksBatchWithPipelining sends blocks in a batch with hashContinue pipelining support
// Uses natural TCP backpressure instead of artificial rate limiting for maximum throughput
// This matches legacy C++ behavior which relies on TCP flow control
func (s *Server) sendBlocksBatchWithPipelining(peer *Peer, hashes []types.Hash, triggerPipelining bool) {
	s.logger.WithFields(logrus.Fields{
		"peer":               peer.GetAddress().String(),
		"block_count":        len(hashes),
		"trigger_pipelining": triggerPipelining,
	}).Debug("Starting block batch send")

	successCount := 0
	failCount := 0

	for i, hash := range hashes {
		// Check if peer is still connected
		if !peer.IsConnected() {
			s.logger.WithFields(logrus.Fields{
				"peer":      peer.GetAddress().String(),
				"sent":      successCount,
				"remaining": len(hashes) - i,
			}).Debug("Peer disconnected during batch send")
			return
		}

		// Send block - rely on TCP backpressure via write queue
		// No artificial rate limiting - matches legacy C++ behavior
		if s.blockchain != nil {
			block, err := s.blockchain.GetBlock(hash)

			if err != nil {
				s.sendNotFound(peer, InvTypeBlock, hash)
				failCount++
				continue
			}

			if block == nil {
				s.sendNotFound(peer, InvTypeBlock, hash)
				failCount++
				continue
			}

			// Use sendBlock directly (no artificial delays during sync)
			s.sendBlock(peer, block)
			successCount++
		} else {
			s.sendNotFound(peer, InvTypeBlock, hash)
			failCount++
		}
	}

	s.logger.WithFields(logrus.Fields{
		"peer":    peer.GetAddress().String(),
		"success": successCount,
		"failed":  failCount,
		"total":   len(hashes),
	}).Debug("Completed block batch send")

	// Trigger pipelining if requested (legacy hashContinue mechanism)
	if triggerPipelining {
		s.triggerHashContinuePipelining(peer)
	}
}

// triggerHashContinuePipelining sends automatic inv for next batch (legacy compatibility)
func (s *Server) triggerHashContinuePipelining(peer *Peer) {
	// Get current blockchain tip
	if s.blockchain == nil {
		return
	}

	bestHash, err := s.blockchain.GetBestBlockHash()
	if err != nil {
		s.logger.WithError(err).Debug("Failed to get best block hash for pipelining")
		return
	}

	bestHeight, _ := s.blockchain.GetBestHeight()

	s.logger.WithFields(logrus.Fields{
		"peer": peer.GetAddress().String(),
		"tip":  bestHash.String(),
	}).Debug("Triggering hashContinue pipelining (legacy compatibility)")

	// Build inv message with current tip (70928: includes height)
	invPayload := s.buildBlockInvForPeer(peer, []blockInvEntry{{Hash: bestHash, Height: bestHeight}})
	invMsg := NewMessage(MsgInv, invPayload, s.getMagicBytes())

	// Send inv (bypass normal filtering - must send even if redundant)
	if err := peer.SendMessage(invMsg); err != nil {
		s.logger.WithError(err).Debug("Failed to send pipelining inv")
		return
	}

	// Clear hashContinue now that we've sent the follow-up inv
	peer.ClearHashContinue()

	s.logger.WithFields(logrus.Fields{
		"peer": peer.GetAddress().String(),
		"tip":  bestHash.String(),
	}).Debug("Sent pipelining inv and cleared hashContinue")
}

// handleBlockMessage processes a block message from a peer
func (s *Server) handleBlockMessage(peer *Peer, msg *Message) {
	block, err := s.deserializeBlock(msg.Payload)
	if err != nil {
		s.logger.WithError(err).WithField("peer", peer.GetAddress().String()).
			Error("Failed to deserialize block")
		return
	}

	blockHash := block.Header.Hash()

	// Remove from pending requests (deduplication cleanup)
	s.pendingBlockRequestsMu.Lock()
	delete(s.pendingBlockRequests, blockHash)
	s.pendingBlockRequestsMu.Unlock()

	// Check if we're in batch sync mode - if so, route to syncer for sequential processing
	syncing := false
	if s.syncer != nil {
		syncing = s.syncer.IsSyncing()
	}
	if s.syncer != nil && syncing {
		s.logger.WithFields(logrus.Fields{
			"peer":         peer.GetAddress().String(),
			"hash":         blockHash.String(),
			"parent":       block.Header.PrevBlockHash.String(),
			"transactions": len(block.Transactions),
			"size_kb":      len(msg.Payload) / 1024,
		}).Debug("Received block (routing to batch sync)")

		// Route to syncer's batch processing channel
		// The syncer will process blocks sequentially in processBatch()
		s.syncer.OnBlockProcessed(block, peer.GetAddress().String())
		return
	}

	// Not syncing - still route to syncer for async processing
	// This ensures all blocks go through single processing goroutine
	s.logger.WithFields(logrus.Fields{
		"peer":         peer.GetAddress().String(),
		"hash":         blockHash.String(),
		"parent":       block.Header.PrevBlockHash.String(),
		"transactions": len(block.Transactions),
		"size_kb":      len(msg.Payload) / 1024,
	}).Debug("Received block (normal processing - routing to syncer)")

	// Route to syncer for processing - single goroutine handles all blocks
	if s.syncer != nil {
		s.syncer.OnBlockProcessed(block, peer.GetAddress().String())
	}
}

// handleTxMessage processes a transaction message from a peer
func (s *Server) handleTxMessage(peer *Peer, msg *Message) {
	tx, err := s.deserializeTx(msg.Payload)
	if err != nil {
		s.logger.WithError(err).WithField("peer", peer.GetAddress().String()).
			Error("Failed to deserialize transaction")
		return
	}

	txHash := tx.Hash()
	s.logger.WithFields(logrus.Fields{
		"peer":    peer.GetAddress().String(),
		"hash":    txHash.String(),
		"inputs":  len(tx.Inputs),
		"outputs": len(tx.Outputs),
	}).Debug("Received transaction from peer")

	// Add to transaction pool (mempool validates it)
	if s.mempool != nil {
		if err := s.mempool.AddTransaction(tx); err != nil {
			s.logger.WithError(err).WithFields(logrus.Fields{
				"peer": peer.GetAddress().String(),
				"hash": txHash.String(),
			}).Debug("Failed to add transaction to mempool")

			// Penalize peer for DoS-worthy transaction rejections.
			// RejectMalformed is unambiguously the peer's fault (score 10).
			// RejectInvalid may include local conditions (score 1, cumulative).
			if mErr, ok := err.(*mempool.MempoolError); ok {
				switch mErr.Code {
				case mempool.RejectMalformed:
					s.Misbehaving(peer, 10, fmt.Sprintf("malformed transaction: %s", mErr.Message))
				case mempool.RejectInvalid:
					s.Misbehaving(peer, 1, fmt.Sprintf("invalid transaction: %s", mErr.Message))
				}
			}
			return
		}

		// Relay to other peers if valid
		s.relayTransaction(tx, peer)
	}
}

// Helper methods for handling specific requests

// handleTxRequest handles a request for a specific transaction
func (s *Server) handleTxRequest(peer *Peer, txHash types.Hash) {
	s.logger.WithFields(logrus.Fields{
		"peer": peer.GetAddress().String(),
		"hash": txHash.String(),
	}).Debug("Peer requested transaction")

	s.ensureTxRelayInitialized()

	// Look up serialized payload in relay cache first (legacy mapRelay-style fast path).
	if payload, ok := s.txRelayCache.get(txHash, time.Now()); ok {
		msg := NewMessage(MsgTx, payload, s.getMagicBytes())
		if err := peer.SendMessage(msg); err == nil {
			s.markPeerInventoryKnown(peer, txHash)
			return
		}
	}

	// Look up transaction in mempool first
	if s.mempool != nil {
		if tx, err := s.mempool.GetTransaction(txHash); err == nil && tx != nil {
			s.sendTransaction(peer, tx)
			return
		}
	}

	// Try blockchain storage if not in mempool
	if s.blockchain != nil {
		if tx, err := s.blockchain.GetTransaction(txHash); err == nil && tx != nil {
			s.sendTransaction(peer, tx)
			return
		}
	}

	// Transaction not found, send notfound
	s.sendNotFound(peer, InvTypeTx, txHash)
}

// handleBlockRequest handles a request for a specific block
func (s *Server) handleBlockRequest(peer *Peer, blockHash types.Hash) {
	// Look up block in blockchain storage
	if s.blockchain != nil {
		block, err := s.blockchain.GetBlock(blockHash)

		if err != nil {
			s.sendNotFound(peer, InvTypeBlock, blockHash)
			return
		}

		if block != nil {
			s.sendBlock(peer, block)
			return
		}
	}

	s.sendNotFound(peer, InvTypeBlock, blockHash)
}

// handleMasternodeRequest handles a request for masternode data
func (s *Server) handleMasternodeRequest(peer *Peer, hash types.Hash) {
	s.logger.WithFields(logrus.Fields{
		"peer": peer.GetAddress().String(),
		"hash": hash.String(),
	}).Debug("Peer requested masternode data")

	// Check if we have a masternode manager
	if s.mnManager == nil {
		s.logger.Debug("No masternode manager configured")
		s.sendNotFound(peer, InvTypeMN, hash)
		return
	}

	// Query masternode manager for the broadcast by hash
	broadcast, err := s.mnManager.GetBroadcastByHash(hash)
	if err != nil {
		s.logger.WithError(err).WithField("hash", hash.String()).
			Debug("Masternode broadcast not found")
		s.sendNotFound(peer, InvTypeMN, hash)
		return
	}

	// Serialize the broadcast message
	payload, err := SerializeMasternodeBroadcast(broadcast)
	if err != nil {
		s.logger.WithError(err).WithField("hash", hash.String()).
			Error("Failed to serialize masternode broadcast")
		s.sendNotFound(peer, InvTypeMN, hash)
		return
	}

	// Send the broadcast to the peer
	// Create and send masternode broadcast message using NewMessage (properly sets checksum and magic)
	msg := NewMessage(MsgMasternode, payload, s.getMagicBytes())

	if err := peer.SendMessage(msg); err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"peer": peer.GetAddress().String(),
			"hash": hash.String(),
		}).Error("Failed to send masternode broadcast")
		return
	}

	s.logger.WithFields(logrus.Fields{
		"peer": peer.GetAddress().String(),
		"hash": hash.String(),
	}).Debug("Sent masternode broadcast to peer")
}

// handleSporkRequest handles a request for spork data
func (s *Server) handleSporkRequest(peer *Peer, hash types.Hash) {
	s.logger.WithFields(logrus.Fields{
		"peer": peer.GetAddress().String(),
		"hash": hash.String(),
	}).Debug("Peer requested spork data")

	// Sporks are broadcast-only in current implementation
	// We don't store them by hash for retrieval, so send notfound
	s.sendNotFound(peer, InvTypeSpork, hash)
}

// handleMasternodePingRequest handles a request for masternode ping data
// Matches C++ getdata handler for MSG_MASTERNODE_PING which looks up mapSeenMasternodePing
func (s *Server) handleMasternodePingRequest(peer *Peer, hash types.Hash) {
	s.logger.WithFields(logrus.Fields{
		"peer": peer.GetAddress().String(),
		"hash": hash.String(),
	}).Debug("Peer requested masternode ping data")

	if s.mnManager == nil {
		s.sendNotFound(peer, InvTypeMasternodePing, hash)
		return
	}

	ping := s.mnManager.GetPingByHash(hash)
	if ping == nil {
		s.sendNotFound(peer, InvTypeMasternodePing, hash)
		return
	}

	payload, err := SerializeMasternodePing(ping)
	if err != nil {
		s.logger.WithError(err).WithField("hash", hash.String()).
			Error("Failed to serialize masternode ping")
		s.sendNotFound(peer, InvTypeMasternodePing, hash)
		return
	}

	msg := NewMessage(MsgMNPing, payload, s.getMagicBytes())
	if err := peer.SendMessage(msg); err != nil {
		s.logger.WithError(err).WithFields(logrus.Fields{
			"peer": peer.GetAddress().String(),
			"hash": hash.String(),
		}).Error("Failed to send masternode ping")
	}
}

// handleMasternodeWinnerRequest handles a request for masternode winner data
func (s *Server) handleMasternodeWinnerRequest(peer *Peer, hash types.Hash) {
	s.logger.WithFields(logrus.Fields{
		"peer": peer.GetAddress().String(),
		"hash": hash.String(),
	}).Debug("Peer requested masternode winner data")

	// Masternode winners are computed on-demand, not stored by hash
	// Send notfound for now
	s.sendNotFound(peer, InvTypeMasternodeWinner, hash)
}

// Helper methods for creating messages

// createVersionMessage creates a version message for handshake
func (s *Server) createVersionMessage(remoteAddr *NetAddress) *VersionMessage {
	// Generate random nonce
	nonce := make([]byte, 8)
	rand.Read(nonce)
	nonceUint64 := binary.LittleEndian.Uint64(nonce)

	// Get current chain height
	startHeight := int32(0)
	if s.blockchain != nil {
		if height, err := s.blockchain.GetBestHeight(); err == nil {
			startHeight = int32(height)
		}
	}

	// Create local address
	var localAddr NetAddress
	if s.localAddr != nil {
		localAddr = *s.localAddr
	} else {
		localAddr = NetAddress{
			Time:     uint32(time.Now().Unix()),
			Services: s.services,
			IP:       make([]byte, 16), // Zero IP if no local address
			Port:     0,
		}
	}

	return &VersionMessage{
		Version:     ProtocolVersion,
		Services:    s.services,
		Timestamp:   time.Now().Unix(),
		AddrRecv:    *remoteAddr,
		AddrFrom:    localAddr,
		Nonce:       nonceUint64,
		UserAgent:   s.userAgent,
		StartHeight: startHeight,
		Relay:       true, // We relay transactions by default
	}
}

// Message sending helpers

// sendTransaction sends a transaction to a peer
func (s *Server) sendTransaction(peer *Peer, tx *types.Transaction) {
	s.logger.WithFields(logrus.Fields{
		"peer": peer.GetAddress().String(),
		"hash": tx.Hash().String(),
	}).Debug("Sending transaction to peer")

	// Serialize transaction
	txBytes, err := tx.Serialize()
	if err != nil {
		s.logger.WithError(err).Error("Failed to serialize transaction")
		return
	}

	// Create and send tx message using NewMessage (properly sets checksum and magic)
	msg := NewMessage(MsgTx, txBytes, s.getMagicBytes())

	if err := peer.SendMessage(msg); err != nil {
		s.logger.WithError(err).Error("Failed to send transaction message")
		return
	}
	s.markPeerInventoryKnown(peer, tx.Hash())
}

// sendBlock sends a block to a peer
func (s *Server) sendBlock(peer *Peer, block *types.Block) {
	s.logger.WithFields(logrus.Fields{
		"peer": peer.GetAddress().String(),
		"hash": block.Header.Hash().String(),
	}).Debug("Sending block to peer")

	// Serialize block
	blockBytes, err := block.Serialize()
	if err != nil {
		s.logger.WithError(err).Error("Failed to serialize block")
		return
	}

	// Create and send block message using NewMessage (properly sets checksum and magic)
	msg := NewMessage(MsgBlock, blockBytes, s.getMagicBytes())

	if err := peer.SendMessage(msg); err != nil {
		s.logger.WithError(err).Error("Failed to send block message")
	}
}

// sendBlockWithRateLimit sends a block with bandwidth rate limiting
func (s *Server) sendBlockWithRateLimit(peer *Peer, block *types.Block) error {
	// Serialize block first to get size
	blockBytes, err := block.Serialize()
	if err != nil {
		return fmt.Errorf("failed to serialize block: %w", err)
	}

	blockSize := uint64(len(blockBytes))

	// Apply bandwidth rate limiting if bandwidth monitor is available
	if s.bandwidthMonitor != nil {
		maxWait := 10 * time.Second
		waited := time.Duration(0)
		waitIncrement := 100 * time.Millisecond

		// Wait for bandwidth tokens to become available
		for !s.bandwidthMonitor.CanUpload(blockSize) {
			if waited >= maxWait {
				return fmt.Errorf("bandwidth limit timeout after %v", maxWait)
			}
			time.Sleep(waitIncrement)
			waited += waitIncrement
		}

		// Record the upload
		s.bandwidthMonitor.RecordUpload(peer.GetAddress().String(), blockSize)
	}

	// Create and send block message
	msg := NewMessage(MsgBlock, blockBytes, s.getMagicBytes())

	if err := peer.SendMessage(msg); err != nil {
		return fmt.Errorf("failed to send block message: %w", err)
	}

	return nil
}

// sendNotFound sends a notfound message to a peer
func (s *Server) sendNotFound(peer *Peer, invType InvType, hash types.Hash) {
	s.logger.WithFields(logrus.Fields{
		"peer": peer.GetAddress().String(),
		"type": invType,
		"hash": hash.String(),
	}).Debug("Sending notfound to peer")

	// Create notfound inventory
	inv := InventoryVector{
		Type: invType,
		Hash: hash,
	}

	// Serialize notfound message (same format as inv)
	var buf bytes.Buffer
	if err := writeCompactSize(&buf, 1); err != nil {
		s.logger.WithError(err).Warn("Failed to write compact size")
		return
	}
	binary.Write(&buf, binary.LittleEndian, uint32(inv.Type))
	buf.Write(inv.Hash[:])

	// Create and send notfound message using NewMessage (properly sets checksum and magic)
	msg := NewMessage(MsgNotFound, buf.Bytes(), s.getMagicBytes())

	if err := peer.SendMessage(msg); err != nil {
		s.logger.WithError(err).Error("Failed to send notfound message")
	}
}

// relayBlock relays a block to other peers (except the one we received it from).
// Protocol 70928+: block inv items include height for peers that support it.
// Also pushes updated chain height to all peers and invalidates chainstate cache.
func (s *Server) relayBlock(block *types.Block, exceptPeer *Peer) {
	blockHash := block.Header.Hash()

	s.logger.WithField("hash", blockHash.String()).Debug("Relaying block to peers")

	// Protocol 70928: Push updated height to all peers and invalidate chainstate cache
	if bestHeight, err := s.blockchain.GetBestHeight(); err == nil {
		s.UpdatePeerHeights(bestHeight)
	}
	s.InvalidateChainStateCache()

	// Get block height for 70928 inv extension
	blockHeight, _ := s.blockchain.GetBlockHeight(blockHash)

	// Per-peer serialization: 70928+ peers get height, legacy peers get standard inv
	s.peers.Range(func(key, value interface{}) bool {
		peer := value.(*Peer)
		if peer == exceptPeer || !peer.IsHandshakeComplete() || !peer.IsConnected() {
			return true
		}

		var buf bytes.Buffer
		if err := writeCompactSize(&buf, 1); err != nil {
			return true
		}
		binary.Write(&buf, binary.LittleEndian, uint32(InvTypeBlock))
		buf.Write(blockHash[:])
		// Protocol 70928: append height for peers that support it
		if peer.SupportsProto70928() {
			binary.Write(&buf, binary.LittleEndian, blockHeight)
		}

		msg := NewMessage(MsgInv, buf.Bytes(), s.getMagicBytes())
		peer.SendMessage(msg)
		return true
	})
}

// relayTransaction relays a transaction to other peers (except the one we received it from)
func (s *Server) relayTransaction(tx *types.Transaction, exceptPeer *Peer) {
	txHash := tx.Hash()

	s.logger.WithField("hash", txHash.String()).Debug("Relaying transaction to peers")

	// Queue per-peer and flush on trickle interval.
	s.relayTransactionToPeers(tx, exceptPeer)
}

// broadcastInventory broadcasts inventory to all peers except one
func (s *Server) broadcastInventory(inv []InventoryVector, exceptPeer *Peer) {
	s.logger.WithField("inv_count", len(inv)).Debug("Broadcasting inventory")

	// Serialize inv message
	var buf bytes.Buffer
	if err := writeCompactSize(&buf, uint64(len(inv))); err != nil {
		s.logger.WithError(err).Warn("Failed to write compact size")
		return
	}

	for _, item := range inv {
		binary.Write(&buf, binary.LittleEndian, uint32(item.Type))
		buf.Write(item.Hash[:])
	}

	// Create inv message using NewMessage (properly sets checksum and magic)
	msg := NewMessage(MsgInv, buf.Bytes(), s.getMagicBytes())

	// Iterate through all peers
	s.peers.Range(func(key, value interface{}) bool {
		peer := value.(*Peer)

		// Skip the peer we received from
		if exceptPeer != nil && peer.GetAddress().String() == exceptPeer.GetAddress().String() {
			return true
		}

		// Send inv message to peer
		if err := peer.SendMessage(msg); err != nil {
			s.logger.WithError(err).WithField("peer", peer.GetAddress().String()).
				Error("Failed to send inventory to peer")
		}

		return true
	})
}

// Message deserialization helpers

// readCompactSize reads a Bitcoin/TWINS varint (compactSize) from a reader
// Format: if < 0xFD: 1 byte, if 0xFD: 3 bytes (0xFD + 2 bytes LE),
//
//	if 0xFE: 5 bytes (0xFE + 4 bytes LE), if 0xFF: 9 bytes (0xFF + 8 bytes LE)
func readCompactSize(r io.Reader) (uint64, error) {
	var first byte
	if err := binary.Read(r, binary.LittleEndian, &first); err != nil {
		return 0, err
	}

	switch first {
	case 0xFD:
		var v uint16
		if err := binary.Read(r, binary.LittleEndian, &v); err != nil {
			return 0, err
		}
		return uint64(v), nil
	case 0xFE:
		var v uint32
		if err := binary.Read(r, binary.LittleEndian, &v); err != nil {
			return 0, err
		}
		return uint64(v), nil
	case 0xFF:
		var v uint64
		if err := binary.Read(r, binary.LittleEndian, &v); err != nil {
			return 0, err
		}
		return v, nil
	default:
		return uint64(first), nil
	}
}

// writeCompactSize writes a Bitcoin/TWINS varint (compactSize) to a writer
func writeCompactSize(w io.Writer, value uint64) error {
	if value < 0xFD {
		return binary.Write(w, binary.LittleEndian, uint8(value))
	} else if value <= 0xFFFF {
		if err := binary.Write(w, binary.LittleEndian, uint8(0xFD)); err != nil {
			return err
		}
		return binary.Write(w, binary.LittleEndian, uint16(value))
	} else if value <= 0xFFFFFFFF {
		if err := binary.Write(w, binary.LittleEndian, uint8(0xFE)); err != nil {
			return err
		}
		return binary.Write(w, binary.LittleEndian, uint32(value))
	} else {
		if err := binary.Write(w, binary.LittleEndian, uint8(0xFF)); err != nil {
			return err
		}
		return binary.Write(w, binary.LittleEndian, value)
	}
}

// deserializeAddrMessage deserializes an addr message
func (s *Server) deserializeAddrMessage(payload []byte) (*AddrMessage, error) {
	buf := bytes.NewReader(payload)

	// Read count (varint/compactSize)
	count, err := readCompactSize(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read addr count: %w", err)
	}

	if count > MaxAddrMessages {
		return nil, fmt.Errorf("addr count too large: %d", count)
	}

	// Pre-allocate slice with capacity, we'll filter invalid entries
	addresses := make([]NetAddress, 0, count)
	skippedCount := 0

	for i := uint64(0); i < count; i++ {
		// Read timestamp (4 bytes)
		var timestamp uint32
		if err := binary.Read(buf, binary.LittleEndian, &timestamp); err != nil {
			return nil, fmt.Errorf("failed to read timestamp: %w", err)
		}

		// Read services (8 bytes)
		var services uint64
		if err := binary.Read(buf, binary.LittleEndian, &services); err != nil {
			return nil, fmt.Errorf("failed to read services: %w", err)
		}

		// Read IP (16 bytes)
		ip := make([]byte, 16)
		if _, err := buf.Read(ip); err != nil {
			return nil, fmt.Errorf("failed to read IP: %w", err)
		}

		// Read port (2 bytes, big endian)
		var port uint16
		if err := binary.Read(buf, binary.BigEndian, &port); err != nil {
			return nil, fmt.Errorf("failed to read port: %w", err)
		}

		// Validate fields - legacy nodes may have corrupted entries in addrman
		// Valid services should be small (NODE_NETWORK=1, NODE_BLOOM=4, NODE_MASTERNODE=32, etc.)
		// Skip entries with garbage services (values > 255 are definitely invalid)
		// Also skip invalid ports (0 and 65535 indicate corrupted data)
		if services > 255 || port == 0 || port == 65535 {
			skippedCount++
			continue
		}

		addresses = append(addresses, NetAddress{
			Time:     timestamp,
			Services: ServiceFlag(services),
			IP:       net.IP(ip),
			Port:     port,
		})
	}

	if skippedCount > 0 {
		s.logger.WithFields(logrus.Fields{
			"total":   count,
			"skipped": skippedCount,
			"valid":   len(addresses),
		}).Debug("Filtered addresses with invalid services field")
	}

	return &AddrMessage{Addresses: addresses}, nil
}

// deserializeInvMessage deserializes an inv message.
// Protocol 70928+: block inv items may be 40 bytes (type + hash + height) instead of 36.
// The heights slice (parallel to InvList) is nil for legacy peers.
func (s *Server) deserializeInvMessage(payload []byte, peer ...*Peer) (*InvMessage, []uint32, error) {
	buf := bytes.NewReader(payload)

	// Read count (varint/compactSize)
	count, err := readCompactSize(buf)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read inv count: %w", err)
	}

	if count > MaxInvMessages {
		return nil, nil, fmt.Errorf("inv count too large: %d", count)
	}

	// Determine if peer supports extended inv format
	extended := len(peer) > 0 && peer[0] != nil && peer[0].SupportsProto70928()

	invList := make([]InventoryVector, count)
	var heights []uint32
	if extended {
		heights = make([]uint32, count)
	}

	for i := uint64(0); i < count; i++ {
		// Read type (4 bytes)
		var invType uint32
		if err := binary.Read(buf, binary.LittleEndian, &invType); err != nil {
			return nil, nil, fmt.Errorf("failed to read inv type: %w", err)
		}

		// Read hash (32 bytes)
		var hash types.Hash
		if _, err := buf.Read(hash[:]); err != nil {
			return nil, nil, fmt.Errorf("failed to read inv hash: %w", err)
		}

		invList[i] = InventoryVector{
			Type: InvType(invType),
			Hash: hash,
		}

		// Protocol 70928: read height for block items from extended peers
		if extended && InvType(invType) == InvTypeBlock {
			var height uint32
			if err := binary.Read(buf, binary.LittleEndian, &height); err != nil {
				return nil, nil, fmt.Errorf("failed to read inv block height: %w", err)
			}
			heights[i] = height
		}
	}

	return &InvMessage{InvList: invList}, heights, nil
}

// deserializeGetDataMessage deserializes a getdata message
func (s *Server) deserializeGetDataMessage(payload []byte) (*GetDataMessage, error) {
	buf := bytes.NewReader(payload)

	// Read count (varint/compactSize)
	count, err := readCompactSize(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read getdata count: %w", err)
	}

	if count > MaxInvMessages {
		return nil, fmt.Errorf("getdata count too large: %d", count)
	}

	invList := make([]InventoryVector, count)
	for i := uint64(0); i < count; i++ {
		// Read type (4 bytes)
		var invType uint32
		if err := binary.Read(buf, binary.LittleEndian, &invType); err != nil {
			return nil, fmt.Errorf("failed to read getdata type: %w", err)
		}

		// Read hash (32 bytes)
		var hash types.Hash
		if _, err := buf.Read(hash[:]); err != nil {
			return nil, fmt.Errorf("failed to read getdata hash: %w", err)
		}

		invList[i] = InventoryVector{
			Type: InvType(invType),
			Hash: hash,
		}
	}

	return &GetDataMessage{InvList: invList}, nil
}

// deserializeBlock deserializes a block message
func (s *Server) deserializeBlock(payload []byte) (*types.Block, error) {
	return types.DeserializeBlock(payload)
}

// deserializeTx deserializes a transaction message
func (s *Server) deserializeTx(payload []byte) (*types.Transaction, error) {
	return types.DeserializeTransaction(payload)
}

// Address relay helpers

// sendAddrMessage sends an addr message with the given addresses to a peer
func (s *Server) sendAddrMessage(peer *Peer, addresses []*NetAddress) {
	if peer == nil || !peer.IsConnected() {
		return
	}

	var buf bytes.Buffer

	// Write count using compactSize varint
	count := len(addresses)
	if count > MaxAddrMessages {
		count = MaxAddrMessages
		addresses = addresses[:count]
	}
	if err := writeCompactSize(&buf, uint64(count)); err != nil {
		s.logger.WithError(err).Warn("Failed to write compact size")
		return
	}

	// Write each address
	for _, addr := range addresses {
		// Time (4 bytes)
		binary.Write(&buf, binary.LittleEndian, addr.Time)
		// Services (8 bytes)
		binary.Write(&buf, binary.LittleEndian, uint64(addr.Services))
		// IP (16 bytes)
		buf.Write(addr.IP.To16())
		// Port (2 bytes, big endian)
		binary.Write(&buf, binary.BigEndian, addr.Port)
	}

	// Create and send addr message using NewMessage (properly sets checksum and magic)
	msg := NewMessage(MsgAddr, buf.Bytes(), s.getMagicBytes())

	if err := peer.SendMessage(msg); err != nil {
		fields := logrus.Fields{
			"peer":  peer.GetAddress().String(),
			"count": len(addresses),
		}
		// Expected race: peer disconnects between selection and SendMessage().
		if strings.Contains(err.Error(), "peer not connected") || strings.Contains(err.Error(), "peer shutting down") {
			s.logger.WithError(err).WithFields(fields).Debug("Skipped addr relay to disconnected peer")
			return
		}
		s.logger.WithError(err).WithFields(fields).Warn("Failed to send addr message")
	}
}

// relayAddresses relays addresses to up to 2 random peers (except the source).
// Legacy main.cpp:5936 uses nRelayNodes = 2 (or 1 if unreachable).
// Broadcasting to ALL peers caused addr amplification storms.
//
// Optimized: uses reservoir sampling to select 2 random peers in a single pass
// over the peer map, avoiding a separate candidates slice + shuffle. Compares
// peers by map key (string) instead of calling NetAddress.String() per iteration.
func (s *Server) relayAddresses(addresses []NetAddress, exceptPeer *Peer) {
	// Cache except-peer key once (peers map key is addr.String())
	var exceptKey string
	if exceptPeer != nil {
		exceptKey = exceptPeer.GetAddress().String()
	}

	// Reservoir sampling: pick up to 2 random peers in a single pass.
	// Fixed-size array avoids heap allocation for the candidates slice.
	const nRelayNodes = 2
	var selected [nRelayNodes]*Peer
	count := 0

	s.peers.Range(func(key, value interface{}) bool {
		peer := value.(*Peer)
		if !peer.IsConnected() || !peer.IsHandshakeComplete() {
			return true
		}
		// Compare by map key — avoids NetAddress.String() per iteration
		if key.(string) == exceptKey {
			return true
		}
		count++
		if count <= nRelayNodes {
			selected[count-1] = peer
		} else {
			j := mrand.Intn(count)
			if j < nRelayNodes {
				selected[j] = peer
			}
		}
		return true
	})

	if count == 0 {
		return
	}

	// Send to selected peers
	n := nRelayNodes
	if count < n {
		n = count
	}
	for i := 0; i < n; i++ {
		s.sendAddrMessageValues(selected[i], addresses)
	}
}

// sendAddrMessageValues sends an addr message using a value slice directly,
// avoiding the pointer-slice allocation needed by sendAddrMessage.
func (s *Server) sendAddrMessageValues(peer *Peer, addresses []NetAddress) {
	if peer == nil || !peer.IsConnected() {
		return
	}

	var buf bytes.Buffer

	count := len(addresses)
	if count > MaxAddrMessages {
		count = MaxAddrMessages
		addresses = addresses[:count]
	}
	if err := writeCompactSize(&buf, uint64(count)); err != nil {
		s.logger.WithError(err).Warn("Failed to write compact size")
		return
	}

	for i := range addresses {
		if err := binary.Write(&buf, binary.LittleEndian, addresses[i].Time); err != nil {
			return
		}
		if err := binary.Write(&buf, binary.LittleEndian, uint64(addresses[i].Services)); err != nil {
			return
		}
		if _, err := buf.Write(addresses[i].IP.To16()); err != nil {
			return
		}
		if err := binary.Write(&buf, binary.BigEndian, addresses[i].Port); err != nil {
			return
		}
	}

	msg := NewMessage(MsgAddr, buf.Bytes(), s.getMagicBytes())

	if err := peer.SendMessage(msg); err != nil {
		fields := logrus.Fields{
			"peer":  peer.GetAddress().String(),
			"count": len(addresses),
		}
		if strings.Contains(err.Error(), "peer not connected") || strings.Contains(err.Error(), "peer shutting down") {
			s.logger.WithError(err).WithFields(fields).Debug("Skipped addr relay to disconnected peer")
			return
		}
		s.logger.WithError(err).WithFields(fields).Warn("Failed to send addr message")
	}
}

// Message serialization helpers

// serializeGetDataMessage serializes a getdata message
func (s *Server) serializeGetDataMessage(msg *GetDataMessage) ([]byte, error) {
	buf := new(bytes.Buffer)

	// Write count of inventory vectors (CompactSize/varint)
	if err := writeCompactSize(buf, uint64(len(msg.InvList))); err != nil {
		return nil, err
	}

	// Write each inventory vector: {type:uint32, hash:[32]byte}
	for _, inv := range msg.InvList {
		// Write type (4 bytes, little-endian)
		if err := binary.Write(buf, binary.LittleEndian, uint32(inv.Type)); err != nil {
			return nil, err
		}

		// Write hash (32 bytes)
		if _, err := buf.Write(inv.Hash[:]); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

// handleGetBlocksMessage handles getblocks message for block synchronization
func (s *Server) handleGetBlocksMessage(peer *Peer, msg *Message) {
	s.logger.WithFields(logrus.Fields{
		"peer": peer.GetAddress().String(),
	}).Debug("Received getblocks message")

	// Parse getblocks message
	// Format: version (4 bytes) + hash count (varint) + block locator hashes + hashStop (32 bytes)
	if len(msg.Payload) < 37 { // Minimum: 4 + 1 + 32 bytes
		s.logger.WithField("peer", peer.GetAddress().String()).
			Warn("Invalid getblocks message size")
		return
	}

	// Parse protocol version
	version := binary.LittleEndian.Uint32(msg.Payload[0:4])
	s.logger.WithField("version", version).Debug("Parsed getblocks version")

	// Parse hash count (compact size / varint)
	hashCount, offset := parseCompactSize(msg.Payload[4:])
	offset += 4 // Add initial 4 bytes

	s.logger.WithField("hash_count", hashCount).Debug("Parsed block locator hash count")

	// Validate hash count (reasonable limit)
	if hashCount > MaxBlockLocatorHashes {
		s.logger.WithField("hash_count", hashCount).
			Warn("Block locator hash count too large")
		return
	}

	// Parse block locator hashes
	blockLocator := make([]types.Hash, 0, hashCount)
	for i := uint64(0); i < hashCount; i++ {
		if offset+32 > len(msg.Payload) {
			s.logger.Warn("getblocks message truncated")
			return
		}

		var hash types.Hash
		copy(hash[:], msg.Payload[offset:offset+32])
		blockLocator = append(blockLocator, hash)
		offset += 32
	}

	// Parse hashStop
	if offset+32 > len(msg.Payload) {
		s.logger.Warn("getblocks message missing hashStop")
		return
	}

	var hashStop types.Hash
	copy(hashStop[:], msg.Payload[offset:offset+32])

	s.logger.WithFields(logrus.Fields{
		"locator_hashes": len(blockLocator),
		"hash_stop":      hashStop.String(),
	}).Debug("Parsed getblocks message")

	// Rate limit: ignore repeated getblocks with same locator within 10 seconds.
	// Uses first hash from locator (or zero hash for empty locators) as dedup key.
	{
		var locatorKey types.Hash
		if len(blockLocator) > 0 {
			locatorKey = blockLocator[0]
		}
		now := time.Now().Unix()
		if prev, ok := peer.lastGetBlocksLocator.Load().(types.Hash); ok {
			if prev == locatorKey && now-peer.lastGetBlocksTime.Load() < 10 {
				s.logger.WithField("peer", peer.GetAddress().String()).
					Debug("Ignoring duplicate getblocks request")
				return
			}
		}
		peer.lastGetBlocksLocator.Store(locatorKey)
		peer.lastGetBlocksTime.Store(now)
	}

	// Find common ancestor from block locator
	if s.blockchain == nil {
		s.logger.Debug("Blockchain not available for getblocks")
		return
	}

	// Find the last known block in the locator that we have
	var startHash types.Hash
	found := false
	for _, hash := range blockLocator {
		if _, err := s.blockchain.GetBlock(hash); err == nil {
			startHash = hash
			found = true
			s.logger.WithField("start_hash", startHash.String()).Debug("Found common block")
			break
		}
	}

	if !found {
		// No common block found - peer is on different chain or we don't have their blocks
		s.logger.Debug("No common block found in locator")
		return
	}

	// Collect block hashes and heights to send (max MaxBlocksPerInventory)
	blockEntries := make([]blockInvEntry, 0, MaxBlocksPerInventory)

	// Get start block height from blockchain
	startHeight, err := s.blockchain.GetBlockHeight(startHash)
	if err != nil {
		s.logger.WithError(err).Debug("Failed to get start block height")
		return
	}
	currentHeight := startHeight + 1

	// Get best height
	bestHeight, err := s.blockchain.GetBestHeight()
	if err != nil {
		s.logger.WithError(err).Debug("Failed to get best height")
		return
	}

	// Collect hashes from startHeight+1 to hashStop or max blocks
	for currentHeight <= bestHeight && len(blockEntries) < MaxBlocksPerInventory {
		block, err := s.blockchain.GetBlockByHeight(currentHeight)
		if err != nil {
			s.logger.WithError(err).WithField("height", currentHeight).
				Debug("Failed to get block at height")
			break
		}

		blockHash := block.Hash()
		blockEntries = append(blockEntries, blockInvEntry{Hash: blockHash, Height: currentHeight})

		// Stop if we reached hashStop
		if blockHash == hashStop {
			break
		}

		currentHeight++
	}

	if len(blockEntries) == 0 {
		s.logger.Debug("No blocks to send")
		return
	}

	// Set hashContinue if we hit the MaxBlocksPerInventory limit (pipelining mechanism from legacy)
	// When peer requests this block via getdata, we'll auto-send next inv batch
	if len(blockEntries) == MaxBlocksPerInventory && currentHeight <= bestHeight {
		lastHash := blockEntries[len(blockEntries)-1].Hash
		peer.SetHashContinue(lastHash)
		s.logger.WithFields(logrus.Fields{
			"hash":   lastHash.String(),
			"height": currentHeight - 1,
		}).Debug("Set hashContinue for pipelining (legacy compatibility)")
	}

	s.logger.WithFields(logrus.Fields{
		"block_count":  len(blockEntries),
		"start_height": startHeight + 1,
		"end_height":   currentHeight - 1,
	}).Debug("Sending block inventory")

	// Build and send inv message with block hashes (70928: includes heights)
	invPayload := s.buildBlockInvForPeer(peer, blockEntries)
	invMsg := NewMessage(MsgInv, invPayload, s.getMagicBytes())

	if err := peer.SendMessage(invMsg); err != nil {
		s.logger.WithError(err).Debug("Failed to send inv message")
		return
	}

	s.logger.WithField("blocks", len(blockEntries)).Debug("Sent block inventory")
}

// buildInvMessage builds an inventory message payload (standard format, no height extension).
// Use buildBlockInvForPeer for block inv items sent to specific peers.
func (s *Server) buildInvMessage(invType InvType, hashes []types.Hash) []byte {
	buf := &bytes.Buffer{}

	// Write count (compact size)
	if err := writeCompactSize(buf, uint64(len(hashes))); err != nil {
		// On error, return empty payload
		return []byte{}
	}

	// Write inventory vectors
	for _, hash := range hashes {
		// Each inv entry: type (4 bytes) + hash (32 bytes)
		binary.Write(buf, binary.LittleEndian, uint32(invType))
		buf.Write(hash[:])
	}

	return buf.Bytes()
}

// blockInvEntry pairs a block hash with its height for extended inv serialization.
type blockInvEntry struct {
	Hash   types.Hash
	Height uint32
}

// buildBlockInvForPeer builds a block inv payload for a specific peer.
// Protocol 70928+: block items include a 4-byte height after the hash.
// Legacy peers get standard 36-byte entries (type + hash).
func (s *Server) buildBlockInvForPeer(peer *Peer, entries []blockInvEntry) []byte {
	extended := peer.SupportsProto70928()

	buf := &bytes.Buffer{}
	if err := writeCompactSize(buf, uint64(len(entries))); err != nil {
		return []byte{}
	}

	for _, entry := range entries {
		binary.Write(buf, binary.LittleEndian, uint32(InvTypeBlock))
		buf.Write(entry.Hash[:])
		if extended {
			binary.Write(buf, binary.LittleEndian, entry.Height)
		}
	}

	return buf.Bytes()
}

// parseCompactSize parses a Bitcoin compact size integer (varint)
func parseCompactSize(data []byte) (uint64, int) {
	if len(data) == 0 {
		return 0, 0
	}

	firstByte := data[0]
	if firstByte < 0xfd {
		return uint64(firstByte), 1
	}

	if firstByte == 0xfd {
		if len(data) < 3 {
			return 0, 0
		}
		return uint64(binary.LittleEndian.Uint16(data[1:3])), 3
	}

	if firstByte == 0xfe {
		if len(data) < 5 {
			return 0, 0
		}
		return uint64(binary.LittleEndian.Uint32(data[1:5])), 5
	}

	// 0xff
	if len(data) < 9 {
		return 0, 0
	}
	return binary.LittleEndian.Uint64(data[1:9]), 9
}

// Note: writeCompactSize is defined at line 737 with error handling

// handleGetHeadersMessage handles getheaders message for header-first sync
func (s *Server) handleGetHeadersMessage(peer *Peer, msg *Message) {
	s.logger.WithFields(logrus.Fields{
		"peer": peer.GetAddress().String(),
	}).Debug("Received getheaders message")

	// Parse getheaders message (same format as getblocks)
	// Format: version (4 bytes) + hash count (varint) + block locator hashes + hashStop (32 bytes)
	if len(msg.Payload) < 37 { // Minimum: 4 + 1 + 32 bytes
		s.logger.WithField("peer", peer.GetAddress().String()).
			Warn("Invalid getheaders message size")
		return
	}

	// Parse protocol version
	version := binary.LittleEndian.Uint32(msg.Payload[0:4])
	s.logger.WithField("version", version).Debug("Parsed getheaders version")

	// Parse hash count (compact size / varint)
	hashCount, offset := parseCompactSize(msg.Payload[4:])
	offset += 4 // Add initial 4 bytes

	s.logger.WithField("hash_count", hashCount).Debug("Parsed block locator hash count")

	// Validate hash count (reasonable limit)
	if hashCount > MaxBlockLocatorHashes {
		s.logger.WithField("hash_count", hashCount).
			Warn("Block locator hash count too large")
		return
	}

	// Parse block locator hashes
	blockLocator := make([]types.Hash, 0, hashCount)
	for i := uint64(0); i < hashCount; i++ {
		if offset+32 > len(msg.Payload) {
			s.logger.Warn("getheaders message truncated")
			return
		}

		var hash types.Hash
		copy(hash[:], msg.Payload[offset:offset+32])
		blockLocator = append(blockLocator, hash)
		offset += 32
	}

	// Parse hashStop
	if offset+32 > len(msg.Payload) {
		s.logger.Warn("getheaders message missing hashStop")
		return
	}

	var hashStop types.Hash
	copy(hashStop[:], msg.Payload[offset:offset+32])

	s.logger.WithFields(logrus.Fields{
		"locator_hashes": len(blockLocator),
		"hash_stop":      hashStop.String(),
	}).Debug("Parsed getheaders message")

	// Rate limit: ignore repeated getheaders with same locator within 10 seconds.
	// Uses first hash from locator (or zero hash for empty locators) as dedup key.
	{
		var locatorKey types.Hash
		if len(blockLocator) > 0 {
			locatorKey = blockLocator[0]
		}
		now := time.Now().Unix()
		if prev, ok := peer.lastGetHeadersLocator.Load().(types.Hash); ok {
			if prev == locatorKey && now-peer.lastGetHeadersTime.Load() < 10 {
				s.logger.WithField("peer", peer.GetAddress().String()).
					Debug("Ignoring duplicate getheaders request")
				return
			}
		}
		peer.lastGetHeadersLocator.Store(locatorKey)
		peer.lastGetHeadersTime.Store(now)
	}

	// Find common ancestor from block locator
	if s.blockchain == nil {
		s.logger.Debug("Blockchain not available for getheaders")
		return
	}

	// Find the last known block in the locator that we have
	var startHash types.Hash
	found := false
	for _, hash := range blockLocator {
		if _, err := s.blockchain.GetBlock(hash); err == nil {
			startHash = hash
			found = true
			s.logger.WithField("start_hash", startHash.String()).Debug("Found common block")
			break
		}
	}

	if !found {
		// No common block found - peer is on different chain or we don't have their blocks
		s.logger.Debug("No common block found in locator")
		return
	}

	// Get block height of start hash
	startBlock, err := s.blockchain.GetBlock(startHash)
	if err != nil {
		s.logger.WithError(err).Debug("Failed to get start block")
		return
	}

	// Collect block headers to send (max 2000 for headers vs 500 for blocks)
	headers := make([]*types.BlockHeader, 0, MaxHeadersPerMessage)

	// Get start block height from blockchain
	startBlockHash := startBlock.Hash()
	startHeightH, heightErrH := s.blockchain.GetBlockHeight(startBlockHash)
	if heightErrH != nil {
		s.logger.WithError(heightErrH).Debug("Failed to get start block height")
		return
	}
	currentHeight := startHeightH + 1

	// Get best height
	bestHeight, err := s.blockchain.GetBestHeight()
	if err != nil {
		s.logger.WithError(err).Debug("Failed to get best height")
		return
	}

	// Collect headers from startHeight+1 to hashStop or max headers
	for currentHeight <= bestHeight && len(headers) < MaxHeadersPerMessage {
		block, err := s.blockchain.GetBlockByHeight(currentHeight)
		if err != nil {
			s.logger.WithError(err).WithField("height", currentHeight).
				Debug("Failed to get block at height")
			break
		}

		headers = append(headers, block.Header)

		// Stop if we reached hashStop
		blockHash := block.Hash()
		if blockHash == hashStop {
			break
		}

		currentHeight++
	}

	if len(headers) == 0 {
		s.logger.Debug("No headers to send")
		return
	}

	s.logger.WithFields(logrus.Fields{
		"header_count": len(headers),
		"start_height": startHeightH + 1,
		"end_height":   currentHeight - 1,
	}).Debug("Sending block headers")

	// Build and send headers message
	headersPayload := s.buildHeadersMessage(headers)
	headersMsg := NewMessage(MsgHeaders, headersPayload, msg.Magic)

	if err := peer.SendMessage(headersMsg); err != nil {
		s.logger.WithError(err).Debug("Failed to send headers message")
		return
	}

	s.logger.WithField("headers", len(headers)).Debug("Sent block headers")
}

// buildHeadersMessage builds a headers message payload
func (s *Server) buildHeadersMessage(headers []*types.BlockHeader) []byte {
	buf := &bytes.Buffer{}

	// Write count (compact size)
	if err := writeCompactSize(buf, uint64(len(headers))); err != nil {
		// On error, return empty payload
		return []byte{}
	}

	// Write each header
	for _, header := range headers {
		// Serialize header (80 bytes for Bitcoin-style header)
		// Version (4) + PrevBlockHash (32) + MerkleRoot (32) + Timestamp (4) + Bits (4) + Nonce (4)
		binary.Write(buf, binary.LittleEndian, header.Version)
		buf.Write(header.PrevBlockHash[:])
		buf.Write(header.MerkleRoot[:])
		binary.Write(buf, binary.LittleEndian, uint32(header.Timestamp))
		binary.Write(buf, binary.LittleEndian, header.Bits)
		binary.Write(buf, binary.LittleEndian, header.Nonce)

		// Transaction count (always 0 for headers message in Bitcoin protocol)
		// This indicates it's a header without transactions
		buf.WriteByte(0)
	}

	return buf.Bytes()
}

// handleHeadersMessage handles headers message for header-first sync
func (s *Server) handleHeadersMessage(peer *Peer, msg *Message) {
	s.logger.WithFields(logrus.Fields{
		"peer": peer.GetAddress().String(),
	}).Debug("Received headers message")

	// Parse headers message
	// Format: count (varint) + headers (81 bytes each: 80 byte header + 1 byte txn count)
	if len(msg.Payload) < 1 {
		s.logger.WithField("peer", peer.GetAddress().String()).
			Warn("Invalid headers message size")
		return
	}

	// Parse header count
	headerCount, offset := parseCompactSize(msg.Payload)
	s.logger.WithField("header_count", headerCount).Debug("Parsed headers count")

	// Validate header count (reasonable limit)
	if headerCount > MaxHeadersPerMessage {
		s.logger.WithField("header_count", headerCount).
			Warn("Headers count too large")
		s.Misbehaving(peer, 20, "oversized headers message")
		return
	}

	if headerCount == 0 {
		s.logger.Debug("Received empty headers message")
		return
	}

	// Parse headers
	headers := make([]*types.BlockHeader, 0, headerCount)
	for i := uint64(0); i < headerCount; i++ {
		// Each header is 81 bytes (80 byte header + 1 byte txn count)
		if offset+81 > len(msg.Payload) {
			s.logger.Warn("Headers message truncated")
			return
		}

		header := &types.BlockHeader{}

		// Parse header fields
		header.Version = binary.LittleEndian.Uint32(msg.Payload[offset : offset+4])
		copy(header.PrevBlockHash[:], msg.Payload[offset+4:offset+36])
		copy(header.MerkleRoot[:], msg.Payload[offset+36:offset+68])
		header.Timestamp = binary.LittleEndian.Uint32(msg.Payload[offset+68 : offset+72])
		header.Bits = binary.LittleEndian.Uint32(msg.Payload[offset+72 : offset+76])
		header.Nonce = binary.LittleEndian.Uint32(msg.Payload[offset+76 : offset+80])

		// Skip txn_count byte at offset+80 (always 0x00 for headers messages per protocol spec)
		headers = append(headers, header)
		offset += 81
	}

	s.logger.WithFields(logrus.Fields{
		"headers": len(headers),
		"first":   headers[0].Hash().String(),
		"last":    headers[len(headers)-1].Hash().String(),
	}).Debug("Parsed headers")

	// Validate headers chain
	if s.blockchain == nil {
		s.logger.Debug("Blockchain not available for headers validation")
		return
	}

	// Validate headers form a continuous chain
	validHeaders := make([]*types.BlockHeader, 0, len(headers))
	var lastValidHash types.Hash

	for i, header := range headers {
		// Validate header linkage (except for first header)
		if i > 0 {
			if header.PrevBlockHash != lastValidHash {
				s.logger.WithFields(logrus.Fields{
					"index":    i,
					"expected": lastValidHash.String(),
					"got":      header.PrevBlockHash.String(),
				}).Warn("Header chain broken")
				s.Misbehaving(peer, 20, "non-continuous headers sequence")
				return
			}
		}

		// Validate proof of work
		headerHash := header.Hash()
		if !s.validateHeaderPoW(header, headerHash) {
			s.logger.WithFields(logrus.Fields{
				"index": i,
				"hash":  headerHash.String(),
			}).Warn("Invalid header PoW")
			break
		}

		// Validate timestamp (not too far in future)
		if !s.validateHeaderTimestamp(header) {
			s.logger.WithFields(logrus.Fields{
				"index":     i,
				"timestamp": header.Timestamp,
			}).Warn("Invalid header timestamp")
			break
		}

		validHeaders = append(validHeaders, header)
		lastValidHash = headerHash
	}

	if len(validHeaders) == 0 {
		s.logger.Debug("No valid headers received")
		return
	}

	s.logger.WithField("valid_headers", len(validHeaders)).Debug("Validated headers")

	// Update peer's best known block from last header (like legacy UpdateBlockAvailability)
	// This gives us the peer's actual current tip, not the static StartHeight
	if len(validHeaders) > 0 {
		lastHeader := validHeaders[len(validHeaders)-1]
		lastHeaderHash := lastHeader.Hash()

		// Use UpdateBlockAvailability to track peer's chain tip
		// This mirrors legacy: UpdateBlockAvailability(pfrom->GetId(), pindexLast->GetBlockHash())
		s.UpdateBlockAvailability(peer, lastHeaderHash)

		s.logger.WithFields(logrus.Fields{
			"peer":         peer.GetAddress().String(),
			"last_header":  lastHeaderHash.String(),
			"headers_sent": len(validHeaders),
		}).Debug("Updated peer block availability from HEADERS")
	}

	// Request full blocks for validated headers
	// Build getdata message with block inventory
	blockHashes := make([]types.Hash, 0, len(validHeaders))
	for _, header := range validHeaders {
		blockHashes = append(blockHashes, header.Hash())
	}

	// Build getdata message
	getdataPayload := s.buildInvMessage(InvTypeBlock, blockHashes)
	getdataMsg := NewMessage(MsgGetData, getdataPayload, msg.Magic)

	if err := peer.SendMessage(getdataMsg); err != nil {
		s.logger.WithError(err).Debug("Failed to send getdata message")
		return
	}

	s.logger.WithField("blocks_requested", len(blockHashes)).Debug("Requested full blocks for headers")

	// Update sync state (track that we're expecting these blocks)
	peer.SetHeadersSyncing(true)
	s.logger.Debug("Updated peer sync state for headers-first sync")
}

// validateHeaderPoW validates the proof of work for a header
func (s *Server) validateHeaderPoW(header *types.BlockHeader, headerHash types.Hash) bool {
	// Check if hash meets difficulty target specified in header.Bits
	// The hash must be less than or equal to the target

	// For PoS blocks after a certain height, PoW validation is different
	// This is a simplified check - full implementation needs consensus rules

	// Convert bits to target (compact representation)
	target := bitsToTarget(header.Bits)

	// Compare hash to target (as big integers)
	hashBigInt := hashToBigInt(headerHash)

	// Hash must be <= target
	return hashBigInt.Cmp(target) <= 0
}

// validateHeaderTimestamp validates the header timestamp
func (s *Server) validateHeaderTimestamp(header *types.BlockHeader) bool {
	// Timestamp must not be more than 2 hours in the future
	now := uint32(time.Now().Unix())

	if header.Timestamp > now+MaxClockOffsetSeconds {
		return false
	}

	// Timestamp must be positive
	if header.Timestamp <= 0 {
		return false
	}

	return true
}

// bitsToTarget converts compact bits representation to target big.Int
func bitsToTarget(bits uint32) *big.Int {
	// Compact bits format: 0xAABBCCDD
	// AA = exponent (number of bytes)
	// BBCCDD = mantissa (coefficient)

	exponent := bits >> 24
	mantissa := bits & 0x00ffffff

	var target *big.Int
	if exponent <= 3 {
		mantissa >>= 8 * (3 - exponent)
		target = big.NewInt(int64(mantissa))
	} else {
		target = big.NewInt(int64(mantissa))
		target.Lsh(target, uint(8*(exponent-3)))
	}

	return target
}

// hashToBigInt converts a hash to a big.Int for comparison
func hashToBigInt(hash types.Hash) *big.Int {
	// Reverse bytes for little-endian interpretation
	reversed := make([]byte, len(hash))
	for i := 0; i < len(hash); i++ {
		reversed[i] = hash[len(hash)-1-i]
	}
	return new(big.Int).SetBytes(reversed)
}

// =============================================================================
// Protocol 70928: getchainstate / chainstate handlers
// =============================================================================

// handleGetChainStateMessage processes a getchainstate request from a peer.
// Rate-limited to one request per 30 seconds per peer. Only accepted from 70928+ peers.
func (s *Server) handleGetChainStateMessage(peer *Peer, msg *Message) {
	// Only 70928+ peers should send getchainstate
	if !peer.SupportsProto70928() {
		s.logger.WithField("peer", peer.GetAddress().String()).
			Debug("Received getchainstate from pre-70928 peer, ignoring")
		s.Misbehaving(peer, 1, "getchainstate from pre-70928 peer")
		return
	}

	// Rate limit: 30 seconds per peer
	now := time.Now().Unix()
	last := peer.lastGetChainStateTime.Load()
	if now-last < 30 {
		s.logger.WithField("peer", peer.GetAddress().String()).
			Debug("Rate-limiting getchainstate request")
		return
	}
	peer.lastGetChainStateTime.Store(now)

	// Get or build cached chain state
	cs, err := s.getOrBuildChainState()
	if err != nil {
		s.logger.WithError(err).Error("Failed to build chainstate response")
		return
	}

	// Serialize and send
	payload, err := SerializeChainStateMessage(cs)
	if err != nil {
		s.logger.WithError(err).Error("Failed to serialize chainstate response")
		return
	}

	csMsg := NewMessage(MsgChainState, payload, s.getMagicBytes())
	if err := peer.SendMessage(csMsg); err != nil {
		s.logger.WithError(err).WithField("peer", peer.GetAddress().String()).
			Error("Failed to send chainstate message")
		return
	}

	s.logger.WithFields(logrus.Fields{
		"peer":       peer.GetAddress().String(),
		"tip_height": cs.TipHeight,
		"locator":    len(cs.Locator),
	}).Debug("Sent chainstate response")
}

// handleChainStateMessage processes a chainstate response from a peer.
func (s *Server) handleChainStateMessage(peer *Peer, msg *Message) {
	cs, err := DeserializeChainStateMessage(msg.Payload)
	if err != nil {
		s.logger.WithError(err).WithField("peer", peer.GetAddress().String()).
			Error("Failed to deserialize chainstate message")
		return
	}

	// Update peer's known height
	peer.SetPeerHeight(cs.TipHeight)
	if s.healthTracker != nil {
		s.healthTracker.UpdateBestKnownHeight(peer.GetAddress().String(), cs.TipHeight)
	}

	// Deliver to the peer's chainstate channel for any waiting requester
	select {
	case peer.chainStateCh <- cs:
	default:
		// Channel full or no one waiting - that's fine
	}

	s.logger.WithFields(logrus.Fields{
		"peer":       peer.GetAddress().String(),
		"tip_height": cs.TipHeight,
		"tip_hash":   cs.TipHash.String()[:16],
		"locator":    len(cs.Locator),
	}).Debug("Received chainstate response")
}
