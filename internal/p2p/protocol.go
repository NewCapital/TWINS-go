package p2p

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/twins-dev/twins-core/pkg/types"
)

// Protocol constants
const (
	// Protocol version (must match legacy TWINS)
	ProtocolVersion = 70928

	// MinPeerProtocol is the minimum accepted peer protocol version.
	// Peers below this are disconnected. Allows 70927 peers to remain connected.
	MinPeerProtocol = 70927

	// Network magic bytes (TWINS-specific, NOT Bitcoin!)
	MainNetMagic = 0x2f1cd30a // TWINS mainnet: 0x2f, 0x1c, 0xd3, 0x0a
	TestNetMagic = 0xe5bac5b6 // TWINS testnet: 0xe5, 0xba, 0xc5, 0xb6
	RegTestMagic = 0xa1cf7eac // TWINS regtest: 0xa1, 0xcf, 0x7e, 0xac

	// Connection limits
	MaxInboundConnections  = 125
	MaxOutboundConnections = 16 // Match legacy C++ implementation

	// Message limits
	MaxProtocolMessageLength = 2 * 1024 * 1024  // 2 MiB - legacy MAX_PROTOCOL_MESSAGE_LENGTH (net.h:54)
	MaxMessageSize           = 32 * 1024 * 1024  // 32 MiB - hard buffer limit
	MaxAddrMessages          = 1000
	MaxInvMessages           = 50000

	// Addr message safeguards (legacy: addrman.cpp:250, main.cpp:5914)
	AddrTimePenalty     = 2 * 60 * 60 // 7200 seconds — matches C++ nTimePenalty in CAddrMan::Add_
	MaxAddrMsgPerWindow = 1000        // Max addr messages per peer per rate limit window
	AddrMsgWindow       = 10          // Rate limit window in seconds
	AddrRelayDedupSec   = 60          // Seconds to suppress duplicate addr relay

	// Timeouts
	HandshakeTimeout = 30 * time.Second
	PingInterval     = 30 * time.Second
	PingTimeout      = 60 * time.Second
	ReadTimeout      = 150 * time.Second // Must be > legacy PING_INTERVAL (120s)
	WriteTimeout     = 30 * time.Second
)

// Message represents a TWINS protocol message
type Message struct {
	Magic    [4]byte  // Network magic bytes
	Command  [12]byte // Command name
	Length   uint32   // Payload length
	Checksum [4]byte  // Payload checksum
	Payload  []byte   // Message payload
}

// MessageType represents different protocol message types
type MessageType string

const (
	// Core protocol messages
	MsgVersion    MessageType = "version"
	MsgVerAck     MessageType = "verack"
	MsgAddr       MessageType = "addr"
	MsgGetAddr    MessageType = "getaddr"
	MsgInv        MessageType = "inv"
	MsgGetData    MessageType = "getdata"
	MsgNotFound   MessageType = "notfound"
	MsgBlock      MessageType = "block"
	MsgTx         MessageType = "tx"
	MsgGetBlocks  MessageType = "getblocks"
	MsgGetHeaders MessageType = "getheaders"
	MsgHeaders    MessageType = "headers"
	MsgPing       MessageType = "ping"
	MsgPong       MessageType = "pong"
	MsgAlert      MessageType = "alert"
	MsgReject     MessageType = "reject"

	// Bloom filter messages (SPV support)
	MsgMemPool     MessageType = "mempool"     // Request transactions in mempool
	MsgFilterLoad  MessageType = "filterload"  // Load bloom filter
	MsgFilterAdd   MessageType = "filteradd"   // Add entry to bloom filter
	MsgFilterClear MessageType = "filterclear" // Clear bloom filter
	MsgMerkleBlock MessageType = "merkleblock" // Filtered block with merkle branch

	// Masternode messages
	MsgMasternode MessageType = "mnb"   // Masternode broadcast (announce)
	MsgMNPing     MessageType = "mnp"   // Masternode ping
	MsgMNGet MessageType = "mnget" // Get masternode list
	MsgDSEG  MessageType = "dseg"  // Get masternode list segment

	// Masternode network messages
	MsgMasternodeWinner        MessageType = "mnw"  // Masternode winner
	MsgMasternodeScanningError MessageType = "mnse" // Masternode scanning error
	MsgMasternodeQuorum        MessageType = "mnq"  // Masternode quorum

	// NOTE: Budget/Governance messages removed (system permanently disabled via SPORK_13)
	// NOTE: SwiftTX/InstantSend messages removed (deprecated in Go implementation)

	// Network management messages
	MsgSpork     MessageType = "spork"     // Network parameter update (spork)
	MsgGetSporks MessageType = "getsporks" // Request all sporks
	MsgSSC       MessageType = "ssc"       // Sync status count

	// Protocol 70928: Chain state query messages
	MsgGetChainState MessageType = "getchainst" // Request peer's chain state (12 chars max)
	MsgChainState    MessageType = "chainstate"  // Chain state response
)

