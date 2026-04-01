package legacy

import (
	"bytes"
	"testing"
)

func TestCWalletTx(t *testing.T) {
	t.Run("basic_transaction", func(t *testing.T) {
		txHash := make([]byte, 32)
		for i := range txHash {
			txHash[i] = byte(i)
		}

		wtx := NewCWalletTx(txHash, 1234567890)
		wtx.OrderPos = 42
		wtx.FromAccount = "test-account"
		wtx.MapValue["comment"] = "Test transaction"
		wtx.MapValue["to"] = "Alice"

		// Test serialization
		serialized, err := SerializeToBytes(wtx)
		if err != nil {
			t.Fatalf("Serialize failed: %v", err)
		}

		// Test deserialization
		var wtx2 CWalletTx
		if err := DeserializeFromBytes(serialized, &wtx2); err != nil {
			t.Fatalf("Deserialize failed: %v", err)
		}

		if !bytes.Equal(wtx2.TxHash, wtx.TxHash) {
			t.Error("TxHash mismatch")
		}
		if wtx2.TimeReceived != wtx.TimeReceived {
			t.Errorf("TimeReceived mismatch: got %d, want %d", wtx2.TimeReceived, wtx.TimeReceived)
		}
		if wtx2.OrderPos != wtx.OrderPos {
			t.Errorf("OrderPos mismatch: got %d, want %d", wtx2.OrderPos, wtx.OrderPos)
		}
		if wtx2.FromAccount != wtx.FromAccount {
			t.Errorf("FromAccount mismatch: got %q, want %q", wtx2.FromAccount, wtx.FromAccount)
		}
		if len(wtx2.MapValue) != len(wtx.MapValue) {
			t.Errorf("MapValue length mismatch: got %d, want %d", len(wtx2.MapValue), len(wtx.MapValue))
		}
		for k, v := range wtx.MapValue {
			if wtx2.MapValue[k] != v {
				t.Errorf("MapValue[%q] mismatch: got %q, want %q", k, wtx2.MapValue[k], v)
			}
		}
	})

	t.Run("empty_mapvalue", func(t *testing.T) {
		txHash := make([]byte, 32)
		wtx := NewCWalletTx(txHash, 1234567890)

		serialized, err := SerializeToBytes(wtx)
		if err != nil {
			t.Fatalf("Serialize failed: %v", err)
		}

		var wtx2 CWalletTx
		if err := DeserializeFromBytes(serialized, &wtx2); err != nil {
			t.Fatalf("Deserialize failed: %v", err)
		}

		if len(wtx2.MapValue) != 0 {
			t.Errorf("Expected empty MapValue, got %d entries", len(wtx2.MapValue))
		}
	})
}

func TestCHDChain(t *testing.T) {
	t.Run("basic_hdchain", func(t *testing.T) {
		seed := make([]byte, 64)
		for i := range seed {
			seed[i] = byte(i)
		}

		hd := NewCHDChain(seed)
		// Set counters via MapAccounts (legacy format uses map)
		hd.MapAccounts[0] = CHDAccount{
			ExternalCounter: 10,
			InternalCounter: 5,
		}
		hd.ExternalCounter = 10
		hd.InternalCounter = 5

		// Test serialization
		serialized, err := SerializeToBytes(hd)
		if err != nil {
			t.Fatalf("Serialize failed: %v", err)
		}

		// Test deserialization
		var hd2 CHDChain
		if err := DeserializeFromBytes(serialized, &hd2); err != nil {
			t.Fatalf("Deserialize failed: %v", err)
		}

		if hd2.Version != hd.Version {
			t.Errorf("Version mismatch: got %d, want %d", hd2.Version, hd.Version)
		}
		if !bytes.Equal(hd2.ChainID, hd.ChainID) {
			t.Error("ChainID mismatch")
		}
		if hd2.ExternalCounter != hd.ExternalCounter {
			t.Errorf("ExternalCounter mismatch: got %d, want %d", hd2.ExternalCounter, hd.ExternalCounter)
		}
		if hd2.InternalCounter != hd.InternalCounter {
			t.Errorf("InternalCounter mismatch: got %d, want %d", hd2.InternalCounter, hd.InternalCounter)
		}
		if !bytes.Equal(hd2.Seed, hd.Seed) {
			t.Error("Seed mismatch")
		}
		// Check MapAccounts
		if len(hd2.MapAccounts) != 1 {
			t.Errorf("MapAccounts length mismatch: got %d, want 1", len(hd2.MapAccounts))
		}
		if acc, ok := hd2.MapAccounts[0]; !ok || acc.ExternalCounter != 10 || acc.InternalCounter != 5 {
			t.Error("MapAccounts[0] mismatch")
		}
	})

	t.Run("zero_counters", func(t *testing.T) {
		seed := make([]byte, 32)
		hd := NewCHDChain(seed)

		serialized, err := SerializeToBytes(hd)
		if err != nil {
			t.Fatalf("Serialize failed: %v", err)
		}

		var hd2 CHDChain
		if err := DeserializeFromBytes(serialized, &hd2); err != nil {
			t.Fatalf("Deserialize failed: %v", err)
		}

		if hd2.ExternalCounter != 0 {
			t.Errorf("Expected zero ExternalCounter, got %d", hd2.ExternalCounter)
		}
		if hd2.InternalCounter != 0 {
			t.Errorf("Expected zero InternalCounter, got %d", hd2.InternalCounter)
		}
	})
}

