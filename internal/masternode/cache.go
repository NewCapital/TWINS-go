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
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/twins-dev/twins-core/pkg/crypto"
	"github.com/twins-dev/twins-core/pkg/types"
)

const (
	// CacheMagicMessage is the legacy magic message for mncache.dat
	CacheMagicMessage = "MasternodeCache"

	// CacheVersion is the current cache format version
	CacheVersion = 1

	// Deserialization sanity limits to prevent DoS from malicious cache files
	maxVarStringLength        = 1000000 // Max string length in readVarString
	maxMasternodeCacheEntries = 100000  // Max masternodes in cache (network has ~hundreds)
)

var (
	// ErrCacheFileNotFound indicates the cache file doesn't exist
	ErrCacheFileNotFound = errors.New("mncache.dat not found")

	// ErrCacheCorrupted indicates the cache file has invalid checksum
	ErrCacheCorrupted = errors.New("mncache.dat corrupted: checksum mismatch")

	// ErrCacheInvalidMagic indicates wrong magic message
	ErrCacheInvalidMagic = errors.New("mncache.dat invalid: wrong magic message")

	// ErrCacheInvalidNetwork indicates wrong network magic
	ErrCacheInvalidNetwork = errors.New("mncache.dat invalid: wrong network magic")
)

// CacheData represents the serializable masternode cache data
// LEGACY COMPATIBILITY: Matches CMasternodeMan serialization format
type CacheData struct {
	// Masternodes list (matches vMasternodes in C++)
	Masternodes []*MasternodeCacheEntry

	// Request tracking maps (serialized for legacy compatibility but not used)
	AskedUsForList  map[string]int64 // mAskedUsForMasternodeList (CNetAddr -> timestamp)
	WeAskedForList  map[string]int64 // mWeAskedForMasternodeList
	WeAskedForEntry map[string]int64 // mWeAskedForMasternodeListEntry (outpoint string -> timestamp)
	DsqCount        int64            // nDsqCount (obfuscation queue, unused but serialized)

	// Seen broadcasts and pings (serialized for relay deduplication)
	SeenBroadcasts map[types.Hash]*MasternodeBroadcast
	SeenPings      map[types.Hash]int64 // ping hash -> sigTime
}

// MasternodeCacheEntry represents a serializable masternode entry
// LEGACY COMPATIBILITY: Matches CMasternode::SerializationOp() from masternode.h:217-237
// Serialization order MUST match exactly:
//  1. vin (CTxIn)
//  2. addr (CService)
//  3. pubKeyCollateralAddress (CPubKey)
//  4. pubKeyMasternode (CPubKey)
//  5. sig (std::vector<unsigned char>)
//  6. sigTime (int64_t)
//  7. protocolVersion (int)
//  8. activeState (int)
//  9. lastPing (CMasternodePing)
//  10. cacheInputAge (int)
//  11. cacheInputAgeBlock (int)
//  12. unitTest (bool)
//  13. allowFreeTx (bool)
//  14. nLastDsq (int64_t)
//  15. nScanningErrorCount (int)
//  16. nLastScanningErrorBlockHeight (int)
type MasternodeCacheEntry struct {
	// Core identity (1-5)
	OutPoint         types.Outpoint
	Addr             string // Service address as string (CService serializes as IP:port)
	PubKeyCollateral []byte // Compressed public key bytes
	PubKeyMasternode []byte // Compressed public key bytes
	Signature        []byte

	// Timing (6-7)
	SigTime  int64
	Protocol int32

	// Status (8)
	Status MasternodeStatus // activeState in legacy

	// Ping (9)
	LastPingMessage *MasternodePingCacheEntry

	// Cache fields (10-11)
	CacheInputAge      int
	CacheInputAgeBlock int32

	// Legacy bool fields (12-13)
	UnitTest    bool
	AllowFreeTx bool

	// Legacy tracking fields (14-16)
	LastDsq                      int64
	ScanningErrorCount           int
	LastScanningErrorBlockHeight int32

	// Go-specific fields (NOT serialized to legacy format, stored separately)
	// These are computed/derived fields that don't exist in legacy CMasternode
	BlockHeight int32          // Computed from cacheInputAgeBlock
	Tier        MasternodeTier // Computed from collateral amount
	Collateral  int64          // Computed from UTXO lookup
	LastPaid    int64          // Tracked separately in Go
}

// MasternodePingCacheEntry represents a serializable ping entry
type MasternodePingCacheEntry struct {
	Outpoint      types.Outpoint
	BlockHash     types.Hash
	SignatureTime int64
	Signature     []byte
}

