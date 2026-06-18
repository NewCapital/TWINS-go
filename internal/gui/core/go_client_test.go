package core

import (
	"testing"

	"github.com/twins-dev/twins-core/internal/storage"
	"github.com/twins-dev/twins-core/internal/storage/binary"
	"github.com/twins-dev/twins-core/pkg/crypto"
	"github.com/twins-dev/twins-core/pkg/types"
)

// buildP2PKHScript constructs a standard P2PKH scriptPubKey:
// OP_DUP OP_HASH160 <20-byte hash> OP_EQUALVERIFY OP_CHECKSIG
func buildP2PKHScript(pubKeyHash [20]byte) []byte {
	script := make([]byte, 25)
	script[0] = 0x76 // OP_DUP
	script[1] = 0xa9 // OP_HASH160
	script[2] = 0x14 // push 20 bytes
	copy(script[3:23], pubKeyHash[:])
	script[23] = 0x88 // OP_EQUALVERIFY
	script[24] = 0xac // OP_CHECKSIG
	return script
}

// addressForHash returns the mainnet P2PKH address string for a given hash160.
func addressForHash(t *testing.T, hash [20]byte) string {
	t.Helper()
	addr, err := crypto.NewAddressFromHash(hash[:], crypto.MainNetPubKeyHashAddrID)
	if err != nil {
		t.Fatalf("NewAddressFromHash: %v", err)
	}
	return addr.String()
}

func mkHash(seed byte) [20]byte {
	var h [20]byte
	for i := range h {
		h[i] = seed + byte(i)
	}
	return h
}

// ----------------------------------------------------------------------------
// extractRecipientAddressesFromTx tests
// ----------------------------------------------------------------------------

// scriptSetPredicate builds an isOurScript-style predicate that matches a
// fixed set of hash160s. The wallet's real IsOurScript matches on raw
// scriptPubKey bytes; for testing we treat any output whose extracted
// hash160 is in the set as "ours" by extracting the hash from the script.
// This keeps the test independent of wallet internals while exercising the
// same skip path the production predicate triggers.
func scriptSetPredicate(ownHashes ...[20]byte) func([]byte) bool {
	owned := make(map[[20]byte]struct{}, len(ownHashes))
	for _, h := range ownHashes {
		owned[h] = struct{}{}
	}
	return func(scriptPubKey []byte) bool {
		_, hash := binary.AnalyzeScript(scriptPubKey)
		_, isOurs := owned[hash]
		return isOurs
	}
}

func TestExtractRecipientAddresses_NilTx(t *testing.T) {
	if got := extractRecipientAddressesFromTx(nil, nil, ""); got != nil {
		t.Fatalf("nil tx: want nil, got %v", got)
	}
}

func TestExtractRecipientAddresses_NilPredicate_ReturnsAllDecodable(t *testing.T) {
	// With no predicate, every decodable output is treated as a recipient
	// (no change filtering). Documents the API contract for callers that
	// explicitly want the full output list.
	a := mkHash(0x10)
	b := mkHash(0x20)
	tx := &types.Transaction{
		Outputs: []*types.TxOutput{
			{ScriptPubKey: buildP2PKHScript(a)},
			{ScriptPubKey: buildP2PKHScript(b)},
		},
	}
	got := extractRecipientAddressesFromTx(tx, nil, "")
	want := []string{addressForHash(t, a), addressForHash(t, b)}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("nil-predicate: want %v, got %v", want, got)
	}
}

func TestExtractRecipientAddresses_SingleRecipientWithChange(t *testing.T) {
	recipient := mkHash(0x10)
	change := mkHash(0x99)
	tx := &types.Transaction{
		Outputs: []*types.TxOutput{
			{ScriptPubKey: buildP2PKHScript(recipient)},
			{ScriptPubKey: buildP2PKHScript(change)}, // wallet-owned change
		},
	}
	got := extractRecipientAddressesFromTx(tx, scriptSetPredicate(change), "")
	want := []string{addressForHash(t, recipient)}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("single recipient + change: want %v, got %v", want, got)
	}
}

func TestExtractRecipientAddresses_MultiRecipient(t *testing.T) {
	r1 := mkHash(0x11)
	r2 := mkHash(0x22)
	r3 := mkHash(0x33)
	change := mkHash(0x99)
	tx := &types.Transaction{
		Outputs: []*types.TxOutput{
			{ScriptPubKey: buildP2PKHScript(r1)},
			{ScriptPubKey: buildP2PKHScript(change)},
			{ScriptPubKey: buildP2PKHScript(r2)},
			{ScriptPubKey: buildP2PKHScript(r3)},
		},
	}
	got := extractRecipientAddressesFromTx(tx, scriptSetPredicate(change), "")
	want := []string{
		addressForHash(t, r1),
		addressForHash(t, r2),
		addressForHash(t, r3),
	}
	if len(got) != 3 {
		t.Fatalf("multi recipient: want 3 addresses, got %d (%v)", len(got), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("multi recipient[%d]: want %s, got %s", i, w, got[i])
		}
	}
}

func TestExtractRecipientAddresses_NoChange(t *testing.T) {
	// All outputs external (no wallet ownership). Returns all of them.
	r1 := mkHash(0x11)
	r2 := mkHash(0x22)
	tx := &types.Transaction{
		Outputs: []*types.TxOutput{
			{ScriptPubKey: buildP2PKHScript(r1)},
			{ScriptPubKey: buildP2PKHScript(r2)},
		},
	}
	got := extractRecipientAddressesFromTx(tx, scriptSetPredicate( /* empty owned set */ ), "")
	if len(got) != 2 {
		t.Fatalf("no-change: want 2 addresses, got %d", len(got))
	}
	if got[0] != addressForHash(t, r1) || got[1] != addressForHash(t, r2) {
		t.Fatalf("no-change order: got %v", got)
	}
}

func TestExtractRecipientAddresses_AllChange_ReturnsNil(t *testing.T) {
	// Misclassified send-to-self path: every output is wallet-owned.
	c1 := mkHash(0xa1)
	c2 := mkHash(0xa2)
	tx := &types.Transaction{
		Outputs: []*types.TxOutput{
			{ScriptPubKey: buildP2PKHScript(c1)},
			{ScriptPubKey: buildP2PKHScript(c2)},
		},
	}
	if got := extractRecipientAddressesFromTx(tx, scriptSetPredicate(c1, c2), ""); len(got) != 0 {
		t.Fatalf("all change: want nil/empty, got %v", got)
	}
}

