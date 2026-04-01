package consensus

import (
	"fmt"
	"testing"
)

func TestMaxTargetPoWConversion(t *testing.T) {
	bits := GetBitsFromTarget(MaxTargetPoW)
	fmt.Printf("MaxTargetPoW converts to bits: 0x%x\n", bits)

	// Convert back
	target := GetTargetFromBits(bits)
	fmt.Printf("MaxTargetPoW: %x\n", MaxTargetPoW.Bytes())
	fmt.Printf("Converted back: %x\n", target.Bytes())

	// Test specific values
	bits1e := uint32(0x1e0fffff)
	bits1f := uint32(0x1f00ffff)

	target1e := GetTargetFromBits(bits1e)
	target1f := GetTargetFromBits(bits1f)

	fmt.Printf("0x1e0fffff -> target: %x\n", target1e.Bytes())
	fmt.Printf("0x1f00ffff -> target: %x\n", target1f.Bytes())

	// Test genesis bits
	genesisTarget := GetTargetFromBits(0x1e0ffff0)
	fmt.Printf("Genesis 0x1e0ffff0 -> target: %x\n", genesisTarget.Bytes())
}
