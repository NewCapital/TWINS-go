/**
 * Amount validation utilities for TWINS wallet
 * Handles TWINS precision (8 decimal places) and validation rules
 */

import { addThousandsSeparators } from '@/shared/utils/format';

// Constants
export const TWINS_PRECISION = 8;
export const SATOSHIS_PER_TWINS = 100_000_000;
export const MAX_TWINS = 100_000_000_000; // 100 billion TWINS
export const MIN_TWINS = 0.00000001; // 1 satoshi
// Dust threshold matches backend: 3 * (182 bytes * minRelayTxFee / 1000)
// With default minRelayTxFee of 10000 satoshis/kB: 3 * 1820 = 5460 satoshis
export const DUST_THRESHOLD = 0.0000546; // 5460 satoshis - matches types.GetDustThreshold()

// Unit types
export enum AmountUnit {
  TWINS = 'TWINS',
  mTWINS = 'mTWINS',
  uTWINS = 'uTWINS'
}

// Unit conversion factors
export const UNIT_FACTORS = {
  [AmountUnit.TWINS]: 1,
  [AmountUnit.mTWINS]: 1000,
  [AmountUnit.uTWINS]: 1_000_000
};

/**
 * Transaction totals interface
 */
export interface TransactionTotals {
  recipientsTotal: number;
  estimatedFee: number;
  grandTotal: number;
  remainingBalance: number;
  canSend: boolean;
}

/**
 * Validation result interface
 */
export interface ValidationResult {
  isValid: boolean;
  error?: string;
}

/**
 * Format amount input as user types
 * - Removes non-numeric characters except decimal point
 * - Limits to 8 decimal places
 * - Prevents multiple decimal points
 */
export const formatAmountInput = (value: string): string => {
  if (!value) return '';

  // Remove non-numeric characters except decimal point
  let formatted = value.replace(/[^0-9.]/g, '');

  // Handle multiple decimal points - keep only the first one
  const parts = formatted.split('.');
  if (parts.length > 2) {
    formatted = parts[0] + '.' + parts[1];  // Keep only first decimal point
  }

  // Limit decimal places to TWINS precision
  if (parts.length === 2 && parts[1].length > TWINS_PRECISION) {
    formatted = parts[0] + '.' + parts[1].substring(0, TWINS_PRECISION);
  }

  // Remove leading zeros except for decimal numbers
  if (formatted.length > 1 && formatted[0] === '0' && formatted[1] !== '.') {
    formatted = formatted.substring(1);
  }

  return formatted;
};

/**
 * Parse amount string to number (in TWINS)
 *
 * Note: This uses floating-point arithmetic which is acceptable for display/validation
 * since we round to satoshi precision. When sending to backend, always use
 * formatForBackend() to get string representation to avoid precision loss.
 * The backend uses decimal.Decimal for precise arithmetic.
 */
export const parseAmount = (value: string): number => {
  if (!value || value === '') return 0;

  const parsed = parseFloat(value);
  if (isNaN(parsed)) return 0;

  // Round to TWINS precision to avoid floating point issues
  // This converts to satoshis (integer) then back to TWINS
  return Math.round(parsed * SATOSHIS_PER_TWINS) / SATOSHIS_PER_TWINS;
};

/**
 * Convert amount to satoshis (smallest unit)
 */
export const toSatoshis = (twins: number): number => {
  return Math.round(twins * SATOSHIS_PER_TWINS);
};

/**
 * Convert satoshis to TWINS
 */
export const fromSatoshis = (satoshis: number): number => {
  return satoshis / SATOSHIS_PER_TWINS;
};

/**
 * Format amount for form input (no thousands separators)
 *
 * Use this when setting values in form fields. Unlike formatAmountDisplay,
 * this does NOT add commas, so the value can be parsed correctly.
 *
 * @param amount - The amount to format
 */
export const formatAmountForInput = (amount: number): string => {
  if (amount === 0) return '0';

  // Format with proper decimal places
  const fixed = amount.toFixed(TWINS_PRECISION);

  // Remove trailing zeros from decimals
  const parts = fixed.split('.');
  if (parts[1]) {
    parts[1] = trimTrailingZeros(parts[1]);
    if (parts[1].length === 0) {
      return parts[0];
    }
  }

  return parts.join('.');
};

/** Remove trailing '0' characters from a string without regex backtracking. */
function trimTrailingZeros(s: string): string {
  let end = s.length;
  while (end > 0 && s[end - 1] === '0') end--;
  return s.slice(0, end);
}

/**
 * Format amount for display with thousands separators
 *
 * @param amount - The amount to format
 * @param showUnit - Whether to include "TWINS" suffix (default: true)
 * @param trimZeros - Whether to remove trailing zeros (default: true for UI display)
 */
