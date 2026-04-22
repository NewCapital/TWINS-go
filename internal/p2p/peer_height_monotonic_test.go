package p2p

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRecordPeerDiscovered_TipHeightMonotonic pins the bugfix: re-calling
// RecordPeerDiscovered on an already-tracked peer with a lower tipHeight
// (the static version.StartHeight from the handshake, passed every
// RebuildPeerList tick) must NOT regress TipHeight. The live per-peer height
// learned from ping/inv/headers via UpdateBestKnownHeight must survive.
//
// Without this guard:
//   - `getsyncstatus` displays the stale handshake-time height per peer
//   - GetConsensusHeightWithFallback() (used by isAtConsensusHeight)
//     computes consensus from stale per-peer heights
//   - The staking worker pauses because localHeight > stale consensusHeight
func TestRecordPeerDiscovered_TipHeightMonotonic(t *testing.T) {
	ht := NewPeerHealthTracker()
	addr := "1.2.3.4:17464"

	// Initial handshake: peer reports StartingHeight = 1_490_178.
	ht.RecordPeerDiscovered(addr, 1_490_178, false, TierNone, true)
	stats := ht.GetStats(addr)
	require.NotNil(t, stats)
	require.EqualValues(t, 1_490_178, stats.TipHeight, "initial TipHeight from handshake")

	// Live advance: a ping (proto 70928), inv, or headers message announces
	// height 1_490_182, cascading through UpdateBestKnownHeight from the
	// handler layer.
	ht.UpdateBestKnownHeight(addr, 1_490_182)
	stats = ht.GetStats(addr)
	require.EqualValues(t, 1_490_182, stats.BestKnownHeight, "BestKnownHeight advanced")
	require.EqualValues(t, 1_490_182, stats.TipHeight, "TipHeight cascaded from BestKnownHeight")

	// Periodic RebuildPeerList re-calls RecordPeerDiscovered with the
	// static handshake StartingHeight. This must NOT regress TipHeight.
	ht.RecordPeerDiscovered(addr, 1_490_178, false, TierNone, true)
	stats = ht.GetStats(addr)
	require.EqualValues(t, 1_490_182, stats.TipHeight,
		"TipHeight must remain at the live value learned from ping/inv, not regress to handshake StartingHeight")
	require.EqualValues(t, 1_490_182, stats.BestKnownHeight,
		"BestKnownHeight is untouched by RecordPeerDiscovered")
}

// TestRecordPeerDiscovered_TipHeightAdvancesOnHigherRediscovery confirms the
// monotonic guard still allows forward motion when a re-discovery brings a
// higher value (defensive — shouldn't normally happen for live peers, but
// the API contract allows it).
func TestRecordPeerDiscovered_TipHeightAdvancesOnHigherRediscovery(t *testing.T) {
	ht := NewPeerHealthTracker()
	addr := "5.6.7.8:17464"

	ht.RecordPeerDiscovered(addr, 1000, false, TierNone, true)
	require.EqualValues(t, 1000, ht.GetStats(addr).TipHeight)

	ht.RecordPeerDiscovered(addr, 2000, false, TierNone, true)
	require.EqualValues(t, 2000, ht.GetStats(addr).TipHeight, "higher rediscovery advances TipHeight")
}
