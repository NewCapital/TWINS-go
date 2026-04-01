package masternode

import (
	"fmt"
	"time"

	"github.com/twins-dev/twins-core/internal/spork"
	"github.com/twins-dev/twins-core/pkg/types"
)

// PaymentValidationTolerance is the allowed difference in satoshis between expected and actual
// masternode payment amounts, accounting for rounding differences.
const PaymentValidationTolerance int64 = 100

// PaymentCalculator handles masternode payment calculations
type PaymentCalculator struct {
	manager    *Manager
	devAddress []byte // Dev fund scriptPubKey for fallback when no masternodes available
}

// NewPaymentCalculator creates a new payment calculator
// devAddress is the scriptPubKey for the dev fund address (used as fallback when no masternodes available)
func NewPaymentCalculator(manager *Manager, devAddress []byte) *PaymentCalculator {
	return &PaymentCalculator{
		manager:    manager,
		devAddress: devAddress,
	}
}

// GetMinMasternodePaymentsProto returns the minimum protocol version required for payment
// Implements legacy CMasternodePayments::GetMinMasternodePaymentsProto()
// When SPORK_10 is active: Only masternodes with ActiveProtocol are paid
// When SPORK_10 is not active: Allow older protocol versions
func (pc *PaymentCalculator) GetMinMasternodePaymentsProto() int32 {
	// Check if SPORK_10_MASTERNODE_PAY_UPDATED_NODES is active
	if pc.manager.sporkManager != nil && pc.manager.sporkManager.IsActive(spork.SporkMasternodePayUpdatedNodes) {
		// SPORK_10 is active - only pay updated masternodes
		return ActiveProtocolVersion
	}
	// SPORK_10 is not active - allow older protocol versions
	return MinPeerProtoBeforeEnforcement
}

// CalculateBlockReward calculates the block reward for a given height.
// This is a direct port of legacy C++ GetBlockValue() from main.cpp.
// Block reward amounts are kept as literal values (not constants) to preserve
// 1:1 correspondence with the C++ source for audit and consensus verification.
func (pc *PaymentCalculator) CalculateBlockReward(height uint32) int64 {
	var nSubsidy int64

	// First block with initial pre-mine
	if height == 1 {
		nSubsidy = 6000000 * CoinUnit
	} else if height < 711111 {
		// Release 15220.70 TWINS as a reward for each block until block 711111
		nSubsidy = 1522070000000 // 15220.70 * CoinUnit
	} else if height < 716666 {
		// Phasing out inflation...
		nSubsidy = 8000 * CoinUnit
	} else if height < 722222 {
		nSubsidy = 4000 * CoinUnit
	} else if height < 727777 {
		nSubsidy = 2000 * CoinUnit
	} else if height < 733333 {
		nSubsidy = 1000 * CoinUnit
	} else if height < 738888 {
		nSubsidy = 500 * CoinUnit
	} else if height < 744444 {
		nSubsidy = 250 * CoinUnit
	} else if height < 750000 {
		nSubsidy = 125 * CoinUnit
	} else if height < 755555 {
		nSubsidy = 60 * CoinUnit
	} else if height < 761111 {
		nSubsidy = 30 * CoinUnit
	} else if height < 766666 {
		nSubsidy = 15 * CoinUnit
	} else if height < 772222 {
		nSubsidy = 8 * CoinUnit
	} else if height < 777777 {
		nSubsidy = 4 * CoinUnit
	} else if height < 910000 {
		nSubsidy = 2 * CoinUnit
	} else if height < 6569605 {
		nSubsidy = 100 * CoinUnit
	} else {
		nSubsidy = 0
	}

	return nSubsidy
}

// CalculateMasternodePayment calculates the payment for a masternode
// In TWINS protocol, ALL masternodes receive the SAME payment amount (80% of block reward)
// regardless of tier. Tier only affects selection probability, not payment amount.
func (pc *PaymentCalculator) CalculateMasternodePayment(blockReward int64) int64 {
	return (blockReward * int64(types.DefaultMasternodeReward)) / 10000
}

// CalculateDevReward calculates the dev reward for a block
// CRITICAL: Dev reward is ONLY for PoS blocks (10% of block value)
// Legacy: CAmount nDevReward = blockValue * .1; (inside if (fProofOfStake))
// For PoW blocks, there is no dev reward
func (pc *PaymentCalculator) CalculateDevReward(blockReward int64, isProofOfStake bool) int64 {
	// Dev reward only applies to PoS blocks
	// Legacy: inside FillBlockPayee(), dev reward is only calculated if (fProofOfStake)
	if !isProofOfStake {
		return 0
	}

	return (blockReward * int64(types.DefaultDevFundReward)) / 10000
}

