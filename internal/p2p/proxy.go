package p2p

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

// SOCKS5 protocol constants
const (
	socks5Version       = 0x05
	socks5AuthNone      = 0x00
	socks5AuthPassword  = 0x02
	socks5CmdConnect    = 0x01
	socks5AddrTypeIPv4  = 0x01
	socks5AddrTypeDomain = 0x03
	socks5AddrTypeIPv6  = 0x04
	socks5ReplySuccess  = 0x00
)

// SOCKS5 errors
var (
	ErrSOCKS5AuthFailed       = errors.New("SOCKS5 authentication failed")
	ErrSOCKS5ConnectFailed    = errors.New("SOCKS5 connect failed")
	ErrSOCKS5UnsupportedVer   = errors.New("unsupported SOCKS version")
	ErrSOCKS5CredentialLength = errors.New("SOCKS5 username or password exceeds 255 bytes (RFC 1929 limit)")
)

// ProxyDialer represents a SOCKS5 proxy dialer
type ProxyDialer struct {
	ProxyAddr string // SOCKS5 proxy address (host:port)
	Username  string // Optional username for authentication
	Password  string // Optional password for authentication
	Timeout   time.Duration
}

// NewProxyDialer creates a new SOCKS5 proxy dialer
func NewProxyDialer(proxyAddr string) *ProxyDialer {
	return &ProxyDialer{
		ProxyAddr: proxyAddr,
		Timeout:   30 * time.Second,
	}
}

// NewProxyDialerWithAuth creates a new SOCKS5 proxy dialer with authentication
// Returns error if username or password exceeds 255 bytes (RFC 1929 limit)
func NewProxyDialerWithAuth(proxyAddr, username, password string) (*ProxyDialer, error) {
	// RFC 1929: Username and password are limited to 255 bytes each
	if len(username) > 255 || len(password) > 255 {
		return nil, ErrSOCKS5CredentialLength
	}
	return &ProxyDialer{
		ProxyAddr: proxyAddr,
		Username:  username,
		Password:  password,
		Timeout:   30 * time.Second,
	}, nil
}

// Dial connects to the target address through the SOCKS5 proxy
func (d *ProxyDialer) Dial(network, addr string) (net.Conn, error) {
	return d.DialTimeout(network, addr, d.Timeout)
}

// DialTimeout connects to the target address through the SOCKS5 proxy with timeout
func (d *ProxyDialer) DialTimeout(network, addr string, timeout time.Duration) (net.Conn, error) {
	// Connect to proxy
	conn, err := net.DialTimeout("tcp", d.ProxyAddr, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to proxy %s: %w", d.ProxyAddr, err)
	}

	// Set deadline for handshake
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to set deadline: %w", err)
	}

	// Perform SOCKS5 handshake
	if err := d.socks5Handshake(conn); err != nil {
		conn.Close()
		return nil, err
	}

	// Request connection to target
	if err := d.socks5Connect(conn, addr); err != nil {
		conn.Close()
		return nil, err
	}

	// Clear deadline after successful handshake
	if err := conn.SetDeadline(time.Time{}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to clear deadline: %w", err)
	}

	return conn, nil
}

// socks5Handshake performs the SOCKS5 authentication handshake
func (d *ProxyDialer) socks5Handshake(conn net.Conn) error {
	// Determine auth methods to offer
	var authMethods []byte
	if d.Username != "" {
		authMethods = []byte{socks5AuthNone, socks5AuthPassword}
	} else {
		authMethods = []byte{socks5AuthNone}
	}

	// Send greeting: version + number of methods + methods
	greeting := make([]byte, 2+len(authMethods))
	greeting[0] = socks5Version
	greeting[1] = byte(len(authMethods))
	copy(greeting[2:], authMethods)

	if _, err := conn.Write(greeting); err != nil {
		return fmt.Errorf("failed to send SOCKS5 greeting: %w", err)
	}

	// Read server's chosen method
	response := make([]byte, 2)
	if _, err := io.ReadFull(conn, response); err != nil {
		return fmt.Errorf("failed to read SOCKS5 response: %w", err)
	}

	if response[0] != socks5Version {
		return ErrSOCKS5UnsupportedVer
	}

	// Handle authentication based on server's choice
	switch response[1] {
	case socks5AuthNone:
		// No authentication required
		return nil

	case socks5AuthPassword:
		// Username/password authentication (RFC 1929)
		if d.Username == "" {
			return ErrSOCKS5AuthFailed
		}
		return d.socks5Auth(conn)

	case 0xFF:
		return errors.New("no acceptable authentication methods")

	default:
		return fmt.Errorf("unsupported authentication method: %d", response[1])
	}
}

