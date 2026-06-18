package wallet

import "testing"

// TestMatchesAmountFilter locks the amount-bound filter semantics shared by
// the GUI core (TWINS at the API boundary, satoshis here in wallet land). The
// helper is symmetric across both bounds: a `0` argument means "no
// constraint" on that side. Edge inclusivity: amount == min and amount == max
// both PASS — the bounds are inclusive.
func TestMatchesAmountFilter(t *testing.T) {
	tests := []struct {
		name      string
		absAmount float64
		min       float64
		max       float64
		want      bool
	}{
		// Both bounds disabled (zero) → match-all
		{"both zero matches zero amount", 0, 0, 0, true},
		{"both zero matches positive amount", 100, 0, 0, true},
		{"both zero matches huge amount", 1e18, 0, 0, true},

		// Only min set (max == 0 means no upper bound)
		{"only min: below min rejects", 4, 5, 0, false},
		{"only min: equal min passes (inclusive lower)", 5, 5, 0, true},
		{"only min: above min passes", 10, 5, 0, true},
		{"only min: very large passes", 1e18, 5, 0, true},

		// Only max set (min == 0 means no lower bound)
		{"only max: zero passes (no lower bound)", 0, 0, 10, true},
		{"only max: below max passes", 5, 0, 10, true},
		{"only max: equal max passes (inclusive upper)", 10, 0, 10, true},
		{"only max: above max rejects", 11, 0, 10, false},

		// Both bounds set — in range / out of range / edges
		{"both: below min rejects", 4, 5, 10, false},
		{"both: equal min passes", 5, 5, 10, true},
		{"both: in range passes", 7, 5, 10, true},
		{"both: equal max passes", 10, 5, 10, true},
		{"both: above max rejects", 11, 5, 10, false},

		// Degenerate equal bounds — accepts only the exact value
		{"both equal: exact match passes", 5, 5, 5, true},
		{"both equal: off by one below rejects", 4, 5, 5, false},
		{"both equal: off by one above rejects", 6, 5, 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesAmountFilter(tt.absAmount, tt.min, tt.max)
			if got != tt.want {
				t.Errorf("matchesAmountFilter(%v, %v, %v) = %v, want %v",
					tt.absAmount, tt.min, tt.max, got, tt.want)
			}
		})
	}
}
