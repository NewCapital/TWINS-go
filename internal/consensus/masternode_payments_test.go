package consensus

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twins-dev/twins-core/pkg/types"
)

// Test helper to create a hash from a string
func hashFromString(s string) types.Hash {
	h := sha256.Sum256([]byte(s))
	var hash types.Hash
	copy(hash[:], h[:])
	return hash
}

// MockSporkManager is a mock spork manager for testing
type MockSporkManager struct {
	active bool
}

func (m *MockSporkManager) IsActive(sporkID int32) bool {
	return m.active
}

func (m *MockSporkManager) GetValue(sporkID int32) int64 {
	if m.active {
		return 1
	}
	return 0
}

// MockMasternodeInterface implements MasternodeInterface for testing
type MockMasternodeInterface struct {
	activeCount     int
	bestHeight      uint32
	masternodes     map[types.Outpoint]MasternodeInfo
	paymentWinner   MasternodeInfo
	paymentWinnerFn func(uint32, types.Hash) (MasternodeInfo, error)
}

func NewMockMasternodeInterface() *MockMasternodeInterface {
	return &MockMasternodeInterface{
		activeCount: 0,
		bestHeight:  1000, // Default to height 1000 for vote window tests
		masternodes: make(map[types.Outpoint]MasternodeInfo),
	}
}

func (m *MockMasternodeInterface) GetActiveCount() int {
	return m.activeCount
}

func (m *MockMasternodeInterface) GetStableCount() int {
	// Mock implementation - return same as active count for testing
	// In real code, this returns masternodes older than 8000 seconds
	return m.activeCount
}

func (m *MockMasternodeInterface) GetBestHeight() (uint32, error) {
	// Mock implementation - return configured best height for vote window validation tests
	return m.bestHeight, nil
}

func (m *MockMasternodeInterface) GetMasternodeByOutpoint(outpoint types.Outpoint) (MasternodeInfo, error) {
	if mn, exists := m.masternodes[outpoint]; exists {
		return mn, nil
	}
	return MasternodeInfo{}, assert.AnError
}

func (m *MockMasternodeInterface) GetNextPaymentWinner(blockHeight uint32, blockHash types.Hash) (MasternodeInfo, error) {
	if m.paymentWinnerFn != nil {
		return m.paymentWinnerFn(blockHeight, blockHash)
	}
	return m.paymentWinner, nil
}

func (m *MockMasternodeInterface) IsActiveAtHeight(outpoint types.Outpoint, height uint32) bool {
	_, exists := m.masternodes[outpoint]
	return exists
}

func (m *MockMasternodeInterface) IsActiveAtHeightLegacy(outpoint types.Outpoint, height uint32) bool {
	// Mock implementation - same as IsActiveAtHeight for testing
	// In real code, this validates current UTXO state
	_, exists := m.masternodes[outpoint]
	return exists
}

func (m *MockMasternodeInterface) GetMasternodePublicKey(outpoint types.Outpoint) ([]byte, error) {
	if mn, exists := m.masternodes[outpoint]; exists {
		return mn.PubKey, nil
	}
	return nil, assert.AnError
}

func (m *MockMasternodeInterface) ProcessPayment(outpoint types.Outpoint, blockHeight int32) error {
	// Mock implementation - just return nil
	return nil
}

func (m *MockMasternodeInterface) ProcessPaymentWithBlockTime(outpoint types.Outpoint, blockHeight int32, blockTime int64, blockHash types.Hash) error {
	// Mock implementation - just return nil
	return nil
}

func (m *MockMasternodeInterface) GetMasternodeRank(outpoint types.Outpoint, blockHeight uint32, minProtocol int32, requireMinAge bool) int {
	// Mock implementation - return a valid rank (within top 10) for all masternodes in list
	if _, exists := m.masternodes[outpoint]; exists {
		return 1 // Always return rank 1 for existing masternodes
	}
	return -1 // Not found
}

