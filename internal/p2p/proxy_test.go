package p2p

import (
	"errors"
	"strings"
	"testing"
)

// TestNewProxyDialerWithAuth_CredentialLength verifies RFC 1929 credential length limits
func TestNewProxyDialerWithAuth_CredentialLength(t *testing.T) {
	testCases := []struct {
		name        string
		username    string
		password    string
		expectError bool
	}{
		// Valid cases (within 255 byte limit)
		{"empty credentials", "", "", false},
		{"short username", "user", "pass", false},
		{"max username 255", strings.Repeat("a", 255), "pass", false},
		{"max password 255", "user", strings.Repeat("b", 255), false},
		{"both max 255", strings.Repeat("a", 255), strings.Repeat("b", 255), false},

		// Invalid cases (exceed 255 byte limit)
		{"username 256 bytes", strings.Repeat("a", 256), "pass", true},
		{"password 256 bytes", "user", strings.Repeat("b", 256), true},
		{"both exceed limit", strings.Repeat("a", 256), strings.Repeat("b", 256), true},
		{"username 1000 bytes", strings.Repeat("x", 1000), "pass", true},
		{"password 1000 bytes", "user", strings.Repeat("y", 1000), true},

		// Unicode credentials (byte length matters, not rune count)
		{"unicode username safe", "пользователь", "pass", false}, // 24 bytes in UTF-8
		{"unicode password safe", "user", "пароль", false},       // 12 bytes in UTF-8
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dialer, err := NewProxyDialerWithAuth("127.0.0.1:9050", tc.username, tc.password)

			if tc.expectError {
				if err == nil {
					t.Error("expected error for credentials exceeding 255 bytes, got nil")
				}
				if !errors.Is(err, ErrSOCKS5CredentialLength) {
					t.Errorf("expected ErrSOCKS5CredentialLength, got %v", err)
				}
				if dialer != nil {
					t.Error("dialer should be nil on error")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if dialer == nil {
					t.Error("dialer should not be nil on success")
				}
				if dialer != nil {
					if dialer.Username != tc.username {
						t.Errorf("username mismatch: got %q, want %q", dialer.Username, tc.username)
					}
					if dialer.Password != tc.password {
						t.Errorf("password mismatch: got %q, want %q", dialer.Password, tc.password)
					}
				}
			}
		})
	}
}

// TestNewProxyDialer_NoAuth verifies basic proxy dialer creation
func TestNewProxyDialer_NoAuth(t *testing.T) {
	proxyAddr := "127.0.0.1:9050"
	dialer := NewProxyDialer(proxyAddr)

	if dialer == nil {
		t.Fatal("dialer should not be nil")
	}
	if dialer.ProxyAddr != proxyAddr {
		t.Errorf("proxy address mismatch: got %q, want %q", dialer.ProxyAddr, proxyAddr)
	}
	if dialer.Username != "" {
		t.Errorf("username should be empty, got %q", dialer.Username)
	}
	if dialer.Password != "" {
		t.Errorf("password should be empty, got %q", dialer.Password)
	}
	if dialer.Timeout != 30*1e9 { // 30 seconds in nanoseconds
		t.Errorf("timeout mismatch: got %v, want 30s", dialer.Timeout)
	}
}

// TestSOCKS5Errors verifies error types are properly defined
func TestSOCKS5Errors(t *testing.T) {
	// Verify all error types are non-nil and have proper messages
	errors := []struct {
		err     error
		message string
	}{
		{ErrSOCKS5AuthFailed, "SOCKS5 authentication failed"},
		{ErrSOCKS5ConnectFailed, "SOCKS5 connect failed"},
		{ErrSOCKS5UnsupportedVer, "unsupported SOCKS version"},
		{ErrSOCKS5CredentialLength, "SOCKS5 username or password exceeds 255 bytes (RFC 1929 limit)"},
	}

	for _, e := range errors {
		if e.err == nil {
			t.Errorf("error %q should not be nil", e.message)
		}
		if e.err.Error() != e.message {
			t.Errorf("error message mismatch: got %q, want %q", e.err.Error(), e.message)
		}
	}
}

// TestCredentialLengthBoundary tests exact boundary conditions
func TestCredentialLengthBoundary(t *testing.T) {
	// Test exact boundaries (254, 255, 256)
	boundaries := []struct {
		length      int
		expectError bool
	}{
		{254, false},
		{255, false}, // RFC 1929 max
		{256, true},  // Exceeds limit
		{257, true},
	}

	for _, b := range boundaries {
		username := strings.Repeat("u", b.length)
		password := strings.Repeat("p", b.length)

		// Test username boundary
		t.Run("username_"+string(rune('0'+b.length%10)), func(t *testing.T) {
			_, err := NewProxyDialerWithAuth("127.0.0.1:9050", username, "pass")
			gotError := err != nil
			if gotError != b.expectError {
				t.Errorf("username length %d: got error=%v, want error=%v", b.length, gotError, b.expectError)
			}
		})

		// Test password boundary
		t.Run("password_"+string(rune('0'+b.length%10)), func(t *testing.T) {
			_, err := NewProxyDialerWithAuth("127.0.0.1:9050", "user", password)
			gotError := err != nil
			if gotError != b.expectError {
				t.Errorf("password length %d: got error=%v, want error=%v", b.length, gotError, b.expectError)
			}
		})
	}
}
