// Copyright (c) 2025 The TWINS Core developers
// Distributed under the MIT software license

package spork

import (
	"encoding/binary"
	"fmt"

	"github.com/cockroachdb/pebble"
)

// deserializeMessage decodes a spork Message from its binary representation.
// Format: SporkID (4) + Value (8) + TimeSigned (8) + SigLen (2) + Signature (variable)
func deserializeMessage(value []byte) (*Message, error) {
	if len(value) < 22 {
		return nil, fmt.Errorf("invalid spork data length: %d", len(value))
	}

	msg := &Message{}
	msg.SporkID = int32(binary.LittleEndian.Uint32(value[0:4]))
	msg.Value = int64(binary.LittleEndian.Uint64(value[4:12]))
	msg.TimeSigned = int64(binary.LittleEndian.Uint64(value[12:20]))

	sigLen := binary.LittleEndian.Uint16(value[20:22])
	if len(value) < 22+int(sigLen) {
		return nil, fmt.Errorf("invalid signature length: %d", sigLen)
	}

	msg.Signature = make([]byte, sigLen)
	copy(msg.Signature, value[22:22+sigLen])

	return msg, nil
}

// PebbleStorage implements Storage interface using Pebble database
type PebbleStorage struct {
	db *pebble.DB
}

// NewPebbleStorage creates a new Pebble-based spork storage
func NewPebbleStorage(db *pebble.DB) *PebbleStorage {
	return &PebbleStorage{db: db}
}

// ReadSpork reads a spork from storage by ID
func (s *PebbleStorage) ReadSpork(sporkID int32) (*Message, error) {
	key := sporkKey(sporkID)

	value, closer, err := s.db.Get(key)
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	defer closer.Close()

	return deserializeMessage(value)
}

// WriteSpork persists a spork to storage
func (s *PebbleStorage) WriteSpork(spork *Message) error {
	key := sporkKey(spork.SporkID)

	// Serialize: SporkID (4) + Value (8) + TimeSigned (8) + SigLen (2) + Signature (variable)
	sigLen := uint16(len(spork.Signature))
	value := make([]byte, 22+sigLen)

	binary.LittleEndian.PutUint32(value[0:4], uint32(spork.SporkID))
	binary.LittleEndian.PutUint64(value[4:12], uint64(spork.Value))
	binary.LittleEndian.PutUint64(value[12:20], uint64(spork.TimeSigned))
	binary.LittleEndian.PutUint16(value[20:22], sigLen)
	copy(value[22:], spork.Signature)

	return s.db.Set(key, value, pebble.Sync)
}

// LoadAllSporks loads all sporks from storage
func (s *PebbleStorage) LoadAllSporks() (map[int32]*Message, error) {
	result := make(map[int32]*Message)

	prefix := []byte("spk:")
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix, 0xff),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		// Silently skip corrupt/truncated entries to allow startup with partial DB corruption.
		// If deserialization fails, the spork will use its default value instead.
		msg, err := deserializeMessage(iter.Value())
		if err != nil {
			continue
		}

		result[msg.SporkID] = msg
	}

	return result, iter.Error()
}

// sporkKey generates the storage key for a spork
// Format: "spk:" + sporkID (4 bytes)
func sporkKey(sporkID int32) []byte {
	key := make([]byte, 8)
	copy(key[0:4], []byte("spk:"))
	binary.LittleEndian.PutUint32(key[4:8], uint32(sporkID))
	return key
}