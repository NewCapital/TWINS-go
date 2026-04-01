// Re-export Wails-generated types from the core namespace
import { core } from '@wailsjs/go/models';

// Export the namespace for direct use
export { core };

// Create type aliases for convenience
export type Balance = core.Balance;
export type Transaction = core.Transaction;
export type BlockchainInfo = core.BlockchainInfo;

// Additional frontend-only types and interfaces

export interface Address {
  address: string;
  label: string;
  isDefault: boolean;
  isUsed: boolean;
  createdAt: Date;
}

export interface WalletInfo {
  version: string;
  protocolVersion: number;
  walletVersion: number;
  blocks: number;
  timeOffset: number;
  connections: number;
  proxy: string;
  difficulty: number;
  testnet: boolean;
  keyPoolOldest: number;
  keyPoolSize: number;
  payTxFee: number;
  relayFee: number;
  errors: string;
}

// Transaction status helper (derived from confirmations)
export type TransactionStatus = 'pending' | 'confirming' | 'confirmed' | 'immature' | 'conflicted';

// Helper function to get transaction status from confirmations
export function getTransactionStatus(confirmations: number, category: string): TransactionStatus {
  if (category === 'immature') return 'immature';
  if (confirmations === 0) return 'pending';
  if (confirmations < 6) return 'confirming';
  return 'confirmed';
}

// Helper function to format transaction for display
export function formatTransactionForDisplay(tx: core.Transaction) {
  const status = getTransactionStatus(tx.confirmations, tx.category);
  const date = new Date(tx.time);
  const displayDate = date.toLocaleString('en-US', {
    month: '2-digit',
    day: '2-digit',
    year: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false
  });

  // Format amount with sign and brackets for unconfirmed
  let displayAmount = (tx.amount >= 0 ? '+' : '') + tx.amount.toFixed(8) + ' TWINS';
  if (status === 'pending' || status === 'confirming') {
    displayAmount = `[${displayAmount}]`;
  }

  return {
    transaction: tx,
    status,
    displayDate,
    displayAmount
  };
}

// Coin Control Types

/**
 * OutPoint identifies a specific transaction output (UTXO)
 * Corresponds to COutPoint in Qt wallet
 */
export interface OutPoint {
  txid: string;  // Transaction hash
  vout: number;  // Output index
}

/**
 * UTXO (Unspent Transaction Output) represents a spendable coin
 * Corresponds to COutput in Qt wallet with additional metadata
 */
export interface UTXO {
  txid: string;
  vout: number;
  address: string;
  label?: string;
  amount: number;
  confirmations: number;
  spendable: boolean;
  solvable: boolean;
  locked: boolean;
  type: 'Personal' | 'MultiSig';
  date: number;  // Unix timestamp
  priority: number;  // Calculated: (amount * confirmations)
}

/**
 * CoinControl configuration for manual UTXO selection
 * Corresponds to CCoinControl in Qt wallet
 *
 * Note: selectedCoins and lockedCoins use Set<string> which is NOT JSON serializable.
 * These fields should NOT be persisted. The current store configuration excludes
 * coinControl from persistence (see useStore.ts partialize function).
 * If persistence scope changes, convert these to arrays or implement custom serialization.
 */
export interface CoinControl {
  // Selected UTXOs (txid:vout format)
  // WARNING: Set type - not serializable, excluded from persistence
  selectedCoins: Set<string>;

  // Locked UTXOs that cannot be spent (txid:vout format)
  // WARNING: Set type - not serializable, excluded from persistence
  lockedCoins: Set<string>;

  // UTXO splitting
  splitBlock: boolean;
  splitCount: number;

  // Allow automatic coin selection for additional inputs
  allowOtherInputs: boolean;

  // Include watch-only addresses
  allowWatchOnly: boolean;

  // Minimum fee
  minimumTotalFee: number;
}

/**
 * View mode for Coin Control dialog
 */
export type CoinControlViewMode = 'tree' | 'list';

/**
 * Filter mode for displaying UTXOs
 */
export type CoinControlFilterMode = 'all' | 'spendable' | 'locked';

/**
 * Sort mode for UTXO list
 */
export type CoinControlSortMode = 'amount' | 'confirmations' | 'address' | 'priority' | 'date';

/**
 * Summary statistics for selected coins
 */
export interface CoinControlSummary {
  quantity: number;       // Number of selected UTXOs
  amount: number;         // Total amount selected
  fee: number;            // Estimated transaction fee
  afterFee: number;       // Amount after fee
  bytes: number;          // Estimated transaction size
  priority: string;       // Priority label (low/medium/high)
  change: number;         // Change amount
  dust: boolean;          // Whether transaction creates dust
}

/**
 * Tree node for hierarchical UTXO display
 * Groups UTXOs by address in tree view mode
 */
export interface UTXOTreeNode {
  type: 'address' | 'utxo';
  address?: string;
  label?: string;
  utxos?: UTXO[];
  utxo?: UTXO;
  totalAmount?: number;
  selected: boolean;
  expanded?: boolean;
}
