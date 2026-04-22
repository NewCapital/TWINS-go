package mempool

import (
	"encoding/hex"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twins-dev/twins-core/internal/blockchain"
	"github.com/twins-dev/twins-core/internal/consensus"
	"github.com/twins-dev/twins-core/internal/storage"
	"github.com/twins-dev/twins-core/internal/storage/binary"
	"github.com/twins-dev/twins-core/pkg/crypto"
	"github.com/twins-dev/twins-core/pkg/types"
)

// Shared deterministic signing key for tests. sync.Once ensures a single key
// is used across the whole package so fixtures and regression tests share the
// same P2PKH scriptPubKey.
var (
	testKeyOnce     sync.Once
	testPrivKey     *crypto.PrivateKey
	testPubKeyBytes []byte
	testP2PKHScript []byte
)

// getTestSigningKey is nil-safe for t so it can be called from helpers without
// a testing.TB (e.g. from createTestTransaction).
func getTestSigningKey(t testing.TB) (*crypto.PrivateKey, []byte, []byte) {
	if t != nil {
		t.Helper()
	}
	testKeyOnce.Do(func() {
		raw, _ := hex.DecodeString("1111111111111111111111111111111111111111111111111111111111111111")
		pk, err := crypto.ParsePrivateKeyFromBytes(raw)
		if err != nil {
			panic(err)
		}
		testPrivKey = pk
		testPubKeyBytes = pk.PublicKey().SerializeCompressed()
		pkh := crypto.Hash160(testPubKeyBytes)
		testP2PKHScript = append([]byte{0x76, 0xa9, 0x14}, pkh...)
		testP2PKHScript = append(testP2PKHScript, 0x88, 0xac)
	})
	return testPrivKey, testPubKeyBytes, testP2PKHScript
}

// signTxInput signs input[i] of tx for a UTXO with the shared test P2PKH
// scriptPubKey and writes the assembled scriptSig back into tx.Inputs[i].
func signTxInput(t testing.TB, tx *types.Transaction, i int) {
	if t != nil {
		t.Helper()
	}
	priv, pubKeyBytes, scriptPubKey := getTestSigningKey(t)

	sigHash := tx.SignatureHash(i, scriptPubKey, types.SigHashAll)
	sig, err := priv.Sign(sigHash[:])
	if err != nil {
		panic(err)
	}
	sigWithHashType := append(sig.Bytes(), byte(types.SigHashAll))

	scriptSig := make([]byte, 0, 1+len(sigWithHashType)+1+len(pubKeyBytes))
	scriptSig = append(scriptSig, byte(len(sigWithHashType)))
	scriptSig = append(scriptSig, sigWithHashType...)
	scriptSig = append(scriptSig, byte(len(pubKeyBytes)))
	scriptSig = append(scriptSig, pubKeyBytes...)

	tx.Inputs[i].ScriptSig = scriptSig
}

type testMempoolEnv struct {
	mp    *TxMempool
	store storage.Storage
}

// testBlockchainWrapper wraps a real blockchain but returns a dummy transaction
// for any GetTransaction call, preventing orphan rejection in tests.
type testBlockchainWrapper struct {
	blockchain.Blockchain
}

func (w *testBlockchainWrapper) GetTransaction(hash types.Hash) (*types.Transaction, error) {
	// Return a dummy transaction so isOrphan() doesn't reject test transactions
	return &types.Transaction{
		Version: 1,
		Outputs: []*types.TxOutput{{Value: 2000000, ScriptPubKey: []byte("test")}},
	}, nil
}

func createTestMempool(t testing.TB) *TxMempool {
	env := createTestMempoolEnv(t)
	return env.mp
}

