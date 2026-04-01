import { useEffect, useRef } from 'react';
import { EventsOn } from '@wailsjs/runtime/runtime';
import {
  EventHandler,
  eventManager,
  BalanceChangedEvent,
  TransactionEvent,
  BlockEvent,
  StakingEvent,
  MasternodeEvent,
  WalletEvents,
  TransactionEvents,
  BlockEvents,
  StakingEvents,
  MasternodeEvents,
} from '@/lib/events';

/**
 * Generic hook for subscribing to wallet events
 * @param eventName - Name of the event to subscribe to
 * @param handler - Handler function to call when event is received
 * @param deps - Dependency array for the handler
 */
export function useWalletEvent<T>(
  eventName: string,
  handler: EventHandler<T>,
  deps: React.DependencyList = []
): void {
  const handlerRef = useRef(handler);

  // Update handler ref when it changes
  useEffect(() => {
    handlerRef.current = handler;
  }, [handler]);

  useEffect(() => {
    // Wrap handler to use latest version
    const wrappedHandler = (data: T) => {
      handlerRef.current(data);
    };

    // Subscribe to event using Wails runtime
    // EventsOn returns an unsubscribe function
    const unsubscribe = EventsOn(eventName, wrappedHandler);

    // Track listener
    eventManager.on(eventName, wrappedHandler);

    // Cleanup on unmount
    return () => {
      unsubscribe(); // Call the unsubscribe function returned by EventsOn
      eventManager.off(eventName, wrappedHandler);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [eventName, ...deps]);
}

/**
 * Hook for balance change events
 */
export function useBalanceChanged(handler: EventHandler<BalanceChangedEvent>) {
  useWalletEvent(WalletEvents.BALANCE_CHANGED, handler);
}

/**
 * Hook for new transaction events
 */
export function useNewTransaction(handler: EventHandler<TransactionEvent>) {
  useWalletEvent(TransactionEvents.NEW, handler);
}

/**
 * Hook for transaction confirmation events
 */
export function useTransactionConfirmed(handler: EventHandler<TransactionEvent>) {
  useWalletEvent(TransactionEvents.CONFIRMED, handler);
}

/**
 * Hook for new block events
 */
export function useNewBlock(handler: EventHandler<BlockEvent>) {
  useWalletEvent(BlockEvents.NEW, handler);
}

/**
 * Hook for staking reward events
 */
export function useStakeReward(handler: EventHandler<StakingEvent>) {
  useWalletEvent(StakingEvents.REWARD, handler);
}

/**
 * Hook for masternode payment events
 */
export function useMasternodePayment(handler: EventHandler<MasternodeEvent>) {
  useWalletEvent(MasternodeEvents.PAYMENT, handler);
}

/**
 * Hook for wallet lock events
 */
export function useWalletLocked(handler: EventHandler<void>) {
  useWalletEvent(WalletEvents.LOCKED, handler);
}

/**
 * Hook for wallet unlock events
 */
export function useWalletUnlocked(handler: EventHandler<void>) {
  useWalletEvent(WalletEvents.UNLOCKED, handler);
}

/**
 * Hook for multiple event subscriptions
 * @param subscriptions - Map of event names to handlers
 */
export function useMultipleEvents(
  subscriptions: Record<string, EventHandler<any>>
): void {
  useEffect(() => {
    const unsubscribers: Array<() => void> = [];

    // Subscribe to all events
    Object.entries(subscriptions).forEach(([eventName, handler]) => {
      const wailsUnsub = EventsOn(eventName, handler);
      const managerUnsub = eventManager.on(eventName, handler);
      unsubscribers.push(wailsUnsub);
      unsubscribers.push(managerUnsub);
    });

    // Cleanup all subscriptions
    return () => {
      unsubscribers.forEach(unsub => unsub());
    };
  }, [subscriptions]);
}
