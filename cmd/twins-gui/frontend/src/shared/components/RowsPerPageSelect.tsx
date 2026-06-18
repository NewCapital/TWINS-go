import React, { useCallback, useEffect, useRef, useState } from 'react';
import { ChevronDown } from 'lucide-react';

// ---------------------------------------------------------------------------
// RowsPerPageSelect — custom dark-themed listbox replacing native `<select>`.
//
// Lifted verbatim from the two pre-existing local implementations in
// `features/wallet/pages/Transactions.tsx` and
// `features/explorer/components/BlockList.tsx` (both copies were
// bit-equivalent — confirmed before extraction). The native `<select>`
// dropdown's UA chrome (light panel + light option rows) clashes with the
// Receive design language; this implementation owns its own keyboard nav,
// outside-click/Escape/scroll/resize close, and ARIA semantics.
//
// Notable race-safety details preserved:
// - Option commit uses `onMouseDown` + `preventDefault` so the commit lands
//   BEFORE the document outside-click listener closes the popover.
// - Escape is captured in capture phase (`useCapture: true`) with
//   `stopPropagation` so parent dialog/page Escape handlers don't also fire.
// - `onTriggerMouseDown` callback lets a parent control fire a synchronous
//   reset BEFORE focus shifts away from a sibling input — critical for the
//   pagination-footer race where the input's onBlur would otherwise commit a
//   stale typed value alongside the page-size change.
// ---------------------------------------------------------------------------
export interface RowsPerPageSelectProps<T extends string | number = number> {
  value: T;
  options: readonly T[];
  onChange: (next: T) => void;
  /** Fired on mousedown of the trigger button BEFORE focus shifts away from
   *  any sibling input. Pagination footer uses this to reset its page-input
   *  draft so the focused input's onBlur cannot commit a stale jump alongside
   *  the page-size change fetch. */
  onTriggerMouseDown?: () => void;
  /** Override the default `Rows per page: ${value}` aria-label. Use for
   *  non-pagination consumers (e.g. unit selectors) so screen readers
   *  announce the actual purpose of the control. */
  ariaLabel?: string;
  /** Text-alignment inside option rows. Default 'right' for numeric
   *  pagination (matches the trigger's right-aligned ChevronDown). Set
   *  'left' for text labels (e.g. unit names). */
  align?: 'left' | 'right';
  /** Style overrides merged over the built-in trigger inline style block.
   *  Spread LAST so caller overrides win over defaults like
   *  `padding: '4px 8px'` and `minWidth: '56px'`. */
  triggerStyle?: React.CSSProperties;
}

