import React, { useEffect, useRef } from 'react';
import { X, AlertTriangle } from 'lucide-react';

export interface SimpleConfirmDialogProps {
  isOpen: boolean;
  title: string;
  message: string;
  confirmText?: string;
  cancelText?: string;
  onConfirm: () => void;
  onCancel: () => void;
  isDestructive?: boolean;
  isLoading?: boolean;
  zIndex?: number;
}

/**
 * Simple confirmation dialog matching Qt wallet style.
 * Used for confirming masternode operations and other simple confirmations.
 */
export const SimpleConfirmDialog: React.FC<SimpleConfirmDialogProps> = ({
  isOpen,
  title,
  message,
  confirmText = 'Yes',
  cancelText = 'No',
  onConfirm,
  onCancel,
  isDestructive = false,
  isLoading = false,
  zIndex,
}) => {
  const confirmButtonRef = useRef<HTMLButtonElement>(null);
  const cancelButtonRef = useRef<HTMLButtonElement>(null);

  // Focus cancel button by default (safer option)
  useEffect(() => {
    if (isOpen && cancelButtonRef.current) {
      cancelButtonRef.current.focus();
    }
  }, [isOpen]);

  // Handle keyboard events
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (!isOpen || isLoading) return;

      if (e.key === 'Escape') {
        onCancel();
      } else if (e.key === 'Enter') {
        // Enter confirms only if confirm button is focused
        if (document.activeElement === confirmButtonRef.current) {
          onConfirm();
        }
      }
    };

    if (isOpen) {
      document.addEventListener('keydown', handleKeyDown);
    }

    return () => {
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [isOpen, isLoading, onConfirm, onCancel]);

  if (!isOpen) return null;

  return (
    <>
      {/* Overlay */}
      <div
        className="fixed inset-0 bg-black/60"
        style={{ zIndex: zIndex ?? 50 }}
        onClick={isLoading ? undefined : onCancel}
      />

      {/* Modal */}
      <div className="fixed inset-0 flex items-center justify-center pointer-events-none" style={{ zIndex: zIndex ?? 50 }}>
        <div
          className="qt-frame pointer-events-auto"
          style={{
            width: '400px',
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
                {isDestructive && (
                  <AlertTriangle size={20} style={{ color: '#ffaa00' }} />
                )}
                <span className="qt-header-label" style={{ fontSize: '14px' }}>
                  {title}
                </span>
              </div>
              <button
                onClick={onCancel}
                disabled={isLoading}
                className="qt-button-icon"
                style={{
                  padding: '4px',
                  backgroundColor: 'transparent',
                  border: 'none',
                  cursor: isLoading ? 'not-allowed' : 'pointer',
                  opacity: isLoading ? 0.5 : 1,
                }}
              >
                <X size={18} style={{ color: '#999' }} />
              </button>
            </div>

            {/* Message */}
            <div style={{
              fontSize: '12px',
              color: '#ddd',
              lineHeight: '1.5',
              paddingLeft: isDestructive ? '28px' : '0',
            }}>
              {message}
            </div>

            {/* Action Buttons */}
            <div className="qt-hbox" style={{ gap: '8px', justifyContent: 'flex-end', marginTop: '8px' }}>
              <button
                ref={cancelButtonRef}
                onClick={onCancel}
                disabled={isLoading}
                className="qt-button"
                style={{
                  padding: '6px 16px',
                  fontSize: '12px',
                  backgroundColor: '#404040',
                  border: '1px solid #555',
                  borderRadius: '3px',
                  color: '#ddd',
                  cursor: isLoading ? 'not-allowed' : 'pointer',
                  opacity: isLoading ? 0.5 : 1,
                  minWidth: '70px',
                }}
              >
                {cancelText}
              </button>
              <button
                ref={confirmButtonRef}
                onClick={onConfirm}
                disabled={isLoading}
                className="qt-button-primary"
                style={{
                  padding: '6px 16px',
                  fontSize: '12px',
                  backgroundColor: isLoading ? '#3a3a3a' : '#5a5a5a',
                  border: '1px solid #666',
                  borderRadius: '3px',
                  color: '#fff',
                  cursor: isLoading ? 'not-allowed' : 'pointer',
                  opacity: isLoading ? 0.5 : 1,
                  minWidth: '70px',
                }}
              >
                {isLoading ? 'Starting...' : confirmText}
              </button>
            </div>
          </div>
        </div>
      </div>
    </>
  );
};
