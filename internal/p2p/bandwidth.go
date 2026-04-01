package p2p

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// BandwidthMonitor tracks and manages bandwidth usage for P2P connections
type BandwidthMonitor struct {
	// Configuration
	logger       *logrus.Entry
	maxUpload    uint64 // Bytes per second
	maxDownload  uint64 // Bytes per second
	samplePeriod time.Duration

	// Global bandwidth tracking
	totalUpload   atomic.Uint64 // Total bytes uploaded
	totalDownload atomic.Uint64 // Total bytes downloaded

	// Rate tracking (bytes per second)
	currentUploadRate   atomic.Uint64 // Current upload rate
	currentDownloadRate atomic.Uint64 // Current download rate

	// Per-peer tracking
	peerStats map[string]*PeerBandwidthStats
	statsMu   sync.RWMutex

	// Rate limiting
	uploadLimiter   *TokenBucket
	downloadLimiter *TokenBucket

	// Time series data for monitoring
	uploadHistory   *CircularBuffer
	downloadHistory *CircularBuffer

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	quit   chan struct{}

	// Events
	onLimitExceeded func(limitType string, current, limit uint64)
}

// PeerBandwidthStats tracks bandwidth statistics for a single peer
type PeerBandwidthStats struct {
	Address          string
	TotalUpload      uint64
	TotalDownload    uint64
	UploadRate       uint64 // Bytes per second
	DownloadRate     uint64 // Bytes per second
	LastActivity     time.Time
	Connected        time.Time
	MessagesSent     uint64
	MessagesReceived uint64

	// Rate tracking
	uploadSamples   *CircularBuffer
	downloadSamples *CircularBuffer
}

// TokenBucket implements a token bucket rate limiter
type TokenBucket struct {
	capacity   uint64        // Maximum tokens
	tokens     atomic.Uint64 // Current tokens
	refillRate uint64        // Tokens per second
	lastRefill atomic.Int64  // Last refill timestamp
	mu         sync.Mutex
}

// CircularBuffer implements a circular buffer for time series data
type CircularBuffer struct {
	data   []uint64
	head   int
	size   int
	length int
	mu     sync.RWMutex
}

// BandwidthSample represents a bandwidth measurement at a point in time
type BandwidthSample struct {
	Timestamp time.Time
	Upload    uint64
	Download  uint64
}

// Bandwidth monitoring constants
const (
	DefaultSamplePeriod    = 10 * time.Second
	DefaultHistorySize     = 360 // 1 hour at 10s intervals
	DefaultRefillInterval  = time.Second
	TokenBucketSize        = 1024 * 1024 // 1MB burst capacity
	PeerStatsCleanupPeriod = 5 * time.Minute
	InactivePeerTimeout    = 10 * time.Minute
)

// NewBandwidthMonitor creates a new bandwidth monitor
func NewBandwidthMonitor(logger *logrus.Logger, maxUpload, maxDownload uint64) *BandwidthMonitor {
	ctx, cancel := context.WithCancel(context.Background())

	bm := &BandwidthMonitor{
		logger:       logger.WithField("component", "bandwidth-monitor"),
		maxUpload:    maxUpload,
		maxDownload:  maxDownload,
		samplePeriod: DefaultSamplePeriod,
		peerStats:    make(map[string]*PeerBandwidthStats),

		uploadHistory:   NewCircularBuffer(DefaultHistorySize),
		downloadHistory: NewCircularBuffer(DefaultHistorySize),

		ctx:    ctx,
		cancel: cancel,
		quit:   make(chan struct{}),
	}

	// Initialize rate limiters if limits are set
	if maxUpload > 0 {
		bm.uploadLimiter = NewTokenBucket(TokenBucketSize, maxUpload)
	}
	if maxDownload > 0 {
		bm.downloadLimiter = NewTokenBucket(TokenBucketSize, maxDownload)
	}

	return bm
}

// Start begins bandwidth monitoring
func (bm *BandwidthMonitor) Start() {
	bm.logger.WithFields(logrus.Fields{
		"max_upload":    bm.maxUpload,
		"max_download":  bm.maxDownload,
		"sample_period": bm.samplePeriod,
	}).Info("Starting bandwidth monitor")

	// Start monitoring goroutines
	bm.wg.Add(3)
	go bm.rateCalculationLoop()
	go bm.statsCleanupLoop()
	go bm.rateLimiterRefillLoop()

	bm.logger.Info("Bandwidth monitor started")
}

