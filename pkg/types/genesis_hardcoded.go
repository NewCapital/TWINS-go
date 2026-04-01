package types

import (
	"encoding/hex"
	"fmt"
)

// HardcodedGenesisBlock returns the exact hardcoded genesis block for mainnet
// These values MUST match the legacy C++ implementation exactly
func HardcodedGenesisBlock() *Block {
	// Create the genesis transaction exactly as it appears in the legacy chain
	// This is the coinbase transaction with hash 4271a3d993d6157f960de646ce8dfad07989dfd0703064f8056d1a7287283d06
	genesisTx := &Transaction{
		Version:  1,
		LockTime: 0,
		Inputs: []*TxInput{
			{
				PreviousOutput: Outpoint{
					Hash:  ZeroHash,
					Index: 0xffffffff, // Coinbase indicator
				},
				// ScriptSig from legacy: contains timestamp message "BBC 2018/12/31 Global markets in worst year since 2008"
				ScriptSig: hexToBytes("04ffff001d01043642424320323031382f31322f333120476c6f62616c206d61726b65747320696e20776f72737420796561722073696e63652032303038"),
				Sequence:  0xffffffff,
			},
		},
		Outputs: []*TxOutput{
			{
				Value: 50 * 100000000, // 50 TWINS
				// ScriptPubKey from legacy: public key with OP_CHECKSIG
				ScriptPubKey: hexToBytes("4104c2851936b2196beb85e7eca91697884918bc6deacd4ca49b52418d376a092913bde42bc868178c0ed436c184259edd0bf2a3ff32388facd6d6332e8de31c9121ac"),
			},
		},
	}

	// Set the hardcoded transaction hash
	txHash := MustParseHash("4271a3d993d6157f960de646ce8dfad07989dfd0703064f8056d1a7287283d06")
	genesisTx.SetCanonicalHash(txHash)

	// Create the genesis block with exact values from the legacy chain
	genesisBlock := &Block{
		Header: &BlockHeader{
			Version:       1,
			PrevBlockHash: ZeroHash, // Genesis has no previous block
			// Hardcoded merkle root from the legacy chain
			MerkleRoot:            MustParseHash("4271a3d993d6157f960de646ce8dfad07989dfd0703064f8056d1a7287283d06"),
			Timestamp:             1546790318, // 2019-01-06 16:38:38 UTC
			Bits:                  0x1e0ffff0, // Initial difficulty
			Nonce:                 348223,     // Genesis nonce
			AccumulatorCheckpoint: ZeroHash,   // Version 1, no accumulator
		},
		Transactions: []*Transaction{genesisTx},
		Signature:    []byte{}, // Genesis has no signature
	}

	// Set the canonical hash (Quark hash) for the genesis block
	// This is the hash that will be used everywhere in the protocol
	genesisHash := MustParseHash("0000071cf2d95aec5ba4818418756c93cb12cd627191710e8969f2f35c3530de")
	genesisBlock.SetCanonicalHash(genesisHash)

	return genesisBlock
}

// HardcodedMainnetGenesis returns the hardcoded genesis block for mainnet
func HardcodedMainnetGenesis() *Block {
	return HardcodedGenesisBlock()
}

// hexToBytes converts a hex string to bytes, panicking on error
func hexToBytes(hexStr string) []byte {
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		panic(fmt.Sprintf("invalid hex string: %v", err))
	}
	return bytes
}