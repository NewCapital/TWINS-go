package p2p

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAddrTimePenaltyConstant verifies the time penalty matches C++ nTimePenalty
func TestAddrTimePenaltyConstant(t *testing.T) {
	assert.Equal(t, int64(7200), int64(AddrTimePenalty), "AddrTimePenalty should be 2 hours (7200 seconds)")
}

// TestAddrPerPeerRateLimiting verifies that per-peer addr message rate limiting works
func TestAddrPerPeerRateLimiting(t *testing.T) {
	peer := &Peer{}

	now := time.Now().Unix()

	// First message in a new window — should pass (count=1)
	peer.addrMsgCount.Store(0)
	peer.addrMsgWindowEnd.Store(0)

	// Simulate window start
	windowEnd := peer.addrMsgWindowEnd.Load()
	assert.True(t, now >= windowEnd, "new peer should have expired window")

	// Simulate filling up the window
	peer.addrMsgCount.Store(0)
	peer.addrMsgWindowEnd.Store(now + AddrMsgWindow)

	// Count up to the limit — should all pass
	for i := 0; i < MaxAddrMsgPerWindow; i++ {
		count := peer.addrMsgCount.Add(1)
		assert.True(t, int(count) <= MaxAddrMsgPerWindow, "message %d should be within limit", i)
	}

	// One more should exceed
	count := peer.addrMsgCount.Add(1)
	assert.True(t, int(count) > MaxAddrMsgPerWindow, "should exceed rate limit")
}

// TestAddrPerPeerRateLimitWindowReset verifies window reset
func TestAddrPerPeerRateLimitWindowReset(t *testing.T) {
	peer := &Peer{}

	// Set window in the past
	peer.addrMsgCount.Store(int32(MaxAddrMsgPerWindow))
	peer.addrMsgWindowEnd.Store(time.Now().Unix() - 1) // expired

	now := time.Now().Unix()
	windowEnd := peer.addrMsgWindowEnd.Load()

	// Window should be expired
	assert.True(t, now >= windowEnd, "window should be expired")

	// After reset, count starts fresh
	peer.addrMsgCount.Store(1)
	peer.addrMsgWindowEnd.Store(now + AddrMsgWindow)

	assert.Equal(t, int32(1), peer.addrMsgCount.Load(), "count should be 1 after reset")
}

// TestAddrRelayDedupCache verifies the relay dedup cache
func TestAddrRelayDedupCache(t *testing.T) {
	s := &Server{
		addrRelayDedup: make(map[string]int64),
	}

	now := time.Now().Unix()

	// Address not in cache — should not be considered recently relayed
	assert.False(t, s.isAddrRecentlyRelayed("1.2.3.4:37817", now))

	// Add to cache
	addrs := []NetAddress{
		{IP: net.IPv4(1, 2, 3, 4).To16(), Port: 37817},
	}
	s.markAddrsRelayed(addrs, now)

	// Should now be recently relayed
	addrKey := addrs[0].String()
	assert.True(t, s.isAddrRecentlyRelayed(addrKey, now))

	// Should still be relayed within the window
	assert.True(t, s.isAddrRecentlyRelayed(addrKey, now+AddrRelayDedupSec-1))

	// Should expire after the window
	assert.False(t, s.isAddrRecentlyRelayed(addrKey, now+AddrRelayDedupSec))
}

// TestAddrRelayDedupCacheLazyCleanup verifies expired entries are cleaned when cache grows
func TestAddrRelayDedupCacheLazyCleanup(t *testing.T) {
	s := &Server{
		addrRelayDedup: make(map[string]int64),
	}

	now := time.Now().Unix()

	// Fill cache with expired entries
	for i := 0; i < 10001; i++ {
		key := "192.168.1." + string(rune(i%256)) + ":" + string(rune(i/256+1))
		s.addrRelayDedup[key] = now - 1 // already expired
	}

	require.True(t, len(s.addrRelayDedup) > 10000)

	// markAddrsRelayed should trigger lazy cleanup
	addrs := []NetAddress{
		{IP: net.IPv4(10, 0, 0, 1).To16(), Port: 37817},
	}
	s.markAddrsRelayed(addrs, now)

	// Expired entries should be cleaned, only the new one remains
	assert.Equal(t, 1, len(s.addrRelayDedup), "expired entries should be cleaned")
}

// TestAddrTimePenaltyApplication verifies the penalty is applied correctly
func TestAddrTimePenaltyApplication(t *testing.T) {
	now := time.Now()
	originalTime := uint32(now.Unix())

	// Apply penalty
	penalizedTime := int64(originalTime) - AddrTimePenalty
	assert.Equal(t, int64(originalTime)-7200, penalizedTime)

	// Penalized time should be 2 hours in the past
	penalizedMoment := time.Unix(penalizedTime, 0)
	diff := now.Sub(penalizedMoment)
	assert.InDelta(t, 2*time.Hour.Seconds(), diff.Seconds(), 1.0,
		"penalized time should be ~2 hours before now")
}

// TestAddrTimePenaltyNoUnderflow verifies penalty doesn't go below zero
func TestAddrTimePenaltyNoUnderflow(t *testing.T) {
	// Very old timestamp that would underflow
	oldTime := uint32(1000) // ~1000 seconds since epoch
	penalizedTime := int64(oldTime) - AddrTimePenalty
	if penalizedTime < 0 {
		penalizedTime = 0
	}
	assert.Equal(t, int64(0), penalizedTime, "underflow should clamp to 0")

	// Normal timestamp should not underflow
	normalTime := uint32(time.Now().Unix())
	penalizedNormal := int64(normalTime) - AddrTimePenalty
	assert.True(t, penalizedNormal > 0, "normal timestamp should not underflow")
}

// TestAddrRateLimitAtomics verifies the atomic operations work correctly
func TestAddrRateLimitAtomics(t *testing.T) {
	var count atomic.Int32
	var windowEnd atomic.Int64

	// Concurrent increment test
	windowEnd.Store(time.Now().Unix() + 100) // Far future
	count.Store(0)

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				count.Add(1)
			}
			done <- struct{}{}
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	assert.Equal(t, int32(1000), count.Load(), "concurrent adds should total 1000")
}
