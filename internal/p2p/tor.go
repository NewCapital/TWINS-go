package p2p

import (
	"bufio"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// TorController manages Tor hidden service via control protocol
type TorController struct {
	mu           sync.Mutex
	controlAddr  string // Tor control port address (default: 127.0.0.1:9051)
	password     string // Control password (legacy: -torpassword)
	conn         net.Conn
	reader       *bufio.Reader
	logger       *logrus.Entry
	onionAddress string // Created .onion address
	servicePort  int    // Local service port
	enabled      bool
}

// NewTorController creates a new Tor controller
func NewTorController(controlAddr, password string, servicePort int, logger *logrus.Logger) *TorController {
	if controlAddr == "" {
		controlAddr = "127.0.0.1:9051"
	}
	return &TorController{
		controlAddr: controlAddr,
		password:    password,
		servicePort: servicePort,
		logger:      logger.WithField("component", "tor"),
	}
}

// Start connects to Tor and creates a hidden service
func (t *TorController) Start() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.logger.WithField("control_addr", t.controlAddr).Debug("Connecting to Tor control port")

	// Connect to Tor control port
	conn, err := net.DialTimeout("tcp", t.controlAddr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to Tor control port: %w", err)
	}
	t.conn = conn
	t.reader = bufio.NewReader(conn)

	// Authenticate
	if err := t.authenticate(); err != nil {
		t.conn.Close()
		return fmt.Errorf("Tor authentication failed: %w", err)
	}

	// Create hidden service
	onionAddr, err := t.createHiddenService()
	if err != nil {
		t.conn.Close()
		return fmt.Errorf("failed to create hidden service: %w", err)
	}

	t.onionAddress = onionAddr
	t.enabled = true

	t.logger.WithField("onion_address", onionAddr).Info("Tor hidden service created")
	return nil
}

// Stop removes the hidden service and closes connection
func (t *TorController) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.enabled {
		return
	}

	// Try to remove hidden service
	if t.onionAddress != "" {
		t.sendCommand(fmt.Sprintf("DEL_ONION %s", strings.TrimSuffix(t.onionAddress, ".onion")))
	}

	if t.conn != nil {
		t.conn.Close()
	}

	t.enabled = false
	t.logger.Info("Tor hidden service stopped")
}

// GetOnionAddress returns the .onion address
func (t *TorController) GetOnionAddress() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.onionAddress
}

// IsEnabled returns whether Tor hidden service is active
func (t *TorController) IsEnabled() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.enabled
}

// authenticate performs Tor control authentication
func (t *TorController) authenticate() error {
	// First, get protocol info to determine auth methods
	resp, err := t.sendCommand("PROTOCOLINFO 1")
	if err != nil {
		return err
	}

	// Parse auth methods from response
	authMethods := t.parseAuthMethods(resp)

	// Try authentication methods in order
	if t.password != "" && contains(authMethods, "HASHEDPASSWORD") {
		return t.authPassword()
	}

	if contains(authMethods, "COOKIE") || contains(authMethods, "SAFECOOKIE") {
		cookiePath := t.parseCookiePath(resp)
		if cookiePath != "" {
			if contains(authMethods, "SAFECOOKIE") {
				return t.authSafeCookie(cookiePath)
			}
			return t.authCookie(cookiePath)
		}
	}

	if contains(authMethods, "NULL") {
		return t.authNull()
	}

	return errors.New("no supported authentication method")
}

// authNull performs NULL authentication (no auth required)
func (t *TorController) authNull() error {
	resp, err := t.sendCommand("AUTHENTICATE")
	if err != nil {
		return err
	}
	if !strings.HasPrefix(resp, "250") {
		return fmt.Errorf("auth failed: %s", resp)
	}
	return nil
}

// authPassword performs password authentication
// Uses hex encoding to prevent command injection vulnerabilities
func (t *TorController) authPassword() error {
	// Encode password as hex to prevent command injection (quotes, special chars)
	hexPassword := hex.EncodeToString([]byte(t.password))
	resp, err := t.sendCommand(fmt.Sprintf("AUTHENTICATE %s", hexPassword))
	if err != nil {
		return err
	}
	if !strings.HasPrefix(resp, "250") {
		return fmt.Errorf("password auth failed: %s", resp)
	}
	return nil
}

