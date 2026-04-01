//go:build !windows
// +build !windows

package initialization

import (
	"golang.org/x/sys/unix"
)

// getDiskSpace returns the available disk space for the given path on Unix-like systems
func getDiskSpace(path string) (available, total uint64, err error) {
	var stat unix.Statfs_t

	if err := unix.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}

	// Available blocks * block size = available bytes
	available = stat.Bavail * uint64(stat.Bsize)

	// Total blocks * block size = total bytes
	total = stat.Blocks * uint64(stat.Bsize)

	return available, total, nil
}