func (m *MockMasternodeInterface) GetMinMasternodePaymentsProto() int32 {
	// Mock implementation - return default protocol version
	return 70927 // MinPeerProtoAfterEnforcement
}

func (m *MockMasternodeInterface) GetMasternodeByPayAddress(payAddress []byte) (MasternodeInfo, error) {
	// Search for masternode by pay address
	for _, mn := range m.masternodes {
		if len(mn.PayAddress) > 0 && len(payAddress) > 0 {
			match := true
			for i := range mn.PayAddress {
				if i >= len(payAddress) || mn.PayAddress[i] != payAddress[i] {
					match = false
					break
				}
			}
			if match && len(mn.PayAddress) == len(payAddress) {
				return mn, nil
			}
		}
	}
	return MasternodeInfo{}, assert.AnError
}

func (m *MockMasternodeInterface) MarkPayeeScheduled(payAddress []byte, blockHeight uint32) error {
	// Mock implementation - no-op for tests
	return nil
}

func (m *MockMasternodeInterface) AddMasternode(mn MasternodeInfo) {
	m.masternodes[mn.Outpoint] = mn
	m.activeCount++
}

func TestMasternodePaymentValidator_ValidateBlockPayment(t *testing.T) {
	// Setup mock masternode interface
	mnInterface := NewMockMasternodeInterface()

	// Create a test masternode
	testMN := MasternodeInfo{
		Outpoint: types.Outpoint{
			Hash:  hashFromString("abc123"),
			Index: 0,
		},
		Tier:       2, // Gold
		PayAddress: []byte("test_address_12345"),
	}
	mnInterface.AddMasternode(testMN)
	mnInterface.paymentWinner = testMN

	// Create validator
	validator := NewMasternodePaymentValidator(mnInterface, types.MainnetParams())

	// Create test block with correct payment
	blockReward := int64(10 * 1e8)        // 10 TWINS
	mnPayment := (blockReward * 80) / 100 // 80% to masternodes

	coinstake := &types.Transaction{
		Version: 1,
		Inputs: []*types.TxInput{
			{
				PreviousOutput: types.Outpoint{
					Hash:  hashFromString("prev"),
					Index: 0,
				},
			},
		},
		Outputs: []*types.TxOutput{
			{Value: 0},                       // Empty first output for coinstake
			{Value: blockReward - mnPayment}, // Stake reward
			{Value: mnPayment, ScriptPubKey: testMN.PayAddress}, // MN payment
		},
	}

	block := &types.Block{
		Header: &types.BlockHeader{
			Version:   1,
			Timestamp: 1234567890,
			Bits:      0x1d00ffff,
		},
		Transactions: []*types.Transaction{
			{}, // Genesis/coinbase
			coinstake,
		},
	}

	// Test successful validation
	err := validator.ValidateBlockPayment(block, 1, blockReward, true)
	assert.NoError(t, err, "Valid payment should pass validation")
}

