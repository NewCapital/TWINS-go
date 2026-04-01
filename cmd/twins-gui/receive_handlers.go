package main

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/twins-dev/twins-core/internal/gui/core"
	"github.com/twins-dev/twins-core/internal/gui/tests/mocks"
	"github.com/twins-dev/twins-core/internal/wallet"
)

// ==========================================
// Receive Page Methods
// ==========================================

// GetReceivingAddresses returns all receiving addresses for the wallet
func (a *App) GetReceivingAddresses() ([]core.ReceivingAddress, error) {
	// Try real wallet first
	a.componentsMu.RLock()
	w := a.wallet
	a.componentsMu.RUnlock()

	if w != nil {
		addresses := w.GetReceivingAddresses()
		result := make([]core.ReceivingAddress, 0, len(addresses))
		for _, addr := range addresses {
			result = append(result, walletAddressToCoreAddress(addr, w))
		}
		return result, nil
	}

	// Fall back to mock for development mode
	if a.coreClient == nil {
		return nil, fmt.Errorf("core client not initialized")
	}
	if mockClient, ok := a.coreClient.(*mocks.MockCoreClient); ok {
		return mockClient.GetReceivingAddresses()
	}

	return nil, fmt.Errorf("wallet not available")
}

// GenerateReceivingAddress generates a new receiving address with optional label
func (a *App) GenerateReceivingAddress(label string) (*core.ReceivingAddress, error) {
	// Try real wallet first
	a.componentsMu.RLock()
	w := a.wallet
	a.componentsMu.RUnlock()

	if w != nil {
		address, err := w.GetReceivingAddress(label)
		if err != nil {
			return nil, fmt.Errorf("failed to generate receiving address: %w", err)
		}

		// Get the full address info to get creation time
		addresses := w.GetReceivingAddresses()
		for _, addr := range addresses {
			if addr.Address == address {
				result := walletAddressToCoreAddress(addr, w)
				return &result, nil
			}
		}

		// Address was just created, return with current time
		result := core.ReceivingAddress{
			Address: address,
			Label:   label,
		}
		return &result, nil
	}

	// Fall back to mock for development mode
	if a.coreClient == nil {
		return nil, fmt.Errorf("core client not initialized")
	}
	if mockClient, ok := a.coreClient.(*mocks.MockCoreClient); ok {
		addr, err := mockClient.GenerateReceivingAddress(label)
		if err != nil {
			return nil, fmt.Errorf("failed to generate receiving address: %w", err)
		}
		return &addr, nil
	}

	return nil, fmt.Errorf("wallet not available")
}

// GetPaymentRequests returns all payment requests
func (a *App) GetPaymentRequests() ([]core.PaymentRequest, error) {
	// Try real wallet first
	a.componentsMu.RLock()
	w := a.wallet
	a.componentsMu.RUnlock()

	if w != nil {
		requests, err := w.GetAllPaymentRequests()
		if err != nil {
			return nil, fmt.Errorf("failed to get payment requests: %w", err)
		}

		result := make([]core.PaymentRequest, 0, len(requests))
		for _, pr := range requests {
			result = append(result, walletPaymentRequestToCoreRequest(pr))
		}
		return result, nil
	}

	// Fall back to mock for development mode
	if a.coreClient == nil {
		return nil, fmt.Errorf("core client not initialized")
	}
	if mockClient, ok := a.coreClient.(*mocks.MockCoreClient); ok {
		return mockClient.GetPaymentRequests()
	}

	return nil, fmt.Errorf("wallet not available")
}

// CreatePaymentRequest creates a new payment request
func (a *App) CreatePaymentRequest(address, label, message string, amount float64) (*core.PaymentRequest, error) {
	// Try real wallet first
	a.componentsMu.RLock()
	w := a.wallet
	a.componentsMu.RUnlock()

	if w != nil {
		pr, err := w.CreatePaymentRequest(address, label, message, amount)
		if err != nil {
			return nil, fmt.Errorf("failed to create payment request: %w", err)
		}

		result := walletPaymentRequestToCoreRequest(pr)
		return &result, nil
	}

	// Fall back to mock for development mode
	if a.coreClient == nil {
		return nil, fmt.Errorf("core client not initialized")
	}
	if mockClient, ok := a.coreClient.(*mocks.MockCoreClient); ok {
		request, err := mockClient.CreatePaymentRequest(address, label, message, amount)
		if err != nil {
			return nil, fmt.Errorf("failed to create payment request: %w", err)
		}
		return &request, nil
	}

	return nil, fmt.Errorf("wallet not available")
}

// RemovePaymentRequest removes a payment request by address and ID
// Note: Payment request IDs are per-address, not global, so both are needed
func (a *App) RemovePaymentRequest(address string, id int64) error {
	// Try real wallet first
	a.componentsMu.RLock()
	w := a.wallet
	a.componentsMu.RUnlock()

	if w != nil {
		return w.DeletePaymentRequest(address, id)
	}

	// Fall back to mock for development mode
	if a.coreClient == nil {
		return fmt.Errorf("core client not initialized")
	}
	if mockClient, ok := a.coreClient.(*mocks.MockCoreClient); ok {
		return mockClient.RemovePaymentRequest(address, id)
	}

	return fmt.Errorf("wallet not available")
}