// Stop stops bandwidth monitoring
func (bm *BandwidthMonitor) Stop() {
	bm.logger.Info("Stopping bandwidth monitor")

	bm.cancel()
	close(bm.quit)

	// Wait for goroutines to finish
	done := make(chan struct{})
	go func() {
		bm.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		bm.logger.Info("Bandwidth monitor stopped")
	case <-time.After(10 * time.Second):
		bm.logger.Warn("Bandwidth monitor stop timeout")
	}
}

// rateCalculationLoop calculates bandwidth rates periodically
func (bm *BandwidthMonitor) rateCalculationLoop() {
	defer bm.wg.Done()

	ticker := time.NewTicker(bm.samplePeriod)
	defer ticker.Stop()

	var lastUpload, lastDownload uint64
	lastTime := time.Now()

	for {
		select {
		case <-ticker.C:
			bm.calculateRates(&lastUpload, &lastDownload, &lastTime)

		case <-bm.quit:
			return
		}
	}
}

// calculateRates calculates current bandwidth rates
func (bm *BandwidthMonitor) calculateRates(lastUpload, lastDownload *uint64, lastTime *time.Time) {
	now := time.Now()
	elapsed := now.Sub(*lastTime).Seconds()

	if elapsed == 0 {
		return
	}

	currentUpload := bm.totalUpload.Load()
	currentDownload := bm.totalDownload.Load()

	uploadRate := uint64(float64(currentUpload-*lastUpload) / elapsed)
	downloadRate := uint64(float64(currentDownload-*lastDownload) / elapsed)

	bm.currentUploadRate.Store(uploadRate)
	bm.currentDownloadRate.Store(downloadRate)

	// Store in history
	bm.uploadHistory.Add(uploadRate)
	bm.downloadHistory.Add(downloadRate)

	// Check limits
	if bm.maxUpload > 0 && uploadRate > bm.maxUpload {
		if bm.onLimitExceeded != nil {
			bm.onLimitExceeded("upload", uploadRate, bm.maxUpload)
		}
	}

	if bm.maxDownload > 0 && downloadRate > bm.maxDownload {
		if bm.onLimitExceeded != nil {
			bm.onLimitExceeded("download", downloadRate, bm.maxDownload)
		}
	}

	bm.logger.WithFields(logrus.Fields{
		"upload_rate":    formatBytes(uploadRate),
		"download_rate":  formatBytes(downloadRate),
		"total_upload":   formatBytes(currentUpload),
		"total_download": formatBytes(currentDownload),
	}).Debug("Bandwidth rates updated")

	*lastUpload = currentUpload
	*lastDownload = currentDownload
	*lastTime = now
}

// statsCleanupLoop periodically cleans up peer statistics
func (bm *BandwidthMonitor) statsCleanupLoop() {
	defer bm.wg.Done()

	ticker := time.NewTicker(PeerStatsCleanupPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			bm.cleanupInactivePeers()

		case <-bm.quit:
			return
		}
	}
}

// cleanupInactivePeers removes statistics for inactive peers
func (bm *BandwidthMonitor) cleanupInactivePeers() {
	bm.statsMu.Lock()
	defer bm.statsMu.Unlock()

	now := time.Now()
	removed := 0

	for addr, stats := range bm.peerStats {
		if now.Sub(stats.LastActivity) > InactivePeerTimeout {
			delete(bm.peerStats, addr)
			removed++
		}
	}

	if removed > 0 {
		bm.logger.WithFields(logrus.Fields{
			"removed": removed,
			"active":  len(bm.peerStats),
		}).Debug("Cleaned up inactive peer statistics")
	}
}

// rateLimiterRefillLoop refills token buckets for rate limiting
func (bm *BandwidthMonitor) rateLimiterRefillLoop() {
	defer bm.wg.Done()

	ticker := time.NewTicker(DefaultRefillInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if bm.uploadLimiter != nil {
				bm.uploadLimiter.Refill()
			}
			if bm.downloadLimiter != nil {
				bm.downloadLimiter.Refill()
			}

		case <-bm.quit:
			return
		}
	}
}

// RecordUpload records uploaded bytes for a peer
func (bm *BandwidthMonitor) RecordUpload(peerAddr string, bytes uint64) {
	bm.totalUpload.Add(bytes)

	// Update peer stats
	bm.updatePeerStats(peerAddr, func(stats *PeerBandwidthStats) {
		stats.TotalUpload += bytes
		stats.LastActivity = time.Now()
		stats.uploadSamples.Add(bytes)
	})
}

