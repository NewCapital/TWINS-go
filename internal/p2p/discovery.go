package p2p

import (
	"context"
	"math"
	"math/rand"
	"net"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// DiscoveryConfig contains configuration for peer discovery.
// Using a config struct instead of many parameters improves readability
// and makes it easier to add new options without changing signatures.
type DiscoveryConfig struct {
	Logger         *logrus.Logger
	Network        string   // Network name (mainnet, testnet, regtest)
	Seeds          []string // Static seed addresses
	DNSSeeds       []string // DNS seed hostnames
	MaxPeers       int      // Maximum number of peers to track
	DataDir        string   // Directory for persistent address storage
	DNSSeedEnabled bool     // Whether DNS seed queries are enabled
}

// PeerDiscovery manages peer discovery and address management
type PeerDiscovery struct {
	// Configuration
	logger         *logrus.Entry
	network        string   // Network name (mainnet, testnet, regtest)
	maxPeers       int
	seeds          []string
	dnsSeeds       []string
	dnsSeedEnabled bool     // Whether DNS seed queries are enabled (legacy: -dnsseed)

	// Address management
	addrMu    sync.RWMutex
	addresses map[string]*KnownAddress // Known peer addresses
	tried     map[string]*KnownAddress // Addresses we've tried
	new       map[string]*KnownAddress // New addresses we haven't tried

	// Persistence
	addrDB *AddressDB

	// Statistics
	totalRequests     atomic.Uint64
	successfulDNS     atomic.Uint64
	failedDNS         atomic.Uint64
	peersDiscovered   atomic.Uint64
	lastSuccessfulDNS atomic.Int64 // Unix timestamp of last successful DNS query

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	quit   chan struct{}

	// Event handlers
	onNewPeer        func(addr *NetAddress)
	requestAddresses func() // Callback to request addresses from connected peers
}

// KnownAddress represents a known peer address with metadata
type KnownAddress struct {
	Addr        *NetAddress
	Source      *NetAddress // Where we learned about this address
	Attempts    int32       // Number of connection attempts
	LastAttempt time.Time   // Last connection attempt
	LastSuccess time.Time   // Last successful connection
	LastSeen    time.Time   // Last time we saw this address
	Services    ServiceFlag // Services provided by this peer
	IsBad       bool        // Whether this address is marked as bad
	Permanent   bool        // Whether this is a permanent peer
}

// Clone creates a deep copy of KnownAddress
func (ka *KnownAddress) Clone() *KnownAddress {
	if ka == nil {
		return nil
	}

	clone := &KnownAddress{
		Attempts:    ka.Attempts,
		LastAttempt: ka.LastAttempt,
		LastSuccess: ka.LastSuccess,
		LastSeen:    ka.LastSeen,
		Services:    ka.Services,
		IsBad:       ka.IsBad,
		Permanent:   ka.Permanent,
	}

	// Deep copy Addr
	if ka.Addr != nil {
		clone.Addr = &NetAddress{
			Time:     ka.Addr.Time,
			Services: ka.Addr.Services,
			IP:       make(net.IP, len(ka.Addr.IP)),
			Port:     ka.Addr.Port,
		}
		copy(clone.Addr.IP, ka.Addr.IP)
	}

	// Deep copy Source
	if ka.Source != nil {
		clone.Source = &NetAddress{
			Time:     ka.Source.Time,
			Services: ka.Source.Services,
			IP:       make(net.IP, len(ka.Source.IP)),
			Port:     ka.Source.Port,
		}
		copy(clone.Source.IP, ka.Source.IP)
	}

	return clone
}

// AddressSource indicates where an address was discovered
type AddressSource int

const (
	SourceSeed AddressSource = iota
	SourceDNS
	SourcePeer
	SourceConfig
	SourceMasternodeCache
)

// Discovery constants
const (
	MaxAddresses         = 20000              // Maximum addresses to store
	MaxTriedAddresses    = 5000               // Maximum tried addresses to store
	MaxNewAddresses      = 15000              // Maximum new addresses to store
	AddressTimeout       = 7 * 24 * time.Hour // Remove addresses after 7 days
	DNSQueryInterval     = 5 * time.Minute    // Interval for DNS seed queries
	AddressCleanup       = 30 * time.Minute   // Cleanup interval
	MaxGetAddrRequests   = 10                 // Maximum concurrent getaddr requests
	FailureThreshold     = 3                  // Mark bad after this many consecutive failures
	FailureDecayDuration = 24 * time.Hour     // Reset failure count after this duration
)

// NewPeerDiscovery creates a new peer discovery instance using DiscoveryConfig.
func NewPeerDiscovery(cfg DiscoveryConfig) *PeerDiscovery {
	ctx, cancel := context.WithCancel(context.Background())

	pd := &PeerDiscovery{
		logger:         cfg.Logger.WithField("component", "peer-discovery"),
		network:        cfg.Network,
		maxPeers:       cfg.MaxPeers,
		seeds:          cfg.Seeds,
		dnsSeeds:       cfg.DNSSeeds,
		dnsSeedEnabled: cfg.DNSSeedEnabled,
		addresses:      make(map[string]*KnownAddress),
		tried:          make(map[string]*KnownAddress),
		new:            make(map[string]*KnownAddress),
		addrDB:         NewAddressDB(cfg.DataDir, cfg.Logger),
		ctx:            ctx,
		cancel:         cancel,
		quit:           make(chan struct{}),
	}

	// Load persisted addresses on startup
	if loadedAddrs, err := pd.addrDB.Load(); err == nil {
		pd.addrMu.Lock()
		for addrStr, known := range loadedAddrs {
			pd.addresses[addrStr] = known
			if known.LastSuccess.IsZero() {
				pd.new[addrStr] = known
			} else {
				pd.tried[addrStr] = known
			}
		}
		pd.addrMu.Unlock()
		pd.logger.WithField("loaded", len(loadedAddrs)).Debug("Loaded addresses from disk")
	} else {
		pd.logger.WithError(err).Warn("Failed to load addresses from disk")
	}

	return pd
}

// Start begins the peer discovery process
func (pd *PeerDiscovery) Start() {
	pd.logger.Info("Starting peer discovery")

	// Load seed addresses
	pd.loadSeedAddresses()

	// Start background routines
	pd.wg.Add(5)
	go pd.dnsDiscoveryLoop()
	go pd.addressCleanupLoop()
	go pd.addressRequestLoop()
	go pd.addressPersistenceLoop()
	go pd.emergencyBootstrapLoop()
	pd.logger.WithFields(logrus.Fields{
		"seeds":     len(pd.seeds),
		"dns_seeds": len(pd.dnsSeeds),
		"max_peers": pd.maxPeers,
	}).Info("Peer discovery started")
}

// Stop stops the peer discovery
func (pd *PeerDiscovery) Stop() {
	pd.logger.Info("Stopping peer discovery")

	pd.cancel()
	close(pd.quit)

	// Wait for goroutines
	done := make(chan struct{})
	go func() {
		pd.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		pd.logger.Info("Peer discovery stopped")
	case <-time.After(10 * time.Second):
		pd.logger.Warn("Peer discovery stop timeout")
	}

	// Final save on shutdown with filtering
	addresses := pd.prepareAddressesForSave()
	if err := pd.addrDB.Save(addresses); err != nil {
		pd.logger.WithError(err).Error("Failed to save addresses on shutdown")
	}
}

// SetPeerAlias sets a friendly alias for a peer address
func (pd *PeerDiscovery) SetPeerAlias(addr string, alias string) error {
	return pd.addrDB.SetAlias(addr, alias)
}

// RemovePeerAlias removes the alias for a peer address
func (pd *PeerDiscovery) RemovePeerAlias(addr string) error {
	return pd.addrDB.RemoveAlias(addr)
}

// GetPeerAliases returns all peer aliases
func (pd *PeerDiscovery) GetPeerAliases() map[string]string {
	return pd.addrDB.GetAliases()
}

// loadSeedAddresses loads initial seed addresses
func (pd *PeerDiscovery) loadSeedAddresses() {
	for _, seed := range pd.seeds {
		host, port, err := net.SplitHostPort(seed)
		if err != nil {
			pd.logger.WithError(err).WithField("seed", seed).
				Warn("Invalid seed address format")
			continue
		}

		// Resolve IP addresses
		ips, err := net.LookupIP(host)
		if err != nil {
			pd.logger.WithError(err).WithField("seed", seed).
				Warn("Failed to resolve seed address")
			continue
		}

		for _, ip := range ips {
			// Parse port - use TWINS network-specific default
			portInt := pd.getDefaultPort() // Default port for this network
			if port != "" {
				if p, err := net.ResolveTCPAddr("tcp", net.JoinHostPort("", port)); err == nil {
					portInt = p.Port
				}
			}

			addr := &NetAddress{
				Time:     uint32(time.Now().Unix()),
				Services: SFNodeNetwork,
				IP:       ip,
				Port:     uint16(portInt),
			}

			known := &KnownAddress{
				Addr:      addr,
				LastSeen:  time.Now(),
				Services:  SFNodeNetwork,
				Permanent: true, // Seed addresses are permanent
			}

			pd.AddAddress(known, nil, SourceSeed)
		}
	}

	pd.logger.WithField("loaded", len(pd.addresses)).
		Info("Loaded seed addresses")
}

// dnsDiscoveryLoop performs DNS seed discovery
func (pd *PeerDiscovery) dnsDiscoveryLoop() {
	defer pd.wg.Done()

	// Check if DNS seed queries are disabled (legacy: -dnsseed=0)
	if !pd.dnsSeedEnabled {
		pd.logger.Debug("DNS seed queries disabled by configuration")
		return
	}

	if len(pd.dnsSeeds) == 0 {
		return
	}

	ticker := time.NewTicker(DNSQueryInterval)
	defer ticker.Stop()

	// Initial DNS query
	pd.queryDNSSeeds()

	for {
		select {
		case <-ticker.C:
			pd.queryDNSSeeds()

		case <-pd.quit:
			return
		}
	}
}

// queryDNSSeeds queries DNS seeds for peer addresses
func (pd *PeerDiscovery) queryDNSSeeds() {
	pd.totalRequests.Add(1)

	for _, dnsSeed := range pd.dnsSeeds {
		go func(seed string) {
			pd.logger.WithField("dns_seed", seed).Debug("Querying DNS seed")

			ips, err := net.LookupIP(seed)
			if err != nil {
				pd.failedDNS.Add(1)
				pd.logger.WithError(err).WithField("dns_seed", seed).
					Debug("DNS seed query failed")
				return
			}

			pd.successfulDNS.Add(1)
			pd.lastSuccessfulDNS.Store(time.Now().Unix())

			discovered := 0

			for _, ip := range ips {
				// Default port for TWINS network
				addr := &NetAddress{
					Time:     uint32(time.Now().Unix()),
					Services: SFNodeNetwork,
					IP:       ip,
					Port:     uint16(pd.getDefaultPort()), // Network-specific default port
				}

				known := &KnownAddress{
					Addr:     addr,
					LastSeen: time.Now(),
					Services: SFNodeNetwork,
				}

				if pd.AddAddress(known, nil, SourceDNS) {
					discovered++
				}
			}

			if discovered > 0 {
				pd.peersDiscovered.Add(uint64(discovered))
				pd.logger.WithFields(logrus.Fields{
					"dns_seed":   seed,
					"discovered": discovered,
					"total_ips":  len(ips),
				}).Debug("DNS seed discovery completed")
			}
		}(dnsSeed)
	}
}

// addressCleanupLoop periodically cleans up old addresses
func (pd *PeerDiscovery) addressCleanupLoop() {
	defer pd.wg.Done()

	ticker := time.NewTicker(AddressCleanup)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pd.cleanupAddresses()

		case <-pd.quit:
			return
		}
	}
}


