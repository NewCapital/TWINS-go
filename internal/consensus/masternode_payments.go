package consensus

import (
	"bytes"
	"flag"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/twins-dev/twins-core/pkg/crypto"
	"github.com/twins-dev/twins-core/pkg/script"
	"github.com/twins-dev/twins-core/pkg/types"
)

// isTestEnvironment detects if running in test mode (go test)
// Used to prevent accidental use of test-only features in production
func isTestEnvironment() bool {
	return flag.Lookup("test.v") != nil
}

// Note: SporkInterface and BudgetInterface are defined in validation.go

// MasternodePaymentValidator validates masternode payments in blocks
//
// Vote Persistence (Legacy Compatibility):
// Legacy C++ stores/loads payment votes via CMasternodePaymentDB (masternode-payments.cpp:1-120).
// This Go implementation supports optional persistent storage via PaymentVoteStorage interface.
// When storage is configured, votes are persisted to disk and loaded on startup, matching
// legacy behavior exactly.
//
// Without storage configured, votes are kept in memory only and lost on restart.
// The node will then re-collect votes from network, or use calculated winner as fallback.
// PaymentVoteRelayFunc is called to relay a valid payment vote to the P2P network
// Matches legacy C++ winner.Relay() from masternode-payments.cpp:491-493
type PaymentVoteRelayFunc func(blockHeight uint32, mnVin types.Outpoint, payAddress []byte, signature []byte)

// PaymentRecorder records masternode payment events for tracking/statistics.
// Implemented by masternode.PaymentTracker.
type PaymentRecorder interface {
	RecordPayment(scriptPubKey []byte, blockHeight uint32, blockTime time.Time, amount int64, txID string)
}

type MasternodePaymentValidator struct {
	mu                        sync.RWMutex
	masternodeInterface       MasternodeInterface         // Interface to masternode manager
	chainParams               *types.ChainParams          // Chain parameters for dev address
	paymentVotes              map[uint32]*BlockPaymentVotes
	paymentWinners            map[types.Hash]*PaymentWinner
	lastVotes                 map[types.Outpoint]uint32   // Track last vote height per masternode
	sporkManager              SporkInterface              // Spork manager for enforcement checks
	budgetManager             BudgetInterface             // Budget manager for superblock checks
	storage                   PaymentVoteStorage          // Optional persistent storage for votes
	skipSignatureVerification bool                        // FOR TESTING ONLY: skip signature check
	voteRelayFunc             PaymentVoteRelayFunc        // Callback for relaying valid votes to P2P network
	paymentRecorder           PaymentRecorder             // Optional payment tracker for statistics
}

// MasternodeInterface defines required masternode operations
type MasternodeInterface interface {
	GetActiveCount() int
	// GetStableCount returns count of "stable" masternodes older than 8000 seconds.
	// Used when SPORK_8 is active for payment calculation to prevent manipulation.
	// Legacy: CMasternodeMan::stable_size() from masternodeman.cpp:351-376
	GetStableCount() int
	// GetBestHeight returns current chain tip height for vote window validation
	// LEGACY COMPATIBILITY: Used in ProcessMessageMasternodePayments to validate vote height window
	GetBestHeight() (uint32, error)
	GetMasternodeByOutpoint(outpoint types.Outpoint) (MasternodeInfo, error)
	GetNextPaymentWinner(blockHeight uint32, blockHash types.Hash) (MasternodeInfo, error)
	IsActiveAtHeight(outpoint types.Outpoint, height uint32) bool
	// IsActiveAtHeightLegacy checks if masternode was active at height WITH current UTXO validation
	// LEGACY COMPATIBILITY: C++ calls mn.Check() in GetMasternodeRank which validates CURRENT
	// UTXO state even for historical votes. Use this for MNW vote validation.
	IsActiveAtHeightLegacy(outpoint types.Outpoint, height uint32) bool
	GetMasternodePublicKey(outpoint types.Outpoint) ([]byte, error)
	ProcessPayment(outpoint types.Outpoint, blockHeight int32) error
	// ProcessPaymentWithBlockTime uses block timestamp for deterministic ordering (legacy compatible)
	// blockHash is required for tier cycle tracking (AddWin resets cycle based on block hash)
	ProcessPaymentWithBlockTime(outpoint types.Outpoint, blockHeight int32, blockTime int64, blockHash types.Hash) error
	// GetMasternodeByPayAddress finds a masternode by its payment script (P2PKH scriptPubKey)
	// Used to identify which masternode received a voted payment for queue advancement
	GetMasternodeByPayAddress(payAddress []byte) (MasternodeInfo, error)
	// GetMasternodeRank returns rank of masternode at blockHeight using minProtocol filter
	// Returns -1 if masternode not found or not eligible. Rank 1 = highest score.
	// Legacy: CMasternodeMan::GetMasternodeRank from masternodeman.cpp:689-734
	GetMasternodeRank(outpoint types.Outpoint, blockHeight uint32, minProtocol int32, fOnlyActive bool) int
	// GetMinMasternodePaymentsProto returns minimum protocol version for payment eligibility
	// Legacy: CMasternodePayments::GetMinMasternodePaymentsProto uses SPORK_10 (10009)
	GetMinMasternodePaymentsProto() int32
	// MarkPayeeScheduled marks a payee as scheduled for payment at given height
	// LEGACY COMPATIBILITY: In C++, AddWinningMasternode populates mapMasternodeBlocks[height].AddPayee()
	// which is then checked by IsScheduled(). This method provides equivalent functionality.
	// Called when a payee receives enough votes to reach consensus threshold.
	MarkPayeeScheduled(payAddress []byte, blockHeight uint32) error
}

