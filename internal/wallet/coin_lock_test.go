package wallet

import (
	"sync"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twins-dev/twins-core/internal/storage"
	"github.com/twins-dev/twins-core/internal/storage/binary"
	"github.com/twins-dev/twins-core/pkg/types"
)

// mockCollateralChecker implements MasternodeCollateralChecker for testing
type mockCollateralChecker struct {
	collaterals map[types.Outpoint]bool
}

func (m *mockCollateralChecker) IsCollateralOutpoint(outpoint types.Outpoint) bool {
	return m.collaterals[outpoint]
}

// makeOutpoint creates a deterministic outpoint for testing
func makeOutpoint(txByte byte, index uint32) types.Outpoint {
	var hash types.Hash
	hash[0] = txByte
	return types.Outpoint{Hash: hash, Index: index}
}

// createIsolatedWallet creates a wallet with its own Pebble DB in a unique temp dir.
// Each test gets an isolated DB, avoiding Pebble lock contention.
func createIsolatedWallet(t *testing.T) *Wallet {
	t.Helper()
	storageConfig := storage.TestStorageConfig()
	storageConfig.Path = t.TempDir()
	store, err := binary.NewBinaryStorage(storageConfig)
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })

	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	config := DefaultConfig()
	config.Network = TestNet

	w, err := NewWallet(config, store, logger)
	require.NoError(t, err)
	return w
}

// addTestUTXO adds a spendable UTXO to a wallet. Caller must hold w.mu.
func addTestUTXO(w *Wallet, outpoint types.Outpoint, value int64, blockHeight int32) {
	w.utxos[outpoint] = &UTXO{
		Outpoint:    outpoint,
		Output:      &types.TxOutput{Value: value, ScriptPubKey: []byte{0x76, 0xa9}},
		BlockHeight: blockHeight,
		Spendable:   true,
		Address:     "TestAddr1",
	}
}

func TestLockCoin(t *testing.T) {
	w := createIsolatedWallet(t)
	op := makeOutpoint(0x01, 0)

	// Initially not locked
	assert.False(t, w.IsLockedCoin(op))

	// Lock it
	w.LockCoin(op)
	assert.True(t, w.IsLockedCoin(op))

	// Lock same coin again (idempotent)
	w.LockCoin(op)
	assert.True(t, w.IsLockedCoin(op))
}

func TestUnlockCoin(t *testing.T) {
	w := createIsolatedWallet(t)
	op := makeOutpoint(0x02, 0)

	w.LockCoin(op)
	assert.True(t, w.IsLockedCoin(op))

	w.UnlockCoin(op)
	assert.False(t, w.IsLockedCoin(op))

	// Unlock non-locked coin is a no-op
	w.UnlockCoin(makeOutpoint(0xFF, 99))
}

func TestUnlockAllCoins(t *testing.T) {
	w := createIsolatedWallet(t)

	op1 := makeOutpoint(0x03, 0)
	op2 := makeOutpoint(0x04, 1)
	op3 := makeOutpoint(0x05, 2)

	w.LockCoin(op1)
	w.LockCoin(op2)
	w.LockCoin(op3)
	assert.True(t, w.IsLockedCoin(op1))
	assert.True(t, w.IsLockedCoin(op2))
	assert.True(t, w.IsLockedCoin(op3))

	w.UnlockAllCoins()
	assert.False(t, w.IsLockedCoin(op1))
	assert.False(t, w.IsLockedCoin(op2))
	assert.False(t, w.IsLockedCoin(op3))
	assert.Empty(t, w.ListLockedCoins())
}

func TestListLockedCoins(t *testing.T) {
	w := createIsolatedWallet(t)

	// Empty initially
	assert.Empty(t, w.ListLockedCoins())

	op1 := makeOutpoint(0x06, 0)
	op2 := makeOutpoint(0x07, 1)

	w.LockCoin(op1)
	w.LockCoin(op2)

	locked := w.ListLockedCoins()
	assert.Len(t, locked, 2)

	// Verify both are present (order not guaranteed)
	lockedSet := make(map[types.Outpoint]bool)
	for _, op := range locked {
		lockedSet[op] = true
	}
	assert.True(t, lockedSet[op1])
	assert.True(t, lockedSet[op2])
}

