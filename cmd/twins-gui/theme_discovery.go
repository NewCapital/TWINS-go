package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

// maxThemeFiles limits the number of files scanned in the themes directory
// to prevent DoS from directories with thousands of files
const maxThemeFiles = 100

// ThemeInfo contains information about an available theme
type ThemeInfo struct {
	Name      string `json:"name"`
	IsBuiltIn bool   `json:"isBuiltIn"`
	Path      string `json:"path,omitempty"` // Empty for built-in themes
}

// builtInThemes are the themes shipped with the application
var builtInThemes = []string{"system", "dark", "light"}

// GetAvailableThemes returns all available themes (built-in + custom from datadir/themes/)
func (a *App) GetAvailableThemes() ([]ThemeInfo, error) {
	themes := make([]ThemeInfo, 0, len(builtInThemes)+10)

	// Add built-in themes first
	for _, name := range builtInThemes {
		themes = append(themes, ThemeInfo{
			Name:      name,
			IsBuiltIn: true,
		})
	}

	// Get data directory
	dataDir := a.getDataDir()
	if dataDir == "" {
		return themes, nil // Return only built-in themes if no dataDir
	}

	// Scan themes directory
	themesDir := filepath.Join(dataDir, "themes")
	customThemes, err := scanThemesDirectory(themesDir)
	if err != nil {
		// Log error but don't fail - just return built-in themes
		logrus.WithError(err).WithField("themesDir", themesDir).Warn("Failed to scan themes directory, using built-in themes only")
		return themes, nil
	}

	themes = append(themes, customThemes...)
	return themes, nil
}

// scanThemesDirectory scans the themes directory for custom themes
// Supports two formats:
// 1. Folder themes: themes/mytheme/theme.qss or themes/mytheme/style.css
// 2. File themes: themes/mytheme.qss or themes/mytheme.css
// Security: Validates all paths stay within the themes directory to prevent path traversal
// DoS Protection: Limits the number of files scanned to maxThemeFiles
func scanThemesDirectory(themesDir string) ([]ThemeInfo, error) {
	if _, err := os.Stat(themesDir); os.IsNotExist(err) {
		return nil, nil // Directory doesn't exist, no custom themes
	}

	// Get absolute path of themes directory for path traversal validation
	absThemesDir, err := filepath.Abs(themesDir)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(themesDir)
	if err != nil {
		return nil, err
	}

	// DoS protection: limit number of files scanned
	if len(entries) > maxThemeFiles {
		return nil, fmt.Errorf("too many files in themes directory (max %d, found %d)", maxThemeFiles, len(entries))
	}

	var themes []ThemeInfo
	seen := make(map[string]bool) // Avoid duplicates

	for _, entry := range entries {
		name := entry.Name()

		// Skip hidden files and suspicious names
		if strings.HasPrefix(name, ".") || strings.Contains(name, "..") {
			continue
		}

		if entry.IsDir() {
			// Folder theme - check for theme.qss or style.css inside
			themeFile := findThemeFile(filepath.Join(themesDir, name))
			if themeFile != "" && !seen[name] {
				// Validate path stays within themes directory using filepath.Rel
				// This is safer than strings.HasPrefix as it handles case-insensitive
				// filesystems and alternate path representations correctly
				if !isPathWithinDir(absThemesDir, themeFile) {
					continue // Skip paths outside themes directory
				}
				themes = append(themes, ThemeInfo{
					Name:      name,
					IsBuiltIn: false,
					Path:      themeFile,
				})
				seen[name] = true
			}
		} else {
			// File theme - check for .qss or .css extension
			ext := strings.ToLower(filepath.Ext(name))
			if ext == ".qss" || ext == ".css" {
				themeName := strings.TrimSuffix(name, ext)
				if !seen[themeName] && !isBuiltInTheme(themeName) {
					filePath := filepath.Join(themesDir, name)
					// Validate path stays within themes directory
					if !isPathWithinDir(absThemesDir, filePath) {
						continue // Skip paths outside themes directory
					}
					themes = append(themes, ThemeInfo{
						Name:      themeName,
						IsBuiltIn: false,
						Path:      filePath,
					})
					seen[themeName] = true
				}
			}
		}
	}

	return themes, nil
}

// isPathWithinDir checks if a path is within a directory using filepath.Rel
// This is safer than strings.HasPrefix as it handles case-insensitive
// filesystems and alternate path representations correctly.
// Security: Resolves symlinks first to prevent path traversal via symbolic links
func isPathWithinDir(dir, path string) bool {
	// Resolve symlinks first to prevent bypass attacks
	// If the file is a symlink pointing outside the directory, this will catch it
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		// If symlink resolution fails (e.g., broken symlink), reject the path
		return false
	}

	absPath, err := filepath.Abs(realPath)
	if err != nil {
		return false
	}

	// Also resolve symlinks in the base directory for consistent comparison
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		// Fall back to original dir if symlink resolution fails
		realDir = dir
	}

	absDir, err := filepath.Abs(realDir)
	if err != nil {
		return false
	}

	// Use filepath.Rel to get relative path from dir to path
	// If the result starts with "..", the path is outside the directory
	relPath, err := filepath.Rel(absDir, absPath)
	if err != nil {
		return false
	}

	// Check if relative path escapes the directory
	return !strings.HasPrefix(relPath, "..") && relPath != ".."
}

// findThemeFile looks for a valid theme file inside a theme folder
func findThemeFile(themeDir string) string {
	candidates := []string{"theme.qss", "style.css", "theme.css", "style.qss"}

	for _, candidate := range candidates {
		path := filepath.Join(themeDir, candidate)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// isBuiltInTheme checks if a theme name matches a built-in theme
func isBuiltInTheme(name string) bool {
	nameLower := strings.ToLower(name)
	for _, builtin := range builtInThemes {
		if strings.ToLower(builtin) == nameLower {
			return true
		}
	}
	return false
}

// getDataDir returns the current data directory (helper method)
func (a *App) getDataDir() string {
	a.componentsMu.RLock()
	defer a.componentsMu.RUnlock()
	return a.dataDir
}
