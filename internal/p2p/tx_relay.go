package p2p

import (
	"bytes"
	"encoding/binary"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/twins-dev/twins-core/pkg/types"
)

type relayCacheEntry struct {
	payload  []byte
	expireAt time.Time
	size     int
}

type txRelayCache struct {
	mu         sync.RWMutex
	items      map[types.Hash]*relayCacheEntry
	order      []types.Hash
	totalBytes int64
}

func newTxRelayCache() *txRelayCache {
	return &txRelayCache{
		items: make(map[types.Hash]*relayCacheEntry),
		order: make([]types.Hash, 0, 1024),
	}
}

func (c *txRelayCache) put(hash types.Hash, payload []byte, now time.Time) {
	if len(payload) == 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if existing, ok := c.items[hash]; ok {
		c.totalBytes -= int64(existing.size)
		// Allocate new slice to avoid mutating backing array visible to concurrent readers.
		existing.payload = append([]byte(nil), payload...)
		existing.expireAt = now.Add(TxRelayCacheTTL)
		existing.size = len(existing.payload)
		c.totalBytes += int64(existing.size)
		c.evictLocked(now)
		return
	}

	entry := &relayCacheEntry{
		payload:  append([]byte(nil), payload...),
		expireAt: now.Add(TxRelayCacheTTL),
		size:     len(payload),
	}
	c.items[hash] = entry
	c.order = append(c.order, hash)
	c.totalBytes += int64(entry.size)
	c.evictLocked(now)
}

func (c *txRelayCache) get(hash types.Hash, now time.Time) ([]byte, bool) {
	c.mu.RLock()
	entry, ok := c.items[hash]
	if !ok {
		c.mu.RUnlock()
		return nil, false
	}
	if now.After(entry.expireAt) {
		c.mu.RUnlock()
		c.mu.Lock()
		// Re-check under write lock.
		entry, ok = c.items[hash]
		if ok && now.After(entry.expireAt) {
			delete(c.items, hash)
			c.totalBytes -= int64(entry.size)
		}
		c.mu.Unlock()
		return nil, false
	}
	payload := append([]byte(nil), entry.payload...)
	c.mu.RUnlock()
	return payload, true
}

func (c *txRelayCache) cleanup(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.evictLocked(now)
}

func (c *txRelayCache) evictLocked(now time.Time) {
	// Phase 1: purge expired.
	for hash, entry := range c.items {
		if now.After(entry.expireAt) {
			delete(c.items, hash)
			c.totalBytes -= int64(entry.size)
		}
	}

	// Phase 2: enforce bounded memory/entries by oldest insertion order.
	for (len(c.items) > TxRelayCacheMaxEntries || c.totalBytes > TxRelayCacheMaxBytes) && len(c.order) > 0 {
		hash := c.order[0]
		c.order = c.order[1:]
		entry, ok := c.items[hash]
		if !ok {
			continue
		}
		delete(c.items, hash)
		c.totalBytes -= int64(entry.size)
	}

	// Compact order if too stale relative to live entries or backing array oversized.
	if (len(c.order) > len(c.items)*2 && len(c.order) > 1024) || cap(c.order) > 4*len(c.order)+1024 {
		compacted := make([]types.Hash, 0, len(c.items))
		for _, hash := range c.order {
			if _, ok := c.items[hash]; ok {
				compacted = append(compacted, hash)
			}
		}
		c.order = compacted
	}
}

type peerTxRelayState struct {
	knownSet       map[types.Hash]struct{}
	knownOrder     []types.Hash
	queue          []types.Hash
	queueSet       map[types.Hash]struct{}
	lastMemPoolReq time.Time
}

func newPeerTxRelayState() *peerTxRelayState {
	return &peerTxRelayState{
		knownSet:   make(map[types.Hash]struct{}),
		knownOrder: make([]types.Hash, 0, 256),
		queue:      make([]types.Hash, 0, 256),
		queueSet:   make(map[types.Hash]struct{}),
	}
}