// SaveCache saves the masternode cache to mncache.dat
// LEGACY COMPATIBILITY: Uses same format as C++ CMasternodeDB::Write
func (m *Manager) SaveCache(dataDir string, networkMagic []byte) error {
	if len(networkMagic) != 4 {
		return fmt.Errorf("network magic must be 4 bytes, got %d", len(networkMagic))
	}

	startTime := time.Now()
	m.mu.RLock()

	// Build cache data from manager state
	data := &CacheData{
		Masternodes:     make([]*MasternodeCacheEntry, 0, len(m.masternodes)),
		AskedUsForList:  make(map[string]int64),
		WeAskedForList:  make(map[string]int64),
		WeAskedForEntry: make(map[string]int64),
		DsqCount:        0,
		SeenBroadcasts:  make(map[types.Hash]*MasternodeBroadcast),
		SeenPings:       make(map[types.Hash]int64),
	}

	// Convert masternodes to cache entries
	// LEGACY COMPATIBILITY: Include all fields required by CMasternode::SerializationOp()
	for _, mn := range m.masternodes {
		// Status demotion: ENABLED -> PRE_ENABLED
		// This ensures masternodes must receive a ping after cache load
		// to prove they're still active (matches legacy CheckAndRemove behavior)
		status := mn.Status
		if status == StatusEnabled {
			status = StatusPreEnabled
		}

		entry := &MasternodeCacheEntry{
			// Core identity
			OutPoint:  mn.OutPoint,
			Signature: mn.Signature,

			// Timing
			SigTime:  mn.SigTime,
			Protocol: mn.Protocol,

			// Status (demoted for ENABLED masternodes)
			Status: status,

			// Cache fields (legacy compatibility)
			CacheInputAge:      mn.CacheInputAge,
			CacheInputAgeBlock: mn.CacheInputAgeBlock,

			// Legacy bool fields
			UnitTest:    mn.UnitTest,
			AllowFreeTx: mn.AllowFreeTx,

			// Legacy tracking fields
			LastDsq:                      mn.LastDsq,
			ScanningErrorCount:           mn.ScanningErrorCount,
			LastScanningErrorBlockHeight: mn.LastScanningErrorBlockHeight,

			// Go-specific fields (for internal use, not in legacy serialization)
			BlockHeight: mn.BlockHeight,
			Tier:        mn.Tier,
			Collateral:  mn.Collateral,
			LastPaid:    mn.LastPaid.Unix(),
		}

		// Service address
		if mn.Addr != nil {
			entry.Addr = mn.Addr.String()
		}

		// Serialize public keys
		if mn.PubKeyCollateral != nil {
			entry.PubKeyCollateral = mn.PubKeyCollateral.SerializeCompressed()
		}
		if mn.PubKey != nil {
			entry.PubKeyMasternode = mn.PubKey.SerializeCompressed()
		}

		// Convert last ping
		if mn.LastPingMessage != nil {
			entry.LastPingMessage = &MasternodePingCacheEntry{
				Outpoint:      mn.LastPingMessage.OutPoint,
				BlockHash:     mn.LastPingMessage.BlockHash,
				SignatureTime: mn.LastPingMessage.SigTime,
				Signature:     mn.LastPingMessage.Signature,
			}
		}

		data.Masternodes = append(data.Masternodes, entry)
	}

	// Copy seen pings
	m.seenPingsMu.RLock()
	for hash, sigTime := range m.seenPings {
		data.SeenPings[hash] = sigTime
	}
	m.seenPingsMu.RUnlock()

	m.mu.RUnlock()

	// Populate fulfilled request maps from sync manager (outside Manager.mu to avoid lock ordering)
	if m.syncManager != nil {
		mnsync, mnwsync := m.syncManager.GetFulfilledMaps()
		data.WeAskedForList = mnsync
		data.WeAskedForEntry = mnwsync
	}

	// Serialize the cache data
	var buf bytes.Buffer

	if err := writeCacheHeader(&buf, CacheMagicMessage, networkMagic); err != nil {
		return err
	}

	if err := serializeCacheData(&buf, data); err != nil {
		return fmt.Errorf("failed to serialize cache data: %w", err)
	}

	hash := calculateSHA256d(buf.Bytes())

	// Write to file
	cachePath := filepath.Join(dataDir, "mncache.dat")
	file, err := os.Create(cachePath)
	if err != nil {
		return fmt.Errorf("failed to create mncache.dat: %w", err)
	}
	defer file.Close()

	// Write data
	if _, err := file.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("failed to write cache data: %w", err)
	}

	// Write checksum
	if _, err := file.Write(hash[:]); err != nil {
		return fmt.Errorf("failed to write checksum: %w", err)
	}

	// Flush to disk before closing to prevent data loss on abrupt exit
	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync mncache.dat: %w", err)
	}

	if m.logger != nil {
		m.logger.WithField("duration_ms", time.Since(startTime).Milliseconds()).
			WithField("masternode_count", len(data.Masternodes)).
			Info("Saved masternode cache to mncache.dat")
	}

	return nil
}

