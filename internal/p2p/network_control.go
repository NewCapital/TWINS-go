// Copyright (c) 2025 The TWINS Core developers
// Distributed under the MIT software license

package p2p

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/twins-dev/twins-core/pkg/types"
)

// Network control data structures
type (
	// addedNode represents a manually added node
	addedNode struct {
		addr      string
		permanent bool
	}

	// bannedSubnet represents a banned IP subnet
	bannedSubnet struct {
		subnet      *net.IPNet
		bannedUntil int64
		banCreated  int64
		reason      string
	}
)

var (
	// Global network control state
	addedNodes      = make(map[string]*addedNode)
	addedNodesMu    sync.RWMutex
	bannedSubnets   = make(map[string]*bannedSubnet)
	bannedSubnetsMu sync.RWMutex
	networkActive   = true
	networkActiveMu sync.RWMutex
)

// GetPeersList returns actual Peer objects (for internal use)
func (s *Server) GetPeersList() []*Peer {
	var peers []*Peer
	s.peers.Range(func(key, value interface{}) bool {
		if peer, ok := value.(*Peer); ok {
			peers = append(peers, peer)
		}
		return true
	})
	return peers
}

// GetPeers returns information about all connected peers
func (s *Server) GetPeers() []PeerInfo {
	peers := make([]PeerInfo, 0)

	peerID := 0
	s.peers.Range(func(key, value interface{}) bool {
		peer := value.(*Peer)
		addr := peer.GetAddress()

		// Get protocol version from peer's version message.
		// If version handshake did not complete yet, keep zero value.
		var protocolVersion int32
		if version := peer.GetVersion(); version != nil {
			protocolVersion = version.Version
		}

		// Get time offset (calculated at handshake, stored in peer)
		timeOffset := peer.GetTimeOffset()

		// Calculate ping time (round-trip time in milliseconds)
		var pingTime float64
		var pingWait float64
		lastPing := peer.lastPing.Load()
		lastPong := peer.lastPong.Load()
		if lastPing > 0 && lastPong > 0 && lastPong >= lastPing {
			pingTime = float64(lastPong-lastPing) / 1e6 // Convert nanoseconds to milliseconds
		}
		// Calculate ping wait time if we have an outstanding ping
		if lastPing > 0 && (lastPong == 0 || lastPing > lastPong) {
			pingWait = float64(time.Now().UnixNano()-lastPing) / 1e6 // Convert nanoseconds to milliseconds
		}

		// Get local address from connection
		var addrLocal string
		if peer.conn != nil {
			if localAddr := peer.conn.LocalAddr(); localAddr != nil {
				addrLocal = localAddr.String()
			}
		}

		peerInfo := PeerInfo{
			ID:              peerID,
			Address:         addr.String(),
			AddrLocal:       addrLocal,
			Services:        uint64(peer.services),
			LastSend:        time.Unix(peer.lastSendTime.Load(), 0),
			LastRecv:        time.Unix(peer.lastMessageTime.Load(), 0),
			BytesSent:       peer.bytesSent.Load(),
			BytesReceived:   peer.bytesReceived.Load(),
			TimeConnected:   peer.timeConnected,
			TimeOffset:      timeOffset,
			PingTime:        pingTime,
			PingWait:        pingWait,
			ProtocolVersion: protocolVersion,
			UserAgent:       peer.userAgent,
			Inbound:         peer.inbound,
			StartHeight:     peer.startHeight,
			BanScore:        int(peer.GetMisbehaviorScore()),
		}

		// Populate effective height from hybrid model (ping/inv/StartHeight)
		peerInfo.SyncedHeight = int32(peer.EffectivePeerHeight())

		// Get sync state from health tracker (for synced_headers, synced_blocks, inflight)
		if s.syncer != nil {
			if healthTracker := s.syncer.GetHealthTracker(); healthTracker != nil {
				if stats := healthTracker.GetStats(addr.String()); stats != nil {
					peerInfo.SyncedHeaders = int32(stats.BestKnownHeight)
					peerInfo.SyncedBlocks = int32(stats.CommonHeight)
					peerInfo.LastHeaderUpdateTime = stats.LastHeaderUpdateTime

					// Copy InFlight if present
					if len(stats.InFlight) > 0 {
						peerInfo.Inflight = make([]int32, len(stats.InFlight))
						for i, h := range stats.InFlight {
							peerInfo.Inflight[i] = int32(h)
						}
					}
				}
			}
		}

		peers = append(peers, peerInfo)
		peerID++
		return true
	})

	return peers
}

