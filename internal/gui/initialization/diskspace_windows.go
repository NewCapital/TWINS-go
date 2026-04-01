//go:build windows
// +build windows

package initialization

import (
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// getDiskSpace returns the available disk space for the given path on Windows
func getDiskSpace(path string) (available, total uint64, err error) {
	// Convert to absolute path if not already
	absPath, err := filepath.Abs(path)
	if err != nil {
		return 0, 0, err
	}

	// Convert to UTF16 for Windows API
	pathPtr, err := windows.UTF16PtrFromString(absPath)
	if err != nil {
		return 0, 0, err
	}

	var freeBytesAvailable, totalBytes, totalFreeBytes uint64

	// Call Windows API to get disk space
	err = windows.GetDiskFreeSpaceEx(
		pathPtr,
		(*uint64)(unsafe.Pointer(&freeBytesAvailable)),
		(*uint64)(unsafe.Pointer(&totalBytes)),
		(*uint64)(unsafe.Pointer(&totalFreeBytes)),
	)

	if err != nil {
		return 0, 0, err
	}

	return freeBytesAvailable, totalBytes, nil
}