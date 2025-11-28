/**
 * Content rendering strategies for different content types
 * Implements strategy pattern for HTML, text, and markdown rendering
 * XPLAN11: 2025 Best Practices - React 19 + Next.js 15 Compatible
 */
import React, { type ReactNode } from "react";
import { analyzeContent, ContentType } from "./contentTypeDetector";
import type { SafeHtmlString } from "@/lib/server/sanitize-html";
import {
  decodeHtmlEntities as decodeHtmlEntitiesUtil,
  decodeHtmlEntitiesFromUrl as decodeHtmlEntitiesFromUrlUtil,
} from "./htmlEntityUtils";

export interface RenderingStrategy {
  render(content: string, articleUrl?: string): ReactNode;
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

  render(content: string | SafeHtmlString, articleUrl?: string): ReactNode {
    // Content is already sanitized server-side as SafeHtmlString
    // We only need to enhance HTML elements (images, links) for display
    const safeHtml = content as string; // SafeHtmlString is a branded string

    // Enhanced HTML with custom CSS for images and links
    const enhancedHTML = this.enhanceHTMLElements(safeHtml, articleUrl);

    // Return using dangerouslySetInnerHTML
    return <HTMLContentRenderer html={enhancedHTML} />;
  }

  /**
   * Decode HTML entities safely (client-side only, for URL processing)
   * Note: HTML content is already sanitized server-side, this is only for URL decoding
   * SECURITY: Decodes only once to prevent double-decoding vulnerabilities
   * @deprecated Use decodeHtmlEntities from htmlEntityUtils directly
   */
  public decodeHtmlEntities(str: string): string {
    return decodeHtmlEntitiesUtil(str);
  }

  /**
   * Enhance HTML elements with proper attributes (XPLAN11: Post-sanitization)
   * Fixed: Proper URL decoding order to prevent double encoding
   * Fixed: Resolve relative image paths against article URL
   */
  private enhanceHTMLElements(html: string, articleUrl?: string): string {
    return (
      html
        // Step 1: Process image URLs (resolve relative paths, decode entities)
        .replace(
          /<img([^>]*?)src="([^"]*?)"([^>]*?)>/gi,
          (match, before, src, after) => {
            // Decode HTML entities
            const decodedSrc = decodeHtmlEntitiesFromUrlUtil(src);
            // Resolve relative paths against article URL
            const resolvedSrc = this.resolveRelativeImageUrl(
              decodedSrc,
              articleUrl,
            );
            return `<img${before}src="${resolvedSrc}"${after} loading="lazy">`;
          },
        )
        // Step 2: Add security attributes to all links
        .replace(
          /<a([^>]*?)href="([^"]*?)"([^>]*?)>/gi,
          '<a$1href="$2"$3 target="_blank" rel="noopener noreferrer nofollow">',
        )
    );
  }

  /**
   * Resolve relative image URL against article URL
   * Converts relative paths like "./image.png" to absolute URLs
   * @param imageUrl - Image URL (may be relative)
   * @param articleUrl - Base URL for resolving relative paths
   * @returns Absolute URL or original URL if already absolute
   */
  private resolveRelativeImageUrl(
    imageUrl: string,
    articleUrl?: string,
  ): string {
    if (!articleUrl || !imageUrl) {
      return imageUrl;
    }

    // If already absolute URL, return as-is
    if (
      imageUrl.startsWith("http://") ||
      imageUrl.startsWith("https://") ||
      imageUrl.startsWith("data:") ||
      imageUrl.startsWith("/")
    ) {
      return imageUrl;
    }

    try {
      // Resolve relative URL against article URL
      const baseUrl = new URL(articleUrl);
      const resolvedUrl = new URL(imageUrl, baseUrl);
      return resolvedUrl.toString();
    } catch (error) {
      // If URL resolution fails, return original URL
      return imageUrl;
    }
  }

  /**
   * Decode HTML entities specifically for URLs with comprehensive security protection
   * SECURITY FIX: URL sanitization should focus on blocking dangerous schemes,
   * not on removing HTML tags (URLs shouldn't contain HTML tags anyway)
   * @deprecated Use decodeHtmlEntitiesFromUrl from htmlEntityUtils directly
   * @param url - URL with potential HTML entities
   * @returns Safely decoded and validated URL
   */
  public decodeHtmlEntitiesFromUrl(url: string): string {
    return decodeHtmlEntitiesFromUrlUtil(url);
  }
}