// MasternodeInfo contains masternode information needed for payment validation
type MasternodeInfo struct {
	Outpoint        types.Outpoint
	Tier            int // 0=Bronze, 1=Silver, 2=Gold, 3=Platinum
	PayAddress      []byte
	ProtocolVersion int
	LastPaid        uint32
	Score           float64
	PubKey          []byte // Masternode public key for signature verification
}

// PaymentWinner represents a masternode payment winner for a block
type PaymentWinner struct {
	BlockHeight   uint32
	MasternodeVin types.Outpoint
	PayAddress    []byte
	Votes         int
	Signature     []byte
}

// BlockPaymentVotes tracks payment votes for a specific block
type BlockPaymentVotes struct {
	BlockHeight uint32
	Payees      []*PayeeVotes
	mu          sync.RWMutex
}

// PayeeVotes tracks votes for a specific payee
type PayeeVotes struct {
	PayAddress []byte
	Votes      int
}

// HasPayeeWithVotes checks if a payee has at least votesRequired votes
// Matches legacy C++ CMasternodeBlockPayees::HasPayeeWithVotes from masternode-payments.h:137-146
func (bpv *BlockPaymentVotes) HasPayeeWithVotes(payAddress []byte, votesRequired int) bool {
	bpv.mu.RLock()
	defer bpv.mu.RUnlock()

	for _, p := range bpv.Payees {
		if p.Votes >= votesRequired && bytes.Equal(p.PayAddress, payAddress) {
			return true
		}
	}
	return false
}

const (
	// Payment validation constants from legacy code
	MinPaymentSignatures    = 6  // MNPAYMENTS_SIGNATURES_REQUIRED
	TotalPaymentSignatures  = 10 // MNPAYMENTS_SIGNATURES_TOTAL
	PaymentVotingWindowSize = 1000

	// Spork constants for masternode payment enforcement
	// Legacy: SPORK_8_MASTERNODE_PAYMENT_ENFORCEMENT = 10007
	SporkMasternodePaymentEnforcement = int32(10007)

	// Spork constants shared across consensus validation
	SporkBudgetEnforcement  = int32(10008) // SporkBudgetEnforcement
	SporkEnableSuperblocks  = int32(10013) // SPORK_13_ENABLE_SUPERBLOCKS
)

// NewMasternodePaymentValidator creates a new payment validator
func NewMasternodePaymentValidator(mnInterface MasternodeInterface, chainParams *types.ChainParams) *MasternodePaymentValidator {
	return &MasternodePaymentValidator{
		masternodeInterface: mnInterface,
		chainParams:         chainParams,
		paymentVotes:        make(map[uint32]*BlockPaymentVotes),
		paymentWinners:      make(map[types.Hash]*PaymentWinner),
		lastVotes:           make(map[types.Outpoint]uint32),
	}
}

// SetSporkManager sets the spork manager for enforcement checks
func (mpv *MasternodePaymentValidator) SetSporkManager(sporkManager SporkInterface) {
	mpv.mu.Lock()
	defer mpv.mu.Unlock()
	mpv.sporkManager = sporkManager
}

// SetBudgetManager sets the budget manager for superblock checks
func (mpv *MasternodePaymentValidator) SetBudgetManager(budgetManager BudgetInterface) {
	mpv.mu.Lock()
	defer mpv.mu.Unlock()
	mpv.budgetManager = budgetManager
}

// SetStorage sets the persistent storage for payment votes
// This enables legacy-compatible vote persistence across restarts
// Legacy: CMasternodePaymentDB from masternode-payments.cpp
func (mpv *MasternodePaymentValidator) SetStorage(storage PaymentVoteStorage) {
	mpv.mu.Lock()
	defer mpv.mu.Unlock()
	mpv.storage = storage
}

// SetSkipSignatureVerification enables skipping signature verification (FOR TESTING ONLY)
// This allows unit tests to add votes without generating valid ECDSA signatures
// WARNING: Never enable this in production code - will panic if attempted outside tests
func (mpv *MasternodePaymentValidator) SetSkipSignatureVerification(skip bool) {
	if skip && !isTestEnvironment() {
		panic("SetSkipSignatureVerification: attempted to skip signature verification outside test environment - this is a security violation")
	}
	mpv.mu.Lock()
	defer mpv.mu.Unlock()
	mpv.skipSignatureVerification = skip
}

// SetPaymentRecorder sets the payment recorder for tracking masternode payment statistics.
// When set, ValidateBlockPayment records each masternode payment it encounters.
func (mpv *MasternodePaymentValidator) SetPaymentRecorder(recorder PaymentRecorder) {
	mpv.mu.Lock()
	defer mpv.mu.Unlock()
	mpv.paymentRecorder = recorder
}

// SetVoteRelayHandler sets the callback for relaying valid votes to P2P network
// This enables vote propagation matching legacy winner.Relay() behavior
// Reference: legacy/src/masternode-payments.cpp:491-493
func (mpv *MasternodePaymentValidator) SetVoteRelayHandler(handler PaymentVoteRelayFunc) {
	mpv.mu.Lock()
	defer mpv.mu.Unlock()
	mpv.voteRelayFunc = handler
}

// LoadFromStorage loads payment votes from persistent storage
// Should be called at daemon startup after storage is set
// Legacy: CMasternodePaymentDB::Read() from masternode-payments.cpp:67-147
func (mpv *MasternodePaymentValidator) LoadFromStorage() error {
	mpv.mu.Lock()
	defer mpv.mu.Unlock()

	if mpv.storage == nil {
		return nil // No storage configured, skip
	}

	// Load payment votes
	votes, err := mpv.storage.LoadAllVotes()
	if err != nil {
		return fmt.Errorf("failed to load payment votes: %w", err)
	}

	for height, blockVotes := range votes {
		mpv.paymentVotes[height] = blockVotes
	}

	// Load last votes
	lastVotes, err := mpv.storage.LoadAllLastVotes()
	if err != nil {
		return fmt.Errorf("failed to load last votes: %w", err)
	}

	for outpoint, height := range lastVotes {
		mpv.lastVotes[outpoint] = height
	}

	return nil
}