func (s *peerTxRelayState) markKnown(hash types.Hash) {
	if _, ok := s.knownSet[hash]; ok {
		return
	}
	s.knownSet[hash] = struct{}{}
	s.knownOrder = append(s.knownOrder, hash)
	if len(s.knownOrder) <= TxRelayPeerKnownInvMax {
		return
	}
	evict := s.knownOrder[0]
	s.knownOrder = s.knownOrder[1:]
	delete(s.knownSet, evict)
}

func (s *peerTxRelayState) enqueue(hash types.Hash) bool {
	if _, ok := s.knownSet[hash]; ok {
		return false
	}
	if _, ok := s.queueSet[hash]; ok {
		return false
	}
	if len(s.queue) >= TxRelayPeerQueueMax {
		// Drop oldest queued inv to keep queue bounded.
		drop := s.queue[0]
		s.queue = s.queue[1:]
		delete(s.queueSet, drop)
	}
	s.queue = append(s.queue, hash)
	s.queueSet[hash] = struct{}{}
	return true
}

func (s *peerTxRelayState) popBatch(max int) []types.Hash {
	if len(s.queue) == 0 {
		return nil
	}
	if max > len(s.queue) {
		max = len(s.queue)
	}
	batch := append([]types.Hash(nil), s.queue[:max]...)
	s.queue = s.queue[max:]
	for _, hash := range batch {
		delete(s.queueSet, hash)
	}
	return batch
}

func (s *peerTxRelayState) markBatchKnown(hashes []types.Hash) {
	for _, hash := range hashes {
		s.markKnown(hash)
	}
}

func (s *peerTxRelayState) requeueFront(hashes []types.Hash) {
	if len(hashes) == 0 {
		return
	}

	// Collect hashes for retry while preserving original ordering.
	retry := make([]types.Hash, 0, len(hashes))
	for _, hash := range hashes {
		if _, ok := s.knownSet[hash]; ok {
			continue
		}
		if _, ok := s.queueSet[hash]; ok {
			continue
		}
		retry = append(retry, hash)
	}
	if len(retry) == 0 {
		return
	}

	// Bound queue size by dropping from tail before prepend.
	overflow := len(s.queue) + len(retry) - TxRelayPeerQueueMax
	if overflow > 0 {
		if overflow >= len(s.queue) {
			for _, drop := range s.queue {
				delete(s.queueSet, drop)
			}
			s.queue = s.queue[:0]
		} else {
			dropped := s.queue[len(s.queue)-overflow:]
			for _, drop := range dropped {
				delete(s.queueSet, drop)
			}
			s.queue = s.queue[:len(s.queue)-overflow]
		}
	}

	newQueue := make([]types.Hash, 0, len(retry)+len(s.queue))
	newQueue = append(newQueue, retry...)
	newQueue = append(newQueue, s.queue...)
	s.queue = newQueue
	for _, hash := range retry {
		s.queueSet[hash] = struct{}{}
	}
}

func (s *Server) txRelayLoop() {
	defer s.wg.Done()

	s.ensureTxRelayInitialized()

	ticker := time.NewTicker(TxRelayTrickleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.flushTxRelayQueues()
			s.txRelayCache.cleanup(time.Now())
		case <-s.quit:
			return
		}
	}
}

func (s *Server) relayTransactionToPeers(tx *types.Transaction, exceptPeer *Peer) {
	if tx == nil {
		return
	}
	s.ensureTxRelayInitialized()

	txHash := tx.Hash()
	txBytes, err := tx.Serialize()
	if err == nil {
		s.txRelayCache.put(txHash, txBytes, time.Now())
	} else {
		s.logger.WithError(err).WithField("hash", txHash.String()).
			Debug("Failed to serialize tx for relay cache")
	}

	s.txRelayMu.Lock()
	defer s.txRelayMu.Unlock()

	s.peers.Range(func(_, value interface{}) bool {
		peer := value.(*Peer)
		if exceptPeer != nil && peer.GetAddress().String() == exceptPeer.GetAddress().String() {
			return true
		}
		if !peer.IsConnected() || !peer.IsHandshakeComplete() {
			return true
		}
		version := peer.GetVersion()
		if version != nil && !version.Relay {
			return true
		}

		peer.mu.RLock()
		filter := peer.bloomFilter
		// Hold RLock through matchesBloomFilter to prevent data race with
		// concurrent filteradd mutations on filter.data.
		if filter != nil && filter.loaded && !s.matchesBloomFilter(tx, filter) {
			peer.mu.RUnlock()
			return true
		}
		peer.mu.RUnlock()

		addr := peer.GetAddress().String()
		state := s.peerTxRelay[addr]
		if state == nil {
			state = newPeerTxRelayState()
			s.peerTxRelay[addr] = state
		}
		state.enqueue(txHash)
		return true
	})
}

