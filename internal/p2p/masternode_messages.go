package p2p

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"

	"github.com/twins-dev/twins-core/internal/masternode"
	"github.com/twins-dev/twins-core/pkg/crypto"
	"github.com/twins-dev/twins-core/pkg/types"
)

// SerializeMasternodeBroadcast serializes a masternode broadcast message
// Matches legacy CMasternodeBroadcast::SerializationOp format
func SerializeMasternodeBroadcast(mnb *masternode.MasternodeBroadcast) ([]byte, error) {
	buf := &bytes.Buffer{}

	// Serialize vin (CTxIn): prevout + scriptSig + sequence
	// Legacy: READWRITE(vin) which is full CTxIn serialization
	if _, err := buf.Write(mnb.OutPoint.Hash[:]); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, mnb.OutPoint.Index); err != nil {
		return nil, err
	}
	// Empty scriptSig for masternode collateral (varbytes with 0 length)
	if err := writeVarBytes(buf, []byte{}); err != nil {
		return nil, err
	}
	// Sequence (4 bytes, typically 0xFFFFFFFF)
	if err := binary.Write(buf, binary.LittleEndian, uint32(0xFFFFFFFF)); err != nil {
		return nil, err
	}

	// Serialize address
	if err := writeNetAddr(buf, mnb.Addr); err != nil {
		return nil, err
	}

	// Serialize pubKeyCollateralAddress
	// Use raw bytes if set (preserves original format), otherwise use compressed (modern default)
	var pubKeyCollateralBytes []byte
	if len(mnb.PubKeyCollateralBytes) > 0 {
		pubKeyCollateralBytes = mnb.PubKeyCollateralBytes
	} else {
		pubKeyCollateralBytes = mnb.PubKeyCollateral.SerializeCompressed()
	}
	if err := writeVarBytes(buf, pubKeyCollateralBytes); err != nil {
		return nil, err
	}

	// Serialize pubKeyMasternode
	// Use raw bytes if set (preserves original format), otherwise use compressed (modern default)
	var pubKeyMasternodeBytes []byte
	if len(mnb.PubKeyMasternodeBytes) > 0 {
		pubKeyMasternodeBytes = mnb.PubKeyMasternodeBytes
	} else {
		pubKeyMasternodeBytes = mnb.PubKeyMasternode.SerializeCompressed()
	}
	if err := writeVarBytes(buf, pubKeyMasternodeBytes); err != nil {
		return nil, err
	}

	// Serialize signature
	if err := writeVarBytes(buf, mnb.Signature); err != nil {
		return nil, err
	}

	// Serialize signature time (8 bytes)
	if err := binary.Write(buf, binary.LittleEndian, mnb.SigTime); err != nil {
		return nil, err
	}

	// Serialize protocol version (4 bytes)
	if err := binary.Write(buf, binary.LittleEndian, mnb.Protocol); err != nil {
		return nil, err
	}

	// Serialize lastPing directly (no flag, no var-bytes wrapper)
	// Legacy: READWRITE(lastPing) - always present
	if mnb.LastPing != nil {
		pingBytes, err := SerializeMasternodePing(mnb.LastPing)
		if err != nil {
			return nil, err
		}
		if _, err := buf.Write(pingBytes); err != nil {
			return nil, err
		}
	} else {
		// If no ping, serialize empty ping
		emptyPing := &masternode.MasternodePing{
			OutPoint:  mnb.OutPoint,
			BlockHash: [32]byte{},
			SigTime:   mnb.SigTime,
			Signature: []byte{},
		}
		pingBytes, err := SerializeMasternodePing(emptyPing)
		if err != nil {
			return nil, err
		}
		if _, err := buf.Write(pingBytes); err != nil {
			return nil, err
		}
	}

	// Serialize nLastDsq (8 bytes)
	if err := binary.Write(buf, binary.LittleEndian, mnb.LastDsq); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// DeserializeMasternodeBroadcast deserializes a masternode broadcast message
