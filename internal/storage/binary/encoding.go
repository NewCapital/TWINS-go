package binary

import (
	"encoding/binary"
	"fmt"

	"github.com/twins-dev/twins-core/pkg/types"
)

// EncodeCompactBlock encodes a compact block to bytes
func EncodeCompactBlock(block *CompactBlock) ([]byte, error) {
	// Calculate size:
	// Height(4) + Version(4) + PrevBlock(32) + Merkle(32) + Timestamp(4) +
	// Bits(4) + Nonce(4) + StakeMod(8) + StakeTime(4) + TxCount(4) + TxHashes(32*n) +
	// SigLen(4) + Signature(variable)
	size := 104 + (32 * len(block.TxHashes)) + len(block.Signature)
	data := make([]byte, size)

	offset := 0

	// Encode fixed fields
	binary.LittleEndian.PutUint32(data[offset:], block.Height)
	offset += 4

	binary.LittleEndian.PutUint32(data[offset:], block.Version)
	offset += 4

	copy(data[offset:], block.PrevBlock[:])
	offset += 32

	copy(data[offset:], block.Merkle[:])
	offset += 32

	binary.LittleEndian.PutUint32(data[offset:], block.Timestamp)
	offset += 4

	binary.LittleEndian.PutUint32(data[offset:], block.Bits)
	offset += 4

	binary.LittleEndian.PutUint32(data[offset:], block.Nonce)
	offset += 4

	binary.LittleEndian.PutUint64(data[offset:], block.StakeMod)
	offset += 8

	binary.LittleEndian.PutUint32(data[offset:], block.StakeTime)
	offset += 4

	binary.LittleEndian.PutUint32(data[offset:], block.TxCount)
	offset += 4

	// Encode transaction hashes
	for _, hash := range block.TxHashes {
		copy(data[offset:], hash[:])
		offset += 32
	}

	// Encode signature (length + data)
	binary.LittleEndian.PutUint32(data[offset:], uint32(len(block.Signature)))
	offset += 4
	if len(block.Signature) > 0 {
		copy(data[offset:], block.Signature)
	}

	return data, nil
}

// DecodeCompactBlock decodes bytes to a compact block
func DecodeCompactBlock(data []byte) (*CompactBlock, error) {
	if len(data) < 100 {
		return nil, fmt.Errorf("invalid compact block data: too short")
	}

	block := &CompactBlock{}
	offset := 0

	// Decode fixed fields
	block.Height = binary.LittleEndian.Uint32(data[offset:])
	offset += 4

	block.Version = binary.LittleEndian.Uint32(data[offset:])
	offset += 4

	copy(block.PrevBlock[:], data[offset:offset+32])
	offset += 32

	copy(block.Merkle[:], data[offset:offset+32])
	offset += 32

	block.Timestamp = binary.LittleEndian.Uint32(data[offset:])
	offset += 4

	block.Bits = binary.LittleEndian.Uint32(data[offset:])
	offset += 4

	block.Nonce = binary.LittleEndian.Uint32(data[offset:])
	offset += 4

	block.StakeMod = binary.LittleEndian.Uint64(data[offset:])
	offset += 8

	block.StakeTime = binary.LittleEndian.Uint32(data[offset:])
	offset += 4

	block.TxCount = binary.LittleEndian.Uint32(data[offset:])
	offset += 4

	// Decode transaction hashes
	block.TxHashes = make([]types.Hash, block.TxCount)
	for i := uint32(0); i < block.TxCount; i++ {
		if offset+32 > len(data) {
			return nil, fmt.Errorf("invalid compact block data: truncated tx hashes")
		}
		copy(block.TxHashes[i][:], data[offset:offset+32])
		offset += 32
	}

	// Decode signature (backwards compatible: old data may not have signature)
	if offset+4 <= len(data) {
		sigLen := binary.LittleEndian.Uint32(data[offset:])
		offset += 4
		if sigLen > 0 {
			if offset+int(sigLen) > len(data) {
				return nil, fmt.Errorf("invalid compact block data: truncated signature")
			}
			block.Signature = make([]byte, sigLen)
			copy(block.Signature, data[offset:offset+int(sigLen)])
		}
	}

	return block, nil
}

// EncodeTransactionData encodes transaction data with location info
func EncodeTransactionData(txData *TransactionData) ([]byte, error) {
	// Encode transaction to bytes using Bitcoin protocol serialization
	txBytes, err := txData.TxData.Serialize()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize transaction: %w", err)
	}

	// Size: BlockHash(32) + Height(4) + TxIndex(4) + TxDataLen(4) + TxData
	size := 44 + len(txBytes)
	data := make([]byte, size)

	offset := 0

	// Encode location info
	copy(data[offset:], txData.BlockHash[:])
	offset += 32

	binary.LittleEndian.PutUint32(data[offset:], txData.Height)
	offset += 4

	binary.LittleEndian.PutUint32(data[offset:], txData.TxIndex)
	offset += 4

	// Encode transaction data length and data
	binary.LittleEndian.PutUint32(data[offset:], uint32(len(txBytes)))
	offset += 4

	copy(data[offset:], txBytes)

	return data, nil
}

