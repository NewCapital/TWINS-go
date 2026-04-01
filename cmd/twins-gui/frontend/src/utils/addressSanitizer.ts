/**
 * Address sanitization utilities for TWINS wallet
 * Provides security against injection attacks and malformed input
 */

// Maximum allowed length for a TWINS address
const MAX_ADDRESS_LENGTH = 35;

// Valid Base58 characters (excludes 0, O, I, l to avoid confusion)
const BASE58_REGEX = /^[1-9A-HJ-NP-Za-km-z]*$/;

/**
 * Sanitizes a TWINS address input
 * - Trims whitespace
 * - Limits length to prevent overflow
 * - Removes invalid characters
 * - Prevents injection attacks
 */
export function sanitizeAddress(address: string): string {
  if (!address || typeof address !== 'string') {
    return '';
  }

  // Trim whitespace
  let sanitized = address.trim();

  // Limit length to prevent overflow attacks
  if (sanitized.length > MAX_ADDRESS_LENGTH) {
    sanitized = sanitized.substring(0, MAX_ADDRESS_LENGTH);
  }

  // Remove any non-Base58 characters to prevent injection
  // This keeps only valid characters for TWINS addresses
  const chars = sanitized.split('');
  const validChars = chars.filter(char => {
    return BASE58_REGEX.test(char);
  });

  return validChars.join('');
}

/**
 * Validates that an address contains only safe characters
 * Returns true if the address is safe to send to backend
 */
export function isAddressSafe(address: string): boolean {
  if (!address || typeof address !== 'string') {
    return false;
  }

  // Check length
  if (address.length > MAX_ADDRESS_LENGTH) {
    return false;
  }

  // Check that all characters are valid Base58
  return BASE58_REGEX.test(address);
}

/**
 * Checks if an address starts with a valid TWINS prefix
 * Mainnet: D (legacy) or T (new format)
 * Testnet: x or y
 */
export function hasValidPrefix(address: string): boolean {
  if (!address || address.length === 0) {
    return false;
  }

  const firstChar = address[0];
  return ['D', 'T', 'x', 'y'].includes(firstChar);
}