// LoadCache loads the masternode cache from mncache.dat
// LEGACY COMPATIBILITY: Uses same format as C++ CMasternodeDB::Read
func (m *Manager) LoadCache(dataDir string, networkMagic []byte) error {
	if len(networkMagic) != 4 {
		return fmt.Errorf("network magic must be 4 bytes, got %d", len(networkMagic))
	}

	startTime := time.Now()
	cachePath := filepath.Join(dataDir, "mncache.dat")

	// Open file
	file, err := os.Open(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrCacheFileNotFound
		}
		return fmt.Errorf("failed to open mncache.dat: %w", err)
	}
	defer file.Close()

	// Get file size
	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat mncache.dat: %w", err)
	}

	// Read all data except checksum (last 32 bytes)
	if stat.Size() < 32 {
		return ErrCacheCorrupted
	}
	dataSize := stat.Size() - 32

	data := make([]byte, dataSize)
	if _, err := io.ReadFull(file, data); err != nil {
		return fmt.Errorf("failed to read cache data: %w", err)
	}

	// Read checksum
	var storedHash [32]byte
	if _, err := io.ReadFull(file, storedHash[:]); err != nil {
		return fmt.Errorf("failed to read checksum: %w", err)
	}

	if !verifySHA256d(data, storedHash) {
		return ErrCacheCorrupted
	}

	buf := bytes.NewReader(data)

	if err := readCacheHeader(buf, CacheMagicMessage, networkMagic, ErrCacheInvalidMagic, ErrCacheInvalidNetwork); err != nil {
		return err
	}

	// Deserialize cache data
	cacheData, err := deserializeCacheData(buf)
	if err != nil {
		return fmt.Errorf("failed to deserialize cache data: %w", err)
	}

	// Apply to manager
	m.mu.Lock()
	defer m.mu.Unlock()

	loadedCount := 0
	for _, entry := range cacheData.Masternodes {
		mn, err := cacheEntryToMasternode(entry)
		if err != nil {
			if m.logger != nil {
				m.logger.WithError(err).Warn("Failed to restore masternode from cache")
			}
			continue
		}

		// Add to maps (without full validation, will be validated on sync)
		m.masternodes[mn.OutPoint] = mn
		if mn.Addr != nil {
			m.addressIndex[mn.Addr.String()] = mn
		}
		if mn.PubKey != nil {
			m.pubkeyIndex[mn.PubKey.Hex()] = mn
		}
		if mn.LastPingMessage != nil {
			m.seenPingMessages[mn.LastPingMessage.GetHash()] = mn.LastPingMessage
		}
		loadedCount++
	}

	// NOTE: We intentionally do NOT recalculate status here.
	// Status demotion on save (ENABLED -> PRE_ENABLED) ensures masternodes
	// must receive a fresh ping from the network to prove they're still active.
	// UpdateStatus() will be called naturally when new pings are processed.

	// Restore seen pings
	m.seenPingsMu.Lock()
	for hash, sigTime := range cacheData.SeenPings {
		m.seenPings[hash] = sigTime
	}
	// Keep dedup entries only for hashes we can actually serve via getdata.
	// Legacy stores full mapSeenMasternodePing payloads; cache format keeps only sigTime.
	// Without this pruning, we'd report "seen" but return notfound for missing payloads.
	for hash := range m.seenPings {
		if _, hasPayload := m.seenPingMessages[hash]; !hasPayload {
			delete(m.seenPings, hash)
		}
	}
	m.seenPingsMu.Unlock()

	// Store fulfilled request maps for later push to SyncManager
	// (pushed in startup code after SyncManager is wired)
	m.cachedFulfilledMNSync = cacheData.WeAskedForList
	m.cachedFulfilledMNWSync = cacheData.WeAskedForEntry

	// Record cache metadata for quick-restart sync skip
	// Use file modification time as cache freshness indicator
	m.cacheLoadedAt = stat.ModTime()
	m.cacheLoadedCount = loadedCount

	if m.logger != nil {
		m.logger.WithField("duration_ms", time.Since(startTime).Milliseconds()).
			WithField("loaded_count", loadedCount).
			WithField("total_entries", len(cacheData.Masternodes)).
			WithField("cache_age", time.Since(stat.ModTime()).Round(time.Second)).
			Info("Loaded masternode cache from mncache.dat")
	}

	return nil
}

// Helper functions for serialization

func writeVarString(w io.Writer, s string) error {
	// Write compact size length
	if err := writeCompactSize(w, uint64(len(s))); err != nil {
		return err
	}
	// Write string bytes
	_, err := w.Write([]byte(s))
	return err
}

func readVarString(r io.Reader) (string, error) {
	length, err := readCompactSize(r)
	if err != nil {
		return "", err
	}
	if length > maxVarStringLength {
		return "", fmt.Errorf("string too long: %d", length)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return "", err
	}
	return string(data), nil
}

func writeCompactSize(w io.Writer, n uint64) error {
	if n < 253 {
		return binary.Write(w, binary.LittleEndian, uint8(n))
	} else if n <= 0xFFFF {
		if err := binary.Write(w, binary.LittleEndian, uint8(253)); err != nil {
			return err
		}
		return binary.Write(w, binary.LittleEndian, uint16(n))
	} else if n <= 0xFFFFFFFF {
		if err := binary.Write(w, binary.LittleEndian, uint8(254)); err != nil {
			return err
		}
		return binary.Write(w, binary.LittleEndian, uint32(n))
	}
	if err := binary.Write(w, binary.LittleEndian, uint8(255)); err != nil {
		return err
	}
	return binary.Write(w, binary.LittleEndian, n)
}

func readCompactSize(r io.Reader) (uint64, error) {
	var first uint8
	if err := binary.Read(r, binary.LittleEndian, &first); err != nil {
		return 0, err
	}
	if first < 253 {
		return uint64(first), nil
	} else if first == 253 {
		var n uint16
		if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
			return 0, err
		}
		return uint64(n), nil
	} else if first == 254 {
		var n uint32
		if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
			return 0, err
		}
		return uint64(n), nil
	}
	var n uint64
	if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
		return 0, err
	}
	return n, nil
}

