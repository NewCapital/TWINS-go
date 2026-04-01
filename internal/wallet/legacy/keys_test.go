package legacy

import (
	"bytes"
	"testing"
)

func TestCPubKey(t *testing.T) {
	t.Run("compressed_pubkey", func(t *testing.T) {
		// Compressed public key (33 bytes, 0x02 prefix)
		compressedKey := make([]byte, 33)
		compressedKey[0] = 0x02
		for i := 1; i < 33; i++ {
			compressedKey[i] = byte(i)
		}

		pk, err := NewCPubKey(compressedKey)
		if err != nil {
			t.Fatalf("NewCPubKey failed: %v", err)
		}

		if !pk.IsCompressed() {
			t.Error("Expected compressed key")
		}

		if !pk.IsValid() {
			t.Error("Expected valid key")
		}

		// Test serialization
		serialized, err := SerializeToBytes(pk)
		if err != nil {
			t.Fatalf("Serialize failed: %v", err)
		}

		// Test deserialization
		var pk2 CPubKey
		if err := DeserializeFromBytes(serialized, &pk2); err != nil {
			t.Fatalf("Deserialize failed: %v", err)
		}

		if !bytes.Equal(pk.Bytes(), pk2.Bytes()) {
			t.Errorf("Round-trip failed: got %x, want %x", pk2.Bytes(), pk.Bytes())
		}
	})

	t.Run("uncompressed_pubkey", func(t *testing.T) {
		// Uncompressed public key (65 bytes, 0x04 prefix)
		uncompressedKey := make([]byte, 65)
		uncompressedKey[0] = 0x04
		for i := 1; i < 65; i++ {
			uncompressedKey[i] = byte(i)
		}

		pk, err := NewCPubKey(uncompressedKey)
		if err != nil {
			t.Fatalf("NewCPubKey failed: %v", err)
		}

		if pk.IsCompressed() {
			t.Error("Expected uncompressed key")
		}

		if !pk.IsValid() {
			t.Error("Expected valid key")
		}
	})

	t.Run("invalid_pubkey", func(t *testing.T) {
		// Wrong length
		_, err := NewCPubKey(make([]byte, 32))
		if err != ErrInvalidPubKey {
			t.Errorf("Expected ErrInvalidPubKey, got %v", err)
		}

		// Wrong prefix for compressed
		invalidCompressed := make([]byte, 33)
		invalidCompressed[0] = 0x05
		_, err = NewCPubKey(invalidCompressed)
		if err != ErrInvalidPubKey {
			t.Errorf("Expected ErrInvalidPubKey, got %v", err)
		}
	})
}

func TestCPrivKey(t *testing.T) {
	t.Run("valid_privkey", func(t *testing.T) {
		// DER-encoded private key (starts with 0x30)
		derKey := make([]byte, 279)
		derKey[0] = 0x30 // DER sequence tag
		derKey[1] = 0x77 // Length
		for i := 2; i < len(derKey); i++ {
			derKey[i] = byte(i)
		}

		pk, err := NewCPrivKey(derKey)
		if err != nil {
			t.Fatalf("NewCPrivKey failed: %v", err)
		}

		// Test serialization
		serialized, err := SerializeToBytes(pk)
		if err != nil {
			t.Fatalf("Serialize failed: %v", err)
		}

		// Test deserialization
		var pk2 CPrivKey
		if err := DeserializeFromBytes(serialized, &pk2); err != nil {
			t.Fatalf("Deserialize failed: %v", err)
		}

		if !bytes.Equal(pk.Bytes(), pk2.Bytes()) {
			t.Errorf("Round-trip failed")
		}
	})

	t.Run("invalid_privkey", func(t *testing.T) {
		// Empty key
		_, err := NewCPrivKey([]byte{})
		if err != ErrInvalidPrivKey {
			t.Errorf("Expected ErrInvalidPrivKey, got %v", err)
		}

		// Invalid DER format
		_, err = NewCPrivKey([]byte{0x00, 0x01, 0x02})
		if err != ErrInvalidPrivKey {
			t.Errorf("Expected ErrInvalidPrivKey, got %v", err)
		}
	})
}

