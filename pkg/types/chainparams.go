package types

import (
	"math/big"
	"strings"
	"time"

	"github.com/twins-dev/twins-core/pkg/crypto"
)

// mustCreateDevScriptPubKey converts a Base58 address to scriptPubKey or panics
// Used for initializing dev fund address in chain parameters
func mustCreateDevScriptPubKey(addressStr string) []byte {
	addr, err := crypto.DecodeAddress(addressStr)
	if err != nil {
		panic("invalid dev fund address: " + err.Error())
	}
	return addr.CreateScriptPubKey()
}

// MasternodeTier represents the different masternode tiers
type MasternodeTier int

const (
	MasternodeTierBronze MasternodeTier = iota
	MasternodeTierSilver
	MasternodeTierGold
	MasternodeTierPlatinum
)

// ChainParams defines the parameters for a specific blockchain network
type ChainParams struct {
	// Network identification
	Name          string   // Network name (mainnet, testnet, regtest)
	NetMagicBytes [4]byte  // Magic bytes for network protocol
	DefaultPort   int      // Default P2P port
	DNSSeeds      []string // DNS seeds for peer discovery

	// PoS parameters
	StakeMinAge           time.Duration // Minimum age for coins to stake
	StakeModifierInterval time.Duration // Stake modifier update interval
	MaxFutureBlockTime    time.Duration // Maximum time a block can be from the future
	CoinbaseMaturity      uint32        // Blocks until coinbase outputs can be spent
	MinStakeAmount        int64         // Minimum amount required to stake (in satoshis)

	// Block parameters
	TargetSpacing      time.Duration // Target time between blocks
	MaxBlockSize       uint32        // Maximum block size in bytes
	DifficultyInterval uint32        // Blocks between difficulty adjustments
	PowLimit           uint32        // Proof-of-work difficulty limit (compact format)
	PowLimitBig        *big.Int      // Proof-of-work difficulty limit as big integer
	MinBlockVersion    uint32        // Minimum block version
	MinBlockInterval   time.Duration // Minimum time between blocks
	GenesisTime        int64         // Genesis block timestamp

	// Activation heights for protocol upgrades
	LastPOWBlock                 uint32 // Last proof-of-work block height
	ModifierUpgradeBlock         uint32 // Block height for stake modifier v2 upgrade
	ZerocoinStartHeight          uint32 // Block height when Zerocoin becomes active
	ZerocoinStartTime            int64  // Timestamp when Zerocoin becomes active
	BlockEnforceSerialRange      uint32 // Enforce serial range starting this block
	BlockRecalculateAccumulators uint32 // Trigger recalculation of accumulators
	BlockFirstFraudulent         uint32 // First block that bad serials emerged
	BlockLastGoodCheckpoint      uint32 // Last valid accumulator checkpoint
	BlockEnforceInvalidUTXO      uint32 // Start enforcing the invalid UTXO's
	InvalidAmountFiltered        int64  // Amount of invalid coins filtered (in satoshis)
	BlockZerocoinV2              uint32 // Block that zerocoin v2 becomes active
	EnforceNewSporkKey           int64  // Timestamp - sporks after this must use new key
	RejectOldSporkKey            int64  // Timestamp - fully reject old spork key after this
	// Masternode tiers and their collateral requirements
	MasternodeTiers map[MasternodeTier]int64

	// Reward parameters (in satoshis)
	BlockReward      int64  // Base block reward
	MasternodeReward int64  // Masternode reward percentage (basis points)
	StakeReward      int64  // Stake reward percentage (basis points)
	DevFundReward    int64  // Development fund percentage (basis points)
	DevAddress       []byte // Development fund payout address (scriptPubKey)

	// Genesis block parameters
	GenesisHash       Hash   // Genesis block hash
	GenesisTimestamp  uint32 // Genesis block timestamp
	GenesisNonce      uint32 // Genesis block nonce
	InitialDifficulty uint32 // Genesis block difficulty (compact format)
	InitialReward     int64  // Initial block reward in satoshis

	// Spork keys (for network governance)
	SporkPubKey    string // Current spork public key (hex)
	SporkPubKeyOld string // Old spork public key for backward compatibility (hex)

	// Note: AssumeValidHash and AssumeValidHeight removed
	// We now use dynamic depth-based validation (last 50 blocks fully validated)
}

