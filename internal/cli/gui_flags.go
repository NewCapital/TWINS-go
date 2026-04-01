package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/urfave/cli/v2"
)

// Version information for GUI (set at build time or defaults)
var (
	GUIVersion   = "1.0.0"
	GUIGitCommit = "unknown"
	GUIBuildDate = "unknown"
)

// GUIConfig holds the parsed configuration for GUI application
// This struct is used by both the GUI and can be converted to frontend-compatible format
type GUIConfig struct {
	// Core settings (shared with daemon)
	DataDir string // -datadir
	Testnet bool   // -testnet
	Regtest bool   // -regtest
	Network string // "mainnet", "testnet", or "regtest"

	// GUI-specific settings
	StartMinimized bool   // -min (start minimized)
	ShowSplash     bool   // -splash (show splash screen)
	SplashSet      bool   // Whether -splash was explicitly set
	ChooseDataDir  bool   // -choosedatadir (force directory picker)
	Language       string // -lang (language code)
	WindowTitle    string // -windowtitle (custom window title)
	ResetSettings  bool   // -resetguisettings (reset GUI settings)

	// Help/version flags (cause early exit)
	Help    bool // -help, -h, -?
	Version bool // -version, -V

	// Development/testing flags (hidden from main help, shown in dev section)
	DevFullLogs bool // -dev-fulllogs (verbose startup logging)
	DevMockMode bool // -dev-mock (use mock core client instead of real daemon)
}

// GetWindowTitle returns the window title with network suffix if applicable.
// If WindowTitle is empty, returns "TWINS Core" as default.
func (c *GUIConfig) GetWindowTitle() string {
	title := c.WindowTitle
	if title == "" {
		title = "TWINS Core"
	}
	if c.Testnet {
		title += " [testnet]"
	} else if c.Regtest {
		title += " [regtest]"
	}
	return title
}

// GUIFlags returns CLI flags specific to GUI application
// These can be combined with common flags for a complete GUI flag set
func GUIFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:  "min",
			Value: false,
			Usage: "Start window minimized",
		},
		&cli.BoolFlag{
			Name:  "splash",
			Value: true,
			Usage: "Show splash screen on startup (default: true)",
		},
		&cli.BoolFlag{
			Name:  "choosedatadir",
			Value: false,
			Usage: "Choose data directory on startup",
		},
		&cli.StringFlag{
			Name:  "lang",
			Value: "",
			Usage: "Set language (e.g., en, es, ru, uk)",
		},
		&cli.StringFlag{
			Name:  "windowtitle",
			Value: "",
			Usage: "Set custom window title",
		},
		&cli.BoolFlag{
			Name:  "resetguisettings",
			Value: false,
			Usage: "Reset all GUI settings to default",
		},
	}
}

