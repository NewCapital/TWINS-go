import type { SliceCreator } from '../store.types';

// Allowed page-size options for the Explorer block list footer. Mirrors the
// Transactions page convention (`PAGE_SIZES` in `transactionsSlice.ts`). The
// value is persisted to localStorage so the user's choice survives reloads;
// any other persisted value falls back to the default of 25 in
// `loadBlocksPerPage`.
export const BLOCKS_PER_PAGE_OPTIONS = [25, 50, 100, 250] as const;
export type BlocksPerPageOption = typeof BLOCKS_PER_PAGE_OPTIONS[number];

const STORAGE_KEY_BLOCKS_PER_PAGE = 'twins_explorerBlocksPerPage';

function loadBlocksPerPage(): BlocksPerPageOption {
  try {
    const stored = localStorage.getItem(STORAGE_KEY_BLOCKS_PER_PAGE);
    if (stored) {
      const num = parseInt(stored, 10);
      if ((BLOCKS_PER_PAGE_OPTIONS as readonly number[]).includes(num)) {
        return num as BlocksPerPageOption;
      }
    }
  } catch {
    // Silently fail — corrupted/blocked localStorage.
  }
  return 25;
}

// Search history (used by the SearchHistoryDropdown on Explorer SearchBar).
// Persisted to localStorage so recent searches survive reloads. Capped at
// MAX_SEARCH_HISTORY entries (newest first). The shape guard rejects entries
// that fail validation so schema-drifted persisted entries cannot poison the
// dropdown render path.
export const MAX_SEARCH_HISTORY = 10;
const STORAGE_KEY_SEARCH_HISTORY = 'twins_explorerSearchHistory';
const VALID_SEARCH_HISTORY_TYPES = ['block', 'transaction', 'address'] as const;

export interface SearchHistoryItem {
  query: string;
  type: 'block' | 'transaction' | 'address';
  timestamp: number;
  label?: string;
}

function isValidSearchHistoryItem(v: unknown): v is SearchHistoryItem {
  if (typeof v !== 'object' || v === null) return false;
  const o = v as Record<string, unknown>;
  if (typeof o.query !== 'string' || o.query === '') return false;
  if (
    typeof o.type !== 'string' ||
    !(VALID_SEARCH_HISTORY_TYPES as readonly string[]).includes(o.type)
  ) {
    return false;
  }
  if (typeof o.timestamp !== 'number' || !Number.isFinite(o.timestamp)) return false;
  if (o.label !== undefined && typeof o.label !== 'string') return false;
  return true;
}

function loadSearchHistory(): SearchHistoryItem[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY_SEARCH_HISTORY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed.filter(isValidSearchHistoryItem).slice(0, MAX_SEARCH_HISTORY);
  } catch {
    return [];
  }
}

function persistSearchHistory(arr: SearchHistoryItem[]): void {
  try {
    localStorage.setItem(STORAGE_KEY_SEARCH_HISTORY, JSON.stringify(arr));
  } catch {
    // Silently swallow quota / disabled-storage errors.
  }
}

// Explorer types matching backend core.* types
export interface BlockSummary {
  height: number;
  hash: string;
  time: string;
  tx_count: number;
  size: number;
  is_pos: boolean;
  reward: number;
}

export interface BlockDetail {
  hash: string;
  height: number;
  confirmations: number;
  size: number;
  time: string;
  previousblockhash: string;
  nextblockhash: string;
  txids: string[];
  is_pos: boolean;
  stake_reward: number;
  masternode_reward: number;
  // Dev fund payout (10% of block reward, paid to chainParams.DevAddress on
  // PoS blocks). Located at outputs[len-1] in the canonical TWINS coinstake
  // layout. Zero when no dev output is present.
  dev_reward: number;
  staker_address: string;
  masternode_address: string;
  // Dev fund payment address (from chainParams.DevAddress). Empty when no
  // dev output is present.
  dev_address: string;
  total_reward: number;
  difficulty: number;
  bits: string;
  nonce: number;
  merkleroot: string;
  // Value of the staker's funding UTXO in TWINS. Zero for PoW blocks or when
  // the funding UTXO cannot be looked up. Optional for backward-compat with
  // pre-migration cached responses.
  stake_amount?: number;
  // Age of the staker's funding UTXO in seconds (current block timestamp
  // minus funding-block timestamp). Zero for PoW blocks or when the parent
  // block lookup fails. Optional for backward-compat.
  stake_age?: number;
  // PoS stake modifier as 0x-prefixed hex (16 chars). Empty string for PoW
  // blocks or when storage.GetStakeModifier returns an error. Optional for
  // backward-compat with pre-migration cached responses.
  stake_modifier?: string;
  // hashProofOfStake (a.k.a. kernel hash) as 0x-prefixed hex (64 chars).
  // Empty string for PoW blocks or when storage.GetBlockPoSMetadata returns
  // an error / zero-hash. Optional for backward-compat.
  proof_hash?: string;
}

