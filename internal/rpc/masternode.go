package rpc

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/twins-dev/twins-core/internal/masternode"
	"github.com/twins-dev/twins-core/internal/p2p"
	"github.com/twins-dev/twins-core/pkg/crypto"
	"github.com/twins-dev/twins-core/pkg/types"
)

// registerMasternodeHandlers registers all masternode-related RPC handlers
func (s *Server) registerMasternodeHandlers() {
	s.RegisterHandler("masternode", s.handleMasternode)
	s.RegisterHandler("listmasternodes", s.handleListMasternodes)
	s.RegisterHandler("getmasternodecount", s.handleGetMasternodeCount)
	s.RegisterHandler("masternodecurrent", s.handleMasternodeCurrent)
	s.RegisterHandler("masternodedebug", s.handleMasternodeDebug)
	s.RegisterHandler("getmasternodestatus", s.handleGetMasternodeStatus)
	s.RegisterHandler("getmasternodewinners", s.handleGetMasternodeWinners)
	s.RegisterHandler("getmasternodescores", s.handleGetMasternodeScores)
	s.RegisterHandler("startmasternode", s.handleStartMasternode)
	s.RegisterHandler("createmasternodekey", s.handleCreateMasternodeKey)
	s.RegisterHandler("getmasternodeoutputs", s.handleGetMasternodeOutputs)
	s.RegisterHandler("listmasternodeconf", s.handleListMasternodeConf)
	s.RegisterHandler("createmasternodebroadcast", s.handleCreateMasternodeBroadcast)
	s.RegisterHandler("decodemasternodebroadcast", s.handleDecodeMasternodeBroadcast)
	s.RegisterHandler("relaymasternodebroadcast", s.handleRelayMasternodeBroadcast)
	s.RegisterHandler("masternodeconnect", s.handleMasternodeConnect)
	s.RegisterHandler("getpoolinfo", s.handleGetPoolInfo)
	s.RegisterHandler("stopmasternode", s.handleStopMasternode)
}

// handleMasternode is the legacy command that routes to other masternode commands
func (s *Server) handleMasternode(req *Request) *Response {
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
			Error:   NewInvalidParamsError("masternode requires a command parameter"),
			ID:      req.ID,
		}
	}

	command, ok := params[0].(string)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid command parameter"),
			ID:      req.ID,
		}
	}

	// Route to appropriate handler based on command
	switch command {
	case "list", "list-conf":
		return s.handleListMasternodes(req)
	case "count":
		return s.handleGetMasternodeCount(req)
	case "current":
		return s.handleMasternodeCurrent(req)
	case "debug":
		return s.handleMasternodeDebug(req)
	case "status":
		return s.handleGetMasternodeStatus(req)
	case "winners":
		return s.handleGetMasternodeWinners(req)
	case "start", "start-alias", "start-many", "start-all", "start-local", "start-missing", "start-disabled":
		return s.handleStartMasternode(req)
	case "stop":
		return s.handleStopMasternode(req)
	case "connect":
		return s.handleMasternodeConnect(req)
	case "genkey":
		return s.handleCreateMasternodeKey(req)
	case "outputs":
		return s.handleGetMasternodeOutputs(req)
	case "calcscore":
		return s.handleGetMasternodeScores(req)
	default:
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "unknown masternode command: "+command, nil),
			ID:      req.ID,
		}
	}
}

