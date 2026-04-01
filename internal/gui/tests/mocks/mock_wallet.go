package mocks

import (
	"crypto/sha256"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/twins-dev/twins-core/internal/gui/core"
)

// Transaction constants for TWINS network
const (
	// DustThreshold is the minimum amount for a valid TWINS transaction
	// This should match the dust threshold in TWINS Core (usually DUST_HARD_LIMIT)
	// TODO: Verify this value against actual TWINS Core implementation
	DustThreshold = 0.001

	// NormalTransactionFee is the standard fee for regular transactions
	NormalTransactionFee = 0.001
)

// Wallet operation implementations for MockCoreClient

// GetBalance implements CoreClient.GetBalance
func (m *MockCoreClient) GetBalance() (core.Balance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return core.Balance{}, fmt.Errorf("core is not running")
	}

	return m.balance, nil
}

// GetNewAddress implements CoreClient.GetNewAddress
func (m *MockCoreClient) GetNewAddress(label string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return "", fmt.Errorf("core is not running")
	}

	// Generate new address with proper checksum
	address := m.generateAddress()
	m.addresses = append(m.addresses, address)

	// Generate a mock public key (64 hex chars = 32 bytes)
	pubkey := m.generatePubKey()

	// Track in ownAddresses map for "isMine" validation
	m.ownAddresses[address] = pubkey

	// Emit event
	m.emitEventLocked(core.NewAddressGeneratedEvent{
		BaseEvent: core.BaseEvent{Type: "new_address_generated", Time: time.Now()},
		Address:   address,
		Label:     label,
	})

	return address, nil
}

// SendToAddress implements CoreClient.SendToAddress
func (m *MockCoreClient) SendToAddress(address string, amount float64, comment string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return "", fmt.Errorf("core is not running")
	}

	if m.locked {
		return "", fmt.Errorf("wallet is locked")
	}

	// Validate address first
	validation, err := m.validateAddressLocked(address)
	if err != nil {
		return "", fmt.Errorf("address validation failed: %w", err)
	}
	if !validation.IsValid {
		return "", fmt.Errorf("invalid address")
	}

	// Validate amount
	if amount <= 0 {
		return "", fmt.Errorf("invalid amount: must be positive")
	}

	// Check dust threshold
	if amount < DustThreshold {
		return "", fmt.Errorf("amount below dust threshold: minimum %.8f TWINS", DustThreshold)
	}

	// Estimate fee
	fee := m.estimateSendFee(amount)
	total := amount + fee

	// Check balance (must have enough for amount + fee)
	if total > m.balance.Spendable {
		return "", fmt.Errorf("insufficient funds: have %.8f, need %.8f (%.8f + %.8f fee)",
			m.balance.Spendable, total, amount, fee)
	}

	// Create transaction
	txid := m.generateTxHash()

	// Determine transaction type
	txType := core.TxTypeSendToAddress
	if validation.IsMine {
		txType = core.TxTypeSendToSelf
	}

	tx := &core.Transaction{
		TxID:          txid,
		Amount:        -amount, // Negative for sends
		Fee:           fee,
		Confirmations: 0,
		BlockHeight:   0, // Unconfirmed initially
		BlockHash:     "",
		Time:          time.Now(),
		Type:          txType,
		Category:      "send",
		Address:       address,
		Comment:       comment,
		IsLocked:      false,
	}

	m.transactions[txid] = tx
	m.txList = append([]string{txid}, m.txList...)

	// Update balance - deduct from spendable and total
	// Note: Pending is for incoming unconfirmed transactions, not outgoing
	// Outgoing transactions immediately reduce Spendable and Total
	m.balance.Spendable -= total
	m.balance.Total -= total

	// Update available (Available = Spendable - Locked)
	m.balance.Available = m.balance.Spendable - m.balance.Locked

	// Emit events
	m.emitEventLocked(core.TransactionSentEvent{
		BaseEvent: core.BaseEvent{Type: "transaction_sent", Time: time.Now()},
		TxID:      txid,
		Amount:    amount,
		Address:   address,
	})

	m.emitEventLocked(core.TransactionReceivedEvent{
		BaseEvent:     core.BaseEvent{Type: "transaction_received", Time: time.Now()},
		TxID:          txid,
		Amount:        -amount, // Negative for outgoing
		Confirmations: 0,
	})

	m.emitEventLocked(core.BalanceChangedEvent{
		BaseEvent: core.BaseEvent{Type: "balance_changed", Time: time.Now()},
		Balance:   m.balance,
	})

	// Start confirmation simulation in background
	m.wg.Add(1)
	go m.simulateNormalConfirmation(txid)

	return txid, nil
}

