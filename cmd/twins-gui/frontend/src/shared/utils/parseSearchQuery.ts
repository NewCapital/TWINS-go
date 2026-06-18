/**
 * Parses a transaction-search input string and classifies it into a structured
 * filter directive. Used by the Transactions page filter bar to auto-detect
 * TXID / address / min-amount queries and convert them to filter chips on
 * Enter, instead of treating every input as opaque search text.
 *
 * Phase 1 of the chip-based filter bar migration. See
 * `team-management/tasks/done/?-research-transactions-filters-architecture.md`.
 */

export type ParsedQuery =
  | { type: 'address'; value: string }
  | { type: 'min_amount'; value: number }
  | { type: 'search'; value: string };

// TXID detection was removed in code review round 2 (Codex critical finding):
// the backend `matchesSearchText` (wallet.go:1587) only matches address+label
// substrings, not TXID. Phase 1 is frontend-only, so dropping TXID detection is
// the correct fix until backend gains a dedicated TXID filter (deferred).
//
// TWINS address regex: 'W'/'a'/'m'/'n' prefix + 33 Base58 chars. Canonical
// regex used by Send.tsx, AddressBookDialog.tsx, RecipientField.tsx, and
// TxFilterBar.tsx (the chip label formatter). 'D' prefix was a stale doc
// reference and is not a valid TWINS address prefix on any current network.
const TWINS_ADDRESS_REGEX = /^[Wamn][a-km-zA-HJ-NP-Z1-9]{33}$/;
// Matches `>N`, `>=N`, optional whitespace, decimal or integer.
const MIN_AMOUNT_REGEX = /^>=?\s*(\d+(?:\.\d+)?)$/;

// Matches a bare positive number (integer or decimal) with no operator prefix.
// Used by the search input UX to detect when the user typed `100` (which the
// backend search treats as an address/label substring scan) and prompt them
// toward the `>100` min-amount syntax instead. Kept in sync with
// MIN_AMOUNT_REGEX's capture group: both accept `100`, `0.5`, and reject
// `100.`, leading dots, leading `+`, scientific notation.
const BARE_POSITIVE_NUMBER_REGEX = /^\d+(?:\.\d+)?$/;

export function isBarePositiveNumber(query: string): boolean {
  const trimmed = query.trim();
  if (!BARE_POSITIVE_NUMBER_REGEX.test(trimmed)) return false;
  return parseFloat(trimmed) > 0;
}

export function parseSearchQuery(query: string): ParsedQuery {
  const trimmed = query.trim();

  if (TWINS_ADDRESS_REGEX.test(trimmed)) {
    return { type: 'address', value: trimmed };
  }

  const minMatch = trimmed.match(MIN_AMOUNT_REGEX);
  if (minMatch) {
    const n = parseFloat(minMatch[1]);
    if (n > 0) {
      return { type: 'min_amount', value: n };
    }
  }

  return { type: 'search', value: trimmed };
}
