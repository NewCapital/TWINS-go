package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	// Test network settings
	if config.Network.Port != 37817 {
		t.Errorf("Expected default port 37817, got %d", config.Network.Port)
	}
	if config.Network.TestNet {
		t.Errorf("Expected testnet flag false, got %v", config.Network.TestNet)
	}
	if len(config.Network.Seeds) == 0 {
		t.Error("Expected seeds to be populated")
	}

	// Test RPC settings
	if config.RPC.Port != 37818 {
		t.Errorf("Expected RPC port 37818, got %d", config.RPC.Port)
	}
}


func TestConfigLoadFromFile(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.yml")

	testConfig := map[string]interface{}{
		"network": map[string]interface{}{
			"port":     9999,
			"testNet":  true,
			"maxPeers": 42,
		},
		"rpc": map[string]interface{}{
			"port":    9998,
			"enabled": true,
		},
		"logging": map[string]interface{}{
			"level": "debug",
		},
	}

	data, err := yaml.Marshal(testConfig)
	if err != nil {
		t.Fatalf("Failed to marshal test config: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	// Load config
	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify loaded values
	if config.Network.Port != 9999 {
		t.Errorf("Expected port 9999, got %d", config.Network.Port)
	}
	if !config.Network.TestNet {
		t.Errorf("Expected testnet true, got %v", config.Network.TestNet)
	}
	if config.Network.MaxPeers != 42 {
		t.Errorf("Expected max_peers 42, got %d", config.Network.MaxPeers)
	}
	if config.RPC.Port != 9998 {
		t.Errorf("Expected RPC port 9998, got %d", config.RPC.Port)
	}
	if config.Logging.Level != "debug" {
		t.Errorf("Expected logging level debug, got %s", config.Logging.Level)
	}
}

// TestOverlayBoolDefaults verifies that explicit false in YAML overrides default true
func TestOverlayBoolDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.yml")

	// Test that listen: false correctly overrides default listen: true
	testConfig := map[string]interface{}{
		"network": map[string]interface{}{
			"listen": false, // Explicitly set to false
		},
	}

	data, err := yaml.Marshal(testConfig)
	if err != nil {
		t.Fatalf("Failed to marshal test config: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify listen is false (overriding default true)
	if config.Network.Listen {
		t.Errorf("Expected listen false (from YAML), got %v", config.Network.Listen)
	}

	// Verify other defaults are preserved
	if !config.Network.DNS {
		t.Errorf("Expected DNS true (default), got %v", config.Network.DNS)
	}
	if config.Network.Port != 37817 {
		t.Errorf("Expected default port 37817, got %d", config.Network.Port)
	}
}

// TestOverlayPreservesDefaults verifies that missing fields keep defaults
func TestOverlayPreservesDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.yml")

	// Minimal config - only set port
	testConfig := map[string]interface{}{
		"network": map[string]interface{}{
			"port": 9999,
		},
	}

	data, err := yaml.Marshal(testConfig)
	if err != nil {
		t.Fatalf("Failed to marshal test config: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify port is overridden
	if config.Network.Port != 9999 {
		t.Errorf("Expected port 9999, got %d", config.Network.Port)
	}

	// Verify bool defaults are preserved (not reset to false)
	if !config.Network.Listen {
		t.Errorf("Expected listen true (default preserved), got %v", config.Network.Listen)
	}
	if !config.Network.DNS {
		t.Errorf("Expected DNS true (default preserved), got %v", config.Network.DNS)
	}
	if !config.Network.UPnP {
		t.Errorf("Expected UPnP true (default preserved), got %v", config.Network.UPnP)
	}
	if !config.RPC.Enabled {
		t.Errorf("Expected RPC.Enabled true (default preserved), got %v", config.RPC.Enabled)
	}
}

func TestConfigLoadFromInvalidFile(t *testing.T) {
	// Test non-existent file
	_, err := LoadConfig("/non/existent/file.yml")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}

	// Test invalid YAML
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yml")

	invalidYAML := "invalid: yaml: content:"
	if err := os.WriteFile(configPath, []byte(invalidYAML), 0644); err != nil {
		t.Fatalf("Failed to write invalid config file: %v", err)
	}

	_, err = LoadConfig(configPath)
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}


func TestGetDefaultSeeds(t *testing.T) {
	mainnetSeeds := GetDefaultSeeds("mainnet")
	if len(mainnetSeeds) == 0 {
		t.Error("Expected mainnet seeds to be populated")
	}

	testnetSeeds := GetDefaultSeeds("testnet")
	if len(testnetSeeds) == 0 {
		t.Error("Expected testnet seeds to be populated")
	}

	regtestSeeds := GetDefaultSeeds("regtest")
	if len(regtestSeeds) != 0 {
		t.Error("Expected regtest seeds to be empty")
	}

	invalidSeeds := GetDefaultSeeds("invalid")
	if len(invalidSeeds) != 0 {
		t.Error("Expected invalid network seeds to be empty")
	}
}

func TestGetDefaultPorts(t *testing.T) {
	tests := []struct {
		network     string
		expectedP2P int
		expectedRPC int
	}{
		{"mainnet", 37817, 37818},
		{"testnet", 37819, 37820},
		{"regtest", 37821, 37822},
		{"invalid", 37817, 37818}, // Falls back to mainnet
	}

	for _, test := range tests {
		t.Run(test.network, func(t *testing.T) {
			p2p, rpc := GetDefaultPorts(test.network)

			if p2p != test.expectedP2P {
				t.Errorf("Expected P2P port %d, got %d", test.expectedP2P, p2p)
			}
			if rpc != test.expectedRPC {
				t.Errorf("Expected RPC port %d, got %d", test.expectedRPC, rpc)
			}
		})
	}
}

func TestGetDefaultDataDir(t *testing.T) {
	tests := []struct {
		network  string
		expected string
	}{
		{"mainnet", "./mainnet"},
		{"testnet", "./testnet"},
		{"regtest", "./regtest"},
		{"invalid", "."},
	}

	for _, test := range tests {
		t.Run(test.network, func(t *testing.T) {
			dataDir := GetDefaultDataDir(test.network)
			if dataDir != test.expected {
				t.Errorf("Expected data dir %s, got %s", test.expected, dataDir)
			}
		})
	}
}

func TestConfigClone(t *testing.T) {
	original := DefaultConfig()
	original.Network.ExternalIP = "1.2.3.4"
	original.Masternode.Enabled = true

	cloned := original.Clone()

	// Verify clone is independent
	cloned.Network.ExternalIP = "5.6.7.8"
	cloned.Masternode.Enabled = false

	if original.Network.ExternalIP == cloned.Network.ExternalIP {
		t.Error("Clone should be independent of original")
	}
	if original.Masternode.Enabled == cloned.Masternode.Enabled {
		t.Error("Clone should be independent of original")
	}
}

func BenchmarkLoadConfig(b *testing.B) {
	// Create temporary config file
	tmpDir := b.TempDir()
	configPath := filepath.Join(tmpDir, "bench.yml")

	config := DefaultConfig()
	data, _ := yaml.Marshal(config)
	_ = os.WriteFile(configPath, data, 0644)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = LoadConfig(configPath)
	}
}

func BenchmarkValidateConfig(b *testing.B) {
	config := DefaultConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ValidateConfig(config)
	}
}
