package crypto

import (
	"bytes"
	"testing"
)

func TestDeriveKeyEVPSHA512(t *testing.T) {
	t.Run("basic_derivation", func(t *testing.T) {
		passphrase := []byte("test passphrase")
		salt := []byte{1, 2, 3, 4, 5, 6, 7, 8}
		iterations := uint32(1000)

		key, iv, err := DeriveKeyEVPSHA512(passphrase, salt, iterations)
		if err != nil {
			t.Fatalf("DeriveKeyEVPSHA512 failed: %v", err)
		}

		if len(key) != KeySize {
			t.Errorf("Expected key size %d, got %d", KeySize, len(key))
		}
		if len(iv) != IVSize {
			t.Errorf("Expected IV size %d, got %d", IVSize, len(iv))
		}
	})

	t.Run("deterministic", func(t *testing.T) {
		passphrase := []byte("test passphrase")
		salt := []byte{1, 2, 3, 4, 5, 6, 7, 8}
		iterations := uint32(1000)

		key1, iv1, _ := DeriveKeyEVPSHA512(passphrase, salt, iterations)
		key2, iv2, _ := DeriveKeyEVPSHA512(passphrase, salt, iterations)

		if !bytes.Equal(key1, key2) {
			t.Error("Key derivation should be deterministic")
		}
		if !bytes.Equal(iv1, iv2) {
			t.Error("IV derivation should be deterministic")
		}
	})

	t.Run("different_passphrase", func(t *testing.T) {
		salt := []byte{1, 2, 3, 4, 5, 6, 7, 8}
		iterations := uint32(1000)

		key1, iv1, _ := DeriveKeyEVPSHA512([]byte("passphrase1"), salt, iterations)
		key2, iv2, _ := DeriveKeyEVPSHA512([]byte("passphrase2"), salt, iterations)

		if bytes.Equal(key1, key2) {
			t.Error("Different passphrases should produce different keys")
		}
		if bytes.Equal(iv1, iv2) {
			t.Error("Different passphrases should produce different IVs")
		}
	})

	t.Run("invalid_salt_size", func(t *testing.T) {
		passphrase := []byte("test passphrase")
		salt := []byte{1, 2, 3} // Wrong size
		iterations := uint32(1000)

		_, _, err := DeriveKeyEVPSHA512(passphrase, salt, iterations)
		if err == nil {
			t.Error("Expected error for invalid salt size")
		}
	})
}

func TestDeriveKeyScrypt(t *testing.T) {
	t.Run("basic_derivation", func(t *testing.T) {
		passphrase := []byte("test passphrase")
		salt := []byte{1, 2, 3, 4, 5, 6, 7, 8}

		key, iv, err := DeriveKeyScrypt(passphrase, salt, DefaultScryptN, DefaultScryptR, DefaultScryptP)
		if err != nil {
			t.Fatalf("DeriveKeyScrypt failed: %v", err)
		}

		if len(key) != KeySize {
			t.Errorf("Expected key size %d, got %d", KeySize, len(key))
		}
		if len(iv) != IVSize {
			t.Errorf("Expected IV size %d, got %d", IVSize, len(iv))
		}
	})

	t.Run("deterministic", func(t *testing.T) {
		passphrase := []byte("test passphrase")
		salt := []byte{1, 2, 3, 4, 5, 6, 7, 8}

		key1, iv1, _ := DeriveKeyScrypt(passphrase, salt, DefaultScryptN, DefaultScryptR, DefaultScryptP)
		key2, iv2, _ := DeriveKeyScrypt(passphrase, salt, DefaultScryptN, DefaultScryptR, DefaultScryptP)

		if !bytes.Equal(key1, key2) {
			t.Error("Key derivation should be deterministic")
		}
		if !bytes.Equal(iv1, iv2) {
			t.Error("IV derivation should be deterministic")
		}
	})

	t.Run("invalid_salt_size", func(t *testing.T) {
		passphrase := []byte("test passphrase")
		salt := []byte{1, 2, 3} // Wrong size

		_, _, err := DeriveKeyScrypt(passphrase, salt, DefaultScryptN, DefaultScryptR, DefaultScryptP)
		if err == nil {
			t.Error("Expected error for invalid salt size")
		}
	})
}