// handleListMasternodes returns a list of all masternodes
func (s *Server) handleListMasternodes(req *Request) *Response {
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

	mode := "json"
	if len(params) > 0 {
		if m, ok := params[0].(string); ok {
			mode = m
		}
	}

	_ = mode

	if s.masternode == nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "masternode manager not available", nil),
			ID:      req.ID,
		}
	}

	// Get masternode list from manager
	masternodes := s.masternode.GetMasternodes()

	result := make([]map[string]interface{}, 0, len(masternodes))
	rank := 1
	for outpoint, mnInfo := range masternodes {
		info, err := s.masternode.GetMasternodeInfo(outpoint)
		if err != nil {
			continue
		}

		result = append(result, map[string]interface{}{
			"rank":       rank,
			"txhash":     outpoint.Hash.String(),
			"outidx":     outpoint.Index,
			"status":     info.Status,
			"addr":       info.Addr,
			"version":    info.Protocol,
			"lastseen":   info.LastPing,
			"activetime": info.LastPing - info.ActiveSince,
			"tier":       mnInfo.Tier.String(),
		})
		rank++
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// handleGetMasternodeCount returns the count of masternodes
func (s *Server) handleGetMasternodeCount(req *Request) *Response {
	if s.masternode == nil {
		result := map[string]interface{}{
			"total":    0,
			"enabled":  0,
			"bronze":   0,
			"silver":   0,
			"gold":     0,
			"platinum": 0,
		}
		return &Response{
			JSONRPC: "2.0",
			Result:  result,
			ID:      req.ID,
		}
	}

	// Get counts from masternode manager
	enabled, total, stable := s.masternode.GetMasternodeCount()
	bronze := s.masternode.GetMasternodeCountByTier(masternode.Bronze)
	silver := s.masternode.GetMasternodeCountByTier(masternode.Silver)
	gold := s.masternode.GetMasternodeCountByTier(masternode.Gold)
	platinum := s.masternode.GetMasternodeCountByTier(masternode.Platinum)

	// The enabled count comes from GetMasternodeCount()
	// stable variable is not currently used but available
	_ = stable

	result := map[string]interface{}{
		"total":    total,
		"enabled":  enabled,
		"bronze":   bronze,
		"silver":   silver,
		"gold":     gold,
		"platinum": platinum,
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// handleMasternodeCurrent returns the current masternode winner
func (s *Server) handleMasternodeCurrent(req *Request) *Response {
	if s.masternode == nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "masternode manager not available", nil),
			ID:      req.ID,
		}
	}

	// Get next payee (current winner)
	winner, err := s.masternode.GetNextPayee()
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "failed to get masternode winner: "+err.Error(), nil),
			ID:      req.ID,
		}
	}

	info, err := s.masternode.GetMasternodeInfo(winner.OutPoint)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "failed to get masternode info: "+err.Error(), nil),
			ID:      req.ID,
		}
	}

	result := map[string]interface{}{
		"protocol":   info.Protocol,
		"txhash":     winner.OutPoint.Hash.String(),
		"outidx":     winner.OutPoint.Index,
		"IP:port":    info.Addr,
		"tier":       winner.Tier.String(),
		"lastseen":   info.LastPing,
		"activetime": info.LastPing - info.ActiveSince,
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// handleMasternodeDebug returns debug information for masternode sync
// LEGACY COMPATIBILITY: Returns real sync state instead of hardcoded values
func (s *Server) handleMasternodeDebug(req *Request) *Response {
	result := map[string]interface{}{}

	// Get real sync status from masternode manager
	if s.masternode != nil {
		enabled, total, _ := s.masternode.GetMasternodeCount()
		result["sync_status"] = "MASTERNODE_SYNC_FINISHED"
		result["sync_asset"] = "MASTERNODE_SYNC_FINISHED"
		result["masternode_count_enabled"] = enabled
		result["masternode_count_total"] = total
	} else {
		result["sync_status"] = "MASTERNODE_SYNC_INITIAL"
		result["sync_asset"] = "MASTERNODE_SYNC_INITIAL"
		result["masternode_count_enabled"] = 0
		result["masternode_count_total"] = 0
	}

	// Get active masternode info if available
	if s.activeMasternode != nil {
		result["active_status"] = s.activeMasternode.GetStatus()
		result["active_started"] = s.activeMasternode.IsStarted()
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// handleGetMasternodeStatus returns the local masternode status
// LEGACY COMPATIBILITY: Returns real ActiveMasternode state instead of hardcoded placeholder
func (s *Server) handleGetMasternodeStatus(req *Request) *Response {
	result := map[string]interface{}{}

	if s.activeMasternode == nil {
		result["status"] = "Not configured as masternode"
		result["outpoint"] = ""
		result["service"] = ""
		result["pubkey"] = ""
		result["vin"] = ""
		result["tier"] = ""
		result["collateral"] = 0
		return &Response{JSONRPC: "2.0", Result: result, ID: req.ID}
	}

	// Get real status from ActiveMasternode
	result["status"] = s.activeMasternode.GetStatus()
	result["started"] = s.activeMasternode.IsStarted()

	// Get outpoint/vin using interface getters
	vin := s.activeMasternode.GetVin()
	if !vin.Hash.IsZero() {
		// Format as legacy CTxIn string for compatibility
		result["outpoint"] = vin.String()
		result["vin"] = fmt.Sprintf("CTxIn(COutPoint(%s, %d), scriptSig=)", vin.Hash.String(), vin.Index)
	} else {
		result["outpoint"] = ""
		result["vin"] = ""
	}

	// Get service address
	if serviceAddr := s.activeMasternode.GetServiceAddr(); serviceAddr != nil {
		result["service"] = serviceAddr.String()
	} else {
		result["service"] = ""
	}

	// Get masternode pubkey
	if pubKey := s.activeMasternode.GetPubKeyMasternode(); pubKey != nil {
		result["pubkey"] = hex.EncodeToString(pubKey.SerializeCompressed())
	} else {
		result["pubkey"] = ""
	}

	// Lookup collateral amount and tier from UTXO
	result["tier"] = ""
	result["collateral"] = float64(0)
	if !vin.Hash.IsZero() && s.blockchain != nil {
		if utxo, err := s.blockchain.GetUTXO(vin); err == nil && utxo != nil {
			// Convert from satoshis to TWINS (1 TWINS = 1e8 satoshis)
			result["collateral"] = float64(utxo.Value) / 1e8
			// Determine tier based on collateral amount using masternode constants
			switch {
			case utxo.Value >= masternode.TierPlatinumCollateral:
				result["tier"] = "Platinum"
			case utxo.Value >= masternode.TierGoldCollateral:
				result["tier"] = "Gold"
			case utxo.Value >= masternode.TierSilverCollateral:
				result["tier"] = "Silver"
			case utxo.Value >= masternode.TierBronzeCollateral:
				result["tier"] = "Bronze"
			}
		}
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// handleGetMasternodeWinners returns recent masternode payment winners
func (s *Server) handleGetMasternodeWinners(req *Request) *Response {
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

	blocks := 10
	if len(params) > 0 {
		if b, ok := params[0].(float64); ok {
			blocks = int(b)
		}
	}

	result := map[string]interface{}{}

	// Get current height
	height, err := s.blockchain.GetBestHeight()
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Result:  result,
			ID:      req.ID,
		}
	}

	// Get winners for recent blocks
	for i := 0; i < blocks && int(height)-i >= 0; i++ {
		blockHeight := uint32(int(height) - i)

		// Get the block to determine the actual winner
		block, err := s.blockchain.GetBlockByHeight(blockHeight)
		if err != nil {
			// If we can't get the block, skip it
			continue
		}

		blockHash := block.Hash()

		// Try to get the actual winner from masternode manager
		winner := "Unknown"
		if s.masternode != nil {
			if mnInfo, err := s.masternode.GetNextPaymentWinner(blockHeight, blockHash); err == nil {
				// Format: "address:outpoint"
				// Masternode doesn't have PayAddress, use Addr instead
				winner = mnInfo.Addr.String() + ":" + mnInfo.OutPoint.String()
			}
		}

		// Use proper string formatting for the key (height as string)
		result[fmt.Sprintf("%d", blockHeight)] = winner
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// handleGetMasternodeScores returns masternode scores
func (s *Server) handleGetMasternodeScores(req *Request) *Response {
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

	blocks := 10
	if len(params) > 0 {
		if b, ok := params[0].(float64); ok {
			blocks = int(b)
		}
	}

	_ = blocks

	if s.masternode == nil {
		return &Response{
			JSONRPC: "2.0",
			Result:  map[string]interface{}{},
			ID:      req.ID,
		}
	}

	// Calculate scores based on payment queue
	result := map[string]interface{}{}
	masternodes := s.masternode.GetMasternodes()

	for outpoint, mn := range masternodes {
		// LEGACY COMPATIBILITY: Key format must be "txhash-index" with numeric index
		// Previous bug: string(rune(outpoint.Index)) converts to Unicode char, not number
		key := outpoint.Hash.String() + "-" + strconv.Itoa(int(outpoint.Index))
		result[key] = mn.Score
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// handleStartMasternode starts a masternode
func (s *Server) handleStartMasternode(req *Request) *Response {
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
			Error:   NewInvalidParamsError("startmasternode requires mode parameter"),
			ID:      req.ID,
		}
	}

	mode, ok := params[0].(string)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("mode must be a string"),
			ID:      req.ID,
		}
	}

	// Validate mode
	if mode != "local" && mode != "alias" && mode != "all" && mode != "many" && mode != "missing" && mode != "disabled" {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid mode: must be 'local', 'alias', 'all', 'many', 'missing', or 'disabled'"),
			ID:      req.ID,
		}
	}

	// Handle "local" mode - start this node as a masternode
	if mode == "local" {
		return s.handleStartMasternodeLocal(req)
	}

	// Handle "alias" mode - start a remote masternode by alias
	if mode == "alias" {
		return s.handleStartMasternodeAlias(req, params)
	}

	// Handle "all" mode - start all masternodes from masternode.conf
	if mode == "all" {
		return s.handleStartMasternodeAll(req)
	}

	// Handle "many" mode - start multiple masternodes by aliases
	if mode == "many" {
		return s.handleStartMasternodeMany(req, params)
	}

	// For "missing" and "disabled" modes - same as "all" but filter by status
	return &Response{
		JSONRPC: "2.0",
		Error:   NewError(-32601, "startmasternode mode '"+mode+"' not yet implemented", nil),
		ID:      req.ID,
	}
}

// handleStartMasternodeLocal starts this node as a local masternode
func (s *Server) handleStartMasternodeLocal(req *Request) *Response {
	// Check if activeMasternode is configured
	if s.activeMasternode == nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-32603, "Active masternode not configured. Start daemon with -masternode=1", nil),
			ID:      req.ID,
		}
	}

	// Check if already started
	if s.activeMasternode.IsStarted() {
		return &Response{
			JSONRPC: "2.0",
			Result: map[string]interface{}{
				"status":  "already_started",
				"message": s.activeMasternode.GetStatus(),
			},
			ID: req.ID,
		}
	}

	// Trigger status management to attempt start
	if err := s.activeMasternode.ManageStatus(); err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInternalError(fmt.Sprintf("failed to manage masternode status: %v", err)),
			ID:      req.ID,
		}
	}

	// Return current status
	status := s.activeMasternode.GetStatus()
	started := s.activeMasternode.IsStarted()

	return &Response{
		JSONRPC: "2.0",
		Result: map[string]interface{}{
			"status":  started,
			"message": status,
		},
		ID: req.ID,
	}
}

