package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFeeRate(t *testing.T) {
	// Test NewFeeRate
	feeRate := NewFeeRate(100000) // 100,000 satoshis per KB
	assert.Equal(t, int64(100000), feeRate.GetFeePerKB())

	// Test GetFee for various sizes
	tests := []struct {
		name     string
		size     int
		expected int64
	}{
		{"1 byte", 1, 100},    // 100000 * 1 / 1000 = 100
		{"100 bytes", 100, 10000},
		{"250 bytes", 250, 25000},
		{"500 bytes", 500, 50000},
		{"1000 bytes (1KB)", 1000, 100000},
		{"1500 bytes", 1500, 150000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fee := feeRate.GetFee(tt.size)
			assert.Equal(t, tt.expected, fee)
		})
	}

	// Test zero fee rate
	zeroRate := NewFeeRate(0)
	assert.Equal(t, int64(0), zeroRate.GetFee(1000))
}

func TestNewFeeRateFromAmount(t *testing.T) {
	tests := []struct {
		name          string
		feePaid       int64
		txSize        int
		expectedPerKB int64
	}{
		{"1000 satoshis for 500 bytes", 1000, 500, 2000},
		{"5000 satoshis for 250 bytes", 5000, 250, 20000},
		{"100000 satoshis for 1000 bytes", 100000, 1000, 100000},
		{"0 fee", 0, 1000, 0},
		{"0 size", 1000, 0, 0}, // Should return 0 rate
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			feeRate := NewFeeRateFromAmount(tt.feePaid, tt.txSize)
			assert.Equal(t, tt.expectedPerKB, feeRate.GetFeePerKB())
		})
	}
}

func TestIsDust(t *testing.T) {
	// Use default minimum relay fee (100,000 satoshis per KB)
	minRelayFee := NewFeeRate(100000)

	// Calculate expected dust threshold
	// Size to spend = 34 (output) + 148 (input) = 182 bytes
	// Fee to spend = 100000 * 182 / 1000 = 18200 satoshis
	// Dust threshold = 3 * 18200 = 54600 satoshis
	expectedDustThreshold := int64(54600)

	tests := []struct {
		name     string
		value    int64
		expected bool
	}{
		{"0 value", 0, true},
		{"1 satoshi", 1, true},
		{"1000 satoshis", 1000, true},
		{"50000 satoshis", 50000, true},
		{"54599 satoshis (just below threshold)", 54599, true},
		{"54600 satoshis (exactly threshold)", 54600, false},
		{"55000 satoshis", 55000, false},
		{"100000 satoshis", 100000, false},
		{"1 TWINS (100000000)", 100000000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isDust := IsDust(tt.value, minRelayFee)
			assert.Equal(t, tt.expected, isDust)
		})
	}

	// Verify dust threshold calculation
	threshold := GetDustThreshold(minRelayFee)
	assert.Equal(t, expectedDustThreshold, threshold)
}

func TestIsFeeTooHigh(t *testing.T) {
	minRelayFee := NewFeeRate(100000) // 100,000 satoshis per KB

	// For 1000 byte tx: min fee = 100000, max = 100000 * 10000 = 1,000,000,000
	tests := []struct {
		name     string
		fee      int64
		txSize   int
		expected bool
	}{
		{"Normal fee", 100000, 1000, false},
		{"High but reasonable fee", 1000000, 1000, false},
		{"10x fee", 1000000, 1000, false},
		{"100x fee", 10000000, 1000, false},
		{"1000x fee", 100000000, 1000, false},
		{"9999x fee", 999900000, 1000, false},
		{"10000x fee (exactly limit)", 1000000000, 1000, false},
		{"10001x fee (over limit)", 1000100000, 1000, true},
		{"Insanely high fee", 10000000000, 1000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isTooHigh := IsFeeTooHigh(tt.fee, tt.txSize, minRelayFee)
			assert.Equal(t, tt.expected, isTooHigh)
		})
	}
}

func TestCalculateMinFee(t *testing.T) {
	minRelayFee := NewFeeRate(100000) // 100,000 satoshis per KB

	tests := []struct {
		name     string
		txSize   int
		expected int64
	}{
		{"0 bytes", 0, 100000},        // GetFee returns minimum when size would give 0
		{"1 byte", 1, 100},           // 100000 * 1 / 1000 = 100
		{"250 bytes", 250, 25000},
		{"500 bytes", 500, 50000},
		{"1000 bytes", 1000, 100000},
		{"2000 bytes", 2000, 200000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			minFee := CalculateMinFee(tt.txSize, minRelayFee)
			assert.Equal(t, tt.expected, minFee)
		})
	}
}

func TestDustThresholdWithDifferentFeeRates(t *testing.T) {
	tests := []struct {
		name              string
		feeRatePerKB      int64
		expectedThreshold int64
	}{
		{"10000 satoshis/KB", 10000, 5460},     // Legacy minimum
		{"50000 satoshis/KB", 50000, 27300},
		{"100000 satoshis/KB", 100000, 54600},  // Current default
		{"200000 satoshis/KB", 200000, 109200},
		{"1000000 satoshis/KB", 1000000, 546000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			feeRate := NewFeeRate(tt.feeRatePerKB)
			threshold := GetDustThreshold(feeRate)
			assert.Equal(t, tt.expectedThreshold, threshold)
		})
	}
}