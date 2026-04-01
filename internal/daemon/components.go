// Package daemon provides shared component initialization for both twinsd and twins-gui.
// This ensures consistent initialization logic across CLI daemon and GUI application.
package daemon

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/sirupsen/logrus"

	"github.com/twins-dev/twins-core/internal/blockchain"
	"github.com/twins-dev/twins-core/internal/consensus"
	"github.com/twins-dev/twins-core/internal/masternode"
	"github.com/twins-dev/twins-core/internal/mempool"
	"github.com/twins-dev/twins-core/internal/p2p"
	"github.com/twins-dev/twins-core/internal/spork"
	"github.com/twins-dev/twins-core/internal/storage"
	"github.com/twins-dev/twins-core/internal/storage/binary"
	"github.com/twins-dev/twins-core/pkg/types"
)

// DBPath returns the standard database path for a given data directory
func DBPath(dataDir string) string {
	return filepath.Join(dataDir, "blockchain.db")
}

// Deprecated: CoreComponents is retained for the GUI bridge transition.
// New code should use the daemon.Node type directly. See node.go.
type CoreComponents struct {
	Storage    storage.Storage            // Database storage
	Blockchain blockchain.Blockchain      // Blockchain state manager
	Consensus  consensus.Engine           // PoS consensus engine
	Mempool    mempool.Mempool            // Transaction mempool
	Masternode *masternode.Manager        // Masternode manager
	Spork      *spork.Manager             // Spork (network parameter) manager
	P2PServer  *p2p.Server                // P2P network server (initialized after splash screen in GUI)
	Syncer     *p2p.BlockchainSyncer      // Blockchain synchronizer (initialized with P2P server)
	// Wallet is initialized via Node.InitWallet() and accessed through Node.Wallet
}

// InitConfig provides configuration for component initialization
type InitConfig struct {
	Network  string // "mainnet", "testnet", or "regtest"
	DataDir  string // Data directory path
	ReadOnly bool   // Open storage in read-only mode (for GUI)
	Logger   *logrus.Entry
}

