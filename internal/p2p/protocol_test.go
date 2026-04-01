package p2p

import (
	"bytes"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageSerialization(t *testing.T) {
	payload := []byte("test payload")
	msg := NewMessage(MsgVersion, payload, MagicToBytes(MainNetMagic))

	// Test serialization
	data, err := msg.Serialize()
	require.NoError(t, err)
	assert.True(t, len(data) >= 24) // Header is 24 bytes

	// Test deserialization
	deserializedMsg, err := DeserializeMessage(data)
	require.NoError(t, err)

	assert.Equal(t, msg.GetCommand(), deserializedMsg.GetCommand())
	assert.Equal(t, msg.Length, deserializedMsg.Length)
	assert.Equal(t, msg.GetMagic(), deserializedMsg.GetMagic())
	assert.Equal(t, payload, deserializedMsg.Payload)
}

func TestMessageChecksum(t *testing.T) {
	payload := []byte("test payload")
	msg := NewMessage(MsgVersion, payload, MagicToBytes(MainNetMagic))

	// Message should have valid checksum
	assert.True(t, msg.ValidateChecksum())

	// Corrupt the payload
	msg.Payload[0] = ^msg.Payload[0]
	assert.False(t, msg.ValidateChecksum())
}

func TestEmptyMessageChecksum(t *testing.T) {
	msg := NewMessage(MsgVerAck, nil, MagicToBytes(MainNetMagic))
	assert.True(t, msg.ValidateChecksum())
}

func TestMessageLimits(t *testing.T) {
	// Test maximum message size
	largePayload := make([]byte, MaxMessageSize+1)
	msg := NewMessage(MsgVersion, largePayload, MagicToBytes(MainNetMagic))

	data, err := msg.Serialize()
	require.NoError(t, err)

	// Should fail to deserialize due to size limit
	_, err = DeserializeMessage(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
}

func TestVersionMessageSerialization(t *testing.T) {
	// Create test addresses
	localAddr := &NetAddress{
		Time:     uint32(time.Now().Unix()),
		Services: SFNodeNetwork,
		IP:       net.ParseIP("127.0.0.1"),
		Port:     18333,
	}

	remoteAddr := &NetAddress{
		Time:     uint32(time.Now().Unix()),
		Services: SFNodeNetwork | SFNodeBloom,
		IP:       net.ParseIP("192.168.1.1"),
		Port:     18333,
	}

	version := &VersionMessage{
		Version:     ProtocolVersion,
		Services:    SFNodeNetwork | SFNodeBloom,
		Timestamp:   time.Now().Unix(),
		AddrRecv:    *remoteAddr,
		AddrFrom:    *localAddr,
		Nonce:       0x123456789abcdef0,
		UserAgent:   "/TWINS-Go:1.0.0/",
		StartHeight: 12345,
		Relay:       true,
	}

	// Serialize
	data, err := SerializeVersionMessage(version)
	require.NoError(t, err)
	assert.True(t, len(data) > 80) // Version messages are quite large

	// Deserialize
	deserializedVersion, err := DeserializeVersionMessage(data)
	require.NoError(t, err)

	assert.Equal(t, version.Version, deserializedVersion.Version)
	assert.Equal(t, version.Services, deserializedVersion.Services)
	assert.Equal(t, version.UserAgent, deserializedVersion.UserAgent)
	assert.Equal(t, version.StartHeight, deserializedVersion.StartHeight)
	assert.Equal(t, version.Relay, deserializedVersion.Relay)
	assert.Equal(t, version.Nonce, deserializedVersion.Nonce)
}

func TestNetAddressSerialization(t *testing.T) {
	addr := &NetAddress{
		Time:     1234567890,
		Services: SFNodeNetwork | SFNodeMasternode,
		IP:       net.ParseIP("192.168.1.100"),
		Port:     18333,
	}

	var buf bytes.Buffer

	// Test with time
	err := serializeNetAddress(&buf, addr, true)
	require.NoError(t, err)
	assert.True(t, buf.Len() > 20)

	// Test deserialization with time
	deserializedAddr := &NetAddress{}
	reader := bytes.NewReader(buf.Bytes())
	err = deserializeNetAddress(reader, deserializedAddr, true)
	require.NoError(t, err)

	assert.Equal(t, addr.Time, deserializedAddr.Time)
	assert.Equal(t, addr.Services, deserializedAddr.Services)
	assert.Equal(t, addr.Port, deserializedAddr.Port)
	assert.True(t, addr.IP.Equal(deserializedAddr.IP))
}

func TestServiceFlagString(t *testing.T) {
	// Test individual flags
	assert.Contains(t, SFNodeNetwork.String(), "NETWORK")
	assert.Contains(t, SFNodeMasternode.String(), "MASTERNODE")

	// Test combined flags
	combined := SFNodeNetwork | SFNodeBloom | SFNodeMasternode
	str := combined.String()
	assert.Contains(t, str, "NETWORK")
	assert.Contains(t, str, "BLOOM")
	assert.Contains(t, str, "MASTERNODE")
}

func TestVarIntSerialization(t *testing.T) {
	testCases := []uint64{
		0,
		1,
		252,
		253,
		0xFFFF,
		0x10000,
		0xFFFFFFFF,
		0x100000000,
		0xFFFFFFFFFFFFFFFF,
	}

	for _, testCase := range testCases {
		var buf bytes.Buffer

		// Write varint
		err := writeVarInt(&buf, testCase)
		require.NoError(t, err, "Failed to write varint %d", testCase)

		// Read varint
		reader := bytes.NewReader(buf.Bytes())
		result, err := readVarInt(reader)
		require.NoError(t, err, "Failed to read varint %d", testCase)

		assert.Equal(t, testCase, result, "VarInt mismatch for value %d", testCase)
	}
}

func TestVarBytesSerialization(t *testing.T) {
	testCases := [][]byte{
		{},
		{0x00},
		{0xFF},
		{0x12, 0x34, 0x56, 0x78},
		make([]byte, 1000), // Large data
	}

	for _, testCase := range testCases {
		var buf bytes.Buffer

		// Write varbytes
		err := writeVarBytes(&buf, testCase)
		require.NoError(t, err, "Failed to write varbytes of length %d", len(testCase))

		// Read varbytes
		reader := bytes.NewReader(buf.Bytes())
		result, err := readVarBytes(reader)
		require.NoError(t, err, "Failed to read varbytes of length %d", len(testCase))

		assert.Equal(t, testCase, result, "VarBytes mismatch")
	}
}

func TestMessageTypes(t *testing.T) {
	// Test all message types can be created
	messageTypes := []MessageType{
		MsgVersion,
		MsgVerAck,
		MsgAddr,
		MsgInv,
		MsgGetData,
		MsgBlock,
		MsgTx,
		MsgGetBlocks,
		MsgGetHeaders,
		MsgHeaders,
		MsgPing,
		MsgPong,
		MsgAlert,
		MsgReject,
		MsgMasternode,
		MsgMNPing,
		MsgDSEG,
	}

	for _, msgType := range messageTypes {
		msg := NewMessage(msgType, []byte("test"), MagicToBytes(MainNetMagic))
		assert.Equal(t, string(msgType), msg.GetCommand())
	}
}

func TestNetworkMagic(t *testing.T) {
	// Test different network magic values
	networks := []uint32{
		MainNetMagic,
		TestNetMagic,
		RegTestMagic,
	}

	for _, magic := range networks {
		msg := NewMessage(MsgVersion, []byte("test"), MagicToBytes(magic))
		assert.Equal(t, magic, msg.GetMagic())

		// Test serialization preserves magic
		data, err := msg.Serialize()
		require.NoError(t, err)

		deserializedMsg, err := DeserializeMessage(data)
		require.NoError(t, err)
		assert.Equal(t, magic, deserializedMsg.GetMagic())
	}
}

func TestInventoryVectorTypes(t *testing.T) {
	// Test basic inventory types
	assert.Equal(t, uint32(1), uint32(InvTypeTx))
	assert.Equal(t, uint32(2), uint32(InvTypeBlock))
	assert.Equal(t, uint32(3), uint32(InvTypeFilteredBlock))

	// Test TWINS-specific inventory types
	assert.Equal(t, uint32(6), uint32(InvTypeSpork))
	assert.Equal(t, uint32(14), uint32(InvTypeMasternodeAnnounce))
	assert.Equal(t, uint32(14), uint32(InvTypeMN)) // Alias

	// All inventory types should be > 0
	invTypes := []InvType{
		InvTypeTx,
		InvTypeBlock,
		InvTypeFilteredBlock,
		InvTypeSpork,
		InvTypeMasternodeAnnounce,
	}

	for _, invType := range invTypes {
		assert.True(t, uint32(invType) > 0, "InvType %d should be > 0", invType)
	}
}

func TestProtocolConstants(t *testing.T) {
	// Verify protocol constants are reasonable
	assert.True(t, ProtocolVersion > 0)
	assert.True(t, MaxInboundConnections > 0)
	assert.True(t, MaxOutboundConnections > 0)
	assert.True(t, MaxMessageSize > 1024) // At least 1KB
	assert.True(t, MaxAddrMessages > 0)
	assert.True(t, MaxInvMessages > 0)

	// Verify timeouts are reasonable
	assert.True(t, HandshakeTimeout > time.Second)
	assert.True(t, PingInterval > time.Second)
	assert.True(t, PingTimeout > PingInterval)
	assert.True(t, ReadTimeout > time.Second)
	assert.True(t, WriteTimeout > time.Second)
}

func TestNetAddressString(t *testing.T) {
	addr := &NetAddress{
		IP:   net.ParseIP("192.168.1.100"),
		Port: 18333,
	}

	str := addr.String()
	assert.Contains(t, str, "192.168.1.100")
	assert.Contains(t, str, "18333")
}

func TestMessageString(t *testing.T) {
	msg := NewMessage(MsgVersion, []byte("test payload"), MagicToBytes(MainNetMagic))
	str := msg.String()

	assert.Contains(t, str, "version")
	assert.Contains(t, str, "12")         // Length of "test payload"
	assert.Contains(t, str, "0x2f1cd30a") // TWINS MainNetMagic in hex
}

func BenchmarkMessageSerialization(b *testing.B) {
	payload := make([]byte, 1024) // 1KB payload
	msg := NewMessage(MsgVersion, payload, MagicToBytes(MainNetMagic))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := msg.Serialize()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMessageDeserialization(b *testing.B) {
	payload := make([]byte, 1024) // 1KB payload
	msg := NewMessage(MsgVersion, payload, MagicToBytes(MainNetMagic))
	data, err := msg.Serialize()
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := DeserializeMessage(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkVersionMessageSerialization(b *testing.B) {
	version := &VersionMessage{
		Version:     ProtocolVersion,
		Services:    SFNodeNetwork,
		Timestamp:   time.Now().Unix(),
		UserAgent:   "/TWINS-Go:1.0.0/",
		StartHeight: 12345,
		Relay:       true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := SerializeVersionMessage(version)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkVarIntSerialization(b *testing.B) {
	values := []uint64{1, 253, 0x10000, 0x100000000}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		for _, val := range values {
			writeVarInt(&buf, val)
		}
	}
}
