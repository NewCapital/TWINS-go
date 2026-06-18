import React, { useEffect, useLayoutEffect, useState, useCallback, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useStore, useTransactions, useNotifications } from '@/store/useStore';
import { core } from '@/shared/types/wallet.types';
import {
  getTransactionTypeIcon,
  formatTransactionAmount,
  getAmountColorClass,
  getTransactionTypeLabel,
} from '@/shared/utils/transactionIcons';
import { ConfirmationRing } from '@/shared/components/ConfirmationRing';
import { useDisplayUnits } from '@/shared/hooks/useDisplayUnits';
import { useDisplayDateTime } from '@/shared/hooks/useDisplayDateTime';
import { truncateAddress } from '@/shared/utils/format';
import { ChevronUp, ChevronDown, Eye, Copy, Edit, ExternalLink, Download, Inbox } from 'lucide-react';
import { IconButton } from '@/shared/components/IconButton';
import { PaginationFooter } from '@/shared/components/PaginationFooter';
import { LEGACY_EXPLORER_TX_FALLBACK, buildExplorerURL } from '@/shared/constants/explorer';
import { TxFilterBar } from '@/features/wallet/components/TxFilterBar';
import type { SortColumn } from '@/store/slices/transactionsSlice';
import { PAGE_SIZES } from '@/store/slices/transactionsSlice';
import { TransactionDetailsDialog } from '../components/TransactionDetailsDialog';
import { EditLabelDialog } from '../components/EditLabelDialog';
import { Banner } from '@/shared/components/Banner';
import { SimpleConfirmDialog } from '@/shared/components/SimpleConfirmDialog';
import { BrowserOpenURL, EventsOn, EventsOff } from '@wailsjs/runtime/runtime';

const DECORATION_SIZE = 32; // ConfirmationRing size for list rows

// Row + header heights used to derive a stable `min-height` on the scroll
// container, so reducing row count mid-session (filter narrows results, page
// size shrinks, etc.) doesn't collapse the container and jump the viewport.
// ROW_HEIGHT_PX = 10*2 top/bottom padding + 14px content + 4px container gap.
// HEADER_HEIGHT_PX = 10*2 top/bottom padding + ~12px label + 1px borderBottom.
const ROW_HEIGHT_PX = 38;
const HEADER_HEIGHT_PX = 33;
const SCROLL_LIST_VPAD_PX = 20; // 8px top + 12px bottom on the row-list wrapper

// Column widths shared between the sticky header row and each TransactionRow / SkeletonRow.
// Address column uses flex: 1 and is not in this table.
const COL = {
  statusIcon: '40px',
  date: '220px', // fits long-date format "May 19, 2026 at 17:13 GMT+2" without overflow
  type: '180px', // fits "Masternode Reward" / "Payment to yourself" at 14px without ellipsis; "Obfuscation Create Denominations" still clips with tooltip fallback
  // amount: 180px fits `-99999999.99 TWINS` (16 chars) and `-9999999999.99 TWINS` (18 chars)
  // at 14px monospace (~8.4px/char) without overflowing into the trailing Eye column.
  // Wider than needed for typical balances; trade-off accepted to prevent text-collision
  // with the Eye icon on long values. mTWINS / µTWINS at extreme magnitudes can still
  // exceed this — graceful overflow (no visual collision because of `whiteSpace: nowrap`
  // pushing past gap, but flexShrink keeps Eye column intact).
  amount: '180px',
  eye: '32px', // trailing Eye IconButton column (26x26 button + 6px slack)
} as const;


// Base style for context-menu items. Spread into each <button> with per-item
// color/cursor overrides for disabled state. Matches the TransactionDetailsDialog
// ExplorerButton popover token (Receive design language).
const MENU_ITEM_BASE_STYLE: React.CSSProperties = {
  width: '100%',
  padding: '6px 12px',
  backgroundColor: 'transparent',
  border: 'none',
  borderRadius: '4px',
  fontSize: '12px',
  textAlign: 'left',
  display: 'flex',
  alignItems: 'center',
  gap: '8px',
};

/**
 * Context menu state interface
 */
interface ContextMenuState {
  visible: boolean;
  x: number;
  y: number;
  transaction: core.Transaction | null;
}


/**
 * Transaction row component for the table
 */
interface TransactionRowProps {
  transaction: core.Transaction;
  // Accepts the transaction as an argument so the parent passes the stable
  // `handleOpenTransactionDetails` reference directly (no inline arrow that
  // would defeat React.memo's shallow compare). The row supplies the closure
  // at the click site below. See task h-fix-tx-row-re-render-on-modal-open.
  onOpenDetails: (transaction: core.Transaction) => void;
  onContextMenu: (e: React.MouseEvent, transaction: core.Transaction) => void;
  viewDetailsLabel: string;
  rowKey: string;
}

