package script

import (
	"encoding/hex"
	"fmt"

	"github.com/twins-dev/twins-core/pkg/crypto"
)

// ScriptType represents the type of script
type ScriptType int

const (
	NonStandardTy ScriptType = iota
	PubKeyTy
	PubKeyHashTy
	ScriptHashTy
	MultiSigTy
	NullDataTy
)

// String returns the string representation of ScriptType
func (t ScriptType) String() string {
	switch t {
	case PubKeyTy:
		return "pubkey"
	case PubKeyHashTy:
		return "pubkeyhash"
	case ScriptHashTy:
		return "scripthash"
	case MultiSigTy:
		return "multisig"
	case NullDataTy:
		return "nulldata"
	default:
		return "nonstandard"
	}
}

// Disassemble converts script bytecode to human-readable ASM
func Disassemble(script []byte) (string, error) {
	if len(script) == 0 {
		return "", nil
	}

	var asm string
	pc := 0

	for pc < len(script) {
		opcode, data, newPC, err := getOp(script, pc)
		if err != nil {
			return asm, fmt.Errorf("disassemble error at pc %d: %w", pc, err)
		}

		// Add space separator if not first opcode
		if asm != "" {
			asm += " "
		}

		// Format opcode
		if opcode <= OP_PUSHDATA4 && len(data) > 0 {
			// Push data operation
			asm += hex.EncodeToString(data)
		} else {
			// Regular opcode
			opName := GetOpcodeName(opcode)
			asm += opName
		}

		pc = newPC
	}

	return asm, nil
}

// GetScriptType determines the type of script
func GetScriptType(script []byte) ScriptType {
	if len(script) == 0 {
		return NonStandardTy
	}

	// Check P2PKH: OP_DUP OP_HASH160 <20 bytes> OP_EQUALVERIFY OP_CHECKSIG
	if len(script) == 25 &&
		script[0] == OP_DUP &&
		script[1] == OP_HASH160 &&
		script[2] == 0x14 && // Push 20 bytes
		script[23] == OP_EQUALVERIFY &&
		script[24] == OP_CHECKSIG {
		return PubKeyHashTy
	}

	// Check P2SH: OP_HASH160 <20 bytes> OP_EQUAL
	if len(script) == 23 &&
		script[0] == OP_HASH160 &&
		script[1] == 0x14 && // Push 20 bytes
		script[22] == OP_EQUAL {
		return ScriptHashTy
	}

	// Check P2PK: <pubkey> OP_CHECKSIG
	// Public keys are either 33 bytes (compressed) or 65 bytes (uncompressed)
	if len(script) > 1 && script[len(script)-1] == OP_CHECKSIG {
		// Check if it's a single push followed by OP_CHECKSIG
		if (len(script) == 35 && script[0] == 0x21) || // 33-byte compressed key
			(len(script) == 67 && script[0] == 0x41) { // 65-byte uncompressed key
			return PubKeyTy
		}
	}

	// Check MultiSig: <M> <pubkey1> ... <pubkeyN> <N> OP_CHECKMULTISIG
	if len(script) > 3 && script[len(script)-1] == OP_CHECKMULTISIG {
		// Basic check - starts with OP_1 to OP_16, ends with OP_CHECKMULTISIG
		if script[0] >= OP_1 && script[0] <= OP_16 {
			return MultiSigTy
		}
	}

	// Check OP_RETURN (null data)
	if len(script) > 0 && script[0] == OP_RETURN {
		return NullDataTy
	}

	return NonStandardTy
}

// ExtractPubKeyHashAddress extracts the address from a P2PKH script
func ExtractPubKeyHashAddress(script []byte) (*crypto.Address, error) {
	// P2PKH: OP_DUP OP_HASH160 <20 bytes> OP_EQUALVERIFY OP_CHECKSIG
	if len(script) != 25 {
		return nil, fmt.Errorf("invalid P2PKH script length: %d", len(script))
	}

	if script[0] != OP_DUP ||
		script[1] != OP_HASH160 ||
		script[2] != 0x14 ||
		script[23] != OP_EQUALVERIFY ||
		script[24] != OP_CHECKSIG {
		return nil, fmt.Errorf("invalid P2PKH script format")
	}

	// Extract pubkey hash (bytes 3-22)
	pkHash := script[3:23]
	addr, err := crypto.NewAddressFromHash(pkHash, crypto.MainNetPubKeyHashAddrID)
	if err != nil {
		return nil, fmt.Errorf("failed to create address: %w", err)
	}
	return addr, nil
}

