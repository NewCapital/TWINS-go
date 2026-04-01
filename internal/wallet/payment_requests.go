package wallet

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// PaymentRequest represents a payment request stored in wallet.dat
// Matches the legacy C++ CPaymentRequestData structure and destdata storage pattern
type PaymentRequest struct {
	// ID is the unique sequential identifier (from "rr{id}" destdata key)
	ID int64 `json:"id"`

	// Date is when the payment request was created
	Date time.Time `json:"date"`

	// Label is an optional label for the payment request
	Label string `json:"label"`

	// Address is the TWINS receiving address
	Address string `json:"address"`

	// Message is an optional message to include in the payment request
	Message string `json:"message"`

	// Amount is the requested amount in TWINS (0 means any amount)
	Amount float64 `json:"amount"`
}

// paymentRequestPrefix is the destdata key prefix for payment requests
// Matches legacy C++ pattern: "rr" + sequential ID
const paymentRequestPrefix = "rr"

// toJSON serializes a PaymentRequest to JSON for wallet.dat storage
func (pr *PaymentRequest) toJSON() (string, error) {
	data, err := json.Marshal(pr)
	if err != nil {
		return "", fmt.Errorf("failed to serialize payment request: %w", err)
	}
	return string(data), nil
}

// paymentRequestFromJSON deserializes a PaymentRequest from JSON
func paymentRequestFromJSON(data string) (*PaymentRequest, error) {
	var pr PaymentRequest
	if err := json.Unmarshal([]byte(data), &pr); err != nil {
		return nil, fmt.Errorf("failed to deserialize payment request: %w", err)
	}
	return &pr, nil
}

// SavePaymentRequest saves a payment request to wallet.dat destdata
// The request is stored under key "rr{id}" for the given address
func (w *Wallet) SavePaymentRequest(pr *PaymentRequest) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.wdb == nil {
		return fmt.Errorf("wallet database not initialized")
	}

	// Serialize to JSON
	jsonData, err := pr.toJSON()
	if err != nil {
		return err
	}

	// Build destdata key: "rr{id}"
	dataKey := fmt.Sprintf("%s%d", paymentRequestPrefix, pr.ID)

	// Write to wallet.dat
	if err := w.wdb.WriteDestData(pr.Address, dataKey, jsonData); err != nil {
		return fmt.Errorf("failed to save payment request: %w", err)
	}

	return nil
}

// LoadPaymentRequest loads a specific payment request from wallet.dat
func (w *Wallet) LoadPaymentRequest(address string, id int64) (*PaymentRequest, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.wdb == nil {
		return nil, fmt.Errorf("wallet database not initialized")
	}

	dataKey := fmt.Sprintf("%s%d", paymentRequestPrefix, id)
	jsonData, err := w.wdb.ReadDestData(address, dataKey)
	if err != nil {
		return nil, fmt.Errorf("payment request not found: %w", err)
	}

	return paymentRequestFromJSON(jsonData)
}

// DeletePaymentRequest removes a payment request from wallet.dat
func (w *Wallet) DeletePaymentRequest(address string, id int64) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.wdb == nil {
		return fmt.Errorf("wallet database not initialized")
	}

	dataKey := fmt.Sprintf("%s%d", paymentRequestPrefix, id)
	if err := w.wdb.EraseDestData(address, dataKey); err != nil {
		return fmt.Errorf("failed to delete payment request: %w", err)
	}

	return nil
}

// GetPaymentRequestsForAddress returns all payment requests for a specific address
func (w *Wallet) GetPaymentRequestsForAddress(address string) ([]*PaymentRequest, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.wdb == nil {
		return nil, fmt.Errorf("wallet database not initialized")
	}

	// Get all destdata for this address
	destdata, err := w.wdb.GetAllDestData(address)
	if err != nil {
		return nil, fmt.Errorf("failed to get destdata: %w", err)
	}

	var requests []*PaymentRequest
	for dataKey, jsonData := range destdata {
		// Only process payment request keys (starting with "rr")
		if !strings.HasPrefix(dataKey, paymentRequestPrefix) {
			continue
		}

		pr, err := paymentRequestFromJSON(jsonData)
		if err != nil {
			// Log and skip corrupted entries
			continue
		}
		requests = append(requests, pr)
	}

	return requests, nil
}

// GetAllPaymentRequests returns all payment requests across all addresses
func (w *Wallet) GetAllPaymentRequests() ([]*PaymentRequest, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.wdb == nil {
		return nil, fmt.Errorf("wallet database not initialized")
	}

	// Get all receiving addresses from the address manager (including keypool)
	// Need to scan all addresses since payment requests can be on any address
	addresses := w.addrMgr.GetAllReceivingAddresses()

	var allRequests []*PaymentRequest
	for _, addr := range addresses {
		// Get all destdata for this address
		destdata, err := w.wdb.GetAllDestData(addr.Address)
		if err != nil {
			// No destdata for this address is normal (not all addresses have payment requests)
			w.logger.Debugf("No destdata for address %s: %v", addr.Address, err)
			continue
		}

		for dataKey, jsonData := range destdata {
			// Only process payment request keys (starting with "rr")
			if !strings.HasPrefix(dataKey, paymentRequestPrefix) {
				continue
			}

			pr, err := paymentRequestFromJSON(jsonData)
			if err != nil {
				// Log corrupted entries for debugging
				w.logger.Warnf("Corrupted payment request %s for address %s: %v", dataKey, addr.Address, err)
				continue
			}
			allRequests = append(allRequests, pr)
		}
	}

	return allRequests, nil
}

// getNextPaymentRequestID scans existing payment requests and returns the next available ID
// Assumes caller holds w.mu.Lock
func (w *Wallet) getNextPaymentRequestID(address string) (int64, error) {
	destdata, err := w.wdb.GetAllDestData(address)
	if err != nil {
		// No existing destdata, start at 0
		return 0, nil
	}

	var maxID int64 = -1
	for dataKey := range destdata {
		if !strings.HasPrefix(dataKey, paymentRequestPrefix) {
			continue
		}

		// Extract ID from "rr{id}"
		idStr := strings.TrimPrefix(dataKey, paymentRequestPrefix)
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			continue
		}

		if id > maxID {
			maxID = id
		}
	}

	return maxID + 1, nil
}

// CreatePaymentRequest creates a new payment request and saves it to wallet.dat
// Returns the created payment request with its assigned ID
func (w *Wallet) CreatePaymentRequest(address, label, message string, amount float64) (*PaymentRequest, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.wdb == nil {
		return nil, fmt.Errorf("wallet database not initialized")
	}

	// Get next available ID for this address
	nextID, err := w.getNextPaymentRequestID(address)
	if err != nil {
		return nil, fmt.Errorf("failed to get next payment request ID: %w", err)
	}

	// Create the payment request
	pr := &PaymentRequest{
		ID:      nextID,
		Date:    time.Now(),
		Label:   label,
		Address: address,
		Message: message,
		Amount:  amount,
	}

	// Serialize to JSON
	jsonData, err := pr.toJSON()
	if err != nil {
		return nil, err
	}

	// Build destdata key: "rr{id}"
	dataKey := fmt.Sprintf("%s%d", paymentRequestPrefix, pr.ID)

	// Write to wallet.dat
	if err := w.wdb.WriteDestData(address, dataKey, jsonData); err != nil {
		return nil, fmt.Errorf("failed to save payment request: %w", err)
	}

	// Also write the label to the address book if provided
	if label != "" {
		if err := w.wdb.WriteName(address, label); err != nil {
			// Non-fatal: payment request saved, just label failed
		}
	}

	return pr, nil
}
