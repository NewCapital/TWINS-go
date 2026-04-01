package mocks

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/twins-dev/twins-core/internal/gui/core"
)

// Data generation utilities for MockCoreClient

// generateAddress generates a realistic TWINS address with proper Base58Check encoding
func (m *MockCoreClient) generateAddress() string {
	// TWINS mainnet P2PKH addresses use version byte 30 (0x1E) which gives prefix 'D'
	const versionByte = byte(30)

	// Generate 20 random bytes for the address payload (RIPEMD160 hash size)
	payload := make([]byte, 21)
	payload[0] = versionByte
	for i := 1; i < 21; i++ {
		payload[i] = byte(m.rng.Intn(256))
	}

	// Calculate checksum: first 4 bytes of SHA256(SHA256(payload))
	hash1 := sha256.Sum256(payload)
	hash2 := sha256.Sum256(hash1[:])
	checksum := hash2[:4]

	// Combine payload and checksum
	fullAddress := append(payload, checksum...)

	// Encode to Base58
	return encodeBase58(fullAddress)
}

// Base58 alphabet (excludes 0, O, I, l to avoid confusion)
const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// encodeBase58 encodes a byte slice to Base58 string
func encodeBase58(input []byte) string {
	// Convert bytes to big integer
	x := new(big.Int).SetBytes(input)
	base := big.NewInt(58)
	zero := big.NewInt(0)
	mod := new(big.Int)

	var result []byte
	for x.Cmp(zero) > 0 {
		x.DivMod(x, base, mod)
		result = append([]byte{base58Alphabet[mod.Int64()]}, result...)
	}

	// Add leading '1's for each leading zero byte
	for _, b := range input {
		if b != 0 {
			break
		}
		result = append([]byte{'1'}, result...)
	}

	return string(result)
}

// generatePubKey generates a mock public key (compressed SECP256k1 format)
func (m *MockCoreClient) generatePubKey() string {
	// Compressed public keys are 33 bytes (0x02/0x03 prefix + 32 bytes X coordinate)
	// Return as hex string (66 characters)
	pubkey := make([]byte, 33)

	// First byte is 0x02 or 0x03 (compressed format indicator)
	if m.rng.Intn(2) == 0 {
		pubkey[0] = 0x02
	} else {
		pubkey[0] = 0x03
	}

	// Remaining 32 bytes are the X coordinate
	for i := 1; i < 33; i++ {
		pubkey[i] = byte(m.rng.Intn(256))
	}

	return hex.EncodeToString(pubkey)
}

// generateTxHash generates a mock transaction hash
func (m *MockCoreClient) generateTxHash() string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("tx_%d_%d", time.Now().UnixNano(), m.rng.Int())))
	return hex.EncodeToString(hash[:])
}

// generateBlockHash generates a mock block hash
func (m *MockCoreClient) generateBlockHash(height int64) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("block_%d_%d", height, time.Now().UnixNano())))
	return hex.EncodeToString(hash[:])
}

// generateInitialBlocks creates the first few blocks
func (m *MockCoreClient) generateInitialBlocks(count int) {
	startHeight := m.currentHeight - int64(count) + 1
	if startHeight < 0 {
		startHeight = 0
	}

	var prevHash string
	for i := int64(0); i < int64(count); i++ {
		height := startHeight + i
		blockHash := m.generateBlockHash(height)

		block := &core.Block{
			Hash:              blockHash,
			Height:            height,
			Confirmations:     int(m.currentHeight - height + 1),
			Size:              m.rng.Intn(100000) + 50000,
			Version:           4,
			Time:              time.Now().Add(-time.Duration(count-int(i)) * time.Hour),
			MedianTime:        time.Now().Add(-time.Duration(count-int(i)+1) * time.Hour),
			Difficulty:        m.difficulty,
			Transactions:      make([]core.Transaction, 0),
			PreviousBlockHash: prevHash,
			Flags:             "proof-of-stake",
		}

		m.blocks[blockHash] = block
		m.blocksByHeight[height] = blockHash

		// Update previous block's next hash
		if prevHash != "" {
			if prevBlock, ok := m.blocks[prevHash]; ok {
				prevBlock.NextBlockHash = blockHash
			}
		}

		prevHash = blockHash
	}

	m.bestBlockHash = prevHash
}

