package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"

	twinslib "github.com/twins-dev/twins-core/internal/cli"
	"github.com/twins-dev/twins-core/internal/config"
	"github.com/twins-dev/twins-core/internal/rpc"
	"github.com/twins-dev/twins-core/pkg/types"
)

func main() {
	app := twinslib.CreateBaseApp("twinsd", "TWINS cryptocurrency daemon", twinslib.String())

	// Prepare daemon-specific flags
	daemonFlags := []cli.Flag{}
	daemonFlags = append(daemonFlags, twinslib.CommonDaemonFlags()...)
	daemonFlags = append(daemonFlags, twinslib.CommonWalletFlags()...)
	daemonFlags = append(daemonFlags, twinslib.DatabaseFlags()...)
	daemonFlags = append(daemonFlags, twinslib.PerformanceFlags()...)

	app.Commands = []*cli.Command{
		{
			Name:  "start",
			Usage: "Start the TWINS daemon",
			Flags: daemonFlags,
			Before: func(c *cli.Context) error {
				return twinslib.SetupLogging(c)
			},
			Action: func(c *cli.Context) error {
				return startDaemonImproved(c)
			},
		},
		{
			Name:  "stop",
			Usage: "Stop the TWINS daemon",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "rpcconnect",
					Value: fmt.Sprintf("%s:%d", types.DefaultRPCHost, types.DefaultRPCPort),
					Usage: "RPC server address",
				},
			},
			Action: func(c *cli.Context) error {
				return stopDaemon(c)
			},
		},
		{
			Name:  "status",
			Usage: "Show daemon status",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:  "rpcconnect",
					Value: fmt.Sprintf("%s:%d", types.DefaultRPCHost, types.DefaultRPCPort),
					Usage: "RPC server address",
				},
			},
			Action: func(c *cli.Context) error {
				return getDaemonStatus(c)
			},
		},
		{
			Name:  "version",
			Usage: "Show version information",
			Action: func(c *cli.Context) error {
				twinslib.PrintVersion()
				return nil
			},
		},
	}

	// Propagate app-level flags to all subcommands so they work regardless
	// of position (e.g., "twinsd start --config=/path" works like
	// "twinsd --config=/path start").
	twinslib.PropagateAppFlags(app)

	if err := app.Run(os.Args); err != nil {
		logrus.WithError(err).Fatal("Application failed")
	}
}

// CLIOnlyConfig holds transient settings not persisted in twinsd.yml.
// These are structural or one-time flags that don't belong in ConfigManager.
type CLIOnlyConfig struct {
	Network           string // structural, determines chain params
	DataDir           string
	Workers           int
	ValidationWorkers int
	DbCacheMB         int
	Reindex           bool
	ExplicitConfig    bool // true when --config was explicitly set on CLI
}

