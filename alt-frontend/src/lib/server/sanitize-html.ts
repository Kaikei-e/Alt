/**
 * Server-only HTML sanitization utilities
 * Uses DOMPurify to sanitize HTML content on the server side only.
 * This module is protected by 'server-only' to prevent client-side bundling.
 */

import "server-only";
import DOMPurify from "isomorphic-dompurify";
import type { Config } from "dompurify";
import * as he from "he";

/**
 * Branded type for sanitized HTML strings
 * This type ensures that only sanitized HTML is passed to dangerouslySetInnerHTML
 */
export type SafeHtmlString = string & { readonly __brand: "SafeHtml" };

/**
 * Create a SafeHtmlString from a sanitized HTML string
 * This should only be called with already-sanitized content
 */
export function createSafeHtml(content: string): SafeHtmlString {
  return content as SafeHtmlString;
}

/**
 * Default DOMPurify configuration for article content
 * Based on existing configurations in renderingStrategies.tsx and RenderFeedDetails.tsx
 */
const defaultArticleConfig: Config = {
  ALLOWED_TAGS: [
    "p",
    "br",
    "strong",
    "b",
    "em",
    "i",
    "u",
    "span",
    "div",
    "h1",
    "h2",
    "h3",
    "h4",
    "h5",
    "h6",
    "ul",
    "ol",
    "li",
    "a",
    "img",
    "blockquote",
    "pre",
    "code",
    "table",
    "thead",
    "tbody",
    "tr",
    "td",
    "th",
  ],
  ALLOWED_ATTR: [
    "class",
    "id",
    "style",
    "href",
    "target",
    "rel",
    "title",
    "src",
    "alt",
    "width",
    "height",
    "loading",
    "data-proxy-url",
  ],
  ALLOW_DATA_ATTR: true,
  ALLOWED_URI_REGEXP:
    /^(?:(?:(?:f|ht)tps?|mailto|tel|callto|sms|cid|xmpp|data):|[^a-z]|[a-z+.\-]+(?:[^a-z+.\-:]|$))/i,
  KEEP_CONTENT: true,
};

/**
 * Default configuration for basic HTML sanitization
 */
const defaultConfig: Config = {
  USE_PROFILES: { html: true },
};

/**
 * Sanitize HTML content for article display
 * Removes dangerous tags and attributes while preserving safe formatting
 * @param dirtyHtml - Raw HTML content from external sources
 * @param config - Optional DOMPurify configuration overrides
 * @returns Sanitized HTML as SafeHtmlString
 */
export function sanitizeForArticle(
  dirtyHtml: string,
  config?: Partial<Config>,
): SafeHtmlString {
  if (!dirtyHtml || typeof dirtyHtml !== "string") {
    return createSafeHtml("");
  }

  const sanitized = DOMPurify.sanitize(dirtyHtml, {
    ...defaultArticleConfig,
    ...config,
  });

  // Post-process to add security attributes to links
  const withSecureLinks = sanitizeLinks(sanitized);

  return createSafeHtml(withSecureLinks);
}

/**
 * Generic HTML sanitization function
 * @param dirtyHtml - Raw HTML content
 * @param config - Optional DOMPurify configuration
 * @returns Sanitized HTML as SafeHtmlString
 */
export function sanitizeHtml(
  dirtyHtml: string,
  config?: Partial<Config>,
): SafeHtmlString {
  if (!dirtyHtml || typeof dirtyHtml !== "string") {
    return createSafeHtml("");
  }

  const sanitized = DOMPurify.sanitize(dirtyHtml, {
    ...defaultConfig,
    ...config,
  });

  return createSafeHtml(sanitized);
}

/**
 * Sanitize HTML to plain text by removing all tags
 * Useful for RSS summaries, search indexes, etc.
 * @param dirtyHtml - Raw HTML content
 * @returns Plain text with all HTML tags removed
 */
export function sanitizeToPlainText(dirtyHtml: string): string {
  if (!dirtyHtml || typeof dirtyHtml !== "string") {
    return "";
  }

  const sanitized = DOMPurify.sanitize(dirtyHtml, {
    ALLOWED_TAGS: [],
    ALLOWED_ATTR: [],
    KEEP_CONTENT: true,
  });

  // DOMPurify with KEEP_CONTENT should remove tags but keep text
  // However, we need to ensure all tags are actually removed
  // Use a simple regex as a fallback to remove any remaining tags
  return sanitized.replace(/<[^>]*>/g, "").trim();
}

/**
 * Post-process sanitized HTML to add security attributes to links
 * Ensures all links have rel="noopener noreferrer nofollow ugc"
 * @param sanitizedHtml - Already sanitized HTML
 * @returns HTML with secure link attributes
 */
function sanitizeLinks(sanitizedHtml: string): string {
  return sanitizedHtml.replace(
    /<a([^>]*?)href="([^"]*?)"([^>]*?)>/gi,
    (match, before, href, after) => {
      // Only allow safe URL schemes
      if (!/^(?:(?:https?|mailto|tel):|\/(?!\/)|#)/i.test(href)) {
        // Remove href attribute for unsafe URLs
        return `<a${before}${after}>`;
      }

      // Force safe attributes on links
      const hasRel = /rel=["'][^"']*["']/i.test(match);
      if (!hasRel) {
        return `<a${before}href="${href}"${after} rel="noopener noreferrer nofollow ugc">`;
      }

      // Update existing rel attribute to include security attributes
      return match.replace(
        /rel=["']([^"']*)["']/i,
        'rel="noopener noreferrer nofollow ugc $1"',
      );
    },
  );
}

/**
 * Decode HTML entities safely
 * Uses 'he' library for server-side HTML entity decoding
 * @param str - String with HTML entities
 * @returns Decoded string
 */
export function decodeHtmlEntities(str: string): string {
  if (!str || typeof str !== "string") {
    return "";
  }

  try {
    return he.decode(str);
  } catch (error) {
    // Fallback: return original string if decoding fails
    return str;
  }
}

/**
 * Extract plain text from HTML content
 * Strips all HTML tags and decodes entities
 * @param html - HTML content
 * @returns Plain text
 */
export function extractPlainText(html: string): string {
  if (!html || typeof html !== "string") {
    return "";
  }

  // First sanitize to remove all tags
  const textOnly = sanitizeToPlainText(html);

  // Then decode HTML entities
  const decoded = decodeHtmlEntities(textOnly);

  // Normalize whitespace
  return decoded.replace(/\s+/g, " ").trim();
}

