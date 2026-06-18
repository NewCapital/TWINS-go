import type { SliceCreator } from '../store.types';
import { core } from '@/shared/types/wallet.types';
import { GetTransactionsPage, ExportFilteredTransactionsCSV } from '@wailsjs/go/main/App';
import { parseSearchQuery } from '@/shared/utils/parseSearchQuery';

/**
 * Transaction filter types - kept identical for dropdown compatibility
 */
export type DateFilter = 'all' | 'today' | 'week' | 'month' | 'lastMonth' | 'year' | 'range';
/**
 * Per-entry type filter keys. The slice stores a SLICE of these (multi-select).
 * An empty slice means "no type filter" (= show all categories).
 *
 * Phase 3 redesign (m-tx-filter-type-multi-select): the legacy 'all' and
 * 'mostCommon' pseudo-entries are gone. 'all' is the implicit state when the
 * slice is empty; 'mostCommon' was a single-select-era grouping that becomes
 * redundant under multi-select.
 */
export type TypeFilter =
  | 'received'
  | 'sent'
  | 'toYourself'
  | 'mined'
  | 'minted'
  | 'masternode'
  | 'consolidation'
  | 'other';
export type WatchOnlyFilter = 'all' | 'yes' | 'no';
export type SortColumn = 'date' | 'type' | 'address' | 'amount';
export type SortDirection = 'asc' | 'desc';

/**
 * Valid page sizes for the page size selector
 */
export const PAGE_SIZES = [25, 50, 100, 250] as const;
export type PageSize = typeof PAGE_SIZES[number];

/**
 * Transactions State
 * Server-side paginated: only the current page is held in state.
 * All filtering, sorting, and pagination happen on the Go backend.
 */
export interface TransactionsState {
  // Current page data (from server)
  transactions: core.Transaction[];
  total: number;        // total matching current filter
  totalAll: number;     // total in wallet (unfiltered)
  totalPages: number;
  isLoadingTransactions: boolean;
  transactionsError: string | null;

  // Pagination
  currentPage: number;   // 1-based
  pageSize: PageSize;

  // Filter state (sent to server)
  dateFilter: DateFilter;
  // typeFilter is a multi-select slice: empty array = no filter (all categories).
  // OR-matched server-side via wallet.matchesTypeFilterWithComment.
  typeFilter: TypeFilter[];
  searchText: string;
  // Amount bounds — both stored as free-form strings (empty = no bound,
  // server-side semantics: 0 = no constraint). Parsed at buildFilter time.
  minAmount: string;
  maxAmount: string;
  dateRangeFrom: string; // ISO date string
  dateRangeTo: string;   // ISO date string
  watchOnlyFilter: WatchOnlyFilter;
  hasWatchOnlyAddresses: boolean;
  hideOrphanStakes: boolean;

  // Sort state (sent to server)
  sortColumn: SortColumn;
  sortDirection: SortDirection;

  // New transaction notification
  newTransactionCount: number;

  // Block explorer URLs (parsed from strThirdPartyTxUrls setting)
  blockExplorerUrls: BlockExplorerUrl[];

  // Saved views (default + user-created). Lazily seeded from localStorage on
  // first load via loadViewsFromStorage(); see TransactionView and DEFAULT_VIEWS.
  views: TransactionView[];
}

/**
 * Parsed block explorer URL
 */
export interface BlockExplorerUrl {
  url: string;
  hostname: string;
}

/**
 * A persisted Transactions view — a named snapshot of filter + sort state that
 * can be one-click reapplied. Default views ship with isDefault=true and are
 * locked from rename/delete in the UI. User-created views carry isDefault=false.
 * `hideOrphanStakes` is intentionally excluded from the snapshot — it's
 * GUISettings-backed and cross-page (see clearFilters()).
 */
// Keep this interface in lockstep with the structurally-identical copy in
// `shared/utils/transactionViewMatching.ts`. Both declarations are duck-typed
// against each other (the slice imports nothing from the util and vice
// versa) — drift would silently break matchesViewSnapshot against applyView.
export interface ViewFilterSnapshot {
  dateFilter: DateFilter;
  dateRangeFrom: string;
  dateRangeTo: string;
  // Phase 3 multi-select: empty array = "no type filter" (match-all).
  // Equality is set-based when matched by matchesViewSnapshot (order-
  // insensitive) — the checkbox editor's click-order does not affect view
  // matching.
  typeFilter: TypeFilter[];
  searchText: string;
  minAmount: string;
  // Phase 4 amount bounds: empty string = unbounded; saved views compare both.
  maxAmount: string;
  watchOnlyFilter: WatchOnlyFilter;
}

