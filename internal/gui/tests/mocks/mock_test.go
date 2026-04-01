package mocks

import (
	"context"
	"testing"
	"time"

	"github.com/twins-dev/twins-core/internal/gui/core"
)

// TestMockCoreClient_Lifecycle tests the core lifecycle operations
func TestMockCoreClient_Lifecycle(t *testing.T) {
	mock := NewMockCoreClient()

	// Test initial state
	if mock.IsRunning() {
		t.Error("expected mock to not be running initially")
	}

	// Test Start
	ctx := context.Background()
	if err := mock.Start(ctx); err != nil {
		t.Fatalf("failed to start mock: %v", err)
	}

	if !mock.IsRunning() {
		t.Error("expected mock to be running after Start")
	}

	// Test double start
	if err := mock.Start(ctx); err == nil {
		t.Error("expected error when starting already running mock")
	}

	// Test Stop
	if err := mock.Stop(); err != nil {
		t.Fatalf("failed to stop mock: %v", err)
	}

	if mock.IsRunning() {
		t.Error("expected mock to not be running after Stop")
	}

	// Test double stop
	if err := mock.Stop(); err == nil {
		t.Error("expected error when stopping already stopped mock")
	}
}

// TestMockCoreClient_Events tests event emission
func TestMockCoreClient_Events(t *testing.T) {
	mock := NewMockCoreClient()
	ctx := context.Background()

	if err := mock.Start(ctx); err != nil {
		t.Fatalf("failed to start mock: %v", err)
	}
	defer mock.Stop()

	// Collect events for a short time
	eventChan := mock.Events()
	receivedEvents := make([]core.CoreEvent, 0)

	timeout := time.After(200 * time.Millisecond)
	collecting := true

	for collecting {
		select {
		case event := <-eventChan:
			receivedEvents = append(receivedEvents, event)
		case <-timeout:
			collecting = false
		}
	}

	// We should have received some initialization events
	if len(receivedEvents) == 0 {
		t.Error("expected to receive some events during startup")
	}

	// Check for init message events
	foundInitEvent := false
	for _, event := range receivedEvents {
		if event.EventType() == "init_message" {
			foundInitEvent = true
			break
		}
	}

	if !foundInitEvent {
		t.Error("expected to receive at least one init_message event")
	}
}

// TestMockCoreClient_GetBalance tests balance retrieval
func TestMockCoreClient_GetBalance(t *testing.T) {
	mock := NewMockCoreClient()
	ctx := context.Background()

	// Test before starting
	if _, err := mock.GetBalance(); err == nil {
		t.Error("expected error when getting balance before start")
	}

	if err := mock.Start(ctx); err != nil {
		t.Fatalf("failed to start mock: %v", err)
	}
	defer mock.Stop()

	balance, err := mock.GetBalance()
	if err != nil {
		t.Fatalf("failed to get balance: %v", err)
	}

	if balance.Total <= 0 {
		t.Error("expected positive total balance")
	}

	if balance.Spendable > balance.Total {
		t.Error("spendable balance should not exceed total balance")
	}
}

// TestMockCoreClient_GetNewAddress tests address generation
func TestMockCoreClient_GetNewAddress(t *testing.T) {
	mock := NewMockCoreClient()
	ctx := context.Background()

	if err := mock.Start(ctx); err != nil {
		t.Fatalf("failed to start mock: %v", err)
	}
	defer mock.Stop()

	addr1, err := mock.GetNewAddress("test label")
	if err != nil {
		t.Fatalf("failed to generate address: %v", err)
	}

	if len(addr1) == 0 {
		t.Error("expected non-empty address")
	}

	if addr1[0] != 'D' {
		t.Error("TWINS addresses should start with 'D'")
	}

	// Generate another address and ensure it's different
	addr2, err := mock.GetNewAddress("")
	if err != nil {
		t.Fatalf("failed to generate second address: %v", err)
	}

	if addr1 == addr2 {
		t.Error("expected different addresses to be generated")
	}
}

