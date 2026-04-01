import React from 'react';
import { useTranslation } from 'react-i18next';
import { core } from '@/shared/types/wallet.types';

interface NetworkStatusWidgetProps {
  networkInfo: core.NetworkInfo | null;
  isLoading?: boolean;
}

/**
 * NetworkStatusWidget displays network connectivity status
 * matching the Qt wallet's network status display
 */
export const NetworkStatusWidget: React.FC<NetworkStatusWidgetProps> = ({
  networkInfo,
  isLoading = false
}) => {
  const { t } = useTranslation(['wallet', 'common']);

  const connections = networkInfo?.connections ?? 0;
  const networkActive = networkInfo?.networkactive ?? false;

  // Determine network status color based on connection count
  const getConnectionColor = () => {
    if (!networkActive) return '#ff0000';
    if (connections === 0) return '#ff0000';
    if (connections < 3) return '#ffaa00';
    return '#00ff00';
  };

  return (
    <div className="qt-frame-secondary" style={{ padding: '0', marginBottom: '20px' }}>
      <div className="qt-hbox" style={{ alignItems: 'baseline', marginBottom: '10px' }}>
        <div className="qt-label" style={{ fontSize: '13px', fontWeight: 'normal' }}>
          {t('common:network.title')}
        </div>
      </div>

      <div style={{ marginTop: '10px' }}>
        {/* Network Active Status */}
        <div className="qt-hbox" style={{ alignItems: 'baseline', marginBottom: '4px' }}>
          <div className="qt-label" style={{ minWidth: '80px', fontSize: '13px' }}>
            {t('common:network.status')}:
          </div>
          {isLoading ? (
            <div className="loading-skeleton" style={{ width: '80px', height: '14px' }} />
          ) : (
            <div style={{
              fontSize: '13px',
              marginLeft: '20px',
              fontWeight: 'bold',
              color: networkActive ? '#00ff00' : '#ff0000'
            }}>
              {networkActive ? t('common:network.active') : t('common:network.inactive')}
            </div>
          )}
        </div>

        {/* Peer Count */}
        <div className="qt-hbox" style={{ alignItems: 'baseline', marginBottom: '4px' }}>
          <div className="qt-label" style={{ minWidth: '80px', fontSize: '13px' }}>
            {t('common:network.connections')}:
          </div>
          {isLoading ? (
            <div className="loading-skeleton" style={{ width: '40px', height: '14px' }} />
          ) : (
            <div style={{
              fontSize: '13px',
              marginLeft: '20px',
              fontWeight: 'bold',
              color: getConnectionColor()
            }}>
              {connections}
            </div>
          )}
        </div>

        {/* Network Block Height (consensus height from peers) */}
        <div className="qt-hbox" style={{ alignItems: 'baseline', marginBottom: '4px' }}>
          <div className="qt-label" style={{ minWidth: '80px', fontSize: '13px' }}>
            {t('common:network.networkHeight')}:
          </div>
          {isLoading ? (
            <div className="loading-skeleton" style={{ width: '80px', height: '14px' }} />
          ) : (
            <div style={{ fontSize: '13px', marginLeft: '20px', fontWeight: 'bold' }}>
              {(networkInfo?.network_height ?? 0) > 0
                ? (networkInfo!.network_height).toLocaleString()
                : 'N/A'}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
