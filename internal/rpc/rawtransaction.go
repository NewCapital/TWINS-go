package rpc

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/twins-dev/twins-core/pkg/crypto"
	"github.com/twins-dev/twins-core/pkg/script"
	"github.com/twins-dev/twins-core/pkg/types"
)

// registerRawTransactionHandlers registers raw transaction RPC handlers
func (s *Server) registerRawTransactionHandlers() {
	s.RegisterHandler("createrawtransaction", s.handleCreateRawTransaction)
	s.RegisterHandler("signrawtransaction", s.handleSignRawTransaction)
	s.RegisterHandler("decodescript", s.handleDecodeScript)
}

// handleCreateRawTransaction creates an unsigned raw transaction
func (s *Server) handleCreateRawTransaction(req *Request) *Response {
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
			Error:   NewInvalidParamsError("missing inputs or outputs"),
			ID:      req.ID,
		}
	}

	// Parse inputs
	inputsParam, ok := params[0].([]interface{})
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("inputs must be an array"),
			ID:      req.ID,
		}
	}

	// Parse outputs
	outputsParam, ok := params[1].(map[string]interface{})
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("outputs must be an object"),
			ID:      req.ID,
		}
	}

	// Parse locktime (optional)
	var locktime uint32 = 0
	if len(params) > 2 {
		locktimeFloat, ok := params[2].(float64)
		if ok {
			locktime = uint32(locktimeFloat)
		}
	}

	// Create transaction
	tx := &types.Transaction{
		Version:  1,
		Inputs:   make([]*types.TxInput, 0),
		Outputs:  make([]*types.TxOutput, 0),
		LockTime: locktime,
	}

	// Process inputs
	for _, inputRaw := range inputsParam {
		inputMap, ok := inputRaw.(map[string]interface{})
		if !ok {
			return &Response{
				JSONRPC: "2.0",
				Error:   NewInvalidParamsError("invalid input format"),
				ID:      req.ID,
			}
		}

		txidStr, ok := inputMap["txid"].(string)
		if !ok {
			return &Response{
				JSONRPC: "2.0",
				Error:   NewInvalidParamsError("input missing txid"),
				ID:      req.ID,
			}
		}

		voutFloat, ok := inputMap["vout"].(float64)
		if !ok {
			return &Response{
				JSONRPC: "2.0",
				Error:   NewInvalidParamsError("input missing vout"),
				ID:      req.ID,
			}
		}

		// Parse transaction hash
		txHash, err := types.NewHashFromString(txidStr)
		if err != nil {
			return &Response{
				JSONRPC: "2.0",
				Error:   NewInvalidParamsError("invalid transaction hash"),
				ID:      req.ID,
			}
		}

		// Add input with empty scriptSig (will be signed later)
		input := &types.TxInput{
			PreviousOutput: types.Outpoint{
				Hash:  txHash,
				Index: uint32(voutFloat),
			},
			ScriptSig: []byte{},
			Sequence:  0xffffffff,
		}

		// Check for optional sequence
		if seqRaw, ok := inputMap["sequence"]; ok {
			if seqFloat, ok := seqRaw.(float64); ok {
				input.Sequence = uint32(seqFloat)
			}
		}

		tx.Inputs = append(tx.Inputs, input)
	}

	// Process outputs
	for addressStr, amountRaw := range outputsParam {
		amountFloat, ok := amountRaw.(float64)
		if !ok {
			return &Response{
				JSONRPC: "2.0",
				Error:   NewInvalidParamsError("invalid amount for address " + addressStr),
				ID:      req.ID,
			}
		}

		// Convert amount from TWINS to satoshis
		amount := int64(amountFloat * 1e8)
		if amount < 0 {
			return &Response{
				JSONRPC: "2.0",
				Error:   NewInvalidParamsError("negative amount not allowed for address " + addressStr),
				ID:      req.ID,
			}
		}

		// Decode address
		addr, err := crypto.DecodeAddress(addressStr)
		if err != nil {
			return &Response{
				JSONRPC: "2.0",
				Error:   NewInvalidParamsError("invalid address: " + addressStr),
				ID:      req.ID,
			}
		}

		// Create output script
		scriptPubKey := addr.CreateScriptPubKey()

		// Validate zero-value outputs (only allowed for OP_RETURN)
		if amount == 0 {
			// Zero-value outputs are only valid for OP_RETURN (data storage) scripts
			// OP_RETURN = 0x6a
			if len(scriptPubKey) == 0 || scriptPubKey[0] != 0x6a {
				return &Response{
					JSONRPC: "2.0",
					Error:   NewInvalidParamsError("zero-value output only allowed for OP_RETURN scripts"),
					ID:      req.ID,
				}
			}
		}

		output := &types.TxOutput{
			Value:        amount,
			ScriptPubKey: scriptPubKey,
		}

		tx.Outputs = append(tx.Outputs, output)
	}

	// Calculate and warn about potential fees if we can estimate them
	// Note: We can't calculate exact fees without input amounts, but we can check output total
	var totalOutput int64
	maxReasonableOutput := int64(1000000 * 1e8) // 1 million TWINS per output

	for _, output := range tx.Outputs {
		totalOutput += output.Value
		// Warn about suspiciously large individual outputs
		if output.Value > maxReasonableOutput {
			s.logger.Warnf("Output of %d satoshis (%.8f TWINS) seems very large, verify amounts",
				output.Value, float64(output.Value)/1e8)
		}
	}

	// Also warn if total outputs seem excessive (more than 21 million TWINS worth of satoshis)
	maxSupply := int64(21000000 * 1e8)
	if totalOutput > maxSupply {
		s.logger.Warnf("Transaction outputs total %d satoshis, exceeds max supply", totalOutput)
	}

	// Serialize transaction
	txBytes, err := tx.Serialize()
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInternalError("failed to serialize transaction: " + err.Error()),
			ID:      req.ID,
		}
	}

	// Validate transaction size (100KB limit)
	const maxTransactionSize = 100000 // 100KB
	if len(txBytes) > maxTransactionSize {
		return &Response{
			JSONRPC: "2.0",
			Error: NewInvalidParamsError(fmt.Sprintf("transaction size %d bytes exceeds maximum of %d bytes",
				len(txBytes), maxTransactionSize)),
			ID: req.ID,
		}
	}

	// Return hex-encoded transaction
	return &Response{
		JSONRPC: "2.0",
		Result:  hex.EncodeToString(txBytes),
		ID:      req.ID,
	}
}

