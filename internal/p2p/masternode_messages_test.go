package p2p

import (
	"bytes"
	"encoding/hex"
	"net"
	"testing"
	"time"

	"github.com/twins-dev/twins-core/internal/masternode"
	"github.com/twins-dev/twins-core/pkg/crypto"
	"github.com/twins-dev/twins-core/pkg/types"
)

func TestMasternodeBroadcastSerialization(t *testing.T) {
	// Create a test private key
	keyPair, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}
	privKey := keyPair.Private
	pubKey := privKey.PublicKey()

	// Create test outpoint
	var txHash types.Hash
	copy(txHash[:], []byte("test-transaction-hash-32byte"))
	outpoint := types.Outpoint{
		Hash:  txHash,
		Index: 0,
	}

	// Create test address
	addr := &net.TCPAddr{
		IP:   net.ParseIP("192.168.1.100"),
		Port: 19901,
	}

	// Create test masternode broadcast
	mnb := &masternode.MasternodeBroadcast{
		OutPoint:         outpoint,
		Addr:             addr,
		PubKeyCollateral: pubKey,
		PubKeyMasternode: pubKey,
		Signature:        []byte("test-signature-data-65-bytes-long-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"),
		SigTime:          time.Now().Unix(),
		Protocol:         masternode.ActiveProtocolVersion,
		LastPing:         nil,
		LastDsq:          0,
	}

	// Test serialization
	serialized, err := SerializeMasternodeBroadcast(mnb)
	if err != nil {
		t.Fatalf("Failed to serialize broadcast: %v", err)
	}

	if len(serialized) == 0 {
		t.Fatal("Serialized data is empty")
	}

	t.Logf("Serialized broadcast: %d bytes", len(serialized))
	t.Logf("Hex: %s", hex.EncodeToString(serialized))

	// Test deserialization
	deserialized, err := DeserializeMasternodeBroadcast(serialized)
	if err != nil {
		t.Fatalf("Failed to deserialize broadcast: %v", err)
	}

	// Verify fields match
	if deserialized.OutPoint.Hash != mnb.OutPoint.Hash {
		t.Errorf("Outpoint hash mismatch: got %v, want %v", deserialized.OutPoint.Hash, mnb.OutPoint.Hash)
	}

	if deserialized.OutPoint.Index != mnb.OutPoint.Index {
		t.Errorf("Outpoint index mismatch: got %d, want %d", deserialized.OutPoint.Index, mnb.OutPoint.Index)
	}

	if deserialized.Addr.String() != mnb.Addr.String() {
		t.Errorf("Address mismatch: got %s, want %s", deserialized.Addr.String(), mnb.Addr.String())
	}

	if !deserialized.PubKeyCollateral.IsEqual(mnb.PubKeyCollateral) {
		t.Error("Collateral public key mismatch")
	}

	if !deserialized.PubKeyMasternode.IsEqual(mnb.PubKeyMasternode) {
		t.Error("Masternode public key mismatch")
	}

	if deserialized.SigTime != mnb.SigTime {
		t.Errorf("SigTime mismatch: got %d, want %d", deserialized.SigTime, mnb.SigTime)
	}

	if deserialized.Protocol != mnb.Protocol {
		t.Errorf("Protocol mismatch: got %d, want %d", deserialized.Protocol, mnb.Protocol)
	}

	t.Log("Serialization/deserialization test passed!")
}