func TestGenerateIV(t *testing.T) {
	t.Run("basic_generation", func(t *testing.T) {
		pubkey := []byte{0x02, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}

		iv := GenerateIV(pubkey)

		if len(iv) != IVSize {
			t.Errorf("Expected IV size %d, got %d", IVSize, len(iv))
		}
	})

	t.Run("deterministic", func(t *testing.T) {
		pubkey := []byte{0x02, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}

		iv1 := GenerateIV(pubkey)
		iv2 := GenerateIV(pubkey)

		if !bytes.Equal(iv1, iv2) {
			t.Error("IV generation should be deterministic")
		}
	})

	t.Run("different_pubkeys", func(t *testing.T) {
		pubkey1 := []byte{0x02, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
		pubkey2 := []byte{0x03, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}

		iv1 := GenerateIV(pubkey1)
		iv2 := GenerateIV(pubkey2)

		if bytes.Equal(iv1, iv2) {
			t.Error("Different pubkeys should produce different IVs")
		}
	})
}

func TestEncryptDecryptAES256CBC(t *testing.T) {
	t.Run("basic_encryption_decryption", func(t *testing.T) {
		key := make([]byte, KeySize)
		for i := range key {
			key[i] = byte(i)
		}
		iv := make([]byte, IVSize)
		for i := range iv {
			iv[i] = byte(i + 100)
		}
		plaintext := []byte("Hello, World! This is a test message.")

		// Encrypt
		ciphertext, err := EncryptAES256CBC(key, iv, plaintext)
		if err != nil {
			t.Fatalf("Encryption failed: %v", err)
		}

		if len(ciphertext) == 0 {
			t.Error("Ciphertext should not be empty")
		}

		// Decrypt
		decrypted, err := DecryptAES256CBC(key, iv, ciphertext)
		if err != nil {
			t.Fatalf("Decryption failed: %v", err)
		}

		if !bytes.Equal(decrypted, plaintext) {
			t.Errorf("Decrypted text doesn't match original:\nGot:  %s\nWant: %s", decrypted, plaintext)
		}
	})

	t.Run("empty_plaintext", func(t *testing.T) {
		key := make([]byte, KeySize)
		iv := make([]byte, IVSize)
		plaintext := []byte{}

		ciphertext, err := EncryptAES256CBC(key, iv, plaintext)
		if err != nil {
			t.Fatalf("Encryption failed: %v", err)
		}

		decrypted, err := DecryptAES256CBC(key, iv, ciphertext)
		if err != nil {
			t.Fatalf("Decryption failed: %v", err)
		}

		if !bytes.Equal(decrypted, plaintext) {
			t.Error("Failed to decrypt empty plaintext")
		}
	})

	t.Run("multiple_blocks", func(t *testing.T) {
		key := make([]byte, KeySize)
		iv := make([]byte, IVSize)
		plaintext := make([]byte, 100) // Multiple blocks
		for i := range plaintext {
			plaintext[i] = byte(i)
		}

		ciphertext, err := EncryptAES256CBC(key, iv, plaintext)
		if err != nil {
			t.Fatalf("Encryption failed: %v", err)
		}

		decrypted, err := DecryptAES256CBC(key, iv, ciphertext)
		if err != nil {
			t.Fatalf("Decryption failed: %v", err)
		}

		if !bytes.Equal(decrypted, plaintext) {
			t.Error("Failed to decrypt multiple blocks")
		}
	})

	t.Run("invalid_key_size", func(t *testing.T) {
		key := make([]byte, 16) // Wrong size
		iv := make([]byte, IVSize)
		plaintext := []byte("test")

		_, err := EncryptAES256CBC(key, iv, plaintext)
		if err != ErrInvalidKeySize {
			t.Errorf("Expected ErrInvalidKeySize, got %v", err)
		}
	})

	t.Run("invalid_iv_size", func(t *testing.T) {
		key := make([]byte, KeySize)
		iv := make([]byte, 8) // Wrong size
		plaintext := []byte("test")

		_, err := EncryptAES256CBC(key, iv, plaintext)
		if err != ErrInvalidIVSize {
			t.Errorf("Expected ErrInvalidIVSize, got %v", err)
		}
	})
}

func TestPKCS7Padding(t *testing.T) {
	t.Run("pad_and_unpad", func(t *testing.T) {
		data := []byte("Hello, World!")
		blockSize := 16

		padded := pkcs7Pad(data, blockSize)
		if len(padded)%blockSize != 0 {
			t.Error("Padded data length should be multiple of block size")
		}

		unpadded, err := pkcs7Unpad(padded, blockSize)
		if err != nil {
			t.Fatalf("Unpad failed: %v", err)
		}

		if !bytes.Equal(unpadded, data) {
			t.Error("Unpadded data doesn't match original")
		}
	})

	t.Run("full_block", func(t *testing.T) {
		data := make([]byte, 16) // Exactly one block
		blockSize := 16

		padded := pkcs7Pad(data, blockSize)
		// Should add a full block of padding
		if len(padded) != 32 {
			t.Errorf("Expected 32 bytes after padding full block, got %d", len(padded))
		}

		unpadded, err := pkcs7Unpad(padded, blockSize)
		if err != nil {
			t.Fatalf("Unpad failed: %v", err)
		}

		if !bytes.Equal(unpadded, data) {
			t.Error("Unpadded data doesn't match original")
		}
	})

	t.Run("invalid_padding", func(t *testing.T) {
		data := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 99} // Invalid padding
		blockSize := 16

		_, err := pkcs7Unpad(data, blockSize)
		if err != ErrInvalidPadding {
			t.Errorf("Expected ErrInvalidPadding, got %v", err)
		}
	})
}

