package cli

import (
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSignalHandler(t *testing.T) {
	sh := NewSignalHandler()
	assert.NotNil(t, sh)
	assert.NotNil(t, sh.shutdown)
	assert.NotNil(t, sh.done)
	assert.NotNil(t, sh.logger)
}

func TestSignalHandler_Start_Stop(t *testing.T) {
	sh := NewSignalHandler()
	sh.Start()

	// Stop the handler
	sh.Stop()

	// Wait should complete quickly
	completed := sh.WaitWithTimeout(1 * time.Second)
	assert.True(t, completed, "Signal handler should have completed within timeout")
}

func TestSignalHandler_ShutdownChannel(t *testing.T) {
	sh := NewSignalHandler()
	sh.Start()

	// Shutdown channel should be available
	shutdownCh := sh.Shutdown()
	assert.NotNil(t, shutdownCh)

	// Should not be closed initially
	select {
	case <-shutdownCh:
		t.Fatal("Shutdown channel should not be closed initially")
	default:
		// Expected
	}

	// Stop and verify shutdown channel is closed
	sh.Stop()

	select {
	case <-shutdownCh:
		// Expected - channel should be closed
	case <-time.After(1 * time.Second):
		t.Fatal("Shutdown channel should be closed after stop")
	}
}

func TestSignalHandler_WaitWithTimeout(t *testing.T) {
	sh := NewSignalHandler()
	sh.Start()

	// Should timeout if handler doesn't complete
	completed := sh.WaitWithTimeout(100 * time.Millisecond)
	assert.False(t, completed, "Should timeout waiting for handler")

	// Stop and should complete quickly
	sh.Stop()
	completed = sh.WaitWithTimeout(1 * time.Second)
	assert.True(t, completed, "Should complete after stop")
}

func TestCreateShutdownContext(t *testing.T) {
	// Test CreateShutdownContext function
	ctx, cancel := CreateShutdownContext()
	defer cancel()

	assert.NotNil(t, ctx)
	assert.NotNil(t, cancel)

	// Context should not be done initially
	select {
	case <-ctx.Done():
		t.Fatal("Context should not be done initially")
	default:
		// Expected
	}

	// Cancel should work
	cancel()
	select {
	case <-ctx.Done():
		// Expected
	case <-time.After(1 * time.Second):
		t.Fatal("Context should be done after cancel")
	}
}

func TestSetupGracefulShutdown(t *testing.T) {
	// Skip this test as SetupGracefulShutdown calls os.Exit(0)
	// which terminates the test process. This is expected behavior
	// for the actual application, but not suitable for unit tests.
	t.Skip("SetupGracefulShutdown calls os.Exit(0), skipping to prevent test termination")
}

// Test signal handling integration
func TestSignalHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test is more complex because it involves actual signal handling
	// We'll create a signal handler and send it a signal programmatically

	sh := NewSignalHandler()
	sh.Start()

	// Give the handler time to start
	time.Sleep(10 * time.Millisecond)

	// Send a signal to ourselves (this is a bit tricky to test reliably)
	pid := os.Getpid()
	process, err := os.FindProcess(pid)
	require.NoError(t, err)

	// Send SIGTERM to trigger shutdown
	err = process.Signal(syscall.SIGTERM)
	require.NoError(t, err)

	// Wait for shutdown to be triggered
	select {
	case <-sh.Shutdown():
		// Expected - shutdown was triggered
	case <-time.After(1 * time.Second):
		t.Fatal("Shutdown should have been triggered by signal")
	}

	// Stop the handler to clean up the goroutine
	sh.Stop()

	// Wait for handler to complete
	completed := sh.WaitWithTimeout(1 * time.Second)
	assert.True(t, completed, "Handler should complete after signal")
}

// Benchmark signal handler operations
func BenchmarkNewSignalHandler(b *testing.B) {
	for i := 0; i < b.N; i++ {
		sh := NewSignalHandler()
		sh.Stop() // Clean up
	}
}

func BenchmarkSignalHandlerStartStop(b *testing.B) {
	for i := 0; i < b.N; i++ {
		sh := NewSignalHandler()
		sh.Start()
		sh.Stop()
		sh.WaitWithTimeout(100 * time.Millisecond)
	}
}

func BenchmarkCreateShutdownContext(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ctx, cancel := CreateShutdownContext()
		cancel()
		<-ctx.Done()
	}
}

// Test edge cases
func TestSignalHandler_MultipleStops(t *testing.T) {
	sh := NewSignalHandler()
	sh.Start()

	// Multiple stops should not panic or cause issues
	sh.Stop()
	sh.Stop()
	sh.Stop()

	completed := sh.WaitWithTimeout(1 * time.Second)
	assert.True(t, completed)
}

func TestSignalHandler_StopBeforeStart(t *testing.T) {
	sh := NewSignalHandler()

	// Stop before start should not panic
	sh.Stop()

	// Start after stop should still work
	sh.Start()
	sh.Stop()

	completed := sh.WaitWithTimeout(1 * time.Second)
	assert.True(t, completed)
}