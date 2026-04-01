package rpc

import (
	"encoding/hex"
	"encoding/json"

	"github.com/twins-dev/twins-core/internal/mempool"
	"github.com/twins-dev/twins-core/pkg/script"
	"github.com/twins-dev/twins-core/pkg/types"
)

// registerTransactionHandlers registers transaction-related RPC handlers
func (s *Server) registerTransactionHandlers() {
	s.RegisterHandler("getrawtransaction", s.handleGetRawTransaction)
	s.RegisterHandler("sendrawtransaction", s.handleSendRawTransaction)
	// Note: gettransaction is a WALLET method, registered in wallet.go
	s.RegisterHandler("decoderawtransaction", s.handleDecodeRawTransaction)
	s.RegisterHandler("gettxout", s.handleGetTxOut)
}

// handleGetRawTransaction returns raw transaction data
func (s *Server) handleGetRawTransaction(req *Request) *Response {
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

	// Check verbosity (default: 0 - raw hex)
	verbose := 0
	if len(params) > 1 {
		if v, ok := params[1].(float64); ok {
			verbose = int(v)
		}
	}

	// Try to find transaction in blockchain first
	tx, err := s.blockchain.GetTransaction(txHash)
	if err != nil {
		// Try mempool
		var ok bool
		tx, ok = s.mempool.GetTransaction(txHash)
		if !ok {
			return &Response{
				JSONRPC: "2.0",
				Error:   NewError(CodeTransactionNotFound, "Transaction not found", txHashStr),
				ID:      req.ID,
			}
		}
	}

	// Verbose 0: Return hex-encoded transaction
	if verbose == 0 {
		txData, err := tx.Serialize()
		if err != nil {
			return &Response{
				JSONRPC: "2.0",
				Error:   NewInternalError("failed to serialize transaction: " + err.Error()),
				ID:      req.ID,
			}
		}

		txHex := hex.EncodeToString(txData)
		return &Response{
			JSONRPC: "2.0",
			Result:  txHex,
			ID:      req.ID,
		}
	}

	// Verbose 1+: Return transaction info
	txInfo, err := s.buildTransactionInfo(tx, txHash)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInternalError(err.Error()),
			ID:      req.ID,
		}
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  txInfo,
		ID:      req.ID,
	}
}

// handleSendRawTransaction submits a raw transaction to the network
func (s *Server) handleSendRawTransaction(req *Request) *Response {
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
			Error:   NewInvalidParamsError("missing transaction hex"),
			ID:      req.ID,
		}
	}

	txHex, ok := params[0].(string)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("transaction hex must be a string"),
			ID:      req.ID,
		}
	}

	// Decode hex
	txData, err := hex.DecodeString(txHex)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid transaction hex"),
			ID:      req.ID,
		}
	}

	// Deserialize transaction
	tx, err := types.DeserializeTransaction(txData)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(CodeInvalidTransaction, "Failed to deserialize transaction", err.Error()),
			ID:      req.ID,
		}
	}

	// Submit to mempool
	if err := s.mempool.AddTransaction(tx); err != nil {
		// Legacy-compatible idempotence: treat "already in mempool" as success.
		if mErr, ok := err.(*mempool.MempoolError); !ok || mErr.Code != mempool.RejectDuplicate {
			return &Response{
				JSONRPC: "2.0",
				Error:   NewError(CodeInvalidTransaction, "Failed to add transaction to mempool", err.Error()),
				ID:      req.ID,
			}
		}
	}

	// Relay accepted transaction to network (legacy parity: accept + relay).
	if s.p2pServer != nil {
		if err := s.p2pServer.RelayTransaction(tx); err != nil {
			s.logger.WithError(err).WithField("tx", tx.Hash().String()).
				Warn("sendrawtransaction: tx accepted but relay failed")
		}
	}

	// Return transaction hash
	txHash := tx.Hash()
	return &Response{
		JSONRPC: "2.0",
		Result:  txHash.String(),
		ID:      req.ID,
	}
}

// handleGetTransaction returns detailed transaction information
// Removed: handleGetTransaction moved to wallet.go as it's a wallet-specific method

// handleDecodeRawTransaction decodes a raw transaction hex
func (s *Server) handleDecodeRawTransaction(req *Request) *Response {
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
			Error:   NewInvalidParamsError("missing transaction hex"),
			ID:      req.ID,
		}
	}

	txHex, ok := params[0].(string)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("transaction hex must be a string"),
			ID:      req.ID,
		}
	}

	// Decode hex
	txData, err := hex.DecodeString(txHex)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid transaction hex"),
			ID:      req.ID,
		}
	}

	// Deserialize transaction
	tx, err := types.DeserializeTransaction(txData)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(CodeInvalidTransaction, "Failed to deserialize transaction", err.Error()),
			ID:      req.ID,
		}
	}

	// Build transaction info
	txHash := tx.Hash()
	txInfo, err := s.buildTransactionInfo(tx, txHash)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInternalError(err.Error()),
			ID:      req.ID,
		}
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  txInfo,
		ID:      req.ID,
	}
}