// TestMockCoreClient_SendToAddress tests sending transactions
func TestMockCoreClient_SendToAddress(t *testing.T) {
	mock := NewMockCoreClient()
	ctx := context.Background()

	if err := mock.Start(ctx); err != nil {
		t.Fatalf("failed to start mock: %v", err)
	}
	defer mock.Stop()

	// Get initial balance
	initialBalance, err := mock.GetBalance()
	if err != nil {
		t.Fatalf("failed to get balance: %v", err)
	}

	// Generate a destination address
	destAddr, err := mock.GetNewAddress("")
	if err != nil {
		t.Fatalf("failed to generate address: %v", err)
	}

	// Send some coins
	amount := 100.0
	txid, err := mock.SendToAddress(destAddr, amount, "test transaction")
	if err != nil {
		t.Fatalf("failed to send: %v", err)
	}

	if len(txid) == 0 {
		t.Error("expected non-empty transaction ID")
	}

	// Check balance changed
	newBalance, err := mock.GetBalance()
	if err != nil {
		t.Fatalf("failed to get new balance: %v", err)
	}

	expectedBalance := initialBalance.Spendable - amount - 0.001 // amount + fee
	if newBalance.Spendable > expectedBalance+0.01 || newBalance.Spendable < expectedBalance-0.01 {
		t.Errorf("expected balance ~%.8f, got %.8f", expectedBalance, newBalance.Spendable)
	}

	// Verify transaction exists
	tx, err := mock.GetTransaction(txid)
	if err != nil {
		t.Fatalf("failed to get transaction: %v", err)
	}

	if tx.Amount != -amount {
		t.Errorf("expected amount -%.8f, got %.8f", amount, tx.Amount)
	}
}

// TestMockCoreClient_ListTransactions tests transaction listing
func TestMockCoreClient_ListTransactions(t *testing.T) {
	mock := NewMockCoreClient()
	ctx := context.Background()

	if err := mock.Start(ctx); err != nil {
		t.Fatalf("failed to start mock: %v", err)
	}
	defer mock.Stop()

	// List transactions
	txs, err := mock.ListTransactions(10, 0)
	if err != nil {
		t.Fatalf("failed to list transactions: %v", err)
	}

	if len(txs) == 0 {
		t.Error("expected some initial transactions")
	}

	// Test pagination
	txs1, err := mock.ListTransactions(5, 0)
	if err != nil {
		t.Fatalf("failed to list first page: %v", err)
	}

	txs2, err := mock.ListTransactions(5, 5)
	if err != nil {
		t.Fatalf("failed to list second page: %v", err)
	}

	if len(txs1) > 0 && len(txs2) > 0 && txs1[0].TxID == txs2[0].TxID {
		t.Error("expected different transactions in different pages")
	}
}

// TestMockCoreClient_WalletEncryption tests wallet encryption and locking
func TestMockCoreClient_WalletEncryption(t *testing.T) {
	mock := NewMockCoreClient()
	ctx := context.Background()

	if err := mock.Start(ctx); err != nil {
		t.Fatalf("failed to start mock: %v", err)
	}
	defer mock.Stop()

	// Test encryption
	passphrase := "test_passphrase_123"
	if err := mock.EncryptWallet(passphrase); err != nil {
		t.Fatalf("failed to encrypt wallet: %v", err)
	}

	// Verify wallet is locked
	info, err := mock.GetWalletInfo()
	if err != nil {
		t.Fatalf("failed to get wallet info: %v", err)
	}

	if !info.Encrypted {
		t.Error("expected wallet to be encrypted")
	}

	if info.Unlocked {
		t.Error("expected wallet to be locked after encryption")
	}

	// Test unlock
	if err := mock.WalletPassphrase(passphrase, 60); err != nil {
		t.Fatalf("failed to unlock wallet: %v", err)
	}

	info, err = mock.GetWalletInfo()
	if err != nil {
		t.Fatalf("failed to get wallet info after unlock: %v", err)
	}

	if !info.Unlocked {
		t.Error("expected wallet to be unlocked")
	}

	// Test lock
	if err := mock.WalletLock(); err != nil {
		t.Fatalf("failed to lock wallet: %v", err)
	}

	info, err = mock.GetWalletInfo()
	if err != nil {
		t.Fatalf("failed to get wallet info after lock: %v", err)
	}

	if info.Unlocked {
		t.Error("expected wallet to be locked")
	}
}

// TestMockCoreClient_Blockchain tests blockchain operations
func TestMockCoreClient_Blockchain(t *testing.T) {
	mock := NewMockCoreClient()
	ctx := context.Background()

	if err := mock.Start(ctx); err != nil {
		t.Fatalf("failed to start mock: %v", err)
	}
	defer mock.Stop()

	// Test blockchain info
	info, err := mock.GetBlockchainInfo()
	if err != nil {
		t.Fatalf("failed to get blockchain info: %v", err)
	}

	if info.Blocks <= 0 {
		t.Error("expected positive block count")
	}

	if info.Chain != "main" {
		t.Error("expected chain to be 'main'")
	}

	// Test block count
	count, err := mock.GetBlockCount()
	if err != nil {
		t.Fatalf("failed to get block count: %v", err)
	}

	if count != info.Blocks {
		t.Error("block count should match blockchain info")
	}

	// Test get block hash
	hash, err := mock.GetBlockHash(count)
	if err != nil {
		t.Fatalf("failed to get block hash: %v", err)
	}

	if len(hash) == 0 {
		t.Error("expected non-empty block hash")
	}

	// Test get block
	block, err := mock.GetBlock(hash)
	if err != nil {
		t.Fatalf("failed to get block: %v", err)
	}

	if block.Height != count {
		t.Errorf("expected block height %d, got %d", count, block.Height)
	}
}

