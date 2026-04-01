// Copyright (c) 2018-2025 The TWINS developers
// Distributed under the MIT/X11 software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package masternode

import (
	"crypto/subtle"
	"fmt"
	"runtime"
	"sync"

	"github.com/twins-dev/twins-core/pkg/crypto"
)

// SecureKey provides secure storage for WIF private keys with explicit zeroing.
// It wraps a WIF string and ensures sensitive data is cleared from memory
// when no longer needed.
//
// SECURITY: This struct is designed to minimize private key exposure:
// - Key data is stored in a byte slice (can be zeroed)
// - Explicit Clear() method zeros memory
// - Use with defer pattern: defer key.Clear()
// - Avoid logging or serializing the key
type SecureKey struct {
	data []byte // WIF key as bytes (zeroable)
	mu   sync.RWMutex
}

// NewSecureKey creates a SecureKey from a WIF string.
// The original string cannot be zeroed (Go strings are immutable),
// but the internal storage can be cleared via Clear().
//
// Usage:
//
//	key := NewSecureKey(wifString)
//	defer key.Clear()
//	privKey, err := key.DecodeWIF()
func NewSecureKey(wif string) *SecureKey {
	if wif == "" {
		return &SecureKey{data: nil}
	}
	// Copy to byte slice so we can zero it later
	data := make([]byte, len(wif))
	copy(data, wif)
	return &SecureKey{data: data}
}

// Clear securely zeros the key data in memory.
// Should be called when the key is no longer needed.
// Safe to call multiple times.
//
// SECURITY LIMITATIONS (Go runtime constraints):
//   - Go's compiler may optimize away zeroing operations
//   - The original WIF string (immutable) cannot be zeroed
//   - GC may create copies before zeroing completes
//   - String interning may retain copies in memory
//
// This is "best effort" defense-in-depth, not a guarantee.
// For maximum security, avoid logging/serializing keys and
// minimize the time keys remain in memory.
func (sk *SecureKey) Clear() {
	if sk == nil {
		return
	}
	sk.mu.Lock()
	defer sk.mu.Unlock()

	if sk.data != nil {
		// Zero the data byte-by-byte
		// Note: compiler may optimize this away, but we try anyway
		for i := range sk.data {
			sk.data[i] = 0
		}
		// Use subtle.ConstantTimeCompare to force a read of zeroed data,
		// making it harder for compiler to skip the zeroing
		_ = subtle.ConstantTimeCompare(sk.data, make([]byte, len(sk.data)))
		sk.data = nil

		// Hint to GC that this memory should be collected soon
		// Note: This doesn't guarantee immediate collection
		runtime.GC()
	}
}

// IsSet returns true if the key contains data.
func (sk *SecureKey) IsSet() bool {
	if sk == nil {
		return false
	}
	sk.mu.RLock()
	defer sk.mu.RUnlock()
	return sk.data != nil && len(sk.data) > 0
}

// DecodeWIF decodes the WIF key and returns the private key.
// The returned *crypto.PrivateKey should also be handled securely.
//
// IMPORTANT: The caller is responsible for zeroing the returned key
// when done using it.
func (sk *SecureKey) DecodeWIF() (*crypto.PrivateKey, error) {
	if sk == nil {
		return nil, fmt.Errorf("secure key is nil")
	}
	sk.mu.RLock()
	defer sk.mu.RUnlock()

	if sk.data == nil || len(sk.data) == 0 {
		return nil, fmt.Errorf("secure key is empty")
	}

	// Convert to string for WIF decoder (unavoidable copy)
	wifStr := string(sk.data)
	return crypto.DecodeWIF(wifStr)
}

// String returns a masked representation for logging.
// NEVER logs the actual key.
func (sk *SecureKey) String() string {
	if sk == nil || !sk.IsSet() {
		return "<unset>"
	}
	return "<secure-key-set>"
}

