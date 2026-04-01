package rpc

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/twins-dev/twins-core/internal/blockchain"
	"github.com/twins-dev/twins-core/internal/consensus"
	"github.com/twins-dev/twins-core/pkg/types"
)

// registerMiningHandlers registers mining and staking related RPC handlers
func (s *Server) registerMiningHandlers() {
	s.RegisterHandler("getnetworkhashps", s.handleGetNetworkHashPS)
	s.RegisterHandler("getmininginfo", s.handleGetMiningInfo)
	s.RegisterHandler("getstakingstatus", s.handleGetStakingStatus)
	s.RegisterHandler("getstakinginfo", s.handleGetStakingStatus) // Alias for compatibility
	s.RegisterHandler("submitblock", s.handleSubmitBlock)
	s.RegisterHandler("getblocktemplate", s.handleGetBlockTemplate)
	s.RegisterHandler("prioritisetransaction", s.handlePrioritiseTransaction)
	s.RegisterHandler("estimatefee", s.handleEstimateFee)
	s.RegisterHandler("estimatepriority", s.handleEstimatePriority)
	s.RegisterHandler("setgenerate", s.handleSetGenerate)
	s.RegisterHandler("getgenerate", s.handleGetGenerate)
	s.RegisterHandler("gethashespersec", s.handleGetHashesPerSec)
}

// handleGetNetworkHashPS returns the estimated network hashes per second
func (s *Server) handleGetNetworkHashPS(req *Request) *Response {
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

	// Default: 120 blocks, current height
	blocks := 120
	height := -1

	if len(params) > 0 {
		if b, ok := params[0].(float64); ok {
			blocks = int(b)
		}
	}

	if len(params) > 1 {
		if h, ok := params[1].(float64); ok {
			height = int(h)
		}
	}

	// Calculate network hashrate
	// For PoS, this represents stake weight rather than traditional hash power
	hashPS := s.calculateNetworkHashPS(blocks, height)

	return &Response{
		JSONRPC: "2.0",
		Result:  hashPS,
		ID:      req.ID,
	}
}

