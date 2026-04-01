package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/twins-dev/twins-core/internal/config"
	"github.com/twins-dev/twins-core/internal/gui/initialization"
)

// Service provides configuration management for the wallet.
// TWINS daemon settings delegate to ConfigManager (set via SetConfigManager after daemon starts).
// Masternode.conf operations are always available.
type Service struct {
	manager *Manager
	cm      atomic.Pointer[config.ConfigManager] // nil until daemon starts; set via SetConfigManager
}

// NewService creates a new configuration service.
// cm may be nil initially; call SetConfigManager after daemon initializes to enable full config access.
func NewService(dataDir string, cm *config.ConfigManager) (*Service, error) {
	manager := NewManager(dataDir)

	if err := manager.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize config manager: %w", err)
	}

	s := &Service{manager: manager}
	if cm != nil {
		s.cm.Store(cm)
	}
	return s, nil
}

// SetConfigManager wires the daemon ConfigManager into the service.
// Call this after daemon.NewNode() returns so TWINS config methods have a live source.
// Safe to call concurrently with any getter method.
func (s *Service) SetConfigManager(cm *config.ConfigManager) {
	s.cm.Store(cm)
}

// GetRPCConfig returns RPC configuration for connecting to the daemon.
func (s *Service) GetRPCConfig() (string, int, string, string, error) {
	cm := s.cm.Load()
	if cm == nil {
		return "localhost", 0, "", "", nil
	}

	host := cm.GetString("rpc.host")
	if host == "" {
		host = "localhost"
	}

	return host, cm.GetInt("rpc.port"), cm.GetString("rpc.username"), cm.GetString("rpc.password"), nil
}

// GetNetwork returns the current network (mainnet, testnet, regtest)
func (s *Service) GetNetwork() string {
	cm := s.cm.Load()
	if cm == nil {
		return "mainnet"
	}
	snap := cm.Snapshot()
	if snap.Network.TestNet {
		return "testnet"
	}
	return "mainnet"
}

// IsTestnet returns true if running on testnet
func (s *Service) IsTestnet() bool {
	return s.GetNetwork() == "testnet"
}

// IsRegtest returns true if running on regtest
func (s *Service) IsRegtest() bool {
	return s.GetNetwork() == "regtest"
}

// GetDataDirectory returns the data directory path
func (s *Service) GetDataDirectory() string {
	return s.manager.dataDir
}

// GetMasternodes returns all configured masternodes
func (s *Service) GetMasternodes() []initialization.MasternodeEntry {
	return s.manager.GetMasternodes()
}

// AddMasternode adds a new masternode configuration
func (s *Service) AddMasternode(alias, address, privKey, txID string, outputIndex int) error {
	entry := initialization.MasternodeEntry{
		Alias:       alias,
		Address:     address,
		PrivKey:     privKey,
		TxID:        txID,
		OutputIndex: outputIndex,
	}

	return s.manager.AddMasternode(entry)
}

// RemoveMasternode removes a masternode by alias
func (s *Service) RemoveMasternode(alias string) error {
	return s.manager.RemoveMasternode(alias)
}

// GetStakingEnabled returns whether staking is enabled
func (s *Service) GetStakingEnabled() bool {
	cm := s.cm.Load()
	if cm == nil {
		return false
	}
	return cm.GetBool("staking.enabled")
}

// GetMaxConnections returns the maximum number of connections
func (s *Service) GetMaxConnections() int {
	cm := s.cm.Load()
	if cm == nil {
		return 125
	}
	return cm.GetInt("network.maxPeers")
}

// ExportConfiguration exports all configuration to a JSON file
func (s *Service) ExportConfiguration(path string) error {
	path = filepath.Clean(path)

	combined := s.GetFullConfiguration()

	data, err := json.MarshalIndent(combined, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal configuration: %w", err)
	}

	return os.WriteFile(path, data, 0600)
}

// ImportConfiguration imports configuration from a JSON file.
// Only masternode entries are imported; TWINS daemon settings must be changed via ConfigManager.
func (s *Service) ImportConfiguration(path string) error {
	path = filepath.Clean(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read import file: %w", err)
	}

	var combined CombinedConfig
	if err := json.Unmarshal(data, &combined); err != nil {
		return fmt.Errorf("failed to unmarshal configuration: %w", err)
	}

	if len(combined.Masternodes) > 0 {
		s.manager.mu.Lock()
		s.manager.masternodes = combined.Masternodes
		err := s.manager.saveMasternodeConfig()
		s.manager.mu.Unlock()
		if err != nil {
			return fmt.Errorf("failed to save masternode config: %w", err)
		}
	}

	return nil
}

// ReloadConfiguration reloads masternode.conf
func (s *Service) ReloadConfiguration() error {
	return s.manager.ReloadMasternodeConfig()
}

// ValidateConfiguration validates the loaded configuration.
// ConfigManager validates daemon settings on load; this is a no-op unless future validation is added.
func (s *Service) ValidateConfiguration() error {
	return nil
}

// OnConfigChange registers a callback for configuration changes
func (s *Service) OnConfigChange(callback func(ConfigChangeEvent)) {
	s.manager.OnConfigChange(callback)
}

// Close shuts down the configuration service
func (s *Service) Close() error {
	return s.manager.Close()
}

// GetFullConfiguration returns the complete combined configuration.
// TWINS daemon settings are sourced from ConfigManager when available.
func (s *Service) GetFullConfiguration() *CombinedConfig {
	masternodes := s.manager.GetMasternodes()
	network := s.GetNetwork()

	var twins *initialization.TWINSConfig
	if cm := s.cm.Load(); cm != nil {
		twins = initialization.ConfigFromYAML(cm.Snapshot())
	}

	return &CombinedConfig{
		TWINS:       twins,
		Masternodes: masternodes,
		DataDir:     s.manager.dataDir,
		Network:     network,
		IsTestnet:   network == "testnet",
		IsRegtest:   network == "regtest",
	}
}
