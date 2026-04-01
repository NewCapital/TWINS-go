package consensus

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetAdjustedTime(t *testing.T) {
	// Reset time data for clean test
	ResetTimeData()

	// Without any peer data, should return current time
	now := uint32(time.Now().Unix())
	adjusted := GetAdjustedTime()
	assert.InDelta(t, now, adjusted, 1, "Should be close to current time")

	// Add some time samples from peers
	AddTimeData("peer1", now+10)
	AddTimeData("peer2", now+12)
	AddTimeData("peer3", now+8)
	AddTimeData("peer4", now+11)
	AddTimeData("peer5", now+9) // Need 5 samples and odd count

	// Should use median offset (which is 10)
	adjusted = GetAdjustedTime()
	expectedOffset := int64(10)
	assert.Equal(t, expectedOffset, GetTimeOffset())
}

func TestAddTimeData(t *testing.T) {
	ResetTimeData()

	now := uint32(time.Now().Unix())

	// Test duplicate peer is ignored
	AddTimeData("peer1", now+5)
	offset1 := GetTimeOffset()
	AddTimeData("peer1", now+100) // Should be ignored
	offset2 := GetTimeOffset()
	assert.Equal(t, offset1, offset2, "Duplicate peer should be ignored")

	// Test large offset is rejected (>70 minutes)
	ResetTimeData()
	AddTimeData("peer1", now+5000) // Way more than 70 minutes
	AddTimeData("peer2", now+5001)
	AddTimeData("peer3", now+5002)
	AddTimeData("peer4", now+5003)
	AddTimeData("peer5", now+5004)

	assert.Equal(t, int64(0), GetTimeOffset(), "Large offsets should be rejected")
}

func TestGetMedianTimePast(t *testing.T) {
	// Mock function to return block times
	blockTimes := map[uint32]uint32{
		0:  1000,
		1:  1060,
		2:  1120,
		3:  1180,
		4:  1240,
		5:  1300,
		6:  1360,
		7:  1420,
		8:  1480,
		9:  1540,
		10: 1600,
		11: 1660,
	}

	getBlockTime := func(height uint32) (uint32, error) {
		if time, ok := blockTimes[height]; ok {
			return time, nil
		}
		return 0, assert.AnError
	}

	// Test with height 0 (genesis)
	median, err := GetMedianTimePast(getBlockTime, 0)
	assert.NoError(t, err)
	assert.Equal(t, uint32(0), median)

	// Test with height 11 (should use all 11 blocks)
	median, err = GetMedianTimePast(getBlockTime, 11)
	assert.NoError(t, err)
	// Median of [1060, 1120, 1180, 1240, 1300, 1360, 1420, 1480, 1540, 1600, 1660]
	// When sorted: [1060, 1120, 1180, 1240, 1300, 1360, 1420, 1480, 1540, 1600, 1660]
	// Median is at index 5 = 1360
	assert.Equal(t, uint32(1360), median)

	// Test with height 5 (fewer than 11 blocks)
	median, err = GetMedianTimePast(getBlockTime, 5)
	assert.NoError(t, err)
	// Should use blocks 1-5: [1060, 1120, 1180, 1240, 1300]
	// Median is at index 2 = 1180
	assert.Equal(t, uint32(1180), median)
}

func TestValidateBlockTime(t *testing.T) {
	// Reset time data for predictable tests
	ResetTimeData()

	now := GetAdjustedTime()
	medianTime := now - 3600 // 1 hour ago

	tests := []struct {
		name        string
		blockTime   uint32
		medianTime  uint32
		isPoS       bool
		shouldError bool
		errorType   error
	}{
		{
			name:        "Valid PoW block time",
			blockTime:   now + 60, // 1 minute in future
			medianTime:  medianTime,
			isPoS:       false,
			shouldError: false,
		},
		{
			name:        "Valid PoS block time",
			blockTime:   now + 60, // 1 minute in future
			medianTime:  medianTime,
			isPoS:       true,
			shouldError: false,
		},
		{
			name:        "Block time before median",
			blockTime:   medianTime - 1,
			medianTime:  medianTime,
			isPoS:       false,
			shouldError: true,
			errorType:   ErrTimeTooOld,
		},
		{
			name:        "Block time equals median",
			blockTime:   medianTime,
			medianTime:  medianTime,
			isPoS:       false,
			shouldError: true,
			errorType:   ErrTimeTooOld,
		},
		{
			name:        "PoW block too far in future",
			blockTime:   now + 7201, // Over 2 hours
			medianTime:  medianTime,
			isPoS:       false,
			shouldError: true,
			errorType:   ErrTimeTooNew,
		},
		{
			name:        "PoS block too far in future",
			blockTime:   now + 181, // Over 3 minutes
			medianTime:  medianTime,
			isPoS:       true,
			shouldError: true,
			errorType:   ErrTimeTooNew,
		},
		{
			name:        "PoW block at max future limit",
			blockTime:   now + 7200, // Exactly 2 hours
			medianTime:  medianTime,
			isPoS:       false,
			shouldError: false,
		},
		{
			name:        "PoS block at max future limit",
			blockTime:   now + 180, // Exactly 3 minutes
			medianTime:  medianTime,
			isPoS:       true,
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBlockTime(tt.blockTime, tt.medianTime, tt.isPoS)
			if tt.shouldError {
				assert.Error(t, err)
				if tt.errorType != nil {
					assert.Equal(t, tt.errorType, err)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTimeOffsetWithManyPeers(t *testing.T) {
	ResetTimeData()

	now := uint32(time.Now().Unix())

	// Add 200+ samples to test the limit
	for i := 0; i < 250; i++ {
		peerID := fmt.Sprintf("peer%d", i)
		// Create a distribution around +30 seconds offset
		offset := int64(25 + (i % 11))
		AddTimeData(peerID, now+uint32(offset))
	}

	// Should keep only last 200 samples
	// But offset calculation happens on odd numbers, so check if it's reasonable
	offset := GetTimeOffset()
	assert.InDelta(t, 30, offset, 6, "Offset should be around 30 seconds")
}