// PingAllPeers queues a ping message for all connected peers
func (s *Server) PingAllPeers() {
	s.peers.Range(func(key, value interface{}) bool {
		peer := value.(*Peer)
		// Send ping to peer (uses peer's sendPing method)
		if peer.IsConnected() && peer.IsHandshakeComplete() {
			if err := peer.sendPing(); err != nil {
				s.logger.WithError(err).WithField("peer", peer.GetAddress().String()).
					Warn("Failed to send ping to peer")
			} else {
				s.logger.WithField("peer", peer.GetAddress().String()).Debug("Sent ping to peer")
			}
		}
		return true
	})
}

// AddNode adds a node to the addnode list
func (s *Server) AddNode(addr string, permanent bool) error {
	addedNodesMu.Lock()
	defer addedNodesMu.Unlock()

	if _, exists := addedNodes[addr]; exists {
		return errors.New("node already added")
	}

	addedNodes[addr] = &addedNode{
		addr:      addr,
		permanent: permanent,
	}

	// Trigger connection if permanent
	if permanent {
		go s.connectToNode(addr)
	}

	return nil
}

// RemoveNode removes a node from the addnode list
func (s *Server) RemoveNode(addr string) error {
	addedNodesMu.Lock()
	defer addedNodesMu.Unlock()

	if _, exists := addedNodes[addr]; !exists {
		return errors.New("node not found in added nodes list")
	}

	delete(addedNodes, addr)
	return nil
}

// ConnectNode attempts a one-time connection to a node
func (s *Server) ConnectNode(addr string) error {
	// Ensure proper address format
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		// No port specified, use default
		addr = net.JoinHostPort(addr, fmt.Sprintf("%d", s.params.DefaultPort))
	} else {
		// Reconstruct with validated parts
		addr = net.JoinHostPort(host, port)
	}

	// Attempt connection
	go s.connectToNode(addr)
	return nil
}

// DisconnectNode disconnects from a specific node
func (s *Server) DisconnectNode(addr string) error {
	var foundPeer *Peer

	s.peers.Range(func(key, value interface{}) bool {
		peer := value.(*Peer)
		peerAddr := peer.GetAddress().String()

		// Check if address matches (flexible matching)
		if peerAddr == addr || key.(string) == addr {
			foundPeer = peer
			return false // Stop iteration
		}
		return true
	})

	if foundPeer == nil {
		return errors.New("node not found in connected peers")
	}

	// Disconnect peer
	foundPeer.Stop()
	return nil
}

// GetAddedNodes returns the list of manually added nodes
func (s *Server) GetAddedNodes() []string {
	addedNodesMu.RLock()
	defer addedNodesMu.RUnlock()

	nodes := make([]string, 0, len(addedNodes))
	for addr := range addedNodes {
		nodes = append(nodes, addr)
	}
	return nodes
}

