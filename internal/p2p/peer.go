package p2p

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/twins-dev/twins-core/pkg/types"
)

// ErrBadMagic indicates the remote sent bytes that don't match any TWINS network magic.
// This is normal internet noise (HTTP probes, TLS scanners, etc.) and should be logged
// at debug level only.
var ErrBadMagic = errors.New("bad network magic")

// isNonProtocolError returns true for errors caused by non-TWINS connections
// (scanners, HTTP probes, etc.) that should be logged at debug level.
func isNonProtocolError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrBadMagic) {
		return true
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "message too large") ||
		strings.Contains(errMsg, "incomplete header")
}

// isExpectedNetworkError returns true for common network errors that are expected
// during normal P2P operation (connection closed, broken pipe, etc.)
func isExpectedNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// Check for common expected errors
	if errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, syscall.EPIPE) || // Broken pipe
		errors.Is(err, syscall.ECONNRESET) { // Connection reset by peer
		return true
	}

	// Check error message for common patterns
	errMsg := strings.ToLower(err.Error())
	if strings.Contains(errMsg, "connection closed") ||
		strings.Contains(errMsg, "broken pipe") ||
		strings.Contains(errMsg, "connection reset") ||
		strings.Contains(errMsg, "use of closed network connection") {
		return true
	}

	return false
}

// Peer represents a connected peer in the TWINS network
type Peer struct {
	// Connection details
	conn        net.Conn
	addr        *NetAddress
	version     *VersionMessage
	services    ServiceFlag
	userAgent   string
	startHeight int32
	timeOffset  int64   // Time offset from peer (calculated at handshake)
	magic       [4]byte // Network magic bytes

	// Connection state
	connected      atomic.Bool
	inbound        bool
	persistent     bool
	handshake      atomic.Bool
	headersSyncing atomic.Bool
	fGetAddr       atomic.Bool // true after we send getaddr; prevents relaying solicited addr responses

	// Message handling
	writeQueue chan outgoingMessage
	stopWrite  chan struct{}
	stopRead   chan struct{}

	// Statistics (atomic for thread safety)
	bytesReceived atomic.Uint64
	bytesSent     atomic.Uint64
	timeConnected time.Time
	lastPing      atomic.Int64 // Unix nanosecond timestamp (time.Now().UnixNano())
	lastPong      atomic.Int64 // Unix nanosecond timestamp (time.Now().UnixNano())
	pingNonce     atomic.Uint64

	// Synchronization and lifecycle
	mu       sync.RWMutex
	quit     chan struct{}
	wg       sync.WaitGroup
	stopOnce sync.Once // Ensures Stop() is only executed once

	// Logging
	logger *logrus.Entry

	// Rate limiting and message tracking
	lastMessageTime atomic.Int64 // Last received message time (Unix seconds)
	lastSendTime    atomic.Int64 // Last sent message time (Unix seconds)
	messageCount    atomic.Uint32

	// Back-reference to server for shared stats (msgTypeSent counters)
	server *Server

	// SPV support
	bloomFilter *BloomFilter // Bloom filter for SPV clients

	// Block sync pipelining (hashContinue mechanism from legacy)
	hashContinueMu sync.Mutex
	hashContinue   *types.Hash // Last block hash from inv batch (triggers auto-inv on getdata)

	// Misbehavior tracking (legacy: -banscore)
	misbehaviorScore atomic.Int32 // Accumulated misbehavior score

	// Self-connection detection: nonce we sent in our version message to this peer.
	// Stored so it can be removed from server.sentNonces when the peer disconnects.
	localNonce atomic.Uint64

	// Rate limiting for repeated getblocks/getheaders requests
	lastGetBlocksLocator  atomic.Value // types.Hash — first hash from last getblocks locator
	lastGetBlocksTime     atomic.Int64 // Unix timestamp of last getblocks
	lastGetHeadersLocator atomic.Value // types.Hash — first hash from last getheaders locator
	lastGetHeadersTime    atomic.Int64 // Unix timestamp of last getheaders

	// Rate limiting for addr messages (per-peer flood protection)
	addrMsgCount     atomic.Int32 // Count of addr messages in current window
	addrMsgWindowEnd atomic.Int64 // Unix timestamp when current window expires

	// Configurable intervals (set by server from config, 0 = use protocol defaults)
	pingInterval   time.Duration // Keep-alive ping interval (default: PingInterval from protocol.go)
	dialTimeout    time.Duration // Outbound connection timeout (default: HandshakeTimeout)

	// Protocol 70928: peer height tracking via extended ping/pong
	peerHeight    atomic.Uint32 // Latest height reported by peer (via ping/pong)
	currentHeight atomic.Uint32 // Our current height, pushed to peer in pings

	// Health tracker reference for EffectivePeerHeight() (set by syncer on peer discovery)
	healthTracker atomic.Pointer[PeerHealthTracker]

	// Protocol 70928: getchainstate rate limiting
	lastGetChainStateTime atomic.Int64 // Unix timestamp of last getchainstate from this peer

	// Protocol 70928: chainstate response channel
	chainStateCh chan *ChainStateMessage // Receives chainstate response from peer
}

// outgoingMessage represents a message to be sent to the peer
type outgoingMessage struct {
	message  *Message
	resultCh chan error // Optional channel to receive send result
}

// PeerStats contains statistics about a peer
type PeerStats struct {
	Address         string
	Services        ServiceFlag
	UserAgent       string
	ProtocolVersion int32
	StartHeight     int32
	Connected       bool
	Inbound         bool
	TimeConnected   time.Time
	BytesReceived   uint64
	BytesSent       uint64
	LastPing        time.Time
	LastPong        time.Time
	MessagesSent    uint32
}