func TestListUnspent_UserLockedShownAsLocked(t *testing.T) {
	w := createIsolatedWallet(t)

	// Set chain height so confirmations calculate properly
	w.heightMu.Lock()
	w.cachedChainHeight = 200
	w.heightMu.Unlock()

	op := makeOutpoint(0x10, 0)
	w.mu.Lock()
	addTestUTXO(w, op, 500000000, 100)
	w.mu.Unlock()

	// Lock the coin
	w.LockCoin(op)

	// ListUnspent should include it with Locked: true, Spendable: false
	result, err := w.ListUnspent(1, 9999999, nil)
	require.NoError(t, err)

	outputs := result.([]*UnspentOutput)
	require.Len(t, outputs, 1)
	assert.True(t, outputs[0].Locked, "user-locked UTXO should have Locked=true")
	assert.False(t, outputs[0].Spendable, "user-locked UTXO should have Spendable=false")
}

func TestListUnspent_CollateralShownAsLocked(t *testing.T) {
	w := createIsolatedWallet(t)

	w.heightMu.Lock()
	w.cachedChainHeight = 200
	w.heightMu.Unlock()

	collateralOp := makeOutpoint(0x20, 0)

	// Set up mock collateral checker
	checker := &mockCollateralChecker{
		collaterals: map[types.Outpoint]bool{collateralOp: true},
	}
	w.SetMasternodeManager(checker)

	w.mu.Lock()
	addTestUTXO(w, collateralOp, 1000000000000, 100)
	w.mu.Unlock()

	// ListUnspent should show collateral as Locked: true, Spendable: false
	result, err := w.ListUnspent(1, 9999999, nil)
	require.NoError(t, err)

	outputs := result.([]*UnspentOutput)
	require.Len(t, outputs, 1)
	assert.True(t, outputs[0].Locked, "collateral UTXO should have Locked=true")
	assert.False(t, outputs[0].Spendable, "collateral UTXO should have Spendable=false")
}

func TestSelectUTXOs_SkipsUserLockedCoins(t *testing.T) {
	w := createIsolatedWallet(t)

	w.heightMu.Lock()
	w.cachedChainHeight = 200
	w.heightMu.Unlock()

	lockedOp := makeOutpoint(0x30, 0)
	freeOp := makeOutpoint(0x31, 1)

	w.mu.Lock()
	addTestUTXO(w, lockedOp, 500000000, 100) // 5 TWINS - will be locked
	addTestUTXO(w, freeOp, 300000000, 100)   // 3 TWINS - free
	w.mu.Unlock()

	// Lock one coin
	w.LockCoin(lockedOp)

	// SelectUTXOs should only pick the free coin
	selected, err := w.SelectUTXOs(100000000, 1, 2) // need 1 TWINS
	require.NoError(t, err)
	require.Len(t, selected, 1)
	assert.Equal(t, freeOp, selected[0].Outpoint, "should select the free UTXO, not the locked one")
}

func TestSelectUTXOs_SkipsCollateralCoins(t *testing.T) {
	w := createIsolatedWallet(t)

	w.heightMu.Lock()
	w.cachedChainHeight = 200
	w.heightMu.Unlock()

	collateralOp := makeOutpoint(0x40, 0)
	freeOp := makeOutpoint(0x41, 1)

	checker := &mockCollateralChecker{
		collaterals: map[types.Outpoint]bool{collateralOp: true},
	}
	w.SetMasternodeManager(checker)

	w.mu.Lock()
	addTestUTXO(w, collateralOp, 1000000000000, 100) // collateral
	addTestUTXO(w, freeOp, 300000000, 100)            // 3 TWINS - free
	w.mu.Unlock()

	selected, err := w.SelectUTXOs(100000000, 1, 2) // need 1 TWINS
	require.NoError(t, err)
	require.Len(t, selected, 1)
	assert.Equal(t, freeOp, selected[0].Outpoint, "should select the free UTXO, not the collateral")
}

func TestCoinLock_ConcurrentAccess(t *testing.T) {
	w := createIsolatedWallet(t)

	var wg sync.WaitGroup
	const goroutines = 10
	const opsPerGoroutine = 100

	// Concurrent lock/unlock/list operations should not race
	wg.Add(goroutines)
	for g := range goroutines {
		go func(id int) {
			defer wg.Done()
			for i := range opsPerGoroutine {
				op := makeOutpoint(byte(id), uint32(i))
				w.LockCoin(op)
				w.IsLockedCoin(op)
				w.ListLockedCoins()
				w.UnlockCoin(op)
			}
		}(g)
	}
	wg.Wait()

	// All should be unlocked after goroutines finish
	assert.Empty(t, w.ListLockedCoins())
}
