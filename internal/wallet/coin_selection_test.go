package wallet

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twins-dev/twins-core/pkg/types"
)

// TestCoinSelectionKnapsack tests the knapsack algorithm with various UTXO distributions
func TestCoinSelectionKnapsack(t *testing.T) {
	tests := []struct {
		name        string
		utxoValues  []int64 // UTXO values in satoshis
		target      int64   // target amount
		wantSuccess bool
	}{
		{
			name:        "exact match single coin",
			utxoValues:  []int64{100000000, 200000000, 500000000}, // 1, 2, 5 TWINS
			target:      200000000,                                 // 2 TWINS
			wantSuccess: true,
		},
		{
			name:        "need subset of coins",
			utxoValues:  []int64{100000000, 200000000, 300000000, 400000000}, // 1,2,3,4 TWINS
			target:      500000000,                                           // 5 TWINS
			wantSuccess: true,
		},
		{
			name:        "many small UTXOs",
			utxoValues:  []int64{10000000, 10000000, 10000000, 10000000, 10000000, 10000000, 10000000, 10000000, 10000000, 10000000}, // 10x 0.1 TWINS
			target:      50000000,                                                                                                     // 0.5 TWINS
			wantSuccess: true,
		},
		{
			name:        "insufficient funds",
			utxoValues:  []int64{100000000, 200000000}, // 1, 2 TWINS
			target:      500000000,                      // 5 TWINS
			wantSuccess: false,
		},
		{
			name:        "single large coin for small target",
			utxoValues:  []int64{1000000000}, // 10 TWINS
			target:      100000000,            // 1 TWINS
			wantSuccess: true,
		},
		{
			name:        "prefer fewer inputs over many small",
			utxoValues:  []int64{50000000, 50000000, 50000000, 50000000, 300000000}, // 4x0.5 + 3 TWINS
			target:      200000000,                                                   // 2 TWINS
			wantSuccess: true,
		},
		{
			name:        "zero-conf change UTXO in tier 3",
			utxoValues:  []int64{500000000}, // 5 TWINS (will be zero-conf change)
			target:      100000000,           // 1 TWINS
			wantSuccess: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := createIsolatedWallet(t)
			w.cachedChainHeight = 1000

			// Add UTXOs
			w.mu.Lock()
			for i, val := range tt.utxoValues {
				op := makeOutpoint(byte(i+1), 0)
				addTestUTXO(w, op, val, 900) // height 900 = 101 confirmations
			}
			w.mu.Unlock()

			// Run selection
			selected, err := w.SelectUTXOs(tt.target, 6, 2)

			// Print diagnostic info
			t.Logf("\n=== %s ===\n", tt.name)
			t.Logf("  Target:     %d satoshis (%.8f TWINS)\n", tt.target, float64(tt.target)/1e8)
			t.Logf("  UTXOs in:   %d\n", len(tt.utxoValues))
			totalIn := int64(0)
			for _, v := range tt.utxoValues {
				totalIn += v
			}
			t.Logf("  Total in:   %d satoshis (%.8f TWINS)\n", totalIn, float64(totalIn)/1e8)

			if err != nil {
				t.Logf("  Result:     ERROR - %v\n", err)
				if tt.wantSuccess {
					t.Errorf("expected success, got error: %v", err)
				}
				return
			}

			totalSelected := int64(0)
			for _, u := range selected {
				totalSelected += u.Output.Value
			}
			change := totalSelected - tt.target
			t.Logf("  Selected:   %d UTXOs\n", len(selected))
			t.Logf("  Total sel:  %d satoshis (%.8f TWINS)\n", totalSelected, float64(totalSelected)/1e8)
			t.Logf("  Change:     %d satoshis (%.8f TWINS)\n", change, float64(change)/1e8)
			for i, u := range selected {
				t.Logf("    UTXO[%d]: %d satoshis (%.8f TWINS)\n", i, u.Output.Value, float64(u.Output.Value)/1e8)
			}

			if !tt.wantSuccess {
				t.Errorf("expected failure, got success with %d UTXOs", len(selected))
				return
			}

			assert.GreaterOrEqual(t, totalSelected, tt.target, "selected total should cover target")
		})
	}
}