// PeerMessage represents a message received from a peer
type PeerMessage struct {
	Peer    *Peer
	Message *Message
}

// NewPeer creates a new peer instance from an existing connection with default queue size
func NewPeer(conn net.Conn, inbound bool, magic [4]byte, logger *logrus.Logger) *Peer {
	return NewPeerWithQueueSize(conn, inbound, magic, logger, 5000) // Default 5000 queue size (10x increase for burst traffic)
}

// NewPeerWithQueueSize creates a new peer instance with configurable queue size
func NewPeerWithQueueSize(conn net.Conn, inbound bool, magic [4]byte, logger *logrus.Logger, queueSize int) *Peer {
	// Create peer address from connection
	addr := &NetAddress{
		Time:     uint32(time.Now().Unix()),
		Services: 0, // Will be set during handshake
		Port:     0, // Will be set during handshake
	}

	// Extract IP from connection
	if tcpAddr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		addr.IP = tcpAddr.IP
		addr.Port = uint16(tcpAddr.Port)
	}

	// Ensure minimum queue size
	if queueSize < 10 {
		queueSize = 10
	}

	peer := &Peer{
		conn:          conn,
		addr:          addr,
		inbound:       inbound,
		magic:         magic,
		timeConnected: time.Now(),
		writeQueue:    make(chan outgoingMessage, queueSize), // Configurable buffered channel
		stopWrite:     make(chan struct{}),
		stopRead:      make(chan struct{}),
		quit:          make(chan struct{}),
		chainStateCh:  make(chan *ChainStateMessage, 1), // Protocol 70928: buffered for async response
		logger: logger.WithFields(logrus.Fields{
			"peer":       conn.RemoteAddr().String(),
			"inbound":    inbound,
			"queue_size": queueSize,
		}),
	}

	peer.connected.Store(false)
	peer.handshake.Store(false)

	return peer
}

// Connect establishes an outbound connection to a peer with default queue size
func Connect(address string, magic [4]byte, logger *logrus.Logger) (*Peer, error) {
	return ConnectWithQueueSize(address, magic, logger, 5000)
}

// ConnectWithQueueSize establishes an outbound connection with configurable queue size
func ConnectWithQueueSize(address string, magic [4]byte, logger *logrus.Logger, queueSize int) (*Peer, error) {
	logger.WithFields(logrus.Fields{
		"address": address,
		"timeout": HandshakeTimeout,
	}).Debug("Initiating outbound connection")

	// Parse address
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid address %s: %w", address, err)
	}

	// Resolve address
	resolveStart := time.Now()
	tcpAddr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(host, port))
	resolveDuration := time.Since(resolveStart)

	logger.WithFields(logrus.Fields{
		"address":          address,
		"resolved_addr":    tcpAddr,
		"resolve_duration": resolveDuration,
	}).Debug("Address resolved")

	if err != nil {
		return nil, fmt.Errorf("failed to resolve address %s: %w", address, err)
	}

	// Establish connection with timeout
	dialStart := time.Now()
	conn, err := net.DialTimeout("tcp", tcpAddr.String(), HandshakeTimeout)
	dialDuration := time.Since(dialStart)

	if err != nil {
		logger.WithFields(logrus.Fields{
			"address":       address,
			"dial_duration": dialDuration,
			"error":         err.Error(),
		}).Debug("Failed to establish connection")
		return nil, fmt.Errorf("failed to connect to %s: %w", address, err)
	}

	logger.WithFields(logrus.Fields{
		"address":       address,
		"local_addr":    conn.LocalAddr(),
		"remote_addr":   conn.RemoteAddr(),
		"dial_duration": dialDuration,
	}).Debug("Connection established")

	// Create peer with configured queue size
	peer := NewPeerWithQueueSize(conn, false, magic, logger, queueSize)
	peer.persistent = true // Outbound connections are persistent by default

	return peer, nil
}

// Start begins message processing for the peer
func (p *Peer) Start(server *Server) {
	p.logger.Debug("Starting peer connection")

	// Mark TCP socket as active. Note: connected flag is set later
	// in MarkHandshakeComplete() — peer is not "connected" until
	// version handshake succeeds.

	// Start message handling goroutines
	p.wg.Add(2)
	go p.writeLoop()
	go p.readLoop(server)

	// Start ping routine for outbound connections
	if !p.inbound {
		p.wg.Add(1)
		go p.pingLoop()
	}
}

// Stop gracefully shuts down the peer connection
func (p *Peer) Stop() {
	p.stopOnce.Do(func() {
		p.logger.Debug("Stopping peer connection")

		// Mark as disconnected
		p.connected.Store(false)

		// Signal shutdown
		close(p.quit)

		// Close connection to break read/write operations
		if p.conn != nil {
			p.conn.Close()
		}

		// Signal write loop to stop
		close(p.stopWrite)

		// Wait for goroutines to finish
		done := make(chan struct{})
		go func() {
			p.wg.Wait()
			close(done)
		}()

		// Wait with timeout
		select {
		case <-done:
			p.logger.Debug("Peer stopped gracefully")
		case <-time.After(5 * time.Second):
			p.logger.Warn("Peer stop timeout")
		}
	})
}

// SendMessage sends a message to the peer asynchronously with default timeout
func (p *Peer) SendMessage(msg *Message) error {
	return p.SendMessageWithTimeout(msg, 30*time.Second)
}

