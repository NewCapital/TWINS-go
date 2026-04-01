// Package core provides the interface and types for the TWINS blockchain core.
// This package defines the contract between the GUI and the blockchain implementation,
// allowing for easy swapping between mock and real implementations.
package core

import (
	"errors"
	"strings"
)

// Lifecycle errors
var (
	// ErrNotRunning is returned when an operation is attempted on a stopped core
	ErrNotRunning = errors.New("core not running")

	// ErrAlreadyRunning is returned when attempting to start an already running core
	ErrAlreadyRunning = errors.New("core already running")
)

// Wallet errors
var (
	// ErrWalletLocked is returned when a wallet operation requires an unlocked wallet
	ErrWalletLocked = errors.New("wallet is locked")

	// ErrWalletNotEncrypted is returned when attempting to unlock a non-encrypted wallet
	ErrWalletNotEncrypted = errors.New("wallet is not encrypted")

	// ErrWalletAlreadyEncrypted is returned when attempting to encrypt an already encrypted wallet
	ErrWalletAlreadyEncrypted = errors.New("wallet already encrypted")

	// ErrInsufficientFunds is returned when attempting to send more than available balance
	ErrInsufficientFunds = errors.New("insufficient funds")

	// ErrInvalidPassphrase is returned when an incorrect passphrase is provided
	ErrInvalidPassphrase = errors.New("invalid passphrase")
)

// Transaction errors
var (
	// ErrTransactionNotFound is returned when a requested transaction doesn't exist
	ErrTransactionNotFound = errors.New("transaction not found")

	// ErrInvalidTransactionID is returned when a transaction ID is malformed
	ErrInvalidTransactionID = errors.New("invalid transaction id")
)

// Address errors
var (
	// ErrInvalidAddress is returned when an address is malformed or invalid
	ErrInvalidAddress = errors.New("invalid address")

	// ErrAddressNotFound is returned when a requested address doesn't exist
	ErrAddressNotFound = errors.New("address not found")
)

// Blockchain errors
var (
	// ErrBlockNotFound is returned when a requested block doesn't exist
	ErrBlockNotFound = errors.New("block not found")

	// ErrInvalidBlockHash is returned when a block hash is malformed
	ErrInvalidBlockHash = errors.New("invalid block hash")
)

// Masternode errors
var (
	// ErrMasternodeNotFound is returned when a requested masternode doesn't exist
	ErrMasternodeNotFound = errors.New("masternode not found")

	// ErrMasternodeAlreadyStarted is returned when attempting to start an already active masternode
	ErrMasternodeAlreadyStarted = errors.New("masternode already started")

	// ErrInsufficientCollateral is returned when masternode collateral is insufficient
	ErrInsufficientCollateral = errors.New("insufficient masternode collateral")
)

// Network errors
var (
	// ErrPeerNotFound is returned when a requested peer doesn't exist
	ErrPeerNotFound = errors.New("peer not found")

	// ErrNetworkNotActive is returned when the network is not active
	ErrNetworkNotActive = errors.New("network not active")
)

// SendErrorCode represents specific error types for transaction sending
type SendErrorCode string

const (
	// Wallet state errors
	SendErrWalletLocked       SendErrorCode = "WALLET_LOCKED"
	SendErrWalletStakingOnly  SendErrorCode = "WALLET_STAKING_ONLY"
	SendErrWalletNotReady     SendErrorCode = "WALLET_NOT_READY"

	// Input validation errors
	SendErrInvalidAddress     SendErrorCode = "INVALID_ADDRESS"
	SendErrInvalidAmount      SendErrorCode = "INVALID_AMOUNT"
	SendErrAmountBelowDust    SendErrorCode = "AMOUNT_BELOW_DUST"
	SendErrNoRecipients       SendErrorCode = "NO_RECIPIENTS"

	// Fund errors
	SendErrInsufficientFunds  SendErrorCode = "INSUFFICIENT_FUNDS"
	SendErrNoUTXOs            SendErrorCode = "NO_UTXOS"
	SendErrUTXONotFound       SendErrorCode = "UTXO_NOT_FOUND"
	SendErrUTXONotSpendable   SendErrorCode = "UTXO_NOT_SPENDABLE"
	SendErrUTXOInsufficient   SendErrorCode = "UTXO_INSUFFICIENT"

	// Fee errors
	SendErrFeeExceedsMax      SendErrorCode = "FEE_EXCEEDS_MAX"
	SendErrFeeUnreasonable    SendErrorCode = "FEE_UNREASONABLE"
	SendErrInsufficientForFee SendErrorCode = "INSUFFICIENT_FOR_FEE"

	// Transaction errors
	SendErrBuildFailed        SendErrorCode = "BUILD_FAILED"
	SendErrSignFailed         SendErrorCode = "SIGN_FAILED"
	SendErrBroadcastFailed    SendErrorCode = "BROADCAST_FAILED"
	SendErrInvalidChangeAddr  SendErrorCode = "INVALID_CHANGE_ADDRESS"

	// System errors
	SendErrNotInitialized     SendErrorCode = "NOT_INITIALIZED"
	SendErrInternal           SendErrorCode = "INTERNAL_ERROR"
)

// SendError represents a structured error for transaction sending operations
// with an error code that the GUI can use for localized/user-friendly messages
type SendError struct {
	Code    SendErrorCode `json:"code"`
	Message string        `json:"message"`
	Details string        `json:"details,omitempty"` // Additional technical details
}

