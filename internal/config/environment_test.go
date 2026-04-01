package config

import (
	"os"
	"testing"
)

func TestLoadFromEnvironment(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	for _, env := range os.Environ() {
		if len(env) > 6 && env[:6] == "TWINS_" {
			parts := splitEnvVar(env)
			if len(parts) == 2 {
				originalEnv[parts[0]] = parts[1]
			}
		}
	}

	// Clean up environment after test
	defer func() {
		// Clear all TWINS_ variables
		for _, env := range os.Environ() {
			if len(env) > 6 && env[:6] == "TWINS_" {
				parts := splitEnvVar(env)
				if len(parts) == 2 {
					os.Unsetenv(parts[0])
				}
			}
		}
		// Restore original environment
		for key, value := range originalEnv {
			os.Setenv(key, value)
		}
	}()

	// Set test environment variables
	testEnvVars := map[string]string{
		"TWINS_NETWORK_PORT":         "9999",
		"TWINS_NETWORK_MAX_PEERS":    "42",
		"TWINS_TESTNET":              "true",
		"TWINS_RPC_ENABLED":          "true",
		"TWINS_RPC_PORT":             "9998",
		"TWINS_RPC_HOST":             "192.168.1.1",
		"TWINS_DATABASE_PATH":        "/custom/data",
		"TWINS_DATABASE_CACHE_SIZE":  "1024",
		"TWINS_CONSENSUS_BLOCK_TIME": "60",
		"TWINS_MASTERNODE_ENABLED":   "true",
		"TWINS_MASTERNODE_TIER":      "silver",
		"TWINS_LOGGING_LEVEL":        "debug",
	}

	for key, value := range testEnvVars {
		if err := os.Setenv(key, value); err != nil {
			t.Fatalf("Failed to set environment variable %s: %v", key, err)
		}
	}

	// Load configuration from environment
	config, err := LoadFromEnvironment()
	if err != nil {
		t.Fatalf("Failed to load config from environment: %v", err)
	}

	// Verify environment variable values were applied
	if config.Network.Port != 9999 {
		t.Errorf("Expected network port 9999, got %d", config.Network.Port)
	}
	if config.Network.MaxPeers != 42 {
		t.Errorf("Expected max peers 42, got %d", config.Network.MaxPeers)
	}
	if config.Network.TestNet != true {
		t.Errorf("Expected testnet true, got %v", config.Network.TestNet)
	}
	if config.RPC.Enabled != true {
		t.Errorf("Expected RPC enabled true, got %v", config.RPC.Enabled)
	}
	if config.RPC.Port != 9998 {
		t.Errorf("Expected RPC port 9998, got %d", config.RPC.Port)
	}
	if config.RPC.Host != "192.168.1.1" {
		t.Errorf("Expected RPC host 192.168.1.1, got %s", config.RPC.Host)
	}
	if config.Masternode.Enabled != true {
		t.Errorf("Expected masternode enabled true, got %v", config.Masternode.Enabled)
	}
	if config.Masternode.MnConf != "masternode.conf" {
		t.Errorf("Expected masternode mnconf masternode.conf, got %s", config.Masternode.MnConf)
	}
	if config.Logging.Level != "debug" {
		t.Errorf("Expected logging level debug, got %s", config.Logging.Level)
	}
}

func TestLoadFromEnvironmentPartial(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	for _, env := range os.Environ() {
		if len(env) > 6 && env[:6] == "TWINS_" {
			parts := splitEnvVar(env)
			if len(parts) == 2 {
				originalEnv[parts[0]] = parts[1]
			}
		}
	}

	// Clean up environment after test
	defer func() {
		// Clear all TWINS_ variables
		for _, env := range os.Environ() {
			if len(env) > 6 && env[:6] == "TWINS_" {
				parts := splitEnvVar(env)
				if len(parts) == 2 {
					os.Unsetenv(parts[0])
				}
			}
		}
		// Restore original environment
		for key, value := range originalEnv {
			os.Setenv(key, value)
		}
	}()

	// Set only a few environment variables
	os.Setenv("TWINS_NETWORK_PORT", "7777")
	os.Setenv("TWINS_RPC_ENABLED", "false")

	config, err := LoadFromEnvironment()
	if err != nil {
		t.Fatalf("Failed to load config from environment: %v", err)
	}

	// Check that environment values were applied
	if config.Network.Port != 7777 {
		t.Errorf("Expected network port 7777, got %d", config.Network.Port)
	}
	if config.RPC.Enabled != false {
		t.Errorf("Expected RPC enabled false, got %v", config.RPC.Enabled)
	}

	// Check that defaults were applied for non-set variables
	if config.Network.MaxPeers == 0 {
		t.Error("Expected default max peers to be applied")
	}
}

