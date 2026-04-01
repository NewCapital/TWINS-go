import React from 'react';
import { useTranslation } from 'react-i18next';
import { core } from '@/shared/types/wallet.types';

interface SyncStatusWidgetProps {
  blockchainInfo: core.BlockchainInfo | null;
  isLoading?: boolean;
}

/**
 * SyncStatusWidget displays blockchain synchronization status
 * matching the Qt wallet's sync status display
 */
export const SyncStatusWidget: React.FC<SyncStatusWidgetProps> = ({
  blockchainInfo,
  isLoading = false
}) => {
  const { t } = useTranslation(['wallet', 'common']);

  // Determine sync status text and color
  const getSyncStatus = () => {
    if (!blockchainInfo) {
      return { text: t('common:status.unknown'), color: '#888888' };
    }
    // Not enough peers for consensus - determined by backend (blockchain.MinSyncPeers)
    if (blockchainInfo.is_connecting) {
      return { text: t('common:status.connecting'), color: '#888888' };
    }
    if (blockchainInfo.is_syncing) {
      return { text: t('common:status.syncing'), color: '#ffaa00' };
    }
    if (blockchainInfo.is_out_of_sync) {
      return { text: t('common:status.outOfSync'), color: '#ff0000' };
    }
    return { text: t('common:status.upToDate'), color: '#00ff00' };
  };

  const syncStatus = getSyncStatus();
  const syncPercentage = blockchainInfo?.sync_percentage ?? 0;
  const behindTime = blockchainInfo?.behind_time ?? '';
  const currentBlock = blockchainInfo?.blocks ?? 0;

  return (
    <div className="qt-frame-secondary" style={{ padding: '0', marginBottom: '20px' }}>
      <div className="qt-hbox" style={{ alignItems: 'baseline', marginBottom: '10px' }}>
        <div className="qt-label" style={{ fontSize: '13px', fontWeight: 'normal' }}>
          {t('common:status.syncStatus')}
        </div>
      </div>

      <div style={{ marginTop: '10px' }}>
        {/* Sync Status */}
        <div className="qt-hbox" style={{ alignItems: 'baseline', marginBottom: '8px' }}>
          <div className="qt-label" style={{ minWidth: '80px', fontSize: '13px' }}>
            {t('common:status.status')}:
          </div>
          {isLoading ? (
            <div className="loading-skeleton" style={{ width: '100px', height: '14px' }} />
          ) : (
            <div style={{ fontSize: '13px', marginLeft: '20px', fontWeight: 'bold', color: syncStatus.color }}>
              {syncStatus.text}
            </div>
          )}
        </div>

        {/* Progress Bar (only show when syncing or out of sync) */}
        {blockchainInfo && (blockchainInfo.is_syncing || blockchainInfo.is_out_of_sync) && (
          <div style={{ marginBottom: '8px' }}>
            <div className="qt-hbox" style={{ alignItems: 'baseline', marginBottom: '4px' }}>
              <div className="qt-label" style={{ minWidth: '80px', fontSize: '13px' }}>
                {t('common:status.progress')}:
              </div>
              {isLoading ? (
                <div className="loading-skeleton" style={{ width: '60px', height: '14px' }} />
              ) : (
                <div style={{ fontSize: '13px', marginLeft: '20px' }}>
                  {syncPercentage.toFixed(2)}%
                </div>
              )}
            </div>
            {/* Progress bar */}
            <div style={{
              marginLeft: '100px',
              height: '8px',
              backgroundColor: '#333333',
              borderRadius: '4px',
              overflow: 'hidden'
            }}>
              <div style={{
                width: `${Math.min(syncPercentage, 100)}%`,
                height: '100%',
                backgroundColor: syncPercentage < 100 ? '#ffaa00' : '#00ff00',
                transition: 'width 0.3s ease'
              }} />
            </div>
          </div>
        )}

        {/* Behind Time (show when syncing or out of sync) */}
        {blockchainInfo && (blockchainInfo.is_syncing || blockchainInfo.is_out_of_sync) && behindTime && (
          <div className="qt-hbox" style={{ alignItems: 'baseline', marginBottom: '4px' }}>
            <div className="qt-label" style={{ minWidth: '80px', fontSize: '13px' }}>
              {t('common:status.behind')}:
            </div>
            {isLoading ? (
              <div className="loading-skeleton" style={{ width: '120px', height: '14px' }} />
            ) : (
              <div style={{ fontSize: '13px', marginLeft: '20px', color: '#ff0000' }}>
                {behindTime}
              </div>
            )}
          </div>
        )}

        {/* Current Block Height */}
        <div className="qt-hbox" style={{ alignItems: 'baseline', marginBottom: '4px' }}>
          <div className="qt-label" style={{ minWidth: '80px', fontSize: '13px' }}>
            {t('common:status.blocks')}:
          </div>
          {isLoading ? (
            <div className="loading-skeleton" style={{ width: '80px', height: '14px' }} />
          ) : (
            <div style={{ fontSize: '13px', marginLeft: '20px', fontWeight: 'bold' }}>
              {currentBlock.toLocaleString()}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
