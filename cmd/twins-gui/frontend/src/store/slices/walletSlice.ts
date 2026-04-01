import type { SliceCreator } from '../store.types';
import { Address, WalletInfo, core } from '@/shared/types/wallet.types';

export interface WalletSlice {
  // State
  walletInfo: WalletInfo | null;
  balance: core.Balance;
  addresses: Address[];
  transactions: core.Transaction[];
  isEncrypted: boolean;
  isLoading: boolean;

  // Actions
  setWalletInfo: (info: WalletInfo) => void;
  setBalance: (balance: core.Balance) => void;
  updateBalance: (balance: Partial<core.Balance>) => void;
  addAddress: (address: Address) => void;
  removeAddress: (address: string) => void;
  addTransaction: (transaction: core.Transaction) => void;
  setTransactions: (transactions: core.Transaction[]) => void;
  setLoading: (loading: boolean) => void;
  resetWallet: () => void;
}

const initialBalance: core.Balance = new core.Balance({
  total: 0,
  available: 0,
  spendable: 0,
  pending: 0,
  immature: 0,
  locked: 0,
});

export const createWalletSlice: SliceCreator<WalletSlice> = (set) => ({
  // Initial state
  walletInfo: null,
  balance: initialBalance,
  addresses: [],
  transactions: [],
  isEncrypted: true,
  isLoading: false,

  // Actions
  setWalletInfo: (info) => set({ walletInfo: info }),

  setBalance: (balance) => set({ balance }),

  updateBalance: (balance) =>
    set((state) => ({
      balance: new core.Balance({ ...state.balance, ...balance }),
    })),

  addAddress: (address) =>
    set((state) => ({
      addresses: [...state.addresses, address],
    })),

  removeAddress: (address) =>
    set((state) => ({
      addresses: state.addresses.filter((a) => a.address !== address),
    })),

  addTransaction: (transaction) =>
    set((state) => ({
      transactions: [transaction, ...state.transactions],
    })),

  setTransactions: (transactions) => set({ transactions }),

  setLoading: (loading) => set({ isLoading: loading }),

  resetWallet: () =>
    set({
      walletInfo: null,
      balance: initialBalance,
      addresses: [],
      transactions: [],
      isLoading: false,
    }),
});