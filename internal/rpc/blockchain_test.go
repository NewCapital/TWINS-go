// Copyright (c) 2025 The TWINS Core developers
// Distributed under the MIT software license

package rpc

import (
	"encoding/json"
	"testing"

	"github.com/twins-dev/twins-core/pkg/types"
)

// Mock blockchain interface for testing
type mockBlockchain struct {
	height         uint32
	bestBlock      *types.Block
	blocks         map[types.Hash]*types.Block
	blocksByHeight map[uint32]*types.Block
	moneySupply    map[uint32]int64 // Money supply per height
}

func (m *mockBlockchain) GetBlockHeight() (uint32, error) {
	return m.height, nil
}

func (m *mockBlockchain) GetBestBlock() (*types.Block, error) {
	return m.bestBlock, nil
}

func (m *mockBlockchain) GetBlock(hash types.Hash) (*types.Block, error) {
	if block, ok := m.blocks[hash]; ok {
		return block, nil
	}
	return nil, NewError(CodeBlockNotFound, "Block not found", nil)
}

func (m *mockBlockchain) GetBlockByHeight(height uint32) (*types.Block, error) {
	if block, ok := m.blocksByHeight[height]; ok {
		return block, nil
	}
	return nil, NewError(CodeBlockNotFound, "Block not found", nil)
}

func (m *mockBlockchain) GetBlockHash(height uint32) (types.Hash, error) {
	if block, ok := m.blocksByHeight[height]; ok {
		return block.Hash(), nil
	}
	return types.ZeroHash, NewError(CodeBlockNotFound, "Block not found", nil)
}

func (m *mockBlockchain) GetBlockHeightByHash(hash types.Hash) (uint32, error) {
	for height, block := range m.blocksByHeight {
		if block.Hash() == hash {
			return height, nil
		}
	}
	return 0, NewError(CodeBlockNotFound, "Block not found", nil)
}

func (m *mockBlockchain) GetBestBlockHash() (types.Hash, error) {
	if m.bestBlock != nil {
		return m.bestBlock.Hash(), nil
	}
	return types.ZeroHash, nil
}

func (m *mockBlockchain) GetBestHeight() (uint32, error) {
	return m.height, nil
}

func (m *mockBlockchain) GetChainWork() (string, error) {
	return "0", nil
}

func (m *mockBlockchain) GetDifficulty() (float64, error) {
	return 1.0, nil
}

func (m *mockBlockchain) GetChainTips() ([]ChainTip, error) {
	return nil, nil
}

func (m *mockBlockchain) GetBlockCount() (int64, error) {
	return int64(m.height), nil
}

func (m *mockBlockchain) ValidateBlock(block *types.Block) error {
	return nil
}

func (m *mockBlockchain) ProcessBlock(block *types.Block) error {
	return nil
}

func (m *mockBlockchain) GetUTXO(outpoint types.Outpoint) (*types.TxOutput, error) {
	return nil, nil
}

func (m *mockBlockchain) GetUTXOSet() (map[types.Outpoint]*types.TxOutput, error) {
	return nil, nil
}

func (m *mockBlockchain) GetTransaction(hash types.Hash) (*types.Transaction, error) {
	return nil, nil
}

func (m *mockBlockchain) GetTransactionBlock(hash types.Hash) (*types.Block, error) {
	return nil, nil
}

func (m *mockBlockchain) GetRawTransaction(hash types.Hash) ([]byte, error) {
	return nil, nil
}

func (m *mockBlockchain) GetChainParams() *types.ChainParams {
	return nil
}

func (m *mockBlockchain) GetVerificationProgress() float64 {
	return 1.0
}

func (m *mockBlockchain) GetMoneySupply(height uint32) (int64, error) {
	if m.moneySupply != nil {
		if supply, ok := m.moneySupply[height]; ok {
			return supply, nil
		}
	}
	return 0, nil
}

func (m *mockBlockchain) IsInitialBlockDownload() bool {
	return false
}

func (m *mockBlockchain) InvalidateBlock(hash types.Hash) error {
	return nil
}

func (m *mockBlockchain) ReconsiderBlock(hash types.Hash) error {
	return nil
}

func (m *mockBlockchain) AddCheckpoint(height uint32, hash types.Hash) error {
	return nil
}