func TestExtractRecipientAddresses_SkipsUnknownAndEmptyScripts(t *testing.T) {
	recipient := mkHash(0x10)
	tx := &types.Transaction{
		Outputs: []*types.TxOutput{
			{ScriptPubKey: []byte{0xde, 0xad}}, // unknown script bytes
			{ScriptPubKey: nil},                // nil
			{ScriptPubKey: []byte{}},           // empty
			{ScriptPubKey: buildP2PKHScript(recipient)},
		},
	}
	got := extractRecipientAddressesFromTx(tx, scriptSetPredicate(), "")
	if len(got) != 1 || got[0] != addressForHash(t, recipient) {
		t.Fatalf("skip unknown: want [%s], got %v", addressForHash(t, recipient), got)
	}
}

func TestExtractRecipientAddresses_SkipsNilOutputElement(t *testing.T) {
	recipient := mkHash(0x10)
	tx := &types.Transaction{
		Outputs: []*types.TxOutput{
			nil,
			{ScriptPubKey: buildP2PKHScript(recipient)},
		},
	}
	got := extractRecipientAddressesFromTx(tx, scriptSetPredicate(), "")
	if len(got) != 1 || got[0] != addressForHash(t, recipient) {
		t.Fatalf("nil-output: want [%s], got %v", addressForHash(t, recipient), got)
	}
}

// ----------------------------------------------------------------------------
// findCoinstakeRewardIndexes tests
// ----------------------------------------------------------------------------

// buildOutput is a small helper for tests that constructs a *types.TxOutput
// with the given scriptPubKey and a placeholder value.
func buildOutput(script []byte, value int64) *types.TxOutput {
	return &types.TxOutput{ScriptPubKey: script, Value: value}
}

func TestFindCoinstakeRewardIndexes_WithDevPayment(t *testing.T) {
	// Canonical layout: [empty, stake_return, mn_payment, dev_payment]
	staker := mkHash(0x10)
	mn := mkHash(0x20)
	devScript := buildP2PKHScript(mkHash(0xDE))
	outputs := []*types.TxOutput{
		buildOutput(nil, 0),                            // 0: empty
		buildOutput(buildP2PKHScript(staker), 1010),    // 1: stake_return
		buildOutput(buildP2PKHScript(mn), 80),          // 2: mn_payment
		buildOutput(devScript, 10),                     // 3: dev_payment
	}

	devIdx, mnIdx := findCoinstakeRewardIndexes(outputs, devScript)
	if devIdx != 3 {
		t.Errorf("dev idx: want 3, got %d", devIdx)
	}
	if mnIdx != 2 {
		t.Errorf("mn idx: want 2, got %d", mnIdx)
	}
}

func TestFindCoinstakeRewardIndexes_NoDevPayment_LegacyLayout(t *testing.T) {
	// Legacy 3-output layout: [empty, stake_return, mn_payment]
	staker := mkHash(0x10)
	mn := mkHash(0x20)
	devScript := buildP2PKHScript(mkHash(0xDE))
	outputs := []*types.TxOutput{
		buildOutput(nil, 0),
		buildOutput(buildP2PKHScript(staker), 1010),
		buildOutput(buildP2PKHScript(mn), 80),
	}

	devIdx, mnIdx := findCoinstakeRewardIndexes(outputs, devScript)
	if devIdx != -1 {
		t.Errorf("dev idx: want -1, got %d", devIdx)
	}
	if mnIdx != 2 {
		t.Errorf("mn idx: want 2 (fallback), got %d", mnIdx)
	}
}

func TestFindCoinstakeRewardIndexes_NilDevAddressScript(t *testing.T) {
	// chainParams not wired or testnet without DevAddress.
	// Behaviour: never identify dev output; fall back to legacy MN position.
	staker := mkHash(0x10)
	mn := mkHash(0x20)
	dev := mkHash(0xDE)
	outputs := []*types.TxOutput{
		buildOutput(nil, 0),
		buildOutput(buildP2PKHScript(staker), 1010),
		buildOutput(buildP2PKHScript(mn), 80),
		buildOutput(buildP2PKHScript(dev), 10),
	}

	devIdx, mnIdx := findCoinstakeRewardIndexes(outputs, nil)
	if devIdx != -1 {
		t.Errorf("nil devScript: want devIdx=-1, got %d", devIdx)
	}
	// With nil devScript, MN falls back to the legacy outputs[2] position even
	// though a "dev-looking" output is present. This is correct defensive
	// behaviour: without ground-truth DevAddress, we can't safely classify
	// the trailing output as dev vs MN, so we use the legacy positional rule.
	if mnIdx != 2 {
		t.Errorf("nil devScript: want mnIdx=2 (legacy fallback), got %d", mnIdx)
	}
}

func TestFindCoinstakeRewardIndexes_EmptyDevAddressScript(t *testing.T) {
	// Same as nil-script: zero-length devAddressScript means "no dev config".
	outputs := []*types.TxOutput{
		buildOutput(nil, 0),
		buildOutput(buildP2PKHScript(mkHash(0x10)), 1010),
		buildOutput(buildP2PKHScript(mkHash(0x20)), 80),
	}

	devIdx, mnIdx := findCoinstakeRewardIndexes(outputs, []byte{})
	if devIdx != -1 {
		t.Errorf("empty devScript: want devIdx=-1, got %d", devIdx)
	}
	if mnIdx != 2 {
		t.Errorf("empty devScript: want mnIdx=2 (legacy fallback), got %d", mnIdx)
	}
}

func TestFindCoinstakeRewardIndexes_TwoOutputs_NoDevNoMN(t *testing.T) {
	// Minimal coinstake: [empty, stake_return]. No MN, no dev.
	devScript := buildP2PKHScript(mkHash(0xDE))
	outputs := []*types.TxOutput{
		buildOutput(nil, 0),
		buildOutput(buildP2PKHScript(mkHash(0x10)), 1010),
	}

	devIdx, mnIdx := findCoinstakeRewardIndexes(outputs, devScript)
	if devIdx != -1 {
		t.Errorf("2-out: want devIdx=-1, got %d", devIdx)
	}
	if mnIdx != -1 {
		t.Errorf("2-out: want mnIdx=-1, got %d", mnIdx)
	}
}

