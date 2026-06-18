/**
 * Detects an inverted From/To amount range in the TxFilterAmountEditor.
 *
 * Returns true when BOTH bounds are non-empty AND From > To. Empty inputs on
 * either side are treated as "unbounded" by the backend filter (see
 * internal/wallet/wallet.go `matchesAmountFilter`), so a single bound is
 * always valid — only the explicit "From > To with both set" case is rejected.
 *
 * Parsing follows the same number coercion as the existing chip-bar dispatch
 * (`parseFloat`). NaN inputs (non-numeric strings the native number input
 * shouldn't allow, but defense-in-depth) are treated as "not inverted" — the
 * Apply path elsewhere already guards against NaN via empty-string checks.
 */
export function isAmountRangeInverted(from: string, to: string): boolean {
  if (from === '' || to === '') return false;
  const fromNum = parseFloat(from);
  const toNum = parseFloat(to);
  if (Number.isNaN(fromNum) || Number.isNaN(toNum)) return false;
  return fromNum > toNum;
}
