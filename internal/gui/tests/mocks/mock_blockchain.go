package mocks

import (
	"github.com/twins-dev/twins-core/internal/gui/core"
	"fmt"
	"time"
)

// Blockchain operation implementations for MockCoreClient

// GetBlockchainInfo implements CoreClient.GetBlockchainInfo
func (m *MockCoreClient) GetBlockchainInfo() (core.BlockchainInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return core.BlockchainInfo{}, fmt.Errorf("core is not running")
	}

	// Calculate sync status
	// For mock, we can simulate being either synced or out of sync
	// Default: fully synced (syncProgress = 1.0, behindBlocks = 0)
	isSyncing := m.initialBlockDownload || m.syncProgress < 1.0
	var behindBlocks int64 = 0
	var behindTime string = "up to date"
	var isOutOfSync bool = false
	var syncPercentage float64 = m.syncProgress * 100.0

	// If in initial block download or not fully synced, calculate behind time
	if isSyncing {
		// Assume network is at height 1550000, we're at 1500000
		networkHeight := int64(1550000)
		behindBlocks = networkHeight - m.currentHeight
		isOutOfSync = behindBlocks > 6 // Out of sync if more than 6 blocks behind

		// Calculate human-readable behind time
		// Approximate: 1 block per minute
		behindMinutes := behindBlocks
		if behindMinutes < 60 {
			behindTime = fmt.Sprintf("%d minutes behind", behindMinutes)
		} else if behindMinutes < 1440 {
			hours := behindMinutes / 60
			behindTime = fmt.Sprintf("%d hours behind", hours)
		} else if behindMinutes < 10080 {
			days := behindMinutes / 1440
			behindTime = fmt.Sprintf("%d days behind", days)
		} else {
			weeks := behindMinutes / 10080
			behindTime = fmt.Sprintf("%d weeks behind", weeks)
		}
	}

	info := core.BlockchainInfo{
		Chain:                "main",
		Blocks:               m.currentHeight,
		Headers:              m.currentHeight,
		BestBlockHash:        m.bestBlockHash,
		Difficulty:           m.difficulty,
		MedianTime:           time.Now().Add(-5 * time.Minute),
		VerificationProgress: m.syncProgress,
		ChainWork:            "00000000000000000000000000000000000000000000000000abcdef1234567890",
		Pruned:               false,
		PruneHeight:          0,
		InitialBlockDownload: m.initialBlockDownload,
		SizeOnDisk:           uint64(m.currentHeight * 50000), // Rough estimate

		// Sync Status Fields
		IsSyncing:        isSyncing,
		IsOutOfSync:      isOutOfSync,
		BehindBlocks:     behindBlocks,
		BehindTime:       behindTime,
		SyncPercentage:   syncPercentage,
		CurrentBlockScan: m.currentHeight, // Currently scanning/validating this block
	}

	return info, nil
}

// GetNetworkInfo implements CoreClient.GetNetworkInfo
func (m *MockCoreClient) GetNetworkInfo() (core.NetworkInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return core.NetworkInfo{}, fmt.Errorf("core is not running")
	}

	info := core.NetworkInfo{
		Version:         70922,
		Subversion:      "/TWINS:2.4.0/",
		ProtocolVersion: 70922,
		LocalServices:   "000000000000040d",
		LocalRelay:      true,
		TimeOffset:      0,
		Connections:     m.connectionCount,
		NetworkActive:   m.networkActive,
		Networks: []core.NetworkType{
			{Name: "ipv4", Limited: false, Reachable: true, Proxy: ""},
			{Name: "ipv6", Limited: false, Reachable: true, Proxy: ""},
			{Name: "onion", Limited: false, Reachable: false, Proxy: ""},
		},
		RelayFee: 0.001,
		LocalAddresses: []core.LocalAddress{
			{Address: "192.168.1.100", Port: 9340, Score: 1},
		},
		Warnings: "",
	}

	return info, nil
}

// GetBlock implements CoreClient.GetBlock
func (m *MockCoreClient) GetBlock(hash string) (core.Block, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return core.Block{}, fmt.Errorf("core is not running")
	}

	block, ok := m.blocks[hash]
	if !ok {
		return core.Block{}, fmt.Errorf("block not found: %s", hash)
	}

	return *block, nil
}

