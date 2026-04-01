package daemon

import (
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/twins-dev/twins-core/internal/config"
)

func TestWireConfigSubscribers_NilManager(t *testing.T) {
	// WireConfigSubscribers should not panic when ConfigManager is nil.
	n := &Node{
		logger: logrus.NewEntry(logrus.StandardLogger()),
	}
	// Should be a no-op, no panic.
	n.WireConfigSubscribers()
}

func TestWireConfigSubscribers_LoggingLevel(t *testing.T) {
	// Create a temp ConfigManager with a temp YAML path.
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "twinsd.yml")
	logger := logrus.NewEntry(logrus.StandardLogger())

	cm := config.NewConfigManager(yamlPath, logger)
	if err := cm.LoadOrCreate(); err != nil {
		t.Fatalf("LoadOrCreate failed: %v", err)
	}

	n := &Node{
		ConfigManager: cm,
		logger:        logger,
	}
	n.WireConfigSubscribers()

	// Set log level to debug via ConfigManager — subscriber should apply it.
	originalLevel := logrus.GetLevel()
	defer logrus.SetLevel(originalLevel)

	if err := cm.Set("logging.level", "debug"); err != nil {
		t.Fatalf("Set logging.level failed: %v", err)
	}

	if logrus.GetLevel() != logrus.DebugLevel {
		t.Errorf("expected log level debug, got %s", logrus.GetLevel())
	}

	// Change to warn level.
	if err := cm.Set("logging.level", "warn"); err != nil {
		t.Fatalf("Set logging.level failed: %v", err)
	}

	if logrus.GetLevel() != logrus.WarnLevel {
		t.Errorf("expected log level warn, got %s", logrus.GetLevel())
	}
}

func TestWireConfigSubscribers_StakingEnabled(t *testing.T) {
	// Verify the subscriber is registered and fires on staking.enabled change.
	// We can't fully test StartStaking/StopStaking without a consensus engine,
	// but we verify the subscriber doesn't panic when Node has no consensus.
	tmpDir := t.TempDir()
	yamlPath := filepath.Join(tmpDir, "twinsd.yml")
	logger := logrus.NewEntry(logrus.StandardLogger())

	cm := config.NewConfigManager(yamlPath, logger)
	if err := cm.LoadOrCreate(); err != nil {
		t.Fatalf("LoadOrCreate failed: %v", err)
	}

	n := &Node{
		ConfigManager: cm,
		logger:        logger,
	}
	n.WireConfigSubscribers()

	// Setting staking.enabled to true without consensus engine should log a warning, not panic.
	err := cm.Set("staking.enabled", true)
	if err != nil {
		t.Fatalf("Set staking.enabled failed: %v", err)
	}

	// Setting it back to false should also not panic.
	err = cm.Set("staking.enabled", false)
	if err != nil {
		t.Fatalf("Set staking.enabled=false failed: %v", err)
	}
}
