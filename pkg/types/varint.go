package types

import (
	"encoding/binary"
	"io"
)

// Bitcoin compact size (varint) encoding
// Matches Bitcoin Core's compact size serialization

// WriteCompactSize writes a Bitcoin compact size (varint) to the writer
func WriteCompactSize(w io.Writer, val uint64) error {
	if val < 0xfd {
		// 1 byte: 0x00-0xfc
		_, err := w.Write([]byte{byte(val)})
		return err
	} else if val <= 0xffff {
		// 3 bytes: 0xfd + 2 byte uint16
		if _, err := w.Write([]byte{0xfd}); err != nil {
			return err
		}
		return binary.Write(w, binary.LittleEndian, uint16(val))
	} else if val <= 0xffffffff {
		// 5 bytes: 0xfe + 4 byte uint32
		if _, err := w.Write([]byte{0xfe}); err != nil {
			return err
		}
		return binary.Write(w, binary.LittleEndian, uint32(val))
	} else {
		// 9 bytes: 0xff + 8 byte uint64
		if _, err := w.Write([]byte{0xff}); err != nil {
			return err
		}
		return binary.Write(w, binary.LittleEndian, val)
	}
}

// ReadCompactSize reads a Bitcoin compact size (varint) from the reader
func ReadCompactSize(r io.Reader) (uint64, error) {
	var first [1]byte
	if _, err := r.Read(first[:]); err != nil {
		return 0, err
	}

	switch first[0] {
	case 0xfd:
		var val uint16
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return 0, err
		}
		return uint64(val), nil
	case 0xfe:
		var val uint32
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return 0, err
		}
		return uint64(val), nil
	case 0xff:
		var val uint64
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return 0, err
		}
		return val, nil
	default:
		return uint64(first[0]), nil
	}
}

// CompactSizeLen returns the length of the compact size encoding for a value
func CompactSizeLen(val uint64) int {
	if val < 0xfd {
		return 1
	} else if val <= 0xffff {
		return 3
	} else if val <= 0xffffffff {
		return 5
	} else {
		return 9
	}
}