// generateInitialTransactions creates some initial transaction history
func (m *MockCoreClient) generateInitialTransactions(count int) {
	// Weighted selection - more common types appear more frequently
	// Transaction types with weighted probability for realistic distribution
	weightedTypes := []core.TransactionType{
		core.TxTypeRecvWithAddress, core.TxTypeRecvWithAddress, core.TxTypeRecvWithAddress,
		core.TxTypeSendToAddress, core.TxTypeSendToAddress, core.TxTypeSendToAddress,
		core.TxTypeStakeMint, core.TxTypeStakeMint,
		core.TxTypeMNReward, core.TxTypeMNReward,
		core.TxTypeSendToOther, core.TxTypeRecvFromOther,
		core.TxTypeSendToSelf,
		core.TxTypeGenerated,
		core.TxTypeOther,
	}

	// Maturity threshold for stake/generated transactions (from chainparams.cpp nMaturity = 60)
	const maturityThreshold = 60

	// Number of blocks generated in mock initialization (see mock.go generateInitialBlocks)
	// Cap confirmations to this range to ensure valid block hashes
	const maxConfirmations = 10

	categories := map[core.TransactionType]string{
		core.TxTypeRecvWithAddress:               "receive",
		core.TxTypeSendToAddress:                 "send",
		core.TxTypeStakeMint:                     "generate",
		core.TxTypeMNReward:                      "masternode",
		core.TxTypeGenerated:                     "generate",
		core.TxTypeSendToOther:                   "send",
		core.TxTypeRecvFromOther:                 "receive",
		core.TxTypeSendToSelf:                    "send",
		core.TxTypeOther:                         "other",
	}

	// Labels for address book entries
	addressLabels := []string{
		"Main Wallet", "Savings Account", "Trading", "Exchange Deposit",
		"Cold Storage", "Staking Rewards", "Alice", "Bob", "Charlie",
		"Payment from Store", "Refund", "",
	}

	// Comments/messages for transactions
	txComments := []string{
		"Payment for services", "Monthly subscription", "Gift from friend",
		"Staking reward", "Masternode payment", "Exchange withdrawal",
		"Invoice #12345", "Thank you!", "Refund for order",
		"Salary payment", "Coffee money", "",
	}

	for i := 0; i < count; i++ {
		txid := m.generateTxHash()
		// Use weighted types for more realistic distribution
		txType := weightedTypes[m.rng.Intn(len(weightedTypes))]

		var amount float64
		// Determine amount based on transaction type
		switch txType {
		case core.TxTypeSendToAddress, core.TxTypeSendToOther:
			amount = -(m.rng.Float64() * 10000.0) // Negative for sends
		case core.TxTypeSendToSelf:
			amount = 0 // Self-sends net to zero (minus fee)
		case core.TxTypeStakeMint:
			amount = 38.0 + m.rng.Float64()*2.0 // Stake rewards around 38-40 TWINS
		case core.TxTypeMNReward:
			amount = 11.4 + m.rng.Float64()*0.6 // MN rewards around 11.4-12 TWINS
		case core.TxTypeGenerated:
			amount = 50.0 // PoW block reward
		case core.TxTypeOther:
			// Random small amount for "other" type
			amount = m.rng.Float64() * 100.0
		default:
			amount = m.rng.Float64() * 5000.0 // Positive for receives
		}

		// Vary confirmations to show different status icons
		// -1 = conflicted, 0 = unconfirmed, 1-5 = confirming (clock icons), 6+ = confirmed
		var confirmations int
		confRand := m.rng.Float64()
		switch {
		case confRand < 0.02: // 2% conflicted (negative confirmations)
			confirmations = -1
		case confRand < 0.12: // 10% unconfirmed
			confirmations = 0
		case confRand < 0.32: // 20% confirming (1-5)
			confirmations = m.rng.Intn(5) + 1
		case confRand < 0.37: // 5% immature (for generated/staked, needs 60 confirmations per chainparams.cpp)
			// Note: In mock, we use smaller range since we only generate maxConfirmations blocks
			// Real wallet would show 1-59 as immature; here we simulate with available range
			confirmations = m.rng.Intn(min(maturityThreshold-1, maxConfirmations-1)) + 1
		default: // 63% fully confirmed
			// Cap to maxConfirmations to ensure valid block hash lookups
			confirmations = m.rng.Intn(maxConfirmations-5) + 6 // 6 to maxConfirmations
		}

		// For non-stake/generated transactions, don't use immature range
		isStakeOrGenerated := txType == core.TxTypeStakeMint ||
			txType == core.TxTypeGenerated || txType == core.TxTypeMNReward
		if !isStakeOrGenerated && confirmations > 5 && confirmations < maturityThreshold {
			confirmations = m.rng.Intn(maxConfirmations-5) + 6 // Regular confirmed range
		}

		blockHeight := m.currentHeight - int64(confirmations)
		if confirmations <= 0 {
			blockHeight = 0 // Unconfirmed/conflicted transactions have no block
		}

		// Get block hash with bounds checking
		var blockHash string
		if blockHeight > 0 {
			blockHash = m.blocksByHeight[blockHeight]
		}

		// Assign label (60% chance)
		var label string
		if m.rng.Float64() < 0.6 {
			label = addressLabels[m.rng.Intn(len(addressLabels))]
		}

		// Assign comment (40% chance)
		var comment string
		if m.rng.Float64() < 0.4 {
			comment = txComments[m.rng.Intn(len(txComments))]
		}

		// Calculate fee based on transaction type
		var fee float64
		if amount < 0 { // Send transactions have fees
			fee = 0.0001 + m.rng.Float64()*0.001 // 0.0001 to 0.0011 TWINS
		}

		// Watch-only flag (5% of receive transactions)
		isWatchOnly := false
		if amount > 0 && m.rng.Float64() < 0.05 {
			isWatchOnly = true
		}

		// Determine category - use "immature" for immature stake/generated
		category := categories[txType]
		if isStakeOrGenerated && confirmations > 0 && confirmations < maturityThreshold {
			category = "immature"
		}
		// Use "orphan" category for conflicted transactions
		if confirmations < 0 {
			category = "orphan"
		}

		// Calculate time offset (use 0 for negative confirmations)
		timeOffset := confirmations
		if timeOffset < 0 {
			timeOffset = 0
		}

		tx := &core.Transaction{
			TxID:          txid,
			Amount:        amount,
			Fee:           fee,
			Confirmations: confirmations,
			BlockHeight:   blockHeight,
			BlockHash:     blockHash,
			Time:          time.Now().Add(-time.Duration(timeOffset) * time.Minute),
			Type:          txType,
			Category:      category,
			Address:       m.addresses[m.rng.Intn(len(m.addresses))],
			Label:         label,
			Comment:       comment,
			IsWatchOnly:   isWatchOnly,
		}

		m.transactions[txid] = tx
		m.txList = append([]string{txid}, m.txList...) // Prepend
	}
}

