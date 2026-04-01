package types

import (
	"testing"
)

func TestOutpointString(t *testing.T) {
	hash := NewHash([]byte("test transaction"))
	outpoint := Outpoint{
		Hash:  hash,
		Index: 5,
	}

	str := outpoint.String()
	expected := hash.String() + ":5"

	if str != expected {
		t.Errorf("Expected %s, got %s", expected, str)
	}
}

func TestUTXOString(t *testing.T) {
	hash := NewHash([]byte("test transaction"))
	outpoint := Outpoint{Hash: hash, Index: 0}
	output := &TxOutput{
		Value:        1000000000,
		ScriptPubKey: []byte{0x76, 0xa9, 0x14},
	}

	// Test regular UTXO
	utxo := &UTXO{
		Outpoint:   outpoint,
		Output:     output,
		Height:     100,
		IsCoinbase: false,
	}

	str := utxo.String()
	if str == "" {
		t.Error("UTXO string should not be empty")
	}

	// Test coinbase UTXO
	coinbaseUtxo := &UTXO{
		Outpoint:   outpoint,
		Output:     output,
		Height:     100,
		IsCoinbase: true,
	}

	coinbaseStr := coinbaseUtxo.String()
	if coinbaseStr == str {
		t.Error("Coinbase UTXO string should be different from regular UTXO")
	}

	if !contains(coinbaseStr, "(coinbase)") {
		t.Error("Coinbase UTXO string should contain '(coinbase)'")
	}
}

func TestUTXOIsSpendable(t *testing.T) {
	output := &TxOutput{
		Value:        1000000000,
		ScriptPubKey: []byte{0x76, 0xa9, 0x14},
	}

	// Test regular UTXO
	regularUtxo := &UTXO{
		Outpoint:   Outpoint{Hash: NewHash([]byte("regular")), Index: 0},
		Output:     output,
		Height:     100,
		IsCoinbase: false,
	}

	// Should be spendable with 1 confirmation at height 101 (1 confirmation)
	if !regularUtxo.IsSpendable(101, 1) {
		t.Error("Regular UTXO should be spendable with sufficient confirmations")
	}

	// Should not be spendable at same height (0 confirmations)
	if regularUtxo.IsSpendable(100, 1) {
		t.Error("UTXO should not be spendable at same height")
	}

	// Should not be spendable with insufficient confirmations
	if regularUtxo.IsSpendable(101, 5) {
		t.Error("UTXO should not be spendable with insufficient confirmations")
	}

	// Test coinbase UTXO
	coinbaseUtxo := &UTXO{
		Outpoint:   Outpoint{Hash: NewHash([]byte("coinbase")), Index: 0},
		Output:     output,
		Height:     100,
		IsCoinbase: true,
	}

	// Should not be spendable with less than 100 confirmations
	if coinbaseUtxo.IsSpendable(150, 1) {
		t.Error("Coinbase UTXO should not be spendable with less than 100 confirmations")
	}

	// Should be spendable with 100 confirmations (at height 200)
	if !coinbaseUtxo.IsSpendable(200, 1) {
		t.Error("Coinbase UTXO should be spendable with 100 confirmations")
	}
}