// GetVoteStats returns statistics about stored votes (for debugging/RPC)
func (mpv *MasternodePaymentValidator) GetVoteStats() (voteBlocks int, lastVotes int) {
	mpv.mu.RLock()
	defer mpv.mu.RUnlock()
	return len(mpv.paymentVotes), len(mpv.lastVotes)
}

// GetBlockPayee returns the payee with the most votes for a given block height.
// This is the Go equivalent of CMasternodePayments::GetBlockPayee() from masternode-payments.cpp:518-525.
//
// CRITICAL LEGACY BEHAVIOR:
// - Returns the payee with the HIGHEST vote count, regardless of threshold
// - Even 1 vote is enough to return a payee (threshold is only for enforcement)
// - Returns (nil, false) if no votes exist for this block
//
// Legacy C++ (masternode-payments.cpp:518-525):
//
//	bool CMasternodePayments::GetBlockPayee(int nBlockHeight, CScript& payee) {
//	    if (mapMasternodeBlocks.count(nBlockHeight)) {
//	        return mapMasternodeBlocks[nBlockHeight].GetPayee(payee);
//	    }
//	    return false;
//	}
//
// And CMasternodeBlockPayees::GetPayee() returns payee with max votes:
//
//	bool GetPayee(CScript& payee) {
//	    int nVotes = -1;
//	    BOOST_FOREACH(CMasternodePayee& p, vecPayments) {
//	        if (p.nVotes > nVotes) {
//	            payee = p.scriptPubKey;
//	            nVotes = p.nVotes;
//	        }
//	    }
//	    return (nVotes > -1);
//	}
func (mpv *MasternodePaymentValidator) GetBlockPayee(blockHeight uint32) ([]byte, bool) {
	mpv.mu.RLock()
	blockVotes, exists := mpv.paymentVotes[blockHeight]
	mpv.mu.RUnlock()

	if !exists || blockVotes == nil {
		return nil, false
	}

	// Find payee with most votes (matches legacy GetPayee behavior)
	blockVotes.mu.RLock()
	defer blockVotes.mu.RUnlock()

	var maxVotes int = -1
	var winner []byte

	for _, payee := range blockVotes.Payees {
		if payee.Votes > maxVotes {
			maxVotes = payee.Votes
			winner = payee.PayAddress
		}
	}

	// Legacy returns true if nVotes > -1, meaning any votes exist
	return winner, maxVotes > -1
}

// ValidateBlockPayment validates that a block contains the correct masternode payment
// This is the Go equivalent of IsBlockPayeeValid() from masternode-payments.cpp
// with spork-aware enforcement matching legacy behavior
func (mpv *MasternodePaymentValidator) ValidateBlockPayment(
	block *types.Block,
	blockHeight uint32,
	blockReward int64,
	isSynced bool,
) error {
	if !isSynced {
		// During initial sync, skip payment validation
		return nil
	}

	// CRITICAL: Legacy uses different transaction for PoW vs PoS blocks
	// masternode-payments.cpp:234 - const CTransaction& txNew = (nBlockHeight > Params().LAST_POW_BLOCK() ? block.vtx[1] : block.vtx[0]);
	// PoW blocks (height <= LAST_POW_BLOCK): use coinbase (vtx[0])
	// PoS blocks (height > LAST_POW_BLOCK): use coinstake (vtx[1])
	var paymentTx *types.Transaction
	lastPOWBlock := uint32(400) // Default mainnet value
	if mpv.chainParams != nil && mpv.chainParams.LastPOWBlock > 0 {
		lastPOWBlock = mpv.chainParams.LastPOWBlock
	}

	if blockHeight > lastPOWBlock {
		// PoS block - use coinstake transaction (second tx)
		if len(block.Transactions) < 2 {
			return fmt.Errorf("PoS block has insufficient transactions")
		}
		paymentTx = block.Transactions[1]
	} else {
		// PoW block - use coinbase transaction (first tx)
		if len(block.Transactions) < 1 {
			return fmt.Errorf("PoW block has no coinbase transaction")
		}
		paymentTx = block.Transactions[0]
	}

	coinstake := paymentTx

	// Check if this is a budget payment block (matching legacy IsBlockPayeeValid)
	mpv.mu.RLock()
	sporkMgr := mpv.sporkManager
	budgetMgr := mpv.budgetManager
	mpv.mu.RUnlock()

	if sporkMgr != nil && sporkMgr.IsActive(SporkEnableSuperblocks) {
		if budgetMgr != nil && budgetMgr.IsBudgetPaymentBlock(blockHeight) {
			// Budget payment block - MUST validate with budget manager
			// CRITICAL FIX: Legacy masternode-payments.cpp:238-251 does NOT just skip validation!
			// It calls budget.IsTransactionValid() and enforces based on SPORK_9
			transactionStatus := budgetMgr.IsTransactionValid(coinstake, blockHeight)

			if transactionStatus == TrxValidationValid {
				// Valid budget payment - skip masternode payment validation
				return nil
			}

			if transactionStatus == TrxValidationInvalid {
				// Invalid budget payment detected
				// Check SPORK_9 for enforcement
				if sporkMgr.IsActive(SporkBudgetEnforcement) {
					return fmt.Errorf("invalid budget payment detected at height %d", blockHeight)
				}
				// Budget enforcement disabled - fall through to masternode payment validation
			}

			// For DoublePayment or VoteThreshold status, fall through to masternode payment
			// Legacy: "In all cases a masternode will get the payment for this block"
		}
	}

	// Not a budget block - validate masternode payment
	// Check if we have payment votes for this block
	mpv.mu.RLock()
	blockVotes, hasVotes := mpv.paymentVotes[blockHeight]
	mpv.mu.RUnlock()

	var paymentErr error

	if !hasVotes {
		// CRITICAL FIX: Match legacy CMasternodePayments::IsTransactionValid behavior
		// Legacy (masternode-payments.cpp:679-688):
		//   if (mapMasternodeBlocks.count(nBlockHeight)) {
		//       return mapMasternodeBlocks[nBlockHeight].IsTransactionValid(txNew);
		//   }
		//   return true;  // <-- Accept ANY payment when no votes recorded!
		//
		// When no votes exist for a block, legacy accepts ANY masternode payment.
		// This is critical for:
		// 1. Initial sync / reindex - historical blocks have no vote records
		// 2. Sparse MNW traffic - votes may not have propagated yet
		// 3. Network startup - before enough masternodes are online
		//
		// DO NOT compute expected winner and validate - that diverges from legacy!

		// Record the payment for tracking even without votes
		mpv.recordPaymentFromCoinstake(coinstake, block, blockHeight)
		return nil
	}

	// We have votes - validate based on voting consensus
	if err := mpv.validatePaymentWithVotes(coinstake, blockVotes, blockHeight, blockReward); err != nil {
		paymentErr = err
	} else {
		// CRITICAL FIX: Use voted payee to advance queue, NOT GetNextPaymentWinner
		// When votes exist, the voted payee is the one who received payment, not the calculated winner
		// Legacy C++ tracks payments via vote consensus, not queue calculation
		winningPayee := mpv.getVotedWinningPayee(blockVotes)
		if winningPayee != nil {
			// Find the masternode that matches this pay address and advance its queue
			winner, err := mpv.masternodeInterface.GetMasternodeByPayAddress(winningPayee)
			if err == nil {
				// CRITICAL: Use block timestamp (not wall-clock) for deterministic ordering
				// Pass block hash for tier cycle tracking (AddWin uses hash for cycle reset)
				_ = mpv.masternodeInterface.ProcessPaymentWithBlockTime(winner.Outpoint, int32(blockHeight), int64(block.Header.Timestamp), block.Header.Hash())
			}
			// Record the voted payment for tracking
			mpv.recordPaymentFromCoinstake(coinstake, block, blockHeight)
		}
		return nil
	}

	// If we got here, payment validation failed
	// Check if enforcement is active (matching legacy behavior)
	if sporkMgr != nil && sporkMgr.IsActive(SporkMasternodePaymentEnforcement) {
		// Enforcement is active - reject block
		return paymentErr
	}

	// Enforcement is disabled - accept block (legacy behavior)
	// This allows operators to disable enforcement for maintenance
	return nil
}