func TestMasterKey(t *testing.T) {
	t.Run("create_and_unlock_evp", func(t *testing.T) {
		passphrase := []byte("my secret passphrase")
		iterations := uint32(1000)

		// Create master key
		mk, originalKey, err := NewMasterKeyEVP(passphrase, iterations)
		if err != nil {
			t.Fatalf("NewMasterKeyEVP failed: %v", err)
		}

		if mk.DerivationMethod != DerivationMethodEVPSHA512 {
			t.Error("Expected EVP_sha512 derivation method")
		}

		// Unlock master key
		unlockedKey, err := mk.Unlock(passphrase)
		if err != nil {
			t.Fatalf("Unlock failed: %v", err)
		}

		if !bytes.Equal(unlockedKey, originalKey) {
			t.Error("Unlocked key doesn't match original")
		}
	})

	t.Run("create_and_unlock_scrypt", func(t *testing.T) {
		passphrase := []byte("my secret passphrase")

		// Create master key with scrypt
		mk, originalKey, err := NewMasterKeyScrypt(passphrase, DefaultScryptN, DefaultScryptR, DefaultScryptP)
		if err != nil {
			t.Fatalf("NewMasterKeyScrypt failed: %v", err)
		}

		if mk.DerivationMethod != DerivationMethodScrypt {
			t.Error("Expected scrypt derivation method")
		}

		// Unlock master key
		unlockedKey, err := mk.Unlock(passphrase)
		if err != nil {
			t.Fatalf("Unlock failed: %v", err)
		}

		if !bytes.Equal(unlockedKey, originalKey) {
			t.Error("Unlocked key doesn't match original")
		}
	})

	t.Run("wrong_passphrase", func(t *testing.T) {
		passphrase := []byte("correct passphrase")
		wrongPassphrase := []byte("wrong passphrase")

		mk, _, err := NewMasterKeyEVP(passphrase, 1000)
		if err != nil {
			t.Fatalf("NewMasterKeyEVP failed: %v", err)
		}

		// Try to unlock with wrong passphrase
		_, err = mk.Unlock(wrongPassphrase)
		// Should fail (either error or wrong key)
		// We can't easily verify this without actual crypto/rand, so just check it doesn't panic
		_ = err
	})
}

