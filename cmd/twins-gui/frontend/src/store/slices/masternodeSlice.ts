import type { SliceCreator } from '../store.types';
import { Masternode, MasternodeTier, MasternodeStatus, NetworkMasternode, NetworkMasternodeFilters } from '@/shared/types/masternode.types';

export interface MasternodeSlice {
  // State - My Masternodes
  masternodes: Masternode[];
  selectedMasternode: Masternode | null;
  isLoading: boolean;
  isStartingMasternode: boolean;
  lastRefresh: number | null;
  operationError: string | null;
  operationSuccess: string | null;

  // State - Network Masternodes
  networkMasternodes: NetworkMasternode[];
  isLoadingNetwork: boolean;
  networkLastRefresh: number | null;
  networkFilters: NetworkMasternodeFilters;
  masternodeActiveTab: 'my' | 'network' | 'debug' | 'payments';

  // Actions - My Masternodes
  setMasternodes: (masternodes: Masternode[]) => void;
  addMasternode: (masternode: Masternode) => void;
  updateMasternode: (id: string, updates: Partial<Masternode>) => void;
  removeMasternode: (id: string) => void;
  selectMasternode: (masternode: Masternode | null) => void;
  setLoading: (loading: boolean) => void;
  setStartingMasternode: (starting: boolean) => void;
  setLastRefresh: (timestamp: number) => void;
  setOperationError: (error: string | null) => void;
  setOperationSuccess: (message: string | null) => void;
  clearOperationMessages: () => void;

  // Actions - Network Masternodes
  setNetworkMasternodes: (masternodes: NetworkMasternode[]) => void;
  setLoadingNetwork: (loading: boolean) => void;
  setNetworkLastRefresh: (timestamp: number) => void;
  setNetworkFilters: (filters: Partial<NetworkMasternodeFilters>) => void;
  setMasternodeActiveTab: (tab: 'my' | 'network' | 'debug' | 'payments') => void;

  // Computed - My Masternodes
  getMasternodesByTier: (tier: MasternodeTier) => Masternode[];
  getActiveMasternodes: () => Masternode[];
  getMasternodesByStatus: (status: MasternodeStatus) => Masternode[];
  getTotalRewards: () => number;
  getMissingMasternodes: () => Masternode[];

  // Computed - Network Masternodes
  getFilteredNetworkMasternodes: () => NetworkMasternode[];
  getNetworkMasternodeCount: () => { filtered: number; total: number };
}

// Default filter state
const defaultNetworkFilters: NetworkMasternodeFilters = {
  tier: 'all',
  status: 'all',
  search: '',
  sortColumn: 'rank',
  sortDirection: 'asc',
};

export const createMasternodeSlice: SliceCreator<MasternodeSlice> = (set, get) => ({
  // Initial state - My Masternodes
  masternodes: [],
  selectedMasternode: null,
  isLoading: false,
  isStartingMasternode: false,
  lastRefresh: null,
  operationError: null,
  operationSuccess: null,

  // Initial state - Network Masternodes
  networkMasternodes: [],
  isLoadingNetwork: false,
  networkLastRefresh: null,
  networkFilters: defaultNetworkFilters,
  masternodeActiveTab: 'my',

  // Actions - My Masternodes
  setMasternodes: (masternodes) => set({ masternodes }),

  addMasternode: (masternode) =>
    set((state) => ({
      masternodes: [...state.masternodes, masternode],
    })),

  updateMasternode: (id, updates) =>
    set((state) => ({
      masternodes: state.masternodes.map((mn) =>
        mn.id === id ? { ...mn, ...updates } : mn
      ),
    })),

  removeMasternode: (id) =>
    set((state) => ({
      masternodes: state.masternodes.filter((mn) => mn.id !== id),
    })),

  selectMasternode: (masternode) => set({ selectedMasternode: masternode }),

  setLoading: (loading) => set({ isLoading: loading }),

  setStartingMasternode: (starting) => set({ isStartingMasternode: starting }),

  setLastRefresh: (timestamp) => set({ lastRefresh: timestamp }),

  setOperationError: (error) => set({ operationError: error }),

  setOperationSuccess: (message) => set({ operationSuccess: message }),

  clearOperationMessages: () => set({ operationError: null, operationSuccess: null }),

  // Actions - Network Masternodes
  setNetworkMasternodes: (masternodes) => set({ networkMasternodes: masternodes }),

  setLoadingNetwork: (loading) => set({ isLoadingNetwork: loading }),

  setNetworkLastRefresh: (timestamp) => set({ networkLastRefresh: timestamp }),

  setNetworkFilters: (filters) =>
    set((state) => ({
      networkFilters: { ...state.networkFilters, ...filters },
    })),

  setMasternodeActiveTab: (tab) => set({ masternodeActiveTab: tab }),

  // Computed - My Masternodes
  getMasternodesByTier: (tier) => get().masternodes.filter((mn) => mn.tier === tier),

  getActiveMasternodes: () => get().masternodes.filter((mn) => mn.status === 'ENABLED'),

  getMasternodesByStatus: (status) => get().masternodes.filter((mn) => mn.status === status),

  getTotalRewards: () =>
    get().masternodes.reduce((total, mn) => total + mn.rewards, 0),

  getMissingMasternodes: () => get().masternodes.filter((mn) => mn.status === 'MISSING'),

  // Computed - Network Masternodes
  getFilteredNetworkMasternodes: () => {
    const { networkMasternodes, networkFilters } = get();
    let filtered = [...networkMasternodes];

    // Filter by tier
    if (networkFilters.tier !== 'all' && networkFilters.tier !== '') {
      filtered = filtered.filter((mn) => mn.tier.toLowerCase() === networkFilters.tier);
    }

    // Filter by status (case-insensitive comparison for robustness)
    if (networkFilters.status !== 'all') {
      const statusLower = networkFilters.status.toLowerCase();
      filtered = filtered.filter((mn) => mn.status.toLowerCase() === statusLower);
    }

    // Filter by search (address or pubkey)
    if (networkFilters.search.trim()) {
      const searchLower = networkFilters.search.toLowerCase().trim();
      filtered = filtered.filter(
        (mn) => mn.paymentaddress.toLowerCase().includes(searchLower)
      );
    }

    // Sort
    if (networkFilters.sortColumn) {
      filtered.sort((a, b) => {
        let aVal: string | number | undefined;
        let bVal: string | number | undefined;

        // Handle virtual columns (not direct properties of NetworkMasternode)
        if (networkFilters.sortColumn === 'network') {
          aVal = a.addr.includes('[') ? 'IPv6' : 'IPv4';
          bVal = b.addr.includes('[') ? 'IPv6' : 'IPv4';
        } else {
          aVal = a[networkFilters.sortColumn as keyof NetworkMasternode];
          bVal = b[networkFilters.sortColumn as keyof NetworkMasternode];
        }

        // Handle numeric vs string comparison
        if (typeof aVal === 'number' && typeof bVal === 'number') {
          return networkFilters.sortDirection === 'asc' ? aVal - bVal : bVal - aVal;
        }

        // String comparison
        const aStr = String(aVal ?? '').toLowerCase();
        const bStr = String(bVal ?? '').toLowerCase();
        if (networkFilters.sortDirection === 'asc') {
          return aStr.localeCompare(bStr);
        }
        return bStr.localeCompare(aStr);
      });
    }

    return filtered;
  },

  getNetworkMasternodeCount: () => {
    const { networkMasternodes } = get();
    const filtered = get().getFilteredNetworkMasternodes();
    return {
      filtered: filtered.length,
      total: networkMasternodes.length,
    };
  },
});