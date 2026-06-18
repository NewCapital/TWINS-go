import React, { useState, useCallback, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { ArrowRight, Check, ChevronDown, ChevronUp, Coins, Copy, Layers, Receipt, RotateCw, Users, X } from 'lucide-react';
import { PaymentStatsResponse, PaymentStatsEntry } from '@/shared/types/masternode.types';
import { useDisplayUnits } from '@/shared/hooks/useDisplayUnits';
import { useDisplayDateTime } from '@/shared/hooks/useDisplayDateTime';
import { RefreshCountdown } from '@/shared/components/RefreshCountdown';
import { DashboardCard } from '@/shared/components/DashboardCard';
import { PaginationFooter } from '@/shared/components/PaginationFooter';
import { Banner } from '@/shared/components/Banner';
import { PillButton } from '@/shared/components/PillButton';
import { IconButton } from '@/shared/components/IconButton';
import { writeToClipboard } from '@/shared/utils/clipboard';
import { GetPaymentStats } from '@wailsjs/go/main/App';

const REFRESH_SECONDS = 60;

const PAGE_SIZES = [10, 25, 50, 100] as const;
type PageSize = typeof PAGE_SIZES[number];

// Tier colors matching MasternodeStatisticsPanel
const TIER_COLORS: Record<string, string> = {
  platinum: '#e5e4e2',
  gold: '#ffd700',
  silver: '#c0c0c0',
  bronze: '#cd7f32',
};

// Sortable columns (address is not sortable)
type SortColumn = 'tier' | 'paymentCount' | 'totalPaid' | 'lastPaidTime';
type SortDirection = 'asc' | 'desc';

// Column widths shared between the sticky header and the row cards.
// COL is referenced inline at each header cell + row cell to keep widths in sync.
// Canonical pattern mirrored from MasternodesTable.tsx:51 (m-restyle-payment-stats-tab-to-receive-design, 2026-06-12).
//
// Column order (left → right): address (with inline Copy IconButton) | tier | paymentCount | <8px spacer> | lastPaidTime | <flex:1 spacer> | totalPaid | refresh
// — totalPaid is rightmost data column matching the Amount-on-right convention from Transactions.tsx.
// — totalPaid = 200px: buffers µTWINS-scaled amounts (e.g. 357,920,000,000.00 ≈ 158px at 14px monospace) without ellipsis.
// — refresh = 32px: trailing actions cell hosts <RefreshCountdown size={26}> in the sticky header; row cards render an
//   empty 32px placeholder so column geometry between header and rows aligns 1:1. Canonical in-header refresh ring
//   pattern from l-explorer-blocks-refresh-ring-in-table-header (BlockList, 2026-06-03) and
//   m-masternodes-table-reorder-and-actions (MasternodesTable, 2026-06-11).
// — 8px spacer between paymentCount and lastPaidTime cells in BOTH sticky header AND PaymentRow doubles the effective
//   gap (parent flex gap: 8px + 8px spacer = 16px visible separation) so Payments numeric value (e.g. 16,456) doesn't
//   visually merge with the Last Payment relative-time string (e.g. "4 days ago"). See m-payment-stats-table-cleanup
//   (2026-06-12) for the rationale.
// — address = 290px: widened from 220px to fit the full 34-character TWINS address (~245px content at 12px monospace)
//   plus the inline 24px Copy IconButton + 6px gap + breathing room. The address cell no longer ellipsis-truncates;
//   full address is rendered verbatim. Slack absorbed by the <flex:1> spacer between lastPaidTime and totalPaid so
//   totalPaid + refresh stay anchored at the row right edge. See m-payment-stats-tier-empty-lastpaid-align-address-full
//   (2026-06-12) for the rationale.
// See m-payment-stats-table-restructure (2026-06-12) for the prior restructure rationale.
const COL = {
  address: '290px',
  tier: '80px',
  paymentCount: '90px',
  totalPaid: '200px',
  lastPaidTime: '120px',
  refresh: '32px',
} as const;

// HeaderCell — module-level sortable/static header cell helper.
// Mirrors MasternodesTable.tsx:206-277 verbatim with PaymentStatsTab's
// SortColumn union. Module-level declaration gives the component a stable
// function reference so React reconciliation preserves its useState(hovered)
// hook across parent re-renders (per the explanatory block in MasternodesTable.tsx
// noting that inline declaration would reset hover state on every parent render).
//
// When `onSort` is provided AND `column !== null`, renders the sortable variant:
// clickable + keyboard-activatable + aria-sort + ChevronUp/Down glyph on the
// active column. When `onSort` is omitted OR `column === null`, renders a
// static label with no listeners (used for the address column where sorting
// produces no meaningful ordering for the user).
interface HeaderCellProps {
  column: SortColumn | null;
  label: React.ReactNode;
  width?: string;
  flex?: number;
  align?: 'left' | 'right' | 'center';
  sortColumn: SortColumn;
  sortDirection: SortDirection;
  onSort?: (column: SortColumn) => void;
}

const HeaderCell: React.FC<HeaderCellProps> = ({
  column,
  label,
  width,
  flex,
  align = 'left',
  sortColumn,
  sortDirection,
  onSort,
}) => {
  const isSortable = onSort !== undefined && column !== null;
  const isActive = isSortable && sortColumn === column;
  const [hovered, setHovered] = useState(false);
  // When non-sortable, color stays #888 regardless of hover (handlers below are
  // gated on isSortable so static cells have no mouse listeners at all).
  const labelColor = isActive ? '#27ae60' : hovered ? '#ddd' : '#888';
  const labelWeight = isActive ? 600 : 500;
  return (
    <div
      role={isSortable ? 'button' : undefined}
      tabIndex={isSortable ? 0 : undefined}
      onClick={isSortable ? () => onSort!(column!) : undefined}
      onKeyDown={
        isSortable
          ? (e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                onSort!(column!);
              }
            }
          : undefined
      }
      onMouseEnter={isSortable ? () => setHovered(true) : undefined}
      onMouseLeave={isSortable ? () => setHovered(false) : undefined}
      aria-sort={
        isActive
          ? sortDirection === 'asc'
            ? 'ascending'
            : 'descending'
          : isSortable
          ? 'none'
          : undefined
      }
      style={{
        width,
        flex,
        minWidth: 0,
        cursor: isSortable ? 'pointer' : 'default',
        userSelect: 'none',
        display: 'flex',
        alignItems: 'center',
        justifyContent: align === 'right' ? 'flex-end' : align === 'center' ? 'center' : 'flex-start',
        gap: '4px',
        whiteSpace: 'nowrap',
      }}
    >
      <span style={{ fontSize: '11px', fontWeight: labelWeight, color: labelColor }}>{label}</span>
      {isActive && (sortDirection === 'asc' ? <ChevronUp size={14} color="#27ae60" /> : <ChevronDown size={14} color="#27ae60" />)}
    </div>
  );
};