func serializeCacheData(w io.Writer, data *CacheData) error {
	// Write masternode count
	if err := writeCompactSize(w, uint64(len(data.Masternodes))); err != nil {
		return err
	}

	// Write each masternode
	for _, entry := range data.Masternodes {
		if err := serializeMasternodeEntry(w, entry); err != nil {
			return err
		}
	}

	// AskedUsForList - always empty (we don't track what peers asked us)
	if err := writeCompactSize(w, 0); err != nil {
		return err
	}
	// WeAskedForList - persisted fulfilled mnsync tracking
	if err := writeCompactSize(w, uint64(len(data.WeAskedForList))); err != nil {
		return err
	}
	for key, ts := range data.WeAskedForList {
		if err := writeVarString(w, key); err != nil {
			return err
		}
		if err := binary.Write(w, binary.LittleEndian, ts); err != nil {
			return err
		}
	}
	// WeAskedForEntry - persisted fulfilled mnwsync tracking
	if err := writeCompactSize(w, uint64(len(data.WeAskedForEntry))); err != nil {
		return err
	}
	for key, ts := range data.WeAskedForEntry {
		if err := writeVarString(w, key); err != nil {
			return err
		}
		if err := binary.Write(w, binary.LittleEndian, ts); err != nil {
			return err
		}
	}
	// DsqCount
	if err := binary.Write(w, binary.LittleEndian, data.DsqCount); err != nil {
		return err
	}

	// Write empty SeenBroadcasts map (simplified)
	if err := writeCompactSize(w, 0); err != nil {
		return err
	}

	// Write SeenPings map
	if err := writeCompactSize(w, uint64(len(data.SeenPings))); err != nil {
		return err
	}
	for hash, sigTime := range data.SeenPings {
		if _, err := w.Write(hash[:]); err != nil {
			return err
		}
		if err := binary.Write(w, binary.LittleEndian, sigTime); err != nil {
			return err
		}
	}

	return nil
}

func deserializeCacheData(r io.Reader) (*CacheData, error) {
	data := &CacheData{
		AskedUsForList:  make(map[string]int64),
		WeAskedForList:  make(map[string]int64),
		WeAskedForEntry: make(map[string]int64),
		SeenBroadcasts:  make(map[types.Hash]*MasternodeBroadcast),
		SeenPings:       make(map[types.Hash]int64),
	}

	// Read masternode count
	count, err := readCompactSize(r)
	if err != nil {
		return nil, err
	}

	if count > maxMasternodeCacheEntries {
		return nil, fmt.Errorf("too many masternodes in cache: %d", count)
	}

	// Read each masternode
	data.Masternodes = make([]*MasternodeCacheEntry, 0, count)
	for i := uint64(0); i < count; i++ {
		entry, err := deserializeMasternodeEntry(r)
		if err != nil {
			return nil, fmt.Errorf("failed to read masternode %d: %w", i, err)
		}
		data.Masternodes = append(data.Masternodes, entry)
	}

	// AskedUsForList - skip (we don't use server-side tracking)
	if mapLen, err := readCompactSize(r); err != nil {
		return nil, err
	} else if mapLen > 0 {
		for i := uint64(0); i < mapLen; i++ {
			if _, err := readVarString(r); err != nil {
				return nil, err
			}
			var ts int64
			if err := binary.Read(r, binary.LittleEndian, &ts); err != nil {
				return nil, err
			}
		}
	}

	// WeAskedForList - restore fulfilled mnsync tracking
	if mapLen, err := readCompactSize(r); err != nil {
		return nil, err
	} else if mapLen > 0 {
		for i := uint64(0); i < mapLen; i++ {
			key, err := readVarString(r)
			if err != nil {
				return nil, err
			}
			var ts int64
			if err := binary.Read(r, binary.LittleEndian, &ts); err != nil {
				return nil, err
			}
			data.WeAskedForList[key] = ts
		}
	}

	// WeAskedForEntry - restore fulfilled mnwsync tracking
	if mapLen, err := readCompactSize(r); err != nil {
		return nil, err
	} else if mapLen > 0 {
		for i := uint64(0); i < mapLen; i++ {
			key, err := readVarString(r)
			if err != nil {
				return nil, err
			}
			var ts int64
			if err := binary.Read(r, binary.LittleEndian, &ts); err != nil {
				return nil, err
			}
			data.WeAskedForEntry[key] = ts
		}
	}

	// DsqCount
	if err := binary.Read(r, binary.LittleEndian, &data.DsqCount); err != nil {
		return nil, err
	}

	// SeenBroadcasts (skip)
	if mapLen, err := readCompactSize(r); err != nil {
		return nil, err
	} else if mapLen > 0 {
		// Skip broadcast entries (complex, just skip bytes)
		// Note: This is simplified - real impl would need full broadcast deserialization
	}

	// SeenPings
	if mapLen, err := readCompactSize(r); err != nil {
		return nil, err
	} else {
		for i := uint64(0); i < mapLen; i++ {
			var hash types.Hash
			if _, err := io.ReadFull(r, hash[:]); err != nil {
				return nil, err
			}
			var sigTime int64
			if err := binary.Read(r, binary.LittleEndian, &sigTime); err != nil {
				return nil, err
			}
			data.SeenPings[hash] = sigTime
		}
	}

	return data, nil
}

