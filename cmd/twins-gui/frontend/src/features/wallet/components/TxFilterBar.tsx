import React, { useMemo, useRef, useState, useCallback, useEffect } from 'react';
import { Plus, X, Search, Eye, EyeOff, Info } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useTransactions } from '@/store/useStore';
import type { DateFilter, TypeFilter } from '@/store/slices/transactionsSlice';
import { isBarePositiveNumber, parseSearchQuery } from '@/shared/utils/parseSearchQuery';
import { Banner } from '@/shared/components/Banner';
import { TxFilterPopover } from './TxFilterPopover';
import { TxFilterDateEditor } from './TxFilterDateEditor';
import { TxFilterTypeEditor } from './TxFilterTypeEditor';
import { TxFilterAmountEditor } from './TxFilterAmountEditor';
import { TxViewsMenu } from './TxViewsMenu';

type FilterKind = 'date' | 'type' | 'amount';

// TWINS address regex: 'W'/'a'/'m'/'n' prefix + 33 Base58 chars. Mirrors the
// canonical regex in parseSearchQuery.ts (which is the source of truth for
// detection); used here only for chip label formatting so the chip says
// "Address: W..." instead of "Search: W..." when an address is in searchText.
const TWINS_ADDRESS_REGEX = /^[Wamn][a-km-zA-HJ-NP-Z1-9]{33}$/;

// i18n key suffixes for date presets (under transactions.filters.chip.datePresets).
const DATE_PRESET_I18N_KEY: Record<Exclude<DateFilter, 'range'>, string> = {
  all: 'allTime',
  today: 'today',
  week: 'thisWeek',
  month: 'thisMonth',
  lastMonth: 'lastMonth',
  year: 'thisYear',
};

// Type filter -> human label. Matches the editor's TYPE_OPTIONS list and the
// previous typeOptions hardcoded labels (never i18n in the original
// implementation, parity preserved).
//
// Phase 3 multi-select: 'all' and 'mostCommon' rows are gone — empty slice IS
// the "all" state, and 'mostCommon' was a single-select-era grouping.
const TYPE_LABELS: Record<TypeFilter, string> = {
  received: 'Received',
  sent: 'Sent',
  toYourself: 'To yourself',
  mined: 'Mined',
  minted: 'Minted',
  masternode: 'Masternode Reward',
  consolidation: 'UTXO Consolidation',
  other: 'Other',
};

const pillButton: React.CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  gap: '6px',
  padding: '6px 12px',
  fontSize: '12px',
  fontWeight: 500,
  color: '#ccc',
  backgroundColor: '#383838',
  border: '1px solid #4a4a4a',
  borderRadius: '999px',
  cursor: 'pointer',
  transition: 'background-color 0.15s, border-color 0.15s',
};

const inputStyle: React.CSSProperties = {
  backgroundColor: '#252525',
  border: '1px solid #3a3a3a',
  borderRadius: '4px',
  padding: '7px 10px 7px 28px',
  fontSize: '12px',
  color: '#ddd',
  outline: 'none',
  width: '100%',
};

const chipStyle: React.CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  gap: '6px',
  padding: '4px 4px 4px 10px',
  fontSize: '11px',
  fontWeight: 500,
  color: '#ddd',
  backgroundColor: '#2a2a2a',
  border: '1px solid #3a3a3a',
  borderRadius: '999px',
  cursor: 'pointer',
  transition: 'background-color 0.15s, border-color 0.15s',
};

const chipDismissBtn: React.CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center',
  width: '18px',
  height: '18px',
  padding: 0,
  border: 'none',
  backgroundColor: 'transparent',
  borderRadius: '50%',
  cursor: 'pointer',
  color: '#888',
  transition: 'background-color 0.15s, color 0.15s',
};

