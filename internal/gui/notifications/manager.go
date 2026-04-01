package notifications

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"sync/atomic"
	"time"
)

// NotificationManager manages notifications
type NotificationManager struct {
	mu            sync.RWMutex
	config        *NotificationConfig
	history       []Notification
	maxHistory    int
	handlers      []NotificationHandler
	totalSent     uint64
	totalDismissed uint64
}

// NotificationConfig holds notification preferences
type NotificationConfig struct {
	Enabled                bool    `json:"enabled"`
	ShowIncomingTx         bool    `json:"show_incoming_tx"`
	ShowOutgoingTx         bool    `json:"show_outgoing_tx"`
	ShowStakingRewards     bool    `json:"show_staking_rewards"`
	ShowMasternodePayments bool    `json:"show_masternode_payments"`
	MinimumAmount          float64 `json:"minimum_amount"`
	ConfirmationsRequired  int     `json:"confirmations_required"`
	PlaySound              bool    `json:"play_sound"`
	ShowDesktopNotif       bool    `json:"show_desktop_notif"`
	AutoDismiss            bool    `json:"auto_dismiss"`
	AutoDismissDelay       int     `json:"auto_dismiss_delay"` // seconds
}

// Notification represents a single notification
type Notification struct {
	ID        string                  `json:"id"`
	Type      NotificationType        `json:"type"`
	Title     string                  `json:"title"`
	Message   string                  `json:"message"`
	Data      interface{}             `json:"data"`
	Severity  NotificationLevel       `json:"severity"`
	Timestamp time.Time               `json:"timestamp"`
	Read      bool                    `json:"read"`
	Dismissed bool                    `json:"dismissed"`
	Actions   []NotificationAction    `json:"actions,omitempty"`
}

// NotificationType represents the type of notification
type NotificationType string

const (
	NotifTypeTransactionIn  NotificationType = "transaction_in"
	NotifTypeTransactionOut NotificationType = "transaction_out"
	NotifTypeStakeReward    NotificationType = "stake_reward"
	NotifTypeMasternodePay  NotificationType = "masternode_payment"
	NotifTypeConfirmation   NotificationType = "confirmation"
	NotifTypeError          NotificationType = "error"
	NotifTypeWarning        NotificationType = "warning"
	NotifTypeInfo           NotificationType = "info"
)

// NotificationLevel represents the severity level
type NotificationLevel string

const (
	LevelInfo    NotificationLevel = "info"
	LevelSuccess NotificationLevel = "success"
	LevelWarning NotificationLevel = "warning"
	LevelError   NotificationLevel = "error"
)

// NotificationAction represents an action button
type NotificationAction struct {
	Label   string `json:"label"`
	Action  string `json:"action"`
	Primary bool   `json:"primary"`
}

// NotificationHandler is a function that handles notifications
type NotificationHandler func(notification Notification)

// NewNotificationManager creates a new notification manager
func NewNotificationManager() *NotificationManager {
	return &NotificationManager{
		config:     DefaultNotificationConfig(),
		history:    make([]Notification, 0),
		maxHistory: 100,
		handlers:   make([]NotificationHandler, 0),
	}
}

// DefaultNotificationConfig returns default notification configuration
func DefaultNotificationConfig() *NotificationConfig {
	return &NotificationConfig{
		Enabled:                true,
		ShowIncomingTx:         true,
		ShowOutgoingTx:         true,
		ShowStakingRewards:     true,
		ShowMasternodePayments: true,
		MinimumAmount:          0.01,
		ConfirmationsRequired:  1,
		PlaySound:              true,
		ShowDesktopNotif:       true,
		AutoDismiss:            true,
		AutoDismissDelay:       10,
	}
}

// Send sends a notification
func (nm *NotificationManager) Send(notif Notification) {
	if !nm.config.Enabled {
		return
	}

	// Generate ID if not provided
	if notif.ID == "" {
		notif.ID = generateNotificationID()
	}

	if notif.Timestamp.IsZero() {
		notif.Timestamp = time.Now()
	}

	// Add to history
	nm.mu.Lock()
	nm.history = append(nm.history, notif)
	if len(nm.history) > nm.maxHistory {
		nm.history = nm.history[1:]
	}
	atomic.AddUint64(&nm.totalSent, 1)
	nm.mu.Unlock()

	// Call handlers
	for _, handler := range nm.handlers {
		go handler(notif)
	}
}

// OnNotification registers a notification handler
func (nm *NotificationManager) OnNotification(handler NotificationHandler) {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	nm.handlers = append(nm.handlers, handler)
}

// GetHistory returns notification history
func (nm *NotificationManager) GetHistory() []Notification {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	return append([]Notification{}, nm.history...)
}

// GetUnread returns unread notifications
func (nm *NotificationManager) GetUnread() []Notification {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	var unread []Notification
	for _, notif := range nm.history {
		if !notif.Read {
			unread = append(unread, notif)
		}
	}
	return unread
}

// MarkRead marks a notification as read
func (nm *NotificationManager) MarkRead(id string) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	for i := range nm.history {
		if nm.history[i].ID == id {
			nm.history[i].Read = true
			break
		}
	}
}

// Dismiss dismisses a notification
func (nm *NotificationManager) Dismiss(id string) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	for i := range nm.history {
		if nm.history[i].ID == id {
			nm.history[i].Dismissed = true
			atomic.AddUint64(&nm.totalDismissed, 1)
			break
		}
	}
}

// ClearAll clears all notifications
func (nm *NotificationManager) ClearAll() {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	nm.history = make([]Notification, 0)
}

// GetConfig returns the current configuration
func (nm *NotificationManager) GetConfig() *NotificationConfig {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	return nm.config
}

// UpdateConfig updates the notification configuration
func (nm *NotificationManager) UpdateConfig(config *NotificationConfig) {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	nm.config = config
}

// GetStats returns notification statistics
func (nm *NotificationManager) GetStats() NotificationStats {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	unreadCount := 0
	for _, n := range nm.history {
		if !n.Read {
			unreadCount++
		}
	}

	return NotificationStats{
		TotalSent:      atomic.LoadUint64(&nm.totalSent),
		TotalDismissed: atomic.LoadUint64(&nm.totalDismissed),
		CurrentCount:   len(nm.history),
		UnreadCount:    unreadCount,
	}
}

// NotificationStats represents notification statistics
type NotificationStats struct {
	TotalSent      uint64 `json:"total_sent"`
	TotalDismissed uint64 `json:"total_dismissed"`
	CurrentCount   int    `json:"current_count"`
	UnreadCount    int    `json:"unread_count"`
}

// generateNotificationID generates a unique notification ID
func generateNotificationID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
