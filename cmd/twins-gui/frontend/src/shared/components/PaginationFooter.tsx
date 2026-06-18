import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import { RowsPerPageSelect } from './RowsPerPageSelect';

// ---------------------------------------------------------------------------
// PaginationFooter — canonical 3-zone paginated-list footer.
//
// Layout: CSS Grid `1fr auto 1fr` (count-left, pagination-center, action-right)
// with a `@media (max-width: 780px)` collapse to single-column stack so all
// three zones remain visible on narrow viewports. The grid keeps zones in
// normal flow with predictable column widths — avoids the absolute-position
// centering trick which broke under flex-wrap when zones overflowed.
//
// Internally owns:
//  - `Intl.NumberFormat('en-US')` memo for thousand-separator count formatting.
//  - Scoped `<style>` block with the Chromium spinner-arrow hide CSS
//    (`.pagination-footer-page-input::-webkit-inner-spin-button { ... }`).
//  - Page-input race-safety state: `pageInputRef`, `lastCommittedPageRef`,
//    `pageInputValue` state, sync useEffect with `document.activeElement`
//    focus guard (skip clobbering while user is typing).
//  - `handleJumpToPage` with strict `/^\d+$/` regex validation and
//    `Math.max(1, Math.min(Math.max(1, totalPages), parsed))` clamp.
//  - Prev/Next `onMouseDown` reset of `pageInputValue` to last-committed —
//    fires BEFORE the focused page-input's `onBlur` per DOM event order
//    (mousedown → blur → click), so the upcoming blur sees no diff and
//    `handleJumpToPage` short-circuits via its existing equality check.
//  - `onTriggerMouseDown` wiring on the embedded RowsPerPageSelect for the
//    same cross-control race-safety.
//
// Contract: `currentPage` is 1-based (1..totalPages). Prev/page-input/Next
// render always with disabled state when `totalPages <= 1` to preserve
// consistent footer chrome across single-page and multi-page result sets.
// The disable is driven by the existing `prevDisabled` / `nextDisabled`
// predicates (which already cover `currentPage <= 1` / `>= totalPages`)
// plus an additional `totalPages <= 1` guard on the page-input's `disabled`
// attribute so users cannot type into a non-paginated state.
// RowsPerPageSelect renders unconditionally so users can change page size
// even on a single-page result set.
// ---------------------------------------------------------------------------

export interface PaginationFooterProps<T extends number = number> {
  /** First row in the current page (1-based). */
  rangeStart: number;
  /** Last row in the current page (1-based). */
  rangeEnd: number;
  /** Number of items matching the active filter (or total when no filter). */
  total: number;
  /**
   * Pre-filter total. When defined AND `!== total`, the count line renders
   * as `X–Y of N filtered (M total)`. When omitted or equal to `total`,
   * renders the simpler `X–Y of N`.
   */
  totalUnfiltered?: number;
  /** Current page, 1-based (1..totalPages). */
  currentPage: number;
  /** Total number of pages. May be 0 (empty result) or 1 (single page). */
  totalPages: number;
  /** Called with a 1-based page number when the user navigates. */
  onPageChange: (page: number) => void;
  /** Currently selected page size. */
  pageSize: T;
  /** Allowed page-size values; passed through to RowsPerPageSelect. */
  pageSizeOptions: readonly T[];
  /** Called with the new page size when the user picks one. */
  onPageSizeChange: (size: T) => void;
  /** Disables Prev/Next/page-input while a fetch is in flight. */
  isLoading?: boolean;
  /** Right grid cell content — typically an Export button. Renders an empty
   *  placeholder div when omitted so the 3-column grid structure stays
   *  symmetric (count stays left, pagination stays centered). */
  rightSlot?: React.ReactNode;
  /**
   * Compact vertical padding for height-constrained surfaces (e.g. Receive
   * page Recent Requests card). When `true`: container padding `6px 16px`
   * (was `12px 16px`), Prev/Next button padding `4px` (was `6px`), page-input
   * padding `4px 6px` (was `6px`). All horizontal sizing, border, radius,
   * gap, font sizes, and race-safety logic are unchanged. Default `false`
   * preserves existing behavior for Transactions + Explorer BlockList consumers.
   * Added by task `l-receive-pagination-compact-height` (2026-06-02).
   */
  dense?: boolean;
}

