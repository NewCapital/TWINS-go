import React from 'react';
import { useTranslation } from 'react-i18next';
import { core } from '@/shared/types/wallet.types';
import { useDisplayUnits } from '@/shared/hooks/useDisplayUnits';
import { DashboardCard } from '@/shared/components/DashboardCard';

interface BalancesStripProps {
  balance: core.Balance;
  isLoading?: boolean;
}

const rowsContainerStyle: React.CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  gap: '6px',
};

const rowStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'baseline',
  justifyContent: 'space-between',
  gap: '12px',
};

const labelStyle: React.CSSProperties = {
  fontSize: '11px',
  color: '#888',
  fontWeight: 500,
};

const valueStyle: React.CSSProperties = {
  fontSize: '14px',
  color: '#ddd',
  fontFamily: 'monospace',
  textAlign: 'right',
  whiteSpace: 'nowrap',
};

// Total row: top divider via marginTop + paddingTop + borderTop. The green
// running-total signal is carried by the bold green value (#27ae60) alone —
// the prior 3px left-accent bar was dropped to restore visual symmetry with
// the Sync card, which has no equivalent emphasized row.
const totalRowStyle: React.CSSProperties = {
  ...rowStyle,
  marginTop: '4px',
  paddingTop: '8px',
  borderTop: '1px solid #3a3a3a',
};

const totalLabelStyle: React.CSSProperties = {
  ...labelStyle,
  fontSize: '12px',
  fontWeight: 600,
  color: '#ddd',
  textTransform: 'none',
  letterSpacing: 'normal',
};

const totalValueStyle: React.CSSProperties = {
  ...valueStyle,
  fontWeight: 600,
  color: '#27ae60',
};

interface RowProps {
  label: string;
  value: number;
  isLoading: boolean;
  isTotal?: boolean;
  tooltip?: string;
}

const Row: React.FC<RowProps> = ({ label, value, isLoading, isTotal, tooltip }) => {
  const { formatAmount } = useDisplayUnits();
  return (
    <div style={isTotal ? totalRowStyle : rowStyle}>
      <span style={isTotal ? totalLabelStyle : labelStyle} title={tooltip}>{label}</span>
      {isLoading ? (
        <div className="loading-skeleton" style={{ width: '100px', height: '14px' }} />
      ) : (
        // includeUnit=false — the unit is rendered once at the card header.
        <span style={isTotal ? totalValueStyle : valueStyle} title={formatAmount(value)}>
          {formatAmount(value, false)}
        </span>
      )}
    </div>
  );
};

export const BalancesStrip: React.FC<BalancesStripProps> = ({
  balance,
  isLoading = false,
}) => {
  const { t } = useTranslation('wallet');
  const { unitLabel } = useDisplayUnits();
  // All four supplemental rows (Available / Pending / Immature / Locked) always
  // render alongside Total so the user always sees the full breakdown of where
  // their coins are. Layout stays unit-stable across TWINS / mTWINS / µTWINS.
  return (
    <DashboardCard title={t('balance.title')} headerRight={unitLabel}>
      <div style={rowsContainerStyle}>
        <Row label={t('balance.available')} value={balance.available} isLoading={isLoading} />
        <Row label={t('balance.pending')} value={balance.pending} isLoading={isLoading} />
        <Row label={t('balance.immature')} value={balance.immature} isLoading={isLoading} />
        <Row label={t('balance.locked')} value={balance.locked} isLoading={isLoading} tooltip={t('balance.lockedTooltip')} />
        <Row label={t('balance.total')} value={balance.total} isLoading={isLoading} isTotal />
      </div>
    </DashboardCard>
  );
};
