import React, { useEffect, useMemo, useRef, useState } from 'react';
import { Calendar as CalendarIcon, ChevronLeft, ChevronRight } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useTransactions } from '@/store/useStore';
import type { DateFilter } from '@/store/slices/transactionsSlice';
import { isoToDisplay, displayToIso } from '../utils/dateFormat';

interface TxFilterDateEditorProps {
  onClose: () => void;
}

const PRESETS: Exclude<DateFilter, 'range'>[] = [
  'today',
  'week',
  'month',
  'lastMonth',
  'year',
  'all',
];

// Maps preset key → i18n key under transactions.filters.chip.datePresets.
const PRESET_I18N_KEY: Record<Exclude<DateFilter, 'range'>, string> = {
  today: 'today',
  week: 'thisWeek',
  month: 'thisMonth',
  lastMonth: 'lastMonth',
  year: 'thisYear',
  all: 'allTime',
};

const pillBase: React.CSSProperties = {
  padding: '6px 10px',
  fontSize: '11px',
  fontWeight: 500,
  borderRadius: '999px',
  cursor: 'pointer',
  textAlign: 'center',
  border: '1px solid #4a4a4a',
  backgroundColor: '#383838',
  color: '#ccc',
  transition: 'background-color 0.15s, border-color 0.15s',
};

const pillSelected: React.CSSProperties = {
  ...pillBase,
  backgroundColor: 'rgba(39, 174, 96, 0.15)',
  borderColor: '#27ae60',
  color: '#27ae60',
};

const inputStyle: React.CSSProperties = {
  backgroundColor: '#252525',
  border: '1px solid #3a3a3a',
  borderRadius: '4px',
  padding: '7px 10px',
  fontSize: '12px',
  color: '#ddd',
  flex: 1,
  outline: 'none',
};

const primaryButton: React.CSSProperties = {
  backgroundColor: '#4a7c59',
  border: '1px solid #5a8c69',
  borderRadius: '6px',
  padding: '6px 14px',
  fontSize: '12px',
  fontWeight: 500,
  color: '#fff',
  cursor: 'pointer',
};

const secondaryButton: React.CSSProperties = {
  backgroundColor: '#383838',
  border: '1px solid #4a4a4a',
  borderRadius: '6px',
  padding: '6px 14px',
  fontSize: '12px',
  color: '#ccc',
  cursor: 'pointer',
};

const calendarIconBtn: React.CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center',
  width: '32px',
  height: '32px',
  padding: 0,
  border: '1px solid #3a3a3a',
  borderRadius: '4px',
  backgroundColor: '#252525',
  color: '#888',
  cursor: 'pointer',
  flexShrink: 0,
  transition: 'border-color 0.15s, color 0.15s',
};

// Local regex matching the helper in utils/dateFormat.ts. Used only to parse
// stored ISO values for the picker's view-month init — out-of-scope-tested
// indirectly by the editor flow tests; the canonical parser lives in
// utils/dateFormat.ts.
const ISO_RE = /^(\d{4})-(\d{2})-(\d{2})$/;

// ---- InlineDatePicker ----

interface InlineDatePickerProps {
  value: string; // ISO YYYY-MM-DD or ''
  onChange: (iso: string) => void;
  onClose: () => void;
}

const MONTH_NAMES = [
  'January', 'February', 'March', 'April', 'May', 'June',
  'July', 'August', 'September', 'October', 'November', 'December',
];

// Monday-first day-of-week order, matching the project's existing native
// date-input convention (which used the OS locale and rendered Monday-first
// in the test environment).
const DAY_LABELS = ['Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa', 'Su'];

/**
 * Custom inline month-grid date picker. Rendered absolutely-positioned
 * underneath an anchor input. Closes on outside click + Escape.
 *
 * No external dependency — uses native Date arithmetic and is small enough
 * to inline rather than pulling in `react-day-picker` (~25KB gzip).
 */
