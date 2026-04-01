package p2p

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/twins-dev/twins-core/pkg/types"
)

// handlers_compat.go implements handlers for legacy protocol messages that are
// deprecated but still need to be acknowledged for protocol compatibility

// CommandToBytes converts a command string to [12]byte array
func CommandToBytes(cmd string) [12]byte {
	var buf [12]byte
	copy(buf[:], cmd)
	return buf
}

// handleAlertMessage handles alert messages (DEPRECATED)
// Legacy nodes may still send alerts - we acknowledge but don't process
func (s *Server) handleAlertMessage(peer *Peer, msg *Message) {
	s.logger.WithField("peer", peer.GetAddress().String()).
		Debug("Received alert message (alerts deprecated, ignoring)")

	// Alerts are deprecated in modern implementations
	// We acknowledge receipt but don't relay or process them
	// This prevents disconnection from legacy nodes that still send alerts
}

// Bloom filter support for SPV (Simplified Payment Verification) wallets
// These allow light clients to filter blockchain data

const (
	maxBloomFilterBytes  = 36000 // BIP37: MAX_BLOOM_FILTER_SIZE
	maxBloomHashFuncs    = 50    // BIP37: MAX_HASH_FUNCS
	maxFilterAddDataSize = 520   // BIP37: MAX_SCRIPT_ELEMENT_SIZE
	maxMerkleHashes      = MaxInvMessages
	maxMerkleFlagsBytes  = MaxInvMessages
)

// handleFilterLoadMessage handles bloom filter load requests
func (s *Server) handleFilterLoadMessage(peer *Peer, msg *Message) {
	s.logger.WithField("peer", peer.GetAddress().String()).
		Debug("Received filterload message")

	filter, err := parseBloomFilterPayload(msg.Payload)
	if err != nil {
		s.logger.WithField("peer", peer.GetAddress().String()).
			WithError(err).Warn("Invalid filterload message")
		// Don't disconnect, just ignore the invalid message
		return
	}

	// Store filter for this peer
	peer.mu.Lock()
	peer.bloomFilter = filter
	peer.mu.Unlock()

	s.logger.WithFields(logrus.Fields{
		"peer":       peer.GetAddress().String(),
		"filterSize": len(filter.data),
		"hashFuncs":  filter.hashFuncs,
		"tweak":      filter.tweak,
	}).Debug("Bloom filter loaded for peer")

	// After loading a filter, the peer expects filtered data
	// Future inv messages should be filtered through this bloom filter
}

// handleFilterAddMessage handles adding data to an existing bloom filter
func (s *Server) handleFilterAddMessage(peer *Peer, msg *Message) {
	s.logger.WithField("peer", peer.GetAddress().String()).
		Debug("Received filteradd message")

	peer.mu.Lock()
	defer peer.mu.Unlock()

	if peer.bloomFilter == nil || !peer.bloomFilter.loaded {
		s.logger.WithField("peer", peer.GetAddress().String()).
			Warn("Received filteradd without active filter")
		return
	}

	elem, err := parseFilterAddPayload(msg.Payload)
	if err != nil {
		s.logger.WithField("peer", peer.GetAddress().String()).
			WithError(err).Warn("Invalid filteradd message")
		return
	}

	// Add data to existing filter.
	peer.bloomFilter.add(elem)
	peer.bloomFilter.elements++

	s.logger.WithField("peer", peer.GetAddress().String()).
		Debug("Added element to bloom filter")
}

// handleFilterClearMessage handles clearing the bloom filter
func (s *Server) handleFilterClearMessage(peer *Peer, msg *Message) {
	s.logger.WithField("peer", peer.GetAddress().String()).
		Debug("Received filterclear message")

	peer.mu.Lock()
	peer.bloomFilter = nil
	peer.mu.Unlock()

	s.logger.WithField("peer", peer.GetAddress().String()).
		Debug("Bloom filter cleared for peer")

	// After clearing, the peer will receive all transactions again
}

