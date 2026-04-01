package binary

import (
	"encoding/binary"

	"github.com/twins-dev/twins-core/pkg/crypto"
	"github.com/twins-dev/twins-core/pkg/types"
)

// Storage prefixes - single byte for each data type
// Simplified from complex composite keys to single-purpose prefixes
const (
	// Core Data Storage
	PrefixBlock          byte = 0x01 // Blocks (compact, headers + tx hashes only)
	PrefixHeightToHash   byte = 0x02 // Height to Hash Index
	PrefixHashToHeight   byte = 0x03 // Hash to Height Index
	PrefixTransaction    byte = 0x04 // Transactions (full data)
	PrefixAddressHistory byte = 0x05 // Address History Index (chronological)
	PrefixAddressUTXO    byte = 0x06 // Address UTXOs for Balance
	PrefixUTXOExist      byte = 0x07 // UTXO Existence Check
	PrefixStakeModifier  byte = 0x08 // Stake Modifiers (PoS)
	// Gap for future use: 0x09-0x0E
	PrefixChainState     byte = 0x0F // Chain State
	PrefixMempoolTx      byte = 0x11 // Mempool Transactions (not in blocks yet)
	PrefixInvalidBlock   byte = 0x12 // Invalid blocks set
	PrefixDynamicCheckpoint byte = 0x13 // Dynamic checkpoints added via RPC
	PrefixBlockPoSMetadata  byte = 0x14 // PoS metadata (checksum + proofHash) for checksum chaining
	PrefixMoneySupply       byte = 0x15 // Money supply at each height
	// Gap for future use: 0x16-0xFE
	PrefixSchemaVersion byte = 0xFF // Schema Version
)

// Schema version for future migrations
const CurrentSchemaVersion uint32 = 1

// BlockKey generates a key for storing block data (compact format)
// Format: [0x01][blockhash:32] -> compact block data
func BlockKey(hash types.Hash) []byte {
	key := make([]byte, 33)
	key[0] = PrefixBlock
	copy(key[1:], hash[:])
	return key
}

// StakeModifierKey generates a key for storing stake modifiers
// Format: [0x08][blockhash:32] -> modifier:8
func StakeModifierKey(blockHash types.Hash) []byte {
	key := make([]byte, 33)
	key[0] = PrefixStakeModifier
	copy(key[1:], blockHash[:])
	return key
}

// HeightToHashKey generates a key for height to hash index
// Format: [0x02][height:4] -> blockhash:32
func HeightToHashKey(height uint32) []byte {
	key := make([]byte, 5)
	key[0] = PrefixHeightToHash
	binary.LittleEndian.PutUint32(key[1:], height)
	return key
}

// HashToHeightKey generates a key for hash to height index
// Format: [0x03][blockhash:32] -> height:4
func HashToHeightKey(hash types.Hash) []byte {
	key := make([]byte, 33)
	key[0] = PrefixHashToHeight
	copy(key[1:], hash[:])
	return key
}

// TransactionKey generates a key for storing transaction data
// Format: [0x04][txhash:32] -> transaction data with location info
func TransactionKey(hash types.Hash) []byte {
	key := make([]byte, 33)
	key[0] = PrefixTransaction
	copy(key[1:], hash[:])
	return key
}

// MempoolTransactionKey generates a key for storing mempool transaction data
// Format: [0x11][txhash:32] -> transaction data (no block location)
func MempoolTransactionKey(hash types.Hash) []byte {
	key := make([]byte, 33)
	key[0] = PrefixMempoolTx
	copy(key[1:], hash[:])
	return key
}

// AddressHistoryKey generates a key for address history index
// Format: [0x05][scripthash:20][height:4][txhash:32][index:2] -> history entry
// This allows chronological ordering by height for address history queries
func AddressHistoryKey(scriptHash [20]byte, height uint32, txHash types.Hash, index uint16) []byte {
	key := make([]byte, 59)
	key[0] = PrefixAddressHistory
	copy(key[1:21], scriptHash[:])
	binary.LittleEndian.PutUint32(key[21:25], height)
	copy(key[25:57], txHash[:])
	binary.LittleEndian.PutUint16(key[57:], index)
	return key
}