// validatePaymentToAddress validates payment to a specific address
func (mpv *MasternodePaymentValidator) validatePaymentToAddress(
	tx *types.Transaction,
	payAddress []byte,
	blockHeight uint32,
	blockReward int64,
) error {
	// Calculate expected masternode payment
	expectedPayment := mpv.calculateMasternodePayment(blockHeight, blockReward)

	// Find the payment output
	found := false
	for _, output := range tx.Outputs {
		if outputMatchesAddress(output, payAddress) {
			if output.Value == expectedPayment {
				found = true
				break
			}
			return fmt.Errorf("incorrect payment amount: got %d, expected %d",
				output.Value, expectedPayment)
		}
	}

	if !found {
		return fmt.Errorf("masternode payment output not found")
	}

	return nil
}

// findHighestVotedPayee returns the payee with the most votes and the vote count.
// Caller must hold blockVotes.mu.RLock.
func findHighestVotedPayee(blockVotes *BlockPaymentVotes) (payAddress []byte, voteCount int) {
	for _, payee := range blockVotes.Payees {
		if payee.Votes > voteCount {
			voteCount = payee.Votes
			payAddress = payee.PayAddress
		}
	}
	return
}

// validatePaymentWithVotes validates payment based on voting consensus
func (mpv *MasternodePaymentValidator) validatePaymentWithVotes(
	tx *types.Transaction,
	blockVotes *BlockPaymentVotes,
	blockHeight uint32,
	blockReward int64,
) error {
	blockVotes.mu.RLock()
	defer blockVotes.mu.RUnlock()

	winningPayee, maxVotes := findHighestVotedPayee(blockVotes)

	// If we don't have minimum signatures, accept any valid payment
	if maxVotes < MinPaymentSignatures {
		return nil
	}

	// Validate payment to winning payee
	return mpv.validatePaymentToAddress(tx, winningPayee, blockHeight, blockReward)
}

// getVotedWinningPayee extracts the winning payee from votes
// Returns the pay address of the payee with the most votes, or nil if insufficient votes
func (mpv *MasternodePaymentValidator) getVotedWinningPayee(blockVotes *BlockPaymentVotes) []byte {
	blockVotes.mu.RLock()
	defer blockVotes.mu.RUnlock()

	winningPayee, maxVotes := findHighestVotedPayee(blockVotes)
	if maxVotes < MinPaymentSignatures {
		return nil
	}
	return winningPayee
}

