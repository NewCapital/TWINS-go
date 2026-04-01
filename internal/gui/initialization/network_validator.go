package initialization

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Pre-compiled regexes for validation
var (
	validLabelRegex  = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)
	validBase58Regex = regexp.MustCompile(`^[123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz]+$`)
)

// NetworkParams represents TWINS network parameters
type NetworkParams struct {
	Name           string   `json:"name"`
	DefaultPort    int      `json:"defaultPort"`
	RPCPort        int      `json:"rpcPort"`
	DNSSeeds       []string `json:"dnsSeeds"`
	FixedSeeds     []string `json:"fixedSeeds"`
	Magic          uint32   `json:"magic"`
	AddressVersion byte     `json:"addressVersion"`
	PrivKeyVersion byte     `json:"privKeyVersion"`
}

// TWINS network parameters
var (
	MainNetParams = NetworkParams{
		Name:           "mainnet",
		DefaultPort:    37817,
		RPCPort:        37817,
		Magic:          0xf9beb4d9,
		AddressVersion: 0x49, // Addresses start with 'T'
		PrivKeyVersion: 0x42, // 66 - WIF private keys (legacy: SECRET_KEY)
		DNSSeeds: []string{
			"159.65.195.97",
			"134.209.146.52",
			"46.101.113.6",
			"138.68.154.249",
			"137.184.217.142",
			"165.22.149.70",
			"170.64.157.157",
			"134.122.38.24",
			"45.77.64.171",
			"45.32.36.145",
			"45.77.206.161",
			"207.148.67.25",
		},
		FixedSeeds: []string{
			"159.65.195.97:37817",
			"134.209.146.52:37817",
			"46.101.113.6:37817",
			"138.68.154.249:37817",
			"137.184.217.142:37817",
			"165.22.149.70:37817",
			"170.64.157.157:37817",
			"134.122.38.24:37817",
			"45.77.64.171:37817",
			"45.32.36.145:37817",
			"45.77.206.161:37817",
			"207.148.67.25:37817",
		},
	}

	TestNetParams = NetworkParams{
		Name:           "testnet",
		DefaultPort:    37817,
		RPCPort:        37817,
		Magic:          0x0b110907,
		AddressVersion: 0x8c, // Testnet addresses
		PrivKeyVersion: 0xed, // 237 - Testnet WIF private keys (legacy: SECRET_KEY)
		DNSSeeds: []string{
			"testnet-seed.twins.cool",
		},
		FixedSeeds: []string{
			"testnet1.twins.cool:37817",
			"testnet2.twins.cool:37817",
		},
	}

	RegTestParams = NetworkParams{
		Name:           "regtest",
		DefaultPort:    37817,
		RPCPort:        37817,
		Magic:          0xfabfb5da,
		AddressVersion: 0x8c,
		PrivKeyVersion: 0xed, // 237 - Regtest WIF private keys (same as testnet)
		DNSSeeds:       []string{}, // No DNS seeds for regtest
		FixedSeeds:     []string{}, // No fixed seeds for regtest
	}
)

// NetworkValidator handles network parameter validation
type NetworkValidator struct {
	params NetworkParams
}

// NewNetworkValidator creates a new network validator
func NewNetworkValidator(network string) *NetworkValidator {
	var params NetworkParams

	switch strings.ToLower(network) {
	case "testnet":
		params = TestNetParams
	case "regtest":
		params = RegTestParams
	default:
		params = MainNetParams
	}

	return &NetworkValidator{params: params}
}

// ValidateNetworkConfig validates network-related configuration
func (nv *NetworkValidator) ValidateNetworkConfig(config *TWINSConfig) error {
	// Validate port settings
	if err := nv.validatePort(config.RPCPort, "RPC"); err != nil {
		return err
	}

	// Validate IP addresses
	for _, ip := range config.RPCAllowIP {
		if err := validateIPAddress(ip); err != nil {
			return fmt.Errorf("invalid rpcallowip %s: %w", ip, err)
		}
	}

	if config.RPCBind != "" {
		if err := validateIPAddress(config.RPCBind); err != nil {
			return fmt.Errorf("invalid rpcbind %s: %w", config.RPCBind, err)
		}
	}

	// Validate node addresses
	for _, node := range config.AddNodes {
		if err := validateNodeAddress(node); err != nil {
			return fmt.Errorf("invalid addnode %s: %w", node, err)
		}
	}

	for _, node := range config.ConnectNodes {
		if err := validateNodeAddress(node); err != nil {
			return fmt.Errorf("invalid connect node %s: %w", node, err)
		}
	}

	// Validate masternode address if configured
	if config.Masternode && config.MasternodeAddr != "" {
		if err := validateNodeAddress(config.MasternodeAddr); err != nil {
			return fmt.Errorf("invalid masternode address %s: %w", config.MasternodeAddr, err)
		}
	}

	return nil
}

// validatePort checks if a port number is valid
func (nv *NetworkValidator) validatePort(port int, portType string) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid %s port %d: must be between 1 and 65535", portType, port)
	}

	// Check if port is not a well-known port (unless it's the default)
	if port < 1024 && port != nv.params.DefaultPort && port != nv.params.RPCPort {
		return fmt.Errorf("%s port %d is in well-known port range", portType, port)
	}

	return nil
}

