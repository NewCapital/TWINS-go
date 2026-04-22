package wallet

import (
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/stretchr/testify/require"

	"github.com/twins-dev/twins-core/pkg/crypto"
	"github.com/twins-dev/twins-core/pkg/types"
)

// extractSigAndPubkey decodes a P2PKH scriptSig:
//   <push_len><sig+hashType><push_len><pubkey>
// Returns the DER signature (without the trailing sighash byte) and the pushed pubkey.
func extractSigAndPubkey(t *testing.T, scriptSig []byte) (sigDER, pubkey []byte) {
	t.Helper()
	require.Greater(t, len(scriptSig), 1, "scriptSig too short")

	sigLen := int(scriptSig[0])
	require.GreaterOrEqual(t, len(scriptSig), 1+sigLen+1, "scriptSig truncated at sig")

	sigWithHashType := scriptSig[1 : 1+sigLen]
	require.Greater(t, len(sigWithHashType), 0, "empty signature")
	sigDER = sigWithHashType[:len(sigWithHashType)-1] // strip sighash byte

	pubLen := int(scriptSig[1+sigLen])
	require.GreaterOrEqual(t, len(scriptSig), 1+sigLen+1+pubLen, "scriptSig truncated at pubkey")
	pubkey = scriptSig[1+sigLen+1 : 1+sigLen+1+pubLen]
	return sigDER, pubkey
}

// extractSigFromP2PKScriptSig decodes a P2PK scriptSig: <push_len><sig+hashType>.
func extractSigFromP2PKScriptSig(t *testing.T, scriptSig []byte) []byte {
	t.Helper()
	require.Greater(t, len(scriptSig), 1, "scriptSig too short")
	sigLen := int(scriptSig[0])
	require.GreaterOrEqual(t, len(scriptSig), 1+sigLen, "scriptSig truncated at sig")
	sigWithHashType := scriptSig[1 : 1+sigLen]
	require.Greater(t, len(sigWithHashType), 0, "empty signature")
	return sigWithHashType[:len(sigWithHashType)-1]
}

func parsePubKey(t *testing.T, raw []byte) *btcec.PublicKey {
	t.Helper()
	pk, err := btcec.ParsePubKey(raw)
	require.NoError(t, err)
	return pk
}

