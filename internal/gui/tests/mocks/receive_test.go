package mocks

import (
	"context"
	"strings"
	"testing"
)

func TestGetReceivingAddresses(t *testing.T) {
	mock := NewMockCoreClient()
	ctx := context.Background()

	if err := mock.Start(ctx); err != nil {
		t.Fatalf("failed to start mock: %v", err)
	}
	defer mock.Stop()

	// Get initial receiving addresses (should have 5 from initialization)
	addresses, err := mock.GetReceivingAddresses()
	if err != nil {
		t.Fatalf("GetReceivingAddresses failed: %v", err)
	}

	if len(addresses) != 5 {
		t.Errorf("expected 5 initial addresses, got %d", len(addresses))
	}

	// Verify addresses have valid format
	for _, addr := range addresses {
		if !strings.HasPrefix(addr.Address, "D") {
			t.Errorf("address should start with 'D', got: %s", addr.Address)
		}
		if len(addr.Address) != 34 {
			t.Errorf("address should be 34 characters, got %d: %s", len(addr.Address), addr.Address)
		}
	}
}

func TestGenerateReceivingAddress(t *testing.T) {
	mock := NewMockCoreClient()
	ctx := context.Background()

	if err := mock.Start(ctx); err != nil {
		t.Fatalf("failed to start mock: %v", err)
	}
	defer mock.Stop()

	// Generate a new address with label
	label := "Test Address"
	addr, err := mock.GenerateReceivingAddress(label)
	if err != nil {
		t.Fatalf("GenerateReceivingAddress failed: %v", err)
	}

	// Verify address format
	if !strings.HasPrefix(addr.Address, "D") {
		t.Errorf("address should start with 'D', got: %s", addr.Address)
	}
	if len(addr.Address) != 34 {
		t.Errorf("address should be 34 characters, got %d", len(addr.Address))
	}
	if addr.Label != label {
		t.Errorf("expected label '%s', got '%s'", label, addr.Label)
	}
	if addr.Created.IsZero() {
		t.Error("Created timestamp should not be zero")
	}

	// Verify it was added to the list
	addresses, err := mock.GetReceivingAddresses()
	if err != nil {
		t.Fatalf("GetReceivingAddresses failed: %v", err)
	}

	if len(addresses) != 6 { // 5 initial + 1 new
		t.Errorf("expected 6 addresses, got %d", len(addresses))
	}
}

func TestCreatePaymentRequest(t *testing.T) {
	mock := NewMockCoreClient()
	ctx := context.Background()

	if err := mock.Start(ctx); err != nil {
		t.Fatalf("failed to start mock: %v", err)
	}
	defer mock.Stop()

	// Get an address to use
	addresses, _ := mock.GetReceivingAddresses()
	testAddr := addresses[0].Address

	// Create a payment request
	label := "Test Payment"
	message := "Please send payment"
	amount := 100.5

	request, err := mock.CreatePaymentRequest(testAddr, label, message, amount)
	if err != nil {
		t.Fatalf("CreatePaymentRequest failed: %v", err)
	}

	// Verify request fields
	if request.ID != 1 {
		t.Errorf("expected ID 1, got %d", request.ID)
	}
	if request.Address != testAddr {
		t.Errorf("expected address '%s', got '%s'", testAddr, request.Address)
	}
	if request.Label != label {
		t.Errorf("expected label '%s', got '%s'", label, request.Label)
	}
	if request.Message != message {
		t.Errorf("expected message '%s', got '%s'", message, request.Message)
	}
	if request.Amount != amount {
		t.Errorf("expected amount %f, got %f", amount, request.Amount)
	}
	if request.Date.IsZero() {
		t.Error("Date should not be zero")
	}

	// Verify it was added to the list
	requests, err := mock.GetPaymentRequests()
	if err != nil {
		t.Fatalf("GetPaymentRequests failed: %v", err)
	}

	if len(requests) != 1 {
		t.Errorf("expected 1 request, got %d", len(requests))
	}
}