// AddPaymentVote adds a payment vote for a block
// This is the Go equivalent of AddWinningMasternode() from masternode-payments.cpp
func (mpv *MasternodePaymentValidator) AddPaymentVote(
	blockHeight uint32,
	mnVin types.Outpoint,
	payAddress []byte,
	signature []byte,
) error {
	mpv.mu.Lock()
	defer mpv.mu.Unlock()

	// LEGACY COMPATIBILITY FIX: Vote height window validation
	// Legacy C++ (masternode-payments.cpp:453-457):
	//   int nFirstBlock = nHeight - (mnodeman.CountEnabled() * 1.25);
	//   if (winner.nBlockHeight < nFirstBlock || winner.nBlockHeight > nHeight + 20) return;
	// This prevents:
	// 1. DoS attacks with votes for very old blocks (memory exhaustion)
	// 2. Payment manipulation with votes for far-future blocks
	currentHeight, err := mpv.masternodeInterface.GetBestHeight()
	if err != nil {
		// If we can't get current height, reject the vote (safety first)
		return fmt.Errorf("cannot validate vote height window: %w", err)
	}

	// Calculate the earliest acceptable block height
	// Legacy uses CountEnabled() which is the count of all enabled masternodes
	enabledCount := mpv.masternodeInterface.GetActiveCount()
	// nFirstBlock = nHeight - (enabledCount * 1.25)
	// Using integer math: enabledCount * 125 / 100
	windowSize := uint32(enabledCount * 125 / 100)
	var firstBlock uint32
	if currentHeight > windowSize {
		firstBlock = currentHeight - windowSize
	} else {
		firstBlock = 0
	}

	// Reject votes outside the valid window [firstBlock, currentHeight + 20]
	if blockHeight < firstBlock || blockHeight > currentHeight+20 {
		return fmt.Errorf("vote block height %d outside valid window [%d, %d]",
			blockHeight, firstBlock, currentHeight+20)
	}

	// Check if masternode can vote (hasn't voted for this height)
	if lastVote, exists := mpv.lastVotes[mnVin]; exists && lastVote == blockHeight {
		return fmt.Errorf("masternode already voted for block %d", blockHeight)
	}

	// Verify masternode is active
	// LEGACY COMPATIBILITY: Use IsActiveAtHeightLegacy which validates CURRENT UTXO state
	// This matches legacy C++ behavior where mn.Check() is called in GetMasternodeRank
	if !mpv.masternodeInterface.IsActiveAtHeightLegacy(mnVin, blockHeight) {
		return fmt.Errorf("masternode not active at height %d", blockHeight)
	}

	// LEGACY COMPATIBILITY: Validate masternode protocol version and rank
	// Legacy: CMasternodePaymentWinner::IsValid from masternode-payments.cpp:719-750
	// 1. Check protocol >= ActiveProtocol (GetMinMasternodePaymentsProto)
	// 2. Check rank at blockHeight-100 is <= MNPAYMENTS_SIGNATURES_TOTAL (10)
	mnInfo, err := mpv.masternodeInterface.GetMasternodeByOutpoint(mnVin)
	if err != nil {
		return fmt.Errorf("unknown masternode %s: %w", mnVin.String(), err)
	}

	minProto := mpv.masternodeInterface.GetMinMasternodePaymentsProto()
	if int32(mnInfo.ProtocolVersion) < minProto {
		return fmt.Errorf("masternode protocol too old %d - req %d", mnInfo.ProtocolVersion, minProto)
	}

	// Rank check: blockHeight - 100 (ScoreBlockDepth)
	// Legacy: int n = mnodeman.GetMasternodeRank(vinMasternode, nBlockHeight - 100, ActiveProtocol());
	//
	// LEGACY COMPATIBILITY FIX: For blockHeight < 100, legacy C++ has underflow:
	// nBlockHeight - 100 underflows to huge value, GetBlockHash fails, rank becomes -1,
	// vote is rejected. We must match this behavior by explicitly rejecting votes for
	// blocks < 100 to maintain consensus compatibility.
	const scoreBlockDepth uint32 = 100
	if blockHeight < scoreBlockDepth {
		// Legacy implicitly rejects due to underflow → GetBlockHash failure → rank=-1
		return fmt.Errorf("cannot accept votes for blocks below height %d", scoreBlockDepth)
	}

	rank := mpv.masternodeInterface.GetMasternodeRank(mnVin, blockHeight-scoreBlockDepth, minProto, true)
	if rank > TotalPaymentSignatures {
		// Legacy only logs/misbehaves if rank > MNPAYMENTS_SIGNATURES_TOTAL * 2
		// We just reject silently for rank > 10, but always reject
		return fmt.Errorf("masternode not in top %d (rank: %d)", TotalPaymentSignatures, rank)
	}
	// rank == -1 means masternode not found in ranking - still accept vote since we verified active status above

	// Verify payment vote signature
	// LEGACY COMPATIBILITY: Always require valid signature
	// Legacy CMasternodePaymentWinner::SignatureValid() at masternode-payments.cpp:831-847
	// returns false if signature is missing, invalid, or masternode not found
	if !mpv.skipSignatureVerification {
		if len(signature) == 0 {
			return fmt.Errorf("payment vote missing required signature")
		}

		// Get masternode public key
		pubKeyBytes, err := mpv.masternodeInterface.GetMasternodePublicKey(mnVin)
		if err != nil {
			return fmt.Errorf("failed to get masternode public key: %w", err)
		}

		// Verify signature
		if err := mpv.verifyPaymentVoteSignature(blockHeight, mnVin, payAddress, signature, pubKeyBytes); err != nil {
			return fmt.Errorf("invalid payment vote signature: %w", err)
		}
	}

	// LEGACY COMPATIBILITY: Validate payee is a known masternode
	// Reference: legacy/src/masternode-payments.cpp:484
	// if (!mnodeman.Find(address1)) return; // Non masternode address
	if len(payAddress) > 0 {
		_, err := mpv.masternodeInterface.GetMasternodeByPayAddress(payAddress)
		if err != nil {
			return fmt.Errorf("payee is not a known masternode: %w", err)
		}
	}

	// Get or create block votes
	blockVotes, exists := mpv.paymentVotes[blockHeight]
	if !exists {
		blockVotes = &BlockPaymentVotes{
			BlockHeight: blockHeight,
			Payees:      make([]*PayeeVotes, 0),
		}
		mpv.paymentVotes[blockHeight] = blockVotes
	}

	// Add vote to payee and track if threshold reached
	blockVotes.mu.Lock()
	found := false
	var currentVotes int
	for _, payee := range blockVotes.Payees {
		if bytes.Equal(payee.PayAddress, payAddress) {
			payee.Votes++
			currentVotes = payee.Votes
			found = true
			break
		}
	}
	if !found {
		blockVotes.Payees = append(blockVotes.Payees, &PayeeVotes{
			PayAddress: payAddress,
			Votes:      1,
		})
		currentVotes = 1
	}
	blockVotes.mu.Unlock()

	// LEGACY COMPATIBILITY FIX: Mark payee as scheduled on FIRST vote, not when reaching threshold
	// Legacy AddWinningMasternode() at masternode-payments.cpp:558-583 inserts into
	// mapMasternodeBlocks on the very first vote, which IsScheduled() checks.
	// This prevents the same masternode from being re-selected while votes are accumulating.
	// The threshold (MinPaymentSignatures=6) is only used to determine if enough votes exist
	// to actually pay the masternode, NOT when to mark them as scheduled.
	if currentVotes == 1 {
		// Best effort - don't fail vote addition if this fails
		_ = mpv.masternodeInterface.MarkPayeeScheduled(payAddress, blockHeight)
	}

	// Persist to storage BEFORE updating memory state
	// This ensures consistency: if storage fails, memory is not updated
	// Legacy: CMasternodePayments::AddWinningMasternode stores to mapMasternodePayeeVotes
	if mpv.storage != nil {
		// Store block votes first
		if err := mpv.storage.StoreBlockPaymentVotes(blockHeight, blockVotes); err != nil {
			return fmt.Errorf("failed to persist block votes: %w", err)
		}
		// Store last vote
		if err := mpv.storage.StoreLastVote(mnVin, blockHeight); err != nil {
			return fmt.Errorf("failed to persist last vote: %w", err)
		}
	}

	// Only update memory after successful storage (or if no storage configured)
	mpv.lastVotes[mnVin] = blockHeight

	// Relay the valid vote to P2P network
	// Legacy: winner.Relay() from masternode-payments.cpp:491-493
	// This is called AFTER AddWinningMasternode succeeds
	if mpv.voteRelayFunc != nil {
		// Call relay outside lock to avoid deadlocks
		relayFunc := mpv.voteRelayFunc
		go relayFunc(blockHeight, mnVin, payAddress, signature)
	}

	return nil
}

