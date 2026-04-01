import type { SliceCreator } from '../store.types';
import {
  UTXO,
  CoinControl,
  CoinControlViewMode,
  CoinControlFilterMode,
  CoinControlSortMode,
  CoinControlSummary,
  UTXOTreeNode,
} from '@/shared/types/wallet.types';
import { MIN_TX_FEE, calculateFee } from '@/utils/feeCalculation';
import { ListUnspent, LockUnspent, ListLockUnspent } from '@wailsjs/go/main/App';

/**
 * Coin Control State
 * Manages UTXO selection, locking, and configuration
 */
export interface CoinControlState {
  // UTXOs
  utxos: UTXO[];
  isLoadingUTXOs: boolean;

  // Coin Control configuration
  coinControl: CoinControl;

  // View preferences (persisted)
  viewMode: CoinControlViewMode;
  filterMode: CoinControlFilterMode;
  sortMode: CoinControlSortMode;
  sortAscending: boolean;

  // Dialog state
  isCoinControlDialogOpen: boolean;
  savedSelection: Set<string>;

  // Tree view state
  expandedAddresses: Set<string>;

  // Summary
  summary: CoinControlSummary | null;
}

/**
 * Coin Control Actions
 */
export interface CoinControlActions {
  // UTXO loading
  loadUTXOs: () => Promise<void>;
  setUTXOs: (utxos: UTXO[]) => void;

  // Coin selection
  selectCoin: (txid: string, vout: number) => void;
  unselectCoin: (txid: string, vout: number) => void;
  selectAllCoins: () => void;  // Renamed to avoid conflict with transactionsSlice.selectAll
  unselectAllCoins: () => void;  // Renamed to avoid conflict with transactionsSlice.unselectAll
  toggleCoinSelection: (txid: string, vout: number) => void;

  // Coin locking
  lockCoin: (txid: string, vout: number) => Promise<boolean>;
  unlockCoin: (txid: string, vout: number) => Promise<boolean>;
  toggleCoinLock: (txid: string, vout: number) => Promise<boolean>;
  toggleAllLocks: (visibleUTXOs: UTXO[]) => Promise<{ success: number; failed: number }>;

  // Configuration
  setSplitBlock: (enabled: boolean, count?: number) => void;
  setAllowOtherInputs: (enabled: boolean) => void;
  setMinimumTotalFee: (fee: number) => void;

  // View preferences
  setViewMode: (mode: CoinControlViewMode) => void;
  setFilterMode: (mode: CoinControlFilterMode) => void;
  setSortMode: (mode: CoinControlSortMode, ascending?: boolean) => void;

  // Tree view
  toggleAddressExpanded: (address: string) => void;
  expandAllAddresses: () => void;
  collapseAllAddresses: () => void;
  selectAddressCoins: (address: string) => void;
  unselectAddressCoins: (address: string) => void;
  buildTreeView: () => UTXOTreeNode[];

  // Dialog
  openDialog: () => void;
  closeDialog: () => void;
  cancelDialog: () => void;

  // Summary calculation
  calculateSummary: (recipientAmount: number, feeRate: number, recipientCount?: number) => void;

  // Reset
  resetCoinControl: () => void;
}

export type CoinControlSlice = CoinControlState & CoinControlActions;

// Helper function to create coin key
const getCoinKey = (txid: string, vout: number): string => `${txid}:${vout}`;

// Initial coin control configuration
const initialCoinControl: CoinControl = {
  selectedCoins: new Set<string>(),
  lockedCoins: new Set<string>(),
  splitBlock: false,
  splitCount: 1,
  allowOtherInputs: false,
  allowWatchOnly: true,
  minimumTotalFee: 0,
};

const initialState: CoinControlState = {
  utxos: [],
  isLoadingUTXOs: false,
  coinControl: initialCoinControl,
  viewMode: 'list',
  filterMode: 'spendable',
  sortMode: 'amount',
  sortAscending: false,
  isCoinControlDialogOpen: false,
  savedSelection: new Set<string>(),
  expandedAddresses: new Set<string>(),
  summary: null,
};