// GetBlockHash implements CoreClient.GetBlockHash
func (m *MockCoreClient) GetBlockHash(height int64) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return "", fmt.Errorf("core is not running")
	}

	if height < 0 || height > m.currentHeight {
		return "", fmt.Errorf("block height out of range: %d", height)
	}

	hash, ok := m.blocksByHeight[height]
	if !ok {
		// Generate on-the-fly if not in memory
		hash = m.generateBlockHash(height)
	}

	return hash, nil
}

// GetBlockCount implements CoreClient.GetBlockCount
func (m *MockCoreClient) GetBlockCount() (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return 0, fmt.Errorf("core is not running")
	}

	return m.currentHeight, nil
}

// GetPeerInfo implements CoreClient.GetPeerInfo
func (m *MockCoreClient) GetPeerInfo() ([]core.PeerInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return nil, fmt.Errorf("core is not running")
	}

	// Return a copy to prevent modification
	result := make([]core.PeerInfo, len(m.peers))
	copy(result, m.peers)

	return result, nil
}

// GetConnectionCount implements CoreClient.GetConnectionCount
func (m *MockCoreClient) GetConnectionCount() (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return 0, fmt.Errorf("core is not running")
	}

	return m.connectionCount, nil
}

// GetInfo implements CoreClient.GetInfo
func (m *MockCoreClient) GetInfo() (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return nil, fmt.Errorf("core is not running")
	}

	info := map[string]interface{}{
		"version":         70922,
		"protocolversion": 70922,
		"walletversion":   169900,
		"balance":         m.balance.Total,
		"blocks":          m.currentHeight,
		"timeoffset":      0,
		"connections":     m.connectionCount,
		"proxy":           "",
		"difficulty":      m.difficulty,
		"testnet":         false,
		"keypoololdest":   time.Now().Add(-365 * 24 * time.Hour).Unix(),
		"keypoolsize":     1000,
		"paytxfee":        0.001,
		"relayfee":        0.001,
		"staking status":  m.stakingActive,
		"errors":          "",
	}

	return info, nil
}

// AddNode implements CoreClient.AddNode
func (m *MockCoreClient) AddNode(node string, command string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("core is not running")
	}

	if command != "add" && command != "remove" && command != "onetry" {
		return fmt.Errorf("invalid command: %s (must be add, remove, or onetry)", command)
	}

	// In a real implementation, this would manage peer connections
	// For mock, we just simulate success
	return nil
}

// DisconnectNode implements CoreClient.DisconnectNode
func (m *MockCoreClient) DisconnectNode(address string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("core is not running")
	}

	// In a real implementation, this would disconnect from a specific peer
	// For mock, we just simulate success
	return nil
}

// GetAddedNodeInfo implements CoreClient.GetAddedNodeInfo
func (m *MockCoreClient) GetAddedNodeInfo(node string) ([]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return nil, fmt.Errorf("core is not running")
	}

	// Return empty list for mock
	return []interface{}{}, nil
}

// SetNetworkActive implements CoreClient.SetNetworkActive
func (m *MockCoreClient) SetNetworkActive(active bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("core is not running")
	}

	wasActive := m.networkActive
	m.networkActive = active

	if wasActive != active {
		m.emitEventLocked(core.NetworkActiveChangedEvent{
			BaseEvent: core.BaseEvent{Type: "network_active_changed", Time: time.Now()},
			Active:    active,
		})
	}

	return nil
}

// InvalidateBlock implements CoreClient.InvalidateBlock
func (m *MockCoreClient) InvalidateBlock(hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("core is not running")
	}

	_, ok := m.blocks[hash]
	if !ok {
		return fmt.Errorf("block not found: %s", hash)
	}

	// In a real implementation, this would mark the block as invalid
	// and trigger a chain reorganization. For mock, we just acknowledge it.
	return nil
}

// ReconsiderBlock implements CoreClient.ReconsiderBlock
func (m *MockCoreClient) ReconsiderBlock(hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("core is not running")
	}

	_, ok := m.blocks[hash]
	if !ok {
		return fmt.Errorf("block not found: %s", hash)
	}

	// In a real implementation, this would remove the invalid status
	// For mock, we just acknowledge it.
	return nil
}

// VerifyChain implements CoreClient.VerifyChain
func (m *MockCoreClient) VerifyChain(checkLevel int, numBlocks int) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return false, fmt.Errorf("core is not running")
	}

	// Validate parameters
	if checkLevel < 0 || checkLevel > 4 {
		return false, fmt.Errorf("invalid check level: %d (must be 0-4)", checkLevel)
	}

	// In a real implementation, this would verify the blockchain
	// For mock, we always return success after a brief delay
	time.Sleep(100 * time.Millisecond)

	return true, nil
}
