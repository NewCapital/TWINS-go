import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { QRCodeCanvas } from 'qrcode.react';
import type { AddressBalance, AddressBasic, AddressStats, AddressTx, AddressUTXO } from '@/store/slices/explorerSlice';
import {
  ArrowLeft,
  Check,
  ChevronLeft,
  ChevronRight,
  Clock,
  Copy,
  Eye,
  TrendingDown,
  TrendingUp,
} from 'lucide-react';
import { DashboardCard } from '@/shared/components/DashboardCard';
import { IconButton } from '@/shared/components/IconButton';
import { RefreshCountdown } from '@/shared/components/RefreshCountdown';
import { RowsPerPageSelect } from '@/shared/components/RowsPerPageSelect';
import { UnitBadge } from '@/shared/components/UnitBadge';
import { writeToClipboard } from '@/shared/utils/clipboard';
import { truncateAddress } from '@/shared/utils/format';
import { useDisplayUnits } from '@/shared/hooks/useDisplayUnits';
import { useDisplayDateTime } from '@/shared/hooks/useDisplayDateTime';

// `DateHeaderInline` was removed in m-fix-date-display-inconsistencies
// (2026-06-04). The dedicated Date column header was dropped from the Tx
// sticky table header when Date moved into a stacked second line under
// each hash in the Hash flex:1 cell. See the rewrite comment in the Tx
// sticky header for details.
import { buildTwinsURI } from '@/shared/utils/twinsUri';
import { createCircularLogoDataURL } from '@/shared/utils/qrLogo';

// Wails-generated TS classes (regenerated bindings expose these shapes)
import type { core } from '@wailsjs/go/models';

// ============================================================================
// Props
// ============================================================================

interface AddressViewProps {
  // Minimal O(1) subset (Address only) — drives the hero header. Null while
  // basic fetch is in flight on first open. Renders immediately because the
  // backend GetAddressBasic only validates the address with no storage access.
  addressBasic: AddressBasic | null;
  // Current spendable balance — populated by GetExplorerAddressBalance via a
  // UTXO prefix scan. Null while the scan is in flight; the Balance row
  // shows a skeleton placeholder until this arrives.
  addressBalance: AddressBalance | null;
  // Aggregate stats (TxCount/TotalReceived/TotalSent/FirstSeen/LastSeen).
  // Null while the slow walk is in flight; the Activity column shows a
  // skeleton until this arrives.
  addressStats: AddressStats | null;
  // True while the balance fetch is in flight. Drives the Balance-row
  // skeleton independently of `isLoading`.
  balanceLoading: boolean;
  // True while the stats fetch is in flight. Drives the Activity-column
  // skeleton independently of `isLoading` (which gates the entire view).
  statsLoading: boolean;
  isLoading: boolean;
  onTxClick: (txid: string) => void;
  onBack: () => void;
  onRefresh: () => void;
  isAnyLoading: boolean;
  // Callback wrappers around the Wails bindings so AddressView stays decoupled
  // from `@wailsjs/...` imports — parent (ExplorerPage) supplies them.
  onFetchTransactions: (page: number, pageSize: number) => Promise<core.AddressTxPage>;
  onFetchUTXOs: (page: number, pageSize: number) => Promise<core.AddressUTXOPage>;
  // Re-fetch the address-level info (basic + stats) — called on auto-refresh
  // tick and manual refresh click. Parent owns the two new Wails calls so the
  // store stays the single source of truth.
  onRefreshAddressInfo: () => Promise<void>;
}

// ============================================================================
// Constants
// ============================================================================

const REFRESH_INTERVAL_SECONDS = 60;
const PAGE_SIZE_OPTIONS = [25, 50, 100, 250] as const;
type PageSize = (typeof PAGE_SIZE_OPTIONS)[number];
const DEFAULT_PAGE_SIZE: PageSize = 25;

// Delay before flipping `isLoading` true in `fetchTransactions` / `fetchUTXOs`.
// On fast backends (small addresses) the entire fetch round-trip completes
// before this threshold, so the loading flag never flips and the user sees
// an instant transition to new rows. On slow backends (~1.75M tx Dev-Fund
// address) the threshold expires first, `isLoading` flips true, the gate
// in TransactionsColumn / UTXOsColumn clears stale rows. 200ms is the
// Material Design / Nielsen Norman threshold below which UI changes read
// as immediate. See l-fix-address-detail-stale-rows-on-pagination round 2
// (2026-06-01).
const LOADING_DELAY_MS = 200;

// ============================================================================
// Style tokens (Receive design language)
// ============================================================================

const pageOuterStyle: React.CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  height: '100%',
  overflow: 'hidden',
};

const pageScrollStyle: React.CSSProperties = {
  flex: 1,
  overflow: 'auto',
  display: 'flex',
  flexDirection: 'column',
  gap: '12px',
  padding: '0',
};

// QR column tokens. The QR is paired visually with the address+balance
// hero zone and sits left of `heroRightColumnStyle` inside `heroZoneStyle`.
// `flexShrink: 0` is load-bearing — without it the QR canvas would compress
// when the address text wraps in the right column, distorting the QR image
// and breaking scannability.
// Width is 224px = 200px canvas + 12px padding × 2 from `qrWrapperStyle`
// below (content-box). Canvas bumped 140px → 200px so the QR roughly fills
// the right column's natural height (address capsule + Balance label + 26px
// balance value + divider + 2-card secondary row ≈ ~220px).
// `alignItems: center` + `justifyContent: center` vertically and horizontally
// center the QR within the column so any residual height slack (when the
// right column grows taller than the QR canvas) distributes symmetrically
// above and below the QR rather than pinning it to the top.
const qrColumnStyle: React.CSSProperties = {
  flexShrink: 0,
  width: '224px',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  flexDirection: 'column',
  gap: '8px',
};

// White-bg wrapper for the QR canvas — mirrors the Receive page hero QR
// wrapper. `lineHeight: 0` collapses the inline-flow baseline space below
// the canvas that would otherwise leak below the 8px-radius rounded
// corner clipping. The 12px white padding gives QR scanners the quiet
// zone they need around the code.
const qrWrapperStyle: React.CSSProperties = {
  padding: '12px',
  backgroundColor: '#ffffff',
  borderRadius: '8px',
  lineHeight: 0,
};

// --- Merged hero + balance card tokens (m-address-detail-hero-redesign) ---
//
// The address bar card and the 3-tile balance strip from the prior
// 2026-05-27 redesign are merged into a single hero card. Hero zone holds
// the address + BALANCE (large green accent); a divider separates it from
// the secondary row that holds Total Received / Total Sent / Activity in
// 3 equal columns.
//
// `formatAddressDateLocal` clones the BlockDetail.tsx pattern verbatim per
// the per-view date-helper convention documented in the Explorer migration
// sequence — Tx Details may want different precision (no seconds), so
// de-duplication into a shared helper is premature.

// Hero zone is a 2-column flex row: QR canvas on the left (via
// `qrColumnStyle` above) and the address + BALANCE stack on the right
// (via `heroRightColumnStyle` below). `gap: 16px` separates the two
// columns; `alignItems: center` vertically centers the QR canvas
// relative to the (typically taller) right column so any residual
// height slack distributes symmetrically above and below the QR
// rather than pinning it to the top with a dead band beneath.
const heroZoneStyle: React.CSSProperties = {
  display: 'flex',
  gap: '16px',
  alignItems: 'center',
};

// `minHeight: 224px` reserves the QR column's height (200px canvas + 12px
// wrapper padding × 2 = 224px) so the card never shrinks when stats are
// in flight. Without this, the right column collapses to its content
// height during the basic-only render and the whole card visibly jumps
// taller when addressStats resolves.
const heroRightColumnStyle: React.CSSProperties = {
  flex: 1,
  display: 'flex',
  flexDirection: 'column',
  gap: '4px',
  minWidth: 0,
  minHeight: '224px',
};

// Hero address row: holds the address capsule + Copy IconButton in a flex
// row. `marginBottom` dropped — vertical spacing is owned by the parent
// `heroRightColumn`'s `gap: 4px`.
const addressRowStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: '8px',
};

