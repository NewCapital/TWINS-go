package debug

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCollectorEmit(t *testing.T) {
	dir := t.TempDir()
	c := NewCollector(dir, 1, 3) // 1MB max

	// Emit when disabled should be a no-op
	c.Emit(Event{Type: "test", Category: CategorySync, Summary: "should not appear"})

	// Enable and emit
	if err := c.Enable(); err != nil {
		t.Fatalf("Enable failed: %v", err)
	}
	defer c.Close()

	c.Emit(Event{
		Type:     TypeSyncStateChange,
		Category: CategorySync,
		Source:   "local",
		Summary:  "switched to MasternodeListSyncing",
	})
	c.EmitBroadcast(TypeBroadcastReceived, "192.168.1.1:37817", "received MNB", map[string]any{
		"outpoint": "abc:0",
		"tier":     "Gold",
	})

	// Verify stats (3 total: session_start + sync + broadcast)
	stats := c.Stats()
	if stats.Total != 3 {
		t.Errorf("expected 3 events, got %d", stats.Total)
	}
	if stats.ByCategory[CategorySession] != 1 {
		t.Errorf("expected 1 session event, got %d", stats.ByCategory[CategorySession])
	}
	if stats.ByCategory[CategorySync] != 1 {
		t.Errorf("expected 1 sync event, got %d", stats.ByCategory[CategorySync])
	}
	if stats.ByCategory[CategoryBroadcast] != 1 {
		t.Errorf("expected 1 broadcast event, got %d", stats.ByCategory[CategoryBroadcast])
	}
	if !stats.Enabled {
		t.Error("expected enabled=true")
	}

	// Verify file has content
	if stats.FileSize == 0 {
		t.Error("expected non-zero file size")
	}
}

func TestCollectorQuery(t *testing.T) {
	dir := t.TempDir()
	c := NewCollector(dir, 1, 3)
	if err := c.Enable(); err != nil {
		t.Fatalf("Enable failed: %v", err)
	}
	defer c.Close()

	now := time.Now()

	// Emit events with different categories
	c.Emit(Event{Timestamp: now.Add(-3 * time.Second), Type: TypeSyncStateChange, Category: CategorySync, Source: "local", Summary: "sync started"})
	c.Emit(Event{Timestamp: now.Add(-2 * time.Second), Type: TypeBroadcastReceived, Category: CategoryBroadcast, Source: "192.168.1.1:37817", Summary: "received MNB for gold tier"})
	c.Emit(Event{Timestamp: now.Add(-1 * time.Second), Type: TypePingStageResult, Category: CategoryPing, Source: "10.0.0.1:37817", Summary: "ping accepted"})

	// Query all (4 total: session_start + 3 emitted)
	events, err := c.Query(Filter{})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(events))
	}

	// Query by category
	events, err = c.Query(Filter{Category: CategoryBroadcast})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 broadcast event, got %d", len(events))
	}
	if events[0].Summary != "received MNB for gold tier" {
		t.Errorf("unexpected summary: %s", events[0].Summary)
	}

	// Query by type
	events, err = c.Query(Filter{Type: TypePingStageResult})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 ping event, got %d", len(events))
	}

	// Query by source: session_start (local) + sync event (local) = 2
	events, err = c.Query(Filter{Source: "local"})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 local events (session_start + sync), got %d", len(events))
	}

	// Query by text search (case-insensitive)
	events, err = c.Query(Filter{Search: "GOLD TIER"})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event matching 'GOLD TIER', got %d", len(events))
	}

	// Query with limit
	events, err = c.Query(Filter{Limit: 1})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event with limit, got %d", len(events))
	}
}

