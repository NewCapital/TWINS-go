package consensus

import (
	"encoding/binary"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/twins-dev/twins-core/pkg/crypto"
	"github.com/twins-dev/twins-core/pkg/types"
)

// BlockBuilder constructs PoS block templates for staking.
type BlockBuilder struct {
	blockchain BlockchainInterface
	params     *types.ChainParams
	logger     *logrus.Entry
}

// NewBlockBuilder creates a new block builder instance.
func NewBlockBuilder(blockchain BlockchainInterface, params *types.ChainParams, logger *logrus.Logger) *BlockBuilder {
	return &BlockBuilder{
		blockchain: blockchain,
		params:     params,
		logger:     logger.WithField("component", "block_builder"),
	}
}

// CreateStakeBlock creates a complete PoS block from a coinstake transaction.
// The block structure:
// - Header: version, prevHash, merkleRoot, timestamp, bits, nonce=0
// - Transactions[0]: Empty coinbase (required placeholder)
// - Transactions[1]: Coinstake transaction (the proof of stake)
// - Transactions[2+]: Optional mempool transactions
// - Signature: DER-encoded ECDSA signature of block hash (added later)
//
// CRITICAL: bits parameter must be calculated via CalculateNextTarget (difficulty retarget)
// Legacy miner.cpp:152: pblock->nBits = GetNextWorkRequired(pindexPrev, pblock)
func (bb *BlockBuilder) CreateStakeBlock(
	coinstakeTx *types.Transaction,
	prevBlock *types.Block,
	prevHeight uint32,
	timestamp uint32,
	bits uint32, // Difficulty bits from CalculateNextTarget (NOT prevBlock.Header.Bits!)
	mempoolTxs []*types.Transaction,
) (*types.Block, error) {
	if coinstakeTx == nil {
		return nil, fmt.Errorf("coinstake transaction is required")
	}
	if prevBlock == nil {
		return nil, fmt.Errorf("previous block is required")
	}

	// Verify coinstake structure
	if !coinstakeTx.IsCoinStake() {
		return nil, fmt.Errorf("transaction is not a valid coinstake")
	}

	// Create empty coinbase transaction (required for PoS blocks)
	coinbaseTx := bb.createEmptyCoinbase(prevHeight + 1)

	// Assemble transactions
	transactions := make([]*types.Transaction, 0, 2+len(mempoolTxs))
	transactions = append(transactions, coinbaseTx)    // Index 0: coinbase
	transactions = append(transactions, coinstakeTx)   // Index 1: coinstake
	transactions = append(transactions, mempoolTxs...) // Index 2+: mempool txs

	// Calculate merkle root
	merkleRoot := bb.calculateMerkleRoot(transactions)

	// NOTE: bits parameter is now passed in from caller (calculated via CalculateNextTarget)
	// This ensures proper difficulty retarget matching legacy miner.cpp:152

	// Create block header
	header := &types.BlockHeader{
		Version:       prevBlock.Header.Version,
		PrevBlockHash: prevBlock.Header.Hash(),
		MerkleRoot:    merkleRoot,
		Timestamp:     timestamp,
		Bits:          bits,
		Nonce:         0, // PoS blocks have nonce=0
	}

	// Create block (signature will be added by SignBlock)
	block := &types.Block{
		Header:       header,
		Transactions: transactions,
		Signature:    nil, // Must be signed before broadcast
	}

	bb.logger.WithFields(logrus.Fields{
		"height":    prevHeight + 1,
		"prev_hash": prevBlock.Header.Hash().String(),
		"merkle":    merkleRoot.String(),
		"tx_count":  len(transactions),
		"timestamp": timestamp,
	}).Debug("Created stake block template")

	return block, nil
}

// createEmptyCoinbase creates an empty coinbase transaction for PoS blocks.
// PoS blocks require a coinbase at index 0, but it has zero value.
func (bb *BlockBuilder) createEmptyCoinbase(height uint32) *types.Transaction {
	// Create coinbase input (null prevout)
	coinbaseInput := &types.TxInput{
		PreviousOutput: types.Outpoint{
			Hash:  types.Hash{}, // Zero hash
			Index: 0xffffffff,   // Max index indicates coinbase
		},
		ScriptSig: bb.createCoinbaseScript(height),
		Sequence:  0xffffffff,
	}

	// Create empty output (zero value)
	coinbaseOutput := &types.TxOutput{
		Value:        0,
		ScriptPubKey: []byte{}, // Empty script
	}

	return &types.Transaction{
		Version:  1,
		Inputs:   []*types.TxInput{coinbaseInput},
		Outputs:  []*types.TxOutput{coinbaseOutput},
		LockTime: 0,
	}
}

// createCoinbaseScript creates the scriptSig for a coinbase transaction.
// Contains the block height (BIP34) and optional extra nonce.
func (bb *BlockBuilder) createCoinbaseScript(height uint32) []byte {
	// BIP34: Coinbase must contain block height
	heightBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(heightBytes, height)

	// Remove leading zeros for compact encoding
	for len(heightBytes) > 1 && heightBytes[len(heightBytes)-1] == 0 {
		heightBytes = heightBytes[:len(heightBytes)-1]
	}

	// Build script: <height_len> <height> <extra_data>
	script := make([]byte, 0, 20)
	script = append(script, byte(len(heightBytes)))
	script = append(script, heightBytes...)

	// Add "TWINS" identifier
	script = append(script, []byte("/TWINS/")...)

	return script
}

