package script

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/twins-dev/twins-core/pkg/crypto"
	"github.com/twins-dev/twins-core/pkg/types"
	"golang.org/x/crypto/ripemd160"
)

// Script flags for validation
const (
	ScriptVerifyNone          = 0
	ScriptVerifyP2SH          = 1 << 0  // Evaluate P2SH subscripts
	ScriptVerifyStrictEnc     = 1 << 1  // Enforce strict DER encoding
	ScriptVerifyDERSig        = 1 << 2  // Enforce strict DER signatures
	ScriptVerifyLowS          = 1 << 3  // Enforce low S values in signatures
	ScriptVerifyNullDummy     = 1 << 4  // Verify dummy stack item consumed by CHECKMULTISIG is of zero-length
	ScriptVerifyCheckLockTime = 1 << 9  // Verify CHECKLOCKTIMEVERIFY
	ScriptVerifyCheckSequence = 1 << 10 // Verify CHECKSEQUENCEVERIFY
)

// Standard script verification flags for TWINS
const StandardScriptVerifyFlags = ScriptVerifyP2SH | ScriptVerifyStrictEnc | ScriptVerifyDERSig

// StandardVerifyFlags is an alias for StandardScriptVerifyFlags for compatibility
const StandardVerifyFlags = StandardScriptVerifyFlags

// Engine represents the script execution engine
type Engine struct {
	stack     [][]byte
	altStack  [][]byte
	script    []byte
	tx        *types.Transaction
	txIndex   int
	flags     uint32
	condStack []bool // Track conditional execution state for IF/ELSE/ENDIF
	skipExec  bool   // True when inside a false branch
}

// NewEngine creates a new script execution engine
func NewEngine(scriptPubKey []byte, tx *types.Transaction, txIndex int, flags uint32) *Engine {
	return &Engine{
		stack:    make([][]byte, 0),
		altStack: make([][]byte, 0),
		script:   scriptPubKey,
		tx:       tx,
		txIndex:  txIndex,
		flags:    flags,
	}
}

// Execute runs the script
func (e *Engine) Execute(scriptSig []byte) error {
	// Execute scriptSig first
	if err := e.executeScript(scriptSig); err != nil {
		return fmt.Errorf("scriptSig execution failed: %w", err)
	}

	// Execute scriptPubKey
	if err := e.executeScript(e.script); err != nil {
		return fmt.Errorf("scriptPubKey execution failed: %w", err)
	}

	// Script succeeds if top stack value is true
	if len(e.stack) == 0 {
		return errors.New("script resulted in empty stack")
	}

	if !e.castToBool(e.stack[len(e.stack)-1]) {
		return errors.New("script evaluation failed")
	}

	return nil
}

// getOp extracts an opcode and its associated data from a script
// Returns the opcode, the data (if any), and the new program counter position
// Bitcoin standard maximum script element size
const maxScriptElementSize = 520

