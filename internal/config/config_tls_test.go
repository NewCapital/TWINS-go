package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRPCTLSConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.RPC.TLS.Enabled {
		t.Error("TLS should be disabled by default")
	}
	if cfg.RPC.TLS.ExpiryWarnDays != 30 {
		t.Errorf("ExpiryWarnDays default should be 30, got %d", cfg.RPC.TLS.ExpiryWarnDays)
	}
	if cfg.RPC.AllowPlaintextPublic {
		t.Error("AllowPlaintextPublic should be false by default")
	}
	if cfg.RPC.TLS.MTLS.Enabled {
		t.Error("mTLS should be disabled by default")
	}
	if cfg.RPC.TLS.Client.CAFile != "" {
		t.Errorf("Client.CAFile should be empty by default, got %q", cfg.RPC.TLS.Client.CAFile)
	}
	if cfg.RPC.TLS.Client.PinSHA256 != "" {
		t.Errorf("Client.PinSHA256 should be empty by default, got %q", cfg.RPC.TLS.Client.PinSHA256)
	}
}

func TestRPCTLSConfigYAMLParsing(t *testing.T) {
	yamlContent := `
rpc:
  enabled: true
  host: "0.0.0.0"
  port: 37818
  allowPlaintextPublic: true
  tls:
    enabled: true
    certFile: "/etc/twins/rpc.crt"
    keyFile: "/etc/twins/rpc.key"
    expiryWarnDays: 14
    reloadPassphraseFile: "/home/twins/.twins/rpcreload.hash"
    mtls:
      enabled: true
      clientCAFile: "/etc/twins/client-ca.pem"
    client:
      caFile: "/etc/twins/ca-bundle.pem"
      pinSHA256: "abc123def456"
`
	tmpFile := filepath.Join(t.TempDir(), "twinsd.yml")
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfg, err := LoadConfig(tmpFile)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if !cfg.RPC.AllowPlaintextPublic {
		t.Error("AllowPlaintextPublic should be true")
	}
	if !cfg.RPC.TLS.Enabled {
		t.Error("TLS.Enabled should be true")
	}
	if cfg.RPC.TLS.CertFile != "/etc/twins/rpc.crt" {
		t.Errorf("CertFile = %q, want /etc/twins/rpc.crt", cfg.RPC.TLS.CertFile)
	}
	if cfg.RPC.TLS.KeyFile != "/etc/twins/rpc.key" {
		t.Errorf("KeyFile = %q, want /etc/twins/rpc.key", cfg.RPC.TLS.KeyFile)
	}
	if cfg.RPC.TLS.ExpiryWarnDays != 14 {
		t.Errorf("ExpiryWarnDays = %d, want 14", cfg.RPC.TLS.ExpiryWarnDays)
	}
	if cfg.RPC.TLS.ReloadPassphraseFile != "/home/twins/.twins/rpcreload.hash" {
		t.Errorf("ReloadPassphraseFile = %q, want /home/twins/.twins/rpcreload.hash", cfg.RPC.TLS.ReloadPassphraseFile)
	}
	if !cfg.RPC.TLS.MTLS.Enabled {
		t.Error("MTLS.Enabled should be true")
	}
	if cfg.RPC.TLS.MTLS.ClientCAFile != "/etc/twins/client-ca.pem" {
		t.Errorf("MTLS.ClientCAFile = %q, want /etc/twins/client-ca.pem", cfg.RPC.TLS.MTLS.ClientCAFile)
	}
	if cfg.RPC.TLS.Client.CAFile != "/etc/twins/ca-bundle.pem" {
		t.Errorf("Client.CAFile = %q, want /etc/twins/ca-bundle.pem", cfg.RPC.TLS.Client.CAFile)
	}
	if cfg.RPC.TLS.Client.PinSHA256 != "abc123def456" {
		t.Errorf("Client.PinSHA256 = %q, want abc123def456", cfg.RPC.TLS.Client.PinSHA256)
	}
}

func TestRPCTLSConfigYAMLMinimal(t *testing.T) {
	// Minimal config with no TLS keys — defaults should be populated
	yamlContent := `
rpc:
  enabled: true
  host: "127.0.0.1"
  port: 37818
`
	tmpFile := filepath.Join(t.TempDir(), "twinsd.yml")
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfg, err := LoadConfig(tmpFile)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// All TLS fields should retain defaults
	if cfg.RPC.TLS.Enabled {
		t.Error("TLS.Enabled should be false (default)")
	}
	if cfg.RPC.TLS.ExpiryWarnDays != 30 {
		t.Errorf("ExpiryWarnDays should be 30 (default), got %d", cfg.RPC.TLS.ExpiryWarnDays)
	}
	if cfg.RPC.AllowPlaintextPublic {
		t.Error("AllowPlaintextPublic should be false (default)")
	}
}