// handleSignRawTransaction signs inputs for a raw transaction
func (s *Server) handleSignRawTransaction(req *Request) *Response {
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

	// Parse transaction hex
	txHex, ok := params[0].(string)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("transaction hex must be a string"),
			ID:      req.ID,
		}
	}

	// Decode transaction
	txBytes, err := hex.DecodeString(txHex)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid transaction hex"),
			ID:      req.ID,
		}
	}

	tx, err := types.DeserializeTransaction(txBytes)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("failed to deserialize transaction"),
			ID:      req.ID,
		}
	}

	// Parse previous transaction outputs (optional)
	prevTxs := make(map[types.Outpoint]*types.TxOutput)
	if len(params) > 1 && params[1] != nil {
		prevTxsParam, ok := params[1].([]interface{})
		if ok {
			for _, prevTxRaw := range prevTxsParam {
				prevTxMap, ok := prevTxRaw.(map[string]interface{})
				if !ok {
					continue
				}

				txidStr, _ := prevTxMap["txid"].(string)
				voutFloat, _ := prevTxMap["vout"].(float64)
				scriptPubKeyHex, _ := prevTxMap["scriptPubKey"].(string)
				amountFloat, _ := prevTxMap["amount"].(float64)

				txHash, err := types.NewHashFromString(txidStr)
				if err != nil {
					continue
				}

				scriptPubKey, err := hex.DecodeString(scriptPubKeyHex)
				if err != nil {
					continue
				}

				outpoint := types.Outpoint{
					Hash:  txHash,
					Index: uint32(voutFloat),
				}

				prevTxs[outpoint] = &types.TxOutput{
					Value:        int64(amountFloat * 1e8),
					ScriptPubKey: scriptPubKey,
				}
			}
		}
	}

	// Parse private keys (optional)
	// SECURITY: Private keys are sensitive - ensure cleanup after use
	privateKeys := make(map[string]*crypto.PrivateKey)
	defer func() {
		// Clear private keys from memory after use
		// While we cannot zero the internal btcec.PrivateKey (it's opaque),
		// we can at least clear our references and any byte copies
		for addr, privKey := range privateKeys {
			// Get the bytes and attempt to zero them (this is a copy but better than nothing)
			keyBytes := privKey.Bytes()
			for i := range keyBytes {
				keyBytes[i] = 0
			}
			// Clear the map entry
			delete(privateKeys, addr)
		}
	}()

	if len(params) > 2 && params[2] != nil {
		keysParam, ok := params[2].([]interface{})
		if ok {
			for _, keyRaw := range keysParam {
				keyWIF, ok := keyRaw.(string)
				if !ok {
					continue
				}

				privKey, _, err := crypto.DecodePrivateKeyWIF(keyWIF)
				if err != nil {
					continue
				}

				// Get public key and address for mapping
				pubKey := privKey.PublicKey()
				addr := crypto.NewAddressFromPubKey(pubKey, crypto.MainNetPubKeyHashAddrID)
				privateKeys[addr.String()] = privKey
			}
		}
	}

	// Parse signature hash type (optional)
	sigHashType := uint32(types.SigHashAll)
	if len(params) > 3 {
		sigHashTypeStr, ok := params[3].(string)
		if ok {
			switch sigHashTypeStr {
			case "ALL":
				sigHashType = types.SigHashAll
			case "NONE":
				sigHashType = types.SigHashNone
			case "SINGLE":
				sigHashType = types.SigHashSingle
			case "ALL|ANYONECANPAY":
				sigHashType = types.SigHashAll | types.SigHashAnyoneCanPay
			case "NONE|ANYONECANPAY":
				sigHashType = types.SigHashNone | types.SigHashAnyoneCanPay
			case "SINGLE|ANYONECANPAY":
				sigHashType = types.SigHashSingle | types.SigHashAnyoneCanPay
			}
		}
	}

	// Sign each input
	allSigned := true
	for i, input := range tx.Inputs {
		// Get previous output
		prevOutput, exists := prevTxs[input.PreviousOutput]
		if !exists {
			// Try to get from blockchain/wallet
			if s.blockchain != nil {
				s.logger.Debugf("Input %d: UTXO %s:%d not in prevtxs, checking blockchain",
					i, input.PreviousOutput.Hash.String(), input.PreviousOutput.Index)

				utxo, err := s.blockchain.GetUTXO(input.PreviousOutput)
				if err != nil {
					s.logger.Warnf("Input %d: UTXO lookup failed: %v",
						i, err)
					allSigned = false
					continue
				}
				if utxo != nil {
					// Fix type mismatch: GetUTXO returns *types.UTXO, but prevOutput needs *types.TxOutput
					prevOutput = &types.TxOutput{
						Value:        utxo.Value,
						ScriptPubKey: utxo.ScriptPubKey,
					}
				} else {
					s.logger.Warnf("Input %d: UTXO not found in blockchain for %s:%d",
						i, input.PreviousOutput.Hash.String(), input.PreviousOutput.Index)
					allSigned = false
					continue
				}
			}
		}

		if prevOutput == nil {
			s.logger.Warnf("Input %d: Cannot sign - missing UTXO data for %s:%d",
				i, input.PreviousOutput.Hash.String(), input.PreviousOutput.Index)
			allSigned = false
			continue
		}

		// Try to sign with provided private keys or wallet
		signed := false

		// First try provided private keys
		for _, privKey := range privateKeys {
			if s.trySignInput(tx, i, prevOutput.ScriptPubKey, privKey, sigHashType) {
				signed = true
				break
			}
		}

		// If not signed and wallet is available, try wallet
		if !signed && s.wallet != nil {
			// Extract address from scriptPubKey and get private key from wallet
			address := s.wallet.ExtractAddress(prevOutput.ScriptPubKey)
			if address != "" {
				wifKey, err := s.wallet.DumpPrivKey(address)
				if err == nil {
					// Parse WIF and try to sign
					privKey, _, err := crypto.DecodePrivateKeyWIF(wifKey)
					if err == nil && s.trySignInput(tx, i, prevOutput.ScriptPubKey, privKey, sigHashType) {
						signed = true
						s.logger.Debugf("Input %d: Signed with wallet key for %s", i, address)
					}
				} else {
					s.logger.Debugf("Input %d: Could not get wallet key for %s: %v", i, address, err)
				}
			}
			if !signed {
				allSigned = false
			}
		} else if !signed {
			s.logger.Debugf("Input %d: No matching private key found", i)
			allSigned = false
		}
	}

	// Serialize signed transaction
	signedTxBytes, err := tx.Serialize()
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInternalError("failed to serialize signed transaction"),
			ID:      req.ID,
		}
	}

	result := map[string]interface{}{
		"hex":      hex.EncodeToString(signedTxBytes),
		"complete": allSigned,
	}

	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// trySignInput attempts to sign a transaction input with a private key
