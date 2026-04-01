package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
)

// BlockHeader represents the block header structure
// Layout matches legacy: version, prevBlock, merkleRoot, time, bits, nonce, accumulatorCheckpoint
type BlockHeader struct {
	Version                uint32 // Block version (int32 in legacy)
	PrevBlockHash          Hash   // Hash of previous block
	MerkleRoot             Hash   // Merkle tree root of transactions
	Timestamp              uint32 // Block creation timestamp
	Bits                   uint32 // Difficulty target in compact format
	Nonce                  uint32 // Proof of work nonce
	AccumulatorCheckpoint  Hash   // Zerocoin accumulator checkpoint (version > 3)
}

// Block represents a complete block including header and transactions
// PoS-specific fields are stored at block level, not in header
type Block struct {
	Header       *BlockHeader   // Block header
	Transactions []*Transaction // List of transactions in this block
	// PoS specific fields (not part of header hash)
	Signature     []byte // Block signature (stored separately from header)
	// Genesis override: set this for genesis block to use Quark hash instead of SHA256
	canonicalHash *Hash  // If set, Hash() returns this instead of calculating
	// Cached height (set when block added to chain, not serialized)
	height uint32 // Block height in the blockchain

	// Stake modifier metadata (for PoS consensus)
	stakeModifier          uint64 // Computed stake modifier for this block
	generatedStakeModifier bool   // Whether this block generated a new stake modifier
	stakeEntropyBit        uint8  // Entropy bit (0 or 1) used in modifier calculation

	// PoS checksum chain fields (for stake modifier checksum validation)
	// These fields are computed during block validation and stored for checksum chaining
	stakeModifierChecksum uint32     // Stake modifier checksum for this block
	hashProofOfStake      Hash       // Kernel hash from PoS validation (zero for PoW blocks)
}

// Hash calculates and returns the hash of the block header
// Matches legacy serialization: version, prevBlock, merkleRoot, time, bits, nonce, accumulator (if version > 3)
func (bh *BlockHeader) Hash() Hash {
	var buf bytes.Buffer

	// Serialize header fields in exact legacy order
	binary.Write(&buf, binary.LittleEndian, bh.Version)
	// PrevBlockHash - write as-is (no reversal)
	buf.Write(bh.PrevBlockHash[:])
	// MerkleRoot - write as-is
	buf.Write(bh.MerkleRoot[:])
	binary.Write(&buf, binary.LittleEndian, bh.Timestamp)
	binary.Write(&buf, binary.LittleEndian, bh.Bits)
	binary.Write(&buf, binary.LittleEndian, bh.Nonce)

	// Accumulator checkpoint only included if version > 3 (zerocoin active)
	if bh.Version > 3 {
		buf.Write(bh.AccumulatorCheckpoint[:])
	}

	return NewHash(buf.Bytes())
}

// Hash returns the hash of the block (same as header hash)
// For genesis block, returns the canonical hash if set (to handle Quark hashing)
func (b *Block) Hash() Hash {
	if b.canonicalHash != nil {
		return *b.canonicalHash
	}
	return b.Header.Hash()
}

// SetCanonicalHash sets the canonical hash for this block (used for genesis with Quark)
func (b *Block) SetCanonicalHash(hash Hash) {
	b.canonicalHash = &hash
}

// IsProofOfStake returns true if this block is a proof-of-stake block
// PoS blocks have a coinstake transaction as the second transaction (index 1)
func (b *Block) IsProofOfStake() bool {
	return len(b.Transactions) > 1 && b.Transactions[1].IsCoinStake()
}

// IsProofOfWork returns true if this block is a proof-of-work block
// PoW blocks do not have a coinstake transaction
func (b *Block) IsProofOfWork() bool {
	return !b.IsProofOfStake()
}

// SetHeight sets the cached height for this block
// Called when block is added to chain or loaded from database
func (b *Block) SetHeight(height uint32) {
	b.height = height
}

// Height returns the cached block height
// Returns the height that was set via SetHeight()
// Must be called after block is added to chain or loaded from database
func (b *Block) Height() uint32 {
	return b.height
}

// SetStakeModifier sets the stake modifier for this block
func (b *Block) SetStakeModifier(modifier uint64, generated bool) {
	b.stakeModifier = modifier
	b.generatedStakeModifier = generated
}

// GetStakeModifier returns the stake modifier for this block
func (b *Block) GetStakeModifier() uint64 {
	return b.stakeModifier
}

// GeneratedStakeModifier returns whether this block generated a new stake modifier
func (b *Block) GeneratedStakeModifier() bool {
	return b.generatedStakeModifier
}

