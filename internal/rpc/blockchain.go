package rpc

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	twinslib "github.com/twins-dev/twins-core/internal/cli"
	"github.com/twins-dev/twins-core/internal/storage/binary"
	"github.com/twins-dev/twins-core/pkg/crypto"
	"github.com/twins-dev/twins-core/pkg/types"
)

// getVersionNumber converts version string "4.0.0" to numeric format 40000
// Format: major*10000 + minor*100 + revision
func getVersionNumber() int {
	parts := strings.Split(twinslib.Version, ".")
	if len(parts) < 1 {
		return 40000 // fallback
	}
	major, _ := strconv.Atoi(parts[0])
	minor := 0
	revision := 0
	if len(parts) >= 2 {
		minor, _ = strconv.Atoi(parts[1])
	}
	if len(parts) >= 3 {
		revision, _ = strconv.Atoi(parts[2])
	}
	return major*10000 + minor*100 + revision
}

// registerBlockchainHandlers registers blockchain-related RPC handlers
func (s *Server) registerBlockchainHandlers() {
	s.RegisterHandler("getinfo", s.handleGetInfo)
	s.RegisterHandler("getblockcount", s.handleGetBlockCount)
	s.RegisterHandler("getblock", s.handleGetBlock)
	s.RegisterHandler("getblockhash", s.handleGetBlockHash)
	s.RegisterHandler("getbestblockhash", s.handleGetBestBlockHash)
	s.RegisterHandler("getblockchaininfo", s.handleGetBlockchainInfo)
	s.RegisterHandler("getdifficulty", s.handleGetDifficulty)
	s.RegisterHandler("getchaintips", s.handleGetChainTips)
	s.RegisterHandler("getblockheader", s.handleGetBlockHeader)
	s.RegisterHandler("gettxoutsetinfo", s.handleGetTxOutSetInfo)

	// Batch 2: Additional blockchain RPC methods
	s.RegisterHandler("verifychain", s.handleVerifyChain)
	s.RegisterHandler("invalidateblock", s.handleInvalidateBlock)
	s.RegisterHandler("reconsiderblock", s.handleReconsiderBlock)
	s.RegisterHandler("addcheckpoint", s.handleAddCheckpoint)
	s.RegisterHandler("getfeeinfo", s.handleGetFeeInfo)
	s.RegisterHandler("getrewardrates", s.handleGetRewardRates)
	// Note: getmininginfo, getstakingstatus, getnetworkhashps are in mining.go

	// Phase 5: Removed reindextx and reindexstatus - no longer needed with new storage
	// All transaction indexes are created during block processing
}

// handleGetInfo returns general information about the node (Bitcoin Core compatible)
func (s *Server) handleGetInfo(req *Request) *Response {
	// Get blockchain info
	height, _ := s.blockchain.GetBlockHeight()
	difficulty, _ := s.blockchain.GetDifficulty()

	// Get network info
	connectionCount := 0
	if s.p2pServer != nil {
		connectionCount = len(s.p2pServer.GetPeers())
	}

	// Get wallet info (if wallet is available)
	balance := 0.0
	walletVersion := 0
	keypoolOldest := int64(0)
	keypoolSize := 0
	if s.wallet != nil {
		walletVersion = 61000        // TWINS wallet version (matches legacy)
		bal := s.wallet.GetBalance() // Get balance
		if bal != nil {
			balance = float64(bal.Confirmed) / 1e8 // Convert satoshis to TWINS
		}
		keypoolOldest = s.wallet.GetKeypoolOldest()
		keypoolSize = s.wallet.GetKeypoolSize()
	}

	// Protocol version (use the canonical constant from p2p package)
	protocolVersion := 70928 // TWINS protocol version - must match p2p.ProtocolVersion

	result := map[string]interface{}{
		"version":         getVersionNumber(), // TWINS Core version (dynamic from cli.Version)
		"protocolversion": protocolVersion,
		"walletversion":   walletVersion,
		"balance":         balance,
		"blocks":          height,
		"timeoffset":      0,
		"connections":     connectionCount,
		"proxy":           "",
		"difficulty":      difficulty,
		"testnet":         s.chainParams != nil && s.chainParams.Name == "testnet",
		"moneysupply":     s.calculateMoneySupply(height),
		"keypoololdest":   keypoolOldest,
		"keypoolsize":     keypoolSize,
		"paytxfee":        0.0,
		"relayfee":        0.001,           // 100,000 satoshi/KB = 0.001 TWINS/KB (matches legacy minRelayTxFee)
		"staking status":  s.getStakingStatusString(), // Legacy compatibility: string not boolean
		"errors":          "",
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// handleGetBlockCount returns the current block height
func (s *Server) handleGetBlockCount(req *Request) *Response {
	height, err := s.blockchain.GetBlockHeight()
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInternalError(err.Error()),
			ID:      req.ID,
		}
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  height,
		ID:      req.ID,
	}
}

