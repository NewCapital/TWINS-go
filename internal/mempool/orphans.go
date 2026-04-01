package mempool

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/twins-dev/twins-core/pkg/types"
)

// addOrphan adds a transaction to the orphan pool
func (mp *TxMempool) addOrphan(tx *types.Transaction) error {
	mp.orphMu.Lock()
	defer mp.orphMu.Unlock()

	txHash := tx.Hash()
	if _, exists := mp.orphans[txHash]; exists {
		return nil
	}

	// Check orphan limit
	if len(mp.orphans) >= mp.config.MaxOrphans {
		// Remove oldest orphan
		mp.evictOldestOrphan()
	}

	// Create entry
	entry, err := mp.createTxEntry(tx, txHash)
	if err != nil {
		return err
	}

	mp.orphans[txHash] = entry

	mp.statsMu.Lock()
	mp.stats.OrphanCount = len(mp.orphans)
	mp.statsMu.Unlock()

	mp.logger.WithField("tx", txHash.String()).Debug("Transaction added to orphan pool")

	return nil
}

// processOrphans tries to add orphans that may now be valid
func (mp *TxMempool) processOrphans(parentHash types.Hash) {
	queue := []types.Hash{parentHash}

	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]

		// Snapshot current candidates while holding read lock.
		mp.orphMu.RLock()
		candidates := make([]*types.Transaction, 0)
		for _, entry := range mp.orphans {
			for _, input := range entry.Tx.Inputs {
				if input.PreviousOutput.Hash == parent {
					candidates = append(candidates, entry.Tx)
					break
				}
			}
		}
		mp.orphMu.RUnlock()

		for _, orphanTx := range candidates {
			orphanHash := orphanTx.Hash()
			err := mp.addTransaction(orphanTx, false)
			if err == nil {
				mp.orphMu.Lock()
				delete(mp.orphans, orphanHash)
				orphanCount := len(mp.orphans)
				mp.orphMu.Unlock()

				mp.statsMu.Lock()
				mp.stats.OrphanCount = orphanCount
				mp.statsMu.Unlock()

				mp.logger.WithField("tx", orphanHash.String()).Debug("Orphan transaction processed")

				mp.onTxMu.RLock()
				orphanCb := mp.onOrphanAccepted
				mp.onTxMu.RUnlock()
				if orphanCb != nil {
					orphanCb(orphanTx)
				}

				queue = append(queue, orphanHash)
				continue
			}

			// If orphan became invalid (not just still-missing inputs), drop it.
			if mErr, ok := err.(*MempoolError); ok && mErr.Code != RejectOrphan {
				mp.orphMu.Lock()
				delete(mp.orphans, orphanHash)
				orphanCount := len(mp.orphans)
				mp.orphMu.Unlock()

				mp.statsMu.Lock()
				mp.stats.OrphanCount = orphanCount
				mp.statsMu.Unlock()

				mp.logger.WithFields(logrus.Fields{
					"tx":    orphanHash.String(),
					"error": err.Error(),
				}).Debug("Removed invalid orphan transaction")
			}
		}
	}
}

// evictOldestOrphan removes the oldest orphan transaction
func (mp *TxMempool) evictOldestOrphan() {
	var oldestHash types.Hash
	var oldestTime time.Time

	for hash, entry := range mp.orphans {
		if oldestTime.IsZero() || entry.Time.Before(oldestTime) {
			oldestHash = hash
			oldestTime = entry.Time
		}
	}

	if !oldestHash.IsZero() {
		delete(mp.orphans, oldestHash)
		mp.logger.WithField("tx", oldestHash.String()).Debug("Evicted oldest orphan")
	}
}