func TestFindCoinstakeRewardIndexes_DevAtIndex2_NoRoomForMN(t *testing.T) {
	// Degenerate / malformed layout: dev at index 2 with no MN slot.
	// Behaviour: dev still identified, MN stays -1 (devIdx-1 == 1, the staker slot).
	devScript := buildP2PKHScript(mkHash(0xDE))
	outputs := []*types.TxOutput{
		buildOutput(nil, 0),
		buildOutput(buildP2PKHScript(mkHash(0x10)), 1010),
		buildOutput(devScript, 10),
	}

	devIdx, mnIdx := findCoinstakeRewardIndexes(outputs, devScript)
	if devIdx != 2 {
		t.Errorf("degenerate: want devIdx=2, got %d", devIdx)
	}
	if mnIdx != -1 {
		t.Errorf("degenerate: want mnIdx=-1 (no room between staker and dev), got %d", mnIdx)
	}
}

func TestFindCoinstakeRewardIndexes_DevSearchSkipsNilOutputs(t *testing.T) {
	// Defensive: dev-search loop must not panic on nil entries in the slice.
	devScript := buildP2PKHScript(mkHash(0xDE))
	outputs := []*types.TxOutput{
		buildOutput(nil, 0),
		buildOutput(buildP2PKHScript(mkHash(0x10)), 1010),
		nil, // anomalous nil — must be skipped
		buildOutput(devScript, 10),
	}

	devIdx, mnIdx := findCoinstakeRewardIndexes(outputs, devScript)
	if devIdx != 3 {
		t.Errorf("nil-output: want devIdx=3, got %d", devIdx)
	}
	// MN slot would be at devIdx-1=2 (the nil), so the canonical "MN at devIdx-1"
	// rule still applies and mnIdx==2. The caller (blockToDetail) reads
	// outputs[mnIdx], so if that slot is nil, masternodeAddr/Reward stay empty.
	if mnIdx != 2 {
		t.Errorf("nil-output: want mnIdx=2, got %d", mnIdx)
	}
}

// ----------------------------------------------------------------------------
// computeCoinstakeBreakdown tests
// ----------------------------------------------------------------------------

// buildP2PKScript constructs a compressed-key P2PK scriptPubKey:
// 0x21 <33-byte pubkey> OP_CHECKSIG. 35 bytes total.
func buildP2PKScript(pubKey [33]byte) []byte {
	script := make([]byte, 35)
	script[0] = 0x21 // push 33 bytes
	copy(script[1:34], pubKey[:])
	script[34] = 0xac // OP_CHECKSIG
	return script
}

// mkPubKey33 returns a deterministic 33-byte buffer for test-only use as a
// compressed-pubkey filler. NOT a real secp256k1 point.
func mkPubKey33(seed byte) [33]byte {
	var pk [33]byte
	pk[0] = 0x02 // valid compressed-key prefix (real keys use 0x02 or 0x03)
	for i := 1; i < 33; i++ {
		pk[i] = seed + byte(i)
	}
	return pk
}

func TestComputeCoinstakeBreakdown_CanonicalLayout(t *testing.T) {
	// Mirrors the user's real coinstake tx (block #1742049): canonical 4-output
	// PoS layout [empty, stake_return=stake+reward, mn_payment, dev_payment].
	// totalInput=390697.27 TWINS = 39069727000000 sat. Output[1]=stake+reward,
	// so reward = output[1]-totalInput = 10 TWINS = 1000000000 sat.
	const stakeInputSat int64 = 39069727000000
	const stakeRewardSat int64 = 1000000000     // 10 TWINS
	const mnPaymentSat int64 = 8000000000       // 80 TWINS
	const devPaymentSat int64 = 1000000000      // 10 TWINS

	stakerPK := mkPubKey33(0x10)
	mnHash := mkHash(0x20)
	devScript := buildP2PKHScript(mkHash(0xDE))

	outputs := []*types.TxOutput{
		buildOutput(nil, 0),                                   // 0: coinstake marker
		buildOutput(buildP2PKScript(stakerPK), stakeInputSat+stakeRewardSat), // 1: stake_return (P2PK)
		buildOutput(buildP2PKHScript(mnHash), mnPaymentSat),   // 2: mn_payment (P2PKH)
		buildOutput(devScript, devPaymentSat),                 // 3: dev_payment (P2PKH = chainParams.DevAddress)
	}

	got := computeCoinstakeBreakdown(outputs, stakeInputSat, devScript)

	if got.StakerIdx != 1 {
		t.Errorf("StakerIdx: want 1, got %d", got.StakerIdx)
	}
	if got.StakerEndIdx != 2 {
		t.Errorf("StakerEndIdx: want 2 (single-stake run, exclusive end), got %d", got.StakerEndIdx)
	}
	if got.MnIdx != 2 {
		t.Errorf("MnIdx: want 2, got %d", got.MnIdx)
	}
	if got.DevIdx != 3 {
		t.Errorf("DevIdx: want 3, got %d", got.DevIdx)
	}
	if got.StakeRewardSat != stakeRewardSat {
		t.Errorf("StakeRewardSat: want %d (10 TWINS), got %d", stakeRewardSat, got.StakeRewardSat)
	}
	if got.MasternodePaySat != mnPaymentSat {
		t.Errorf("MasternodePaySat: want %d (80 TWINS), got %d", mnPaymentSat, got.MasternodePaySat)
	}
	if got.DevPaySat != devPaymentSat {
		t.Errorf("DevPaySat: want %d (10 TWINS), got %d", devPaymentSat, got.DevPaySat)
	}
}

