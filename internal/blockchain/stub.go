package blockchain

import (
	"fmt"

	"github.com/twins-dev/twins-core/pkg/types"
)

// StubBlockchain is a minimal stub implementation for RPC building
type StubBlockchain struct{}

// Block retrieval
func (s *StubBlockchain) GetBestBlock() (*types.Block, error) {
	return &types.Block{Header: &types.BlockHeader{}}, nil
}

func (s *StubBlockchain) GetBestHeight() (uint32, error) {
	return 0, nil
}

func (s *StubBlockchain) GetBlock(hash types.Hash) (*types.Block, error) {
	return &types.Block{Header: &types.BlockHeader{}}, nil
}

func (s *StubBlockchain) GetBlockByHeight(height uint32) (*types.Block, error) {
	return &types.Block{Header: &types.BlockHeader{}}, nil
}

func (s *StubBlockchain) GetBlockHash(height uint32) (types.Hash, error) {
	return types.Hash{}, nil
}

func (s *StubBlockchain) GetBlockHeight() (uint32, error) {
	return 0, nil
}

// Chain info
func (s *StubBlockchain) GetChainWork() (string, error) {
	return "0", nil
}

func (s *StubBlockchain) GetDifficulty() (float64, error) {
	return 1.0, nil
}

func (s *StubBlockchain) GetChainTips() ([]interface{}, error) {
	return nil, nil
}

func (s *StubBlockchain) GetBlockCount() (int64, error) {
	return 0, nil
}

// Validation
func (s *StubBlockchain) ValidateBlock(block *types.Block) error {
	return nil
}

func (s *StubBlockchain) ProcessBlock(block *types.Block) error {
	return nil
}

// UTXO operations
func (s *StubBlockchain) GetUTXO(outpoint types.Outpoint) (*types.TxOutput, error) {
	return nil, nil
}

func (s *StubBlockchain) GetUTXOSet() (map[types.Outpoint]*types.TxOutput, error) {
	return nil, fmt.Errorf("GetUTXOSet not implemented: use GetUTXO for individual lookups")
}

// Transaction operations
func (s *StubBlockchain) GetTransaction(hash types.Hash) (*types.Transaction, error) {
	return nil, nil
}

func (s *StubBlockchain) GetRawTransaction(hash types.Hash) ([]byte, error) {
	return nil, nil
}

// Chain state
func (s *StubBlockchain) GetChainParams() *types.ChainParams {
	return types.MainnetParams()
}

func (s *StubBlockchain) IsInitialBlockDownload() bool {
	return false
}

func (s *StubBlockchain) GetVerificationProgress() float64 {
	return 1.0
}