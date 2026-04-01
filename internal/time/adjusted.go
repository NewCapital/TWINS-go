// Package time implements network-adjusted time for cryptocurrency consensus.
// It collects time offsets from connected peers and calculates a median offset
// to synchronize the local clock with network time.
//
// This is critical for PoS consensus where block timestamps must be validated
// against network time, not just local system time.
//
// Legacy: timedata.cpp in Bitcoin/PIVX/TWINS
package time

import (
	"net"
	"sort"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	// Maximum allowed time adjustment (70 minutes, as in Bitcoin)
	// Legacy: 70 * 60 seconds
	MaxTimeAdjustment = 70 * time.Minute

	// Maximum number of time samples to keep
	// Legacy: BITCOIN_TIMEDATA_MAX_SAMPLES = 200
	MaxTimeSamples = 200

	// Minimum number of samples before applying adjustment
	// Legacy: requires at least 5 samples AND odd count
	MinTimeSamples = 5

	// WarningThreshold is the threshold (5 minutes) beyond which we warn
	// that the system clock may be wrong.
	// Legacy: 5 * 60 seconds
	WarningThreshold = 5 * 60
)

// TimeData manages network time offset calculation
type TimeData struct {
	mu       sync.RWMutex
	samples  []int64 // time offsets in seconds
	logger   *logrus.Logger
	adjusted int64 // median adjustment in seconds

	// knownPeers tracks which peers have already contributed time data
	// to prevent duplicates. Key is peer IP address string.
	// Legacy: static set<CNetAddr> setKnown in timedata.cpp
	knownPeers map[string]struct{}

	// warningIssued tracks if we've already warned about clock issues
	// Legacy: static bool fDone in timedata.cpp
	warningIssued bool
}

// NewTimeData creates a new TimeData instance
func NewTimeData(logger *logrus.Logger) *TimeData {
	return &TimeData{
		samples:    make([]int64, 0, MaxTimeSamples),
		logger:     logger,
		knownPeers: make(map[string]struct{}),
	}
}

// AddTimeSample adds a time sample from a peer
func (td *TimeData) AddTimeSample(peerTime time.Time, localTime time.Time) {
	td.mu.Lock()
	defer td.mu.Unlock()

	// Calculate offset (peer time - local time)
	offset := peerTime.Unix() - localTime.Unix()

	// Add to samples
	if len(td.samples) >= MaxTimeSamples {
		// Remove oldest sample
		td.samples = td.samples[1:]
	}
	td.samples = append(td.samples, offset)

	// Recalculate median if we have enough samples
	if len(td.samples) >= MinTimeSamples {
		td.recalculateMedian()
	}

	td.logger.WithFields(logrus.Fields{
		"peer_time":   peerTime.Format(time.RFC3339),
		"local_time":  localTime.Format(time.RFC3339),
		"offset":      offset,
		"num_samples": len(td.samples),
		"adjustment":  td.adjusted,
	}).Debug("Added time sample")
}

// AddTimeDataFromPeer adds a time sample from a peer with IP deduplication.
// This matches legacy AddTimeData() in timedata.cpp which ignores duplicate IPs.
// peerIP is the peer's IP address (for deduplication).
// peerTimestamp is the peer's reported Unix timestamp.
func (td *TimeData) AddTimeDataFromPeer(peerIP net.IP, peerTimestamp int64) {
	td.mu.Lock()
	defer td.mu.Unlock()

	// Ignore duplicates (legacy behavior)
	ipKey := peerIP.String()
	if _, exists := td.knownPeers[ipKey]; exists {
		td.logger.WithField("peer", ipKey).Debug("Ignoring duplicate time data from peer")
		return
	}
	td.knownPeers[ipKey] = struct{}{}

	// Calculate offset: peer time - local time
	localTime := time.Now().Unix()
	offsetSample := peerTimestamp - localTime

	// Add sample to the list
	if len(td.samples) >= MaxTimeSamples {
		// Replace oldest entry (circular buffer behavior)
		td.samples = td.samples[1:]
	}
	td.samples = append(td.samples, offsetSample)

	td.logger.WithFields(logrus.Fields{
		"peer":    ipKey,
		"samples": len(td.samples),
		"offset":  offsetSample,
		"minutes": offsetSample / 60,
	}).Debug("Added time data sample from peer")

	// Update median offset if we have enough samples and odd count
	// Legacy: only updates when vTimeOffsets.size() >= 5 && vTimeOffsets.size() % 2 == 1
	td.recalculateMedianLegacy()
}

// recalculateMedian calculates the median time offset
func (td *TimeData) recalculateMedian() {
	if len(td.samples) == 0 {
		td.adjusted = 0
		return
	}

	// Make a sorted copy
	sorted := make([]int64, len(td.samples))
	copy(sorted, td.samples)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	// Calculate median
	var median int64
	n := len(sorted)
	if n%2 == 0 {
		median = (sorted[n/2-1] + sorted[n/2]) / 2
	} else {
		median = sorted[n/2]
	}

	// Apply bounds
	maxAdj := int64(MaxTimeAdjustment.Seconds())
	if median > maxAdj {
		median = maxAdj
		td.logger.WithField("clamped_to", maxAdj).Warn("Time adjustment clamped to maximum")
	} else if median < -maxAdj {
		median = -maxAdj
		td.logger.WithField("clamped_to", -maxAdj).Warn("Time adjustment clamped to minimum")
	}

	td.adjusted = median

	if median != 0 {
		td.logger.WithFields(logrus.Fields{
			"median_offset": median,
			"num_samples":   len(td.samples),
		}).Debug("Network time offset updated")
	}
}

