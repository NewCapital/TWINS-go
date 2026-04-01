package blockchain

// Block processing constants
const (
	// ValidationHeightThreshold is the minimum block height after which
	// periodic validation is performed during IBD (Initial Block Download).
	// Below this height, simplified validation is always used for performance.
	ValidationHeightThreshold = 200000

	// ValidationFrequency determines how often full validation is performed
	// during IBD. Every Nth block after ValidationHeightThreshold gets validated.
	// This balances security (catching invalid blocks) with sync performance.
	ValidationFrequency = 1000

	// MaxBatchSizeForOptimalPerformance is the maximum number of blocks
	// processed in a single batch for optimal Pebble database performance.
	// Larger batches increase memory usage; smaller batches increase I/O overhead.
	MaxBatchSizeForOptimalPerformance = 500

	// MaxOrphanProcessingDepth limits recursion when processing orphan blocks.
	// This prevents stack overflow from maliciously crafted orphan chains
	// and bounds memory usage during orphan resolution.
	MaxOrphanProcessingDepth = 100
)
