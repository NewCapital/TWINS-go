package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// generateCommentedYAML creates a well-commented twinsd.yml with all default values.
// The output matches the polished layout with section separators, blank lines between
// settings, and (default: X) annotations.
// Caller must hold cm.mu.
func (cm *ConfigManager) generateCommentedYAML() error {
	var b strings.Builder

	// File header
	b.WriteString("# ==========================================\n")
	b.WriteString("# TWINS Core Configuration (twinsd.yml)\n")
	b.WriteString("# ==========================================\n")
	b.WriteString("#\n")
	b.WriteString("# All settings can also be changed via the GUI settings dialog.\n")
	b.WriteString("# CLI flags (--flag) override values in this file when specified.\n")
	b.WriteString("# Environment variables (TWINS_*) override this file but not CLI flags.\n")
	b.WriteString("#\n")
	b.WriteString("# Priority: CLI flags > environment variables > this file > defaults\n")
	b.WriteString("\n")

	// Group settings by category
	type catEntry struct {
		key string
		def *settingDef
	}
	categories := make(map[string][]catEntry)
	for key, def := range cm.registry {
		categories[def.Category] = append(categories[def.Category], catEntry{key, def})
	}

	// Sort entries within each category by key
	for cat := range categories {
		sort.Slice(categories[cat], func(i, j int) bool {
			return categories[cat][i].key < categories[cat][j].key
		})
	}

	// Write each category section in registration order
	for _, cat := range cm.categoryOrder {
		entries, ok := categories[cat]
		if !ok || len(entries) == 0 {
			continue
		}

		// Category header with separator bars
		title := categoryTitle(cat)
		b.WriteString("# ==========================================\n")
		fmt.Fprintf(&b, "# %s\n", title)
		b.WriteString("# ==========================================\n")
		fmt.Fprintf(&b, "%s:\n", cat)

		for i, entry := range entries {
			def := entry.def
			// Extract the field name after the dot (e.g., "staking.enabled" → "enabled")
			parts := strings.SplitN(def.Key, ".", 2)
			if len(parts) != 2 {
				continue
			}
			fieldName := parts[1]

			// Write description with (default: X) annotation
			comment := "  # " + def.Description
			if def.Units != "" {
				comment += ", " + def.Units
			}
			comment += " (default: " + formatYAMLValue(def.Default) + ")"
			if def.CLIFlag != "" {
				comment += " [CLI: --" + def.CLIFlag + "]"
			}
			b.WriteString(comment + "\n")

			// Write the current value
			currentVal := def.getter(cm.config)
			fmt.Fprintf(&b, "  %s: %s\n", fieldName, formatYAMLValue(currentVal))

			// Blank line between settings within a category (not after last)
			if i < len(entries)-1 {
				b.WriteString("\n")
			}
		}

		b.WriteString("\n")
	}

	// Ensure parent directory exists
	dir := filepath.Dir(cm.yamlPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write atomically
	tmpPath := cm.yamlPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(b.String()), 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	if err := os.Rename(tmpPath, cm.yamlPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename config: %w", err)
	}

	return nil
}

// GenerateDefaultConfig creates a default twinsd.yml at the given path.
// This is a standalone function for use outside the ConfigManager.
func GenerateDefaultConfig(path string) error {
	cm := NewConfigManager(path, nil)
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.generateCommentedYAML()
}

// formatYAMLValue formats a Go value as a YAML literal.
func formatYAMLValue(v interface{}) string {
	switch val := v.(type) {
	case bool:
		if val {
			return "true"
		}
		return "false"
	case int:
		return fmt.Sprintf("%d", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case uint32:
		return fmt.Sprintf("%d", val)
	case float64:
		// Use clean formatting for round numbers
		if val == float64(int64(val)) {
			return fmt.Sprintf("%.1f", val)
		}
		return fmt.Sprintf("%g", val)
	case string:
		if val == "" {
			return `""`
		}
		// Quote strings that could be misinterpreted by YAML
		if needsQuoting(val) {
			return fmt.Sprintf("%q", val)
		}
		return val
	case []string:
		if len(val) == 0 {
			return "[]"
		}
		var parts []string
		for _, s := range val {
			parts = append(parts, fmt.Sprintf("%q", s))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// needsQuoting returns true if a YAML string value needs quoting.
func needsQuoting(s string) bool {
	// YAML special values that could be misinterpreted
	switch strings.ToLower(s) {
	case "true", "false", "yes", "no", "on", "off", "null", "~":
		return true
	}
	// Quote if contains special YAML characters
	for _, c := range s {
		switch c {
		case ':', '#', '[', ']', '{', '}', ',', '&', '*', '?', '|', '-', '<', '>', '=', '!', '%', '@', '`':
			return true
		}
	}
	return false
}

// categoryTitle returns a human-readable title for a setting category.
func categoryTitle(cat string) string {
	titles := map[string]string{
		"staking":    "Staking Settings",
		"wallet":     "Wallet Settings",
		"network":    "Network Settings",
		"rpc":        "RPC Server Settings",
		"masternode": "Masternode Settings",
		"logging":    "Logging Settings",
		"sync":       "Sync Settings",
	}
	if title, ok := titles[cat]; ok {
		return title
	}
	// Capitalize first letter manually (strings.Title is deprecated)
	if len(cat) > 0 {
		return strings.ToUpper(cat[:1]) + cat[1:] + " Settings"
	}
	return "Settings"
}