// SendMessageWithTimeout sends a message with configurable timeout for backpressure
func (p *Peer) SendMessageWithTimeout(msg *Message, timeout time.Duration) error {
	// Check if peer is shutting down (quit channel closed).
	// Do NOT check connected flag here — SendMessage must work for
	// pre-handshake messages (version, verack) before connected is set.
	select {
	case <-p.quit:
		return fmt.Errorf("peer shutting down")
	default:
	}

	// Try immediate non-blocking send first
	select {
	case p.writeQueue <- outgoingMessage{message: msg, resultCh: nil}:
		return nil
	default:
		// Queue is full, apply backpressure with timeout
	}

	// Queue full - wait with timeout
	queueLen := len(p.writeQueue)
	queueCap := cap(p.writeQueue)
	utilization := float64(queueLen) / float64(queueCap) * 100

	p.logger.WithFields(logrus.Fields{
		"command":         msg.GetCommand(),
		"queue_len":       queueLen,
		"queue_cap":       queueCap,
		"queue_util_pct":  utilization,
		"timeout_seconds": timeout.Seconds(),
	}).Warn("Write queue full, applying backpressure")

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case p.writeQueue <- outgoingMessage{message: msg, resultCh: nil}:
		return nil
	case <-timer.C:
		p.logger.WithFields(logrus.Fields{
			"command":   msg.GetCommand(),
			"queue_len": len(p.writeQueue),
			"queue_cap": cap(p.writeQueue),
			"timeout":   timeout,
		}).Error("Write queue timeout - peer too slow, dropping message")
		return fmt.Errorf("write queue timeout after %v", timeout)
	case <-p.quit:
		return fmt.Errorf("peer shutting down")
	}
}

// SendMessageSync sends a message synchronously with WriteTimeout.
func (p *Peer) SendMessageSync(msg *Message) error {
	return p.SendMessageWithTimeout(msg, WriteTimeout)
}

// writeLoop handles outgoing messages with retry logic for transient errors
func (p *Peer) writeLoop() {
	defer p.wg.Done()

	p.logger.Debug("Starting write loop")

	const maxRetries = 3
	const baseBackoff = 100 * time.Millisecond

	for {
		select {
		case outMsg := <-p.writeQueue:
			var err error

			// Retry loop for transient errors
			for attempt := 0; attempt <= maxRetries; attempt++ {
				err = p.writeMessage(outMsg.message)

				if err == nil {
					if p.server != nil {
						p.server.incrMsgType(&p.server.msgTypeSent, outMsg.message.GetCommand())
					}
					break
				}

				// Fatal error - connection truly closed, exit immediately
				if isExpectedNetworkError(err) {
					p.logger.WithError(err).Debug("Connection closed while writing")
					// Send error result if channel provided
					if outMsg.resultCh != nil {
						select {
						case outMsg.resultCh <- err:
						default:
						}
					}
					return
				}

				// Transient error - retry with exponential backoff
				if attempt < maxRetries {
					backoff := baseBackoff * time.Duration(1<<uint(attempt)) // 100ms, 200ms, 400ms
					p.logger.WithFields(logrus.Fields{
						"error":   err.Error(),
						"attempt": attempt + 1,
						"max":     maxRetries + 1,
						"backoff": backoff,
						"command": outMsg.message.GetCommand(),
					}).Warn("Write failed, retrying after backoff")
					time.Sleep(backoff)
				} else {
					// Max retries exceeded - log and drop message
					p.logger.WithFields(logrus.Fields{
						"error":    err.Error(),
						"attempts": maxRetries + 1,
						"command":  outMsg.message.GetCommand(),
					}).Error("Write failed after all retries, dropping message")
				}
			}

			// Send result if channel is provided (success or final error)
			if outMsg.resultCh != nil {
				select {
				case outMsg.resultCh <- err:
				default:
				}
			}

			// Continue processing next message even if this one failed
			// (connection is still alive, just had temporary congestion)

		case <-p.stopWrite:
			p.logger.Debug("Write loop stopping")
			return

		case <-p.quit:
			p.logger.Debug("Write loop stopping due to peer shutdown")
			return
		}
	}
}

