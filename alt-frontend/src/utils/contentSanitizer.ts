/**
 * Content sanitization utilities for XSS prevention
 * NOTE: HTML content is now sanitized server-side.
 * This module provides client-side utilities for text processing only.
 */
import { replaceUntilStable } from "./htmlEntityUtils";

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
 * Sanitize content to prevent XSS attacks (client-side text processing only)
 * NOTE: HTML content should be sanitized server-side before reaching the client.
 * This function only handles text truncation and basic HTML tag removal for plain text.
 * @param content - Content to sanitize
 * @param options - Sanitization options
 * @returns Sanitized content (plain text)
 */
export function sanitizeContent(
  content: string | null | undefined,
  options: SanitizerOptions = {},
): string {
  if (!content) return "";

  const mergedOptions = { ...DEFAULT_OPTIONS, ...options };
  const { maxLength } = mergedOptions;

  // Truncate content if too long
  const truncated = content.slice(0, maxLength);

  // Remove HTML tags (SECURITY FIX: use repeated replacement to prevent incomplete multi-character sanitization)
  // For HTML content, use server-side sanitization
  const textOnly = replaceUntilStable(truncated, /<[^>]*>/g, "");

  return textOnly.trim();
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