func TestRPCTLSConfigOverlayMerge(t *testing.T) {
	cfg := DefaultConfig()

	certFile := "/etc/twins/rpc.crt"
	keyFile := "/etc/twins/rpc.key"
	enabled := true
	expiryDays := 7
	mtlsEnabled := true
	clientCA := "/etc/twins/client-ca.pem"
	pinSHA := "sha256pin"

	overlay := &ConfigOverlay{
		RPC: &RPCConfigOverlay{
			TLS: &RPCTLSConfigOverlay{
				Enabled:  &enabled,
				CertFile: &certFile,
				KeyFile:  &keyFile,
				ExpiryWarnDays: &expiryDays,
				MTLS: &RPCMTLSConfigOverlay{
					Enabled:      &mtlsEnabled,
					ClientCAFile: &clientCA,
				},
				Client: &RPCClientTLSConfigOverlay{
					PinSHA256: &pinSHA,
				},
			},
		},
	}

	cfg.MergeFromOverlay(overlay)

	if !cfg.RPC.TLS.Enabled {
		t.Error("TLS.Enabled should be true after merge")
	}
	if cfg.RPC.TLS.CertFile != certFile {
		t.Errorf("CertFile = %q, want %q", cfg.RPC.TLS.CertFile, certFile)
	}
	if cfg.RPC.TLS.KeyFile != keyFile {
		t.Errorf("KeyFile = %q, want %q", cfg.RPC.TLS.KeyFile, keyFile)
	}
	if cfg.RPC.TLS.ExpiryWarnDays != 7 {
		t.Errorf("ExpiryWarnDays = %d, want 7", cfg.RPC.TLS.ExpiryWarnDays)
	}
	if !cfg.RPC.TLS.MTLS.Enabled {
		t.Error("MTLS.Enabled should be true after merge")
	}
	if cfg.RPC.TLS.MTLS.ClientCAFile != clientCA {
		t.Errorf("MTLS.ClientCAFile = %q, want %q", cfg.RPC.TLS.MTLS.ClientCAFile, clientCA)
	}
	if cfg.RPC.TLS.Client.PinSHA256 != pinSHA {
		t.Errorf("Client.PinSHA256 = %q, want %q", cfg.RPC.TLS.Client.PinSHA256, pinSHA)
	}
	// CAFile not in overlay — should retain default (empty)
	if cfg.RPC.TLS.Client.CAFile != "" {
		t.Errorf("Client.CAFile should be empty (not in overlay), got %q", cfg.RPC.TLS.Client.CAFile)
	}
	// ReloadPassphraseFile not in overlay — should retain default (empty)
	if cfg.RPC.TLS.ReloadPassphraseFile != "" {
		t.Errorf("ReloadPassphraseFile should be empty (not in overlay), got %q", cfg.RPC.TLS.ReloadPassphraseFile)
	}
}

func TestRPCTLSValidation_EnabledNoCert(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RPC.TLS.Enabled = true
	cfg.RPC.TLS.CertFile = "" // missing
	cfg.RPC.TLS.KeyFile = "/etc/twins/rpc.key"

	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for missing certFile")
	}
	if !strings.Contains(err.Error(), "certFile") {
		t.Errorf("error should mention certFile, got: %v", err)
	}
}

func TestRPCTLSValidation_EnabledNoKey(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RPC.TLS.Enabled = true
	cfg.RPC.TLS.CertFile = "/etc/twins/rpc.crt"
	cfg.RPC.TLS.KeyFile = "" // missing

	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for missing keyFile")
	}
	if !strings.Contains(err.Error(), "keyFile") {
		t.Errorf("error should mention keyFile, got: %v", err)
	}
}

func TestRPCTLSValidation_ExpiryWarnDaysZero(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RPC.TLS.Enabled = true
	cfg.RPC.TLS.CertFile = "/etc/twins/rpc.crt"
	cfg.RPC.TLS.KeyFile = "/etc/twins/rpc.key"
	cfg.RPC.TLS.ExpiryWarnDays = 0

	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for ExpiryWarnDays=0")
	}
	if !strings.Contains(err.Error(), "expiryWarnDays") {
		t.Errorf("error should mention expiryWarnDays, got: %v", err)
	}
}

func TestRPCTLSValidation_MTLSNoClientCA(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RPC.TLS.Enabled = true
	cfg.RPC.TLS.CertFile = "/etc/twins/rpc.crt"
	cfg.RPC.TLS.KeyFile = "/etc/twins/rpc.key"
	cfg.RPC.TLS.MTLS.Enabled = true
	cfg.RPC.TLS.MTLS.ClientCAFile = "" // missing

	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for mTLS without clientCAFile")
	}
	if !strings.Contains(err.Error(), "clientCAFile") {
		t.Errorf("error should mention clientCAFile, got: %v", err)
	}
}

