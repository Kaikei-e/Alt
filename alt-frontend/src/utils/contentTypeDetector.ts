/**
 * Smart content type detection utilities
 * Intelligently determines the format of content for optimal rendering
 */

export enum ContentType {
  HTML = 'html',
  TEXT = 'text',
  MARKDOWN = 'markdown',
  PLAIN = 'plain'
}

/**
 * Detects the content type from a string and optional declared type
 * @param content - The content string to analyze
 * @param declaredType - Optional declared content type from API
 * @returns The detected ContentType
 */
export const detectContentType = (content: string, declaredType?: string): ContentType => {
  // 1. Use declared type if explicitly provided and valid
  if (declaredType) {
    const normalizedType = declaredType.toLowerCase();
    if (Object.values(ContentType).includes(normalizedType as ContentType)) {
      return normalizedType as ContentType;
    }
  }

  // 2. Check for HTML tags presence
  const htmlTagRegex = /<[^>]+>/g;
  if (htmlTagRegex.test(content)) {
    return ContentType.HTML;
  }

  // 3. Check for Markdown patterns
  const markdownPatterns = [
    /^#{1,6}\s/m,           // Headers
    /^\*\s|\*\*.*\*\*/m,    // Bold/Lists  
    /\[.*\]\(.*\)/,         // Links
    /^>\s/m,                // Blockquotes
    /```[\s\S]*?```/,       // Code blocks
    /^\d+\.\s/m,            // Ordered lists
  ];

  const hasMarkdown = markdownPatterns.some(pattern => pattern.test(content));
  if (hasMarkdown) {
    return ContentType.MARKDOWN;
  }

  // 4. Default to plain text
  return ContentType.TEXT;
};

/**
 * Checks if content contains potentially unsafe HTML
 * @param content - Content to check
 * @returns True if content needs sanitization
 */
export const needsSanitization = (content: string): boolean => {
  const dangerousPatterns = [
    /<script[\s\S]*?<\/script>/gi,
    /javascript:/gi,
    /on\w+\s*=/gi,
    /<iframe[\s\S]*?<\/iframe>/gi,
    /<object[\s\S]*?<\/object>/gi,
    /<embed[\s\S]*?>/gi,
  ];

  return dangerousPatterns.some(pattern => pattern.test(content));
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

export const analyzeContent = (content: string, declaredType?: string): ContentAnalysis => {
  const type = detectContentType(content, declaredType);
  const hasImages = /<img[\s\S]*?>/gi.test(content) || /!\[.*\]\(.*\)/g.test(content);
  const hasLinks = /<a[\s\S]*?>/gi.test(content) || /\[.*\]\(.*\)/g.test(content);
  const hasLists = /<[uo]l[\s\S]*?>/gi.test(content) || /^\s*[-*]\s/m.test(content);
  
  // Simple word count (excluding HTML tags)
  const textOnly = content.replace(/<[^>]*>/g, '').replace(/\s+/g, ' ').trim();
  const wordCount = textOnly.split(' ').length;
  
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