//go:build integration
// +build integration

// Copyright (c) 2025 The TWINS Core developers
// Distributed under the MIT software license

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// NodeType represents the node implementation type
type NodeType string

const (
	NodeTypeGo  NodeType = "go"
	NodeTypeCpp NodeType = "cpp"
)

// TestNode represents a running node instance for testing
type TestNode struct {
	Type       NodeType
	BinaryPath string
	DataDir    string
	RPCPort    int
	P2PPort    int
	RPCUser    string
	RPCPass    string
	Process    *exec.Cmd
	T          *testing.T
}

// NodeConfig contains configuration for starting a test node
type NodeConfig struct {
	Type        NodeType
	Network     string // mainnet, testnet, regtest
	RPCPort     int
	P2PPort     int
	ExtraArgs   []string
	RPCUser     string
	RPCPass     string
}

// DefaultGoConfig returns default configuration for Go node
func DefaultGoConfig() NodeConfig {
	return NodeConfig{
		Type:    NodeTypeGo,
		Network: "regtest",
		RPCPort: 18444,
		P2PPort: 18445,
		RPCUser: "test",
		RPCPass: "test",
	}
}

// DefaultCppConfig returns default configuration for C++ node
func DefaultCppConfig() NodeConfig {
	return NodeConfig{
		Type:    NodeTypeCpp,
		Network: "regtest",
		RPCPort: 18544,
		P2PPort: 18545,
		RPCUser: "test",
		RPCPass: "test",
	}
}

// StartNode starts a test node with the given configuration
func StartNode(t *testing.T, config NodeConfig) *TestNode {
	t.Helper()

	// Get binary path from environment or default
	binaryPath := getBinaryPath(config.Type)
	if binaryPath == "" {
		t.Skipf("Binary for %s node not found, set TWINS_%s_BIN environment variable",
			config.Type, string(config.Type))
	}

	// Create temporary data directory
	dataDir := t.TempDir()

	node := &TestNode{
		Type:       config.Type,
		BinaryPath: binaryPath,
		DataDir:    dataDir,
		RPCPort:    config.RPCPort,
		P2PPort:    config.P2PPort,
		RPCUser:    config.RPCUser,
		RPCPass:    config.RPCPass,
		T:          t,
	}

	// Build command arguments
	args := buildNodeArgs(node, config)

	// Start node process
	cmd := exec.Command(binaryPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start %s node: %v", config.Type, err)
	}

	node.Process = cmd

	// Wait for node to be ready
	if !node.WaitForReady(30 * time.Second) {
		node.Stop()
		t.Fatalf("Node %s did not become ready in time", config.Type)
	}

	t.Logf("Started %s node on RPC port %d", config.Type, config.RPCPort)

	return node
}

// getBinaryPath gets the binary path from environment or default location
func getBinaryPath(nodeType NodeType) string {
	switch nodeType {
	case NodeTypeGo:
		if path := os.Getenv("TWINS_GO_BIN"); path != "" {
			return path
		}
		return "./twinsd"
	case NodeTypeCpp:
		if path := os.Getenv("TWINS_CPP_BIN"); path != "" {
			return path
		}
		return "../legacy/src/twinsd"
	default:
		return ""
	}
}

// buildNodeArgs builds command line arguments for starting a node
func buildNodeArgs(node *TestNode, config NodeConfig) []string {
	args := []string{
		"-datadir=" + node.DataDir,
		"-" + config.Network,
		"-rpcport=" + fmt.Sprintf("%d", node.RPCPort),
		"-port=" + fmt.Sprintf("%d", node.P2PPort),
		"-rpcuser=" + node.RPCUser,
		"-rpcpassword=" + node.RPCPass,
		"-server",
		"-listen",
		"-discover=0",
		"-debug",
	}

	args = append(args, config.ExtraArgs...)

	return args
}

// Stop stops the test node gracefully with SIGINT, falling back to SIGKILL.
// Matches CLAUDE.md guidance: use SIGTERM/SIGINT for clean shutdown.
func (n *TestNode) Stop() {
	if n.Process == nil || n.Process.Process == nil {
		return
	}
	// Try graceful shutdown first
	n.Process.Process.Signal(syscall.SIGINT)
	done := make(chan error, 1)
	go func() { done <- n.Process.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		// Escalate to SIGKILL if graceful shutdown times out
		n.Process.Process.Kill()
		<-done
	}
}

// WaitForReady waits for the node to be ready to accept RPC requests
func (n *TestNode) WaitForReady(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		_, err := n.CallRPC("getblockcount", nil)
		if err == nil {
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}

	return false
}