// Deprecated: InitializeCoreComponents is retained for the GUI bridge transition.
// New code should use daemon.NewNode which centralizes all initialization. See node.go.
func InitializeCoreComponents(cfg *InitConfig) (*CoreComponents, error) {
	if cfg.Logger == nil {
		cfg.Logger = logrus.NewEntry(logrus.StandardLogger())
	}
	logger := cfg.Logger

	components := &CoreComponents{}

	// Initialize chain parameters
	chainParams, err := ResolveChainParams(cfg.Network)
	if err != nil {
		return nil, err
	}

	// Initialize storage
	dbPath := DBPath(cfg.DataDir)
	storageConfig := storage.DefaultStorageConfig()
	storageConfig.Path = dbPath
	storageConfig.ReadOnly = cfg.ReadOnly

	stor, err := binary.NewBinaryStorage(storageConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}
	components.Storage = stor

	// Ensure genesis block exists (skip for read-only mode)
	if !cfg.ReadOnly {
		if err := EnsureGenesisBlock(stor, chainParams, logger); err != nil {
			stor.Close()
			return nil, fmt.Errorf("failed to ensure genesis block: %w", err)
		}
	}

	// Initialize consensus engine
	consensusEngine := consensus.NewProofOfStake(stor, chainParams, logrus.StandardLogger())
	if err := consensusEngine.Start(context.Background()); err != nil {
		stor.Close()
		return nil, fmt.Errorf("failed to start consensus engine: %w", err)
	}
	components.Consensus = consensusEngine

	// Initialize blockchain
	blockchainConfig := blockchain.DefaultConfig()
	blockchainConfig.Storage = stor
	blockchainConfig.Consensus = consensusEngine
	blockchainConfig.ChainParams = chainParams
	blockchainConfig.Network = cfg.Network

	bc, err := blockchain.New(blockchainConfig)
	if err != nil {
		consensusEngine.Stop()
		stor.Close()
		return nil, fmt.Errorf("failed to initialize blockchain: %w", err)
	}
	components.Blockchain = bc

	// Initialize mempool (skip for read-only GUI mode)
	if !cfg.ReadOnly {
		mempoolConfig := mempool.DefaultConfig()
		mempoolConfig.Blockchain = bc
		mempoolConfig.Consensus = consensusEngine
		mempoolConfig.ChainParams = chainParams
		mempoolConfig.UTXOSet = bc
		mp, err := mempool.New(mempoolConfig)
		if err != nil {
			consensusEngine.Stop()
			stor.Close()
			return nil, fmt.Errorf("failed to initialize mempool: %w", err)
		}
		components.Mempool = mp
	}

	// Initialize spork manager (skip for read-only GUI mode)
	// CRITICAL: Must be initialized BEFORE masternode manager for tier validation
	if !cfg.ReadOnly {
		// Get underlying Pebble DB for spork storage
		// Note: stor is already *binary.BinaryStorage from NewBinaryStorage()
		sporkStorage := spork.NewPebbleStorage(stor.GetDB())

		sporkManager, err := spork.NewManager(
			chainParams.SporkPubKey,
			chainParams.SporkPubKeyOld,
			sporkStorage,
		)
		if err != nil {
			if components.Mempool != nil {
				components.Mempool.Stop()
			}
			consensusEngine.Stop()
			stor.Close()
			return nil, fmt.Errorf("failed to initialize spork manager: %w", err)
		}
		components.Spork = sporkManager
	}

	// Initialize masternode manager (skip for read-only GUI mode)
	if !cfg.ReadOnly {
		mnConfig := masternode.DefaultConfig()
		mnManager, err := masternode.NewManager(mnConfig, logrus.StandardLogger())
		if err != nil {
			if components.Mempool != nil {
				components.Mempool.Stop()
			}
			consensusEngine.Stop()
			stor.Close()
			return nil, fmt.Errorf("failed to initialize masternode manager: %w", err)
		}
		components.Masternode = mnManager
		mnManager.SetBlockchain(bc)

		// CRITICAL FIX: Wire spork manager to masternode manager
		// Without this, validateCollateralWithSpork() defaults tiersEnabled=false
		// and only Bronze tier is accepted. Legacy C++ always has sporks available.
		if components.Spork != nil {
			mnManager.SetSporkManager(components.Spork)
		}

		// Wire masternode payment validator
		paymentValidator := consensus.NewMasternodePaymentValidator(mnManager, chainParams)
		blockValidator := consensusEngine.GetBlockValidator()
		blockValidator.SetPaymentValidator(paymentValidator)
		consensusEngine.SetPaymentValidator(paymentValidator)
	}

	logger.Info("Core blockchain components initialized")
	return components, nil
}