// serializeMasternodeEntry serializes a masternode entry in LEGACY C++ compatible format.
// CRITICAL: Order and format MUST match CMasternode::SerializationOp() exactly!
// Legacy order from masternode.h:217-237:
//  1. vin (CTxIn: prevout + scriptSig + nSequence)
//  2. addr (CService: IP bytes + port)
//  3. pubKeyCollateralAddress (CPubKey as varbytes)
//  4. pubKeyMasternode (CPubKey as varbytes)
//  5. sig (std::vector<unsigned char> as varbytes)
//  6. sigTime (int64_t)
//  7. protocolVersion (int32)
//  8. activeState (int32)
//  9. lastPing (CMasternodePing - always serialized, empty if none)
//  10. cacheInputAge (int32)
//  11. cacheInputAgeBlock (int32)
//  12. unitTest (bool as char)
//  13. allowFreeTx (bool as char)
//  14. nLastDsq (int64_t)
//  15. nScanningErrorCount (int32)
//  16. nLastScanningErrorBlockHeight (int32)
func serializeMasternodeEntry(w io.Writer, entry *MasternodeCacheEntry) error {
	// 1. vin (CTxIn format: COutPoint + scriptSig + nSequence)
	// COutPoint: hash (32 bytes) + index (uint32)
	if _, err := w.Write(entry.OutPoint.Hash[:]); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, entry.OutPoint.Index); err != nil {
		return err
	}
	// scriptSig (empty for masternode collateral)
	if err := writeCompactSize(w, 0); err != nil {
		return err
	}
	// nSequence (0xFFFFFFFF for final)
	if err := binary.Write(w, binary.LittleEndian, uint32(0xFFFFFFFF)); err != nil {
		return err
	}

	// 2. addr (CService format)
	// Legacy CService serializes as: IP (16 bytes for IPv6/mapped IPv4) + port (2 bytes big-endian)
	if err := serializeCService(w, entry.Addr); err != nil {
		return err
	}

	// 3. pubKeyCollateralAddress (CPubKey as varbytes)
	if err := writeCompactSize(w, uint64(len(entry.PubKeyCollateral))); err != nil {
		return err
	}
	if _, err := w.Write(entry.PubKeyCollateral); err != nil {
		return err
	}

	// 4. pubKeyMasternode (CPubKey as varbytes)
	if err := writeCompactSize(w, uint64(len(entry.PubKeyMasternode))); err != nil {
		return err
	}
	if _, err := w.Write(entry.PubKeyMasternode); err != nil {
		return err
	}

	// 5. sig (std::vector<unsigned char> as varbytes)
	if err := writeCompactSize(w, uint64(len(entry.Signature))); err != nil {
		return err
	}
	if _, err := w.Write(entry.Signature); err != nil {
		return err
	}

	// 6. sigTime (int64_t)
	if err := binary.Write(w, binary.LittleEndian, entry.SigTime); err != nil {
		return err
	}

	// 7. protocolVersion (int32)
	if err := binary.Write(w, binary.LittleEndian, entry.Protocol); err != nil {
		return err
	}

	// 8. activeState (int32) - clamp to legacy range (0-8)
	status := int32(entry.Status)
	if status > int32(StatusPosError) {
		status = int32(StatusPosError)
	}
	if err := binary.Write(w, binary.LittleEndian, status); err != nil {
		return err
	}

	// 9. lastPing (CMasternodePing - ALWAYS serialized, empty values if nil)
	if err := serializePingEntryLegacy(w, entry.LastPingMessage); err != nil {
		return err
	}

	// 10. cacheInputAge (int32)
	if err := binary.Write(w, binary.LittleEndian, int32(entry.CacheInputAge)); err != nil {
		return err
	}

	// 11. cacheInputAgeBlock (int32)
	if err := binary.Write(w, binary.LittleEndian, entry.CacheInputAgeBlock); err != nil {
		return err
	}

	// 12. unitTest (bool as single byte: 0 or 1)
	unitTestByte := byte(0)
	if entry.UnitTest {
		unitTestByte = 1
	}
	if _, err := w.Write([]byte{unitTestByte}); err != nil {
		return err
	}

	// 13. allowFreeTx (bool as single byte)
	allowFreeTxByte := byte(0)
	if entry.AllowFreeTx {
		allowFreeTxByte = 1
	}
	if _, err := w.Write([]byte{allowFreeTxByte}); err != nil {
		return err
	}

	// 14. nLastDsq (int64_t)
	if err := binary.Write(w, binary.LittleEndian, entry.LastDsq); err != nil {
		return err
	}

	// 15. nScanningErrorCount (int32)
	if err := binary.Write(w, binary.LittleEndian, int32(entry.ScanningErrorCount)); err != nil {
		return err
	}

	// 16. nLastScanningErrorBlockHeight (int32)
	if err := binary.Write(w, binary.LittleEndian, entry.LastScanningErrorBlockHeight); err != nil {
		return err
	}

	// 17. Collateral (int64) - Go extension for tier calculation
	// Written AFTER legacy fields to maintain backwards compatibility
	if err := binary.Write(w, binary.LittleEndian, entry.Collateral); err != nil {
		return err
	}

	return nil
}