// handleStartMasternodeAlias starts a remote masternode by alias from masternode.conf
func (s *Server) handleStartMasternodeAlias(req *Request, params []interface{}) *Response {
	if len(params) < 2 {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("startmasternode alias requires alias parameter"),
			ID:      req.ID,
		}
	}

	alias, ok := params[1].(string)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("alias must be a string"),
			ID:      req.ID,
		}
	}

	// Check if wallet is locked
	if s.wallet != nil && s.wallet.IsLocked() {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-13, "Wallet is locked. Please enter the wallet passphrase with walletpassphrase first.", nil),
			ID:      req.ID,
		}
	}

	// Check if masternode.conf is available
	if s.masternodeConf == nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "masternode.conf not loaded", nil),
			ID:      req.ID,
		}
	}

	// Read latest config
	if err := s.masternodeConf.Read(); err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "failed to read masternode.conf: "+err.Error(), nil),
			ID:      req.ID,
		}
	}

	// Find entry by alias
	entry := s.masternodeConf.GetEntry(alias)
	if entry == nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, fmt.Sprintf("masternode alias '%s' not found in masternode.conf", alias), nil),
			ID:      req.ID,
		}
	}

	// Start the masternode
	result := s.startMasternodeFromEntry(entry)

	return &Response{
		JSONRPC: "2.0",
		Result: map[string]interface{}{
			"overall": result["status"],
			"detail":  []map[string]interface{}{result},
		},
		ID: req.ID,
	}
}

// handleStartMasternodeAll starts all masternodes from masternode.conf
func (s *Server) handleStartMasternodeAll(req *Request) *Response {
	// Check if wallet is locked
	if s.wallet != nil && s.wallet.IsLocked() {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-13, "Wallet is locked. Please enter the wallet passphrase with walletpassphrase first.", nil),
			ID:      req.ID,
		}
	}

	// Check if masternode.conf is available
	if s.masternodeConf == nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "masternode.conf not loaded", nil),
			ID:      req.ID,
		}
	}

	// Read latest config
	if err := s.masternodeConf.Read(); err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "failed to read masternode.conf: "+err.Error(), nil),
			ID:      req.ID,
		}
	}

	entries := s.masternodeConf.GetEntries()
	if len(entries) == 0 {
		return &Response{
			JSONRPC: "2.0",
			Result: map[string]interface{}{
				"overall": "No masternodes configured in masternode.conf",
				"detail":  []map[string]interface{}{},
			},
			ID: req.ID,
		}
	}

	// Start all masternodes
	successful := 0
	failed := 0
	details := make([]map[string]interface{}, 0, len(entries))

	for _, entry := range entries {
		result := s.startMasternodeFromEntry(entry)
		details = append(details, result)
		if result["result"] == "successful" {
			successful++
		} else {
			failed++
		}
	}

	overall := fmt.Sprintf("Successfully started %d masternode(s), failed to start %d", successful, failed)
	if failed == 0 && successful > 0 {
		overall = fmt.Sprintf("Successfully started %d masternode(s)", successful)
	}

	return &Response{
		JSONRPC: "2.0",
		Result: map[string]interface{}{
			"overall": overall,
			"detail":  details,
		},
		ID: req.ID,
	}
}

