package masternode

import (
	"bytes"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/twins-dev/twins-core/internal/consensus"
	"github.com/twins-dev/twins-core/pkg/crypto"
	"github.com/twins-dev/twins-core/pkg/script"
	"github.com/twins-dev/twins-core/pkg/types"
)

// CoinUnit is the number of satoshis in 1 TWINS (base denomination unit)
const CoinUnit int64 = 1e8

// Masternode tier collateral amounts (in satoshis)
const (
	TierBronzeCollateral   = 1000000 * 1e8
	TierSilverCollateral   = 5000000 * 1e8
	TierGoldCollateral     = 20000000 * 1e8
	TierPlatinumCollateral = 100000000 * 1e8
)

// Spork IDs used by masternode module
// Duplicated here to avoid circular import with internal/spork
const (
	// SporkTwinsEnableMasternodeTiers controls multi-tier masternode support
	// When OFF (default): Only Bronze tier (1M TWINS) is accepted
	// When ON: All 4 tiers (Bronze/Silver/Gold/Platinum) are accepted
	// Legacy C++ reference: SPORK_TWINS_01_ENABLE_MASTERNODE_TIERS (20190001)
	SporkTwinsEnableMasternodeTiers int32 = 20190001
)

// Masternode tier selection probabilities (legacy weights from chainparams.cpp:234)
// These are relative weights used for weighted random selection, not fixed percentages
const (
	TierBronzeProbability   = 1   // Weight 1 (lowest priority)
	TierSilverProbability   = 5   // Weight 5
	TierGoldProbability     = 20  // Weight 20
	TierPlatinumProbability = 100 // Weight 100 (highest priority)
)

// MasternodeTier represents the masternode tier level
type MasternodeTier int

const (
	Bronze MasternodeTier = iota
	Silver
	Gold
	Platinum
)

func (t MasternodeTier) String() string {
	switch t {
	case Bronze:
		return "bronze"
	case Silver:
		return "silver"
	case Gold:
		return "gold"
	case Platinum:
		return "platinum"
	default:
		return "unknown"
	}
}

// Collateral returns the required collateral amount for the tier
func (t MasternodeTier) Collateral() int64 {
	switch t {
	case Bronze:
		return TierBronzeCollateral
	case Silver:
		return TierSilverCollateral
	case Gold:
		return TierGoldCollateral
	case Platinum:
		return TierPlatinumCollateral
	default:
		return 0
	}
}

// SelectionWeight returns the probability weight for masternode selection
// Higher weights = higher chance of being selected for payment
func (t MasternodeTier) SelectionWeight() int {
	switch t {
	case Bronze:
		return TierBronzeProbability
	case Silver:
		return TierSilverProbability
	case Gold:
		return TierGoldProbability
	case Platinum:
		return TierPlatinumProbability
	default:
		return 0
	}
}

// BlockchainReader provides read-only access to blockchain state
// for masternode operations (ping creation, score calculation, etc.)
type BlockchainReader interface {
	// GetBestHeight returns the current chain tip height
	GetBestHeight() (uint32, error)
	// GetBlockByHeight returns a block by its height
	GetBlockByHeight(height uint32) (*types.Block, error)
}

// UTXOChecker provides UTXO validation for masternode collateral checks
// Legacy C++ re-validates UTXO each status check cycle (masternode.cpp:235-260)
type UTXOChecker interface {
	// IsUTXOSpent checks if a UTXO has been spent
	// Returns true if spent, false if unspent
	IsUTXOSpent(outpoint types.Outpoint) (bool, error)

	// GetUTXOValue returns the value of a UTXO in satoshis
	// Returns 0 and error if UTXO is spent or not found
	// Used for spork-aware collateral validation (legacy: isMasternodeCollateral)
	GetUTXOValue(outpoint types.Outpoint) (int64, error)
}

// PingBlockDepth is how far back from chain tip to get block hash for pings
// Legacy C++ uses tip - 12 (see activemasternode.cpp:SendMasternodePing)
const PingBlockDepth = 12

// ScoreBlockDepth is how far back from current height to get block hash for scoring
// Legacy C++ uses blockHeight - 100 (see masternodeman.cpp:GetNextMasternodeInQueueForPayment)
const ScoreBlockDepth uint32 = 100

// WinnerVoteBlocksAhead is how many blocks ahead to vote for payment winners
// Legacy C++ votes for currentHeight + 10 (see masternode-payments.cpp:ProcessBlock)
const WinnerVoteBlocksAhead uint32 = 10

// =============================================================================
// Masternode Winner Vote (mnw message)
// =============================================================================

// MasternodeWinnerVote represents an outgoing masternode winner vote.
// This is used to create and broadcast mnw messages to the network.
// Matches legacy CMasternodePaymentWinner from masternode-payments.h
type MasternodeWinnerVote struct {
	// VoterOutpoint is the collateral outpoint of the voting masternode (vinMasternode)
	VoterOutpoint types.Outpoint

	// BlockHeight is the block height this vote is for (nBlockHeight)
	// Legacy votes for currentHeight + 10
	BlockHeight uint32

	// PayeeScript is the payment script (scriptPubKey) for the winning masternode
	// This is a P2PKH script derived from the winner's collateral public key
	PayeeScript []byte

	// Signature is the compact signature over the vote message
	// Signed with the voting masternode's operator private key
	Signature []byte
}

// GetSignatureMessage returns the string message to sign for winner votes.
// CRITICAL: Must match legacy CMasternodePaymentWinner::Sign() format EXACTLY.
// Legacy C++ (masternode-payments.cpp:503):
//
//	strMessage = vinMasternode.prevout.ToStringShort() + std::to_string(nBlockHeight) + payee.ToString()
//
// Where:
//   - ToStringShort() = "hash-index" (hash is hex-reversed, index is decimal)
//   - std::to_string(nBlockHeight) = decimal string of block height
//   - payee.ToString() = CScript::ToString() which returns ASM like "OP_DUP OP_HASH160 <hash> OP_EQUALVERIFY OP_CHECKSIG"
//
// Result: "<hash>-<index><height><asm>" (NO separators between components)
func (v *MasternodeWinnerVote) GetSignatureMessage() string {
	// CRITICAL: Use script.Disassemble() to match legacy CScript::ToString()
	// Legacy C++ returns ASM format, not raw hex!
	payeeASM, err := script.Disassemble(v.PayeeScript)
	if err != nil {
		// Fallback to hex on error (should not happen for valid scripts)
		payeeASM = fmt.Sprintf("%x", v.PayeeScript)
	}
	// Format: "hash-index" + "blockheight" + "asm script"
	// Note: ToStringShort uses reversed hash (big-endian display)
	return fmt.Sprintf("%s-%d%d%s",
		v.VoterOutpoint.Hash.String(), // Big-endian (reversed) to match legacy
		v.VoterOutpoint.Index,
		v.BlockHeight,
		payeeASM)
}

// GetHash returns the hash of this vote for inventory purposes
// CRITICAL: Must match legacy C++ CMasternodePaymentWinner::GetHash() exactly
// Legacy format from masternode-payments.h:185-193:
//
//	CHashWriter ss(SER_GETHASH, PROTOCOL_VERSION);
//	ss << payee;                    // 1. CScript (varbytes: compactsize + data)
//	ss << nBlockHeight;             // 2. uint32 (little-endian)
//	ss << vinMasternode.prevout;    // 3. COutPoint (hash + uint32 index)
//	return ss.GetHash();            // Double SHA-256
func (v *MasternodeWinnerVote) GetHash() types.Hash {
	buf := new(bytes.Buffer)

	// 1. Serialize payee (CScript format: compactsize length + raw bytes)
	types.WriteCompactSize(buf, uint64(len(v.PayeeScript)))
	buf.Write(v.PayeeScript)

	// 2. Serialize block height (uint32, little-endian)
	binary.Write(buf, binary.LittleEndian, v.BlockHeight)

	// 3. Serialize outpoint (COutPoint: 32-byte hash + uint32 index, both little-endian)
	buf.Write(v.VoterOutpoint.Hash[:])
	binary.Write(buf, binary.LittleEndian, v.VoterOutpoint.Index)

	// Double SHA-256 to match legacy CHashWriter::GetHash()
	return types.NewHash(buf.Bytes())
}

// Serialize produces the wire format for P2P transmission.
// CRITICAL: Format MUST match legacy C++ serialization exactly for network compatibility.
// Legacy format from masternode-payments.h CMasternodePaymentWinner::SerializationOp():
//   - vinMasternode (CTxIn: hash + index + scriptSig + sequence)
//   - nBlockHeight (uint32)
//   - payee (CScript as varbytes)
//   - vchSig (signature as varbytes)
func (v *MasternodeWinnerVote) Serialize() ([]byte, error) {
	buf := new(bytes.Buffer)

	// 1. vinMasternode as CTxIn
	// CTxIn format: prevout (COutPoint) + scriptSig (varbytes) + nSequence (uint32)
	// COutPoint format: hash (32 bytes) + n (uint32)

	// Write prevout hash (32 bytes, little-endian byte order)
	if _, err := buf.Write(v.VoterOutpoint.Hash[:]); err != nil {
		return nil, fmt.Errorf("failed to write outpoint hash: %w", err)
	}

	// Write prevout index (uint32, little-endian)
	if err := binary.Write(buf, binary.LittleEndian, v.VoterOutpoint.Index); err != nil {
		return nil, fmt.Errorf("failed to write outpoint index: %w", err)
	}

	// Write scriptSig as varbytes (empty for masternode collateral)
	// Legacy: CTxIn has empty scriptSig for masternode collateral inputs
	if err := writeVarInt(buf, 0); err != nil {
		return nil, fmt.Errorf("failed to write scriptSig length: %w", err)
	}

	// Write nSequence (uint32, 0xFFFFFFFF for final)
	// Legacy: CTxIn default sequence is std::numeric_limits<uint32_t>::max()
	if err := binary.Write(buf, binary.LittleEndian, uint32(0xFFFFFFFF)); err != nil {
		return nil, fmt.Errorf("failed to write sequence: %w", err)
	}

	// 2. nBlockHeight (uint32, little-endian)
	if err := binary.Write(buf, binary.LittleEndian, v.BlockHeight); err != nil {
		return nil, fmt.Errorf("failed to write block height: %w", err)
	}

	// 3. payee (CScript as varbytes)
	if err := writeVarInt(buf, uint64(len(v.PayeeScript))); err != nil {
		return nil, fmt.Errorf("failed to write payee length: %w", err)
	}
	if _, err := buf.Write(v.PayeeScript); err != nil {
		return nil, fmt.Errorf("failed to write payee script: %w", err)
	}

	// 4. vchSig (signature as varbytes)
	if err := writeVarInt(buf, uint64(len(v.Signature))); err != nil {
		return nil, fmt.Errorf("failed to write signature length: %w", err)
	}
	if _, err := buf.Write(v.Signature); err != nil {
		return nil, fmt.Errorf("failed to write signature: %w", err)
	}

	return buf.Bytes(), nil
}

// MasternodePayee represents a single payee with vote count
// LEGACY COMPATIBILITY: Matches CMasternodePayee serialization
// C++ Reference: masternode-payments.h:75-88
type MasternodePayee struct {
	ScriptPubKey []byte // scriptPubKey (CScript)
	Votes        int    // nVotes - number of votes for this payee
}

// MasternodeBlockPayees stores accumulated votes per block for payee selection
// LEGACY COMPATIBILITY: Matches CMasternodeBlockPayees structure
// C++ Reference: masternode-payments.h:90-159
type MasternodeBlockPayees struct {
	BlockHeight uint32             // nBlockHeight
	Payees      []*MasternodePayee // vecPayees - list of payees with vote counts
	mu          sync.RWMutex       // Thread-safe access
}

// NewMasternodeBlockPayees creates a new block payees tracker
func NewMasternodeBlockPayees(blockHeight uint32) *MasternodeBlockPayees {
	return &MasternodeBlockPayees{
		BlockHeight: blockHeight,
		Payees:      make([]*MasternodePayee, 0),
	}
}

// AddPayee adds or increments votes for a payee script
// LEGACY COMPATIBILITY: Matches CMasternodeBlockPayees::AddPayee()
// C++ Reference: masternode-payments.cpp:31-44
func (bp *MasternodeBlockPayees) AddPayee(scriptPubKey []byte, increment int) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	for i := range bp.Payees {
		if bytes.Equal(bp.Payees[i].ScriptPubKey, scriptPubKey) {
			bp.Payees[i].Votes += increment
			return
		}
	}
	bp.Payees = append(bp.Payees, &MasternodePayee{
		ScriptPubKey: scriptPubKey,
		Votes:        increment,
	})
}

