import React from 'react';
import { useTranslation } from 'react-i18next';
import { AlertTriangle, RefreshCw, X } from 'lucide-react';
import '@/styles/qt-theme.css';

interface P2PErrorDialogProps {
  isOpen: boolean;
  error: string;
  onRetry: () => void;
  onDismiss: () => void;
}

/**
 * Dialog shown when P2P connection fails.
 * Provides options to retry connection or dismiss the error.
 */
export const P2PErrorDialog: React.FC<P2PErrorDialogProps> = ({
  isOpen,
  error,
  onRetry,
  onDismiss,
}) => {
  const { t } = useTranslation('common');

  if (!isOpen) return null;

  return (
    <div
      className="fixed inset-0 flex items-center justify-center z-50"
      style={{ backgroundColor: 'rgba(0, 0, 0, 0.7)' }}
    >
      <div
        className="rounded-lg shadow-xl max-w-md w-full mx-4"
        style={{
          backgroundColor: 'var(--qt-bg-secondary)',
          border: '1px solid var(--qt-border-color)',
        }}
      >
        {/* Header */}
        <div
          className="flex items-center justify-between px-4 py-3"
          style={{
            borderBottom: '1px solid var(--qt-border-color)',
          }}
        >
          <div className="flex items-center gap-2">
            <AlertTriangle
              size={20}
              style={{ color: 'var(--qt-warning-color, #f59e0b)' }}
            />
            <h3
              className="font-medium"
              style={{ color: 'var(--qt-text-primary)', fontSize: '14px' }}
            >
              {t('p2pError.title')}
            </h3>
          </div>
          <button
            onClick={onDismiss}
            className="p-1 rounded hover:bg-gray-700"
            title={t('buttons.close')}
          >
            <X size={16} style={{ color: 'var(--qt-text-secondary)' }} />
          </button>
        </div>

        {/* Content */}
        <div className="px-4 py-4">
          <p
            style={{
              color: 'var(--qt-text-primary)',
              fontSize: '13px',
              marginBottom: '12px',
            }}
          >
            {t('p2pError.failedToConnect')}
          </p>
          <div
            className="rounded p-3"
            style={{
              backgroundColor: 'var(--qt-bg-primary)',
              border: '1px solid var(--qt-border-color)',
            }}
          >
            <p
              style={{
                color: 'var(--qt-text-secondary)',
                fontSize: '12px',
                fontFamily: 'monospace',
                wordBreak: 'break-word',
              }}
            >
              {error}
            </p>
          </div>
          <p
            style={{
              color: 'var(--qt-text-secondary)',
              fontSize: '12px',
              marginTop: '12px',
            }}
          >
            {t('p2pError.checkConnection')}
          </p>
        </div>

        {/* Actions */}
        <div
          className="flex justify-end gap-2 px-4 py-3"
          style={{
            borderTop: '1px solid var(--qt-border-color)',
          }}
        >
          <button
            onClick={onDismiss}
            className="px-4 py-2 rounded"
            style={{
              backgroundColor: 'var(--qt-bg-primary)',
              border: '1px solid var(--qt-border-color)',
              color: 'var(--qt-text-primary)',
              fontSize: '13px',
            }}
          >
            {t('buttons.dismiss')}
          </button>
          <button
            onClick={onRetry}
            className="px-4 py-2 rounded flex items-center gap-2"
            style={{
              backgroundColor: 'var(--qt-accent-color, #3b82f6)',
              border: 'none',
              color: 'white',
              fontSize: '13px',
            }}
          >
            <RefreshCw size={14} />
            {t('buttons.retryConnection')}
          </button>
        </div>
      </div>
    </div>
  );
};

export default P2PErrorDialog;