func (s *Server) flushTxRelayQueues() {
	s.ensureTxRelayInitialized()

	type peerBatch struct {
		addr   string
		peer   *Peer
		hashes []types.Hash
	}

	batches := make([]peerBatch, 0, 32)

	s.txRelayMu.Lock()
	for addr, state := range s.peerTxRelay {
		value, ok := s.peers.Load(addr)
		if !ok {
			delete(s.peerTxRelay, addr)
			continue
		}
		peer := value.(*Peer)
		if !peer.IsConnected() || !peer.IsHandshakeComplete() {
			continue
		}

		version := peer.GetVersion()
		if version != nil && !version.Relay {
			continue
		}

		hashes := state.popBatch(TxRelayTrickleBatchMax)
		if len(hashes) == 0 {
			continue
		}
		batches = append(batches, peerBatch{addr: addr, peer: peer, hashes: hashes})
	}
	s.txRelayMu.Unlock()

	for _, batch := range batches {
		payload := s.buildInvMessage(InvTypeTx, batch.hashes)
		msg := NewMessage(MsgInv, payload, s.getMagicBytes())
		if err := batch.peer.SendMessageWithTimeout(msg, TxRelaySendTimeout); err != nil {
			s.txRelayMu.Lock()
			if state := s.peerTxRelay[batch.addr]; state != nil {
				state.requeueFront(batch.hashes)
			}
			s.txRelayMu.Unlock()

			s.logger.WithFields(logrus.Fields{
				"peer":  batch.peer.GetAddress().String(),
				"count": len(batch.hashes),
			}).WithError(err).Debug("Failed to flush tx relay inventory")
			continue
		}

		s.txRelayMu.Lock()
		if state := s.peerTxRelay[batch.addr]; state != nil {
			state.markBatchKnown(batch.hashes)
		}
		s.txRelayMu.Unlock()
	}
}

func (s *Server) allowMemPoolRequest(peer *Peer, now time.Time) bool {
	s.ensureTxRelayInitialized()
	addr := peer.GetAddress().String()
	s.txRelayMu.Lock()
	defer s.txRelayMu.Unlock()

	state := s.peerTxRelay[addr]
	if state == nil {
		state = newPeerTxRelayState()
		s.peerTxRelay[addr] = state
	}
	if !state.lastMemPoolReq.IsZero() && now.Sub(state.lastMemPoolReq) < TxMemPoolRequestMinInterval {
		return false
	}
	state.lastMemPoolReq = now
	return true
}

func (s *Server) markPeerInventoryKnown(peer *Peer, hash types.Hash) {
	s.ensureTxRelayInitialized()
	addr := peer.GetAddress().String()
	s.txRelayMu.Lock()
	defer s.txRelayMu.Unlock()
	state := s.peerTxRelay[addr]
	if state == nil {
		state = newPeerTxRelayState()
		s.peerTxRelay[addr] = state
	}
	state.markKnown(hash)
}

func serializeInvVectors(invType InvType, hashes []types.Hash) []byte {
	var buf bytes.Buffer
	_ = writeCompactSize(&buf, uint64(len(hashes)))
	for _, hash := range hashes {
		_ = binary.Write(&buf, binary.LittleEndian, uint32(invType))
		buf.Write(hash[:])
	}
	return buf.Bytes()
}

func (s *Server) ensureTxRelayInitialized() {
	s.txRelayMu.Lock()
	defer s.txRelayMu.Unlock()
	if s.txRelayCache == nil {
		s.txRelayCache = newTxRelayCache()
	}
	if s.peerTxRelay == nil {
		s.peerTxRelay = make(map[string]*peerTxRelayState)
	}
}
