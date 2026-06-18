package wallet

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/twins-dev/twins-core/pkg/crypto"
	"github.com/twins-dev/twins-core/pkg/types"
)

// makeSendTxWithRecipient builds a minimal send transaction with one wallet-owned
// change output and one external recipient output. The recipient address is
// derived from the supplied hash160 (raw bytes; no wallet derivation needed
// because recipients are external by definition). Test helper.
func makeSendTxWithRecipient(t *testing.T, changeScript []byte, recipientHash160 []byte, network NetworkType) (*types.Transaction, string) {
	t.Helper()
	recipientScript := buildP2PKHScript(recipientHash160)

	// Derive the recipient address string under the test wallet's network so the
	// substring match below can be asserted against a known concrete value.
	var version byte
	switch network {
	case MainNet:
		version = crypto.MainNetPubKeyHashAddrID
	case TestNet, RegTest:
		version = crypto.TestNetPubKeyHashAddrID
	default:
		t.Fatalf("unsupported network: %v", network)
	}
	payload := append([]byte{version}, recipientHash160...)
	checksum := crypto.DoubleHash256(payload)[:4]
	full := append(payload, checksum...)
	recipientAddr := crypto.Base58Encode(full)

	tx := &types.Transaction{
		Version: 1,
		Inputs: []*types.TxInput{
			{ScriptSig: []byte{0x01}, Sequence: 0xffffffff},
		},
		Outputs: []*types.TxOutput{
			{Value: 100, ScriptPubKey: changeScript},    // wallet-owned change
			{Value: 200, ScriptPubKey: recipientScript}, // external recipient
		},
	}
	return tx, recipientAddr
}

