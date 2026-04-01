package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/urfave/cli/v2"
)

// RPCClient represents a JSON-RPC client
type RPCClient struct {
	host     string
	port     int
	user     string
	password string
	timeout  time.Duration
	useTLS   bool
	client   *http.Client
}

// RPCRequest represents a JSON-RPC request
type RPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

// RPCResponse represents a JSON-RPC response
type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
	ID      int             `json:"id"`
}

// RPCError represents a JSON-RPC error
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NewRPCClient creates a new RPC client from CLI context
func NewRPCClient(c *cli.Context) *RPCClient {
	return &RPCClient{
		host:     c.String("rpc-host"),
		port:     c.Int("rpc-port"),
		user:     c.String("rpc-user"),
		password: c.String("rpc-password"),
		timeout:  c.Duration("rpc-timeout") * time.Second,
		useTLS:   c.Bool("rpc-tls"),
		client: &http.Client{
			Timeout: c.Duration("rpc-timeout") * time.Second,
		},
	}
}

// Call makes an RPC call and returns the result
func (r *RPCClient) Call(method string, params ...interface{}) (json.RawMessage, error) {
	// Build request
	req := RPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Build URL
	scheme := "http"
	if r.useTLS {
		scheme = "https"
	}
	url := fmt.Sprintf("%s://%s:%d", scheme, r.host, r.port)

	// Create HTTP request
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	if r.user != "" || r.password != "" {
		httpReq.SetBasicAuth(r.user, r.password)
	}

	// Make request
	resp, err := r.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("RPC call failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse response
	var rpcResp RPCResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for RPC error
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}

// CallWithResult makes an RPC call and unmarshals the result into v
func (r *RPCClient) CallWithResult(method string, result interface{}, params ...interface{}) error {
	rawResult, err := r.Call(method, params...)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(rawResult, result); err != nil {
		return fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return nil
}
