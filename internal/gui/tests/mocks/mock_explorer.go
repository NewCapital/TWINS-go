package mocks

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/twins-dev/twins-core/internal/gui/core"
)

// ==========================================
// Explorer Operations
// ==========================================

// GetLatestBlocks returns the most recent blocks for explorer view
func (m *MockCoreClient) GetLatestBlocks(limit, offset int) ([]core.BlockSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return nil, fmt.Errorf("core client not running")
	}

	// Default values
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}

	blocks := make([]core.BlockSummary, 0, limit)

	// Start from the tip and go backwards
	startHeight := m.currentHeight - int64(offset)
	if startHeight < 0 {
		startHeight = 0
	}

	for height := startHeight; height >= 0 && len(blocks) < limit; height-- {
		blockHash, exists := m.blocksByHeight[height]
		if !exists {
			continue
		}

		block, exists := m.blocks[blockHash]
		if !exists {
			continue
		}

		// Determine if PoS based on flags
		isPoS := strings.Contains(block.Flags, "proof-of-stake")

		// Calculate reward (mock: varies by height)
		reward := 100.0 + float64(height%50) // Base reward + variation

		summary := core.BlockSummary{
			Height:  height,
			Hash:    blockHash,
			Time:    block.Time,
			TxCount: len(block.Transactions) + 1, // +1 for coinbase/coinstake
			Size:    block.Size,
			IsPoS:   isPoS,
			Reward:  reward,
		}

		blocks = append(blocks, summary)
	}

	return blocks, nil
}

// GetExplorerBlock returns detailed block information by hash or height
func (m *MockCoreClient) GetExplorerBlock(query string) (core.BlockDetail, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return core.BlockDetail{}, fmt.Errorf("core client not running")
	}

	var blockHash string
	var height int64

	// Check if query is a height (numeric) or hash (hex)
	if isNumeric(query) {
		h, err := strconv.ParseInt(query, 10, 64)
		if err != nil {
			return core.BlockDetail{}, fmt.Errorf("invalid block height: %s", query)
		}
		height = h

		hash, exists := m.blocksByHeight[height]
		if !exists {
			return core.BlockDetail{}, fmt.Errorf("block not found at height %d", height)
		}
		blockHash = hash
	} else if isHexHash(query) {
		blockHash = query
		// Find height from hash
		for h, hash := range m.blocksByHeight {
			if hash == blockHash {
				height = h
				break
			}
		}
	} else {
		return core.BlockDetail{}, fmt.Errorf("invalid query: must be block height or hash")
	}

	block, exists := m.blocks[blockHash]
	if !exists {
		return core.BlockDetail{}, fmt.Errorf("block not found: %s", query)
	}

	// Generate mock transaction IDs for this block
	txIDs := make([]string, 0)
	txIDs = append(txIDs, m.generateTxHashForBlock(height, 0)) // Coinstake tx

	// Add some regular transactions
	numTxs := 2 + int(height%5) // 2-6 transactions per block
	for i := 1; i < numTxs; i++ {
		txIDs = append(txIDs, m.generateTxHashForBlock(height, i))
	}

	// Determine PoS info
	isPoS := strings.Contains(block.Flags, "proof-of-stake")
	stakeReward := 80.0  // 80% to staker
	mnReward := 20.0     // 20% to masternode
	if height < 100000 {
		// Early blocks had different reward structure
		stakeReward = 90.0
		mnReward = 10.0
	}

	// Generate addresses (deterministic based on height)
	stakerAddr := fmt.Sprintf("D%s", m.generateHashFromSeed(height*2)[:33])
	mnAddr := fmt.Sprintf("D%s", m.generateHashFromSeed(height*3)[:33])

	detail := core.BlockDetail{
		Block:             *block,
		TxIDs:             txIDs,
		IsPoS:             isPoS,
		StakeReward:       stakeReward,
		MasternodeReward:  mnReward,
		StakerAddress:     stakerAddr,
		MasternodeAddress: mnAddr,
		TotalReward:       stakeReward + mnReward,
	}

	return detail, nil
}

