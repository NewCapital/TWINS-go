import React from 'react';
import { useDisplayUnits } from '@/shared/hooks/useDisplayUnits';

interface BalanceRowProps {
  label: string;
  value: number;
  watchOnlyValue?: number | null;
  isBold?: boolean;
  isLoading?: boolean;
}

export const BalanceRow: React.FC<BalanceRowProps> = ({
  label,
  value,
  watchOnlyValue,
  isBold = false,
  isLoading = false
}) => {
  const { formatAmount } = useDisplayUnits();
  const showWatchOnly = watchOnlyValue !== null && watchOnlyValue !== undefined && watchOnlyValue > 0;

  return (
    <div className="qt-hbox" style={{ alignItems: 'baseline', marginTop: '2px' }}>
      <div className="qt-label" style={{ minWidth: '90px' }}>
        {label}
      </div>
      {isLoading ? (
        <div className="loading-skeleton" style={{ width: '120px', height: '14px' }} />
      ) : (
        <>
          <div className={`qt-balance-value ${isBold ? 'qt-balance-value-bold' : ''}`}>
            {formatAmount(value)}
          </div>
          {showWatchOnly && (
            <div className="qt-balance-watch-only" style={{ marginLeft: '12px', fontSize: '12px', color: '#888888' }}>
              ({formatAmount(watchOnlyValue!)})
            </div>
          )}
        </>
      )}
    </div>
  );
};
