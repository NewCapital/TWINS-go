// Fallback block-explorer URL used when the user has not configured
// `strThirdPartyTxUrls` in Settings → Display → Third-party transaction URLs.
// `%s` is replaced with the (URL-encoded) txid at click time.
//
// Kept here so the Transactions page context menu and the Transaction Details
// dialog share a single source of truth — if the legacy fallback ever needs
// to change (e.g. domain migration), update it once.
export const LEGACY_EXPLORER_TX_FALLBACK = 'https://explorer.win.win/tx/%s';

/**
 * Build a usable explorer URL from a `%s`-templated string and a value.
 * The value is URL-encoded so it can never break out of the path segment.
 */
export function buildExplorerURL(template: string, value: string): string {
  return template.replace('%s', encodeURIComponent(value));
}