// RecordDownload records downloaded bytes for a peer
func (bm *BandwidthMonitor) RecordDownload(peerAddr string, bytes uint64) {
	bm.totalDownload.Add(bytes)

	// Update peer stats
	bm.updatePeerStats(peerAddr, func(stats *PeerBandwidthStats) {
		stats.TotalDownload += bytes
		stats.LastActivity = time.Now()
		stats.downloadSamples.Add(bytes)
	})
}

// RecordMessage records a message sent or received
func (bm *BandwidthMonitor) RecordMessage(peerAddr string, sent bool, bytes uint64) {
	if sent {
		bm.RecordUpload(peerAddr, bytes)
		bm.updatePeerStats(peerAddr, func(stats *PeerBandwidthStats) {
			stats.MessagesSent++
		})
	} else {
		bm.RecordDownload(peerAddr, bytes)
		bm.updatePeerStats(peerAddr, func(stats *PeerBandwidthStats) {
			stats.MessagesReceived++
		})
	}
}

// updatePeerStats updates peer statistics with the given function
func (bm *BandwidthMonitor) updatePeerStats(peerAddr string, updateFunc func(*PeerBandwidthStats)) {
	bm.statsMu.Lock()
	defer bm.statsMu.Unlock()

	stats, exists := bm.peerStats[peerAddr]
	if !exists {
		stats = &PeerBandwidthStats{
			Address:         peerAddr,
			Connected:       time.Now(),
			LastActivity:    time.Now(),
			uploadSamples:   NewCircularBuffer(DefaultHistorySize / 6), // Smaller buffer for peers
			downloadSamples: NewCircularBuffer(DefaultHistorySize / 6),
		}
		bm.peerStats[peerAddr] = stats
	}

	updateFunc(stats)
}

// CanUpload checks if upload is allowed under rate limits
func (bm *BandwidthMonitor) CanUpload(bytes uint64) bool {
	if bm.uploadLimiter == nil {
		return true
	}
	return bm.uploadLimiter.Take(bytes)
}

// CanDownload checks if download is allowed under rate limits
func (bm *BandwidthMonitor) CanDownload(bytes uint64) bool {
	if bm.downloadLimiter == nil {
		return true
	}
	return bm.downloadLimiter.Take(bytes)
}

// GetStats returns overall bandwidth statistics
func (bm *BandwidthMonitor) GetStats() map[string]interface{} {
	bm.statsMu.RLock()
	defer bm.statsMu.RUnlock()

	stats := map[string]interface{}{
		"total_upload":          bm.totalUpload.Load(),
		"total_download":        bm.totalDownload.Load(),
		"current_upload_rate":   bm.currentUploadRate.Load(),
		"current_download_rate": bm.currentDownloadRate.Load(),
		"max_upload_rate":       bm.maxUpload,
		"max_download_rate":     bm.maxDownload,
		"active_peers":          len(bm.peerStats),
	}

	// Add rate limit status
	if bm.uploadLimiter != nil {
		stats["upload_tokens"] = bm.uploadLimiter.tokens.Load()
	}
	if bm.downloadLimiter != nil {
		stats["download_tokens"] = bm.downloadLimiter.tokens.Load()
	}

	return stats
}

// GetPeerStats returns bandwidth statistics for a specific peer
func (bm *BandwidthMonitor) GetPeerStats(peerAddr string) *PeerBandwidthStats {
	bm.statsMu.RLock()
	defer bm.statsMu.RUnlock()

	if stats, exists := bm.peerStats[peerAddr]; exists {
		// Return a copy to avoid race conditions
		statsCopy := *stats
		return &statsCopy
	}

	return nil
}

// GetAllPeerStats returns bandwidth statistics for all peers
func (bm *BandwidthMonitor) GetAllPeerStats() map[string]*PeerBandwidthStats {
	bm.statsMu.RLock()
	defer bm.statsMu.RUnlock()

	result := make(map[string]*PeerBandwidthStats)
	for addr, stats := range bm.peerStats {
		// Return copies to avoid race conditions
		statsCopy := *stats
		result[addr] = &statsCopy
	}

	return result
}