func TestMasternodePingSerialization(t *testing.T) {
	// Create test outpoint
	var txHash types.Hash
	copy(txHash[:], []byte("test-transaction-hash-32byte"))
	outpoint := types.Outpoint{
		Hash:  txHash,
		Index: 1,
	}

	// Create test block hash
	var blockHash types.Hash
	copy(blockHash[:], []byte("test-block-hash-data-32bytes"))

	// Create test masternode ping (legacy format - no sentinel fields)
	mnp := &masternode.MasternodePing{
		OutPoint:  outpoint,
		BlockHash: blockHash,
		SigTime:   time.Now().Unix(),
		Signature: []byte("test-ping-signature-data-65-bytes-xxxxxxxxxxxxxxxxxxxxxxxxxx"),
		// Note: SentinelPing and SentinelVersion are not part of legacy wire format
	}

	// Test serialization
	serialized, err := SerializeMasternodePing(mnp)
	if err != nil {
		t.Fatalf("Failed to serialize ping: %v", err)
	}

	if len(serialized) == 0 {
		t.Fatal("Serialized data is empty")
	}

	t.Logf("Serialized ping: %d bytes", len(serialized))
	t.Logf("Hex: %s", hex.EncodeToString(serialized))

	// Test deserialization
	deserialized, err := DeserializeMasternodePing(serialized)
	if err != nil {
		t.Fatalf("Failed to deserialize ping: %v", err)
	}

	// Verify fields match (legacy format fields only)
	if deserialized.OutPoint.Hash != mnp.OutPoint.Hash {
		t.Errorf("Outpoint hash mismatch: got %v, want %v", deserialized.OutPoint.Hash, mnp.OutPoint.Hash)
	}

	if deserialized.OutPoint.Index != mnp.OutPoint.Index {
		t.Errorf("Outpoint index mismatch: got %d, want %d", deserialized.OutPoint.Index, mnp.OutPoint.Index)
	}

	if deserialized.BlockHash != mnp.BlockHash {
		t.Errorf("BlockHash mismatch: got %v, want %v", deserialized.BlockHash, mnp.BlockHash)
	}

	if deserialized.SigTime != mnp.SigTime {
		t.Errorf("SigTime mismatch: got %d, want %d", deserialized.SigTime, mnp.SigTime)
	}

	if !bytes.Equal(deserialized.Signature, mnp.Signature) {
		t.Errorf("Signature mismatch")
	}

	t.Log("Ping serialization/deserialization test passed!")
}

func TestMasternodeBroadcastWithPing(t *testing.T) {
	// Create a test private key
	keyPair, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}
	privKey := keyPair.Private
	pubKey := privKey.PublicKey()

	// Create test data
	var txHash types.Hash
	copy(txHash[:], []byte("test-transaction-hash-32byte"))
	outpoint := types.Outpoint{
		Hash:  txHash,
		Index: 0,
	}

	var blockHash types.Hash
	copy(blockHash[:], []byte("test-block-hash-data-32bytes"))

	addr := &net.TCPAddr{
		IP:   net.ParseIP("10.0.0.50"),
		Port: 19901,
	}

	// Create ping
	ping := &masternode.MasternodePing{
		OutPoint:        outpoint,
		BlockHash:       blockHash,
		SigTime:         time.Now().Unix(),
		Signature:       []byte("test-signature"),
		SentinelPing:    false,
		SentinelVersion: "",
	}

	// Create broadcast with ping
	mnb := &masternode.MasternodeBroadcast{
		OutPoint:         outpoint,
		Addr:             addr,
		PubKeyCollateral: pubKey,
		PubKeyMasternode: pubKey,
		Signature:        []byte("test-signature-broadcast"),
		SigTime:          time.Now().Unix(),
		Protocol:         masternode.ActiveProtocolVersion,
		LastPing:         ping,
		LastDsq:          0,
	}

	// Test serialization
	serialized, err := SerializeMasternodeBroadcast(mnb)
	if err != nil {
		t.Fatalf("Failed to serialize broadcast with ping: %v", err)
	}

	// Test deserialization
	deserialized, err := DeserializeMasternodeBroadcast(serialized)
	if err != nil {
		t.Fatalf("Failed to deserialize broadcast with ping: %v", err)
	}

	// Verify ping was included
	if deserialized.LastPing == nil {
		t.Fatal("LastPing should not be nil after deserialization")
	}

	if deserialized.LastPing.SigTime != ping.SigTime {
		t.Errorf("Ping SigTime mismatch: got %d, want %d", deserialized.LastPing.SigTime, ping.SigTime)
	}

	t.Log("Broadcast with ping serialization test passed!")
}

func TestMasternodeVarIntSerialization(t *testing.T) {
	tests := []struct {
		name  string
		value uint64
	}{
		{"small", 0xfc},
		{"medium", 0xfdff},
		{"large", 0xffffffff},
		{"very large", 0xffffffffffffffff},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Serialize
			buf := &bytes.Buffer{}
			if err := writeVarInt(buf, tt.value); err != nil {
				t.Fatalf("Failed to write varint: %v", err)
			}

			// Deserialize
			result, err := readVarInt(buf)
			if err != nil {
				t.Fatalf("Failed to read varint: %v", err)
			}

			if result != tt.value {
				t.Errorf("VarInt mismatch: got %d, want %d", result, tt.value)
			}
		})
	}
}
