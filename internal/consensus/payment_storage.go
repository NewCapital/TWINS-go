package consensus

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/twins-dev/twins-core/pkg/types"
)

// PaymentVoteStorage defines the interface for persistent payment vote storage
// This matches the legacy CMasternodePaymentDB functionality from masternode-payments.cpp
//
// Legacy file: mnpayments.dat
// Legacy structure: CMasternodePayments (mapMasternodePayeeVotes + mapMasternodeBlocks)
type PaymentVoteStorage interface {
	// StoreBlockPaymentVotes stores votes for a specific block height
	StoreBlockPaymentVotes(height uint32, votes *BlockPaymentVotes) error

	// GetBlockPaymentVotes retrieves votes for a specific block height
	GetBlockPaymentVotes(height uint32) (*BlockPaymentVotes, error)

	// DeleteBlockPaymentVotes removes votes for a specific block height
	DeleteBlockPaymentVotes(height uint32) error

	// StoreLastVote stores the last vote height for a masternode
	StoreLastVote(outpoint types.Outpoint, height uint32) error

	// GetLastVote retrieves the last vote height for a masternode
	GetLastVote(outpoint types.Outpoint) (uint32, error)

	// DeleteLastVote removes the last vote record for a masternode
	DeleteLastVote(outpoint types.Outpoint) error

	// LoadAllVotes loads all payment votes from storage into memory
	// Returns map[height] → BlockPaymentVotes
	LoadAllVotes() (map[uint32]*BlockPaymentVotes, error)

	// LoadAllLastVotes loads all last vote records from storage
	// Returns map[outpoint] → height
	LoadAllLastVotes() (map[types.Outpoint]uint32, error)

	// CleanOldVotes removes votes older than cutoffHeight
	CleanOldVotes(cutoffHeight uint32) error
}

// Storage key prefixes for payment vote data
// Using 0x20-0x2F range for masternode payment data
const (
	// PrefixPaymentVotes stores BlockPaymentVotes by height
	// Key: 0x20 + uint32(height) → serialized BlockPaymentVotes
	PrefixPaymentVotes byte = 0x20

	// PrefixLastVote stores last vote height by masternode outpoint
	// Key: 0x21 + Hash(32) + Index(4) → uint32(height)
	PrefixLastVote byte = 0x21
)

// PaymentVoteDB implements PaymentVoteStorage using Pebble database
type PaymentVoteDB struct {
	db PebbleDBInterface
	mu sync.RWMutex
}

// PebbleDBInterface defines the minimal interface needed from Pebble
// This allows the payment storage to use the existing storage layer
type PebbleDBInterface interface {
	Get(key []byte) ([]byte, error)
	Set(key, value []byte) error
	Delete(key []byte) error
	// NewIterPrefix returns an iterator over keys with the given prefix
	NewIterPrefix(prefix []byte) (PebbleIterator, error)
}

// PebbleIterator defines the iterator interface
type PebbleIterator interface {
	Valid() bool
	Key() []byte
	Value() []byte
	Next() bool
	Close() error
}

// NewPaymentVoteDB creates a new payment vote database
func NewPaymentVoteDB(db PebbleDBInterface) *PaymentVoteDB {
	return &PaymentVoteDB{
		db: db,
	}
}

// makePaymentVotesKey creates a key for BlockPaymentVotes
func makePaymentVotesKey(height uint32) []byte {
	key := make([]byte, 5)
	key[0] = PrefixPaymentVotes
	binary.LittleEndian.PutUint32(key[1:], height)
	return key
}

// makeLastVoteKey creates a key for last vote record
func makeLastVoteKey(outpoint types.Outpoint) []byte {
	key := make([]byte, 37) // 1 + 32 + 4
	key[0] = PrefixLastVote
	copy(key[1:33], outpoint.Hash[:])
	binary.LittleEndian.PutUint32(key[33:], outpoint.Index)
	return key
}