// cleanupAddresses removes old and bad addresses
func (pd *PeerDiscovery) cleanupAddresses() {
	pd.addrMu.Lock()
	defer pd.addrMu.Unlock()

	now := time.Now()
	removedCount := 0
	removedByReason := make(map[string]int)

	// Clean up addresses
	for addrStr, known := range pd.addresses {
		shouldRemove := false
		reason := ""

		// Remove addresses that are too old
		if now.Sub(known.LastSeen) > AddressTimeout && !known.Permanent {
			shouldRemove = true
			reason = "stale"
		}

		// Remove addresses with too many failed attempts
		if known.Attempts > 10 && known.LastSuccess.IsZero() && !known.Permanent {
			shouldRemove = true
			reason = "too_many_failures"
		}

		// Remove bad addresses
		if known.IsBad && !known.Permanent {
			shouldRemove = true
			reason = "marked_bad"
		}

		if shouldRemove {
			delete(pd.addresses, addrStr)
			delete(pd.tried, addrStr)
			delete(pd.new, addrStr)
			removedCount++
			removedByReason[reason]++
		}
	}

	// If we have too many addresses, remove some older ones
	if len(pd.addresses) > MaxAddresses {
		excess := len(pd.addresses) - MaxAddresses
		pd.removeExcessAddresses(excess)
		removedCount += excess
		removedByReason["excess"] += excess
	}

	if removedCount > 0 {
		pd.logger.WithFields(logrus.Fields{
			"removed": removedCount,
			"total":   len(pd.addresses),
			"tried":   len(pd.tried),
			"new":     len(pd.new),
		}).Debug("Cleaned up addresses")
	}

}

