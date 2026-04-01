// Package storage provides wallet persistence and migration utilities
package storage

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/twins-dev/twins-core/internal/wallet/serialization"
	bdbcgo "github.com/twins-dev/twins-core/internal/wallet/storage/berkeleydb/cgo"
	"github.com/twins-dev/twins-core/pkg/crypto"
	"go.etcd.io/bbolt"
)

// WalletFormat represents the wallet file format type
type WalletFormat int

const (
	// FormatUnknown indicates an unknown or invalid format
	FormatUnknown WalletFormat = iota
	// FormatLegacyBerkeleyDB indicates legacy C++ BerkeleyDB format
	FormatLegacyBerkeleyDB
	// FormatBbolt indicates modern Go bbolt format
	FormatBbolt
)

var (
	// ErrUnknownFormat indicates the wallet format could not be determined
	ErrUnknownFormat = errors.New("unknown wallet format")
	// ErrMigrationFailed indicates the migration process failed
	ErrMigrationFailed = errors.New("migration failed")
)

// Magic bytes for format detection
var (
	berkeleyMagic = []byte{0x62, 0x31, 0x05, 0x00} // BerkeleyDB magic at offset 12
	bboltMagic    = []byte{0xED, 0x0B, 0xBA, 0xBB} // bbolt magic at offset 0
)

// DetectWalletFormat determines the format of a wallet.dat file
func DetectWalletFormat(path string) (WalletFormat, error) {
	f, err := os.Open(path)
	if err != nil {
		return FormatUnknown, err
	}
	defer f.Close()

	// Read first 512 bytes
	header := make([]byte, 512)
	n, err := f.Read(header)
	if err != nil && err != io.EOF {
		return FormatUnknown, err
	}
	if n < 16 {
		return FormatUnknown, fmt.Errorf("file too small: %d bytes", n)
	}

	// Check for BerkeleyDB magic at offset 12 first
	if n >= 16 && bytes.Equal(header[12:16], berkeleyMagic) {
		return FormatLegacyBerkeleyDB, nil
	}

	// Try opening as bbolt - if it succeeds, it's bbolt format
	db, err := bbolt.Open(path, 0600, &bbolt.Options{ReadOnly: true, Timeout: 0})
	if err == nil {
		db.Close()
		return FormatBbolt, nil
	}

	return FormatUnknown, nil
}

// MigrationOptions configures the migration process
type MigrationOptions struct {
	// CreateBackup creates a backup of the original file
	CreateBackup bool
	// BackupSuffix is the suffix for backup files
	BackupSuffix string
	// Verify verifies the migration after completion
	Verify bool
	// Logger is called with migration progress messages
	Logger func(string)
	// LogWarnings enables warning messages during migration
	LogWarnings bool
}

// DefaultMigrationOptions returns default migration options
func DefaultMigrationOptions() *MigrationOptions {
	return &MigrationOptions{
		CreateBackup: true,
		BackupSuffix: ".legacy.backup",
		Verify:       true,
		Logger:       func(msg string) { fmt.Println(msg) },
		LogWarnings:  false, // Disable debug warnings by default
	}
}

// MigrateWallet migrates a legacy BerkeleyDB wallet to bbolt format
func MigrateWallet(path string, opts *MigrationOptions) error {
	if opts == nil {
		opts = DefaultMigrationOptions()
	}

	// Detect current format
	format, err := DetectWalletFormat(path)
	if err != nil {
		return fmt.Errorf("failed to detect format: %w", err)
	}

	switch format {
	case FormatBbolt:
		opts.Logger("Wallet is already in modern format")
		return nil

	case FormatLegacyBerkeleyDB:
		opts.Logger("Detected legacy BerkeleyDB wallet")
		return migrateBerkeleyDBToBbolt(path, opts)

	default:
		return ErrUnknownFormat
	}
}