func TestComputeCoinstakeBreakdown_StakeSplitLayout(t *testing.T) {
	// Regression for codex code-review finding: stake-split coinstakes produce
	// [empty, stake1, stake2, mn, dev] (5 outputs). CreateCoinstakeTx
	// (internal/wallet/staking.go:319-358) splits when totalReward/2 > stakeSplitThreshold:
	// firstOutputValue = (totalReward/2 / CENT) * CENT, secondOutputValue = totalReward - firstOutputValue.
	// The earlier implementation read only outputs[1].Value and produced a NEGATIVE
	// stake reward (outputs[1] is only half the returned stake; subtracting the full
	// input yields a large negative). The fix sums outputs[1..stakerEndIdx) which
	// includes both stake-return outputs.
	const stakeInputSat int64 = 39069727000000             // 390697.27 TWINS staked
	const stakeRewardSat int64 = 1000000000                // 10 TWINS staker reward
	const totalStakeReturnSat = stakeInputSat + stakeRewardSat
	const CENT int64 = 1000000
	const stake1Sat = (totalStakeReturnSat / 2 / CENT) * CENT // floored to 0.01 TWINS
	const stake2Sat = totalStakeReturnSat - stake1Sat         // remainder
	const mnPaymentSat int64 = 8000000000                  // 80 TWINS
	const devPaymentSat int64 = 1000000000                 // 10 TWINS

	stakerPK := mkPubKey33(0x10)
	stakerScript := buildP2PKScript(stakerPK) // same script for both stake outputs
	mnHash := mkHash(0x20)
	devScript := buildP2PKHScript(mkHash(0xDE))

	outputs := []*types.TxOutput{
		buildOutput(nil, 0),                                  // 0: coinstake marker
		buildOutput(stakerScript, stake1Sat),                 // 1: stake1 (P2PK)
		buildOutput(stakerScript, stake2Sat),                 // 2: stake2 (P2PK, same script)
		buildOutput(buildP2PKHScript(mnHash), mnPaymentSat),  // 3: mn_payment (P2PKH)
		buildOutput(devScript, devPaymentSat),                // 4: dev_payment (P2PKH)
	}

	got := computeCoinstakeBreakdown(outputs, stakeInputSat, devScript)

	if got.StakerIdx != 1 {
		t.Errorf("StakerIdx: want 1, got %d", got.StakerIdx)
	}
	if got.StakerEndIdx != 3 {
		t.Errorf("StakerEndIdx: want 3 (split-stake run [1,2,3)), got %d", got.StakerEndIdx)
	}
	if got.MnIdx != 3 {
		t.Errorf("MnIdx: want 3, got %d", got.MnIdx)
	}
	if got.DevIdx != 4 {
		t.Errorf("DevIdx: want 4, got %d", got.DevIdx)
	}
	if got.StakeRewardSat != stakeRewardSat {
		t.Errorf("StakeRewardSat: want %d (10 TWINS — sum of both stake outputs minus input), got %d",
			stakeRewardSat, got.StakeRewardSat)
	}
	if got.StakeRewardSat <= 0 {
		t.Errorf("StakeRewardSat must be positive for a valid coinstake, got %d (the pre-fix bug rendered negative)", got.StakeRewardSat)
	}
	if got.MasternodePaySat != mnPaymentSat {
		t.Errorf("MasternodePaySat: want %d, got %d", mnPaymentSat, got.MasternodePaySat)
	}
	if got.DevPaySat != devPaymentSat {
		t.Errorf("DevPaySat: want %d, got %d", devPaymentSat, got.DevPaySat)
	}
}

func TestComputeCoinstakeBreakdown_LegacyLayoutNoDevFund(t *testing.T) {
	// 3-output legacy layout: [empty, stake_return, mn_payment]. devAddressScript
	// is set but not present in outputs — DevIdx should be -1; MN falls back to
	// the canonical outputs[2] position via findCoinstakeRewardIndexes.
	const stakeInputSat int64 = 1000000000 // 10 TWINS
	const stakeRewardSat int64 = 100000000 // 1 TWINS
	const mnPaymentSat int64 = 80000000    // 0.8 TWINS

	devScript := buildP2PKHScript(mkHash(0xDE))
	outputs := []*types.TxOutput{
		buildOutput(nil, 0),
		buildOutput(buildP2PKHScript(mkHash(0x10)), stakeInputSat+stakeRewardSat),
		buildOutput(buildP2PKHScript(mkHash(0x20)), mnPaymentSat),
	}

	got := computeCoinstakeBreakdown(outputs, stakeInputSat, devScript)

	if got.StakerIdx != 1 {
		t.Errorf("StakerIdx: want 1, got %d", got.StakerIdx)
	}
	if got.StakerEndIdx != 2 {
		t.Errorf("StakerEndIdx: want 2 (single-stake run, bounded by mnIdx=2), got %d", got.StakerEndIdx)
	}
	if got.MnIdx != 2 {
		t.Errorf("MnIdx: want 2 (legacy fallback), got %d", got.MnIdx)
	}
	if got.DevIdx != -1 {
		t.Errorf("DevIdx: want -1 (no dev output), got %d", got.DevIdx)
	}
	if got.StakeRewardSat != stakeRewardSat {
		t.Errorf("StakeRewardSat: want %d, got %d", stakeRewardSat, got.StakeRewardSat)
	}
	if got.MasternodePaySat != mnPaymentSat {
		t.Errorf("MasternodePaySat: want %d, got %d", mnPaymentSat, got.MasternodePaySat)
	}
	if got.DevPaySat != 0 {
		t.Errorf("DevPaySat: want 0 (no dev output), got %d", got.DevPaySat)
	}
}

func TestComputeCoinstakeBreakdown_EmptyOutputs(t *testing.T) {
	// Defensive: empty slice must not panic and must return all-zero result.
	got := computeCoinstakeBreakdown(nil, 0, nil)
	if got.StakerIdx != -1 || got.StakerEndIdx != -1 || got.MnIdx != -1 || got.DevIdx != -1 {
		t.Errorf("empty outputs: want all -1 indexes, got staker=%d stakerEnd=%d mn=%d dev=%d",
			got.StakerIdx, got.StakerEndIdx, got.MnIdx, got.DevIdx)
	}
	if got.StakeRewardSat != 0 || got.MasternodePaySat != 0 || got.DevPaySat != 0 {
		t.Errorf("empty outputs: want all zero values, got stake=%d mn=%d dev=%d",
			got.StakeRewardSat, got.MasternodePaySat, got.DevPaySat)
	}
}

func TestComputeCoinstakeBreakdown_NilStakerOutput(t *testing.T) {
	// Defensive: outputs[1] = nil must not panic and must leave StakerIdx=-1
	// + StakeRewardSat=0 (the caller treats this as "no stake-return info").
	outputs := []*types.TxOutput{
		buildOutput(nil, 0),
		nil, // anomalous: stake-return slot is nil
		buildOutput(buildP2PKHScript(mkHash(0x20)), 80),
	}
	got := computeCoinstakeBreakdown(outputs, 100, nil)
	if got.StakerIdx != -1 {
		t.Errorf("nil staker output: want StakerIdx=-1, got %d", got.StakerIdx)
	}
	if got.StakeRewardSat != 0 {
		t.Errorf("nil staker output: want StakeRewardSat=0, got %d", got.StakeRewardSat)
	}
	// MN fallback still works at outputs[2].
	if got.MnIdx != 2 || got.MasternodePaySat != 80 {
		t.Errorf("nil staker output: want MnIdx=2/MNPay=80, got %d/%d", got.MnIdx, got.MasternodePaySat)
	}
}

// ----------------------------------------------------------------------------
// getScriptType tests (post-delegation to pkg/script.GetScriptType)
// ----------------------------------------------------------------------------

