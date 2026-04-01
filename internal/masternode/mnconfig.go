// Copyright (c) 2018-2025 The TWINS developers
// Distributed under the MIT/X11 software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package masternode

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/twins-dev/twins-core/pkg/types"
)

// MasternodeEntry represents a single entry from masternode.conf
// Format: alias ip:port privkey txHash outputIndex [donationAddress:percent]
type MasternodeEntry struct {
	Alias       string         `json:"alias"`
	IP          string         `json:"ip"`   // host:port
	PrivKey     string         `json:"privkey"`
	TxHash      types.Hash     `json:"txhash"`
	OutputIndex uint32         `json:"outputindex"`
	// Optional donation fields (legacy support)
	DonationAddress string `json:"donation_address,omitempty"`
	DonationPercent int    `json:"donation_percent,omitempty"` // 0-100
}

// GetOutpoint returns the outpoint for this masternode entry
func (e *MasternodeEntry) GetOutpoint() types.Outpoint {
	return types.Outpoint{
		Hash:  e.TxHash,
		Index: e.OutputIndex,
	}
}

// GetHost returns the host part of IP (without port).
// Supports both IPv4 (1.2.3.4:port) and IPv6 ([::1]:port) formats.
func (e *MasternodeEntry) GetHost() string {
	host, _, err := net.SplitHostPort(e.IP)
	if err != nil {
		return e.IP
	}
	return host
}

// GetPort returns the port part of IP.
// Supports both IPv4 (1.2.3.4:port) and IPv6 ([::1]:port) formats.
func (e *MasternodeEntry) GetPort() int {
	_, portStr, err := net.SplitHostPort(e.IP)
	if err != nil {
		return 0
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0
	}
	return port
}

// MasternodeConfFile manages the masternode.conf file
type MasternodeConfFile struct {
	entries  []*MasternodeEntry
	filepath string
	mu       sync.RWMutex
}

// NewMasternodeConfFile creates a new masternode.conf manager
func NewMasternodeConfFile(dataDir, filename string) *MasternodeConfFile {
	return &MasternodeConfFile{
		entries:  make([]*MasternodeEntry, 0),
		filepath: filepath.Join(dataDir, filename),
	}
}

// Read reads and parses the masternode.conf file
// Returns an error if the file exists but contains invalid entries
// Returns nil if the file doesn't exist (creates empty template)
func (mc *MasternodeConfFile) Read() error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	// Clear existing entries
	mc.entries = make([]*MasternodeEntry, 0)

	file, err := os.Open(mc.filepath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create default template file
			return mc.createTemplate()
		}
		return fmt.Errorf("failed to open masternode.conf: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines
		if line == "" {
			continue
		}

		// Skip comments
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Parse the entry
		entry, err := mc.parseLine(line, lineNumber)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineNumber, err)
		}

		mc.entries = append(mc.entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading masternode.conf: %w", err)
	}

	return nil
}

// parseLine parses a single line from masternode.conf
// Format: alias ip:port privkey txHash outputIndex [donationAddress:percent]
func (mc *MasternodeConfFile) parseLine(line string, lineNum int) (*MasternodeEntry, error) {
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return nil, fmt.Errorf("invalid format: expected at least 5 fields (alias ip:port privkey txHash outputIndex), got %d", len(fields))
	}

	entry := &MasternodeEntry{
		Alias:   fields[0],
		IP:      fields[1],
		PrivKey: fields[2],
	}

	// Parse txHash (64 hex characters)
	txHashStr := fields[3]
	if len(txHashStr) != 64 {
		return nil, fmt.Errorf("invalid txHash length: expected 64 characters, got %d", len(txHashStr))
	}
	hash, err := types.NewHashFromString(txHashStr)
	if err != nil {
		return nil, fmt.Errorf("invalid txHash: %w", err)
	}
	entry.TxHash = hash

	// Parse outputIndex
	outputIndex, err := strconv.ParseUint(fields[4], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid outputIndex: %w", err)
	}
	entry.OutputIndex = uint32(outputIndex)

	// Validate IP:port format
	host := entry.GetHost()
	port := entry.GetPort()
	if host == "" {
		return nil, fmt.Errorf("invalid IP: missing host")
	}
	// Validate port was parsed successfully (GetPort returns 0 on parse error)
	if port == 0 {
		return nil, fmt.Errorf("invalid port: failed to parse from '%s'", entry.IP)
	}
	// Validate port upper bound (port > 0 guaranteed by check above)
	if port > 65535 {
		return nil, fmt.Errorf("invalid port %d: must be between 1 and 65535", port)
	}

	// Parse optional donation field (donationAddress:percent)
	if len(fields) >= 6 {
		donationParts := strings.Split(fields[5], ":")
		if len(donationParts) == 2 {
			entry.DonationAddress = donationParts[0]
			percent, err := strconv.Atoi(donationParts[1])
			if err != nil {
				return nil, fmt.Errorf("invalid donation percent: %w", err)
			}
			if percent < 0 || percent > 100 {
				return nil, fmt.Errorf("invalid donation percent %d: must be between 0 and 100", percent)
			}
			entry.DonationPercent = percent
		} else if len(donationParts) != 1 || donationParts[0] != "" {
			return nil, fmt.Errorf("invalid donation format: expected 'address:percent', got '%s'", fields[5])
		}
	}

	return entry, nil
}