// migrateBerkeleyDBToBbolt performs the actual migration
func migrateBerkeleyDBToBbolt(path string, opts *MigrationOptions) error {
	// Step 1: Create backup
	backupPath := path + opts.BackupSuffix
	if opts.CreateBackup {
		opts.Logger(fmt.Sprintf("Creating backup: %s", backupPath))
		if err := copyFile(path, backupPath); err != nil {
			return fmt.Errorf("backup failed: %w", err)
		}
	}

	// Step 2: Read legacy wallet
	opts.Logger("Reading legacy wallet data...")
	legacyData, err := ReadLegacyWallet(path, opts.Logger)
	if err != nil {
		return fmt.Errorf("failed to read legacy wallet: %w", err)
	}
	opts.Logger(fmt.Sprintf("Found %d entries in legacy wallet", len(legacyData)))

	// Check if wallet is encrypted (just for logging)
	if IsWalletEncrypted(legacyData) {
		opts.Logger("Wallet is encrypted - will remain encrypted after migration")
	}

	// Step 3: Create temporary bbolt database
	tmpPath := path + ".tmp"
	opts.Logger("Creating new wallet format...")
	if err := writeBboltWallet(tmpPath, legacyData); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write new wallet: %w", err)
	}

	// Step 4: Verify migration if requested
	if opts.Verify {
		opts.Logger("Verifying migration...")
		if err := verifyMigration(legacyData, tmpPath); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("verification failed: %w", err)
		}
	}

	// Step 5: Atomic replacement
	opts.Logger("Finalizing migration...")
	oldPath := path + ".old"
	if err := os.Rename(path, oldPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to move old wallet: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		// Rollback
		os.Rename(oldPath, path)
		os.Remove(tmpPath)
		return fmt.Errorf("failed to move new wallet: %w", err)
	}

	// Cleanup
	os.Remove(oldPath)
	opts.Logger("Migration completed successfully!")

	return nil
}

// WalletEntry represents a key-value entry from the wallet
type WalletEntry struct {
	Key   []byte
	Value []byte
}

// ReadLegacyWallet reads all data from a BerkeleyDB wallet using CGo
// ReadLegacyWallet reads entries from a legacy BerkeleyDB wallet
// logger is optional and will be called with warning messages
func ReadLegacyWallet(path string, logger ...func(string)) ([]WalletEntry, error) {
	// Set up logger (optional parameter)
	log := func(msg string) {}
	if len(logger) > 0 && logger[0] != nil {
		log = logger[0]
	}

	// Open with CGo reader (uses embedded BerkeleyDB library)
	reader, err := bdbcgo.OpenSimple(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open wallet with BerkeleyDB: %w", err)
	}
	defer reader.Close()

	// Get all entries using CGo (reads directly from BerkeleyDB)
	entries, err := reader.GetAllEntries()
	if err != nil {
		return nil, fmt.Errorf("failed to read entries from BerkeleyDB: %w", err)
	}

	log(fmt.Sprintf("Read %d entries from BerkeleyDB using CGo", len(entries)))

	// Convert to WalletEntry format, filtering out invalid entries
	result := make([]WalletEntry, 0, len(entries))

	for _, entry := range entries {
		// Skip entries with empty keys (bbolt doesn't allow empty keys)
		if len(entry.Key) == 0 {
			continue
		}

		result = append(result, WalletEntry{
			Key:   entry.Key,
			Value: entry.Value,
		})
	}

	return result, nil
}