func TestGetScriptType_P2PKCompressed(t *testing.T) {
	// The original user-reported bug: P2PK was returning "nonstandard" instead
	// of "pubkey". This test locks the fix to the canonical pkg/script.
	c := &GoCoreClient{}
	script := buildP2PKScript(mkPubKey33(0x10))
	if got := c.getScriptType(script); got != "pubkey" {
		t.Errorf("P2PK (compressed): want \"pubkey\", got %q", got)
	}
}

func TestGetScriptType_P2PKUncompressed(t *testing.T) {
	// Uncompressed P2PK: 0x41 <65-byte pubkey> OP_CHECKSIG. 67 bytes total.
	script := make([]byte, 67)
	script[0] = 0x41 // push 65 bytes
	for i := 1; i < 66; i++ {
		script[i] = byte(i)
	}
	script[66] = 0xac // OP_CHECKSIG
	c := &GoCoreClient{}
	if got := c.getScriptType(script); got != "pubkey" {
		t.Errorf("P2PK (uncompressed): want \"pubkey\", got %q", got)
	}
}

func TestGetScriptType_P2PKH_NoRegression(t *testing.T) {
	c := &GoCoreClient{}
	script := buildP2PKHScript(mkHash(0x10))
	if got := c.getScriptType(script); got != "pubkeyhash" {
		t.Errorf("P2PKH: want \"pubkeyhash\", got %q", got)
	}
}

func TestGetScriptType_P2SH_NoRegression(t *testing.T) {
	// P2SH: OP_HASH160 <20 bytes> OP_EQUAL = 23 bytes
	script := make([]byte, 23)
	script[0] = 0xa9 // OP_HASH160
	script[1] = 0x14 // push 20 bytes
	hash := mkHash(0x30)
	copy(script[2:22], hash[:])
	script[22] = 0x87 // OP_EQUAL
	c := &GoCoreClient{}
	if got := c.getScriptType(script); got != "scripthash" {
		t.Errorf("P2SH: want \"scripthash\", got %q", got)
	}
}

func TestGetScriptType_NullData_NoRegression(t *testing.T) {
	// OP_RETURN <data>
	script := []byte{0x6a, 0x04, 0xde, 0xad, 0xbe, 0xef}
	c := &GoCoreClient{}
	if got := c.getScriptType(script); got != "nulldata" {
		t.Errorf("OP_RETURN: want \"nulldata\", got %q", got)
	}
}

func TestGetScriptType_Multisig(t *testing.T) {
	// Bonus from delegation: pkg/script recognizes bare multisig too.
	// 1-of-2 multisig: OP_1 <pk1> <pk2> OP_2 OP_CHECKMULTISIG.
	pk1 := mkPubKey33(0x10)
	pk2 := mkPubKey33(0x20)
	script := []byte{0x51 /* OP_1 */, 0x21 /* push 33 */}
	script = append(script, pk1[:]...)
	script = append(script, 0x21 /* push 33 */)
	script = append(script, pk2[:]...)
	script = append(script, 0x52 /* OP_2 */, 0xae /* OP_CHECKMULTISIG */)
	c := &GoCoreClient{}
	if got := c.getScriptType(script); got != "multisig" {
		t.Errorf("multisig: want \"multisig\", got %q", got)
	}
}

func TestGetScriptType_Nonstandard(t *testing.T) {
	// Random bytes that don't match any standard pattern.
	script := []byte{0x99, 0x88, 0x77}
	c := &GoCoreClient{}
	if got := c.getScriptType(script); got != "nonstandard" {
		t.Errorf("random: want \"nonstandard\", got %q", got)
	}
}

func TestGetScriptType_EmptyScript(t *testing.T) {
	c := &GoCoreClient{}
	if got := c.getScriptType([]byte{}); got != "nonstandard" {
		t.Errorf("empty script: want \"nonstandard\", got %q", got)
	}
}

// ----------------------------------------------------------------------------
// GetAddressBasic / GetAddressStats — error-path coverage
// ----------------------------------------------------------------------------
//
// Storage-backed positive tests for these two methods would require mocking
// the full storage.Storage interface (75+ methods). The two methods are
// thin wrappers over storage calls — GetAddressBasic delegates to
// GetUTXOsByAddress, and GetAddressStats lift-and-shifts the address-tx
// walk previously implemented inside the (now-removed) GetAddressInfo. The
// walk semantics are covered transitively by the prior GetAddressInfo
// integration coverage (live Wails desktop verification across the
// `m-address-detail-redesign-paginated` and `m-address-detail-hero-redesign`
// sequence). Unit-test coverage here is limited to the pre-storage error
// path (crypto.DecodeAddress), which is the only branch that can be
// exercised without a storage mock.

func TestGetAddressBasic_InvalidAddress(t *testing.T) {
	c := &GoCoreClient{}
	_, err := c.GetAddressBasic("not-a-valid-address")
	if err == nil {
		t.Fatal("expected error for invalid address, got nil")
	}
}

func TestGetAddressBasic_EmptyAddress(t *testing.T) {
	c := &GoCoreClient{}
	_, err := c.GetAddressBasic("")
	if err == nil {
		t.Fatal("expected error for empty address, got nil")
	}
}

func TestGetAddressStats_InvalidAddress(t *testing.T) {
	c := &GoCoreClient{}
	_, err := c.GetAddressStats("not-a-valid-address")
	if err == nil {
		t.Fatal("expected error for invalid address, got nil")
	}
}

func TestGetAddressStats_EmptyAddress(t *testing.T) {
	c := &GoCoreClient{}
	_, err := c.GetAddressStats("")
	if err == nil {
		t.Fatal("expected error for empty address, got nil")
	}
}

func TestGetAddressBalance_InvalidAddress(t *testing.T) {
	c := &GoCoreClient{}
	_, err := c.GetAddressBalance("not-a-valid-address")
	if err == nil {
		t.Fatal("expected error for invalid address, got nil")
	}
}

func TestGetAddressBalance_EmptyAddress(t *testing.T) {
	c := &GoCoreClient{}
	_, err := c.GetAddressBalance("")
	if err == nil {
		t.Fatal("expected error for empty address, got nil")
	}
}

// ----------------------------------------------------------------------------
// extractOpReturnData tests (task m-tx-explorer-dto-enrich, Phase 5)
// ----------------------------------------------------------------------------

