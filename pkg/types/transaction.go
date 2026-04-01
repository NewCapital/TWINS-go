package types

import (
	"bytes"
	"encoding/binary"
)

// SIGHASH types
const (
	SigHashAll          = 1
	SigHashNone         = 2
	SigHashSingle       = 3
	SigHashAnyoneCanPay = 0x80

	// OpCodeSeparator opcode
	OpCodeSeparator = 0xab
)

// TxInput represents a transaction input
type TxInput struct {
	PreviousOutput Outpoint // Reference to the output being spent
	ScriptSig      []byte   // Signature script that satisfies the conditions
	Sequence       uint32   // Transaction version as defined by the sender
}

// TxOutput represents a transaction output
type TxOutput struct {
	Value        int64  // Value in satoshis
	ScriptPubKey []byte // Public key script defining spending conditions
}

// Transaction represents a complete transaction
type Transaction struct {
	Version  uint32      // Transaction data format version
	Inputs   []*TxInput  // List of transaction inputs
	Outputs  []*TxOutput // List of transaction outputs
	LockTime uint32      // Block height or timestamp when transaction is final

	// Genesis override: set this for genesis transaction to use hardcoded hash
	canonicalHash *Hash // If set, Hash() returns this instead of calculating
}

// Hash calculates and returns the double SHA-256 hash of the transaction
// Uses Bitcoin canonical serialization with varints
func (tx *Transaction) Hash() Hash {
	// Return canonical hash if set (for genesis transaction)
	if tx.canonicalHash != nil {
		return *tx.canonicalHash
	}

	var buf bytes.Buffer

	// Serialize transaction for hashing (matches legacy exactly)
	binary.Write(&buf, binary.LittleEndian, tx.Version)

	// Write input count (Bitcoin compact size / varint)
	WriteCompactSize(&buf, uint64(len(tx.Inputs)))

	// Write inputs
	for _, input := range tx.Inputs {
		buf.Write(input.PreviousOutput.Hash[:])
		binary.Write(&buf, binary.LittleEndian, input.PreviousOutput.Index)

		// Write script length (varint) and script
		WriteCompactSize(&buf, uint64(len(input.ScriptSig)))
		buf.Write(input.ScriptSig)

		binary.Write(&buf, binary.LittleEndian, input.Sequence)
	}

	// Write output count (varint)
	WriteCompactSize(&buf, uint64(len(tx.Outputs)))

	// Write outputs
	for _, output := range tx.Outputs {
		binary.Write(&buf, binary.LittleEndian, output.Value)

		// Write script length (varint) and script
		WriteCompactSize(&buf, uint64(len(output.ScriptPubKey)))
		buf.Write(output.ScriptPubKey)
	}

	binary.Write(&buf, binary.LittleEndian, tx.LockTime)

	return NewHash(buf.Bytes())
}

// SetCanonicalHash sets the canonical hash for this transaction (used for genesis)
func (tx *Transaction) SetCanonicalHash(hash Hash) {
	tx.canonicalHash = &hash
}

// IsCoinbase returns true if the transaction is a coinbase transaction
func (tx *Transaction) IsCoinbase() bool {
	return len(tx.Inputs) == 1 &&
		tx.Inputs[0].PreviousOutput.Hash.IsZero() &&
		tx.Inputs[0].PreviousOutput.Index == 0xffffffff
}

// IsCoinStake returns true if the transaction is a coinstake transaction (PoS)
// A coinstake transaction has:
// - At least 1 input with non-null prevout (distinguishes from coinbase)
// - At least 2 outputs
// - First output is empty (value = 0 AND scriptPubKey is empty)
//
// Legacy compliance: C++ CTxOut::IsEmpty() checks BOTH nValue==0 AND scriptPubKey.empty()
// See legacy/src/primitives/transaction.h:155-157, transaction.cpp:147
func (tx *Transaction) IsCoinStake() bool {
	return len(tx.Inputs) >= 1 &&
		!tx.Inputs[0].PreviousOutput.Hash.IsZero() &&
		len(tx.Outputs) >= 2 &&
		tx.Outputs[0].Value == 0 &&
		len(tx.Outputs[0].ScriptPubKey) == 0 // Legacy: scriptPubKey.empty()
}

