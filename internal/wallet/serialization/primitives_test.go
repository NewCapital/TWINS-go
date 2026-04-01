package serialization

import (
	"bytes"
	"testing"
)

func TestCompactSize(t *testing.T) {
	tests := []struct {
		name     string
		value    uint64
		expected []byte
	}{
		{"zero", 0, []byte{0x00}},
		{"small", 42, []byte{0x2A}},
		{"max_single_byte", 252, []byte{0xFC}},
		{"min_two_bytes", 253, []byte{0xFD, 0xFD, 0x00}},
		{"two_bytes", 0xFFFF, []byte{0xFD, 0xFF, 0xFF}},
		{"min_four_bytes", 0x10000, []byte{0xFE, 0x00, 0x00, 0x01, 0x00}},
		{"four_bytes", 0xFFFFFFFF, []byte{0xFE, 0xFF, 0xFF, 0xFF, 0xFF}},
		{"min_eight_bytes", 0x100000000, []byte{0xFF, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00}},
		{"eight_bytes", 0xFFFFFFFFFFFFFFFF, []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_write", func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteCompactSize(&buf, tt.value); err != nil {
				t.Fatalf("WriteCompactSize failed: %v", err)
			}
			if !bytes.Equal(buf.Bytes(), tt.expected) {
				t.Errorf("WriteCompactSize(%d) = %x, want %x", tt.value, buf.Bytes(), tt.expected)
			}
		})

		t.Run(tt.name+"_read", func(t *testing.T) {
			buf := bytes.NewReader(tt.expected)
			value, err := ReadCompactSize(buf)
			if err != nil {
				t.Fatalf("ReadCompactSize failed: %v", err)
			}
			if value != tt.value {
				t.Errorf("ReadCompactSize(%x) = %d, want %d", tt.expected, value, tt.value)
			}
		})
	}
}

func TestUint16(t *testing.T) {
	tests := []struct {
		value    uint16
		expected []byte
	}{
		{0, []byte{0x00, 0x00}},
		{1, []byte{0x01, 0x00}},
		{256, []byte{0x00, 0x01}},
		{0xFFFF, []byte{0xFF, 0xFF}},
		{0x1234, []byte{0x34, 0x12}}, // Little-endian
	}

	for _, tt := range tests {
		t.Run("write", func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteUint16(&buf, tt.value); err != nil {
				t.Fatalf("WriteUint16 failed: %v", err)
			}
			if !bytes.Equal(buf.Bytes(), tt.expected) {
				t.Errorf("WriteUint16(%d) = %x, want %x", tt.value, buf.Bytes(), tt.expected)
			}
		})

		t.Run("read", func(t *testing.T) {
			buf := bytes.NewReader(tt.expected)
			value, err := ReadUint16(buf)
			if err != nil {
				t.Fatalf("ReadUint16 failed: %v", err)
			}
			if value != tt.value {
				t.Errorf("ReadUint16(%x) = %d, want %d", tt.expected, value, tt.value)
			}
		})
	}
}

func TestUint32(t *testing.T) {
	tests := []struct {
		value    uint32
		expected []byte
	}{
		{0, []byte{0x00, 0x00, 0x00, 0x00}},
		{1, []byte{0x01, 0x00, 0x00, 0x00}},
		{256, []byte{0x00, 0x01, 0x00, 0x00}},
		{0xFFFFFFFF, []byte{0xFF, 0xFF, 0xFF, 0xFF}},
		{0x12345678, []byte{0x78, 0x56, 0x34, 0x12}}, // Little-endian
	}

	for _, tt := range tests {
		t.Run("write", func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteUint32(&buf, tt.value); err != nil {
				t.Fatalf("WriteUint32 failed: %v", err)
			}
			if !bytes.Equal(buf.Bytes(), tt.expected) {
				t.Errorf("WriteUint32(%d) = %x, want %x", tt.value, buf.Bytes(), tt.expected)
			}
		})

		t.Run("read", func(t *testing.T) {
			buf := bytes.NewReader(tt.expected)
			value, err := ReadUint32(buf)
			if err != nil {
				t.Fatalf("ReadUint32 failed: %v", err)
			}
			if value != tt.value {
				t.Errorf("ReadUint32(%x) = %d, want %d", tt.expected, value, tt.value)
			}
		})
	}
}

func TestUint64(t *testing.T) {
	tests := []struct {
		value    uint64
		expected []byte
	}{
		{0, []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{1, []byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{0xFFFFFFFFFFFFFFFF, []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}},
		{0x123456789ABCDEF0, []byte{0xF0, 0xDE, 0xBC, 0x9A, 0x78, 0x56, 0x34, 0x12}}, // Little-endian
	}

	for _, tt := range tests {
		t.Run("write", func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteUint64(&buf, tt.value); err != nil {
				t.Fatalf("WriteUint64 failed: %v", err)
			}
			if !bytes.Equal(buf.Bytes(), tt.expected) {
				t.Errorf("WriteUint64(%d) = %x, want %x", tt.value, buf.Bytes(), tt.expected)
			}
		})

		t.Run("read", func(t *testing.T) {
			buf := bytes.NewReader(tt.expected)
			value, err := ReadUint64(buf)
			if err != nil {
				t.Fatalf("ReadUint64 failed: %v", err)
			}
			if value != tt.value {
				t.Errorf("ReadUint64(%x) = %d, want %d", tt.expected, value, tt.value)
			}
		})
	}
}

