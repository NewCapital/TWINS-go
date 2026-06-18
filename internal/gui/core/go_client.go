package core

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io/fs"
	"math"
	"math/big"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/twins-dev/twins-core/internal/blockchain"
	"github.com/twins-dev/twins-core/internal/daemon"
	"github.com/twins-dev/twins-core/internal/masternode"
	"github.com/twins-dev/twins-core/internal/spork"
	"github.com/twins-dev/twins-core/internal/storage"
	"github.com/twins-dev/twins-core/internal/storage/binary"
	"github.com/twins-dev/twins-core/internal/wallet"
	"github.com/twins-dev/twins-core/pkg/crypto"
	pkgscript "github.com/twins-dev/twins-core/pkg/script"
	"github.com/twins-dev/twins-core/pkg/types"
)

// satoshisPerTWINS is the conversion factor between satoshis and TWINS.
const satoshisPerTWINS = 100_000_000.0

// SyncerInterface defines the methods needed from the P2P syncer
// to provide sync status information without circular imports.
type SyncerInterface interface {
	// IsSyncing returns whether we're currently syncing
	IsSyncing() bool
	// IsSynced returns whether the node is synced with the network
	IsSynced() bool
	// GetSyncProgress returns current height, target height, and sync peer address
	GetSyncProgress() (current, target uint32, peer string)
	// GetNetworkHeight returns the best known network height from peer consensus
	GetNetworkHeight() uint32
}

// P2PServerInterface defines the methods needed from the P2P server
// to provide network status information without circular imports.
type P2PServerInterface interface {
	// GetPeerCount returns the current peer count
	GetPeerCount() int32
	// GetInboundCount returns the current inbound peer count
	GetInboundCount() int32
	// GetOutboundCount returns the current outbound peer count
	GetOutboundCount() int32
	// IsStarted returns whether the server is started
	IsStarted() bool
}

// ConsensusInterface defines the methods needed from the consensus engine
// to provide staking status information without circular imports.
type ConsensusInterface interface {
	// IsStaking returns whether the consensus engine is actively staking
	IsStaking() bool
}

// BlockchainSupplyInterface exposes per-height chain-intrinsic data not covered
// by the syncer / P2P / consensus interfaces. *blockchain.BlockChain satisfies
// this via its existing GetMoneySupply method.
type BlockchainSupplyInterface interface {
	// GetMoneySupply returns the total satoshis in circulation at the given height.
	GetMoneySupply(height uint32) (int64, error)
}

// GoCoreClient implements CoreClient with direct storage access.
// This is the production implementation that reads blockchain data
// directly from the Pebble storage layer.
type GoCoreClient struct {
	storage storage.Storage

	// Full daemon components (optional, for full functionality)
	wallet         *wallet.Wallet
	masternode     *masternode.Manager
	spork          *spork.Manager
	paymentTracker *masternode.PaymentTracker // Payment stats for LastPaid (optional)
	syncer         SyncerInterface            // P2P syncer for sync status (optional)
	p2pServer      P2PServerInterface         // P2P server for network info (optional)
	consensus      ConsensusInterface         // Consensus engine for staking info (optional)
	blockchain     BlockchainSupplyInterface  // Blockchain supply lookup for money_supply (optional)
	// Difficulty is computed inline from the cached tip block in
	// GetBlockchainInfo; no separate component reference is needed.
	dataDir string // Data directory for chain-size walks (optional)

	// Staking configuration
	stakingEnabled bool // Whether staking is enabled in settings

	// Network name (e.g. "mainnet" / "testnet" / "regtest") used by address
	// encoding helpers that need network-aware prefixes. Empty string falls back
	// to mainnet at crypto.GetPubKeyHashNetworkID / crypto.GetScriptHashNetworkID.
	// Wired from daemon.Node.ChainParams.Name via SetNetwork in
	// cmd/twins-gui/app.go.
	network string

	// chainParams is the active chain parameter snapshot. Used by blockToDetail
	// to locate the dev fund output in PoS coinstakes by matching against
	// chainParams.DevAddress (the canonical scriptPubKey of the dev fund payout).
	// Nil falls back to legacy positional parsing (outputs[2] for MN, no dev).
	// Wired from daemon.Node.ChainParams via SetChainParams in cmd/twins-gui/app.go.
	chainParams *types.ChainParams

	// State
	running bool
	mu      sync.RWMutex

	// Cached chain tip timestamp to avoid full-block reads on every status poll.
	// Invalidated when tipHash changes; protected by tipTimeMu.
	tipTimeMu   sync.Mutex
	tipTimeHash types.Hash
	tipTimeUnix int64

	// Cached chain-size-on-disk with chainSizeCacheTTL TTL (60s). Uses a
	// stale-while-revalidate pattern: callers always get the cached value
	// instantly (returning 0 only on the very first call before any walk
	// has completed), and an expired cache triggers a background refresh
	// goroutine — the GUI status RPC hot path never blocks on the walk.
	// chainSizeWalking is an atomic guard so at most one refresh goroutine
	// is in flight at a time.
	chainSizeMu      sync.Mutex
	chainSizeBytes   int64
	chainSizeExpiry  time.Time
	chainSizeWalking atomic.Bool

	// Event system
	events     chan CoreEvent
	eventsDone chan struct{}

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

// NewGoCoreClient creates a new GoCoreClient with the given storage.
// This is the basic constructor for backward compatibility.
func NewGoCoreClient(store storage.Storage) *GoCoreClient {
	return &GoCoreClient{
		storage:    store,
		events:     make(chan CoreEvent, 100),
		eventsDone: make(chan struct{}),
	}
}

// NewGoCoreClientWithComponents creates a new GoCoreClient with full daemon components.
// This constructor provides access to masternode and spork functionality.
// Note: Wallet is not yet implemented (requires legacy.CMasterKey types).
func NewGoCoreClientWithComponents(components *daemon.CoreComponents) *GoCoreClient {
	client := &GoCoreClient{
		storage:    components.Storage,
		masternode: components.Masternode,
		spork:      components.Spork,
		events:     make(chan CoreEvent, 100),
		eventsDone: make(chan struct{}),
	}
	return client
}

// SetWallet sets the wallet instance for transaction operations.
func (c *GoCoreClient) SetWallet(w *wallet.Wallet) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wallet = w
}

// SetSyncer sets the P2P syncer for sync status information.
func (c *GoCoreClient) SetSyncer(s SyncerInterface) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.syncer = s
}

// SetP2PServer sets the P2P server for network status information.
func (c *GoCoreClient) SetP2PServer(p P2PServerInterface) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.p2pServer = p
}

// SetPaymentTracker sets the payment tracker for masternode LastPaid lookups.
func (c *GoCoreClient) SetPaymentTracker(tracker *masternode.PaymentTracker) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.paymentTracker = tracker
}

// SetConsensus sets the consensus engine for staking status information.
func (c *GoCoreClient) SetConsensus(cons ConsensusInterface) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.consensus = cons
}

// SetBlockchain wires a per-height supply lookup so GetBlockchainInfo can
// populate BlockchainInfo.MoneySupply. Mirrors the SetMempool / SetConsensus
// optional-component pattern.
func (c *GoCoreClient) SetBlockchain(bc BlockchainSupplyInterface) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.blockchain = bc
}

// SetDataDir wires the data directory path so the chain-size walker can
// find blockchain.db. Documented as called once at startup, but defensively
// invalidates the chain-size cache so a re-call (e.g. tests, reconfiguration)
// does not return a stale walk result from a previous data directory.
func (c *GoCoreClient) SetDataDir(dir string) {
	c.mu.Lock()
	c.dataDir = dir
	c.mu.Unlock()

	c.chainSizeMu.Lock()
	c.chainSizeBytes = 0
	c.chainSizeExpiry = time.Time{}
	c.chainSizeMu.Unlock()
}

// SetNetwork wires the active network name ("mainnet" / "testnet" / "regtest")
// so address-encoding helpers can pick the correct address prefix. Called once
// at startup from cmd/twins-gui/app.go from daemon.Node.ChainParams.Name.
func (c *GoCoreClient) SetNetwork(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.network = name
}

// SetChainParams wires the active chain parameter snapshot so blockToDetail
// can locate the dev fund output by matching scriptPubKey against
// chainParams.DevAddress. Called once at startup from cmd/twins-gui/app.go
// from daemon.Node.ChainParams (alongside SetNetwork).
func (c *GoCoreClient) SetChainParams(params *types.ChainParams) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.chainParams = params
}

// SetStakingEnabled sets whether staking is enabled in GUI settings.
func (c *GoCoreClient) SetStakingEnabled(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stakingEnabled = enabled
}

// Start initializes the core client.
func (c *GoCoreClient) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return fmt.Errorf("core client already running")
	}

	c.ctx, c.cancel = context.WithCancel(ctx)
	c.running = true

	return nil
}

// Stop gracefully shuts down the core client.
func (c *GoCoreClient) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}

	c.cancel()
	close(c.events)
	c.running = false

	return nil
}

// IsRunning returns true if the core is running.
func (c *GoCoreClient) IsRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.running
}

// Events returns a channel for core events.
func (c *GoCoreClient) Events() <-chan CoreEvent {
	return c.events
}

// ==========================================
// Explorer Operations (Real Implementation)
// ==========================================

// GetLatestBlocks returns the most recent blocks.
func (c *GoCoreClient) GetLatestBlocks(limit, offset int) ([]BlockSummary, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}

	height, err := c.storage.GetChainHeight()
	if err != nil {
		return nil, fmt.Errorf("failed to get chain height: %w", err)
	}

	blocks := make([]BlockSummary, 0, limit)
	startHeight := int64(height) - int64(offset)

	for i := 0; i < limit && startHeight-int64(i) >= 0; i++ {
		h := uint32(startHeight - int64(i))
		block, err := c.storage.GetBlockByHeight(h)
		if err != nil {
			continue // Skip missing blocks
		}

		summary := c.blockToSummary(block, h)
		blocks = append(blocks, summary)
	}

	return blocks, nil
}

// GetExplorerBlock returns detailed block information.
func (c *GoCoreClient) GetExplorerBlock(query string) (BlockDetail, error) {
	var block *types.Block
	var height uint32
	var err error

	// Try parsing as height first
	if h, parseErr := strconv.ParseUint(query, 10, 32); parseErr == nil {
		height = uint32(h)
		block, err = c.storage.GetBlockByHeight(height)
	} else {
		// Parse as hash (display format is reversed)
		hashBytes, decodeErr := hex.DecodeString(query)
		if decodeErr != nil || len(hashBytes) != 32 {
			return BlockDetail{}, fmt.Errorf("invalid block hash or height: %s", query)
		}
		// Reverse bytes from display format to internal format
		var hash types.Hash
		for i := 0; i < 32; i++ {
			hash[i] = hashBytes[31-i]
		}
		block, err = c.storage.GetBlock(hash)
		if err == nil {
			height, err = c.storage.GetBlockHeight(hash)
		}
	}

	if err != nil {
		return BlockDetail{}, fmt.Errorf("block not found: %w", err)
	}

	return c.blockToDetail(block, height)
}

// GetExplorerTransaction returns detailed transaction information.
func (c *GoCoreClient) GetExplorerTransaction(txid string) (ExplorerTransaction, error) {
	hashBytes, err := hex.DecodeString(txid)
	if err != nil || len(hashBytes) != 32 {
		return ExplorerTransaction{}, fmt.Errorf("invalid transaction id: %s", txid)
	}

	// Reverse bytes from display format to internal format
	var hash types.Hash
	for i := 0; i < 32; i++ {
		hash[i] = hashBytes[31-i]
	}

	txData, err := c.storage.GetTransactionData(hash)
	if err != nil {
		return ExplorerTransaction{}, fmt.Errorf("transaction not found: %w", err)
	}

	return c.txToExplorerTx(txData)
}

// GetAddressBasic returns the minimal, O(1) subset of address information
// (Address only) after crypto.DecodeAddress validation. No storage access.
// Returns instantly regardless of the address's historical activity or
// current UTXO set size, so the Explorer Address Detail hero header
// (address text + QR code) and the address-search code path are never
// blocked by per-address work.
//
// Balance is fetched separately via GetAddressBalance (UTXO prefix scan,
// O(U) cost). Aggregate stats are fetched via GetAddressStats (O(N) tx
// history walk, the slow path).
func (c *GoCoreClient) GetAddressBasic(address string) (AddressBasic, error) {
	if _, err := crypto.DecodeAddress(address); err != nil {
		return AddressBasic{}, fmt.Errorf("invalid address: %w", err)
	}
	return AddressBasic{Address: address}, nil
}

// GetAddressBalance returns the address's current spendable balance via a
// GetUTXOsByAddress prefix scan and sat-sum across the resulting UTXO set.
// Cost is O(U) where U = current UTXO count for the address. For addresses
// with large UTXO sets (e.g. high-traffic payment addresses) this can take
// seconds; called separately from GetAddressBasic so the hero header is
// not blocked. Does NOT walk the address tx history index.
//
// Known duplication: GetAddressUTXOs also performs the same prefix scan
// when the UTXOs panel mounts. A shared LRU cache is a candidate follow-up
// but adds invalidation complexity (UTXO set changes on every new block)
// and is not required to deliver the perceived-instant page-open win.
func (c *GoCoreClient) GetAddressBalance(address string) (AddressBalance, error) {
	if _, err := crypto.DecodeAddress(address); err != nil {
		return AddressBalance{}, fmt.Errorf("invalid address: %w", err)
	}

	utxos, err := c.storage.GetUTXOsByAddress(address)
	if err != nil && !storage.IsNotFoundError(err) {
		return AddressBalance{}, fmt.Errorf("failed to get UTXOs: %w", err)
	}

	var balanceSat int64
	for _, utxo := range utxos {
		balanceSat += utxo.Output.Value
	}

	return AddressBalance{
		Balance: float64(balanceSat) / satoshisPerTWINS,
	}, nil
}

// GetAddressStats returns the expensive aggregate statistics for an
// address: TxCount, TotalReceived, TotalSent, FirstSeen, LastSeen.
//
// Implementation: a single Pebble prefix scan via
// storage.GetAddressAggregates over the 0x05 address-history index. Each
// 0x05 entry carries (IsInput, Value, BlockHash) in its 41-byte value
// payload, written at index time by IndexTransactionByAddress. Summing
// these values is sufficient to compute TotalReceived/TotalSent without
// loading any transaction body. TxCount is the count of UNIQUE txhashes
// touched (one tx with N outputs/inputs to the address contributes once,
// not N times).
//
// Replaced the prior O(N + N×I) per-transaction load (N tx-data reads
// plus I previous-tx reads per input) with this O(N) index walk to fix
// the 50+ second latency on high-activity addresses (1M+ tx). The prior
// implementation also had a per-tx-multi-output over-count bug:
// transactions with multiple outputs to the same address produced
// multiple index entries, and the inner output loop summed ALL matching
// outputs on EACH outer iteration, multiplying the contribution. The
// new index-value walk reads each scalar value exactly once, so this
// bug is fixed as a side effect.
//
// FirstSeen/LastSeen are resolved via AT MOST 2 GetBlockByHeight lookups
// (one for the min height, one for the max — collapsed to 1 when the
// address has txs in a single block, 0 when the address has no txs).
//
// AddressAggregates' MinHeight/MaxHeight are tracked during the same
// prefix scan because AddressHistoryKey encodes height as little-endian,
// so Pebble lexicographic iteration is NOT in height order — the
// explicit min/max scan is load-bearing.
//
// The 0x05 index contains only CONFIRMED transactions (written during
// block processing); unconfirmed mempool txs do not appear here. An
// address with only unconfirmed activity returns TxCount=0 and
// FirstSeen=LastSeen=0; the frontend renders "N/A" which is the correct
// UX for "no confirmed activity yet".
func (c *GoCoreClient) GetAddressStats(address string) (AddressStats, error) {
	addr, err := crypto.DecodeAddress(address)
	if err != nil {
		return AddressStats{}, fmt.Errorf("invalid address: %w", err)
	}
	addressBinary := make([]byte, 21)
	addressBinary[0] = addr.NetID()
	copy(addressBinary[1:], addr.Hash160())

	agg, err := c.storage.GetAddressAggregates(addressBinary)
	if err != nil {
		return AddressStats{}, fmt.Errorf("get address aggregates: %w", err)
	}

	var firstSeen, lastSeen int64
	if agg.HasHeights {
		if block, err := c.storage.GetBlockByHeight(agg.MinHeight); err == nil && block != nil {
			firstSeen = int64(block.Header.Timestamp)
		}
		if agg.MaxHeight == agg.MinHeight {
			lastSeen = firstSeen
		} else {
			if block, err := c.storage.GetBlockByHeight(agg.MaxHeight); err == nil && block != nil {
				lastSeen = int64(block.Header.Timestamp)
			}
		}
	}

	return AddressStats{
		TxCount:       agg.TxCount,
		TotalReceived: float64(agg.TotalReceivedSat) / satoshisPerTWINS,
		TotalSent:     float64(agg.TotalSentSat) / satoshisPerTWINS,
		FirstSeen:     firstSeen,
		LastSeen:      lastSeen,
	}, nil
}