// TestMockCoreClient_Masternodes tests masternode operations
func TestMockCoreClient_Masternodes(t *testing.T) {
	mock := NewMockCoreClient()
	ctx := context.Background()

	if err := mock.Start(ctx); err != nil {
		t.Fatalf("failed to start mock: %v", err)
	}
	defer mock.Stop()

	// Test masternode list
	mns, err := mock.MasternodeList("")
	if err != nil {
		t.Fatalf("failed to list masternodes: %v", err)
	}

	if len(mns) == 0 {
		t.Error("expected some masternodes in list")
	}

	// Test filtered list
	enabledMns, err := mock.MasternodeList("enabled")
	if err != nil {
		t.Fatalf("failed to list enabled masternodes: %v", err)
	}

	for _, mn := range enabledMns {
		if mn.Status != "ENABLED" {
			t.Error("expected only ENABLED masternodes in filtered list")
		}
	}

	// Test masternode count
	count, err := mock.GetMasternodeCount()
	if err != nil {
		t.Fatalf("failed to get masternode count: %v", err)
	}

	if count.Total == 0 {
		t.Error("expected non-zero total masternode count")
	}

	// Unlock wallet for masternode operations
	mock.EncryptWallet("testpass123")
	mock.WalletPassphrase("testpass123", 60)

	// Test masternode start
	if err := mock.MasternodeStart("test_mn"); err != nil {
		t.Fatalf("failed to start masternode: %v", err)
	}
}

// TestMockCoreClient_Staking tests staking operations
func TestMockCoreClient_Staking(t *testing.T) {
	mock := NewMockCoreClient()
	ctx := context.Background()

	if err := mock.Start(ctx); err != nil {
		t.Fatalf("failed to start mock: %v", err)
	}
	defer mock.Stop()

	// Test staking info
	info, err := mock.GetStakingInfo()
	if err != nil {
		t.Fatalf("failed to get staking info: %v", err)
	}

	// Test enable staking
	if err := mock.SetStaking(true); err != nil {
		t.Fatalf("failed to enable staking: %v", err)
	}

	// Verify staking is enabled
	enabled, err := mock.GetStakingStatus()
	if err != nil {
		t.Fatalf("failed to get staking status: %v", err)
	}

	if !enabled {
		t.Error("expected staking to be enabled")
	}

	// Test disable staking
	if err := mock.SetStaking(false); err != nil {
		t.Fatalf("failed to disable staking: %v", err)
	}

	enabled, err = mock.GetStakingStatus()
	if err != nil {
		t.Fatalf("failed to get staking status after disable: %v", err)
	}

	if enabled {
		t.Error("expected staking to be disabled")
	}

	// Verify staking info fields
	if !info.Enabled && !info.Staking {
		t.Log("staking info retrieved successfully")
	}
}

// TestMockCoreClient_SignVerify tests message signing and verification
func TestMockCoreClient_SignVerify(t *testing.T) {
	mock := NewMockCoreClient()
	ctx := context.Background()

	if err := mock.Start(ctx); err != nil {
		t.Fatalf("failed to start mock: %v", err)
	}
	defer mock.Stop()

	// Generate an address
	addr, err := mock.GetNewAddress("signing test")
	if err != nil {
		t.Fatalf("failed to generate address: %v", err)
	}

	message := "Hello, TWINS!"

	// Sign message
	signature, err := mock.SignMessage(addr, message)
	if err != nil {
		t.Fatalf("failed to sign message: %v", err)
	}

	if len(signature) == 0 {
		t.Error("expected non-empty signature")
	}

	// Verify signature
	valid, err := mock.VerifyMessage(addr, signature, message)
	if err != nil {
		t.Fatalf("failed to verify message: %v", err)
	}

	if !valid {
		t.Error("expected signature to be valid")
	}

	// Verify with wrong message
	valid, err = mock.VerifyMessage(addr, signature, "Wrong message")
	if err != nil {
		t.Fatalf("failed to verify wrong message: %v", err)
	}

	if valid {
		t.Error("expected signature to be invalid for wrong message")
	}
}