func TestCHDPubKey(t *testing.T) {
	t.Run("basic_hdpubkey", func(t *testing.T) {
		// Create compressed public key
		compressedKey := make([]byte, 33)
		compressedKey[0] = 0x02
		for i := 1; i < 33; i++ {
			compressedKey[i] = byte(i)
		}

		chainCode := [32]byte{}
		for i := range chainCode {
			chainCode[i] = byte(i)
		}
		parentFP := [4]byte{0x01, 0x02, 0x03, 0x04}

		// Create CHDPubKey with the current struct-based approach
		hpk := &CHDPubKey{
			Version: 1,
			ExtPubKey: CExtPubKey{
				Depth:       3,
				Fingerprint: parentFP,
				Child:       5,
				ChainCode:   chainCode,
				PubKey:      compressedKey,
			},
			HDChainID:    make([]byte, 32),
			AccountIndex: 0,
			ChangeIndex:  0,
		}

		// Test serialization
		serialized, err := SerializeToBytes(hpk)
		if err != nil {
			t.Fatalf("Serialize failed: %v", err)
		}

		// Test deserialization
		var hpk2 CHDPubKey
		if err := DeserializeFromBytes(serialized, &hpk2); err != nil {
			t.Fatalf("Deserialize failed: %v", err)
		}

		if !bytes.Equal(hpk2.ExtPubKey.PubKey, hpk.ExtPubKey.PubKey) {
			t.Error("ExtPubKey.PubKey mismatch")
		}
		if hpk2.ExtPubKey.ChainCode != hpk.ExtPubKey.ChainCode {
			t.Error("ChainCode mismatch")
		}
		if hpk2.ExtPubKey.Fingerprint != hpk.ExtPubKey.Fingerprint {
			t.Error("Fingerprint mismatch")
		}
		if hpk2.ExtPubKey.Child != hpk.ExtPubKey.Child {
			t.Errorf("Child mismatch: got %d, want %d", hpk2.ExtPubKey.Child, hpk.ExtPubKey.Child)
		}
		if hpk2.AccountIndex != hpk.AccountIndex {
			t.Errorf("AccountIndex mismatch: got %d, want %d", hpk2.AccountIndex, hpk.AccountIndex)
		}
		if hpk2.ChangeIndex != hpk.ChangeIndex {
			t.Errorf("ChangeIndex mismatch: got %d, want %d", hpk2.ChangeIndex, hpk.ChangeIndex)
		}
	})

	t.Run("hardened_child", func(t *testing.T) {
		compressedKey := make([]byte, 33)
		compressedKey[0] = 0x03
		for i := 1; i < 33; i++ {
			compressedKey[i] = byte(i + 100)
		}

		// Hardened child index (0x80000000 | 44)
		hpk := &CHDPubKey{
			Version: 1,
			ExtPubKey: CExtPubKey{
				Depth:       2,
				Fingerprint: [4]byte{0x00, 0x00, 0x00, 0x00},
				Child:       0x80000000, // Hardened child
				ChainCode:   [32]byte{},
				PubKey:      compressedKey,
			},
			HDChainID:    make([]byte, 32),
			AccountIndex: 0,
			ChangeIndex:  0,
		}

		serialized, err := SerializeToBytes(hpk)
		if err != nil {
			t.Fatalf("Serialize failed: %v", err)
		}

		var hpk2 CHDPubKey
		if err := DeserializeFromBytes(serialized, &hpk2); err != nil {
			t.Fatalf("Deserialize failed: %v", err)
		}

		if hpk2.ExtPubKey.Child != 0x80000000 {
			t.Errorf("Expected hardened child number 0x80000000, got 0x%x", hpk2.ExtPubKey.Child)
		}
	})
}

