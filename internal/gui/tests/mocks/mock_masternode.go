package mocks

import (
	"github.com/twins-dev/twins-core/internal/gui/core"
	"fmt"
	"time"
)

// Masternode operation implementations for MockCoreClient

// MasternodeList implements CoreClient.MasternodeList
func (m *MockCoreClient) MasternodeList(filter string) ([]core.MasternodeInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return nil, fmt.Errorf("core is not running")
	}

	// If no filter, return all masternodes
	if filter == "" {
		result := make([]core.MasternodeInfo, len(m.masternodes))
		copy(result, m.masternodes)
		return result, nil
	}

	// Apply filter
	result := make([]core.MasternodeInfo, 0)
	for _, mn := range m.masternodes {
		include := false

		switch filter {
		case "active", "enabled":
			include = mn.Status == "ENABLED"
		case "expired":
			include = mn.Status == "EXPIRED"
		case "pre_enabled":
			include = mn.Status == "PRE_ENABLED"
		case "rank":
			// For "rank" filter, include all and they're already sorted
			include = true
		default:
			// Unknown filter, return all
			include = true
		}

		if include {
			result = append(result, mn)
		}
	}

	return result, nil
}

// MasternodeStart implements CoreClient.MasternodeStart
func (m *MockCoreClient) MasternodeStart(alias string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("core is not running")
	}

	if m.locked {
		return fmt.Errorf("wallet is locked")
	}

	// Safety check for addresses slice
	if len(m.addresses) == 0 {
		return fmt.Errorf("wallet not initialized")
	}

	// Check if this masternode exists in our local config
	status, exists := m.myMasternodes[alias]
	if !exists {
		// Create a new masternode entry
		status = core.MasternodeStatus{
			Status:  "PRE_ENABLED",
			Message: "Masternode successfully started",
			Txhash:  m.generateTxHash(),
			Outidx:  0,
			NetAddr: "0.0.0.0:9340",
			Addr:    m.addresses[0],
			PubKey:  "mock_pubkey_" + alias,
		}
		m.myMasternodes[alias] = status
	} else {
		// Update existing masternode
		status.Status = "PRE_ENABLED"
		status.Message = "Masternode successfully started"
		m.myMasternodes[alias] = status
	}

	// Sync myMasternodeConfigs for UI consistency
	if config, configExists := m.myMasternodeConfigs[alias]; configExists {
		config.Status = "PRE_ENABLED"
		config.ActiveSeconds = 0
		config.LastSeen = time.Now()
		m.myMasternodeConfigs[alias] = config
	}

	// Emit events
	m.emitEventLocked(core.MasternodeStartedEvent{
		BaseEvent: core.BaseEvent{Type: "masternode_started", Time: time.Now()},
		Alias:     alias,
		Txhash:    status.Txhash,
	})

	m.emitEventLocked(core.MasternodeListChangedEvent{
		BaseEvent: core.BaseEvent{Type: "masternode_list_changed", Time: time.Now()},
	})

	return nil
}

// MasternodeStartAll implements CoreClient.MasternodeStartAll
func (m *MockCoreClient) MasternodeStartAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("core is not running")
	}

	if m.locked {
		return fmt.Errorf("wallet is locked")
	}

	// Safety check for addresses slice
	if len(m.addresses) == 0 {
		return fmt.Errorf("wallet not initialized")
	}

	// Start all masternodes in our local config
	startedCount := 0

	// If we have no masternodes configured, create a few mock ones
	if len(m.myMasternodes) == 0 {
		mockAliases := []string{"mn1", "mn2", "mn3"}
		for _, alias := range mockAliases {
			status := core.MasternodeStatus{
				Status:  "PRE_ENABLED",
				Message: "Masternode successfully started",
				Txhash:  m.generateTxHash(),
				Outidx:  0,
				NetAddr: "0.0.0.0:9340",
				Addr:    m.addresses[0],
				PubKey:  "mock_pubkey_" + alias,
			}
			m.myMasternodes[alias] = status
			startedCount++

			m.emitEventLocked(core.MasternodeStartedEvent{
				BaseEvent: core.BaseEvent{Type: "masternode_started", Time: time.Now()},
				Alias:     alias,
				Txhash:    status.Txhash,
			})
		}
	} else {
		// Start all existing masternodes
		for alias, status := range m.myMasternodes {
			status.Status = "PRE_ENABLED"
			status.Message = "Masternode successfully started"
			m.myMasternodes[alias] = status
			startedCount++

			// Sync myMasternodeConfigs for UI consistency
			if config, configExists := m.myMasternodeConfigs[alias]; configExists {
				config.Status = "PRE_ENABLED"
				config.ActiveSeconds = 0
				config.LastSeen = time.Now()
				m.myMasternodeConfigs[alias] = config
			}

			m.emitEventLocked(core.MasternodeStartedEvent{
				BaseEvent: core.BaseEvent{Type: "masternode_started", Time: time.Now()},
				Alias:     alias,
				Txhash:    status.Txhash,
			})
		}
	}

	// Emit list changed event for UI refresh
	if startedCount > 0 {
		m.emitEventLocked(core.MasternodeListChangedEvent{
			BaseEvent: core.BaseEvent{Type: "masternode_list_changed", Time: time.Now()},
		})
	}

	return nil
}