// handleMemPoolMessage handles mempool requests
func (s *Server) handleMemPoolMessage(peer *Peer, msg *Message) {
	peerAddr := peer.GetAddress().String()
	s.logger.WithField("peer", peerAddr).Debug("Received mempool message")

	if s.mempool == nil {
		emptyInv := NewMessage(MsgInv, serializeInvVectors(InvTypeTx, nil), s.getMagicBytes())
		if err := peer.SendMessageWithTimeout(emptyInv, TxRelaySendTimeout); err != nil {
			s.logger.WithField("peer", peerAddr).WithError(err).Debug("Failed to send empty mempool inventory")
		}
		return
	}

	now := time.Now()
	if !s.allowMemPoolRequest(peer, now) {
		s.logger.WithField("peer", peerAddr).Debug("Mempool request rate limited")
		return
	}

	// Snapshot mempool transactions and derive inventory hashes.
	// Hold RLock through all matchesBloomFilter calls to prevent data race
	// with concurrent filteradd mutations on filter.data.
	txs := s.mempool.GetTransactions(TxMemPoolResponseMaxItems)
	hashes := make([]types.Hash, 0, len(txs))

	peer.mu.RLock()
	filter := peer.bloomFilter
	hasFilter := filter != nil && filter.loaded
	for _, tx := range txs {
		if hasFilter && !s.matchesBloomFilter(tx, filter) {
			continue
		}
		hashes = append(hashes, tx.Hash())
	}
	peer.mu.RUnlock()

	if len(hashes) == 0 {
		emptyInv := NewMessage(MsgInv, serializeInvVectors(InvTypeTx, nil), s.getMagicBytes())
		if err := peer.SendMessageWithTimeout(emptyInv, TxRelaySendTimeout); err != nil {
			s.logger.WithField("peer", peerAddr).WithError(err).Debug("Failed to send empty mempool inventory")
		}
		return
	}

	// Send inventory in protocol-sized chunks (legacy MAX_INV_SZ behavior).
	for start := 0; start < len(hashes); start += MaxInvMessages {
		end := start + MaxInvMessages
		if end > len(hashes) {
			end = len(hashes)
		}
		chunk := hashes[start:end]
		payload := serializeInvVectors(InvTypeTx, chunk)
		invMsg := NewMessage(MsgInv, payload, s.getMagicBytes())
		if err := peer.SendMessageWithTimeout(invMsg, TxRelaySendTimeout); err != nil {
			s.logger.WithFields(logrus.Fields{
				"peer":  peerAddr,
				"count": len(chunk),
				"error": err,
			}).Warn("Failed to send mempool inventory chunk")
			return
		}

		for _, hash := range chunk {
			s.markPeerInventoryKnown(peer, hash)
		}
	}
}

// handleMerkleBlockMessage handles merkleblock requests (for SPV)
func (s *Server) handleMerkleBlockMessage(peer *Peer, msg *Message) {
	s.logger.WithField("peer", peer.GetAddress().String()).
		Debug("Received merkleblock message")

	// Merkle blocks are filtered blocks sent to SPV clients
	// They contain the block header and a merkle branch proving
	// inclusion of matching transactions

	// This is typically sent in response to getdata requests from
	// SPV clients that have a bloom filter loaded
}

// Budget finalization messages (legacy governance system)

// handleMNFinalMessage handles masternode budget finalization messages
func (s *Server) handleMNFinalMessage(peer *Peer, msg *Message) {
	s.logger.WithField("peer", peer.GetAddress().String()).
		Debug("Received mnfinal message (budget system disabled)")

	// Budget system is deprecated but we acknowledge to maintain compatibility
	// Legacy nodes may still send these during budget cycles
}

// handleFBVoteMessage handles finalized budget vote messages
func (s *Server) handleFBVoteMessage(peer *Peer, msg *Message) {
	s.logger.WithField("peer", peer.GetAddress().String()).
		Debug("Received fbvote message (budget system disabled)")

	// Budget voting is deprecated but we acknowledge for compatibility
}

// BloomFilter represents a bloom filter for SPV support
type BloomFilter struct {
	data      []byte
	loaded    bool
	elements  int
	hashFuncs uint32
	tweak     uint32
	flags     uint8
}

// Serialize serializes an InvMessage
func (m *InvMessage) Serialize() ([]byte, error) {
	// Count varint + inventory vectors
	buf := make([]byte, 0, 1+len(m.InvList)*36)

	// Add count as varint
	countBytes := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(countBytes, uint64(len(m.InvList)))
	buf = append(buf, countBytes[:n]...)

	// Add each inventory vector (4 bytes type + 32 bytes hash)
	for _, inv := range m.InvList {
		var typeBuf [4]byte
		binary.LittleEndian.PutUint32(typeBuf[:], uint32(inv.Type))
		buf = append(buf, typeBuf[:]...)
		buf = append(buf, inv.Hash[:]...)
	}

	return buf, nil
}

// MerkleBlockMessage represents a filtered block with merkle proof
type MerkleBlockMessage struct {
	Header  types.BlockHeader
	TxCount uint32
	Hashes  []types.Hash
	Flags   []byte
}