// removeExcessAddresses removes excess addresses, prioritizing older and less successful ones
func (pd *PeerDiscovery) removeExcessAddresses(count int) {
	// Create a slice of addresses sorted by priority (lower priority = more likely to remove)
	type addressPriority struct {
		addr     string
		priority float64
	}

	var addresses []addressPriority

	for addrStr, known := range pd.addresses {
		if known.Permanent {
			continue // Never remove permanent addresses
		}

		priority := pd.calculateAddressPriority(known)
		addresses = append(addresses, addressPriority{
			addr:     addrStr,
			priority: priority,
		})
	}

	// Sort by priority (ascending, so lowest priority first)
	sort.Slice(addresses, func(i, j int) bool {
		return addresses[i].priority < addresses[j].priority
	})

	// Remove the lowest priority addresses
	removed := 0
	for _, ap := range addresses {
		if removed >= count {
			break
		}

		delete(pd.addresses, ap.addr)
		delete(pd.tried, ap.addr)
		delete(pd.new, ap.addr)
		removed++
	}
}

// calculateAddressPriority calculates priority for address retention
func (pd *PeerDiscovery) calculateAddressPriority(known *KnownAddress) float64 {
	priority := 1.0

	// Boost priority for recent successful connections
	if !known.LastSuccess.IsZero() {
		timeSinceSuccess := time.Since(known.LastSuccess).Hours()
		priority += 100.0 / (1.0 + timeSinceSuccess)
	}

	// Boost priority for recently seen addresses
	timeSinceSeen := time.Since(known.LastSeen).Hours()
	priority += 10.0 / (1.0 + timeSinceSeen)

	// Reduce priority for addresses with many failed attempts
	priority -= float64(known.Attempts) * 0.5

	// Boost priority for addresses that provide useful services
	if known.Services&SFNodeNetwork != 0 {
		priority += 5.0
	}
	// Significant boost for masternodes - they are preferred for syncing
	// Masternodes have better uptime, more resources, and faster responses
	if known.Services&SFNodeMasternode != 0 {
		priority += 50.0 // Increased from 10.0 to 50.0 for much higher selection chance
	}

	return priority
}