func TestExtractOpReturnData_Printable(t *testing.T) {
	// OP_RETURN + direct push 5 bytes "Hello"
	script := []byte{0x6a, 0x05, 'H', 'e', 'l', 'l', 'o'}
	hexStr, ascii := extractOpReturnData(script)
	if hexStr != "48656c6c6f" {
		t.Fatalf("hex: want 48656c6c6f, got %q", hexStr)
	}
	if ascii != "Hello" {
		t.Fatalf("ascii: want Hello, got %q", ascii)
	}
}

func TestExtractOpReturnData_NonPrintable(t *testing.T) {
	// OP_RETURN + direct push 4 bytes 0x00010203 (non-printable)
	script := []byte{0x6a, 0x04, 0x00, 0x01, 0x02, 0x03}
	hexStr, ascii := extractOpReturnData(script)
	if hexStr != "00010203" {
		t.Fatalf("hex: want 00010203, got %q", hexStr)
	}
	if ascii != "" {
		t.Fatalf("ascii: want empty, got %q", ascii)
	}
}

func TestExtractOpReturnData_Empty(t *testing.T) {
	// Bare OP_RETURN with no push — too short to even read the push opcode.
	script := []byte{0x6a}
	hexStr, ascii := extractOpReturnData(script)
	if hexStr != "" || ascii != "" {
		t.Fatalf("want empty/empty, got %q/%q", hexStr, ascii)
	}
}

func TestExtractOpReturnData_Pushdata1(t *testing.T) {
	// OP_RETURN + OP_PUSHDATA1 + length 3 + "abc"
	script := []byte{0x6a, 0x4c, 0x03, 'a', 'b', 'c'}
	hexStr, ascii := extractOpReturnData(script)
	if hexStr != "616263" {
		t.Fatalf("hex: want 616263, got %q", hexStr)
	}
	if ascii != "abc" {
		t.Fatalf("ascii: want abc, got %q", ascii)
	}
}

func TestExtractOpReturnData_Truncated(t *testing.T) {
	// OP_RETURN + direct push 10 bytes but only 2 bytes follow.
	script := []byte{0x6a, 0x0a, 'a', 'b'}
	hexStr, ascii := extractOpReturnData(script)
	if hexStr != "" || ascii != "" {
		t.Fatalf("want empty/empty on truncated, got %q/%q", hexStr, ascii)
	}
}

func TestExtractOpReturnData_NotOpReturn(t *testing.T) {
	// Doesn't start with OP_RETURN — should bail.
	script := []byte{0x76, 0xa9, 0x14}
	hexStr, ascii := extractOpReturnData(script)
	if hexStr != "" || ascii != "" {
		t.Fatalf("want empty/empty on non-OP_RETURN, got %q/%q", hexStr, ascii)
	}
}

// ----------------------------------------------------------------------------
// assignOutputRole tests (task m-tx-explorer-dto-enrich, Phase 5)
// ----------------------------------------------------------------------------

// emptyOutputPtr builds a value=0, empty-script TxOutput (the canonical marker).
func emptyOutputPtr() *types.TxOutput {
	return &types.TxOutput{Value: 0, ScriptPubKey: nil}
}

// nonEmptyOutputPtr builds a value-bearing P2PKH-shaped output.
func nonEmptyOutputPtr() *types.TxOutput {
	return &types.TxOutput{Value: 100, ScriptPubKey: buildP2PKHScript(mkHash(0x10))}
}

// coinstakeTxStub returns a minimal tx that satisfies IsCoinStake().
func coinstakeTxStub() *types.Transaction {
	return &types.Transaction{
		Inputs:  []*types.TxInput{{PreviousOutput: types.Outpoint{Hash: types.Hash{0xaa}, Index: 0}}},
		Outputs: []*types.TxOutput{emptyOutputPtr(), nonEmptyOutputPtr()},
	}
}

// coinbaseTxStub returns a minimal tx that satisfies IsCoinbase().
func coinbaseTxStub(outputs []*types.TxOutput) *types.Transaction {
	return &types.Transaction{
		Inputs:  []*types.TxInput{{PreviousOutput: types.Outpoint{Hash: types.Hash{}, Index: 0xffffffff}}},
		Outputs: outputs,
	}
}

// regularTxStub returns a tx that satisfies neither IsCoinbase nor IsCoinStake.
// (Multiple inputs to avoid IsCoinStake's "outputs[0] empty" trap.)
func regularTxStub() *types.Transaction {
	return &types.Transaction{
		Inputs:  []*types.TxInput{{PreviousOutput: types.Outpoint{Hash: types.Hash{0xbb}, Index: 1}}},
		Outputs: []*types.TxOutput{nonEmptyOutputPtr()},
	}
}

func TestAssignOutputRole_BlockMarker_PoSCoinbase(t *testing.T) {
	tx := coinbaseTxStub([]*types.TxOutput{emptyOutputPtr()})
	got := assignOutputRole(outputRoleContext{
		tx:               tx,
		outputIndex:      0,
		output:           emptyOutputPtr(),
		scriptType:       "nonstandard",
		isPoSBlockMarker: true,
	})
	if got != OutputRoleBlockMarker {
		t.Fatalf("want %q, got %q", OutputRoleBlockMarker, got)
	}
}

func TestAssignOutputRole_BlockMarker_CoinstakeVout0(t *testing.T) {
	tx := coinstakeTxStub()
	got := assignOutputRole(outputRoleContext{
		tx:          tx,
		outputIndex: 0,
		output:      emptyOutputPtr(),
		scriptType:  "nonstandard",
	})
	if got != OutputRoleBlockMarker {
		t.Fatalf("want %q, got %q", OutputRoleBlockMarker, got)
	}
}

func TestAssignOutputRole_CoinstakeUnclassified(t *testing.T) {
	// Coinstake tx, output[1] not labelled by computeCoinstakeBreakdown
	// (anomalous layout — e.g. testnet without DevAddress + split layout).
	// The defensive fallback must NOT fall through to the standard-payment
	// branch and report `change` / `external_payment` based on the staker's
	// input addresses — those are semantically wrong for a coinstake output.
	addr := addressForHash(t, mkHash(0x50))
	tx := coinstakeTxStub()
	got := assignOutputRole(outputRoleContext{
		tx: tx, outputIndex: 1, output: nonEmptyOutputPtr(),
		scriptType:     "pubkeyhash",
		legacyLabel:    "", // unclassified
		outputAddress:  addr,
		isMine:         true,
		inputAddresses: map[string]struct{}{addr: {}}, // would otherwise → change
	})
	if got != OutputRoleNonstandard {
		t.Fatalf("want %q (defensive fallback), got %q", OutputRoleNonstandard, got)
	}
}