// Serialize serializes a merkle block message
func (m *MerkleBlockMessage) Serialize() ([]byte, error) {
	var buf bytes.Buffer

	headerBytes, err := m.Header.Serialize()
	if err != nil {
		return nil, err
	}
	buf.Write(headerBytes)

	if err := binary.Write(&buf, binary.LittleEndian, m.TxCount); err != nil {
		return nil, err
	}
	if err := writeCompactSize(&buf, uint64(len(m.Hashes))); err != nil {
		return nil, err
	}
	for _, hash := range m.Hashes {
		buf.Write(hash[:])
	}
	if err := writeCompactSize(&buf, uint64(len(m.Flags))); err != nil {
		return nil, err
	}
	buf.Write(m.Flags)
	return buf.Bytes(), nil
}

// Deserialize deserializes a merkle block message
func (m *MerkleBlockMessage) Deserialize(data []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("merkle block message too short")
	}

	version := int32(binary.LittleEndian.Uint32(data[:4]))
	headerSize := 80
	if version > 3 {
		headerSize += 32
	}
	if len(data) < headerSize+4 {
		return fmt.Errorf("merkle block message too short for header")
	}

	header, err := types.DeserializeBlockHeader(data[:headerSize])
	if err != nil {
		return err
	}
	m.Header = *header

	reader := bytes.NewReader(data[headerSize:])
	if err := binary.Read(reader, binary.LittleEndian, &m.TxCount); err != nil {
		return err
	}

	hashCount, err := readCompactSize(reader)
	if err != nil {
		return err
	}
	if hashCount > maxMerkleHashes {
		return fmt.Errorf("too many merkle hashes: %d", hashCount)
	}
	m.Hashes = make([]types.Hash, hashCount)
	for i := uint64(0); i < hashCount; i++ {
		if _, err := io.ReadFull(reader, m.Hashes[i][:]); err != nil {
			return err
		}
	}

	flagCount, err := readCompactSize(reader)
	if err != nil {
		return err
	}
	if flagCount > maxMerkleFlagsBytes {
		return fmt.Errorf("too many merkle flags: %d", flagCount)
	}
	m.Flags = make([]byte, flagCount)
	if _, err := io.ReadFull(reader, m.Flags); err != nil {
		return err
	}
	if reader.Len() != 0 {
		return fmt.Errorf("unexpected trailing bytes in merkle block message")
	}

	return nil
}

// Additional message types for full compatibility

const (
	// Additional deprecated message types
	MsgMNFinal MessageType = "mnfinal" // Masternode budget finalization
	MsgFBVote  MessageType = "fbvote"  // Finalized budget vote
)

// Helper function to check if a transaction matches a bloom filter
func (s *Server) matchesBloomFilter(tx *types.Transaction, filter *BloomFilter) bool {
	if filter == nil || !filter.loaded {
		return true // No filter means accept all
	}
	if len(filter.data) == 0 || filter.hashFuncs == 0 {
		return false
	}

	txHash := tx.Hash()
	if filter.contains(txHash[:]) {
		return true
	}

	var outpointBuf [36]byte
	for _, input := range tx.Inputs {
		prevHash := input.PreviousOutput.Hash
		if filter.contains(prevHash[:]) {
			return true
		}
		copy(outpointBuf[:32], prevHash[:])
		binary.LittleEndian.PutUint32(outpointBuf[32:], input.PreviousOutput.Index)
		if filter.contains(outpointBuf[:]) {
			return true
		}
		if len(input.ScriptSig) > 0 && filter.contains(input.ScriptSig) {
			return true
		}
	}

	for _, output := range tx.Outputs {
		if len(output.ScriptPubKey) > 0 && filter.contains(output.ScriptPubKey) {
			return true
		}
	}

	return false
}

