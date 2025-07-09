/**
 * Content sanitization utilities for XSS prevention
 * Provides safe HTML content processing for external data
 */

export interface SanitizerOptions {
  allowedTags?: string[];
  allowedAttributes?: Record<string, string[]>;
  maxLength?: number;
}

const DEFAULT_OPTIONS: Required<SanitizerOptions> = {
  allowedTags: ['b', 'i', 'em', 'strong', 'p', 'br', 'a'],
  allowedAttributes: {
    'a': ['href'],
    'img': ['src', 'alt']
  },
  maxLength: 1000
};

/**
 * Sanitize content to prevent XSS attacks
 * @param content - Content to sanitize
 * @param options - Sanitization options
 * @returns Sanitized content
 */
export function sanitizeContent(
  content: string | null | undefined,
  options: SanitizerOptions = {}
): string {
  if (!content) return '';

  const {
    allowedTags,
    maxLength
  } = { ...DEFAULT_OPTIONS, ...options };

  // Truncate content if too long
  let sanitized = content.slice(0, maxLength);

  // Remove dangerous tags
  sanitized = removeDangerousTags(sanitized, allowedTags);

  // Remove dangerous attributes
  sanitized = removeDangerousAttributes(sanitized);

  // Remove XSS attack patterns
  sanitized = removeXssPatterns(sanitized);

  return sanitized.trim();
}

/**
 * Remove HTML tags not in the allowed list
 * @param html - HTML content
 * @param allowedTags - List of allowed HTML tags
 * @returns HTML with only allowed tags
 */
function removeDangerousTags(html: string, allowedTags: string[]): string {
  // First, always remove dangerous tags and their content
  const dangerousTags = ['script', 'style', 'iframe', 'object', 'embed', 'form', 'input', 'textarea', 'button', 'meta', 'link'];
  
  // Remove dangerous tags along with their content
  for (const tag of dangerousTags) {
    const regex = new RegExp(`<${tag}\\b[^>]*>.*?<\\/${tag}>`, 'gsi');
    html = html.replace(regex, '');
    // Also remove self-closing tags
    const selfClosingRegex = new RegExp(`<${tag}\\b[^>]*\\/>`, 'gsi');
    html = html.replace(selfClosingRegex, '');
    // Remove any remaining opening/closing tags
    const openingRegex = new RegExp(`<\\/?${tag}\\b[^>]*>`, 'gsi');
    html = html.replace(openingRegex, '');
  }

  if (allowedTags.length === 0) {
    // Remove all remaining HTML tags
    return html.replace(/<[^>]*>/g, '');
  }

  // Then remove any remaining tags not in the allowed list
  const allowedPattern = allowedTags.join('|');
  const regex = new RegExp(`<(?!\\/?(?:${allowedPattern})\\b)[^>]*>`, 'gi');
  
  return html.replace(regex, '');
}

/**
 * Remove dangerous attributes from HTML elements
 * @param html - HTML content
 * @param allowedAttributes - Map of allowed attributes per tag
 * @returns HTML with dangerous attributes removed
 */
function removeDangerousAttributes(html: string): string {
  // Remove event handlers and dangerous attributes
  let cleaned = html.replace(/\s*on\w+\s*=\s*["'][^"']*["']/gi, '');
  
  // Remove javascript:, vbscript:, data: protocols
  cleaned = cleaned.replace(/\s*href\s*=\s*["'](?:javascript|vbscript|data):[^"']*["']/gi, '');
  
  // Remove style attributes that could contain expressions
  cleaned = cleaned.replace(/\s*style\s*=\s*["'][^"']*["']/gi, '');
  
  // For now, keep only safe href attributes for links
  // More sophisticated attribute filtering could be added here
  
  return cleaned;
}

/**
 * Remove known XSS attack patterns
 * @param html - HTML content
 * @returns HTML with XSS patterns removed
 */
function removeXssPatterns(html: string): string {
  const xssPatterns = [
    /javascript:/gi,
    /vbscript:/gi,
    /data:/gi,
    /expression\s*\(/gi,
    /@import/gi,
    /binding\s*:/gi,
    /eval\s*\(/gi,
    /setTimeout\s*\(/gi,
    /setInterval\s*\(/gi
  ];

  return xssPatterns.reduce((content, pattern) => {
    return content.replace(pattern, '');
  }, html);
}

/**
 * Validate and sanitize URL
 * @param url - URL to validate
 * @returns Safe URL or empty string if invalid
 */
function sanitizeUrl(url: string): string {
  if (!url) return '';
  
  // Allow only http and https protocols
  const urlPattern = /^https?:\/\//i;
  if (!urlPattern.test(url)) {
    return '';
  }
  
  // Remove javascript: and other dangerous protocols
  const dangerousProtocols = /^(javascript|vbscript|data|ftp|file):/i;
  if (dangerousProtocols.test(url)) {
    return '';
  }
  
  return url;
}

/**
 * Sanitize feed content specifically
 * @param feed - Feed object to sanitize
 * @returns Sanitized feed object
 */
export function sanitizeFeedContent(feed: { title?: string; description?: string; author?: string; link?: string }): {
  title: string;
  description: string;
  author: string;
  link: string;
} {
  return {
    title: sanitizeContent(feed.title || '', { maxLength: 200 }),
    description: sanitizeContent(feed.description || '', { maxLength: 500 }),
    author: sanitizeContent(feed.author || '', { 
      maxLength: 100, 
      allowedTags: [] // Remove all HTML tags from author names
    }),
    link: sanitizeUrl(feed.link || '')
  };
}