// readLoop handles incoming messages
func (p *Peer) readLoop(server *Server) {
	defer p.wg.Done()

	// Track if we exited due to an error (not graceful shutdown)
	exitError := true
	defer func() {
		if exitError && server != nil {
			// Notify server to remove this peer on error exit.
			// Do not drop this signal: leaked half-dead peers pollute getpeerinfo.
			go func() {
				select {
				case server.donePeers <- p:
					p.logger.Debug("Notified server to remove peer after read error")
				case <-server.quit:
				case <-p.quit:
				}
			}()
		}
	}()

	p.logger.Debug("Starting read loop")

	for {
		select {
		case <-p.quit:
			p.logger.Debug("Read loop stopping due to peer shutdown")
			exitError = false // Graceful shutdown
			return
		default:
		}

		// Set read timeout
		deadline := time.Now().Add(ReadTimeout)
		if err := p.conn.SetReadDeadline(deadline); err != nil {
			p.logger.WithError(err).Error("Failed to set read deadline")
			return
		}

		p.logger.WithFields(logrus.Fields{
			"deadline":        deadline,
			"timeout_seconds": ReadTimeout.Seconds(),
		}).Trace("Set read deadline")

		// Read message
		msg, err := p.readMessage()
		if err != nil {
			// Check if peer is shutting down (quit channel closed)
			select {
			case <-p.quit:
				p.logger.Debug("Connection already closed, exiting read loop")
				return
			default:
			}

			// Check if it's a timeout or expected network error
			netErr, isNetErr := err.(net.Error)

			logFields := logrus.Fields{
				"error":          err.Error(),
				"is_net_error":   isNetErr,
				"is_timeout":     isNetErr && netErr.Timeout(),
				"is_temporary":   isNetErr && netErr.Temporary(),
				"handshake_done": p.handshake.Load(),
				"bytes_received": p.bytesReceived.Load(),
				"bytes_sent":     p.bytesSent.Load(),
				"time_connected": time.Since(p.timeConnected),
				"last_msg_time":  time.Unix(p.lastMessageTime.Load(), 0),
			}

			// Use DEBUG for expected network errors and non-protocol connections
			// (scanners, HTTP probes, TLS probes, etc.), ERROR for unexpected failures.
			if isExpectedNetworkError(err) {
				p.logger.WithFields(logFields).Debug("Connection closed while reading")
			} else if isNonProtocolError(err) {
				p.logger.WithFields(logFields).Debug("Non-protocol connection rejected")
			} else {
				p.logger.WithFields(logFields).Error("Failed to read message")
			}
			return
		}

		if msg != nil {
			// Update statistics
			p.bytesReceived.Add(uint64(len(msg.Payload) + 24)) // 24 bytes header
			p.lastMessageTime.Store(time.Now().Unix())
			p.messageCount.Add(1)

			// Handle pong messages directly to avoid msgChan bottleneck during sync.
			// Pong processing is critical for connection health and should never be dropped.
			// Protocol 70928+: pong is 12 bytes (nonce + height). Legacy: 8 bytes (nonce only).
			if msg.GetCommand() == string(MsgPong) {
				switch len(msg.Payload) {
				case 12:
					// Protocol 70928: Nonce(8) + Height(4)
					nonce := binary.LittleEndian.Uint64(msg.Payload[0:8])
					height := binary.LittleEndian.Uint32(msg.Payload[8:12])
					p.HandlePong(nonce, height)
				case 8:
					// Legacy: Nonce(8) only
					nonce := binary.LittleEndian.Uint64(msg.Payload)
					p.HandlePong(nonce, 0)
				default:
					p.logger.Warn("Invalid pong message size")
				}
				continue
			}

			// Route addr messages to low-priority channel to prevent sync stall.
			// When 17+ peers each send 1000 addresses, addr floods fill the shared
			// msgChan and block sync-critical messages (block, header, inv).
			if msg.GetCommand() == string(MsgAddr) {
				select {
				case server.addrChan <- &PeerMessage{Peer: p, Message: msg}:
				case <-p.quit:
					return
				default:
					// Silently drop addr when low-priority channel full — addr loss is harmless
				}
				continue
			}

			// Handshake-critical messages routed to dedicated handshakeChan.
			// This channel is separate from msgChan so version/verack are never
			// blocked by a full msgChan (which caused permanent handshake stalls).
			// Unlike addr (best-effort, thousands), handshake messages are exactly 2 per peer —
			// dropping one loses the entire peer connection. Blocking is safe here because:
			// - each peer has its own readLoop goroutine (no cross-peer impact)
			// - pre-handshake peers have no other important messages to read
			// - quit channel provides escape (watchdog closes quit via peer.Stop())
			cmd := MessageType(msg.GetCommand())
			if cmd == MsgVersion || cmd == MsgVerAck {
				select {
				case server.handshakeChan <- &PeerMessage{Peer: p, Message: msg}:
				case <-p.quit:
					return
				}
				continue
			}

			// Send all other messages to high-priority channel
			select {
			case server.msgChan <- &PeerMessage{Peer: p, Message: msg}:
			case <-p.quit:
				return
			default:
				server.droppedMessages.Add(1)
				p.logger.WithField("command", msg.GetCommand()).Warn("Server message queue full, dropping message")
			}
		}
	}
}

// getPingInterval returns the configured ping interval, falling back to protocol default.
func (p *Peer) getPingInterval() time.Duration {
	if p.pingInterval > 0 {
		return p.pingInterval
	}
	return PingInterval
}

// getDialTimeout returns the configured dial timeout, falling back to protocol default.
func (p *Peer) getDialTimeout() time.Duration {
	if p.dialTimeout > 0 {
		return p.dialTimeout
	}
	return HandshakeTimeout
}

// pingLoop sends periodic pings to keep the connection alive
func (p *Peer) pingLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.getPingInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Do not send keepalive traffic before handshake completes.
			// Peers with incomplete handshake are handled by server-side timeout cleanup.
			if !p.IsHandshakeComplete() {
				continue
			}

			if err := p.sendPing(); err != nil {
				p.logger.WithError(err).Error("Failed to send ping")
				return
			}

			// Check for ping timeout
			lastPong := p.lastPong.Load()
			if lastPong > 0 && time.Since(time.Unix(0, lastPong)) > PingTimeout {
				p.logger.Warn("Ping timeout, disconnecting peer")
				go p.Stop() // Actually disconnect the peer
				return
			}

		case <-p.quit:
			p.logger.Debug("Ping loop stopping")
			return
		}
	}
}

// writeMessage writes a message to the connection
func (p *Peer) writeMessage(msg *Message) error {
	// Set write timeout
	if err := p.conn.SetWriteDeadline(time.Now().Add(WriteTimeout)); err != nil {
		return fmt.Errorf("failed to set write deadline: %w", err)
	}

	// Serialize message
	data, err := msg.Serialize()
	if err != nil {
		return fmt.Errorf("failed to serialize message: %w", err)
	}

	// Send-side size check: don't send messages that exceed protocol limit.
	// Peers enforce MaxProtocolMessageLength and would disconnect us.
	// Legacy: no explicit send check, but peers enforce MAX_PROTOCOL_MESSAGE_LENGTH (net.h:54)
	if len(data) < 24 {
		return fmt.Errorf("serialized message too short: %d bytes (expected at least 24-byte header)", len(data))
	}
	payloadLen := len(data) - 24 // subtract 24-byte header
	if payloadLen > MaxProtocolMessageLength {
		p.logger.WithFields(logrus.Fields{
			"command":      msg.GetCommand(),
			"payload_size": payloadLen,
			"max_size":     MaxProtocolMessageLength,
		}).Warn("Dropping outgoing message exceeding protocol limit")
		return fmt.Errorf("outgoing message %s too large: %d bytes (max: %d)",
			msg.GetCommand(), payloadLen, MaxProtocolMessageLength)
	}

	// Write to connection
	_, err = p.conn.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	// Update statistics
	p.bytesSent.Add(uint64(len(data)))
	p.lastSendTime.Store(time.Now().Unix())

	p.logger.WithFields(logrus.Fields{
		"command": msg.GetCommand(),
		"size":    len(data),
	}).Debug("Sent message")

	return nil
}

