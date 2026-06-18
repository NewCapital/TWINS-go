import React, { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ChevronDown, ChevronUp, Pencil, Play, Trash2 } from 'lucide-react';
import { Masternode, MasternodeStatus } from '@/shared/types/masternode.types';
import { useDisplayDateTime } from '@/shared/hooks/useDisplayDateTime';
import { IconButton } from '@/shared/components/IconButton';
import { RefreshCountdown } from '@/shared/components/RefreshCountdown';

// Column widths shared between the sticky header and the row cards.
// collateralAddress column uses flex: 1 (not in COL) with minWidth on the cell
// to prevent ellipsis collapse-to-zero next to the Actions column.
// Widths originally derived from Qt masternodelist.cpp:36-49.
// Per the canonical Receive row-card pattern (Transactions.tsx / Receive.tsx),
// COL is referenced inline at each header cell + row cell to keep widths in sync.
//
// Column order updated by m-masternodes-table-reorder-and-actions (2026-06-11):
// Alias → Status → Last Seen → Active Time → Protocol → Address → Collateral → Actions.
// Rationale: identity (Alias) → operational state (Status) → uptime
// (Last Seen, Active Time) → technical metadata (Protocol, Address) →
// long collateral address → per-row Pencil edit + in-header RefreshCountdown ring.
//
// Widths tuned on 2026-06-11 (same task, post live-Wails feedback): shrank
// lastSeen/active/protocol/address to reclaim ~140px of horizontal space for
// the Collateral Address column so it does not visually collapse next to the
// Pencil. Each cell still fits its longest realistic content at 12px monospace:
//   lastSeen 140px: "2026-06-11 12:30" + header "Last Seen (GMT+2)" tz suffix
//   active 80px:    "365d 22h 33m" ~70px monospace
//   protocol 50px:  "70928"        ~40px monospace
//   address 140px:  "203.0.113.99:37817" ~118px monospace + ellipsis safety
//
// lastSeen widened 90→140px AND actions widened 50→64px by
// m-masternodes-table-ux-cleanup (2026-06-11) — lastSeen fits "Last Seen (GMT+2)"
// header label without bleeding into next column; actions fits Play+Pencil
// IconButtons (26+4+26 = 56px + breathing room) replacing the prior single
// Pencil. The ~64px reclaimed from collateralAddress (flex:1) stays well above
// the 80px minWidth floor on that cell.
//
// lastSeen tightened 140→110px by m-masternodes-table-ux-refinements (2026-06-11)
// after live testing showed the visible date `MM-DD HH:MM` (e.g. `06-11 16:44` at
// 12px monospace ≈ 70px content) fits comfortably in 110px with the chevron icon
// (14px) + gap (8px) + breathing room. Header label `Last Seen (GMT+2)` at 11px
// 500 also fits in 110px. The freed 30px is reclaimed by collateralAddress
// flex:1 (still well above its 80px minWidth floor).
//
// actions widened 64→96px by m-masternodes-actions-restructure (2026-06-11) to
// fit a 3rd Trash IconButton: 3×26 + 2×4 = 86px content + 10px breathing room
// = 96px. The new Trash button delegates per-row delete (replacing the deleted
// Delete button in the now-retired MasternodeConfigDialog list view). The 32px
// reclaimed comes again from collateralAddress flex:1, still above the 80px
// minWidth floor.
const COL = {
  alias: '100px',
  status: '80px',
  lastSeen: '110px',
  active: '80px',
  protocol: '50px',
  address: '140px',
  // collateralAddress column uses flex: 1 with minWidth on the cell itself
  actions: '96px',
} as const;

export type SortColumn =
  | 'alias'
  | 'address'
  | 'protocol'
  | 'status'
  | 'active'
  | 'lastSeen'
  | 'collateralAddress';
export type SortDirection = 'asc' | 'desc';

// Format active time (seconds) to human-readable format (e.g., "2d 5h 30m").
// Returns empty string for non-positive input (m-masternodes-table-ux-refinements
// 2026-06-11): MISSING masternodes have activeTime=0 by protocol convention;
// rendering "0s" reads as "running for 0 seconds" instead of "never been active".
// Empty string is the correct signal. Network masternodes (re-export consumer in
// NetworkMasternodesTable.tsx) benefit from the same change.
export const formatActiveTime = (seconds: number): string => {
  if (seconds <= 0) return '';

  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = seconds % 60;

  const parts: string[] = [];
  if (days > 0) parts.push(`${days}d`);
  if (hours > 0) parts.push(`${hours}h`);
  if (minutes > 0) parts.push(`${minutes}m`);
  if (secs > 0 && days === 0) parts.push(`${secs}s`); // Only show seconds if less than a day

  return parts.join(' ') || '';
};

