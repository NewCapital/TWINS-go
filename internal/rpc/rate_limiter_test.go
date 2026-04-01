package rpc

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestRateLimiterAllow(t *testing.T) {
	rl := NewRateLimiter(5, logrus.NewEntry(logrus.New()))

	// First 5 requests should be allowed
	for i := 0; i < 5; i++ {
		if !rl.Allow("192.168.1.1") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	// 6th request should be rejected
	if rl.Allow("192.168.1.1") {
		t.Fatal("6th request should be rejected")
	}
}

func TestRateLimiterPerIP(t *testing.T) {
	rl := NewRateLimiter(2, logrus.NewEntry(logrus.New()))

	// 2 requests from IP A
	if !rl.Allow("10.0.0.1") {
		t.Fatal("first request from IP A should be allowed")
	}
	if !rl.Allow("10.0.0.1") {
		t.Fatal("second request from IP A should be allowed")
	}

	// IP A exhausted
	if rl.Allow("10.0.0.1") {
		t.Fatal("third request from IP A should be rejected")
	}

	// IP B should still work independently
	if !rl.Allow("10.0.0.2") {
		t.Fatal("first request from IP B should be allowed")
	}
	if !rl.Allow("10.0.0.2") {
		t.Fatal("second request from IP B should be allowed")
	}
}

func TestRateLimiterDisabled(t *testing.T) {
	rl := NewRateLimiter(0, logrus.NewEntry(logrus.New()))

	// All requests should be allowed when disabled
	for i := 0; i < 100; i++ {
		if !rl.Allow("192.168.1.1") {
			t.Fatalf("request %d should be allowed when rate limiting is disabled", i+1)
		}
	}
}

func TestRateLimiterNegativeDisabled(t *testing.T) {
	rl := NewRateLimiter(-1, logrus.NewEntry(logrus.New()))

	for i := 0; i < 10; i++ {
		if !rl.Allow("192.168.1.1") {
			t.Fatalf("request %d should be allowed when rate limiting is disabled", i+1)
		}
	}
}

func TestRateLimiterWindowExpiry(t *testing.T) {
	rl := NewRateLimiter(2, logrus.NewEntry(logrus.New()))

	// Manually inject old timestamps to simulate time passing
	rl.mu.Lock()
	cw := &clientWindow{
		timestamps: []time.Time{
			time.Now().Add(-2 * time.Minute), // Expired
			time.Now().Add(-2 * time.Minute), // Expired
		},
	}
	rl.clients["192.168.1.1"] = cw
	rl.mu.Unlock()

	// Old timestamps should be expired, so new requests should be allowed
	if !rl.Allow("192.168.1.1") {
		t.Fatal("request should be allowed after window expires")
	}
	if !rl.Allow("192.168.1.1") {
		t.Fatal("second request should be allowed after window expires")
	}

	// Now exhausted again
	if rl.Allow("192.168.1.1") {
		t.Fatal("third request should be rejected")
	}
}

func TestRateLimiterMiddleware(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
	rl := NewRateLimiter(2, logger)

	handlerCalled := 0
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled++
		w.WriteHeader(http.StatusOK)
	})

	handler := rl.Middleware(inner)

	// First 2 requests succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	// 3rd request should get 429
	req := httptest.NewRequest("POST", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}

	if handlerCalled != 2 {
		t.Fatalf("expected inner handler called 2 times, got %d", handlerCalled)
	}
}

func TestRateLimiterMiddlewareDisabled(t *testing.T) {
	rl := NewRateLimiter(0, logrus.NewEntry(logrus.New()))

	handlerCalled := 0
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled++
		w.WriteHeader(http.StatusOK)
	})

	handler := rl.Middleware(inner)

	// All requests should pass through when disabled
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("POST", "/", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	if handlerCalled != 10 {
		t.Fatalf("expected 10 calls, got %d", handlerCalled)
	}
}

func TestRateLimiterStats(t *testing.T) {
	rl := NewRateLimiter(2, logrus.NewEntry(logrus.New()))

	rl.Allow("10.0.0.1") // allowed - but Allow doesn't track stats

	// Use middleware to test stats
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := rl.Middleware(inner)

	// Reset with fresh limiter for stats test
	rl2 := NewRateLimiter(2, logrus.NewEntry(logrus.New()))
	handler2 := rl2.Middleware(inner)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		w := httptest.NewRecorder()
		handler2.ServeHTTP(w, req)
	}

	allowed, rejected := rl2.Stats()
	if allowed != 2 {
		t.Fatalf("expected 2 allowed, got %d", allowed)
	}
	if rejected != 1 {
		t.Fatalf("expected 1 rejected, got %d", rejected)
	}

	// Suppress unused variable warnings
	_ = handler
}

func TestRateLimiterCleanup(t *testing.T) {
	rl := NewRateLimiter(10, logrus.NewEntry(logrus.New()))

	// Add entries with expired timestamps
	rl.mu.Lock()
	rl.clients["expired-ip"] = &clientWindow{
		timestamps: []time.Time{time.Now().Add(-2 * time.Minute)},
	}
	rl.clients["active-ip"] = &clientWindow{
		timestamps: []time.Time{time.Now()},
	}
	rl.mu.Unlock()

	rl.cleanup()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	if _, exists := rl.clients["expired-ip"]; exists {
		t.Fatal("expired-ip should have been cleaned up")
	}
	if _, exists := rl.clients["active-ip"]; !exists {
		t.Fatal("active-ip should still exist")
	}
}

func TestExtractIP(t *testing.T) {
	tests := []struct {
		remoteAddr string
		expected   string
	}{
		{"192.168.1.1:12345", "192.168.1.1"},
		{"10.0.0.1:80", "10.0.0.1"},
		{"[::1]:8080", "::1"},
		{"invalid-no-port", "invalid-no-port"},
	}

	for _, tt := range tests {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tt.remoteAddr
		got := extractIP(r)
		if got != tt.expected {
			t.Errorf("extractIP(%q) = %q, want %q", tt.remoteAddr, got, tt.expected)
		}
	}
}
