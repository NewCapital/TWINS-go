// Copyright (c) 2018-2025 The TWINS developers
// Distributed under the MIT/X11 software license, see the accompanying
// file COPYING or http://www.opensource.org/licenses/mit-license.php.

package masternode

import (
	"errors"
	"fmt"

	"github.com/twins-dev/twins-core/pkg/types"
)


// Collateral validation errors
var (
	ErrCollateralNotFound     = errors.New("collateral UTXO not found (already spent or invalid)")
	ErrCollateralSpent        = errors.New("collateral UTXO has been spent")
	ErrCollateralInvalidTier  = errors.New("collateral amount does not match any valid tier")
	ErrCollateralLowConf      = errors.New("collateral has insufficient confirmations")
	ErrCollateralInvalidTx    = errors.New("collateral transaction not found")
	ErrCollateralInvalidIndex = errors.New("collateral output index is invalid")
)

// Broadcast processing errors
var (
	// ErrBroadcastAlreadySeen is returned by ProcessBroadcast when the broadcast hash
	// is already in seenBroadcasts. This is NOT a failure - it means the broadcast was
	// previously processed. Callers should NOT relay or log warnings for this error.
	// Legacy: mapSeenMasternodeBroadcast.count(mnb.GetHash()) returns early (masternodeman.cpp:831-833)
	ErrBroadcastAlreadySeen = errors.New("broadcast already seen")
)

// CollateralValidator provides methods to validate masternode collateral UTXOs.
// This is designed to be used by the RPC layer to validate collateral before
// retrieving private keys.
type CollateralValidator struct {
	// GetUTXO returns the UTXO for an outpoint, or nil if spent/not found
	GetUTXO func(outpoint types.Outpoint) (*types.TxOutput, error)

	// GetBestHeight returns the current best block height
	GetBestHeight func() (uint32, error)

	// GetBlockHeightByHash returns the height of a block by its hash
	GetBlockHeightByHash func(hash types.Hash) (uint32, error)

	// GetTransactionBlockHash returns the block hash containing a transaction
	// Returns ZeroHash if not in a block (mempool) or not found
	GetTransactionBlockHash func(txHash types.Hash) (types.Hash, error)

	// IsMultiTierEnabled checks if SPORK_TWINS_01_ENABLE_MASTERNODE_TIERS is active
	// If nil, defaults to false (only Bronze tier valid)
	// Legacy C++: IsSporkActive(SPORK_TWINS_01_ENABLE_MASTERNODE_TIERS) from main.cpp
	IsMultiTierEnabled func() bool
}

// CollateralInfo contains validated collateral information
type CollateralInfo struct {
	Outpoint      types.Outpoint
	Amount        int64
	Tier          MasternodeTier
	Confirmations uint32
	Address       string // Extracted from scriptPubKey
}

// ValidateCollateral validates a collateral UTXO for masternode use.
// It checks:
// 1. UTXO exists and is unspent
// 2. Amount matches one of the valid tier amounts
// 3. Has at least MinConfirmations confirmations
//
// Returns detailed CollateralInfo on success, or an error with clear message.
func (cv *CollateralValidator) ValidateCollateral(outpoint types.Outpoint) (*CollateralInfo, error) {
	if cv.GetUTXO == nil || cv.GetBestHeight == nil {
		return nil, fmt.Errorf("collateral validator not properly initialized")
	}

	// Step 1: Check if UTXO exists (is unspent)
	utxo, err := cv.GetUTXO(outpoint)
	if err != nil {
		// Could be spent or doesn't exist
		return nil, fmt.Errorf("%w: %v", ErrCollateralNotFound, err)
	}
	if utxo == nil {
		return nil, ErrCollateralSpent
	}

	// Step 2: Validate amount matches a valid tier (spork-aware)
	tier, ok := cv.getTierForAmountSporkAware(utxo.Value)
	if !ok {
		// Determine error message based on spork status
		tierEnabled := cv.IsMultiTierEnabled != nil && cv.IsMultiTierEnabled()
		if !tierEnabled {
			return nil, fmt.Errorf("%w: amount %d TWINS - only Bronze (1M) accepted (SPORK_TWINS_01 not active)",
				ErrCollateralInvalidTier, utxo.Value/1e8)
		}
		return nil, fmt.Errorf("%w: amount %d TWINS doesn't match Bronze (1M), Silver (5M), Gold (20M), or Platinum (100M)",
			ErrCollateralInvalidTier, utxo.Value/1e8)
	}

	// Step 3: Calculate confirmations
	confirmations, err := cv.getConfirmations(outpoint.Hash)
	if err != nil {
		// If we can't get confirmations, assume 0
		confirmations = 0
	}

	// Step 4: Validate minimum confirmations
	if confirmations < MinConfirmations {
		return nil, fmt.Errorf("%w: has %d, requires %d",
			ErrCollateralLowConf, confirmations, MinConfirmations)
	}

	return &CollateralInfo{
		Outpoint:      outpoint,
		Amount:        utxo.Value,
		Tier:          tier,
		Confirmations: confirmations,
	}, nil
}

