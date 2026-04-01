package debug

import (
	"bufio"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultMaxSizeMB = 50
	defaultMaxFiles  = 3
	defaultFilename  = "mn-debug.jsonl"
	defaultQueryLimit = 1000
)

// Collector captures masternode debug events to JSONL files.
// Uses atomic.Bool for zero-cost disabled check — callers should check
// IsEnabled() before constructing event payloads.
//
// Lock ordering: statsMu is a leaf lock — never acquires mu while held.
// mu may acquire statsMu (Emit, Clear). Stats() only acquires statsMu.
type Collector struct {
	enabled atomic.Bool

	mu       sync.Mutex
	file     *os.File
	writer   *bufio.Writer
	filePath string
	dataDir  string

	maxSizeBytes int64
	maxFiles     int

	// statsMu protects stats and currentSize together.
	// Lock ordering: mu → statsMu (never reverse).
	statsMu     sync.RWMutex
	stats       Stats
	currentSize int64
}

// NewCollector creates a new debug event collector.
// The collector starts disabled; call Enable() to begin collecting.
func NewCollector(dataDir string, maxSizeMB, maxFiles int) *Collector {
	if maxSizeMB <= 0 {
		maxSizeMB = defaultMaxSizeMB
	}
	if maxFiles <= 0 {
		maxFiles = defaultMaxFiles
	}

	return &Collector{
		dataDir:      dataDir,
		filePath:     filepath.Join(dataDir, defaultFilename),
		maxSizeBytes: int64(maxSizeMB) * 1024 * 1024,
		maxFiles:     maxFiles,
		stats: Stats{
			ByCategory: make(map[string]int64),
		},
	}
}

// IsEnabled returns true if debug collection is active.
// This is the fast-path check — use before constructing event data.
func (c *Collector) IsEnabled() bool {
	return c.enabled.Load()
}

// Enable starts debug collection, opening the JSONL file for writing.
func (c *Collector) Enable() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.file != nil {
		// Already enabled
		c.enabled.Store(true)
		return nil
	}

	f, err := os.OpenFile(c.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("debug collector: failed to open %s: %w", c.filePath, err)
	}

	info, _ := f.Stat()
	if info != nil {
		c.statsMu.Lock()
		c.currentSize = info.Size()
		c.statsMu.Unlock()
	}

	c.file = f
	c.writer = bufio.NewWriterSize(f, 64*1024) // 64KB write buffer
	c.enabled.Store(true)

	// Write session start marker directly (we hold c.mu, cannot call Emit()).
	sessionEvent := Event{
		Timestamp: time.Now(),
		Type:      TypeSessionStart,
		Category:  CategorySession,
		Source:    "local",
		Summary:   "Debug session started",
	}
	if data, err := json.Marshal(sessionEvent); err == nil {
		data = append(data, '\n')
		if n, err := c.writer.Write(data); err == nil {
			c.writer.Flush()
			c.statsMu.Lock()
			c.currentSize += int64(n)
			c.stats.Total++
			c.stats.ByCategory[sessionEvent.Category]++
			c.statsMu.Unlock()
		}
	}

	return nil
}

// Disable stops debug collection and closes the file.
func (c *Collector) Disable() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.enabled.Store(false)
	c.closeFileLocked()
}

