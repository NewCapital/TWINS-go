package blockchain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	binarystore "github.com/twins-dev/twins-core/internal/storage/binary"
	"github.com/twins-dev/twins-core/pkg/types"
)

// buildP2PKHScript constructs a canonical P2PKH scriptPubKey:
//
//	OP_DUP OP_HASH160 <20-byte hash> OP_EQUALVERIFY OP_CHECKSIG
//
// AnalyzeScript classifies this as ScriptTypeP2PKH so the indexer derives
// a non-nil addressBinary and writes an address-history entry.
func buildP2PKHScript(hash160 [20]byte) []byte {
	script := make([]byte, 25)
	script[0] = 0x76 // OP_DUP
	script[1] = 0xa9 // OP_HASH160
	script[2] = 0x14 // push 20 bytes
	copy(script[3:23], hash160[:])
	script[23] = 0x88 // OP_EQUALVERIFY
	script[24] = 0xac // OP_CHECKSIG
	return script
}

// TestIndexTransactionAddresses_InputValueNotOverflowed is the regression
// test for the address-history input-value overflow bug.
//
// Bug summary: indexTransactionAddresses previously passed the spent input
// value to IndexTransactionByAddress NEGATED (-spentOutput.Output.Value).
// IndexTransactionByAddress casts its int64 value parameter to uint64; a
// negated positive int64 becomes a huge uint64 via two's complement
// (uint64(-1000) == 18446744073709550616). GetAddressAggregates sums those
// entries directly into TotalSentSat, inflating the GUI's "Total Sent" by
// roughly 2^64 / 1e8 ≈ 184 billion TWINS per address.
//
// The fix removes the unary minus at unified_processor.go:106. Inputs are
// already disambiguated from outputs by the IsInput=true flag on the entry,
// so the sign was redundant and harmful.
func TestIndexTransactionAddresses_InputValueNotOverflowed(t *testing.T) {
	bc := createTestBlockchain(t)
	defer bc.storage.Close()

	// Build a deterministic P2PKH script for the spender's address.
	var hash160 [20]byte
	for i := range hash160 {
		hash160[i] = 0xAA
	}
	spendScript := buildP2PKHScript(hash160)

	// Derive the expected addressBinary the same way the indexer does, so
	// we can query GetAddressAggregates for the right key after the commit.
	scriptType, scriptHash := binarystore.AnalyzeScript(spendScript)
	require.Equal(t, binarystore.ScriptTypeP2PKH, scriptType, "test script must be P2PKH")
	addrBinary := binarystore.ScriptHashToAddressBinary(scriptType, scriptHash, bc.config.IsTestNet())
	require.NotNil(t, addrBinary, "addressBinary must be derivable from the test P2PKH script")

	// The funding outpoint and its prevOut. The indexer reads the value
	// from precomputedOutputs[input.PreviousOutput], so we can avoid any DB
	// lookup by passing the map directly.
	const inputValueSat int64 = 1_000_000_000 // 10 TWINS
	prevHash := types.Hash{0x10, 0x11, 0x12}
	prevOutpoint := types.Outpoint{Hash: prevHash, Index: 0}
	precomputed := map[types.Outpoint]*types.TxOutput{
		prevOutpoint: {
			Value:        inputValueSat,
			ScriptPubKey: spendScript,
		},
	}

	// Build a non-coinbase tx that spends prevOutpoint and produces a
	// throwaway OP_RETURN-style output. The output has an unrecognized
	// script so AnalyzeScript returns ScriptTypeUnknown and the indexer
	// skips it — we want this test to assert input behaviour only.
	tx := &types.Transaction{
		Version: 1,
		Inputs: []*types.TxInput{
			{
				PreviousOutput: prevOutpoint,
				ScriptSig:      []byte{0x00},
				Sequence:       0xffffffff,
			},
		},
		Outputs: []*types.TxOutput{
			{
				Value:        0,
				ScriptPubKey: []byte{0x6a}, // OP_RETURN — non-addressable
			},
		},
		LockTime: 0,
	}

	const blockHeight uint32 = 42
	const txIndex uint32 = 0
	blockHash := types.Hash{0x20, 0x21, 0x22}

	batch := bc.storage.NewBatch()
	require.NoError(t,
		bc.indexTransactionAddresses(tx, blockHeight, txIndex, batch, blockHash, precomputed),
	)
	require.NoError(t, batch.Commit())

	agg, err := bc.storage.GetAddressAggregates(addrBinary)
	require.NoError(t, err)

	// Without the fix this assertion fails with TotalSentSat ≈ 2^64 -
	// inputValueSat (~1.8e19). With the fix it equals inputValueSat exactly.
	assert.Equal(t, uint64(inputValueSat), agg.TotalSentSat,
		"TotalSentSat must equal the spent input value, not an overflowed two's-complement of -value")
	assert.Equal(t, uint64(0), agg.TotalReceivedSat,
		"output uses non-addressable OP_RETURN script; TotalReceivedSat must remain zero")
	assert.Equal(t, 1, agg.TxCount, "exactly one tx contributed to this address")
	assert.Equal(t, blockHeight, agg.MinHeight)
	assert.Equal(t, blockHeight, agg.MaxHeight)
	assert.True(t, agg.HasHeights)
}

