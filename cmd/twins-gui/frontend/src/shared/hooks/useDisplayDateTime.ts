import { useCallback } from 'react';
import { useStore } from '@/store/useStore';
import {
  toDate,
  formatAbsoluteLocal,
  formatAbsoluteLocalNoTz,
  formatAbsoluteLocalShort,
  formatAbsoluteUTC,
  formatAbsoluteUTCNoTz,
  formatAbsoluteUTCShort,
  formatAgeShort,
  formatTooltipFor,
  getLocalTzAbbrev,
  type DateInput,
} from './useDisplayDateTime.helpers';

/**
 * Date format constants — match backend `preferences.DateFormat*` enum and
 * the `nDateDisplayFormat` GUISetting value.
 */
export const DATE_FORMAT_LOCAL = 0;
export const DATE_FORMAT_UTC = 1;
export const DATE_FORMAT_AGE = 2;

export type { DateInput };

/**
 * Hook providing the user-selected date/age display format and reactive
 * formatter helpers. Pages and table cells should consume this hook so a
 * change to the global date setting re-renders every consumer.
 *
 * **Two value formatters**:
 *  - `formatDateTime(ts)` — full form WITH timezone token inline. Use in
 *    places without a table column header (hero rows, ledger rows, single
 *    date displays). Example: `2026-06-04 10:38:10 GMT+2`.
 *  - `formatDateTimeShort(ts)` — narrow form WITHOUT year, seconds, or TZ.
 *    Use in narrow table cells (~80-130px). Example: `06-04 10:38`. The TZ
 *    moves to the column header via `formatTzSuffix()` so per-row noise is
 *    eliminated. In Age mode both formatters return the same compact
 *    `5m ago` form — relative time is already short.
 *
 * **Header helpers**:
 *  - `formatDateHeader()` — generic table header: `Date (GMT+2)` /
 *    `Date (UTC)` / `Age`. Use for columns whose canonical label is just
 *    "Date".
 *  - `formatTzSuffix()` — TZ token to append to domain-specific headers
 *    like `Last Seen (GMT+2)` / `Last Paid (UTC)` / `Connection Time`. In
 *    Age mode returns empty string (Age has no TZ semantics).
 *
 * **Known DST trade-off on headers**: both header helpers resolve the local
 * TZ via `getLocalTzAbbrev()` with no date argument — i.e. "now's offset".
 * Per-row values via `formatDateTime(ts)` / `formatDateTimeShort(ts)` use the
 * row's actual TZ (correct seasonal DST handling). When a table contains
 * rows spanning a DST cutoff, the column header shows the CURRENT offset
 * while individual rows show their HISTORICAL offset. Acceptable because
 * (a) headers are inherently "right now" — they describe the column's
 * current rendering context, (b) per-row values carry their own correct TZ
 * inline in full form, and (c) the tooltip is always UTC for unambiguous
 * reference. Documenting rather than papering over with a complex header
 * derivation.
 *
 * **Tooltip**:
 *  - `formatTooltip(ts)` — returns the OPPOSITE representation of the
 *    current mode so the tooltip never duplicates the visible cell value:
 *    Local mode → UTC tooltip; UTC mode → Local tooltip; Age mode → UTC
 *    tooltip (Age has no clock token, so UTC is the most useful
 *    unambiguous reference time on hover).
 */
export function useDisplayDateTime() {
  const dateFormat = useStore(state => state.dateFormat);

  const formatDateTime = useCallback(
    (input: DateInput): string => {
      const date = toDate(input);
      if (!date) return '';
      switch (dateFormat) {
        case DATE_FORMAT_UTC:
          return formatAbsoluteUTC(date);
        case DATE_FORMAT_AGE:
          return formatAgeShort(date);
        case DATE_FORMAT_LOCAL:
        default:
          return formatAbsoluteLocal(date);
      }
    },
    [dateFormat]
  );

  const formatDateTimeShort = useCallback(
    (input: DateInput): string => {
      const date = toDate(input);
      if (!date) return '';
      switch (dateFormat) {
        case DATE_FORMAT_UTC:
          return formatAbsoluteUTCShort(date);
        case DATE_FORMAT_AGE:
          return formatAgeShort(date);
        case DATE_FORMAT_LOCAL:
        default:
          return formatAbsoluteLocalShort(date);
      }
    },
    [dateFormat]
  );

  /**
   * Full ISO-like value WITHOUT timezone token — `YYYY-MM-DD HH:MM:SS` in
   * Local/UTC modes, `5m ago` in Age mode. Use this in table cells where the
   * TZ has been hoisted into the column header via `formatDateHeader()` — the
   * per-row TZ repetition would otherwise be visual noise. The tooltip
   * (`formatTooltip`) still shows the opposite representation with the TZ
   * suffix so users can hover-disambiguate Local vs UTC.
   */
  const formatDateValue = useCallback(
    (input: DateInput): string => {
      const date = toDate(input);
      if (!date) return '';
      switch (dateFormat) {
        case DATE_FORMAT_UTC:
          return formatAbsoluteUTCNoTz(date);
        case DATE_FORMAT_AGE:
          return formatAgeShort(date);
        case DATE_FORMAT_LOCAL:
        default:
          return formatAbsoluteLocalNoTz(date);
      }
    },
    [dateFormat]
  );

  const formatTooltip = useCallback(
    (input: DateInput): string => {
      const date = toDate(input);
      if (!date) return '';
      return formatTooltipFor(dateFormat, date);
    },
    [dateFormat]
  );

  const formatDateHeader = useCallback((): string => {
    if (dateFormat === DATE_FORMAT_AGE) return 'Age';
    if (dateFormat === DATE_FORMAT_UTC) return 'Date (UTC)';
    const tz = getLocalTzAbbrev();
    return tz ? `Date (${tz})` : 'Date';
  }, [dateFormat]);

  /**
   * Returns `' (GMT+2)'` / `' (UTC)'` / `''` for appending to domain-specific
   * header labels. Includes the leading space so consumers can write
   * `Last Seen${formatTzSuffix()}` without thinking about whitespace.
   */
  const formatTzSuffix = useCallback((): string => {
    if (dateFormat === DATE_FORMAT_AGE) return '';
    if (dateFormat === DATE_FORMAT_UTC) return ' (UTC)';
    const tz = getLocalTzAbbrev();
    return tz ? ` (${tz})` : '';
  }, [dateFormat]);

  return {
    dateFormat,
    formatDateTime,
    formatDateTimeShort,
    formatDateValue,
    formatTooltip,
    formatDateHeader,
    formatTzSuffix,
  };
}
