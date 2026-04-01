import React from 'react';
import { useTranslation } from 'react-i18next';
import { NetworkMasternodeFilters } from '@/shared/types/masternode.types';
import { RefreshCountdown } from '@/shared/components/RefreshCountdown';

// Status options based on masternode statuses from daemon (lowercase with hyphens)
// See internal/masternode/types.go MasternodeStatus.String() for backend values
const STATUS_OPTIONS = [
  { value: 'all', labelKey: 'network.filters.statusAll' },
  { value: 'enabled', labelKey: 'status.enabled' },
  { value: 'pre-enabled', labelKey: 'status.preEnabled' },
  { value: 'expired', labelKey: 'status.expired' },
  { value: 'outpoint-spent', labelKey: 'status.outpointSpent' },
  { value: 'removed', labelKey: 'status.removed' },
  { value: 'watchdog-expired', labelKey: 'status.watchdogExpired' },
  { value: 'pose-ban', labelKey: 'status.poseBan' },
  { value: 'inactive', labelKey: 'status.inactive' },
] as const;

// Tier options
const TIER_OPTIONS = [
  { value: 'all', labelKey: 'network.filters.tierAll' },
  { value: 'bronze', labelKey: 'tiers.bronze' },
  { value: 'silver', labelKey: 'tiers.silver' },
  { value: 'gold', labelKey: 'tiers.gold' },
  { value: 'platinum', labelKey: 'tiers.platinum' },
] as const;

export interface NetworkMasternodesFiltersProps {
  filters: NetworkMasternodeFilters;
  filteredCount: number;
  totalCount: number;
  countdown: number;
  countdownTotal: number;
  onFilterChange: (filters: Partial<NetworkMasternodeFilters>) => void;
  onRefresh: () => void;
  isLoading: boolean;
}

export const NetworkMasternodesFilters: React.FC<NetworkMasternodesFiltersProps> = ({
  filters,
  filteredCount,
  totalCount,
  countdown,
  countdownTotal,
  onFilterChange,
  onRefresh,
  isLoading,
}) => {
  const { t } = useTranslation('masternode');

  const selectStyle: React.CSSProperties = {
    padding: '4px 8px',
    fontSize: '11px',
    backgroundColor: '#2b2b2b',
    color: '#ddd',
    border: '1px solid #4a4a4a',
    borderRadius: '2px',
    cursor: 'pointer',
    minWidth: '100px',
  };

  const inputStyle: React.CSSProperties = {
    padding: '4px 8px',
    fontSize: '11px',
    backgroundColor: '#2b2b2b',
    color: '#ddd',
    border: '1px solid #4a4a4a',
    borderRadius: '2px',
    width: '200px',
  };

  return (
    <div style={{
      display: 'flex',
      flexDirection: 'column',
      gap: '8px',
      marginBottom: '8px',
    }}>
      {/* Filters Row */}
      <div style={{
        display: 'flex',
        alignItems: 'center',
        gap: '12px',
        flexWrap: 'wrap',
      }}>
        {/* Tier Filter */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          <label style={{ fontSize: '11px', color: '#999' }}>
            {t('network.filters.tier')}:
          </label>
          <select
            value={filters.tier}
            onChange={(e) => onFilterChange({ tier: e.target.value as any })}
            style={selectStyle}
          >
            {TIER_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {t(option.labelKey)}
              </option>
            ))}
          </select>
        </div>

        {/* Status Filter */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          <label style={{ fontSize: '11px', color: '#999' }}>
            {t('network.filters.status')}:
          </label>
          <select
            value={filters.status}
            onChange={(e) => onFilterChange({ status: e.target.value as any })}
            style={selectStyle}
          >
            {STATUS_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {t(option.labelKey)}
              </option>
            ))}
          </select>
        </div>

        {/* Search Input */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          <label style={{ fontSize: '11px', color: '#999' }}>
            {t('network.filters.search')}:
          </label>
          <input
            type="text"
            value={filters.search}
            onChange={(e) => onFilterChange({ search: e.target.value })}
            placeholder={t('network.filters.searchPlaceholder')}
            style={inputStyle}
          />
        </div>

        {/* Clear Filters Button */}
        {(filters.tier !== 'all' || filters.status !== 'all' || filters.search) && (
          <button
            onClick={() => onFilterChange({ tier: 'all', status: 'all', search: '' })}
            style={{
              padding: '4px 8px',
              fontSize: '11px',
              backgroundColor: '#3a3a3a',
              color: '#ddd',
              border: '1px solid #4a4a4a',
              borderRadius: '2px',
              cursor: 'pointer',
            }}
          >
            {t('network.filters.clear')}
          </button>
        )}
      </div>

      {/* Status Row */}
      <div style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
      }}>
        {/* Count Display */}
        <div style={{ fontSize: '11px', color: '#999' }}>
          {filteredCount === totalCount ? (
            t('network.count.total', { count: totalCount })
          ) : (
            t('network.count.filtered', { filtered: filteredCount, total: totalCount })
          )}
        </div>

        {/* Refresh Countdown Ring */}
        <RefreshCountdown
          countdown={countdown}
          total={countdownTotal}
          mode="interactive"
          onRefresh={onRefresh}
          isLoading={isLoading}
        />
      </div>
    </div>
  );
};
