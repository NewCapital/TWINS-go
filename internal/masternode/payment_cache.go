// Copyright (c) 2018-2025 The TWINS developers
// Distributed under the MIT/X11 software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package masternode

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/twins-dev/twins-core/pkg/types"
)

const (
	// PaymentCacheMagicMessage is the legacy magic message for mnpayments.dat
	// Matches C++ strMagicMessage = "MasternodePayments" in CMasternodePaymentDB
	PaymentCacheMagicMessage = "MasternodePayments"

	// Deserialization sanity limits to prevent DoS from malicious cache files
	maxPaymentCacheVotes  = 1000000 // Max winner votes in cache
	maxPaymentCacheBlocks = 1000000 // Max block payee entries in cache
	maxPaymentCachePayees = 10000   // Max payees per block or per deserialization call
	maxPayeeScriptSize    = 10000   // Max script size in bytes (CScript)
	maxPayeeSignatureSize = 1000    // Max signature size in bytes (vchSig)
)

var (
	// ErrPaymentCacheNotFound indicates the payment cache file doesn't exist
	ErrPaymentCacheNotFound = errors.New("mnpayments.dat not found")

	// ErrPaymentCacheCorrupted indicates the payment cache file has invalid checksum
	ErrPaymentCacheCorrupted = errors.New("mnpayments.dat corrupted: checksum mismatch")

	// ErrPaymentCacheInvalidMagic indicates wrong magic message
	ErrPaymentCacheInvalidMagic = errors.New("mnpayments.dat invalid: wrong magic message")

	// ErrPaymentCacheInvalidNetwork indicates wrong network magic
	ErrPaymentCacheInvalidNetwork = errors.New("mnpayments.dat invalid: wrong network magic")
)

// PaymentCacheData represents the serializable payment votes cache
// LEGACY COMPATIBILITY: Matches CMasternodePayments serialization format:
//   - mapMasternodePayeeVotes: map<uint256, CMasternodePaymentWinner>
//   - mapMasternodeBlocks: map<int, CMasternodeBlockPayees>
type PaymentCacheData struct {
	// WinnerVotes stores all payment votes (matches mapMasternodePayeeVotes)
	// Key is the vote hash, value is the full winner vote with signature
	WinnerVotes map[types.Hash]*PaymentWinnerCacheEntry

	// BlockPayees stores accumulated votes per block (matches mapMasternodeBlocks)
	// Key is block height, value is list of payees with vote counts
	BlockPayees map[uint32]*BlockPayeesCacheEntry
}

// PaymentWinnerCacheEntry represents a cached payment winner vote
// LEGACY COMPATIBILITY: Matches CMasternodePaymentWinner serialization:
//   - vinMasternode (CTxIn): collateral outpoint of voting masternode
//   - nBlockHeight: target block height for payment
//   - payee (CScript): payment script of winning masternode
//   - vchSig: signature over the vote
type PaymentWinnerCacheEntry struct {
	VoterOutpoint types.Outpoint // vinMasternode.prevout
	BlockHeight   uint32         // nBlockHeight
	PayeeScript   []byte         // payee (CScript)
	Signature     []byte         // vchSig
}

// BlockPayeesCacheEntry represents cached payee votes for a specific block
// LEGACY COMPATIBILITY: Matches CMasternodeBlockPayees serialization:
//   - nBlockHeight: the block height
//   - vecPayments: vector of (scriptPubKey, nVotes) pairs
type BlockPayeesCacheEntry struct {
	BlockHeight uint32               // nBlockHeight
	Payees      []*PayeeCacheEntry   // vecPayments
}

// PayeeCacheEntry represents a single payee with vote count
// LEGACY COMPATIBILITY: Matches CMasternodePayee serialization
type PayeeCacheEntry struct {
	ScriptPubKey []byte // scriptPubKey (CScript)
	Votes        int32  // nVotes
}

