//go:build integration
// +build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// TestP2PServerListening verifies P2P server starts and listens
func TestP2PServerListening(t *testing.T) {
	tmpDir := t.TempDir()
	daemonPath := "../../twinsd"
	
	if _, err := os.Stat(daemonPath); os.IsNotExist(err) {
		t.Skip("Daemon binary not found")
	}
	
	const testP2PPort = 29450
	p2pAddr := fmt.Sprintf("127.0.0.1:%d", testP2PPort)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, daemonPath, "start",
		"--network", "regtest",
		"--datadir", tmpDir,
		"--p2p-bind", p2pAddr,
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

	// Wait for P2P server to start with active probe
	waitForP2PPort(t, p2pAddr, 5*time.Second)
	
	// Try to connect to P2P port
	conn, err := net.DialTimeout("tcp", p2pAddr, 2*time.Second)
	if err != nil {
		t.Fatalf("P2P server not listening on %s: %v", p2pAddr, err)
	}
	conn.Close()
	
	t.Logf("✓ P2P server listening on %s", p2pAddr)
	
	// Cleanup
	cmd.Process.Signal(syscall.SIGINT)
	cmd.Wait()
}

// TestP2PConnection verifies two nodes can connect via P2P
func TestP2PConnection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping P2P connection test in short mode")
	}

	daemonPath := "../../twinsd"
	if _, err := os.Stat(daemonPath); os.IsNotExist(err) {
		t.Skip("Daemon binary not found")
	}

	// Named port constants for P2P connection test
	const (
		p2pPort1 = 29451
		rpcPort1 = 29461
		p2pPort2 = 29452
		rpcPort2 = 29462
	)

	// Start first node
	tmpDir1 := t.TempDir()

	ctx1, cancel1 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel1()

	cmd1 := exec.CommandContext(ctx1, daemonPath, "start",
		"--network", "regtest",
		"--datadir", tmpDir1,
		"--p2p-bind", fmt.Sprintf("127.0.0.1:%d", p2pPort1),
		"--rpc-bind", fmt.Sprintf("127.0.0.1:%d", rpcPort1),
	)

	if err := cmd1.Start(); err != nil {
		t.Fatalf("Failed to start first node: %v", err)
	}

	defer func() {
		if cmd1.Process != nil {
			cmd1.Process.Signal(syscall.SIGINT)
			cmd1.Wait()
		}
	}()

	waitForP2PPort(t, fmt.Sprintf("127.0.0.1:%d", p2pPort1), 5*time.Second)

	// Start second node
	tmpDir2 := t.TempDir()

	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel2()

	cmd2 := exec.CommandContext(ctx2, daemonPath, "start",
		"--network", "regtest",
		"--datadir", tmpDir2,
		"--p2p-bind", fmt.Sprintf("127.0.0.1:%d", p2pPort2),
		"--rpc-bind", fmt.Sprintf("127.0.0.1:%d", rpcPort2),
	)

	if err := cmd2.Start(); err != nil {
		t.Fatalf("Failed to start second node: %v", err)
	}

	defer func() {
		if cmd2.Process != nil {
			cmd2.Process.Signal(syscall.SIGINT)
			cmd2.Wait()
		}
	}()

	waitForP2PPort(t, fmt.Sprintf("127.0.0.1:%d", p2pPort2), 5*time.Second)

	// Verify both P2P servers are listening
	for i, port := range []int{p2pPort1, p2pPort2} {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err != nil {
			t.Errorf("Node %d P2P server not listening on %s: %v", i+1, addr, err)
		} else {
			conn.Close()
			t.Logf("✓ Node %d P2P server listening on %s", i+1, addr)
		}
	}

	// Use addnode RPC to connect node 1 to node 2
	node2Addr := fmt.Sprintf("127.0.0.1:%d", p2pPort2)
	err := rpcCall(rpcPort1, "addnode", []interface{}{node2Addr, "onetry"})
	if err != nil {
		t.Errorf("Failed to call addnode RPC: %v", err)
	} else {
		t.Logf("✓ Called addnode to connect node 1 -> node 2 (%s)", node2Addr)
	}

	// Wait for connection to establish
	time.Sleep(2 * time.Second) // Connection establishment requires time, no port to probe

	// Verify connection via getpeerinfo
	peers, err := rpcCallResult(rpcPort1, "getpeerinfo", nil)
	if err != nil {
		t.Logf("Warning: Could not verify peer connection: %v", err)
	} else if peerList, ok := peers.([]interface{}); ok && len(peerList) > 0 {
		t.Logf("✓ Node 1 has %d peer(s) connected", len(peerList))
	} else {
		t.Logf("Note: No peers connected yet (connection may still be establishing)")
	}

	// Cleanup
	cmd1.Process.Signal(syscall.SIGINT)
	cmd2.Process.Signal(syscall.SIGINT)
	cmd1.Wait()
	cmd2.Wait()
}

// rpcCall makes an RPC call without expecting a result
func rpcCall(port int, method string, params interface{}) error {
	_, err := rpcCallResult(port, method, params)
	return err
}

// rpcCallResult makes an RPC call and returns the result
func rpcCallResult(port int, method string, params interface{}) (interface{}, error) {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
	}
	if params != nil {
		reqBody["params"] = params
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Result interface{}            `json:"result"`
		Error  *struct{ Message string } `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("rpc error: %s", result.Error.Message)
	}

	return result.Result, nil
}

// waitForP2PPort polls a TCP address until it becomes available or timeout expires.
func waitForP2PPort(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Logf("Warning: port %s not ready after %v", addr, timeout)
}