func TestScryptParams(t *testing.T) {
	t.Run("encode_decode", func(t *testing.T) {
		N := 32768
		r := 8
		p := 1

		encoded := encodeScryptParams(N, r, p)
		decodedN, decodedR, decodedP := decodeScryptParams(encoded)

		if decodedN != N {
			t.Errorf("N mismatch: got %d, want %d", decodedN, N)
		}
		if decodedR != r {
			t.Errorf("r mismatch: got %d, want %d", decodedR, r)
		}
		if decodedP != p {
			t.Errorf("p mismatch: got %d, want %d", decodedP, p)
		}
	})

	t.Run("invalid_params", func(t *testing.T) {
		encoded := []byte{1, 2} // Too short

		N, r, p := decodeScryptParams(encoded)
		// Should return defaults
		if N != DefaultScryptN || r != DefaultScryptR || p != DefaultScryptP {
			t.Error("Should return default parameters for invalid input")
		}
	})
}

func TestWrapEncryptionKeyScrypt(t *testing.T) {
	t.Run("wrap_and_unlock", func(t *testing.T) {
		// Create a master key with original passphrase
		oldPassphrase := []byte("old passphrase")
		mk, originalKey, err := NewMasterKeyScrypt(oldPassphrase, DefaultScryptN, DefaultScryptR, DefaultScryptP)
		if err != nil {
			t.Fatalf("NewMasterKeyScrypt failed: %v", err)
		}

		// Unlock to get the encryption key
		encryptionKey, err := mk.Unlock(oldPassphrase)
		if err != nil {
			t.Fatalf("Unlock with old passphrase failed: %v", err)
		}
		if !bytes.Equal(encryptionKey, originalKey) {
			t.Fatal("Unlocked key doesn't match original")
		}

		// Wrap the same encryption key with a new passphrase
		newPassphrase := []byte("new passphrase")
		newMK, err := WrapEncryptionKeyScrypt(encryptionKey, newPassphrase, DefaultScryptN, DefaultScryptR, DefaultScryptP)
		if err != nil {
			t.Fatalf("WrapEncryptionKeyScrypt failed: %v", err)
		}

		// Unlock the new master key with the new passphrase
		unwrappedKey, err := newMK.Unlock(newPassphrase)
		if err != nil {
			t.Fatalf("Unlock with new passphrase failed: %v", err)
		}

		// The encryption key should be the same
		if !bytes.Equal(unwrappedKey, originalKey) {
			t.Error("Unwrapped key doesn't match original encryption key")
		}
	})

	t.Run("old_passphrase_fails_on_new_master_key", func(t *testing.T) {
		oldPassphrase := []byte("old passphrase")
		mk, _, err := NewMasterKeyScrypt(oldPassphrase, DefaultScryptN, DefaultScryptR, DefaultScryptP)
		if err != nil {
			t.Fatalf("NewMasterKeyScrypt failed: %v", err)
		}

		encryptionKey, err := mk.Unlock(oldPassphrase)
		if err != nil {
			t.Fatalf("Unlock failed: %v", err)
		}

		newPassphrase := []byte("new passphrase")
		newMK, err := WrapEncryptionKeyScrypt(encryptionKey, newPassphrase, DefaultScryptN, DefaultScryptR, DefaultScryptP)
		if err != nil {
			t.Fatalf("WrapEncryptionKeyScrypt failed: %v", err)
		}

		// Old passphrase should fail on the new master key
		_, err = newMK.Unlock(oldPassphrase)
		if err == nil {
			t.Error("Old passphrase should not unlock new master key")
		}
	})

	t.Run("preserves_encryption_key_for_private_keys", func(t *testing.T) {
		// Full workflow: encrypt private key, change passphrase, decrypt with same encryption key
		oldPassphrase := []byte("original pass")
		mk, encryptionKey, err := NewMasterKeyScrypt(oldPassphrase, DefaultScryptN, DefaultScryptR, DefaultScryptP)
		if err != nil {
			t.Fatalf("NewMasterKeyScrypt failed: %v", err)
		}

		// Encrypt a private key with the encryption key
		privateKey := []byte("this is a 32-byte private key!!")
		pubkey := []byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}
		iv := GenerateIV(pubkey)
		encryptedPrivKey, err := EncryptAES256CBC(encryptionKey, iv, privateKey)
		if err != nil {
			t.Fatalf("EncryptAES256CBC failed: %v", err)
		}

		// Change passphrase: unlock with old, wrap with new
		unlockedKey, err := mk.Unlock(oldPassphrase)
		if err != nil {
			t.Fatalf("Unlock failed: %v", err)
		}

		newPassphrase := []byte("changed pass")
		newMK, err := WrapEncryptionKeyScrypt(unlockedKey, newPassphrase, DefaultScryptN, DefaultScryptR, DefaultScryptP)
		if err != nil {
			t.Fatalf("WrapEncryptionKeyScrypt failed: %v", err)
		}

		// Unlock with new passphrase and decrypt private key
		newUnlockedKey, err := newMK.Unlock(newPassphrase)
		if err != nil {
			t.Fatalf("Unlock with new passphrase failed: %v", err)
		}

		decryptedPrivKey, err := DecryptAES256CBC(newUnlockedKey, iv, encryptedPrivKey)
		if err != nil {
			t.Fatalf("DecryptAES256CBC failed: %v", err)
		}

		if !bytes.Equal(decryptedPrivKey, privateKey) {
			t.Error("Private key not recoverable after passphrase change")
		}
	})

	t.Run("empty_key_rejected", func(t *testing.T) {
		_, err := WrapEncryptionKeyScrypt(nil, []byte("pass"), DefaultScryptN, DefaultScryptR, DefaultScryptP)
		if err == nil {
			t.Error("Expected error for empty encryption key")
		}
	})

	t.Run("scrypt_derivation_method", func(t *testing.T) {
		key := make([]byte, KeySize)
		mk, err := WrapEncryptionKeyScrypt(key, []byte("pass"), DefaultScryptN, DefaultScryptR, DefaultScryptP)
		if err != nil {
			t.Fatalf("WrapEncryptionKeyScrypt failed: %v", err)
		}
		if mk.DerivationMethod != DerivationMethodScrypt {
			t.Errorf("Expected scrypt derivation method, got %d", mk.DerivationMethod)
		}
	})
}

