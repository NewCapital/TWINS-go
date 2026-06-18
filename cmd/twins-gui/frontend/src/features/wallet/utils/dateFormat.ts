// Date format helpers shared by the Transactions date-filter editor and its
// inline date picker. ISO YYYY-MM-DD is the slice's storage format; DD/MM/YYYY
// is the visible text-input format (European locale convention, matches the
// existing dd/mm/yyyy hint in the chip-bar context).

const ISO_RE = /^(\d{4})-(\d{2})-(\d{2})$/;
const DISPLAY_RE = /^(\d{1,2})\/(\d{1,2})\/(\d{4})$/;

/**
 * Convert an ISO YYYY-MM-DD date to display DD/MM/YYYY form. Returns ''
 * for empty/invalid input (callers should treat '' as "no date set").
 */
export const isoToDisplay = (iso: string): string => {
  if (!iso) return '';
  const m = iso.match(ISO_RE);
  if (!m) return '';
  const [, y, mo, d] = m;
  return `${d}/${mo}/${y}`;
};

/**
 * Parse a DD/MM/YYYY string into an ISO YYYY-MM-DD string, or null if the
 * input is malformed or names an invalid calendar date.
 *
 * Validation rules:
 * - Whitespace trimmed before parsing.
 * - Day and month allow 1 or 2 digits; year must be exactly 4.
 * - Month must be 1-12.
 * - Day must be 1-N where N is the actual number of days in the given
 *   month (rejects e.g. 31/02/2026, 30/02/2024).
 *
 * Returns ISO with zero-padded month and day.
 */
export const displayToIso = (display: string): string | null => {
  const m = display.trim().match(DISPLAY_RE);
  if (!m) return null;
  const [, dStr, moStr, yStr] = m;
  const d = parseInt(dStr, 10);
  const mo = parseInt(moStr, 10);
  const y = parseInt(yStr, 10);
  if (mo < 1 || mo > 12) return null;
  // Trick: `new Date(year, month, 0)` returns the last day of the previous
  // month, so passing `mo` (1-12) here gives us the last day of month `mo`.
  const daysInMonth = new Date(y, mo, 0).getDate();
  if (d < 1 || d > daysInMonth) return null;
  return `${y.toString().padStart(4, '0')}-${mo.toString().padStart(2, '0')}-${d.toString().padStart(2, '0')}`;
};
