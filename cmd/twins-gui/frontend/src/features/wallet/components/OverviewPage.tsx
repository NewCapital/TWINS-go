import React, { useEffect, useCallback, useState, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useStore } from '@/store/useStore';
import { useShallow } from 'zustand/react/shallow';
import { useWalletActions } from '@/shared/hooks/useWalletActions';
import { BalancesStrip } from './BalancesStrip';
import { SyncCard } from './SyncCard';
import { SyncProgressRow } from './SyncProgressRow';
import { NetworkCard } from './NetworkCard';
import { StakingCard } from './StakingCard';
import { LoadingSpinner } from '@/shared/components/LoadingSpinner';
import { TransactionList } from './TransactionItem';
import { TransactionDetailsDialog } from './TransactionDetailsDialog';
import { DashboardCard } from '@/shared/components/DashboardCard';
import { useDisplayUnits } from '@/shared/hooks/useDisplayUnits';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { GetBalance, GetRecentTransactions, GetNetworkInfo, GetStakingInfo } from '@wailsjs/go/main/App';
import { core } from '@/shared/types/wallet.types';
import { logger } from '@/shared/utils/logger';

// Auto-refresh interval in milliseconds (10 seconds)
const STATUS_REFRESH_INTERVAL = 10000;

// Receive design-language tokens (mirrors Send.tsx shell pattern)
const pageOuter: React.CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  height: '100%',
  overflow: 'hidden',
};

const pageScroll: React.CSSProperties = {
  flex: 1,
  overflow: 'auto',
  display: 'flex',
  flexDirection: 'column',
  gap: '12px',
  padding: '12px 16px',
};

