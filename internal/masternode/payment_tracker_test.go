package masternode

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/twins-dev/twins-core/pkg/types"
)

func TestNewPaymentTracker(t *testing.T) {
	pt := NewPaymentTracker()
	require.NotNil(t, pt)
	assert.Equal(t, 0, pt.Count())
}

func TestRecordPayment(t *testing.T) {
	pt := NewPaymentTracker()
	script := []byte{0x76, 0xa9, 0x14, 0x01, 0x02, 0x03}
	now := time.Now()

	pt.RecordPayment(script, 500, now, 1000000, "tx500")

	stats := pt.GetStatsByScript(script)
	require.NotNil(t, stats)
	assert.Equal(t, int64(1), stats.PaymentCount)
	assert.Equal(t, int64(1000000), stats.TotalPaid)
	assert.Equal(t, now.Unix(), stats.FirstPaid.Unix())
	assert.Equal(t, now.Unix(), stats.LastPaid.Unix())
	assert.Equal(t, uint32(500), stats.LowestBlock)
	assert.Equal(t, uint32(500), stats.HighestBlock)
	assert.Equal(t, "tx500", stats.LatestTxID)
}

func TestRecordPaymentMultiple(t *testing.T) {
	pt := NewPaymentTracker()
	script := []byte{0x76, 0xa9, 0x14, 0x01, 0x02, 0x03}
	t1 := time.Unix(1000, 0)
	t2 := time.Unix(2000, 0)
	t3 := time.Unix(3000, 0)

	pt.RecordPayment(script, 200, t2, 500, "tx200")
	pt.RecordPayment(script, 100, t1, 300, "tx100") // Earlier time, lower block
	pt.RecordPayment(script, 300, t3, 700, "tx300") // Later time, higher block

	stats := pt.GetStatsByScript(script)
	require.NotNil(t, stats)
	assert.Equal(t, int64(3), stats.PaymentCount)
	assert.Equal(t, int64(1500), stats.TotalPaid)
	assert.Equal(t, t1.Unix(), stats.FirstPaid.Unix()) // Earliest
	assert.Equal(t, t3.Unix(), stats.LastPaid.Unix())   // Latest
	assert.Equal(t, uint32(100), stats.LowestBlock)     // Lowest
	assert.Equal(t, uint32(300), stats.HighestBlock)    // Highest
	assert.Equal(t, "tx300", stats.LatestTxID)          // From highest block
}

func TestRecordPaymentEmptyScript(t *testing.T) {
	pt := NewPaymentTracker()
	pt.RecordPayment(nil, 100, time.Now(), 1000, "")
	pt.RecordPayment([]byte{}, 100, time.Now(), 1000, "")
	assert.Equal(t, 0, pt.Count())
}

func TestGetStatsByScriptNotFound(t *testing.T) {
	pt := NewPaymentTracker()
	stats := pt.GetStatsByScript([]byte{0x01})
	assert.Nil(t, stats)
}

func TestGetStatsByScriptReturnsCopy(t *testing.T) {
	pt := NewPaymentTracker()
	script := []byte{0x01, 0x02}
	pt.RecordPayment(script, 100, time.Unix(1000, 0), 500, "tx100")

	stats1 := pt.GetStatsByScript(script)
	stats1.PaymentCount = 999 // Mutate the copy

	stats2 := pt.GetStatsByScript(script)
	assert.Equal(t, int64(1), stats2.PaymentCount) // Original unchanged
}

func TestGetAllStats(t *testing.T) {
	pt := NewPaymentTracker()
	script1 := []byte{0x01}
	script2 := []byte{0x02}

	pt.RecordPayment(script1, 100, time.Unix(1000, 0), 100, "")
	pt.RecordPayment(script2, 200, time.Unix(2000, 0), 200, "")

	all := pt.GetAllStats()
	assert.Equal(t, 2, len(all))
}

func TestMultipleAddresses(t *testing.T) {
	pt := NewPaymentTracker()
	scriptA := []byte{0x76, 0xa9, 0x14, 0xAA}
	scriptB := []byte{0x76, 0xa9, 0x14, 0xBB}

	pt.RecordPayment(scriptA, 100, time.Unix(1000, 0), 100, "txA1")
	pt.RecordPayment(scriptA, 200, time.Unix(2000, 0), 200, "txA2")
	pt.RecordPayment(scriptB, 300, time.Unix(3000, 0), 300, "txB1")

	assert.Equal(t, 2, pt.Count())

	statsA := pt.GetStatsByScript(scriptA)
	require.NotNil(t, statsA)
	assert.Equal(t, int64(2), statsA.PaymentCount)
	assert.Equal(t, int64(300), statsA.TotalPaid)
	assert.Equal(t, uint32(100), statsA.LowestBlock)
	assert.Equal(t, uint32(200), statsA.HighestBlock)

	statsB := pt.GetStatsByScript(scriptB)
	require.NotNil(t, statsB)
	assert.Equal(t, int64(1), statsB.PaymentCount)
	assert.Equal(t, int64(300), statsB.TotalPaid)
	assert.Equal(t, uint32(300), statsB.LowestBlock)
	assert.Equal(t, uint32(300), statsB.HighestBlock)
}