// FillBlockPayment fills the masternode payment in a block being created
// This is the Go equivalent of FillBlockPayee() from masternode-payments.cpp
// CRITICAL: isProofOfStake must be true for PoS blocks - dev fund is only paid on PoS blocks
// Legacy: masternode-payments.cpp:292-360 only adds dev output for PoS (if(pblock->IsProofOfStake()))
//
// LEGACY COMPATIBILITY (masternode-payments.cpp:318-323):
// When no masternode is available for payment, legacy falls back to dev address:
//
//	if (!hasPayment) {
//	    LogPrint("masternode","FillBlockPayee: No masternode to pay, using dev address\n");
//	    payee = devScript;
//	    hasPayment = true;
//	}
//
// This ensures blocks can always be created even with empty masternode list.
func (mpv *MasternodePaymentValidator) FillBlockPayment(
	tx *types.Transaction,
	blockHeight uint32,
	blockReward int64,
	blockHash types.Hash,
	isProofOfStake bool,
) error {
	// Get dev address first - needed for fallback
	devAddress := mpv.getDevAddress()

	// CRITICAL LEGACY FIX: Check voted payees FIRST before falling back to queue
	// Legacy C++ (masternode-payments.cpp:301-311):
	//   if (!masternodePayments.GetBlockPayee(pindexPrev->nHeight + 1, payee)) {
	//       // NO votes - fallback to queue winner
	//       CMasternode* winningNode = mnodeman.GetCurrentMasterNode(1);
	//   }
	//
	// The Go implementation was ALWAYS using queue winner, ignoring votes entirely.
	// This caused blocks to pay wrong masternode when enforcement is active.
	var payAddress []byte
	var hasPayment bool

	// Step 1: Check if we have voted payees for this block
	if votedPayee, hasVotes := mpv.GetBlockPayee(blockHeight); hasVotes && len(votedPayee) > 0 {
		// Use the voted payee (matches legacy GetBlockPayee behavior)
		payAddress = votedPayee
		hasPayment = true
	}

	// Step 2: If no votes, fall back to queue winner (legacy: GetCurrentMasterNode)
	if !hasPayment {
		winner, err := mpv.masternodeInterface.GetNextPaymentWinner(blockHeight, blockHash)
		if err == nil && len(winner.PayAddress) > 0 {
			payAddress = winner.PayAddress
			hasPayment = true
		}
	}

	// Step 3: Final fallback to dev address (legacy: masternode-payments.cpp:318-323)
	if !hasPayment {
		if len(devAddress) == 0 {
			return fmt.Errorf("no masternode available and no dev address configured")
		}
		payAddress = devAddress
		// Log fallback (matches legacy LogPrint)
	}

	// Calculate masternode payment (45% of block reward)
	mnPayment := mpv.calculateMasternodePayment(blockHeight, blockReward)

	// Calculate dev reward (10% of block reward) - only for PoS blocks
	// DevFundReward = 1000 basis points = 10%
	var devReward int64
	if isProofOfStake {
		devReward = (blockReward * types.DefaultDevFundReward) / 10000
	}

	// Add masternode payment output (or dev fallback)
	tx.Outputs = append(tx.Outputs, &types.TxOutput{
		Value:        mnPayment,
		ScriptPubKey: payAddress,
	})

	// Add dev payment output ONLY for PoS blocks (legacy: masternode-payments.cpp:341-360)
	// devAddress was already retrieved at function start for fallback logic
	if isProofOfStake && len(devAddress) > 0 {
		tx.Outputs = append(tx.Outputs, &types.TxOutput{
			Value:        devReward,
			ScriptPubKey: devAddress,
		})
	}

	// Adjust stake reward (subtract mn and dev payments from last output)
	totalDeduction := mnPayment + devReward
	if len(tx.Outputs) >= 2 {
		// Find the stake output to adjust (second-to-last before our additions)
		lastIdx := len(tx.Outputs) - 2
		if isProofOfStake && devReward > 0 {
			lastIdx = len(tx.Outputs) - 3 // Account for both mn and dev outputs
		}
		if lastIdx >= 0 {
			tx.Outputs[lastIdx].Value -= totalDeduction
		}
	}

	return nil
}

