package masternode

import (
	"bytes"
	"encoding/hex"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/twins-dev/twins-core/internal/storage"
	"github.com/twins-dev/twins-core/pkg/types"
)

// DefaultPaymentScanDepth is the number of blocks to scan backwards on startup.
// 50,000 blocks ≈ 35 days at 1 block/min. Covers several payment cycles.
const DefaultPaymentScanDepth uint32 = 50000

// PaymentStats holds aggregate payment statistics for a single masternode address.
type PaymentStats struct {
	FirstPaid    time.Time // Timestamp of first observed payment
	LastPaid     time.Time // Timestamp of most recent payment
	PaymentCount int64     // Total number of payments
	TotalPaid    int64     // Sum of all payments in satoshis
	LowestBlock  uint32    // Lowest block height with a payment
	HighestBlock uint32    // Highest block height with a payment
	LatestTxID   string    // Transaction ID of the most recent payment (coinstake txid)
}

// PaymentTracker maintains in-memory aggregate payment statistics per masternode
// payment address. Built from blockchain scan on startup and updated incrementally
// as new blocks arrive. Not persisted — rebuilt from blockchain on each daemon start.
type PaymentTracker struct {
	mu    sync.RWMutex
	stats map[string]*PaymentStats // key: hex-encoded scriptPubKey
}

// NewPaymentTracker creates a new empty PaymentTracker.
func NewPaymentTracker() *PaymentTracker {
	return &PaymentTracker{
		stats: make(map[string]*PaymentStats),
	}
}

// RecordPayment records a masternode payment. Thread-safe.
// scriptPubKey is the raw payment script from the block output.
// blockHeight is the block height. blockTime is the block's timestamp. amount is in satoshis.
// txID is the coinstake transaction hash (empty string if unavailable).
func (pt *PaymentTracker) RecordPayment(scriptPubKey []byte, blockHeight uint32, blockTime time.Time, amount int64, txID string) {
	if len(scriptPubKey) == 0 {
		return
	}

	key := hex.EncodeToString(scriptPubKey)

	pt.mu.Lock()
	defer pt.mu.Unlock()

	stat := pt.stats[key]
	if stat == nil {
		stat = &PaymentStats{
			FirstPaid:    blockTime,
			LowestBlock:  blockHeight,
			HighestBlock: blockHeight,
			LatestTxID:   txID,
		}
		pt.stats[key] = stat
	}
	if blockTime.After(stat.LastPaid) {
		stat.LastPaid = blockTime
	}
	if blockTime.Before(stat.FirstPaid) {
		stat.FirstPaid = blockTime
	}
	if blockHeight < stat.LowestBlock {
		stat.LowestBlock = blockHeight
	}
	if blockHeight > stat.HighestBlock {
		stat.HighestBlock = blockHeight
		stat.LatestTxID = txID
	}
	stat.PaymentCount++
	stat.TotalPaid += amount
}

// GetStatsByScript returns a copy of the payment stats for the given scriptPubKey.
// Returns nil if no payments have been recorded for this address.
func (pt *PaymentTracker) GetStatsByScript(scriptPubKey []byte) *PaymentStats {
	if len(scriptPubKey) == 0 {
		return nil
	}

	key := hex.EncodeToString(scriptPubKey)

	pt.mu.RLock()
	defer pt.mu.RUnlock()

	stat := pt.stats[key]
	if stat == nil {
		return nil
	}
	// Return a copy to avoid data races
	copy := *stat
	return &copy
}

// GetAllStats returns a copy of all payment statistics.
func (pt *PaymentTracker) GetAllStats() map[string]*PaymentStats {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	result := make(map[string]*PaymentStats, len(pt.stats))
	for k, v := range pt.stats {
		copy := *v
		result[k] = &copy
	}
	return result
}