// p2pkhFixture is a per-test address scaffold: the script, its derived
// scriptHash, and the matching network-specific addressBinary. Sharing the
// derivation in one helper keeps the three multi-i/o regression tests below
// readable.
type p2pkhFixture struct {
	script     []byte
	scriptHash [20]byte
	addrBinary []byte
}

func newP2PKHFixture(t *testing.T, bc *BlockChain, seed byte) p2pkhFixture {
	t.Helper()
	var hash160 [20]byte
	for i := range hash160 {
		hash160[i] = seed
	}
	script := buildP2PKHScript(hash160)
	scriptType, scriptHash := binarystore.AnalyzeScript(script)
	require.Equal(t, binarystore.ScriptTypeP2PKH, scriptType, "fixture script must classify as P2PKH")
	addrBinary := binarystore.ScriptHashToAddressBinary(scriptType, scriptHash, bc.config.IsTestNet())
	require.NotNil(t, addrBinary, "fixture addressBinary must be derivable")
	return p2pkhFixture{
		script:     script,
		scriptHash: scriptHash,
		addrBinary: addrBinary,
	}
}

// TestIndexTransactionAddresses_MultiInputFromSameAddress reproduces the
// undercount-on-Sent failure mode the user observed on a heavy
// consolidate-then-withdraw address (1.75M txs, 916K UTXOs).
//
// Under the pre-2026-06-01 schema, all inputs of one tx to one address
// shared a single 0x05 key (the index was the tx-position-in-block, identical
// for every input of the same tx). Each batch.Set overwrote the previous,
// so only the LAST input value survived — Sent was massively undercounted
// for any tx with multiple inputs from the same address.
//
// Under the post-2026-06-01 schema, each input's ioIdx is encoded into the
// key (0x8000 | inIdx), so each input gets a unique key. TotalSentSat is
// the sum of all three input values, not just the last.
func TestIndexTransactionAddresses_MultiInputFromSameAddress(t *testing.T) {
	bc := createTestBlockchain(t)
	defer bc.storage.Close()

	a := newP2PKHFixture(t, bc, 0xBB)

	// Three inputs from address A with distinct funding outpoints.
	const v0, v1, v2 int64 = 100, 200, 300
	op0 := types.Outpoint{Hash: types.Hash{0x10}, Index: 0}
	op1 := types.Outpoint{Hash: types.Hash{0x11}, Index: 0}
	op2 := types.Outpoint{Hash: types.Hash{0x12}, Index: 0}

	precomputed := map[types.Outpoint]*types.TxOutput{
		op0: {Value: v0, ScriptPubKey: a.script},
		op1: {Value: v1, ScriptPubKey: a.script},
		op2: {Value: v2, ScriptPubKey: a.script},
	}

	tx := &types.Transaction{
		Version: 1,
		Inputs: []*types.TxInput{
			{PreviousOutput: op0, ScriptSig: []byte{0x00}, Sequence: 0xffffffff},
			{PreviousOutput: op1, ScriptSig: []byte{0x00}, Sequence: 0xffffffff},
			{PreviousOutput: op2, ScriptSig: []byte{0x00}, Sequence: 0xffffffff},
		},
		Outputs: []*types.TxOutput{
			{Value: 0, ScriptPubKey: []byte{0x6a}}, // OP_RETURN — non-addressable.
		},
		LockTime: 0,
	}

	const blockHeight uint32 = 50
	blockHash := types.Hash{0x30, 0x31}

	batch := bc.storage.NewBatch()
	require.NoError(t, bc.indexTransactionAddresses(tx, blockHeight, 0, batch, blockHash, precomputed))
	require.NoError(t, batch.Commit())

	agg, err := bc.storage.GetAddressAggregates(a.addrBinary)
	require.NoError(t, err)

	assert.Equal(t, uint64(v0+v1+v2), agg.TotalSentSat,
		"all three inputs must contribute to TotalSentSat; pre-fix only the last input survived key collision")
	assert.Equal(t, uint64(0), agg.TotalReceivedSat, "no addressable outputs in this tx")
	assert.Equal(t, 1, agg.TxCount, "TxCount is unique-txhashes, not entry-count")
}