export interface TxInput {
  txid: string;
  vout: number;
  address: string;
  amount: number;
  is_coinbase: boolean;
  // Wallet-ownership flag from m-tx-explorer-dto-enrich (2026-05-29). False
  // when wallet is not available (pure-explorer context) or address is not
  // wallet-owned. Optional in TS to tolerate pre-enrichment cached responses.
  is_mine?: boolean;
  // True when this input is the coinstake kernel UTXO (input[0] of a
  // coinstake tx). Used to render a KERNEL chip in InputRow.
  is_coinstake_kernel?: boolean;
}

export interface TxOutput {
  index: number;
  address: string;
  amount: number;
  script_type: string;
  // Semantic label populated for coinstake-transaction outputs only
  // (e.g. "Stake Return", "Masternode Payment", "Dev Fund", "Coinstake Marker").
  // Empty/undefined for regular transactions.
  label?: string;
  is_spent: boolean;
  // Fields added by m-tx-explorer-dto-enrich (2026-05-29). All optional in TS
  // to tolerate pre-enrichment cached responses from a daemon running an
  // older binary; consumers must guard with `?? <fallback>` accordingly.
  // `role` is one of the 12 OutputRole* constants from internal/gui/core/types.go
  // (block_marker, stake_return, masternode_payment, dev_fund, external_payment,
  // change, self_send, data_carrier, mining_reward, premine, nonstandard, multisig).
  role?: string;
  // Wallet-ownership flag. False in pure-explorer context.
  is_mine?: boolean;
  // True when is_mine AND output address appears in any of the transaction's
  // input addresses (change pattern).
  is_change?: boolean;
  // True when output value > 0 AND value < dustThresholdSatoshis (~5460 sat ~ 0.0000546 TWINS)
  // AND script_type != 'nulldata' (markers excluded from dust by definition).
  is_dust?: boolean;
  // OP_RETURN payload: hex-encoded bytes (data_hex) and printable-ASCII
  // best-effort decode (data_ascii — empty if any non-printable byte).
  data_hex?: string;
  data_ascii?: string;
  // Multisig representation. `addresses` lists all N keys; `required_sigs`
  // is the M threshold. Empty/undefined for non-multisig outputs.
  addresses?: string[];
  required_sigs?: number;
}

export interface ExplorerTransaction {
  txid: string;
  block_hash: string;
  block_height: number;
  confirmations: number;
  time: string;
  size: number;
  fee: number;
  is_coinbase: boolean;
  is_coinstake: boolean;
  // Coinstake-only reward breakdown. Zero for non-coinstake transactions.
  // stake_reward = sum(outputs[stakerIdx..stakerEndIdx)).value - sum(inputs) (computed by
  // computeCoinstakeBreakdown which handles stake-split layouts).
  // masternode_reward = outputs[mnIdx].value, 0 when no MN output.
  // dev_reward = outputs[devIdx].value, 0 when no dev fund output.
  // Per-output recipient addresses live on TxOutput.label / TxOutput.address —
  // no separate top-level address fields are shipped on the wire (YAGNI; the
  // breakdown card displays amounts only, and the per-output cards show addresses).
  stake_reward: number;
  masternode_reward: number;
  dev_reward: number;
  inputs: TxInput[];
  outputs: TxOutput[];
  total_input: number;
  total_output: number;
  raw_hex?: string;
}

export interface AddressTx {
  txid: string;
  block_height: number;
  time: string;
  amount: number;
  confirmations: number;
}

export interface AddressUTXO {
  txid: string;
  vout: number;
  amount: number;
  confirmations: number;
  block_height: number;
}

/**
 * Cheap subset of address information for the Explorer Address Detail hero
 * card. Computed from a single GetUTXOsByAddress storage lookup; safe to
 * fetch on initial page load because it does not walk the address tx
 * history. Mirror of Go `core.AddressBasic`.
 */
export interface AddressBasic {
  address: string;
}

