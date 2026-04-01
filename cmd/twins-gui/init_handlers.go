package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/twins-dev/twins-core/internal/gui/initialization"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ==========================================
// Initialization and Data Directory Methods
// ==========================================

// GetDefaultDataDirectory returns the default TWINS data directory
func (a *App) GetDefaultDataDirectory() string {
	if a.initService == nil {
		return ""
	}
	return a.initService.GetDefaultDataDirectory()
}

// GetStoredDataDirectory returns the stored data directory from preferences
func (a *App) GetStoredDataDirectory() string {
	if a.prefsService == nil {
		return ""
	}
	return a.prefsService.GetDataDirectory()
}

// CheckFirstRun checks if this is the first run by looking for existing configuration
// Also returns isFirstRun=true if -choosedatadir flag is set (to force data directory selection)
// Returns showSplash=false if -nosplash flag is set (to skip splash screen)
func (a *App) CheckFirstRun() map[string]interface{} {
	// Default showSplash to true - only skip if explicitly disabled via CLI flag
	showSplash := a.guiConfig == nil || a.guiConfig.ShowSplash

	result := map[string]interface{}{
		"isFirstRun": false,
		"dataDir":    "",
		"showSplash": showSplash,
		"error":      nil,
	}

	// Use a.dataDir which was already resolved in startup() with CLI > prefs > default priority
	// This ensures CLI -datadir flag is respected throughout the application
	// Thread-safe read of a.dataDir
	a.componentsMu.RLock()
	dataDir := a.dataDir
	a.componentsMu.RUnlock()

	if dataDir == "" {
		// Fallback if startup() didn't set it (shouldn't happen in normal flow)
		if a.prefsService != nil && a.prefsService.HasDataDirectory() {
			dataDir = a.prefsService.GetDataDirectory()
		} else if a.initService != nil {
			dataDir = a.initService.GetDefaultDataDirectory()
		}
	}

	// Log the source of dataDir for debugging
	if a.guiConfig != nil && a.guiConfig.DataDir != "" {
		fmt.Printf("CheckFirstRun: Using CLI-provided data directory: %s\n", dataDir)
	} else {
		fmt.Printf("CheckFirstRun: Using resolved data directory: %s\n", dataDir)
	}

	// Check if -choosedatadir flag forces showing the intro dialog
	if a.guiConfig != nil && a.guiConfig.ChooseDataDir {
		fmt.Println("-choosedatadir flag set - treating as first run")
		result["isFirstRun"] = true
		result["dataDir"] = dataDir
		return result
	}

	result["dataDir"] = dataDir

	// If CLI provided -datadir, user explicitly chose their directory - skip first run check
	// They've already made their choice via command line, no need for selection dialog
	if a.guiConfig != nil && a.guiConfig.DataDir != "" {
		fmt.Println("CLI -datadir provided - skipping first run check, using specified directory")
		// Validate directory accessibility (permission errors, etc.)
		// Note: os.IsNotExist is okay - directory will be created during initialization
		if _, err := os.Stat(dataDir); err != nil && !os.IsNotExist(err) {
			result["error"] = fmt.Sprintf("CLI -datadir '%s' error: %v", dataDir, err)
			return result
		}
		result["isFirstRun"] = false
		return result
	}

	// Check if twinsd.yml or legacy twins.conf exists in the data directory
	ymlPath := filepath.Join(dataDir, "twinsd.yml")
	confPath := filepath.Join(dataDir, "twins.conf")
	_, ymlErr := os.Stat(ymlPath)
	_, confErr := os.Stat(confPath)

	if os.IsNotExist(ymlErr) && os.IsNotExist(confErr) {
		// Neither config file exists - this is a first run for this directory
		result["isFirstRun"] = true
	} else if ymlErr != nil && !os.IsNotExist(ymlErr) {
		result["error"] = ymlErr.Error()
	} else if confErr != nil && !os.IsNotExist(confErr) && os.IsNotExist(ymlErr) {
		result["error"] = confErr.Error()
	}

	return result
}

