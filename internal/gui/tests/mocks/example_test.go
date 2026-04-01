package mocks

import (
	"context"
	"fmt"
	"time"
)

// ExampleMockCoreClient demonstrates basic usage of the MockCoreClient
func ExampleMockCoreClient() {
	// Create a new mock client
	mock := NewMockCoreClient()

	// Start the mock core
	ctx := context.Background()
	if err := mock.Start(ctx); err != nil {
		panic(err)
	}
	defer mock.Stop()

	// Get wallet balance
	balance, _ := mock.GetBalance()
	fmt.Printf("Total Balance: %.2f TWINS\n", balance.Total)
	fmt.Printf("Spendable: %.2f TWINS\n", balance.Spendable)

	// Get blockchain info
	info, _ := mock.GetBlockchainInfo()
	fmt.Printf("Current Height: %d\n", info.Blocks)
	fmt.Printf("Sync Progress: %.0f%%\n", info.VerificationProgress*100)
	fmt.Printf("Chain: %s\n", info.Chain)

	// Output:
	// Total Balance: 1100000.50 TWINS
	// Spendable: 1000000.00 TWINS
	// Current Height: 1500000
	// Sync Progress: 100%
	// Chain: main
}

// ExampleMockCoreClient_events demonstrates event handling
func ExampleMockCoreClient_events() {
	mock := NewMockCoreClient()
	ctx := context.Background()
	mock.Start(ctx)
	defer mock.Stop()

	// Listen for events in a goroutine
	go func() {
		eventChan := mock.Events()
		timeout := time.After(500 * time.Millisecond)

		for {
			select {
			case event := <-eventChan:
				fmt.Printf("Received event: %s\n", event.EventType())
			case <-timeout:
				return
			}
		}
	}()

	// Give events time to be emitted
	time.Sleep(600 * time.Millisecond)
}

// ExampleMockCoreClient_transactions demonstrates transaction handling
func ExampleMockCoreClient_transactions() {
	mock := NewMockCoreClient()
	ctx := context.Background()
	mock.Start(ctx)
	defer mock.Stop()

	// List recent transactions
	transactions, _ := mock.ListTransactions(5, 0)
	fmt.Printf("Recent transactions: %d\n", len(transactions))

	if len(transactions) > 0 {
		tx := transactions[0]
		fmt.Printf("Type: %s\n", tx.Type)
		fmt.Printf("Confirmations: %d\n", tx.Confirmations)
	}
}

// ExampleMockCoreClient_masternodes demonstrates masternode operations
func ExampleMockCoreClient_masternodes() {
	mock := NewMockCoreClient()
	ctx := context.Background()
	mock.Start(ctx)
	defer mock.Stop()

	// Get masternode count
	count, _ := mock.GetMasternodeCount()
	fmt.Printf("Total Masternodes: %d\n", count.Total)
	fmt.Printf("Enabled: %d\n", count.Enabled)
	fmt.Printf("Tier 1M: %d\n", count.Tier1M)
	fmt.Printf("Tier 5M: %d\n", count.Tier5M)
	fmt.Printf("Tier 20M: %d\n", count.Tier20M)
	fmt.Printf("Tier 100M: %d\n", count.Tier100M)

	// Output:
	// Total Masternodes: 50
	// Enabled: 42
	// Tier 1M: 20
	// Tier 5M: 15
	// Tier 20M: 10
	// Tier 100M: 5
}

// ExampleMockCoreClient_staking demonstrates staking operations
func ExampleMockCoreClient_staking() {
	mock := NewMockCoreClient()
	ctx := context.Background()
	mock.Start(ctx)
	defer mock.Stop()

	// Enable staking
	mock.SetStaking(true)

	// Get staking info
	info, _ := mock.GetStakingInfo()
	fmt.Printf("Staking Enabled: %t\n", info.Enabled)
	fmt.Printf("Currently Staking: %t\n", info.Staking)
	fmt.Printf("WalletUnlocked: %t\n", info.WalletUnlocked)
}
