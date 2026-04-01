package preferences

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// Preferences holds the application preferences
type Preferences struct {
	DataDirectory string `json:"dataDirectory"`
	LastUsed      string `json:"lastUsed"`
}

// Service manages application preferences
type Service struct {
	mu           sync.RWMutex
	preferences  *Preferences
	prefsPath    string
}

// NewService creates a new preferences service
func NewService() (*Service, error) {
	prefsPath := getPreferencesPath()

	// Ensure the preferences directory exists
	prefsDir := filepath.Dir(prefsPath)
	if err := os.MkdirAll(prefsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create preferences directory: %w", err)
	}

	service := &Service{
		prefsPath: prefsPath,
	}

	// Load existing preferences if they exist
	if err := service.Load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load preferences: %w", err)
	}

	if service.preferences == nil {
		service.preferences = &Preferences{}
	}

	return service, nil
}

// getPreferencesPath returns the path to the preferences file
func getPreferencesPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}

	switch runtime.GOOS {
	case "windows":
		return filepath.Join(homeDir, "AppData", "Local", "TWINS-Wallet", "preferences.json")
	case "darwin":
		return filepath.Join(homeDir, "Library", "Preferences", "com.twins.wallet", "preferences.json")
	default: // Linux and others
		configDir := os.Getenv("XDG_CONFIG_HOME")
		if configDir == "" {
			configDir = filepath.Join(homeDir, ".config")
		}
		return filepath.Join(configDir, "twins-wallet", "preferences.json")
	}
}

// Load reads preferences from disk
func (s *Service) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.prefsPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No preferences file yet, that's okay
			s.preferences = &Preferences{}
			return nil
		}
		return err
	}

	var prefs Preferences
	if err := json.Unmarshal(data, &prefs); err != nil {
		return fmt.Errorf("failed to parse preferences: %w", err)
	}

	s.preferences = &prefs
	return nil
}

// Save writes preferences to disk
func (s *Service) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.saveLocked()
}

// saveLocked writes preferences to disk (caller must hold lock)
func (s *Service) saveLocked() error {
	if s.preferences == nil {
		return fmt.Errorf("no preferences to save")
	}

	data, err := json.MarshalIndent(s.preferences, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal preferences: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(s.prefsPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create preferences directory: %w", err)
	}

	// Write atomically with secure permissions (0600 = owner-only)
	tempPath := s.prefsPath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write preferences: %w", err)
	}

	if err := os.Rename(tempPath, s.prefsPath); err != nil {
		os.Remove(tempPath) // Clean up temp file
		return fmt.Errorf("failed to save preferences: %w", err)
	}

	return nil
}

// GetDataDirectory returns the stored data directory
func (s *Service) GetDataDirectory() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.preferences == nil {
		return ""
	}

	return s.preferences.DataDirectory
}

// SetDataDirectory sets and saves the data directory
func (s *Service) SetDataDirectory(dir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.preferences == nil {
		s.preferences = &Preferences{}
	}
	s.preferences.DataDirectory = dir

	// Save immediately after setting (keeps lock throughout)
	return s.saveLocked()
}

// HasDataDirectory checks if a data directory has been set
func (s *Service) HasDataDirectory() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.preferences != nil && s.preferences.DataDirectory != ""
}

// Reset clears all preferences
func (s *Service) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.preferences = &Preferences{}

	// Remove the preferences file
	if err := os.Remove(s.prefsPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove preferences file: %w", err)
	}

	return nil
}

// GetPreferencesPath returns the path to the preferences file
func (s *Service) GetPreferencesPath() string {
	return s.prefsPath
}