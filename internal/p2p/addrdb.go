package p2p

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// AddressDB manages persistence of peer addresses and aliases
type AddressDB struct {
	path   string
	logger *logrus.Entry

	// Alias storage (IP:Port -> friendly name)
	mu            sync.RWMutex
	aliases       map[string]string
	lastAddresses []SerializableAddress // cached for re-save on alias change
}

// peersEnvelope is the envelope format for peers.json
type peersEnvelope struct {
	Addresses []SerializableAddress `json:"addresses"`
	Aliases   map[string]string     `json:"aliases"`
}

// SerializableAddress represents a known address for persistence
type SerializableAddress struct {
	IP          string    `json:"ip"`
	Port        uint16    `json:"port"`
	Services    uint64    `json:"services"`
	LastSeen    time.Time `json:"last_seen"`
	LastSuccess time.Time `json:"last_success,omitempty"`
	LastAttempt time.Time `json:"last_attempt,omitempty"`
	Attempts    int32     `json:"attempts"`
	Source      string    `json:"source,omitempty"`
	Permanent   bool      `json:"permanent,omitempty"`
	IsBad       bool      `json:"is_bad,omitempty"`
}

// NewAddressDB creates a new address database
// dataDir must be a valid, existing directory where the peers.json file will be stored.
// The caller is responsible for ensuring dataDir exists and is writable.
func NewAddressDB(dataDir string, logger *logrus.Logger) *AddressDB {
	return &AddressDB{
		path:    filepath.Join(dataDir, "peers.json"),
		logger:  logger.WithField("component", "addrdb"),
		aliases: make(map[string]string),
	}
}

// Save persists addresses to disk in envelope format
func (db *AddressDB) Save(addresses map[string]*KnownAddress) error {
	db.logger.WithField("count", len(addresses)).Debug("Saving peer addresses")

	// Convert to serializable format
	serializable := make([]SerializableAddress, 0, len(addresses))
	for _, known := range addresses {
		// Skip bad non-permanent addresses
		if known.IsBad && !known.Permanent {
			continue
		}

		sourceIP := ""
		if known.Source != nil {
			sourceIP = known.Source.String()
		}

		serializable = append(serializable, SerializableAddress{
			IP:          known.Addr.IP.String(),
			Port:        known.Addr.Port,
			Services:    uint64(known.Services),
			LastSeen:    known.LastSeen,
			LastSuccess: known.LastSuccess,
			LastAttempt: known.LastAttempt,
			Attempts:    known.Attempts,
			Source:      sourceIP,
			Permanent:   known.Permanent,
			IsBad:       known.IsBad,
		})
	}

	db.mu.Lock()
	db.lastAddresses = serializable
	aliases := make(map[string]string, len(db.aliases))
	for k, v := range db.aliases {
		aliases[k] = v
	}
	db.mu.Unlock()

	return db.writeEnvelope(serializable, aliases)
}