func (s *Server) trySignInput(tx *types.Transaction, inputIndex int, scriptPubKey []byte, privKey *crypto.PrivateKey, sigHashType uint32) bool {
	// Calculate signature hash
	sigHash := tx.SignatureHash(inputIndex, scriptPubKey, sigHashType)

	// Sign the hash
	signature, err := privKey.Sign(sigHash[:])
	if err != nil {
		return false
	}

	// Append hash type to signature
	sigWithType := append(signature.Bytes(), byte(sigHashType))

	// Get public key bytes
	pubKeyBytes := privKey.PublicKey().CompressedBytes()

	// Build scriptSig: <signature> <pubkey>
	scriptSig := make([]byte, 0)

	// Push signature
	scriptSig = append(scriptSig, byte(len(sigWithType)))
	scriptSig = append(scriptSig, sigWithType...)

	// Push public key
	scriptSig = append(scriptSig, byte(len(pubKeyBytes)))
	scriptSig = append(scriptSig, pubKeyBytes...)

	// Verify the signature would work using full consensus flags
	// Use StandardVerifyFlags which includes all necessary validation
	engine := script.NewEngine(scriptPubKey, tx, inputIndex, script.StandardVerifyFlags)
	if err := engine.Execute(scriptSig); err != nil {
		s.logger.Debugf("Signature verification failed for input %d: %v", inputIndex, err)
		return false
	}

	// Set the scriptSig
	tx.Inputs[inputIndex].ScriptSig = scriptSig
	return true
}