export const createCoinControlSlice: SliceCreator<CoinControlSlice> = (set, get) => ({
  ...initialState,

  // UTXO loading
  loadUTXOs: async () => {
    set((state) => {
      state.isLoadingUTXOs = true;
    });

    try {
      // Call backend ListUnspent method (minConf=0, maxConf=9999999)
      const backendUTXOs = await ListUnspent(0, 9999999);

      // Also get locked coins to sync state
      const lockedOutPoints = await ListLockUnspent();
      const lockedKeys = new Set(lockedOutPoints.map((op: { txid: string; vout: number }) => `${op.txid}:${op.vout}`));

      // Map backend UTXOs to frontend format and sync locked state
      const utxos: UTXO[] = backendUTXOs.map((u: any) => ({
        txid: u.txid,
        vout: u.vout,
        address: u.address,
        label: u.label || '',
        amount: u.amount,
        confirmations: u.confirmations,
        spendable: u.spendable,
        solvable: u.solvable ?? true,
        locked: u.locked || lockedKeys.has(`${u.txid}:${u.vout}`),
        type: u.type || 'Personal',
        date: u.date || Math.floor(Date.now() / 1000),
        priority: u.priority || (u.amount * u.confirmations) / 148,
      }));

      // Pre-compute fresh UTXO key set for stale-coin cleanup (applied in both branches)
      const freshKeys = new Set(utxos.map(u => getCoinKey(u.txid, u.vout)));

      // Check if still mounted/needed before setting state
      const currentState = get();
      if (currentState.isCoinControlDialogOpen) {
        set((state) => {
          state.utxos = utxos;
          state.isLoadingUTXOs = false;
          // Sync locked coins from backend
          state.coinControl.lockedCoins = lockedKeys;
          // Remove stale selected coins that no longer exist in the refreshed UTXO set
          state.coinControl.selectedCoins = new Set(
            [...state.coinControl.selectedCoins].filter(key => freshKeys.has(key))
          );
        });
      } else {
        // Dialog closed — still clean up stale selected coins (e.g. future background refresh)
        set((state) => {
          state.isLoadingUTXOs = false;
          state.coinControl.selectedCoins = new Set(
            [...state.coinControl.selectedCoins].filter(key => freshKeys.has(key))
          );
        });
      }
    } catch (error) {
      console.error('Failed to load UTXOs:', error);
      set((state) => {
        state.isLoadingUTXOs = false;
      });
    }
  },

  setUTXOs: (utxos: UTXO[]) => {
    set((state) => {
      state.utxos = utxos;
    });
  },

  // Coin selection
  // Note: We explicitly create new Sets to ensure Zustand detects state changes
  // (immer + Set mutations can have edge cases with reference equality)
  selectCoin: (txid: string, vout: number) => {
    const key = getCoinKey(txid, vout);
    set((state) => {
      const newSet = new Set(state.coinControl.selectedCoins);
      newSet.add(key);
      state.coinControl.selectedCoins = newSet;
    });
  },

  unselectCoin: (txid: string, vout: number) => {
    const key = getCoinKey(txid, vout);
    set((state) => {
      const newSet = new Set(state.coinControl.selectedCoins);
      newSet.delete(key);
      state.coinControl.selectedCoins = newSet;
    });
  },

  selectAllCoins: () => {
    set((state) => {
      const { utxos, coinControl } = state;
      const newSet = new Set(coinControl.selectedCoins);
      // Select all spendable UTXOs and locked UTXOs (locked coins may have spendable=false locally)
      utxos.forEach((utxo) => {
        const key = getCoinKey(utxo.txid, utxo.vout);
        if (utxo.spendable || coinControl.lockedCoins.has(key)) {
          newSet.add(key);
        }
      });
      state.coinControl.selectedCoins = newSet;
    });
  },

  unselectAllCoins: () => {
    set((state) => {
      state.coinControl.selectedCoins = new Set();
    });
  },

  toggleCoinSelection: (txid: string, vout: number) => {
    const key = getCoinKey(txid, vout);
    const { coinControl } = get();

    if (coinControl.selectedCoins.has(key)) {
      get().unselectCoin(txid, vout);
    } else {
      get().selectCoin(txid, vout);
    }
  },

  // Coin locking
  // Note: We explicitly create new Sets to ensure Zustand detects state changes
  lockCoin: async (txid: string, vout: number): Promise<boolean> => {
    const key = getCoinKey(txid, vout);
    try {
      // Call backend LockUnspent method (false = lock)
      await LockUnspent(false, [{ txid, vout }]);

      set((state) => {
        const newLockedCoins = new Set(state.coinControl.lockedCoins);
        newLockedCoins.add(key);
        state.coinControl.lockedCoins = newLockedCoins;

        const newSelectedCoins = new Set(state.coinControl.selectedCoins);
        newSelectedCoins.delete(key); // Can't select locked coins
        state.coinControl.selectedCoins = newSelectedCoins;

        // Update UTXO locked status
        const utxo = state.utxos.find(u => u.txid === txid && u.vout === vout);
        if (utxo) {
          utxo.locked = true;
          utxo.spendable = false;
        }
      });
      return true;
    } catch (error) {
      console.error('Failed to lock coin:', error);
      // State not updated on error - UI remains consistent with backend
      return false;
    }
  },

  unlockCoin: async (txid: string, vout: number): Promise<boolean> => {
    const key = getCoinKey(txid, vout);
    try {
      // Call backend LockUnspent method (true = unlock)
      await LockUnspent(true, [{ txid, vout }]);

      set((state) => {
        const newLockedCoins = new Set(state.coinControl.lockedCoins);
        newLockedCoins.delete(key);
        state.coinControl.lockedCoins = newLockedCoins;

        // Update UTXO locked status
        const utxo = state.utxos.find(u => u.txid === txid && u.vout === vout);
        if (utxo) {
          utxo.locked = false;
          utxo.spendable = true;
        }
      });
      return true;
    } catch (error) {
      console.error('Failed to unlock coin:', error);
      // State not updated on error - UI remains consistent with backend
      return false;
    }
  },

  toggleCoinLock: async (txid: string, vout: number): Promise<boolean> => {
    const key = getCoinKey(txid, vout);
    const { coinControl } = get();

    if (coinControl.lockedCoins.has(key)) {
      return get().unlockCoin(txid, vout);
    } else {
      return get().lockCoin(txid, vout);
    }
  },

  toggleAllLocks: async (visibleUTXOs: UTXO[]): Promise<{ success: number; failed: number }> => {
    const { coinControl } = get();

    // Per-coin toggle matching legacy C++ behavior (coincontroldialog.cpp:207-242):
    // Each coin individually toggles - locked becomes unlocked, unlocked becomes locked
    // Batch into two groups for efficiency (backend LockUnspent accepts arrays)
    const toLock: { txid: string; vout: number }[] = [];
    const toUnlock: { txid: string; vout: number }[] = [];

    for (const utxo of visibleUTXOs) {
      const key = getCoinKey(utxo.txid, utxo.vout);
      if (coinControl.lockedCoins.has(key)) {
        toUnlock.push({ txid: utxo.txid, vout: utxo.vout });
      } else {
        toLock.push({ txid: utxo.txid, vout: utxo.vout });
      }
    }

    let lockSucceeded = false;
    let unlockSucceeded = false;

    // Execute as two batch backend calls instead of N individual calls
    try {
      if (toLock.length > 0) {
        await LockUnspent(false, toLock);
      }
      lockSucceeded = true;
    } catch (error) {
      console.error('Failed to batch lock coins:', error);
    }

    try {
      if (toUnlock.length > 0) {
        await LockUnspent(true, toUnlock);
      }
      unlockSucceeded = true;
    } catch (error) {
      console.error('Failed to batch unlock coins:', error);
    }

    // Only update state for operations that succeeded on the backend
    set((state) => {
      const newLockedCoins = new Set(state.coinControl.lockedCoins);
      const newSelectedCoins = new Set(state.coinControl.selectedCoins);

      if (lockSucceeded) {
        for (const op of toLock) {
          const key = getCoinKey(op.txid, op.vout);
          newLockedCoins.add(key);
          newSelectedCoins.delete(key); // Can't select locked coins
          const utxo = state.utxos.find(u => u.txid === op.txid && u.vout === op.vout);
          if (utxo) {
            utxo.locked = true;
            utxo.spendable = false;
          }
        }
      }

      if (unlockSucceeded) {
        for (const op of toUnlock) {
          const key = getCoinKey(op.txid, op.vout);
          newLockedCoins.delete(key);
          const utxo = state.utxos.find(u => u.txid === op.txid && u.vout === op.vout);
          if (utxo) {
            utxo.locked = false;
            utxo.spendable = true;
          }
        }
      }

      state.coinControl.lockedCoins = newLockedCoins;
      state.coinControl.selectedCoins = newSelectedCoins;
    });

    const failed = (lockSucceeded ? 0 : toLock.length) + (unlockSucceeded ? 0 : toUnlock.length);
    const success = visibleUTXOs.length - failed;
    if (failed > 0) {
      console.warn(`toggleAllLocks: ${success} succeeded, ${failed} failed`);
    }

    return { success, failed };
  },

  // Configuration
  setSplitBlock: (enabled: boolean, count: number = 1) => {
    set((state) => {
      state.coinControl.splitBlock = enabled;
      state.coinControl.splitCount = count;
    });
  },

  setAllowOtherInputs: (enabled: boolean) => {
    set((state) => {
      state.coinControl.allowOtherInputs = enabled;
    });
  },

  setMinimumTotalFee: (fee: number) => {
    set((state) => {
      state.coinControl.minimumTotalFee = fee;
    });
  },

  // View preferences
  setViewMode: (mode: CoinControlViewMode) => {
    set((state) => {
      state.viewMode = mode;
    });
  },

  setFilterMode: (mode: CoinControlFilterMode) => {
    set((state) => {
      state.filterMode = mode;
    });
  },

  setSortMode: (mode: CoinControlSortMode, ascending?: boolean) => {
    set((state) => {
      if (ascending !== undefined) {
        state.sortAscending = ascending;
      } else if (state.sortMode === mode) {
        // Same column clicked - toggle direction
        state.sortAscending = !state.sortAscending;
      } else {
        // New column - set default sort order
        state.sortAscending = mode === 'address';
      }
      state.sortMode = mode;
    });
  },

  // Dialog
  openDialog: () => {
    set((state) => {
      state.savedSelection = new Set(state.coinControl.selectedCoins);
      state.isCoinControlDialogOpen = true;
    });
    // CoinControlDialog's useEffect handles loadUTXOs when isDialogOpen becomes true
  },

  closeDialog: () => {
    set((state) => {
      state.isCoinControlDialogOpen = false;
    });
  },

  cancelDialog: () => {
    set((state) => {
      state.coinControl.selectedCoins = new Set(state.savedSelection);
      state.isCoinControlDialogOpen = false;
    });
  },

  // Summary calculation
  calculateSummary: (recipientAmount: number, feeRate: number, recipientCount: number = 1) => {
    const { utxos, coinControl } = get();

    // Get selected UTXOs
    const selectedUTXOs = utxos.filter((utxo) => {
      const key = getCoinKey(utxo.txid, utxo.vout);
      return coinControl.selectedCoins.has(key);
    });

    if (selectedUTXOs.length === 0) {
      set((state) => {
        state.summary = null;
      });
      return;
    }

    // Calculate totals
    const quantity = selectedUTXOs.length;
    const amount = selectedUTXOs.reduce((sum, utxo) => sum + utxo.amount, 0);

    // Estimate transaction size matching wallet/transactions.go EstimateFee:
    // 190*inputs + 34*(recipients+1) + 10  (the +1 accounts for the change output)
    const bytes = quantity * 190 + (recipientCount + 1) * 34 + 10;

    // Calculate fee using satoshi-precise arithmetic (matches feeCalculation.ts and backend)
    const fee = Math.max(calculateFee(bytes, feeRate), MIN_TX_FEE);

    // Change is only meaningful when a recipient amount is set; show 0 otherwise
    const change = recipientAmount > 0 ? amount - recipientAmount - fee : 0;
    const afterFee = amount - fee;

    // Calculate priority using the C++ formula from coincontroldialog.cpp:601:
    //   dPriority = Σ(nValue * (nDepth+1)) / (nBytes - nBytesInputs)
    // The divisor is output-only bytes (excludes input bytes), matching C++ behavior.
    const SATOSHIS_PER_TWINS = 100_000_000;
    const outputBytes = (recipientCount + 1) * 34 + 10;
    const priorityScore = selectedUTXOs.reduce(
      (sum, u) => sum + u.amount * SATOSHIS_PER_TWINS * (u.confirmations + 1), 0
    ) / outputBytes;
    // PRIORITY_MEDIUM = AllowFreeThreshold from legacy/src/txmempool.h: COIN*1440/250
    // PRIORITY_HIGH   maps to the C++ "high" tier (dPriority/10000 > dPriorityMedium).
    // Anything above this threshold also displays as "high" — the UI intentionally
    // collapses the upper C++ tiers (higher, highest) into a single "high" label.
    const PRIORITY_MEDIUM = (100_000_000 * 1440) / 250;        //     576,000,000
    const PRIORITY_HIGH   = PRIORITY_MEDIUM * 10_000;           // 5,760,000,000,000
    let priority = 'low';
    if (priorityScore > PRIORITY_HIGH) priority = 'high';
    else if (priorityScore > PRIORITY_MEDIUM) priority = 'medium';

    // Check for dust (change < 0.00000546 TWINS = 546 satoshis)
    const dust = change > 0 && change < 0.00000546;

    set((state) => {
      state.summary = {
        quantity,
        amount,
        fee,
        afterFee,
        bytes,
        priority,
        change,
        dust,
      };
    });
  },

  // Tree view actions
  toggleAddressExpanded: (address: string) => {
    set((state) => {
      if (state.expandedAddresses.has(address)) {
        state.expandedAddresses.delete(address);
      } else {
        state.expandedAddresses.add(address);
      }
    });
  },

  expandAllAddresses: () => {
    const { utxos } = get();
    const addresses = new Set(utxos.map((u) => u.address));
    set((state) => {
      state.expandedAddresses = addresses;
    });
  },

  collapseAllAddresses: () => {
    set((state) => {
      state.expandedAddresses.clear();
    });
  },

  selectAddressCoins: (address: string) => {
    const { utxos, coinControl } = get();
    set((state) => {
      const newSet = new Set(state.coinControl.selectedCoins);
      utxos
        .filter((u) => u.address === address && u.spendable && !coinControl.lockedCoins.has(getCoinKey(u.txid, u.vout)))
        .forEach((u) => {
          newSet.add(getCoinKey(u.txid, u.vout));
        });
      state.coinControl.selectedCoins = newSet;
    });
  },

  unselectAddressCoins: (address: string) => {
    const { utxos } = get();
    set((state) => {
      const newSet = new Set(state.coinControl.selectedCoins);
      utxos
        .filter((u) => u.address === address)
        .forEach((u) => {
          newSet.delete(getCoinKey(u.txid, u.vout));
        });
      state.coinControl.selectedCoins = newSet;
    });
  },

  buildTreeView: (): UTXOTreeNode[] => {
    const { utxos, coinControl, filterMode, sortMode, sortAscending } = get();

    // Filter UTXOs
    let filtered = [...utxos];
    switch (filterMode) {
      case 'spendable':
        filtered = filtered.filter((u) => u.spendable && !u.locked);
        break;
      case 'locked':
        filtered = filtered.filter((u) => u.locked);
        break;
      case 'all':
      default:
        break;
    }

    // Group by address
    const addressMap = new Map<string, UTXO[]>();
    filtered.forEach((utxo) => {
      const existing = addressMap.get(utxo.address) || [];
      existing.push(utxo);
      addressMap.set(utxo.address, existing);
    });

    // Build tree nodes
    const treeNodes: UTXOTreeNode[] = [];
    addressMap.forEach((addressUtxos, address) => {
      // Sort UTXOs within address group
      addressUtxos.sort((a, b) => {
        let comparison = 0;
        switch (sortMode) {
          case 'amount':
            comparison = a.amount - b.amount;
            break;
          case 'confirmations':
            comparison = a.confirmations - b.confirmations;
            break;
          case 'priority':
            comparison = a.priority - b.priority;
            break;
          case 'date':
            comparison = a.date - b.date;
            break;
          default:
            break;
        }
        return sortAscending ? comparison : -comparison;
      });

      // Calculate total amount for address
      const totalAmount = addressUtxos.reduce((sum, u) => sum + u.amount, 0);

      // Check if all coins in this address are selected
      const allSelected = addressUtxos.every((u) => {
        const key = getCoinKey(u.txid, u.vout);
        return coinControl.selectedCoins.has(key) || u.locked;
      });

      // Get label from first UTXO with a label
      const label = addressUtxos.find((u) => u.label)?.label || '';

      treeNodes.push({
        type: 'address',
        address,
        label,
        utxos: addressUtxos,
        totalAmount,
        selected: allSelected,
        expanded: get().expandedAddresses.has(address),
      });
    });

    // Sort address nodes
    treeNodes.sort((a, b) => {
      if (sortMode === 'address') {
        const comparison = (a.address || '').localeCompare(b.address || '');
        return sortAscending ? comparison : -comparison;
      }
      // Sort by total amount by default
      const comparison = (a.totalAmount || 0) - (b.totalAmount || 0);
      return sortAscending ? comparison : -comparison;
    });

    return treeNodes;
  },

  // Reset
  resetCoinControl: () => {
    set((state) => {
      state.coinControl = { ...initialCoinControl, lockedCoins: state.coinControl.lockedCoins };
      state.savedSelection = new Set<string>();
      state.expandedAddresses.clear();
      state.summary = null;
    });
  },
});