// SendToAddressWithOptions implements CoreClient.SendToAddressWithOptions
// For mock, it just delegates to SendToAddress (ignores advanced options)
func (m *MockCoreClient) SendToAddressWithOptions(address string, amount float64, comment string, opts *core.SendOptions) (string, error) {
	// Mock implementation ignores advanced options and just calls SendToAddress
	return m.SendToAddress(address, amount, comment)
}

// SendMany implements CoreClient.SendMany
// For mock, it sends to each recipient individually (simplified implementation)
func (m *MockCoreClient) SendMany(recipients map[string]float64, comment string, opts *core.SendOptions) (string, error) {
	if len(recipients) == 0 {
		return "", fmt.Errorf("no recipients specified")
	}

	// For mock, just use the first recipient to generate a txid.
	// In reality, this would create a single transaction with multiple outputs.
	//
	// NOTE: Map iteration order in Go is non-deterministic, so which recipient
	// is used for the mock txid is random. This is acceptable for mock testing
	// as the behavior (returning a valid txid) is consistent, only the specific
	// txid value varies. Production code uses wallet.SendManyWithOptions which
	// creates a real transaction with all recipients.
	for addr, amount := range recipients {
		return m.SendToAddress(addr, amount, comment)
	}

	return "", fmt.Errorf("unexpected error in SendMany")
}

// validateAddressLocked validates an address without locking (caller must hold lock)
// This is a simplified version used by SendToAddress for quick validation
func (m *MockCoreClient) validateAddressLocked(address string) (core.AddressValidation, error) {
	// Trim whitespace
	address = strings.TrimSpace(address)

	// TWINS address validation:
	// - Mainnet addresses start with 'D' (legacy) or 'T' (new format)
	// - Testnet addresses start with 'x' or 'y'
	// - Length should be exactly 34 characters for P2PKH addresses
	// - Must be valid Base58 characters (no 0, O, I, l)

	isValid := false
	if len(address) == 34 {
		firstChar := address[0]
		// Check for valid TWINS address prefix (mainnet only for now)
		if firstChar == 'D' || firstChar == 'T' {
			// Check if all characters are valid Base58
			isValid = true
			for _, c := range address {
				if !isValidBase58Char(c) {
					isValid = false
					break
				}
			}
		}
	}

	// Check if it's one of our addresses using the ownAddresses map
	pubkey, isMine := m.ownAddresses[address]

	validation := core.AddressValidation{
		IsValid:     isValid,
		Address:     address,
		IsMine:      isMine,
		IsWatchOnly: false,
		IsScript:    address[0] == 'T',
		PubKey:      pubkey,
		Account:     "",
	}

	return validation, nil
}

// estimateSendFee estimates the fee for a send transaction
func (m *MockCoreClient) estimateSendFee(amount float64) float64 {
	// Base fee for normal transaction
	return NormalTransactionFee
}

