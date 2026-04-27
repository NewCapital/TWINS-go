package config

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
)

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string
	Value   interface{}
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error for %s (value: %v): %s", e.Field, e.Value, e.Message)
}

// ValidationErrors represents multiple validation errors
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	if len(e) == 1 {
		return e[0].Error()
	}

	var messages []string
	for _, err := range e {
		messages = append(messages, err.Error())
	}
	return fmt.Sprintf("multiple validation errors: %s", strings.Join(messages, "; "))
}

// ValidateConfig validates the entire configuration and returns detailed errors
func ValidateConfig(config *Config) error {
	if config == nil {
		return fmt.Errorf("configuration cannot be nil")
	}

	var errors ValidationErrors

	// Validate each section
	if err := validateNetworkConfig(&config.Network); err != nil {
		errors = append(errors, ValidationError{Field: "network", Message: err.Error()})
	}

	if err := validateRPCConfig(&config.RPC); err != nil {
		errors = append(errors, ValidationError{Field: "rpc", Message: err.Error()})
	}

	if err := validateMasternodeConfig(&config.Masternode); err != nil {
		errors = append(errors, ValidationError{Field: "masternode", Message: err.Error()})
	}

	if err := validateLoggingConfig(&config.Logging); err != nil {
		errors = append(errors, ValidationError{Field: "logging", Message: err.Error()})
	}

	if err := validateWalletConfig(&config.Wallet); err != nil {
		errors = append(errors, ValidationError{Field: "wallet", Message: err.Error()})
	}

	if err := validateStakingConfig(&config.Staking); err != nil {
		errors = append(errors, ValidationError{Field: "staking", Message: err.Error()})
	}

	if err := validateSyncConfig(&config.Sync); err != nil {
		errors = append(errors, ValidationError{Field: "sync", Message: err.Error()})
	}

	// Cross-section validation
	if err := detectPortConflicts(config); err != nil {
		errors = append(errors, ValidationError{Field: "ports", Message: err.Error()})
	}

	if len(errors) > 0 {
		return errors
	}

	return nil
}

// validateNetworkConfig validates network configuration
func validateNetworkConfig(config *NetworkConfig) error {
	// Validate port
	if config.Port < 1024 || config.Port > 65535 {
		return fmt.Errorf("port must be between 1024 and 65535, got %d", config.Port)
	}

	// Validate max peers
	if config.MaxPeers < 1 {
		return fmt.Errorf("max_peers must be at least 1, got %d", config.MaxPeers)
	}
	if config.MaxPeers > 10000 {
		return fmt.Errorf("max_peers cannot exceed 10000, got %d", config.MaxPeers)
	}

	// Validate seeds
	for i, seed := range config.Seeds {
		if err := validateSeedAddress(fmt.Sprintf("seeds[%d]", i), seed); err != nil {
			return err
		}
	}

	// Validate listen address
	if config.ListenAddr != "" {
		if ip := net.ParseIP(config.ListenAddr); ip == nil {
			return fmt.Errorf("invalid listen_addr: %s", config.ListenAddr)
		}
	}

	// Validate timeout
	if config.Timeout < 1 {
		return fmt.Errorf("timeout must be at least 1 second, got %d", config.Timeout)
	}

	// Validate keep alive
	if config.KeepAlive < 1 {
		return fmt.Errorf("keep_alive must be at least 1 second, got %d", config.KeepAlive)
	}

	return nil
}

// validateRPCConfig validates RPC configuration
func validateRPCConfig(config *RPCConfig) error {
	if !config.Enabled {
		return nil // Skip validation if RPC is disabled
	}

	// Validate port
	if config.Port < 1024 || config.Port > 65535 {
		return fmt.Errorf("port must be between 1024 and 65535, got %d", config.Port)
	}

	// Validate host
	if config.Host != "" {
		if ip := net.ParseIP(config.Host); ip == nil {
			// Check if it's a valid hostname
			if matched, _ := regexp.MatchString(`^[a-zA-Z0-9.-]+$`, config.Host); !matched {
				return fmt.Errorf("invalid host: %s", config.Host)
			}
		}
	}

	// Validate max clients
	if config.MaxClients < 1 {
		return fmt.Errorf("max_clients must be at least 1, got %d", config.MaxClients)
	}

	// Validate allowed IPs
	for i, ip := range config.AllowedIPs {
		if net.ParseIP(ip) == nil {
			return fmt.Errorf("invalid IP address in allowed_ips[%d]: %s", i, ip)
		}
	}

	// Validate rate limit
	if config.RateLimit < 1 {
		return fmt.Errorf("rate_limit must be at least 1, got %d", config.RateLimit)
	}

	// Validate timeout
	if config.Timeout < 1 {
		return fmt.Errorf("timeout must be at least 1 second, got %d", config.Timeout)
	}

	// Validate TLS config if enabled
	if config.TLS.Enabled {
		if err := validateRPCTLSConfig(&config.TLS); err != nil {
			return err
		}
	}

	// Validate mTLS requires TLS
	if config.TLS.MTLS.Enabled && !config.TLS.Enabled {
		return fmt.Errorf("mTLS requires TLS to be enabled (rpc.tls.enabled must be true)")
	}

	return nil
}