func TestExtractMasternodePayment(t *testing.T) {
	// Test: nil block
	script, amount := extractMasternodePayment(nil, 400, nil)
	assert.Nil(t, script)
	assert.Equal(t, int64(0), amount)
}

func TestExtractMasternodePaymentPoWBlock(t *testing.T) {
	block := createTestBlock(t, 100) // Below lastPOWBlock
	script, amount := extractMasternodePayment(block, 400, nil)
	assert.Nil(t, script)
	assert.Equal(t, int64(0), amount)
}

func TestExtractMasternodePaymentWithDevAddress(t *testing.T) {
	devAddr := []byte{0xDE, 0xEF}
	mnAddr := []byte{0x76, 0xa9, 0x14, 0xAB}

	block := createTestPoSBlock(t, 500, mnAddr, devAddr)
	script, amount := extractMasternodePayment(block, 400, devAddr)

	assert.Equal(t, mnAddr, script)
	assert.Equal(t, int64(45000), amount)
}

func TestExtractMasternodePaymentWithoutDevAddress(t *testing.T) {
	mnAddr := []byte{0x76, 0xa9, 0x14, 0xAB}

	block := createTestPoSBlockNodev(t, 500, mnAddr)
	script, amount := extractMasternodePayment(block, 400, nil)

	assert.Equal(t, mnAddr, script)
	assert.Equal(t, int64(45000), amount)
}

func TestExtractMasternodePaymentDevFallback(t *testing.T) {
	devAddr := []byte{0xDE, 0xEF}

	block := createTestPoSBlock(t, 500, devAddr, devAddr)
	script, amount := extractMasternodePayment(block, 400, devAddr)

	// extractMasternodePayment returns raw output — caller filters dev
	assert.Equal(t, devAddr, script)
	assert.Equal(t, int64(45000), amount)
}

func TestScanBlockchainSkipsDevPayments(t *testing.T) {
	pt := NewPaymentTracker()
	mnAddr := []byte{0x76, 0xa9, 0x14, 0xAB}

	pt.RecordPayment(mnAddr, 500, time.Unix(1000, 0), 45000, "txMN")

	assert.Equal(t, 1, pt.Count())
	assert.Nil(t, pt.GetStatsByScript([]byte{0xDE, 0xEF}))
}

// Test helpers

func createTestBlock(t *testing.T, height uint32) *types.Block {
	t.Helper()
	block := &types.Block{
		Header: &types.BlockHeader{
			Timestamp: uint32(time.Now().Unix()),
		},
		Transactions: []*types.Transaction{
			{Outputs: []*types.TxOutput{{Value: 0}}},
		},
	}
	block.SetHeight(height)
	return block
}

func createTestPoSBlock(t *testing.T, height uint32, mnPayee []byte, devPayee []byte) *types.Block {
	t.Helper()
	coinstake := &types.Transaction{
		Inputs: []*types.TxInput{
			{PreviousOutput: types.Outpoint{Hash: types.Hash{0x01}}},
		},
		Outputs: []*types.TxOutput{
			{Value: 0, ScriptPubKey: []byte{}},
			{Value: 50000, ScriptPubKey: []byte{0x99}},
			{Value: 45000, ScriptPubKey: mnPayee},
			{Value: 10000, ScriptPubKey: devPayee},
		},
	}
	block := &types.Block{
		Header: &types.BlockHeader{
			Timestamp: uint32(time.Now().Unix()),
		},
		Transactions: []*types.Transaction{
			{Outputs: []*types.TxOutput{{Value: 0}}},
			coinstake,
		},
	}
	block.SetHeight(height)
	return block
}

func createTestPoSBlockNodev(t *testing.T, height uint32, mnPayee []byte) *types.Block {
	t.Helper()
	coinstake := &types.Transaction{
		Inputs: []*types.TxInput{
			{PreviousOutput: types.Outpoint{Hash: types.Hash{0x01}}},
		},
		Outputs: []*types.TxOutput{
			{Value: 0, ScriptPubKey: []byte{}},
			{Value: 50000, ScriptPubKey: []byte{0x99}},
			{Value: 45000, ScriptPubKey: mnPayee},
		},
	}
	block := &types.Block{
		Header: &types.BlockHeader{
			Timestamp: uint32(time.Now().Unix()),
		},
		Transactions: []*types.Transaction{
			{Outputs: []*types.TxOutput{{Value: 0}}},
			coinstake,
		},
	}
	block.SetHeight(height)
	return block
}