export interface TransactionView {
  id: string;
  name: string;
  isDefault: boolean;
  filters: ViewFilterSnapshot;
  sortColumn: SortColumn;
  sortDirection: SortDirection;
}

/**
 * Transactions Actions
 */
export interface TransactionsActions {
  // Data loading - fetches current page from server
  fetchPage: (page?: number) => Promise<void>;

  // Pagination actions
  setPage: (page: number) => void;
  setPageSize: (size: PageSize) => void;
  goToFirstPage: () => void;
  goToLastPage: () => void;
  goToPrevPage: () => void;
  goToNextPage: () => void;

  // Filter actions - each resets to page 1 and fetches
  setDateFilter: (filter: DateFilter) => void;
  setTypeFilter: (filters: TypeFilter[]) => void;
  setSearchText: (text: string) => void;
  setMinAmount: (amount: string) => void;
  setMaxAmount: (amount: string) => void;
  // Batched setter for the Amount editor's Apply/Clear flow: updates both
  // bounds in ONE state mutation and fires exactly ONE fetchPage(1).
  // Without this, calling setMinAmount + setMaxAmount sequentially would
  // schedule two independent 300ms debounce timers and dispatch two identical
  // fetches ~300ms apart. Mirrors `applyCustomDateRange`'s batched pattern.
  setAmountRange: (min: string, max: string) => void;
  setDateRange: (from: string, to: string) => void;
  applyCustomDateRange: (from: string, to: string) => void;
  setWatchOnlyFilter: (filter: WatchOnlyFilter) => void;
  syncHideOrphanStakes: () => Promise<void>;
  clearFilters: () => void;
  dispatchSmartSearch: (query: string) => void;

  // Sort actions
  setSortColumn: (column: SortColumn) => void;
  toggleSortDirection: () => void;

  // Export (server-side CSV generation)
  exportCSV: () => Promise<boolean>;

  // Notification
  incrementNewTransactionCount: () => void;
  clearNewTransactionCount: () => void;

  // Block explorer URLs
  syncBlockExplorerUrls: () => Promise<void>;

  // Saved views
  loadViews: () => void;
  saveCurrentAs: (name: string) => void;
  applyView: (id: string) => void;
  renameView: (id: string, newName: string) => void;
  deleteView: (id: string) => void;
}

export type TransactionsSlice = TransactionsState & TransactionsActions;

// Helper to get default date range (last 7 days to today)
function getDefaultDateRange(): { from: string; to: string } {
  const today = new Date();
  const lastWeek = new Date(today);
  lastWeek.setDate(lastWeek.getDate() - 7);
  return {
    from: lastWeek.toISOString().split('T')[0],
    to: today.toISOString().split('T')[0],
  };
}

// localStorage key constants
const STORAGE_KEY_DATE_FILTER = 'twins_transactionDateFilter';
const STORAGE_KEY_TYPE_FILTER = 'twins_transactionTypeFilter';
const STORAGE_KEY_PAGE_SIZE = 'twins_transactionPageSize';
const STORAGE_KEY_VIEWS = 'twins_transactionViews';

// Default views shipped with the wallet. Seeded lazily on first read when
// localStorage is empty or corrupt. Date-range fields use the default 7-day
// window — they are inert for these defaults because each carries
// dateFilter !== 'range'.
function buildDefaultViews(): TransactionView[] {
  const range = getDefaultDateRange();
  const baseFilters: ViewFilterSnapshot = {
    dateFilter: 'all',
    dateRangeFrom: range.from,
    dateRangeTo: range.to,
    // Phase 3 multi-select: empty array means "no type filter" (match-all).
    typeFilter: [],
    searchText: '',
    minAmount: '',
    maxAmount: '',
    watchOnlyFilter: 'all',
  };
  const mk = (
    id: string,
    name: string,
    typeFilter: TypeFilter[],
  ): TransactionView => ({
    id,
    name,
    isDefault: true,
    filters: { ...baseFilters, typeFilter },
    sortColumn: 'date',
    sortDirection: 'desc',
  });
  return [
    mk('default-all', 'All', []),
    mk('default-received', 'Received', ['received']),
    mk('default-sent', 'Sent', ['sent']),
    mk('default-staking', 'Staking rewards', ['minted']),
    mk('default-masternode', 'Masternode rewards', ['masternode']),
  ];
}