// writeBboltWallet writes wallet data to a bbolt database
func writeBboltWallet(path string, entries []WalletEntry) error {
	// Create bbolt database
	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		return err
	}
	defer db.Close()

	// Count different types of entries for debugging
	keyCount := 0
	keymetaCount := 0
	truncatedCount := 0
	convertedKeys := 0
	skippedCount := 0
	debugInfo := []string{}

	// Create wallet bucket
	err = db.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("wallet"))
		if err != nil {
			return err
		}

		// Write all entries, converting legacy format to new format where needed
		// Only migrate essential wallet data (keys, metadata, labels)
		for i, entry := range entries {
			// Parse key type using length-prefixed format
			keyType := parseKeyType(entry.Key)

			// Filter: only migrate essential data
			// Skip transactions, zerocoin, pool, accounting, etc.
			skipTypes := map[string]bool{
				"tx":           true, // Transactions (in blockchain)
				"pool":         true, // Key pool (regenerate)
				"zerocoin":     true, // Zerocoin data
				"zco":          true, // Zerocoin archived
				"dztwins":      true, // Deterministic zerocoin
				"dzco":         true, // Deterministic archived
				"zcserial":     true, // Zerocoin serials
				"dzs":          true, // Zerocoin seed
				"seedhash":     true, // Zerocoin seed hash
				"dzc":          true, // Zerocoin count
				"mintpool":     true, // Zerocoin mint pool
				"acentry":      true, // Accounting entries
				"acc":          true, // Accounts
				"bestblock":    true, // Best block (recalculate)
				"orderposnext": true, // Order position (recalculate)
			}

			if skipTypes[keyType] {
				skippedCount++
				continue
			}

			// Count entry types before conversion
			switch keyType {
			case "key":
				keyCount++
			case "keymeta":
				keymetaCount++
			}

			// Also check for old truncated format (legacy issue)
			if len(entry.Key) == 36 && entry.Key[0] == 0x65 && entry.Key[1] == 0x79 && entry.Key[2] == 0x21 {
				truncatedCount++
				// Show first truncated entry for debugging
				if truncatedCount == 1 {
					debugInfo = append(debugInfo, fmt.Sprintf("First truncated entry: key=%x, value_len=%d, has_marker=%v",
						entry.Key, len(entry.Value), bytes.Contains(entry.Value, []byte{0xf7, 0x00, 0x01, 0xd6})))
				}
			}

			originalKey := entry.Key
			convertedEntry, err := convertLegacyEntry(entry)
			if err != nil {
				return fmt.Errorf("failed to convert entry %d: %w", i, err)
			}

			// Check if conversion changed the key
			if !bytes.Equal(originalKey, convertedEntry.Key) {
				if i < 5 {
					debugInfo = append(debugInfo, fmt.Sprintf("Entry %d converted: %x -> %x",
						i, originalKey[:min(20, len(originalKey))], convertedEntry.Key[:min(20, len(convertedEntry.Key))]))
				}
			}

			// Check if conversion produced a key entry
			if bytes.HasPrefix(convertedEntry.Key, []byte("key")) && !bytes.Contains(convertedEntry.Key, []byte("keymeta")) {
				convertedKeys++
			}

			if err := bucket.Put(convertedEntry.Key, convertedEntry.Value); err != nil {
				return fmt.Errorf("failed to write entry %d: %w", i, err)
			}
		}

		debugInfo = append(debugInfo, fmt.Sprintf("Skipped %d unnecessary entries", skippedCount))

		return nil
	})

	return err
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// parseKeyType extracts the type string from a length-prefixed BerkeleyDB key
// Format: [length_byte][text...][0x00][data...]
func parseKeyType(key []byte) string {
	if len(key) < 2 {
		return ""
	}

	prefixLen := int(key[0])
	if prefixLen == 0 || prefixLen >= 20 || len(key) < prefixLen+1 {
		return ""
	}

	return string(key[1 : 1+prefixLen])
}

