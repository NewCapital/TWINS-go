package startup

import (
	"context"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/twins-dev/twins-core/internal/config"
	"github.com/twins-dev/twins-core/internal/daemon"
)

// Config configures the unified startup sequence.
type Config struct {
	// Node configuration
	Network string // "mainnet", "testnet", or "regtest"
	DataDir string
	Logger  *logrus.Entry
	Reindex bool

	// OnProgress is called during initialization phases for GUI splash screen.
	// Daemon callers can pass nil (no progress reporting) or a logging callback.
	OnProgress func(phase string, pct float64)

	// Performance tuning (from CLI flags, 0 = use defaults)
	Workers           int // Number of worker goroutines
	ValidationWorkers int // Number of block validation workers
	DbCacheMB         int // Database cache size in MB

	// Masternode debug (zero-cost when disabled)
	MasternodeDebug         bool
	MasternodeDebugMaxMB    int
	MasternodeDebugMaxFiles int

	// Wallet configuration (skipped if DisableWallet is true)
	DisableWallet bool
	WalletConfig  daemon.WalletConfig

	// P2P configuration
	P2PConfig daemon.P2PConfig

	// Staking: if true, start staking after all components are ready.
	Staking bool

	// ConfigManager is the unified configuration authority (optional, nil for GUI until Phase 3).
	ConfigManager *config.ConfigManager

	// RPCConfig: if non-nil, start the RPC server as the final step.
	// GUI typically passes nil here and starts RPC via node.InitRPC() later.
	RPCConfig *daemon.RPCConfig
}

// Start runs the unified startup sequence used by both twinsd and twins-gui.
// It creates a Node, validates the chain, initializes wallet and masternodes,
// runs mempool and P2P in parallel, and optionally starts staking and RPC.
//
// After Start returns, the caller owns the Node and is responsible for:
//   - Daemon: waiting for shutdown signal, then calling node.Shutdown()
//   - GUI: CoreClient wiring, collateral setup, then calling node.Shutdown()
func Start(ctx context.Context, cfg Config) (*daemon.Node, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = logrus.NewEntry(logrus.StandardLogger())
	}

	progress := func(phase string, pct float64) {
		if cfg.OnProgress != nil {
			cfg.OnProgress(phase, pct)
		}
	}

	// Phase 1: Create Node with all core components
	logger.Debug("Phase 1: Initializing core blockchain components...")
	progress("core", 0)

	node, err := daemon.NewNode(daemon.NodeConfig{
		Network:                 cfg.Network,
		DataDir:                 cfg.DataDir,
		Logger:                  logger,
		OnProgress:              cfg.OnProgress,
		Reindex:                 cfg.Reindex,
		Workers:                 cfg.Workers,
		ValidationWorkers:       cfg.ValidationWorkers,
		DbCacheMB:               cfg.DbCacheMB,
		ConfigManager:           cfg.ConfigManager,
		MasternodeDebug:         cfg.MasternodeDebug,
		MasternodeDebugMaxMB:    cfg.MasternodeDebugMaxMB,
		MasternodeDebugMaxFiles: cfg.MasternodeDebugMaxFiles,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize core components: %w", err)
	}

	// Phase 2: Validate chain integrity (always runs for both daemon and GUI)
	logger.Debug("Phase 2: Validating chain integrity...")
	progress("validation", 0)

	if err := node.ValidateChain(ctx); err != nil {
		node.Shutdown()
		return nil, fmt.Errorf("chain validation failed: %w", err)
	}

	// Phase 3a: Wallet initialization (sequential — must complete before P2P
	// starts syncer, otherwise SetWallet/SetBroadcaster race with block processing)
	if !cfg.DisableWallet {
		logger.Debug("Phase 3a: Initializing wallet...")
		progress("wallet", 0)

		if err := node.InitWallet(cfg.WalletConfig); err != nil {
			node.Shutdown()
			return nil, fmt.Errorf("wallet initialization failed: %w", err)
		}
	}

	// Phase 3b: Load masternode cache (must complete before P2P so cached
	// masternode addresses can be injected as priority bootstrap peers)
	logger.Debug("Phase 3b: Loading masternode cache...")
	progress("mncache", 0)

	if err := node.LoadMasternodeCache(); err != nil {
		// Non-fatal: daemon can still operate without cached masternode data.
		logger.WithError(err).Warn("Masternode cache loading failed, continuing without cached MN peers")
	}

	// Phase 4: Start mempool and P2P in parallel.
	// Mempool failure is fatal; P2P failure is non-fatal (node can operate offline).
	logger.Debug("Phase 4: Starting mempool and P2P in parallel...")
	progress("parallel", 0)

	var p2pErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		p2pErr = node.InitP2P(ctx, cfg.P2PConfig)
	}()

	if err := node.Mempool.Start(); err != nil {
		wg.Wait()
		node.Shutdown()
		return nil, fmt.Errorf("mempool start failed: %w", err)
	}
	wg.Wait()

	if p2pErr != nil {
		logger.WithError(p2pErr).Error("P2P initialization failed, continuing without network")
	}

	// Phase 4.5: Load masternode.conf (must be after wallet is ready)
	logger.Debug("Phase 4.5: Loading masternode configuration...")
	progress("mnconf", 0)
	node.LoadMasternodeConf()

	// Phase 5a: Wire ConfigManager subscribers for hot-reload (staking, logging, fees).
	// Must be before staking start so subscribers are registered before any future Set() calls.
	if node.ConfigManager != nil {
		node.WireConfigSubscribers()
	}

	// Phase 5b: Optional staking (StartStaking logs its own status including wallet-locked skip)
	if cfg.Staking {
		if err := node.StartStaking(); err != nil {
			logger.WithError(err).Warn("Failed to start staking")
		}
	}

	// Phase 6: Optional RPC server
	if cfg.RPCConfig != nil {
		logger.Debug("Phase 6: Starting RPC server...")
		if err := node.InitRPC(*cfg.RPCConfig); err != nil {
			node.Shutdown()
			return nil, fmt.Errorf("failed to start RPC server: %w", err)
		}
	}

	logger.Info("Startup sequence completed")
	return node, nil
}
