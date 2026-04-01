package main

import (
	"fmt"

	"github.com/twins-dev/twins-core/internal/masternode/debug"
)

// ==========================================
// Masternode Debug Handlers
// ==========================================

// DebugStatusResponse contains the current debug system status.
type DebugStatusResponse struct {
	Enabled    bool             `json:"enabled"`
	Total      int64            `json:"total"`
	ByCategory map[string]int64 `json:"byCategory"`
	FileSize   int64            `json:"fileSize"`
}

// DebugEvent is a frontend-friendly representation of a debug event.
type DebugEvent struct {
	Timestamp string         `json:"timestamp"` // ISO 8601
	Type      string         `json:"type"`
	Category  string         `json:"category"`
	Source    string         `json:"source"`
	Summary   string         `json:"summary"`
	Payload   string         `json:"payload"` // raw JSON string
}

// DebugFilter specifies criteria for querying debug events from the frontend.
type DebugFilter struct {
	Category string `json:"category,omitempty"`
	Type     string `json:"type,omitempty"`
	Source   string `json:"source,omitempty"`
	Search   string `json:"search,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

// GetDebugStatus returns the current debug system status and event counts.
func (a *App) GetDebugStatus() (*DebugStatusResponse, error) {
	collector := a.getDebugCollector()
	if collector == nil {
		return &DebugStatusResponse{
			Enabled:    false,
			ByCategory: make(map[string]int64),
		}, nil
	}

	stats := collector.Stats()
	return &DebugStatusResponse{
		Enabled:    stats.Enabled,
		Total:      stats.Total,
		ByCategory: stats.ByCategory,
		FileSize:   stats.FileSize,
	}, nil
}

// GetDebugEvents queries the debug log with optional filters.
// Returns at most 1000 events.
func (a *App) GetDebugEvents(filter DebugFilter) ([]DebugEvent, error) {
	collector := a.getDebugCollector()
	if collector == nil {
		return []DebugEvent{}, nil
	}

	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}

	events, err := collector.Query(debug.Filter{
		Category: filter.Category,
		Type:     filter.Type,
		Source:   filter.Source,
		Search:   filter.Search,
		Limit:    limit,
		Newest:   true,
	})
	if err != nil {
		return nil, fmt.Errorf("query debug events: %w", err)
	}

	// Query returns newest-first when Newest is set
	result := make([]DebugEvent, 0, len(events))
	for _, e := range events {
		result = append(result, DebugEvent{
			Timestamp: e.Timestamp.Format("2006-01-02T15:04:05.000Z07:00"),
			Type:      e.Type,
			Category:  e.Category,
			Source:    e.Source,
			Summary:   e.Summary,
			Payload:   string(e.Payload),
		})
	}

	return result, nil
}

// GetDebugSummary returns aggregated statistics from all debug events.
func (a *App) GetDebugSummary() (*debug.Summary, error) {
	collector := a.getDebugCollector()
	if collector == nil {
		return &debug.Summary{
			TierBreakdown:     make(map[string]int64),
			RejectReasons:     []debug.ReasonCount{},
			TopSources:        []debug.SourceCount{},
			SyncTransitions:   []debug.StatusTransition{},
			StatusChanges:     []debug.ReasonCount{},
			ActiveMNChanges:   []debug.StatusTransition{},
			PeerDetails:       []debug.PeerDetail{},
			MasternodeDetails: []debug.MasternodeDetail{},
		}, nil
	}

	summary, err := collector.Summary()
	if err != nil {
		return nil, fmt.Errorf("get debug summary: %w", err)
	}
	return summary, nil
}

// ClearDebugLog truncates the current debug log file.
func (a *App) ClearDebugLog() error {
	collector := a.getDebugCollector()
	if collector == nil {
		return fmt.Errorf("debug collector not initialized")
	}

	return collector.Clear()
}

// getDebugCollector returns the debug collector from the node, or nil.
func (a *App) getDebugCollector() *debug.Collector {
	a.componentsMu.RLock()
	node := a.node
	a.componentsMu.RUnlock()

	if node == nil {
		return nil
	}
	return node.DebugCollector
}
