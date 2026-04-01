// Copyright (c) 2025 The TWINS Core developers
// Distributed under the MIT software license

package p2p

import "time"

// PeerInfo represents information about a connected peer (exported for RPC)
type PeerInfo struct {
	ID              int
	Address         string
	AddrLocal       string    // Local address of the connection
	Services        uint64
	LastSend        time.Time
	LastRecv        time.Time
	BytesSent       uint64
	BytesReceived   uint64
	TimeConnected   time.Time
	TimeOffset      int64
	PingTime        float64
	PingWait        float64
	ProtocolVersion int32
	UserAgent       string
	Inbound         bool
	StartHeight     int32
	BanScore        int
	SyncedHeaders        int32     // Last header we have in common with this peer
	SyncedBlocks         int32     // Last block we have in common with this peer
	SyncedHeight         int32     // Best effective height: max(SyncedHeaders, ping/StartHeight)
	Inflight             []int32   // Heights of blocks we're currently requesting from this peer
	LastHeaderUpdateTime time.Time // Last time synced_headers was updated
	Whitelisted          bool      // Whether peer is whitelisted
}

// BanInfo represents information about a banned subnet
type BanInfo struct {
	Subnet      string
	BannedUntil int64
	BanCreated  int64
	Reason      string
}

// P2PServer interface defines methods for RPC network control
type P2PServer interface {
	// Peer management
	GetPeers() []PeerInfo
	GetStats() ServerStats
	PingAllPeers()

	// Node management
	AddNode(addr string, permanent bool) error
	RemoveNode(addr string) error
	ConnectNode(addr string) error
	DisconnectNode(addr string) error
	GetAddedNodes() []string

	// Ban management
	BanSubnet(subnet string, banTime int64, absolute bool, reason string) error
	UnbanSubnet(subnet string) error
	GetBannedList() []BanInfo
	ClearBannedList()

	// Network control
	SetNetworkActive(active bool)

	// Peer aliases
	SetPeerAlias(addr string, alias string) error
	RemovePeerAlias(addr string) error
	GetPeerAliases() map[string]string
}
