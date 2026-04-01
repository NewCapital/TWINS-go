package mocks

import (
	"fmt"
	"net/url"
	"time"
	"github.com/twins-dev/twins-core/internal/gui/core"
)

// Receive page mock implementations for MockCoreClient

// initializeReceivingAddresses creates initial receiving addresses for testing
func (m *MockCoreClient) initializeReceivingAddresses() {
	labels := []string{"Main Wallet", "Savings", "Trading", "Donations", ""}

	// Create 5 initial receiving addresses
	for i := 0; i < 5; i++ {
		addr := m.generateAddress()
		pubkey := m.generatePubKey()

		// Track in ownAddresses for validation
		m.ownAddresses[addr] = pubkey

		// Create receiving address entry
		receivingAddr := core.ReceivingAddress{
			Address: addr,
			Label:   labels[i],
			Created: time.Now().Add(-time.Duration(i*24) * time.Hour), // Spread creation times
		}

		m.receivingAddresses = append(m.receivingAddresses, receivingAddr)
	}
}

// GetReceivingAddresses returns all receiving addresses for the wallet
func (m *MockCoreClient) GetReceivingAddresses() ([]core.ReceivingAddress, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return nil, fmt.Errorf("core is not running")
	}

	// Return a copy to prevent external modification
	result := make([]core.ReceivingAddress, len(m.receivingAddresses))
	copy(result, m.receivingAddresses)

	return result, nil
}

// GenerateReceivingAddress generates a new receiving address with optional label
func (m *MockCoreClient) GenerateReceivingAddress(label string) (core.ReceivingAddress, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return core.ReceivingAddress{}, fmt.Errorf("core is not running")
	}

	return m.generateReceivingAddressLocked(label), nil
}

// generateReceivingAddressLocked generates a new receiving address while lock is held
// Caller must hold the write lock (m.mu.Lock())
func (m *MockCoreClient) generateReceivingAddressLocked(label string) core.ReceivingAddress {
	// Generate new address with proper checksum
	addr := m.generateAddress()
	pubkey := m.generatePubKey()

	// Track in ownAddresses for validation
	m.ownAddresses[addr] = pubkey

	// Also add to addresses slice for backward compatibility
	m.addresses = append(m.addresses, addr)

	// Create receiving address entry
	receivingAddr := core.ReceivingAddress{
		Address: addr,
		Label:   label,
		Created: time.Now(),
	}

	m.receivingAddresses = append(m.receivingAddresses, receivingAddr)

	// Emit event
	m.emitEventLocked(core.NewAddressGeneratedEvent{
		BaseEvent: core.BaseEvent{Type: "new_address_generated", Time: time.Now()},
		Address:   addr,
		Label:     label,
	})

	return receivingAddr
}

// GetPaymentRequests returns all payment requests
func (m *MockCoreClient) GetPaymentRequests() ([]core.PaymentRequest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return nil, fmt.Errorf("core is not running")
	}

	// Return a copy to prevent external modification
	result := make([]core.PaymentRequest, len(m.paymentRequests))
	copy(result, m.paymentRequests)

	return result, nil
}

// CreatePaymentRequest creates a new payment request
func (m *MockCoreClient) CreatePaymentRequest(address, label, message string, amount float64) (core.PaymentRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return core.PaymentRequest{}, fmt.Errorf("core is not running")
	}

	// Validate address
	validation, err := m.validateAddressLocked(address)
	if err != nil {
		return core.PaymentRequest{}, fmt.Errorf("address validation failed: %w", err)
	}
	if !validation.IsValid {
		return core.PaymentRequest{}, fmt.Errorf("invalid address")
	}

	// Validate amount (0 means any amount, negative not allowed)
	if amount < 0 {
		return core.PaymentRequest{}, fmt.Errorf("amount cannot be negative")
	}

	// Create payment request
	request := core.PaymentRequest{
		ID:      m.nextPaymentRequestID,
		Date:    time.Now(),
		Label:   label,
		Address: address,
		Message: message,
		Amount:  amount,
	}

	m.nextPaymentRequestID++
	m.paymentRequests = append(m.paymentRequests, request)

	// Emit event
	m.emitEventLocked(core.PaymentRequestCreatedEvent{
		BaseEvent: core.BaseEvent{Type: "payment_request_created", Time: time.Now()},
		ID:        request.ID,
		Address:   request.Address,
		Amount:    request.Amount,
		Label:     request.Label,
		Message:   request.Message,
	})

	return request, nil
}

// RemovePaymentRequest removes a payment request by address and ID
// Note: Payment request IDs are per-address, not global, so both are needed
func (m *MockCoreClient) RemovePaymentRequest(address string, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("core is not running")
	}

	// Find and remove the payment request (matching both address and ID)
	for i, req := range m.paymentRequests {
		if req.Address == address && req.ID == id {
			m.paymentRequests = append(m.paymentRequests[:i], m.paymentRequests[i+1:]...)

			// Emit event
			m.emitEventLocked(core.PaymentRequestRemovedEvent{
				BaseEvent: core.BaseEvent{Type: "payment_request_removed", Time: time.Now()},
				ID:        id,
			})

			return nil
		}
	}

	return fmt.Errorf("payment request not found: address=%s id=%d", address, id)
}

// GenerateTwinsURI generates a twins: URI for payment requests
// Format: twins:ADDRESS?amount=VALUE&label=LABEL&message=MESSAGE
func GenerateTwinsURI(address string, amount float64, label, message string) string {
	// Start with base URI
	uri := "twins:" + address

	// Build query parameters
	params := url.Values{}

	if amount > 0 {
		params.Set("amount", fmt.Sprintf("%.8f", amount))
	}

	if label != "" {
		params.Set("label", label)
	}

	if message != "" {
		params.Set("message", message)
	}

	// Append parameters if any exist
	if len(params) > 0 {
		uri += "?" + params.Encode()
	}

	return uri
}

// GetCurrentReceivingAddress returns the most recent receiving address
// or generates a new one if none exist
func (m *MockCoreClient) GetCurrentReceivingAddress() (core.ReceivingAddress, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return core.ReceivingAddress{}, fmt.Errorf("core is not running")
	}

	// If we have addresses, return the most recent one
	if len(m.receivingAddresses) > 0 {
		return m.receivingAddresses[len(m.receivingAddresses)-1], nil
	}

	// Otherwise generate a new one using the locked version
	return m.generateReceivingAddressLocked(""), nil
}

// SetAddressLabel sets or updates the label for an address
func (m *MockCoreClient) SetAddressLabel(address string, label string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("core is not running")
	}

	// Find the address in receiving addresses and update its label
	for i, addr := range m.receivingAddresses {
		if addr.Address == address {
			m.receivingAddresses[i].Label = label
			return nil
		}
	}

	// Address not found - this is valid, we can set labels for any address
	// (e.g., addresses from transactions)
	return nil
}

// GetAddressLabel returns the label for an address
func (m *MockCoreClient) GetAddressLabel(address string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return "", fmt.Errorf("core is not running")
	}

	// Find the address in receiving addresses
	for _, addr := range m.receivingAddresses {
		if addr.Address == address {
			return addr.Label, nil
		}
	}

	// Address not found - return empty string (no label)
	return "", nil
}