// GetPayee returns the payee script with the most votes
// LEGACY COMPATIBILITY: Matches CMasternodeBlockPayees::GetPayee()
// C++ Reference: masternode-payments.cpp:46-63
func (bp *MasternodeBlockPayees) GetPayee() []byte {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	var bestPayee []byte
	maxVotes := -1
	for _, p := range bp.Payees {
		if p.Votes > maxVotes {
			maxVotes = p.Votes
			bestPayee = p.ScriptPubKey
		}
	}
	return bestPayee
}

// HasPayeeWithVotes checks if a specific payee has minimum required votes
// LEGACY COMPATIBILITY: Matches CMasternodeBlockPayees::HasPayeeWithVotes()
// C++ Reference: masternode-payments.cpp:65-77
// Used to verify MASTERNODE_PAYMENT_SIGNATURES (6) requirement
func (bp *MasternodeBlockPayees) HasPayeeWithVotes(scriptPubKey []byte, votesRequired int) bool {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	for _, p := range bp.Payees {
		if p.Votes >= votesRequired && bytes.Equal(p.ScriptPubKey, scriptPubKey) {
			return true
		}
	}
	return false
}

// GetTierFromCollateral returns the tier based on collateral amount
func GetTierFromCollateral(collateral int64) (MasternodeTier, error) {
	switch collateral {
	case TierBronzeCollateral:
		return Bronze, nil
	case TierSilverCollateral:
		return Silver, nil
	case TierGoldCollateral:
		return Gold, nil
	case TierPlatinumCollateral:
		return Platinum, nil
	default:
		return Bronze, fmt.Errorf("invalid collateral amount: %d", collateral)
	}
}

// MasternodeStatus represents the status of a masternode
// CRITICAL: Values MUST match legacy C++ enum order for P2P serialization compatibility
// Legacy enum from masternode.h lines 116-126:
//   MASTERNODE_PRE_ENABLED=0, MASTERNODE_ENABLED=1, MASTERNODE_EXPIRED=2,
//   MASTERNODE_OUTPOINT_SPENT=3, MASTERNODE_REMOVE=4, MASTERNODE_WATCHDOG_EXPIRED=5,
//   MASTERNODE_POSE_BAN=6, MASTERNODE_VIN_SPENT=7, MASTERNODE_POS_ERROR=8
type MasternodeStatus int

const (
	StatusPreEnabled    MasternodeStatus = iota // 0 - Legacy: MASTERNODE_PRE_ENABLED
	StatusEnabled                               // 1 - Legacy: MASTERNODE_ENABLED
	StatusExpired                               // 2 - Legacy: MASTERNODE_EXPIRED
	StatusOutpointSpent                         // 3 - Legacy: MASTERNODE_OUTPOINT_SPENT
	StatusRemoved                               // 4 - Legacy: MASTERNODE_REMOVE
	StatusWatchdog                              // 5 - Legacy: MASTERNODE_WATCHDOG_EXPIRED
	StatusPoseban                               // 6 - Legacy: MASTERNODE_POSE_BAN
	StatusVinSpent                              // 7 - Legacy: MASTERNODE_VIN_SPENT
	StatusPosError                              // 8 - Legacy: MASTERNODE_POS_ERROR

	// StatusInactive is Go-internal only and MUST NOT be serialized to P2P or cache.
	// Legacy C++ has only 9 states (0-8). If this value is persisted, C++ nodes will
	// encounter an unknown enum value, potentially causing undefined behavior.
	// Used only for internal Go bookkeeping that doesn't need cross-node consensus.
	// WARNING: Do not use for masternodes that will be saved to mncache.dat!
	StatusInactive // 9 - Go-internal ONLY, not in legacy C++
)

func (s MasternodeStatus) String() string {
	switch s {
	case StatusPreEnabled:
		return "pre-enabled"
	case StatusEnabled:
		return "enabled"
	case StatusExpired:
		return "expired"
	case StatusOutpointSpent:
		return "outpoint-spent"
	case StatusRemoved:
		return "removed"
	case StatusWatchdog:
		return "watchdog-expired"
	case StatusPoseban:
		return "pose-ban"
	case StatusVinSpent:
		return "vin-spent"
	case StatusPosError:
		return "pos-error"
	case StatusInactive:
		return "inactive"
	default:
		return "unknown"
	}
}

// Masternode represents a masternode in the network
type Masternode struct {
	// Identity
	OutPoint         types.Outpoint    `json:"outpoint"`
	Addr             net.Addr          `json:"addr"`
	PubKey           *crypto.PublicKey `json:"pubkey"`            // Operator key
	PubKeyCollateral *crypto.PublicKey `json:"pubkey_collateral"` // Collateral key (from broadcast)
	Signature        []byte            `json:"signature"`         // Original broadcast signature
	SigTime          int64             `json:"sigtime"`           // Signature timestamp
	Tier             MasternodeTier    `json:"tier"`
	Collateral       int64             `json:"collateral"`

	// Status
	Status             MasternodeStatus `json:"status"`
	Protocol           int32            `json:"protocol"`
	ActiveSince        time.Time        `json:"activesince"`
	ActiveHeight       uint32           `json:"activeheight"`        // Block height when masternode became active
	CollateralTxHeight uint32           `json:"collateral_tx_height"` // Block height when collateral TX was confirmed (for input age)
	LastPing           time.Time        `json:"lastping"`            // Timestamp of last ping (for display)
	LastPingMessage    *MasternodePing  `json:"lastpingmessage"`     // Full ping message for serialization
	LastPaid           time.Time        `json:"lastpaid"`
	LastSeen           time.Time        `json:"lastseen"`
	BlockHeight        int32            `json:"blockheight"`

	// Network
	SentinelVersion string    `json:"sentinelversion"`
	SentinelPing    time.Time `json:"sentinelping"`

	// Voting and governance
	VoteHash types.Hash `json:"votehash"`
	VoteTime time.Time  `json:"votetime"`

	// Performance tracking
	// Score is full 32-byte hash for deterministic ordering (matches legacy uint256)
	Score        types.Hash `json:"-"`                // Internal full score
	ScoreCompact float64    `json:"score"`            // Compact score for JSON API (first 8 bytes as float64)
	Rank         int        `json:"rank"`
	PaymentCount int64      `json:"paymentcount"`

	// Payment cycle tracking (legacy compatibility)
	// Used by SecondsSincePayment() for payment selection
	PrevCycleLastPaymentTime int64      `json:"prev_cycle_last_payment_time"` // Previous cycle's last payment timestamp
	PrevCycleLastPaymentHash types.Hash `json:"prev_cycle_last_payment_hash"` // Previous cycle's last payment block hash
	WinsThisCycle            int        `json:"wins_this_cycle"`              // Wins in current payment cycle

	// Legacy C++ compatibility fields (Issue #16, #17)
	// These fields are required for mncache.dat compatibility with legacy nodes
	LastDsq                       int64 `json:"last_dsq"`                         // nLastDsq - Last DarkSend queue position (legacy obfuscation)
	ScanningErrorCount            int   `json:"scanning_error_count"`             // nScanningErrorCount - Masternode scanning errors
	LastScanningErrorBlockHeight  int32 `json:"last_scanning_error_block_height"` // nLastScanningErrorBlockHeight - Height of last scan error

	// Cache fields for input age optimization (legacy compatibility)
	// These avoid re-scanning blockchain for input age on every check
	CacheInputAge      int   `json:"cache_input_age"`       // cacheInputAge - Cached number of confirmations
	CacheInputAgeBlock int32 `json:"cache_input_age_block"` // cacheInputAgeBlock - Height when cache was last updated

	// Legacy bool fields for serialization compatibility
	UnitTest     bool `json:"unit_test"`      // unitTest - Testing flag (legacy)
	AllowFreeTx  bool `json:"allow_free_tx"`  // allowFreeTx - Free transaction flag (legacy obfuscation)

	mu sync.RWMutex
}

// =============================================================================
// Legacy C++ Compatible String Formatting
// =============================================================================
// These functions produce strings that EXACTLY match the legacy C++ format
// for signature message construction. This is CRITICAL for network compatibility.
// Changing these formats would break signature verification with legacy nodes.
//
// MALLEABILITY NOTE: String concatenation without delimiters has theoretical
// malleability concerns, but changing the format would break protocol compatibility.
// The components (hashes, indices, timestamps) have fixed formats that mitigate risk.
// =============================================================================

// LegacyOutpointString formats an outpoint like C++ COutPoint::ToString()
// Format: "COutPoint(%s, %u)" where %s is the hex hash and %u is the index
// Example: "COutPoint(0000000000000000000000000000000000000000000000000000000000001234, 0)"
func LegacyOutpointString(outpoint types.Outpoint) string {
	return fmt.Sprintf("COutPoint(%s, %d)", outpoint.Hash.String(), outpoint.Index)
}

// LegacyTxInString formats an outpoint as a CTxIn like C++ CTxIn::ToString()
// For masternode collateral, scriptSig is empty and nSequence is max uint32
// Format: "CTxIn(COutPoint(%s, %u), scriptSig=)"
// Note: nSequence is omitted when it's at max value (0xFFFFFFFF)
func LegacyTxInString(outpoint types.Outpoint) string {
	// For masternode collateral, scriptSig is always empty and nSequence is max
	// This matches the C++ behavior where CTxIn is created with just the outpoint
	return fmt.Sprintf("CTxIn(%s, scriptSig=)", LegacyOutpointString(outpoint))
}

// MasternodeBroadcast represents a masternode announcement broadcast
type MasternodeBroadcast struct {
	OutPoint         types.Outpoint    `json:"outpoint"`
	Addr             net.Addr          `json:"addr"`
	PubKeyCollateral *crypto.PublicKey `json:"pubkey_collateral"` // Key for collateral address
	PubKeyMasternode *crypto.PublicKey `json:"pubkey_masternode"` // Key for masternode operator
	Signature        []byte            `json:"signature"`
	SigTime          int64             `json:"sigtime"`
	Protocol         int32             `json:"protocol"`
	LastPing         *MasternodePing   `json:"lastping"`
	LastDsq          int64             `json:"last_dsq"` // Last DarkSend queue position

	// Raw public key bytes for serialization (if set, used instead of deriving from PubKey fields)
	// This allows preserving the exact format (compressed vs uncompressed) from the original UTXO
	// Empty means use SerializeCompressed() from the PubKey fields (default for modern wallets)
	PubKeyCollateralBytes []byte `json:"-"`
	PubKeyMasternodeBytes []byte `json:"-"`
}

// GetHash returns the hash used for inventory (inv) messages
// Matches legacy CMasternodeBroadcast::GetHash() behavior:
// Hash of (sigTime + pubKeyCollateralAddress)
func (mnb *MasternodeBroadcast) GetHash() types.Hash {
	// LEGACY COMPATIBILITY FIX: Match C++ serialization format
	// Legacy (masternode.h:345-351):
	//   CHashWriter ss(SER_GETHASH, PROTOCOL_VERSION);
	//   ss << sigTime;           // int64_t (8 bytes, little-endian)
	//   ss << pubKeyCollateralAddress;  // CPubKey with CompactSize prefix
	// CPubKey serialization (pubkey.h:131-135):
	//   WriteCompactSize(s, len);  // 1 byte for len<=252 (33 for compressed)
	//   s.write(vch, len);         // raw pubkey bytes
	buf := make([]byte, 0, 8+1+33) // sigTime (8) + CompactSize (1) + pubkey (33)

	// Append sigTime (8 bytes, little endian)
	sigTimeBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(sigTimeBytes, uint64(mnb.SigTime))
	buf = append(buf, sigTimeBytes...)

	// Append compressed public key with CompactSize length prefix
	// For 33-byte compressed pubkey, CompactSize is single byte 0x21 (33)
	pubKeyBytes := mnb.PubKeyCollateral.SerializeCompressed()
	buf = append(buf, byte(len(pubKeyBytes))) // CompactSize prefix (1 byte for len <= 252)
	buf = append(buf, pubKeyBytes...)

	// Double SHA256 hash (Bitcoin/TWINS standard)
	hash1 := sha256.Sum256(buf)
	hash2 := sha256.Sum256(hash1[:])

	return hash2
}

// MasternodePing represents a masternode ping message
type MasternodePing struct {
	OutPoint        types.Outpoint `json:"outpoint"`
	BlockHash       types.Hash     `json:"blockhash"`
	SigTime         int64          `json:"sigtime"`
	Signature       []byte         `json:"signature"`
	SentinelPing    bool           `json:"sentinelping"`
	SentinelVersion string         `json:"sentinelversion"`
}

