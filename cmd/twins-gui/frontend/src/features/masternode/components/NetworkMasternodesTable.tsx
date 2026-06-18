import React, { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ChevronDown, ChevronUp } from 'lucide-react';
import { NetworkMasternode, NetworkMasternodeFilters } from '@/shared/types/masternode.types';
import { useDisplayDateTime } from '@/shared/hooks/useDisplayDateTime';
import { RefreshCountdown } from '@/shared/components/RefreshCountdown';
import { formatActiveTime, formatActiveSinceUTC, formatTimeAgo, formatDateUTC, getStatusColor } from './MasternodesTable';

// Column widths shared between the sticky header and the row cards.
// paymentaddress column uses flex: 1 (not in COL).
// Per the canonical Receive row-card pattern (Transactions.tsx / Receive.tsx),
// COL is referenced inline at each header cell + row cell to keep widths in sync.
// PR 11 (m-masternode-tables-row-card-conversion, 2026-06-10) — see CLAUDE.md.
// Network + Protocol columns dropped, remaining widths shrunk in
// m-network-masternodes-table-narrow-fit (2026-06-12) so the 6-column table
// fits in narrow GUI windows (~720px minimum content width vs prior 1000px).
//
// `actions` (40px) added by m-network-masternodes-table-style-parity (2026-06-12):
// reserved trailing slot for the in-header RefreshCountdown ring, mirrors the
// MasternodesTable.tsx Actions cell pattern (96px there because of the per-row
// Play/Pencil/Trash IconButtons). Network table is read-only — no per-row
// action buttons — so 40px is just enough to host the 26px ring centered.
// Row cards render a matching placeholder div with the same width so header
// and rows stay column-aligned.
const COL = {
  rank: '60px',
  status: '80px',
  tier: '70px',
  activetime: '85px',
  lastseen: '120px',
  lastpaid: '120px',
  // `actions: '28px'` — tightened from 40px → 30px (m-network-masternodes-table-style-parity
  // post-merge round 2) → 28px (l-network-masternodes-cosmetic-polish, 2026-06-12) after
  // user re-tested the live build and still perceived a visible empty vertical band on
  // every row's right edge. Network table is read-only — row placeholders contain
  // nothing. Ring `size={26}` + 1px breathing room each side = 28px floor.
  actions: '28px',
} as const;

export type NetworkSortColumn =
  | 'rank'
  | 'status'
  | 'tier'
  | 'paymentaddress'
  | 'activetime'
  | 'lastseen'
  | 'lastpaid';

// Module-level sortable header cell (canonical Receive convention) mirroring
// MasternodesTable.tsx:217-277. Promoted out of the component body so the
// local useState(hovered) survives the 1/sec countdown re-render that the
// in-header RefreshCountdown ring triggers — inlining the cell as a closure
// would create a fresh function reference per render, React would unmount and
// remount the cell instance, and the hover state would flicker.
// HeaderCell receives sortColumn/sortDirection/onSort as props (not closure
// capture) so it's a pure, stable, module-level component.
interface HeaderCellProps {
  column: NetworkSortColumn;
  label: React.ReactNode;
  width?: string;
  flex?: number;
  align?: 'left' | 'right' | 'center';
  // sortColumn matches NetworkMasternodeFilters.sortColumn shape (wider union
  // than NetworkSortColumn — covers every NetworkMasternode field + '' sentinel
  // for "no active sort"). The `sortColumn === column` comparison narrows
  // automatically at runtime because `column` is always a NetworkSortColumn.
  sortColumn: keyof NetworkMasternode | '';
  sortDirection: 'asc' | 'desc';
  onSort: (column: NetworkSortColumn) => void;
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
  const isActive = sortColumn === column;
  const [hovered, setHovered] = useState(false);
  const labelColor = isActive ? '#27ae60' : hovered ? '#ddd' : '#888';
  const labelWeight = isActive ? 600 : 500;
  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => onSort(column)}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onSort(column);
        }
      }}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      aria-sort={isActive ? (sortDirection === 'asc' ? 'ascending' : 'descending') : 'none'}
      style={{
        width,
        flex,
        minWidth: 0,
        cursor: 'pointer',
        userSelect: 'none',
        display: 'flex',
        alignItems: 'center',
        justifyContent: align === 'right' ? 'flex-end' : align === 'center' ? 'center' : 'flex-start',
        gap: '4px',
        whiteSpace: 'nowrap',
      }}
    >
      <span style={{ fontSize: '11px', fontWeight: labelWeight, color: labelColor }}>{label}</span>
      {isActive &&
        (sortDirection === 'asc' ? (
          <ChevronUp size={16} color="#27ae60" />
        ) : (
          <ChevronDown size={16} color="#27ae60" />
        ))}
    </div>
  );
};

// Format tier name with capitalization
const formatTierName = (tier: string): string => {
  if (!tier) return 'Unknown';
  return tier.charAt(0).toUpperCase() + tier.slice(1).toLowerCase();
};

// Validate network masternode has required fields for rendering
const isValidNetworkMasternode = (mn: NetworkMasternode): boolean => {
  return !!(
    mn &&
    typeof mn.rank === 'number' &&
    typeof mn.addr === 'string' &&
    typeof mn.status === 'string'
  );
};

