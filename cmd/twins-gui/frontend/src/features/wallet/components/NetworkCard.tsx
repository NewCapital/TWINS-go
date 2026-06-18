import React from 'react';
import { useTranslation } from 'react-i18next';
import { DashboardCard } from '@/shared/components/DashboardCard';
import { StatusPill, StatusPillTone } from '@/shared/components/StatusPill';
import { core } from '@/shared/types/wallet.types';

interface NetworkCardProps {
  networkInfo: core.NetworkInfo | null;
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

const skeletonStyle: React.CSSProperties = {
  width: '80px',
  height: '14px',
};

export const NetworkCard: React.FC<NetworkCardProps> = ({ networkInfo, isLoading = false }) => {
  const { t } = useTranslation(['wallet', 'common']);

  const connections = networkInfo?.connections ?? 0;
  const networkActive = networkInfo?.networkactive ?? false;
  const inbound = networkInfo?.inbound_peers ?? 0;
  const outbound = networkInfo?.outbound_peers ?? 0;
  const networkHeight = networkInfo?.network_height ?? 0;
  const networkHeightLabel = networkHeight > 0 ? networkHeight.toLocaleString() : 'N/A';

  const networkStatus: { tone: StatusPillTone; label: string } = !networkActive
    ? { tone: 'error', label: t('common:network.inactive') }
    : { tone: 'success', label: t('common:network.active') };

  const headerRight = isLoading ? (
    <div className="loading-skeleton" style={skeletonStyle} />
  ) : (
    <StatusPill tone={networkStatus.tone} label={networkStatus.label} />
  );

  return (
    <DashboardCard title={t('wallet:network.title')} headerRight={headerRight}>
      {!isLoading && (
        <div style={rowMainStyle}>
          <span style={labelStyle}>{t('wallet:network.connectionsInOut')}</span>
          <span style={valueStyle}>{`${connections} (${inbound}/${outbound})`}</span>
        </div>
      )}

      {!isLoading && (
        <div style={rowMainStyle}>
          <span style={labelStyle}>{t('wallet:network.networkHeight')}</span>
          <span style={valueStyle}>{networkHeightLabel}</span>
        </div>
      )}
    </DashboardCard>
  );
};
