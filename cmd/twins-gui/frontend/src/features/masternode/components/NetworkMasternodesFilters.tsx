import React, { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Search, X } from 'lucide-react';
import { NetworkMasternodeFilters } from '@/shared/types/masternode.types';
import { RowsPerPageSelect } from '@/shared/components/RowsPerPageSelect';
import { IconButton } from '@/shared/components/IconButton';

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

// Receive design-language card chrome — wraps the entire filter row, bringing
// it in line with the TxFilterBar card on Transactions.tsx:1043-1052. The
// previously-naked flex row was visually weak next to the rest of the wallet.
// See l-network-masternodes-filters-card-wrap-and-search-icon (2026-06-12).
const cardStyle: React.CSSProperties = {
  backgroundColor: '#2f2f2f',
  border: '1px solid #3a3a3a',
  borderRadius: '8px',
  padding: '12px 16px',
};

// Receive input chrome for the Search field. The left padding (36px) clears
// the inline Search icon positioned at `left: 10px` (icon width 14px +
// breathing room). The right padding is overridden inline per render: 36px
// when the conditional clear-X is visible (which sits at right: 4px with a
// 24px IconButton, leaving an ~8px gap to the text), else the canonical 10px.
// The 200px fixed width is preserved verbatim from the prior implementation.
// `outline: 'none'` is intentionally OMITTED — preserves the UA default
// keyboard-focus ring for WCAG 2.4.7 (Focus Visible). Documented convention
// per `m-restyle-transactions-filter-bar` (MR !643): the canonical Explorer
// SearchBar.tsx exception uses `outline: 'none'` because it carries its own
// `aria-invalid` border-color flip; this filter input has no such flip, so
// we follow the broader Receive-input convention to keep keyboard users from
// losing the focus indicator.
const inputStyle: React.CSSProperties = {
  padding: '7px 10px 7px 36px',
  fontSize: '12px',
  backgroundColor: '#252525',
  color: '#ddd',
  border: '1px solid #3a3a3a',
  borderRadius: '4px',
  width: '200px',
};

export interface NetworkMasternodesFiltersProps {
  filters: NetworkMasternodeFilters;
  filteredCount: number;
  totalCount: number;
  onFilterChange: (filters: Partial<NetworkMasternodeFilters>) => void;
}

