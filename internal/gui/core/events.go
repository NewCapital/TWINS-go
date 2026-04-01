package core

import "time"

// CoreEvent is the interface that all events must implement.
// This mirrors the Boost signals2 pattern from the C++ uiInterface.
// Events are emitted by the core and consumed by the GUI layer.
type CoreEvent interface {
	// EventType returns the event type identifier
	EventType() string

	// Timestamp returns when the event occurred
	Timestamp() time.Time
}

// BaseEvent provides common fields for all events
type BaseEvent struct {
	Type string    `json:"type"`
	Time time.Time `json:"time"`
}

// EventType implements CoreEvent interface
func (e BaseEvent) EventType() string {
	return e.Type
}

// Timestamp implements CoreEvent interface
func (e BaseEvent) Timestamp() time.Time {
	return e.Time
}

// ==========================================
// Core Lifecycle Events
// ==========================================

// InitMessageEvent is emitted during initialization with progress messages.
// Equivalent to: uiInterface.InitMessage
type InitMessageEvent struct {
	BaseEvent
	Message string `json:"message"`
}

// ShowProgressEvent shows progress for long operations (sync, verify, rescan, etc.)
// Equivalent to: uiInterface.ShowProgress
type ShowProgressEvent struct {
	BaseEvent
	Title    string `json:"title"`
	Progress int    `json:"progress"` // 0-100
}

// ==========================================
// Blockchain Events
// ==========================================

// BlockConnectedEvent is emitted when a new block is connected to the chain.
// Equivalent to: uiInterface.NotifyBlockTip
type BlockConnectedEvent struct {
	BaseEvent
	Hash   string `json:"hash"`
	Height int64  `json:"height"`
	Size   int    `json:"size"`
}

// ChainSyncUpdateEvent is emitted during blockchain synchronization.
type ChainSyncUpdateEvent struct {
	BaseEvent
	CurrentHeight int64   `json:"current_height"`
	TargetHeight  int64   `json:"target_height"`
	Progress      float64 `json:"progress"` // 0.0-1.0
}

// ReindexingEvent is emitted when blockchain reindexing progress updates.
type ReindexingEvent struct {
	BaseEvent
	Progress float64 `json:"progress"` // 0.0-1.0
}

// ==========================================
// Network Events
// ==========================================

// ConnectionCountChangedEvent is emitted when the number of network connections changes.
// Equivalent to: uiInterface.NotifyNumConnectionsChanged
type ConnectionCountChangedEvent struct {
	BaseEvent
	Count int `json:"count"`
}

// PeerConnectedEvent is emitted when a new peer connects.
type PeerConnectedEvent struct {
	BaseEvent
	PeerID  int    `json:"peer_id"`
	Address string `json:"address"`
}

// PeerDisconnectedEvent is emitted when a peer disconnects.
type PeerDisconnectedEvent struct {
	BaseEvent
	PeerID  int    `json:"peer_id"`
	Address string `json:"address"`
}

// NetworkActiveChangedEvent is emitted when network activity is enabled/disabled.
type NetworkActiveChangedEvent struct {
	BaseEvent
	Active bool `json:"active"`
}

// ==========================================
// Wallet Events
// ==========================================

// BalanceChangedEvent is emitted when wallet balance changes.
type BalanceChangedEvent struct {
	BaseEvent
	Balance Balance `json:"balance"`
}

// TransactionReceivedEvent is emitted when a new transaction is received.
type TransactionReceivedEvent struct {
	BaseEvent
	TxID          string  `json:"txid"`
	Amount        float64 `json:"amount"`
	Confirmations int     `json:"confirmations"`
}

// TransactionConfirmedEvent is emitted when a transaction gets a new confirmation.
type TransactionConfirmedEvent struct {
	BaseEvent
	TxID          string `json:"txid"`
	Confirmations int    `json:"confirmations"`
}

// TransactionSentEvent is emitted when a transaction is sent.
type TransactionSentEvent struct {
	BaseEvent
	TxID    string  `json:"txid"`
	Amount  float64 `json:"amount"`
	Address string  `json:"address"`
}

// WalletLockedEvent is emitted when the wallet is locked.
type WalletLockedEvent struct {
	BaseEvent
}

// WalletUnlockedEvent is emitted when the wallet is unlocked.
type WalletUnlockedEvent struct {
	BaseEvent
	TimeoutSeconds int `json:"timeout_seconds"`
}

