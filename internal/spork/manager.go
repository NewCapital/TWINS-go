// Copyright (c) 2025 The TWINS Core developers
// Distributed under the MIT software license

package spork

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/twins-dev/twins-core/pkg/crypto"
)

var (
	// ErrInvalidSignature indicates the spork signature is invalid
	ErrInvalidSignature = errors.New("invalid spork signature")

	// ErrUnknownSpork indicates an unknown spork ID
	ErrUnknownSpork = errors.New("unknown spork ID")

	// ErrOlderSpork indicates the received spork is older than cached
	ErrOlderSpork = errors.New("spork is older than cached version")
)

// SporkKeyTransitionWindow is the duration in seconds during which the legacy
// public key remains valid after key rotation (30 days).
const SporkKeyTransitionWindow int64 = 30 * 24 * 60 * 60

// isValidSporkID returns true if id is a known, active spork ID.
func isValidSporkID(id int32) bool {
	return getDefaultValue(id) != -1
}

// Manager handles spork state, validation, and broadcasting
type Manager struct {
	mu sync.RWMutex

	// Active sporks by ID (most recent valid spork for each ID)
	active map[int32]*Message

	// All received sporks by hash (for relay/history)
	received map[[32]byte]*Message

	// Spork signing key (only for spork administrators)
	privKey *btcec.PrivateKey

	// Network spork public key for signature verification
	sporkPubKey *btcec.PublicKey

	// Legacy spork public key (for old signature validation window)
	sporkPubKeyOld *btcec.PublicKey

	// Storage for persistence
	storage Storage

	// Relay function for broadcasting sporks to peers
	relayFunc func(msg *Message)
}

// NewManager creates a new spork manager
func NewManager(sporkPubKeyHex string, sporkPubKeyOldHex string, storage Storage) (*Manager, error) {
	sporkPubKey, err := crypto.ParsePublicKeyHex(sporkPubKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid spork public key: %w", err)
	}

	var sporkPubKeyOld *btcec.PublicKey
	if sporkPubKeyOldHex != "" {
		sporkPubKeyOld, err = crypto.ParsePublicKeyHex(sporkPubKeyOldHex)
		if err != nil {
			return nil, fmt.Errorf("invalid old spork public key: %w", err)
		}
	}

	m := &Manager{
		active:         make(map[int32]*Message),
		received:       make(map[[32]byte]*Message),
		sporkPubKey:    sporkPubKey,
		sporkPubKeyOld: sporkPubKeyOld,
		storage:        storage,
	}

	// Load sporks from database
	if err := m.LoadFromStorage(); err != nil {
		return nil, fmt.Errorf("failed to load sporks: %w", err)
	}

	return m, nil
}

// SetRelayFunc sets the function used to relay sporks to peers
func (m *Manager) SetRelayFunc(f func(msg *Message)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.relayFunc = f
}

// SetPrivateKey sets the private key for signing sporks (admin only)
func (m *Manager) SetPrivateKey(privKey *btcec.PrivateKey) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Test signing to validate the key
	testMsg := &Message{
		SporkID:    SporkMaxValue,
		Value:      0,
		TimeSigned: time.Now().Unix(),
	}

	if err := m.signMessage(testMsg, privKey); err != nil {
		return fmt.Errorf("failed to sign test message: %w", err)
	}

	// Verify with current spork public key
	if !m.verifySignature(testMsg, m.sporkPubKey) {
		return errors.New("private key does not match spork public key")
	}

	m.privKey = privKey
	return nil
}

// ProcessMessage validates and processes a spork message from the network
func (m *Manager) ProcessMessage(msg *Message, checkSigner bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate spork ID
	if !isValidSporkID(msg.SporkID) {
		return ErrUnknownSpork
	}

	// Check if we already have a newer spork for this ID
	if existing, ok := m.active[msg.SporkID]; ok {
		if existing.TimeSigned >= msg.TimeSigned {
			return ErrOlderSpork
		}
	}

	// Verify signature
	if !m.checkSignature(msg, checkSigner) {
		return ErrInvalidSignature
	}

	return m.storeAndRelayLocked(msg)
}

// GetValue returns the current value of a spork (or default if not set)
func (m *Manager) GetValue(sporkID int32) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if msg, ok := m.active[sporkID]; ok {
		return msg.Value
	}

	return getDefaultValue(sporkID)
}

// IsActive checks if a spork is currently active
// Sporks are "active" if their value (as timestamp) < current time
func (m *Manager) IsActive(sporkID int32) bool {
	value := m.GetValue(sporkID)
	if value == -1 {
		return false
	}
	return value < time.Now().Unix()
}

// GetAllSporks returns information about all known sporks
func (m *Manager) GetAllSporks() []SporkInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := getAllSporkIDs()
	now := time.Now().Unix()
	result := make([]SporkInfo, 0, len(ids))

	for _, id := range ids {
		defaultVal := getDefaultValue(id)
		value := defaultVal
		var timeSigned time.Time

		if msg, ok := m.active[id]; ok {
			value = msg.Value
			timeSigned = time.Unix(msg.TimeSigned, 0)
		}

		active := value != -1 && value < now

		result = append(result, SporkInfo{
			ID:         id,
			Name:       GetSporkName(id),
			Value:      value,
			DefaultVal: defaultVal,
			Active:     active,
			TimeSigned: timeSigned,
		})
	}

	return result
}

