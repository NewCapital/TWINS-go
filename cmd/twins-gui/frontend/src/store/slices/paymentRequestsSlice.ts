import type { SliceCreator } from '../store.types';
import type { core } from '@wailsjs/go/models';
import { GetPaymentRequestsPage } from '@wailsjs/go/main/App';

/**
 * Payment Requests slice
 * ----------------------------------------------------------------------------
 * Server-side paginated state for the Receive page's "Recent Requests" table.
 * Mirrors the receivingAddressesSlice pattern: only the current page is held
 * in state; all sorting/pagination happen on the Go backend via
 * `GetPaymentRequestsPage(filter)`.
 *
 * Naming convention: every field and action is prefixed with `paymentReqs` to
 * avoid name collisions when slices are merged into the combined store via
 * object spread. The transactions slice already owns short names like `total`,
 * `pageSize`, `fetchPage`, `setSortColumn`, etc.
 *
 * No filter UI is exposed (the Receive page's Recent Requests has no
 * search/min-amount inputs); only sort and pagination drive the backend.
 */

export const PAYMENT_REQUESTS_PAGE_SIZES = [25, 50, 100, 250] as const;
export type PaymentRequestsPageSize = (typeof PAYMENT_REQUESTS_PAGE_SIZES)[number];

/** Sortable columns for the Recent Requests table. */
export type PaymentRequestsSortColumn = 'date' | 'label' | 'amount';
export type PaymentRequestsSortDirection = 'asc' | 'desc';

const STORAGE_KEY_PAGE_SIZE = 'twins_paymentReqsPageSize';
const STORAGE_KEY_SORT_COLUMN = 'twins_paymentReqsSortColumn';
const STORAGE_KEY_SORT_DIRECTION = 'twins_paymentReqsSortDirection';

const validSortColumns: PaymentRequestsSortColumn[] = ['date', 'label', 'amount'];
const validSortDirections: PaymentRequestsSortDirection[] = ['asc', 'desc'];

function loadPageSize(): PaymentRequestsPageSize {
  try {
    const stored = localStorage.getItem(STORAGE_KEY_PAGE_SIZE);
    if (stored) {
      const num = parseInt(stored, 10);
      if ((PAYMENT_REQUESTS_PAGE_SIZES as readonly number[]).includes(num)) {
        return num as PaymentRequestsPageSize;
      }
    }
  } catch {
    // Silently fall through to default
  }
  return 25;
}

function loadSortColumn(): PaymentRequestsSortColumn {
  try {
    const stored = localStorage.getItem(STORAGE_KEY_SORT_COLUMN);
    if (stored && validSortColumns.includes(stored as PaymentRequestsSortColumn)) {
      return stored as PaymentRequestsSortColumn;
    }
  } catch {
    // Silently fall through to default
  }
  return 'date';
}

function loadSortDirection(): PaymentRequestsSortDirection {
  try {
    const stored = localStorage.getItem(STORAGE_KEY_SORT_DIRECTION);
    if (stored && validSortDirections.includes(stored as PaymentRequestsSortDirection)) {
      return stored as PaymentRequestsSortDirection;
    }
  } catch {
    // Silently fall through to default
  }
  return 'desc';
}

function persist(key: string, value: string): void {
  try {
    localStorage.setItem(key, value);
  } catch {
    // Silently fail — persistence is best-effort
  }
}

export interface PaymentRequestsState {
  // Current page data (from server)
  paymentReqsRequests: core.PaymentRequest[];
  paymentReqsTotal: number;
  paymentReqsTotalPages: number;
  paymentReqsIsLoading: boolean;
  paymentReqsError: string | null;

  // Pagination
  paymentReqsCurrentPage: number; // 1-based
  paymentReqsPageSize: PaymentRequestsPageSize;

  // Sort
  paymentReqsSortColumn: PaymentRequestsSortColumn;
  paymentReqsSortDirection: PaymentRequestsSortDirection;
}

export interface PaymentRequestsActions {
  // Data loading
  paymentReqsFetchPage: (page?: number) => Promise<void>;

  // Pagination
  paymentReqsSetPage: (page: number) => void;
  paymentReqsSetPageSize: (size: PaymentRequestsPageSize) => void;
  paymentReqsGoToFirstPage: () => void;
  paymentReqsGoToPrevPage: () => void;
  paymentReqsGoToNextPage: () => void;
  paymentReqsGoToLastPage: () => void;

  // Sort
  paymentReqsSetSortColumn: (column: PaymentRequestsSortColumn) => void;
  paymentReqsToggleSortDirection: () => void;
}

export type PaymentRequestsSlice = PaymentRequestsState & PaymentRequestsActions;