// Emit writes a debug event to the JSONL file.
// Returns immediately if collection is disabled (zero-cost atomic check).
func (c *Collector) Emit(event Event) {
	if !c.enabled.Load() {
		return
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	data = append(data, '\n')

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.writer == nil {
		return
	}

	n, err := c.writer.Write(data)
	if err != nil {
		return
	}

	// Flush periodically (every ~4KB of buffered data)
	if c.writer.Buffered() > 4096 {
		c.writer.Flush()
	}

	// Update stats and currentSize together under statsMu
	c.statsMu.Lock()
	c.currentSize += int64(n)
	c.stats.Total++
	c.stats.ByCategory[event.Category]++
	needsRotation := c.currentSize >= c.maxSizeBytes
	c.statsMu.Unlock()

	// Check if rotation is needed
	if needsRotation {
		c.rotateLocked()
	}
}

// EmitSync emits a sync-category event.
func (c *Collector) EmitSync(eventType, source, summary string, payload map[string]any) {
	if !c.enabled.Load() {
		return
	}
	c.emitWithPayload(CategorySync, eventType, source, summary, payload)
}

// EmitBroadcast emits a broadcast-category event.
func (c *Collector) EmitBroadcast(eventType, source, summary string, payload map[string]any) {
	if !c.enabled.Load() {
		return
	}
	c.emitWithPayload(CategoryBroadcast, eventType, source, summary, payload)
}

// EmitPing emits a ping-category event.
func (c *Collector) EmitPing(eventType, source, summary string, payload map[string]any) {
	if !c.enabled.Load() {
		return
	}
	c.emitWithPayload(CategoryPing, eventType, source, summary, payload)
}

// EmitStatus emits a status-category event.
func (c *Collector) EmitStatus(eventType, source, summary string, payload map[string]any) {
	if !c.enabled.Load() {
		return
	}
	c.emitWithPayload(CategoryStatus, eventType, source, summary, payload)
}

// EmitWinner emits a winner-category event.
func (c *Collector) EmitWinner(eventType, source, summary string, payload map[string]any) {
	if !c.enabled.Load() {
		return
	}
	c.emitWithPayload(CategoryWinner, eventType, source, summary, payload)
}

// EmitActive emits an active-masternode-category event.
func (c *Collector) EmitActive(eventType, source, summary string, payload map[string]any) {
	if !c.enabled.Load() {
		return
	}
	c.emitWithPayload(CategoryActive, eventType, source, summary, payload)
}

// EmitNetwork emits a network/P2P-category event.
func (c *Collector) EmitNetwork(eventType, source, summary string, payload map[string]any) {
	if !c.enabled.Load() {
		return
	}
	c.emitWithPayload(CategoryNetwork, eventType, source, summary, payload)
}

func (c *Collector) emitWithPayload(category, eventType, source, summary string, payload map[string]any) {
	var raw json.RawMessage
	if payload != nil {
		data, err := json.Marshal(payload)
		if err == nil {
			raw = data
		}
	}
	c.Emit(Event{
		Timestamp: time.Now(),
		Type:      eventType,
		Category:  category,
		Source:    source,
		Summary:   summary,
		Payload:   raw,
	})
}

// Stats returns current event statistics.
func (c *Collector) Stats() Stats {
	c.statsMu.RLock()
	stats := Stats{
		Total:      c.stats.Total,
		ByCategory: make(map[string]int64, len(c.stats.ByCategory)),
		Enabled:    c.enabled.Load(),
		FileSize:   c.currentSize,
	}
	maps.Copy(stats.ByCategory, c.stats.ByCategory)
	c.statsMu.RUnlock()

	return stats
}

// mnAccumHelper tracks per-masternode event counts during summary computation.
type mnAccumHelper struct {
	addr  string
	tier  string
	count int64
}

// Summary reads all events from the JSONL file and computes aggregated statistics.
func (c *Collector) Summary() (*Summary, error) {
	c.mu.Lock()
	if c.writer != nil {
		c.writer.Flush()
	}
	filePath := c.filePath
	c.mu.Unlock()

	s := &Summary{
		TierBreakdown: make(map[string]int64),
	}

	// Get file size from stats
	c.statsMu.RLock()
	s.FileSize = c.currentSize
	s.TotalEvents = c.stats.Total
	c.statsMu.RUnlock()

	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			s.RejectReasons = []ReasonCount{}
			s.TopSources = []SourceCount{}
			s.SyncTransitions = []StatusTransition{}
			s.StatusChanges = []ReasonCount{}
			s.ActiveMNChanges = []StatusTransition{}
			return s, nil
		}
		return nil, fmt.Errorf("debug collector: failed to open for summary: %w", err)
	}
	defer f.Close()

	// Tracking maps
	uniqueOutpoints := make(map[string]struct{})
	sourceCounts := make(map[string]int64)
	peerEventCounts := make(map[string]int64)

	mnEventCounts := make(map[string]*mnAccumHelper)
	rejectReasons := make(map[string]int64)
	statusChangeCounts := make(map[string]int64)
	var dsegTotalServed int64

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}

		ts := event.Timestamp.Format("2006-01-02T15:04:05.000Z07:00")

		// Track time range
		if s.FirstEvent == "" {
			s.FirstEvent = ts
		}
		s.LastEvent = ts

		// Track unique peers (non-local sources, excluding broadcast category and outpoints)
		if event.Source != "" && event.Source != "local" {
			if event.Category != CategoryBroadcast && isNetworkAddress(event.Source) {
				peerEventCounts[event.Source]++
			}
		}

		// Parse payload once
		var payload map[string]any
		if len(event.Payload) > 0 {
			json.Unmarshal(event.Payload, &payload)
		}

		switch event.Category {
		case CategorySession:
			if event.Type == TypeSessionStart {
				s.SessionCount++
			}

		case CategoryBroadcast:
			if event.Source != "" && event.Source != "local" {
				sourceCounts[event.Source]++
			}
			// Track MN outpoint across ALL broadcast event types
			if outpoint, ok := payload["outpoint"].(string); ok {
				uniqueOutpoints[outpoint] = struct{}{}
				tier, _ := payload["tier"].(string)
				addr, _ := payload["payee"].(string)
				if acc, exists := mnEventCounts[outpoint]; exists {
					acc.count++
					if tier != "" && acc.tier == "" {
						acc.tier = tier
					}
					if addr != "" && acc.addr == "" {
						acc.addr = addr
					}
				} else {
					mnEventCounts[outpoint] = &mnAccumHelper{addr: addr, tier: tier, count: 1}
				}
			}
			switch event.Type {
			case TypeBroadcastReceived:
				s.BroadcastReceived++
			case TypeBroadcastAccepted:
				s.BroadcastAccepted++
				if tier, ok := payload["tier"].(string); ok && tier != "" {
					s.TierBreakdown[tier]++
				}
			case TypeBroadcastRejected:
				s.BroadcastRejected++
				if reason, ok := payload["reason"].(string); ok && reason != "" {
					rejectReasons[reason]++
				} else if errStr, ok := payload["error"].(string); ok && errStr != "" {
					rejectReasons[errStr]++
				}
			case TypeBroadcastDedup:
				s.BroadcastDedup++
			}

		case CategoryPing:
			switch event.Type {
			case "ping_received":
				s.PingReceived++
			case "ping_accepted":
				s.PingAccepted++
			default:
				// Any other ping event type is a failure
				if event.Type != TypePingStageResult {
					s.PingFailed++
				}
			}

		case CategoryStatus:
			if event.Type == TypeStatusUpdate {
				label := event.Summary
				if prevStatus, ok := payload["prev_status"].(string); ok {
					if newStatus, ok2 := payload["new_status"].(string); ok2 {
						label = prevStatus + " → " + newStatus
					}
				}
				statusChangeCounts[label]++
			}

		case CategoryWinner:
			// winner events tracked by count only (already in Stats)

		case CategoryActive:
			switch event.Type {
			case "active_ping_sent":
				s.ActivePingsSent++
				if success, ok := payload["success"].(bool); ok {
					if success {
						s.ActivePingsSuccess++
					} else {
						s.ActivePingsFailed++
					}
				}
			case "active_state_change":
				from, _ := payload["prev_status"].(string)
				to, _ := payload["new_status"].(string)
				s.ActiveMNChanges = append(s.ActiveMNChanges, StatusTransition{
					Timestamp: ts,
					From:      from,
					To:        to,
				})
			}

		case CategoryNetwork:
			switch event.Type {
			case "network_mnb_received":
				s.NetworkMNBCount++
			case "network_mnp_received":
				s.NetworkMNPCount++
			case "dseg_request":
				s.DSEGRequests++
			case "dseg_response":
				s.DSEGResponses++
				if sentCount, ok := payload["sent_count"].(float64); ok {
					dsegTotalServed += int64(sentCount)
				}
			}

		case CategorySync:
			if event.Type == TypeSyncStateChange {
				from, _ := payload["prev_state"].(string)
				to, _ := payload["new_state"].(string)
				s.SyncTransitions = append(s.SyncTransitions, StatusTransition{
					Timestamp: ts,
					From:      from,
					To:        to,
				})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("debug collector: summary scan error: %w", err)
	}

	// Compute derived values
	s.UniqueMasternodes = int64(len(uniqueOutpoints))
	s.UniquePeers = int64(len(peerEventCounts))

	total := s.BroadcastReceived + s.BroadcastAccepted + s.BroadcastRejected
	if total > 0 {
		s.AcceptRate = float64(s.BroadcastAccepted) / float64(total) * 100
	}

	pingTotal := s.PingReceived + s.PingAccepted + s.PingFailed
	if pingTotal > 0 {
		s.PingAcceptRate = float64(s.PingAccepted) / float64(pingTotal) * 100
	}

	if s.DSEGResponses > 0 {
		s.AvgMNsServed = float64(dsegTotalServed) / float64(s.DSEGResponses)
	}

	// Build sorted reject reasons (top 10)
	s.RejectReasons = buildTopN(rejectReasons, 10)

	// Build sorted top sources (top 5)
	s.TopSources = buildTopSources(sourceCounts, 5)

	// Build status changes
	s.StatusChanges = buildTopN(statusChangeCounts, 20)

	// Ensure slices are non-nil for JSON
	if s.RejectReasons == nil {
		s.RejectReasons = []ReasonCount{}
	}
	if s.TopSources == nil {
		s.TopSources = []SourceCount{}
	}
	if s.SyncTransitions == nil {
		s.SyncTransitions = []StatusTransition{}
	}
	if s.StatusChanges == nil {
		s.StatusChanges = []ReasonCount{}
	}
	if s.ActiveMNChanges == nil {
		s.ActiveMNChanges = []StatusTransition{}
	}

	// Build peer detail list sorted by event count descending
	s.PeerDetails = buildPeerDetails(peerEventCounts)
	if s.PeerDetails == nil {
		s.PeerDetails = []PeerDetail{}
	}

	// Build masternode detail list sorted by event count descending
	s.MasternodeDetails = buildMasternodeDetails(mnEventCounts)
	if s.MasternodeDetails == nil {
		s.MasternodeDetails = []MasternodeDetail{}
	}

	return s, nil
}

