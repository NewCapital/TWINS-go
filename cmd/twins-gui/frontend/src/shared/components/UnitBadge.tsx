import React from 'react';
import { useDisplayUnits } from '@/shared/hooks/useDisplayUnits';

/**
 * UnitBadge — passive pill displaying the user's current display unit
 * (TWINS / mTWINS / µTWINS) sourced from the global useDisplayUnits hook.
 *
 * Chrome matches StatusPill's `neutral` tone:
 *   bg     rgba(136, 136, 136, 0.15)
 *   border 1px solid rgba(136, 136, 136, 0.4)
 *   radius 999px
 *   pad    2px 8px
 *   font   11px 500 #888
 *
 * CRITICAL: NO `textTransform: 'uppercase'`. CSS uppercase maps `µ` (U+00B5
 * MICRO SIGN) to Greek `Μ` (U+039C) which renders as Latin `M`, so `µTWINS`
 * would display as `MTWINS` — indistinguishable from milliTWINS. The label
 * from `useDisplayUnits().unitLabel` is already authored in correct case
 * (`TWINS` / `mTWINS` / `µTWINS`); render it verbatim. See the 2026-05-27
 * TransactionDetail round-2 fix entry in `cmd/twins-gui/frontend/CLAUDE.md`
 * for the original incident.
 */
export const UnitBadge: React.FC = () => {
  const { unitLabel } = useDisplayUnits();
  return (
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        backgroundColor: 'rgba(136, 136, 136, 0.15)',
        border: '1px solid rgba(136, 136, 136, 0.4)',
        borderRadius: '999px',
        padding: '2px 8px',
        fontSize: '11px',
        fontWeight: 500,
        color: '#888888',
        whiteSpace: 'nowrap',
      }}
    >
      {unitLabel}
    </span>
  );
};