export const formatAmountDisplay = (
  amount: number,
  showUnit: boolean = true,
  trimZeros: boolean = true
): string => {
  if (amount === 0) return showUnit ? '0 TWINS' : '0';

  // Format with proper decimal places
  const fixed = amount.toFixed(TWINS_PRECISION);

  // Add thousands separators
  const parts = fixed.split('.');
  parts[0] = addThousandsSeparators(parts[0]);

  // Optionally remove trailing zeros from decimals
  if (trimZeros && parts[1]) {
    parts[1] = trimTrailingZeros(parts[1]);
    if (parts[1].length === 0) {
      return showUnit ? `${parts[0]} TWINS` : parts[0];
    }
  }

  const formatted = parts.join('.');
  return showUnit ? `${formatted} TWINS` : formatted;
};

/**
 * Convert between units
 */
export const convertUnit = (amount: number, fromUnit: AmountUnit, toUnit: AmountUnit): number => {
  const inTwins = amount / UNIT_FACTORS[fromUnit];
  return inTwins * UNIT_FACTORS[toUnit];
};

/**
 * Validate amount value
 */
export const validateAmount = (
  amount: string | number,
  balance: number,
  includesFee: number = 0
): ValidationResult => {
  const value = typeof amount === 'string' ? parseAmount(amount) : amount;

  // Check if amount is provided
  if (value === 0) {
    return { isValid: false, error: 'Amount is required' };
  }

  // Check if amount is positive
  if (value < 0) {
    return { isValid: false, error: 'Amount must be greater than 0' };
  }

  // Check minimum amount (dust threshold)
  if (value < DUST_THRESHOLD) {
    return {
      isValid: false,
      error: `Amount below minimum transaction size (${DUST_THRESHOLD} TWINS)`
    };
  }

  // Check maximum amount
  if (value > MAX_TWINS) {
    return {
      isValid: false,
      error: `Amount exceeds maximum (${formatAmountDisplay(MAX_TWINS)})`
    };
  }

  // Check against available balance
  const total = value + includesFee;
  if (total > balance) {
    return {
      isValid: false,
      error: `Insufficient balance (available: ${formatAmountDisplay(balance)})`
    };
  }

  return { isValid: true };
};

/**
 * Calculate transaction totals
 */
export const calculateTotals = (
  recipients: Array<{ amount: string }>,
  estimatedFee: number,
  balance: number
): TransactionTotals => {
  // Calculate total for all recipients
  const recipientsTotal = recipients.reduce((sum, recipient) => {
    const amount = parseAmount(recipient.amount || '0');
    return sum + amount;
  }, 0);

  // Calculate grand total
  const grandTotal = recipientsTotal + estimatedFee;

  // Calculate remaining balance
  const remainingBalance = Math.max(0, balance - grandTotal);

  // Check if transaction can be sent
  const canSend = grandTotal <= balance && recipientsTotal > 0;

  return {
    recipientsTotal,
    estimatedFee,
    grandTotal,
    remainingBalance,
    canSend
  };
};

/**
 * Calculate maximum sendable amount (accounting for fees)
 * Returns 0 if fee is invalid or exceeds balance
 */
export const calculateMaxSendable = (balance: number, estimatedFee: number): number => {
  // Validate inputs
  if (!isFinite(balance) || balance < 0) return 0;
  if (!isFinite(estimatedFee) || estimatedFee < 0) return 0;

  // If fee exceeds balance, nothing can be sent
  if (estimatedFee >= balance) return 0;

  const maxAmount = balance - estimatedFee;

  // Ensure result is not below dust threshold
  if (maxAmount < DUST_THRESHOLD) return 0;

  return maxAmount;
};

/**
 * Validate multiple amounts for batch sending
 */
export const validateMultipleAmounts = (
  amounts: Array<string | number>,
  balance: number,
  estimatedFee: number
): ValidationResult => {
  let total = 0;

  for (const amount of amounts) {
    const value = typeof amount === 'string' ? parseAmount(amount) : amount;

    // Check individual amount
    if (value < DUST_THRESHOLD && value > 0) {
      return {
        isValid: false,
        error: `One or more amounts below minimum (${DUST_THRESHOLD} TWINS)`
      };
    }

    total += value;
  }

  // Check total against balance
  const grandTotal = total + estimatedFee;
  if (grandTotal > balance) {
    return {
      isValid: false,
      error: 'Total amount exceeds available balance'
    };
  }

  return { isValid: true };
};

/**
 * Format amount with proper precision for backend
 */
export const formatForBackend = (amount: number): string => {
  return amount.toFixed(TWINS_PRECISION);
};

/**
 * Check if string is a valid amount format
 */
export const isValidAmountFormat = (value: string): boolean => {
  if (!value) return false;

  // Check for valid number format with optional decimal
  const regex = /^\d+(\.\d{0,8})?$/;
  return regex.test(value);
};