func TestCKeyMetadata(t *testing.T) {
	t.Run("basic_metadata", func(t *testing.T) {
		km := NewCKeyMetadata(1234567890)
		km.HDKeyPath = "m/44'/0'/0'/0/0"
		km.HDMasterKeyID = []byte{0x01, 0x02, 0x03, 0x04}

		// Test serialization
		serialized, err := SerializeToBytes(km)
		if err != nil {
			t.Fatalf("Serialize failed: %v", err)
		}

		// Test deserialization
		var km2 CKeyMetadata
		if err := DeserializeFromBytes(serialized, &km2); err != nil {
			t.Fatalf("Deserialize failed: %v", err)
		}

		if km2.Version != km.Version {
			t.Errorf("Version mismatch: got %d, want %d", km2.Version, km.Version)
		}
		if km2.CreateTime != km.CreateTime {
			t.Errorf("CreateTime mismatch: got %d, want %d", km2.CreateTime, km.CreateTime)
		}
		if km2.HDKeyPath != km.HDKeyPath {
			t.Errorf("HDKeyPath mismatch: got %q, want %q", km2.HDKeyPath, km.HDKeyPath)
		}
		if !bytes.Equal(km2.HDMasterKeyID, km.HDMasterKeyID) {
			t.Errorf("HDMasterKeyID mismatch")
		}
	})

	t.Run("empty_hd_fields", func(t *testing.T) {
		km := NewCKeyMetadata(1234567890)
		// Leave HD fields empty

		serialized, err := SerializeToBytes(km)
		if err != nil {
			t.Fatalf("Serialize failed: %v", err)
		}

		var km2 CKeyMetadata
		if err := DeserializeFromBytes(serialized, &km2); err != nil {
			t.Fatalf("Deserialize failed: %v", err)
		}

		if km2.HDKeyPath != "" {
			t.Errorf("Expected empty HDKeyPath, got %q", km2.HDKeyPath)
		}
		if len(km2.HDMasterKeyID) != 0 {
			t.Errorf("Expected empty HDMasterKeyID")
		}
	})
}

func TestCMasterKey(t *testing.T) {
	t.Run("sha512_derivation", func(t *testing.T) {
		encryptedKey := make([]byte, 32)
		salt := make([]byte, 8)
		for i := range encryptedKey {
			encryptedKey[i] = byte(i)
		}
		for i := range salt {
			salt[i] = byte(i + 100)
		}

		mk := NewCMasterKey(encryptedKey, salt, DerivationMethodSHA512, 25000)

		// Test serialization
		serialized, err := SerializeToBytes(mk)
		if err != nil {
			t.Fatalf("Serialize failed: %v", err)
		}

		// Test deserialization
		var mk2 CMasterKey
		if err := DeserializeFromBytes(serialized, &mk2); err != nil {
			t.Fatalf("Deserialize failed: %v", err)
		}

		if !bytes.Equal(mk2.EncryptedKey, mk.EncryptedKey) {
			t.Error("EncryptedKey mismatch")
		}
		if !bytes.Equal(mk2.Salt, mk.Salt) {
			t.Error("Salt mismatch")
		}
		if mk2.DerivationMethod != mk.DerivationMethod {
			t.Errorf("DerivationMethod mismatch: got %d, want %d", mk2.DerivationMethod, mk.DerivationMethod)
		}
		if mk2.DeriveIterations != mk.DeriveIterations {
			t.Errorf("DeriveIterations mismatch: got %d, want %d", mk2.DeriveIterations, mk.DeriveIterations)
		}
	})

	t.Run("scrypt_derivation", func(t *testing.T) {
		encryptedKey := make([]byte, 32)
		salt := make([]byte, 8)
		scryptParams := []byte{0x00, 0x80, 0x00, 0x00} // N=32768 in little-endian

		mk := NewCMasterKey(encryptedKey, salt, DerivationMethodScrypt, 1)
		mk.OtherDerivationParameters = scryptParams

		serialized, err := SerializeToBytes(mk)
		if err != nil {
			t.Fatalf("Serialize failed: %v", err)
		}

		var mk2 CMasterKey
		if err := DeserializeFromBytes(serialized, &mk2); err != nil {
			t.Fatalf("Deserialize failed: %v", err)
		}

		if mk2.DerivationMethod != DerivationMethodScrypt {
			t.Errorf("Expected scrypt derivation method")
		}
		if !bytes.Equal(mk2.OtherDerivationParameters, scryptParams) {
			t.Error("OtherDerivationParameters mismatch")
		}
	})
}

func TestRoundTrip(t *testing.T) {
	t.Run("all_key_types", func(t *testing.T) {
		// Test multiple structures in sequence
		var buf bytes.Buffer

		// CPubKey
		compressedKey := make([]byte, 33)
		compressedKey[0] = 0x02
		pk, _ := NewCPubKey(compressedKey)
		if err := pk.Serialize(&buf); err != nil {
			t.Fatal(err)
		}

		// CKeyMetadata
		km := NewCKeyMetadata(1234567890)
		if err := km.Serialize(&buf); err != nil {
			t.Fatal(err)
		}

		// CMasterKey
		mk := NewCMasterKey(make([]byte, 32), make([]byte, 8), DerivationMethodSHA512, 25000)
		if err := mk.Serialize(&buf); err != nil {
			t.Fatal(err)
		}

		// Read them back
		r := bytes.NewReader(buf.Bytes())

		var pk2 CPubKey
		if err := pk2.Deserialize(r); err != nil {
			t.Fatal(err)
		}

		var km2 CKeyMetadata
		if err := km2.Deserialize(r); err != nil {
			t.Fatal(err)
		}

		var mk2 CMasterKey
		if err := mk2.Deserialize(r); err != nil {
			t.Fatal(err)
		}

		// Verify
		if !bytes.Equal(pk.Bytes(), pk2.Bytes()) {
			t.Error("CPubKey mismatch")
		}
		if km2.CreateTime != km.CreateTime {
			t.Error("CKeyMetadata mismatch")
		}
		if mk2.DeriveIterations != mk.DeriveIterations {
			t.Error("CMasterKey mismatch")
		}
	})
}