// handleGetBlock returns block information
func (s *Server) handleGetBlock(req *Request) *Response {
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
			Error:   NewInvalidParamsError("missing block hash"),
			ID:      req.ID,
		}
	}

	hashStr, ok := params[0].(string)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("block hash must be a string"),
			ID:      req.ID,
		}
	}

	hash, err := types.NewHashFromString(hashStr)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid block hash"),
			ID:      req.ID,
		}
	}

	// Check verbosity level (default: 1 - full block info)
	verbose := 1
	if len(params) > 1 {
		if v, ok := params[1].(float64); ok {
			verbose = int(v)
		}
	}

	block, err := s.blockchain.GetBlock(hash)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(CodeBlockNotFound, "Block not found", hashStr),
			ID:      req.ID,
		}
	}

	// Verbose 0: Return hex-encoded block
	if verbose == 0 {
		blockData, err := block.Serialize()
		if err != nil {
			return &Response{
				JSONRPC: "2.0",
				Error:   NewInternalError("failed to serialize block: " + err.Error()),
				ID:      req.ID,
			}
		}

		blockHex := hex.EncodeToString(blockData)
		return &Response{
			JSONRPC: "2.0",
			Result:  blockHex,
			ID:      req.ID,
		}
	}

	// Verbose 1: Return block info
	blockInfo, err := s.buildBlockInfo(block)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInternalError(err.Error()),
			ID:      req.ID,
		}
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  blockInfo,
		ID:      req.ID,
	}
}

// handleGetBlockHash returns block hash at a specific height
func (s *Server) handleGetBlockHash(req *Request) *Response {
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
			Error:   NewInvalidParamsError("missing block height"),
			ID:      req.ID,
		}
	}

	heightFloat, ok := params[0].(float64)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("block height must be a number"),
			ID:      req.ID,
		}
	}

	// Validate height range to prevent underflow/overflow
	if heightFloat < 0 || heightFloat > 4294967295 { // 0 to math.MaxUint32
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("height must be between 0 and 4294967295"),
			ID:      req.ID,
		}
	}

	height := uint32(heightFloat)
	hash, err := s.blockchain.GetBlockHash(height)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(CodeBlockHeightNotFound, "Block height not found", height),
			ID:      req.ID,
		}
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  hash.String(),
		ID:      req.ID,
	}
}

// handleGetBestBlockHash returns the hash of the best block
func (s *Server) handleGetBestBlockHash(req *Request) *Response {
	bestBlock, err := s.blockchain.GetBestBlock()
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInternalError(err.Error()),
			ID:      req.ID,
		}
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  bestBlock.Hash().String(),
		ID:      req.ID,
	}
}

// handleGetBlockchainInfo returns blockchain information
func (s *Server) handleGetBlockchainInfo(req *Request) *Response {
	bestBlock, err := s.blockchain.GetBestBlock()
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInternalError(err.Error()),
			ID:      req.ID,
		}
	}

	height, err := s.blockchain.GetBlockHeight()
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInternalError(err.Error()),
			ID:      req.ID,
		}
	}

	chainWork, _ := s.blockchain.GetChainWork()

	info := &ChainInfo{
		Chain:                "main",
		Blocks:               int64(height),
		Headers:              int64(height),
		BestBlockHash:        bestBlock.Hash().String(),
		Difficulty:           s.calculateDifficulty(bestBlock.Header.Bits),
		MedianTime:           int64(bestBlock.Header.Timestamp),
		VerificationProgress: 1.0,
		ChainWork:            chainWork,
		Pruned:               false,
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  info,
		ID:      req.ID,
	}
}

