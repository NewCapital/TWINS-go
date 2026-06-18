// OpReturnDataDialog.tsx — R7 OP_RETURN full-payload viewer.
//
// Companion to the data_carrier branch of <OutputRow> in TransactionDetail.tsx.
// Opens on click of the preview text and shows the full hex + best-effort
// ASCII representations of the payload, each with a Copy affordance.
//
// Scope is strictly OP_RETURN — nonstandard scriptPubKey hex display is
// deferred to a separate task that requires backend extraction (currently
// the explorer DTO only ships data_hex/data_ascii for `nulldata` outputs).
//
// Close affordances: backdrop click, Escape key, X icon button. Focus moves
// to the close button on open so keyboard users can dismiss without picking
// up a Tab cycle.

import React, { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { X, Copy, Check } from 'lucide-react';
import { IconButton } from '@/shared/components/IconButton';
import { writeToClipboard } from '@/shared/utils/clipboard';

export interface OpReturnDataDialogProps {
  isOpen: boolean;
  dataHex: string;
  dataAscii: string;
  onClose: () => void;
  zIndex?: number;
}

export const OpReturnDataDialog: React.FC<OpReturnDataDialogProps> = ({
  isOpen,
  dataHex,
  dataAscii,
  onClose,
  zIndex,
}) => {
  const { t } = useTranslation('common');
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const [copiedField, setCopiedField] = useState<'hex' | 'ascii' | null>(null);
  // Track the running 2s Check-icon timer in a ref so concurrent re-copies
  // cancel the prior schedule (mirrors TransactionDetail.handleCopy's
  // copyTimerRef pattern at TransactionDetail.tsx:1051). Unmount-cleanup
  // useEffect below clears any pending timer when the dialog leaves the tree.
  const copyTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Reset stale Check-icon state whenever the dialog closes so the next
  // open does not flash a 2s-old "copied" indicator.
  useEffect(() => {
    if (!isOpen) {
      setCopiedField(null);
      if (copyTimerRef.current !== null) {
        clearTimeout(copyTimerRef.current);
        copyTimerRef.current = null;
      }
    }
  }, [isOpen]);

  // Unmount-cleanup: clear any pending Check-revert so React does not emit
  // a "state update on unmounted component" warning if the dialog tree
  // unmounts (tx navigation) mid-timer.
  useEffect(() => {
    return () => {
      if (copyTimerRef.current !== null) {
        clearTimeout(copyTimerRef.current);
        copyTimerRef.current = null;
      }
    };
  }, []);

  // Autofocus close on open — keyboard users get an obvious dismiss target
  // without needing to Tab through hex/ASCII Copy buttons first.
  useEffect(() => {
    if (isOpen && closeButtonRef.current) {
      closeButtonRef.current.focus();
    }
  }, [isOpen]);

  // Escape key handler scoped to open state.
  useEffect(() => {
    if (!isOpen) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [isOpen, onClose]);

  if (!isOpen) return null;

  const handleCopy = async (text: string, field: 'hex' | 'ascii') => {
    // Defer the success indicator until the clipboard write actually
    // succeeds. writeToClipboard returns false on permission denial or
    // unavailable clipboard API (Wails restricted-context fallback), in
    // which case flashing a green Check would be misleading.
    const ok = await writeToClipboard(text);
    if (!ok) return;
    // Cancel any in-flight Check-revert so a fresh copy resets the 2s
    // window cleanly rather than letting the prior timer flip Check->Copy
    // mid-display.
    if (copyTimerRef.current !== null) {
      clearTimeout(copyTimerRef.current);
    }
    setCopiedField(field);
    copyTimerRef.current = setTimeout(() => {
      setCopiedField((curr) => (curr === field ? null : curr));
      copyTimerRef.current = null;
    }, 2000);
  };

  const asciiIsEmpty = dataAscii === '';
  const hexIsEmpty = dataHex === '';
  const overlayZ = zIndex ?? 1000;
  const modalZ = overlayZ + 1;

  const sectionLabelStyle: React.CSSProperties = {
    fontSize: '11px',
    fontWeight: 500,
    color: '#888',
  };

  const sectionHeaderRowStyle: React.CSSProperties = {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
  };

  const payloadBlockStyle: React.CSSProperties = {
    backgroundColor: '#1f1f1f',
    border: '1px solid #3a3a3a',
    borderRadius: '4px',
    padding: '8px 10px',
    fontFamily: 'monospace',
    fontSize: '11px',
    whiteSpace: 'pre-wrap',
    wordBreak: 'break-all',
    maxHeight: '200px',
    overflow: 'auto',
  };

  return (
    <>
      <div
        onClick={onClose}
        style={{
          position: 'fixed',
          inset: 0,
          backgroundColor: 'rgba(0, 0, 0, 0.6)',
          zIndex: overlayZ,
        }}
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-label={t('explorer.tx.dataModal.title')}
        style={{
          position: 'fixed',
          top: '50%',
          left: '50%',
          transform: 'translate(-50%, -50%)',
          width: '90%',
          maxWidth: '600px',
          maxHeight: '80vh',
          display: 'flex',
          flexDirection: 'column',
          backgroundColor: '#2f2f2f',
          border: '1px solid #3a3a3a',
          borderRadius: '8px',
          padding: '16px 20px',
          gap: '12px',
          zIndex: modalZ,
          color: '#ddd',
        }}
      >
        <div style={sectionHeaderRowStyle}>
          <div style={{ fontSize: '14px', fontWeight: 600 }}>
            {t('explorer.tx.dataModal.title')}
          </div>
          <IconButton
            ref={closeButtonRef}
            onClick={onClose}
            title={t('explorer.tx.dataModal.close')}
            ariaLabel={t('explorer.tx.dataModal.close')}
            icon={<X size={14} />}
          />
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
          <div style={sectionHeaderRowStyle}>
            <div style={sectionLabelStyle}>{t('explorer.tx.dataModal.hexLabel')}</div>
            <IconButton
              onClick={() => handleCopy(dataHex, 'hex')}
              title={t('explorer.tx.dataModal.copyHex')}
              ariaLabel={t('explorer.tx.dataModal.copyHex')}
              disabled={hexIsEmpty}
              icon={
                copiedField === 'hex' ? (
                  <Check size={12} color="#27ae60" />
                ) : (
                  <Copy size={12} />
                )
              }
            />
          </div>
          <div style={payloadBlockStyle}>{dataHex || '—'}</div>
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
          <div style={sectionHeaderRowStyle}>
            <div style={sectionLabelStyle}>{t('explorer.tx.dataModal.asciiLabel')}</div>
            <IconButton
              onClick={() => handleCopy(dataAscii, 'ascii')}
              title={t('explorer.tx.dataModal.copyAscii')}
              ariaLabel={t('explorer.tx.dataModal.copyAscii')}
              disabled={asciiIsEmpty}
              icon={
                copiedField === 'ascii' ? (
                  <Check size={12} color="#27ae60" />
                ) : (
                  <Copy size={12} />
                )
              }
            />
          </div>
          <div
            style={{
              ...payloadBlockStyle,
              color: asciiIsEmpty ? '#666' : '#ddd',
              fontStyle: asciiIsEmpty ? 'italic' : 'normal',
            }}
          >
            {asciiIsEmpty ? t('explorer.tx.dataModal.binaryPlaceholder') : dataAscii}
          </div>
        </div>
      </div>
    </>
  );
};
