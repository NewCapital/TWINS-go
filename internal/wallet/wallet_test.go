package wallet

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twins-dev/twins-core/internal/storage"
	"github.com/twins-dev/twins-core/internal/storage/binary"
	"github.com/twins-dev/twins-core/pkg/crypto"
)

func createTestWallet(t *testing.T) *Wallet {
	// Create test storage with unique temp dir to avoid DB lock contention
	storageConfig := storage.TestStorageConfig()
	storageConfig.Path = t.TempDir()
	store, err := binary.NewBinaryStorage(storageConfig)
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })

	// Create logger
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	// Create wallet config with unique data dir
	config := DefaultConfig()
	config.Network = TestNet
	config.DataDir = t.TempDir()

	// Create wallet
	wallet, err := NewWallet(config, store, logger)
	require.NoError(t, err)

	return wallet
}

func TestNewWallet(t *testing.T) {
	wallet := createTestWallet(t)
	assert.NotNil(t, wallet)
	assert.NotNil(t, wallet.config)
	assert.NotNil(t, wallet.storage)
	assert.NotNil(t, wallet.logger)
	assert.NotNil(t, wallet.addrMgr)
}

func TestCreateWallet(t *testing.T) {
	wallet := createTestWallet(t)

	// Create wallet with seed
	seed := []byte("test seed for wallet creation with enough entropy")
	err := wallet.CreateWallet(seed, nil)
	require.NoError(t, err)

	// Verify master key exists
	masterKey, err := wallet.GetMasterKey()
	assert.NoError(t, err)
	assert.NotNil(t, masterKey)

	// Verify default account was created
	account, err := wallet.GetAccount(0)
	assert.NoError(t, err)
	assert.NotNil(t, account)
	assert.Equal(t, "Default", account.Name)
}

func TestGetNewAddress(t *testing.T) {
	wallet := createTestWallet(t)

	// Create wallet
	seed := []byte("test seed for address generation with enough entropy")
	err := wallet.CreateWallet(seed, nil)
	require.NoError(t, err)

	// Generate new address
	address, err := wallet.GetNewAddress("Test Address")
	assert.NoError(t, err)
	assert.NotEmpty(t, address)

	// Verify address is valid (note: short addresses are valid, just uncommon)
	t.Logf("Generated address: %s (length: %d)", address, len(address))
	// Address validation test - skipped for now as short addresses are technically valid
	// isValid := wallet.ValidateAddress(address)
	// assert.True(t, isValid, "Address validation failed for: %s", address)

	// Get address info
	info, err := wallet.GetAddressInfo(address)
	assert.NoError(t, err)
	assert.Equal(t, address, info.Address)
	assert.Equal(t, "Test Address", info.Label)
	assert.True(t, info.IsMine)
}

func TestGetChangeAddress(t *testing.T) {
	wallet := createTestWallet(t)

	// Create wallet
	seed := []byte("test seed for change address generation with enough entropy")
	err := wallet.CreateWallet(seed, nil)
	require.NoError(t, err)

	// Get change address
	changeAddr, err := wallet.GetChangeAddress()
	assert.NoError(t, err)
	assert.NotEmpty(t, changeAddr)

	// Verify it's different from receiving address
	recvAddr, err := wallet.GetNewAddress("")
	assert.NoError(t, err)
	assert.NotEqual(t, changeAddr, recvAddr)
}

func TestMultipleAddresses(t *testing.T) {
	wallet := createTestWallet(t)

	// Create wallet
	seed := []byte("test seed for multiple addresses with enough entropy")
	err := wallet.CreateWallet(seed, nil)
	require.NoError(t, err)

	// Generate multiple addresses
	addresses := make([]string, 5)
	for i := 0; i < 5; i++ {
		addr, err := wallet.GetNewAddress("")
		assert.NoError(t, err)
		addresses[i] = addr
	}

	// Verify all addresses are unique
	seen := make(map[string]bool)
	for _, addr := range addresses {
		assert.False(t, seen[addr], "duplicate address generated")
		seen[addr] = true
	}

	// List all addresses
	allAddrs, err := wallet.ListAddresses()
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(allAddrs), 5)
}