// buildTopN returns the top N entries from a count map, sorted by count descending.
func buildTopN(counts map[string]int64, n int) []ReasonCount {
	if len(counts) == 0 {
		return nil
	}
	result := make([]ReasonCount, 0, len(counts))
	for label, count := range counts {
		result = append(result, ReasonCount{Label: label, Count: count})
	}
	// Sort descending by count
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].Count > result[i].Count {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	if len(result) > n {
		result = result[:n]
	}
	return result
}

// buildTopSources returns the top N source addresses by event count.
func buildTopSources(counts map[string]int64, n int) []SourceCount {
	if len(counts) == 0 {
		return nil
	}
	result := make([]SourceCount, 0, len(counts))
	for source, count := range counts {
		result = append(result, SourceCount{Source: source, Count: count})
	}
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].Count > result[i].Count {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	if len(result) > n {
		result = result[:n]
	}
	return result
}

// isNetworkAddress returns true if s looks like an IP:port address rather than an outpoint (txid:vout).
func isNetworkAddress(s string) bool {
	// IPv4 addresses contain dots, IPv6 addresses contain brackets
	return strings.Contains(s, ".") || strings.Contains(s, "[")
}

// buildPeerDetails returns all peer addresses with their event counts, sorted by count descending.
func buildPeerDetails(counts map[string]int64) []PeerDetail {
	if len(counts) == 0 {
		return nil
	}
	result := make([]PeerDetail, 0, len(counts))
	for addr, count := range counts {
		result = append(result, PeerDetail{Address: addr, EventCount: count})
	}
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].EventCount > result[i].EventCount {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

// buildMasternodeDetails returns all masternode outpoints with tier and event counts, sorted by count descending.
func buildMasternodeDetails(counts map[string]*mnAccumHelper) []MasternodeDetail {
	if len(counts) == 0 {
		return nil
	}
	result := make([]MasternodeDetail, 0, len(counts))
	for outpoint, acc := range counts {
		result = append(result, MasternodeDetail{Outpoint: outpoint, Address: acc.addr, Tier: acc.tier, EventCount: acc.count})
	}
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].EventCount > result[i].EventCount {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

// Query reads the JSONL file and returns filtered events.
// When Newest is true, results are returned in reverse chronological order (newest first).
// Otherwise, results are returned in chronological order (oldest first).
func (c *Collector) Query(filter Filter) ([]Event, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultQueryLimit
	}

	c.mu.Lock()
	// Flush any buffered data before reading
	if c.writer != nil {
		c.writer.Flush()
	}
	filePath := c.filePath
	c.mu.Unlock()

	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("debug collector: failed to open for query: %w", err)
	}
	defer f.Close()

	var results []Event
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024) // 1MB max line

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var event Event
		if err := json.Unmarshal(line, &event); err != nil {
			continue // skip malformed lines
		}

		if !matchesFilter(event, filter) {
			continue
		}

		results = append(results, event)
		if !filter.Newest && len(results) >= limit {
			break
		}
	}

	// When Newest is set, keep only the last `limit` events and reverse
	// so the caller receives them in newest-first order directly.
	if filter.Newest {
		if len(results) > limit {
			results = results[len(results)-limit:]
		}
		for i, j := 0, len(results)-1; i < j; i, j = i+1, j-1 {
			results[i], results[j] = results[j], results[i]
		}
	}

	return results, scanner.Err()
}