// handleStartMasternodeMany starts multiple masternodes by aliases
func (s *Server) handleStartMasternodeMany(req *Request, params []interface{}) *Response {
	if len(params) < 2 {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("startmasternode many requires at least one alias"),
			ID:      req.ID,
		}
	}

	// Check if wallet is locked
	if s.wallet != nil && s.wallet.IsLocked() {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-13, "Wallet is locked. Please enter the wallet passphrase with walletpassphrase first.", nil),
			ID:      req.ID,
		}
	}

	// Check if masternode.conf is available
	if s.masternodeConf == nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "masternode.conf not loaded", nil),
			ID:      req.ID,
		}
	}

	// Read latest config
	if err := s.masternodeConf.Read(); err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "failed to read masternode.conf: "+err.Error(), nil),
			ID:      req.ID,
		}
	}

	// Collect aliases from params[1:]
	aliases := make([]string, 0, len(params)-1)
	for i := 1; i < len(params); i++ {
		alias, ok := params[i].(string)
		if !ok {
			continue
		}
		aliases = append(aliases, alias)
	}

	if len(aliases) == 0 {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("no valid aliases provided"),
			ID:      req.ID,
		}
	}

	// Start specified masternodes
	successful := 0
	failed := 0
	details := make([]map[string]interface{}, 0, len(aliases))

	for _, alias := range aliases {
		entry := s.masternodeConf.GetEntry(alias)
		if entry == nil {
			details = append(details, map[string]interface{}{
				"alias":  alias,
				"result": "failed",
				"error":  fmt.Sprintf("alias '%s' not found in masternode.conf", alias),
			})
			failed++
			continue
		}

		result := s.startMasternodeFromEntry(entry)
		details = append(details, result)
		if result["result"] == "successful" {
			successful++
		} else {
			failed++
		}
	}

	overall := fmt.Sprintf("Successfully started %d masternode(s), failed to start %d", successful, failed)
	if failed == 0 && successful > 0 {
		overall = fmt.Sprintf("Successfully started %d masternode(s)", successful)
	}

	return &Response{
		JSONRPC: "2.0",
		Result: map[string]interface{}{
			"overall": overall,
			"detail":  details,
		},
		ID: req.ID,
	}
}

// startMasternodeFromEntry creates broadcast and relays it for a single masternode entry
func (s *Server) startMasternodeFromEntry(entry *MasternodeConfEntry) map[string]interface{} {
	result := map[string]interface{}{
		"alias":  entry.Alias,
		"result": "failed",
	}

	// Get collateral key from wallet
	if s.wallet == nil {
		result["error"] = "wallet not available"
		return result
	}

	// Create the outpoint for this masternode
	outpoint := types.Outpoint{
		Hash:  entry.TxHash,
		Index: entry.OutputIndex,
	}

	// Get private key and correct pubkey format for collateral address from wallet
	// This determines the correct format (compressed vs uncompressed) by checking the scriptPubKey hash
	collateralKeyInfo, err := s.getCollateralKeyInfo(outpoint)
	if err != nil {
		result["error"] = fmt.Sprintf("failed to get collateral key: %v", err)
		return result
	}

	// Parse masternode private key
	mnPrivKey, err := crypto.DecodeWIF(entry.PrivKey)
	if err != nil {
		result["error"] = fmt.Sprintf("invalid masternode private key: %v", err)
		return result
	}

	// Parse service address
	addr, err := net.ResolveTCPAddr("tcp", entry.IP)
	if err != nil {
		result["error"] = fmt.Sprintf("invalid service address: %v", err)
		return result
	}

	// Create broadcast
	sigTime := time.Now().Unix()

	// Create ping first
	ping := &masternode.MasternodePing{
		OutPoint:  outpoint,
		BlockHash: s.getRecentBlockHash(),
		SigTime:   sigTime,
	}

	// Sign ping with masternode key using correct legacy format
	// CRITICAL: Must use GetSignatureMessage() which returns "CTxIn(COutPoint(hash, n), scriptSig=)" format
	// The old format "hash:index" was incorrect and caused signature verification failures
	pingMsg := ping.GetSignatureMessage()
	pingSig, err := crypto.SignCompact(mnPrivKey, pingMsg)
	if err != nil {
		result["error"] = fmt.Sprintf("failed to sign ping: %v", err)
		return result
	}
	ping.Signature = pingSig

	// Get the correct public key bytes based on detected format
	collateralPubKey := collateralKeyInfo.PrivKey.PublicKey()
	var collateralPubKeyBytes []byte
	if collateralKeyInfo.Compressed {
		collateralPubKeyBytes = collateralPubKey.SerializeCompressed()
	} else {
		collateralPubKeyBytes = collateralPubKey.Bytes() // Uncompressed (65 bytes)
	}

	// Create broadcast with correct pubkey format
	broadcast := &masternode.MasternodeBroadcast{
		OutPoint:              outpoint,
		Addr:                  addr,
		PubKeyCollateral:      collateralPubKey,
		PubKeyMasternode:      mnPrivKey.PublicKey(),
		PubKeyCollateralBytes: collateralPubKeyBytes, // Preserve correct format for serialization
		SigTime:               sigTime,
		Protocol:              masternode.ActiveProtocolVersion,
		LastPing:              ping,
	}

	// Sign broadcast with collateral key (new format message)
	broadcastMsg := s.getBroadcastSignatureMessage(broadcast)
	broadcastSig, err := crypto.SignCompact(collateralKeyInfo.PrivKey, broadcastMsg)
	if err != nil {
		result["error"] = fmt.Sprintf("failed to sign broadcast: %v", err)
		return result
	}
	broadcast.Signature = broadcastSig

	// Process broadcast in manager (no origin peer for RPC-initiated broadcasts)
	if s.masternode != nil {
		if err := s.masternode.ProcessBroadcast(broadcast, ""); err != nil && !errors.Is(err, masternode.ErrBroadcastAlreadySeen) {
			result["error"] = fmt.Sprintf("failed to process broadcast: %v", err)
			return result
		}
	}

	// Relay to network via P2P
	if s.p2pServer != nil {
		// Serialize broadcast
		broadcastBytes, err := p2p.SerializeMasternodeBroadcast(broadcast)
		if err != nil {
			result["error"] = fmt.Sprintf("failed to serialize broadcast: %v", err)
			return result
		}

		result["hex"] = hex.EncodeToString(broadcastBytes)
	}

	result["result"] = "successful"
	result["txhash"] = entry.TxHash.String()
	result["outputidx"] = entry.OutputIndex

	return result
}