// simulateNormalConfirmation simulates normal transaction confirmations
func (m *MockCoreClient) simulateNormalConfirmation(txid string) {
	defer m.wg.Done()

	// Normal TWINS block time is approximately 2 minutes
	blockTime := 2 * time.Minute

	// Simulate confirmations: 0 → 1 → 2 → 3 → 4 → 5 → 6+
	for conf := 1; conf <= 6; conf++ {
		// Wait for next block
		select {
		case <-m.ctx.Done():
			return
		case <-time.After(blockTime):
		}

		m.mu.Lock()
		tx, exists := m.transactions[txid]
		if !exists {
			m.mu.Unlock()
			return
		}

		// Update transaction confirmations
		tx.Confirmations = conf
		if conf == 1 {
			tx.BlockHeight = m.currentHeight
			tx.BlockHash = m.bestBlockHash
		}

		// At 6 confirmations, transaction is fully confirmed
		if conf == 6 {
			// Move from pending to confirmed (balance already deducted)
			// No balance change needed as we already deducted on send
		}

		// Emit confirmation event
		m.emitEventLocked(core.TransactionConfirmedEvent{
			BaseEvent:     core.BaseEvent{Type: "transaction_confirmed", Time: time.Now()},
			TxID:          txid,
			Confirmations: conf,
		})

		m.mu.Unlock()
	}
}

// GetTransaction implements CoreClient.GetTransaction
func (m *MockCoreClient) GetTransaction(txid string) (core.Transaction, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return core.Transaction{}, fmt.Errorf("core is not running")
	}

	tx, ok := m.transactions[txid]
	if !ok {
		return core.Transaction{}, fmt.Errorf("transaction not found: %s", txid)
	}

	return *tx, nil
}

// ListTransactions implements CoreClient.ListTransactions
func (m *MockCoreClient) ListTransactions(count int, skip int) ([]core.Transaction, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return nil, fmt.Errorf("core is not running")
	}

	// Handle default values
	if count <= 0 {
		count = 10
	}

	// Calculate slice bounds
	start := skip
	end := skip + count

	if start >= len(m.txList) {
		return []core.Transaction{}, nil
	}

	if end > len(m.txList) {
		end = len(m.txList)
	}

	// Build result list
	result := make([]core.Transaction, 0, end-start)
	for i := start; i < end; i++ {
		txid := m.txList[i]
		if tx, ok := m.transactions[txid]; ok {
			result = append(result, *tx)
		}
	}

	return result, nil
}

// ListTransactionsFiltered implements CoreClient.ListTransactionsFiltered
func (m *MockCoreClient) ListTransactionsFiltered(filter core.TransactionFilter) (core.TransactionPage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return core.TransactionPage{}, fmt.Errorf("core is not running")
	}

	totalAll := len(m.txList)
	pageSize := filter.PageSize

	// PageSize <= 0 means return all results (no pagination)
	if pageSize <= 0 {
		result := make([]core.Transaction, 0, totalAll)
		for _, txid := range m.txList {
			if tx, ok := m.transactions[txid]; ok {
				result = append(result, *tx)
			}
		}
		return core.TransactionPage{
			Transactions: result,
			Total:        totalAll,
			TotalAll:     totalAll,
			Page:         1,
			PageSize:     0,
			TotalPages:   1,
		}, nil
	}

	page := filter.Page
	if page < 1 {
		page = 1
	}

	// Simple mock: return a slice of all transactions with pagination only (no filtering)
	start := (page - 1) * pageSize
	end := start + pageSize
	if start >= totalAll {
		start = totalAll
	}
	if end > totalAll {
		end = totalAll
	}

	result := make([]core.Transaction, 0, end-start)
	for i := start; i < end; i++ {
		txid := m.txList[i]
		if tx, ok := m.transactions[txid]; ok {
			result = append(result, *tx)
		}
	}

	totalPages := 0
	if totalAll > 0 {
		totalPages = (totalAll + pageSize - 1) / pageSize
	}

	return core.TransactionPage{
		Transactions: result,
		Total:        totalAll,
		TotalAll:     totalAll,
		Page:         page,
		PageSize:     pageSize,
		TotalPages:   totalPages,
	}, nil
}

