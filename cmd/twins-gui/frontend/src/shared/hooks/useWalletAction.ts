import { useState, useCallback, useRef } from 'react';
import { GetWalletEncryptionStatus } from '@wailsjs/go/main/App';
import { restoreWalletState } from '@/shared/utils/walletState';

export interface UseWalletActionOptions {
  /** Whether to restore wallet state (staking-only or locked) after action completes. Default: true */
  restoreAfter?: boolean;
  /** Called when user cancels the unlock dialog */
  onCancel?: () => void;
}

/**
 * Hook that consolidates the unlock → action → restore wallet state pattern.
 *
 * Usage:
 *   const { showUnlockDialog, executeWithUnlock, unlockDialogProps } = useWalletAction({ ... });
 *   // Call executeWithUnlock(async () => { ... }) to run an action that may require wallet unlock.
 *   // Render <UnlockWalletDialog isOpen={showUnlockDialog} {...unlockDialogProps} /> in your component.
 */
export function useWalletAction(options?: UseWalletActionOptions) {
  const restoreAfter = options?.restoreAfter ?? true;

  const [showUnlockDialog, setShowUnlockDialog] = useState(false);
  const pendingActionRef = useRef<(() => Promise<void>) | null>(null);
  const priorStatusRef = useRef('');
  const unlockSucceededRef = useRef(false);
  const onCancelRef = useRef(options?.onCancel);
  onCancelRef.current = options?.onCancel;

  const runAction = useCallback(async (action: () => Promise<void>) => {
    try {
      await action();
    } finally {
      if (restoreAfter) {
        const priorStatus = priorStatusRef.current;
        if (priorStatus) {
          priorStatusRef.current = '';
          try {
            await restoreWalletState(priorStatus);
          } catch {
            // Ignore restore errors
          }
        }
      }
    }
  }, [restoreAfter]);

  /**
   * Execute an async action, unlocking the wallet first if needed.
   * If the wallet is already unlocked, the action runs immediately.
   * If the wallet is locked/staking-only, the unlock dialog is shown
   * and the action runs after successful unlock.
   */
  const executeWithUnlock = useCallback(async (action: () => Promise<void>) => {
    priorStatusRef.current = '';
    unlockSucceededRef.current = false;

    try {
      const status = await GetWalletEncryptionStatus();
      if (status?.encrypted && (status?.locked || status?.status === 'unlocked_staking')) {
        // Wallet needs unlocking — show dialog, defer action
        priorStatusRef.current = status.status;
        pendingActionRef.current = action;
        setShowUnlockDialog(true);
        return;
      }
    } catch {
      // On error checking status, try to execute anyway (backend will handle)
    }

    // Wallet is already unlocked or not encrypted — execute directly
    await runAction(action);
  }, [runAction]);

  // Called by UnlockWalletDialog onSuccess
  const handleUnlockSuccess = useCallback(() => {
    unlockSucceededRef.current = true;
    setShowUnlockDialog(false);
    const action = pendingActionRef.current;
    pendingActionRef.current = null;
    if (action) {
      runAction(action);
    }
  }, [runAction]);

  // Called by UnlockWalletDialog onClose
  // IMPORTANT: UnlockWalletDialog calls onClose() 500ms AFTER onSuccess() for auto-close.
  // The unlockSucceededRef pattern ensures we only treat it as cancellation when the user
  // actually clicked Cancel/X/overlay, not when the dialog auto-closes after success.
  const handleUnlockClose = useCallback(() => {
    setShowUnlockDialog(false);
    if (!unlockSucceededRef.current) {
      // User actually cancelled
      pendingActionRef.current = null;
      priorStatusRef.current = '';
      onCancelRef.current?.();
    }
    unlockSucceededRef.current = false;
  }, []);

  return {
    showUnlockDialog,
    executeWithUnlock,
    /** Props to spread on UnlockWalletDialog: { onSuccess, onClose } */
    unlockDialogProps: {
      onSuccess: handleUnlockSuccess,
      onClose: handleUnlockClose,
    },
  };
}