// generateInitialUTXOs creates some initial unspent outputs
// This generates realistic UTXO data for coin control dialog testing
func (m *MockCoreClient) generateInitialUTXOs(count int) {
	// Labels for wallet addresses
	addressLabels := []string{"Main Wallet", "Savings", "Trading", "Staking", ""}

	// Current time for date calculations
	now := time.Now().Unix()

	for i := 0; i < count; i++ {
		address := m.addresses[m.rng.Intn(len(m.addresses))]
		confirmations := m.rng.Intn(5000) + 1 // 1 to 5000 confirmations

		// Vary amounts realistically:
		// - Some small amounts (dust-ish)
		// - Some medium amounts
		// - Some large amounts (masternode collateral-sized)
		var amount float64
		switch m.rng.Intn(10) {
		case 0: // 10% small amounts
			amount = m.rng.Float64() * 100.0
		case 1, 2, 3: // 30% medium amounts
			amount = m.rng.Float64() * 10000.0
		case 4, 5, 6, 7: // 40% larger amounts
			amount = m.rng.Float64() * 100000.0
		case 8: // 10% staking rewards sized (~38-40 TWINS)
			amount = 38.0 + m.rng.Float64()*2.0
		case 9: // 10% masternode rewards sized (~11.4-12 TWINS)
			amount = 11.4 + m.rng.Float64()*0.6
		}

		// Calculate priority as (amount * confirmations) / estimated_input_size
		// Simplified: amount * confirmations / 148 (typical input size)
		priority := (amount * float64(confirmations)) / 148.0

		// Determine if coin should be locked (10% chance)
		locked := m.rng.Float64() < 0.1

		// Assign a label with 70% probability
		var label string
		if m.rng.Float64() < 0.7 {
			label = addressLabels[m.rng.Intn(len(addressLabels))]
		}

		// Generate a realistic date (between 1 day and 1 year ago)
		daysAgo := m.rng.Intn(365) + 1
		date := now - int64(daysAgo*24*60*60)

		// UTXO type (mostly Personal, some MultiSig)
		utxoType := "Personal"
		if m.rng.Float64() < 0.1 {
			utxoType = "MultiSig"
		}

		utxo := core.UTXO{
			TxID:          m.generateTxHash(),
			Vout:          uint32(m.rng.Intn(10)),
			Address:       address,
			Label:         label,
			ScriptPubKey:  hex.EncodeToString([]byte(fmt.Sprintf("script_%d", i))),
			Amount:        amount,
			Confirmations: confirmations,
			Spendable:     !locked, // Locked coins are not spendable
			Solvable:      true,
			Locked:        locked,
			Type:          utxoType,
			Date:          date,
			Priority:      priority,
		}
		m.utxos = append(m.utxos, utxo)

		// If locked, add to locked coins map
		if locked {
			key := fmt.Sprintf("%s:%d", utxo.TxID, utxo.Vout)
			m.lockedCoins[key] = true
		}
	}
}

