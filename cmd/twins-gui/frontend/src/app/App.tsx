import { useState, useEffect } from 'react';
import { RouterProvider } from 'react-router';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { StoreProvider } from '@/app/providers/StoreProvider';
import { router } from '@/app/router';
import { IntroDialog } from '@/shared/components/IntroDialog';
import { SplashScreenWithEvents } from '@/shared/components/SplashScreenWithEvents';
import ShutdownDialog from '@/shared/components/ShutdownDialog';
import { OptionsDialog } from '@/features/settings/components/OptionsDialog';
import { ToolsDialog } from '@/features/tools/components/ToolsDialog';
import { ToolsTab, type ToolsTabValue } from '@/features/tools/constants';
import { useOptions, useTools, useNotifications, useSignVerify, useAddressBook, useTransactions, useStore } from '@/store/useStore';
import { EventsOn, EventsOff, WindowShow } from '@wailsjs/runtime/runtime';
import {
  SetWindowToSplash,
  SetWindowToMain,
  InitializeDataDirectory,
  LoadConfiguration,
  StartInitialization,
  CheckFirstRun,
  InitiateShutdown,
  IsShuttingDown,
  GetPendingRepairResult,
  BackupWallet,
  HandleWindowMinimized
} from '@wailsjs/go/main/App';
import { SignVerifyMessageDialog } from '@/features/wallet/components/SignVerifyMessageDialog';
import { AddressBookDialog } from '@/features/wallet/components/AddressBookDialog';
import { logger } from '@/shared/utils/logger';
import '@/styles/index.css';
import '@/styles/qt-theme.css';

// Component that handles menu events and opens Options dialog
// Must be inside StoreProvider to access useOptions hook
function MenuEventHandler() {
  const { openOptionsDialog } = useOptions();

  useEffect(() => {
    const unlisten = EventsOn('menu:open-preferences', () => {
      openOptionsDialog();
    });

    return () => {
      unlisten();
    };
  }, [openOptionsDialog]);

  // Also render the OptionsDialog here so it's available app-wide
  return <OptionsDialog />;
}

// Component that handles autocombine consolidation complete events
function AutoCombineEventHandler() {
  const { addNotification } = useNotifications();

  useEffect(() => {
    const unlisten = EventsOn('autocombine:complete', (data: { txCount: number; amount: number }) => {
      addNotification({
        type: 'success',
        title: 'UTXO Consolidation',
        message: `Consolidated ${data.txCount} transaction${data.txCount > 1 ? 's' : ''} (${data.amount.toFixed(2)} TWINS)`,
        duration: 8000,
      });
    });
    return () => { unlisten(); };
  }, [addNotification]);

  return null;
}

// Component that handles Backup Wallet menu event
// Must be inside StoreProvider to access useNotifications hook
function BackupWalletEventHandler() {
  const { addNotification } = useNotifications();

  useEffect(() => {
    const unlisten = EventsOn('menu:backup-wallet', () => {
      BackupWallet().then((saved) => {
        if (saved) {
          addNotification({
            type: 'success',
            title: 'Backup Wallet',
            message: 'Wallet backup saved successfully.',
            duration: 5000,
          });
        }
      }).catch((err) => {
        addNotification({
          type: 'error',
          title: 'Backup Wallet',
          message: `Wallet backup failed: ${err}`,
          duration: 10000,
        });
      });
    });

    return () => {
      unlisten();
    };
  }, [addNotification]);

  return null;
}

// Component that handles Tools Window menu events
// Must be inside StoreProvider to access useTools hook
function ToolsWindowEventHandler() {
  const { openToolsDialog } = useTools();

  useEffect(() => {
    const unlisten = EventsOn('menu:open-tools-window', (tabIndex: ToolsTabValue) => {
      openToolsDialog(tabIndex);
    });

    return () => {
      unlisten();
    };
  }, [openToolsDialog]);

  return <ToolsDialog />;
}

// Component that handles Sign/Verify Message menu event
// Must be inside StoreProvider to access useSignVerify hook
function SignVerifyEventHandler() {
  const { openSignVerifyDialog } = useSignVerify();

  useEffect(() => {
    const unlisten = EventsOn('menu:open-sign-verify', () => {
      openSignVerifyDialog();
    });

    return () => {
      unlisten();
    };
  }, [openSignVerifyDialog]);

  return <SignVerifyMessageDialog />;
}

