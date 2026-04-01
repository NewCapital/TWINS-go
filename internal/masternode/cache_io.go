package masternode

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
)

// writeCacheHeader writes the standard cache file header: magic message + network magic.
func writeCacheHeader(w io.Writer, magicMessage string, networkMagic []byte) error {
	if err := writeVarString(w, magicMessage); err != nil {
		return fmt.Errorf("failed to write magic message: %w", err)
	}
	if _, err := w.Write(networkMagic); err != nil {
		return fmt.Errorf("failed to write network magic: %w", err)
	}
	return nil
}

// readCacheHeader reads and validates the standard cache file header.
func readCacheHeader(r io.Reader, expectedMagic string, networkMagic []byte, errInvalidMagic, errInvalidNetwork error) error {
	magicMsg, err := readVarString(r)
	if err != nil {
		return fmt.Errorf("failed to read magic message: %w", err)
	}
	if magicMsg != expectedMagic {
		return errInvalidMagic
	}

	var readMagic [4]byte
	if _, err := io.ReadFull(r, readMagic[:]); err != nil {
		return fmt.Errorf("failed to read network magic: %w", err)
	}
	if !bytes.Equal(readMagic[:], networkMagic) {
		return errInvalidNetwork
	}
	return nil
}

// calculateSHA256d computes double SHA-256 of the given data (Bitcoin Hash256).
func calculateSHA256d(data []byte) [32]byte {
	hash := sha256.Sum256(data)
	return sha256.Sum256(hash[:])
}

// verifySHA256d checks whether stored matches the double SHA-256 of data.
func verifySHA256d(data []byte, stored [32]byte) bool {
	return calculateSHA256d(data) == stored
}

// writeVarInt writes a variable-length integer to the writer.
// Format matches Bitcoin's CompactSize encoding.
func writeVarInt(w *bytes.Buffer, n uint64) error {
	if n < 0xFD {
		return w.WriteByte(byte(n))
	} else if n <= 0xFFFF {
		if err := w.WriteByte(0xFD); err != nil {
			return err
		}
		return binary.Write(w, binary.LittleEndian, uint16(n))
	} else if n <= 0xFFFFFFFF {
		if err := w.WriteByte(0xFE); err != nil {
			return err
		}
		return binary.Write(w, binary.LittleEndian, uint32(n))
	} else {
		if err := w.WriteByte(0xFF); err != nil {
			return err
		}
		return binary.Write(w, binary.LittleEndian, n)
	}
}
