package consensus

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/twins-dev/twins-core/pkg/types"
)

func TestValidateBlockVersion_PoS(t *testing.T) {
	params := types.MainnetParams()

	// Test PoS block with version 4 (valid)
	block := &types.Block{
		Header: &types.BlockHeader{
			Version: uint32(BlockVersionPoSv1),
		},
		Transactions: []*types.Transaction{
			// Coinbase transaction
			{
				Version: 1,
				Inputs: []*types.TxInput{{
					PreviousOutput: types.Outpoint{Index: 0xffffffff},
				}},
				Outputs: []*types.TxOutput{{Value: 0}},
			},
			// Coinstake transaction
			{
				Version: 1,
				Inputs: []*types.TxInput{
					{PreviousOutput: types.Outpoint{
						Hash:  types.NewHash([]byte("test")), // Non-zero hash for coinstake
						Index: 0,
					}},
				},
				Outputs: []*types.TxOutput{
					{Value: 0}, // First output empty for coinstake
					{Value: 100 * 1e8},
				},
			},
		},
		Signature: []byte{0x01}, // Non-empty signature indicates PoS
	}

	err := ValidateBlockVersion(block, nil, params)
	assert.NoError(t, err)

	// Test PoS block with version 5 (valid, current version)
	block.Header.Version = uint32(BlockVersionCurrent)
	err = ValidateBlockVersion(block, nil, params)
	assert.NoError(t, err)

	// Test PoS block with version 3 (invalid, too old)
	block.Header.Version = uint32(BlockVersionPoW)
	err = ValidateBlockVersion(block, nil, params)
	assert.Error(t, err)
	if err != nil {
		validationErr, ok := err.(*ValidationError)
		assert.True(t, ok)
		assert.Equal(t, "BAD_VERSION", validationErr.Code)
	}
}

func TestValidateBlockVersion_PoW(t *testing.T) {
	params := types.MainnetParams()

	// Test PoW block with version 3 (valid if no supermajority)
	block := &types.Block{
		Header: &types.BlockHeader{
			Version: uint32(BlockVersionPoW),
		},
		// No signature indicates PoW
	}

	err := ValidateBlockVersion(block, nil, params)
	assert.NoError(t, err)

	// Test with previous block index that has supermajority
	prevIndex := createTestBlockIndex(5, 1000)

	// Version 1 block should be rejected if supermajority is version 2+
	block.Header.Version = 1
	err = ValidateBlockVersion(block, prevIndex, params)
	assert.Error(t, err)
	validationErr, ok := err.(*ValidationError)
	assert.True(t, ok)
	assert.Equal(t, "OBSOLETE", validationErr.Code)
}

func TestIsSuperMajority(t *testing.T) {
	// Create a chain of block indexes
	chain := createTestBlockIndex(5, 1000)

	// Test supermajority detection
	result := IsSuperMajority(5, chain, 950)
	assert.True(t, result)

	// Test when not enough blocks have the version
	chain = createTestBlockIndex(3, 500)
	chain = appendTestBlockIndex(chain, 5, 500)
	result = IsSuperMajority(5, chain, 950)
	assert.False(t, result)
}

func TestShouldEnforceBIP66(t *testing.T) {
	// Test mainnet
	params := types.MainnetParams()
	assert.False(t, ShouldEnforceBIP66(891729, params))
	assert.True(t, ShouldEnforceBIP66(891730, params))
	assert.True(t, ShouldEnforceBIP66(1000000, params))

	// Test testnet (always enforced)
	testnetParams := types.TestnetParams()
	assert.True(t, ShouldEnforceBIP66(1, testnetParams))
	assert.True(t, ShouldEnforceBIP66(891730, testnetParams))
}

func TestValidateBlockVersionContext(t *testing.T) {
	params := types.MainnetParams()

	// Create block with version 2 and coinbase
	block := &types.Block{
		Header: &types.BlockHeader{
			Version: 2,
		},
		Transactions: []*types.Transaction{
			{
				Version: 1,
				Inputs: []*types.TxInput{
					{
						PreviousOutput: types.Outpoint{
							Hash:  types.Hash{},
							Index: 0xffffffff, // Coinbase marker
						},
						ScriptSig: []byte{0x01, 0x02, 0x03}, // Has some data
					},
				},
				Outputs: []*types.TxOutput{
					{Value: 50 * 1e8},
				},
			},
		},
	}

	// Create previous index with supermajority
	prevIndex := createTestBlockIndex(2, 1000)
	prevIndex.Height = 100

	err := ValidateBlockVersionContext(block, prevIndex, params)
	assert.NoError(t, err)

	// Test with no coinbase (should fail)
	block.Transactions = []*types.Transaction{}
	err = ValidateBlockVersionContext(block, prevIndex, params)
	assert.Error(t, err)
	validationErr, ok := err.(*ValidationError)
	assert.True(t, ok)
	assert.Equal(t, "BAD_COINBASE", validationErr.Code)
}

// Helper functions for tests

func createTestBlockIndex(version int32, count int) *BlockIndex {
	var head *BlockIndex
	var current *BlockIndex

	for i := 0; i < count; i++ {
		index := &BlockIndex{
			Version:   version,
			Height:    uint32(i),
			PrevIndex: current,
		}
		if head == nil {
			head = index
		}
		current = index
	}

	return current
}

func appendTestBlockIndex(chain *BlockIndex, version int32, count int) *BlockIndex {
	current := chain
	startHeight := current.Height + 1

	for i := 0; i < count; i++ {
		index := &BlockIndex{
			Version:   version,
			Height:    startHeight + uint32(i),
			PrevIndex: current,
		}
		current = index
	}

	return current
}