func TestCBlockLocator(t *testing.T) {
	t.Run("basic_locator", func(t *testing.T) {
		hashes := make([][]byte, 3)
		for i := range hashes {
			hashes[i] = make([]byte, 32)
			for j := range hashes[i] {
				hashes[i][j] = byte(i*32 + j)
			}
		}

		bl := NewCBlockLocator(hashes)

		// Test serialization
		serialized, err := SerializeToBytes(bl)
		if err != nil {
			t.Fatalf("Serialize failed: %v", err)
		}

		// Test deserialization
		var bl2 CBlockLocator
		if err := DeserializeFromBytes(serialized, &bl2); err != nil {
			t.Fatalf("Deserialize failed: %v", err)
		}

		if len(bl2.BlockHashes) != len(bl.BlockHashes) {
			t.Errorf("BlockHashes length mismatch: got %d, want %d", len(bl2.BlockHashes), len(bl.BlockHashes))
		}

		for i := range bl.BlockHashes {
			if !bytes.Equal(bl2.BlockHashes[i], bl.BlockHashes[i]) {
				t.Errorf("BlockHash[%d] mismatch", i)
			}
		}
	})

	t.Run("empty_locator", func(t *testing.T) {
		bl := NewCBlockLocator([][]byte{})

		serialized, err := SerializeToBytes(bl)
		if err != nil {
			t.Fatalf("Serialize failed: %v", err)
		}

		var bl2 CBlockLocator
		if err := DeserializeFromBytes(serialized, &bl2); err != nil {
			t.Fatalf("Deserialize failed: %v", err)
		}

		if len(bl2.BlockHashes) != 0 {
			t.Errorf("Expected empty BlockHashes, got %d entries", len(bl2.BlockHashes))
		}
	})

	t.Run("single_hash", func(t *testing.T) {
		hash := make([]byte, 32)
		for i := range hash {
			hash[i] = 0xFF
		}

		bl := NewCBlockLocator([][]byte{hash})

		serialized, err := SerializeToBytes(bl)
		if err != nil {
			t.Fatalf("Serialize failed: %v", err)
		}

		var bl2 CBlockLocator
		if err := DeserializeFromBytes(serialized, &bl2); err != nil {
			t.Fatalf("Deserialize failed: %v", err)
		}

		if len(bl2.BlockHashes) != 1 {
			t.Errorf("Expected 1 hash, got %d", len(bl2.BlockHashes))
		}
		if !bytes.Equal(bl2.BlockHashes[0], hash) {
			t.Error("Hash mismatch")
		}
	})
}

func TestComplexRoundTrip(t *testing.T) {
	t.Run("all_wallet_types", func(t *testing.T) {
		var buf bytes.Buffer

		// CWalletTx
		txHash := make([]byte, 32)
		wtx := NewCWalletTx(txHash, 1234567890)
		wtx.MapValue["test"] = "value"
		if err := wtx.Serialize(&buf); err != nil {
			t.Fatal(err)
		}

		// CHDChain
		hd := NewCHDChain(make([]byte, 64))
		if err := hd.Serialize(&buf); err != nil {
			t.Fatal(err)
		}

		// CHDPubKey
		hpk := &CHDPubKey{
			Version: 1,
			ExtPubKey: CExtPubKey{
				Depth:       0,
				Fingerprint: [4]byte{},
				Child:       0,
				ChainCode:   [32]byte{},
				PubKey:      append([]byte{0x02}, make([]byte, 32)...),
			},
			HDChainID:    make([]byte, 32),
			AccountIndex: 0,
			ChangeIndex:  0,
		}
		if err := hpk.Serialize(&buf); err != nil {
			t.Fatal(err)
		}

		// CBlockLocator
		bl := NewCBlockLocator([][]byte{make([]byte, 32)})
		if err := bl.Serialize(&buf); err != nil {
			t.Fatal(err)
		}

		// Read them all back
		r := bytes.NewReader(buf.Bytes())

		var wtx2 CWalletTx
		if err := wtx2.Deserialize(r); err != nil {
			t.Fatal(err)
		}

		var hd2 CHDChain
		if err := hd2.Deserialize(r); err != nil {
			t.Fatal(err)
		}

		var hpk2 CHDPubKey
		if err := hpk2.Deserialize(r); err != nil {
			t.Fatal(err)
		}

		var bl2 CBlockLocator
		if err := bl2.Deserialize(r); err != nil {
			t.Fatal(err)
		}

		// Verify
		if wtx2.TimeReceived != wtx.TimeReceived {
			t.Error("CWalletTx mismatch")
		}
		if hd2.ExternalCounter != hd.ExternalCounter {
			t.Error("CHDChain mismatch")
		}
		if hpk2.ExtPubKey.Child != hpk.ExtPubKey.Child {
			t.Error("CHDPubKey mismatch")
		}
		if len(bl2.BlockHashes) != len(bl.BlockHashes) {
			t.Error("CBlockLocator mismatch")
		}
	})
}
