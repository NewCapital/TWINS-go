package crypto

import (
	"crypto"
	"testing"
)

func TestTWINSMessageSigner(t *testing.T) {
	keyPair, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	signer := NewTWINSMessageSigner(keyPair.Private)

	// Test Public() method
	publicKey := signer.Public()
	if publicKey == nil {
		t.Error("Public key is nil")
	}

	// Test SignMessage
	message := []byte("test message for TWINS signer")
	opts := NewSignerOpts(crypto.SHA256)

	signature, err := signer.SignMessage(message, opts)
	if err != nil {
		t.Fatalf("SignMessage failed: %v", err)
	}

	// Signature should be in DER format (variable length, typically 70-72 bytes)
	if len(signature) < 64 || len(signature) > 73 {
		t.Errorf("Expected DER signature length 64-73 bytes, got %d", len(signature))
	}

	// Verify signature
	hash := Hash256(message)
	sig, err := ParseSignatureFromBytes(signature)
	if err != nil {
		t.Fatalf("ParseSignatureFromBytes failed: %v", err)
	}

	pubKey := signer.GetPublicKey()
	if !pubKey.Verify(hash, sig) {
		t.Error("Signature verification failed")
	}
}

func TestMasternodeSigner(t *testing.T) {
	keyPair, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	nodeID := "masternode-test-001"
	masternodeSigner := NewMasternodeSigner(keyPair.Private, nodeID)

	if masternodeSigner.GetNodeID() != nodeID {
		t.Error("NodeID doesn't match")
	}

	// Test masternode message signing
	message := []byte("masternode test message")
	signature, err := masternodeSigner.SignMasternodeMessage(message)
	if err != nil {
		t.Fatalf("SignMasternodeMessage failed: %v", err)
	}

	// Verify masternode signature
	publicKey := masternodeSigner.GetPublicKey()
	if !VerifyMasternodeSignature(publicKey, nodeID, message, signature) {
		t.Error("Masternode signature verification failed")
	}

	// Test with wrong node ID
	if VerifyMasternodeSignature(publicKey, "wrong-node-id", message, signature) {
		t.Error("Masternode signature should fail with wrong node ID")
	}
}

func TestBatchSigner(t *testing.T) {
	keyPair, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	batchSigner := NewBatchSigner(keyPair.Private)

	// Test batch signing
	messages := [][]byte{
		[]byte("message 1"),
		[]byte("message 2"),
		[]byte("message 3"),
	}

	signatures, err := batchSigner.SignBatch(messages)
	if err != nil {
		t.Fatalf("SignBatch failed: %v", err)
	}

	if len(signatures) != len(messages) {
		t.Errorf("Expected %d signatures, got %d", len(messages), len(signatures))
	}

	// Verify each signature
	for i, message := range messages {
		if !VerifyMessageSignature(keyPair.Public, message, signatures[i]) {
			t.Errorf("Signature verification failed for message %d", i)
		}
	}

	// Test concurrent batch signing
	concurrentSignatures, err := batchSigner.SignBatchConcurrent(messages)
	if err != nil {
		t.Fatalf("SignBatchConcurrent failed: %v", err)
	}

	if len(concurrentSignatures) != len(messages) {
		t.Errorf("Expected %d concurrent signatures, got %d", len(messages), len(concurrentSignatures))
	}

	// Verify concurrent signatures
	for i, message := range messages {
		if !VerifyMessageSignature(keyPair.Public, message, concurrentSignatures[i]) {
			t.Errorf("Concurrent signature verification failed for message %d", i)
		}
	}
}

func TestMultiSigValidator(t *testing.T) {
	// Generate multiple key pairs
	var keyPairs []*KeyPair
	var publicKeys []*PublicKey

	for i := 0; i < 5; i++ {
		kp, err := GenerateKeyPair()
		if err != nil {
			t.Fatalf("GenerateKeyPair failed: %v", err)
		}
		keyPairs = append(keyPairs, kp)
		publicKeys = append(publicKeys, kp.Public)
	}

	threshold := 3
	validator := NewMultiSigValidator(threshold, publicKeys)

	if validator.GetThreshold() != threshold {
		t.Error("Threshold doesn't match")
	}

	if len(validator.GetPublicKeys()) != len(publicKeys) {
		t.Error("Public keys count doesn't match")
	}

	// Create signatures from different signers
	message := []byte("multi-signature test message")
	var signatures [][]byte
	var signerIndices []int

	// Sign with first 3 signers (meets threshold)
	for i := 0; i < 3; i++ {
		sig, err := CreateMessageSignature(keyPairs[i].Private, message)
		if err != nil {
			t.Fatalf("CreateMessageSignature failed: %v", err)
		}
		signatures = append(signatures, sig)
		signerIndices = append(signerIndices, i)
	}

	// Test multi-signature validation
	if !validator.ValidateMultiSig(message, signatures) {
		t.Error("Multi-signature validation failed")
	}

	// Test partial multi-signature validation
	if !validator.ValidatePartialMultiSig(message, signatures, signerIndices) {
		t.Error("Partial multi-signature validation failed")
	}

	// Test with insufficient signatures (only 2)
	if validator.ValidateMultiSig(message, signatures[:2]) {
		t.Error("Multi-signature validation should fail with insufficient signatures")
	}

	// Test adding/removing public keys
	newKeyPair, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	validator.AddPublicKey(newKeyPair.Public)
	if len(validator.GetPublicKeys()) != len(publicKeys)+1 {
		t.Error("AddPublicKey failed")
	}

	err = validator.RemovePublicKey(0)
	if err != nil {
		t.Fatalf("RemovePublicKey failed: %v", err)
	}

	if len(validator.GetPublicKeys()) != len(publicKeys) {
		t.Error("RemovePublicKey failed")
	}
}

