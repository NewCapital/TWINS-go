package masternode

import (
	"fmt"
	"net"
)

// parseTCPAddr parses a TCP address string
func parseTCPAddr(addr string) (*net.TCPAddr, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("invalid TCP address: %w", err)
	}

	return tcpAddr, nil
}