// AddressTxPage represents a page of address transactions
type AddressTxPage struct {
	Transactions []AddressTx `json:"transactions"`
	Total        int         `json:"total"`
	HasMore      bool        `json:"has_more"`
}

// GetAddressTransactions returns a page of transactions for an address.
// limit: number of transactions per page (1-10000)
// offset: starting position (0-based, from most recent)
func (c *GoCoreClient) GetAddressTransactions(address string, limit, offset int) (AddressTxPage, error) {
	// Input validation to prevent DoS and memory issues
	if limit <= 0 {
		limit = 50
	}
	if limit > 10000 {
		return AddressTxPage{}, fmt.Errorf("invalid limit: %d (must be 1-10000)", limit)
	}
	if offset < 0 {
		return AddressTxPage{}, fmt.Errorf("invalid offset: %d (must be >= 0)", offset)
	}

	// Decode address
	addr, err := crypto.DecodeAddress(address)
	if err != nil {
		return AddressTxPage{}, fmt.Errorf("invalid address: %w", err)
	}
	addressBinary := make([]byte, 21)
	addressBinary[0] = addr.NetID()
	copy(addressBinary[1:], addr.Hash160())

	// Get all transaction references
	addrTxs, err := c.storage.GetTransactionsByAddress(addressBinary)
	if err != nil && !storage.IsNotFoundError(err) {
		return AddressTxPage{}, fmt.Errorf("failed to get address transactions: %w", err)
	}

	// Pebble's lexicographic iteration over AddressHistoryKey does NOT yield
	// height-ascending order: the height inside the key is encoded via
	// binary.LittleEndian, so the least-significant byte sorts first. Without
	// an explicit sort here, the page would return transactions in arbitrary
	// order. Sort descending (height DESC, txIndex DESC within a block) so
	// pagination produces a stable newest-first view across pages. See the
	// "Critical implementation note" in internal/gui/core/CLAUDE.md under the
	// Address Detail Hero entry for the original write-up of this constraint.
	sort.SliceStable(addrTxs, func(i, j int) bool {
		if addrTxs[i].Height != addrTxs[j].Height {
			return addrTxs[i].Height > addrTxs[j].Height
		}
		return addrTxs[i].TxIndex > addrTxs[j].TxIndex
	})

	total := len(addrTxs)
	transactions := make([]AddressTx, 0, limit)
	chainHeight, _ := c.storage.GetChainHeight()

	startIdx := offset
	if startIdx > total {
		startIdx = total
	}
	endIdx := startIdx + limit
	if endIdx > total {
		endIdx = total
	}

	var failedCount int
	for i := startIdx; i < endIdx; i++ {
		addrTx := addrTxs[i]

		txData, err := c.storage.GetTransactionData(addrTx.TxHash)
		if err != nil {
			failedCount++
			log.Warnf("Failed to load transaction %s for address %s: %v",
				addrTx.TxHash.String(), address, err)
			continue
		}

		// Calculate net amount for this address
		var netAmount int64

		// Add outputs to this address
		for _, output := range txData.TxData.Outputs {
			outAddr := c.extractAddressFromScript(output.ScriptPubKey)
			if outAddr == address {
				netAmount += output.Value
			}
		}

		// Subtract inputs from this address
		for _, input := range txData.TxData.Inputs {
			// Skip coinbase inputs
			if input.PreviousOutput.Hash.IsZero() {
				continue
			}
			// Get the previous transaction to find the input value
			prevTxData, err := c.storage.GetTransactionData(input.PreviousOutput.Hash)
			if err != nil {
				continue
			}
			if int(input.PreviousOutput.Index) < len(prevTxData.TxData.Outputs) {
				prevOutput := prevTxData.TxData.Outputs[input.PreviousOutput.Index]
				prevAddr := c.extractAddressFromScript(prevOutput.ScriptPubKey)
				if prevAddr == address {
					netAmount -= prevOutput.Value
				}
			}
		}

		confirmations := int(chainHeight) - int(txData.Height) + 1
		if confirmations < 0 {
			confirmations = 0
		}

		// Get block for timestamp
		block, _ := c.storage.GetBlockByHeight(txData.Height)
		var txTime time.Time
		if block != nil {
			txTime = time.Unix(int64(block.Header.Timestamp), 0)
		}

		transactions = append(transactions, AddressTx{
			TxID:          addrTx.TxHash.String(),
			BlockHeight:   int64(txData.Height),
			Time:          txTime,
			Amount:        float64(netAmount) / satoshisPerTWINS,
			Confirmations: confirmations,
		})
	}

	if failedCount > 0 {
		log.Warnf("Failed to load %d/%d transactions for address %s",
			failedCount, total, address)
	}

	hasMore := offset+len(transactions) < total

	return AddressTxPage{
		Transactions: transactions,
		Total:        total,
		HasMore:      hasMore,
	}, nil
}

// GetAddressUTXOs returns a page of unspent outputs for an address.
// limit: number of UTXOs per page (1-10000)
// offset: starting position (0-based, from newest first by confirmations)
//
// Sort order: confirmations ASC (newest first), matching the convention
// already used by AddressView for the legacy preloaded list. UTXOs of
// identical confirmation get a stable secondary sort by txid+vout.
//
// The full UTXO set for an address must be loaded from storage to compute
// the total and slice the page (Pebble's GetUTXOsByAddress has no offset
// support). The cost is O(n) in the address's UTXO count per call, which
// is acceptable on the GUI's 10s+ refresh cadence even for high-UTXO
// addresses (the storage iteration is sub-millisecond per UTXO).
func (c *GoCoreClient) GetAddressUTXOs(address string, limit, offset int) (AddressUTXOPage, error) {
	// Input validation to prevent DoS and memory issues
	if limit <= 0 {
		limit = 50
	}
	if limit > 10000 {
		return AddressUTXOPage{}, fmt.Errorf("invalid limit: %d (must be 1-10000)", limit)
	}
	if offset < 0 {
		return AddressUTXOPage{}, fmt.Errorf("invalid offset: %d (must be >= 0)", offset)
	}

	// Validate address (DecodeAddress on the string form; we don't need the
	// binary form because GetUTXOsByAddress accepts the string address).
	if _, err := crypto.DecodeAddress(address); err != nil {
		return AddressUTXOPage{}, fmt.Errorf("invalid address: %w", err)
	}

	// Get all UTXOs from storage. Mirror GetAddressBasic which uses the
	// same accessor (and the prior GetAddressInfo before the basic/stats
	// split).
	utxos, err := c.storage.GetUTXOsByAddress(address)
	if err != nil && !storage.IsNotFoundError(err) {
		return AddressUTXOPage{}, fmt.Errorf("failed to get utxos: %w", err)
	}

	chainHeight, _ := c.storage.GetChainHeight()

	// Convert storage UTXOs to []AddressUTXO.
	all := make([]AddressUTXO, 0, len(utxos))
	for _, u := range utxos {
		confirmations := int(chainHeight) - int(u.Height) + 1
		if confirmations < 0 {
			confirmations = 0
		}
		all = append(all, AddressUTXO{
			TxID:          u.Outpoint.Hash.String(),
			Vout:          u.Outpoint.Index,
			Amount:        float64(u.Output.Value) / satoshisPerTWINS,
			Confirmations: confirmations,
			BlockHeight:   int64(u.Height),
		})
	}

	// Sort by confirmations ASC (newest first). Secondary sort on TxID+Vout
	// for stable ordering across calls (otherwise two equally-confirmed UTXOs
	// would shuffle order between page fetches based on the storage
	// iteration order).
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Confirmations != all[j].Confirmations {
			return all[i].Confirmations < all[j].Confirmations
		}
		if all[i].TxID != all[j].TxID {
			return all[i].TxID < all[j].TxID
		}
		return all[i].Vout < all[j].Vout
	})

	total := len(all)

	// Slice the page. Out-of-range offset returns an empty slice + HasMore=false.
	if offset >= total {
		return AddressUTXOPage{
			Utxos:   []AddressUTXO{},
			Total:   total,
			HasMore: false,
		}, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}

	page := make([]AddressUTXO, end-offset)
	copy(page, all[offset:end])

	return AddressUTXOPage{
		Utxos:   page,
		Total:   total,
		HasMore: end < total,
	}, nil
}

// SearchExplorer searches for a block, transaction, or address.
func (c *GoCoreClient) SearchExplorer(query string) (SearchResult, error) {
	result := SearchResult{Query: query}

	// Try block height
	if height, err := strconv.ParseUint(query, 10, 32); err == nil {
		block, err := c.GetExplorerBlock(fmt.Sprintf("%d", height))
		if err == nil {
			result.Type = "block"
			result.Block = &block
			return result, nil
		}
	}

	// Try block hash (64 hex chars)
	if len(query) == 64 && isHex(query) {
		block, err := c.GetExplorerBlock(query)
		if err == nil {
			result.Type = "block"
			result.Block = &block
			return result, nil
		}

		// Also try as transaction hash
		tx, err := c.GetExplorerTransaction(query)
		if err == nil {
			result.Type = "transaction"
			result.Transaction = &tx
			return result, nil
		}
	}

	// Try address (D prefix for mainnet)
	if len(query) >= 26 && len(query) <= 35 {
		if _, err := crypto.DecodeAddress(query); err == nil {
			addr, err := c.GetAddressBasic(query)
			if err == nil {
				result.Type = "address"
				result.Address = &addr
				return result, nil
			}
		}
	}

	result.Type = "not_found"
	result.Error = fmt.Sprintf("no results found for: %s", query)
	return result, nil
}

// ==========================================
// Helper Functions
// ==========================================

func (c *GoCoreClient) blockToSummary(block *types.Block, height uint32) BlockSummary {
	isPoS := len(block.Transactions) > 1 && block.Transactions[1].IsCoinStake()

	// Block reward computation.
	//
	// PoW: Transactions[0] is the coinbase carrying the full block subsidy as
	// outputs (no inputs to subtract). Sum coinbase outputs in satoshis.
	//
	// PoS: Transactions[0] is an EMPTY marker (no value), and the subsidy is
	// distributed inside the coinstake at Transactions[1] as
	// (sum outputs) - (sum input UTXO values). The coinstake spends the
	// staker's funding UTXO and creates outputs covering the original stake
	// PLUS the new reward (stake + masternode + dev).
	//
	// Input-value lookup uses storage.GetTransactionData (the funding-tx
	// lookup) NOT storage.GetUTXO (the unspent-set lookup). At the moment a
	// confirmed PoS block is summarized, the coinstake's input UTXOs are
	// ALREADY SPENT (consumed by the coinstake itself), so the unspent-set
	// lookup is fragile: CachedStorage invalidates spent UTXOs (see
	// internal/storage/cached_storage.go) so cache-miss + lookup-from-Pebble
	// can return nil even though the funding-tx record persists. Mirrors
	// blockToDetail at line 1062 exactly — the canonical pattern for
	// retrieving spent-input values.
	//
	// Per-input lookup failure (storage error, deleted/orphaned funding tx,
	// out-of-bounds index) leaves inputSat at 0 — defensive zero-fallback so
	// the row renders rather than failing the entire block summary.
	var reward float64
	if isPoS && len(block.Transactions) >= 2 {
		coinstake := block.Transactions[1]
		var outputSat, inputSat int64
		for _, out := range coinstake.Outputs {
			if out == nil {
				continue
			}
			outputSat += out.Value
		}
		for _, in := range coinstake.Inputs {
			if in == nil {
				continue
			}
			txData, err := c.storage.GetTransactionData(in.PreviousOutput.Hash)
			if err != nil || txData == nil || int(in.PreviousOutput.Index) >= len(txData.TxData.Outputs) {
				continue
			}
			inputSat += txData.TxData.Outputs[in.PreviousOutput.Index].Value
		}
		if outputSat > inputSat {
			reward = float64(outputSat-inputSat) / satoshisPerTWINS
		}
	} else if len(block.Transactions) > 0 {
		for _, out := range block.Transactions[0].Outputs {
			if out == nil {
				continue
			}
			reward += float64(out.Value) / satoshisPerTWINS
		}
	}

	return BlockSummary{
		Height:  int64(height),
		Hash:    block.Header.Hash().String(),
		Time:    time.Unix(int64(block.Header.Timestamp), 0),
		TxCount: len(block.Transactions),
		Size:    block.SerializeSize(),
		IsPoS:   isPoS,
		Reward:  reward,
	}
}

// findCoinstakeRewardIndexes locates the dev fund and masternode payment
// outputs in a PoS coinstake transaction.
//
// Canonical TWINS PoS coinstake layout (see
// internal/consensus/masternode_payments.go:1022):
//
//	With dev:    [empty(0), stake_return..., mn_payment, dev_payment]
//	Without dev: [empty(0), stake_return..., mn_payment]
//
// Dev output is located by matching scriptPubKey against devAddressScript
// (chainParams.DevAddress). This is more robust than hardcoded positional
// indexes because (a) stake_return may span multiple outputs shifting mn/dev
// positions, and (b) future layout changes won't silently misclassify outputs.
//
// Returns (devIdx, mnIdx) where -1 means "not present":
//   - If dev output found: dev at devIdx (typically len-1), MN at devIdx-1
//   - If no dev output AND >2 outputs: legacy 3-output layout, MN at outputs[2]
//   - Else: both -1
//
// devAddressScript may be nil/empty (testnet without DevAddress, or chainParams
// not wired) — function gracefully falls back to legacy positional MN parsing.
func findCoinstakeRewardIndexes(outputs []*types.TxOutput, devAddressScript []byte) (devIdx, mnIdx int) {
	devIdx = -1
	mnIdx = -1

	if len(devAddressScript) > 0 {
		// Search from the end; dev_payment is at outputs[len-1] in the canonical
		// layout. Lower bound is index 1 to skip the always-empty output[0].
		for i := len(outputs) - 1; i >= 1; i-- {
			if outputs[i] != nil && bytes.Equal(outputs[i].ScriptPubKey, devAddressScript) {
				devIdx = i
				break
			}
		}
	}

	switch {
	case devIdx >= 2 && devIdx-1 >= 2:
		// Dev found and there's room for MN between staker (idx 1) and dev.
		mnIdx = devIdx - 1
	case devIdx < 0 && len(outputs) > 2:
		// Legacy 3-output layout: [empty, stake_return, mn_payment].
		mnIdx = 2
	}

	return devIdx, mnIdx
}

