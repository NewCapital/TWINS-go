package mocks

import (
	"context"
	"testing"
)

// TestEstimateFee tests basic fee estimation
func TestEstimateFee(t *testing.T) {
	mock := NewMockCoreClient()
	ctx := context.Background()

	if err := mock.Start(ctx); err != nil {
		t.Fatalf("failed to start mock: %v", err)
	}
	defer mock.Stop()

	tests := []struct {
		name        string
		blocks      int
		wantErr     bool
		minExpected float64
		maxExpected float64
	}{
		{
			name:        "fast confirmation (1 block)",
			blocks:      1,
			wantErr:     false,
			minExpected: MinRelayFeeRate,
			maxExpected: FastFeeRate * HighCongestionMultiplier,
		},
		{
			name:        "normal confirmation (3 blocks)",
			blocks:      3,
			wantErr:     false,
			minExpected: MinRelayFeeRate,
			maxExpected: DefaultFeeRate * HighCongestionMultiplier,
		},
		{
			name:        "slow confirmation (6 blocks)",
			blocks:      6,
			wantErr:     false,
			minExpected: MinRelayFeeRate,
			maxExpected: (DefaultFeeRate * 0.5) * HighCongestionMultiplier,
		},
		{
			name:        "very slow confirmation (10 blocks)",
			blocks:      10,
			wantErr:     false,
			minExpected: MinRelayFeeRate,
			maxExpected: (MinRelayFeeRate * 2) * HighCongestionMultiplier,
		},
		{
			name:    "invalid blocks (0)",
			blocks:  0,
			wantErr: true,
		},
		{
			name:    "invalid blocks (negative)",
			blocks:  -1,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fee, err := mock.EstimateFee(tt.blocks)

			if tt.wantErr {
				if err == nil {
					t.Errorf("EstimateFee() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("EstimateFee() unexpected error: %v", err)
				return
			}

			if fee < tt.minExpected || fee > tt.maxExpected {
				t.Errorf("EstimateFee() = %v, want between %v and %v",
					fee, tt.minExpected, tt.maxExpected)
			}

			// Ensure fee meets minimum relay requirement
			if fee < MinRelayFeeRate {
				t.Errorf("EstimateFee() = %v, below minimum relay fee %v",
					fee, MinRelayFeeRate)
			}
		})
	}
}

// TestGetFeeEstimates tests the fee tier helper
func TestGetFeeEstimates(t *testing.T) {
	mock := NewMockCoreClient()
	ctx := context.Background()

	if err := mock.Start(ctx); err != nil {
		t.Fatalf("failed to start mock: %v", err)
	}
	defer mock.Stop()

	estimates, err := mock.GetFeeEstimates()
	if err != nil {
		t.Fatalf("GetFeeEstimates() unexpected error: %v", err)
	}

	// Fast should be highest
	if estimates.Fast < estimates.Normal {
		t.Errorf("Fast fee (%v) should be >= Normal fee (%v)",
			estimates.Fast, estimates.Normal)
	}

	// Normal should be higher than slow
	if estimates.Normal < estimates.Slow {
		t.Errorf("Normal fee (%v) should be >= Slow fee (%v)",
			estimates.Normal, estimates.Slow)
	}

	// All should meet minimum
	if estimates.Fast < MinRelayFeeRate {
		t.Errorf("Fast fee (%v) below minimum (%v)",
			estimates.Fast, MinRelayFeeRate)
	}
	if estimates.Normal < MinRelayFeeRate {
		t.Errorf("Normal fee (%v) below minimum (%v)",
			estimates.Normal, MinRelayFeeRate)
	}
	if estimates.Slow < MinRelayFeeRate {
		t.Errorf("Slow fee (%v) below minimum (%v)",
			estimates.Slow, MinRelayFeeRate)
	}
}

// TestEstimateTransactionSize tests transaction size calculation
func TestEstimateTransactionSize(t *testing.T) {
	tests := []struct {
		name       string
		numInputs  int
		numOutputs int
		want       int
		wantErr    bool
	}{
		{
			name:       "single input, single output",
			numInputs:  1,
			numOutputs: 1,
			want:       TransactionBaseSize + (1 * TransactionInputSize) + (1 * TransactionOutputSize),
			wantErr:    false,
		},
		{
			name:       "two inputs, two outputs",
			numInputs:  2,
			numOutputs: 2,
			want:       TransactionBaseSize + (2 * TransactionInputSize) + (2 * TransactionOutputSize),
			wantErr:    false,
		},
		{
			name:       "five inputs, three outputs",
			numInputs:  5,
			numOutputs: 3,
			want:       TransactionBaseSize + (5 * TransactionInputSize) + (3 * TransactionOutputSize),
			wantErr:    false,
		},
		{
			name:       "zero inputs (invalid)",
			numInputs:  0,
			numOutputs: 1,
			wantErr:    true,
		},
		{
			name:       "zero outputs (invalid)",
			numInputs:  1,
			numOutputs: 0,
			wantErr:    true,
		},
		{
			name:       "negative inputs (invalid)",
			numInputs:  -1,
			numOutputs: 1,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EstimateTransactionSize(tt.numInputs, tt.numOutputs)

			if tt.wantErr {
				if err == nil {
					t.Errorf("EstimateTransactionSize(%d, %d) expected error but got none",
						tt.numInputs, tt.numOutputs)
				}
				return
			}

			if err != nil {
				t.Errorf("EstimateTransactionSize(%d, %d) unexpected error: %v",
					tt.numInputs, tt.numOutputs, err)
				return
			}

			if got != tt.want {
				t.Errorf("EstimateTransactionSize(%d, %d) = %v, want %v",
					tt.numInputs, tt.numOutputs, got, tt.want)
			}
		})
	}
}