// ExportFilteredTransactionsCSV implements CoreClient.ExportFilteredTransactionsCSV
func (m *MockCoreClient) ExportFilteredTransactionsCSV(filter core.TransactionFilter) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return "", fmt.Errorf("core is not running")
	}

	// Simple mock: return CSV header + all transactions
	csv := "\"Confirmed\",\"Date\",\"Type\",\"Label\",\"Address\",\"Amount (TWINS)\",\"ID\"\n"
	for _, txid := range m.txList {
		if tx, ok := m.transactions[txid]; ok {
			confirmed := "false"
			if tx.Confirmations >= 6 {
				confirmed = "true"
			}
			csv += fmt.Sprintf("\"%s\",\"%s\",\"%s\",\"%s\",\"%s\",\"%.8f\",\"%s\"\n",
				confirmed, tx.Time.Format("2006-01-02T15:04:05"),
				string(tx.Type), tx.Label, tx.Address, tx.Amount, tx.TxID)
		}
	}

	return csv, nil
}

// ValidateAddress implements CoreClient.ValidateAddress
func (m *MockCoreClient) ValidateAddress(address string) (core.AddressValidation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return core.AddressValidation{}, fmt.Errorf("core is not running")
	}

	// Trim whitespace
	address = strings.TrimSpace(address)

	// TWINS address validation:
	// - Mainnet addresses start with 'D' (P2PKH) or 'T' (P2SH)
	// - Testnet addresses start with 'x' or 'y' (not supported yet)
	// - Length should be 26-35 characters (typically 34)
	// - Must be valid Base58 characters (no 0, O, I, l)
	// - Must have valid Base58Check checksum

	const (
		MinAddressLength = 26
		MaxAddressLength = 35
	)

	validation := core.AddressValidation{
		Address:     address,
		IsValid:     false,
		IsMine:      false,
		IsWatchOnly: false,
		IsScript:    false,
		PubKey:      "",
		Account:     "",
	}

	// Check length bounds
	addrLen := len(address)
	if addrLen < MinAddressLength || addrLen > MaxAddressLength {
		return validation, nil
	}

	// Check prefix - must be D or T for mainnet
	firstChar := address[0]
	if firstChar != 'D' && firstChar != 'T' {
		return validation, nil
	}

	// Check if all characters are valid Base58
	for _, c := range address {
		if !isValidBase58Char(c) {
			return validation, nil
		}
	}

	// Perform Base58Check checksum validation
	if !validateBase58Check(address) {
		return validation, nil
	}

	// If we get here, the address is valid
	validation.IsValid = true

	// Determine if this is a script address (P2SH)
	validation.IsScript = (firstChar == 'T')

	// Check if it's one of our addresses using the ownAddresses map
	pubkey, isMine := m.ownAddresses[address]
	validation.IsMine = isMine
	if isMine {
		validation.PubKey = pubkey
		validation.Account = "default" // Mock uses single account
	}

	// Simulate some watch-only addresses for testing
	// In production, these would be managed via importaddress RPC
	// NOTE: These are test fixtures for watch-only address behavior
	if validation.IsValid && !validation.IsMine {
		// Check against known watch-only test addresses
		// DLabKGMfKsV3xWZ3R9MFPJ1vJmr11xHWhh is a valid test address
		if address == "DLabKGMfKsV3xWZ3R9MFPJ1vJmr11xHWhh" {
			validation.IsWatchOnly = true
		}
	}

	return validation, nil
}

// isValidBase58Char checks if a rune is a valid Base58 character
func isValidBase58Char(c rune) bool {
	// Base58 excludes: 0, O, I, l to avoid confusion
	return (c >= '1' && c <= '9') ||
		(c >= 'A' && c <= 'H') ||
		(c >= 'J' && c <= 'N') ||
		(c >= 'P' && c <= 'Z') ||
		(c >= 'a' && c <= 'k') ||
		(c >= 'm' && c <= 'z')
}

