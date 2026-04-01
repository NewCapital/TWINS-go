import React, { useEffect, useState, useRef, useCallback } from 'react';
import { X, AlertTriangle, Eye, EyeOff, KeyRound, Shield, ShieldCheck, ShieldAlert } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { sanitizeErrorMessage } from '@/shared/utils/sanitize';

export interface ChangePassphraseDialogProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess?: () => void;
  /** Override z-index for overlay and modal (default: 50). Use when rendering inside a higher z-index dialog. */
  zIndex?: number;
}

type DialogState = 'input' | 'changing' | 'success' | 'error';
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
  if (/[a-z]/.test(passphrase)) score += 10;
  if (/[A-Z]/.test(passphrase)) score += 10;
  if (/[0-9]/.test(passphrase)) score += 10;
  if (/[^a-zA-Z0-9]/.test(passphrase)) score += 10;

  // Bonus for mixed character positions (up to 20 points)
  const hasUpperInMiddle = passphrase.length >= 3 && /[A-Z]/.test(passphrase.slice(1, -1));
  const hasNumberInMiddle = passphrase.length >= 3 && /[0-9]/.test(passphrase.slice(1, -1));
  if (hasUpperInMiddle) score += 10;
  if (hasNumberInMiddle) score += 10;

  score = Math.min(100, score);

  let level: StrengthLevel;
  if (score < 30) level = 'weak';
  else if (score < 50) level = 'medium';
  else if (score < 70) level = 'strong';
  else level = 'very-strong';

  return { score, level };
}

/**
 * Dialog for changing the wallet passphrase.
 * Features:
 * - Old passphrase input
 * - New passphrase input with strength indicator
 * - Confirm new passphrase input
 * - Show/hide toggles for each field
 * - Caps Lock warning
 */