export const NetworkMasternodesFilters: React.FC<NetworkMasternodesFiltersProps> = ({
  filters,
  filteredCount,
  totalCount,
  onFilterChange,
}) => {
  const { t } = useTranslation('masternode');

  // Compose translated labels + reverse maps for Tier and Status dropdowns.
  // RowsPerPageSelect renders the option string AS the visible label, so non-
  // numeric consumers compose translated label arrays and reverse-map back to
  // value tokens. Established codebase pattern: MasternodeEditDialog.tsx
  // (m-redesign-masternode-edit-dialog 2026-06-11) Collateral UTXO field,
  // Receive.tsx (l-receive-form-select-new-address-and-header-padding
  // 2026-06-03) amount unit picker. Math.max(0, findIndex(...)) defends
  // against a transiently-unknown filter value by falling back to the first
  // option (which is "all" in both arrays).
  const tierLabels = useMemo(
    () => TIER_OPTIONS.map((opt) => t(opt.labelKey)),
    [t]
  );
  const tierLabelToValue = useMemo(
    () => new Map(TIER_OPTIONS.map((opt, i) => [tierLabels[i], opt.value])),
    [tierLabels]
  );
  const currentTierLabel =
    tierLabels[
      Math.max(
        0,
        TIER_OPTIONS.findIndex((opt) => opt.value === filters.tier)
      )
    ];

  const statusLabels = useMemo(
    () => STATUS_OPTIONS.map((opt) => t(opt.labelKey)),
    [t]
  );
  const statusLabelToValue = useMemo(
    () => new Map(STATUS_OPTIONS.map((opt, i) => [statusLabels[i], opt.value])),
    [statusLabels]
  );
  const currentStatusLabel =
    statusLabels[
      Math.max(
        0,
        STATUS_OPTIONS.findIndex((opt) => opt.value === filters.status)
      )
    ];

  const hasSearchText = filters.search.length > 0;

  // Outer Receive card wraps a single flex row:
  //   Tier | Status | Search (with icon + conditional clear-X) | Clear (conditional) | Count.
  // The 3 inline visible labels (`Tier:`, `Status:`, `Search:`) were removed
  // by this task — selects already display their selected value, and the
  // Search input has both a placeholder and an `aria-label` for screen
  // readers. The counter floats to the right edge via `marginLeft: 'auto'`;
  // right-anchor stays stable regardless of whether the conditional Clear
  // button is rendered. See l-network-masternodes-filters-counter-and-selects
  // (2026-06-12) for the counter relocation and
  // l-network-masternodes-filters-card-wrap-and-search-icon (2026-06-12) for
  // the card wrap + search-icon polish.
  return (
    <div style={cardStyle}>
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '12px',
          flexWrap: 'wrap',
        }}
      >
        {/* Tier Filter */}
        <RowsPerPageSelect<string>
          value={currentTierLabel}
          options={tierLabels}
          onChange={(label) => {
            const next = tierLabelToValue.get(label);
            if (next) onFilterChange({ tier: next as NetworkMasternodeFilters['tier'] });
          }}
          ariaLabel={`${t('network.filters.tier')}: ${currentTierLabel}`}
          align="left"
          triggerStyle={{ minWidth: '110px', padding: '7px 10px' }}
        />

        {/* Status Filter */}
        <RowsPerPageSelect<string>
          value={currentStatusLabel}
          options={statusLabels}
          onChange={(label) => {
            const next = statusLabelToValue.get(label);
            if (next) onFilterChange({ status: next as NetworkMasternodeFilters['status'] });
          }}
          ariaLabel={`${t('network.filters.status')}: ${currentStatusLabel}`}
          align="left"
          triggerStyle={{ minWidth: '140px', padding: '7px 10px' }}
        />

        {/* Search Input — wrapped in a position:relative container hosting the
            leading Search icon (always) and the trailing clear-X IconButton
            (conditional on non-empty search text). Padding-right flips to
            36px when clear-X is visible so the text does not slide under the
            button. Canonical pattern mirrors features/explorer/components/SearchBar.tsx.
            The clear-X is a bare IconButton — no `<span onMouseDown preventDefault>`
            wrapper is needed because this input has no popover/dropdown whose
            blur lifecycle would care about the brief focus transition. */}
        <div style={{ position: 'relative', display: 'inline-flex', alignItems: 'center' }}>
          <input
            type="text"
            value={filters.search}
            onChange={(e) => onFilterChange({ search: e.target.value })}
            placeholder={t('network.filters.searchPlaceholder')}
            aria-label={t('network.filters.search')}
            style={{
              ...inputStyle,
              paddingRight: hasSearchText ? '36px' : '10px',
            }}
          />
          <Search
            size={14}
            color="#888"
            style={{
              position: 'absolute',
              left: '10px',
              top: '50%',
              transform: 'translateY(-50%)',
              pointerEvents: 'none',
            }}
          />
          {hasSearchText && (
            <div
              style={{
                position: 'absolute',
                right: '4px',
                top: '50%',
                transform: 'translateY(-50%)',
              }}
            >
              <IconButton
                onClick={() => onFilterChange({ search: '' })}
                title={t('network.filters.clearSearch')}
                ariaLabel={t('network.filters.clearSearch')}
                icon={<X size={12} />}
              />
            </div>
          )}
        </div>

        {/* Clear Filters Button */}
        {(filters.tier !== 'all' || filters.status !== 'all' || filters.search) && (
          <button
            onClick={() => onFilterChange({ tier: 'all', status: 'all', search: '' })}
            style={{
              padding: '8px 16px',
              fontSize: '12px',
              backgroundColor: '#383838',
              color: '#ccc',
              border: '1px solid #4a4a4a',
              borderRadius: '6px',
              cursor: 'pointer',
            }}
          >
            {t('network.filters.clear')}
          </button>
        )}

        {/* Count Display — relocated INTO the filter row per
            l-network-masternodes-filters-counter-and-selects (2026-06-12).
            `marginLeft: 'auto'` floats it to the right edge of the card so
            (a) the previously-wasted horizontal space right of Search is filled,
            (b) the standalone counter row that previously sat below the filters
            is gone. Right-anchor stays stable whether or not the conditional
            Clear button is rendered. */}
        <div style={{ fontSize: '12px', color: '#ddd', marginLeft: 'auto' }}>
          {filteredCount === totalCount
            ? t('network.count.total', { count: totalCount })
            : t('network.count.filtered', { filtered: filteredCount, total: totalCount })}
        </div>
      </div>
    </div>
  );
};
