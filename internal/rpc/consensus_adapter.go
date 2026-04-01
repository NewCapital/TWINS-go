package rpc

import (
	"fmt"

	"github.com/twins-dev/twins-core/internal/consensus"
	"github.com/twins-dev/twins-core/pkg/types"
)

// ConsensusAdapter adapts consensus.Engine to rpc.ConsensusInterface
type ConsensusAdapter struct {
	engine consensus.Engine
}

// NewConsensusAdapter creates a new consensus adapter
func NewConsensusAdapter(engine consensus.Engine) *ConsensusAdapter {
	return &ConsensusAdapter{engine: engine}
}

// GetStakingInfo returns staking information
func (a *ConsensusAdapter) GetStakingInfo() StakingInfo {
	stats := a.engine.GetStats()

	return StakingInfo{
		Enabled:          true,
		Staking:          stats.StakingActive,
		Errors:           "",
		CurrentBlockSize: 0,
		CurrentBlockTx:   0,
		Difficulty:       float64(stats.Difficulty),
		SearchInterval:   0,
		Weight:           int64(stats.MyWeight),
		NetStakeWeight:   int64(stats.NetworkWeight),
		ExpectedTime:     int64(stats.NextStakeTime.Sub(stats.LastBlockTime).Seconds()),
	}
}

// IsStaking returns whether staking is active
func (a *ConsensusAdapter) IsStaking() bool {
	return a.engine.IsStaking()
}

// GetStakeWeight returns the wallet's stake weight
func (a *ConsensusAdapter) GetStakeWeight() int64 {
	stats := a.engine.GetStats()
	return int64(stats.MyWeight)
}

// GetNetworkStakeWeight returns the network's total stake weight
func (a *ConsensusAdapter) GetNetworkStakeWeight() int64 {
	stats := a.engine.GetStats()
	return int64(stats.NetworkWeight)
}

// GetExpectedTime returns expected time to stake
func (a *ConsensusAdapter) GetExpectedTime() int64 {
	stats := a.engine.GetStats()
	if stats.NextStakeTime.IsZero() {
		return 0
	}
	return int64(stats.NextStakeTime.Sub(stats.LastBlockTime).Seconds())
}

// GetMiningInfo returns mining information
func (a *ConsensusAdapter) GetMiningInfo() MiningInfo {
	stats := a.engine.GetStats()

	return MiningInfo{
		Blocks:             int64(stats.BlocksProcessed),
		CurrentBlockSize:   0,
		CurrentBlockWeight: 0,
		CurrentBlockTx:     0,
		Difficulty:         float64(stats.Difficulty),
		Errors:             "",
		NetworkHashPS:      float64(stats.NetworkWeight),
		PooledTx:           0,
		Chain:              "main",
	}
}

// GetNetworkHashPS returns network hash rate (for PoS this is stake weight)
func (a *ConsensusAdapter) GetNetworkHashPS(blocks int, height int) float64 {
	stats := a.engine.GetStats()
	return float64(stats.NetworkWeight)
}

// GetStats returns consensus statistics
func (a *ConsensusAdapter) GetStats() interface{} {
	stats := a.engine.GetStats()

	return map[string]interface{}{
		"staking":           stats.StakingActive,
		"blocks_validated":  stats.BlocksValidated,
		"blocks_processed":  stats.BlocksProcessed,
		"network_weight":    stats.NetworkWeight,
		"my_weight":         stats.MyWeight,
		"difficulty":        stats.Difficulty,
		"last_block_time":   stats.LastBlockTime.Unix(),
		"next_stake_time":   stats.NextStakeTime.Unix(),
		"stake_modifier":    stats.StakeModifier,
		"active_stakers":    stats.ActiveStakers,
		"validation_time":   stats.ValidationTime.Milliseconds(),
		"processing_time":   stats.ProcessingTime.Milliseconds(),
	}
}

// GetBlockTemplate returns a block template for mining (PoS staking)
func (a *ConsensusAdapter) GetBlockTemplate(request *BlockTemplateRequest) (*BlockTemplate, error) {
	stats := a.engine.GetStats()
	nextTime := a.engine.GetNextBlockTime()

	return &BlockTemplate{
		Version:           1,
		PreviousBlockHash: "",
		Transactions:      []string{},
		CoinbaseAux:       make(map[string]string),
		CoinbaseValue:     int64(stats.StakeReward),
		Target:            "",
		MinTime:           nextTime.Unix(),
		Mutable:           []string{"time", "transactions"},
		NonceRange:        "00000000ffffffff",
		SigOpLimit:        20000,
		SizeLimit:         1000000,
		CurTime:           nextTime.Unix(),
		Bits:              fmt.Sprintf("%08x", stats.Difficulty),
		Height:            0,
	}, nil
}

// SubmitBlock submits a new block
func (a *ConsensusAdapter) SubmitBlock(block *types.Block) error {
	return a.engine.ValidateBlock(block)
}

// ValidateBlock validates a block
func (a *ConsensusAdapter) ValidateBlock(block *types.Block) error {
	return a.engine.ValidateBlock(block)
}

// StartStaking enables staking in the consensus engine
func (a *ConsensusAdapter) StartStaking() error {
	return a.engine.StartStaking()
}

// StopStaking disables staking in the consensus engine
func (a *ConsensusAdapter) StopStaking() error {
	return a.engine.StopStaking()
}