func createTestMempoolEnv(t testing.TB) *testMempoolEnv {
	// Create test storage
	storageConfig := storage.TestStorageConfig()
	store, err := binary.NewBinaryStorage(storageConfig)
	require.NoError(t, err)

	// Create test consensus
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)
	pos := consensus.NewProofOfStake(store, types.DefaultChainParams(), logger)

	// Create blockchain
	bcConfig := blockchain.DefaultConfig()
	bcConfig.Storage = store
	bcConfig.Consensus = pos
	bcConfig.ChainParams = types.DefaultChainParams()

	bc, err := blockchain.New(bcConfig)
	require.NoError(t, err)

	// Create mempool
	config := DefaultConfig()
	config.Blockchain = bc
	config.Consensus = pos
	config.UTXOSet = bc  // Blockchain implements UTXOGetter
	config.MaxTransactions = 100
	config.MaxSize = 1024 * 1024 // 1MB
	config.CleanupInterval = 100 * time.Millisecond
	config.ExpiryInterval = 100 * time.Millisecond

	mp, err := New(config)
	require.NoError(t, err)

	// Wrap blockchain so GetTransaction returns dummy tx (prevents orphan rejection in tests)
	mp.blockchain = &testBlockchainWrapper{Blockchain: bc}

	return &testMempoolEnv{mp: mp, store: store}
}

// createTestTransaction builds a 1-in 1-out transaction signed by the shared
// test key against a P2PKH funding UTXO. Must be paired with seedUTXOForTx
// (which stores the matching scriptPubKey) so mempool signature verification
// passes.
func createTestTransaction(nonce uint32) *types.Transaction {
	_, pubKeyBytes, _ := getTestSigningKey(nil)
	pkh := crypto.Hash160(pubKeyBytes)
	outScript := append([]byte{0x76, 0xa9, 0x14}, pkh...)
	outScript = append(outScript, 0x88, 0xac)

	tx := &types.Transaction{
		Version: 1,
		Inputs: []*types.TxInput{{
			PreviousOutput: types.Outpoint{
				Hash:  types.NewHash([]byte{byte(nonce)}),
				Index: nonce,
			},
			Sequence: 0xffffffff,
		}},
		Outputs: []*types.TxOutput{{
			Value:        1000000,
			ScriptPubKey: outScript,
		}},
		LockTime: 0,
	}
	signTxInput(nil, tx, 0)
	return tx
}

// seedUTXOForTx pre-seeds the UTXO referenced by a test transaction so
// mempool validation doesn't reject it as an orphan, using the shared test
// key's P2PKH scriptPubKey so signature verification succeeds.
func seedUTXOForTx(t testing.TB, store storage.Storage, tx *types.Transaction) {
	_, _, scriptPubKey := getTestSigningKey(t)
	var totalOutput int64
	for _, out := range tx.Outputs {
		totalOutput += out.Value
	}
	for _, in := range tx.Inputs {
		err := store.StoreUTXO(in.PreviousOutput, &types.TxOutput{
			Value:        totalOutput + 10000, // output + 0.0001 TWINS fee
			ScriptPubKey: scriptPubKey,
		}, 1, false)
		require.NoError(t, err)
	}
}

func TestNew(t *testing.T) {
	mp := createTestMempool(t)
	assert.NotNil(t, mp)
	assert.NotNil(t, mp.blockchain)
	assert.NotNil(t, mp.consensus)
	assert.NotNil(t, mp.logger)
}

func TestNewWithNilConfig(t *testing.T) {
	_, err := New(nil)
	assert.Error(t, err)
}

func TestNewWithoutBlockchain(t *testing.T) {
	config := DefaultConfig()
	config.Consensus = &struct{}{}

	_, err := New(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "blockchain")
}

func TestNewWithoutConsensus(t *testing.T) {
	config := DefaultConfig()
	config.Blockchain = &struct{}{}

	_, err := New(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "consensus")
}

func TestStartStop(t *testing.T) {
	mp := createTestMempool(t)

	err := mp.Start()
	assert.NoError(t, err)

	err = mp.Stop()
	assert.NoError(t, err)
}

func TestAddTransaction(t *testing.T) {
	env := createTestMempoolEnv(t)

	tx := createTestTransaction(1)
	seedUTXOForTx(t, env.store, tx)

	err := env.mp.AddTransaction(tx)
	assert.NoError(t, err)

	assert.Equal(t, 1, env.mp.Count())
	assert.True(t, env.mp.HasTransaction(tx.Hash()))
}