// ServiceFlag represents the services a peer provides
// Reference: legacy/src/protocol.h:69-81
type ServiceFlag uint64

const (
	SFNodeNetwork        ServiceFlag = 1 << 0 // NODE_NETWORK - Full node, can relay blocks and transactions
	SFNodeGetUTXO        ServiceFlag = 1 << 1 // NODE_GETUTXO - UTXO lookup service (not used in TWINS)
	SFNodeBloom          ServiceFlag = 1 << 2 // NODE_BLOOM - Bloom filter service (SPV support)
	SFNodeWitness        ServiceFlag = 1 << 3 // NODE_WITNESS - Segregated witness support (not used in TWINS)
	SFNodeBloomWithoutMN ServiceFlag = 1 << 4 // NODE_BLOOM_WITHOUT_MN - Bloom filter but no masternode messages
	SFNodeMasternode     ServiceFlag = 1 << 5 // NODE_MASTERNODE - Masternode capable node (TWINS specific)
	// Note: Bits 5-23 reserved for future use
	// Note: Bits 24-31 reserved for temporary experiments
)

// NetAddress represents a network address with service flags
type NetAddress struct {
	Time     uint32      // Timestamp when address was last seen
	Services ServiceFlag // Service flags
	IP       net.IP      // IP address (16 bytes for IPv6 compatibility)
	Port     uint16      // Port number
}

// IsRoutable returns true if the address is publicly routable.
// Matches legacy CNetAddr::IsRoutable() (netbase.cpp:859-862) which excludes
// RFC1918, RFC2544, RFC3927, RFC4862, RFC4843, RFC5737, RFC6598, RFC4193, loopback.
func (a *NetAddress) IsRoutable() bool {
	if a.IP == nil || a.IP.IsUnspecified() || a.IP.IsLoopback() {
		return false
	}
	if a.IP.IsPrivate() || a.IP.IsLinkLocalUnicast() || a.IP.IsLinkLocalMulticast() {
		return false
	}
	// Ranges not covered by Go stdlib that legacy CNetAddr::IsRoutable() excludes
	ip4 := a.IP.To4()
	if ip4 != nil {
		// RFC2544 - 198.18.0.0/15 (benchmark testing)
		if ip4[0] == 198 && (ip4[1]&0xFE) == 18 {
			return false
		}
		// RFC5737 - 192.0.2.0/24 (TEST-NET-1), 198.51.100.0/24 (TEST-NET-2), 203.0.113.0/24 (TEST-NET-3)
		if (ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 2) ||
			(ip4[0] == 198 && ip4[1] == 51 && ip4[2] == 100) ||
			(ip4[0] == 203 && ip4[1] == 0 && ip4[2] == 113) {
			return false
		}
		// RFC6598 - 100.64.0.0/10 (CGNAT)
		if ip4[0] == 100 && (ip4[1]&0xC0) == 64 {
			return false
		}
	} else {
		ip16 := a.IP.To16()
		if ip16 != nil {
			// RFC4843 - 2001:10::/28 (ORCHID)
			if ip16[0] == 0x20 && ip16[1] == 0x01 && ip16[2] == 0x00 && (ip16[3]&0xF0) == 0x10 {
				return false
			}
			// RFC3849 - 2001:db8::/32 (documentation IPv6, rejected by legacy IsValid)
			if ip16[0] == 0x20 && ip16[1] == 0x01 && ip16[2] == 0x0D && ip16[3] == 0xB8 {
				return false
			}
		}
	}
	// NOTE: Legacy exempts Tor onion-cat addresses (fd87:d87e:eb43::/48 within RFC4193 fc00::/7)
	// from the non-routable classification. Go's IsPrivate() blocks all RFC4193 including Tor.
	// This is intentional since the Go implementation does not support Tor networking.
	return true
}