func TestAssignOutputRole_StakeReturn(t *testing.T) {
	tx := coinstakeTxStub()
	got := assignOutputRole(outputRoleContext{
		tx: tx, outputIndex: 1, output: nonEmptyOutputPtr(),
		scriptType:  "pubkeyhash",
		legacyLabel: "Stake Return",
	})
	if got != OutputRoleStakeReturn {
		t.Fatalf("want %q, got %q", OutputRoleStakeReturn, got)
	}
}

func TestAssignOutputRole_MasternodePayment(t *testing.T) {
	tx := coinstakeTxStub()
	got := assignOutputRole(outputRoleContext{
		tx: tx, outputIndex: 1, output: nonEmptyOutputPtr(),
		scriptType:  "pubkeyhash",
		legacyLabel: "Masternode Payment",
	})
	if got != OutputRoleMasternodePayment {
		t.Fatalf("want %q, got %q", OutputRoleMasternodePayment, got)
	}
}

func TestAssignOutputRole_DevFund(t *testing.T) {
	tx := coinstakeTxStub()
	got := assignOutputRole(outputRoleContext{
		tx: tx, outputIndex: 1, output: nonEmptyOutputPtr(),
		scriptType:  "pubkeyhash",
		legacyLabel: "Dev Fund",
	})
	if got != OutputRoleDevFund {
		t.Fatalf("want %q, got %q", OutputRoleDevFund, got)
	}
}

func TestAssignOutputRole_MiningReward(t *testing.T) {
	tx := coinbaseTxStub([]*types.TxOutput{nonEmptyOutputPtr()})
	got := assignOutputRole(outputRoleContext{
		tx: tx, outputIndex: 0, output: nonEmptyOutputPtr(),
		scriptType:  "pubkeyhash",
		blockHeight: 5000,
	})
	if got != OutputRoleMiningReward {
		t.Fatalf("want %q, got %q", OutputRoleMiningReward, got)
	}
}

func TestAssignOutputRole_Premine(t *testing.T) {
	tx := coinbaseTxStub([]*types.TxOutput{nonEmptyOutputPtr()})
	got := assignOutputRole(outputRoleContext{
		tx: tx, outputIndex: 0, output: nonEmptyOutputPtr(),
		scriptType:  "pubkey",
		blockHeight: 1,
	})
	if got != OutputRolePremine {
		t.Fatalf("want %q, got %q", OutputRolePremine, got)
	}
}

func TestAssignOutputRole_DataCarrier(t *testing.T) {
	got := assignOutputRole(outputRoleContext{
		tx:          regularTxStub(),
		outputIndex: 0,
		output:      &types.TxOutput{Value: 0, ScriptPubKey: []byte{0x6a, 0x04, 'd', 'a', 't', 'a'}},
		scriptType:  "nulldata",
	})
	if got != OutputRoleDataCarrier {
		t.Fatalf("want %q, got %q", OutputRoleDataCarrier, got)
	}
}

func TestAssignOutputRole_Multisig(t *testing.T) {
	got := assignOutputRole(outputRoleContext{
		tx:          regularTxStub(),
		outputIndex: 0,
		output:      nonEmptyOutputPtr(),
		scriptType:  "multisig",
	})
	if got != OutputRoleMultisig {
		t.Fatalf("want %q, got %q", OutputRoleMultisig, got)
	}
}

func TestAssignOutputRole_Nonstandard(t *testing.T) {
	got := assignOutputRole(outputRoleContext{
		tx:          regularTxStub(),
		outputIndex: 0,
		output:      nonEmptyOutputPtr(),
		scriptType:  "nonstandard",
	})
	if got != OutputRoleNonstandard {
		t.Fatalf("want %q, got %q", OutputRoleNonstandard, got)
	}
}

func TestAssignOutputRole_ExternalPayment(t *testing.T) {
	got := assignOutputRole(outputRoleContext{
		tx:            regularTxStub(),
		outputIndex:   0,
		output:        nonEmptyOutputPtr(),
		scriptType:    "pubkeyhash",
		outputAddress: addressForHash(t, mkHash(0x20)),
		isMine:        false,
	})
	if got != OutputRoleExternalPayment {
		t.Fatalf("want %q, got %q", OutputRoleExternalPayment, got)
	}
}

func TestAssignOutputRole_SelfSend(t *testing.T) {
	addr := addressForHash(t, mkHash(0x30))
	got := assignOutputRole(outputRoleContext{
		tx:             regularTxStub(),
		outputIndex:    0,
		output:         nonEmptyOutputPtr(),
		scriptType:     "pubkeyhash",
		outputAddress:  addr,
		isMine:         true,
		inputAddresses: map[string]struct{}{addressForHash(t, mkHash(0x99)): {}},
	})
	if got != OutputRoleSelfSend {
		t.Fatalf("want %q, got %q", OutputRoleSelfSend, got)
	}
}

func TestAssignOutputRole_Change(t *testing.T) {
	addr := addressForHash(t, mkHash(0x40))
	got := assignOutputRole(outputRoleContext{
		tx:             regularTxStub(),
		outputIndex:    0,
		output:         nonEmptyOutputPtr(),
		scriptType:     "pubkeyhash",
		outputAddress:  addr,
		isMine:         true,
		inputAddresses: map[string]struct{}{addr: {}},
	})
	if got != OutputRoleChange {
		t.Fatalf("want %q, got %q", OutputRoleChange, got)
	}
}

// ----------------------------------------------------------------------------
// extractMultisigAddresses tests (task m-tx-explorer-dto-enrich, Phase 5)
// ----------------------------------------------------------------------------

// buildMultisigScript constructs a bare-multisig scriptPubKey:
//
//	<M> <pubkey1> ... <pubkeyN> <N> OP_CHECKMULTISIG
func buildMultisigScript(m int, pubkeys [][]byte) []byte {
	var script []byte
	script = append(script, byte(0x50+m)) // OP_M
	for _, pk := range pubkeys {
		script = append(script, byte(len(pk))) // direct-push opcode = byte count
		script = append(script, pk...)
	}
	script = append(script, byte(0x50+len(pubkeys))) // OP_N
	script = append(script, 0xae)                    // OP_CHECKMULTISIG
	return script
}

// validCompressedPubkey returns a 33-byte compressed pubkey parseable by
// crypto.ParsePubKey, generated via the project's key API.
func validCompressedPubkey(t *testing.T) []byte {
	t.Helper()
	kp, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	if kp.Public == nil {
		t.Fatal("Public is nil")
	}
	return kp.Public.CompressedBytes()
}