// TestAddTransaction_RejectsInvalidSignature pins the fix for the mempool
// signature-verification gap: a tx with properly-formed DER that fails actual
// secp256k1 verification (flipped last byte of S) must be rejected, not
// accepted and relayed (which would earn a ban from legacy peers at
// CScriptCheck — see legacy/src/main.cpp:2102-2107).
func TestAddTransaction_RejectsInvalidSignature(t *testing.T) {
	env := createTestMempoolEnv(t)

	tx := createTestTransaction(1)
	seedUTXOForTx(t, env.store, tx)

	// Corrupt the signature by flipping the last byte of the DER-encoded S
	// component. The result is still valid DER that parses fine but fails to
	// verify against (pubkey, sighash).
	//
	// scriptSig layout: ss[0] = push opcode (= len(sigWithHashType)),
	//                   ss[1..1+sigLen-1] = sigWithHashType,
	//                   sigWithHashType = DER || 0x01.
	// Index of sighash byte = sigLen. Last byte of S = sigLen-1.
	ss := tx.Inputs[0].ScriptSig
	require.Greater(t, len(ss), 2, "scriptSig too short")
	sigLen := int(ss[0])
	require.GreaterOrEqual(t, len(ss), 1+sigLen)
	ss[sigLen-1] ^= 0x01

	err := env.mp.AddTransaction(tx)
	require.Error(t, err)
	me, ok := err.(*MempoolError)
	require.True(t, ok, "expected MempoolError, got %T", err)
	assert.Equal(t, RejectInvalid, me.Code)
	assert.Contains(t, err.Error(), "script verification failed")
}

// TestAddTransaction_AcceptsValidSignature is the positive control: an
// unmodified, properly-signed test transaction is accepted. Guards against an
// over-eager rejection path in the new verification loop.
func TestAddTransaction_AcceptsValidSignature(t *testing.T) {
	env := createTestMempoolEnv(t)

	tx := createTestTransaction(42)
	seedUTXOForTx(t, env.store, tx)

	err := env.mp.AddTransaction(tx)
	assert.NoError(t, err)
	assert.True(t, env.mp.HasTransaction(tx.Hash()))
}

func TestAddTransaction_Nil(t *testing.T) {
	mp := createTestMempool(t)

	err := mp.AddTransaction(nil)
	assert.Error(t, err)

	mempoolErr, ok := err.(*MempoolError)
	assert.True(t, ok)
	assert.Equal(t, RejectInvalid, mempoolErr.Code)
}

func TestAddTransaction_Duplicate(t *testing.T) {
	env := createTestMempoolEnv(t)

	tx := createTestTransaction(1)
	seedUTXOForTx(t, env.store, tx)

	err := env.mp.AddTransaction(tx)
	assert.NoError(t, err)

	// Try to add again
	err = env.mp.AddTransaction(tx)
	assert.Error(t, err)

	mempoolErr, ok := err.(*MempoolError)
	assert.True(t, ok)
	assert.Equal(t, RejectDuplicate, mempoolErr.Code)
}

func TestRemoveTransaction(t *testing.T) {
	env := createTestMempoolEnv(t)

	tx := createTestTransaction(1)
	seedUTXOForTx(t, env.store, tx)

	err := env.mp.AddTransaction(tx)
	require.NoError(t, err)

	err = env.mp.RemoveTransaction(tx.Hash())
	assert.NoError(t, err)

	assert.Equal(t, 0, env.mp.Count())
	assert.False(t, env.mp.HasTransaction(tx.Hash()))
}

func TestRemoveTransaction_NotFound(t *testing.T) {
	mp := createTestMempool(t)

	err := mp.RemoveTransaction(types.NewHash([]byte("notfound")))
	assert.Error(t, err)
}

func TestRemoveTransactions(t *testing.T) {
	env := createTestMempoolEnv(t)

	// Add multiple transactions
	tx1 := createTestTransaction(1)
	tx2 := createTestTransaction(2)
	seedUTXOForTx(t, env.store, tx1)
	seedUTXOForTx(t, env.store, tx2)

	env.mp.AddTransaction(tx1)
	env.mp.AddTransaction(tx2)

	assert.Equal(t, 2, env.mp.Count())

	// Remove both
	hashes := []types.Hash{tx1.Hash(), tx2.Hash()}
	err := env.mp.RemoveTransactions(hashes)
	assert.NoError(t, err)

	assert.Equal(t, 0, env.mp.Count())
}