// BanSubnet bans an IP subnet with the given reason.
func (s *Server) BanSubnet(subnet string, banTime int64, absolute bool, reason string) error {
	_, ipNet, err := net.ParseCIDR(subnet)
	if err != nil {
		return fmt.Errorf("invalid subnet: %w", err)
	}

	bannedSubnetsMu.Lock()
	defer bannedSubnetsMu.Unlock()

	if _, exists := bannedSubnets[subnet]; exists {
		return errors.New("subnet already banned")
	}

	now := time.Now().Unix()
	var bannedUntil int64

	if absolute {
		bannedUntil = banTime
	} else {
		if banTime == 0 {
			banTime = 24 * 60 * 60 // Default 24 hours
		}
		bannedUntil = now + banTime
	}

	bannedSubnets[subnet] = &bannedSubnet{
		subnet:      ipNet,
		bannedUntil: bannedUntil,
		banCreated:  now,
		reason:      reason,
	}

	// Disconnect any connected peers in this subnet
	s.peers.Range(func(key, value interface{}) bool {
		peer := value.(*Peer)
		peerAddr := peer.GetAddress()

		if ipNet.Contains(peerAddr.IP) {
			s.logger.WithField("peer", peerAddr.String()).Info("Disconnecting peer in banned subnet")
			peer.Stop()
		}
		return true
	})

	return nil
}

// UnbanSubnet removes a subnet from the ban list
func (s *Server) UnbanSubnet(subnet string) error {
	bannedSubnetsMu.Lock()
	defer bannedSubnetsMu.Unlock()

	if _, exists := bannedSubnets[subnet]; !exists {
		return errors.New("subnet not found in ban list")
	}

	delete(bannedSubnets, subnet)
	return nil
}

// GetBannedList returns the list of banned subnets
func (s *Server) GetBannedList() []BanInfo {
	bannedSubnetsMu.RLock()
	defer bannedSubnetsMu.RUnlock()

	now := time.Now().Unix()
	bans := make([]BanInfo, 0, len(bannedSubnets))

	for subnet, ban := range bannedSubnets {
		// Skip expired bans
		if ban.bannedUntil < now {
			continue
		}

		bans = append(bans, BanInfo{
			Subnet:      subnet,
			BannedUntil: ban.bannedUntil,
			BanCreated:  ban.banCreated,
			Reason:      ban.reason,
		})
	}

	return bans
}

// ClearBannedList clears all banned subnets
func (s *Server) ClearBannedList() {
	bannedSubnetsMu.Lock()
	defer bannedSubnetsMu.Unlock()

	bannedSubnets = make(map[string]*bannedSubnet)
	s.logger.Info("Cleared all banned subnets")
}

// SetNetworkActive enables or disables network activity
func (s *Server) SetNetworkActive(active bool) {
	networkActiveMu.Lock()
	defer networkActiveMu.Unlock()

	networkActive = active
	s.logger.WithField("active", active).Info("Network active state changed")

	if !active {
		// Disconnect all peers when disabling network
		s.peers.Range(func(key, value interface{}) bool {
			peer := value.(*Peer)
			peer.Stop()
			return true
		})
	}
}

// IsNetworkActive returns whether network is active
func (s *Server) IsNetworkActive() bool {
	networkActiveMu.RLock()
	defer networkActiveMu.RUnlock()
	return networkActive
}

// RelayBlock broadcasts a block to all connected peers.
// Used by staking and submitblock RPC to propagate locally produced blocks.
// Also updates the syncer's height so getsyncstatus reflects locally produced blocks.
func (s *Server) RelayBlock(block *types.Block) {
	s.relayBlock(block, nil)

	// Update syncer height for locally produced blocks (staking, submitblock).
	// Without this, syncer.bestHeight only advances from peer-synced blocks,
	// causing getsyncstatus to report stale current_height.
	if s.syncer != nil && s.blockchain != nil {
		if height, err := s.blockchain.GetBestHeight(); err == nil {
			s.syncer.UpdateLocalHeight(height)
		}
	}
}

// IsBanned checks if an IP address is banned
func (s *Server) IsBanned(ip net.IP) bool {
	bannedSubnetsMu.RLock()
	defer bannedSubnetsMu.RUnlock()

	now := time.Now().Unix()

	for _, ban := range bannedSubnets {
		// Skip expired bans
		if ban.bannedUntil < now {
			continue
		}

		if ban.subnet.Contains(ip) {
			return true
		}
	}

	return false
}

