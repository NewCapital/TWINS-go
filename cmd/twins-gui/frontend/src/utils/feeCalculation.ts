/**
 * Fee calculation utilities for TWINS wallet transactions
 * Uses satoshi-based arithmetic for precision
 */

// Network constants
export const MIN_RELAY_FEE = 0.00001; // TWINS per KB - minimum network relay fee
export const HIGH_FEE_THRESHOLD = 0.01; // TWINS per KB - warn if fee rate exceeds this
export const DEFAULT_FEE_RATE = 0.0005; // TWINS per KB - default normal fee rate
export const SATOSHIS_PER_TWINS = 100000000; // 1 TWINS = 100,000,000 satoshis

// Transaction size estimation constants (in bytes)
// Must match wallet/transactions.go formula: 190*inputs + 34*(outputs+1) + 10
// where outputs = recipients (the +1 accounts for the change output)
const TX_BASE_SIZE = 10; // Base transaction overhead
const TX_INPUT_SIZE = 190; // Size per input including signature (matches wallet/transactions.go)
const TX_OUTPUT_SIZE = 34; // Size per output (includes change output in estimateTxSize)

// Minimum transaction fee in TWINS (10000 satoshis = wallet default MinTxFee)
// Must match internal/wallet/transactions.go MinTxFee default
export const MIN_TX_FEE = 0.0001;

/**
 * Estimate transaction size in bytes
 * @param recipientCount Number of recipients (outputs)
 * @param inputCount Number of inputs (default: 2 for typical transactions)
 * @returns Estimated transaction size in bytes
 */
export const estimateTxSize = (
  recipientCount: number,
  inputCount: number = 2
): number => {
  // Base size + (inputs × input size) + (outputs × output size)
  // Note: outputs = recipients + 1 change address
  const outputs = recipientCount + 1;
  return TX_BASE_SIZE + (inputCount * TX_INPUT_SIZE) + (outputs * TX_OUTPUT_SIZE);
};

/**
 * Calculate fee amount based on transaction size and fee rate
 * Uses satoshi-based arithmetic for precision
 * @param txSize Transaction size in bytes
 * @param feeRate Fee rate in TWINS per KB
 * @returns Fee amount in TWINS
 */
export const calculateFee = (
  txSize: number,
  feeRate: number
): number => {
  // Convert fee rate to satoshis per KB for precise calculation
  const satoshisPerKB = Math.round(feeRate * SATOSHIS_PER_TWINS);

  // Calculate fee in satoshis: (size in bytes * satoshis per KB) / 1000
  const feeSatoshis = Math.round((txSize * satoshisPerKB) / 1000);

  // Convert back to TWINS
  return feeSatoshis / SATOSHIS_PER_TWINS;
};

/**
 * Calculate total fee for a transaction
 * @param recipientCount Number of recipients
 * @param feeRate Fee rate in TWINS per KB
 * @param inputCount Number of inputs (default: 2)
 * @returns Total fee in TWINS
 */
export const calculateTotalFee = (
  recipientCount: number,
  feeRate: number,
  inputCount: number = 2
): number => {
  const txSize = estimateTxSize(recipientCount, inputCount);
  return calculateFee(txSize, feeRate);
};

/**
 * Validate fee rate and return warning level
 * @param feeRate Fee rate in TWINS per KB
 * @returns Warning level: 'high', 'low', or null
 */
export const validateFeeRate = (
  feeRate: number
): 'high' | 'low' | null => {
  if (feeRate < MIN_RELAY_FEE) {
    return 'low';
  }
  if (feeRate > HIGH_FEE_THRESHOLD) {
    return 'high';
  }
  return null;
};

/**
 * Check if fee is a significant percentage of the transaction amount
 * @param feeAmount Fee amount in TWINS
 * @param transactionAmount Transaction amount in TWINS
 * @param threshold Percentage threshold (default: 1%)
 * @returns True if fee exceeds threshold percentage
 */
export const isFeeHighPercentage = (
  feeAmount: number,
  transactionAmount: number,
  threshold: number = 0.01
): boolean => {
  if (transactionAmount <= 0) return false;
  return (feeAmount / transactionAmount) > threshold;
};

/**
 * Format fee rate for display
 * @param rate Fee rate in TWINS per KB
 * @returns Formatted string with 8 decimal places
 */
export const formatFeeRate = (rate: number): string => {
  return rate.toFixed(8);
};

/**
 * Format fee amount for display
 * @param amount Fee amount in TWINS
 * @returns Formatted string with 8 decimal places
 */
export const formatFeeAmount = (amount: number): string => {
  return amount.toFixed(8);
};

/**
 * Calculate adjusted amount when subtracting fee
 * @param originalAmount Original amount to send
 * @param feeAmount Fee amount
 * @param dustThreshold Minimum amount (dust threshold)
 * @returns Adjusted amount after subtracting fee, or null if below dust threshold
 */
export const calculateAmountMinusFee = (
  originalAmount: number,
  feeAmount: number,
  dustThreshold: number = 0.00000546
): number | null => {
  const adjustedAmount = originalAmount - feeAmount;
  if (adjustedAmount < dustThreshold) {
    return null; // Below dust threshold
  }
  return adjustedAmount;
};