// handleGetDifficulty returns the current difficulty
func (s *Server) handleGetDifficulty(req *Request) *Response {
	// Get difficulty from blockchain interface
	difficulty, err := s.blockchain.GetDifficulty()
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInternalError(err.Error()),
			ID:      req.ID,
		}
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  difficulty,
		ID:      req.ID,
	}
}

// handleGetChainTips returns information about all known chain tips
func (s *Server) handleGetChainTips(req *Request) *Response {
	tips, err := s.blockchain.GetChainTips()
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInternalError(err.Error()),
			ID:      req.ID,
		}
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  tips,
		ID:      req.ID,
	}
}

// handleGetBlockHeader returns block header information
func (s *Server) handleGetBlockHeader(req *Request) *Response {
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
			Error:   NewInvalidParamsError("missing block hash"),
			ID:      req.ID,
		}
	}

	hashStr, ok := params[0].(string)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("block hash must be a string"),
			ID:      req.ID,
		}
	}

	hash, err := types.NewHashFromString(hashStr)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid block hash"),
			ID:      req.ID,
		}
	}

	// Check verbosity
	verbose := true
	if len(params) > 1 {
		if v, ok := params[1].(bool); ok {
			verbose = v
		}
	}

	block, err := s.blockchain.GetBlock(hash)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(CodeBlockNotFound, "Block not found", hashStr),
			ID:      req.ID,
		}
	}

	// Non-verbose: Return hex-encoded header
	if !verbose {
		headerData, err := block.Header.Serialize()
		if err != nil {
			return &Response{
				JSONRPC: "2.0",
				Error:   NewInternalError("failed to serialize block header: " + err.Error()),
				ID:      req.ID,
			}
		}

		headerHex := hex.EncodeToString(headerData)
		return &Response{
			JSONRPC: "2.0",
			Result:  headerHex,
			ID:      req.ID,
		}
	}

	// Verbose: Return header info
	// Get block height from blockchain index
	bestHeight, _ := s.blockchain.GetBlockHeight()
	height := s.getBlockHeight(hash)
	if height == 0 {
		// Fallback to best height if we can't determine block height
		height = bestHeight
	}

	confirmations := int64(0)
	if height <= bestHeight {
		confirmations = int64(bestHeight - height + 1)
	}

	// Calculate median time
	medianTime := s.calculateMedianTime(height)

	headerInfo := map[string]interface{}{
		"hash":          block.Hash().String(),
		"confirmations": confirmations,
		"height":        height,
		"version":       block.Header.Version,
		"merkleroot":    block.Header.MerkleRoot.String(),
		"time":          block.Header.Timestamp,
		"mediantime":    medianTime,
		"nonce":         block.Header.Nonce,
		"bits":          fmt.Sprintf("%08x", block.Header.Bits),
		"difficulty":    s.calculateDifficulty(block.Header.Bits),
	}

	if !block.Header.PrevBlockHash.IsZero() {
		headerInfo["previousblockhash"] = block.Header.PrevBlockHash.String()
	}

	// Try to get next block
	if nextBlock, err := s.blockchain.GetBlockByHeight(height + 1); err == nil {
		headerInfo["nextblockhash"] = nextBlock.Hash().String()
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  headerInfo,
		ID:      req.ID,
	}
}