// Standard reward percentages (in basis points, 1% = 100 bp)
const (
	DefaultBlockReward      = 5000000000 // 50 TWINS in satoshis
	DefaultMasternodeReward = 8000       // 80%
	DefaultStakeReward      = 1000       // 10%
	DefaultDevFundReward    = 1000       // 10%
)

// GetTierCollateral returns the collateral requirement for a specific masternode tier
func (cp *ChainParams) GetTierCollateral(tier MasternodeTier) int64 {
	if collateral, exists := cp.MasternodeTiers[tier]; exists {
		return collateral
	}
	return 0
}

// IsValidTier checks if the given collateral amount corresponds to a valid tier
func (cp *ChainParams) IsValidTier(collateral int64) bool {
	for _, requiredCollateral := range cp.MasternodeTiers {
		if collateral == requiredCollateral {
			return true
		}
	}
	return false
}

// GetTierFromCollateral returns the masternode tier for a given collateral amount
func (cp *ChainParams) GetTierFromCollateral(collateral int64) (MasternodeTier, bool) {
	for tier, requiredCollateral := range cp.MasternodeTiers {
		if collateral == requiredCollateral {
			return tier, true
		}
	}
	return MasternodeTierBronze, false
}

// GetTierRewardPercentage returns the reward percentage for a specific tier
func (cp *ChainParams) GetTierRewardPercentage(tier MasternodeTier) int64 {
	switch tier {
	case MasternodeTierBronze:
		return 1000 // 10%
	case MasternodeTierSilver:
		return 2000 // 20%
	case MasternodeTierGold:
		return 3000 // 30%
	case MasternodeTierPlatinum:
		return 4000 // 40%
	default:
		return 0
	}
}

// MainnetParams returns the chain parameters for the main network
func MainnetParams() *ChainParams {
	return &ChainParams{
		Name:          "mainnet",
		NetMagicBytes: [4]byte{0x2f, 0x1c, 0xd3, 0x0a}, // TWINS mainnet magic bytes
		DefaultPort:   37817,                           // TWINS mainnet port
		DNSSeeds: []string{
			"159.65.195.97",
			"134.209.146.52",
			"46.101.113.6",
			"138.68.154.249",
			"137.184.217.142",
			"165.22.149.70",
			"170.64.157.157",
			"134.122.38.24",
			"45.77.64.171",
			"45.32.36.145",
			"45.77.206.161",
			"207.148.67.25",
		},

		// PoS parameters (from legacy TWINS)
		StakeMinAge:           3 * time.Hour,     // 3 hours (legacy: nStakeMinAge = 3 * 60 * 60)
		StakeModifierInterval: 60 * time.Second,  // 60 seconds (legacy: MODIFIER_INTERVAL = 60)
		MaxFutureBlockTime:    2 * time.Hour,     // 2 hours
		CoinbaseMaturity:      60,                // 60 blocks (legacy: nMaturity)
		MinStakeAmount:        12000 * 100000000, // 12000 TWINS (legacy: nStakeMinInput)

		// Block parameters
		TargetSpacing:      2 * time.Minute,                 // 2 minutes (legacy: nTargetSpacing)
		MaxBlockSize:       1000000,                         // 1MB
		DifficultyInterval: 2016,                            // ~2 weeks at 1 minute blocks
		PowLimit:           0x1e0fffff,                      // Max PoW target (compact format)
		PowLimitBig:        powLimitFromCompact(0x1e0fffff), // ~uint256(0) >> 20 (legacy)

		// Activation heights (from legacy chainparams.cpp:246-259)
		LastPOWBlock:                 400,
		ModifierUpgradeBlock:         200,
		ZerocoinStartHeight:          15000000,
		ZerocoinStartTime:            4070908800,
		BlockEnforceSerialRange:      895400,
		BlockRecalculateAccumulators: 6569605,
		BlockFirstFraudulent:         891737,
		BlockLastGoodCheckpoint:      891730,
		BlockEnforceInvalidUTXO:      902850,
		InvalidAmountFiltered:        268200 * 100000000, // 268200 COIN
		BlockZerocoinV2:              104153160,
		EnforceNewSporkKey:           1547424000, // Mon Jan 14 2019 00:00:00 GMT
		RejectOldSporkKey:            1547510400, // Tue Jan 15 2019 00:00:00 GMT

		// Masternode tiers (amounts in satoshis)
		MasternodeTiers: map[MasternodeTier]int64{
			MasternodeTierBronze:   100000000000000,   // 1M TWINS
			MasternodeTierSilver:   500000000000000,   // 5M TWINS
			MasternodeTierGold:     2000000000000000,  // 20M TWINS
			MasternodeTierPlatinum: 10000000000000000, // 100M TWINS
		},

		// Rewards
		BlockReward:      DefaultBlockReward,
		MasternodeReward: DefaultMasternodeReward,
		StakeReward:      DefaultStakeReward,
		DevFundReward:    DefaultDevFundReward,
		DevAddress:       mustCreateDevScriptPubKey("WmXhHCV6PjXjxJdSXPeC8e4PrY8qTQMBFg"), // TWINS mainnet dev fund address

		// Genesis block parameters (from legacy TWINS)
		GenesisHash:       MustParseHash("0000071cf2d95aec5ba4818418756c93cb12cd627191710e8969f2f35c3530de"), // TWINS mainnet genesis
		GenesisTimestamp:  1546790318,                                                                        // 2019-01-06 16:38:38 UTC
		GenesisNonce:      348223,                                                                            // Legacy TWINS nonce
		InitialDifficulty: 0x1e0ffff0,                                                                        // Initial difficulty
		InitialReward:     50 * 100000000,                                                                    // 50 TWINS

		// Spork keys (from legacy chainparams.cpp:325-326)
		SporkPubKey:    "0496c1186ed9170fe353a6287c6f2b1ec768dcfc0fe71943067a0c21349dcf22af77a37f3202d540e85092ad4179f9b44806269450cd0982071ea7a9375ac7d949",
		SporkPubKeyOld: "04d8ef5cc6ef836335a868be72cf1fa97bb2628a36febc54c004809259b64f2cc8b0dacfd72ca69b3a692c719672ca4f2cbbd7cdd140ad3e1544479ea378a21cc2",

		// DYNAMIC VALIDATION: Depth-based validation (last 50 blocks fully validated)
		// - Blocks >50 blocks old: Minimal validation (fast)
		// - Last 50 blocks: Full PoS validation (secure)
		// - Checkpoints provide chain validity verification
		// - No configuration needed, works automatically!
	}
}

