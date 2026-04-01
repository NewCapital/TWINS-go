// Copyright (c) 2025 The TWINS Core developers
// Distributed under the MIT software license

package spork

import (
	"testing"
	"time"

	"github.com/twins-dev/twins-core/pkg/crypto"
)

// mockStorage implements Storage interface for testing
type mockStorage struct {
	sporks map[int32]*Message
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		sporks: make(map[int32]*Message),
	}
}

func (m *mockStorage) ReadSpork(sporkID int32) (*Message, error) {
	msg, ok := m.sporks[sporkID]
	if !ok {
		return nil, nil
	}
	return msg, nil
}

func (m *mockStorage) WriteSpork(spork *Message) error {
	m.sporks[spork.SporkID] = spork
	return nil
}

func (m *mockStorage) LoadAllSporks() (map[int32]*Message, error) {
	return m.sporks, nil
}

func TestNewManager(t *testing.T) {
	// Generate test keys
	privKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	pubKeyHex := crypto.PublicKeyToHex(privKey.PubKey())

	storage := newMockStorage()
	manager, err := NewManager(pubKeyHex, "", storage)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	if manager == nil {
		t.Fatal("Manager is nil")
	}
}

func TestGetValue(t *testing.T) {
	privKey, _ := crypto.GenerateKey()
	pubKeyHex := crypto.PublicKeyToHex(privKey.PubKey())

	storage := newMockStorage()
	manager, _ := NewManager(pubKeyHex, "", storage)

	// Test default value for SporkMaxValue (numeric value, not timestamp)
	value := manager.GetValue(SporkMaxValue)
	if value != DefaultMaxValue {
		t.Errorf("Expected default value %d, got %d", DefaultMaxValue, value)
	}

	// Test default value for SporkMasternodePayUpdatedNodes (timestamp-based OFF)
	value = manager.GetValue(SporkMasternodePayUpdatedNodes)
	if value != DefaultMasternodePayUpdatedNodes {
		t.Errorf("Expected default value %d, got %d", DefaultMasternodePayUpdatedNodes, value)
	}

	// Test unknown spork
	value = manager.GetValue(99999)
	if value != -1 {
		t.Errorf("Expected -1 for unknown spork, got %d", value)
	}
}

func TestIsActive(t *testing.T) {
	privKey, _ := crypto.GenerateKey()
	pubKeyHex := crypto.PublicKeyToHex(privKey.PubKey())

	storage := newMockStorage()
	manager, _ := NewManager(pubKeyHex, "", storage)

	// SporkMasternodePayUpdatedNodes default is 4070908800 (year 2099) - should be inactive
	if manager.IsActive(SporkMasternodePayUpdatedNodes) {
		t.Error("SporkMasternodePayUpdatedNodes should NOT be active (default OFF)")
	}

	// SporkTwinsEnableMasternodeTiers default is 4070908800 (year 2099), which is in future
	if manager.IsActive(SporkTwinsEnableMasternodeTiers) {
		t.Error("SporkTwinsEnableMasternodeTiers should not be active (default timestamp is in future)")
	}

	// SporkMasternodeScanning default is 978307200 (2001-01-01) - should be active
	if !manager.IsActive(SporkMasternodeScanning) {
		t.Error("SporkMasternodeScanning should be active (default ON)")
	}
}

func TestUpdateSpork(t *testing.T) {
	privKey, _ := crypto.GenerateKey()
	pubKeyHex := crypto.PublicKeyToHex(privKey.PubKey())

	storage := newMockStorage()
	manager, _ := NewManager(pubKeyHex, "", storage)

	// Set private key
	if err := manager.SetPrivateKey(privKey); err != nil {
		t.Fatalf("Failed to set private key: %v", err)
	}

	// Update spork
	newValue := time.Now().Unix() - 3600 // 1 hour ago
	if err := manager.UpdateSpork(SporkMasternodePayUpdatedNodes, newValue); err != nil {
		t.Fatalf("Failed to update spork: %v", err)
	}

	// Verify value was updated
	value := manager.GetValue(SporkMasternodePayUpdatedNodes)
	if value != newValue {
		t.Errorf("Expected value %d, got %d", newValue, value)
	}

	// Verify it was persisted
	stored, err := storage.ReadSpork(SporkMasternodePayUpdatedNodes)
	if err != nil {
		t.Fatalf("Failed to read from storage: %v", err)
	}
	if stored == nil {
		t.Fatal("Spork not found in storage")
	}
	if stored.Value != newValue {
		t.Errorf("Expected stored value %d, got %d", newValue, stored.Value)
	}
}

