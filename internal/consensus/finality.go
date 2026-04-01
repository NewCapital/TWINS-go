package consensus

import (
	"github.com/twins-dev/twins-core/pkg/types"
)

// Transaction finality constants
const (
	// LOCKTIME_THRESHOLD separates block height from unix timestamp in nLockTime
	// Values below this are block heights, values above are timestamps
	// Legacy: src/script/script.h:39
	LOCKTIME_THRESHOLD = 500000000

	// SEQUENCE_FINAL indicates no relative timelock (BIP68)
	// Legacy value: 0xffffffff
	SEQUENCE_FINAL = 0xffffffff

	// SEQUENCE_LOCKTIME_DISABLE_FLAG disables BIP68 relative timelocks
	SEQUENCE_LOCKTIME_DISABLE_FLAG = 1 << 31

	// SEQUENCE_LOCKTIME_TYPE_FLAG determines if relative lock is time or block-based
	// 0 = block count, 1 = 512-second intervals
	SEQUENCE_LOCKTIME_TYPE_FLAG = 1 << 22

	// SEQUENCE_LOCKTIME_MASK extracts the actual locktime value
	SEQUENCE_LOCKTIME_MASK = 0x0000ffff
)

// IsFinalTx checks if transaction is final and can be included in a block
// Implements logic from legacy IsFinalTx (main.cpp:862-881)
//
// A transaction is final if:
// 1. nLockTime is 0 (no lock), OR
// 2. nLockTime is in the past (height or timestamp), AND
// 3. All inputs have final nSequence (0xFFFFFFFF)
//
// Legacy reference: src/main.cpp:862-881
func IsFinalTx(tx *types.Transaction, blockHeight uint32, blockTime uint32) bool {
	// No locktime = always final
	if tx.LockTime == 0 {
		return true
	}

	// Check if locktime is satisfied
	var lockTimePassed bool
	if tx.LockTime < LOCKTIME_THRESHOLD {
		// Block height based locktime
		lockTimePassed = tx.LockTime < blockHeight
	} else {
		// Unix timestamp based locktime
		lockTimePassed = tx.LockTime < blockTime
	}

	if !lockTimePassed {
		// Locktime not yet satisfied
		return false
	}

	// Even if nLockTime is satisfied, check all inputs
	// If any input is non-final, entire transaction is non-final
	for _, input := range tx.Inputs {
		if !IsInputFinal(input.Sequence) {
			return false
		}
	}

	return true
}

// IsInputFinal checks if an input's sequence number is final
// An input is final if its sequence is SEQUENCE_FINAL (0xFFFFFFFF)
func IsInputFinal(sequence uint32) bool {
	return sequence == SEQUENCE_FINAL
}

// IsBIP68Enabled checks if BIP68 relative timelocks are active for this sequence
// BIP68 is disabled if:
// - Sequence is FINAL (0xFFFFFFFF), OR
// - Disable flag (bit 31) is set
func IsBIP68Enabled(sequence uint32) bool {
	// BIP68 disabled if sequence is FINAL
	if sequence == SEQUENCE_FINAL {
		return false
	}
	// BIP68 disabled if disable flag is set
	if sequence&SEQUENCE_LOCKTIME_DISABLE_FLAG != 0 {
		return false
	}
	return true
}

// GetSequenceLockTime extracts relative locktime value from sequence
// Returns (value, isTime) where:
// - value: number of blocks OR number of 512-second intervals
// - isTime: true if time-based, false if block-based
func GetSequenceLockTime(sequence uint32) (value uint32, isTime bool) {
	value = sequence & SEQUENCE_LOCKTIME_MASK
	isTime = (sequence & SEQUENCE_LOCKTIME_TYPE_FLAG) != 0
	return
}