// TestnetParams returns the chain parameters for the test network
func TestnetParams() *ChainParams {
	params := MainnetParams()
	params.Name = "testnet"
	params.NetMagicBytes = [4]byte{0xe5, 0xba, 0xc5, 0xb6} // TWINS testnet magic (legacy line 365-368)
	params.DefaultPort = 37847                             // Legacy line 370
	params.DNSSeeds = []string{
		// Legacy testnet seeds (lines 419-428)
		"46.19.210.197",  // Germany
		"46.19.214.68",   // Singapore
		"142.93.145.197", // Toronto
		"159.65.84.118",  // London
		"167.99.223.138", // Amsterdam
		"68.183.161.44",  // San Francisco
		"46.19.212.68",   // LA
		"46.19.213.68",   // Miami
		"46.19.209.68",   // New York
	}

	// Testnet timing parameters (legacy lines 376-378)
	params.TargetSpacing = 2 * time.Minute // 2 minutes (same as mainnet)
	params.CoinbaseMaturity = 15           // Legacy line 378

	// Testnet collateral (legacy lines 383-392) - same as mainnet
	params.MasternodeTiers = map[MasternodeTier]int64{
		MasternodeTierBronze:   100000000000000,   // 1M TWINS
		MasternodeTierSilver:   500000000000000,   // 5M TWINS
		MasternodeTierGold:     2000000000000000,  // 20M TWINS
		MasternodeTierPlatinum: 10000000000000000, // 100M TWINS
	}

	// Testnet genesis (legacy lines 408-412)
	params.GenesisHash = MustParseHash("00000c538590ec8fc7c6725262788f25cb5cd4aa3120f1fcb4fe5f135f6a0eeb")
	params.GenesisTimestamp = 1559924843 // Legacy line 408
	params.GenesisNonce = 36377          // Legacy line 409

	// Testnet dev address (legacy line 429)
	params.DevAddress = mustCreateDevScriptPubKey("XiAHWrbngwovQPdtWzuehx4BL4dvCFKSW3")

	// Testnet spork keys (from legacy chainparams.cpp:452-453)
	params.SporkPubKey = "04dc3b42d79cdbd29a4694040a060ef5e5b2f50a8d52a28d133c506352e2bc43328ab94f4dc508c9c0a61bb381c98b6e0b7319bf87b4f76a52af55058ecaefe968"
	params.SporkPubKeyOld = "048c5897893ef51a021ef4aa4f095790942ec289d01da5c1f1488d0eccdb08762c3a815a91871526ed2861a1551881f7fc91d8ebc8d84f0f849689ca5211807852"

	// Testnet activation heights (legacy lines 395-405)
	params.ZerocoinStartHeight = 200
	params.ZerocoinStartTime = 1537223238
	params.BlockEnforceSerialRange = 1
	params.BlockRecalculateAccumulators = 9908000
	params.BlockFirstFraudulent = 9891737
	params.BlockLastGoodCheckpoint = 9891730
	params.BlockEnforceInvalidUTXO = 9902850
	params.InvalidAmountFiltered = 0
	params.BlockZerocoinV2 = 15444020
	params.ModifierUpgradeBlock = 51197 // Legacy line 380

	return params
}