// authCookie performs cookie authentication
func (t *TorController) authCookie(cookiePath string) error {
	cookie, err := os.ReadFile(cookiePath)
	if err != nil {
		return fmt.Errorf("failed to read cookie: %w", err)
	}

	resp, err := t.sendCommand(fmt.Sprintf("AUTHENTICATE %s", hex.EncodeToString(cookie)))
	if err != nil {
		return err
	}
	if !strings.HasPrefix(resp, "250") {
		return fmt.Errorf("cookie auth failed: %s", resp)
	}
	return nil
}

// authSafeCookie performs SAFECOOKIE authentication (HMAC-based)
func (t *TorController) authSafeCookie(cookiePath string) error {
	cookie, err := os.ReadFile(cookiePath)
	if err != nil {
		return fmt.Errorf("failed to read cookie: %w", err)
	}

	// Generate client nonce
	clientNonce := make([]byte, 32)
	if _, err := rand.Read(clientNonce); err != nil {
		return err
	}

	// Send AUTHCHALLENGE
	resp, err := t.sendCommand(fmt.Sprintf("AUTHCHALLENGE SAFECOOKIE %s", hex.EncodeToString(clientNonce)))
	if err != nil {
		return err
	}

	// Parse server nonce and hash from response
	serverNonce, serverHash, err := t.parseAuthChallenge(resp)
	if err != nil {
		return err
	}

	// Compute expected server hash and verify
	expectedServerHash := computeTorHMAC(cookie, clientNonce, serverNonce, true)
	if !hmac.Equal(serverHash, expectedServerHash) {
		return errors.New("server hash verification failed")
	}

	// Compute client hash for authentication
	clientHash := computeTorHMAC(cookie, clientNonce, serverNonce, false)

	resp, err = t.sendCommand(fmt.Sprintf("AUTHENTICATE %s", hex.EncodeToString(clientHash)))
	if err != nil {
		return err
	}
	if !strings.HasPrefix(resp, "250") {
		return fmt.Errorf("safecookie auth failed: %s", resp)
	}
	return nil
}

// createHiddenService creates a new ephemeral hidden service
func (t *TorController) createHiddenService() (string, error) {
	// ADD_ONION creates an ephemeral hidden service
	// NEW:BEST generates a new key with best available algorithm
	cmd := fmt.Sprintf("ADD_ONION NEW:BEST Port=%d,127.0.0.1:%d", t.servicePort, t.servicePort)

	resp, err := t.sendCommand(cmd)
	if err != nil {
		return "", err
	}

	// Parse onion address from response
	// Response format: 250-ServiceID=<onion_address_without_.onion>
	lines := strings.Split(resp, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "250-ServiceID=") {
			serviceID := strings.TrimPrefix(line, "250-ServiceID=")
			serviceID = strings.TrimSpace(serviceID)
			return serviceID + ".onion", nil
		}
	}

	return "", fmt.Errorf("failed to parse onion address from response: %s", resp)
}

// sendCommand sends a command and reads the response
// Note: This method assumes the caller holds t.mu lock for thread safety
func (t *TorController) sendCommand(cmd string) (string, error) {
	if t.conn == nil {
		return "", errors.New("not connected to Tor control port")
	}

	// Set deadline
	t.conn.SetDeadline(time.Now().Add(30 * time.Second))
	defer t.conn.SetDeadline(time.Time{})

	// Send command
	if _, err := fmt.Fprintf(t.conn, "%s\r\n", cmd); err != nil {
		return "", err
	}

	// Read response (may be multiple lines)
	var response strings.Builder
	for {
		line, err := t.reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		response.WriteString(line)

		// Check for end of response
		// 250 = success, 5xx = error, single line ends with space after code
		if len(line) >= 4 {
			if line[3] == ' ' || line[3] == '\r' {
				break
			}
		}
	}

	return response.String(), nil
}

// parseAuthMethods extracts auth methods from PROTOCOLINFO response
func (t *TorController) parseAuthMethods(resp string) []string {
	var methods []string
	for _, line := range strings.Split(resp, "\n") {
		if strings.HasPrefix(line, "250-AUTH METHODS=") {
			parts := strings.SplitN(line, "METHODS=", 2)
			if len(parts) == 2 {
				methodsPart := strings.Split(parts[1], " ")[0]
				methods = strings.Split(methodsPart, ",")
			}
		}
	}
	return methods
}