func TestRPCTLSValidation_MTLSWithoutTLS(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RPC.TLS.Enabled = false
	cfg.RPC.TLS.MTLS.Enabled = true

	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error for mTLS without TLS")
	}
	if !strings.Contains(err.Error(), "mTLS requires TLS") {
		t.Errorf("error should mention mTLS requires TLS, got: %v", err)
	}
}

func TestRPCTLSValidation_ValidFullConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RPC.TLS.Enabled = true
	cfg.RPC.TLS.CertFile = "/etc/twins/rpc.crt"
	cfg.RPC.TLS.KeyFile = "/etc/twins/rpc.key"
	cfg.RPC.TLS.ExpiryWarnDays = 14
	cfg.RPC.TLS.MTLS.Enabled = true
	cfg.RPC.TLS.MTLS.ClientCAFile = "/etc/twins/client-ca.pem"

	err := ValidateConfig(cfg)
	if err != nil {
		t.Fatalf("expected no validation error, got: %v", err)
	}
}

func TestRPCTLSConfigManagerGetSet(t *testing.T) {
	cm := NewConfigManager(filepath.Join(t.TempDir(), "twinsd.yml"), nil)

	// Check defaults via ConfigManager
	if cm.GetBool("rpc.tls.enabled") {
		t.Error("rpc.tls.enabled default should be false")
	}
	if cm.GetInt("rpc.tls.expiryWarnDays") != 30 {
		t.Errorf("rpc.tls.expiryWarnDays default = %d, want 30", cm.GetInt("rpc.tls.expiryWarnDays"))
	}
	if cm.GetString("rpc.tls.certFile") != "" {
		t.Errorf("rpc.tls.certFile default should be empty, got %q", cm.GetString("rpc.tls.certFile"))
	}
	if cm.GetBool("rpc.tls.mtls.enabled") {
		t.Error("rpc.tls.mtls.enabled default should be false")
	}
	if cm.GetBool("rpc.allowPlaintextPublic") {
		t.Error("rpc.allowPlaintextPublic default should be false")
	}

	// Set values via SetFromCLI (bypasses persistence)
	if err := cm.SetFromCLI("rpc.tls.enabled", true); err != nil {
		t.Fatalf("SetFromCLI rpc.tls.enabled: %v", err)
	}
	if err := cm.SetFromCLI("rpc.tls.certFile", "/etc/twins/rpc.crt"); err != nil {
		t.Fatalf("SetFromCLI rpc.tls.certFile: %v", err)
	}
	if err := cm.SetFromCLI("rpc.tls.keyFile", "/etc/twins/rpc.key"); err != nil {
		t.Fatalf("SetFromCLI rpc.tls.keyFile: %v", err)
	}
	if err := cm.SetFromCLI("rpc.tls.expiryWarnDays", 7); err != nil {
		t.Fatalf("SetFromCLI rpc.tls.expiryWarnDays: %v", err)
	}
	if err := cm.SetFromCLI("rpc.tls.client.pinSHA256", "abc123"); err != nil {
		t.Fatalf("SetFromCLI rpc.tls.client.pinSHA256: %v", err)
	}

	// Verify values set correctly
	if !cm.GetBool("rpc.tls.enabled") {
		t.Error("rpc.tls.enabled should be true after SetFromCLI")
	}
	if cm.GetString("rpc.tls.certFile") != "/etc/twins/rpc.crt" {
		t.Errorf("rpc.tls.certFile = %q after set", cm.GetString("rpc.tls.certFile"))
	}
	if cm.GetInt("rpc.tls.expiryWarnDays") != 7 {
		t.Errorf("rpc.tls.expiryWarnDays = %d, want 7", cm.GetInt("rpc.tls.expiryWarnDays"))
	}
	if cm.GetString("rpc.tls.client.pinSHA256") != "abc123" {
		t.Errorf("rpc.tls.client.pinSHA256 = %q, want abc123", cm.GetString("rpc.tls.client.pinSHA256"))
	}
}

