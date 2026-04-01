import React from 'react';
import { useTranslation } from 'react-i18next';
import { core } from '@/shared/types/wallet.types';

interface StakingStatusWidgetProps {
  stakingInfo: core.StakingInfo | null;
  isLoading?: boolean;
}

/**
 * StakingStatusWidget displays staking status and statistics
 * matching the Qt wallet's staking status display
 */
export const StakingStatusWidget: React.FC<StakingStatusWidgetProps> = ({
  stakingInfo,
  isLoading = false
}) => {
  const { t } = useTranslation(['wallet', 'common']);

  const stakingEnabled = stakingInfo?.enabled ?? false;
  const isStaking = stakingInfo?.staking ?? false;
  const walletUnlocked = stakingInfo?.walletunlocked ?? false;
  const expectedStakeTime = stakingInfo?.expectedstaketime ?? 0;

  const formatExpectedTime = (seconds: number): string => {
    if (seconds <= 0) return 'N/A';
    if (seconds < 120) return '~1 minute';
    if (seconds < 3600) return `~${Math.round(seconds / 60)} minutes`;
    const hours = Math.round(seconds / 3600);
    if (seconds < 86400) return `~${hours} hour${hours !== 1 ? 's' : ''}`;
    const days = Math.round(seconds / 86400);
    if (seconds < 86400 * 365) return `~${days} day${days !== 1 ? 's' : ''}`;
    const years = Math.round(seconds / (86400 * 365));
    return `~${years} year${years !== 1 ? 's' : ''}`;
  };

  const formattedExpectedTime = formatExpectedTime(expectedStakeTime);

  // Determine staking status text and color
  const getStakingStatus = () => {
    if (!stakingEnabled) {
      return { text: t('common:staking.disabled'), color: '#888888' };
    }
    if (isStaking) {
      return { text: t('common:staking.active'), color: '#00ff00' };
    }
    if (!walletUnlocked) {
      return { text: t('common:staking.walletLocked'), color: '#ff6666' };
    }
    return { text: t('common:staking.enabled'), color: '#ffaa00' };
  };

  const status = getStakingStatus();

  return (
    <div className="qt-frame-secondary" style={{ padding: '0', marginBottom: '20px' }}>
      <div className="qt-hbox" style={{ alignItems: 'baseline', marginBottom: '10px' }}>
        <div className="qt-label" style={{ fontSize: '13px', fontWeight: 'normal' }}>
          {t('common:staking.title')}
        </div>
      </div>

      <div style={{ marginTop: '10px' }}>
        {/* Staking Status */}
        <div className="qt-hbox" style={{ alignItems: 'baseline', marginBottom: '4px' }}>
          <div className="qt-label" style={{ minWidth: '100px', fontSize: '13px' }}>
            {t('common:status.status')}:
          </div>
          {isLoading ? (
            <div className="loading-skeleton" style={{ width: '80px', height: '14px' }} />
          ) : (
            <div style={{
              fontSize: '13px',
              marginLeft: '20px',
              fontWeight: 'bold',
              color: status.color
            }}>
              {status.text}
            </div>
          )}
        </div>

        {/* Expected Stake Time */}
        <div className="qt-hbox" style={{ alignItems: 'baseline', marginBottom: '4px' }}>
          <div className="qt-label" style={{ minWidth: '100px', fontSize: '13px' }}>
            {t('common:staking.expectedTime')}:
          </div>
          {isLoading ? (
            <div className="loading-skeleton" style={{ width: '80px', height: '14px' }} />
          ) : (
            <div style={{ fontSize: '13px', marginLeft: '20px', color: (!stakingEnabled || !walletUnlocked) ? '#888888' : undefined }}>
              {formattedExpectedTime}
            </div>
          )}
        </div>

      </div>
    </div>
  );
};
