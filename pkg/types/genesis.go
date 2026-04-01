package types

import (
	"encoding/hex"
)

// GenesisBlock creates the genesis block for a given network
func GenesisBlock(params *ChainParams) *Block {
	if params == nil {
		params = MainnetParams()
	}

	// Create genesis transaction (coinbase) - MUST match legacy C++ exactly
	// Legacy: txNew.vin[0].scriptSig = CScript() << 486604799 << CScriptNum(4) << vector<unsigned char>(pszTimestamp)
	// The scriptSig format: [compact bits] [extra nonce] [timestamp string]
	pszTimestamp := "BBC 2018/12/31 Global markets in worst year since 2008"

	// Build scriptSig to match legacy C++ format EXACTLY
	// CScript() << 486604799 << CScriptNum(4) << vector<unsigned char>(pszTimestamp)
	scriptSig := make([]byte, 0, 100)

	// First push: 486604799 (0x1d00ffff) as CScriptNum
	// CScriptNum::serialize produces little-endian bytes with sign handling
	// 486604799 = 0x1d00ffff -> bytes: ff ff 00 1d (little-endian, no sign extension needed)
	scriptSig = append(scriptSig, 0x04)             // Push 4 bytes
	scriptSig = append(scriptSig, 0xff, 0xff, 0x00, 0x1d) // 486604799 in CScriptNum format

	// Second push: CScriptNum(4) - the number 4 as a script number
	scriptSig = append(scriptSig, 0x01) // Push 1 byte
	scriptSig = append(scriptSig, 0x04) // The value 4

	// Third push: timestamp string as raw bytes
	scriptSig = append(scriptSig, byte(len(pszTimestamp))) // Push N bytes
	scriptSig = append(scriptSig, []byte(pszTimestamp)...)

	// Genesis output pubkey from legacy (raw pubkey, not address!)
	// Legacy: ParseHex("04c2851936b2196beb85e7eca91697884918bc6deacd4ca49b52418d376a092913bde42bc868178c0ed436c184259edd0bf2a3ff32388facd6d6332e8de31c9121")
	genesisPubKey, _ := hex.DecodeString("04c2851936b2196beb85e7eca91697884918bc6deacd4ca49b52418d376a092913bde42bc868178c0ed436c184259edd0bf2a3ff32388facd6d6332e8de31c9121")

	// Build scriptPubKey: <pubkey> OP_CHECKSIG
	scriptPubKey := make([]byte, 0, len(genesisPubKey)+2)
	scriptPubKey = append(scriptPubKey, 0x41) // OP_PUSHDATA 65 bytes
	scriptPubKey = append(scriptPubKey, genesisPubKey...)
	scriptPubKey = append(scriptPubKey, 0xac) // OP_CHECKSIG

	genesisTx := &Transaction{
		Version:  1,
		Inputs:   []*TxInput{},
		Outputs:  []*TxOutput{},
		LockTime: 0,
	}

	// Add coinbase input
	genesisTx.Inputs = append(genesisTx.Inputs, &TxInput{
		PreviousOutput: Outpoint{
			Hash:  ZeroHash,
			Index: 0xffffffff, // Coinbase indicator
		},
		ScriptSig: scriptSig,
		Sequence:  0xffffffff,
	})

	// Add genesis output
	genesisTx.Outputs = append(genesisTx.Outputs, &TxOutput{
		Value:        params.InitialReward,
		ScriptPubKey: scriptPubKey,
	})

	// Create genesis block with transaction
	genesisBlock := &Block{
		Header: &BlockHeader{
			Version:               1,
			PrevBlockHash:         ZeroHash,
			MerkleRoot:            ZeroHash, // Will be calculated below
			Timestamp:             params.GenesisTimestamp,
			Bits:                  params.InitialDifficulty,
			Nonce:                 params.GenesisNonce,
			AccumulatorCheckpoint: ZeroHash, // Version 1, no accumulator
		},
		Transactions: []*Transaction{genesisTx},
		Signature:    []byte{}, // Genesis has no signature
	}

	// Calculate merkle root from transactions (matching legacy BuildMerkleTree())
	genesisBlock.Header.MerkleRoot = genesisBlock.CalculateMerkleRoot()

	// Set canonical hash from chainparams (genesis uses Quark hashing, not SHA256)
	// This ensures Hash() returns the correct Quark hash from legacy
	genesisBlock.SetCanonicalHash(params.GenesisHash)

	return genesisBlock
}

// MainnetGenesisBlock returns the mainnet genesis block
func MainnetGenesisBlock() *Block {
	// Use the hardcoded genesis block for mainnet to ensure exact compatibility
	return HardcodedMainnetGenesis()
}

// TestnetGenesisBlock returns the testnet genesis block
func TestnetGenesisBlock() *Block {
	return GenesisBlock(TestnetParams())
}

// RegtestGenesisBlock returns the regtest genesis block
func RegtestGenesisBlock() *Block {
	return GenesisBlock(RegtestParams())
}

// GenesisBlockHash returns the hash of the genesis block for a network
func GenesisBlockHash(params *ChainParams) Hash {
	return GenesisBlock(params).Header.Hash()
}

// UpdateGenesisHash updates the genesis hash in chain params after creation
func UpdateGenesisHash(params *ChainParams) {
	params.GenesisHash = GenesisBlockHash(params)
}

// InitMainnetGenesis initializes mainnet genesis and returns the params.
// Genesis hashes are hardcoded in chainparams.go, not computed at runtime.
func InitMainnetGenesis() *ChainParams {
	return MainnetParams()
}

// InitTestnetGenesis initializes testnet genesis and returns the params.
func InitTestnetGenesis() *ChainParams {
	return TestnetParams()
}

// InitRegtestGenesis initializes regtest genesis and returns the params.
func InitRegtestGenesis() *ChainParams {
	return RegtestParams()
}

// ParseGenesisHash parses a hex-encoded genesis hash
func ParseGenesisHash(hexStr string) (Hash, error) {
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return ZeroHash, err
	}

	var hash Hash
	copy(hash[:], bytes)
	return hash, nil
}
