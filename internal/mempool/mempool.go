package mempool

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/twins-dev/twins-core/internal/blockchain"
	"github.com/twins-dev/twins-core/internal/consensus"
	"github.com/twins-dev/twins-core/pkg/script"
	"github.com/twins-dev/twins-core/pkg/types"
)

const (
	// MaxStandardTxSize is the maximum size of a standard transaction in bytes.
	// Legacy: MAX_STANDARD_TX_SIZE = 100000 (main.h:70)
	// Defined independently from wallet package to avoid circular import.
	MaxStandardTxSize = 100_000
)

// UTXOGetter interface for UTXO queries
type UTXOGetter interface {
	GetUTXO(outpoint types.Outpoint) (*types.UTXO, error)
}

// TxMempool implements the Mempool interface.
// Lock ordering: txsMu → indexMu → statsMu → onTxMu
type TxMempool struct {
	config      *Config
	blockchain  blockchain.Blockchain
	consensus   consensus.Engine
	chainParams *types.ChainParams
	utxoSet     UTXOGetter
	logger      *logrus.Entry

	// Transaction storage
	txs     map[types.Hash]*TxEntry
	txsMu   sync.RWMutex
	orphans map[types.Hash]*TxEntry
	orphMu  sync.RWMutex

	// Spent-outpoint index: maps each outpoint spent by a mempool tx to the spending tx hash.
	// Replaces O(n*m) brute-force conflict detection with O(1) lookup.
	// Protected by txsMu (same lock as txs map).
	spentOutpoints map[types.Outpoint]types.Hash

	// Indexes for efficient lookups
	byFeeRate []*TxEntry
	indexMu   sync.RWMutex

	// Statistics
	stats   Stats
	statsMu sync.RWMutex

	// Rate limiting
	peerStats   map[string]*PeerStats
	peerStatsMu sync.RWMutex

	// Transaction notification callbacks (invoked outside txsMu for wallet integration)
	onTransaction       func(*types.Transaction) // Called when tx is accepted
	onRemoveTransaction func(types.Hash)         // Called when tx is removed/evicted
	onOrphanAccepted    func(*types.Transaction) // Called when orphan tx is accepted after parent arrival
	onTxMu              sync.RWMutex

	// Lifecycle
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// PeerStats tracks per-peer statistics for rate limiting
type PeerStats struct {
	TxCount      int
	LastTxTime   time.Time
	Rejections   int
	LastRejected time.Time
	Banned       bool
	BanExpiry    time.Time
}

// New creates a new transaction mempool
func New(config *Config) (*TxMempool, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if config.Blockchain == nil {
		return nil, fmt.Errorf("blockchain is required")
	}

	if config.Consensus == nil {
		return nil, fmt.Errorf("consensus is required")
	}

	mp := &TxMempool{
		config:         config,
		blockchain:     config.Blockchain.(blockchain.Blockchain),
		consensus:      config.Consensus.(consensus.Engine),
		chainParams:    config.ChainParams,
		utxoSet:        config.UTXOSet.(UTXOGetter),
		logger:         logrus.WithField("component", "mempool"),
		txs:            make(map[types.Hash]*TxEntry),
		orphans:        make(map[types.Hash]*TxEntry),
		spentOutpoints: make(map[types.Outpoint]types.Hash),
		byFeeRate:      make([]*TxEntry, 0),
		peerStats:      make(map[string]*PeerStats),
		stopChan:       make(chan struct{}),
	}

	mp.logger.Info("Mempool initialized")

	return mp, nil
}

// Start starts the mempool background tasks
func (mp *TxMempool) Start() error {
	mp.logger.Info("Starting mempool")

	// Start cleanup routine
	mp.wg.Add(1)
	go mp.cleanupRoutine()

	// Start expiry routine
	mp.wg.Add(1)
	go mp.expiryRoutine()

	return nil
}

// Stop stops the mempool
func (mp *TxMempool) Stop() error {
	mp.logger.Info("Stopping mempool")

	close(mp.stopChan)
	mp.wg.Wait()

	return nil
}

// Clear removes all transactions from the mempool
func (mp *TxMempool) Clear() error {
	// Collect tx hashes before clearing for removal callbacks
	mp.txsMu.Lock()
	cleared := make([]types.Hash, 0, len(mp.txs))
	for hash := range mp.txs {
		cleared = append(cleared, hash)
	}

	mp.txs = make(map[types.Hash]*TxEntry)
	mp.spentOutpoints = make(map[types.Outpoint]types.Hash)

	mp.indexMu.Lock()
	mp.byFeeRate = make([]*TxEntry, 0)
	mp.indexMu.Unlock()

	mp.txsMu.Unlock()

	// Reset stats to match empty mempool
	mp.statsMu.Lock()
	mp.stats.TransactionCount = 0
	mp.stats.TotalSize = 0
	mp.stats.TotalFees = 0
	mp.statsMu.Unlock()

	mp.orphMu.Lock()
	mp.orphans = make(map[types.Hash]*TxEntry)
	mp.orphMu.Unlock()

	// Invoke removal callbacks outside all locks
	if len(cleared) > 0 {
		mp.onTxMu.RLock()
		removeCb := mp.onRemoveTransaction
		mp.onTxMu.RUnlock()
		if removeCb != nil {
			for _, hash := range cleared {
				removeCb(hash)
			}
		}
	}

	mp.logger.Debug("Mempool cleared")

	return nil
}

// AddTransaction adds a transaction to the mempool
func (mp *TxMempool) AddTransaction(tx *types.Transaction) error {
	return mp.addTransaction(tx, true)
}

func (mp *TxMempool) addTransaction(tx *types.Transaction, processDependents bool) error {
	if tx == nil {
		return NewMempoolError(RejectInvalid, "transaction is nil", types.ZeroHash)
	}

	txHash := tx.Hash()
	start := time.Now()

	mp.logger.WithField("tx", txHash.String()).Debug("Adding transaction to mempool")

	// Check if already in mempool
	if mp.HasTransaction(txHash) {
		return NewMempoolError(RejectDuplicate, "transaction already in mempool", txHash)
	}

	// Handle missing-input transactions as orphans (legacy-compatible behavior).
	isOrphan, err := mp.isOrphan(tx)
	if err != nil {
		return NewMempoolError(RejectInvalid, fmt.Sprintf("failed orphan check: %v", err), txHash)
	}
	if isOrphan {
		if err := mp.addOrphan(tx); err != nil {
			return NewMempoolError(RejectOrphan, fmt.Sprintf("failed to store orphan: %v", err), txHash)
		}
		return NewMempoolError(RejectOrphan, "missing inputs (stored as orphan)", txHash)
	}

	// Check for conflicts FIRST. This is an O(1) map lookup and must happen
	// before the expensive per-input script verification in validateTransaction,
	// otherwise a peer can flood conflicting variants of an already-pooled spend
	// and force repeated ECDSA work for transactions guaranteed to be rejected.
	if err := mp.checkConflicts(tx, txHash); err != nil {
		return err
	}

	// Validate transaction (includes per-input ECDSA signature verification).
	if err := mp.validateTransaction(tx); err != nil {
		mp.incrementRejections()
		return err
	}

	// Calculate fee and priority
	entry, err := mp.createTxEntry(tx, txHash)
	if err != nil {
		return NewMempoolError(RejectInvalid, fmt.Sprintf("failed to create entry: %v", err), txHash)
	}

	// Check fee requirements
	if entry.FeeRate < mp.config.MinRelayFee {
		return NewMempoolError(RejectInsufficientFee,
			fmt.Sprintf("fee rate %d below minimum %d", entry.FeeRate, mp.config.MinRelayFee),
			txHash)
	}

	// Check if mempool is full
	if err := mp.checkCapacity(entry); err != nil {
		return err
	}

	// Add to mempool and update spent-outpoint index.
	// Re-check for conflicts under write lock to prevent TOCTOU race
	// (another goroutine may have inserted a conflicting tx since checkConflicts).
	mp.txsMu.Lock()
	for _, input := range tx.Inputs {
		if _, exists := mp.spentOutpoints[input.PreviousOutput]; exists {
			mp.txsMu.Unlock()
			return NewMempoolError(RejectConflict,
				"transaction spends output already spent by mempool transaction", txHash)
		}
	}
	mp.txs[txHash] = entry
	for _, input := range tx.Inputs {
		mp.spentOutpoints[input.PreviousOutput] = txHash
	}
	mp.txsMu.Unlock()

	// Copy callback outside lock (Lock-Copy-Invoke pattern)
	mp.onTxMu.RLock()
	cb := mp.onTransaction
	mp.onTxMu.RUnlock()
	if cb != nil {
		cb(tx)
	}

	// Update indexes
	mp.updateIndexes(entry)

	// Update statistics
	mp.updateAddStats(entry, time.Since(start))

	mp.logger.WithFields(logrus.Fields{
		"tx":       txHash.String(),
		"size":     entry.Size,
		"fee":      entry.Fee,
		"fee_rate": entry.FeeRate,
	}).Debug("Transaction added to mempool")

	// Try to process orphan transactions that depended on this newly accepted tx.
	if processDependents {
		mp.processOrphans(txHash)
	}

	return nil
}

// RemoveTransaction removes a transaction from the mempool
func (mp *TxMempool) RemoveTransaction(hash types.Hash) error {
	mp.txsMu.Lock()

	entry, exists := mp.txs[hash]
	if !exists {
		mp.txsMu.Unlock()
		return fmt.Errorf("transaction not found")
	}

	// Clean spent-outpoint index
	for _, input := range entry.Tx.Inputs {
		delete(mp.spentOutpoints, input.PreviousOutput)
	}

	delete(mp.txs, hash)

	// Remove from indexes
	mp.removeFromIndexes(entry)

	mp.txsMu.Unlock()

	// Invoke removal callback outside txsMu (Lock-Copy-Invoke pattern)
	mp.onTxMu.RLock()
	removeCb := mp.onRemoveTransaction
	mp.onTxMu.RUnlock()
	if removeCb != nil {
		removeCb(hash)
	}

	// Update statistics with safe subtraction to prevent underflow if Clear() raced
	mp.statsMu.Lock()
	if mp.stats.TransactionCount > 0 {
		mp.stats.TransactionCount--
	}
	if mp.stats.TotalSize >= uint64(entry.Size) {
		mp.stats.TotalSize -= uint64(entry.Size)
	} else {
		mp.stats.TotalSize = 0
	}
	if mp.stats.TotalFees >= entry.Fee {
		mp.stats.TotalFees -= entry.Fee
	} else {
		mp.stats.TotalFees = 0
	}
	mp.stats.RemovedLast1Min++
	mp.statsMu.Unlock()

	mp.logger.WithField("tx", hash.String()).Debug("Transaction removed from mempool")

	return nil
}

// RemoveTransactions removes multiple transactions
func (mp *TxMempool) RemoveTransactions(hashes []types.Hash) error {
	for _, hash := range hashes {
		if err := mp.RemoveTransaction(hash); err != nil {
			mp.logger.WithError(err).WithField("tx", hash.String()).Warn("Failed to remove transaction")
		}
	}
	return nil
}

// RemoveConfirmedTransactions removes all transactions that were confirmed in a block,
// plus any conflicting transactions whose inputs overlap with block transactions.
// This is the primary mempool cleanup method called after every block connection
// (both locally-staked and P2P-received blocks).
// Skips coinbase (index 0) and coinstake (index 1) as they are system transactions.
func (mp *TxMempool) RemoveConfirmedTransactions(block *types.Block) {
	if block == nil || len(block.Transactions) <= 2 {
		return
	}

	// Phase 1: Collect all outpoints spent by block transactions (excluding coinbase/coinstake)
	blockSpentOutpoints := make(map[types.Outpoint]struct{})
	confirmedHashes := make(map[types.Hash]struct{})

	for _, tx := range block.Transactions[2:] {
		confirmedHashes[tx.Hash()] = struct{}{}
		for _, input := range tx.Inputs {
			blockSpentOutpoints[input.PreviousOutput] = struct{}{}
		}
	}

	// Phase 2: Find mempool transactions to remove (confirmed + conflicting)
	var toRemove []types.Hash

	mp.txsMu.RLock()
	for hash, entry := range mp.txs {
		// Remove if this tx was included in the block
		if _, confirmed := confirmedHashes[hash]; confirmed {
			toRemove = append(toRemove, hash)
			continue
		}

		// Remove if any of this tx's inputs conflict with a block transaction
		for _, input := range entry.Tx.Inputs {
			if _, conflict := blockSpentOutpoints[input.PreviousOutput]; conflict {
				toRemove = append(toRemove, hash)
				break
			}
		}
	}
	mp.txsMu.RUnlock()

	// Phase 3: Remove collected transactions (uses existing RemoveTransaction with callbacks)
	for _, hash := range toRemove {
		_ = mp.RemoveTransaction(hash)
	}

	if len(toRemove) > 0 {
		mp.logger.WithFields(logrus.Fields{
			"block":    block.Hash().String(),
			"removed":  len(toRemove),
			"included": len(confirmedHashes),
		}).Debug("Removed confirmed and conflicting transactions from mempool")
	}
}

// HasTransaction checks if a transaction exists in the mempool
func (mp *TxMempool) HasTransaction(hash types.Hash) bool {
	mp.txsMu.RLock()
	defer mp.txsMu.RUnlock()

	_, exists := mp.txs[hash]
	return exists
}

// GetTransaction retrieves a transaction from the mempool
func (mp *TxMempool) GetTransaction(hash types.Hash) (*types.Transaction, error) {
	mp.txsMu.RLock()
	defer mp.txsMu.RUnlock()

	entry, exists := mp.txs[hash]
	if !exists {
		return nil, fmt.Errorf("transaction not found")
	}

	return entry.Tx, nil
}

// UpdatePriority adjusts the priority and fee of a transaction in the mempool
// Used by prioritisetransaction RPC to boost transaction selection priority
func (mp *TxMempool) UpdatePriority(hash types.Hash, priorityDelta float64, feeDelta int64) error {
	mp.txsMu.Lock()
	defer mp.txsMu.Unlock()

	entry, exists := mp.txs[hash]
	if !exists {
		return fmt.Errorf("transaction %s not found in mempool", hash.String())
	}

	// Update priority and fee
	entry.Priority += priorityDelta
	entry.Fee += feeDelta

	// Recalculate fee rate
	if entry.Size > 0 {
		entry.FeeRate = (entry.Fee * 1000) / int64(entry.Size) // satoshis per KB
	}

	// Re-sort the fee rate index
	mp.indexMu.Lock()
	mp.rebuildFeeRateIndex()
	mp.indexMu.Unlock()

	mp.logger.WithFields(logrus.Fields{
		"txid":          hash.String(),
		"priorityDelta": priorityDelta,
		"feeDelta":      feeDelta,
		"newPriority":   entry.Priority,
		"newFee":        entry.Fee,
		"newFeeRate":    entry.FeeRate,
	}).Debug("Transaction priority updated")

	return nil
}

// rebuildFeeRateIndex rebuilds the fee rate sorted index
// Must be called with indexMu held
func (mp *TxMempool) rebuildFeeRateIndex() {
	mp.byFeeRate = make([]*TxEntry, 0, len(mp.txs))
	for _, entry := range mp.txs {
		mp.byFeeRate = append(mp.byFeeRate, entry)
	}
	// Sort by fee rate descending (highest first)
	sort.Slice(mp.byFeeRate, func(i, j int) bool {
		return mp.byFeeRate[i].FeeRate > mp.byFeeRate[j].FeeRate
	})
}

// GetTransactions returns up to maxCount transactions
func (mp *TxMempool) GetTransactions(maxCount int) []*types.Transaction {
	mp.txsMu.RLock()
	defer mp.txsMu.RUnlock()

	txs := make([]*types.Transaction, 0, maxCount)
	count := 0

	for _, entry := range mp.txs {
		if count >= maxCount {
			break
		}
		txs = append(txs, entry.Tx)
		count++
	}

	return txs
}

// GetTransactionsForBlock returns transactions suitable for including in a block
func (mp *TxMempool) GetTransactionsForBlock(maxSize uint32, maxCount int) []*types.Transaction {
	mp.txsMu.RLock()
	defer mp.txsMu.RUnlock()

	mp.indexMu.RLock()
	defer mp.indexMu.RUnlock()

	txs := make([]*types.Transaction, 0, maxCount)
	var totalSize uint32

	// Get transactions sorted by fee rate (highest first)
	for _, entry := range mp.byFeeRate {
		if len(txs) >= maxCount {
			break
		}

		if totalSize+entry.Size > maxSize {
			continue // Skip oversized tx, try smaller ones for better block filling
		}

		// Skip if has unconfirmed dependencies
		if entry.SpendsPending {
			continue
		}

		txs = append(txs, entry.Tx)
		totalSize += entry.Size
	}

	return txs
}

// GetHighPriorityTransactions returns the highest priority transactions
func (mp *TxMempool) GetHighPriorityTransactions(count int) []*types.Transaction {
	mp.txsMu.RLock()
	defer mp.txsMu.RUnlock()

	mp.indexMu.RLock()
	defer mp.indexMu.RUnlock()

	txs := make([]*types.Transaction, 0, count)

	for i := 0; i < len(mp.byFeeRate) && i < count; i++ {
		txs = append(txs, mp.byFeeRate[i].Tx)
	}

	return txs
}

// GetTransactionsByFeeRate returns transactions with at least the minimum fee rate
func (mp *TxMempool) GetTransactionsByFeeRate(minFeeRate int64, maxCount int) []*types.Transaction {
	mp.txsMu.RLock()
	defer mp.txsMu.RUnlock()

	mp.indexMu.RLock()
	defer mp.indexMu.RUnlock()

	txs := make([]*types.Transaction, 0, maxCount)

	for _, entry := range mp.byFeeRate {
		if entry.FeeRate < minFeeRate {
			break // Sorted by fee rate, so we can stop here
		}

		if len(txs) >= maxCount {
			break
		}

		txs = append(txs, entry.Tx)
	}

	return txs
}

// Count returns the number of transactions in the mempool
func (mp *TxMempool) Count() int {
	mp.txsMu.RLock()
	defer mp.txsMu.RUnlock()

	return len(mp.txs)
}

// Size returns the total size of transactions in the mempool
func (mp *TxMempool) Size() uint64 {
	mp.statsMu.RLock()
	defer mp.statsMu.RUnlock()

	return mp.stats.TotalSize
}

// GetStats returns mempool statistics
func (mp *TxMempool) GetStats() *Stats {
	mp.statsMu.RLock()
	defer mp.statsMu.RUnlock()

	// Return a copy
	stats := mp.stats
	return &stats
}

// GetOrphanTransactions returns all orphan transactions
func (mp *TxMempool) GetOrphanTransactions() []*types.Transaction {
	mp.orphMu.RLock()
	defer mp.orphMu.RUnlock()

	txs := make([]*types.Transaction, 0, len(mp.orphans))
	for _, entry := range mp.orphans {
		txs = append(txs, entry.Tx)
	}

	return txs
}

// RemoveOrphanTransaction removes an orphan transaction
func (mp *TxMempool) RemoveOrphanTransaction(hash types.Hash) error {
	mp.orphMu.Lock()
	defer mp.orphMu.Unlock()

	if _, exists := mp.orphans[hash]; !exists {
		return fmt.Errorf("orphan transaction not found")
	}

	delete(mp.orphans, hash)
	return nil
}

// validateTransaction validates a transaction
func (mp *TxMempool) validateTransaction(tx *types.Transaction) error {
	// Basic validation
	if tx == nil {
		return NewMempoolError(RejectInvalid, "transaction is nil", types.ZeroHash)
	}

	txHash := tx.Hash()

	// Check transaction size against standard limit.
	// Legacy: MAX_STANDARD_TX_SIZE = 100000 bytes (main.h:70)
	txSize := tx.SerializeSize()
	if txSize > MaxStandardTxSize {
		return NewMempoolError(RejectTooLarge,
			fmt.Sprintf("transaction size %d exceeds maximum standard size %d", txSize, MaxStandardTxSize),
			txHash)
	}

	// Check inputs
	if len(tx.Inputs) == 0 {
		return NewMempoolError(RejectInvalid, "transaction has no inputs", txHash)
	}

	// Check outputs
	if len(tx.Outputs) == 0 {
		return NewMempoolError(RejectInvalid, "transaction has no outputs", txHash)
	}

	// Validate coinbase maturity for inputs
	currentHeight, err := mp.blockchain.GetBestHeight()
	if err != nil {
		return NewMempoolError(RejectInvalid, "cannot get current height", txHash)
	}

	// Calculate total input value and check each input
	var totalInputValue int64
	for _, input := range tx.Inputs {
		// Get the UTXO being spent
		utxo, err := mp.utxoSet.GetUTXO(input.PreviousOutput)
		if err != nil || utxo == nil {
			// UTXO not found - treat as orphan (missing inputs) rather than invalid.
			return NewMempoolError(RejectOrphan,
				fmt.Sprintf("input UTXO not found: %s:%d",
					input.PreviousOutput.Hash.String(), input.PreviousOutput.Index),
				txHash)
		}

		// Check if UTXO is already spent in the blockchain (mark-as-spent model).
		// SpendingHeight > 0 indicates the UTXO was spent, but we must verify the
		// spending reference is still valid — it could be orphaned from a disconnected block.
		if utxo.SpendingHeight > 0 {
			if mp.isUTXOGenuinelySpent(utxo) {
				return NewMempoolError(RejectInvalid,
					fmt.Sprintf("input UTXO already spent in block %d: %s:%d",
						utxo.SpendingHeight,
						input.PreviousOutput.Hash.String(), input.PreviousOutput.Index),
					txHash)
			}
			// Orphaned spending reference — treat as unspent, allow the transaction
		}

		totalInputValue += utxo.Output.Value

		// Check coinbase/coinstake maturity
		if utxo.IsCoinbase {
			// Get chain params for maturity requirement
			maturity := uint32(types.DefaultCoinbaseMaturity)
			if mp.chainParams != nil {
				maturity = mp.chainParams.CoinbaseMaturity
			}

			depth := currentHeight - utxo.Height
			if depth < maturity {
				return NewMempoolError(RejectInvalid,
					fmt.Sprintf("tried to spend immature coinbase/coinstake at depth %d (required: %d)",
						depth, maturity),
					txHash)
			}
		}
	}

	// Check output dust threshold and calculate total output value
	minRelayFee := types.DefaultMinRelayTxFee
	if mp.config.MinRelayFee > 0 {
		minRelayFee = types.NewFeeRate(mp.config.MinRelayFee)
	}

	var totalOutputValue int64
	for _, output := range tx.Outputs {
		totalOutputValue += output.Value

		// Check if output is dust (too small to be worth spending)
		if types.IsDust(output.Value, minRelayFee) {
			return NewMempoolError(RejectDust,
				fmt.Sprintf("output value %d is below dust threshold", output.Value),
				txHash)
		}
	}

	// Calculate transaction fee
	fee := totalInputValue - totalOutputValue
	if fee < 0 {
		return NewMempoolError(RejectInvalid,
			"transaction has negative fee (outputs exceed inputs)",
			txHash)
	}

	// Check minimum relay fee (txSize computed at start of validateTransaction)
	minFee := types.CalculateMinFee(txSize, minRelayFee)
	if fee < minFee {
		return NewMempoolError(RejectInsufficientFee,
			fmt.Sprintf("transaction fee %d below minimum %d", fee, minFee),
			txHash)
	}

	// Check for insanely high fees (likely user error)
	if types.IsFeeTooHigh(fee, txSize, minRelayFee) {
		return NewMempoolError(RejectInvalid,
			fmt.Sprintf("transaction fee %d is insanely high (max reasonable: %d)",
				fee, minRelayFee.GetFee(txSize)*types.MaxTxFeeMultiplier),
			txHash)
	}

	// Full script and ECDSA signature verification for every input.
	//
	// Without this check, an incoming transaction whose DER structure parses
	// but whose signatures do not verify against (pubkey, sighash) is accepted
	// into our mempool and relayed to peers. Legacy C++ nodes then reject the
	// same transaction at CScriptCheck and ban us for propagating invalid data
	// (see legacy/src/main.cpp:2102-2107 CScriptCheck::operator()).
	//
	// Mirrors the per-input loop in consensus/validation.go:773-787 used by
	// block validation. StandardScriptVerifyFlags is an intentional subset of
	// legacy STANDARD_SCRIPT_VERIFY_FLAGS (P2SH + strict DER + strict encoding),
	// chosen so the Go mempool never rejects a tx that legacy would accept.
	for i, input := range tx.Inputs {
		utxo, err := mp.utxoSet.GetUTXO(input.PreviousOutput)
		if err != nil || utxo == nil || utxo.Output == nil {
			// Defensive: the earlier orphan loop already rejected missing-input
			// txs, but the UTXO-set view can shift between the two reads.
			return NewMempoolError(RejectOrphan,
				fmt.Sprintf("input UTXO missing during script verification: %s:%d",
					input.PreviousOutput.Hash.String(), input.PreviousOutput.Index),
				txHash)
		}

		if err := script.VerifyScript(
			input.ScriptSig,
			utxo.Output.ScriptPubKey,
			tx,
			i,
			script.StandardScriptVerifyFlags,
		); err != nil {
			return NewMempoolError(RejectInvalid,
				fmt.Sprintf("script verification failed for input %d (%s:%d): %v",
					i, input.PreviousOutput.Hash.String(), input.PreviousOutput.Index, err),
				txHash)
		}
	}

	return nil
}

// isUTXOGenuinelySpent verifies that a UTXO's spending reference is still valid.
// A UTXO with SpendingHeight > 0 may have an orphaned reference from a disconnected
// block (fork recovery, rollback). We verify by checking that the spending block and
// transaction still exist in the active chain.
// Returns true if the UTXO is genuinely spent, false if the reference is orphaned.
func (mp *TxMempool) isUTXOGenuinelySpent(utxo *types.UTXO) bool {
	// Verify block exists at the spending height
	blockHash, err := mp.blockchain.GetBlockHash(utxo.SpendingHeight)
	if err != nil {
		// No block at that height — orphaned reference
		return false
	}

	// Verify the spending transaction is in that block
	block, err := mp.blockchain.GetBlock(blockHash)
	if err != nil {
		return false
	}

	for _, tx := range block.Transactions {
		if tx.Hash() == utxo.SpendingTxHash {
			return true // Genuinely spent
		}
	}

	// Spending tx not found in the block — orphaned reference
	return false
}

// checkConflicts checks for conflicting transactions using O(1) spent-outpoint index.
func (mp *TxMempool) checkConflicts(tx *types.Transaction, txHash types.Hash) error {
	mp.txsMu.RLock()
	defer mp.txsMu.RUnlock()

	for _, input := range tx.Inputs {
		if _, exists := mp.spentOutpoints[input.PreviousOutput]; exists {
			return NewMempoolError(RejectConflict,
				"transaction spends output already spent by mempool transaction",
				txHash)
		}
	}

	return nil
}

// createTxEntry creates a mempool entry for a transaction
func (mp *TxMempool) createTxEntry(tx *types.Transaction, txHash types.Hash) (*TxEntry, error) {
	size := uint32(tx.SerializeSize())

	// Calculate fee by getting actual UTXO values
	var totalInputValue int64
	var totalOutputValue int64

	// Calculate total input value from UTXOs
	for _, input := range tx.Inputs {
		if mp.utxoSet != nil {
			utxo, err := mp.utxoSet.GetUTXO(input.PreviousOutput)
			if err != nil || utxo == nil {
				// UTXO should have been validated already
				continue
			}
			totalInputValue += utxo.Output.Value
		}
	}

	// Calculate total output value
	for _, output := range tx.Outputs {
		totalOutputValue += output.Value
	}

	// Fee is the difference
	fee := totalInputValue - totalOutputValue
	if fee < 0 {
		fee = 0 // Should not happen if validated properly
	}

	// Calculate fee rate using the types.FeeRate system
	feeRate := types.NewFeeRateFromAmount(fee, int(size))

	// Get current height
	height, _ := mp.blockchain.GetBestHeight()

	// Calculate priority (can be enhanced with coin age for legacy compatibility)
	// For now, use fee rate as priority
	priority := float64(feeRate.GetFeePerKB())

	entry := &TxEntry{
		Tx:            tx,
		Hash:          txHash,
		Size:          size,
		Fee:           fee,
		FeeRate:       feeRate.GetFeePerKB(),
		Time:          time.Now(),
		Height:        height,
		Priority:      priority,
		Dependencies:  make([]types.Hash, 0),
		Descendants:   make([]types.Hash, 0),
		SpendsPending: false,
	}

	return entry, nil
}

// checkCapacity checks if mempool has capacity for a new transaction
func (mp *TxMempool) checkCapacity(entry *TxEntry) error {
	mp.statsMu.RLock()
	defer mp.statsMu.RUnlock()

	// Check transaction count
	if mp.stats.TransactionCount >= mp.config.MaxTransactions {
		return NewMempoolError(RejectPoolFull,
			fmt.Sprintf("mempool full (%d transactions)", mp.stats.TransactionCount),
			entry.Hash)
	}

	// Check size
	if mp.stats.TotalSize+uint64(entry.Size) > mp.config.MaxSize {
		return NewMempoolError(RejectPoolFull,
			fmt.Sprintf("mempool size full (%d bytes)", mp.stats.TotalSize),
			entry.Hash)
	}

	return nil
}

// updateIndexes updates the fee rate index
func (mp *TxMempool) updateIndexes(entry *TxEntry) {
	mp.indexMu.Lock()
	defer mp.indexMu.Unlock()

	// Insert in sorted order by fee rate (highest first)
	inserted := false
	for i, existing := range mp.byFeeRate {
		if entry.FeeRate > existing.FeeRate {
			// Insert before this entry
			mp.byFeeRate = append(mp.byFeeRate[:i], append([]*TxEntry{entry}, mp.byFeeRate[i:]...)...)
			inserted = true
			break
		}
	}

	if !inserted {
		mp.byFeeRate = append(mp.byFeeRate, entry)
	}
}

// removeFromIndexes removes an entry from indexes
func (mp *TxMempool) removeFromIndexes(entry *TxEntry) {
	mp.indexMu.Lock()
	defer mp.indexMu.Unlock()

	// Remove from fee rate index
	for i, existing := range mp.byFeeRate {
		if existing.Hash == entry.Hash {
			mp.byFeeRate = append(mp.byFeeRate[:i], mp.byFeeRate[i+1:]...)
			break
		}
	}
}

// updateAddStats updates statistics after adding a transaction
func (mp *TxMempool) updateAddStats(entry *TxEntry, duration time.Duration) {
	mp.statsMu.Lock()
	defer mp.statsMu.Unlock()

	mp.stats.TransactionCount++
	mp.stats.TotalSize += uint64(entry.Size)
	mp.stats.TotalFees += entry.Fee
	mp.stats.AddedLast1Min++

	// Update average add time
	if mp.stats.AverageAddTime == 0 {
		mp.stats.AverageAddTime = duration
	} else {
		mp.stats.AverageAddTime = (mp.stats.AverageAddTime + duration) / 2
	}

	// Update fee rate stats
	if entry.FeeRate < mp.stats.MinFeeRate || mp.stats.MinFeeRate == 0 {
		mp.stats.MinFeeRate = entry.FeeRate
	}
	if entry.FeeRate > mp.stats.MaxFeeRate {
		mp.stats.MaxFeeRate = entry.FeeRate
	}
}

// incrementRejections increments rejection counter
func (mp *TxMempool) incrementRejections() {
	mp.statsMu.Lock()
	defer mp.statsMu.Unlock()

	mp.stats.RejectedCount++
}

// SetOnTransaction registers a callback invoked when a transaction is accepted into the mempool.
// The callback is invoked outside txsMu to avoid deadlock with wallet locks.
func (mp *TxMempool) SetOnTransaction(fn func(*types.Transaction)) {
	mp.onTxMu.Lock()
	defer mp.onTxMu.Unlock()
	mp.onTransaction = fn
}

// SetOnRemoveTransaction registers a callback invoked when a transaction is removed from the mempool
// (explicit removal, expiry, or eviction). Used by wallet to evict pending state.
// The callback is invoked outside txsMu to avoid deadlock with wallet locks.
func (mp *TxMempool) SetOnRemoveTransaction(fn func(types.Hash)) {
	mp.onTxMu.Lock()
	defer mp.onTxMu.Unlock()
	mp.onRemoveTransaction = fn
}

// SetOnOrphanAccepted registers a callback invoked when an orphan transaction
// becomes valid and is accepted after its parent arrives.
func (mp *TxMempool) SetOnOrphanAccepted(fn func(*types.Transaction)) {
	mp.onTxMu.Lock()
	defer mp.onTxMu.Unlock()
	mp.onOrphanAccepted = fn
}

// cleanupRoutine periodically cleans up the mempool
func (mp *TxMempool) cleanupRoutine() {
	defer mp.wg.Done()

	ticker := time.NewTicker(mp.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			mp.cleanup()
		case <-mp.stopChan:
			return
		}
	}
}