// ParseGUIArgs parses command-line arguments for the GUI application.
// This function is designed to be called BEFORE Wails initializes,
// allowing for early exit on -help/-version.
//
// Returns:
//   - config: Parsed GUI configuration
//   - shouldExit: True if program should exit after parsing (help/version)
//   - error: Any parsing or validation error
func ParseGUIArgs(args []string) (*GUIConfig, bool, error) {
	config := &GUIConfig{
		ShowSplash: true, // Default: show splash
		Network:    "mainnet",
	}

	// Check for help/version flags early (before normalization)
	// These need special handling because -? is not valid for Go's flag package
	for _, arg := range args {
		if IsHelpFlag(arg) {
			config.Help = true
			PrintGUIHelp()
			return config, true, nil
		}
		if IsVersionFlag(arg) {
			config.Version = true
			PrintGUIVersion()
			return config, true, nil
		}
	}

	// Normalize arguments (Windows /arg, GNU --arg)
	normalizedArgs, negatedFlags := NormalizeAndProcessArgs(args)

	// Track explicitly set flags
	explicitlySet := make(map[string]bool)

	// Parse arguments manually (simpler than urfave/cli for GUI without subcommands)
	for i := 0; i < len(normalizedArgs); i++ {
		arg := normalizedArgs[i]

		// Skip non-flag arguments
		if !strings.HasPrefix(arg, "-") {
			continue
		}

		// Parse key=value or key value
		key := strings.TrimPrefix(arg, "-")
		var value string
		hasValue := false

		if idx := strings.Index(key, "="); idx != -1 {
			value = key[idx+1:]
			key = key[:idx]
			hasValue = true
		}

		// Convert to lowercase for matching
		lowerKey := strings.ToLower(key)

		switch lowerKey {
		case "datadir":
			if !hasValue && i+1 < len(normalizedArgs) && !strings.HasPrefix(normalizedArgs[i+1], "-") {
				value = normalizedArgs[i+1]
				i++
			}
			if value != "" {
				expanded, err := ExpandPath(value)
				if err != nil {
					return nil, true, fmt.Errorf("invalid -datadir: %w", err)
				}
				if err := ValidateDataDir(expanded); err != nil {
					return nil, true, err
				}
				config.DataDir = expanded
				explicitlySet["datadir"] = true
			}

		case "testnet":
			config.Testnet = parseBoolValue(value, hasValue, true)
			if config.Testnet {
				config.Network = "testnet"
			}
			explicitlySet["testnet"] = true

		case "regtest":
			config.Regtest = parseBoolValue(value, hasValue, true)
			if config.Regtest {
				config.Network = "regtest"
			}
			explicitlySet["regtest"] = true

		case "min":
			config.StartMinimized = parseBoolValue(value, hasValue, true)
			explicitlySet["min"] = true

		case "splash":
			config.ShowSplash = parseBoolValue(value, hasValue, true)
			config.SplashSet = true
			explicitlySet["splash"] = true

		case "choosedatadir":
			config.ChooseDataDir = parseBoolValue(value, hasValue, true)
			explicitlySet["choosedatadir"] = true

		case "lang":
			if !hasValue && i+1 < len(normalizedArgs) && !strings.HasPrefix(normalizedArgs[i+1], "-") {
				value = normalizedArgs[i+1]
				i++
			}
			config.Language = value
			explicitlySet["lang"] = true

		case "windowtitle":
			if !hasValue && i+1 < len(normalizedArgs) && !strings.HasPrefix(normalizedArgs[i+1], "-") {
				value = normalizedArgs[i+1]
				i++
			}
			config.WindowTitle = value
			explicitlySet["windowtitle"] = true

		case "resetguisettings":
			config.ResetSettings = parseBoolValue(value, hasValue, true)
			explicitlySet["resetguisettings"] = true

		case "dev-fulllogs":
			config.DevFullLogs = parseBoolValue(value, hasValue, true)
			explicitlySet["dev-fulllogs"] = true

		case "dev-mock":
			config.DevMockMode = parseBoolValue(value, hasValue, true)
			explicitlySet["dev-mock"] = true
		}
	}

	// Apply negated flags from -noX processing
	if negatedFlags["splash"] && !explicitlySet["splash"] {
		config.ShowSplash = false
		config.SplashSet = true
	}

	// Validate flag combinations
	if config.Testnet && config.Regtest {
		return nil, true, fmt.Errorf("cannot use both -testnet and -regtest")
	}

	return config, false, nil
}

// parseBoolValue interprets a boolean flag value
func parseBoolValue(value string, hasValue bool, defaultIfNoValue bool) bool {
	if !hasValue {
		return defaultIfNoValue
	}
	lower := strings.ToLower(value)
	switch lower {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		return defaultIfNoValue
	}
}

// GetDefaultGUIDataDir returns the default data directory for GUI application
// Following C++ legacy paths:
//   - Windows: %APPDATA%\TWINS
//   - macOS: ~/Library/Application Support/TWINS
//   - Linux: ~/.twins
func GetDefaultGUIDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}

	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData != "" {
			return filepath.Join(appData, "TWINS")
		}
		return filepath.Join(home, "AppData", "Roaming", "TWINS")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "TWINS")
	default: // Linux and others
		return filepath.Join(home, ".twins")
	}
}

// ExpandPath expands ~ to home directory and converts to absolute path.
// Also validates against path traversal attacks.
func ExpandPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	// Expand ~ to home directory
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot expand ~: %w", err)
		}
		path = filepath.Join(home, path[1:])
	}

	// Clean the path to normalize . and .. sequences
	cleaned := filepath.Clean(path)

	// Security: Reject paths that escape upward (relative paths starting with ..)
	if strings.HasPrefix(cleaned, "..") {
		return "", fmt.Errorf("path attempts to escape working directory: %s", path)
	}

	// Convert to absolute path if relative
	if !filepath.IsAbs(cleaned) {
		abs, err := filepath.Abs(cleaned)
		if err != nil {
			return "", fmt.Errorf("cannot resolve absolute path: %w", err)
		}
		cleaned = abs
	}

	return cleaned, nil
}