// SetStakeEntropyBit sets the entropy bit for this block (0 or 1)
func (b *Block) SetStakeEntropyBit(bit uint8) {
	b.stakeEntropyBit = bit & 1 // Ensure only 0 or 1
}

// GetStakeEntropyBit returns the entropy bit for this block
func (b *Block) GetStakeEntropyBit() uint8 {
	return b.stakeEntropyBit
}

// SetStakeModifierChecksum sets the stake modifier checksum for this block
func (b *Block) SetStakeModifierChecksum(checksum uint32) {
	b.stakeModifierChecksum = checksum
}

// GetStakeModifierChecksum returns the stake modifier checksum for this block
func (b *Block) GetStakeModifierChecksum() uint32 {
	return b.stakeModifierChecksum
}

// SetHashProofOfStake sets the kernel hash (hashProofOfStake) for this block
// This is the hash computed during PoS validation that proves the stake
func (b *Block) SetHashProofOfStake(hash Hash) {
	b.hashProofOfStake = hash
}

// GetHashProofOfStake returns the kernel hash (hashProofOfStake) for this block
// Returns zero hash for PoW blocks
func (b *Block) GetHashProofOfStake() Hash {
	return b.hashProofOfStake
}

// SerializedSize returns the serialized size of the block
func (b *Block) SerializedSize() int {
	size := 80 // Header size (version:4 + prevHash:32 + merkleRoot:32 + time:4 + bits:4 + nonce:4)

	// VarInt size for transaction count
	txCount := len(b.Transactions)
	size += CompactSizeLen(uint64(txCount))

	// Transaction sizes
	for _, tx := range b.Transactions {
		size += tx.SerializedSize()
	}

	// Block signature (PoS blocks)
	if len(b.Signature) > 0 {
		size += CompactSizeLen(uint64(len(b.Signature))) // VarInt for signature length
		size += len(b.Signature)
	}

	return size
}

// CalculateMerkleRoot computes the merkle tree root of all transactions in the block
func (b *Block) CalculateMerkleRoot() Hash {
	return CalculateMerkleRoot(b.Transactions)
}

// CalculateMerkleRoot computes the merkle tree root of a list of transactions
func CalculateMerkleRoot(transactions []*Transaction) Hash {
	if len(transactions) == 0 {
		return ZeroHash
	}

	if len(transactions) == 1 {
		return transactions[0].Hash()
	}

	// Create list of transaction hashes
	hashes := make([]Hash, len(transactions))
	for i, tx := range transactions {
		hashes[i] = tx.Hash()
	}

	// Build merkle tree by repeatedly hashing pairs
	for len(hashes) > 1 {
		var nextLevel []Hash

		for i := 0; i < len(hashes); i += 2 {
			var combined []byte
			combined = append(combined, hashes[i][:]...)

			if i+1 < len(hashes) {
				combined = append(combined, hashes[i+1][:]...)
			} else {
				// If odd number, duplicate the last hash
				combined = append(combined, hashes[i][:]...)
			}

			hash := sha256.Sum256(combined)
			doubleHash := sha256.Sum256(hash[:])
			nextLevel = append(nextLevel, doubleHash)
		}

		hashes = nextLevel
	}

	return hashes[0]
}

// Serialize encodes the block header to bytes
func (bh *BlockHeader) Serialize() ([]byte, error) {
	var buf bytes.Buffer

	if err := binary.Write(&buf, binary.LittleEndian, bh.Version); err != nil {
		return nil, err
	}
	// PrevBlockHash - write as-is (no reversal)
	buf.Write(bh.PrevBlockHash[:])
	// MerkleRoot - write as-is
	buf.Write(bh.MerkleRoot[:])
	if err := binary.Write(&buf, binary.LittleEndian, bh.Timestamp); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, bh.Bits); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, bh.Nonce); err != nil {
		return nil, err
	}

	// Accumulator checkpoint only included if version > 3 (matches legacy)
	if bh.Version > 3 {
		buf.Write(bh.AccumulatorCheckpoint[:])
	}

	return buf.Bytes(), nil
}