// buildConfigManager creates a ConfigManager loaded from twinsd.yml and applies CLI overrides.
// Returns the manager and transient CLI-only flags (including whether --config was explicitly set).
func buildConfigManager(c *cli.Context, dataDir string) (*config.ConfigManager, *CLIOnlyConfig, error) {
	// Determine YAML path
	var yamlPath string
	explicitConfig := twinslib.ConfigWasExplicitlySet(c)
	if explicitConfig {
		yamlPath = twinslib.GetConfigPath(c)
	} else {
		yamlPath = filepath.Join(dataDir, "twinsd.yml")
	}

	logger := logrus.WithField("component", "config")
	cm := config.NewConfigManager(yamlPath, logger)

	// Load existing YAML or create defaults
	if err := cm.LoadOrCreate(); err != nil {
		if explicitConfig {
			return nil, nil, fmt.Errorf("failed to load config file %s: %w", yamlPath, err)
		}
		logrus.WithError(err).WithField("path", yamlPath).Warn("Failed to load config, using defaults")
	}

	// Apply CLI flag overrides via SetFromCLI (marks them as locked in GUI)
	if c.IsSet("staking") {
		if err := cm.SetFromCLI("staking.enabled", c.Bool("staking")); err != nil {
			return nil, nil, fmt.Errorf("failed to apply --staking: %w", err)
		}
	}

	if c.IsSet("reservebalance") {
		rb := c.Int64("reservebalance")
		if rb < 0 {
			return nil, nil, fmt.Errorf("invalid value for --reservebalance: must be >= 0, got %d", rb)
		}
		if rb > types.MaxReserveBalanceSatoshis {
			return nil, nil, fmt.Errorf("invalid value for --reservebalance: cannot exceed 100M TWINS (%d satoshis), got %d", types.MaxReserveBalanceSatoshis, rb)
		}
		if err := cm.SetFromCLI("staking.reserveBalance", rb); err != nil {
			return nil, nil, fmt.Errorf("failed to apply --reservebalance: %w", err)
		}
	}

	if c.IsSet("masternode") {
		if err := cm.SetFromCLI("masternode.enabled", c.Bool("masternode")); err != nil {
			return nil, nil, fmt.Errorf("failed to apply --masternode: %w", err)
		}
	}

	if c.IsSet("disablewallet") {
		// Inverted: --disablewallet=true means wallet.enabled=false
		if err := cm.SetFromCLI("wallet.enabled", !c.Bool("disablewallet")); err != nil {
			return nil, nil, fmt.Errorf("failed to apply --disablewallet: %w", err)
		}
	}

	if c.IsSet("log-level") {
		if err := cm.SetFromCLI("logging.level", c.String("log-level")); err != nil {
			return nil, nil, fmt.Errorf("failed to apply --log-level: %w", err)
		}
	}

	if c.IsSet("bind") {
		host, port, err := splitHostPort(c.String("bind"), types.DefaultRPCPort)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid --bind address: %w", err)
		}
		if err := cm.SetFromCLI("rpc.host", host); err != nil {
			return nil, nil, fmt.Errorf("failed to apply --bind host: %w", err)
		}
		if err := cm.SetFromCLI("rpc.port", port); err != nil {
			return nil, nil, fmt.Errorf("failed to apply --bind port: %w", err)
		}
	}

	if c.IsSet("p2p-bind") {
		host, port, err := splitHostPort(c.String("p2p-bind"), types.DefaultP2PPort)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid --p2p-bind address: %w", err)
		}
		if err := cm.SetFromCLI("network.listenAddr", host); err != nil {
			return nil, nil, fmt.Errorf("failed to apply --p2p-bind host: %w", err)
		}
		if err := cm.SetFromCLI("network.port", port); err != nil {
			return nil, nil, fmt.Errorf("failed to apply --p2p-bind port: %w", err)
		}
	}

	if c.IsSet("rpc-user") {
		if err := cm.SetFromCLI("rpc.username", c.String("rpc-user")); err != nil {
			return nil, nil, fmt.Errorf("failed to apply --rpc-user: %w", err)
		}
	}

	if c.IsSet("rpc-password") {
		if err := cm.SetFromCLI("rpc.password", c.String("rpc-password")); err != nil {
			return nil, nil, fmt.Errorf("failed to apply --rpc-password: %w", err)
		}
	}

	if c.IsSet("masternode-key") {
		if err := cm.SetFromCLI("masternode.privateKey", c.String("masternode-key")); err != nil {
			return nil, nil, fmt.Errorf("failed to apply --masternode-key: %w", err)
		}
	}

	if c.IsSet("paytxfee") {
		if err := cm.SetFromCLI("wallet.payTxFee", c.Int64("paytxfee")); err != nil {
			return nil, nil, fmt.Errorf("failed to apply --paytxfee: %w", err)
		}
	}

	if c.IsSet("mintxfee") {
		if err := cm.SetFromCLI("wallet.minTxFee", c.Int64("mintxfee")); err != nil {
			return nil, nil, fmt.Errorf("failed to apply --mintxfee: %w", err)
		}
	}

	if c.IsSet("maxtxfee") {
		if err := cm.SetFromCLI("wallet.maxTxFee", c.Int64("maxtxfee")); err != nil {
			return nil, nil, fmt.Errorf("failed to apply --maxtxfee: %w", err)
		}
	}

	// Reject legacy --rpcssl* flags with migration error
	if err := rejectLegacyRPCSSLFlags(c); err != nil {
		return nil, nil, err
	}

	// Apply RPC TLS flag overrides
	if c.IsSet("rpc-tls-enabled") {
		if err := cm.SetFromCLI("rpc.tls.enabled", c.Bool("rpc-tls-enabled")); err != nil {
			return nil, nil, fmt.Errorf("failed to apply --rpc-tls-enabled: %w", err)
		}
	}

	if c.IsSet("rpc-tls-cert") {
		if err := cm.SetFromCLI("rpc.tls.certFile", c.String("rpc-tls-cert")); err != nil {
			return nil, nil, fmt.Errorf("failed to apply --rpc-tls-cert: %w", err)
		}
	}

	if c.IsSet("rpc-tls-key") {
		if err := cm.SetFromCLI("rpc.tls.keyFile", c.String("rpc-tls-key")); err != nil {
			return nil, nil, fmt.Errorf("failed to apply --rpc-tls-key: %w", err)
		}
	}

	if c.IsSet("rpc-tls-expiry-warn-days") {
		if err := cm.SetFromCLI("rpc.tls.expiryWarnDays", c.Int("rpc-tls-expiry-warn-days")); err != nil {
			return nil, nil, fmt.Errorf("failed to apply --rpc-tls-expiry-warn-days: %w", err)
		}
	}

	if c.IsSet("rpc-tls-reload-passphrase-file") {
		if err := cm.SetFromCLI("rpc.tls.reloadPassphraseFile", c.String("rpc-tls-reload-passphrase-file")); err != nil {
			return nil, nil, fmt.Errorf("failed to apply --rpc-tls-reload-passphrase-file: %w", err)
		}
	}

	if c.IsSet("rpc-tls-mtls-enabled") {
		if err := cm.SetFromCLI("rpc.tls.mtls.enabled", c.Bool("rpc-tls-mtls-enabled")); err != nil {
			return nil, nil, fmt.Errorf("failed to apply --rpc-tls-mtls-enabled: %w", err)
		}
	}

	if c.IsSet("rpc-tls-mtls-client-ca") {
		if err := cm.SetFromCLI("rpc.tls.mtls.clientCAFile", c.String("rpc-tls-mtls-client-ca")); err != nil {
			return nil, nil, fmt.Errorf("failed to apply --rpc-tls-mtls-client-ca: %w", err)
		}
	}

	if c.IsSet("rpc-allow-plaintext-public") {
		if err := cm.SetFromCLI("rpc.allowPlaintextPublic", c.Bool("rpc-allow-plaintext-public")); err != nil {
			return nil, nil, fmt.Errorf("failed to apply --rpc-allow-plaintext-public: %w", err)
		}
	}

	cliOnly := &CLIOnlyConfig{
		Network:           twinslib.GetNetwork(c),
		DataDir:           dataDir,
		Workers:           c.Int("workers"),
		ValidationWorkers: c.Int("validation-workers"),
		DbCacheMB:         c.Int("db-cache"),
		Reindex:           c.Bool("reindex"),
		ExplicitConfig:    explicitConfig,
	}

	return cm, cliOnly, nil
}