// VersionMessage represents the version handshake message
type VersionMessage struct {
	Version     int32       // Protocol version
	Services    ServiceFlag // Local services
	Timestamp   int64       // Current Unix timestamp
	AddrRecv    NetAddress  // Remote peer's address
	AddrFrom    NetAddress  // Local peer's address
	Nonce       uint64      // Random nonce for connection identification
	UserAgent   string      // Client identification string
	StartHeight int32       // Last block height known to the transmitting node
	Relay       bool        // Whether the peer should relay transactions
}

// InventoryVector represents an inventory entry
type InventoryVector struct {
	Type InvType    // Type of object
	Hash types.Hash // Hash of the object
}

// InvType represents different inventory types
// Reference: legacy/src/protocol.h:176-188
type InvType uint32

const (
	InvTypeTx                      InvType = 1  // MSG_TX - Transaction
	InvTypeBlock                   InvType = 2  // MSG_BLOCK - Block
	InvTypeFilteredBlock           InvType = 3  // MSG_FILTERED_BLOCK - Merkle block (for SPV clients)
	InvTypeSpork                   InvType = 6  // MSG_SPORK - Network parameter update
	InvTypeMasternodeWinner        InvType = 7  // MSG_MASTERNODE_WINNER - Masternode payment winner
	InvTypeMasternodeScanningError InvType = 8  // MSG_MASTERNODE_SCANNING_ERROR - Masternode scanning error
	InvTypeMasternodeQuorum        InvType = 13 // MSG_MASTERNODE_QUORUM - Masternode quorum
	InvTypeMasternodeAnnounce      InvType = 14 // MSG_MASTERNODE_ANNOUNCE - Masternode announcement/broadcast
	InvTypeMasternodePing          InvType = 15 // MSG_MASTERNODE_PING - Masternode ping

	// Legacy aliases for compatibility
	InvTypeMN = InvTypeMasternodeAnnounce // Alias for masternode broadcasts
)

// PingMessage represents a ping message
type PingMessage struct {
	Nonce uint64 // Random nonce
}

// PongMessage represents a pong message
type PongMessage struct {
	Nonce uint64 // Nonce from the ping message
}

// AddrMessage represents an addr message containing peer addresses
type AddrMessage struct {
	Addresses []NetAddress // List of network addresses
}

// InvMessage represents an inv message
type InvMessage struct {
	InvList []InventoryVector // List of inventory vectors
}

// GetDataMessage represents a getdata message
type GetDataMessage struct {
	InvList []InventoryVector // List of requested inventory vectors
}

// GetBlocksMessage represents a getblocks message
type GetBlocksMessage struct {
	Version      uint32       // Protocol version
	BlockLocator []types.Hash // Block locator hashes
	HashStop     types.Hash   // Hash to stop at (zero hash = get all)
}

// GetHeadersMessage represents a getheaders message
type GetHeadersMessage struct {
	Version      uint32       // Protocol version
	BlockLocator []types.Hash // Block locator hashes
	HashStop     types.Hash   // Hash to stop at (zero hash = get all)
}