export function PaginationFooter<T extends number = number>({
  rangeStart,
  rangeEnd,
  total,
  totalUnfiltered,
  currentPage,
  totalPages,
  onPageChange,
  pageSize,
  pageSizeOptions,
  onPageSizeChange,
  isLoading = false,
  rightSlot,
  dense = false,
}: PaginationFooterProps<T>) {
  // String-typed local state so the user can clear and retype freely. The
  // ref tracks the last value committed by either this handler or an
  // external page change so double-fire scenarios (Enter calls handler then
  // .blur() retriggers it, or Prev/Next click steals focus from a typed-
  // but-uncommitted input) collapse to one effective navigation.
  const pageInputRef = useRef<HTMLInputElement | null>(null);
  const [pageInputValue, setPageInputValue] = useState(String(currentPage));
  const lastCommittedPageRef = useRef<number>(currentPage);
  // Synchronous flag set by `onMouseDown` on Prev / Next / RowsPerPageSelect
  // trigger so the page-input's `onBlur` (which fires AFTER mousedown but
  // BEFORE the button's `onClick` per DOM event order mousedown → blur →
  // click) can detect that the blur was caused by a sibling pager click and
  // skip committing the stale typed value.
  //
  // Why a sync ref instead of just relying on `setPageInputValue` reaching
  // the blur handler: React batches state updates across event ticks, so the
  // `setPageInputValue(lastCommitted)` call from mousedown is not guaranteed
  // to have flushed by the time blur runs. The blur's `handleJumpToPage`
  // captures `pageInputValue` from its closure — the STALE pre-mousedown
  // value — and the equality short-circuit (`clamped === lastCommittedPageRef
  // .current`) does not catch this because the closure-captured value is
  // what was just typed, not what mousedown wrote. Result without the flag:
  // typing "5" + clicking Prev fires two store writes (jump-to-5 then
  // navigate-from-5), reproducing the race documented as needing back-port
  // in Transactions.tsx by the prior `m-restyle-explorer-block-list` task.
  // The shared component standardizes on the safer Explorer pattern.
  const suppressNextBlurSubmitRef = useRef(false);
  useEffect(() => {
    lastCommittedPageRef.current = currentPage;
    // Don't clobber a partial typed value while the input has focus.
    if (document.activeElement === pageInputRef.current) return;
    setPageInputValue(String(currentPage));
  }, [currentPage]);

  const handleJumpToPage = useCallback(() => {
    // Suppress the stale-blur commit when a sibling pager control caused
    // the blur (Prev / Next / RowsTrigger sets the ref synchronously in
    // its mousedown handler). Reset the draft to canonical and bail; the
    // pager button's own onClick will run next and handle navigation.
    if (suppressNextBlurSubmitRef.current) {
      suppressNextBlurSubmitRef.current = false;
      setPageInputValue(String(lastCommittedPageRef.current));
      return;
    }
    const trimmed = pageInputValue.trim();
    // Strict integer match — rejects empty / whitespace / decimals / trailing
    // garbage that parseInt would silently truncate.
    if (!/^\d+$/.test(trimmed)) {
      setPageInputValue(String(lastCommittedPageRef.current));
      return;
    }
    const parsed = parseInt(trimmed, 10);
    // Inner Math.max(1, totalPages) defends against the totalPages === 0
    // empty-result edge where Math.min(0, ...) would otherwise produce 0.
    const clamped = Math.max(1, Math.min(Math.max(1, totalPages), parsed));
    if (clamped === lastCommittedPageRef.current) {
      // Out-of-range typed value or canonicalization (e.g. "007" → "7"); snap
      // the input back to the canonical form without firing onPageChange.
      setPageInputValue(String(clamped));
      return;
    }
    lastCommittedPageRef.current = clamped;
    onPageChange(clamped);
  }, [pageInputValue, totalPages, onPageChange]);

  // Format integers with thousands separators (en-US) for the count line.
  const numberFmt = useMemo(() => new Intl.NumberFormat('en-US'), []);

  // Dynamic page-input width based on the digit count of `totalPages`.
  // Formula: `digits * 8px (12px monospace char width + safety) + 12px padding
  // + 2px border + 6px extra safety margin = digits * 8 + 20`.
  //   - 3 digits (totalPages 1..999):     44px  (unchanged — preserves the
  //                                              historic 44px from task
  //                                              `l-tx-footer-page-input-width`
  //                                              2026-05-20, no Transactions
  //                                              regression at typical page
  //                                              counts).
  //   - 4 digits (totalPages 1000..9999): 52px
  //   - 5 digits (10000..99999):           60px  (fixes the Explorer blocks
  //                                              list 5-digit truncation
  //                                              reported on 2026-06-01 at
  //                                              69,812 total pages).
  //   - 6 digits (100000..999999):         68px
  //   - 7 digits (1000000..9999999):       76px
  // The `Math.max(1, totalPages)` defends against the `totalPages === 0`
  // empty-result edge — defense-in-depth (the existing `handleJumpToPage`
  // clamp already uses the same guard on the parsed value), keeps the
  // input's minimum width at 1 digit (28px) instead of degenerating to
  // 0px when an upstream consumer briefly renders an empty-result footer.
  const pageInputWidth = useMemo(() => {
    const digits = String(Math.max(1, totalPages)).length;
    return `${digits * 8 + 20}px`;
  }, [totalPages]);

  // Set the suppress flag synchronously AND reset the input draft so the
  // upcoming blur (a) sees the suppress flag and skips committing, and (b)
  // visually snaps back to canonical even if React batches the state update
  // past the blur frame. Belt-and-suspenders — either branch alone would be
  // a race; both together close the window.
  const resetPageInput = useCallback(() => {
    suppressNextBlurSubmitRef.current = true;
    setPageInputValue(String(lastCommittedPageRef.current));
  }, []);

  const isFiltered =
    totalUnfiltered !== undefined && totalUnfiltered !== total;

  const countText =
    total > 0
      ? isFiltered
        ? `${numberFmt.format(rangeStart)}–${numberFmt.format(rangeEnd)} of ${numberFmt.format(total)} filtered (${numberFmt.format(totalUnfiltered as number)} total)`
        : `${numberFmt.format(rangeStart)}–${numberFmt.format(rangeEnd)} of ${numberFmt.format(total)}`
      : isFiltered
        ? `0 of ${numberFmt.format(totalUnfiltered as number)}`
        : '0';

  const prevDisabled = currentPage <= 1 || isLoading;
  const nextDisabled = currentPage >= totalPages || isLoading;

  return (
    <div
      className="pagination-footer-grid"
      style={{
        padding: dense ? '6px 16px' : '12px 16px',
        border: '1px solid #3a3a3a',
        borderRadius: '8px',
        backgroundColor: '#2f2f2f',
        display: 'grid',
        gridTemplateColumns: '1fr auto 1fr',
        alignItems: 'center',
        gap: '24px',
      }}
    >
      {/* Scoped CSS — hides the native number-input spinner arrows on
          Chromium (WebKit pseudo-elements can't be set via inline style)
          and on Firefox (-moz-appearance: textfield). Also defines the
          narrow-window media query that collapses the 3-column grid to a
          single left-aligned column. */}
      <style>{`
        .pagination-footer-page-input::-webkit-inner-spin-button,
        .pagination-footer-page-input::-webkit-outer-spin-button {
          -webkit-appearance: none;
          margin: 0;
        }
        .pagination-footer-page-input { -moz-appearance: textfield; appearance: textfield; }
        @media (max-width: 780px) {
          .pagination-footer-grid {
            grid-template-columns: 1fr;
            row-gap: 12px;
          }
          .pagination-footer-grid > * {
            justify-self: start !important;
          }
        }
      `}</style>

      {/* Left cell: row-count summary. */}
      <div
        style={{
          fontSize: '12px',
          color: '#ddd',
          justifySelf: 'start',
          display: 'flex',
          alignItems: 'center',
          flexWrap: 'wrap',
          gap: '8px',
          rowGap: '6px',
          minWidth: 0,
        }}
      >
        <span>{countText}</span>
      </div>

      {/* Center cell: pagination cluster + RowsPerPageSelect. The page-nav
          buttons (Prev / page input / Next) render always; the existing
          `prevDisabled` / `nextDisabled` predicates naturally disable them
          when totalPages <= 1 (since currentPage is clamped to 1 in that
          case, both `currentPage <= 1` and `currentPage >= totalPages` are
          true). The page-input gains a `totalPages <= 1` guard on its
          `disabled` attribute so the user cannot type into a non-paginated
          state. The Rows selector renders unconditionally so users can
          change page size even on a single-page result set. */}
      <div style={{ justifySelf: 'center' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
          <button
            type="button"
            onClick={() => onPageChange(currentPage - 1)}
            onMouseDown={resetPageInput}
            disabled={prevDisabled}
            style={{
              padding: dense ? '4px' : '6px',
              backgroundColor: '#383838',
              border: '1px solid #4a4a4a',
              borderRadius: '6px',
              color: '#ccc',
              opacity: prevDisabled ? 0.5 : 1,
              cursor: prevDisabled ? 'not-allowed' : 'pointer',
              display: 'flex',
              alignItems: 'center',
            }}
            title="Previous page"
            aria-label="Previous page"
          >
            <ChevronLeft size={14} />
          </button>

          <span
            style={{
              fontSize: '12px',
              color: '#ddd',
              display: 'flex',
              alignItems: 'center',
              gap: '6px',
            }}
          >
            <input
              ref={pageInputRef}
              className="pagination-footer-page-input"
              type="number"
              min={1}
              max={totalPages}
              value={pageInputValue}
              onChange={(e) => setPageInputValue(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  handleJumpToPage();
                  (e.currentTarget as HTMLInputElement).blur();
                } else if (e.key === 'Escape') {
                  // Reset to last-committed (mirrors the regex-reject
                  // path in handleJumpToPage). Using the ref instead of
                  // `currentPage` keeps Escape correct even if an async
                  // store update has not yet propagated.
                  setPageInputValue(String(lastCommittedPageRef.current));
                  (e.currentTarget as HTMLInputElement).blur();
                }
              }}
              onBlur={handleJumpToPage}
              disabled={isLoading || totalPages <= 1}
              style={{
                width: pageInputWidth,
                padding: dense ? '4px 6px' : '6px',
                fontSize: '12px',
                textAlign: 'center',
                backgroundColor: '#252525',
                border: '1px solid #3a3a3a',
                borderRadius: '6px',
                color: '#ddd',
                outline: 'none',
              }}
              title="Type a page number and press Enter to jump"
              aria-label={`Page ${currentPage} of ${totalPages}`}
            />
            <span style={{ color: '#888' }}>
              / {numberFmt.format(totalPages)}
            </span>
          </span>

          <button
            type="button"
            onClick={() => onPageChange(currentPage + 1)}
            onMouseDown={resetPageInput}
            disabled={nextDisabled}
            style={{
              padding: dense ? '4px' : '6px',
              backgroundColor: '#383838',
              border: '1px solid #4a4a4a',
              borderRadius: '6px',
              color: '#ccc',
              opacity: nextDisabled ? 0.5 : 1,
              cursor: nextDisabled ? 'not-allowed' : 'pointer',
              display: 'flex',
              alignItems: 'center',
            }}
            title="Next page"
            aria-label="Next page"
          >
            <ChevronRight size={14} />
          </button>

          <RowsPerPageSelect
            value={pageSize}
            options={pageSizeOptions}
            onChange={onPageSizeChange}
            onTriggerMouseDown={resetPageInput}
          />
        </div>
      </div>

      {/* Right cell: optional action slot (Export on Transactions, empty on
          Explorer). Renders an empty <div /> placeholder when omitted so the
          3-column grid keeps the center cluster optically centered. */}
      <div
        style={{
          justifySelf: 'end',
          display: 'flex',
          alignItems: 'center',
          gap: '8px',
        }}
      >
        {rightSlot}
      </div>
    </div>
  );
}