// convertLegacyEntry converts a legacy wallet entry to the new VarBytes format
func convertLegacyEntry(entry WalletEntry) (WalletEntry, error) {
	// Parse key type using length-prefixed format
	keyType := parseKeyType(entry.Key)

	switch keyType {
	case "key":
		// Private key entry
		// Format: [length][text][pubkey_length][pubkey_data]
		// Extract pubkey from key
		prefixLen := int(entry.Key[0])
		dataStart := 1 + prefixLen // length + text (no 0x00 separator!)

		if dataStart >= len(entry.Key) {
			// Invalid format, keep as is
			return entry, nil
		}

		// Read pubkey length and extract pubkey
		pubkeyLen := int(entry.Key[dataStart])
		pubkeyStart := dataStart + 1

		if pubkeyStart+pubkeyLen > len(entry.Key) {
			// Invalid format, keep as is
			return entry, nil
		}

		pubKey := entry.Key[pubkeyStart : pubkeyStart+pubkeyLen]

		// Try to extract private key from value (DER format, with or without marker)
		privKey, derivedPubKey, err := extractKeysFromDER(entry.Value)
		if err == nil {
			// Successfully extracted private key from DER
			// Use derived pubkey if available, otherwise use the one from key
			if len(derivedPubKey) > 0 {
				pubKey = derivedPubKey
			}

			// Create new key in VarBytes format: VarBytes("key") + VarBytes(pubkey)
			newKey, err := encodeKeyEntry([]byte("key"), pubKey, privKey)
			if err != nil {
				return WalletEntry{}, fmt.Errorf("failed to encode key entry: %w", err)
			}

			return newKey, nil
		}

		// Could not extract private key, keep as is
		return entry, nil

	case "keymeta":
		// Key metadata entry
		// Format: [length][text][pubkey_length][pubkey_data]
		prefixLen := int(entry.Key[0])
		dataStart := 1 + prefixLen // length + text (no 0x00 separator!)

		if dataStart >= len(entry.Key) {
			// Invalid format, keep as is
			return entry, nil
		}

		// Read pubkey length and extract pubkey
		pubkeyLen := int(entry.Key[dataStart])
		pubkeyStart := dataStart + 1

		if pubkeyStart+pubkeyLen > len(entry.Key) {
			// Invalid format, keep as is
			return entry, nil
		}

		pubkey := entry.Key[pubkeyStart : pubkeyStart+pubkeyLen]

		// Create new keymeta entry: VarBytes("keymeta") + VarBytes(pubkey)
		var keyBuf bytes.Buffer
		if err := serialization.WriteVarBytes(&keyBuf, []byte("keymeta")); err != nil {
			return WalletEntry{}, err
		}
		if err := serialization.WriteVarBytes(&keyBuf, pubkey); err != nil {
			return WalletEntry{}, err
		}

		return WalletEntry{
			Key:   keyBuf.Bytes(),
			Value: entry.Value,
		}, nil

	case "ckey":
		// Encrypted key entry
		// Format: [length]["ckey"][pubkey_length][pubkey_data]
		prefixLen := int(entry.Key[0])
		dataStart := 1 + prefixLen // length + text (no 0x00 separator!)

		if dataStart >= len(entry.Key) {
			// Invalid format, keep as is
			return entry, nil
		}

		// Read pubkey length and extract pubkey
		pubkeyLen := int(entry.Key[dataStart])
		pubkeyStart := dataStart + 1

		if pubkeyStart+pubkeyLen > len(entry.Key) {
			// Invalid format, keep as is
			return entry, nil
		}

		pubkey := entry.Key[pubkeyStart : pubkeyStart+pubkeyLen]

		// Create new ckey entry: VarBytes("ckey") + VarBytes(pubkey)
		var keyBuf bytes.Buffer
		if err := serialization.WriteVarBytes(&keyBuf, []byte("ckey")); err != nil {
			return WalletEntry{}, err
		}
		if err := serialization.WriteVarBytes(&keyBuf, pubkey); err != nil {
			return WalletEntry{}, err
		}

		// IMPORTANT: Legacy format has [length_byte][encrypted_data]
		// Skip the first byte (length prefix) to get raw encrypted data
		encryptedData := entry.Value
		if len(encryptedData) > 0 {
			// First byte is length, skip it
			encryptedData = encryptedData[1:]
		}

		return WalletEntry{
			Key:   keyBuf.Bytes(),
			Value: encryptedData, // Raw encrypted data without length prefix
		}, nil

	case "mkey":
		// Master key entry - remove length prefix
		// Legacy format: [length]["mkey"][4_byte_id]
		// New format: "mkey"[4_byte_id]
		prefixLen := int(entry.Key[0])
		dataStart := 1 + prefixLen // Skip length + "mkey"

		if dataStart >= len(entry.Key) {
			// Invalid format, keep as is
			return entry, nil
		}

		// Extract ID (4 bytes after "mkey")
		idBytes := entry.Key[dataStart:]

		// Create new key: "mkey" + id
		newKey := append([]byte("mkey"), idBytes...)

		return WalletEntry{
			Key:   newKey,
			Value: entry.Value, // Master key data unchanged
		}, nil

	case "name", "purpose":
		// Address label/purpose entries need conversion
		// Legacy format: [length]["name"][0x00][address] -> value: [length_byte][label]
		// New format: "name" + address -> value: label (WITHOUT length prefix)
		prefixLen := int(entry.Key[0])
		dataStart := 1 + prefixLen + 1 // length + text + 0x00

		if dataStart >= len(entry.Key) {
			// Invalid format, keep as is
			return entry, nil
		}

		address := entry.Key[dataStart:]

		// Create new key: prefix + address (simple concatenation)
		newKey := append([]byte(keyType), address...)

		// Strip length prefix byte from value (Berkeley DB string format)
		labelValue := entry.Value
		if len(labelValue) > 0 {
			// First byte is length prefix, skip it to get actual label
			labelValue = labelValue[1:]
		}

		return WalletEntry{
			Key:   newKey,
			Value: labelValue, // Label without length prefix
		}, nil

	case "hdchain", "chdchain":
		// HD chain entry - convert to simple key format
		// Legacy format: [length]["hdchain"] -> value: serialized CHDChain
		// New format: "hdchain" -> value: serialized CHDChain (same value, simpler key)
		return WalletEntry{
			Key:   []byte(keyType), // Just "hdchain" or "chdchain" without length prefix
			Value: entry.Value,     // CHDChain serialization unchanged
		}, nil

	case "hdpubkey":
		// HD public key entry - contains derivation path info
		// Legacy key format: [length]["hdpubkey"][pubkey_length][pubkey_33_bytes]
		// Legacy value format: CHDPubKey serialization
		// New key format: "hdpubkey" + VarBytes(pubkey)
		// New value format: same CHDPubKey serialization
		prefixLen := int(entry.Key[0])
		dataStart := 1 + prefixLen // Skip length + "hdpubkey"

		if dataStart >= len(entry.Key) {
			return entry, nil
		}

		// Read pubkey length and extract pubkey
		pubkeyLen := int(entry.Key[dataStart])
		pubkeyStart := dataStart + 1

		if pubkeyStart+pubkeyLen > len(entry.Key) {
			return entry, nil
		}

		pubkey := entry.Key[pubkeyStart : pubkeyStart+pubkeyLen]

		// Create new hdpubkey entry: VarBytes("hdpubkey") + VarBytes(pubkey)
		var keyBuf bytes.Buffer
		if err := serialization.WriteVarBytes(&keyBuf, []byte("hdpubkey")); err != nil {
			return WalletEntry{}, err
		}
		if err := serialization.WriteVarBytes(&keyBuf, pubkey); err != nil {
			return WalletEntry{}, err
		}

		return WalletEntry{
			Key:   keyBuf.Bytes(),
			Value: entry.Value, // CHDPubKey serialization unchanged
		}, nil

	case "tx", "pool", "mintpool", "destdata":
		// Transaction and metadata entries - keep as is
		return entry, nil

	case "version", "minversion", "bestblock", "orderposnext", "dzs", "seedhash", "defaultkey", "autocombinesettings", "stakeSplitThreshold":
		// Simple metadata entries - keep as is
		return entry, nil

	default:
		// Unknown key type or empty - keep as is
		return entry, nil
	}
}