func getOp(script []byte, pc int) (opcode byte, data []byte, newPC int, err error) {
	if pc >= len(script) {
		return 0, nil, pc, errors.New("pc out of bounds")
	}

	opcode = script[pc]
	newPC = pc + 1

	// Handle push opcodes - extract data and advance PC past it
	if opcode <= OP_PUSHDATA4 {
		var dataSize int

		if opcode < OP_PUSHDATA1 {
			// Direct push of N bytes (opcodes 0x01 to 0x4b)
			dataSize = int(opcode)
		} else if opcode == OP_PUSHDATA1 {
			if newPC >= len(script) {
				return 0, nil, pc, errors.New("OP_PUSHDATA1: missing length byte")
			}
			dataSize = int(script[newPC])
			newPC++
		} else if opcode == OP_PUSHDATA2 {
			// Need 2 bytes for length
			if newPC+2 > len(script) {
				return 0, nil, pc, errors.New("OP_PUSHDATA2: missing length bytes")
			}
			dataSize = int(script[newPC]) | (int(script[newPC+1]) << 8)
			newPC += 2
		} else if opcode == OP_PUSHDATA4 {
			// Need 4 bytes for length
			if newPC+4 > len(script) {
				return 0, nil, pc, errors.New("OP_PUSHDATA4: missing length bytes")
			}
			dataSize = int(script[newPC]) | (int(script[newPC+1]) << 8) |
			          (int(script[newPC+2]) << 16) | (int(script[newPC+3]) << 24)
			newPC += 4

			// Validate size to prevent integer overflow and excessive allocation
			if dataSize < 0 || dataSize > maxScriptElementSize {
				return 0, nil, pc, fmt.Errorf("push data size out of bounds: %d (max %d)", dataSize, maxScriptElementSize)
			}
		}

		// Validate dataSize for all PUSHDATA operations
		if dataSize < 0 || dataSize > maxScriptElementSize {
			return 0, nil, pc, fmt.Errorf("push data size out of bounds: %d (max %d)", dataSize, maxScriptElementSize)
		}

		// Extract the data
		if dataSize > 0 {
			if newPC+dataSize > len(script) {
				return 0, nil, pc, fmt.Errorf("push data exceeds script length: need %d bytes, have %d",
					dataSize, len(script)-newPC)
			}
			data = make([]byte, dataSize)
			copy(data, script[newPC:newPC+dataSize])
			newPC += dataSize
		}
	}

	return opcode, data, newPC, nil
}

// executeScript executes a script
func (e *Engine) executeScript(script []byte) error {
	pc := 0
	for pc < len(script) {
		opcode, data, newPC, err := getOp(script, pc)
		if err != nil {
			return err
		}

		// Update program counter to skip past opcode and any data
		pc = newPC

		// Control flow opcodes must always execute to maintain proper nesting
		isControlFlow := opcode == OP_IF || opcode == OP_NOTIF || opcode == OP_ELSE || opcode == OP_ENDIF

		// Skip execution if in false branch (except for control flow opcodes)
		if e.skipExec && !isControlFlow {
			// We already skipped past data in getOp, so just continue
			continue
		}

		// Handle push opcodes - push the data onto the stack
		if opcode > 0 && opcode <= OP_PUSHDATA4 {
			if data != nil {
				e.push(data)
			} else if opcode > 0 {
				// Push empty data for OP_0 equivalent push
				e.push([]byte{})
			}
			continue
		}

		// Special case: OP_0 should push empty value
		if opcode == 0 {
			e.push([]byte{})
			continue
		}

		// Execute regular opcodes
		if err := e.executeOpcode(opcode); err != nil {
			return fmt.Errorf("opcode %s failed: %w", GetOpcodeName(opcode), err)
		}
	}

	return nil
}

