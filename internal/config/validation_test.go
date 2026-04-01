package config

import (
	"strings"
	"testing"
)

func TestValidateConfig(t *testing.T) {
	// Test valid config
	validConfig := DefaultConfig()
	if err := ValidateConfig(validConfig); err != nil {
		t.Errorf("Valid config should pass validation: %v", err)
	}

	// Test nil config
	if err := ValidateConfig(nil); err == nil {
		t.Error("Nil config should fail validation")
	}
}

func TestValidateNetworkConfig(t *testing.T) {
	tests := []struct {
		name          string
		config        NetworkConfig
		expectError   bool
		errorContains string
	}{
		{
			name: "valid config",
			config: NetworkConfig{
				Port:         8333,
				MaxPeers:     100,
				ListenAddr:   "0.0.0.0",
				Timeout:      30,
				KeepAlive:    60,
				MaxBandwidth: 0,
			},
			expectError: false,
		},
		{
			name: "invalid port - too low",
			config: NetworkConfig{
				Port:     1023,
				MaxPeers: 100,
			},
			expectError:   true,
			errorContains: "port must be between 1024 and 65535",
		},
		{
			name: "invalid port - too high",
			config: NetworkConfig{
				Port:     65536,
				MaxPeers: 100,
			},
			expectError:   true,
			errorContains: "port must be between 1024 and 65535",
		},
		{
			name: "invalid max peers - too low",
			config: NetworkConfig{
				Port:     8333,
				MaxPeers: 0,
			},
			expectError:   true,
			errorContains: "max_peers must be at least 1",
		},
		{
			name: "invalid max peers - too high",
			config: NetworkConfig{
				Port:     8333,
				MaxPeers: 10001,
			},
			expectError:   true,
			errorContains: "max_peers cannot exceed 10000",
		},
		{
			name: "invalid listen addr",
			config: NetworkConfig{
				Port:       8333,
				MaxPeers:   100,
				ListenAddr: "invalid-address",
			},
			expectError:   true,
			errorContains: "invalid listen_addr",
		},
		{
			name: "invalid timeout",
			config: NetworkConfig{
				Port:     8333,
				MaxPeers: 100,
				Timeout:  0,
			},
			expectError:   true,
			errorContains: "timeout must be at least 1 second",
		},
		{
			name: "invalid keep alive",
			config: NetworkConfig{
				Port:      8333,
				MaxPeers:  100,
				Timeout:   30,
				KeepAlive: 0,
			},
			expectError:   true,
			errorContains: "keep_alive must be at least 1 second",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateNetworkConfig(&test.config)

			if test.expectError {
				if err == nil {
					t.Error("Expected validation error")
					return
				}
				if test.errorContains != "" && !strings.Contains(err.Error(), test.errorContains) {
					t.Errorf("Expected error containing '%s', got: %v", test.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected validation error: %v", err)
				}
			}
		})
	}
}

func TestValidateRPCConfig(t *testing.T) {
	tests := []struct {
		name          string
		config        RPCConfig
		expectError   bool
		errorContains string
	}{
		{
			name: "valid config",
			config: RPCConfig{
				Enabled:    true,
				Port:       8332,
				Host:       "127.0.0.1",
				MaxClients: 100,
				AllowedIPs: []string{"127.0.0.1", "::1"},
				RateLimit:  100,
				Timeout:    30,
			},
			expectError: false,
		},
		{
			name: "disabled RPC",
			config: RPCConfig{
				Enabled: false,
			},
			expectError: false,
		},
		{
			name: "invalid port",
			config: RPCConfig{
				Enabled: true,
				Port:    0,
			},
			expectError:   true,
			errorContains: "port must be between 1024 and 65535",
		},
		{
			name: "invalid host",
			config: RPCConfig{
				Enabled:    true,
				Port:       8332,
				Host:       "invalid_host!",
				MaxClients: 100,
				RateLimit:  100,
				Timeout:    30,
			},
			expectError:   true,
			errorContains: "invalid host",
		},
		{
			name: "invalid max clients",
			config: RPCConfig{
				Enabled:    true,
				Port:       8332,
				Host:       "127.0.0.1",
				MaxClients: 0,
			},
			expectError:   true,
			errorContains: "max_clients must be at least 1",
		},
		{
			name: "invalid allowed IP",
			config: RPCConfig{
				Enabled:    true,
				Port:       8332,
				Host:       "127.0.0.1",
				MaxClients: 100,
				AllowedIPs: []string{"invalid-ip"},
			},
			expectError:   true,
			errorContains: "invalid IP address in allowed_ips",
		},
		{
			name: "invalid rate limit",
			config: RPCConfig{
				Enabled:    true,
				Port:       8332,
				Host:       "127.0.0.1",
				MaxClients: 100,
				RateLimit:  0,
			},
			expectError:   true,
			errorContains: "rate_limit must be at least 1",
		},
		{
			name: "invalid timeout",
			config: RPCConfig{
				Enabled:    true,
				Port:       8332,
				Host:       "127.0.0.1",
				MaxClients: 100,
				RateLimit:  100,
				Timeout:    0,
			},
			expectError:   true,
			errorContains: "timeout must be at least 1 second",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateRPCConfig(&test.config)

			if test.expectError {
				if err == nil {
					t.Error("Expected validation error")
					return
				}
				if test.errorContains != "" && !strings.Contains(err.Error(), test.errorContains) {
					t.Errorf("Expected error containing '%s', got: %v", test.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected validation error: %v", err)
				}
			}
		})
	}
}