// HeadersMessage represents a headers message
type HeadersMessage struct {
	Headers []*types.BlockHeader // List of block headers
}

// RejectMessage represents a reject message
type RejectMessage struct {
	Message string // Message that was rejected
	CCode   uint8  // Rejection code
	Reason  string // Reason for rejection
	Data    []byte // Extra data (optional)
}

// ChainStateMessage represents a chainstate response (protocol 70928+).
// Wire format: Version(4) + TipHeight(4) + TipHash(32) + LocatorCount(varint) + Locator hashes(32 each)
type ChainStateMessage struct {
	Version   uint32       // Protocol version of the responder
	TipHeight uint32       // Best block height
	TipHash   types.Hash   // Best block hash
	Locator   []types.Hash // Block locator (exponential step-back, ~25 hashes)
}

// SerializeChainStateMessage serializes a ChainStateMessage to bytes.
func SerializeChainStateMessage(cs *ChainStateMessage) ([]byte, error) {
	// Pre-allocate: 4 + 4 + 32 + varint(~1) + locator*32
	buf := make([]byte, 0, 4+4+32+1+len(cs.Locator)*32)
	b := make([]byte, 4)

	// Version (4 bytes LE)
	binary.LittleEndian.PutUint32(b, cs.Version)
	buf = append(buf, b...)

	// TipHeight (4 bytes LE)
	binary.LittleEndian.PutUint32(b, cs.TipHeight)
	buf = append(buf, b...)

	// TipHash (32 bytes)
	buf = append(buf, cs.TipHash[:]...)

	// Locator count (varint)
	buf = append(buf, encodeVarInt(uint64(len(cs.Locator)))...)

	// Locator hashes
	for _, hash := range cs.Locator {
		buf = append(buf, hash[:]...)
	}

	return buf, nil
}

// DeserializeChainStateMessage deserializes bytes into a ChainStateMessage.
func DeserializeChainStateMessage(data []byte) (*ChainStateMessage, error) {
	if len(data) < 40 { // 4 + 4 + 32 minimum
		return nil, fmt.Errorf("chainstate message too short: %d bytes", len(data))
	}

	cs := &ChainStateMessage{}
	buf := bytes.NewReader(data)

	if err := binary.Read(buf, binary.LittleEndian, &cs.Version); err != nil {
		return nil, fmt.Errorf("failed to read version: %w", err)
	}
	if err := binary.Read(buf, binary.LittleEndian, &cs.TipHeight); err != nil {
		return nil, fmt.Errorf("failed to read tip height: %w", err)
	}
	if _, err := io.ReadFull(buf, cs.TipHash[:]); err != nil {
		return nil, fmt.Errorf("failed to read tip hash: %w", err)
	}

	count, err := readVarInt(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read locator count: %w", err)
	}
	if count > MaxBlockLocatorHashes {
		return nil, fmt.Errorf("locator count too large: %d", count)
	}

	cs.Locator = make([]types.Hash, count)
	for i := uint64(0); i < count; i++ {
		if _, err := io.ReadFull(buf, cs.Locator[i][:]); err != nil {
			return nil, fmt.Errorf("failed to read locator hash %d: %w", i, err)
		}
	}

	return cs, nil
}

// NewMessage creates a new protocol message
func NewMessage(msgType MessageType, payload []byte, magic [4]byte) *Message {
	msg := &Message{
		Payload: payload,
		Length:  uint32(len(payload)),
	}

	// Set magic bytes directly (no endianness conversion needed)
	msg.Magic = magic

	// Set command (padded with null bytes)
	copy(msg.Command[:], []byte(msgType))

	// Calculate checksum (always, even for empty payload)
	// Double-SHA256 of empty payload = 0x5df6e0e2 (matches legacy: net.cpp:202)
	hash := sha256.Sum256(payload)
	secondHash := sha256.Sum256(hash[:])
	copy(msg.Checksum[:], secondHash[:4])

	return msg
}