// GenerateTwinsURI generates a twins: URI for payment requests
// Format: twins:ADDRESS?amount=VALUE&label=LABEL&message=MESSAGE
func (a *App) GenerateTwinsURI(address string, amount float64, label, message string) string {
	var params []string

	if amount > 0 {
		params = append(params, fmt.Sprintf("amount=%s", strconv.FormatFloat(amount, 'f', -1, 64)))
	}
	if label != "" {
		params = append(params, fmt.Sprintf("label=%s", url.QueryEscape(label)))
	}
	if message != "" {
		params = append(params, fmt.Sprintf("message=%s", url.QueryEscape(message)))
	}

	uri := "twins:" + address
	if len(params) > 0 {
		uri += "?" + strings.Join(params, "&")
	}
	return uri
}

// SetAddressLabel sets or updates the label for an address
// This is used by the Edit Label feature in the transactions context menu
func (a *App) SetAddressLabel(address string, label string) error {
	// Validate address is not empty (defensive check)
	if address == "" {
		return fmt.Errorf("cannot set label for empty address")
	}

	// Try real wallet first
	a.componentsMu.RLock()
	w := a.wallet
	a.componentsMu.RUnlock()

	if w != nil {
		if err := w.SetAddressLabel(address, label); err != nil {
			return fmt.Errorf("failed to set address label: %w", err)
		}
		return nil
	}

	// Fall back to mock for development mode
	if a.coreClient == nil {
		return fmt.Errorf("core client not initialized")
	}
	if mockClient, ok := a.coreClient.(*mocks.MockCoreClient); ok {
		return mockClient.SetAddressLabel(address, label)
	}

	return fmt.Errorf("wallet not available")
}

// GetAddressLabel returns the label for an address
func (a *App) GetAddressLabel(address string) (string, error) {
	// Try real wallet first
	a.componentsMu.RLock()
	w := a.wallet
	a.componentsMu.RUnlock()

	if w != nil {
		return w.GetAddressLabel(address), nil
	}

	// Fall back to mock for development mode
	if a.coreClient == nil {
		return "", fmt.Errorf("core client not initialized")
	}
	if mockClient, ok := a.coreClient.(*mocks.MockCoreClient); ok {
		return mockClient.GetAddressLabel(address)
	}

	return "", fmt.Errorf("wallet not available")
}

// GetCurrentReceivingAddress returns the most recent receiving address
func (a *App) GetCurrentReceivingAddress() (*core.ReceivingAddress, error) {
	// Try real wallet first
	a.componentsMu.RLock()
	w := a.wallet
	a.componentsMu.RUnlock()

	if w != nil {
		// Use GetAllReceivingAddresses to include keypool addresses
		// (GetReceivingAddresses only returns labeled/used ones)
		addresses := w.GetAllReceivingAddresses()
		if len(addresses) == 0 {
			// Generate a new address if none exist
			address, err := w.GetReceivingAddress("")
			if err != nil {
				return nil, fmt.Errorf("failed to get receiving address: %w", err)
			}
			result := core.ReceivingAddress{
				Address: address,
				Label:   "",
			}
			return &result, nil
		}

		// Return the most recently created address from the full pool
		var newest *wallet.Address
		for _, addr := range addresses {
			if newest == nil || addr.CreatedAt.After(newest.CreatedAt) {
				newest = addr
			}
		}

		if newest != nil {
			result := walletAddressToCoreAddress(newest, w)
			return &result, nil
		}
	}

	// Fall back to mock for development mode
	if a.coreClient == nil {
		return nil, fmt.Errorf("core client not initialized")
	}
	if mockClient, ok := a.coreClient.(*mocks.MockCoreClient); ok {
		addr, err := mockClient.GetCurrentReceivingAddress()
		if err != nil {
			return nil, fmt.Errorf("failed to get current receiving address: %w", err)
		}
		return &addr, nil
	}

	return nil, fmt.Errorf("wallet not available")
}

// ==========================================
// Type Conversion Helpers
// ==========================================

// walletAddressToCoreAddress converts a wallet.Address to core.ReceivingAddress
func walletAddressToCoreAddress(addr *wallet.Address, w *wallet.Wallet) core.ReceivingAddress {
	label := addr.Label
	// If no label on address, try to get it from the address book
	if label == "" && w != nil {
		label = w.GetAddressLabel(addr.Address)
	}

	return core.ReceivingAddress{
		Address: addr.Address,
		Label:   label,
		Created: addr.CreatedAt,
	}
}

// walletPaymentRequestToCoreRequest converts a wallet.PaymentRequest to core.PaymentRequest
func walletPaymentRequestToCoreRequest(pr *wallet.PaymentRequest) core.PaymentRequest {
	return core.PaymentRequest{
		ID:      pr.ID,
		Date:    pr.Date,
		Label:   pr.Label,
		Address: pr.Address,
		Message: pr.Message,
		Amount:  pr.Amount,
	}
}