// sendFilteredBlock sends a merkle block to an SPV client
func (s *Server) sendFilteredBlock(peer *Peer, block *types.Block) error {
	peer.mu.RLock()
	filter := peer.bloomFilter
	filterLoaded := filter != nil && filter.loaded
	if !filterLoaded {
		peer.mu.RUnlock()
		blockBytes, err := block.Serialize()
		if err != nil {
			return err
		}
		msg := NewMessage(MsgBlock, blockBytes, peer.magic)
		if err := peer.SendMessage(msg); err != nil {
			s.logger.WithError(err).WithField("peer", peer.GetAddress().String()).
				Warn("Failed to send full block to peer")
			return err
		}
		return nil
	}

	// Build merkle block with matching transactions.
	// Hold RLock through all matchesBloomFilter calls to prevent data race
	// with concurrent filteradd mutations on filter.data.
	var matchingTxObjs []*types.Transaction
	matches := make([]bool, 0, len(block.Transactions))
	for _, tx := range block.Transactions {
		matched := s.matchesBloomFilter(tx, filter)
		matches = append(matches, matched)
		if matched {
			matchingTxObjs = append(matchingTxObjs, tx)
		}
	}
	peer.mu.RUnlock()

	hashes, flags := buildPartialMerkleTree(block, matches)

	// Create merkle block message
	merkle := &MerkleBlockMessage{
		Header:  *block.Header,
		TxCount: uint32(len(block.Transactions)),
		Hashes:  hashes,
		Flags:   flags,
	}
	payload, err := merkle.Serialize()
	if err != nil {
		s.logger.WithError(err).WithField("peer", peer.GetAddress().String()).
			Warn("Failed to serialize merkle block, sending full block instead")
		blockBytes, blockErr := block.Serialize()
		if blockErr != nil {
			return blockErr
		}
		fullBlockMsg := NewMessage(MsgBlock, blockBytes, peer.magic)
		if sendErr := peer.SendMessage(fullBlockMsg); sendErr != nil {
			return sendErr
		}
		return nil
	}

	// Send merkle block.
	msg := NewMessage(MsgMerkleBlock, payload, peer.magic)
	if err := peer.SendMessage(msg); err != nil {
		s.logger.WithError(err).WithField("peer", peer.GetAddress().String()).
			Warn("Failed to send merkle block to peer")
		return err
	}

	// Send matching transactions that pass bloom filter.
	for _, tx := range matchingTxObjs {
		s.sendTransaction(peer, tx)
	}

	return nil
}

func parseBloomFilterPayload(payload []byte) (*BloomFilter, error) {
	reader := bytes.NewReader(payload)
	filterSize, err := readCompactSize(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read filter size: %w", err)
	}
	if filterSize > maxBloomFilterBytes {
		return nil, fmt.Errorf("filter too large: %d", filterSize)
	}
	if reader.Len() < int(filterSize)+9 {
		return nil, fmt.Errorf("filterload payload too short")
	}

	filterData := make([]byte, filterSize)
	if _, err := io.ReadFull(reader, filterData); err != nil {
		return nil, fmt.Errorf("failed to read filter data: %w", err)
	}

	var hashFuncs uint32
	if err := binary.Read(reader, binary.LittleEndian, &hashFuncs); err != nil {
		return nil, fmt.Errorf("failed to read hash funcs: %w", err)
	}
	if hashFuncs > maxBloomHashFuncs {
		return nil, fmt.Errorf("too many hash funcs: %d", hashFuncs)
	}

	var tweak uint32
	if err := binary.Read(reader, binary.LittleEndian, &tweak); err != nil {
		return nil, fmt.Errorf("failed to read tweak: %w", err)
	}

	flags, err := reader.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("failed to read flags: %w", err)
	}
	if reader.Len() != 0 {
		return nil, fmt.Errorf("unexpected trailing bytes in filterload payload")
	}

	return &BloomFilter{
		data:      filterData,
		loaded:    true,
		elements:  0,
		hashFuncs: hashFuncs,
		tweak:     tweak,
		flags:     flags,
	}, nil
}

func parseFilterAddPayload(payload []byte) ([]byte, error) {
	reader := bytes.NewReader(payload)
	elemSize, err := readCompactSize(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read element size: %w", err)
	}
	if elemSize > maxFilterAddDataSize {
		return nil, fmt.Errorf("filteradd element too large: %d", elemSize)
	}
	if reader.Len() != int(elemSize) {
		return nil, fmt.Errorf("invalid filteradd payload size")
	}

	elem := make([]byte, elemSize)
	if _, err := io.ReadFull(reader, elem); err != nil {
		return nil, fmt.Errorf("failed to read filteradd element: %w", err)
	}
	return elem, nil
}

func (f *BloomFilter) contains(data []byte) bool {
	if f == nil || !f.loaded || len(f.data) == 0 || f.hashFuncs == 0 {
		return false
	}
	bitCount := uint32(len(f.data) * 8)
	for i := uint32(0); i < f.hashFuncs; i++ {
		bit := bloomHash(i, f.tweak, data) % bitCount
		if !f.isBitSet(bit) {
			return false
		}
	}
	return true
}

