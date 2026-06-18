import React from 'react';
import { useTranslation } from 'react-i18next';
import { DashboardCard } from '@/shared/components/DashboardCard';
import { StatusPill, StatusPillTone } from '@/shared/components/StatusPill';
import { core } from '@/shared/types/wallet.types';

interface StakingCardProps {
  stakingInfo: core.StakingInfo | null;
  isLoading?: boolean;
}

const rowMainStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  gap: '8px',
  flexWrap: 'wrap',
  justifyContent: 'space-between',
};

const labelStyle: React.CSSProperties = {
  fontSize: '11px',
  color: '#888',
  fontWeight: 500,
  minWidth: '60px',
};

const valueStyle: React.CSSProperties = {
  fontSize: '14px',
  color: '#ddd',
  fontFamily: 'monospace',
};

const valueDimStyle: React.CSSProperties = {
  fontSize: '14px',
  color: '#888',
  fontFamily: 'monospace',
};

const skeletonStyle: React.CSSProperties = {
  width: '80px',
  height: '14px',
};

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

// Reserve balance is sourced from wallet.GetReserveBalance() (satoshis / 1e8),
// so it can legitimately carry up to 8 decimal places. Pad to a minimum of 2
// for the common whole-coin case ("1,000.00") but preserve full satoshi
// precision when configured ("0.12345678").
const formatReserveBalance = (twins: number): string =>
  twins.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 8 });

export const StakingCard: React.FC<StakingCardProps> = ({ stakingInfo, isLoading = false }) => {
  const { t } = useTranslation(['wallet', 'common']);

  const stakingEnabled = stakingInfo?.enabled ?? false;
  const isStaking = stakingInfo?.staking ?? false;
  const walletUnlocked = stakingInfo?.walletunlocked ?? false;
  const expectedStakeTime = stakingInfo?.expectedstaketime ?? 0;
  const reserveBalance = stakingInfo?.reserve_balance ?? 0;

  const getStakingTone = (): { tone: StatusPillTone; label: string } => {
    if (!stakingEnabled) {
      return { tone: 'neutral', label: t('common:staking.disabled') };
    }
    if (isStaking) {
      return { tone: 'success', label: t('common:staking.active') };
    }
    if (!walletUnlocked) {
      return { tone: 'error', label: t('common:staking.walletLocked') };
    }
    return { tone: 'warning', label: t('common:staking.enabled') };
  };
  const stakingStatus = getStakingTone();

  // Est. Time row is dimmed when the wallet can't actually produce stakes
  // (staking disabled or wallet locked) — value is informational-only.
  const expectedTimeDimmed = !stakingEnabled || !walletUnlocked;

  const headerRight = isLoading ? (
    <div className="loading-skeleton" style={skeletonStyle} />
  ) : (
    <StatusPill tone={stakingStatus.tone} label={stakingStatus.label} />
  );

  return (
    <DashboardCard title={t('wallet:staking.title')} headerRight={headerRight}>
      {!isLoading && (
        <div style={rowMainStyle}>
          <span style={labelStyle}>{t('wallet:staking.reserveBalance')}</span>
          <span style={valueStyle}>{formatReserveBalance(reserveBalance)}</span>
        </div>
      )}

      {!isLoading && (
        <div style={rowMainStyle}>
          <span style={labelStyle}>{t('common:staking.expectedTime')}</span>
          <span style={expectedTimeDimmed ? valueDimStyle : valueStyle}>
            {/* Render N/A explicitly when staking is disabled rather than
                showing a real-looking dimmed estimate that is misleading
                because no stake will be produced until staking is re-enabled. */}
            {stakingEnabled ? formatExpectedTime(expectedStakeTime) : 'N/A'}
          </span>
        </div>
      )}
    </DashboardCard>
  );
};
