import React, { useRef, useState, useEffect } from 'react';
import { Search, X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useStore } from '@/store/useStore';
import { useShallow } from 'zustand/react/shallow';
import { IconButton } from '@/shared/components/IconButton';
import { classifyExplorerQuery } from '@/shared/utils/parseExplorerSearchQuery';
import { SearchHistoryDropdown } from './SearchHistoryDropdown';

interface SearchBarProps {
  value: string;
  isSearching: boolean;
  onSearch: (query: string) => void;
  onChange: (value: string) => void;
}

/**
 * Explorer search bar with real-time type detection, clear-X affordance,
 * Escape-to-clear, and a history dropdown that opens on input focus when
 * the persisted search history is non-empty. Reads searchHistory from the
 * Zustand explorer slice directly to avoid prop drilling through BlockList.
 */
export const SearchBar: React.FC<SearchBarProps> = ({
  value,
  isSearching,
  onSearch,
  onChange,
}) => {
  const { t } = useTranslation('common');
  const { searchHistory, clearSearchHistory } = useStore(
    useShallow((state) => ({
      searchHistory: state.searchHistory,
      clearSearchHistory: state.clearSearchHistory,
    }))
  );

  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const [isDropdownOpen, setDropdownOpen] = useState(false);
  const [highlightedIdx, setHighlightedIdx] = useState(-1);

  const detection = classifyExplorerQuery(value);
  const showBadge = value.trim().length > 0;
  const showClear = value.length > 0;
  const isInvalid = detection.type === 'invalid';
  const submitDisabled = isSearching || !value.trim() || isInvalid;

  const badgeColor: string =
    detection.type === 'block_height'
      ? '#27ae60'
      : detection.type === 'block_or_tx_hash'
        ? '#6699cc'
        : detection.type === 'address'
          ? '#bb88dd'
          : '#ff6666';

  const closeDropdown = () => {
    setDropdownOpen(false);
    setHighlightedIdx(-1);
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (submitDisabled) return;
    closeDropdown();
    onSearch(value.trim());
  };

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    onChange(e.target.value);
    // Reset keyboard navigation cursor when the user edits the input —
    // pressing ArrowDown later should start fresh from the top.
    setHighlightedIdx(-1);
  };

  const handleFocus = () => {
    if (searchHistory.length > 0) {
      setDropdownOpen(true);
    }
  };

  const handleBlur = (e: React.FocusEvent<HTMLInputElement>) => {
    // Only close when focus moves outside the SearchBar root (input +
    // dropdown). When focus moves INTO the dropdown (e.g. user clicks an
    // item) keep the dropdown alive so the click-commit can complete.
    const next = e.relatedTarget as Node | null;
    if (!next || (containerRef.current && !containerRef.current.contains(next))) {
      // Defer slightly so an in-flight mousedown on a dropdown item can
      // dispatch its onSelect before close fires.
      setTimeout(() => {
        // Re-check focus when the timer fires — if the input or dropdown
        // got focus back, leave the dropdown open. If containerRef.current
        // is null (component unmounted mid-blur, e.g. routing change),
        // close defensively so isDropdownOpen state cannot get stuck true.
        if (!containerRef.current || !containerRef.current.contains(document.activeElement)) {
          closeDropdown();
        }
      }, 0);
    }
  };

  const commitHistoryItem = (query: string) => {
    onChange(query);
    closeDropdown();
    onSearch(query);
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    // ArrowDown: open dropdown if closed + history non-empty, else move highlight
    if (e.key === 'ArrowDown') {
      if (searchHistory.length === 0) return;
      e.preventDefault();
      if (!isDropdownOpen) {
        setDropdownOpen(true);
        setHighlightedIdx(0);
        return;
      }
      setHighlightedIdx((prev) => (prev + 1) % searchHistory.length);
      return;
    }

    if (e.key === 'ArrowUp') {
      if (!isDropdownOpen || searchHistory.length === 0) return;
      e.preventDefault();
      setHighlightedIdx((prev) =>
        prev <= 0 ? searchHistory.length - 1 : prev - 1
      );
      return;
    }

    if (e.key === 'Enter') {
      if (isDropdownOpen && highlightedIdx >= 0 && highlightedIdx < searchHistory.length) {
        e.preventDefault();
        commitHistoryItem(searchHistory[highlightedIdx].query);
        return;
      }
      // Otherwise let the form's onSubmit handle Enter for the input value.
      return;
    }

    if (e.key === 'Escape') {
      // Escape precedence: close dropdown first (if open), else clear input.
      if (isDropdownOpen) {
        e.preventDefault();
        closeDropdown();
        return;
      }
      if (value.length > 0) {
        e.preventDefault();
        onChange('');
      }
    }
  };

  // Listen for the global "explorer:focus-search" event dispatched by the
  // not-found Banner's Try again button. Focuses the input so the user can
  // immediately retype a corrected query without reaching for the mouse.
  useEffect(() => {
    const handler = () => {
      inputRef.current?.focus();
    };
    window.addEventListener('explorer:focus-search', handler);
    return () => window.removeEventListener('explorer:focus-search', handler);
  }, []);

  // Reset highlight if history changes while dropdown is open (e.g. an
  // in-flight search succeeds and prepends a new entry).
  useEffect(() => {
    if (isDropdownOpen && highlightedIdx >= searchHistory.length) {
      setHighlightedIdx(searchHistory.length === 0 ? -1 : 0);
    }
    // Auto-close if history was cleared while dropdown was open.
    if (isDropdownOpen && searchHistory.length === 0) {
      setDropdownOpen(false);
    }
  }, [searchHistory.length, isDropdownOpen, highlightedIdx]);

  const handleClearHistory = () => {
    clearSearchHistory();
    closeDropdown();
    // Re-focus input so the user can keep typing without re-clicking.
    inputRef.current?.focus();
  };

  return (
    <form onSubmit={handleSubmit} style={{ flex: 1 }}>
      <div ref={containerRef} style={{ display: 'flex', gap: '8px', position: 'relative' }}>
        <div
          style={{
            flex: 1,
            position: 'relative',
            display: 'flex',
            alignItems: 'center',
          }}
        >
          <input
            ref={inputRef}
            type="text"
            value={value}
            onChange={handleChange}
            onKeyDown={handleKeyDown}
            onFocus={handleFocus}
            onBlur={handleBlur}
            placeholder={t('explorer.search.placeholder')}
            aria-invalid={isInvalid && showBadge}
            aria-haspopup="listbox"
            aria-expanded={isDropdownOpen}
            autoComplete="off"
            autoCorrect="off"
            autoCapitalize="off"
            spellCheck={false}
            name="explorer-search"
            style={{
              width: '100%',
              padding: '7px 110px 7px 36px',
              fontSize: '12px',
              backgroundColor: '#252525',
              border: `1px solid ${isInvalid && showBadge ? '#ff6666' : '#3a3a3a'}`,
              borderRadius: '4px',
              color: '#ddd',
              outline: 'none',
            }}
            disabled={isSearching}
          />
          <Search
            size={14}
            style={{
              position: 'absolute',
              left: '10px',
              top: '50%',
              transform: 'translateY(-50%)',
              color: '#888',
              pointerEvents: 'none',
            }}
          />
          <div
            style={{
              position: 'absolute',
              right: '6px',
              top: '50%',
              transform: 'translateY(-50%)',
              display: 'flex',
              alignItems: 'center',
              gap: '6px',
            }}
          >
            {showBadge && (
              <span
                title={t('explorer.search.typeDetectedTitle')}
                style={{
                  fontSize: '11px',
                  fontWeight: 500,
                  color: badgeColor,
                  letterSpacing: '0.5px',
                  whiteSpace: 'nowrap',
                  pointerEvents: 'none',
                }}
              >
                {t(`explorer.search.typeDetected.${detection.type}`)}
              </span>
            )}
            {showClear && (
              <IconButton
                onClick={() => onChange('')}
                title={t('explorer.search.clearTitle')}
                ariaLabel={t('explorer.search.clearLabel')}
                icon={<X size={12} />}
                disabled={isSearching}
              />
            )}
          </div>
          {isDropdownOpen && searchHistory.length > 0 && (
            <SearchHistoryDropdown
              history={searchHistory}
              onSelect={commitHistoryItem}
              onClear={handleClearHistory}
              onClose={closeDropdown}
              highlightedIndex={highlightedIdx}
              onHighlightChange={setHighlightedIdx}
            />
          )}
        </div>
      </div>
    </form>
  );
};
