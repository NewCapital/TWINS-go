// Package storage provides wallet persistence and migration utilities
package storage

import (
	"crypto/subtle"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/term"
)

// PasswordPromptFunc is the function type for password prompting
// This allows GUI applications to replace the default terminal prompt
type PasswordPromptFunc func(prompt string) ([]byte, error)

// DefaultPasswordPrompt is the default terminal-based password prompt
// It masks input and returns the password as a byte slice
var DefaultPasswordPrompt PasswordPromptFunc = SecurePasswordPrompt

// SecurePasswordPrompt safely prompts for a password without echoing to terminal
// This is the default implementation for CLI usage
func SecurePasswordPrompt(prompt string) ([]byte, error) {
	fmt.Fprint(os.Stderr, prompt)

	// Read password without echoing
	password, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return nil, fmt.Errorf("failed to read password: %w", err)
	}

	fmt.Fprintln(os.Stderr) // New line after password input
	return password, nil
}

// SetPasswordPrompt allows replacing the password prompt function
// GUI applications should call this to provide their own password dialog
func SetPasswordPrompt(promptFunc PasswordPromptFunc) {
	DefaultPasswordPrompt = promptFunc
}

// ClearPassword securely clears a password from memory
// Should be called with defer immediately after obtaining password
func ClearPassword(password []byte) {
	for i := range password {
		password[i] = 0
	}
}

// PromptForPassword prompts the user for a password using the configured prompt function
// It includes an optional confirmation step for additional safety
func PromptForPassword(prompt string, confirm bool) ([]byte, error) {
	// First prompt
	password, err := DefaultPasswordPrompt(prompt)
	if err != nil {
		return nil, err
	}

	if !confirm {
		return password, nil
	}

	// Confirmation prompt
	confirmPassword, err := DefaultPasswordPrompt("Confirm passphrase: ")
	if err != nil {
		ClearPassword(password)
		return nil, fmt.Errorf("failed to read confirmation: %w", err)
	}
	defer ClearPassword(confirmPassword)

	// Check if passwords match using constant-time comparison (prevents timing attacks)
	if subtle.ConstantTimeCompare(password, confirmPassword) != 1 {
		ClearPassword(password)
		return nil, fmt.Errorf("passphrases do not match")
	}

	return password, nil
}