// Parse and validate a date input, returning a valid Date or null
const parseDate = (date: Date | string | number | null | undefined): Date | null => {
  if (date === null || date === undefined) return null;
  if (typeof date === 'string' && date.trim() === '') return null;
  if (typeof date === 'number' && (isNaN(date) || !isFinite(date))) return null;

  const d = date instanceof Date ? date : new Date(date);
  if (!d || isNaN(d.getTime())) return null;

  // Detect zero/epoch dates: Go zero time (year 1) or Unix epoch (year 1970)
  if (d.getUTCFullYear() <= 1970) return null;

  return d;
};

// Format date as relative time (e.g., "5 minutes ago")
export const formatTimeAgo = (date: Date | string | number | null | undefined): string => {
  const d = parseDate(date);
  if (!d) return 'N/A';

  const now = Date.now();
  const diffSec = Math.floor((now - d.getTime()) / 1000);

  if (diffSec < 0) return 'just now';
  if (diffSec < 60) return 'just now';
  if (diffSec < 3600) {
    const mins = Math.floor(diffSec / 60);
    return `${mins} minute${mins !== 1 ? 's' : ''} ago`;
  }
  if (diffSec < 86400) {
    const hours = Math.floor(diffSec / 3600);
    return `${hours} hour${hours !== 1 ? 's' : ''} ago`;
  }
  if (diffSec < 2592000) { // 30 days
    const days = Math.floor(diffSec / 86400);
    return `${days} day${days !== 1 ? 's' : ''} ago`;
  }
  const months = Math.floor(diffSec / 2592000);
  return `${months} month${months !== 1 ? 's' : ''} ago`;
};

// Format date as UTC string for tooltip display
export const formatDateUTC = (date: Date | string | number | null | undefined): string => {
  const d = parseDate(date);
  if (!d) return '';
  return d.toISOString().replace('T', ' ').slice(0, 19) + ' UTC';
};

// Format active time (seconds) as "YYYY-MM-DD HH:MM:SS UTC" for tooltip
export const formatActiveSinceUTC = (seconds: number): string => {
  if (seconds <= 0) return '';
  const activeSince = new Date(Date.now() - seconds * 1000);
  return activeSince.toISOString().replace('T', ' ').slice(0, 19) + ' UTC';
};

// Format last seen/paid date as relative time (backward compatible alias)
export const formatLastSeen = formatTimeAgo;

// Get status color based on masternode status
export const getStatusColor = (status: MasternodeStatus): string => {
  switch (status) {
    case 'ENABLED':
      return '#00ff00'; // Green
    case 'PRE_ENABLED':
      return '#ffff00'; // Yellow
    case 'MISSING':
    case 'EXPIRED':
    case 'VIN_SPENT':
    case 'REMOVE':
      return '#ff6666'; // Red
    case 'NEW_START_REQUIRED':
    case 'UPDATE_REQUIRED':
      return '#ffaa00'; // Orange
    default:
      return '#999999'; // Gray
  }
};

// Validate masternode has required fields for rendering
const isValidMasternode = (mn: Masternode): boolean => {
  return !!(
    mn &&
    typeof mn.id === 'string' &&
    typeof mn.alias === 'string' &&
    typeof mn.address === 'string' &&
    typeof mn.status === 'string'
  );
};