// Helper to create a test block
func createTestBlock(height uint32, numTx int) *types.Block {
	block := &types.Block{
		Header: &types.BlockHeader{
			Version:    1,
			Timestamp:  uint32(1700000000 + height*600), // 10 minutes per block
			Bits:       0x1d00ffff,
			Nonce:      12345,
			MerkleRoot: types.ZeroHash,
		},
		Transactions: make([]*types.Transaction, numTx),
	}

	// Create coinbase transaction
	if numTx > 0 {
		block.Transactions[0] = &types.Transaction{
			Version: 1,
			Inputs: []*types.TxInput{
				{
					PreviousOutput: types.Outpoint{Hash: types.ZeroHash, Index: 0xffffffff},
					ScriptSig:      []byte{},
					Sequence:       0xffffffff,
				},
			},
			Outputs: []*types.TxOutput{
				{
					Value:        50 * 1e8, // 50 TWINS
					ScriptPubKey: []byte{},
				},
			},
			LockTime: 0,
		}

		// Create regular transactions for the rest
		for i := 1; i < numTx; i++ {
			block.Transactions[i] = &types.Transaction{
				Version: 1,
				Inputs: []*types.TxInput{
					{
						PreviousOutput: types.Outpoint{
							Hash:  types.ZeroHash, // Dummy input
							Index: uint32(i),
						},
						ScriptSig: []byte{byte(i)},
						Sequence:  0xffffffff,
					},
				},
				Outputs: []*types.TxOutput{
					{
						Value:        1 * 1e8, // 1 TWINS
						ScriptPubKey: []byte{byte(i)},
					},
				},
				LockTime: 0,
			}
		}
	}

	return block
}

// Test getblockcount
func TestHandleGetBlockCount(t *testing.T) {
	mock := &mockBlockchain{
		height: 12345,
	}

	server := &Server{
		blockchain: mock,
	}

	req := &Request{
		JSONRPC: "2.0",
		Method:  "getblockcount",
		Params:  json.RawMessage("[]"),
		ID:      json.RawMessage("1"),
	}

	resp := server.handleGetBlockCount(req)

	if resp.Error != nil {
		t.Fatalf("Expected no error, got: %v", resp.Error)
	}

	height, ok := resp.Result.(uint32)
	if !ok {
		t.Fatalf("Expected uint32 result, got: %T", resp.Result)
	}

	if height != 12345 {
		t.Errorf("Expected height 12345, got: %d", height)
	}
}

// Test getbestblockhash
func TestHandleGetBestBlockHash(t *testing.T) {
	testBlock := createTestBlock(100, 1)
	mock := &mockBlockchain{
		bestBlock: testBlock,
	}

	server := &Server{
		blockchain: mock,
	}

	req := &Request{
		JSONRPC: "2.0",
		Method:  "getbestblockhash",
		Params:  json.RawMessage("[]"),
		ID:      json.RawMessage("1"),
	}

	resp := server.handleGetBestBlockHash(req)

	if resp.Error != nil {
		t.Fatalf("Expected no error, got: %v", resp.Error)
	}

	hashStr, ok := resp.Result.(string)
	if !ok {
		t.Fatalf("Expected string result, got: %T", resp.Result)
	}

	if hashStr != testBlock.Hash().String() {
		t.Errorf("Expected hash %s, got: %s", testBlock.Hash().String(), hashStr)
	}
}

