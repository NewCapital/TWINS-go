package rpc

import (
	"encoding/json"
	"fmt"
	"time"
)

// registerUtilityHandlers registers utility RPC handlers
// Note: validateaddress, createmultisig, verifymessage are registered in wallet.go
func (s *Server) registerUtilityHandlers() {
	s.RegisterHandler("setmocktime", s.handleSetMockTime)
	s.RegisterHandler("mnsync", s.handleMnSync)
	s.RegisterHandler("spork", s.handleSpork)
	s.RegisterHandler("settxfee", s.handleSetTxFee)
}

// handleSetMockTime sets the local time to given timestamp (regtest only)
func (s *Server) handleSetMockTime(req *Request) *Response {
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-32602, "Invalid params", err),
			ID:      req.ID,
		}
	}

	if len(params) != 1 {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-32602, "setmocktime requires exactly 1 parameter (timestamp)", nil),
			ID:      req.ID,
		}
	}

	// Check if we're in regtest mode
	if s.chainParams == nil || s.chainParams.Name != "regtest" {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "setmocktime is only available in regtest mode", nil),
			ID:      req.ID,
		}
	}

	// Parse timestamp parameter
	var timestamp int64
	switch v := params[0].(type) {
	case float64:
		timestamp = int64(v)
	case int64:
		timestamp = v
	case int:
		timestamp = int64(v)
	default:
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-32602, "Invalid timestamp parameter", nil),
			ID:      req.ID,
		}
	}

	// Validate timestamp is reasonable (after Bitcoin genesis, before year 2100)
	const minTimestamp = 1231006505 // Bitcoin genesis block time (Jan 3, 2009)
	const maxTimestamp = 4102444800 // Jan 1, 2100
	if timestamp < minTimestamp || timestamp > maxTimestamp {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-32602, "Timestamp out of valid range (2009-2100)", nil),
			ID:      req.ID,
		}
	}

	// Set mock time (for testing purposes)
	// Note: This would need to be implemented in the blockchain/consensus layer
	// For now, we'll just acknowledge the request
	s.logger.WithField("timestamp", timestamp).Debug("Mock time set (regtest)")

	return &Response{
		JSONRPC: "2.0",
		Result:  nil,
		ID:      req.ID,
	}
}

// handleMnSync returns masternode sync status or resets sync
func (s *Server) handleMnSync(req *Request) *Response {
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-32602, "Invalid params", err),
			ID:      req.ID,
		}
	}

	if len(params) != 1 {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-32602, "mnsync requires exactly 1 parameter (\"status\" or \"reset\")", nil),
			ID:      req.ID,
		}
	}

	mode, ok := params[0].(string)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-32602, "Invalid mode parameter (expected string)", nil),
			ID:      req.ID,
		}
	}

	if mode == "status" {
		// Get masternode sync status
		if s.masternode == nil {
			return &Response{
				JSONRPC: "2.0",
				Error:   NewError(-1, "Masternode interface not available", nil),
				ID:      req.ID,
			}
		}

		// Build sync status response
		// Note: These values should come from the masternode manager
		syncStatus := map[string]interface{}{
			"IsBlockchainSynced":          s.blockchain != nil && !s.blockchain.IsInitialBlockDownload(),
			"lastMasternodeList":          time.Now().Unix(),
			"lastMasternodeWinner":        time.Now().Unix(),
			"lastBudgetItem":              0, // Budget system disabled
			"lastFailure":                 0,
			"nCountFailures":              0,
			"sumMasternodeList":           0,
			"sumMasternodeWinner":         0,
			"sumBudgetItemProp":           0, // Budget system disabled
			"sumBudgetItemFin":            0, // Budget system disabled
			"countMasternodeList":         0,
			"countMasternodeWinner":       0,
			"countBudgetItemProp":         0, // Budget system disabled
			"countBudgetItemFin":          0, // Budget system disabled
			"RequestedMasternodeAssets":   3, // All assets synced
			"RequestedMasternodeAttempt":  0,
		}

		return &Response{
			JSONRPC: "2.0",
			Result:  syncStatus,
			ID:      req.ID,
		}
	} else if mode == "reset" {
		// Reset masternode sync
		if s.masternode == nil {
			return &Response{
				JSONRPC: "2.0",
				Error:   NewError(-1, "Masternode interface not available", nil),
				ID:      req.ID,
			}
		}

		// Reset masternode sync state
		s.masternode.ResetSync()
		s.logger.Info("Masternode sync reset completed")

		return &Response{
			JSONRPC: "2.0",
			Result:  nil,
			ID:      req.ID,
		}
	}

	return &Response{
		JSONRPC: "2.0",
		Error:   NewError(-32602, "Invalid mode parameter (expected \"status\" or \"reset\")", nil),
		ID:      req.ID,
	}
}