// executeOpcode executes a single opcode
func (e *Engine) executeOpcode(opcode byte) error {
	switch opcode {
	case OP_0:
		e.push([]byte{})
	case OP_1:
		e.push([]byte{1})
	case OP_2, OP_2 + 1, OP_2 + 2, OP_2 + 3, OP_2 + 4, OP_2 + 5, OP_2 + 6, OP_2 + 7,
		OP_2 + 8, OP_2 + 9, OP_2 + 10, OP_2 + 11, OP_2 + 12, OP_2 + 13, OP_2 + 14:
		// OP_2 through OP_16 (0x52-0x60): push small integers 2-16
		value := opcode - OP_2 + 2
		e.push([]byte{value})

	// Flow control
	case OP_NOP:
		// No operation
	case OP_IF:
		return e.opIf()
	case OP_NOTIF:
		return e.opNotIf()
	case OP_ELSE:
		return e.opElse()
	case OP_ENDIF:
		return e.opEndIf()
	case OP_RETURN:
		return errors.New("OP_RETURN executed")

	// Stack operations
	case OP_TOALTSTACK:
		return e.opToAltStack()
	case OP_FROMALTSTACK:
		return e.opFromAltStack()
	case OP_2DROP:
		return e.op2Drop()
	case OP_2DUP:
		return e.op2Dup()
	case OP_DUP:
		return e.opDup()
	case OP_DROP:
		return e.opDrop()

	// Arithmetic operations
	case OP_1ADD:
		return e.op1Add()
	case OP_1SUB:
		return e.op1Sub()
	case OP_NEGATE:
		return e.opNegate()
	case OP_ABS:
		return e.opAbs()
	case OP_NOT:
		return e.opNot()
	case OP_0NOTEQUAL:
		return e.op0NotEqual()
	case OP_ADD:
		return e.opAdd()
	case OP_SUB:
		return e.opSub()

	// Crypto operations
	case OP_RIPEMD160:
		return e.opRIPEMD160()
	case OP_SHA256:
		return e.opSHA256()
	case OP_HASH160:
		return e.opHash160()
	case OP_HASH256:
		return e.opHash256()

	// Signature verification
	case OP_EQUAL:
		return e.opEqual()
	case OP_EQUALVERIFY:
		if err := e.opEqual(); err != nil {
			return err
		}
		return e.opVerify()
	case OP_CHECKSIG:
		return e.opCheckSig()
	case OP_CHECKSIGVERIFY:
		if err := e.opCheckSig(); err != nil {
			return err
		}
		return e.opVerify()
	case OP_CHECKMULTISIG:
		return e.opCheckMultiSig()

	// Timelock
	case OP_CHECKLOCKTIMEVERIFY:
		return e.opCheckLockTimeVerify()

	case OP_VERIFY:
		return e.opVerify()

	// Zerocoin opcodes (not implemented, return error)
	case 0xc1: // OP_ZEROCOINMINT
		return errors.New("OP_ZEROCOINMINT not implemented")
	case 0xc2: // OP_ZEROCOINSPEND
		return errors.New("OP_ZEROCOINSPEND not implemented")

	default:
		return fmt.Errorf("opcode not implemented: %s (0x%02x)", GetOpcodeName(opcode), opcode)
	}
	return nil
}

// Stack operations

func (e *Engine) push(data []byte) {
	e.stack = append(e.stack, data)
}

func (e *Engine) pop() ([]byte, error) {
	if len(e.stack) == 0 {
		return nil, errors.New("stack underflow")
	}
	val := e.stack[len(e.stack)-1]
	e.stack = e.stack[:len(e.stack)-1]
	return val, nil
}

func (e *Engine) peek(n int) ([]byte, error) {
	if len(e.stack) < n+1 {
		return nil, errors.New("stack underflow")
	}
	return e.stack[len(e.stack)-1-n], nil
}

func (e *Engine) castToBool(data []byte) bool {
	for i, b := range data {
		if b != 0 {
			// Negative zero is false
			if i == len(data)-1 && b == 0x80 {
				return false
			}
			return true
		}
	}
	return false
}

// Opcode implementations

func (e *Engine) opDup() error {
	val, err := e.peek(0)
	if err != nil {
		return err
	}
	e.push(val)
	return nil
}

func (e *Engine) opHash160() error {
	data, err := e.pop()
	if err != nil {
		return err
	}

	// HASH160 = RIPEMD160(SHA256(data))
	sha := sha256.Sum256(data)
	hasher := ripemd160.New()
	hasher.Write(sha[:])
	hash := hasher.Sum(nil)

	e.push(hash)
	return nil
}

func (e *Engine) opEqual() error {
	a, err := e.pop()
	if err != nil {
		return err
	}
	b, err := e.pop()
	if err != nil {
		return err
	}

	if bytes.Equal(a, b) {
		e.push([]byte{1})
	} else {
		e.push([]byte{})
	}
	return nil
}

func (e *Engine) opVerify() error {
	val, err := e.pop()
	if err != nil {
		return err
	}
	if !e.castToBool(val) {
		return errors.New("verify failed")
	}
	return nil
}

func (e *Engine) opDrop() error {
	_, err := e.pop()
	return err
}