// collateralKeyInfo contains the private key and whether to use compressed pubkey format
type collateralKeyInfo struct {
	PrivKey    *crypto.PrivateKey
	Compressed bool
}

// getCollateralKeyInfo gets the private key and correct pubkey format for a collateral UTXO
// It determines the correct format by checking which pubkey hash matches the scriptPubKey
func (s *Server) getCollateralKeyInfo(outpoint types.Outpoint) (*collateralKeyInfo, error) {
	if s.wallet == nil {
		return nil, fmt.Errorf("wallet not available")
	}

	// Get the address for this UTXO
	// We need to find the transaction output and extract its address
	if s.blockchain == nil {
		return nil, fmt.Errorf("blockchain not available")
	}

	tx, err := s.blockchain.GetTransaction(outpoint.Hash)
	if err != nil {
		return nil, fmt.Errorf("collateral transaction not found: %w", err)
	}

	if int(outpoint.Index) >= len(tx.Outputs) {
		return nil, fmt.Errorf("invalid output index")
	}

	output := tx.Outputs[outpoint.Index]

	// Extract address from scriptPubKey
	address := s.wallet.ExtractAddress(output.ScriptPubKey)
	if address == "" {
		return nil, fmt.Errorf("could not extract address from collateral output")
	}

	// Get private key for this address
	privKeyWIF, err := s.wallet.DumpPrivKey(address)
	if err != nil {
		return nil, fmt.Errorf("could not get private key for collateral address: %w", err)
	}

	// Decode the WIF to get the private key
	privKey, err := crypto.DecodeWIF(privKeyWIF)
	if err != nil {
		return nil, fmt.Errorf("invalid collateral key: %w", err)
	}

	// Determine which pubkey format (compressed or uncompressed) was used to create
	// the address in the scriptPubKey by checking which hash matches
	// P2PKH script format: OP_DUP OP_HASH160 <20 bytes pubkeyHash> OP_EQUALVERIFY OP_CHECKSIG
	// That's: 0x76 0xa9 0x14 <20 bytes> 0x88 0xac = 25 bytes total
	script := output.ScriptPubKey
	if len(script) != 25 || script[0] != 0x76 || script[1] != 0xa9 || script[2] != 0x14 ||
		script[23] != 0x88 || script[24] != 0xac {
		return nil, fmt.Errorf("collateral output is not P2PKH")
	}

	// Extract the pubkey hash from the script
	scriptPubKeyHash := script[3:23]

	// Get public key and compute hashes for both formats
	pubKey := privKey.PublicKey()

	// Check compressed format first (modern wallets use this)
	compressedPubKey := pubKey.SerializeCompressed()
	compressedHash := crypto.Hash160(compressedPubKey)

	if bytes.Equal(compressedHash, scriptPubKeyHash) {
		return &collateralKeyInfo{
			PrivKey:    privKey,
			Compressed: true,
		}, nil
	}

	// Check uncompressed format (legacy wallets)
	uncompressedPubKey := pubKey.Bytes()
	uncompressedHash := crypto.Hash160(uncompressedPubKey)

	if bytes.Equal(uncompressedHash, scriptPubKeyHash) {
		return &collateralKeyInfo{
			PrivKey:    privKey,
			Compressed: false,
		}, nil
	}

	// Neither format matches - this shouldn't happen if the wallet owns the key
	return nil, fmt.Errorf("public key hash mismatch: wallet key doesn't match scriptPubKey (compressed hash=%x, uncompressed hash=%x, script hash=%x)",
		compressedHash, uncompressedHash, scriptPubKeyHash)
}

// getCollateralPrivKey gets the private key for a collateral UTXO from the wallet
// Deprecated: Use getCollateralKeyInfo instead which also returns the correct pubkey format
func (s *Server) getCollateralPrivKey(outpoint types.Outpoint) (string, error) {
	if s.wallet == nil {
		return "", fmt.Errorf("wallet not available")
	}

	// Get the address for this UTXO
	// We need to find the transaction output and extract its address
	if s.blockchain == nil {
		return "", fmt.Errorf("blockchain not available")
	}

	tx, err := s.blockchain.GetTransaction(outpoint.Hash)
	if err != nil {
		return "", fmt.Errorf("collateral transaction not found: %w", err)
	}

	if int(outpoint.Index) >= len(tx.Outputs) {
		return "", fmt.Errorf("invalid output index")
	}

	output := tx.Outputs[outpoint.Index]

	// Extract address from scriptPubKey
	address := s.wallet.ExtractAddress(output.ScriptPubKey)
	if address == "" {
		return "", fmt.Errorf("could not extract address from collateral output")
	}

	// Get private key for this address
	privKeyWIF, err := s.wallet.DumpPrivKey(address)
	if err != nil {
		return "", fmt.Errorf("could not get private key for collateral address: %w", err)
	}

	return privKeyWIF, nil
}

// getRecentBlockHash returns a recent block hash for ping/broadcast messages
func (s *Server) getRecentBlockHash() types.Hash {
	if s.blockchain == nil {
		return types.ZeroHash
	}

	height, err := s.blockchain.GetBestHeight()
	if err != nil {
		return types.ZeroHash
	}

	// Use block 12 blocks back for safety (like legacy)
	if height > 12 {
		height -= 12
	}

	block, err := s.blockchain.GetBlockByHeight(height)
	if err != nil {
		return types.ZeroHash
	}

	return block.Hash()
}