// Module-level sortable header cell (canonical Transactions.tsx SortableHeader
// pattern at features/wallet/pages/Transactions.tsx:338-409). Promoted out of
// the MasternodesTable function body by m-masternodes-table-ux-cleanup post-
// review fix (2026-06-11): with React.memo intentionally removed (see comment
// above MasternodesTable below), the parent re-renders 1/sec from the
// RefreshCountdown ring's countdown prop, and a child function declared inside
// the parent body gets a NEW reference on every render — React unmounts and
// remounts the child instance, resetting its `useState(hovered)` to false at
// each tick and visibly flickering the hover-flip color (#888 → #ddd). Module-
// level declaration gives HeaderCell a stable function reference so React
// reconciliation preserves the component instance + hook state across parent
// re-renders. Active-column retone (#27ae60 + fontWeight 600) and ChevronUp/
// ChevronDown sort glyph behavior are unchanged.
// `onSort` is OPTIONAL by m-masternodes-table-ux-refinements (2026-06-11).
// When `onSort` is undefined, the cell renders as a static non-interactive
// label: no `role="button"`, no `tabIndex`, no `onClick`/`onKeyDown`, no
// `cursor: pointer`, no hover-color flip, no chevron icon. Used for the
// Protocol, Address, and Collateral Address columns where sorting produces no
// meaningful ordering for the user (long strings / arbitrary IP addresses).
// When `onSort` is provided, behaves exactly as before — clickable sortable
// header with the canonical Receive sort chrome (ChevronUp asc / ChevronDown
// desc, hover #888 → #ddd, active column #27ae60 + fontWeight 600).
interface HeaderCellProps {
  column: SortColumn;
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
  const isSortable = onSort !== undefined;
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
      onClick={isSortable ? () => onSort(column) : undefined}
      onKeyDown={
        isSortable
          ? (e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                onSort(column);
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

export interface MasternodesTableProps {
  masternodes: Masternode[];
  isLoading: boolean;
  sortColumn: SortColumn;
  sortDirection: SortDirection;
  onSort: (column: SortColumn) => void;
  // Added by m-masternodes-table-reorder-and-actions (2026-06-11):
  // countdown ring + per-row Pencil edit affordance.
  countdown: number;
  countdownTotal: number;
  onRefresh: () => void;
  isRefreshing: boolean;
  onEditMasternode: (masternode: Masternode) => void;
  // Added by m-masternodes-table-ux-cleanup (2026-06-11):
  // per-row Play IconButton replaces the prior actions-row Start alias button
  // (which was gated on selectedMasternode — selection is dropped end-to-end
  // alongside the right-click context menu in the same task).
  onStartMasternode: (masternode: Masternode) => void;
  // Added by m-masternodes-actions-restructure (2026-06-11):
  // per-row Trash IconButton replaces the Delete button that lived inside the
  // (now-retired) MasternodeConfigDialog list-view toolbar. Parent wires this
  // to a SimpleConfirmDialog gate before calling DeleteMasternodeConfig.
  onDeleteMasternode: (masternode: Masternode) => void;
}

// React.memo wrapper removed by m-masternodes-table-reorder-and-actions (2026-06-11).
// The wrapper was added 2026-03-17 ("Masternode Table React.memo Fix") to prevent
// countdown-driven re-renders from recalculating formatTimeAgo on rows. With the
// RefreshCountdown ring now rendered IN the sticky table header and countdown
// threaded as a prop, the memo bailout is defeated 1/sec by design — render cost
// for ~20 rows is negligible, and Last Seen updates in sync with the ring read
// as more coherent UX.
export const MasternodesTable: React.FC<MasternodesTableProps> = ({
  masternodes,
  isLoading,
  sortColumn,
  sortDirection,
  onSort,
  countdown,
  countdownTotal,
  onRefresh,
  isRefreshing,
  onEditMasternode,
  onStartMasternode,
  onDeleteMasternode,
}) => {
  const { t } = useTranslation('masternode');
  const { formatDateTimeShort, formatTooltip, formatTzSuffix } = useDisplayDateTime();
  // Sort masternodes based on current sort state, filtering out invalid entries
  const sortedMasternodes = useMemo(() => {
    // Filter out invalid masternodes first
    const validMasternodes = masternodes.filter(isValidMasternode);

    const sorted = [...validMasternodes].sort((a, b) => {
      let aValue: string | number | Date;
      let bValue: string | number | Date;

      switch (sortColumn) {
        case 'alias':
          aValue = a.alias.toLowerCase();
          bValue = b.alias.toLowerCase();
          break;
        case 'address':
          aValue = a.address.toLowerCase();
          bValue = b.address.toLowerCase();
          break;
        case 'protocol':
          aValue = a.protocol;
          bValue = b.protocol;
          break;
        case 'status':
          aValue = a.status;
          bValue = b.status;
          break;
        case 'active':
          aValue = a.activeTime;
          bValue = b.activeTime;
          break;
        case 'lastSeen':
          aValue = new Date(a.lastSeen).getTime();
          bValue = new Date(b.lastSeen).getTime();
          break;
        case 'collateralAddress':
          aValue = a.collateralAddress.toLowerCase();
          bValue = b.collateralAddress.toLowerCase();
          break;
        default:
          return 0;
      }

      if (aValue < bValue) return sortDirection === 'asc' ? -1 : 1;
      if (aValue > bValue) return sortDirection === 'asc' ? 1 : -1;
      return 0;
    });

    return sorted;
  }, [masternodes, sortColumn, sortDirection]);

  return (
    <div
      style={{
        flex: 1,
        overflow: 'auto',
        border: '1px solid #3a3a3a',
        borderRadius: '8px',
        backgroundColor: '#2f2f2f',
        // Minimum width from Qt (695px) to ensure columns fit.
        minWidth: '700px',
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
        <HeaderCell column="alias" label={t('table.alias')} width={COL.alias} sortColumn={sortColumn} sortDirection={sortDirection} onSort={onSort} />
        <HeaderCell column="status" label={t('table.status')} width={COL.status} sortColumn={sortColumn} sortDirection={sortDirection} onSort={onSort} />
        <HeaderCell
          column="lastSeen"
          label={
            <>
              {t('table.lastSeen')}
              {formatTzSuffix()}
            </>
          }
          width={COL.lastSeen}
          sortColumn={sortColumn}
          sortDirection={sortDirection}
          onSort={onSort}
        />
        <HeaderCell column="active" label={t('table.activeTime')} width={COL.active} sortColumn={sortColumn} sortDirection={sortDirection} onSort={onSort} />
        {/* Protocol, Address, Collateral Address headers are non-sortable static
            labels per m-masternodes-table-ux-refinements (2026-06-11) — sorting
            by these columns produces no meaningful ordering for the user.
            Omitting the `onSort` prop is the documented signal for
            HeaderCell to render as a static label. */}
        <HeaderCell column="protocol" label={t('table.protocol')} width={COL.protocol} align="center" sortColumn={sortColumn} sortDirection={sortDirection} />
        <HeaderCell column="address" label={t('table.address')} width={COL.address} sortColumn={sortColumn} sortDirection={sortDirection} />
        <HeaderCell column="collateralAddress" label={t('table.collateralAddress')} flex={1} sortColumn={sortColumn} sortDirection={sortDirection} />
        {/* Actions cell — hosts the interactive RefreshCountdown ring above the
            per-row Pencil edit IconButton. Mirrors the BlockList in-header ring
            precedent from l-explorer-blocks-refresh-ring-in-table-header (2026-06-03). */}
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
        {isLoading ? (
          <div style={{ padding: '32px', textAlign: 'center', color: '#888', fontSize: '12px' }}>
            {t('common:loading.masternodes')}
          </div>
        ) : sortedMasternodes.length === 0 ? (
          <div style={{ padding: '32px', textAlign: 'center', color: '#888', fontSize: '12px' }}>
            {t('noMasternodes')}
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
              return (
                <div
                  key={mn.id}
                  data-mn-key={mn.id}
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
                    // Canonical Receive row-card chrome. Row selection chrome
                    // (green border + bg) was dropped end-to-end by
                    // m-masternodes-table-ux-cleanup (2026-06-11) alongside the
                    // row click handlers and right-click context menu — Play +
                    // Pencil IconButtons in the Actions column are the sole
                    // per-row affordances. Matches the precedent set by
                    // l-remove-tx-table-row-selection (2026-05-20, Transactions)
                    // and m-block-detail-tx-eye-and-stretch (2026-05-26, BlockDetail).
                    backgroundColor: '#2a2a2a',
                    border: '1px solid transparent',
                    borderRadius: '6px',
                    cursor: 'default',
                    transition: 'border-color 0.15s, background-color 0.15s',
                  }}
                >
                  {/* Alias */}
                  <div
                    style={{
                      width: COL.alias,
                      fontSize: '12px',
                      color: '#ddd',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                    }}
                  >
                    {mn.alias}
                  </div>
                  {/* Status — semantic color via getStatusColor (PR 3 domain palette). */}
                  <div
                    style={{
                      width: COL.status,
                      fontSize: '12px',
                      color: getStatusColor(mn.status),
                      fontWeight: 'bold',
                    }}
                  >
                    {mn.status}
                  </div>
                  {/* Last Seen — short relative form; full timestamp in title tooltip. */}
                  <div
                    title={formatTooltip(mn.lastSeen)}
                    style={{ width: COL.lastSeen, fontSize: '12px', color: '#ddd' }}
                  >
                    {formatDateTimeShort(mn.lastSeen)}
                  </div>
                  {/* Active Time. */}
                  <div
                    title={formatActiveSinceUTC(mn.activeTime)}
                    style={{ width: COL.active, fontSize: '12px', color: '#ddd' }}
                  >
                    {formatActiveTime(mn.activeTime)}
                  </div>
                  {/* Protocol. */}
                  <div
                    style={{
                      width: COL.protocol,
                      fontSize: '12px',
                      color: '#ddd',
                      textAlign: 'center',
                    }}
                  >
                    {mn.protocol}
                  </div>
                  {/* Address (IP:port). */}
                  <div
                    style={{
                      width: COL.address,
                      fontSize: '12px',
                      color: '#ddd',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                    }}
                  >
                    {mn.address}
                  </div>
                  {/* Collateral Address (flex stretch). minWidth 80px prevents
                      ellipsis collapse-to-zero next to the Actions column at
                      narrow viewports; rows can overflow horizontally if needed
                      and the parent table has overflow:auto. */}
                  <div
                    title={mn.collateralAddress}
                    style={{
                      flex: 1,
                      minWidth: '80px',
                      fontFamily: 'monospace',
                      fontSize: '10px',
                      color: '#ddd',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                    }}
                  >
                    {mn.collateralAddress}
                  </div>
                  {/* Actions — Play (start_alias) + Pencil (edit) + Trash (delete)
                      IconButtons. Wrapper spans intercept click + keydown bubble
                      — defense-in-depth even though the row no longer has
                      handlers after m-masternodes-table-ux-cleanup (2026-06-11),
                      so that any future reintroduction of row clicks cannot
                      interfere with icon activation. Width widened 64→96px by
                      m-masternodes-actions-restructure (2026-06-11) to fit
                      three buttons (3×26+2×4 = 86px content + 10px breathing
                      room). marginLeft:8 gives extra spacing from the
                      Collateral text. Trash uses variant="danger" (red color)
                      so users can distinguish destructive action at a glance.
                      Play is status-gated by m-masternodes-table-ux-refinements
                      (2026-06-11): disabled when status is ENABLED (already
                      running) or PRE_ENABLED (already starting) — starting
                      these is a no-op at best and confusing at worst. All
                      other statuses (MISSING / EXPIRED / NEW_START_REQUIRED /
                      UPDATE_REQUIRED / VIN_SPENT / REMOVE / POS_ERROR /
                      WATCHDOG_EXPIRED / SENTINEL_PING_EXPIRED) remain
                      clickable so the user can attempt to restart. */}
                  <div
                    style={{
                      width: COL.actions,
                      display: 'flex',
                      justifyContent: 'center',
                      flexShrink: 0,
                      marginLeft: '8px',
                      gap: '4px',
                    }}
                  >
                    <span
                      onClick={(e) => e.stopPropagation()}
                      onKeyDown={(e) => e.stopPropagation()}
                    >
                      <IconButton
                        size={26}
                        icon={<Play size={14} />}
                        title={t('actions.startAlias')}
                        ariaLabel={t('actions.startAlias')}
                        onClick={() => onStartMasternode(mn)}
                        disabled={mn.status === 'ENABLED' || mn.status === 'PRE_ENABLED'}
                      />
                    </span>
                    <span
                      onClick={(e) => e.stopPropagation()}
                      onKeyDown={(e) => e.stopPropagation()}
                    >
                      <IconButton
                        size={26}
                        icon={<Pencil size={14} />}
                        title={t('actions.editMasternode')}
                        ariaLabel={t('actions.editMasternode')}
                        onClick={() => onEditMasternode(mn)}
                      />
                    </span>
                    <span
                      onClick={(e) => e.stopPropagation()}
                      onKeyDown={(e) => e.stopPropagation()}
                    >
                      <IconButton
                        size={26}
                        variant="danger"
                        icon={<Trash2 size={14} />}
                        title={t('actions.deleteMasternode')}
                        ariaLabel={t('actions.deleteMasternode')}
                        onClick={() => onDeleteMasternode(mn)}
                      />
                    </span>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
};