// splitHostPort splits an address into host and port, using defaultPort if no port specified.
// Supports IPv4 ("1.2.3.4:8080"), IPv6 with brackets ("[::1]:8080"), and host-only ("1.2.3.4").
// Bare IPv6 without brackets (e.g. "::1") is detected and treated as host-only with defaultPort.
func splitHostPort(addr string, defaultPort int) (string, int, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		// Detect bare IPv6 address (contains ":" but no brackets or port separator).
		// net.SplitHostPort fails on these; treat as host-only with default port.
		if ip := net.ParseIP(addr); ip != nil {
			return addr, defaultPort, nil
		}
		// Plain hostname or IPv4 without port
		if !strings.Contains(addr, ":") {
			return addr, defaultPort, nil
		}
		return "", 0, fmt.Errorf("invalid address %q: use host:port or [ipv6]:port format", addr)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port %q: %w", portStr, err)
	}
	return host, port, nil
}

// rejectLegacyRPCSSLFlags checks for deprecated --rpcssl* flags and returns an error
// with a migration message pointing to the modern equivalent. The legacy C++ daemon
// never implemented these flags (hard-rejects at httpserver.cpp:384-389); we match
// that behavior exactly.
func rejectLegacyRPCSSLFlags(c *cli.Context) error {
	legacyFlags := []struct {
		flag    string
		message string
	}{
		{"rpcssl", "flag --rpcssl is no longer supported; use --rpc-tls-enabled instead"},
		{"rpcsslcertificatechainfile", "flag --rpcsslcertificatechainfile is no longer supported; use --rpc-tls-cert instead"},
		{"rpcsslprivatekeyfile", "flag --rpcsslprivatekeyfile is no longer supported; use --rpc-tls-key instead"},
		{"rpcsslciphers", "flag --rpcsslciphers is no longer supported; TLS 1.3 manages cipher selection automatically"},
	}
	for _, lf := range legacyFlags {
		if c.IsSet(lf.flag) {
			return fmt.Errorf("%s", lf.message)
		}
	}
	return nil
}

// stopDaemon stops a running daemon via RPC
func stopDaemon(c *cli.Context) error {
	rpcAddr := c.String("rpcconnect")

	// Simple RPC call to stop command
	client := rpc.NewClient(rpcAddr, "", "")

	ctx, cancel := context.WithTimeout(context.Background(), types.RPCCallTimeout)
	defer cancel()

	if err := client.Call(ctx, "stop", nil, nil); err != nil {
		return fmt.Errorf("failed to stop daemon: %w", err)
	}

	fmt.Println("✓ TWINS daemon stopped")
	return nil
}

// getDaemonStatus gets the status of a running daemon via RPC
func getDaemonStatus(c *cli.Context) error {
	rpcAddr := c.String("rpcconnect")

	client := rpc.NewClient(rpcAddr, "", "")

	ctx, cancel := context.WithTimeout(context.Background(), types.RPCStatusTimeout)
	defer cancel()

	// Get blockchain info
	var info map[string]interface{}
	if err := client.Call(ctx, "getblockchaininfo", nil, &info); err != nil {
		fmt.Printf("✗ Daemon is not running or not reachable at %s\n", rpcAddr)
		return fmt.Errorf("failed to get status: %w", err)
	}

	fmt.Println("✓ TWINS daemon is running")
	if blocks, ok := info["blocks"].(float64); ok {
		fmt.Printf("  Blocks: %.0f\n", blocks)
	}
	if chain, ok := info["chain"].(string); ok {
		fmt.Printf("  Chain: %s\n", chain)
	}

	return nil
}
