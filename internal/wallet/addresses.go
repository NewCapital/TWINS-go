package wallet

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/twins-dev/twins-core/internal/wallet/legacy"
	"github.com/twins-dev/twins-core/pkg/crypto"
)

// AddressManager manages wallet addresses
type AddressManager struct {
	wallet   *Wallet
	accounts map[uint32]*Account
	pool     *AddressPool
	mu       sync.RWMutex
}

// AddressPool maintains a pool of generated addresses
type AddressPool struct {
	external []*Address // Receiving addresses
	internal []*Address // Change addresses
	used     map[string]bool
	mu       sync.RWMutex
}

// NewAddressManager creates a new address manager
func NewAddressManager(wallet *Wallet) *AddressManager {
	return &AddressManager{
		wallet:   wallet,
		accounts: make(map[uint32]*Account),
		pool: &AddressPool{
			external: make([]*Address, 0),
			internal: make([]*Address, 0),
			used:     make(map[string]bool),
		},
	}
}

// GetNewAddress generates a new receiving address for an account
func (am *AddressManager) GetNewAddress(accountID uint32, label string) (string, error) {
	am.mu.Lock()
	defer am.mu.Unlock()

	// Check if this is an HD wallet or legacy non-HD wallet
	if am.wallet.masterKey == nil {
		// Non-HD legacy wallet - generate random address
		address, err := am.generateRandomAddress(label)
		if err != nil {
			return "", fmt.Errorf("failed to generate random address: %w", err)
		}

		// Add to wallet addresses
		am.wallet.addresses[address.Address] = address

		// Also add to binary address map for fast script matching
		if binaryKey, ok := am.wallet.addressToBinaryKey(address.Address); ok {
			am.wallet.addressesBinary[binaryKey] = address
		}

		// Add to pool
		am.pool.mu.Lock()
		am.pool.external = append(am.pool.external, address)
		am.pool.mu.Unlock()

		return address.Address, nil
	}

	// HD wallet - derive from master key
	account, exists := am.accounts[accountID]
	if !exists {
		// Create new account if it doesn't exist
		var err error
		account, err = am.createAccount(accountID, fmt.Sprintf("Account %d", accountID))
		if err != nil {
			return "", fmt.Errorf("failed to create account: %w", err)
		}
	}

	// Generate next address in external chain
	address, err := am.deriveNextAddress(account, false)
	if err != nil {
		return "", fmt.Errorf("failed to derive address: %w", err)
	}

	address.Label = label
	address.Used = false

	// Persist label to wallet.dat for HD addresses (non-HD handles this in generateRandomAddress)
	if label != "" && am.wallet.wdb != nil {
		if err := am.wallet.wdb.WriteName(address.Address, label); err != nil {
			return "", fmt.Errorf("failed to save address label: %w", err)
		}
	}

	// Add to wallet addresses
	am.wallet.addresses[address.Address] = address

	// Also add to binary address map for fast script matching
	if binaryKey, ok := am.wallet.addressToBinaryKey(address.Address); ok {
		am.wallet.addressesBinary[binaryKey] = address
	}

	// Add to pool
	am.pool.mu.Lock()
	am.pool.external = append(am.pool.external, address)
	am.pool.mu.Unlock()

	return address.Address, nil
}

// GetChangeAddress gets or generates a change address for an account
func (am *AddressManager) GetChangeAddress(accountID uint32) (string, error) {
	am.mu.Lock()
	defer am.mu.Unlock()

	// Check if this is an HD wallet or legacy non-HD wallet
	if am.wallet.masterKey == nil {
		// Non-HD legacy wallet - generate random address as change address
		address, err := am.generateRandomAddress("")
		if err != nil {
			return "", fmt.Errorf("failed to generate random change address: %w", err)
		}

		address.Internal = true // Mark as change address

		// Add to wallet addresses
		am.wallet.addresses[address.Address] = address

		// Also add to binary address map for fast script matching
		if binaryKey, ok := am.wallet.addressToBinaryKey(address.Address); ok {
			am.wallet.addressesBinary[binaryKey] = address
		}

		// Add to pool
		am.pool.mu.Lock()
		am.pool.internal = append(am.pool.internal, address)
		am.pool.mu.Unlock()

		return address.Address, nil
	}

	// HD wallet - derive from master key
	account, exists := am.accounts[accountID]
	if !exists {
		// Create new account if it doesn't exist
		var err error
		account, err = am.createAccount(accountID, fmt.Sprintf("Account %d", accountID))
		if err != nil {
			return "", fmt.Errorf("failed to create account: %w", err)
		}
	}

	// Check for unused change address in pool
	am.pool.mu.RLock()
	for _, addr := range am.pool.internal {
		if addr.Account == accountID && !addr.Used {
			am.pool.mu.RUnlock()
			return addr.Address, nil
		}
	}
	am.pool.mu.RUnlock()

	// Generate new change address
	address, err := am.deriveNextAddress(account, true)
	if err != nil {
		return "", fmt.Errorf("failed to derive change address: %w", err)
	}

	address.Used = false
	address.Internal = true

	// Add to wallet addresses
	am.wallet.addresses[address.Address] = address

	// Also add to binary address map for fast script matching
	if binaryKey, ok := am.wallet.addressToBinaryKey(address.Address); ok {
		am.wallet.addressesBinary[binaryKey] = address
	}

	// Add to pool
	am.pool.mu.Lock()
	am.pool.internal = append(am.pool.internal, address)
	am.pool.mu.Unlock()

	return address.Address, nil
}