// SavePaymentVotes saves payment votes to mnpayments.dat
// LEGACY COMPATIBILITY: Uses same format as C++ CMasternodePaymentDB::Write
//
// File format:
//  1. Magic message ("MasternodePayments") as var string
//  2. Network magic (4 bytes)
//  3. Serialized CMasternodePayments (mapMasternodePayeeVotes + mapMasternodeBlocks)
//  4. SHA256d checksum (32 bytes)
func (m *Manager) SavePaymentVotes(dataDir string, networkMagic []byte) error {
	if len(networkMagic) != 4 {
		return fmt.Errorf("network magic must be 4 bytes, got %d", len(networkMagic))
	}

	startTime := time.Now()

	// Build cache data from manager state
	m.mu.RLock()

	// Create PaymentCacheData from winnerVotes and scheduledPayments
	// LEGACY COMPAT: Now stores full winner votes with signatures (matches mapMasternodePayeeVotes)
	data := &PaymentCacheData{
		WinnerVotes: make(map[types.Hash]*PaymentWinnerCacheEntry),
		BlockPayees: make(map[uint32]*BlockPayeesCacheEntry),
	}

	// Copy winner votes with full data (matches legacy mapMasternodePayeeVotes)
	m.winnerVotesMu.RLock()
	for hash, vote := range m.winnerVotes {
		data.WinnerVotes[hash] = &PaymentWinnerCacheEntry{
			VoterOutpoint: vote.VoterOutpoint,
			BlockHeight:   vote.BlockHeight,
			PayeeScript:   vote.PayeeScript,
			Signature:     vote.Signature,
		}
	}
	winnerCount := len(m.winnerVotes)
	m.winnerVotesMu.RUnlock()

	// Build BlockPayees from winner votes (aggregate by block height)
	// This matches legacy mapMasternodeBlocks which tracks vote counts per payee
	blockPayees := make(map[uint32]map[string]int32) // height -> script(hex) -> votes
	for _, vote := range data.WinnerVotes {
		if blockPayees[vote.BlockHeight] == nil {
			blockPayees[vote.BlockHeight] = make(map[string]int32)
		}
		scriptKey := string(vote.PayeeScript)
		blockPayees[vote.BlockHeight][scriptKey]++
	}

	// Also include scheduledPayments (may have payments without full vote data)
	for height, script := range m.scheduledPayments {
		if blockPayees[height] == nil {
			blockPayees[height] = make(map[string]int32)
		}
		scriptKey := string(script)
		if blockPayees[height][scriptKey] == 0 {
			blockPayees[height][scriptKey] = 1
		}
	}

	// Convert aggregated votes to BlockPayeesCacheEntry format
	for height, payees := range blockPayees {
		entry := &BlockPayeesCacheEntry{
			BlockHeight: height,
			Payees:      make([]*PayeeCacheEntry, 0, len(payees)),
		}
		for scriptKey, votes := range payees {
			entry.Payees = append(entry.Payees, &PayeeCacheEntry{
				ScriptPubKey: []byte(scriptKey),
				Votes:        votes,
			})
		}
		data.BlockPayees[height] = entry
	}

	m.mu.RUnlock()

	// Serialize the cache data
	var buf bytes.Buffer

	if err := writeCacheHeader(&buf, PaymentCacheMagicMessage, networkMagic); err != nil {
		return err
	}

	if err := serializePaymentCacheData(&buf, data); err != nil {
		return fmt.Errorf("failed to serialize payment cache data: %w", err)
	}

	hash := calculateSHA256d(buf.Bytes())

	// Write to file atomically (write to temp, then rename)
	cachePath := filepath.Join(dataDir, "mnpayments.dat")
	tempPath := cachePath + ".tmp"

	file, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create mnpayments.dat.tmp: %w", err)
	}

	// removeTempFile attempts to clean up the temp file and logs on failure.
	removeTempFile := func() {
		if removeErr := os.Remove(tempPath); removeErr != nil && !os.IsNotExist(removeErr) {
			if m.logger != nil {
				m.logger.WithError(removeErr).Warn("Failed to remove temp file mnpayments.dat.tmp")
			}
		}
	}

	// Write data
	if _, err := file.Write(buf.Bytes()); err != nil {
		file.Close()
		removeTempFile()
		return fmt.Errorf("failed to write payment cache data: %w", err)
	}

	// Write checksum
	if _, err := file.Write(hash[:]); err != nil {
		file.Close()
		removeTempFile()
		return fmt.Errorf("failed to write checksum: %w", err)
	}

	if err := file.Sync(); err != nil {
		file.Close()
		removeTempFile()
		return fmt.Errorf("failed to sync mnpayments.dat: %w", err)
	}

	if err := file.Close(); err != nil {
		removeTempFile()
		return fmt.Errorf("failed to close mnpayments.dat: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, cachePath); err != nil {
		removeTempFile()
		return fmt.Errorf("failed to rename mnpayments.dat: %w", err)
	}

	if m.logger != nil {
		m.logger.WithField("duration_ms", time.Since(startTime).Milliseconds()).
			WithField("block_payees_count", len(data.BlockPayees)).
			WithField("winner_votes_count", winnerCount).
			Info("Saved payment votes to mnpayments.dat")
	}

	return nil
}