func TestMasternodePaymentValidator_ValidateBlockPayment_WrongAmount(t *testing.T) {
	mnInterface := NewMockMasternodeInterface()

	testMN := MasternodeInfo{
		Outpoint:        types.Outpoint{Hash: hashFromString("abc"), Index: 0},
		Tier:            1, // Silver
		PayAddress:      []byte("test_address"),
		ProtocolVersion: 70928,
	}
	mnInterface.AddMasternode(testMN)
	mnInterface.paymentWinner = testMN

	validator := NewMasternodePaymentValidator(mnInterface, types.MainnetParams())
	validator.SetSkipSignatureVerification(true) // Skip signature verification for unit tests

	// Add mock spork manager that has enforcement enabled
	validator.SetSporkManager(&MockSporkManager{active: true})

	// CRITICAL: Must add votes for payment validation to occur
	// Legacy behavior: when no votes exist, ANY payment is accepted (CMasternodePayments::IsTransactionValid returns true)
	// We need votes to test wrong amount validation
	blockHeight := uint32(500)

	// Add vote directly to internal map (bypassing validation for unit test)
	// We can't use AddPaymentVote because it validates the masternode is active
	validator.mu.Lock()
	validator.paymentVotes[blockHeight] = &BlockPaymentVotes{
		BlockHeight: blockHeight,
		Payees: []*PayeeVotes{
			{
				PayAddress: testMN.PayAddress,
				Votes:      MinPaymentSignatures, // Enough votes to pass threshold
			},
		},
	}
	validator.mu.Unlock()

	blockReward := int64(10 * 1e8)
	wrongPayment := int64(1 * 1e8) // Wrong amount

	coinstake := &types.Transaction{
		Outputs: []*types.TxOutput{
			{Value: 0},
			{Value: blockReward - wrongPayment},
			{Value: wrongPayment, ScriptPubKey: testMN.PayAddress}, // Wrong amount
		},
	}

	block := &types.Block{
		Header:       &types.BlockHeader{Version: 1},
		Transactions: []*types.Transaction{{}, coinstake},
	}

	// Use blockHeight > 400 (LastPOWBlock) to trigger PoS path which uses Transactions[1]
	err := validator.ValidateBlockPayment(block, blockHeight, blockReward, true)
	assert.Error(t, err, "Wrong payment amount should fail validation")
	if err != nil {
		assert.Contains(t, err.Error(), "incorrect payment amount")
	}
}

func TestMasternodePaymentValidator_AddPaymentVote(t *testing.T) {
	mnInterface := NewMockMasternodeInterface()
	// Set bestHeight to allow votes for heights 100-101 (window: [bestHeight - activeCount*1.25, bestHeight + 20])
	// With activeCount=0, window is [bestHeight, bestHeight+20], so bestHeight=100 allows 100-120
	mnInterface.bestHeight = 100

	testMN := MasternodeInfo{
		Outpoint:        types.Outpoint{Hash: hashFromString("mn1"), Index: 0},
		PayAddress:      []byte("mn1_address"),
		ProtocolVersion: 70928,
	}
	mnInterface.AddMasternode(testMN)

	validator := NewMasternodePaymentValidator(mnInterface, types.MainnetParams())
	validator.SetSkipSignatureVerification(true) // Skip signature verification for unit tests

	// Add first vote
	err := validator.AddPaymentVote(100, testMN.Outpoint, testMN.PayAddress, nil)
	assert.NoError(t, err, "First vote should succeed")

	// Try to vote again for same height
	err = validator.AddPaymentVote(100, testMN.Outpoint, testMN.PayAddress, nil)
	assert.Error(t, err, "Duplicate vote should fail")
	assert.Contains(t, err.Error(), "already voted")

	// Vote for different height should succeed
	err = validator.AddPaymentVote(101, testMN.Outpoint, testMN.PayAddress, nil)
	assert.NoError(t, err, "Vote for different height should succeed")
}

// TestMasternodePaymentValidator_SignatureRequired verifies that signature verification
// is enforced by default (when SetSkipSignatureVerification is not called)
func TestMasternodePaymentValidator_SignatureRequired(t *testing.T) {
	mnInterface := NewMockMasternodeInterface()
	// Set bestHeight to allow votes for height 100
	mnInterface.bestHeight = 100

	testMN := MasternodeInfo{
		Outpoint:        types.Outpoint{Hash: hashFromString("mn_sig_test"), Index: 0},
		PayAddress:      []byte("sig_test_address"),
		ProtocolVersion: 70928,
	}
	mnInterface.AddMasternode(testMN)

	validator := NewMasternodePaymentValidator(mnInterface, types.MainnetParams())
	// NOTE: NOT calling SetSkipSignatureVerification - should enforce signatures

	// Try to add vote with nil signature - should be rejected
	err := validator.AddPaymentVote(100, testMN.Outpoint, testMN.PayAddress, nil)
	assert.Error(t, err, "Should reject vote with nil signature")
	assert.Contains(t, err.Error(), "missing required signature")

	// Try to add vote with empty signature - should also be rejected
	err = validator.AddPaymentVote(100, testMN.Outpoint, testMN.PayAddress, []byte{})
	assert.Error(t, err, "Should reject vote with empty signature")
	assert.Contains(t, err.Error(), "missing required signature")
}