func TestValidateMasternodeConfig(t *testing.T) {
	tests := []struct {
		name          string
		config        MasternodeConfig
		expectError   bool
		errorContains string
	}{
		{
			name: "disabled masternode",
			config: MasternodeConfig{
				Enabled: false,
			},
			expectError: false,
		},
		{
			name: "valid enabled masternode",
			config: MasternodeConfig{
				Enabled:     true,
				PrivateKey:  "valid-private-key",
				ServiceAddr: "192.168.1.1:8333",
			},
			expectError: false,
		},
		{
			name: "enabled but missing private key",
			config: MasternodeConfig{
				Enabled: true,
			},
			expectError:   true,
			errorContains: "private_key is required when masternode is enabled",
		},
		{
			name: "enabled but missing service addr",
			config: MasternodeConfig{
				Enabled:    true,
				PrivateKey: "valid-key",
			},
			expectError:   true,
			errorContains: "service_addr is required when masternode is enabled",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateMasternodeConfig(&test.config)

			if test.expectError {
				if err == nil {
					t.Error("Expected validation error")
					return
				}
				if test.errorContains != "" && !strings.Contains(err.Error(), test.errorContains) {
					t.Errorf("Expected error containing '%s', got: %v", test.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected validation error: %v", err)
				}
			}
		})
	}
}

func TestValidateLoggingConfig(t *testing.T) {
	tests := []struct {
		name          string
		config        LoggingConfig
		expectError   bool
		errorContains string
	}{
		{
			name: "valid config",
			config: LoggingConfig{
				Level:  "info",
				Format: "text",
				Output: "stdout",
			},
			expectError: false,
		},
		{
			name: "invalid level",
			config: LoggingConfig{
				Level: "invalid",
			},
			expectError:   true,
			errorContains: "invalid logging level",
		},
		{
			name: "invalid format",
			config: LoggingConfig{
				Level:  "info",
				Format: "invalid",
			},
			expectError:   true,
			errorContains: "unsupported logging format",
		},
		{
			name: "empty output",
			config: LoggingConfig{
				Level:  "info",
				Format: "text",
				Output: "",
			},
			expectError:   true,
			errorContains: "logging output must be",
		},
		{
			name: "file path output",
			config: LoggingConfig{
				Level:  "info",
				Format: "text",
				Output: "/var/log/twinsd.log",
			},
			expectError: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateLoggingConfig(&test.config)

			if test.expectError {
				if err == nil {
					t.Error("Expected validation error")
					return
				}
				if test.errorContains != "" && !strings.Contains(err.Error(), test.errorContains) {
					t.Errorf("Expected error containing '%s', got: %v", test.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected validation error: %v", err)
				}
			}
		})
	}
}

func TestDetectPortConflicts(t *testing.T) {
	tests := []struct {
		name          string
		config        *Config
		expectError   bool
		errorContains string
	}{
		{
			name:        "no conflicts",
			config:      DefaultConfig(),
			expectError: false,
		},
		{
			name: "network and RPC port conflict",
			config: &Config{
				Network: NetworkConfig{Port: 8333},
				RPC:     RPCConfig{Enabled: true, Port: 8333},
			},
			expectError:   true,
			errorContains: "port conflict between rpc and network",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := detectPortConflicts(test.config)

			if test.expectError {
				if err == nil {
					t.Error("Expected port conflict error")
					return
				}
				if test.errorContains != "" && !strings.Contains(err.Error(), test.errorContains) {
					t.Errorf("Expected error containing '%s', got: %v", test.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected port conflict error: %v", err)
				}
			}
		})
	}
}