// GetAddressInfo returns detailed information about an address
func (am *AddressManager) GetAddressInfo(address string) (*AddressInfo, error) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	addr, exists := am.wallet.addresses[address]
	if !exists {
		return nil, fmt.Errorf("address not found")
	}

	// Build HD key path string
	keyPath := fmt.Sprintf("m/44'/%d'/%d'/%d/%d",
		TWINSCoinType-HardenedKeyStart,
		addr.Account,
		boolToUint32(addr.Internal),
		addr.Index,
	)

	info := &AddressInfo{
		Address:      addr.Address,
		Account:      fmt.Sprintf("Account %d", addr.Account),
		Label:        addr.Label,
		ScriptType:   addr.ScriptType,
		IsCompressed: true,
		IsWatchOnly:  addr.PrivKey == nil,
		IsMine:       addr.PrivKey != nil,
		IsValid:      true,
		HDKeyPath:    keyPath,
	}

	if addr.PubKey != nil {
		info.PubKey = addr.PubKey.CompressedHex()
	}

	return info, nil
}

// ListAddresses returns all addresses for an account
func (am *AddressManager) ListAddresses(accountID uint32) ([]*Address, error) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	addresses := make([]*Address, 0)
	for _, addr := range am.wallet.addresses {
		if addr.Account == accountID {
			addresses = append(addresses, addr)
		}
	}

	return addresses, nil
}

// ImportAddress imports an address for watch-only purposes
func (am *AddressManager) ImportAddress(address string, label string, rescan bool) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	// Check if address already exists
	if _, exists := am.wallet.addresses[address]; exists {
		return fmt.Errorf("address already exists")
	}

	// Validate address format
	if err := crypto.ValidateAddress(address); err != nil {
		return fmt.Errorf("invalid address: %w", err)
	}

	// Create watch-only address
	addr := &Address{
		Address:    address,
		PubKey:     nil,
		PrivKey:    nil,
		ScriptType: ScriptTypeP2PKH,
		Account:    0, // Watch-only addresses go to account 0
		Used:       false,
		Label:      label,
	}

	am.wallet.addresses[address] = addr

	// Also add to binary address map for fast script matching
	if binaryKey, ok := am.wallet.addressToBinaryKey(address); ok {
		am.wallet.addressesBinary[binaryKey] = addr
	}

	// Trigger blockchain rescan if requested
	if rescan {
		// Rescan from wallet's sync height to find historical transactions for this address
		// This will scan the blockchain and update the wallet's UTXO set and transaction history
		am.wallet.logger.WithFields(logrus.Fields{
			"address": address,
			"height":  am.wallet.syncHeight,
		}).Info("Triggering blockchain rescan for imported address")

		// Note: Blockchain rescan integration requires blockchain interface to be available
		// When blockchain interface is integrated, the rescan should:
		// 1. Start from the wallet's current sync height (or specified height)
		// 2. Scan all blocks for transactions involving this address
		// 3. Update UTXO set with any outputs to this address
		// 4. Update transaction history with any relevant transactions
		// 5. Update balance accordingly
		// For now, rescan flag is acknowledged but not executed
	}

	return nil
}

// SetAddressLabel sets the label for an address
func (am *AddressManager) SetAddressLabel(address string, label string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	addr, exists := am.wallet.addresses[address]
	if !exists {
		return fmt.Errorf("address not found")
	}

	addr.Label = label
	return nil
}

