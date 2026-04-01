package main

import (
	"fmt"

	"github.com/twins-dev/twins-core/internal/gui/core"
)

// ==========================================
// UTXO / Coin Control Operations
// ==========================================

// ListUnspent returns unspent transaction outputs (UTXOs) for coin control
func (a *App) ListUnspent(minConf int, maxConf int) ([]core.UTXO, error) {
	if a.coreClient == nil {
		return nil, fmt.Errorf("core client not initialized")
	}
	// Validate confirmation range
	if minConf < 0 {
		return nil, fmt.Errorf("invalid minConf: must be non-negative, got %d", minConf)
	}
	if maxConf < minConf {
		return nil, fmt.Errorf("invalid confirmation range: maxConf (%d) must be >= minConf (%d)", maxConf, minConf)
	}
	utxos, err := a.coreClient.ListUnspent(minConf, maxConf)
	if err != nil {
		return nil, fmt.Errorf("failed to list unspent outputs: %w", err)
	}
	return utxos, nil
}

// LockUnspent locks or unlocks specified transaction outputs
// unlock: true to unlock, false to lock
// outputs: list of transaction outputs to lock/unlock
func (a *App) LockUnspent(unlock bool, outputs []core.OutPoint) error {
	if a.coreClient == nil {
		return fmt.Errorf("core client not initialized")
	}
	// Validate outputs
	if len(outputs) == 0 {
		return fmt.Errorf("no outputs specified")
	}
	for i, op := range outputs {
		if op.TxID == "" {
			return fmt.Errorf("invalid output at index %d: empty txid", i)
		}
	}
	err := a.coreClient.LockUnspent(unlock, outputs)
	if err != nil {
		return fmt.Errorf("failed to lock/unlock outputs: %w", err)
	}
	return nil
}

// ListLockUnspent returns list of temporarily locked outputs
func (a *App) ListLockUnspent() ([]core.OutPoint, error) {
	if a.coreClient == nil {
		return nil, fmt.Errorf("core client not initialized")
	}
	locked, err := a.coreClient.ListLockUnspent()
	if err != nil {
		return nil, fmt.Errorf("failed to list locked outputs: %w", err)
	}
	return locked, nil
}

// ==========================================
// Transaction Operations
// ==========================================

// GetTransactions returns recent transactions from the core client
func (a *App) GetTransactions(limit int) ([]core.Transaction, error) {
	if a.coreClient == nil {
		return nil, fmt.Errorf("core client not initialized")
	}
	transactions, err := a.coreClient.ListTransactions(limit, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get transactions: %w", err)
	}
	return transactions, nil
}

// GetTransactionsPage returns a paginated, filtered, and sorted page of wallet transactions
func (a *App) GetTransactionsPage(filter core.TransactionFilter) (*core.TransactionPage, error) {
	if a.coreClient == nil {
		return nil, fmt.Errorf("core client not initialized")
	}
	page, err := a.coreClient.ListTransactionsFiltered(filter)
	if err != nil {
		return nil, fmt.Errorf("failed to get transactions page: %w", err)
	}
	return &page, nil
}

// ExportFilteredTransactionsCSV generates CSV for all filtered transactions and opens a save dialog
func (a *App) ExportFilteredTransactionsCSV(filter core.TransactionFilter) (bool, error) {
	if a.coreClient == nil {
		return false, fmt.Errorf("core client not initialized")
	}
	csvContent, err := a.coreClient.ExportFilteredTransactionsCSV(filter)
	if err != nil {
		return false, fmt.Errorf("failed to export transactions: %w", err)
	}
	return a.SaveCSVFile(csvContent, "transactions.csv", "Export Transactions")
}

// GetRecentTransactions returns the 9 most recent transactions for the overview page
func (a *App) GetRecentTransactions() ([]core.Transaction, error) {
	return a.GetTransactions(9)
}

// GetTransactionDetails returns detailed information about a specific transaction
func (a *App) GetTransactionDetails(txid string) (*core.Transaction, error) {
	if a.coreClient == nil {
		return nil, fmt.Errorf("core client not initialized")
	}
	if txid == "" {
		return nil, fmt.Errorf("transaction ID cannot be empty")
	}
	tx, err := a.coreClient.GetTransaction(txid)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction details: %w", err)
	}
	return &tx, nil
}

// SendTransactionResult contains the result of a send transaction operation
type SendTransactionResult struct {
	TxID  string          `json:"txid,omitempty"`
	Error *core.SendError `json:"error,omitempty"`
}