// LoadPaymentVotes loads payment votes from mnpayments.dat
// LEGACY COMPATIBILITY: Uses same format as C++ CMasternodePaymentDB::Read
func (m *Manager) LoadPaymentVotes(dataDir string, networkMagic []byte) error {
	if len(networkMagic) != 4 {
		return fmt.Errorf("network magic must be 4 bytes, got %d", len(networkMagic))
	}

	startTime := time.Now()
	cachePath := filepath.Join(dataDir, "mnpayments.dat")

	// Open file
	file, err := os.Open(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrPaymentCacheNotFound
		}
		return fmt.Errorf("failed to open mnpayments.dat: %w", err)
	}
	defer file.Close()

	// Get file size
	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat mnpayments.dat: %w", err)
	}

	// File must be at least 32 bytes (checksum size)
	if stat.Size() < 32 {
		return ErrPaymentCacheCorrupted
	}
	dataSize := stat.Size() - 32

	// Read all data except checksum
	data := make([]byte, dataSize)
	if _, err := io.ReadFull(file, data); err != nil {
		return fmt.Errorf("failed to read payment cache data: %w", err)
	}

	// Read checksum
	var storedHash [32]byte
	if _, err := io.ReadFull(file, storedHash[:]); err != nil {
		return fmt.Errorf("failed to read checksum: %w", err)
	}

	if !verifySHA256d(data, storedHash) {
		return ErrPaymentCacheCorrupted
	}

	buf := bytes.NewReader(data)

	if err := readCacheHeader(buf, PaymentCacheMagicMessage, networkMagic, ErrPaymentCacheInvalidMagic, ErrPaymentCacheInvalidNetwork); err != nil {
		return err
	}

	// Deserialize payment cache data
	cacheData, err := deserializePaymentCacheData(buf)
	if err != nil {
		return fmt.Errorf("failed to deserialize payment cache data: %w", err)
	}

	// Apply to manager state
	m.mu.Lock()
	defer m.mu.Unlock()

	loadedCount := 0
	masternodeBlocksCount := 0

	// Restore scheduledPayments AND masternodeBlocks from BlockPayees
	// LEGACY COMPAT: Populate both for deterministic payment queue after restart
	// C++ Reference: masternode-payments.cpp:67-147
	//
	// Lock ordering: mu (held above) -> masternodeBlocksMu.
	// This is the only site that nests these locks; all other masternodeBlocksMu
	// acquisitions are independent of mu.
	m.masternodeBlocksMu.Lock()
	for height, blockPayees := range cacheData.BlockPayees {
		if len(blockPayees.Payees) > 0 {
			// Create MasternodeBlockPayees entry for vote aggregation
			mnBlockPayees := NewMasternodeBlockPayees(height)
			var bestPayee *PayeeCacheEntry

			for _, payee := range blockPayees.Payees {
				// Add all payees to the masternodeBlocks entry
				mnBlockPayees.AddPayee(payee.ScriptPubKey, int(payee.Votes))

				// Track best payee for scheduledPayments
				if bestPayee == nil || payee.Votes > bestPayee.Votes {
					bestPayee = payee
				}
			}

			// Store in masternodeBlocks for IsScheduled/vote aggregation
			m.masternodeBlocks[height] = mnBlockPayees
			masternodeBlocksCount++

			// Store best payee in scheduledPayments for backward compatibility
			if bestPayee != nil && len(bestPayee.ScriptPubKey) > 0 {
				m.scheduledPayments[height] = bestPayee.ScriptPubKey
				loadedCount++
			}
		}
	}
	m.masternodeBlocksMu.Unlock()

	// Also restore winnerVotes for persistence
	m.winnerVotesMu.Lock()
	for hash, winner := range cacheData.WinnerVotes {
		m.winnerVotes[hash] = winner
	}
	winnerVotesCount := len(cacheData.WinnerVotes)
	m.winnerVotesMu.Unlock()

	if m.logger != nil {
		m.logger.WithField("duration_ms", time.Since(startTime).Milliseconds()).
			WithField("scheduled_payments", loadedCount).
			WithField("masternode_blocks", masternodeBlocksCount).
			WithField("winner_votes", winnerVotesCount).
			WithField("total_block_entries", len(cacheData.BlockPayees)).
			Info("Loaded payment votes from mnpayments.dat")
	}

	return nil
}

