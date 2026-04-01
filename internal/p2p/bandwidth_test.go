package p2p

import (
	"fmt"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBandwidthMonitor(t *testing.T) {
	logger := logrus.New()
	bm := NewBandwidthMonitor(logger, 1000000, 2000000) // 1MB/s up, 2MB/s down

	assert.NotNil(t, bm)
	assert.Equal(t, uint64(1000000), bm.maxUpload)
	assert.Equal(t, uint64(2000000), bm.maxDownload)
	assert.NotNil(t, bm.uploadLimiter)
	assert.NotNil(t, bm.downloadLimiter)
}

func TestBandwidthMonitorWithoutLimits(t *testing.T) {
	logger := logrus.New()
	bm := NewBandwidthMonitor(logger, 0, 0) // No limits

	assert.Nil(t, bm.uploadLimiter)
	assert.Nil(t, bm.downloadLimiter)
	assert.True(t, bm.CanUpload(1000000))
	assert.True(t, bm.CanDownload(1000000))
}

func TestRecordUploadDownload(t *testing.T) {
	logger := logrus.New()
	bm := NewBandwidthMonitor(logger, 0, 0)

	peerAddr := "192.168.1.100:18333"

	// Record some traffic
	bm.RecordUpload(peerAddr, 1024)
	bm.RecordDownload(peerAddr, 2048)

	// Check totals
	assert.Equal(t, uint64(1024), bm.totalUpload.Load())
	assert.Equal(t, uint64(2048), bm.totalDownload.Load())

	// Check peer stats
	stats := bm.GetPeerStats(peerAddr)
	require.NotNil(t, stats)
	assert.Equal(t, uint64(1024), stats.TotalUpload)
	assert.Equal(t, uint64(2048), stats.TotalDownload)
}

func TestRecordMessage(t *testing.T) {
	logger := logrus.New()
	bm := NewBandwidthMonitor(logger, 0, 0)

	peerAddr := "192.168.1.100:18333"

	// Record sent message
	bm.RecordMessage(peerAddr, true, 500)

	// Record received message
	bm.RecordMessage(peerAddr, false, 1000)

	stats := bm.GetPeerStats(peerAddr)
	require.NotNil(t, stats)

	assert.Equal(t, uint64(500), stats.TotalUpload)
	assert.Equal(t, uint64(1000), stats.TotalDownload)
	assert.Equal(t, uint64(1), stats.MessagesSent)
	assert.Equal(t, uint64(1), stats.MessagesReceived)
}

func TestTokenBucket(t *testing.T) {
	tb := NewTokenBucket(1000, 100) // 1000 capacity, 100 tokens/sec

	// Should be able to take initial tokens
	assert.True(t, tb.Take(500))
	assert.Equal(t, uint64(500), tb.tokens.Load())

	// Should be able to take remaining tokens
	assert.True(t, tb.Take(500))
	assert.Equal(t, uint64(0), tb.tokens.Load())

	// Should not be able to take more tokens
	assert.False(t, tb.Take(1))

	// Test refill
	tb.lastRefill.Store(time.Now().Unix() - 2) // 2 seconds ago
	tb.Refill()

	// Should have gained 200 tokens (2 seconds * 100 tokens/sec)
	assert.Equal(t, uint64(200), tb.tokens.Load())
}

func TestTokenBucketCapacityLimit(t *testing.T) {
	tb := NewTokenBucket(100, 1000) // 100 capacity, 1000 tokens/sec

	// Simulate long time period
	tb.lastRefill.Store(time.Now().Unix() - 10) // 10 seconds ago
	tb.Refill()

	// Should be capped at capacity
	assert.Equal(t, uint64(100), tb.tokens.Load())
}

func TestCircularBuffer(t *testing.T) {
	cb := NewCircularBuffer(3)

	// Add some values
	cb.Add(10)
	cb.Add(20)
	cb.Add(30)

	data := cb.GetData()
	assert.Equal(t, []uint64{10, 20, 30}, data)

	// Add more values (should wrap around)
	cb.Add(40)
	data = cb.GetData()
	assert.Equal(t, []uint64{20, 30, 40}, data)

	// Test average
	avg := cb.GetAverage()
	assert.Equal(t, uint64(30), avg) // (20+30+40)/3 = 30
}

func TestCircularBufferEmpty(t *testing.T) {
	cb := NewCircularBuffer(5)

	data := cb.GetData()
	assert.Nil(t, data)

	avg := cb.GetAverage()
	assert.Equal(t, uint64(0), avg)
}

func TestBandwidthStats(t *testing.T) {
	logger := logrus.New()
	bm := NewBandwidthMonitor(logger, 1000, 2000)

	// Record some data
	bm.totalUpload.Store(5000)
	bm.totalDownload.Store(10000)
	bm.currentUploadRate.Store(100)
	bm.currentDownloadRate.Store(200)

	stats := bm.GetStats()

	assert.Equal(t, uint64(5000), stats["total_upload"])
	assert.Equal(t, uint64(10000), stats["total_download"])
	assert.Equal(t, uint64(100), stats["current_upload_rate"])
	assert.Equal(t, uint64(200), stats["current_download_rate"])
	assert.Equal(t, uint64(1000), stats["max_upload_rate"])
	assert.Equal(t, uint64(2000), stats["max_download_rate"])
}

func TestGetAllPeerStats(t *testing.T) {
	logger := logrus.New()
	bm := NewBandwidthMonitor(logger, 0, 0)

	// Record data for multiple peers
	peers := []string{
		"192.168.1.100:18333",
		"192.168.1.101:18333",
		"192.168.1.102:18333",
	}

	for i, peer := range peers {
		bm.RecordUpload(peer, uint64((i+1)*1000))
		bm.RecordDownload(peer, uint64((i+1)*2000))
	}

	allStats := bm.GetAllPeerStats()
	assert.Equal(t, 3, len(allStats))

	for i, peer := range peers {
		stats, exists := allStats[peer]
		require.True(t, exists)
		assert.Equal(t, uint64((i+1)*1000), stats.TotalUpload)
		assert.Equal(t, uint64((i+1)*2000), stats.TotalDownload)
	}
}

func TestUpdateLimits(t *testing.T) {
	logger := logrus.New()
	bm := NewBandwidthMonitor(logger, 1000, 2000)

	// Update limits
	bm.UpdateLimits(5000, 10000)

	assert.Equal(t, uint64(5000), bm.maxUpload)
	assert.Equal(t, uint64(10000), bm.maxDownload)
	assert.NotNil(t, bm.uploadLimiter)
	assert.NotNil(t, bm.downloadLimiter)

	// Disable limits
	bm.UpdateLimits(0, 0)
	assert.Nil(t, bm.uploadLimiter)
	assert.Nil(t, bm.downloadLimiter)
}

func TestLimitExceededHandler(t *testing.T) {
	logger := logrus.New()
	bm := NewBandwidthMonitor(logger, 0, 0) // Start with no limits

	limitExceeded := false
	var limitType string
	var current, limit uint64

	bm.SetLimitExceededHandler(func(lt string, c, l uint64) {
		limitExceeded = true
		limitType = lt
		current = c
		limit = l
	})

	// Manually set limits and simulate bandwidth usage
	bm.maxUpload = 1000

	// Simulate 2000 bytes uploaded over 1 second
	var lastUpload, lastDownload uint64
	lastTime := time.Now().Add(-1 * time.Second) // 1 second ago
	lastUpload = 0
	bm.totalUpload.Store(2000) // 2000 bytes uploaded since lastUpload

	bm.calculateRates(&lastUpload, &lastDownload, &lastTime)

	// Handler should have been called
	assert.True(t, limitExceeded)
	assert.Equal(t, "upload", limitType)
	// Allow for small timing variations (within 5%)
	assert.InDelta(t, uint64(2000), current, 100) // 2000 ± 100
	assert.Equal(t, uint64(1000), limit)
}

func TestFormatBytes(t *testing.T) {
	testCases := []struct {
		bytes    uint64
		expected string
	}{
		{0, "0 B/s"},
		{500, "500 B/s"},
		{1024, "1.0 KB/s"},
		{1536, "1.5 KB/s"}, // 1.5 KB
		{1024 * 1024, "1.0 MB/s"},
		{1024 * 1024 * 1024, "1.0 GB/s"},
	}

	for _, tc := range testCases {
		result := formatBytes(tc.bytes)
		assert.Equal(t, tc.expected, result)
	}
}

func TestBandwidthHistory(t *testing.T) {
	logger := logrus.New()
	bm := NewBandwidthMonitor(logger, 0, 0)

	// Add some history data
	bm.uploadHistory.Add(100)
	bm.uploadHistory.Add(200)
	bm.downloadHistory.Add(150)
	bm.downloadHistory.Add(250)

	history := bm.GetHistory()
	assert.Equal(t, 2, len(history))

	// Check that timestamps are reasonable
	for _, sample := range history {
		assert.False(t, sample.Timestamp.IsZero())
		assert.True(t, sample.Upload > 0)
		assert.True(t, sample.Download > 0)
	}
}

func TestConcurrentBandwidthAccess(t *testing.T) {
	logger := logrus.New()
	bm := NewBandwidthMonitor(logger, 1000000, 2000000)

	// Start monitoring
	bm.Start()
	defer bm.Stop()

	// Simulate concurrent access from multiple goroutines
	numGoroutines := 10
	numOperations := 100

	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			peerAddr := fmt.Sprintf("192.168.1.%d:18333", id)
			for j := 0; j < numOperations; j++ {
				bm.RecordUpload(peerAddr, uint64(j*10))
				bm.RecordDownload(peerAddr, uint64(j*20))

				// Occasionally check stats
				if j%10 == 0 {
					stats := bm.GetStats()
					assert.NotNil(t, stats)
					peerStats := bm.GetPeerStats(peerAddr)
					assert.NotNil(t, peerStats)
				}
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify final stats
	stats := bm.GetStats()
	assert.NotNil(t, stats)
	assert.Equal(t, numGoroutines, stats["active_peers"])

	allPeerStats := bm.GetAllPeerStats()
	assert.Equal(t, numGoroutines, len(allPeerStats))
}

func BenchmarkRecordUpload(b *testing.B) {
	logger := logrus.New()
	bm := NewBandwidthMonitor(logger, 0, 0)

	peerAddr := "192.168.1.100:18333"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bm.RecordUpload(peerAddr, 1024)
	}
}

func BenchmarkRecordMessage(b *testing.B) {
	logger := logrus.New()
	bm := NewBandwidthMonitor(logger, 0, 0)

	peerAddr := "192.168.1.100:18333"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bm.RecordMessage(peerAddr, true, 512)
	}
}

func BenchmarkTokenBucketTake(b *testing.B) {
	tb := NewTokenBucket(1000000, 100000) // Large capacity for benchmark

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tb.Take(1)
	}
}

func BenchmarkCircularBufferAdd(b *testing.B) {
	cb := NewCircularBuffer(1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.Add(uint64(i))
	}
}

func BenchmarkGetAllPeerStats(b *testing.B) {
	logger := logrus.New()
	bm := NewBandwidthMonitor(logger, 0, 0)

	// Add 100 peers
	for i := 0; i < 100; i++ {
		peerAddr := fmt.Sprintf("192.168.1.%d:18333", i)
		bm.RecordUpload(peerAddr, uint64(i*1000))
		bm.RecordDownload(peerAddr, uint64(i*2000))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stats := bm.GetAllPeerStats()
		_ = stats
	}
}
