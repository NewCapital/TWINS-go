package script

import (
	"github.com/twins-dev/twins-core/pkg/types"
)

// SigOps limits - legacy: src/consensus/consensus.h
const (
	// MAX_PUBKEYS_PER_MULTISIG is the maximum public keys in OP_CHECKMULTISIG
	MAX_PUBKEYS_PER_MULTISIG = 20

	// MAX_TX_SIGOPS_COUNT is maximum sigops per transaction
	MAX_TX_SIGOPS_COUNT = 4000

	// Legacy block sigops limits (for reference - use GetMaxBlockSigOps for dynamic calculation)
	MAX_BLOCK_SIGOPS_LEGACY  = 20000 // 1MB blocks
	MAX_BLOCK_SIGOPS_CURRENT = 40000 // 2MB blocks after Zerocoin
)

// GetSigOpCount counts signature operations in a script
// Legacy reference: GetLegacySigOpCount() in script/script.cpp
//
// Parameters:
//   - script: The script to analyze (scriptSig or scriptPubKey)
//   - accurate: If true, uses actual OP_N values for multisig; if false, assumes max (20)
//
// Counted operations:
//   - OP_CHECKSIG = 1 sigop
//   - OP_CHECKSIGVERIFY = 1 sigop
//   - OP_CHECKMULTISIG = up to 20 sigops (depends on accurate flag)
//   - OP_CHECKMULTISIGVERIFY = up to 20 sigops (depends on accurate flag)
func GetSigOpCount(script []byte, accurate bool) int {
	sigOps := 0
	pc := 0
	lastOpcode := byte(OP_0)

	for pc < len(script) {
		opcode, _, newPC, err := getOp(script, pc)
		if err != nil {
			// If script is malformed, stop counting
			break
		}

		switch opcode {
		case OP_CHECKSIG, OP_CHECKSIGVERIFY:
			sigOps++

		case OP_CHECKMULTISIG, OP_CHECKMULTISIGVERIFY:
			if accurate && lastOpcode >= OP_1 && lastOpcode <= OP_16 {
				// Accurate count from previous OP_N (OP_1 = 0x51, represents 1)
				sigOps += int(lastOpcode - OP_1 + 1)
			} else {
				// Conservative: assume maximum
				sigOps += MAX_PUBKEYS_PER_MULTISIG
			}
		}

		lastOpcode = opcode
		pc = newPC
	}

	return sigOps
}

// GetTransactionSigOpCount counts all sigops in a transaction
// This includes sigops in both scriptSig (inputs) and scriptPubKey (outputs)
//
// Legacy reference: GetLegacySigOpCount(tx) in main.cpp
func GetTransactionSigOpCount(tx *types.Transaction) int {
	sigOps := 0

	// Count in all scriptSigs (inputs)
	for _, input := range tx.Inputs {
		sigOps += GetSigOpCount(input.ScriptSig, false)
	}

	// Count in all scriptPubKeys (outputs)
	for _, output := range tx.Outputs {
		sigOps += GetSigOpCount(output.ScriptPubKey, false)
	}

	return sigOps
}

// GetBlockSigOpCount counts all sigops in a block
// Sums sigops from all transactions in the block
func GetBlockSigOpCount(block *types.Block) int {
	sigOps := 0

	for _, tx := range block.Transactions {
		sigOps += GetTransactionSigOpCount(tx)
	}

	return sigOps
}

// IsPayToScriptHash checks if script is P2SH format
// P2SH format: OP_HASH160 <20 bytes> OP_EQUAL
//
// Legacy reference: IsPayToScriptHash() in script/script.cpp
func IsPayToScriptHash(script []byte) bool {
	return len(script) == 23 &&
		script[0] == OP_HASH160 &&
		script[1] == 0x14 && // push 20 bytes
		script[22] == OP_EQUAL
}

// ExtractRedeemScript extracts the redeemScript from P2SH scriptSig
// The redeemScript is the last data push in the scriptSig
//
// For P2SH transactions:
//   scriptSig: <sig1> <sig2> ... <redeemScript>
//   scriptPubKey: OP_HASH160 <hash(redeemScript)> OP_EQUAL
//
// We need the redeemScript to count its sigops
func ExtractRedeemScript(scriptSig []byte) []byte {
	if len(scriptSig) == 0 {
		return nil
	}

	// Parse scriptSig and get last data push
	pc := 0
	var lastData []byte

	for pc < len(scriptSig) {
		_, data, newPC, err := getOp(scriptSig, pc)
		if err != nil {
			break
		}

		// Any data push might be the redeemScript
		// We keep track of the last one
		if data != nil && len(data) > 0 {
			lastData = data
		}

		pc = newPC
	}

	return lastData
}

// GetMaxBlockSigOps returns the sigops limit based on Zerocoin activation
// Legacy: MAX_BLOCK_SIGOPS changes from 20k to 40k after Zerocoin (main.cpp:4056-4063)
func GetMaxBlockSigOps(zerocoinActive bool) int {
	if zerocoinActive {
		// After Zerocoin: 2MB blocks = 40,000 sigops
		return MAX_BLOCK_SIGOPS_CURRENT
	}
	// Before Zerocoin: 1MB blocks = 20,000 sigops
	return MAX_BLOCK_SIGOPS_LEGACY
}

// GetP2SHSigOpCount counts sigops in P2SH redemption scripts
// Must be called after basic validation to ensure UTXO availability
//
// For P2SH outputs, we need to:
// 1. Identify P2SH outputs being spent
// 2. Extract the redeemScript from the spending scriptSig
// 3. Count sigops in the redeemScript
//
// Legacy reference: GetP2SHSigOpCount() in main.cpp
//
// Note: This is a simplified version. Full implementation requires:
// - Access to UTXO set to get previous outputs
// - Only counts after BIP16 activation height
// - Only for non-coinbase transactions
func GetP2SHSigOpCount(tx *types.Transaction, getPrevOutput func(types.Outpoint) (*types.TxOutput, error)) int {
	if tx.IsCoinbase() {
		return 0
	}

	sigOps := 0

	for _, input := range tx.Inputs {
		// Get previous output being spent
		prevOut, err := getPrevOutput(input.PreviousOutput)
		if err != nil {
			// If we can't get the previous output, skip
			// This shouldn't happen in normal operation
			continue
		}

		// Check if previous output is P2SH
		if IsPayToScriptHash(prevOut.ScriptPubKey) {
			// Extract redeemScript from scriptSig
			redeemScript := ExtractRedeemScript(input.ScriptSig)
			if redeemScript != nil {
				// Count sigops in the redeemScript with accurate counting
				// P2SH scripts can use accurate counting since we have the full script
				sigOps += GetSigOpCount(redeemScript, true)
			}
		}
	}

	return sigOps
}
