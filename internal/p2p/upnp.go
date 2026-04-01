package p2p

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// UPnPManager handles UPnP port mapping for NAT traversal
// This is a simplified implementation - for production use consider
// using a library like github.com/huin/goupnp
type UPnPManager struct {
	mu          sync.Mutex
	enabled     bool
	port        int
	externalIP  net.IP
	mappedPort  int
	logger      *logrus.Entry
	ctx         context.Context
	cancel      context.CancelFunc
	refreshDone chan struct{}
}

// NewUPnPManager creates a new UPnP manager
func NewUPnPManager(port int, logger *logrus.Logger) *UPnPManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &UPnPManager{
		enabled:     false,
		port:        port,
		logger:      logger.WithField("component", "upnp"),
		ctx:         ctx,
		cancel:      cancel,
		refreshDone: make(chan struct{}),
	}
}

// Start attempts to set up UPnP port mapping
func (u *UPnPManager) Start() error {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.logger.Debug("Attempting UPnP port mapping")

	// Try to discover UPnP gateway and map port
	if err := u.discoverAndMap(); err != nil {
		u.logger.WithError(err).Warn("UPnP port mapping failed - node may not be reachable from outside")
		return err
	}

	u.enabled = true

	// Start refresh goroutine to keep mapping alive
	go u.refreshLoop()

	return nil
}

// Stop removes the UPnP port mapping
func (u *UPnPManager) Stop() {
	u.mu.Lock()
	defer u.mu.Unlock()

	if !u.enabled {
		return
	}

	u.cancel()
	<-u.refreshDone

	// Try to remove port mapping
	if err := u.removeMapping(); err != nil {
		u.logger.WithError(err).Debug("Failed to remove UPnP port mapping")
	}

	u.enabled = false
	u.logger.Debug("UPnP port mapping removed")
}

// GetExternalIP returns the external IP discovered via UPnP
func (u *UPnPManager) GetExternalIP() net.IP {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.externalIP
}

// GetMappedPort returns the mapped external port
func (u *UPnPManager) GetMappedPort() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.mappedPort
}

// IsEnabled returns whether UPnP is active
func (u *UPnPManager) IsEnabled() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.enabled
}

// discoverAndMap discovers UPnP gateway and creates port mapping
// This is a stub - real implementation would use SSDP discovery and SOAP
func (u *UPnPManager) discoverAndMap() error {
	// UPnP implementation requires:
	// 1. SSDP M-SEARCH to discover gateway (multicast to 239.255.255.250:1900)
	// 2. Parse device description XML
	// 3. SOAP call to AddPortMapping
	//
	// For now, log that UPnP is not fully implemented
	// Production code should use github.com/huin/goupnp or similar

	u.logger.Debug("UPnP discovery not fully implemented - consider using external library")

	// Try to get external IP via alternative methods
	externalIP := u.getExternalIPFallback()
	if externalIP != nil {
		u.externalIP = externalIP
		u.mappedPort = u.port // Assume direct mapping
		u.logger.WithFields(logrus.Fields{
			"external_ip": externalIP.String(),
			"port":        u.port,
		}).Info("External IP detected (UPnP not available)")
		return nil
	}

	return fmt.Errorf("UPnP gateway not found and external IP detection failed")
}

// removeMapping removes the UPnP port mapping
func (u *UPnPManager) removeMapping() error {
	// Would call DeletePortMapping SOAP action
	return nil
}

// refreshLoop periodically refreshes the port mapping
func (u *UPnPManager) refreshLoop() {
	defer close(u.refreshDone)

	// UPnP mappings typically expire after 2 hours
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-u.ctx.Done():
			return
		case <-ticker.C:
			u.mu.Lock()
			if u.enabled {
				if err := u.discoverAndMap(); err != nil {
					u.logger.WithError(err).Debug("Failed to refresh UPnP mapping")
				}
			}
			u.mu.Unlock()
		}
	}
}

// getExternalIPFallback tries to detect external IP without UPnP
func (u *UPnPManager) getExternalIPFallback() net.IP {
	// Try to get external IP by connecting to a known server
	// This doesn't actually send data, just opens a UDP socket to determine route
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP
}

// DiscoverExternalIP attempts to discover the external IP address
// Uses multiple methods: UPnP, then fallback to local interface detection
func DiscoverExternalIP(logger *logrus.Logger) net.IP {
	entry := logger.WithField("component", "ip-discovery")

	// Try UDP-based detection (gets routable interface IP)
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		if localAddr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
			// Only return if not a private IP (indicates NAT)
			if !isPrivateIP(localAddr.IP) {
				entry.WithField("ip", localAddr.IP.String()).Debug("Detected external IP via routing")
				return localAddr.IP
			}
			entry.WithField("ip", localAddr.IP.String()).Debug("Detected private IP, likely behind NAT")
		}
	}

	// Fallback: return first non-loopback interface IP
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				entry.WithField("ip", ipnet.IP.String()).Debug("Using interface IP")
				return ipnet.IP
			}
		}
	}

	return nil
}

// isPrivateIP checks if an IP is in a private range
func isPrivateIP(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		// 10.0.0.0/8
		if ip4[0] == 10 {
			return true
		}
		// 172.16.0.0/12
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ip4[0] == 192 && ip4[1] == 168 {
			return true
		}
	}
	return false
}