// serializePaymentCacheData serializes PaymentCacheData to match CMasternodePayments format
// Legacy format:
//   - mapMasternodePayeeVotes: map<uint256, CMasternodePaymentWinner>
//   - mapMasternodeBlocks: map<int, CMasternodeBlockPayees>
func serializePaymentCacheData(w io.Writer, data *PaymentCacheData) error {
	// 1. Write mapMasternodePayeeVotes (map<uint256, CMasternodePaymentWinner>)
	// For Go implementation, this is empty since we don't store full votes
	if err := writeCompactSize(w, uint64(len(data.WinnerVotes))); err != nil {
		return err
	}
	for hash, winner := range data.WinnerVotes {
		// Write key (uint256 hash)
		if _, err := w.Write(hash[:]); err != nil {
			return err
		}
		// Write value (CMasternodePaymentWinner)
		if err := serializePaymentWinner(w, winner); err != nil {
			return err
		}
	}

	// 2. Write mapMasternodeBlocks (map<int, CMasternodeBlockPayees>)
	if err := writeCompactSize(w, uint64(len(data.BlockPayees))); err != nil {
		return err
	}
	for height, blockPayees := range data.BlockPayees {
		// Write key (int32 height)
		if err := binary.Write(w, binary.LittleEndian, int32(height)); err != nil {
			return err
		}
		// Write value (CMasternodeBlockPayees)
		if err := serializeBlockPayees(w, blockPayees); err != nil {
			return err
		}
	}

	return nil
}

