package daemon

import (
	"fmt"

	"github.com/twins-dev/twins-core/pkg/types"
)

// ResolveChainParams returns the chain parameters for the given network name.
// Centralizes the network→chainParams mapping used by both daemon and GUI.
func ResolveChainParams(network string) (*types.ChainParams, error) {
	switch network {
	case "mainnet":
		return types.InitMainnetGenesis(), nil
	case "testnet":
		return types.InitTestnetGenesis(), nil
	case "regtest":
		return types.InitRegtestGenesis(), nil
	default:
		return nil, fmt.Errorf("unknown network: %s", network)
	}
}