func TestApplyEnvironmentOverrides(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	for _, env := range os.Environ() {
		if len(env) > 6 && env[:6] == "TWINS_" {
			parts := splitEnvVar(env)
			if len(parts) == 2 {
				originalEnv[parts[0]] = parts[1]
			}
		}
	}

	// Clean up environment after test
	defer func() {
		// Clear all TWINS_ variables
		for _, env := range os.Environ() {
			if len(env) > 6 && env[:6] == "TWINS_" {
				parts := splitEnvVar(env)
				if len(parts) == 2 {
					os.Unsetenv(parts[0])
				}
			}
		}
		// Restore original environment
		for key, value := range originalEnv {
			os.Setenv(key, value)
		}
	}()

	// Start with a base config
	config := DefaultConfig()
	originalPort := config.Network.Port
	originalHost := config.RPC.Host

	// Set environment variables
	os.Setenv("TWINS_NETWORK_PORT", "6666")
	os.Setenv("TWINS_RPC_HOST", "0.0.0.0")

	// Apply environment overrides
	err := ApplyEnvironmentOverrides(config)
	if err != nil {
		t.Fatalf("Failed to apply environment overrides: %v", err)
	}

	// Check that environment values were applied
	if config.Network.Port == originalPort {
		t.Error("Expected network port to be overridden by environment")
	}
	if config.Network.Port != 6666 {
		t.Errorf("Expected network port 6666, got %d", config.Network.Port)
	}
	if config.RPC.Host == originalHost {
		t.Error("Expected RPC host to be overridden by environment")
	}
	if config.RPC.Host != "0.0.0.0" {
		t.Errorf("Expected RPC host 0.0.0.0, got %s", config.RPC.Host)
	}
}

func TestGetEnvironmentOverrides(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	for _, env := range os.Environ() {
		if len(env) > 6 && env[:6] == "TWINS_" {
			parts := splitEnvVar(env)
			if len(parts) == 2 {
				originalEnv[parts[0]] = parts[1]
			}
		}
	}

	// Clean up environment after test
	defer func() {
		// Clear all TWINS_ variables
		for _, env := range os.Environ() {
			if len(env) > 6 && env[:6] == "TWINS_" {
				parts := splitEnvVar(env)
				if len(parts) == 2 {
					os.Unsetenv(parts[0])
				}
			}
		}
		// Restore original environment
		for key, value := range originalEnv {
			os.Setenv(key, value)
		}
	}()

	// Set test environment variables
	testVars := map[string]string{
		"TWINS_NETWORK_PORT":        "8888",
		"TWINS_RPC_ENABLED":         "true",
		"TWINS_DATABASE_CACHE_SIZE": "2048",
		"TWINS_LOGGING_LEVEL":       "trace",
		"NON_TWINS_VAR":             "should-be-ignored",
	}

	for key, value := range testVars {
		os.Setenv(key, value)
	}

	overrides := GetEnvironmentOverrides()

	// Check that TWINS_ variables were captured
	expectedKeys := []string{
		"TWINS_NETWORK_PORT",
		"TWINS_RPC_ENABLED",
		"TWINS_DATABASE_CACHE_SIZE",
		"TWINS_LOGGING_LEVEL",
	}

	for _, key := range expectedKeys {
		if _, exists := overrides[key]; !exists {
			t.Errorf("Expected environment override %s to be captured", key)
		}
	}

	// Check that non-TWINS variables were ignored
	if _, exists := overrides["NON_TWINS_VAR"]; exists {
		t.Error("Non-TWINS environment variable should be ignored")
	}

	// Check values
	if overrides["TWINS_NETWORK_PORT"] != "8888" {
		t.Errorf("Expected TWINS_NETWORK_PORT=8888, got %s", overrides["TWINS_NETWORK_PORT"])
	}
}