// serializeCService writes a CService (IP:port) in legacy format
// Legacy format: 16 bytes IP (IPv6 or IPv4-mapped) + 2 bytes port (big-endian!)
func serializeCService(w io.Writer, addrStr string) error {
	// Parse address string
	host, portStr, err := net.SplitHostPort(addrStr)
	if err != nil {
		// If no port, assume default and use whole string as host
		host = addrStr
		portStr = "37817" // Default mainnet port
	}

	// Parse IP
	ip := net.ParseIP(host)
	if ip == nil {
		ip = net.IPv4zero
	}

	// Convert to 16-byte format (IPv6 or IPv4-mapped-to-IPv6)
	ip16 := ip.To16()
	if ip16 == nil {
		ip16 = make([]byte, 16)
	}
	if _, err := w.Write(ip16); err != nil {
		return err
	}

	// Parse and write port (big-endian, as per CService serialization)
	var port uint16
	fmt.Sscanf(portStr, "%d", &port)
	if err := binary.Write(w, binary.BigEndian, port); err != nil {
		return err
	}

	return nil
}

// serializePingEntryLegacy serializes a ping in LEGACY format
// CRITICAL: Always serializes all fields, even if ping is nil (uses zero values)
// Legacy CMasternodePing format from masternode.h:54-61:
//  1. vin (CTxIn)
//  2. blockHash (uint256)
//  3. sigTime (int64_t)
//  4. vchSig (std::vector<unsigned char>)
func serializePingEntryLegacy(w io.Writer, ping *MasternodePingCacheEntry) error {
	if ping == nil {
		// Write empty/zero ping (legacy always serializes, never optional)
		// Empty CTxIn: zero hash + 0 index + empty scriptSig + max sequence
		if _, err := w.Write(make([]byte, 32)); err != nil { // zero hash
			return err
		}
		if err := binary.Write(w, binary.LittleEndian, uint32(0)); err != nil { // index
			return err
		}
		if err := writeCompactSize(w, 0); err != nil { // empty scriptSig
			return err
		}
		if err := binary.Write(w, binary.LittleEndian, uint32(0xFFFFFFFF)); err != nil { // sequence
			return err
		}
		// Zero blockHash
		if _, err := w.Write(make([]byte, 32)); err != nil {
			return err
		}
		// Zero sigTime
		if err := binary.Write(w, binary.LittleEndian, int64(0)); err != nil {
			return err
		}
		// Empty signature
		if err := writeCompactSize(w, 0); err != nil {
			return err
		}
		return nil
	}

	// 1. vin (CTxIn)
	if _, err := w.Write(ping.Outpoint.Hash[:]); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, ping.Outpoint.Index); err != nil {
		return err
	}
	if err := writeCompactSize(w, 0); err != nil { // empty scriptSig
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(0xFFFFFFFF)); err != nil { // sequence
		return err
	}

	// 2. blockHash
	if _, err := w.Write(ping.BlockHash[:]); err != nil {
		return err
	}

	// 3. sigTime
	if err := binary.Write(w, binary.LittleEndian, ping.SignatureTime); err != nil {
		return err
	}

	// 4. vchSig
	if err := writeCompactSize(w, uint64(len(ping.Signature))); err != nil {
		return err
	}
	if _, err := w.Write(ping.Signature); err != nil {
		return err
	}

	return nil
}