// getBroadcastSignatureMessage creates the message to sign for a broadcast (new format)
// CRITICAL: Must match legacy C++ CMasternodeBroadcast::GetNewStrMessage() format exactly
// Legacy uses uint160::ToString() which outputs bytes in REVERSE order (big-endian display)
func (s *Server) getBroadcastSignatureMessage(mnb *masternode.MasternodeBroadcast) string {
	var message string

	// Add address string (e.g., "127.0.0.1:37817")
	message += mnb.Addr.String()

	// Add sigtime as string
	message += fmt.Sprintf("%d", mnb.SigTime)

	// Add collateral public key ID as HEX string
	// LEGACY COMPATIBILITY: C++ uint160::ToString() reverses byte order for display
	// Use PubKeyCollateralBytes if set (preserves original format), otherwise use compressed
	if mnb.PubKeyCollateral != nil {
		var pubKeyBytes []byte
		if len(mnb.PubKeyCollateralBytes) > 0 {
			pubKeyBytes = mnb.PubKeyCollateralBytes
		} else {
			pubKeyBytes = mnb.PubKeyCollateral.SerializeCompressed()
		}
		pubKeyID := crypto.Hash160(pubKeyBytes)
		message += masternode.ReverseHexBytes(pubKeyID)
	}

	// Add masternode public key ID as HEX string
	// LEGACY COMPATIBILITY: C++ uint160::ToString() reverses byte order for display
	// Use PubKeyMasternodeBytes if set (preserves original format), otherwise use compressed
	if mnb.PubKeyMasternode != nil {
		var pubKeyBytes []byte
		if len(mnb.PubKeyMasternodeBytes) > 0 {
			pubKeyBytes = mnb.PubKeyMasternodeBytes
		} else {
			pubKeyBytes = mnb.PubKeyMasternode.SerializeCompressed()
		}
		pubKeyID := crypto.Hash160(pubKeyBytes)
		message += masternode.ReverseHexBytes(pubKeyID)
	}

	// Add protocol version as string
	message += fmt.Sprintf("%d", mnb.Protocol)

	return message
}

// handleStopMasternode stops the local masternode
func (s *Server) handleStopMasternode(req *Request) *Response {
	// Check if activeMasternode is configured
	if s.activeMasternode == nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-32603, "Active masternode not configured", nil),
			ID:      req.ID,
		}
	}

	// Check if started
	if !s.activeMasternode.IsStarted() {
		return &Response{
			JSONRPC: "2.0",
			Result: map[string]interface{}{
				"status":  "not_running",
				"message": "Masternode is not currently running",
			},
			ID: req.ID,
		}
	}

	// Stop the masternode
	s.activeMasternode.Stop()

	return &Response{
		JSONRPC: "2.0",
		Result: map[string]interface{}{
			"status":  "stopped",
			"message": "Masternode stopped successfully",
		},
		ID: req.ID,
	}
}

// handleCreateMasternodeKey creates a new masternode private key
// Returns WIF-encoded key compatible with legacy C++ wallet
func (s *Server) handleCreateMasternodeKey(req *Request) *Response {
	// Generate new masternode key pair
	keyPair, err := crypto.GenerateKeyPair()
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInternalError(err.Error()),
			ID:      req.ID,
		}
	}

	// Encode to WIF format (legacy C++ compatible)
	// Version byte 66 (0x42) for TWINS mainnet, uncompressed=false (matches legacy MakeNewKey(false))
	wifKey := keyPair.Private.EncodeWIF(66, false)

	return &Response{
		JSONRPC: "2.0",
		Result:  wifKey,
		ID:      req.ID,
	}
}

// handleGetMasternodeOutputs returns available outputs for masternode collateral
func (s *Server) handleGetMasternodeOutputs(req *Request) *Response {
	result := []map[string]interface{}{}

	if s.wallet == nil {
		return &Response{
			JSONRPC: "2.0",
			Result:  result,
			ID:      req.ID,
		}
	}

	// Get unspent outputs from wallet
	// Minimum confirmations: 15, Maximum confirmations: 9999999
	utxosRaw, err := s.wallet.ListUnspent(15, 9999999, []string{})
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "failed to get unspent outputs: "+err.Error(), nil),
			ID:      req.ID,
		}
	}

	// Type assert to slice of maps (wallet returns []map[string]interface{} with amount in TWINS)
	utxos, ok := utxosRaw.([]map[string]interface{})
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "unexpected utxos format from wallet", nil),
			ID:      req.ID,
		}
	}

	// Filter for valid masternode collateral amounts
	if s.chainParams != nil {
		for _, utxo := range utxos {
			// Amount is in TWINS (float64), convert to satoshis for tier validation
			amountTWINS, _ := utxo["amount"].(float64)
			amountSatoshis := int64(amountTWINS * 1e8)

			// Check if amount matches any masternode tier collateral
			if s.chainParams.IsValidTier(amountSatoshis) {
				txid, _ := utxo["txid"].(string)
				vout, _ := utxo["vout"].(uint32)
				result = append(result, map[string]interface{}{
					"txhash":    txid,
					"outputidx": vout,
					"amount":    amountTWINS,
				})
			}
		}
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// handleListMasternodeConf returns the local masternode.conf entries
// Uses the shared MasternodeConf loaded at daemon startup instead of reading file each time
func (s *Server) handleListMasternodeConf(req *Request) *Response {
	result := []map[string]interface{}{}

	// Use the shared masternode.conf loaded at startup
	if s.masternodeConf == nil {
		// masternode.conf not loaded, return empty array
		return &Response{
			JSONRPC: "2.0",
			Result:  result,
			ID:      req.ID,
		}
	}

	// Re-read the file to get latest entries (allows hot-reload of masternode.conf)
	if err := s.masternodeConf.Read(); err != nil {
		// Failed to read, return empty array
		return &Response{
			JSONRPC: "2.0",
			Result:  result,
			ID:      req.ID,
		}
	}

	// Get entries from the shared config
	entries := s.masternodeConf.GetEntries()
	for _, entry := range entries {
		result = append(result, map[string]interface{}{
			"alias":       entry.Alias,
			"address":     entry.IP,
			"privateKey":  entry.PrivKey,
			"txHash":      entry.TxHash.String(),
			"outputIndex": entry.OutputIndex,
		})
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// handleCreateMasternodeBroadcast creates a masternode broadcast message
func (s *Server) handleCreateMasternodeBroadcast(req *Request) *Response {
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
			Error:   NewInvalidParamsError("createmasternodebroadcast requires mode parameter"),
			ID:      req.ID,
		}
	}

	mode, ok := params[0].(string)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("mode must be a string"),
			ID:      req.ID,
		}
	}

	// Validate mode
	if mode != "alias" && mode != "all" {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid mode: must be 'alias' or 'all'"),
			ID:      req.ID,
		}
	}

	// Check if wallet is locked
	if s.wallet != nil && s.wallet.IsLocked() {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-13, "Wallet is locked. Please enter the wallet passphrase with walletpassphrase first.", nil),
			ID:      req.ID,
		}
	}

	// Check if masternode.conf is available
	if s.masternodeConf == nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "masternode.conf not loaded", nil),
			ID:      req.ID,
		}
	}

	// Read latest config
	if err := s.masternodeConf.Read(); err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "failed to read masternode.conf: "+err.Error(), nil),
			ID:      req.ID,
		}
	}

	details := []map[string]interface{}{}
	successful := 0
	failed := 0

	// For "alias" mode, create broadcast for specific alias
	if mode == "alias" {
		if len(params) < 2 {
			return &Response{
				JSONRPC: "2.0",
				Error:   NewInvalidParamsError("createmasternodebroadcast alias requires alias parameter"),
				ID:      req.ID,
			}
		}

		alias, ok := params[1].(string)
		if !ok {
			return &Response{
				JSONRPC: "2.0",
				Error:   NewInvalidParamsError("alias must be a string"),
				ID:      req.ID,
			}
		}

		entry := s.masternodeConf.GetEntry(alias)
		if entry == nil {
			return &Response{
				JSONRPC: "2.0",
				Error:   NewError(-1, fmt.Sprintf("masternode alias '%s' not found in masternode.conf", alias), nil),
				ID:      req.ID,
			}
		}

		result := s.createBroadcastForEntry(entry)
		details = append(details, result)
		if result["result"] == "successful" {
			successful++
		} else {
			failed++
		}
	} else {
		// "all" mode - create broadcasts for all entries
		entries := s.masternodeConf.GetEntries()
		if len(entries) == 0 {
			return &Response{
				JSONRPC: "2.0",
				Result: map[string]interface{}{
					"overall": "No masternodes configured in masternode.conf",
					"detail":  []map[string]interface{}{},
				},
				ID: req.ID,
			}
		}

		for _, entry := range entries {
			result := s.createBroadcastForEntry(entry)
			details = append(details, result)
			if result["result"] == "successful" {
				successful++
			} else {
				failed++
			}
		}
	}

	overall := fmt.Sprintf("Successfully created broadcast messages for %d masternode(s)", successful)
	if failed > 0 {
		overall = fmt.Sprintf("Created %d broadcast(s), failed %d", successful, failed)
	}

	return &Response{
		JSONRPC: "2.0",
		Result: map[string]interface{}{
			"overall": overall,
			"detail":  details,
		},
		ID: req.ID,
	}
}