const OverviewPage: React.FC = () => {
  const { t } = useTranslation('wallet');
  const { unitLabel } = useDisplayUnits();
  const { balance, isLoading } = useStore(useShallow((s) => ({
    balance: s.balance,
    isLoading: s.isLoading,
  })));
  const { refreshBalance } = useWalletActions();
  const [recentTransactions, setRecentTransactions] = useState<core.Transaction[]>([]);
  const [txLoading, setTxLoading] = useState(false);
  const [selectedTransaction, setSelectedTransaction] = useState<core.Transaction | null>(null);

  // Blockchain info from shared store (populated by useP2PEvents in MainLayout)
  const blockchainInfo = useStore((state) => state.blockchainInfo);

  // Status info state
  const [networkInfo, setNetworkInfo] = useState<core.NetworkInfo | null>(null);
  const [stakingInfo, setStakingInfo] = useState<core.StakingInfo | null>(null);
  const [statusLoading, setStatusLoading] = useState(false);

  // Ref to track if component is mounted (for cleanup)
  const isMountedRef = useRef(true);

  // Maximum number of recent transactions to display
  const MAX_RECENT_TRANSACTIONS = 9;

  // Serialized transaction refresh: prevents concurrent fetches from producing
  // stale or accumulated results. Only one fetch runs at a time; if a new
  // request arrives while one is in-flight, it runs after the current completes.
  const txFetchInFlightRef = useRef(false);
  const txFetchPendingRef = useRef(false);

  const refreshTransactions = useCallback(async (showLoading = false) => {
    if (txFetchInFlightRef.current) {
      txFetchPendingRef.current = true;
      return;
    }
    txFetchInFlightRef.current = true;
    if (showLoading) setTxLoading(true);

    try {
      const txs = await GetRecentTransactions();
      if (isMountedRef.current) {
        setRecentTransactions(txs.map(tx => new core.Transaction(tx)).slice(0, MAX_RECENT_TRANSACTIONS));
      }
    } catch (error) {
      logger.error('OverviewPage: Failed to fetch recent transactions', error);
      if (isMountedRef.current) setRecentTransactions([]);
    } finally {
      if (showLoading) setTxLoading(false);
      txFetchInFlightRef.current = false;
      // If a request came in while we were fetching, run it now
      if (txFetchPendingRef.current && isMountedRef.current) {
        txFetchPendingRef.current = false;
        refreshTransactions(false);
      }
    }
  }, []);

  // Initial fetch with loading indicator
  const fetchRecentTransactions = useCallback(async () => {
    await refreshTransactions(true);
  }, [refreshTransactions]);

  // Fetch blockchain, network, and staking info
  const fetchStatusInfo = useCallback(async () => {
    try {
      setStatusLoading(true);
      logger.debug('OverviewPage: Fetching status info...');

      // Fetch network and staking info in parallel
      // Note: blockchainInfo is fetched by useP2PEvents (MainLayout) and read from store
      const [network, staking] = await Promise.all([
        GetNetworkInfo().catch(err => {
          logger.error('Failed to get network info:', err);
          return null;
        }),
        GetStakingInfo().catch(err => {
          logger.error('Failed to get staking info:', err);
          return null;
        }),
      ]);

      // Only update state if component is still mounted
      if (isMountedRef.current) {
        if (network) {
          setNetworkInfo(new core.NetworkInfo(network));
        }
        if (staking) {
          setStakingInfo(new core.StakingInfo(staking));
        }
        logger.debug('OverviewPage: Status info updated');
      }
    } catch (error) {
      logger.error('OverviewPage: Failed to fetch status info', error);
    } finally {
      if (isMountedRef.current) {
        setStatusLoading(false);
      }
    }
  }, []);

  // Silent refresh for auto-refresh interval: updates data without loading indicators
  // to prevent UI flicker every 10 seconds.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const silentRefresh = useCallback(() => {
    // Refresh network and staking info without loading indicator
    // Note: blockchainInfo is refreshed by useP2PEvents (MainLayout) on P2P events and every 10s
    Promise.all([
      GetNetworkInfo().catch(err => { logger.debug('Silent refresh: network info failed', err); return null; }),
      GetStakingInfo().catch(err => { logger.debug('Silent refresh: staking info failed', err); return null; }),
    ]).then(([network, staking]) => {
      if (!isMountedRef.current) return;
      if (network) setNetworkInfo(new core.NetworkInfo(network));
      if (staking) setStakingInfo(new core.StakingInfo(staking));
    });

    // Refresh balance without store loading indicator
    GetBalance().then(b => {
      if (b && isMountedRef.current) {
        useStore.getState().setBalance(new core.Balance(b));
      }
    }).catch(err => { logger.debug('Silent refresh: balance failed', err); });

    // Refresh transactions without txLoading indicator (serialized)
    refreshTransactions(false);
  }, []);

  // Load data on mount and set up auto-refresh
  useEffect(() => {
    isMountedRef.current = true;
    logger.debug('OverviewPage: Loading initial data...');

    // Initial data fetch (with loading indicators for first load)
    refreshBalance(true); // silent=true to suppress error notifications during startup
    fetchRecentTransactions();
    fetchStatusInfo();

    // Set up auto-refresh interval (10 seconds)
    // Uses silentRefresh to avoid loading indicator flicker.
    const statusInterval = setInterval(() => {
      if (isMountedRef.current) {
        silentRefresh();
      }
    }, STATUS_REFRESH_INTERVAL);

    // Cleanup on unmount
    return () => {
      isMountedRef.current = false;
      clearInterval(statusInterval);
    };
  }, [refreshBalance, fetchRecentTransactions, fetchStatusInfo, silentRefresh]);

  // Debounced silentRefresh to coalesce rapid P2P events (max once per second)
  const debounceTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const debouncedSilentRefresh = useCallback(() => {
    if (debounceTimerRef.current) return; // Already scheduled
    debounceTimerRef.current = setTimeout(() => {
      debounceTimerRef.current = null;
      silentRefresh();
    }, 1000);
  }, [silentRefresh]);

  // Subscribe to balance changes and P2P events from backend
  useEffect(() => {
    logger.debug('OverviewPage: Setting up event listeners...');

    // Listen for balance changes
    const unsubscribeBalance = EventsOn('balance:changed', (newBalance: any) => {
      logger.debug('OverviewPage: Balance changed event received', newBalance);
      const balanceInstance = new core.Balance(newBalance);
      useStore.getState().setBalance(balanceInstance);
    });

    // Listen for new transactions (silent refresh to avoid loading spinner flash)
    const unsubscribeTransaction = EventsOn('transaction:received', () => {
      logger.debug('OverviewPage: Transaction received event');
      refreshTransactions(false);
    });

    // Listen for P2P events to update status widgets in real-time (debounced)
    const unsubscribePeerCount = EventsOn('p2p:peer_count', () => {
      debouncedSilentRefresh();
    });
    const unsubscribeSyncing = EventsOn('p2p:syncing', () => {
      debouncedSilentRefresh();
    });
    const unsubscribeSynced = EventsOn('p2p:synced', () => {
      debouncedSilentRefresh();
    });
    const unsubscribeChainSync = EventsOn('chain:sync', () => {
      debouncedSilentRefresh();
    });

    // Listen for staking setting changes (from Options dialog or ToggleStaking)
    const unsubscribeStaking = EventsOn('staking:changed', () => {
      debouncedSilentRefresh();
    });

    // Listen for wallet lock/unlock events to update staking widget immediately
    const unsubscribeWalletLocked = EventsOn('wallet:locked', () => {
      debouncedSilentRefresh();
    });
    const unsubscribeWalletUnlocked = EventsOn('wallet:unlocked', () => {
      debouncedSilentRefresh();
    });

    return () => {
      unsubscribeBalance();
      unsubscribeTransaction();
      unsubscribePeerCount();
      unsubscribeSyncing();
      unsubscribeSynced();
      unsubscribeChainSync();
      unsubscribeStaking();
      unsubscribeWalletLocked();
      unsubscribeWalletUnlocked();
      if (debounceTimerRef.current) clearTimeout(debounceTimerRef.current);
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debouncedSilentRefresh, refreshTransactions]);

  return (
    <>
      <div style={pageOuter}>
        <div style={pageScroll}>
          {/*
            The page-level out-of-sync Banner was removed in 2026-05-06 because
            the SyncCard now surfaces the same state via its StatusPill +
            behind-time Banner. Three indicators of the same fact in one
            viewport (page banner + Sync pill + Sync behind-time banner) was
            redundant; the Sync card is the single source of truth for sync
            state on the Overview page. The status bar at the bottom of the
            window remains as the always-visible secondary signal.
          */}
          <SyncProgressRow blockchainInfo={blockchainInfo} />
          <div style={{
            display: 'grid',
            // Fixed 2x2 grid at every viewport width: Balance + Sync (row 1),
            // Network + Staking (row 2). `minmax(0, 1fr)` lets columns shrink
            // below intrinsic content width without forcing horizontal scroll.
            gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
            gridAutoRows: 'min-content',
            gap: '12px',
          }}>
            <BalancesStrip
              balance={balance}
              isLoading={isLoading}
            />
            <SyncCard blockchainInfo={blockchainInfo} networkInfo={networkInfo} isLoading={statusLoading} />
            <StakingCard stakingInfo={stakingInfo} isLoading={statusLoading} />
            <NetworkCard networkInfo={networkInfo} isLoading={statusLoading} />
          </div>

          {/*
            Recent Transactions card stretches to fill the remaining viewport
            space below the 2x2 grid (and the SyncProgressRow when active).
            `flex: 1, minHeight: 0` on the card chrome lets it grow inside the
            `pageScroll` flex column; the inner scroll container inherits the
            same flex sizing so the list / empty state / spinner all center
            inside the card. The empty state (rendered by TransactionList when
            transactions.length === 0) is centered vertically; long lists scroll
            inside the card without forcing the outer page to scroll.
          */}
          <DashboardCard
            title={t('overview.recentTransactions')}
            headerRight={unitLabel}
            style={{ flex: 1, minHeight: 0 }}
          >
            <div
              style={{
                position: 'relative',
                flex: 1,
                minHeight: 0,
                display: 'flex',
                flexDirection: 'column',
              }}
            >
              {txLoading ? (
                <div
                  style={{
                    flex: 1,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    padding: '24px 0',
                  }}
                >
                  <LoadingSpinner message={t('common:loading.transactions')} />
                </div>
              ) : (
                <div
                  style={{
                    flex: 1,
                    minHeight: 0,
                    overflowY: 'auto',
                    display: 'flex',
                    flexDirection: 'column',
                  }}
                >
                  <TransactionList
                    transactions={recentTransactions}
                    limit={9}
                    onTransactionClick={(tx) => setSelectedTransaction(tx)}
                  />
                </div>
              )}
            </div>
          </DashboardCard>
        </div>
      </div>

      <TransactionDetailsDialog
        isOpen={selectedTransaction !== null}
        transaction={selectedTransaction}
        onClose={() => setSelectedTransaction(null)}
      />
    </>
  );
};

export default OverviewPage;