// coinstakeBreakdown holds the computed reward economics for a PoS coinstake
// transaction. All amounts are in satoshis; convert to TWINS at the call site.
//
// Field semantics:
//   - StakeRewardSat = outputs[1].Value - totalInputSat (the +reward the staker received).
//     Zero when outputs has fewer than 2 entries or outputs[1] is nil.
//   - MasternodePaySat = outputs[mnIdx].Value, zero when no MN output is present.
//   - DevPaySat = outputs[devIdx].Value, zero when no dev fund output is present.
//   - StakerIdx is always 1 (the canonical stake-return slot) when outputs has >= 2
//     entries; -1 otherwise. MnIdx and DevIdx mirror findCoinstakeRewardIndexes.
type coinstakeBreakdown struct {
	StakeRewardSat   int64
	MasternodePaySat int64
	DevPaySat        int64
	// StakerIdx is the start of the stake-return run (always 1 by canonical layout),
	// or -1 when there are no stake-return outputs.
	StakerIdx int
	// StakerEndIdx is the end-exclusive index of the stake-return run. Stake-return
	// outputs occupy outputs[StakerIdx..StakerEndIdx). For non-split coinstakes
	// StakerEndIdx == StakerIdx+1; for stake-split coinstakes StakerEndIdx == StakerIdx+2.
	// Set to -1 when StakerIdx is -1.
	StakerEndIdx int
	MnIdx        int
	DevIdx       int
}

// computeCoinstakeBreakdown extracts the reward-economics breakdown for a
// PoS coinstake transaction. Pure function so unit tests can drive it
// directly with synthetic outputs.
//
// Inputs:
//   - outputs: the coinstake transaction's output slice (raw types.TxOutput pointers).
//   - totalInputSat: sum of input UTXO values in satoshis (already resolved by the caller).
//   - devAddressScript: chainParams.DevAddress scriptPubKey. May be nil/empty
//     on testnet without DevAddress or when chainParams is not wired.
//
// Stake-split coinstake handling: CreateCoinstakeTx (internal/wallet/staking.go:319-358)
// can produce stake returns split across outputs[1] AND outputs[2] when totalReward/2
// exceeds the stake split threshold. The canonical TWINS layout with all rewards is:
//
//	no split:  [empty, stake_return, mn_payment, dev_payment]                     (4 outputs)
//	split:     [empty, stake1, stake2, mn_payment, dev_payment]                   (5 outputs)
//
// Stake reward is computed as `sum(outputs[1..stakerEndIdx)) - totalInputSat` where
// stakerEndIdx is the lower of mnIdx and devIdx (when both are present), or whichever
// non-negative index is set, or len(outputs) when neither MN nor dev is present.
// Summing all stake-return outputs (rather than reading only outputs[1]) is load-bearing
// for split coinstakes — without this, the reward would be reported as a large negative
// number (only half the stake comes back at outputs[1] minus the full input).
//
// Returns the per-recipient amounts plus the indexes of stake/MN/dev outputs.
// StakerIdx is the START of the stake-return run (always 1 by canonical layout);
// callers that need to label individual outputs must iterate [StakerIdx..StakerEndIdx).
// Defensive against nil output pointers and short output slices.
//
// Known limitation: when devIdx < 0 AND len(outputs) > 2, findCoinstakeRewardIndexes
// falls back to mnIdx = 2 unconditionally (assumes [empty, stake_return, mn] legacy
// layout). On a split coinstake without dev fund (e.g. testnet without DevAddress with
// 4 outputs [empty, stake1, stake2, mn]), the helper returns mnIdx=2 which misidentifies
// stake2 as the MN payment. This is a pre-existing limitation in findCoinstakeRewardIndexes
// also affecting blockToDetail; not addressed in this helper. On mainnet (chainParams.DevAddress
// wired, dev fund always appended) the limitation does not manifest because devIdx is always >= 0.
func computeCoinstakeBreakdown(outputs []*types.TxOutput, totalInputSat int64, devAddressScript []byte) coinstakeBreakdown {
	result := coinstakeBreakdown{
		StakerIdx:    -1,
		StakerEndIdx: -1,
		MnIdx:        -1,
		DevIdx:       -1,
	}

	devIdx, mnIdx := findCoinstakeRewardIndexes(outputs, devAddressScript)
	result.MnIdx = mnIdx
	result.DevIdx = devIdx

	// Compute the end-exclusive index for stake-return outputs. Stake returns occupy
	// outputs[1..stakerEndIdx) — typically just outputs[1], but outputs[1] AND outputs[2]
	// on stake-split coinstakes. The end is bounded by whichever of MN / dev comes first
	// (since they're appended AFTER all stake returns), or len(outputs) when neither
	// is present.
	stakerEndIdx := len(outputs)
	switch {
	case mnIdx >= 0 && devIdx >= 0:
		if mnIdx < devIdx {
			stakerEndIdx = mnIdx
		} else {
			stakerEndIdx = devIdx
		}
	case mnIdx >= 0:
		stakerEndIdx = mnIdx
	case devIdx >= 0:
		stakerEndIdx = devIdx
	}

	// Sum all stake-return outputs (handles both single-stake and stake-split layouts).
	// Defensive nil guard on outputs[1] mirrors the pre-fix behavior: if the canonical
	// stake-return slot is nil (anomalous), report "no staker info" rather than guessing
	// downstream non-nil outputs are stake returns.
	if stakerEndIdx > 1 && outputs[1] != nil {
		result.StakerIdx = 1
		result.StakerEndIdx = stakerEndIdx
		var stakeTotal int64
		for i := 1; i < stakerEndIdx; i++ {
			if outputs[i] != nil {
				stakeTotal += outputs[i].Value
			}
		}
		result.StakeRewardSat = stakeTotal - totalInputSat
	}
	if mnIdx >= 0 && mnIdx < len(outputs) && outputs[mnIdx] != nil {
		result.MasternodePaySat = outputs[mnIdx].Value
	}
	if devIdx >= 0 && devIdx < len(outputs) && outputs[devIdx] != nil {
		result.DevPaySat = outputs[devIdx].Value
	}

	return result
}

// blockToDetail converts a stored block into the GUI BlockDetail view model.
//
// Locking: acquires c.mu.RLock() to snapshot c.chainParams. Callers must NOT
// hold c.mu in write mode when invoking this method. The snapshot pattern
// keeps the actual block-parsing work outside the lock to minimize contention.
func (c *GoCoreClient) blockToDetail(block *types.Block, height uint32) (BlockDetail, error) {
	chainHeight, _ := c.storage.GetChainHeight()
	confirmations := int(chainHeight) - int(height) + 1

	isPoS := len(block.Transactions) > 1 && block.Transactions[1].IsCoinStake()

	var stakeReward, masternodeReward, devReward, totalReward, stakeAmount float64
	var stakerAddr, masternodeAddr, devAddr, stakeModifierHex, proofHashHex string
	var stakeAge int64

	// Snapshot chainParams under read lock for layout-aware dev fund parsing.
	c.mu.RLock()
	chainParams := c.chainParams
	c.mu.RUnlock()

	if isPoS && len(block.Transactions) > 1 {
		coinstake := block.Transactions[1]

		// Calculate total inputs by looking up the original transactions
		// (UTXOs are already spent, so we need to fetch from the source tx).
		// Also derive stake_amount + stake_age from the FIRST input's funding
		// UTXO (the staked output): stake_amount = funding UTXO value,
		// stake_age = current block timestamp - funding block timestamp.
		// Any lookup failure leaves stake_amount / stake_age at 0; never fails
		// the whole render because PoS metadata is unavailable.
		var totalInputs int64
		for i, in := range coinstake.Inputs {
			// Get the transaction that contains this output
			txData, err := c.storage.GetTransactionData(in.PreviousOutput.Hash)
			if err != nil || txData == nil || int(in.PreviousOutput.Index) >= len(txData.TxData.Outputs) {
				continue
			}
			prevOutValue := txData.TxData.Outputs[in.PreviousOutput.Index].Value
			totalInputs += prevOutValue
			if i == 0 {
				stakeAmount = float64(prevOutValue) / satoshisPerTWINS
				// Funding-block timestamp lookup. txData.Height is the height
				// at which the funding tx was confirmed. Parent block read
				// failure leaves stake_age at 0.
				parentBlock, perr := c.storage.GetBlockByHeight(txData.Height)
				if perr == nil && parentBlock != nil {
					stakeAge = int64(block.Header.Timestamp) - int64(parentBlock.Header.Timestamp)
					if stakeAge < 0 {
						stakeAge = 0
					}
				}
			}
		}

		// Calculate total outputs. coinstake.Outputs is []*TxOutput; nil entries
		// are skipped defensively (malformed data should not panic the GUI).
		var totalOutputs int64
		for _, out := range coinstake.Outputs {
			if out != nil {
				totalOutputs += out.Value
			}
		}

		// Total reward = newly created coins = totalOutputs - totalInputs.
		// For well-formed coinstakes this equals stakeReward + masternodeReward
		// + devReward, since stake_return = stakeInput + stakeReward and all other
		// outputs are pure additions. We compute via IO delta (rather than
		// summing the component rewards) so the value remains correct even when
		// the coinstake layout has additional fee/refund/governance outputs that
		// are not yet surfaced as named fields.
		totalReward = float64(totalOutputs-totalInputs) / satoshisPerTWINS

		// Locate dev fund and masternode payment outputs using layout-aware
		// parsing (matches scriptPubKey against chainParams.DevAddress rather
		// than hardcoded positional indexes). See findCoinstakeRewardIndexes.
		var devAddressScript []byte
		if chainParams != nil {
			devAddressScript = chainParams.DevAddress
		}
		devOutputIdx, mnOutputIdx := findCoinstakeRewardIndexes(coinstake.Outputs, devAddressScript)

		// All three slot reads below check the pointer non-nil. The dev slot
		// is already guaranteed non-nil by findCoinstakeRewardIndexes (it
		// skips nil entries during the script-match scan), but the staker
		// (outputs[1]) and the MN slot (devIdx-1 or legacy outputs[2]) are
		// returned without nil-check. A malformed coinstake with a nil entry
		// at one of those positions would otherwise panic the GUI core when
		// opening the block detail view (codex 2026-05-27 review W1).
		if devOutputIdx >= 0 && coinstake.Outputs[devOutputIdx] != nil {
			devAddr = c.extractAddressFromScript(coinstake.Outputs[devOutputIdx].ScriptPubKey)
			devReward = float64(coinstake.Outputs[devOutputIdx].Value) / satoshisPerTWINS
		}

		// Staker reward: output 1 is stake_return (= original stake + stake reward).
		// stakeReward = output[1].Value - totalInputs.
		if len(coinstake.Outputs) > 1 && coinstake.Outputs[1] != nil {
			stakerAddr = c.extractAddressFromScript(coinstake.Outputs[1].ScriptPubKey)
			stakeReward = float64(coinstake.Outputs[1].Value-totalInputs) / satoshisPerTWINS
		}

		if mnOutputIdx >= 0 && coinstake.Outputs[mnOutputIdx] != nil {
			masternodeAddr = c.extractAddressFromScript(coinstake.Outputs[mnOutputIdx].ScriptPubKey)
			masternodeReward = float64(coinstake.Outputs[mnOutputIdx].Value) / satoshisPerTWINS
		}

		// PoS internals: stake modifier + hashProofOfStake (a.k.a. kernel hash).
		// Both are already persisted in storage by the consensus engine during
		// block connect (see internal/consensus/pos.go:1247-1260 for stake
		// modifier and the StoreBlockPoSMetadata wiring for proof hash). Defensive
		// zero-on-failure: any storage error or not-found leaves the local hex
		// string at "" and falls through; the GUI renders "—" placeholder.
		blockHash := block.Header.Hash()
		if modifier, merr := c.storage.GetStakeModifier(blockHash); merr == nil {
			stakeModifierHex = fmt.Sprintf("0x%016x", modifier)
		}
		if _, proofHash, perr := c.storage.GetBlockPoSMetadata(blockHash); perr == nil && !proofHash.IsZero() {
			proofHashHex = proofHash.String()
		}
	}

	txids := make([]string, len(block.Transactions))
	for i, tx := range block.Transactions {
		txids[i] = tx.Hash().String()
	}

	// Get previous/next block hashes
	var prevHash, nextHash string
	prevHash = block.Header.PrevBlockHash.String()

	if height < chainHeight {
		nextBlock, err := c.storage.GetBlockByHeight(height + 1)
		if err == nil {
			nextHash = nextBlock.Header.Hash().String()
		}
	}

	return BlockDetail{
		Block: Block{
			Hash:              block.Header.Hash().String(),
			Height:            int64(height),
			Confirmations:     confirmations,
			Size:              block.SerializeSize(),
			Time:              time.Unix(int64(block.Header.Timestamp), 0),
			PreviousBlockHash: prevHash,
			NextBlockHash:     nextHash,
			Difficulty:        float64(block.Header.Bits), // Simplified
			Bits:              fmt.Sprintf("%08x", block.Header.Bits),
			Nonce:             block.Header.Nonce,
			MerkleRoot:        block.Header.MerkleRoot.String(),
		},
		TxIDs:             txids,
		IsPoS:             isPoS,
		StakeReward:       stakeReward,
		MasternodeReward:  masternodeReward,
		DevReward:         devReward,
		StakerAddress:     stakerAddr,
		MasternodeAddress: masternodeAddr,
		DevAddress:        devAddr,
		TotalReward:       totalReward,
		StakeAmount:       stakeAmount,
		StakeAge:          stakeAge,
		StakeModifier:     stakeModifierHex,
		ProofHash:         proofHashHex,
	}, nil
}

// extractOpReturnData parses an OP_RETURN (nulldata) scriptPubKey and returns
// the hex-encoded payload and a printable-ASCII rendering of it. The ASCII
// string is returned ONLY when every byte of the payload falls in the
// printable ASCII range [0x20, 0x7e]; otherwise it is empty and the frontend
// should fall back to the hex string.
//
// Supports both direct pushes (opcode bytes 0x01..0x4b) and the explicit
// OP_PUSHDATA1 / OP_PUSHDATA2 push opcodes. OP_PUSHDATA4 is not handled —
// the legacy protocol caps OP_RETURN payloads at 83 bytes
// (MAX_OP_RETURN_RELAY) so a 4-byte length field is never required, and
// rejecting it defensively prevents potential parser footguns.
//
// Returns ("", "") on parse failure (script too short, truncated push, length
// declares more bytes than the script carries). Note: `OP_RETURN OP_0`
// (`0x6a 0x00`, zero-byte payload) also returns ("", "") — behaviourally
// identical to "no payload extracted". Callers that need to distinguish
// "OP_RETURN with zero-byte payload" from "OP_RETURN parse failure" should
// branch on `scriptType == "nulldata"` first.
func extractOpReturnData(script []byte) (hexStr, ascii string) {
	if len(script) < 2 || script[0] != pkgscript.OP_RETURN {
		return "", ""
	}

	pos := 1
	pushOp := script[pos]
	pos++

	var payloadLen int
	switch {
	case pushOp >= 0x01 && pushOp <= 0x4b:
		// Direct push: opcode value is the byte count.
		payloadLen = int(pushOp)
	case pushOp == pkgscript.OP_PUSHDATA1:
		if pos+1 > len(script) {
			return "", ""
		}
		payloadLen = int(script[pos])
		pos++
	case pushOp == pkgscript.OP_PUSHDATA2:
		if pos+2 > len(script) {
			return "", ""
		}
		payloadLen = int(script[pos]) | int(script[pos+1])<<8
		pos += 2
	default:
		// Includes OP_0 (empty push), OP_PUSHDATA4 (rejected — see doc above),
		// and anything else that does not push raw data.
		return "", ""
	}

	if pos+payloadLen > len(script) {
		// Declared length exceeds script bytes — truncated push.
		return "", ""
	}

	payload := script[pos : pos+payloadLen]
	hexStr = hex.EncodeToString(payload)

	allPrintable := true
	for _, b := range payload {
		if b < 0x20 || b > 0x7e {
			allPrintable = false
			break
		}
	}
	if allPrintable {
		ascii = string(payload)
	}
	return hexStr, ascii
}

