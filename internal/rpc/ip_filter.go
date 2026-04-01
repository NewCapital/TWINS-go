package rpc

import (
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

// IPFilter provides IP-based access control for RPC connections.
// Supports CIDR notation, single IP addresses, and IPv4/IPv6.
// Compatible with legacy -rpcallowip behavior.
type IPFilter struct {
	allowedNets []*net.IPNet
	allowedIPs  []net.IP
	mu          sync.RWMutex
	logger      *logrus.Entry
}

// NewIPFilter creates a new IP filter from a list of allowed IP/CIDR strings.
// If allowedIPs is empty, only localhost (127.0.0.1, ::1) is allowed.
// Supports formats:
//   - Single IP: "192.168.1.1"
//   - CIDR: "192.168.1.0/24"
//   - IPv6: "::1", "fe80::/10"
func NewIPFilter(allowedIPs []string, logger *logrus.Entry) *IPFilter {
	filter := &IPFilter{
		allowedNets: make([]*net.IPNet, 0),
		allowedIPs:  make([]net.IP, 0),
		logger:      logger,
	}

	// Default: only localhost if no IPs specified
	if len(allowedIPs) == 0 {
		allowedIPs = []string{"127.0.0.1", "::1"}
	}

	for _, ipStr := range allowedIPs {
		ipStr = strings.TrimSpace(ipStr)
		if ipStr == "" {
			continue
		}

		// Try to parse as CIDR first
		if strings.Contains(ipStr, "/") {
			_, ipNet, err := net.ParseCIDR(ipStr)
			if err != nil {
				if logger != nil {
					logger.WithError(err).Warnf("Invalid CIDR notation: %s", ipStr)
				}
				continue
			}
			filter.allowedNets = append(filter.allowedNets, ipNet)
			if logger != nil {
				logger.Debugf("Added allowed CIDR: %s", ipNet.String())
			}
		} else {
			// Parse as single IP
			ip := net.ParseIP(ipStr)
			if ip == nil {
				if logger != nil {
					logger.Warnf("Invalid IP address: %s", ipStr)
				}
				continue
			}
			filter.allowedIPs = append(filter.allowedIPs, ip)
			if logger != nil {
				logger.Debugf("Added allowed IP: %s", ip.String())
			}
		}
	}

	return filter
}

// IsAllowed checks if the given IP address is allowed.
func (f *IPFilter) IsAllowed(ipStr string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	// Normalize IPv4-mapped IPv6 addresses (::ffff:192.168.1.1) to IPv4
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}

	// Check single IPs
	for _, allowedIP := range f.allowedIPs {
		checkIP := allowedIP
		if v4 := checkIP.To4(); v4 != nil {
			checkIP = v4
		}
		if checkIP.Equal(ip) {
			return true
		}
	}

	// Check CIDR networks
	for _, ipNet := range f.allowedNets {
		if ipNet.Contains(ip) {
			return true
		}
	}

	return false
}

// IsAllowedAddr checks if the given address (host:port) is allowed.
// Extracts the host part and checks against allowed IPs.
func (f *IPFilter) IsAllowedAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// Try to parse as IP without port
		host = addr
	}

	// Handle IPv6 addresses with brackets
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")

	return f.IsAllowed(host)
}

// Middleware returns an HTTP middleware that filters requests by IP.
// Rejected requests receive HTTP 403 Forbidden.
func (f *IPFilter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := f.extractClientIP(r)

		if !f.IsAllowed(clientIP) {
			if f.logger != nil {
				f.logger.WithField("ip", clientIP).Warn("RPC connection rejected: IP not allowed")
			}
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// extractClientIP extracts the client IP from the request.
// SECURITY: Only uses RemoteAddr to prevent IP spoofing via X-Forwarded-For.
// Proxy headers (X-Forwarded-For, X-Real-IP) are intentionally ignored
// because they can be forged by attackers to bypass IP filtering.
// If running behind a reverse proxy, configure the proxy to set RemoteAddr correctly.
func (f *IPFilter) extractClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// AddAllowedIP adds an IP address to the allowed list at runtime.
func (f *IPFilter) AddAllowedIP(ipStr string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	ipStr = strings.TrimSpace(ipStr)

	if strings.Contains(ipStr, "/") {
		_, ipNet, err := net.ParseCIDR(ipStr)
		if err != nil {
			return err
		}
		f.allowedNets = append(f.allowedNets, ipNet)
	} else {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			return &net.ParseError{Type: "IP address", Text: ipStr}
		}
		f.allowedIPs = append(f.allowedIPs, ip)
	}

	return nil
}

// AllowedCount returns the number of allowed IPs and networks.
func (f *IPFilter) AllowedCount() (ips int, nets int) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.allowedIPs), len(f.allowedNets)
}