// calculateChance returns selection probability (0.0-1.0) for an address
// Matches legacy CAddrMan::GetChance() behavior with masternode boost
func (pd *PeerDiscovery) calculateChance(known *KnownAddress) float64 {
	now := time.Now()
	chance := 1.0

	// Time since last attempt
	timeSinceAttempt := now.Sub(known.LastAttempt)

	// Deprioritize very recent attempts (within 10 minutes)
	if timeSinceAttempt < 10*time.Minute {
		chance *= 0.01
	}

	// Exponential penalty for failures (matches legacy)
	// chance *= pow(0.66, min(attempts, 8))
	attempts := int(known.Attempts)
	if attempts > 8 {
		attempts = 8 // Cap at 8 for penalty calculation
	}
	chance *= math.Pow(0.66, float64(attempts))

	// Boost for successful connections
	if !known.LastSuccess.IsZero() {
		hoursSinceSuccess := now.Sub(known.LastSuccess).Hours()
		// Gradually decay the success boost over time
		successBoost := 100.0 / (1.0 + hoursSinceSuccess)
		chance *= (1.0 + successBoost/100.0)
	}

	// Significant boost for masternodes (5x more likely to be selected)
	// Masternodes are more reliable for syncing and have better uptime
	if known.Services&SFNodeMasternode != 0 {
		chance *= 5.0
	}

	return chance
}