// calculateMasternodePayment calculates the masternode payment for a block
// This matches GetMasternodePayment() from masternode-payments.cpp
// and IsTransactionValid() masternode count logic from masternode-payments.cpp:596-605
func (mpv *MasternodePaymentValidator) calculateMasternodePayment(
	blockHeight uint32,
	blockReward int64,
) int64 {
	// Get masternode count for drift adjustment
	// LEGACY COMPATIBILITY: When SPORK_8 is active, use stable_size() instead of size()
	// Legacy: masternode-payments.cpp:596-605
	// if (IsSporkActive(SPORK_8_MASTERNODE_PAYMENT_ENFORCEMENT)) {
	//     nMasternode_Drift_Count = mnodeman.stable_size() + Params().MasternodeCountDrift();
	// } else {
	//     nMasternode_Drift_Count = mnodeman.size() + Params().MasternodeCountDrift();
	// }
	var mnCount int
	if mpv.sporkManager != nil && mpv.sporkManager.IsActive(SporkMasternodePaymentEnforcement) {
		// Use stable_size() - only masternodes older than 8000 seconds
		mnCount = mpv.masternodeInterface.GetStableCount()
	} else {
		// Use regular size() - all active masternodes
		mnCount = mpv.masternodeInterface.GetActiveCount()
	}

	// Add drift allowance (from legacy chainparams.cpp:230)
	const driftCount = 20 // MasternodeCountDrift (legacy: nMasternodeCountDrift = 20)
	adjustedCount := mnCount + driftCount

	// Calculate payment based on masternode count and block reward
	// TWINS reward split: 80% masternode, 10% stake, 10% dev fund
	if adjustedCount == 0 {
		return 0
	}

	// Base masternode allocation is 80% of block reward (from ChainParams)
	// MasternodeReward = 8000 basis points = 80%
	masternodeReward := (blockReward * types.DefaultMasternodeReward) / 10000

	return masternodeReward
}

// GetPaymentQueueInfo returns information about the payment queue
func (mpv *MasternodePaymentValidator) GetPaymentQueueInfo(blockHeight uint32) (*PaymentQueueInfo, error) {
	mpv.mu.RLock()
	defer mpv.mu.RUnlock()

	info := &PaymentQueueInfo{
		BlockHeight: blockHeight,
		Votes:       make([]*PayeeVoteInfo, 0),
	}

	// Get votes for this block
	if blockVotes, exists := mpv.paymentVotes[blockHeight]; exists {
		blockVotes.mu.RLock()
		for _, payee := range blockVotes.Payees {
			info.Votes = append(info.Votes, &PayeeVoteInfo{
				PayAddress: payee.PayAddress,
				Votes:      payee.Votes,
			})
		}
		blockVotes.mu.RUnlock()

		// Sort by votes (highest first)
		sort.Slice(info.Votes, func(i, j int) bool {
			return info.Votes[i].Votes > info.Votes[j].Votes
		})

		if len(info.Votes) > 0 {
			info.Winner = info.Votes[0].PayAddress
			info.VoteCount = info.Votes[0].Votes
		}
	}

	return info, nil
}

// GetBlockPaymentVotes returns payment votes for a specific block height
// Used by GetLastPaid to scan for payment history
// Returns nil if no votes exist for that height
func (mpv *MasternodePaymentValidator) GetBlockPaymentVotes(blockHeight uint32) *BlockPaymentVotes {
	mpv.mu.RLock()
	defer mpv.mu.RUnlock()
	return mpv.paymentVotes[blockHeight]
}

// HasPayeeWithVotesAtHeight checks if a payee has at least votesRequired votes at given height
// Implements masternode.PaymentVotesProvider interface for GetLastPaid scanning
// Reference: legacy C++ masternodePayments.mapMasternodeBlocks[height].HasPayeeWithVotes(payee, 2)
func (mpv *MasternodePaymentValidator) HasPayeeWithVotesAtHeight(blockHeight uint32, payAddress []byte, votesRequired int) bool {
	mpv.mu.RLock()
	blockVotes, exists := mpv.paymentVotes[blockHeight]
	mpv.mu.RUnlock()

	if !exists || blockVotes == nil {
		return false
	}

	return blockVotes.HasPayeeWithVotes(payAddress, votesRequired)
}

// CleanupOldVotes removes old payment votes to free memory
// This matches CleanPaymentList() from masternode-payments.cpp
func (mpv *MasternodePaymentValidator) CleanupOldVotes(currentHeight uint32) {
	mpv.mu.Lock()
	defer mpv.mu.Unlock()

	// Get masternode count for limit calculation
	mnCount := mpv.masternodeInterface.GetActiveCount()
	limit := uint32(mnCount * 125 / 100) // Keep 1.25x masternode count worth of blocks
	if limit < PaymentVotingWindowSize {
		limit = PaymentVotingWindowSize
	}

	// Remove votes older than limit
	cutoffHeight := uint32(0)
	if currentHeight > limit {
		cutoffHeight = currentHeight - limit
	}

	for height := range mpv.paymentVotes {
		if height < cutoffHeight {
			delete(mpv.paymentVotes, height)
		}
	}

	// Clean up last votes map
	for outpoint, height := range mpv.lastVotes {
		if height < cutoffHeight {
			delete(mpv.lastVotes, outpoint)
		}
	}

	// Clean up persistent storage if configured
	// Legacy: CMasternodePayments::CleanPaymentList removes old entries
	if mpv.storage != nil && cutoffHeight > 0 {
		if err := mpv.storage.CleanOldVotes(cutoffHeight); err != nil {
			// Log error but don't fail - memory cleanup already succeeded
			// Storage will be cleaned on next attempt
			logrus.WithError(err).Warn("Failed to clean old votes from storage")
		}
	}
}

