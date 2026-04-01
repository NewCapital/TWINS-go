package events

import (
	"context"
	"testing"
)

func TestNewEventBridge(t *testing.T) {
	ctx := context.Background()
	bridge := NewEventBridge(ctx)

	if bridge == nil {
		t.Fatal("Expected event bridge, got nil")
	}

	if bridge.ctx != ctx {
		t.Error("Context not set correctly")
	}
}

func TestEventBridge_GetStats(t *testing.T) {
	ctx := context.Background()
	bridge := NewEventBridge(ctx)

	stats := bridge.GetStats()

	if stats.EventsEmitted != 0 {
		t.Errorf("Expected 0 events emitted initially, got %d", stats.EventsEmitted)
	}

	if stats.ActiveListeners != 0 {
		t.Errorf("Expected 0 active listeners initially, got %d", stats.ActiveListeners)
	}
}

func TestEventBridge_RegisterListener(t *testing.T) {
	ctx := context.Background()
	bridge := NewEventBridge(ctx)

	bridge.RegisterListener("test:event")
	bridge.RegisterListener("test:event")
	bridge.RegisterListener("other:event")

	count := bridge.GetListenerCount("test:event")
	if count != 2 {
		t.Errorf("Expected 2 listeners for 'test:event', got %d", count)
	}

	stats := bridge.GetStats()
	if stats.ActiveListeners != 3 {
		t.Errorf("Expected 3 total active listeners, got %d", stats.ActiveListeners)
	}
}

func TestEventBridge_UnregisterListener(t *testing.T) {
	ctx := context.Background()
	bridge := NewEventBridge(ctx)

	bridge.RegisterListener("test:event")
	bridge.RegisterListener("test:event")

	bridge.UnregisterListener("test:event")

	count := bridge.GetListenerCount("test:event")
	if count != 1 {
		t.Errorf("Expected 1 listener after unregister, got %d", count)
	}

	bridge.UnregisterListener("test:event")

	count = bridge.GetListenerCount("test:event")
	if count != 0 {
		t.Errorf("Expected 0 listeners after second unregister, got %d", count)
	}
}

func TestEventBridge_GetAllListeners(t *testing.T) {
	ctx := context.Background()
	bridge := NewEventBridge(ctx)

	bridge.RegisterListener("event1")
	bridge.RegisterListener("event2")
	bridge.RegisterListener("event3")

	listeners := bridge.GetAllListeners()

	if len(listeners) != 3 {
		t.Errorf("Expected 3 listener types, got %d", len(listeners))
	}
}

func TestEventBridge_EmitJSON(t *testing.T) {
	// Skip this test as it requires Wails runtime context
	// which is not available in unit tests
	t.Skip("Skipping EmitJSON test - requires Wails application context")
}

func TestBridgeStats_Structure(t *testing.T) {
	stats := BridgeStats{
		EventsEmitted:   100,
		EventsFailed:    5,
		ActiveListeners: 15,
	}

	if stats.EventsEmitted != 100 {
		t.Errorf("Expected 100 events emitted, got %d", stats.EventsEmitted)
	}

	if stats.ActiveListeners != 15 {
		t.Errorf("Expected 15 active listeners, got %d", stats.ActiveListeners)
	}
}

func TestEventBridge_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	bridge := NewEventBridge(ctx)

	done := make(chan bool)

	// Concurrent registrations
	go func() {
		for i := 0; i < 100; i++ {
			bridge.RegisterListener("test:event")
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 50; i++ {
			bridge.UnregisterListener("test:event")
		}
		done <- true
	}()

	// Wait for goroutines
	<-done
	<-done

	// Should not panic
	count := bridge.GetListenerCount("test:event")
	if count < 0 {
		t.Error("Listener count should not be negative")
	}
}

func TestEventBridge_MultipleEventTypes(t *testing.T) {
	ctx := context.Background()
	bridge := NewEventBridge(ctx)

	events := []string{
		"wallet:balance",
		"tx:new",
		"block:new",
		"stake:reward",
		"masternode:payment",
	}

	for _, event := range events {
		bridge.RegisterListener(event)
	}

	listeners := bridge.GetAllListeners()
	if len(listeners) != len(events) {
		t.Errorf("Expected %d listener types, got %d", len(events), len(listeners))
	}
}

func TestEventBridge_EmitTiming(t *testing.T) {
	// Skip this test as it requires Wails runtime context
	t.Skip("Skipping Emit test - requires Wails application context")
}