// Address capsule: input-chrome wrapper around the address text, mirroring
// the Receive design-language input token (#252525 bg / 1px #3a3a3a / 4px
// radius / 7px 10px padding). The capsule visually anchors the address as
// the page's primary identity above the BALANCE figure (priority order:
// QR → Address → Balance → Statistics per user-approved hierarchy).
const addressCapsuleStyle: React.CSSProperties = {
  flex: 1,
  minWidth: 0,
  backgroundColor: '#252525',
  border: '1px solid #3a3a3a',
  borderRadius: '4px',
  padding: '7px 10px',
  textAlign: 'center',
};

// Address text: 22px 600 monospace inside the capsule. Larger than every
// other text on the page so the user reads the address before the BALANCE
// figure — the address IS the entity this page describes, the balance is
// a property of it.
const addressTextStyle: React.CSSProperties = {
  fontFamily: 'monospace',
  fontSize: '22px',
  fontWeight: 600,
  color: '#ddd',
  wordBreak: 'break-all',
  userSelect: 'none',
  WebkitUserSelect: 'none',
};

// BALANCE hero value: 26px (up from 20px) with tabular-nums so the digits
// align in a consistent column if the user opens multiple Address Detail
// views back-to-back. Still smaller than the 22px address typography by
// effective weight (monospace digits at 26px read narrower than 22px
// monospace text) — preserves the Address > Balance hierarchy.
const balanceHeroValueStyle: React.CSSProperties = {
  fontFamily: 'monospace',
  fontSize: '26px',
  fontWeight: 600,
  color: '#27ae60', // TWINS green hero accent
  fontVariantNumeric: 'tabular-nums',
  lineHeight: 1,
  userSelect: 'none',
  WebkitUserSelect: 'none',
};

const dividerStyle: React.CSSProperties = {
  height: '1px',
  backgroundColor: '#3a3a3a',
  margin: '12px 0',
};

// 3-column CSS grid (Variant A). `minmax(0, 1fr)` lets columns shrink
// without overflowing the card on narrow viewports; large numeric values
// truncate via the column's existing `minWidth: 0` rather than forcing
// horizontal scroll. `gap: 12px` (down from 24px) because each metric
// now lives inside a card-capsule wrapper with its own internal padding,
// so the row gap can be tighter.
const secondaryRowStyle: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
  gap: '12px',
  alignItems: 'stretch',
};

// Variant of `metricCardStyle` for the combined Received + Sent card.
// Holds two stacked label+value sub-rows; `gap: 10px` separates them so the
// two flow metrics read as paired siblings rather than a tight stack.
const metricCardCombinedStyle: React.CSSProperties = {
  backgroundColor: '#2a2a2a',
  border: '1px solid #3a3a3a',
  borderRadius: '6px',
  padding: '12px 16px',
  display: 'flex',
  flexDirection: 'column',
  gap: '10px',
  minWidth: 0,
};

// Single-metric sub-row inside the combined card: label row above value.
const metricSubRowStyle: React.CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  gap: '4px',
  minWidth: 0,
};

// Card-capsule wrapper for each of the three secondary metrics (Received,
// Sent, Activity). Replaces the prior flat-column layout — gives each
// metric its own visual container so the semantic-color value reads as
// a discrete data point rather than a column entry.
const metricCardStyle: React.CSSProperties = {
  backgroundColor: '#2a2a2a',
  border: '1px solid #3a3a3a',
  borderRadius: '6px',
  padding: '12px 16px',
  display: 'flex',
  flexDirection: 'column',
  gap: '4px',
  minWidth: 0,
};

// Activity-card variant of `metricCardStyle`. The 2-column secondary grid
// uses `alignItems: stretch`, so the Activity card grows to match the
// taller combined Received+Sent card (which has 2 sub-rows). Activity has
// only label + primary + secondary.
//
// Layout strategy (SC9 of `m-address-detail-cosmetic-fixes-and-audit`,
// 2026-05-30): the label row sits at the top of the card (and is pushed
// to the right edge by `metricLabelRowStyle.justifyContent: 'flex-end'`
// from SC8 — landing the label in the TOP-RIGHT CORNER), while the
// primary value + secondary date range get wrapped in `activityContentStyle`
// (`flex: 1, justifyContent: center, alignItems: center`) so they occupy
// all remaining vertical space and center both horizontally and vertically
// within it.
//
// Earlier SC2 implementation set `justifyContent: 'center'` here to center
// the ENTIRE column (label + primary + secondary as one stacked block).
// That was superseded by SC9 — the new wrapper-based approach gives the
// "label in corner + content centered" pattern the user requested after
// live verification of the SC2 baseline.
//
// Combined card explicitly uses `metricCardCombinedStyle` (no centering
// needed there because the 2 sub-rows already fill the height).
const metricCardActivityStyle: React.CSSProperties = {
  ...metricCardStyle,
};

// Wrapper around the Activity card's primary value + secondary date range.
// `flex: 1` consumes all the vertical space the label row doesn't claim;
// `justifyContent: 'center'` + `alignItems: 'center'` center the content
// both horizontally and vertically within that space. `gap: 4px` separates
// the primary span from the secondary span. Introduced for SC9 of
// `m-address-detail-cosmetic-fixes-and-audit` (2026-05-30).
const activityContentStyle: React.CSSProperties = {
  flex: 1,
  display: 'flex',
  flexDirection: 'column',
  alignItems: 'center',
  justifyContent: 'center',
  gap: '4px',
};

// Label row with leading icon. Icon color matches the value color
// (Received green / Sent red / Activity neutral) — set via inline color
// prop on the lucide icon at the render site, not on this style.
//
// `justifyContent: 'flex-end'` (added 2026-05-30, SC8 of
// `m-address-detail-cosmetic-fixes-and-audit`) pushes the icon + label
// cluster to the right edge of its container. For Received/Sent sub-rows
// in the combined card, this stacks the label flush-right above the
// already-right-aligned value (SC3) — labels and values both align to
// the card's right edge in a mini-table style. For the Activity card,
// the same flex-end behavior puts the Activity label in the top-right
// corner — which is exactly what SC9 wants. One token, both outcomes.
const metricLabelRowStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: '6px',
  justifyContent: 'flex-end',
};

const metricLabelTextStyle: React.CSSProperties = {
  fontSize: '11px',
  fontWeight: 500,
  color: '#888',
};

// Value tokens — three variants for semantic coloring. All share the
// `tabular-nums` font feature so digit widths stay column-aligned across
// the three cards regardless of value magnitude.
//
// Received and Sent variants add `alignSelf: 'flex-end'` so the monetary
// value sits flush against the right edge of its cell — the financial-UI
// convention of right-aligned monetary columns. Label row sits at the top
// of the cell at the natural left edge (its own flex row). Activity does
// NOT get this — its primary value (`1,386 days active`) is a phrase, not
// a number, and reads better left-aligned. Introduced by task
// `m-address-detail-cosmetic-fixes-and-audit` (2026-05-30).
const metricValueBaseStyle: React.CSSProperties = {
  fontFamily: 'monospace',
  fontSize: '14px',
  fontWeight: 500,
  fontVariantNumeric: 'tabular-nums',
  lineHeight: 1,
};
const metricValueReceivedStyle: React.CSSProperties = {
  ...metricValueBaseStyle,
  color: '#27ae60',
  alignSelf: 'flex-end',
};
const metricValueSentStyle: React.CSSProperties = {
  ...metricValueBaseStyle,
  color: '#ff6666',
  alignSelf: 'flex-end',
};
const metricValueActivityStyle: React.CSSProperties = { ...metricValueBaseStyle, color: '#ddd' };

// Secondary line under the Activity primary — shows the date range as
// supporting context after the primary `N days active` number.
const metricSecondaryStyle: React.CSSProperties = {
  fontSize: '11px',
  color: '#888',
  fontVariantNumeric: 'tabular-nums',
  lineHeight: 1,
};

// Skeleton placeholder for the secondary-row stats values while the slow
// `addressStats` fetch is in flight. Sized to roughly match the value row's
// content box (14px font-size + a touch of padding) so the layout doesn't
// shift when the real value lands. Tone matches the Receive design language
// muted-fill convention (`#3a3a3a` on the `#2f2f2f` card surface).
const skeletonValueStyle: React.CSSProperties = {
  display: 'inline-block',
  width: '120px',
  height: '14px',
  borderRadius: '3px',
  backgroundColor: '#3a3a3a',
};