// Test getblock with all required fields including mediantime and moneysupply
func TestHandleGetBlock(t *testing.T) {
	testBlock := createTestBlock(100, 2)
	blockHash := testBlock.Hash()

	mock := &mockBlockchain{
		height: 105,
		blocks: map[types.Hash]*types.Block{
			blockHash: testBlock,
		},
		blocksByHeight: map[uint32]*types.Block{
			100: testBlock,
			101: createTestBlock(101, 1),
		},
		moneySupply: map[uint32]int64{
			100: 500000000000, // 5000 TWINS at height 100
		},
	}

	// Add more blocks for median time calculation
	for i := uint32(90); i < 100; i++ {
		mock.blocksByHeight[i] = createTestBlock(i, 1)
	}

	server := &Server{
		blockchain: mock,
		chainParams: &types.ChainParams{
			BlockReward: 50 * 1e8,
		},
	}

	// Test verbose=1
	params, _ := json.Marshal([]interface{}{blockHash.String(), 1})
	req := &Request{
		JSONRPC: "2.0",
		Method:  "getblock",
		Params:  params,
		ID:      json.RawMessage("1"),
	}

	resp := server.handleGetBlock(req)

	if resp.Error != nil {
		t.Fatalf("Expected no error, got: %v", resp.Error)
	}

	blockInfo, ok := resp.Result.(*BlockInfo)
	if !ok {
		t.Fatalf("Expected *BlockInfo result, got: %T", resp.Result)
	}

	// Verify all required fields
	if blockInfo.Hash == "" {
		t.Error("Missing hash field")
	}
	if blockInfo.Height == 0 {
		t.Error("Missing height field")
	}
	if blockInfo.MedianTime == 0 {
		t.Error("Missing mediantime field - NEW FIELD")
	}
	if blockInfo.MoneySupply == 0 {
		t.Error("Missing moneysupply field - NEW FIELD")
	}
	if len(blockInfo.Tx) != 2 {
		t.Errorf("Expected 2 transactions, got: %d", len(blockInfo.Tx))
	}
}

// Test getblockhash
func TestHandleGetBlockHash(t *testing.T) {
	testBlock := createTestBlock(100, 1)

	mock := &mockBlockchain{
		blocksByHeight: map[uint32]*types.Block{
			100: testBlock,
		},
	}

	server := &Server{
		blockchain: mock,
	}

	params, _ := json.Marshal([]interface{}{100})
	req := &Request{
		JSONRPC: "2.0",
		Method:  "getblockhash",
		Params:  params,
		ID:      json.RawMessage("1"),
	}

	resp := server.handleGetBlockHash(req)

	if resp.Error != nil {
		t.Fatalf("Expected no error, got: %v", resp.Error)
	}

	hashStr, ok := resp.Result.(string)
	if !ok {
		t.Fatalf("Expected string result, got: %T", resp.Result)
	}

	if hashStr != testBlock.Hash().String() {
		t.Errorf("Expected hash %s, got: %s", testBlock.Hash().String(), hashStr)
	}
}

// Test getblockheader with mediantime field
func TestHandleGetBlockHeader(t *testing.T) {
	testBlock := createTestBlock(100, 1)
	blockHash := testBlock.Hash()

	mock := &mockBlockchain{
		height: 105,
		blocks: map[types.Hash]*types.Block{
			blockHash: testBlock,
		},
		blocksByHeight: map[uint32]*types.Block{
			100: testBlock,
			101: createTestBlock(101, 1),
		},
	}

	// Add blocks for median time
	for i := uint32(90); i < 100; i++ {
		mock.blocksByHeight[i] = createTestBlock(i, 1)
	}

	server := &Server{
		blockchain: mock,
	}

	params, _ := json.Marshal([]interface{}{blockHash.String(), true})
	req := &Request{
		JSONRPC: "2.0",
		Method:  "getblockheader",
		Params:  params,
		ID:      json.RawMessage("1"),
	}

	resp := server.handleGetBlockHeader(req)

	if resp.Error != nil {
		t.Fatalf("Expected no error, got: %v", resp.Error)
	}

	headerInfo, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map result, got: %T", resp.Result)
	}

	// Verify required fields including mediantime
	requiredFields := []string{"hash", "confirmations", "height", "version", "merkleroot", "time", "mediantime", "nonce", "bits", "difficulty"}
	for _, field := range requiredFields {
		if _, exists := headerInfo[field]; !exists {
			t.Errorf("Missing required field: %s", field)
		}
	}

	// Verify mediantime is present and non-zero
	if medianTime, ok := headerInfo["mediantime"].(int64); !ok || medianTime == 0 {
		t.Error("mediantime field missing or zero - NEW FIELD")
	}
}

// Test gettxoutsetinfo
func TestHandleGetTxOutSetInfo(t *testing.T) {
	testBlock := createTestBlock(100, 1)

	mock := &mockBlockchain{
		height:    100,
		bestBlock: testBlock,
	}

	server := &Server{
		blockchain: mock,
	}

	req := &Request{
		JSONRPC: "2.0",
		Method:  "gettxoutsetinfo",
		Params:  json.RawMessage("[]"),
		ID:      json.RawMessage("1"),
	}

	resp := server.handleGetTxOutSetInfo(req)

	if resp.Error != nil {
		t.Fatalf("Expected no error, got: %v", resp.Error)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map result, got: %T", resp.Result)
	}

	// Verify all required fields
	requiredFields := []string{"height", "bestblock", "transactions", "txouts", "bytes_serialized", "hash_serialized", "total_amount"}
	for _, field := range requiredFields {
		if _, exists := result[field]; !exists {
			t.Errorf("Missing required field: %s", field)
		}
	}
}