// Close stops and closes all components in the correct order
func (c *CoreComponents) Close() error {
	var lastErr error

	// Stop P2P components first (they depend on other components)
	if c.Syncer != nil {
		c.Syncer.Stop()
	}

	if c.P2PServer != nil {
		c.P2PServer.Stop()
	}

	// Wallet cleanup would go here when implemented

	if c.Masternode != nil {
		c.Masternode.Stop()
	}

	if c.Mempool != nil {
		c.Mempool.Stop()
	}

	// Spork manager doesn't need explicit cleanup (no Stop method)

	if c.Consensus != nil {
		c.Consensus.Stop()
	}

	if c.Storage != nil {
		if err := c.Storage.Close(); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// EnsureGenesisBlock ensures the genesis block exists in storage
// This handles the dual-hash situation where genesis has both SHA256 and Quark hashes
func EnsureGenesisBlock(stor storage.Storage, chainParams *types.ChainParams, logger *logrus.Entry) error {
	// Check if genesis block exists under the canonical Quark hash
	genesisHash := chainParams.GenesisHash
	_, err := stor.GetBlock(genesisHash)
	if err == nil {
		logger.Debug("Genesis block already exists")
		return nil
	}

	// Genesis block doesn't exist at all
	// Create it from the known genesis block structure
	logger.Info("Genesis block not found - creating from known genesis data")

	// CRITICAL FIX: Store the genesis block under the Quark hash
	// The genesis block has TWO valid hashes due to legacy Quark hashing:
	// 1. SHA256 hash (calculated by block.Hash()) = actual SHA256 of the block
	// 2. Quark hash (the canonical hash from chainparams) = "0000071cf2..."
	//
	// The block DATA must be stored under the Quark hash (the canonical one)
	// so it can be retrieved when peers request it by that hash.

	genesisBlock := types.GenesisBlock(chainParams)
	sha256Hash := genesisBlock.Hash()

	batch := stor.NewBatch()
	binaryBatch := batch.(*binary.BinaryBatch)

	// CRITICAL: Store genesis block AND its transactions properly
	// StoreBlockWithHeight stores both compact block and all transactions separately
	// This ensures GetBlock can reconstruct the full block with transactions
	if err := binaryBatch.StoreBlockWithHeight(genesisBlock, 0); err != nil {
		return fmt.Errorf("failed to store genesis block with height: %w", err)
	}

	// CRITICAL: Genesis block has TWO valid hashes due to legacy Quark hashing:
	// 1. SHA256 hash (stored above by StoreBlockWithHeight)
	// 2. Quark hash (canonical hash from chainparams = "0000071cf2...")
	//
	// We need to store the block under BOTH hashes so it can be retrieved by either.
	// The transactions are already stored by StoreBlockWithHeight above.
	// We just need to store the compact block data under the Quark hash as well.

	if sha256Hash != genesisHash {
		// Recreate the compact block data to store under Quark hash
		compact := &binary.CompactBlock{
			Height:    0,
			Version:   genesisBlock.Header.Version,
			PrevBlock: genesisBlock.Header.PrevBlockHash,
			Merkle:    genesisBlock.Header.MerkleRoot,
			Timestamp: genesisBlock.Header.Timestamp,
			Bits:      genesisBlock.Header.Bits,
			Nonce:     genesisBlock.Header.Nonce,
			StakeMod:  0,
			StakeTime: 0,
			TxCount:   uint32(len(genesisBlock.Transactions)),
			TxHashes:  make([]types.Hash, len(genesisBlock.Transactions)),
		}

		for i, tx := range genesisBlock.Transactions {
			compact.TxHashes[i] = tx.Hash()
		}

		compactData, err := binary.EncodeCompactBlock(compact)
		if err != nil {
			return fmt.Errorf("failed to encode genesis compact block: %w", err)
		}

		// Store the compact block data under the Quark hash
		quarkBlockKey := binary.BlockKey(genesisHash)
		if err := binaryBatch.SetRaw(quarkBlockKey, compactData); err != nil {
			return fmt.Errorf("failed to store genesis under Quark hash: %w", err)
		}
	}

	// Index genesis at height 0 with BOTH hashes so it can be found either way
	// This ensures the block can be retrieved by either hash

	// Index with the canonical Quark hash (for protocol compatibility)
	if err := batch.StoreBlockIndex(genesisHash, 0); err != nil {
		return fmt.Errorf("failed to index genesis with Quark hash: %w", err)
	}

	// Also index with the SHA256 hash (for internal consistency)
	if err := batch.StoreBlockIndex(sha256Hash, 0); err != nil {
		return fmt.Errorf("failed to index genesis with SHA256 hash: %w", err)
	}

	// Set chain tip to the canonical Quark hash
	if err := batch.SetChainState(0, genesisHash); err != nil {
		return fmt.Errorf("failed to set chain tip to genesis: %w", err)
	}

	// Commit the batch
	if err := batch.Commit(); err != nil {
		return fmt.Errorf("failed to commit genesis: %w", err)
	}

	// CRITICAL: Sync storage to flush indices to disk
	// Without this, the height index may not be visible to subsequent reads
	if err := stor.Sync(); err != nil {
		return fmt.Errorf("failed to sync storage after genesis commit: %w", err)
	}

	logger.WithFields(logrus.Fields{
		"canonical_hash": genesisHash.String(),
		"storage_hash":   sha256Hash.String(),
		"height":         0,
	}).Info("Genesis block created and indexed")
	return nil
}
