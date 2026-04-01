package rpc

import (
	"github.com/twins-dev/twins-core/internal/masternode"
	"github.com/twins-dev/twins-core/pkg/types"
)

// BlockchainInterface defines the blockchain operations needed by RPC handlers
type BlockchainInterface interface {
	// Block retrieval
	GetBestBlock() (*types.Block, error)
	GetBestHeight() (uint32, error) // Add this method
	GetBestBlockHash() (types.Hash, error)
	GetBlock(hash types.Hash) (*types.Block, error)
	GetBlockByHeight(height uint32) (*types.Block, error)
	GetBlockHash(height uint32) (types.Hash, error)
	GetBlockHeight() (uint32, error)
	GetBlockHeightByHash(hash types.Hash) (uint32, error)

	// Chain info
	GetChainWork() (string, error)
	GetDifficulty() (float64, error)
	GetChainTips() ([]ChainTip, error)
	GetBlockCount() (int64, error)

	// Validation
	ValidateBlock(block *types.Block) error
	ProcessBlock(block *types.Block) error

	// UTXO operations
	GetUTXO(outpoint types.Outpoint) (*types.TxOutput, error)
	GetUTXOSet() (map[types.Outpoint]*types.TxOutput, error)

	// Transaction operations
	GetTransaction(hash types.Hash) (*types.Transaction, error)
	GetTransactionBlock(hash types.Hash) (*types.Block, error)
	GetRawTransaction(hash types.Hash) ([]byte, error)

	// Chain state
	GetChainParams() *types.ChainParams
	IsInitialBlockDownload() bool
	GetVerificationProgress() float64
	GetMoneySupply(height uint32) (int64, error)

	// Block invalidation and reconsideration
	InvalidateBlock(hash types.Hash) error
	ReconsiderBlock(hash types.Hash) error
	AddCheckpoint(height uint32, hash types.Hash) error
}

// ChainTip represents information about a chain tip
type ChainTip struct {
	Height    int64  `json:"height"`
	Hash      string `json:"hash"`
	BranchLen int    `json:"branchlen"`
	Status    string `json:"status"`
}

// MempoolInterface defines mempool operations needed by RPC handlers
type MempoolInterface interface {
	// Transaction operations
	AddTransaction(tx *types.Transaction) error
	GetTransaction(hash types.Hash) (*types.Transaction, bool)
	GetRawMempool() []types.Hash
	GetMempoolInfo() MempoolInfo
	GetMempoolEntry(hash types.Hash) (*MempoolEntry, error)
	RemoveTransaction(hash types.Hash)
	HasTransaction(hash types.Hash) bool
	GetTransactions() []*types.Transaction

	// Mempool queries
	GetMempoolAncestors(hash types.Hash) ([]types.Hash, error)
	GetMempoolDescendants(hash types.Hash) ([]types.Hash, error)

	// Validation
	ValidateTransaction(tx *types.Transaction) error

	// Stats
	GetMempoolSize() int
	GetMempoolBytes() uint64
	GetStats() interface{} // Returns mempool statistics

	// Priority management (for prioritisetransaction RPC)
	UpdatePriority(hash types.Hash, priorityDelta float64, feeDelta int64) error
}

// MempoolEntry represents a transaction in the mempool
type MempoolEntry struct {
	Size             int     `json:"size"`
	Fee              int64   `json:"fee"`
	ModifiedFee      int64   `json:"modifiedfee"`
	Time             int64   `json:"time"`
	Height           int64   `json:"height"`
	StartingPriority float64 `json:"startingpriority"`
	CurrentPriority  float64 `json:"currentpriority"`
	DescendantCount  int     `json:"descendantcount"`
	DescendantSize   int     `json:"descendantsize"`
	DescendantFees   int64   `json:"descendantfees"`
	AncestorCount    int     `json:"ancestorcount"`
	AncestorSize     int     `json:"ancestorsize"`
	AncestorFees     int64   `json:"ancestorfees"`
	Depends          []string `json:"depends"`
}

// ConsensusInterface defines consensus operations needed by RPC handlers
type ConsensusInterface interface {
	// Staking info
	GetStakingInfo() StakingInfo
	IsStaking() bool
	GetStakeWeight() int64
	GetNetworkStakeWeight() int64
	GetExpectedTime() int64

	// Staking control (for setgenerate RPC)
	StartStaking() error
	StopStaking() error

	// Mining info
	GetMiningInfo() MiningInfo
	GetNetworkHashPS(blocks int, height int) float64
	GetStats() interface{} // Returns consensus statistics

	// Block template
	GetBlockTemplate(request *BlockTemplateRequest) (*BlockTemplate, error)
	SubmitBlock(block *types.Block) error
	ValidateBlock(block *types.Block) error
}