// expiryRoutine periodically removes expired transactions
func (mp *TxMempool) expiryRoutine() {
	defer mp.wg.Done()

	ticker := time.NewTicker(mp.config.ExpiryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			mp.removeExpired()
		case <-mp.stopChan:
			return
		}
	}
}

// cleanup performs mempool cleanup
func (mp *TxMempool) cleanup() {
	mp.logger.Debug("Running mempool cleanup")

	// Reset rate counters
	mp.statsMu.Lock()
	mp.stats.AddedLast5Min = mp.stats.AddedLast1Min
	mp.stats.RemovedLast5Min = mp.stats.RemovedLast1Min
	mp.stats.AddedLast1Min = 0
	mp.stats.RemovedLast1Min = 0
	mp.statsMu.Unlock()
}

// removeExpired removes expired transactions
func (mp *TxMempool) removeExpired() {
	mp.txsMu.Lock()

	now := time.Now()
	type expiredEntry struct {
		hash types.Hash
		size uint32
		fee  int64
	}
	var expired []expiredEntry

	for hash, entry := range mp.txs {
		if now.Sub(entry.Time) > mp.config.MaxTransactionAge {
			expired = append(expired, expiredEntry{hash: hash, size: entry.Size, fee: entry.Fee})
			// Clean spent-outpoint index for each expired tx
			for _, input := range entry.Tx.Inputs {
				delete(mp.spentOutpoints, input.PreviousOutput)
			}
		}
	}

	for _, e := range expired {
		if entry, exists := mp.txs[e.hash]; exists {
			mp.removeFromIndexes(entry)
		}
		delete(mp.txs, e.hash)
	}

	mp.txsMu.Unlock()

	// Update stats and invoke removal callbacks outside txsMu
	if len(expired) > 0 {
		// Batch stats update in single lock to avoid race with Clear() resetting to zero.
		// Use safe subtraction to prevent underflow if Clear() ran between txsMu release and here.
		mp.statsMu.Lock()
		for _, e := range expired {
			if mp.stats.TransactionCount > 0 {
				mp.stats.TransactionCount--
			}
			if mp.stats.TotalSize >= uint64(e.size) {
				mp.stats.TotalSize -= uint64(e.size)
			} else {
				mp.stats.TotalSize = 0
			}
			if mp.stats.TotalFees >= e.fee {
				mp.stats.TotalFees -= e.fee
			} else {
				mp.stats.TotalFees = 0
			}
			mp.stats.EvictedCount++
			mp.stats.RemovedLast1Min++
		}
		mp.statsMu.Unlock()

		mp.onTxMu.RLock()
		removeCb := mp.onRemoveTransaction
		mp.onTxMu.RUnlock()

		if removeCb != nil {
			for _, e := range expired {
				removeCb(e.hash)
			}
		}

		mp.logger.WithField("count", len(expired)).Debug("Removed expired transactions")
	}
}