// ExtractScriptHash extracts the address from a P2SH script
func ExtractScriptHash(script []byte) (*crypto.Address, error) {
	// P2SH: OP_HASH160 <20 bytes> OP_EQUAL
	if len(script) != 23 {
		return nil, fmt.Errorf("invalid P2SH script length: %d", len(script))
	}

	if script[0] != OP_HASH160 ||
		script[1] != 0x14 ||
		script[22] != OP_EQUAL {
		return nil, fmt.Errorf("invalid P2SH script format")
	}

	// Extract script hash (bytes 2-21)
	scriptHash := script[2:22]
	addr, err := crypto.NewAddressFromHash(scriptHash, crypto.MainNetScriptHashAddrID)
	if err != nil {
		return nil, fmt.Errorf("failed to create address: %w", err)
	}
	return addr, nil
}

// ExtractPubKey extracts the address from a P2PK script
func ExtractPubKey(script []byte) (*crypto.Address, error) {
	// P2PK: <pubkey> OP_CHECKSIG
	if len(script) < 2 || script[len(script)-1] != OP_CHECKSIG {
		return nil, fmt.Errorf("invalid P2PK script format")
	}

	// Extract pubkey (all bytes except last OP_CHECKSIG)
	var pubKeyBytes []byte

	// Compressed pubkey: 33 bytes
	if len(script) == 35 && script[0] == 0x21 {
		pubKeyBytes = script[1:34]
	} else if len(script) == 67 && script[0] == 0x41 {
		// Uncompressed pubkey: 65 bytes
		pubKeyBytes = script[1:66]
	} else {
		return nil, fmt.Errorf("invalid P2PK pubkey length")
	}

	// Parse public key
	pubKey, err := crypto.ParsePubKey(pubKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	// Generate address from public key
	addr := crypto.NewAddressFromPubKey(pubKey, crypto.MainNetPubKeyHashAddrID)
	return addr, nil
}

// ExtractMultisig extracts addresses and required signatures from a multisig script
func ExtractMultisig(script []byte) ([]*crypto.Address, int, error) {
	// MultiSig: <M> <pubkey1> ... <pubkeyN> <N> OP_CHECKMULTISIG
	if len(script) < 4 || script[len(script)-1] != OP_CHECKMULTISIG {
		return nil, 0, fmt.Errorf("invalid multisig script format")
	}

	// Extract M (required signatures)
	if script[0] < OP_1 || script[0] > OP_16 {
		return nil, 0, fmt.Errorf("invalid M value in multisig")
	}
	m := int(script[0] - OP_1 + 1)

	// Extract N (total pubkeys) - second to last byte
	nByte := script[len(script)-2]
	if nByte < OP_1 || nByte > OP_16 {
		return nil, 0, fmt.Errorf("invalid N value in multisig")
	}
	n := int(nByte - OP_1 + 1)

	if m > n {
		return nil, 0, fmt.Errorf("M > N in multisig")
	}

	// Parse pubkeys
	var addresses []*crypto.Address
	pc := 1 // Start after M

	for pc < len(script)-2 { // Stop before N and OP_CHECKMULTISIG
		opcode, data, newPC, err := getOp(script, pc)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to parse multisig pubkey: %w", err)
		}

		// Pubkeys should be push operations
		if opcode > OP_PUSHDATA4 {
			break
		}

		// Parse public key
		if len(data) == 33 || len(data) == 65 {
			pubKey, err := crypto.ParsePubKey(data)
			if err != nil {
				return nil, 0, fmt.Errorf("failed to parse multisig public key: %w", err)
			}
			addr := crypto.NewAddressFromPubKey(pubKey, crypto.MainNetPubKeyHashAddrID)
			addresses = append(addresses, addr)
		}

		pc = newPC
	}

	if len(addresses) != n {
		return nil, 0, fmt.Errorf("expected %d pubkeys, found %d", n, len(addresses))
	}

	return addresses, m, nil
}

