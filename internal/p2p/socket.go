package p2p

import (
	"net"

	"github.com/sirupsen/logrus"
)

// SocketOptions contains socket configuration options
type SocketOptions struct {
	MaxReceiveBuffer int // Receive buffer size in bytes (legacy: -maxreceivebuffer * 1000)
	MaxSendBuffer    int // Send buffer size in bytes (legacy: -maxsendbuffer * 1000)
}

// DefaultSocketOptions returns default socket options matching legacy C++ defaults
func DefaultSocketOptions() *SocketOptions {
	return &SocketOptions{
		MaxReceiveBuffer: 5000 * 1000,  // 5MB (legacy default: 5000 * 1000)
		MaxSendBuffer:    1000 * 1000,  // 1MB (legacy default: 1000 * 1000)
	}
}

// ApplySocketOptions applies buffer size options to a TCP connection
// This matches the legacy C++ SetSocketOptions behavior
func ApplySocketOptions(conn net.Conn, opts *SocketOptions, logger *logrus.Entry) {
	if opts == nil {
		return
	}

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		if logger != nil {
			logger.Debug("Connection is not TCP, skipping socket options")
		}
		return
	}

	// Get raw socket file descriptor
	rawConn, err := tcpConn.SyscallConn()
	if err != nil {
		if logger != nil {
			logger.WithError(err).Debug("Failed to get raw connection for socket options")
		}
		return
	}

	var setErr error
	err = rawConn.Control(func(fd uintptr) {
		// Set receive buffer size (SO_RCVBUF)
		if opts.MaxReceiveBuffer > 0 {
			if err := setsockoptInt(fd, soSOCKET, soRCVBUF, opts.MaxReceiveBuffer); err != nil {
				setErr = err
				if logger != nil {
					logger.WithError(err).WithField("size", opts.MaxReceiveBuffer).Debug("Failed to set SO_RCVBUF")
				}
			} else if logger != nil {
				logger.WithField("size", opts.MaxReceiveBuffer).Debug("Set SO_RCVBUF")
			}
		}

		// Set send buffer size (SO_SNDBUF)
		if opts.MaxSendBuffer > 0 {
			if err := setsockoptInt(fd, soSOCKET, soSNDBUF, opts.MaxSendBuffer); err != nil {
				if setErr == nil {
					setErr = err
				}
				if logger != nil {
					logger.WithError(err).WithField("size", opts.MaxSendBuffer).Debug("Failed to set SO_SNDBUF")
				}
			} else if logger != nil {
				logger.WithField("size", opts.MaxSendBuffer).Debug("Set SO_SNDBUF")
			}
		}
	})

	if err != nil && logger != nil {
		logger.WithError(err).Debug("Failed to control raw connection")
	}
}

// SetTCPNoDelay enables TCP_NODELAY on the connection (disable Nagle's algorithm)
func SetTCPNoDelay(conn net.Conn, enable bool) error {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil
	}
	return tcpConn.SetNoDelay(enable)
}

// SetTCPKeepAlive enables TCP keepalive on the connection
func SetTCPKeepAlive(conn net.Conn, enable bool) error {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil
	}
	return tcpConn.SetKeepAlive(enable)
}