// validateRPCTLSConfig validates RPC TLS configuration when TLS is enabled
func validateRPCTLSConfig(config *RPCTLSConfig) error {
	// Certificate and key must both be provided
	if config.CertFile == "" {
		return fmt.Errorf("rpc.tls.certFile is required when TLS is enabled")
	}
	if config.KeyFile == "" {
		return fmt.Errorf("rpc.tls.keyFile is required when TLS is enabled")
	}

	// Expiry warning days must be positive
	if config.ExpiryWarnDays < 1 {
		return fmt.Errorf("rpc.tls.expiryWarnDays must be at least 1, got %d", config.ExpiryWarnDays)
	}
	if config.ExpiryWarnDays > 365 {
		return fmt.Errorf("rpc.tls.expiryWarnDays cannot exceed 365, got %d", config.ExpiryWarnDays)
	}

	// mTLS: client CA file required when mTLS enabled
	if config.MTLS.Enabled && config.MTLS.ClientCAFile == "" {
		return fmt.Errorf("rpc.tls.mtls.clientCAFile is required when mTLS is enabled")
	}

	return nil
}

// validateMasternodeConfig validates masternode configuration
func validateMasternodeConfig(config *MasternodeConfig) error {
	if !config.Enabled {
		return nil // Skip validation if masternode is disabled
	}

	// Validate required fields when enabled
	if config.PrivateKey == "" {
		return fmt.Errorf("private_key is required when masternode is enabled")
	}

	if config.ServiceAddr == "" {
		return fmt.Errorf("service_addr is required when masternode is enabled")
	}

	return nil
}

// validateLoggingConfig validates logging configuration
func validateLoggingConfig(config *LoggingConfig) error {
	// Validate level
	validLevels := []string{"trace", "debug", "info", "warn", "error", "fatal"}
	validLevel := false
	for _, level := range validLevels {
		if config.Level == level {
			validLevel = true
			break
		}
	}
	if !validLevel {
		return fmt.Errorf("invalid logging level: %s, valid levels: %v", config.Level, validLevels)
	}

	// Validate format
	validFormats := []string{"text", "json"}
	validFormat := false
	for _, format := range validFormats {
		if config.Format == format {
			validFormat = true
			break
		}
	}
	if !validFormat {
		return fmt.Errorf("unsupported logging format: %s, supported formats: %v", config.Format, validFormats)
	}

	// Validate output: "stdout", "stderr", or a file name/path
	if config.Output == "" {
		return fmt.Errorf("logging output must be 'stdout', 'stderr', or a file name (e.g., 'twins.log')")
	}
	if config.Output != "stdout" && config.Output != "stderr" && strings.Contains(config.Output, "..") {
		return fmt.Errorf("logging output path must not contain '..' segments")
	}

	return nil
}

