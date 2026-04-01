import React, { useState, useCallback, useEffect, forwardRef } from 'react';
import { Eye, EyeOff, AlertTriangle } from 'lucide-react';
import { useTranslation } from 'react-i18next';

export interface PassphraseInputProps {
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  placeholder?: string;
  /** Override tooltip for show passphrase toggle */
  showLabel?: string;
  /** Override tooltip for hide passphrase toggle */
  hideLabel?: string;
  /** Override caps lock warning text */
  capsLockLabel?: string;
}

/**
 * Passphrase input with show/hide toggle and caps lock warning.
 * Manages showPassphrase and capsLock state internally.
 * Resets visual state when value is cleared.
 */
export const PassphraseInput = forwardRef<HTMLInputElement, PassphraseInputProps>(({
  value,
  onChange,
  disabled = false,
  placeholder,
  showLabel,
  hideLabel,
  capsLockLabel,
}, ref) => {
  const { t } = useTranslation();
  const [showPassphrase, setShowPassphrase] = useState(false);
  const [capsLockOn, setCapsLockOn] = useState(false);

  // Reset visual state when value is cleared (e.g. dialog reopens)
  useEffect(() => {
    if (!value) {
      setShowPassphrase(false);
      setCapsLockOn(false);
    }
  }, [value]);

  const handleKeyEvent = useCallback((e: React.KeyboardEvent) => {
    setCapsLockOn(e.getModifierState('CapsLock'));
  }, []);

  return (
    <>
      <div className="qt-hbox" style={{ gap: '8px', alignItems: 'center' }}>
        <input
          ref={ref}
          type={showPassphrase ? 'text' : 'password'}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={handleKeyEvent}
          onKeyUp={handleKeyEvent}
          placeholder={placeholder}
          disabled={disabled}
          className="qt-input"
          style={{
            flex: 1,
            padding: '8px 10px',
            fontSize: '12px',
            backgroundColor: disabled ? '#232323' : '#2b2b2b',
            border: '1px solid #1a1a1a',
            opacity: disabled ? 0.5 : 1,
          }}
        />
        <button
          onClick={() => setShowPassphrase(!showPassphrase)}
          disabled={disabled}
          className="qt-button-icon"
          style={{
            padding: '8px',
            backgroundColor: '#404040',
            border: '1px solid #555',
            borderRadius: '2px',
            cursor: disabled ? 'not-allowed' : 'pointer',
            opacity: disabled ? 0.5 : 1,
          }}
          title={showPassphrase
            ? (hideLabel || t('walletEncryption.hidePassphrase'))
            : (showLabel || t('walletEncryption.showPassphrase'))}
        >
          {showPassphrase ? (
            <EyeOff size={16} style={{ color: '#ddd' }} />
          ) : (
            <Eye size={16} style={{ color: '#ddd' }} />
          )}
        </button>
      </div>
      {capsLockOn && (
        <div className="qt-hbox" style={{ gap: '4px', alignItems: 'center' }}>
          <AlertTriangle size={14} style={{ color: '#ffaa00' }} />
          <span style={{ fontSize: '11px', color: '#ffaa00' }}>
            {capsLockLabel || t('walletEncryption.capsLockOn')}
          </span>
        </div>
      )}
    </>
  );
});
