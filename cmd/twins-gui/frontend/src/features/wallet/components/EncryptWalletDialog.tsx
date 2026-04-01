import React, { useEffect, useState, useRef, useCallback } from 'react';
import { X, AlertTriangle, Eye, EyeOff, Lock, Shield, ShieldCheck, ShieldAlert } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { sanitizeErrorMessage } from '@/shared/utils/sanitize';

export interface EncryptWalletDialogProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess?: () => void;
  /** Override z-index for overlay and modal (default: 50). Use when rendering inside a higher z-index dialog. */
  zIndex?: number;
}

type DialogState = 'input' | 'encrypting' | 'success' | 'error';
type StrengthLevel = 'weak' | 'medium' | 'strong' | 'very-strong';

/**
 * Evaluates passphrase strength based on length and character variety.
 * Returns a score from 0-100 and a strength level.
 */
function evaluatePassphraseStrength(passphrase: string): { score: number; level: StrengthLevel } {
  if (!passphrase) {
    return { score: 0, level: 'weak' };
  }

  let score = 0;

  // Length scoring (up to 40 points)
  if (passphrase.length >= 8) score += 10;
  if (passphrase.length >= 12) score += 10;
  if (passphrase.length >= 16) score += 10;
  if (passphrase.length >= 20) score += 10;

  // Character variety (up to 40 points)
  if (/[a-z]/.test(passphrase)) score += 10; // lowercase
  if (/[A-Z]/.test(passphrase)) score += 10; // uppercase
  if (/[0-9]/.test(passphrase)) score += 10; // numbers
  if (/[^a-zA-Z0-9]/.test(passphrase)) score += 10; // special chars

  // Bonus for mixed character positions (up to 20 points)
  const hasUpperInMiddle = passphrase.length >= 3 && /[A-Z]/.test(passphrase.slice(1, -1));
  const hasNumberInMiddle = passphrase.length >= 3 && /[0-9]/.test(passphrase.slice(1, -1));
  if (hasUpperInMiddle) score += 10;
  if (hasNumberInMiddle) score += 10;

  // Clamp to 100
  score = Math.min(100, score);

  // Determine level
  let level: StrengthLevel;
  if (score < 30) level = 'weak';
  else if (score < 50) level = 'medium';
  else if (score < 70) level = 'strong';
  else level = 'very-strong';

  return { score, level };
}

/**
 * Dialog for encrypting an unencrypted wallet.
 * Features:
 * - Passphrase input with show/hide toggle
 * - Confirmation input (must match)
 * - Passphrase strength indicator
 * - Caps Lock warning
 * - Warning about backup importance
 */
