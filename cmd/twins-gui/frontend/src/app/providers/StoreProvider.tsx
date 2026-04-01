import { ReactNode, useEffect } from 'react';
import { useStore } from '@/store/useStore';
import { useShallow } from 'zustand/react/shallow';
import { EventsOn } from '@wailsjs/runtime/runtime';

interface StoreProviderProps {
  children: ReactNode;
}

export const StoreProvider: React.FC<StoreProviderProps> = ({ children }) => {
  const { updateBalance, addTransaction, setConnectionStatus } = useStore(useShallow((s) => ({
    updateBalance: s.updateBalance,
    addTransaction: s.addTransaction,
    setConnectionStatus: s.setConnectionStatus,
  })));

  useEffect(() => {
    // Subscribe to backend events
    const unsubscribeBalance = EventsOn('wallet:balance:update', (balance) => {
      updateBalance(balance);
    });

    const unsubscribeTransaction = EventsOn('wallet:transaction:new', (transaction) => {
      addTransaction(transaction);
    });

    const unsubscribeConnection = EventsOn('connection:status', (status) => {
      setConnectionStatus(status);
    });

    return () => {
      unsubscribeBalance();
      unsubscribeTransaction();
      unsubscribeConnection();
    };
  }, [updateBalance, addTransaction, setConnectionStatus]);

  return <>{children}</>;
};