// connectToNode attempts to connect to a specific node
func (s *Server) connectToNode(addr string) {
	// Check if network is active
	if !s.IsNetworkActive() {
		s.logger.Debug("Network not active, skipping connection")
		return
	}

	// Check connection limits (use config-derived cap, not hardcoded constant)
	if s.outbounds.Load() >= s.maxOutbound {
		s.logger.Debug("Outbound connection limit reached")
		return
	}

	s.logger.WithField("addr", addr).Debug("Attempting to connect to node")

	// Parse address to NetAddress for discovery tracking
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		s.logger.WithError(err).WithField("addr", addr).Warn("Failed to parse address")
		return
	}

	ip := net.ParseIP(host)
	if ip == nil {
		// Try resolving if not an IP
		ips, err := net.LookupIP(host)
		if err != nil || len(ips) == 0 {
			s.logger.WithError(err).WithField("addr", addr).Warn("Failed to resolve address")
			return
		}
		ip = ips[0]
	}

	port := 0
	if p, err := net.ResolveTCPAddr("tcp", net.JoinHostPort("", portStr)); err == nil {
		port = p.Port
	}

	netAddr := &NetAddress{
		Time:     uint32(time.Now().Unix()),
		Services: SFNodeNetwork,
		IP:       ip,
		Port:     uint16(port),
	}

	// Skip banned peers before dialing
	if s.IsBanned(ip) {
		s.logger.WithField("addr", addr).Debug("Skipping connection to banned peer")
		return
	}

	// Mark connection attempt in discovery system
	if s.discovery != nil {
		s.discovery.MarkAttempt(netAddr)
	}

	// Dial with timeout - use proxy if configured (timeout from config.Network.Timeout)
	dialTimeout := s.getDialTimeout()
	var conn net.Conn
	if IsOnionAddress(addr) && s.onionProxyDialer != nil {
		// Use onion proxy for .onion addresses
		conn, err = s.onionProxyDialer.DialTimeout("tcp", addr, dialTimeout)
	} else if s.proxyDialer != nil {
		// Use main proxy for regular addresses
		conn, err = s.proxyDialer.DialTimeout("tcp", addr, dialTimeout)
	} else {
		// Direct connection
		conn, err = net.DialTimeout("tcp", addr, dialTimeout)
	}
	if err != nil {
		s.logger.WithError(err).WithField("addr", addr).Warn("Failed to connect to node")

		// Mark as failed (respects threshold, not immediate ban)
		if s.discovery != nil {
			s.discovery.MarkFailure(netAddr, fmt.Sprintf("connection failed: %v", err))
		}
		return
	}

	// Apply socket buffer options (legacy: -maxreceivebuffer, -maxsendbuffer)
	if s.socketOpts != nil {
		ApplySocketOptions(conn, s.socketOpts, s.logger)
	}

	// Create peer
	peer := NewPeer(conn, false, s.params.NetMagicBytes, s.logger.Logger)
	peer.server = s
	s.applyConfigToPeer(peer)
	s.totalConnections.Add(1)

	s.logger.WithFields(map[string]interface{}{
		"remote_addr": addr,
		"inbound":     false,
	}).Debug("New outbound connection")

	// Add to peer management
	select {
	case s.newPeers <- peer:
	case <-s.quit:
		peer.Stop()
		return
	}

	// Start peer processing
	peer.Start(s)
}

// SetPeerAlias sets a friendly alias for a peer address
func (s *Server) SetPeerAlias(addr string, alias string) error {
	if s.discovery == nil {
		return errors.New("peer discovery not initialized")
	}
	return s.discovery.SetPeerAlias(addr, alias)
}

// RemovePeerAlias removes the alias for a peer address
func (s *Server) RemovePeerAlias(addr string) error {
	if s.discovery == nil {
		return errors.New("peer discovery not initialized")
	}
	return s.discovery.RemovePeerAlias(addr)
}

// GetPeerAliases returns all peer aliases
func (s *Server) GetPeerAliases() map[string]string {
	if s.discovery == nil {
		return nil
	}
	return s.discovery.GetPeerAliases()
}

// generateNonce generates a random nonce for ping messages
func generateNonce() uint64 {
	return uint64(time.Now().UnixNano())
}
