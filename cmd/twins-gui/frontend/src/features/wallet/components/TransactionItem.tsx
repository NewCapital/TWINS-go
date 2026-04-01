/**
 * TransactionItem Component
 * Replicates the exact appearance of Qt wallet's TxViewDelegate
 * Based on overviewpage.cpp lines 43-94
 */

import React from 'react';
import { useTranslation } from 'react-i18next';
import { core } from '@/shared/types/wallet.types';
import {
  getTransactionTypeIcon,
  formatTransactionAmount,
  formatTransactionDate,
  formatTransactionDateUTC,
  getAmountColorClass,
} from '@/shared/utils/transactionIcons';
import { ConfirmationRing } from '@/shared/components/ConfirmationRing';
import { useDisplayUnits } from '@/shared/hooks/useDisplayUnits';

interface TransactionItemProps {
  transaction: core.Transaction;
  onClick?: (transaction: core.Transaction) => void;
}

// Qt constants from overviewpage.cpp
const DECORATION_SIZE = 48; // Icon size
const ICON_OFFSET = 16; // Left margin before icon

export const TransactionItem: React.FC<TransactionItemProps> = ({ transaction, onClick }) => {
  const typeIcon = getTransactionTypeIcon(transaction.type);
  const { displayUnit, displayDigits } = useDisplayUnits();

  // Format the display values
  const formattedAmount = formatTransactionAmount(
    transaction.amount,
    transaction.confirmations || 0,
    displayUnit,
    displayDigits
  );
  const formattedDate = formatTransactionDate(transaction.time);
  const formattedDateUTC = formatTransactionDateUTC(transaction.time);
  const amountColorClass = getAmountColorClass(transaction.amount);

  // Use address or label for display
  const displayAddress = transaction.label || transaction.address || 'Unknown';

  return (
    <div
      className="transaction-item relative flex items-center cursor-pointer hover:bg-gray-800/30 transition-colors"
      style={{
        minHeight: `${DECORATION_SIZE + 12}px`, // DECORATION_SIZE + vertical padding
        paddingLeft: `${ICON_OFFSET}px`,
        paddingTop: '6px',
        paddingBottom: '6px',
      }}
      onClick={() => onClick?.(transaction)}
    >
      {/* Icon Section - Transaction Type with Confirmation Ring */}
      <div className="flex-shrink-0">
        <ConfirmationRing
          typeIcon={typeIcon}
          confirmations={transaction.confirmations || 0}
          isConflicted={transaction.is_conflicted || false}
          isCoinstake={transaction.is_coinstake || false}
          maturesIn={transaction.matures_in || 0}
          size={DECORATION_SIZE}
        />
      </div>

      {/* Text Section */}
      <div
        className="flex-1 flex flex-col justify-center"
        style={{
          marginLeft: '8px', // xspace gap after icon
          minHeight: `${DECORATION_SIZE}px`,
        }}
      >
        {/* Top Line: Date (left) and Amount (right) */}
        <div className="flex justify-between items-center">
          <span
            className="text-white text-sm"
            style={{ cursor: 'help' }}
            title={`UTC: ${formattedDateUTC}`}
          >
            {formattedDate}
          </span>
          <span className={`text-sm font-mono ${amountColorClass}`}>
            {formattedAmount}
          </span>
        </div>

        {/* Bottom Line: Address/Label */}
        <div className="flex items-center">
          <span className="text-gray-400 text-sm truncate">
            {displayAddress}
          </span>

          {/* Watch-only indicator (if applicable) */}
          {transaction.is_watch_only && (
            <span className="ml-2 text-xs text-yellow-500">[watch-only]</span>
          )}
        </div>
      </div>
    </div>
  );
};

/**
 * TransactionList Component
 * Container for transaction items with Qt-style appearance
 */
interface TransactionListProps {
  transactions: core.Transaction[];
  onTransactionClick?: (transaction: core.Transaction) => void;
  limit?: number;
}

export const TransactionList: React.FC<TransactionListProps> = ({
  transactions,
  onTransactionClick,
  limit = 9, // NUM_ITEMS from Qt
}) => {
  const { t } = useTranslation('wallet');
  // Limit transactions to specified number (9 for overview page)
  const displayTransactions = transactions.slice(0, limit);

  return (
    <div className="transaction-list">
      {displayTransactions.length === 0 ? (
        <div className="text-center text-gray-500 py-8">
          {t('transactions.noTransactions')}
        </div>
      ) : (
        displayTransactions.map((tx) => (
          <TransactionItem
            key={`${tx.txid}:${tx.vout}`}
            transaction={tx}
            onClick={onTransactionClick}
          />
        ))
      )}
    </div>
  );
};