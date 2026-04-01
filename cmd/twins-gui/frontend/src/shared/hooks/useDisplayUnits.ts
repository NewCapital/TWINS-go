import { useCallback } from 'react';
import { useStore } from '@/store/useStore';
import { convertToDisplayUnit, getUnitLabel, formatBalance } from '@/shared/utils/format';

/**
 * Hook providing display unit settings and a unit-aware amount formatter.
 *
 * Usage:
 *   const { formatAmount, unitLabel } = useDisplayUnits();
 *   formatAmount(1.5)          // "1,500.00000 mTWINS" (if unit=mTWINS, digits=5)
 *   formatAmount(1.5, false)   // "1,500.00000"
 */
export function useDisplayUnits() {
  const displayUnit = useStore(state => state.displayUnit);
  const displayDigits = useStore(state => state.displayDigits);
  const unitLabel = getUnitLabel(displayUnit);

  const formatAmount = useCallback(
    (amount: number, includeUnit: boolean = true): string => {
      const converted = convertToDisplayUnit(amount, displayUnit);
      const formatted = formatBalance(converted, displayDigits, false);
      return includeUnit ? `${formatted} ${unitLabel}` : formatted;
    },
    [displayUnit, displayDigits, unitLabel]
  );

  return { displayUnit, displayDigits, unitLabel, formatAmount };
}