func (e *Engine) opCheckSig() error {
	pubKeyBytes, err := e.pop()
	if err != nil {
		return err
	}
	sigBytes, err := e.pop()
	if err != nil {
		return err
	}

	// Parse public key
	pubKey, err := crypto.ParsePublicKeyFromBytes(pubKeyBytes)
	if err != nil {
		e.push([]byte{}) // Push false
		return nil       // CheckSig failure is not an error, just pushes false
	}

	// Extract SIGHASH type from signature (last byte)
	if len(sigBytes) < 1 {
		e.push([]byte{})
		return nil
	}
	sigHashType := uint32(sigBytes[len(sigBytes)-1])
	sig := sigBytes[:len(sigBytes)-1]

	// Get signature hash for this input using proper SIGHASH serialization
	// This uses the scriptPubKey being executed (e.script in this context)
	sigHash := e.tx.SignatureHash(e.txIndex, e.script, sigHashType)

	// Verify signature using pre-computed hash (sigHash is already hashed, don't double-hash)
	// DER-encoded signatures from legacy nodes are handled by VerifySignature
	if crypto.VerifySignature(pubKey, sigHash[:], sig) {
		e.push([]byte{1})
	} else {
		e.push([]byte{})
	}

	return nil
}

func (e *Engine) opCheckMultiSig() error {
	// Pop number of public keys
	nPubKeysBytes, err := e.pop()
	if err != nil {
		return err
	}
	if len(nPubKeysBytes) == 0 {
		return errors.New("invalid pubkey count")
	}
	nPubKeys := int(nPubKeysBytes[0])

	if nPubKeys < 0 || nPubKeys > 20 {
		return errors.New("pubkey count out of range")
	}

	// Pop public keys
	pubKeys := make([][]byte, nPubKeys)
	for i := 0; i < nPubKeys; i++ {
		pubKeys[i], err = e.pop()
		if err != nil {
			return err
		}
	}

	// Pop number of signatures
	nSigsBytes, err := e.pop()
	if err != nil {
		return err
	}
	if len(nSigsBytes) == 0 {
		return errors.New("invalid signature count")
	}
	nSigs := int(nSigsBytes[0])

	if nSigs < 0 || nSigs > nPubKeys {
		return errors.New("signature count out of range")
	}

	// Pop signatures
	sigs := make([][]byte, nSigs)
	for i := 0; i < nSigs; i++ {
		sigs[i], err = e.pop()
		if err != nil {
			return err
		}
	}

	// Pop dummy element (Bitcoin bug compatibility)
	_, err = e.pop()
	if err != nil {
		return err
	}

	// Verify signatures
	// Check if we have enough valid signatures
	validCount := 0

	for _, sigBytes := range sigs {
		if len(sigBytes) < 1 {
			continue
		}

		// Extract SIGHASH type from signature (last byte)
		sigHashType := uint32(sigBytes[len(sigBytes)-1])
		sig := sigBytes[:len(sigBytes)-1]

		// Get signature hash using proper SIGHASH serialization
		sigHash := e.tx.SignatureHash(e.txIndex, e.script, sigHashType)

		for _, pubKeyBytes := range pubKeys {
			pubKey, err := crypto.ParsePublicKeyFromBytes(pubKeyBytes)
			if err != nil {
				continue
			}
			// Verify using pre-computed hash (sigHash is already hashed)
			// DER-encoded signatures from legacy nodes are handled by VerifySignature
			if crypto.VerifySignature(pubKey, sigHash[:], sig) {
				validCount++
				break
			}
		}
	}

	if validCount >= nSigs {
		e.push([]byte{1})
	} else {
		e.push([]byte{})
	}

	return nil
}

// VerifyScript verifies a transaction input script
func VerifyScript(scriptSig, scriptPubKey []byte, tx *types.Transaction, txIndex int, flags uint32) error {
	engine := NewEngine(scriptPubKey, tx, txIndex, flags)
	return engine.Execute(scriptSig)
}

// IsP2PKH checks if a script is Pay-to-PubKey-Hash
func IsP2PKH(script []byte) bool {
	// P2PKH: OP_DUP OP_HASH160 <20 bytes> OP_EQUALVERIFY OP_CHECKSIG
	return len(script) == 25 &&
		script[0] == OP_DUP &&
		script[1] == OP_HASH160 &&
		script[2] == 0x14 && // 20 bytes
		script[23] == OP_EQUALVERIFY &&
		script[24] == OP_CHECKSIG
}