// GetExplorerTransaction returns detailed transaction information
func (m *MockCoreClient) GetExplorerTransaction(txid string) (core.ExplorerTransaction, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return core.ExplorerTransaction{}, fmt.Errorf("core client not running")
	}

	// Check if we have this transaction in our mock data
	tx, exists := m.transactions[txid]
	if exists {
		return m.convertToExplorerTx(tx), nil
	}

	// Generate a mock transaction for any valid-looking txid
	if !isHexHash(txid) {
		return core.ExplorerTransaction{}, fmt.Errorf("invalid transaction hash")
	}

	// Create a mock transaction
	mockTx := m.generateMockExplorerTx(txid)
	return mockTx, nil
}

// GetAddressInfo returns information about an address
func (m *MockCoreClient) GetAddressInfo(address string, limit int) (core.AddressInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return core.AddressInfo{}, fmt.Errorf("core client not running")
	}

	// Validate address format (basic check)
	if !isValidTWINSAddress(address) {
		return core.AddressInfo{}, fmt.Errorf("invalid address format")
	}

	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}

	// Generate deterministic mock data based on address
	seed := hashString(address)

	// Generate balance (deterministic from address)
	balance := float64(seed%100000) / 100.0
	totalReceived := balance * 1.5
	totalSent := totalReceived - balance
	txCount := int(seed%50) + 5

	// Generate transaction history
	transactions := make([]core.AddressTx, 0, limit)
	for i := 0; i < limit && i < txCount; i++ {
		txSeed := seed + int64(i)
		isReceive := txSeed%2 == 0
		amount := float64(txSeed%10000) / 100.0
		if !isReceive {
			amount = -amount
		}

		tx := core.AddressTx{
			TxID:          m.generateHashFromSeed(txSeed),
			BlockHeight:   m.currentHeight - int64(i*10),
			Time:          time.Now().Add(-time.Duration(i*10) * time.Minute),
			Amount:        amount,
			Confirmations: i*10 + 1,
		}
		transactions = append(transactions, tx)
	}

	// Generate UTXOs
	utxos := make([]core.AddressUTXO, 0)
	numUtxos := int(seed%5) + 1
	for i := 0; i < numUtxos; i++ {
		utxoSeed := seed + int64(100+i)
		utxo := core.AddressUTXO{
			TxID:          m.generateHashFromSeed(utxoSeed),
			Vout:          uint32(i),
			Amount:        float64(utxoSeed%5000) / 100.0,
			Confirmations: int(utxoSeed%1000) + 1,
			BlockHeight:   m.currentHeight - int64(utxoSeed%1000),
		}
		utxos = append(utxos, utxo)
	}

	info := core.AddressInfo{
		Address:            address,
		Balance:            balance,
		TotalReceived:      totalReceived,
		TotalSent:          totalSent,
		TxCount:            txCount,
		UnconfirmedBalance: 0,
		Transactions:       transactions,
		UTXOs:              utxos,
	}

	return info, nil
}

