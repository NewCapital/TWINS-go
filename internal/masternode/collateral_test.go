// Copyright (c) 2018-2025 The TWINS developers
// Distributed under the MIT/X11 software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package masternode

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twins-dev/twins-core/pkg/types"
)

func TestGetTierFromCollateral(t *testing.T) {
	tests := []struct {
		name     string
		amount   int64
		wantTier MasternodeTier
		wantErr  bool
	}{
		{"Bronze", TierBronzeCollateral, Bronze, false},
		{"Silver", TierSilverCollateral, Silver, false},
		{"Gold", TierGoldCollateral, Gold, false},
		{"Platinum", TierPlatinumCollateral, Platinum, false},
		{"Invalid zero", 0, Bronze, true},
		{"Invalid small", 100000 * 1e8, Bronze, true},
		{"Invalid between tiers", 3000000 * 1e8, Bronze, true},
		{"Invalid large", 200000000 * 1e8, Bronze, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tier, err := GetTierFromCollateral(tt.amount)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantTier, tier)
			}
		})
	}
}

func TestValidateCollateral_Success(t *testing.T) {
	testHash, _ := types.NewHashFromString("0000000000000000000000000000000000000000000000000000000000000001")
	testOutpoint := types.Outpoint{
		Hash:  testHash,
		Index: 0,
	}
	testBlockHash, _ := types.NewHashFromString("0000000000000000000000000000000000000000000000000000000000000002")

	cv := &CollateralValidator{
		GetUTXO: func(outpoint types.Outpoint) (*types.TxOutput, error) {
			return &types.TxOutput{
				Value: TierSilverCollateral, // 5M TWINS
			}, nil
		},
		GetBestHeight: func() (uint32, error) {
			return 1000, nil
		},
		GetBlockHeightByHash: func(hash types.Hash) (uint32, error) {
			return 980, nil // 20 confirmations
		},
		GetTransactionBlockHash: func(txHash types.Hash) (types.Hash, error) {
			return testBlockHash, nil
		},
		// Enable multi-tier support for this test
		IsMultiTierEnabled: func() bool { return true },
	}

	info, err := cv.ValidateCollateral(testOutpoint)
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, int64(TierSilverCollateral), info.Amount)
	assert.Equal(t, Silver, info.Tier)
	assert.Equal(t, uint32(21), info.Confirmations) // 1000 - 980 + 1 = 21
}

func TestValidateCollateral_UTXONotFound(t *testing.T) {
	testHash, _ := types.NewHashFromString("0000000000000000000000000000000000000000000000000000000000000001")
	testOutpoint := types.Outpoint{
		Hash:  testHash,
		Index: 0,
	}

	cv := &CollateralValidator{
		GetUTXO: func(outpoint types.Outpoint) (*types.TxOutput, error) {
			return nil, errors.New("UTXO not found")
		},
		GetBestHeight: func() (uint32, error) {
			return 1000, nil
		},
	}

	info, err := cv.ValidateCollateral(testOutpoint)
	assert.Error(t, err)
	assert.Nil(t, info)
	assert.ErrorIs(t, err, ErrCollateralNotFound)
}

func TestValidateCollateral_UTXOSpent(t *testing.T) {
	testHash, _ := types.NewHashFromString("0000000000000000000000000000000000000000000000000000000000000001")
	testOutpoint := types.Outpoint{
		Hash:  testHash,
		Index: 0,
	}

	cv := &CollateralValidator{
		GetUTXO: func(outpoint types.Outpoint) (*types.TxOutput, error) {
			return nil, nil // nil UTXO means spent
		},
		GetBestHeight: func() (uint32, error) {
			return 1000, nil
		},
	}

	info, err := cv.ValidateCollateral(testOutpoint)
	assert.Error(t, err)
	assert.Nil(t, info)
	assert.ErrorIs(t, err, ErrCollateralSpent)
}

func TestValidateCollateral_InvalidTier(t *testing.T) {
	testHash, _ := types.NewHashFromString("0000000000000000000000000000000000000000000000000000000000000001")
	testOutpoint := types.Outpoint{
		Hash:  testHash,
		Index: 0,
	}

	cv := &CollateralValidator{
		GetUTXO: func(outpoint types.Outpoint) (*types.TxOutput, error) {
			return &types.TxOutput{
				Value: 500000 * 1e8, // Invalid amount (500K TWINS)
			}, nil
		},
		GetBestHeight: func() (uint32, error) {
			return 1000, nil
		},
	}

	info, err := cv.ValidateCollateral(testOutpoint)
	assert.Error(t, err)
	assert.Nil(t, info)
	assert.ErrorIs(t, err, ErrCollateralInvalidTier)
}