// IsP2PK checks if a script is Pay-to-PubKey
func IsP2PK(script []byte) bool {
	// P2PK: <pubkey> OP_CHECKSIG
	if len(script) < 2 {
		return false
	}
	lastByte := script[len(script)-1]
	if lastByte != OP_CHECKSIG {
		return false
	}
	// Check for compressed (33 bytes) or uncompressed (65 bytes) pubkey
	return len(script) == 35 || len(script) == 67
}

// ExtractPubKeyHash extracts the pubkey hash from a P2PKH script
func ExtractPubKeyHash(script []byte) []byte {
	if !IsP2PKH(script) {
		return nil
	}
	return script[3:23]
}

// Additional opcode implementations

// Flow control operations

func (e *Engine) opIf() error {
	if e.skipExec {
		e.condStack = append(e.condStack, false)
		return nil
	}

	val, err := e.pop()
	if err != nil {
		return err
	}

	condition := e.castToBool(val)
	e.condStack = append(e.condStack, condition)
	if !condition {
		e.skipExec = true
	}
	return nil
}

func (e *Engine) opNotIf() error {
	if e.skipExec {
		e.condStack = append(e.condStack, false)
		return nil
	}

	val, err := e.pop()
	if err != nil {
		return err
	}

	condition := !e.castToBool(val)
	e.condStack = append(e.condStack, condition)
	if !condition {
		e.skipExec = true
	}
	return nil
}

func (e *Engine) opElse() error {
	if len(e.condStack) == 0 {
		return errors.New("OP_ELSE without OP_IF")
	}

	// Toggle the top condition
	e.condStack[len(e.condStack)-1] = !e.condStack[len(e.condStack)-1]

	// Update skipExec based on all conditions
	e.skipExec = false
	for _, cond := range e.condStack {
		if !cond {
			e.skipExec = true
			break
		}
	}
	return nil
}

func (e *Engine) opEndIf() error {
	if len(e.condStack) == 0 {
		return errors.New("OP_ENDIF without OP_IF")
	}

	// Pop the condition
	e.condStack = e.condStack[:len(e.condStack)-1]

	// Update skipExec based on remaining conditions
	e.skipExec = false
	for _, cond := range e.condStack {
		if !cond {
			e.skipExec = true
			break
		}
	}
	return nil
}

// Stack operations

func (e *Engine) opToAltStack() error {
	val, err := e.pop()
	if err != nil {
		return err
	}
	e.altStack = append(e.altStack, val)
	return nil
}

func (e *Engine) opFromAltStack() error {
	if len(e.altStack) == 0 {
		return errors.New("alt stack underflow")
	}
	val := e.altStack[len(e.altStack)-1]
	e.altStack = e.altStack[:len(e.altStack)-1]
	e.push(val)
	return nil
}

func (e *Engine) op2Drop() error {
	_, err := e.pop()
	if err != nil {
		return err
	}
	_, err = e.pop()
	return err
}

func (e *Engine) op2Dup() error {
	val1, err := e.peek(1)
	if err != nil {
		return err
	}
	val2, err := e.peek(0)
	if err != nil {
		return err
	}
	e.push(val1)
	e.push(val2)
	return nil
}

// Arithmetic operations

// asInt converts script bytes to int64 for arithmetic operations
func asInt(b []byte) (int64, error) {
	if len(b) == 0 {
		return 0, nil
	}
	if len(b) > 4 {
		return 0, errors.New("numeric value too large")
	}

	// Parse little-endian integer
	var result int64
	negative := (b[len(b)-1] & 0x80) != 0

	for i := 0; i < len(b); i++ {
		val := int64(b[i])
		if i == len(b)-1 && negative {
			val &= 0x7f
		}
		result |= val << uint(8*i)
	}

	if negative {
		result = -result
	}
	return result, nil
}

// fromInt converts int64 to script bytes
func fromInt(n int64) []byte {
	if n == 0 {
		return []byte{}
	}

	negative := n < 0
	if negative {
		n = -n
	}

	result := []byte{}
	for n > 0 {
		result = append(result, byte(n&0xff))
		n >>= 8
	}

	// Add sign bit if needed
	if (result[len(result)-1] & 0x80) != 0 {
		if negative {
			result = append(result, 0x80)
		} else {
			result = append(result, 0x00)
		}
	} else if negative {
		result[len(result)-1] |= 0x80
	}

	return result
}