// Clear truncates the debug log file.
func (c *Collector) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.writer != nil {
		c.writer.Flush()
	}

	c.closeFileLocked()

	// Truncate the file
	f, err := os.Create(c.filePath)
	if err != nil {
		return fmt.Errorf("debug collector: failed to clear: %w", err)
	}
	f.Close()

	// Reset stats and currentSize together under statsMu
	c.statsMu.Lock()
	c.currentSize = 0
	c.stats.Total = 0
	c.stats.ByCategory = make(map[string]int64)
	c.statsMu.Unlock()

	// Re-open if enabled
	if c.enabled.Load() {
		return c.reopenFileLocked()
	}
	return nil
}

// Close stops collection and releases resources.
func (c *Collector) Close() {
	c.Disable()
}

// rotateLocked rotates log files: mn-debug.jsonl → .1.jsonl → .2.jsonl → ...
// Caller must hold c.mu.
func (c *Collector) rotateLocked() {
	c.closeFileLocked()

	base := strings.TrimSuffix(c.filePath, ".jsonl")

	// Remove oldest file
	oldest := fmt.Sprintf("%s.%d.jsonl", base, c.maxFiles)
	os.Remove(oldest)

	// Shift existing rotated files
	for i := c.maxFiles - 1; i >= 1; i-- {
		oldName := fmt.Sprintf("%s.%d.jsonl", base, i)
		newName := fmt.Sprintf("%s.%d.jsonl", base, i+1)
		os.Rename(oldName, newName)
	}

	// Rotate current file to .1
	os.Rename(c.filePath, fmt.Sprintf("%s.1.jsonl", base))

	c.statsMu.Lock()
	c.currentSize = 0
	c.statsMu.Unlock()
	if err := c.reopenFileLocked(); err != nil {
		// Disable collection — subsequent Emit() calls will see writer==nil
		// and return early, preventing silent data loss.
		c.enabled.Store(false)
	}
}

// closeFileLocked closes the current file. Caller must hold c.mu.
func (c *Collector) closeFileLocked() {
	if c.writer != nil {
		c.writer.Flush()
		c.writer = nil
	}
	if c.file != nil {
		c.file.Close()
		c.file = nil
	}
}

// reopenFileLocked opens the log file for appending. Caller must hold c.mu.
func (c *Collector) reopenFileLocked() error {
	f, err := os.OpenFile(c.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	c.file = f
	c.writer = bufio.NewWriterSize(f, 64*1024)
	return nil
}

// matchesFilter checks if an event matches all filter criteria.
func matchesFilter(event Event, filter Filter) bool {
	if filter.Category != "" && event.Category != filter.Category {
		return false
	}
	if filter.Type != "" && event.Type != filter.Type {
		return false
	}
	if filter.Source != "" && event.Source != filter.Source {
		return false
	}
	if filter.Search != "" {
		searchLower := strings.ToLower(filter.Search)
		if !strings.Contains(strings.ToLower(event.Summary), searchLower) {
			return false
		}
	}
	return true
}
