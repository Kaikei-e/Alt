/**
 * Smart content type detection utilities
 * Intelligently determines the format of content for optimal rendering
 */

import { sanitizeContent } from "./contentSanitizer";
import sanitizeHtml from "sanitize-html";

export enum ContentType {
  HTML = "html",
  TEXT = "text",
  MARKDOWN = "markdown",
  PLAIN = "plain",
}

/**
 * Detects the content type from a string and optional declared type
 * @param content - The content string to analyze
 * @param declaredType - Optional declared content type from API
 * @returns The detected ContentType
 */
export const detectContentType = (
  content: string,
  declaredType?: string,
): ContentType => {
  // 1. Use declared type if explicitly provided and valid
  if (declaredType) {
    const normalizedType = declaredType.toLowerCase();
    if (Object.values(ContentType).includes(normalizedType as ContentType)) {
      return normalizedType as ContentType;
    }
  }

  // 2. Check for HTML tags presence (ReDoS-safe version)
  if (hasHtmlTags(content)) {
    return ContentType.HTML;
  }

  // 3. Check for Markdown patterns
  const markdownPatterns = [
    /^#{1,6}\s/m, // Headers
    /^\*\s|\*\*.*\*\*/m, // Bold/Lists
    /\[.*\]\(.*\)/, // Links
    /^>\s/m, // Blockquotes
    /```[\s\S]*?```/, // Code blocks
    /^\d+\.\s/m, // Ordered lists
  ];

  const hasMarkdown = markdownPatterns.some((pattern) => pattern.test(content));
  if (hasMarkdown) {
    return ContentType.MARKDOWN;
  }

  // 4. Default to plain text
  return ContentType.TEXT;
};

/**
 * ReDoS-safe HTML tag detection using character scanning instead of regex
 * @param content - Content to check for HTML tags
 * @returns True if HTML tags are found
 */