func TestCollectorQueryNewest(t *testing.T) {
	dir := t.TempDir()
	c := NewCollector(dir, 1, 3)
	if err := c.Enable(); err != nil {
		t.Fatalf("Enable failed: %v", err)
	}
	defer c.Close()

	now := time.Now()

	// Emit 10 events with sequential timestamps
	for i := 0; i < 10; i++ {
		c.Emit(Event{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Type:      TypeSyncStateChange,
			Category:  CategorySync,
			Source:    "local",
			Summary:   fmt.Sprintf("event-%d", i),
		})
	}

	// Query with Newest=false and Limit=3, filtered to sync category to skip session_start
	events, err := c.Query(Filter{Limit: 3, Newest: false, Category: CategorySync})
	if err != nil {
		t.Fatalf("Query (oldest) failed: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 oldest sync events, got %d", len(events))
	}
	if events[0].Summary != "event-0" {
		t.Errorf("expected oldest event-0, got %s", events[0].Summary)
	}
	if events[2].Summary != "event-2" {
		t.Errorf("expected oldest event-2, got %s", events[2].Summary)
	}

	// Query with Newest=true and Limit=3, filtered to sync category.
	// Results are returned newest-first (reverse chronological order).
	events, err = c.Query(Filter{Limit: 3, Newest: true, Category: CategorySync})
	if err != nil {
		t.Fatalf("Query (newest) failed: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 newest sync events, got %d", len(events))
	}
	if events[0].Summary != "event-9" {
		t.Errorf("expected newest-first event-9, got %s", events[0].Summary)
	}
	if events[2].Summary != "event-7" {
		t.Errorf("expected newest-first event-7 at end, got %s", events[2].Summary)
	}

	// Query with Newest=true and Limit=0 uses the default cap (defaultQueryLimit=1000).
	// With 11 total events (< 1000) all are returned.
	events, err = c.Query(Filter{Newest: true})
	if err != nil {
		t.Fatalf("Query (newest, default limit) failed: %v", err)
	}
	if len(events) != 11 {
		t.Errorf("expected 11 events (all under default limit), got %d", len(events))
	}
}

func TestCollectorRotation(t *testing.T) {
	dir := t.TempDir()
	// Very small max size to trigger rotation quickly
	c := NewCollector(dir, 0, 3) // Will use default 50MB
	// Override to tiny size for testing
	c.maxSizeBytes = 500 // 500 bytes

	if err := c.Enable(); err != nil {
		t.Fatalf("Enable failed: %v", err)
	}
	defer c.Close()

	// Emit enough events to trigger rotation
	for i := 0; i < 20; i++ {
		c.Emit(Event{
			Type:     TypeSyncStateChange,
			Category: CategorySync,
			Source:   "local",
			Summary:  "sync state change event with enough text to fill the buffer quickly",
		})
	}

	// Check that rotated files exist
	base := filepath.Join(dir, "mn-debug")
	if _, err := os.Stat(base + ".1.jsonl"); os.IsNotExist(err) {
		t.Error("expected rotated file .1.jsonl to exist")
	}

	// Current file should exist and be smaller than max
	info, err := os.Stat(filepath.Join(dir, defaultFilename))
	if err != nil {
		t.Fatalf("current file should exist: %v", err)
	}
	if info.Size() > 500 {
		t.Errorf("current file too large after rotation: %d bytes", info.Size())
	}
}

func TestCollectorMaxFiles(t *testing.T) {
	dir := t.TempDir()
	c := NewCollector(dir, 0, 2) // max 2 rotated files
	c.maxSizeBytes = 200

	if err := c.Enable(); err != nil {
		t.Fatalf("Enable failed: %v", err)
	}
	defer c.Close()

	// Emit many events to trigger multiple rotations
	for i := 0; i < 50; i++ {
		c.Emit(Event{
			Type:     TypeSyncStateChange,
			Category: CategorySync,
			Source:   "local",
			Summary:  "rotation test event with some padding text here",
		})
	}

	base := filepath.Join(dir, "mn-debug")

	// Files .1 and .2 should exist (maxFiles=2)
	if _, err := os.Stat(base + ".1.jsonl"); os.IsNotExist(err) {
		t.Error("expected .1.jsonl to exist")
	}
	if _, err := os.Stat(base + ".2.jsonl"); os.IsNotExist(err) {
		t.Error("expected .2.jsonl to exist")
	}

	// File .3 should NOT exist (exceeds maxFiles)
	if _, err := os.Stat(base + ".3.jsonl"); !os.IsNotExist(err) {
		t.Error("expected .3.jsonl to NOT exist (maxFiles=2)")
	}
}

func TestCollectorClear(t *testing.T) {
	dir := t.TempDir()
	c := NewCollector(dir, 1, 3)
	if err := c.Enable(); err != nil {
		t.Fatalf("Enable failed: %v", err)
	}
	defer c.Close()

	c.Emit(Event{Type: TypeSyncStateChange, Category: CategorySync, Source: "local", Summary: "test"})
	c.Emit(Event{Type: TypePingStageResult, Category: CategoryPing, Source: "local", Summary: "test2"})

	if err := c.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	// Stats should be reset
	stats := c.Stats()
	if stats.Total != 0 {
		t.Errorf("expected 0 total after clear, got %d", stats.Total)
	}
	if stats.FileSize != 0 {
		t.Errorf("expected 0 file size after clear, got %d", stats.FileSize)
	}

	// Query should return empty
	events, err := c.Query(Filter{})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events after clear, got %d", len(events))
	}

	// Should still be enabled and accepting events
	c.Emit(Event{Type: TypeSyncStateChange, Category: CategorySync, Source: "local", Summary: "after clear"})
	stats = c.Stats()
	if stats.Total != 1 {
		t.Errorf("expected 1 event after clear+emit, got %d", stats.Total)
	}
}