// Component that handles Address Book menu event
// Must be inside StoreProvider to access useAddressBook hook
function AddressBookEventHandler() {
  const { openAddressBookDialog } = useAddressBook();

  useEffect(() => {
    const unlisten = EventsOn('menu:open-address-book', () => {
      openAddressBookDialog('edit');
    });

    return () => {
      unlisten();
    };
  }, [openAddressBookDialog]);

  return <AddressBookDialog />;
}

// Eager-prefetch the first page of transactions on app boot so the Transactions
// page renders rows immediately on first visit (no skeleton flash). Subsequent
// visits also benefit — the slice keeps the cached page in memory across route
// changes, and the page's own on-mount fetchPage() refresh runs in the
// background without unmounting the visible rows (per the antiflicker render
// branch in `pages/Transactions.tsx`).
//
// Trade-off: one extra RPC at boot even if the user never opens Transactions.
// The fetch is small (one page = up to 250 rows) and runs in parallel with
// other boot-time work, so the cost is negligible vs. the first-open flash
// the user reported.
function TransactionsPreloadHandler() {
  const { fetchPage, syncHideOrphanStakes, syncBlockExplorerUrls } = useTransactions();
  useEffect(() => {
    // Mirror the Transactions page's own mount effect: sync the orphan-stakes
    // filter and the block-explorer URL list from backend settings BEFORE
    // fetching, so the preloaded page matches what the page would fetch on
    // its own. Without this, a user with `fHideOrphans=true` would see
    // orphan rows briefly when navigating to Transactions for the first
    // time (preloaded page used the default `false`, page mount re-syncs
    // and refetches with `true` → rows swap).
    //
    // Both syncs run in parallel — `syncBlockExplorerUrls` doesn't affect
    // the fetch payload (URLs are consumed by the context-menu, not the
    // fetch path), but its early resolution closes the race where a user
    // could right-click a transaction with `blockExplorerUrls === []`
    // before the sync lands and get routed to the legacy fallback even
    // when they have custom URLs configured. See Transactions.tsx
    // `urlsResolved` guard for the symmetric defense.
    //
    // Race defense (Codex round-4 finding): `fetchPage(1)` in the slice has
    // no request-versioning, so a late preload response could overwrite a
    // newer user-initiated fetch (e.g. user opens Transactions and changes
    // filter before preload lands). After the syncs resolve, read the LIVE
    // store state via `useStore.getState()` — if `transactions.length > 0`,
    // someone else (page mount or user) already populated the cache during
    // the ~100ms sync window, and we no-op. Closes the visible-flash case.
    Promise.all([syncBlockExplorerUrls(), syncHideOrphanStakes()])
      .then(() => {
        // Cache-warm guard: skip the preload fetch if EITHER (a) the store
        // already has rows (page mount or user fetch completed during sync
        // window), or (b) a fetch is currently in flight. Both conditions
        // matter — `syncHideOrphanStakes` internally dispatches `fetchPage(1)`
        // when the GUISetting value differs from the slice default (see
        // transactionsSlice.ts line 454), so by the time we reach this
        // `.then()` block there may already be a fire-and-forget fetch in
        // flight that hasn't yet populated `transactions`. Without the
        // `isLoadingTransactions` check we'd issue a duplicate fetch with
        // identical args; the two responses then race in the slice.
        const live = useStore.getState();
        if (live.transactions.length > 0 || live.isLoadingTransactions) return;
        fetchPage(1);
      })
      .catch((err) => logger.warn('Transactions preload failed:', err));
    // Intentionally empty deps: run exactly once on mount. Store-action
    // references are stable in practice but listing them would suggest
    // re-running on identity change, which is wrong for an eager preload.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return null;
}

// Component that checks for repair results after app restart.
// Uses pull-based approach (GetPendingRepairResult) on mount to avoid timing issues
// with event emission during splash-to-main transition.
// Also listens for repair:error events which fire during the same session (not across restarts).
function RepairResultHandler() {
  const { openToolsDialog, setLastRepairResult } = useTools();

  useEffect(() => {
    // Pull-based: check if a repair action completed during this startup
    GetPendingRepairResult().then((result) => {
      if (result && result.action) {
        setLastRepairResult({ action: result.action, success: result.success });
        openToolsDialog(ToolsTab.WalletRepair);
      }
    }).catch(() => {
      // Ignore errors (e.g., backend not ready)
    });

    // Event-based: listen for restart failure during current session
    const unlistenError = EventsOn('repair:error', (result: { action: string; success: boolean; error?: string }) => {
      setLastRepairResult({ action: result.action, success: false, error: result.error });
      openToolsDialog(ToolsTab.WalletRepair);
    });

    return () => {
      unlistenError();
    };
  }, [openToolsDialog, setLastRepairResult]);

  return null;
}

// Component that detects window minimize and notifies Go backend.
// When fMinimizeToTray is enabled, the backend will hide the window to the system tray.
// Uses the Page Visibility API which fires when the Wails window is minimized.
function MinimizeToTrayHandler() {
  useEffect(() => {
    const handleVisibilityChange = () => {
      if (document.hidden) {
        // Window became hidden (minimized) - let backend decide whether to hide to tray
        HandleWindowMinimized().catch(() => {
          // Ignore errors (e.g., backend not ready)
        });
      }
    };
    document.addEventListener('visibilitychange', handleVisibilityChange);
    return () => document.removeEventListener('visibilitychange', handleVisibilityChange);
  }, []);

  return null;
}

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 3,
      retryDelay: (attemptIndex) => Math.min(1000 * 2 ** attemptIndex, 30000),
      staleTime: 5 * 60 * 1000, // 5 minutes
      gcTime: 10 * 60 * 1000, // 10 minutes
    },
  },
});