// readMessage reads a message from the connection
func (p *Peer) readMessage() (*Message, error) {
	// Log before attempting header read
	p.logger.WithFields(logrus.Fields{
		"remote_addr":    p.conn.RemoteAddr(),
		"local_addr":     p.conn.LocalAddr(),
		"handshake_done": p.handshake.Load(),
		"last_msg_time":  time.Unix(p.lastMessageTime.Load(), 0),
		"msg_count":      p.messageCount.Load(),
	}).Debug("Attempting to read message header")

	// Read header first (24 bytes) using io.ReadFull to ensure complete read
	headerBuf := make([]byte, 24)
	startTime := time.Now()
	n, err := io.ReadFull(p.conn, headerBuf)
	readDuration := time.Since(startTime)

	p.logger.WithFields(logrus.Fields{
		"bytes_read":     n,
		"read_duration":  readDuration,
		"expected_bytes": 24,
	}).Debug("Header read attempt completed")

	if err != nil {
		logFields := logrus.Fields{
			"error":          err.Error(),
			"bytes_read":     n,
			"read_duration":  readDuration,
			"remote_addr":    p.conn.RemoteAddr(),
			"handshake_done": p.handshake.Load(),
		}

		// Use DEBUG for EOF/connection closed during handshake, ERROR for other failures
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			p.logger.WithFields(logFields).Debug("Connection closed while reading header")
		} else {
			p.logger.WithFields(logFields).Debug("Failed to read header")
		}

		if err == io.EOF {
			return nil, fmt.Errorf("connection closed while reading header from %s (handshake: %v, msgs: %d)",
				p.conn.RemoteAddr(), p.handshake.Load(), p.messageCount.Load())
		}
		if err == io.ErrUnexpectedEOF {
			return nil, fmt.Errorf("incomplete header from %s (handshake: %v, msgs: %d, got: %d/24 bytes)",
				p.conn.RemoteAddr(), p.handshake.Load(), p.messageCount.Load(), n)
		}
		return nil, fmt.Errorf("failed to read header from %s (handshake: %v, msgs: %d): %w",
			p.conn.RemoteAddr(), p.handshake.Load(), p.messageCount.Load(), err)
	}

	// Validate magic bytes before parsing anything else.
	// Rejects non-TWINS connections (HTTP probes, TLS scanners, etc.) immediately
	// without wasting cycles on payload length or command parsing.
	var headerMagic [4]byte
	copy(headerMagic[:], headerBuf[0:4])
	if headerMagic != p.magic {
		return nil, fmt.Errorf("bad magic from %s: got %x, want %x (handshake: %v, msgs: %d): %w",
			p.conn.RemoteAddr(), headerMagic, p.magic, p.handshake.Load(), p.messageCount.Load(), ErrBadMagic)
	}

	// Parse header to get payload length and command
	payloadLength := uint32(headerBuf[16]) |
		uint32(headerBuf[17])<<8 |
		uint32(headerBuf[18])<<16 |
		uint32(headerBuf[19])<<24

	// Extract command from header (bytes 4-15, null-terminated)
	commandBytes := headerBuf[4:16]
	commandEnd := 0
	for i, b := range commandBytes {
		if b == 0 {
			commandEnd = i
			break
		}
	}
	if commandEnd == 0 {
		commandEnd = len(commandBytes)
	}
	command := string(commandBytes[:commandEnd])

	p.logger.WithFields(logrus.Fields{
		"command":        command,
		"payload_length": payloadLength,
		"magic":          fmt.Sprintf("%x", headerBuf[0:4]),
	}).Debug("Parsed message header")

	// Protocol-level size check: disconnect peers sending oversized messages.
	// Legacy: MAX_PROTOCOL_MESSAGE_LENGTH = 2 MiB (net.h:54), disconnects without banning.
	if payloadLength > MaxProtocolMessageLength {
		p.logger.WithFields(logrus.Fields{
			"payload_length": payloadLength,
			"max_size":       MaxProtocolMessageLength,
			"command":        command,
			"remote_addr":    p.conn.RemoteAddr(),
		}).Debug("Message exceeds protocol limit, disconnecting peer")
		return nil, fmt.Errorf("message too large from %s: %d bytes (protocol max: %d, cmd: %s)",
			p.conn.RemoteAddr(), payloadLength, MaxProtocolMessageLength, command)
	}

	// Hard buffer limit (should never be reached after protocol check above)
	if payloadLength > MaxMessageSize {
		p.logger.WithFields(logrus.Fields{
			"payload_length": payloadLength,
			"max_size":       MaxMessageSize,
		}).Error("Message exceeds maximum buffer size")
		return nil, fmt.Errorf("message too large from %s: %d bytes (max: %d, cmd: %s)",
			p.conn.RemoteAddr(), payloadLength, MaxMessageSize, command)
	}

	// Read payload if present using io.ReadFull to ensure complete read
	var fullMessage []byte
	if payloadLength > 0 {
		p.logger.WithFields(logrus.Fields{
			"command":        command,
			"payload_length": payloadLength,
		}).Debug("Reading message payload")

		payloadBuf := make([]byte, payloadLength)
		startTime := time.Now()
		n, err := io.ReadFull(p.conn, payloadBuf)
		readDuration := time.Since(startTime)

		p.logger.WithFields(logrus.Fields{
			"bytes_read":     n,
			"expected_bytes": payloadLength,
			"read_duration":  readDuration,
		}).Debug("Payload read attempt completed")

		if err != nil {
			p.logger.WithFields(logrus.Fields{
				"error":          err.Error(),
				"bytes_read":     n,
				"expected_bytes": payloadLength,
				"read_duration":  readDuration,
			}).Error("Failed to read payload")

			if err == io.EOF {
				return nil, fmt.Errorf("connection closed while reading %s payload from %s (got: %d/%d bytes)",
					command, p.conn.RemoteAddr(), n, payloadLength)
			}
			if err == io.ErrUnexpectedEOF {
				return nil, fmt.Errorf("incomplete %s payload from %s (got: %d/%d bytes)",
					command, p.conn.RemoteAddr(), n, payloadLength)
			}
			return nil, fmt.Errorf("failed to read %s payload from %s (got: %d/%d bytes): %w",
				command, p.conn.RemoteAddr(), n, payloadLength, err)
		}
		fullMessage = append(headerBuf, payloadBuf...)
	} else {
		fullMessage = headerBuf
	}

	// Deserialize message
	msg, err := DeserializeMessage(fullMessage)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize %s message from %s (%d bytes): %w",
			command, p.conn.RemoteAddr(), len(fullMessage), err)
	}

	p.logger.WithFields(logrus.Fields{
		"command": msg.GetCommand(),
		"size":    len(fullMessage),
	}).Debug("Received message")

	return msg, nil
}

