package types

// Fee-related constants matching legacy implementation
const (
	// MinRelayTxFeePerKB is the minimum fee for relaying transactions
	// Legacy: minRelayTxFee = CFeeRate(100000) - 100,000 satoshis per KB
	MinRelayTxFeePerKB int64 = 100000

	// DustRelayFeeMultiplier determines dust threshold
	// If you'd pay more than 1/3 in fees to spend something, it's dust
	DustRelayFeeMultiplier = 3

	// MaxTxFeeMultiplier for detecting insane fees
	// Fees above MinRelayTxFee * 10000 are considered insane
	MaxTxFeeMultiplier = 10000

	// StandardTxOutSize is the typical size of a transaction output
	// Used for dust calculations (34 bytes)
	StandardTxOutSize = 34

	// StandardTxInSize is the typical size needed to spend an output
	// Used for dust calculations (148 bytes)
	StandardTxInSize = 148
)

// FeeRate represents a fee rate in satoshis per kilobyte
type FeeRate struct {
	satoshisPerKB int64
}

// NewFeeRate creates a new fee rate from satoshis per KB
func NewFeeRate(satoshisPerKB int64) FeeRate {
	return FeeRate{satoshisPerKB: satoshisPerKB}
}

// NewFeeRateFromAmount creates a fee rate from total fee and transaction size
func NewFeeRateFromAmount(feePaid int64, txSize int) FeeRate {
	if txSize == 0 {
		return FeeRate{0}
	}
	// Calculate fee per KB
	satoshisPerKB := (feePaid * 1000) / int64(txSize)
	return FeeRate{satoshisPerKB: satoshisPerKB}
}

// GetFee calculates the fee for a given transaction size in bytes
// Matches legacy CFeeRate::GetFee implementation
func (f FeeRate) GetFee(sizeInBytes int) int64 {
	fee := f.satoshisPerKB * int64(sizeInBytes) / 1000

	// If fee is 0 but rate is positive, use minimum (matches legacy)
	if fee == 0 && f.satoshisPerKB > 0 {
		fee = f.satoshisPerKB
	}

	return fee
}

// GetFeePerKB returns the fee rate in satoshis per kilobyte
func (f FeeRate) GetFeePerKB() int64 {
	return f.satoshisPerKB
}

// IsDust checks if a transaction output amount is considered dust
// Matches legacy CTxOut::IsDust implementation
func IsDust(outputValue int64, minRelayTxFee FeeRate) bool {
	// "Dust" is defined in terms of minRelayTxFee
	// If you'd pay more than 1/3 in fees to spend something, then we consider it dust
	// A typical txout is 34 bytes, and will need a CTxIn of at least 148 bytes to spend
	// Total is 148 + 34 = 182 bytes
	size := StandardTxOutSize + StandardTxInSize // 182 bytes

	// Calculate the fee needed to spend this output
	feeToSpend := minRelayTxFee.GetFee(size)

	// Dust threshold is 3 times the fee to spend
	dustThreshold := DustRelayFeeMultiplier * feeToSpend

	return outputValue < dustThreshold
}

// GetDustThreshold returns the minimum output value that isn't considered dust
func GetDustThreshold(minRelayTxFee FeeRate) int64 {
	size := StandardTxOutSize + StandardTxInSize // 182 bytes
	feeToSpend := minRelayTxFee.GetFee(size)
	return DustRelayFeeMultiplier * feeToSpend
}

// IsFeeTooHigh checks if a fee is insanely high (potential mistake)
// Matches legacy's check for insane fees
func IsFeeTooHigh(fee int64, txSize int, minRelayTxFee FeeRate) bool {
	maxReasonableFee := minRelayTxFee.GetFee(txSize) * MaxTxFeeMultiplier
	return fee > maxReasonableFee
}

// CalculateMinFee calculates the minimum required fee for a transaction
// based on its size and the minimum relay fee rate
func CalculateMinFee(txSize int, minRelayTxFee FeeRate) int64 {
	return minRelayTxFee.GetFee(txSize)
}

// Global default fee rates (can be overridden by configuration)
var (
	// DefaultMinRelayTxFee is the default minimum relay fee rate
	DefaultMinRelayTxFee = NewFeeRate(MinRelayTxFeePerKB)
)