// createTemplate creates a template masternode.conf file
func (mc *MasternodeConfFile) createTemplate() error {
	file, err := os.Create(mc.filepath)
	if err != nil {
		return fmt.Errorf("failed to create masternode.conf: %w", err)
	}
	defer file.Close()

	header := `# Masternode config file
# Format: alias IP:port masternodeprivkey collateral_output_txid collateral_output_index
# Example (IPv4): mn1 127.0.0.2:37817 93HaYBVUCYjEMeeH1Y4sBGLALQZE1Yc1K64xiqgX37tGBDQL8Xg 2bcd3c84c84f87eaa86e4e56834c92927a07f9e18718810b92e0d0324456a67c 0
# Example (IPv6): mn2 [2001:db8::1]:37817 93HaYBVUCYjEMeeH1Y4sBGLALQZE1Yc1K64xiqgX37tGBDQL8Xg 2bcd3c84c84f87eaa86e4e56834c92927a07f9e18718810b92e0d0324456a67c 0
`
	_, err = file.WriteString(header)
	if err != nil {
		return fmt.Errorf("failed to write template: %w", err)
	}

	return nil
}

// GetEntries returns all masternode entries
func (mc *MasternodeConfFile) GetEntries() []*MasternodeEntry {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make([]*MasternodeEntry, len(mc.entries))
	copy(result, mc.entries)
	return result
}

// GetEntry returns a masternode entry by alias
func (mc *MasternodeConfFile) GetEntry(alias string) *MasternodeEntry {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	for _, entry := range mc.entries {
		if entry.Alias == alias {
			return entry
		}
	}
	return nil
}

// GetCount returns the number of masternode entries
func (mc *MasternodeConfFile) GetCount() int {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return len(mc.entries)
}

// Add adds a new masternode entry
func (mc *MasternodeConfFile) Add(entry *MasternodeEntry) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	// Check for duplicate alias
	for _, e := range mc.entries {
		if e.Alias == entry.Alias {
			return fmt.Errorf("masternode alias '%s' already exists", entry.Alias)
		}
	}

	mc.entries = append(mc.entries, entry)
	return nil
}

// Remove removes a masternode entry by alias
func (mc *MasternodeConfFile) Remove(alias string) bool {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	for i, entry := range mc.entries {
		if entry.Alias == alias {
			mc.entries = append(mc.entries[:i], mc.entries[i+1:]...)
			return true
		}
	}
	return false
}

// Update atomically replaces an existing entry with a new one.
// This is safer than separate Remove/Add calls as it validates and modifies
// within a single lock acquisition, preventing race conditions.
func (mc *MasternodeConfFile) Update(oldAlias string, newEntry *MasternodeEntry) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	// Find the old entry
	oldIndex := -1
	for i, entry := range mc.entries {
		if entry.Alias == oldAlias {
			oldIndex = i
			break
		}
	}
	if oldIndex == -1 {
		return fmt.Errorf("masternode alias '%s' not found", oldAlias)
	}

	// If alias is changing, check new alias doesn't already exist
	if oldAlias != newEntry.Alias {
		for _, entry := range mc.entries {
			if entry.Alias == newEntry.Alias {
				return fmt.Errorf("masternode alias '%s' already exists", newEntry.Alias)
			}
		}
	}

	// Replace the entry atomically
	mc.entries[oldIndex] = newEntry
	return nil
}

// Save writes the current entries to the masternode.conf file
func (mc *MasternodeConfFile) Save() error {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	file, err := os.Create(mc.filepath)
	if err != nil {
		return fmt.Errorf("failed to create masternode.conf: %w", err)
	}
	defer file.Close()

	// Write header
	header := `# Masternode config file
# Format: alias IP:port masternodeprivkey collateral_output_txid collateral_output_index
# Example (IPv4): mn1 127.0.0.2:37817 93HaYBVUCYjEMeeH1Y4sBGLALQZE1Yc1K64xiqgX37tGBDQL8Xg 2bcd3c84c84f87eaa86e4e56834c92927a07f9e18718810b92e0d0324456a67c 0
# Example (IPv6): mn2 [2001:db8::1]:37817 93HaYBVUCYjEMeeH1Y4sBGLALQZE1Yc1K64xiqgX37tGBDQL8Xg 2bcd3c84c84f87eaa86e4e56834c92927a07f9e18718810b92e0d0324456a67c 0

`
	if _, err := file.WriteString(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Write entries
	for _, entry := range mc.entries {
		line := fmt.Sprintf("%s %s %s %s %d",
			entry.Alias,
			entry.IP,
			entry.PrivKey,
			entry.TxHash.String(),
			entry.OutputIndex,
		)

		// Add optional donation info
		if entry.DonationAddress != "" && entry.DonationPercent > 0 {
			line += fmt.Sprintf(" %s:%d", entry.DonationAddress, entry.DonationPercent)
		}

		line += "\n"

		if _, err := file.WriteString(line); err != nil {
			return fmt.Errorf("failed to write entry: %w", err)
		}
	}

	return nil
}

// ValidatePort validates the port based on network
// mainnet requires MainnetDefaultPort, testnet cannot use it
func ValidatePort(port int, isMainnet bool) error {
	if isMainnet {
		if port != MainnetDefaultPort {
			return fmt.Errorf("invalid port %d: mainnet requires port %d", port, MainnetDefaultPort)
		}
	} else {
		if port == MainnetDefaultPort {
			return fmt.Errorf("invalid port %d: port %d is reserved for mainnet", port, MainnetDefaultPort)
		}
	}
	return nil
}

// GetFilePath returns the path to the masternode.conf file
func (mc *MasternodeConfFile) GetFilePath() string {
	return mc.filepath
}