// Matches legacy CMasternodeBroadcast::SerializationOp format
func DeserializeMasternodeBroadcast(data []byte) (*masternode.MasternodeBroadcast, error) {
	buf := bytes.NewReader(data)

	mnb := &masternode.MasternodeBroadcast{}

	// Deserialize vin (CTxIn): prevout + scriptSig + sequence
	if _, err := io.ReadFull(buf, mnb.OutPoint.Hash[:]); err != nil {
		return nil, fmt.Errorf("failed to read outpoint hash: %w", err)
	}
	if err := binary.Read(buf, binary.LittleEndian, &mnb.OutPoint.Index); err != nil {
		return nil, fmt.Errorf("failed to read outpoint index: %w", err)
	}
	// Read scriptSig (varbytes) - typically empty for masternode collateral
	_, err := readVarBytes(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read scriptSig: %w", err)
	}
	// Read sequence (4 bytes)
	var sequence uint32
	if err := binary.Read(buf, binary.LittleEndian, &sequence); err != nil {
		return nil, fmt.Errorf("failed to read sequence: %w", err)
	}

	// Deserialize address
	addr, err := readNetAddr(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read address: %w", err)
	}
	mnb.Addr = addr

	// Deserialize pubKeyCollateralAddress
	pubKeyCollateralBytes, err := readVarBytes(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read collateral public key: %w", err)
	}
	pubKeyCollateral, err := crypto.ParsePublicKeyFromBytes(pubKeyCollateralBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse collateral public key: %w", err)
	}
	mnb.PubKeyCollateral = pubKeyCollateral
	// Store raw bytes to preserve original format (compressed vs uncompressed) for signature verification
	mnb.PubKeyCollateralBytes = pubKeyCollateralBytes

	// Deserialize pubKeyMasternode
	pubKeyMasternodeBytes, err := readVarBytes(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read masternode public key: %w", err)
	}
	pubKeyMasternode, err := crypto.ParsePublicKeyFromBytes(pubKeyMasternodeBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse masternode public key: %w", err)
	}
	mnb.PubKeyMasternode = pubKeyMasternode
	// Store raw bytes to preserve original format (compressed vs uncompressed) for signature verification
	mnb.PubKeyMasternodeBytes = pubKeyMasternodeBytes

	// Deserialize signature
	mnb.Signature, err = readVarBytes(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read signature: %w", err)
	}

	// Deserialize signature time
	if err := binary.Read(buf, binary.LittleEndian, &mnb.SigTime); err != nil {
		return nil, fmt.Errorf("failed to read signature time: %w", err)
	}

	// Deserialize protocol version
	if err := binary.Read(buf, binary.LittleEndian, &mnb.Protocol); err != nil {
		return nil, fmt.Errorf("failed to read protocol version: %w", err)
	}

	// Deserialize lastPing directly (no flag, no var-bytes wrapper)
	// Legacy: READWRITE(lastPing) - always present
	// Read remaining data for ping deserialization
	remainingBytes, err := io.ReadAll(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read remaining data: %w", err)
	}

	// Find where lastPing ends and nLastDsq begins (last 8 bytes)
	if len(remainingBytes) < 8 {
		return nil, fmt.Errorf("insufficient data for lastPing and nLastDsq")
	}

	pingDataLen := len(remainingBytes) - 8
	pingData := remainingBytes[:pingDataLen]

	mnb.LastPing, err = DeserializeMasternodePing(pingData)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize ping: %w", err)
	}

	// Deserialize nLastDsq (last 8 bytes)
	nLastDsqBuf := bytes.NewReader(remainingBytes[pingDataLen:])
	if err := binary.Read(nLastDsqBuf, binary.LittleEndian, &mnb.LastDsq); err != nil {
		return nil, fmt.Errorf("failed to read last dsq: %w", err)
	}

	return mnb, nil
}

// SerializeMasternodePing serializes a masternode ping message
// Matches legacy CMasternodePing::SerializationOp format
func SerializeMasternodePing(mnp *masternode.MasternodePing) ([]byte, error) {
	buf := &bytes.Buffer{}

	// Serialize vin (CTxIn): prevout + scriptSig + sequence
	// Legacy: READWRITE(vin)
	if _, err := buf.Write(mnp.OutPoint.Hash[:]); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, mnp.OutPoint.Index); err != nil {
		return nil, err
	}
	// Empty scriptSig (varbytes with 0 length)
	if err := writeVarBytes(buf, []byte{}); err != nil {
		return nil, err
	}
	// Sequence (4 bytes)
	if err := binary.Write(buf, binary.LittleEndian, uint32(0xFFFFFFFF)); err != nil {
		return nil, err
	}

	// Serialize block hash (32 bytes)
	// Legacy: READWRITE(blockHash)
	if _, err := buf.Write(mnp.BlockHash[:]); err != nil {
		return nil, err
	}

	// Serialize signature time (8 bytes)
	// Legacy: READWRITE(sigTime)
	if err := binary.Write(buf, binary.LittleEndian, mnp.SigTime); err != nil {
		return nil, err
	}

	// Serialize signature
	// Legacy: READWRITE(vchSig)
	if err := writeVarBytes(buf, mnp.Signature); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// DeserializeMasternodePing deserializes a masternode ping message