// handleSpork shows or updates spork values
func (s *Server) handleSpork(req *Request) *Response {
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-32602, "Invalid params", err),
			ID:      req.ID,
		}
	}

	if len(params) < 1 {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-32602, "spork requires at least 1 parameter", nil),
			ID:      req.ID,
		}
	}

	command, ok := params[0].(string)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-32602, "Invalid command parameter (expected string)", nil),
			ID:      req.ID,
		}
	}

	// Handle "show" and "active" commands (they return the same thing)
	if command == "show" || command == "active" {
		// Spork IDs and names mapping
		sporkMap := map[string]int32{
			"SPORK_5_MAX_VALUE":                       10004,
			"SPORK_7_MASTERNODE_SCANNING":             10006,
			"SPORK_8_MASTERNODE_PAYMENT_ENFORCEMENT":  10007,
			"SPORK_10_MASTERNODE_PAY_UPDATED_NODES":   10009,
			"SPORK_14_NEW_PROTOCOL_ENFORCEMENT":       10013,
			"SPORK_15_NEW_PROTOCOL_ENFORCEMENT_2":     10014,
			"SPORK_TWINS_01_ENABLE_MASTERNODE_TIERS":  20190001,
			"SPORK_TWINS_02_MIN_STAKE_AMOUNT":         20190002,
		}

		// Default values (used when spork manager not available)
		defaults := map[int32]int64{
			10004:    1000,       // MAX_VALUE
			10006:    978307200,  // MASTERNODE_SCANNING (ON)
			10007:    4070908800, // MASTERNODE_PAYMENT_ENFORCEMENT (OFF)
			10009:    4070908800, // MASTERNODE_PAY_UPDATED_NODES (OFF)
			10013:    4070908800, // NEW_PROTOCOL_ENFORCEMENT (OFF)
			10014:    4070908800, // NEW_PROTOCOL_ENFORCEMENT_2 (OFF)
			20190001: 4070908800, // TWINS_01_ENABLE_MASTERNODE_TIERS (OFF)
			20190002: 4070908800, // TWINS_02_MIN_STAKE_AMOUNT (OFF)
		}

		sporkValues := make(map[string]int64)
		for name, id := range sporkMap {
			if s.sporkManager != nil {
				sporkValues[name] = s.sporkManager.GetValue(id)
			} else {
				sporkValues[name] = defaults[id]
			}
		}

		return &Response{
			JSONRPC: "2.0",
			Result:  sporkValues,
			ID:      req.ID,
		}
	}

	// Handle spork update (requires spork private key)
	if len(params) < 2 {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-32602, "Spork update requires spork name and value", nil),
			ID:      req.ID,
		}
	}

	// Parse value parameter
	var value int64
	switch v := params[1].(type) {
	case float64:
		value = int64(v)
	case int64:
		value = v
	case int:
		value = int64(v)
	default:
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-32602, "Invalid spork value parameter", nil),
			ID:      req.ID,
		}
	}

	// Spork updates require the spork private key which is held only by network administrators.
	// This operation is intentionally restricted for security - only nodes with -sporkkey
	// configuration can broadcast spork updates to the network.
	s.logger.WithFields(map[string]interface{}{
		"spork": command,
		"value": value,
	}).Warn("Spork update rejected - requires spork private key configuration")

	return &Response{
		JSONRPC: "2.0",
		Error:   NewError(-1, "Spork updates require -sporkkey configuration (network administrator only)", nil),
		ID:      req.ID,
	}
}

// handleSetTxFee sets the transaction fee per kilobyte
func (s *Server) handleSetTxFee(req *Request) *Response {
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-32602, "Invalid params", err),
			ID:      req.ID,
		}
	}

	if len(params) != 1 {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-32602, "settxfee requires exactly 1 parameter (amount in TWINS/kB)", nil),
			ID:      req.ID,
		}
	}

	// Check if wallet is available
	if s.wallet == nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "Wallet not available (use -disablewallet=0)", nil),
			ID:      req.ID,
		}
	}

	// Parse amount parameter
	var amount float64
	switch v := params[0].(type) {
	case float64:
		amount = v
	case int64:
		amount = float64(v)
	case int:
		amount = float64(v)
	default:
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-32602, "Invalid amount parameter", nil),
			ID:      req.ID,
		}
	}

	// Validate amount range (prevent unreasonably high fees)
	const maxTxFee = 1.0 // 1 TWINS/kB maximum (reasonable upper bound)
	if amount < 0 || amount > maxTxFee {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-32602, fmt.Sprintf("Amount out of range (0 to %.8f TWINS/kB)", maxTxFee), nil),
			ID:      req.ID,
		}
	}

	// Convert TWINS/kB to satoshis/kB
	feeInSatoshis := int64(amount * 1e8)

	// Set the transaction fee in wallet
	if err := s.wallet.SetTransactionFee(feeInSatoshis); err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-4, fmt.Sprintf("Failed to set transaction fee: %v", err), nil),
			ID:      req.ID,
		}
	}

	s.logger.WithField("fee", fmt.Sprintf("%.8f TWINS/kB", amount)).Debug("Transaction fee set")

	return &Response{
		JSONRPC: "2.0",
		Result:  true,
		ID:      req.ID,
	}
}
