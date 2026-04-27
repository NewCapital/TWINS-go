package crypto

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
)

// fixedKey returns a deterministic test private key.
// Using a fixed value lets us compare SignCompact vs Sign byte-for-byte.
func fixedKey(t *testing.T) *btcec.PrivateKey {
	t.Helper()
	raw, _ := hex.DecodeString("1111111111111111111111111111111111111111111111111111111111111111")
	priv, _ := btcec.PrivKeyFromBytes(raw)
	return priv
}

// fixedHash returns a deterministic 32-byte sighash-like digest.
func fixedHash() []byte {
	h := sha256.Sum256([]byte("twins-go tx signing round-trip digest"))
	return h[:]
}

// TestWalletSignPath_ProducesVerifiableSignature runs the exact path used by
// Wallet.SignTransaction (PrivateKey.Sign + Signature.Bytes) and verifies the
// resulting DER bytes against btcec's strict ParseDERSignature and Verify.
// Legacy C++ uses secp256k1_ecdsa_signature_parse_der_lax + normalize + verify,
// which is STRICTLY MORE PERMISSIVE than btcec's strict path — so if this
// passes, legacy accepts the signature.
func TestWalletSignPath_ProducesVerifiableSignature(t *testing.T) {
	priv := fixedKey(t)
	hash := fixedHash()

	pk := &PrivateKey{key: priv}
	sig, err := pk.Sign(hash)
	if err != nil {
		t.Fatalf("Wallet sign path failed: %v", err)
	}

	der := sig.Bytes()
	t.Logf("DER bytes (len=%d): %s", len(der), hex.EncodeToString(der))

	parsed, err := ecdsa.ParseDERSignature(der)
	if err != nil {
		t.Fatalf("btcec strict DER parse REJECTED wallet signature: %v\nDER: %s",
			err, hex.EncodeToString(der))
	}

	if !parsed.Verify(hash, priv.PubKey()) {
		t.Fatalf("btcec VERIFICATION FAILED for wallet signature — legacy would reject with SCRIPT_ERR_EVAL_FALSE")
	}
}

// TestSignCompact_vs_Sign_ProduceSameDER establishes whether the two signing
// paths (ecdsa.SignCompact used by PrivateKey.Sign, vs ecdsa.Sign used by the
// message-signing FIXMessageSigner path) produce byte-identical DER signatures
// for the same key and hash. If they diverge, we have located a class of bug.
func TestSignCompact_vs_Sign_ProduceSameDER(t *testing.T) {
	priv := fixedKey(t)
	hash := fixedHash()

	// Path A: canonical ecdsa.Sign + sig.Serialize
	canonicalSig := ecdsa.Sign(priv, hash)
	canonicalDER := canonicalSig.Serialize()
	t.Logf("canonical DER   (len=%d): %s", len(canonicalDER), hex.EncodeToString(canonicalDER))

	// Path B: wallet's PrivateKey.Sign (SignCompact + manual Bytes)
	pk := &PrivateKey{key: priv}
	walletSig, err := pk.Sign(hash)
	if err != nil {
		t.Fatalf("wallet Sign failed: %v", err)
	}
	walletDER := walletSig.Bytes()
	t.Logf("wallet   DER    (len=%d): %s", len(walletDER), hex.EncodeToString(walletDER))

	if !bytes.Equal(canonicalDER, walletDER) {
		t.Errorf("DIVERGENCE between canonical ecdsa.Sign and wallet PrivateKey.Sign:\n  canonical: %s\n  wallet:    %s",
			hex.EncodeToString(canonicalDER), hex.EncodeToString(walletDER))
	}
}

// TestSignCompact_S_IsLow asserts that SignCompact output has low-S.
// If this fails, the wallet produces high-S signatures — legacy would still
// normalize before verifying but it would point at a different suspect area.
func TestSignCompact_S_IsLow(t *testing.T) {
	priv := fixedKey(t)
	hash := fixedHash()

	compact := ecdsa.SignCompact(priv, hash, true)
	if len(compact) != 65 {
		t.Fatalf("unexpected compact length: %d", len(compact))
	}
	sBytes := compact[33:65]

	// secp256k1 curve order N
	nHex := "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141"
	nRaw, _ := hex.DecodeString(nHex)

	// halfN = N >> 1
	halfN := make([]byte, 32)
	var carry uint16
	for i := 0; i < 32; i++ {
		v := uint16(nRaw[i]) | (carry << 8)
		halfN[i] = byte(v >> 1)
		carry = v & 1
	}

	if bytes.Compare(sBytes, halfN) > 0 {
		t.Errorf("SignCompact produced HIGH-S signature:\n  S:    %s\n  N/2:  %s",
			hex.EncodeToString(sBytes), hex.EncodeToString(halfN))
	}
}

// TestManualDER_Equivalent_To_NativeDER checks whether the manual DER encoder
// in Signature.Bytes() produces identical output to btcec's native
// sig.Serialize() given the same R and S values.
func TestManualDER_Equivalent_To_NativeDER(t *testing.T) {
	priv := fixedKey(t)
	hash := fixedHash()

	native := ecdsa.Sign(priv, hash)
	nativeDER := native.Serialize()

	parsed, err := ParseSignatureFromBytes(nativeDER)
	if err != nil {
		t.Fatalf("ParseSignatureFromBytes failed: %v", err)
	}
	manualDER := parsed.Bytes()

	if !bytes.Equal(nativeDER, manualDER) {
		t.Errorf("manual DER encoder diverges from btcec native:\n  native: %s\n  manual: %s",
			hex.EncodeToString(nativeDER), hex.EncodeToString(manualDER))
	}
}