// validateWalletConfig validates wallet configuration (legacy C++ compatible ranges)
func validateWalletConfig(config *WalletConfig) error {
	// COIN = 100,000,000 satoshis (1 TWINS)
	const COIN int64 = 100000000

	// Validate PayTxFee (legacy: -paytxfee)
	// Range: 0 (dynamic) to 1 COIN per KB (sanity limit)
	if config.PayTxFee < 0 {
		return fmt.Errorf("payTxFee cannot be negative, got %d", config.PayTxFee)
	}
	if config.PayTxFee > COIN {
		return fmt.Errorf("payTxFee cannot exceed 1 TWINS (100000000 satoshis) per KB, got %d", config.PayTxFee)
	}

	// Validate MinTxFee (legacy: -mintxfee)
	// Default: 10000 satoshis (0.0001 TWINS) per KB
	// Range: 0 to MaxTxFee (if set) or 1 COIN
	if config.MinTxFee < 0 {
		return fmt.Errorf("minTxFee cannot be negative, got %d", config.MinTxFee)
	}
	if config.MinTxFee > COIN {
		return fmt.Errorf("minTxFee cannot exceed 1 TWINS (100000000 satoshis) per KB, got %d", config.MinTxFee)
	}

	// Validate MaxTxFee (legacy: -maxtxfee)
	// Default: 1 TWINS (100000000 satoshis) - legacy default is 1 COIN
	// Range: MinTxFee (or 10000) to 10 COINS (reasonable upper limit)
	if config.MaxTxFee < 0 {
		return fmt.Errorf("maxTxFee cannot be negative, got %d", config.MaxTxFee)
	}
	if config.MaxTxFee > 0 && config.MaxTxFee < 10000 {
		return fmt.Errorf("maxTxFee must be at least 10000 satoshis (0.0001 TWINS), got %d", config.MaxTxFee)
	}
	if config.MaxTxFee > 10*COIN {
		return fmt.Errorf("maxTxFee cannot exceed 10 TWINS (1000000000 satoshis), got %d", config.MaxTxFee)
	}

	// Validate MinTxFee <= MaxTxFee (if MaxTxFee is set)
	if config.MaxTxFee > 0 && config.MinTxFee > config.MaxTxFee {
		return fmt.Errorf("minTxFee (%d) cannot exceed maxTxFee (%d)", config.MinTxFee, config.MaxTxFee)
	}

	// Validate TxConfirmTarget (legacy: -txconfirmtarget)
	// Range: 1-25 blocks (legacy default: 1)
	if config.TxConfirmTarget < 1 {
		return fmt.Errorf("txConfirmTarget must be at least 1, got %d", config.TxConfirmTarget)
	}
	if config.TxConfirmTarget > 25 {
		return fmt.Errorf("txConfirmTarget cannot exceed 25 blocks, got %d", config.TxConfirmTarget)
	}

	// Validate Keypool (legacy: -keypool)
	// Default: 1000, Range: 1-100000
	if config.Keypool < 1 {
		return fmt.Errorf("keypool must be at least 1, got %d", config.Keypool)
	}
	if config.Keypool > 100000 {
		return fmt.Errorf("keypool cannot exceed 100000, got %d", config.Keypool)
	}

	// Validate CreateWalletBackups (legacy: -createwalletbackups)
	// Default: 10, Range: 0 (disabled) to 100
	if config.CreateWalletBackups < 0 {
		return fmt.Errorf("createWalletBackups cannot be negative, got %d", config.CreateWalletBackups)
	}
	if config.CreateWalletBackups > 100 {
		return fmt.Errorf("createWalletBackups cannot exceed 100, got %d", config.CreateWalletBackups)
	}

	return nil
}

// validateStakingConfig validates staking configuration (legacy C++ compatible)
func validateStakingConfig(config *StakingConfig) error {
	// COIN = 100,000,000 satoshis (1 TWINS)
	const COIN int64 = 100000000

	// Validate ReserveBalance (legacy: -reservebalance)
	// Range: 0 to 100M TWINS (sanity limit to prevent unreasonable values)
	if config.ReserveBalance < 0 {
		return fmt.Errorf("reserveBalance cannot be negative, got %d", config.ReserveBalance)
	}

	// Sanity limit: 100M TWINS (100,000,000 * COIN = 10^16 satoshis)
	// This is well below int64 max and covers any reasonable reserve
	maxReserve := int64(100000000) * COIN // 100M TWINS
	if config.ReserveBalance > maxReserve {
		return fmt.Errorf("reserveBalance cannot exceed 100M TWINS (%d satoshis), got %d", maxReserve, config.ReserveBalance)
	}

	return nil
}

// detectPortConflicts checks for port conflicts between services
func detectPortConflicts(config *Config) error {
	ports := make(map[int]string)

	// Check network port
	if existing, exists := ports[config.Network.Port]; exists {
		return fmt.Errorf("port conflict between network and %s: %d", existing, config.Network.Port)
	}
	ports[config.Network.Port] = "network"

	// Check RPC port if enabled
	if config.RPC.Enabled {
		if existing, exists := ports[config.RPC.Port]; exists {
			return fmt.Errorf("port conflict between rpc and %s: %d", existing, config.RPC.Port)
		}
		ports[config.RPC.Port] = "rpc"
	}

	return nil
}

// Helper validation functions