// serializeBlockPaymentVotes serializes BlockPaymentVotes for storage
// Format: [BlockHeight:4][NumPayees:4][Payee1...PayeeN]
// Each Payee: [ScriptLen:4][Script:...][Votes:4]
func serializeBlockPaymentVotes(votes *BlockPaymentVotes) ([]byte, error) {
	votes.mu.RLock()
	defer votes.mu.RUnlock()

	buf := new(bytes.Buffer)

	// Write block height
	if err := binary.Write(buf, binary.LittleEndian, votes.BlockHeight); err != nil {
		return nil, fmt.Errorf("failed to write block height: %w", err)
	}

	// Write number of payees
	numPayees := uint32(len(votes.Payees))
	if err := binary.Write(buf, binary.LittleEndian, numPayees); err != nil {
		return nil, fmt.Errorf("failed to write num payees: %w", err)
	}

	// Write each payee
	for _, payee := range votes.Payees {
		// Script length
		scriptLen := uint32(len(payee.PayAddress))
		if err := binary.Write(buf, binary.LittleEndian, scriptLen); err != nil {
			return nil, fmt.Errorf("failed to write script length: %w", err)
		}

		// Script bytes
		if _, err := buf.Write(payee.PayAddress); err != nil {
			return nil, fmt.Errorf("failed to write script: %w", err)
		}

		// Vote count
		if err := binary.Write(buf, binary.LittleEndian, int32(payee.Votes)); err != nil {
			return nil, fmt.Errorf("failed to write votes: %w", err)
		}
	}

	return buf.Bytes(), nil
}

// deserializeBlockPaymentVotes deserializes BlockPaymentVotes from storage
func deserializeBlockPaymentVotes(data []byte) (*BlockPaymentVotes, error) {
	buf := bytes.NewReader(data)

	votes := &BlockPaymentVotes{
		Payees: make([]*PayeeVotes, 0),
	}

	// Read block height
	if err := binary.Read(buf, binary.LittleEndian, &votes.BlockHeight); err != nil {
		return nil, fmt.Errorf("failed to read block height: %w", err)
	}

	// Read number of payees
	var numPayees uint32
	if err := binary.Read(buf, binary.LittleEndian, &numPayees); err != nil {
		return nil, fmt.Errorf("failed to read num payees: %w", err)
	}

	// Read each payee
	for i := uint32(0); i < numPayees; i++ {
		// Script length
		var scriptLen uint32
		if err := binary.Read(buf, binary.LittleEndian, &scriptLen); err != nil {
			return nil, fmt.Errorf("failed to read script length: %w", err)
		}

		// Script bytes
		script := make([]byte, scriptLen)
		if _, err := buf.Read(script); err != nil {
			return nil, fmt.Errorf("failed to read script: %w", err)
		}

		// Vote count
		var voteCount int32
		if err := binary.Read(buf, binary.LittleEndian, &voteCount); err != nil {
			return nil, fmt.Errorf("failed to read votes: %w", err)
		}

		votes.Payees = append(votes.Payees, &PayeeVotes{
			PayAddress: script,
			Votes:      int(voteCount),
		})
	}

	return votes, nil
}

// StoreBlockPaymentVotes stores votes for a specific block height
func (pv *PaymentVoteDB) StoreBlockPaymentVotes(height uint32, votes *BlockPaymentVotes) error {
	pv.mu.Lock()
	defer pv.mu.Unlock()

	data, err := serializeBlockPaymentVotes(votes)
	if err != nil {
		return fmt.Errorf("failed to serialize votes: %w", err)
	}

	key := makePaymentVotesKey(height)
	if err := pv.db.Set(key, data); err != nil {
		return fmt.Errorf("failed to store votes for height %d: %w", height, err)
	}

	return nil
}

// GetBlockPaymentVotes retrieves votes for a specific block height
// Returns nil, nil if no votes found (consistent with RawGet pattern)
func (pv *PaymentVoteDB) GetBlockPaymentVotes(height uint32) (*BlockPaymentVotes, error) {
	pv.mu.RLock()
	defer pv.mu.RUnlock()

	key := makePaymentVotesKey(height)
	data, err := pv.db.Get(key)
	if err != nil {
		return nil, fmt.Errorf("failed to get votes for height %d: %w", height, err)
	}

	// Not found is valid - return nil, nil (consistent with RawGet pattern)
	if len(data) == 0 {
		return nil, nil
	}

	return deserializeBlockPaymentVotes(data)
}

// DeleteBlockPaymentVotes removes votes for a specific block height
func (pv *PaymentVoteDB) DeleteBlockPaymentVotes(height uint32) error {
	pv.mu.Lock()
	defer pv.mu.Unlock()

	key := makePaymentVotesKey(height)
	return pv.db.Delete(key)
}