// No props — PaymentStatsTab is fully self-contained.

// Format a number with thousands separators
function formatNumber(n: number): string {
  return n.toLocaleString();
}

export const PaymentStatsTab: React.FC = React.memo(() => {
  const { t } = useTranslation('masternode');
  const { formatAmount, unitLabel } = useDisplayUnits();
  const { formatTzSuffix } = useDisplayDateTime();

  // Sort state
  const [sortColumn, setSortColumn] = useState<SortColumn>('totalPaid');
  const [sortDirection, setSortDirection] = useState<SortDirection>('desc');
  const sortColumnRef = useRef<SortColumn>(sortColumn);
  sortColumnRef.current = sortColumn;

  // Pagination state
  const [currentPage, setCurrentPage] = useState(1);
  const [pageSize, setPageSize] = useState<PageSize>(10);

  // Data state — component owns its own data fetching
  const [stats, setStats] = useState<PaymentStatsResponse | null>(null);
  const [isFetching, setIsFetching] = useState(false);
  // Error state for the most recent fetch. Null when the last fetch succeeded
  // or the user dismissed the banner. Stays set across polls until cleared.
  // We deliberately do NOT clear `stats` on error so stale data remains visible.
  const [fetchError, setFetchError] = useState<string | null>(null);

  // Auto-refresh countdown
  const [countdown, setCountdown] = useState(REFRESH_SECONDS);
  const countdownRef = useRef(REFRESH_SECONDS);

  // Copy feedback state — tracks the most recently copied address for 2s green Check swap.
  const [copiedAddress, setCopiedAddress] = useState<string | null>(null);
  const copyTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Mounted ref to prevent state updates after unmount
  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      if (copyTimerRef.current) clearTimeout(copyTimerRef.current);
    };
  }, []);

  // Request-ID counter: prevents stale responses from overwriting fresh data
  // when multiple fetches are in flight (e.g. spam-clicking refresh or a slow
  // request crossing the auto-refresh boundary).
  const fetchIdRef = useRef(0);

  // Fetch data from backend with current sort/pagination params
  const fetchData = useCallback(async (page: number, size: PageSize, column: SortColumn, direction: SortDirection) => {
    const localFetchId = ++fetchIdRef.current;
    setIsFetching(true);
    try {
      const result = await GetPaymentStats({
        sortColumn: column,
        sortDirection: direction,
        page,
        pageSize: size,
      });
      if (mountedRef.current && localFetchId === fetchIdRef.current && result) {
        setStats(result as PaymentStatsResponse);
        setFetchError(null);
      }
    } catch (error) {
      console.error('Failed to fetch payment stats:', error);
      if (mountedRef.current && localFetchId === fetchIdRef.current) {
        // Keep existing `stats` untouched so stale data stays visible.
        setFetchError(error instanceof Error ? error.message : String(error));
      }
    } finally {
      if (mountedRef.current && localFetchId === fetchIdRef.current) {
        setIsFetching(false);
      }
    }
  }, []);

  // Stable ref for fetchData params to use in timer
  const fetchParamsRef = useRef({ currentPage, pageSize, sortColumn, sortDirection });
  fetchParamsRef.current = { currentPage, pageSize, sortColumn, sortDirection };

  // Initial fetch + refetch on sort/page changes — also resets countdown
  useEffect(() => {
    fetchData(currentPage, pageSize, sortColumn, sortDirection);
    countdownRef.current = REFRESH_SECONDS;
    setCountdown(REFRESH_SECONDS);
  }, [currentPage, pageSize, sortColumn, sortDirection, fetchData]);

  // Auto-refresh countdown timer
  useEffect(() => {
    const interval = setInterval(() => {
      countdownRef.current -= 1;
      setCountdown(countdownRef.current);
      if (countdownRef.current <= 0) {
        const p = fetchParamsRef.current;
        fetchData(p.currentPage, p.pageSize, p.sortColumn, p.sortDirection);
        countdownRef.current = REFRESH_SECONDS;
        setCountdown(REFRESH_SECONDS);
      }
    }, 1000);
    return () => clearInterval(interval);
  }, [fetchData]);

  const handleSort = useCallback((column: SortColumn) => {
    if (sortColumnRef.current === column) {
      setSortDirection(d => d === 'asc' ? 'desc' : 'asc');
    } else {
      setSortColumn(column);
      setSortDirection(column === 'tier' ? 'asc' : 'desc');
    }
    setCurrentPage(1);
  }, []);

  const handlePageSizeChange = useCallback((newSize: PageSize) => {
    setPageSize(newSize);
    setCurrentPage(1);
  }, []);

  const handleCopyAddress = useCallback(async (address: string) => {
    const ok = await writeToClipboard(address);
    if (!ok) return;
    if (copyTimerRef.current) clearTimeout(copyTimerRef.current);
    setCopiedAddress(address);
    copyTimerRef.current = setTimeout(() => {
      if (mountedRef.current) setCopiedAddress(null);
      copyTimerRef.current = null;
    }, 2000);
  }, []);

  const handleRefresh = useCallback(() => {
    const p = fetchParamsRef.current;
    fetchData(p.currentPage, p.pageSize, p.sortColumn, p.sortDirection);
    countdownRef.current = REFRESH_SECONDS;
    setCountdown(REFRESH_SECONDS);
  }, [fetchData]);

  // Retry the last fetch using current params. Also resets the auto-refresh
  // countdown so the user is not immediately polled again after retrying.
  const handleRetry = useCallback(() => {
    handleRefresh();
  }, [handleRefresh]);

  const handleDismissError = useCallback(() => {
    setFetchError(null);
  }, []);

  // Loading skeleton — only shown while actively fetching with no prior data
  // and no error. If an error occurred on the first fetch, skip the skeleton
  // and render the error banner instead so the user knows what happened.
  if (isFetching && !stats && !fetchError) {
    return (
      <div style={{ padding: '16px', color: '#888', fontSize: '12px' }}>
        {t('paymentStats.loading')}
      </div>
    );
  }

  // First-load error: show the banner standalone so the user can see what
  // failed and retry without being told "No data available" (which would be
  // misleading when the real problem is a failed RPC call).
  if (!stats && fetchError) {
    return (
      <div style={{ padding: '16px' }}>
        <Banner
          variant="warning"
          message={`${t('paymentStats.fetchError')} — ${fetchError}`}
          role="alert"
        >
          <div style={{ display: 'flex', justifyContent: 'center', gap: '8px', marginTop: '6px' }}>
            <PillButton
              onClick={handleRetry}
              title={t('paymentStats.retry')}
              ariaLabel={t('paymentStats.retry')}
              icon={<RotateCw size={11} />}
              label={t('paymentStats.retry')}
              disabled={isFetching}
            />
            <IconButton
              onClick={handleDismissError}
              title={t('paymentStats.dismiss')}
              ariaLabel={t('paymentStats.dismiss')}
              icon={<X size={14} />}
              variant="danger"
              size={24}
            />
          </div>
        </Banner>
      </div>
    );
  }

  // No data (genuinely empty database, no error)
  if (!stats || !stats.entries?.length) {
    return (
      <div style={{ padding: '16px', color: '#888', fontSize: '12px' }}>
        {t('paymentStats.noData')}
      </div>
    );
  }

  const totalPages = stats.totalPages || 1;
  const safePage = stats.currentPage || 1;
  const totalEntries = stats.totalEntries || 0;
  const rangeStart = totalEntries > 0 ? (safePage - 1) * pageSize + 1 : 0;
  const rangeEnd = Math.min(safePage * pageSize, totalEntries);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', flex: 1, minHeight: 0 }}>
      {/* Summary Cards */}
      <div style={{
        display: 'grid',
        gridTemplateColumns: 'repeat(4, 1fr)',
        gap: '8px',
        marginBottom: '12px',
      }}>
        <DashboardCard title={t('paymentStats.summary.totalPaid')} headerLeft={<Coins size={14} color="#27ae60" />} headerRight={unitLabel}>
          <div style={{ fontSize: '18px', color: '#27ae60', fontWeight: 600, fontFamily: 'monospace', textAlign: 'center' }}>
            {formatAmount(stats.totalPaid, false)}
          </div>
        </DashboardCard>
        <DashboardCard title={t('paymentStats.summary.totalPayments')} headerLeft={<Receipt size={14} color="#6699cc" />}>
          <div style={{ fontSize: '18px', color: '#ddd', fontWeight: 600, fontFamily: 'monospace', textAlign: 'center' }}>
            {formatNumber(stats.totalPayments)}
          </div>
        </DashboardCard>
        <DashboardCard title={t('paymentStats.summary.uniquePaymentAddresses')} headerLeft={<Users size={14} color="#bb88dd" />}>
          <div style={{ fontSize: '18px', color: '#ddd', fontWeight: 600, fontFamily: 'monospace', textAlign: 'center' }}>
            {formatNumber(stats.uniquePaymentAddresses)}
          </div>
        </DashboardCard>
        <DashboardCard title={t('paymentStats.summary.scannedBlocks')} headerLeft={<Layers size={14} color="#ff9966" />}>
          <div style={{ fontSize: '16px', color: '#ddd', fontWeight: 600, fontFamily: 'monospace', display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '6px' }}>
            <span>{formatNumber(stats.lowestBlock)}</span>
            <ArrowRight size={12} color="#888" />
            <span>{formatNumber(stats.highestBlock)}</span>
          </div>
        </DashboardCard>
      </div>

      {/* Fetch error banner — shown above the refresh countdown when a fetch
          fails while stale data is still on screen. Does NOT clear stats. */}
      {fetchError && (
        <Banner
          variant="warning"
          message={`${t('paymentStats.fetchError')} — ${fetchError}`}
          role="alert"
        >
          <div style={{ display: 'flex', justifyContent: 'center', gap: '8px', marginTop: '6px' }}>
            <PillButton
              onClick={handleRetry}
              title={t('paymentStats.retry')}
              ariaLabel={t('paymentStats.retry')}
              icon={<RotateCw size={11} />}
              label={t('paymentStats.retry')}
              disabled={isFetching}
            />
            <IconButton
              onClick={handleDismissError}
              title={t('paymentStats.dismiss')}
              ariaLabel={t('paymentStats.dismiss')}
              icon={<X size={14} />}
              variant="danger"
              size={24}
            />
          </div>
        </Banner>
      )}

      {/* Sticky-header + flex row-card list (canonical Receive pattern, mirrors
          MasternodesTable.tsx:377-510). Replaces the prior HTML table markup
          (thead + tbody + tr + th + td). Outer container provides the card
          chrome; inner sticky div hosts header cells; scroll body renders each
          entry as a flex row card (no alternating zebra).
          See m-restyle-payment-stats-tab-to-receive-design (2026-06-12). */}
      <div style={{
        flex: 1,
        overflow: 'hidden',
        minHeight: 0,
        border: '1px solid #3a3a3a',
        borderRadius: '8px',
        backgroundColor: '#2f2f2f',
        opacity: isFetching ? 0.7 : 1,
        transition: 'opacity 0.15s',
        display: 'flex',
        flexDirection: 'column',
      }}>
        {/* Sticky column header — opaque #2f2f2f bg covers scrolling rows.
            Column order: Address | Tier | Payments | Last Paid | Latest TX (flex:1) | Total Paid | Refresh.
            gap: '8px' (was 12px, m-payment-stats-table-restructure 2026-06-12) tightens the visible
            whitespace between Tier↔Payments narrow cells. Lockstep with the PaymentRow card below
            so column alignment between header and rows stays 1:1. */}
        <div style={{
          display: 'flex',
          gap: '8px',
          alignItems: 'center',
          padding: '10px 20px',
          borderBottom: '1px solid #3a3a3a',
          position: 'sticky',
          top: 0,
          zIndex: 10,
          backgroundColor: '#2f2f2f',
          flexShrink: 0,
        }}>
          <HeaderCell
            column={null}
            label={t('paymentStats.table.address')}
            width={COL.address}
            sortColumn={sortColumn}
            sortDirection={sortDirection}
          />
          <HeaderCell
            column="tier"
            label={t('paymentStats.table.tier')}
            width={COL.tier}
            sortColumn={sortColumn}
            sortDirection={sortDirection}
            onSort={handleSort}
          />
          <HeaderCell
            column="paymentCount"
            label={t('paymentStats.table.paymentCount')}
            width={COL.paymentCount}
            align="right"
            sortColumn={sortColumn}
            sortDirection={sortDirection}
            onSort={handleSort}
          />
          {/* 8px spacer between Payments and Last Paid (m-payment-stats-table-cleanup, 2026-06-12).
              Combined with the parent flex gap of 8px, this gives ~16px visible separation so the
              right-aligned numeric Payments value doesn't visually merge with the Last Payment string.
              Lockstep with the matching spacer in PaymentRow below to keep column geometry 1:1. */}
          <div style={{ width: '8px', flexShrink: 0 }} />
          <HeaderCell
            column="lastPaidTime"
            label={
              <>
                {t('paymentStats.table.lastPaidTime')}
                {formatTzSuffix()}
              </>
            }
            width={COL.lastPaidTime}
            align="right"
            sortColumn={sortColumn}
            sortDirection={sortDirection}
            onSort={handleSort}
          />
          {/* Empty flex:1 spacer replaces the dropped Latest TX cell — absorbs slack between
              Last Paid and Total Paid so Total Paid + Refresh stay anchored at the right edge. */}
          <div style={{ flex: 1 }} />
          <HeaderCell
            column="totalPaid"
            label={t('paymentStats.table.totalPaid', { unit: unitLabel })}
            width={COL.totalPaid}
            align="right"
            sortColumn={sortColumn}
            sortDirection={sortDirection}
            onSort={handleSort}
          />
          {/* Trailing refresh actions cell — hosts <RefreshCountdown size={26}> centered in 32px column.
              Row cards render an empty 32px placeholder in this slot for column alignment. */}
          <div style={{ width: COL.refresh, flexShrink: 0, display: 'flex', justifyContent: 'center' }}>
            <RefreshCountdown
              countdown={countdown}
              total={REFRESH_SECONDS}
              mode="interactive"
              onRefresh={handleRefresh}
              isLoading={isFetching}
              size={26}
            />
          </div>
        </div>
        {/* Scroll body — flex: 1 fills remaining height; overflow-y triggers on tall lists. */}
        <div style={{ flex: 1, overflowY: 'auto' }}>
          <div style={{
            display: 'flex',
            flexDirection: 'column',
            gap: '4px',
            padding: '8px 8px 12px 8px',
          }}>
            {stats.entries.map((entry) => (
              <PaymentRow
                key={entry.address}
                entry={entry}
                copiedAddress={copiedAddress}
                onCopyAddress={handleCopyAddress}
              />
            ))}
          </div>
        </div>
      </div>

      {/* Pagination */}
      <PaginationFooter
        rangeStart={rangeStart}
        rangeEnd={rangeEnd}
        total={totalEntries}
        currentPage={safePage}
        totalPages={totalPages}
        onPageChange={setCurrentPage}
        pageSize={pageSize}
        pageSizeOptions={PAGE_SIZES}
        onPageSizeChange={handlePageSizeChange}
        isLoading={isFetching}
      />
    </div>
  );
});

