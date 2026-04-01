package mocks

import (
	"context"
	"testing"
)

func TestAddressValidation(t *testing.T) {
	// Create and start mock client
	mock := NewMockCoreClient()
	ctx := context.Background()
	if err := mock.Start(ctx); err != nil {
		t.Fatalf("Failed to start mock: %v", err)
	}
	defer mock.Stop()

	tests := []struct {
		name          string
		address       string
		shouldBeValid bool
		shouldBeScript bool
	}{
		{
			name:          "Real TWINS address from codebase (D prefix)",
			address:       "D7VFR83SQbiezrW72hjcWJtcfip5krte2Z",
			shouldBeValid: true,
			shouldBeScript: false,
		},
		{
			name:          "Invalid - too short",
			address:       "D1234567890",
			shouldBeValid: false,
		},
		{
			name:          "Invalid - wrong prefix",
			address:       "XXikstk7ktDie7NoP24KkmC5S7WWy6nJVF",
			shouldBeValid: false,
		},
		{
			name:          "Invalid - too long",
			address:       "D1234567890123456789012345678901234567890",
			shouldBeValid: false,
		},
		{
			name:          "Invalid - contains invalid Base58 char (0)",
			address:       "D0234567890123456789012345678901234",
			shouldBeValid: false,
		},
		{
			name:          "Invalid - bad checksum",
			address:       "DY7g3RjDc4KDv3yA6vy66owse7QXpvhtw9",
			shouldBeValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validation, err := mock.ValidateAddress(tt.address)
			if err != nil {
				t.Fatalf("ValidateAddress returned error: %v", err)
			}

			if validation.IsValid != tt.shouldBeValid {
				t.Errorf("Expected IsValid=%v, got %v", tt.shouldBeValid, validation.IsValid)
			}

			if tt.shouldBeValid && validation.IsScript != tt.shouldBeScript {
				t.Errorf("Expected IsScript=%v, got %v", tt.shouldBeScript, validation.IsScript)
			}
		})
	}
}

func TestGeneratedAddressValidation(t *testing.T) {
	// Create and start mock client
	mock := NewMockCoreClient()
	ctx := context.Background()
	if err := mock.Start(ctx); err != nil {
		t.Fatalf("Failed to start mock: %v", err)
	}
	defer mock.Stop()

	// Generate new address
	newAddr, err := mock.GetNewAddress("test")
	if err != nil {
		t.Fatalf("GetNewAddress failed: %v", err)
	}

	// Validate it
	validation, err := mock.ValidateAddress(newAddr)
	if err != nil {
		t.Fatalf("ValidateAddress failed: %v", err)
	}

	// Should be valid
	if !validation.IsValid {
		t.Error("Generated address should be valid")
	}

	// Should be mine
	if !validation.IsMine {
		t.Error("Generated address should be IsMine=true")
	}

	// Should have a pubkey
	if validation.PubKey == "" {
		t.Error("Generated address should have a pubkey")
	}

	// Should start with 'D' (P2PKH)
	if newAddr[0] != 'D' {
		t.Errorf("Generated address should start with 'D', got %c", newAddr[0])
	}

	// Should be typical length (usually 34 chars)
	if len(newAddr) < 26 || len(newAddr) > 35 {
		t.Errorf("Generated address length should be 26-35, got %d", len(newAddr))
	}
}

func TestOwnAddressTracking(t *testing.T) {
	// Create and start mock client
	mock := NewMockCoreClient()
	ctx := context.Background()
	if err := mock.Start(ctx); err != nil {
		t.Fatalf("Failed to start mock: %v", err)
	}
	defer mock.Stop()

	// Generate multiple addresses
	addresses := make([]string, 3)
	for i := 0; i < 3; i++ {
		addr, err := mock.GetNewAddress("test")
		if err != nil {
			t.Fatalf("GetNewAddress failed: %v", err)
		}
		addresses[i] = addr
	}

	// All should be tracked as mine
	for _, addr := range addresses {
		validation, err := mock.ValidateAddress(addr)
		if err != nil {
			t.Fatalf("ValidateAddress failed: %v", err)
		}

		if !validation.IsMine {
			t.Errorf("Address %s should be IsMine=true", addr)
		}

		if validation.PubKey == "" {
			t.Errorf("Address %s should have pubkey", addr)
		}
	}
}