func TestValidateCollateral_InsufficientConfirmations(t *testing.T) {
	testHash, _ := types.NewHashFromString("0000000000000000000000000000000000000000000000000000000000000001")
	testOutpoint := types.Outpoint{
		Hash:  testHash,
		Index: 0,
	}
	testBlockHash, _ := types.NewHashFromString("0000000000000000000000000000000000000000000000000000000000000002")

	cv := &CollateralValidator{
		GetUTXO: func(outpoint types.Outpoint) (*types.TxOutput, error) {
			return &types.TxOutput{
				Value: TierBronzeCollateral,
			}, nil
		},
		GetBestHeight: func() (uint32, error) {
			return 1000, nil
		},
		GetBlockHeightByHash: func(hash types.Hash) (uint32, error) {
			return 995, nil // Only 6 confirmations (1000 - 995 + 1 = 6)
		},
		GetTransactionBlockHash: func(txHash types.Hash) (types.Hash, error) {
			return testBlockHash, nil
		},
	}

	info, err := cv.ValidateCollateral(testOutpoint)
	assert.Error(t, err)
	assert.Nil(t, info)
	assert.ErrorIs(t, err, ErrCollateralLowConf)
}

func TestValidateCollateral_AllTiers(t *testing.T) {
	testBlockHash, _ := types.NewHashFromString("0000000000000000000000000000000000000000000000000000000000000002")

	tiers := []struct {
		name   string
		amount int64
		tier   MasternodeTier
	}{
		{"Bronze", TierBronzeCollateral, Bronze},
		{"Silver", TierSilverCollateral, Silver},
		{"Gold", TierGoldCollateral, Gold},
		{"Platinum", TierPlatinumCollateral, Platinum},
	}

	for _, tt := range tiers {
		t.Run(tt.name, func(t *testing.T) {
			testHash, _ := types.NewHashFromString("0000000000000000000000000000000000000000000000000000000000000001")
			testOutpoint := types.Outpoint{
				Hash:  testHash,
				Index: 0,
			}

			cv := &CollateralValidator{
				GetUTXO: func(outpoint types.Outpoint) (*types.TxOutput, error) {
					return &types.TxOutput{
						Value: tt.amount,
					}, nil
				},
				GetBestHeight: func() (uint32, error) {
					return 1000, nil
				},
				GetBlockHeightByHash: func(hash types.Hash) (uint32, error) {
					return 900, nil // 101 confirmations
				},
				GetTransactionBlockHash: func(txHash types.Hash) (types.Hash, error) {
					return testBlockHash, nil
				},
				// Enable multi-tier support for this test
				IsMultiTierEnabled: func() bool { return true },
			}

			info, err := cv.ValidateCollateral(testOutpoint)
			require.NoError(t, err)
			require.NotNil(t, info)
			assert.Equal(t, tt.amount, info.Amount)
			assert.Equal(t, tt.tier, info.Tier)
		})
	}
}

func TestValidateCollateral_ExactMinConfirmations(t *testing.T) {
	testHash, _ := types.NewHashFromString("0000000000000000000000000000000000000000000000000000000000000001")
	testOutpoint := types.Outpoint{
		Hash:  testHash,
		Index: 0,
	}
	testBlockHash, _ := types.NewHashFromString("0000000000000000000000000000000000000000000000000000000000000002")

	cv := &CollateralValidator{
		GetUTXO: func(outpoint types.Outpoint) (*types.TxOutput, error) {
			return &types.TxOutput{
				Value: TierGoldCollateral,
			}, nil
		},
		GetBestHeight: func() (uint32, error) {
			return 1000, nil
		},
		GetBlockHeightByHash: func(hash types.Hash) (uint32, error) {
			return 986, nil // Exactly 15 confirmations (1000 - 986 + 1 = 15)
		},
		GetTransactionBlockHash: func(txHash types.Hash) (types.Hash, error) {
			return testBlockHash, nil
		},
		// Enable multi-tier support for this test (Gold tier)
		IsMultiTierEnabled: func() bool { return true },
	}

	// Should pass with exactly MinConfirmations
	info, err := cv.ValidateCollateral(testOutpoint)
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, uint32(15), info.Confirmations)
	assert.Equal(t, Gold, info.Tier)
}

