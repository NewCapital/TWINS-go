package initialization

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/twins-dev/twins-core/internal/config"
)

// TWINSConfig represents the parsed twins.conf configuration
type TWINSConfig struct {
	// Network settings
	Testnet bool   `json:"testnet"`
	Regtest bool   `json:"regtest"`
	Network string `json:"network"` // "mainnet", "testnet", or "regtest"

	// RPC settings
	RPCUser     string   `json:"rpcuser"`
	RPCPassword string   `json:"rpcpassword"`
	RPCPort     int      `json:"rpcport"`
	RPCAllowIP  []string `json:"rpcallowip"`
	RPCBind     string   `json:"rpcbind"`

	// Wallet settings
	WalletEnabled    bool   `json:"wallet"`
	StakingEnabled   bool   `json:"staking"`
	// Masternode settings
	Masternode       bool   `json:"masternode"`
	MasternodePrivKey string `json:"masternodeprivkey"`
	MasternodeAddr   string `json:"masternodeaddr"`

	// Performance settings
	DBCache        int `json:"dbcache"`
	MaxConnections int `json:"maxconnections"`
	MaxMempool     int `json:"maxmempool"`
	MempoolExpiry  int `json:"mempoolexpiry"`

	// Data directory
	DataDir string `json:"datadir"`

	// Additional settings
	TxIndex    bool     `json:"txindex"`
	AddNodes   []string `json:"addnode"`
	ConnectNodes []string `json:"connect"`
	BanNodes   []string `json:"ban"`

	// Debug settings
	Debug      bool     `json:"debug"`
	DebugCategories []string `json:"debugcategories"`
}

// ParseConfigFile parses the twins.conf configuration file
func ParseConfigFile(configPath string) (*TWINSConfig, error) {
	config := &TWINSConfig{
		// Set defaults
		Network:        "mainnet",
		RPCPort:        37817,
		WalletEnabled:  true,
		StakingEnabled: false,
		DBCache:        1024,
		MaxConnections: 125,
		MaxMempool:     300,
		MempoolExpiry:  336,
		RPCAllowIP:     []string{},
		AddNodes:       []string{},
		ConnectNodes:   []string{},
		BanNodes:       []string{},
		DebugCategories: []string{},
	}

	file, err := os.Open(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Config file doesn't exist, return defaults
			return config, nil
		}
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse key=value pairs
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue // Skip malformed lines
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		value = strings.Trim(value, "\"'")

		// Parse configuration values
		switch key {
		// Network settings
		case "testnet":
			config.Testnet = parseBool(value)
			if config.Testnet {
				config.Network = "testnet"
				config.RPCPort = 37817 // Testnet port
			}
		case "regtest":
			config.Regtest = parseBool(value)
			if config.Regtest {
				config.Network = "regtest"
				config.RPCPort = 37817 // Regtest port
			}

		// RPC settings
		case "rpcuser":
			config.RPCUser = value
		case "rpcpassword":
			config.RPCPassword = value
		case "rpcport":
			if port, err := strconv.Atoi(value); err == nil {
				config.RPCPort = port
			}
		case "rpcallowip":
			config.RPCAllowIP = append(config.RPCAllowIP, value)
		case "rpcbind":
			config.RPCBind = value

		// Wallet settings
		case "wallet":
			config.WalletEnabled = parseBool(value)
		case "staking":
			config.StakingEnabled = parseBool(value)
		// Masternode settings
		case "masternode":
			config.Masternode = parseBool(value)
		case "masternodeprivkey":
			config.MasternodePrivKey = value
		case "masternodeaddr":
			config.MasternodeAddr = value

		// Performance settings
		case "dbcache":
			if cache, err := strconv.Atoi(value); err == nil {
				config.DBCache = cache
			}
		case "maxconnections":
			if conn, err := strconv.Atoi(value); err == nil {
				config.MaxConnections = conn
			}
		case "maxmempool":
			if mem, err := strconv.Atoi(value); err == nil {
				config.MaxMempool = mem
			}
		case "mempoolexpiry":
			if exp, err := strconv.Atoi(value); err == nil {
				config.MempoolExpiry = exp
			}

		// Data directory
		case "datadir":
			config.DataDir = expandPath(value)

		// Additional settings
		case "txindex":
			config.TxIndex = parseBool(value)
		case "addnode":
			config.AddNodes = append(config.AddNodes, value)
		case "connect":
			config.ConnectNodes = append(config.ConnectNodes, value)
		case "ban":
			config.BanNodes = append(config.BanNodes, value)

		// Debug settings
		case "debug":
			if value == "1" || strings.ToLower(value) == "true" {
				config.Debug = true
			} else {
				// Could be a debug category
				config.DebugCategories = append(config.DebugCategories, value)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading config file: %w", err)
	}

	return config, nil
}

// ValidateConfig validates the configuration parameters
func ValidateConfig(config *TWINSConfig) error {
	// Validate network settings
	if config.Testnet && config.Regtest {
		return fmt.Errorf("cannot use both testnet and regtest")
	}

	// Validate RPC settings
	if config.RPCUser == "" || config.RPCPassword == "" {
		return fmt.Errorf("rpcuser and rpcpassword must be set")
	}

	if config.RPCPort < 1 || config.RPCPort > 65535 {
		return fmt.Errorf("invalid rpc port: %d", config.RPCPort)
	}

	// Validate masternode settings
	if config.Masternode {
		if config.MasternodePrivKey == "" {
			return fmt.Errorf("masternode private key required when masternode=1")
		}
		// Validate private key format (should be 51 characters for TWINS)
		if len(config.MasternodePrivKey) != 51 {
			return fmt.Errorf("invalid masternode private key length")
		}
	}

	// Validate performance settings
	if config.DBCache < 4 {
		return fmt.Errorf("dbcache must be at least 4MB")
	}

	if config.MaxConnections < 0 || config.MaxConnections > 1000 {
		return fmt.Errorf("maxconnections must be between 0 and 1000")
	}

	return nil
}

// LoadMasternodeConf loads the masternode.conf file if it exists
func LoadMasternodeConf(dataDir string) ([]MasternodeEntry, error) {
	confPath := filepath.Join(dataDir, "masternode.conf")

	file, err := os.Open(confPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []MasternodeEntry{}, nil // No masternode.conf is ok
		}
		return nil, fmt.Errorf("failed to open masternode.conf: %w", err)
	}
	defer file.Close()

	var entries []MasternodeEntry
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse masternode entry: alias IP:port privkey txid outputindex
		parts := strings.Fields(line)
		if len(parts) != 5 {
			continue // Skip malformed entries
		}

		outputIndex, err := strconv.Atoi(parts[4])
		if err != nil {
			continue
		}

		entry := MasternodeEntry{
			Alias:       parts[0],
			Address:     parts[1],
			PrivKey:     parts[2],
			TxID:        parts[3],
			OutputIndex: outputIndex,
		}

		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading masternode.conf: %w", err)
	}

	return entries, nil
}

