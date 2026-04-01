// Package serialization implements Bitcoin-compatible binary serialization
// for wallet.dat compatibility with the legacy C++ TWINS client.
//
// This package provides low-level primitives for serializing data structures
// in the exact format expected by BerkeleyDB-based wallet files.
package serialization

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

var (
	// ErrInvalidCompactSize indicates an invalid CompactSize encoding
	ErrInvalidCompactSize = errors.New("invalid compact size encoding")
	// ErrBufferTooSmall indicates the buffer is too small for the requested operation
	ErrBufferTooSmall = errors.New("buffer too small")
)

// CompactSize represents Bitcoin's variable-length integer encoding
// Used for encoding lengths of vectors, strings, and maps.
//
// Encoding rules:
//   - Values < 0xFD: 1 byte
//   - Values 0xFD-0xFFFF: 0xFD + 2 bytes (little-endian uint16)
//   - Values 0x10000-0xFFFFFFFF: 0xFE + 4 bytes (little-endian uint32)
//   - Values > 0xFFFFFFFF: 0xFF + 8 bytes (little-endian uint64)
type CompactSize uint64

// WriteCompactSize writes a CompactSize-encoded integer to the writer
func WriteCompactSize(w io.Writer, n uint64) error {
	var buf [9]byte
	var size int

	if n < 0xFD {
		buf[0] = byte(n)
		size = 1
	} else if n <= 0xFFFF {
		buf[0] = 0xFD
		binary.LittleEndian.PutUint16(buf[1:3], uint16(n))
		size = 3
	} else if n <= 0xFFFFFFFF {
		buf[0] = 0xFE
		binary.LittleEndian.PutUint32(buf[1:5], uint32(n))
		size = 5
	} else {
		buf[0] = 0xFF
		binary.LittleEndian.PutUint64(buf[1:9], n)
		size = 9
	}

	_, err := w.Write(buf[:size])
	return err
}

// ReadCompactSize reads a CompactSize-encoded integer from the reader
func ReadCompactSize(r io.Reader) (uint64, error) {
	var firstByte [1]byte
	if _, err := io.ReadFull(r, firstByte[:]); err != nil {
		return 0, err
	}

	switch firstByte[0] {
	case 0xFF:
		var buf [8]byte
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return 0, err
		}
		return binary.LittleEndian.Uint64(buf[:]), nil

	case 0xFE:
		var buf [4]byte
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return 0, err
		}
		return uint64(binary.LittleEndian.Uint32(buf[:])), nil

	case 0xFD:
		var buf [2]byte
		if _, err := io.ReadFull(r, buf[:]); err != nil {
			return 0, err
		}
		return uint64(binary.LittleEndian.Uint16(buf[:])), nil

	default:
		return uint64(firstByte[0]), nil
	}
}

// WriteUint8 writes a single byte
func WriteUint8(w io.Writer, n uint8) error {
	_, err := w.Write([]byte{n})
	return err
}

// ReadUint8 reads a single byte
func ReadUint8(r io.Reader) (uint8, error) {
	var buf [1]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return buf[0], nil
}

// WriteUint16 writes a uint16 in little-endian format
func WriteUint16(w io.Writer, n uint16) error {
	var buf [2]byte
	binary.LittleEndian.PutUint16(buf[:], n)
	_, err := w.Write(buf[:])
	return err
}

// ReadUint16 reads a uint16 in little-endian format
func ReadUint16(r io.Reader) (uint16, error) {
	var buf [2]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(buf[:]), nil
}

// WriteUint32 writes a uint32 in little-endian format
func WriteUint32(w io.Writer, n uint32) error {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], n)
	_, err := w.Write(buf[:])
	return err
}

// ReadUint32 reads a uint32 in little-endian format
func ReadUint32(r io.Reader) (uint32, error) {
	var buf [4]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(buf[:]), nil
}

// WriteUint64 writes a uint64 in little-endian format
func WriteUint64(w io.Writer, n uint64) error {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], n)
	_, err := w.Write(buf[:])
	return err
}

// ReadUint64 reads a uint64 in little-endian format
func ReadUint64(r io.Reader) (uint64, error) {
	var buf [8]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(buf[:]), nil
}

// WriteInt32 writes an int32 in little-endian format
func WriteInt32(w io.Writer, n int32) error {
	return WriteUint32(w, uint32(n))
}

// ReadInt32 reads an int32 in little-endian format
func ReadInt32(r io.Reader) (int32, error) {
	u, err := ReadUint32(r)
	return int32(u), err
}

// WriteInt64 writes an int64 in little-endian format
func WriteInt64(w io.Writer, n int64) error {
	return WriteUint64(w, uint64(n))
}

// ReadInt64 reads an int64 in little-endian format
func ReadInt64(r io.Reader) (int64, error) {
	u, err := ReadUint64(r)
	return int64(u), err
}

// WriteVarBytes writes a byte slice with CompactSize length prefix
func WriteVarBytes(w io.Writer, data []byte) error {
	if err := WriteCompactSize(w, uint64(len(data))); err != nil {
		return err
	}
	if len(data) > 0 {
		_, err := w.Write(data)
		return err
	}
	return nil
}

// ReadVarBytes reads a byte slice with CompactSize length prefix
func ReadVarBytes(r io.Reader) ([]byte, error) {
	length, err := ReadCompactSize(r)
	if err != nil {
		return nil, err
	}

	if length == 0 {
		return []byte{}, nil
	}

	// Validate length to prevent panic on corrupted data
	// Maximum reasonable size for wallet data is 10MB
	const maxLength = 10 * 1024 * 1024
	if length > maxLength {
		return nil, fmt.Errorf("invalid length: %d (max %d)", length, maxLength)
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}

	return data, nil
}

// WriteByte writes a single byte
func WriteByte(w io.Writer, b byte) error {
	_, err := w.Write([]byte{b})
	return err
}

// ReadByte reads a single byte
func ReadByte(r io.Reader) (byte, error) {
	var buf [1]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return 0, err
	}
	return buf[0], nil
}

// WriteString writes a string with CompactSize length prefix (UTF-8 encoded)
func WriteString(w io.Writer, s string) error {
	return WriteVarBytes(w, []byte(s))
}

// ReadString reads a string with CompactSize length prefix (UTF-8 encoded)
func ReadString(r io.Reader) (string, error) {
	data, err := ReadVarBytes(r)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteBool writes a boolean as a single byte (0x00 or 0x01)
func WriteBool(w io.Writer, b bool) error {
	if b {
		return WriteUint8(w, 1)
	}
	return WriteUint8(w, 0)
}

// ReadBool reads a boolean as a single byte
func ReadBool(r io.Reader) (bool, error) {
	b, err := ReadUint8(r)
	if err != nil {
		return false, err
	}
	return b != 0, nil
}

// WriteFixedBytes writes a fixed-length byte array (no length prefix)
func WriteFixedBytes(w io.Writer, data []byte) error {
	_, err := w.Write(data)
	return err
}

// ReadFixedBytes reads a fixed-length byte array (no length prefix)
func ReadFixedBytes(r io.Reader, length int) ([]byte, error) {
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}
	return data, nil
}