func TestFormatCollateralAmount(t *testing.T) {
	tests := []struct {
		satoshis int64
		expected string
	}{
		{TierBronzeCollateral, "1M TWINS (Bronze)"},
		{TierSilverCollateral, "5M TWINS (Silver)"},
		{TierGoldCollateral, "20M TWINS (Gold)"},
		{TierPlatinumCollateral, "100M TWINS (Platinum)"},
		{500000 * 1e8, "500000 TWINS (invalid)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := FormatCollateralAmount(tt.satoshis)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidCollateralAmounts(t *testing.T) {
	amounts := ValidCollateralAmounts()
	assert.Len(t, amounts, 4)
	assert.Contains(t, amounts, int64(TierBronzeCollateral))
	assert.Contains(t, amounts, int64(TierSilverCollateral))
	assert.Contains(t, amounts, int64(TierGoldCollateral))
	assert.Contains(t, amounts, int64(TierPlatinumCollateral))
}

// mockSporkManager implements SporkInterface for testing
type mockSporkManager struct {
	tiersActive bool
}

func (m *mockSporkManager) IsActive(sporkID int32) bool {
	if sporkID == SporkTwinsEnableMasternodeTiers {
		return m.tiersActive
	}
	return false
}

func (m *mockSporkManager) GetValue(sporkID int32) int64 {
	return 0
}

func TestIsValidCollateral_SporkOff(t *testing.T) {
	// Create manager with spork OFF
	manager, err := NewManager(nil, nil)
	require.NoError(t, err)
	manager.SetSporkManager(&mockSporkManager{tiersActive: false})

	tests := []struct {
		name      string
		amount    int64
		wantTier  MasternodeTier
		wantValid bool
	}{
		{"Bronze - always valid", TierBronzeCollateral, Bronze, true},
		{"Silver - rejected when spork off", TierSilverCollateral, Bronze, false},
		{"Gold - rejected when spork off", TierGoldCollateral, Bronze, false},
		{"Platinum - rejected when spork off", TierPlatinumCollateral, Bronze, false},
		{"Invalid amount", 500000 * 1e8, Bronze, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tier, valid := manager.IsValidCollateral(tt.amount)
			assert.Equal(t, tt.wantValid, valid, "validity mismatch for %s", tt.name)
			if valid {
				assert.Equal(t, tt.wantTier, tier, "tier mismatch for %s", tt.name)
			}
		})
	}
}

func TestIsValidCollateral_SporkOn(t *testing.T) {
	// Create manager with spork ON
	manager, err := NewManager(nil, nil)
	require.NoError(t, err)
	manager.SetSporkManager(&mockSporkManager{tiersActive: true})

	tests := []struct {
		name      string
		amount    int64
		wantTier  MasternodeTier
		wantValid bool
	}{
		{"Bronze - valid", TierBronzeCollateral, Bronze, true},
		{"Silver - valid when spork on", TierSilverCollateral, Silver, true},
		{"Gold - valid when spork on", TierGoldCollateral, Gold, true},
		{"Platinum - valid when spork on", TierPlatinumCollateral, Platinum, true},
		{"Invalid amount", 500000 * 1e8, Bronze, false},
		{"Invalid between tiers", 3000000 * 1e8, Bronze, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tier, valid := manager.IsValidCollateral(tt.amount)
			assert.Equal(t, tt.wantValid, valid, "validity mismatch for %s", tt.name)
			if valid {
				assert.Equal(t, tt.wantTier, tier, "tier mismatch for %s", tt.name)
			}
		})
	}
}

func TestIsValidCollateral_NoSporkManager(t *testing.T) {
	// Create manager without spork manager (nil)
	manager, err := NewManager(nil, nil)
	require.NoError(t, err)
	// Don't set spork manager - should default to Bronze only

	tests := []struct {
		name      string
		amount    int64
		wantTier  MasternodeTier
		wantValid bool
	}{
		{"Bronze - always valid", TierBronzeCollateral, Bronze, true},
		{"Silver - rejected without spork manager", TierSilverCollateral, Bronze, false},
		{"Gold - rejected without spork manager", TierGoldCollateral, Bronze, false},
		{"Platinum - rejected without spork manager", TierPlatinumCollateral, Bronze, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tier, valid := manager.IsValidCollateral(tt.amount)
			assert.Equal(t, tt.wantValid, valid, "validity mismatch for %s", tt.name)
			if valid {
				assert.Equal(t, tt.wantTier, tier, "tier mismatch for %s", tt.name)
			}
		})
	}
}

