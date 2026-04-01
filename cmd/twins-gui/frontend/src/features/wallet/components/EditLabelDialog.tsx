/**
 * EditLabelDialog Component
 * Dialog for editing address labels in the transactions context menu.
 * Matches Qt wallet's editaddressdialog functionality.
 */

import React, { useEffect, useRef, useState } from 'react';
import { X, Tag } from 'lucide-react';
import { SetAddressLabel } from '@wailsjs/go/main/App';
import { sanitizeText } from '@/shared/utils/sanitize';

export interface EditLabelDialogProps {
  isOpen: boolean;
  address: string;
  currentLabel: string;
  onClose: () => void;
  onLabelUpdated: (address: string, newLabel: string) => void;
}

/**
 * Dialog for editing the label associated with an address.
 * Used by the transactions context menu "Edit label" action.
 */
export const EditLabelDialog: React.FC<EditLabelDialogProps> = ({
  isOpen,
  address,
  currentLabel,
  onClose,
  onLabelUpdated,
}) => {
  const [label, setLabel] = useState(currentLabel);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const okButtonRef = useRef<HTMLButtonElement>(null);

  // Reset state when dialog opens with new address
  useEffect(() => {
    if (isOpen) {
      setLabel(currentLabel);
      setError(null);
      setIsLoading(false);
      // Focus input after a short delay to ensure dialog is rendered
      setTimeout(() => {
        inputRef.current?.focus();
        inputRef.current?.select();
      }, 50);
    }
  }, [isOpen, currentLabel]);

  // Handle keyboard events
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (!isOpen || isLoading) return;

      if (e.key === 'Escape') {
        onClose();
      } else if (e.key === 'Enter') {
        handleSave();
      }
    };

    if (isOpen) {
      document.addEventListener('keydown', handleKeyDown);
    }

    return () => {
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [isOpen, isLoading, label]);

  const handleSave = async () => {
    // Validate label length (max 100 characters like Qt)
    if (label.length > 100) {
      setError('Label must be 100 characters or less');
      return;
    }

    setIsLoading(true);
    setError(null);

    try {
      await SetAddressLabel(address, label.trim());
      onLabelUpdated(address, label.trim());
      onClose();
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to update label';
      setError(sanitizeText(errorMessage));
    } finally {
      setIsLoading(false);
    }
  };

  if (!isOpen) return null;

  return (
    <>
      {/* Overlay */}
      <div
        className="fixed inset-0 bg-black/60 z-50"
        onClick={isLoading ? undefined : onClose}
      />

      {/* Modal */}
      <div className="fixed inset-0 flex items-center justify-center z-50 pointer-events-none">
        <div
          className="qt-frame pointer-events-auto"
          style={{
            width: '450px',
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
                <Tag size={18} style={{ color: '#888' }} />
                <span className="qt-header-label" style={{ fontSize: '14px' }}>
                  Edit Address Label
                </span>
              </div>
              <button
                type="button"
                onClick={onClose}
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

            {/* Address (read-only) */}
            <div className="qt-vbox" style={{ gap: '6px' }}>
              <label className="qt-label" style={{ fontSize: '12px', color: '#888' }}>
                Address
              </label>
              <div
                style={{
                  padding: '8px 10px',
                  backgroundColor: '#3a3a3a',
                  border: '1px solid #4a4a4a',
                  borderRadius: '2px',
                  fontFamily: 'monospace',
                  fontSize: '11px',
                  color: '#bbb',
                  wordBreak: 'break-all',
                }}
              >
                {address}
              </div>
            </div>

            {/* Label input */}
            <div className="qt-vbox" style={{ gap: '6px' }}>
              <label className="qt-label" style={{ fontSize: '12px', color: '#888' }}>
                Label
              </label>
              <input
                ref={inputRef}
                type="text"
                value={label}
                onChange={(e) => setLabel(e.target.value)}
                disabled={isLoading}
                placeholder="Enter a label for this address"
                maxLength={100}
                className="qt-input"
                style={{
                  padding: '8px 10px',
                  backgroundColor: '#3a3a3a',
                  border: '1px solid #4a4a4a',
                  borderRadius: '2px',
                  fontSize: '12px',
                  color: '#ddd',
                  outline: 'none',
                }}
              />
              <span className="qt-label" style={{ fontSize: '10px', color: '#666', textAlign: 'right' }}>
                {label.length}/100
              </span>
            </div>

            {/* Error message */}
            {error && (
              <div
                style={{
                  padding: '8px 10px',
                  backgroundColor: 'rgba(255, 0, 0, 0.1)',
                  border: '1px solid rgba(255, 0, 0, 0.3)',
                  borderRadius: '2px',
                  fontSize: '11px',
                  color: '#ff6b6b',
                }}
              >
                {error}
              </div>
            )}

            {/* Action Buttons */}
            <div className="qt-hbox" style={{ gap: '8px', justifyContent: 'flex-end', marginTop: '8px' }}>
              <button
                type="button"
                onClick={onClose}
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
                Cancel
              </button>
              <button
                ref={okButtonRef}
                type="button"
                onClick={handleSave}
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
                {isLoading ? 'Saving...' : 'OK'}
              </button>
            </div>
          </div>
        </div>
      </div>
    </>
  );
};