// MarkAddressUsed marks an address as used
func (am *AddressManager) MarkAddressUsed(address string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	addr, exists := am.wallet.addresses[address]
	if !exists {
		return fmt.Errorf("address not found")
	}

	addr.Used = true
	am.pool.mu.Lock()
	am.pool.used[address] = true
	am.pool.mu.Unlock()

	return nil
}

// IsAddressUsed checks if an address has been used
func (am *AddressManager) IsAddressUsed(address string) bool {
	am.pool.mu.RLock()
	defer am.pool.mu.RUnlock()
	return am.pool.used[address]
}

// GetReceivingAddresses returns external addresses that have been labeled or used
// This filters out pre-generated keypool addresses that haven't been explicitly used
func (am *AddressManager) GetReceivingAddresses() []*Address {
	am.pool.mu.RLock()
	defer am.pool.mu.RUnlock()

	// Only return addresses with labels or that have been used
	// This matches legacy Qt wallet behavior where keypool addresses are hidden
	result := make([]*Address, 0)
	for _, addr := range am.pool.external {
		if addr.Label != "" || addr.Used {
			result = append(result, addr)
		}
	}
	return result
}

// GetAllReceivingAddresses returns all external addresses including keypool
// Use this when you need the full address pool (e.g., for scanning)
func (am *AddressManager) GetAllReceivingAddresses() []*Address {
	am.pool.mu.RLock()
	defer am.pool.mu.RUnlock()

	result := make([]*Address, len(am.pool.external))
	copy(result, am.pool.external)
	return result
}

// GetAccount returns an account by ID
func (am *AddressManager) GetAccount(accountID uint32) (*Account, error) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	account, exists := am.accounts[accountID]
	if !exists {
		return nil, fmt.Errorf("account not found: %d", accountID)
	}

	return account, nil
}

// CreateAccount creates a new account
func (am *AddressManager) CreateAccount(accountID uint32, name string) (*Account, error) {
	am.mu.Lock()
	defer am.mu.Unlock()

	return am.createAccount(accountID, name)
}

// createAccount creates a new account (internal, must be called with lock held)
func (am *AddressManager) createAccount(accountID uint32, name string) (*Account, error) {
	// Check if account already exists
	if _, exists := am.accounts[accountID]; exists {
		return nil, fmt.Errorf("account already exists: %d", accountID)
	}

	// Derive account key from master key
	path := &KeyPath{
		Purpose:  44,
		CoinType: TWINSCoinType - HardenedKeyStart,
		Account:  accountID,
		Change:   0,
		Index:    0,
	}

	accountKey, err := am.wallet.masterKey.DerivePath(path)
	if err != nil {
		return nil, fmt.Errorf("failed to derive account key: %w", err)
	}

	// Create address chains
	externalChain := &AddressChain{
		internal:  false,
		nextIndex: 0,
		addresses: make([]*Address, 0),
		gap:       0,
		maxGap:    am.wallet.config.AccountLookahead,
	}

	internalChain := &AddressChain{
		internal:  true,
		nextIndex: 0,
		addresses: make([]*Address, 0),
		gap:       0,
		maxGap:    am.wallet.config.AccountLookahead,
	}

	account := &Account{
		ID:            accountID,
		Name:          name,
		ExtendedKey:   accountKey,
		ExternalChain: externalChain,
		InternalChain: internalChain,
		Balance: &Balance{
			Confirmed:   0,
			Unconfirmed: 0,
			Immature:    0,
		},
	}

	externalChain.account = account
	internalChain.account = account

	am.accounts[accountID] = account
	am.wallet.accounts[accountID] = account

	return account, nil
}