// recalculateMedianLegacy calculates the median using legacy rules.
// Must be called with mutex held.
// Legacy behavior from timedata.cpp:
// - Only updates when count >= 5 AND count is odd
// - If median > 70 minutes, sets offset to 0 and warns once
func (td *TimeData) recalculateMedianLegacy() {
	count := len(td.samples)

	// Need at least MinTimeSamples AND odd count (legacy behavior)
	// Legacy: if (vTimeOffsets.size() >= 5 && vTimeOffsets.size() % 2 == 1)
	if count < MinTimeSamples || count%2 == 0 {
		return
	}

	// Calculate median
	sorted := make([]int64, count)
	copy(sorted, td.samples)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})
	median := sorted[count/2]

	// Only apply offset if within max allowed range
	maxAdj := int64(MaxTimeAdjustment.Seconds())
	if abs64(median) < maxAdj {
		td.adjusted = median
		td.logger.WithFields(logrus.Fields{
			"offset":  td.adjusted,
			"minutes": td.adjusted / 60,
			"samples": count,
		}).Debug("Updated network time offset")
	} else {
		// Offset too large, reset to 0 and warn
		td.adjusted = 0

		if !td.warningIssued {
			// Check if any peer has a time within warning threshold
			hasCloseTime := false
			for _, offset := range sorted {
				if offset != 0 && abs64(offset) < WarningThreshold {
					hasCloseTime = true
					break
				}
			}

			if !hasCloseTime {
				td.warningIssued = true
				td.logger.Warn("Please check that your computer's date and time are correct! " +
					"If your clock is wrong, TWINS Core will not work properly.")
			}
		}
	}

	// Log sorted offsets for debugging (legacy behavior)
	td.logger.WithFields(logrus.Fields{
		"final_offset":  td.adjusted,
		"final_minutes": td.adjusted / 60,
		"samples":       count,
	}).Debug("Time offset calculation complete")
}

// abs64 returns the absolute value of an int64.
func abs64(n int64) int64 {
	if n >= 0 {
		return n
	}
	return -n
}

// GetAdjustedTime returns the current time adjusted by network offset
func (td *TimeData) GetAdjustedTime() time.Time {
	td.mu.RLock()
	adjustment := td.adjusted
	td.mu.RUnlock()

	return time.Now().Add(time.Duration(adjustment) * time.Second)
}

// GetAdjustedUnix returns the current Unix timestamp adjusted by network offset
func (td *TimeData) GetAdjustedUnix() int64 {
	return td.GetAdjustedTime().Unix()
}

// GetTimeOffset returns the current time adjustment in seconds
func (td *TimeData) GetTimeOffset() int64 {
	td.mu.RLock()
	defer td.mu.RUnlock()
	return td.adjusted
}

// Reset clears all time samples and resets adjustment
func (td *TimeData) Reset() {
	td.mu.Lock()
	defer td.mu.Unlock()

	td.samples = td.samples[:0]
	td.adjusted = 0
	td.knownPeers = make(map[string]struct{})
	td.warningIssued = false
}

// GetStats returns statistics about time adjustment
func (td *TimeData) GetStats() (samples int, offset int64) {
	td.mu.RLock()
	defer td.mu.RUnlock()
	return len(td.samples), td.adjusted
}

// Global instance for package-level functions
var globalTimeData *TimeData

// InitGlobalTimeData initializes the global time data
func InitGlobalTimeData(logger *logrus.Logger) {
	globalTimeData = NewTimeData(logger)
}

// GetAdjustedTime returns network-adjusted time using global instance
func GetAdjustedTime() time.Time {
	if globalTimeData == nil {
		return time.Now()
	}
	return globalTimeData.GetAdjustedTime()
}

// GetAdjustedUnix returns network-adjusted Unix timestamp using global instance
func GetAdjustedUnix() int64 {
	if globalTimeData == nil {
		return time.Now().Unix()
	}
	return globalTimeData.GetAdjustedUnix()
}

// AddTimeSample adds a peer time sample to global instance
func AddTimeSample(peerTime time.Time, localTime time.Time) {
	if globalTimeData != nil {
		globalTimeData.AddTimeSample(peerTime, localTime)
	}
}

// AddTimeDataFromPeer adds a peer time sample with IP deduplication to global instance.
// This is the preferred method for P2P layer integration (matches legacy timedata.cpp).
func AddTimeDataFromPeer(peerIP net.IP, peerTimestamp int64) {
	if globalTimeData != nil {
		globalTimeData.AddTimeDataFromPeer(peerIP, peerTimestamp)
	}
}

// GetTimeOffset returns the current time offset using global instance
func GetTimeOffset() int64 {
	if globalTimeData == nil {
		return 0
	}
	return globalTimeData.GetTimeOffset()
}