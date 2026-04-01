import React from 'react';
import { useTranslation } from 'react-i18next';
import { core } from '@/shared/types/wallet.types';
import { useDisplayUnits } from '@/shared/hooks/useDisplayUnits';

interface CombinedBalanceCardProps {
  balance: core.Balance;
  isOutOfSync: boolean;
  isLoading?: boolean;
}

export const CombinedBalanceCard: React.FC<CombinedBalanceCardProps> = ({
  balance,
  isOutOfSync,
  isLoading = false
}) => {
  const { t } = useTranslation('wallet');
  const { formatAmount } = useDisplayUnits();

  return (
    <div className="qt-frame-secondary" style={{ padding: '0', marginBottom: '20px' }}>
      <div className="qt-hbox" style={{ alignItems: 'baseline', marginBottom: '8px' }}>
        <div className="qt-label" style={{ fontSize: '13px', fontWeight: 'normal' }}>
          {t('balance.combined')}
        </div>
        {isOutOfSync && (
          <span className="qt-out-of-sync" style={{ marginLeft: '10px', fontSize: '12px', color: '#ff0000' }}>
            {t('common:status.outOfSync')}
          </span>
        )}
      </div>

      {/* Horizontal line separator */}
      <div style={{ height: '1px', backgroundColor: '#555555', marginBottom: '10px' }} />

      <div className="qt-hbox" style={{ alignItems: 'baseline' }}>
        <div className="qt-label" style={{ minWidth: '60px', fontSize: '13px' }}>{t('balance.total')}:</div>
        {isLoading ? (
          <div className="loading-skeleton" style={{ width: '150px', height: '14px' }} />
        ) : (
          <div className="qt-balance-value" style={{ fontSize: '13px', marginLeft: '20px', fontWeight: 'bold' }}>
            {formatAmount(balance.total)}
          </div>
        )}
      </div>
    </div>
  );
};