// extractMultisigAddresses extracts the N pubkey-derived addresses and the M
// required-signatures count from a bare multisig scriptPubKey, re-encoding
// each address with the active network prefix. pkg/script.ExtractMultisig
// hardcodes the mainnet ID; this helper takes the returned addresses, pulls
// each one's hash160, and rebuilds via crypto.NewAddressFromHash with the
// correct netID for the current network.
//
// Returns (nil, 0) on any parse failure or when the script is not a valid
// multisig pattern. Mirrors the network-aware encoding pattern used by
// extractAddressFromScript.
func extractMultisigAddresses(script []byte, networkName string) ([]string, int) {
	addrs, m, err := pkgscript.ExtractMultisig(script)
	if err != nil {
		return nil, 0
	}
	netID := crypto.GetPubKeyHashNetworkID(networkName)
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if a == nil {
			continue
		}
		rebuilt, rerr := crypto.NewAddressFromHash(a.Hash160(), netID)
		if rerr != nil {
			// Should not happen — Hash160() always returns 20 bytes — but
			// be defensive so a single bad pubkey does not nil out the
			// whole list.
			continue
		}
		out = append(out, rebuilt.String())
	}
	if len(out) == 0 {
		return nil, 0
	}
	return out, m
}

// outputRoleContext bundles the inputs that assignOutputRole needs into one
// struct so the priority-ordered switch in the implementation stays readable
// and the call site does not collapse into a 9-arg function signature.
type outputRoleContext struct {
	tx               *types.Transaction
	outputIndex      int
	output           *types.TxOutput
	scriptType       string
	legacyLabel      string
	outputAddress    string
	inputAddresses   map[string]struct{}
	blockHeight      int64
	isPoSBlockMarker bool
	isMine           bool
}

// assignOutputRole returns the semantic-role enum value for a single output
// in priority order. The priority is documented inline so a future maintainer
// can reason about precedence without consulting the research spec:
//
//  1. PoS Block Marker (vout[0] of an empty-coinbase paired with a coinstake)
//  2. Coinstake roles (marker / stake_return / mn_payment / dev_fund) — keyed
//     off legacyLabel which the caller already populated via
//     computeCoinstakeBreakdown
//  3. Coinbase roles (premine for block 1, mining_reward otherwise)
//  4. Script-type roles for non-payment scripts (nulldata / multisig / nonstandard)
//  5. Standard payment script + wallet ownership rules
//     (change if mine AND addr ∈ inputAddresses; self_send if mine AND not in
//     inputAddresses; external_payment otherwise)
//
// Pure function so unit tests can drive it directly with synthetic contexts.
func assignOutputRole(ctx outputRoleContext) string {
	if ctx.isPoSBlockMarker {
		return OutputRoleBlockMarker
	}

	if ctx.tx != nil && ctx.tx.IsCoinStake() {
		if ctx.outputIndex == 0 && ctx.output != nil && ctx.output.Value == 0 && len(ctx.output.ScriptPubKey) == 0 {
			return OutputRoleBlockMarker
		}
		switch ctx.legacyLabel {
		case "Stake Return":
			return OutputRoleStakeReturn
		case "Masternode Payment":
			return OutputRoleMasternodePayment
		case "Dev Fund":
			return OutputRoleDevFund
		}
		// Defensive fallback for outputs of a coinstake that were not
		// classified by computeCoinstakeBreakdown (e.g. testnet without
		// DevAddress configured + a stake-split layout where the helper
		// misidentifies stake2 as MN). Returning OutputRoleNonstandard
		// rather than falling through prevents the standard-payment branch
		// below from labelling an unknown coinstake output as `change` or
		// `external_payment` based on the staker's input addresses — both
		// of which are semantically wrong for a coinstake output.
		return OutputRoleNonstandard
	}

	if ctx.tx != nil && ctx.tx.IsCoinbase() {
		if ctx.blockHeight == 1 {
			return OutputRolePremine
		}
		return OutputRoleMiningReward
	}

	switch ctx.scriptType {
	case "nulldata":
		return OutputRoleDataCarrier
	case "multisig":
		return OutputRoleMultisig
	case "nonstandard":
		return OutputRoleNonstandard
	}

	// Standard payment script (pubkey / pubkeyhash / scripthash).
	if ctx.isMine {
		if ctx.outputAddress != "" {
			if _, ok := ctx.inputAddresses[ctx.outputAddress]; ok {
				return OutputRoleChange
			}
		}
		return OutputRoleSelfSend
	}
	return OutputRoleExternalPayment
}

// isPoSEmptyCoinbase returns true when the given transaction is the
// protocol-mandated empty coinbase that pairs with a coinstake at vtx[1] of
// every PoS block. Detection requires the containing block's transaction list
// (the "empty + 1 output" shape alone is ambiguous with a degenerate PoW
// coinbase), so this helper performs ONE storage.GetBlock lookup using the
// already-resolved blockHash. The lookup fires only for the rare empty-output
// pattern; standard tx fetches never trigger it.
//
// Returns false on storage error, missing block, or any structural mismatch.
// Silent degrade: a coinbase that should be marked block_marker but cannot
// be confirmed (storage error) falls through to OutputRoleNonstandard, which
// is the pre-fix behavior — the new flag is purely additive.
func (c *GoCoreClient) isPoSEmptyCoinbase(tx *types.Transaction, blockHash types.Hash) bool {
	if tx == nil || !tx.IsCoinbase() {
		return false
	}
	if len(tx.Outputs) != 1 {
		return false
	}
	if tx.Outputs[0] == nil || tx.Outputs[0].Value != 0 || len(tx.Outputs[0].ScriptPubKey) != 0 {
		return false
	}
	if blockHash.IsZero() {
		return false
	}
	block, err := c.storage.GetBlock(blockHash)
	if err != nil || block == nil {
		return false
	}
	return len(block.Transactions) >= 2 && block.Transactions[1].IsCoinStake()
}

// computeOutputSpentStatus classifies a TxOutput's is_spent flag from the
// result of a Storage.GetUTXO(outpoint) call. Pure helper — takes no storage
// handle so it can be tested without mocking the storage layer.
//
// Classification logic:
//
//  1. utxo != nil && SpendingHeight > 0 → true  (consumed by another tx, not
//     yet pruned by CachedStorage / chain depth).
//  2. utxo != nil && SpendingHeight == 0 → false (unspent).
//  3. lookupErr is a not-found error (generic NOT_FOUND / TX_NOT_FOUND /
//     HEIGHT_NOT_FOUND recognized by storage.IsNotFoundError, OR the
//     storage-specific UTXO_NOT_FOUND code emitted by BinaryStorage.GetUTXO
//     which IsNotFoundError does not yet recognize):
//     a. scriptType == "nulldata" → false  (OP_RETURN is unspendable by
//     protocol; never was a UTXO entry).
//     b. scriptType == "nonstandard" && value == 0 → false  (block_marker /
//     coinstake marker; never was a UTXO entry).
//     c. otherwise → true  (standard script absent from UTXO set = consumed
//     and pruned at depth).
//  4. lookupErr is a non-NotFound error → false  (defensive; transient
//     storage errors should not panic the GUI core. Caller logs).
func computeOutputSpentStatus(scriptType string, value int64, utxo *types.UTXO, lookupErr error) bool {
	if lookupErr == nil && utxo != nil {
		return utxo.SpendingHeight > 0
	}
	// storage.IsNotFoundError (per its 2026-04 widening at interface.go:444)
	// covers NOT_FOUND / TX_NOT_FOUND / HEIGHT_NOT_FOUND but NOT the
	// UTXO_NOT_FOUND code BinaryStorage.GetUTXO emits on a missing UTXO entry
	// (interface_impl.go:1348). Recognize both shapes so the script-type-aware
	// branches below fire for both code paths.
	isNotFound := storage.IsNotFoundError(lookupErr)
	if !isNotFound {
		if se, ok := lookupErr.(*storage.StorageError); ok && se.Code == "UTXO_NOT_FOUND" {
			isNotFound = true
		}
	}
	if !isNotFound {
		return false
	}
	if scriptType == "nulldata" {
		return false
	}
	if scriptType == "nonstandard" && value == 0 {
		return false
	}
	return true
}

func (c *GoCoreClient) txToExplorerTx(txData *storage.TransactionData) (ExplorerTransaction, error) {
	tx := txData.TxData
	chainHeight, _ := c.storage.GetChainHeight()
	confirmations := int(chainHeight) - int(txData.Height) + 1

	// Get block for timestamp
	block, _ := c.storage.GetBlockByHeight(txData.Height)
	var txTime time.Time
	if block != nil {
		txTime = time.Unix(int64(block.Header.Timestamp), 0)
	}

	// Snapshot wallet + network once at the top; mirrors the pattern in
	// extractRecipientAddressesFromTx / blockToDetail. The wallet snapshot
	// drives the IsMine flags below; nil snapshot (explorer-only context)
	// collapses every is_mine / is_change check to false.
	c.mu.RLock()
	w := c.wallet
	networkName := c.network
	chainParams := c.chainParams
	c.mu.RUnlock()

	isCoinbase := tx.IsCoinbase()
	isCoinstake := tx.IsCoinStake()

	// Pass 1 — Inputs. Existing prevout resolution preserved; additionally
	// compute IsMine via wallet.IsOurScript on the resolved prevout
	// scriptPubKey, and collect the resolved input addresses into a set the
	// output pass will use for change detection.
	inputs := make([]TxInput, len(tx.Inputs))
	inputAddresses := make(map[string]struct{}, len(tx.Inputs))
	var totalInput int64

	for i, in := range tx.Inputs {
		// For coinbase, the first input is special (no previous output)
		isCoinbaseInput := isCoinbase && i == 0
		inputs[i] = TxInput{
			TxID:              in.PreviousOutput.Hash.String(),
			Vout:              in.PreviousOutput.Index,
			IsCoinbase:        isCoinbaseInput,
			IsCoinstakeKernel: isCoinstake && i == 0,
		}

		if !isCoinbaseInput {
			// Get previous output from the source transaction
			// (UTXOs may already be spent, so we need to fetch from the source tx)
			prevTxData, err := c.storage.GetTransactionData(in.PreviousOutput.Hash)
			if err == nil && prevTxData != nil && int(in.PreviousOutput.Index) < len(prevTxData.TxData.Outputs) {
				prevOutput := prevTxData.TxData.Outputs[in.PreviousOutput.Index]
				inputs[i].Amount = float64(prevOutput.Value) / satoshisPerTWINS
				addr := c.extractAddressFromScript(prevOutput.ScriptPubKey)
				inputs[i].Address = addr
				totalInput += prevOutput.Value
				if addr != "" {
					inputAddresses[addr] = struct{}{}
				}
				if w != nil {
					if _, mine := w.IsOurScript(prevOutput.ScriptPubKey); mine {
						inputs[i].IsMine = true
					}
				}
			}
		}
	}

	// Detect the PoS Block Marker shape once outside the per-output loop.
	// The lookup fires only for the rare empty-coinbase pattern; standard
	// fetches skip the storage round-trip.
	posBlockMarker := c.isPoSEmptyCoinbase(tx, txData.BlockHash)

	outputs := make([]TxOutput, len(tx.Outputs))
	var totalOutput int64

	txHash := tx.Hash()
	for i, out := range tx.Outputs {
		addr := c.extractAddressFromScript(out.ScriptPubKey)
		scriptType := c.getScriptType(out.ScriptPubKey)
		// Resolve spent-flag from the UTXO set. Per-output Pebble point-lookup
		// (CachedStorage in front) is sub-ms; cost analysis in task
		// `l-tx-explorer-spent-flag` confirmed hot-path safety for both the
		// detail view and tx-search call sites. Non-NotFound storage errors are
		// unexpected here and logged at debug level; the helper returns false
		// defensively in that case so the row still renders.
		outpoint := types.Outpoint{Hash: txHash, Index: uint32(i)}
		utxo, lookupErr := c.storage.GetUTXO(outpoint)
		if lookupErr != nil && !storage.IsNotFoundError(lookupErr) {
			if se, ok := lookupErr.(*storage.StorageError); !ok || se.Code != "UTXO_NOT_FOUND" {
				log.WithError(lookupErr).WithField("outpoint", outpoint.String()).
					Debug("GetUTXO failed during spent-flag derivation")
			}
		}
		isSpent := computeOutputSpentStatus(scriptType, out.Value, utxo, lookupErr)
		outputs[i] = TxOutput{
			Index:      uint32(i),
			Address:    addr,
			Amount:     float64(out.Value) / satoshisPerTWINS,
			ScriptType: scriptType,
			IsSpent:    isSpent,
		}
		totalOutput += out.Value

		// Multisig: surface all N keys + M required. Address keeps the first
		// key for back-compat with the current frontend.
		if scriptType == "multisig" {
			addrs, m := extractMultisigAddresses(out.ScriptPubKey, networkName)
			if len(addrs) > 0 {
				outputs[i].Addresses = addrs
				outputs[i].RequiredSigs = m
				if outputs[i].Address == "" {
					outputs[i].Address = addrs[0]
				}
			}
		}

		// OP_RETURN: decode payload.
		if scriptType == "nulldata" {
			hexStr, ascii := extractOpReturnData(out.ScriptPubKey)
			outputs[i].DataHex = hexStr
			outputs[i].DataASCII = ascii
		}

		// IsMine via wallet ownership.
		if w != nil {
			if _, mine := w.IsOurScript(out.ScriptPubKey); mine {
				outputs[i].IsMine = true
			}
		}

		// Dust: value-bearing standard outputs below the threshold. Nulldata
		// is excluded so OP_RETURN payloads are not visually flagged as dust;
		// zero-value outputs (the canonical marker shape) are excluded by the
		// `> 0` guard.
		if out.Value > 0 && out.Value < dustThresholdSatoshis && scriptType != "nulldata" {
			outputs[i].IsDust = true
		}
	}

	fee := totalInput - totalOutput
	if fee < 0 {
		fee = 0 // Coinbase/coinstake
	}

	// Coinstake reward breakdown: mirrors blockToDetail's logic so the Transaction
	// Detail view can show the same Stake / Masternode / Dev Fund split that the
	// Block Detail view already shows. The block-builder layout is canonical:
	//     [outputs[0]=empty-marker, outputs[1..]=stake_return(s), outputs[len-2]=mn, outputs[len-1]=dev]
	// findCoinstakeRewardIndexes is the single source of truth for locating
	// the MN and dev fund slots via scriptPubKey match against chainParams.DevAddress.
	// The legacy Label strings are kept for back-compat; the new Role field
	// is the machine-readable replacement assigned per-output below.
	var stakeReward, masternodeReward, devReward float64
	if isCoinstake {
		var devAddressScript []byte
		if chainParams != nil {
			devAddressScript = chainParams.DevAddress
		}

		breakdown := computeCoinstakeBreakdown(tx.Outputs, totalInput, devAddressScript)

		// Label every output in [StakerIdx..StakerEndIdx) so stake-split coinstakes
		// (two stake-return outputs to the same P2PK script, see
		// internal/wallet/staking.go:331-358) get the GUI badge on BOTH outputs.
		if breakdown.StakerIdx >= 0 {
			stakeReward = float64(breakdown.StakeRewardSat) / satoshisPerTWINS
			for i := breakdown.StakerIdx; i < breakdown.StakerEndIdx && i < len(outputs); i++ {
				if tx.Outputs[i] != nil {
					outputs[i].Label = "Stake Return"
				}
			}
		}
		if breakdown.MnIdx >= 0 && breakdown.MnIdx < len(outputs) && tx.Outputs[breakdown.MnIdx] != nil {
			masternodeReward = float64(breakdown.MasternodePaySat) / satoshisPerTWINS
			outputs[breakdown.MnIdx].Label = "Masternode Payment"
		}
		if breakdown.DevIdx >= 0 && breakdown.DevIdx < len(outputs) && tx.Outputs[breakdown.DevIdx] != nil {
			devReward = float64(breakdown.DevPaySat) / satoshisPerTWINS
			outputs[breakdown.DevIdx].Label = "Dev Fund"
		}
		if len(outputs) > 0 {
			outputs[0].Label = "Coinstake Marker"
		}
	}

	// Pass 2b — assign Role + IsChange per output, now that legacy Labels
	// are populated. Role is the machine-readable replacement consumed by
	// the new display matrix; IsChange is surfaced explicitly so the
	// frontend does not have to re-implement the addr-in-inputs rule.
	for i, out := range tx.Outputs {
		// Only outputs[0] participates in the posBlockMarker check; later
		// outputs of an empty-coinbase tx never exist (shape requires len==1),
		// but the gate is kept explicit so the role of the single output is
		// computed correctly without depending on loop position semantics.
		marker := posBlockMarker && i == 0
		outputs[i].Role = assignOutputRole(outputRoleContext{
			tx:               tx,
			outputIndex:      i,
			output:           out,
			scriptType:       outputs[i].ScriptType,
			legacyLabel:      outputs[i].Label,
			outputAddress:    outputs[i].Address,
			inputAddresses:   inputAddresses,
			blockHeight:      int64(txData.Height),
			isPoSBlockMarker: marker,
			isMine:           outputs[i].IsMine,
		})
		outputs[i].IsChange = outputs[i].Role == OutputRoleChange
	}

	return ExplorerTransaction{
		TxID:             tx.Hash().String(),
		BlockHash:        txData.BlockHash.String(),
		BlockHeight:      int64(txData.Height),
		Confirmations:    confirmations,
		Time:             txTime,
		Size:             tx.SerializeSize(),
		Fee:              float64(fee) / satoshisPerTWINS,
		IsCoinbase:       isCoinbase,
		IsCoinStake:      tx.IsCoinStake(),
		StakeReward:      stakeReward,
		MasternodeReward: masternodeReward,
		DevReward:        devReward,
		Inputs:           inputs,
		Outputs:          outputs,
		TotalInput:       float64(totalInput) / satoshisPerTWINS,
		TotalOutput:      float64(totalOutput) / satoshisPerTWINS,
	}, nil
}

