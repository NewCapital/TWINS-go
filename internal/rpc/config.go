package rpc

import "time"

// Config holds RPC server configuration
type Config struct {
	// Network settings
	Host string
	Port int

	// Authentication
	Username      string
	Password      string
	UseCookieAuth bool
	DataDir       string

	// Connection settings
	MaxClients     int
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	IdleTimeout    time.Duration
	MaxRequestSize int64
	RateLimit      int // Maximum requests per minute per IP (0 = no limit)

	// CORS settings
	EnableCORS     bool
	AllowedOrigins []string

	// IP filtering - compatible with legacy -rpcallowip
	// If empty, only localhost (127.0.0.1, ::1) is allowed
	// Supports CIDR notation (192.168.1.0/24) and single IPs
	AllowedIPs []string

	// TLS settings
	TLSEnabled bool
	CertFile   string
	KeyFile    string
}

// DefaultConfig returns a default RPC configuration
func DefaultConfig() *Config {
	return &Config{
		Host:           "127.0.0.1",
		Port:           11771, // Default TWINS RPC port
		UseCookieAuth:  true,
		MaxClients:     100,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		IdleTimeout:    120 * time.Second,
		MaxRequestSize: 10 * 1024 * 1024, // 10MB
		EnableCORS:     false,
	}
}