func (e *Engine) op1Add() error {
	val, err := e.pop()
	if err != nil {
		return err
	}
	n, err := asInt(val)
	if err != nil {
		return err
	}
	e.push(fromInt(n + 1))
	return nil
}

func (e *Engine) op1Sub() error {
	val, err := e.pop()
	if err != nil {
		return err
	}
	n, err := asInt(val)
	if err != nil {
		return err
	}
	e.push(fromInt(n - 1))
	return nil
}

func (e *Engine) opNegate() error {
	val, err := e.pop()
	if err != nil {
		return err
	}
	n, err := asInt(val)
	if err != nil {
		return err
	}
	e.push(fromInt(-n))
	return nil
}

func (e *Engine) opAbs() error {
	val, err := e.pop()
	if err != nil {
		return err
	}
	n, err := asInt(val)
	if err != nil {
		return err
	}
	if n < 0 {
		n = -n
	}
	e.push(fromInt(n))
	return nil
}

func (e *Engine) opNot() error {
	val, err := e.pop()
	if err != nil {
		return err
	}
	n, err := asInt(val)
	if err != nil {
		return err
	}
	if n == 0 {
		e.push([]byte{1})
	} else {
		e.push([]byte{})
	}
	return nil
}

func (e *Engine) op0NotEqual() error {
	val, err := e.pop()
	if err != nil {
		return err
	}
	n, err := asInt(val)
	if err != nil {
		return err
	}
	if n != 0 {
		e.push([]byte{1})
	} else {
		e.push([]byte{})
	}
	return nil
}

func (e *Engine) opAdd() error {
	b, err := e.pop()
	if err != nil {
		return err
	}
	a, err := e.pop()
	if err != nil {
		return err
	}

	na, err := asInt(a)
	if err != nil {
		return err
	}
	nb, err := asInt(b)
	if err != nil {
		return err
	}

	e.push(fromInt(na + nb))
	return nil
}

func (e *Engine) opSub() error {
	b, err := e.pop()
	if err != nil {
		return err
	}
	a, err := e.pop()
	if err != nil {
		return err
	}

	na, err := asInt(a)
	if err != nil {
		return err
	}
	nb, err := asInt(b)
	if err != nil {
		return err
	}

	e.push(fromInt(na - nb))
	return nil
}

// Crypto operations

func (e *Engine) opRIPEMD160() error {
	data, err := e.pop()
	if err != nil {
		return err
	}

	hasher := ripemd160.New()
	hasher.Write(data)
	hash := hasher.Sum(nil)

	e.push(hash)
	return nil
}

func (e *Engine) opSHA256() error {
	data, err := e.pop()
	if err != nil {
		return err
	}

	hash := sha256.Sum256(data)
	e.push(hash[:])
	return nil
}

func (e *Engine) opHash256() error {
	data, err := e.pop()
	if err != nil {
		return err
	}

	// HASH256 = SHA256(SHA256(data))
	first := sha256.Sum256(data)
	second := sha256.Sum256(first[:])

	e.push(second[:])
	return nil
}

// Timelock operations

const LOCKTIME_THRESHOLD = 500000000 // Tue Nov  5 00:53:20 1985 UTC

func (e *Engine) opCheckLockTimeVerify() error {
	if len(e.stack) == 0 {
		return errors.New("stack empty for CHECKLOCKTIMEVERIFY")
	}

	// Peek at the top stack value (don't pop)
	lockTimeBytes := e.stack[len(e.stack)-1]
	lockTime, err := asInt(lockTimeBytes)
	if err != nil {
		return err
	}

	if lockTime < 0 {
		return errors.New("negative locktime")
	}

	// Check if locktime is disabled (nSequence == 0xffffffff)
	if e.tx.Inputs[e.txIndex].Sequence == 0xffffffff {
		return errors.New("transaction locktime disabled")
	}

	// Check locktime threshold consistency
	txLockTime := int64(e.tx.LockTime)
	if (txLockTime < LOCKTIME_THRESHOLD && lockTime >= LOCKTIME_THRESHOLD) ||
		(txLockTime >= LOCKTIME_THRESHOLD && lockTime < LOCKTIME_THRESHOLD) {
		return errors.New("locktime threshold mismatch")
	}

	// Verify the transaction's locktime is >= the script's locktime
	if txLockTime < lockTime {
		return errors.New("locktime requirement not satisfied")
	}

	return nil
}

