package mocks

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"
	"github.com/twins-dev/twins-core/internal/gui/core"
)

// NetworkCongestion represents the simulated network congestion level
type NetworkCongestion int

const (
	CongestionLow NetworkCongestion = iota
	CongestionNormal
	CongestionHigh
)

// Fee rate constants (TWINS per KB)
const (
	MinRelayFeeRate            = 0.00001 // Minimum to relay transaction
	DefaultFeeRate             = 0.0001  // Conservative default
	FastFeeRate                = 0.001   // High priority
	LowCongestionMultiplier    = 0.5     // 50% of default
	NormalCongestionMultiplier = 1.0     // 100% of default
	HighCongestionMultiplier   = 2.0     // 200% of default
)

// Transaction size constants (bytes)
const (
	TransactionBaseSize   = 10  // Version, locktime, etc.
	TransactionInputSize  = 148 // Previous tx + signature + pubkey
	TransactionOutputSize = 34  // Value + script
)

// MockCoreClient is a mock implementation of CoreClient for development and testing.
// It simulates the TWINS blockchain core with realistic data generation.
type MockCoreClient struct {
	mu sync.RWMutex

	// Lifecycle
	running    bool
	startTime  time.Time
	ctx        context.Context
	cancel     context.CancelFunc
	eventChan  chan core.CoreEvent
	wg         sync.WaitGroup

	// Blockchain state
	currentHeight    int64
	bestBlockHash    string
	blocks           map[string]*core.Block
	blocksByHeight   map[int64]string
	difficulty       float64
	syncProgress     float64
	initialBlockDownload bool

	// Network state
	connectionCount int
	peers           []core.PeerInfo
	networkActive   bool

	// Wallet state
	balance          core.Balance
	addresses        []string
	ownAddresses     map[string]string // address -> pubkey mapping for "isMine" addresses
	transactions     map[string]*core.Transaction
	txList           []string // ordered list of txids
	utxos            []core.UTXO
	lockedCoins      map[string]bool // txid:vout -> locked status
	encrypted        bool
	locked           bool
	unlockedUntil    time.Time

	// Masternode state
	masternodes          []core.MasternodeInfo
	myMasternodes        map[string]core.MasternodeStatus
	myMasternodeConfigs  map[string]core.MyMasternode // User's configured masternodes for UI
	masternodeCount      core.MasternodeCount

	// Staking state
	stakingEnabled   bool
	stakingActive    bool

	// Fee estimation state
	congestionLevel NetworkCongestion

	// Receive page state
	receivingAddresses []core.ReceivingAddress
	paymentRequests    []core.PaymentRequest
	nextPaymentRequestID int64

	// Random number generator with fixed seed for deterministic testing
	rng *rand.Rand
}

// NewMockCoreClient creates a new mock core client.
func NewMockCoreClient() *MockCoreClient {
	// Use a fixed seed for deterministic behavior in tests
	// In production, you might want to use time-based seed for variety
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	mock := &MockCoreClient{
		eventChan:            make(chan core.CoreEvent, 100),
		blocks:               make(map[string]*core.Block),
		blocksByHeight:       make(map[int64]string),
		transactions:         make(map[string]*core.Transaction),
		txList:               make([]string, 0),
		addresses:            make([]string, 0),
		ownAddresses:         make(map[string]string),
		utxos:                make([]core.UTXO, 0),
		lockedCoins:          make(map[string]bool),
		myMasternodes:        make(map[string]core.MasternodeStatus),
		myMasternodeConfigs:  make(map[string]core.MyMasternode),
		peers:                make([]core.PeerInfo, 0),
		rng:                  rng,
		networkActive:        true,
		connectionCount:      8,
		difficulty:           1234.56,
		initialBlockDownload: false,
		congestionLevel:      CongestionNormal,
		receivingAddresses:   make([]core.ReceivingAddress, 0),
		paymentRequests:      make([]core.PaymentRequest, 0),
		nextPaymentRequestID: 1,
	}

	// Initialize with some realistic data
	mock.initializeMockData()

	return mock
}

