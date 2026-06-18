/**
 * Transaction Icon Mapping Utilities
 * Maps transaction types and statuses to their corresponding icons
 * Based on Qt wallet implementation in transactiontablemodel.cpp
 */

import { convertToDisplayUnit, getUnitLabel } from '@/shared/utils/format';

// Import all transaction icons
import txMinedIcon from '@/assets/icons/transactions/tx_mined.png';
import txInputIcon from '@/assets/icons/transactions/tx_input.png';
import txOutputIcon from '@/assets/icons/transactions/tx_output.png';
import txInOutIcon from '@/assets/icons/transactions/tx_inout.png';

/**
 * Transaction status enum matching Qt implementation
 */
export enum TransactionStatus {
  Confirmed = 'confirmed',
  Unconfirmed = 'unconfirmed',
  Confirming = 'confirming',
  Conflicted = 'conflicted',
  Immature = 'immature',
  MaturesWarning = 'matures_warning',
  NotAccepted = 'not_accepted',
}

/**
 * Get the transaction type icon based on transaction type
 * Maps to Qt's txAddressDecoration() logic
 */
export function getTransactionTypeIcon(type: string): string {
  switch (type) {
    // Mining and staking rewards use pickaxe icon
    case 'generated':
    case 'stake':
    case 'masternode':
      return txMinedIcon;

    // Receive transactions use green arrow down
    case 'receive':
    case 'receive_from_other':
    case 'receive_with_obfuscation':
      return txInputIcon;

    // Send transactions use red arrow up
    case 'send':
    case 'send_to_other':
      return txOutputIcon;

    // UTXO consolidation uses bidirectional arrows
    case 'consolidation':

    // Self-transfers and other types use bidirectional arrows
    case 'send_to_self':
    case 'obfuscation_denominate':
    case 'obfuscation_collateral_payment':
    case 'obfuscation_make_collaterals':
    case 'obfuscation_create_denominations':
    case 'obfuscated':
    case 'other':
    default:
      return txInOutIcon;
  }
}

/**
 * Determine transaction status from confirmations
 */
export function getTransactionStatus(confirmations: number, isConflicted: boolean = false): TransactionStatus {
  if (isConflicted) {
    return TransactionStatus.Conflicted;
  }

  if (confirmations === 0) {
    return TransactionStatus.Unconfirmed;
  }

  if (confirmations >= 1 && confirmations < 6) {
    return TransactionStatus.Confirming;
  }

  return TransactionStatus.Confirmed;
}

// Module-level Intl.NumberFormat cache: avoid constructing a fresh formatter
// per row × per render. Keyed on displayDigits (typically 0-8); the cache
// stays small and lives for the application lifetime.
const formatterCache = new Map<number, Intl.NumberFormat>();

function getNumberFormatter(digits: number): Intl.NumberFormat {
  let formatter = formatterCache.get(digits);
  if (!formatter) {
    formatter = new Intl.NumberFormat('en-US', {
      minimumFractionDigits: digits,
      maximumFractionDigits: digits,
    });
    formatterCache.set(digits, formatter);
  }
  return formatter;
}

/**
 * Format amount for display with brackets if unconfirmed
 * Matches Qt's formatTxAmount() behavior.
 * Pass `includeUnit=false` when the unit label is rendered once at the parent
 * card header (e.g. Overview Recent Transactions card via DashboardCard's
 * headerRight slot, or Transactions list column header).
 */
export function formatTransactionAmount(
  amount: number,
  confirmations: number,
  displayUnit: number = 0,
  displayDigits: number = 8,
  includeUnit: boolean = true
): string {
  const formattedAmount = formatAmount(amount, displayUnit, displayDigits, includeUnit);

  // Wrap unconfirmed amounts in brackets
  if (confirmations === 0) {
    return `[${formattedAmount}]`;
  }

  return formattedAmount;
}

/**
 * Format amount with sign, locale-formatted thousands separators, and optional
 * unit label. Uses cached `Intl.NumberFormat('en-US')` so large values render
 * with comma separators (`+180,010.45` instead of `+180010.45`). Pass
 * `includeUnit=false` to omit the trailing ` <unit>` suffix when the unit is
 * rendered once at a parent card header.
 */
export function formatAmount(
  amount: number,
  displayUnit: number = 0,
  displayDigits: number = 8,
  includeUnit: boolean = true
): string {
  const converted = convertToDisplayUnit(amount, displayUnit);
  const sign = converted >= 0 ? '+' : '';
  const numericPart = `${sign}${getNumberFormatter(displayDigits).format(converted)}`;
  return includeUnit ? `${numericPart} ${getUnitLabel(displayUnit)}` : numericPart;
}

/**
 * Get hex color token for amount based on value (Receive design language).
 * Returns hex strings consumed by inline `style={{ color }}` at callsites.
 * Negative -> error fg, positive -> TWINS green, zero -> primary text.
 */
export function getAmountColorClass(amount: number): string {
  if (amount < 0) return '#ff6666';
  if (amount > 0) return '#27ae60';
  return '#ddd';
}

/**
 * Format transaction date/time with timezone abbreviation
 * Shows local time with timezone indicator for clarity
 * Example: "Nov 03, 2025 14:30 PST"
 */
export function formatTransactionDate(timestamp: number | string | Date): string {
  const date = new Date(timestamp);

  // Format: "Nov 03, 2025 14:30 PST"
  const options: Intl.DateTimeFormatOptions = {
    month: 'short',
    day: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
    timeZoneName: 'short',
  };

  return date.toLocaleString('en-US', options);
}

/**
 * Format transaction date/time in UTC for tooltips
 * Provides unambiguous time reference
 * Example: "Nov 03, 2025 22:30 UTC"
 */
export function formatTransactionDateUTC(timestamp: number | string | Date): string {
  const date = new Date(timestamp);

  // Format in UTC: "Nov 03, 2025 22:30 UTC"
  const options: Intl.DateTimeFormatOptions = {
    month: 'short',
    day: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
    timeZone: 'UTC',
    timeZoneName: 'short',
  };

  return date.toLocaleString('en-US', options);
}

/**
 * Get human-readable transaction type label
 */
export function getTransactionTypeLabel(type: string): string {
  // Labels match legacy C++ Qt wallet (transactionrecord.cpp)
  const labels: Record<string, string> = {
    'generated': 'Mined',
    'stake': 'TWINS Stake',
    'masternode': 'Masternode Reward',
    'send': 'Sent to',
    'send_to_other': 'Sent to',
    'send_to_self': 'Payment to yourself',
    'consolidation': 'UTXO Consolidation',
    'receive': 'Received with',
    'receive_from_other': 'Received with',
    'receive_with_obfuscation': 'Obfuscation',
    'obfuscation_denominate': 'Obfuscation Denominate',
    'obfuscation_collateral_payment': 'Obfuscation Collateral Payment',
    'obfuscation_make_collaterals': 'Obfuscation Make Collaterals',
    'obfuscation_create_denominations': 'Obfuscation Create Denominations',
    'obfuscated': 'Obfuscated',
    'other': 'Other',
  };

  return labels[type] || 'Unknown';
}