/**
 * Current spendable balance for an address. Computed from a GetUTXOsByAddress
 * prefix scan; O(U) cost where U = current UTXO count. Fetched separately from
 * AddressBasic so the hero header is not blocked by the UTXO scan. Mirror of
 * Go `core.AddressBalance`.
 */
export interface AddressBalance {
  balance: number;
}

/**
 * Expensive aggregate statistics for an address. Cost is O(n) storage reads
 * where n = address tx count. Fetched separately from AddressBasic so the
 * hero card renders immediately while the Activity column shows a skeleton.
 * Mirror of Go `core.AddressStats`.
 */
export interface AddressStats {
  tx_count: number;
  total_received: number;
  total_sent: number;
  /**
   * Unix timestamp (seconds) of the earliest tx involving this address.
   * Zero when the address has no transactions or block-time lookup failed.
   */
  first_seen: number;
  /**
   * Unix timestamp (seconds) of the latest tx involving this address.
   * Zero when the address has no transactions or block-time lookup failed.
   */
  last_seen: number;
}

export interface AddressTxPage {
  transactions: AddressTx[];
  total: number;
  has_more: boolean;
}

export type SearchResultType = 'block' | 'transaction' | 'address' | 'not_found';

export interface SearchResult {
  type: SearchResultType;
  query: string;
  block?: BlockDetail;
  transaction?: ExplorerTransaction;
  address?: AddressBasic;
  error?: string;
}

// Explorer view types
export type ExplorerView = 'blocks' | 'block' | 'transaction' | 'address' | 'search';

// Parent context for back navigation. Discriminated union so each parent kind
// carries exactly the identifier needed to re-fetch its view (block→hash,
// transaction→txid, address→address). The stack (see `parentStack` below) is
// pushed when the user drills DOWN to a different view (block→tx, tx→address,
// etc.) and popped on the Back button — so block→tx→address→Back→tx→Back→
// block→Back→blocks-list walks the chain in reverse. Same-view peer navigation
// (block→block via Prev/Next, tx→tx via sibling pills) deliberately does NOT
// push, so the stack only records vertical descents in the navigation tree.
export type ParentContext =
  | { view: 'block'; blockHash: string }
  | { view: 'transaction'; txid: string }
  | { view: 'address'; address: string };

export interface ExplorerSlice {
  // State
  view: ExplorerView;
  blocks: BlockSummary[];
  currentBlock: BlockDetail | null;
  currentTransaction: ExplorerTransaction | null;
  /**
   * Fast subset (Address + Balance) — populated by GetExplorerAddressBasic.
   * Drives the hero card on the Address Detail page.
   */
  explorerAddressBasic: AddressBasic | null;
  /**
   * Current balance — populated by GetExplorerAddressBalance. Null while the
   * UTXO prefix scan is in flight; the Balance row in the hero card shows a
   * skeleton until this arrives.
   */
  explorerAddressBalance: AddressBalance | null;
  /**
   * Aggregate stats (TxCount, TotalReceived, TotalSent, FirstSeen, LastSeen)
   * — populated by GetExplorerAddressStats. Null while loading; the Activity
   * column on the Address Detail page shows a skeleton until this arrives.
   */
  explorerAddressStats: AddressStats | null;
  searchResult: SearchResult | null;
  searchQuery: string;

  // Parent context stack for back navigation. Bottom of stack = oldest
  // ancestor; top = most recent. Empty stack means Back falls through to the
  // blocks list. See `ParentContext` doc above for the push/pop semantics.
  parentStack: ParentContext[];

  // Search history persisted to localStorage, populated by handleSearch on
  // successful searches only (not_found / errored / cancelled don't push).
  // Capped at MAX_SEARCH_HISTORY (10), newest-first, dedup-on-query.
  searchHistory: SearchHistoryItem[];

  // Pagination
  blocksPage: number;
  blocksPerPage: BlocksPerPageOption;
  totalBlocks: number;

  // Loading states
  isLoadingBlocks: boolean;
  isLoadingBlock: boolean;
  isLoadingTransaction: boolean;
  isLoadingAddress: boolean;
  isLoadingAddressBalance: boolean;
  isLoadingAddressStats: boolean;
  isSearching: boolean;

  // Error states
  error: string | null;