// StoreLastVote stores the last vote height for a masternode
func (pv *PaymentVoteDB) StoreLastVote(outpoint types.Outpoint, height uint32) error {
	pv.mu.Lock()
	defer pv.mu.Unlock()

	key := makeLastVoteKey(outpoint)
	value := make([]byte, 4)
	binary.LittleEndian.PutUint32(value, height)

	return pv.db.Set(key, value)
}

// GetLastVote retrieves the last vote height for a masternode
func (pv *PaymentVoteDB) GetLastVote(outpoint types.Outpoint) (uint32, error) {
	pv.mu.RLock()
	defer pv.mu.RUnlock()

	key := makeLastVoteKey(outpoint)
	data, err := pv.db.Get(key)
	if err != nil {
		return 0, err
	}

	if len(data) != 4 {
		return 0, fmt.Errorf("invalid last vote data length: %d", len(data))
	}

	return binary.LittleEndian.Uint32(data), nil
}

// DeleteLastVote removes the last vote record for a masternode
func (pv *PaymentVoteDB) DeleteLastVote(outpoint types.Outpoint) error {
	pv.mu.Lock()
	defer pv.mu.Unlock()

	key := makeLastVoteKey(outpoint)
	return pv.db.Delete(key)
}

// LoadAllVotesResult contains the result of LoadAllVotes including any errors
type LoadAllVotesResult struct {
	Votes       map[uint32]*BlockPaymentVotes
	FailedCount int // Number of entries that failed to deserialize
}