// GetHistory returns bandwidth history
func (bm *BandwidthMonitor) GetHistory() []BandwidthSample {
	uploadData := bm.uploadHistory.GetData()
	downloadData := bm.downloadHistory.GetData()

	length := len(uploadData)
	if len(downloadData) < length {
		length = len(downloadData)
	}

	history := make([]BandwidthSample, length)
	now := time.Now()

	for i := 0; i < length; i++ {
		history[i] = BandwidthSample{
			Timestamp: now.Add(-time.Duration(length-i-1) * bm.samplePeriod),
			Upload:    uploadData[i],
			Download:  downloadData[i],
		}
	}

	return history
}

// SetLimitExceededHandler sets the handler for limit exceeded events
func (bm *BandwidthMonitor) SetLimitExceededHandler(handler func(limitType string, current, limit uint64)) {
	bm.onLimitExceeded = handler
}

// UpdateLimits updates the bandwidth limits
func (bm *BandwidthMonitor) UpdateLimits(maxUpload, maxDownload uint64) {
	bm.maxUpload = maxUpload
	bm.maxDownload = maxDownload

	// Update rate limiters
	if maxUpload > 0 {
		if bm.uploadLimiter == nil {
			bm.uploadLimiter = NewTokenBucket(TokenBucketSize, maxUpload)
		} else {
			bm.uploadLimiter.SetRefillRate(maxUpload)
		}
	} else {
		bm.uploadLimiter = nil
	}

	if maxDownload > 0 {
		if bm.downloadLimiter == nil {
			bm.downloadLimiter = NewTokenBucket(TokenBucketSize, maxDownload)
		} else {
			bm.downloadLimiter.SetRefillRate(maxDownload)
		}
	} else {
		bm.downloadLimiter = nil
	}

	bm.logger.WithFields(logrus.Fields{
		"max_upload":   maxUpload,
		"max_download": maxDownload,
	}).Debug("Updated bandwidth limits")
}

// TokenBucket implementation

// NewTokenBucket creates a new token bucket
func NewTokenBucket(capacity, refillRate uint64) *TokenBucket {
	tb := &TokenBucket{
		capacity:   capacity,
		refillRate: refillRate,
	}
	tb.tokens.Store(capacity)
	tb.lastRefill.Store(time.Now().Unix())
	return tb
}

// Take attempts to take tokens from the bucket
func (tb *TokenBucket) Take(tokens uint64) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	available := tb.tokens.Load()
	if available >= tokens {
		tb.tokens.Store(available - tokens)
		return true
	}
	return false
}

// Refill adds tokens to the bucket based on elapsed time
func (tb *TokenBucket) Refill() {
	now := time.Now().Unix()
	last := tb.lastRefill.Load()
	elapsed := uint64(now - last)

	if elapsed == 0 {
		return
	}

	tokensToAdd := elapsed * tb.refillRate
	current := tb.tokens.Load()
	newTokens := current + tokensToAdd

	if newTokens > tb.capacity {
		newTokens = tb.capacity
	}

	tb.tokens.Store(newTokens)
	tb.lastRefill.Store(now)
}

// SetRefillRate updates the refill rate
func (tb *TokenBucket) SetRefillRate(rate uint64) {
	tb.refillRate = rate
}

// CircularBuffer implementation

// NewCircularBuffer creates a new circular buffer
func NewCircularBuffer(size int) *CircularBuffer {
	return &CircularBuffer{
		data: make([]uint64, size),
		size: size,
	}
}

// Add adds a value to the buffer
func (cb *CircularBuffer) Add(value uint64) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.data[cb.head] = value
	cb.head = (cb.head + 1) % cb.size

	if cb.length < cb.size {
		cb.length++
	}
}

// GetData returns a copy of the buffer data
func (cb *CircularBuffer) GetData() []uint64 {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if cb.length == 0 {
		return nil
	}

	data := make([]uint64, cb.length)
	start := (cb.head - cb.length + cb.size) % cb.size

	for i := 0; i < cb.length; i++ {
		data[i] = cb.data[(start+i)%cb.size]
	}

	return data
}

// GetAverage returns the average of values in the buffer
func (cb *CircularBuffer) GetAverage() uint64 {
	data := cb.GetData()
	if len(data) == 0 {
		return 0
	}

	var sum uint64
	for _, value := range data {
		sum += value
	}

	return sum / uint64(len(data))
}

// Helper functions

// formatBytes formats bytes as human-readable string
func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B/s", bytes)
	}

	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	units := []string{"KB/s", "MB/s", "GB/s", "TB/s"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}