func TestProcessMessage(t *testing.T) {
	privKey, _ := crypto.GenerateKey()
	pubKeyHex := crypto.PublicKeyToHex(privKey.PubKey())

	storage := newMockStorage()
	manager, _ := NewManager(pubKeyHex, "", storage)

	// Set private key for signing
	if err := manager.SetPrivateKey(privKey); err != nil {
		t.Fatalf("Failed to set private key: %v", err)
	}

	// Create and sign a spork message
	msg := &Message{
		SporkID:    SporkMasternodePayUpdatedNodes,
		Value:      time.Now().Unix(),
		TimeSigned: time.Now().Unix(),
	}

	if err := manager.signMessage(msg, privKey); err != nil {
		t.Fatalf("Failed to sign message: %v", err)
	}

	// Process the message
	if err := manager.ProcessMessage(msg, false); err != nil {
		t.Fatalf("Failed to process message: %v", err)
	}

	// Verify it was accepted
	value := manager.GetValue(SporkMasternodePayUpdatedNodes)
	if value != msg.Value {
		t.Errorf("Expected value %d, got %d", msg.Value, value)
	}
}

func TestProcessMessage_InvalidSignature(t *testing.T) {
	privKey, _ := crypto.GenerateKey()
	pubKeyHex := crypto.PublicKeyToHex(privKey.PubKey())

	storage := newMockStorage()
	manager, _ := NewManager(pubKeyHex, "", storage)

	// Create unsigned message
	msg := &Message{
		SporkID:    SporkMasternodePayUpdatedNodes,
		Value:      time.Now().Unix(),
		TimeSigned: time.Now().Unix(),
		Signature:  make([]byte, 65), // Invalid signature
	}

	// Should reject invalid signature
	err := manager.ProcessMessage(msg, false)
	if err != ErrInvalidSignature {
		t.Errorf("Expected ErrInvalidSignature, got %v", err)
	}
}

func TestProcessMessage_UnknownSpork(t *testing.T) {
	privKey, _ := crypto.GenerateKey()
	pubKeyHex := crypto.PublicKeyToHex(privKey.PubKey())

	storage := newMockStorage()
	manager, _ := NewManager(pubKeyHex, "", storage)

	msg := &Message{
		SporkID:    99999, // Unknown spork
		Value:      time.Now().Unix(),
		TimeSigned: time.Now().Unix(),
	}

	err := manager.ProcessMessage(msg, false)
	if err != ErrUnknownSpork {
		t.Errorf("Expected ErrUnknownSpork, got %v", err)
	}
}

func TestProcessMessage_OlderSpork(t *testing.T) {
	privKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	pubKeyHex := crypto.PublicKeyToHex(privKey.PubKey())

	storage := newMockStorage()
	manager, err := NewManager(pubKeyHex, "", storage)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	if err := manager.SetPrivateKey(privKey); err != nil {
		t.Fatalf("Failed to set private key: %v", err)
	}

	// Add a newer spork
	newer := &Message{
		SporkID:    SporkMasternodePayUpdatedNodes,
		Value:      100,
		TimeSigned: time.Now().Unix(),
	}
	if err := manager.signMessage(newer, privKey); err != nil {
		t.Fatalf("Failed to sign newer message: %v", err)
	}
	if err := manager.ProcessMessage(newer, false); err != nil {
		t.Fatalf("Failed to process newer message: %v", err)
	}

	// Try to add an older spork
	older := &Message{
		SporkID:    SporkMasternodePayUpdatedNodes,
		Value:      50,
		TimeSigned: time.Now().Unix() - 3600, // 1 hour ago
	}
	if err := manager.signMessage(older, privKey); err != nil {
		t.Fatalf("Failed to sign older message: %v", err)
	}

	err = manager.ProcessMessage(older, false)
	if err != ErrOlderSpork {
		t.Errorf("Expected ErrOlderSpork, got %v", err)
	}
}

