package types

// CheckpointManager defines the interface for checkpoint management
// This interface is used to avoid circular dependencies between
// blockchain and consensus packages
type CheckpointManager interface {
	// IsCheckpointHeight returns true if the given height is a checkpoint
	IsCheckpointHeight(height uint32) bool

	// GetCheckpoint returns the expected hash at a checkpoint height
	GetCheckpoint(height uint32) (Hash, bool)
}
