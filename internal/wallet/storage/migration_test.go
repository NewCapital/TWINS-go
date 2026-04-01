package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectWalletFormat(t *testing.T) {
	t.Run("detect_berkeleydb", func(t *testing.T) {
		// Create temporary file with BerkeleyDB magic
		tmpfile, err := os.CreateTemp("", "wallet-*.dat")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpfile.Name())

		// Write BerkeleyDB header
		header := make([]byte, 512)
		copy(header[12:16], berkeleyMagic)
		tmpfile.Write(header)
		tmpfile.Close()

		format, err := DetectWalletFormat(tmpfile.Name())
		if err != nil {
			t.Fatalf("DetectWalletFormat failed: %v", err)
		}

		if format != FormatLegacyBerkeleyDB {
			t.Errorf("Expected FormatLegacyBerkeleyDB, got %v", format)
		}
	})

	t.Run("detect_bbolt", func(t *testing.T) {
		// Create temporary bbolt database
		tmpfile, err := os.CreateTemp("", "wallet-*.dat")
		if err != nil {
			t.Fatal(err)
		}
		tmpfile.Close()
		defer os.Remove(tmpfile.Name())

		// Create actual bbolt database
		entries := []WalletEntry{{Key: []byte("test"), Value: []byte("value")}}
		if err := writeBboltWallet(tmpfile.Name(), entries); err != nil {
			t.Fatal(err)
		}

		format, err := DetectWalletFormat(tmpfile.Name())
		if err != nil {
			t.Fatalf("DetectWalletFormat failed: %v", err)
		}

		if format != FormatBbolt {
			t.Errorf("Expected FormatBbolt, got %v", format)
		}
	})

	t.Run("detect_unknown", func(t *testing.T) {
		// Create temporary file with invalid magic
		tmpfile, err := os.CreateTemp("", "wallet-*.dat")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpfile.Name())

		// Write random data
		header := make([]byte, 512)
		for i := range header {
			header[i] = byte(i)
		}
		tmpfile.Write(header)
		tmpfile.Close()

		format, err := DetectWalletFormat(tmpfile.Name())
		if err != nil {
			t.Fatalf("DetectWalletFormat failed: %v", err)
		}

		if format != FormatUnknown {
			t.Errorf("Expected FormatUnknown, got %v", format)
		}
	})

	t.Run("file_too_small", func(t *testing.T) {
		// Create temporary file with insufficient data
		tmpfile, err := os.CreateTemp("", "wallet-*.dat")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpfile.Name())

		// Write only 4 bytes
		tmpfile.Write([]byte{0x01, 0x02, 0x03, 0x04})
		tmpfile.Close()

		_, err = DetectWalletFormat(tmpfile.Name())
		if err == nil {
			t.Error("Expected error for too small file")
		}
	})

	t.Run("nonexistent_file", func(t *testing.T) {
		_, err := DetectWalletFormat("/nonexistent/path/wallet.dat")
		if err == nil {
			t.Error("Expected error for nonexistent file")
		}
	})
}

func TestWalletEntry(t *testing.T) {
	t.Run("create_entries", func(t *testing.T) {
		entries := []WalletEntry{
			{Key: []byte("key1"), Value: []byte("value1")},
			{Key: []byte("key2"), Value: []byte("value2")},
			{Key: []byte("key3"), Value: []byte("value3")},
		}

		if len(entries) != 3 {
			t.Errorf("Expected 3 entries, got %d", len(entries))
		}

		for i, entry := range entries {
			if entry.Key == nil || entry.Value == nil {
				t.Errorf("Entry %d has nil key or value", i)
			}
		}
	})
}

func TestCopyFile(t *testing.T) {
	t.Run("successful_copy", func(t *testing.T) {
		// Create source file
		srcFile, err := os.CreateTemp("", "source-*.dat")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(srcFile.Name())

		testData := []byte("test wallet data")
		srcFile.Write(testData)
		srcFile.Close()

		// Copy to destination
		dstPath := srcFile.Name() + ".copy"
		defer os.Remove(dstPath)

		if err := copyFile(srcFile.Name(), dstPath); err != nil {
			t.Fatalf("copyFile failed: %v", err)
		}

		// Verify content
		dstData, err := os.ReadFile(dstPath)
		if err != nil {
			t.Fatalf("Failed to read destination: %v", err)
		}

		if !bytes.Equal(dstData, testData) {
			t.Error("Copied file content doesn't match original")
		}
	})

	t.Run("source_not_exists", func(t *testing.T) {
		err := copyFile("/nonexistent/source.dat", "/tmp/dest.dat")
		if err == nil {
			t.Error("Expected error for nonexistent source file")
		}
	})
}

