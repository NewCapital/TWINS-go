package notifications

import (
	"testing"
	"time"
)

func TestNewNotificationManager(t *testing.T) {
	nm := NewNotificationManager()

	if nm == nil {
		t.Fatal("Expected notification manager, got nil")
	}

	if nm.maxHistory != 100 {
		t.Errorf("Expected max history 100, got %d", nm.maxHistory)
	}

	if nm.config == nil {
		t.Error("Config should be initialized")
	}
}

func TestDefaultNotificationConfig(t *testing.T) {
	config := DefaultNotificationConfig()

	if !config.Enabled {
		t.Error("Expected Enabled to be true by default")
	}

	if !config.ShowIncomingTx {
		t.Error("Expected ShowIncomingTx to be true by default")
	}

	if config.MinimumAmount != 0.01 {
		t.Errorf("Expected minimum amount 0.01, got %f", config.MinimumAmount)
	}

	if config.AutoDismissDelay != 10 {
		t.Errorf("Expected auto dismiss delay 10, got %d", config.AutoDismissDelay)
	}
}

func TestNotificationManager_Send(t *testing.T) {
	nm := NewNotificationManager()

	notif := Notification{
		Type:     NotifTypeInfo,
		Title:    "Test",
		Message:  "Test message",
		Severity: LevelInfo,
	}

	nm.Send(notif)

	history := nm.GetHistory()
	if len(history) != 1 {
		t.Errorf("Expected 1 notification in history, got %d", len(history))
	}

	if history[0].ID == "" {
		t.Error("Notification ID should be generated")
	}

	if history[0].Timestamp.IsZero() {
		t.Error("Timestamp should be set")
	}
}

func TestNotificationManager_OnNotification(t *testing.T) {
	nm := NewNotificationManager()

	received := false
	var receivedNotif Notification

	nm.OnNotification(func(notif Notification) {
		received = true
		receivedNotif = notif
	})

	testNotif := Notification{
		Type:    NotifTypeInfo,
		Title:   "Test",
		Message: "Test message",
	}

	nm.Send(testNotif)

	time.Sleep(50 * time.Millisecond)

	if !received {
		t.Error("Notification handler was not called")
	}

	if receivedNotif.Title != "Test" {
		t.Errorf("Expected title 'Test', got '%s'", receivedNotif.Title)
	}
}

func TestNotificationManager_GetUnread(t *testing.T) {
	nm := NewNotificationManager()

	nm.Send(Notification{Type: NotifTypeInfo, Title: "1", Message: "Msg1"})
	nm.Send(Notification{Type: NotifTypeInfo, Title: "2", Message: "Msg2"})
	nm.Send(Notification{Type: NotifTypeInfo, Title: "3", Message: "Msg3"})

	unread := nm.GetUnread()
	if len(unread) != 3 {
		t.Errorf("Expected 3 unread notifications, got %d", len(unread))
	}

	// Mark one as read
	nm.MarkRead(unread[0].ID)

	unread = nm.GetUnread()
	if len(unread) != 2 {
		t.Errorf("Expected 2 unread after marking one read, got %d", len(unread))
	}
}

func TestNotificationManager_MarkRead(t *testing.T) {
	nm := NewNotificationManager()

	nm.Send(Notification{Type: NotifTypeInfo, Title: "Test", Message: "Msg"})

	history := nm.GetHistory()
	id := history[0].ID

	if history[0].Read {
		t.Error("Notification should not be read initially")
	}

	nm.MarkRead(id)

	history = nm.GetHistory()
	if !history[0].Read {
		t.Error("Notification should be marked as read")
	}
}

func TestNotificationManager_Dismiss(t *testing.T) {
	nm := NewNotificationManager()

	nm.Send(Notification{Type: NotifTypeInfo, Title: "Test", Message: "Msg"})

	history := nm.GetHistory()
	id := history[0].ID

	if history[0].Dismissed {
		t.Error("Notification should not be dismissed initially")
	}

	nm.Dismiss(id)

	history = nm.GetHistory()
	if !history[0].Dismissed {
		t.Error("Notification should be dismissed")
	}
}