// ValidateDataDir checks if the datadir path is valid and accessible.
func ValidateDataDir(path string) error {
	info, err := os.Stat(path)
	if err == nil {
		// Path exists - must be a directory
		if !info.IsDir() {
			return fmt.Errorf("-datadir must be a directory, not a file: %s", path)
		}
		return nil
	}

	if !os.IsNotExist(err) {
		return fmt.Errorf("cannot access -datadir: %w", err)
	}

	// Path doesn't exist - check if parent directory is writable
	parent := filepath.Dir(path)
	parentInfo, err := os.Stat(parent)
	if err != nil {
		return fmt.Errorf("parent directory does not exist: %s", parent)
	}
	if !parentInfo.IsDir() {
		return fmt.Errorf("parent path is not a directory: %s", parent)
	}

	return nil
}

// ToMap converts GUIConfig to a map for frontend access via Wails
func (c *GUIConfig) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"dataDir":        c.DataDir,
		"testnet":        c.Testnet,
		"regtest":        c.Regtest,
		"network":        c.Network,
		"startMinimized": c.StartMinimized,
		"showSplash":     c.ShowSplash,
		"splashSet":      c.SplashSet,
		"chooseDataDir":  c.ChooseDataDir,
		"language":       c.Language,
		"windowTitle":    c.WindowTitle,
		"resetSettings":  c.ResetSettings,
		"devMockMode":    c.DevMockMode,
	}
}

// PrintGUIHelp prints the help message to stdout
func PrintGUIHelp() {
	fmt.Println("TWINS Core Wallet - GUI Application")
	fmt.Println()
	fmt.Println("Usage: twins-gui [options]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println()
	fmt.Println("  -datadir=<dir>       Specify data directory")
	fmt.Println("  -testnet             Use the test network")
	fmt.Println("  -regtest             Use the regression test network")
	fmt.Println()
	fmt.Println("  -min                 Start window minimized")
	fmt.Println("  -splash              Show splash screen on startup (default: true)")
	fmt.Println("  -nosplash            Do not show splash screen")
	fmt.Println("  -choosedatadir       Choose data directory on startup")
	fmt.Println("  -lang=<lang>         Set language (e.g., en, es, ru, uk)")
	fmt.Println("  -windowtitle=<name>  Set custom window title")
	fmt.Println("  -resetguisettings    Reset all GUI settings to default")
	fmt.Println()
	fmt.Println("  -help, -h, -?        Display this help message and exit")
	fmt.Println("  -version, -V         Display version information and exit")
	fmt.Println()
	fmt.Println("Development options:")
	fmt.Println("  -dev-mock            Use mock blockchain (for UI development/testing)")
	fmt.Println("  -dev-fulllogs        Show verbose startup logging")
	fmt.Println()
	fmt.Println("Data directory defaults:")
	fmt.Printf("  Windows:  %%APPDATA%%\\TWINS\n")
	fmt.Printf("  macOS:    ~/Library/Application Support/TWINS\n")
	fmt.Printf("  Linux:    ~/.twins\n")
}

// PrintGUIVersion prints version information to stdout
func PrintGUIVersion() {
	fmt.Printf("TWINS Core Wallet (GUI) version %s\n", GUIVersion)
	fmt.Printf("Git commit: %s\n", GUIGitCommit)
	fmt.Printf("Build date: %s\n", GUIBuildDate)
	fmt.Printf("Go version: %s\n", runtime.Version())
	fmt.Printf("OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
}

// ResolveDataDir resolves the data directory from multiple sources with priority:
// CLI flag (highest) > preferences > OS default (lowest)
//
// Parameters:
//   - cliDataDir: Data directory from CLI flag (-datadir), empty if not provided
//   - prefsDataDir: Data directory from preferences.json, empty if not set
//
// Returns the resolved absolute path to the data directory
func ResolveDataDir(cliDataDir, prefsDataDir string) string {
	// Priority 1: CLI flag (highest priority)
	if cliDataDir != "" {
		return cliDataDir
	}

	// Priority 2: Preferences
	if prefsDataDir != "" {
		// Expand and validate preferences path
		expanded, err := ExpandPath(prefsDataDir)
		if err == nil {
			return expanded
		}
		// Log warning and fall through to default if preferences path is invalid
		fmt.Printf("Warning: Invalid preferences datadir '%s': %v, using OS default\n", prefsDataDir, err)
	}

	// Priority 3: OS default (lowest priority)
	return GetDefaultGUIDataDir()
}
