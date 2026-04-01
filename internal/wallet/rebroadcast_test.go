package wallet

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/twins-dev/twins-core/pkg/types"
)

func rebroadcastTestTx(seed byte) *types.Transaction {
	return &types.Transaction{
		Version: 1,
		Inputs: []*types.TxInput{
			{
				PreviousOutput: types.Outpoint{
					Hash:  types.NewHash([]byte{seed}),
					Index: uint32(seed),
				},
				ScriptSig: []byte{seed},
				Sequence:  0xffffffff,
			},
		},
		Outputs: []*types.TxOutput{
			{
				Value:        1000000,
				ScriptPubKey: []byte{0x51}, // OP_TRUE (placeholder test script)
			},
		},
	}
}

func TestCollectRebroadcastCandidates_FiltersAndBudget(t *testing.T) {
	w := &Wallet{
		logger:          logrus.NewEntry(logrus.New()),
		pendingTxs:      make(map[types.Hash]*WalletTransaction),
		lastRebroadcast: make(map[types.Hash]time.Time),
	}
	now := time.Now()

	sendOld := rebroadcastTestTx(1)
	sendFresh := rebroadcastTestTx(2)
	recvOld := rebroadcastTestTx(3)
	sendCooldown := rebroadcastTestTx(4)

	w.pendingMu.Lock()
	w.pendingTxs[sendOld.Hash()] = &WalletTransaction{Tx: sendOld, Hash: sendOld.Hash(), Category: TxCategorySend, Time: now.Add(-40 * time.Minute)}
	w.pendingTxs[sendFresh.Hash()] = &WalletTransaction{Tx: sendFresh, Hash: sendFresh.Hash(), Category: TxCategorySend, Time: now.Add(-2 * time.Minute)}
	w.pendingTxs[recvOld.Hash()] = &WalletTransaction{Tx: recvOld, Hash: recvOld.Hash(), Category: TxCategoryReceive, Time: now.Add(-40 * time.Minute)}
	w.pendingTxs[sendCooldown.Hash()] = &WalletTransaction{Tx: sendCooldown, Hash: sendCooldown.Hash(), Category: TxCategorySend, Time: now.Add(-40 * time.Minute)}
	w.lastRebroadcast[sendCooldown.Hash()] = now.Add(-5 * time.Minute)
	w.pendingMu.Unlock()

	candidates := w.collectRebroadcastCandidates(now)
	if assert.Len(t, candidates, 1) {
		assert.Equal(t, sendOld.Hash(), candidates[0].Hash())
	}
}

func TestCollectRebroadcastCandidates_OrdersOldestFirst(t *testing.T) {
	w := &Wallet{
		logger:          logrus.NewEntry(logrus.New()),
		pendingTxs:      make(map[types.Hash]*WalletTransaction),
		lastRebroadcast: make(map[types.Hash]time.Time),
	}
	now := time.Now()

	newer := rebroadcastTestTx(10)
	older := rebroadcastTestTx(11)

	w.pendingMu.Lock()
	w.pendingTxs[newer.Hash()] = &WalletTransaction{Tx: newer, Hash: newer.Hash(), Category: TxCategorySend, Time: now.Add(-20 * time.Minute)}
	w.pendingTxs[older.Hash()] = &WalletTransaction{Tx: older, Hash: older.Hash(), Category: TxCategorySend, Time: now.Add(-40 * time.Minute)}
	w.pendingMu.Unlock()

	candidates := w.collectRebroadcastCandidates(now)
	if assert.Len(t, candidates, 2) {
		assert.Equal(t, older.Hash(), candidates[0].Hash())
		assert.Equal(t, newer.Hash(), candidates[1].Hash())
	}
}