// ScanBlockchain scans stored blocks backwards from chainTip to build initial
// payment statistics. Only processes PoS blocks (height > lastPOWBlock).
// devAddress is the dev fund scriptPubKey to exclude from masternode payment stats.
// scanDepth limits how many blocks to scan (0 = use DefaultPaymentScanDepth).
func (pt *PaymentTracker) ScanBlockchain(
	stor storage.Storage,
	chainTip uint32,
	lastPOWBlock uint32,
	devAddress []byte,
	scanDepth uint32,
) error {
	if scanDepth == 0 {
		scanDepth = DefaultPaymentScanDepth
	}

	// Calculate scan range
	startHeight := chainTip
	var endHeight uint32
	if chainTip > scanDepth {
		endHeight = chainTip - scanDepth
	}
	// Don't scan below the last PoW block — no MN payments there
	if endHeight <= lastPOWBlock {
		endHeight = lastPOWBlock + 1
	}

	scanStart := time.Now()
	scanned := 0
	recorded := 0

	for height := startHeight; height >= endHeight; height-- {
		block, err := stor.GetBlockByHeight(height)
		if err != nil {
			// Skip blocks we can't read (gaps in storage)
			continue
		}
		scanned++

		scriptPubKey, amount := extractMasternodePaymentAtHeight(block, height, lastPOWBlock, devAddress)
		if scriptPubKey == nil {
			continue
		}

		// Skip dev address payments
		if len(devAddress) > 0 && bytes.Equal(scriptPubKey, devAddress) {
			continue
		}

		// Get coinstake txid for the latest payment tracking
		var txID string
		if len(block.Transactions) >= 2 {
			txID = block.Transactions[1].Hash().String()
		}

		blockTime := time.Unix(int64(block.Header.Timestamp), 0)
		pt.RecordPayment(scriptPubKey, height, blockTime, amount, txID)
		recorded++

		// Prevent underflow when height is 0
		if height == 0 {
			break
		}
	}

	logrus.WithFields(logrus.Fields{
		"scanned":  scanned,
		"recorded": recorded,
		"from":     startHeight,
		"to":       endHeight,
		"unique":   pt.Count(),
		"duration": time.Since(scanStart).Round(time.Millisecond).String(),
	}).Info("Payment tracker: blockchain scan complete")

	return nil
}

// Count returns the number of unique payment addresses tracked.
func (pt *PaymentTracker) Count() int {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return len(pt.stats)
}

// extractMasternodePayment extracts the masternode payment output from a block.
// Uses block.Height() for the PoW check. For blocks loaded from storage where
// Height() may be zero, use extractMasternodePaymentAtHeight instead.
func extractMasternodePayment(block *types.Block, lastPOWBlock uint32, devAddress []byte) ([]byte, int64) {
	if block == nil {
		return nil, 0
	}
	return extractMasternodePaymentAtHeight(block, block.Height(), lastPOWBlock, devAddress)
}

// extractMasternodePaymentAtHeight extracts the masternode payment output from a block
// using an explicit height. Returns the scriptPubKey and amount, or (nil, 0) if no payment found.
// Matches legacy C++ main.cpp:4722: vtx[1].vout[vout.size() - 2]
func extractMasternodePaymentAtHeight(block *types.Block, height uint32, lastPOWBlock uint32, devAddress []byte) ([]byte, int64) {
	if block == nil {
		return nil, 0
	}

	// Only PoS blocks have masternode payments
	if height <= lastPOWBlock {
		return nil, 0
	}

	// PoS blocks use coinstake at index 1
	if len(block.Transactions) < 2 {
		return nil, 0
	}

	coinstake := block.Transactions[1]
	if !coinstake.IsCoinStake() {
		return nil, 0
	}

	// Need at least 3 outputs: [empty, stake, mn_payment]
	if len(coinstake.Outputs) < 3 {
		return nil, 0
	}

	// Determine MN payment output index.
	// Layout WITH dev: [empty(0), stake..., mn_payment, dev_payment] → MN at len-2
	// Layout WITHOUT dev: [empty(0), stake..., mn_payment] → MN at len-1
	// Matches legacy C++: vtx[1].vout[vout.size() - 2] (always has dev output)
	var mnIdx int
	if len(devAddress) > 0 {
		lastOutput := coinstake.Outputs[len(coinstake.Outputs)-1]
		if bytes.Equal(lastOutput.ScriptPubKey, devAddress) {
			// Dev output is last — MN payment is second-to-last
			mnIdx = len(coinstake.Outputs) - 2
		} else {
			// Last output is not dev — MN payment is last
			mnIdx = len(coinstake.Outputs) - 1
		}
	} else {
		// No dev address configured — MN payment is last output
		mnIdx = len(coinstake.Outputs) - 1
	}

	output := coinstake.Outputs[mnIdx]
	if len(output.ScriptPubKey) == 0 || output.Value <= 0 {
		return nil, 0
	}

	return output.ScriptPubKey, output.Value
}