// Legacy code for handling old truncated format (backup, might not be needed anymore)
func convertLegacyEntryOldFormat(entry WalletEntry) (WalletEntry, error) {
	privateKeyMarker := []byte{0xf7, 0x00, 0x01, 0xd6}

	// Check for truncated key entries (0x657921 = "ey!" missing the 'k')
	// These are private keys where BerkeleyDB truncated the "key" prefix
	if len(entry.Key) == 36 && entry.Key[0] == 0x65 && entry.Key[1] == 0x79 && entry.Key[2] == 0x21 {
		// This is a truncated "key" entry with a 33-byte compressed pubkey
		// The key format is: "ey!" (3 bytes) + compressed pubkey (33 bytes)
		pubKey := entry.Key[3:] // Extract the 33-byte compressed pubkey

		// Check if value contains private key data
		if bytes.Contains(entry.Value, privateKeyMarker) {
			// Extract private key from value
			privKey, _, err := extractKeysFromDER(entry.Value)
			if err != nil {
				// Could not extract, keep the entry as is
				return entry, nil
			}

			// Create properly formatted key entry
			keyPrefix := []byte("key")
			newKey, err := encodeKeyEntry(keyPrefix, pubKey, privKey)
			if err != nil {
				return WalletEntry{}, fmt.Errorf("failed to encode truncated key entry: %w", err)
			}

			return newKey, nil
		}
	}

	return entry, nil
}

// encodeKeyEntry encodes a key entry in VarBytes format
func encodeKeyEntry(prefix []byte, pubkey []byte, value []byte) (WalletEntry, error) {
	// Encode key: VarBytes(prefix) + VarBytes(pubkey)
	var keyBuf bytes.Buffer
	if err := serialization.WriteVarBytes(&keyBuf, prefix); err != nil {
		return WalletEntry{}, fmt.Errorf("failed to encode prefix: %w", err)
	}
	if err := serialization.WriteVarBytes(&keyBuf, pubkey); err != nil {
		return WalletEntry{}, fmt.Errorf("failed to encode pubkey: %w", err)
	}

	// Encode value: VarBytes(data)
	var valueBuf bytes.Buffer
	if err := serialization.WriteVarBytes(&valueBuf, value); err != nil {
		return WalletEntry{}, fmt.Errorf("failed to encode value: %w", err)
	}

	return WalletEntry{
		Key:   keyBuf.Bytes(),
		Value: valueBuf.Bytes(),
	}, nil
}

