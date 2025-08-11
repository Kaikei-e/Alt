/**
 * Content rendering strategies for different content types
 * Implements strategy pattern for HTML, text, and markdown rendering
 * XPLAN11: 2025 Best Practices - React 19 + Next.js 15 Compatible
 */
import React from 'react';
import { ReactNode } from 'react';
import { ContentType, analyzeContent } from './contentTypeDetector';
import DOMPurify from 'isomorphic-dompurify';

// DOMPurify config interface for type safety
interface DOMPurifyConfig {
  ALLOWED_TAGS?: string[];
  ALLOWED_ATTR?: string[];
  ALLOW_DATA_ATTR?: boolean;
  ALLOWED_URI_REGEXP?: RegExp;
  ADD_TAGS?: string[];
  ADD_ATTR?: string[];
}

export interface RenderingStrategy {
  render(content: string): ReactNode;
  shouldUse(content: string, declaredType?: string): boolean;
}

/**
 * HTML Content Rendering Strategy - XPLAN11 Option A Implementation
 * High-performance dangerouslySetInnerHTML + isomorphic-dompurify approach
 * React 19 + Next.js 15 compatible, solves &amp; entity issues
 */
export class HTMLRenderingStrategy implements RenderingStrategy {
  shouldUse(content: string, declaredType?: string): boolean {
    const analysis = analyzeContent(content, declaredType);
    return analysis.type === ContentType.HTML;
  }

  render(content: string): ReactNode {
    // Step 1: Decode HTML entities BEFORE sanitization (XPLAN11 solution)
    const decodedHTML = this.decodeHtmlEntities(content);
    
    // Step 2: Sanitize with isomorphic-dompurify (SSR-safe)
    const sanitizedHTML = DOMPurify.sanitize(decodedHTML, {
      ALLOWED_TAGS: [
        'p', 'br', 'strong', 'b', 'em', 'i', 'u', 'span', 'div',
        'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
        'ul', 'ol', 'li',
        'a', 'img',
        'blockquote', 'pre', 'code',
        'table', 'thead', 'tbody', 'tr', 'td', 'th'
      ],
      ALLOWED_ATTR: [
        'href', 'target', 'rel',
        'src', 'alt', 'title',
        'class', 'id', 'style',
        'width', 'height', 'loading'
      ],
      ALLOW_DATA_ATTR: false,
      ALLOWED_URI_REGEXP: /^https?:\/\//i,
    });

    // Step 3: Enhanced HTML with custom CSS for images and links
    const enhancedHTML = this.enhanceHTMLElements(sanitizedHTML);
    
    // Step 4: Return using dangerouslySetInnerHTML (XPLAN11 high-performance approach)
    return (
      <div 
        className="smart-content-html"
        dangerouslySetInnerHTML={{ __html: enhancedHTML }}
        style={{
          lineHeight: '1.7',
          color: 'var(--text-primary)'
        } as React.CSSProperties}
      />
    );
  }

  /**
   * Decode HTML entities (XPLAN11: Pre-sanitization step)
   */
  private decodeHtmlEntities(str: string): string {
    if (typeof window !== 'undefined') {
      const textarea = document.createElement('textarea');
      textarea.innerHTML = str;
      return textarea.value;
    }
    // Enhanced fallback for server-side rendering (more entities covered)
    return str
      .replace(/&amp;/g, '&')
      .replace(/&lt;/g, '<')
      .replace(/&gt;/g, '>')
      .replace(/&quot;/g, '"')
      .replace(/&#39;/g, "'")
      .replace(/&nbsp;/g, ' ')
      .replace(/&copy;/g, '©')
      .replace(/&reg;/g, '®')
      .replace(/&trade;/g, '™');
  }

  /**
   * Enhance HTML elements with proper attributes (XPLAN11: Post-sanitization)
   */
  private enhanceHTMLElements(html: string): string {
    return html
      // Add loading="lazy" to all images
      .replace(/<img([^>]*?)>/gi, '<img$1 loading="lazy">')
      // Add security attributes to all links
      .replace(/<a([^>]*?)href="([^"]*?)"([^>]*?)>/gi, 
        '<a$1href="$2"$3 target="_blank" rel="noopener noreferrer nofollow">');
  }

}