// validateBase58Check performs proper Base58Check validation with checksum verification
// This validates that an address has a valid Base58Check encoding with proper checksum
func validateBase58Check(address string) bool {
	// Decode the Base58 string
	decoded := decodeBase58(address)

	// Base58Check addresses must be at least 25 bytes (21 bytes payload + 4 bytes checksum)
	if len(decoded) < 25 {
		return false
	}

	// Split into payload and checksum
	payload := decoded[:len(decoded)-4]
	checksum := decoded[len(decoded)-4:]

	// Calculate expected checksum: SHA256(SHA256(payload))
	hash1 := sha256.Sum256(payload)
	hash2 := sha256.Sum256(hash1[:])
	expectedChecksum := hash2[:4]

	// Compare checksums
	for i := 0; i < 4; i++ {
		if checksum[i] != expectedChecksum[i] {
			return false
		}
	}

	return true
}

// decodeBase58 decodes a Base58 string to bytes
func decodeBase58(input string) []byte {
	result := big.NewInt(0)
	base := big.NewInt(58)

	for _, c := range input {
		idx := strings.IndexRune(base58Alphabet, c)
		if idx == -1 {
			return nil // Invalid character
		}
		result.Mul(result, base)
		result.Add(result, big.NewInt(int64(idx)))
	}

	// Count leading '1's (which represent leading zero bytes)
	leadingZeros := 0
	for _, c := range input {
		if c != '1' {
			break
		}
		leadingZeros++
	}

	// Convert to bytes and prepend leading zeros
	decoded := result.Bytes()
	return append(make([]byte, leadingZeros), decoded...)
}

// EncryptWallet implements CoreClient.EncryptWallet
func (m *MockCoreClient) EncryptWallet(passphrase string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("core is not running")
	}

	if m.encrypted {
		return fmt.Errorf("wallet is already encrypted")
	}

	if len(passphrase) < 8 {
		return fmt.Errorf("passphrase too short: minimum 8 characters")
	}

	m.encrypted = true
	m.locked = true

	// Emit event
	m.emitEventLocked(core.WalletEncryptedEvent{
		BaseEvent: core.BaseEvent{Type: "wallet_encrypted", Time: time.Now()},
	})

	m.emitEventLocked(core.WalletLockedEvent{
		BaseEvent: core.BaseEvent{Type: "wallet_locked", Time: time.Now()},
	})

	return nil
}

// WalletLock implements CoreClient.WalletLock
func (m *MockCoreClient) WalletLock() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("core is not running")
	}

	if !m.encrypted {
		return fmt.Errorf("wallet is not encrypted")
	}

	m.locked = true
	m.unlockedUntil = time.Time{}

	// Emit event
	m.emitEventLocked(core.WalletLockedEvent{
		BaseEvent: core.BaseEvent{Type: "wallet_locked", Time: time.Now()},
	})

	return nil
}

// WalletPassphrase implements CoreClient.WalletPassphrase
func (m *MockCoreClient) WalletPassphrase(passphrase string, timeout int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("core is not running")
	}

	if !m.encrypted {
		return fmt.Errorf("wallet is not encrypted")
	}

	if len(passphrase) < 8 {
		return fmt.Errorf("invalid passphrase")
	}

	m.locked = false
	if timeout > 0 {
		m.unlockedUntil = time.Now().Add(time.Duration(timeout) * time.Second)

		// Start a timer to re-lock the wallet
		go func() {
			time.Sleep(time.Duration(timeout) * time.Second)
			m.mu.Lock()
			if !m.unlockedUntil.IsZero() && time.Now().After(m.unlockedUntil) {
				m.locked = true
				m.unlockedUntil = time.Time{}
				m.emitEventLocked(core.WalletLockedEvent{
					BaseEvent: core.BaseEvent{Type: "wallet_locked", Time: time.Now()},
				})
			}
			m.mu.Unlock()
		}()
	}

	// Emit event
	m.emitEventLocked(core.WalletUnlockedEvent{
		BaseEvent:      core.BaseEvent{Type: "wallet_unlocked", Time: time.Now()},
		TimeoutSeconds: timeout,
	})

	return nil
}