func (f *BloomFilter) add(data []byte) {
	if f == nil || !f.loaded || len(f.data) == 0 || f.hashFuncs == 0 {
		return
	}
	bitCount := uint32(len(f.data) * 8)
	for i := uint32(0); i < f.hashFuncs; i++ {
		bit := bloomHash(i, f.tweak, data) % bitCount
		f.setBit(bit)
	}
}

func (f *BloomFilter) isBitSet(bit uint32) bool {
	idx := bit / 8
	mask := byte(1 << (bit % 8))
	return (f.data[idx] & mask) != 0
}

func (f *BloomFilter) setBit(bit uint32) {
	idx := bit / 8
	mask := byte(1 << (bit % 8))
	f.data[idx] |= mask
}

func bloomHash(hashNum, tweak uint32, data []byte) uint32 {
	seed := hashNum*0xFBA4C795 + tweak
	return murmurHash3(seed, data)
}

func buildPartialMerkleTree(block *types.Block, matches []bool) ([]types.Hash, []byte) {
	if block == nil || len(block.Transactions) == 0 {
		return nil, nil
	}

	txHashes := make([]types.Hash, len(block.Transactions))
	for i, tx := range block.Transactions {
		txHashes[i] = tx.Hash()
	}

	if len(matches) != len(txHashes) {
		matches = make([]bool, len(txHashes))
	}

	height := 0
	for calcTreeWidth(height, len(txHashes)) > 1 {
		height++
	}

	hashes := make([]types.Hash, 0, len(txHashes))
	bits := make([]bool, 0, len(txHashes)*2)

	var traverse func(h, pos int)
	traverse = func(h, pos int) {
		match := false
		start := pos << h
		end := (pos + 1) << h
		if end > len(matches) {
			end = len(matches)
		}
		for i := start; i < end; i++ {
			if matches[i] {
				match = true
				break
			}
		}

		bits = append(bits, match)
		if h == 0 || !match {
			hashes = append(hashes, calcTreeHash(h, pos, txHashes))
			return
		}

		traverse(h-1, pos*2)
		if pos*2+1 < calcTreeWidth(h-1, len(txHashes)) {
			traverse(h-1, pos*2+1)
		}
	}

	traverse(height, 0)
	return hashes, packFlags(bits)
}

func calcTreeWidth(height, txCount int) int {
	return (txCount + (1 << height) - 1) >> height
}

func calcTreeHash(height, pos int, txHashes []types.Hash) types.Hash {
	if height == 0 {
		return txHashes[pos]
	}

	left := calcTreeHash(height-1, pos*2, txHashes)
	right := left
	if pos*2+1 < calcTreeWidth(height-1, len(txHashes)) {
		right = calcTreeHash(height-1, pos*2+1, txHashes)
	}

	buf := make([]byte, 0, 64)
	buf = append(buf, left[:]...)
	buf = append(buf, right[:]...)
	first := sha256.Sum256(buf)
	second := sha256.Sum256(first[:])
	return second
}

func packFlags(bits []bool) []byte {
	if len(bits) == 0 {
		return nil
	}
	flags := make([]byte, (len(bits)+7)/8)
	for i, bit := range bits {
		if bit {
			flags[i/8] |= 1 << uint(i%8)
		}
	}
	return flags
}

// murmurHash3 implements 32-bit MurmurHash3 used by BIP37 bloom filters.
func murmurHash3(seed uint32, data []byte) uint32 {
	const (
		c1 uint32 = 0xcc9e2d51
		c2 uint32 = 0x1b873593
	)

	h := seed
	nBlocks := len(data) / 4

	for i := 0; i < nBlocks; i++ {
		k := binary.LittleEndian.Uint32(data[i*4 : i*4+4])
		k *= c1
		k = (k << 15) | (k >> 17)
		k *= c2

		h ^= k
		h = (h << 13) | (h >> 19)
		h = h*5 + 0xe6546b64
	}

	var k1 uint32
	tail := data[nBlocks*4:]
	switch len(tail) {
	case 3:
		k1 ^= uint32(tail[2]) << 16
		fallthrough
	case 2:
		k1 ^= uint32(tail[1]) << 8
		fallthrough
	case 1:
		k1 ^= uint32(tail[0])
		k1 *= c1
		k1 = (k1 << 15) | (k1 >> 17)
		k1 *= c2
		h ^= k1
	}

	h ^= uint32(len(data))
	h ^= h >> 16
	h *= 0x85ebca6b
	h ^= h >> 13
	h *= 0xc2b2ae35
	h ^= h >> 16

	return h
}