// Wider skeleton for the Activity column, which renders a date range
// like "Aug 13, 2022 → May 28, 2026" (~160px at 14px monospace) once
// stats resolve. Matching the final width prevents a layout shift.
// Height overridden to 11px to match `metricSecondaryStyle.fontSize`
// (with `lineHeight: 1`) — the row this skeleton replaces, so the
// secondary line height stays identical pre- and post-load.
const skeletonActivityStyle: React.CSSProperties = {
  ...skeletonValueStyle,
  width: '160px',
  height: '11px',
};

/**
 * Format a Unix timestamp (seconds) as a local-time string matching the
 * Block Detail page convention (e.g. "May 27, 2026, 16:07:53 GMT+2").
 *
 * Cloned verbatim from `BlockDetail.tsx:formatBlockDateLocal` per the
 * per-view date-helper convention. Returns empty string for zero/negative
 * timestamps; callers should branch on `> 0` before invoking and render
 * "N/A" themselves for the unknown case.
 */
function formatAddressDateLocal(timestampSec: number): string {
  if (timestampSec <= 0) return '';
  return new Intl.DateTimeFormat('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
    timeZoneName: 'short',
  }).format(new Date(timestampSec * 1000));
}

/**
 * Format a short date for the Activity range display (e.g. "Mar 9, 2026").
 * No time, no timezone — those live in the tooltip via `formatActivityTooltip`.
 * Returns empty string for zero/negative timestamps.
 */
function formatAddressDateShort(timestampSec: number): string {
  if (timestampSec <= 0) return '';
  return new Intl.DateTimeFormat('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  }).format(new Date(timestampSec * 1000));
}

/**
 * Format the Activity column as a single-line "First → Last" range. Returns
 * "N/A" when both timestamps are zero. When one is zero, shows the available
 * end with the other side rendered as "N/A".
 */
function formatActivityRange(firstSec: number, lastSec: number): string {
  const first = firstSec > 0 ? formatAddressDateShort(firstSec) : 'N/A';
  const last = lastSec > 0 ? formatAddressDateShort(lastSec) : 'N/A';
  if (first === 'N/A' && last === 'N/A') return 'N/A';
  return `${first} → ${last}`;
}

/**
 * Build the multi-line tooltip text for the Activity range. Preserves the
 * full local-time precision (HH:MM:SS + GMT offset) that the prior stacked
 * First/Last sub-rows showed inline.
 */
function formatActivityTooltip(firstSec: number, lastSec: number): string {
  const first = firstSec > 0 ? formatAddressDateLocal(firstSec) : 'N/A';
  const last = lastSec > 0 ? formatAddressDateLocal(lastSec) : 'N/A';
  return `First: ${first}\nLast: ${last}`;
}

/**
 * Format the Activity primary value as a duration in days. Returns "N/A"
 * when either timestamp is missing (≤ 0). Renders a human-readable string:
 *   - "< 1 day active" when duration spans less than 24 hours
 *   - "~1 day active" when exactly one day
 *   - "{N} days active" for any longer duration (with thousands separators)
 *
 * The duration is the primary information on the Activity card; the date
 * range below it (rendered via `formatActivityRange`) is secondary context.
 */
function formatActivityDuration(firstSec: number, lastSec: number): string {
  if (firstSec <= 0 || lastSec <= 0) return 'N/A';
  const durationDays = Math.floor((lastSec - firstSec) / 86400);
  if (durationDays < 1) return '< 1 day active';
  if (durationDays === 1) return '~1 day active';
  return `${durationDays.toLocaleString('en-US')} days active`;
}

// --- 2-column grid layout ---
const gridContainerStyle: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
  gap: '12px',
  flex: 1,
  // Viewport-aware floor: 400px is the minimum readable column height, but
  // when the viewport is shorter than ~880px (hero+address+balance cards
  // already consume ~280px, leaving ~480px chrome budget) we shrink to fit
  // so the page never forces outer scroll. Mirrors the formula used by
  // `features/wallet/pages/Transactions.tsx` scroll container after
  // `m-tx-table-context-menu-and-antiflicker` (2026-05-20).
  minHeight: 'min(400px, calc(100vh - 480px))',
};

const columnCardStyle: React.CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  backgroundColor: '#2f2f2f',
  border: '1px solid #3a3a3a',
  borderRadius: '8px',
  overflow: 'hidden',
  minHeight: 0,
};

const columnHeaderRowStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: '8px',
  padding: '12px 16px',
  borderBottom: '1px solid #3a3a3a',
  fontSize: '13px',
  fontWeight: 600,
  color: '#ccc',
  flexShrink: 0,
};

const columnScrollStyle: React.CSSProperties = {
  flex: 1,
  overflow: 'auto',
  display: 'flex',
  flexDirection: 'column',
  minHeight: 0,
};

const stickyTableHeaderStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: '12px',
  padding: '8px 12px',
  position: 'sticky',
  top: 0,
  zIndex: 5,
  backgroundColor: '#2f2f2f',
  borderBottom: '1px solid #3a3a3a',
};

const tableHeaderCellStyle: React.CSSProperties = {
  fontSize: '11px',
  fontWeight: 500,
  color: '#888',
};

const rowListStyle: React.CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  gap: '4px',
  padding: '8px',
};

const rowCardStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: '12px',
  padding: '8px 12px',
  backgroundColor: '#2a2a2a',
  border: '1px solid transparent',
  borderRadius: '6px',
  cursor: 'default',
  transition: 'border-color 0.15s',
};

const monoCellStyle: React.CSSProperties = {
  fontFamily: 'monospace',
  fontSize: '12px',
  color: '#ddd',
};

const paginationFooterStyle: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '1fr auto 1fr',
  alignItems: 'center',
  gap: '8px',
  padding: '8px 12px',
  borderTop: '1px solid #3a3a3a',
  flexShrink: 0,
  fontSize: '11px',
  color: '#888',
};

const paginationButtonStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  width: '24px',
  height: '24px',
  backgroundColor: '#383838',
  border: '1px solid #4a4a4a',
  borderRadius: '4px',
  color: '#ccc',
  cursor: 'pointer',
  flexShrink: 0,
};

const paginationButtonDisabledStyle: React.CSSProperties = {
  ...paginationButtonStyle,
  cursor: 'not-allowed',
  opacity: 0.4,
};

const emptyStateStyle: React.CSSProperties = {
  flex: 1,
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  padding: '32px',
  color: '#888',
  fontSize: '12px',
};

// ============================================================================
// Helpers
// ============================================================================

// Legacy helpers — kept for reference. The global useDisplayDateTime hook
// owns the Tx-row Age cell rendering now; `void` markers below suppress the
// TS6133 unused-local warning so the file builds cleanly without dropping
// the historical implementations.
const formatTime = (timeStr: string) => new Date(timeStr).toLocaleString();

// Compact relative-age helper for the Tx column. Returns "<1m ago" for sub-minute
// freshness, then "Nm ago" / "Nh ago" / "Nd ago" for older entries. Pattern ported
// from features/explorer/components/BlockList.tsx:formatAgeShort.
// Math.max(0, ...) is load-bearing — clamps negative deltas from clock skew so a
// future-stamped tx doesn't render as a nonsense negative duration.
const formatAgeShort = (timestamp: string | number): string => {
  const nowSec = Date.now() / 1000;
  const tsSec = typeof timestamp === 'number' ? timestamp : new Date(timestamp).getTime() / 1000;
  const diffSec = Math.max(0, nowSec - tsSec);
  if (diffSec < 60) return '<1m ago';
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m ago`;
  if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h ago`;
  const days = Math.floor(diffSec / 86400);
  if (days < 30) return `${days}d ago`;
  if (days < 365) return `${Math.floor(days / 30)}mo ago`;
  const years = Math.floor(days / 365);
  const monthsRemainder = Math.floor((days % 365) / 30);
  return monthsRemainder > 0 ? `${years}y ${monthsRemainder}mo ago` : `${years}y ago`;
};
void formatTime;
void formatAgeShort;