// handleGetMiningInfo returns mining-related information
func (s *Server) handleGetMiningInfo(req *Request) *Response {
	// Get mempool size
	txs := s.mempool.GetTransactions()
	mempoolSize := len(txs)

	// Get blockchain info
	height, _ := s.blockchain.GetBestHeight()

	// Get last block for size and tx count
	lastBlock, err := s.blockchain.GetBestBlock()
	lastBlockSize := 0
	lastBlockTxCount := 0
	if err == nil && lastBlock != nil {
		lastBlockSize = lastBlock.SerializeSize()
		lastBlockTxCount = len(lastBlock.Transactions)
	}

	// Get difficulty from blockchain
	difficulty := 0.0
	if s.blockchain != nil {
		diff, err := s.blockchain.GetDifficulty()
		if err == nil {
			difficulty = diff
		}
	}

	// Detect network type and chain name
	testnet := false
	chainName := "main"
	if s.chainParams != nil {
		testnet = (s.chainParams.Name == "testnet")
		chainName = s.chainParams.Name
	}

	// Calculate stake weight (network weight from consensus)
	stakeWeight := int64(0)
	if s.consensus != nil {
		stakeWeight = s.consensus.GetNetworkStakeWeight()
	}

	// Get warnings/errors (simple implementation)
	errors := s.getChainWarnings()

	result := map[string]interface{}{
		"blocks":           height,
		"currentblocksize": lastBlockSize,
		"currentblocktx":   lastBlockTxCount,
		"difficulty":       difficulty,
		"errors":           errors,
		"networkhashps":    s.calculateNetworkHashPS(120, -1),
		"pooledtx":         mempoolSize,
		"testnet":          testnet,
		"chain":            chainName,
		"stakeweight":      stakeWeight,
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// handleGetStakingStatus returns staking status information
func (s *Server) handleGetStakingStatus(req *Request) *Response {
	// Check wallet lock status
	walletUnlocked := false
	mintableCoins := false
	enoughCoins := false

	if s.wallet != nil {
		// Wallet is present and can provide staking info
		walletUnlocked = !s.wallet.IsLocked()

		// Check if wallet has mature coins for staking
		if walletUnlocked {
			// Get balance with minimum 1 confirmation
			bal := s.wallet.GetBalance()
			if bal != nil && bal.Confirmed > 0 {
				mintableCoins = true
				// Check if balance exceeds reserve (assume 1 TWINS reserve)
				reserve := int64(100000000) // 1 TWINS in satoshis
				enoughCoins = bal.Confirmed > reserve
			}
		}
	}

	// Check masternode sync status (assume synced if we have connections)
	mnSync := s.hasConnections()

	result := map[string]interface{}{
		"validtime":       true,                // Chain tip is valid for staking
		"haveconnections": s.hasConnections(),  // Network connections present
		"walletunlocked":  walletUnlocked,      // Wallet is unlocked for staking
		"mintablecoins":   mintableCoins,       // Wallet has mature coins for staking
		"enoughcoins":     enoughCoins,         // Balance exceeds reserve
		"mnsync":          mnSync,              // Masternode list synced
		"staking status":  s.isStaking(),       // Currently staking
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// handleSubmitBlock submits a new block to the network
func (s *Server) handleSubmitBlock(req *Request) *Response {
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
			Error:   NewInvalidParamsError("submitblock requires hex data parameter"),
			ID:      req.ID,
		}
	}

	hexData, ok := params[0].(string)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("hex data must be a string"),
			ID:      req.ID,
		}
	}

	// Decode hex data to bytes
	blockBytes, err := hex.DecodeString(hexData)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid hex data: " + err.Error()),
			ID:      req.ID,
		}
	}

	// Deserialize block
	block, err := types.DeserializeBlock(blockBytes)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("failed to deserialize block: " + err.Error()),
			ID:      req.ID,
		}
	}

	// Validate block through consensus
	if s.consensus != nil {
		if err := s.consensus.ValidateBlock(block); err != nil {
			return &Response{
				JSONRPC: "2.0",
				Error:   NewError(-25, "block validation failed: "+err.Error(), nil),
				ID:      req.ID,
			}
		}
	}

	// Process block through blockchain layer
	if s.blockchain == nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "blockchain not initialized", nil),
			ID:      req.ID,
		}
	}

	if err := s.blockchain.ProcessBlock(block); err != nil {
		// Check for duplicate block using typed error (not an error, just already processed)
		if errors.Is(err, blockchain.ErrBlockExists) {
			// Block already in chain - return success (Bitcoin Core behavior)
			return &Response{
				JSONRPC: "2.0",
				Result:  nil,
				ID:      req.ID,
			}
		}
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-25, "block processing failed: "+err.Error(), nil),
			ID:      req.ID,
		}
	}

	// Relay block to network peers
	if s.p2pServer != nil {
		s.p2pServer.RelayBlock(block)
		s.logger.WithField("hash", block.Header.Hash().String()).Info("Block submitted and relayed to network")
	}

	// Return null on success (Bitcoin Core compatibility)
	return &Response{
		JSONRPC: "2.0",
		Result:  nil,
		ID:      req.ID,
	}
}