// CallRPC makes an RPC call to the node
func (n *TestNode) CallRPC(method string, params interface{}) (interface{}, error) {
	url := fmt.Sprintf("http://localhost:%d", n.RPCPort)

	requestBody := map[string]interface{}{
		"jsonrpc": "1.0",
		"id":      "test",
		"method":  method,
		"params":  params,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(n.RPCUser, n.RPCPass)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var rpcResp struct {
		Result interface{}            `json:"result"`
		Error  map[string]interface{} `json:"error"`
		ID     string                 `json:"id"`
	}

	if err := json.Unmarshal(respBytes, &rpcResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error: %v", rpcResp.Error)
	}

	return rpcResp.Result, nil
}

// CompareRPCResults compares two RPC results for equality
func CompareRPCResults(t *testing.T, result1, result2 interface{}, ignoreFields []string) bool {
	t.Helper()

	// Convert to JSON for comparison
	json1, err1 := json.Marshal(result1)
	json2, err2 := json.Marshal(result2)

	if err1 != nil || err2 != nil {
		t.Errorf("Failed to marshal results for comparison")
		return false
	}

	// Parse into maps for field-by-field comparison
	var map1, map2 map[string]interface{}
	if err := json.Unmarshal(json1, &map1); err != nil {
		t.Errorf("Failed to unmarshal first result as object: %v", err)
		return false
	}
	if err := json.Unmarshal(json2, &map2); err != nil {
		t.Errorf("Failed to unmarshal second result as object: %v", err)
		return false
	}

	return compareMaps(t, map1, map2, ignoreFields)
}

// compareMaps compares two maps, ignoring specified fields
func compareMaps(t *testing.T, m1, m2 map[string]interface{}, ignoreFields []string) bool {
	t.Helper()

	// Create ignore map for fast lookup
	ignore := make(map[string]bool)
	for _, field := range ignoreFields {
		ignore[field] = true
	}

	// Check all keys in map1
	for key, val1 := range m1 {
		if ignore[key] {
			continue
		}

		val2, exists := m2[key]
		if !exists {
			t.Errorf("Field %s missing in second result", key)
			return false
		}

		if !compareValues(val1, val2) {
			t.Errorf("Field %s differs: %v vs %v", key, val1, val2)
			return false
		}
	}

	// Check for extra keys in map2
	for key := range m2 {
		if ignore[key] {
			continue
		}
		if _, exists := m1[key]; !exists {
			t.Errorf("Extra field %s in second result", key)
			return false
		}
	}

	return true
}

// compareValues compares two values with type-aware logic
func compareValues(v1, v2 interface{}) bool {
	// Handle nil values
	if v1 == nil && v2 == nil {
		return true
	}
	if v1 == nil || v2 == nil {
		return false
	}

	// Compare based on type
	switch val1 := v1.(type) {
	case float64:
		val2, ok := v2.(float64)
		if !ok {
			return false
		}
		// Allow small floating point differences
		return math.Abs(val1-val2) < 0.0001
	case string:
		val2, ok := v2.(string)
		return ok && val1 == val2
	case bool:
		val2, ok := v2.(bool)
		return ok && val1 == val2
	case []interface{}:
		val2, ok := v2.([]interface{})
		if !ok || len(val1) != len(val2) {
			return false
		}
		for i := range val1 {
			if !compareValues(val1[i], val2[i]) {
				return false
			}
		}
		return true
	case map[string]interface{}:
		val2, ok := v2.(map[string]interface{})
		if !ok {
			return false
		}
		for key, subVal1 := range val1 {
			subVal2, exists := val2[key]
			if !exists || !compareValues(subVal1, subVal2) {
				return false
			}
		}
		return true
	default:
		return fmt.Sprintf("%v", v1) == fmt.Sprintf("%v", v2)
	}
}


// SaveTestArtifacts saves test artifacts for debugging
func SaveTestArtifacts(t *testing.T, node *TestNode) {
	t.Helper()

	artifactDir := os.Getenv("TWINS_TEST_ARTIFACTS")
	if artifactDir == "" {
		return
	}

	// Create artifact directory
	testDir := filepath.Join(artifactDir, t.Name())
	os.MkdirAll(testDir, 0755)

	// Copy node logs
	logFiles := []string{"debug.log", "error.log"}
	for _, logFile := range logFiles {
		src := filepath.Join(node.DataDir, logFile)
		dst := filepath.Join(testDir, fmt.Sprintf("%s-%s", node.Type, logFile))

		if data, err := os.ReadFile(src); err == nil {
			os.WriteFile(dst, data, 0644)
		}
	}

	t.Logf("Test artifacts saved to: %s", testDir)
}