// handleGetTxOut returns details about an unspent transaction output
func (s *Server) handleGetTxOut(req *Request) *Response {
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid parameters"),
			ID:      req.ID,
		}
	}

	if len(params) < 2 {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("missing transaction hash or output index"),
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

	nFloat, ok := params[1].(float64)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("output index must be a number"),
			ID:      req.ID,
		}
	}
	n := uint32(nFloat)

	// Try to get UTXO
	outpoint := types.Outpoint{Hash: txHash, Index: n}
	utxo, err := s.blockchain.GetUTXO(outpoint)
	if err != nil {
		// Output not found or already spent
		return &Response{
			JSONRPC: "2.0",
			Result:  nil,
			ID:      req.ID,
		}
	}

	// Get best block
	bestBlock, _ := s.blockchain.GetBestBlock()

	// UTXO confirmations would require tracking block height where UTXO was created
	// For now, return 1 to indicate it's confirmed
	confirmations := int64(1)

	// Try to get the full transaction to extract version
	txVersion := uint32(1) // Default version
	isCoinbase := false
	tx, err := s.blockchain.GetTransaction(txHash)
	if err == nil && tx != nil {
		txVersion = tx.Version
		// Check if this is a coinbase transaction
		if len(tx.Inputs) > 0 && tx.Inputs[0].PreviousOutput.Hash.IsZero() {
			isCoinbase = true
		}
	}

	bestBlockHash := ""
	if bestBlock != nil {
		bestBlockHash = bestBlock.Hash().String()
	}

	result := map[string]interface{}{
		"bestblock":     bestBlockHash,
		"confirmations": confirmations,
		"value":         float64(utxo.Value) / 1e8,
		"scriptPubKey": map[string]interface{}{
			"hex": hex.EncodeToString(utxo.ScriptPubKey),
		},
		"version":  txVersion,
		"coinbase": isCoinbase,
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// buildTransactionInfo constructs detailed transaction information
func (s *Server) buildTransactionInfo(tx *types.Transaction, txHash types.Hash) (*TransactionInfo, error) {
	// Serialize transaction to hex
	txData, err := tx.Serialize()
	if err != nil {
		return nil, err
	}
	txHex := hex.EncodeToString(txData)

	// Build inputs
	vins := make([]VinInfo, len(tx.Inputs))
	for i, input := range tx.Inputs {
		vin := VinInfo{
			Sequence: input.Sequence,
		}

		// Check if coinbase
		if input.PreviousOutput.Hash.IsZero() {
			vin.Coinbase = hex.EncodeToString(input.ScriptSig)
		} else {
			vin.TxID = input.PreviousOutput.Hash.String()
			vin.Vout = input.PreviousOutput.Index
			scriptSigHex := hex.EncodeToString(input.ScriptSig)
			// Decode scriptSig to ASM
			scriptSigAsm := ""
			if asm, err := script.Disassemble(input.ScriptSig); err == nil {
				scriptSigAsm = asm
			}
			vin.ScriptSig = &ScriptSigInfo{
				Asm: scriptSigAsm,
				Hex: scriptSigHex,
			}
		}

		vins[i] = vin
	}

	// Build outputs
	vouts := make([]VoutInfo, len(tx.Outputs))
	for i, output := range tx.Outputs {
		scriptHex := hex.EncodeToString(output.ScriptPubKey)

		// Create script info with analysis
		scriptInfo := &ScriptPubKeyInfo{
			Hex: scriptHex,
		}

		// Disassemble script to get ASM representation
		if asm, err := script.Disassemble(output.ScriptPubKey); err == nil {
			scriptInfo.Asm = asm
		}

		// Determine script type and extract addresses
		scriptType := script.GetScriptType(output.ScriptPubKey)
		scriptInfo.Type = scriptType.String()

		// Extract addresses based on script type
		switch scriptType {
		case script.PubKeyHashTy:
			if addr, err := script.ExtractPubKeyHashAddress(output.ScriptPubKey); err == nil {
				scriptInfo.Addresses = []string{addr.String()}
				scriptInfo.ReqSigs = 1
			}
		case script.ScriptHashTy:
			if addr, err := script.ExtractScriptHash(output.ScriptPubKey); err == nil {
				scriptInfo.Addresses = []string{addr.String()}
				scriptInfo.ReqSigs = 1
			}
		case script.PubKeyTy:
			if addr, err := script.ExtractPubKey(output.ScriptPubKey); err == nil {
				scriptInfo.Addresses = []string{addr.String()}
				scriptInfo.ReqSigs = 1
			}
		case script.MultiSigTy:
			if addrs, m, err := script.ExtractMultisig(output.ScriptPubKey); err == nil {
				addrStrs := make([]string, len(addrs))
				for i, addr := range addrs {
					addrStrs[i] = addr.String()
				}
				scriptInfo.Addresses = addrStrs
				scriptInfo.ReqSigs = m
			}
		}

		vout := VoutInfo{
			Value:        float64(output.Value) / 1e8,
			N:            i,
			ScriptPubKey: scriptInfo,
		}
		vouts[i] = vout
	}

	// Calculate actual transaction size using serialization
	txSize := tx.SerializeSize()

	info := &TransactionInfo{
		Hex:      txHex,
		TxID:     txHash.String(),
		Hash:     txHash.String(),
		Version:  int(tx.Version),
		Size:     txSize,
		VSize:    txSize, // For PoS, vsize = size (no witness data)
		LockTime: tx.LockTime,
		Vin:      vins,
		Vout:     vouts,
	}

	// Try to get block information for transaction if it's confirmed
	if s.blockchain != nil {
		// Check if transaction is in mempool (unconfirmed)
		if s.mempool != nil && s.mempool.HasTransaction(tx.Hash()) {
			info.Confirmations = 0
		} else {
			// Transaction is confirmed - get block info
			block, err := s.blockchain.GetTransactionBlock(txHash)
			if err == nil && block != nil {
				blockHash := block.Hash()
				info.BlockHash = blockHash.String()
				info.Time = int64(block.Header.Timestamp)
				info.BlockTime = int64(block.Header.Timestamp)

				// Calculate confirmations
				blockHeight, err := s.blockchain.GetBlockHeightByHash(blockHash)
				if err == nil {
					bestHeight, err := s.blockchain.GetBestHeight()
					if err == nil {
						info.Confirmations = int64(bestHeight) - int64(blockHeight) + 1
					}
				}
			}
		}
	}

	return info, nil
}