const InlineDatePicker: React.FC<InlineDatePickerProps> = ({ value, onChange, onClose }) => {
  // Track which month is currently visible. Initialize from value if set,
  // otherwise from today.
  const today = useMemo(() => new Date(), []);
  const initialDate = useMemo(() => {
    if (value) {
      const m = value.match(ISO_RE);
      if (m) return new Date(parseInt(m[1], 10), parseInt(m[2], 10) - 1, parseInt(m[3], 10));
    }
    return today;
  }, [value, today]);
  const [viewYear, setViewYear] = useState(initialDate.getFullYear());
  const [viewMonth, setViewMonth] = useState(initialDate.getMonth()); // 0-11

  const containerRef = useRef<HTMLDivElement>(null);

  // Outside click + Escape close. Capture phase + stopPropagation on Escape
  // to prevent the parent TxFilterPopover's bubble-phase listener from also
  // closing the popover (which would close BOTH this picker and the parent
  // editor in one keystroke).
  useEffect(() => {
    const handleMouseDown = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        onClose();
      }
    };
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        // stopImmediatePropagation (not just stopPropagation) is required to
        // prevent the parent TxFilterPopover's keydown listener — attached to
        // the same `document` target in the same capture phase — from also
        // firing and closing the entire date filter editor. The two listeners
        // would otherwise both run on the same keystroke; stopPropagation
        // only blocks bubbling to ancestor nodes, not sibling listeners on
        // the same node.
        e.stopImmediatePropagation();
        onClose();
      }
    };
    document.addEventListener('mousedown', handleMouseDown);
    document.addEventListener('keydown', handleKeyDown, true);
    return () => {
      document.removeEventListener('mousedown', handleMouseDown);
      document.removeEventListener('keydown', handleKeyDown, true);
    };
  }, [onClose]);

  const goPrev = () => {
    if (viewMonth === 0) {
      setViewMonth(11);
      setViewYear(viewYear - 1);
    } else {
      setViewMonth(viewMonth - 1);
    }
  };

  const goNext = () => {
    if (viewMonth === 11) {
      setViewMonth(0);
      setViewYear(viewYear + 1);
    } else {
      setViewMonth(viewMonth + 1);
    }
  };

  // Build the 6-row × 7-col day grid. JS Date: getDay() returns 0=Sun..6=Sat.
  // Convert to Monday-first index: (getDay() + 6) % 7  →  0=Mon..6=Sun.
  const gridDays = useMemo(() => {
    const firstOfMonth = new Date(viewYear, viewMonth, 1);
    const firstWeekdayMondayFirst = (firstOfMonth.getDay() + 6) % 7;
    // Start from the Monday on or before the 1st.
    const gridStart = new Date(viewYear, viewMonth, 1 - firstWeekdayMondayFirst);
    const days: { date: Date; inMonth: boolean }[] = [];
    for (let i = 0; i < 42; i++) {
      const d = new Date(gridStart.getFullYear(), gridStart.getMonth(), gridStart.getDate() + i);
      days.push({ date: d, inMonth: d.getMonth() === viewMonth });
    }
    return days;
  }, [viewYear, viewMonth]);

  const selectedIso = value;
  const todayIso = `${today.getFullYear()}-${(today.getMonth() + 1).toString().padStart(2, '0')}-${today.getDate().toString().padStart(2, '0')}`;

  const dayCellBase: React.CSSProperties = {
    width: '28px',
    height: '28px',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    fontSize: '11px',
    border: 'none',
    borderRadius: '4px',
    cursor: 'pointer',
    padding: 0,
    backgroundColor: 'transparent',
  };

  return (
    <div
      ref={containerRef}
      style={{
        position: 'absolute',
        top: '100%',
        left: 0,
        marginTop: '4px',
        backgroundColor: '#2f2f2f',
        border: '1px solid #3a3a3a',
        borderRadius: '6px',
        padding: '8px',
        boxShadow: '0 4px 12px rgba(0,0,0,0.6)',
        zIndex: 70,
        width: '232px',
      }}
      onMouseDown={(e) => {
        // Prevent the parent popover's outside-mousedown listener from closing
        // it when interacting inside the picker. The picker IS a logical
        // child of the popover even though it's positioned outside the
        // popover's DOM subtree (it's absolutely-positioned to the input).
        e.stopPropagation();
      }}
    >
      {/* Header: prev / month-year / next */}
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '6px' }}>
        <button
          type="button"
          onClick={goPrev}
          aria-label="Previous month"
          style={{
            width: '24px',
            height: '24px',
            padding: 0,
            border: 'none',
            backgroundColor: 'transparent',
            color: '#888',
            cursor: 'pointer',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            borderRadius: '4px',
          }}
        >
          <ChevronLeft size={14} />
        </button>
        <span style={{ fontSize: '12px', fontWeight: 500, color: '#ddd' }}>
          {MONTH_NAMES[viewMonth]} {viewYear}
        </span>
        <button
          type="button"
          onClick={goNext}
          aria-label="Next month"
          style={{
            width: '24px',
            height: '24px',
            padding: 0,
            border: 'none',
            backgroundColor: 'transparent',
            color: '#888',
            cursor: 'pointer',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            borderRadius: '4px',
          }}
        >
          <ChevronRight size={14} />
        </button>
      </div>

      {/* Day-of-week header */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(7, 28px)', gap: '2px', marginBottom: '4px' }}>
        {DAY_LABELS.map((label) => (
          <div
            key={label}
            style={{
              width: '28px',
              height: '20px',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              fontSize: '10px',
              fontWeight: 500,
              color: '#888',
              letterSpacing: '0.5px',
              textTransform: 'uppercase',
            }}
          >
            {label}
          </div>
        ))}
      </div>

      {/* 6-row × 7-col day grid */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(7, 28px)', gap: '2px' }}>
        {gridDays.map(({ date, inMonth }) => {
          const iso = `${date.getFullYear()}-${(date.getMonth() + 1).toString().padStart(2, '0')}-${date.getDate().toString().padStart(2, '0')}`;
          const isSelected = iso === selectedIso;
          const isToday = iso === todayIso;
          const cellStyle: React.CSSProperties = {
            ...dayCellBase,
            color: isSelected ? '#fff' : inMonth ? '#ddd' : '#555',
            backgroundColor: isSelected ? '#27ae60' : 'transparent',
            fontWeight: isSelected ? 600 : 400,
            textDecoration: isToday && !isSelected ? 'underline' : 'none',
          };
          return (
            <button
              key={iso}
              type="button"
              style={cellStyle}
              onClick={() => {
                onChange(iso);
                onClose();
              }}
              onMouseEnter={(e) => {
                if (!isSelected) e.currentTarget.style.backgroundColor = '#383838';
              }}
              onMouseLeave={(e) => {
                if (!isSelected) e.currentTarget.style.backgroundColor = 'transparent';
              }}
              aria-label={iso}
              aria-pressed={isSelected}
            >
              {date.getDate()}
            </button>
          );
        })}
      </div>
    </div>
  );
};

