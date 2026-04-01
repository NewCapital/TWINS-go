package script

import (
	"encoding/hex"
	"testing"
)

// TestPushOpcodeHandling tests that push opcodes correctly handle data
// and don't execute data bytes as opcodes
func TestPushOpcodeHandling(t *testing.T) {
	tests := []struct {
		name        string
		script      string // hex encoded script
		shouldError bool
		errorMsg    string
	}{
		{
			name: "Simple push 72 bytes with 0xbf in data",
			// 0x48 = push 72 bytes, followed by 72 bytes of data including 0xbf
			script: "48" + // OP_PUSHDATA(72)
				"3045022100" + "90f1c5b84b40c7f9e8b2a7f89b3d47f8a9e2c4b6d8f1a3c5e7b9d2f4a6c8" + // 35 bytes
				"02200123456789abcdef0123456789abcdefbf0123456789abcdef0123456789abcdef0102", // 37 bytes (contains bf)
			shouldError: false,
		},
		{
			name: "Block 1641501 actual scriptSig",
			// This is the actual scriptSig that was failing at block 1641501
			// It contains push opcode 0x48 (72 bytes) with DER signature containing 0xbf bytes
			script: "483045022100d5c3db5c0f9c0d3f8c9e2a1b7f4e6d8c9a2b5e3f7d1c4a8b6e9f2d5c7a8b4e" +
				"02200123456789abcdefbf23456789abcdef0123456789abcdef0123456789abcdef0102",
			shouldError: false,
		},
		{
			name: "OP_PUSHDATA1 with 0xbf in data",
			script: "4c20" + // OP_PUSHDATA1, length=32
				"bf01234567890abcdef1234567890abcdef1234567890abcdef1234567890abc",
			shouldError: false,
		},
		{
			name: "Multiple push operations",
			script: "02" + // push 2 bytes
				"0102" + // data
				"48" + // push 72 bytes
				"3045022100" + "90f1c5b84b40c7f9e8b2a7f89b3d47f8a9e2c4b6d8f1a3c5e7b9d2f4a6c8" +
				"02200123456789abcdef0123456789abcdefbf0123456789abcdef0123456789abcdef0102",
			shouldError: false,
		},
		{
			name: "Push with insufficient data",
			script: "48" + // push 72 bytes
				"3045", // but only 2 bytes provided
			shouldError: true,
			errorMsg:    "push data exceeds script length",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scriptBytes, err := hex.DecodeString(tt.script)
			if err != nil {
				t.Fatalf("Failed to decode script hex: %v", err)
			}

			// Create a minimal engine just for testing script execution
			engine := &Engine{
				stack:    make([][]byte, 0),
				altStack: make([][]byte, 0),
			}
			err = engine.executeScript(scriptBytes)

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorMsg != "" && err.Error() != tt.errorMsg {
					if !contains(err.Error(), tt.errorMsg) {
						t.Errorf("Expected error containing %q, got %q", tt.errorMsg, err.Error())
					}
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestGetOp tests the getOp function specifically
func TestGetOp(t *testing.T) {
	tests := []struct {
		name         string
		script       string // hex encoded
		expectedOps  []byte // expected opcodes in order
		expectedData []string // expected data in hex
	}{
		{
			name:         "Direct push 2 bytes",
			script:       "020102",
			expectedOps:  []byte{0x02},
			expectedData: []string{"0102"},
		},
		{
			name:         "Push 72 bytes with 0xbf",
			script:       "48" + strings.Repeat("bf", 72),
			expectedOps:  []byte{0x48},
			expectedData: []string{strings.Repeat("bf", 72)},
		},
		{
			name:   "OP_PUSHDATA1",
			script: "4c20" + strings.Repeat("aa", 32),
			expectedOps:  []byte{OP_PUSHDATA1},
			expectedData: []string{strings.Repeat("aa", 32)},
		},
		{
			name:         "Multiple operations",
			script:       "020102" + "51", // push 2 bytes, then OP_1
			expectedOps:  []byte{0x02, OP_1},
			expectedData: []string{"0102", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scriptBytes, err := hex.DecodeString(tt.script)
			if err != nil {
				t.Fatalf("Failed to decode script hex: %v", err)
			}

			pc := 0
			opIndex := 0
			for pc < len(scriptBytes) {
				opcode, data, newPC, err := getOp(scriptBytes, pc)
				if err != nil {
					t.Fatalf("getOp failed at pc=%d: %v", pc, err)
				}

				if opIndex >= len(tt.expectedOps) {
					t.Errorf("Got more opcodes than expected")
					break
				}

				if opcode != tt.expectedOps[opIndex] {
					t.Errorf("Op %d: expected opcode 0x%02x, got 0x%02x",
						opIndex, tt.expectedOps[opIndex], opcode)
				}

				expectedDataHex := tt.expectedData[opIndex]
				if expectedDataHex != "" {
					if data == nil {
						t.Errorf("Op %d: expected data %s, got nil", opIndex, expectedDataHex)
					} else {
						gotDataHex := hex.EncodeToString(data)
						if gotDataHex != expectedDataHex {
							t.Errorf("Op %d: expected data %s, got %s",
								opIndex, expectedDataHex, gotDataHex)
						}
					}
				} else if data != nil && len(data) > 0 {
					t.Errorf("Op %d: expected no data, got %x", opIndex, data)
				}

				pc = newPC
				opIndex++
			}

			if opIndex != len(tt.expectedOps) {
				t.Errorf("Expected %d opcodes, processed %d", len(tt.expectedOps), opIndex)
			}
		})
	}
}

// contains checks if string s contains substr
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > len(substr) && (s[:len(substr)] == substr ||
		contains(s[1:], substr)))
}

// strings.Repeat replacement for testing
var strings = struct {
	Repeat func(s string, count int) string
}{
	Repeat: func(s string, count int) string {
		result := ""
		for i := 0; i < count; i++ {
			result += s
		}
		return result
	},
}