  // Actions
  setView: (view: ExplorerView) => void;
  setBlocks: (blocks: BlockSummary[]) => void;
  setCurrentBlock: (block: BlockDetail | null) => void;
  setCurrentTransaction: (tx: ExplorerTransaction | null) => void;
  setExplorerAddressBasic: (address: AddressBasic | null) => void;
  setExplorerAddressBalance: (balance: AddressBalance | null) => void;
  setExplorerAddressStats: (stats: AddressStats | null) => void;
  setSearchResult: (result: SearchResult | null) => void;
  setSearchQuery: (query: string) => void;
  setBlocksPage: (page: number) => void;
  setBlocksPerPage: (size: BlocksPerPageOption) => void;
  setTotalBlocks: (total: number) => void;
  setLoadingBlocks: (loading: boolean) => void;
  setLoadingBlock: (loading: boolean) => void;
  setLoadingTransaction: (loading: boolean) => void;
  setLoadingAddress: (loading: boolean) => void;
  setLoadingAddressBalance: (loading: boolean) => void;
  setLoadingAddressStats: (loading: boolean) => void;
  setSearching: (searching: boolean) => void;
  setError: (error: string | null) => void;
  // Parent stack actions. `pushParentContext` appends to the stack on
  // drill-down navigation (block→tx, tx→address, etc.). `popParentContext`
  // removes the top entry and returns it (null when empty) — called by the
  // Back button on each detail view. `clearParentStack` resets to [] —
  // called by search-driven navigation (search destinations have no relation
  // to whatever chain the user was on before) and by the default `handleBack`
  // fallback when popping a non-block context that wasn't representable in
  // the prior single-level model.
  pushParentContext: (ctx: ParentContext) => void;
  popParentContext: () => ParentContext | null;
  clearParentStack: () => void;
  // Search history actions. `addToSearchHistory` prepends after deduping on
  // query and caps at MAX_SEARCH_HISTORY, then persists. `clearSearchHistory`
  // wipes state and persists empty array.
  addToSearchHistory: (item: SearchHistoryItem) => void;
  clearSearchHistory: () => void;
  resetExplorer: () => void;
}