func TestHasTransaction(t *testing.T) {
	env := createTestMempoolEnv(t)

	tx := createTestTransaction(1)
	seedUTXOForTx(t, env.store, tx)

	assert.False(t, env.mp.HasTransaction(tx.Hash()))

	env.mp.AddTransaction(tx)

	assert.True(t, env.mp.HasTransaction(tx.Hash()))
}

func TestGetTransaction(t *testing.T) {
	env := createTestMempoolEnv(t)

	tx := createTestTransaction(1)
	seedUTXOForTx(t, env.store, tx)

	env.mp.AddTransaction(tx)

	retrieved, err := env.mp.GetTransaction(tx.Hash())
	assert.NoError(t, err)
	assert.Equal(t, tx.Hash(), retrieved.Hash())
}

func TestGetTransaction_NotFound(t *testing.T) {
	mp := createTestMempool(t)

	_, err := mp.GetTransaction(types.NewHash([]byte("notfound")))
	assert.Error(t, err)
}

func TestGetTransactions(t *testing.T) {
	env := createTestMempoolEnv(t)

	// Add multiple transactions
	for i := uint32(0); i < 5; i++ {
		tx := createTestTransaction(i)
		seedUTXOForTx(t, env.store, tx)
		env.mp.AddTransaction(tx)
	}

	txs := env.mp.GetTransactions(3)
	assert.Len(t, txs, 3)

	txs = env.mp.GetTransactions(10)
	assert.Len(t, txs, 5)
}

func TestGetTransactionsForBlock(t *testing.T) {
	env := createTestMempoolEnv(t)

	// Add multiple transactions
	for i := uint32(0); i < 5; i++ {
		tx := createTestTransaction(i)
		seedUTXOForTx(t, env.store, tx)
		env.mp.AddTransaction(tx)
	}

	txs := env.mp.GetTransactionsForBlock(100000, 3)
	assert.LessOrEqual(t, len(txs), 3)
}

func TestGetHighPriorityTransactions(t *testing.T) {
	env := createTestMempoolEnv(t)

	// Add multiple transactions
	for i := uint32(0); i < 5; i++ {
		tx := createTestTransaction(i)
		seedUTXOForTx(t, env.store, tx)
		env.mp.AddTransaction(tx)
	}

	txs := env.mp.GetHighPriorityTransactions(3)
	assert.LessOrEqual(t, len(txs), 3)
}

func TestGetTransactionsByFeeRate(t *testing.T) {
	env := createTestMempoolEnv(t)

	// Add multiple transactions
	for i := uint32(0); i < 5; i++ {
		tx := createTestTransaction(i)
		seedUTXOForTx(t, env.store, tx)
		env.mp.AddTransaction(tx)
	}

	txs := env.mp.GetTransactionsByFeeRate(1, 10)
	assert.NotNil(t, txs)
}

func TestCount(t *testing.T) {
	env := createTestMempoolEnv(t)

	assert.Equal(t, 0, env.mp.Count())

	tx1 := createTestTransaction(1)
	seedUTXOForTx(t, env.store, tx1)
	env.mp.AddTransaction(tx1)

	assert.Equal(t, 1, env.mp.Count())

	tx2 := createTestTransaction(2)
	seedUTXOForTx(t, env.store, tx2)
	env.mp.AddTransaction(tx2)

	assert.Equal(t, 2, env.mp.Count())
}

func TestSize(t *testing.T) {
	env := createTestMempoolEnv(t)

	assert.Equal(t, uint64(0), env.mp.Size())

	tx := createTestTransaction(1)
	seedUTXOForTx(t, env.store, tx)
	env.mp.AddTransaction(tx)

	assert.Greater(t, env.mp.Size(), uint64(0))
}

func TestGetStats(t *testing.T) {
	env := createTestMempoolEnv(t)

	stats := env.mp.GetStats()
	assert.NotNil(t, stats)
	assert.Equal(t, 0, stats.TransactionCount)

	tx := createTestTransaction(1)
	seedUTXOForTx(t, env.store, tx)
	env.mp.AddTransaction(tx)

	stats = env.mp.GetStats()
	assert.Equal(t, 1, stats.TransactionCount)
	assert.Greater(t, stats.TotalSize, uint64(0))
	assert.Greater(t, stats.TotalFees, int64(0))
}