// ============================================================================
// Main component
// ============================================================================

export const AddressView: React.FC<AddressViewProps> = ({
  addressBasic,
  addressBalance,
  addressStats,
  balanceLoading,
  statsLoading,
  isLoading,
  onTxClick,
  onBack,
  onRefresh: _onRefresh,
  isAnyLoading,
  onFetchTransactions,
  onFetchUTXOs,
  onRefreshAddressInfo,
}) => {
  const { t } = useTranslation('common');
  const { formatAmount } = useDisplayUnits();

  // --- Pagination state (independent per column) ---
  const [txRows, setTxRows] = useState<AddressTx[]>([]);
  const [txTotal, setTxTotal] = useState(0);
  const [txPage, setTxPage] = useState(1);
  const [txPageSize, setTxPageSize] = useState<PageSize>(DEFAULT_PAGE_SIZE);
  const [txIsLoading, setTxIsLoading] = useState(false);

  const [utxoRows, setUtxoRows] = useState<AddressUTXO[]>([]);
  const [utxoTotal, setUtxoTotal] = useState(0);
  const [utxoPage, setUtxoPage] = useState(1);
  const [utxoPageSize, setUtxoPageSize] = useState<PageSize>(DEFAULT_PAGE_SIZE);
  const [utxoIsLoading, setUtxoIsLoading] = useState(false);

  // --- Auto-refresh countdown + copy feedback ---
  const [countdown, setCountdown] = useState(REFRESH_INTERVAL_SECONDS);
  const [copiedField, setCopiedField] = useState<string | null>(null);

  // --- QR logo data URL ---
  // Built once on mount from the static `/icons/twins-logo.png` asset via
  // the shared `createCircularLogoDataURL` helper (same path the Receive
  // page hero uses, scaled down to match the 180px QR canvas: 56px inner
  // diameter inside a 64px image-settings cell, leaving margin for the
  // 4px green circular border at #27ae60). The proportion is slightly
  // tighter than Receive (64/76 → 56/64) — intentional for the secondary
  // Explorer view per the user-approved task spec. `.catch(() => {})`
  // handles image-load failure gracefully: when the logo never resolves,
  // `qrLogoSrc` stays empty and the QR renders without the center
  // overlay (still scannable). `mountedRef` guards against the React
  // memory-leak warning when the user navigates away before the
  // canvas-rendering promise resolves.
  const [qrLogoSrc, setQrLogoSrc] = useState<string>('');
  useEffect(() => {
    let mounted = true;
    createCircularLogoDataURL('/icons/twins-logo.png', 56, 4, '#27ae60')
      .then((dataURL) => {
        if (mounted) setQrLogoSrc(dataURL);
      })
      .catch(() => {});
    return () => {
      mounted = false;
    };
  }, []);

  // --- Refs ---
  const copyTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const silentInFlightRef = useRef(false);
  // Monotonic token bumped on every address change. Captured by each in-
  // flight silent refresh; the `.finally()` only clears the inflight lock
  // when the token still matches. This mirrors the BlockDetail.tsx pattern
  // (m-block-detail-auto-refresh-and-nav-pills, 2026-05-26) and prevents
  // a stale .finally() for the prior address from accidentally unlocking
  // a fresh in-flight fetch on the new address (Claude code-review W2).
  const silentTokenRef = useRef(0);
  // Per-column monotonic fetch sequence counters. Each fetchTransactions /
  // fetchUTXOs / auto-refresh-tick call captures its own seq at start; the
  // response-commit check rejects writes whose seq is no longer the latest.
  // This catches the page-change race that the address-only guard misses:
  // user paginates from page N → page N+1 while the N-fetch is in flight;
  // the N-response would otherwise overwrite the N+1-rendered rows (codex
  // round-3 W1+W2). Mirrors the `fetchSeqRef` / `fetchBlockSeqRef` pattern
  // in ExplorerPage.
  const txFetchSeqRef = useRef(0);
  const utxoFetchSeqRef = useRef(0);
  const addressRef = useRef<string | null>(null);
  // Latest fetch/refresh callbacks, mirrored in refs so the interval tick can
  // call them without re-keying the interval `useEffect` on every parent
  // re-render (which would otherwise leak intervals).
  const onFetchTransactionsRef = useRef(onFetchTransactions);
  const onFetchUTXOsRef = useRef(onFetchUTXOs);
  const onRefreshAddressInfoRef = useRef(onRefreshAddressInfo);
  // Latest pagination state mirrored into refs so the auto-refresh interval
  // tick can read the CURRENT page/pageSize without re-keying the interval
  // useEffect (Claude code-review W1). Previously the interval `useEffect`
  // deps array included [txPage, txPageSize, utxoPage, utxoPageSize] which
  // tore down + recreated the interval (and reset the countdown to 60s) on
  // every page change — a user who paginates frequently never saw an auto-
  // refresh fire.
  const txPageRef = useRef(txPage);
  const txPageSizeRef = useRef(txPageSize);
  const utxoPageRef = useRef(utxoPage);
  const utxoPageSizeRef = useRef(utxoPageSize);

  // Refs on the per-column inner scroll containers (columnScrollStyle div).
  // Used by the two scroll-to-top useEffects below so paginating Tx/UTXO
  // resets that column's scroll position to top — previously the user
  // landed mid-list on a freshly-fetched page. Per-column independence
  // matters because the two columns paginate independently.
  const txScrollRef = useRef<HTMLDivElement>(null);
  const utxoScrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    onFetchTransactionsRef.current = onFetchTransactions;
  }, [onFetchTransactions]);
  useEffect(() => {
    onFetchUTXOsRef.current = onFetchUTXOs;
  }, [onFetchUTXOs]);
  useEffect(() => {
    onRefreshAddressInfoRef.current = onRefreshAddressInfo;
  }, [onRefreshAddressInfo]);
  useEffect(() => { txPageRef.current = txPage; }, [txPage]);
  useEffect(() => { txPageSizeRef.current = txPageSize; }, [txPageSize]);
  useEffect(() => { utxoPageRef.current = utxoPage; }, [utxoPage]);
  useEffect(() => { utxoPageSizeRef.current = utxoPageSize; }, [utxoPageSize]);

  // Reset per-column scroll position to top whenever the page changes so the
  // user never lands mid-list on a freshly fetched page. Two independent
  // effects (one per column) so Tx and UTXO pagination remain decoupled.
  // The ref null-check is defensive — under StrictMode the effect can fire
  // briefly before the ref attaches to a DOM node.
  useEffect(() => {
    if (txScrollRef.current) txScrollRef.current.scrollTop = 0;
  }, [txPage]);
  useEffect(() => {
    if (utxoScrollRef.current) utxoScrollRef.current.scrollTop = 0;
  }, [utxoPage]);

  // ----- Fetch transactions page -----
  // Captures `seq` at dispatch; only the newest fetch commits its result
  // (codex round-3 W1). The address-only guard alone is not enough — page
  // changes also produce in-flight + new-fetch races.
  const fetchTransactions = useCallback(
    async (page: number, pageSize: number) => {
      if (!addressBasic) return;
      const seq = ++txFetchSeqRef.current;
      // Delay the isLoading flip by LOADING_DELAY_MS so fast backends skip
      // the loading placeholder entirely (no flicker on small addresses).
      // The finally block cancels this timer — if the fetch resolved before
      // the threshold, `isLoading` stayed false the whole time.
      const loadingTimer = setTimeout(() => {
        if (seq === txFetchSeqRef.current) setTxIsLoading(true);
      }, LOADING_DELAY_MS);
      try {
        const result = await onFetchTransactionsRef.current(page, pageSize);
        // Stale-fetch guard: abandon if a newer fetch superseded this one
        // OR the address itself changed mid-flight.
        if (seq !== txFetchSeqRef.current) return;
        if (addressRef.current !== addressBasic.address) return;
        if (result) {
          setTxRows(result.transactions || []);
          setTxTotal(result.total ?? 0);
        }
      } catch (err) {
        if (seq !== txFetchSeqRef.current) return;
        console.error('Failed to fetch transactions page:', err);
      } finally {
        // Cancel the deferred loading flip unconditionally — if it already
        // fired, the seq-guarded setTxIsLoading(false) below resets it.
        clearTimeout(loadingTimer);
        // Only the newest fetch clears the loading flag; stale fetches must
        // not flip the spinner off while a newer fetch is still in flight.
        if (seq === txFetchSeqRef.current) {
          setTxIsLoading(false);
        }
      }
    },
    [addressBasic],
  );

  // ----- Fetch UTXOs page -----
  // Same seq pattern as fetchTransactions above (codex round-3 W2).
  const fetchUTXOs = useCallback(
    async (page: number, pageSize: number) => {
      if (!addressBasic) return;
      const seq = ++utxoFetchSeqRef.current;
      // Same delayed-loading pattern as fetchTransactions — see comment above.
      const loadingTimer = setTimeout(() => {
        if (seq === utxoFetchSeqRef.current) setUtxoIsLoading(true);
      }, LOADING_DELAY_MS);
      try {
        const result = await onFetchUTXOsRef.current(page, pageSize);
        if (seq !== utxoFetchSeqRef.current) return;
        if (addressRef.current !== addressBasic.address) return;
        if (result) {
          setUtxoRows(result.utxos || []);
          setUtxoTotal(result.total ?? 0);
        }
      } catch (err) {
        if (seq !== utxoFetchSeqRef.current) return;
        console.error('Failed to fetch utxos page:', err);
      } finally {
        clearTimeout(loadingTimer);
        if (seq === utxoFetchSeqRef.current) {
          setUtxoIsLoading(false);
        }
      }
    },
    [addressBasic],
  );

  // ----- Reset pagination + countdown on address change -----
  // Also bumps `silentTokenRef` so any in-flight silent refresh for the
  // PRIOR address cannot un-set `silentInFlightRef` after the address has
  // changed (Claude code-review W2).
  useEffect(() => {
    if (!addressBasic) return;
    addressRef.current = addressBasic.address;
    silentTokenRef.current += 1;
    silentInFlightRef.current = false;
    setTxPage(1);
    setUtxoPage(1);
    setCountdown(REFRESH_INTERVAL_SECONDS);
  }, [addressBasic?.address]); // eslint-disable-line react-hooks/exhaustive-deps

  // ----- Fetch tx page on (address, page, pageSize) change -----
  useEffect(() => {
    if (!addressBasic) return;
    fetchTransactions(txPage, txPageSize);
  }, [addressBasic?.address, txPage, txPageSize, fetchTransactions]); // eslint-disable-line react-hooks/exhaustive-deps

  // ----- Fetch utxo page on (address, page, pageSize) change -----
  useEffect(() => {
    if (!addressBasic) return;
    fetchUTXOs(utxoPage, utxoPageSize);
  }, [addressBasic?.address, utxoPage, utxoPageSize, fetchUTXOs]); // eslint-disable-line react-hooks/exhaustive-deps

  // ----- Auto-refresh interval -----
  // Deps keyed ONLY on address (Claude code-review W1) — page/pageSize are
  // read from refs at tick time so paginating doesn't tear down + recreate
  // the interval (which would reset the countdown to 60s and starve the
  // silent refresh on heavily-paginated addresses).
  useEffect(() => {
    if (!addressBasic) return;
    intervalRef.current = setInterval(() => {
      setCountdown((c) => {
        if (c > 1) return c - 1;
        // Trigger silent refresh of all three sources. Token captured at
        // dispatch time; `.finally()` only clears the inflight lock when
        // the token still matches — prevents a stale .finally() for the
        // prior address from unlocking a fresh in-flight fetch on the new
        // address (Claude code-review W2).
        if (!silentInFlightRef.current) {
          silentInFlightRef.current = true;
          const myToken = silentTokenRef.current;
          const curTxPage = txPageRef.current;
          const curTxPageSize = txPageSizeRef.current;
          const curUtxoPage = utxoPageRef.current;
          const curUtxoPageSize = utxoPageSizeRef.current;
          // Capture per-column seq numbers so the silent-refresh response
          // also respects the page-change race guard (codex round-3 W1+W2).
          // Without these, a slow silent refresh for page N could resolve
          // after the user paginated to N+1 and overwrite the N+1 rows.
          const txSeq = ++txFetchSeqRef.current;
          const utxoSeq = ++utxoFetchSeqRef.current;
          Promise.all([
            onRefreshAddressInfoRef.current(),
            onFetchTransactionsRef.current(curTxPage, curTxPageSize).then((r) => {
              if (txSeq !== txFetchSeqRef.current) return;
              if (addressRef.current !== addressBasic.address) return;
              if (r) {
                setTxRows(r.transactions || []);
                setTxTotal(r.total ?? 0);
              }
            }),
            onFetchUTXOsRef.current(curUtxoPage, curUtxoPageSize).then((r) => {
              if (utxoSeq !== utxoFetchSeqRef.current) return;
              if (addressRef.current !== addressBasic.address) return;
              if (r) {
                setUtxoRows(r.utxos || []);
                setUtxoTotal(r.total ?? 0);
              }
            }),
          ])
            .catch((err) => console.error('Auto-refresh failed:', err))
            .finally(() => {
              if (silentTokenRef.current === myToken) {
                silentInFlightRef.current = false;
              }
            });
        }
        return REFRESH_INTERVAL_SECONDS;
      });
    }, 1000);
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current);
    };
  }, [addressBasic?.address]); // eslint-disable-line react-hooks/exhaustive-deps

  // ----- Cleanup copy timer on unmount -----
  useEffect(
    () => () => {
      if (copyTimerRef.current) clearTimeout(copyTimerRef.current);
    },
    [],
  );

  // ----- Handlers -----
  const handleManualRefresh = useCallback(async () => {
    if (!addressBasic) return;
    setCountdown(REFRESH_INTERVAL_SECONDS);
    try {
      await Promise.all([
        onRefreshAddressInfoRef.current(),
        fetchTransactions(txPage, txPageSize),
        fetchUTXOs(utxoPage, utxoPageSize),
      ]);
    } catch (err) {
      console.error('Manual refresh failed:', err);
    }
  }, [addressBasic, fetchTransactions, fetchUTXOs, txPage, txPageSize, utxoPage, utxoPageSize]);

  const handleCopy = useCallback(async (field: string, value: string) => {
    const ok = await writeToClipboard(value);
    if (!ok) return;
    if (copyTimerRef.current) clearTimeout(copyTimerRef.current);
    setCopiedField(field);
    copyTimerRef.current = setTimeout(() => {
      setCopiedField(null);
      copyTimerRef.current = null;
    }, 2000);
  }, []);

  // ============================================================================
  // Loading / not-found states
  // ============================================================================

  if (isLoading) {
    return <div style={{ ...emptyStateStyle, height: '100%' }}>{t('explorer.loadingAddress')}</div>;
  }

  if (!addressBasic) {
    return (
      <div style={{ ...emptyStateStyle, height: '100%' }}>{t('explorer.addressNotFound')}</div>
    );
  }

  // ============================================================================
  // Derived
  // ============================================================================

  const txTotalPages = Math.max(1, Math.ceil(txTotal / txPageSize));
  const utxoTotalPages = Math.max(1, Math.ceil(utxoTotal / utxoPageSize));

  // ============================================================================
  // Render
  // ============================================================================

  return (
    <div style={pageOuterStyle}>
      <div style={pageScrollStyle}>
        {/* Merged Address Details card (m-address-detail-merge-cards-and-qr).
            The standalone hero header (back button + title + refresh ring)
            that previously sat above the Balance Information card has been
            folded into this DashboardCard:
              - Back IconButton  -> `headerLeft` slot
              - Title            -> `t('explorer.addressDetails')` (replaces
                                    the prior `explorer.balanceInfo` label
                                    so the merged card carries the page name)
              - Unit label + interactive RefreshCountdown ring -> `headerRight`
                composite (inline flex with 8px gap)
            Body adds a QR-code column on the left of `heroZoneStyle`,
            paired visually with the address + BALANCE hero. The 3 secondary
            metrics (Total Received, Total Sent, Activity) sit below a
            divider in a 3-column row INSIDE the right column per Variant A
            (divider does not span the full card width — keeps the QR
            visually paired with the hero zone only). */}
        <DashboardCard
          title={t('explorer.addressDetails')}
          headerLeft={
            <IconButton
              onClick={onBack}
              title={t('buttons.back')}
              ariaLabel={t('buttons.back')}
              icon={<ArrowLeft size={14} />}
            />
          }
          headerRight={
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <UnitBadge />
              <RefreshCountdown
                countdown={countdown}
                total={REFRESH_INTERVAL_SECONDS}
                mode="interactive"
                onRefresh={handleManualRefresh}
                isLoading={isAnyLoading}
              />
            </div>
          }
        >
          {/* Hero zone: QR column (left) + address/balance column (right). */}
          <div style={heroZoneStyle}>
            <div style={qrColumnStyle}>
              <div style={qrWrapperStyle}>
                <QRCodeCanvas
                  value={buildTwinsURI(addressBasic.address)}
                  size={200}
                  level="H"
                  includeMargin={false}
                  bgColor="#ffffff"
                  fgColor="#000000"
                  imageSettings={qrLogoSrc ? {
                    src: qrLogoSrc,
                    height: 64,
                    width: 64,
                    excavate: true,
                  } : undefined}
                />
              </div>
            </div>
            <div style={heroRightColumnStyle}>
              <div style={addressRowStyle}>
                <div style={addressCapsuleStyle}>
                  <span style={addressTextStyle}>
                    {addressBasic.address}
                  </span>
                </div>
                <IconButton
                  onClick={() => handleCopy('address', addressBasic.address)}
                  title={t('explorer.copyAddress', { defaultValue: 'Copy address' })}
                  ariaLabel={t('explorer.copyAddress', { defaultValue: 'Copy address' })}
                  icon={
                    copiedField === 'address' ? (
                      <Check size={14} color="#27ae60" />
                    ) : (
                      <Copy size={14} />
                    )
                  }
                />
              </div>

              {/* Balance row — functionally symmetric with the address row
                  above: [capsule (#252525 / 1px #3a3a3a / 4px / textAlign:
                  center)] + [Copy IconButton]. Balance value is non-selectable
                  via `userSelect: 'none'` on `balanceHeroValueStyle`; Copy
                  IconButton reads from props (`addressBalance.balance`) via
                  the generic `handleCopy(field, value)` callback. IconButton
                  rendered only when `addressBalance != null` — no point
                  copying a skeleton or em-dash fallback. `aria-label` on the
                  value span preserves screen-reader semantics; the IconButton
                  has its own `ariaLabel` for the Copy action. */}
              <div style={addressRowStyle}>
                <div style={addressCapsuleStyle}>
                  {addressBalance != null ? (
                    <span style={balanceHeroValueStyle} aria-label={t('explorer.balance')}>
                      {formatAmount(addressBalance.balance, false)}
                    </span>
                  ) : balanceLoading ? (
                    <span
                      style={{ ...skeletonValueStyle, height: '26px', width: '180px' }}
                      aria-busy={balanceLoading}
                    />
                  ) : (
                    <span style={balanceHeroValueStyle}>—</span>
                  )}
                </div>
                {/* Copy IconButton always rendered so the row geometry is stable
                    from first paint (no layout shift when balance lands).
                    `disabled` ties to addressBalance null-state so the button
                    greys out during the balance fetch window. The Check icon
                    swap is double-gated (copiedField === 'balance' AND
                    addressBalance != null) so the green Check cannot leak
                    during the loading window — defense-in-depth against any
                    future race where copyTimerRef survives an address change.
                    Pre-`m-address-detail-cosmetic-fixes-and-audit` the button
                    was conditionally rendered (`addressBalance != null && ...`)
                    which left the copy slot empty during the async fetch. */}
                <IconButton
                  onClick={() => {
                    if (addressBalance == null) return;
                    handleCopy(
                      'balance',
                      formatAmount(addressBalance.balance, true),
                    );
                  }}
                  disabled={addressBalance == null}
                  title={t('explorer.copyBalance')}
                  ariaLabel={t('explorer.copyBalance')}
                  icon={
                    copiedField === 'balance' && addressBalance != null ? (
                      <Check size={14} color="#27ae60" />
                    ) : (
                      <Copy size={14} />
                    )
                  }
                />
              </div>

              {/* Divider + secondary row live INSIDE the right column so the
                  divider visually crosses only the right column (Variant A —
                  user-approved layout). Keeping them as siblings of
                  `heroZoneStyle` would draw the divider across the full
                  card width below the QR column, breaking the visual
                  pairing of QR with the entire address/balance/activity
                  stack. */}
              <div style={dividerStyle} />

              {/* Secondary row: TOTAL RECEIVED | TOTAL SENT | ACTIVITY.
                  These three values come from the slower `addressStats`
                  fetch (full address tx-index walk). When stats haven't
                  arrived yet, each value renders as a muted skeleton bar
                  so the row reserves its layout space and the user sees
                  progress instead of a frozen empty card. */}
              <div style={secondaryRowStyle}>
                {/* Combined card: TOTAL RECEIVED + TOTAL SENT stacked
                    vertically. Pairs the two flow metrics as siblings;
                    ACTIVITY (temporal) lives in its own card to the right. */}
                <div style={metricCardCombinedStyle}>
                  {/* TOTAL RECEIVED sub-row */}
                  <div style={metricSubRowStyle}>
                    <div style={metricLabelRowStyle}>
                      <TrendingUp size={12} color="#27ae60" />
                      <span style={metricLabelTextStyle}>{t('explorer.totalReceived')}</span>
                    </div>
                    {addressStats ? (
                      <span style={metricValueReceivedStyle}>
                        {formatAmount(addressStats.total_received, false)}
                      </span>
                    ) : (
                      <span
                        style={{ ...skeletonValueStyle, alignSelf: 'flex-end' }}
                        aria-busy={statsLoading}
                      />
                    )}
                  </div>

                  {/* TOTAL SENT sub-row */}
                  <div style={metricSubRowStyle}>
                    <div style={metricLabelRowStyle}>
                      <TrendingDown size={12} color="#ff6666" />
                      <span style={metricLabelTextStyle}>{t('explorer.totalSent')}</span>
                    </div>
                    {addressStats ? (
                      <span style={metricValueSentStyle}>
                        {formatAmount(addressStats.total_sent, false)}
                      </span>
                    ) : (
                      <span
                        style={{ ...skeletonValueStyle, alignSelf: 'flex-end' }}
                        aria-busy={statsLoading}
                      />
                    )}
                  </div>
                </div>

                {/* ACTIVITY — neutral; primary = "N days active", secondary = date range.
                    Layout per SC9 of `m-address-detail-cosmetic-fixes-and-audit`
                    (2026-05-30): label row sits at the top of the card AND gets
                    pushed to the right edge by `metricLabelRowStyle`'s
                    `justifyContent: 'flex-end'` (SC8 of the same task) — net
                    effect is "label in the top-right corner of the card". The
                    primary value + secondary date range live inside the new
                    `activityContentStyle` wrapper (`flex: 1, center both axes`)
                    so they occupy all remaining vertical space and center both
                    horizontally and vertically within it. */}
                <div style={metricCardActivityStyle}>
                  <div style={metricLabelRowStyle}>
                    <Clock size={12} color="#888" />
                    <span style={metricLabelTextStyle}>
                      {t('explorer.activity', { defaultValue: 'Activity' })}
                    </span>
                  </div>
                  <div style={activityContentStyle}>
                    {addressStats ? (
                      <>
                        <span
                          style={metricValueActivityStyle}
                          title={formatActivityTooltip(addressStats.first_seen, addressStats.last_seen)}
                        >
                          {formatActivityDuration(addressStats.first_seen, addressStats.last_seen)}
                        </span>
                        <span style={metricSecondaryStyle}>
                          {formatActivityRange(addressStats.first_seen, addressStats.last_seen)}
                        </span>
                      </>
                    ) : (
                      <>
                        <span style={skeletonValueStyle} aria-busy={statsLoading} />
                        {/* marginTop dropped — `activityContentStyle.gap: 4px`
                            handles the separator between primary + secondary. */}
                        <span style={skeletonActivityStyle} aria-busy={statsLoading} />
                      </>
                    )}
                  </div>
                </div>
              </div>
            </div>
          </div>
        </DashboardCard>

        {/* 2-column grid: Transactions | UTXOs */}
        <div style={gridContainerStyle}>
          <TransactionsColumn
            rows={txRows}
            total={txTotal}
            page={txPage}
            pageSize={txPageSize}
            totalPages={txTotalPages}
            isLoading={txIsLoading}
            onPageChange={setTxPage}
            onPageSizeChange={(s) => {
              setTxPageSize(s);
              setTxPage(1);
            }}
            onTxClick={onTxClick}
            scrollContainerRef={txScrollRef}
          />
          <UTXOsColumn
            rows={utxoRows}
            total={utxoTotal}
            page={utxoPage}
            pageSize={utxoPageSize}
            totalPages={utxoTotalPages}
            isLoading={utxoIsLoading}
            onPageChange={setUtxoPage}
            onPageSizeChange={(s) => {
              setUtxoPageSize(s);
              setUtxoPage(1);
            }}
            onTxClick={onTxClick}
            scrollContainerRef={utxoScrollRef}
          />
        </div>
      </div>

    </div>
  );
};