func TestWriteBboltWallet(t *testing.T) {
	t.Run("write_entries", func(t *testing.T) {
		tmpfile, err := os.CreateTemp("", "wallet-*.dat")
		if err != nil {
			t.Fatal(err)
		}
		tmpfile.Close()
		defer os.Remove(tmpfile.Name())

		entries := []WalletEntry{
			{Key: []byte("key1"), Value: []byte("value1")},
			{Key: []byte("key2"), Value: []byte("value2")},
			{Key: []byte("key3"), Value: []byte("value3")},
		}

		if err := writeBboltWallet(tmpfile.Name(), entries); err != nil {
			t.Fatalf("writeBboltWallet failed: %v", err)
		}

		// Verify file was created and is bbolt format
		format, err := DetectWalletFormat(tmpfile.Name())
		if err != nil {
			t.Fatalf("DetectWalletFormat failed: %v", err)
		}

		if format != FormatBbolt {
			t.Errorf("Expected FormatBbolt, got %v", format)
		}
	})

	t.Run("empty_entries", func(t *testing.T) {
		tmpfile, err := os.CreateTemp("", "wallet-*.dat")
		if err != nil {
			t.Fatal(err)
		}
		tmpfile.Close()
		defer os.Remove(tmpfile.Name())

		entries := []WalletEntry{}

		if err := writeBboltWallet(tmpfile.Name(), entries); err != nil {
			t.Fatalf("writeBboltWallet failed: %v", err)
		}
	})
}

func TestVerifyMigration(t *testing.T) {
	t.Run("successful_verification", func(t *testing.T) {
		tmpfile, err := os.CreateTemp("", "wallet-*.dat")
		if err != nil {
			t.Fatal(err)
		}
		tmpfile.Close()
		defer os.Remove(tmpfile.Name())

		entries := []WalletEntry{
			{Key: []byte("key1"), Value: []byte("value1")},
			{Key: []byte("key2"), Value: []byte("value2")},
		}

		// Write entries
		if err := writeBboltWallet(tmpfile.Name(), entries); err != nil {
			t.Fatalf("writeBboltWallet failed: %v", err)
		}

		// Verify
		if err := verifyMigration(entries, tmpfile.Name()); err != nil {
			t.Errorf("verifyMigration failed: %v", err)
		}
	})

	t.Run("no_entries_migrated", func(t *testing.T) {
		tmpfile, err := os.CreateTemp("", "wallet-*.dat")
		if err != nil {
			t.Fatal(err)
		}
		tmpfile.Close()
		defer os.Remove(tmpfile.Name())

		original := []WalletEntry{
			{Key: []byte("key1"), Value: []byte("value1")},
			{Key: []byte("key2"), Value: []byte("value2")},
		}

		// Write zero entries - verifyMigration checks totalMigrated == 0
		if err := writeBboltWallet(tmpfile.Name(), []WalletEntry{}); err != nil {
			t.Fatalf("writeBboltWallet failed: %v", err)
		}

		// Verify should fail because no entries were migrated
		if err := verifyMigration(original, tmpfile.Name()); err == nil {
			t.Error("Expected verification to fail with no entries migrated")
		}
	})
}

func TestMigrationOptions(t *testing.T) {
	t.Run("default_options", func(t *testing.T) {
		opts := DefaultMigrationOptions()

		if opts == nil {
			t.Fatal("DefaultMigrationOptions returned nil")
		}

		if !opts.CreateBackup {
			t.Error("Expected CreateBackup to be true")
		}

		if opts.BackupSuffix == "" {
			t.Error("Expected non-empty BackupSuffix")
		}

		if !opts.Verify {
			t.Error("Expected Verify to be true")
		}

		if opts.Logger == nil {
			t.Error("Expected non-nil Logger")
		}
	})

	t.Run("custom_options", func(t *testing.T) {
		customCalled := false
		opts := &MigrationOptions{
			CreateBackup: false,
			BackupSuffix: ".custom",
			Verify:       false,
			Logger:       func(msg string) { customCalled = true },
		}

		opts.Logger("test")

		if !customCalled {
			t.Error("Custom logger was not called")
		}

		if opts.CreateBackup {
			t.Error("Expected CreateBackup to be false")
		}

		if opts.BackupSuffix != ".custom" {
			t.Errorf("Expected BackupSuffix '.custom', got %q", opts.BackupSuffix)
		}
	})
}

func TestAutoMigrateWallet(t *testing.T) {
	t.Run("no_wallet_exists", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "wallet-test-")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		// Should not error when no wallet exists
		if err := AutoMigrateWallet(tmpDir); err != nil {
			t.Errorf("AutoMigrateWallet failed with no wallet: %v", err)
		}
	})

	t.Run("bbolt_wallet_no_migration", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "wallet-test-")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		// Create bbolt wallet
		walletPath := filepath.Join(tmpDir, "wallet.dat")
		entries := []WalletEntry{{Key: []byte("test"), Value: []byte("value")}}
		if err := writeBboltWallet(walletPath, entries); err != nil {
			t.Fatal(err)
		}

		// Should not migrate bbolt wallet
		if err := AutoMigrateWallet(tmpDir); err != nil {
			t.Errorf("AutoMigrateWallet failed: %v", err)
		}

		// Verify still bbolt format
		format, err := DetectWalletFormat(walletPath)
		if err != nil {
			t.Fatal(err)
		}

		if format != FormatBbolt {
			t.Errorf("Expected wallet to remain FormatBbolt, got %v", format)
		}
	})
}
