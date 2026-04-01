package main

import (
	"fmt"

	"github.com/twins-dev/twins-core/pkg/crypto"
)

// ==========================================
// Sign / Verify Message Methods
// ==========================================

// SignMessage signs a message with the private key of the given address.
// Returns a base64-encoded compact signature (65 bytes).
// The wallet must be unlocked if encrypted.
func (a *App) SignMessage(address, message string) (string, error) {
	if address == "" {
		return "", fmt.Errorf("address is required")
	}
	if message == "" {
		return "", fmt.Errorf("message is required")
	}

	// Validate address format
	if err := crypto.ValidateAddress(address); err != nil {
		return "", fmt.Errorf("invalid address: %w", err)
	}

	// Get wallet
	a.componentsMu.RLock()
	w := a.wallet
	a.componentsMu.RUnlock()

	if w == nil {
		return "", fmt.Errorf("wallet not available")
	}

	// Match legacy C++ GUI behavior: staking-only unlock is not sufficient for signing.
	// The C++ signverifymessagedialog.cpp calls requestUnlock() which checks for
	// UnlockedForStaking and prompts for full unlock before allowing message signing.
	if w.IsUnlockedForStakingOnly() {
		return "", fmt.Errorf("wallet is unlocked for staking only; please fully unlock the wallet to sign messages")
	}

	// Delegate to wallet.SignMessage which handles:
	// - Lock check
	// - Address lookup
	// - Private key access
	// - crypto.SignCompact + Base64Encode
	return w.SignMessage(address, message)
}

// VerifyMessage verifies a message signature against an address.
// The signature should be base64-encoded compact signature (65 bytes).
// No wallet unlock is required — verification uses public key recovery only.
func (a *App) VerifyMessage(address, signature, message string) (bool, error) {
	if address == "" {
		return false, fmt.Errorf("address is required")
	}
	if signature == "" {
		return false, fmt.Errorf("signature is required")
	}
	if message == "" {
		return false, fmt.Errorf("message is required")
	}

	// Validate address format
	if err := crypto.ValidateAddress(address); err != nil {
		return false, fmt.Errorf("invalid address: %w", err)
	}

	// Get wallet for network-aware verification
	a.componentsMu.RLock()
	w := a.wallet
	a.componentsMu.RUnlock()

	if w == nil {
		return false, fmt.Errorf("wallet not available")
	}

	// Use wallet.VerifyMessage which handles network-aware address derivation
	// (derives the correct address prefix based on mainnet/testnet/regtest config)
	return w.VerifyMessage(address, signature, message)
}