func TestUTXOSetOperations(t *testing.T) {
	utxoSet := NewUTXOSet()

	if utxoSet.Size() != 0 {
		t.Error("New UTXO set should be empty")
	}

	if utxoSet.TotalValue() != 0 {
		t.Error("New UTXO set should have zero total value")
	}

	// Create test UTXOs
	hash1 := NewHash([]byte("transaction 1"))
	hash2 := NewHash([]byte("transaction 2"))

	outpoint1 := Outpoint{Hash: hash1, Index: 0}
	outpoint2 := Outpoint{Hash: hash2, Index: 1}

	utxo1 := &UTXO{
		Outpoint: outpoint1,
		Output: &TxOutput{
			Value:        1000000000,
			ScriptPubKey: []byte{0x76, 0xa9, 0x14, 0x01},
		},
		Height:     100,
		IsCoinbase: false,
	}

	utxo2 := &UTXO{
		Outpoint: outpoint2,
		Output: &TxOutput{
			Value:        2000000000,
			ScriptPubKey: []byte{0x76, 0xa9, 0x14, 0x02},
		},
		Height:     101,
		IsCoinbase: true,
	}

	// Test Add operation
	utxoSet.Add(utxo1)
	utxoSet.Add(utxo2)

	if utxoSet.Size() != 2 {
		t.Errorf("Expected UTXO set size 2, got %d", utxoSet.Size())
	}

	expectedTotal := int64(3000000000)
	if utxoSet.TotalValue() != expectedTotal {
		t.Errorf("Expected total value %d, got %d", expectedTotal, utxoSet.TotalValue())
	}

	// Test Get operation
	retrievedUtxo1, exists := utxoSet.Get(outpoint1)
	if !exists {
		t.Error("Should be able to retrieve added UTXO")
	}

	if retrievedUtxo1.Output.Value != utxo1.Output.Value {
		t.Error("Retrieved UTXO should match original")
	}

	// Test Get non-existent UTXO
	nonExistentOutpoint := Outpoint{Hash: NewHash([]byte("non-existent")), Index: 0}
	_, exists = utxoSet.Get(nonExistentOutpoint)
	if exists {
		t.Error("Should not find non-existent UTXO")
	}

	// Test Remove operation
	utxoSet.Remove(outpoint1)

	if utxoSet.Size() != 1 {
		t.Errorf("Expected UTXO set size 1 after removal, got %d", utxoSet.Size())
	}

	_, exists = utxoSet.Get(outpoint1)
	if exists {
		t.Error("Should not find removed UTXO")
	}

	// Test removing non-existent UTXO (should not crash)
	utxoSet.Remove(nonExistentOutpoint)

	if utxoSet.Size() != 1 {
		t.Error("Removing non-existent UTXO should not change size")
	}
}

func TestUTXOSetGetBalance(t *testing.T) {
	utxoSet := NewUTXOSet()

	// Create UTXOs with same script
	script1 := []byte{0x76, 0xa9, 0x14, 0x01}
	script2 := []byte{0x76, 0xa9, 0x14, 0x02}

	// Regular UTXO with script1
	utxo1 := &UTXO{
		Outpoint: Outpoint{Hash: NewHash([]byte("tx1")), Index: 0},
		Output: &TxOutput{
			Value:        1000000000,
			ScriptPubKey: script1,
		},
		Height:     100,
		IsCoinbase: false,
	}

	// Another regular UTXO with script1
	utxo2 := &UTXO{
		Outpoint: Outpoint{Hash: NewHash([]byte("tx2")), Index: 0},
		Output: &TxOutput{
			Value:        500000000,
			ScriptPubKey: script1,
		},
		Height:     101,
		IsCoinbase: false,
	}

	// UTXO with different script
	utxo3 := &UTXO{
		Outpoint: Outpoint{Hash: NewHash([]byte("tx3")), Index: 0},
		Output: &TxOutput{
			Value:        2000000000,
			ScriptPubKey: script2,
		},
		Height:     102,
		IsCoinbase: false,
	}

	// Coinbase UTXO with script1 (won't be spendable immediately)
	utxo4 := &UTXO{
		Outpoint: Outpoint{Hash: NewHash([]byte("tx4")), Index: 0},
		Output: &TxOutput{
			Value:        3000000000,
			ScriptPubKey: script1,
		},
		Height:     103,
		IsCoinbase: true,
	}

	utxoSet.Add(utxo1)
	utxoSet.Add(utxo2)
	utxoSet.Add(utxo3)
	utxoSet.Add(utxo4)

	// Test balance for script1 at height 110 with 1 confirmation
	balance1 := utxoSet.GetBalance(script1, 110, 1)
	expectedBalance1 := int64(1500000000) // utxo1 + utxo2 (coinbase not spendable yet)

	if balance1 != expectedBalance1 {
		t.Errorf("Expected balance %d for script1, got %d", expectedBalance1, balance1)
	}

	// Test balance for script2
	balance2 := utxoSet.GetBalance(script2, 110, 1)
	expectedBalance2 := int64(2000000000) // utxo3

	if balance2 != expectedBalance2 {
		t.Errorf("Expected balance %d for script2, got %d", expectedBalance2, balance2)
	}

	// Test balance for script1 at height 203 (coinbase should be spendable with 100 confirmations)
	balance3 := utxoSet.GetBalance(script1, 203, 1)
	expectedBalance3 := int64(4500000000) // utxo1 + utxo2 + utxo4

	if balance3 != expectedBalance3 {
		t.Errorf("Expected balance %d for script1 with coinbase, got %d", expectedBalance3, balance3)
	}

	// Test balance for non-existent script
	nonExistentScript := []byte{0x51}
	balance4 := utxoSet.GetBalance(nonExistentScript, 110, 1)

	if balance4 != 0 {
		t.Errorf("Expected zero balance for non-existent script, got %d", balance4)
	}
}