// buildBlockInfo constructs detailed block information
func (s *Server) buildBlockInfo(block *types.Block) (*BlockInfo, error) {
	hash := block.Hash()

	// Get block height from blockchain index
	height := s.getBlockHeight(hash)

	bestHeight, err := s.blockchain.GetBlockHeight()
	if err != nil {
		return nil, err
	}

	// If height is 0 (not found), use approximation
	if height == 0 {
		height = bestHeight
	}

	confirmations := int64(0)
	if height <= bestHeight {
		confirmations = int64(bestHeight - height + 1)
	}

	// Get transaction hashes
	txHashes := make([]string, len(block.Transactions))
	for i, tx := range block.Transactions {
		txHashes[i] = tx.Hash().String()
	}

	// Calculate actual block size
	blockSize := block.SerializeSize()

	// Calculate median time (median of last 11 blocks' timestamps)
	medianTime := s.calculateMedianTime(height)

	// Calculate money supply
	moneySupply := s.calculateMoneySupply(height)

	info := &BlockInfo{
		Hash:          hash.String(),
		Confirmations: confirmations,
		Size:          blockSize,
		Height:        int64(height),
		Version:       int(block.Header.Version),
		MerkleRoot:    block.Header.MerkleRoot.String(),
		Tx:            txHashes,
		Time:          int64(block.Header.Timestamp),
		MedianTime:    medianTime,
		Nonce:         block.Header.Nonce,
		Bits:          fmt.Sprintf("%08x", block.Header.Bits),
		Difficulty:    s.calculateDifficulty(block.Header.Bits),
		MoneySupply:   moneySupply,
	}

	// Add previous block hash if not genesis
	if !block.Header.PrevBlockHash.IsZero() {
		info.PreviousBlockHash = block.Header.PrevBlockHash.String()
	}

	// Try to get next block hash
	if nextBlock, err := s.blockchain.GetBlockByHeight(height + 1); err == nil {
		info.NextBlockHash = nextBlock.Hash().String()
	}

	// Add PoS-specific fields from consensus layer
	if s.consensus != nil && len(block.Transactions) > 0 && block.Transactions[0].IsCoinStake() {
		// Note: StakeModifier would be retrieved from consensus engine if needed
		// This is available through ValidateBlock results but not needed for basic block info
	}

	return info, nil
}

// getBlockHeight retrieves the height of a block by its hash
func (s *Server) getBlockHeight(hash types.Hash) uint32 {
	// Use the blockchain's hash->height index for O(1) lookup
	height, err := s.blockchain.GetBlockHeightByHash(hash)
	if err != nil {
		return 0
	}
	return height
}

// calculateDifficulty converts bits to difficulty
// Uses TWINS PoS max target (0x1e0fffff) instead of Bitcoin PoW target
func (s *Server) calculateDifficulty(bits uint32) float64 {
	return calculateDifficultyFromBits(bits)
}

// calculateMedianTime calculates the median time of the last 11 blocks
func (s *Server) calculateMedianTime(height uint32) int64 {
	// Get timestamps of last 11 blocks (or less if near genesis)
	numBlocks := uint32(11)
	if height < numBlocks {
		numBlocks = height + 1
	}

	timestamps := make([]int64, 0, numBlocks)
	for i := uint32(0); i < numBlocks; i++ {
		if height < i {
			break
		}
		blockHeight := height - i
		block, err := s.blockchain.GetBlockByHeight(blockHeight)
		if err != nil {
			continue
		}
		timestamps = append(timestamps, int64(block.Header.Timestamp))
	}

	if len(timestamps) == 0 {
		return 0
	}

	// Sort timestamps using standard library (more efficient and safe)
	sort.Slice(timestamps, func(i, j int) bool {
		return timestamps[i] < timestamps[j]
	})

	// Return median
	return timestamps[len(timestamps)/2]
}

// calculateMoneySupply retrieves total TWINS in circulation at given height
// Uses incrementally tracked money supply from storage
func (s *Server) calculateMoneySupply(height uint32) float64 {
	if s.blockchain == nil {
		return 0
	}

	// Get money supply from storage (calculated during block processing)
	supply, err := s.blockchain.GetMoneySupply(height)
	if err != nil {
		// Fall back to 0 if not available (e.g., during initial sync)
		return 0
	}

	return float64(supply) / 1e8 // Convert satoshis to TWINS
}

