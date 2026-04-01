package wallet

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// BackupManager handles automatic wallet backups (legacy: -createwalletbackups)
type BackupManager struct {
	walletPath string         // Path to wallet.dat
	backupDir  string         // Directory for backups
	maxBackups int            // Maximum number of backups to keep (0 = disabled)
	logger     *logrus.Entry // Logger for non-fatal warnings
}

// NewBackupManager creates a new backup manager
func NewBackupManager(walletPath, backupDir string, maxBackups int, logger *logrus.Entry) *BackupManager {
	return &BackupManager{
		walletPath: walletPath,
		backupDir:  backupDir,
		maxBackups: maxBackups,
		logger:     logger,
	}
}

// CreateBackup creates a timestamped backup of the wallet file
// Returns the path to the created backup or error
func (bm *BackupManager) CreateBackup() (string, error) {
	if bm.maxBackups <= 0 {
		return "", nil // Backups disabled
	}

	// Ensure backup directory exists
	if err := os.MkdirAll(bm.backupDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Check if wallet file exists
	if _, err := os.Stat(bm.walletPath); os.IsNotExist(err) {
		return "", fmt.Errorf("wallet file does not exist: %s", bm.walletPath)
	}

	// Generate backup filename with timestamp
	// Format: wallet.dat.YYYY-MM-DD-HHMMSS
	timestamp := time.Now().Format("2006-01-02-150405")
	backupName := fmt.Sprintf("wallet.dat.%s", timestamp)
	backupPath := filepath.Join(bm.backupDir, backupName)

	// Copy wallet file to backup location (atomic operation via copyFile)
	// Note: copyFile already sets 0600 permissions atomically via temp file
	if err := copyFile(bm.walletPath, backupPath); err != nil {
		return "", fmt.Errorf("failed to create backup: %w", err)
	}

	// Prune old backups if we exceed the limit
	if err := bm.pruneOldBackups(); err != nil {
		// Non-fatal, just log
		if bm.logger != nil {
			bm.logger.Warnf("Failed to prune old backups: %v", err)
		}
	}

	return backupPath, nil
}

// pruneOldBackups removes oldest backups if we exceed maxBackups
func (bm *BackupManager) pruneOldBackups() error {
	if bm.maxBackups <= 0 {
		return nil
	}

	// List all backup files
	backups, err := bm.ListBackups()
	if err != nil {
		return err
	}

	// If we're within limit, nothing to do
	if len(backups) <= bm.maxBackups {
		return nil
	}

	// Sort by modification time (oldest first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].ModTime.Before(backups[j].ModTime)
	})

	// Remove oldest backups until we're at the limit
	toRemove := len(backups) - bm.maxBackups
	for i := 0; i < toRemove; i++ {
		if err := os.Remove(backups[i].Path); err != nil {
			return fmt.Errorf("failed to remove old backup %s: %w", backups[i].Path, err)
		}
	}

	return nil
}

// BackupInfo contains information about a backup file
type BackupInfo struct {
	Path    string
	Name    string
	Size    int64
	ModTime time.Time
}

// ListBackups returns a list of all wallet backups
func (bm *BackupManager) ListBackups() ([]BackupInfo, error) {
	var backups []BackupInfo

	// Check if backup directory exists
	if _, err := os.Stat(bm.backupDir); os.IsNotExist(err) {
		return backups, nil // No backups yet
	}

	entries, err := os.ReadDir(bm.backupDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read backup directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Match wallet.dat.* pattern
		if !strings.HasPrefix(entry.Name(), "wallet.dat.") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		backups = append(backups, BackupInfo{
			Path:    filepath.Join(bm.backupDir, entry.Name()),
			Name:    entry.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}

	return backups, nil
}

// RestoreBackup restores a wallet from a backup file
// WARNING: This overwrites the current wallet file!
func (bm *BackupManager) RestoreBackup(backupPath string) error {
	// Verify backup exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("backup file does not exist: %s", backupPath)
	}

	// Create a backup of current wallet before restoring (safety measure)
	if _, err := os.Stat(bm.walletPath); err == nil {
		safetyBackup := bm.walletPath + ".pre-restore"
		if err := copyFile(bm.walletPath, safetyBackup); err != nil {
			return fmt.Errorf("failed to create safety backup: %w", err)
		}
	}

	// Copy backup to wallet location
	if err := copyFile(backupPath, bm.walletPath); err != nil {
		return fmt.Errorf("failed to restore backup: %w", err)
	}

	return nil
}

// GetBackupDir returns the configured backup directory
func (bm *BackupManager) GetBackupDir() string {
	return bm.backupDir
}

// GetMaxBackups returns the maximum number of backups
func (bm *BackupManager) GetMaxBackups() int {
	return bm.maxBackups
}

// SetMaxBackups updates the maximum number of backups
func (bm *BackupManager) SetMaxBackups(max int) {
	bm.maxBackups = max
}

// copyFile copies a file from src to dst atomically with secure permissions
// Uses write-to-temp + rename pattern to prevent partial writes on failure
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// Create temp file in same directory as destination for atomic rename
	// (rename across filesystems is not atomic)
	dstDir := filepath.Dir(dst)
	tempFile, err := os.CreateTemp(dstDir, ".wallet-backup-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tempFile.Name()

	// Cleanup temp file on any error
	success := false
	defer func() {
		if !success {
			os.Remove(tempPath)
		}
	}()

	// Copy contents to temp file first
	if _, err := io.Copy(tempFile, sourceFile); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to copy data: %w", err)
	}

	// Set secure permissions AFTER successful copy (more logical order)
	if err := tempFile.Chmod(0600); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to set permissions on temp file: %w", err)
	}

	// Ensure data is written to disk before rename
	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	// Close temp file before rename (required on Windows)
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename - this is the commit point
	// Either the old file exists or the new one does, never a partial state
	if err := os.Rename(tempPath, dst); err != nil {
		return fmt.Errorf("failed to rename temp file to destination: %w", err)
	}

	success = true
	return nil
}

// BackupOnModification should be called after wallet modifications
// to create automatic backups if enabled
func (w *Wallet) BackupOnModification() error {
	if w.config.CreateWalletBackups <= 0 {
		return nil // Backups disabled
	}

	// Determine backup directory
	backupDir := w.config.BackupPath
	if backupDir == "" {
		// Default: <wallet_dir>/backups
		backupDir = filepath.Join(w.config.DataDir, "backups")
	}

	walletPath := filepath.Join(w.config.DataDir, "wallet.dat")
	bm := NewBackupManager(walletPath, backupDir, w.config.CreateWalletBackups, w.logger)

	backupPath, err := bm.CreateBackup()
	if err != nil {
		return fmt.Errorf("failed to create automatic backup: %w", err)
	}

	if backupPath != "" {
		w.logger.WithField("path", backupPath).Debug("Created automatic wallet backup")
	}

	return nil
}

// GetBackupManager returns a backup manager for this wallet
func (w *Wallet) GetBackupManager() *BackupManager {
	backupDir := w.config.BackupPath
	if backupDir == "" {
		backupDir = filepath.Join(w.config.DataDir, "backups")
	}

	walletPath := filepath.Join(w.config.DataDir, "wallet.dat")
	return NewBackupManager(walletPath, backupDir, w.config.CreateWalletBackups, w.logger)
}