// ExtractSignatureAndPubKey extracts signature and public key from a scriptSig
// This works for standard P2PKH scriptSig: <signature> <pubkey>
func ExtractSignatureAndPubKey(scriptSig []byte) (sig []byte, pubKey []byte, err error) {
	if len(scriptSig) == 0 {
		return nil, nil, errors.New("empty scriptSig")
	}

	// Parse the script to extract components
	// Standard P2PKH scriptSig has two push operations: signature and pubkey
	pos := 0

	// Extract signature
	if pos >= len(scriptSig) {
		return nil, nil, errors.New("scriptSig too short for signature")
	}
	sigLen := int(scriptSig[pos])
	pos++

	if pos+sigLen > len(scriptSig) {
		return nil, nil, errors.New("invalid signature length in scriptSig")
	}
	sig = scriptSig[pos : pos+sigLen]
	pos += sigLen

	// Extract public key
	if pos >= len(scriptSig) {
		return nil, nil, errors.New("scriptSig too short for pubkey")
	}
	pubKeyLen := int(scriptSig[pos])
	pos++

	if pos+pubKeyLen > len(scriptSig) {
		return nil, nil, errors.New("invalid pubkey length in scriptSig")
	}
	pubKey = scriptSig[pos : pos+pubKeyLen]

	return sig, pubKey, nil
}

// ExtractPubKeyFromScript extracts a public key from a scriptPubKey
// This handles P2PK scripts and coinstake outputs
func ExtractPubKeyFromScript(scriptPubKey []byte) ([]byte, error) {
	if len(scriptPubKey) == 0 {
		return nil, errors.New("empty script")
	}

	// Check for P2PK script: <pubkey> OP_CHECKSIG
	if IsP2PK(scriptPubKey) {
		// Extract the public key (all bytes except the last OP_CHECKSIG)
		pubKeyLen := len(scriptPubKey) - 1
		if pubKeyLen == 34 { // 33 bytes compressed + 1 byte length prefix
			return scriptPubKey[1:34], nil
		} else if pubKeyLen == 66 { // 65 bytes uncompressed + 1 byte length prefix
			return scriptPubKey[1:66], nil
		}
	}

	// Check for P2PKH script - can't extract pubkey directly
	if IsP2PKH(scriptPubKey) {
		return nil, errors.New("cannot extract pubkey from P2PKH script")
	}

	// For other script types, try to find a pubkey push
	// Look for 33-byte (compressed) or 65-byte (uncompressed) pushes
	pos := 0
	for pos < len(scriptPubKey) {
		opcode := scriptPubKey[pos]
		pos++

		// Check for pubkey push operations
		if opcode == 33 && pos+33 <= len(scriptPubKey) {
			// Compressed pubkey
			pubKey := scriptPubKey[pos : pos+33]
			// Verify it looks like a compressed pubkey (starts with 0x02 or 0x03)
			if pubKey[0] == 0x02 || pubKey[0] == 0x03 {
				return pubKey, nil
			}
			pos += 33
		} else if opcode == 65 && pos+65 <= len(scriptPubKey) {
			// Uncompressed pubkey
			pubKey := scriptPubKey[pos : pos+65]
			// Verify it looks like an uncompressed pubkey (starts with 0x04)
			if pubKey[0] == 0x04 {
				return pubKey, nil
			}
			pos += 65
		} else if opcode <= 75 {
			// Other push operation, skip the data
			pos += int(opcode)
		}
	}

	return nil, errors.New("no valid pubkey found in script")
}
