package script

import "fmt"

// Bitcoin script opcodes
// Reference: legacy/src/script/script.h

// Constants
const (
	OP_0         = 0x00
	OP_FALSE     = OP_0
	OP_PUSHDATA1 = 0x4c
	OP_PUSHDATA2 = 0x4d
	OP_PUSHDATA4 = 0x4e
	OP_1NEGATE   = 0x4f
	OP_1         = 0x51
	OP_TRUE      = OP_1
	OP_2         = 0x52
	OP_3         = 0x53
	OP_4         = 0x54
	OP_5         = 0x55
	OP_6         = 0x56
	OP_7         = 0x57
	OP_8         = 0x58
	OP_9         = 0x59
	OP_10        = 0x5a
	OP_11        = 0x5b
	OP_12        = 0x5c
	OP_13        = 0x5d
	OP_14        = 0x5e
	OP_15        = 0x5f
	OP_16        = 0x60
)

// Flow control
const (
	OP_NOP    = 0x61
	OP_IF     = 0x63
	OP_NOTIF  = 0x64
	OP_ELSE   = 0x67
	OP_ENDIF  = 0x68
	OP_VERIFY = 0x69
	OP_RETURN = 0x6a
)

// Stack
const (
	OP_TOALTSTACK   = 0x6b
	OP_FROMALTSTACK = 0x6c
	OP_IFDUP        = 0x73
	OP_DEPTH        = 0x74
	OP_DROP         = 0x75
	OP_DUP          = 0x76
	OP_NIP          = 0x77
	OP_OVER         = 0x78
	OP_PICK         = 0x79
	OP_ROLL         = 0x7a
	OP_ROT          = 0x7b
	OP_SWAP         = 0x7c
	OP_TUCK         = 0x7d
	OP_2DROP        = 0x6d
	OP_2DUP         = 0x6e
	OP_3DUP         = 0x6f
	OP_2OVER        = 0x70
	OP_2ROT         = 0x71
	OP_2SWAP        = 0x72
)

// Splice
const (
	OP_SIZE = 0x82
)

// Bitwise logic
const (
	OP_EQUAL       = 0x87
	OP_EQUALVERIFY = 0x88
)

// Arithmetic
const (
	OP_1ADD               = 0x8b
	OP_1SUB               = 0x8c
	OP_NEGATE             = 0x8f
	OP_ABS                = 0x90
	OP_NOT                = 0x91
	OP_0NOTEQUAL          = 0x92
	OP_ADD                = 0x93
	OP_SUB                = 0x94
	OP_BOOLAND            = 0x9a
	OP_BOOLOR             = 0x9b
	OP_NUMEQUAL           = 0x9c
	OP_NUMEQUALVERIFY     = 0x9d
	OP_NUMNOTEQUAL        = 0x9e
	OP_LESSTHAN           = 0x9f
	OP_GREATERTHAN        = 0xa0
	OP_LESSTHANOREQUAL    = 0xa1
	OP_GREATERTHANOREQUAL = 0xa2
	OP_MIN                = 0xa3
	OP_MAX                = 0xa4
	OP_WITHIN             = 0xa5
)

// Crypto
const (
	OP_RIPEMD160           = 0xa6
	OP_SHA1                = 0xa7
	OP_SHA256              = 0xa8
	OP_HASH160             = 0xa9
	OP_HASH256             = 0xaa
	OP_CODESEPARATOR       = 0xab
	OP_CHECKSIG            = 0xac
	OP_CHECKSIGVERIFY      = 0xad
	OP_CHECKMULTISIG       = 0xae
	OP_CHECKMULTISIGVERIFY = 0xaf
)

// Expansion
const (
	OP_NOP1                = 0xb0
	OP_CHECKLOCKTIMEVERIFY = 0xb1
	OP_NOP2                = OP_CHECKLOCKTIMEVERIFY
	OP_CHECKSEQUENCEVERIFY = 0xb2
	OP_NOP3                = OP_CHECKSEQUENCEVERIFY
	OP_NOP4                = 0xb3
	OP_NOP5                = 0xb4
	OP_NOP6                = 0xb5
	OP_NOP7                = 0xb6
	OP_NOP8                = 0xb7
	OP_NOP9                = 0xb8
	OP_NOP10               = 0xb9
)