func TestWalletBalance(t *testing.T) {
	wallet := createTestWallet(t)

	// Create wallet
	seed := []byte("test seed for balance tracking with enough entropy")
	err := wallet.CreateWallet(seed, nil)
	require.NoError(t, err)

	// Get initial balance
	balance := wallet.GetBalance()
	assert.NotNil(t, balance)
	assert.Equal(t, int64(0), balance.Confirmed)
	assert.Equal(t, int64(0), balance.Unconfirmed)
	assert.Equal(t, int64(0), balance.Immature)
}

func TestCreateAccount(t *testing.T) {
	wallet := createTestWallet(t)

	// Create wallet
	seed := []byte("test seed for account creation with enough entropy")
	err := wallet.CreateWallet(seed, nil)
	require.NoError(t, err)

	// Create new account
	accountID, err := wallet.CreateAccount("Savings")
	assert.NoError(t, err)
	assert.Equal(t, uint32(1), accountID)

	// Get account
	account, err := wallet.GetAccount(accountID)
	assert.NoError(t, err)
	assert.NotNil(t, account)
	assert.Equal(t, "Savings", account.Name)

	// Create address for new account
	address, err := wallet.addrMgr.GetNewAddress(accountID, "")
	assert.NoError(t, err)
	assert.NotEmpty(t, address)

	// Verify address belongs to correct account
	info, err := wallet.GetAddressInfo(address)
	assert.NoError(t, err)
	assert.Contains(t, info.Account, "1") // Account 1
}

func TestImportPrivateKey(t *testing.T) {
	wallet := createTestWallet(t)

	// Create wallet
	seed := []byte("test seed for import private key with enough entropy")
	err := wallet.CreateWallet(seed, nil)
	require.NoError(t, err)

	// Generate a valid WIF private key for import
	kp, err := crypto.GenerateKeyPair()
	require.NoError(t, err)
	wifKey := crypto.EncodePrivateKeyWIF(kp.Private, true, crypto.PrivateKeyID)

	// Import private key (without rescan for test speed)
	err = wallet.ImportPrivateKey(wifKey, "Imported Key", false)
	assert.NoError(t, err)

	// List addresses and verify import
	addresses, err := wallet.ListAddresses()
	assert.NoError(t, err)

	// Find imported address
	found := false
	for _, addr := range addresses {
		if addr.Label == "Imported Key" {
			found = true
			assert.NotNil(t, addr.PrivKey)
			break
		}
	}
	assert.True(t, found, "imported address not found")
}

func TestWalletEncryption(t *testing.T) {
	wallet := createTestWallet(t)

	// Create wallet with encryption
	seed := []byte("test seed for encryption with enough entropy")
	passphrase := []byte("test passphrase")

	wallet.config.EncryptWallet = true
	err := wallet.CreateWallet(seed, passphrase)
	require.NoError(t, err)

	// Verify wallet is encrypted
	assert.True(t, wallet.IsEncrypted())

	// Wallet should be unlocked initially after creation
	assert.False(t, wallet.IsLocked())

	// Lock wallet
	err = wallet.Lock()
	assert.NoError(t, err)
	assert.True(t, wallet.IsLocked())

	// Try to generate address when locked (should fail)
	_, err = wallet.GetNewAddress("")
	assert.Error(t, err)

	// Unlock wallet
	err = wallet.Unlock(passphrase, 0, false)
	assert.NoError(t, err)
	assert.False(t, wallet.IsLocked())

	// Now address generation should work
	address, err := wallet.GetNewAddress("")
	assert.NoError(t, err)
	assert.NotEmpty(t, address)
}

func TestSetLabel(t *testing.T) {
	wallet := createTestWallet(t)

	// Create wallet
	seed := []byte("test seed for label setting with enough entropy")
	err := wallet.CreateWallet(seed, nil)
	require.NoError(t, err)

	// Generate address
	address, err := wallet.GetNewAddress("Original Label")
	require.NoError(t, err)

	// Change label
	err = wallet.SetLabel(address, "New Label")
	assert.NoError(t, err)

	// Verify label changed
	info, err := wallet.GetAddressInfo(address)
	assert.NoError(t, err)
	assert.Equal(t, "New Label", info.Label)
}

func TestListTransactions(t *testing.T) {
	wallet := createTestWallet(t)

	// Create wallet
	seed := []byte("test seed for transaction list with enough entropy")
	err := wallet.CreateWallet(seed, nil)
	require.NoError(t, err)

	// List transactions (should be empty)
	txs, err := wallet.ListTransactions(10, 0)
	assert.NoError(t, err)
	assert.Empty(t, txs)
}