// deriveNextAddress derives the next address in a chain
// generateRandomAddress generates a non-HD address with a random private key
func (am *AddressManager) generateRandomAddress(label string) (*Address, error) {
	// Generate random 32-byte private key
	privKeyBytes := make([]byte, 32)
	if _, err := rand.Read(privKeyBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random private key: %w", err)
	}

	// Parse private key
	privKey, err := crypto.ParsePrivateKeyFromBytes(privKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// Get public key
	pubKey := privKey.PublicKey()

	// Generate address
	var netID byte
	switch am.wallet.config.Network {
	case MainNet:
		netID = crypto.MainNetPubKeyHashAddrID
	case TestNet, RegTest:
		netID = crypto.TestNetPubKeyHashAddrID
	default:
		netID = crypto.MainNetPubKeyHashAddrID
	}

	addr := crypto.NewAddressFromPubKey(pubKey, netID)
	addressStr := addr.String()

	address := &Address{
		Address:    addressStr,
		PubKey:     pubKey,
		PrivKey:    privKey,
		ScriptType: ScriptTypeP2PKH,
		Account:    0,
		Index:      0, // No index for non-HD keys
		Internal:   false,
		Used:       false,
		Label:      label,
		CreatedAt:  time.Now(),
	}

	// Save key to wallet.dat if WalletDB is initialized
	if am.wallet.wdb != nil {
		// Create key metadata (no HD path for random keys)
		metadata := &legacy.CKeyMetadata{
			Version:    1,
			CreateTime: time.Now().Unix(),
		}

		// Write key to database
		pubKeyBytes := pubKey.CompressedBytes()
		if err := am.wallet.wdb.WriteKey(pubKeyBytes, privKeyBytes, metadata); err != nil {
			return nil, fmt.Errorf("failed to save key to wallet.dat: %w", err)
		}

		// Write label if provided
		if label != "" {
			if err := am.wallet.wdb.WriteName(addressStr, label); err != nil {
				return nil, fmt.Errorf("failed to save address label: %w", err)
			}
		}
	}

	return address, nil
}

func (am *AddressManager) deriveNextAddress(account *Account, internal bool) (*Address, error) {
	var chain *AddressChain
	if internal {
		chain = account.InternalChain
	} else {
		chain = account.ExternalChain
	}

	// Check if master key is available
	if am.wallet.masterKey == nil {
		return nil, fmt.Errorf("HD wallet master key not available - wallet may need to be unlocked")
	}

	// Derive key at next index
	path := &KeyPath{
		Purpose:  44,
		CoinType: TWINSCoinType - HardenedKeyStart,
		Account:  account.ID,
		Change:   boolToUint32(internal),
		Index:    chain.nextIndex,
	}

	key, err := am.wallet.masterKey.DerivePath(path)
	if err != nil {
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}

	// Generate address
	addressStr, err := key.GetAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to get address: %w", err)
	}

	address := &Address{
		Address:    addressStr,
		PubKey:     key.PublicKey(),
		PrivKey:    key.PrivateKey(),
		ScriptType: ScriptTypeP2PKH,
		Account:    account.ID,
		Index:      chain.nextIndex,
		Internal:   internal,
		Used:       false,
		Label:      "",
		CreatedAt:  time.Now(),
	}

	// Save key to wallet.dat if WalletDB is initialized
	if am.wallet.wdb != nil && address.PubKey != nil && address.PrivKey != nil {
		// Get master key fingerprint
		var masterKeyID []byte
		if am.wallet.masterKey != nil {
			masterKeyID = am.wallet.masterKey.Fingerprint()
		}

		// Create key metadata
		metadata := &legacy.CKeyMetadata{
			Version:       1,
			CreateTime:    time.Now().Unix(),
			HDKeyPath:     fmt.Sprintf("m/44'/%d'/%d'/%d/%d", path.CoinType, path.Account, path.Change, path.Index),
			HDMasterKeyID: masterKeyID,
		}

		// Write key to database
		pubKeyBytes := address.PubKey.CompressedBytes()
		privKeyBytes := address.PrivKey.Bytes()
		if err := am.wallet.wdb.WriteKey(pubKeyBytes, privKeyBytes, metadata); err != nil {
			return nil, fmt.Errorf("failed to save key to wallet.dat: %w", err)
		}
	}

	chain.addresses = append(chain.addresses, address)
	chain.nextIndex++

	return address, nil
}

// FillAddressPool generates addresses up to the lookahead limit
func (am *AddressManager) FillAddressPool(accountID uint32) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	account, exists := am.accounts[accountID]
	if !exists {
		return fmt.Errorf("account not found: %d", accountID)
	}

	// Fill external chain
	for len(account.ExternalChain.addresses) < am.wallet.config.AccountLookahead {
		addr, err := am.deriveNextAddress(account, false)
		if err != nil {
			return fmt.Errorf("failed to derive external address: %w", err)
		}
		am.wallet.addresses[addr.Address] = addr

		// Also add to binary address map for fast script matching
		if binaryKey, ok := am.wallet.addressToBinaryKey(addr.Address); ok {
			am.wallet.addressesBinary[binaryKey] = addr
		}

		am.pool.mu.Lock()
		am.pool.external = append(am.pool.external, addr)
		am.pool.mu.Unlock()
	}

	// Fill internal chain
	for len(account.InternalChain.addresses) < am.wallet.config.AccountLookahead {
		addr, err := am.deriveNextAddress(account, true)
		if err != nil {
			return fmt.Errorf("failed to derive internal address: %w", err)
		}
		am.wallet.addresses[addr.Address] = addr

		// Also add to binary address map for fast script matching
		if binaryKey, ok := am.wallet.addressToBinaryKey(addr.Address); ok {
			am.wallet.addressesBinary[binaryKey] = addr
		}

		am.pool.mu.Lock()
		am.pool.internal = append(am.pool.internal, addr)
		am.pool.mu.Unlock()
	}

	return nil
}

