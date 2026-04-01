//go:build windows

package p2p

import "syscall"

const (
	soSOCKET = syscall.SOL_SOCKET
	soRCVBUF = syscall.SO_RCVBUF
	soSNDBUF = syscall.SO_SNDBUF
)

func setsockoptInt(fd uintptr, level, opt, value int) error {
	return syscall.SetsockoptInt(syscall.Handle(fd), level, opt, value)
}