// TestCalculateFee tests total fee calculation
func TestCalculateFee(t *testing.T) {
	mock := NewMockCoreClient()
	ctx := context.Background()

	if err := mock.Start(ctx); err != nil {
		t.Fatalf("failed to start mock: %v", err)
	}
	defer mock.Stop()

	// Test with typical transaction (2 inputs, 2 outputs)
	fee, err := mock.CalculateFee(2, 2, 3)
	if err != nil {
		t.Fatalf("CalculateFee() unexpected error: %v", err)
	}

	if fee <= 0 {
		t.Errorf("CalculateFee() = %v, want positive fee", fee)
	}

	// Fee should be reasonable for a ~400 byte transaction
	// At normal congestion (0.0001 TWINS/KB), expect ~0.00004 TWINS
	if fee > 0.001 {
		t.Errorf("CalculateFee() = %v, seems too high for normal transaction", fee)
	}
}

// TestCongestionLevels tests fee estimation with different congestion levels
func TestCongestionLevels(t *testing.T) {
	mock := NewMockCoreClient()
	ctx := context.Background()

	if err := mock.Start(ctx); err != nil {
		t.Fatalf("failed to start mock: %v", err)
	}
	defer mock.Stop()

	// Test with low congestion
	mock.mu.Lock()
	mock.congestionLevel = CongestionLow
	mock.mu.Unlock()

	lowFee, err := mock.EstimateFee(3)
	if err != nil {
		t.Fatalf("EstimateFee() with low congestion error: %v", err)
	}

	// Test with normal congestion
	mock.mu.Lock()
	mock.congestionLevel = CongestionNormal
	mock.mu.Unlock()

	normalFee, err := mock.EstimateFee(3)
	if err != nil {
		t.Fatalf("EstimateFee() with normal congestion error: %v", err)
	}

	// Test with high congestion
	mock.mu.Lock()
	mock.congestionLevel = CongestionHigh
	mock.mu.Unlock()

	highFee, err := mock.EstimateFee(3)
	if err != nil {
		t.Fatalf("EstimateFee() with high congestion error: %v", err)
	}

	// Verify fee ordering: low < normal < high
	if lowFee >= normalFee {
		t.Errorf("Low congestion fee (%v) should be < normal (%v)",
			lowFee, normalFee)
	}
	if normalFee >= highFee {
		t.Errorf("Normal congestion fee (%v) should be < high (%v)",
			normalFee, highFee)
	}

	t.Logf("Fees by congestion level - Low: %v, Normal: %v, High: %v",
		lowFee, normalFee, highFee)
}