// sendPing sends a ping message to the peer
func (p *Peer) sendPing() error {
	// Generate random nonce
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("failed to generate nonce: %w", err)
	}

	pingNonce := uint64(nonce[0]) |
		uint64(nonce[1])<<8 |
		uint64(nonce[2])<<16 |
		uint64(nonce[3])<<24 |
		uint64(nonce[4])<<32 |
		uint64(nonce[5])<<40 |
		uint64(nonce[6])<<48 |
		uint64(nonce[7])<<56

	// Store nonce for pong verification
	p.pingNonce.Store(pingNonce)
	p.lastPing.Store(time.Now().UnixNano())

	// Create and send ping message
	ping := &PingMessage{Nonce: pingNonce}
	payload, err := p.serializePing(ping)
	if err != nil {
		return fmt.Errorf("failed to serialize ping: %w", err)
	}

	msg := NewMessage(MsgPing, payload, p.getMagic())
	return p.SendMessage(msg)
}

// HandlePong processes a pong message.
// Any pong counts as proof-of-life (updates lastPong) regardless of nonce match.
// Nonce is only used for RTT measurement, not keepalive verification.
// Protocol 70928+: height is the peer's chain height from the pong payload.
func (p *Peer) HandlePong(nonce uint64, height uint32) error {
	p.lastPong.Store(time.Now().UnixNano())

	// Protocol 70928: update peer height from pong
	if height > 0 {
		p.peerHeight.Store(height)
		// Propagate to health tracker for stale height detection
		if ht := p.healthTracker.Load(); ht != nil {
			ht.UpdatePingHeight(p.GetAddress().String(), height)
		}
	}

	expectedNonce := p.pingNonce.Load()
	if nonce != expectedNonce {
		p.logger.WithFields(logrus.Fields{
			"expected": expectedNonce,
			"received": nonce,
		}).Debug("Received pong with mismatched nonce")
	}
	return nil
}

// serializePing serializes a ping message.
// Protocol 70928+: Nonce(8) + Height(4) = 12 bytes.
// Legacy (<70928): Nonce(8) = 8 bytes.
func (p *Peer) serializePing(ping *PingMessage) ([]byte, error) {
	if p.SupportsProto70928() {
		buf := make([]byte, 12)
		binary.LittleEndian.PutUint64(buf[0:8], ping.Nonce)
		binary.LittleEndian.PutUint32(buf[8:12], p.currentHeight.Load())
		return buf, nil
	}
	// Legacy 8-byte ping
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf[0:8], ping.Nonce)
	return buf, nil
}

// getMagic returns the appropriate magic bytes for the network
func (p *Peer) getMagic() [4]byte {
	return p.magic
}

// IsConnected returns whether the peer is connected
func (p *Peer) IsConnected() bool {
	return p.connected.Load()
}

// IsHandshakeComplete returns whether the handshake is complete
func (p *Peer) IsHandshakeComplete() bool {
	return p.handshake.Load()
}

// SetPeerVersion stores the remote peer's version information without marking
// the handshake as complete. This must be called before MarkHandshakeComplete.
func (p *Peer) SetPeerVersion(version *VersionMessage) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.version = version
	p.services = version.Services
	p.userAgent = version.UserAgent
	p.startHeight = version.StartHeight
	// Calculate time offset at handshake (peer's time - our time)
	p.timeOffset = version.Timestamp - time.Now().Unix()
}

// MarkHandshakeComplete marks the handshake as complete. Must be called AFTER
// version+verack have been queued to the writeQueue so that concurrent broadcasts
// cannot sneak into the TCP stream before the handshake messages.
// Also sets connected=true — peer is only considered "connected" after version check.
func (p *Peer) MarkHandshakeComplete() {
	p.handshake.Store(true)
	p.connected.Store(true)

	p.mu.RLock()
	v := p.version
	p.mu.RUnlock()

	if v != nil {
		p.logger.WithFields(logrus.Fields{
			"version":      v.Version,
			"services":     v.Services,
			"user_agent":   v.UserAgent,
			"start_height": v.StartHeight,
		}).Debug("Handshake completed")
	}
}

