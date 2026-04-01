package p2p

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWhitelistManager_Empty(t *testing.T) {
	wm := NewWhitelistManager(nil)

	// Empty whitelist should allow all
	assert.False(t, wm.IsEnabled())
	assert.True(t, wm.IsWhitelisted(net.ParseIP("192.168.1.1")))
	assert.True(t, wm.IsWhitelisted(net.ParseIP("10.0.0.1")))
	assert.Equal(t, 0, wm.Count())
}

func TestWhitelistManager_SingleIP(t *testing.T) {
	wm := NewWhitelistManager([]string{"192.168.1.100"})

	assert.True(t, wm.IsEnabled())
	assert.Equal(t, 1, wm.Count())

	// Whitelisted IP should be allowed
	assert.True(t, wm.IsWhitelisted(net.ParseIP("192.168.1.100")))

	// Other IPs should be blocked
	assert.False(t, wm.IsWhitelisted(net.ParseIP("192.168.1.101")))
	assert.False(t, wm.IsWhitelisted(net.ParseIP("10.0.0.1")))
}

func TestWhitelistManager_CIDR(t *testing.T) {
	wm := NewWhitelistManager([]string{"192.168.1.0/24"})

	assert.True(t, wm.IsEnabled())
	assert.Equal(t, 1, wm.Count())

	// IPs in the CIDR range should be allowed
	assert.True(t, wm.IsWhitelisted(net.ParseIP("192.168.1.1")))
	assert.True(t, wm.IsWhitelisted(net.ParseIP("192.168.1.100")))
	assert.True(t, wm.IsWhitelisted(net.ParseIP("192.168.1.255")))

	// IPs outside the CIDR range should be blocked
	assert.False(t, wm.IsWhitelisted(net.ParseIP("192.168.2.1")))
	assert.False(t, wm.IsWhitelisted(net.ParseIP("10.0.0.1")))
}

func TestWhitelistManager_Mixed(t *testing.T) {
	wm := NewWhitelistManager([]string{
		"192.168.1.0/24",
		"10.0.0.1",
		"172.16.0.0/16",
	})

	assert.True(t, wm.IsEnabled())
	assert.Equal(t, 3, wm.Count())

	// All whitelisted should be allowed
	assert.True(t, wm.IsWhitelisted(net.ParseIP("192.168.1.50")))
	assert.True(t, wm.IsWhitelisted(net.ParseIP("10.0.0.1")))
	assert.True(t, wm.IsWhitelisted(net.ParseIP("172.16.100.200")))

	// Non-whitelisted should be blocked
	assert.False(t, wm.IsWhitelisted(net.ParseIP("10.0.0.2")))
	assert.False(t, wm.IsWhitelisted(net.ParseIP("8.8.8.8")))
}

func TestWhitelistManager_IPv6(t *testing.T) {
	wm := NewWhitelistManager([]string{
		"::1",
		"fe80::/10",
	})

	assert.True(t, wm.IsEnabled())
	assert.Equal(t, 2, wm.Count())

	// Loopback should be allowed
	assert.True(t, wm.IsWhitelisted(net.ParseIP("::1")))

	// Link-local addresses should be allowed
	assert.True(t, wm.IsWhitelisted(net.ParseIP("fe80::1")))
	assert.True(t, wm.IsWhitelisted(net.ParseIP("fe80::abcd:1234")))

	// Other IPv6 should be blocked
	assert.False(t, wm.IsWhitelisted(net.ParseIP("2001:db8::1")))
}

func TestWhitelistManager_Add(t *testing.T) {
	wm := NewWhitelistManager(nil)

	assert.False(t, wm.IsEnabled())

	// Add single IP
	assert.True(t, wm.Add("192.168.1.1"))
	assert.True(t, wm.IsEnabled())
	assert.True(t, wm.IsWhitelisted(net.ParseIP("192.168.1.1")))

	// Add CIDR
	assert.True(t, wm.Add("10.0.0.0/8"))
	assert.True(t, wm.IsWhitelisted(net.ParseIP("10.255.255.255")))

	// Invalid entry
	assert.False(t, wm.Add("invalid"))
}

func TestWhitelistManager_IsWhitelistedAddr(t *testing.T) {
	wm := NewWhitelistManager([]string{"192.168.1.0/24"})

	// Valid TCP address in whitelist
	tcpAddr := &net.TCPAddr{IP: net.ParseIP("192.168.1.50"), Port: 37817}
	assert.True(t, wm.IsWhitelistedAddr(tcpAddr))

	// TCP address not in whitelist
	tcpAddr2 := &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 37817}
	assert.False(t, wm.IsWhitelistedAddr(tcpAddr2))
}

func TestIsOnionAddress(t *testing.T) {
	// .onion addresses
	assert.True(t, IsOnionAddress("example.onion:9050"))
	assert.True(t, IsOnionAddress("abcdefghij1234567890.onion:37817"))

	// Regular addresses
	assert.False(t, IsOnionAddress("192.168.1.1:37817"))
	assert.False(t, IsOnionAddress("example.com:37817"))
	assert.False(t, IsOnionAddress("onion.example.com:37817"))
}