// AddressHistoryPrefix generates a prefix for scanning address history
// Format: [0x05][scripthash:20] - returns all history for an address
func AddressHistoryPrefix(scriptHash [20]byte) []byte {
	key := make([]byte, 21)
	key[0] = PrefixAddressHistory
	copy(key[1:], scriptHash[:])
	return key
}

// AddressUTXOKey generates a key for address UTXO tracking
// Format: [0x06][scripthash:20][txhash:32][index:4] -> UTXO value
// Used for quick balance calculation by summing all values
func AddressUTXOKey(scriptHash [20]byte, txHash types.Hash, index uint32) []byte {
	key := make([]byte, 57)
	key[0] = PrefixAddressUTXO
	copy(key[1:21], scriptHash[:])
	copy(key[21:53], txHash[:])
	binary.LittleEndian.PutUint32(key[53:], index)
	return key
}

// AddressUTXOPrefix generates a prefix for scanning address UTXOs
// Format: [0x06][scripthash:20] - returns all UTXOs for an address
func AddressUTXOPrefix(scriptHash [20]byte) []byte {
	key := make([]byte, 21)
	key[0] = PrefixAddressUTXO
	copy(key[1:], scriptHash[:])
	return key
}

// UTXOExistKey generates a key for UTXO existence checking
// Format: [0x07][txhash:32][index:4] -> UTXO data
// Presence of key indicates UTXO exists, absence means spent
func UTXOExistKey(txHash types.Hash, index uint32) []byte {
	key := make([]byte, 37)
	key[0] = PrefixUTXOExist
	copy(key[1:33], txHash[:])
	binary.LittleEndian.PutUint32(key[33:], index)
	return key
}

// ChainStateKey generates a key for chain state storage
// Format: [0x0F] -> chain state data (height + tip)
func ChainStateKey() []byte {
	return []byte{PrefixChainState}
}

// InvalidBlockKey generates a key for invalid block tracking
// Format: [0x12][blockhash:32] -> timestamp:8
func InvalidBlockKey(hash types.Hash) []byte {
	key := make([]byte, 33)
	key[0] = PrefixInvalidBlock
	copy(key[1:], hash[:])
	return key
}

// DynamicCheckpointKey generates a key for dynamic checkpoint storage
// Format: [0x13][height:4] -> blockhash:32
func DynamicCheckpointKey(height uint32) []byte {
	key := make([]byte, 5)
	key[0] = PrefixDynamicCheckpoint
	binary.LittleEndian.PutUint32(key[1:], height)
	return key
}

// MoneySupplyKey generates a key for storing money supply at a height
// Format: [0x15][height:4] -> moneysupply:8 (int64 in satoshis)
func MoneySupplyKey(height uint32) []byte {
	key := make([]byte, 5)
	key[0] = PrefixMoneySupply
	binary.LittleEndian.PutUint32(key[1:], height)
	return key
}

// BlockPoSMetadataKey generates a key for storing PoS checksum chain metadata
// Format: [0x14][blockhash:32] -> checksum:4 + proofHash:32 (36 bytes)
func BlockPoSMetadataKey(blockHash types.Hash) []byte {
	key := make([]byte, 33)
	key[0] = PrefixBlockPoSMetadata
	copy(key[1:], blockHash[:])
	return key
}

// SchemaVersionKey generates a key for schema version
// Format: [0xFF][0x01] -> version:4
func SchemaVersionKey() []byte {
	return []byte{PrefixSchemaVersion, 0x01}
}