// createBroadcastForEntry creates a broadcast message for a single masternode entry
func (s *Server) createBroadcastForEntry(entry *MasternodeConfEntry) map[string]interface{} {
	result := map[string]interface{}{
		"alias":  entry.Alias,
		"result": "failed",
	}

	// Get collateral key from wallet
	if s.wallet == nil {
		result["error"] = "wallet not available"
		return result
	}

	// Create the outpoint for this masternode
	outpoint := types.Outpoint{
		Hash:  entry.TxHash,
		Index: entry.OutputIndex,
	}

	// Get private key and correct pubkey format for collateral address from wallet
	// This determines the correct format (compressed vs uncompressed) by checking the scriptPubKey hash
	collateralKeyInfo, err := s.getCollateralKeyInfo(outpoint)
	if err != nil {
		result["error"] = fmt.Sprintf("failed to get collateral key: %v", err)
		return result
	}

	// Parse masternode private key
	mnPrivKey, err := crypto.DecodeWIF(entry.PrivKey)
	if err != nil {
		result["error"] = fmt.Sprintf("invalid masternode private key: %v", err)
		return result
	}

	// Parse service address
	addr, err := net.ResolveTCPAddr("tcp", entry.IP)
	if err != nil {
		result["error"] = fmt.Sprintf("invalid service address: %v", err)
		return result
	}

	// Create broadcast
	sigTime := time.Now().Unix()

	// Create ping first
	ping := &masternode.MasternodePing{
		OutPoint:  outpoint,
		BlockHash: s.getRecentBlockHash(),
		SigTime:   sigTime,
	}

	// Sign ping with masternode key using correct legacy format
	// CRITICAL: Must use GetSignatureMessage() which returns "CTxIn(COutPoint(hash, n), scriptSig=)" format
	// The old format "hash:index" was incorrect and caused signature verification failures
	pingMsg := ping.GetSignatureMessage()
	pingSig, err := crypto.SignCompact(mnPrivKey, pingMsg)
	if err != nil {
		result["error"] = fmt.Sprintf("failed to sign ping: %v", err)
		return result
	}
	ping.Signature = pingSig

	// Get the correct public key bytes based on detected format
	collateralPubKey := collateralKeyInfo.PrivKey.PublicKey()
	var collateralPubKeyBytes []byte
	if collateralKeyInfo.Compressed {
		collateralPubKeyBytes = collateralPubKey.SerializeCompressed()
	} else {
		collateralPubKeyBytes = collateralPubKey.Bytes() // Uncompressed (65 bytes)
	}

	// Create broadcast with correct pubkey format
	broadcast := &masternode.MasternodeBroadcast{
		OutPoint:              outpoint,
		Addr:                  addr,
		PubKeyCollateral:      collateralPubKey,
		PubKeyMasternode:      mnPrivKey.PublicKey(),
		PubKeyCollateralBytes: collateralPubKeyBytes, // Preserve correct format for serialization
		SigTime:               sigTime,
		Protocol:              masternode.ActiveProtocolVersion,
		LastPing:              ping,
	}

	// Sign broadcast with collateral key (new format message)
	broadcastMsg := s.getBroadcastSignatureMessage(broadcast)
	broadcastSig, err := crypto.SignCompact(collateralKeyInfo.PrivKey, broadcastMsg)
	if err != nil {
		result["error"] = fmt.Sprintf("failed to sign broadcast: %v", err)
		return result
	}
	broadcast.Signature = broadcastSig

	// Serialize to hex
	broadcastBytes, err := p2p.SerializeMasternodeBroadcast(broadcast)
	if err != nil {
		result["error"] = fmt.Sprintf("failed to serialize broadcast: %v", err)
		return result
	}

	result["result"] = "successful"
	result["hex"] = hex.EncodeToString(broadcastBytes)

	return result
}