func TestIsTierSporkActive(t *testing.T) {
	manager, err := NewManager(nil, nil)
	require.NoError(t, err)

	// Without spork manager
	assert.False(t, manager.IsTierSporkActive(), "should be false without spork manager")

	// With spork OFF
	manager.SetSporkManager(&mockSporkManager{tiersActive: false})
	assert.False(t, manager.IsTierSporkActive(), "should be false when spork off")

	// With spork ON
	manager.SetSporkManager(&mockSporkManager{tiersActive: true})
	assert.True(t, manager.IsTierSporkActive(), "should be true when spork on")
}

// TestCollateralValidator_SporkAware tests the CollateralValidator spork-aware behavior
func TestCollateralValidator_SporkAware(t *testing.T) {
	testHash, _ := types.NewHashFromString("0000000000000000000000000000000000000000000000000000000000000001")
	testOutpoint := types.Outpoint{
		Hash:  testHash,
		Index: 0,
	}
	testBlockHash, _ := types.NewHashFromString("0000000000000000000000000000000000000000000000000000000000000002")

	makeValidator := func(amount int64, multiTierEnabled func() bool) *CollateralValidator {
		return &CollateralValidator{
			GetUTXO: func(outpoint types.Outpoint) (*types.TxOutput, error) {
				return &types.TxOutput{Value: amount}, nil
			},
			GetBestHeight: func() (uint32, error) {
				return 1000, nil
			},
			GetBlockHeightByHash: func(hash types.Hash) (uint32, error) {
				return 900, nil // 101 confirmations
			},
			GetTransactionBlockHash: func(txHash types.Hash) (types.Hash, error) {
				return testBlockHash, nil
			},
			IsMultiTierEnabled: multiTierEnabled,
		}
	}

	t.Run("Spork OFF - Bronze accepted", func(t *testing.T) {
		cv := makeValidator(TierBronzeCollateral, func() bool { return false })
		info, err := cv.ValidateCollateral(testOutpoint)
		require.NoError(t, err)
		assert.Equal(t, Bronze, info.Tier)
	})

	t.Run("Spork OFF - Silver rejected", func(t *testing.T) {
		cv := makeValidator(TierSilverCollateral, func() bool { return false })
		_, err := cv.ValidateCollateral(testOutpoint)
		assert.ErrorIs(t, err, ErrCollateralInvalidTier)
		assert.Contains(t, err.Error(), "SPORK_TWINS_01 not active")
	})

	t.Run("Spork OFF - Gold rejected", func(t *testing.T) {
		cv := makeValidator(TierGoldCollateral, func() bool { return false })
		_, err := cv.ValidateCollateral(testOutpoint)
		assert.ErrorIs(t, err, ErrCollateralInvalidTier)
		assert.Contains(t, err.Error(), "SPORK_TWINS_01 not active")
	})

	t.Run("Spork OFF - Platinum rejected", func(t *testing.T) {
		cv := makeValidator(TierPlatinumCollateral, func() bool { return false })
		_, err := cv.ValidateCollateral(testOutpoint)
		assert.ErrorIs(t, err, ErrCollateralInvalidTier)
		assert.Contains(t, err.Error(), "SPORK_TWINS_01 not active")
	})

	t.Run("Spork ON - all tiers accepted", func(t *testing.T) {
		tiers := []struct {
			amount int64
			tier   MasternodeTier
		}{
			{TierBronzeCollateral, Bronze},
			{TierSilverCollateral, Silver},
			{TierGoldCollateral, Gold},
			{TierPlatinumCollateral, Platinum},
		}
		for _, tc := range tiers {
			cv := makeValidator(tc.amount, func() bool { return true })
			info, err := cv.ValidateCollateral(testOutpoint)
			require.NoError(t, err, "tier %v should be accepted", tc.tier)
			assert.Equal(t, tc.tier, info.Tier)
		}
	})

	t.Run("No spork checker (nil) - only Bronze accepted", func(t *testing.T) {
		cv := makeValidator(TierBronzeCollateral, nil)
		info, err := cv.ValidateCollateral(testOutpoint)
		require.NoError(t, err)
		assert.Equal(t, Bronze, info.Tier)

		cv = makeValidator(TierSilverCollateral, nil)
		_, err = cv.ValidateCollateral(testOutpoint)
		assert.ErrorIs(t, err, ErrCollateralInvalidTier)
	})
}