// parseCookiePath extracts cookie file path from PROTOCOLINFO response
func (t *TorController) parseCookiePath(resp string) string {
	for _, line := range strings.Split(resp, "\n") {
		if strings.Contains(line, "COOKIEFILE=") {
			parts := strings.SplitN(line, "COOKIEFILE=\"", 2)
			if len(parts) == 2 {
				path := strings.SplitN(parts[1], "\"", 2)[0]
				// Expand path if needed
				if strings.HasPrefix(path, "~") {
					home, _ := os.UserHomeDir()
					path = filepath.Join(home, path[1:])
				}
				return path
			}
		}
	}
	return ""
}

// parseAuthChallenge parses AUTHCHALLENGE response
func (t *TorController) parseAuthChallenge(resp string) (serverNonce, serverHash []byte, err error) {
	for _, line := range strings.Split(resp, "\n") {
		if strings.HasPrefix(line, "250 AUTHCHALLENGE") {
			// Parse SERVERHASH and SERVERNONCE
			parts := strings.Fields(line)
			for _, part := range parts {
				if strings.HasPrefix(part, "SERVERHASH=") {
					hashHex := strings.TrimPrefix(part, "SERVERHASH=")
					serverHash, err = hex.DecodeString(hashHex)
					if err != nil {
						return nil, nil, err
					}
				}
				if strings.HasPrefix(part, "SERVERNONCE=") {
					nonceHex := strings.TrimPrefix(part, "SERVERNONCE=")
					serverNonce, err = hex.DecodeString(nonceHex)
					if err != nil {
						return nil, nil, err
					}
				}
			}
		}
	}
	if serverNonce == nil || serverHash == nil {
		return nil, nil, errors.New("failed to parse AUTHCHALLENGE response")
	}
	return serverNonce, serverHash, nil
}

// computeTorHMAC computes HMAC for SAFECOOKIE auth
func computeTorHMAC(cookie, clientNonce, serverNonce []byte, isServer bool) []byte {
	var key string
	if isServer {
		key = "Tor safe cookie authentication server-to-controller hash"
	} else {
		key = "Tor safe cookie authentication controller-to-server hash"
	}

	h := hmac.New(sha256.New, []byte(key))
	h.Write(cookie)
	h.Write(clientNonce)
	h.Write(serverNonce)
	return h.Sum(nil)
}

// contains checks if slice contains string
func contains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// ConnectViaTor connects to a .onion address through Tor SOCKS proxy
func ConnectViaTor(torProxy, onionAddr string, timeout time.Duration) (net.Conn, error) {
	dialer := NewProxyDialer(torProxy)
	return dialer.DialTimeout("tcp", onionAddr, timeout)
}

// IsTorAvailable checks if Tor control port is accessible
func IsTorAvailable(controlAddr string) bool {
	if controlAddr == "" {
		controlAddr = "127.0.0.1:9051"
	}
	conn, err := net.DialTimeout("tcp", controlAddr, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// GetDefaultTorProxy returns the default Tor SOCKS proxy address
func GetDefaultTorProxy() string {
	return "127.0.0.1:9050"
}

// ReadCookieFile reads and returns Tor auth cookie
func ReadCookieFile(dataDir string) ([]byte, error) {
	// Try common cookie locations
	paths := []string{
		filepath.Join(dataDir, "control_auth_cookie"),
		"/var/run/tor/control.authcookie",
		"/var/lib/tor/control_auth_cookie",
	}

	home, _ := os.UserHomeDir()
	if home != "" {
		paths = append(paths,
			filepath.Join(home, ".tor", "control_auth_cookie"),
			filepath.Join(home, "Library", "Application Support", "TorBrowser-Data", "Tor", "control_auth_cookie"),
		)
	}

	for _, path := range paths {
		cookie, err := os.ReadFile(path)
		if err == nil && len(cookie) == 32 {
			return cookie, nil
		}
	}

	return nil, errors.New("tor cookie file not found")
}