func (c *GoCoreClient) extractAddressFromScript(scriptBytes []byte) string {
	scriptType, scriptHash := binary.AnalyzeScript(scriptBytes)

	// Snapshot network name under RLock — same pattern as the existing
	// `SetNetwork` consumer in `extractRecipientAddressesFromTx`. Empty
	// string falls back to mainnet via crypto.GetPubKeyHashNetworkID /
	// GetScriptHashNetworkID semantics, preserving backwards-compatible
	// behavior when SetNetwork was never wired (gemini code-review round 2:
	// the prior hardcoded mainnet IDs broke explorer address matching on
	// testnet / regtest).
	c.mu.RLock()
	networkName := c.network
	c.mu.RUnlock()

	var netID byte
	switch scriptType {
	case binary.ScriptTypeP2PKH, binary.ScriptTypeP2PK:
		netID = crypto.GetPubKeyHashNetworkID(networkName)
	case binary.ScriptTypeP2SH:
		netID = crypto.GetScriptHashNetworkID(networkName)
	default:
		return ""
	}

	addr, err := crypto.NewAddressFromHash(scriptHash[:], netID)
	if err != nil {
		return ""
	}
	return addr.String()
}

// getScriptType returns the script type token expected by the frontend
// (pubkey / pubkeyhash / scripthash / multisig / nulldata / nonstandard).
//
// Delegates to pkg/script.GetScriptType which is the canonical implementation
// and also recognizes P2PK (commonly used by PoS stakers for stake-return outputs)
// and multisig — both of which the prior inline byte-pattern matcher missed and
// would have returned as "nonstandard".
func (c *GoCoreClient) getScriptType(script []byte) string {
	return pkgscript.GetScriptType(script).String()
}

func isHex(s string) bool {
	matched, _ := regexp.MatchString("^[0-9a-fA-F]+$", s)
	return matched
}

// ==========================================
// Stub implementations for other methods
// These return errors until fully implemented
// ==========================================

func (c *GoCoreClient) GetBalance() (Balance, error) {
	c.mu.RLock()
	w := c.wallet
	c.mu.RUnlock()

	if w == nil {
		return Balance{}, fmt.Errorf("wallet not initialized")
	}

	// Get balance from wallet (values are in satoshis)
	walletBalance := w.GetBalance()
	if walletBalance == nil {
		return Balance{}, fmt.Errorf("failed to get wallet balance")
	}

	// Convert satoshis to TWINS (1 TWINS = 100,000,000 satoshis)

	confirmed := float64(walletBalance.Confirmed) / satoshisPerTWINS
	unconfirmed := float64(walletBalance.Unconfirmed) / satoshisPerTWINS
	immature := float64(walletBalance.Immature) / satoshisPerTWINS

	// Calculate totals
	// Available = Confirmed (spendable balance)
	// Pending = Unconfirmed
	// Total = Available + Pending + Immature
	total := confirmed + unconfirmed + immature

	// Calculate locked balance from masternode UTXOs using atomic wallet method
	// This ensures consistent results by holding the wallet lock during iteration
	lockedSatoshis, _ := w.GetLockedCollateralInfo()
	locked := float64(lockedSatoshis) / satoshisPerTWINS

	// Available = Confirmed balance minus locked
	// This is what the user can actually spend
	available := confirmed - locked

	// Spendable = Available (after subtracting locked)
	spendable := available

	return Balance{
		Total:     total,
		Available: available,
		Spendable: spendable,
		Pending:   unconfirmed,
		Immature:  immature,
		Locked:    locked,
	}, nil
}

func (c *GoCoreClient) GetNewAddress(label string) (string, error) {
	// Wallet not yet implemented - requires legacy.CMasterKey types
	return "", fmt.Errorf("wallet not implemented")
}

func (c *GoCoreClient) SendToAddress(address string, amount float64, comment string) (string, error) {
	c.mu.RLock()
	w := c.wallet
	c.mu.RUnlock()

	if w == nil {
		return "", fmt.Errorf("wallet not initialized")
	}

	// Convert amount from TWINS to satoshis (1 TWINS = 100,000,000 satoshis)
	// Use math.Round to avoid floating-point precision errors

	amountSatoshis := int64(math.Round(amount * satoshisPerTWINS))

	if amountSatoshis <= 0 {
		return "", fmt.Errorf("invalid amount: must be positive")
	}

	// Call wallet's SendToAddress
	// subtractFee=false means fee is added on top of the amount
	txid, err := w.SendToAddress(address, amountSatoshis, comment, false)
	if err != nil {
		return "", fmt.Errorf("send failed: %w", err)
	}

	return txid, nil
}

// SendOptions contains options for advanced transaction sending
type SendOptions struct {
	// SelectedUTXOs are the specific UTXOs to use (coin control)
	// Format: ["txid:vout", ...]
	SelectedUTXOs []string `json:"selectedUtxos"`

	// ChangeAddress overrides the default change address
	ChangeAddress string `json:"changeAddress"`

	// SplitCount splits each output into multiple UTXOs
	SplitCount int `json:"splitCount"`

	// FeeRate is the user-selected fee rate in TWINS/kB
	// If 0 or omitted, uses wallet default fee rate
	FeeRate float64 `json:"feeRate"`
}

// SendToAddressWithOptions sends TWINS with advanced options (coin control, custom change, split)
func (c *GoCoreClient) SendToAddressWithOptions(address string, amount float64, comment string, opts *SendOptions) (string, error) {
	c.mu.RLock()
	w := c.wallet
	c.mu.RUnlock()

	if w == nil {
		return "", fmt.Errorf("wallet not initialized")
	}

	// Convert amount from TWINS to satoshis
	// Use math.Round to avoid floating-point precision errors

	amountSatoshis := int64(math.Round(amount * satoshisPerTWINS))

	if amountSatoshis <= 0 {
		return "", fmt.Errorf("invalid amount: must be positive")
	}

	// Build wallet options
	var walletOpts *wallet.SendOptions
	if opts != nil {
		walletOpts = &wallet.SendOptions{
			ChangeAddress: opts.ChangeAddress,
			SplitCount:    opts.SplitCount,
		}

		// Convert fee rate from TWINS/kB to satoshis/kB
		if opts.FeeRate > 0 {
			walletOpts.FeeRate = int64(math.Round(opts.FeeRate * satoshisPerTWINS))
		}

		// Parse selected UTXOs from strings "txid:vout"
		if len(opts.SelectedUTXOs) > 0 {
			outpoints, err := parseUTXOStrings(opts.SelectedUTXOs)
			if err != nil {
				return "", err
			}
			walletOpts.SelectedUTXOs = outpoints
		}
	}

	// Create recipients map
	recipients := map[string]int64{
		address: amountSatoshis,
	}

	// Call wallet's SendManyWithOptions
	txid, err := w.SendManyWithOptions(recipients, comment, walletOpts)
	if err != nil {
		return "", fmt.Errorf("send failed: %w", err)
	}

	return txid, nil
}

// splitUTXOString splits a UTXO string by the last colon
func splitUTXOString(s string) []string {
	lastColon := -1
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			lastColon = i
			break
		}
	}
	if lastColon == -1 {
		return []string{s}
	}
	return []string{s[:lastColon], s[lastColon+1:]}
}

// parseUTXOStrings parses a slice of UTXO strings in "txid:vout" format
// into a slice of types.Outpoint. Returns an error if any UTXO is malformed.
func parseUTXOStrings(utxoStrings []string) ([]types.Outpoint, error) {
	if len(utxoStrings) == 0 {
		return nil, nil
	}

	outpoints := make([]types.Outpoint, 0, len(utxoStrings))
	for _, utxoStr := range utxoStrings {
		var txid string
		var vout uint32

		// Parse "txid:vout" format using splitUTXOString (handles txids with colons)
		parts := splitUTXOString(utxoStr)
		if len(parts) != 2 || parts[1] == "" {
			return nil, fmt.Errorf("invalid UTXO format: %s (expected txid:vout)", utxoStr)
		}

		txid = parts[0]
		var voutInt int
		_, err := fmt.Sscanf(parts[1], "%d", &voutInt)
		if err != nil {
			return nil, fmt.Errorf("invalid UTXO vout: %s", parts[1])
		}
		vout = uint32(voutInt)

		hash, err := types.NewHashFromString(txid)
		if err != nil {
			return nil, fmt.Errorf("invalid UTXO txid: %s", txid)
		}

		outpoints = append(outpoints, types.Outpoint{
			Hash:  hash,
			Index: vout,
		})
	}

	return outpoints, nil
}

// SendMany sends coins to multiple recipients in a single transaction.
// recipients: map of address → amount in TWINS
// Supports all options from SendOptions (coin control, custom change, UTXO split).
func (c *GoCoreClient) SendMany(recipients map[string]float64, comment string, opts *SendOptions) (string, error) {
	c.mu.RLock()
	w := c.wallet
	c.mu.RUnlock()

	if w == nil {
		return "", fmt.Errorf("wallet not set - call SetWallet first")
	}

	if len(recipients) == 0 {
		return "", fmt.Errorf("no recipients specified")
	}

	// Convert recipients to satoshis
	recipientsSatoshis := make(map[string]int64, len(recipients))
	for addr, amount := range recipients {
		if amount <= 0 {
			return "", fmt.Errorf("invalid amount for address %s: must be positive", addr)
		}
		// Convert TWINS to satoshis (1 TWINS = 100,000,000 satoshis)
		// Use math.Round to avoid floating-point precision errors
		amountSatoshis := int64(math.Round(amount * satoshisPerTWINS))
		recipientsSatoshis[addr] = amountSatoshis
	}

	// Build wallet options
	walletOpts := &wallet.SendOptions{}

	if opts != nil {
		walletOpts.ChangeAddress = opts.ChangeAddress
		walletOpts.SplitCount = opts.SplitCount

		// Convert fee rate from TWINS/kB to satoshis/kB
		if opts.FeeRate > 0 {
			walletOpts.FeeRate = int64(math.Round(opts.FeeRate * satoshisPerTWINS))
		}

		// Parse selected UTXOs using shared helper function
		if len(opts.SelectedUTXOs) > 0 {
			outpoints, err := parseUTXOStrings(opts.SelectedUTXOs)
			if err != nil {
				return "", err
			}
			walletOpts.SelectedUTXOs = outpoints
		}
	}

	// Call wallet's SendManyWithOptions
	txid, err := w.SendManyWithOptions(recipientsSatoshis, comment, walletOpts)
	if err != nil {
		return "", fmt.Errorf("send failed: %w", err)
	}

	return txid, nil
}

// FeeEstimateResult contains detailed fee estimation for GUI display
type FeeEstimateResult struct {
	Fee        float64 `json:"fee"`        // Estimated fee in TWINS
	InputCount int     `json:"inputCount"` // Number of inputs that would be used
	TxSize     int     `json:"txSize"`     // Estimated transaction size in bytes
}

