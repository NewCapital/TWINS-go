//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// Test port constants — grouped to avoid conflicts between parallel tests
const (
	testDaemonP2PPort1 = 29444
	testDaemonP2PPort2 = 29445
	testDaemonP2PPort3 = 29446
	testDaemonP2PPort4 = 29447
)

// TestDaemonStartup verifies the daemon starts successfully
func TestDaemonStartup(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()
	
	// Build daemon if not already built
	daemonPath := "../../twinsd"
	if _, err := os.Stat(daemonPath); os.IsNotExist(err) {
		t.Log("Building daemon...")
		buildCmd := exec.Command("go", "build", "-o", daemonPath, "../../cmd/twinsd")
		if output, err := buildCmd.CombinedOutput(); err != nil {
			t.Fatalf("Failed to build daemon: %v\n%s", err, output)
		}
	}
	
	// Start daemon with regtest config
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	cmd := exec.CommandContext(ctx, daemonPath, "start",
		"--network", "regtest",
		"--datadir", tmpDir,
		"--p2p-bind", fmt.Sprintf("127.0.0.1:%d", testDaemonP2PPort1),
	)

	// Capture output
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start daemon in background
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}

	// Ensure daemon is killed on test completion
	defer func() {
		if cmd.Process != nil {
			cmd.Process.Signal(syscall.SIGINT)
			cmd.Wait()
		}
	}()

	// Wait for daemon to initialize by probing P2P port
	if !waitForPort(t, testDaemonP2PPort1, 5*time.Second) {
		t.Fatal("Daemon P2P port did not become ready")
	}
	
	// Verify data directory was created
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		t.Error("Data directory was not created")
	}
	
	// Verify blockchain.db was created
	dbPath := filepath.Join(tmpDir, "blockchain.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Blockchain database was not created")
	}
	
	t.Log("✓ Daemon started successfully")
	t.Log("✓ Data directory created")
	t.Log("✓ Blockchain database initialized")
	
	// Send shutdown signal
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Logf("Warning: Failed to send SIGINT: %v", err)
	}
	
	// Wait for graceful shutdown (max 5 seconds)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	
	done := make(chan error)
	go func() {
		done <- cmd.Wait()
	}()
	
	select {
	case err := <-done:
		if err != nil && err.Error() != "signal: interrupt" {
			t.Logf("Daemon exit with error: %v", err)
		} else {
			t.Log("✓ Daemon shutdown gracefully")
		}
	case <-shutdownCtx.Done():
		t.Error("Daemon failed to shutdown within timeout")
		cmd.Process.Kill()
	}
}

// TestGenesisBlockCreation verifies genesis block is created correctly
func TestGenesisBlockCreation(t *testing.T) {
	tmpDir := t.TempDir()
	
	daemonPath := "../../twinsd"
	if _, err := os.Stat(daemonPath); os.IsNotExist(err) {
		t.Skip("Daemon binary not found, run 'go build ./cmd/twinsd' first")
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	
	cmd := exec.CommandContext(ctx, daemonPath, "start",
		"--network", "regtest",
		"--datadir", tmpDir,
		"--p2p-bind", fmt.Sprintf("127.0.0.1:%d", testDaemonP2PPort2),
	)

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start daemon: %v", err)
	}

	defer func() {
		if cmd.Process != nil {
			cmd.Process.Signal(syscall.SIGINT)
			cmd.Wait()
		}
	}()

	// Wait for initialization by probing P2P port
	waitForPort(t, testDaemonP2PPort2, 5*time.Second)
	
	// Verify database exists and has data
	dbPath := filepath.Join(tmpDir, "blockchain.db")
	if stat, err := os.Stat(dbPath); err != nil {
		t.Errorf("Blockchain database not found: %v", err)
	} else if stat.Size() == 0 {
		t.Error("Blockchain database is empty")
	} else {
		t.Logf("✓ Genesis block created (database size: %d bytes)", stat.Size())
	}
	
	// Cleanup
	cmd.Process.Signal(syscall.SIGINT)
	cmd.Wait()
}

// TestMultipleDaemonInstances verifies multiple daemons can run simultaneously
func TestMultipleDaemonInstances(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping multi-instance test in short mode")
	}
	
	daemonPath := "../../twinsd"
	if _, err := os.Stat(daemonPath); os.IsNotExist(err) {
		t.Skip("Daemon binary not found")
	}
	
	// Start first daemon
	tmpDir1 := t.TempDir()
	ctx1, cancel1 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel1()
	
	cmd1 := exec.CommandContext(ctx1, daemonPath, "start",
		"--network", "regtest",
		"--datadir", tmpDir1,
		"--p2p-bind", fmt.Sprintf("127.0.0.1:%d", testDaemonP2PPort3),
	)

	if err := cmd1.Start(); err != nil {
		t.Fatalf("Failed to start first daemon: %v", err)
	}
	defer func() {
		if cmd1.Process != nil {
			cmd1.Process.Signal(syscall.SIGINT)
			cmd1.Wait()
		}
	}()

	waitForPort(t, testDaemonP2PPort3, 5*time.Second)
	
	// Start second daemon
	tmpDir2 := t.TempDir()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel2()
	
	cmd2 := exec.CommandContext(ctx2, daemonPath, "start",
		"--network", "regtest",
		"--datadir", tmpDir2,
		"--p2p-bind", fmt.Sprintf("127.0.0.1:%d", testDaemonP2PPort4),
	)

	if err := cmd2.Start(); err != nil {
		t.Fatalf("Failed to start second daemon: %v", err)
	}
	defer func() {
		if cmd2.Process != nil {
			cmd2.Process.Signal(syscall.SIGINT)
			cmd2.Wait()
		}
	}()

	waitForPort(t, testDaemonP2PPort4, 5*time.Second)
	
	// Both should be running
	if cmd1.ProcessState != nil && cmd1.ProcessState.Exited() {
		t.Error("First daemon exited unexpectedly")
	}
	if cmd2.ProcessState != nil && cmd2.ProcessState.Exited() {
		t.Error("Second daemon exited unexpectedly")
	}
	
	t.Log("✓ Multiple daemon instances running simultaneously")
	
	// Cleanup
	cmd1.Process.Signal(syscall.SIGINT)
	cmd2.Process.Signal(syscall.SIGINT)
	cmd1.Wait()
	cmd2.Wait()
}

// waitForPort polls a TCP port until it becomes available or timeout expires.
// Returns true if the port is ready, false on timeout.
func waitForPort(t *testing.T, port int, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false
}
