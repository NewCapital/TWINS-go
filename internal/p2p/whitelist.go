package p2p

import (
	"net"
	"sync"
)

// WhitelistManager manages IP whitelisting for incoming connections
type WhitelistManager struct {
	mu        sync.RWMutex
	networks  []*net.IPNet // Whitelisted CIDR networks
	addresses []net.IP     // Whitelisted individual IPs
	enabled   bool         // Whether whitelist filtering is active
}

// NewWhitelistManager creates a new whitelist manager from config strings
// Accepts formats: "192.168.1.1", "192.168.0.0/24", "::1", "fe80::/10"
func NewWhitelistManager(whitelist []string) *WhitelistManager {
	wm := &WhitelistManager{
		networks:  make([]*net.IPNet, 0),
		addresses: make([]net.IP, 0),
		enabled:   len(whitelist) > 0,
	}

	for _, entry := range whitelist {
		// Try parsing as CIDR first
		_, network, err := net.ParseCIDR(entry)
		if err == nil {
			wm.networks = append(wm.networks, network)
			continue
		}

		// Try parsing as individual IP
		ip := net.ParseIP(entry)
		if ip != nil {
			wm.addresses = append(wm.addresses, ip)
		}
	}

	return wm
}

// IsWhitelisted checks if an IP address is in the whitelist
// Returns true if:
// - Whitelist is empty (not enabled - all IPs allowed)
// - IP matches an entry in the whitelist
func (wm *WhitelistManager) IsWhitelisted(ip net.IP) bool {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	// If whitelist is not enabled, all IPs are allowed
	if !wm.enabled {
		return true
	}

	// Check individual IPs
	for _, whiteIP := range wm.addresses {
		if ip.Equal(whiteIP) {
			return true
		}
	}

	// Check CIDR networks
	for _, network := range wm.networks {
		if network.Contains(ip) {
			return true
		}
	}

	return false
}

// IsWhitelistedAddr checks if a net.Addr is whitelisted
func (wm *WhitelistManager) IsWhitelistedAddr(addr net.Addr) bool {
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok {
		return false
	}
	return wm.IsWhitelisted(tcpAddr.IP)
}

// Add adds an IP or CIDR to the whitelist
func (wm *WhitelistManager) Add(entry string) bool {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	// Try parsing as CIDR first
	_, network, err := net.ParseCIDR(entry)
	if err == nil {
		wm.networks = append(wm.networks, network)
		wm.enabled = true
		return true
	}

	// Try parsing as individual IP
	ip := net.ParseIP(entry)
	if ip != nil {
		wm.addresses = append(wm.addresses, ip)
		wm.enabled = true
		return true
	}

	return false
}

// IsEnabled returns whether whitelist filtering is active
func (wm *WhitelistManager) IsEnabled() bool {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return wm.enabled
}

// Count returns the number of whitelist entries
func (wm *WhitelistManager) Count() int {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return len(wm.networks) + len(wm.addresses)
}

// WhiteBindManager manages addresses that are both bound and whitelisted
// When a peer connects to a whitebind address, they are automatically whitelisted
type WhiteBindManager struct {
	mu         sync.RWMutex
	bindAddrs  map[string]bool // Map of bind addresses that auto-whitelist
	whitelists *WhitelistManager
}

// NewWhiteBindManager creates a new whitebind manager
func NewWhiteBindManager(whitebind []string, whitelist *WhitelistManager) *WhiteBindManager {
	wbm := &WhiteBindManager{
		bindAddrs:  make(map[string]bool),
		whitelists: whitelist,
	}

	for _, addr := range whitebind {
		wbm.bindAddrs[addr] = true
	}

	return wbm
}

// IsWhiteBindAddr checks if an address is a whitebind address
func (wbm *WhiteBindManager) IsWhiteBindAddr(addr string) bool {
	wbm.mu.RLock()
	defer wbm.mu.RUnlock()
	return wbm.bindAddrs[addr]
}

// GetWhiteBindAddrs returns all whitebind addresses
func (wbm *WhiteBindManager) GetWhiteBindAddrs() []string {
	wbm.mu.RLock()
	defer wbm.mu.RUnlock()

	addrs := make([]string, 0, len(wbm.bindAddrs))
	for addr := range wbm.bindAddrs {
		addrs = append(addrs, addr)
	}
	return addrs
}

// ShouldWhitelistPeer checks if a peer connecting to localAddr should be whitelisted
// This is used when a peer connects to a whitebind address
func (wbm *WhiteBindManager) ShouldWhitelistPeer(localAddr string) bool {
	return wbm.IsWhiteBindAddr(localAddr)
}