// TestIndexTransactionAddresses_MultiOutputToSameAddressNoOverflow reproduces
// the split-stake overflow that survived the line-106-only fix. A coinstake
// tx with one input and two outputs to the same staker address used to
// produce an entry with `value = output - input` that went negative when
// each output carried only half the staked amount; the int64→uint64 cast
// then stored a huge uint64 that inflated TotalReceivedSat by ~184e9 TWINS
// per overflow entry.
//
// Under the post-2026-06-01 schema, the input lands at key index 0x8000
// and the two outputs at 0x0000 and 0x0001 — three distinct keys, three
// entries, no subtraction. TotalReceivedSat is the sum of the output
// values; TotalSentSat is the input value; the math is honest.
func TestIndexTransactionAddresses_MultiOutputToSameAddressNoOverflow(t *testing.T) {
	bc := createTestBlockchain(t)
	defer bc.storage.Close()

	a := newP2PKHFixture(t, bc, 0xCC)

	const inputVal int64 = 1000
	const outputVal int64 = 500 // each half of the input — net zero, like a stake-split fee scenario.

	op := types.Outpoint{Hash: types.Hash{0x20}, Index: 0}
	precomputed := map[types.Outpoint]*types.TxOutput{
		op: {Value: inputVal, ScriptPubKey: a.script},
	}

	tx := &types.Transaction{
		Version: 1,
		Inputs: []*types.TxInput{
			{PreviousOutput: op, ScriptSig: []byte{0x00}, Sequence: 0xffffffff},
		},
		Outputs: []*types.TxOutput{
			{Value: outputVal, ScriptPubKey: a.script},
			{Value: outputVal, ScriptPubKey: a.script},
		},
		LockTime: 0,
	}

	const blockHeight uint32 = 60
	blockHash := types.Hash{0x40, 0x41}

	batch := bc.storage.NewBatch()
	require.NoError(t, bc.indexTransactionAddresses(tx, blockHeight, 0, batch, blockHash, precomputed))
	require.NoError(t, batch.Commit())

	agg, err := bc.storage.GetAddressAggregates(a.addrBinary)
	require.NoError(t, err)

	assert.Equal(t, uint64(2*outputVal), agg.TotalReceivedSat,
		"both outputs to the same address must be summed; pre-fix output-path subtraction produced negative→overflow")
	assert.Equal(t, uint64(inputVal), agg.TotalSentSat,
		"input is recorded separately under its own unique key")
	assert.Less(t, agg.TotalReceivedSat, uint64(1<<32),
		"sanity: TotalReceivedSat must not be in the overflow magnitude range (≈2^64)")
	assert.Equal(t, 1, agg.TxCount, "one unique txhash")
}

// TestIndexTransactionAddresses_SinglePaymentNet verifies that the
// canonical single-input single-output self-pay (a typical PoS coinstake
// reward, output > input) still produces honest Received and Sent under the
// new no-netting schema. Before 2026-06-01 the entry was collapsed to
// `received = output - input = R` via spentValues subtraction; now each
// input and each output is stored separately and GetAddressAggregates
// derives the net change as Received - Sent.
func TestIndexTransactionAddresses_SinglePaymentNet(t *testing.T) {
	bc := createTestBlockchain(t)
	defer bc.storage.Close()

	a := newP2PKHFixture(t, bc, 0xDD)

	const inputVal int64 = 1000
	const outputVal int64 = 1005 // +5 reward, like a PoS stake.

	op := types.Outpoint{Hash: types.Hash{0x30}, Index: 0}
	precomputed := map[types.Outpoint]*types.TxOutput{
		op: {Value: inputVal, ScriptPubKey: a.script},
	}

	tx := &types.Transaction{
		Version: 1,
		Inputs: []*types.TxInput{
			{PreviousOutput: op, ScriptSig: []byte{0x00}, Sequence: 0xffffffff},
		},
		Outputs: []*types.TxOutput{
			{Value: outputVal, ScriptPubKey: a.script},
		},
		LockTime: 0,
	}

	const blockHeight uint32 = 70
	blockHash := types.Hash{0x50, 0x51}

	batch := bc.storage.NewBatch()
	require.NoError(t, bc.indexTransactionAddresses(tx, blockHeight, 0, batch, blockHash, precomputed))
	require.NoError(t, batch.Commit())

	agg, err := bc.storage.GetAddressAggregates(a.addrBinary)
	require.NoError(t, err)

	assert.Equal(t, uint64(outputVal), agg.TotalReceivedSat,
		"the single output's full value lands in TotalReceivedSat")
	assert.Equal(t, uint64(inputVal), agg.TotalSentSat,
		"the single input's full value lands in TotalSentSat; the prior spentValues subtraction is gone")
	netChange := int64(agg.TotalReceivedSat) - int64(agg.TotalSentSat)
	assert.Equal(t, int64(5), netChange, "net change must equal the stake reward (output - input)")
	assert.Equal(t, 1, agg.TxCount)
}