// selectWeighted performs weighted random selection from address pool
// Matches legacy CAddrMan::Select_() behavior
func (pd *PeerDiscovery) selectWeighted(pool map[string]*KnownAddress) *KnownAddress {
	const maxIterations = 100

	// Convert map to slice for random access
	poolSlice := make([]*KnownAddress, 0, len(pool))
	for _, known := range pool {
		poolSlice = append(poolSlice, known)
	}

	if len(poolSlice) == 0 {
		return nil
	}

	// Try weighted selection with rejection sampling
	for i := 0; i < maxIterations; i++ {
		// Select random address
		idx := rand.Intn(len(poolSlice))
		candidate := poolSlice[idx]

		// Calculate acceptance probability
		chance := pd.calculateChance(candidate)

		// Accept with probability = chance
		if rand.Float64() < chance {
			return candidate
		}
	}

	// Fallback to random if weighted selection fails
	if len(poolSlice) > 0 {
		return poolSlice[rand.Intn(len(poolSlice))]
	}

	return nil
}

// addressRequestLoop handles periodic address requests
func (pd *PeerDiscovery) addressRequestLoop() {
	defer pd.wg.Done()

	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Request addresses from connected peers if we need more
			if pd.needsMoreAddresses() && pd.requestAddresses != nil {
				pd.logger.Debug("Requesting addresses from connected peers")
				pd.requestAddresses()
			}

		case <-pd.quit:
			return
		}
	}
}

// needsMoreAddresses checks if we need to request more addresses
func (pd *PeerDiscovery) needsMoreAddresses() bool {
	pd.addrMu.RLock()
	defer pd.addrMu.RUnlock()

	// Request more addresses if we have fewer than 1000 new addresses
	// or if total addresses is less than 25% of maximum
	return len(pd.new) < 1000 || len(pd.addresses) < MaxAddresses/4
}

// AddAddress adds a new address to the discovery system
func (pd *PeerDiscovery) AddAddress(known *KnownAddress, source *NetAddress, src AddressSource) bool {
	if known == nil || known.Addr == nil {
		return false
	}

	// Validate address
	if !pd.isValidAddress(known.Addr) {
		return false
	}

	addrStr := known.Addr.String()

	pd.addrMu.Lock()
	defer pd.addrMu.Unlock()

	// Check if we already have this address
	if existing, exists := pd.addresses[addrStr]; exists {
		// Update last seen time from incoming data (not time.Now()),
		// clamped to prevent future timestamps from misbehaving peers
		incomingLastSeen := known.LastSeen
		now := time.Now()
		if incomingLastSeen.After(now) {
			incomingLastSeen = now
		}
		if incomingLastSeen.After(existing.LastSeen) {
			existing.LastSeen = incomingLastSeen
		}
		if known.Services != 0 {
			existing.Services = known.Services
		}
		if source != nil {
			existing.Source = source
		}
		return false
	}

	// Add source information
	if source != nil {
		known.Source = source
	}

	// Add to addresses
	pd.addresses[addrStr] = known
	pd.new[addrStr] = known

	pd.logger.WithFields(logrus.Fields{
		"address": addrStr,
		"source":  pd.sourceToString(src),
		"total":   len(pd.addresses),
	}).Debug("Added new peer address")

	// Trigger event handler
	if pd.onNewPeer != nil {
		pd.onNewPeer(known.Addr)
	}

	return true
}

// GetAddresses returns a list of addresses for connection attempts
func (pd *PeerDiscovery) GetAddresses(count int) []*NetAddress {
	pd.addrMu.RLock()
	defer pd.addrMu.RUnlock()

	var addresses []*NetAddress

	// Collect new addresses with priorities
	type addrWithPriority struct {
		addr     *KnownAddress
		priority float64
	}

	newAddrs := make([]addrWithPriority, 0, len(pd.new))
	for _, known := range pd.new {
		if !known.IsBad && known.Attempts < 3 {
			priority := pd.calculateAddressPriority(known)
			newAddrs = append(newAddrs, addrWithPriority{
				addr:     known,
				priority: priority,
			})
		}
	}

	// Sort by priority (highest first) instead of random shuffle
	sort.Slice(newAddrs, func(i, j int) bool {
		return newAddrs[i].priority > newAddrs[j].priority
	})

	// Add top priority new addresses first
	for _, ap := range newAddrs {
		if len(addresses) >= count {
			break
		}
		addresses = append(addresses, ap.addr.Addr)
	}

	// If we need more, add some tried addresses
	if len(addresses) < count {
		triedAddrs := make([]addrWithPriority, 0, len(pd.tried))
		for _, known := range pd.tried {
			if !known.IsBad && time.Since(known.LastAttempt) > time.Hour {
				priority := pd.calculateAddressPriority(known)
				triedAddrs = append(triedAddrs, addrWithPriority{
					addr:     known,
					priority: priority,
				})
			}
		}

		// Sort by priority (highest first)
		sort.Slice(triedAddrs, func(i, j int) bool {
			return triedAddrs[i].priority > triedAddrs[j].priority
		})

		for _, ap := range triedAddrs {
			if len(addresses) >= count {
				break
			}
			addresses = append(addresses, ap.addr.Addr)
		}
	}

	return addresses
}