// handleGetBlockTemplate returns a block template for mining/staking
func (s *Server) handleGetBlockTemplate(req *Request) *Response {
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

	// Get best block hash
	bestHash, err := s.blockchain.GetBestBlockHash()
	prevBlockHash := ""
	if err == nil {
		prevBlockHash = bestHash.String()
	}

	// Get mempool transactions
	mempoolTxs := s.mempool.GetTransactions()
	transactions := make([]interface{}, 0, len(mempoolTxs))
	for _, tx := range mempoolTxs {
		// Serialize transaction to hex format
		txBytes, err := tx.Serialize()
		if err != nil {
			s.logger.WithError(err).Warn("Failed to serialize transaction")
			continue
		}
		txHex := hex.EncodeToString(txBytes)

		// Calculate transaction fee (sum of inputs - sum of outputs)
		fee := s.calculateTransactionFee(tx)

		transactions = append(transactions, map[string]interface{}{
			"data":    txHex,
			"txid":    tx.Hash().String(),
			"fee":     fee,
			"sigops":  0, // Simplified - would need proper sigop counting
			"weight":  tx.SerializeSize(),
			"depends": []interface{}{}, // Transaction dependencies
		})
	}

	// Get current target/bits
	bits := ""
	target := ""
	if s.blockchain != nil {
		diff, err := s.blockchain.GetDifficulty()
		if err == nil {
			// Convert difficulty to compact bits format (uint32)
			diffBits := uint32(diff)
			bits = encodeBits(diffBits)
			target = encodeTarget(diffBits)
		}
	}

	// Get current height
	height, _ := s.blockchain.GetBestHeight()
	nextHeight := height + 1

	// Calculate block reward using height-adjusted schedule
	// Uses the TWINS reward schedule from legacy GetBlockValue()
	blockReward := consensus.GetBlockValue(nextHeight)

	// Calculate minimum time (previous block time + minimum interval)
	minTime := time.Now().Unix()
	if bestBlock, err := s.blockchain.GetBestBlock(); err == nil && bestBlock != nil {
		minTime = int64(bestBlock.Header.Timestamp) + 1
		if s.chainParams != nil {
			minTime = int64(bestBlock.Header.Timestamp) + int64(s.chainParams.MinBlockInterval.Seconds())
		}
	}

	// Current time
	curTime := time.Now().Unix()

	// Size and sig op limits
	sizeLimit := uint32(1000000) // 1 MB default
	if s.chainParams != nil {
		sizeLimit = s.chainParams.MaxBlockSize
	}

	result := map[string]interface{}{
		"version":           1,
		"previousblockhash": prevBlockHash,
		"transactions":      transactions,
		"coinbaseaux":       map[string]interface{}{},
		"coinbasevalue":     blockReward,
		"target":            target,
		"mintime":           minTime,
		"mutable":           []string{"time", "transactions", "prevblock"},
		"noncerange":        "00000000ffffffff",
		"sigoplimit":        20000,
		"sizelimit":         sizeLimit,
		"curtime":           curTime,
		"bits":              bits,
		"height":            nextHeight,
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// Helper functions

// calculateNetworkHashPS calculates network hash rate over specified blocks
func (s *Server) calculateNetworkHashPS(blocks int, height int) float64 {
	// For PoS networks, this represents stake weight rather than hash power
	// Use consensus GetNetworkStakeWeight to get network weight
	if s.consensus != nil {
		// Return network weight as approximation of "hash power" for PoS
		return float64(s.consensus.GetNetworkStakeWeight())
	}

	// Fallback: try to calculate from blockchain
	// Get end height (use current if height == -1)
	endHeight := uint32(height)
	if height == -1 {
		h, err := s.blockchain.GetBestHeight()
		if err != nil {
			return 0.0
		}
		endHeight = h
	}

	// Calculate start height
	startHeight := uint32(0)
	if int(endHeight) > blocks {
		startHeight = endHeight - uint32(blocks)
	}

	// Get time difference between start and end blocks
	startBlock, err1 := s.blockchain.GetBlockByHeight(startHeight)
	endBlock, err2 := s.blockchain.GetBlockByHeight(endHeight)

	if err1 != nil || err2 != nil || startBlock == nil || endBlock == nil {
		return 0.0
	}

	timeDiff := int64(endBlock.Header.Timestamp) - int64(startBlock.Header.Timestamp)
	if timeDiff <= 0 {
		return 0.0
	}

	// For PoS, calculate approximate stake weight based on difficulty
	// This is a simplified calculation
	avgDifficulty := float64(endBlock.Header.Bits)
	hashesPerSecond := avgDifficulty * float64(blocks) / float64(timeDiff)

	return hashesPerSecond
}

// hasConnections checks if there are active network connections
func (s *Server) hasConnections() bool {
	if s.p2pServer != nil {
		peers := s.p2pServer.GetPeers()
		return len(peers) > 0
	}
	return false
}

// isStaking checks if the node is currently staking
func (s *Server) isStaking() bool {
	// Check consensus engine for staking status
	if s.consensus != nil {
		return s.consensus.IsStaking()
	}

	// Without a consensus engine, staking is definitionally not active
	return false
}

// getChainWarnings returns any warnings about the blockchain state
func (s *Server) getChainWarnings() string {
	// Check for common warning conditions
	warnings := []string{}

	// Check if we're behind in sync
	if s.p2pServer != nil {
		peers := s.p2pServer.GetPeers()
		if len(peers) == 0 {
			warnings = append(warnings, "No network connections")
		}
	}

	// Check if chain is stalled
	bestBlock, err := s.blockchain.GetBestBlock()
	if err == nil && bestBlock != nil {
		blockAge := time.Since(time.Unix(int64(bestBlock.Header.Timestamp), 0))
		// Warn if last block is more than 1 hour old
		if blockAge > time.Hour {
			warnings = append(warnings, "Chain may be stalled - last block is old")
		}
	}

	if len(warnings) == 0 {
		return ""
	}

	// Join warnings with semicolon
	result := ""
	for i, w := range warnings {
		if i > 0 {
			result += "; "
		}
		result += w
	}

	return result
}

// encodeBits encodes difficulty as compact "bits" representation
func encodeBits(difficulty uint32) string {
	// Convert difficulty to compact hex format (4 bytes)
	bits := []byte{
		byte(difficulty >> 24),
		byte(difficulty >> 16),
		byte(difficulty >> 8),
		byte(difficulty),
	}
	return hex.EncodeToString(bits)
}

// encodeTarget encodes difficulty as full target hash
func encodeTarget(difficulty uint32) string {
	// Convert compact difficulty (bits) to full 256-bit target
	// Compact format: first byte is exponent, remaining 3 bytes are mantissa

	if difficulty == 0 {
		return types.ZeroHash.String()
	}

	// Extract exponent and mantissa from compact representation
	exponent := difficulty >> 24
	mantissa := difficulty & 0x00ffffff

	// Calculate target as mantissa * 256^(exponent - 3)
	// Target is stored as big-endian 256-bit number
	target := make([]byte, 32)

	// Place mantissa at the appropriate position
	if exponent <= 3 {
		// Mantissa fits in first few bytes
		shift := 3 - exponent
		target[shift] = byte(mantissa >> 16)
		if exponent >= 2 {
			target[shift+1] = byte(mantissa >> 8)
		}
		if exponent >= 3 {
			target[shift+2] = byte(mantissa)
		}
	} else {
		// Mantissa needs to be shifted left (little-endian: Hash stores LE internally,
		// String() reverses for display, matching C++ uint256::GetHex())
		offset := exponent - 3
		if offset < 29 {
			target[offset] = byte(mantissa)
			target[offset+1] = byte(mantissa >> 8)
			target[offset+2] = byte(mantissa >> 16)
		}
	}

	// Convert to hash string (little-endian for consistency)
	var hash types.Hash
	copy(hash[:], target)
	return hash.String()
}

// calculateTransactionFee calculates the fee for a transaction
func (s *Server) calculateTransactionFee(tx *types.Transaction) int64 {
	// Calculate total input value
	inputValue := int64(0)
	for _, input := range tx.Inputs {
		// Try to get the UTXO for this input
		utxo, err := s.blockchain.GetUTXO(input.PreviousOutput)
		if err == nil && utxo != nil {
			inputValue += utxo.Value
		}
	}

	// Calculate total output value
	outputValue := int64(0)
	for _, output := range tx.Outputs {
		outputValue += output.Value
	}

	// Fee is the difference
	fee := inputValue - outputValue
	if fee < 0 {
		return 0 // Invalid transaction, but return 0 instead of negative
	}

	return fee
}

// handlePrioritiseTransaction accepts a transaction into mined blocks at a higher priority
func (s *Server) handlePrioritiseTransaction(req *Request) *Response {
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid parameters"),
			ID:      req.ID,
		}
	}

	if len(params) < 3 {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("prioritisetransaction requires txid, priority_delta, and fee_delta parameters"),
			ID:      req.ID,
		}
	}

	// Parse txid
	txid, ok := params[0].(string)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("txid must be a string"),
			ID:      req.ID,
		}
	}

	// Parse priority_delta
	priorityDelta, ok := params[1].(float64)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("priority_delta must be a number"),
			ID:      req.ID,
		}
	}

	// Parse fee_delta (in satoshis)
	feeDelta, ok := params[2].(float64)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("fee_delta must be a number"),
			ID:      req.ID,
		}
	}

	// Parse the transaction hash
	hash, err := types.NewHashFromString(txid)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid transaction id: " + err.Error()),
			ID:      req.ID,
		}
	}

	// Update transaction priority in mempool
	if s.mempool == nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, "mempool not initialized", nil),
			ID:      req.ID,
		}
	}

	if err := s.mempool.UpdatePriority(hash, priorityDelta, int64(feeDelta)); err != nil {
		// Transaction not in mempool - still return true for Bitcoin Core compatibility
		// (priority is stored even if tx not currently in mempool)
		s.logger.WithField("txid", hash.String()).
			WithField("error", err.Error()).
			Debug("Transaction not in mempool, priority update noted")
	}

	s.logger.WithField("txid", hash.String()).
		WithField("priority_delta", priorityDelta).
		WithField("fee_delta", feeDelta).
		Info("Transaction priority updated")

	// Return true on success (legacy behavior)
	return &Response{
		JSONRPC: "2.0",
		Result:  true,
		ID:      req.ID,
	}
}