// TestSignCoinstakeTransaction_P2PKH_VerifiesViaStrictDER exercises
// signCoinstakeTransaction with a P2PKH funding UTXO and verifies the resulting
// signature with btcec's STRICT DER parse + verify against
// tx.SignatureHash(0, scriptPubKey, SigHashAll).
//
// Legacy C++ CPubKey::Verify uses lax DER + secp256k1_ecdsa_signature_normalize
// + secp256k1_ecdsa_verify, which is strictly MORE PERMISSIVE than btcec's
// strict path. If this test passes, legacy accepts the signature.
func TestSignCoinstakeTransaction_P2PKH_VerifiesViaStrictDER(t *testing.T) {
	w := createTestWallet(t)

	privRaw, _ := hex.DecodeString("1111111111111111111111111111111111111111111111111111111111111111")
	privKey, err := crypto.ParsePrivateKeyFromBytes(privRaw)
	require.NoError(t, err)
	pubKey := privKey.PublicKey()
	pubKeyBytes := pubKey.SerializeCompressed()
	pubKeyHash := crypto.Hash160(pubKeyBytes)

	// P2PKH scriptPubKey: OP_DUP OP_HASH160 <20> <hash> OP_EQUALVERIFY OP_CHECKSIG
	scriptPubKey := make([]byte, 0, 25)
	scriptPubKey = append(scriptPubKey, 0x76, 0xa9, 0x14)
	scriptPubKey = append(scriptPubKey, pubKeyHash...)
	scriptPubKey = append(scriptPubKey, 0x88, 0xac)
	require.Len(t, scriptPubKey, 25)

	// Coinstake-shaped tx: 1 input + 2 outputs (empty marker + P2PK stake return).
	p2pkOut := make([]byte, 0, 35)
	p2pkOut = append(p2pkOut, byte(len(pubKeyBytes)))
	p2pkOut = append(p2pkOut, pubKeyBytes...)
	p2pkOut = append(p2pkOut, 0xac)

	tx := &types.Transaction{
		Version: 1,
		Inputs: []*types.TxInput{{
			PreviousOutput: types.Outpoint{Hash: types.Hash{0x01, 0x02, 0x03}, Index: 0},
			Sequence:       0xffffffff,
		}},
		Outputs: []*types.TxOutput{
			{Value: 0, ScriptPubKey: []byte{}},
			{Value: 10_000_000_000, ScriptPubKey: p2pkOut},
		},
	}

	stakeUTXO := &StakeableUTXO{
		Outpoint:     tx.Inputs[0].PreviousOutput,
		Amount:       10_000_000_000,
		ScriptPubKey: scriptPubKey,
	}

	_, err = w.signCoinstakeTransaction(tx, stakeUTXO, privKey)
	require.NoError(t, err)

	// Canonical audited sighash path.
	expectedHash := tx.SignatureHash(0, scriptPubKey, types.SigHashAll)

	sigDER, pushedPubkey := extractSigAndPubkey(t, tx.Inputs[0].ScriptSig)
	require.Equal(t, pubKeyBytes, pushedPubkey, "scriptSig must push the signer's compressed pubkey")

	parsed, err := ecdsa.ParseDERSignature(sigDER)
	require.NoError(t, err, "coinstake signature must parse under btcec strict DER")

	require.True(t, parsed.Verify(expectedHash[:], parsePubKey(t, pushedPubkey)),
		"coinstake signature must verify against Transaction.SignatureHash (legacy sighash)")
}

// TestSignCoinstakeTransaction_P2PK_VerifiesViaStrictDER covers the variant
// where the UTXO being spent is already P2PK (scriptSig is just <sig>).
func TestSignCoinstakeTransaction_P2PK_VerifiesViaStrictDER(t *testing.T) {
	w := createTestWallet(t)

	privRaw, _ := hex.DecodeString("2222222222222222222222222222222222222222222222222222222222222222")
	privKey, err := crypto.ParsePrivateKeyFromBytes(privRaw)
	require.NoError(t, err)
	pubKeyBytes := privKey.PublicKey().SerializeCompressed()

	scriptPubKey := make([]byte, 0, 35)
	scriptPubKey = append(scriptPubKey, byte(len(pubKeyBytes)))
	scriptPubKey = append(scriptPubKey, pubKeyBytes...)
	scriptPubKey = append(scriptPubKey, 0xac)
	require.Len(t, scriptPubKey, 35)

	tx := &types.Transaction{
		Version: 1,
		Inputs: []*types.TxInput{{
			PreviousOutput: types.Outpoint{Hash: types.Hash{0x0a, 0x0b}, Index: 1},
			Sequence:       0xffffffff,
		}},
		Outputs: []*types.TxOutput{
			{Value: 0, ScriptPubKey: []byte{}},
			{Value: 5_000_000_000, ScriptPubKey: scriptPubKey},
		},
	}

	stakeUTXO := &StakeableUTXO{
		Outpoint:     tx.Inputs[0].PreviousOutput,
		Amount:       5_000_000_000,
		ScriptPubKey: scriptPubKey,
	}

	_, err = w.signCoinstakeTransaction(tx, stakeUTXO, privKey)
	require.NoError(t, err)

	expectedHash := tx.SignatureHash(0, scriptPubKey, types.SigHashAll)
	sigDER := extractSigFromP2PKScriptSig(t, tx.Inputs[0].ScriptSig)

	parsed, err := ecdsa.ParseDERSignature(sigDER)
	require.NoError(t, err)
	require.True(t, parsed.Verify(expectedHash[:], parsePubKey(t, pubKeyBytes)),
		"P2PK coinstake signature must verify")
}
