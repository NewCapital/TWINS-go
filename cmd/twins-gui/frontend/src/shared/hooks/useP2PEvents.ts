import { useEffect, useRef, useState, useCallback } from 'react';
import { EventsOn, EventsOff } from '@wailsjs/runtime/runtime';
import { useStore } from '@/store/useStore';
import { GetP2PStatus, StartP2P, GetBlockchainInfo } from '@wailsjs/go/main/App';
import { core } from '@wailsjs/go/models';

interface P2PErrorState {
  isOpen: boolean;
  error: string;
}

/**
 * Hook to subscribe to P2P network events and update connection/blockchain status in the store.
 * Should be used in a component that is mounted when the main app is visible (MainLayout).
 *
 * - Connectivity state (isConnected, isConnecting, peers) is updated from GetP2PStatus() and P2P events.
 * - Blockchain info (sync status, behind_time, sync_percentage) is fetched from GetBlockchainInfo()
 *   on startup, every 10s, and on each P2P sync event. This is the single authoritative source
 *   shared with SyncStatusWidget via the Zustand store, eliminating inconsistency.
 *
 * Returns error dialog state and handlers.
 */
export const useP2PEvents = () => {
  const setConnectionStatus = useStore((state) => state.setConnectionStatus);
  const setBlockchainInfo = useStore((state) => state.setBlockchainInfo);
  const [errorState, setErrorState] = useState<P2PErrorState>({ isOpen: false, error: '' });

  // Ref for debouncing rapid-fire P2P event blockchain info refreshes
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    // Fetch and store blockchain info (sync status, behind_time, sync_percentage, etc.)
    const fetchBlockchainInfo = async () => {
      try {
        const info = await GetBlockchainInfo();
        setBlockchainInfo(new core.BlockchainInfo(info));
      } catch (err) {
        console.error('Failed to fetch blockchain info:', err);
      }
    };

    // Trailing-edge debounce: resets timer on every call, fires 500ms after the last event.
    // Ensures the fetch always captures the most recent sync state after a burst of P2P events.
    const debouncedFetchBlockchainInfo = () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
      debounceRef.current = setTimeout(() => {
        debounceRef.current = null;
        fetchBlockchainInfo();
      }, 500);
    };

    // Fetch initial connectivity state from GetP2PStatus (connectivity only, not sync status)
    const fetchInitialStatus = async () => {
      try {
        const status = await GetP2PStatus();
        setConnectionStatus({
          isConnected: status.connected as boolean,
          peers: status.peers as number,
        });
      } catch (error) {
        console.error('Failed to fetch P2P status:', error);
      }
    };

    fetchInitialStatus();
    fetchBlockchainInfo(); // Initial blockchain info fetch (authoritative sync state)

    // Poll blockchain info every 10 seconds to keep status bar and sync widget in sync
    const pollInterval = setInterval(fetchBlockchainInfo, 10000);

    // Handle P2P connecting event
    const handleConnecting = () => {
      setConnectionStatus({ isConnected: false });
      debouncedFetchBlockchainInfo();
    };

    // Handle P2P connected event
    const handleConnected = (data: { peers: number }) => {
      setConnectionStatus({ isConnected: true, peers: data.peers });
      debouncedFetchBlockchainInfo();
    };

    // Handle peer count change
    const handlePeerCount = (data: { peers: number }) => {
      setConnectionStatus({ peers: data.peers, isConnected: data.peers > 0 });
    };

    // Handle syncing event — refresh blockchain info for accurate sync_percentage and behind_time
    const handleSyncing = () => {
      debouncedFetchBlockchainInfo();
    };

    // Handle chain sync progress — refresh blockchain info
    const handleChainSync = () => {
      debouncedFetchBlockchainInfo();
    };

    // Handle synced event — refresh blockchain info to confirm synced state
    const handleSynced = () => {
      debouncedFetchBlockchainInfo();
    };

    // Handle P2P error
    const handleError = (data: { error: string }) => {
      setConnectionStatus({ error: data.error });
      setErrorState({ isOpen: true, error: data.error });
    };

    // Subscribe to events
    EventsOn('p2p:connecting', handleConnecting);
    EventsOn('p2p:connected', handleConnected);
    EventsOn('p2p:peer_count', handlePeerCount);
    EventsOn('p2p:syncing', handleSyncing);
    EventsOn('chain:sync', handleChainSync);
    EventsOn('p2p:synced', handleSynced);
    EventsOn('p2p:error', handleError);

    // Cleanup
    return () => {
      clearInterval(pollInterval);
      if (debounceRef.current) clearTimeout(debounceRef.current);
      EventsOff('p2p:connecting');
      EventsOff('p2p:connected');
      EventsOff('p2p:peer_count');
      EventsOff('p2p:syncing');
      EventsOff('chain:sync');
      EventsOff('p2p:synced');
      EventsOff('p2p:error');
    };
  }, [setConnectionStatus, setBlockchainInfo]);

  // Handler to dismiss error dialog
  const dismissError = useCallback(() => {
    setErrorState({ isOpen: false, error: '' });
  }, []);

  // Handler to retry P2P connection
  const retryConnection = useCallback(() => {
    setErrorState({ isOpen: false, error: '' });
    setConnectionStatus({ error: undefined });
    StartP2P().catch((err) => {
      console.error('Failed to retry P2P connection:', err);
    });
  }, [setConnectionStatus]);

  return {
    errorDialogOpen: errorState.isOpen,
    errorMessage: errorState.error,
    dismissError,
    retryConnection,
  };
};