// SearchExplorer searches for a block, transaction, or address
func (m *MockCoreClient) SearchExplorer(query string) (core.SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return core.SearchResult{}, fmt.Errorf("core client not running")
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return core.SearchResult{
			Type:  core.SearchResultNotFound,
			Query: query,
			Error: "empty search query",
		}, nil
	}

	// Check if it's a block height
	if isNumeric(query) {
		height, _ := strconv.ParseInt(query, 10, 64)
		if height >= 0 && height <= m.currentHeight {
			// Release lock before calling GetExplorerBlock (it acquires its own lock)
			m.mu.RUnlock()
			block, err := m.GetExplorerBlock(query)
			m.mu.RLock()
			if err == nil {
				return core.SearchResult{
					Type:  core.SearchResultBlock,
					Query: query,
					Block: &block,
				}, nil
			}
		}
	}

	// Check if it's a valid hex hash (could be block or tx)
	if isHexHash(query) {
		// Try as block hash first
		if _, exists := m.blocks[query]; exists {
			m.mu.RUnlock()
			block, err := m.GetExplorerBlock(query)
			m.mu.RLock()
			if err == nil {
				return core.SearchResult{
					Type:  core.SearchResultBlock,
					Query: query,
					Block: &block,
				}, nil
			}
		}

		// Try as transaction hash
		m.mu.RUnlock()
		tx, err := m.GetExplorerTransaction(query)
		m.mu.RLock()
		if err == nil {
			return core.SearchResult{
				Type:        core.SearchResultTransaction,
				Query:       query,
				Transaction: &tx,
			}, nil
		}
	}

	// Check if it's a valid TWINS address
	if isValidTWINSAddress(query) {
		m.mu.RUnlock()
		addr, err := m.GetAddressInfo(query, 25)
		m.mu.RLock()
		if err == nil {
			return core.SearchResult{
				Type:    core.SearchResultAddress,
				Query:   query,
				Address: &addr,
			}, nil
		}
	}

	return core.SearchResult{
		Type:  core.SearchResultNotFound,
		Query: query,
		Error: "no results found",
	}, nil
}

// Helper functions

// convertToExplorerTx converts a wallet Transaction to ExplorerTransaction
func (m *MockCoreClient) convertToExplorerTx(tx *core.Transaction) core.ExplorerTransaction {
	// Generate mock inputs and outputs
	inputs := []core.TxInput{
		{
			TxID:       m.generateHashFromSeed(hashString(tx.TxID)),
			Vout:       0,
			Address:    tx.Address,
			Amount:     tx.Amount + tx.Fee,
			IsCoinbase: tx.Type == core.TxTypeGenerated,
		},
	}

	outputs := []core.TxOutput{
		{
			Index:      0,
			Address:    tx.Address,
			Amount:     tx.Amount,
			ScriptType: "pubkeyhash",
			IsSpent:    tx.Confirmations > 0,
		},
	}

	isCoinbase := tx.Type == core.TxTypeGenerated
	isCoinStake := tx.Type == core.TxTypeStakeMint

	return core.ExplorerTransaction{
		TxID:          tx.TxID,
		BlockHash:     tx.BlockHash,
		BlockHeight:   tx.BlockHeight,
		Confirmations: tx.Confirmations,
		Time:          tx.Time,
		Size:          250, // Mock size
		Fee:           tx.Fee,
		IsCoinbase:    isCoinbase,
		IsCoinStake:   isCoinStake,
		Inputs:        inputs,
		Outputs:       outputs,
		TotalInput:    tx.Amount + tx.Fee,
		TotalOutput:   tx.Amount,
	}
}

// generateMockExplorerTx generates a mock explorer transaction for any txid
func (m *MockCoreClient) generateMockExplorerTx(txid string) core.ExplorerTransaction {
	seed := hashString(txid)

	numInputs := int(seed%3) + 1
	numOutputs := int(seed%4) + 1

	totalInput := float64(seed%100000) / 100.0
	fee := 0.0001
	totalOutput := totalInput - fee

	inputs := make([]core.TxInput, numInputs)
	for i := 0; i < numInputs; i++ {
		inputs[i] = core.TxInput{
			TxID:       m.generateHashFromSeed(seed + int64(i*100)),
			Vout:       uint32(i),
			Address:    fmt.Sprintf("D%s", m.generateHashFromSeed(seed+int64(i*200))[:33]),
			Amount:     totalInput / float64(numInputs),
			IsCoinbase: false,
		}
	}

	outputs := make([]core.TxOutput, numOutputs)
	outputAmount := totalOutput / float64(numOutputs)
	for i := 0; i < numOutputs; i++ {
		outputs[i] = core.TxOutput{
			Index:      uint32(i),
			Address:    fmt.Sprintf("D%s", m.generateHashFromSeed(seed+int64(i*300))[:33]),
			Amount:     outputAmount,
			ScriptType: "pubkeyhash",
			IsSpent:    seed%2 == 0,
		}
	}

	blockHeight := m.currentHeight - int64(seed%10000)
	if blockHeight < 0 {
		blockHeight = 0
	}

	return core.ExplorerTransaction{
		TxID:          txid,
		BlockHash:     m.generateHashFromSeed(seed + 1000),
		BlockHeight:   blockHeight,
		Confirmations: int(m.currentHeight - blockHeight),
		Time:          time.Now().Add(-time.Duration(seed%10000) * time.Minute),
		Size:          200 + int(seed%500),
		Fee:           fee,
		IsCoinbase:    false,
		IsCoinStake:   false,
		Inputs:        inputs,
		Outputs:       outputs,
		TotalInput:    totalInput,
		TotalOutput:   totalOutput,
	}
}