// Ghost button styling — previously read as a link (#6699cc, underlined-looking)
// which buried the fact that "Clear all" performs a destructive multi-filter
// reset. Now styled as a tertiary button consistent with the Add filter pill,
// with leading X icon to telegraph the destructive intent.
const clearAllBtn: React.CSSProperties = {
  marginLeft: 'auto',
  display: 'inline-flex',
  alignItems: 'center',
  gap: '6px',
  padding: '6px 12px',
  fontSize: '11px',
  fontWeight: 500,
  color: '#aaa',
  backgroundColor: '#383838',
  border: '1px solid #4a4a4a',
  borderRadius: '6px',
  cursor: 'pointer',
  transition: 'background-color 0.15s, color 0.15s',
};

const watchBtn = (active: boolean): React.CSSProperties => ({
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  width: '24px',
  height: '24px',
  padding: 0,
  backgroundColor: active ? 'rgba(39, 174, 96, 0.1)' : 'transparent',
  border: '1px solid #3a3a3a',
  borderRadius: '4px',
  cursor: 'pointer',
  transition: 'border-color 0.15s, background-color 0.15s',
});

interface ChipDescriptor {
  id: 'date' | 'type' | 'amount' | 'search';
  label: string;
  editorKind: FilterKind | null; // null means non-editable (search only dismissable)
  onDismiss: () => void;
}

/**
 * Chip-based filter bar for the Transactions page. Phase 1 of the
 * research-driven migration. Renders the `+ Add filter` PillButton, smart
 * search input, watch-only icon trio, and the active-filter chip strip.
 *
 * Filter editor state (`activeEditor` + anchor ref) lives here so the same
 * editor popover handles both the `Add filter` flow and the edit-chip flow.
 */