// getConfirmations calculates the number of confirmations for a transaction
func (cv *CollateralValidator) getConfirmations(txHash types.Hash) (uint32, error) {
	if cv.GetTransactionBlockHash == nil || cv.GetBestHeight == nil || cv.GetBlockHeightByHash == nil {
		return 0, fmt.Errorf("confirmation calculation not supported")
	}

	// Get the block hash containing this transaction
	blockHash, err := cv.GetTransactionBlockHash(txHash)
	if err != nil {
		return 0, err
	}

	// If zero hash, transaction is not in a block yet
	if blockHash == types.ZeroHash {
		return 0, nil
	}

	// Get block height
	txHeight, err := cv.GetBlockHeightByHash(blockHash)
	if err != nil {
		return 0, err
	}

	// Get current height
	currentHeight, err := cv.GetBestHeight()
	if err != nil {
		return 0, err
	}

	// Calculate confirmations (including the block itself)
	if currentHeight >= txHeight {
		return currentHeight - txHeight + 1, nil
	}
	return 0, nil
}

// getTierForAmountSporkAware returns the masternode tier for a given collateral amount,
// respecting the SPORK_TWINS_01_ENABLE_MASTERNODE_TIERS spork status.
// When spork is OFF (or IsMultiTierEnabled is nil), only Bronze tier is valid.
// When spork is ON, all 4 tiers are valid.
// Legacy C++: isMasternodeCollateral() checks SPORK_TWINS_01_ENABLE_MASTERNODE_TIERS
func (cv *CollateralValidator) getTierForAmountSporkAware(amount int64) (MasternodeTier, bool) {
	// Bronze (1M TWINS) is always valid regardless of spork state
	if amount == TierBronzeCollateral {
		return Bronze, true
	}

	// Higher tiers require SPORK_TWINS_01_ENABLE_MASTERNODE_TIERS to be active
	tierEnabled := cv.IsMultiTierEnabled != nil && cv.IsMultiTierEnabled()
	if !tierEnabled {
		// Spork OFF or no spork checker - only Bronze tier allowed
		return Bronze, false
	}

	// Spork ON - check all tiers
	switch amount {
	case TierSilverCollateral:
		return Silver, true
	case TierGoldCollateral:
		return Gold, true
	case TierPlatinumCollateral:
		return Platinum, true
	default:
		return Bronze, false
	}
}

// ValidCollateralAmounts returns all valid collateral amounts for documentation
func ValidCollateralAmounts() []int64 {
	return []int64{
		TierBronzeCollateral,
		TierSilverCollateral,
		TierGoldCollateral,
		TierPlatinumCollateral,
	}
}

// FormatCollateralAmount formats a collateral amount in TWINS (not satoshis)
func FormatCollateralAmount(satoshis int64) string {
	switch satoshis {
	case TierBronzeCollateral:
		return "1M TWINS (Bronze)"
	case TierSilverCollateral:
		return "5M TWINS (Silver)"
	case TierGoldCollateral:
		return "20M TWINS (Gold)"
	case TierPlatinumCollateral:
		return "100M TWINS (Platinum)"
	default:
		return fmt.Sprintf("%d TWINS (invalid)", satoshis/CoinUnit)
	}
}
