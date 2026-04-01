package main

import (
	"sync"
	"time"
)

const (
	trafficCollectInterval = 1 * time.Second
	trafficMaxAge          = 24 * time.Hour
)

// TrafficSample represents a single network traffic measurement.
// JSON tags match the frontend TrafficSample TypeScript interface.
type TrafficSample struct {
	Timestamp int64   `json:"timestamp"` // Unix milliseconds
	BytesIn   uint64  `json:"bytesIn"`   // Delta bytes received since last sample
	BytesOut  uint64  `json:"bytesOut"`  // Delta bytes sent since last sample
	RateIn    float64 `json:"rateIn"`    // KB/s received
	RateOut   float64 `json:"rateOut"`   // KB/s sent
}

// TrafficCollector continuously samples P2P network traffic in the background.
// Samples are retained for up to 24 hours and served to the frontend on demand.
type TrafficCollector struct {
	mu       sync.RWMutex
	samples  []TrafficSample
	lastRecv uint64
	lastSent uint64
	lastTime time.Time
	getStats func() (recv, sent uint64)
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewTrafficCollector creates a new collector. getStats returns cumulative
// bytes received and sent from the P2P layer.
func NewTrafficCollector(getStats func() (uint64, uint64)) *TrafficCollector {
	return &TrafficCollector{
		getStats: getStats,
		stopCh:   make(chan struct{}),
	}
}

// Start launches the background collection goroutine with a 1-second ticker.
func (tc *TrafficCollector) Start() {
	go tc.run()
}

// Stop signals the collection goroutine to exit. Safe to call multiple times.
func (tc *TrafficCollector) Stop() {
	tc.stopOnce.Do(func() {
		close(tc.stopCh)
	})
}

func (tc *TrafficCollector) run() {
	ticker := time.NewTicker(trafficCollectInterval)
	defer ticker.Stop()

	for {
		select {
		case <-tc.stopCh:
			return
		case <-ticker.C:
			tc.collect()
		}
	}
}

func (tc *TrafficCollector) collect() {
	recv, sent := tc.getStats()
	now := time.Now()

	tc.mu.Lock()
	defer tc.mu.Unlock()

	if !tc.lastTime.IsZero() {
		elapsed := now.Sub(tc.lastTime).Seconds()
		if elapsed <= 0 {
			tc.lastRecv = recv
			tc.lastSent = sent
			tc.lastTime = now
			return
		}

		var bytesIn, bytesOut uint64
		if recv >= tc.lastRecv {
			bytesIn = recv - tc.lastRecv
		}
		if sent >= tc.lastSent {
			bytesOut = sent - tc.lastSent
		}

		rateIn := float64(bytesIn) / 1024.0 / elapsed  // KB/s
		rateOut := float64(bytesOut) / 1024.0 / elapsed // KB/s

		tc.samples = append(tc.samples, TrafficSample{
			Timestamp: now.UnixMilli(),
			BytesIn:   bytesIn,
			BytesOut:  bytesOut,
			RateIn:    rateIn,
			RateOut:   rateOut,
		})

		// Prune samples older than 24h (slice is chronological)
		cutoff := now.Add(-trafficMaxAge).UnixMilli()
		start := 0
		for start < len(tc.samples) && tc.samples[start].Timestamp < cutoff {
			start++
		}
		if start > 0 {
			n := copy(tc.samples, tc.samples[start:])
			tc.samples = tc.samples[:n]
		}
	}

	tc.lastRecv = recv
	tc.lastSent = sent
	tc.lastTime = now
}

// GetHistory returns traffic samples for the requested time range, downsampled
// to at most maxSamples entries.
func (tc *TrafficCollector) GetHistory(rangeMinutes, maxSamples int) []TrafficSample {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	if len(tc.samples) == 0 {
		return []TrafficSample{}
	}

	now := time.Now().UnixMilli()
	cutoff := now - int64(rangeMinutes)*60*1000

	// Filter to requested time range using binary search on sorted timestamps
	lo := 0
	for lo < len(tc.samples) && tc.samples[lo].Timestamp < cutoff {
		lo++
	}
	visible := tc.samples[lo:]

	if len(visible) == 0 {
		return []TrafficSample{}
	}

	// If within budget, return a copy as-is
	if len(visible) <= maxSamples || maxSamples <= 0 {
		result := make([]TrafficSample, len(visible))
		copy(result, visible)
		return result
	}

	// Downsample into equal-duration buckets
	rangeMs := float64(now - cutoff)
	bucketDuration := rangeMs / float64(maxSamples)
	result := make([]TrafficSample, 0, maxSamples)

	bucketStart := float64(cutoff)
	i := 0
	for b := 0; b < maxSamples && i < len(visible); b++ {
		bucketEnd := bucketStart + bucketDuration
		var sumRateIn, sumRateOut float64
		var sumBytesIn, sumBytesOut uint64
		count := 0

		for i < len(visible) && float64(visible[i].Timestamp) < bucketEnd {
			sumRateIn += visible[i].RateIn
			sumRateOut += visible[i].RateOut
			sumBytesIn += visible[i].BytesIn
			sumBytesOut += visible[i].BytesOut
			count++
			i++
		}

		if count > 0 {
			result = append(result, TrafficSample{
				Timestamp: int64(bucketStart + bucketDuration/2),
				BytesIn:   sumBytesIn,
				BytesOut:  sumBytesOut,
				RateIn:    sumRateIn / float64(count),
				RateOut:   sumRateOut / float64(count),
			})
		}
		bucketStart = bucketEnd
	}

	return result
}