// SendTransaction sends TWINS to an address
// Returns a SendTransactionResult with either txid on success or a structured SendError
func (a *App) SendTransaction(address string, amount float64) *SendTransactionResult {
	if a.coreClient == nil {
		return &SendTransactionResult{
			Error: core.NewSendError(core.SendErrNotInitialized, "Wallet is not ready. Please wait for initialization."),
		}
	}

	// Validate parameters
	if address == "" {
		return &SendTransactionResult{
			Error: core.NewSendError(core.SendErrInvalidAddress, "Please enter a recipient address."),
		}
	}
	if amount <= 0 {
		return &SendTransactionResult{
			Error: core.NewSendError(core.SendErrInvalidAmount, "Please enter a positive amount."),
		}
	}

	// Check wallet status before attempting to send
	walletInfo, err := a.coreClient.GetWalletInfo()
	if err != nil {
		return &SendTransactionResult{
			Error: core.TranslateSendError(err),
		}
	}

	// If wallet is encrypted and locked, return a specific error for the UI
	if walletInfo.Encrypted && !walletInfo.Unlocked {
		return &SendTransactionResult{
			Error: core.NewSendError(core.SendErrWalletLocked, "Wallet is locked. Please unlock your wallet to send transactions."),
		}
	}

	// Send the transaction via core client
	txid, err := a.coreClient.SendToAddress(address, amount, "")
	if err != nil {
		return &SendTransactionResult{
			Error: core.TranslateSendError(err),
		}
	}

	return &SendTransactionResult{TxID: txid}
}

// SendTransactionOptions contains options for advanced transaction sending
type SendTransactionOptions struct {
	// SelectedUTXOs are the specific UTXOs to use (coin control)
	// Format: ["txid:vout", ...]
	SelectedUTXOs []string `json:"selectedUtxos"`

	// ChangeAddress overrides the default change address
	ChangeAddress string `json:"changeAddress"`

	// SplitCount splits each output into multiple UTXOs (for staking optimization)
	SplitCount int `json:"splitCount"`

	// FeeRate is the user-selected fee rate in TWINS/kB
	// If 0 or omitted, uses wallet default fee rate
	FeeRate float64 `json:"feeRate"`
}

// SendTransactionWithOptions sends TWINS with advanced options
// Supports coin control (specific UTXO selection), custom change address, and UTXO splitting
// Returns a SendTransactionResult with either txid on success or a structured SendError
func (a *App) SendTransactionWithOptions(address string, amount float64, opts *SendTransactionOptions) *SendTransactionResult {
	if a.coreClient == nil {
		return &SendTransactionResult{
			Error: core.NewSendError(core.SendErrNotInitialized, "Wallet is not ready. Please wait for initialization."),
		}
	}

	// Validate parameters
	if address == "" {
		return &SendTransactionResult{
			Error: core.NewSendError(core.SendErrInvalidAddress, "Please enter a recipient address."),
		}
	}
	if amount <= 0 {
		return &SendTransactionResult{
			Error: core.NewSendError(core.SendErrInvalidAmount, "Please enter a positive amount."),
		}
	}

	// Check wallet status before attempting to send
	walletInfo, err := a.coreClient.GetWalletInfo()
	if err != nil {
		return &SendTransactionResult{
			Error: core.TranslateSendError(err),
		}
	}

	if walletInfo.Encrypted && !walletInfo.Unlocked {
		return &SendTransactionResult{
			Error: core.NewSendError(core.SendErrWalletLocked, "Wallet is locked. Please unlock your wallet to send transactions."),
		}
	}

	// Convert options to core.SendOptions
	var coreOpts *core.SendOptions
	if opts != nil {
		coreOpts = &core.SendOptions{
			SelectedUTXOs: opts.SelectedUTXOs,
			ChangeAddress: opts.ChangeAddress,
			SplitCount:    opts.SplitCount,
			FeeRate:       opts.FeeRate,
		}
	}

	// Send the transaction with options
	txid, err := a.coreClient.SendToAddressWithOptions(address, amount, "", coreOpts)
	if err != nil {
		return &SendTransactionResult{
			Error: core.TranslateSendError(err),
		}
	}

	return &SendTransactionResult{TxID: txid}
}

// SendTransactionMultiRequest represents a multi-recipient transaction request
type SendTransactionMultiRequest struct {
	// Recipients maps addresses to amounts in TWINS
	Recipients map[string]float64 `json:"recipients"`

	// Options for advanced transaction features
	Options *SendTransactionOptions `json:"options,omitempty"`
}