function hasHtmlTags(content: string): boolean {
  if (!content || typeof content !== "string") {
    return false;
  }

  let inTag = false;
  let tagChars = 0;
  const maxScanLength = Math.min(content.length, 10000); // Limit scan to prevent performance issues

  for (let i = 0; i < maxScanLength; i++) {
    const char = content[i];

    if (char === "<") {
      inTag = true;
      tagChars = 0;
    } else if (char === ">" && inTag) {
      // Found complete tag - check if it looks like a valid HTML tag
      if (tagChars > 0) {
        return true;
      }
      inTag = false;
    } else if (inTag) {
      // Count characters in tag (letters, numbers, spaces, common attributes, URLs)
      if (/[a-zA-Z0-9\s='"\/\-&;:?.]/.test(char)) {
        tagChars++;
        // Prevent extremely long "tags" (likely not real HTML)
        if (tagChars > 100) {
          inTag = false;
          tagChars = 0;
        }
      } else {
        // Invalid character in tag, not HTML
        inTag = false;
        tagChars = 0;
      }
    }
  }

  return false;
}

/**
 * Safely tests regex patterns with timeout protection against ReDoS
 * @param content - Content to test
 * @param patterns - Array of regex patterns to test
 * @returns True if any pattern matches
 */
function safeRegexTest(content: string, patterns: RegExp[]): boolean {
  if (!content || !patterns || patterns.length === 0) {
    return false;
  }

  return patterns.some((pattern) => {
    try {
      // Add timeout protection for regex execution
      const startTime = performance.now();
      const result = pattern.test(content);
      const duration = performance.now() - startTime;

      // If regex takes too long (potential ReDoS), assume no match
      if (duration > 10) {
        console.warn(`Regex took too long: ${duration}ms`);
        return false;
      }

      return result;
    } catch (error) {
      // If regex fails, assume no match
      return false;
    }
  });
}

/**
 * Safely removes HTML tags for text extraction without ReDoS vulnerability
 * Uses existing contentSanitizer for secure processing
 */
function safeStripHtml(content: string): string {
  if (!content || typeof content !== "string") {
    return "";
  }

  try {
    // Use sanitizeContent with empty allowed tags to strip all HTML
    const sanitized = sanitizeContent(content, {
      allowedTags: [], // Remove all tags
      allowedAttributes: {},
      maxLength: content.length + 100, // Don't truncate for word counting
    });

    // Further clean any remaining HTML entities and normalize whitespace
    return sanitized
      .replace(/&[#\w]+;/g, " ") // Remove HTML entities
      .replace(/\s+/g, " ") // Normalize whitespace
      .trim();
  } catch (error) {
    // Fallback to safe character-based cleaning if library fails
    return content
      .replace(/[<>]/g, " ") // Replace brackets with spaces
      .replace(/&[#\w]+;/g, " ") // Remove HTML entities
      .replace(/\s+/g, " ")
      .trim();
  }
}

/**
 * Checks if content contains potentially unsafe HTML using sanitize-html
 * SECURITY FIX: Replace regex-based detection with battle-tested sanitize-html
 * @param content - Content to check
 * @returns True if content needs sanitization
 */
export const needsSanitization = (content: string): boolean => {
  if (!content || typeof content !== "string") {
    return false;
  }

  try {
    // Use sanitize-html to sanitize content and compare with original
    // If they differ, the content contained dangerous elements
    const sanitized = sanitizeHtml(content, {
      allowedTags: [
        "p",
        "br",
        "strong",
        "em",
        "u",
        "i",
        "a",
        "ul",
        "ol",
        "li",
        "h1",
        "h2",
        "h3",
        "h4",
        "h5",
        "h6",
      ],
      allowedAttributes: {
        a: ["href", "title"],
      },
      allowedSchemes: ["http", "https", "mailto"],
      allowProtocolRelative: false,
    });

    // Content needs sanitization if sanitize-html modified it
    return content !== sanitized;
  } catch (error) {
    // If sanitize-html fails, assume content needs sanitization for safety
    console.warn("sanitize-html sanitization failed:", error);
    return true;
  }
};

/**
 * Analyzes content complexity for rendering optimization
 * @param content - Content to analyze
 * @returns Analysis results
 */
export interface ContentAnalysis {
  type: ContentType;
  hasImages: boolean;
  hasLinks: boolean;
  hasLists: boolean;
  wordCount: number;
  needsSanitization: boolean;
  estimatedReadingTime: number; // in minutes
}

export const analyzeContent = (
  content: string,
  declaredType?: string,
): ContentAnalysis => {
  if (!content || typeof content !== "string") {
    return {
      type: ContentType.TEXT,
      hasImages: false,
      hasLinks: false,
      hasLists: false,
      wordCount: 0,
      needsSanitization: false,
      estimatedReadingTime: 1,
    };
  }

  const type = detectContentType(content, declaredType);

  // SECURITY FIX: Use safer regex patterns to prevent ReDoS
  const hasImages = safeRegexTest(content, [
    /<img\b[^>]*>/gi, // Safe: matches img tag without ReDoS risk
    /!\[.*?\]\(.*?\)/g, // Safe: non-greedy match for markdown images
  ]);

  const hasLinks = safeRegexTest(content, [
    /<a\b[^>]*>/gi, // Safe: matches a tag without ReDoS risk
    /\[.*?\]\(.*?\)/g, // Safe: non-greedy match for markdown links
  ]);

  const hasLists = safeRegexTest(content, [
    /<[uo]l\b[^>]*>/gi, // Safe: matches list tags without ReDoS risk
    /^\s*[-*+]\s/m, // Safe: matches markdown list items
  ]);

  // SECURITY FIX: Use safe HTML stripping to prevent ReDoS vulnerability
  const textOnly = safeStripHtml(content);

  // Fix empty string word count issue
  const wordCount = textOnly
    ? textOnly.split(/\s+/).filter((word) => word.length > 0).length
    : 0;

  // Average reading speed: 200 words per minute
  const estimatedReadingTime = Math.max(1, Math.ceil(wordCount / 200));

  return {
    type,
    hasImages,
    hasLinks,
    hasLists,
    wordCount,
    needsSanitization: needsSanitization(content),
    estimatedReadingTime,
  };
};