// EstimateTransactionFee estimates the transaction fee based on recipients and options
// This method works even when the wallet is locked (no signing required)
// Parameters:
//   - recipients: map of address → amount in TWINS
//   - opts: optional SendOptions (coin control UTXOs, fee rate, split count)
//
// Returns FeeEstimateResult with fee (in TWINS), input count, and transaction size
func (c *GoCoreClient) EstimateTransactionFee(recipients map[string]float64, opts *SendOptions) (*FeeEstimateResult, error) {
	c.mu.RLock()
	w := c.wallet
	c.mu.RUnlock()

	if w == nil {
		return nil, fmt.Errorf("wallet not initialized")
	}

	if len(recipients) == 0 {
		return nil, fmt.Errorf("no recipients specified")
	}

	// Convert recipients to satoshis
	// Max safe amount before int64 overflow: 92,233,720,368 TWINS (int64_max / satoshisPerTWINS)
	// Use 21 billion as practical limit (well below overflow threshold)
	const maxAmountTWINS = 21_000_000_000.0
	recipientsSatoshis := make(map[string]int64, len(recipients))
	for addr, amount := range recipients {
		if amount <= 0 {
			return nil, fmt.Errorf("invalid amount for address %s: must be positive", addr)
		}
		if amount > maxAmountTWINS {
			return nil, fmt.Errorf("invalid amount for address %s: exceeds maximum (%g TWINS)", addr, maxAmountTWINS)
		}
		// Convert TWINS to satoshis (1 TWINS = 100,000,000 satoshis)
		amountSatoshis := int64(math.Round(amount * satoshisPerTWINS))
		recipientsSatoshis[addr] = amountSatoshis
	}

	// Parse options
	var selectedUTXOs []types.Outpoint
	var feeRate int64
	var splitCount int

	if opts != nil {
		// Convert fee rate from TWINS/kB to satoshis/kB
		if opts.FeeRate > 0 {
			feeRate = int64(math.Round(opts.FeeRate * satoshisPerTWINS))
		}

		splitCount = opts.SplitCount

		// Parse selected UTXOs
		if len(opts.SelectedUTXOs) > 0 {
			var err error
			selectedUTXOs, err = parseUTXOStrings(opts.SelectedUTXOs)
			if err != nil {
				return nil, err
			}
		}
	}

	// Call wallet's EstimateFee
	result, err := w.EstimateFee(recipientsSatoshis, selectedUTXOs, feeRate, splitCount)
	if err != nil {
		return nil, fmt.Errorf("fee estimation failed: %w", err)
	}

	// Convert fee from satoshis to TWINS
	feeInTWINS := float64(result.Fee) / satoshisPerTWINS

	return &FeeEstimateResult{
		Fee:        feeInTWINS,
		InputCount: result.InputCount,
		TxSize:     result.TxSize,
	}, nil
}

func (c *GoCoreClient) GetTransaction(txid string) (Transaction, error) {
	c.mu.RLock()
	w := c.wallet
	c.mu.RUnlock()

	if w == nil {
		return Transaction{}, fmt.Errorf("wallet not initialized")
	}

	hash, err := types.NewHashFromString(txid)
	if err != nil {
		return Transaction{}, fmt.Errorf("invalid transaction ID: %w", err)
	}

	wtx, err := w.GetTransaction(hash)
	if err != nil {
		return Transaction{}, fmt.Errorf("transaction not found: %w", err)
	}

	return c.convertWalletTransaction(wtx), nil
}

func (c *GoCoreClient) ListTransactions(count int, skip int) ([]Transaction, error) {
	c.mu.RLock()
	w := c.wallet
	c.mu.RUnlock()

	if w == nil {
		return nil, fmt.Errorf("wallet not initialized")
	}

	// Get transactions from wallet
	walletTxs, err := w.ListTransactions(count, skip)
	if err != nil {
		return nil, fmt.Errorf("failed to list transactions: %w", err)
	}

	// Convert wallet transactions to core transactions
	txs := make([]Transaction, 0, len(walletTxs))
	for _, wtx := range walletTxs {
		tx := c.convertWalletTransaction(wtx)
		txs = append(txs, tx)
	}

	return txs, nil
}

// convertWalletTransaction converts a wallet.WalletTransaction to core.Transaction
func (c *GoCoreClient) convertWalletTransaction(wtx *wallet.WalletTransaction) Transaction {
	// Convert satoshis to TWINS (1 TWINS = 100,000,000 satoshis)

	// Coinbase maturity constant (must match chainparams)
	// TWINS mainnet uses 60 blocks for coinbase maturity
	const coinbaseMaturity = 60

	// Map wallet TxCategory to core TransactionType
	txType := mapCategoryToType(wtx.Category)

	// Override type for autocombine consolidation transactions
	if txType == TxTypeSendToSelf && wtx.Comment == "autocombine" {
		txType = TxTypeConsolidation
	}

	// Calculate amount as float64
	amount := float64(wtx.Amount) / satoshisPerTWINS
	fee := float64(wtx.Fee) / satoshisPerTWINS

	// Calculate debit/credit based on amount sign
	var debit, credit float64
	if amount < 0 {
		debit = -amount
	} else {
		credit = amount
	}

	// Determine if coinbase or coinstake
	isCoinbase := wtx.Category == wallet.TxCategoryCoinBase || wtx.Category == wallet.TxCategoryGenerate
	isCoinstake := wtx.Category == wallet.TxCategoryCoinStake || wtx.Category == wallet.TxCategoryMasternode

	// Calculate blocks until maturity for coinbase/coinstake
	var maturesIn int
	if (isCoinbase || isCoinstake) && int(wtx.Confirmations) < coinbaseMaturity {
		maturesIn = coinbaseMaturity - int(wtx.Confirmations)
		if maturesIn < 0 {
			maturesIn = 0
		}
	}

	// Look up label dynamically from address book for current value
	// This ensures labels always reflect the latest saved value
	label := wtx.Label
	if c.wallet != nil && wtx.Address != "" {
		if addressLabel := c.wallet.GetAddressLabel(wtx.Address); addressLabel != "" {
			label = addressLabel
		}
	}

	// For send transactions, extract the external recipient address(es) from
	// the raw tx outputs (with wallet-owned change filtered out). wtx.Address
	// is firstSpendAddress (the wallet's funding address, NOT the recipient),
	// so it cannot be displayed under a "To" label without misleading the
	// user. For cache-loaded entries with nil wtx.Tx, fetch the raw tx via
	// storage; on storage miss recipients stays nil and the frontend falls
	// back to displaying wtx.Address under a "Sent from" label.
	//
	// Only TxCategorySend populates this field. TxCategoryToSelf and
	// consolidation skip extraction because all of their value outputs are
	// wallet-owned by definition (the filter would drop everything) and the
	// frontend already labels these rows correctly ("To yourself" /
	// "Consolidated to") via the existing wtx.Address path.
	var recipients []string
	if wtx.Category == wallet.TxCategorySend {
		matchTx := wtx.Tx
		if matchTx == nil && c.storage != nil {
			if td, err := c.storage.GetTransactionData(wtx.Hash); err == nil && td != nil {
				matchTx = td.TxData
			}
		}
		if matchTx != nil {
			var isOurScript func([]byte) bool
			if c.wallet != nil {
				w := c.wallet
				isOurScript = func(script []byte) bool {
					_, isOurs := w.IsOurScript(script)
					return isOurs
				}
			}
			recipients = extractRecipientAddressesFromTx(matchTx, isOurScript, c.network)
		}
	}

	return Transaction{
		TxID:               wtx.Hash.String(),
		Vout:               int(wtx.Vout),
		Amount:             amount,
		Fee:                fee,
		Confirmations:      int(wtx.Confirmations),
		BlockHash:          blockHashStr(wtx.BlockHash),
		BlockHeight:        int64(wtx.BlockHeight),
		Time:               wtx.Time,
		Type:               txType,
		Address:            wtx.Address,
		RecipientAddresses: recipients,
		FromAddress:        wtx.FromAddress,
		Label:              label,
		Comment:            wtx.Comment,
		Category:           string(wtx.Category),
		IsWatchOnly:        wtx.WatchOnly,
		IsLocked:           false, // SwiftTX not implemented
		IsConflicted:       wtx.IsConflicted,
		IsCoinbase:         isCoinbase,
		IsCoinstake:        isCoinstake,
		MaturesIn:          maturesIn,
		Debit:              debit,
		Credit:             credit,
	}
}

// blockHashStr returns the block hash as a string, or empty string for zero hashes.
// This prevents the frontend from displaying "000...000" for unconfirmed transactions.
func blockHashStr(h types.Hash) string {
	if h.IsZero() {
		return ""
	}
	return h.String()
}

// extractRecipientAddressesFromTx returns the external (non-wallet) output
// addresses on a transaction in natural output order. Wallet-owned outputs
// (i.e. change) are filtered out via isOurScript so the result represents
// the user-visible recipients of a send.
//
// The isOurScript predicate is supplied by the caller (typically wired to
// wallet.Wallet.IsOurScript) so this helper can be unit-tested without
// constructing a real wallet. A nil predicate is treated as "no filter" —
// all decoded output addresses are returned, including any wallet-owned
// change. Callers in production paths MUST pass a non-nil predicate.
//
// `networkName` selects the address prefix ("mainnet" / "testnet" / "regtest").
// Empty / unknown values fall back to mainnet per
// `crypto.GetPubKeyHashNetworkID` semantics. Wired from
// `GoCoreClient.network` (set by `SetNetwork`).
//
// Returns nil when:
//   - tx is nil
//   - every output is wallet-owned (e.g. send-to-self misclassified as send)
//   - every output has an unknown / undecodable script
//
// Multisig and other non-P2PKH/P2SH/P2PK script types are skipped silently
// rather than producing a placeholder — the audit task targets the common
// P2PKH send case, and a future enhancement can add multisig display.
func extractRecipientAddressesFromTx(tx *types.Transaction, isOurScript func([]byte) bool, networkName string) []string {
	if tx == nil {
		return nil
	}
	var recipients []string
	for _, out := range tx.Outputs {
		if out == nil || len(out.ScriptPubKey) == 0 {
			continue
		}
		// Skip wallet-owned outputs (change). The wallet does the canonical
		// match on raw script bytes — no address-string round-trip required.
		if isOurScript != nil && isOurScript(out.ScriptPubKey) {
			continue
		}
		// Classify and extract the recipient address. `binary.AnalyzeScript`
		// returns the hash160 directly for P2PKH/P2SH and `hash160(pubkey)`
		// for P2PK, so a single `crypto.NewAddressFromHash` call covers all
		// three with the network-aware prefix.
		scriptType, scriptHash := binary.AnalyzeScript(out.ScriptPubKey)
		var netID byte
		switch scriptType {
		case binary.ScriptTypeP2PKH, binary.ScriptTypeP2PK:
			netID = crypto.GetPubKeyHashNetworkID(networkName)
		case binary.ScriptTypeP2SH:
			netID = crypto.GetScriptHashNetworkID(networkName)
		default:
			// Unknown / multisig / OP_RETURN — skip.
			continue
		}
		addr, err := crypto.NewAddressFromHash(scriptHash[:], netID)
		if err != nil || addr == nil {
			continue
		}
		recipients = append(recipients, addr.String())
	}
	return recipients
}

// mapCategoryToType maps wallet.TxCategory to core.TransactionType
func mapCategoryToType(cat wallet.TxCategory) TransactionType {
	switch cat {
	case wallet.TxCategorySend:
		return TxTypeSendToAddress
	case wallet.TxCategoryReceive:
		return TxTypeRecvWithAddress
	case wallet.TxCategoryCoinStake:
		return TxTypeStakeMint
	case wallet.TxCategoryCoinBase:
		return TxTypeGenerated
	case wallet.TxCategoryMasternode:
		return TxTypeMNReward
	case wallet.TxCategoryGenerate:
		return TxTypeGenerated
	case wallet.TxCategoryToSelf:
		return TxTypeSendToSelf
	default:
		return TxTypeOther
	}
}

func (c *GoCoreClient) ListTransactionsFiltered(filter TransactionFilter) (TransactionPage, error) {
	c.mu.RLock()
	w := c.wallet
	c.mu.RUnlock()

	if w == nil {
		return TransactionPage{}, fmt.Errorf("wallet not initialized")
	}

	// Validate page size
	validSizes := map[int]bool{25: true, 50: true, 100: true, 250: true}
	if !validSizes[filter.PageSize] {
		filter.PageSize = 25
	}
	if filter.Page < 1 {
		filter.Page = 1
	}

	// Convert MinAmount / MaxAmount from TWINS to satoshis for wallet layer.
	// Both bounds are 0-means-unbounded, so we only multiply non-zero values.
	minAmountSat := float64(0)
	if filter.MinAmount > 0 {
		minAmountSat = filter.MinAmount * satoshisPerTWINS
	}
	maxAmountSat := float64(0)
	if filter.MaxAmount > 0 {
		maxAmountSat = filter.MaxAmount * satoshisPerTWINS
	}

	params := wallet.TransactionFilterParams{
		Page:             filter.Page,
		PageSize:         filter.PageSize,
		DateFilter:       filter.DateFilter,
		DateRangeFrom:    filter.DateRangeFrom,
		DateRangeTo:      filter.DateRangeTo,
		TypeFilter:       filter.TypeFilter,
		SearchText:       filter.SearchText,
		MinAmount:        minAmountSat,
		MaxAmount:        maxAmountSat,
		WatchOnlyFilter:  filter.WatchOnlyFilter,
		HideOrphanStakes: filter.HideOrphanStakes,
		SortColumn:       filter.SortColumn,
		SortDirection:    filter.SortDirection,
	}

	result, err := w.ListTransactionsFiltered(params)
	if err != nil {
		return TransactionPage{}, fmt.Errorf("failed to list filtered transactions: %w", err)
	}

	// Convert wallet transactions to core transactions
	txs := make([]Transaction, 0, len(result.Transactions))
	for _, wtx := range result.Transactions {
		txs = append(txs, c.convertWalletTransaction(wtx))
	}

	totalPages := 0
	if result.Total > 0 {
		totalPages = (result.Total + filter.PageSize - 1) / filter.PageSize
	}

	// Derive actual page from the data the wallet returned rather than
	// re-clamping independently (wallet already clamps out-of-range pages).
	actualPage := filter.Page
	if totalPages > 0 {
		if actualPage > totalPages {
			actualPage = totalPages
		}
	} else {
		actualPage = 1
	}

	return TransactionPage{
		Transactions: txs,
		Total:        result.Total,
		TotalAll:     result.TotalAll,
		Page:         actualPage,
		PageSize:     filter.PageSize,
		TotalPages:   totalPages,
	}, nil
}

func (c *GoCoreClient) ExportFilteredTransactionsCSV(filter TransactionFilter) (string, error) {
	c.mu.RLock()
	w := c.wallet
	c.mu.RUnlock()

	if w == nil {
		return "", fmt.Errorf("wallet not initialized")
	}

	// Convert MinAmount / MaxAmount from TWINS to satoshis for wallet layer.
	// Both bounds are 0-means-unbounded.
	minAmountSat := float64(0)
	if filter.MinAmount > 0 {
		minAmountSat = filter.MinAmount * satoshisPerTWINS
	}
	maxAmountSat := float64(0)
	if filter.MaxAmount > 0 {
		maxAmountSat = filter.MaxAmount * satoshisPerTWINS
	}

	// PageSize <= 0 returns all matching results (no pagination)
	params := wallet.TransactionFilterParams{
		Page:             1,
		PageSize:         0,
		DateFilter:       filter.DateFilter,
		DateRangeFrom:    filter.DateRangeFrom,
		DateRangeTo:      filter.DateRangeTo,
		TypeFilter:       filter.TypeFilter,
		SearchText:       filter.SearchText,
		MinAmount:        minAmountSat,
		MaxAmount:        maxAmountSat,
		WatchOnlyFilter:  filter.WatchOnlyFilter,
		HideOrphanStakes: filter.HideOrphanStakes,
		SortColumn:       filter.SortColumn,
		SortDirection:    filter.SortDirection,
	}

	result, err := w.ListTransactionsFiltered(params)
	if err != nil {
		return "", fmt.Errorf("failed to list filtered transactions: %w", err)
	}

	// Convert and generate CSV
	var sb strings.Builder
	sb.WriteString("\"Confirmed\",\"Date\",\"Type\",\"Label\",\"Address\",\"Amount (TWINS)\",\"ID\"\n")

	for _, wtx := range result.Transactions {
		tx := c.convertWalletTransaction(wtx)
		confirmed := "false"
		if tx.Confirmations >= 6 {
			confirmed = "true"
		}
		date := tx.Time.Format("2006-01-02T15:04:05")
		typeLabel := csvEscape(mapTypeToLabel(tx.Type))
		label := csvEscape(tx.Label)
		address := csvEscape(tx.Address)
		amount := fmt.Sprintf("%.8f", tx.Amount)
		txid := tx.TxID

		sb.WriteString(fmt.Sprintf("\"%s\",\"%s\",%s,%s,%s,\"%s\",\"%s\"\n",
			confirmed, date, typeLabel, label, address, amount, txid))
	}

	return sb.String(), nil
}

