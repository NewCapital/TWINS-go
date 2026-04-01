import { LockWallet, RestoreToStakingOnlyMode } from '@wailsjs/go/main/App';

/**
 * Restores the wallet to the state it was in before a temporary full unlock.
 *
 * - 'unlocked_staking' → RestoreToStakingOnlyMode()
 *   Keys stay in memory, staking continues, sends become blocked again.
 * - 'locked'           → LockWallet()
 *   Keys cleared from memory, staking stops.
 * - anything else      → no-op
 *   Wallet was fully unlocked or unencrypted; no state change needed.
 */
export async function restoreWalletState(priorStatus: string): Promise<void> {
  if (priorStatus === 'unlocked_staking') {
    await RestoreToStakingOnlyMode();
  } else if (priorStatus === 'locked') {
    await LockWallet();
  }
}