// MarkAttempt marks an address as attempted
func (pd *PeerDiscovery) MarkAttempt(addr *NetAddress) {
	addrStr := addr.String()

	pd.addrMu.Lock()
	defer pd.addrMu.Unlock()

	if known, exists := pd.addresses[addrStr]; exists {
		now := time.Now()

		// Decay: reset attempts if enough time has passed since last attempt
		if !known.LastAttempt.IsZero() && now.Sub(known.LastAttempt) > FailureDecayDuration {
			known.Attempts = 0
		}

		known.Attempts++
		known.LastAttempt = now

		// Move from new to tried
		delete(pd.new, addrStr)
		pd.tried[addrStr] = known
	}
}

// MarkSuccess marks an address as successfully connected
func (pd *PeerDiscovery) MarkSuccess(addr *NetAddress) {
	addrStr := addr.String()

	pd.addrMu.Lock()
	defer pd.addrMu.Unlock()

	if known, exists := pd.addresses[addrStr]; exists {
		known.LastSuccess = time.Now()
		known.LastSeen = time.Now()
		known.IsBad = false
		known.Attempts = 0 // Reset failure counter on successful connection
	}
}

// MarkBad marks an address as bad after repeated failures or explicit misbehavior.
// For connection failures, call MarkFailure instead to respect the failure threshold.
func (pd *PeerDiscovery) MarkBad(addr *NetAddress, reason string) {
	addrStr := addr.String()

	pd.addrMu.Lock()
	defer pd.addrMu.Unlock()

	if known, exists := pd.addresses[addrStr]; exists && !known.Permanent {
		known.IsBad = true
		pd.logger.WithFields(logrus.Fields{
			"address": addrStr,
			"reason":  reason,
		}).Debug("Marked address as bad")
	}
}

// MarkFailure increments failure counter and marks bad only after threshold.
// Use this for transient connection failures to avoid premature eviction.
func (pd *PeerDiscovery) MarkFailure(addr *NetAddress, reason string) {
	addrStr := addr.String()

	pd.addrMu.Lock()
	defer pd.addrMu.Unlock()

	if known, exists := pd.addresses[addrStr]; exists && !known.Permanent {
		// Attempts already incremented by MarkAttempt
		if known.Attempts >= FailureThreshold {
			known.IsBad = true
			pd.logger.WithFields(logrus.Fields{
				"address":  addrStr,
				"attempts": known.Attempts,
				"reason":   reason,
			}).Debug("Marked address as bad after threshold failures")
		}
	}
}

// GetStats returns discovery statistics
func (pd *PeerDiscovery) GetStats() map[string]interface{} {
	pd.addrMu.RLock()
	defer pd.addrMu.RUnlock()

	return map[string]interface{}{
		"total_addresses":      len(pd.addresses),
		"new_addresses":        len(pd.new),
		"tried_addresses":      len(pd.tried),
		"total_requests":       pd.totalRequests.Load(),
		"successful_dns":       pd.successfulDNS.Load(),
		"failed_dns":           pd.failedDNS.Load(),
		"peers_discovered":     pd.peersDiscovered.Load(),
		"seeds_configured":     len(pd.seeds),
		"dns_seeds_configured": len(pd.dnsSeeds),
	}
}

// SetOnNewPeerHandler sets the handler for new peer discovery events
func (pd *PeerDiscovery) SetOnNewPeerHandler(handler func(addr *NetAddress)) {
	pd.onNewPeer = handler
}

// SetRequestAddressesHandler sets the handler for requesting addresses from connected peers
func (pd *PeerDiscovery) SetRequestAddressesHandler(handler func()) {
	pd.requestAddresses = handler
}

// Helper methods

