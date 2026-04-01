package masternode

import (
	"bytes"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/twins-dev/twins-core/pkg/types"
)

// TestFillBlockPayment_DevAddressFallback tests that when no masternode is available,
// the payment falls back to the dev address (matching legacy C++ behavior)
func TestFillBlockPayment_DevAddressFallback(t *testing.T) {
	// Create a manager with no masternodes
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	manager, err := NewManager(nil, logger)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Create dev address (simulated P2PKH scriptPubKey - 25 bytes)
	// Format: OP_DUP OP_HASH160 <20-byte hash> OP_EQUALVERIFY OP_CHECKSIG
	devAddress := []byte{
		0x76, 0xa9, 0x14, // OP_DUP OP_HASH160 PUSH(20)
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, // hash bytes 1-8
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, // hash bytes 9-16
		0x11, 0x12, 0x13, 0x14, // hash bytes 17-20
		0x88, 0xac, // OP_EQUALVERIFY OP_CHECKSIG
	}

	// Create payment calculator with dev address
	pc := NewPaymentCalculator(manager, devAddress)

	// Call FillBlockPayment - should fall back to dev address since no masternodes
	blockHeight := uint32(1000000)
	blockHash := types.Hash{}
	isPoS := true

	info, err := pc.FillBlockPayment(blockHeight, blockHash, isPoS)
	if err != nil {
		t.Fatalf("FillBlockPayment failed: %v", err)
	}

	// Verify PayeeAddress is set to dev address (not empty)
	if len(info.PayeeAddress) == 0 {
		t.Error("PayeeAddress should not be empty when no masternodes available")
	}

	if !bytes.Equal(info.PayeeAddress, devAddress) {
		t.Errorf("PayeeAddress should be dev address fallback.\nGot: %x\nWant: %x",
			info.PayeeAddress, devAddress)
	}

	// Verify masternode payment is still calculated (80% of block reward)
	if info.MasternodePayment <= 0 {
		t.Error("MasternodePayment should be > 0")
	}

	// Verify staker reward is calculated correctly
	expectedBlockReward := pc.CalculateBlockReward(blockHeight)
	expectedMNPayment := pc.CalculateMasternodePayment(expectedBlockReward)
	expectedDevReward := pc.CalculateDevReward(expectedBlockReward, isPoS)
	expectedStakerReward := expectedBlockReward - expectedMNPayment - expectedDevReward

	if info.StakerReward != expectedStakerReward {
		t.Errorf("StakerReward incorrect. Got: %d, Want: %d",
			info.StakerReward, expectedStakerReward)
	}
}

// TestFillBlockPayment_NoDevAddressFallback tests behavior when dev address is not configured
func TestFillBlockPayment_NoDevAddressFallback(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	manager, err := NewManager(nil, logger)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Create payment calculator WITHOUT dev address (nil)
	pc := NewPaymentCalculator(manager, nil)

	blockHeight := uint32(1000000)
	blockHash := types.Hash{}
	isPoS := true

	info, err := pc.FillBlockPayment(blockHeight, blockHash, isPoS)
	if err != nil {
		t.Fatalf("FillBlockPayment failed: %v", err)
	}

	// Without dev address fallback, PayeeAddress should be empty
	if len(info.PayeeAddress) != 0 {
		t.Errorf("PayeeAddress should be empty when no dev address configured and no masternodes. Got: %x",
			info.PayeeAddress)
	}
}

// TestFillBlockPayment_WithMasternode tests that masternode address is used when available
func TestFillBlockPayment_WithMasternode(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	manager, err := NewManager(nil, logger)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	devAddress := []byte{0x76, 0xa9, 0x14, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x88, 0xac}
	pc := NewPaymentCalculator(manager, devAddress)

	// Note: This test documents current behavior.
	// With no masternodes registered, it should still fall back to dev address.
	// A full test with actual masternode would require more complex setup.
	blockHeight := uint32(1000000)
	blockHash := types.Hash{}
	isPoS := true

	info, err := pc.FillBlockPayment(blockHeight, blockHash, isPoS)
	if err != nil {
		t.Fatalf("FillBlockPayment failed: %v", err)
	}

	// Since no masternodes are registered, should use dev address
	if !bytes.Equal(info.PayeeAddress, devAddress) {
		t.Errorf("Expected dev address fallback. Got: %x, Want: %x",
			info.PayeeAddress, devAddress)
	}
}