// CompactBlock represents block data in compact format
// Only stores header and transaction hashes, not full transactions
type CompactBlock struct {
	Height    uint32       // 4 bytes
	Version   uint32       // 4 bytes
	PrevBlock types.Hash   // 32 bytes
	Merkle    types.Hash   // 32 bytes
	Timestamp uint32       // 4 bytes
	Bits      uint32       // 4 bytes (difficulty)
	Nonce     uint32       // 4 bytes
	StakeMod  uint64       // 8 bytes (PoS)
	StakeTime uint32       // 4 bytes (PoS)
	TxCount   uint32       // 4 bytes
	TxHashes  []types.Hash // 32 bytes each
	Signature []byte       // variable length (PoS block signature)
}

// TransactionData represents transaction data with location info
type TransactionData struct {
	BlockHash types.Hash         // 32 bytes - for block lookup
	Height    uint32             // 4 bytes - for quick reference
	TxIndex   uint32             // 4 bytes - position in block
	TxData    *types.Transaction // variable - full transaction
}

// AddressHistoryEntry represents an address history entry
type AddressHistoryEntry struct {
	IsInput   bool       // 1 byte - true for inputs, false for outputs
	Value     uint64     // 8 bytes - amount changed by the transaction
	BlockHash types.Hash // 32 bytes - block reference
}

// UTXOData represents UTXO data with spending tracking
type UTXOData struct {
	// Creation metadata
	Value      uint64   // 8 bytes - amount in satoshis
	ScriptHash [20]byte // 20 bytes - for address verification
	Height     uint32   // 4 bytes - block height where created (creationHeight)
	IsCoinbase bool     // 1 byte - consensus-critical for PoS validation
	Script     []byte   // variable - full script for spending validation

	// Spending metadata (0 = unspent)
	SpendingHeight uint32     // 4 bytes - 0=unspent, N=spent at block N
	SpendingTxHash types.Hash // 32 bytes - empty=unspent, hash of spending tx
}

// IsUnspent returns true if this UTXO has not been spent
func (d *UTXOData) IsUnspent() bool {
	return d.SpendingHeight == 0
}

// IsSpent returns true if this UTXO has been spent
func (d *UTXOData) IsSpent() bool {
	return d.SpendingHeight != 0
}

// Script type constants for address indexing
const (
	ScriptTypeP2PKH   = iota // Pay to Public Key Hash
	ScriptTypeP2SH           // Pay to Script Hash
	ScriptTypeP2PK           // Pay to Public Key (legacy format)
	ScriptTypeUnknown        // Unknown/unsupported script type
)