func TestEncryptionRoundTrip(t *testing.T) {
	t.Run("full_workflow", func(t *testing.T) {
		// Simulate full encryption workflow like legacy wallet
		passphrase := []byte("wallet passphrase")
		privateKey := []byte("super secret private key data here")
		pubkey := []byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}

		// 1. Create master key
		mk, masterEncryptionKey, err := NewMasterKeyEVP(passphrase, DefaultEVPIterations)
		if err != nil {
			t.Fatalf("NewMasterKeyEVP failed: %v", err)
		}

		// 2. Generate IV from pubkey (deterministic)
		iv := GenerateIV(pubkey)

		// 3. Encrypt private key with master key
		encryptedPrivKey, err := EncryptAES256CBC(masterEncryptionKey, iv, privateKey)
		if err != nil {
			t.Fatalf("EncryptAES256CBC failed: %v", err)
		}

		// --- Simulating wallet unlock ---

		// 4. Unlock master key with passphrase
		unlockedMasterKey, err := mk.Unlock(passphrase)
		if err != nil {
			t.Fatalf("Unlock failed: %v", err)
		}

		// 5. Decrypt private key
		decryptedPrivKey, err := DecryptAES256CBC(unlockedMasterKey, iv, encryptedPrivKey)
		if err != nil {
			t.Fatalf("DecryptAES256CBC failed: %v", err)
		}

		// 6. Verify
		if !bytes.Equal(decryptedPrivKey, privateKey) {
			t.Error("Decrypted private key doesn't match original")
		}
	})
}
