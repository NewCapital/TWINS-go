import React, { useState, useEffect, useRef, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { Check, Copy, Eye } from 'lucide-react';
import type { BlockSummary } from '@/store/slices/explorerSlice';
import {
  BLOCKS_PER_PAGE_OPTIONS,
  type BlocksPerPageOption,
} from '@/store/slices/explorerSlice';
import { PaginationFooter } from '@/shared/components/PaginationFooter';
import { IconButton } from '@/shared/components/IconButton';
import { RefreshCountdown } from '@/shared/components/RefreshCountdown';
import { StatusPill } from '@/shared/components/StatusPill';
import { truncateAddress } from '@/shared/utils/format';
import { writeToClipboard } from '@/shared/utils/clipboard';
import { useDisplayUnits } from '@/shared/hooks/useDisplayUnits';
import { useDisplayDateTime } from '@/shared/hooks/useDisplayDateTime';
import { SearchBar } from './SearchBar';
import { cardStyle } from '../pages/ExplorerPage';

// Auto-refresh polling interval (seconds) for the latest-blocks list.
// Mirrors BlockDetail/TransactionDetail/AddressView convention (all 60s).
// TWINS PoS produces ~1 block every 60-120s on average, so 60s sampling
// delivers approximately one block of freshness per tick without
// doubling RPC load on the daemon. Wired into <RefreshCountdown total>
// and into the interval useEffect reset / fire path.
const REFRESH_INTERVAL_SECONDS = 60;

// Module-level column-width tokens. Both the sticky header row and each
// per-block row consume this constant so the columns can never drift apart.
// Layout (m-explorer-blocks-table-redesign 2026-05-28 round 5): ALL cells
// FIXED-width including Hash. Row container uses justifyContent:
// 'space-between' to distribute leftover slack uniformly across all 6
// inter-cell gaps. Hash was content-width in round 4 (different content
// in header "Hash" vs row "abc…def + CopyIcon") which broke vertical
// alignment because space-between divides slack equally based on each
// row's residual width — the differing Hash widths produced cumulative
// drift (~120px) between header and row columns. Round 5 fixes Hash at
// 145px (fits the 6+6 truncated hash ~109px + Copy IconButton 24px +
// gap 6px + safety) so header and row geometries are identical and
// columns align perfectly. Reward 140px fits µTWINS values up to
// 100,000,000.00 (~14 chars at 14px monospace).
const COL = {
  height: '90px',
  hash: '145px',
  type: '50px',
  reward: '140px',
  // Widened from 80px to 220px in m-fix-date-display-inconsistencies
  // (2026-06-04) so the column fits the full `YYYY-MM-DD HH:MM:SS GMT+N`
  // (Local) or `YYYY-MM-DD HH:MM:SS UTC` (UTC) form from
  // useDisplayDateTime.formatDateTime. Age mode still fits in this width
  // (e.g. `5m ago`, `<1m ago`, `Xy Ymo ago`).
  age: '220px',
  txCount: '50px',
  eye: '32px',
} as const;

// The local `formatAgeShort` helper was removed in
// m-fix-date-display-inconsistencies (2026-06-04) — the Age column now
// renders via the global `useDisplayDateTime.formatDateTime` so the
// per-row value respects the user's Local/UTC/Age preference.

interface BlockListProps {
  blocks: BlockSummary[];
  isLoading: boolean;
  currentPage: number;
  totalBlocks: number;
  blocksPerPage: BlocksPerPageOption;
  onBlockClick: (query: string) => void;
  onPageChange: (page: number) => void;
  onBlocksPerPageChange: (size: BlocksPerPageOption) => void;
  // SearchBar + Refresh props (moved from ExplorerPage shell into BlockList
  // as part of m-explorer-search-bar-scope-to-blocklist so search is visible
  // only on the BlockList view). onRefresh signature widened in
  // m-explorer-blocklist-refresh-countdown-and-autopoll (2026-06-03) to
  // accept an optional silent flag — the 60s auto-poll tick fires
  // onRefresh({silent:true}) to skip the loading flag toggle (so the table
  // doesn't blank on every tick). Manual click via handleManualRefresh
  // calls onRefresh() with no options (foreground path).
  searchQuery: string;
  isSearching: boolean;
  onSearch: (query: string) => void;
  onSearchChange: (value: string) => void;
  onRefresh: (options?: { silent?: boolean }) => Promise<void> | void;
  isAnyLoading: boolean;
}


// ---------------------------------------------------------------------------
// BlockRow — single block as a Receive-style row card. Hover-border via
// inline mouseEnter/Leave (zero-re-render convention matching every other
// row-card list in the codebase).
// ---------------------------------------------------------------------------
interface BlockRowProps {
  block: BlockSummary;
  onClick: (hash: string) => void;
}

const BlockRow: React.FC<BlockRowProps> = ({ block, onClick }) => {
  const { t } = useTranslation('common');
  const { formatAmount } = useDisplayUnits();
  const [copied, setCopied] = React.useState(false);
  const copyTimerRef = React.useRef<ReturnType<typeof setTimeout> | null>(null);

  // Cleanup pending copy timer on unmount so we never write state to a torn-down row
  // (e.g. user paginates while the 2s feedback window is open).
  React.useEffect(() => {
    return () => {
      if (copyTimerRef.current) clearTimeout(copyTimerRef.current);
    };
  }, []);

  const handleCopyHash = React.useCallback(async () => {
    const ok = await writeToClipboard(block.hash);
    if (!ok) return;
    if (copyTimerRef.current) clearTimeout(copyTimerRef.current);
    setCopied(true);
    copyTimerRef.current = setTimeout(() => {
      setCopied(false);
      copyTimerRef.current = null;
    }, 2000);
  }, [block.hash]);

  const { formatDateValue, formatTooltip } = useDisplayDateTime();
  // Age column value: no TZ suffix (column header carries the TZ via
  // formatDateHeader()). The tooltip below still uses formatTooltip so users
  // can hover-disambiguate Local vs UTC by seeing the opposite representation
  // with its TZ token intact. See l-date-display-suffix-cleanup (2026-06-04).
  const formattedAge = formatDateValue(block.time);
  const formattedTimeLocal = formatTooltip(block.time);
  const truncatedHash = React.useMemo(
    () => truncateAddress(block.hash, 6, 6),
    [block.hash]
  );
  const formattedHeight = React.useMemo(
    () => block.height.toLocaleString(),
    [block.height]
  );

  return (
    <div
      // Row click is intentionally a no-op — the trailing Eye <IconButton> is
      // the sole open-details affordance, matching the Transactions page
      // pattern. Rows are flat inside the merged table card; hover-bg flip
      // provides subtle visual feedback.
      onMouseEnter={(e) => {
        e.currentTarget.style.borderColor = '#444';
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.borderColor = 'transparent';
      }}
      style={{
        display: 'flex',
        gap: '12px',
        justifyContent: 'space-between',
        alignItems: 'center',
        padding: '10px 12px',
        backgroundColor: '#2a2a2a',
        border: '1px solid transparent',
        borderRadius: '6px',
        cursor: 'default',
        transition: 'border-color 0.15s',
        outline: 'none',
      }}
    >
      {/* Height — rendered in primary text color (#ddd) matching the rest of
          the row, with locale-formatted thousands separators (e.g. 1,740,549)
          mirroring how transaction amounts are formatted. */}
      <div
        style={{
          width: COL.height,
          flexShrink: 0,
          color: '#ddd',
          fontSize: '14px',
        }}
      >
        {formattedHeight}
      </div>

      {/* Hash — content-width (flex: 0 0 auto) hosting the 6+6 truncated
          mono value plus an inline Copy IconButton. Round 2 of the redesign
          flipped this cell from flex: 1 to content-width so the leftover
          horizontal space is absorbed by the new Reward column (right-aligned
          numeric) instead of producing a ~490px dead band between Hash and
          Age. Copy IconButton swaps to a green Check for 2s on successful
          copy (matches BlockDetail.tsx copy-feedback convention). */}
      <div
        style={{
          width: COL.hash,
          flexShrink: 0,
          display: 'flex',
          alignItems: 'center',
          gap: '6px',
        }}
      >
        <span
          title={block.hash}
          style={{
            color: '#ddd',
            fontSize: '14px',
            fontFamily: 'monospace',
            whiteSpace: 'nowrap',
          }}
        >
          {truncatedHash}
        </span>
        <IconButton
          size={24}
          icon={
            copied ? (
              <Check size={12} color="#27ae60" />
            ) : (
              <Copy size={12} />
            )
          }
          title={t('explorer.copyHash')}
          ariaLabel={t('explorer.copyHash')}
          onClick={handleCopyHash}
        />
      </div>

      {/* Type — compact StatusPill (PoS success-green / PoW neutral-grey).
          No marginLeft: 'auto' — round 4 dropped the auto-margin because
          it produced one large dead band; the row container's
          justifyContent: 'space-between' now distributes slack uniformly
          across all 6 inter-cell gaps so no single gap reads as broken
          alignment. */}
      <div
        style={{
          width: COL.type,
          flexShrink: 0,
          display: 'flex',
          alignItems: 'center',
        }}
      >
        <StatusPill
          tone={block.is_pos ? 'success' : 'neutral'}
          label={block.is_pos ? 'PoS' : 'PoW'}
        />
      </div>

      {/* Reward — fixed width 110px, right-aligned, monospace 14px 600
          #27ae60 matching the Transactions Amount column tokens (m-align-
          tx-row-design-overview-and-transactions 2026-05-20). Block rewards
          are always non-negative; the leading "+" that formatAmount adds
          for positives is stripped to avoid every row carrying a redundant
          "+". The unit (TWINS / mTWINS / µTWINS) lives once in the column
          header via the unitLabel hoist (m-tx-list-unit-once-in-column-
          header 2026-05-27 pattern), not per-row. 110px comfortably fits
          values up to ~7 integer digits at 14px monospace. */}
      <div
        style={{
          width: COL.reward,
          flexShrink: 0,
          color: '#27ae60',
          fontSize: '14px',
          fontFamily: 'monospace',
          fontWeight: 600,
          textAlign: 'right',
          whiteSpace: 'nowrap',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
        }}
      >
        {formatAmount(block.reward, false).replace(/^\+/, '')}
      </div>

      {/* Age — rendered via the global useDisplayDateTime.formatDateTime so
          the per-row value respects the user's selected Local/UTC/Age
          mode. 220px fits the widest variant (`YYYY-MM-DD HH:MM:SS GMT+N`
          local form ≈ 200px at 14px monospace). Tooltip is the inverse
          representation via formatTooltip (Local→UTC, UTC→Local, Age→UTC). */}
      <div
        title={formattedTimeLocal}
        style={{
          width: COL.age,
          flexShrink: 0,
          color: '#ddd',
          fontSize: '14px',
          whiteSpace: 'nowrap',
        }}
      >
        {formattedAge}
      </div>

      {/* Tx count — centered for parity with header alignment. */}
      <div
        style={{
          width: COL.txCount,
          flexShrink: 0,
          color: '#ddd',
          fontSize: '14px',
          textAlign: 'center',
        }}
      >
        {block.tx_count}
      </div>

      {/* Eye — sole open-details affordance; matches the Transactions page
          trailing-Eye column convention. */}
      <div
        style={{
          width: COL.eye,
          flexShrink: 0,
          display: 'flex',
          justifyContent: 'center',
        }}
      >
        <IconButton
          size={26}
          icon={<Eye size={14} />}
          title={t('explorer.viewBlockDetails')}
          ariaLabel={t('explorer.viewBlockDetails')}
          onClick={() => onClick(block.hash)}
        />
      </div>
    </div>
  );
};

// ---------------------------------------------------------------------------
// SkeletonRow — same row-card structure as a live block row, but with grey
// `animate-pulse` placeholders sized to match each column. Keeps the
// loading state visually stable (no layout shift between skeleton and
// first real fetch).
// ---------------------------------------------------------------------------
const SkeletonRow: React.FC = () => (
  <div
    style={{
      display: 'flex',
      gap: '12px',
      justifyContent: 'space-between',
      alignItems: 'center',
      padding: '10px 12px',
      backgroundColor: '#2a2a2a',
      border: '1px solid transparent',
      borderRadius: '6px',
    }}
  >
    <div style={{ width: COL.height, flexShrink: 0 }}>
      <div
        className="animate-pulse"
        style={{
          height: '14px',
          width: '60%',
          backgroundColor: '#3a3a3a',
          borderRadius: '4px',
        }}
      />
    </div>
    {/* Hash placeholder — sized to COL.hash so skeleton column geometry
        matches live row column geometry (load-bearing for header-row
        alignment under justifyContent: space-between). */}
    <div style={{ width: COL.hash, flexShrink: 0 }}>
      <div
        className="animate-pulse"
        style={{
          height: '14px',
          width: '140px',
          backgroundColor: '#3a3a3a',
          borderRadius: '4px',
        }}
      />
    </div>
    {/* Type placeholder — pill-shaped (999px radius) to match the live
        StatusPill chrome. */}
    <div style={{ width: COL.type, flexShrink: 0 }}>
      <div
        className="animate-pulse"
        style={{
          height: '14px',
          width: '36px',
          backgroundColor: '#3a3a3a',
          borderRadius: '999px',
        }}
      />
    </div>
    {/* Reward placeholder — fixed-width to mirror the live numeric column. */}
    <div style={{ width: COL.reward, flexShrink: 0 }}>
      <div
        className="animate-pulse"
        style={{
          height: '14px',
          width: '60px',
          marginLeft: 'auto',
          backgroundColor: '#3a3a3a',
          borderRadius: '4px',
        }}
      />
    </div>
    <div style={{ width: COL.age, flexShrink: 0 }}>
      <div
        className="animate-pulse"
        style={{
          height: '14px',
          width: '60%',
          backgroundColor: '#3a3a3a',
          borderRadius: '4px',
        }}
      />
    </div>
    <div style={{ width: COL.txCount, flexShrink: 0 }}>
      <div
        className="animate-pulse"
        style={{
          height: '14px',
          width: '40%',
          margin: '0 auto',
          backgroundColor: '#3a3a3a',
          borderRadius: '4px',
        }}
      />
    </div>
    {/* Eye-column placeholder so skeleton rows match the live row layout. */}
    <div
      style={{
        width: COL.eye,
        flexShrink: 0,
        display: 'flex',
        justifyContent: 'center',
      }}
    >
      <div
        className="animate-pulse"
        style={{
          height: '14px',
          width: '14px',
          backgroundColor: '#3a3a3a',
          borderRadius: '4px',
        }}
      />
    </div>
  </div>
);

// ---------------------------------------------------------------------------
// BlockList — Explorer block-list view with Receive-design-language tokens.
// ---------------------------------------------------------------------------
export const BlockList: React.FC<BlockListProps> = ({
  blocks,
  isLoading,
  currentPage,
  totalBlocks,
  blocksPerPage,
  onBlockClick,
  onPageChange,
  onBlocksPerPageChange,
  searchQuery,
  isSearching,
  onSearch,
  onSearchChange,
  onRefresh,
  isAnyLoading,
}) => {
  const { t } = useTranslation('common');
  const { unitLabel } = useDisplayUnits();
  const { formatDateHeader } = useDisplayDateTime();
  const totalPages = Math.ceil(totalBlocks / blocksPerPage);

  // Range for the count line. `currentPage` is 0-indexed in this slice; the
  // shared <PaginationFooter> contract is 1-based, so we convert at the prop
  // boundary below. Page-input race-safety, jump-to-page handler, scoped
  // Chromium spinner CSS, and Intl-formatted count line all live inside
  // <PaginationFooter>.
  const rangeStart = totalBlocks > 0 ? currentPage * blocksPerPage + 1 : 0;
  const rangeEnd = Math.min((currentPage + 1) * blocksPerPage, totalBlocks);

  // ===========================================================================
  // 60s auto-refresh polling, page-gated to currentPage === 0
  // (m-explorer-blocklist-refresh-countdown-and-autopoll 2026-06-03)
  // ===========================================================================
  //
  // Reference implementation: BlockDetail.tsx auto-refresh, hardened across 13
  // rounds of parallel review in m-block-detail-auto-refresh-and-nav-pills
  // (2026-05-26). The race-safety triad is mirror-copied verbatim:
  //   * countdownRef / onRefreshRef — sync refs so the interval callback reads
  //     the LATEST state/prop at tick time rather than the closure-captured
  //     value (which would be stale after every re-render).
  //   * silentInFlightRef — prevents tick-stacking when a silent RPC exceeds
  //     the 60s interval. Set true when dispatching a silent fetch, cleared
  //     in the fetch's .finally().
  //   * silentTokenRef — invalidates a stale .finally() from a prior page's
  //     in-flight silent fetch. Bumped on every [currentPage] change; the
  //     .finally() only clears silentInFlightRef when its captured token
  //     still matches.
  // Existing fetchSeqRef counter on ExplorerPage.fetchBlocks completes the
  // race-safety: stale silent responses cannot overwrite a newer manual fetch
  // or page-change fetch.
  //
  // Page-gating: the interval callback early-returns when currentPage !== 0,
  // so on pages > 0 the countdown holds at REFRESH_INTERVAL_SECONDS without
  // ticking. The ring stays visible and manual click via handleManualRefresh
  // still works. When the user navigates back to page 0, the [currentPage]
  // reset effect restarts the countdown.
  const [countdown, setCountdown] = useState(REFRESH_INTERVAL_SECONDS);
  const countdownRef = useRef(countdown);
  const onRefreshRef = useRef(onRefresh);
  const silentInFlightRef = useRef(false);
  const silentTokenRef = useRef(0);
  // Ref to the inner scroll container (the `overflow: 'auto'` div that holds
  // the sticky column header + row list, ~line 592 below). Consumed by the
  // scroll-reset effect added in l-fix-explorer-blocks-pagination-loading-flicker
  // (2026-06-04, scope-extension round) — see the dedicated effect block
  // immediately after the existing [currentPage] reset effect for the
  // rationale and precedent cross-references.
  const tableScrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => { countdownRef.current = countdown; }, [countdown]);
  useEffect(() => { onRefreshRef.current = onRefresh; }, [onRefresh]);

  // Reset effect: on every page change (a) reset the countdown so the user
  // gets a fresh 60s window, (b) bump silentTokenRef so any in-flight
  // .finally() from the prior page becomes a no-op when it resolves (cannot
  // clear the lock for a newer fetch on the new page), and (c) defensively
  // clear silentInFlightRef so the next page-0 tick is not held off by a
  // stale lock.
  useEffect(() => {
    setCountdown(REFRESH_INTERVAL_SECONDS);
    silentTokenRef.current += 1;
    silentInFlightRef.current = false;
  }, [currentPage]);

  // Scroll-reset effect (l-fix-explorer-blocks-pagination-loading-flicker
  // 2026-06-04 scope-extension round): on every page change, reset the inner
  // scroll container's scrollTop to 0 so the user lands on the first row of
  // the freshly-fetched page rather than mid-list. Without this, the prior
  // page's scroll position persists into the new page — visible (and
  // jarring) AFTER the same task's anti-flicker fix kept the prior rows on
  // screen during refetch. Canonical precedent: Transactions.tsx scroll-reset
  // effect via tableScrollRef (l-tx-table-sort-highlight-and-scroll-reset
  // 2026-05-20) and AddressView.tsx per-column resets via txScrollRef /
  // utxoScrollRef (m-restyle-address-tx-utxo-columns 2026-05-29).
  // Deliberately NOT keyed on [blocksPerPage] — the slice's setBlocksPerPage
  // already atomically resets blocksPage to 0 and clears blocks, so the
  // [currentPage] dep above catches page-size changes that cross a
  // page-0-to-page-N boundary; same documented U7 trade-off the Transactions
  // scroll-reset locked in (see "Transactions Table: Active Sort Column
  // Highlight + Scroll Reset on Sort/Filter/Page Change" 2026-05-20
  // Recent Changes entry in cmd/twins-gui/frontend/CLAUDE.md).
  useEffect(() => {
    if (tableScrollRef.current) {
      tableScrollRef.current.scrollTop = 0;
    }
  }, [currentPage]);

  // Interval effect: 1-second tick. Four guards make this safe:
  //   1. currentPage !== 0  → no auto-fetch on pages > 0 (would shift
  //      pagination by overwriting the user's page-N view with page-0 data).
  //   2. isAnyLoading       → don't fire while a manual or seq-counter-driven
  //      fetch is already in flight (avoids redundant work and visible flash).
  //   3. silentInFlightRef  → prevents tick-stacking when an RPC exceeds the
  //      60s interval (a second tick would dispatch a second silent fetch
  //      while the first is unresolved).
  //   4. ExplorerPage.fetchBlocks fetchSeqRef counter (already in place) →
  //      any stale silent response that resolves AFTER a newer fetch landed
  //      bails on its commit and finally branches.
  // Promise.resolve(...) wrapper handles both Promise<void> AND void return
  // values from onRefreshRef.current?.({ silent: true }).
  useEffect(() => {
    const interval = setInterval(() => {
      if (currentPage !== 0 || isAnyLoading || silentInFlightRef.current === true) {
        return;
      }
      if (countdownRef.current <= 1) {
        const myToken = silentTokenRef.current;
        silentInFlightRef.current = true;
        Promise.resolve(onRefreshRef.current?.({ silent: true })).finally(() => {
          if (silentTokenRef.current === myToken) {
            silentInFlightRef.current = false;
          }
        });
        setCountdown(REFRESH_INTERVAL_SECONDS);
      } else {
        setCountdown((prev) => prev - 1);
      }
    }, 1000);
    return () => clearInterval(interval);
  }, [currentPage, isAnyLoading]);

  // Manual refresh: take the foreground path (no silent option) so the
  // loading flag surfaces visually, and reset the countdown so the user gets
  // a fresh window after pressing refresh. Wired into the <RefreshCountdown>
  // mode="interactive" ring below.
  const handleManualRefresh = useCallback(() => {
    setCountdown(REFRESH_INTERVAL_SECONDS);
    onRefresh();
  }, [onRefresh]);

  return (
    <div
      style={{
        flex: 1,
        minHeight: 0,
        display: 'flex',
        flexDirection: 'column',
        gap: '12px',
      }}
    >
      {/* SearchBar — Receive-design card. Moved from ExplorerPage shell as
          part of m-explorer-search-bar-scope-to-blocklist so search is visible
          only on the BlockList view; detail sub-views render without it. The
          RefreshCountdown ring lived here briefly (2026-06-03) but moved into
          the sticky table header's Eye column as part of
          l-explorer-blocks-refresh-ring-in-table-header so the SearchBar can
          own the full width of its card. */}
      <div style={cardStyle}>
        <SearchBar
          value={searchQuery}
          isSearching={isSearching}
          onSearch={onSearch}
          onChange={onSearchChange}
        />
      </div>

      {/* Scroll container — wraps the sticky header + the row list, NOT the
          footer. Decoupling the footer from the scroll container keeps the
          pagination controls (Prev / Next / jump-input / RowsPerPageSelect)
          pinned at the bottom of the page regardless of page size — the
          original BlockList kept this property; Codex round-4 P2 found that
          collapsing the outer to a single scrollable container hides the
          footer at page sizes 100/250. The header stays sticky INSIDE this
          inner wrapper as the spec mandates (criterion line 38).
          ref={tableScrollRef} added in
          l-fix-explorer-blocks-pagination-loading-flicker (2026-06-04
          scope-extension round): wires this container to the page-change
          scroll-reset effect declared above so user lands on row 1 of the
          new page rather than mid-list. */}
      <div
        ref={tableScrollRef}
        style={{
          flex: 1,
          minHeight: 0,
          overflow: 'auto',
          display: 'flex',
          flexDirection: 'column',
          backgroundColor: '#2f2f2f',
          border: '1px solid #3a3a3a',
          borderRadius: '8px',
        }}
      >
      {/* Sticky column header inside the scroll container so it stays
          visible while the rows scroll. Header uses the Receive form-label
          tokens (`11px 500 #888`, sentence-case — uppercase + letterSpacing
          dropped in `l-explorer-blocks-table-parity-with-transactions` to
          match the Transactions page SortableHeader convention). */}
      <div
        style={{
          display: 'flex',
          gap: '12px',
          justifyContent: 'space-between',
          alignItems: 'center',
          padding: '10px 12px',
          position: 'sticky',
          top: 0,
          zIndex: 10,
          backgroundColor: '#2f2f2f',
          borderBottom: '1px solid #3a3a3a',
        }}
      >
        <div
          style={{
            width: COL.height,
            flexShrink: 0,
            fontSize: '11px',
            fontWeight: 500,
            color: '#888',
          }}
        >
          {t('explorer.height')}
        </div>
        <div
          style={{
            width: COL.hash,
            flexShrink: 0,
            fontSize: '11px',
            fontWeight: 500,
            color: '#888',
          }}
        >
          {t('explorer.hash')}
        </div>
        <div
          style={{
            width: COL.type,
            flexShrink: 0,
            fontSize: '11px',
            fontWeight: 500,
            color: '#888',
          }}
        >
          {t('explorer.type')}
        </div>
        <div
          style={{
            width: COL.reward,
            flexShrink: 0,
            fontSize: '11px',
            fontWeight: 500,
            color: '#888',
            textAlign: 'right',
          }}
        >
          {t('explorer.rewardWithUnit', { unit: unitLabel })}
        </div>
        <div
          style={{
            width: COL.age,
            flexShrink: 0,
            fontSize: '11px',
            fontWeight: 500,
            color: '#888',
          }}
        >
          {formatDateHeader()}
        </div>
        <div
          style={{
            width: COL.txCount,
            flexShrink: 0,
            fontSize: '11px',
            fontWeight: 500,
            color: '#888',
            textAlign: 'center',
          }}
        >
          {t('explorer.txCount')}
        </div>
        {/* Eye-column cell hosts the <RefreshCountdown> ring (60s auto-poll on
            page 0 + click to refresh on any page). Moved here from the SearchBar
            card as part of l-explorer-blocks-refresh-ring-in-table-header so the
            SearchBar owns the full width of its card AND the ring stays visible
            during scroll thanks to the parent sticky header. size={26} matches
            the row-level Eye <IconButton size={26}> for visual parity (ring sits
            directly above the row icons). All countdown wiring (countdown state,
            handleManualRefresh, auto-poll useEffect, race-safety refs) lives in
            the parent BlockList component and is unchanged. */}
        <div
          style={{
            width: COL.eye,
            flexShrink: 0,
            display: 'flex',
            justifyContent: 'center',
          }}
        >
          <RefreshCountdown
            mode="interactive"
            countdown={countdown}
            total={REFRESH_INTERVAL_SECONDS}
            onRefresh={handleManualRefresh}
            isLoading={isAnyLoading}
            size={26}
          />
        </div>
      </div>

      {/* Row list — row-cards stacked inside the merged table card with a
          small gap so each row reads as its own card (matches the
          Transactions table convention). */}
      <div
        style={{
          display: 'flex',
          flexDirection: 'column',
          gap: '4px',
          padding: '8px 8px 12px 8px',
        }}
      >
        {/* Anti-flicker render switch (l-fix-explorer-blocks-pagination-loading-flicker
            2026-06-04): skeletons render only on truly-empty first load
            (isLoading && blocks.length === 0). On page switch the slice keeps
            the prior page's rows visible during the in-flight refetch instead
            of rendering a `Loading blocks...` text placeholder — which used to
            cause a visible row → text → row blanking on every Prev/Next/page-
            number click. The existing fetchSeqRef monotonic counter in
            ExplorerPage.fetchBlocks already guarantees only the newest fetch's
            response commits, so the stale-data window during refetch is
            race-safe by construction. Mirrors the canonical anti-flicker
            pattern established by Transactions.tsx (since 2026-05-20
            m-tx-table-context-menu-and-antiflicker) and
            ReceivingAddressesDialog.tsx (since 2026-04-08). */}
        {isLoading && blocks.length === 0 ? (
          // Skeleton state — same chrome as a real row but with placeholders.
          // Show 5 to give the user a sense of the table density.
          <>
            <SkeletonRow />
            <SkeletonRow />
            <SkeletonRow />
            <SkeletonRow />
            <SkeletonRow />
          </>
        ) : blocks.length === 0 ? (
          <div
            style={{
              padding: '32px',
              textAlign: 'center',
              color: '#888',
              fontSize: '12px',
            }}
          >
            {t('explorer.noBlocks')}
          </div>
        ) : (
          blocks.map((block) => (
            <BlockRow key={block.hash} block={block} onClick={onBlockClick} />
          ))
        )}
      </div>

      </div>
      {/* Footer — shared <PaginationFooter> lives OUTSIDE the scroll container
          so the pagination controls stay pinned at the bottom of the view
          regardless of how many rows the user is paging through. The shared
          component owns the 3-zone CSS Grid (count-left, pagination-center,
          right-slot empty here), narrow-window media query, race-safety state
          machine, scoped Chromium spinner CSS, and Intl-formatted count line.
          Page-index conversion at the prop boundary: slice is 0-based,
          PaginationFooter contract is 1-based. */}
      <PaginationFooter
        rangeStart={rangeStart}
        rangeEnd={rangeEnd}
        total={totalBlocks}
        currentPage={currentPage + 1}
        totalPages={totalPages}
        onPageChange={(page) => onPageChange(page - 1)}
        pageSize={blocksPerPage}
        pageSizeOptions={BLOCKS_PER_PAGE_OPTIONS}
        onPageSizeChange={onBlocksPerPageChange}
        isLoading={isLoading}
      />
    </div>
  );
};
