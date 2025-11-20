/**
 * Content sanitization utilities for XSS prevention
 * Provides safe HTML content processing for external data
 */
import DOMPurify from "isomorphic-dompurify";

export interface SanitizerOptions {
  allowedTags?: string[];
  allowedAttributes?: Record<string, string[]>;
  maxLength?: number;
}

const DEFAULT_OPTIONS: Required<SanitizerOptions> = {
  allowedTags: ["b", "i", "em", "strong", "p", "br", "a"],
  allowedAttributes: {
    a: ["href"],
    img: ["src", "alt"],
  },
  maxLength: 1000,
};

/**
 * Sanitize content to prevent XSS attacks
 * @param content - Content to sanitize
 * @param options - Sanitization options
 * @returns Sanitized content
 */
export function sanitizeContent(
  content: string | null | undefined,
  options: SanitizerOptions = {},
): string {
  if (!content) return "";

  const mergedOptions = { ...DEFAULT_OPTIONS, ...options };
  const { maxLength, allowedTags, allowedAttributes } = mergedOptions;

  // Truncate content if too long
  const truncated = content.slice(0, maxLength);

  // Convert allowedAttributes map to list for DOMPurify
  // DOMPurify applies allowed attributes globally to allowed tags.
  const allowedAttrList = new Set<string>();
  if (allowedAttributes) {
    Object.values(allowedAttributes).forEach((attrs) => {
      attrs.forEach((attr) => allowedAttrList.add(attr));
    });
  }

  // Sanitize using DOMPurify
  const sanitized = DOMPurify.sanitize(truncated, {
    ALLOWED_TAGS: allowedTags,
    ALLOWED_ATTR: Array.from(allowedAttrList),
    // DOMPurify defaults are safe for protocols (http, https, mailto, etc.)
  });

  return sanitized.trim();
}

/**
 * Validate and sanitize URL
 * @param url - URL to validate
 * @returns Safe URL or empty string if invalid
 */
function sanitizeUrl(url: string): string {
  if (!url) return "";

  // Allow only http and https protocols
  const urlPattern = /^https?:\/\//i;
  if (!urlPattern.test(url)) {
    return "";
  }

  // Remove javascript: and other dangerous protocols
  const dangerousProtocols = /^(javascript|vbscript|data|ftp|file):/i;
  if (dangerousProtocols.test(url)) {
    return "";
  }

  return url;
}

/**
 * Sanitize feed content specifically
 * @param feed - Feed object to sanitize
 * @returns Sanitized feed object
 */
export function sanitizeFeedContent(feed: {
  title?: string;
  description?: string;
  author?: string;
  link?: string;
}): {
  title: string;
  description: string;
  author: string;
  link: string;
} {
  return {
    title: sanitizeContent(feed.title || "", { maxLength: 200 }),
    description: sanitizeContent(feed.description || "", { maxLength: 500 }),
    author: sanitizeContent(feed.author || "", {
      maxLength: 100,
      allowedTags: [], // Remove all HTML tags from author names
    }),
    link: sanitizeUrl(feed.link || ""),
  };
}