// ============================================================================
// TransactionsColumn sub-component
// ============================================================================

interface TransactionsColumnProps {
  rows: AddressTx[];
  total: number;
  page: number;
  pageSize: PageSize;
  totalPages: number;
  isLoading: boolean;
  onPageChange: (page: number) => void;
  onPageSizeChange: (size: PageSize) => void;
  onTxClick: (txid: string) => void;
  // Ref on the scrollable container so the parent can reset scrollTop=0
  // when the page changes (otherwise the user lands mid-list on a fresh page).
  scrollContainerRef: React.RefObject<HTMLDivElement | null>;
}

const TransactionsColumn: React.FC<TransactionsColumnProps> = ({
  rows,
  total,
  page,
  pageSize,
  totalPages,
  isLoading,
  onPageChange,
  onPageSizeChange,
  onTxClick,
  scrollContainerRef,
}) => {
  const { t } = useTranslation('common');
  const rangeStart = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const rangeEnd = Math.min(page * pageSize, total);

  return (
    <div style={columnCardStyle}>
      <div style={columnHeaderRowStyle}>
        {t('explorer.transactions')}
      </div>

      <div style={columnScrollStyle} ref={scrollContainerRef}>
        {/* Sticky table header — 3-cell geometry mirrors the restructured
           TxRowCard: Hash+Date (flex 1, vertically stacked in row body) |
           Amount (120px right) | 32px Eye spacer. The dedicated Date column
           header was removed in m-fix-date-display-inconsistencies
           (2026-06-04) because Date now lives in a second line under each
           hash in the same flex:1 cell — the date format already carries
           its own TZ token inline, so a separate "Date (UTC)" header is
           redundant and would have to label only the lower half of every
           row. The `DateHeaderInline` component was also dropped in the
           same task (it had no remaining consumers after the column
           removal). */}
        <div style={stickyTableHeaderStyle}>
          <div style={{ ...tableHeaderCellStyle, flex: 1, minWidth: 0 }}>
            {t('explorer.hash', { defaultValue: 'Hash' })}
          </div>
          <div style={{ ...tableHeaderCellStyle, width: '120px', textAlign: 'right' }}>
            {t('explorer.amount', { defaultValue: 'Amount' })}
          </div>
          <div style={{ width: '32px' }} />
        </div>

        {/* Rows — isLoading wins over rows so stale rows from a previous page
           are cleared the moment the user clicks pagination. Without this gate
           the old page would visibly linger for the full backend walk (10–30s
           on a Dev-Fund-class address with ~1.75M tx). See
           l-fix-address-detail-stale-rows-on-pagination (2026-06-01). */}
        {isLoading ? (
          <div style={emptyStateStyle}>
            {t('explorer.loadingTransactions', { defaultValue: 'Loading transactions...' })}
          </div>
        ) : rows.length === 0 ? (
          <div style={emptyStateStyle}>
            {t('explorer.noTransactions')}
          </div>
        ) : (
          <div style={rowListStyle}>
            {rows.map((tx, idx) => (
              <TxRowCard
                key={`${tx.txid}:${idx}`}
                tx={tx}
                onTxClick={onTxClick}
              />
            ))}
          </div>
        )}
      </div>

      <PaginationFooter
        rangeStart={rangeStart}
        rangeEnd={rangeEnd}
        total={total}
        page={page}
        totalPages={totalPages}
        pageSize={pageSize}
        onPageChange={onPageChange}
        onPageSizeChange={onPageSizeChange}
        isLoading={isLoading}
      />
    </div>
  );
};