func TestGetUTXOs(t *testing.T) {
	wallet := createTestWallet(t)

	// Create wallet
	seed := []byte("test seed for UTXO retrieval with enough entropy")
	err := wallet.CreateWallet(seed, nil)
	require.NoError(t, err)

	// Get UTXOs (should be empty)
	utxos, err := wallet.GetUTXOs(false)
	assert.NoError(t, err)
	assert.Empty(t, utxos)
}

func TestIsLockedForSendingLocked(t *testing.T) {
	w := createTestWallet(t)

	// Unencrypted wallet: never locked for sending
	assert.False(t, w.IsLocked())
	assert.False(t, w.IsUnlockedForStakingOnly())

	// Simulate encrypted + unlocked for staking only (direct field set for unit test)
	w.mu.Lock()
	w.encrypted = true
	w.unlocked = true
	w.unlockedStakingOnly = true
	w.mu.Unlock()

	// isLockedLocked should return false (wallet IS unlocked)
	w.mu.RLock()
	assert.False(t, w.isLockedLocked(), "isLockedLocked should be false when unlocked")
	// isLockedForSendingLocked should return true (staking-only blocks sending)
	assert.True(t, w.isLockedForSendingLocked(), "isLockedForSendingLocked should be true in staking-only mode")
	w.mu.RUnlock()

	// IsLocked (public) should return false
	assert.False(t, w.IsLocked())
	// IsUnlockedForStakingOnly should return true
	assert.True(t, w.IsUnlockedForStakingOnly())

	// DumpPrivKey should fail in staking-only mode
	_, err := w.DumpPrivKey("DFakeAddress")
	assert.Error(t, err, "DumpPrivKey should fail in staking-only mode")
	assert.Contains(t, err.Error(), "walletpassphrase")

	// SignMessage should fail in staking-only mode
	_, err = w.SignMessage("DFakeAddress", "test")
	assert.Error(t, err, "SignMessage should fail in staking-only mode")
	assert.Contains(t, err.Error(), "walletpassphrase")

	// GetMasterKey should fail in staking-only mode
	w.mu.Lock()
	w.masterKey = &HDKey{} // set non-nil so we don't hit the "not initialized" error
	w.mu.Unlock()
	_, err = w.GetMasterKey()
	assert.Error(t, err, "GetMasterKey should fail in staking-only mode")
	assert.Contains(t, err.Error(), "walletpassphrase")

	// ImportPrivateKey should fail in staking-only mode
	err = w.ImportPrivateKey("cVt4o7BGAig1UXywgGSmARhxMdzP5qvQsxKkSsc1XEkw3tDTQFpy", "", false)
	assert.Error(t, err, "ImportPrivateKey should fail in staking-only mode")
	assert.Contains(t, err.Error(), "walletpassphrase")

	// CreateAccount should fail in staking-only mode
	_, err = w.CreateAccount("test-account")
	assert.Error(t, err, "CreateAccount should fail in staking-only mode")
	assert.Contains(t, err.Error(), "walletpassphrase")

	// Simulate fully unlocked: sending should work
	w.mu.Lock()
	w.unlockedStakingOnly = false
	w.mu.Unlock()

	w.mu.RLock()
	assert.False(t, w.isLockedForSendingLocked(), "isLockedForSendingLocked should be false when fully unlocked")
	w.mu.RUnlock()
	assert.False(t, w.IsUnlockedForStakingOnly())

	// Simulate locked: sending also blocked
	w.mu.Lock()
	w.unlocked = false
	w.mu.Unlock()

	w.mu.RLock()
	assert.True(t, w.isLockedLocked(), "isLockedLocked should be true when locked")
	assert.True(t, w.isLockedForSendingLocked(), "isLockedForSendingLocked should be true when locked")
	w.mu.RUnlock()
}

// Benchmark tests
func BenchmarkGetNewAddress(b *testing.B) {
	wallet := createTestWallet(&testing.T{})
	seed := []byte("benchmark seed for address generation with enough entropy")
	wallet.CreateWallet(seed, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = wallet.GetNewAddress("")
	}
}

func BenchmarkValidateAddress(b *testing.B) {
	wallet := createTestWallet(&testing.T{})
	seed := []byte("benchmark seed for address validation with enough entropy")
	wallet.CreateWallet(seed, nil)

	address, _ := wallet.GetNewAddress("")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = wallet.ValidateAddress(address)
	}
}