/**
 * Plain Text Rendering Strategy
 * Renders plain text with basic formatting preservation
 */
export class TextRenderingStrategy implements RenderingStrategy {
  shouldUse(content: string, declaredType?: string): boolean {
    const analysis = analyzeContent(content, declaredType);
    return analysis.type === ContentType.TEXT || analysis.type === ContentType.PLAIN;
  }

  render(content: string): ReactNode {
    // Split content into paragraphs and preserve line breaks
    const paragraphs = content
      .split(/\n\s*\n/) // Split on double line breaks
      .map(paragraph => paragraph.trim())
      .filter(paragraph => paragraph.length > 0);

    return (
      <div style={{ whiteSpace: 'pre-wrap', lineHeight: '1.7' }}>
        {paragraphs.map((paragraph, index) => (
          <p key={index} style={{ marginBottom: '1em' }}>
            {paragraph.split('\n').map((line, lineIndex) => (
              <span key={lineIndex}>
                {line}
                {lineIndex < paragraph.split('\n').length - 1 && <br />}
              </span>
            ))}
          </p>
        ))}
      </div>
    );
  }
}

/**
 * Markdown Rendering Strategy - XPLAN11 Compatible
 * Basic markdown rendering with isomorphic-dompurify
 */
export class MarkdownRenderingStrategy implements RenderingStrategy {
  shouldUse(content: string, declaredType?: string): boolean {
    const analysis = analyzeContent(content, declaredType);
    return analysis.type === ContentType.MARKDOWN;
  }

  render(content: string): ReactNode {
    // Basic markdown-to-HTML conversion
    let html = content
      // Headers
      .replace(/^### (.*$)/gim, '<h3>$1</h3>')
      .replace(/^## (.*$)/gim, '<h2>$1</h2>')
      .replace(/^# (.*$)/gim, '<h1>$1</h1>')
      // Bold and italic
      .replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
      .replace(/\*(.*?)\*/g, '<em>$1</em>')
      // Links
      .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>')
      // Line breaks
      .replace(/\n/g, '<br />');

    // Sanitize the converted HTML with isomorphic-dompurify
    const sanitizedHTML = DOMPurify.sanitize(html, {
      ALLOWED_TAGS: ['h1', 'h2', 'h3', 'strong', 'em', 'a', 'br', 'p'],
      ALLOWED_ATTR: ['href', 'target', 'rel'],
    });

    return (
      <div 
        className="smart-content-markdown"
        dangerouslySetInnerHTML={{ __html: sanitizedHTML }}
        style={{
          lineHeight: '1.7',
          color: 'var(--text-primary)'
        }}
      />
    );
  }
}

/**
 * Strategy Registry
 * Manages and selects appropriate rendering strategies
 */
export class RenderingStrategyRegistry {
  private strategies: RenderingStrategy[] = [
    new HTMLRenderingStrategy(),
    new MarkdownRenderingStrategy(),
    new TextRenderingStrategy(), // Keep text strategy last as fallback
  ];

  /**
   * Select the appropriate rendering strategy for content
   * @param content - Content to render
   * @param declaredType - Optional declared content type
   * @returns The appropriate rendering strategy
   */
  selectStrategy(content: string, declaredType?: string): RenderingStrategy {
    for (const strategy of this.strategies) {
      if (strategy.shouldUse(content, declaredType)) {
        return strategy;
      }
    }
    
    // Fallback to text strategy
    return new TextRenderingStrategy();
  }

  /**
   * Render content using the appropriate strategy
   * @param content - Content to render
   * @param declaredType - Optional declared content type
   * @returns Rendered React nodes
   */
  render(content: string, declaredType?: string): ReactNode {
    if (!content || content.trim().length === 0) {
      return <span style={{ fontStyle: 'italic', color: '#888' }}>No content available</span>;
    }

    const strategy = this.selectStrategy(content, declaredType);
    return strategy.render(content);
  }
}

// Export singleton instance
export const renderingRegistry = new RenderingStrategyRegistry();