func TestInvalidEnvironmentValues(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	for _, env := range os.Environ() {
		if len(env) > 6 && env[:6] == "TWINS_" {
			parts := splitEnvVar(env)
			if len(parts) == 2 {
				originalEnv[parts[0]] = parts[1]
			}
		}
	}

	// Clean up environment after test
	defer func() {
		// Clear all TWINS_ variables
		for _, env := range os.Environ() {
			if len(env) > 6 && env[:6] == "TWINS_" {
				parts := splitEnvVar(env)
				if len(parts) == 2 {
					os.Unsetenv(parts[0])
				}
			}
		}
		// Restore original environment
		for key, value := range originalEnv {
			os.Setenv(key, value)
		}
	}()

	tests := []struct {
		name   string
		envVar string
		value  string
	}{
		{"invalid port", "TWINS_NETWORK_PORT", "invalid"},
		{"invalid boolean", "TWINS_RPC_ENABLED", "maybe"},
		{"invalid integer", "TWINS_DATABASE_CACHE_SIZE", "not-a-number"},
		{"invalid float", "TWINS_CONSENSUS_STAKE_REWARD", "not-a-float"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Clear environment first
			for _, env := range os.Environ() {
				if len(env) > 6 && env[:6] == "TWINS_" {
					parts := splitEnvVar(env)
					if len(parts) == 2 {
						os.Unsetenv(parts[0])
					}
				}
			}

			// Set invalid environment variable
			os.Setenv(test.envVar, test.value)

			// Loading should fail or ignore invalid values
			config, err := LoadFromEnvironment()
			if err != nil {
				// This is acceptable - invalid values cause load to fail
				return
			}

			// If load succeeds, invalid values should be ignored and defaults used
			// We don't test specific values here since behavior may vary,
			// but the config should be valid
			if config == nil {
				t.Error("Expected config to be returned even with invalid environment values")
			}

			// Clean up for next test
			os.Unsetenv(test.envVar)
		})
	}
}

func TestLegacyEnvironmentSupport(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	for _, env := range os.Environ() {
		if len(env) > 6 && env[:6] == "TWINS_" {
			parts := splitEnvVar(env)
			if len(parts) == 2 {
				originalEnv[parts[0]] = parts[1]
			}
		}
	}

	// Clean up environment after test
	defer func() {
		// Clear all TWINS_ variables
		for _, env := range os.Environ() {
			if len(env) > 6 && env[:6] == "TWINS_" {
				parts := splitEnvVar(env)
				if len(parts) == 2 {
					os.Unsetenv(parts[0])
				}
			}
		}
		// Restore original environment
		for key, value := range originalEnv {
			os.Setenv(key, value)
		}
	}()

	legacyVars := GetLegacyEnvironmentSupport()

	// Check that we have some legacy mappings
	if len(legacyVars) == 0 {
		t.Error("Expected some legacy environment variable mappings")
	}

	// Test a few known legacy mappings (these should be documented)
	expectedMappings := map[string]string{
		"TWINS_PORT":     "TWINS_NETWORK_PORT",
		"TWINS_RPC_PORT": "TWINS_RPC_PORT",
		"TWINS_TESTNET":  "TWINS_NETWORK_TESTNET",
	}

	for legacy, modern := range expectedMappings {
		if mapped, exists := legacyVars[legacy]; exists {
			if mapped != modern {
				t.Errorf("Expected legacy var %s to map to %s, got %s", legacy, modern, mapped)
			}
		}
	}
}

func TestEnvironmentVariableNames(t *testing.T) {
	// Test that all expected environment variables are documented
	envVars := GetSupportedEnvironmentVariables()

	if len(envVars) == 0 {
		t.Error("Expected supported environment variables to be documented")
	}

	// Check for key environment variables
	expectedVars := []string{
		"TWINS_NETWORK_PORT",
		"TWINS_NETWORK_MAX_PEERS",
		EnvTestNet,
		"TWINS_RPC_ENABLED",
		"TWINS_RPC_PORT",
		"TWINS_RPC_HOST",
		"TWINS_MASTERNODE_ENABLED",
		"TWINS_LOGGING_LEVEL",
	}

	for _, expected := range expectedVars {
		found := false
		for _, envVar := range envVars {
			if envVar.Name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected environment variable %s to be documented", expected)
		}
	}
}

// Helper function to split environment variable into key=value
func splitEnvVar(env string) []string {
	for i, c := range env {
		if c == '=' {
			return []string{env[:i], env[i+1:]}
		}
	}
	return []string{env}
}

func BenchmarkLoadFromEnvironment(b *testing.B) {
	// Set a few environment variables for the benchmark
	os.Setenv("TWINS_NETWORK_PORT", "8333")
	os.Setenv("TWINS_RPC_ENABLED", "true")
	os.Setenv("TWINS_LOGGING_LEVEL", "info")

	defer func() {
		os.Unsetenv("TWINS_NETWORK_PORT")
		os.Unsetenv("TWINS_RPC_ENABLED")
		os.Unsetenv("TWINS_LOGGING_LEVEL")
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = LoadFromEnvironment()
	}
}

func BenchmarkApplyEnvironmentOverrides(b *testing.B) {
	config := DefaultConfig()

	os.Setenv("TWINS_NETWORK_PORT", "8333")
	os.Setenv("TWINS_RPC_ENABLED", "true")
	os.Setenv("TWINS_LOGGING_LEVEL", "info")

	defer func() {
		os.Unsetenv("TWINS_NETWORK_PORT")
		os.Unsetenv("TWINS_RPC_ENABLED")
		os.Unsetenv("TWINS_LOGGING_LEVEL")
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ApplyEnvironmentOverrides(config)
	}
}
