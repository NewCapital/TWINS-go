import React, { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Box, ArrowRightLeft, Wallet } from 'lucide-react';
import type { SearchHistoryItem } from '@/store/slices/explorerSlice';
import { truncateAddress } from '@/shared/utils/format';

interface SearchHistoryDropdownProps {
  history: SearchHistoryItem[];
  onSelect: (query: string) => void;
  onClear: () => void;
  onClose: () => void;
  // Highlighted index controlled by parent SearchBar so its onKeyDown handler
  // (Arrow keys / Enter while the input has focus) can drive selection without
  // forwarding focus down to the listbox. The dropdown also tracks its own
  // hover-driven highlight changes via onHighlightChange.
  highlightedIndex: number;
  onHighlightChange: (index: number) => void;
}

/**
 * Search history dropdown for Explorer SearchBar. Renders a list of recent
 * successful searches with type icon + truncated query + optional label
 * (e.g. block height for hash searches). Follows the dropdown lifecycle
 * convention from RowsPerPageSelect: outside mousedown closes, Escape closes
 * with stopPropagation + capture phase, window scroll/resize closes.
 *
 * Items use onMouseDown + preventDefault to win the race against the input's
 * blur event — without this, clicking an item would blur the input first,
 * potentially closing the dropdown before the click handler fires.
 *
 * Keyboard navigation is driven by the parent SearchBar via highlightedIndex
 * + onHighlightChange so the input keeps focus while the user arrows through
 * history items.
 */
export const SearchHistoryDropdown: React.FC<SearchHistoryDropdownProps> = ({
  history,
  onSelect,
  onClear,
  onClose,
  highlightedIndex,
  onHighlightChange,
}) => {
  const { t } = useTranslation('common');
  const rootRef = useRef<HTMLDivElement>(null);
  const [clearHovered, setClearHovered] = useState(false);

  // Outside mousedown close: if click target is outside the dropdown root,
  // close. Capture phase not needed — bubble-phase mousedown fires before
  // any onClick handler inside the dropdown.
  useEffect(() => {
    const handleOutside = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        onClose();
      }
    };
    document.addEventListener('mousedown', handleOutside);
    return () => document.removeEventListener('mousedown', handleOutside);
  }, [onClose]);

  // Escape close in capture phase + stopPropagation so the parent
  // SearchBar's input onKeyDown (also handles Escape to clear input)
  // doesn't ALSO fire. Pressing Escape with the dropdown open should
  // dismiss only the dropdown, not clear the input.
  useEffect(() => {
    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        onClose();
      }
    };
    document.addEventListener('keydown', handleEscape, true);
    return () => document.removeEventListener('keydown', handleEscape, true);
  }, [onClose]);

  // Window scroll/resize close: prevent the absolute-positioned dropdown
  // from visually detaching from its anchor input when the page reflows.
  useEffect(() => {
    const handleScrollResize = () => onClose();
    window.addEventListener('scroll', handleScrollResize, true);
    window.addEventListener('resize', handleScrollResize);
    return () => {
      window.removeEventListener('scroll', handleScrollResize, true);
      window.removeEventListener('resize', handleScrollResize);
    };
  }, [onClose]);

  // Parent gates rendering on history.length > 0, but defense-in-depth.
  if (history.length === 0) return null;

  const iconForType = (type: SearchHistoryItem['type']) => {
    switch (type) {
      case 'block':
        return <Box size={12} color="#888" />;
      case 'transaction':
        return <ArrowRightLeft size={12} color="#888" />;
      case 'address':
        return <Wallet size={12} color="#888" />;
    }
  };

  const displayQuery = (item: SearchHistoryItem): string => {
    // Block-height numeric queries render verbatim; hash/address truncate.
    if (item.type === 'block' && /^\d+$/.test(item.query)) {
      return item.query;
    }
    if (item.query.length > 24) {
      return truncateAddress(item.query, 8, 8);
    }
    return item.query;
  };

  return (
    <div
      ref={rootRef}
      role="listbox"
      aria-label={t('explorer.search.history.recentSearches')}
      style={{
        position: 'absolute',
        top: 'calc(100% + 4px)',
        left: 0,
        right: 0,
        backgroundColor: '#2f2f2f',
        border: '1px solid #3a3a3a',
        borderRadius: '6px',
        boxShadow: '0 4px 12px rgba(0,0,0,0.6)',
        zIndex: 50,
        padding: '4px',
        maxHeight: '320px',
        overflowY: 'auto',
      }}
    >
      {history.map((item, i) => {
        const isHighlighted = i === highlightedIndex;
        return (
          <div
            key={`${item.query}-${item.timestamp}`}
            role="option"
            aria-selected={isHighlighted}
            onMouseDown={(e) => {
              // preventDefault wins the race against the input's blur
              // event — without this, blur could fire onClose before this
              // handler runs and the item would never commit.
              e.preventDefault();
              onSelect(item.query);
            }}
            onMouseEnter={() => onHighlightChange(i)}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
              padding: '6px 10px',
              borderRadius: '4px',
              cursor: 'pointer',
              backgroundColor: isHighlighted ? '#383838' : 'transparent',
            }}
          >
            <span style={{ flexShrink: 0, display: 'flex', alignItems: 'center' }}>
              {iconForType(item.type)}
            </span>
            <span
              style={{
                flex: 1,
                fontSize: '13px',
                fontFamily: 'monospace',
                color: '#ddd',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
              }}
              title={item.query}
            >
              {displayQuery(item)}
            </span>
            {item.label && (
              <span
                style={{
                  flexShrink: 0,
                  fontSize: '11px',
                  color: '#888',
                  fontFamily: 'monospace',
                }}
              >
                {item.label}
              </span>
            )}
          </div>
        );
      })}
      <div
        style={{
          borderTop: '1px solid #3a3a3a',
          marginTop: '4px',
          paddingTop: '4px',
        }}
      >
        <button
          type="button"
          onMouseDown={(e) => {
            // preventDefault to keep the input focused after clear so the
            // user can immediately type a fresh query.
            e.preventDefault();
            onClear();
          }}
          onMouseEnter={() => setClearHovered(true)}
          onMouseLeave={() => setClearHovered(false)}
          style={{
            width: '100%',
            padding: '6px 10px',
            backgroundColor: clearHovered ? '#383838' : 'transparent',
            border: 'none',
            borderRadius: '4px',
            fontSize: '11px',
            color: '#888',
            cursor: 'pointer',
            textAlign: 'left',
          }}
        >
          {t('explorer.search.history.clearHistory')}
        </button>
      </div>
    </div>
  );
};
