export interface Notification {
  id: string;
  type: 'success' | 'error' | 'warning' | 'info';
  title: string;
  message?: string;
  duration?: number;
  timestamp: Date;
}

export interface AppSettings {
  theme: 'light' | 'dark' | 'system';
  language: string;
  currency: string;
  autoLock: boolean;
  autoLockTimeout: number;
  minimizeToTray: boolean;
  startMinimized: boolean;
  enableNotifications: boolean;
}

export interface ConnectionStatus {
  isConnected: boolean;
  peers: number;
  error?: string;
}