// handleGetTxOutSetInfo returns statistics about the UTXO set
func (s *Server) handleGetTxOutSetInfo(req *Request) *Response {
	// Get current blockchain state
	bestBlock, err := s.blockchain.GetBestBlock()
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInternalError(err.Error()),
			ID:      req.ID,
		}
	}

	height, err := s.blockchain.GetBlockHeight()
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInternalError(err.Error()),
			ID:      req.ID,
		}
	}

	// Get storage statistics
	// Note: This requires the storage interface to provide these stats
	// For now, we'll return basic information
	result := map[string]interface{}{
		"height":    height,
		"bestblock": bestBlock.Hash().String(),
	}

	// Try to get storage stats if available
	if storageGetter, ok := s.blockchain.(interface {
		GetStorage() interface{}
	}); ok {
		if storage := storageGetter.GetStorage(); storage != nil {
			if statsGetter, ok := storage.(interface {
				GetStats() (interface{}, error)
			}); ok {
				if stats, err := statsGetter.GetStats(); err == nil {
					// Extract relevant fields from stats
					if statsMap, ok := stats.(map[string]interface{}); ok {
						if txCount, ok := statsMap["transactions"].(int64); ok {
							result["transactions"] = txCount
						}
						if utxoCount, ok := statsMap["utxos"].(int64); ok {
							result["txouts"] = utxoCount
						}
						if size, ok := statsMap["size"].(int64); ok {
							result["bytes_serialized"] = size
						}
					}
				}
			}
		}
	}

	// Add placeholder values for fields we don't track yet
	if _, ok := result["transactions"]; !ok {
		result["transactions"] = 0
	}
	if _, ok := result["txouts"]; !ok {
		result["txouts"] = 0
	}
	if _, ok := result["bytes_serialized"]; !ok {
		result["bytes_serialized"] = 0
	}
	result["hash_serialized"] = ""
	result["total_amount"] = 0.0

	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// analyzeScriptPubKey analyzes a script and returns ScriptPubKeyInfo
func (s *Server) analyzeScriptPubKey(script []byte) map[string]interface{} {
	result := map[string]interface{}{
		"hex": hex.EncodeToString(script),
	}

	// Analyze script type and extract address
	scriptType, scriptHash := binary.AnalyzeScript(script)

	// Set script type
	switch scriptType {
	case binary.ScriptTypeP2PKH:
		result["type"] = "pubkeyhash"
		result["reqSigs"] = 1
	case binary.ScriptTypeP2SH:
		result["type"] = "scripthash"
		result["reqSigs"] = 1
	case binary.ScriptTypeP2PK:
		result["type"] = "pubkey"
		result["reqSigs"] = 1
	default:
		result["type"] = "nonstandard"
		return result
	}

	// Generate address from script hash
	if scriptHash != [20]byte{} {
		// Determine network ID
		netID := byte(0x50) // Mainnet
		if s.chainParams != nil && s.chainParams.Name == "testnet" {
			netID = 0x8B // Testnet
		}

		// Create address
		addr, err := crypto.NewAddressFromHash(scriptHash[:], netID)
		if err == nil {
			result["addresses"] = []string{addr.String()}
		}
	}

	// Generate ASM representation (simplified)
	result["asm"] = generateScriptASM(script)

	return result
}

// generateScriptASM generates a human-readable ASM representation of a script
func generateScriptASM(script []byte) string {
	if len(script) == 0 {
		return ""
	}

	// P2PKH
	if len(script) == 25 && script[0] == 0x76 {
		return fmt.Sprintf("OP_DUP OP_HASH160 %s OP_EQUALVERIFY OP_CHECKSIG", hex.EncodeToString(script[3:23]))
	}

	// P2SH
	if len(script) == 23 && script[0] == 0xa9 {
		return fmt.Sprintf("OP_HASH160 %s OP_EQUAL", hex.EncodeToString(script[2:22]))
	}

	// P2PK
	if (len(script) == 35 || len(script) == 67) && script[len(script)-1] == 0xac {
		pubKeyLen := len(script) - 2
		return fmt.Sprintf("%s OP_CHECKSIG", hex.EncodeToString(script[1:1+pubKeyLen]))
	}

	// Fallback: return hex
	return hex.EncodeToString(script)
}

