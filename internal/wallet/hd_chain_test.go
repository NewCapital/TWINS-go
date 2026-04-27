package wallet

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgcrypto "github.com/twins-dev/twins-core/pkg/crypto"
)

func TestErrHDChainNotFound(t *testing.T) {
	// On a fresh wallet (no CreateWallet called), ReadHDChain should return ErrHDChainNotFound
	wallet := createTestWallet(t)

	// Open wallet DB directly
	wdb, err := OpenWalletDB(wallet.config.DataDir)
	require.NoError(t, err)
	defer wdb.Close()

	chain, isEncrypted, err := wdb.ReadHDChain()
	assert.Nil(t, chain)
	assert.False(t, isEncrypted)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrHDChainNotFound), "expected ErrHDChainNotFound, got: %v", err)
}

func TestWriteHDChainInCreateWallet(t *testing.T) {
	wallet := createTestWallet(t)

	// Create wallet with seed (no encryption)
	seed := []byte("test seed for HD chain persistence verification!!")
	err := wallet.CreateWallet(seed, nil)
	require.NoError(t, err)

	// ReadHDChain should now succeed
	chain, isEncrypted, err := wallet.wdb.ReadHDChain()
	require.NoError(t, err, "ReadHDChain should succeed after CreateWallet")
	require.NotNil(t, chain)
	assert.False(t, isEncrypted, "unencrypted wallet should have unencrypted HD chain")

	// Verify ChainID matches SHA256d of seed
	expectedChainID := pkgcrypto.DoubleHash256(seed)
	assert.Equal(t, expectedChainID, chain.ChainID, "ChainID should be SHA256d of seed")

	// Verify counters match addrMgr state
	account, err := wallet.addrMgr.GetAccount(0)
	require.NoError(t, err)
	assert.Equal(t, account.ExternalChain.nextIndex, chain.ExternalCounter, "ExternalCounter should match addrMgr")
	assert.Equal(t, account.InternalChain.nextIndex, chain.InternalCounter, "InternalCounter should match addrMgr")
}

func TestIsHDEnabledAfterCreateWallet(t *testing.T) {
	wallet := createTestWallet(t)

	// Before CreateWallet, hdEnabled should be false
	assert.False(t, wallet.IsHDEnabled(), "IsHDEnabled should be false before CreateWallet")

	// Create wallet with seed
	seed := []byte("test seed for HD enabled after create wallet!!!!")
	err := wallet.CreateWallet(seed, nil)
	require.NoError(t, err)

	// After CreateWallet, hdEnabled should be true
	assert.True(t, wallet.IsHDEnabled(), "IsHDEnabled should be true after CreateWallet")
}

func TestIsHDEnabledAfterLoadWallet(t *testing.T) {
	wallet := createTestWallet(t)

	// Create wallet with seed
	seed := []byte("test seed for HD enabled after load wallet!!!!!")
	err := wallet.CreateWallet(seed, nil)
	require.NoError(t, err)
	assert.True(t, wallet.IsHDEnabled())

	// Close the wallet DB
	wallet.wdb.Close()

	// Create a new wallet instance pointing to same data dir and LoadWallet
	wallet2 := createTestWallet(t)
	wallet2.config.DataDir = wallet.config.DataDir
	assert.False(t, wallet2.IsHDEnabled(), "IsHDEnabled should be false before LoadWallet")

	err = wallet2.LoadWallet()
	require.NoError(t, err)
	defer wallet2.wdb.Close()

	assert.True(t, wallet2.IsHDEnabled(), "IsHDEnabled should be true after LoadWallet round-trip")
}

func TestIsHDEnabledFalseForNonHDWallet(t *testing.T) {
	wallet := createTestWallet(t)

	// A wallet that never had CreateWallet called has no HD chain
	assert.False(t, wallet.IsHDEnabled(), "IsHDEnabled should be false for non-HD wallet")
}

func TestEncryptedWalletHDSeed(t *testing.T) {
	wallet := createTestWallet(t)
	wallet.config.EncryptWallet = true

	// Create wallet with seed and passphrase
	seed := []byte("test seed for encrypted HD chain verification!!!!")
	passphrase := []byte("test-passphrase-123")
	err := wallet.CreateWallet(seed, passphrase)
	require.NoError(t, err)

	// ReadHDChain should return encrypted chain
	chain, isEncrypted, err := wallet.wdb.ReadHDChain()
	require.NoError(t, err, "ReadHDChain should succeed after encrypted CreateWallet")
	require.NotNil(t, chain)
	assert.True(t, isEncrypted, "encrypted wallet should have encrypted HD chain")

	// Seed should be encrypted (not equal to original)
	assert.NotEqual(t, seed, chain.Seed, "encrypted seed should differ from original")
	assert.Greater(t, len(chain.Seed), 0, "encrypted seed should not be empty")
}
