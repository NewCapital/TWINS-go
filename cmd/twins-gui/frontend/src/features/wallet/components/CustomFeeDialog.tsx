import React, { useState, useEffect } from 'react';

export interface CustomFeeDialogProps {
  isOpen: boolean;
  currentFeeRate: number;
  onClose: () => void;
  onConfirm: (feeRate: number) => void;
}

export const CustomFeeDialog: React.FC<CustomFeeDialogProps> = ({
  isOpen,
  currentFeeRate,
  onClose,
  onConfirm,
}) => {
  const [inputValue, setInputValue] = useState('');
  const [error, setError] = useState<string | null>(null);

  // Reset input when dialog opens
  useEffect(() => {
    if (isOpen) {
      setInputValue(currentFeeRate.toFixed(8));
      setError(null);
    }
  }, [isOpen, currentFeeRate]);

  if (!isOpen) return null;

  const handleConfirm = () => {
    const value = parseFloat(inputValue);
    if (isNaN(value) || value <= 0) {
      setError('Please enter a valid positive fee rate');
      return;
    }
    if (value < 0.00001) {
      setError('Fee rate too low (minimum: 0.00001 TWINS/kB)');
      return;
    }
    if (value > 1) {
      setError('Fee rate too high (maximum: 1 TWINS/kB)');
      return;
    }
    onConfirm(value);
    onClose();
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') handleConfirm();
    if (e.key === 'Escape') onClose();
  };

  return (
    <div style={{
      position: 'fixed',
      top: 0,
      left: 0,
      right: 0,
      bottom: 0,
      backgroundColor: 'rgba(0, 0, 0, 0.7)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      zIndex: 1000,
    }}>
      <div style={{
        backgroundColor: '#3a3a3a',
        border: '1px solid #555',
        borderRadius: '4px',
        padding: '16px',
        minWidth: '320px',
        boxShadow: '0 4px 12px rgba(0,0,0,0.5)',
      }}>
        <h3 style={{ margin: '0 0 16px 0', fontSize: '14px', color: '#ddd' }}>
          Custom Fee Rate
        </h3>

        <div style={{ marginBottom: '12px' }}>
          <label style={{ fontSize: '12px', color: '#aaa', display: 'block', marginBottom: '4px' }}>
            Fee Rate (TWINS/kB):
          </label>
          <input
            type="text"
            value={inputValue}
            onChange={(e) => { setInputValue(e.target.value); setError(null); }}
            onKeyDown={handleKeyDown}
            autoFocus
            style={{
              width: '100%',
              padding: '8px',
              backgroundColor: '#2a2a2a',
              border: error ? '1px solid #c00' : '1px solid #555',
              borderRadius: '2px',
              color: '#ddd',
              fontSize: '13px',
              boxSizing: 'border-box',
            }}
          />
          {error && (
            <div style={{ color: '#f66', fontSize: '11px', marginTop: '4px' }}>
              {error}
            </div>
          )}
        </div>

        <div style={{ fontSize: '11px', color: '#888', marginBottom: '16px' }}>
          Default: 0.0001 TWINS/kB (normal) to 0.001 TWINS/kB (fast)
        </div>

        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '8px' }}>
          <button
            onClick={onClose}
            style={{
              padding: '6px 16px',
              backgroundColor: '#4a4a4a',
              border: '1px solid #555',
              borderRadius: '2px',
              color: '#ddd',
              cursor: 'pointer',
              fontSize: '12px',
            }}
          >
            Cancel
          </button>
          <button
            onClick={handleConfirm}
            style={{
              padding: '6px 16px',
              backgroundColor: '#0066cc',
              border: '1px solid #0055aa',
              borderRadius: '2px',
              color: '#fff',
              cursor: 'pointer',
              fontSize: '12px',
            }}
          >
            OK
          </button>
        </div>
      </div>
    </div>
  );
};
