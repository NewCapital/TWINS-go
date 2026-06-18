/**
 * Pure date formatters consumed by `useDisplayDateTime`. Extracted into a
 * standalone module so unit tests can import them WITHOUT pulling in the
 * Zustand store (which transitively imports `@wailsjs/go/models` and fails
 * to resolve under vitest).
 */

export type DateInput = number | string | Date | null | undefined;

export function toDate(input: DateInput): Date | null {
  if (input == null) return null;
  if (input instanceof Date) {
    return isNaN(input.getTime()) ? null : input;
  }
  if (typeof input === 'number') {
    // Unix seconds (backend convention) — heuristic: if < 1e12 treat as seconds.
    const ms = input < 1e12 ? input * 1000 : input;
    const date = new Date(ms);
    return isNaN(date.getTime()) || date.getUTCFullYear() <= 1970 ? null : date;
  }
  if (typeof input === 'string') {
    const trimmed = input.trim();
    if (!trimmed) return null;
    const date = new Date(trimmed);
    return isNaN(date.getTime()) || date.getUTCFullYear() <= 1970 ? null : date;
  }
  return null;
}

/**
 * Local-timezone absolute date — `YYYY-MM-DD HH:MM:SS GMT+N` (ISO-like form
 * matching the structure of `formatAbsoluteUTC`, just with the user's local
 * timezone instead of UTC). Both modes share the same visual shape — the
 * only difference is the TZ token. Hardcoded en-US locale so the GMT-offset
 * resolves as a numeric suffix rather than a localized abbreviation.
 */
export function formatAbsoluteLocal(date: Date): string {
  const y = date.getFullYear();
  const mo = String(date.getMonth() + 1).padStart(2, '0');
  const d = String(date.getDate()).padStart(2, '0');
  const h = String(date.getHours()).padStart(2, '0');
  const m = String(date.getMinutes()).padStart(2, '0');
  const s = String(date.getSeconds()).padStart(2, '0');
  const base = `${y}-${mo}-${d} ${h}:${m}:${s}`;
  // `getLocalTzAbbrev` may return '' on environments where Intl is partially
  // polyfilled or strips the `timeZoneName` part. Guard the trailing space so
  // the rendered value never carries a stray whitespace token.
  const tz = getLocalTzAbbrev(date);
  return tz ? `${base} ${tz}` : base;
}

/**
 * Short local-timezone date — `MM-DD HH:MM` (no year, no seconds, no TZ).
 * Used for narrow table cells (~78px @ 12px monospace). The TZ moves to the
 * column header via `formatTzSuffix()` so per-row noise is eliminated.
 */
export function formatAbsoluteLocalShort(date: Date): string {
  const mo = String(date.getMonth() + 1).padStart(2, '0');
  const d = String(date.getDate()).padStart(2, '0');
  const h = String(date.getHours()).padStart(2, '0');
  const m = String(date.getMinutes()).padStart(2, '0');
  return `${mo}-${d} ${h}:${m}`;
}

/**
 * Short UTC date — `MM-DD HH:MM` (no year, no seconds, no UTC suffix).
 * Symmetric with `formatAbsoluteLocalShort` for narrow table cells.
 */
export function formatAbsoluteUTCShort(date: Date): string {
  const mo = String(date.getUTCMonth() + 1).padStart(2, '0');
  const d = String(date.getUTCDate()).padStart(2, '0');
  const h = String(date.getUTCHours()).padStart(2, '0');
  const m = String(date.getUTCMinutes()).padStart(2, '0');
  return `${mo}-${d} ${h}:${m}`;
}

/**
 * Local timezone abbreviation as a GMT-offset token (e.g. `GMT+2`, `GMT-5`,
 * `GMT+5:30`). Uses `Intl.DateTimeFormat` with `'en-US'` locale so the
 * `timeZoneName: 'short'` token resolves as a numeric GMT offset rather
 * than a localized abbreviation like "CEST" or "EST". Returns empty string
 * on environments where the API doesn't surface the timezone part.
 *
 * Accepts an optional `date` parameter (defaults to "now") because the
 * offset varies seasonally (DST). The caller passes the date being formatted
 * so the rendered TZ matches the date's actual offset.
 */
export function getLocalTzAbbrev(date: Date = new Date()): string {
  try {
    const parts = new Intl.DateTimeFormat('en-US', { timeZoneName: 'short' }).formatToParts(date);
    return parts.find(p => p.type === 'timeZoneName')?.value ?? '';
  } catch {
    return '';
  }
}

