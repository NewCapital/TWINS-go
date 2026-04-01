package binary

import (
	"encoding/binary"
	"fmt"

	"github.com/cockroachdb/pebble"
	"github.com/sirupsen/logrus"
	"github.com/twins-dev/twins-core/internal/storage"
)

// BinaryStorage implements the simplified storage interface
type BinaryStorage struct {
	db     *pebble.DB
	config *storage.StorageConfig
	logger *logrus.Logger
}

// noopLogger implements pebble.Logger to suppress Pebble's internal logs
type noopLogger struct{}

func (noopLogger) Infof(format string, args ...interface{})  {}
func (noopLogger) Fatalf(format string, args ...interface{}) { panic(fmt.Sprintf(format, args...)) }

// NewBinaryStorage creates a new binary storage instance
func NewBinaryStorage(config *storage.StorageConfig) (*BinaryStorage, error) {
	if err := storage.ValidateStorageConfig(config); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	// Configure Pebble optimized for blockchain workloads
	opts := &pebble.Options{
		Cache:                       pebble.NewCache(config.BlockCacheSize * 1024 * 1024),
		MemTableSize:                uint64(config.MemTableSize),
		MemTableStopWritesThreshold: config.WriteBuffer,
		MaxOpenFiles:                config.MaxOpenFiles,
		Logger:                      noopLogger{}, // Suppress Pebble's internal logs

		// Optimized for write-heavy blockchain workload
		L0CompactionThreshold: 4,
		L0StopWritesThreshold: 12,
		LBaseMaxBytes:         256 << 20, // 256MB

		// Level options for optimal performance
		Levels: []pebble.LevelOptions{
			{BlockSize: 32 << 10, Compression: pebble.SnappyCompression, TargetFileSize: 4 << 20},
			{BlockSize: 32 << 10, Compression: pebble.SnappyCompression, TargetFileSize: 8 << 20},
			{BlockSize: 32 << 10, Compression: pebble.SnappyCompression, TargetFileSize: 16 << 20},
		},
	}

	// Disable sync for better performance if configured
	if config.ForceNoFsync {
		opts.DisableWAL = true
	}

	db, err := pebble.Open(config.Path, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	s := &BinaryStorage{
		db:     db,
		config: config,
		logger: logger,
	}

	// Initialize schema version
	if err := s.initializeSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return s, nil
}

// initializeSchema checks and sets the schema version
func (s *BinaryStorage) initializeSchema() error {
	key := SchemaVersionKey()

	// Check if schema version exists
	val, closer, err := s.db.Get(key)
	if err == pebble.ErrNotFound {
		// New database, set schema version
		var buf [4]byte
		binary.LittleEndian.PutUint32(buf[:], CurrentSchemaVersion)
		return s.db.Set(key, buf[:], pebble.Sync)
	}
	if err != nil {
		return err
	}
	defer closer.Close()

	// Check schema version
	if len(val) != 4 {
		return fmt.Errorf("invalid schema version data")
	}

	version := binary.LittleEndian.Uint32(val)
	if version != CurrentSchemaVersion {
		return fmt.Errorf("schema version mismatch: got %d, expected %d", version, CurrentSchemaVersion)
	}

	return nil
}

// All interface implementation methods are in interface_impl.go
// This file only contains the struct definition and initialization