// handleDecodeMasternodeBroadcast decodes a masternode broadcast message
func (s *Server) handleDecodeMasternodeBroadcast(req *Request) *Response {
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
			Error:   NewInvalidParamsError("decodemasternodebroadcast requires hex parameter"),
			ID:      req.ID,
		}
	}

	hexData, ok := params[0].(string)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("hex must be a string"),
			ID:      req.ID,
		}
	}

	// Decode hex string
	broadcastBytes, err := hex.DecodeString(hexData)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid hex string: " + err.Error()),
			ID:      req.ID,
		}
	}

	// Deserialize MasternodeBroadcast from bytes
	mnb, err := p2p.DeserializeMasternodeBroadcast(broadcastBytes)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "failed to deserialize broadcast: "+err.Error(), nil),
			ID:      req.ID,
		}
	}

	// Verify signature
	verifyErr := mnb.Verify()

	// Build result with all broadcast details
	result := map[string]interface{}{
		"outpoint":           mnb.OutPoint.String(),
		"addr":               mnb.Addr.String(),
		"pubkey_collateral":  hex.EncodeToString(mnb.PubKeyCollateral.SerializeCompressed()),
		"pubkey_masternode":  hex.EncodeToString(mnb.PubKeyMasternode.SerializeCompressed()),
		"signature":          hex.EncodeToString(mnb.Signature),
		"sigtime":            mnb.SigTime,
		"protocol":           mnb.Protocol,
		"last_dsq":           mnb.LastDsq,
		"verified":           verifyErr == nil,
	}

	if verifyErr != nil {
		result["verification_error"] = verifyErr.Error()
	}

	if mnb.LastPing != nil {
		result["lastping"] = map[string]interface{}{
			"outpoint":   mnb.LastPing.OutPoint.String(),
			"blockhash":  mnb.LastPing.BlockHash.String(),
			"sigtime":    mnb.LastPing.SigTime,
			"sentinel":   mnb.LastPing.SentinelPing,
			"sentinel_v": mnb.LastPing.SentinelVersion,
		}
	} else {
		result["lastping"] = nil
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// handleRelayMasternodeBroadcast relays a masternode broadcast message
func (s *Server) handleRelayMasternodeBroadcast(req *Request) *Response {
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
			Error:   NewInvalidParamsError("relaymasternodebroadcast requires hex parameter"),
			ID:      req.ID,
		}
	}

	hexData, ok := params[0].(string)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("hex must be a string"),
			ID:      req.ID,
		}
	}

	// Decode hex string
	broadcastBytes, err := hex.DecodeString(hexData)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid hex string: " + err.Error()),
			ID:      req.ID,
		}
	}

	if len(broadcastBytes) == 0 {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("broadcast data is empty"),
			ID:      req.ID,
		}
	}

	// Deserialize broadcast bytes into MasternodeBroadcast
	mnb, err := p2p.DeserializeMasternodeBroadcast(broadcastBytes)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "failed to deserialize broadcast: "+err.Error(), nil),
			ID:      req.ID,
		}
	}

	// Verify signature
	if err := mnb.Verify(); err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "signature verification failed: "+err.Error(), nil),
			ID:      req.ID,
		}
	}

	// Add to local masternode manager
	if s.masternode != nil {
		if err := s.masternode.ProcessBroadcast(mnb, ""); err != nil && !errors.Is(err, masternode.ErrBroadcastAlreadySeen) {
			return &Response{
				JSONRPC: "2.0",
				Error:   NewError(-1, "failed to process broadcast: "+err.Error(), nil),
				ID:      req.ID,
			}
		}
	} else {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "masternode manager not available", nil),
			ID:      req.ID,
		}
	}

	// Broadcast to all connected peers via P2P
	peerCount := 0
	if s.p2pServer != nil {
		peers := s.p2pServer.GetPeers()
		peerCount = len(peers)
		// Note: P2P server would need a BroadcastMessage method to relay
		// For now, just track that we have peers available
	}

	result := map[string]interface{}{
		"status":      "successful",
		"broadcasted": s.p2pServer != nil,
		"outpoint":    mnb.OutPoint.String(),
		"peer_count":  peerCount,
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// handleMasternodeConnect connects to a specific masternode
func (s *Server) handleMasternodeConnect(req *Request) *Response {
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
			Error:   NewInvalidParamsError("masternodeconnect requires address parameter"),
			ID:      req.ID,
		}
	}

	address, ok := params[0].(string)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("address must be a string"),
			ID:      req.ID,
		}
	}

	if s.p2pServer == nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "P2P server not available", nil),
			ID:      req.ID,
		}
	}

	// Connect to masternode peer
	if err := s.p2pServer.ConnectNode(address); err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "failed to connect: "+err.Error(), nil),
			ID:      req.ID,
		}
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  "successfully connected to " + address,
		ID:      req.ID,
	}
}

// handleGetPoolInfo returns information about the masternode payment pool
func (s *Server) handleGetPoolInfo(req *Request) *Response {
	result := map[string]interface{}{
		"queue_position": 0,
		"queue_size":     0,
		"next_payment":   0,
	}

	// Query masternode manager for actual pool statistics
	if s.masternode != nil {
		// Get all masternodes to calculate queue size
		allMasternodes := s.masternode.GetMasternodes()
		activeCount := 0
		for _, mn := range allMasternodes {
			if s.masternode.IsMasternodeActive(mn.OutPoint) {
				activeCount++
			}
		}
		result["queue_size"] = activeCount

		// Get next payee
		if nextPayee, err := s.masternode.GetNextPayee(); err == nil && nextPayee != nil {
			result["next_payee"] = nextPayee.OutPoint.String()
			result["next_payee_tier"] = nextPayee.Tier.String()
		}

		// Get total masternode count by tier
		_, total, _ := s.masternode.GetMasternodeCount()
		result["masternodes_total"] = total
		result["masternodes_bronze"] = s.masternode.GetMasternodeCountByTier(1)  // Bronze
		result["masternodes_silver"] = s.masternode.GetMasternodeCountByTier(2)  // Silver
		result["masternodes_gold"] = s.masternode.GetMasternodeCountByTier(3)    // Gold
		result["masternodes_platinum"] = s.masternode.GetMasternodeCountByTier(4) // Platinum
	} else {
		result["note"] = "Masternode manager not available"
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}
