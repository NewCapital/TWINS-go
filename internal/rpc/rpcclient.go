package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client represents an RPC client for connecting to twinsd
type Client struct {
	address  string
	username string
	password string
	client   *http.Client
}

// NewClient creates a new RPC client
func NewClient(address, username, password string) *Client {
	return &Client{
		address:  address,
		username: username,
		password: password,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Call makes an RPC call to the daemon
func (c *Client) Call(ctx context.Context, method string, params interface{}, result interface{}) error {
	// Convert params to json.RawMessage
	var paramData json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("failed to marshal params: %w", err)
		}
		paramData = data
	}

	// Create request
	req := Request{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramData,
		ID:      1,
	}

	// Marshal request
	reqData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", "http://"+c.address, bytes.NewReader(reqData))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	if c.username != "" && c.password != "" {
		httpReq.SetBasicAuth(c.username, c.password)
	}

	// Send request
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("RPC connection failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Parse response
	var rpcResp Response
	if err := json.Unmarshal(respData, &rpcResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for RPC error
	if rpcResp.Error != nil {
		return rpcResp.Error
	}

	// Unmarshal result if provided
	if result != nil && rpcResp.Result != nil {
		// First marshal the result interface{} to JSON
		resultData, err := json.Marshal(rpcResp.Result)
		if err != nil {
			return fmt.Errorf("failed to marshal intermediate result: %w", err)
		}
		// Then unmarshal into the target
		if err := json.Unmarshal(resultData, result); err != nil {
			return fmt.Errorf("failed to unmarshal result: %w", err)
		}
	}

	return nil
}

// Close closes the client (no-op for now, but allows future cleanup)
func (c *Client) Close() error {
	// Future: could close persistent connections, cancel contexts, etc.
	return nil
}