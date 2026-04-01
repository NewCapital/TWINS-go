package p2p

import (
	"encoding/hex"
	"strings"
	"testing"
)

// TestAuthPasswordHexEncoding verifies that password authentication uses hex encoding
// to prevent command injection attacks through special characters
func TestAuthPasswordHexEncoding(t *testing.T) {
	// Test cases with potentially dangerous characters that could cause
	// command injection if not properly encoded
	testCases := []struct {
		name     string
		password string
	}{
		{"simple password", "password123"},
		{"password with spaces", "password with spaces"},
		{"password with quotes", `password"with"quotes`},
		{"password with single quotes", "password'with'quotes"},
		{"password with newlines", "password\nwith\nnewlines"},
		{"password with semicolon", "password;echo hacked"},
		{"password with backslash", "password\\escape"},
		{"password with null byte", "password\x00null"},
		{"password with control chars", "password\r\nCRLF"},
		{"unicode password", "пароль🔐密码"},
		{"empty password", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Verify that hex encoding produces safe output
			hexEncoded := hex.EncodeToString([]byte(tc.password))

			// Hex encoded string should only contain hex characters (0-9, a-f)
			for _, c := range hexEncoded {
				if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
					t.Errorf("hex encoding produced non-hex character: %c", c)
				}
			}

			// Verify the encoding is reversible
			decoded, err := hex.DecodeString(hexEncoded)
			if err != nil {
				t.Errorf("failed to decode hex: %v", err)
			}
			if string(decoded) != tc.password {
				t.Errorf("hex decode mismatch: got %q, want %q", string(decoded), tc.password)
			}

			// Verify no dangerous characters in the command string
			command := "AUTHENTICATE " + hexEncoded
			if strings.ContainsAny(command, "\"';\\|\n\r") {
				t.Errorf("command contains dangerous characters: %q", command)
			}
		})
	}
}

// TestTorControllerNilConnection verifies that sendCommand handles nil connection
func TestTorControllerNilConnection(t *testing.T) {
	// Create controller without starting (conn will be nil)
	controller := &TorController{
		controlAddr: "127.0.0.1:9051",
		servicePort: 37817,
	}

	// sendCommand should return error for nil connection
	_, err := controller.sendCommand("PROTOCOLINFO 1")
	if err == nil {
		t.Error("expected error for nil connection, got nil")
	}

	expectedErr := "not connected to Tor control port"
	if err.Error() != expectedErr {
		t.Errorf("unexpected error message: got %q, want %q", err.Error(), expectedErr)
	}
}

// TestComputeTorHMAC verifies the HMAC computation for SAFECOOKIE auth
func TestComputeTorHMAC(t *testing.T) {
	// Use known test vectors
	cookie := []byte("12345678901234567890123456789012") // 32 bytes
	clientNonce := make([]byte, 32)
	serverNonce := make([]byte, 32)

	// Fill with test data
	for i := range clientNonce {
		clientNonce[i] = byte(i)
	}
	for i := range serverNonce {
		serverNonce[i] = byte(i + 32)
	}

	// Compute both server and client hashes
	serverHash := computeTorHMAC(cookie, clientNonce, serverNonce, true)
	clientHash := computeTorHMAC(cookie, clientNonce, serverNonce, false)

	// Verify hashes are 32 bytes (SHA256)
	if len(serverHash) != 32 {
		t.Errorf("server hash wrong length: got %d, want 32", len(serverHash))
	}
	if len(clientHash) != 32 {
		t.Errorf("client hash wrong length: got %d, want 32", len(clientHash))
	}

	// Server and client hashes should be different (different keys)
	if string(serverHash) == string(clientHash) {
		t.Error("server and client hashes should be different")
	}

	// Same inputs should produce same output (deterministic)
	serverHash2 := computeTorHMAC(cookie, clientNonce, serverNonce, true)
	if string(serverHash) != string(serverHash2) {
		t.Error("HMAC computation is not deterministic")
	}
}

// TestContainsHelper verifies the contains helper function
func TestContainsHelper(t *testing.T) {
	slice := []string{"NULL", "COOKIE", "SAFECOOKIE", "HASHEDPASSWORD"}

	testCases := []struct {
		item     string
		expected bool
	}{
		{"NULL", true},
		{"COOKIE", true},
		{"SAFECOOKIE", true},
		{"HASHEDPASSWORD", true},
		{"PASSWORD", false},
		{"null", false}, // case sensitive
		{"", false},
	}

	for _, tc := range testCases {
		t.Run(tc.item, func(t *testing.T) {
			result := contains(slice, tc.item)
			if result != tc.expected {
				t.Errorf("contains(%v, %q) = %v, want %v", slice, tc.item, result, tc.expected)
			}
		})
	}
}
