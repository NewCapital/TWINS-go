package p2p

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	blockchainpkg "github.com/twins-dev/twins-core/internal/blockchain"
	"github.com/twins-dev/twins-core/pkg/types"
)

// Fork detection constants
const (
	// forkCheckInterval is the minimum time between fork detection cycles.
	// Prevents excessive getchainstate traffic on the network.
	forkCheckInterval = 60 * time.Second

	// forkCheckMinPeers is the minimum number of proto-70928 peers required
	// to run a fork check. Below this threshold, we don't have enough data
	// to distinguish a real fork from individual peer issues.
	forkCheckMinPeers = 3

	// forkCheckMaxPeers caps the number of peers queried per check cycle
	// to limit network traffic and processing time.
	forkCheckMaxPeers = 8

	// forkCheckTimeout is the maximum time to wait for a chainstate response
	// from a single peer.
	forkCheckTimeout = 10 * time.Second

	// forkHeightDivergence is the minimum height difference between our chain
	// and the peer median before we investigate. Small differences are normal
	// during block propagation.
	forkHeightDivergence = uint32(2)

	// forkSubnetMaxPerSubnet limits counting peers from the same /24 subnet
	// to mitigate Sybil attacks.
	forkSubnetMaxPerSubnet = 2
)

// forkCheckResult holds the result of querying a single peer's chainstate.
type forkCheckResult struct {
	peerAddr  string
	tipHeight uint32
	tipHash   types.Hash
	locator   []types.Hash
	err       error
}

// checkForForks performs quorum-based proactive fork detection using proto-70928
// chainstate queries. Called periodically from syncMaintenance.
//
// Flow:
//  1. Signal: Compare our height against peer ping-reported heights
//  2. Investigation: Query getchainstate from multiple proto-70928 peers
//  3. Quorum Vote: Group peers by tip hash at the same height
//  4. Decision: Fork confirmed only if majority disagree with our tip
//  5. Recovery: Find fork point from locator and trigger RecoverFromFork
func (bs *BlockchainSyncer) checkForForks() {
	// Skip during IBD — forks are expected during initial sync
	if bs.initialSync.Load() {
		return
	}

	// Skip if we're actively syncing — let the sync complete first
	if bs.syncing.Load() {
		return
	}

	// Skip if no consensus validator or server
	if bs.consensusValidator == nil || bs.server == nil {
		return
	}

	// Rate limit: don't check more than once per forkCheckInterval
	now := time.Now()
	bs.forkDetectionMu.Lock()
	if now.Sub(bs.lastForkCheck) < forkCheckInterval {
		bs.forkDetectionMu.Unlock()
		return
	}
	bs.lastForkCheck = now
	bs.forkDetectionMu.Unlock()

	// Collect proto-70928 peers with their ping heights
	ourHeight := bs.bestHeight.Load()
	if ourHeight == 0 {
		return
	}

	ourTipHash, err := bs.blockchain.GetBestBlockHash()
	if err != nil {
		return
	}

	candidates := bs.collectForkCheckCandidates(ourHeight)
	if len(candidates) < forkCheckMinPeers {
		return
	}

	// Check if there's meaningful divergence worth investigating.
	// If no peer is ahead of us by forkHeightDivergence, skip.
	hasAheadPeer := false
	for _, peer := range candidates {
		if peer.EffectivePeerHeight() >= ourHeight+forkHeightDivergence {
			hasAheadPeer = true
			break
		}
	}
	if !hasAheadPeer {
		return
	}

	// Query chainstate from candidates (concurrently, with timeout)
	results := bs.queryPeerChainStates(candidates)
	if len(results) < forkCheckMinPeers {
		bs.logger.WithFields(logrus.Fields{
			"responses": len(results),
			"required":  forkCheckMinPeers,
		}).Debug("Insufficient chainstate responses for fork check")
		return
	}

	// Analyze results: group by tip hash at our height or higher
	bs.analyzeForkResults(results, ourHeight, ourTipHash)
}

