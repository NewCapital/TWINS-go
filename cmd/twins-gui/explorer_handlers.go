package main

import (
	"fmt"

	"github.com/twins-dev/twins-core/internal/gui/core"
)

// ==========================================
// Explorer Operations
// ==========================================

// GetLatestBlocks returns the most recent blocks for the explorer view.
// limit: maximum number of blocks to return (default 25, max 100)
// offset: number of blocks to skip from the tip (for pagination)
func (a *App) GetLatestBlocks(limit, offset int) ([]core.BlockSummary, error) {
	if a.coreClient == nil {
		return nil, fmt.Errorf("core client not initialized")
	}

	blocks, err := a.coreClient.GetLatestBlocks(limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest blocks: %w", err)
	}

	return blocks, nil
}

// GetExplorerBlock returns detailed block information by hash or height.
// query can be a block hash (64 hex chars) or block height (number).
func (a *App) GetExplorerBlock(query string) (*core.BlockDetail, error) {
	if a.coreClient == nil {
		return nil, fmt.Errorf("core client not initialized")
	}

	block, err := a.coreClient.GetExplorerBlock(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %w", err)
	}

	return &block, nil
}

// GetExplorerTransaction returns detailed transaction information.
// txid: the transaction hash
func (a *App) GetExplorerTransaction(txid string) (*core.ExplorerTransaction, error) {
	if a.coreClient == nil {
		return nil, fmt.Errorf("core client not initialized")
	}

	tx, err := a.coreClient.GetExplorerTransaction(txid)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction: %w", err)
	}

	return &tx, nil
}

// GetExplorerAddressBasic returns the minimal, O(1) subset of address
// information (Address only) for the Explorer Address Detail hero header.
// No storage access beyond crypto.DecodeAddress validation — renders the
// hero header (address text + QR) immediately while balance and stats
// fetches are still in flight.
func (a *App) GetExplorerAddressBasic(address string) (*core.AddressBasic, error) {
	if a.coreClient == nil {
		return nil, fmt.Errorf("core client not initialized")
	}

	info, err := a.coreClient.GetAddressBasic(address)
	if err != nil {
		return nil, fmt.Errorf("failed to get address basic: %w", err)
	}

	return &info, nil
}

// GetExplorerAddressBalance returns the address's current spendable
// balance via a GetUTXOsByAddress prefix scan. Cost is O(U) where
// U = current UTXO count; called separately from GetExplorerAddressBasic
// so the hero header is not blocked by the UTXO scan.
func (a *App) GetExplorerAddressBalance(address string) (*core.AddressBalance, error) {
	if a.coreClient == nil {
		return nil, fmt.Errorf("core client not initialized")
	}

	info, err := a.coreClient.GetAddressBalance(address)
	if err != nil {
		return nil, fmt.Errorf("failed to get address balance: %w", err)
	}

	return &info, nil
}

// GetExplorerAddressStats returns the expensive aggregate statistics for
// an address (TxCount, TotalReceived, TotalSent, FirstSeen, LastSeen).
// Cost is O(n) storage reads; called separately from the basic fetch so
// the hero card does not block on this work.
func (a *App) GetExplorerAddressStats(address string) (*core.AddressStats, error) {
	if a.coreClient == nil {
		return nil, fmt.Errorf("core client not initialized")
	}

	stats, err := a.coreClient.GetAddressStats(address)
	if err != nil {
		return nil, fmt.Errorf("failed to get address stats: %w", err)
	}

	return &stats, nil
}

// ExplorerSearch searches for a block, transaction, or address.
// query: can be block hash, block height, transaction hash, or address
func (a *App) ExplorerSearch(query string) (*core.SearchResult, error) {
	if a.coreClient == nil {
		return nil, fmt.Errorf("core client not initialized")
	}

	result, err := a.coreClient.SearchExplorer(query)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}

	return &result, nil
}

// GetAddressTransactions returns a page of transactions for an address.
// address: the TWINS address
// limit: number of transactions per batch
// offset: starting position (0-based, from most recent)
func (a *App) GetAddressTransactions(address string, limit, offset int) (*core.AddressTxPage, error) {
	if a.coreClient == nil {
		return nil, fmt.Errorf("core client not initialized")
	}

	page, err := a.coreClient.GetAddressTransactions(address, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get address transactions: %w", err)
	}

	return &page, nil
}

// GetAddressUTXOs returns a page of unspent outputs for an address.
// address: the TWINS address
// limit: number of UTXOs per batch
// offset: starting position (0-based, from newest first by confirmations)
func (a *App) GetAddressUTXOs(address string, limit, offset int) (*core.AddressUTXOPage, error) {
	if a.coreClient == nil {
		return nil, fmt.Errorf("core client not initialized")
	}

	page, err := a.coreClient.GetAddressUTXOs(address, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get address utxos: %w", err)
	}

	return &page, nil
}