// generateTxHashForBlock generates a deterministic tx hash for a block
func (m *MockCoreClient) generateTxHashForBlock(height int64, txIndex int) string {
	return m.generateHashFromSeed(height*1000 + int64(txIndex))
}

// generateHashFromSeed generates a deterministic 64-char hex hash from a seed
func (m *MockCoreClient) generateHashFromSeed(seed int64) string {
	// Use seed to generate a deterministic hash-like string
	hash := fmt.Sprintf("%016x%016x%016x%016x", seed, seed*31, seed*37, seed*41)
	if len(hash) > 64 {
		hash = hash[:64]
	}
	return hash
}

// isNumeric checks if a string is a valid number
func isNumeric(s string) bool {
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}

// isHexHash checks if a string looks like a 64-character hex hash
func isHexHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	matched, _ := regexp.MatchString("^[a-fA-F0-9]{64}$", s)
	return matched
}

// isValidTWINSAddress checks if a string looks like a valid TWINS address
func isValidTWINSAddress(s string) bool {
	// TWINS addresses start with 'D' and are 34 characters
	if len(s) != 34 {
		return false
	}
	if s[0] != 'D' {
		return false
	}
	// Basic base58 character check
	matched, _ := regexp.MatchString("^[123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz]+$", s)
	return matched
}

// hashString generates a simple numeric hash from a string
func hashString(s string) int64 {
	var hash int64
	for i, c := range s {
		hash += int64(c) * int64(i+1)
	}
	if hash < 0 {
		hash = -hash
	}
	return hash
}

// GetAddressTransactions returns a page of transactions for an address
func (m *MockCoreClient) GetAddressTransactions(address string, limit, offset int) (core.AddressTxPage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return core.AddressTxPage{}, fmt.Errorf("core client not running")
	}

	if !isValidTWINSAddress(address) {
		return core.AddressTxPage{}, fmt.Errorf("invalid address format")
	}

	if limit <= 0 {
		limit = 50
	}

	// Generate deterministic mock data
	seed := hashString(address)
	total := int(seed%100) + 10

	transactions := make([]core.AddressTx, 0, limit)
	for i := offset; i < offset+limit && i < total; i++ {
		txSeed := seed + int64(i)
		isReceive := txSeed%2 == 0
		amount := float64(txSeed%10000) / 100.0
		if !isReceive {
			amount = -amount
		}

		tx := core.AddressTx{
			TxID:          m.generateHashFromSeed(txSeed),
			BlockHeight:   m.currentHeight - int64(i*10),
			Time:          time.Now().Add(-time.Duration(i*10) * time.Minute),
			Amount:        amount,
			Confirmations: i*10 + 1,
		}
		transactions = append(transactions, tx)
	}

	return core.AddressTxPage{
		Transactions: transactions,
		Total:        total,
		HasMore:      offset+len(transactions) < total,
	}, nil
}
