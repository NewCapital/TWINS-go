import React from 'react';
import { useTranslation } from 'react-i18next';
import { core } from '@/shared/types/wallet.types';
import { useDisplayUnits } from '@/shared/hooks/useDisplayUnits';

interface TWINSBalanceCardProps {
  balance: core.Balance;
  showWatchOnly?: boolean;
  hideZeroBalances?: boolean;
  isLoading?: boolean;
}

// Balance row component for consistent styling
const BalanceRow: React.FC<{
  label: string;
  value: number;
  isLoading: boolean;
  hideIfZero?: boolean;
}> = ({ label, value, isLoading, hideIfZero = false }) => {
  const { formatAmount } = useDisplayUnits();

  // Hide row if value is zero and hideIfZero is true
  if (hideIfZero && value === 0) {
    return null;
  }

  return (
    <div className="qt-hbox" style={{ alignItems: 'baseline', marginBottom: '4px' }}>
      <div className="qt-label" style={{ minWidth: '80px', fontSize: '13px' }}>{label}:</div>
      {isLoading ? (
        <div className="loading-skeleton" style={{ width: '150px', height: '14px' }} />
      ) : (
        <div className="qt-balance-value" style={{ fontSize: '13px', marginLeft: '20px', fontWeight: 'bold' }}>
          {formatAmount(value)}
        </div>
      )}
    </div>
  );
};

export const TWINSBalanceCard: React.FC<TWINSBalanceCardProps> = ({
  balance,
  hideZeroBalances = false,
  isLoading = false
}) => {
  const { t } = useTranslation('wallet');

  return (
    <div className="qt-frame-secondary" style={{ padding: '0', marginBottom: '20px', marginTop: '15px' }}>
      <div className="qt-hbox" style={{ alignItems: 'baseline', marginBottom: '10px' }}>
        <div className="qt-label" style={{ fontSize: '13px', fontWeight: 'normal' }}>
          {t('balance.twins')}
        </div>
      </div>

      <div style={{ marginTop: '10px' }}>
        {/* Available balance - spendable funds */}
        <BalanceRow
          label={t('balance.available')}
          value={balance.available}
          isLoading={isLoading}
        />

        {/* Pending balance - unconfirmed transactions */}
        <BalanceRow
          label={t('balance.pending')}
          value={balance.pending}
          isLoading={isLoading}
          hideIfZero={hideZeroBalances}
        />

        {/* Immature balance - staking rewards < 120 confirmations */}
        <BalanceRow
          label={t('balance.immature')}
          value={balance.immature}
          isLoading={isLoading}
          hideIfZero={hideZeroBalances}
        />

        {/* Locked balance - masternode collateral */}
        <BalanceRow
          label={t('balance.locked')}
          value={balance.locked}
          isLoading={isLoading}
          hideIfZero={hideZeroBalances}
        />

        {/* Total balance */}
        <BalanceRow
          label={t('balance.total')}
          value={balance.total}
          isLoading={isLoading}
        />
      </div>
    </div>
  );
};