// initializeMockData sets up initial mock data
func (m *MockCoreClient) initializeMockData() {
	// Generate genesis block and some initial blocks
	m.currentHeight = 1500000 // Reasonable block height for established chain
	m.syncProgress = 1.0

	// Create a few blocks
	m.generateInitialBlocks(10)

	// Initialize wallet balance with realistic values
	// Following the Qt wallet's balance calculation logic from overviewpage.cpp
	spendable := 1000000.0
	pending := 50000.0
	immature := 50000.5
	locked := 200000.0 // Locked in masternode collateral

	// Calculate derived fields
	available := spendable - locked  // Available = Spendable - Locked
	total := spendable + pending + immature

	m.balance = core.Balance{
		Total:     total,
		Available: available,
		Spendable: spendable,
		Pending:   pending,
		Immature:  immature,
		Locked:    locked,
	}

	// Generate some initial addresses with pubkeys
	for i := 0; i < 5; i++ {
		addr := m.generateAddress()
		pubkey := m.generatePubKey()
		m.addresses = append(m.addresses, addr)
		m.ownAddresses[addr] = pubkey
	}

	// Generate some initial transactions
	m.generateInitialTransactions(20)

	// Generate some UTXOs
	m.generateInitialUTXOs(15)

	// Initialize masternodes
	m.initializeMasternodes()

	// Initialize peers
	m.initializePeers()

	// Initialize receiving addresses
	m.initializeReceivingAddresses()
}

// Start implements CoreClient.Start
func (m *MockCoreClient) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("mock core is already running")
	}

	m.ctx, m.cancel = context.WithCancel(ctx)
	m.running = true
	m.startTime = time.Now()

	// Emit initialization events
	m.emitEventLocked(core.InitMessageEvent{
		BaseEvent: core.BaseEvent{Type: "init_message", Time: time.Now()},
		Message:   "Loading block index...",
	})

	m.emitEventLocked(core.InitMessageEvent{
		BaseEvent: core.BaseEvent{Type: "init_message", Time: time.Now()},
		Message:   "Verifying blocks...",
	})

	m.emitEventLocked(core.ShowProgressEvent{
		BaseEvent: core.BaseEvent{Type: "show_progress", Time: time.Now()},
		Title:     "Verifying blocks",
		Progress:  100,
	})

	m.emitEventLocked(core.InitMessageEvent{
		BaseEvent: core.BaseEvent{Type: "init_message", Time: time.Now()},
		Message:   "Loading wallet...",
	})

	// Start background simulation goroutines
	m.wg.Add(4)
	go m.simulateBlockGeneration()
	go m.simulateNetworkActivity()
	go m.simulateStaking()
	go m.simulateCongestionChanges()

	return nil
}

// Stop implements CoreClient.Stop
func (m *MockCoreClient) Stop() error {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return fmt.Errorf("mock core is not running")
	}

	m.running = false
	m.cancel()
	m.mu.Unlock()

	// Wait for all goroutines to finish
	m.wg.Wait()

	// Close event channel
	close(m.eventChan)

	return nil
}

// IsRunning implements CoreClient.IsRunning
func (m *MockCoreClient) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// Events implements CoreClient.Events
func (m *MockCoreClient) Events() <-chan core.CoreEvent {
	return m.eventChan
}

// emitEvent sends an event to the event channel (thread-safe)
func (m *MockCoreClient) emitEvent(event core.CoreEvent) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	m.emitEventLocked(event)
}

// emitEventLocked sends an event without locking (caller must hold lock)
func (m *MockCoreClient) emitEventLocked(event core.CoreEvent) {
	if m.running {
		select {
		case m.eventChan <- event:
		default:
			// Event channel full, drop event
			// Log in debug/development mode for troubleshooting
			log.Printf("WARNING: Dropped event (channel full): type=%s", event.EventType())
		}
	}
}

// simulateBlockGeneration simulates new blocks being added to the chain
func (m *MockCoreClient) simulateBlockGeneration() {
	defer m.wg.Done()

	ticker := time.NewTicker(60 * time.Second) // New block every 60 seconds
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.generateNewBlock()
		}
	}
}

// simulateNetworkActivity simulates network events
func (m *MockCoreClient) simulateNetworkActivity() {
	defer m.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.simulateNetworkChange()
		}
	}
}

// simulateStaking simulates staking rewards
func (m *MockCoreClient) simulateStaking() {
	defer m.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.mu.RLock()
			shouldStake := m.stakingEnabled && m.stakingActive && !m.locked
			m.mu.RUnlock()

			if shouldStake {
				m.generateStakeReward()
			}
		}
	}
}