// WalletPassphraseChange implements CoreClient.WalletPassphraseChange
func (m *MockCoreClient) WalletPassphraseChange(oldPassphrase string, newPassphrase string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("core is not running")
	}

	if !m.encrypted {
		return fmt.Errorf("wallet is not encrypted")
	}

	if len(oldPassphrase) < 8 || len(newPassphrase) < 8 {
		return fmt.Errorf("passphrase too short: minimum 8 characters")
	}

	// In a real implementation, we would verify the old passphrase
	// For mock, we just accept it

	// Emit event
	m.emitEventLocked(core.WalletPassphraseChangedEvent{
		BaseEvent: core.BaseEvent{Type: "wallet_passphrase_changed", Time: time.Now()},
	})

	return nil
}

// GetWalletInfo implements CoreClient.GetWalletInfo
func (m *MockCoreClient) GetWalletInfo() (core.WalletInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return core.WalletInfo{}, fmt.Errorf("core is not running")
	}

	info := core.WalletInfo{
		Version:            169900,
		Balance:            m.balance.Total,
		UnconfirmedBalance: m.balance.Pending,
		ImmatureBalance:    m.balance.Immature,
		TxCount:            len(m.transactions),
		KeyPoolSize:        1000,
		KeyPoolOldest:      time.Now().Add(-365 * 24 * time.Hour),
		Unlocked:           !m.locked,
		UnlockedUntil:      m.unlockedUntil,
		Encrypted:          m.encrypted,
		PayTxFee:           0.001,
		HDMasterKeyID:      "",
	}

	return info, nil
}

// BackupWallet implements CoreClient.BackupWallet
func (m *MockCoreClient) BackupWallet(destination string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return fmt.Errorf("core is not running")
	}

	if destination == "" {
		return fmt.Errorf("destination path cannot be empty")
	}

	// In a real implementation, we would actually copy the wallet file
	// For mock, we just simulate success

	// Emit event (unlock to emit)
	m.mu.RUnlock()
	m.emitEvent(core.WalletBackupCompletedEvent{
		BaseEvent:   core.BaseEvent{Type: "wallet_backup_completed", Time: time.Now()},
		Destination: destination,
	})
	m.mu.RLock()

	return nil
}

// ListUnspent implements CoreClient.ListUnspent
func (m *MockCoreClient) ListUnspent(minConf int, maxConf int) ([]core.UTXO, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return nil, fmt.Errorf("core is not running")
	}

	// Default values
	if minConf <= 0 {
		minConf = 1
	}
	if maxConf <= 0 {
		maxConf = 9999999
	}

	// Filter UTXOs by confirmation count
	result := make([]core.UTXO, 0)
	for _, utxo := range m.utxos {
		if utxo.Confirmations >= minConf && utxo.Confirmations <= maxConf {
			result = append(result, utxo)
		}
	}

	return result, nil
}

// LockUnspent implements CoreClient.LockUnspent
// Locks or unlocks specified transaction outputs
func (m *MockCoreClient) LockUnspent(unlock bool, outputs []core.OutPoint) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("core is not running")
	}

	// Process each output
	for _, output := range outputs {
		key := fmt.Sprintf("%s:%d", output.TxID, output.Vout)

		if unlock {
			// Remove from locked coins
			delete(m.lockedCoins, key)

			// Update UTXO locked status
			for i := range m.utxos {
				if m.utxos[i].TxID == output.TxID && m.utxos[i].Vout == output.Vout {
					m.utxos[i].Locked = false
					break
				}
			}
		} else {
			// Add to locked coins
			m.lockedCoins[key] = true

			// Update UTXO locked status
			for i := range m.utxos {
				if m.utxos[i].TxID == output.TxID && m.utxos[i].Vout == output.Vout {
					m.utxos[i].Locked = true
					break
				}
			}
		}
	}

	return nil
}

