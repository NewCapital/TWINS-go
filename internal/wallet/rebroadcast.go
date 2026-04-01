package wallet

import (
	"context"
	"math/rand"
	"sort"
	"time"

	"github.com/twins-dev/twins-core/pkg/types"
)

const (
	// Legacy-like random resend window.
	rebroadcastIntervalMin = 12 * time.Minute
	rebroadcastIntervalMax = 30 * time.Minute

	// Skip very fresh txs and cap each cycle to protect network/CPU.
	rebroadcastTxMinAge    = 5 * time.Minute
	rebroadcastCycleBudget = 200

	// Per-tx cooldown to prevent repeated re-announcement storms.
	rebroadcastTxCooldown = 30 * time.Minute
)

func (w *Wallet) rebroadcastLoop(ctx context.Context) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	var lastBestHeight uint32

	for {
		const rebroadcastDelta = rebroadcastIntervalMax - rebroadcastIntervalMin
		wait := rebroadcastIntervalMin + time.Duration(rng.Int63n(int64(rebroadcastDelta)))

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		height, ok := w.bestHeight()
		if !ok {
			continue
		}
		// Gate rebroadcast on chain progress to avoid idle spam during stalled sync.
		if lastBestHeight != 0 && height <= lastBestHeight {
			continue
		}
		lastBestHeight = height

		txs := w.collectRebroadcastCandidates(time.Now())
		if len(txs) == 0 {
			continue
		}

		w.mu.RLock()
		broadcaster := w.broadcaster
		w.mu.RUnlock()
		if broadcaster == nil {
			continue
		}

		now := time.Now()
		for _, tx := range txs {
			if tx == nil {
				continue
			}
			if err := broadcaster.BroadcastTransaction(tx); err != nil {
				w.logger.WithError(err).WithField("tx", tx.Hash().String()).
					Debug("Wallet rebroadcast failed")
				continue
			}
			w.markRebroadcasted(tx.Hash(), now)
		}
	}
}

func (w *Wallet) bestHeight() (uint32, bool) {
	w.mu.RLock()
	bc := w.blockchain
	w.mu.RUnlock()
	if bc == nil {
		return 0, false
	}
	height, err := bc.GetBestHeight()
	if err != nil {
		w.logger.WithError(err).Debug("Wallet rebroadcast: failed to read chain height")
		return 0, false
	}
	return height, true
}

func (w *Wallet) collectRebroadcastCandidates(now time.Time) []*types.Transaction {
	type candidate struct {
		hash   types.Hash
		txTime time.Time
		tx     *types.Transaction
	}

	candidates := make([]candidate, 0, 32)

	w.pendingMu.RLock()
	for hash, ptx := range w.pendingTxs {
		if ptx == nil || ptx.Tx == nil {
			continue
		}
		// Rebroadcast only locally-originated outgoing wallet transactions.
		if ptx.Category != TxCategorySend {
			continue
		}
		if now.Sub(ptx.Time) < rebroadcastTxMinAge {
			continue
		}
		if last, ok := w.lastRebroadcast[hash]; ok && now.Sub(last) < rebroadcastTxCooldown {
			continue
		}
		candidates = append(candidates, candidate{
			hash: hash,
			txTime: ptx.Time,
			tx:   ptx.Tx,
		})
	}
	w.pendingMu.RUnlock()

	// Clean stale entries from lastRebroadcast (tx confirmed without pending path).
	w.pendingMu.Lock()
	for hash, lastTime := range w.lastRebroadcast {
		if now.Sub(lastTime) > 2*rebroadcastTxCooldown {
			delete(w.lastRebroadcast, hash)
		}
	}
	w.pendingMu.Unlock()

	if len(candidates) == 0 {
		return nil
	}

	// Oldest first, deterministic.
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].txTime.Equal(candidates[j].txTime) {
			return candidates[i].hash.String() < candidates[j].hash.String()
		}
		return candidates[i].txTime.Before(candidates[j].txTime)
	})

	if len(candidates) > rebroadcastCycleBudget {
		candidates = candidates[:rebroadcastCycleBudget]
	}

	result := make([]*types.Transaction, 0, len(candidates))
	for _, c := range candidates {
		result = append(result, c.tx)
	}
	return result
}

func (w *Wallet) markRebroadcasted(hash types.Hash, now time.Time) {
	w.pendingMu.Lock()
	w.lastRebroadcast[hash] = now
	w.pendingMu.Unlock()
}