// isValidAddress validates whether an address is valid for connection
func (pd *PeerDiscovery) isValidAddress(addr *NetAddress) bool {
	// Check IP validity
	if addr.IP == nil || addr.IP.IsUnspecified() {
		return false
	}

	// Don't connect to localhost unless in test mode
	if addr.IP.IsLoopback() {
		// Allow loopback in testnet and regtest
		if pd.network != "testnet" && pd.network != "regtest" {
			return false
		}
	}

	// Don't connect to private networks in mainnet
	if addr.IP.IsPrivate() {
		// Allow private IPs in testnet and regtest
		if pd.network != "testnet" && pd.network != "regtest" {
			return false
		}
	}

	// Check port
	if addr.Port == 0 {
		return false
	}

	// Check services
	if addr.Services&SFNodeNetwork == 0 {
		// Peer must at least provide network service
		return false
	}

	return true
}

// sourceToString converts AddressSource to string for logging
func (pd *PeerDiscovery) sourceToString(src AddressSource) string {
	switch src {
	case SourceSeed:
		return "seed"
	case SourceDNS:
		return "dns"
	case SourcePeer:
		return "peer"
	case SourceConfig:
		return "config"
	case SourceMasternodeCache:
		return "mncache"
	default:
		return "unknown"
	}
}

// getDefaultPort returns the default port for the current network
// Replaces hardcoded Bitcoin port 18333 with TWINS network-specific ports
func (pd *PeerDiscovery) getDefaultPort() int {
	switch pd.network {
	case "mainnet":
		return 37817 // TWINS mainnet port
	case "testnet":
		return 37847 // TWINS testnet port (legacy chainparams.cpp:370)
	case "regtest":
		return 5467 // TWINS regtest port (legacy chainparams.cpp:494)
	default:
		return 37817 // Default to mainnet
	}
}

// prepareAddressesForSave creates a filtered deep copy of addresses for persistence.
// Filters out stale peers and non-standard port peers we've never connected to.
func (pd *PeerDiscovery) prepareAddressesForSave() map[string]*KnownAddress {
	pd.addrMu.RLock()
	defer pd.addrMu.RUnlock()

	now := time.Now()
	defaultPort := uint16(pd.getDefaultPort())
	addresses := make(map[string]*KnownAddress, len(pd.addresses))
	filtered := 0

	for k, v := range pd.addresses {
		// Skip stale addresses (LastSeen > 7 days) unless permanent
		if now.Sub(v.LastSeen) > AddressTimeout && !v.Permanent {
			filtered++
			continue
		}

		// Skip non-standard port peers we've never successfully connected to
		if v.Addr.Port != defaultPort && v.LastSuccess.IsZero() && !v.Permanent {
			filtered++
			continue
		}

		addresses[k] = v.Clone()
	}

	if filtered > 0 {
		pd.logger.WithFields(logrus.Fields{
			"filtered": filtered,
			"kept":     len(addresses),
		}).Debug("Filtered addresses for persistence")
	}

	return addresses
}

// addressPersistenceLoop periodically saves addresses to disk
func (pd *PeerDiscovery) addressPersistenceLoop() {
	defer pd.wg.Done()

	// Match legacy interval: 15 minutes
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			addresses := pd.prepareAddressesForSave()
			if err := pd.addrDB.Save(addresses); err != nil {
				pd.logger.WithError(err).Error("Failed to save addresses")
			}

		case <-pd.quit:
			return
		}
	}
}

// emergencyBootstrapLoop checks for emergency bootstrap conditions
func (pd *PeerDiscovery) emergencyBootstrapLoop() {
	defer pd.wg.Done()

	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pd.checkEmergencyBootstrap()

		case <-pd.quit:
			return
		}
	}
}

// checkEmergencyBootstrap checks if emergency bootstrap is needed
func (pd *PeerDiscovery) checkEmergencyBootstrap() {
	pd.addrMu.RLock()
	addressCount := len(pd.addresses)
	pd.addrMu.RUnlock()

	lastDNS := pd.lastSuccessfulDNS.Load()
	var timeSinceSuccess time.Duration
	if lastDNS > 0 {
		timeSinceSuccess = time.Since(time.Unix(lastDNS, 0))
	} else {
		// If never had successful DNS, use a large value
		timeSinceSuccess = 10 * time.Minute
	}

	// Emergency condition: very few addresses and DNS failing for > 5 minutes
	if addressCount < 10 && timeSinceSuccess > 5*time.Minute {
		pd.logger.WithFields(logrus.Fields{
			"addresses":              addressCount,
			"time_since_dns_success": timeSinceSuccess,
		}).Warn("Emergency bootstrap: loading fixed seed nodes")
		pd.loadFixedSeeds()
	}
}