// ----- Single tx row card -----
// 4-cell flex row mirroring the sticky header geometry:
//   Hash (flex 1) | Age (90px) | Amount (120px right) | Eye (32px IconButton)
// Hash is plain non-clickable text — Copy IconButton was dropped in
// m-restyle-address-tx-utxo-columns (2026-05-29) because the Eye is the
// canonical drill-down and per-row Copy duplicated functionality already on
// the parent tx detail view.
interface TxRowCardProps {
  tx: AddressTx;
  onTxClick: (txid: string) => void;
}

const TxRowCard: React.FC<TxRowCardProps> = ({ tx, onTxClick }) => {
  const { t } = useTranslation('common');
  const { formatAmount } = useDisplayUnits();
  const { formatDateTime, formatTooltip } = useDisplayDateTime();
  return (
    <div
      style={rowCardStyle}
      onMouseEnter={(e) => {
        e.currentTarget.style.borderColor = '#444';
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.borderColor = 'transparent';
      }}
    >
      {/* Hash + Date stacked vertically in one cell so the narrow split
          column (Transactions card content ~290px when the page is split
          50/50 with Unspent Outputs) can show both at once. The flat 4-cell
          layout from m-restyle-address-tx-utxo-columns (2026-05-29) put
          Date in its own 90px column with a short `MM-DD HH:MM` form. When
          m-fix-date-display-inconsistencies (2026-06-04) switched Date to
          the full `YYYY-MM-DD HH:MM:SS GMT+N` form (220px), the Hash flex:1
          column collapsed to 0px and the hash disappeared. Stacking
          restores both — Hash on top (primary identifier), Date below
          (secondary metadata). */}
      <div style={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', gap: '2px' }}>
        <span
          style={{ ...monoCellStyle, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
          title={tx.txid}
        >
          {truncateAddress(tx.txid, 6, 4)}
        </span>
        <span
          style={{
            fontSize: '11px',
            color: '#888',
            whiteSpace: 'nowrap',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
          }}
          title={formatTooltip(tx.time)}
        >
          {formatDateTime(tx.time)}
        </span>
      </div>
      <div
        style={{
          width: '120px',
          textAlign: 'right',
          fontFamily: 'monospace',
          fontSize: '12px',
          color: tx.amount >= 0 ? '#27ae60' : '#ff6666',
          flexShrink: 0,
        }}
      >
        {tx.amount >= 0 ? '+' : ''}
        {formatAmount(Math.abs(tx.amount), false)}
      </div>
      <IconButton
        onClick={() => onTxClick(tx.txid)}
        title={t('explorer.viewTxDetails', { defaultValue: 'View transaction details' })}
        ariaLabel={t('explorer.viewTxDetails', { defaultValue: 'View transaction details' })}
        icon={<Eye size={14} />}
      />
    </div>
  );
};

// ============================================================================
// UTXOsColumn sub-component
// ============================================================================

interface UTXOsColumnProps {
  rows: AddressUTXO[];
  total: number;
  page: number;
  pageSize: PageSize;
  totalPages: number;
  isLoading: boolean;
  onPageChange: (page: number) => void;
  onPageSizeChange: (size: PageSize) => void;
  onTxClick: (txid: string) => void;
  // Ref on the scrollable container so the parent can reset scrollTop=0
  // when the page changes (mirrors TransactionsColumn).
  scrollContainerRef: React.RefObject<HTMLDivElement | null>;
}

const UTXOsColumn: React.FC<UTXOsColumnProps> = ({
  rows,
  total,
  page,
  pageSize,
  totalPages,
  isLoading,
  onPageChange,
  onPageSizeChange,
  onTxClick,
  scrollContainerRef,
}) => {
  const { t } = useTranslation('common');
  const rangeStart = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const rangeEnd = Math.min(page * pageSize, total);

  return (
    <div style={columnCardStyle}>
      <div style={columnHeaderRowStyle}>
        {t('explorer.unspentOutputs')}
      </div>

      <div style={columnScrollStyle} ref={scrollContainerRef}>
        {/* Sticky table header — 4-cell geometry must mirror UtxoRowCard:
           Outpoint (flex 1) | Confirmations (70px right) | Amount (100px right) | 32px Eye spacer.
           Confirmations + Amount intentionally narrower than the sibling Tx column
           (which uses Age 90px + Amount 120px) so the flex-1 Outpoint can fit the
           full `hash6...4:vout` content (multi-digit vout included) without CSS
           ellipsis kicking in at typical viewport widths. UTXO has no timestamp
           field, so confirmations is the canonical maturity signal (per
           m-restyle-address-tx-utxo-columns 2026-05-29). Brackets around the
           outpoint were dropped in `m-address-detail-cosmetic-fixes-and-audit`
           (2026-05-30) — see UtxoRowCard comment below for full rationale. */}
        <div style={stickyTableHeaderStyle}>
          <div style={{ ...tableHeaderCellStyle, flex: 1, minWidth: 0 }}>
            {t('explorer.outpoint', { defaultValue: 'Outpoint' })}
          </div>
          <div style={{ ...tableHeaderCellStyle, width: '70px', textAlign: 'right' }}>
            {t('explorer.confirmations', { defaultValue: 'Confirmations' })}
          </div>
          <div style={{ ...tableHeaderCellStyle, width: '100px', textAlign: 'right' }}>
            {t('explorer.amount', { defaultValue: 'Amount' })}
          </div>
          <div style={{ width: '32px' }} />
        </div>

        {/* Rows — isLoading wins over rows so stale rows from a previous page
           are cleared the moment the user clicks pagination. Symmetric with
           TransactionsColumn. See
           l-fix-address-detail-stale-rows-on-pagination (2026-06-01). */}
        {isLoading ? (
          <div style={emptyStateStyle}>
            {t('explorer.loadingUtxos', { defaultValue: 'Loading unspent outputs...' })}
          </div>
        ) : rows.length === 0 ? (
          <div style={emptyStateStyle}>
            {t('explorer.noUtxos', { defaultValue: 'No unspent outputs' })}
          </div>
        ) : (
          <div style={rowListStyle}>
            {rows.map((utxo) => (
              <UtxoRowCard
                key={`${utxo.txid}:${utxo.vout}`}
                utxo={utxo}
                onTxClick={onTxClick}
              />
            ))}
          </div>
        )}
      </div>

      <PaginationFooter
        rangeStart={rangeStart}
        rangeEnd={rangeEnd}
        total={total}
        page={page}
        totalPages={totalPages}
        pageSize={pageSize}
        onPageChange={onPageChange}
        onPageSizeChange={onPageSizeChange}
        isLoading={isLoading}
      />
    </div>
  );
};

// ----- Single UTXO row card -----
// 4-cell flex row mirroring the sticky header geometry:
//   Outpoint (flex 1) | Confirmations (70px right) | Amount (100px right) | Eye (32px IconButton)
// Outpoint is plain non-clickable text — Copy IconButton was dropped in
// m-restyle-address-tx-utxo-columns (2026-05-29) since per-row copy on UTXO
// reads as dead-weight; the Eye opens the parent tx where copy is available.
// Confirmations + Amount intentionally narrower than the sibling Tx row
// (Age 90px + Amount 120px) so the flex-1 Outpoint cell can fit the full
// `hash6...4:vout` content without CSS ellipsis clipping the multi-digit
// vout at typical viewport widths. Brackets around the outpoint were
// dropped in `m-address-detail-cosmetic-fixes-and-audit` (2026-05-30) —
// the cell is already visually distinct via positioning and monospace font.
interface UtxoRowCardProps {
  utxo: AddressUTXO;
  onTxClick: (txid: string) => void;
}

const UtxoRowCard: React.FC<UtxoRowCardProps> = ({ utxo, onTxClick }) => {
  const { t } = useTranslation('common');
  const { formatAmount } = useDisplayUnits();
  return (
    <div
      style={rowCardStyle}
      onMouseEnter={(e) => {
        e.currentTarget.style.borderColor = '#444';
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.borderColor = 'transparent';
      }}
    >
      <span
        style={{ ...monoCellStyle, flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
        title={`${utxo.txid}:${utxo.vout}`}
      >
        {truncateAddress(utxo.txid, 6, 4)}:{utxo.vout}
      </span>
      <span
        style={{
          width: '70px',
          textAlign: 'right',
          fontSize: '12px',
          color: '#ddd',
          fontVariantNumeric: 'tabular-nums',
          flexShrink: 0,
        }}
      >
        {utxo.confirmations.toLocaleString()}
      </span>
      <div
        style={{
          width: '100px',
          textAlign: 'right',
          fontFamily: 'monospace',
          fontSize: '12px',
          color: '#27ae60',
          flexShrink: 0,
        }}
      >
        {formatAmount(utxo.amount, false)}
      </div>
      <IconButton
        onClick={() => onTxClick(utxo.txid)}
        title={t('explorer.viewTxDetails', { defaultValue: 'View transaction details' })}
        ariaLabel={t('explorer.viewTxDetails', { defaultValue: 'View transaction details' })}
        icon={<Eye size={14} />}
      />
    </div>
  );
};

// ============================================================================
// PaginationFooter sub-component
// ============================================================================

interface PaginationFooterProps {
  rangeStart: number;
  rangeEnd: number;
  total: number;
  page: number;
  totalPages: number;
  pageSize: PageSize;
  onPageChange: (page: number) => void;
  onPageSizeChange: (size: PageSize) => void;
  isLoading: boolean;
}

const PaginationFooter: React.FC<PaginationFooterProps> = ({
  rangeStart,
  rangeEnd,
  total,
  page,
  totalPages,
  pageSize,
  onPageChange,
  onPageSizeChange,
  isLoading,
}) => {
  const prevDisabled = isLoading || page <= 1;
  const nextDisabled = isLoading || page >= totalPages;
  const numberFmt = useMemo(() => new Intl.NumberFormat('en-US'), []);

  return (
    <div style={paginationFooterStyle}>
      {/* Cell 1: count line (left-aligned within 1fr cell) */}
      <div>
        {numberFmt.format(rangeStart)}–{numberFmt.format(rangeEnd)} of {numberFmt.format(total)}
      </div>
      {/* Cell 2: pagination cluster (auto-sized, naturally centered between two 1fr cells) */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
        <button
          type="button"
          style={prevDisabled ? paginationButtonDisabledStyle : paginationButtonStyle}
          disabled={prevDisabled}
          onClick={() => onPageChange(page - 1)}
          aria-label="Previous page"
          title="Previous page"
        >
          <ChevronLeft size={14} />
        </button>
        <span>
          {numberFmt.format(page)} / {numberFmt.format(totalPages)}
        </span>
        <button
          type="button"
          style={nextDisabled ? paginationButtonDisabledStyle : paginationButtonStyle}
          disabled={nextDisabled}
          onClick={() => onPageChange(page + 1)}
          aria-label="Next page"
          title="Next page"
        >
          <ChevronRight size={14} />
        </button>
      </div>
      {/* Cell 3: page-size selector (right-aligned within 1fr cell).
         Uses shared RowsPerPageSelect (dark Receive listbox with keyboard
         nav + outside-click + race-safety) instead of the native <select>
         per m-restyle-address-tx-utxo-columns (2026-05-29). Matches the
         pattern already used by features/explorer/components/BlockList.tsx
         and the shared PaginationFooter primitive. */}
      <div style={{ justifySelf: 'end' }}>
        <RowsPerPageSelect<PageSize>
          value={pageSize}
          options={PAGE_SIZE_OPTIONS}
          onChange={onPageSizeChange}
        />
      </div>
    </div>
  );
};