// Allow-lists for enum-typed fields. Used by isValidTransactionView so a
// schema-drifted localStorage entry with a stale enum value (e.g. an old
// build's deprecated dateFilter token, or a forged value like sortColumn:'fee')
// cannot reach applyView and poison the in-memory filter state.
const VALID_DATE_FILTERS: readonly string[] = [
  'all', 'today', 'week', 'month', 'lastMonth', 'year', 'range',
];
// Phase 3 multi-select dropped 'all' and 'mostCommon' as filter keys: 'all'
// is the implicit state of an empty typeFilter slice; 'mostCommon' was a
// single-select-era grouping that becomes redundant under multi-select.
// Persisted views from earlier Phase 2 builds that contain these legacy keys
// fall back to defaults via isValidTransactionView rejection.
const VALID_TYPE_FILTERS: readonly string[] = [
  'received', 'sent', 'toYourself',
  'mined', 'minted', 'masternode', 'consolidation', 'other',
];
const VALID_WATCH_ONLY_FILTERS: readonly string[] = ['all', 'yes', 'no'];
const VALID_SORT_COLUMNS: readonly string[] = ['date', 'type', 'address', 'amount'];
const VALID_SORT_DIRECTIONS: readonly string[] = ['asc', 'desc'];

function isValidTransactionView(v: unknown): v is TransactionView {
  if (!v || typeof v !== 'object') return false;
  const o = v as Record<string, unknown>;
  if (typeof o.id !== 'string' || !o.id) return false;
  if (typeof o.name !== 'string') return false;
  if (typeof o.isDefault !== 'boolean') return false;
  if (!o.filters || typeof o.filters !== 'object') return false;
  const f = o.filters as Record<string, unknown>;
  if (typeof f.dateFilter !== 'string' || !VALID_DATE_FILTERS.includes(f.dateFilter)) return false;
  if (typeof f.dateRangeFrom !== 'string') return false;
  if (typeof f.dateRangeTo !== 'string') return false;
  // Phase 3 multi-select: typeFilter is now an array. Every entry must be a
  // recognized filter key (Phase 2 single-string and legacy 'all'/'mostCommon'
  // entries are rejected and fall back to defaults via loadViewsFromStorage).
  if (!Array.isArray(f.typeFilter)) return false;
  if (!f.typeFilter.every((v: unknown) => typeof v === 'string' && VALID_TYPE_FILTERS.includes(v))) return false;
  if (typeof f.searchText !== 'string') return false;
  if (typeof f.minAmount !== 'string') return false;
  // Phase 4 max-amount migration: pre-Phase-4 persisted views have no
  // `maxAmount` field. Accept undefined here so the view survives the shape
  // guard; loadViewsFromStorage coerces undefined → '' before returning so
  // downstream consumers (applyView, matchesViewSnapshot, snapshot writes)
  // never observe the missing-field state.
  if (f.maxAmount !== undefined && typeof f.maxAmount !== 'string') return false;
  if (typeof f.watchOnlyFilter !== 'string' || !VALID_WATCH_ONLY_FILTERS.includes(f.watchOnlyFilter)) return false;
  if (typeof o.sortColumn !== 'string' || !VALID_SORT_COLUMNS.includes(o.sortColumn)) return false;
  if (typeof o.sortDirection !== 'string' || !VALID_SORT_DIRECTIONS.includes(o.sortDirection)) return false;
  return true;
}

function loadViewsFromStorage(): TransactionView[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY_VIEWS);
    if (!raw) return buildDefaultViews();
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed) || parsed.length === 0) return buildDefaultViews();
    // Per-entry shape validation: a manually-edited or schema-drifted entry
    // would otherwise reach the UI and crash TxViewsMenu via undefined
    // .filters.* dereferences. Drop bad entries; if nothing valid remains,
    // re-seed defaults.
    const valid = parsed.filter(isValidTransactionView);
    if (valid.length === 0) return buildDefaultViews();
    // Phase 4 max-amount migration: coerce missing `maxAmount` to '' so the
    // returned views are always shape-complete. Pre-Phase-4 user-saved views
    // would otherwise carry `maxAmount: undefined` through to applyView and
    // crash the downstream `s.maxAmount = view.filters.maxAmount` assignment
    // (or worse, silently drift the snapshot type).
    return valid.map((v: TransactionView) => (
      typeof v.filters.maxAmount === 'string'
        ? v
        : { ...v, filters: { ...v.filters, maxAmount: '' } }
    ));
  } catch {
    return buildDefaultViews();
  }
}

function persistViews(views: TransactionView[]): void {
  try {
    localStorage.setItem(STORAGE_KEY_VIEWS, JSON.stringify(views));
  } catch {
    // Silently fail
  }
}

