package p2p

import (
	"net"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// TestPeerQueueSize verifies peers can be created with custom queue sizes
func TestPeerQueueSize(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Create mock connection
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Test with different queue sizes
	testCases := []struct {
		name      string
		queueSize int
		expected  int
	}{
		{"Default 500", 500, 500},
		{"Large 2000", 2000, 2000},
		{"Small 50", 50, 50},
		{"Minimum enforced", 5, 10}, // Should be bumped to minimum of 10
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			peer := NewPeerWithQueueSize(server, true, [4]byte{0x01, 0x02, 0x03, 0x04}, logger, tc.queueSize)
			assert.Equal(t, tc.expected, cap(peer.writeQueue), "Queue capacity should match expected")
		})
	}
}

// TestWriteQueueBackpressure verifies backpressure mechanism
func TestWriteQueueBackpressure(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Create mock connection
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Create peer with very small queue for testing
	peer := NewPeerWithQueueSize(server, true, [4]byte{0x01, 0x02, 0x03, 0x04}, logger, 10)
	peer.connected.Store(true)

	// Fill the queue completely (don't start write loop so messages stay queued)
	magic := [4]byte{0x01, 0x02, 0x03, 0x04}
	for i := 0; i < 10; i++ {
		msg := NewMessage(MsgPing, []byte{0x00}, magic)
		err := peer.SendMessageWithTimeout(msg, 10*time.Millisecond)
		assert.NoError(t, err, "Should successfully queue first 10 messages")
	}

	// Next message should trigger backpressure
	msg := NewMessage(MsgPing, []byte{0x00}, magic)
	err := peer.SendMessageWithTimeout(msg, 50*time.Millisecond)
	assert.Error(t, err, "Should timeout when queue is full")
	assert.Contains(t, err.Error(), "timeout", "Error should mention timeout")
}

// TestQueueHealthMonitoring verifies queue health metrics
func TestQueueHealthMonitoring(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Create mock connection
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Create peer with known queue size
	peer := NewPeerWithQueueSize(server, true, [4]byte{0x01, 0x02, 0x03, 0x04}, logger, 100)
	peer.connected.Store(true)

	// Check initial health (empty queue)
	health := peer.GetQueueHealth()
	assert.Equal(t, 0, health["queue_length"], "Queue should be empty initially")
	assert.Equal(t, 100, health["queue_capacity"], "Capacity should match configured size")
	assert.Equal(t, 0.0, health["queue_utilization"], "Utilization should be 0%")
	assert.Equal(t, false, health["is_saturated"], "Should not be saturated")

	// Fill queue partially (50%)
	magic := [4]byte{0x01, 0x02, 0x03, 0x04}
	for i := 0; i < 50; i++ {
		msg := NewMessage(MsgPing, []byte{0x00}, magic)
		peer.SendMessageWithTimeout(msg, 10*time.Millisecond)
	}

	// Check health at 50% capacity
	health = peer.GetQueueHealth()
	assert.Equal(t, 50, health["queue_length"], "Queue should have 50 items")
	assert.Equal(t, 50.0, health["queue_utilization"], "Utilization should be 50%")
	assert.Equal(t, false, health["is_saturated"], "Should not be saturated at 50%")

	// Fill to 85% (saturated threshold is 80%)
	for i := 0; i < 35; i++ {
		msg := NewMessage(MsgPing, []byte{0x00}, magic)
		peer.SendMessageWithTimeout(msg, 10*time.Millisecond)
	}

	// Check health at 85% capacity
	health = peer.GetQueueHealth()
	assert.Equal(t, 85, health["queue_length"], "Queue should have 85 items")
	assert.Equal(t, 85.0, health["queue_utilization"], "Utilization should be 85%")
	assert.Equal(t, true, health["is_saturated"], "Should be saturated at 85%")
}

// BenchmarkWriteQueueThroughput measures queue throughput
func BenchmarkWriteQueueThroughput(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	peer := NewPeerWithQueueSize(server, true, [4]byte{0x01, 0x02, 0x03, 0x04}, logger, 2000)
	peer.connected.Store(true)

	magic := [4]byte{0x01, 0x02, 0x03, 0x04}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msg := NewMessage(MsgPing, []byte{0x00}, magic)
		peer.SendMessage(msg) // Ignore errors for benchmark
	}
}