func TestClear(t *testing.T) {
	env := createTestMempoolEnv(t)

	// Add transactions
	for i := uint32(0); i < 5; i++ {
		tx := createTestTransaction(i)
		seedUTXOForTx(t, env.store, tx)
		env.mp.AddTransaction(tx)
	}

	assert.Equal(t, 5, env.mp.Count())

	err := env.mp.Clear()
	assert.NoError(t, err)

	assert.Equal(t, 0, env.mp.Count())
}

func TestOrphanTransactions(t *testing.T) {
	mp := createTestMempool(t)

	// Create an orphan transaction (references non-existent parent)
	orphanTx := createTestTransaction(100)

	// Add to orphan pool manually for testing
	err := mp.addOrphan(orphanTx)
	assert.NoError(t, err)

	assert.Equal(t, 1, mp.GetOrphanCount())
	assert.True(t, mp.HasOrphan(orphanTx.Hash()))

	// Get orphans
	orphans := mp.GetOrphanTransactions()
	assert.Len(t, orphans, 1)
}

func TestRemoveOrphanTransaction(t *testing.T) {
	mp := createTestMempool(t)

	orphanTx := createTestTransaction(100)
	mp.addOrphan(orphanTx)

	err := mp.RemoveOrphanTransaction(orphanTx.Hash())
	assert.NoError(t, err)

	assert.Equal(t, 0, mp.GetOrphanCount())
}

func TestRateLimiting(t *testing.T) {
	env := createTestMempoolEnv(t)

	peerID := "peer1"
	tx := createTestTransaction(1)
	seedUTXOForTx(t, env.store, tx)

	err := env.mp.AddTransactionFromPeer(tx, peerID)
	assert.NoError(t, err)

	// Check peer stats
	stats, err := env.mp.GetPeerStats(peerID)
	assert.NoError(t, err)
	assert.Equal(t, 1, stats.TxCount)
}

func TestPeerBanning(t *testing.T) {
	mp := createTestMempool(t)
	mp.config.MaxRejectionsRate = 2

	peerID := "bad_peer"

	// Track multiple rejections
	for i := 0; i < 3; i++ {
		mp.trackPeerRejection(peerID, types.ZeroHash)
	}

	// Peer should be banned
	assert.True(t, mp.isPeerBanned(peerID))

	banned := mp.GetBannedPeers()
	assert.Contains(t, banned, peerID)
}

func TestUnbanPeer(t *testing.T) {
	mp := createTestMempool(t)

	peerID := "peer1"

	// Ban the peer
	mp.peerStatsMu.Lock()
	mp.peerStats[peerID] = &PeerStats{
		Banned:    true,
		BanExpiry: time.Now().Add(1 * time.Hour),
	}
	mp.peerStatsMu.Unlock()

	assert.True(t, mp.isPeerBanned(peerID))

	// Unban
	err := mp.UnbanPeer(peerID)
	assert.NoError(t, err)

	assert.False(t, mp.isPeerBanned(peerID))
}

func TestRejectCodeString(t *testing.T) {
	assert.Equal(t, "invalid", RejectInvalid.String())
	assert.Equal(t, "duplicate", RejectDuplicate.String())
	assert.Equal(t, "insufficient-fee", RejectInsufficientFee.String())
	assert.Equal(t, "pool-full", RejectPoolFull.String())
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()
	assert.NotNil(t, config)
	assert.Equal(t, 50000, config.MaxTransactions)
	assert.Equal(t, 1000, config.MaxOrphans)
	assert.Equal(t, 4, config.ValidationWorkers)
	assert.Greater(t, config.MaxSize, uint64(0))
}

// Benchmark tests
func BenchmarkAddTransaction(b *testing.B) {
	mp := createTestMempool(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx := createTestTransaction(uint32(i))
		mp.AddTransaction(tx)
	}
}

func BenchmarkGetTransactionsForBlock(b *testing.B) {
	mp := createTestMempool(b)

	// Pre-populate mempool
	for i := uint32(0); i < 1000; i++ {
		tx := createTestTransaction(i)
		mp.AddTransaction(tx)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mp.GetTransactionsForBlock(1000000, 100)
	}
}

func BenchmarkHasTransaction(b *testing.B) {
	mp := createTestMempool(b)

	tx := createTestTransaction(1)
	mp.AddTransaction(tx)
	hash := tx.Hash()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mp.HasTransaction(hash)
	}
}