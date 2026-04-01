// Event type definitions matching Go backend

// Event names - must match backend constants exactly
export const WalletEvents = {
  BALANCE_CHANGED: 'wallet:balance',
  LOCKED: 'wallet:locked',
  UNLOCKED: 'wallet:unlocked',
} as const;

export const TransactionEvents = {
  NEW: 'tx:new',
  CONFIRMED: 'tx:confirmed',
  MINED: 'tx:mined',
} as const;

export const BlockEvents = {
  NEW: 'block:new',
  CONFIRMED: 'block:confirmed',
  ORPHAN: 'block:orphan',
} as const;

export const MasternodeEvents = {
  STARTED: 'masternode:started',
  STOPPED: 'masternode:stopped',
  PAYMENT: 'masternode:payment',
  WINNER: 'masternode:winner',
} as const;

export const StakingEvents = {
  REWARD: 'stake:reward',
  STARTED: 'staking:started',
  STOPPED: 'staking:stopped',
} as const;

export const NetworkEvents = {
  PEER_CONNECTED: 'network:peer:connected',
  PEER_DISCONNECTED: 'network:peer:disconnected',
  SYNC_PROGRESS: 'network:sync:progress',
} as const;

// Event payload type definitions

export interface BalanceChangedEvent {
  confirmed: number;
  unconfirmed: number;
  immature: number;
  total: number;
}

export interface TransactionEvent {
  txid: string;
  type: string;
  amount: number;
  fee: number;
  confirmations: number;
  time: number;
  address?: string;
}

export interface BlockEvent {
  height: number;
  hash: string;
  time: number;
  transactions: number;
  confirmations: number;
}

export interface MasternodeEvent {
  alias: string;
  status: string;
  tier: string;
  amount?: number;
}

export interface StakingEvent {
  amount: number;
  address: string;
  block_height: number;
  time: number;
}

export interface NetworkEvent {
  peer?: string;
  connected?: boolean;
  progress?: number;
}

// Event handler type
export type EventHandler<T> = (data: T) => void;

// Event manager class for centralized event handling
export class EventManager {
  private static instance: EventManager;
  private listeners: Map<string, Set<Function>> = new Map();

  private constructor() {}

  static getInstance(): EventManager {
    if (!EventManager.instance) {
      EventManager.instance = new EventManager();
    }
    return EventManager.instance;
  }

  on<T>(eventName: string, handler: EventHandler<T>): () => void {
    // Note: EventsOn is imported from Wails runtime in the hook
    // We just track listeners here
    if (!this.listeners.has(eventName)) {
      this.listeners.set(eventName, new Set());
    }
    this.listeners.get(eventName)!.add(handler);

    // Return unsubscribe function
    return () => this.off(eventName, handler);
  }

  off<T>(eventName: string, handler: EventHandler<T>): void {
    const handlers = this.listeners.get(eventName);
    if (handlers) {
      handlers.delete(handler);
      if (handlers.size === 0) {
        this.listeners.delete(eventName);
      }
    }
  }

  offAll(eventName: string): void {
    this.listeners.delete(eventName);
  }

  clear(): void {
    this.listeners.clear();
  }

  getActiveListeners(): string[] {
    return Array.from(this.listeners.keys());
  }

  getListenerCount(eventName: string): number {
    return this.listeners.get(eventName)?.size || 0;
  }
}

// Global event manager instance
export const eventManager = EventManager.getInstance();
