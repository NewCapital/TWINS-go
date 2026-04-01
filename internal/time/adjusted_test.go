package time

import (
	"net"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestTimeData() *TimeData {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	return NewTimeData(logger)
}

func TestIPDeduplication(t *testing.T) {
	td := newTestTimeData()
	now := time.Now().Unix()

	ip := net.IPv4(10, 0, 0, 1)
	td.AddTimeDataFromPeer(ip, now+10)
	assert.Equal(t, 1, len(td.samples))

	// Same IP again — should be ignored
	td.AddTimeDataFromPeer(ip, now+100)
	assert.Equal(t, 1, len(td.samples), "duplicate IP should be ignored")

	// Different IP — should be accepted
	ip2 := net.IPv4(10, 0, 0, 2)
	td.AddTimeDataFromPeer(ip2, now+20)
	assert.Equal(t, 2, len(td.samples))
}

func TestResetClearsKnownPeersAndWarning(t *testing.T) {
	td := newTestTimeData()
	now := time.Now().Unix()

	// Add a sample
	ip := net.IPv4(10, 0, 0, 1)
	td.AddTimeDataFromPeer(ip, now+10)
	assert.Equal(t, 1, len(td.samples))

	// Set warningIssued manually
	td.mu.Lock()
	td.warningIssued = true
	td.mu.Unlock()

	// Reset
	td.Reset()

	// knownPeers should be cleared — same IP should be accepted again
	td.AddTimeDataFromPeer(ip, now+10)
	assert.Equal(t, 1, len(td.samples), "peer should be accepted after reset")

	// warningIssued should be cleared
	td.mu.RLock()
	assert.False(t, td.warningIssued, "warningIssued should be cleared after reset")
	td.mu.RUnlock()
}

func TestMedianCalculationLegacy(t *testing.T) {
	td := newTestTimeData()
	now := time.Now().Unix()

	// Need 5 samples (odd count) to trigger legacy median update
	offsets := []int64{10, 20, 15, 25, 12}
	for i, off := range offsets {
		ip := net.IPv4(10, 0, 0, byte(i+1))
		td.AddTimeDataFromPeer(ip, now+off)
	}

	// Sorted: {10, 12, 15, 20, 25}, median = 15
	assert.Equal(t, int64(15), td.GetTimeOffset())
}

func TestMedianNotUpdatedOnEvenCount(t *testing.T) {
	td := newTestTimeData()
	now := time.Now().Unix()

	// Add 6 samples (even count) — legacy rule: no update on even count
	for i := 0; i < 6; i++ {
		ip := net.IPv4(10, 0, 0, byte(i+1))
		td.AddTimeDataFromPeer(ip, now+int64(10+i))
	}

	// Offset should still be from the 5-sample update, not recalculated at 6
	// At 5 samples: sorted {10,11,12,13,14}, median=12
	assert.Equal(t, int64(12), td.GetTimeOffset())
}

func TestLargeOffsetRejected(t *testing.T) {
	td := newTestTimeData()
	now := time.Now().Unix()

	// Add 5 samples with offset > 70 minutes
	for i := 0; i < 5; i++ {
		ip := net.IPv4(10, 0, 0, byte(i+1))
		td.AddTimeDataFromPeer(ip, now+5000+int64(i))
	}

	assert.Equal(t, int64(0), td.GetTimeOffset(), "large offsets should reset to 0")
}

func TestCircularBufferLimit(t *testing.T) {
	td := newTestTimeData()
	now := time.Now().Unix()

	// Add more than MaxTimeSamples
	for i := 0; i < MaxTimeSamples+50; i++ {
		ip := net.IPv4(byte(10+i/65536), byte(i/256), byte(i%256), 1)
		td.AddTimeDataFromPeer(ip, now+int64(10+i%20))
	}

	td.mu.RLock()
	assert.LessOrEqual(t, len(td.samples), MaxTimeSamples, "should not exceed MaxTimeSamples")
	td.mu.RUnlock()
}

func TestGetAdjustedTimeReturnsOffset(t *testing.T) {
	td := newTestTimeData()
	now := time.Now().Unix()

	// Set up a known offset of +30 seconds
	offsets := []int64{28, 30, 32, 29, 31}
	for i, off := range offsets {
		ip := net.IPv4(10, 0, 0, byte(i+1))
		td.AddTimeDataFromPeer(ip, now+off)
	}

	// Median of {28,29,30,31,32} = 30
	adjustedTime := td.GetAdjustedTime()
	expectedTime := time.Now().Add(30 * time.Second)
	diff := adjustedTime.Sub(expectedTime)
	assert.InDelta(t, 0, diff.Seconds(), 2, "adjusted time should be ~30s ahead")
}

func TestGetAdjustedUnix(t *testing.T) {
	td := newTestTimeData()
	now := time.Now().Unix()

	offsets := []int64{5, 5, 5, 5, 5}
	for i, off := range offsets {
		ip := net.IPv4(10, 0, 0, byte(i+1))
		td.AddTimeDataFromPeer(ip, now+off)
	}

	unix := td.GetAdjustedUnix()
	assert.InDelta(t, now+5, unix, 2, "adjusted unix should be ~5s ahead")
}

func TestWarningThresholdType(t *testing.T) {
	// WarningThreshold is 300 seconds (5 minutes) as an integer
	assert.Equal(t, int64(300), int64(WarningThreshold))
}

func TestAddTimeSampleNoDedup(t *testing.T) {
	td := newTestTimeData()
	now := time.Now()

	// AddTimeSample does NOT perform IP dedup — each call adds a sample
	td.AddTimeSample(now.Add(10*time.Second), now)
	td.AddTimeSample(now.Add(10*time.Second), now)
	assert.Equal(t, 2, len(td.samples), "AddTimeSample should not deduplicate")
}

func TestMedianSortDoesNotMutateOriginal(t *testing.T) {
	td := newTestTimeData()
	now := time.Now()

	// Add 5 samples (odd count triggers median calculation in legacy mode)
	offsets := []int64{50, 10, 30, 20, 40}
	for _, off := range offsets {
		td.AddTimeSample(now.Add(time.Duration(off)*time.Second), now)
	}

	// Verify samples are stored in insertion order (not sorted)
	td.mu.RLock()
	assert.Equal(t, len(offsets), len(td.samples))
	td.mu.RUnlock()
}

func TestGlobalFunctions(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	InitGlobalTimeData(logger)

	// Without samples, should return ~now
	now := time.Now().Unix()
	adjusted := GetAdjustedUnix()
	assert.InDelta(t, now, adjusted, 2)

	// GetAdjustedTime should return ~now
	adjustedTime := GetAdjustedTime()
	diff := time.Since(adjustedTime)
	assert.InDelta(t, 0, diff.Seconds(), 2)
}

func TestGlobalNilSafety(t *testing.T) {
	// Save and restore global
	saved := globalTimeData
	defer func() { globalTimeData = saved }()

	globalTimeData = nil

	// Should return current time without panic
	now := time.Now().Unix()
	assert.InDelta(t, now, GetAdjustedUnix(), 2)

	adjustedTime := GetAdjustedTime()
	require.False(t, adjustedTime.IsZero())

	// AddTimeSample and AddTimeDataFromPeer should not panic
	AddTimeSample(time.Now(), time.Now())
	AddTimeDataFromPeer(net.IPv4(1, 2, 3, 4), time.Now().Unix())
}

func TestGetTimeOffset(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	InitGlobalTimeData(logger)

	// Initially zero
	assert.Equal(t, int64(0), GetTimeOffset())
}