func TestVarBytes(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected []byte
	}{
		{"empty", []byte{}, []byte{0x00}},
		{"single_byte", []byte{0x42}, []byte{0x01, 0x42}},
		{"multiple_bytes", []byte{0x01, 0x02, 0x03}, []byte{0x03, 0x01, 0x02, 0x03}},
		{"256_bytes", make([]byte, 256), append([]byte{0xFD, 0x00, 0x01}, make([]byte, 256)...)},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_write", func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteVarBytes(&buf, tt.data); err != nil {
				t.Fatalf("WriteVarBytes failed: %v", err)
			}
			if !bytes.Equal(buf.Bytes(), tt.expected) {
				t.Errorf("WriteVarBytes(%x) = %x, want %x", tt.data, buf.Bytes(), tt.expected)
			}
		})

		t.Run(tt.name+"_read", func(t *testing.T) {
			buf := bytes.NewReader(tt.expected)
			data, err := ReadVarBytes(buf)
			if err != nil {
				t.Fatalf("ReadVarBytes failed: %v", err)
			}
			if !bytes.Equal(data, tt.data) {
				t.Errorf("ReadVarBytes(%x) = %x, want %x", tt.expected, data, tt.data)
			}
		})
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		name     string
		str      string
		expected []byte
	}{
		{"empty", "", []byte{0x00}},
		{"simple", "test", []byte{0x04, 0x74, 0x65, 0x73, 0x74}},
		{"utf8", "тест", []byte{0x08, 0xD1, 0x82, 0xD0, 0xB5, 0xD1, 0x81, 0xD1, 0x82}},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_write", func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteString(&buf, tt.str); err != nil {
				t.Fatalf("WriteString failed: %v", err)
			}
			if !bytes.Equal(buf.Bytes(), tt.expected) {
				t.Errorf("WriteString(%q) = %x, want %x", tt.str, buf.Bytes(), tt.expected)
			}
		})

		t.Run(tt.name+"_read", func(t *testing.T) {
			buf := bytes.NewReader(tt.expected)
			str, err := ReadString(buf)
			if err != nil {
				t.Fatalf("ReadString failed: %v", err)
			}
			if str != tt.str {
				t.Errorf("ReadString(%x) = %q, want %q", tt.expected, str, tt.str)
			}
		})
	}
}

func TestBool(t *testing.T) {
	tests := []struct {
		value    bool
		expected []byte
	}{
		{false, []byte{0x00}},
		{true, []byte{0x01}},
	}

	for _, tt := range tests {
		t.Run("write", func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteBool(&buf, tt.value); err != nil {
				t.Fatalf("WriteBool failed: %v", err)
			}
			if !bytes.Equal(buf.Bytes(), tt.expected) {
				t.Errorf("WriteBool(%v) = %x, want %x", tt.value, buf.Bytes(), tt.expected)
			}
		})

		t.Run("read", func(t *testing.T) {
			buf := bytes.NewReader(tt.expected)
			value, err := ReadBool(buf)
			if err != nil {
				t.Fatalf("ReadBool failed: %v", err)
			}
			if value != tt.value {
				t.Errorf("ReadBool(%x) = %v, want %v", tt.expected, value, tt.value)
			}
		})
	}
}

func TestFixedBytes(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"single", []byte{0x42}},
		{"hash", []byte{
			0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
			0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F,
			0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
			0x18, 0x19, 0x1A, 0x1B, 0x1C, 0x1D, 0x1E, 0x1F,
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_write", func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteFixedBytes(&buf, tt.data); err != nil {
				t.Fatalf("WriteFixedBytes failed: %v", err)
			}
			if !bytes.Equal(buf.Bytes(), tt.data) {
				t.Errorf("WriteFixedBytes(%x) = %x, want %x", tt.data, buf.Bytes(), tt.data)
			}
		})

		t.Run(tt.name+"_read", func(t *testing.T) {
			buf := bytes.NewReader(tt.data)
			data, err := ReadFixedBytes(buf, len(tt.data))
			if err != nil {
				t.Fatalf("ReadFixedBytes failed: %v", err)
			}
			if !bytes.Equal(data, tt.data) {
				t.Errorf("ReadFixedBytes(%x) = %x, want %x", tt.data, data, tt.data)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	// Test complex round-trip scenarios
	t.Run("multiple_values", func(t *testing.T) {
		var buf bytes.Buffer

		// Write multiple values
		if err := WriteUint32(&buf, 12345); err != nil {
			t.Fatal(err)
		}
		if err := WriteString(&buf, "hello"); err != nil {
			t.Fatal(err)
		}
		if err := WriteBool(&buf, true); err != nil {
			t.Fatal(err)
		}
		if err := WriteVarBytes(&buf, []byte{1, 2, 3}); err != nil {
			t.Fatal(err)
		}

		// Read them back
		r := bytes.NewReader(buf.Bytes())

		v1, err := ReadUint32(r)
		if err != nil || v1 != 12345 {
			t.Errorf("ReadUint32 = %d, %v; want 12345, nil", v1, err)
		}

		v2, err := ReadString(r)
		if err != nil || v2 != "hello" {
			t.Errorf("ReadString = %q, %v; want \"hello\", nil", v2, err)
		}

		v3, err := ReadBool(r)
		if err != nil || v3 != true {
			t.Errorf("ReadBool = %v, %v; want true, nil", v3, err)
		}

		v4, err := ReadVarBytes(r)
		if err != nil || !bytes.Equal(v4, []byte{1, 2, 3}) {
			t.Errorf("ReadVarBytes = %x, %v; want [1 2 3], nil", v4, err)
		}
	})
}
