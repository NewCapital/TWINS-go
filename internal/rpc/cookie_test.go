// Copyright (c) 2025 The TWINS Core developers
// Distributed under the MIT software license

package rpc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateCookie(t *testing.T) {
	cookie, err := GenerateCookie()
	if err != nil {
		t.Fatalf("Failed to generate cookie: %v", err)
	}

	if cookie.Username == "" {
		t.Error("Cookie username is empty")
	}

	if cookie.Password == "" {
		t.Error("Cookie password is empty")
	}

	if len(cookie.Password) < 16 {
		t.Errorf("Cookie password too short: %d characters", len(cookie.Password))
	}

	// Verify cookie validates
	if err := cookie.Validate(); err != nil {
		t.Errorf("Generated cookie failed validation: %v", err)
	}
}

func TestWriteAndReadCookieFile(t *testing.T) {
	// Create temp directory
	tempDir := t.TempDir()

	// Generate cookie
	cookie, err := GenerateCookie()
	if err != nil {
		t.Fatalf("Failed to generate cookie: %v", err)
	}

	// Write cookie file
	if err := WriteCookieFile(tempDir, cookie); err != nil {
		t.Fatalf("Failed to write cookie file: %v", err)
	}

	// Verify file exists
	cookiePath := filepath.Join(tempDir, CookieFilename)
	if _, err := os.Stat(cookiePath); os.IsNotExist(err) {
		t.Fatal("Cookie file was not created")
	}

	// Read cookie file
	readCookie, err := ReadCookieFile(tempDir)
	if err != nil {
		t.Fatalf("Failed to read cookie file: %v", err)
	}

	// Verify contents match
	if readCookie.Username != cookie.Username {
		t.Errorf("Username mismatch: expected %s, got %s", cookie.Username, readCookie.Username)
	}

	if readCookie.Password != cookie.Password {
		t.Errorf("Password mismatch: expected %s, got %s", cookie.Password, readCookie.Password)
	}
}

func TestCookieFilePermissions(t *testing.T) {
	tempDir := t.TempDir()

	cookie, err := GenerateCookie()
	if err != nil {
		t.Fatalf("Failed to generate cookie: %v", err)
	}

	if err := WriteCookieFile(tempDir, cookie); err != nil {
		t.Fatalf("Failed to write cookie file: %v", err)
	}

	cookiePath := filepath.Join(tempDir, CookieFilename)
	fileInfo, err := os.Stat(cookiePath)
	if err != nil {
		t.Fatalf("Failed to stat cookie file: %v", err)
	}

	// Check permissions (should be 0600)
	mode := fileInfo.Mode()
	expected := os.FileMode(0600)
	if mode != expected {
		t.Errorf("Cookie file has incorrect permissions: expected %o, got %o", expected, mode)
	}
}

func TestDeleteCookieFile(t *testing.T) {
	tempDir := t.TempDir()

	cookie, err := GenerateCookie()
	if err != nil {
		t.Fatalf("Failed to generate cookie: %v", err)
	}

	// Write cookie
	if err := WriteCookieFile(tempDir, cookie); err != nil {
		t.Fatalf("Failed to write cookie file: %v", err)
	}

	// Verify it exists
	if !CookieExists(tempDir) {
		t.Fatal("Cookie file does not exist after writing")
	}

	// Delete cookie
	if err := DeleteCookieFile(tempDir); err != nil {
		t.Fatalf("Failed to delete cookie file: %v", err)
	}

	// Verify it's gone
	if CookieExists(tempDir) {
		t.Error("Cookie file still exists after deletion")
	}

	// Deleting non-existent cookie should not error
	if err := DeleteCookieFile(tempDir); err != nil {
		t.Errorf("Deleting non-existent cookie returned error: %v", err)
	}
}

func TestCookieExists(t *testing.T) {
	tempDir := t.TempDir()

	// Should not exist initially
	if CookieExists(tempDir) {
		t.Error("Cookie exists in empty directory")
	}

	// Write cookie
	cookie, _ := GenerateCookie()
	WriteCookieFile(tempDir, cookie)

	// Should exist now
	if !CookieExists(tempDir) {
		t.Error("Cookie does not exist after writing")
	}
}

func TestReadCookieFile_NotExists(t *testing.T) {
	tempDir := t.TempDir()

	_, err := ReadCookieFile(tempDir)
	if err == nil {
		t.Error("Expected error when reading non-existent cookie file")
	}
}

func TestReadCookieFile_InvalidFormat(t *testing.T) {
	tempDir := t.TempDir()

	// Write invalid cookie content (missing colon)
	cookiePath := filepath.Join(tempDir, CookieFilename)
	os.WriteFile(cookiePath, []byte("invalid_no_colon\n"), 0600)

	_, err := ReadCookieFile(tempDir)
	if err == nil {
		t.Error("Expected error when reading invalid cookie format")
	}
}

func TestCookieValidation(t *testing.T) {
	tests := []struct {
		name      string
		cookie    *Cookie
		wantError bool
	}{
		{
			name:      "valid cookie",
			cookie:    &Cookie{Username: "test", Password: "1234567890123456"},
			wantError: false,
		},
		{
			name:      "empty username",
			cookie:    &Cookie{Username: "", Password: "1234567890123456"},
			wantError: true,
		},
		{
			name:      "empty password",
			cookie:    &Cookie{Username: "test", Password: ""},
			wantError: true,
		},
		{
			name:      "password too short",
			cookie:    &Cookie{Username: "test", Password: "short"},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cookie.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestGetCookiePath(t *testing.T) {
	dataDir := "/test/data"
	expected := filepath.Join(dataDir, CookieFilename)
	got := GetCookiePath(dataDir)

	if got != expected {
		t.Errorf("GetCookiePath() = %s, want %s", got, expected)
	}
}

func TestCookieString(t *testing.T) {
	cookie := &Cookie{
		Username: "testuser",
		Password: "secretpassword123",
	}

	str := cookie.String()

	// Should contain username but not password
	if !contains(str, "testuser") {
		t.Error("Cookie string does not contain username")
	}

	if contains(str, "secretpassword123") {
		t.Error("Cookie string exposes password")
	}

	if !contains(str, "redacted") {
		t.Error("Cookie string does not indicate password is redacted")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}