func TestUTXOSetGetUTXOs(t *testing.T) {
	utxoSet := NewUTXOSet()
	script := []byte{0x76, 0xa9, 0x14, 0x01}
	otherScript := []byte{0x76, 0xa9, 0x14, 0x02}

	// Add UTXOs with target script
	utxo1 := &UTXO{
		Outpoint: Outpoint{Hash: NewHash([]byte("tx1")), Index: 0},
		Output:   &TxOutput{Value: 1000000000, ScriptPubKey: script},
		Height:   100,
	}

	utxo2 := &UTXO{
		Outpoint: Outpoint{Hash: NewHash([]byte("tx2")), Index: 0},
		Output:   &TxOutput{Value: 2000000000, ScriptPubKey: script},
		Height:   101,
	}

	// Add UTXO with different script
	utxo3 := &UTXO{
		Outpoint: Outpoint{Hash: NewHash([]byte("tx3")), Index: 0},
		Output:   &TxOutput{Value: 3000000000, ScriptPubKey: otherScript},
		Height:   102,
	}

	utxoSet.Add(utxo1)
	utxoSet.Add(utxo2)
	utxoSet.Add(utxo3)

	// Get UTXOs for target script
	utxos := utxoSet.GetUTXOs(script)

	if len(utxos) != 2 {
		t.Errorf("Expected 2 UTXOs for script, got %d", len(utxos))
	}

	// Verify we got the right UTXOs
	found1, found2 := false, false
	for _, utxo := range utxos {
		if utxo.Output.Value == 1000000000 {
			found1 = true
		}
		if utxo.Output.Value == 2000000000 {
			found2 = true
		}
	}

	if !found1 || !found2 {
		t.Error("Did not find expected UTXOs")
	}

	// Get UTXOs for other script
	otherUtxos := utxoSet.GetUTXOs(otherScript)

	if len(otherUtxos) != 1 {
		t.Errorf("Expected 1 UTXO for other script, got %d", len(otherUtxos))
	}

	// Get UTXOs for non-existent script
	nonExistentUtxos := utxoSet.GetUTXOs([]byte{0x51})

	if len(nonExistentUtxos) != 0 {
		t.Errorf("Expected 0 UTXOs for non-existent script, got %d", len(nonExistentUtxos))
	}
}