PaymentStatsTab.displayName = 'PaymentStatsTab';

// --- Sub-components ---

interface PaymentRowProps {
  entry: PaymentStatsEntry;
  copiedAddress: string | null;
  onCopyAddress: (address: string) => void;
}

// Flex row card matching the canonical Receive design language (mirrors
// MasternodesTable.tsx row card chrome). Hover via inline onMouseEnter/
// onMouseLeave flipping borderColor transparent → #444 (zero-re-render
// convention). isEven zebra striping dropped — every row uses #2a2a2a.
// Total Paid rendered in TWINS-green #27ae60 (14px 600 monospace) matching
// the Transactions Amount column precedent. Copy IconButton on the Address
// cell uses the shared primitive with 2s green Check feedback gated on
// copiedAddress === entry.address.
const PaymentRow: React.FC<PaymentRowProps> = React.memo(({ entry, copiedAddress, onCopyAddress }) => {
  const { t } = useTranslation('common');
  const { formatAmount } = useDisplayUnits();
  const { formatDateTimeShort, formatTooltip } = useDisplayDateTime();
  const tierColor = TIER_COLORS[entry.tier] || '#999';
  const tierLabel = entry.tier ? entry.tier.charAt(0).toUpperCase() + entry.tier.slice(1) : '';
  const isAddressCopied = copiedAddress === entry.address;

  return (
    <div
      onMouseEnter={(e) => {
        e.currentTarget.style.borderColor = '#444';
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.borderColor = 'transparent';
      }}
      style={{
        // Column order: Address (with inline Copy IconButton) | Tier | Payments | <8px spacer> | Last Paid | <flex:1 spacer> | Total Paid | refresh-placeholder.
        // gap: '8px' — lockstep with the sticky header above so column alignment stays 1:1.
        display: 'flex',
        gap: '8px',
        alignItems: 'center',
        padding: '10px 12px',
        backgroundColor: '#2a2a2a',
        border: '1px solid transparent',
        borderRadius: '6px',
        cursor: 'default',
        transition: 'border-color 0.15s',
      }}
    >
      {/* Address cell — flex row containing the full address + inline Copy IconButton.
          Outer width pinned to COL.address (290px) so column alignment with sticky header stays 1:1.
          The inner address span renders the full 34-character TWINS address without ellipsis truncation —
          290px column - 24px IconButton - 6px gap = ~260px inner div, comfortably fitting ~245px of
          12px monospace address text. `title` tooltip preserved as defense-in-depth.
          See m-payment-stats-tier-empty-lastpaid-align-address-full (2026-06-12). */}
      <div
        style={{
          width: COL.address,
          display: 'flex',
          alignItems: 'center',
          gap: '6px',
        }}
      >
        <div
          style={{
            flex: 1,
            minWidth: 0,
            fontFamily: 'monospace',
            fontSize: '12px',
            color: '#ddd',
          }}
          title={entry.address}
        >
          {entry.address}
        </div>
        <IconButton
          onClick={() => onCopyAddress(entry.address)}
          title={t('explorer.copyAddress')}
          ariaLabel={t('explorer.copyAddress')}
          icon={isAddressCopied ? <Check size={12} color="#27ae60" /> : <Copy size={12} />}
          size={24}
        />
      </div>
      {/* Tier — renders empty when entry.tier is unset (payment address not a known masternode).
          Em-dash fallback dropped in m-payment-stats-tier-empty-lastpaid-align-address-full (2026-06-12)
          so empty cells stay visually quiet and don't compete with real Platinum/Gold/Silver/Bronze labels. */}
      <div
        style={{
          width: COL.tier,
          color: tierColor,
          fontWeight: entry.tier ? 'bold' : 'normal',
          fontSize: '12px',
        }}
      >
        {tierLabel || ''}
      </div>
      {/* Payments */}
      <div
        style={{
          width: COL.paymentCount,
          textAlign: 'right',
          fontSize: '12px',
          color: '#ddd',
        }}
      >
        {entry.paymentCount.toLocaleString()}
      </div>
      {/* 8px spacer between Payments and Last Paid — lockstep with sticky header (see COL doc above). */}
      <div style={{ width: '8px', flexShrink: 0 }} />
      {/* Last Paid — right-aligned per m-payment-stats-tier-empty-lastpaid-align-address-full (2026-06-12)
          so the relative-time string sits flush against the next column boundary, matching the right-aligned
          Payments column convention to its left. */}
      <div
        style={{
          width: COL.lastPaidTime,
          textAlign: 'right',
          fontSize: '12px',
          color: '#ddd',
        }}
        title={formatTooltip(entry.lastPaidTime)}
      >
        {formatDateTimeShort(entry.lastPaidTime)}
      </div>
      {/* Empty flex:1 spacer replaces the dropped Latest TX cell — absorbs slack so Total Paid
          and Refresh stay anchored at the right edge of the row. Lockstep with sticky header. */}
      <div style={{ flex: 1 }} />
      {/* Total Paid — TWINS-green monetary token (matches Transactions Amount), rightmost data column. */}
      <div
        style={{
          width: COL.totalPaid,
          textAlign: 'right',
          fontFamily: 'monospace',
          fontSize: '14px',
          fontWeight: 600,
          color: '#27ae60',
        }}
      >
        {formatAmount(entry.totalPaid, false)}
      </div>
      {/* Trailing refresh-actions placeholder — empty 32px cell mirroring the sticky header's
          refresh-ring column so row geometry aligns 1:1. */}
      <div style={{ width: COL.refresh, flexShrink: 0 }} />
    </div>
  );
});

PaymentRow.displayName = 'PaymentRow';
