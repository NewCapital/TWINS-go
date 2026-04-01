package rpc

import (
	"net/http"
	"sync/atomic"

	"github.com/sirupsen/logrus"
)

// ConnectionLimiter limits the number of concurrent RPC connections.
// Thread-safe using atomic operations.
type ConnectionLimiter struct {
	maxConnections int32
	current        atomic.Int32
	totalAccepted  atomic.Uint64
	totalRejected  atomic.Uint64
	logger         *logrus.Entry
}

// NewConnectionLimiter creates a new connection limiter.
// If maxConnections is 0 or negative, no limit is enforced.
func NewConnectionLimiter(maxConnections int, logger *logrus.Entry) *ConnectionLimiter {
	return &ConnectionLimiter{
		maxConnections: int32(maxConnections),
		logger:         logger,
	}
}

// Acquire attempts to acquire a connection slot.
// Returns true if successful, false if the limit has been reached.
func (l *ConnectionLimiter) Acquire() bool {
	// No limit if maxConnections is 0 or negative
	if l.maxConnections <= 0 {
		l.current.Add(1)
		return true
	}

	for {
		current := l.current.Load()
		if current >= l.maxConnections {
			return false
		}
		if l.current.CompareAndSwap(current, current+1) {
			return true
		}
		// CAS failed, retry
	}
}

// Release releases a connection slot.
func (l *ConnectionLimiter) Release() {
	l.current.Add(-1)
}

// Current returns the current number of active connections.
func (l *ConnectionLimiter) Current() int {
	return int(l.current.Load())
}

// Max returns the maximum allowed connections.
func (l *ConnectionLimiter) Max() int {
	return int(l.maxConnections)
}

// Middleware returns an HTTP middleware that limits concurrent connections.
// Rejected requests receive HTTP 503 Service Unavailable.
func (l *ConnectionLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.Acquire() {
			l.totalRejected.Add(1)
			if l.logger != nil {
				l.logger.WithFields(logrus.Fields{
					"current": l.Current(),
					"max":     l.Max(),
				}).Warn("RPC connection rejected: max connections reached")
			}
			http.Error(w, "Service Unavailable: too many connections", http.StatusServiceUnavailable)
			return
		}
		l.totalAccepted.Add(1)
		defer l.Release()

		next.ServeHTTP(w, r)
	})
}

// Stats returns connection limiter statistics.
func (l *ConnectionLimiter) Stats() (current, max int, accepted, rejected uint64) {
	return l.Current(), l.Max(), l.totalAccepted.Load(), l.totalRejected.Load()
}