// GetHash returns the hash used for inventory (inv) messages and seenPings deduplication
// Matches legacy CMasternodePing::GetHash() behavior (masternode.h:68-74):
// Hash of serialized (vin + sigTime) - blockHash is NOT included in hash
func (mnp *MasternodePing) GetHash() types.Hash {
	// LEGACY COMPATIBILITY FIX: Match C++ CTxIn serialization
	// Legacy (masternode.h:68-74): Hash of (vin + sigTime)
	// CTxIn serialization (transaction.h:86-90):
	//   READWRITE(prevout);    // COutPoint: hash (32) + n (4) = 36 bytes
	//   READWRITE(scriptSig);  // CScript: varint length + bytes (0 for masternodes)
	//   READWRITE(nSequence);  // uint32_t: 4 bytes (0xffffffff for masternodes)
	// Buffer: prevout (36) + scriptSig length (1) + nSequence (4) + sigTime (8) = 49 bytes
	buf := make([]byte, 0, 36+1+4+8)

	// Append prevout (COutPoint: hash + n)
	buf = append(buf, mnp.OutPoint.Hash[:]...)
	indexBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(indexBytes, mnp.OutPoint.Index)
	buf = append(buf, indexBytes...)

	// Append empty scriptSig (varint 0x00 for empty script)
	buf = append(buf, 0x00)

	// Append nSequence (0xffffffff for masternodes - default max value)
	seqBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(seqBytes, 0xffffffff)
	buf = append(buf, seqBytes...)

	// Append sigTime (NO blockHash - that's only for signature verification)
	sigTimeBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(sigTimeBytes, uint64(mnp.SigTime))
	buf = append(buf, sigTimeBytes...)

	// Double SHA256 hash (Bitcoin/TWINS standard)
	hash1 := sha256.Sum256(buf)
	hash2 := sha256.Sum256(hash1[:])

	return hash2
}

// MasternodeInfo is the JSON representation of masternode info for RPC
type MasternodeInfo struct {
	OutPoint        string  `json:"outpoint"`
	Status          string  `json:"status"`
	Tier            string  `json:"tier"`
	Collateral      int64   `json:"collateral"`
	Protocol        int32   `json:"protocol"`
	Addr            string  `json:"addr"`
	Payee           string  `json:"payee"`
	ActiveSince     int64   `json:"activesince"`
	LastPing        int64   `json:"lastping"`
	LastPaid        int64   `json:"lastpaid"`
	LastSeen        int64   `json:"lastseen"`
	Rank            int     `json:"rank"`
	PaymentCount    int64   `json:"paymentcount"`
	Score           float64 `json:"score"`
	SentinelVersion string  `json:"sentinelversion"`
	SentinelPing    int64   `json:"sentinelping"`
}

// PaymentInfo represents payment information for a masternode
type PaymentInfo struct {
	OutPoint      types.Outpoint `json:"outpoint"`
	Tier          MasternodeTier `json:"tier"`
	Address       string         `json:"address"`
	LastPaid      time.Time      `json:"lastpaid"`
	NextPayment   time.Time      `json:"nextpayment"`
	PaymentHeight int32          `json:"paymentheight"`
}

// PaymentQueue manages the masternode payment queue
type PaymentQueue struct {
	queue      []*Masternode
	lastPaid   map[types.Outpoint]time.Time
	paymentPos int
	mu         sync.RWMutex
}

// MasternodeList represents a list of masternodes with rankings
type MasternodeList struct {
	Masternodes []*Masternode
	Synced      bool
	UpdateTime  time.Time
}

// Protocol version constants - MUST match legacy C++ values from version.h
// Used for SPORK_10 enforcement logic
const (
	// MIN_PEER_PROTO_VERSION_BEFORE_ENFORCEMENT - allow old peers when SPORK_10 is OFF
	MinPeerProtoBeforeEnforcement int32 = 70926

	// MIN_PEER_PROTO_VERSION_AFTER_ENFORCEMENT - require updated peers when SPORK_10 is ON
	MinPeerProtoAfterEnforcement int32 = 70927

	// ActiveProtocol - current expected protocol version (can be upgraded via spork)
	ActiveProtocolVersion int32 = 70928
)

// Masternode timing constants — derived from legacy C++ values in masternode.h
// CRITICAL: Most values are consensus-critical. MinPingSeconds intentionally
// reduced from legacy 600 to 300 to match PingInterval.
const (
	// MASTERNODE_MIN_MNP_SECONDS - minimum seconds between pings
	// Reduced from 600 (10 min) to 300 (5 min) to match PingInterval
	MinPingSeconds int64 = 300 // 5 minutes

	// MASTERNODE_MIN_MNB_SECONDS (5 * 60) - minimum seconds between broadcasts
	MinBroadcastSeconds int64 = 300 // 5 minutes (legacy: 5 * 60)

	// MASTERNODE_PING_SECONDS (5 * 60) - expected ping interval
	PingSeconds int64 = 300 // 5 minutes (legacy: 5 * 60)

	// MASTERNODE_EXPIRATION_SECONDS (120 * 60) - time before masternode expires
	ExpirationSeconds int64 = 7200 // 2 hours (legacy: 120 * 60)

	// MASTERNODE_REMOVAL_SECONDS (130 * 60) - time before masternode removal
	RemovalSeconds int64 = 7800 // 130 minutes (legacy: 130 * 60)

	// MASTERNODE_CHECK_SECONDS - interval for status checks
	CheckSeconds int64 = 5 // 5 seconds (legacy: 5)

	// MN_WINNER_MINIMUM_AGE - minimum age for payment eligibility (blocks)
	WinnerMinimumAge int64 = 8000

	// MonthSeconds - 30 days in seconds for SecondsSincePayment tiebreaker
	// Legacy: used in SecondsSincePayment() to cap return value before adding hash-based tiebreaker
	MonthSeconds int64 = 30 * 24 * 60 * 60 // 2,592,000 seconds
)

// Network port constants - MUST match legacy TWINS chainparams.cpp values
// Used for masternode address validation (CheckDefaultPort)
const (
	// MainnetDefaultPort is the default P2P port for mainnet
	// Legacy: Params().GetDefaultPort() for mainnet
	MainnetDefaultPort = 37817

	// TestnetDefaultPort is the default P2P port for testnet
	// Legacy: Params().GetDefaultPort() for testnet
	TestnetDefaultPort = 37847

	// RegtestDefaultPort is the default P2P port for regtest
	// Legacy: Params().GetDefaultPort() for regtest
	RegtestDefaultPort = 51478
)

// NetworkType represents the network type (mainnet, testnet, regtest)
type NetworkType int

const (
	NetworkMainnet NetworkType = iota
	NetworkTestnet
	NetworkRegtest
)

// GetDefaultPort returns the default P2P port for the network type
func (n NetworkType) GetDefaultPort() int {
	switch n {
	case NetworkMainnet:
		return MainnetDefaultPort
	case NetworkTestnet:
		return TestnetDefaultPort
	case NetworkRegtest:
		return RegtestDefaultPort
	default:
		return MainnetDefaultPort
	}
}

// String returns the network name
func (n NetworkType) String() string {
	switch n {
	case NetworkMainnet:
		return "mainnet"
	case NetworkTestnet:
		return "testnet"
	case NetworkRegtest:
		return "regtest"
	default:
		return "unknown"
	}
}

// GetNetworkID returns the address network ID byte for this network type
// Used for generating P2PKH scripts from public keys
func (n NetworkType) GetNetworkID() byte {
	switch n {
	case NetworkMainnet:
		return crypto.MainNetPubKeyHashAddrID
	case NetworkTestnet, NetworkRegtest:
		return crypto.TestNetPubKeyHashAddrID
	default:
		return crypto.MainNetPubKeyHashAddrID
	}
}

// Config contains masternode manager configuration
type Config struct {
	RequiredConfirmations int           // Confirmations needed for collateral
	UpdateInterval        time.Duration // Status update interval
	ExpireTime            time.Duration // Time before marking expired
	PingTimeout           time.Duration // Ping timeout
	SentinelPingInterval  time.Duration // Sentinel ping interval
	MinProtocolVersion    int32         // Minimum protocol version
	MaxMasternodes        int           // Maximum number of masternodes
	PaymentInterval       int32         // Blocks between payments
	NetworkType           NetworkType   // Network type (mainnet/testnet/regtest) for port validation
}

// DefaultConfig returns default masternode configuration
// Uses legacy-compatible timing constants
func DefaultConfig() *Config {
	return &Config{
		RequiredConfirmations: MinConfirmations,
		UpdateInterval:        time.Duration(CheckSeconds) * time.Second,
		ExpireTime:            time.Duration(ExpirationSeconds) * time.Second,
		PingTimeout:           time.Duration(PingSeconds) * time.Second,
		SentinelPingInterval:  1 * time.Hour,
		MinProtocolVersion:    70000,
		MaxMasternodes:        5000,
		PaymentInterval:       10,              // Every 10 blocks
		NetworkType:           NetworkMainnet,  // Default to mainnet
	}
}

// IsActive returns whether the masternode is active
func (mn *Masternode) IsActive() bool {
	mn.mu.RLock()
	defer mn.mu.RUnlock()
	return mn.Status == StatusEnabled || mn.Status == StatusPreEnabled
}

// IsPosebanActive returns whether the masternode is pose-banned
func (mn *Masternode) IsPosebanActive() bool {
	mn.mu.RLock()
	defer mn.mu.RUnlock()
	return mn.Status == StatusPoseban
}

// IsExpired returns whether the masternode has expired
func (mn *Masternode) IsExpired() bool {
	mn.mu.RLock()
	defer mn.mu.RUnlock()
	return mn.Status == StatusExpired
}

// IsBroadcastedWithin checks if the masternode was broadcast within the given seconds
// Matches legacy CMasternode::IsBroadcastedWithin() from masternode.h:252-255
// Legacy: return (GetAdjustedTime() - sigTime) < seconds;
// LEGACY COMPATIBILITY: Uses GetAdjustedTime() for network-synchronized time
func (mn *Masternode) IsBroadcastedWithin(seconds int64) bool {
	mn.mu.RLock()
	defer mn.mu.RUnlock()
	// LEGACY COMPATIBILITY: Use network-adjusted time like legacy C++ (masternode.h:253)
	now := consensus.GetAdjustedTimeUnix()
	return (now - mn.SigTime) < seconds
}

// IsPingedWithin checks if the masternode was pinged within the given seconds
// Matches legacy CMasternode::IsPingedWithin() from masternode.h:257-262
// Legacy: return (lastPing == CMasternodePing()) ? false : now - lastPing.sigTime < seconds;
// LEGACY COMPATIBILITY: Uses GetAdjustedTime() for network-synchronized time
func (mn *Masternode) IsPingedWithin(seconds int64) bool {
	mn.mu.RLock()
	defer mn.mu.RUnlock()
	// If no ping message, return false (matches legacy null ping check)
	if mn.LastPingMessage == nil {
		return false
	}
	// LEGACY COMPATIBILITY: Use network-adjusted time like legacy C++ (masternode.h:260)
	now := consensus.GetAdjustedTimeUnix()
	return (now - mn.LastPingMessage.SigTime) < seconds
}

// IsValidNetAddr checks if the masternode address is a valid network address
// DEPRECATED: Use IsValidNetAddrForNetwork(network) instead - this method hardcodes NetworkMainnet
// Matches legacy CMasternode::IsValidNetAddr() from masternode.cpp
// Legacy: return Params().NetworkID() == CBaseChainParams::REGTEST || (IsReachable(addr) && addr.IsRoutable())
// LEGACY COMPATIBILITY: Legacy C++ uses Params().NetworkID() to get current network at runtime.
// Go callers should use IsValidNetAddrForNetwork() and pass the network from Manager config.
func (mn *Masternode) IsValidNetAddr() bool {
	mn.mu.RLock()
	defer mn.mu.RUnlock()
	return IsValidNetAddrWithNetwork(mn.Addr, NetworkMainnet)
}

// IsValidNetAddrForNetwork checks if the masternode address is valid for the given network
// For regtest, all addresses are valid (matches legacy behavior)
// For other networks, checks IsReachable() && IsRoutable()
func (mn *Masternode) IsValidNetAddrForNetwork(network NetworkType) bool {
	mn.mu.RLock()
	defer mn.mu.RUnlock()
	return IsValidNetAddrWithNetwork(mn.Addr, network)
}

// CheckDefaultPort validates that the masternode uses the correct port for the network
// Matches legacy CMasternodeBroadcast::CheckDefaultPort()
func (mn *Masternode) CheckDefaultPort(network NetworkType) (bool, string) {
	mn.mu.RLock()
	defer mn.mu.RUnlock()
	return CheckDefaultPort(mn.Addr, network)
}

