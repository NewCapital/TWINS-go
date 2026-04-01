package types

import "time"

// ============================================================================
// Currency Constants
// ============================================================================

const (
	// SatoshisPerCoin is the number of satoshis in one TWINS coin
	// Legacy: COIN = 100000000 (defined in amount.h)
	SatoshisPerCoin int64 = 100_000_000

	// MaxReserveBalanceTWINS is the maximum reserve balance in TWINS (100M)
	// This is a sanity limit for the --reservebalance CLI flag
	MaxReserveBalanceTWINS int64 = 100_000_000

	// MaxReserveBalanceSatoshis is the maximum reserve balance in satoshis
	// Calculated as MaxReserveBalanceTWINS * SatoshisPerCoin = 10^16 satoshis
	MaxReserveBalanceSatoshis int64 = MaxReserveBalanceTWINS * SatoshisPerCoin
)

// ============================================================================
// Network Port Constants
// ============================================================================

const (
	// DefaultRPCPort is the default RPC server port for mainnet
	DefaultRPCPort = 37818

	// DefaultP2PPort is the default P2P network port for mainnet
	// This is also defined in ChainParams.DefaultPort but repeated here
	// for convenience in daemon configuration
	DefaultP2PPort = 37817

	// DefaultRPCHost is the default RPC bind address (localhost only)
	DefaultRPCHost = "127.0.0.1"

	// DefaultP2PHost is the default P2P bind address (all interfaces)
	DefaultP2PHost = "0.0.0.0"
)

// ============================================================================
// Timeout Constants
// ============================================================================

const (
	// RPCCallTimeout is the default timeout for RPC calls
	RPCCallTimeout = 10 * time.Second

	// RPCStatusTimeout is the timeout for RPC status checks
	RPCStatusTimeout = 5 * time.Second

	// RPCStartDelay is the delay after starting RPC server
	RPCStartDelay = 100 * time.Millisecond

	// PeerMonitorInterval is the interval for peer count monitoring
	PeerMonitorInterval = 10 * time.Second

	// ShutdownTimeout is the maximum time to wait for graceful shutdown
	ShutdownTimeout = 30 * time.Second

	// MasternodeSaveTimeout is the timeout for saving masternode data during shutdown
	MasternodeSaveTimeout = 5 * time.Second

	// ConsensusStopTimeout is the timeout for stopping the consensus engine during shutdown
	ConsensusStopTimeout = 15 * time.Second
)

// ============================================================================
// Consensus Constants
// ============================================================================

const (
	// DefaultCoinbaseMaturity is the mainnet coinbase/coinstake maturity requirement.
	// Used as fallback when ChainParams is unavailable.
	DefaultCoinbaseMaturity = 60
)

// ============================================================================
// Cryptographic Seed Constants
// ============================================================================

const (
	// SeedLengthBytes is the required length for HD wallet seeds
	SeedLengthBytes = 32

	// MinUniqueByteEntropy is the minimum number of unique bytes required
	// in a 32-byte seed for basic entropy validation.
	// Rationale: For a 32-byte seed, requiring at least 8 unique values (~25%)
	// provides a basic check against obviously weak patterns like repeated bytes.
	// Note: This is a basic sanity check, not a substitute for proper entropy measurement.
	MinUniqueByteEntropy = 8

	// BIP39Iterations is the number of PBKDF2 iterations for BIP39 seed derivation
	BIP39Iterations = 2048

	// BIP39SeedLength is the length of the derived BIP39 seed in bytes
	BIP39SeedLength = 64
)

// ============================================================================
// Default Network Configuration
// ============================================================================

const (
	// DefaultMaxPeers is the default maximum number of P2P connections
	DefaultMaxPeers = 125
)

// ============================================================================
// Component Names for Logging
// ============================================================================

const (
	// ComponentDaemon is the logging component name for the main daemon
	ComponentDaemon = "daemon"

	// ComponentRPC is the logging component name for the RPC server
	ComponentRPC = "rpc"

	// ComponentSyncer is the logging component name for the blockchain syncer
	ComponentSyncer = "syncer"
)