// WalletEncryptedEvent is emitted when the wallet is first encrypted.
type WalletEncryptedEvent struct {
	BaseEvent
}

// WalletPassphraseChangedEvent is emitted when the wallet passphrase is changed.
type WalletPassphraseChangedEvent struct {
	BaseEvent
}

// NewAddressGeneratedEvent is emitted when a new address is generated.
type NewAddressGeneratedEvent struct {
	BaseEvent
	Address string `json:"address"`
	Label   string `json:"label"`
}

// PaymentRequestCreatedEvent is emitted when a payment request is created.
type PaymentRequestCreatedEvent struct {
	BaseEvent
	ID      int64   `json:"id"`
	Address string  `json:"address"`
	Amount  float64 `json:"amount"`
	Label   string  `json:"label"`
	Message string  `json:"message"`
}

// PaymentRequestRemovedEvent is emitted when a payment request is removed.
type PaymentRequestRemovedEvent struct {
	BaseEvent
	ID int64 `json:"id"`
}

// ==========================================
// Alert Events
// ==========================================

// AlertChangedEvent is emitted for network alerts.
// Equivalent to: uiInterface.NotifyAlertChanged
type AlertChangedEvent struct {
	BaseEvent
	Hash    string `json:"hash"`
	Status  string `json:"status"` // new, updated, cancelled
	Message string `json:"message"`
	Severity string `json:"severity"` // info, warning, error
}

// WarningEvent is emitted for general warnings.
type WarningEvent struct {
	BaseEvent
	Warning string `json:"warning"`
}

// ==========================================
// Masternode Events
// ==========================================

// MasternodeListChangedEvent is emitted when the masternode list changes.
type MasternodeListChangedEvent struct {
	BaseEvent
	Count int `json:"count"`
}

// MasternodeStatusChangedEvent is emitted when a masternode status changes.
type MasternodeStatusChangedEvent struct {
	BaseEvent
	Alias  string `json:"alias"`
	Txhash string `json:"txhash"`
	Status string `json:"status"`
}

// MasternodeStartedEvent is emitted when a masternode is started.
type MasternodeStartedEvent struct {
	BaseEvent
	Alias  string `json:"alias"`
	Txhash string `json:"txhash"`
}

// MasternodeStoppedEvent is emitted when a masternode is stopped.
type MasternodeStoppedEvent struct {
	BaseEvent
	Alias  string `json:"alias"`
	Txhash string `json:"txhash"`
}

// MasternodePaymentReceivedEvent is emitted when a masternode payment is received.
type MasternodePaymentReceivedEvent struct {
	BaseEvent
	Alias  string  `json:"alias"`
	TxID   string  `json:"txid"`
	Amount float64 `json:"amount"`
}

// ==========================================
// Staking Events
// ==========================================

// StakingStatusChangedEvent is emitted when staking status changes.
type StakingStatusChangedEvent struct {
	BaseEvent
	Enabled bool `json:"enabled"`
	Staking bool `json:"staking"`
}

// StakeRewardEvent is emitted when a stake reward is received.
type StakeRewardEvent struct {
	BaseEvent
	TxID   string  `json:"txid"`
	Amount float64 `json:"amount"`
	Height int64   `json:"height"`
}

// StakingDifficultyChangedEvent is emitted when staking difficulty changes.
type StakingDifficultyChangedEvent struct {
	BaseEvent
	Difficulty float64 `json:"difficulty"`
}

// ==========================================
// Error Events
// ==========================================

// ErrorEvent is emitted when an error occurs that should be shown to the user.
type ErrorEvent struct {
	BaseEvent
	Error   string `json:"error"`
	Details string `json:"details"`
}

// FatalErrorEvent is emitted for fatal errors that require shutdown.
type FatalErrorEvent struct {
	BaseEvent
	Error   string `json:"error"`
	Details string `json:"details"`
}

// ==========================================
// Backup/Restore Events
// ==========================================

// WalletBackupCompletedEvent is emitted when a wallet backup completes.
type WalletBackupCompletedEvent struct {
	BaseEvent
	Destination string `json:"destination"`
}

// WalletBackupFailedEvent is emitted when a wallet backup fails.
type WalletBackupFailedEvent struct {
	BaseEvent
	Error string `json:"error"`
}

// ==========================================
// Debug Events (for development/testing)
// ==========================================

// DebugEvent is emitted for debugging purposes during development.
type DebugEvent struct {
	BaseEvent
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data"`
}
