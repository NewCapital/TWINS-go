package events

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// EventBridge bridges Go backend events to the frontend using Wails runtime
type EventBridge struct {
	ctx       context.Context
	mu        sync.RWMutex
	listeners map[string]int
	stats     BridgeStats
}

// BridgeStats tracks event bridge statistics
type BridgeStats struct {
	EventsEmitted   uint64 `json:"events_emitted"`
	EventsFailed    uint64 `json:"events_failed"`
	ActiveListeners int    `json:"active_listeners"`
}

// NewEventBridge creates a new event bridge
func NewEventBridge(ctx context.Context) *EventBridge {
	return &EventBridge{
		ctx:       ctx,
		listeners: make(map[string]int),
	}
}

// Emit emits an event to the frontend
func (eb *EventBridge) Emit(eventName string, data interface{}) error {
	atomic.AddUint64(&eb.stats.EventsEmitted, 1)

	// Emit event to frontend using Wails runtime
	runtime.EventsEmit(eb.ctx, eventName, data)

	return nil
}

// EmitJSON emits an event with JSON-serialized data
func (eb *EventBridge) EmitJSON(eventName string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		atomic.AddUint64(&eb.stats.EventsFailed, 1)
		return err
	}

	return eb.Emit(eventName, string(jsonData))
}

// RegisterListener tracks frontend listeners
func (eb *EventBridge) RegisterListener(eventName string) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	eb.listeners[eventName]++
	eb.updateActiveListeners()
}

// UnregisterListener removes frontend listener tracking
func (eb *EventBridge) UnregisterListener(eventName string) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if count, ok := eb.listeners[eventName]; ok {
		if count > 1 {
			eb.listeners[eventName]--
		} else {
			delete(eb.listeners, eventName)
		}
	}

	eb.updateActiveListeners()
}

// updateActiveListeners updates the active listener count
func (eb *EventBridge) updateActiveListeners() {
	count := 0
	for _, c := range eb.listeners {
		count += c
	}
	eb.stats.ActiveListeners = count
}

// GetStats returns bridge statistics
func (eb *EventBridge) GetStats() BridgeStats {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	return BridgeStats{
		EventsEmitted:   atomic.LoadUint64(&eb.stats.EventsEmitted),
		EventsFailed:    atomic.LoadUint64(&eb.stats.EventsFailed),
		ActiveListeners: eb.stats.ActiveListeners,
	}
}

// GetListenerCount returns the number of listeners for an event
func (eb *EventBridge) GetListenerCount(eventName string) int {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	return eb.listeners[eventName]
}

// GetAllListeners returns all event names with active listeners
func (eb *EventBridge) GetAllListeners() []string {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	names := make([]string, 0, len(eb.listeners))
	for name := range eb.listeners {
		names = append(names, name)
	}

	return names
}