export interface NetworkMasternodesTableProps {
  masternodes: NetworkMasternode[];
  isLoading: boolean;
  hasLoaded: boolean; // True after first successful data load
  filters: NetworkMasternodeFilters;
  onSort: (column: NetworkSortColumn) => void;
  // Added by m-network-masternodes-table-style-parity (2026-06-12):
  // in-header RefreshCountdown ring, relocated from NetworkMasternodesFilters
  // so the ring lives next to the data it refreshes (canonical
  // MasternodesTable.tsx pattern).
  countdown: number;
  countdownTotal: number;
  onRefresh: () => void;
  isRefreshing: boolean;
}

export const NetworkMasternodesTable: React.FC<NetworkMasternodesTableProps> = React.memo(({
  masternodes,
  isLoading,
  hasLoaded,
  filters,
  onSort,
  countdown,
  countdownTotal,
  onRefresh,
  isRefreshing,
}) => {
  const { t } = useTranslation('masternode');
  const { formatDateTimeShort, formatTooltip, formatTzSuffix } = useDisplayDateTime();
  void formatTimeAgo;
  void formatDateUTC;

  // Sort masternodes based on current sort state, filtering out invalid entries
  const sortedMasternodes = useMemo(() => {
    // Filter out invalid masternodes first
    const validMasternodes = masternodes.filter(isValidNetworkMasternode);

    const sorted = [...validMasternodes].sort((a, b) => {
      let aValue: string | number;
      let bValue: string | number;

      const sortColumn = filters.sortColumn as NetworkSortColumn;

      switch (sortColumn) {
        case 'rank':
          aValue = a.rank;
          bValue = b.rank;
          break;
        case 'status':
          aValue = a.status;
          bValue = b.status;
          break;
        case 'tier':
          aValue = a.tier.toLowerCase();
          bValue = b.tier.toLowerCase();
          break;
        case 'paymentaddress':
          aValue = a.paymentaddress.toLowerCase();
          bValue = b.paymentaddress.toLowerCase();
          break;
        case 'activetime':
          aValue = a.activetime;
          bValue = b.activetime;
          break;
        case 'lastseen':
          aValue = a.lastseen ? new Date(a.lastseen).getTime() || 0 : 0;
          bValue = b.lastseen ? new Date(b.lastseen).getTime() || 0 : 0;
          break;
        case 'lastpaid':
          aValue = a.lastpaid ? new Date(a.lastpaid).getTime() || 0 : 0;
          bValue = b.lastpaid ? new Date(b.lastpaid).getTime() || 0 : 0;
          break;
        default:
          return 0;
      }

      if (aValue < bValue) return filters.sortDirection === 'asc' ? -1 : 1;
      if (aValue > bValue) return filters.sortDirection === 'asc' ? 1 : -1;
      return 0;
    });

    return sorted;
  }, [masternodes, filters.sortColumn, filters.sortDirection]);

  return (
    <div
      style={{
        flex: 1,
        overflow: 'auto',
        border: '1px solid #3a3a3a',
        borderRadius: '8px',
        backgroundColor: '#2f2f2f',
        // Minimum width to ensure 6 columns fit on narrow windows
        // (~720px floor; m-network-masternodes-table-narrow-fit 2026-06-12).
        minWidth: '720px',
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      {/* Sticky column header — canonical Receive pattern (Transactions.tsx).
          Sits above the scroll body; opaque #2f2f2f bg covers scrolling rows. */}
      <div
        style={{
          display: 'flex',
          gap: '12px',
          alignItems: 'center',
          padding: '10px 20px',
          borderBottom: '1px solid #3a3a3a',
          position: 'sticky',
          top: 0,
          zIndex: 10,
          backgroundColor: '#2f2f2f',
          flexShrink: 0,
        }}
      >
        <HeaderCell
          column="rank"
          label={t('table.payRank')}
          width={COL.rank}
          sortColumn={filters.sortColumn}
          sortDirection={filters.sortDirection}
          onSort={onSort}
        />
        <HeaderCell
          column="status"
          label={t('table.status')}
          width={COL.status}
          sortColumn={filters.sortColumn}
          sortDirection={filters.sortDirection}
          onSort={onSort}
        />
        <HeaderCell
          column="tier"
          label={t('table.tier')}
          width={COL.tier}
          sortColumn={filters.sortColumn}
          sortDirection={filters.sortDirection}
          onSort={onSort}
        />
        <HeaderCell
          column="paymentaddress"
          label={t('table.walletAddress')}
          flex={1}
          sortColumn={filters.sortColumn}
          sortDirection={filters.sortDirection}
          onSort={onSort}
        />
        <HeaderCell
          column="activetime"
          label={t('table.activeTime')}
          width={COL.activetime}
          align="right"
          sortColumn={filters.sortColumn}
          sortDirection={filters.sortDirection}
          onSort={onSort}
        />
        <HeaderCell
          column="lastseen"
          label={
            <>
              {t('table.lastSeen')}
              {formatTzSuffix()}
            </>
          }
          width={COL.lastseen}
          align="right"
          sortColumn={filters.sortColumn}
          sortDirection={filters.sortDirection}
          onSort={onSort}
        />
        <HeaderCell
          column="lastpaid"
          label={
            <>
              {t('table.lastPaid')}
              {formatTzSuffix()}
            </>
          }
          width={COL.lastpaid}
          align="right"
          sortColumn={filters.sortColumn}
          sortDirection={filters.sortDirection}
          onSort={onSort}
        />
        {/* Actions cell — hosts the interactive RefreshCountdown ring.
            Mirrors the canonical MasternodesTable.tsx:434-451 Actions cell
            (96px there because of per-row Play/Pencil/Trash IconButtons; this
            Network table is read-only so 40px is just enough for the 26px
            ring centered). Added by m-network-masternodes-table-style-parity
            (2026-06-12); the previous standalone RefreshCountdown lived
            above the table in NetworkMasternodesFilters. */}
        <div
          style={{
            width: COL.actions,
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            flexShrink: 0,
          }}
        >
          <RefreshCountdown
            mode="interactive"
            size={26}
            countdown={countdown}
            total={countdownTotal}
            onRefresh={onRefresh}
            isLoading={isRefreshing}
          />
        </div>
      </div>

      {/* Scroll body — flex: 1 fills remaining height; overflow-y triggers on tall lists. */}
      <div style={{ flex: 1, overflowY: 'auto' }}>
        {/* Only show loading on first load — prevents blink during refresh. */}
        {isLoading && !hasLoaded ? (
          <div style={{ padding: '32px', textAlign: 'center', color: '#888', fontSize: '12px' }}>
            {t('common:loading.masternodes')}
          </div>
        ) : sortedMasternodes.length === 0 ? (
          <div style={{ padding: '32px', textAlign: 'center', color: '#888', fontSize: '12px' }}>
            {t('network.noResults')}
          </div>
        ) : (
          <div
            style={{
              display: 'flex',
              flexDirection: 'column',
              gap: '4px',
              padding: '8px 8px 12px 8px',
            }}
          >
            {sortedMasternodes.map((mn) => {
              const rowKey = `${mn.txhash}:${mn.outidx}`;
              return (
                <div
                  key={rowKey}
                  data-mn-key={rowKey}
                  onMouseEnter={(e) => {
                    e.currentTarget.style.borderColor = '#444';
                  }}
                  onMouseLeave={(e) => {
                    e.currentTarget.style.borderColor = 'transparent';
                  }}
                  style={{
                    display: 'flex',
                    gap: '12px',
                    alignItems: 'center',
                    padding: '10px 12px',
                    // Canonical Receive row-card chrome. No selection state on this
                    // table — read-only network view (NetworkMasternodesTableProps
                    // has no selectedMasternode prop).
                    backgroundColor: '#2a2a2a',
                    border: '1px solid transparent',
                    borderRadius: '6px',
                    transition: 'border-color 0.15s',
                  }}
                >
                  <div
                    style={{
                      width: COL.rank,
                      fontSize: '12px',
                      color: '#ddd',
                    }}
                  >
                    {mn.rank}
                  </div>
                  <div
                    style={{
                      width: COL.status,
                      fontSize: '12px',
                      // getStatusColor palette preserved verbatim (domain semantics).
                      color: getStatusColor(mn.status as any),
                      fontWeight: 'bold',
                    }}
                  >
                    {mn.status}
                  </div>
                  <div
                    style={{
                      width: COL.tier,
                      fontSize: '12px',
                      color: '#ddd',
                      fontWeight: 500,
                    }}
                  >
                    {formatTierName(mn.tier)}
                  </div>
                  <div
                    style={{
                      flex: 1,
                      minWidth: 0,
                      fontFamily: 'monospace',
                      fontSize: '10px',
                      color: '#ddd',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                    }}
                  >
                    {mn.paymentaddress}
                  </div>
                  <div
                    title={formatActiveSinceUTC(mn.activetime)}
                    style={{ width: COL.activetime, fontSize: '12px', color: '#ddd', textAlign: 'right', fontFamily: 'monospace' }}
                  >
                    {formatActiveTime(mn.activetime)}
                  </div>
                  <div
                    title={formatTooltip(mn.lastseen)}
                    style={{ width: COL.lastseen, fontSize: '12px', color: '#ddd', textAlign: 'right', fontFamily: 'monospace' }}
                  >
                    {formatDateTimeShort(mn.lastseen)}
                  </div>
                  <div
                    title={formatTooltip(mn.lastpaid)}
                    style={{ width: COL.lastpaid, fontSize: '12px', color: '#ddd', textAlign: 'right', fontFamily: 'monospace' }}
                  >
                    {formatDateTimeShort(mn.lastpaid)}
                  </div>
                  {/* Trailing placeholder mirrors the sticky-header Actions
                      cell so row geometry stays column-aligned. Network table
                      is read-only — no per-row buttons go here. Added by
                      m-network-masternodes-table-style-parity (2026-06-12). */}
                  <div style={{ width: COL.actions, flexShrink: 0 }} />
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
});