func TestSignerOpts(t *testing.T) {
	opts := NewSignerOpts(crypto.SHA256)
	if opts.HashFunc() != crypto.SHA256 {
		t.Error("HashFunc doesn't match")
	}

	masternodeOpts := NewMasternodeSignerOpts(crypto.SHA256, "test-node")
	if masternodeOpts.GetMasternodeID() != "test-node" {
		t.Error("MasternodeID doesn't match")
	}
}

func TestSignatureUtilityFunctions(t *testing.T) {
	keyPair, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	message := []byte("utility test message")

	// Test CreateMessageSignature
	signature, err := CreateMessageSignature(keyPair.Private, message)
	if err != nil {
		t.Fatalf("CreateMessageSignature failed: %v", err)
	}

	// Test VerifyMessageSignature
	if !VerifyMessageSignature(keyPair.Public, message, signature) {
		t.Error("VerifyMessageSignature failed")
	}

	// Test ValidateSignatureFormat
	if err := ValidateSignatureFormat(signature); err != nil {
		t.Errorf("ValidateSignatureFormat failed: %v", err)
	}

	// Test IsValidSignature
	if !IsValidSignature(signature) {
		t.Error("IsValidSignature failed")
	}

	// Test with invalid signature format
	invalidSig := []byte{1, 2, 3}
	if err := ValidateSignatureFormat(invalidSig); err == nil {
		t.Error("ValidateSignatureFormat should fail with invalid signature")
	}

	if IsValidSignature(invalidSig) {
		t.Error("IsValidSignature should fail with invalid signature")
	}
}

func BenchmarkTWINSMessageSigner(b *testing.B) {
	keyPair, err := GenerateKeyPair()
	if err != nil {
		b.Fatalf("GenerateKeyPair failed: %v", err)
	}

	signer := NewTWINSMessageSigner(keyPair.Private)
	message := []byte("benchmark message for TWINS signer")
	opts := NewSignerOpts(crypto.SHA256)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := signer.SignMessage(message, opts)
		if err != nil {
			b.Fatalf("SignMessage failed: %v", err)
		}
	}
}

func BenchmarkMasternodeSign(b *testing.B) {
	keyPair, err := GenerateKeyPair()
	if err != nil {
		b.Fatalf("GenerateKeyPair failed: %v", err)
	}

	masternodeSigner := NewMasternodeSigner(keyPair.Private, "benchmark-node")
	message := []byte("benchmark message for masternode signer")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := masternodeSigner.SignMasternodeMessage(message)
		if err != nil {
			b.Fatalf("SignMasternodeMessage failed: %v", err)
		}
	}
}

func BenchmarkBatchSign(b *testing.B) {
	keyPair, err := GenerateKeyPair()
	if err != nil {
		b.Fatalf("GenerateKeyPair failed: %v", err)
	}

	batchSigner := NewBatchSigner(keyPair.Private)
	messages := [][]byte{
		[]byte("batch message 1"),
		[]byte("batch message 2"),
		[]byte("batch message 3"),
		[]byte("batch message 4"),
		[]byte("batch message 5"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := batchSigner.SignBatch(messages)
		if err != nil {
			b.Fatalf("SignBatch failed: %v", err)
		}
	}
}

func BenchmarkMultiSigValidation(b *testing.B) {
	// Generate key pairs
	var keyPairs []*KeyPair
	var publicKeys []*PublicKey

	for i := 0; i < 5; i++ {
		kp, err := GenerateKeyPair()
		if err != nil {
			b.Fatalf("GenerateKeyPair failed: %v", err)
		}
		keyPairs = append(keyPairs, kp)
		publicKeys = append(publicKeys, kp.Public)
	}

	validator := NewMultiSigValidator(3, publicKeys)
	message := []byte("benchmark multi-signature validation")

	// Create signatures
	var signatures [][]byte
	for i := 0; i < 3; i++ {
		sig, err := CreateMessageSignature(keyPairs[i].Private, message)
		if err != nil {
			b.Fatalf("CreateMessageSignature failed: %v", err)
		}
		signatures = append(signatures, sig)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !validator.ValidateMultiSig(message, signatures) {
			b.Fatal("Multi-signature validation failed")
		}
	}
}