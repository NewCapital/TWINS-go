import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { DashboardCard } from '@/shared/components/DashboardCard';
import { StatusPill, StatusPillTone } from '@/shared/components/StatusPill';
import { useDisplayUnits } from '@/shared/hooks/useDisplayUnits';
import { useDisplayDateTime } from '@/shared/hooks/useDisplayDateTime';
import { convertToDisplayUnit } from '@/shared/utils/format';
import { core } from '@/shared/types/wallet.types';

interface SyncCardProps {
  blockchainInfo: core.BlockchainInfo | null;
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

// Two-cell row for unit-bearing values (Chain difficulty, Chain size on disk,
// Money supply): number + unit rendered as a content-sized inline cluster
// that the parent row's `justifyContent: space-between` pushes flush to the
// right edge of the card. No fixed slot widths — the cluster sizes to its
// content so the unit token (`M` / `GB` / `T µTWINS`) always ends at the card
// right edge with no dead space. Trade-off: the digit columns across the
// three rows do not form a strict vertical stack (each row's number-end
// position depends on its unit width), but the consistent right anchor reads
// cleaner than the prior gap-prone fixed-slot layout.
const unitRowRightStyle: React.CSSProperties = {
  display: 'flex',
  alignItems: 'baseline',
  gap: '6px',
};

const numberCellStyle: React.CSSProperties = {
  ...valueStyle,
};

const unitCellStyle: React.CSSProperties = {
  ...valueStyle,
  color: '#aaa',
};

const skeletonStyle: React.CSSProperties = {
  width: '80px',
  height: '14px',
};

// (Last block time helpers deleted — replaced by the global useDisplayDateTime hook.)

// Tuple form of the formatters: returns { number, unit } so the row JSX can
// render the two parts into separate cells (right-aligned number column +
// left-aligned unit column) instead of a single right-aligned joined string.
// `N/A` is returned with an empty unit so the unit slot collapses cleanly.
type NumberAndUnit = { number: string; unit: string };

const NA: NumberAndUnit = { number: 'N/A', unit: '' };

// Format byte count as B / KB / MB / GB tuple.
const formatBytes = (bytes: number): NumberAndUnit => {
  if (!bytes || bytes <= 0) return NA;
  if (bytes < 1024) return { number: `${bytes}`, unit: 'B' };
  if (bytes < 1024 * 1024) return { number: (bytes / 1024).toFixed(1), unit: 'KB' };
  if (bytes < 1024 * 1024 * 1024) return { number: (bytes / 1024 / 1024).toFixed(1), unit: 'MB' };
  return { number: (bytes / 1024 / 1024 / 1024).toFixed(2), unit: 'GB' };
};

// Compact-suffix table shared by formatDifficulty and formatCompactAmount.
const COMPACT_SUFFIXES: Array<{ v: number; s: string }> = [
  { v: 1e12, s: 'T' },
  { v: 1e9, s: 'B' },
  { v: 1e6, s: 'M' },
  { v: 1e3, s: 'K' },
];

// Format difficulty with a compact suffix for large values as a tuple,
// e.g. { number: "20.09", unit: "M" }. Values < 1000 fall through to the
// existing thousands-separator path with up to 3 decimals (and an empty
// unit slot); <= 0 renders as N/A.
const formatDifficulty = (diff: number): NumberAndUnit => {
  if (!diff || diff <= 0) return NA;
  if (diff < 1000) {
    return {
      number: new Intl.NumberFormat('en-US', { maximumFractionDigits: 3 }).format(diff),
      unit: '',
    };
  }
  for (const { v, s } of COMPACT_SUFFIXES) {
    if (diff >= v) {
      return { number: (diff / v).toFixed(2), unit: s };
    }
  }
  return {
    number: new Intl.NumberFormat('en-US', { maximumFractionDigits: 3 }).format(diff),
    unit: '',
  };
};

// Format an amount (already converted to the user's display unit) with a
// compact K/M/B/T suffix, returning a tuple so the row can render the
// number and the unit (e.g. "B TWINS") in separate cells. Caller must pre-
// convert the raw TWINS amount via convertToDisplayUnit so the compact
// scaling matches the user's selected unit (mTWINS / µTWINS shift the
// magnitude, which would otherwise mis-classify the suffix tier).
const formatCompactAmount = (convertedAmount: number, unitLabel: string): NumberAndUnit => {
  if (!convertedAmount || convertedAmount <= 0) return NA;
  if (convertedAmount < 1000) {
    return {
      number: new Intl.NumberFormat('en-US', { maximumFractionDigits: 2 }).format(convertedAmount),
      unit: unitLabel,
    };
  }
  for (const { v, s } of COMPACT_SUFFIXES) {
    if (convertedAmount >= v) {
      return { number: (convertedAmount / v).toFixed(2), unit: `${s} ${unitLabel}` };
    }
  }
  return {
    number: new Intl.NumberFormat('en-US', { maximumFractionDigits: 2 }).format(convertedAmount),
    unit: unitLabel,
  };
};

export const SyncCard: React.FC<SyncCardProps> = ({ blockchainInfo, networkInfo: _networkInfo, isLoading = false }) => {
  const { t } = useTranslation(['wallet', 'common']);
  const { formatAmount, displayUnit, unitLabel } = useDisplayUnits();

  // Tick once per second so the "Last block time" relative value updates between
  // status polls instead of staying frozen until the next 10s blockchainInfo refresh.
  const [, setTick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => setTick((n) => n + 1), 1000);
    return () => clearInterval(id);
  }, []);

  // Click-to-toggle state was removed — the global Status Bar dropdown owns
  // the date/age preference now and the Last block time row consumes it.

  const getSyncTone = (): { tone: StatusPillTone; label: string } => {
    if (!blockchainInfo) {
      return { tone: 'neutral', label: t('common:status.unknown') };
    }
    if (blockchainInfo.is_connecting) {
      return { tone: 'neutral', label: t('common:status.connecting') };
    }
    if (blockchainInfo.is_syncing) {
      return { tone: 'warning', label: t('common:status.syncing') };
    }
    if (blockchainInfo.is_out_of_sync) {
      return { tone: 'error', label: t('common:status.outOfSync') };
    }
    return { tone: 'success', label: t('common:status.upToDate') };
  };

  const syncStatus = getSyncTone();
  const currentBlock = blockchainInfo?.blocks ?? 0;
  const lastBlockTime = blockchainInfo?.last_block_time ?? 0;
  const difficulty = blockchainInfo?.difficulty ?? 0;
  const chainSizeBytes = blockchainInfo?.chain_size_bytes ?? 0;
  const moneySupply = blockchainInfo?.money_supply ?? 0;

  const headerRight = isLoading ? (
    <div className="loading-skeleton" style={skeletonStyle} />
  ) : (
    <StatusPill tone={syncStatus.tone} label={syncStatus.label} />
  );

  // The Last block time row consumes the global useDisplayDateTime hook so
  // the value reflects the user's current selection in the Status Bar
  // (Local / UTC / Age). Tooltip always shows UTC for unambiguous reference.
  const { formatDateTime, formatTooltip } = useDisplayDateTime();
  const lastBlockTimeValue = lastBlockTime > 0 ? formatDateTime(lastBlockTime) : 'N/A';
  const lastBlockTimeTooltip = lastBlockTime > 0 ? formatTooltip(lastBlockTime) : '';

  // Toggle-aware value style: pointer cursor + visual hover affordance.
  // Hover state is local to this row's value span; one re-render per
  // mouse-enter / mouse-leave is acceptable.
  const [timeHover, setTimeHover] = useState(false);
  const lastBlockTimeValueStyle: React.CSSProperties = {
    ...valueStyle,
    color: timeHover && lastBlockTime > 0 ? '#fff' : valueStyle.color,
    userSelect: 'none',
  };

  return (
    <DashboardCard title={t('wallet:sync.title')} headerRight={headerRight}>
      {isLoading ? (
        // Skeleton: 5 rows matching the post-load row count.
        <>
          {[0, 1, 2, 3, 4].map((i) => (
            <div key={i} style={rowMainStyle}>
              <div className="loading-skeleton" style={{ width: '90px', height: '12px' }} />
              <div className="loading-skeleton" style={{ width: '110px', height: '14px' }} />
            </div>
          ))}
        </>
      ) : (
        <>
          <div style={rowMainStyle}>
            <span style={labelStyle}>{t('wallet:sync.lastBlock')}</span>
            <span style={valueStyle}>
              {blockchainInfo ? currentBlock.toLocaleString() : 'N/A'}
            </span>
          </div>

          <div style={rowMainStyle}>
            <span style={labelStyle}>{t('wallet:sync.lastBlockTime')}</span>
            <span
              style={lastBlockTimeValueStyle}
              title={lastBlockTime > 0 ? lastBlockTimeTooltip : undefined}
              onMouseEnter={() => setTimeHover(true)}
              onMouseLeave={() => setTimeHover(false)}
            >
              {lastBlockTimeValue}
            </span>
          </div>

          {(() => {
            const diff = formatDifficulty(difficulty);
            const bytes = formatBytes(chainSizeBytes);
            const supply = formatCompactAmount(
              convertToDisplayUnit(moneySupply, displayUnit),
              unitLabel,
            );
            return (
              <>
                <div style={rowMainStyle}>
                  <span style={labelStyle}>{t('wallet:sync.chainDifficulty')}</span>
                  <div style={unitRowRightStyle}>
                    <span style={numberCellStyle}>{diff.number}</span>
                    <span style={unitCellStyle}>{diff.unit}</span>
                  </div>
                </div>

                <div style={rowMainStyle}>
                  <span style={labelStyle}>{t('wallet:sync.chainSizeOnDisk')}</span>
                  <div style={unitRowRightStyle}>
                    <span style={numberCellStyle}>{bytes.number}</span>
                    <span style={unitCellStyle}>{bytes.unit}</span>
                  </div>
                </div>

                <div style={rowMainStyle}>
                  <span style={labelStyle}>{t('wallet:sync.moneySupply')}</span>
                  <div
                    style={unitRowRightStyle}
                    title={moneySupply > 0 ? formatAmount(moneySupply) : undefined}
                  >
                    <span style={numberCellStyle}>{supply.number}</span>
                    <span style={unitCellStyle}>{supply.unit}</span>
                  </div>
                </div>
              </>
            );
          })()}
        </>
      )}
    </DashboardCard>
  );
};
