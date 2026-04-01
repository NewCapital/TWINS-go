import { describe, it, expect, beforeEach } from 'vitest';
import { useStore } from '../useStore';
import { core } from '@/shared/types/wallet.types';

describe('Wallet Slice', () => {
  beforeEach(() => {
    useStore.setState({
      balance: new core.Balance({
        total: 0,
        available: 0,
        spendable: 0,
        pending: 0,
        immature: 0,
        locked: 0,
      }),
      addresses: [],
      transactions: [],
    });
  });

  it('should update balance', () => {
    const { updateBalance } = useStore.getState();

    updateBalance({ available: 100, total: 100 });

    const newState = useStore.getState();
    expect(newState.balance.available).toBe(100);
    expect(newState.balance.total).toBe(100);
  });

  it('should add address', () => {
    const { addAddress } = useStore.getState();

    const newAddress = {
      address: 'twins123...',
      label: 'Main',
      isDefault: true,
      isUsed: false,
      createdAt: new Date(),
    };

    addAddress(newAddress);

    const newState = useStore.getState();
    expect(newState.addresses).toHaveLength(1);
    expect(newState.addresses[0].address).toBe('twins123...');
  });

});