// csvEscape escapes a string for CSV with formula injection protection
func csvEscape(s string) string {
	// Sanitize formula injection
	if len(s) > 0 && (s[0] == '=' || s[0] == '+' || s[0] == '-' || s[0] == '@') {
		s = "'" + s
	}
	// Replace control characters
	s = strings.NewReplacer("\t", " ", "\n", " ", "\r", " ").Replace(s)
	return "\"" + strings.ReplaceAll(s, "\"", "\"\"") + "\""
}

// mapTypeToLabel maps a TransactionType to a human-readable label for CSV export
func mapTypeToLabel(t TransactionType) string {
	switch t {
	case TxTypeSendToAddress, TxTypeSendToOther:
		return "Sent to"
	case TxTypeRecvWithAddress, TxTypeRecvFromOther:
		return "Received with"
	case TxTypeSendToSelf:
		return "Payment to yourself"
	case TxTypeGenerated:
		return "Mined"
	case TxTypeStakeMint:
		return "Minted"
	case TxTypeMNReward:
		return "Masternode Reward"
	case TxTypeConsolidation:
		return "UTXO Consolidation"
	default:
		return string(t)
	}
}

func (c *GoCoreClient) ValidateAddress(address string) (AddressValidation, error) {
	// First validate address format
	_, err := crypto.DecodeAddress(address)
	if err != nil {
		return AddressValidation{
			IsValid: false,
			Address: address,
			IsMine:  false,
		}, nil
	}

	// Check wallet ownership - wallet must be available for IsMine check
	c.mu.RLock()
	w := c.wallet
	c.mu.RUnlock()

	if w == nil {
		// Return error so frontend can show appropriate message
		return AddressValidation{
			IsValid: true,
			Address: address,
			IsMine:  false,
		}, fmt.Errorf("wallet not initialized")
	}

	return AddressValidation{
		IsValid: true,
		Address: address,
		IsMine:  w.IsOurAddress(address),
	}, nil
}

func (c *GoCoreClient) EncryptWallet(passphrase string) error {
	return fmt.Errorf("not implemented: use wallet layer")
}

func (c *GoCoreClient) WalletLock() error {
	return fmt.Errorf("not implemented: use wallet layer")
}

func (c *GoCoreClient) WalletPassphrase(passphrase string, timeout int) error {
	return fmt.Errorf("not implemented: use wallet layer")
}

func (c *GoCoreClient) WalletPassphraseChange(oldPassphrase string, newPassphrase string) error {
	return fmt.Errorf("not implemented: use wallet layer")
}

func (c *GoCoreClient) GetWalletInfo() (WalletInfo, error) {
	c.mu.RLock()
	w := c.wallet
	c.mu.RUnlock()

	if w == nil {
		return WalletInfo{}, fmt.Errorf("wallet not initialized")
	}

	// Convert satoshis to TWINS (1 TWINS = 100,000,000 satoshis)

	balance := w.GetBalance()
	info := WalletInfo{
		Version:            1,
		Balance:            float64(balance.Confirmed) / satoshisPerTWINS,
		UnconfirmedBalance: float64(balance.Unconfirmed) / satoshisPerTWINS,
		ImmatureBalance:    float64(balance.Immature) / satoshisPerTWINS,
		Encrypted:          w.IsEncrypted(),
		Unlocked:           !w.IsLocked(),
		UnlockedUntil:      w.UnlockTime(),
		PayTxFee:           0.0001, // Default fee
	}

	return info, nil
}

func (c *GoCoreClient) BackupWallet(destination string) error {
	c.mu.RLock()
	w := c.wallet
	c.mu.RUnlock()

	if w == nil {
		return fmt.Errorf("wallet not initialized")
	}

	return w.BackupWallet(destination)
}

func (c *GoCoreClient) ListUnspent(minConf int, maxConf int) ([]UTXO, error) {
	c.mu.RLock()
	w := c.wallet
	c.mu.RUnlock()

	if w == nil {
		return nil, fmt.Errorf("wallet not initialized")
	}

	// Call wallet ListUnspent with empty address filter (all addresses)
	// The wallet now returns Locked/Spendable fields reflecting both user locks and collateral
	result, err := w.ListUnspent(minConf, maxConf, []string{})
	if err != nil {
		return nil, fmt.Errorf("failed to list unspent: %w", err)
	}

	// Type assert result to []*wallet.UnspentOutput
	unspentOutputs, ok := result.([]*wallet.UnspentOutput)
	if !ok {
		return nil, fmt.Errorf("unexpected type from wallet.ListUnspent: %T", result)
	}

	// Convert to core.UTXO type
	utxos := make([]UTXO, 0, len(unspentOutputs))
	for _, uo := range unspentOutputs {
		// Calculate priority: (amount * confirmations) / 148
		// 148 is approximate size of a typical input in bytes
		priority := uo.Amount * float64(uo.Confirmations) / 148.0

		utxos = append(utxos, UTXO{
			TxID:          uo.TxID,
			Vout:          uo.Vout,
			Address:       uo.Address,
			Label:         "", // TODO: Get from address manager when available
			ScriptPubKey:  uo.ScriptPubKey,
			Amount:        uo.Amount,
			Confirmations: int(uo.Confirmations),
			Spendable:     uo.Spendable,
			Solvable:      true, // Assume solvable for wallet UTXOs
			Locked:        uo.Locked,
			Type:          "Personal", // TODO: Detect multisig when available
			Date:          int64(uo.BlockTime),
			Priority:      priority,
		})
	}

	return utxos, nil
}

// LockUnspent locks or unlocks UTXOs via the unified wallet lock store.
// Legacy: Delegates to CWallet::LockCoin/UnlockCoin — shared by both GUI and RPC.
func (c *GoCoreClient) LockUnspent(unlock bool, outputs []OutPoint) error {
	c.mu.RLock()
	w := c.wallet
	c.mu.RUnlock()

	if w == nil {
		return fmt.Errorf("wallet not initialized")
	}

	for _, op := range outputs {
		outpoint, err := parseOutPoint(op.TxID, op.Vout)
		if err != nil {
			return fmt.Errorf("invalid outpoint %s:%d: %w", op.TxID, op.Vout, err)
		}

		if unlock {
			w.UnlockCoin(outpoint)
		} else {
			w.LockCoin(outpoint)
		}
	}

	return nil
}

// ListLockUnspent returns all currently locked UTXOs from the unified wallet lock store.
func (c *GoCoreClient) ListLockUnspent() ([]OutPoint, error) {
	c.mu.RLock()
	w := c.wallet
	c.mu.RUnlock()

	if w == nil {
		return nil, fmt.Errorf("wallet not initialized")
	}

	locked := w.ListLockedCoins()
	result := make([]OutPoint, 0, len(locked))
	for _, op := range locked {
		result = append(result, OutPoint{
			TxID: op.Hash.String(),
			Vout: op.Index,
		})
	}

	return result, nil
}

// parseOutPoint converts a txid string and vout to a types.Outpoint.
func parseOutPoint(txid string, vout uint32) (types.Outpoint, error) {
	hash, err := types.NewHashFromString(txid)
	if err != nil {
		return types.Outpoint{}, fmt.Errorf("invalid txid: %w", err)
	}
	return types.Outpoint{Hash: hash, Index: vout}, nil
}

// EstimateFee estimates the fee rate (in TWINS/kB) for confirmation within the specified number of blocks.
// For TWINS, this returns a fee rate based on desired confirmation time:
// - 1-2 blocks (fast): Higher fee rate for priority
// - 3-6 blocks (normal): Standard fee rate
// - 7+ blocks (economy): Minimum relay fee
func (c *GoCoreClient) EstimateFee(blocks int) (float64, error) {
	// TWINS default fee is 0.0001 TWINS/kB (10000 satoshis/kB)
	const (
		defaultFeePerKB  = 0.0001  // Standard fee (10000 satoshis/kB)
		priorityFeePerKB = 0.001   // Priority fee (100000 satoshis/kB)
		minFeePerKB      = 0.00001 // Minimum relay fee (1000 satoshis/kB)
	)

	// Check if wallet is available to get configured fee
	c.mu.RLock()
	w := c.wallet
	c.mu.RUnlock()

	var configuredFee float64
	if w != nil {
		// Get fee from wallet configuration (in satoshis/kB)
		feePerKB := w.GetTransactionFee()
		if feePerKB > 0 {
			configuredFee = float64(feePerKB) / satoshisPerTWINS // Convert satoshis to TWINS
		}
	}

	// Return fee based on confirmation urgency
	switch {
	case blocks <= 2:
		// Fast confirmation - use priority fee or 10x configured
		if configuredFee > 0 {
			return configuredFee * 10, nil
		}
		return priorityFeePerKB, nil
	case blocks <= 6:
		// Normal confirmation - use configured or default fee
		if configuredFee > 0 {
			return configuredFee, nil
		}
		return defaultFeePerKB, nil
	default:
		// Economy - use minimum fee
		return minFeePerKB, nil
	}
}

func (c *GoCoreClient) GetBlockchainInfo() (BlockchainInfo, error) {
	height, err := c.storage.GetChainHeight()
	if err != nil {
		return BlockchainInfo{}, err
	}
	tip, _ := c.storage.GetChainTip()

	info := BlockchainInfo{
		Blocks:        int64(height),
		Headers:       int64(height),
		BestBlockHash: tip.String(),
		Chain:         "main",
	}

	// Populate last block time from chain tip block header timestamp.
	// Cache by tip hash so successive 10-second status polls don't repeatedly
	// load the full tip block (which fetches all transactions). The full read
	// happens at most once per new chain tip.
	info.LastBlockTime = c.lastBlockTimeForTip(tip)

	// Populate sync status fields from syncer and p2p server if available
	c.mu.RLock()
	syncer := c.syncer
	p2p := c.p2pServer
	dataDir := c.dataDir
	bc := c.blockchain
	c.mu.RUnlock()

	// Difficulty: derived inline from the cached tip block's compact target.
	// lastBlockTimeForTip above populates the bits cache for this tip.
	info.Difficulty = c.difficultyForTip(tip)

	// ChainSizeBytes: total size of <dataDir>/blockchain.db, cached with 10s TTL.
	info.ChainSizeBytes = c.chainSizeForDataDir(dataDir)

	// MoneySupply: total TWINS in circulation at the chain tip. Optional read;
	// errors are silently swallowed (field stays at 0 → frontend renders "N/A").
	if bc != nil {
		if sat, err := bc.GetMoneySupply(height); err == nil && sat > 0 {
			info.MoneySupply = float64(sat) / satoshisPerTWINS
		}
	}

	// Populate peer count and connecting state for frontend sync status determination
	if p2p != nil {
		info.PeerCount = int(p2p.GetPeerCount())
		info.IsConnecting = info.PeerCount < blockchain.MinSyncPeers
	}

	if syncer != nil {
		current, target, _ := syncer.GetSyncProgress()
		isSyncing := syncer.IsSyncing()
		isSynced := syncer.IsSynced()
		networkHeight := syncer.GetNetworkHeight()

		// If network consensus height is 0, we have no reliable consensus
		// and cannot claim to be synced regardless of state machine
		if networkHeight == 0 {
			isSynced = false
		}

		info.IsSyncing = isSyncing

		// Note: we do NOT populate info.Headers from networkHeight as a proxy.
		// The syncer doesn't track headers separately from blocks today, so
		// Headers stays equal to Blocks. The Sync card shows Network height
		// directly from NetworkInfo.network_height instead, which is the
		// authoritative source for "how far ahead the network is".

		// Calculate behind blocks (only if target > current)
		if target > current {
			behindBlocks := int64(target - current)
			info.BehindBlocks = behindBlocks
			info.IsOutOfSync = true

			// Calculate sync percentage
			if target > 0 {
				info.SyncPercentage = (float64(current) / float64(target)) * 100
			}

			// Calculate behind time (TWINS has ~60 second block time)
			info.BehindTime = formatBehindTime(behindBlocks)
		} else if !isSynced {
			// Not synced but target <= current (e.g., no consensus, too few peers)
			info.IsOutOfSync = true
			info.SyncPercentage = 0
			info.BehindTime = ""
		} else {
			// We're synced
			info.IsOutOfSync = false
			info.SyncPercentage = 100.0
			info.BehindTime = "up to date"
		}

		// Current block being processed during sync
		if isSyncing {
			info.CurrentBlockScan = int64(current)
		}
	}

	return info, nil
}

// lastBlockTimeForTip returns the chain-tip block header timestamp (Unix seconds),
// caching by tip hash so successive calls within the same chain tip do not re-read
// the full block (which fetches all transactions). Returns 0 if the tip block can't
// be loaded — the GUI renders that as "N/A".
func (c *GoCoreClient) lastBlockTimeForTip(tip types.Hash) int64 {
	c.tipTimeMu.Lock()
	defer c.tipTimeMu.Unlock()

	if c.tipTimeUnix > 0 && c.tipTimeHash == tip {
		return c.tipTimeUnix
	}

	tipBlock, err := c.storage.GetBlock(tip)
	if err != nil || tipBlock == nil || tipBlock.Header == nil {
		return 0
	}
	c.tipTimeHash = tip
	c.tipTimeUnix = int64(tipBlock.Header.Timestamp)
	return c.tipTimeUnix
}

// difficultyForTip returns the current PoS difficulty derived from the chain
// tip block's compact-target field. Reads the tip block directly from storage
// (no cache) to keep the implementation independent of lastBlockTimeForTip's
// cache state. Returns 0 if the tip block can't be loaded or the target is
// degenerate.
//
// Uses the Bitcoin-style RPC display formula (matches RPC `getdifficulty`
// output bit-for-bit). Mirrors `calculateDifficultyFromBits` in
// internal/rpc/blockchain_adapter.go:76. Note that `BlockChain.GetDifficulty`
// uses a different formula based on `uint64(1) << multiplier` which overflows
// to 0 for typical TWINS bits exponents (>= 0x0a) — this 256-step multiply/
// divide form is the correct one.
func (c *GoCoreClient) difficultyForTip(tip types.Hash) float64 {
	tipBlock, err := c.storage.GetBlock(tip)
	if err != nil || tipBlock == nil || tipBlock.Header == nil {
		return 0
	}
	bits := tipBlock.Header.Bits
	if bits == 0 {
		return 0
	}

	// Extract exponent (nShift) from high byte
	nShift := int((bits >> 24) & 0xff)
	// Extract mantissa from lower 3 bytes
	mantissa := bits & 0x00ffffff
	if mantissa == 0 {
		return 0
	}

	// Base difficulty = 0x0000ffff / mantissa, adjusted for exponent
	// difference from reference exponent 29 (×256 per step below, ÷256 above).
	dDiff := float64(0x0000ffff) / float64(mantissa)
	for nShift < 29 {
		dDiff *= 256.0
		nShift++
	}
	for nShift > 29 {
		dDiff /= 256.0
		nShift--
	}
	return dDiff
}

// chainSizeCacheTTL bounds how often chainSizeForDataDir performs the full
// recursive WalkDir of the Pebble database directory. Pebble databases are
// typically hundreds of .sst files; the on-disk total grows slowly compared
// to the GUI status poll cadence (~10s), so a 60s TTL keeps the displayed
// "Chain size on disk" value fresh enough without paying the walk cost on
// every poll.
const chainSizeCacheTTL = 60 * time.Second