// GetPayee returns the payment address for this masternode as a base58 string
// DEPRECATED: Use GetPayeeScript() for consensus validation
// Uses PubKeyCollateral (not PubKey) to match legacy behavior
func (mn *Masternode) GetPayee() string {
	mn.mu.RLock()
	defer mn.mu.RUnlock()

	// Use collateral key (matches GetPayeeScript behavior)
	if mn.PubKeyCollateral == nil {
		return ""
	}

	// Create address from collateral public key using TWINS mainnet prefix
	address := crypto.NewAddressFromPubKey(mn.PubKeyCollateral, crypto.TWINSMainNetPubKeyHashAddrID)
	return address.String()
}

// GetPayeeScript returns the payment script (scriptPubKey) for this masternode
// Returns P2PKH script matching legacy format for consensus validation
// Uses PubKeyCollateral (not PubKey) to match legacy behavior
func (mn *Masternode) GetPayeeScript() []byte {
	mn.mu.RLock()
	defer mn.mu.RUnlock()

	// Use collateral key (the key that locks the collateral funds)
	// Legacy nodes send payments to pubKeyCollateralAddress, not operator key
	if mn.PubKeyCollateral == nil {
		return nil
	}

	// Create P2PKH script from public key hash
	// Script format: OP_DUP OP_HASH160 <20-byte-pubkey-hash> OP_EQUALVERIFY OP_CHECKSIG
	// This matches Bitcoin/TWINS standard P2PKH format
	pubKeyHash := crypto.Hash160(mn.PubKeyCollateral.SerializeCompressed())

	script := make([]byte, 25)
	script[0] = 0x76 // OP_DUP
	script[1] = 0xa9 // OP_HASH160
	script[2] = 0x14 // Push 20 bytes
	copy(script[3:23], pubKeyHash)
	script[23] = 0x88 // OP_EQUALVERIFY
	script[24] = 0xac // OP_CHECKSIG

	return script
}