const TransactionRow = React.memo<TransactionRowProps>(function TransactionRow({
  transaction,
  onOpenDetails,
  onContextMenu,
  viewDetailsLabel,
  rowKey,
}) {
  const { t } = useTranslation('wallet');
  const typeIcon = getTransactionTypeIcon(transaction.type);
  const { displayUnit, displayDigits } = useDisplayUnits();
  const formattedAmount = formatTransactionAmount(transaction.amount, transaction.confirmations || 0, displayUnit, displayDigits, false);
  const { formatDateTimeShort, formatTooltip } = useDisplayDateTime();
  const formattedDate = formatDateTimeShort(transaction.time);
  const formattedDateUTC = formatTooltip(transaction.time);
  const amountColorClass = getAmountColorClass(transaction.amount);
  const typeLabel = getTransactionTypeLabel(transaction.type);

  // U2 (2026-05-22): unified address-cell rendering with directional prefix.
  // Maps the actual transaction.type strings (see shared/utils/transactionIcons.ts)
  // onto three behavior buckets. Visible strings come from i18n keys under
  // transactions.address.* (toPrefix / fromPrefix / moreSuffix / unknown) so
  // non-English locales render correctly.
  //   - incoming      → "<fromPrefix>: <truncated from_address>" when
  //                     from_address is populated; falls back to the local
  //                     label/address otherwise (sender unknown)
  //   - outgoing      → "<toPrefix>: <truncated recipient or label>" with a
  //                     <moreSuffix> when 2+ recipients
  //   - self-transfer → directional prefix omitted (consolidation /
  //                     send_to_self read fine without "To:" / "From:")
  const type = transaction.type;
  const isSelfTransfer = type === 'send_to_self' || type === 'consolidation';
  const isOutgoing =
    type === 'send' ||
    type === 'send_to_other' ||
    type === 'obfuscation_denominate' ||
    type === 'obfuscation_collateral_payment' ||
    type === 'obfuscation_make_collaterals' ||
    type === 'obfuscation_create_denominations';

  // labelOrAddr is derived from transaction.label / transaction.address. For
  // received transactions transaction.label is OUR receiving address's label
  // (not the sender), and for sent transactions transaction.address is the
  // wallet's funding-source address (not a recipient) — see notes in
  // TransactionDetailsDialog and internal/gui/core/CLAUDE.md. So this fallback
  // is only safe for the self-transfer / no-`from_address` paths below.
  const labelOrAddr = transaction.label || (transaction.address ? truncateAddress(transaction.address, 12, 10) : '');
  let displayAddress: string;
  let displayTooltip: string;

  if (isSelfTransfer) {
    displayAddress = labelOrAddr || t('transactions.address.unknown');
    displayTooltip = transaction.address || displayAddress;
  } else if (isOutgoing) {
    // Drive the visible "To: …" from recipient_addresses[0] when available.
    // transaction.address is the wallet's funding-source address for send
    // transactions and MUST NOT be used as a recipient fallback (would render
    // "To: <our own address>", a real bug both Codex rounds caught). When
    // recipients are unavailable (cache miss / storage miss / backend gap),
    // prefer the user-supplied label captured at send time; if neither
    // recipient nor label is present, render "To: Unknown" — surfacing some
    // address is NOT better than honesty here, the funding-source address
    // would actively mislead. TransactionDetailsDialog applies the same
    // discipline ("Sent from" fallback path).
    const recipients = transaction.recipient_addresses ?? [];
    const primaryRecipient = recipients[0] ?? '';
    const truncatedPrimary = primaryRecipient ? truncateAddress(primaryRecipient, 12, 10) : '';
    const recipientLabel = truncatedPrimary || transaction.label || '';
    const extra =
      recipients.length >= 2 ? ` ${t('transactions.address.moreSuffix', { count: recipients.length - 1 })}` : '';
    const toPrefix = t('transactions.address.toPrefix');
    const unknownLabel = t('transactions.address.unknown');
    displayAddress = recipientLabel
      ? `${toPrefix}: ${recipientLabel}${extra}`
      : `${toPrefix}: ${unknownLabel}${extra}`;
    displayTooltip =
      recipients.length >= 2
        ? recipients.join('\n')
        : (primaryRecipient || transaction.label || unknownLabel);
  } else {
    // Incoming bucket: receive / generated / stake / masternode / minted /
    // obfuscated / other. When from_address is populated we MUST show the
    // sender's address (truncated) — using labelOrAddr here would render
    // "From: <our own receiving label>", which is misleading. We don't carry
    // a separate label for the sender. When from_address is missing, fall
    // back to our local label/address (which IS the labeled receiving entry
    // — e.g. "Savings" reads fine as a bare row identity in that case).
    const hasFrom = (transaction.from_address ?? '') !== '';
    const unknownLabel = t('transactions.address.unknown');
    if (hasFrom) {
      const fromAddr = transaction.from_address!;
      displayAddress = `${t('transactions.address.fromPrefix')}: ${truncateAddress(fromAddr, 12, 10)}`;
      displayTooltip = fromAddr;
    } else {
      displayAddress = labelOrAddr || unknownLabel;
      displayTooltip = transaction.address || labelOrAddr || unknownLabel;
    }
  }

  return (
    <div
      data-tx-key={rowKey}
      style={{
        display: 'flex',
        gap: '12px',
        alignItems: 'center',
        padding: '10px 12px',
        backgroundColor: '#2a2a2a',
        border: '1px solid transparent',
        borderRadius: '6px',
        cursor: 'default',
        transition: 'border-color 0.15s',
      }}
      onContextMenu={(e) => onContextMenu(e, transaction)}
      onMouseEnter={(e) => { e.currentTarget.style.borderColor = '#444'; }}
      onMouseLeave={(e) => { e.currentTarget.style.borderColor = 'transparent'; }}
    >
      {/* Status Icon */}
      <div style={{ width: COL.statusIcon, flexShrink: 0, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <ConfirmationRing
          typeIcon={typeIcon}
          confirmations={transaction.confirmations || 0}
          isConflicted={transaction.is_conflicted || false}
          isCoinstake={transaction.is_coinstake || false}
          maturesIn={transaction.matures_in || 0}
          size={DECORATION_SIZE}
        />
      </div>

      {/* Date */}
      <span
        style={{ width: COL.date, flexShrink: 0, fontSize: '14px', color: '#ddd', whiteSpace: 'nowrap', cursor: 'default' }}
        title={formattedDateUTC}
      >
        {formattedDate}
      </span>

      {/* Type */}
      <span
        style={{ width: COL.type, flexShrink: 0, fontSize: '14px', color: '#ddd', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}
        title={typeLabel}
      >
        {typeLabel}
      </span>

      {/* Address */}
      <span
        style={{ flex: 1, minWidth: 0, fontSize: '14px', color: '#ddd', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
        title={displayTooltip}
      >
        {displayAddress}
      </span>

      {/* Amount */}
      <span
        style={{ width: COL.amount, flexShrink: 0, fontSize: '14px', fontWeight: 600, fontFamily: 'monospace', textAlign: 'right', color: amountColorClass, whiteSpace: 'nowrap' }}
      >
        {formattedAmount}
      </span>

      {/* Eye (open details) */}
      <div style={{ width: COL.eye, flexShrink: 0, display: 'flex', justifyContent: 'center' }}>
        <IconButton
          size={26}
          icon={<Eye size={14} />}
          onClick={() => onOpenDetails(transaction)}
          title={viewDetailsLabel}
          ariaLabel={viewDetailsLabel}
        />
      </div>
    </div>
  );
});

/**
 * Skeleton row component for loading state
 * Mimics TransactionRow structure with animated placeholders
 */
const SkeletonRow: React.FC<{ index: number }> = ({ index }) => {
  // Vary widths slightly for more natural look
  const addressWidth = 120 + (index % 3) * 40;
  const amountWidth = 60 + (index % 2) * 20;

  return (
    <div
      style={{
        display: 'flex',
        gap: '12px',
        alignItems: 'center',
        padding: '10px 12px',
        backgroundColor: '#2a2a2a',
        border: '1px solid transparent',
        borderRadius: '6px',
      }}
    >
      <div style={{ width: COL.statusIcon, flexShrink: 0, display: 'flex', justifyContent: 'center' }}>
        <div
          className="animate-pulse"
          style={{ width: '32px', height: '32px', backgroundColor: '#3a3a3a', borderRadius: '50%', animationDelay: `${index * 50 + 25}ms` }}
        />
      </div>
      <div
        className="animate-pulse"
        style={{ width: '100px', height: '14px', backgroundColor: '#3a3a3a', borderRadius: '4px', flexShrink: 0, animationDelay: `${index * 50 + 50}ms` }}
      />
      <div
        className="animate-pulse"
        style={{ width: '80px', height: '14px', backgroundColor: '#3a3a3a', borderRadius: '4px', flexShrink: 0, animationDelay: `${index * 50 + 75}ms` }}
      />
      <div
        className="animate-pulse"
        style={{ width: `${addressWidth}px`, height: '14px', backgroundColor: '#3a3a3a', borderRadius: '4px', flex: 1, animationDelay: `${index * 50 + 100}ms` }}
      />
      <div
        className="animate-pulse"
        style={{ width: `${amountWidth}px`, height: '14px', backgroundColor: '#3a3a3a', borderRadius: '4px', flexShrink: 0, marginLeft: 'auto', animationDelay: `${index * 50 + 125}ms` }}
      />
      <div style={{ width: COL.eye, flexShrink: 0 }} />
    </div>
  );
};

/**
 * Skeleton loader component showing multiple skeleton rows
 */
const TransactionsSkeleton: React.FC = () => {
  // Show 10 skeleton rows for visual consistency
  const skeletonRows = Array.from({ length: 10 }, (_, i) => i);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '4px', padding: '8px 8px 12px 8px' }}>
      {skeletonRows.map((i) => (
        <SkeletonRow key={i} index={i} />
      ))}
    </div>
  );
};

/**
 * Sortable column header component
 */
interface SortableHeaderProps {
  label: string;
  column: SortColumn;
  currentColumn: SortColumn;
  direction: 'asc' | 'desc';
  onSort: (column: SortColumn) => void;
  width?: string;
  flex?: number;
  align?: 'left' | 'right';
}

const SortableHeader: React.FC<SortableHeaderProps> = ({
  label,
  column,
  currentColumn,
  direction,
  onSort,
  width,
  flex,
  align = 'left',
}) => {
  const isActive = currentColumn === column;
  const [hovered, setHovered] = useState(false);

  // Active column reads in TWINS green + bold so the sorted column is unambiguous
  // against the inactive #888 labels on the #2f2f2f card. Hover-flip to #ddd is
  // gated on !isActive so it cannot override the active-column color.
  const labelColor = isActive ? '#27ae60' : hovered ? '#ddd' : '#888';
  const labelWeight = isActive ? 600 : 500;

  // A4: aria-sort surfaces the active sort direction to assistive tech.
  // `none` for inactive columns, `ascending`/`descending` for the active column.
  const ariaSort: 'ascending' | 'descending' | 'none' = isActive
    ? direction === 'asc'
      ? 'ascending'
      : 'descending'
    : 'none';

  return (
    <div
      role="columnheader"
      aria-sort={ariaSort}
      style={{
        width,
        flex,
        minWidth: flex ? 0 : undefined,
        fontSize: '11px',
        fontWeight: labelWeight,
        color: labelColor,
        // U5 (2026-05-22): dropped textTransform: uppercase + letterSpacing 0.5px
        // for visual parity with row cells (which render in Title Case). The
        // SortableHeader callsites at the bottom of this file pass Title Case
        // literals ("Date" / "Type" / "Address" / "Amount") so no i18n string
        // edits are required.
        cursor: 'pointer',
        userSelect: 'none',
        display: 'flex',
        alignItems: 'center',
        gap: '4px',
        justifyContent: align === 'right' ? 'flex-end' : 'flex-start',
        flexShrink: 0,
        transition: 'color 0.15s',
      }}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      onClick={() => onSort(column)}
    >
      <span>{label}</span>
      {isActive && (direction === 'asc' ? <ChevronUp size={14} color="#27ae60" /> : <ChevronDown size={14} color="#27ae60" />)}
    </div>
  );
};

// (Local RowsPerPageSelect was promoted to `shared/components/RowsPerPageSelect.tsx`
// and the entire footer state machine moved into `shared/components/PaginationFooter.tsx`
// in task m-extract-shared-pagination-footer when the Explorer BlockList became
// the second consumer.)

/**
 * Main Transactions Page Component
 * Matches Qt wallet's transactionview appearance
 */
export const Transactions: React.FC = () => {
  const { t } = useTranslation('wallet');
  const { unitLabel } = useDisplayUnits();
  const {
    transactions,
    total,
    totalAll,
    totalPages,
    isLoading,
    error,
    currentPage,
    pageSize,
    dateFilter,
    typeFilter,
    searchText,
    minAmount,
    dateRangeFrom,
    dateRangeTo,
    watchOnlyFilter,
    syncHideOrphanStakes,
    syncBlockExplorerUrls,
    blockExplorerUrls,
    sortColumn,
    sortDirection,
    newTransactionCount,
    fetchPage,
    setPage,
    setPageSize,
    setSortColumn,
    exportCSV,
    incrementNewTransactionCount,
    clearNewTransactionCount,
    clearFilters,
  } = useTransactions();

  const { addNotification } = useNotifications();
  const { formatDateHeader } = useDisplayDateTime();

  // Exporting state
  const [isExporting, setIsExporting] = useState(false);
  // U1 (2026-05-22): Export now opens a confirm dialog before triggering the
  // native save dialog so the user sees exactly what is being exported and in
  // what format. Resolves the prior UX where the native save dialog was the
  // only feedback, and users couldn't tell whether the export would include
  // the active filter set or the entire transaction list.
  const [exportConfirmOpen, setExportConfirmOpen] = useState(false);

  // Tracks whether `blockExplorerUrls` reflects the user's settings or is
  // still at its initial empty state during the sync window. Without this
  // gate, a user with custom URLs configured can right-click immediately
  // after page mount (or during the brief boot-to-preload window) and the
  // "Open in explorer" menu item routes them to the legacy fallback
  // instead of their configured explorer. Mirrors the symmetric guard in
  // `TransactionDetailsDialog.tsx` (`urlsResolved` state).
  //
  // Initialized `true` when the store already has URLs (the common case
  // because the `TransactionsPreloadHandler` in `app/App.tsx` runs on app
  // boot and primes the slice). Set to `true` after the in-page sync
  // settles, regardless of whether the user has URLs configured (an
  // empty resolved state is a valid signal — use the legacy fallback).
  const [urlsResolved, setUrlsResolved] = useState(blockExplorerUrls.length > 0);

  // Sync settings from GUISettings and fetch first page on mount
  useEffect(() => {
    let cancelled = false;
    Promise.resolve(syncBlockExplorerUrls()).finally(() => {
      if (!cancelled) setUrlsResolved(true);
    });
    syncHideOrphanStakes().then(() => fetchPage());
    return () => { cancelled = true; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Re-evaluate `urlsResolved` when `blockExplorerUrls` changes mid-session.
  // The Transactions page can stay mounted for hours (user navigates away and
  // back via tabs without unmount). If the user clears `strThirdPartyTxUrls`
  // in Settings (hot-reloaded via `syncBlockExplorerUrls()` from
  // `optionsSlice.applySettings`), the store flips to `[]` but `urlsResolved`
  // remains `true` from the initial sync — opening the menu item would route
  // to the legacy fallback even though the user's intent was "no explorer".
  // Mirror the `TransactionDetailsDialog.tsx` pattern: when the store flips
  // to empty, mark unresolved and re-run the sync; when the store has URLs,
  // mark resolved immediately.
  useEffect(() => {
    if (blockExplorerUrls.length > 0) {
      setUrlsResolved(true);
      return;
    }
    let cancelled = false;
    setUrlsResolved(false);
    Promise.resolve(syncBlockExplorerUrls()).finally(() => {
      if (!cancelled) setUrlsResolved(true);
    });
    return () => { cancelled = true; };
  }, [blockExplorerUrls.length, syncBlockExplorerUrls]);

  // Calculate display range for footer
  const rangeStart = total > 0 ? (currentPage - 1) * pageSize + 1 : 0;
  const rangeEnd = Math.min(currentPage * pageSize, total);

  // Page-input race-safety, jump-to-page handler, and Intl-formatted count
  // line all live inside the shared <PaginationFooter> below. The Export
  // confirm dialog still consumes a top-level `numberFmt` (one-line memo) to
  // format counts in its message body.
  const numberFmt = React.useMemo(() => new Intl.NumberFormat('en-US'), []);

  // Context menu state
  const [contextMenu, setContextMenu] = useState<ContextMenuState>({
    visible: false,
    x: 0,
    y: 0,
    transaction: null,
  });
  const contextMenuRef = useRef<HTMLDivElement>(null);
  const tableScrollRef = useRef<HTMLDivElement>(null);

  // Sub-popover state for the "Open in explorer" menu item when the user has
  // 2+ block-explorer URLs configured. Position is captured (fixed) at click
  // time relative to the trigger button's bounding rect; popover closes on
  // outside click, Escape (capture-phase + stopPropagation so the parent
  // context-menu Escape listener does not also fire), and scroll/resize
  // (parent context menu is also position: fixed; closing the popover keeps
  // the two from visually decoupling).
  const [explorerPopover, setExplorerPopover] = useState<{ x: number; y: number } | null>(null);
  const explorerTriggerRef = useRef<HTMLButtonElement>(null);
  const explorerPopoverRef = useRef<HTMLDivElement>(null);

  // FLIP row-reorder animation: when the row order changes (sort, page nav,
  // filter narrowing), rows that exist in both before and after render frames
  // animate from their old position to their new position. New rows fade in;
  // removed rows unmount without exit animation (acceptable MVP trade-off).
  //
  // Approach: data-tx-key attribute on each row, measure top in useLayoutEffect
  // by querying `rowListRef.current.querySelectorAll('[data-tx-key]')`. On
  // commit, compute delta = prev_top - current_top; if non-zero, apply inverse
  // translateY synchronously, then on next animation frame transition to
  // translateY(0) with a 250ms easing curve.
  const rowListRef = useRef<HTMLDivElement>(null);
  const previousRowTopsRef = useRef<Map<string, number>>(new Map());

  // FLIP row-reorder animation. Runs after every render that may have changed
  // the row order. Steps per row that exists in BOTH frames:
  //   1. Read new top via offsetTop (scroll-invariant; viewport-relative
  //      getBoundingClientRect().top was scroll-polluted and caused a false
  //      delta = scrollTop on the next commit after user-scrolled).
  //   2. delta = prev_top - new_top. If non-zero, apply inverse translateY
  //      synchronously (no transition) so the row visually stays where it was.
  //   3. requestAnimationFrame: clear transform + add a transform transition.
  //      The browser animates from the inverse translate back to 0.
  // New rows (no entry in previousRowTopsRef): no animation; they appear in
  // place. Removed rows: unmount immediately (no exit animation; deferred).
  //
  // Honor prefers-reduced-motion: if the user has the OS-level reduce-motion
  // accessibility setting on, skip animation entirely and just record positions.
  //
  // Dep array enumerates the only state changes that can reorder rows:
  // `transactions` (the slice array reference changes on any data refresh
  // including fetchPage), `sortColumn` / `sortDirection`, `currentPage`,
  // `pageSize`. Skipping the dep array would force a 100-row
  // `getBoundingClientRect()` reflow on every parent re-render (e.g. opening
  // the Export confirm modal via `setExportConfirmOpen(true)`), which the
  // user perceives as a flicker of the entire table. The previous
  // justification — "after every commit, even after non-reorder renders
  // (selection toggle)" — became stale when `l-remove-tx-table-row-selection`
  // (2026-05-20) removed checkbox selection, so no non-reorder render path
  // needs FLIP position capture any more. See task
  // h-fix-tx-row-re-render-on-modal-open.
  useLayoutEffect(() => {
    const container = rowListRef.current;
    if (!container) return;

    const reduceMotion =
      typeof window !== 'undefined' &&
      window.matchMedia &&
      window.matchMedia('(prefers-reduced-motion: reduce)').matches;

    const rows = container.querySelectorAll<HTMLElement>('[data-tx-key]');
    const previousTops = previousRowTopsRef.current;
    const nextTops = new Map<string, number>();

    // Clear any leftover transform/transition from a still-in-flight prior
    // FLIP cycle BEFORE measuring. Without this, a rapid second sort/filter
    // would measure rects mid-animation (translateY still applied) and
    // compute a wrong inverse — producing a visible jump on the second
    // animation. Resetting inline transform first ensures every measurement
    // starts from the layout-committed steady-state.
    //
    // ONLY touch rows that have an in-flight FLIP — gate by inline `transform`.
    // Unconditionally clearing `transition` on every row would wipe the
    // row's base `transition: border-color 0.15s` set in TransactionRow's
    // style block, killing the hover/selection border animation for the
    // common case of non-reorder renders (initial mount, checkbox toggle).
    // The base transition is set inline on the row element itself, so
    // CSS cascade can't restore it for us after we overwrote it.
    rows.forEach((row) => {
      if (row.style.transform) {
        row.style.transform = '';
        row.style.transition = '';
      }
    });

    rows.forEach((row) => {
      const key = row.getAttribute('data-tx-key');
      if (!key) return;
      // Use offsetTop (relative to scroll container's content) instead of
      // getBoundingClientRect().top (viewport-relative). offsetTop is
      // scroll-invariant: scrolling does NOT change a row's offsetTop, only
      // its rect.top. Critical because user scrolling does NOT trigger a
      // React commit, so previousRowTopsRef is never refreshed during scroll.
      // If we stored viewport-relative tops, the next commit (e.g.
      // fetchPage on Export click) would see stale mount-time positions,
      // compute a false delta equal to scrollTop, and FLIP would animate
      // every row back to its mount-time viewport position — the exact
      // visible "list rolls back to row 1 then settles at row 7" bug
      // confirmed by user screen recording in task h-fix-tx-row-re-render-on-modal-open.
      const top = (row as HTMLElement).offsetTop;
      nextTops.set(key, top);

      if (reduceMotion) return;

      const prevTop = previousTops.get(key);
      if (prevTop === undefined) return; // new row — no FLIP
      const delta = prevTop - top;
      if (delta === 0 || Math.abs(delta) < 1) return;

      // Step 2: invert
      row.style.transition = 'none';
      row.style.transform = `translateY(${delta}px)`;
    });

    previousRowTopsRef.current = nextTops;

    if (reduceMotion) return;

    // Step 3: play on next frame so the browser has painted the inverse
    // transform. Cancel rAF on cleanup so a rapid re-render doesn't mutate
    // detached/recycled nodes. Containment check on each row is an extra
    // belt-and-suspenders against unmount-between-effect-and-rAF.
    const rafId = requestAnimationFrame(() => {
      rows.forEach((row) => {
        if (!container.contains(row)) return;
        if (row.style.transform) {
          row.style.transition = 'transform 250ms cubic-bezier(0.25, 0.1, 0.25, 1), border-color 0.15s';
          row.style.transform = '';
        }
      });
    });
    return () => cancelAnimationFrame(rafId);
  }, [transactions, sortColumn, sortDirection, currentPage, pageSize]);

  // Reset the table scroll position to the top whenever the displayed dataset
  // changes shape — sort, pagination, or any filter. NOTE: `pageSize` is
  // intentionally NOT in this dependency array. U7 (2026-05-22) explicitly
  // preserves viewport position on page-size changes via setPageSize's page
  // recompute; resetting scrollTop here would defeat that. Sort / page /
  // filter changes still reset because they produce semantically different
  // top rows. Instant (no smooth animation) per user spec.
  useEffect(() => {
    if (tableScrollRef.current) {
      tableScrollRef.current.scrollTop = 0;
    }
  }, [
    sortColumn,
    sortDirection,
    currentPage,
    dateFilter,
    typeFilter,
    searchText,
    minAmount,
    dateRangeFrom,
    dateRangeTo,
    watchOnlyFilter,
  ]);

  // Transaction details dialog state
  const [detailsDialogOpen, setDetailsDialogOpen] = useState(false);
  const [selectedTransaction, setSelectedTransaction] = useState<core.Transaction | null>(null);

  // Edit label dialog state
  const [editLabelDialogOpen, setEditLabelDialogOpen] = useState(false);
  const [editLabelAddress, setEditLabelAddress] = useState('');
  const [editLabelCurrentLabel, setEditLabelCurrentLabel] = useState('');


  // Subscribe to transaction events - show notification banner instead of auto-refreshing
  useEffect(() => {
    const unsubReceived = EventsOn('transaction:received', () => {
      incrementNewTransactionCount();
    });

    const unsubConfirmed = EventsOn('transaction:confirmed', () => {
      incrementNewTransactionCount();
    });

    return () => {
      EventsOff('transaction:received');
      EventsOff('transaction:confirmed');
      if (typeof unsubReceived === 'function') unsubReceived();
      if (typeof unsubConfirmed === 'function') unsubConfirmed();
    };
  }, [incrementNewTransactionCount]);

  // Handle double-click to open transaction details
  const handleOpenTransactionDetails = useCallback((transaction: core.Transaction) => {
    setSelectedTransaction(transaction);
    setDetailsDialogOpen(true);
  }, []);

  // Close transaction details dialog
  const handleCloseDetailsDialog = useCallback(() => {
    setDetailsDialogOpen(false);
    setSelectedTransaction(null);
  }, []);

  // Handle right-click on transaction row
  const handleContextMenu = useCallback((e: React.MouseEvent, transaction: core.Transaction) => {
    e.preventDefault();
    e.stopPropagation();

    // Calculate position with viewport boundary checking
    // Menu dimensions estimated from styling (minWidth: 200px, ~12 items at 32px each)
    const menuWidth = 220;
    const menuHeight = 420;
    const padding = 10;

    let x = e.clientX;
    let y = e.clientY;

    // Prevent menu from rendering off-screen (right/bottom edges)
    if (x + menuWidth + padding > window.innerWidth) {
      x = window.innerWidth - menuWidth - padding;
    }
    if (y + menuHeight + padding > window.innerHeight) {
      y = window.innerHeight - menuHeight - padding;
    }
    // Ensure minimum padding from left/top edges
    x = Math.max(padding, x);
    y = Math.max(padding, y);

    setContextMenu({
      visible: true,
      x,
      y,
      transaction,
    });
  }, []);

  // Close context menu
  const closeContextMenu = useCallback(() => {
    setContextMenu(prev => ({ ...prev, visible: false }));
  }, []);

  // Close context menu on click outside. The sub-popover for multi-URL
  // explorer picking is rendered as a sibling of `contextMenuRef`, not a
  // descendant — so we MUST treat `explorerPopoverRef` as logically inside
  // the menu here. Without this, mousedown on a popover item closes the
  // parent menu before the child <button>'s onClick (mouseup) ever fires,
  // and the user can never pick a URL when 2+ are configured.
  useEffect(() => {
    if (contextMenu.visible) {
      const handleClickOutside = (e: MouseEvent) => {
        const target = e.target as Node;
        const inMenu = contextMenuRef.current?.contains(target);
        const inPopover = explorerPopoverRef.current?.contains(target);
        if (!inMenu && !inPopover) {
          closeContextMenu();
        }
      };
      document.addEventListener('mousedown', handleClickOutside);
      return () => document.removeEventListener('mousedown', handleClickOutside);
    }
  }, [contextMenu.visible, closeContextMenu]);

  // Close context menu on Escape key
  useEffect(() => {
    if (contextMenu.visible) {
      const handleKeyDown = (e: KeyboardEvent) => {
        if (e.key === 'Escape') {
          closeContextMenu();
        }
      };
      document.addEventListener('keydown', handleKeyDown);
      return () => document.removeEventListener('keydown', handleKeyDown);
    }
  }, [contextMenu.visible, closeContextMenu]);

  // Copy to clipboard helper
  const copyToClipboard = useCallback(async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      // Clipboard API may fail in some contexts - silently fail
    }
    closeContextMenu();
  }, [closeContextMenu]);

  // Context menu action handlers
  const handleCopyAddress = useCallback(() => {
    if (contextMenu.transaction?.address) {
      copyToClipboard(contextMenu.transaction.address);
    }
  }, [contextMenu.transaction, copyToClipboard]);

  const handleCopyTxID = useCallback(() => {
    if (contextMenu.transaction?.txid) {
      copyToClipboard(contextMenu.transaction.txid);
    }
  }, [contextMenu.transaction, copyToClipboard]);

  const handleEditLabel = useCallback(() => {
    if (contextMenu.transaction?.address) {
      setEditLabelAddress(contextMenu.transaction.address);
      setEditLabelCurrentLabel(contextMenu.transaction.label || '');
      setEditLabelDialogOpen(true);
    }
    closeContextMenu();
  }, [contextMenu.transaction, closeContextMenu]);

  // Handle label updated - refresh transactions to show new label
  const handleLabelUpdated = useCallback((_address: string, newLabel: string) => {
    // Refresh current page to show updated label
    fetchPage();
    addNotification({
      type: 'success',
      title: 'Label updated',
      message: newLabel ? `Label set to "${newLabel}"` : 'Label cleared',
    });
  }, [fetchPage, addNotification]);

  // Close edit label dialog
  const handleCloseEditLabelDialog = useCallback(() => {
    setEditLabelDialogOpen(false);
  }, []);

  // Open the given block-explorer URL template (with `%s` placeholder) for the
  // current context-menu txid. Validates txid as 64-hex so a malformed value
  // can never break out of the URL path segment.
  const openExplorerWithTemplate = useCallback((urlTemplate: string) => {
    const txid = contextMenu.transaction?.txid;
    if (!txid) return;
    const txidRegex = /^[a-fA-F0-9]{64}$/;
    if (!txidRegex.test(txid)) return;
    BrowserOpenURL(buildExplorerURL(urlTemplate, txid));
  }, [contextMenu.transaction]);

  // "Open in explorer" menu-item click handler. Three branches:
  //   0 URLs configured → open the legacy fallback directly
  //   1 URL configured  → open that URL directly
  //   2+ URLs           → toggle the sub-popover (rendered to the right of
  //                       the trigger button, position captured at click time)
  //
  // Guarded by `urlsResolved`: if the user has custom URLs configured in
  // `strThirdPartyTxUrls` but the sync hasn't landed yet, `blockExplorerUrls`
  // is transiently `[]` and would route to the legacy fallback — flagged
  // by codex as a regression. The button's `disabled` attribute covers the
  // visual + onClick path, but the inline `onKeyDown` handler for Enter/Space
  // is NOT gated by HTML `disabled` (browsers fire keydown on disabled
  // buttons), so this early-return is load-bearing for keyboard-accessibility.
  // Do not remove without also removing the onKeyDown handler.
  const handleOpenInExplorerClick = useCallback(() => {
    if (!urlsResolved) return;
    if (blockExplorerUrls.length === 0) {
      openExplorerWithTemplate(LEGACY_EXPLORER_TX_FALLBACK);
      closeContextMenu();
      return;
    }
    if (blockExplorerUrls.length === 1) {
      openExplorerWithTemplate(blockExplorerUrls[0].url);
      closeContextMenu();
      return;
    }
    // 2+ URLs: toggle popover
    if (explorerPopover) {
      setExplorerPopover(null);
      return;
    }
    if (explorerTriggerRef.current) {
      const rect = explorerTriggerRef.current.getBoundingClientRect();
      // Anchor at the right edge of the trigger menu item; the popover itself
      // renders with `left: rect.right` and grows rightward. If it would
      // overflow the viewport on the right, flip to the left of the trigger.
      const popoverWidth = 200;
      const viewportRight = window.innerWidth - 10;
      const x = rect.right + popoverWidth > viewportRight
        ? Math.max(10, rect.left - popoverWidth)
        : rect.right;
      setExplorerPopover({ x, y: rect.top });
    }
  }, [urlsResolved, blockExplorerUrls, openExplorerWithTemplate, closeContextMenu, explorerPopover]);

  const handleExplorerPick = useCallback((url: string) => {
    openExplorerWithTemplate(url);
    setExplorerPopover(null);
    closeContextMenu();
  }, [openExplorerWithTemplate, closeContextMenu]);

  // Close the explorer sub-popover whenever the parent context menu closes —
  // prevents an orphaned popover surviving past its trigger.
  useEffect(() => {
    if (!contextMenu.visible) {
      setExplorerPopover(null);
    }
  }, [contextMenu.visible]);

  // Outside-click closes the explorer sub-popover (but does not close the
  // parent context menu — that has its own listener).
  useEffect(() => {
    if (!explorerPopover) return;
    const handler = (e: MouseEvent) => {
      const target = e.target as Node;
      if (
        explorerPopoverRef.current && !explorerPopoverRef.current.contains(target) &&
        explorerTriggerRef.current && !explorerTriggerRef.current.contains(target)
      ) {
        setExplorerPopover(null);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [explorerPopover]);

  // Escape closes the popover only (not the parent menu). Capture phase +
  // stopPropagation prevents the parent context-menu Escape listener from
  // also firing — without this guard, Escape would dismiss the whole menu
  // instead of just the popover.
  useEffect(() => {
    if (!explorerPopover) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        setExplorerPopover(null);
      }
    };
    document.addEventListener('keydown', handler, true);
    return () => document.removeEventListener('keydown', handler, true);
  }, [explorerPopover]);

  // Scroll / resize: popover is position: fixed at click-time coordinates, so
  // a viewport change would visually decouple it from its trigger. Close it.
  useEffect(() => {
    if (!explorerPopover) return;
    const handler = () => setExplorerPopover(null);
    window.addEventListener('resize', handler);
    document.addEventListener('scroll', handler, true);
    return () => {
      window.removeEventListener('resize', handler);
      document.removeEventListener('scroll', handler, true);
    };
  }, [explorerPopover]);

  // Handle new transaction banner click - refresh and clear count
  // Also reset scroll: if user was already on page 1, currentPage does not change,
  // so the dependency-array effect would not fire. Reset scrollTop directly here.
  const handleNewTransactionBannerClick = useCallback(() => {
    clearNewTransactionCount();
    fetchPage(1);
    if (tableScrollRef.current) {
      tableScrollRef.current.scrollTop = 0;
    }
  }, [clearNewTransactionCount, fetchPage]);

  // Export button click handler: opens the confirm dialog. The native save
  // dialog (and the backend CSV generation) only fires after the user
  // confirms via executeExport below.
  //
  // Forces a fresh `fetchPage(currentPage)` BEFORE opening the dialog so the
  // displayed `total` / `totalAll` reflect the latest in-memory filters even
  // when the user clicks Export within the 300 ms search/amount debounce
  // window. Without this, the confirm copy could disagree with the actual
  // count `exportCSV()` writes (Codex R3 W1, strict-mode enforced).
  const handleExport = useCallback(async () => {
    setIsExporting(true);
    try {
      await fetchPage(currentPage);
    } finally {
      setIsExporting(false);
    }
    const freshTotal = useStore.getState().total;
    if (freshTotal === 0) {
      addNotification({
        type: 'warning',
        title: 'No transactions to export',
        message: 'There are no transactions matching the current filters.',
      });
      return;
    }
    setExportConfirmOpen(true);
  }, [currentPage, fetchPage, addNotification]);

  const executeExport = useCallback(async () => {
    setExportConfirmOpen(false);
    setIsExporting(true);
    try {
      const saved = await exportCSV();
      if (saved) {
        // Read fresh total from the store rather than the closed-over value:
        // mirrors the freshness discipline in handleExport so the success
        // toast cannot drift from what exportCSV() actually wrote
        // (e.g. when a background P2P-driven refetch landed between the
        // confirm dialog opening and the user confirming).
        const freshTotal = useStore.getState().total;
        addNotification({
          type: 'success',
          title: 'Export successful',
          message: `Exported ${freshTotal} transaction${freshTotal !== 1 ? 's' : ''} to CSV file`,
        });
      }
      // If saved is false, user cancelled the native save dialog — silent.
    } catch (error) {
      addNotification({
        type: 'error',
        title: 'Export failed',
        message: error instanceof Error ? error.message : 'Failed to export transactions',
      });
    } finally {
      setIsExporting(false);
    }
  }, [exportCSV, addNotification]);

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <div style={{ flex: 1, overflow: 'auto', padding: '12px 16px', display: 'flex', flexDirection: 'column', gap: '12px', minHeight: 0 }}>
        {/* Filter Bar — chip-based UI (Phase 1 of filter migration). */}
        <div
          style={{
            backgroundColor: '#2f2f2f',
            border: '1px solid #3a3a3a',
            borderRadius: '8px',
            padding: '12px 16px',
          }}
        >
          <TxFilterBar />
        </div>

        {/* New Transaction Notification Banner */}
        {newTransactionCount > 0 && (
          <button
            type="button"
            onClick={handleNewTransactionBannerClick}
            style={{
              display: 'block',
              width: '100%',
              padding: 0,
              margin: 0,
              border: 'none',
              background: 'transparent',
              cursor: 'pointer',
              boxSizing: 'border-box',
              font: 'inherit',
              color: 'inherit',
              textAlign: 'left',
            }}
          >
            <Banner
              variant="info"
              message={`${newTransactionCount} new transaction${newTransactionCount !== 1 ? 's' : ''} - click to refresh`}
            />
          </button>
        )}

        {/* Transaction List — header + rows in a single Receive design-language card */}
        {/*
          minHeight prevents the card from collapsing when a filter narrows the
          result set (the "jump" the user flagged). Capped at
          `calc(100vh - 360px)` so a large pageSize (e.g. 250) doesn't force the
          outer page to scroll — the card stops growing once it fills the
          available viewport, and excess rows scroll inside the card via
          overflow:auto. The `min(...)` chooses whichever is smaller: enough
          height for pageSize rows, or what fits in the viewport.
        */}
        <div
          ref={tableScrollRef}
          style={{
            flex: 1,
            minHeight: `min(${HEADER_HEIGHT_PX + pageSize * ROW_HEIGHT_PX + SCROLL_LIST_VPAD_PX}px, calc(100vh - 360px))`,
            overflow: 'auto',
            display: 'flex',
            flexDirection: 'column',
            backgroundColor: '#2f2f2f',
            border: '1px solid #3a3a3a',
            borderRadius: '8px',
          }}
        >
          {/*
            Render order matters: we keep stale rows visible during refetches
            instead of flashing the skeleton on every filter/sort/page change.
            Branch precedence:
              1. Error → always replaces (broken state is more important than stale).
              2. Empty + loading → first-load skeleton (no stale data yet).
              3. Empty + not loading → empty-state placeholder.
              4. Has rows → render them, regardless of `isLoading`. The slice
                 keeps the previous page in state until the new one arrives,
                 so subsequent fetches show stale rows for the 10–50ms request
                 window and then swap in place. Matches the convention adopted
                 by `ReceivingAddressesDialog` (2026-04-08).
          */}
          {error ? (
            <div style={{ padding: '32px', textAlign: 'center', color: '#ff6666', fontSize: '12px' }}>
              {t('transactions.error', { message: error })}
            </div>
          ) : transactions.length === 0 && isLoading ? (
            <TransactionsSkeleton />
          ) : transactions.length === 0 && totalAll > 0 ? (
            // U8 (2026-05-22): proper empty-state when filters narrow the list
            // to zero matches. Distinct from the truly-empty wallet branch
            // below (totalAll === 0) which keeps the legacy single-line copy.
            <div
              style={{
                padding: '48px 32px',
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                gap: '8px',
              }}
            >
              <Inbox size={32} color="#666" />
              <div style={{ fontSize: '13px', fontWeight: 600, color: '#ccc' }}>
                {t('transactions.emptyState.filteredTitle')}
              </div>
              <div style={{ fontSize: '12px', color: '#888', textAlign: 'center', maxWidth: '320px' }}>
                {t('transactions.emptyState.filteredMessage')}
              </div>
              <button
                onClick={() => clearFilters()}
                style={{
                  marginTop: '8px',
                  padding: '6px 14px',
                  fontSize: '12px',
                  backgroundColor: 'transparent',
                  border: '1px solid #4a4a4a',
                  borderRadius: '6px',
                  color: '#ccc',
                  cursor: 'pointer',
                  transition: 'background-color 0.15s, border-color 0.15s',
                }}
                onMouseEnter={(e) => {
                  e.currentTarget.style.backgroundColor = '#444';
                  e.currentTarget.style.borderColor = '#5a5a5a';
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.backgroundColor = 'transparent';
                  e.currentTarget.style.borderColor = '#4a4a4a';
                }}
              >
                {t('transactions.emptyState.clearFiltersButton')}
              </button>
            </div>
          ) : transactions.length === 0 ? (
            <div style={{ padding: '32px', textAlign: 'center', color: '#888', fontSize: '12px' }}>
              {t('transactions.noTransactions')}
            </div>
          ) : (
            <>
              {/* Column header row — inside the same card as rows, separated by border-bottom */}
              <div
                style={{
                  display: 'flex',
                  gap: '12px',
                  alignItems: 'center',
                  // padding-left/right of 20px = 8px row-list wrapper padding + 12px row padding,
                  // so the header columns align with the row columns.
                  padding: '10px 20px',
                  borderBottom: '1px solid #3a3a3a',
                  position: 'sticky',
                  top: 0,
                  zIndex: 10,
                  backgroundColor: '#2f2f2f',
                }}
              >
                {/* Status Icon Header (no sort) */}
                <div style={{ width: COL.statusIcon, flexShrink: 0 }} />

                <SortableHeader
                  label={formatDateHeader()}
                  column="date"
                  currentColumn={sortColumn}
                  direction={sortDirection}
                  onSort={setSortColumn}
                  width={COL.date}
                />

                <SortableHeader
                  label="Type"
                  column="type"
                  currentColumn={sortColumn}
                  direction={sortDirection}
                  onSort={setSortColumn}
                  width={COL.type}
                />

                <SortableHeader
                  label="Address"
                  column="address"
                  currentColumn={sortColumn}
                  direction={sortDirection}
                  onSort={setSortColumn}
                  flex={1}
                />

                <SortableHeader
                  label={`Amount (${unitLabel})`}
                  column="amount"
                  currentColumn={sortColumn}
                  direction={sortDirection}
                  onSort={setSortColumn}
                  width={COL.amount}
                  align="right"
                />

                {/* Eye column placeholder (aligns with trailing IconButton on each row) */}
                <div style={{ width: COL.eye, flexShrink: 0 }} />
              </div>

              {/* Row list */}
              <div ref={rowListRef} style={{ display: 'flex', flexDirection: 'column', gap: '4px', padding: '8px' }}>
                {transactions.map((tx) => {
                  const k = `${tx.txid}:${tx.vout}`;
                  return (
                    <TransactionRow
                      key={k}
                      rowKey={k}
                      transaction={tx}
                      onOpenDetails={handleOpenTransactionDetails}
                      onContextMenu={handleContextMenu}
                      viewDetailsLabel={t('transactions.viewDetails')}
                    />
                  );
                })}
              </div>
            </>
          )}
        </div>

        {/* Context Menu */}
        {contextMenu.visible && contextMenu.transaction && (
          <div
            ref={contextMenuRef}
            role="menu"
            aria-label="Transaction context menu"
            style={{
              position: 'fixed',
              top: contextMenu.y,
              left: contextMenu.x,
              backgroundColor: '#2f2f2f',
              border: '1px solid #3a3a3a',
              borderRadius: '6px',
              boxShadow: '0 4px 12px rgba(0,0,0,0.6)',
              zIndex: 1000,
              minWidth: '200px',
              padding: '4px',
            }}
          >
            {/* Copy address */}
            <button
              type="button"
              role="menuitem"
              onClick={handleCopyAddress}
              onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); handleCopyAddress(); } }}
              disabled={!contextMenu.transaction.address}
              style={{
                ...MENU_ITEM_BASE_STYLE,
                color: contextMenu.transaction.address ? '#ddd' : '#666',
                cursor: contextMenu.transaction.address ? 'pointer' : 'not-allowed',
              }}
              onMouseEnter={(e) => {
                if (contextMenu.transaction?.address) {
                  e.currentTarget.style.backgroundColor = '#383838';
                }
              }}
              onMouseLeave={(e) => (e.currentTarget.style.backgroundColor = 'transparent')}
            >
              <Copy size={14} />
              Copy address
            </button>

            {/* Copy transaction ID */}
            <button
              type="button"
              role="menuitem"
              onClick={handleCopyTxID}
              onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); handleCopyTxID(); } }}
              style={{ ...MENU_ITEM_BASE_STYLE, color: '#ddd', cursor: 'pointer' }}
              onMouseEnter={(e) => (e.currentTarget.style.backgroundColor = '#383838')}
              onMouseLeave={(e) => (e.currentTarget.style.backgroundColor = 'transparent')}
            >
              <Copy size={14} />
              Copy transaction ID
            </button>

            {/* Separator */}
            <div role="separator" style={{ height: '1px', backgroundColor: '#3a3a3a', margin: '4px 0' }} />

            {/* Edit label */}
            <button
              type="button"
              role="menuitem"
              onClick={handleEditLabel}
              onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); handleEditLabel(); } }}
              disabled={!contextMenu.transaction.address}
              style={{
                ...MENU_ITEM_BASE_STYLE,
                color: contextMenu.transaction.address ? '#ddd' : '#666',
                cursor: contextMenu.transaction.address ? 'pointer' : 'not-allowed',
              }}
              onMouseEnter={(e) => {
                if (contextMenu.transaction?.address) {
                  e.currentTarget.style.backgroundColor = '#383838';
                }
              }}
              onMouseLeave={(e) => (e.currentTarget.style.backgroundColor = 'transparent')}
            >
              <Edit size={14} />
              Edit label
            </button>

            {/* Open in explorer (single fixed item; 2+ URLs open a sub-popover).
                Disabled until `urlsResolved` is true — otherwise a user with
                custom URLs configured could right-click during the brief
                page-mount sync window and get routed to the legacy fallback. */}
            <button
              ref={explorerTriggerRef}
              type="button"
              role="menuitem"
              aria-haspopup={blockExplorerUrls.length >= 2 ? 'menu' : undefined}
              aria-expanded={blockExplorerUrls.length >= 2 ? !!explorerPopover : undefined}
              disabled={!urlsResolved}
              onClick={handleOpenInExplorerClick}
              onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); handleOpenInExplorerClick(); } }}
              style={{
                ...MENU_ITEM_BASE_STYLE,
                color: urlsResolved ? '#ddd' : '#666',
                cursor: urlsResolved ? 'pointer' : 'not-allowed',
              }}
              onMouseEnter={(e) => { if (urlsResolved) e.currentTarget.style.backgroundColor = '#383838'; }}
              onMouseLeave={(e) => (e.currentTarget.style.backgroundColor = 'transparent')}
            >
              <ExternalLink size={14} />
              Open in explorer
              {blockExplorerUrls.length >= 2 && (
                <span style={{ marginLeft: 'auto', color: '#888', fontSize: '11px' }}>▸</span>
              )}
            </button>

          </div>
        )}

        {/* Sub-popover for "Open in explorer" when 2+ URLs are configured.
            Rendered as a sibling of the main context menu so it floats above
            and is unaffected by the menu's clipping. */}
        {contextMenu.visible && explorerPopover && blockExplorerUrls.length >= 2 && (
          <div
            ref={explorerPopoverRef}
            role="menu"
            aria-label="Block explorer URLs"
            style={{
              position: 'fixed',
              top: explorerPopover.y,
              left: explorerPopover.x,
              backgroundColor: '#2f2f2f',
              border: '1px solid #3a3a3a',
              borderRadius: '6px',
              padding: '4px',
              boxShadow: '0 4px 12px rgba(0,0,0,0.6)',
              minWidth: '200px',
              zIndex: 1001,
            }}
          >
            {blockExplorerUrls.map((explorer) => (
              <button
                key={explorer.url}
                type="button"
                role="menuitem"
                onClick={() => handleExplorerPick(explorer.url)}
                onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); handleExplorerPick(explorer.url); } }}
                style={{ ...MENU_ITEM_BASE_STYLE, color: '#ddd', cursor: 'pointer' }}
                onMouseEnter={(e) => (e.currentTarget.style.backgroundColor = '#383838')}
                onMouseLeave={(e) => (e.currentTarget.style.backgroundColor = 'transparent')}
              >
                <ExternalLink size={14} />
                {explorer.hostname}
              </button>
            ))}
          </div>
        )}

        {/* Footer — shared <PaginationFooter> owns the 3-zone CSS Grid layout,
            page-input race-safety state machine, scoped Chromium spinner CSS,
            narrow-window media query, and Intl-formatted count line. Tx adds
            the Export button via `rightSlot`. Page indexing is 1-based on both
            the slice and the component contract — no conversion needed.
            `setPage` and `setPageSize` are passed through directly. */}
        <PaginationFooter
          rangeStart={rangeStart}
          rangeEnd={rangeEnd}
          total={total}
          totalUnfiltered={totalAll !== total ? totalAll : undefined}
          currentPage={currentPage}
          totalPages={totalPages}
          onPageChange={setPage}
          pageSize={pageSize}
          pageSizeOptions={PAGE_SIZES}
          onPageSizeChange={setPageSize}
          isLoading={isLoading}
          rightSlot={
            <button
              type="button"
              onClick={handleExport}
              disabled={isExporting}
              style={{
                padding: '8px 16px',
                fontSize: '12px',
                fontWeight: 500,
                backgroundColor: 'transparent',
                border: '1px solid #4a4a4a',
                borderRadius: '6px',
                color: isExporting ? '#888' : '#ccc',
                cursor: isExporting ? 'not-allowed' : 'pointer',
                display: 'flex',
                alignItems: 'center',
                gap: '6px',
                transition: 'background-color 0.15s',
              }}
              onMouseEnter={(e) => {
                if (!isExporting) e.currentTarget.style.backgroundColor = 'rgba(255, 255, 255, 0.03)';
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.backgroundColor = 'transparent';
              }}
            >
              <Download size={14} />
              {isExporting ? 'Exporting...' : t('transactions.footer.export')}
            </button>
          }
        />
      </div>

      {/* Transaction Details Dialog */}
      <TransactionDetailsDialog
        isOpen={detailsDialogOpen}
        onClose={handleCloseDetailsDialog}
        transaction={selectedTransaction}
      />

      {/* Edit Label Dialog */}
      <EditLabelDialog
        isOpen={editLabelDialogOpen}
        address={editLabelAddress}
        currentLabel={editLabelCurrentLabel}
        onClose={handleCloseEditLabelDialog}
        onLabelUpdated={handleLabelUpdated}
      />

      {/* U1 (2026-05-22): Export confirm dialog — see executeExport above. */}
      <SimpleConfirmDialog
        isOpen={exportConfirmOpen}
        title={t('transactions.export.confirmTitle')}
        message={
          total < totalAll
            ? `${t('transactions.export.confirmMessage', { total: numberFmt.format(total) })} ${t('transactions.export.filterInfo', { totalAll: numberFmt.format(totalAll) })}`
            : t('transactions.export.confirmMessage', { total: numberFmt.format(total) })
        }
        confirmText={t('transactions.export.confirmButton')}
        cancelText={t('transactions.export.cancelButton')}
        onConfirm={executeExport}
        onCancel={() => setExportConfirmOpen(false)}
        isLoading={isExporting}
      />
    </div>
  );
};