// AnalyzeScript analyzes a script and returns its type and extracted address hash
// This is used for address indexing and UTXO tracking
// Supports P2PKH, P2SH, and P2PK (legacy) script types
func AnalyzeScript(script []byte) (scriptType int, scriptHash [20]byte) {
	// P2PKH: OP_DUP OP_HASH160 <20 bytes> OP_EQUALVERIFY OP_CHECKSIG
	// Format: 0x76 0xa9 0x14 <20-byte hash> 0x88 0xac
	// Total: 25 bytes
	if len(script) == 25 &&
		script[0] == 0x76 && script[1] == 0xa9 && script[2] == 0x14 &&
		script[23] == 0x88 && script[24] == 0xac {
		copy(scriptHash[:], script[3:23])
		return ScriptTypeP2PKH, scriptHash
	}

	// P2SH: OP_HASH160 <20 bytes> OP_EQUAL
	// Format: 0xa9 0x14 <20-byte hash> 0x87
	// Total: 23 bytes
	if len(script) == 23 &&
		script[0] == 0xa9 && script[1] == 0x14 && script[22] == 0x87 {
		copy(scriptHash[:], script[2:22])
		return ScriptTypeP2SH, scriptHash
	}

	// P2PK: <pubkey> OP_CHECKSIG (legacy format from early Bitcoin/TWINS blocks)
	// Compressed pubkey: 0x21 <33 bytes> 0xac (total 35 bytes)
	// Uncompressed pubkey: 0x41 <65 bytes> 0xac (total 67 bytes)
	if len(script) == 35 || len(script) == 67 {
		// Check for OP_CHECKSIG at the end
		if script[len(script)-1] != 0xac {
			return ScriptTypeUnknown, scriptHash
		}

		var pubKey []byte

		if len(script) == 35 {
			// Compressed pubkey (33 bytes)
			// First byte is length prefix (0x21 = 33 bytes)
			if script[0] != 0x21 {
				return ScriptTypeUnknown, scriptHash
			}
			// Second byte must be valid compressed pubkey prefix (0x02 or 0x03)
			if script[1] != 0x02 && script[1] != 0x03 {
				return ScriptTypeUnknown, scriptHash
			}
			pubKey = script[1:34]
		} else if len(script) == 67 {
			// Uncompressed pubkey (65 bytes)
			// First byte is length prefix (0x41 = 65 bytes)
			if script[0] != 0x41 {
				return ScriptTypeUnknown, scriptHash
			}
			// Second byte must be valid uncompressed pubkey prefix (0x04)
			if script[1] != 0x04 {
				return ScriptTypeUnknown, scriptHash
			}
			pubKey = script[1:66]
		} else {
			return ScriptTypeUnknown, scriptHash
		}

		// Validate that the public key is a valid secp256k1 point before hashing
		// This prevents indexing invalid P2PK scripts that can never be spent
		if !crypto.IsValidPublicKey(pubKey) {
			return ScriptTypeUnknown, scriptHash
		}

		// Calculate Hash160 of the public key to get the address hash
		// P2PK outputs are converted to P2PKH-equivalent addresses for indexing
		hash := crypto.Hash160(pubKey)
		copy(scriptHash[:], hash)
		return ScriptTypeP2PK, scriptHash
	}

	// Unknown script type
	return ScriptTypeUnknown, scriptHash
}

// ScriptHashToAddressBinary converts script hash to 21-byte binary address format
// Returns: [netID(1)][hash160(20)]
// This is used for AddressHistory indexing which requires network-aware addresses
//
// CRITICAL: Caller MUST provide correct isTestNet value matching the blockchain network.
// Providing wrong network flag will cause address indexing corruption where:
// - Mainnet blocks will be indexed under testnet addresses (or vice versa)
// - Balance queries will return incorrect results
// - Address history will show transactions from wrong network
//
// Recommended usage:
//   - Blockchain layer: pass bc.config.IsTestNet()
//   - Wallet layer: pass wallet network config (NOT hardcoded false)
//   - Storage layer: pass stored network configuration
func ScriptHashToAddressBinary(scriptType int, scriptHash [20]byte, isTestNet bool) []byte {
	// Return nil for unknown scripts or empty hash
	if scriptType == ScriptTypeUnknown || scriptHash == [20]byte{} {
		return nil
	}

	// Determine network prefix based on script type
	var prefix byte
	switch scriptType {
	case ScriptTypeP2PKH, ScriptTypeP2PK:
		// P2PKH and P2PK use the same address prefix (both are pubkey-based)
		if isTestNet {
			prefix = crypto.TestNetPubKeyHashAddrID // 0x6F - m.../n... addresses
		} else {
			prefix = crypto.MainNetPubKeyHashAddrID // 0x49 - W... addresses
		}
	case ScriptTypeP2SH:
		if isTestNet {
			prefix = crypto.TestNetScriptHashAddrID // 0xC4 - 2... addresses
		} else {
			prefix = crypto.MainNetScriptHashAddrID // 0x53 - a... addresses
		}
	default:
		return nil
	}

	// Create 21-byte binary address: netID (1 byte) + hash160 (20 bytes)
	addressBinary := make([]byte, 21)
	addressBinary[0] = prefix
	copy(addressBinary[1:], scriptHash[:])

	return addressBinary
}