// SetHandshakeComplete stores version info and marks handshake as complete in one call.
// Convenience method for tests. Production code should use SetPeerVersion + MarkHandshakeComplete
// separately to avoid the race where broadcasts reach the writeQueue before version/verack.
func (p *Peer) SetHandshakeComplete(version *VersionMessage) {
	p.SetPeerVersion(version)
	p.MarkHandshakeComplete()
}

// GetStats returns peer statistics
func (p *Peer) GetStats() *PeerStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := &PeerStats{
		Address:       p.addr.String(),
		Services:      p.services,
		UserAgent:     p.userAgent,
		StartHeight:   p.startHeight,
		Connected:     p.connected.Load(),
		Inbound:       p.inbound,
		TimeConnected: p.timeConnected,
		BytesReceived: p.bytesReceived.Load(),
		BytesSent:     p.bytesSent.Load(),
		MessagesSent:  p.messageCount.Load(),
	}

	if version := p.version; version != nil {
		stats.ProtocolVersion = version.Version
	}

	lastPing := p.lastPing.Load()
	if lastPing > 0 {
		stats.LastPing = time.Unix(0, lastPing)
	}

	lastPong := p.lastPong.Load()
	if lastPong > 0 {
		stats.LastPong = time.Unix(0, lastPong)
	}

	return stats
}

// GetAddress returns the peer's network address
func (p *Peer) GetAddress() *NetAddress {
	return p.addr
}

// GetServices returns the peer's service flags
func (p *Peer) GetServices() ServiceFlag {
	return p.services
}

// GetVersion returns the peer's version message
func (p *Peer) GetVersion() *VersionMessage {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.version
}

// GetTimeOffset returns the time offset from peer (calculated at handshake)
func (p *Peer) GetTimeOffset() int64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.timeOffset
}

// GetID returns a unique identifier for the peer
func (p *Peer) GetID() string {
	if p.addr != nil {
		return p.addr.String()
	}
	return fmt.Sprintf("peer-%p", p)
}

// IsPersistent returns whether this is a persistent connection
func (p *Peer) IsPersistent() bool {
	return p.persistent
}

// SetPersistent sets the persistent flag
func (p *Peer) SetPersistent(persistent bool) {
	p.persistent = persistent
}

// SetHeadersSyncing sets the headers syncing state
func (p *Peer) SetHeadersSyncing(syncing bool) {
	p.headersSyncing.Store(syncing)
}

// IsHeadersSyncing returns true if the peer is syncing headers
func (p *Peer) IsHeadersSyncing() bool {
	return p.headersSyncing.Load()
}

// encodeVarInt encodes an integer as a variable-length integer (CompactSize encoding)
func encodeVarInt(n uint64) []byte {
	if n < 0xfd {
		return []byte{byte(n)}
	} else if n <= 0xffff {
		buf := make([]byte, 3)
		buf[0] = 0xfd
		binary.LittleEndian.PutUint16(buf[1:], uint16(n))
		return buf
	} else if n <= 0xffffffff {
		buf := make([]byte, 5)
		buf[0] = 0xfe
		binary.LittleEndian.PutUint32(buf[1:], uint32(n))
		return buf
	} else {
		buf := make([]byte, 9)
		buf[0] = 0xff
		binary.LittleEndian.PutUint64(buf[1:], n)
		return buf
	}
}

// SendGetHeaders sends a getheaders message to the peer
func (p *Peer) SendGetHeaders(locator []types.Hash, stop types.Hash) error {
	// Serialize getheaders message inline (Bitcoin protocol format)
	// CBlockLocator format:
	// - Version (4 bytes) - protocol version
	// - Hash count (varint)
	// - Block locator hashes (32 bytes each)
	// Then:
	// - Hash stop (32 bytes)

	buf := make([]byte, 0, 4+9+len(locator)*32+32)

	// Protocol version (4 bytes, little-endian)
	version := uint32(ProtocolVersion)
	versionBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(versionBytes, version)
	buf = append(buf, versionBytes...)

	// Hash count (varint)
	countBytes := encodeVarInt(uint64(len(locator)))
	buf = append(buf, countBytes...)

	// Block locator hashes
	for _, hash := range locator {
		buf = append(buf, hash[:]...)
	}

	// Hash stop
	buf = append(buf, stop[:]...)

	// Create and send the message
	msg := NewMessage(MsgGetHeaders, buf, p.magic)
	if err := p.SendMessage(msg); err != nil {
		p.logger.WithError(err).Error("Failed to send getheaders message")
		return err
	}

	p.logger.WithFields(logrus.Fields{
		"locator_count": len(locator),
		"stop":          stop.String()[:8],
	}).Debug("Sent getheaders message")

	return nil
}

// SendGetBlocks sends a getblocks message to the peer
func (p *Peer) SendGetBlocks(locator []types.Hash, stop types.Hash) error {
	// Serialize getblocks message (same format as getheaders)
	buf := make([]byte, 0, 4+9+len(locator)*32+32)

	// Protocol version (4 bytes, little-endian)
	version := uint32(ProtocolVersion)
	versionBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(versionBytes, version)
	buf = append(buf, versionBytes...)

	// Hash count (varint)
	countBytes := encodeVarInt(uint64(len(locator)))
	buf = append(buf, countBytes...)

	// Block locator hashes
	for _, hash := range locator {
		buf = append(buf, hash[:]...)
	}

	// Hash stop
	buf = append(buf, stop[:]...)

	// Create and send the message
	msg := NewMessage(MsgGetBlocks, buf, p.magic)
	if err := p.SendMessage(msg); err != nil {
		p.logger.WithError(err).Error("Failed to send getblocks message")
		return err
	}

	p.logger.WithFields(logrus.Fields{
		"locator_count": len(locator),
		"stop":          stop.String()[:8],
	}).Debug("Sent getblocks message")

	return nil
}