// compareHashBytes compares two 32-byte slices as uint256 values (most significant byte first).
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
// CRITICAL: This compares from byte index 31 down to 0 to match C++ uint256::CompareTo.
// Both slices must be exactly 32 bytes (the hash length).
func compareHashBytes(a, b []byte) int {
	// Compare from most significant byte (index 31) to least significant (index 0)
	for i := 31; i >= 0; i-- {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

// addUint256AndUint32 performs arithmetic addition of a 32-byte hash and a uint32.
// This matches legacy C++ uint256::operator+=(uint64_t) from uint256.h:170-176
// The uint256 is stored in little-endian format (lowest bytes first).
// Legacy: uint256 aux = vin.prevout.hash + vin.prevout.n;
func addUint256AndUint32(hash types.Hash, n uint32) types.Hash {
	var result types.Hash
	copy(result[:], hash[:])

	// uint256 is stored as 8 uint32 words in little-endian order
	// We add n to the first word and propagate carry
	carry := uint64(n)
	for i := 0; i < 32 && carry > 0; i += 4 {
		// Read current 4-byte word as little-endian uint32
		val := uint64(binary.LittleEndian.Uint32(result[i : i+4]))
		sum := val + carry
		// Store low 32 bits back
		binary.LittleEndian.PutUint32(result[i:i+4], uint32(sum))
		// Carry is high bits
		carry = sum >> 32
	}
	return result
}

// CalculateScore calculates the masternode score for payment ordering
// This MUST match the legacy algorithm EXACTLY from masternode.cpp:200-228
// Legacy reference: CMasternode::CalculateScore()
//
// Legacy algorithm:
//  1. hash2 = Hash(blockHash) - FIXED, used for all comparisons
//  2. ss2 starts with blockHash
//  3. For each round: ss2 << aux; hash3 = ss2.GetHash(); diff = |hash3 - hash2|; r = max(diff, r)
//  4. Note: ss2 ACCUMULATES (same stream used across rounds)
//
// CRITICAL: Returns full types.Hash (32 bytes) for deterministic ordering
// Legacy C++ returns uint256 and compares with if (n > nHigh)
// Using float64 (only 8 bytes) caused ordering divergence on hash collisions
func (mn *Masternode) CalculateScore(blockHash types.Hash) types.Hash {
	mn.mu.RLock()
	defer mn.mu.RUnlock()

	// Get nRounds based on tier (SelectionWeight: 1, 5, 20, 100)
	nRounds := mn.Tier.SelectionWeight()

	// Calculate aux = vin.prevout.hash + vin.prevout.n
	// CRITICAL: This is ARITHMETIC addition (uint256 + uint32), NOT concatenation!
	// Legacy C++ (masternode.cpp:202): uint256 aux = vin.prevout.hash + vin.prevout.n;
	// Result is 32 bytes (uint256), not 36 bytes
	aux := addUint256AndUint32(mn.OutPoint.Hash, mn.OutPoint.Index)

	// Helper function for double SHA256 (Bitcoin Hash256)
	doubleHash := func(data []byte) []byte {
		first := sha256.Sum256(data)
		second := sha256.Sum256(first[:])
		return second[:]
	}

	// CRITICAL: hash2 is FIXED - Hash(blockHash), used for ALL comparisons
	// Legacy: CHashWriter ss(SER_GETHASH, PROTOCOL_VERSION); ss << hash; uint256 hash2 = ss.GetHash();
	hash2 := doubleHash(blockHash[:])

	// CRITICAL: ss2 ACCUMULATES data across rounds (same stream)
	// Legacy: CHashWriter ss2(SER_GETHASH, PROTOCOL_VERSION); ss2 << hash;
	// Then in loop: ss2 << aux; (appends to existing data)
	// Note: aux is now 32 bytes (uint256), not 36 bytes
	accumulatedData := make([]byte, 0, 32+nRounds*32)
	accumulatedData = append(accumulatedData, blockHash[:]...)

	var maxDiff types.Hash

	for i := 0; i < nRounds; i++ {
		// ACCUMULATE aux to the stream (not replace!)
		// Legacy: ss2 << aux;
		accumulatedData = append(accumulatedData, aux[:]...)

		// hash3 = ss2.GetHash() - hash of ALL accumulated data
		hash3 := doubleHash(accumulatedData)

		// Calculate absolute difference: |hash3 - hash2|
		// Need to handle both hash3 > hash2 and hash3 < hash2
		// CRITICAL: Use compareHashBytes which compares from most significant byte (like C++ uint256)
		var diff types.Hash
		if compareHashBytes(hash3, hash2) >= 0 {
			// hash3 >= hash2: diff = hash3 - hash2
			var borrow int
			for j := 31; j >= 0; j-- {
				result := int(hash3[j]) - int(hash2[j]) - borrow
				if result < 0 {
					result += 256
					borrow = 1
				} else {
					borrow = 0
				}
				diff[j] = byte(result)
			}
		} else {
			// hash3 < hash2: diff = hash2 - hash3
			var borrow int
			for j := 31; j >= 0; j-- {
				result := int(hash2[j]) - int(hash3[j]) - borrow
				if result < 0 {
					result += 256
					borrow = 1
				} else {
					borrow = 0
				}
				diff[j] = byte(result)
			}
		}

		// Keep maximum difference: r = max(hashdiff, r)
		// CRITICAL: Use CompareTo which compares from most significant byte (like C++ uint256)
		if diff.CompareTo(maxDiff) > 0 {
			maxDiff = diff
		}
	}

	// Return full 32-byte hash for deterministic ordering
	// Legacy C++ compares uint256 values directly with if (n > nHigh)
	return maxDiff
}

// CalculateScoreCompact returns a compact float64 score for JSON API compatibility
// Uses only first 8 bytes - for display/API only, NOT for consensus ordering!
func (mn *Masternode) CalculateScoreCompact(blockHash types.Hash) float64 {
	score := mn.CalculateScore(blockHash)
	return float64(binary.LittleEndian.Uint64(score[0:8]))
}

// UpdateStatus updates the masternode status based on current conditions
// Implements legacy CMasternode::Check() state machine from masternode.cpp:230-270
// States: PRE_ENABLED -> ENABLED -> EXPIRED -> REMOVE
// CRITICAL: Check order MUST match legacy: REMOVAL -> EXPIRATION -> PRE_ENABLED
// CRITICAL: Must use lastPing.sigTime (signed message time), NOT wall-clock receive time
func (mn *Masternode) UpdateStatus(currentTime time.Time, expireTime time.Duration) {
	mn.mu.Lock()
	defer mn.mu.Unlock()

	// Skip if already in terminal state (VIN_SPENT only)
	// CRITICAL: StatusRemoved is NOT terminal - matches C++ Check() (masternode.cpp:239)
	// C++ only skips VIN_SPENT, allowing REMOVED masternodes to recover when fresh
	// pings arrive via broadcasts. Without this, REMOVED MNs can never transition back.
	if mn.Status == StatusOutpointSpent {
		return
	}

	currentUnix := currentTime.Unix()

	// Get last ping sigTime - CRITICAL: use signed message time, not wall-clock
	// Legacy uses lastPing.sigTime for all expiry checks (masternode.cpp:230-270)
	var lastPingSigTime int64
	if mn.LastPingMessage != nil {
		lastPingSigTime = mn.LastPingMessage.SigTime
	} else {
		// Fallback to SigTime from broadcast if no ping yet
		// Legacy: new masternodes start with lastPing from broadcast
		lastPingSigTime = mn.SigTime
	}

	// 1. Check for REMOVAL first (not pinged within REMOVAL_SECONDS)
	// Legacy: lastPing.sigTime + MASTERNODE_REMOVAL_SECONDS < GetAdjustedTime()
	// CRITICAL: Legacy checks this FIRST - oldest condition takes priority
	if lastPingSigTime > 0 && lastPingSigTime+RemovalSeconds < currentUnix {
		mn.Status = StatusRemoved
		return
	}

	// 2. Check for EXPIRED (not pinged within EXPIRATION_SECONDS)
	// Legacy: lastPing.sigTime + MASTERNODE_EXPIRATION_SECONDS < GetAdjustedTime()
	if lastPingSigTime > 0 && lastPingSigTime+ExpirationSeconds < currentUnix {
		mn.Status = StatusExpired
		return
	}

	// 3. Check for PRE_ENABLED state
	// Legacy: if(lastPing.sigTime - sigTime < MASTERNODE_MIN_MNP_SECONDS) { activeState = MASTERNODE_PRE_ENABLED; }
	// PRE_ENABLED until lastPing is at least MIN_MNP_SECONDS after broadcast sigTime
	// CRITICAL: This checks the GAP between broadcast and first ping, not time since broadcast
	if mn.LastPingMessage != nil {
		if mn.LastPingMessage.SigTime-mn.SigTime < MinPingSeconds {
			mn.Status = StatusPreEnabled
			return
		}
	} else {
		// No ping yet - PRE_ENABLED until MIN_PING_SECONDS after broadcast
		if mn.SigTime+MinPingSeconds > currentUnix {
			mn.Status = StatusPreEnabled
			return
		}
	}

	// 4. If we pass all checks, masternode is ENABLED
	// Legacy: activeState = MASTERNODE_ENABLED; (masternode.cpp:269)
	// NOTE: C++ CMasternode::Check() does NOT have a ping-to-now freshness check.
	// It only checks the gap between broadcast sigTime and first ping sigTime (step 3 above).
	mn.Status = StatusEnabled
}

// UpdateStatusWithUTXO updates masternode status with UTXO spent check
// Implements full legacy CMasternode::Check() including collateral validation
// Legacy order (masternode.cpp:230-270):
//   0. Skip if VIN_SPENT (terminal state)
//   1. Check REMOVAL (not pinged > 7800s)
//   2. Check EXPIRED (not pinged > 7200s)
//   3. Check PRE_ENABLED (lastPing.sigTime - sigTime < MIN_SECONDS)
//   4. Check UTXO spent AND collateral validity (coins validation + isMasternodeCollateral)
//   5. Set ENABLED
// CRITICAL: Must match legacy order exactly for P2P compatibility
// CRITICAL: Must use lastPing.sigTime (signed message time), NOT wall-clock receive time
// CRITICAL: multiTierEnabled controls collateral validation (legacy: SPORK_TWINS_01_ENABLE_MASTERNODE_TIERS)
//   - When false: Only TierBronzeCollateral (1M) is valid
//   - When true: All 4 tiers are valid (1M/5M/20M/100M)
func (mn *Masternode) UpdateStatusWithUTXO(currentTime time.Time, expireTime time.Duration, utxoChecker UTXOChecker, multiTierEnabled bool) {
	mn.mu.Lock()
	defer mn.mu.Unlock()

	prevStatus := mn.Status

	// 0. Skip if already in terminal state (VIN_SPENT)
	// Legacy: if (activeState == MASTERNODE_VIN_SPENT) return; (line 239)
	if mn.Status == StatusVinSpent || mn.Status == StatusOutpointSpent {
		return
	}

	currentUnix := currentTime.Unix()

	// Get last ping sigTime - CRITICAL: use signed message time, not wall-clock
	// Legacy uses lastPing.sigTime for all expiry checks (masternode.cpp:230-270)
	var lastPingSigTime int64
	if mn.LastPingMessage != nil {
		lastPingSigTime = mn.LastPingMessage.SigTime
	} else {
		// Fallback to SigTime from broadcast if no ping yet
		// Legacy: new masternodes start with lastPing from broadcast
		lastPingSigTime = mn.SigTime
	}

	// 1. Check for REMOVAL first (not pinged within REMOVAL_SECONDS)
	// Legacy: if (!IsPingedWithin(MASTERNODE_REMOVAL_SECONDS)) { activeState = MASTERNODE_REMOVE; } (line 242)
	if lastPingSigTime > 0 && lastPingSigTime+RemovalSeconds < currentUnix {
		mn.Status = StatusRemoved
		if mn.Status != prevStatus {
			logrus.WithFields(logrus.Fields{
				"outpoint":        mn.OutPoint.String(),
				"last_ping_age_s": currentUnix - lastPingSigTime,
				"removal_s":       RemovalSeconds,
			}).Debug("UpdateStatusWithUTXO: marking REMOVED - not pinged within removal window")
		}
		return
	}

	// 2. Check for EXPIRED (not pinged within EXPIRATION_SECONDS)
	// Legacy: if (!IsPingedWithin(MASTERNODE_EXPIRATION_SECONDS)) { activeState = MASTERNODE_EXPIRED; } (line 247)
	if lastPingSigTime > 0 && lastPingSigTime+ExpirationSeconds < currentUnix {
		mn.Status = StatusExpired
		if mn.Status != prevStatus {
			logrus.WithFields(logrus.Fields{
				"outpoint":        mn.OutPoint.String(),
				"last_ping_age_s": currentUnix - lastPingSigTime,
				"expiration_s":    ExpirationSeconds,
			}).Debug("UpdateStatusWithUTXO: marking EXPIRED - not pinged within expiration window")
		}
		return
	}

	// 3. Check for PRE_ENABLED state
	// Legacy: if(lastPing.sigTime - sigTime < MASTERNODE_MIN_MNP_SECONDS) { activeState = MASTERNODE_PRE_ENABLED; } (line 252)
	// Note: Legacy uses (lastPing.sigTime - sigTime), we use (sigTime + MIN_SECONDS > now) which is equivalent
	// for determining if enough time has passed since broadcast
	if mn.LastPingMessage != nil {
		if mn.LastPingMessage.SigTime-mn.SigTime < MinPingSeconds {
			mn.Status = StatusPreEnabled
			return
		}
	} else {
		// No ping yet - PRE_ENABLED until MIN_PING_SECONDS after broadcast
		if mn.SigTime+MinPingSeconds > currentUnix {
			mn.Status = StatusPreEnabled
			return
		}
	}

	// 4. Check UTXO spent status AND collateral validity (AFTER time-based checks, matching legacy order)
	// Legacy: coins check at line 257-266 includes isMasternodeCollateral() validation
	// Legacy: if (!coins || !coins->IsAvailable(vin.prevout.n) || !isMasternodeCollateral(coins->vout[vin.prevout.n].nValue))
	// CRITICAL FIX: Legacy treats lookup failure (!coins) the SAME as spent collateral
	if utxoChecker != nil {
		// 4a. Check if UTXO is spent
		// LEGACY COMPATIBILITY: Error during lookup = treat as spent (masternode.cpp:257-266)
		// Legacy: if (!coins || !coins->IsAvailable(...)) { activeState = MASTERNODE_VIN_SPENT; }
		spent, err := utxoChecker.IsUTXOSpent(mn.OutPoint)
		if err != nil {
			// Legacy treats lookup failure same as spent collateral
			mn.Status = StatusVinSpent
			logrus.WithFields(logrus.Fields{
				"outpoint": mn.OutPoint.String(),
				"error":    err.Error(),
			}).Warn("UpdateStatusWithUTXO: marking VIN_SPENT due to UTXO lookup error (may be transient DB issue)")
			return
		}
		if spent {
			mn.Status = StatusVinSpent
			if mn.Status != prevStatus {
				logrus.WithField("outpoint", mn.OutPoint.String()).
					Info("UpdateStatusWithUTXO: marking VIN_SPENT - collateral UTXO confirmed spent")
			}
			return
		}

		// 4b. LEGACY COMPATIBILITY: Validate collateral amount based on spork state
		// Legacy: isMasternodeCollateral() checks SPORK_TWINS_01_ENABLE_MASTERNODE_TIERS
		// - When spork OFF: Only base collateral (1M TWINS) is valid
		// - When spork ON: All 4 tier collaterals are valid (1M/5M/20M/100M)
		// LEGACY COMPATIBILITY: Error getting value = treat as spent (missing coins)
		utxoValue, err := utxoChecker.GetUTXOValue(mn.OutPoint)
		if err != nil {
			// Legacy treats missing coins as spent collateral
			mn.Status = StatusVinSpent
			logrus.WithFields(logrus.Fields{
				"outpoint": mn.OutPoint.String(),
				"error":    err.Error(),
			}).Warn("UpdateStatusWithUTXO: marking VIN_SPENT due to UTXO value lookup error (may be transient DB issue)")
			return
		}
		validCollateral := false
		if multiTierEnabled {
			// Multi-tier mode: all 4 collaterals valid
			validCollateral = utxoValue == TierBronzeCollateral ||
				utxoValue == TierSilverCollateral ||
				utxoValue == TierGoldCollateral ||
				utxoValue == TierPlatinumCollateral
		} else {
			// Single-tier mode: only Bronze (1M) valid
			validCollateral = utxoValue == TierBronzeCollateral
		}
		if !validCollateral {
			// Invalid collateral amount - treat as spent (matches legacy behavior)
			mn.Status = StatusVinSpent
			if mn.Status != prevStatus {
				logrus.WithFields(logrus.Fields{
					"outpoint":   mn.OutPoint.String(),
					"utxo_value": utxoValue,
					"multi_tier": multiTierEnabled,
				}).Warn("UpdateStatusWithUTXO: marking VIN_SPENT - invalid collateral amount")
			}
			return
		}
	}

	// 5. If we pass all checks, masternode is ENABLED
	// Legacy: activeState = MASTERNODE_ENABLED; (line 269)
	mn.Status = StatusEnabled
}

// SecondsSincePayment returns seconds since last payment with legacy tiebreaker
// Implements legacy CMasternode::SecondsSincePayment() from masternode.cpp:287-311
//
// Algorithm:
// 1. If prevCycleLastPaymentTime > 0: sec = GetAdjustedTime() - prevCycleLastPaymentTime
// 2. If sec < MonthSeconds (30 days): return sec
// 3. Otherwise: return MonthSeconds + Hash(vin + sigTime).GetCompact(false)
//
// The hash-based tiebreaker ensures deterministic ordering for masternodes
// that haven't been paid in over 30 days, preventing ordering disputes.
// LEGACY COMPATIBILITY: Uses GetAdjustedTime() like legacy C++ (masternode.cpp:292-294)
// LEGACY FIX: Mutates PrevCycleLastPaymentTime when invalid (C++ behavior)
// C++ Reference: masternode.cpp:292-300
func (mn *Masternode) SecondsSincePayment() int64 {
	// Need write lock for potential mutation of PrevCycleLastPaymentTime
	mn.mu.Lock()
	defer mn.mu.Unlock()

	// LEGACY COMPATIBILITY: Use GetAdjustedTime() like legacy C++ (masternode.cpp:292)
	currentTime := consensus.GetAdjustedTimeUnix()

	var sec int64

	// LEGACY FIX: Check BOTH > 0 AND < currentTime (time must be valid and in the past)
	// C++ Reference: masternode.cpp:292-300
	// if (prevCycleLastPaymentTime > 0 && prevCycleLastPaymentTime < GetAdjustedTime()) {
	//     sec = (GetAdjustedTime() - prevCycleLastPaymentTime);
	// } else {
	//     prevCycleLastPaymentTime = GetAdjustedTime();  // MUTATES!
	//     sec = 0;  // effectively
	// }
	if mn.PrevCycleLastPaymentTime > 0 && mn.PrevCycleLastPaymentTime < currentTime {
		sec = currentTime - mn.PrevCycleLastPaymentTime
	} else {
		// LEGACY COMPATIBILITY: C++ mutates prevCycleLastPaymentTime to current time
		// and returns sec = 0 for ALL other cases (zero, future, or exactly current).
		// C++ Reference: masternode.cpp:296-299
		//   prevCycleLastPaymentTime = GetAdjustedTime();
		//   sec = (GetAdjustedTime() - prevCycleLastPaymentTime); // effectively 0
		// This means new masternodes (PrevCycleLastPaymentTime == 0) appear as "just paid",
		// placing them at the back of the payment queue until cycleDataValid resets them.
		mn.PrevCycleLastPaymentTime = currentTime
		sec = 0
	}

	// If under 30 days, return actual seconds
	if sec < MonthSeconds {
		return sec
	}

	// Over 30 days: add hash-based tiebreaker
	// Legacy: return month + Hash(vin + sigTime).GetCompact(false)
	// GetCompact(false) returns the upper 32 bits as a deterministic value
	tiebreaker := mn.calculatePaymentTiebreaker()
	return MonthSeconds + tiebreaker
}

// calculatePaymentTiebreaker generates a deterministic tiebreaker value
// Matches legacy Hash(vin + sigTime).GetCompact(false)
//
// Legacy C++ serializes full CTxIn structure (masternode.cpp:304-310):
//   CHashWriter ss(SER_GETHASH, PROTOCOL_VERSION);
//   ss << vin;      // Full CTxIn: prevout + scriptSig + nSequence
//   ss << sigTime;
//
// CTxIn serialization (primitives/transaction.h:86-90):
//   - COutPoint prevout: hash (32 bytes) + n (4 bytes LE)
//   - CScript scriptSig: varint length + script bytes (empty for MN = 0x00)
//   - uint32_t nSequence: 4 bytes LE (0xFFFFFFFF for MN)
func (mn *Masternode) calculatePaymentTiebreaker() int64 {
	// Build CTxIn serialization: prevout (36) + scriptSig (1) + nSequence (4) = 41 bytes
	// Plus sigTime (8 bytes) = 49 bytes total
	buf := make([]byte, 0, 49)

	// 1. COutPoint prevout: hash (32 bytes) + n (4 bytes LE)
	buf = append(buf, mn.OutPoint.Hash[:]...)
	indexBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(indexBytes, mn.OutPoint.Index)
	buf = append(buf, indexBytes...)

	// 2. CScript scriptSig: varint length (0x00 for empty script)
	buf = append(buf, 0x00)

	// 3. uint32_t nSequence: 0xFFFFFFFF (default for masternode vin)
	buf = append(buf, 0xFF, 0xFF, 0xFF, 0xFF)

	// 4. sigTime (8 bytes LE) - signed int64 in legacy
	sigTimeBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(sigTimeBytes, uint64(mn.SigTime))
	buf = append(buf, sigTimeBytes...)

	// Double SHA256 (Bitcoin standard)
	hash1 := sha256.Sum256(buf)
	hash2 := sha256.Sum256(hash1[:])

	// Convert to types.Hash and use proper GetCompact() method
	// CRITICAL: Must match legacy uint256::GetCompact(false) exactly
	// GetCompact uses floating-point-like format: [1 byte exponent][3 bytes mantissa]
	var hash types.Hash
	copy(hash[:], hash2[:])
	return int64(hash.GetCompact())
}

// cycleDataValid checks if the cycle tracking data is still valid
// Legacy: CMasternode::cycleDataValid() from masternode.cpp:276-285
//
// Legacy C++ Implementation:
//
//	bool CMasternode::cycleDataValid() {
//	    CBlock block;
//	    CBlockIndex* pblockindex = mapBlockIndex[prevCycleLastPaymentHash];
//	    if (!ReadBlockFromDisk(block, pblockindex))
//	        return false;
//	    if (abs(block.GetBlockTime() - prevCycleLastPaymentTime) > 600)
//	        return false;
//	    return true;
//	}
//
// The legacy implementation looks up the block by prevCycleLastPaymentHash and
// compares the block's timestamp with prevCycleLastPaymentTime. This validates
// that the stored cycle data is internally consistent (the time matches the block).
//
// This is NOT a freshness check! It's a consistency validation.
// A cycle reset happens when the block lookup fails OR when there's a time mismatch.
//
// IMPORTANT: This method requires blockchain access to implement correctly.
// Use Manager.cycleDataValidWithBlockchain() for the proper legacy-compatible check.
// This fallback returns true if blockchain lookup is unavailable, assuming data is valid.
//
// This is called without holding the lock from the caller
func (mn *Masternode) cycleDataValid(currentTime int64) bool {
	// If no cycle data set, it's invalid - same as legacy failing ReadBlockFromDisk
	if mn.PrevCycleLastPaymentTime == 0 {
		return false
	}

	// If PrevCycleLastPaymentHash is zero, cycle was never properly initialized
	// Legacy would fail at mapBlockIndex lookup
	var zeroHash types.Hash
	if mn.PrevCycleLastPaymentHash == zeroHash {
		return false
	}

	// NOTE: Without blockchain access, we cannot implement the full legacy check.
	// The legacy check validates that block.GetBlockTime() at PrevCycleLastPaymentHash
	// matches PrevCycleLastPaymentTime within 600 seconds.
	//
	// This fallback assumes the data is valid if it exists. The proper check is
	// performed in Manager.cycleDataValidWithBlockchain() which has blockchain access.
	return true
}

// AddWin records a payment win and manages cycle tracking
// Implements legacy CMasternode::addWin() from masternode.cpp:181-190
//
// Algorithm:
//  1. Increment wins counter
//  2. If wins >= tier rounds (selection weight), reset cycle:
//     - wins = wins - tier
//     - prevCycleLastPaymentTime = GetAdjustedTime()
//     - prevCycleLastPaymentHash = currentBlockHash
//
// Note: This method locks the mutex internally - do NOT call while holding the lock
// LEGACY COMPATIBILITY: Uses GetAdjustedTime() for cycle reset timing (masternode.cpp:187)
func (mn *Masternode) AddWin(currentBlockHash types.Hash) {
	mn.mu.Lock()
	defer mn.mu.Unlock()

	// Get tier rounds (selection weight: Bronze=1, Silver=5, Gold=20, Platinum=100)
	tierRounds := mn.Tier.SelectionWeight()

	// Increment wins
	mn.WinsThisCycle++

	// Check if we've completed a cycle
	// Legacy: if (++wins >= tier) { wins = wins - tier; reset cycle data }
	if mn.WinsThisCycle >= tierRounds {
		mn.WinsThisCycle -= tierRounds
		// LEGACY COMPATIBILITY: Use GetAdjustedTime() like legacy C++ (masternode.cpp:187)
		mn.PrevCycleLastPaymentTime = consensus.GetAdjustedTimeUnix()
		mn.PrevCycleLastPaymentHash = currentBlockHash
	}

	// Also update LastPaid for RPC/display purposes using GetAdjustedTime()
	mn.LastPaid = consensus.GetAdjustedTimeAsTime()
	mn.PaymentCount++
}

// ResetCycleData initializes cycle tracking data when invalid
// Called when cycleDataValid() returns false
// Legacy: Sets prevCycleLastPaymentHash = chainActive.Tip()->GetBlockHash()
//
//	        prevCycleLastPaymentTime = GetAdjustedTime()
//	        wins = 0
//
// LEGACY COMPATIBILITY: Uses GetAdjustedTime() like legacy C++ (masternode.cpp:154,298)
func (mn *Masternode) ResetCycleData(currentBlockHash types.Hash) {
	mn.mu.Lock()
	defer mn.mu.Unlock()

	// LEGACY COMPATIBILITY: Use GetAdjustedTime() like legacy C++
	mn.PrevCycleLastPaymentTime = consensus.GetAdjustedTimeUnix()
	mn.PrevCycleLastPaymentHash = currentBlockHash
	mn.WinsThisCycle = 0
}

// CycleDataValidWithReset checks cycle validity and resets if invalid
// This is a convenience method combining cycleDataValid check with reset
// The caller must provide the current block hash for reset
// Note: This method locks - do NOT call while holding lock
// LEGACY COMPATIBILITY: Uses GetAdjustedTime() like legacy C++
func (mn *Masternode) CycleDataValidWithReset(currentBlockHash types.Hash) bool {
	mn.mu.Lock()
	defer mn.mu.Unlock()

	// LEGACY COMPATIBILITY: Use GetAdjustedTime() like legacy C++
	currentTime := consensus.GetAdjustedTimeUnix()

	// Check if cycle data is valid
	if mn.PrevCycleLastPaymentTime == 0 {
		// No cycle data - initialize
		mn.PrevCycleLastPaymentTime = currentTime
		mn.PrevCycleLastPaymentHash = currentBlockHash
		mn.WinsThisCycle = 0
		return false
	}

	// Check time difference
	timeDiff := currentTime - mn.PrevCycleLastPaymentTime
	if timeDiff < 0 {
		timeDiff = -timeDiff
	}

	if timeDiff > 600 {
		// Invalid - reset
		mn.PrevCycleLastPaymentTime = currentTime
		mn.PrevCycleLastPaymentHash = currentBlockHash
		mn.WinsThisCycle = 0
		return false
	}

	return true
}

// ToInfo converts a Masternode to MasternodeInfo for RPC
func (mn *Masternode) ToInfo() *MasternodeInfo {
	mn.mu.RLock()
	defer mn.mu.RUnlock()

	return &MasternodeInfo{
		OutPoint:        mn.OutPoint.String(),
		Status:          mn.Status.String(),
		Tier:            mn.Tier.String(),
		Collateral:      mn.Collateral,
		Protocol:        mn.Protocol,
		Addr:            mn.Addr.String(),
		Payee:           mn.GetPayee(),
		ActiveSince:     mn.ActiveSince.Unix(),
		LastPing:        mn.LastPing.Unix(),
		LastPaid:        mn.LastPaid.Unix(),
		LastSeen:        mn.LastSeen.Unix(),
		Rank:            mn.Rank,
		PaymentCount:    mn.PaymentCount,
		Score:           mn.ScoreCompact, // Use compact score for JSON API
		SentinelVersion: mn.SentinelVersion,
		SentinelPing:    mn.SentinelPing.Unix(),
	}
}

// ToBroadcast converts a Masternode to a MasternodeBroadcast for P2P transmission
func (mn *Masternode) ToBroadcast() *MasternodeBroadcast {
	mn.mu.RLock()
	defer mn.mu.RUnlock()

	return &MasternodeBroadcast{
		OutPoint:         mn.OutPoint,
		Addr:             mn.Addr,
		PubKeyCollateral: mn.PubKeyCollateral,
		PubKeyMasternode: mn.PubKey,
		Signature:        mn.Signature,
		SigTime:          mn.SigTime,
		Protocol:         mn.Protocol,
		LastPing:         mn.LastPingMessage,
		LastDsq:          0, // Not used in current implementation
	}
}

// Verify verifies the masternode broadcast signature
func (mnb *MasternodeBroadcast) Verify() error {
	if mnb.PubKeyCollateral == nil {
		return fmt.Errorf("masternode broadcast has no collateral public key")
	}

	if mnb.PubKeyMasternode == nil {
		return fmt.Errorf("masternode broadcast has no masternode public key")
	}

	if len(mnb.Signature) == 0 {
		return fmt.Errorf("masternode broadcast has no signature")
	}

	// Legacy uses compact signatures (65 bytes)
	if len(mnb.Signature) != 65 {
		return fmt.Errorf("masternode broadcast signature must be 65 bytes (compact), got %d", len(mnb.Signature))
	}

	// Create the message to verify (new format with hex-encoded IDs)
	message := mnb.getNewSignatureMessage()

	// Verify the signature using compact signature verification (matches C++ obfuScationSigner.VerifyMessage)
	valid, err := crypto.VerifyCompactSignature(mnb.PubKeyCollateral, message, mnb.Signature)

	// If new format fails (error or invalid), try old format for backward compatibility
	if err != nil || !valid {
		messageOld := mnb.getOldSignatureMessage()
		validOld, errOld := crypto.VerifyCompactSignature(mnb.PubKeyCollateral, messageOld, mnb.Signature)
		if errOld == nil && validOld {
			// Old format succeeded
			return nil
		}
		// Both formats failed
		if err != nil {
			return fmt.Errorf("masternode broadcast signature verification failed (new format error: %v)", err)
		}
		if errOld != nil {
			return fmt.Errorf("masternode broadcast signature verification failed (old format error: %v)", errOld)
		}
		return fmt.Errorf("masternode broadcast signature verification failed: signature invalid")
	}

	return nil
}

// BroadcastValidationResult contains the result of broadcast validation
type BroadcastValidationResult struct {
	Valid      bool   // Whether the broadcast is valid
	DoS        int    // Denial-of-service punishment score (0 = don't punish, 100 = ban)
	Error      string // Error message if not valid
	ShouldSkip bool   // If true, skip processing but don't punish (e.g., duplicate)
}

// CheckAndUpdate performs full broadcast validation matching legacy CMasternodeBroadcast::CheckAndUpdate()
// This validates ALL aspects of the broadcast before accepting it into the masternode list
// Parameters:
//   - currentTime: current Unix timestamp (GetAdjustedTime in legacy)
//   - minProtocol: minimum required protocol version (from GetMinMasternodePaymentsProto)
//   - network: network type for port validation (matches legacy Params().GetDefaultPort())
//   - existingMN: existing masternode with same outpoint, or nil if new
//
// Returns: BroadcastValidationResult with validity, DoS score, and error message
func (mnb *MasternodeBroadcast) CheckAndUpdate(currentTime int64, minProtocol int32, network NetworkType, existingMN *Masternode) BroadcastValidationResult {
	// 1. Check sigTime is not too far in the future (max 1 hour ahead)
	// Legacy: if (sigTime > GetAdjustedTime() + 60 * 60)
	if mnb.SigTime > currentTime+3600 {
		return BroadcastValidationResult{
			Valid: false,
			DoS:   1, // Low punishment - could be clock drift
			Error: fmt.Sprintf("signature rejected, too far into the future: sigTime=%d, now=%d", mnb.SigTime, currentTime),
		}
	}

	// 2. Check lastPing exists and is valid
	// Legacy: if(lastPing == CMasternodePing() || !lastPing.CheckAndUpdate(nDos, false, true)) return false
	// The legacy CheckAndUpdate for ping validates:
	//   - sigTime not in future (> now + 1 hour)
	//   - sigTime not too old (< now - 1 hour)
	if mnb.LastPing == nil {
		return BroadcastValidationResult{
			Valid: false,
			DoS:   0, // No punishment - might be incomplete broadcast
			Error: "broadcast has no lastPing",
		}
	}

	// 2a. Validate lastPing sigTime (matches CMasternodePing::CheckAndUpdate with fCheckSigTimeOnly=true)
	// Legacy: if (sigTime > GetAdjustedTime() + 60 * 60) - reject future pings
	if mnb.LastPing.SigTime > currentTime+3600 {
		return BroadcastValidationResult{
			Valid: false,
			DoS:   1, // Low punishment - could be clock drift
			Error: fmt.Sprintf("lastPing sigTime rejected, too far in the future: %d", mnb.LastPing.SigTime),
		}
	}

	// Legacy: if (sigTime <= GetAdjustedTime() - 60 * 60) - reject old pings
	if mnb.LastPing.SigTime <= currentTime-3600 {
		return BroadcastValidationResult{
			Valid: false,
			DoS:   1, // Low punishment - could be clock drift
			Error: fmt.Sprintf("lastPing sigTime rejected, too far in the past: %d (now=%d, threshold=%d, age=%d sec)",
				mnb.LastPing.SigTime, currentTime, currentTime-3600, currentTime-mnb.LastPing.SigTime),
		}
	}

	// 2b. Validate lastPing signature (matches CMasternodePing::CheckAndUpdate signature verification)
	// Legacy: !lastPing.CheckAndUpdate(nDos, false, true) includes signature verification
	// The ping must be signed by the masternode operator key (PubKeyMasternode)
	if mnb.PubKeyMasternode != nil {
		if err := mnb.LastPing.Verify(mnb.PubKeyMasternode); err != nil {
			return BroadcastValidationResult{
				Valid: false,
				DoS:   100, // High punishment - invalid signature is serious
				Error: fmt.Sprintf("lastPing signature verification failed: %v", err),
			}
		}
	}

	// 3. Check protocol version
	// Legacy: if (protocolVersion < masternodePayments.GetMinMasternodePaymentsProto())
	if mnb.Protocol < minProtocol {
		return BroadcastValidationResult{
			Valid: false,
			DoS:   0, // No punishment - just outdated node
			Error: fmt.Sprintf("ignoring outdated masternode protocol %d, min required %d", mnb.Protocol, minProtocol),
		}
	}

	// 4. Validate pubkey script sizes (must create 25-byte P2PKH scripts)
	// Legacy: pubkeyScript = GetScriptForDestination(pubKeyCollateralAddress.GetID()); if (pubkeyScript.size() != 25)
	if mnb.PubKeyCollateral != nil {
		pubKeyHash := crypto.Hash160(mnb.PubKeyCollateral.SerializeCompressed())
		// P2PKH script format: OP_DUP(1) + OP_HASH160(1) + PUSH_20(1) + hash(20) + OP_EQUALVERIFY(1) + OP_CHECKSIG(1) = 25 bytes
		scriptLen := 25
		if len(pubKeyHash) != 20 {
			return BroadcastValidationResult{
				Valid: false,
				DoS:   100, // High punishment - malformed pubkey
				Error: "collateral pubkey creates wrong script size",
			}
		}
		_ = scriptLen // Validate script would be 25 bytes (P2PKH format)
	}

	if mnb.PubKeyMasternode != nil {
		pubKeyHash := crypto.Hash160(mnb.PubKeyMasternode.SerializeCompressed())
		if len(pubKeyHash) != 20 {
			return BroadcastValidationResult{
				Valid: false,
				DoS:   100, // High punishment - malformed pubkey
				Error: "masternode pubkey creates wrong script size",
			}
		}
	}

	// 5. Port validation using CheckDefaultPort (matches legacy Params().GetDefaultPort())
	// Legacy: CMasternodeBroadcast::CheckDefaultPort validates port matches network
	valid, errMsg := CheckDefaultPort(mnb.Addr, network)
	if !valid {
		return BroadcastValidationResult{
			Valid: false,
			DoS:   0, // Don't punish - might be node on different network
			Error: errMsg,
		}
	}

	// 6. Verify broadcast signature
	// This tries both new and old signature formats (handled in Verify())
	if err := mnb.Verify(); err != nil {
		// DoS depends on protocol version - old masternodes might have buggy signatures
		dos := 100
		if mnb.Protocol < MinPeerProtoAfterEnforcement {
			dos = 0 // Don't ban old protocol versions
		}
		return BroadcastValidationResult{
			Valid: false,
			DoS:   dos,
			Error: fmt.Sprintf("bad signature: %v", err),
		}
	}

	// 7. Check against existing masternode (if exists)
	if existingMN != nil {
		existingMN.mu.RLock()
		existingSigTime := existingMN.SigTime
		existingStatus := existingMN.Status
		existingMN.mu.RUnlock()

		// Reject if existing has newer or equal sigTime
		// Legacy: if(pmn->sigTime >= sigTime)
		if existingSigTime >= mnb.SigTime {
			return BroadcastValidationResult{
				Valid:      false,
				DoS:        0, // Don't punish - could be duplicate relay
				Error:      fmt.Sprintf("bad sigTime %d, existing is at %d", mnb.SigTime, existingSigTime),
				ShouldSkip: true, // Skip but don't punish
			}
		}

		// Legacy: if (!pmn->IsEnabled()) return true — C++ skips update for non-enabled.
		// We diverge for PreEnabled: during fresh sync, masternodes start as PreEnabled
		// and a second dseg response with a newer broadcast arrives before UpdateStatus
		// promotes them. Without this, the newer broadcast is silently discarded.
		// For Expired/Removed/VinSpent, match C++ behavior and skip the update.
		if existingStatus != StatusEnabled && existingStatus != StatusPreEnabled {
			return BroadcastValidationResult{
				Valid:      true,
				DoS:        0,
				ShouldSkip: true, // Skip update for expired/removed nodes (match C++)
			}
		}
	}

	// All validation passed
	return BroadcastValidationResult{
		Valid: true,
		DoS:   0,
	}
}

// CheckInputsAndAdd performs collateral validation matching legacy CMasternodeBroadcast::CheckInputsAndAdd()
// This validates the UTXO and collateral requirements before adding a NEW masternode
// Note: This should only be called AFTER CheckAndUpdate() passes
// Parameters:
//   - utxoChecker: interface to check if UTXO exists and is unspent
//   - getInputAge: function to get confirmation count for the collateral tx
//   - getCollateralBlockTime: function to get block time when collateral got required confirmations
//   - currentTime: current Unix timestamp
//
// Returns: BroadcastValidationResult with validity, DoS score, and error message
func (mnb *MasternodeBroadcast) CheckInputsAndAdd(
	utxoChecker UTXOChecker,
	getInputAge func(types.Outpoint) (int, error),
	getCollateralBlockTime func(types.Outpoint, int) (int64, error),
	currentTime int64,
) BroadcastValidationResult {
	// 1. Check lastPing is valid
	// Legacy: if(lastPing == CMasternodePing() || !lastPing.CheckAndUpdate(nDoS, false, true)) return false
	if mnb.LastPing == nil {
		return BroadcastValidationResult{
			Valid: false,
			DoS:   0,
			Error: "broadcast has no lastPing",
		}
	}

	// 2. Check UTXO exists and is unspent
	// Legacy: if (!coins || !coins->IsAvailable(vin.prevout.n) || !isMasternodeCollateral(coins->vout[vin.prevout.n].nValue))
	if utxoChecker != nil {
		isSpent, err := utxoChecker.IsUTXOSpent(mnb.OutPoint)
		if err != nil {
			return BroadcastValidationResult{
				Valid: false,
				DoS:   0, // Don't punish - might be our issue
				Error: fmt.Sprintf("failed to check UTXO: %v", err),
			}
		}
		if isSpent {
			return BroadcastValidationResult{
				Valid: false,
				DoS:   100, // Punish - trying to register with spent collateral
				Error: "collateral UTXO is spent",
			}
		}
	}

	// 3. Check input age (minimum confirmations)
	// Legacy: if (GetInputAge(vin) < MASTERNODE_MIN_CONFIRMATIONS)
	if getInputAge != nil {
		confirmations, err := getInputAge(mnb.OutPoint)
		if err != nil {
			return BroadcastValidationResult{
				Valid:      false,
				DoS:        0,
				Error:      fmt.Sprintf("failed to get input age: %v", err),
				ShouldSkip: true, // Let it be checked again later
			}
		}
		if confirmations < MinConfirmations {
			return BroadcastValidationResult{
				Valid:      false,
				DoS:        0, // Don't punish - might just be too new
				Error:      fmt.Sprintf("input must have at least %d confirmations, has %d", MinConfirmations, confirmations),
				ShouldSkip: true, // Let it be checked again later
			}
		}
	}

	// 4. Verify sigTime is not earlier than when collateral got required confirmations
	// Legacy: if (pConfIndex->GetBlockTime() > sigTime) return false
	if getCollateralBlockTime != nil {
		confBlockTime, err := getCollateralBlockTime(mnb.OutPoint, MinConfirmations)
		if err == nil { // Only validate if we can get the block time
			if confBlockTime > mnb.SigTime {
				return BroadcastValidationResult{
					Valid: false,
					DoS:   0, // Don't punish - could be clock issue
					Error: fmt.Sprintf("bad sigTime %d, confirmation block is at %d", mnb.SigTime, confBlockTime),
				}
			}
		}
	}

	// All validation passed
	return BroadcastValidationResult{
		Valid: true,
		DoS:   0,
	}
}

// IsValidNetAddr checks if the broadcast address is a valid network address
// DEPRECATED: Use IsValidNetAddrForNetwork(network) instead - this method hardcodes NetworkMainnet
// Matches legacy CMasternode::IsValidNetAddr() from masternode.cpp
// Legacy: return Params().NetworkID() == CBaseChainParams::REGTEST || (IsReachable(addr) && addr.IsRoutable())
// LEGACY COMPATIBILITY: Legacy C++ uses Params().NetworkID() to get current network at runtime.
// Go callers should use IsValidNetAddrForNetwork() and pass the network from Manager config.
func (mnb *MasternodeBroadcast) IsValidNetAddr() bool {
	return IsValidNetAddrWithNetwork(mnb.Addr, NetworkMainnet)
}

// IsValidNetAddrForNetwork checks if the broadcast address is valid for the given network
// For regtest, all addresses are valid (matches legacy behavior)
// For other networks, checks IsReachable() && IsRoutable()
func (mnb *MasternodeBroadcast) IsValidNetAddrForNetwork(network NetworkType) bool {
	return IsValidNetAddrWithNetwork(mnb.Addr, network)
}

// OnionCat prefix for encoding Tor v2 addresses as IPv6
// Tor addresses are mapped to IPv6 range fd87:d87e:eb43::/48 (RFC 4193 unique local)
// Legacy: static const unsigned char pchOnionCat[] = {0xFD, 0x87, 0xD8, 0x7E, 0xEB, 0x43};
var OnionCatPrefix = []byte{0xFD, 0x87, 0xD8, 0x7E, 0xEB, 0x43}

// IsTorAddress checks if the given IP is a Tor OnionCat-encoded address
// Tor addresses use IPv6 range fd87:d87e:eb43::/48
// Legacy: return (memcmp(ip, pchOnionCat, sizeof(pchOnionCat)) == 0);
func IsTorAddress(ip net.IP) bool {
	if ip == nil {
		return false
	}
	ip6 := ip.To16()
	if ip6 == nil {
		return false
	}
	// Check if first 6 bytes match OnionCat prefix
	for i := 0; i < len(OnionCatPrefix); i++ {
		if ip6[i] != OnionCatPrefix[i] {
			return false
		}
	}
	return true
}

// IsTorHostname checks if the hostname is a Tor .onion address
func IsTorHostname(host string) bool {
	return len(host) > 6 && strings.HasSuffix(strings.ToLower(host), ".onion")
}

// EncodeOnionCat converts a .onion hostname to OnionCat IPv6 address
// Input: "abc123xyz.onion" (16 chars base32 + .onion)
// Output: net.IP with fd87:d87e:eb43::<10 bytes from base32>
// Legacy: memcpy(ip, pchOnionCat, sizeof(pchOnionCat)); for (i=0; i<10; i++) ip[i+6] = vchAddr[i];
func EncodeOnionCat(onionHost string) net.IP {
	if !IsTorHostname(onionHost) {
		return nil
	}
	// Remove .onion suffix
	name := strings.TrimSuffix(strings.ToLower(onionHost), ".onion")
	// Tor v2 addresses are 16 base32 characters = 10 bytes
	if len(name) != 16 {
		return nil
	}
	// Decode base32
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(name))
	if err != nil || len(decoded) != 10 {
		return nil
	}
	// Build IPv6 address: OnionCat prefix (6 bytes) + decoded (10 bytes)
	ip := make(net.IP, 16)
	copy(ip[:6], OnionCatPrefix)
	copy(ip[6:], decoded)
	return ip
}

// DecodeOnionCat converts an OnionCat IPv6 address back to .onion hostname
// Input: net.IP with fd87:d87e:eb43:: prefix
// Output: "abc123xyz.onion"
// Legacy: return EncodeBase32(&ip[6], 10) + ".onion";
func DecodeOnionCat(ip net.IP) string {
	if !IsTorAddress(ip) {
		return ""
	}
	ip6 := ip.To16()
	// Encode last 10 bytes as base32
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(ip6[6:])
	return strings.ToLower(encoded) + ".onion"
}

// IsValidNetAddrWithNetwork is a standalone function to check if an address is valid for a network
// Matches legacy CMasternode::IsValidNetAddr() from masternode.cpp
// Legacy: return Params().NetworkID() == CBaseChainParams::REGTEST || (IsReachable(addr) && addr.IsRoutable())
func IsValidNetAddrWithNetwork(addr net.Addr, network NetworkType) bool {
	// Regtest allows any address (for testing purposes)
	// Legacy: if (Params().NetworkID() == CBaseChainParams::REGTEST) return true
	if network == NetworkRegtest {
		return true
	}

	if addr == nil {
		return false
	}

	var ip net.IP
	if tcpAddr, ok := addr.(*net.TCPAddr); ok {
		ip = tcpAddr.IP
	} else {
		// Try to parse from string
		host, _, err := net.SplitHostPort(addr.String())
		if err != nil {
			return false
		}

		// Check for Tor .onion addresses BEFORE trying net.ParseIP
		// .onion hostnames are not valid IPs, but we can encode them to OnionCat IPv6
		// Legacy: CNetAddr::SetSpecial() handles .onion addresses
		if IsTorHostname(host) {
			ip = EncodeOnionCat(host)
			if ip == nil {
				// Invalid .onion format (wrong length, bad base32, etc.)
				return false
			}
		} else {
			ip = net.ParseIP(host)
		}
	}

	if ip == nil {
		return false
	}

	// IsReachable: Address must be reachable (not internal/reserved)
	// IsRoutable: Address must be globally routable
	// Legacy checks: IsIPv4(), IsTor(), IsI2P(), IsCJDNS(), IsReachable(), IsRoutable()
	// Now supports IPv4, IPv6, and Tor (.onion) addresses
	return isRoutable(ip)
}

// isRoutable checks if an IP address is globally routable
// Matches legacy CNetAddr::IsRoutable() which checks !IsLocal() && !IsInternal() && !IsValid()
func isRoutable(ip net.IP) bool {
	if ip == nil {
		return false
	}

	// Not routable if unspecified (0.0.0.0 or ::)
	if ip.IsUnspecified() {
		return false
	}

	// Not routable if loopback (127.0.0.0/8 or ::1)
	if ip.IsLoopback() {
		return false
	}

	// Not routable if private (RFC 1918)
	// 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
	if ip.IsPrivate() {
		return false
	}

	// Not routable if link-local (169.254.0.0/16 or fe80::/10)
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}

	// Not routable if multicast
	if ip.IsMulticast() {
		return false
	}

	// IPv4 specific checks
	if ip4 := ip.To4(); ip4 != nil {
		// 0.0.0.0/8 - Current network (RFC 5735)
		if ip4[0] == 0 {
			return false
		}

		// 100.64.0.0/10 - Shared address space (RFC 6598) - Carrier-grade NAT
		if ip4[0] == 100 && (ip4[1]&0xC0) == 64 {
			return false
		}

		// 192.0.0.0/24 - IETF Protocol Assignments (RFC 6890)
		if ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 0 {
			return false
		}

		// 192.0.2.0/24 - TEST-NET-1 (RFC 5737)
		if ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 2 {
			return false
		}

		// 198.18.0.0/15 - Benchmarking (RFC 2544)
		if ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19) {
			return false
		}

		// 198.51.100.0/24 - TEST-NET-2 (RFC 5737)
		if ip4[0] == 198 && ip4[1] == 51 && ip4[2] == 100 {
			return false
		}

		// 203.0.113.0/24 - TEST-NET-3 (RFC 5737)
		if ip4[0] == 203 && ip4[1] == 0 && ip4[2] == 113 {
			return false
		}

		// 224.0.0.0/4 - Multicast (already checked above, but be explicit)
		if ip4[0] >= 224 && ip4[0] <= 239 {
			return false
		}

		// 240.0.0.0/4 - Reserved for future use (RFC 1112)
		if ip4[0] >= 240 {
			return false
		}
	} else {
		// IPv6 specific checks (ip.To4() returned nil, so it's IPv6)
		ip6 := ip.To16()
		if ip6 != nil {
			// Check for Tor OnionCat addresses FIRST - these ARE routable
			// Tor uses IPv6 range fd87:d87e:eb43::/48 which falls under RFC 4193
			// Legacy: (IsRFC4193() && !IsTor()) - Tor addresses are allowed
			if IsTorAddress(ip6) {
				return true
			}

			// RFC 4193 - IPv6 unique local addresses (fc00::/7)
			// Legacy: IsRFC4193() - private IPv6 addresses (except Tor)
			if (ip6[0] & 0xFE) == 0xFC {
				return false
			}

			// RFC 4843 - IPv6 ORCHID (2001:10::/28)
			// Legacy: IsRFC4843() - Overlay Routable Cryptographic Hash Identifiers
			if ip6[0] == 0x20 && ip6[1] == 0x01 && ip6[2] == 0x00 && (ip6[3]&0xF0) == 0x10 {
				return false
			}
		}
	}

	// Address passed all checks - it's routable
	return true
}