// SendTransactionMulti sends TWINS to multiple recipients in a single transaction
// Returns a SendTransactionResult with either txid on success or a structured SendError
func (a *App) SendTransactionMulti(req SendTransactionMultiRequest) *SendTransactionResult {
	if a.coreClient == nil {
		return &SendTransactionResult{
			Error: core.NewSendError(core.SendErrNotInitialized, "Wallet is not ready. Please wait for initialization."),
		}
	}

	// Validate recipients
	if len(req.Recipients) == 0 {
		return &SendTransactionResult{
			Error: core.NewSendError(core.SendErrNoRecipients, "Please add at least one recipient."),
		}
	}

	for addr, amount := range req.Recipients {
		if addr == "" {
			return &SendTransactionResult{
				Error: core.NewSendError(core.SendErrInvalidAddress, "One or more recipients have an empty address."),
			}
		}
		if amount <= 0 {
			return &SendTransactionResult{
				Error: core.NewSendError(core.SendErrInvalidAmount, "All amounts must be positive."),
			}
		}
	}

	// Check wallet status before attempting to send
	walletInfo, err := a.coreClient.GetWalletInfo()
	if err != nil {
		return &SendTransactionResult{
			Error: core.TranslateSendError(err),
		}
	}

	if walletInfo.Encrypted && !walletInfo.Unlocked {
		return &SendTransactionResult{
			Error: core.NewSendError(core.SendErrWalletLocked, "Wallet is locked. Please unlock your wallet to send transactions."),
		}
	}

	// Convert options to core.SendOptions
	var coreOpts *core.SendOptions
	if req.Options != nil {
		coreOpts = &core.SendOptions{
			SelectedUTXOs: req.Options.SelectedUTXOs,
			ChangeAddress: req.Options.ChangeAddress,
			SplitCount:    req.Options.SplitCount,
			FeeRate:       req.Options.FeeRate,
		}
	}

	// Send the transaction with multiple recipients
	txid, err := a.coreClient.SendMany(req.Recipients, "", coreOpts)
	if err != nil {
		return &SendTransactionResult{
			Error: core.TranslateSendError(err),
		}
	}

	return &SendTransactionResult{TxID: txid}
}

// WalletStatus represents the current wallet encryption and lock state
type WalletStatus struct {
	Encrypted bool `json:"encrypted"` // Whether wallet is encrypted
	Unlocked  bool `json:"unlocked"`  // Whether wallet is unlocked (can send)
}

// GetWalletStatus returns the current wallet encryption and lock status
// Used by frontend to determine if unlock prompt is needed before sending
func (a *App) GetWalletStatus() (*WalletStatus, error) {
	if a.coreClient == nil {
		return nil, fmt.Errorf("core client not initialized")
	}

	walletInfo, err := a.coreClient.GetWalletInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to get wallet info: %w", err)
	}

	return &WalletStatus{
		Encrypted: walletInfo.Encrypted,
		Unlocked:  walletInfo.Unlocked,
	}, nil
}

// EstimateFee estimates the fee rate for confirmation within the specified number of blocks
func (a *App) EstimateFee(blocks int) (float64, error) {
	if a.coreClient == nil {
		return 0, fmt.Errorf("core client not initialized")
	}

	feeRate, err := a.coreClient.EstimateFee(blocks)
	if err != nil {
		return 0, fmt.Errorf("failed to estimate fee: %w", err)
	}

	return feeRate, nil
}

// FeeEstimateResult contains detailed fee estimation for GUI display
type FeeEstimateResult struct {
	Fee        float64 `json:"fee"`        // Estimated fee in TWINS
	InputCount int     `json:"inputCount"` // Number of inputs that would be used
	TxSize     int     `json:"txSize"`     // Estimated transaction size in bytes
}

// FeeEstimateRequest represents a request to estimate transaction fee
type FeeEstimateRequest struct {
	// Recipients maps addresses to amounts in TWINS
	Recipients map[string]float64 `json:"recipients"`

	// Options for transaction parameters (coin control, fee rate, split count)
	Options *SendTransactionOptions `json:"options,omitempty"`
}

// EstimateTransactionFee estimates the transaction fee based on recipients and options
// This works even when the wallet is locked (no signing required)
// Returns the estimated fee in TWINS, number of inputs that would be used, and tx size in bytes
func (a *App) EstimateTransactionFee(req FeeEstimateRequest) (*FeeEstimateResult, error) {
	if a.coreClient == nil {
		return nil, fmt.Errorf("wallet not initialized")
	}

	// Validate recipients
	if len(req.Recipients) == 0 {
		return nil, fmt.Errorf("no recipients specified")
	}

	for addr, amount := range req.Recipients {
		if addr == "" {
			return nil, fmt.Errorf("recipient address cannot be empty")
		}
		if amount <= 0 {
			return nil, fmt.Errorf("amount must be positive")
		}
	}

	// Convert options to core.SendOptions
	var coreOpts *core.SendOptions
	if req.Options != nil {
		coreOpts = &core.SendOptions{
			SelectedUTXOs: req.Options.SelectedUTXOs,
			ChangeAddress: req.Options.ChangeAddress,
			SplitCount:    req.Options.SplitCount,
			FeeRate:       req.Options.FeeRate,
		}
	}

	// Call core client's EstimateTransactionFee
	result, err := a.coreClient.EstimateTransactionFee(req.Recipients, coreOpts)
	if err != nil {
		return nil, fmt.Errorf("fee estimation failed: %w", err)
	}

	return &FeeEstimateResult{
		Fee:        result.Fee,
		InputCount: result.InputCount,
		TxSize:     result.TxSize,
	}, nil
}