// SendGetData sends a getdata message to the peer
func (p *Peer) SendGetData(inv []InventoryVector) error {
	if len(inv) == 0 {
		return nil
	}

	// Serialize getdata message: varint count + (4 bytes type + 32 bytes hash) per item
	buf := make([]byte, 0, 9+len(inv)*36) // Max varint + items

	// Add count varint
	countBytes := encodeVarInt(uint64(len(inv)))
	buf = append(buf, countBytes...)

	// Add each inventory vector
	for _, item := range inv {
		typeBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(typeBytes, uint32(item.Type))
		buf = append(buf, typeBytes...)
		buf = append(buf, item.Hash[:]...)
	}

	// Create and send the message
	msg := NewMessage(MsgGetData, buf, p.magic)
	if err := p.SendMessage(msg); err != nil {
		p.logger.WithError(err).Error("Failed to send getdata message")
		return err
	}

	p.logger.WithField("inv_count", len(inv)).Debug("Sent getdata message")
	return nil
}

// GetQueueHealth returns write queue health metrics
func (p *Peer) GetQueueHealth() map[string]interface{} {
	queueLen := len(p.writeQueue)
	queueCap := cap(p.writeQueue)
	utilization := float64(queueLen) / float64(queueCap) * 100

	return map[string]interface{}{
		"queue_length":      queueLen,
		"queue_capacity":    queueCap,
		"queue_utilization": utilization,
		"is_saturated":      utilization > 80,
		"bytes_sent":        p.bytesSent.Load(),
		"messages_sent":     p.messageCount.Load(),
		"time_connected":    time.Since(p.timeConnected).Seconds(),
	}
}

// SetHashContinue stores the last block hash from an inventory batch (for pipelining)
func (p *Peer) SetHashContinue(hash types.Hash) {
	p.hashContinueMu.Lock()
	defer p.hashContinueMu.Unlock()
	p.hashContinue = &hash
}

// GetHashContinue retrieves the stored hashContinue value
func (p *Peer) GetHashContinue() *types.Hash {
	p.hashContinueMu.Lock()
	defer p.hashContinueMu.Unlock()
	return p.hashContinue
}

// ClearHashContinue clears the stored hashContinue value
func (p *Peer) ClearHashContinue() {
	p.hashContinueMu.Lock()
	defer p.hashContinueMu.Unlock()
	p.hashContinue = nil
}

// String returns a string representation of the peer
func (p *Peer) String() string {
	direction := "outbound"
	if p.inbound {
		direction = "inbound"
	}
	return fmt.Sprintf("Peer{%s, %s, connected: %v}",
		p.addr.String(), direction, p.connected.Load())
}

// AddMisbehavior adds points to the peer's misbehavior score.
// Returns the new total score (for threshold checking by caller).
// Legacy: implements TWINS Misbehaving() function from main.cpp
func (p *Peer) AddMisbehavior(howmuch int32) int32 {
	newScore := p.misbehaviorScore.Add(howmuch)
	p.logger.WithFields(logrus.Fields{
		"added":     howmuch,
		"new_score": newScore,
	}).Debug("Peer misbehavior score increased")
	return newScore
}

// GetMisbehaviorScore returns the current misbehavior score
func (p *Peer) GetMisbehaviorScore() int32 {
	return p.misbehaviorScore.Load()
}

// ResetMisbehaviorScore resets the misbehavior score to zero
func (p *Peer) ResetMisbehaviorScore() {
	p.misbehaviorScore.Store(0)
}

// SupportsProto70928 returns true if the peer's protocol version is >= 70928.
// Used to gate extended ping/pong, inv-with-height, and getchainstate features.
func (p *Peer) SupportsProto70928() bool {
	p.mu.RLock()
	v := p.version
	p.mu.RUnlock()
	return v != nil && v.Version >= 70928
}

// GetPeerHeight returns the latest height reported by this peer via extended ping/pong.
func (p *Peer) GetPeerHeight() uint32 {
	return p.peerHeight.Load()
}

// SetPeerHeight updates the peer's reported height.
func (p *Peer) SetPeerHeight(height uint32) {
	p.peerHeight.Store(height)
}

// SetCurrentHeight sets our local chain height on this peer (sent in pings).
func (p *Peer) SetCurrentHeight(height uint32) {
	p.currentHeight.Store(height)
}

// SetHealthTracker sets the health tracker reference for EffectivePeerHeight().
// Called by the syncer when the peer is discovered.
func (p *Peer) SetHealthTracker(ht *PeerHealthTracker) {
	p.healthTracker.Store(ht)
}

// EffectivePeerHeight returns the best available height estimate for this peer.
//
// For 70928 peers: max(SyncedHeaders, GetPeerHeight())
//   - GetPeerHeight() is initialized from StartHeight at handshake and updated via ping/pong (~30s)
//   - SyncedHeaders (BestKnownHeight) is updated when peer sends inv/headers
//   - StartHeight is omitted because GetPeerHeight() >= StartHeight by construction
//
// For 70927 peers: max(SyncedHeaders, StartHeight)
//   - SyncedHeaders may be 0 if peer hasn't sent inv/headers yet
//   - StartHeight is the floor from the version handshake
func (p *Peer) EffectivePeerHeight() uint32 {
	var syncedHeaders uint32
	if ht := p.healthTracker.Load(); ht != nil {
		syncedHeaders = ht.GetBestKnownHeight(p.GetAddress().String())
	}

	if p.SupportsProto70928() {
		pingHeight := p.GetPeerHeight()
		if pingHeight > 0 {
			if syncedHeaders > pingHeight {
				return syncedHeaders
			}
			return pingHeight
		}
	}

	startHeight := uint32(p.startHeight)
	if syncedHeaders > startHeight {
		return syncedHeaders
	}
	return startHeight
}