func TestNotificationManager_ClearAll(t *testing.T) {
	nm := NewNotificationManager()

	nm.Send(Notification{Type: NotifTypeInfo, Title: "1", Message: "Msg1"})
	nm.Send(Notification{Type: NotifTypeInfo, Title: "2", Message: "Msg2"})

	if len(nm.GetHistory()) != 2 {
		t.Error("Expected 2 notifications before clear")
	}

	nm.ClearAll()

	if len(nm.GetHistory()) != 0 {
		t.Error("Expected 0 notifications after clear")
	}
}

func TestNotificationManager_MaxHistory(t *testing.T) {
	nm := NewNotificationManager()
	nm.maxHistory = 5

	// Send more than max
	for i := 0; i < 10; i++ {
		nm.Send(Notification{Type: NotifTypeInfo, Title: "Test", Message: "Msg"})
	}

	history := nm.GetHistory()
	if len(history) != 5 {
		t.Errorf("Expected history limited to 5, got %d", len(history))
	}
}

func TestNotificationManager_UpdateConfig(t *testing.T) {
	nm := NewNotificationManager()

	newConfig := &NotificationConfig{
		Enabled:               false,
		ShowIncomingTx:        false,
		MinimumAmount:         1.0,
		ConfirmationsRequired: 6,
	}

	nm.UpdateConfig(newConfig)

	config := nm.GetConfig()
	if config.Enabled {
		t.Error("Config should be updated")
	}

	if config.MinimumAmount != 1.0 {
		t.Errorf("Expected minimum amount 1.0, got %f", config.MinimumAmount)
	}
}

func TestNotificationManager_GetStats(t *testing.T) {
	nm := NewNotificationManager()

	nm.Send(Notification{Type: NotifTypeInfo, Title: "1", Message: "Msg1"})
	nm.Send(Notification{Type: NotifTypeInfo, Title: "2", Message: "Msg2"})

	stats := nm.GetStats()

	if stats.TotalSent != 2 {
		t.Errorf("Expected 2 sent, got %d", stats.TotalSent)
	}

	if stats.CurrentCount != 2 {
		t.Errorf("Expected 2 current, got %d", stats.CurrentCount)
	}

	if stats.UnreadCount != 2 {
		t.Errorf("Expected 2 unread, got %d", stats.UnreadCount)
	}

	// Dismiss one
	history := nm.GetHistory()
	nm.Dismiss(history[0].ID)

	stats = nm.GetStats()
	if stats.TotalDismissed != 1 {
		t.Errorf("Expected 1 dismissed, got %d", stats.TotalDismissed)
	}
}

func TestNotification_Structure(t *testing.T) {
	notif := Notification{
		ID:        "id123",
		Type:      NotifTypeTransactionIn,
		Title:     "Incoming TX",
		Message:   "Received 10 TWINS",
		Severity:  LevelSuccess,
		Timestamp: time.Now(),
		Read:      false,
		Dismissed: false,
		Actions: []NotificationAction{
			{Label: "View", Action: "view_tx:123", Primary: true},
		},
	}

	if notif.Type != NotifTypeTransactionIn {
		t.Errorf("Expected type transaction_in, got %s", notif.Type)
	}

	if len(notif.Actions) != 1 {
		t.Errorf("Expected 1 action, got %d", len(notif.Actions))
	}

	if !notif.Actions[0].Primary {
		t.Error("Expected primary action")
	}
}

func TestNotificationTypes(t *testing.T) {
	types := []NotificationType{
		NotifTypeTransactionIn,
		NotifTypeTransactionOut,
		NotifTypeStakeReward,
		NotifTypeMasternodePay,
		NotifTypeConfirmation,
		NotifTypeError,
		NotifTypeWarning,
		NotifTypeInfo,
	}

	for _, typ := range types {
		if typ == "" {
			t.Error("Notification type should not be empty")
		}
	}
}

func TestNotificationLevels(t *testing.T) {
	levels := []NotificationLevel{
		LevelInfo,
		LevelSuccess,
		LevelWarning,
		LevelError,
	}

	for _, level := range levels {
		if level == "" {
			t.Error("Notification level should not be empty")
		}
	}
}

func TestGenerateNotificationID(t *testing.T) {
	id1 := generateNotificationID()
	id2 := generateNotificationID()

	if id1 == "" {
		t.Error("ID should not be empty")
	}

	if id1 == id2 {
		t.Error("IDs should be unique")
	}

	if len(id1) != 32 { // 16 bytes hex encoded = 32 chars
		t.Errorf("Expected ID length 32, got %d", len(id1))
	}
}