// SelectDataDirectory opens a directory selection dialog
func (a *App) SelectDataDirectory() (string, error) {
	if a.initService == nil {
		return "", fmt.Errorf("initialization service not initialized")
	}
	return a.initService.SelectDataDirectory(a.ctx)
}

// CheckDiskSpace checks if the specified directory has enough space
func (a *App) CheckDiskSpace(directory string) (*initialization.DiskSpaceInfo, error) {
	if a.initService == nil {
		return nil, fmt.Errorf("initialization service not initialized")
	}
	return a.initService.CheckDiskSpace(directory)
}

// ValidateDataDirectory validates the selected data directory
func (a *App) ValidateDataDirectory(directory string) error {
	if a.initService == nil {
		return fmt.Errorf("initialization service not initialized")
	}
	return a.initService.ValidateDataDirectory(directory)
}

// InitializeDataDirectory creates the data directory structure and saves the preference
func (a *App) InitializeDataDirectory(directory string) error {
	if a.initService == nil {
		return fmt.Errorf("initialization service not initialized")
	}

	// Initialize the directory structure
	if err := a.initService.InitializeDataDirectory(directory); err != nil {
		return err
	}

	// Save the selected directory to preferences
	if a.prefsService != nil {
		if err := a.prefsService.SetDataDirectory(directory); err != nil {
			fmt.Printf("Warning: Failed to save data directory preference: %v\n", err)
			// Don't fail the whole operation just because we couldn't save the preference
		} else {
			fmt.Printf("Saved data directory preference: %s\n", directory)
		}
	}

	return nil
}

// StartInitialization begins the wallet initialization process with progress events.
// In mock mode, emits immediate completion. In real mode, starts full daemon init.
// If initialization already completed (e.g. browser dev mode reconnection), re-emits
// the completion event so late-connecting frontends can transition past the splash screen.
func (a *App) StartInitialization() error {
	if a.guiConfig != nil && a.guiConfig.DevMockMode {
		// Mock mode: emit complete immediately (core client created in domReady)
		runtime.EventsEmit(a.ctx, "initialization:complete", initialization.InitProgress{
			Step:        "complete",
			Description: "Mock mode ready",
			Progress:    100,
			TotalSteps:  1,
			CurrentStep: 1,
			IsComplete:  true,
		})
		return nil
	}

	// Check if initialization already started (atomic guard in initializeFullDaemon).
	// This handles browser dev mode: the native window already triggered init,
	// so a late-connecting browser needs to know the current state.
	if a.initStarting.Load() {
		if a.initCompleted.Load() {
			// Init already completed — re-emit completion for late-connecting clients
			fmt.Println("StartInitialization: already completed, re-emitting completion event")
			runtime.EventsEmit(a.ctx, "initialization:complete", initialization.InitProgress{
				Step:        "complete",
				Description: "Wallet initialized successfully!",
				Progress:    100,
				TotalSteps:  1,
				CurrentStep: 1,
				IsComplete:  true,
			})
		} else {
			// Check if init failed (error stored by emitInitFatal)
			a.componentsMu.RLock()
			initErr := a.initError
			a.componentsMu.RUnlock()

			if initErr != "" {
				// Init failed — re-emit fatal error for late-connecting clients
				fmt.Println("StartInitialization: initialization previously failed, re-emitting fatal event")
				runtime.EventsEmit(a.ctx, "initialization:fatal", map[string]interface{}{
					"error":      initErr,
					"shouldExit": true,
				})
			} else {
				// Init in progress — browser will receive remaining events naturally
				fmt.Println("StartInitialization: already in progress, browser will receive remaining events")
			}
		}
		return nil
	}

	// Real mode: start full daemon initialization with P2P
	go a.initializeFullDaemon()
	return nil
}

// ResetPreferences clears stored preferences (useful for testing)
func (a *App) ResetPreferences() error {
	if a.prefsService == nil {
		return fmt.Errorf("preferences service not initialized")
	}
	return a.prefsService.Reset()
}