export const TxFilterBar: React.FC = () => {
  const { t } = useTranslation('wallet');
  const {
    dateFilter,
    dateRangeFrom,
    dateRangeTo,
    typeFilter,
    searchText,
    minAmount,
    maxAmount,
    watchOnlyFilter,
    hasWatchOnlyAddresses,
    setDateFilter,
    setTypeFilter,
    setSearchText,
    setAmountRange,
    setWatchOnlyFilter,
    clearFilters,
    dispatchSmartSearch,
    total,
    totalAll,
    isLoading,
  } = useTransactions();

  // The shared editor state — which editor is open + the DOM element to anchor on.
  const [activeEditor, setActiveEditor] = useState<FilterKind | null>(null);
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);
  const [addFilterOpen, setAddFilterOpen] = useState(false);

  const addFilterBtnRef = useRef<HTMLButtonElement>(null);

  // Local mirror of the search input. Cleared on Escape; kept verbatim on
  // Enter (post-B4 fix) so the user can see the active filter in the field.
  const [searchInputValue, setSearchInputValue] = useState('');

  // Sync the visible input to the slice's searchText so external mutations —
  // chip dismiss, clearFilters, saved view apply, the zero-result CTA, etc. —
  // propagate to the field. Without this, the input would keep displaying a
  // stale query after any of those code paths cleared the underlying filter.
  useEffect(() => {
    setSearchInputValue(searchText);
  }, [searchText]);

  // B1 hint: when the user is currently typing a bare positive number into
  // the search input, surface an inline icon suggesting the `>N` min-amount
  // syntax. This catches the typo before they hit Enter and get 0 results.
  const searchHintVisible = isBarePositiveNumber(searchInputValue);

  // B1 zero-result banner: a bare-positive-number search that returned 0
  // results in a non-empty dataset is almost certainly a user trying to
  // filter by amount with the wrong syntax. Offer one-click conversion to
  // a min_amount filter via the CTA below the filter bar.
  //
  // Gated on `!isLoading` to avoid a stale-total flash: dispatchSmartSearch
  // updates searchText synchronously but the slice debounces the fetch by
  // 300ms; during that window `total` reflects the PRIOR query's result, so
  // the banner could briefly advertise zero-result conversion for a query
  // that is actually about to return matches.
  const zeroResultBareNumberSearch = useMemo(() => {
    if (isLoading) return null;
    if (!isBarePositiveNumber(searchText)) return null;
    if (total !== 0 || totalAll === 0) return null;
    // Hide the banner while the user is mid-typing a different draft —
    // showing "Use \"100\" as min amount" while they're already typing
    // "alice" would mislead them into converting the wrong query. The
    // banner only makes sense when the input still shows the committed
    // bare-number query.
    if (searchInputValue !== searchText) return null;
    // Hide the banner if any amount filter is already active. The CTA's
    // `setAmountRange(value, '')` would clobber an intentional existing
    // min/max bound; better to suppress the hint than risk silent data loss
    // on click. Date / type / watch-only filters are unaffected by the CTA,
    // so they don't gate the banner.
    if (minAmount !== '' || maxAmount !== '') return null;
    const value = parseFloat(searchText.trim());
    if (!(value > 0)) return null;
    return { query: searchText.trim(), value };
  }, [searchText, searchInputValue, total, totalAll, isLoading, minAmount, maxAmount]);

  const handleConvertSearchToMinAmount = useCallback(() => {
    if (!zeroResultBareNumberSearch) return;
    setSearchText('');
    // Use the batched range setter (clears both bounds in one dispatch) so
    // any existing maxAmount is overwritten — otherwise a pre-existing upper
    // bound below the new lower bound would silently create an inverted
    // range that yields zero results, defeating the CTA's intent.
    setAmountRange(String(zeroResultBareNumberSearch.value), '');
  }, [zeroResultBareNumberSearch, setSearchText, setAmountRange]);

  const openEditor = useCallback((kind: FilterKind, el: HTMLElement) => {
    setAnchorEl(el);
    setActiveEditor(kind);
    setAddFilterOpen(false);
  }, []);

  const closeEditor = useCallback(() => {
    setActiveEditor(null);
    setAnchorEl(null);
  }, []);

  const handleAddFilterToggle = () => {
    if (addFilterOpen) {
      setAddFilterOpen(false);
    } else {
      setAnchorEl(addFilterBtnRef.current);
      setActiveEditor(null);
      setAddFilterOpen(true);
    }
  };

  const handleAddFilterPick = (kind: FilterKind) => {
    if (addFilterBtnRef.current) {
      openEditor(kind, addFilterBtnRef.current);
    }
  };

  const handleSearchKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      const query = searchInputValue.trim();
      if (!query) return;
      const parsed = parseSearchQuery(query);
      dispatchSmartSearch(query);
      // Keep the typed query visible after Enter ONLY when the dispatch will
      // create a search chip (i.e. the value remains in searchText). When the
      // input was parsed as an address or `>N` min_amount, dispatchSmartSearch
      // updates a different slice field (address still goes through
      // setSearchText so the useEffect mirror catches it, but min_amount
      // doesn't), so we must clear the local input here to keep the field's
      // visible state consistent with the active filter chips.
      if (parsed.type === 'min_amount') {
        setSearchInputValue('');
      }
      e.currentTarget.blur();
    } else if (e.key === 'Escape') {
      e.preventDefault();
      // Clear local input unconditionally — covers the unsubmitted-draft
      // case (`searchText === ''`, no chip to dismiss, just clear the draft).
      setSearchInputValue('');
      // Only call the slice setter if there's actually a chip to dismiss.
      // The slice debounces fetches off setSearchText, so a no-op call
      // when searchText is already '' would still schedule a redundant
      // 300ms-delayed fetch with identical state.
      if (searchText !== '') {
        setSearchText('');
      }
    }
  };

  // Compose the active-filter chip list.
  const chips: ChipDescriptor[] = useMemo(() => {
    const list: ChipDescriptor[] = [];

    if (dateFilter !== 'all') {
      const dateLabel =
        dateFilter === 'range'
          ? `${t('transactions.filters.chip.dateLabel')}: ${dateRangeFrom} → ${dateRangeTo}`
          : `${t('transactions.filters.chip.dateLabel')}: ${t(`transactions.filters.chip.datePresets.${DATE_PRESET_I18N_KEY[dateFilter]}`)}`;
      list.push({
        id: 'date',
        label: dateLabel,
        editorKind: 'date',
        onDismiss: () => setDateFilter('all'),
      });
    }

    if (typeFilter.length > 0) {
      const firstLabel = TYPE_LABELS[typeFilter[0]];
      const typeChipLabel =
        typeFilter.length === 1
          ? `${t('transactions.filters.chip.typeLabel')}: ${firstLabel}`
          : `${t('transactions.filters.chip.typeLabel')}: ${firstLabel} ${t(
              'transactions.filters.chip.typeMore',
              { count: typeFilter.length - 1 },
            )}`;
      list.push({
        id: 'type',
        label: typeChipLabel,
        editorKind: 'type',
        onDismiss: () => setTypeFilter([]),
      });
    }

    // Phase 4 amount bounds: single combined chip with dynamic label.
    // - only min  → "Amount: ≥X TWINS"
    // - only max  → "Amount: ≤Y TWINS"
    // - both      → "Amount: X–Y TWINS"
    // Dismiss clears BOTH bounds. A bound is "active" iff its string parses
    // to a finite positive number. This handles non-canonical zero strings
    // ('0', '0.0', '00', '0.00'), whitespace, non-numeric ('abc' → NaN), and
    // negatives ('-5') consistently with the backend, which treats any of
    // these as "no constraint" via `parseFloat || 0` in buildFilter plus
    // the wallet's `min/max > 0` guard in matchesAmountFilter. Codex round-3
    // caught the original `!== '0'` predicate failing for these variants.
    const isActiveBound = (s: string): boolean => {
      const n = parseFloat(s);
      return Number.isFinite(n) && n > 0;
    };
    const hasMin = isActiveBound(minAmount);
    const hasMax = isActiveBound(maxAmount);
    if (hasMin || hasMax) {
      const suffix = t('transactions.filters.chip.amountSuffix');
      const prefix = `${t('transactions.filters.chip.amountLabel')}: `;
      let label: string;
      if (hasMin && hasMax) {
        label = `${prefix}${minAmount}–${maxAmount} ${suffix}`;
      } else if (hasMin) {
        label = `${prefix}≥${minAmount} ${suffix}`;
      } else {
        label = `${prefix}≤${maxAmount} ${suffix}`;
      }
      list.push({
        id: 'amount',
        label,
        editorKind: 'amount',
        // Use the batched setter so dismiss fires ONE fetch, not two —
        // matches the editor's Apply/Clear path.
        onDismiss: () => setAmountRange('', ''),
      });
    }

    if (searchText) {
      const searchLabel = TWINS_ADDRESS_REGEX.test(searchText)
        ? `${t('transactions.filters.chip.addressLabel')}: ${searchText.slice(0, 8)}…`
        : `${t('transactions.filters.chip.searchLabel')}: ${searchText}`;
      list.push({
        id: 'search',
        label: searchLabel,
        editorKind: null,
        onDismiss: () => setSearchText(''),
      });
    }

    return list;
  }, [
    dateFilter,
    dateRangeFrom,
    dateRangeTo,
    typeFilter,
    minAmount,
    maxAmount,
    searchText,
    setDateFilter,
    setTypeFilter,
    setAmountRange,
    setSearchText,
    t,
  ]);

  const handleChipClick = (chip: ChipDescriptor, e: React.MouseEvent<HTMLDivElement>) => {
    if (!chip.editorKind) return;
    openEditor(chip.editorKind, e.currentTarget);
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
      {/* Row 1: filter bar */}
      <div style={{ display: 'flex', gap: '8px', alignItems: 'center' }}>
        <button
          ref={addFilterBtnRef}
          type="button"
          style={pillButton}
          onClick={handleAddFilterToggle}
          aria-haspopup="menu"
          aria-expanded={addFilterOpen}
        >
          <Plus size={12} />
          {t('transactions.filters.chip.addFilter')}
        </button>

        <div style={{ flex: 1, position: 'relative', display: 'flex', alignItems: 'center' }}>
          <Search
            size={14}
            color="#888"
            style={{ position: 'absolute', left: '8px', pointerEvents: 'none' }}
          />
          <input
            type="text"
            style={{
              ...inputStyle,
              paddingRight: searchHintVisible ? '28px' : '10px',
            }}
            value={searchInputValue}
            onChange={(e) => setSearchInputValue(e.target.value)}
            onKeyDown={handleSearchKeyDown}
            placeholder={t('transactions.filters.chip.searchPlaceholder')}
            aria-label={t('transactions.filters.chip.searchLabel')}
            autoCapitalize="off"
            autoCorrect="off"
            spellCheck={false}
            autoComplete="off"
          />
          {searchHintVisible && (
            <span
              title={t('transactions.filters.chip.searchAmountHint')}
              aria-label={t('transactions.filters.chip.searchAmountHint')}
              style={{
                position: 'absolute',
                right: '8px',
                display: 'flex',
                alignItems: 'center',
                cursor: 'help',
              }}
            >
              <Info size={14} color="#ff9966" />
            </span>
          )}
        </div>

        {/* Watch-only icon trio. Kept compact and outside the chip model
            per the research/investigation decision (uncommon use case,
            tri-state preset, deserves a permanent visual slot). */}
        {hasWatchOnlyAddresses && (
          <div style={{ display: 'flex', gap: '2px', alignItems: 'center', height: '32px' }}>
            {([
              { value: 'all' as const, icon: <Eye size={14} color="#888" />, title: t('transactions.filters.chip.watchOnlyAll') },
              { value: 'yes' as const, icon: <Eye size={14} color="#27ae60" />, title: t('transactions.filters.chip.watchOnlyOnly') },
              { value: 'no' as const, icon: <EyeOff size={14} color="#ff6666" />, title: t('transactions.filters.chip.watchOnlyNon') },
            ]).map((opt) => {
              const isActive = watchOnlyFilter === opt.value;
              return (
                <button
                  key={opt.value}
                  type="button"
                  onClick={() => setWatchOnlyFilter(opt.value)}
                  title={opt.title}
                  aria-label={opt.title}
                  aria-pressed={isActive}
                  style={watchBtn(isActive)}
                  onMouseEnter={(e) => {
                    e.currentTarget.style.borderColor = '#555';
                  }}
                  onMouseLeave={(e) => {
                    e.currentTarget.style.borderColor = '#3a3a3a';
                  }}
                >
                  {opt.icon}
                </button>
              );
            })}
          </div>
        )}

        {/* Saved views menu (Phase 2). Anchored on the right edge of row 1. */}
        <TxViewsMenu />
      </div>

      {/* B1 zero-result banner: bare-positive-number search that returned 0
          rows is almost always a user trying to filter by amount with the
          wrong syntax. Offer a one-click conversion to a min_amount filter. */}
      {zeroResultBareNumberSearch && (
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
            flexWrap: 'wrap',
          }}
        >
          <div style={{ flex: 1, minWidth: '200px' }}>
            <Banner
              variant="warning"
              message={t('transactions.filters.chip.searchAmountBannerMessage', {
                query: zeroResultBareNumberSearch.query,
              })}
            />
          </div>
          <button
            type="button"
            onClick={handleConvertSearchToMinAmount}
            style={{
              backgroundColor: '#4a7c59',
              border: '1px solid #5a8c69',
              borderRadius: '6px',
              padding: '6px 14px',
              fontSize: '12px',
              fontWeight: 500,
              color: '#fff',
              cursor: 'pointer',
              whiteSpace: 'nowrap',
            }}
          >
            {t('transactions.filters.chip.searchAmountBannerCTA', {
              value: zeroResultBareNumberSearch.value,
            })}
          </button>
        </div>
      )}

      {/* Row 2: active filter chip strip (only when ≥ 1 active) */}
      {chips.length > 0 && (
        <div
          style={{
            display: 'flex',
            flexWrap: 'wrap',
            gap: '6px',
            alignItems: 'center',
          }}
        >
          {chips.map((chip) => (
            <div
              key={chip.id}
              style={chipStyle}
              onClick={(e) => handleChipClick(chip, e)}
              role={chip.editorKind ? 'button' : undefined}
              tabIndex={chip.editorKind ? 0 : undefined}
              onKeyDown={(e) => {
                if (chip.editorKind && (e.key === 'Enter' || e.key === ' ')) {
                  e.preventDefault();
                  handleChipClick(chip, e as unknown as React.MouseEvent<HTMLDivElement>);
                }
              }}
              onMouseEnter={(e) => {
                e.currentTarget.style.borderColor = '#444';
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.borderColor = '#3a3a3a';
              }}
            >
              <span>{chip.label}</span>
              <button
                type="button"
                style={chipDismissBtn}
                onClick={(e) => {
                  e.stopPropagation();
                  chip.onDismiss();
                }}
                title={t('transactions.filters.chip.removeFilter')}
                aria-label={`${t('transactions.filters.chip.removeFilter')}: ${chip.label}`}
                onMouseEnter={(e) => {
                  e.currentTarget.style.backgroundColor = '#3a3a3a';
                  e.currentTarget.style.color = '#ddd';
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.backgroundColor = 'transparent';
                  e.currentTarget.style.color = '#888';
                }}
              >
                <X size={10} />
              </button>
            </div>
          ))}

          <button
            type="button"
            style={clearAllBtn}
            onClick={() => {
              // Close any open editor first — clearing filters removes the chip
              // that supplied the editor's anchorEl, otherwise the popover
              // would linger against a detached/incorrect anchor.
              closeEditor();
              setAddFilterOpen(false);
              clearFilters();
            }}
            onMouseEnter={(e) => {
              e.currentTarget.style.backgroundColor = '#444';
              e.currentTarget.style.color = '#ddd';
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.backgroundColor = '#383838';
              e.currentTarget.style.color = '#aaa';
            }}
          >
            <X size={12} />
            {t('transactions.filters.chip.clearAll')}
          </button>
        </div>
      )}

      {/* Add Filter dropdown popover */}
      <TxFilterPopover
        anchorEl={addFilterBtnRef.current}
        isOpen={addFilterOpen}
        onClose={() => setAddFilterOpen(false)}
        width={180}
        padding="4px"
      >
        <div style={{ display: 'flex', flexDirection: 'column' }} role="menu" aria-label="Add filter">
          {([
            { kind: 'date' as const, label: t('transactions.filters.chip.dateLabel') },
            { kind: 'type' as const, label: t('transactions.filters.chip.typeLabel') },
            { kind: 'amount' as const, label: t('transactions.filters.chip.amountLabel') },
          ]).map((opt) => (
            <button
              key={opt.kind}
              type="button"
              role="menuitem"
              style={{
                padding: '6px 12px',
                fontSize: '12px',
                color: '#ddd',
                backgroundColor: 'transparent',
                border: 'none',
                borderRadius: '4px',
                cursor: 'pointer',
                textAlign: 'left',
              }}
              onClick={() => handleAddFilterPick(opt.kind)}
              onMouseEnter={(e) => {
                e.currentTarget.style.backgroundColor = '#383838';
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.backgroundColor = 'transparent';
              }}
            >
              {opt.label}
            </button>
          ))}
        </div>
      </TxFilterPopover>

      {/*
        Editor popovers: four <TxFilterPopover> instances are rendered in this
        subtree (Add Filter dropdown + three editor popovers). Mutual exclusion
        is enforced by state: `addFilterOpen` is set to false whenever an editor
        opens (see `openEditor`), and `activeEditor` is a single string so only
        one of {date,type,amount} can be truthy at a time. Each popover's
        global listeners short-circuit via the `isOpen` guard, so in practice
        only one set of mousedown/Escape/scroll/resize handlers is active.
        Future maintainers: preserve this invariant when modifying open/close
        flows, or consolidate into a single popover with switched children.
      */}
      <TxFilterPopover anchorEl={anchorEl} isOpen={activeEditor === 'date'} onClose={closeEditor} width={280}>
        <TxFilterDateEditor onClose={closeEditor} />
      </TxFilterPopover>
      <TxFilterPopover anchorEl={anchorEl} isOpen={activeEditor === 'type'} onClose={closeEditor} width={240} padding="4px">
        <TxFilterTypeEditor onClose={closeEditor} />
      </TxFilterPopover>
      <TxFilterPopover anchorEl={anchorEl} isOpen={activeEditor === 'amount'} onClose={closeEditor} width={260}>
        <TxFilterAmountEditor onClose={closeEditor} />
      </TxFilterPopover>
    </div>
  );
};
