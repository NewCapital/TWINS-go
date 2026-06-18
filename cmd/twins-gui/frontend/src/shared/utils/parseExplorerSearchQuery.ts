/**
 * Classifies an Explorer search-bar input string into a discriminated union
 * so the SearchBar can render a real-time type-badge and the page can pick
 * type-aware not-found copy. Mirrors the backend SearchExplorer first-wins
 * dispatch order (internal/gui/core/go_client.go:767-814) — but is tighter:
 * the backend accepts permissive 26-35 char addresses via DecodeAddress, the
 * frontend only classifies the canonical 34-char W/m/n/a-prefixed form so the
 * type-badge stays accurate.
 *
 * Classification priority:
 *   1. empty / whitespace-only → invalid:empty
 *   2. all-digits, fits uint32 → block_height
 *   3. all-digits, overflows uint32 → invalid:overflow_height
 *   4. 64 hex chars → block_or_tx_hash (backend disambiguates first-wins)
 *   5. TWINS address regex → address
 *   6. 1-63 hex chars → invalid:short_hash
 *   7. 65+ hex chars → invalid:long_hash
 *   8. fallthrough → invalid:unknown
 *
 * The TWINS_ADDRESS_REGEX is intentionally duplicated here (not imported from
 * parseSearchQuery.ts) to keep the Tx-search and Explorer-search classifiers
 * independent — they have different type-sets and different edge cases. A
 * future cross-cutting task can dedupe the regex across all four call sites
 * (Send.tsx, AddressBookDialog.tsx, RecipientField.tsx, parseSearchQuery.ts).
 */

const TWINS_ADDRESS_REGEX = /^[Wamn][a-km-zA-HJ-NP-Z1-9]{33}$/;
const HEX_REGEX = /^[a-fA-F0-9]+$/;
const DIGITS_REGEX = /^\d+$/;
const BLOCK_HEIGHT_MAX = 4294967295; // uint32 max — matches backend strconv.ParseUint(s, 10, 32)
const HASH_LENGTH = 64;

export type ExplorerQueryClassification =
  | { type: 'block_height'; value: number }
  | { type: 'block_or_tx_hash'; value: string }
  | { type: 'address'; value: string }
  | { type: 'invalid'; reason: 'empty' | 'short_hash' | 'long_hash' | 'overflow_height' | 'unknown' };

export function classifyExplorerQuery(query: string): ExplorerQueryClassification {
  const trimmed = query.trim();

  if (trimmed.length === 0) {
    return { type: 'invalid', reason: 'empty' };
  }

  if (DIGITS_REGEX.test(trimmed)) {
    // Use Number to detect overflow against the uint32 ceiling. Inputs at the
    // boundary (4294967295) are valid; anything larger overflows the backend
    // strconv.ParseUint(s, 10, 32) and would silently fall through to the hash
    // branch — surface it explicitly so the type-badge can flag it.
    const n = Number(trimmed);
    if (n <= BLOCK_HEIGHT_MAX) {
      return { type: 'block_height', value: n };
    }
    return { type: 'invalid', reason: 'overflow_height' };
  }

  if (trimmed.length === HASH_LENGTH && HEX_REGEX.test(trimmed)) {
    return { type: 'block_or_tx_hash', value: trimmed };
  }

  if (TWINS_ADDRESS_REGEX.test(trimmed)) {
    return { type: 'address', value: trimmed };
  }

  if (HEX_REGEX.test(trimmed)) {
    if (trimmed.length < HASH_LENGTH) {
      return { type: 'invalid', reason: 'short_hash' };
    }
    return { type: 'invalid', reason: 'long_hash' };
  }

  return { type: 'invalid', reason: 'unknown' };
}