// MasternodeStatus implements CoreClient.MasternodeStatus
func (m *MockCoreClient) MasternodeStatus() (core.MasternodeStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return core.MasternodeStatus{}, fmt.Errorf("core is not running")
	}

	// Return status of the first masternode if any exist
	for _, status := range m.myMasternodes {
		return status, nil
	}

	// No masternodes configured
	return core.MasternodeStatus{
		Status:  "Not capable masternode",
		Message: "No masternode configured",
	}, nil
}

// GetMasternodeCount implements CoreClient.GetMasternodeCount
func (m *MockCoreClient) GetMasternodeCount() (core.MasternodeCount, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return core.MasternodeCount{}, fmt.Errorf("core is not running")
	}

	return m.masternodeCount, nil
}

// MasternodeCurrentWinner implements CoreClient.MasternodeCurrentWinner
func (m *MockCoreClient) MasternodeCurrentWinner() (core.MasternodeInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return core.MasternodeInfo{}, fmt.Errorf("core is not running")
	}

	// Return the top-ranked enabled masternode
	for _, mn := range m.masternodes {
		if mn.Status == "ENABLED" {
			return mn, nil
		}
	}

	return core.MasternodeInfo{}, fmt.Errorf("no enabled masternodes found")
}

// GetMyMasternodes implements CoreClient.GetMyMasternodes
// Returns user's configured masternodes for the UI table
func (m *MockCoreClient) GetMyMasternodes() ([]core.MyMasternode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return nil, fmt.Errorf("core is not running")
	}

	result := make([]core.MyMasternode, 0, len(m.myMasternodeConfigs))
	for _, config := range m.myMasternodeConfigs {
		result = append(result, config)
	}

	return result, nil
}

// MasternodeStartMissing implements CoreClient.MasternodeStartMissing
// Only starts masternodes with MISSING status
func (m *MockCoreClient) MasternodeStartMissing() (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return 0, fmt.Errorf("core is not running")
	}

	if m.locked {
		return 0, fmt.Errorf("wallet is locked")
	}

	// Safety check for addresses slice (consistency with other Start methods)
	if len(m.addresses) == 0 {
		return 0, fmt.Errorf("wallet not initialized")
	}

	startedCount := 0
	for alias, config := range m.myMasternodeConfigs {
		if config.Status == "MISSING" {
			// Update status to PRE_ENABLED (simulating start)
			config.Status = "PRE_ENABLED"
			config.ActiveSeconds = 0
			config.LastSeen = time.Now()
			m.myMasternodeConfigs[alias] = config
			startedCount++

			// Update myMasternodes status map for compatibility
			if status, exists := m.myMasternodes[alias]; exists {
				status.Status = "PRE_ENABLED"
				status.Message = "Masternode successfully started"
				m.myMasternodes[alias] = status
			}

			m.emitEventLocked(core.MasternodeStartedEvent{
				BaseEvent: core.BaseEvent{Type: "masternode_started", Time: time.Now()},
				Alias:     alias,
				Txhash:    config.TxHash,
			})
		}
	}

	// Emit list changed event for UI refresh
	if startedCount > 0 {
		m.emitEventLocked(core.MasternodeListChangedEvent{
			BaseEvent: core.BaseEvent{Type: "masternode_list_changed", Time: time.Now()},
		})
	}

	return startedCount, nil
}
