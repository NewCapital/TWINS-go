package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"

	"github.com/twins-dev/twins-core/internal/gui/initialization"
)

// CombinedConfig represents the complete wallet configuration view.
type CombinedConfig struct {
	TWINS       *initialization.TWINSConfig      `json:"twins"`
	Masternodes []initialization.MasternodeEntry `json:"masternodes"`
	DataDir     string                           `json:"dataDir"`
	Network     string                           `json:"network"`
	IsTestnet   bool                             `json:"isTestnet"`
	IsRegtest   bool                             `json:"isRegtest"`
}

// ConfigChangeEvent represents a configuration change
type ConfigChangeEvent struct {
	Type      string    `json:"type"`      // "masternode"
	Path      string    `json:"path"`      // File path that changed
	Timestamp time.Time `json:"timestamp"`
	OldHash   string    `json:"oldHash"`
	NewHash   string    `json:"newHash"`
}

// Manager handles masternode.conf management for the TWINS wallet.
// TWINS daemon config (twinsd.yml / twins.conf) is owned by ConfigManager;
// this type only manages masternode.conf.
type Manager struct {
	mu sync.RWMutex

	// Masternode configuration
	masternodes []initialization.MasternodeEntry

	// Paths
	dataDir    string
	mnConfPath string

	// State
	isInitialized bool
	lastLoaded    time.Time
	watcher       *fsnotify.Watcher
	ctx           context.Context
	cancel        context.CancelFunc

	// Callbacks
	changeCallbacks []func(ConfigChangeEvent)
}

// NewManager creates a new configuration manager
func NewManager(dataDir string) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	return &Manager{
		dataDir:         dataDir,
		mnConfPath:      filepath.Join(dataDir, "masternode.conf"),
		ctx:             ctx,
		cancel:          cancel,
		changeCallbacks: make([]func(ConfigChangeEvent), 0),
	}
}

// Initialize loads masternode configuration and starts watching for changes
func (m *Manager) Initialize() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.loadMasternodeConfig(); err != nil {
		return fmt.Errorf("failed to load masternode config: %w", err)
	}

	if err := m.startWatcher(); err != nil {
		return fmt.Errorf("failed to start config watcher: %w", err)
	}

	m.isInitialized = true
	m.lastLoaded = time.Now()

	return nil
}

// loadMasternodeConfig loads the masternode.conf file
func (m *Manager) loadMasternodeConfig() error {
	masternodes, err := initialization.LoadMasternodeConf(m.dataDir)
	if err != nil {
		return err
	}
	m.masternodes = masternodes
	return nil
}

// startWatcher starts watching masternode.conf for changes
func (m *Manager) startWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create watcher: %w", err)
	}

	m.watcher = watcher

	if _, err := os.Stat(m.mnConfPath); err == nil {
		if err := watcher.Add(m.mnConfPath); err != nil {
			return fmt.Errorf("failed to watch %s: %w", m.mnConfPath, err)
		}
	}

	go m.watchLoop()

	return nil
}

// watchLoop handles file system events
func (m *Manager) watchLoop() {
	for {
		select {
		case <-m.ctx.Done():
			return

		case event, ok := <-m.watcher.Events:
			if !ok {
				return
			}

			if event.Op&fsnotify.Write == fsnotify.Write {
				m.handleFileChange(event.Name)
			}

		case err, ok := <-m.watcher.Errors:
			if !ok {
				return
			}
			log.WithError(err).Warn("config watcher error")
		}
	}
}

// handleFileChange handles masternode.conf file changes
func (m *Manager) handleFileChange(path string) {
	if path != m.mnConfPath {
		return
	}

	if err := m.ReloadMasternodeConfig(); err != nil {
		log.WithError(err).Error("failed to reload masternode config")
		return
	}

	m.notifyCallbacks(ConfigChangeEvent{
		Type:      "masternode",
		Path:      path,
		Timestamp: time.Now(),
	})
}

// ReloadMasternodeConfig reloads the masternode configuration
func (m *Manager) ReloadMasternodeConfig() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.loadMasternodeConfig()
}

// GetMasternodes returns the masternode configurations
func (m *Manager) GetMasternodes() []initialization.MasternodeEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.masternodes
}

// AddMasternode adds a new masternode configuration
func (m *Manager) AddMasternode(entry initialization.MasternodeEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, mn := range m.masternodes {
		if mn.Alias == entry.Alias {
			return fmt.Errorf("masternode with alias %s already exists", entry.Alias)
		}
	}

	m.masternodes = append(m.masternodes, entry)
	return m.saveMasternodeConfig()
}

// RemoveMasternode removes a masternode by alias
func (m *Manager) RemoveMasternode(alias string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var newList []initialization.MasternodeEntry
	found := false

	for _, mn := range m.masternodes {
		if mn.Alias != alias {
			newList = append(newList, mn)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("masternode %s not found", alias)
	}

	m.masternodes = newList
	return m.saveMasternodeConfig()
}

// saveMasternodeConfig saves the masternode configuration
func (m *Manager) saveMasternodeConfig() error {
	var lines []string

	lines = append(lines, "# Masternode config file")
	lines = append(lines, "# Format: alias IP:port masternodeprivkey collateral_output_txid collateral_output_index")
	lines = append(lines, "# Example: mn1 127.0.0.1:37817 93HaYBVUCYjEMeeH1Y4sBGLALQZE1Yc1K64xiqgX37tGBDQL8Xg 2bcd3c84c84f87333b3e5c2d3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e3e 0")
	lines = append(lines, "")

	for _, mn := range m.masternodes {
		line := fmt.Sprintf("%s %s %s %s %d",
			mn.Alias,
			mn.Address,
			mn.PrivKey,
			mn.TxID,
			mn.OutputIndex,
		)
		lines = append(lines, line)
	}

	var b strings.Builder
	for _, line := range lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}

	return os.WriteFile(m.mnConfPath, []byte(b.String()), 0600)
}

// OnConfigChange registers a callback for configuration changes
func (m *Manager) OnConfigChange(callback func(ConfigChangeEvent)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.changeCallbacks = append(m.changeCallbacks, callback)
}

// notifyCallbacks notifies all registered callbacks of a configuration change
func (m *Manager) notifyCallbacks(event ConfigChangeEvent) {
	m.mu.RLock()
	callbacks := make([]func(ConfigChangeEvent), len(m.changeCallbacks))
	copy(callbacks, m.changeCallbacks)
	m.mu.RUnlock()

	for _, callback := range callbacks {
		go callback(event)
	}
}

// Close stops the configuration manager
func (m *Manager) Close() error {
	m.cancel()

	if m.watcher != nil {
		return m.watcher.Close()
	}

	return nil
}

// IsInitialized returns whether the configuration manager is initialized
func (m *Manager) IsInitialized() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.isInitialized
}

// GetLastLoaded returns when configurations were last loaded
func (m *Manager) GetLastLoaded() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.lastLoaded
}