// loadFixedSeeds loads hardcoded fixed seed nodes for emergency bootstrap
func (pd *PeerDiscovery) loadFixedSeeds() {
	// Fixed seeds for TWINS network based on network type
	var fixedSeeds []string

	switch pd.network {
	case "mainnet":
		// Fixed seeds from production TWINS network (from twinsd.yml)
		// Geographically distributed seed nodes for emergency bootstrap
		fixedSeeds = []string{
			"159.65.195.97:37817",
			"134.209.146.52:37817",
			"46.101.113.6:37817",
			"138.68.154.249:37817",
			"137.184.217.142:37817",
			"165.22.149.70:37817",
			"170.64.157.157:37817",
			"134.122.38.24:37817",
			"45.77.64.171:37817",
			"45.32.36.145:37817",
			"45.77.206.161:37817",
			"207.148.67.25:37817",
		}
	case "testnet":
		// Testnet fixed seeds (subset of mainnet nodes that support testnet)
		// Note: These may need to be updated with actual testnet-specific seeds
		fixedSeeds = []string{
			"45.77.64.171:37847",    // Germany (testnet port)
			"149.28.149.146:37847",  // Singapore (testnet port)
			"45.32.36.145:37847",    // Japan (testnet port)
		}
	case "regtest":
		// Regtest typically doesn't need fixed seeds
		fixedSeeds = []string{}
	default:
		pd.logger.Warn("Unknown network type for fixed seeds")
		return
	}

	if len(fixedSeeds) == 0 {
		pd.logger.Debug("No fixed seeds configured for this network")
		return
	}

	for _, seedAddr := range fixedSeeds {
		host, portStr, err := net.SplitHostPort(seedAddr)
		if err != nil {
			pd.logger.WithError(err).WithField("seed", seedAddr).
				Warn("Invalid fixed seed address format")
			continue
		}

		// Resolve IP
		ips, err := net.LookupIP(host)
		if err != nil {
			pd.logger.WithError(err).WithField("seed", seedAddr).
				Warn("Failed to resolve fixed seed address")
			continue
		}

		for _, ip := range ips {
			// Parse port
			port := pd.getDefaultPort()
			if portStr != "" {
				if p, err := net.ResolveTCPAddr("tcp", net.JoinHostPort("", portStr)); err == nil {
					port = p.Port
				}
			}

			addr := &NetAddress{
				Time:     uint32(time.Now().Unix()),
				Services: SFNodeNetwork,
				IP:       ip,
				Port:     uint16(port),
			}

			known := &KnownAddress{
				Addr:      addr,
				LastSeen:  time.Now(),
				Services:  SFNodeNetwork,
				Permanent: true, // Fixed seeds are permanent
			}

			if pd.AddAddress(known, nil, SourceSeed) {
				pd.logger.WithField("seed", addr.String()).Debug("Added fixed seed node")
			}
		}
	}
}

// AddMasternodeAddresses injects masternode addresses from mncache.dat into the
// peer discovery system as priority bootstrap peers. These addresses are marked
// with SFNodeMasternode service flag so they receive the existing priority boost
// (+50 priority, 5x selection chance) during peer selection.
func (pd *PeerDiscovery) AddMasternodeAddresses(addrs []string) int {
	added := 0
	for _, addrStr := range addrs {
		host, portStr, err := net.SplitHostPort(addrStr)
		if err != nil {
			continue
		}

		ip := net.ParseIP(host)
		if ip == nil {
			continue
		}

		port := pd.getDefaultPort()
		if portStr != "" {
			if p, err := strconv.Atoi(portStr); err == nil && p > 0 && p <= 65535 {
				port = p
			}
		}

		addr := &NetAddress{
			Time:     uint32(time.Now().Unix()),
			Services: SFNodeNetwork | SFNodeMasternode,
			IP:       ip,
			Port:     uint16(port),
		}

		known := &KnownAddress{
			Addr:     addr,
			LastSeen: time.Now(),
			Services: SFNodeNetwork | SFNodeMasternode,
		}

		if pd.AddAddress(known, nil, SourceMasternodeCache) {
			added++
		}
	}
	return added
}