// simulateCongestionChanges simulates network congestion changes over time
func (m *MockCoreClient) simulateCongestionChanges() {
	defer m.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.mu.Lock()
			// Random congestion level with weighted probabilities
			// 60% normal, 20% low, 20% high
			r := m.rng.Float64()
			switch {
			case r < 0.2:
				m.congestionLevel = CongestionHigh
			case r < 0.4:
				m.congestionLevel = CongestionLow
			default:
				m.congestionLevel = CongestionNormal
			}
			m.mu.Unlock()
		}
	}
}

// generateNewBlock creates a new block and emits events
func (m *MockCoreClient) generateNewBlock() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.currentHeight++
	blockHash := m.generateBlockHash(m.currentHeight)
	m.bestBlockHash = blockHash

	// Create the block
	block := &core.Block{
		Hash:              blockHash,
		Height:            m.currentHeight,
		Confirmations:     1,
		Size:              m.rng.Intn(100000) + 50000,
		Version:           4,
		Time:              time.Now(),
		MedianTime:        time.Now().Add(-5 * time.Minute),
		Difficulty:        m.difficulty,
		Transactions:      make([]core.Transaction, 0),
		PreviousBlockHash: m.blocksByHeight[m.currentHeight-1],
		Flags:             "proof-of-stake",
	}

	m.blocks[blockHash] = block
	m.blocksByHeight[m.currentHeight] = blockHash

	// Update previous block
	if prevHash, ok := m.blocksByHeight[m.currentHeight-1]; ok {
		if prevBlock, ok := m.blocks[prevHash]; ok {
			prevBlock.NextBlockHash = blockHash
			prevBlock.Confirmations++
		}
	}

	// Emit block connected event
	m.emitEventLocked(core.BlockConnectedEvent{
		BaseEvent: core.BaseEvent{Type: "block_connected", Time: time.Now()},
		Hash:      blockHash,
		Height:    m.currentHeight,
		Size:      block.Size,
	})

	// Maybe generate a transaction
	if m.rng.Float64() < 0.3 { // 30% chance of new transaction per block
		m.generateRandomTransaction()
	}
}

// simulateNetworkChange simulates network connection changes
func (m *MockCoreClient) simulateNetworkChange() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Randomly add or remove a connection
	if m.rng.Float64() < 0.5 && m.connectionCount < 125 {
		m.connectionCount++
		m.emitEventLocked(core.ConnectionCountChangedEvent{
			BaseEvent: core.BaseEvent{Type: "connection_count_changed", Time: time.Now()},
			Count:     m.connectionCount,
		})
	} else if m.connectionCount > 1 {
		m.connectionCount--
		m.emitEventLocked(core.ConnectionCountChangedEvent{
			BaseEvent: core.BaseEvent{Type: "connection_count_changed", Time: time.Now()},
			Count:     m.connectionCount,
		})
	}
}

// generateStakeReward generates a staking reward
func (m *MockCoreClient) generateStakeReward() {
	m.mu.Lock()
	defer m.mu.Unlock()

	reward := 100.0 + m.rng.Float64()*50.0 // 100-150 TWINS reward

	// Create stake transaction
	txid := m.generateTxHash()
	tx := &core.Transaction{
		TxID:          txid,
		Amount:        reward,
		Fee:           0.0,
		Confirmations: 1,
		BlockHeight:   m.currentHeight,
		Time:          time.Now(),
		Type:          "stake",
		Category:      "generate",
		Address:       m.addresses[0],
	}

	m.transactions[txid] = tx
	m.txList = append([]string{txid}, m.txList...) // Prepend to list

	// Update balance
	m.balance.Immature += reward
	m.balance.Total += reward

	// Emit events
	m.emitEventLocked(core.StakeRewardEvent{
		BaseEvent: core.BaseEvent{Type: "stake_reward", Time: time.Now()},
		TxID:      txid,
		Amount:    reward,
		Height:    m.currentHeight,
	})

	m.emitEventLocked(core.BalanceChangedEvent{
		BaseEvent: core.BaseEvent{Type: "balance_changed", Time: time.Now()},
		Balance:   m.balance,
	})
}
