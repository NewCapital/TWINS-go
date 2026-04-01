package crypto

import (
	"bytes"
	"crypto/ed25519"
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	keyPair, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	if keyPair == nil {
		t.Fatal("KeyPair is nil")
	}

	if keyPair.Private == nil {
		t.Fatal("Private key is nil")
	}

	if keyPair.Public == nil {
		t.Fatal("Public key is nil")
	}

	// Test that public key matches private key
	derivedPublic := keyPair.Private.PublicKey()
	if !keyPair.Public.IsEqual(derivedPublic) {
		t.Error("Public key doesn't match private key")
	}
}

func TestGenerateKeyPairFromSeed(t *testing.T) {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}

	keyPair1, err := GenerateKeyPairFromSeed(seed)
	if err != nil {
		t.Fatalf("GenerateKeyPairFromSeed failed: %v", err)
	}

	keyPair2, err := GenerateKeyPairFromSeed(seed)
	if err != nil {
		t.Fatalf("GenerateKeyPairFromSeed failed: %v", err)
	}

	// Same seed should produce same key pair
	if !bytes.Equal(keyPair1.Private.Bytes(), keyPair2.Private.Bytes()) {
		t.Error("Same seed produced different private keys")
	}

	if !keyPair1.Public.IsEqual(keyPair2.Public) {
		t.Error("Same seed produced different public keys")
	}

	// Different seed should produce different key pair
	seed[0] = 255
	keyPair3, err := GenerateKeyPairFromSeed(seed)
	if err != nil {
		t.Fatalf("GenerateKeyPairFromSeed failed: %v", err)
	}

	if bytes.Equal(keyPair1.Private.Bytes(), keyPair3.Private.Bytes()) {
		t.Error("Different seeds produced same private key")
	}
}

func TestGenerateKeyPairFromSeedInvalidSeed(t *testing.T) {
	// Test with seed too short
	shortSeed := make([]byte, 16)
	_, err := GenerateKeyPairFromSeed(shortSeed)
	if err == nil {
		t.Error("Expected error for short seed")
	}
}

func TestGenerateEd25519KeyPair(t *testing.T) {
	keyPair, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateEd25519KeyPair failed: %v", err)
	}

	if keyPair == nil {
		t.Fatal("Ed25519KeyPair is nil")
	}

	if len(keyPair.Private) != ed25519.PrivateKeySize {
		t.Errorf("Private key size incorrect: expected %d, got %d", ed25519.PrivateKeySize, len(keyPair.Private))
	}

	if len(keyPair.Public) != ed25519.PublicKeySize {
		t.Errorf("Public key size incorrect: expected %d, got %d", ed25519.PublicKeySize, len(keyPair.Public))
	}

	// Test signing and verification
	message := []byte("test message")
	signature := keyPair.Sign(message)

	if !keyPair.Verify(message, signature) {
		t.Error("Ed25519 signature verification failed")
	}

	// Test with wrong message
	wrongMessage := []byte("wrong message")
	if keyPair.Verify(wrongMessage, signature) {
		t.Error("Ed25519 verification should fail with wrong message")
	}
}

func TestPrivateKeyOperations(t *testing.T) {
	keyPair, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	privateKey := keyPair.Private

	// Test Bytes()
	privateBytes := privateKey.Bytes()
	if len(privateBytes) == 0 {
		t.Error("Private key bytes should not be empty")
	}

	// Test Hex()
	privateHex := privateKey.Hex()
	if len(privateHex) == 0 {
		t.Error("Private key hex should not be empty")
	}

	// Test signing
	message := []byte("test message for signing")
	hash := Hash256(message)

	signature, err := privateKey.Sign(hash)
	if err != nil {
		t.Fatalf("Private key signing failed: %v", err)
	}

	if signature == nil {
		t.Fatal("Signature is nil")
	}

	// Test verification with public key
	publicKey := privateKey.PublicKey()
	if !publicKey.Verify(hash, signature) {
		t.Error("Signature verification failed")
	}
}

