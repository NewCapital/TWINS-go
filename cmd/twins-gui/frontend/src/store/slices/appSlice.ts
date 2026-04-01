import type { SliceCreator } from '../store.types';
import { Notification, AppSettings, ConnectionStatus } from '@/shared/types/app.types';
import { core } from '@wailsjs/go/models';

export interface AppSlice {
  // State
  notifications: Notification[];
  settings: AppSettings;
  connectionStatus: ConnectionStatus;
  blockchainInfo: core.BlockchainInfo | null;
  coinControlEnabled: boolean; // Expert setting: fCoinControlFeatures
  isInitialized: boolean;
  displayUnit: number;   // 0=TWINS, 1=mTWINS, 2=uTWINS
  displayDigits: number; // Decimal places (2-8)

  // Actions
  addNotification: (notification: Omit<Notification, 'id' | 'timestamp'>) => void;
  removeNotification: (id: string) => void;
  clearNotifications: () => void;
  updateSettings: (settings: Partial<AppSettings>) => void;
  setConnectionStatus: (status: Partial<ConnectionStatus>) => void;
  setBlockchainInfo: (info: core.BlockchainInfo | null) => void;
  setCoinControlEnabled: (enabled: boolean) => void;
  syncCoinControlEnabled: () => Promise<void>;
  setInitialized: (initialized: boolean) => void;
  setDisplayUnit: (unit: number) => void;
  setDisplayDigits: (digits: number) => void;
  loadDisplayUnits: () => Promise<void>;

  // Computed
  getLatestNotification: () => Notification | undefined;
}

const defaultSettings: AppSettings = {
  theme: 'system',
  language: 'en',
  currency: 'USD',
  autoLock: true,
  autoLockTimeout: 10,
  minimizeToTray: true,
  startMinimized: false,
  enableNotifications: true,
};

const initialConnectionStatus: ConnectionStatus = {
  isConnected: false,
  peers: 0,
};

export const createAppSlice: SliceCreator<AppSlice> = (set, get) => ({
  // Initial state
  notifications: [],
  settings: defaultSettings,
  connectionStatus: initialConnectionStatus,
  blockchainInfo: null,
  coinControlEnabled: false, // Matches backend default for fCoinControlFeatures
  isInitialized: false,
  displayUnit: 0,
  displayDigits: 8,

  // Actions
  addNotification: (notification) => {
    const newNotification: Notification = {
      ...notification,
      id: crypto.randomUUID(),
      timestamp: new Date(),
    };

    set((state) => ({
      notifications: [newNotification, ...state.notifications],
    }));

    // Auto-remove after duration
    if (notification.duration) {
      setTimeout(() => {
        get().removeNotification(newNotification.id);
      }, notification.duration);
    }
  },

  removeNotification: (id) =>
    set((state) => ({
      notifications: state.notifications.filter((n) => n.id !== id),
    })),

  clearNotifications: () => set({ notifications: [] }),

  updateSettings: (settings) =>
    set((state) => ({
      settings: { ...state.settings, ...settings },
    })),

  setConnectionStatus: (status) =>
    set((state) => ({
      connectionStatus: { ...state.connectionStatus, ...status },
    })),

  setBlockchainInfo: (info) => set({ blockchainInfo: info }),

  setCoinControlEnabled: (enabled) => set({ coinControlEnabled: enabled }),

  syncCoinControlEnabled: async () => {
    try {
      const { GetSettingBool } = await import('@wailsjs/go/main/App');
      const enabled = await GetSettingBool('fCoinControlFeatures');
      set({ coinControlEnabled: enabled });
    } catch {
      // Silently fail, keep current value
    }
  },

  setInitialized: (initialized) => set({ isInitialized: initialized }),

  setDisplayUnit: (unit) => set({ displayUnit: unit }),

  setDisplayDigits: (digits) => set({ displayDigits: digits }),

  loadDisplayUnits: async () => {
    try {
      const { GetSettings } = await import('@wailsjs/go/main/App');
      const settings = await GetSettings();
      set({
        displayUnit: (settings as any).nDisplayUnit ?? 0,
        displayDigits: (settings as any).digits ?? 8,
      });
    } catch {
      // Use defaults on error
    }
  },

  // Computed
  getLatestNotification: () => get().notifications[0],
});