func TestRPCTLSConfigManagerMetadata(t *testing.T) {
	cm := NewConfigManager(filepath.Join(t.TempDir(), "twinsd.yml"), nil)
	meta := cm.GetAllMetadata()

	// Check that all TLS keys are registered
	tlsKeys := map[string]bool{
		"rpc.allowPlaintextPublic":      false,
		"rpc.tls.enabled":              false,
		"rpc.tls.certFile":             false,
		"rpc.tls.keyFile":              false,
		"rpc.tls.expiryWarnDays":       false,
		"rpc.tls.reloadPassphraseFile": false,
		"rpc.tls.mtls.enabled":         false,
		"rpc.tls.mtls.clientCAFile":    false,
		"rpc.tls.client.caFile":        false,
		"rpc.tls.client.pinSHA256":     false,
	}

	for _, m := range meta {
		if _, ok := tlsKeys[m.Key]; ok {
			tlsKeys[m.Key] = true
			if m.Category != "rpc" {
				t.Errorf("key %q should be in category 'rpc', got %q", m.Key, m.Category)
			}
		}
	}

	for key, found := range tlsKeys {
		if !found {
			t.Errorf("TLS key %q not found in metadata", key)
		}
	}
}

func TestRPCTLSYAMLGeneration(t *testing.T) {
	yamlPath := filepath.Join(t.TempDir(), "twinsd.yml")
	cm := NewConfigManager(yamlPath, nil)
	if err := cm.LoadOrCreate(); err != nil {
		t.Fatalf("LoadOrCreate failed: %v", err)
	}

	// Read generated YAML
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("failed to read generated config: %v", err)
	}
	content := string(data)

	// The generated YAML should have nested tls: section, not flat tls.enabled:
	if strings.Contains(content, "tls.enabled:") {
		t.Error("YAML should use nested 'tls:' section, not flat 'tls.enabled:' key")
	}
	if !strings.Contains(content, "tls:") {
		t.Error("YAML should contain 'tls:' section header")
	}
	if !strings.Contains(content, "mtls:") {
		t.Error("YAML should contain 'mtls:' sub-section header")
	}
	if !strings.Contains(content, "allowPlaintextPublic:") {
		t.Error("YAML should contain 'allowPlaintextPublic:' key")
	}

	// Verify the generated YAML can be parsed back
	var overlay ConfigOverlay
	if err := yaml.Unmarshal(data, &overlay); err != nil {
		t.Fatalf("generated YAML is not valid: %v", err)
	}
	if overlay.RPC == nil {
		t.Fatal("overlay.RPC is nil after parsing generated YAML")
	}
	if overlay.RPC.TLS == nil {
		t.Fatal("overlay.RPC.TLS is nil after parsing generated YAML")
	}
	if overlay.RPC.TLS.Enabled == nil {
		t.Fatal("overlay.RPC.TLS.Enabled is nil after parsing generated YAML")
	}
	if *overlay.RPC.TLS.Enabled {
		t.Error("TLS.Enabled should be false in generated YAML")
	}
	if overlay.RPC.TLS.ExpiryWarnDays == nil || *overlay.RPC.TLS.ExpiryWarnDays != 30 {
		t.Error("ExpiryWarnDays should be 30 in generated YAML")
	}
}

func TestRPCTLSYAMLRoundTrip(t *testing.T) {
	// Create config with TLS settings
	yamlPath := filepath.Join(t.TempDir(), "twinsd.yml")
	cm := NewConfigManager(yamlPath, nil)
	if err := cm.LoadOrCreate(); err != nil {
		t.Fatalf("LoadOrCreate failed: %v", err)
	}

	// Modify TLS settings via Set (persists to YAML)
	if err := cm.Set("rpc.tls.enabled", true); err != nil {
		t.Fatalf("Set rpc.tls.enabled: %v", err)
	}
	if err := cm.Set("rpc.tls.certFile", "/etc/twins/rpc.crt"); err != nil {
		t.Fatalf("Set rpc.tls.certFile: %v", err)
	}
	if err := cm.Set("rpc.tls.expiryWarnDays", 14); err != nil {
		t.Fatalf("Set rpc.tls.expiryWarnDays: %v", err)
	}

	// Load into a fresh ConfigManager to verify round-trip
	cm2 := NewConfigManager(yamlPath, nil)
	if err := cm2.LoadOrCreate(); err != nil {
		t.Fatalf("second LoadOrCreate failed: %v", err)
	}

	if !cm2.GetBool("rpc.tls.enabled") {
		t.Error("rpc.tls.enabled should be true after round-trip")
	}
	if cm2.GetString("rpc.tls.certFile") != "/etc/twins/rpc.crt" {
		t.Errorf("rpc.tls.certFile = %q after round-trip, want /etc/twins/rpc.crt", cm2.GetString("rpc.tls.certFile"))
	}
	if cm2.GetInt("rpc.tls.expiryWarnDays") != 14 {
		t.Errorf("rpc.tls.expiryWarnDays = %d after round-trip, want 14", cm2.GetInt("rpc.tls.expiryWarnDays"))
	}
	// Unmodified nested key should retain default
	if cm2.GetBool("rpc.tls.mtls.enabled") {
		t.Error("rpc.tls.mtls.enabled should be false (unmodified) after round-trip")
	}
}
