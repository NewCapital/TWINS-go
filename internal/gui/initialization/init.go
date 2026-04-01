package initialization

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Service handles wallet initialization tasks
type Service struct {
	config    *TWINSConfig
	validator *NetworkValidator
}

// NewService creates a new initialization service
func NewService() *Service {
	return &Service{}
}

// InitializationConfig holds the initialization settings
type InitializationConfig struct {
	DataDirectory string `json:"dataDirectory"`
	UseDefault    bool   `json:"useDefault"`
}

// DiskSpaceInfo contains disk space information
type DiskSpaceInfo struct {
	Available uint64 `json:"available"` // Available space in bytes
	Required  uint64 `json:"required"`  // Required space in bytes (1GB)
	HasSpace  bool   `json:"hasSpace"`  // Whether there's enough space
}

// GetDefaultDataDirectory returns the default TWINS data directory
func (s *Service) GetDefaultDataDirectory() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	switch goruntime.GOOS {
	case "windows":
		return filepath.Join(homeDir, "AppData", "Roaming", "TWINS")
	case "darwin":
		return filepath.Join(homeDir, "Library", "Application Support", "TWINS")
	default: // Linux and others
		return filepath.Join(homeDir, ".twins")
	}
}

// SelectDataDirectory opens a directory selection dialog
func (s *Service) SelectDataDirectory(ctx context.Context) (string, error) {
	options := wailsruntime.OpenDialogOptions{
		Title: "Select TWINS Data Directory",
	}

	selectedDir, err := wailsruntime.OpenDirectoryDialog(ctx, options)
	if err != nil {
		return "", fmt.Errorf("failed to open directory dialog: %w", err)
	}

	if selectedDir == "" {
		return "", fmt.Errorf("no directory selected")
	}

	return selectedDir, nil
}

// CheckDiskSpace checks if the specified directory has enough space
func (s *Service) CheckDiskSpace(directory string) (*DiskSpaceInfo, error) {
	// Ensure directory exists or parent directory exists
	dir := directory
	for {
		if _, err := os.Stat(dir); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, fmt.Errorf("cannot find valid directory")
		}
		dir = parent
	}

	// Get actual disk space using platform-specific implementation
	available, _, err := getDiskSpace(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to get disk space: %w", err)
	}

	// Check if we have at least 1GB of free space
	requiredSpace := uint64(1 * 1024 * 1024 * 1024) // 1GB in bytes

	info := &DiskSpaceInfo{
		Available: available,
		Required:  requiredSpace,
		HasSpace:  available >= requiredSpace,
	}

	// Basic check - ensure directory is writable
	stat, err := os.Stat(dir)
	if err != nil {
		return info, fmt.Errorf("failed to stat directory: %w", err)
	}

	if stat.IsDir() {
		testFile := filepath.Join(dir, ".twins_test")
		if f, err := os.Create(testFile); err == nil {
			f.Close()
			os.Remove(testFile)
		} else {
			info.HasSpace = false
			return info, fmt.Errorf("directory is not writable: %w", err)
		}
	}

	return info, nil
}

// ValidateDataDirectory validates the selected data directory
func (s *Service) ValidateDataDirectory(directory string) error {
	if directory == "" {
		return fmt.Errorf("directory path cannot be empty")
	}

	// Check if path is absolute
	if !filepath.IsAbs(directory) {
		return fmt.Errorf("directory path must be absolute")
	}

	// Check disk space
	spaceInfo, err := s.CheckDiskSpace(directory)
	if err != nil {
		return fmt.Errorf("failed to check disk space: %w", err)
	}

	if !spaceInfo.HasSpace {
		return fmt.Errorf("insufficient disk space: need at least 1GB")
	}

	return nil
}

// InitializeDataDirectory creates the data directory structure
// Flat structure - all files directly in datadir:
//
//	~/.twins/               (base directory = "directory" parameter)
//	├── twinsd.yml          # config file (YAML, recommended)
//	├── blockchain.db/      # Pebble database (created by storage layer)
//	├── wallet.dat          # wallet file
//	├── txcache.dat         # transaction cache
//	├── peers.json          # peer addresses
//	├── mncache.dat         # masternode cache
//	└── backups/            # backup files
func (s *Service) InitializeDataDirectory(directory string) error {
	// Create main directory
	if err := os.MkdirAll(directory, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Create backups subdirectory only
	backupsPath := filepath.Join(directory, "backups")
	if err := os.MkdirAll(backupsPath, 0755); err != nil {
		return fmt.Errorf("failed to create backups directory: %w", err)
	}

	// twinsd.yml creation is handled by ConfigManager.LoadOrCreate()
	// which generates a well-commented file from the registry defaults.

	return nil
}

// VerifyBlockchainIntegrity performs blockchain verification
func (s *Service) VerifyBlockchainIntegrity(dataDir string) (*BlockchainInfo, error) {
	network := "mainnet"
	if s.config != nil {
		if s.config.Testnet {
			network = "testnet"
		} else if s.config.Regtest {
			network = "regtest"
		}
	}

	info, err := VerifyBlockchain(dataDir, network)
	if err != nil {
		return nil, fmt.Errorf("blockchain verification failed: %w", err)
	}

	return info, nil
}

// CheckNetworkConnection validates network connectivity
func (s *Service) CheckNetworkConnection() error {
	if s.validator == nil {
		s.validator = NewNetworkValidator("mainnet")
	}

	if err := s.validator.CheckNetworkConnectivity(); err != nil {
		return fmt.Errorf("network connectivity check failed: %w", err)
	}

	return nil
}

// GetConfiguration returns the loaded configuration
func (s *Service) GetConfiguration() *TWINSConfig {
	return s.config
}

// GetNetworkValidator returns the network validator
func (s *Service) GetNetworkValidator() *NetworkValidator {
	return s.validator
}