func validateSeedAddress(field, address string) error {
	// Seeds should be in format host:port
	parts := strings.Split(address, ":")
	if len(parts) != 2 {
		return fmt.Errorf("invalid seed address format in %s: %s (should be host:port)", field, address)
	}

	host, portStr := parts[0], parts[1]

	// Validate host (can be IP or hostname)
	if ip := net.ParseIP(host); ip == nil {
		// Check if it's a valid hostname
		if matched, _ := regexp.MatchString(`^[a-zA-Z0-9.-]+$`, host); !matched {
			return fmt.Errorf("invalid host in %s: %s", field, host)
		}
	}

	// Validate port
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		return fmt.Errorf("invalid port in %s: %s", field, portStr)
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port range in %s: %d", field, port)
	}

	return nil
}

// validateSyncConfig validates sync configuration
func validateSyncConfig(config *SyncConfig) error {
	if config.BootstrapMinPeers < 1 || config.BootstrapMinPeers > 100 {
		return fmt.Errorf("invalid bootstrap min peers: %d", config.BootstrapMinPeers)
	}

	if config.BootstrapMinWait < 0 || config.BootstrapMinWait > 300 {
		return fmt.Errorf("invalid bootstrap min wait: %d seconds", config.BootstrapMinWait)
	}

	if config.BootstrapMaxWait < config.BootstrapMinWait || config.BootstrapMaxWait > 600 {
		return fmt.Errorf("invalid bootstrap max wait: %d seconds (must be >= min wait and <= 600)", config.BootstrapMaxWait)
	}

	if config.IBDThreshold < 100 || config.IBDThreshold > 1000000 {
		return fmt.Errorf("invalid IBD threshold: %d blocks", config.IBDThreshold)
	}

	if config.MaxSyncPeers < 5 || config.MaxSyncPeers > 100 {
		return fmt.Errorf("invalid max sync peers: %d", config.MaxSyncPeers)
	}

	if config.ErrorScoreCooldown < 1.0 || config.ErrorScoreCooldown > 100.0 {
		return fmt.Errorf("invalid error score cooldown: %.1f", config.ErrorScoreCooldown)
	}

	if config.CooldownDuration < 60 || config.CooldownDuration > 3600 {
		return fmt.Errorf("invalid cooldown duration: %d seconds", config.CooldownDuration)
	}

	if config.HealthDecayTime < 600 || config.HealthDecayTime > 86400 {
		return fmt.Errorf("invalid health decay time: %d seconds", config.HealthDecayTime)
	}

	if config.HealthThreshold < 0.0 || config.HealthThreshold > 100.0 {
		return fmt.Errorf("invalid health threshold: %.1f", config.HealthThreshold)
	}

	if config.ErrorWeightInvalidBlock < 0.0 || config.ErrorWeightInvalidBlock > 10.0 {
		return fmt.Errorf("invalid error weight invalid block: %.1f", config.ErrorWeightInvalidBlock)
	}

	if config.ErrorWeightTimeout < 0.0 || config.ErrorWeightTimeout > 10.0 {
		return fmt.Errorf("invalid error weight timeout: %.1f", config.ErrorWeightTimeout)
	}

	if config.ErrorWeightConnectionDrop < 0.0 || config.ErrorWeightConnectionDrop > 10.0 {
		return fmt.Errorf("invalid error weight connection drop: %.1f", config.ErrorWeightConnectionDrop)
	}

	if config.ErrorWeightSlowResponse < 0.0 || config.ErrorWeightSlowResponse > 10.0 {
		return fmt.Errorf("invalid error weight slow response: %.1f", config.ErrorWeightSlowResponse)
	}

	if config.ErrorWeightSendFailed < 0.0 || config.ErrorWeightSendFailed > 10.0 {
		return fmt.Errorf("invalid error weight send failed: %.1f", config.ErrorWeightSendFailed)
	}

	if config.BatchTimeout < 10 || config.BatchTimeout > 600 {
		return fmt.Errorf("invalid batch timeout: %d seconds", config.BatchTimeout)
	}

	if config.RoundsBeforeRebuild < 1 || config.RoundsBeforeRebuild > 100 {
		return fmt.Errorf("invalid rounds before rebuild: %d", config.RoundsBeforeRebuild)
	}

	if config.ReorgWindow < 60 || config.ReorgWindow > 86400 {
		return fmt.Errorf("invalid reorg window: %d seconds", config.ReorgWindow)
	}

	if config.MaxAutoReorgs < 0 || config.MaxAutoReorgs > 10 {
		return fmt.Errorf("invalid max auto reorgs: %d", config.MaxAutoReorgs)
	}

	if config.ProgressLogInterval < 1 || config.ProgressLogInterval > 300 {
		return fmt.Errorf("invalid progress log interval: %d seconds", config.ProgressLogInterval)
	}

	return nil
}

func validateFileExists(field, path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("%s file does not exist: %s", field, path)
	}
	return nil
}