// handleEstimateFee estimates the transaction fee per kilobyte for confirmation within nblocks
func (s *Server) handleEstimateFee(req *Request) *Response {
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
			Error:   NewInvalidParamsError("estimatefee requires nblocks parameter"),
			ID:      req.ID,
		}
	}

	// Parse nblocks
	nblocks, ok := params[0].(float64)
	if !ok || nblocks < 1 {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("nblocks must be a positive number"),
			ID:      req.ID,
		}
	}

	// Calculate fee estimate based on mempool statistics
	// For TWINS, use a simplified fee estimation
	var feeRate float64

	if s.mempool != nil {
		// Get mempool transactions
		txs := s.mempool.GetTransactions()

		if len(txs) == 0 {
			// No transactions in mempool, return minimum fee
			feeRate = 0.00001 // 0.00001 TWINS per kB (minimum)
		} else {
			// Calculate average fee rate from recent transactions
			totalFees := int64(0)
			totalSize := int64(0)

			for _, tx := range txs {
				fee := s.calculateTransactionFee(tx)
				size := int64(tx.SerializeSize())
				totalFees += fee
				totalSize += size
			}

			if totalSize > 0 {
				// Convert to TWINS per kB
				feeRate = float64(totalFees) / float64(totalSize) * 1000 / 100000000

				// Apply urgency factor based on nblocks
				if nblocks <= 2 {
					feeRate *= 1.5 // Higher fee for faster confirmation
				} else if nblocks <= 6 {
					feeRate *= 1.2
				}

				// Ensure minimum fee
				if feeRate < 0.00001 {
					feeRate = 0.00001
				}
			} else {
				feeRate = 0.00001
			}
		}
	} else {
		// No mempool available, return -1 to indicate insufficient data
		feeRate = -1.0
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  feeRate,
		ID:      req.ID,
	}
}