// Serialize serializes the message to bytes
func (m *Message) Serialize() ([]byte, error) {
	buf := &bytes.Buffer{}

	// Write header
	if err := binary.Write(buf, binary.LittleEndian, m.Magic); err != nil {
		return nil, fmt.Errorf("failed to write magic: %w", err)
	}

	if err := binary.Write(buf, binary.LittleEndian, m.Command); err != nil {
		return nil, fmt.Errorf("failed to write command: %w", err)
	}

	if err := binary.Write(buf, binary.LittleEndian, m.Length); err != nil {
		return nil, fmt.Errorf("failed to write length: %w", err)
	}

	if err := binary.Write(buf, binary.LittleEndian, m.Checksum); err != nil {
		return nil, fmt.Errorf("failed to write checksum: %w", err)
	}

	// Write payload
	if _, err := buf.Write(m.Payload); err != nil {
		return nil, fmt.Errorf("failed to write payload: %w", err)
	}

	return buf.Bytes(), nil
}

// Deserialize deserializes bytes into a message
func DeserializeMessage(data []byte) (*Message, error) {
	if len(data) < 24 { // Header size
		return nil, fmt.Errorf("message too short: %d bytes", len(data))
	}

	msg := &Message{}
	buf := bytes.NewReader(data)

	// Read header
	if err := binary.Read(buf, binary.LittleEndian, &msg.Magic); err != nil {
		return nil, fmt.Errorf("failed to read magic: %w", err)
	}

	if err := binary.Read(buf, binary.LittleEndian, &msg.Command); err != nil {
		return nil, fmt.Errorf("failed to read command: %w", err)
	}

	if err := binary.Read(buf, binary.LittleEndian, &msg.Length); err != nil {
		return nil, fmt.Errorf("failed to read length: %w", err)
	}

	if err := binary.Read(buf, binary.LittleEndian, &msg.Checksum); err != nil {
		return nil, fmt.Errorf("failed to read checksum: %w", err)
	}

	// Validate message length
	if msg.Length > MaxMessageSize {
		return nil, fmt.Errorf("message too large: %d bytes", msg.Length)
	}

	if msg.Length > 0 {
		// Read payload
		msg.Payload = make([]byte, msg.Length)
		if _, err := io.ReadFull(buf, msg.Payload); err != nil {
			return nil, fmt.Errorf("failed to read payload: %w", err)
		}

		// Validate checksum
		if !msg.ValidateChecksum() {
			return nil, fmt.Errorf("invalid checksum")
		}
	}

	return msg, nil
}

// ValidateChecksum validates the message checksum
func (m *Message) ValidateChecksum() bool {
	// Always calculate checksum, even for empty payload
	// Double-SHA256 of empty payload = 0x5df6e0e2 (matches legacy: net.cpp:202)
	hash := sha256.Sum256(m.Payload)
	secondHash := sha256.Sum256(hash[:])
	return bytes.Equal(m.Checksum[:], secondHash[:4])
}

// GetCommand returns the command as a string
func (m *Message) GetCommand() string {
	// Find null terminator
	end := bytes.IndexByte(m.Command[:], 0)
	if end == -1 {
		end = len(m.Command)
	}
	return string(m.Command[:end])
}

// GetMagic returns the magic bytes as uint32
func (m *Message) GetMagic() uint32 {
	return binary.LittleEndian.Uint32(m.Magic[:])
}

// MagicToBytes converts a uint32 magic value to [4]byte in little-endian format
func MagicToBytes(magic uint32) [4]byte {
	var result [4]byte
	binary.LittleEndian.PutUint32(result[:], magic)
	return result
}

// Serialization helpers for specific message types

