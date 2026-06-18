/**
 * Utility for comparing current Transactions filter+sort state against a
 * saved view snapshot. Used by the Views menu to highlight the matching view
 * and to drive the dynamic "Views: <name>" label on the closed PillButton.
 *
 * Excludes `hideOrphanStakes` from comparison: it is GUISettings-backed and
 * cross-page, consistent with how `clearFilters()` excludes it from reset
 * scope. Including it here would cause a Settings toggle (which silently
 * changes that field) to drop the user out of an active view.
 *
 * Range-date fields (`dateRangeFrom`, `dateRangeTo`) only contribute to the
 * comparison when `dateFilter === 'range'`. For any other `dateFilter` value,
 * the range fields are ignored — they default to "last 7 days" and would
 * otherwise prevent matches whenever the user has scrolled past the seed-day
 * boundary.
 */

import type {
  DateFilter,
  TypeFilter,
  WatchOnlyFilter,
  SortColumn,
  SortDirection,
} from '@/store/slices/transactionsSlice';

export interface ViewFilterSnapshot {
  dateFilter: DateFilter;
  dateRangeFrom: string;
  dateRangeTo: string;
  // Phase 3 multi-select: snapshot stores the full selection slice. Empty
  // array means "no type filter" (match-all). Equality is set-based (length +
  // membership), order-insensitive — `[sent, received]` matches a saved view
  // of `[received, sent]`. This is the user-facing model: the backend OR-
  // combines the entries regardless of position, and the checkbox editor
  // appends selections in click-order so two equivalent re-selections would
  // otherwise produce non-matching arrays. See matchesViewSnapshot.
  typeFilter: TypeFilter[];
  searchText: string;
  minAmount: string;
  // Phase 4 amount bounds: empty string = unbounded. Saved views compare both
  // min and max — see matchesViewSnapshot.
  maxAmount: string;
  watchOnlyFilter: WatchOnlyFilter;
}

export interface ViewSnapshot {
  filters: ViewFilterSnapshot;
  sortColumn: SortColumn;
  sortDirection: SortDirection;
}

export interface CurrentTransactionsState extends ViewFilterSnapshot {
  sortColumn: SortColumn;
  sortDirection: SortDirection;
}

export function matchesViewSnapshot(
  current: CurrentTransactionsState,
  view: ViewSnapshot,
): boolean {
  const f = view.filters;
  if (current.dateFilter !== f.dateFilter) return false;
  // Set equality on typeFilter (Phase 3 multi-select): same length + every
  // current entry exists in the view set. Order-insensitive by design — the
  // checkbox editor appends selections in click-order, but two equivalent
  // selections must match the same saved view regardless of how the user
  // ticked the boxes. Both sides are deduped at write-time by `setTypeFilter`,
  // so length-match + one-sided includes is a sufficient set comparison.
  if (current.typeFilter.length !== f.typeFilter.length) return false;
  for (const v of current.typeFilter) {
    if (!f.typeFilter.includes(v)) return false;
  }
  if (current.searchText !== f.searchText) return false;
  if (current.minAmount !== f.minAmount) return false;
  if (current.maxAmount !== f.maxAmount) return false;
  if (current.watchOnlyFilter !== f.watchOnlyFilter) return false;
  if (current.sortColumn !== view.sortColumn) return false;
  if (current.sortDirection !== view.sortDirection) return false;

  // Range fields only matter when dateFilter is 'range'.
  if (current.dateFilter === 'range') {
    if (current.dateRangeFrom !== f.dateRangeFrom) return false;
    if (current.dateRangeTo !== f.dateRangeTo) return false;
  }

  return true;
}