// Application states
type AppState = 'loading' | 'intro' | 'splash' | 'main' | 'shutdown' | 'error';

interface AppFlowState {
  state: AppState;
  dataDirectory: string | null;
  isFirstRun: boolean;
  error: string | null;
  isShuttingDown: boolean;
}

function App() {
  const { t } = useTranslation('common');
  const [appState, setAppState] = useState<AppFlowState>({
    state: 'loading',
    dataDirectory: null,
    isFirstRun: true,
    error: null,
    isShuttingDown: false
  });

  // Check if this is the first run
  useEffect(() => {
    const initializeApp = async () => {
      try {
        // Backend has already set the window state (intro or splash)
        // Just check the result to determine which UI to show
        const result = await CheckFirstRun() as any;

        if (result.error) {
          throw new Error(result.error);
        }

        const isFirstRun = result.isFirstRun;
        const dataDir = result.dataDir;
        const showSplash = result.showSplash !== false; // Default to true if not specified

        if (isFirstRun) {
          // First run - backend prepared intro window, now update state and show
          setAppState({
            ...appState,
            state: 'intro',
            isFirstRun: true,
            dataDirectory: dataDir
          });
          // Show window now that React is ready
          WindowShow();
        } else if (showSplash) {
          // Not first run, show splash - backend prepared splash window
          await LoadConfiguration(dataDir);
          setAppState({
            ...appState,
            state: 'splash',
            isFirstRun: false,
            dataDirectory: dataDir
          });
          // Show window now that React is ready
          WindowShow();
          // Note: StartInitialization() is called by SplashScreenWithEvents when it mounts
        } else {
          // -nosplash flag set - skip splash, go directly to main
          // Backend already prepared main window size
          logger.info('-nosplash flag set - skipping splash screen');
          await LoadConfiguration(dataDir);
          setAppState({
            ...appState,
            state: 'main',
            isFirstRun: false,
            dataDirectory: dataDir
          });
          // Show window now that React is ready
          WindowShow();
          // Start initialization (no SplashScreenWithEvents, so call directly)
          await StartInitialization();
        }
      } catch (error) {
        logger.error('Failed to check first run:', error);
        setAppState({
          ...appState,
          state: 'error',
          error: error instanceof Error ? error.message : 'Unknown error'
        });
      }
    };

    initializeApp();

    // Listen for shutdown initiation
    EventsOn('app:shutdown', async () => {
      logger.info('Shutdown requested');
      await handleShutdown();
    });

    // Listen for shutdown complete
    EventsOn('shutdown:complete', () => {
      logger.info('Shutdown complete - app will close');
      // The backend will handle the actual app closure
    });

    return () => {
      EventsOff('app:shutdown');
      EventsOff('shutdown:complete');
    };
  }, []);

  // Handle intro dialog completion
  const handleIntroComplete = async (dataDirectory: string) => {
    try {
      logger.info('Intro complete, selected directory:', dataDirectory);

      // Initialize the data directory
      await InitializeDataDirectory(dataDirectory);

      // Load configuration
      await LoadConfiguration(dataDirectory);

      // Transition to splash screen
      await SetWindowToSplash();
      setAppState({
        ...appState,
        state: 'splash',
        dataDirectory: dataDirectory
      });
      // Note: StartInitialization() is called by SplashScreenWithEvents when it mounts
    } catch (error) {
      logger.error('Failed to complete intro:', error);
      setAppState({
        ...appState,
        state: 'error',
        error: error instanceof Error ? error.message : 'Failed to initialize'
      });
    }
  };

  // Handle splash screen completion
  const handleSplashComplete = async () => {
    try {
      logger.info('Splash complete, transitioning to main window');

      // Transition to main window
      await SetWindowToMain();
      setAppState({
        ...appState,
        state: 'main'
      });
    } catch (error) {
      logger.error('Failed to complete splash:', error);
      setAppState({
        ...appState,
        state: 'error',
        error: error instanceof Error ? error.message : 'Failed to start main application'
      });
    }
  };

  // Handle splash screen error
  const handleSplashError = (error: string) => {
    logger.error('Splash screen error:', error);
    setAppState({
      ...appState,
      state: 'error',
      error: error
    });
  };

  // Handle intro cancellation
  const handleIntroCancel = () => {
    // In a real app, you might want to quit here
    console.log('Intro cancelled - app cannot continue without data directory');
    setAppState({
      ...appState,
      state: 'error',
      error: 'Setup cancelled - TWINS Wallet requires a data directory to operate'
    });
  };

  // Handle shutdown
  const handleShutdown = async () => {
    try {
      // Check if already shutting down
      const isShuttingDown = await IsShuttingDown();
      if (isShuttingDown) {
        console.log('Already shutting down');
        return;
      }

      // Update state to show shutdown dialog
      setAppState(prev => ({
        ...prev,
        state: 'shutdown',
        isShuttingDown: true
      }));

      // Initiate the shutdown process
      await InitiateShutdown();
    } catch (error) {
      console.error('Failed to initiate shutdown:', error);
      // Still show shutdown state even if there's an error
      setAppState(prev => ({
        ...prev,
        state: 'shutdown',
        isShuttingDown: true
      }));
    }
  };

  // Render based on current state
  switch (appState.state) {
    case 'loading':
      return (
        <div className="flex items-center justify-center h-screen bg-gray-900">
          <div className="text-white">{t('loading.default')}</div>
        </div>
      );

    case 'intro':
      return (
        <IntroDialog
          onComplete={handleIntroComplete}
          onCancel={handleIntroCancel}
        />
      );

    case 'splash':
      return (
        <SplashScreenWithEvents
          onComplete={handleSplashComplete}
          onError={handleSplashError}
        />
      );

    case 'main':
      return (
        <QueryClientProvider client={queryClient}>
          <StoreProvider>
            <MenuEventHandler />
            <AutoCombineEventHandler />
            <BackupWalletEventHandler />
            <ToolsWindowEventHandler />
            <SignVerifyEventHandler />
            <AddressBookEventHandler />
            <RepairResultHandler />
            <MinimizeToTrayHandler />
            <TransactionsPreloadHandler />
            <RouterProvider router={router} />
          </StoreProvider>
        </QueryClientProvider>
      );

    case 'shutdown':
      return <ShutdownDialog />;

    case 'error':
      return (
        <div className="flex items-center justify-center h-screen bg-red-900">
          <div className="text-white text-center p-8">
            <h1 className="text-2xl font-bold mb-4">{t('status.error')}</h1>
            <p>{appState.error}</p>
            <button
              onClick={() => window.location.reload()}
              className="mt-4 px-4 py-2 bg-red-700 hover:bg-red-600 rounded"
            >
              {t('buttons.retry')}
            </button>
          </div>
        </div>
      );

    default:
      return null;
  }
}

export default App;