// LoadAllVotes loads all payment votes from storage
// Returns partial results if some entries fail to deserialize (FailedCount > 0)
func (pv *PaymentVoteDB) LoadAllVotes() (map[uint32]*BlockPaymentVotes, error) {
	pv.mu.RLock()
	defer pv.mu.RUnlock()

	result := make(map[uint32]*BlockPaymentVotes)
	var failedCount int

	prefix := []byte{PrefixPaymentVotes}
	iter, err := pv.db.NewIterPrefix(prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close()

	for iter.Valid() {
		key := iter.Key()
		if len(key) != 5 {
			iter.Next()
			continue
		}

		height := binary.LittleEndian.Uint32(key[1:])
		votes, err := deserializeBlockPaymentVotes(iter.Value())
		if err != nil {
			// Count failed deserialization but continue loading other votes
			failedCount++
			iter.Next()
			continue
		}

		result[height] = votes
		iter.Next()
	}

	// Return error if any entries failed (caller can log warning)
	if failedCount > 0 {
		return result, fmt.Errorf("failed to deserialize %d vote entries", failedCount)
	}

	return result, nil
}

// LoadAllLastVotes loads all last vote records from storage
func (pv *PaymentVoteDB) LoadAllLastVotes() (map[types.Outpoint]uint32, error) {
	pv.mu.RLock()
	defer pv.mu.RUnlock()

	result := make(map[types.Outpoint]uint32)

	prefix := []byte{PrefixLastVote}
	iter, err := pv.db.NewIterPrefix(prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close()

	for iter.Valid() {
		key := iter.Key()
		if len(key) != 37 {
			iter.Next()
			continue
		}

		var outpoint types.Outpoint
		copy(outpoint.Hash[:], key[1:33])
		outpoint.Index = binary.LittleEndian.Uint32(key[33:])

		data := iter.Value()
		if len(data) != 4 {
			iter.Next()
			continue
		}

		height := binary.LittleEndian.Uint32(data)
		result[outpoint] = height
		iter.Next()
	}

	return result, nil
}

// CleanOldVotes removes votes older than cutoffHeight
func (pv *PaymentVoteDB) CleanOldVotes(cutoffHeight uint32) error {
	pv.mu.Lock()
	defer pv.mu.Unlock()

	// Clean payment votes
	if err := pv.cleanPaymentVotesLocked(cutoffHeight); err != nil {
		return err
	}

	// Clean last votes
	return pv.cleanLastVotesLocked(cutoffHeight)
}

// cleanPaymentVotesLocked cleans old payment votes (caller must hold lock)
func (pv *PaymentVoteDB) cleanPaymentVotesLocked(cutoffHeight uint32) error {
	prefix := []byte{PrefixPaymentVotes}
	iter, err := pv.db.NewIterPrefix(prefix)
	if err != nil {
		return fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close() // Guaranteed cleanup even on early return

	keysToDelete := make([][]byte, 0)
	for iter.Valid() {
		key := iter.Key()
		if len(key) == 5 {
			height := binary.LittleEndian.Uint32(key[1:])
			if height < cutoffHeight {
				keysToDelete = append(keysToDelete, append([]byte{}, key...))
			}
		}
		iter.Next()
	}

	for _, key := range keysToDelete {
		if err := pv.db.Delete(key); err != nil {
			return fmt.Errorf("failed to delete old vote: %w", err)
		}
	}

	return nil
}

// cleanLastVotesLocked cleans old last vote records (caller must hold lock)
func (pv *PaymentVoteDB) cleanLastVotesLocked(cutoffHeight uint32) error {
	prefix := []byte{PrefixLastVote}
	iter, err := pv.db.NewIterPrefix(prefix)
	if err != nil {
		return fmt.Errorf("failed to create last vote iterator: %w", err)
	}
	defer iter.Close() // Guaranteed cleanup even on early return

	keysToDelete := make([][]byte, 0)
	for iter.Valid() {
		key := iter.Key()
		data := iter.Value()
		if len(key) == 37 && len(data) == 4 {
			height := binary.LittleEndian.Uint32(data)
			if height < cutoffHeight {
				keysToDelete = append(keysToDelete, append([]byte{}, key...))
			}
		}
		iter.Next()
	}

	for _, key := range keysToDelete {
		if err := pv.db.Delete(key); err != nil {
			return fmt.Errorf("failed to delete old last vote: %w", err)
		}
	}

	return nil
}

// RawStorage defines the minimal interface for low-level database access
// This is implemented by storage.BinaryStorage (via RawAccessAdapter)
type RawStorage interface {
	// RawGet retrieves a value by key, returns nil for not found
	RawGet(key []byte) ([]byte, error)
	// RawSet stores a key-value pair
	RawSet(key, value []byte) error
	// RawDelete removes a key
	RawDelete(key []byte) error
	// RawIterPrefix iterates over keys with given prefix
	RawIterPrefix(prefix []byte, fn func(key, value []byte) bool) error
}

// RawAccessAdapter adapts RawStorage to PebbleDBInterface
// This bridges the gap between the storage layer and payment vote storage
type RawAccessAdapter struct {
	storage RawStorage
}

// NewRawAccessAdapter creates a new adapter for raw storage access
func NewRawAccessAdapter(storage RawStorage) *RawAccessAdapter {
	return &RawAccessAdapter{storage: storage}
}

// Get retrieves a value by key
func (a *RawAccessAdapter) Get(key []byte) ([]byte, error) {
	return a.storage.RawGet(key)
}

// Set stores a key-value pair
func (a *RawAccessAdapter) Set(key, value []byte) error {
	return a.storage.RawSet(key, value)
}

// Delete removes a key
func (a *RawAccessAdapter) Delete(key []byte) error {
	return a.storage.RawDelete(key)
}

// NewIterPrefix returns an iterator over keys with the given prefix
func (a *RawAccessAdapter) NewIterPrefix(prefix []byte) (PebbleIterator, error) {
	// Collect all matching keys into a slice for iteration
	var entries []struct {
		key   []byte
		value []byte
	}

	err := a.storage.RawIterPrefix(prefix, func(key, value []byte) bool {
		// Copy key and value to avoid iterator invalidation
		keyCopy := make([]byte, len(key))
		copy(keyCopy, key)
		valueCopy := make([]byte, len(value))
		copy(valueCopy, value)
		entries = append(entries, struct {
			key   []byte
			value []byte
		}{keyCopy, valueCopy})
		return true // continue iteration
	})
	if err != nil {
		return nil, err
	}

	return &sliceIterator{
		entries: entries,
		index:   0,
	}, nil
}

// sliceIterator implements PebbleIterator over a slice of entries
type sliceIterator struct {
	entries []struct {
		key   []byte
		value []byte
	}
	index int
}

func (si *sliceIterator) Valid() bool {
	return si.index < len(si.entries)
}

func (si *sliceIterator) Key() []byte {
	if si.index < len(si.entries) {
		return si.entries[si.index].key
	}
	return nil
}

func (si *sliceIterator) Value() []byte {
	if si.index < len(si.entries) {
		return si.entries[si.index].value
	}
	return nil
}

func (si *sliceIterator) Next() bool {
	si.index++
	return si.Valid()
}

func (si *sliceIterator) Close() error {
	si.entries = nil
	return nil
}