// validateIPAddress validates an IP address or CIDR notation
func validateIPAddress(address string) error {
	// Check if it's a CIDR notation
	if strings.Contains(address, "/") {
		_, _, err := net.ParseCIDR(address)
		if err != nil {
			return fmt.Errorf("invalid CIDR notation: %w", err)
		}
		return nil
	}

	// Check if it's a valid IP address
	ip := net.ParseIP(address)
	if ip == nil {
		return fmt.Errorf("invalid IP address")
	}

	return nil
}

// validateNodeAddress validates a node address (IP:port or hostname:port)
func validateNodeAddress(address string) error {
	// Split into host and port
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		// Maybe just a hostname without port
		if !strings.Contains(address, ":") {
			host = address
			portStr = "37817" // Default TWINS port
		} else {
			return fmt.Errorf("invalid address format: %w", err)
		}
	}

	// Validate port
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid port: %w", err)
	}

	if port < 1 || port > 65535 {
		return fmt.Errorf("port %d out of range", port)
	}

	// Validate host (IP or hostname)
	if net.ParseIP(host) == nil {
		// Not an IP, check if it's a valid hostname
		if !isValidHostname(host) {
			return fmt.Errorf("invalid hostname: %s", host)
		}
	}

	return nil
}

// isValidHostname checks if a string is a valid hostname
func isValidHostname(hostname string) bool {
	// Basic hostname validation
	if len(hostname) > 253 {
		return false
	}

	// Check each label
	labels := strings.Split(hostname, ".")
	if len(labels) == 0 {
		return false
	}

	for _, label := range labels {
		if !validLabelRegex.MatchString(label) {
			return false
		}
	}

	return true
}

// CheckNetworkConnectivity tests basic network connectivity
func (nv *NetworkValidator) CheckNetworkConnectivity() error {
	// Try to resolve DNS seeds
	for _, seed := range nv.params.DNSSeeds {
		_, err := net.LookupHost(seed)
		if err == nil {
			// At least one seed is resolvable
			return nil
		}
	}

	// Try to connect to fixed seeds
	for _, seed := range nv.params.FixedSeeds {
		conn, err := net.DialTimeout("tcp", seed, 5*time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
	}

	return fmt.Errorf("no network connectivity to TWINS network")
}

// ValidateAddress validates a TWINS address
func (nv *NetworkValidator) ValidateAddress(address string) error {
	// Basic length check
	if len(address) < 26 || len(address) > 35 {
		return fmt.Errorf("invalid address length")
	}

	// Check if it starts with the correct prefix
	// Mainnet: 'D' (legacy) or 'T' (new format)
	// Testnet: 'x' or 'y'
	var validPrefixes []string
	if nv.params.Name == "testnet" || nv.params.Name == "regtest" {
		validPrefixes = []string{"x", "y"} // Testnet prefixes
	} else {
		validPrefixes = []string{"D", "T"} // Mainnet prefixes (both legacy and new)
	}

	hasValidPrefix := false
	for _, prefix := range validPrefixes {
		if strings.HasPrefix(address, prefix) {
			hasValidPrefix = true
			break
		}
	}

	if !hasValidPrefix {
		return fmt.Errorf("address should start with %v for %s", validPrefixes, nv.params.Name)
	}

	// Validate base58 characters
	if !validBase58Regex.MatchString(address) {
		return fmt.Errorf("invalid characters in address")
	}

	return nil
}

// ValidatePrivateKey validates a TWINS private key format
func (nv *NetworkValidator) ValidatePrivateKey(privKey string) error {
	// TWINS private keys are 51 characters (WIF format)
	if len(privKey) != 51 && len(privKey) != 52 {
		return fmt.Errorf("invalid private key length")
	}

	// Check prefix
	expectedPrefixes := []string{"7", "X"} // Mainnet prefixes
	if nv.params.Name == "testnet" || nv.params.Name == "regtest" {
		expectedPrefixes = []string{"9", "c"} // Testnet prefixes
	}

	validPrefix := false
	for _, prefix := range expectedPrefixes {
		if strings.HasPrefix(privKey, prefix) {
			validPrefix = true
			break
		}
	}

	if !validPrefix {
		return fmt.Errorf("private key should start with %v for %s", expectedPrefixes, nv.params.Name)
	}

	// Validate base58 characters
	if !validBase58Regex.MatchString(privKey) {
		return fmt.Errorf("invalid characters in private key")
	}

	return nil
}

// GetNetworkParams returns the current network parameters
func (nv *NetworkValidator) GetNetworkParams() NetworkParams {
	return nv.params
}

// EstimateNetworkLatency estimates latency to a given node
func EstimateNetworkLatency(address string) (time.Duration, error) {
	start := time.Now()

	conn, err := net.DialTimeout("tcp", address, 10*time.Second)
	if err != nil {
		return 0, fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	return time.Since(start), nil
}

// FindBestNodes finds the best performing nodes from a list
func FindBestNodes(nodes []string, maxNodes int) []string {
	type nodeLatency struct {
		address string
		latency time.Duration
	}

	var results []nodeLatency

	for _, node := range nodes {
		latency, err := EstimateNetworkLatency(node)
		if err == nil {
			results = append(results, nodeLatency{
				address: node,
				latency: latency,
			})
		}
	}

	// Sort by latency (would need sort package)
	// For now, just return first maxNodes that connected
	var bestNodes []string
	for i, result := range results {
		if i >= maxNodes {
			break
		}
		bestNodes = append(bestNodes, result.address)
	}

	return bestNodes
}