// RegtestParams returns the chain parameters for regression testing
func RegtestParams() *ChainParams {
	params := TestnetParams()
	params.Name = "regtest"
	params.NetMagicBytes = [4]byte{0xa1, 0xcf, 0x7e, 0xac} // Legacy line 477-480
	params.DefaultPort = 5467                              // Legacy line 494
	params.DNSSeeds = []string{}                           // No DNS seeds for regtest (legacy line 497-498)

	// Timing parameters (legacy lines 486-487)
	params.StakeMinAge = 1 * time.Minute
	params.TargetSpacing = 2 * time.Minute // 2 minutes (legacy line 487)
	params.CoinbaseMaturity = 1            // 1 block
	params.DifficultyInterval = 10         // Quick difficulty adjustments

	// Minimal collateral for regression testing
	params.MasternodeTiers = map[MasternodeTier]int64{
		MasternodeTierBronze:   100000000,   // 1 TWINS
		MasternodeTierSilver:   500000000,   // 5 TWINS
		MasternodeTierGold:     2000000000,  // 20 TWINS
		MasternodeTierPlatinum: 10000000000, // 100 TWINS
	}

	// Regtest genesis (legacy lines 489-491)
	params.GenesisTimestamp = 1537120201  // Legacy line 489
	params.GenesisNonce = 12345           // Legacy line 491
	params.InitialDifficulty = 0x207fffff // Minimal difficulty (legacy line 490)

	return params
}

// DefaultChainParams returns the default chain parameters (mainnet)
func DefaultChainParams() *ChainParams {
	return MainnetParams()
}

// MustParseHash parses a hex-encoded hash string and panics on error
// Used for initializing hardcoded genesis hashes
// Bitcoin/TWINS convention: hash strings are in little-endian (display format)
func MustParseHash(hexStr string) Hash {
	// Remove 0x prefix if present
	if strings.HasPrefix(hexStr, "0x") || strings.HasPrefix(hexStr, "0X") {
		hexStr = hexStr[2:]
	}

	hash, err := NewHashFromString(hexStr)
	if err != nil {
		panic("invalid genesis hash: " + err.Error())
	}

	return hash
}

// powLimitFromCompact converts a compact difficulty representation to big.Int
// Compact format: 0x1e0ffff0 = 0x0ffff0 * 2^(8*(0x1e - 3))
func powLimitFromCompact(compact uint32) *big.Int {
	// Extract size and mantissa from compact representation
	size := compact >> 24
	mantissa := compact & 0x00ffffff

	// Check for negative or overflow
	if mantissa > 0x7fffff {
		return big.NewInt(0)
	}

	// Calculate result
	result := big.NewInt(int64(mantissa))
	if size <= 3 {
		result.Rsh(result, uint(8*(3-size)))
	} else {
		result.Lsh(result, uint(8*(size-3)))
	}

	return result
}