func TestRemovePaymentRequest(t *testing.T) {
	mock := NewMockCoreClient()
	ctx := context.Background()

	if err := mock.Start(ctx); err != nil {
		t.Fatalf("failed to start mock: %v", err)
	}
	defer mock.Stop()

	// Get an address and create a payment request
	addresses, _ := mock.GetReceivingAddresses()
	testAddr := addresses[0].Address

	request, _ := mock.CreatePaymentRequest(testAddr, "Test", "", 50.0)

	// Remove the payment request (both address and ID required since IDs are per-address)
	err := mock.RemovePaymentRequest(request.Address, request.ID)
	if err != nil {
		t.Fatalf("RemovePaymentRequest failed: %v", err)
	}

	// Verify it was removed
	requests, _ := mock.GetPaymentRequests()
	if len(requests) != 0 {
		t.Errorf("expected 0 requests after removal, got %d", len(requests))
	}

	// Try to remove non-existent request
	err = mock.RemovePaymentRequest(testAddr, 999)
	if err == nil {
		t.Error("expected error when removing non-existent request")
	}
}

func TestGenerateTwinsURI(t *testing.T) {
	tests := []struct {
		name     string
		address  string
		amount   float64
		label    string
		message  string
		expected string
	}{
		{
			name:     "address only",
			address:  "D1234567890123456789012345678901234",
			amount:   0,
			label:    "",
			message:  "",
			expected: "twins:D1234567890123456789012345678901234",
		},
		{
			name:     "with amount",
			address:  "D1234567890123456789012345678901234",
			amount:   100.5,
			label:    "",
			message:  "",
			expected: "twins:D1234567890123456789012345678901234?amount=100.50000000",
		},
		{
			name:     "with label",
			address:  "D1234567890123456789012345678901234",
			amount:   0,
			label:    "Test Label",
			message:  "",
			expected: "twins:D1234567890123456789012345678901234?label=Test+Label",
		},
		{
			name:     "with all params",
			address:  "D1234567890123456789012345678901234",
			amount:   50.0,
			label:    "Payment",
			message:  "Thank you",
			expected: "twins:D1234567890123456789012345678901234?amount=50.00000000&label=Payment&message=Thank+you",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uri := GenerateTwinsURI(tt.address, tt.amount, tt.label, tt.message)
			if uri != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, uri)
			}
		})
	}
}

func TestCreatePaymentRequestValidation(t *testing.T) {
	mock := NewMockCoreClient()
	ctx := context.Background()

	if err := mock.Start(ctx); err != nil {
		t.Fatalf("failed to start mock: %v", err)
	}
	defer mock.Stop()

	// Test with invalid address
	_, err := mock.CreatePaymentRequest("invalid", "Test", "", 100.0)
	if err == nil {
		t.Error("expected error for invalid address")
	}

	// Test with negative amount
	addresses, _ := mock.GetReceivingAddresses()
	_, err = mock.CreatePaymentRequest(addresses[0].Address, "Test", "", -50.0)
	if err == nil {
		t.Error("expected error for negative amount")
	}

	// Test with zero amount (should succeed - means any amount)
	request, err := mock.CreatePaymentRequest(addresses[0].Address, "Test", "", 0)
	if err != nil {
		t.Errorf("zero amount should be valid: %v", err)
	}
	if request.Amount != 0 {
		t.Errorf("expected amount 0, got %f", request.Amount)
	}
}

func TestGetCurrentReceivingAddress(t *testing.T) {
	mock := NewMockCoreClient()
	ctx := context.Background()

	if err := mock.Start(ctx); err != nil {
		t.Fatalf("failed to start mock: %v", err)
	}
	defer mock.Stop()

	// Get current address
	addr, err := mock.GetCurrentReceivingAddress()
	if err != nil {
		t.Fatalf("GetCurrentReceivingAddress failed: %v", err)
	}

	// Should return the last address from initialization
	if !strings.HasPrefix(addr.Address, "D") {
		t.Errorf("address should start with 'D', got: %s", addr.Address)
	}

	// Generate a new address
	newAddr, _ := mock.GenerateReceivingAddress("New")

	// Current should now be the new address
	currentAddr, _ := mock.GetCurrentReceivingAddress()
	if currentAddr.Address != newAddr.Address {
		t.Errorf("current address should be the newly generated one")
	}
}