// DecodeTransactionData decodes bytes to transaction data
func DecodeTransactionData(data []byte) (*TransactionData, error) {
	if len(data) < 44 {
		return nil, fmt.Errorf("invalid transaction data: too short")
	}

	txData := &TransactionData{}
	offset := 0

	// Decode location info
	copy(txData.BlockHash[:], data[offset:offset+32])
	offset += 32

	txData.Height = binary.LittleEndian.Uint32(data[offset:])
	offset += 4

	txData.TxIndex = binary.LittleEndian.Uint32(data[offset:])
	offset += 4

	// Decode transaction data length
	txLen := binary.LittleEndian.Uint32(data[offset:])
	offset += 4

	if len(data) != int(44+txLen) {
		return nil, fmt.Errorf("invalid transaction data: size mismatch")
	}

	// Decode transaction using Bitcoin protocol deserialization
	tx, err := types.DeserializeTransaction(data[offset : offset+int(txLen)])
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize transaction: %w", err)
	}
	txData.TxData = tx

	return txData, nil
}

// EncodeAddressHistoryEntry encodes an address history entry
func EncodeAddressHistoryEntry(entry *AddressHistoryEntry) ([]byte, error) {
	// Size: IsInput(1) + Value(8) + BlockHash(32) = 41 bytes
	data := make([]byte, 41)

	// Encode IsInput as byte
	if entry.IsInput {
		data[0] = 1
	} else {
		data[0] = 0
	}

	// Encode value
	binary.LittleEndian.PutUint64(data[1:9], entry.Value)

	// Encode block hash
	copy(data[9:41], entry.BlockHash[:])

	return data, nil
}

// DecodeAddressHistoryEntry decodes bytes to an address history entry
func DecodeAddressHistoryEntry(data []byte) (*AddressHistoryEntry, error) {
	if len(data) != 41 {
		return nil, fmt.Errorf("invalid history entry data: wrong size")
	}

	entry := &AddressHistoryEntry{
		IsInput: data[0] == 1,
		Value:   binary.LittleEndian.Uint64(data[1:9]),
	}

	copy(entry.BlockHash[:], data[9:41])

	return entry, nil
}

// EncodeUTXOData encodes UTXO data with spending tracking
// Format: [height:4][spendingHeight:4][value:8][isCoinbase:1][scriptHash:20][scriptLen:2][script:var][spendingTxHash:32]
func EncodeUTXOData(utxo *UTXOData) ([]byte, error) {
	// Size: Height(4) + SpendingHeight(4) + Value(8) + IsCoinbase(1) +
	//       ScriptHash(20) + ScriptLen(2) + Script + SpendingTxHash(32)
	size := 71 + len(utxo.Script)
	data := make([]byte, size)

	offset := 0

	// Encode creation height
	binary.LittleEndian.PutUint32(data[offset:], utxo.Height)
	offset += 4

	// Encode spending height (0 = unspent)
	binary.LittleEndian.PutUint32(data[offset:], utxo.SpendingHeight)
	offset += 4

	// Encode value
	binary.LittleEndian.PutUint64(data[offset:], utxo.Value)
	offset += 8

	// Encode IsCoinbase flag (consensus-critical)
	if utxo.IsCoinbase {
		data[offset] = 1
	} else {
		data[offset] = 0
	}
	offset++

	// Encode script hash
	copy(data[offset:], utxo.ScriptHash[:])
	offset += 20

	// Encode script length (uint16) and script
	binary.LittleEndian.PutUint16(data[offset:], uint16(len(utxo.Script)))
	offset += 2

	copy(data[offset:], utxo.Script)
	offset += len(utxo.Script)

	// Encode spending tx hash (empty if unspent)
	copy(data[offset:], utxo.SpendingTxHash[:])

	return data, nil
}

// DecodeUTXOData decodes bytes to UTXO data with spending tracking
// Format: [height:4][spendingHeight:4][value:8][isCoinbase:1][scriptHash:20][scriptLen:2][script:var][spendingTxHash:32]
func DecodeUTXOData(data []byte) (*UTXOData, error) {
	// Minimum size: 4+4+8+1+20+2+0+32 = 71 bytes (empty script)
	if len(data) < 71 {
		return nil, fmt.Errorf("invalid UTXO data: too short (%d bytes)", len(data))
	}

	utxo := &UTXOData{}
	offset := 0

	// Decode creation height
	utxo.Height = binary.LittleEndian.Uint32(data[offset:])
	offset += 4

	// Decode spending height (0 = unspent)
	utxo.SpendingHeight = binary.LittleEndian.Uint32(data[offset:])
	offset += 4

	// Decode value
	utxo.Value = binary.LittleEndian.Uint64(data[offset:])
	offset += 8

	// Decode IsCoinbase flag (consensus-critical)
	isCoinbaseVal := data[offset]
	if isCoinbaseVal != 0 && isCoinbaseVal != 1 {
		return nil, fmt.Errorf("invalid IsCoinbase flag value: %d", isCoinbaseVal)
	}
	utxo.IsCoinbase = isCoinbaseVal == 1
	offset++

	// Decode script hash
	copy(utxo.ScriptHash[:], data[offset:offset+20])
	offset += 20

	// Decode script length (uint16)
	scriptLen := binary.LittleEndian.Uint16(data[offset:])
	offset += 2

	// Validate total size: 71 + scriptLen
	expectedSize := 71 + int(scriptLen)
	if len(data) != expectedSize {
		return nil, fmt.Errorf("invalid UTXO data: size mismatch (got %d, expected %d)", len(data), expectedSize)
	}

	// Decode script
	utxo.Script = make([]byte, scriptLen)
	copy(utxo.Script, data[offset:offset+int(scriptLen)])
	offset += int(scriptLen)

	// Decode spending tx hash
	copy(utxo.SpendingTxHash[:], data[offset:offset+32])

	return utxo, nil
}

