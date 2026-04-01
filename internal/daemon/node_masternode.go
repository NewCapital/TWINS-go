package daemon

import (
	"errors"

	"github.com/twins-dev/twins-core/internal/masternode"
)

// LoadMasternodeCache loads masternode data from disk (mncache.dat and mnpayments.dat).
// This replaces:
//   - cmd/twinsd/startup_improved.go:startMasternodeManager() (cache loading part)
//   - cmd/twins-gui/app.go masternode cache loading
func (n *Node) LoadMasternodeCache() error {
	if n.Masternode == nil || n.ChainParams == nil {
		return nil
	}

	networkMagic := n.ChainParams.NetMagicBytes[:]

	// Start masternode manager
	if err := n.Masternode.Start(); err != nil {
		return err
	}

	// Load masternode cache (mncache.dat)
	if err := n.Masternode.LoadCache(n.Config.DataDir, networkMagic); err != nil {
		if !errors.Is(err, masternode.ErrCacheFileNotFound) {
			n.logger.WithError(err).Warn("Failed to load masternode cache")
		} else {
			n.logger.Debug("No existing masternode cache found")
		}
	} else {
		n.logger.Debug("Loaded masternode cache from mncache.dat")

		// Notify sync manager about loaded cache for quick-restart skip.
		if syncMgr := n.Masternode.GetSyncManager(); syncMgr != nil {
			loadedAt, count := n.Masternode.GetCacheInfo()
			syncMgr.NotifyCacheLoaded(loadedAt, count)

			// Restore persisted fulfilled request maps
			mnsync, mnwsync := n.Masternode.GetCachedFulfilledMaps()
			if len(mnsync) > 0 || len(mnwsync) > 0 {
				syncMgr.SetFulfilledMaps(mnsync, mnwsync)
			}
		}
	}

	// Load payment votes (mnpayments.dat)
	if err := n.Masternode.LoadPaymentVotes(n.Config.DataDir, networkMagic); err != nil {
		if !errors.Is(err, masternode.ErrPaymentCacheNotFound) {
			n.logger.WithError(err).Warn("Failed to load payment votes")
		} else {
			n.logger.Debug("No existing payment votes found")
		}
	} else {
		n.logger.Debug("Loaded payment votes from mnpayments.dat")
	}

	n.logger.Debug("Masternode manager ready")
	return nil
}

// LoadMasternodeConf loads masternode.conf and wires collateral filtering.
// Must be called AFTER wallet initialization for proper integration.
// This replaces:
//   - cmd/twinsd/startup_improved.go:initializeMasternodeConf()
func (n *Node) LoadMasternodeConf() {
	baseDir := n.Config.DataDir

	// Read mnConf filename from ConfigManager (user-configurable), fall back to default
	mnConfName := "masternode.conf"
	if n.ConfigManager != nil {
		if v := n.ConfigManager.GetString("masternode.mnConf"); v != "" {
			mnConfName = v
		}
	}
	confFile := masternode.NewMasternodeConfFile(baseDir, mnConfName)

	if err := confFile.Read(); err != nil {
		n.logger.WithError(err).Debug("No masternode.conf found or read error (this is normal if not running masternodes)")
		n.mu.Lock()
		n.MasternodeConf = confFile
		n.mu.Unlock()
		return
	}

	n.mu.Lock()
	n.MasternodeConf = confFile
	n.mu.Unlock()

	// Wire masternode.conf to masternode manager for collateral checking
	if n.Masternode != nil {
		n.Masternode.SetConfFile(confFile)
	}

	// Wire wallet integration for collateral UTXO locking if mnConfLock is enabled.
	// When mnConfLock is false, collateral UTXOs remain spendable (not locked).
	mnConfLock := true
	if n.ConfigManager != nil {
		mnConfLock = n.ConfigManager.GetBool("masternode.mnConfLock")
	}

	n.mu.RLock()
	w := n.Wallet
	mnManager := n.Masternode
	n.mu.RUnlock()

	if w != nil && mnManager != nil && mnConfLock {
		w.SetMasternodeManager(mnManager)
		n.logger.Debug("Wallet integrated with masternode manager for collateral filtering")
	} else if !mnConfLock {
		n.logger.Debug("Masternode collateral locking disabled (mnConfLock=false)")
	}

	if confFile.GetCount() == 0 {
		n.logger.Debug("masternode.conf is empty")
		return
	}

	n.logger.WithField("entries", confFile.GetCount()).Debug("Loaded masternode.conf")
}