// StakingInfo contains staking status information
type StakingInfo struct {
	Enabled          bool    `json:"enabled"`
	Staking          bool    `json:"staking"`
	Errors           string  `json:"errors,omitempty"`
	CurrentBlockSize int     `json:"currentblocksize"`
	CurrentBlockTx   int     `json:"currentblocktx"`
	Difficulty       float64 `json:"difficulty"`
	SearchInterval   int     `json:"search-interval"`
	Weight           int64   `json:"weight"`
	NetStakeWeight   int64   `json:"netstakeweight"`
	ExpectedTime     int64   `json:"expectedtime"`
}

// MiningInfo contains mining status information
type MiningInfo struct {
	Blocks             int64   `json:"blocks"`
	CurrentBlockSize   int     `json:"currentblocksize"`
	CurrentBlockWeight int     `json:"currentblockweight"`
	CurrentBlockTx     int     `json:"currentblocktx"`
	Difficulty         float64 `json:"difficulty"`
	Errors             string  `json:"errors,omitempty"`
	NetworkHashPS      float64 `json:"networkhashps"`
	PooledTx           int     `json:"pooledtx"`
	Chain              string  `json:"chain"`
}

// BlockTemplateRequest represents a getblocktemplate request
type BlockTemplateRequest struct {
	Mode         string   `json:"mode,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// BlockTemplate represents a block template for mining
type BlockTemplate struct {
	Version           int32    `json:"version"`
	PreviousBlockHash string   `json:"previousblockhash"`
	Transactions      []string `json:"transactions"`
	CoinbaseAux       map[string]string `json:"coinbaseaux,omitempty"`
	CoinbaseValue     int64    `json:"coinbasevalue"`
	Target            string   `json:"target"`
	MinTime           int64    `json:"mintime"`
	Mutable           []string `json:"mutable"`
	NonceRange        string   `json:"noncerange"`
	SigOpLimit        int      `json:"sigoplimit,omitempty"`
	SizeLimit         int      `json:"sizelimit,omitempty"`
	CurTime           int64    `json:"curtime"`
	Bits              string   `json:"bits"`
	Height            int64    `json:"height"`
}

// MasternodeInterface defines masternode operations needed by RPC handlers
type MasternodeInterface interface {
	// Masternode queries
	GetMasternodeCount() (int, int, int)  // enabled, total, stable
	GetMasternodeList(filter string) []MasternodeInfo
	GetMasternodeStatus(outpoint types.Outpoint) (*MasternodeStatus, error)
	GetMasternodeWinners(blocks int, filter string) []MasternodeWinner
	GetMasternodeScores(blocks int) []MasternodeScore

	// Additional methods needed by RPC
	GetMasternodes() map[types.Outpoint]*masternode.Masternode
	GetMasternodeInfo(outpoint types.Outpoint) (*masternode.MasternodeInfo, error)
	GetMasternodeCountByTier(tier masternode.MasternodeTier) int
	IsMasternodeActive(outpoint types.Outpoint) bool
	GetNextPayee() (*masternode.Masternode, error)
	GetNextPaymentWinner(blockHeight uint32, blockHash types.Hash) (*masternode.Masternode, error)
	ProcessBroadcast(broadcast *masternode.MasternodeBroadcast, originAddr string) error

	// Masternode operations
	StartMasternode(alias string, lockWallet bool) (string, error)
	StartMasternodeMany() (int, int)
	CreateMasternodeBroadcast(alias string) (string, error)
	RelayMasternodeBroadcast(hex string) error

	// Sync management
	ResetSync()
}

// MasternodeStatus represents the status of a masternode
type MasternodeStatus struct {
	Outpoint      types.Outpoint `json:"outpoint"`
	Service       string         `json:"service"`
	Payee         string         `json:"payee"`
	Status        string         `json:"status"`
	ProtocolVersion int          `json:"protocolversion"`
	LastSeen      int64          `json:"lastseen"`
	ActiveSeconds int64          `json:"activeseconds"`
	LastPaidTime  int64          `json:"lastpaidtime"`
	LastPaidBlock int64          `json:"lastpaidblock"`
	Tier          int            `json:"tier"`
}

// MasternodeWinner represents a masternode payment winner
type MasternodeWinner struct {
	Height   int    `json:"height"`
	Payee    string `json:"payee"`
	Votes    int    `json:"votes"`
}

// MasternodeScore represents a masternode's score
type MasternodeScore struct {
	Outpoint types.Outpoint `json:"outpoint"`
	Score    int64          `json:"score"`
}