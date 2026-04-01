package main

import (
	"fmt"

	configservice "github.com/twins-dev/twins-core/internal/gui/config"
	"github.com/twins-dev/twins-core/internal/gui/initialization"
)

// ==========================================
// Configuration Management Methods
// ==========================================

// LoadConfiguration loads and validates all configuration files
// Also initializes blockchain storage and switches from mock to real client
func (a *App) LoadConfiguration(dataDir string) error {
	fmt.Printf("LoadConfiguration called with dataDir: %s\n", dataDir)

	// Thread-safe check if CLI provided dataDir (CLI has highest priority)
	// a.dataDir was already resolved in startup() using CLI > prefs > default priority
	a.componentsMu.RLock()
	cliProvidedDataDir := a.guiConfig != nil && a.guiConfig.DataDir != ""
	currentDataDir := a.dataDir
	a.componentsMu.RUnlock()

	if cliProvidedDataDir {
		// CLI provided dataDir - use the value resolved in startup()
		fmt.Printf("CLI -datadir flag detected, using CLI value: %s\n", currentDataDir)
		dataDir = currentDataDir
	} else {
		// No CLI flag - use frontend-provided dataDir and update a.dataDir
		a.componentsMu.Lock()
		a.dataDir = dataDir
		a.componentsMu.Unlock()
	}

	// Initialize config service (skip if already initialized)
	if a.configService != nil {
		fmt.Println("Config service already initialized, skipping")
	} else {
		var err error
		a.configService, err = configservice.NewService(dataDir, nil)
		if err != nil {
			// Config service failure is non-fatal - continue without it
			// This allows the window to show even if config parsing has issues
			fmt.Printf("Warning: Config service initialization failed: %v\n", err)
		} else {
			fmt.Println("Config service initialized successfully")
		}
	}

	// Storage initialization is handled by initializeFullDaemon() via daemon.NewNode().
	// Do NOT open the database here - it would conflict with NewNode's database lock.

	return nil
}

// GetConfiguration returns the current configuration
func (a *App) GetConfiguration() interface{} {
	if a.configService == nil {
		return nil
	}
	return a.configService.GetFullConfiguration()
}

// GetMasternodeList returns all configured masternodes
func (a *App) GetMasternodeList() []initialization.MasternodeEntry {
	if a.configService == nil {
		return []initialization.MasternodeEntry{}
	}
	return a.configService.GetMasternodes()
}

// AddMasternode adds a new masternode configuration
func (a *App) AddMasternode(alias, address, privKey, txID string, outputIndex int) error {
	if a.configService == nil {
		return fmt.Errorf("configuration service not initialized")
	}
	return a.configService.AddMasternode(alias, address, privKey, txID, outputIndex)
}

// RemoveMasternode removes a masternode by alias
func (a *App) RemoveMasternode(alias string) error {
	if a.configService == nil {
		return fmt.Errorf("configuration service not initialized")
	}
	return a.configService.RemoveMasternode(alias)
}

// ExportConfig exports configuration to a file
func (a *App) ExportConfig(path string) error {
	if a.configService == nil {
		return fmt.Errorf("configuration service not initialized")
	}
	return a.configService.ExportConfiguration(path)
}

// ImportConfig imports configuration from a file
func (a *App) ImportConfig(path string) error {
	if a.configService == nil {
		return fmt.Errorf("configuration service not initialized")
	}
	return a.configService.ImportConfiguration(path)
}

// ReloadConfiguration reloads all configuration files
func (a *App) ReloadConfiguration() error {
	if a.configService == nil {
		return fmt.Errorf("configuration service not initialized")
	}
	return a.configService.ReloadConfiguration()
}

// ValidateConfiguration validates all loaded configurations
func (a *App) ValidateConfiguration() error {
	if a.configService == nil {
		return fmt.Errorf("configuration service not initialized")
	}
	return a.configService.ValidateConfiguration()
}
