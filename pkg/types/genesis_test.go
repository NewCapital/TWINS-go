package types

import (
	"testing"
)

func TestGenesisBlock(t *testing.T) {
	tests := []struct {
		name   string
		params *ChainParams
	}{
		{"mainnet", MainnetParams()},
		{"testnet", TestnetParams()},
		{"regtest", RegtestParams()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			genesis := GenesisBlock(tt.params)

			if genesis == nil {
				t.Fatal("Genesis block is nil")
			}

			if genesis.Header == nil {
				t.Fatal("Genesis header is nil")
			}

			if len(genesis.Transactions) == 0 {
				t.Fatal("Genesis block has no transactions")
			}

			// Check coinbase transaction
			coinbase := genesis.Transactions[0]
			if coinbase == nil {
				t.Fatal("Coinbase transaction is nil")
			}

			if len(coinbase.Inputs) == 0 {
				t.Fatal("Coinbase has no inputs")
			}

			// Check coinbase indicator
			if coinbase.Inputs[0].PreviousOutput.Index != 0xffffffff {
				t.Errorf("Coinbase input index = %d, want 0xffffffff",
					coinbase.Inputs[0].PreviousOutput.Index)
			}

			// Check merkle root matches coinbase hash
			coinbaseHash := coinbase.Hash()
			if genesis.Header.MerkleRoot != coinbaseHash {
				t.Errorf("Merkle root mismatch: got %s, want %s",
					genesis.Header.MerkleRoot, coinbaseHash)
			}

			// Check prev block hash is zero
			if genesis.Header.PrevBlockHash != ZeroHash {
				t.Errorf("Genesis prev block hash is not zero: %s",
					genesis.Header.PrevBlockHash)
			}
		})
	}
}

func TestMainnetGenesisBlock(t *testing.T) {
	genesis := MainnetGenesisBlock()
	if genesis == nil {
		t.Fatal("Mainnet genesis block is nil")
	}

	// Check timestamp matches mainnet params
	params := MainnetParams()
	if genesis.Header.Timestamp != params.GenesisTimestamp {
		t.Errorf("Genesis timestamp = %d, want %d",
			genesis.Header.Timestamp, params.GenesisTimestamp)
	}

	// Check difficulty matches
	if genesis.Header.Bits != params.InitialDifficulty {
		t.Errorf("Genesis difficulty = %x, want %x",
			genesis.Header.Bits, params.InitialDifficulty)
	}
}

func TestTestnetGenesisBlock(t *testing.T) {
	genesis := TestnetGenesisBlock()
	if genesis == nil {
		t.Fatal("Testnet genesis block is nil")
	}

	params := TestnetParams()
	if genesis.Header.Timestamp != params.GenesisTimestamp {
		t.Errorf("Genesis timestamp = %d, want %d",
			genesis.Header.Timestamp, params.GenesisTimestamp)
	}
}

func TestRegtestGenesisBlock(t *testing.T) {
	genesis := RegtestGenesisBlock()
	if genesis == nil {
		t.Fatal("Regtest genesis block is nil")
	}

	params := RegtestParams()
	if genesis.Header.Timestamp != params.GenesisTimestamp {
		t.Errorf("Genesis timestamp = %d, want %d",
			genesis.Header.Timestamp, params.GenesisTimestamp)
	}

	// Regtest should have minimal difficulty
	if genesis.Header.Bits != params.InitialDifficulty {
		t.Errorf("Genesis difficulty = %x, want %x",
			genesis.Header.Bits, params.InitialDifficulty)
	}
}

func TestGenesisBlockHash(t *testing.T) {
	params := MainnetParams()
	hash := GenesisBlockHash(params)

	if hash == ZeroHash {
		t.Error("Genesis hash should not be zero hash")
	}

	// Test that hash is deterministic
	hash2 := GenesisBlockHash(params)
	if hash != hash2 {
		t.Error("Genesis hash is not deterministic")
	}
}

func TestInitGenesisParams(t *testing.T) {
	// Test mainnet
	mainnetParams := InitMainnetGenesis()
	if mainnetParams.GenesisHash == ZeroHash {
		t.Error("Mainnet genesis hash not initialized")
	}

	// Test testnet
	testnetParams := InitTestnetGenesis()
	if testnetParams.GenesisHash == ZeroHash {
		t.Error("Testnet genesis hash not initialized")
	}

	// Test regtest
	regtestParams := InitRegtestGenesis()
	if regtestParams.GenesisHash == ZeroHash {
		t.Error("Regtest genesis hash not initialized")
	}

	// Hashes should be different for different networks
	if mainnetParams.GenesisHash == testnetParams.GenesisHash {
		t.Error("Mainnet and testnet genesis hashes should be different")
	}

	if mainnetParams.GenesisHash == regtestParams.GenesisHash {
		t.Error("Mainnet and regtest genesis hashes should be different")
	}
}

func TestUpdateGenesisHash(t *testing.T) {
	// Create a params with zero genesis hash to test UpdateGenesisHash
	params := &ChainParams{
		Name:              "test",
		GenesisTimestamp:  1609459200,        // 2021-01-01
		GenesisNonce:      1234567,
		InitialDifficulty: 0x1e0ffff0,
		InitialReward:     50 * 100000000, // 50 TWINS
		GenesisHash:       ZeroHash,       // Start with zero
	}

	// Should start with zero hash
	if params.GenesisHash != ZeroHash {
		t.Error("Initial genesis hash should be zero")
	}

	UpdateGenesisHash(params)

	// Should be updated after calling UpdateGenesisHash
	if params.GenesisHash == ZeroHash {
		t.Error("Genesis hash not updated")
	}

	// Should match GenesisBlockHash
	expectedHash := GenesisBlockHash(params)
	if params.GenesisHash != expectedHash {
		t.Errorf("Updated genesis hash = %s, want %s",
			params.GenesisHash, expectedHash)
	}
}