/**
 * Tooltip dispatcher — returns the OPPOSITE representation of the current
 * `dateFormat` so the tooltip never duplicates the visible cell value.
 *
 * - `DATE_FORMAT_LOCAL` (0)  →  UTC tooltip (`YYYY-MM-DD HH:MM:SS UTC`)
 * - `DATE_FORMAT_UTC`   (1)  →  Local tooltip (`YYYY-MM-DD HH:MM:SS GMT+N`)
 * - `DATE_FORMAT_AGE`   (2)  →  UTC tooltip (Age has no clock token; the
 *                               unambiguous UTC reference time is the
 *                               most useful hover affordance)
 *
 * Kept as a pure helper (no Zustand, no React) so the inversion logic is
 * testable in isolation. Consumed by the hook's `formatTooltip` callback.
 */
export function formatTooltipFor(dateFormat: number, date: Date): string {
  // dateFormat constants are duplicated here as numeric literals (rather than
  // imported from useDisplayDateTime.ts) so this helpers module stays free of
  // Zustand transitive imports — keeps it cleanly testable under vitest.
  if (dateFormat === 1) return formatAbsoluteLocal(date); // UTC mode → local tooltip
  return formatAbsoluteUTC(date); // Local + Age modes → UTC tooltip
}

/**
 * UTC absolute date — `YYYY-MM-DD HH:MM:SS UTC`. Used in UTC mode as the
 * visible cell value AND as the hover-tooltip reference in Local/Age modes.
 */
export function formatAbsoluteUTC(date: Date): string {
  const y = date.getUTCFullYear();
  const mo = String(date.getUTCMonth() + 1).padStart(2, '0');
  const d = String(date.getUTCDate()).padStart(2, '0');
  const h = String(date.getUTCHours()).padStart(2, '0');
  const m = String(date.getUTCMinutes()).padStart(2, '0');
  const s = String(date.getUTCSeconds()).padStart(2, '0');
  return `${y}-${mo}-${d} ${h}:${m}:${s} UTC`;
}

/**
 * Local absolute date WITHOUT timezone token — `YYYY-MM-DD HH:MM:SS`.
 * Used by `formatDateValue` for table cells where the TZ has been hoisted
 * into the column header (via `formatDateHeader()`). The full form (year +
 * seconds) is preserved; only the trailing ` GMT+N` is dropped vs the
 * canonical `formatAbsoluteLocal`.
 */
export function formatAbsoluteLocalNoTz(date: Date): string {
  const y = date.getFullYear();
  const mo = String(date.getMonth() + 1).padStart(2, '0');
  const d = String(date.getDate()).padStart(2, '0');
  const h = String(date.getHours()).padStart(2, '0');
  const m = String(date.getMinutes()).padStart(2, '0');
  const s = String(date.getSeconds()).padStart(2, '0');
  return `${y}-${mo}-${d} ${h}:${m}:${s}`;
}

/**
 * UTC absolute date WITHOUT timezone token — `YYYY-MM-DD HH:MM:SS`.
 * Symmetric with `formatAbsoluteLocalNoTz` for UTC display mode. The trailing
 * ` UTC` literal is dropped vs the canonical `formatAbsoluteUTC` because the
 * column header already announces UTC via `formatDateHeader()`.
 */
export function formatAbsoluteUTCNoTz(date: Date): string {
  const y = date.getUTCFullYear();
  const mo = String(date.getUTCMonth() + 1).padStart(2, '0');
  const d = String(date.getUTCDate()).padStart(2, '0');
  const h = String(date.getUTCHours()).padStart(2, '0');
  const m = String(date.getUTCMinutes()).padStart(2, '0');
  const s = String(date.getUTCSeconds()).padStart(2, '0');
  return `${y}-${mo}-${d} ${h}:${m}:${s}`;
}

/**
 * Relative-age short form — `<1m ago` / `Nm ago` / `Nh ago` / `Nd ago` /
 * `Nmo ago` / `Xy ago` / `Xy Ymo ago`. Approximate calendar arithmetic
 * (30 days = 1 month, 365 days = 1 year) — acceptable trade-off because
 * the value is a freshness signal, not a precise duration.
 */
export function formatAgeShort(date: Date): string {
  const diffSec = Math.max(0, Math.floor((Date.now() - date.getTime()) / 1000));
  if (diffSec < 60) return '<1m ago';
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m ago`;
  if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h ago`;
  const days = Math.floor(diffSec / 86400);
  if (days < 30) return `${days}d ago`;
  if (days < 365) return `${Math.floor(days / 30)}mo ago`;
  const years = Math.floor(days / 365);
  const remMonths = Math.floor((days - years * 365) / 30);
  return remMonths > 0 ? `${years}y ${remMonths}mo ago` : `${years}y ago`;
}