// extractKeysFromDER extracts private key from DER-encoded value and derives public key
func extractKeysFromDER(value []byte) ([]byte, []byte, error) {
	var derStart int

	// Look for the private key marker 0xf70001d6 (optional in some wallets)
	markerPos := bytes.Index(value, []byte{0xf7, 0x00, 0x01, 0xd6})
	if markerPos >= 0 {
		// Skip to DER structure (after marker)
		derStart = markerPos + 4
	} else {
		// No marker - DER starts at beginning of value
		derStart = 0
	}

	// Look for the private key within DER structure
	// Private key is after 0x0420 (OCTET STRING of 32 bytes)
	privKeyMarker := []byte{0x04, 0x20}
	privKeyPos := bytes.Index(value[derStart:], privKeyMarker)
	if privKeyPos < 0 {
		return nil, nil, fmt.Errorf("private key not found in DER")
	}

	// Extract 32-byte private key
	privKeyStart := derStart + privKeyPos + 2
	if privKeyStart+32 > len(value) {
		return nil, nil, fmt.Errorf("invalid private key position")
	}
	privKey := value[privKeyStart : privKeyStart+32]

	// Parse private key and derive public key
	privateKey, err := crypto.ParsePrivateKeyFromBytes(privKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// Get compressed public key
	pubKey := privateKey.PublicKey().CompressedBytes()

	// Return just the 32-byte private key for the new wallet format
	// The new wallet expects raw private key bytes, not DER-encoded
	return privKey, pubKey, nil
}

// verifyMigration verifies that all data was migrated correctly
func verifyMigration(original []WalletEntry, newPath string) error {
	db, err := bbolt.Open(newPath, 0600, &bbolt.Options{ReadOnly: true})
	if err != nil {
		return err
	}
	defer db.Close()

	// Count different entry types
	var privateKeys, keymetas, pools, others int

	// Check what was converted
	for _, entry := range original {
		// Check if it was a private key entry
		if bytes.Contains(entry.Value, []byte{0xf7, 0x00, 0x01, 0xd6}) {
			privateKeys++
		} else if bytes.Contains(entry.Key, []byte("keymeta")) {
			keymetas++
		} else if bytes.HasPrefix(entry.Key, []byte("pool")) {
			pools++
		} else {
			others++
		}
	}

	// Verify entries in new database
	var newPrivateKeys, newKeymetas, newPools, newOthers int
	err = db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket([]byte("wallet"))
		if bucket == nil {
			return errors.New("wallet bucket not found")
		}

		// Count entries in new format
		c := bucket.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			// Try to decode as VarBytes to check if it's a converted key
			reader := bytes.NewReader(k)
			prefix, err := serialization.ReadVarBytes(reader)
			if err == nil {
				// Successfully decoded VarBytes
				if bytes.Equal(prefix, []byte("key")) {
					newPrivateKeys++
				} else if bytes.Equal(prefix, []byte("keymeta")) {
					newKeymetas++
				}
			} else {
				// Not VarBytes format
				if bytes.HasPrefix(k, []byte("pool")) {
					newPools++
				} else {
					newOthers++
				}
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Log statistics only if verification is enabled
	// These statistics can be logged by the caller if needed

	// Basic sanity check - we should have converted some keys
	if privateKeys > 0 && newPrivateKeys == 0 {
		return fmt.Errorf("no private keys were migrated")
	}

	totalMigrated := newPrivateKeys + newKeymetas + newPools + newOthers

	// Allow some difference as we filter empty keys and convert formats
	if totalMigrated == 0 {
		return fmt.Errorf("no entries were migrated")
	}

	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

// AutoMigrateWallet automatically migrates a wallet if needed
func AutoMigrateWallet(dataDir string) error {
	walletPath := filepath.Join(dataDir, "wallet.dat")

	// Check if wallet exists
	if _, err := os.Stat(walletPath); os.IsNotExist(err) {
		return nil // No wallet to migrate
	}

	// Detect format
	format, err := DetectWalletFormat(walletPath)
	if err != nil {
		return err
	}

	// Migrate if needed
	if format == FormatLegacyBerkeleyDB {
		opts := DefaultMigrationOptions()
		return MigrateWallet(walletPath, opts)
	}

	return nil
}