func TestGetAllSporks(t *testing.T) {
	privKey, _ := crypto.GenerateKey()
	pubKeyHex := crypto.PublicKeyToHex(privKey.PubKey())

	storage := newMockStorage()
	manager, _ := NewManager(pubKeyHex, "", storage)

	sporks := manager.GetAllSporks()
	// Now only 8 active sporks after cleanup (see types.go)
	if len(sporks) != 8 {
		t.Errorf("Expected 8 sporks, got %d", len(sporks))
	}

	// Verify each spork has required fields
	for _, spork := range sporks {
		if spork.Name == "" || spork.Name == "Unknown" {
			t.Errorf("Spork %d has invalid name: %s", spork.ID, spork.Name)
		}
		// Note: DefaultMaxValue is 1000, not 0, so all defaults should be non-zero
		// except for numeric values which can be any non-negative value
	}
}

func TestGetSporkName(t *testing.T) {
	tests := []struct {
		id   int32
		name string
	}{
		{SporkMaxValue, "SPORK_5_MAX_VALUE"},
		{SporkMasternodeScanning, "SPORK_7_MASTERNODE_SCANNING"},
		{SporkMasternodePaymentEnforcement, "SPORK_8_MASTERNODE_PAYMENT_ENFORCEMENT"},
		{SporkMasternodePayUpdatedNodes, "SPORK_10_MASTERNODE_PAY_UPDATED_NODES"},
		{SporkNewProtocolEnforcement, "SPORK_14_NEW_PROTOCOL_ENFORCEMENT"},
		{SporkNewProtocolEnforcement2, "SPORK_15_NEW_PROTOCOL_ENFORCEMENT_2"},
		{SporkTwinsEnableMasternodeTiers, "SPORK_TWINS_01_ENABLE_MASTERNODE_TIERS"},
		{SporkTwinsMinStakeAmount, "SPORK_TWINS_02_MIN_STAKE_AMOUNT"},
		{99999, "Unknown"},
	}

	for _, tt := range tests {
		name := GetSporkName(tt.id)
		if name != tt.name {
			t.Errorf("GetSporkName(%d): expected %s, got %s", tt.id, tt.name, name)
		}
	}
}

func TestGetSporkID(t *testing.T) {
	tests := []struct {
		name string
		id   int32
	}{
		{"SPORK_5_MAX_VALUE", SporkMaxValue},
		{"SPORK_7_MASTERNODE_SCANNING", SporkMasternodeScanning},
		{"SPORK_8_MASTERNODE_PAYMENT_ENFORCEMENT", SporkMasternodePaymentEnforcement},
		{"SPORK_10_MASTERNODE_PAY_UPDATED_NODES", SporkMasternodePayUpdatedNodes},
		{"SPORK_14_NEW_PROTOCOL_ENFORCEMENT", SporkNewProtocolEnforcement},
		{"SPORK_15_NEW_PROTOCOL_ENFORCEMENT_2", SporkNewProtocolEnforcement2},
		{"SPORK_TWINS_01_ENABLE_MASTERNODE_TIERS", SporkTwinsEnableMasternodeTiers},
		{"SPORK_TWINS_02_MIN_STAKE_AMOUNT", SporkTwinsMinStakeAmount},
		{"UNKNOWN_SPORK", -1},
	}

	for _, tt := range tests {
		id := GetSporkID(tt.name)
		if id != tt.id {
			t.Errorf("GetSporkID(%s): expected %d, got %d", tt.name, tt.id, id)
		}
	}
}
