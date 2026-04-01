package types

import (
	"math/big"
)

// StakeKernel represents the data used to compute stake kernel hash
type StakeKernel struct {
	StakeModifier uint64   // Stake modifier from previous blocks
	Timestamp     uint32   // Block timestamp
	PrevOut       Outpoint // Previous output being staked
	StakeValue    uint64   // Value of the stake
	PrevBlockTime uint32   // Previous block timestamp
}

// StakeModifier is stored in block headers for PoS
type StakeModifier uint64

// CompactToBig converts a compact representation to a big integer (for difficulty targets)
func CompactToBig(compact uint32) *big.Int {
	// The compact format is a representation of a 256-bit number
	// Format: [1 byte exponent][3 bytes mantissa], e.g. 0x1e0fffff = mantissa 0x0fffff * 2^(8*(0x1e-3))
	mantissa := compact & 0x007fffff
	exponent := compact >> 24

	var result *big.Int
	if exponent <= 3 {
		result = big.NewInt(int64(mantissa >> uint(8*(3-exponent))))
	} else {
		result = big.NewInt(int64(mantissa))
		result.Lsh(result, uint(8*(exponent-3)))
	}

	// Handle negative flag
	if mantissa&0x00800000 != 0 {
		result.Neg(result)
	}

	return result
}

// BigToCompact converts a big integer to compact representation
func BigToCompact(n *big.Int) uint32 {
	if n.Sign() == 0 {
		return 0
	}

	// Get the absolute value
	val := new(big.Int).Abs(n)

	// Find the exponent
	bytes := val.Bytes()
	exponent := uint32(len(bytes))

	// Get mantissa (first 3 bytes)
	var mantissa uint32
	if exponent <= 3 {
		mantissa = uint32(val.Uint64())
		mantissa <<= uint(8 * (3 - exponent))
		exponent = 0
	} else {
		// Take the first 3 bytes
		mantissa = uint32(bytes[0]) << 16
		if len(bytes) > 1 {
			mantissa |= uint32(bytes[1]) << 8
		}
		if len(bytes) > 2 {
			mantissa |= uint32(bytes[2])
		}
	}

	// Set negative flag if needed
	if n.Sign() < 0 && mantissa&0x00800000 == 0 {
		mantissa |= 0x00800000
	}

	// Combine exponent and mantissa
	return (exponent << 24) | (mantissa & 0x007fffff)
}

// Constants for PoS validation
const (
	// CurrentBlockVersion is the current block version
	CurrentBlockVersion = 7

	// CurrentTxVersion is the current transaction version
	CurrentTxVersion = 1
)