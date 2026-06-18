package binary

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twins-dev/twins-core/pkg/types"
)

// makeTestAddressBinary builds a synthetic 21-byte addressBinary (netID +
// hash160) where the hash160 is deterministically derived from seed.
func makeTestAddressBinary(seed byte) []byte {
	addr := make([]byte, 21)
	addr[0] = 0x1e // arbitrary netID; storage layer does not validate it
	for i := 1; i < 21; i++ {
		addr[i] = seed
	}
	return addr
}

// makeTestTxHash builds a synthetic types.Hash where every byte is seed.
func makeTestTxHash(seed byte) types.Hash {
	var h types.Hash
	for i := range h {
		h[i] = seed
	}
	return h
}

// TestGetAddressAggregates_BasicAggregation indexes 3 entries for the same
// address across 2 distinct transactions and asserts that the aggregate
// values match: received = sum of !IsInput values, sent = sum of IsInput
// values, txCount = unique txhash count, min/maxHeight bracket the entries,
// HasHeights == true.
func TestGetAddressAggregates_BasicAggregation(t *testing.T) {
	stor := newTestStorage(t)

	addrBin := makeTestAddressBinary(0xAA)
	txA := makeTestTxHash(0x01)
	txB := makeTestTxHash(0x02)
	blockHashA := makeTestTxHash(0x11)
	blockHashB := makeTestTxHash(0x12)

	// txA: 2 outputs to addr (received) at height 10
	require.NoError(t, stor.IndexTransactionByAddress(addrBin, txA, 10, 0, 100, false, blockHashA))
	require.NoError(t, stor.IndexTransactionByAddress(addrBin, txA, 10, 1, 50, false, blockHashA))
	// txB: 1 input from addr (sent) at height 20
	require.NoError(t, stor.IndexTransactionByAddress(addrBin, txB, 20, 0, 30, true, blockHashB))

	agg, err := stor.GetAddressAggregates(addrBin)
	require.NoError(t, err)

	assert.Equal(t, uint64(150), agg.TotalReceivedSat, "received = 100 + 50 from txA outputs")
	assert.Equal(t, uint64(30), agg.TotalSentSat, "sent = 30 from txB input")
	assert.Equal(t, 2, agg.TxCount, "two distinct txhashes: txA and txB")
	assert.Equal(t, uint32(10), agg.MinHeight)
	assert.Equal(t, uint32(20), agg.MaxHeight)
	assert.True(t, agg.HasHeights)
}

// TestGetAddressAggregates_EmptyAddress confirms an address with zero
// index entries returns a zero-valued struct with HasHeights == false
// and a nil error (the caller treats HasHeights==false as "unknown
// timestamps" and skips the GetBlockByHeight lookups).
func TestGetAddressAggregates_EmptyAddress(t *testing.T) {
	stor := newTestStorage(t)

	addrBin := makeTestAddressBinary(0xBB)
	agg, err := stor.GetAddressAggregates(addrBin)
	require.NoError(t, err)

	assert.Equal(t, uint64(0), agg.TotalReceivedSat)
	assert.Equal(t, uint64(0), agg.TotalSentSat)
	assert.Equal(t, 0, agg.TxCount)
	assert.Equal(t, uint32(0), agg.MinHeight)
	assert.Equal(t, uint32(0), agg.MaxHeight)
	assert.False(t, agg.HasHeights, "HasHeights must be false on empty address")
}

// TestGetAddressAggregates_MultipleOutputsSameTx verifies the bug-fix
// arithmetic: a tx with 2 outputs to the same address must contribute its
// scalar values exactly once each — 100 + 100 = 200, NOT 400. The prior
// GetAddressStats implementation in GoCoreClient (replaced in this task)
// over-counted because it iterated the index entries AND, on each
// iteration, summed all matching outputs in the full transaction — for
// 2 entries × 2 matching outputs that produced 4× the correct value.
// TxCount must also be 1 (one distinct txhash), not 2 (entry count).
func TestGetAddressAggregates_MultipleOutputsSameTx(t *testing.T) {
	stor := newTestStorage(t)

	addrBin := makeTestAddressBinary(0xCC)
	txA := makeTestTxHash(0x21)
	blockHashA := makeTestTxHash(0x31)

	// One transaction, two outputs to the SAME address.
	require.NoError(t, stor.IndexTransactionByAddress(addrBin, txA, 5, 0, 100, false, blockHashA))
	require.NoError(t, stor.IndexTransactionByAddress(addrBin, txA, 5, 1, 100, false, blockHashA))

	agg, err := stor.GetAddressAggregates(addrBin)
	require.NoError(t, err)

	assert.Equal(t, uint64(200), agg.TotalReceivedSat, "must be 100+100=200, NOT the prior buggy 400")
	assert.Equal(t, uint64(0), agg.TotalSentSat)
	assert.Equal(t, 1, agg.TxCount, "one distinct txhash; entry count was 2")
	assert.Equal(t, uint32(5), agg.MinHeight)
	assert.Equal(t, uint32(5), agg.MaxHeight)
	assert.True(t, agg.HasHeights)
}

// TestIndexTransactionByAddress_NegativeValueOverflowsToHugeUint64 pins down
// the storage-layer trap that motivated the address-history overflow bug fix
// in indexTransactionAddresses (internal/blockchain/unified_processor.go).
//
// IndexTransactionByAddress accepts an int64 value but stores it as uint64
// via a direct cast (batch.go:717 `Value: uint64(value)`). Passing a negative
// int64 — as the buggy caller did via `-spentOutput.Output.Value` for spent
// inputs — produces a huge two's-complement uint64 that GetAddressAggregates
// then sums into TotalSentSat, inflating the GUI's "Total Sent" field.
//
// The storage API contract is: callers MUST pass positive sats; direction
// (received vs sent) is conveyed by the IsInput flag, not by the sign of
// Value. This test prevents anyone from "fixing" the storage layer by adding
// sign-preservation logic and locks in the contract from the storage side.
func TestIndexTransactionByAddress_NegativeValueOverflowsToHugeUint64(t *testing.T) {
	stor := newTestStorage(t)

	addrBin := makeTestAddressBinary(0xDE)
	txA := makeTestTxHash(0x55)
	blockHash := makeTestTxHash(0x66)

	// Mimic the buggy caller: hand a NEGATED positive sat value to the API.
	require.NoError(t, stor.IndexTransactionByAddress(addrBin, txA, 1, 0, -1000, true, blockHash))

	agg, err := stor.GetAddressAggregates(addrBin)
	require.NoError(t, err)

	// uint64(int64(-1000)) == 2^64 - 1000 == 18446744073709550616.
	// This is exactly the inflation we observed on the Address Details page
	// (≈184.467e9 TWINS once divided by 1e8 satoshis-per-TWINS).
	assert.Equal(t, uint64(18446744073709550616), agg.TotalSentSat,
		"negative int64 value casts to overflowed uint64 — storage layer must not be 'fixed' to preserve sign; callers must pass positive sats and use IsInput for direction")
}

// TestGetAddressAggregates_InvalidAddressBinaryLength rejects malformed
// input rather than scanning a wrong prefix.
func TestGetAddressAggregates_InvalidAddressBinaryLength(t *testing.T) {
	stor := newTestStorage(t)

	_, err := stor.GetAddressAggregates([]byte{0x1e, 0x00, 0x00}) // 3 bytes, not 21
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid address binary length")
}