func TestMasternodePaymentValidator_GetPaymentQueueInfo(t *testing.T) {
	mnInterface := NewMockMasternodeInterface()
	// Set bestHeight to allow votes for height 100
	mnInterface.bestHeight = 100

	mn1 := MasternodeInfo{
		Outpoint:        types.Outpoint{Hash: hashFromString("mn1"), Index: 0},
		PayAddress:      []byte("addr1"),
		ProtocolVersion: 70928,
	}
	mn2 := MasternodeInfo{
		Outpoint:        types.Outpoint{Hash: hashFromString("mn2"), Index: 0},
		PayAddress:      []byte("addr2"),
		ProtocolVersion: 70928,
	}

	mnInterface.AddMasternode(mn1)
	mnInterface.AddMasternode(mn2)

	validator := NewMasternodePaymentValidator(mnInterface, types.MainnetParams())
	validator.SetSkipSignatureVerification(true) // Skip signature verification for unit tests

	// Add votes
	validator.AddPaymentVote(100, mn1.Outpoint, mn1.PayAddress, nil)
	validator.AddPaymentVote(100, mn2.Outpoint, mn1.PayAddress, nil) // Vote for addr1
	validator.AddPaymentVote(100, mn1.Outpoint, mn2.PayAddress, nil) // This should fail (already voted)

	// Get queue info
	info, err := validator.GetPaymentQueueInfo(100)
	require.NoError(t, err)
	assert.NotNil(t, info)

	// addr1 should have 2 votes and be the winner
	assert.Equal(t, mn1.PayAddress, info.Winner)
	assert.Equal(t, 2, info.VoteCount)
}

func TestMasternodePaymentValidator_FillBlockPayment(t *testing.T) {
	mnInterface := NewMockMasternodeInterface()

	testMN := MasternodeInfo{
		Outpoint:   types.Outpoint{Hash: hashFromString("mn1"), Index: 0},
		PayAddress: []byte("payment_address"),
		Tier:       3, // Platinum
	}
	mnInterface.AddMasternode(testMN)
	mnInterface.paymentWinner = testMN

	validator := NewMasternodePaymentValidator(mnInterface, types.MainnetParams())

	// Create coinstake transaction
	tx := &types.Transaction{
		Outputs: []*types.TxOutput{
			{Value: 0},        // Empty first output
			{Value: 10 * 1e8}, // Initial stake reward
		},
	}

	blockReward := int64(10 * 1e8)
	blockHash := hashFromString("block123")

	// Fill payment (true = PoS block, so dev fund will be added)
	err := validator.FillBlockPayment(tx, 1, blockReward, blockHash, true)
	assert.NoError(t, err)

	// Check outputs were added
	assert.Greater(t, len(tx.Outputs), 2, "Payment outputs should be added")

	// Check masternode payment output
	mnPayment := (blockReward * 80) / 100
	foundMNPayment := false
	for _, output := range tx.Outputs {
		if string(output.ScriptPubKey) == string(testMN.PayAddress) {
			assert.Equal(t, mnPayment, output.Value)
			foundMNPayment = true
			break
		}
	}
	assert.True(t, foundMNPayment, "Masternode payment output should exist")
}

