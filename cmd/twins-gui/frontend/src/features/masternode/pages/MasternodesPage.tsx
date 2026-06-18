import React, { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Plus } from 'lucide-react';
import { useMasternodes, useNotifications } from '@/store/useStore';
import { Masternode, NetworkMasternode, MasternodeStatistics } from '@/shared/types/masternode.types';
import { SimpleConfirmDialog } from '@/shared/components/SimpleConfirmDialog';
import { Banner } from '@/shared/components/Banner';
import { UnlockWalletDialog } from '@/features/wallet/components/UnlockWalletDialog';
import { sanitizeErrorMessage } from '@/shared/utils/sanitize';
import { GetMyMasternodes, StartMasternode, DeleteMasternodeConfig, GetNetworkMasternodes, GetMasternodeStatistics, IsDebugCollectorActive } from '@wailsjs/go/main/App';
import { useWalletAction } from '@/shared/hooks/useWalletAction';
import { EventsOn, EventsOff } from '@wailsjs/runtime/runtime';
import {
  MasternodesTable,
  MasternodeEditDialog,
  MasternodeSetupWizard,
  NetworkMasternodesTable,
  NetworkMasternodesFilters,
  MasternodeStatisticsPanel,
  MasternodeDebugPanel,
  PaymentStatsTab,
  type SortColumn,
  type SortDirection,
  type NetworkSortColumn,
} from '../components';

// Auto-refresh interval from Qt: MY_MASTERNODELIST_UPDATE_SECONDS = 60
const MY_MASTERNODES_REFRESH_SECONDS = 60;

// Network masternodes refresh interval
const NETWORK_REFRESH_SECONDS = 60;

// Confirmation dialog types — narrowed to start_alias only after
// m-masternodes-actions-restructure (2026-06-11) dropped bulk Start All /
// Start MISSING buttons. Bulk operations were redundant with per-row Play
// IconButton; keeping the type as a union makes the future re-introduction
// of a bulk action mechanical.
type ConfirmAction = 'start_alias' | null;

// Pending action after wallet unlock
type PendingAction = 'start_alias' | null;

// Map backend MyMasternode to frontend Masternode type
const mapToMasternode = (mn: any): Masternode => ({
  id: mn.alias || mn.id || '',
  alias: mn.alias || '',
  address: mn.address || '',
  protocol: mn.protocol || 0,
  status: mn.status || 'MISSING',
  activeTime: mn.activeTime || mn.active_time || mn.active_seconds || 0,
  lastSeen: mn.lastSeen || mn.last_seen || new Date(),
  tier: mn.tier || 'bronze',
  txHash: mn.txHash || mn.tx_hash || '',
  outputIndex: mn.outputIndex || mn.output_index || 0,
  collateralAddress: mn.collateralAddress || mn.collateral_address || '',
  rewards: mn.rewards || 0,
});

// Map backend MasternodeInfo to frontend NetworkMasternode type with validation
// Backend returns core.MasternodeInfo with json tags matching our field names
const mapToNetworkMasternode = (mn: any): NetworkMasternode | null => {
  // Validate required fields exist
  if (typeof mn.rank !== 'number' || typeof mn.addr !== 'string' || typeof mn.status !== 'string') {
    return null;
  }
  return {
    rank: mn.rank,
    txhash: mn.txhash || '',
    outidx: mn.outidx || 0,
    status: mn.status,
    addr: mn.addr,
    version: mn.version || 0,
    lastseen: mn.lastseen || '',
    activetime: mn.activetime || 0,
    lastpaid: mn.lastpaid || '',
    tier: mn.tier || '',
    paymentaddress: mn.paymentaddress || '',
    pubkey: mn.pubkey || '',
    pubkey_operator: mn.pubkey_operator || '',
  };
};

