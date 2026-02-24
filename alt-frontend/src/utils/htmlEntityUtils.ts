/**
 * HTML Entity Utilities
 * Security-focused utilities for HTML entity decoding and sanitization
 * Based on PLAN.md recommendations for safe HTML processing without libraries
 */

/**
 * Generic helper for repeated regex replacement until stable
 * SECURITY FIX: Prevents incomplete multi-character sanitization vulnerabilities
 * Based on PLAN.md recommendation: apply regex replacement repeatedly until no more replacements
 * This ensures that unsafe text does not re-appear in the sanitized input
 * @param input - Input string to sanitize
 * @param regex - Regular expression pattern to match
 * @param replacement - Replacement string
 * @returns Sanitized string with all matches removed
 */
export function replaceUntilStable(
  input: string,
  regex: RegExp,
  replacement: string,
): string {
  let previous: string;
  let current = input;
  do {
    previous = current;
    current = current.replace(regex, replacement);
  } while (current !== previous);
  return current;
}

/**
 * Decode HTML entities manually using repeated replacement
 * PLAN.md: Use repeated replacement to handle double-encoded entities
 * Apply entity decoding repeatedly until no more replacements occur
 * @param str - String with HTML entities
 * @returns Decoded string
 */
export function decodeHtmlEntitiesManually(str: string): string {
  if (!str || typeof str !== "string") {
    return "";
  }

  // PLAN.md: Use repeated replacement to handle double-encoded entities
  // Apply entity decoding repeatedly until no more replacements occur
  let previous: string;
  let current = str;
  do {
    previous = current;
    // Order matters: decode &amp; last to handle double-encoded entities
    current = current
      .replace(/&nbsp;/g, " ")
      .replace(/&quot;/g, '"')
      .replace(/&#39;/g, "'")
      .replace(/&apos;/g, "'")
      .replace(/&lt;/g, "<")
      .replace(/&gt;/g, ">")
      .replace(/&copy;/g, "©")
      .replace(/&reg;/g, "®")
      .replace(/&trade;/g, "™")
      .replace(/&amp;/g, "&");
  } while (current !== previous);
  return current;
}

/**
 * Decode HTML entities safely with repeated decoding support
 * PLAN.md: Use browser's native decoder, but apply repeatedly to handle double-encoded entities
 * Browser's textarea.innerHTML decoder may only decode once, so we repeat until stable
 * @param value - String with HTML entities
 * @returns Decoded string
 */
export function decodeEntitiesSafely(value: string): string {
  if (!value || typeof value !== "string") {
    return "";
  }

  if (typeof document !== "undefined") {
    // PLAN.md: Use browser's native decoder (DOMParser), but apply repeatedly to handle double-encoded entities
    // DOMParser is safer than textarea.innerHTML as it doesn't execute scripts
    let previous: string;
    let current = value;
    do {
      previous = current;
      const parser = new DOMParser();
      const doc = parser.parseFromString(current, "text/html");
      current = doc.documentElement.textContent || "";
      // Break if no change occurred to avoid infinite loop
      if (current === previous) {
        break;
      }
    } while (true);
    return current;
  }

  return decodeHtmlEntitiesManually(value);
}

/**
 * Decode HTML entities exactly once (security best practice)
 * SECURITY: Decodes only once to prevent double-decoding vulnerabilities
 * This is different from decodeHtmlEntitiesFromUrl which fully decodes URLs
 * @param str - String with HTML entities
 * @returns Decoded string (single decode only)
 */
export function decodeHtmlEntities(str: string): string {
  if (!str || typeof str !== "string") {
    return "";
  }

  // Decode only once to prevent double-decoding (security best practice)
  // This is different from decodeHtmlEntitiesFromUrl which fully decodes URLs
  if (typeof document !== "undefined") {
    const parser = new DOMParser();
    const doc = parser.parseFromString(str, "text/html");
    return doc.documentElement.textContent || "";
  }

  // Fallback: manual single decode
  return str
    .replace(/&nbsp;/g, " ")
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .replace(/&apos;/g, "'")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&copy;/g, "©")
    .replace(/&reg;/g, "®")
    .replace(/&trade;/g, "™")
    .replace(/&amp;/g, "&");
}

/**
 * Escape HTML to prevent XSS in data attributes
 * @param str - String to escape
 * @returns HTML-escaped string
 */
export function escapeHtmlForAttributes(str: string): string {
  if (!str || typeof str !== "string") {
    return "";
  }

  return str
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

/**
 * Decode HTML entities specifically for URLs with comprehensive security protection
 * SECURITY FIX: Based on PLAN.md - URL sanitization should focus on blocking dangerous schemes,
 * not on removing HTML tags (URLs shouldn't contain HTML tags anyway)
 * @param url - URL with potential HTML entities
 * @returns Safely decoded and validated URL
 */
export function decodeHtmlEntitiesFromUrl(url: string): string {
  // Input validation for security
  if (!url || typeof url !== "string") {
    return "";
  }

  const trimmedUrl = url.trim();
  if (!trimmedUrl) {
    return "";
  }

  // SECURITY: Block dangerous URL schemes immediately (PLAN.md approach)
  // This is the primary security boundary for URLs
  const dangerousSchemes =
    /^(javascript|vbscript|data:text\/html|data:text\/javascript|file|ftp):/i;
  if (
    dangerousSchemes.test(trimmedUrl) ||
    trimmedUrl.startsWith("data:text/html,") ||
    trimmedUrl.startsWith("data:text/javascript,")
  ) {
    return ""; // Block completely
  }

  // SECURITY: Block URLs containing suspicious patterns even when encoded
  // PLAN.md: Block suspicious JavaScript patterns before any processing
  const suspiciousPatterns =
    /alert\(|eval\(|document\.|window\.|location\.|onerror=|onload=|javascript:/i;
  if (suspiciousPatterns.test(trimmedUrl)) {
    return ""; // Block URLs containing suspicious JavaScript patterns
  }

  try {
    // URL decoding: decode HTML entities safely
    // PLAN.md: For URLs, we don't need to remove HTML tags (they shouldn't be in URLs)
    // Focus on safe entity decoding and scheme validation
    const decoded = decodeEntitiesSafely(trimmedUrl);

    const normalized = decoded.trim();

    if (!normalized) {
      return "";
    }

    // Re-validate after decoding (entities might have revealed dangerous schemes)
    if (
      dangerousSchemes.test(normalized) ||
      normalized.startsWith("data:text/html,") ||
      normalized.startsWith("data:text/javascript,")
    ) {
      return "";
    }

    // Final check for malicious patterns after decoding
    const maliciousPatterns = /<script|<iframe|onerror=|onload=|javascript:/i;
    if (
      maliciousPatterns.test(normalized) ||
      suspiciousPatterns.test(normalized)
    ) {
      return "";
    }

    // PLAN.md: URLs shouldn't contain HTML tags, but if they do, remove them using repeated replacement
    if (/<[^>]*>/i.test(normalized)) {
      // If HTML tags are found in URL (unexpected), remove them using repeated replacement
      const tagFree = replaceUntilStable(normalized, /<[^>]*>/g, "");
      // Re-check for dangerous schemes after tag removal
      if (dangerousSchemes.test(tagFree) || suspiciousPatterns.test(tagFree)) {
        return "";
      }
      return tagFree.trim();
    }

    return normalized;
  } catch (_error) {
    // PLAN.md: Fail-safe - return empty string on any error
    return "";
  }
}