// SerializeVersionMessage serializes a version message
func SerializeVersionMessage(vm *VersionMessage) ([]byte, error) {
	buf := &bytes.Buffer{}

	// Write version message fields
	binary.Write(buf, binary.LittleEndian, vm.Version)
	binary.Write(buf, binary.LittleEndian, uint64(vm.Services))
	binary.Write(buf, binary.LittleEndian, vm.Timestamp)

	// Write addresses
	if err := serializeNetAddress(buf, &vm.AddrRecv, false); err != nil {
		return nil, err
	}
	if err := serializeNetAddress(buf, &vm.AddrFrom, false); err != nil {
		return nil, err
	}

	binary.Write(buf, binary.LittleEndian, vm.Nonce)

	// Write user agent
	userAgentBytes := []byte(vm.UserAgent)
	if err := writeVarBytes(buf, userAgentBytes); err != nil {
		return nil, err
	}

	binary.Write(buf, binary.LittleEndian, vm.StartHeight)

	if vm.Version >= 70001 {
		if vm.Relay {
			buf.WriteByte(1)
		} else {
			buf.WriteByte(0)
		}
	}

	return buf.Bytes(), nil
}

// DeserializeVersionMessage deserializes a version message
func DeserializeVersionMessage(data []byte) (*VersionMessage, error) {
	if len(data) < 85 { // Minimum version message size
		return nil, fmt.Errorf("version message too short")
	}

	vm := &VersionMessage{}
	buf := bytes.NewReader(data)

	binary.Read(buf, binary.LittleEndian, &vm.Version)
	var services uint64
	binary.Read(buf, binary.LittleEndian, &services)
	vm.Services = ServiceFlag(services)
	binary.Read(buf, binary.LittleEndian, &vm.Timestamp)

	// Read addresses
	if err := deserializeNetAddress(buf, &vm.AddrRecv, false); err != nil {
		return nil, err
	}
	if err := deserializeNetAddress(buf, &vm.AddrFrom, false); err != nil {
		return nil, err
	}

	binary.Read(buf, binary.LittleEndian, &vm.Nonce)

	// Read user agent
	userAgentBytes, err := readVarBytes(buf)
	if err != nil {
		return nil, err
	}
	vm.UserAgent = string(userAgentBytes)

	binary.Read(buf, binary.LittleEndian, &vm.StartHeight)

	if vm.Version >= 70001 && buf.Len() > 0 {
		var relay uint8
		binary.Read(buf, binary.LittleEndian, &relay)
		vm.Relay = relay != 0
	} else {
		vm.Relay = true // Default to true for older versions
	}

	return vm, nil
}

// Helper functions for serialization

func serializeNetAddress(w io.Writer, addr *NetAddress, includeTime bool) error {
	if includeTime {
		binary.Write(w, binary.LittleEndian, addr.Time)
	}
	binary.Write(w, binary.LittleEndian, uint64(addr.Services))

	// Write IP address (16 bytes for IPv6 compatibility)
	ip := addr.IP.To16()
	if ip == nil {
		ip = make([]byte, 16) // Zero IP if conversion fails
	}
	w.Write(ip)

	binary.Write(w, binary.BigEndian, addr.Port) // Port is big-endian
	return nil
}

func deserializeNetAddress(r io.Reader, addr *NetAddress, includeTime bool) error {
	if includeTime {
		binary.Read(r, binary.LittleEndian, &addr.Time)
	}
	var services uint64
	binary.Read(r, binary.LittleEndian, &services)
	addr.Services = ServiceFlag(services)

	// Read IP address (16 bytes in Bitcoin protocol format)
	// IPv4 addresses are stored as IPv4-mapped IPv6: 10 bytes 0x00, 2 bytes 0xFF, 4 bytes IPv4
	ipBytes := make([]byte, 16)
	if _, err := io.ReadFull(r, ipBytes); err != nil {
		return err
	}

	// Check if this is an IPv4-mapped IPv6 address
	// Format: ::ffff:a.b.c.d = 00 00 00 00 00 00 00 00 00 00 FF FF xx xx xx xx
	isIPv4Mapped := true
	for i := 0; i < 10; i++ {
		if ipBytes[i] != 0x00 {
			isIPv4Mapped = false
			break
		}
	}
	if isIPv4Mapped && ipBytes[10] == 0xFF && ipBytes[11] == 0xFF {
		// Extract IPv4 address from last 4 bytes
		addr.IP = net.IPv4(ipBytes[12], ipBytes[13], ipBytes[14], ipBytes[15])
	} else {
		addr.IP = net.IP(ipBytes)
	}

	binary.Read(r, binary.BigEndian, &addr.Port) // Port is big-endian
	return nil
}