// handleVerifyChain verifies blockchain database integrity
func (s *Server) handleVerifyChain(req *Request) *Response {
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid parameters"),
			ID:      req.ID,
		}
	}

	// Default parameters
	checkLevel := uint32(3)
	numBlocks := uint32(288)

	// Parse checklevel parameter
	if len(params) > 0 {
		if level, ok := params[0].(float64); ok {
			if level < 0 || level > 4 {
				return &Response{
					JSONRPC: "2.0",
					Error:   NewInvalidParamsError("checklevel must be between 0 and 4"),
					ID:      req.ID,
				}
			}
			checkLevel = uint32(level)
		}
	}

	// Parse numblocks parameter
	if len(params) > 1 {
		if blocks, ok := params[1].(float64); ok {
			if blocks < 0 {
				return &Response{
					JSONRPC: "2.0",
					Error:   NewInvalidParamsError("numblocks must be non-negative"),
					ID:      req.ID,
				}
			}
			numBlocks = uint32(blocks)
		}
	}

	// Get current height
	height, err := s.blockchain.GetBlockHeight()
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInternalError(err.Error()),
			ID:      req.ID,
		}
	}

	// Determine start height for verification
	startHeight := uint32(0)
	if height > numBlocks {
		startHeight = height - numBlocks
	}

	// Verify chain segment based on check level
	// Level 0: Check that blocks exist and are linked (parent hash chain)
	// Level 1-4: Full validation via ValidateChainSegment
	if checkLevel == 0 {
		// Level 0: Lightweight check - verify blocks exist and are linked
		for h := startHeight; h <= height; h++ {
			block, err := s.blockchain.GetBlockByHeight(h)
			if err != nil {
				s.logger.WithError(err).WithField("height", h).Error("Block not found during chain verification")
				return &Response{
					JSONRPC: "2.0",
					Result:  false,
					ID:      req.ID,
				}
			}

			// Check parent link (except for genesis)
			if h > 0 {
				parentBlock, err := s.blockchain.GetBlockByHeight(h - 1)
				if err != nil {
					s.logger.WithError(err).WithField("height", h-1).Error("Parent block not found")
					return &Response{
						JSONRPC: "2.0",
						Result:  false,
						ID:      req.ID,
					}
				}
				if block.Header.PrevBlockHash != parentBlock.Hash() {
					s.logger.WithFields(map[string]interface{}{
						"height":      h,
						"prev_block":  block.Header.PrevBlockHash.String(),
						"parent_hash": parentBlock.Hash().String(),
					}).Error("Block parent hash mismatch")
					return &Response{
						JSONRPC: "2.0",
						Result:  false,
						ID:      req.ID,
					}
				}
			}
		}
	} else {
		// Level 1-4: Full validation
		if verifier, ok := s.blockchain.(interface {
			ValidateChainSegment(fromHeight, toHeight uint32) error
		}); ok {
			if err := verifier.ValidateChainSegment(startHeight, height); err != nil {
				s.logger.WithError(err).WithFields(map[string]interface{}{
					"check_level": checkLevel,
					"start":       startHeight,
					"end":         height,
				}).Error("Chain verification failed")
				return &Response{
					JSONRPC: "2.0",
					Result:  false,
					ID:      req.ID,
				}
			}
		}
	}

	s.logger.WithFields(map[string]interface{}{
		"check_level": checkLevel,
		"num_blocks":  numBlocks,
		"start":       startHeight,
		"end":         height,
	}).Debug("Chain verification passed")

	return &Response{
		JSONRPC: "2.0",
		Result:  true,
		ID:      req.ID,
	}
}

// handleInvalidateBlock marks a block as invalid
func (s *Server) handleInvalidateBlock(req *Request) *Response {
	// WARNING: This is a dangerous operation that can disrupt the chain
	// It is intended for manually fixing chain state issues on any network

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
			Error:   NewInvalidParamsError("missing block hash"),
			ID:      req.ID,
		}
	}

	hashStr, ok := params[0].(string)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("block hash must be a string"),
			ID:      req.ID,
		}
	}

	hash, err := types.NewHashFromString(hashStr)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid block hash"),
			ID:      req.ID,
		}
	}

	// Check if block exists
	_, err = s.blockchain.GetBlock(hash)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(CodeBlockNotFound, "Block not found", hashStr),
			ID:      req.ID,
		}
	}

	// Log the invalidation
	s.logger.WithField("hash", hash.String()).Warn("Marking block as invalid and triggering reorganization")

	// Call blockchain layer to invalidate the block
	if err := s.blockchain.InvalidateBlock(hash); err != nil {
		s.logger.WithError(err).WithField("hash", hash.String()).Error("Failed to invalidate block")
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, fmt.Sprintf("Failed to invalidate block: %v", err), nil),
			ID:      req.ID,
		}
	}

	s.logger.WithField("hash", hash.String()).Info("Block successfully invalidated")

	// Return null result on success (Bitcoin Core compatible)
	return &Response{
		JSONRPC: "2.0",
		Result:  nil,
		ID:      req.ID,
	}
}