// Test getdifficulty
func TestHandleGetDifficulty(t *testing.T) {
	testBlock := createTestBlock(100, 1)

	mock := &mockBlockchain{
		bestBlock: testBlock,
	}

	server := &Server{
		blockchain: mock,
	}

	req := &Request{
		JSONRPC: "2.0",
		Method:  "getdifficulty",
		Params:  json.RawMessage("[]"),
		ID:      json.RawMessage("1"),
	}

	resp := server.handleGetDifficulty(req)

	if resp.Error != nil {
		t.Fatalf("Expected no error, got: %v", resp.Error)
	}

	difficulty, ok := resp.Result.(float64)
	if !ok {
		t.Fatalf("Expected float64 result, got: %T", resp.Result)
	}

	if difficulty <= 0 {
		t.Errorf("Expected positive difficulty, got: %f", difficulty)
	}
}

// Test calculateMedianTime helper function
func TestCalculateMedianTime(t *testing.T) {
	mock := &mockBlockchain{
		height:         14,
		blocksByHeight: make(map[uint32]*types.Block),
	}

	// Create blocks with incrementing timestamps
	for i := uint32(0); i <= 14; i++ {
		block := createTestBlock(i, 1)
		mock.blocksByHeight[i] = block
	}

	server := &Server{
		blockchain: mock,
	}

	medianTime := server.calculateMedianTime(14)

	if medianTime == 0 {
		t.Error("Median time should not be zero")
	}

	// Median of last 11 blocks (4 to 14) should be block 9's timestamp
	expectedTime := int64(mock.blocksByHeight[9].Header.Timestamp)
	if medianTime != expectedTime {
		t.Errorf("Expected median time %d, got: %d", expectedTime, medianTime)
	}
}

// Test calculateMoneySupply helper function (uses storage-based approach)
func TestCalculateMoneySupply(t *testing.T) {
	mock := &mockBlockchain{
		moneySupply: map[uint32]int64{
			100: 500000000000, // 5000 TWINS in satoshis
		},
	}

	server := &Server{
		blockchain: mock,
	}

	supply := server.calculateMoneySupply(100)

	expected := 5000.0 // 5000 TWINS
	if supply != expected {
		t.Errorf("Expected supply %f, got: %f", expected, supply)
	}

	// Test missing height returns 0
	supplyMissing := server.calculateMoneySupply(999)
	if supplyMissing != 0 {
		t.Errorf("Expected 0 for missing height, got: %f", supplyMissing)
	}
}

// Test Bitcoin-style difficulty calculation
// Reference: legacy/src/rpc/blockchain.cpp:94-120 GetDifficulty()
func TestCalculateDifficultyFromBits(t *testing.T) {
	tests := []struct {
		name     string
		bits     uint32
		expected float64
		tolerance float64
	}{
		{
			name:      "Genesis block difficulty (0x1d00ffff)",
			bits:      0x1d00ffff,
			expected:  1.0,
			tolerance: 0.0001,
		},
		{
			name:      "TWINS PoS typical difficulty (0x1e0fffff)",
			bits:      0x1e0fffff,
			expected:  0.00024414, // Very low difficulty for PoS
			tolerance: 0.00001,
		},
		{
			name:      "Zero bits",
			bits:      0,
			expected:  0,
			tolerance: 0,
		},
		{
			name:      "Zero mantissa",
			bits:      0x1d000000,
			expected:  0,
			tolerance: 0,
		},
		{
			name:      "High difficulty (exponent < 29)",
			bits:      0x1b0404cb, // Bitcoin block 100000-ish difficulty
			expected:  16307.420938, // Approximately
			tolerance: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateDifficultyFromBits(tt.bits)
			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > tt.tolerance {
				t.Errorf("calculateDifficultyFromBits(0x%08x) = %f, want %f (tolerance %f)",
					tt.bits, result, tt.expected, tt.tolerance)
			}
		})
	}
}
