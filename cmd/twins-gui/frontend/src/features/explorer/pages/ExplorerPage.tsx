import React, { useEffect, useCallback, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useStore } from '@/store/useStore';
import { useShallow } from 'zustand/react/shallow';
import { classifyExplorerQuery } from '@/shared/utils/parseExplorerSearchQuery';
import type { ParentContext } from '@/store/slices/explorerSlice';
import {
  GetLatestBlocks,
  GetExplorerBlock,
  GetExplorerTransaction,
  GetExplorerAddressBasic,
  GetExplorerAddressBalance,
  GetExplorerAddressStats,
  GetAddressTransactions,
  GetAddressUTXOs,
  ExplorerSearch,
} from '@wailsjs/go/main/App';
import { BlockList } from '../components/BlockList';
import { BlockDetail } from '../components/BlockDetail';
import { TransactionDetail } from '../components/TransactionDetail';
import { AddressView } from '../components/AddressView';
import { Banner } from '@/shared/components/Banner';
// Dev-only debug surface for the Inputs/Outputs synthetic fixture catalog.
// Hidden by default. Enable via either:
//   - URL query param: append `?fixtures=1` to the explorer URL (works in
//     `wails dev` browser context).
//   - localStorage flag: `localStorage.setItem('twins_dev_fixtures', '1')`
//     via dev tools console (persists across page reloads).
// Disable by removing the URL param or `localStorage.removeItem(
//   'twins_dev_fixtures')`. Files preserved for future Tx/Block/Address
// detail-view redesign tasks — see
// `cmd/twins-gui/frontend/src/features/explorer/fixtures/`.
import { FixtureOverlay } from '../fixtures/FixtureOverlay';

// Receive design-language tokens. Exported so PRs 2-5 (BlockList/BlockDetail/...) can
// consume the same constants without duplication.
export const pageOuter: React.CSSProperties = {
  height: '100%',
  display: 'flex',
  flexDirection: 'column',
  overflow: 'hidden',
};
export const pageScroll: React.CSSProperties = {
  flex: 1,
  overflow: 'auto',
  padding: '12px 16px',
  display: 'flex',
  flexDirection: 'column',
  gap: '12px',
};
export const cardStyle: React.CSSProperties = {
  backgroundColor: '#2f2f2f',
  border: '1px solid #3a3a3a',
  borderRadius: '8px',
  padding: '12px 16px',
};