func writeVarBytes(w io.Writer, data []byte) error {
	length := len(data)
	if err := writeVarInt(w, uint64(length)); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

func readVarBytes(r io.Reader) ([]byte, error) {
	length, err := readVarInt(r)
	if err != nil {
		return nil, err
	}
	if length > MaxMessageSize {
		return nil, fmt.Errorf("var bytes too long: %d", length)
	}
	data := make([]byte, length)
	_, err = io.ReadFull(r, data)
	return data, err
}

func writeVarString(w io.Writer, str string) error {
	return writeVarBytes(w, []byte(str))
}

func readVarString(r io.Reader) (string, error) {
	data, err := readVarBytes(r)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func writeVarInt(w io.Writer, val uint64) error {
	if val < 0xfd {
		return binary.Write(w, binary.LittleEndian, uint8(val))
	} else if val <= 0xffff {
		binary.Write(w, binary.LittleEndian, uint8(0xfd))
		return binary.Write(w, binary.LittleEndian, uint16(val))
	} else if val <= 0xffffffff {
		binary.Write(w, binary.LittleEndian, uint8(0xfe))
		return binary.Write(w, binary.LittleEndian, uint32(val))
	} else {
		binary.Write(w, binary.LittleEndian, uint8(0xff))
		return binary.Write(w, binary.LittleEndian, val)
	}
}

func readVarInt(r io.Reader) (uint64, error) {
	var discriminant uint8
	if err := binary.Read(r, binary.LittleEndian, &discriminant); err != nil {
		return 0, err
	}

	switch discriminant {
	case 0xff:
		var val uint64
		err := binary.Read(r, binary.LittleEndian, &val)
		return val, err
	case 0xfe:
		var val uint32
		err := binary.Read(r, binary.LittleEndian, &val)
		return uint64(val), err
	case 0xfd:
		var val uint16
		err := binary.Read(r, binary.LittleEndian, &val)
		return uint64(val), err
	default:
		return uint64(discriminant), nil
	}
}

// String returns a string representation of the message
func (m *Message) String() string {
	return fmt.Sprintf("Message{Command: %s, Length: %d, Magic: 0x%08x}",
		m.GetCommand(), m.Length, m.GetMagic())
}

// String returns a string representation of ServiceFlag
func (sf ServiceFlag) String() string {
	var services []string
	if sf&SFNodeNetwork != 0 {
		services = append(services, "NETWORK")
	}
	if sf&SFNodeGetUTXO != 0 {
		services = append(services, "GETUTXO")
	}
	if sf&SFNodeBloom != 0 {
		services = append(services, "BLOOM")
	}
	if sf&SFNodeWitness != 0 {
		services = append(services, "WITNESS")
	}
	if sf&SFNodeBloomWithoutMN != 0 {
		services = append(services, "BLOOM_WITHOUT_MN")
	}
	if sf&SFNodeMasternode != 0 {
		services = append(services, "MASTERNODE")
	}
	if len(services) == 0 {
		return "[]"
	}

	result := "["
	for i, service := range services {
		if i > 0 {
			result += "|"
		}
		result += service
	}
	result += "]"
	return result
}

// String returns a string representation of NetAddress
// Uses net.JoinHostPort to correctly format IPv6 addresses as [IPv6]:port
func (na *NetAddress) String() string {
	return net.JoinHostPort(na.IP.String(), strconv.FormatUint(uint64(na.Port), 10))
}