// SerializeSize returns the approximate serialized size of the transaction
func (tx *Transaction) SerializeSize() int {
	size := 4                                      // Version
	size += CompactSizeLen(uint64(len(tx.Inputs))) // Input count (varint)

	// Add size for inputs
	for _, input := range tx.Inputs {
		size += 32                                           // PrevTxHash
		size += 4                                            // Index
		size += CompactSizeLen(uint64(len(input.ScriptSig))) // Script length (varint)
		size += len(input.ScriptSig)
		size += 4 // Sequence
	}

	size += CompactSizeLen(uint64(len(tx.Outputs))) // Output count (varint)

	// Add size for outputs
	for _, output := range tx.Outputs {
		size += 8                                                // Value
		size += CompactSizeLen(uint64(len(output.ScriptPubKey))) // Script length (varint)
		size += len(output.ScriptPubKey)
	}

	size += 4 // LockTime

	return size
}

// SerializedSize is an alias for SerializeSize for compatibility
func (tx *Transaction) SerializedSize() int {
	return tx.SerializeSize()
}

// Serialize encodes the transaction to bytes (Bitcoin wire protocol format with varints)
func (tx *Transaction) Serialize() ([]byte, error) {
	var buf bytes.Buffer

	// Version
	if err := binary.Write(&buf, binary.LittleEndian, tx.Version); err != nil {
		return nil, err
	}

	// Input count (Bitcoin compact size / varint)
	if err := WriteCompactSize(&buf, uint64(len(tx.Inputs))); err != nil {
		return nil, err
	}

	// Inputs
	for _, input := range tx.Inputs {
		buf.Write(input.PreviousOutput.Hash[:])
		if err := binary.Write(&buf, binary.LittleEndian, input.PreviousOutput.Index); err != nil {
			return nil, err
		}

		// Script length (varint) and script
		if err := WriteCompactSize(&buf, uint64(len(input.ScriptSig))); err != nil {
			return nil, err
		}
		buf.Write(input.ScriptSig)

		if err := binary.Write(&buf, binary.LittleEndian, input.Sequence); err != nil {
			return nil, err
		}
	}

	// Output count (varint)
	if err := WriteCompactSize(&buf, uint64(len(tx.Outputs))); err != nil {
		return nil, err
	}

	// Outputs
	for _, output := range tx.Outputs {
		if err := binary.Write(&buf, binary.LittleEndian, output.Value); err != nil {
			return nil, err
		}

		// Script length (varint) and script
		if err := WriteCompactSize(&buf, uint64(len(output.ScriptPubKey))); err != nil {
			return nil, err
		}
		buf.Write(output.ScriptPubKey)
	}

	// LockTime
	if err := binary.Write(&buf, binary.LittleEndian, tx.LockTime); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// Deserialize decodes a transaction from bytes (Bitcoin wire protocol format with varints)
func DeserializeTransaction(data []byte) (*Transaction, error) {
	buf := bytes.NewReader(data)
	tx := &Transaction{}

	// Version
	if err := binary.Read(buf, binary.LittleEndian, &tx.Version); err != nil {
		return nil, err
	}

	// Input count (Bitcoin compact size / varint)
	inputCount, err := ReadCompactSize(buf)
	if err != nil {
		return nil, err
	}

	// Inputs
	tx.Inputs = make([]*TxInput, inputCount)
	for i := uint64(0); i < inputCount; i++ {
		input := &TxInput{}

		// Previous output hash
		if _, err := buf.Read(input.PreviousOutput.Hash[:]); err != nil {
			return nil, err
		}

		// Previous output index
		if err := binary.Read(buf, binary.LittleEndian, &input.PreviousOutput.Index); err != nil {
			return nil, err
		}

		// Script length (varint)
		scriptLen, err := ReadCompactSize(buf)
		if err != nil {
			return nil, err
		}

		// Script
		input.ScriptSig = make([]byte, scriptLen)
		if _, err := buf.Read(input.ScriptSig); err != nil {
			return nil, err
		}

		// Sequence
		if err := binary.Read(buf, binary.LittleEndian, &input.Sequence); err != nil {
			return nil, err
		}

		tx.Inputs[i] = input
	}

	// Output count (varint)
	outputCount, err := ReadCompactSize(buf)
	if err != nil {
		return nil, err
	}

	// Outputs
	tx.Outputs = make([]*TxOutput, outputCount)
	for i := uint64(0); i < outputCount; i++ {
		output := &TxOutput{}

		// Value
		if err := binary.Read(buf, binary.LittleEndian, &output.Value); err != nil {
			return nil, err
		}

		// Script length (varint)
		scriptLen, err := ReadCompactSize(buf)
		if err != nil {
			return nil, err
		}

		// Script
		output.ScriptPubKey = make([]byte, scriptLen)
		if _, err := buf.Read(output.ScriptPubKey); err != nil {
			return nil, err
		}

		tx.Outputs[i] = output
	}

	// LockTime
	if err := binary.Read(buf, binary.LittleEndian, &tx.LockTime); err != nil {
		return nil, err
	}

	return tx, nil
}

// SignatureHash creates the hash to be signed for a specific input
// This is used during transaction signing and verification
// Follows Bitcoin/TWINS legacy serialization format with SIGHASH support
func (tx *Transaction) SignatureHash(inputIndex int, scriptPubKey []byte, sigHashType uint32) Hash {
	if inputIndex >= len(tx.Inputs) {
		return ZeroHash
	}

	var buf bytes.Buffer

	// Write version
	binary.Write(&buf, binary.LittleEndian, tx.Version)

	// Handle ANYONECANPAY flag
	anyoneCanPay := (sigHashType & SigHashAnyoneCanPay) != 0
	baseType := sigHashType & 0x1f

	// Write inputs
	if anyoneCanPay {
		// ANYONECANPAY: only sign single input
		WriteCompactSize(&buf, 1)
		input := tx.Inputs[inputIndex]
		buf.Write(input.PreviousOutput.Hash[:])
		binary.Write(&buf, binary.LittleEndian, input.PreviousOutput.Index)

		// Use scriptPubKey for this input (remove OP_CODESEPARATOR if present)
		cleanScript := removeCodeSeparator(scriptPubKey)
		WriteCompactSize(&buf, uint64(len(cleanScript)))
		buf.Write(cleanScript)

		binary.Write(&buf, binary.LittleEndian, input.Sequence)
	} else {
		// Normal: include all inputs
		WriteCompactSize(&buf, uint64(len(tx.Inputs)))
		for i, input := range tx.Inputs {
			buf.Write(input.PreviousOutput.Hash[:])
			binary.Write(&buf, binary.LittleEndian, input.PreviousOutput.Index)

			if i == inputIndex {
				// For the input being signed, use scriptPubKey (cleaned)
				cleanScript := removeCodeSeparator(scriptPubKey)
				WriteCompactSize(&buf, uint64(len(cleanScript)))
				buf.Write(cleanScript)
			} else {
				// For other inputs, use empty script
				WriteCompactSize(&buf, 0)
			}

			// CRITICAL FIX: Sequence number handling for SIGHASH_SINGLE and SIGHASH_NONE
			// Legacy C++ behavior (interpreter.cpp:1043-1047):
			// - For SIGHASH_SINGLE or SIGHASH_NONE, inputs other than the one being signed
			//   have their sequence numbers set to 0 to allow others to update them
			// - Only the input being signed preserves its actual sequence number
			if i != inputIndex && (baseType == SigHashSingle || baseType == SigHashNone) {
				// Let the others update at will (set sequence to 0)
				binary.Write(&buf, binary.LittleEndian, uint32(0))
			} else {
				// Use actual sequence number
				binary.Write(&buf, binary.LittleEndian, input.Sequence)
			}
		}
	}

	// Write outputs based on SIGHASH type
	switch baseType {
	case SigHashNone:
		// SIGHASH_NONE: no outputs
		WriteCompactSize(&buf, 0)

	case SigHashSingle:
		// SIGHASH_SINGLE: only output at same index
		if inputIndex >= len(tx.Outputs) {
			// Invalid: return error hash (Bitcoin Core behavior)
			return NewHash([]byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
		}

		// Write outputs up to and including inputIndex
		WriteCompactSize(&buf, uint64(inputIndex+1))
		for i := 0; i <= inputIndex; i++ {
			if i < inputIndex {
				// Blank outputs before inputIndex
				binary.Write(&buf, binary.LittleEndian, int64(-1))
				WriteCompactSize(&buf, 0)
			} else {
				// Write the actual output at inputIndex
				binary.Write(&buf, binary.LittleEndian, tx.Outputs[i].Value)
				WriteCompactSize(&buf, uint64(len(tx.Outputs[i].ScriptPubKey)))
				buf.Write(tx.Outputs[i].ScriptPubKey)
			}
		}

	default: // SigHashAll
		// SIGHASH_ALL: include all outputs
		WriteCompactSize(&buf, uint64(len(tx.Outputs)))
		for _, output := range tx.Outputs {
			binary.Write(&buf, binary.LittleEndian, output.Value)
			WriteCompactSize(&buf, uint64(len(output.ScriptPubKey)))
			buf.Write(output.ScriptPubKey)
		}
	}

	binary.Write(&buf, binary.LittleEndian, tx.LockTime)

	// Append SIGHASH type (4 bytes)
	binary.Write(&buf, binary.LittleEndian, sigHashType)

	return NewHash(buf.Bytes())
}

// removeCodeSeparator removes OP_CODESEPARATOR opcodes from script
// This must parse the script properly to avoid removing 0xAB bytes that are part of pushed data
func removeCodeSeparator(script []byte) []byte {
	if len(script) == 0 {
		return script
	}

	result := make([]byte, 0, len(script))
	pc := 0

	for pc < len(script) {
		opcode := script[pc]
		pc++

		// Skip OP_CODESEPARATOR (don't add it to result)
		if opcode == OpCodeSeparator {
			continue
		}

		// Add the opcode
		result = append(result, opcode)

		// Handle data push opcodes - we must include all their data
		if opcode >= 0x01 && opcode <= 0x4b {
			// Direct push of N bytes
			dataLen := int(opcode)
			if pc+dataLen > len(script) {
				// Malformed script, but copy what we can
				result = append(result, script[pc:]...)
				break
			}
			result = append(result, script[pc:pc+dataLen]...)
			pc += dataLen
		} else if opcode == 0x4c { // OP_PUSHDATA1
			if pc >= len(script) {
				break
			}
			dataLen := int(script[pc])
			result = append(result, script[pc]) // Include length byte
			pc++
			if pc+dataLen > len(script) {
				result = append(result, script[pc:]...)
				break
			}
			result = append(result, script[pc:pc+dataLen]...)
			pc += dataLen
		} else if opcode == 0x4d { // OP_PUSHDATA2
			if pc+1 >= len(script) {
				break
			}
			result = append(result, script[pc:pc+2]...) // Include 2 length bytes
			dataLen := int(script[pc]) | (int(script[pc+1]) << 8)
			pc += 2
			if pc+dataLen > len(script) {
				result = append(result, script[pc:]...)
				break
			}
			result = append(result, script[pc:pc+dataLen]...)
			pc += dataLen
		} else if opcode == 0x4e { // OP_PUSHDATA4
			if pc+3 >= len(script) {
				break
			}
			result = append(result, script[pc:pc+4]...) // Include 4 length bytes
			dataLen := int(script[pc]) | (int(script[pc+1]) << 8) | (int(script[pc+2]) << 16) | (int(script[pc+3]) << 24)
			pc += 4
			if pc+dataLen > len(script) {
				result = append(result, script[pc:]...)
				break
			}
			result = append(result, script[pc:pc+dataLen]...)
			pc += dataLen
		}
	}

	return result
}
