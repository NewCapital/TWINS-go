package daemon

import (
	"fmt"
	"net"

	"github.com/twins-dev/twins-core/internal/masternode"
	"github.com/twins-dev/twins-core/pkg/crypto"
	"github.com/twins-dev/twins-core/pkg/types"
)

// MasternodeNotifierAdapter adapts masternode.Manager to blockchain.MasternodeNotifier interface.
// Go interfaces require exact method signatures, and masternode.Manager returns
// *MasternodeWinnerVote while the interface uses interface{} to avoid circular imports.
type MasternodeNotifierAdapter struct {
	Manager *masternode.Manager
}

// ProcessBlockForWinner implements blockchain.MasternodeNotifier.
func (a *MasternodeNotifierAdapter) ProcessBlockForWinner(currentHeight uint32) (interface{}, error) {
	return a.Manager.ProcessBlockForWinner(currentHeight)
}

// ActiveMasternodeAdapter adapts masternode.ActiveMasternode to rpc.ActiveMasternodeInterface.
type ActiveMasternodeAdapter struct {
	AM *masternode.ActiveMasternode
}

func (a *ActiveMasternodeAdapter) GetStatus() string        { return a.AM.GetStatus() }
func (a *ActiveMasternodeAdapter) IsStarted() bool          { return a.AM.IsStarted() }
func (a *ActiveMasternodeAdapter) GetVin() types.Outpoint   { return a.AM.GetVin() }
func (a *ActiveMasternodeAdapter) GetServiceAddr() net.Addr { return a.AM.GetServiceAddr() }
func (a *ActiveMasternodeAdapter) GetPubKeyMasternode() *crypto.PublicKey {
	return a.AM.GetPubKeyMasternode()
}
func (a *ActiveMasternodeAdapter) IsAutoManagementRunning() bool {
	return a.AM.IsAutoManagementRunning()
}
func (a *ActiveMasternodeAdapter) Initialize(privKeyWIF, serviceAddr string) error {
	return a.AM.Initialize(privKeyWIF, serviceAddr)
}
func (a *ActiveMasternodeAdapter) Start(collateralTx types.Hash, collateralIdx uint32, collateralKey interface{}) error {
	if collateralKey == nil {
		return a.AM.Start(collateralTx, collateralIdx, nil)
	}
	key, ok := collateralKey.(*crypto.PrivateKey)
	if !ok {
		return fmt.Errorf("collateralKey must be *crypto.PrivateKey")
	}
	return a.AM.Start(collateralTx, collateralIdx, key)
}
func (a *ActiveMasternodeAdapter) Stop()                            { a.AM.Stop() }
func (a *ActiveMasternodeAdapter) ManageStatus() error              { return a.AM.ManageStatus() }
func (a *ActiveMasternodeAdapter) SetSyncChecker(fn func() bool)    { a.AM.SetSyncChecker(fn) }
func (a *ActiveMasternodeAdapter) SetBalanceGetter(fn func() int64) { a.AM.SetBalanceGetter(fn) }