// Error implements the error interface
func (e *SendError) Error() string {
	if e.Details != "" {
		return e.Message + ": " + e.Details
	}
	return e.Message
}

// NewSendError creates a new SendError with the given code and message
func NewSendError(code SendErrorCode, message string) *SendError {
	return &SendError{
		Code:    code,
		Message: message,
	}
}

// NewSendErrorWithDetails creates a new SendError with additional details
func NewSendErrorWithDetails(code SendErrorCode, message, details string) *SendError {
	return &SendError{
		Code:    code,
		Message: message,
		Details: details,
	}
}

// TranslateSendError converts a wallet/internal error to a GUI-friendly SendError
func TranslateSendError(err error) *SendError {
	if err == nil {
		return nil
	}

	errStr := err.Error()

	// Wallet state errors
	if contains(errStr, "wallet is locked") {
		if contains(errStr, "staking only") {
			return NewSendError(SendErrWalletStakingOnly,
				"Wallet is unlocked for staking only. Please fully unlock your wallet to send transactions.")
		}
		return NewSendError(SendErrWalletLocked,
			"Wallet is locked. Please unlock your wallet to send transactions.")
	}

	// System not ready errors
	if contains(errStr, "not initialized") || contains(errStr, "not set") {
		return NewSendError(SendErrWalletNotReady,
			"Wallet is not ready. Please wait for initialization to complete.")
	}

	// Address validation errors
	if contains(errStr, "invalid address") {
		return NewSendErrorWithDetails(SendErrInvalidAddress,
			"Invalid TWINS address. Please check the address and try again.", errStr)
	}
	if contains(errStr, "invalid change address") || contains(errStr, "invalid custom change") {
		return NewSendErrorWithDetails(SendErrInvalidChangeAddr,
			"Invalid change address. Please check the address format.", errStr)
	}

	// Amount validation errors
	if contains(errStr, "invalid amount") || contains(errStr, "amount must be positive") {
		return NewSendError(SendErrInvalidAmount,
			"Invalid amount. Please enter a positive amount.")
	}
	if contains(errStr, "dust threshold") || contains(errStr, "below dust") {
		return NewSendError(SendErrAmountBelowDust,
			"Amount is too small. The minimum amount is 0.00000546 TWINS.")
	}
	if contains(errStr, "no recipients") {
		return NewSendError(SendErrNoRecipients,
			"No recipients specified. Please add at least one recipient.")
	}

	// Fund availability errors
	if contains(errStr, "insufficient funds") {
		if contains(errStr, "for fee") {
			return NewSendErrorWithDetails(SendErrInsufficientForFee,
				"Insufficient funds to cover the transaction fee.", errStr)
		}
		return NewSendErrorWithDetails(SendErrInsufficientFunds,
			"Insufficient funds. Your balance is too low for this transaction.", errStr)
	}
	if contains(errStr, "no spendable UTXOs") {
		return NewSendError(SendErrNoUTXOs,
			"No spendable funds available. You may need to wait for confirmations.")
	}
	if contains(errStr, "UTXO not found") {
		return NewSendErrorWithDetails(SendErrUTXONotFound,
			"Selected coin not found. It may have been spent already.", errStr)
	}
	if contains(errStr, "UTXO not spendable") {
		return NewSendError(SendErrUTXONotSpendable,
			"Selected coin is not spendable. It may be locked or require more confirmations.")
	}
	if contains(errStr, "UTXOs insufficient") || contains(errStr, "selected UTXOs insufficient") {
		return NewSendErrorWithDetails(SendErrUTXOInsufficient,
			"Selected coins are insufficient for this transaction.", errStr)
	}

	// Fee errors
	if contains(errStr, "fee") && contains(errStr, "exceeds") {
		if contains(errStr, "maximum reasonable") || contains(errStr, "10%") {
			return NewSendErrorWithDetails(SendErrFeeUnreasonable,
				"Transaction fee is unusually high (more than 10% of amount). This may indicate a problem.", errStr)
		}
		return NewSendErrorWithDetails(SendErrFeeExceedsMax,
			"Transaction fee exceeds the maximum allowed.", errStr)
	}

	// Transaction building errors
	if contains(errStr, "failed to build") || contains(errStr, "build transaction") {
		return NewSendErrorWithDetails(SendErrBuildFailed,
			"Failed to create transaction. Please try again.", errStr)
	}
	if contains(errStr, "failed to sign") || contains(errStr, "sign transaction") ||
		contains(errStr, "private key not available") {
		return NewSendErrorWithDetails(SendErrSignFailed,
			"Failed to sign transaction. The required keys may not be available.", errStr)
	}
	if contains(errStr, "already in mempool") {
		return NewSendErrorWithDetails(SendErrBroadcastFailed,
			"Transaction already pending. Please wait for the previous transaction to confirm.", errStr)
	}
	if contains(errStr, "failed to broadcast") || contains(errStr, "broadcast transaction") {
		return NewSendErrorWithDetails(SendErrBroadcastFailed,
			"Failed to broadcast transaction. Please check your network connection.", errStr)
	}

	// Generic fallback
	return NewSendErrorWithDetails(SendErrInternal,
		"Transaction failed. Please try again.", errStr)
}

// contains is a helper for case-insensitive string matching
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
