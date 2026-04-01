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

// GetExplorerAddress returns information about an address including balance and history.
// address: the TWINS address to look up
// limit: maximum number of transactions to include in history (default 25)
func (a *App) GetExplorerAddress(address string, limit int) (*core.AddressInfo, error) {
	if a.coreClient == nil {
		return nil, fmt.Errorf("core client not initialized")
	}

	info, err := a.coreClient.GetAddressInfo(address, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get address info: %w", err)
	}

	return &info, nil
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