// CheckDefaultPort validates that the address uses the correct port for the network
// Matches legacy CMasternodeBroadcast::CheckDefaultPort() from masternode.cpp
// Legacy: if (service.GetPort() != nDefaultPort) { ... return false; } return true;
// Returns: valid (bool), error message (string)
func CheckDefaultPort(addr net.Addr, network NetworkType) (bool, string) {
	if addr == nil {
		return false, "address is nil"
	}

	var port int
	if tcpAddr, ok := addr.(*net.TCPAddr); ok {
		port = tcpAddr.Port
	} else {
		// Try to parse from string
		_, portStr, err := net.SplitHostPort(addr.String())
		if err != nil {
			return false, fmt.Sprintf("failed to parse port from address: %v", err)
		}
		if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
			return false, fmt.Sprintf("failed to parse port number: %v", err)
		}
	}

	expectedPort := network.GetDefaultPort()

	if port != expectedPort {
		// Legacy format: "Invalid port %u for masternode %s, only %d is supported on %s-net."
		return false, fmt.Sprintf("Invalid port %d for masternode %s, only %d is supported on %s-net",
			port, addr.String(), expectedPort, network.String())
	}

	return true, ""
}

// CheckDefaultPortResult contains the result of CheckDefaultPort validation
type CheckDefaultPortResult struct {
	Valid   bool
	Port    int
	Error   string
	Context string // Additional context for error messages
}