func TestUTXOSetGetSpendableUTXOs(t *testing.T) {
	utxoSet := NewUTXOSet()
	script := []byte{0x76, 0xa9, 0x14, 0x01}

	// Regular spendable UTXO
	utxo1 := &UTXO{
		Outpoint:   Outpoint{Hash: NewHash([]byte("tx1")), Index: 0},
		Output:     &TxOutput{Value: 1000000000, ScriptPubKey: script},
		Height:     100,
		IsCoinbase: false,
	}

	// Regular UTXO without enough confirmations
	utxo2 := &UTXO{
		Outpoint:   Outpoint{Hash: NewHash([]byte("tx2")), Index: 0},
		Output:     &TxOutput{Value: 2000000000, ScriptPubKey: script},
		Height:     105,
		IsCoinbase: false,
	}

	// Coinbase UTXO (not spendable yet)
	utxo3 := &UTXO{
		Outpoint:   Outpoint{Hash: NewHash([]byte("tx3")), Index: 0},
		Output:     &TxOutput{Value: 3000000000, ScriptPubKey: script},
		Height:     100,
		IsCoinbase: true,
	}

	utxoSet.Add(utxo1)
	utxoSet.Add(utxo2)
	utxoSet.Add(utxo3)

	// Get spendable UTXOs at height 110 with 5 confirmations
	spendableUtxos := utxoSet.GetSpendableUTXOs(script, 110, 5)

	// Only utxo1 should be spendable (height 100, confirmations = 110-100 = 10 >= 5)
	// utxo2 at height 105 has confirmations = 110-105 = 5 >= 5, so it's also spendable
	// utxo3 (coinbase) at height 100 has 10 confirmations < 100, so NOT spendable
	expectedCount := 2
	if len(spendableUtxos) != expectedCount {
		t.Errorf("Expected %d spendable UTXOs, got %d", expectedCount, len(spendableUtxos))
		for i, utxo := range spendableUtxos {
			t.Errorf("  UTXO %d: value=%d, height=%d, coinbase=%v", i, utxo.Output.Value, utxo.Height, utxo.IsCoinbase)
		}
	}

	// Check that correct UTXOs are returned (should NOT include coinbase)
	foundUtxo1, foundUtxo2, foundCoinbase := false, false, false
	for _, utxo := range spendableUtxos {
		if utxo.Output.Value == 1000000000 {
			foundUtxo1 = true
		}
		if utxo.Output.Value == 2000000000 {
			foundUtxo2 = true
		}
		if utxo.IsCoinbase {
			foundCoinbase = true
		}
	}

	if !foundUtxo1 || !foundUtxo2 {
		t.Error("Expected regular UTXOs not found in spendable UTXOs")
	}

	if foundCoinbase {
		t.Error("Coinbase UTXO should not be spendable yet")
	}

	// Get spendable UTXOs with coinbase maturity
	spendableUtxosWithCoinbase := utxoSet.GetSpendableUTXOs(script, 202, 1)

	// At height 202, coinbase from height 100 should have 102 confirmations (>= 100)
	// So utxo1, utxo2, and utxo3 (coinbase) should all be spendable
	if len(spendableUtxosWithCoinbase) != 3 {
		t.Errorf("Expected 3 spendable UTXOs with coinbase maturity, got %d", len(spendableUtxosWithCoinbase))
	}

	// Test with coinbase maturity at height 203 (coinbase will have 100 confirmations)
	spendableUtxosWithCoinbase = utxoSet.GetSpendableUTXOs(script, 203, 1)
	if len(spendableUtxosWithCoinbase) != 3 {
		t.Errorf("Expected 3 spendable UTXOs with coinbase maturity, got %d", len(spendableUtxosWithCoinbase))
	}
}

func TestUTXOSetConcurrency(t *testing.T) {
	utxoSet := NewUTXOSet()

	// Test concurrent access (basic smoke test)
	// In a real implementation, you might use goroutines here
	utxo1 := &UTXO{
		Outpoint: Outpoint{Hash: NewHash([]byte("concurrent1")), Index: 0},
		Output:   &TxOutput{Value: 1000000000, ScriptPubKey: []byte{0x76}},
		Height:   100,
	}

	utxo2 := &UTXO{
		Outpoint: Outpoint{Hash: NewHash([]byte("concurrent2")), Index: 0},
		Output:   &TxOutput{Value: 2000000000, ScriptPubKey: []byte{0x76}},
		Height:   101,
	}

	// Add and remove operations
	utxoSet.Add(utxo1)
	utxoSet.Add(utxo2)

	size := utxoSet.Size()
	if size != 2 {
		t.Errorf("Expected size 2, got %d", size)
	}

	totalValue := utxoSet.TotalValue()
	if totalValue != 3000000000 {
		t.Errorf("Expected total value 3000000000, got %d", totalValue)
	}

	utxoSet.Remove(utxo1.Outpoint)

	finalSize := utxoSet.Size()
	if finalSize != 1 {
		t.Errorf("Expected final size 1, got %d", finalSize)
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func BenchmarkUTXOSetAdd(b *testing.B) {
	utxoSet := NewUTXOSet()
	utxo := &UTXO{
		Outpoint: Outpoint{Hash: NewHash([]byte("benchmark")), Index: 0},
		Output:   &TxOutput{Value: 1000000000, ScriptPubKey: []byte{0x76}},
		Height:   100,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		utxo.Outpoint.Index = uint32(i)
		utxoSet.Add(utxo)
	}
}

func BenchmarkUTXOSetGet(b *testing.B) {
	utxoSet := NewUTXOSet()
	outpoint := Outpoint{Hash: NewHash([]byte("benchmark")), Index: 0}
	utxo := &UTXO{
		Outpoint: outpoint,
		Output:   &TxOutput{Value: 1000000000, ScriptPubKey: []byte{0x76}},
		Height:   100,
	}
	utxoSet.Add(utxo)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = utxoSet.Get(outpoint)
	}
}