export const ExplorerPage: React.FC = () => {
  const { t } = useTranslation('common');
  const {
    view,
    blocks,
    currentBlock,
    currentTransaction,
    explorerAddressBasic,
    explorerAddressBalance,
    explorerAddressStats,
    searchQuery,
    blocksPage,
    blocksPerPage,
    totalBlocks,
    isLoadingBlocks,
    isLoadingBlock,
    isLoadingTransaction,
    isLoadingAddress,
    isLoadingAddressBalance,
    isLoadingAddressStats,
    isSearching,
    error,
    setView,
    setBlocks,
    setCurrentBlock,
    setCurrentTransaction,
    setExplorerAddressBasic,
    setExplorerAddressBalance,
    setExplorerAddressStats,
    setSearchQuery,
    pushParentContext,
    popParentContext,
    clearParentStack,
    addToSearchHistory,
    setBlocksPage,
    setBlocksPerPage,
    setTotalBlocks,
    setLoadingBlocks,
    setLoadingBlock,
    setLoadingTransaction,
    setLoadingAddress,
    setLoadingAddressBalance,
    setLoadingAddressStats,
    setSearching,
    setError,
  } = useStore(useShallow((state) => ({
    view: state.view,
    blocks: state.blocks,
    currentBlock: state.currentBlock,
    currentTransaction: state.currentTransaction,
    explorerAddressBasic: state.explorerAddressBasic,
    explorerAddressBalance: state.explorerAddressBalance,
    explorerAddressStats: state.explorerAddressStats,
    searchQuery: state.searchQuery,
    blocksPage: state.blocksPage,
    blocksPerPage: state.blocksPerPage,
    totalBlocks: state.totalBlocks,
    isLoadingBlocks: state.isLoadingBlocks,
    isLoadingBlock: state.isLoadingBlock,
    isLoadingTransaction: state.isLoadingTransaction,
    isLoadingAddress: state.isLoadingAddress,
    isLoadingAddressBalance: state.isLoadingAddressBalance,
    isLoadingAddressStats: state.isLoadingAddressStats,
    isSearching: state.isSearching,
    error: state.error,
    setView: state.setView,
    setBlocks: state.setBlocks,
    setCurrentBlock: state.setCurrentBlock,
    setCurrentTransaction: state.setCurrentTransaction,
    setExplorerAddressBasic: state.setExplorerAddressBasic,
    setExplorerAddressBalance: state.setExplorerAddressBalance,
    setExplorerAddressStats: state.setExplorerAddressStats,
    setSearchQuery: state.setSearchQuery,
    pushParentContext: state.pushParentContext,
    popParentContext: state.popParentContext,
    clearParentStack: state.clearParentStack,
    addToSearchHistory: state.addToSearchHistory,
    setBlocksPage: state.setBlocksPage,
    setBlocksPerPage: state.setBlocksPerPage,
    setTotalBlocks: state.setTotalBlocks,
    setLoadingBlocks: state.setLoadingBlocks,
    setLoadingBlock: state.setLoadingBlock,
    setLoadingTransaction: state.setLoadingTransaction,
    setLoadingAddress: state.setLoadingAddress,
    setLoadingAddressBalance: state.setLoadingAddressBalance,
    setLoadingAddressStats: state.setLoadingAddressStats,
    setSearching: state.setSearching,
    setError: state.setError,
  })));

  // Fetch latest blocks. Uses a monotonic request-id counter (instead of the
  // prior boolean `isLoadingRef` guard) so a new fetch triggered by a page-
  // size change does NOT get silently dropped if a prior fetch is still in
  // flight. The race scenario the boolean guard had: user changes
  // `blocksPerPage` while a fetch is mid-air → the useEffect keyed on
  // `fetchBlocks` identity re-runs → new `fetchBlocks(0)` call → boolean
  // guard returns early → table renders old page-size rows under the new
  // page-size footer (Codex P2 finding for the m-restyle-explorer-block-list
  // PR). Counter pattern: each invocation captures its own seq id; only the
  // newest fetch's results are committed; stale fetches abandon their
  // setBlocks / setTotalBlocks writes.
  // Debug fixture overlay visibility gate. See the `FixtureOverlay` import
  // comment block at the top of this file for activation instructions.
  // Mount-only evaluation (no dependency on prop changes) — the user enables
  // either via URL param at navigation OR via localStorage flag before page
  // load. Toggling mid-session requires a reload.
  const showFixtures = useMemo(() => {
    if (typeof window === 'undefined') return false;
    try {
      const params = new URLSearchParams(window.location.search);
      if (params.get('fixtures') === '1') return true;
    } catch {
      // window.location.search inaccessible — fall through to localStorage.
    }
    try {
      return localStorage.getItem('twins_dev_fixtures') === '1';
    } catch {
      return false;
    }
  }, []);

  const fetchSeqRef = useRef(0);
  const fetchBlocks = useCallback(async (page: number = 0, options?: { silent?: boolean }) => {
    // Silent path (m-explorer-blocklist-refresh-countdown-and-autopoll
    // 2026-06-03): when BlockList's 60s interval fires a silent refresh,
    // skip the loading flag toggle (so the table doesn't blank on every
    // tick) and suppress error banner writes (the existing table stays
    // rendered; next tick retries). The existing `fetchSeqRef` counter
    // already prevents stale silent responses from overwriting a newer
    // manual fetch or page-change fetch — no extra guard needed here.
    const silent = options?.silent === true;
    const seq = ++fetchSeqRef.current;
    if (!silent) setLoadingBlocks(true);
    setError(null);

    try {
      const offset = page * blocksPerPage;
      const result = await GetLatestBlocks(blocksPerPage, offset);
      // Stale-fetch guard — abandon if a newer fetch superseded this one.
      if (seq !== fetchSeqRef.current) return;
      if (result) {
        setBlocks(result);
        // Update total blocks from first block height if available
        if (result.length > 0 && page === 0) {
          setTotalBlocks(result[0].height + 1);
        }
      }
    } catch (err) {
      if (seq !== fetchSeqRef.current) return;
      console.error('Failed to fetch blocks:', err);
      // Silent fetch errors are suppressed — the existing table stays
      // rendered and the next interval tick retries. Surfacing a banner
      // on every transient network blip would be noisy.
      if (!silent) setError('Failed to fetch blocks');
    } finally {
      // Only the newest fetch clears the loading flag; stale fetches must
      // not flip the spinner off while a newer fetch is still in flight.
      // Silent path never set the loading flag true, so it also doesn't
      // need to flip it false.
      if (!silent && seq === fetchSeqRef.current) {
        setLoadingBlocks(false);
      }
    }
  }, [blocksPerPage, setBlocks, setTotalBlocks, setLoadingBlocks, setError]);

  // Fetch block details (from block list - no parent, or from navigation like Previous/Next)
  // Independent monotonic counter for single-block fetches (mirrors the
  // fetchSeqRef pattern above on fetchBlocks). The auto-refresh interval on
  // BlockDetail.tsx (60s) fires `fetchBlock` in the background, so a slow
  // response for block A could otherwise clobber a navigation to block B if
  // the user clicks Prev/Next mid-flight. Each call captures its own seq id;
  // only the newest fetch commits its result.
  const fetchBlockSeqRef = useRef(0);
  // viewRef mirrors `view` so the in-flight silent fetchBlock can check the
  // CURRENT view at response time (the closure captures the view value at
  // call time, which is stale by the time the RPC resolves). Without this,
  // an auto-refresh fired while view==='block' would still write
  // setCurrentBlock when the user has since navigated to transaction/address,
  // visibly snapping back the wrong card.
  const viewRef = useRef(view);
  useEffect(() => { viewRef.current = view; }, [view]);
  // Mirror of currentBlock.hash so a silent fetchBlock can confirm that the
  // user is still viewing the block whose hash it just fetched. Without this
  // a slow auto-refresh for block A could resolve after the user Prev/Next-ed
  // to block B and overwrite B's data with A's.
  const currentBlockHashRef = useRef<string | undefined>(currentBlock?.hash);
  useEffect(() => { currentBlockHashRef.current = currentBlock?.hash; }, [currentBlock?.hash]);
  // Same identity-guard pattern as currentBlockHashRef but for transactions —
  // a silent fetchTransaction can confirm that the user is still viewing the
  // tx whose txid it just fetched. Symmetric with the block path; required
  // for TransactionDetail's 60s auto-refresh tick.
  const currentTxidRef = useRef<string | undefined>(currentTransaction?.txid);
  useEffect(() => { currentTxidRef.current = currentTransaction?.txid; }, [currentTransaction?.txid]);
  // Sibling-tx list for the Transaction Detail view's Prev/Next pills.
  // Populated from the parent block's `txids[]` whenever the user is viewing
  // a transaction that has a known `block_hash`. Null when the user is not
  // on a tx view, when the current tx is mempool-only (no block_hash), or
  // while the silent background fetch of the parent block is in flight.
  // The two Prev/Next pills render disabled during the in-flight window and
  // also disabled when the current tx is the first/last sibling in the
  // block (handled inside TransactionDetail via indexOf bounds checks).
  const [siblingTxids, setSiblingTxids] = useState<string[] | null>(null);
  // Type discriminator for the search not-found banner — set in the handleSearch
  // `not_found` branch alongside setError, used to switch the banner render
  // from a plain string Banner to a structured one with "Supported formats"
  // chips + Try again button. Cleared whenever `error` flips back to null
  // (other setError callers do not populate this).
  const [searchNotFoundType, setSearchNotFoundType] =
    useState<'block_height' | 'block_or_tx_hash' | 'address' | 'unknownFormat' | null>(null);

  // Clear searchNotFoundType whenever the slice error flips back to null
  // (other setError(null) callers don't know about this local state).
  useEffect(() => {
    if (!error && searchNotFoundType !== null) {
      setSearchNotFoundType(null);
    }
  }, [error, searchNotFoundType]);

  // "Try again" focuses the search input via a custom DOM event that
  // SearchBar listens for. Avoids ref-forwarding through BlockList.
  const handleTryAgain = useCallback(() => {
    window.dispatchEvent(new CustomEvent('explorer:focus-search'));
  }, []);
  // Monotonic seq counter for the sibling-block background fetch. Mirrors
  // the established fetchBlockSeqRef pattern: a slow background fetch for
  // block A must not commit its txids when the user has since navigated
  // away to a tx in block B.
  const fetchSiblingBlockSeqRef = useRef(0);

  // Mirror of explorerAddressBasic?.address. Used by buildSourceContext below
  // so the parent-stack push reads a fresh source identifier even when the
  // fetch callback's useCallback dep array does not list explorerAddressBasic
  // (the existing fetchAddress callback's deps omit it for stability).
  const currentAddressRef = useRef<string | undefined>(explorerAddressBasic?.address);
  useEffect(() => { currentAddressRef.current = explorerAddressBasic?.address; }, [explorerAddressBasic?.address]);

  // Build a ParentContext for the CURRENT view (the source of the impending
  // navigation). Returns null when on the blocks list (nothing to push — the
  // blocks list is the bottom of the navigation tree) or when the current
  // view has no resolvable identifier (defensive — shouldn't happen during
  // normal flow). Called by fetchBlock / fetchTransaction / fetchAddress
  // synchronously at call time, before the await, so the captured source is
  // the view the user just clicked AWAY from.
  const buildSourceContext = useCallback((): ParentContext | null => {
    const sourceView = viewRef.current;
    if (sourceView === 'block' && currentBlockHashRef.current) {
      return { view: 'block', blockHash: currentBlockHashRef.current };
    }
    if (sourceView === 'transaction' && currentTxidRef.current) {
      return { view: 'transaction', txid: currentTxidRef.current };
    }
    if (sourceView === 'address' && currentAddressRef.current) {
      return { view: 'address', address: currentAddressRef.current };
    }
    return null;
  }, []);
  const fetchBlock = useCallback(async (
    query: string,
    options?: { silent?: boolean; skipPush?: boolean },
  ) => {
    const silent = options?.silent === true;
    const skipPush = options?.skipPush === true;
    const seq = ++fetchBlockSeqRef.current;
    if (!silent) {
      // Foreground/navigation path: push the current view onto the parent
      // stack if this is a drill-down into a different view kind (e.g.
      // tx→block via the Block #N pill). Same-view block-to-block navigation
      // (e.g. block list → block, or block Prev/Next) is treated as peer
      // navigation and intentionally does NOT push — the prior block falls
      // out of the chain, mirroring how browser-pagination back-stacks work.
      // skipPush=true is set by handleBack so popping the stack and
      // re-fetching the popped block does not re-push that block onto the
      // stack we just popped from.
      if (!skipPush) {
        const source = buildSourceContext();
        if (source && source.view !== 'block') {
          pushParentContext(source);
        }
      }
      setLoadingBlock(true);
    }
    setError(null);

    try {
      const result = await GetExplorerBlock(query);
      if (seq !== fetchBlockSeqRef.current) return;
      if (silent) {
        // Silent refresh: only commit if the user is still on the block view
        // for the block we fetched. Without this guard, an auto-refresh that
        // resolves AFTER the user navigated to a tx/address would force them
        // back to block view (codex round-3 critical: setView would snap).
        if (result && viewRef.current === 'block' && currentBlockHashRef.current === query) {
          setCurrentBlock(result);
        }
      } else if (result) {
        setCurrentBlock(result);
        setView('block');
      }
    } catch (err) {
      if (seq !== fetchBlockSeqRef.current) return;
      console.error('Failed to fetch block:', err);
      // Silent fetch errors should not surface a banner — the existing block
      // is still displayed and the next tick will retry.
      if (!silent) setError('Block not found');
    } finally {
      if (!silent && seq === fetchBlockSeqRef.current) {
        setLoadingBlock(false);
      }
    }
  }, [buildSourceContext, pushParentContext, setCurrentBlock, setView, setLoadingBlock, setError]);

  // Fetch transaction details (from block - parent is block).
  // Mirrors the fetchBlock seq/silent pattern above. Silent path (used by
  // TransactionDetail's 60s auto-refresh tick): skips loading state +
  // skips parent-context mutation + commits result only if user is still
  // viewing the same tx, and never switches view back to 'transaction'
  // if the user navigated away (e.g. to an input/output address) during
  // the in-flight RPC.
  const fetchTransactionSeqRef = useRef(0);
  const fetchTransaction = useCallback(async (
    txid: string,
    options?: { silent?: boolean; skipPush?: boolean },
  ): Promise<void> => {
    const silent = options?.silent === true;
    const skipPush = options?.skipPush === true;
    const seq = ++fetchTransactionSeqRef.current;
    if (!silent) {
      // Foreground/navigation path: push the current view onto the parent
      // stack on drill-down (block→tx, address→tx). Same-view tx-to-tx
      // navigation (via the new Prev/Next sibling pills on TransactionDetail
      // or via clicking an input's source-tx link) is peer navigation and
      // does NOT push — the prior tx falls out of the chain. skipPush is
      // used by handleBack to prevent re-pushing the popped context.
      if (!skipPush) {
        const source = buildSourceContext();
        if (source && source.view !== 'transaction') {
          pushParentContext(source);
        }
      }
      setLoadingTransaction(true);
    }
    setError(null);

    try {
      const result = await GetExplorerTransaction(txid);
      if (seq !== fetchTransactionSeqRef.current) return;
      if (silent) {
        // Silent refresh: only commit if the user is still on the tx view
        // for the tx we fetched. Without this guard, an auto-refresh that
        // resolves AFTER the user navigated to an address would force them
        // back to the tx view.
        if (result && viewRef.current === 'transaction' && currentTxidRef.current === txid) {
          setCurrentTransaction(result);
        }
      } else if (result) {
        setCurrentTransaction(result);
        setView('transaction');
      }
    } catch (err) {
      if (seq !== fetchTransactionSeqRef.current) return;
      console.error('Failed to fetch transaction:', err);
      // Silent fetch errors should not surface a banner — the existing tx is
      // still displayed and the next tick will retry.
      if (!silent) setError('Transaction not found');
    } finally {
      if (!silent && seq === fetchTransactionSeqRef.current) {
        setLoadingTransaction(false);
      }
    }
  }, [buildSourceContext, pushParentContext, setCurrentTransaction, setView, setLoadingTransaction, setError]);

  // Fetch address info — split into two independent calls per the
  // `m-explorer-address-detail-split-fast-slow-fetch` task. Basic (address +
  // balance) is fast and unblocks the hero card; Stats (txcount/totals/first
  // seen/last seen) walks the address tx index and is slow on high-traffic
  // addresses, so the Activity column shows a skeleton until it arrives.
  //
  // Monotonic seq-counter guard mirrors the established `fetchSeqRef` /
  // `fetchBlockSeqRef` / `fetchTransactionSeqRef` pattern in this file:
  // every fetchAddress / refreshAddressInfo / search-address dispatch
  // captures its own seq id and only commits when its seq is still the
  // newest. Without this, a slow stats response for address A could land
  // AFTER the user navigated to address B and clear the new `statsLoading`
  // spinner via the stale `.finally(() => setLoadingAddressStats(false))`.
  const fetchAddressSeqRef = useRef(0);

  // Monotonic seq counter for the search dispatch. Unlike the sibling fetch
  // callbacks above, `handleSearch` previously had no race-safety guard:
  // submit-spam could let a stale ExplorerSearch response overwrite a newer
  // commit (last-writer-wins). Mirrored from fetchSeqRef pattern.
  const searchSeqRef = useRef(0);

  const fetchAddress = useCallback(async (
    address: string,
    options?: { skipPush?: boolean },
  ) => {
    const skipPush = options?.skipPush === true;
    const seq = ++fetchAddressSeqRef.current;

    // Push the current view onto the parent stack on drill-down (block→
    // address via reward-card click, tx→address via input/output click).
    // Same-view address-to-address navigation does NOT push. This is the
    // direct fix for bug #1 ("address Back goes to blocks list instead of
    // the originating block / tx") — the prior single-level setParentContext
    // pattern was never invoked from fetchAddress, so the parent was always
    // null when handleBack ran on the address view. skipPush=true is set by
    // handleBack so popping address from the stack does not re-push.
    if (!skipPush) {
      const source = buildSourceContext();
      if (source && source.view !== 'address') {
        pushParentContext(source);
      }
    }

    setLoadingAddress(true);
    setLoadingAddressBalance(true);
    setLoadingAddressStats(true);
    setExplorerAddressBalance(null);
    setExplorerAddressStats(null);
    setError(null);

    // Fire all three in parallel; each commit and each loading-flag clear is
    // guarded by `seq === fetchAddressSeqRef.current` so a stale promise
    // from a prior fetchAddress cannot clobber the current address state.
    const basicPromise = GetExplorerAddressBasic(address)
      .then((basic) => {
        if (basic && seq === fetchAddressSeqRef.current) {
          setExplorerAddressBasic(basic);
          setView('address');
        }
      })
      .catch((err) => {
        console.error('Failed to fetch address basic:', err);
        if (seq === fetchAddressSeqRef.current) {
          setError('Address not found');
        }
      })
      .finally(() => {
        if (seq === fetchAddressSeqRef.current) {
          setLoadingAddress(false);
        }
      });

    const balancePromise = GetExplorerAddressBalance(address)
      .then((balance) => {
        if (balance && seq === fetchAddressSeqRef.current) {
          setExplorerAddressBalance(balance);
        }
      })
      .catch((err) => {
        console.error('Failed to fetch address balance:', err);
        // Don't surface balance errors as a banner — basic is the primary
        // data; missing balance leaves the Balance row at em-dash.
      })
      .finally(() => {
        if (seq === fetchAddressSeqRef.current) {
          setLoadingAddressBalance(false);
        }
      });

    const statsPromise = GetExplorerAddressStats(address)
      .then((stats) => {
        if (stats && seq === fetchAddressSeqRef.current) {
          setExplorerAddressStats(stats);
        }
      })
      .catch((err) => {
        console.error('Failed to fetch address stats:', err);
        // Don't surface stats errors as a banner — basic is the primary
        // data; missing stats just leaves the Activity column at "N/A".
      })
      .finally(() => {
        if (seq === fetchAddressSeqRef.current) {
          setLoadingAddressStats(false);
        }
      });

    await Promise.all([basicPromise, balancePromise, statsPromise]);
  }, [
    buildSourceContext,
    pushParentContext,
    setExplorerAddressBasic,
    setExplorerAddressBalance,
    setExplorerAddressStats,
    setView,
    setLoadingAddress,
    setLoadingAddressBalance,
    setLoadingAddressStats,
    setError,
  ]);

  // Callbacks passed down to AddressView for paginated per-page fetches.
  // AddressView owns its own pagination state; these are thin wrappers over
  // the Wails bindings that compute offset from page/pageSize.
  const handleFetchAddressTransactions = useCallback(
    (page: number, pageSize: number) => {
      const address = explorerAddressBasic?.address;
      if (!address) {
        return Promise.resolve({ transactions: [], total: 0, has_more: false } as any);
      }
      return GetAddressTransactions(address, pageSize, (page - 1) * pageSize);
    },
    [explorerAddressBasic?.address],
  );

  const handleFetchAddressUTXOs = useCallback(
    (page: number, pageSize: number) => {
      const address = explorerAddressBasic?.address;
      if (!address) {
        return Promise.resolve({ utxos: [], total: 0, has_more: false } as any);
      }
      return GetAddressUTXOs(address, pageSize, (page - 1) * pageSize);
    },
    [explorerAddressBasic?.address],
  );

  // Re-fetch basic + balance + stats without touching the 'address' view state.
  // Called by AddressView on auto-refresh tick and manual refresh click. Fires
  // all three Wails calls in parallel under the same `fetchAddressSeqRef` seq
  // counter as `fetchAddress` — every `.then` commit is guarded by
  // `seq === fetchAddressSeqRef.current` so a navigation to a different
  // address (which bumps the seq via fetchAddress) cancels any in-flight
  // refresh from the prior address. Refresh does NOT toggle the loading flags
  // because the previous values stay rendered until the new commit lands.
  const refreshAddressInfo = useCallback(async () => {
    const address = explorerAddressBasic?.address;
    if (!address) return;
    const seq = ++fetchAddressSeqRef.current;

    const basicPromise = GetExplorerAddressBasic(address)
      .then((basic) => {
        if (basic && seq === fetchAddressSeqRef.current) {
          setExplorerAddressBasic(basic);
        }
      })
      .catch((err) => {
        console.error('Failed to refresh address basic:', err);
      });

    const balancePromise = GetExplorerAddressBalance(address)
      .then((balance) => {
        if (balance && seq === fetchAddressSeqRef.current) {
          setExplorerAddressBalance(balance);
        }
      })
      .catch((err) => {
        console.error('Failed to refresh address balance:', err);
      });

    const statsPromise = GetExplorerAddressStats(address)
      .then((stats) => {
        if (stats && seq === fetchAddressSeqRef.current) {
          setExplorerAddressStats(stats);
        }
      })
      .catch((err) => {
        console.error('Failed to refresh address stats:', err);
      });

    await Promise.all([basicPromise, balancePromise, statsPromise]);
  }, [
    explorerAddressBasic?.address,
    setExplorerAddressBasic,
    setExplorerAddressBalance,
    setExplorerAddressStats,
  ]);

  // Sibling-tx fetch effect. Keeps `siblingTxids` in sync with the parent
  // block of the currently-viewed transaction. Three cases:
  //
  //   1. Not on a tx view, or currentTransaction is null, or the tx has no
  //      block_hash (mempool) → siblingTxids := null. Prev/Next render
  //      disabled.
  //   2. The parent block is already in store and matches the tx's block_hash
  //      → siblingTxids := currentBlock.txids. Prev/Next become active.
  //   3. The parent block is NOT in store (or stale relative to the tx) →
  //      fire a silent GetExplorerBlock(block_hash) under a dedicated seq
  //      counter and commit on resolve. The fetch is silent (does not touch
  //      currentBlock or any loading flags) because we only want the txids
  //      list for Prev/Next computation, not to swap the user's view.
  //
  // Effect key includes both currentBlock?.hash (so case 2 fires when the
  // parent block lands via a prior navigation) and currentTransaction's
  // identity fields (so case 1/3 fires on tx navigation). The seq counter
  // protects against races where the user navigates to a different tx
  // mid-fetch.
  useEffect(() => {
    if (view !== 'transaction' || !currentTransaction) {
      setSiblingTxids(null);
      return;
    }
    const blockHash = currentTransaction.block_hash;
    if (!blockHash) {
      // Mempool tx — no parent block to enumerate. Both pills disabled.
      setSiblingTxids(null);
      return;
    }
    if (currentBlock && currentBlock.hash === blockHash) {
      // Parent block already in store. The `?? []` fallback covers
      // hypothetical pre-enrichment cached responses with undefined txids.
      setSiblingTxids(currentBlock.txids ?? []);
      return;
    }
    // Parent block not in store (or stale). Fire silent background fetch.
    const seq = ++fetchSiblingBlockSeqRef.current;
    // Reset to null so the pills disable while in flight — prevents a
    // stale txids list (from a prior tx's parent block) from being used
    // for the new tx's Prev/Next computation.
    setSiblingTxids(null);
    GetExplorerBlock(blockHash)
      .then((result) => {
        if (seq !== fetchSiblingBlockSeqRef.current) return;
        if (result) {
          setSiblingTxids(result.txids ?? []);
        }
      })
      .catch((err) => {
        if (seq !== fetchSiblingBlockSeqRef.current) return;
        console.error('Failed to fetch sibling block for Prev/Next:', err);
        // Leave siblingTxids as null; pills stay disabled.
      });
  }, [view, currentTransaction, currentBlock]);

  // Search handler — race-safety guarded via searchSeqRef. Submit-spam keeps
  // only the latest dispatch's response committed (last-writer-wins is the
  // bug we are fixing). Mirrors the established fetchSeqRef / fetchBlockSeqRef
  // / fetchTransactionSeqRef pattern in this file.
  const handleSearch = useCallback(async (query: string) => {
    const trimmed = query.trim();
    if (!trimmed) return;

    const seq = ++searchSeqRef.current;
    setSearching(true);
    setSearchQuery(query);
    setError(null);
    setSearchNotFoundType(null);

    try {
      const result = await ExplorerSearch(query);
      // Stale-fetch guard — abandon if a newer search superseded this one.
      if (seq !== searchSeqRef.current) return;
      if (result) {
        switch (result.type) {
          case 'block':
            if (result.block) {
              // Search destinations have no relation to whatever chain the
              // user was on before — clear the parent stack so Back from a
              // search result returns to the blocks list, not to an
              // unrelated stale ancestor.
              clearParentStack();
              setCurrentBlock(result.block);
              setView('block');
              addToSearchHistory({
                query: trimmed,
                type: 'block',
                timestamp: Date.now(),
                label: String(result.block.height),
              });
            }
            break;
          case 'transaction':
            if (result.transaction) {
              clearParentStack();
              setCurrentTransaction(result.transaction);
              setView('transaction');
              addToSearchHistory({
                query: trimmed,
                type: 'transaction',
                timestamp: Date.now(),
              });
            }
            break;
          case 'address':
            if (result.address) {
              // Backend SearchExplorer returns AddressBasic (fast subset) —
              // commit it directly, then dispatch a stats fetch under the
              // shared fetchAddressSeqRef seq so rapid search→navigate
              // sequences can't clear `statsLoading` for a fresh address.
              clearParentStack();
              const addrSeq = ++fetchAddressSeqRef.current;
              addToSearchHistory({
                query: trimmed,
                type: 'address',
                timestamp: Date.now(),
              });
              setExplorerAddressBasic(result.address);
              setExplorerAddressBalance(null);
              setExplorerAddressStats(null);
              setLoadingAddressBalance(true);
              setLoadingAddressStats(true);
              setView('address');
              const addr = result.address.address;
              GetExplorerAddressBalance(addr)
                .then((balance) => {
                  if (balance && addrSeq === fetchAddressSeqRef.current) {
                    setExplorerAddressBalance(balance);
                  }
                })
                .catch((err) => {
                  console.error('Failed to fetch address balance from search:', err);
                })
                .finally(() => {
                  if (addrSeq === fetchAddressSeqRef.current) {
                    setLoadingAddressBalance(false);
                  }
                });
              GetExplorerAddressStats(addr)
                .then((stats) => {
                  if (stats && addrSeq === fetchAddressSeqRef.current) {
                    setExplorerAddressStats(stats);
                  }
                })
                .catch((err) => {
                  console.error('Failed to fetch address stats from search:', err);
                })
                .finally(() => {
                  if (addrSeq === fetchAddressSeqRef.current) {
                    setLoadingAddressStats(false);
                  }
                });
            }
            break;
          case 'not_found': {
            // Type-aware not-found copy. The classifier mirrors the backend
            // dispatch order, so the message tells the user what kind of
            // lookup actually ran and failed instead of a generic banner.
            const detection = classifyExplorerQuery(trimmed);
            let message: string;
            let nfType: 'block_height' | 'block_or_tx_hash' | 'address' | 'unknownFormat';
            switch (detection.type) {
              case 'block_height':
                message = t('explorer.search.notFound.lookedForBlockHeight', { value: detection.value });
                nfType = 'block_height';
                break;
              case 'block_or_tx_hash':
                message = t('explorer.search.notFound.lookedForBlockOrTxHash', { value: detection.value });
                nfType = 'block_or_tx_hash';
                break;
              case 'address':
                message = t('explorer.search.notFound.lookedForAddress', { value: detection.value });
                nfType = 'address';
                break;
              default:
                message = t('explorer.search.notFound.unknownFormat');
                nfType = 'unknownFormat';
                break;
            }
            setError(message);
            setSearchNotFoundType(nfType);
            break;
          }
        }
      }
    } catch (err) {
      if (seq !== searchSeqRef.current) return;
      console.error('Search failed:', err);
      setError(t('explorer.search.error.searchFailed'));
    } finally {
      // Only the latest dispatch clears the loading flag — mirrors
      // fetchBlocks/fetchBlock finally-branch guard.
      if (seq === searchSeqRef.current) {
        setSearching(false);
      }
    }
  }, [t, setSearchQuery, setSearching, setCurrentBlock, setCurrentTransaction, setExplorerAddressBasic, setExplorerAddressBalance, setExplorerAddressStats, setLoadingAddressBalance, setLoadingAddressStats, setView, setError, clearParentStack, addToSearchHistory]);

  // Initial fetch
  useEffect(() => {
    fetchBlocks(0);
  }, [fetchBlocks]);

  // Page change handler
  const handlePageChange = (page: number) => {
    setBlocksPage(page);
    fetchBlocks(page);
  };

  // Navigation handlers
  const handleBlockClick = (query: string) => {
    fetchBlock(query);
  };

  const handleTxClick = (txid: string) => {
    fetchTransaction(txid);
  };

  const handleAddressClick = (address: string) => {
    fetchAddress(address);
  };

  const handleBack = useCallback(async () => {
    setError(null);

    // Pop one entry off the parent stack. If empty, fall through to the
    // blocks list (the default ancestor at the bottom of the navigation
    // tree). When non-null, branch on the popped view kind and re-fetch
    // the ancestor's data — passing skipPush=true so the fetch does not
    // re-add the popped context back onto the stack we just popped from.
    const parent = popParentContext();
    if (parent === null) {
      // Default: blocks list. Clear all detail-view state so a subsequent
      // navigation starts from a clean slate. clearParentStack is a no-op
      // here (we just confirmed the stack is empty) but kept for symmetry
      // with the prior behavior of resetting context on the default branch.
      setView('blocks');
      setCurrentBlock(null);
      setCurrentTransaction(null);
      setExplorerAddressBasic(null);
      setExplorerAddressStats(null);
      clearParentStack();
      return;
    }

    if (parent.view === 'block') {
      // skipPush=true: fetchBlock would otherwise see viewRef.current === the
      // view we're leaving (transaction/address) and push it onto the stack,
      // canceling the pop we just did.
      await fetchBlock(parent.blockHash, { skipPush: true });
      return;
    }
    if (parent.view === 'transaction') {
      await fetchTransaction(parent.txid, { skipPush: true });
      return;
    }
    if (parent.view === 'address') {
      await fetchAddress(parent.address, { skipPush: true });
      return;
    }
    // Unreachable for ParentContext discriminated union, but defensive
    // fallback if a future view kind is added to the union without updating
    // this switch: drop to the blocks list rather than getting stuck on the
    // current view.
    setView('blocks');
  }, [popParentContext, clearParentStack, fetchBlock, fetchTransaction, fetchAddress, setView, setCurrentBlock, setCurrentTransaction, setExplorerAddressBasic, setExplorerAddressStats, setError]);

  // Refresh handler. Optional `{ silent: true }` is used by BlockDetail's
  // 60s auto-refresh tick to avoid (a) flipping the loading flag, which
  // would blank the page on every tick, and (b) snapping the view back to
  // 'block' when the response lands after the user navigated to a tx /
  // address (codex round-3 critical + W1). Manual user-initiated refresh
  // (click on the RefreshCountdown ring) calls with no options — full
  // foreground flow with loading state.
  // Returns the in-flight fetch promise (or `undefined` when no fetch is
  // applicable) so callers — specifically BlockDetail's auto-refresh tick —
  // can await completion to avoid tick-stacking when the RPC exceeds the
  // 60s interval (codex round-4 W1).
  const handleRefresh = (options?: { silent?: boolean }): Promise<void> | void => {
    switch (view) {
      case 'blocks':
        return fetchBlocks(blocksPage, options);
      case 'block':
        return currentBlock ? fetchBlock(currentBlock.hash, options) : undefined;
      case 'transaction':
        return currentTransaction ? fetchTransaction(currentTransaction.txid, options) : undefined;
      case 'address':
        return explorerAddressBasic ? fetchAddress(explorerAddressBasic.address) : undefined;
    }
  };

  const isAnyLoading = isLoadingBlocks || isLoadingBlock || isLoadingTransaction || isLoadingAddress || isSearching;

  // Render current view
  const renderView = () => {
    switch (view) {
      case 'block':
        return (
          <BlockDetail
            block={currentBlock}
            isLoading={isLoadingBlock}
            onTxClick={handleTxClick}
            onBlockClick={handleBlockClick}
            onAddressClick={handleAddressClick}
            onBack={handleBack}
            onRefresh={handleRefresh}
            isAnyLoading={isAnyLoading}
          />
        );
      case 'transaction':
        return (
          <TransactionDetail
            transaction={currentTransaction}
            isLoading={isLoadingTransaction}
            onAddressClick={handleAddressClick}
            onTxClick={handleTxClick}
            onBlockClick={handleBlockClick}
            onBack={handleBack}
            onRefresh={handleRefresh}
            isAnyLoading={isAnyLoading}
            siblingTxids={siblingTxids}
            onSiblingTxClick={handleTxClick}
          />
        );
      case 'address':
        return (
          <AddressView
            addressBasic={explorerAddressBasic}
            addressBalance={explorerAddressBalance}
            addressStats={explorerAddressStats}
            balanceLoading={isLoadingAddressBalance}
            statsLoading={isLoadingAddressStats}
            isLoading={isLoadingAddress}
            onTxClick={handleTxClick}
            onBack={handleBack}
            onRefresh={handleRefresh as () => void}
            isAnyLoading={isAnyLoading}
            onFetchTransactions={handleFetchAddressTransactions}
            onFetchUTXOs={handleFetchAddressUTXOs}
            onRefreshAddressInfo={refreshAddressInfo}
          />
        );
      default:
        return (
          <BlockList
            blocks={blocks}
            isLoading={isLoadingBlocks}
            currentPage={blocksPage}
            totalBlocks={totalBlocks}
            blocksPerPage={blocksPerPage}
            onBlockClick={handleBlockClick}
            onPageChange={handlePageChange}
            onBlocksPerPageChange={setBlocksPerPage}
            searchQuery={searchQuery}
            isSearching={isSearching}
            onSearch={handleSearch}
            onSearchChange={setSearchQuery}
            onRefresh={handleRefresh}
            isAnyLoading={isAnyLoading}
          />
        );
    }
  };

  return (
    <div style={pageOuter}>
      <div style={pageScroll}>
        {/* Debug fixture overlay — gated by URL query param OR localStorage
            flag (see FixtureOverlay import comment above for activation). */}
        {showFixtures && <FixtureOverlay />}

        {/* Error Message — polished structured form when the error came from
            a search not_found result (searchNotFoundType non-null), plain
            message form otherwise. */}
        {error && searchNotFoundType ? (
          <Banner variant="error" message={error}>
            <div
              style={{
                marginTop: '8px',
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                gap: '6px',
              }}
            >
              <div style={{ fontSize: '11px', color: '#ddd' }}>
                {t('explorer.search.notFound.supportedFormats')}
              </div>
              <div
                style={{
                  display: 'flex',
                  flexWrap: 'wrap',
                  gap: '6px',
                  justifyContent: 'center',
                }}
              >
                {(['blockHeight', 'blockHash', 'txid', 'address'] as const).map((k) => (
                  <span
                    key={k}
                    style={{
                      backgroundColor: '#252525',
                      border: '1px solid #3a3a3a',
                      borderRadius: '999px',
                      padding: '2px 8px',
                      fontSize: '11px',
                      color: '#ddd',
                    }}
                  >
                    {t(`explorer.search.notFound.formatLabels.${k}`)}
                  </span>
                ))}
              </div>
              <button
                type="button"
                onClick={handleTryAgain}
                style={{
                  marginTop: '4px',
                  background: 'transparent',
                  border: 'none',
                  color: '#6699cc',
                  fontSize: '11px',
                  cursor: 'pointer',
                  textDecoration: 'underline',
                  padding: 0,
                }}
              >
                {t('explorer.search.notFound.tryAgain')}
              </button>
            </div>
          </Banner>
        ) : (
          error && <Banner variant="error" message={error} />
        )}

        {/* Content Area — sub-views render unchanged via renderView() switch.
            SearchBar lives inside BlockList only; Refresh lives inside each
            sub-view header (per m-explorer-search-bar-scope-to-blocklist). */}
        <div style={{ flex: 1, minHeight: 0, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
          {renderView()}
        </div>
      </div>
    </div>
  );
};

export { ExplorerPage as Explorer };