// handleReconsiderBlock removes invalid status from a block
func (s *Server) handleReconsiderBlock(req *Request) *Response {
	// WARNING: This is a dangerous operation that can disrupt the chain
	// It is intended for manually fixing chain state issues on any network

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
			Error:   NewInvalidParamsError("missing block hash"),
			ID:      req.ID,
		}
	}

	hashStr, ok := params[0].(string)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("block hash must be a string"),
			ID:      req.ID,
		}
	}

	hash, err := types.NewHashFromString(hashStr)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid block hash"),
			ID:      req.ID,
		}
	}

	// Check if block exists
	_, err = s.blockchain.GetBlock(hash)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(CodeBlockNotFound, "Block not found", hashStr),
			ID:      req.ID,
		}
	}

	// Log the reconsideration
	s.logger.WithField("hash", hash.String()).Debug("Reconsidering block and triggering reorganization")

	// Call blockchain layer to reconsider the block
	if err := s.blockchain.ReconsiderBlock(hash); err != nil {
		s.logger.WithError(err).WithField("hash", hash.String()).Error("Failed to reconsider block")
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, fmt.Sprintf("Failed to reconsider block: %v", err), nil),
			ID:      req.ID,
		}
	}

	s.logger.WithField("hash", hash.String()).Info("Block successfully reconsidered")

	// Return null result on success (Bitcoin Core compatible)
	return &Response{
		JSONRPC: "2.0",
		Result:  nil,
		ID:      req.ID,
	}
}

// handleAddCheckpoint adds a checkpoint to the blockchain
func (s *Server) handleAddCheckpoint(req *Request) *Response {
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
			Error:   NewInvalidParamsError("missing height and/or hash"),
			ID:      req.ID,
		}
	}

	heightFloat, ok := params[0].(float64)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("height must be a number"),
			ID:      req.ID,
		}
	}

	hashStr, ok := params[1].(string)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("hash must be a string"),
			ID:      req.ID,
		}
	}

	height := uint32(heightFloat)
	hash, err := types.NewHashFromString(hashStr)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid block hash"),
			ID:      req.ID,
		}
	}

	// Log the checkpoint addition
	s.logger.WithFields(map[string]interface{}{
		"height": height,
		"hash":   hash.String(),
	}).Debug("Adding checkpoint to blockchain")

	// Call blockchain layer to add the checkpoint
	if err := s.blockchain.AddCheckpoint(height, hash); err != nil {
		s.logger.WithError(err).WithFields(map[string]interface{}{
			"height": height,
			"hash":   hash.String(),
		}).Error("Failed to add checkpoint")
		return &Response{
			JSONRPC: "2.0",
			Error:   NewError(-1, fmt.Sprintf("Failed to add checkpoint: %v", err), nil),
			ID:      req.ID,
		}
	}

	s.logger.WithFields(map[string]interface{}{
		"height": height,
		"hash":   hash.String(),
	}).Info("Checkpoint successfully added")

	// Return null result on success (Bitcoin Core compatible)
	return &Response{
		JSONRPC: "2.0",
		Result:  nil,
		ID:      req.ID,
	}
}