// Matches legacy CMasternodePing::SerializationOp format
func DeserializeMasternodePing(data []byte) (*masternode.MasternodePing, error) {
	buf := bytes.NewReader(data)

	mnp := &masternode.MasternodePing{}

	// Deserialize vin (CTxIn): prevout + scriptSig + sequence
	// Legacy: READWRITE(vin)
	if _, err := io.ReadFull(buf, mnp.OutPoint.Hash[:]); err != nil {
		return nil, fmt.Errorf("failed to read outpoint hash: %w", err)
	}
	if err := binary.Read(buf, binary.LittleEndian, &mnp.OutPoint.Index); err != nil {
		return nil, fmt.Errorf("failed to read outpoint index: %w", err)
	}
	// Read scriptSig (varbytes) - typically empty
	_, err := readVarBytes(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read scriptSig: %w", err)
	}
	// Read sequence (4 bytes)
	var sequence uint32
	if err := binary.Read(buf, binary.LittleEndian, &sequence); err != nil {
		return nil, fmt.Errorf("failed to read sequence: %w", err)
	}

	// Deserialize block hash
	// Legacy: READWRITE(blockHash)
	if _, err := io.ReadFull(buf, mnp.BlockHash[:]); err != nil {
		return nil, fmt.Errorf("failed to read block hash: %w", err)
	}

	// Deserialize signature time
	// Legacy: READWRITE(sigTime)
	if err := binary.Read(buf, binary.LittleEndian, &mnp.SigTime); err != nil {
		return nil, fmt.Errorf("failed to read signature time: %w", err)
	}

	// Deserialize signature
	// Legacy: READWRITE(vchSig)
	mnp.Signature, err = readVarBytes(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read signature: %w", err)
	}

	return mnp, nil
}

// Note: Helper functions writeVarInt, readVarInt, writeVarBytes, readVarBytes
// are defined in protocol.go and shared across the p2p package.

// DeserializeCTxIn deserializes a CTxIn (prevout + scriptSig + sequence)
// This matches Bitcoin/TWINS CTxIn serialization format
func DeserializeCTxIn(r io.Reader) (outpoint types.Outpoint, scriptSig []byte, sequence uint32, err error) {
	// Read prevout hash (32 bytes)
	if _, err = io.ReadFull(r, outpoint.Hash[:]); err != nil {
		return outpoint, nil, 0, fmt.Errorf("failed to read prevout hash: %w", err)
	}

	// Read prevout index (4 bytes)
	if err = binary.Read(r, binary.LittleEndian, &outpoint.Index); err != nil {
		return outpoint, nil, 0, fmt.Errorf("failed to read prevout index: %w", err)
	}

	// Read scriptSig (varbytes)
	br, ok := r.(*bytes.Reader)
	if !ok {
		// If not a bytes.Reader, we need to handle differently
		return outpoint, nil, 0, fmt.Errorf("reader must be *bytes.Reader for scriptSig parsing")
	}
	scriptSig, err = readVarBytes(br)
	if err != nil {
		return outpoint, nil, 0, fmt.Errorf("failed to read scriptSig: %w", err)
	}

	// Read sequence (4 bytes)
	if err = binary.Read(r, binary.LittleEndian, &sequence); err != nil {
		return outpoint, nil, 0, fmt.Errorf("failed to read sequence: %w", err)
	}

	return outpoint, scriptSig, sequence, nil
}

func writeNetAddr(buf *bytes.Buffer, addr net.Addr) error {
	// Parse address
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok {
		return fmt.Errorf("unsupported address type: %T", addr)
	}

	// Write IP (16 bytes, IPv6 format)
	ip := tcpAddr.IP.To16()
	if ip == nil {
		return fmt.Errorf("invalid IP address")
	}
	if _, err := buf.Write(ip); err != nil {
		return err
	}

	// Write port (2 bytes, big endian for network byte order)
	return binary.Write(buf, binary.BigEndian, uint16(tcpAddr.Port))
}

func readNetAddr(r io.Reader) (net.Addr, error) {
	// Read IP (16 bytes)
	ipBytes := make([]byte, 16)
	if _, err := io.ReadFull(r, ipBytes); err != nil {
		return nil, err
	}
	ip := net.IP(ipBytes)

	// Read port (2 bytes, big endian)
	var port uint16
	if err := binary.Read(r, binary.BigEndian, &port); err != nil {
		return nil, err
	}

	return &net.TCPAddr{
		IP:   ip,
		Port: int(port),
	}, nil
}