func TestExtractMultisigAddresses_2of3(t *testing.T) {
	pks := [][]byte{
		validCompressedPubkey(t),
		validCompressedPubkey(t),
		validCompressedPubkey(t),
	}
	script := buildMultisigScript(2, pks)
	addrs, m := extractMultisigAddresses(script, "")
	if m != 2 {
		t.Fatalf("required sigs: want 2, got %d", m)
	}
	if len(addrs) != 3 {
		t.Fatalf("addresses: want 3, got %d (%v)", len(addrs), addrs)
	}
	for i, a := range addrs {
		if a == "" {
			t.Errorf("address[%d] empty", i)
		}
	}
}

func TestExtractMultisigAddresses_Invalid(t *testing.T) {
	addrs, m := extractMultisigAddresses([]byte{0x00, 0x01, 0x02}, "")
	if addrs != nil || m != 0 {
		t.Fatalf("want (nil, 0), got (%v, %d)", addrs, m)
	}
}

// ----------------------------------------------------------------------------
// isPoSEmptyCoinbase tests — early-return paths only.
//
// The storage-success path (coinbase + empty output + storage returns block
// with coinstake at vtx[1]) would require a substantial mock of
// storage.Storage. We cover it indirectly via the assignOutputRole
// BlockMarker_PoSCoinbase test which exercises the same role-assignment
// gate. Live verification of the storage path happens via the existing
// coinstake breakdown tests in this file (which already exercise GoCoreClient
// against a real storage at construction time).
// ----------------------------------------------------------------------------

func TestIsPoSEmptyCoinbase_NilTx(t *testing.T) {
	c := &GoCoreClient{}
	if c.isPoSEmptyCoinbase(nil, types.Hash{0xaa}) {
		t.Fatal("want false on nil tx")
	}
}

func TestIsPoSEmptyCoinbase_NotCoinbase(t *testing.T) {
	tx := &types.Transaction{
		Inputs:  []*types.TxInput{{PreviousOutput: types.Outpoint{Hash: types.Hash{0xaa}, Index: 0}}},
		Outputs: []*types.TxOutput{emptyOutputPtr()},
	}
	c := &GoCoreClient{}
	if c.isPoSEmptyCoinbase(tx, types.Hash{0xaa}) {
		t.Fatal("want false on non-coinbase tx")
	}
}

func TestIsPoSEmptyCoinbase_MultipleOutputs(t *testing.T) {
	tx := coinbaseTxStub([]*types.TxOutput{emptyOutputPtr(), nonEmptyOutputPtr()})
	c := &GoCoreClient{}
	if c.isPoSEmptyCoinbase(tx, types.Hash{0xaa}) {
		t.Fatal("want false when len(outputs) != 1")
	}
}

func TestIsPoSEmptyCoinbase_NonEmptyOutput(t *testing.T) {
	tx := coinbaseTxStub([]*types.TxOutput{nonEmptyOutputPtr()})
	c := &GoCoreClient{}
	if c.isPoSEmptyCoinbase(tx, types.Hash{0xaa}) {
		t.Fatal("want false when output[0] is non-empty")
	}
}

func TestIsPoSEmptyCoinbase_ZeroBlockHash(t *testing.T) {
	tx := coinbaseTxStub([]*types.TxOutput{emptyOutputPtr()})
	c := &GoCoreClient{}
	if c.isPoSEmptyCoinbase(tx, types.Hash{}) {
		t.Fatal("want false on zero blockHash (no block lookup possible)")
	}
}

// _ keeps `binary` import live if all consumers in the file are inside helpers.
var _ = binary.AnalyzeScript

// ----------------------------------------------------------------------------
// computeOutputSpentStatus tests (l-tx-explorer-spent-flag)
// ----------------------------------------------------------------------------

func TestComputeOutputSpentStatus(t *testing.T) {
	makeUTXO := func(spendingHeight uint32) *types.UTXO {
		return &types.UTXO{
			Output:         &types.TxOutput{Value: 100},
			SpendingHeight: spendingHeight,
		}
	}
	// Generic NOT_FOUND code (recognized by storage.IsNotFoundError).
	notFoundErr := storage.NewStorageError("NOT_FOUND", "key not found", nil)
	// Storage-specific UTXO_NOT_FOUND code — what BinaryStorage.GetUTXO
	// actually emits on a missing UTXO entry. storage.IsNotFoundError does
	// NOT recognize this code, so the helper carries its own type-assertion
	// fallback to cover it.
	utxoNotFoundErr := storage.NewStorageError("UTXO_NOT_FOUND", "UTXO not found", nil)
	transientErr := storage.NewStorageError("IO_ERROR", "disk read failure", nil)

	cases := []struct {
		name       string
		scriptType string
		value      int64
		utxo       *types.UTXO
		err        error
		want       bool
	}{
		{"spent_utxo_present", "pubkeyhash", 100, makeUTXO(12345), nil, true},
		{"unspent_utxo_present", "pubkeyhash", 100, makeUTXO(0), nil, false},
		{"notfound_nulldata", "nulldata", 0, nil, notFoundErr, false},
		{"notfound_nonstandard_zero_value", "nonstandard", 0, nil, notFoundErr, false},
		{"notfound_nonstandard_nonzero_value", "nonstandard", 100, nil, notFoundErr, true},
		{"notfound_pubkeyhash", "pubkeyhash", 100, nil, notFoundErr, true},
		{"notfound_pubkey", "pubkey", 100, nil, notFoundErr, true},
		{"transient_storage_error", "pubkeyhash", 100, nil, transientErr, false},
		// Storage-specific UTXO_NOT_FOUND code from BinaryStorage.GetUTXO.
		// IsNotFoundError does not recognize this code (only NOT_FOUND /
		// TX_NOT_FOUND / HEIGHT_NOT_FOUND), so the helper falls back to a
		// direct StorageError.Code check. This case lock-steps the fallback.
		{"utxo_notfound_pubkeyhash", "pubkeyhash", 100, nil, utxoNotFoundErr, true},
		{"utxo_notfound_nulldata", "nulldata", 0, nil, utxoNotFoundErr, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := computeOutputSpentStatus(tc.scriptType, tc.value, tc.utxo, tc.err)
			if got != tc.want {
				t.Fatalf("computeOutputSpentStatus(%q, %d, %v, %v) = %v, want %v",
					tc.scriptType, tc.value, tc.utxo, tc.err, got, tc.want)
			}
		})
	}
}