func TestPublicKeyOperations(t *testing.T) {
	keyPair, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	publicKey := keyPair.Public

	// Test Bytes() - uncompressed format
	publicBytes := publicKey.Bytes()
	if len(publicBytes) != 65 {
		t.Errorf("Uncompressed public key should be 65 bytes, got %d", len(publicBytes))
	}

	if publicBytes[0] != 0x04 {
		t.Errorf("Uncompressed public key should start with 0x04, got 0x%02x", publicBytes[0])
	}

	// Test CompressedBytes()
	compressedBytes := publicKey.CompressedBytes()
	if len(compressedBytes) != 33 {
		t.Errorf("Compressed public key should be 33 bytes, got %d", len(compressedBytes))
	}

	if compressedBytes[0] != 0x02 && compressedBytes[0] != 0x03 {
		t.Errorf("Compressed public key should start with 0x02 or 0x03, got 0x%02x", compressedBytes[0])
	}

	// Test Hex()
	publicHex := publicKey.Hex()
	if len(publicHex) != 130 { // 65 bytes * 2 chars per byte
		t.Errorf("Public key hex should be 130 characters, got %d", len(publicHex))
	}

	// Test CompressedHex()
	compressedHex := publicKey.CompressedHex()
	if len(compressedHex) != 66 { // 33 bytes * 2 chars per byte
		t.Errorf("Compressed public key hex should be 66 characters, got %d", len(compressedHex))
	}

	// Test IsEqual()
	if !publicKey.IsEqual(publicKey) {
		t.Error("Public key should be equal to itself")
	}

	otherKeyPair, _ := GenerateKeyPair()
	if publicKey.IsEqual(otherKeyPair.Public) {
		t.Error("Different public keys should not be equal")
	}
}

func TestKeyParsing(t *testing.T) {
	// Generate a key pair for testing
	keyPair, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	// Test private key parsing from bytes
	privateBytes := keyPair.Private.Bytes()
	parsedPrivate, err := ParsePrivateKeyFromBytes(privateBytes)
	if err != nil {
		t.Fatalf("ParsePrivateKeyFromBytes failed: %v", err)
	}

	if !bytes.Equal(keyPair.Private.Bytes(), parsedPrivate.Bytes()) {
		t.Error("Parsed private key doesn't match original")
	}

	// Test private key parsing from hex
	privateHex := keyPair.Private.Hex()
	parsedPrivateHex, err := ParsePrivateKeyFromHex(privateHex)
	if err != nil {
		t.Fatalf("ParsePrivateKeyFromHex failed: %v", err)
	}

	if !bytes.Equal(keyPair.Private.Bytes(), parsedPrivateHex.Bytes()) {
		t.Error("Parsed private key from hex doesn't match original")
	}

	// Test public key parsing from bytes (uncompressed)
	publicBytes := keyPair.Public.Bytes()
	parsedPublic, err := ParsePublicKeyFromBytes(publicBytes)
	if err != nil {
		t.Fatalf("ParsePublicKeyFromBytes failed: %v", err)
	}

	if !keyPair.Public.IsEqual(parsedPublic) {
		t.Error("Parsed public key doesn't match original")
	}

	// Test public key parsing from compressed bytes
	compressedBytes := keyPair.Public.CompressedBytes()
	parsedCompressed, err := ParsePublicKeyFromBytes(compressedBytes)
	if err != nil {
		t.Fatalf("ParsePublicKeyFromBytes (compressed) failed: %v", err)
	}

	if !keyPair.Public.IsEqual(parsedCompressed) {
		t.Error("Parsed compressed public key doesn't match original")
	}

	// Test public key parsing from hex
	publicHex := keyPair.Public.Hex()
	parsedPublicHex, err := ParsePublicKeyFromHex(publicHex)
	if err != nil {
		t.Fatalf("ParsePublicKeyFromHex failed: %v", err)
	}

	if !keyPair.Public.IsEqual(parsedPublicHex) {
		t.Error("Parsed public key from hex doesn't match original")
	}
}

func TestSignature(t *testing.T) {
	keyPair, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	message := []byte("test message for signature")
	hash := Hash256(message)

	// Create signature
	signature, err := keyPair.Private.Sign(hash)
	if err != nil {
		t.Fatalf("Signing failed: %v", err)
	}

	// Test signature bytes - DER format (typically 70-72 bytes)
	sigBytes := signature.Bytes()
	if len(sigBytes) < 68 || len(sigBytes) > 72 {
		t.Errorf("DER signature should be 68-72 bytes, got %d", len(sigBytes))
	}
	// Verify DER structure: starts with 0x30 (SEQUENCE)
	if sigBytes[0] != 0x30 {
		t.Errorf("DER signature should start with 0x30, got 0x%02x", sigBytes[0])
	}

	// Test RawBytes returns 64-byte R||S format
	rawBytes := signature.RawBytes()
	if len(rawBytes) != 64 {
		t.Errorf("RawBytes should be 64 bytes, got %d", len(rawBytes))
	}

	// Test signature hex (DER format)
	sigHex := signature.Hex()
	if len(sigHex) < 136 || len(sigHex) > 144 { // 68-72 bytes * 2 chars per byte
		t.Errorf("Signature hex should be 136-144 characters, got %d", len(sigHex))
	}

	// Test signature parsing from bytes
	parsedSig, err := ParseSignatureFromBytes(sigBytes)
	if err != nil {
		t.Fatalf("ParseSignatureFromBytes failed: %v", err)
	}

	if !bytes.Equal(signature.Bytes(), parsedSig.Bytes()) {
		t.Error("Parsed signature doesn't match original")
	}

	// Test signature parsing from hex
	parsedSigHex, err := ParseSignatureFromHex(sigHex)
	if err != nil {
		t.Fatalf("ParseSignatureFromHex failed: %v", err)
	}

	if !bytes.Equal(signature.Bytes(), parsedSigHex.Bytes()) {
		t.Error("Parsed signature from hex doesn't match original")
	}

	// Test verification
	if !keyPair.Public.Verify(hash, signature) {
		t.Error("Signature verification failed")
	}

	// Test verification with wrong hash
	wrongHash := Hash256([]byte("wrong message"))
	if keyPair.Public.Verify(wrongHash, signature) {
		t.Error("Signature verification should fail with wrong hash")
	}
}