// UpdateSpork creates and signs a new spork update (requires private key)
func (m *Manager) UpdateSpork(sporkID int32, value int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.privKey == nil {
		return errors.New("no private key set - cannot sign spork")
	}

	msg := &Message{
		SporkID:    sporkID,
		Value:      value,
		TimeSigned: time.Now().Unix(),
	}

	if err := m.signMessage(msg, m.privKey); err != nil {
		return fmt.Errorf("failed to sign spork: %w", err)
	}

	return m.storeAndRelayLocked(msg)
}

// LoadFromStorage loads all sporks from persistent storage
func (m *Manager) LoadFromStorage() error {
	if m.storage == nil {
		return nil
	}

	sporks, err := m.storage.LoadAllSporks()
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for id, msg := range sporks {
		hash := msg.Hash()
		m.received[hash] = msg
		m.active[id] = msg
	}

	return nil
}

// storeAndRelayLocked persists and broadcasts a validated spork message.
// Caller must hold m.mu.Lock().
func (m *Manager) storeAndRelayLocked(msg *Message) error {
	hash := msg.Hash()
	m.received[hash] = msg
	m.active[msg.SporkID] = msg

	if m.storage != nil {
		if err := m.storage.WriteSpork(msg); err != nil {
			return fmt.Errorf("failed to save spork: %w", err)
		}
	}

	if m.relayFunc != nil {
		m.relayFunc(msg)
	}

	return nil
}

// checkSignature verifies a spork message signature
func (m *Manager) checkSignature(msg *Message, requireNew bool) bool {
	// Try new key first
	if m.verifySignature(msg, m.sporkPubKey) {
		return true
	}

	if requireNew {
		return false
	}

	// Allow old key verification in transition window
	// The transition window is a grace period where both old and new spork keys are valid
	// This allows for network-wide key rotation without disruption
	if m.sporkPubKeyOld != nil {
		currentTime := time.Now().Unix()
		sporkAge := currentTime - msg.TimeSigned

		// Only accept old key signatures within the transition window
		if sporkAge >= 0 && sporkAge <= SporkKeyTransitionWindow {
			return m.verifySignature(msg, m.sporkPubKeyOld)
		}
	}

	return false
}

// verifySignature verifies message signature with given public key
func (m *Manager) verifySignature(msg *Message, pubKey *btcec.PublicKey) bool {
	// Spork signatures should be 65-byte compact signatures
	if len(msg.Signature) != 65 {
		return false
	}

	sigMsg := msg.SignatureMessage()
	// Use VerifyCompactSignature for spork messages (legacy compatibility)
	cryptoPubKey := crypto.NewPublicKeyFromBTCEC(pubKey)
	verified, err := crypto.VerifyCompactSignature(cryptoPubKey, sigMsg, msg.Signature)
	if err != nil {
		return false
	}
	return verified
}

// signMessage signs a spork message with the given private key
func (m *Manager) signMessage(msg *Message, privKey *btcec.PrivateKey) error {
	sigMsg := msg.SignatureMessage()
	// Use SignCompact for spork messages (legacy compatibility)
	cryptoPrivKey := crypto.NewPrivateKeyFromBTCEC(privKey)
	signature, err := crypto.SignCompact(cryptoPrivKey, sigMsg)
	if err != nil {
		return err
	}

	msg.Signature = signature
	return nil
}

// getDefaultValue returns the default value for a spork ID
func getDefaultValue(sporkID int32) int64 {
	switch sporkID {
	case SporkMaxValue:
		return DefaultMaxValue
	case SporkMasternodeScanning:
		return DefaultMasternodeScanning
	case SporkMasternodePaymentEnforcement:
		return DefaultMasternodePaymentEnforcement
	case SporkMasternodePayUpdatedNodes:
		return DefaultMasternodePayUpdatedNodes
	case SporkNewProtocolEnforcement:
		return DefaultNewProtocolEnforcement
	case SporkNewProtocolEnforcement2:
		return DefaultNewProtocolEnforcement2
	case SporkTwinsEnableMasternodeTiers:
		return DefaultTwinsEnableMasternodeTiers
	case SporkTwinsMinStakeAmount:
		return DefaultTwinsMinStakeAmount
	default:
		return -1
	}
}

// getAllSporkIDs returns all valid spork IDs
func getAllSporkIDs() []int32 {
	return []int32{
		SporkMaxValue,
		SporkMasternodeScanning,
		SporkMasternodePaymentEnforcement,
		SporkMasternodePayUpdatedNodes,
		SporkNewProtocolEnforcement,
		SporkNewProtocolEnforcement2,
		SporkTwinsEnableMasternodeTiers,
		SporkTwinsMinStakeAmount,
	}
}