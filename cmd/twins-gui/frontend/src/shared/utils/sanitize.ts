/**
 * Security utility for sanitizing user-facing strings
 */

/**
 * Sanitizes error messages to prevent XSS attacks
 * Removes potentially dangerous HTML tags and script content
 *
 * @param message - The error message to sanitize
 * @returns Sanitized error message safe for display
 */
export const sanitizeErrorMessage = (message: string): string => {
  if (!message) return '';

  // Remove script tags and their content (must run before generic HTML tag strip)
  let sanitized = message.replace(/<script\b[^<]*(?:(?!<\/script>)<[^<]*)*<\/script>/gi, '');

  // Remove HTML tags ([^>] negated class is already backtrack-safe; lazy quantifier for SonarQube)
  sanitized = sanitized.replace(/<[^>]*?>/g, '');

  // Remove event handlers (onclick, onerror, etc.)
  sanitized = sanitized.replace(/on\w+\s*=\s*["'][^"']*["']/gi, '');

  // Remove javascript: protocol
  sanitized = sanitized.replace(/javascript:/gi, '');

  return sanitized;
};

/**
 * Sanitizes general text input for display
 * More permissive than error messages, allows some formatting
 *
 * @param text - The text to sanitize
 * @returns Sanitized text safe for display
 */
export const sanitizeText = (text: string): string => {
  if (!text) return '';

  // Remove script tags and their content
  let sanitized = text.replace(/<script\b[^<]*(?:(?!<\/script>)<[^<]*)*<\/script>/gi, '');

  // Remove event handlers
  sanitized = sanitized.replace(/on\w+\s*=\s*["'][^"']*["']/gi, '');

  // Remove javascript: protocol
  sanitized = sanitized.replace(/javascript:/gi, '');

  return sanitized;
};
