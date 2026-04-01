// Copyright (c) 2025 The TWINS Core developers
// Distributed under the MIT software license

package rpc

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// CookieFilename is the name of the RPC authentication cookie file
	CookieFilename = ".cookie"

	// CookieSize is the number of random bytes in the cookie
	CookieSize = 32
)

// Cookie represents an RPC authentication cookie
type Cookie struct {
	Username string
	Password string
}

// GenerateCookie generates a random RPC authentication cookie
func GenerateCookie() (*Cookie, error) {
	// Generate random bytes for password
	randomBytes := make([]byte, CookieSize)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random cookie: %w", err)
	}

	// Use hex encoding for password
	password := hex.EncodeToString(randomBytes)

	return &Cookie{
		Username: "__cookie__",
		Password: password,
	}, nil
}

// WriteCookieFile writes the cookie to a file
func WriteCookieFile(dataDir string, cookie *Cookie) error {
	cookiePath := filepath.Join(dataDir, CookieFilename)

	// Create cookie content: username:password
	content := fmt.Sprintf("%s:%s\n", cookie.Username, cookie.Password)

	// Write with restricted permissions (0600 = read/write for owner only)
	if err := os.WriteFile(cookiePath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write cookie file: %w", err)
	}

	return nil
}

// ReadCookieFile reads the cookie from a file
func ReadCookieFile(dataDir string) (*Cookie, error) {
	cookiePath := filepath.Join(dataDir, CookieFilename)

	// Read cookie file
	content, err := os.ReadFile(cookiePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("cookie file does not exist")
		}
		return nil, fmt.Errorf("failed to read cookie file: %w", err)
	}

	// Parse cookie content: username:password
	parts := strings.SplitN(strings.TrimSpace(string(content)), ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid cookie file format")
	}

	return &Cookie{
		Username: parts[0],
		Password: parts[1],
	}, nil
}

// DeleteCookieFile removes the cookie file
func DeleteCookieFile(dataDir string) error {
	cookiePath := filepath.Join(dataDir, CookieFilename)

	if err := os.Remove(cookiePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete cookie file: %w", err)
	}

	return nil
}

// GetCookiePath returns the path to the cookie file
func GetCookiePath(dataDir string) string {
	return filepath.Join(dataDir, CookieFilename)
}

// CookieExists checks if a cookie file exists
func CookieExists(dataDir string) bool {
	cookiePath := filepath.Join(dataDir, CookieFilename)
	_, err := os.Stat(cookiePath)
	return err == nil
}

// String returns a string representation of the cookie (for logging - password redacted)
func (c *Cookie) String() string {
	return fmt.Sprintf("Cookie{Username: %s, Password: <redacted>}", c.Username)
}

// Validate checks if the cookie has valid values
func (c *Cookie) Validate() error {
	if c.Username == "" {
		return fmt.Errorf("cookie username is empty")
	}
	if c.Password == "" {
		return fmt.Errorf("cookie password is empty")
	}
	if len(c.Password) < 16 {
		return fmt.Errorf("cookie password too short (minimum 16 characters)")
	}
	return nil
}