// Zerocoin
const (
	OP_ZEROCOINMINT  = 0xc1
	OP_ZEROCOINSPEND = 0xc2
)

// GetOpcodeName returns the name of an opcode
func GetOpcodeName(opcode byte) string {
	switch opcode {
	// Constants
	case OP_0:
		return "OP_0"
	case OP_PUSHDATA1:
		return "OP_PUSHDATA1"
	case OP_PUSHDATA2:
		return "OP_PUSHDATA2"
	case OP_PUSHDATA4:
		return "OP_PUSHDATA4"
	case OP_1NEGATE:
		return "OP_1NEGATE"
	case OP_1:
		return "OP_1"
	case OP_2:
		return "OP_2"
	case OP_16:
		return "OP_16"

	// Flow control
	case OP_NOP:
		return "OP_NOP"
	case OP_IF:
		return "OP_IF"
	case OP_NOTIF:
		return "OP_NOTIF"
	case OP_ELSE:
		return "OP_ELSE"
	case OP_ENDIF:
		return "OP_ENDIF"
	case OP_VERIFY:
		return "OP_VERIFY"
	case OP_RETURN:
		return "OP_RETURN"

	// Stack
	case OP_TOALTSTACK:
		return "OP_TOALTSTACK"
	case OP_FROMALTSTACK:
		return "OP_FROMALTSTACK"
	case OP_2DROP:
		return "OP_2DROP"
	case OP_2DUP:
		return "OP_2DUP"
	case OP_DROP:
		return "OP_DROP"
	case OP_DUP:
		return "OP_DUP"
	case OP_SWAP:
		return "OP_SWAP"

	// Arithmetic
	case OP_1ADD:
		return "OP_1ADD"
	case OP_1SUB:
		return "OP_1SUB"
	case OP_NEGATE:
		return "OP_NEGATE"
	case OP_ABS:
		return "OP_ABS"
	case OP_NOT:
		return "OP_NOT"
	case OP_0NOTEQUAL:
		return "OP_0NOTEQUAL"
	case OP_ADD:
		return "OP_ADD"
	case OP_SUB:
		return "OP_SUB"

	// Crypto
	case OP_RIPEMD160:
		return "OP_RIPEMD160"
	case OP_SHA256:
		return "OP_SHA256"
	case OP_HASH160:
		return "OP_HASH160"
	case OP_HASH256:
		return "OP_HASH256"
	case OP_CHECKSIG:
		return "OP_CHECKSIG"
	case OP_CHECKSIGVERIFY:
		return "OP_CHECKSIGVERIFY"
	case OP_CHECKMULTISIG:
		return "OP_CHECKMULTISIG"
	case OP_CHECKMULTISIGVERIFY:
		return "OP_CHECKMULTISIGVERIFY"

	// Bitwise
	case OP_EQUAL:
		return "OP_EQUAL"
	case OP_EQUALVERIFY:
		return "OP_EQUALVERIFY"

	// Timelock
	case OP_CHECKLOCKTIMEVERIFY:
		return "OP_CHECKLOCKTIMEVERIFY"
	case OP_CHECKSEQUENCEVERIFY:
		return "OP_CHECKSEQUENCEVERIFY"

	// Zerocoin
	case OP_ZEROCOINMINT:
		return "OP_ZEROCOINMINT"
	case OP_ZEROCOINSPEND:
		return "OP_ZEROCOINSPEND"

	default:
		if opcode >= 0x01 && opcode <= 0x4b {
			return "OP_PUSHBYTES"
		}
		if opcode >= OP_2 && opcode <= OP_16 {
			return fmt.Sprintf("OP_%d", opcode-OP_2+2)
		}
		return "OP_UNKNOWN"
	}
}