export const MasternodesPage: React.FC = () => {
  const { t } = useTranslation('masternode');
  const { addNotification } = useNotifications();
  const {
    masternodes,
    isLoading,
    isStartingMasternode,
    setMasternodes,
    setLoading,
    setStartingMasternode,
    setLastRefresh,
    // Network masternodes state
    networkMasternodes,
    isLoadingNetwork,
    networkFilters,
    masternodeActiveTab,
    setNetworkMasternodes,
    setLoadingNetwork,
    setNetworkLastRefresh,
    setNetworkFilters,
    setMasternodeActiveTab,
    getFilteredNetworkMasternodes,
    getNetworkMasternodeCount,
  } = useMasternodes();

  // Sorting state for My Masternodes tab
  const [sortColumn, setSortColumn] = useState<SortColumn>('alias');
  const [sortDirection, setSortDirection] = useState<SortDirection>('asc');

  // Auto-refresh countdown state for My Masternodes
  const [myCountdown, setMyCountdown] = useState<number>(MY_MASTERNODES_REFRESH_SECONDS);
  const myCountdownRef = useRef<number>(MY_MASTERNODES_REFRESH_SECONDS);

  // Auto-refresh countdown state for Network Masternodes
  const [networkCountdown, setNetworkCountdown] = useState<number>(NETWORK_REFRESH_SECONDS);
  const networkCountdownRef = useRef<number>(NETWORK_REFRESH_SECONDS);

  // Confirmation dialog state
  const [confirmAction, setConfirmAction] = useState<ConfirmAction>(null);

  // Edit dialog state — renamed from `configDialogOpen` by
  // m-masternodes-actions-restructure (2026-06-11) alongside the migration
  // from MasternodeConfigDialog (list + edit) to MasternodeEditDialog
  // (edit only).
  const [editDialogOpen, setEditDialogOpen] = useState(false);

  // Row data to pre-populate MasternodeEditDialog when opened via the per-row
  // Pencil IconButton. Includes alias + ip (from row.address) + txHash +
  // outputIndex so the dialog opens immediately in edit mode without waiting
  // for masternode.conf to fetch. Cleared on dialog close.
  // Added by m-masternodes-table-reorder-and-actions (2026-06-11).
  const [editingEntry, setEditingEntry] = useState<{
    alias: string;
    ip: string;
    txHash: string;
    outputIndex: number;
  } | null>(null);

  // Setup wizard state
  const [wizardOpen, setWizardOpen] = useState(false);

  // Delete confirmation state. Set by per-row Trash IconButton (via
  // handleDeleteMasternode); consumed by the second SimpleConfirmDialog at the
  // bottom of the page. pendingDeleteRef carries the target masternode across
  // the confirm/cancel callbacks so the deletion identifies the right alias.
  // Added by m-masternodes-actions-restructure (2026-06-11).
  const [deleteConfirm, setDeleteConfirm] = useState<boolean>(false);
  const pendingDeleteRef = useRef<Masternode | null>(null);

  // Whether the masternode debug subsystem is enabled in twinsd.yml.
  // Controls visibility of the Debug tab. Synced on mount and updated live via
  // the masternode:debug-changed Wails event emitted from app.go's ConfigManager
  // subscriber, so toggling masternode.debug from the Options dialog re-renders
  // the tab without a daemon restart.
  const [debugEnabled, setDebugEnabled] = useState<boolean>(false);

  // Wallet unlock hook (matches legacy masternodelist.cpp:265-280)
  const { showUnlockDialog, executeWithUnlock, unlockDialogProps } = useWalletAction({
    restoreAfter: true,
    onCancel: () => addNotification({ type: 'error', title: t('messages.unlockCancelled'), duration: 5000 }),
  });

  // Refs to track state for stable callbacks (avoids stale closures)
  const isLoadingRef = useRef(false);

  // Identifies which masternode the per-row Play IconButton requested to start.
  // Set in handleStartMasternode immediately before opening the start_alias
  // SimpleConfirmDialog; consumed by runMasternodeAction case 'start_alias'.
  // Cleared in the finally block after the action runs (success OR failure)
  // and in the SimpleConfirmDialog onCancel handler. Replaces the prior
  // selectedMasternodeRef pattern that depended on the now-dropped row-selection
  // state (m-masternodes-table-ux-cleanup, 2026-06-11).
  const pendingStartAliasRef = useRef<Masternode | null>(null);

  // Fetch my masternodes from backend
  const fetchMasternodes = useCallback(async () => {
    if (isLoadingRef.current) return; // Prevent concurrent fetches

    isLoadingRef.current = true;
    setLoading(true);
    try {
      const result = await GetMyMasternodes();
      if (result) {
        const mapped = result.map(mapToMasternode);
        setMasternodes(mapped);
      }
      setLastRefresh(Date.now());
      // Reset countdown after refresh
      myCountdownRef.current = MY_MASTERNODES_REFRESH_SECONDS;
      setMyCountdown(MY_MASTERNODES_REFRESH_SECONDS);
    } catch (error) {
      console.error('Failed to fetch masternodes:', error);
      addNotification({ type: 'error', title: t('messages.fetchFailed'), duration: 5000 });
    } finally {
      isLoadingRef.current = false;
      setLoading(false);
    }
  }, [setLoading, setMasternodes, setLastRefresh, addNotification, t]);

  // Stable ref for fetchMasternodes to avoid effect re-runs
  const fetchMasternodesRef = useRef(fetchMasternodes);
  fetchMasternodesRef.current = fetchMasternodes;

  // Ref to track network loading state
  const isLoadingNetworkRef = useRef(false);

  // Statistics state
  const [statistics, setStatistics] = useState<MasternodeStatistics | null>(null);
  const [isLoadingStatistics, setIsLoadingStatistics] = useState(false);

  // Fetch network masternodes and statistics from backend
  const fetchNetworkMasternodes = useCallback(async () => {
    if (isLoadingNetworkRef.current) return; // Prevent concurrent fetches

    isLoadingNetworkRef.current = true;
    setLoadingNetwork(true);
    setIsLoadingStatistics(true);
    try {
      // Fetch masternodes and statistics in parallel
      const [networkResult, statsResult] = await Promise.all([
        GetNetworkMasternodes(),
        GetMasternodeStatistics(),
      ]);

      if (networkResult && Array.isArray(networkResult)) {
        // Map and filter with proper type validation
        const mapped = networkResult
          .map(mapToNetworkMasternode)
          .filter((mn): mn is NetworkMasternode => mn !== null);
        setNetworkMasternodes(mapped);
      }

      if (statsResult) {
        setStatistics(statsResult as MasternodeStatistics);
      }

      setNetworkLastRefresh(Date.now());
      // Reset countdown after refresh
      networkCountdownRef.current = NETWORK_REFRESH_SECONDS;
      setNetworkCountdown(NETWORK_REFRESH_SECONDS);
    } catch (error) {
      console.error('Failed to fetch network masternodes:', error);
    } finally {
      isLoadingNetworkRef.current = false;
      setLoadingNetwork(false);
      setIsLoadingStatistics(false);
    }
  }, [setLoadingNetwork, setNetworkMasternodes, setNetworkLastRefresh]);

  // Stable ref for fetchNetworkMasternodes to avoid effect re-runs
  const fetchNetworkMasternodesRef = useRef(fetchNetworkMasternodes);
  fetchNetworkMasternodesRef.current = fetchNetworkMasternodes;

  // Auto-refresh timer for My Masternodes (only runs when tab is active)
  useEffect(() => {
    if (masternodeActiveTab !== 'my') return;

    // Fresh fetch and countdown reset on every tab entry
    myCountdownRef.current = MY_MASTERNODES_REFRESH_SECONDS;
    setMyCountdown(MY_MASTERNODES_REFRESH_SECONDS);
    fetchMasternodesRef.current();

    // Countdown timer - runs every second
    const countdownInterval = setInterval(() => {
      myCountdownRef.current -= 1;
      setMyCountdown(myCountdownRef.current);

      if (myCountdownRef.current <= 0) {
        // Use ref to get latest function without causing effect re-run
        fetchMasternodesRef.current();
      }
    }, 1000);

    return () => {
      clearInterval(countdownInterval);
    };
  }, [masternodeActiveTab]); // Re-run when tab changes: pause on exit, fetch + reset on entry

  // Auto-refresh timer for Network Masternodes (only runs when tab is active)
  useEffect(() => {
    if (masternodeActiveTab !== 'network') return;

    // Fresh fetch and countdown reset on every tab entry
    networkCountdownRef.current = NETWORK_REFRESH_SECONDS;
    setNetworkCountdown(NETWORK_REFRESH_SECONDS);
    fetchNetworkMasternodesRef.current();

    // Countdown timer - runs every second
    const countdownInterval = setInterval(() => {
      networkCountdownRef.current -= 1;
      setNetworkCountdown(networkCountdownRef.current);

      if (networkCountdownRef.current <= 0) {
        fetchNetworkMasternodesRef.current();
      }
    }, 1000);

    return () => {
      clearInterval(countdownInterval);
    };
  }, [masternodeActiveTab]); // Re-run when tab changes: pause on exit, fetch + reset on entry

  // Event listener for backend updates (separate effect)
  useEffect(() => {
    EventsOn('masternode:updated', () => {
      fetchMasternodesRef.current();
    });

    return () => {
      EventsOff('masternode:updated');
    };
  }, []); // Empty deps - event subscription runs once

  // Sync the Debug tab gate from the EFFECTIVE collector state on mount and on
  // live changes. IsDebugCollectorActive returns whether the collector is
  // actually running (Node.DebugCollector pointer non-nil), not the raw config
  // value — collector startup can fail (e.g. read-only data directory) and we
  // must not advertise a Debug tab whose backend never started. The Wails
  // event masternode:debug-changed reports the same effective state on live
  // toggles, so the tab appears or disappears without requiring a daemon
  // restart.
  useEffect(() => {
    let mounted = true;
    IsDebugCollectorActive()
      .then((v: boolean) => {
        if (mounted) setDebugEnabled(v);
      })
      .catch((err) => {
        console.error('Failed to read masternode debug collector state:', err);
      });
    EventsOn('masternode:debug-changed', (enabled: boolean) => {
      if (mounted) setDebugEnabled(enabled);
    });
    return () => {
      mounted = false;
      // EventsOff removes ALL handlers for this event globally. Today this
      // page is the only subscriber, matching the existing pattern used a few
      // lines above for masternode:updated. If a future component listens
      // for masternode:debug-changed, both will need to switch to per-handler
      // cleanup using the function returned by EventsOn.
      EventsOff('masternode:debug-changed');
    };
  }, []);

  // If the Debug tab becomes hidden while the user is viewing it, fall back
  // to the My Masternodes tab so the page never lands on a non-rendered tab.
  useEffect(() => {
    if (!debugEnabled && masternodeActiveTab === 'debug') {
      setMasternodeActiveTab('my');
    }
  }, [debugEnabled, masternodeActiveTab, setMasternodeActiveTab]);

  // Execute a masternode action with wallet unlock if needed.
  // Narrowed to start_alias only by m-masternodes-actions-restructure
  // (2026-06-11) — bulk start_all / start_missing cases were dropped alongside
  // the bottom-of-page action bar. Per-row Play IconButton fires only
  // start_alias.
  const runMasternodeAction = useCallback(async (action: PendingAction) => {
    if (!action) return;

    setStartingMasternode(true);

    try {
      if (action === 'start_alias') {
        if (!pendingStartAliasRef.current) {
          // Defensive: if the ref was cleared before this ran, abort.
          return;
        }
        await StartMasternode(pendingStartAliasRef.current.alias);
        addNotification({
          type: 'success',
          title: t('messages.startSuccess', { alias: pendingStartAliasRef.current.alias }),
          duration: 5000,
        });
      }
      await fetchMasternodes();
    } catch (error) {
      const errorMsg = error instanceof Error ? error.message : String(error);
      addNotification({ type: 'error', title: sanitizeErrorMessage(errorMsg), duration: 5000 });
    } finally {
      // Always clear the pending-start ref after start_alias (success OR error)
      // so a subsequent click can't see stale state from a prior masternode.
      if (action === 'start_alias') {
        pendingStartAliasRef.current = null;
      }
      setStartingMasternode(false);
    }
  }, [t, fetchMasternodes, setStartingMasternode, addNotification]);

  // Check wallet and execute with unlock if needed
  const checkWalletAndExecute = useCallback(async (action: PendingAction) => {
    await executeWithUnlock(async () => {
      await runMasternodeAction(action);
    });
  }, [executeWithUnlock, runMasternodeAction]);

  // Handle column header click for sorting (My Masternodes).
  //
  // Previous implementation called setSortDirection inside the setSortColumn
  // updater, which triggered double-execution under React Strict Mode (which
  // invokes state updater functions twice to detect impurities). The inner
  // setSortDirection's own updater also runs twice for each setSortColumn pass,
  // flipping the direction an even number of times — visually stuck on 'asc'.
  // Fixed by m-masternodes-table-ux-refinements (2026-06-11) via the
  // sortColumnRef pattern that mirrors networkFiltersRef below for
  // handleNetworkSort — read sortColumn synchronously without putting it in
  // the useCallback deps array.
  const sortColumnRef = useRef(sortColumn);
  sortColumnRef.current = sortColumn;
  const handleSort = useCallback((column: SortColumn) => {
    if (sortColumnRef.current === column) {
      // toggle direction on same column
      setSortDirection(prev => prev === 'asc' ? 'desc' : 'asc');
    } else {
      // new column → set + reset to asc
      setSortColumn(column);
      setSortDirection('asc');
    }
  }, []);

  // Handle column header click for sorting (Network Masternodes)
  const networkFiltersRef = useRef(networkFilters);
  networkFiltersRef.current = networkFilters;
  const handleNetworkSort = useCallback((column: NetworkSortColumn) => {
    const filters = networkFiltersRef.current;
    if (filters.sortColumn === column) {
      setNetworkFilters({ sortDirection: filters.sortDirection === 'asc' ? 'desc' : 'asc' });
    } else {
      setNetworkFilters({ sortColumn: column, sortDirection: 'asc' });
    }
  }, [setNetworkFilters]);

  // Handle per-row Play IconButton click — sets the pending-start ref then
  // opens the start_alias confirmation dialog. Replaces the prior selection-
  // gated actions-row "Start alias" button + right-click "Start alias" context
  // menu (both dropped by m-masternodes-table-ux-cleanup 2026-06-11).
  const handleStartMasternode = useCallback((masternode: Masternode) => {
    pendingStartAliasRef.current = masternode;
    setConfirmAction('start_alias');
  }, []);

  // Per-row Trash IconButton — opens the delete confirmation dialog with the
  // target masternode captured in pendingDeleteRef.
  // Added by m-masternodes-actions-restructure (2026-06-11).
  const handleDeleteMasternode = useCallback((masternode: Masternode) => {
    pendingDeleteRef.current = masternode;
    setDeleteConfirm(true);
  }, []);

  // Delete confirm flow: read pendingDeleteRef, call backend, refresh, notify,
  // clear ref + close confirm in finally.
  const handleConfirmDelete = useCallback(async () => {
    const mn = pendingDeleteRef.current;
    if (!mn) {
      setDeleteConfirm(false);
      return;
    }
    try {
      await DeleteMasternodeConfig(mn.alias);
      addNotification({
        type: 'success',
        title: t('messages.deleteSuccess', { alias: mn.alias }),
        duration: 5000,
      });
      await fetchMasternodes();
    } catch (error) {
      const errorMsg = error instanceof Error ? error.message : String(error);
      addNotification({ type: 'error', title: sanitizeErrorMessage(errorMsg), duration: 5000 });
    } finally {
      pendingDeleteRef.current = null;
      setDeleteConfirm(false);
    }
  }, [t, addNotification, fetchMasternodes]);

  const handleCancelDelete = useCallback(() => {
    pendingDeleteRef.current = null;
    setDeleteConfirm(false);
  }, []);

  // Confirm dialog handler - triggers wallet check before action
  const handleConfirmAction = () => {
    setConfirmAction(null);
    checkWalletAndExecute(confirmAction);
  };

  const getConfirmMessage = (): string => {
    if (confirmAction === 'start_alias') {
      return t('dialogs.startConfirm.message', { alias: pendingStartAliasRef.current?.alias });
    }
    return '';
  };

  // Memoize filtered network masternodes to prevent re-computation on countdown ticks.
  // Deps include both the data inputs and the getter functions themselves for correctness.
  const filteredNetworkMasternodes = useMemo(
    () => getFilteredNetworkMasternodes(),
    [getFilteredNetworkMasternodes, networkMasternodes, networkFilters]
  );
  const networkCount = useMemo(
    () => getNetworkMasternodeCount(),
    [getNetworkMasternodeCount, networkMasternodes, networkFilters]
  );

  // Check if network data has been loaded at least once (for loading indicator)
  const { networkLastRefresh } = useMasternodes();
  const networkHasLoaded = networkLastRefresh !== null;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', overflow: 'hidden' }}>
      <div style={{ flex: 1, overflow: 'auto', padding: '12px 16px', display: 'flex', flexDirection: 'column', gap: '12px', minHeight: 0 }}>
        {/* Tab Buttons */}
        <div style={{
          display: 'flex',
          gap: '0',
          borderBottom: '1px solid #4a4a4a',
        }}>
          <button
            onClick={() => setMasternodeActiveTab('my')}
            style={{
              padding: '8px 16px',
              fontSize: '12px',
              fontWeight: masternodeActiveTab === 'my' ? 'bold' : 'normal',
              backgroundColor: masternodeActiveTab === 'my' ? '#3a3a3a' : 'transparent',
              color: masternodeActiveTab === 'my' ? '#fff' : '#999',
              border: 'none',
              borderBottom: masternodeActiveTab === 'my' ? '2px solid #27ae60' : '2px solid transparent',
              cursor: 'pointer',
              transition: 'all 0.15s',
            }}
          >
            {t('tabs.myMasternodes')}
          </button>
          <button
            onClick={() => setMasternodeActiveTab('network')}
            style={{
              padding: '8px 16px',
              fontSize: '12px',
              fontWeight: masternodeActiveTab === 'network' ? 'bold' : 'normal',
              backgroundColor: masternodeActiveTab === 'network' ? '#3a3a3a' : 'transparent',
              color: masternodeActiveTab === 'network' ? '#fff' : '#999',
              border: 'none',
              borderBottom: masternodeActiveTab === 'network' ? '2px solid #27ae60' : '2px solid transparent',
              cursor: 'pointer',
              transition: 'all 0.15s',
            }}
          >
            {t('tabs.network')}
          </button>
          <button
            onClick={() => setMasternodeActiveTab('payments')}
            style={{
              padding: '8px 16px',
              fontSize: '12px',
              fontWeight: masternodeActiveTab === 'payments' ? 'bold' : 'normal',
              backgroundColor: masternodeActiveTab === 'payments' ? '#3a3a3a' : 'transparent',
              color: masternodeActiveTab === 'payments' ? '#fff' : '#999',
              border: 'none',
              borderBottom: masternodeActiveTab === 'payments' ? '2px solid #27ae60' : '2px solid transparent',
              cursor: 'pointer',
              transition: 'all 0.15s',
            }}
          >
            {t('tabs.paymentStats')}
          </button>
          {debugEnabled && (
            <button
              onClick={() => setMasternodeActiveTab('debug')}
              style={{
                padding: '8px 16px',
                fontSize: '12px',
                fontWeight: masternodeActiveTab === 'debug' ? 'bold' : 'normal',
                backgroundColor: masternodeActiveTab === 'debug' ? '#3a3a3a' : 'transparent',
                color: masternodeActiveTab === 'debug' ? '#fff' : '#999',
                border: 'none',
                borderBottom: masternodeActiveTab === 'debug' ? '2px solid #27ae60' : '2px solid transparent',
                cursor: 'pointer',
                transition: 'all 0.15s',
              }}
            >
              Debug
            </button>
          )}
          {/* Setup New Masternode — page-level primary action. Relocated from
              the bottom action bar (which was removed entirely) by
              m-masternodes-actions-restructure (2026-06-11). Visible across
              all tabs because creating a masternode is a page-level action,
              not tab-specific.
              Layout: marginLeft:auto pushes it to the right edge of the tab
              bar row; alignSelf:flex-end + marginBottom:2px aligns its bottom
              edge with the active tab's content baseline (the 2px green
              border-bottom accent on the active tab extends below the content
              into the parent's #4a4a4a divider; 2px marginBottom matches that
              extension so Setup's lower edge reads as flush with the active
              tab's text baseline rather than sitting taller than the tabs).
              Code-review fix from this same task — initial alignSelf:center
              was flagged as 1-2px misaligned due to Setup's 1px full border
              vs the tabs' bottom-border-only chrome (Setup is ~2px taller
              than tabs). Primary green tokens match the Send button precedent
              established in m-restyle-send-shell (2026-04-29). */}
          <button
            onClick={() => setWizardOpen(true)}
            style={{
              marginLeft: 'auto',
              alignSelf: 'flex-end',
              marginBottom: '2px',
              backgroundColor: '#4a7c59',
              border: '1px solid #5a8c69',
              borderRadius: '6px',
              padding: '8px 16px',
              fontSize: '12px',
              fontWeight: 500,
              color: '#fff',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: '6px',
            }}
          >
            <Plus size={12} />
            {t('wizard.title')}
          </button>
        </div>

        {/* My Masternodes Tab Content */}
        {masternodeActiveTab === 'my' && (
          <>
            {/* Informational note */}
            <Banner
              variant="info"
              message={`${t('note.title')} ${t('note.line1')} ${t('note.line2')} ${t('note.line3')}`}
            />

            {/* Masternodes Table.
                RefreshCountdown ring relocated INTO the sticky table header's
                Actions column by m-masternodes-table-reorder-and-actions
                (2026-06-11). The standalone right-aligned ring row that
                previously rendered here is gone; per-row edit affordance lives
                in the Actions column. */}
            <MasternodesTable
              masternodes={masternodes}
              isLoading={isLoading}
              sortColumn={sortColumn}
              sortDirection={sortDirection}
              onSort={handleSort}
              countdown={myCountdown}
              countdownTotal={MY_MASTERNODES_REFRESH_SECONDS}
              onRefresh={fetchMasternodes}
              isRefreshing={isLoading}
              onStartMasternode={handleStartMasternode}
              onEditMasternode={(mn) => {
                // CONTRACT: setEditingEntry and setEditDialogOpen(true) must be
                // called as a pair. The dialog's Phase 1 auto-edit useEffect uses
                // an autoEditAppliedRef one-shot guard that only resets on
                // isOpen=false. A future refactor that emits a new editingEntry
                // WITHOUT first closing the dialog would silently skip applying
                // the new entry. If such a "switch entry without close" UX is
                // ever introduced, either drop the autoEditAppliedRef guard or
                // key it on initialEntry.alias.
                setEditingEntry({
                  alias: mn.alias,
                  ip: mn.address,
                  txHash: mn.txHash,
                  outputIndex: mn.outputIndex,
                });
                setEditDialogOpen(true);
              }}
              onDeleteMasternode={handleDeleteMasternode}
            />

            {/* Bottom action bar removed by m-masternodes-actions-restructure
                (2026-06-11). The 5 bottom buttons (Start All, Start MISSING,
                Update status, Configure, Setup New Masternode) collapsed into:
                  - per-row Play IconButton (start_alias) on each table row
                  - per-row Pencil IconButton (edit) on each table row
                  - per-row Trash IconButton (delete) on each table row
                  - in-header RefreshCountdown ring (manual refresh)
                  - Setup New Masternode in the tab bar (page-level action)
                Bulk Start All / Start MISSING were dropped entirely. */}
          </>
        )}

        {/* Network Masternodes Tab Content */}
        {masternodeActiveTab === 'network' && (
          <>
            {/* Statistics Panel */}
            <MasternodeStatisticsPanel
              statistics={statistics}
              isLoading={isLoadingStatistics}
            />

            {/* Filters and Count — RefreshCountdown relocated into the
                table sticky header by m-network-masternodes-table-style-parity
                (2026-06-12), so Filters now only owns filter controls + count. */}
            <NetworkMasternodesFilters
              filters={networkFilters}
              filteredCount={networkCount.filtered}
              totalCount={networkCount.total}
              onFilterChange={setNetworkFilters}
            />

            {/* Network Masternodes Table */}
            <NetworkMasternodesTable
              masternodes={filteredNetworkMasternodes}
              isLoading={isLoadingNetwork}
              hasLoaded={networkHasLoaded}
              filters={networkFilters}
              onSort={handleNetworkSort}
              countdown={networkCountdown}
              countdownTotal={NETWORK_REFRESH_SECONDS}
              onRefresh={fetchNetworkMasternodes}
              isRefreshing={isLoadingNetwork}
            />
          </>
        )}

        {/* Payment Stats Tab Content */}
        {masternodeActiveTab === 'payments' && (
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minHeight: 0, padding: '0' }}>
            <PaymentStatsTab />
          </div>
        )}

        {/* Debug Tab Content */}
        {debugEnabled && masternodeActiveTab === 'debug' && (
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minHeight: 0 }}>
            <MasternodeDebugPanel />
          </div>
        )}
      </div>

      {/* Confirmation Dialog */}
      <SimpleConfirmDialog
        isOpen={confirmAction !== null}
        title={t('dialogs.startConfirm.title')}
        message={getConfirmMessage()}
        confirmText={t('common:buttons.yes')}
        cancelText={t('common:buttons.no')}
        onConfirm={handleConfirmAction}
        onCancel={() => {
          setConfirmAction(null);
          // Clear the pending-start ref on cancel so a subsequent Play click
          // on a different row can't fire against stale state. The success
          // path clears it inside runMasternodeAction's finally block.
          pendingStartAliasRef.current = null;
        }}
        isLoading={isStartingMasternode}
      />

      {/* Delete Confirmation Dialog.
          Added by m-masternodes-actions-restructure (2026-06-11) alongside the
          per-row Trash IconButton in MasternodesTable. pendingDeleteRef carries
          the target masternode across confirm/cancel. */}
      <SimpleConfirmDialog
        isOpen={deleteConfirm}
        title={t('dialogs.deleteConfirm.title')}
        message={
          pendingDeleteRef.current
            ? t('dialogs.deleteConfirm.message', { alias: pendingDeleteRef.current.alias })
            : ''
        }
        confirmText={t('common:buttons.delete', { defaultValue: 'Delete' })}
        cancelText={t('common:buttons.cancel')}
        isDestructive
        onConfirm={handleConfirmDelete}
        onCancel={handleCancelDelete}
      />

      {/* Edit Dialog.
          Renamed from MasternodeConfigDialog (which had list + add + edit + delete
          + reload modes) to MasternodeEditDialog (edit only) by
          m-masternodes-actions-restructure (2026-06-11). initialEntry is now
          REQUIRED — the dialog only opens when a row's Pencil IconButton sets it.
          Add is covered by MasternodeSetupWizard; Delete is the per-row Trash;
          Reload was dropped. */}
      <MasternodeEditDialog
        isOpen={editDialogOpen}
        initialEntry={editingEntry}
        onClose={() => {
          setEditDialogOpen(false);
          setEditingEntry(null);
          // Refresh masternodes list when dialog closes in case config changed
          fetchMasternodes();
        }}
      />

      {/* Setup Wizard */}
      <MasternodeSetupWizard
        isOpen={wizardOpen}
        onClose={() => setWizardOpen(false)}
        onSuccess={() => {
          fetchMasternodes();
        }}
      />

      {/* Wallet Unlock Dialog (matches legacy masternodelist.cpp:265-280) */}
      <UnlockWalletDialog
        isOpen={showUnlockDialog}
        {...unlockDialogProps}
        temporaryUnlock
      />
    </div>
  );
};

export { MasternodesPage as Masternodes };