func TestMasternodePaymentValidator_CleanupOldVotes(t *testing.T) {
	mnInterface := NewMockMasternodeInterface()
	// Set activeCount high enough to create a large vote window for testing
	// Window size = activeCount * 1.25 = 2000 * 1.25 = 2500
	// This allows votes from height 100 to 1500 when bestHeight starts at 100
	mnInterface.activeCount = 2000

	mn := MasternodeInfo{
		Outpoint:        types.Outpoint{Hash: hashFromString("mn1"), Index: 0},
		PayAddress:      []byte("addr"),
		ProtocolVersion: 70928,
	}
	mnInterface.AddMasternode(mn)

	validator := NewMasternodePaymentValidator(mnInterface, types.MainnetParams())
	validator.SetSkipSignatureVerification(true) // Skip signature verification for unit tests

	// Add votes at various heights: 100, 200, 300, ..., 1500
	// Note: Heights < 100 are rejected due to legacy compatibility (rank check underflow)
	// We need to adjust bestHeight for each vote to be within the window
	for height := uint32(100); height <= 1500; height += 100 {
		// Set bestHeight so this height is within the window [bestHeight - 2500, bestHeight + 20]
		mnInterface.bestHeight = height
		validator.AddPaymentVote(height, mn.Outpoint, mn.PayAddress, nil)
		// Reset vote tracking to allow multiple votes for testing
		delete(validator.lastVotes, mn.Outpoint)
	}

	// Should have votes for 15 blocks (100, 200, 300, ..., 1500)
	assert.Equal(t, 15, len(validator.paymentVotes))

	// Cleanup old votes
	// With activeCount=2000, limit = max(2000*1.25, 1000) = max(2500, 1000) = 2500
	// Cutoff = 1500 - 2500 = negative -> 0, so all votes would be kept
	// Let's use a lower activeCount for the cleanup test
	mnInterface.activeCount = 100
	// With activeCount=100, limit = max(100*1.25, 1000) = max(125, 1000) = 1000
	// Cutoff = 1500 - 1000 = 500, so keep blocks >= 500
	validator.CleanupOldVotes(1500)

	// Should keep votes >= 500 (that's 500, 600, 700, ... 1500)
	assert.Less(t, len(validator.paymentVotes), 15, "Old votes should be cleaned up")
	assert.Greater(t, len(validator.paymentVotes), 0, "Should keep some recent votes")

	// Verify old votes are gone
	_, exists := validator.paymentVotes[100]
	assert.False(t, exists, "Very old votes should be removed")
	_, exists = validator.paymentVotes[400]
	assert.False(t, exists, "Votes below cutoff should be removed")

	// Recent votes should still exist
	_, exists = validator.paymentVotes[500]
	assert.True(t, exists, "Votes at cutoff boundary should be kept")
	_, exists = validator.paymentVotes[1500]
	assert.True(t, exists, "Recent votes should be kept")
}

func TestMasternodePaymentValidator_CalculateMasternodePayment(t *testing.T) {
	mnInterface := NewMockMasternodeInterface()
	mnInterface.activeCount = 50

	validator := NewMasternodePaymentValidator(mnInterface, types.MainnetParams())

	blockReward := int64(10 * 1e8) // 10 TWINS

	payment := validator.calculateMasternodePayment(1, blockReward)

	// Should be 80% of block reward
	expectedPayment := (blockReward * 80) / 100
	assert.Equal(t, expectedPayment, payment)
}

func TestMasternodePaymentValidator_NoMasternodesAvailable(t *testing.T) {
	mnInterface := NewMockMasternodeInterface()
	mnInterface.activeCount = 0

	validator := NewMasternodePaymentValidator(mnInterface, types.MainnetParams())
	validator.SetSporkManager(&MockSporkManager{active: true})

	block := &types.Block{
		Header:       &types.BlockHeader{Version: 1},
		Transactions: []*types.Transaction{{}, {}},
	}

	// LEGACY COMPATIBILITY: When no votes exist, payment validation returns success
	// This matches CMasternodePayments::IsTransactionValid which returns true when
	// no entry exists in mapMasternodeBlocks (masternode-payments.cpp:679-688)
	// This is critical for initial sync, reindex, and network startup scenarios
	err := validator.ValidateBlockPayment(block, 1, 10*1e8, true)
	assert.NoError(t, err, "Should accept payment when no votes exist (legacy compatibility)")
}