func TestCollectorDisabledEmit(t *testing.T) {
	dir := t.TempDir()
	c := NewCollector(dir, 1, 3)

	// Not enabled - all emit methods should be no-ops
	c.Emit(Event{Type: "test", Category: "test"})
	c.EmitSync("test", "local", "test", nil)
	c.EmitBroadcast("test", "local", "test", nil)
	c.EmitPing("test", "local", "test", nil)
	c.EmitStatus("test", "local", "test", nil)
	c.EmitWinner("test", "local", "test", nil)
	c.EmitActive("test", "local", "test", nil)
	c.EmitNetwork("test", "local", "test", nil)

	stats := c.Stats()
	if stats.Total != 0 {
		t.Errorf("expected 0 events when disabled, got %d", stats.Total)
	}
	if stats.Enabled {
		t.Error("expected enabled=false")
	}

	// File should not exist
	if _, err := os.Stat(filepath.Join(dir, defaultFilename)); !os.IsNotExist(err) {
		t.Error("expected no file when never enabled")
	}
}

func TestCollectorDoubleEnable(t *testing.T) {
	dir := t.TempDir()
	c := NewCollector(dir, 1, 3)

	// First Enable — should write one session_start and open the file.
	if err := c.Enable(); err != nil {
		t.Fatalf("first Enable failed: %v", err)
	}
	// Second Enable without Disable — early-return path (file still open).
	// Must be a no-op: no second session_start, enabled flag stays true.
	if err := c.Enable(); err != nil {
		t.Fatalf("second Enable failed: %v", err)
	}
	c.Emit(Event{Type: TypeSyncStateChange, Category: CategorySync, Source: "local", Summary: "after double enable"})
	c.Close()

	c2 := NewCollector(dir, 1, 3)
	events, err := c2.Query(Filter{})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	// Exactly 2 events: one session_start (first Enable only) + one user event.
	// The second Enable must NOT emit a second session_start.
	if len(events) != 2 {
		t.Errorf("expected 2 events (1 session_start + 1 user), got %d", len(events))
	}
	if events[0].Type != TypeSessionStart {
		t.Errorf("expected first event to be session_start, got %s", events[0].Type)
	}
	sessionCount := 0
	for _, e := range events {
		if e.Type == TypeSessionStart {
			sessionCount++
		}
	}
	if sessionCount != 1 {
		t.Errorf("expected exactly 1 session_start event, got %d", sessionCount)
	}
}

func TestCollectorEnableDisableToggle(t *testing.T) {
	dir := t.TempDir()
	c := NewCollector(dir, 1, 3)

	// Enable
	if err := c.Enable(); err != nil {
		t.Fatalf("Enable failed: %v", err)
	}
	if !c.IsEnabled() {
		t.Error("expected IsEnabled=true after Enable")
	}

	c.Emit(Event{Type: "test", Category: CategorySync, Source: "local", Summary: "while enabled"})

	// Disable
	c.Disable()
	if c.IsEnabled() {
		t.Error("expected IsEnabled=false after Disable")
	}

	c.Emit(Event{Type: "test", Category: CategorySync, Source: "local", Summary: "while disabled"})

	// Re-enable
	if err := c.Enable(); err != nil {
		t.Fatalf("Re-Enable failed: %v", err)
	}
	c.Emit(Event{Type: "test", Category: CategorySync, Source: "local", Summary: "after re-enable"})
	c.Close()

	// Should have 4 events: session_start (1st Enable) + "while enabled" +
	// session_start (re-Enable) + "after re-enable". The disabled emit is skipped.
	c2 := NewCollector(dir, 1, 3)
	events, err := c2.Query(Filter{})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 4 {
		t.Errorf("expected 4 events (2 session_start + 2 user, disabled emit skipped), got %d", len(events))
	}
}

func TestCollectorEventPayload(t *testing.T) {
	dir := t.TempDir()
	c := NewCollector(dir, 1, 3)
	if err := c.Enable(); err != nil {
		t.Fatalf("Enable failed: %v", err)
	}
	defer c.Close()

	payload := map[string]any{
		"outpoint": "abc123:0",
		"tier":     "Gold",
		"protocol": 70928,
	}
	c.EmitBroadcast(TypeBroadcastAccepted, "10.0.0.1:37817", "accepted MNB", payload)

	events, err := c.Query(Filter{Category: CategoryBroadcast})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	// Verify payload round-trips
	var p map[string]any
	if err := json.Unmarshal(events[0].Payload, &p); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if p["tier"] != "Gold" {
		t.Errorf("expected tier=Gold, got %v", p["tier"])
	}
	if p["outpoint"] != "abc123:0" {
		t.Errorf("expected outpoint=abc123:0, got %v", p["outpoint"])
	}
}

