package types

import (
	"fmt"
	"math/big"
)

// BigInt wraps big.Int for blockchain work calculations
type BigInt struct {
	value *big.Int
}

// NewBigInt creates a new BigInt from an int64
func NewBigInt(x int64) *BigInt {
	return &BigInt{
		value: big.NewInt(x),
	}
}

// NewBigIntFromBytes creates a BigInt from bytes
func NewBigIntFromBytes(b []byte) *BigInt {
	return &BigInt{
		value: new(big.Int).SetBytes(b),
	}
}

// NewBigIntFromString creates a BigInt from a string
func NewBigIntFromString(s string, base int) (*BigInt, error) {
	value, ok := new(big.Int).SetString(s, base)
	if !ok {
		return nil, fmt.Errorf("invalid BigInt string")
	}
	return &BigInt{value: value}, nil
}

// Int64 returns the int64 representation
func (b *BigInt) Int64() int64 {
	if b == nil || b.value == nil {
		return 0
	}
	return b.value.Int64()
}

// Uint64 returns the uint64 representation
func (b *BigInt) Uint64() uint64 {
	if b == nil || b.value == nil {
		return 0
	}
	return b.value.Uint64()
}

// Bytes returns the absolute value as a byte slice
func (b *BigInt) Bytes() []byte {
	if b == nil || b.value == nil {
		return []byte{0}
	}
	return b.value.Bytes()
}

// String returns the decimal string representation
func (b *BigInt) String() string {
	if b == nil || b.value == nil {
		return "0"
	}
	return b.value.String()
}

// Add adds two BigInts
func (b *BigInt) Add(other *BigInt) *BigInt {
	if b == nil || other == nil {
		return NewBigInt(0)
	}
	result := new(big.Int).Add(b.value, other.value)
	return &BigInt{value: result}
}

// Sub subtracts two BigInts
func (b *BigInt) Sub(other *BigInt) *BigInt {
	if b == nil || other == nil {
		return NewBigInt(0)
	}
	result := new(big.Int).Sub(b.value, other.value)
	return &BigInt{value: result}
}

// Mul multiplies two BigInts
func (b *BigInt) Mul(other *BigInt) *BigInt {
	if b == nil || other == nil {
		return NewBigInt(0)
	}
	result := new(big.Int).Mul(b.value, other.value)
	return &BigInt{value: result}
}

// Div divides two BigInts
func (b *BigInt) Div(other *BigInt) *BigInt {
	if b == nil || other == nil || other.value.Sign() == 0 {
		return NewBigInt(0)
	}
	result := new(big.Int).Div(b.value, other.value)
	return &BigInt{value: result}
}

// Cmp compares two BigInts
// Returns -1 if b < other, 0 if b == other, 1 if b > other
func (b *BigInt) Cmp(other *BigInt) int {
	if b == nil || b.value == nil {
		if other == nil || other.value == nil {
			return 0
		}
		return -1
	}
	if other == nil || other.value == nil {
		return 1
	}
	return b.value.Cmp(other.value)
}

// IsZero returns true if the BigInt is zero
func (b *BigInt) IsZero() bool {
	return b == nil || b.value == nil || b.value.Sign() == 0
}

// Sign returns -1, 0, or 1 depending on whether b is negative, zero, or positive
func (b *BigInt) Sign() int {
	if b == nil || b.value == nil {
		return 0
	}
	return b.value.Sign()
}