function generateViewId(): string {
  try {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
      return crypto.randomUUID();
    }
  } catch {
    // Fall through
  }
  return `view-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
}

// Valid filter values for validation
const validDateFilters: DateFilter[] = ['all', 'today', 'week', 'month', 'lastMonth', 'year', 'range'];
const validTypeFilters: TypeFilter[] = [
  'received', 'sent', 'toYourself', 'mined', 'minted',
  'masternode', 'consolidation', 'other',
];

function loadDateFilter(): DateFilter {
  try {
    const stored = localStorage.getItem(STORAGE_KEY_DATE_FILTER);
    if (stored && validDateFilters.includes(stored as DateFilter)) {
      return stored as DateFilter;
    }
  } catch {
    // Silently fail
  }
  return 'all';
}

/**
 * Load and normalize the persisted type-filter selection.
 *
 * Storage format (current): JSON array of TypeFilter strings,
 * e.g. '["sent","received"]'.
 *
 * Legacy single-string payloads from the Phase 2 single-select era (e.g. raw
 * 'received' / 'all' / 'mostCommon') ALL resolve to [] (match-all = Views: All)
 * as of 2026-05-22 task `l-tx-export-and-ux-polish` (B6). The previous behavior
 * that migrated a stored 'received' into ['received'] is intentionally removed
 * — it produced a misleading "Views: Received" initial state for users with
 * stale localStorage from prior builds.
 *
 * Unknown/invalid entries in a JSON array are filtered out. Any parse error or
 * non-array payload falls back to [].
 */
function loadTypeFilter(): TypeFilter[] {
  // B6 (2026-05-22): legacy single-string values from the Phase 2 single-select
  // era (e.g. raw 'received') now resolve to [] (match-all = Views: All) instead
  // of being migrated to [value]. The legacy migration produced a misleading
  // "Views: Received" initial state for users carrying stale localStorage.
  try {
    const stored = localStorage.getItem(STORAGE_KEY_TYPE_FILTER);
    if (!stored) return [];

    const parsed = JSON.parse(stored);
    if (Array.isArray(parsed)) {
      return parsed.filter((v): v is TypeFilter =>
        typeof v === 'string' && (validTypeFilters as string[]).includes(v),
      );
    }
    // Any non-array payload (legacy single-string, JSON object, etc.) →
    // match-all. The previous migration path that returned [stored] for valid
    // single-string keys is intentionally removed.
    return [];
  } catch {
    // Silently fail — corrupted/blocked localStorage.
    return [];
  }
}

function loadPageSize(): PageSize {
  try {
    const stored = localStorage.getItem(STORAGE_KEY_PAGE_SIZE);
    if (stored) {
      const num = parseInt(stored, 10);
      if ((PAGE_SIZES as readonly number[]).includes(num)) {
        return num as PageSize;
      }
    }
  } catch {
    // Silently fail
  }
  return 25;
}

/**
 * Parse third-party transaction URLs from settings.
 * Format: pipe-separated URLs with %s placeholder for txid.
 */
function parseBlockExplorerUrls(urlsString: string): BlockExplorerUrl[] {
  if (!urlsString) return [];

  return urlsString
    .split('|')
    .map(url => url.trim())
    .filter(url => url && url.includes('%s'))
    .map(url => {
      try {
        const parsed = new URL(url.replace('%s', 'placeholder'));
        return { url, hostname: parsed.hostname };
      } catch {
        return null;
      }
    })
    .filter((item): item is BlockExplorerUrl => item !== null);
}

const defaultDateRange = getDefaultDateRange();

// Initial state
const initialState: TransactionsState = {
  transactions: [],
  total: 0,
  totalAll: 0,
  totalPages: 0,
  isLoadingTransactions: false,
  transactionsError: null,

  currentPage: 1,
  pageSize: loadPageSize(),

  dateFilter: loadDateFilter(),
  typeFilter: loadTypeFilter(),
  searchText: '',
  minAmount: '',
  maxAmount: '',

  dateRangeFrom: defaultDateRange.from,
  dateRangeTo: defaultDateRange.to,

  watchOnlyFilter: 'all',
  hasWatchOnlyAddresses: false,

  hideOrphanStakes: false, // Synced from GUISettings via syncHideOrphanStakes()

  sortColumn: 'date',
  sortDirection: 'desc',

  newTransactionCount: 0,

  blockExplorerUrls: [],

  views: loadViewsFromStorage(),
};

/**
 * Build a TransactionFilter from current state for the backend call
 */
function buildFilter(state: TransactionsState, pageOverride?: number): Record<string, unknown> {
  return {
    page: pageOverride ?? state.currentPage,
    page_size: state.pageSize,
    date_filter: state.dateFilter,
    date_range_from: state.dateFilter === 'range' ? state.dateRangeFrom : '',
    date_range_to: state.dateFilter === 'range' ? state.dateRangeTo : '',
    type_filter: state.typeFilter,
    search_text: state.searchText,
    min_amount: state.minAmount ? parseFloat(state.minAmount) || 0 : 0,
    max_amount: state.maxAmount ? parseFloat(state.maxAmount) || 0 : 0,
    watch_only_filter: state.watchOnlyFilter,
    hide_orphan_stakes: state.hideOrphanStakes,
    sort_column: state.sortColumn,
    sort_direction: state.sortDirection,
  };
}

// Separate debounce timers for search text and amount bounds. Min and max
// have their own timers so typing into one input does not cancel a pending
// fetch from the other.
let searchDebounceTimer: ReturnType<typeof setTimeout> | null = null;
let amountDebounceTimer: ReturnType<typeof setTimeout> | null = null;
let amountMaxDebounceTimer: ReturnType<typeof setTimeout> | null = null;

export const createTransactionsSlice: SliceCreator<TransactionsSlice> = (set, get) => ({
  ...initialState,

  // Fetch a page from the server
  fetchPage: async (page?: number) => {
    const state = get();
    const targetPage = page ?? state.currentPage;

    set((s) => {
      s.isLoadingTransactions = true;
      s.transactionsError = null;
      if (page !== undefined) {
        s.currentPage = page;
      }
    });

    try {
      const filter = buildFilter(get(), targetPage);
      const result = await GetTransactionsPage(filter as any);

      set((s) => {
        s.transactions = result.transactions || [];
        s.total = result.total;
        s.totalAll = result.total_all;
        s.totalPages = result.total_pages;
        s.currentPage = result.page;
        s.pageSize = result.page_size as PageSize;
        s.isLoadingTransactions = false;
      });
    } catch (error) {
      console.error('Failed to fetch transactions page:', error);
      set((s) => {
        s.transactionsError = error instanceof Error ? error.message : 'Failed to load transactions';
        s.isLoadingTransactions = false;
      });
    }
  },

  // Pagination actions
  setPage: (page: number) => {
    get().fetchPage(page);
  },

  setPageSize: (size: PageSize) => {
    // U7 (2026-05-22): preserve the topmost row of the user's current viewport
    // after the page-size change instead of always resetting to page 1.
    // Top row index (zero-based) = (currentPage - 1) * oldPageSize.
    // newPage = floor(topRowIndex / newSize) + 1 (clamped to >= 1).
    const { currentPage, pageSize: oldSize } = get();
    const newPage =
      currentPage <= 1
        ? 1
        : Math.max(1, Math.floor((currentPage - 1) * oldSize / size) + 1);
    set((s) => {
      s.pageSize = size;
      s.currentPage = newPage;
    });
    try {
      localStorage.setItem(STORAGE_KEY_PAGE_SIZE, size.toString());
    } catch {
      // Silently fail
    }
    get().fetchPage(newPage);
  },

  goToFirstPage: () => {
    get().fetchPage(1);
  },

  goToLastPage: () => {
    const { totalPages } = get();
    if (totalPages > 0) {
      get().fetchPage(totalPages);
    }
  },

  goToPrevPage: () => {
    const { currentPage } = get();
    if (currentPage > 1) {
      get().fetchPage(currentPage - 1);
    }
  },

  goToNextPage: () => {
    const { currentPage, totalPages } = get();
    if (currentPage < totalPages) {
      get().fetchPage(currentPage + 1);
    }
  },

  // Filter actions - each resets to page 1 and fetches
  setDateFilter: (filter: DateFilter) => {
    set((s) => {
      s.dateFilter = filter;
      s.currentPage = 1;
    });
    try {
      localStorage.setItem(STORAGE_KEY_DATE_FILTER, filter);
    } catch {
      // Silently fail
    }
    get().fetchPage(1);
  },

  setTypeFilter: (filters: TypeFilter[]) => {
    // Defensive: dedupe + validate so callers can pass any string slice without
    // corrupting state. Unknown entries are dropped silently — UI sources of
    // truth are typed, so this only matters for the migration-from-legacy path.
    const seen = new Set<TypeFilter>();
    const cleaned: TypeFilter[] = [];
    for (const f of filters) {
      if (!seen.has(f) && (validTypeFilters as string[]).includes(f)) {
        seen.add(f);
        cleaned.push(f);
      }
    }
    set((s) => {
      s.typeFilter = cleaned;
      s.currentPage = 1;
    });
    try {
      localStorage.setItem(STORAGE_KEY_TYPE_FILTER, JSON.stringify(cleaned));
    } catch {
      // Silently fail
    }
    get().fetchPage(1);
  },

  setSearchText: (text: string) => {
    set((s) => {
      s.searchText = text;
    });
    // Debounce search: wait 300ms before fetching
    if (searchDebounceTimer) clearTimeout(searchDebounceTimer);
    searchDebounceTimer = setTimeout(() => {
      set((s) => { s.currentPage = 1; });
      get().fetchPage(1);
    }, 300);
  },

  setMinAmount: (amount: string) => {
    set((s) => {
      s.minAmount = amount;
    });
    // Debounce amount filter with its own timer
    if (amountDebounceTimer) clearTimeout(amountDebounceTimer);
    amountDebounceTimer = setTimeout(() => {
      set((s) => { s.currentPage = 1; });
      get().fetchPage(1);
    }, 300);
  },

  setMaxAmount: (amount: string) => {
    set((s) => {
      s.maxAmount = amount;
    });
    // Separate debounce timer from min — typing in one input must not
    // cancel a pending fetch from the other.
    if (amountMaxDebounceTimer) clearTimeout(amountMaxDebounceTimer);
    amountMaxDebounceTimer = setTimeout(() => {
      set((s) => { s.currentPage = 1; });
      get().fetchPage(1);
    }, 300);
  },

  setAmountRange: (min: string, max: string) => {
    // Batched setter for the Amount editor's Apply/Clear flow. Cancel any
    // pending min/max debounce timers (in case the user typed into the
    // inputs and then clicked Apply within 300ms — we must NOT fire a stale
    // single-side fetch after the batched one). Then mutate both fields,
    // reset to page 1, and fire exactly ONE fetchPage(1) synchronously.
    if (amountDebounceTimer) {
      clearTimeout(amountDebounceTimer);
      amountDebounceTimer = null;
    }
    if (amountMaxDebounceTimer) {
      clearTimeout(amountMaxDebounceTimer);
      amountMaxDebounceTimer = null;
    }

    // Auto-swap reversed ranges. Without this, a user typing From=10 / To=5
    // would silently get zero results because the backend's matchesAmountFilter
    // treats min > max as "match nothing" (correctly, given inclusive bounds).
    // Swapping is the user-friendly choice — mirrors applyCustomDateRange's
    // identical fix for date ranges.
    //
    // Guard: BOTH sides must be non-empty AND parse to STRICTLY POSITIVE
    // finite numbers. Zero is the codebase-wide "no constraint" sentinel
    // (buildFilter coerces `parseFloat || 0`; backend matchesAmountFilter
    // gates on `min/max > 0`; chip isActiveBound rejects parseFloat <= 0).
    // Codex round-4 caught a UX bug where treating 0 as a real bound would
    // invert intent: From=1, To=0 swapped to min=0/max=1, producing `≤1`
    // instead of preserving the user's `≥1, no upper bound` intent. With
    // the strict-positive guard, To=0 passes through as the explicit
    // unbounded sentinel and only finite positive ranges get swapped.
    let normMin = min;
    let normMax = max;
    if (min !== '' && max !== '') {
      const minNum = parseFloat(min);
      const maxNum = parseFloat(max);
      if (
        Number.isFinite(minNum) && Number.isFinite(maxNum) &&
        minNum > 0 && maxNum > 0 &&
        minNum > maxNum
      ) {
        normMin = max;
        normMax = min;
      }
    }

    set((s) => {
      s.minAmount = normMin;
      s.maxAmount = normMax;
      s.currentPage = 1;
    });
    get().fetchPage(1);
  },

  setDateRange: (from: string, to: string) => {
    const fromDate = new Date(from);
    const toDate = new Date(to);

    // Validate: swap if from is after to
    if (fromDate > toDate) {
      set((s) => {
        s.dateRangeFrom = to;
        s.dateRangeTo = from;
        s.currentPage = 1;
      });
    } else {
      set((s) => {
        s.dateRangeFrom = from;
        s.dateRangeTo = to;
        s.currentPage = 1;
      });
    }
    get().fetchPage(1);
  },

  // Batched setter for the chip-bar custom-range editor: updates dateFilter,
  // dateRangeFrom, dateRangeTo, and currentPage in a single state mutation and
  // fires exactly ONE fetchPage(1). Without this, calling setDateRange + then
  // setDateFilter('range') would dispatch two concurrent fetches and a stale
  // first response could clobber the correct one. Use this from any callsite
  // that needs to enter 'range' mode AND set the range together.
  applyCustomDateRange: (from: string, to: string) => {
    const fromDate = new Date(from);
    const toDate = new Date(to);
    const [normFrom, normTo] = fromDate > toDate ? [to, from] : [from, to];
    set((s) => {
      s.dateFilter = 'range';
      s.dateRangeFrom = normFrom;
      s.dateRangeTo = normTo;
      s.currentPage = 1;
    });
    try {
      localStorage.setItem(STORAGE_KEY_DATE_FILTER, 'range');
    } catch {
      // Silently fail
    }
    get().fetchPage(1);
  },

  setWatchOnlyFilter: (filter: WatchOnlyFilter) => {
    set((s) => {
      s.watchOnlyFilter = filter;
      s.currentPage = 1;
    });
    get().fetchPage(1);
  },

  syncHideOrphanStakes: async () => {
    try {
      const { GetSettingBool } = await import('@wailsjs/go/main/App');
      const hide = await GetSettingBool('fHideOrphans');
      const prev = get().hideOrphanStakes;
      if (hide !== prev) {
        set((s) => {
          s.hideOrphanStakes = hide;
          s.currentPage = 1;
        });
        get().fetchPage(1);
      }
    } catch {
      // Silently fail — keep current value
    }
  },

  syncBlockExplorerUrls: async () => {
    try {
      const { GetSettingString } = await import('@wailsjs/go/main/App');
      const urlsString = await GetSettingString('strThirdPartyTxUrls');
      const urls = parseBlockExplorerUrls(urlsString);
      set((s) => {
        s.blockExplorerUrls = urls;
      });
    } catch {
      // Silently fail — block explorer is optional
    }
  },

  clearFilters: () => {
    const defaultRange = getDefaultDateRange();
    set((s) => {
      s.dateFilter = 'all';
      s.typeFilter = [];
      s.searchText = '';
      s.minAmount = '';
      s.maxAmount = '';
      s.dateRangeFrom = defaultRange.from;
      s.dateRangeTo = defaultRange.to;
      s.watchOnlyFilter = 'all';
      s.currentPage = 1;
      // Note: hideOrphanStakes is NOT cleared — it's managed via GUISettings (Settings dialog)
    });
    // Persist cleared state to localStorage so a reload doesn't restore the
    // previously-active filters (otherwise the in-memory clear silently
    // disagrees with the on-disk state). Type filter is persisted as JSON to
    // match the multi-select storage format.
    try {
      localStorage.setItem(STORAGE_KEY_DATE_FILTER, 'all');
      localStorage.setItem(STORAGE_KEY_TYPE_FILTER, JSON.stringify([]));
    } catch {
      // Silently fail
    }
    get().fetchPage(1);
  },

  // Smart-search dispatcher: parses the query, routes to the appropriate
  // filter setter (TXID/address/search -> searchText, min_amount -> setAmountRange).
  // For numeric `>50` queries we use setAmountRange so an existing upper
  // bound is CLEARED — the user typing `>50` expects `amount >= 50` with no
  // upper cap, but a stale maxAmount from a prior editor session would
  // otherwise narrow the result to `[50, oldMax]`. Mirrors the editor's
  // Apply semantics (one fetch, full range overwrite).
  dispatchSmartSearch: (query: string) => {
    const parsed = parseSearchQuery(query);
    switch (parsed.type) {
      case 'address':
      case 'search':
        get().setSearchText(parsed.value);
        break;
      case 'min_amount':
        get().setAmountRange(String(parsed.value), '');
        break;
    }
  },

  // Sort actions
  setSortColumn: (column: SortColumn) => {
    set((s) => {
      if (s.sortColumn === column) {
        s.sortDirection = s.sortDirection === 'asc' ? 'desc' : 'asc';
      } else {
        s.sortColumn = column;
        s.sortDirection = column === 'date' ? 'desc' : 'asc';
      }
      s.currentPage = 1;
    });
    get().fetchPage(1);
  },

  toggleSortDirection: () => {
    set((s) => {
      s.sortDirection = s.sortDirection === 'asc' ? 'desc' : 'asc';
      s.currentPage = 1;
    });
    get().fetchPage(1);
  },

  // Export - delegates to backend for all matching results
  exportCSV: async () => {
    try {
      const state = get();
      const filter = buildFilter(state);
      const saved = await ExportFilteredTransactionsCSV(filter as any);
      return saved;
    } catch (error) {
      console.error('Failed to export transactions CSV:', error);
      throw error;
    }
  },

  // Notification
  incrementNewTransactionCount: () => {
    set((s) => {
      s.newTransactionCount += 1;
    });
  },

  clearNewTransactionCount: () => {
    set((s) => {
      s.newTransactionCount = 0;
    });
  },

  // Saved views — re-seed from localStorage. Useful when a component mounts
  // and wants to guarantee the defaults are populated (e.g. TxFilterBar's
  // one-shot useEffect on mount).
  loadViews: () => {
    set((s) => {
      s.views = loadViewsFromStorage();
    });
  },

  // Append a new user view from the current filter+sort state. Trims name;
  // whitespace-only is rejected. Persists to localStorage. Does NOT touch
  // active filter state — saving is a snapshot, not an apply.
  saveCurrentAs: (name: string) => {
    const trimmed = name.trim();
    if (!trimmed) return;
    const s = get();
    const newView: TransactionView = {
      id: generateViewId(),
      name: trimmed,
      isDefault: false,
      filters: {
        dateFilter: s.dateFilter,
        dateRangeFrom: s.dateRangeFrom,
        dateRangeTo: s.dateRangeTo,
        typeFilter: s.typeFilter,
        searchText: s.searchText,
        minAmount: s.minAmount,
        maxAmount: s.maxAmount,
        watchOnlyFilter: s.watchOnlyFilter,
      },
      sortColumn: s.sortColumn,
      sortDirection: s.sortDirection,
    };
    const nextViews = [...s.views, newView];
    set((draft) => {
      draft.views = nextViews;
    });
    persistViews(nextViews);
  },

  // Apply a view: in ONE set(), copy snapshot fields onto state + reset
  // currentPage to 1 + sync STORAGE_KEY_DATE_FILTER / STORAGE_KEY_TYPE_FILTER
  // so a reload doesn't disagree with the in-memory state. Then fire exactly
  // ONE fetchPage(1). Mirrors applyCustomDateRange's batched-setter pattern.
  applyView: (id: string) => {
    const view = get().views.find((v) => v.id === id);
    if (!view) return;
    set((s) => {
      s.dateFilter = view.filters.dateFilter;
      // Only overwrite range fields when the view's dateFilter is 'range'.
      // For preset filters (all/today/week/...), the snapshot's range fields
      // are inert and would silently clobber the user's last custom range —
      // breaking the TxFilterDateEditor draft. Mirrors the read-side gate in
      // matchesViewSnapshot.
      if (view.filters.dateFilter === 'range') {
        s.dateRangeFrom = view.filters.dateRangeFrom;
        s.dateRangeTo = view.filters.dateRangeTo;
      }
      s.typeFilter = view.filters.typeFilter;
      s.searchText = view.filters.searchText;
      s.minAmount = view.filters.minAmount;
      s.maxAmount = view.filters.maxAmount;
      s.watchOnlyFilter = view.filters.watchOnlyFilter;
      s.sortColumn = view.sortColumn;
      s.sortDirection = view.sortDirection;
      s.currentPage = 1;
    });
    try {
      localStorage.setItem(STORAGE_KEY_DATE_FILTER, view.filters.dateFilter);
      // Phase 3 multi-select: persist as JSON array to match loadTypeFilter()'s
      // expected wire format. Raw stringification would write "received,sent"
      // (Array.toString) which the loader would treat as a legacy single-key.
      localStorage.setItem(STORAGE_KEY_TYPE_FILTER, JSON.stringify(view.filters.typeFilter));
    } catch {
      // Silently fail
    }
    get().fetchPage(1);
  },

  // Rename a user view. Default views are locked (isDefault === true).
  renameView: (id: string, newName: string) => {
    const trimmed = newName.trim();
    if (!trimmed) return;
    const current = get().views;
    const target = current.find((v) => v.id === id);
    if (!target || target.isDefault) return;
    const nextViews = current.map((v) =>
      v.id === id ? { ...v, name: trimmed } : v,
    );
    set((s) => {
      s.views = nextViews;
    });
    persistViews(nextViews);
  },

  // Delete a user view. Default views are locked (isDefault === true).
  deleteView: (id: string) => {
    const current = get().views;
    const target = current.find((v) => v.id === id);
    if (!target || target.isDefault) return;
    const nextViews = current.filter((v) => v.id !== id);
    set((s) => {
      s.views = nextViews;
    });
    persistViews(nextViews);
  },
});
