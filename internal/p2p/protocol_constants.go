package p2p

import "time"

// Protocol constants extracted from Bitcoin/TWINS protocol specification.
// These values are critical for network compatibility and should not be changed
// without understanding their impact on peer communication.

// =============================================================================
// Block Inventory Limits
// =============================================================================

// MaxBlocksPerInventory is the maximum number of blocks that can be sent in a
// single INV message response to getblocks. This is a Bitcoin protocol limit.
//
// Reference: Bitcoin Core src/net_processing.cpp MAX_BLOCKS_IN_TRANSIT_PER_PEER
// Legacy: legacy/src/main.cpp - MAX_BLOCKS_IN_TRANSIT_PER_PEER = 500
//
// When a peer requests blocks via getblocks, we respond with up to this many
// block hashes. If more blocks are available, we set hashContinue for pipelining.
const MaxBlocksPerInventory = 500

// MaxBlockLocatorHashes is the maximum number of hashes allowed in a block
// locator (getblocks/getheaders request). This prevents DoS via oversized locators.
//
// Reference: Bitcoin Core limits locator to ~101 entries (logarithmic backoff)
// We use 500 as a generous upper bound for compatibility.
const MaxBlockLocatorHashes = 500

// MaxHeadersPerMessage is the maximum number of headers in a headers message.
// This is higher than block inventory because headers are much smaller.
//
// Reference: Bitcoin Core MAX_HEADERS_RESULTS = 2000
const MaxHeadersPerMessage = 2000

// =============================================================================
// Time Validation Limits
// =============================================================================

// MaxClockOffsetSeconds is the maximum acceptable clock offset between peers
// and the maximum future timestamp allowed for block headers (2 hours).
//
// Reference: Bitcoin Core main.cpp:5865 (peer time), main.cpp:3081 (block time)
const MaxClockOffsetSeconds = 2 * 60 * 60

// =============================================================================
// Network Diversity Limits (Sybil Protection)
// =============================================================================

// MaxConnectionsPerNetworkGroup limits connections from the same /16 subnet.
// This prevents Sybil attacks where an attacker controls many IPs in one range.
//
// Reference: Bitcoin Core CConnman::Options::nMaxOutboundConnections
// A /16 subnet contains up to 65,536 IPs (e.g., 192.168.0.0 - 192.168.255.255).
// Limiting to 2 connections per /16 ensures network diversity.
const MaxConnectionsPerNetworkGroup = 2

// MinPeersBeforeSeeds is the minimum number of outbound connections before
// activating seed nodes. If we have fewer peers after the seed timeout,
// seeds are activated as an emergency fallback.
//
// Reference: Bitcoin Core nMinConnections in ThreadOpenConnections
// Seeds should be a last resort; prefer known peers from address book.
const MinPeersBeforeSeeds = 3

// =============================================================================
// Sync and Queue Sizes
// =============================================================================

// DefaultBlockQueueSize is the default size for block processing queues.
// This should be large enough to hold a full inventory response (500 blocks)
// without blocking, but not so large as to consume excessive memory.
const DefaultBlockQueueSize = 500

// DefaultInvQueueSize is the default size for inventory processing queues.
const DefaultInvQueueSize = 500

// =============================================================================
// Message Validation Limits
// =============================================================================

// MaxInvCount is the maximum number of inventory vectors in an INV message.
// Already defined in protocol.go as MaxInvMessages = 50000
// Kept here for documentation purposes.
// const MaxInvCount = 50000

// =============================================================================
// Batch Processing Thresholds
// =============================================================================

// DefaultMaxBatchSize is the default maximum batch size for block processing
// when no sync configuration is available.
const DefaultMaxBatchSize = 500

// =============================================================================
// TX Relay Stability Limits
// =============================================================================

// Relay cache bounds (legacy mapRelay-like behavior)
const (
	TxRelayCacheTTL        = 15 * time.Minute
	TxRelayCacheMaxEntries = 50_000
	TxRelayCacheMaxBytes   = 64 * 1024 * 1024 // 64 MiB
)

// Per-peer tx relay queue and dedup bounds
const (
	TxRelayPeerKnownInvMax = 50_000
	TxRelayPeerQueueMax    = 5_000
)

// Trickle flushing behavior for tx inventory
const (
	TxRelayTrickleInterval = 100 * time.Millisecond
	TxRelayTrickleBatchMax = 1_000
	TxRelaySendTimeout     = 3 * time.Second
)

// Mempool request throttling for P2P mempool command
const (
	TxMemPoolRequestMinInterval = 30 * time.Second
	TxMemPoolResponseMaxItems   = 50_000
)
