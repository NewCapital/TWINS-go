import { forwardRef } from 'react';
import { useTranslation } from 'react-i18next';

export interface MasternodesContextMenuProps {
  visible: boolean;
  x: number;
  y: number;
  isStartingMasternode: boolean;
  onStartAlias: () => void;
}

export const MasternodesContextMenu = forwardRef<HTMLDivElement, MasternodesContextMenuProps>(
  ({ visible, x, y, isStartingMasternode, onStartAlias }, ref) => {
    const { t } = useTranslation('masternode');

    if (!visible) return null;

    // Defensive bounds checking for coordinates
    const safeX = Math.max(0, Math.min(x, window.innerWidth - 140));
    const safeY = Math.max(0, Math.min(y, window.innerHeight - 50));

    return (
      <div
        ref={ref}
        role="menu"
        aria-label="Masternode context menu"
        style={{
          position: 'fixed',
          top: safeY,
          left: safeX,
          backgroundColor: '#3a3a3a',
          border: '1px solid #5a5a5a',
          borderRadius: '2px',
          boxShadow: '2px 2px 8px rgba(0, 0, 0, 0.5)',
          zIndex: 1000,
          minWidth: '120px',
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <button
          onClick={onStartAlias}
          disabled={isStartingMasternode}
          style={{
            display: 'block',
            width: '100%',
            padding: '8px 12px',
            textAlign: 'left',
            backgroundColor: 'transparent',
            border: 'none',
            color: isStartingMasternode ? '#666' : '#ddd',
            fontSize: '12px',
            cursor: isStartingMasternode ? 'not-allowed' : 'pointer',
          }}
          onMouseEnter={(e) => {
            if (!isStartingMasternode) {
              e.currentTarget.style.backgroundColor = '#4a6a8a';
            }
          }}
          onMouseLeave={(e) => {
            e.currentTarget.style.backgroundColor = 'transparent';
          }}
        >
          {t('actions.startAlias')}
        </button>
      </div>
    );
  }
);

MasternodesContextMenu.displayName = 'MasternodesContextMenu';