// ListLockUnspent implements CoreClient.ListLockUnspent
// Returns list of temporarily locked outputs
func (m *MockCoreClient) ListLockUnspent() ([]core.OutPoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return nil, fmt.Errorf("core is not running")
	}

	result := make([]core.OutPoint, 0, len(m.lockedCoins))

	// Convert locked coins map to OutPoint slice
	for key := range m.lockedCoins {
		// Parse txid:vout format
		parts := strings.Split(key, ":")
		if len(parts) != 2 {
			continue // Skip malformed keys
		}

		var vout uint32
		_, err := fmt.Sscanf(parts[1], "%d", &vout)
		if err != nil {
			continue // Skip if vout is not a valid number
		}

		result = append(result, core.OutPoint{
			TxID: parts[0],
			Vout: vout,
		})
	}

	return result, nil
}

// EstimateFee implements CoreClient.EstimateFee
// Returns the fee rate in TWINS per KB for confirmation within targetBlocks
func (m *MockCoreClient) EstimateFee(blocks int) (float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return 0, fmt.Errorf("core is not running")
	}

	if blocks < 1 {
		return 0, fmt.Errorf("invalid target blocks: must be at least 1")
	}

	// Get current network congestion multiplier
	congestionMultiplier := m.getCongestionMultiplierLocked()

	// Determine base fee rate based on confirmation target
	var baseFee float64
	switch {
	case blocks == 1:
		// Fast: ~2 minutes (1 block)
		baseFee = FastFeeRate
	case blocks <= 3:
		// Normal: ~6 minutes (3 blocks)
		baseFee = DefaultFeeRate
	case blocks <= 6:
		// Slow: ~12 minutes (6 blocks)
		baseFee = DefaultFeeRate * 0.5
	default:
		// Very slow: minimum relay fee * 2
		baseFee = MinRelayFeeRate * 2
	}

	// Apply congestion multiplier
	estimatedFee := baseFee * congestionMultiplier

	// Ensure minimum relay fee
	if estimatedFee < MinRelayFeeRate {
		estimatedFee = MinRelayFeeRate
	}

	return estimatedFee, nil
}

// getCongestionMultiplierLocked returns the fee multiplier based on current congestion
// Caller must hold read lock
func (m *MockCoreClient) getCongestionMultiplierLocked() float64 {
	switch m.congestionLevel {
	case CongestionLow:
		return LowCongestionMultiplier
	case CongestionHigh:
		return HighCongestionMultiplier
	default:
		return NormalCongestionMultiplier
	}
}

// EstimateTransactionSize calculates approximate transaction size in bytes
func EstimateTransactionSize(numInputs, numOutputs int) (int, error) {
	if numInputs < 1 {
		return 0, fmt.Errorf("must have at least 1 input")
	}
	if numOutputs < 1 {
		return 0, fmt.Errorf("must have at least 1 output")
	}
	return TransactionBaseSize + (numInputs * TransactionInputSize) + (numOutputs * TransactionOutputSize), nil
}

// CalculateFee computes total fee for a transaction
func (m *MockCoreClient) CalculateFee(numInputs, numOutputs int, targetBlocks int) (float64, error) {
	// Get fee rate
	feeRate, err := m.EstimateFee(targetBlocks)
	if err != nil {
		return 0, err
	}

	// Calculate size with validation
	txSize, err := EstimateTransactionSize(numInputs, numOutputs)
	if err != nil {
		return 0, err
	}

	// Calculate fee (size in KB * rate per KB)
	fee := (float64(txSize) / 1000.0) * feeRate

	// Round up to nearest satoshi (0.00000001 TWINS)
	fee = math.Ceil(fee*100000000) / 100000000

	return fee, nil
}

// FeeEstimate contains fee estimates for different confirmation targets
type FeeEstimate struct {
	Fast   float64 // 1 block (~2 minutes)
	Normal float64 // 3 blocks (~6 minutes)
	Slow   float64 // 6 blocks (~12 minutes)
}

