// Copyright (c) 2018-2025 The TWINS developers
// Distributed under the MIT/X11 software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package masternode

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMasternodeConfFile_Read(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "mnconfig_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test masternode.conf
	confContent := `# Masternode config file
# Comment line
mn1 127.0.0.1:37817 93HaYBVUCYjEMeeH1Y4sBGLALQZE1Yc1K64xiqgX37tGBDQL8Xg 2bcd3c84c84f87eaa86e4e56834c92927a07f9e18718810b92e0d0324456a67c 0
mn2 192.168.1.100:37817 93HaYBVUCYjEMeeH1Y4sBGLALQZE1Yc1K64xiqgX37tGBDQL8Xg 3bcd3c84c84f87eaa86e4e56834c92927a07f9e18718810b92e0d0324456a67c 1

# Another comment
mn3 10.0.0.1:37817 93HaYBVUCYjEMeeH1Y4sBGLALQZE1Yc1K64xiqgX37tGBDQL8Xg 4bcd3c84c84f87eaa86e4e56834c92927a07f9e18718810b92e0d0324456a67c 2 DAddr123:25
`
	confPath := filepath.Join(tmpDir, "masternode.conf")
	if err := os.WriteFile(confPath, []byte(confContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Test reading
	mc := NewMasternodeConfFile(tmpDir, "masternode.conf")
	if err := mc.Read(); err != nil {
		t.Fatalf("Read() failed: %v", err)
	}

	// Verify entries
	entries := mc.GetEntries()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Check first entry
	if entries[0].Alias != "mn1" {
		t.Errorf("entry 0: expected alias 'mn1', got '%s'", entries[0].Alias)
	}
	if entries[0].IP != "127.0.0.1:37817" {
		t.Errorf("entry 0: expected IP '127.0.0.1:37817', got '%s'", entries[0].IP)
	}
	if entries[0].OutputIndex != 0 {
		t.Errorf("entry 0: expected outputIndex 0, got %d", entries[0].OutputIndex)
	}

	// Check entry with donation
	if entries[2].DonationAddress != "DAddr123" {
		t.Errorf("entry 2: expected donation address 'DAddr123', got '%s'", entries[2].DonationAddress)
	}
	if entries[2].DonationPercent != 25 {
		t.Errorf("entry 2: expected donation percent 25, got %d", entries[2].DonationPercent)
	}
}

func TestMasternodeConfFile_CreateTemplate(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "mnconfig_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Read non-existent file (should create template)
	mc := NewMasternodeConfFile(tmpDir, "masternode.conf")
	if err := mc.Read(); err != nil {
		t.Fatalf("Read() failed: %v", err)
	}

	// Verify template was created
	confPath := filepath.Join(tmpDir, "masternode.conf")
	if _, err := os.Stat(confPath); os.IsNotExist(err) {
		t.Fatalf("template file was not created")
	}

	// Verify no entries
	if mc.GetCount() != 0 {
		t.Errorf("expected 0 entries in template, got %d", mc.GetCount())
	}
}

func TestMasternodeConfFile_GetEntry(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "mnconfig_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	confContent := `mn1 127.0.0.1:37817 privkey1 2bcd3c84c84f87eaa86e4e56834c92927a07f9e18718810b92e0d0324456a67c 0
mn2 127.0.0.2:37817 privkey2 3bcd3c84c84f87eaa86e4e56834c92927a07f9e18718810b92e0d0324456a67c 1
`
	confPath := filepath.Join(tmpDir, "masternode.conf")
	if err := os.WriteFile(confPath, []byte(confContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	mc := NewMasternodeConfFile(tmpDir, "masternode.conf")
	if err := mc.Read(); err != nil {
		t.Fatalf("Read() failed: %v", err)
	}

	// Test GetEntry
	entry := mc.GetEntry("mn1")
	if entry == nil {
		t.Fatal("GetEntry('mn1') returned nil")
	}
	if entry.PrivKey != "privkey1" {
		t.Errorf("expected privkey 'privkey1', got '%s'", entry.PrivKey)
	}

	// Test non-existent entry
	entry = mc.GetEntry("nonexistent")
	if entry != nil {
		t.Error("GetEntry('nonexistent') should return nil")
	}
}

func TestMasternodeConfFile_AddRemove(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "mnconfig_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mc := NewMasternodeConfFile(tmpDir, "masternode.conf")
	if err := mc.Read(); err != nil {
		t.Fatalf("Read() failed: %v", err)
	}

	// Add entry
	entry := &MasternodeEntry{
		Alias:       "testmn",
		IP:          "192.168.1.1:37817",
		PrivKey:     "testprivkey",
		OutputIndex: 0,
	}
	if err := mc.Add(entry); err != nil {
		t.Fatalf("Add() failed: %v", err)
	}

	if mc.GetCount() != 1 {
		t.Errorf("expected 1 entry, got %d", mc.GetCount())
	}

	// Test duplicate alias
	if err := mc.Add(entry); err == nil {
		t.Error("Add() should fail for duplicate alias")
	}

	// Remove entry
	if !mc.Remove("testmn") {
		t.Error("Remove() returned false for existing entry")
	}

	if mc.GetCount() != 0 {
		t.Errorf("expected 0 entries after remove, got %d", mc.GetCount())
	}

	// Remove non-existent
	if mc.Remove("nonexistent") {
		t.Error("Remove() returned true for non-existent entry")
	}
}

func TestMasternodeConfFile_Save(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "mnconfig_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mc := NewMasternodeConfFile(tmpDir, "masternode.conf")
	if err := mc.Read(); err != nil {
		t.Fatalf("Read() failed: %v", err)
	}

	// Add entries
	entry1 := &MasternodeEntry{
		Alias:       "mn1",
		IP:          "127.0.0.1:37817",
		PrivKey:     "privkey1",
		OutputIndex: 0,
	}
	entry1.TxHash[0] = 0x2b // Set some hash bytes
	mc.Add(entry1)

	entry2 := &MasternodeEntry{
		Alias:           "mn2",
		IP:              "127.0.0.2:37817",
		PrivKey:         "privkey2",
		OutputIndex:     1,
		DonationAddress: "DonationAddr",
		DonationPercent: 50,
	}
	entry2.TxHash[0] = 0x3b
	mc.Add(entry2)

	// Save
	if err := mc.Save(); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Read back and verify
	mc2 := NewMasternodeConfFile(tmpDir, "masternode.conf")
	if err := mc2.Read(); err != nil {
		t.Fatalf("Read() after save failed: %v", err)
	}

	if mc2.GetCount() != 2 {
		t.Errorf("expected 2 entries after reload, got %d", mc2.GetCount())
	}

	// Verify donation info preserved
	e2 := mc2.GetEntry("mn2")
	if e2 == nil {
		t.Fatal("entry 'mn2' not found after reload")
	}
	if e2.DonationAddress != "DonationAddr" {
		t.Errorf("expected donation address 'DonationAddr', got '%s'", e2.DonationAddress)
	}
	if e2.DonationPercent != 50 {
		t.Errorf("expected donation percent 50, got %d", e2.DonationPercent)
	}
}

func TestMasternodeEntry_GetHostPort(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		wantHost string
		wantPort int
	}{
		{"IPv4", "192.168.1.100:37817", "192.168.1.100", 37817},
		{"IPv4 localhost", "127.0.0.1:37817", "127.0.0.1", 37817},
		{"IPv6 full", "[2001:db8::1]:37817", "2001:db8::1", 37817},
		{"IPv6 loopback", "[::1]:37817", "::1", 37817},
		{"IPv6 all zeros", "[::]:37817", "::", 37817},
		{"IPv6 long", "[2001:0db8:85a3:0000:0000:8a2e:0370:7334]:37817", "2001:0db8:85a3:0000:0000:8a2e:0370:7334", 37817},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &MasternodeEntry{IP: tt.ip}
			if got := entry.GetHost(); got != tt.wantHost {
				t.Errorf("GetHost() = %q, want %q", got, tt.wantHost)
			}
			if got := entry.GetPort(); got != tt.wantPort {
				t.Errorf("GetPort() = %d, want %d", got, tt.wantPort)
			}
		})
	}
}

func TestMasternodeConfFile_IPv6(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mnconfig_ipv6_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	confContent := `# IPv6 masternode entries
mn_v4 127.0.0.1:37817 93HaYBVUCYjEMeeH1Y4sBGLALQZE1Yc1K64xiqgX37tGBDQL8Xg 2bcd3c84c84f87eaa86e4e56834c92927a07f9e18718810b92e0d0324456a67c 0
mn_v6 [2001:db8::1]:37817 93HaYBVUCYjEMeeH1Y4sBGLALQZE1Yc1K64xiqgX37tGBDQL8Xg 3bcd3c84c84f87eaa86e4e56834c92927a07f9e18718810b92e0d0324456a67c 1
mn_v6loop [::1]:37817 93HaYBVUCYjEMeeH1Y4sBGLALQZE1Yc1K64xiqgX37tGBDQL8Xg 4bcd3c84c84f87eaa86e4e56834c92927a07f9e18718810b92e0d0324456a67c 2
`
	confPath := filepath.Join(tmpDir, "masternode.conf")
	if err := os.WriteFile(confPath, []byte(confContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	mc := NewMasternodeConfFile(tmpDir, "masternode.conf")
	if err := mc.Read(); err != nil {
		t.Fatalf("Read() failed: %v", err)
	}

	entries := mc.GetEntries()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// IPv4 entry
	if entries[0].GetHost() != "127.0.0.1" {
		t.Errorf("IPv4: expected host '127.0.0.1', got '%s'", entries[0].GetHost())
	}
	if entries[0].GetPort() != 37817 {
		t.Errorf("IPv4: expected port 37817, got %d", entries[0].GetPort())
	}

	// IPv6 entry
	if entries[1].GetHost() != "2001:db8::1" {
		t.Errorf("IPv6: expected host '2001:db8::1', got '%s'", entries[1].GetHost())
	}
	if entries[1].GetPort() != 37817 {
		t.Errorf("IPv6: expected port 37817, got %d", entries[1].GetPort())
	}

	// IPv6 loopback
	if entries[2].GetHost() != "::1" {
		t.Errorf("IPv6 loopback: expected host '::1', got '%s'", entries[2].GetHost())
	}
	if entries[2].GetPort() != 37817 {
		t.Errorf("IPv6 loopback: expected port 37817, got %d", entries[2].GetPort())
	}

	// Test save and reload round-trip
	if err := mc.Save(); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	mc2 := NewMasternodeConfFile(tmpDir, "masternode.conf")
	if err := mc2.Read(); err != nil {
		t.Fatalf("Read() after save failed: %v", err)
	}

	if mc2.GetCount() != 3 {
		t.Fatalf("expected 3 entries after reload, got %d", mc2.GetCount())
	}

	// Verify IPv6 survived round-trip
	e := mc2.GetEntry("mn_v6")
	if e == nil {
		t.Fatal("entry 'mn_v6' not found after reload")
	}
	if e.GetHost() != "2001:db8::1" {
		t.Errorf("after reload: expected host '2001:db8::1', got '%s'", e.GetHost())
	}
	if e.GetPort() != 37817 {
		t.Errorf("after reload: expected port 37817, got %d", e.GetPort())
	}
}

func TestValidatePort(t *testing.T) {
	tests := []struct {
		name      string
		port      int
		isMainnet bool
		wantErr   bool
	}{
		{"mainnet correct port", 37817, true, false},
		{"mainnet wrong port", 37818, true, true},
		{"testnet non-mainnet port", 37818, false, false},
		{"testnet mainnet port", 37817, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePort(tt.port, tt.isMainnet)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePort(%d, %v) error = %v, wantErr %v", tt.port, tt.isMainnet, err, tt.wantErr)
			}
		})
	}
}

func TestMasternodeConfFile_InvalidFormat(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "mnconfig_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name    string
		content string
	}{
		{"too few fields", "mn1 127.0.0.1:37817 privkey"},
		{"invalid txhash length", "mn1 127.0.0.1:37817 privkey abc 0"},
		{"invalid output index", "mn1 127.0.0.1:37817 privkey 2bcd3c84c84f87eaa86e4e56834c92927a07f9e18718810b92e0d0324456a67c notanumber"},
		{"missing port", "mn1 127.0.0.1 privkey 2bcd3c84c84f87eaa86e4e56834c92927a07f9e18718810b92e0d0324456a67c 0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confPath := filepath.Join(tmpDir, "masternode.conf")
			if err := os.WriteFile(confPath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			mc := NewMasternodeConfFile(tmpDir, "masternode.conf")
			if err := mc.Read(); err == nil {
				t.Error("Read() should fail for invalid format")
			}
		})
	}
}