export const ChangePassphraseDialog: React.FC<ChangePassphraseDialogProps> = ({
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
  const [oldPassphrase, setOldPassphrase] = useState('');
  const [newPassphrase, setNewPassphrase] = useState('');
  const [confirmPassphrase, setConfirmPassphrase] = useState('');
  const [showOldPassphrase, setShowOldPassphrase] = useState(false);
  const [showNewPassphrase, setShowNewPassphrase] = useState(false);
  const [showConfirmPassphrase, setShowConfirmPassphrase] = useState(false);
  const [capsLockOn, setCapsLockOn] = useState(false);

  // Refs
  const oldPassphraseInputRef = useRef<HTMLInputElement>(null);

  // Calculate new passphrase strength
  const { score: strengthScore, level: strengthLevel } = evaluatePassphraseStrength(newPassphrase);

  // Validation
  const passphrasesMatch = newPassphrase === confirmPassphrase;
  const isOldValid = oldPassphrase.length >= 1;
  const isNewValid = newPassphrase.length >= 1;
  const isConfirmValid = confirmPassphrase.length >= 1 && passphrasesMatch;
  const canChange = isOldValid && isNewValid && isConfirmValid && strengthLevel !== 'weak';

  // Reset state when dialog opens
  useEffect(() => {
    if (isOpen) {
      setDialogState('input');
      setOldPassphrase('');
      setNewPassphrase('');
      setConfirmPassphrase('');
      setShowOldPassphrase(false);
      setShowNewPassphrase(false);
      setShowConfirmPassphrase(false);
      setErrorMessage('');
      setCapsLockOn(false);
      // Focus old passphrase input after a short delay
      setTimeout(() => {
        oldPassphraseInputRef.current?.focus();
      }, 100);
    }
  }, [isOpen]);

  // Caps Lock detection
  const handleKeyEvent = useCallback((e: React.KeyboardEvent) => {
    setCapsLockOn(e.getModifierState('CapsLock'));
  }, []);

  // Handle change action
  const handleChange = useCallback(async () => {
    if (!canChange || dialogState === 'changing') return;

    setDialogState('changing');
    setErrorMessage('');

    try {
      // Dynamic import of Wails bindings
      const { ChangeWalletPassphrase } = await import('@wailsjs/go/main/App');

      const result = await ChangeWalletPassphrase({
        oldPassphrase,
        newPassphrase,
      });

      if (result.success) {
        setDialogState('success');
        // Clear passphrases from state immediately
        setOldPassphrase('');
        setNewPassphrase('');
        setConfirmPassphrase('');
        onSuccess?.();
      } else {
        setDialogState('error');
        setErrorMessage(sanitizeErrorMessage(result.error || t('walletEncryption.changePassphrase.errorGeneric')));
      }
    } catch (err) {
      setDialogState('error');
      setErrorMessage(sanitizeErrorMessage(err instanceof Error ? err.message : t('walletEncryption.changePassphrase.errorGeneric')));
    }
  }, [canChange, dialogState, oldPassphrase, newPassphrase, onSuccess, t]);

  // Keyboard handling
  useEffect(() => {
    if (!isOpen) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && dialogState !== 'changing') {
        onClose();
      } else if (e.key === 'Enter' && dialogState === 'input' && canChange) {
        handleChange();
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, dialogState, canChange, handleChange, onClose]);

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
        onClick={dialogState !== 'changing' ? onClose : undefined}
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
                <KeyRound size={20} style={{ color: '#0088ff' }} />
                <span className="qt-header-label" style={{ fontSize: '14px' }}>
                  {t('walletEncryption.changePassphrase.title')}
                </span>
              </div>
              <button
                onClick={onClose}
                disabled={dialogState === 'changing'}
                className="qt-button-icon"
                aria-label={t('buttons.close')}
                style={{
                  padding: '4px',
                  backgroundColor: 'transparent',
                  border: 'none',
                  cursor: dialogState === 'changing' ? 'not-allowed' : 'pointer',
                  opacity: dialogState === 'changing' ? 0.5 : 1,
                }}
              >
                <X size={18} style={{ color: '#999' }} />
              </button>
            </div>

            {dialogState === 'input' && (
              <>
                {/* Old Passphrase Input */}
                <div className="qt-vbox" style={{ gap: '8px' }}>
                  <span className="qt-label" style={{ fontSize: '12px' }}>
                    {t('walletEncryption.changePassphrase.oldPassphraseLabel')}
                  </span>
                  <div className="qt-hbox" style={{ gap: '8px', alignItems: 'center' }}>
                    <input
                      ref={oldPassphraseInputRef}
                      type={showOldPassphrase ? 'text' : 'password'}
                      value={oldPassphrase}
                      onChange={(e) => setOldPassphrase(e.target.value)}
                      onKeyDown={handleKeyEvent}
                      onKeyUp={handleKeyEvent}
                      placeholder={t('walletEncryption.changePassphrase.oldPassphrasePlaceholder')}
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
                      onClick={() => setShowOldPassphrase(!showOldPassphrase)}
                      className="qt-button-icon"
                      style={{
                        padding: '8px',
                        backgroundColor: '#404040',
                        border: '1px solid #555',
                        borderRadius: '2px',
                      }}
                      title={showOldPassphrase ? t('walletEncryption.hidePassphrase') : t('walletEncryption.showPassphrase')}
                    >
                      {showOldPassphrase ? (
                        <EyeOff size={16} style={{ color: '#ddd' }} />
                      ) : (
                        <Eye size={16} style={{ color: '#ddd' }} />
                      )}
                    </button>
                  </div>
                </div>

                {/* New Passphrase Input */}
                <div className="qt-vbox" style={{ gap: '8px' }}>
                  <span className="qt-label" style={{ fontSize: '12px' }}>
                    {t('walletEncryption.changePassphrase.newPassphraseLabel')}
                  </span>
                  <div className="qt-hbox" style={{ gap: '8px', alignItems: 'center' }}>
                    <input
                      type={showNewPassphrase ? 'text' : 'password'}
                      value={newPassphrase}
                      onChange={(e) => setNewPassphrase(e.target.value)}
                      onKeyDown={handleKeyEvent}
                      onKeyUp={handleKeyEvent}
                      placeholder={t('walletEncryption.changePassphrase.newPassphrasePlaceholder')}
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
                      onClick={() => setShowNewPassphrase(!showNewPassphrase)}
                      className="qt-button-icon"
                      style={{
                        padding: '8px',
                        backgroundColor: '#404040',
                        border: '1px solid #555',
                        borderRadius: '2px',
                      }}
                      title={showNewPassphrase ? t('walletEncryption.hidePassphrase') : t('walletEncryption.showPassphrase')}
                    >
                      {showNewPassphrase ? (
                        <EyeOff size={16} style={{ color: '#ddd' }} />
                      ) : (
                        <Eye size={16} style={{ color: '#ddd' }} />
                      )}
                    </button>
                  </div>

                  {/* Strength Indicator */}
                  {newPassphrase.length > 0 && (
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

                {/* Confirm New Passphrase Input */}
                <div className="qt-vbox" style={{ gap: '8px' }}>
                  <span className="qt-label" style={{ fontSize: '12px' }}>
                    {t('walletEncryption.changePassphrase.confirmPassphraseLabel')}
                  </span>
                  <div className="qt-hbox" style={{ gap: '8px', alignItems: 'center' }}>
                    <input
                      type={showConfirmPassphrase ? 'text' : 'password'}
                      value={confirmPassphrase}
                      onChange={(e) => setConfirmPassphrase(e.target.value)}
                      onKeyDown={handleKeyEvent}
                      onKeyUp={handleKeyEvent}
                      placeholder={t('walletEncryption.changePassphrase.confirmPassphrasePlaceholder')}
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
                      {t('walletEncryption.changePassphrase.passphrasesMismatch')}
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
                {newPassphrase.length > 0 && strengthLevel === 'weak' && (
                  <div className="qt-hbox" style={{ gap: '4px', alignItems: 'center' }}>
                    <ShieldAlert size={14} style={{ color: '#ff4444' }} />
                    <span style={{ fontSize: '11px', color: '#ff4444' }}>
                      {t('walletEncryption.changePassphrase.weakPassphraseWarning')}
                    </span>
                  </div>
                )}
              </>
            )}

            {dialogState === 'changing' && (
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
                  {t('walletEncryption.changePassphrase.changing')}
                </span>
              </div>
            )}

            {dialogState === 'success' && (
              <div className="qt-vbox" style={{ alignItems: 'center', padding: '20px 0', gap: '12px' }}>
                <ShieldCheck size={48} style={{ color: '#44bb44' }} />
                <span style={{ fontSize: '14px', color: '#44bb44', fontWeight: 'bold' }}>
                  {t('walletEncryption.changePassphrase.successTitle')}
                </span>
                <span style={{ fontSize: '12px', color: '#ddd', textAlign: 'center', lineHeight: '1.5' }}>
                  {t('walletEncryption.changePassphrase.successMessage')}
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
                    onClick={handleChange}
                    disabled={!canChange}
                    className="qt-button-primary"
                    style={{
                      padding: '8px 20px',
                      fontSize: '12px',
                      backgroundColor: canChange ? '#5a5a5a' : '#3a3a3a',
                      border: '1px solid #666',
                      borderRadius: '3px',
                      color: '#fff',
                      cursor: canChange ? 'pointer' : 'not-allowed',
                      opacity: canChange ? 1 : 0.5,
                      minWidth: '80px',
                    }}
                  >
                    {t('walletEncryption.changePassphrase.changeButton')}
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