func TestMasternodePaymentValidator_VotingConsensus(t *testing.T) {
	mnInterface := NewMockMasternodeInterface()
	// Set bestHeight to allow votes for height 500
	mnInterface.bestHeight = 500

	// Create multiple masternodes
	masternodes := []MasternodeInfo{
		{Outpoint: types.Outpoint{Hash: hashFromString("mn1"), Index: 0}, PayAddress: []byte("addr1"), ProtocolVersion: 70928},
		{Outpoint: types.Outpoint{Hash: hashFromString("mn2"), Index: 0}, PayAddress: []byte("addr2"), ProtocolVersion: 70928},
		{Outpoint: types.Outpoint{Hash: hashFromString("mn3"), Index: 0}, PayAddress: []byte("addr3"), ProtocolVersion: 70928},
		{Outpoint: types.Outpoint{Hash: hashFromString("mn4"), Index: 0}, PayAddress: []byte("addr4"), ProtocolVersion: 70928},
		{Outpoint: types.Outpoint{Hash: hashFromString("mn5"), Index: 0}, PayAddress: []byte("addr5"), ProtocolVersion: 70928},
		{Outpoint: types.Outpoint{Hash: hashFromString("mn6"), Index: 0}, PayAddress: []byte("addr6"), ProtocolVersion: 70928},
		{Outpoint: types.Outpoint{Hash: hashFromString("mn7"), Index: 0}, PayAddress: []byte("addr7"), ProtocolVersion: 70928},
	}

	for _, mn := range masternodes {
		mnInterface.AddMasternode(mn)
	}

	validator := NewMasternodePaymentValidator(mnInterface, types.MainnetParams())
	validator.SetSporkManager(&MockSporkManager{active: true})
	validator.SetSkipSignatureVerification(true) // Skip signature verification for unit tests

	// Use blockHeight > 400 (LastPOWBlock) to trigger PoS path which uses Transactions[1]
	blockHeight := uint32(500)
	blockReward := int64(10 * 1e8)

	// 6 masternodes vote for addr1, 1 votes for addr2
	for i := 0; i < 6; i++ {
		validator.AddPaymentVote(blockHeight, masternodes[i].Outpoint, []byte("addr1"), nil)
	}
	validator.AddPaymentVote(blockHeight, masternodes[6].Outpoint, []byte("addr2"), nil)

	// Create block with payment to addr1
	mnPayment := (blockReward * 80) / 100
	coinstake := &types.Transaction{
		Outputs: []*types.TxOutput{
			{Value: 0},
			{Value: blockReward - mnPayment},
			{Value: mnPayment, ScriptPubKey: []byte("addr1")},
		},
	}

	block := &types.Block{
		Header:       &types.BlockHeader{Version: 1},
		Transactions: []*types.Transaction{{}, coinstake},
	}

	// Should pass - addr1 has majority votes
	err := validator.ValidateBlockPayment(block, blockHeight, blockReward, true)
	assert.NoError(t, err)

	// Create block with payment to addr2 (minority)
	coinstake2 := &types.Transaction{
		Outputs: []*types.TxOutput{
			{Value: 0},
			{Value: blockReward - mnPayment},
			{Value: mnPayment, ScriptPubKey: []byte("addr2")},
		},
	}

	block2 := &types.Block{
		Header:       &types.BlockHeader{Version: 1},
		Transactions: []*types.Transaction{{}, coinstake2},
	}

	// Should fail - addr2 doesn't have majority
	err = validator.ValidateBlockPayment(block2, blockHeight, blockReward, true)
	assert.Error(t, err)
}