// deserializeMasternodeEntry deserializes a masternode entry from LEGACY C++ format.
// CRITICAL: Order and format MUST match CMasternode::SerializationOp() exactly!
func deserializeMasternodeEntry(r io.Reader) (*MasternodeCacheEntry, error) {
	entry := &MasternodeCacheEntry{}

	// 1. vin (CTxIn format: COutPoint + scriptSig + nSequence)
	// COutPoint: hash (32 bytes) + index (uint32)
	if _, err := io.ReadFull(r, entry.OutPoint.Hash[:]); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &entry.OutPoint.Index); err != nil {
		return nil, err
	}
	// Skip scriptSig (varbytes)
	scriptSigLen, err := readCompactSize(r)
	if err != nil {
		return nil, err
	}
	if scriptSigLen > 0 {
		// Discard scriptSig bytes
		discard := make([]byte, scriptSigLen)
		if _, err := io.ReadFull(r, discard); err != nil {
			return nil, err
		}
	}
	// Skip nSequence (uint32)
	var sequence uint32
	if err := binary.Read(r, binary.LittleEndian, &sequence); err != nil {
		return nil, err
	}

	// 2. addr (CService format: 16 bytes IP + 2 bytes port big-endian)
	entry.Addr, err = deserializeCService(r)
	if err != nil {
		return nil, err
	}

	// 3. pubKeyCollateralAddress (CPubKey as varbytes)
	pkLen, err := readCompactSize(r)
	if err != nil {
		return nil, err
	}
	if pkLen > 0 {
		entry.PubKeyCollateral = make([]byte, pkLen)
		if _, err := io.ReadFull(r, entry.PubKeyCollateral); err != nil {
			return nil, err
		}
	}

	// 4. pubKeyMasternode (CPubKey as varbytes)
	pkLen, err = readCompactSize(r)
	if err != nil {
		return nil, err
	}
	if pkLen > 0 {
		entry.PubKeyMasternode = make([]byte, pkLen)
		if _, err := io.ReadFull(r, entry.PubKeyMasternode); err != nil {
			return nil, err
		}
	}

	// 5. sig (std::vector<unsigned char> as varbytes)
	sigLen, err := readCompactSize(r)
	if err != nil {
		return nil, err
	}
	if sigLen > 0 {
		entry.Signature = make([]byte, sigLen)
		if _, err := io.ReadFull(r, entry.Signature); err != nil {
			return nil, err
		}
	}

	// 6. sigTime (int64_t)
	if err := binary.Read(r, binary.LittleEndian, &entry.SigTime); err != nil {
		return nil, err
	}

	// 7. protocolVersion (int32)
	if err := binary.Read(r, binary.LittleEndian, &entry.Protocol); err != nil {
		return nil, err
	}

	// 8. activeState (int32)
	var status int32
	if err := binary.Read(r, binary.LittleEndian, &status); err != nil {
		return nil, err
	}
	entry.Status = MasternodeStatus(status)

	// 9. lastPing (CMasternodePing - ALWAYS present in legacy format)
	entry.LastPingMessage, err = deserializePingEntryLegacy(r)
	if err != nil {
		return nil, err
	}

	// 10. cacheInputAge (int32)
	var cacheInputAge int32
	if err := binary.Read(r, binary.LittleEndian, &cacheInputAge); err != nil {
		return nil, err
	}
	entry.CacheInputAge = int(cacheInputAge)

	// 11. cacheInputAgeBlock (int32)
	if err := binary.Read(r, binary.LittleEndian, &entry.CacheInputAgeBlock); err != nil {
		return nil, err
	}

	// 12. unitTest (bool as single byte)
	var unitTestByte byte
	if err := binary.Read(r, binary.LittleEndian, &unitTestByte); err != nil {
		return nil, err
	}
	entry.UnitTest = unitTestByte != 0

	// 13. allowFreeTx (bool as single byte)
	var allowFreeTxByte byte
	if err := binary.Read(r, binary.LittleEndian, &allowFreeTxByte); err != nil {
		return nil, err
	}
	entry.AllowFreeTx = allowFreeTxByte != 0

	// 14. nLastDsq (int64_t)
	if err := binary.Read(r, binary.LittleEndian, &entry.LastDsq); err != nil {
		return nil, err
	}

	// 15. nScanningErrorCount (int32)
	var scanErrorCount int32
	if err := binary.Read(r, binary.LittleEndian, &scanErrorCount); err != nil {
		return nil, err
	}
	entry.ScanningErrorCount = int(scanErrorCount)

	// 16. nLastScanningErrorBlockHeight (int32)
	if err := binary.Read(r, binary.LittleEndian, &entry.LastScanningErrorBlockHeight); err != nil {
		return nil, err
	}

	// 17. Collateral (int64) - Go extension for tier calculation
	// Backwards compatible: if EOF, old cache format without collateral
	if err := binary.Read(r, binary.LittleEndian, &entry.Collateral); err != nil {
		// Old cache format - collateral not stored, will be 0
		entry.Collateral = 0
	}

	// Derive Go-specific fields from legacy data
	entry.BlockHeight = entry.CacheInputAgeBlock

	// Derive Tier from Collateral (if available)
	if entry.Collateral > 0 {
		entry.Tier, _ = GetTierFromCollateral(entry.Collateral)
	}

	return entry, nil
}

// deserializeCService reads a CService (IP:port) from legacy format
// Legacy format: 16 bytes IP (IPv6 or IPv4-mapped) + 2 bytes port (big-endian!)
func deserializeCService(r io.Reader) (string, error) {
	// Read 16 bytes IP
	ip16 := make([]byte, 16)
	if _, err := io.ReadFull(r, ip16); err != nil {
		return "", err
	}

	// Read 2 bytes port (big-endian)
	var port uint16
	if err := binary.Read(r, binary.BigEndian, &port); err != nil {
		return "", err
	}

	// Convert IP bytes to string
	ip := net.IP(ip16)

	// Check if it's an IPv4-mapped IPv6 address
	if ip4 := ip.To4(); ip4 != nil {
		return fmt.Sprintf("%s:%d", ip4.String(), port), nil
	}

	// IPv6 address
	return fmt.Sprintf("[%s]:%d", ip.String(), port), nil
}

// deserializePingEntryLegacy deserializes a ping from LEGACY format
// CRITICAL: Always reads all fields (legacy format has no optional flag)
func deserializePingEntryLegacy(r io.Reader) (*MasternodePingCacheEntry, error) {
	ping := &MasternodePingCacheEntry{}

	// 1. vin (CTxIn: outpoint + scriptSig + sequence)
	if _, err := io.ReadFull(r, ping.Outpoint.Hash[:]); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &ping.Outpoint.Index); err != nil {
		return nil, err
	}
	// Skip scriptSig
	scriptSigLen, err := readCompactSize(r)
	if err != nil {
		return nil, err
	}
	if scriptSigLen > 0 {
		discard := make([]byte, scriptSigLen)
		if _, err := io.ReadFull(r, discard); err != nil {
			return nil, err
		}
	}
	// Skip nSequence
	var sequence uint32
	if err := binary.Read(r, binary.LittleEndian, &sequence); err != nil {
		return nil, err
	}

	// 2. blockHash
	if _, err := io.ReadFull(r, ping.BlockHash[:]); err != nil {
		return nil, err
	}

	// 3. sigTime
	if err := binary.Read(r, binary.LittleEndian, &ping.SignatureTime); err != nil {
		return nil, err
	}

	// 4. vchSig
	sigLen, err := readCompactSize(r)
	if err != nil {
		return nil, err
	}
	if sigLen > 0 {
		ping.Signature = make([]byte, sigLen)
		if _, err := io.ReadFull(r, ping.Signature); err != nil {
			return nil, err
		}
	}

	// Check if ping is empty (zero hash indicates no valid ping)
	var zeroHash types.Hash
	if ping.Outpoint.Hash == zeroHash && ping.SignatureTime == 0 {
		return nil, nil // No valid ping
	}

	return ping, nil
}

