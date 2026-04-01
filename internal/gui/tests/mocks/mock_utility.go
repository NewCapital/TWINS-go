package mocks

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// Utility operation implementations for MockCoreClient

// SignMessage implements CoreClient.SignMessage
func (m *MockCoreClient) SignMessage(address string, message string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return "", fmt.Errorf("core is not running")
	}

	if m.locked {
		return "", fmt.Errorf("wallet is locked")
	}

	// Check if this is one of our addresses
	isMine := false
	for _, addr := range m.addresses {
		if addr == address {
			isMine = true
			break
		}
	}

	if !isMine {
		return "", fmt.Errorf("address not found in wallet: %s", address)
	}

	// Generate a mock signature
	// In a real implementation, this would use the private key
	data := []byte(fmt.Sprintf("%s:%s:mock_signature", address, message))
	hash := sha256.Sum256(data)
	signature := base64.StdEncoding.EncodeToString(hash[:])

	return signature, nil
}

// VerifyMessage implements CoreClient.VerifyMessage
func (m *MockCoreClient) VerifyMessage(address string, signature string, message string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return false, fmt.Errorf("core is not running")
	}

	// Validate address format
	if len(address) < 20 || address[0] != 'D' {
		return false, fmt.Errorf("invalid address")
	}

	// Mock verification: check if signature matches our mock format
	// In a real implementation, this would verify the cryptographic signature
	data := []byte(fmt.Sprintf("%s:%s:mock_signature", address, message))
	hash := sha256.Sum256(data)
	expectedSignature := base64.StdEncoding.EncodeToString(hash[:])

	return signature == expectedSignature, nil
}