func TestCollectorQueryNoFile(t *testing.T) {
	dir := t.TempDir()
	c := NewCollector(dir, 1, 3)

	// Query without ever enabling (no file exists)
	events, err := c.Query(Filter{})
	if err != nil {
		t.Fatalf("Query should not error on missing file: %v", err)
	}
	if events != nil {
		t.Errorf("expected nil events for missing file, got %d", len(events))
	}
}

func TestCollectorSummary(t *testing.T) {
	dir := t.TempDir()
	c := NewCollector(dir, 1, 3)

	if err := c.Enable(); err != nil {
		t.Fatalf("Enable failed: %v", err)
	}
	defer c.Close()

	// Emit a variety of events to exercise all summary aggregation paths
	c.EmitBroadcast(TypeBroadcastReceived, "10.0.0.1:37817", "received MNB", map[string]any{
		"outpoint": "aaa:0", "tier": "Gold",
	})
	c.EmitBroadcast(TypeBroadcastAccepted, "10.0.0.1:37817", "accepted MNB", map[string]any{
		"outpoint": "aaa:0", "tier": "Gold",
	})
	c.EmitBroadcast(TypeBroadcastReceived, "10.0.0.2:37817", "received MNB", map[string]any{
		"outpoint": "bbb:0", "tier": "Silver",
	})
	c.EmitBroadcast(TypeBroadcastRejected, "10.0.0.2:37817", "rejected MNB", map[string]any{
		"outpoint": "bbb:0", "reason": "bad signature",
	})
	c.EmitBroadcast(TypeBroadcastDedup, "10.0.0.1:37817", "dedup MNB", map[string]any{
		"outpoint": "aaa:0",
	})

	c.EmitPing("ping_received", "10.0.0.1:37817", "ping received", nil)
	c.EmitPing("ping_accepted", "10.0.0.1:37817", "ping accepted", nil)

	c.EmitStatus(TypeStatusUpdate, "aaa:0", "PreEnabled -> Enabled", map[string]any{
		"prev_status": "PreEnabled", "new_status": "Enabled",
	})

	c.EmitActive("active_ping_sent", "local", "ping sent ok", map[string]any{"success": true})
	c.EmitActive("active_ping_sent", "local", "ping sent fail", map[string]any{"success": false})
	c.EmitActive("active_state_change", "local", "state changed", map[string]any{
		"prev_status": "Initial", "new_status": "Started",
	})

	c.EmitNetwork("network_mnb_received", "10.0.0.3:37817", "MNB from net", map[string]any{"outpoint": "ccc:0"})
	c.EmitNetwork("dseg_request", "10.0.0.3:37817", "DSEG request", map[string]any{"payload_size": 100})
	c.EmitNetwork("dseg_response", "10.0.0.3:37817", "DSEG response", map[string]any{"sent_count": float64(500)})

	c.EmitSync(TypeSyncStateChange, "local", "sync state change", map[string]any{
		"prev_state": "Initial", "new_state": "MasternodeListSyncing",
	})

	summary, err := c.Summary()
	if err != nil {
		t.Fatalf("Summary() error: %v", err)
	}

	// Overview
	if summary.SessionCount != 1 {
		t.Errorf("SessionCount = %d, want 1", summary.SessionCount)
	}
	if summary.FirstEvent == "" || summary.LastEvent == "" {
		t.Error("Expected non-empty FirstEvent/LastEvent")
	}

	// Broadcast
	if summary.BroadcastReceived != 2 {
		t.Errorf("BroadcastReceived = %d, want 2", summary.BroadcastReceived)
	}
	if summary.BroadcastAccepted != 1 {
		t.Errorf("BroadcastAccepted = %d, want 1", summary.BroadcastAccepted)
	}
	if summary.BroadcastRejected != 1 {
		t.Errorf("BroadcastRejected = %d, want 1", summary.BroadcastRejected)
	}
	if summary.BroadcastDedup != 1 {
		t.Errorf("BroadcastDedup = %d, want 1", summary.BroadcastDedup)
	}
	if summary.UniqueMasternodes != 2 {
		t.Errorf("UniqueMasternodes = %d, want 2", summary.UniqueMasternodes)
	}
	if summary.TierBreakdown["Gold"] != 1 {
		t.Errorf("TierBreakdown[Gold] = %d, want 1", summary.TierBreakdown["Gold"])
	}
	if len(summary.RejectReasons) == 0 || summary.RejectReasons[0].Label != "bad signature" {
		t.Errorf("Expected 'bad signature' as top reject reason, got %v", summary.RejectReasons)
	}
	if len(summary.TopSources) == 0 {
		t.Error("Expected non-empty TopSources")
	}

	// Ping
	if summary.PingReceived != 1 {
		t.Errorf("PingReceived = %d, want 1", summary.PingReceived)
	}
	if summary.PingAccepted != 1 {
		t.Errorf("PingAccepted = %d, want 1", summary.PingAccepted)
	}

	// Active
	if summary.ActivePingsSent != 2 {
		t.Errorf("ActivePingsSent = %d, want 2", summary.ActivePingsSent)
	}
	if summary.ActivePingsSuccess != 1 {
		t.Errorf("ActivePingsSuccess = %d, want 1", summary.ActivePingsSuccess)
	}
	if summary.ActivePingsFailed != 1 {
		t.Errorf("ActivePingsFailed = %d, want 1", summary.ActivePingsFailed)
	}
	if len(summary.ActiveMNChanges) != 1 {
		t.Errorf("ActiveMNChanges length = %d, want 1", len(summary.ActiveMNChanges))
	}

	// Network
	if summary.NetworkMNBCount != 1 {
		t.Errorf("NetworkMNBCount = %d, want 1", summary.NetworkMNBCount)
	}
	if summary.DSEGRequests != 1 {
		t.Errorf("DSEGRequests = %d, want 1", summary.DSEGRequests)
	}
	if summary.DSEGResponses != 1 {
		t.Errorf("DSEGResponses = %d, want 1", summary.DSEGResponses)
	}
	if summary.AvgMNsServed != 500 {
		t.Errorf("AvgMNsServed = %f, want 500", summary.AvgMNsServed)
	}
	if summary.UniquePeers != 2 {
		t.Errorf("UniquePeers = %d, want 2", summary.UniquePeers)
	}

	// Sync
	if len(summary.SyncTransitions) != 1 {
		t.Errorf("SyncTransitions length = %d, want 1", len(summary.SyncTransitions))
	}

	// Status
	if len(summary.StatusChanges) != 1 {
		t.Errorf("StatusChanges length = %d, want 1", len(summary.StatusChanges))
	}
	if summary.StatusChanges[0].Label != "PreEnabled → Enabled" {
		t.Errorf("StatusChanges[0].Label = %q, want 'PreEnabled → Enabled'", summary.StatusChanges[0].Label)
	}

	// Accept rates
	if summary.AcceptRate <= 0 {
		t.Errorf("AcceptRate = %f, want > 0", summary.AcceptRate)
	}
}