func TestEd25519Operations(t *testing.T) {
	keyPair, err := GenerateEd25519KeyPair()
	if err != nil {
		t.Fatalf("GenerateEd25519KeyPair failed: %v", err)
	}

	// Test key bytes
	privateBytes := keyPair.PrivateBytes()
	if len(privateBytes) != ed25519.PrivateKeySize {
		t.Errorf("Ed25519 private key should be %d bytes, got %d", ed25519.PrivateKeySize, len(privateBytes))
	}

	publicBytes := keyPair.PublicBytes()
	if len(publicBytes) != ed25519.PublicKeySize {
		t.Errorf("Ed25519 public key should be %d bytes, got %d", ed25519.PublicKeySize, len(publicBytes))
	}

	// Test signing and verification
	message := []byte("Ed25519 test message")
	signature := keyPair.Sign(message)

	if len(signature) != ed25519.SignatureSize {
		t.Errorf("Ed25519 signature should be %d bytes, got %d", ed25519.SignatureSize, len(signature))
	}

	if !keyPair.Verify(message, signature) {
		t.Error("Ed25519 signature verification failed")
	}

	// Test with wrong message
	wrongMessage := []byte("wrong message")
	if keyPair.Verify(wrongMessage, signature) {
		t.Error("Ed25519 verification should fail with wrong message")
	}

	// Test standalone verification
	if !VerifyEd25519(publicBytes, message, signature) {
		t.Error("Standalone Ed25519 verification failed")
	}

	if VerifyEd25519(publicBytes, wrongMessage, signature) {
		t.Error("Standalone Ed25519 verification should fail with wrong message")
	}
}

func TestInvalidInputs(t *testing.T) {
	// Test invalid private key bytes (wrong length)
	_, err := ParsePrivateKeyFromBytes([]byte{1, 2, 3})
	if err == nil {
		t.Error("Expected error for invalid private key bytes")
	}

	// Test invalid private key hex
	_, err = ParsePrivateKeyFromHex("invalid_hex")
	if err == nil {
		t.Error("Expected error for invalid private key hex")
	}

	// Test invalid public key bytes
	_, err = ParsePublicKeyFromBytes([]byte{1, 2, 3})
	if err == nil {
		t.Error("Expected error for invalid public key bytes")
	}

	// Test invalid public key hex
	_, err = ParsePublicKeyFromHex("invalid_hex")
	if err == nil {
		t.Error("Expected error for invalid public key hex")
	}

	// Test invalid signature bytes
	_, err = ParseSignatureFromBytes([]byte{1, 2, 3})
	if err == nil {
		t.Error("Expected error for invalid signature bytes")
	}

	// Test invalid signature hex
	_, err = ParseSignatureFromHex("invalid_hex")
	if err == nil {
		t.Error("Expected error for invalid signature hex")
	}

	// Test VerifyEd25519 with invalid inputs
	if VerifyEd25519([]byte{1, 2, 3}, []byte("message"), make([]byte, ed25519.SignatureSize)) {
		t.Error("VerifyEd25519 should fail with invalid public key")
	}

	if VerifyEd25519(make([]byte, ed25519.PublicKeySize), []byte("message"), []byte{1, 2, 3}) {
		t.Error("VerifyEd25519 should fail with invalid signature")
	}
}

