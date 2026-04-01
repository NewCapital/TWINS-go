import { StateCreator, StoreMutatorIdentifier } from 'zustand';

type Logger = <
  T,
  Mps extends [StoreMutatorIdentifier, unknown][] = [],
  Mcs extends [StoreMutatorIdentifier, unknown][] = []
>(
  f: StateCreator<T, Mps, Mcs>,
  name?: string
) => StateCreator<T, Mps, Mcs>;

type LoggerImpl = <T>(
  f: StateCreator<T, [], []>,
  name?: string
) => StateCreator<T, [], []>;

const loggerImpl: LoggerImpl = (f, name) => (set, get, store) => {
  // Zustand v5 has stricter set() overloads. Use explicit parameter types
  // instead of rest args to satisfy both overloads.
  const loggedSet: typeof set = ((partial: unknown, replace?: boolean) => {
    const previousState = get();
    (set as (partial: unknown, replace?: boolean) => void)(partial, replace);
    const nextState = get();

    if (import.meta.env.DEV) {
      console.group(`[${name || 'Store'}] State Update`);
      console.log('Previous State:', previousState);
      console.log('Next State:', nextState);
      console.groupEnd();
    }
  }) as typeof set;

  return f(loggedSet, get, store);
};

// Standard zustand middleware cast: LoggerImpl uses concrete types for body inference,
// Logger provides the generic signature for middleware composition (see zustand devtools source).
export const logger = loggerImpl as unknown as Logger;