export const EncryptWalletDialog: React.FC<EncryptWalletDialogProps> = ({
  isOpen,
  onClose,
  onSuccess,
  zIndex,
}) => {
  const { t } = useTranslation();

  // Dialog state
  const [dialogState, setDialogState] = useState<DialogState>('input');
  const [errorMessage, setErrorMessage] = useState<string>('');

  // Form state
  const [passphrase, setPassphrase] = useState('');
  const [confirmPassphrase, setConfirmPassphrase] = useState('');
  const [showPassphrase, setShowPassphrase] = useState(false);
  const [showConfirmPassphrase, setShowConfirmPassphrase] = useState(false);
  const [capsLockOn, setCapsLockOn] = useState(false);

  // Refs
  const passphraseInputRef = useRef<HTMLInputElement>(null);

  // Calculate passphrase strength
  const { score: strengthScore, level: strengthLevel } = evaluatePassphraseStrength(passphrase);

  // Validation
  const passphrasesMatch = passphrase === confirmPassphrase;
  const isPassphraseValid = passphrase.length >= 1;
  const isConfirmValid = confirmPassphrase.length >= 1 && passphrasesMatch;
  const canEncrypt = isPassphraseValid && isConfirmValid && strengthLevel !== 'weak';

  // Reset state when dialog opens
  useEffect(() => {
    if (isOpen) {
      setDialogState('input');
      setPassphrase('');
      setConfirmPassphrase('');
      setShowPassphrase(false);
      setShowConfirmPassphrase(false);
      setErrorMessage('');
      setCapsLockOn(false);
      // Focus passphrase input after a short delay
      setTimeout(() => {
        passphraseInputRef.current?.focus();
      }, 100);
    }
  }, [isOpen]);

  // Caps Lock detection
  const handleKeyEvent = useCallback((e: React.KeyboardEvent) => {
    setCapsLockOn(e.getModifierState('CapsLock'));
  }, []);

  // Handle encrypt action
  const handleEncrypt = useCallback(async () => {
    if (!canEncrypt || dialogState === 'encrypting') return;

    setDialogState('encrypting');
    setErrorMessage('');

    try {
      // Dynamic import of Wails bindings
      const { EncryptWallet } = await import('@wailsjs/go/main/App');

      const result = await EncryptWallet({ passphrase });

      if (result.success) {
        setDialogState('success');
        // Clear passphrase from state immediately
        setPassphrase('');
        setConfirmPassphrase('');
        onSuccess?.();
      } else {
        setDialogState('error');
        setErrorMessage(sanitizeErrorMessage(result.error || t('walletEncryption.encrypt.errorGeneric')));
      }
    } catch (err) {
      setDialogState('error');
      setErrorMessage(sanitizeErrorMessage(err instanceof Error ? err.message : t('walletEncryption.encrypt.errorGeneric')));
    }
  }, [canEncrypt, dialogState, passphrase, onSuccess, t]);

  // Keyboard handling
  useEffect(() => {
    if (!isOpen) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && dialogState !== 'encrypting') {
        onClose();
      } else if (e.key === 'Enter' && dialogState === 'input' && canEncrypt) {
        handleEncrypt();
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, dialogState, canEncrypt, handleEncrypt, onClose]);

  if (!isOpen) return null;

  // Strength indicator colors
  const strengthColors: Record<StrengthLevel, string> = {
    'weak': '#ff4444',
    'medium': '#ffaa00',
    'strong': '#44bb44',
    'very-strong': '#00cc66',
  };

  const strengthLabels: Record<StrengthLevel, string> = {
    'weak': t('walletEncryption.strength.weak'),
    'medium': t('walletEncryption.strength.medium'),
    'strong': t('walletEncryption.strength.strong'),
    'very-strong': t('walletEncryption.strength.veryStrong'),
  };

  const StrengthIcon = strengthLevel === 'weak' ? ShieldAlert :
                       strengthLevel === 'medium' ? Shield :
                       ShieldCheck;

  return (
    <>
      {/* Overlay */}
      <div
        className="fixed inset-0 bg-black/60"
        style={{ zIndex: zIndex ?? 50 }}
        onClick={dialogState !== 'encrypting' ? onClose : undefined}
      />

      {/* Modal */}
      <div className="fixed inset-0 flex items-center justify-center pointer-events-none" style={{ zIndex: zIndex ?? 50 }}>
        <div
          className="qt-frame pointer-events-auto"
          style={{
            width: '480px',
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
                <Lock size={20} style={{ color: '#ffaa00' }} />
                <span className="qt-header-label" style={{ fontSize: '14px' }}>
                  {t('walletEncryption.encrypt.title')}
                </span>
              </div>
              <button
                onClick={onClose}
                disabled={dialogState === 'encrypting'}
                className="qt-button-icon"
                aria-label={t('buttons.close')}
                style={{
                  padding: '4px',
                  backgroundColor: 'transparent',
                  border: 'none',
                  cursor: dialogState === 'encrypting' ? 'not-allowed' : 'pointer',
                  opacity: dialogState === 'encrypting' ? 0.5 : 1,
                }}
              >
                <X size={18} style={{ color: '#999' }} />
              </button>
            </div>

            {dialogState === 'input' && (
              <>
                {/* Warning */}
                <div className="qt-frame-secondary" style={{
                  padding: '12px',
                  backgroundColor: '#4a3a2a',
                  border: '1px solid #ffaa00',
                  borderRadius: '2px',
                }}>
                  <div className="qt-hbox" style={{ gap: '8px', alignItems: 'flex-start' }}>
                    <AlertTriangle size={18} style={{ color: '#ffaa00', flexShrink: 0, marginTop: '2px' }} />
                    <div className="qt-vbox" style={{ gap: '8px' }}>
                      <span style={{ fontSize: '12px', color: '#ffaa00', fontWeight: 'bold' }}>
                        {t('walletEncryption.encrypt.warningTitle')}
                      </span>
                      <span style={{ fontSize: '11px', color: '#ddd', lineHeight: '1.5' }}>
                        {t('walletEncryption.encrypt.warningMessage')}
                      </span>
                    </div>
                  </div>
                </div>

                {/* Passphrase Input */}
                <div className="qt-vbox" style={{ gap: '8px' }}>
                  <span className="qt-label" style={{ fontSize: '12px' }}>
                    {t('walletEncryption.encrypt.passphraseLabel')}
                  </span>
                  <div className="qt-hbox" style={{ gap: '8px', alignItems: 'center' }}>
                    <input
                      ref={passphraseInputRef}
                      type={showPassphrase ? 'text' : 'password'}
                      value={passphrase}
                      onChange={(e) => setPassphrase(e.target.value)}
                      onKeyDown={handleKeyEvent}
                      onKeyUp={handleKeyEvent}
                      placeholder={t('walletEncryption.encrypt.passphrasePlaceholder')}
                      className="qt-input"
                      style={{
                        flex: 1,
                        padding: '8px 10px',
                        fontSize: '12px',
                        backgroundColor: '#2b2b2b',
                        border: '1px solid #1a1a1a',
                      }}
                    />
                    <button
                      onClick={() => setShowPassphrase(!showPassphrase)}
                      className="qt-button-icon"
                      style={{
                        padding: '8px',
                        backgroundColor: '#404040',
                        border: '1px solid #555',
                        borderRadius: '2px',
                      }}
                      title={showPassphrase ? t('walletEncryption.hidePassphrase') : t('walletEncryption.showPassphrase')}
                    >
                      {showPassphrase ? (
                        <EyeOff size={16} style={{ color: '#ddd' }} />
                      ) : (
                        <Eye size={16} style={{ color: '#ddd' }} />
                      )}
                    </button>
                  </div>

                  {/* Strength Indicator */}
                  {passphrase.length > 0 && (
                    <div className="qt-vbox" style={{ gap: '4px' }}>
                      <div className="qt-hbox" style={{ gap: '8px', alignItems: 'center' }}>
                        <StrengthIcon size={14} style={{ color: strengthColors[strengthLevel] }} />
                        <span style={{ fontSize: '11px', color: strengthColors[strengthLevel] }}>
                          {strengthLabels[strengthLevel]}
                        </span>
                      </div>
                      <div style={{
                        height: '4px',
                        backgroundColor: '#1a1a1a',
                        borderRadius: '2px',
                        overflow: 'hidden',
                      }}>
                        <div style={{
                          height: '100%',
                          width: `${strengthScore}%`,
                          backgroundColor: strengthColors[strengthLevel],
                          transition: 'width 0.2s, background-color 0.2s',
                        }} />
                      </div>
                    </div>
                  )}
                </div>

                {/* Confirm Passphrase Input */}
                <div className="qt-vbox" style={{ gap: '8px' }}>
                  <span className="qt-label" style={{ fontSize: '12px' }}>
                    {t('walletEncryption.encrypt.confirmLabel')}
                  </span>
                  <div className="qt-hbox" style={{ gap: '8px', alignItems: 'center' }}>
                    <input
                      type={showConfirmPassphrase ? 'text' : 'password'}
                      value={confirmPassphrase}
                      onChange={(e) => setConfirmPassphrase(e.target.value)}
                      onKeyDown={handleKeyEvent}
                      onKeyUp={handleKeyEvent}
                      placeholder={t('walletEncryption.encrypt.confirmPlaceholder')}
                      className="qt-input"
                      style={{
                        flex: 1,
                        padding: '8px 10px',
                        fontSize: '12px',
                        backgroundColor: '#2b2b2b',
                        border: `1px solid ${confirmPassphrase.length > 0 && !passphrasesMatch ? '#ff4444' : '#1a1a1a'}`,
                      }}
                    />
                    <button
                      onClick={() => setShowConfirmPassphrase(!showConfirmPassphrase)}
                      className="qt-button-icon"
                      style={{
                        padding: '8px',
                        backgroundColor: '#404040',
                        border: '1px solid #555',
                        borderRadius: '2px',
                      }}
                      title={showConfirmPassphrase ? t('walletEncryption.hidePassphrase') : t('walletEncryption.showPassphrase')}
                    >
                      {showConfirmPassphrase ? (
                        <EyeOff size={16} style={{ color: '#ddd' }} />
                      ) : (
                        <Eye size={16} style={{ color: '#ddd' }} />
                      )}
                    </button>
                  </div>
                  {confirmPassphrase.length > 0 && !passphrasesMatch && (
                    <span style={{ fontSize: '11px', color: '#ff4444' }}>
                      {t('walletEncryption.encrypt.passphrasesMismatch')}
                    </span>
                  )}
                </div>

                {/* Caps Lock Warning */}
                {capsLockOn && (
                  <div className="qt-hbox" style={{ gap: '4px', alignItems: 'center' }}>
                    <AlertTriangle size={14} style={{ color: '#ffaa00' }} />
                    <span style={{ fontSize: '11px', color: '#ffaa00' }}>
                      {t('walletEncryption.capsLockOn')}
                    </span>
                  </div>
                )}

                {/* Weak passphrase warning */}
                {passphrase.length > 0 && strengthLevel === 'weak' && (
                  <div className="qt-hbox" style={{ gap: '4px', alignItems: 'center' }}>
                    <ShieldAlert size={14} style={{ color: '#ff4444' }} />
                    <span style={{ fontSize: '11px', color: '#ff4444' }}>
                      {t('walletEncryption.encrypt.weakPassphraseWarning')}
                    </span>
                  </div>
                )}
              </>
            )}

            {dialogState === 'encrypting' && (
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
                  {t('walletEncryption.encrypt.encrypting')}
                </span>
              </div>
            )}

            {dialogState === 'success' && (
              <div className="qt-vbox" style={{ alignItems: 'center', padding: '20px 0', gap: '12px' }}>
                <ShieldCheck size={48} style={{ color: '#44bb44' }} />
                <span style={{ fontSize: '14px', color: '#44bb44', fontWeight: 'bold' }}>
                  {t('walletEncryption.encrypt.successTitle')}
                </span>
                <span style={{ fontSize: '12px', color: '#ddd', textAlign: 'center', lineHeight: '1.5' }}>
                  {t('walletEncryption.encrypt.successMessage')}
                </span>
              </div>
            )}

            {dialogState === 'error' && (
              <div className="qt-vbox" style={{ padding: '12px', gap: '12px' }}>
                <div className="qt-hbox" style={{ gap: '8px', alignItems: 'flex-start' }}>
                  <AlertTriangle size={18} style={{ color: '#ff4444', flexShrink: 0 }} />
                  <span style={{ fontSize: '12px', color: '#ff4444', lineHeight: '1.5' }}>
                    {errorMessage}
                  </span>
                </div>
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
                    onClick={handleEncrypt}
                    disabled={!canEncrypt}
                    className="qt-button-primary"
                    style={{
                      padding: '8px 20px',
                      fontSize: '12px',
                      backgroundColor: canEncrypt ? '#5a5a5a' : '#3a3a3a',
                      border: '1px solid #666',
                      borderRadius: '3px',
                      color: '#fff',
                      cursor: canEncrypt ? 'pointer' : 'not-allowed',
                      opacity: canEncrypt ? 1 : 0.5,
                      minWidth: '80px',
                    }}
                  >
                    {t('walletEncryption.encrypt.encryptButton')}
                  </button>
                </>
              )}
              {dialogState === 'success' && (
                <button
                  onClick={onClose}
                  className="qt-button-primary"
                  style={{
                    padding: '8px 20px',
                    fontSize: '12px',
                    backgroundColor: '#5a5a5a',
                    border: '1px solid #666',
                    borderRadius: '3px',
                    color: '#fff',
                    minWidth: '80px',
                  }}
                >
                  {t('buttons.ok')}
                </button>
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