export const createExplorerSlice: SliceCreator<ExplorerSlice> = (set) => ({
  // Initial state
  view: 'blocks',
  blocks: [],
  currentBlock: null,
  currentTransaction: null,
  explorerAddressBasic: null,
  explorerAddressBalance: null,
  explorerAddressStats: null,
  searchResult: null,
  searchQuery: '',

  // Parent context stack for back navigation
  parentStack: [],

  // Search history seeded from localStorage on slice init.
  searchHistory: loadSearchHistory(),

  // Pagination
  blocksPage: 0,
  blocksPerPage: loadBlocksPerPage(),
  totalBlocks: 0,

  // Loading states
  isLoadingBlocks: false,
  isLoadingBlock: false,
  isLoadingTransaction: false,
  isLoadingAddress: false,
  isLoadingAddressBalance: false,
  isLoadingAddressStats: false,
  isSearching: false,

  // Error states
  error: null,

  // Actions
  setView: (view) => set({ view }),
  setBlocks: (blocks) => set({ blocks }),
  setCurrentBlock: (block) => set({ currentBlock: block }),
  setCurrentTransaction: (tx) => set({ currentTransaction: tx }),
  setExplorerAddressBasic: (address) => set({ explorerAddressBasic: address }),
  setExplorerAddressBalance: (balance) => set({ explorerAddressBalance: balance }),
  setExplorerAddressStats: (stats) => set({ explorerAddressStats: stats }),
  setSearchResult: (result) => set({ searchResult: result }),
  setSearchQuery: (query) => set({ searchQuery: query }),
  setBlocksPage: (page) => set({ blocksPage: page }),
  setBlocksPerPage: (size) => {
    // Clear `blocks` and flip `isLoadingBlocks: true` in the SAME synchronous
    // `set()` call alongside the page-size change. Without this, the brief
    // window between the page-size mutation and the async refetch (next
    // commit + useEffect tick + Wails RPC await) renders the previous
    // page's rows under the new header/footer (e.g. 25 stale rows visible
    // while the footer advertises page 1 of N at size 100). Codex round-3
    // P2 finding. The skeleton branch in BlockList (`isLoading && blocks
    // .length === 0`) covers the gap until the refetch resolves.
    set({
      blocksPerPage: size,
      blocksPage: 0,
      blocks: [],
      isLoadingBlocks: true,
    });
    try {
      localStorage.setItem(STORAGE_KEY_BLOCKS_PER_PAGE, String(size));
    } catch {
      // Silently fail — corrupted/blocked localStorage.
    }
    // Note: re-fetch is handled by the ExplorerPage's existing useEffect
    // keyed on `fetchBlocks`, whose useCallback identity changes when
    // blocksPerPage changes — so the new size triggers a fresh page-0 fetch.
  },
  setTotalBlocks: (total) => set({ totalBlocks: total }),
  setLoadingBlocks: (loading) => set({ isLoadingBlocks: loading }),
  setLoadingBlock: (loading) => set({ isLoadingBlock: loading }),
  setLoadingTransaction: (loading) => set({ isLoadingTransaction: loading }),
  setLoadingAddress: (loading) => set({ isLoadingAddress: loading }),
  setLoadingAddressBalance: (loading) => set({ isLoadingAddressBalance: loading }),
  setLoadingAddressStats: (loading) => set({ isLoadingAddressStats: loading }),
  setSearching: (searching) => set({ isSearching: searching }),
  setError: (error) => set({ error }),
  // The store is wrapped in Zustand's immer middleware (see useStore.ts).
  // Under immer, set((state) => ...) recipes MUST mutate the draft and
  // either return undefined OR return the draft itself. Returning a new
  // partial object goes through immer's "replace" path and changes the
  // semantics; worse, any draft references read inside the recipe are
  // REVOKED PROXIES after produce() finalizes, so storing them in closure
  // variables and dereferencing them later throws "Cannot perform 'get'
  // on a proxy that has been revoked". Both actions below mutate the
  // draft via array methods (.push / .pop) and snapshot the popped entry
  // as a plain object (spread copy) before the proxy is revoked.
  pushParentContext: (ctx) =>
    set((state) => {
      state.parentStack.push(ctx);
    }),
  popParentContext: () => {
    // `popParentContext` mutates state AND returns the popped entry in one
    // call so callers (specifically `handleBack` in ExplorerPage) can branch
    // on the popped context without a separate read of `parentStack` after
    // the set — which would race the React batching cycle and read the pre-
    // pop value. We snapshot the popped entry as a PLAIN OBJECT (spread
    // copy) inside the recipe so the returned reference survives the
    // produce() proxy revocation. The discriminated-union variants have
    // different shapes (block.blockHash, transaction.txid, address.address),
    // so the spread captures whichever shape is present.
    let popped: ParentContext | null = null;
    set((state) => {
      if (state.parentStack.length === 0) {
        return;
      }
      const last = state.parentStack[state.parentStack.length - 1];
      // Snapshot the discriminated-union variant as a plain object — the
      // draft proxy `last` is revoked after produce() ends, so storing it
      // in the outer `popped` would yield a revoked-proxy TypeError when
      // handleBack later reads `parent.view`. The `as ParentContext` cast
      // re-narrows after the spread (TS widens spread to a flat object
      // type and loses the discriminated-union narrowing).
      popped = { ...last } as ParentContext;
      state.parentStack.pop();
    });
    return popped;
  },
  // Direct partial form — works fine with immer middleware (immer only
  // wraps function-form set calls). Setting an empty array shallowly
  // merges into existing state.
  clearParentStack: () => set({ parentStack: [] }),

  addToSearchHistory: (item) => {
    set((state) => {
      // Dedup on query (filter out existing entries with same query), then
      // prepend the new entry. Cap at MAX_SEARCH_HISTORY (10).
      const deduped = state.searchHistory.filter((e) => e.query !== item.query);
      const next = [item, ...deduped].slice(0, MAX_SEARCH_HISTORY);
      state.searchHistory = next;
      // Snapshot for persistence outside the immer draft (the persisted
      // array reference must be a plain JS array, not an immer proxy).
      persistSearchHistory(next.map((e) => ({ ...e })));
    });
  },
  clearSearchHistory: () => {
    set({ searchHistory: [] });
    persistSearchHistory([]);
  },

  resetExplorer: () =>
    set({
      view: 'blocks',
      blocks: [],
      currentBlock: null,
      currentTransaction: null,
      explorerAddressBasic: null,
      explorerAddressBalance: null,
      explorerAddressStats: null,
      searchResult: null,
      searchQuery: '',
      parentStack: [],
      blocksPage: 0,
      isLoadingBlocks: false,
      isLoadingBlock: false,
      isLoadingTransaction: false,
      isLoadingAddress: false,
      isLoadingAddressBalance: false,
      isLoadingAddressStats: false,
      isSearching: false,
      error: null,
    }),
});