func TestCollectorSummaryNoFile(t *testing.T) {
	dir := t.TempDir()
	c := NewCollector(dir, 1, 3)

	// Summary without ever enabling should return empty summary
	summary, err := c.Summary()
	if err != nil {
		t.Fatalf("Summary should not error on missing file: %v", err)
	}
	if summary.TotalEvents != 0 {
		t.Errorf("expected 0 total events, got %d", summary.TotalEvents)
	}
	if summary.RejectReasons == nil {
		t.Error("expected non-nil RejectReasons slice")
	}
}

func TestMatchesFilter(t *testing.T) {
	now := time.Now()
	event := Event{
		Timestamp: now,
		Type:      TypeBroadcastReceived,
		Category:  CategoryBroadcast,
		Source:    "192.168.1.1:37817",
		Summary:   "Received Gold tier MNB from peer",
	}

	tests := []struct {
		name    string
		filter  Filter
		matches bool
	}{
		{"empty filter matches all", Filter{}, true},
		{"matching category", Filter{Category: CategoryBroadcast}, true},
		{"non-matching category", Filter{Category: CategorySync}, false},
		{"matching type", Filter{Type: TypeBroadcastReceived}, true},
		{"non-matching type", Filter{Type: TypeSyncStateChange}, false},
		{"matching source", Filter{Source: "192.168.1.1:37817"}, true},
		{"non-matching source", Filter{Source: "10.0.0.1:37817"}, false},
		{"matching search", Filter{Search: "gold tier"}, true},
		{"non-matching search", Filter{Search: "platinum"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesFilter(event, tt.filter)
			if got != tt.matches {
				t.Errorf("matchesFilter() = %v, want %v", got, tt.matches)
			}
		})
	}
}