// BlockPaymentInfo contains the payment breakdown for a block
// Used by FillBlockPayment to return all payment details
type BlockPaymentInfo struct {
	MasternodePayment int64  // Amount paid to masternode (80% of block reward)
	DevReward         int64  // Amount paid to dev address (10% for PoS, 0 for PoW)
	StakerReward      int64  // Remaining amount for staker/miner
	PayeeAddress      []byte // Script/address for masternode payment
	DevAddress        []byte // Script/address for dev payment (only if DevReward > 0)
	IsProofOfStake    bool   // Whether this is a PoS block
}

// FillBlockPayment calculates block payment distribution matching legacy FillBlockPayee()
// CRITICAL: Dev output is ONLY added for PoS blocks, NOT for PoW blocks
//
// Legacy behavior (masternode-payments.cpp:FillBlockPayee):
// - PoS block: vout.push_back(CTxOut(nDevReward, DEV_SCRIPT)) // Dev output
// - PoS block: vout[i].nValue = masternodePayment             // MN output
// - PoS block: vout[i-1].nValue -= masternodePayment + nDevReward // Staker pays MN+dev
// - PoW block: vout[1].nValue = masternodePayment             // MN output
// - PoW block: vout[0].nValue = blockValue - masternodePayment // Miner gets rest
func (pc *PaymentCalculator) FillBlockPayment(blockHeight uint32, blockHash types.Hash, isProofOfStake bool) (*BlockPaymentInfo, error) {
	// Calculate block reward for this height
	blockReward := pc.CalculateBlockReward(blockHeight)

	// Get masternode payment (80% of block reward)
	// Legacy: CAmount masternodePayment = GetMasternodePayment(pindexPrev->nHeight + 1, blockValue, 0, fZTWINSStake)
	masternodePayment := pc.CalculateMasternodePayment(blockReward)

	// Calculate dev reward (10% for PoS only)
	// Legacy: CAmount nDevReward = blockValue * .1; (inside if (fProofOfStake) only!)
	devReward := pc.CalculateDevReward(blockReward, isProofOfStake)

	// Get masternode winner for payment
	winner, err := pc.GetNextPaymentWinner(blockHash)
	if err != nil {
		// Fallback: no masternode available, payment goes to dev address
		// Legacy: if (!hasPayment) { payee = devScript; hasPayment = true; }
		pc.manager.logger.WithError(err).Warn("No masternode available for payment, using dev address fallback")
	}

	info := &BlockPaymentInfo{
		MasternodePayment: masternodePayment,
		DevReward:         devReward,
		IsProofOfStake:    isProofOfStake,
	}

	// Calculate staker/miner reward (what's left after MN and dev payments)
	// PoS: Staker pays both MN and dev from their stake reward
	// PoW: Miner pays only MN from their coinbase reward
	if isProofOfStake {
		// Legacy: txNew.vout[i - 1].nValue -= masternodePayment + nDevReward
		info.StakerReward = blockReward - masternodePayment - devReward
	} else {
		// Legacy: txNew.vout[0].nValue = blockValue - masternodePayment
		info.StakerReward = blockReward - masternodePayment
	}

	// Set payee address (masternode collateral address or dev address fallback)
	if winner != nil && winner.PubKeyCollateral != nil {
		// Legacy: payee = GetScriptForDestination(winningNode->pubKeyCollateralAddress.GetID())
		// CRITICAL: Must return P2PKH scriptPubKey (25 bytes), NOT raw pubkey (33 bytes)
		// Use GetPayeeScript() which creates proper P2PKH script
		info.PayeeAddress = winner.GetPayeeScript()
	} else if len(pc.devAddress) > 0 {
		// Fallback to dev address when no masternode winner available
		// Legacy: if (!hasPayment) { payee = devScript; hasPayment = true; }
		// This ensures the masternode payment still goes somewhere valid
		// instead of being lost or causing invalid block construction
		info.PayeeAddress = pc.devAddress
	}

	return info, nil
}

// CalculateTotalMasternodeReward calculates total masternode rewards for a block
// CRITICAL: Must match legacy GetMasternodePayment() which returns blockValue * 0.8
func (pc *PaymentCalculator) CalculateTotalMasternodeReward(blockReward int64) int64 {
	// Get active masternode counts by tier
	bronzeCount := pc.manager.GetMasternodeCountByTier(Bronze)
	silverCount := pc.manager.GetMasternodeCountByTier(Silver)
	goldCount := pc.manager.GetMasternodeCountByTier(Gold)
	platinumCount := pc.manager.GetMasternodeCountByTier(Platinum)

	totalMNs := bronzeCount + silverCount + goldCount + platinumCount

	if totalMNs == 0 {
		return 0
	}

	// CRITICAL: Legacy TWINS allocates 80% of block reward to masternodes
	// Reference: main.cpp GetMasternodePayment() returns blockValue * 0.8
	// NOT 45% like Dash - TWINS has higher masternode reward share
	totalMNReward := (blockReward * int64(types.DefaultMasternodeReward)) / 10000

	return totalMNReward
}