const initialState: PaymentRequestsState = {
  paymentReqsRequests: [],
  paymentReqsTotal: 0,
  paymentReqsTotalPages: 0,
  paymentReqsIsLoading: false,
  paymentReqsError: null,

  paymentReqsCurrentPage: 1,
  paymentReqsPageSize: loadPageSize(),

  paymentReqsSortColumn: loadSortColumn(),
  paymentReqsSortDirection: loadSortDirection(),
};

/**
 * Build the backend filter object from current slice state. Snake_case keys
 * match the Go `core.PaymentRequestFilter` JSON tags so the Wails-serialized
 * object passes through unchanged.
 */
function buildFilter(
  state: PaymentRequestsState,
  pageOverride?: number,
): Record<string, unknown> {
  return {
    page: pageOverride ?? state.paymentReqsCurrentPage,
    page_size: state.paymentReqsPageSize,
    sort_column: state.paymentReqsSortColumn,
    sort_direction: state.paymentReqsSortDirection,
  };
}

export const createPaymentRequestsSlice: SliceCreator<PaymentRequestsSlice> = (set, get) => ({
  ...initialState,

  paymentReqsFetchPage: async (page?: number) => {
    const state = get();
    const targetPage = page ?? state.paymentReqsCurrentPage;

    set((s) => {
      s.paymentReqsIsLoading = true;
      s.paymentReqsError = null;
      if (page !== undefined) {
        s.paymentReqsCurrentPage = page;
      }
    });

    try {
      const filter = buildFilter(get(), targetPage);
      const result = await GetPaymentRequestsPage(filter as never);

      set((s) => {
        s.paymentReqsRequests = result.requests || [];
        s.paymentReqsTotal = result.total;
        s.paymentReqsTotalPages = result.total_pages;
        s.paymentReqsCurrentPage = result.page;
        s.paymentReqsPageSize = result.page_size as PaymentRequestsPageSize;
        s.paymentReqsIsLoading = false;
      });
    } catch (error) {
      console.error('Failed to fetch payment requests page:', error);
      set((s) => {
        s.paymentReqsError =
          error instanceof Error ? error.message : 'Failed to load payment requests';
        s.paymentReqsIsLoading = false;
      });
    }
  },

  paymentReqsSetPage: (page: number) => {
    get().paymentReqsFetchPage(page);
  },

  paymentReqsSetPageSize: (size: PaymentRequestsPageSize) => {
    set((s) => {
      s.paymentReqsPageSize = size;
      s.paymentReqsCurrentPage = 1;
    });
    persist(STORAGE_KEY_PAGE_SIZE, size.toString());
    get().paymentReqsFetchPage(1);
  },

  paymentReqsGoToFirstPage: () => {
    get().paymentReqsFetchPage(1);
  },

  paymentReqsGoToPrevPage: () => {
    const { paymentReqsCurrentPage } = get();
    if (paymentReqsCurrentPage > 1) {
      get().paymentReqsFetchPage(paymentReqsCurrentPage - 1);
    }
  },

  paymentReqsGoToNextPage: () => {
    const { paymentReqsCurrentPage, paymentReqsTotalPages } = get();
    if (paymentReqsCurrentPage < paymentReqsTotalPages) {
      get().paymentReqsFetchPage(paymentReqsCurrentPage + 1);
    }
  },

  paymentReqsGoToLastPage: () => {
    const { paymentReqsTotalPages } = get();
    if (paymentReqsTotalPages > 0) {
      get().paymentReqsFetchPage(paymentReqsTotalPages);
    }
  },

  paymentReqsSetSortColumn: (column: PaymentRequestsSortColumn) => {
    set((s) => {
      if (s.paymentReqsSortColumn === column) {
        // Toggle direction when clicking the same column
        s.paymentReqsSortDirection = s.paymentReqsSortDirection === 'asc' ? 'desc' : 'asc';
      } else {
        s.paymentReqsSortColumn = column;
        // Sensible per-column default direction:
        //   date   -> desc (newest first, matches prior local-sort default)
        //   label  -> asc (alphabetical)
        //   amount -> desc (largest first)
        s.paymentReqsSortDirection = column === 'label' ? 'asc' : 'desc';
      }
      s.paymentReqsCurrentPage = 1;
    });
    const next = get();
    persist(STORAGE_KEY_SORT_COLUMN, next.paymentReqsSortColumn);
    persist(STORAGE_KEY_SORT_DIRECTION, next.paymentReqsSortDirection);
    get().paymentReqsFetchPage(1);
  },

  paymentReqsToggleSortDirection: () => {
    set((s) => {
      s.paymentReqsSortDirection = s.paymentReqsSortDirection === 'asc' ? 'desc' : 'asc';
      s.paymentReqsCurrentPage = 1;
    });
    persist(STORAGE_KEY_SORT_DIRECTION, get().paymentReqsSortDirection);
    get().paymentReqsFetchPage(1);
  },
});