export function RowsPerPageSelect<T extends string | number = number>({
  value,
  options,
  onChange,
  onTriggerMouseDown,
  ariaLabel,
  align = 'right',
  triggerStyle,
}: RowsPerPageSelectProps<T>) {
  const [isOpen, setIsOpen] = useState(false);
  const [highlightedIndex, setHighlightedIndex] = useState<number>(() =>
    Math.max(0, options.indexOf(value))
  );
  const rootRef = useRef<HTMLDivElement | null>(null);
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const listboxRef = useRef<HTMLDivElement | null>(null);

  // Sync highlighted index to currently-selected value whenever the popover
  // (re)opens — so the keyboard cursor starts on the right item. Also move
  // focus into the listbox once (not on every render) so ArrowUp/Down work
  // immediately without yanking focus back from any later tab-stop.
  useEffect(() => {
    if (isOpen) {
      const idx = options.indexOf(value);
      setHighlightedIndex(idx >= 0 ? idx : 0);
      listboxRef.current?.focus();
    }
  }, [isOpen, value, options]);

  // Close on outside click, scroll, resize, or Escape (global, capture phase
  // so it stopPropagations before any parent listener fires).
  useEffect(() => {
    if (!isOpen) return;
    const handleMouseDown = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
        setIsOpen(false);
      }
    };
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        setIsOpen(false);
        triggerRef.current?.focus();
      }
    };
    const close = () => setIsOpen(false);
    document.addEventListener('mousedown', handleMouseDown);
    document.addEventListener('keydown', handleKeyDown, true);
    window.addEventListener('scroll', close, true);
    window.addEventListener('resize', close);
    return () => {
      document.removeEventListener('mousedown', handleMouseDown);
      document.removeEventListener('keydown', handleKeyDown, true);
      window.removeEventListener('scroll', close, true);
      window.removeEventListener('resize', close);
    };
  }, [isOpen]);

  const commit = useCallback(
    (next: T) => {
      if (next !== value) onChange(next);
      setIsOpen(false);
      triggerRef.current?.focus();
    },
    [value, onChange]
  );

  const handleTriggerKeyDown = (e: React.KeyboardEvent<HTMLButtonElement>) => {
    // Only ArrowDown / ArrowUp force-open via keydown — Enter and Space are
    // intentionally NOT handled here because a native <button> synthesizes a
    // click on those keys, which fires onClick and toggles isOpen. Handling
    // them in keydown too would open the popover and then immediately re-close
    // it via the click toggle.
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      setIsOpen(true);
    }
  };

  const handleListKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setHighlightedIndex((i) => Math.min(options.length - 1, i + 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setHighlightedIndex((i) => Math.max(0, i - 1));
    } else if (e.key === 'Home') {
      e.preventDefault();
      setHighlightedIndex(0);
    } else if (e.key === 'End') {
      e.preventDefault();
      setHighlightedIndex(options.length - 1);
    } else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      const choice = options[highlightedIndex];
      if (choice !== undefined) commit(choice);
    }
  };

  return (
    <div ref={rootRef} style={{ position: 'relative', display: 'inline-flex' }}>
      <button
        ref={triggerRef}
        type="button"
        onMouseDown={onTriggerMouseDown}
        onClick={() => setIsOpen((v) => !v)}
        onKeyDown={handleTriggerKeyDown}
        aria-haspopup="listbox"
        aria-expanded={isOpen}
        aria-label={ariaLabel ?? `Rows per page: ${value}`}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '6px',
          padding: '4px 8px',
          fontSize: '12px',
          backgroundColor: '#252525',
          border: '1px solid #3a3a3a',
          borderRadius: '4px',
          color: '#ddd',
          cursor: 'pointer',
          outline: 'none',
          minWidth: '56px',
          justifyContent: 'space-between',
          ...triggerStyle,
        }}
      >
        <span>{value}</span>
        <ChevronDown size={12} color="#888" />
      </button>
      {isOpen && (
        <div
          ref={listboxRef}
          role="listbox"
          tabIndex={-1}
          onKeyDown={handleListKeyDown}
          onBlur={(e) => {
            // Close when keyboard focus traverses outside the select root
            // (Tab / Shift+Tab). `relatedTarget === null` (e.g. programmatic
            // blur with no new target) is treated as in-root — leave open and
            // let Escape / outside-click close.
            const next = e.relatedTarget as Node | null;
            if (next && rootRef.current && !rootRef.current.contains(next)) {
              setIsOpen(false);
            }
          }}
          style={{
            position: 'absolute',
            bottom: 'calc(100% + 4px)',
            right: 0,
            minWidth: '100%',
            backgroundColor: '#2f2f2f',
            border: '1px solid #3a3a3a',
            borderRadius: '6px',
            padding: '4px',
            boxShadow: '0 4px 12px rgba(0, 0, 0, 0.6)',
            zIndex: 50,
            outline: 'none',
          }}
        >
          {options.map((opt, idx) => {
            const isSelected = opt === value;
            const isHighlighted = idx === highlightedIndex;
            return (
              <div
                key={opt}
                role="option"
                aria-selected={isSelected}
                onMouseEnter={() => setHighlightedIndex(idx)}
                onMouseDown={(e) => {
                  // mousedown (not click) so the option commits before the
                  // outside-click handler on document fires and races to
                  // close the popover.
                  e.preventDefault();
                  commit(opt);
                }}
                style={{
                  padding: '6px 12px',
                  fontSize: '12px',
                  borderRadius: '4px',
                  color: isSelected ? '#27ae60' : '#ddd',
                  backgroundColor: isHighlighted ? '#383838' : 'transparent',
                  cursor: 'pointer',
                  textAlign: align,
                  minWidth: '48px',
                }}
              >
                {opt}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