// deserializePaymentCacheData deserializes PaymentCacheData from CMasternodePayments format
func deserializePaymentCacheData(r io.Reader) (*PaymentCacheData, error) {
	data := &PaymentCacheData{
		WinnerVotes: make(map[types.Hash]*PaymentWinnerCacheEntry),
		BlockPayees: make(map[uint32]*BlockPayeesCacheEntry),
	}

	// 1. Read mapMasternodePayeeVotes
	voteCount, err := readCompactSize(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read vote count: %w", err)
	}
	if voteCount > maxPaymentCacheVotes {
		return nil, fmt.Errorf("too many votes in cache: %d", voteCount)
	}

	for i := uint64(0); i < voteCount; i++ {
		// Read key (uint256 hash)
		var hash types.Hash
		if _, err := io.ReadFull(r, hash[:]); err != nil {
			return nil, fmt.Errorf("failed to read vote hash %d: %w", i, err)
		}
		// Read value (CMasternodePaymentWinner)
		winner, err := deserializePaymentWinner(r)
		if err != nil {
			return nil, fmt.Errorf("failed to read winner %d: %w", i, err)
		}
		data.WinnerVotes[hash] = winner
	}

	// 2. Read mapMasternodeBlocks
	blockCount, err := readCompactSize(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read block count: %w", err)
	}
	if blockCount > maxPaymentCacheBlocks {
		return nil, fmt.Errorf("too many blocks in cache: %d", blockCount)
	}

	for i := uint64(0); i < blockCount; i++ {
		// Read key (int32 height)
		var height int32
		if err := binary.Read(r, binary.LittleEndian, &height); err != nil {
			return nil, fmt.Errorf("failed to read block height %d: %w", i, err)
		}
		// Read value (CMasternodeBlockPayees)
		blockPayees, err := deserializeBlockPayees(r)
		if err != nil {
			return nil, fmt.Errorf("failed to read block payees %d: %w", i, err)
		}
		data.BlockPayees[uint32(height)] = blockPayees
	}

	return data, nil
}

// serializePaymentWinner serializes CMasternodePaymentWinner format
// Legacy format:
//   - vinMasternode (CTxIn: hash + index + scriptSig + sequence)
//   - nBlockHeight (int32)
//   - payee (CScript as varbytes)
//   - vchSig (signature as varbytes)
func serializePaymentWinner(w io.Writer, winner *PaymentWinnerCacheEntry) error {
	// vinMasternode as CTxIn
	// Outpoint hash
	if _, err := w.Write(winner.VoterOutpoint.Hash[:]); err != nil {
		return err
	}
	// Outpoint index
	if err := binary.Write(w, binary.LittleEndian, winner.VoterOutpoint.Index); err != nil {
		return err
	}
	// scriptSig (empty for masternode CTxIn)
	if err := writeCompactSize(w, 0); err != nil {
		return err
	}
	// nSequence (default 0xFFFFFFFF)
	if err := binary.Write(w, binary.LittleEndian, uint32(0xFFFFFFFF)); err != nil {
		return err
	}

	// nBlockHeight
	if err := binary.Write(w, binary.LittleEndian, int32(winner.BlockHeight)); err != nil {
		return err
	}

	// payee (CScript as varbytes)
	if err := writeCompactSize(w, uint64(len(winner.PayeeScript))); err != nil {
		return err
	}
	if _, err := w.Write(winner.PayeeScript); err != nil {
		return err
	}

	// vchSig (signature as varbytes)
	if err := writeCompactSize(w, uint64(len(winner.Signature))); err != nil {
		return err
	}
	if _, err := w.Write(winner.Signature); err != nil {
		return err
	}

	return nil
}

// deserializePaymentWinner deserializes CMasternodePaymentWinner format
func deserializePaymentWinner(r io.Reader) (*PaymentWinnerCacheEntry, error) {
	winner := &PaymentWinnerCacheEntry{}

	// vinMasternode as CTxIn
	// Outpoint hash
	if _, err := io.ReadFull(r, winner.VoterOutpoint.Hash[:]); err != nil {
		return nil, err
	}
	// Outpoint index
	if err := binary.Read(r, binary.LittleEndian, &winner.VoterOutpoint.Index); err != nil {
		return nil, err
	}
	// scriptSig (skip)
	sigLen, err := readCompactSize(r)
	if err != nil {
		return nil, err
	}
	if sigLen > 0 {
		// Skip scriptSig bytes
		skip := make([]byte, sigLen)
		if _, err := io.ReadFull(r, skip); err != nil {
			return nil, err
		}
	}
	// nSequence (skip)
	var seq uint32
	if err := binary.Read(r, binary.LittleEndian, &seq); err != nil {
		return nil, err
	}

	// nBlockHeight
	var height int32
	if err := binary.Read(r, binary.LittleEndian, &height); err != nil {
		return nil, err
	}
	winner.BlockHeight = uint32(height)

	// payee (CScript as varbytes)
	payeeLen, err := readCompactSize(r)
	if err != nil {
		return nil, err
	}
	if payeeLen > maxPayeeScriptSize {
		return nil, fmt.Errorf("payee script too long: %d", payeeLen)
	}
	if payeeLen > 0 {
		winner.PayeeScript = make([]byte, payeeLen)
		if _, err := io.ReadFull(r, winner.PayeeScript); err != nil {
			return nil, err
		}
	}

	// vchSig (signature as varbytes)
	sigLen, err = readCompactSize(r)
	if err != nil {
		return nil, err
	}
	if sigLen > maxPayeeSignatureSize {
		return nil, fmt.Errorf("signature too long: %d", sigLen)
	}
	if sigLen > 0 {
		winner.Signature = make([]byte, sigLen)
		if _, err := io.ReadFull(r, winner.Signature); err != nil {
			return nil, err
		}
	}

	return winner, nil
}