// TestCoinSelection3TierFallback verifies the 3-tier confirmation fallback
func TestCoinSelection3TierFallback(t *testing.T) {
	w := createIsolatedWallet(t)
	w.cachedChainHeight = 1000
	w.config.SpendZeroConfChange = true

	w.mu.Lock()
	// UTXO with 101 confs (height 900) - eligible in tier 1 (1,6)
	addTestUTXO(w, makeOutpoint(0x01, 0), 50000000, 900) // 0.5 TWINS

	// UTXO with 3 confs (height 998) - NOT eligible tier 1, eligible tier 2 (1,1)
	addTestUTXO(w, makeOutpoint(0x02, 0), 200000000, 998) // 2 TWINS

	// Zero-conf change UTXO (height > chain = unconfirmed) - eligible tier 3 (0,1)
	op3 := makeOutpoint(0x03, 0)
	w.utxos[op3] = &UTXO{
		Outpoint:    op3,
		Output:      &types.TxOutput{Value: 300000000, ScriptPubKey: []byte{0x76, 0xa9}},
		BlockHeight: 1001, // future = 0 confirmations
		Spendable:   true,
		IsChange:    true, // This is a change output
		Address:     "TestAddr1",
	}
	w.mu.Unlock()

	t.Log("\n=== 3-Tier Fallback Test ===")

	// Test tier 1: target 0.3 TWINS - only 0.5 UTXO (101 confs) is eligible at tier 1
	selected, err := w.SelectUTXOs(30000000, 6, 2)
	require.NoError(t, err)
	t.Logf("  Tier 1 (target 0.3): %d UTXOs selected, total=%d\n", len(selected), sumUTXOs(selected))
	assert.Equal(t, 1, len(selected))

	// Test tier 2: target 2 TWINS - needs the 3-conf UTXO (falls through to tier 2)
	selected, err = w.SelectUTXOs(200000000, 6, 2)
	require.NoError(t, err)
	t.Logf("  Tier 2 (target 2.0): %d UTXOs selected, total=%d\n", len(selected), sumUTXOs(selected))

	// Test tier 3: target 5 TWINS - needs all including zero-conf change
	selected, err = w.SelectUTXOs(500000000, 6, 2)
	require.NoError(t, err)
	t.Logf("  Tier 3 (target 5.0): %d UTXOs selected, total=%d\n", len(selected), sumUTXOs(selected))
	assert.Equal(t, 3, len(selected))
}

// TestTxSizePreEstimateCheck tests that oversized transactions are rejected before signing
func TestTxSizePreEstimateCheck(t *testing.T) {
	w := createIsolatedWallet(t)
	w.cachedChainHeight = 1000

	// Create 600 UTXOs - 600 * 190 = 114000 bytes > 100000 MaxStandardTxSize
	w.mu.Lock()
	utxos := make([]*UTXO, 0, 600)
	for i := 0; i < 600; i++ {
		op := makeOutpoint(byte(i%256), uint32(i/256))
		u := &UTXO{
			Outpoint:    op,
			Output:      &types.TxOutput{Value: 100000, ScriptPubKey: []byte{0x76, 0xa9}},
			BlockHeight: 900,
			Spendable:   true,
			Address:     "TestAddr1",
		}
		w.utxos[op] = u
		utxos = append(utxos, u)
	}
	w.mu.Unlock()

	recipients := map[string]int64{
		"DTestAddr1111111111111111111111": 50000000, // 0.5 TWINS
	}

	_, _, err := w.BuildTransaction(utxos, recipients)
	t.Logf("\n=== TX Size Pre-Estimate Check ===\n")
	t.Logf("  Inputs:     %d\n", len(utxos))
	t.Logf("  Est size:   %d bytes\n", 190*len(utxos)+34*len(recipients)+34+10)
	t.Logf("  Max size:   %d bytes\n", MaxStandardTxSize)
	if err != nil {
		t.Logf("  Result:     REJECTED - %v\n", err)
	} else {
		t.Logf("  Result:     ACCEPTED (unexpected)\n")
	}
	assert.Error(t, err, "should reject oversized transaction")
	assert.Contains(t, err.Error(), "exceeds maximum")
}