// GetFeeEstimates returns fee estimates for fast/normal/slow confirmation
func (m *MockCoreClient) GetFeeEstimates() (FeeEstimate, error) {
	fast, err := m.EstimateFee(1)
	if err != nil {
		return FeeEstimate{}, err
	}

	normal, err := m.EstimateFee(3)
	if err != nil {
		return FeeEstimate{}, err
	}

	slow, err := m.EstimateFee(6)
	if err != nil {
		return FeeEstimate{}, err
	}

	return FeeEstimate{
		Fast:   fast,
		Normal: normal,
		Slow:   slow,
	}, nil
}

// EstimateTransactionFee estimates the transaction fee based on recipients and options.
// This works even when the wallet is locked (no signing required).
// Returns the fee in TWINS, input count, and transaction size for UI display.
func (m *MockCoreClient) EstimateTransactionFee(recipients map[string]float64, opts *core.SendOptions) (*core.FeeEstimateResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return nil, fmt.Errorf("core is not running")
	}

	if len(recipients) == 0 {
		return nil, fmt.Errorf("no recipients specified")
	}

	// Calculate total output value
	totalOutput := 0.0
	for _, amount := range recipients {
		if amount <= 0 {
			return nil, fmt.Errorf("invalid amount: must be positive")
		}
		totalOutput += amount
	}

	// Determine fee rate (from options or default)
	var feeRate float64
	if opts != nil && opts.FeeRate > 0 {
		feeRate = opts.FeeRate
	} else {
		// Use default fee rate
		var err error
		feeRate, err = m.EstimateFee(3) // 3-block target for normal priority
		if err != nil {
			feeRate = DefaultFeeRate
		}
	}

	// Determine number of inputs
	var numInputs int
	if opts != nil && len(opts.SelectedUTXOs) > 0 {
		// Coin control: use specified UTXO count
		numInputs = len(opts.SelectedUTXOs)
	} else {
		// Automatic selection: estimate based on total amount
		// Simulate UTXO selection by counting how many we'd need
		numInputs = m.estimateInputCount(totalOutput, feeRate)
	}

	// Determine number of outputs
	splitCount := 1
	if opts != nil && opts.SplitCount > 1 {
		splitCount = opts.SplitCount
	}
	numOutputs := len(recipients)*splitCount + 1 // recipients * split + change

	// Calculate transaction size: 180 bytes/input + 34 bytes/output + 10 bytes base
	txSize := 180*numInputs + 34*numOutputs + 10

	// Calculate fee: (size in bytes * fee rate per KB) / 1000
	fee := (float64(txSize) / 1000.0) * feeRate

	// Round up to nearest satoshi
	fee = math.Ceil(fee*100000000) / 100000000

	// Ensure minimum relay fee
	if fee < MinRelayFeeRate {
		fee = MinRelayFeeRate
	}

	return &core.FeeEstimateResult{
		Fee:        fee,
		InputCount: numInputs,
		TxSize:     txSize,
	}, nil
}

// estimateInputCount estimates how many UTXOs would be needed for the given amount
func (m *MockCoreClient) estimateInputCount(amount float64, feeRate float64) int {
	// Start with estimate of 2 inputs
	numInputs := 2
	totalNeeded := amount

	// Iterate to find sufficient input count
	for i := 0; i < 10; i++ { // Max 10 iterations
		// Calculate fee for current input count (assume 2 outputs: recipient + change)
		txSize := 180*numInputs + 34*2 + 10
		fee := (float64(txSize) / 1000.0) * feeRate
		totalNeeded = amount + fee

		// Count how many UTXOs we'd need
		availableFromUTXOs := 0.0
		inputsNeeded := 0
		for _, utxo := range m.utxos {
			if !utxo.Locked && utxo.Confirmations >= 1 {
				availableFromUTXOs += utxo.Amount
				inputsNeeded++
				if availableFromUTXOs >= totalNeeded {
					break
				}
			}
		}

		if inputsNeeded == numInputs {
			break // Converged
		}
		numInputs = inputsNeeded
		if numInputs == 0 {
			numInputs = 1 // Minimum 1 input
			break
		}
	}

	return numInputs
}