// serializeBlockPayees serializes CMasternodeBlockPayees format
// Legacy format:
//   - nBlockHeight (int32)
//   - vecPayments (vector of CMasternodePayee)
func serializeBlockPayees(w io.Writer, bp *BlockPayeesCacheEntry) error {
	// nBlockHeight
	if err := binary.Write(w, binary.LittleEndian, int32(bp.BlockHeight)); err != nil {
		return err
	}

	// vecPayments
	if err := writeCompactSize(w, uint64(len(bp.Payees))); err != nil {
		return err
	}
	for _, payee := range bp.Payees {
		if err := serializePayee(w, payee); err != nil {
			return err
		}
	}

	return nil
}

// deserializeBlockPayees deserializes CMasternodeBlockPayees format
func deserializeBlockPayees(r io.Reader) (*BlockPayeesCacheEntry, error) {
	bp := &BlockPayeesCacheEntry{}

	// nBlockHeight
	var height int32
	if err := binary.Read(r, binary.LittleEndian, &height); err != nil {
		return nil, err
	}
	bp.BlockHeight = uint32(height)

	// vecPayments
	payeeCount, err := readCompactSize(r)
	if err != nil {
		return nil, err
	}
	if payeeCount > maxPaymentCachePayees {
		return nil, fmt.Errorf("too many payees: %d", payeeCount)
	}

	bp.Payees = make([]*PayeeCacheEntry, 0, payeeCount)
	for i := uint64(0); i < payeeCount; i++ {
		payee, err := deserializePayee(r)
		if err != nil {
			return nil, fmt.Errorf("failed to read payee %d: %w", i, err)
		}
		bp.Payees = append(bp.Payees, payee)
	}

	return bp, nil
}

// serializePayee serializes CMasternodePayee format
// Legacy format:
//   - scriptPubKey (CScript as varbytes)
//   - nVotes (int32)
func serializePayee(w io.Writer, payee *PayeeCacheEntry) error {
	// scriptPubKey
	if err := writeCompactSize(w, uint64(len(payee.ScriptPubKey))); err != nil {
		return err
	}
	if _, err := w.Write(payee.ScriptPubKey); err != nil {
		return err
	}

	// nVotes
	if err := binary.Write(w, binary.LittleEndian, payee.Votes); err != nil {
		return err
	}

	return nil
}

// deserializePayee deserializes CMasternodePayee format
func deserializePayee(r io.Reader) (*PayeeCacheEntry, error) {
	payee := &PayeeCacheEntry{}

	// scriptPubKey
	scriptLen, err := readCompactSize(r)
	if err != nil {
		return nil, err
	}
	if scriptLen > maxPayeeScriptSize {
		return nil, fmt.Errorf("script too long: %d", scriptLen)
	}
	if scriptLen > 0 {
		payee.ScriptPubKey = make([]byte, scriptLen)
		if _, err := io.ReadFull(r, payee.ScriptPubKey); err != nil {
			return nil, err
		}
	}

	// nVotes
	if err := binary.Read(r, binary.LittleEndian, &payee.Votes); err != nil {
		return nil, err
	}

	return payee, nil
}
