package rpc

import (
	"encoding/json"

	"github.com/twins-dev/twins-core/pkg/types"
)

// registerMempoolHandlers registers mempool-related RPC handlers
func (s *Server) registerMempoolHandlers() {
	s.RegisterHandler("getrawmempool", s.handleGetRawMempool)
	s.RegisterHandler("getmempoolinfo", s.handleGetMempoolInfo)
	s.RegisterHandler("getmempoolentry", s.handleGetMempoolEntry)
	s.RegisterHandler("getmempoolancestors", s.handleGetMempoolAncestors)
	s.RegisterHandler("getmempooldescendants", s.handleGetMempoolDescendants)
}

// handleGetRawMempool returns all transaction IDs in the mempool
func (s *Server) handleGetRawMempool(req *Request) *Response {
	var params []interface{}
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return &Response{
				JSONRPC: "2.0",
				Error:   NewInvalidParamsError("invalid parameters"),
				ID:      req.ID,
			}
		}
	}

	// Check verbosity (default: false - just array of txids)
	verbose := false
	if len(params) > 0 {
		if v, ok := params[0].(bool); ok {
			verbose = v
		}
	}

	// Get all transactions
	txs := s.mempool.GetTransactions()

	if !verbose {
		// Return just transaction IDs
		txids := make([]string, len(txs))
		for i, tx := range txs {
			txids[i] = tx.Hash().String()
		}

		return &Response{
			JSONRPC: "2.0",
			Result:  txids,
			ID:      req.ID,
		}
	}

	// Return detailed information
	result := make(map[string]interface{})
	for _, tx := range txs {
		txHash := tx.Hash()

		entry := map[string]interface{}{
			"size":    100, // Placeholder size
			"fee":     s.calculateTxFee(tx),
			"time":    0, // Would need to track entry time
			"height":  0, // Current block height when entered
			"depends": []string{}, // Dependencies (would need to calculate)
		}

		result[txHash.String()] = entry
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// handleGetMempoolInfo returns mempool statistics
func (s *Server) handleGetMempoolInfo(req *Request) *Response {
	mempoolInfo := s.mempool.GetMempoolInfo()

	info := &MempoolInfo{
		Size:          mempoolInfo.Size,
		Bytes:         mempoolInfo.Bytes,
		Usage:         mempoolInfo.Usage,
		MaxMempool:    mempoolInfo.MaxMempool,
		MempoolMinFee: mempoolInfo.MempoolMinFee,
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  info,
		ID:      req.ID,
	}
}

// handleGetMempoolEntry returns mempool entry for a specific transaction
func (s *Server) handleGetMempoolEntry(req *Request) *Response {
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid parameters"),
			ID:      req.ID,
		}
	}

	if len(params) < 1 {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("missing transaction hash"),
			ID:      req.ID,
		}
	}

	txHashStr, ok := params[0].(string)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("transaction hash must be a string"),
			ID:      req.ID,
		}
	}

	txHash, err := types.NewHashFromString(txHashStr)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid transaction hash"),
			ID:      req.ID,
		}
	}

	// Check if transaction exists in mempool
	if !s.mempool.HasTransaction(txHash) {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(CodeTransactionNotFound, "Transaction not in mempool", txHashStr),
			ID:      req.ID,
		}
	}

	tx, ok := s.mempool.GetTransaction(txHash)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(CodeTransactionNotFound, "Transaction not in mempool", txHashStr),
			ID:      req.ID,
		}
	}

	entry := map[string]interface{}{
		"size":    100, // Placeholder size
		"fee":     s.calculateTxFee(tx),
		"time":    0,
		"height":  0,
		"depends": []string{},
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  entry,
		ID:      req.ID,
	}
}

// handleGetMempoolAncestors returns ancestor transactions in mempool
func (s *Server) handleGetMempoolAncestors(req *Request) *Response {
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid parameters"),
			ID:      req.ID,
		}
	}

	if len(params) < 1 {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("missing transaction hash"),
			ID:      req.ID,
		}
	}

	txHashStr, ok := params[0].(string)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("transaction hash must be a string"),
			ID:      req.ID,
		}
	}

	txHash, err := types.NewHashFromString(txHashStr)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid transaction hash"),
			ID:      req.ID,
		}
	}

	// Check if transaction exists in mempool
	if !s.mempool.HasTransaction(txHash) {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(CodeTransactionNotFound, "Transaction not in mempool", txHashStr),
			ID:      req.ID,
		}
	}

	// Get transaction
	tx, ok := s.mempool.GetTransaction(txHash)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(CodeTransactionNotFound, "Transaction not in mempool", txHashStr),
			ID:      req.ID,
		}
	}

	// Find ancestor transactions (transactions that this one spends)
	ancestors := make([]string, 0)
	for _, input := range tx.Inputs {
		if input.PreviousOutput.Hash.IsZero() {
			continue // Skip coinbase
		}

		if s.mempool.HasTransaction(input.PreviousOutput.Hash) {
			ancestors = append(ancestors, input.PreviousOutput.Hash.String())
		}
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  ancestors,
		ID:      req.ID,
	}
}

// handleGetMempoolDescendants returns descendant transactions in mempool
func (s *Server) handleGetMempoolDescendants(req *Request) *Response {
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid parameters"),
			ID:      req.ID,
		}
	}

	if len(params) < 1 {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("missing transaction hash"),
			ID:      req.ID,
		}
	}

	txHashStr, ok := params[0].(string)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("transaction hash must be a string"),
			ID:      req.ID,
		}
	}

	txHash, err := types.NewHashFromString(txHashStr)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid transaction hash"),
			ID:      req.ID,
		}
	}

	// Check if transaction exists in mempool
	if !s.mempool.HasTransaction(txHash) {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(CodeTransactionNotFound, "Transaction not in mempool", txHashStr),
			ID:      req.ID,
		}
	}

	// Find descendant transactions (transactions that spend this one)
	descendants := make([]string, 0)
	allTxs := s.mempool.GetTransactions()

	for _, tx := range allTxs {
		for _, input := range tx.Inputs {
			if input.PreviousOutput.Hash == txHash {
				descendants = append(descendants, tx.Hash().String())
				break
			}
		}
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  descendants,
		ID:      req.ID,
	}
}

// calculateTxFee calculates transaction fee
func (s *Server) calculateTxFee(tx *types.Transaction) float64 {
	// Calculate input value
	inputValue := int64(0)
	for _, input := range tx.Inputs {
		if input.PreviousOutput.Hash.IsZero() {
			continue // Skip coinbase
		}

		// Try to get UTXO
		utxo, err := s.blockchain.GetUTXO(input.PreviousOutput)
		if err != nil {
			continue
		}
		inputValue += utxo.Value
	}

	// Calculate output value
	outputValue := int64(0)
	for _, output := range tx.Outputs {
		outputValue += output.Value
	}

	fee := inputValue - outputValue
	if fee < 0 {
		fee = 0
	}

	return float64(fee) / 1e8
}