// ---- TxFilterDateEditor ----

/**
 * Date filter editor. Apply/Clear commit pattern: typing dates or clicking
 * the inline picker only mutates draft state; the slice is only mutated
 * when the user clicks Apply. Preset pills bypass the draft and apply
 * immediately (existing UX), since they're a deliberate one-click action.
 *
 * Replaces the previous native <input type="date"> with a custom inline
 * month-grid picker so the look matches the dark TWINS theme. Text inputs
 * accept DD/MM/YYYY for manual entry.
 */
export const TxFilterDateEditor: React.FC<TxFilterDateEditorProps> = ({ onClose }) => {
  const { t } = useTranslation('wallet');
  const { dateFilter, dateRangeFrom, dateRangeTo, setDateFilter, setDateRange, applyCustomDateRange } = useTransactions();

  // Draft state: ISO YYYY-MM-DD or ''. Mirror visible text in displayFrom/To
  // so users can type partial values without losing them.
  //
  // Seed inputs from the slice's stored range ONLY when the active filter
  // is 'range'. When a preset (today/week/month/...) or 'all' is active,
  // the slice's dateRangeFrom/To may still hold a previously-applied custom
  // range (preserved deliberately so applyView's preset-views and a
  // subsequent return to 'range' can restore the user's last custom
  // selection — see transactionsSlice.applyView's "range fields are inert"
  // comment). Surfacing those stale bounds in the editor's From/To inputs
  // while a preset is active looks like the filter is mis-applied. Source
  // bug: ?-research-tx-date-filter-presets BUG 2.
  //
  // The editor mounts fresh on each open (TxFilterPopover unmounts on
  // close), so these useState initializers re-run on every open and the
  // gate is re-evaluated against the current dateFilter — no resync
  // useEffect needed.
  const seedFromIso = dateFilter === 'range' ? dateRangeFrom : '';
  const seedToIso = dateFilter === 'range' ? dateRangeTo : '';
  const [draftFromIso, setDraftFromIso] = useState(seedFromIso);
  const [draftToIso, setDraftToIso] = useState(seedToIso);
  const [displayFrom, setDisplayFrom] = useState(isoToDisplay(seedFromIso));
  const [displayTo, setDisplayTo] = useState(isoToDisplay(seedToIso));
  const [errorFrom, setErrorFrom] = useState(false);
  const [errorTo, setErrorTo] = useState(false);
  const [pickerOpen, setPickerOpen] = useState<null | 'from' | 'to'>(null);

  // Resolve the browser's IANA timezone once on mount. Surfaced as a subtitle
  // below to disambiguate the bug context from research
  // ?-research-tx-date-filter-timezone: ISO dates are parsed in the daemon's
  // local TZ via time.ParseInLocation(..., time.Local). On a desktop wallet
  // server-local == user-local, so the resolved browser TZ is correct.
  // Empty string fallback hides the subtitle in headless environments.
  const resolvedTz = useMemo(() => {
    try {
      return Intl.DateTimeFormat().resolvedOptions().timeZone || '';
    } catch {
      return '';
    }
  }, []);

  const handlePresetClick = (preset: Exclude<DateFilter, 'range'>) => {
    setDateFilter(preset);
    onClose();
  };

  // Parse display string on blur and update draft + error state. Empty input
  // is valid (means "no bound").
  const commitDisplayFrom = () => {
    if (!displayFrom.trim()) {
      setDraftFromIso('');
      setErrorFrom(false);
      return;
    }
    const iso = displayToIso(displayFrom);
    if (iso) {
      setDraftFromIso(iso);
      setDisplayFrom(isoToDisplay(iso));
      setErrorFrom(false);
    } else {
      setErrorFrom(true);
    }
  };

  const commitDisplayTo = () => {
    if (!displayTo.trim()) {
      setDraftToIso('');
      setErrorTo(false);
      return;
    }
    const iso = displayToIso(displayTo);
    if (iso) {
      setDraftToIso(iso);
      setDisplayTo(isoToDisplay(iso));
      setErrorTo(false);
    } else {
      setErrorTo(true);
    }
  };

  const handleApply = () => {
    // Always re-parse the visible display text when non-empty — do NOT
    // short-circuit on the existing `draftFromIso` / `draftToIso`. The drafts
    // can be stale if the user edited the text input without blurring before
    // clicking Apply: the input's onChange updates `displayFrom`/`displayTo`
    // but intentionally leaves the ISO draft untouched (so partial typing
    // doesn't clobber a previously-committed value mid-edit). Falling back to
    // the stored ISO is only correct when the display field is empty (e.g.
    // user opened the editor with no prior range set and only used the
    // calendar picker — picker writes both display and ISO via onChange).
    let fromIso: string;
    let toIso: string;
    if (displayFrom.trim()) {
      const parsed = displayToIso(displayFrom);
      if (!parsed) {
        setErrorFrom(true);
        return;
      }
      fromIso = parsed;
    } else {
      fromIso = '';
    }
    if (displayTo.trim()) {
      const parsed = displayToIso(displayTo);
      if (!parsed) {
        setErrorTo(true);
        return;
      }
      toIso = parsed;
    } else {
      toIso = '';
    }
    if (fromIso && toIso) {
      applyCustomDateRange(fromIso, toIso);
      onClose();
    }
    // If only one is set, no-op silently — Apply requires both bounds for
    // a range. (User can still pick a preset for open-ended ranges.)
  };

  const handleClear = () => {
    setDraftFromIso('');
    setDraftToIso('');
    setDisplayFrom('');
    setDisplayTo('');
    setErrorFrom(false);
    setErrorTo(false);
    // Reset to 'all' preset AND clear the stored custom range bounds. The
    // bounds are not consumed when dateFilter !== 'range', but they DO seed
    // the draft state on the next editor open — leaving them stale would
    // resurrect the previously-cleared dates the next time the user opens
    // the date filter.
    //
    // Order matters: setDateFilter('all') FIRST, then setDateRange('', '').
    // Both setters dispatch fetchPage(1) independently. If setDateRange ran
    // first, fetch #1 would race with dateFilter still === 'range' but
    // dateRangeFrom/To === '' — an inconsistent transient state. By flipping
    // to 'all' first, BOTH fetches see dateFilter === 'all' (which causes
    // the backend to ignore the range bounds entirely), so whichever arrives
    // last lands on a correct empty result regardless of network ordering.
    setDateFilter('all');
    setDateRange('', '');
    onClose();
  };

  const inputErrorStyle = (hasError: boolean): React.CSSProperties => ({
    ...inputStyle,
    borderColor: hasError ? '#ff6666' : '#3a3a3a',
  });

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '12px', minWidth: '260px' }}>
      {resolvedTz && (
        <div style={{ color: '#888', fontSize: '11px', lineHeight: 1.4 }}>
          {t('transactions.filters.chip.localTzHint', { tz: resolvedTz })}
        </div>
      )}
      <div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: '6px' }}>
          {PRESETS.map((preset) => (
            <button
              key={preset}
              type="button"
              style={dateFilter === preset ? pillSelected : pillBase}
              onClick={() => handlePresetClick(preset)}
            >
              {t(`transactions.filters.chip.datePresets.${PRESET_I18N_KEY[preset]}`)}
            </button>
          ))}
        </div>
      </div>

      <div style={{ height: '1px', backgroundColor: '#3a3a3a' }} />

      {/* A5: visible 'From' label above the From input, mirroring the existing 'To' label below.
          Renders as a span (not <label htmlFor>) to mirror the existing 'To' markup exactly; the
          From input still carries an aria-label so screen readers receive the same association. */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
        <span style={{ color: '#888', fontSize: '11px' }}>
          {t('transactions.filters.chip.customRangeFrom')}
        </span>
      </div>

      {/* From row */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', position: 'relative' }}>
        <input
          type="text"
          placeholder="DD/MM/YYYY"
          style={inputErrorStyle(errorFrom)}
          value={displayFrom}
          onChange={(e) => {
            setDisplayFrom(e.target.value);
            if (errorFrom) setErrorFrom(false);
          }}
          onBlur={commitDisplayFrom}
          autoCapitalize="off"
          autoCorrect="off"
          spellCheck={false}
          autoComplete="off"
          aria-label={t('transactions.filters.chip.customRangeFrom')}
        />
        <button
          type="button"
          style={calendarIconBtn}
          onClick={() => setPickerOpen(pickerOpen === 'from' ? null : 'from')}
          aria-label="Open calendar"
          onMouseEnter={(e) => {
            e.currentTarget.style.borderColor = '#555';
            e.currentTarget.style.color = '#ddd';
          }}
          onMouseLeave={(e) => {
            e.currentTarget.style.borderColor = '#3a3a3a';
            e.currentTarget.style.color = '#888';
          }}
        >
          <CalendarIcon size={14} />
        </button>
        {pickerOpen === 'from' && (
          <InlineDatePicker
            value={draftFromIso}
            onChange={(iso) => {
              setDraftFromIso(iso);
              setDisplayFrom(isoToDisplay(iso));
              setErrorFrom(false);
            }}
            onClose={() => setPickerOpen(null)}
          />
        )}
      </div>

      <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
        <span style={{ color: '#888', fontSize: '11px' }}>
          {t('transactions.filters.chip.customRangeTo')}
        </span>
      </div>

      {/* To row */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', position: 'relative' }}>
        <input
          type="text"
          placeholder="DD/MM/YYYY"
          style={inputErrorStyle(errorTo)}
          value={displayTo}
          onChange={(e) => {
            setDisplayTo(e.target.value);
            if (errorTo) setErrorTo(false);
          }}
          onBlur={commitDisplayTo}
          autoCapitalize="off"
          autoCorrect="off"
          spellCheck={false}
          autoComplete="off"
          aria-label={t('transactions.filters.chip.customRangeTo')}
        />
        <button
          type="button"
          style={calendarIconBtn}
          onClick={() => setPickerOpen(pickerOpen === 'to' ? null : 'to')}
          aria-label="Open calendar"
          onMouseEnter={(e) => {
            e.currentTarget.style.borderColor = '#555';
            e.currentTarget.style.color = '#ddd';
          }}
          onMouseLeave={(e) => {
            e.currentTarget.style.borderColor = '#3a3a3a';
            e.currentTarget.style.color = '#888';
          }}
        >
          <CalendarIcon size={14} />
        </button>
        {pickerOpen === 'to' && (
          <InlineDatePicker
            value={draftToIso}
            onChange={(iso) => {
              setDraftToIso(iso);
              setDisplayTo(isoToDisplay(iso));
              setErrorTo(false);
            }}
            onClose={() => setPickerOpen(null)}
          />
        )}
      </div>

      {dateFilter === 'range' && (
        <div style={{ fontSize: '10px', color: '#27ae60' }}>
          {t('transactions.filters.chip.customRangeActive')}
        </div>
      )}

      {/* Apply / Clear footer — matches TxFilterAmountEditor convention */}
      <div style={{ display: 'flex', gap: '8px', justifyContent: 'flex-end' }}>
        <button type="button" style={secondaryButton} onClick={handleClear}>
          {t('transactions.filters.chip.amountClear')}
        </button>
        <button type="button" style={primaryButton} onClick={handleApply}>
          {t('transactions.filters.chip.amountApply')}
        </button>
      </div>
    </div>
  );
};