// Bitcoin standard maximum script size for standard transactions
const maxScriptSize = 10000

// handleDecodeScript decodes a hex-encoded script
func (s *Server) handleDecodeScript(req *Request) *Response {
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
			Error:   NewInvalidParamsError("missing script hex"),
			ID:      req.ID,
		}
	}

	scriptHex, ok := params[0].(string)
	if !ok {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("script hex must be a string"),
			ID:      req.ID,
		}
	}

	// Decode hex
	scriptBytes, err := hex.DecodeString(scriptHex)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError("invalid script hex"),
			ID:      req.ID,
		}
	}

	// Validate script size to prevent DoS attacks
	if len(scriptBytes) > maxScriptSize {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInvalidParamsError(fmt.Sprintf("script too large: %d bytes (max %d)", len(scriptBytes), maxScriptSize)),
			ID:      req.ID,
		}
	}

	// Disassemble script to get asm representation
	asm, err := script.Disassemble(scriptBytes)
	if err != nil {
		asm = "error: " + err.Error()
	}

	// Determine script type and extract addresses
	scriptType := script.GetScriptType(scriptBytes)
	var addresses []string
	var reqSigs int

	switch scriptType {
	case script.PubKeyHashTy:
		// P2PKH - extract address from script
		if addr, err := script.ExtractPubKeyHashAddress(scriptBytes); err == nil {
			addresses = append(addresses, addr.String())
			reqSigs = 1
		}
	case script.ScriptHashTy:
		// P2SH - extract address
		if addr, err := script.ExtractScriptHash(scriptBytes); err == nil {
			addresses = append(addresses, addr.String())
			// For P2SH, we don't know reqSigs without the redeem script
			reqSigs = 1
		}
	case script.MultiSigTy:
		// Multisig - extract required sigs and addresses
		if addrs, required, err := script.ExtractMultisig(scriptBytes); err == nil {
			for _, addr := range addrs {
				addresses = append(addresses, addr.String())
			}
			reqSigs = required
		}
	case script.PubKeyTy:
		// P2PK - extract address from public key
		if addr, err := script.ExtractPubKey(scriptBytes); err == nil {
			addresses = append(addresses, addr.String())
			reqSigs = 1
		}
	}

	// Build response
	result := map[string]interface{}{
		"asm":  asm,
		"type": scriptType.String(),
	}

	if reqSigs > 0 {
		result["reqSigs"] = reqSigs
	}

	if len(addresses) > 0 {
		result["addresses"] = addresses
	}

	// Generate P2SH address for this script
	scriptHash := crypto.Hash160(scriptBytes)
	p2shAddr, err := crypto.NewAddressFromHash(scriptHash, crypto.MainNetScriptHashAddrID)
	if err != nil {
		return &Response{
			JSONRPC: "2.0",
			Error:   NewInternalError(fmt.Sprintf("failed to create P2SH address: %v", err)),
			ID:      req.ID,
		}
	}
	result["p2sh"] = p2shAddr.String()

	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}