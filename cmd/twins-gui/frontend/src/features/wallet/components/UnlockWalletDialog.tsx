import React, { useEffect, useState, useRef, useCallback } from 'react';
import { X, AlertTriangle, Unlock, Lock, Clock, Coins } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { sanitizeErrorMessage } from '@/shared/utils/sanitize';
import { PassphraseInput } from '@/shared/components/PassphraseInput';

export interface UnlockWalletDialogProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess?: () => void;
  /** If true, default to staking-only unlock */
  defaultStakingOnly?: boolean;
  /** Override z-index for overlay and modal (default: 50). Use when rendering inside a higher z-index dialog. */
  zIndex?: number;
  /** If true, hides timeout and staking-only options. Used for action-based unlocks where wallet is restored automatically. */
  temporaryUnlock?: boolean;
}

type DialogState = 'input' | 'unlocking' | 'success' | 'error';

// Timeout options in seconds (0 = until close/manual lock)
const TIMEOUT_OPTIONS = [
  { value: 300, labelKey: 'walletEncryption.unlock.timeout5min' },
  { value: 900, labelKey: 'walletEncryption.unlock.timeout15min' },
  { value: 3600, labelKey: 'walletEncryption.unlock.timeout1hour' },
  { value: 0, labelKey: 'walletEncryption.unlock.timeoutUntilClose' },
] as const;

/**
 * Dialog for unlocking an encrypted wallet.
 * Features:
 * - Passphrase input with show/hide toggle
 * - Timeout selection (5 min, 15 min, 1 hour, until close)
 * - Staking-only mode option
 * - Caps Lock warning
 */