// generateRandomTransaction creates a random transaction and emits events
func (m *MockCoreClient) generateRandomTransaction() {
	// Generate a receive transaction (most common for wallet)
	txid := m.generateTxHash()
	amount := m.rng.Float64() * 1000.0

	tx := &core.Transaction{
		TxID:          txid,
		Amount:        amount,
		Fee:           0.001,
		Confirmations: 0,
		BlockHeight:   m.currentHeight,
		BlockHash:     m.bestBlockHash,
		Time:          time.Now(),
		Type:          "receive",
		Category:      "receive",
		Address:       m.addresses[m.rng.Intn(len(m.addresses))],
	}

	m.transactions[txid] = tx
	m.txList = append([]string{txid}, m.txList...)

	// Update balance
	m.balance.Pending += amount
	m.balance.Total += amount

	// Emit events
	m.emitEventLocked(core.TransactionReceivedEvent{
		BaseEvent:     core.BaseEvent{Type: "transaction_received", Time: time.Now()},
		TxID:          txid,
		Amount:        amount,
		Confirmations: 0,
	})

	m.emitEventLocked(core.BalanceChangedEvent{
		BaseEvent: core.BaseEvent{Type: "balance_changed", Time: time.Now()},
		Balance:   m.balance,
	})
}

// initializeMasternodes creates initial masternode data
func (m *MockCoreClient) initializeMasternodes() {
	tiers := []string{"1M", "5M", "20M", "100M"}
	statuses := []string{"ENABLED", "ENABLED", "ENABLED", "PRE_ENABLED", "EXPIRED"}

	// Create 50 network masternodes
	for i := 0; i < 50; i++ {
		tier := tiers[m.rng.Intn(len(tiers))]
		status := statuses[m.rng.Intn(len(statuses))]

		mn := core.MasternodeInfo{
			Rank:           i + 1,
			Txhash:         m.generateTxHash(),
			Outidx:         0,
			Status:         status,
			Address:        fmt.Sprintf("45.%d.%d.%d:9340", m.rng.Intn(256), m.rng.Intn(256), m.rng.Intn(256)),
			Version:        70922,
			LastSeen:       time.Now().Add(-time.Duration(m.rng.Intn(3600)) * time.Second),
			ActiveTime:     int64(m.rng.Intn(10000000)),
			LastPaid:       time.Now().Add(-time.Duration(m.rng.Intn(86400)) * time.Second),
			Tier:           tier,
			PaymentAddress: m.generateAddress(),
			PubKey:         hex.EncodeToString([]byte(fmt.Sprintf("pubkey_%d", i))),
		}

		m.masternodes = append(m.masternodes, mn)
	}

	// Update masternode count
	m.masternodeCount = core.MasternodeCount{
		Total:    50,
		Enabled:  42,
		InQueue:  42,
		Ipv4:     45,
		Ipv6:     3,
		Onion:    2,
		Tier1M:   20,
		Tier5M:   15,
		Tier20M:  10,
		Tier100M: 5,
	}

	// Initialize user's configured masternodes (from masternode.conf)
	m.initializeMyMasternodes()
}