// handleEstimatePriority estimates the priority for zero-fee confirmation within nblocks
func (s *Server) handleEstimatePriority(req *Request) *Response {
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
			Error:   NewInvalidParamsError("estimatepriority requires nblocks parameter"),
			ID:      req.ID,
		}
	}

	// Parse nblocks
	nblocks, ok := params[0].(float64)
	if !ok || nblocks < 1 {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("nblocks must be a positive number"),
			ID:      req.ID,
		}
	}

	// Calculate priority estimate
	// Priority is calculated as: sum(value * age) / size
	// For zero-fee transactions to be included
	var priority float64

	if s.mempool != nil && s.blockchain != nil {
		// Get current height for age calculation
		_, err := s.blockchain.GetBestHeight()
		if err == nil {
			// Base priority requirement (coin-days destroyed per byte)
			// Higher values mean transaction needs more coin age
			basePriority := 57600000.0 // Standard priority threshold

			// Adjust based on desired confirmation speed
			if nblocks <= 2 {
				priority = basePriority * 2 // Need higher priority for fast confirmation
			} else if nblocks <= 6 {
				priority = basePriority * 1.5
			} else {
				priority = basePriority
			}

			// Check mempool congestion
			txs := s.mempool.GetTransactions()
			if len(txs) > 100 {
				// Mempool is congested, increase priority requirement
				priority *= 1.5
			}
		} else {
			// Error getting height, return -1
			priority = -1.0
		}
	} else {
		// Insufficient data
		priority = -1.0
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  priority,
		ID:      req.ID,
	}
}