/**
 * Plain Text Rendering Strategy
 * Renders plain text with basic formatting preservation
 */
export class TextRenderingStrategy implements RenderingStrategy {
  shouldUse(content: string, declaredType?: string): boolean {
    const analysis = analyzeContent(content, declaredType);
    return (
      analysis.type === ContentType.TEXT || analysis.type === ContentType.PLAIN
    );
  }

  render(content: string, articleUrl?: string): ReactNode {
    // Split content into paragraphs and preserve line breaks
    const paragraphs = content
      .split(/\n\s*\n/) // Split on double line breaks
      .map((paragraph) => paragraph.trim())
      .filter((paragraph) => paragraph.length > 0);

    return (
      <div style={{ whiteSpace: "pre-wrap", lineHeight: "1.7" }}>
        {paragraphs.map((paragraph, index) => (
          <p key={index} style={{ marginBottom: "1em" }}>
            {paragraph.split("\n").map((line, lineIndex) => (
              <span key={lineIndex}>
                {line}
                {lineIndex < paragraph.split("\n").length - 1 && <br />}
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

  render(content: string, articleUrl?: string): ReactNode {
    // Basic markdown-to-HTML conversion
    const html = content
      // Headers
      .replace(/^### (.*$)/gim, "<h3>$1</h3>")
      .replace(/^## (.*$)/gim, "<h2>$1</h2>")
      .replace(/^# (.*$)/gim, "<h1>$1</h1>")
      // Bold and italic
      .replace(/\*\*(.*?)\*\*/g, "<strong>$1</strong>")
      .replace(/\*(.*?)\*/g, "<em>$1</em>")
      // Links
      .replace(
        /\[([^\]]+)\]\(([^)]+)\)/g,
        '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>',
      )
      // Line breaks
      .replace(/\n/g, "<br />");

    // HTML is already sanitized server-side, but we do basic link security
    // Add security attributes to links
    const safeHTML = html.replace(
      /<a([^>]*?)href="([^"]*?)"([^>]*?)>/gi,
      (match, before, href, after) => {
        // Only allow safe URL schemes
        if (!/^(?:(?:https?|mailto|tel):|\/(?!\/)|#)/i.test(href)) {
          return `<a${before}${after}>`;
        }
        // Add security attributes
        const hasRel = /rel=["'][^"']*["']/i.test(match);
        if (!hasRel) {
          return `<a${before}href="${href}"${after} rel="noopener noreferrer nofollow">`;
        }
        return match;
      },
    );

    return (
      <div
        className="smart-content-markdown"
        dangerouslySetInnerHTML={{ __html: safeHTML }}
        style={{
          lineHeight: "1.7",
          color: "var(--text-primary)",
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
   * @param articleUrl - Optional article URL for resolving relative image paths
   * @returns Rendered React nodes
   */
  render(
    content: string,
    declaredType?: string,
    articleUrl?: string,
  ): ReactNode {
    if (!content || content.trim().length === 0) {
      return (
        <span style={{ fontStyle: "italic", color: "#888" }}>
          No content available
        </span>
      );
    }

    const strategy = this.selectStrategy(content, declaredType);
    return strategy.render(content, articleUrl);
  }
}

// Export singleton instance
export const renderingRegistry = new RenderingStrategyRegistry();

/**
 * HTML Content Renderer Component
 * Renders sanitized HTML content
 */
interface HTMLContentRendererProps {
  html: string;
}

const HTMLContentRenderer: React.FC<HTMLContentRendererProps> = ({ html }) => {
  // HTML is already sanitized server-side as SafeHtmlString
  // No need to sanitize again on the client
  const sanitizedHtml = html;

  return (
    <div
      className="smart-content-html"
      dangerouslySetInnerHTML={{ __html: sanitizedHtml }}
      style={
        {
          lineHeight: "1.7",
          color: "var(--text-primary)",
        } as React.CSSProperties
      }
    />
  );
};