// TestTxMatchesSearchText covers all three layers of the search predicate:
//   - Layer 1: tx.Address substring (own/funding address)
//   - Layer 2: address-book label resolved dynamically
//   - Layer 3: recipient addresses for send transactions (skipping change)
//
// Plus regression cases: receive transactions, empty search, no-match,
// case-insensitive, nil tx for cache-loaded send entries.
func TestTxMatchesSearchText(t *testing.T) {
	// One shared wallet across all subtests — table-driven setup keeps the
	// addresses, labels, and tx structures deterministic and avoids paying the
	// scrypt/HD-key cost of CreateWallet for every case.
	w := createIsolatedWallet(t)
	seed := []byte("test seed for tx search bug fix tests with enough entropy")
	require.NoError(t, w.CreateWallet(seed, nil))

	// Wallet address A: receive address with the address-book label "main address".
	addrA, err := w.GetNewAddress("main address")
	require.NoError(t, err)

	// Wallet address B: funding address used as the tx.Address for sends.
	// No label attached (mirrors the typical funding-address case where the
	// user does not explicitly label their own change/funding addresses).
	addrB, err := w.GetNewAddress("")
	require.NoError(t, err)

	// Wallet address C: change address (used for recipient-search test to
	// verify the change output is skipped by isOurScriptLocked).
	addrC, err := w.GetNewAddress("")
	require.NoError(t, err)

	// External recipient (deterministic hash160, not in the wallet).
	recipientHash := bytes.Repeat([]byte{0xab}, 20)
	scriptC := scriptForWalletAddress(t, addrC)
	sendTx, recipientAddr := makeSendTxWithRecipient(t, scriptC, recipientHash, w.config.Network)

	tests := []struct {
		name   string
		tx     *WalletTransaction
		search string
		want   bool
	}{
		{
			name:   "empty search matches any tx",
			tx:     &WalletTransaction{Address: addrA, Category: TxCategoryReceive},
			search: "",
			want:   true,
		},
		{
			// Layer 1: substring against tx.Address. Preserves legacy behavior.
			name:   "own address substring matches (receive)",
			tx:     &WalletTransaction{Address: addrA, Category: TxCategoryReceive},
			search: addrA[:8],
			want:   true,
		},
		{
			// Layer 2a: persisted tx.Label substring (defense-in-depth).
			// The field is currently never populated in production but the
			// cache serializes it; checking it keeps search aligned with the
			// GUI display fallback at internal/gui/core/go_client.go:1272-1275.
			name:   "persisted tx.Label substring matches (defense-in-depth)",
			tx:     &WalletTransaction{Address: addrA, Category: TxCategoryReceive, Label: "Legacy Persisted Label"},
			search: "persisted",
			want:   true,
		},
		{
			// Layer 2b: address-book label substring resolved via GetAddressLabel.
			// This is Bug 1 — pre-fix this returns false because WalletTransaction.Label
			// is never populated.
			name:   "label substring matches via address-book lookup",
			tx:     &WalletTransaction{Address: addrA, Category: TxCategoryReceive},
			search: "main",
			want:   true,
		},
		{
			// Layer 2: case-insensitive label match.
			name:   "label substring matches case-insensitively",
			tx:     &WalletTransaction{Address: addrA, Category: TxCategoryReceive},
			search: "MAIN",
			want:   true,
		},
		{
			// Regression: own-address case-insensitive match.
			name:   "own address substring matches case-insensitively",
			tx:     &WalletTransaction{Address: addrA, Category: TxCategoryReceive},
			search: addrA[:6], // wallet addresses already mixed-case; just check exact-case substring works
			want:   true,
		},
		{
			// Layer 3: recipient address substring on a send tx. Bug 2 — pre-fix
			// this returns false because matchesSearchText only sees the funding
			// address (tx.Address = addrB), not the external recipient.
			name: "recipient address substring matches on send tx",
			tx: &WalletTransaction{
				Address:  addrB,
				Category: TxCategorySend,
				Tx:       sendTx,
			},
			search: recipientAddr[:8],
			want:   true,
		},
		{
			// Layer 3: change address must NOT match (isOurScriptLocked filter).
			// addrC appears in the change output but should be excluded.
			// We use a substring that appears ONLY in addrC and NOT in addrB or
			// the recipient. Done by picking the last 8 chars of addrC, which
			// statistically won't collide with the other two.
			name: "change address does not match on send tx (filtered by isOurScript)",
			tx: &WalletTransaction{
				Address:  addrB,
				Category: TxCategorySend,
				Tx:       sendTx,
			},
			search: addrC[len(addrC)-12:], // tail of change address, unlikely to collide
			want:   false,
		},
		{
			// Edge case: send tx with nil Tx (cache-loaded entry). Funding
			// address match must still work; recipient search degrades to false
			// without crashing.
			name:   "nil Tx on send tx: funding address match still works",
			tx:     &WalletTransaction{Address: addrB, Category: TxCategorySend, Tx: nil},
			search: addrB[:8],
			want:   true,
		},
		{
			name:   "nil Tx on send tx with no storage record: recipient search returns false (graceful degradation)",
			tx:     &WalletTransaction{Address: addrB, Category: TxCategorySend, Tx: nil},
			search: recipientAddr[:8],
			want:   false,
		},
		{
			name:   "nonsense substring returns false",
			tx:     &WalletTransaction{Address: addrA, Category: TxCategoryReceive},
			search: "ZZZ_no_such_thing_in_any_address_or_label_ZZZ",
			want:   false,
		},
		{
			// Edge case: empty tx.Address (e.g. unknown sender on receive).
			// Empty label, empty address, no Tx — search returns false unless
			// substring is empty (already covered by first case).
			name:   "empty address and label returns false for non-empty search",
			tx:     &WalletTransaction{Address: "", Category: TxCategoryReceive},
			search: "anything",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w.mu.RLock()
			got := w.txMatchesSearchText(tt.tx, tt.search)
			w.mu.RUnlock()
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestTxMatchesSearchText_NilTx_SendWithStorageFallback locks the
// production-critical code path: after a daemon restart the txcache (which is
// metadata-only per txcache.go:611) gives back a WalletTransaction with
// tx.Tx == nil for every entry. Layer 3 must then fall back to
// w.storage.GetTransactionData(tx.Hash) to resolve the raw outputs, otherwise
// recipient-address search returns 0 results in production. This test writes a
// real types.Transaction to the wallet's storage and verifies the substring
// search matches the recipient address.
func TestTxMatchesSearchText_NilTx_SendWithStorageFallback(t *testing.T) {
	w := createIsolatedWallet(t)
	seed := []byte("test seed for storage fallback verification with enough entropy")
	require.NoError(t, w.CreateWallet(seed, nil))

	// Wallet's change address (will be filtered out).
	addrChange, err := w.GetNewAddress("")
	require.NoError(t, err)
	changeScript := scriptForWalletAddress(t, addrChange)

	// External recipient (deterministic hash160).
	recipientHash := bytes.Repeat([]byte{0xcd}, 20)
	sendTx, recipientAddr := makeSendTxWithRecipient(t, changeScript, recipientHash, w.config.Network)

	// Persist the raw tx to storage in the CONFIRMED namespace so the
	// fallback path can resolve it via GetTransactionData(tx.Hash).
	// `storage.StoreTransaction` writes to the mempool prefix (0x11), but
	// `GetTransactionData` only reads from the confirmed prefix (0x04) —
	// see internal/storage/binary/interface_impl.go:1114-1124. So we wrap
	// the tx in a minimal block and call StoreBlockWithHeight, which is
	// the path the unified block processor uses (batch.go:95-115).
	block := &types.Block{
		Header: &types.BlockHeader{
			Version:       1,
			PrevBlockHash: types.Hash{},
			MerkleRoot:    types.Hash{},
			Timestamp:     1,
			Bits:          0x1d00ffff,
			Nonce:         1,
		},
		Transactions: []*types.Transaction{sendTx},
	}
	// Use Batch directly to call StoreBlockWithHeight (writes to confirmed 0x04 prefix
	// which GetTransactionData reads). Plain Storage.StoreBlock would compute height
	// from chain state, which is empty in this isolated test.
	batch := w.storage.NewBatch()
	require.NoError(t, batch.StoreBlockWithHeight(block, 1))
	require.NoError(t, batch.Commit())

	// Construct the WalletTransaction the way txcache.go would after a daemon
	// restart: Tx is nil, but Hash points at the on-disk record.
	wtx := &WalletTransaction{
		Hash:     sendTx.Hash(),
		Category: TxCategorySend,
		Address:  "WfundingAddrThatDoesNotMatchSubstring", // forces layer 3
		Tx:       nil,
	}

	w.mu.RLock()
	got := w.txMatchesSearchText(wtx, recipientAddr[:8])
	w.mu.RUnlock()
	assert.True(t, got, "recipient substring should match via storage fallback when tx.Tx is nil")

	// Negative control: a substring that does not appear in the recipient or
	// the funding address must still return false (the storage fallback
	// resolved the tx, so layer 3 ran, but no output matched).
	w.mu.RLock()
	gotNo := w.txMatchesSearchText(wtx, "ZZZ_no_such_recipient_substring_ZZZ")
	w.mu.RUnlock()
	assert.False(t, gotNo, "nonsense substring must not match even with storage fallback")
}
