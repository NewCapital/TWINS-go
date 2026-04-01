package blockchain

import (
	"errors"
	"fmt"
)

// Block processing errors
var (
	// ErrBlockExists indicates the block already exists in the chain
	// This is not necessarily an error - it means we can skip processing
	ErrBlockExists = errors.New("block already exists")

	// ErrAllBlocksExist indicates all blocks in a batch already exist
	// This signals to the sync layer that fork detection should be triggered
	// The node may be on a different fork than the peer
	ErrAllBlocksExist = errors.New("all blocks in batch already exist")

	// ErrParentNotFound indicates the parent block is not in the chain
	// This is a fatal error indicating a gap in the chain
	ErrParentNotFound = errors.New("parent block not found")

	// ErrCheckpointFailed indicates checkpoint validation failed
	// This means the peer is on a wrong fork
	ErrCheckpointFailed = errors.New("checkpoint validation failed")

	// ErrInvalidBlock indicates block validation failed
	// This means the block is cryptographically invalid or violates consensus rules
	ErrInvalidBlock = errors.New("block validation failed")

	// ErrSequencingGap indicates a batch sequencing issue
	// This is not the peer's fault - we requested the wrong range
	ErrSequencingGap = errors.New("batch sequencing gap")

	// ErrHeightNotAdvancing indicates a single block was rejected because its
	// height does not advance beyond the current chain tip. The block is NOT
	// stored. This is distinct from ErrBlockExists (block already in storage).
	ErrHeightNotAdvancing = errors.New("block height does not advance chain tip")

	// ErrUTXONotFound indicates a UTXO is missing during block processing
	// This is a critical error indicating database corruption or missing blocks
	// Should trigger recovery/reorg
	ErrUTXONotFound = errors.New("UTXO not found")

	// ErrTransactionNotFound indicates a transaction is missing from storage
	// This is a critical error when a block header exists but its transactions are missing
	// This indicates database corruption (incomplete block storage)
	// Should trigger recovery/rollback
	ErrTransactionNotFound = errors.New("transaction not found")
)

// ErrForkDuplicateSpend indicates the same transaction spends the same UTXO
// at a different block height, signaling that our chain contains a fork block.
// The ForkHeight field points to the block in our chain that should be rolled back.
type ErrForkDuplicateSpend struct {
	ForkHeight uint32 // height of the fork block in our chain
	TxHash     string // spending transaction hash
	Outpoint   string // the UTXO being double-spent
}

func (e *ErrForkDuplicateSpend) Error() string {
	return fmt.Sprintf("fork detected: tx %s spends UTXO %s at height %d (fork block in our chain)",
		e.TxHash, e.Outpoint, e.ForkHeight)
}