// handleSetGenerate sets generation (staking) on or off
func (s *Server) handleSetGenerate(req *Request) *Response {
	// Check if wallet is available
	if s.wallet == nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-32601, "Method not found (wallet disabled)", nil),
			ID:      req.ID,
		}
	}

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
			Error:   NewInvalidParamsError("setgenerate requires generate parameter"),
			ID:      req.ID,
		}
	}

	// Parse generate flag
	generate, ok := params[0].(bool)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("generate must be a boolean"),
			ID:      req.ID,
		}
	}

	// Parse optional genproclimit (processor limit for generation)
	genProcLimit := -1 // Default: use all processors
	if len(params) > 1 {
		if limit, ok := params[1].(float64); ok {
			genProcLimit = int(limit)
		}
	}

	// Set staking state in consensus engine
	if generate {
		if err := s.consensus.StartStaking(); err != nil {
			return &Response{
				JSONRPC: "2.0",
				Error:   NewError(-32603, "failed to start staking: "+err.Error(), nil),
				ID:      req.ID,
			}
		}
		s.logger.WithField("limit", genProcLimit).Info("Staking enabled")
	} else {
		if err := s.consensus.StopStaking(); err != nil {
			return &Response{
				JSONRPC: "2.0",
				Error:   NewError(-32603, "failed to stop staking: "+err.Error(), nil),
				ID:      req.ID,
			}
		}
		s.logger.Info("Staking disabled")
	}

	// Return null on success (legacy behavior)
	return &Response{
		JSONRPC: "2.0",
		Result:  nil,
		ID:      req.ID,
	}
}

// handleGetGenerate returns if the server is set to generate coins (stake)
func (s *Server) handleGetGenerate(req *Request) *Response {
	// Check if wallet is available
	if s.wallet == nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-32601, "Method not found (wallet disabled)", nil),
			ID:      req.ID,
		}
	}

	// Check staking status from consensus engine
	isStaking := false
	if s.consensus != nil {
		isStaking = s.consensus.IsStaking()
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  isStaking,
		ID:      req.ID,
	}
}

// handleGetHashesPerSec returns a recent hashes per second performance measurement (deprecated)
func (s *Server) handleGetHashesPerSec(req *Request) *Response {
	// Check if wallet is available
	if s.wallet == nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-32601, "Method not found (wallet disabled)", nil),
			ID:      req.ID,
		}
	}

	// This method is deprecated for PoS coins
	// Always return 0 as specified in the legacy documentation
	return &Response{
		JSONRPC: "2.0",
		Result:  float64(0),
		ID:      req.ID,
	}
}