// CheckDefaultPortWithContext validates port with additional error context
// Matches legacy CMasternodeBroadcast::CheckDefaultPort(std::string strService, std::string& strErrorRet, std::string strContext)
func CheckDefaultPortWithContext(addr net.Addr, network NetworkType, context string) CheckDefaultPortResult {
	result := CheckDefaultPortResult{
		Context: context,
	}

	if addr == nil {
		result.Valid = false
		result.Error = "address is nil"
		return result
	}

	var port int
	if tcpAddr, ok := addr.(*net.TCPAddr); ok {
		port = tcpAddr.Port
	} else {
		_, portStr, err := net.SplitHostPort(addr.String())
		if err != nil {
			result.Valid = false
			result.Error = fmt.Sprintf("failed to parse port: %v", err)
			return result
		}
		if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
			result.Valid = false
			result.Error = fmt.Sprintf("failed to parse port number: %v", err)
			return result
		}
	}

	result.Port = port
	expectedPort := network.GetDefaultPort()

	if port != expectedPort {
		result.Valid = false
		// Legacy format: "%s: Invalid port %u for masternode %s, only %d is supported on %s-net."
		result.Error = fmt.Sprintf("%s: Invalid port %d for masternode %s, only %d is supported on %s-net",
			context, port, addr.String(), expectedPort, network.String())
		return result
	}

	result.Valid = true
	return result
}