// collectForkCheckCandidates returns proto-70928 peers suitable for fork checking.
// Applies subnet diversity limits to mitigate Sybil attacks.
func (bs *BlockchainSyncer) collectForkCheckCandidates(ourHeight uint32) []*Peer {
	var candidates []*Peer
	subnetCounts := make(map[string]int)

	bs.server.peers.Range(func(key, value interface{}) bool {
		peer := value.(*Peer)
		if !peer.IsConnected() || !peer.IsHandshakeComplete() {
			return true
		}
		if !peer.SupportsProto70928() {
			return true
		}

		// Subnet diversity check: limit peers from same /24
		peerAddr := peer.GetAddress()
		if peerAddr != nil && peerAddr.IP != nil {
			subnet := subnetKey(peerAddr.IP)
			if subnetCounts[subnet] >= forkSubnetMaxPerSubnet {
				return true
			}
			subnetCounts[subnet]++
		}

		candidates = append(candidates, peer)
		return len(candidates) < forkCheckMaxPeers
	})

	return candidates
}

// subnetKey returns the /24 subnet key for an IP address.
func subnetKey(ip net.IP) string {
	ip = ip.To4()
	if ip == nil {
		return "unknown"
	}
	return ip.Mask(net.CIDRMask(24, 32)).String()
}

// queryPeerChainStates sends getchainstate to multiple peers concurrently
// and collects responses with a timeout.
func (bs *BlockchainSyncer) queryPeerChainStates(peers []*Peer) []forkCheckResult {
	var wg sync.WaitGroup
	resultsCh := make(chan forkCheckResult, len(peers))

	for _, peer := range peers {
		wg.Add(1)
		go func(p *Peer) {
			defer wg.Done()
			result := bs.queryPeerChainState(p)
			resultsCh <- result
		}(peer)
	}

	// Wait for all queries to complete
	wg.Wait()
	close(resultsCh)

	var results []forkCheckResult
	for r := range resultsCh {
		if r.err == nil {
			results = append(results, r)
		}
	}
	return results
}

// queryPeerChainState sends a getchainstate request to a single peer and
// waits for the response with a timeout.
func (bs *BlockchainSyncer) queryPeerChainState(peer *Peer) forkCheckResult {
	peerAddr := peer.GetAddress()
	if peerAddr == nil {
		return forkCheckResult{peerAddr: "unknown", err: errNilChainState}
	}
	addr := peerAddr.String()

	// Drain any stale chainstate response from a previous cycle or
	// late arrival after a timeout. Without this, we'd consume outdated data.
	select {
	case <-peer.chainStateCh:
	default:
	}

	// Send getchainstate (empty payload — it's a request message)
	msg := NewMessage(MsgGetChainState, nil, bs.server.getMagicBytes())
	if err := peer.SendMessage(msg); err != nil {
		return forkCheckResult{peerAddr: addr, err: err}
	}

	// Wait for chainstate response on peer's dedicated channel
	select {
	case cs := <-peer.chainStateCh:
		if cs == nil {
			return forkCheckResult{peerAddr: addr, err: errNilChainState}
		}
		return forkCheckResult{
			peerAddr:  addr,
			tipHeight: cs.TipHeight,
			tipHash:   cs.TipHash,
			locator:   cs.Locator,
		}
	case <-time.After(forkCheckTimeout):
		return forkCheckResult{peerAddr: addr, err: errChainStateTimeout}
	case <-bs.quit:
		return forkCheckResult{peerAddr: addr, err: errSyncerStopped}
	}
}

// sentinel errors for chainstate queries
var (
	errNilChainState     = errorString("nil chainstate response")
	errChainStateTimeout = errorString("chainstate response timeout")
	errSyncerStopped     = errorString("syncer stopped")
)

// errorString is a simple string-based error type for sentinel errors.
type errorString string

func (e errorString) Error() string { return string(e) }