// cleanOrphans removes expired orphan transactions
func (mp *TxMempool) cleanOrphans() {
	mp.orphMu.Lock()
	defer mp.orphMu.Unlock()

	now := time.Now()
	expired := make([]types.Hash, 0)

	for hash, entry := range mp.orphans {
		if now.Sub(entry.Time) > mp.config.MaxTransactionAge {
			expired = append(expired, hash)
		}
	}

	for _, hash := range expired {
		delete(mp.orphans, hash)
	}

	if len(expired) > 0 {
		mp.statsMu.Lock()
		mp.stats.OrphanCount = len(mp.orphans)
		mp.statsMu.Unlock()

		mp.logger.WithField("count", len(expired)).Debug("Removed expired orphan transactions")
	}
}

// isOrphan checks if a transaction is an orphan (has missing inputs)
func (mp *TxMempool) isOrphan(tx *types.Transaction) (bool, error) {
	// Check each input
	for _, input := range tx.Inputs {
		// Skip coinbase inputs
		if input.PreviousOutput.Hash.IsZero() {
			continue
		}

		// Check if input exists in mempool
		if mp.HasTransaction(input.PreviousOutput.Hash) {
			continue
		}

		// Check if input exists in blockchain
		_, err := mp.blockchain.GetTransaction(input.PreviousOutput.Hash)
		if err != nil {
			// Input not found - this is an orphan
			return true, nil
		}
	}

	return false, nil
}

// getOrphanDependencies returns all orphans that depend on a transaction
func (mp *TxMempool) getOrphanDependencies(txHash types.Hash) []*TxEntry {
	mp.orphMu.RLock()
	defer mp.orphMu.RUnlock()

	dependencies := make([]*TxEntry, 0)

	for _, entry := range mp.orphans {
		for _, input := range entry.Tx.Inputs {
			if input.PreviousOutput.Hash == txHash {
				dependencies = append(dependencies, entry)
				break
			}
		}
	}

	return dependencies
}

// RemoveOrphansByParent removes all orphans that depend on a specific transaction
func (mp *TxMempool) RemoveOrphansByParent(parentHash types.Hash) error {
	mp.orphMu.Lock()
	defer mp.orphMu.Unlock()

	removed := make([]types.Hash, 0)

	for hash, entry := range mp.orphans {
		for _, input := range entry.Tx.Inputs {
			if input.PreviousOutput.Hash == parentHash {
				removed = append(removed, hash)
				break
			}
		}
	}

	for _, hash := range removed {
		delete(mp.orphans, hash)
	}

	if len(removed) > 0 {
		mp.statsMu.Lock()
		mp.stats.OrphanCount = len(mp.orphans)
		mp.statsMu.Unlock()

		mp.logger.WithFields(logrus.Fields{
			"parent": parentHash.String(),
			"count":  len(removed),
		}).Debug("Removed orphans by parent")
	}

	return nil
}

// GetOrphanCount returns the number of orphan transactions
func (mp *TxMempool) GetOrphanCount() int {
	mp.orphMu.RLock()
	defer mp.orphMu.RUnlock()

	return len(mp.orphans)
}

// HasOrphan checks if a transaction is in the orphan pool
func (mp *TxMempool) HasOrphan(hash types.Hash) bool {
	mp.orphMu.RLock()
	defer mp.orphMu.RUnlock()

	_, exists := mp.orphans[hash]
	return exists
}

// GetOrphan retrieves an orphan transaction
func (mp *TxMempool) GetOrphan(hash types.Hash) (*types.Transaction, error) {
	mp.orphMu.RLock()
	defer mp.orphMu.RUnlock()

	entry, exists := mp.orphans[hash]
	if !exists {
		return nil, fmt.Errorf("orphan transaction not found")
	}

	return entry.Tx, nil
}

// GetOrphansByInput returns orphans that spend a specific output
func (mp *TxMempool) GetOrphansByInput(outpoint types.Outpoint) []*types.Transaction {
	mp.orphMu.RLock()
	defer mp.orphMu.RUnlock()

	txs := make([]*types.Transaction, 0)

	for _, entry := range mp.orphans {
		for _, input := range entry.Tx.Inputs {
			if input.PreviousOutput == outpoint {
				txs = append(txs, entry.Tx)
				break
			}
		}
	}

	return txs
}
