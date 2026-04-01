package consensus

import (
	"github.com/twins-dev/twins-core/internal/wallet"
	"github.com/twins-dev/twins-core/pkg/crypto"
	"github.com/twins-dev/twins-core/pkg/types"
)

// StakingWalletAdapter adapts wallet.Wallet to StakingWalletInterface.
// This is necessary because wallet.StakeableUTXO and consensus.StakeableUTXO are
// separate types (to avoid import cycles) but have identical structure.
type StakingWalletAdapter struct {
	w *wallet.Wallet
}

// NewStakingWalletAdapter creates a new adapter for staking operations.
func NewStakingWalletAdapter(w *wallet.Wallet) *StakingWalletAdapter {
	return &StakingWalletAdapter{w: w}
}

// IsLocked returns true if the wallet is encrypted and locked.
func (a *StakingWalletAdapter) IsLocked() bool {
	return a.w.IsLocked()
}

// GetStakeableUTXOs returns UTXOs eligible for staking.
func (a *StakingWalletAdapter) GetStakeableUTXOs(chainHeight uint32, chainTime uint32) ([]*StakeableUTXO, error) {
	walletUTXOs, err := a.w.GetStakeableUTXOs(chainHeight, chainTime)
	if err != nil {
		return nil, err
	}

	// Convert wallet.StakeableUTXO to consensus.StakeableUTXO
	result := make([]*StakeableUTXO, len(walletUTXOs))
	for i, wu := range walletUTXOs {
		result[i] = &StakeableUTXO{
			Outpoint:      wu.Outpoint,
			Amount:        wu.Amount,
			Address:       wu.Address,
			BlockHeight:   wu.BlockHeight,
			BlockTime:     wu.BlockTime,
			Confirmations: wu.Confirmations,
			CoinAge:       wu.CoinAge,
			ScriptPubKey:  wu.ScriptPubKey,
		}
	}
	return result, nil
}

// CreateCoinstakeTx builds the coinstake transaction for a valid stake.
func (a *StakingWalletAdapter) CreateCoinstakeTx(
	stakeUTXO *StakeableUTXO,
	blockReward int64,
	blockTime uint32,
) (*types.Transaction, error) {
	// Convert consensus.StakeableUTXO to wallet.StakeableUTXO
	walletUTXO := &wallet.StakeableUTXO{
		Outpoint:      stakeUTXO.Outpoint,
		Amount:        stakeUTXO.Amount,
		Address:       stakeUTXO.Address,
		BlockHeight:   stakeUTXO.BlockHeight,
		BlockTime:     stakeUTXO.BlockTime,
		Confirmations: stakeUTXO.Confirmations,
		CoinAge:       stakeUTXO.CoinAge,
		ScriptPubKey:  stakeUTXO.ScriptPubKey,
	}
	return a.w.CreateCoinstakeTx(walletUTXO, blockReward, blockTime)
}

// SignCoinstakeTx signs an unsigned coinstake transaction.
// Must be called AFTER all outputs are added (masternode, dev fund, etc.)
func (a *StakingWalletAdapter) SignCoinstakeTx(tx *types.Transaction, stakeUTXO *StakeableUTXO) error {
	// Convert consensus.StakeableUTXO to wallet.StakeableUTXO
	walletUTXO := &wallet.StakeableUTXO{
		Outpoint:      stakeUTXO.Outpoint,
		Amount:        stakeUTXO.Amount,
		Address:       stakeUTXO.Address,
		BlockHeight:   stakeUTXO.BlockHeight,
		BlockTime:     stakeUTXO.BlockTime,
		Confirmations: stakeUTXO.Confirmations,
		CoinAge:       stakeUTXO.CoinAge,
		ScriptPubKey:  stakeUTXO.ScriptPubKey,
	}
	return a.w.SignCoinstakeTx(tx, walletUTXO)
}

// GetPrivateKeyForAddress returns the private key for signing blocks.
func (a *StakingWalletAdapter) GetPrivateKeyForAddress(address string) (*crypto.PrivateKey, error) {
	return a.w.GetPrivateKeyForAddress(address)
}

// SignMessageBytes signs arbitrary data with the key for the given address.
func (a *StakingWalletAdapter) SignMessageBytes(address string, message []byte) ([]byte, error) {
	return a.w.SignMessageBytes(address, message)
}