// MasternodeEntry represents a masternode configuration entry
type MasternodeEntry struct {
	Alias       string `json:"alias"`
	Address     string `json:"address"`
	PrivKey     string `json:"privkey"`
	TxID        string `json:"txid"`
	OutputIndex int    `json:"outputIndex"`
}

// ParseYAMLConfigFile parses a twinsd.yml YAML configuration file into TWINSConfig.
// Reuses internal/config.LoadConfig() for YAML parsing and maps fields to TWINSConfig.
func ParseYAMLConfigFile(configPath string) (*TWINSConfig, error) {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load YAML config: %w", err)
	}

	return ConfigFromYAML(cfg), nil
}

// ConfigFromYAML maps a config.Config (daemon YAML config) to TWINSConfig (GUI config).
func ConfigFromYAML(cfg *config.Config) *TWINSConfig {
	network := "mainnet"
	if cfg.Network.TestNet {
		network = "testnet"
	}

	return &TWINSConfig{
		Testnet:          cfg.Network.TestNet,
		Network:          network,
		RPCUser:          cfg.RPC.Username,
		RPCPassword:      cfg.RPC.Password,
		RPCPort:          cfg.RPC.Port,
		RPCAllowIP:       cfg.RPC.AllowedIPs,
		RPCBind:          cfg.RPC.Host,
		WalletEnabled:    cfg.Wallet.Enabled,
		StakingEnabled:   cfg.Staking.Enabled,
		Masternode:       cfg.Masternode.Enabled,
		MasternodePrivKey: cfg.Masternode.PrivateKey,
		MasternodeAddr:   cfg.Masternode.ServiceAddr,
		MaxConnections:   cfg.Network.MaxPeers,
		DataDir:          cfg.DataDir,
		AddNodes:         cfg.Network.AddNodes,
		ConnectNodes:     cfg.Network.ConnectOnly,
		DBCache:          1024, // Default, not in YAML config
	}
}

// Helper functions

func parseBool(value string) bool {
	return value == "1" || strings.ToLower(value) == "true" || strings.ToLower(value) == "yes"
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	return path
}