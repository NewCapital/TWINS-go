package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"

	"github.com/twins-dev/twins-core/internal/config"
	"github.com/twins-dev/twins-core/internal/gui/preferences"
)

// migrateDaemonSettings performs a one-time migration of daemon-related settings
// from settings.json (GUI preferences) into twinsd.yml (ConfigManager).
// Only non-default values are migrated. A marker file prevents re-migration.
//
// Because daemon fields have been removed from the GUISettings struct, this
// function reads settings.json as raw JSON to access the legacy fields.
func migrateDaemonSettings(ss *preferences.SettingsService, cm *config.ConfigManager, dataDir string) {
	markerPath := filepath.Join(dataDir, ".gui-config-migrated")

	// Check if migration was already done
	if _, err := os.Stat(markerPath); err == nil {
		return
	}

	// Read settings.json as raw JSON to access removed daemon fields
	settingsPath := ss.GetSettingsPath()
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if !os.IsNotExist(err) {
			logrus.WithError(err).Warn("Migration: failed to read settings.json")
		}
		// No settings file — nothing to migrate, write marker and return
		_ = os.WriteFile(markerPath, []byte("1"), 0600)
		return
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		logrus.WithError(err).Warn("Migration: failed to parse settings.json, will retry next startup")
		return
	}

	migrated, failed := 0, 0

	// Helper to extract a bool from raw JSON
	getBool := func(key string) (bool, bool) {
		v, ok := raw[key]
		if !ok {
			return false, false
		}
		var b bool
		if err := json.Unmarshal(v, &b); err != nil {
			return false, false
		}
		return b, true
	}

	// Helper to extract a float64 from raw JSON
	getFloat64 := func(key string) (float64, bool) {
		v, ok := raw[key]
		if !ok {
			return 0, false
		}
		var f float64
		if err := json.Unmarshal(v, &f); err != nil {
			return 0, false
		}
		return f, true
	}

	// Helper to extract a string from raw JSON
	getString := func(key string) (string, bool) {
		v, ok := raw[key]
		if !ok {
			return "", false
		}
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return "", false
		}
		return s, true
	}

	// Staking
	if enabled, ok := getBool("fStaking"); ok && enabled {
		if err := cm.Set("staking.enabled", true); err != nil {
			logrus.WithError(err).Warn("Migration: failed to set staking.enabled")
			failed++
		} else {
			migrated++
		}
	}
	if reserve, ok := getFloat64("nReserveBalance"); ok && reserve > 0 {
		satoshis := int64(math.Round(reserve * 1e8))
		if err := cm.Set("staking.reserveBalance", satoshis); err != nil {
			logrus.WithError(err).Warn("Migration: failed to set staking.reserveBalance")
			failed++
		} else {
			migrated++
		}
	}

	// Network
	if listen, ok := getBool("fListen"); ok && !listen {
		// Default is true; only migrate if user disabled it
		if err := cm.Set("network.listen", false); err != nil {
			logrus.WithError(err).Warn("Migration: failed to set network.listen")
			failed++
		} else {
			migrated++
		}
	}
	if upnp, ok := getBool("fUseUPnP"); ok && !upnp {
		// Default is true; only migrate if user disabled it
		if err := cm.Set("network.upnp", false); err != nil {
			logrus.WithError(err).Warn("Migration: failed to set network.upnp")
			failed++
		} else {
			migrated++
		}
	}
	useProxy, _ := getBool("fUseProxy")
	proxyAddr, _ := getString("addrProxy")
	if useProxy && proxyAddr != "" {
		if err := cm.Set("network.proxy", proxyAddr); err != nil {
			logrus.WithError(err).Warn("Migration: failed to set network.proxy")
			failed++
		} else {
			migrated++
		}
	}

	// Wallet
	if spendZero, ok := getBool("bSpendZeroConfChange"); ok && spendZero {
		// Default is false; only migrate if user explicitly enabled it
		if err := cm.Set("wallet.spendZeroConfChange", true); err != nil {
			logrus.WithError(err).Warn("Migration: failed to set wallet.spendZeroConfChange")
			failed++
		} else {
			migrated++
		}
	}

	if migrated > 0 {
		logrus.WithField("count", migrated).Info("Migrated daemon settings from settings.json to twinsd.yml")
	}

	// Only write the marker if all attempted migrations succeeded.
	// If any YAML write failed, skip the marker so we retry on next launch.
	if failed > 0 {
		logrus.WithField("failed", failed).Warn("Migration incomplete, will retry on next launch")
		return
	}
	if err := os.WriteFile(markerPath, []byte("1"), 0600); err != nil {
		logrus.WithError(err).Warn("Failed to write migration marker file")
	}
}