// socks5Auth performs username/password authentication (RFC 1929)
func (d *ProxyDialer) socks5Auth(conn net.Conn) error {
	// Build auth request: version (1) + ulen (1) + username + plen (1) + password
	authReq := make([]byte, 0, 3+len(d.Username)+len(d.Password))
	authReq = append(authReq, 0x01) // Auth version
	authReq = append(authReq, byte(len(d.Username)))
	authReq = append(authReq, []byte(d.Username)...)
	authReq = append(authReq, byte(len(d.Password)))
	authReq = append(authReq, []byte(d.Password)...)

	if _, err := conn.Write(authReq); err != nil {
		return fmt.Errorf("failed to send auth request: %w", err)
	}

	// Read auth response
	response := make([]byte, 2)
	if _, err := io.ReadFull(conn, response); err != nil {
		return fmt.Errorf("failed to read auth response: %w", err)
	}

	if response[1] != 0x00 {
		return ErrSOCKS5AuthFailed
	}

	return nil
}

// socks5Connect requests a connection to the target address
func (d *ProxyDialer) socks5Connect(conn net.Conn, addr string) error {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid target address: %w", err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid port: %w", err)
	}

	// Build connect request
	// VER + CMD + RSV + ATYP + DST.ADDR + DST.PORT
	request := make([]byte, 0, 10+len(host))
	request = append(request, socks5Version)
	request = append(request, socks5CmdConnect)
	request = append(request, 0x00) // Reserved

	// Determine address type
	ip := net.ParseIP(host)
	if ip == nil {
		// Domain name
		if len(host) > 255 {
			return errors.New("domain name too long")
		}
		request = append(request, socks5AddrTypeDomain)
		request = append(request, byte(len(host)))
		request = append(request, []byte(host)...)
	} else if ip4 := ip.To4(); ip4 != nil {
		// IPv4
		request = append(request, socks5AddrTypeIPv4)
		request = append(request, ip4...)
	} else {
		// IPv6
		request = append(request, socks5AddrTypeIPv6)
		request = append(request, ip.To16()...)
	}

	// Append port (big-endian)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(port))
	request = append(request, portBytes...)

	if _, err := conn.Write(request); err != nil {
		return fmt.Errorf("failed to send connect request: %w", err)
	}

	// Read response header (VER + REP + RSV + ATYP)
	response := make([]byte, 4)
	if _, err := io.ReadFull(conn, response); err != nil {
		return fmt.Errorf("failed to read connect response: %w", err)
	}

	if response[0] != socks5Version {
		return ErrSOCKS5UnsupportedVer
	}

	if response[1] != socks5ReplySuccess {
		return fmt.Errorf("%w: reply code %d", ErrSOCKS5ConnectFailed, response[1])
	}

	// Read and discard bound address based on address type
	switch response[3] {
	case socks5AddrTypeIPv4:
		// 4 bytes IPv4 + 2 bytes port
		discard := make([]byte, 6)
		if _, err := io.ReadFull(conn, discard); err != nil {
			return fmt.Errorf("failed to read bound address: %w", err)
		}
	case socks5AddrTypeDomain:
		// 1 byte length + domain + 2 bytes port
		lenByte := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenByte); err != nil {
			return fmt.Errorf("failed to read domain length: %w", err)
		}
		discard := make([]byte, int(lenByte[0])+2)
		if _, err := io.ReadFull(conn, discard); err != nil {
			return fmt.Errorf("failed to read bound address: %w", err)
		}
	case socks5AddrTypeIPv6:
		// 16 bytes IPv6 + 2 bytes port
		discard := make([]byte, 18)
		if _, err := io.ReadFull(conn, discard); err != nil {
			return fmt.Errorf("failed to read bound address: %w", err)
		}
	default:
		return fmt.Errorf("unknown address type in response: %d", response[3])
	}

	return nil
}

// IsOnionAddress checks if an address is a .onion (Tor) address
func IsOnionAddress(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	// Check for .onion suffix (Tor hidden services)
	return len(host) > 6 && host[len(host)-6:] == ".onion"
}