// GetAddressPrivateKey returns the private key for an address
func (am *AddressManager) GetAddressPrivateKey(address string) (*crypto.PrivateKey, error) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	addr, exists := am.wallet.addresses[address]
	if !exists {
		return nil, fmt.Errorf("address not found")
	}

	if addr.PrivKey == nil {
		return nil, fmt.Errorf("address is watch-only")
	}

	return addr.PrivKey, nil
}

// ValidateAddress validates an address format
func (am *AddressManager) ValidateAddress(address string) bool {
	// Decode base58
	decoded, err := crypto.Base58Decode(address)
	if err != nil {
		return false
	}

	// Check minimum length (version byte + 20 byte hash + 4 byte checksum)
	if len(decoded) < 25 {
		return false
	}

	// Extract checksum
	payload := decoded[:len(decoded)-4]
	checksum := decoded[len(decoded)-4:]

	// Calculate expected checksum
	expectedChecksum := crypto.DoubleHash256(payload)[:4]

	// Compare checksums
	for i := 0; i < 4; i++ {
		if checksum[i] != expectedChecksum[i] {
			return false
		}
	}

	return true
}

// Helper function to convert bool to uint32
func boolToUint32(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

// RefillKeypool refills the address keypool to the specified size
func (am *AddressManager) RefillKeypool(accountID uint32, newsize int) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	// Legacy wallets (non-HD) don't support keypoolrefill
	// They use pre-generated keys from wallet.dat
	if am.wallet.masterKey == nil {
		return fmt.Errorf("keypoolrefill not supported for legacy wallets")
	}

	account, exists := am.accounts[accountID]
	if !exists {
		return fmt.Errorf("account not found: %d", accountID)
	}

	// If newsize is specified and positive, update the lookahead
	if newsize > 0 {
		am.wallet.config.AccountLookahead = newsize
	}

	targetSize := am.wallet.config.AccountLookahead

	// Initialize chains if nil
	if account.ExternalChain == nil {
		account.ExternalChain = &AddressChain{
			account:   account,
			internal:  false,
			nextIndex: 0,
			addresses: make([]*Address, 0),
			gap:       0,
		}
	}
	if account.InternalChain == nil {
		account.InternalChain = &AddressChain{
			account:   account,
			internal:  true,
			nextIndex: 0,
			addresses: make([]*Address, 0),
			gap:       0,
		}
	}

	// Early return if CHAIN already has sufficient addresses
	// The chain is the source of truth - pool is just a cache
	// This avoids expensive derivation for wallets with large transaction history
	if len(account.ExternalChain.addresses) >= targetSize &&
	   len(account.InternalChain.addresses) >= targetSize {
		return nil
	}

	// DON'T clear existing pool - just add missing addresses to both chain and pool
	// This avoids expensive regeneration when chain is partially full

	// Refill the pool with the new size
	// Fill external chain
	for len(account.ExternalChain.addresses) < targetSize {
		addr, err := am.deriveNextAddress(account, false)
		if err != nil {
			return fmt.Errorf("failed to derive external address: %w", err)
		}
		am.wallet.addresses[addr.Address] = addr

		// Also add to binary address map
		if binaryKey, ok := am.wallet.addressToBinaryKey(addr.Address); ok {
			am.wallet.addressesBinary[binaryKey] = addr
		}

		am.pool.mu.Lock()
		am.pool.external = append(am.pool.external, addr)
		am.pool.mu.Unlock()
	}

	// Fill internal chain
	for len(account.InternalChain.addresses) < targetSize {
		addr, err := am.deriveNextAddress(account, true)
		if err != nil {
			return fmt.Errorf("failed to derive internal address: %w", err)
		}
		am.wallet.addresses[addr.Address] = addr

		// Also add to binary address map
		if binaryKey, ok := am.wallet.addressToBinaryKey(addr.Address); ok {
			am.wallet.addressesBinary[binaryKey] = addr
		}

		am.pool.mu.Lock()
		am.pool.internal = append(am.pool.internal, addr)
		am.pool.mu.Unlock()
	}

	return nil
}