// calculateMerkleRoot calculates the merkle root for a list of transactions.
func (bb *BlockBuilder) calculateMerkleRoot(transactions []*types.Transaction) types.Hash {
	if len(transactions) == 0 {
		return types.Hash{}
	}

	// Get transaction hashes
	hashes := make([]types.Hash, len(transactions))
	for i, tx := range transactions {
		hashes[i] = tx.Hash()
	}

	// Build merkle tree
	for len(hashes) > 1 {
		// If odd number, duplicate last hash
		if len(hashes)%2 != 0 {
			hashes = append(hashes, hashes[len(hashes)-1])
		}

		// Combine pairs
		newHashes := make([]types.Hash, len(hashes)/2)
		for i := 0; i < len(hashes); i += 2 {
			newHashes[i/2] = hashPair(hashes[i], hashes[i+1])
		}
		hashes = newHashes
	}

	return hashes[0]
}

// hashPair combines two hashes using double SHA256.
func hashPair(a, b types.Hash) types.Hash {
	combined := make([]byte, 64)
	copy(combined[:32], a[:])
	copy(combined[32:], b[:])
	// types.NewHash performs double SHA256 and returns Hash
	return types.NewHash(combined)
}

// SignBlock signs a PoS block with the staker's private key.
// The signature proves ownership of the stake input.
func (bb *BlockBuilder) SignBlock(block *types.Block, wallet StakingWalletInterface) error {
	if block == nil {
		return fmt.Errorf("block is nil")
	}
	if len(block.Transactions) < 2 {
		return fmt.Errorf("block must have at least coinbase and coinstake")
	}

	coinstake := block.Transactions[1]
	if !coinstake.IsCoinStake() {
		return fmt.Errorf("transaction at index 1 is not a coinstake")
	}

	// Get the address from the coinstake output (output[1] is the stake return)
	if len(coinstake.Outputs) < 2 {
		return fmt.Errorf("coinstake has insufficient outputs")
	}

	// Extract address from stake output script
	stakeOutput := coinstake.Outputs[1]
	address := extractAddressFromScript(stakeOutput.ScriptPubKey)
	if address == "" {
		return fmt.Errorf("cannot extract address from coinstake output")
	}

	// Get block hash to sign
	blockHash := block.Header.Hash()

	// Sign the block hash
	signature, err := wallet.SignMessageBytes(address, blockHash[:])
	if err != nil {
		return fmt.Errorf("failed to sign block: %w", err)
	}

	block.Signature = signature

	bb.logger.WithFields(logrus.Fields{
		"block_hash": blockHash.String(),
		"address":    address,
		"sig_len":    len(signature),
	}).Debug("Block signed successfully")

	return nil
}

// extractAddressFromScript extracts a Base58 address from a scriptPubKey.
// Supports both P2PKH and P2PK scripts (legacy compliance for block signing).
func extractAddressFromScript(script []byte) string {
	// P2PKH: OP_DUP OP_HASH160 <20 bytes> OP_EQUALVERIFY OP_CHECKSIG
	if len(script) == 25 && script[0] == 0x76 && script[1] == 0xa9 && script[2] == 0x14 {
		pubKeyHash := script[3:23]
		// Use mainnet prefix (TODO: make configurable)
		return pubKeyHashToBase58(pubKeyHash, true)
	}

	// P2PK compressed: <33 bytes pubkey> OP_CHECKSIG
	// Legacy: blocksignature.cpp - SignBlock handles P2PK scripts
	if len(script) == 35 && script[0] == 0x21 && script[34] == 0xac {
		pubKey := script[1:34]
		pubKeyHash := crypto.Hash160(pubKey)
		return pubKeyHashToBase58(pubKeyHash, true)
	}

	// P2PK uncompressed: <65 bytes pubkey> OP_CHECKSIG
	if len(script) == 67 && script[0] == 0x41 && script[66] == 0xac {
		pubKey := script[1:66]
		pubKeyHash := crypto.Hash160(pubKey)
		return pubKeyHashToBase58(pubKeyHash, true)
	}

	return ""
}

// pubKeyHashToBase58 converts a public key hash to a Base58Check address.
// Uses crypto.NewAddressFromHash for proper Base58Check encoding.
func pubKeyHashToBase58(pubKeyHash []byte, mainnet bool) string {
	var netID byte = crypto.MainNetPubKeyHashAddrID // 0x49 = 'W' prefix
	if !mainnet {
		netID = crypto.TestNetPubKeyHashAddrID // 0x6F
	}

	addr, err := crypto.NewAddressFromHash(pubKeyHash, netID)
	if err != nil {
		return ""
	}
	return addr.String()
}

// GetBlockReward returns the block reward for a given height.
// This is a wrapper around the validation function.
func (bb *BlockBuilder) GetBlockReward(height uint32) int64 {
	return GetBlockValue(height)
}
