package utils

import (
	"github.com/shopspring/decimal"
)

// TWINSAmount represents a TWINS amount with proper decimal handling
type TWINSAmount struct {
	Value decimal.Decimal
}

// NewTWINSAmount creates a new TWINS amount from string
func NewTWINSAmount(amount string) (*TWINSAmount, error) {
	d, err := decimal.NewFromString(amount)
	if err != nil {
		return nil, err
	}
	return &TWINSAmount{Value: d}, nil
}

// ToSatoshi converts TWINS to satoshi (smallest unit)
func (a *TWINSAmount) ToSatoshi() int64 {
	satoshi := a.Value.Mul(decimal.NewFromInt(100000000))
	return satoshi.IntPart()
}

// FromSatoshi creates TWINSAmount from satoshi
func FromSatoshi(satoshi int64) *TWINSAmount {
	d := decimal.NewFromInt(satoshi).Div(decimal.NewFromInt(100000000))
	return &TWINSAmount{Value: d}
}

// String returns string representation
func (a *TWINSAmount) String() string {
	return a.Value.StringFixed(8)
}