func serializePingEntry(w io.Writer, ping *MasternodePingCacheEntry) error {
	// Outpoint
	if _, err := w.Write(ping.Outpoint.Hash[:]); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, ping.Outpoint.Index); err != nil {
		return err
	}

	// BlockHash
	if _, err := w.Write(ping.BlockHash[:]); err != nil {
		return err
	}

	// SignatureTime
	if err := binary.Write(w, binary.LittleEndian, ping.SignatureTime); err != nil {
		return err
	}

	// Signature
	if err := writeCompactSize(w, uint64(len(ping.Signature))); err != nil {
		return err
	}
	if _, err := w.Write(ping.Signature); err != nil {
		return err
	}

	return nil
}

func deserializePingEntry(r io.Reader) (*MasternodePingCacheEntry, error) {
	ping := &MasternodePingCacheEntry{}

	// Outpoint
	if _, err := io.ReadFull(r, ping.Outpoint.Hash[:]); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &ping.Outpoint.Index); err != nil {
		return nil, err
	}

	// BlockHash
	if _, err := io.ReadFull(r, ping.BlockHash[:]); err != nil {
		return nil, err
	}

	// SignatureTime
	if err := binary.Read(r, binary.LittleEndian, &ping.SignatureTime); err != nil {
		return nil, err
	}

	// Signature
	sigLen, err := readCompactSize(r)
	if err != nil {
		return nil, err
	}
	if sigLen > 0 {
		ping.Signature = make([]byte, sigLen)
		if _, err := io.ReadFull(r, ping.Signature); err != nil {
			return nil, err
		}
	}

	return ping, nil
}

// cacheEntryToMasternode converts a cache entry back to a Masternode
// LEGACY COMPATIBILITY: Restores all fields from legacy CMasternode format
func cacheEntryToMasternode(entry *MasternodeCacheEntry) (*Masternode, error) {
	// Initialize time fields from SigTime (will be overridden by LastPingMessage if present)
	sigTimeT := time.Unix(entry.SigTime, 0)

	mn := &Masternode{
		// Core identity
		OutPoint:  entry.OutPoint,
		Signature: entry.Signature,

		// Timing
		SigTime:  entry.SigTime,
		Protocol: entry.Protocol,

		// Status
		Status: entry.Status,

		// Time fields - initialize from SigTime (legacy compatible)
		// These will be overridden below if LastPingMessage is present
		ActiveSince: sigTimeT,
		LastSeen:    sigTimeT,
		LastPing:    sigTimeT,

		// Cache fields (legacy compatibility)
		CacheInputAge:      entry.CacheInputAge,
		CacheInputAgeBlock: entry.CacheInputAgeBlock,

		// Legacy bool fields
		UnitTest:    entry.UnitTest,
		AllowFreeTx: entry.AllowFreeTx,

		// Legacy tracking fields
		LastDsq:                      entry.LastDsq,
		ScanningErrorCount:           entry.ScanningErrorCount,
		LastScanningErrorBlockHeight: entry.LastScanningErrorBlockHeight,

		// Go-specific fields
		BlockHeight: entry.BlockHeight,
		Tier:        entry.Tier,
		Collateral:  entry.Collateral,
		LastPaid:    time.Unix(entry.LastPaid, 0),
	}

	// Parse service address
	if entry.Addr != "" {
		mn.Addr = &net.TCPAddr{}
		// Simple parsing - could be improved with proper net.ResolveTCPAddr
		if addr, err := net.ResolveTCPAddr("tcp", entry.Addr); err == nil {
			mn.Addr = addr
		}
	}

	// Parse public keys
	if len(entry.PubKeyCollateral) > 0 {
		var err error
		mn.PubKeyCollateral, err = crypto.ParsePubKey(entry.PubKeyCollateral)
		if err != nil {
			return nil, fmt.Errorf("failed to parse collateral pubkey: %w", err)
		}
	}

	if len(entry.PubKeyMasternode) > 0 {
		var err error
		mn.PubKey, err = crypto.ParsePubKey(entry.PubKeyMasternode)
		if err != nil {
			return nil, fmt.Errorf("failed to parse masternode pubkey: %w", err)
		}
	}

	// Convert ping and update time fields from ping's SigTime
	if entry.LastPingMessage != nil {
		mn.LastPingMessage = &MasternodePing{
			OutPoint:  entry.LastPingMessage.Outpoint,
			BlockHash: entry.LastPingMessage.BlockHash,
			SigTime:   entry.LastPingMessage.SignatureTime,
			Signature: entry.LastPingMessage.Signature,
		}
		// Override LastSeen/LastPing with ping's SigTime (more recent than broadcast)
		pingTimeT := time.Unix(entry.LastPingMessage.SignatureTime, 0)
		mn.LastSeen = pingTimeT
		mn.LastPing = pingTimeT
	}

	return mn, nil
}
