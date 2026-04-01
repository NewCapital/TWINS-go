package core

import "context"

// CoreClient is the main interface to the TWINS blockchain core.
// It mirrors the functionality provided by the C++ core to the Qt GUI
// via direct function calls and the uiInterface signal system.
//
// This interface allows for multiple implementations:
//   - MockCoreClient: For development and testing (Phase 1)
//   - GoCoreClient: Real Go blockchain implementation (Future)
//
// All methods are synchronous unless they return a channel.
// Events are delivered via the Events() channel for async notifications.
type CoreClient interface {
	// ==========================================
	// Lifecycle Management
	// ==========================================

	// Start initializes the core and begins blockchain operations.
	// This is equivalent to AppInit2() in the C++ core.
	// The context can be used to cancel the startup process.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the core.
	// All goroutines should be stopped and resources cleaned up.
	Stop() error

	// IsRunning returns true if the core is currently running.
	IsRunning() bool

	// Events returns a read-only channel for receiving core events.
	// The channel is closed when the core stops.
	// Events mirror the C++ uiInterface signals (NotifyBlockTip, etc.)
	Events() <-chan CoreEvent

	// ==========================================
	// Wallet Operations (Direct Calls)
	// ==========================================

	// GetBalance returns all wallet balance types.
	// Equivalent to: CWallet::GetBalance()
	GetBalance() (Balance, error)

	// GetNewAddress generates a new receiving address with optional label.
	// Equivalent to: getnewaddress RPC
	GetNewAddress(label string) (string, error)

	// SendToAddress sends coins to an address.
	// Equivalent to: sendtoaddress RPC
	// Returns the transaction ID on success.
	SendToAddress(address string, amount float64, comment string) (string, error)

	// SendToAddressWithOptions sends coins with advanced options.
	// Supports coin control (specific UTXO selection), custom change address, and UTXO splitting.
	SendToAddressWithOptions(address string, amount float64, comment string, opts *SendOptions) (string, error)

	// SendMany sends coins to multiple recipients in a single transaction.
	// recipients: map of address → amount in TWINS
	// Supports all options from SendOptions (coin control, custom change, UTXO split).
	SendMany(recipients map[string]float64, comment string, opts *SendOptions) (string, error)

	// GetTransaction gets transaction details by ID.
	// Equivalent to: gettransaction RPC
	GetTransaction(txid string) (Transaction, error)

	// ListTransactions returns recent transactions.
	// Equivalent to: listtransactions RPC
	// count: number of transactions to return
	// skip: number of transactions to skip
	ListTransactions(count int, skip int) ([]Transaction, error)

	// ListTransactionsFiltered returns a paginated, filtered, and sorted page of transactions.
	// All filtering and sorting is performed server-side.
	ListTransactionsFiltered(filter TransactionFilter) (TransactionPage, error)

	// ExportFilteredTransactionsCSV returns CSV content for all transactions matching the filter.
	// The filter's Page/PageSize are ignored; all matching transactions are included.
	ExportFilteredTransactionsCSV(filter TransactionFilter) (string, error)

	// ValidateAddress checks if an address is valid and provides detailed information.
	// Equivalent to: validateaddress RPC
	ValidateAddress(address string) (AddressValidation, error)

	// EncryptWallet encrypts the wallet with a passphrase.
	// Equivalent to: encryptwallet RPC
	// WARNING: This operation requires a restart in the C++ version.
	EncryptWallet(passphrase string) error

	// WalletLock locks the wallet.
	// Equivalent to: walletlock RPC
	WalletLock() error

	// WalletPassphrase unlocks the wallet for a duration (seconds).
	// Equivalent to: walletpassphrase RPC
	WalletPassphrase(passphrase string, timeout int) error

	// WalletPassphraseChange changes the wallet passphrase.
	// Equivalent to: walletpassphrasechange RPC
	WalletPassphraseChange(oldPassphrase string, newPassphrase string) error

	// GetWalletInfo returns wallet information and status.
	// Equivalent to: getwalletinfo RPC
	GetWalletInfo() (WalletInfo, error)

	// BackupWallet backs up the wallet to a destination file.
	// Equivalent to: backupwallet RPC
	BackupWallet(destination string) error

	// ListUnspent returns unspent transaction outputs.
	// Equivalent to: listunspent RPC
	ListUnspent(minConf int, maxConf int) ([]UTXO, error)

	// LockUnspent locks or unlocks specified transaction outputs.
	// Equivalent to: lockunspent RPC
	// unlock: true to unlock, false to lock
	// outputs: list of outputs to lock/unlock
	LockUnspent(unlock bool, outputs []OutPoint) error

	// ListLockUnspent returns list of temporarily locked outputs.
	// Equivalent to: listlockunspent RPC
	ListLockUnspent() ([]OutPoint, error)

	// EstimateFee estimates the fee rate for confirmation within the specified number of blocks.
	// Equivalent to: estimatefee RPC
	// Returns the fee rate in TWINS per KB.
	EstimateFee(blocks int) (float64, error)

	// EstimateTransactionFee estimates the actual transaction fee based on recipients and options.
	// Unlike EstimateFee (which returns a fee rate), this method:
	// - Selects UTXOs (automatically or from coin control selection)
	// - Calculates the exact transaction size based on actual input count
	// - Returns the fee in TWINS along with input count and tx size for UI display
	// Works even when wallet is locked (no signing required).
	EstimateTransactionFee(recipients map[string]float64, opts *SendOptions) (*FeeEstimateResult, error)

	// ==========================================
	// Blockchain Operations
	// ==========================================

	// GetBlockchainInfo returns blockchain state information.
	// Equivalent to: getblockchaininfo RPC + chainActive.Height()
	GetBlockchainInfo() (BlockchainInfo, error)

	// GetNetworkInfo returns network information.
	// Equivalent to: getnetworkinfo RPC + vNodes access
	GetNetworkInfo() (NetworkInfo, error)

	// GetBlock returns block data by hash.
	// Equivalent to: getblock RPC
	GetBlock(hash string) (Block, error)

	// GetBlockHash returns block hash at given height.
	// Equivalent to: getblockhash RPC
	GetBlockHash(height int64) (string, error)

	// GetBlockCount returns the current block height.
	// Equivalent to: getblockcount RPC
	GetBlockCount() (int64, error)

	// GetPeerInfo returns information about connected peers.
	// Equivalent to: getpeerinfo RPC + vNodes
	GetPeerInfo() ([]PeerInfo, error)

	// GetConnectionCount returns the number of connections.
	// Equivalent to: getconnectioncount RPC
	GetConnectionCount() (int, error)

	// ==========================================
	// Masternode Operations
	// ==========================================

	// MasternodeList returns list of masternodes.
	// Equivalent to: masternode list RPC
	// filter: empty string for all, or specific filter (rank, active, etc.)
	MasternodeList(filter string) ([]MasternodeInfo, error)

	// MasternodeStart starts a masternode by alias.
	// Equivalent to: masternode start-alias RPC
	MasternodeStart(alias string) error

	// MasternodeStartAll starts all configured masternodes.
	// Equivalent to: masternode start-all RPC
	MasternodeStartAll() error

	// MasternodeStatus returns local masternode status.
	// Equivalent to: masternode status RPC
	MasternodeStatus() (MasternodeStatus, error)

	// GetMasternodeCount returns masternode count statistics.
	// Equivalent to: masternode count RPC
	GetMasternodeCount() (MasternodeCount, error)

	// MasternodeCurrentWinner returns the current masternode winner.
	// Equivalent to: masternode winner RPC
	MasternodeCurrentWinner() (MasternodeInfo, error)

	// GetMyMasternodes returns the user's configured masternodes for the UI table.
	// This combines data from masternode.conf with network status.
	// Equivalent to iterating masternodeConfig.getEntries() in Qt wallet.
	GetMyMasternodes() ([]MyMasternode, error)

	// MasternodeStartMissing starts only masternodes with MISSING status.
	// Equivalent to: masternode start-missing RPC
	MasternodeStartMissing() (int, error)

	// ==========================================
	// Staking Operations
	// ==========================================

	// GetStakingInfo returns staking status and statistics.
	// Equivalent to: getstakinginfo RPC
	GetStakingInfo() (StakingInfo, error)

	// SetStaking enables or disables staking.
	// Equivalent to: setstaking RPC
	SetStaking(enabled bool) error

	// GetStakingStatus returns current staking status.
	// Simple boolean version of GetStakingInfo.
	GetStakingStatus() (bool, error)

	// ==========================================
	// Explorer Operations
	// ==========================================

	// GetLatestBlocks returns the most recent blocks for explorer view.
	// Returns blocks in descending order (newest first).
	// limit: maximum number of blocks to return (default 25)
	// offset: number of blocks to skip from the tip (for pagination)
	GetLatestBlocks(limit, offset int) ([]BlockSummary, error)

	// GetExplorerBlock returns detailed block information by hash or height.
	// query can be a block hash (64 hex chars) or block height (number).
	GetExplorerBlock(query string) (BlockDetail, error)

	// GetExplorerTransaction returns detailed transaction information.
	// txid: the transaction hash
	GetExplorerTransaction(txid string) (ExplorerTransaction, error)

	// GetAddressInfo returns information about an address including balance and history.
	// address: the TWINS address to look up
	// limit: maximum number of transactions to include in history (default 25)
	GetAddressInfo(address string, limit int) (AddressInfo, error)

	// SearchExplorer searches for a block, transaction, or address.
	// query: can be block hash, block height, transaction hash, or address
	SearchExplorer(query string) (SearchResult, error)

	// GetAddressTransactions returns a page of transactions for an address.
	// address: the TWINS address
	// limit: number of transactions per batch
	// offset: starting position (0-based, from most recent)
	GetAddressTransactions(address string, limit, offset int) (AddressTxPage, error)

	// ==========================================
	// Utility Operations
	// ==========================================

	// SignMessage signs a message with an address's private key.
	// Equivalent to: signmessage RPC
	SignMessage(address string, message string) (string, error)

	// VerifyMessage verifies a signed message.
	// Equivalent to: verifymessage RPC
	VerifyMessage(address string, signature string, message string) (bool, error)

	// GetInfo returns general information about the node.
	// Equivalent to: getinfo RPC (deprecated but useful)
	GetInfo() (map[string]interface{}, error)

	// ==========================================
	// Network Management
	// ==========================================

	// AddNode adds a node to connect to.
	// Equivalent to: addnode RPC
	AddNode(node string, command string) error

	// DisconnectNode disconnects from a node.
	// Equivalent to: disconnectnode RPC
	DisconnectNode(address string) error

	// GetAddedNodeInfo returns information about manually added nodes.
	// Equivalent to: getaddednodeinfo RPC
	GetAddedNodeInfo(node string) ([]interface{}, error)

	// SetNetworkActive enables or disables all network activity.
	// Equivalent to: setnetworkactive RPC
	SetNetworkActive(active bool) error

	// ==========================================
	// Blockchain Maintenance
	// ==========================================

	// InvalidateBlock marks a block as invalid.
	// Equivalent to: invalidateblock RPC
	InvalidateBlock(hash string) error

	// ReconsiderBlock removes invalidity status of a block.
	// Equivalent to: reconsiderblock RPC
	ReconsiderBlock(hash string) error

	// VerifyChain verifies blockchain database.
	// Equivalent to: verifychain RPC
	VerifyChain(checkLevel int, numBlocks int) (bool, error)
}