// GetNextPaymentWinner selects the next masternode to receive payment
// Delegates to Manager.GetNextMasternodeInQueueForPayment which correctly calls
// UpdateStatusWithUTXO (equivalent to legacy mn.Check()) on each masternode.
//
// DEPRECATED: Use GetNextPaymentWinnerForHeight for explicit height parameter
func (pc *PaymentCalculator) GetNextPaymentWinner(blockHash types.Hash) (*Masternode, error) {
	// Derive blockHeight from hash, fallback to bestHeight+1
	var blockHeight uint32
	if pc.manager.blockchain != nil {
		if h, err := pc.manager.blockchain.GetBlockHeight(blockHash); err == nil {
			blockHeight = h
		} else if h, err := pc.manager.blockchain.GetBestHeight(); err == nil {
			blockHeight = h + 1
		}
	}
	return pc.GetNextPaymentWinnerForHeight(blockHash, blockHeight)
}

// GetNextPaymentWinnerForHeight selects the next masternode for payment at a specific height
// Delegates to Manager.GetNextMasternodeInQueueForPayment for the actual selection.
//
// LEGACY COMPATIBILITY FIX: This method does NOT update scheduledPayments.
// In legacy C++, GetNextMasternodeInQueueForPayment() is a pure query that does NOT modify
// mapMasternodeBlocks. The scheduledPayments (mapMasternodeBlocks) is only populated by
// AddWinningMasternode() which is called AFTER a signed vote is created/received.
//
// Scheduling happens in:
// - ProcessPaymentWithBlockTime() when a block is processed
// - AddScheduledPayment() when a payment vote is received
//
// This is the preferred method - explicitly pass the target blockHeight to avoid
// mis-evaluating IsScheduled/age filters when building new blocks (hash not yet known).
func (pc *PaymentCalculator) GetNextPaymentWinnerForHeight(blockHash types.Hash, blockHeight uint32) (*Masternode, error) {
	// Delegate to Manager which correctly calls UpdateStatusWithUTXO on each masternode
	// This matches legacy C++ behavior where mn.Check() is called inline
	bestMN, count := pc.manager.GetNextMasternodeInQueueForPayment(blockHeight, true)

	if bestMN == nil {
		return nil, fmt.Errorf("no eligible masternodes available for payment (checked %d)", count)
	}

	// NOTE: Do NOT update scheduledPayments here.
	// Legacy C++: GetNextMasternodeInQueueForPayment does NOT call AddWinningMasternode
	// Scheduling only happens when:
	// 1. A signed payment vote is received (ProcessPaymentVote -> AddScheduledPayment)
	// 2. A block with masternode payment is processed (ProcessPaymentWithBlockTime)

	return bestMN, nil
}

// ValidatePayment validates a masternode payment
func (pc *PaymentCalculator) ValidatePayment(
	outpoint types.Outpoint,
	amount int64,
	blockHeight uint32,
) error {
	// Get masternode
	mn, err := pc.manager.GetMasternode(outpoint)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrMasternodeNotFound, err)
	}

	// Check if masternode is active
	if !mn.IsActive() {
		return fmt.Errorf("masternode is not active")
	}

	// Calculate expected payment
	blockReward := pc.CalculateBlockReward(blockHeight)
	expectedPayment := pc.CalculateMasternodePayment(blockReward)

	diff := amount - expectedPayment
	if diff < -PaymentValidationTolerance || diff > PaymentValidationTolerance {
		return fmt.Errorf("invalid payment amount: got %d, expected %d", amount, expectedPayment)
	}

	return nil
}

// PaymentHistory tracks payment history for a masternode
type PaymentHistory struct {
	OutPoint      types.Outpoint
	Payments      []*PaymentRecord
	TotalReceived int64
	LastPayment   time.Time
}

// PaymentRecord represents a single payment
type PaymentRecord struct {
	BlockHeight  uint32
	BlockHash    types.Hash
	Amount       int64
	Timestamp    time.Time
	Confirmed    bool
	Confirmations int
}

// AddPaymentRecord adds a payment record to history
func (ph *PaymentHistory) AddPaymentRecord(record *PaymentRecord) {
	ph.Payments = append(ph.Payments, record)
	ph.TotalReceived += record.Amount
	ph.LastPayment = record.Timestamp
}

// GetRecentPayments returns the N most recent payments
func (ph *PaymentHistory) GetRecentPayments(n int) []*PaymentRecord {
	if len(ph.Payments) == 0 {
		return nil
	}

	start := len(ph.Payments) - n
	if start < 0 {
		start = 0
	}

	return ph.Payments[start:]
}

// GetPaymentCount returns the total number of payments
func (ph *PaymentHistory) GetPaymentCount() int {
	return len(ph.Payments)
}

// GetAveragePaymentAmount returns the average payment amount
func (ph *PaymentHistory) GetAveragePaymentAmount() int64 {
	if len(ph.Payments) == 0 {
		return 0
	}

	return ph.TotalReceived / int64(len(ph.Payments))
}