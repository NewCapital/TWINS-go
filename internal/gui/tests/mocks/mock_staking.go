package mocks

import (
	"fmt"
	"time"

	"github.com/twins-dev/twins-core/internal/gui/core"
)

// Staking operation implementations for MockCoreClient

// GetStakingInfo implements CoreClient.GetStakingInfo
func (m *MockCoreClient) GetStakingInfo() (core.StakingInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return core.StakingInfo{}, fmt.Errorf("core is not running")
	}

	info := core.StakingInfo{
		Enabled:          m.stakingEnabled,
		Staking:          m.stakingActive && !m.locked,
		Errors:           "",
		CurrentBlockSize: int64(m.rng.Intn(100000)) + 50000,
		CurrentBlockTx:   m.rng.Intn(50) + 1,
		PooledTx:         m.rng.Intn(100),
		Difficulty:       m.difficulty,
		SearchInterval:   m.rng.Intn(60) + 1,
	}

	// Add error message if wallet is locked
	if m.stakingEnabled && m.locked {
		info.Errors = "Wallet is locked"
		info.Staking = false
	}

	return info, nil
}

// SetStaking implements CoreClient.SetStaking
func (m *MockCoreClient) SetStaking(enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("core is not running")
	}

	wasEnabled := m.stakingEnabled
	m.stakingEnabled = enabled

	// Update staking active status based on lock state
	if enabled && !m.locked {
		m.stakingActive = true
	} else {
		m.stakingActive = false
	}

	// Emit event if status changed
	if wasEnabled != enabled {
		m.emitEventLocked(core.StakingStatusChangedEvent{
			BaseEvent: core.BaseEvent{Type: "staking_status_changed", Time: time.Now()},
			Enabled:   m.stakingEnabled,
			Staking:   m.stakingActive,
		})
	}

	return nil
}

// GetStakingStatus implements CoreClient.GetStakingStatus
func (m *MockCoreClient) GetStakingStatus() (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return false, fmt.Errorf("core is not running")
	}

	return m.stakingActive && !m.locked, nil
}
