package consensus

import (
	"sort"
	"sync"
	"time"
)

// TimeData manages network-adjusted time
type TimeData struct {
	mu         sync.RWMutex
	timeOffset int64 // Offset in seconds from system time
	samples    []timeOffsetSample
	knownPeers map[string]bool // Track peers we've received time from
}

type timeOffsetSample struct {
	peerID string
	offset int64
}

// Global time data instance
var globalTimeData = &TimeData{
	knownPeers: make(map[string]bool),
}

// GetAdjustedTime returns the current time adjusted by network peer offset
// Matches legacy GetAdjustedTime() from timedata.cpp
func GetAdjustedTime() uint32 {
	globalTimeData.mu.RLock()
	offset := globalTimeData.timeOffset
	globalTimeData.mu.RUnlock()

	return uint32(time.Now().Unix() + offset)
}

// GetAdjustedTimeAsTime returns network-adjusted time as time.Time
// Use this when you need time.Duration comparisons (e.g., time.Since)
func GetAdjustedTimeAsTime() time.Time {
	globalTimeData.mu.RLock()
	offset := globalTimeData.timeOffset
	globalTimeData.mu.RUnlock()

	return time.Now().Add(time.Duration(offset) * time.Second)
}

// GetAdjustedTimeUnix returns network-adjusted time as Unix timestamp (int64)
// Use this when comparing with sigTime or other int64 timestamps
func GetAdjustedTimeUnix() int64 {
	globalTimeData.mu.RLock()
	offset := globalTimeData.timeOffset
	globalTimeData.mu.RUnlock()

	return time.Now().Unix() + offset
}

// GetTimeOffset returns the current time offset from network peers
func GetTimeOffset() int64 {
	globalTimeData.mu.RLock()
	defer globalTimeData.mu.RUnlock()
	return globalTimeData.timeOffset
}

// AddTimeData adds a time sample from a peer
// Matches legacy AddTimeData() which maintains median of peer time offsets
func AddTimeData(peerID string, peerTime uint32) {
	now := uint32(time.Now().Unix())
	offsetSample := int64(peerTime) - int64(now)

	globalTimeData.mu.Lock()
	defer globalTimeData.mu.Unlock()

	// Ignore duplicates from same peer (matches legacy)
	if globalTimeData.knownPeers[peerID] {
		return
	}
	globalTimeData.knownPeers[peerID] = true

	// Add sample (legacy keeps max 200 samples)
	globalTimeData.samples = append(globalTimeData.samples, timeOffsetSample{
		peerID: peerID,
		offset: offsetSample,
	})

	// Keep only last 200 samples like legacy
	if len(globalTimeData.samples) > 200 {
		globalTimeData.samples = globalTimeData.samples[len(globalTimeData.samples)-200:]
	}

	// Update offset if we have enough samples (matches legacy: >= 5 samples and odd number)
	if len(globalTimeData.samples) >= 5 && len(globalTimeData.samples)%2 == 1 {
		// Calculate median
		offsets := make([]int64, len(globalTimeData.samples))
		for i, s := range globalTimeData.samples {
			offsets[i] = s.offset
		}
		sort.Slice(offsets, func(i, j int) bool {
			return offsets[i] < offsets[j]
		})

		median := offsets[len(offsets)/2]

		// Only accept offsets within 70 minutes (legacy: 70 * 60 seconds)
		if abs64(median) < 70*60 {
			globalTimeData.timeOffset = median
		} else {
			globalTimeData.timeOffset = 0
		}
	}
}

// ResetTimeData clears all time offset data
func ResetTimeData() {
	globalTimeData.mu.Lock()
	defer globalTimeData.mu.Unlock()

	globalTimeData.timeOffset = 0
	globalTimeData.samples = nil
	globalTimeData.knownPeers = make(map[string]bool)
}

// GetMedianTimePast calculates the median timestamp of the last nMedianTimeSpan blocks
// This matches the legacy GetMedianTimePast() from chain.h
func GetMedianTimePast(getBlockTime func(height uint32) (uint32, error), currentHeight uint32) (uint32, error) {
	const nMedianTimeSpan = 11

	if currentHeight == 0 {
		return 0, nil
	}

	timestamps := make([]uint32, 0, nMedianTimeSpan)

	// Get timestamps from last 11 blocks (or fewer if at genesis)
	for i := 0; i < nMedianTimeSpan && currentHeight > uint32(i); i++ {
		blockTime, err := getBlockTime(currentHeight - uint32(i))
		if err != nil {
			if i == 0 {
				return 0, err // Must have at least current block
			}
			break
		}
		timestamps = append(timestamps, blockTime)
	}

	if len(timestamps) == 0 {
		return 0, nil
	}

	// Sort and return median
	sort.Slice(timestamps, func(i, j int) bool {
		return timestamps[i] < timestamps[j]
	})

	return timestamps[len(timestamps)/2], nil
}

// ValidateBlockTime validates block timestamp according to consensus rules
// Matches legacy validation from main.cpp CheckBlock()
func ValidateBlockTime(blockTime uint32, medianTimePast uint32, isPoS bool) error {
	// Block time must be greater than median of last 11 blocks
	if blockTime <= medianTimePast {
		return ErrTimeTooOld
	}

	// Check future time limit
	// Legacy: PoS blocks can be 3 minutes in future, PoW blocks 2 hours
	now := GetAdjustedTime()
	var maxFuture uint32
	if isPoS {
		maxFuture = now + 180 // 3 minutes for PoS
	} else {
		maxFuture = now + 7200 // 2 hours for PoW
	}

	if blockTime > maxFuture {
		return ErrTimeTooNew
	}

	return nil
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}