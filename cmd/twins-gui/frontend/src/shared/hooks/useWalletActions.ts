import { useCallback } from 'react';
import { useStore } from '@/store/useStore';
import { useShallow } from 'zustand/react/shallow';
import { GetBalance, GetTransactions } from '@wailsjs/go/main/App';
import { core } from '@/shared/types/wallet.types';

export const useWalletActions = () => {
  const {
    setBalance,
    setTransactions,
    addNotification,
    setLoading,
  } = useStore(useShallow((s) => ({
    setBalance: s.setBalance,
    setTransactions: s.setTransactions,
    addNotification: s.addNotification,
    setLoading: s.setLoading,
  })));

  // silent=true suppresses error notifications (used during startup when backend may not be ready)
  const refreshBalance = useCallback(async (silent = false) => {
    try {
      setLoading(true);
      const balance = await GetBalance();
      // Convert plain object to core.Balance instance
      if (balance) {
        setBalance(new core.Balance(balance));
      }
    } catch (error) {
      if (!silent) {
        addNotification({
          type: 'error',
          title: 'Failed to refresh balance',
          message: error instanceof Error ? error.message : 'Unknown error',
          duration: 5000,
        });
      }
    } finally {
      setLoading(false);
    }
  }, [setBalance, addNotification, setLoading]);

  // silent=true suppresses error notifications (used during startup when backend may not be ready)
  const refreshTransactions = useCallback(async (silent = false) => {
    try {
      setLoading(true);
      const transactions = await GetTransactions(50); // Get last 50 transactions
      // Convert plain objects to core.Transaction instances
      const txInstances = transactions.map(tx => new core.Transaction(tx));
      setTransactions(txInstances);
    } catch (error) {
      if (!silent) {
        addNotification({
          type: 'error',
          title: 'Failed to refresh transactions',
          message: error instanceof Error ? error.message : 'Unknown error',
          duration: 5000,
        });
      }
    } finally {
      setLoading(false);
    }
  }, [setTransactions, addNotification, setLoading]);

  const refreshWallet = useCallback(async (silent = false) => {
    await Promise.all([
      refreshBalance(silent),
      refreshTransactions(silent),
    ]);
  }, [refreshBalance, refreshTransactions]);

  return {
    refreshBalance,
    refreshTransactions,
    refreshWallet,
  };
};