// handleGetFeeInfo returns fee information for the network
func (s *Server) handleGetFeeInfo(req *Request) *Response {
	var params []interface{}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid parameters"),
			ID:      req.ID,
		}
	}

	// Default to analyzing last 10 blocks
	blocks := uint32(10)
	if len(params) > 0 {
		if b, ok := params[0].(float64); ok {
			blocks = uint32(b)
		}
	}

	height, err := s.blockchain.GetBlockHeight()
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInternalError(err.Error()),
			ID:      req.ID,
		}
	}

	// Calculate fee statistics from recent blocks
	startHeight := uint32(0)
	if height > blocks {
		startHeight = height - blocks
	}

	var txCount int64
	var txBytes int64
	var totalFees int64
	var fees []int64

	for h := startHeight; h <= height; h++ {
		block, err := s.blockchain.GetBlockByHeight(h)
		if err != nil {
			continue
		}

		for _, tx := range block.Transactions {
			if tx.IsCoinbase() || tx.IsCoinStake() {
				continue
			}

			txCount++
			txBytes += int64(tx.SerializeSize())

			// Calculate actual transaction fee
			var inputSum int64
			var missingUTXO bool
			for _, input := range tx.Inputs {
				outpoint := types.Outpoint{
					Hash:  input.PreviousOutput.Hash,
					Index: input.PreviousOutput.Index,
				}
				utxo, err := s.blockchain.GetUTXO(outpoint)
				if err != nil {
					// UTXO not found - might be from same block or mempool
					// Skip this entire transaction's fee calculation
					s.logger.WithFields(map[string]interface{}{
						"tx":    tx.Hash().String(),
						"input": outpoint.Hash.String(),
					}).Debug("UTXO not found for fee calculation")
					missingUTXO = true
					break // Break from input loop
				}
				inputSum += utxo.Value
			}

			// Skip transaction if any UTXO was missing
			if missingUTXO {
				continue
			}
			var outputSum int64
			for _, output := range tx.Outputs {
				outputSum += output.Value
			}

			fee := inputSum - outputSum
			if fee > 0 {
				totalFees += fee
				fees = append(fees, fee)
			} else if fee < 0 {
				// Invalid transaction (outputs > inputs), skip it
				s.logger.WithField("tx", tx.Hash().String()).Warn("Transaction has negative fee, skipping")
			}
		}
	}

	// Calculate statistics
	minFee := int64(0)
	maxFee := int64(0)
	avgFee := int64(0)

	if len(fees) > 0 {
		sort.Slice(fees, func(i, j int) bool { return fees[i] < fees[j] })
		minFee = fees[0]
		maxFee = fees[len(fees)-1]
		avgFee = totalFees / int64(len(fees))
	}

	result := map[string]interface{}{
		"txcount":     txCount,
		"txbytes":     txBytes,
		"ttlfee":      float64(totalFees) / 1e8,
		"minpriority": 0.0,
		"minfee":      float64(minFee) / 1e5, // TWINS/kB
		"maxfee":      float64(maxFee) / 1e5,
		"avgfee":      float64(avgFee) / 1e5,
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// handleGetRewardRates returns block reward distribution rates
func (s *Server) handleGetRewardRates(req *Request) *Response {
	// TWINS 4-tier masternode system reward distribution
	// Staker gets base reward, masternodes get additional percentage
	// Bronze (1M): 10%, Silver (5M): 20%, Gold (20M): 30%, Platinum (100M): 40%

	result := map[string]interface{}{
		"staker_reward":     60.0, // Base staker reward percentage
		"masternode_reward": 40.0, // Masternode reward percentage (varies by tier)
		"tiers": map[string]interface{}{
			"bronze": map[string]interface{}{
				"collateral":     1000000,
				"reward_percent": 10.0,
				"min_collateral": "1000000 TWINS",
			},
			"silver": map[string]interface{}{
				"collateral":     5000000,
				"reward_percent": 20.0,
				"min_collateral": "5000000 TWINS",
			},
			"gold": map[string]interface{}{
				"collateral":     20000000,
				"reward_percent": 30.0,
				"min_collateral": "20000000 TWINS",
			},
			"platinum": map[string]interface{}{
				"collateral":     100000000,
				"reward_percent": 40.0,
				"min_collateral": "100000000 TWINS",
			},
		},
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// getStakingStatusString returns staking status as a string for legacy compatibility
// Returns "Staking Active" or "Staking Not Active" matching legacy C++ getinfo format
func (s *Server) getStakingStatusString() string {
	if s.isStaking() {
		return "Staking Active"
	}
	return "Staking Not Active"
}

// Phase 5: Removed handleReindexTransactions and handleReindexStatus
// These functions are no longer needed with the new storage architecture.
// All transaction indexes are created automatically during block processing
// with the new unified processor, eliminating the need for separate reindexing.