// TestP2PMessageSizeConstants verifies the protocol constants
func TestP2PMessageSizeConstants(t *testing.T) {
	t.Log("\n=== P2P Message Size Constants ===")
	t.Logf("  MaxStandardTxSize (wallet):  %d bytes (%.1f KB)\n", MaxStandardTxSize, float64(MaxStandardTxSize)/1024)
	// Mempool constant is in a different package, just verify wallet one
	assert.Equal(t, 100_000, MaxStandardTxSize)
	assert.Equal(t, int64(1_000_000), CentThreshold)
	t.Logf("  CentThreshold:               %d satoshis (%.8f TWINS)\n", CentThreshold, float64(CentThreshold)/1e8)
}

// TestCoinSelectionSizeAwareFallback verifies that when knapsack selects too many
// small UTXOs (exceeding MaxStandardTxSize), the size-aware fallback in SelectUTXOs
// switches to largest-first selection that picks fewer, larger coins.
func TestCoinSelectionSizeAwareFallback(t *testing.T) {
	w := createIsolatedWallet(t)
	w.cachedChainHeight = 1000

	w.mu.Lock()
	// Create 1500 small UTXOs (each 100,000 sat = 0.001 TWINS)
	// Total: 150,000,000 sat = 1.5 TWINS
	// Knapsack needs ~1000 of them for 100M target = ~190KB TX (way over 100KB limit)
	for i := 0; i < 1500; i++ {
		op := makeOutpoint(byte(i%256), uint32(i/256))
		addTestUTXO(w, op, 100000, 900) // height 900 = 101 confirmations
	}
	// Create 2 large UTXOs (each 500,000,000 sat = 5 TWINS)
	addTestUTXO(w, makeOutpoint(0xFE, 0), 500000000, 900)
	addTestUTXO(w, makeOutpoint(0xFF, 0), 500000000, 900)
	w.mu.Unlock()

	t.Log("\n=== Size-Aware Fallback Test ===")

	// Target 1 TWINS = 100,000,000 sat
	// Knapsack would select hundreds of small UTXOs (oversized TX),
	// but the fallback should pick 1 large UTXO instead.
	selected, err := w.SelectUTXOs(100000000, 6, 2)
	require.NoError(t, err)

	totalSelected := sumUTXOs(selected)
	estimatedSize := 190*len(selected) + 34*2 + 10

	t.Logf("  Target:     100000000 satoshis (1.0 TWINS)\n")
	t.Logf("  Selected:   %d UTXOs\n", len(selected))
	t.Logf("  Total sel:  %d satoshis (%.8f TWINS)\n", totalSelected, float64(totalSelected)/1e8)
	t.Logf("  Est size:   %d bytes (max: %d)\n", estimatedSize, MaxStandardTxSize)

	assert.GreaterOrEqual(t, totalSelected, int64(100000000), "selected total should cover target")
	assert.LessOrEqual(t, len(selected), 10, "should use few large UTXOs, not hundreds of small ones")
	assert.LessOrEqual(t, estimatedSize, MaxStandardTxSize, "estimated TX size should be within limits")
}

func sumUTXOs(utxos []*UTXO) int64 {
	total := int64(0)
	for _, u := range utxos {
		total += u.Output.Value
	}
	return total
}
