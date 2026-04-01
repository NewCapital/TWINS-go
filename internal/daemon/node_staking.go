package daemon

import (
	"fmt"

	"github.com/twins-dev/twins-core/internal/consensus"
)

// StartStaking starts the consensus engine staking loop.
// If the wallet is encrypted and locked, staking is skipped with a warning
// (the GUI auto-starts staking on wallet unlock via UnlockWallet).
// This provides a single check for both daemon and GUI paths.
func (n *Node) StartStaking() error {
	posEngine, ok := n.Consensus.(*consensus.ProofOfStake)
	if !ok {
		return fmt.Errorf("consensus engine is not PoS")
	}

	// Staking requires an unlocked wallet to cryptographically sign
	// coinstake transactions during proof-of-stake block generation.
	if n.Wallet != nil && n.Wallet.IsEncrypted() && n.Wallet.IsLocked() {
		n.logger.Warn("Wallet is encrypted and locked, staking skipped (unlock wallet to enable)")
		return nil
	}

	// Wire mempool for including pending transactions in staked blocks
	if n.Mempool != nil {
		posEngine.SetMempool(n.Mempool)
	}

	n.logger.Info("Starting staking...")
	return posEngine.StartStaking()
}

// StopStaking stops the consensus engine staking loop.
func (n *Node) StopStaking() {
	if posEngine, ok := n.Consensus.(*consensus.ProofOfStake); ok {
		posEngine.StopStaking()
		n.logger.Info("Staking stopped")
	}
}