// getNewSignatureMessage creates the NEW format message for signature verification
// Legacy C++ reference: CMasternodeBroadcast::GetNewStrMessage()
// Format: addr.ToString() + std::to_string(sigTime) + pubKeyCollateralAddress.GetID().ToString() + pubKeyMasternode.GetID().ToString() + std::to_string(protocolVersion)
// Note: GetID().ToString() returns hex-encoded string of the 20-byte hash160
func (mnb *MasternodeBroadcast) getNewSignatureMessage() string {
	var message string

	// Add address string (e.g., "127.0.0.1:37817")
	message += mnb.Addr.String()

	// Add sigtime as string (std::to_string)
	message += fmt.Sprintf("%d", mnb.SigTime)

	// Add collateral public key ID as HEX string
	// LEGACY COMPATIBILITY: C++ uint160::ToString() reverses byte order for display
	if mnb.PubKeyCollateral != nil {
		pubKeyID := crypto.Hash160(mnb.PubKeyCollateral.SerializeCompressed())
		message += ReverseHexBytes(pubKeyID)
	}

	// Add masternode public key ID as HEX string
	// LEGACY COMPATIBILITY: C++ uint160::ToString() reverses byte order for display
	if mnb.PubKeyMasternode != nil {
		pubKeyID := crypto.Hash160(mnb.PubKeyMasternode.SerializeCompressed())
		message += ReverseHexBytes(pubKeyID)
	}

	// Add protocol version as string
	message += fmt.Sprintf("%d", mnb.Protocol)

	return message
}

// ReverseHexBytes returns hex string with bytes in reverse order (matches C++ uint160::ToString())
func ReverseHexBytes(data []byte) string {
	reversed := make([]byte, len(data))
	for i := 0; i < len(data); i++ {
		reversed[i] = data[len(data)-1-i]
	}
	return hex.EncodeToString(reversed)
}

// getOldSignatureMessage creates the OLD format message for backward compatibility
// Legacy C++ reference: CMasternodeBroadcast::GetOldStrMessage()
// Format: addr.ToString() + std::to_string(sigTime) + vchPubKey + vchPubKey2 + std::to_string(protocolVersion)
// Note: Old format uses raw pubkey bytes converted to string (not GetID())
// CRITICAL: Must use original pubkey bytes (may be uncompressed 65 bytes or compressed 33 bytes)
func (mnb *MasternodeBroadcast) getOldSignatureMessage() string {
	var message string

	// Add address string
	message += mnb.Addr.String()

	// Add sigtime as string
	message += fmt.Sprintf("%d", mnb.SigTime)

	// Add raw collateral public key bytes as string (std::string(pubKeyCollateralAddress.begin(), pubKeyCollateralAddress.end()))
	// CRITICAL: Use original bytes if available to preserve compressed/uncompressed format
	if len(mnb.PubKeyCollateralBytes) > 0 {
		message += string(mnb.PubKeyCollateralBytes)
	} else if mnb.PubKeyCollateral != nil {
		message += string(mnb.PubKeyCollateral.SerializeCompressed())
	}

	// Add raw masternode public key bytes as string
	// CRITICAL: Use original bytes if available to preserve compressed/uncompressed format
	if len(mnb.PubKeyMasternodeBytes) > 0 {
		message += string(mnb.PubKeyMasternodeBytes)
	} else if mnb.PubKeyMasternode != nil {
		message += string(mnb.PubKeyMasternode.SerializeCompressed())
	}

	// Add protocol version as string
	message += fmt.Sprintf("%d", mnb.Protocol)

	return message
}

// Verify verifies the masternode ping signature
// Note: Requires the masternode public key to be passed in since MasternodePing doesn't store it
func (mnp *MasternodePing) Verify(pubKey *crypto.PublicKey) error {
	if pubKey == nil {
		return fmt.Errorf("masternode ping verification requires public key")
	}

	if len(mnp.Signature) == 0 {
		return fmt.Errorf("masternode ping has no signature")
	}

	// Legacy uses compact signatures (65 bytes)
	if len(mnp.Signature) != 65 {
		return fmt.Errorf("masternode ping signature must be 65 bytes (compact), got %d", len(mnp.Signature))
	}

	// Create the message to verify
	// Format: vin.ToString() + blockHash.ToString() + std::to_string(sigTime)
	message := mnp.getSignatureMessage()

	// Verify using compact signature (matches C++ obfuScationSigner.VerifyMessage)
	valid, err := crypto.VerifyCompactSignature(pubKey, message, mnp.Signature)
	if err != nil {
		return fmt.Errorf("masternode ping signature verification failed: %w", err)
	}

	if !valid {
		return fmt.Errorf("masternode ping signature verification failed: signature invalid")
	}

	return nil
}

// GetSignatureMessage returns the message that should be signed for ping.
// This is the public interface for external packages (like RPC) that need to sign pings.
// This MUST match the legacy format: vin.ToString() + blockHash.ToString() + std::to_string(sigTime)
// Legacy reference: masternode.cpp CMasternodePing::Sign() line 788
//
// CRITICAL: vin.ToString() in C++ returns the FULL CTxIn format:
//   "CTxIn(COutPoint(hash, index), scriptSig=)"
// NOT just the simple "hash:index" format. This is essential for network compatibility.
func (mnp *MasternodePing) GetSignatureMessage() string {
	return mnp.getSignatureMessage()
}

// getSignatureMessage creates the message that should be signed for ping (internal use)
func (mnp *MasternodePing) getSignatureMessage() string {
	// Build string message matching legacy format EXACTLY
	// Format: vin.ToString() + blockHash.ToString() + std::to_string(sigTime)
	var message string

	// Add vin as string using legacy CTxIn format
	// C++ CTxIn::ToString() outputs: "CTxIn(COutPoint(hash, n), scriptSig=...)"
	message += LegacyTxInString(mnp.OutPoint)

	// Add block hash as hex string (64 chars)
	message += mnp.BlockHash.String()

	// Add sigtime as decimal string
	message += fmt.Sprintf("%d", mnp.SigTime)

	return message
}