// analyzeForkResults compares chainstate responses to determine if we're on a fork.
func (bs *BlockchainSyncer) analyzeForkResults(results []forkCheckResult, ourHeight uint32, ourTipHash types.Hash) {
	// Group peers by their tip hash.
	// We compare at our height: if a peer is at the same or higher height,
	// their tip at our height should match ours if on the same chain.
	type tipGroup struct {
		hash    types.Hash
		peers   []string
		locator []types.Hash // from first peer in group
	}

	groups := make(map[types.Hash]*tipGroup)
	agreesWithUs := 0
	totalVoters := 0

	for _, r := range results {
		// Only consider peers at our height or above
		if r.tipHeight < ourHeight {
			continue
		}

		totalVoters++

		if r.tipHeight == ourHeight {
			// Same height — direct tip comparison
			if r.tipHash == ourTipHash {
				agreesWithUs++
				continue
			}
		} else {
			// Peer is ahead — check if we share a common ancestor via their locator.
			// The locator uses exponential step-back (~25 hashes), so for peers far
			// ahead our exact tip may not appear. Instead, check if ANY locator hash
			// exists in our chain — if so, we share a common chain prefix (same fork,
			// we're just behind).
			if bs.locatorSharesChain(r.locator) {
				agreesWithUs++
				continue
			}
		}

		// This peer disagrees with us
		g, ok := groups[r.tipHash]
		if !ok {
			g = &tipGroup{hash: r.tipHash, locator: r.locator}
			groups[r.tipHash] = g
		}
		g.peers = append(g.peers, r.peerAddr)
	}

	if totalVoters < forkCheckMinPeers {
		bs.logger.WithFields(logrus.Fields{
			"total_voters": totalVoters,
			"required":     forkCheckMinPeers,
		}).Debug("Not enough peers at our height for fork check")
		return
	}

	// Find largest disagreeing group
	var largestGroup *tipGroup
	for _, g := range groups {
		if largestGroup == nil || len(g.peers) > len(largestGroup.peers) {
			largestGroup = g
		}
	}

	if largestGroup == nil {
		// All peers agree with us — no fork
		return
	}

	disagreeCount := totalVoters - agreesWithUs

	bs.logger.WithFields(logrus.Fields{
		"our_height":     ourHeight,
		"our_tip":        ourTipHash.String()[:16],
		"agrees_with_us": agreesWithUs,
		"disagrees":      disagreeCount,
		"total_voters":   totalVoters,
		"largest_group":  len(largestGroup.peers),
		"majority_tip":   largestGroup.hash.String()[:16],
	}).Info("Fork detection quorum results")

	// Quorum check: majority (>50%) must disagree with us
	if disagreeCount <= totalVoters/2 {
		bs.logger.Debug("Fork not confirmed — minority of peers disagree")
		return
	}

	// Fork confirmed! Find the fork point from the majority group's locator.
	forkHeight, found := bs.findForkPointFromLocator(largestGroup.locator)
	if !found {
		bs.logger.Warn("Fork confirmed by quorum but no common block found in locator — skipping recovery")
		return
	}

	bs.logger.WithFields(logrus.Fields{
		"fork_height":    forkHeight,
		"our_height":     ourHeight,
		"majority_tip":   largestGroup.hash.String()[:16],
		"majority_peers": len(largestGroup.peers),
		"total_voters":   totalVoters,
	}).Warn("Fork confirmed by peer quorum — triggering recovery")

	// Trigger recovery (requires concrete type assertion — TriggerRecovery
	// is on *BlockChain, not the Blockchain interface)
	bc, ok := bs.blockchain.(*blockchainpkg.BlockChain)
	if !ok {
		bs.logger.Error("Failed to trigger fork recovery: blockchain type assertion failed")
		return
	}
	forkErr := fmt.Errorf("proactive fork detection: quorum confirmed fork at height %d", forkHeight)
	if err := bc.TriggerRecovery(forkHeight, forkErr); err != nil {
		bs.logger.WithError(err).Error("Failed to trigger fork recovery")
	}
}

// locatorSharesChain checks if any hash in a peer's block locator exists in our chain.
// Used for peers ahead of us: if we share any locator block, we're on the same chain
// and just behind. This is more robust than checking only our tip hash, because the
// locator uses exponential step-back and may skip our exact tip for peers far ahead.
func (bs *BlockchainSyncer) locatorSharesChain(locator []types.Hash) bool {
	if bs.blockchain == nil {
		return false
	}
	for _, hash := range locator {
		has, err := bs.blockchain.HasBlock(hash)
		if err != nil {
			continue
		}
		if has {
			return true
		}
	}
	return false
}

// findForkPointFromLocator walks a peer's block locator to find the highest
// block hash that exists in our chain. That's the fork point.
// Returns (height, true) if found, or (0, false) if no common block exists.
func (bs *BlockchainSyncer) findForkPointFromLocator(locator []types.Hash) (uint32, bool) {
	for _, hash := range locator {
		has, err := bs.blockchain.HasBlock(hash)
		if err != nil {
			continue
		}
		if has {
			height, err := bs.blockchain.GetBlockHeight(hash)
			if err != nil {
				continue
			}
			return height, true
		}
	}
	return 0, false
}