func TestKeyCompatibility(t *testing.T) {
	// Test that keys work consistently across different operations
	keyPair, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	message := []byte("compatibility test message")
	hash := Hash256(message)

	// Sign with private key
	signature, err := keyPair.Private.Sign(hash)
	if err != nil {
		t.Fatalf("Signing failed: %v", err)
	}

	// Verify with public key from key pair
	if !keyPair.Public.Verify(hash, signature) {
		t.Error("Verification with key pair public key failed")
	}

	// Verify with derived public key
	derivedPublic := keyPair.Private.PublicKey()
	if !derivedPublic.Verify(hash, signature) {
		t.Error("Verification with derived public key failed")
	}

	// Round-trip test through serialization
	privateBytes := keyPair.Private.Bytes()
	parsedPrivate, err := ParsePrivateKeyFromBytes(privateBytes)
	if err != nil {
		t.Fatalf("Private key parsing failed: %v", err)
	}

	// Sign with parsed private key
	signature2, err := parsedPrivate.Sign(hash)
	if err != nil {
		t.Fatalf("Signing with parsed private key failed: %v", err)
	}

	// Verify with original public key
	if !keyPair.Public.Verify(hash, signature2) {
		t.Error("Cross-verification failed")
	}
}

func BenchmarkGenerateKeyPair(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GenerateKeyPair()
		if err != nil {
			b.Fatalf("GenerateKeyPair failed: %v", err)
		}
	}
}

func BenchmarkGenerateEd25519KeyPair(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GenerateEd25519KeyPair()
		if err != nil {
			b.Fatalf("GenerateEd25519KeyPair failed: %v", err)
		}
	}
}

func BenchmarkECDSASign(b *testing.B) {
	keyPair, err := GenerateKeyPair()
	if err != nil {
		b.Fatalf("GenerateKeyPair failed: %v", err)
	}

	message := []byte("benchmark message for ECDSA signing")
	hash := Hash256(message)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := keyPair.Private.Sign(hash)
		if err != nil {
			b.Fatalf("ECDSA signing failed: %v", err)
		}
	}
}

func BenchmarkECDSAVerify(b *testing.B) {
	keyPair, err := GenerateKeyPair()
	if err != nil {
		b.Fatalf("GenerateKeyPair failed: %v", err)
	}

	message := []byte("benchmark message for ECDSA verification")
	hash := Hash256(message)

	signature, err := keyPair.Private.Sign(hash)
	if err != nil {
		b.Fatalf("ECDSA signing failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !keyPair.Public.Verify(hash, signature) {
			b.Fatal("ECDSA verification failed")
		}
	}
}

func BenchmarkEd25519Sign(b *testing.B) {
	keyPair, err := GenerateEd25519KeyPair()
	if err != nil {
		b.Fatalf("GenerateEd25519KeyPair failed: %v", err)
	}

	message := []byte("benchmark message for Ed25519 signing")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = keyPair.Sign(message)
	}
}

func BenchmarkEd25519Verify(b *testing.B) {
	keyPair, err := GenerateEd25519KeyPair()
	if err != nil {
		b.Fatalf("GenerateEd25519KeyPair failed: %v", err)
	}

	message := []byte("benchmark message for Ed25519 verification")
	signature := keyPair.Sign(message)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !keyPair.Verify(message, signature) {
			b.Fatal("Ed25519 verification failed")
		}
	}
}

func BenchmarkPublicKeyCompression(b *testing.B) {
	keyPair, err := GenerateKeyPair()
	if err != nil {
		b.Fatalf("GenerateKeyPair failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = keyPair.Public.CompressedBytes()
	}
}

func BenchmarkPrivateKeyParsing(b *testing.B) {
	keyPair, err := GenerateKeyPair()
	if err != nil {
		b.Fatalf("GenerateKeyPair failed: %v", err)
	}

	privateBytes := keyPair.Private.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParsePrivateKeyFromBytes(privateBytes)
		if err != nil {
			b.Fatalf("ParsePrivateKeyFromBytes failed: %v", err)
		}
	}
}

func TestIsValidPublicKey(t *testing.T) {
	// Test valid compressed public key
	keyPair, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	compressedKey := keyPair.Public.CompressedBytes()
	if !IsValidPublicKey(compressedKey) {
		t.Error("Valid compressed public key was rejected")
	}

	// Test valid uncompressed public key
	uncompressedKey := keyPair.Public.Bytes()
	if !IsValidPublicKey(uncompressedKey) {
		t.Error("Valid uncompressed public key was rejected")
	}

	// Test invalid public key - all zeros
	invalidKey := make([]byte, 33)
	invalidKey[0] = 0x02 // Valid prefix but invalid curve point
	if IsValidPublicKey(invalidKey) {
		t.Error("Invalid public key (all zeros) was accepted")
	}

	// Test invalid public key - wrong length
	wrongLength := make([]byte, 32)
	if IsValidPublicKey(wrongLength) {
		t.Error("Invalid public key (wrong length) was accepted")
	}

	// Test empty key
	if IsValidPublicKey([]byte{}) {
		t.Error("Empty public key was accepted")
	}
}