// initializeMyMasternodes creates mock user-configured masternodes for the UI table
// These represent entries that would be in masternode.conf
func (m *MockCoreClient) initializeMyMasternodes() {
	// Realistic masternode configurations with varied statuses
	myMNConfigs := []struct {
		alias    string
		status   string
		ip       string
		active   int64 // seconds
		lastSeen int   // seconds ago
	}{
		{"mn1", "ENABLED", "45.76.123.45:9340", 8640000, 120},      // ~100 days active
		{"masternode-silver", "ENABLED", "95.179.200.12:9340", 2592000, 45},  // ~30 days
		{"gold-node-1", "MISSING", "149.28.45.89:9340", 0, 86400},  // Missing, last seen 1 day ago
		{"mn-backup", "PRE_ENABLED", "207.148.16.78:9340", 300, 30}, // Just started
		{"copper-mn", "EXPIRED", "45.32.188.201:9340", 5184000, 172800}, // Expired, 60 days active before
	}

	for _, cfg := range myMNConfigs {
		txHash := m.generateTxHash()
		pubkey := m.generatePubKey()

		myMN := core.MyMasternode{
			Alias:         cfg.alias,
			Address:       cfg.ip,
			Protocol:      70922,
			Status:        cfg.status,
			ActiveSeconds: cfg.active,
			LastSeen:      time.Now().Add(-time.Duration(cfg.lastSeen) * time.Second),
			CollateralAddress: pubkey,
			TxHash:        txHash,
			OutputIndex:   0,
		}
		m.myMasternodeConfigs[cfg.alias] = myMN

		// Also add to myMasternodes for compatibility with existing MasternodeStart
		m.myMasternodes[cfg.alias] = core.MasternodeStatus{
			Status:  cfg.status,
			Message: "",
			Txhash:  txHash,
			Outidx:  0,
			NetAddr: cfg.ip,
			Addr:    m.addresses[0],
			PubKey:  pubkey,
		}
	}
}

// initializePeers creates initial peer connections
func (m *MockCoreClient) initializePeers() {
	for i := 0; i < m.connectionCount; i++ {
		peer := core.PeerInfo{
			ID:             i + 1,
			Address:        fmt.Sprintf("45.%d.%d.%d:9340", m.rng.Intn(256), m.rng.Intn(256), m.rng.Intn(256)),
			AddressLocal:   fmt.Sprintf("192.168.1.%d:9340", m.rng.Intn(256)),
			Services:       "000000000000040d",
			LastSend:       time.Now().Add(-time.Duration(m.rng.Intn(60)) * time.Second),
			LastRecv:       time.Now().Add(-time.Duration(m.rng.Intn(60)) * time.Second),
			BytesSent:      uint64(m.rng.Intn(10000000)),
			BytesRecv:      uint64(m.rng.Intn(10000000)),
			ConnTime:       time.Now().Add(-time.Duration(m.rng.Intn(7200)) * time.Second),
			TimeOffset:     m.rng.Intn(10) - 5,
			PingTime:       m.rng.Float64() * 0.5,
			MinPing:        m.rng.Float64() * 0.2,
			Version:        70922,
			SubVer:         "/TWINS:2.4.0/",
			Inbound:        m.rng.Float64() < 0.5,
			StartingHeight: m.currentHeight - int64(m.rng.Intn(100)),
			BanScore:       0,
			SyncedHeaders:    m.currentHeight,
			SyncedBlocks:     m.currentHeight,
			LastHeaderUpdate: time.Now().Unix(),
			WhiteListed:      false,
		}
		m.peers = append(m.peers, peer)
	}
}
