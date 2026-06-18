/**
 * TransactionItem Component
 * Single-line row card matching the Receive page Recent Requests design language.
 * 6-column layout: ConfirmationRing | Date | Label/Address | watch-only pill | Amount | Eye button.
 */

import React from 'react';
import { useTranslation } from 'react-i18next';
import { Eye } from 'lucide-react';
import { core } from '@/shared/types/wallet.types';
import {
  getTransactionTypeIcon,
  formatTransactionAmount,
  getAmountColorClass,
  getTransactionTypeLabel,
} from '@/shared/utils/transactionIcons';
import { ConfirmationRing } from '@/shared/components/ConfirmationRing';
import { IconButton } from '@/shared/components/IconButton';
import { useDisplayUnits } from '@/shared/hooks/useDisplayUnits';
import { useDisplayDateTime } from '@/shared/hooks/useDisplayDateTime';

interface TransactionItemProps {
  transaction: core.Transaction;
  onClick?: (transaction: core.Transaction) => void;
}

export const TransactionItem: React.FC<TransactionItemProps> = ({ transaction, onClick }) => {
  const { t } = useTranslation('wallet');
  const typeIcon = getTransactionTypeIcon(transaction.type);
  const { displayUnit, displayDigits } = useDisplayUnits();
  const { formatDateTime, formatTooltip } = useDisplayDateTime();

  const formattedAmount = formatTransactionAmount(
    transaction.amount,
    transaction.confirmations || 0,
    displayUnit,
    displayDigits,
    false // unit rendered once at the card header via DashboardCard's headerRight slot
  );
  const longLocalDate = formatDateTime(transaction.time);
  const longUtcDate = formatTooltip(transaction.time);
  const amountColorClass = getAmountColorClass(transaction.amount);
  const typeLabel = getTransactionTypeLabel(transaction.type);

  const displayAddress = transaction.label || transaction.address || 'Unknown';

  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: '12px',
        padding: '8px 12px',
        backgroundColor: '#2a2a2a',
        borderRadius: '6px',
        border: '1px solid transparent',
        cursor: 'default',
        transition: 'border-color 0.15s',
      }}
      onMouseEnter={(e) => { (e.currentTarget as HTMLDivElement).style.borderColor = '#444'; }}
      onMouseLeave={(e) => { (e.currentTarget as HTMLDivElement).style.borderColor = 'transparent'; }}
    >
      {/* Combined icon + confirmation ring column (28×28, canonical pattern matching
          TransactionDetailsDialog and Transactions page). One token carries both the
          transaction type and its confirmation status. */}
      <div style={{ flexShrink: 0, display: 'flex', alignItems: 'center' }}>
        <ConfirmationRing
          typeIcon={typeIcon}
          confirmations={transaction.confirmations || 0}
          isConflicted={transaction.is_conflicted || false}
          isCoinstake={transaction.is_coinstake || false}
          maturesIn={transaction.matures_in || 0}
          size={32}
          showIcon={true}
        />
      </div>

      {/* Date column — long "MMM DD, YYYY at HH:mm Z" form; UTC variant in tooltip.
          minWidth 220px matches Transactions page COL.date so the long-date format
          ("May 20, 2026 at 07:52 GMT+2") never overflows. */}
      <span
        style={{ fontSize: '14px', color: '#ddd', flexShrink: 0, minWidth: '220px', whiteSpace: 'nowrap' }}
        title={longUtcDate}
      >
        {longLocalDate}
      </span>

      {/* Type column — matches Transactions page COL.type (180px, 14px #ddd).
          180px fits "Masternode Reward" / "Payment to yourself" at 14px without ellipsis. */}
      <span
        style={{ fontSize: '14px', color: '#ddd', flexShrink: 0, width: '180px', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}
        title={typeLabel}
      >
        {typeLabel}
      </span>

      {/* Label / address column (flex, ellipsis) */}
      <span
        style={{
          flex: 1,
          minWidth: 0,
          fontSize: '14px',
          color: '#ddd',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
        }}
      >
        {displayAddress}
      </span>

      {/* Watch-only pill (conditional) */}
      {transaction.is_watch_only && (
        <span
          style={{
            flexShrink: 0,
            color: '#ff9966',
            backgroundColor: 'rgba(255,153,102,0.1)',
            borderRadius: '999px',
            padding: '1px 6px',
            fontSize: '10px',
          }}
        >
          {t('transactions.watchOnly', { defaultValue: '[watch-only]' })}
        </span>
      )}

      {/* Amount column */}
      <span
        style={{
          flexShrink: 0,
          minWidth: '100px',
          textAlign: 'right',
          fontSize: '14px',
          fontWeight: 600,
          fontFamily: 'monospace',
          color: amountColorClass,
        }}
      >
        {formattedAmount}
      </span>

      {/* Eye button — explicit View affordance (shared IconButton, matches Transactions page) */}
      <div style={{ flexShrink: 0 }}>
        <IconButton
          size={26}
          icon={<Eye size={14} />}
          onClick={() => onClick?.(transaction)}
          title={t('transactions.viewDetails', { defaultValue: 'View details' })}
          ariaLabel={t('transactions.viewDetails', { defaultValue: 'View details' })}
        />
      </div>
    </div>
  );
};

/**
 * TransactionList Component
 * Vertical stack of TransactionItem rows with `gap: 4px` (matches Receive's row stack).
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
  const displayTransactions = transactions.slice(0, limit);

  // Outer container uses flex:1/minHeight:0 so the list can grow with a
  // flex-column parent (e.g. the Overview Recent Transactions card stretches
  // to fill the viewport below the 2x2 status grid — see OverviewPage.tsx).
  // Non-flex parents are unaffected because flex:1 only resolves to a real
  // size when the parent is itself a flex container.
  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        gap: '4px',
        flex: 1,
        minHeight: 0,
      }}
    >
      {displayTransactions.length === 0 ? (
        // Empty state: grow to fill the remaining card height and center the
        // placeholder text on both axes. When the parent card sizes to
        // content (non-stretched callsites) this still renders centered
        // horizontally with vertical padding via the alignItems/justifyContent
        // pair; flex:1 just collapses to the natural single-line height.
        <div
          style={{
            flex: 1,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            color: '#666',
            fontSize: '11px',
            fontStyle: 'italic',
            padding: '24px 0',
            textAlign: 'center',
          }}
        >
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