export const UnlockWalletDialog: React.FC<UnlockWalletDialogProps> = ({
  isOpen,
  onClose,
  onSuccess,
  defaultStakingOnly = false,
  zIndex,
  temporaryUnlock = false,
}) => {
  const { t } = useTranslation();

  // Dialog state
  const [dialogState, setDialogState] = useState<DialogState>('input');
  const [errorMessage, setErrorMessage] = useState<string>('');

  // Form state
  const [passphrase, setPassphrase] = useState('');
  const [timeout, setTimeout] = useState<number>(TIMEOUT_OPTIONS[0].value);
  const [stakingOnly, setStakingOnly] = useState(defaultStakingOnly);

  // Refs
  const passphraseInputRef = useRef<HTMLInputElement>(null);

  // Validation
  const canUnlock = passphrase.length >= 1;
  const isLoading = dialogState === 'unlocking';

  // Reset state when dialog opens
  useEffect(() => {
    if (isOpen) {
      setDialogState('input');
      setPassphrase('');
      setTimeout(TIMEOUT_OPTIONS[0].value);
      setStakingOnly(defaultStakingOnly);
      setErrorMessage('');
      // Focus passphrase input after a short delay
      window.setTimeout(() => {
        passphraseInputRef.current?.focus();
      }, 100);
    }
  }, [isOpen, defaultStakingOnly]);

  // Handle unlock action
  const handleUnlock = useCallback(async () => {
    if (!canUnlock || dialogState === 'unlocking') return;

    setDialogState('unlocking');
    setErrorMessage('');

    try {
      // Dynamic import of Wails bindings
      const { UnlockWallet } = await import('@wailsjs/go/main/App');

      const result = await UnlockWallet({
        passphrase,
        timeout: temporaryUnlock ? 60 : (stakingOnly ? 0 : timeout),
        stakingOnly: temporaryUnlock ? false : stakingOnly,
      });

      if (result.success) {
        setDialogState('success');
        // Clear passphrase from state immediately
        setPassphrase('');
        onSuccess?.();
        // Auto-close on success after brief delay
        window.setTimeout(() => {
          onClose();
        }, 500);
      } else {
        setDialogState('error');
        setErrorMessage(sanitizeErrorMessage(result.error || t('walletEncryption.unlock.errorGeneric')));
      }
    } catch (err) {
      setDialogState('error');
      setErrorMessage(sanitizeErrorMessage(err instanceof Error ? err.message : t('walletEncryption.unlock.errorGeneric')));
    }
  }, [canUnlock, dialogState, passphrase, timeout, stakingOnly, temporaryUnlock, onSuccess, onClose, t]);

  // Keyboard handling
  useEffect(() => {
    if (!isOpen) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && dialogState !== 'unlocking') {
        onClose();
      } else if (e.key === 'Enter' && dialogState === 'input' && canUnlock) {
        handleUnlock();
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, dialogState, canUnlock, handleUnlock, onClose]);

  if (!isOpen) return null;

  return (
    <>
      {/* Overlay */}
      <div
        className="fixed inset-0 bg-black/60"
        style={{ zIndex: zIndex ?? 50 }}
        onClick={dialogState !== 'unlocking' ? onClose : undefined}
      />

      {/* Modal */}
      <div className="fixed inset-0 flex items-center justify-center pointer-events-none" style={{ zIndex: zIndex ?? 50 }}>
        <div
          className="qt-frame pointer-events-auto"
          style={{
            width: '420px',
            maxWidth: '90vw',
            backgroundColor: '#2b2b2b',
            border: '1px solid #4a4a4a',
            borderRadius: '4px',
            boxShadow: '0 8px 32px rgba(0, 0, 0, 0.8)',
          }}
          onClick={(e) => e.stopPropagation()}
        >
          <div className="qt-vbox" style={{ padding: '20px', gap: '16px' }}>
            {/* Header */}
            <div className="qt-hbox" style={{ justifyContent: 'space-between', alignItems: 'center' }}>
              <div className="qt-hbox" style={{ gap: '8px', alignItems: 'center' }}>
                <Unlock size={20} style={{ color: '#44bb44' }} />
                <span className="qt-header-label" style={{ fontSize: '14px' }}>
                  {t('walletEncryption.unlock.title')}
                </span>
              </div>
              <button
                onClick={onClose}
                disabled={dialogState === 'unlocking'}
                className="qt-button-icon"
                aria-label={t('buttons.close')}
                style={{
                  padding: '4px',
                  backgroundColor: 'transparent',
                  border: 'none',
                  cursor: dialogState === 'unlocking' ? 'not-allowed' : 'pointer',
                  opacity: dialogState === 'unlocking' ? 0.5 : 1,
                }}
              >
                <X size={18} style={{ color: '#999' }} />
              </button>
            </div>

            {(dialogState === 'input' || dialogState === 'error') && (
              <>
                {/* Description */}
                <span style={{ fontSize: '12px', color: '#ddd', lineHeight: '1.5' }}>
                  {t('walletEncryption.unlock.description')}
                </span>

                {/* Passphrase Input */}
                <div className="qt-vbox" style={{ gap: '8px' }}>
                  <span className="qt-label" style={{ fontSize: '12px' }}>
                    {t('walletEncryption.unlock.passphraseLabel')}
                  </span>
                  <PassphraseInput
                    ref={passphraseInputRef}
                    value={passphrase}
                    onChange={setPassphrase}
                    disabled={isLoading}
                    placeholder={t('walletEncryption.unlock.passphrasePlaceholder')}
                  />
                </div>

                {temporaryUnlock ? (
                  /* Temporary unlock hint — no timeout/staking options needed */
                  <span style={{ fontSize: '11px', color: '#999', lineHeight: '1.5' }}>
                    {t('walletEncryption.unlock.temporaryHint', 'Wallet will be temporarily unlocked for this action.')}
                  </span>
                ) : (
                  <>
                    {/* Staking Only Option */}
                    <div className="qt-hbox" style={{ gap: '8px', alignItems: 'center' }}>
                      <input
                        type="checkbox"
                        id="stakingOnly"
                        checked={stakingOnly}
                        onChange={(e) => setStakingOnly(e.target.checked)}
                        disabled={isLoading}
                        style={{
                          width: '16px',
                          height: '16px',
                          cursor: isLoading ? 'not-allowed' : 'pointer',
                        }}
                      />
                      <label
                        htmlFor="stakingOnly"
                        className="qt-hbox"
                        style={{
                          gap: '6px',
                          alignItems: 'center',
                          cursor: isLoading ? 'not-allowed' : 'pointer',
                          opacity: isLoading ? 0.5 : 1,
                        }}
                      >
                        <Coins size={14} style={{ color: '#999' }} />
                        <span style={{ fontSize: '12px', color: '#ddd' }}>
                          {t('walletEncryption.unlock.stakingOnlyLabel')}
                        </span>
                      </label>
                    </div>
                    <span style={{ fontSize: '11px', color: '#999', marginTop: '-8px', paddingLeft: '24px' }}>
                      {t('walletEncryption.unlock.stakingOnlyDescription')}
                    </span>

                    {/* Timeout Selection */}
                    <div className="qt-vbox" style={{ gap: '8px', opacity: stakingOnly ? 0.4 : 1 }}>
                      <div className="qt-hbox" style={{ gap: '6px', alignItems: 'center' }}>
                        <Clock size={14} style={{ color: '#999' }} />
                        <span className="qt-label" style={{ fontSize: '12px' }}>
                          {t('walletEncryption.unlock.timeoutLabel')}
                        </span>
                      </div>
                      <select
                        value={stakingOnly ? 0 : timeout}
                        onChange={(e) => setTimeout(Number(e.target.value))}
                        disabled={isLoading || stakingOnly}
                        className="qt-input"
                        style={{
                          padding: '8px 10px',
                          fontSize: '12px',
                          backgroundColor: (isLoading || stakingOnly) ? '#232323' : '#2b2b2b',
                          border: '1px solid #1a1a1a',
                          color: '#ddd',
                          cursor: (isLoading || stakingOnly) ? 'not-allowed' : 'pointer',
                          opacity: (isLoading || stakingOnly) ? 0.5 : 1,
                        }}
                      >
                        {TIMEOUT_OPTIONS.map((option) => (
                          <option key={option.value} value={option.value}>
                            {t(option.labelKey)}
                          </option>
                        ))}
                      </select>
                    </div>
                  </>
                )}

                {/* Error Message */}
                {dialogState === 'error' && errorMessage && (
                  <div className="qt-hbox" style={{ gap: '8px', alignItems: 'flex-start' }}>
                    <AlertTriangle size={16} style={{ color: '#ff4444', flexShrink: 0, marginTop: '1px' }} />
                    <span style={{ fontSize: '12px', color: '#ff4444', lineHeight: '1.4' }}>
                      {errorMessage}
                    </span>
                  </div>
                )}
              </>
            )}

            {dialogState === 'unlocking' && (
              <div className="qt-vbox" style={{ alignItems: 'center', padding: '20px 0', gap: '12px' }}>
                <div className="spinner" style={{
                  width: '32px',
                  height: '32px',
                  border: '3px solid #333',
                  borderTop: '3px solid #0066cc',
                  borderRadius: '50%',
                  animation: 'spin 1s linear infinite',
                }} />
                <span style={{ fontSize: '12px', color: '#ddd' }}>
                  {t('walletEncryption.unlock.unlocking')}
                </span>
              </div>
            )}

            {dialogState === 'success' && (
              <div className="qt-vbox" style={{ alignItems: 'center', padding: '20px 0', gap: '12px' }}>
                {stakingOnly && !temporaryUnlock ? (
                  <>
                    <Lock size={48} style={{ color: '#cc9944' }} />
                    <span style={{ fontSize: '14px', color: '#cc9944', fontWeight: 'bold' }}>
                      {t('walletEncryption.unlock.successStakingOnly', 'Wallet Unlocked For Staking Only')}
                    </span>
                  </>
                ) : (
                  <>
                    <Unlock size={48} style={{ color: '#44bb44' }} />
                    <span style={{ fontSize: '14px', color: '#44bb44', fontWeight: 'bold' }}>
                      {t('walletEncryption.unlock.successTitle')}
                    </span>
                  </>
                )}
              </div>
            )}

            {/* Action Buttons */}
            <div className="qt-hbox" style={{ gap: '8px', justifyContent: 'flex-end', marginTop: '8px' }}>
              {(dialogState === 'input' || dialogState === 'error') && (
                <>
                  <button
                    onClick={onClose}
                    className="qt-button"
                    style={{
                      padding: '8px 20px',
                      fontSize: '12px',
                      backgroundColor: '#404040',
                      border: '1px solid #555',
                      borderRadius: '3px',
                      color: '#ddd',
                      minWidth: '80px',
                    }}
                  >
                    {t('buttons.cancel')}
                  </button>
                  <button
                    onClick={handleUnlock}
                    disabled={!canUnlock}
                    className="qt-button-primary"
                    style={{
                      padding: '8px 20px',
                      fontSize: '12px',
                      backgroundColor: canUnlock ? '#5a5a5a' : '#3a3a3a',
                      border: '1px solid #666',
                      borderRadius: '3px',
                      color: '#fff',
                      cursor: canUnlock ? 'pointer' : 'not-allowed',
                      opacity: canUnlock ? 1 : 0.5,
                      minWidth: '80px',
                    }}
                  >
                    {t('walletEncryption.unlock.unlockButton')}
                  </button>
                </>
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Spinner animation */}
      <style>{`
        @keyframes spin {
          0% { transform: rotate(0deg); }
          100% { transform: rotate(360deg); }
        }
      `}</style>
    </>
  );
};
