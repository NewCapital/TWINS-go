//go:build cgo

package cgo

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// embeddedLibDB and embeddedLibName are defined in platform-specific embed_*.go files
// using build tags. Each platform only includes its own library binary.

var (
	// Track if library is already loaded
	libLoaded    bool
	libLoadMutex sync.Mutex
	libPath      string
)

// InitEmbeddedLibrary extracts and loads the embedded BerkeleyDB library
func InitEmbeddedLibrary() (string, error) {
	libLoadMutex.Lock()
	defer libLoadMutex.Unlock()

	// Return if already loaded
	if libLoaded {
		return libPath, nil
	}

	// Use platform-specific embedded library (from embed_*.go files)
	libData := embeddedLibDB
	libName := embeddedLibName

	// Check if library data was actually embedded
	if len(libData) == 0 {
		return "", fmt.Errorf("BerkeleyDB library not embedded for %s_%s - legacy wallet migration unavailable on this platform", runtime.GOOS, runtime.GOARCH)
	}

	// Create temporary directory for the library
	tmpDir := filepath.Join(os.TempDir(), "twins-bdb-lib")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Write the library to temp directory
	libPath = filepath.Join(tmpDir, libName)

	// Check if library already exists and has correct size
	if info, err := os.Stat(libPath); err == nil && info.Size() == int64(len(libData)) {
		// Library already extracted
		libLoaded = true
		return libPath, nil
	}

	// Write the library file
	if err := os.WriteFile(libPath, libData, 0755); err != nil {
		return "", fmt.Errorf("failed to write library file: %w", err)
	}

	// On macOS, we need to set the library path before any dlopen calls
	// Note: This won't affect the current process, but we keep it for child processes
	if runtime.GOOS == "darwin" {
		// Set DYLD_LIBRARY_PATH to include our temp directory
		currentPath := os.Getenv("DYLD_LIBRARY_PATH")
		if currentPath != "" {
			os.Setenv("DYLD_LIBRARY_PATH", tmpDir+":"+currentPath)
		} else {
			os.Setenv("DYLD_LIBRARY_PATH", tmpDir)
		}
	}

	libLoaded = true
	return libPath, nil
}

// GetLibraryPath returns the path to the extracted library
func GetLibraryPath() string {
	return libPath
}

// CleanupLibrary removes the extracted library (optional cleanup)
func CleanupLibrary() error {
	libLoadMutex.Lock()
	defer libLoadMutex.Unlock()

	if libPath != "" {
		if err := os.Remove(libPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		libPath = ""
		libLoaded = false
	}
	return nil
}