// chainSizeForDataDir returns the cached total size of <dataDir>/blockchain.db
// in bytes. Uses stale-while-revalidate semantics:
//   - Returns the current cached value immediately (never blocks the caller).
//   - When the cache is expired, kicks off a background goroutine to refresh
//     it. The chainSizeWalking atomic guard ensures at most one refresh is
//     in flight at a time.
//   - On the very first call (before any walk has completed) returns 0.
//
// This pattern is critical because GetBlockchainInfo() is on the GUI status
// hot path (10s polling cycle); a synchronous WalkDir of a large Pebble
// database would freeze the UI for seconds every time the cache expires.
func (c *GoCoreClient) chainSizeForDataDir(dataDir string) int64 {
	c.chainSizeMu.Lock()
	cached := c.chainSizeBytes
	expired := time.Now().After(c.chainSizeExpiry)
	c.chainSizeMu.Unlock()

	if !expired || dataDir == "" {
		return cached
	}

	// Cache expired: trigger a background refresh if no other walk is in flight.
	// CompareAndSwap on the atomic guard makes this race-free without holding
	// chainSizeMu across the goroutine spawn.
	if c.chainSizeWalking.CompareAndSwap(false, true) {
		go c.refreshChainSize(dataDir)
	}

	// Return the stale cached value while the refresh runs.
	return cached
}

// refreshChainSize performs the recursive WalkDir of <dataDir>/blockchain.db
// off the hot path and updates the chain-size cache atomically on success.
// On total walk failure (e.g. missing directory) leaves the cache untouched
// but bumps the expiry by chainSizeCacheTTL so we don't hammer a broken path
// on every poll. Always clears chainSizeWalking before returning.
func (c *GoCoreClient) refreshChainSize(dataDir string) {
	defer c.chainSizeWalking.Store(false)

	dbPath := filepath.Join(dataDir, "blockchain.db")
	var total int64
	walkErr := filepath.WalkDir(dbPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Skip unreadable entries; partial size is acceptable.
			return nil
		}
		if !d.IsDir() {
			if fi, statErr := d.Info(); statErr == nil {
				total += fi.Size()
			}
		}
		return nil
	})

	c.chainSizeMu.Lock()
	defer c.chainSizeMu.Unlock()
	if walkErr != nil {
		// Don't overwrite the cached value on a transient walk error, but
		// bump expiry so the next caller doesn't immediately retry.
		c.chainSizeExpiry = time.Now().Add(chainSizeCacheTTL)
		return
	}
	c.chainSizeBytes = total
	c.chainSizeExpiry = time.Now().Add(chainSizeCacheTTL)
}

// formatBehindTime converts blocks behind into a human-readable time string.
// TWINS has approximately 60 second block times.
func formatBehindTime(blocks int64) string {
	if blocks <= 0 {
		return "up to date"
	}

	// TWINS block time is ~60 seconds
	totalSeconds := blocks * 60

	minutes := totalSeconds / 60
	hours := minutes / 60
	days := hours / 24
	weeks := days / 7

	if weeks > 0 {
		return fmt.Sprintf("%d weeks behind", weeks)
	}
	if days > 0 {
		return fmt.Sprintf("%d days behind", days)
	}
	if hours > 0 {
		return fmt.Sprintf("%d hours behind", hours)
	}
	if minutes > 0 {
		return fmt.Sprintf("%d minutes behind", minutes)
	}
	return fmt.Sprintf("%d blocks behind", blocks)
}

func (c *GoCoreClient) GetNetworkInfo() (NetworkInfo, error) {
	c.mu.RLock()
	p2pServer := c.p2pServer
	syncer := c.syncer
	c.mu.RUnlock()

	info := NetworkInfo{
		Version:         70928, // Protocol version
		Subversion:      "/TWINS Core:2.0.0/",
		ProtocolVersion: 70928,
		LocalRelay:      true,
		RelayFee:        0.0001,
	}

	if p2pServer != nil {
		info.Connections = int(p2pServer.GetPeerCount())
		info.InboundPeers = int(p2pServer.GetInboundCount())
		info.OutboundPeers = int(p2pServer.GetOutboundCount())
		info.NetworkActive = p2pServer.IsStarted()
	} else {
		// P2P not initialized yet
		info.Connections = 0
		info.InboundPeers = 0
		info.OutboundPeers = 0
		info.NetworkActive = false
	}

	// Populate network consensus height from syncer
	if syncer != nil {
		info.NetworkHeight = int64(syncer.GetNetworkHeight())
	}

	return info, nil
}

func (c *GoCoreClient) GetBlock(hash string) (Block, error) {
	return Block{}, fmt.Errorf("not implemented: use GetExplorerBlock")
}

func (c *GoCoreClient) GetBlockHash(height int64) (string, error) {
	hash, err := c.storage.GetBlockHashByHeight(uint32(height))
	if err != nil {
		return "", err
	}
	return hash.String(), nil
}

func (c *GoCoreClient) GetBlockCount() (int64, error) {
	height, err := c.storage.GetChainHeight()
	return int64(height), err
}

func (c *GoCoreClient) GetPeerInfo() ([]PeerInfo, error) {
	return nil, fmt.Errorf("not implemented: use p2p layer")
}

func (c *GoCoreClient) GetConnectionCount() (int, error) {
	return 0, fmt.Errorf("not implemented: use p2p layer")
}

func (c *GoCoreClient) MasternodeList(filter string) ([]MasternodeInfo, error) {
	if c.masternode == nil {
		return nil, fmt.Errorf("masternode manager not initialized")
	}

	// Get current block height for rank calculation
	var blockHeight uint32
	if c.storage != nil {
		if height, err := c.storage.GetChainHeight(); err == nil && height > masternode.ScoreBlockDepth {
			blockHeight = height - masternode.ScoreBlockDepth
		}
	}

	// Get masternodes with calculated ranks
	// GetMasternodeRanks returns masternodes sorted by rank with proper score calculation
	rankedMns := c.masternode.GetMasternodeRanks(blockHeight, masternode.ActiveProtocolVersion)

	// Build lookup map for ranks by outpoint
	rankMap := make(map[string]int)
	for _, entry := range rankedMns {
		outpointKey := entry.Masternode.OutPoint.Hash.String() + ":" + fmt.Sprintf("%d", entry.Masternode.OutPoint.Index)
		rankMap[outpointKey] = entry.Rank
	}

	// Get all masternodes and populate with ranks
	mns := c.masternode.GetMasternodes()
	result := make([]MasternodeInfo, 0, len(mns))
	currentTime := time.Now()
	expireTime := time.Duration(masternode.ExpirationSeconds) * time.Second

	for outpoint, mn := range mns {
		// Refresh status before reading to match legacy mn.Check() behavior.
		// Without this, masternodes stay stuck at PRE_ENABLED in the GUI
		// because UpdateStatus() is what transitions PRE_ENABLED -> ENABLED.
		// See: masternode_adapter.go:54 (RPC path has same fix)
		mn.UpdateStatus(currentTime, expireTime)

		// Get address string from net.Addr
		addrStr := ""
		if mn.Addr != nil {
			addrStr = mn.Addr.String()
		}
		// Get public keys as hex strings
		pubKey := ""
		if mn.PubKeyCollateral != nil {
			pubKey = mn.PubKeyCollateral.CompressedHex()
		}
		pubKeyOperator := ""
		if mn.PubKey != nil {
			pubKeyOperator = mn.PubKey.CompressedHex()
		}

		// Look up calculated rank
		outpointKey := outpoint.Hash.String() + ":" + fmt.Sprintf("%d", outpoint.Index)
		rank := rankMap[outpointKey] // 0 if not found

		// Calculate active time as live-incrementing duration since activation
		activeTime := int64(0)
		if !mn.ActiveSince.IsZero() {
			activeTime = time.Now().Unix() - mn.ActiveSince.Unix()
			if activeTime < 0 {
				activeTime = 0 // Guard against clock skew
			}
		}

		// Use payment tracker for LastPaid if available, fall back to mn.LastPaid
		// Normalize: zero time or Unix epoch both mean "never paid"
		lastPaid := mn.LastPaid
		if c.paymentTracker != nil {
			if stats := c.paymentTracker.GetStatsByScript(mn.GetPayeeScript()); stats != nil {
				lastPaid = stats.LastPaid
			}
		}
		if lastPaid.Unix() <= 0 {
			lastPaid = time.Time{}
		}

		info := MasternodeInfo{
			Rank:           rank,
			Address:        addrStr,
			Status:         mn.Status.String(),
			ActiveTime:     activeTime,
			LastSeen:       mn.LastSeen,
			LastPaid:       lastPaid,
			Txhash:         outpoint.Hash.String(),
			Outidx:         int(outpoint.Index),
			Tier:           mn.Tier.String(),
			Version:        int(mn.Protocol),
			PubKey:         pubKey,
			PubKeyOperator: pubKeyOperator,
			PaymentAddress: mn.GetPayee(), // Get payment address from collateral pubkey
		}
		// Apply filter if provided
		if filter == "" || filter == "all" ||
			(filter == "enabled" && mn.Status == masternode.StatusEnabled) {
			result = append(result, info)
		}
	}
	return result, nil
}

func (c *GoCoreClient) MasternodeStart(alias string) error {
	return fmt.Errorf("not implemented: use masternode layer")
}

func (c *GoCoreClient) MasternodeStartAll() error {
	return fmt.Errorf("not implemented: use masternode layer")
}

func (c *GoCoreClient) MasternodeStatus() (MasternodeStatus, error) {
	return MasternodeStatus{}, fmt.Errorf("not implemented: use masternode layer")
}

func (c *GoCoreClient) GetMasternodeCount() (MasternodeCount, error) {
	if c.masternode == nil {
		return MasternodeCount{}, fmt.Errorf("masternode manager not initialized")
	}
	total := c.masternode.GetMasternodeCount()
	enabled := c.masternode.GetActiveCount()
	return MasternodeCount{
		Total:   total,
		Enabled: enabled,
	}, nil
}

func (c *GoCoreClient) MasternodeCurrentWinner() (MasternodeInfo, error) {
	return MasternodeInfo{}, fmt.Errorf("not implemented: use masternode layer")
}

func (c *GoCoreClient) GetMyMasternodes() ([]MyMasternode, error) {
	return nil, fmt.Errorf("not implemented: use masternode layer")
}

func (c *GoCoreClient) MasternodeStartMissing() (int, error) {
	return 0, fmt.Errorf("not implemented: use masternode layer")
}

func (c *GoCoreClient) GetStakingInfo() (StakingInfo, error) {
	c.mu.RLock()
	consensus := c.consensus
	w := c.wallet
	stakingEnabled := c.stakingEnabled
	store := c.storage
	c.mu.RUnlock()

	info := StakingInfo{
		Enabled: stakingEnabled,
	}

	// Get staking status from consensus engine
	if consensus != nil {
		info.Staking = consensus.IsStaking()
	}

	// Get wallet lock status and reserve balance.
	// GetReserveBalance returns (enabled, amount, err). The GUI displays the
	// CONFIGURED threshold so the user can always see what they set in Options
	// — regardless of whether reserve staking is currently active. The
	// `enabled` flag governs runtime staking behaviour, not display.
	if w != nil {
		info.WalletUnlocked = !w.IsLocked()
		if _, sat, err := w.GetReserveBalance(); err == nil {
			info.ReserveBalance = float64(sat) / satoshisPerTWINS
		}
	}

	// Compute expected time to next stake from chain data and wallet UTXOs.
	//
	// The TWINS PoS kernel runs once per second per UTXO; a hit occurs when:
	//   hash(kernelData) < target × (utxo.Amount/100)
	// where target = CompactToBig(block.Bits) is the per-second-per-coin-unit difficulty.
	//
	// Probability of wallet finding a block in any given second:
	//   P = target × walletWeight / 2^256   (walletWeight = Σ utxo.Amount/100)
	// Expected seconds until next stake:
	//   E[t] = 1/P = 2^256 / (target × walletWeight)
	//
	// This is equivalent to (networkWeight / walletWeight) × target_spacing because the
	// difficulty adjustment sets target = 2^256 / (networkWeight × target_spacing), so
	// 2^256 / (target × walletWeight) = target_spacing × networkWeight / walletWeight.
	// No additional target_spacing factor is needed here.
	if store != nil && w != nil {
		if chainHeight, err := store.GetChainHeight(); err == nil {
			if bestBlock, err := store.GetBlockByHeight(chainHeight); err == nil && bestBlock != nil {
				chainTime := bestBlock.Header.Timestamp
				if utxos, err := w.GetStakeableUTXOs(chainHeight, chainTime); err == nil && len(utxos) > 0 {
					walletWeightBig := new(big.Int)
					for _, utxo := range utxos {
						walletWeightBig.Add(walletWeightBig, big.NewInt(utxo.Amount/100))
					}
					if walletWeightBig.Sign() > 0 {
						target := types.CompactToBig(bestBlock.Header.Bits)
						if target.Sign() > 0 {
							two256 := new(big.Int).Lsh(big.NewInt(1), 256)
							effectiveTarget := new(big.Int).Mul(target, walletWeightBig)
							if effectiveTarget.Cmp(two256) >= 0 {
								// Theoretical overflow guard: effectiveTarget >= 2^256 would
								// produce a nonsensical result. In practice this cannot occur
								// (walletWeight in satoshis/100 is far below 2^256/target), but
								// we guard defensively and display N/A.
								info.ExpectedStakeTime = 0
							} else {
								expected := new(big.Int).Div(two256, effectiveTarget)
								const maxExpected = int64(10 * 365 * 24 * 3600)
								if expected.IsInt64() && expected.Int64() < maxExpected {
									info.ExpectedStakeTime = expected.Int64()
								} else {
									info.ExpectedStakeTime = maxExpected
								}
							}
						}
					}
				}
			}
		}
	}

	return info, nil
}

func (c *GoCoreClient) SetStaking(enabled bool) error {
	return fmt.Errorf("not implemented: use staking layer")
}

func (c *GoCoreClient) GetStakingStatus() (bool, error) {
	return false, fmt.Errorf("not implemented: use staking layer")
}

func (c *GoCoreClient) SignMessage(address string, message string) (string, error) {
	return "", fmt.Errorf("not implemented: use wallet layer")
}

func (c *GoCoreClient) VerifyMessage(address string, signature string, message string) (bool, error) {
	return false, fmt.Errorf("not implemented")
}

func (c *GoCoreClient) GetInfo() (map[string]interface{}, error) {
	height, _ := c.storage.GetChainHeight()
	return map[string]interface{}{
		"blocks": height,
	}, nil
}

func (c *GoCoreClient) AddNode(node string, command string) error {
	return fmt.Errorf("not implemented: use p2p layer")
}

func (c *GoCoreClient) DisconnectNode(address string) error {
	return fmt.Errorf("not implemented: use p2p layer")
}

func (c *GoCoreClient) GetAddedNodeInfo(node string) ([]interface{}, error) {
	return nil, fmt.Errorf("not implemented: use p2p layer")
}

func (c *GoCoreClient) SetNetworkActive(active bool) error {
	return fmt.Errorf("not implemented: use p2p layer")
}

func (c *GoCoreClient) InvalidateBlock(hash string) error {
	return fmt.Errorf("not implemented: use blockchain layer")
}

func (c *GoCoreClient) ReconsiderBlock(hash string) error {
	return fmt.Errorf("not implemented: use blockchain layer")
}

func (c *GoCoreClient) VerifyChain(checkLevel int, numBlocks int) (bool, error) {
	return false, fmt.Errorf("not implemented: use blockchain layer")
}