// writeEnvelope writes the envelope format atomically to disk
func (db *AddressDB) writeEnvelope(addresses []SerializableAddress, aliases map[string]string) error {
	envelope := peersEnvelope{
		Addresses: addresses,
		Aliases:   aliases,
	}

	// Ensure directory exists
	dir := filepath.Dir(db.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Write atomically using temporary file
	tmpPath := db.path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	encodeErr := encoder.Encode(envelope)

	// Close file before rename
	closeErr := f.Close()

	if encodeErr != nil {
		os.Remove(tmpPath)
		return encodeErr
	}

	if closeErr != nil {
		os.Remove(tmpPath)
		return closeErr
	}

	// Atomic rename
	if err := os.Rename(tmpPath, db.path); err != nil {
		os.Remove(tmpPath)
		return err
	}

	db.logger.WithField("count", len(addresses)).Debug("Saved peer addresses")
	return nil
}

// Load reads addresses from disk with backwards-compatible format detection.
// Old format: bare JSON array of SerializableAddress.
// New format: envelope object with "addresses" and "aliases" keys.
func (db *AddressDB) Load() (map[string]*KnownAddress, error) {
	data, err := os.ReadFile(db.path)
	if err != nil {
		if os.IsNotExist(err) {
			db.logger.Info("No saved addresses found, starting fresh")
			return make(map[string]*KnownAddress), nil
		}
		return nil, err
	}

	// Detect format by first non-whitespace byte
	var serializable []SerializableAddress
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		db.logger.Info("Empty peers file, starting fresh")
		return make(map[string]*KnownAddress), nil
	}

	if trimmed[0] == '[' {
		// Old flat array format
		if err := json.Unmarshal(data, &serializable); err != nil {
			db.logger.WithError(err).Warn("Failed to decode addresses, starting fresh")
			return make(map[string]*KnownAddress), nil
		}
		db.logger.Info("Loaded peers.json in legacy array format, will upgrade to envelope on next save")
	} else {
		// New envelope format
		var envelope peersEnvelope
		if err := json.Unmarshal(data, &envelope); err != nil {
			db.logger.WithError(err).Warn("Failed to decode peers envelope, starting fresh")
			return make(map[string]*KnownAddress), nil
		}
		serializable = envelope.Addresses
		db.mu.Lock()
		if envelope.Aliases != nil {
			db.aliases = envelope.Aliases
		}
		db.mu.Unlock()
	}

	// Cache for re-save on alias changes
	db.mu.Lock()
	db.lastAddresses = serializable
	db.mu.Unlock()

	// Convert back to internal format
	addresses := make(map[string]*KnownAddress)
	for _, s := range serializable {
		ip := net.ParseIP(s.IP)
		if ip == nil {
			db.logger.WithField("ip", s.IP).Warn("Invalid IP address in database, skipping")
			continue
		}

		addr := &NetAddress{
			Time:     uint32(s.LastSeen.Unix()),
			Services: ServiceFlag(s.Services),
			IP:       ip,
			Port:     s.Port,
		}

		// Parse source address if present
		var source *NetAddress
		if s.Source != "" {
			sourceHost, sourcePortStr, err := net.SplitHostPort(s.Source)
			if err == nil {
				sourceIP := net.ParseIP(sourceHost)
				if sourceIP != nil {
					var sourcePort uint16
					if p, err := net.ResolveTCPAddr("tcp", net.JoinHostPort("", sourcePortStr)); err == nil {
						sourcePort = uint16(p.Port)
					}
					source = &NetAddress{
						IP:   sourceIP,
						Port: sourcePort,
					}
				}
			}
		}

		known := &KnownAddress{
			Addr:        addr,
			Source:      source,
			LastSeen:    s.LastSeen,
			LastSuccess: s.LastSuccess,
			LastAttempt: s.LastAttempt,
			Attempts:    s.Attempts,
			Services:    ServiceFlag(s.Services),
			Permanent:   s.Permanent,
			IsBad:       s.IsBad,
		}

		addresses[addr.String()] = known
	}

	db.logger.WithField("count", len(addresses)).Debug("Loaded peer addresses")
	return addresses, nil
}

// MaxAliasLength is the maximum length of a peer alias
const MaxAliasLength = 64

// SetAlias sets an alias for a peer address and persists immediately
func (db *AddressDB) SetAlias(addr string, alias string) error {
	if alias == "" {
		return db.RemoveAlias(addr)
	}
	if len(alias) > MaxAliasLength {
		return fmt.Errorf("alias too long (max %d characters)", MaxAliasLength)
	}

	db.mu.Lock()
	db.aliases[addr] = alias
	addrs := make([]SerializableAddress, len(db.lastAddresses))
	copy(addrs, db.lastAddresses)
	aliases := make(map[string]string, len(db.aliases))
	for k, v := range db.aliases {
		aliases[k] = v
	}
	db.mu.Unlock()

	return db.writeEnvelope(addrs, aliases)
}

// RemoveAlias removes an alias for a peer address and persists immediately
func (db *AddressDB) RemoveAlias(addr string) error {
	db.mu.Lock()
	if _, exists := db.aliases[addr]; !exists {
		db.mu.Unlock()
		return nil
	}
	delete(db.aliases, addr)
	addrs := make([]SerializableAddress, len(db.lastAddresses))
	copy(addrs, db.lastAddresses)
	aliases := make(map[string]string, len(db.aliases))
	for k, v := range db.aliases {
		aliases[k] = v
	}
	db.mu.Unlock()

	return db.writeEnvelope(addrs, aliases)
}

// GetAlias returns the alias for a peer address, or empty string if none
func (db *AddressDB) GetAlias(addr string) string {
	db.mu.RLock()
	alias := db.aliases[addr]
	db.mu.RUnlock()
	return alias
}

// GetAliases returns a copy of all aliases
func (db *AddressDB) GetAliases() map[string]string {
	db.mu.RLock()
	result := make(map[string]string, len(db.aliases))
	for k, v := range db.aliases {
		result[k] = v
	}
	db.mu.RUnlock()
	return result
}