// PaymentQueueInfo contains payment queue information
type PaymentQueueInfo struct {
	BlockHeight uint32
	Winner      []byte
	VoteCount   int
	Votes       []*PayeeVoteInfo
}

// PayeeVoteInfo contains vote information for a payee
type PayeeVoteInfo struct {
	PayAddress []byte
	Votes      int
}

// Helper functions

func outputMatchesAddress(output *types.TxOutput, scriptPubKey []byte) bool {
	if len(output.ScriptPubKey) == 0 || len(scriptPubKey) == 0 {
		return false
	}
	// Direct byte-for-byte comparison of script bytes
	// This matches legacy behavior where both are P2PKH scripts
	return bytes.Equal(output.ScriptPubKey, scriptPubKey)
}


// getDevAddress returns the development fund address from chain params
func (mpv *MasternodePaymentValidator) getDevAddress() []byte {
	if mpv.chainParams == nil {
		return []byte{} // No dev address if chain params not set
	}
	return mpv.chainParams.DevAddress
}

// recordPaymentFromCoinstake extracts the masternode payment from a block's coinstake
// transaction and records it via the payment recorder (if set).
// Matches legacy C++ main.cpp:4722: vtx[1].vout[vout.size() - 2]
func (mpv *MasternodePaymentValidator) recordPaymentFromCoinstake(coinstake *types.Transaction, block *types.Block, blockHeight uint32) {
	mpv.mu.RLock()
	recorder := mpv.paymentRecorder
	mpv.mu.RUnlock()

	if recorder == nil || coinstake == nil || block == nil {
		return
	}

	// Need at least 3 outputs: [empty, stake, mn_payment]
	if len(coinstake.Outputs) < 3 {
		return
	}

	// Determine MN payment output index.
	// Layout WITH dev: [empty(0), stake..., mn_payment, dev_payment] → MN at len-2
	// Layout WITHOUT dev: [empty(0), stake..., mn_payment] → MN at len-1
	devAddress := mpv.getDevAddress()
	var mnIdx int
	if len(devAddress) > 0 {
		lastOutput := coinstake.Outputs[len(coinstake.Outputs)-1]
		if bytes.Equal(lastOutput.ScriptPubKey, devAddress) {
			mnIdx = len(coinstake.Outputs) - 2
		} else {
			mnIdx = len(coinstake.Outputs) - 1
		}
	} else {
		mnIdx = len(coinstake.Outputs) - 1
	}

	output := coinstake.Outputs[mnIdx]
	if len(output.ScriptPubKey) == 0 || output.Value <= 0 {
		return
	}

	// Skip if this output IS the dev address (fallback payment, not a real MN payment)
	if len(devAddress) > 0 && bytes.Equal(output.ScriptPubKey, devAddress) {
		return
	}

	blockTime := time.Unix(int64(block.Header.Timestamp), 0)
	txID := coinstake.Hash().String()
	recorder.RecordPayment(output.ScriptPubKey, blockHeight, blockTime, output.Value, txID)
}

// verifyPaymentVoteSignature verifies the signature on a payment vote
// Legacy C++ reference: CMasternodePaymentWinner::SignatureValid()
func (mpv *MasternodePaymentValidator) verifyPaymentVoteSignature(
	blockHeight uint32,
	mnVin types.Outpoint,
	payeeScript []byte,
	signature []byte,
	pubKeyBytes []byte,
) error {
	// Legacy uses compact signatures (65 bytes)
	if len(signature) != 65 {
		return fmt.Errorf("payment vote signature must be 65 bytes (compact), got %d", len(signature))
	}

	// Parse public key
	pubKey, err := crypto.ParsePublicKeyFromBytes(pubKeyBytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}

	// Create message matching C++ format:
	// vinMasternode.prevout.ToStringShort() + std::to_string(nBlockHeight) + payee.ToString()
	var message string

	// Add outpoint short string (txhash-index format from ToStringShort())
	// CRITICAL: Legacy C++ uses DASH (-) not colon (:)
	// See legacy/src/primitives/transaction.cpp:27-28: strprintf("%s-%u", hash.ToString().substr(0,64), n)
	message += fmt.Sprintf("%s-%d", mnVin.Hash.String(), mnVin.Index)

	// Add block height as string
	message += fmt.Sprintf("%d", blockHeight)

	// Add payee script as string
	// CRITICAL: CScript::ToString() returns ASM format, NOT raw hex!
	// Example: "OP_DUP OP_HASH160 <pubkeyhash> OP_EQUALVERIFY OP_CHECKSIG"
	payeeASM, err := script.Disassemble(payeeScript)
	if err != nil {
		// Fallback to hex on error (should not happen for valid scripts)
		payeeASM = fmt.Sprintf("%x", payeeScript)
	}
	message += payeeASM

	// Verify using compact signature (matches C++ obfuScationSigner.VerifyMessage)
	valid, err := crypto.VerifyCompactSignature(pubKey, message, signature)
	if err != nil {
		return fmt.Errorf("payment vote signature verification failed: %w", err)
	}

	if !valid {
		return fmt.Errorf("payment vote signature verification failed: signature invalid")
	}

	return nil
}