// DeserializeBlockHeader decodes a block header from bytes
func DeserializeBlockHeader(data []byte) (*BlockHeader, error) {
	minSize := 4 + 32 + 32 + 4 + 4 + 4 // version + prevHash + merkleRoot + time + bits + nonce
	if len(data) < minSize {
		return nil, bytes.ErrTooLarge
	}

	buf := bytes.NewReader(data)
	bh := &BlockHeader{}

	if err := binary.Read(buf, binary.LittleEndian, &bh.Version); err != nil {
		return nil, err
	}
	// PrevBlockHash - read as-is (no reversal)
	if _, err := buf.Read(bh.PrevBlockHash[:]); err != nil {
		return nil, err
	}
	// MerkleRoot - read as-is
	if _, err := buf.Read(bh.MerkleRoot[:]); err != nil {
		return nil, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &bh.Timestamp); err != nil {
		return nil, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &bh.Bits); err != nil {
		return nil, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &bh.Nonce); err != nil {
		return nil, err
	}

	// Accumulator checkpoint only included if version > 3 (matches legacy)
	if bh.Version > 3 {
		if _, err := buf.Read(bh.AccumulatorCheckpoint[:]); err != nil {
			return nil, err
		}
	}

	return bh, nil
}

// Serialize encodes the complete block to bytes (header + transactions + signature)
// Matches legacy: header, varint tx count, transactions, signature (if PoS)
func (b *Block) Serialize() ([]byte, error) {
	var buf bytes.Buffer

	// Serialize header
	headerBytes, err := b.Header.Serialize()
	if err != nil {
		return nil, err
	}
	buf.Write(headerBytes)

	// Transaction count (Bitcoin compact size / varint)
	if err := WriteCompactSize(&buf, uint64(len(b.Transactions))); err != nil {
		return nil, err
	}

	// Serialize each transaction
	for _, tx := range b.Transactions {
		txBytes, err := tx.Serialize()
		if err != nil {
			return nil, err
		}
		buf.Write(txBytes)
	}

	// Block signature (only for PoS blocks: vtx.size() > 1 && vtx[1].IsCoinStake())
	// This matches legacy primitives/block.h:117-118
	if len(b.Transactions) > 1 && b.Transactions[1].IsCoinStake() {
		// Write signature as vector<unsigned char> (compact size + data)
		if err := WriteCompactSize(&buf, uint64(len(b.Signature))); err != nil {
			return nil, err
		}
		if len(b.Signature) > 0 {
			buf.Write(b.Signature)
		}
	}

	return buf.Bytes(), nil
}

// DeserializeBlock decodes a complete block from bytes
// Matches legacy: header, varint tx count, transactions, signature (if PoS)
func DeserializeBlock(data []byte) (*Block, error) {
	buf := bytes.NewReader(data)

	// Deserialize header
	header, err := DeserializeBlockHeader(data)
	if err != nil {
		return nil, err
	}

	// Skip past header bytes
	headerSize := header.SerializeSize()
	buf.Seek(int64(headerSize), 0)

	block := &Block{Header: header}

	// Transaction count (Bitcoin compact size / varint)
	txCount, err := ReadCompactSize(buf)
	if err != nil {
		return nil, err
	}

	// Deserialize each transaction
	block.Transactions = make([]*Transaction, txCount)
	for i := uint64(0); i < txCount; i++ {
		// Read remaining bytes for transaction deserialization
		remaining := make([]byte, buf.Len())
		if _, err := buf.Read(remaining); err != nil {
			return nil, err
		}

		tx, err := DeserializeTransaction(remaining)
		if err != nil {
			return nil, err
		}
		block.Transactions[i] = tx

		// Calculate how many bytes the transaction used
		txBytes, _ := tx.Serialize()
		// Reset buffer to position after this transaction
		buf = bytes.NewReader(remaining[len(txBytes):])
	}

	// Block signature (only for PoS blocks: vtx.size() > 1 && vtx[1].IsCoinStake())
	// This matches legacy primitives/block.h:117-118
	if txCount > 1 && block.Transactions[1].IsCoinStake() {
		sigLen, err := ReadCompactSize(buf)
		if err != nil {
			// No signature present, not an error
			return block, nil
		}
		if sigLen > 0 {
			block.Signature = make([]byte, sigLen)
			if _, err := buf.Read(block.Signature); err != nil {
				return nil, err
			}
		}
	}

	return block, nil
}

// SerializeSize returns the serialized size of the block header
func (bh *BlockHeader) SerializeSize() int {
	size := 4 + // Version
		32 + // PrevBlockHash
		32 + // MerkleRoot
		4 + // Timestamp
		4 + // Bits
		4 // Nonce

	// Accumulator checkpoint only if version > 3
	if bh.Version > 3 {
		size += 32 // AccumulatorCheckpoint
	}

	return size
}

// SerializeSize returns the approximate serialized size of the entire block
func (b *Block) SerializeSize() int {
	size := b.Header.SerializeSize()

	// Add size for transaction count (varint, approximate as 4 bytes)
	size += 4

	// Add size for all transactions
	for _, tx := range b.Transactions {
		size += tx.SerializeSize()
	}

	return size
}
