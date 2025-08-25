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
        'width', 'height', 'loading',
        'data-proxy-url', 'onload', 'onerror' // Allow proxy attributes
      ],
      ALLOW_DATA_ATTR: true, // Enable data attributes for proxy functionality
      ALLOWED_URI_REGEXP: /^(https?:\/\/|data:)/i, // Allow data URLs
    });

    // Step 3: Enhanced HTML with custom CSS for images and links
    const enhancedHTML = this.enhanceHTMLElements(sanitizedHTML);
    
    // Step 4: Return using dangerouslySetInnerHTML with proxy image handler
    return (
      <HTMLContentRenderer 
        html={enhancedHTML}
      />
    );
  }

  /**
   * Decode HTML entities (XPLAN11: Pre-sanitization step)
   * SECURITY FIX: Using textContent instead of innerHTML to prevent XSS
   */
  public decodeHtmlEntities(str: string): string {
    // Input validation for security
    if (!str || typeof str !== 'string') {
      return '';
    }

    if (typeof window !== 'undefined') {
      // SECURITY FIX: Create a safe decoding method that doesn't execute scripts
      // Use DOMParser to safely parse and decode HTML entities without script execution risk
      try {
        const parser = new DOMParser();
        const doc = parser.parseFromString(`<!doctype html><body>${str}`, 'text/html');
        return doc.body.textContent || '';
      } catch (e) {
        // Fallback to manual decoding if DOMParser fails
        // This is safer than textarea.innerHTML approach
      }
    }
    // SECURITY FIX: Safe fallback with correct decoding order (decode &amp; last)
    return str
      .replace(/&lt;/g, '<')
      .replace(/&gt;/g, '>')
      .replace(/&quot;/g, '"')
      .replace(/&#39;/g, "'")
      .replace(/&nbsp;/g, ' ')
      .replace(/&copy;/g, '©')
      .replace(/&reg;/g, '®')
      .replace(/&trade;/g, '™')
      .replace(/&amp;/g, '&'); // CRITICAL: Decode &amp; LAST to prevent double-decoding
  }

  /**
   * Enhance HTML elements with proper attributes (XPLAN11: Post-sanitization)
   * COEP Bypass: Convert external image URLs to proxy URLs with POST API support
   * Fixed: Proper URL decoding order to prevent double encoding
   */
  private enhanceHTMLElements(html: string): string {
    return html
      // Step 1: Convert external image URLs to proxy URLs (COEP bypass)
      .replace(/<img([^>]*?)src="([^"]*?)"([^>]*?)>/gi, (match, before, src, after) => {
        // Fix: Decode HTML entities BEFORE converting to proxy URL
        const decodedSrc = this.decodeHtmlEntitiesFromUrl(src);
        const proxiedSrc = this.convertToProxyUrl(decodedSrc);
        
        // If this is a proxy URL, add special attributes for lazy loading (CSP compliant - no inline handlers)
        if (proxiedSrc.startsWith('data:image/proxy,')) {
          const originalUrl = decodeURIComponent(proxiedSrc.replace('data:image/proxy,', ''));
          return `<img${before}src="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7" data-proxy-url="${originalUrl}" data-fallback-src="${this.escapeHtml(decodedSrc)}"${after} loading="lazy" style="opacity:0;transition:opacity 0.3s" class="proxy-image">`;
        }
        
        return `<img${before}src="${proxiedSrc}"${after} loading="lazy">`;
      })
      // Step 2: Add security attributes to all links
      .replace(/<a([^>]*?)href="([^"]*?)"([^>]*?)>/gi, 
        '<a$1href="$2"$3 target="_blank" rel="noopener noreferrer nofollow">');
  }

  /**
   * Convert external image URL to proxy URL for COEP bypass
   * Phase 4: Updated to use POST /v1/images/fetch endpoint with Envoy proxy support
   * @param imageUrl - Original image URL
   * @returns Data URL (base64) or original URL if not applicable
   */
  private convertToProxyUrl(imageUrl: string): string {
    try {
      // Performance: Skip obviously local or data URLs
      if (imageUrl.startsWith('data:') || 
          imageUrl.startsWith('/') || 
          imageUrl.startsWith('./') || 
          imageUrl.startsWith('../')) {
        return imageUrl;
      }

      // Parse URL
      const url = new URL(imageUrl);
      
      // Performance: Skip localhost or local development URLs
      if (url.hostname === 'localhost' || 
          url.hostname.includes('127.0.0.1') || 
          url.hostname.includes('192.168.') ||
          url.hostname.includes('10.0.') ||
          url.hostname.endsWith('.local')) {
        return imageUrl;
      }
      
      // Define allowed external domains that need proxy (cached for performance)
      const externalDomains = this.getExternalDomainList();
      
      // Check if this is an external domain that needs proxying
      const needsProxy = externalDomains.some(domain => 
        url.hostname === domain || url.hostname.endsWith('.' + domain)
      );
      
      // Only proxy HTTPS URLs from allowed domains
      if (needsProxy && url.protocol === 'https:') {
        // Performance: Add URL validation for common image formats
        const validImagePath = this.isValidImagePath(url.pathname);
        if (!validImagePath) {
          console.debug('Skipping proxy for non-image URL:', imageUrl);
          return imageUrl;
        }

        // Create a blob URL placeholder that will be replaced by actual image data
        // This approach ensures we don't make the API call during HTML parsing
        // The actual fetching will happen when the image is loaded
        const proxyImageUrl = this.createProxyImageUrl(imageUrl);
        
        // Debug logging in development
        if (process.env.NODE_ENV === 'development') {
          console.debug('Converting image URL to POST API proxy:', {
            original: imageUrl,
            proxy: proxyImageUrl,
            domain: url.hostname
          });
        }
        
        return proxyImageUrl;
      }
      
      // Return original URL for local or non-proxied images
      return imageUrl;
      
    } catch (error) {
      // Enhanced error handling with different error types
      if (error instanceof TypeError && error.message.includes('Invalid URL')) {
        console.warn('Invalid URL format for proxy conversion:', imageUrl);
      } else {
        console.warn('Failed to parse image URL for proxy conversion:', imageUrl, error);
      }
      return imageUrl;
    }
  }

  /**
   * Create a special proxy image URL that triggers POST API fetch
   * Uses a data-proxy-url attribute to store original URL for lazy loading
   * @param originalUrl - Original image URL
   * @returns Special proxy URL that will be handled by image load handler
   */
  private createProxyImageUrl(originalUrl: string): string {
    // Encode the original URL to make it safe for use as a data attribute
    const encodedOriginalUrl = encodeURIComponent(originalUrl);
    
    // Return a special URL format that our image load handler can recognize
    // Format: data:image/proxy,<encoded-original-url>
    return `data:image/proxy,${encodedOriginalUrl}`;
  }

  /**
   * Cache external domains list for performance
   * @returns Array of external domains
   */
  private getExternalDomainList(): string[] {
    // Static cache to avoid repeated array creation
    if (!HTMLRenderingStrategy.externalDomainsCache) {
      HTMLRenderingStrategy.externalDomainsCache = [
        '9to5mac.com',
        'techcrunch.com', 
        'arstechnica.com',
        'theverge.com',
        'engadget.com',
        'wired.com',
        'cdn.mos.cms.futurecdn.net',
        'images.unsplash.com',
        'img.youtube.com',
        // Additional common image domains
        'i.imgur.com',
        'pbs.twimg.com',
        'images.pexels.com',
        'cdn.pixabay.com'
      ];
    }
    return HTMLRenderingStrategy.externalDomainsCache;
  }

  /**
   * Validate if URL path likely contains an image
   * @param pathname - URL pathname
   * @returns True if likely an image
   */
  private isValidImagePath(pathname: string): boolean {
    const imageExtensions = /\.(jpg|jpeg|png|gif|webp|svg|bmp|ico|tiff)(\?|$)/i;
    const imageKeywords = /(image|img|photo|picture|avatar|thumb|logo)/i;
    
    // Check file extension first (most reliable)
    if (imageExtensions.test(pathname)) {
      return true;
    }
    
    // Check for image-related keywords in path
    if (imageKeywords.test(pathname)) {
      return true;
    }
    
    // Default to true for paths that might be dynamic image URLs
    return true;
  }

  // Static cache for external domains (class-level)
  private static externalDomainsCache: string[] | null = null;

  /**
   * Escape HTML to prevent XSS in data attributes
   * @param str - String to escape
   * @returns HTML-escaped string
   */
  private escapeHtml(str: string): string {
    return str
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  /**
   * Decode HTML entities specifically for URLs (separate from general content decoding)
   * Prevents double encoding issues with &amp; -> %26amp%3B
   * SECURITY FIX: Using textContent instead of innerHTML to prevent XSS
   * @param url - URL with potential HTML entities
   * @returns Decoded URL
   */
  public decodeHtmlEntitiesFromUrl(url: string): string {
    // Input validation for security
    if (!url || typeof url !== 'string') {
      return '';
    }

    if (typeof window !== 'undefined') {
      // SECURITY FIX: Use DOMParser to safely decode HTML entities without script execution risk
      try {
        const parser = new DOMParser();
        const doc = parser.parseFromString(`<!doctype html><body>${url}`, 'text/html');
        return doc.body.textContent || '';
      } catch (e) {
        // Fallback to manual decoding if DOMParser fails
        // This is safer than textarea.innerHTML approach
      }
    }
    
    // SECURITY FIX: Safe URL decoding with correct order (decode ampersands last)
    return url
      .replace(/&lt;/g, '<')       // Less common but possible
      .replace(/&gt;/g, '>')       // Less common but possible
      .replace(/&quot;/g, '"')     // Can appear in URL parameters
      .replace(/&#39;/g, "'")      // Can appear in URL parameters
      .replace(/&nbsp;/g, ' ')     // Space encoding
      .replace(/&#x3D;/g, '=')     // Equals sign
      .replace(/&#x26;/g, '&')     // Alternative ampersand encoding
      .replace(/&#38;/g, '&')      // Decimal ampersand encoding
      .replace(/&amp;/g, '&');     // CRITICAL: Decode &amp; LAST to prevent double-decoding
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

/**
 * HTML Content Renderer Component with POST API Image Proxy Support
 * Handles lazy loading of proxied images using the new /v1/images/fetch endpoint
 */
interface HTMLContentRendererProps {
  html: string;
}

const HTMLContentRenderer: React.FC<HTMLContentRendererProps> = ({ html }) => {
  const containerRef = React.useRef<HTMLDivElement>(null);

  React.useEffect(() => {
    if (!containerRef.current) return;

    const container = containerRef.current;
    const proxyImages = container.querySelectorAll('img[data-proxy-url]');

    // Setup CSP-compliant event handlers for proxy images
    const setupImageEventHandlers = (img: HTMLImageElement) => {
      const originalUrl = img.getAttribute('data-proxy-url');
      const fallbackSrc = img.getAttribute('data-fallback-src');
      
      if (!originalUrl) return;

      // Success handler
      const handleImageLoad = () => {
        img.style.opacity = '1';
        img.removeAttribute('data-proxy-url');
        img.removeAttribute('data-fallback-src');
        img.removeEventListener('load', handleImageLoad);
        img.removeEventListener('error', handleImageError);
      };

      // Error handler with fallback
      const handleImageError = () => {
        console.warn('Proxy image load failed, falling back to original URL:', originalUrl);
        if (fallbackSrc) {
          img.src = fallbackSrc;
        }
        img.removeAttribute('data-proxy-url');
        img.removeAttribute('data-fallback-src');
        img.style.opacity = '1';
        img.style.border = '2px solid #ff6b6b';
        img.title = 'Failed to load via proxy, showing original image';
        img.removeEventListener('load', handleImageLoad);
        img.removeEventListener('error', handleImageError);
      };

      // Attach event handlers
      img.addEventListener('load', handleImageLoad);
      img.addEventListener('error', handleImageError);
    };

    // Setup intersection observer for lazy loading
    const imageObserver = new IntersectionObserver(
      async (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            const img = entry.target as HTMLImageElement;
            const originalUrl = img.getAttribute('data-proxy-url');
            
            if (originalUrl) {
              imageObserver.unobserve(img);
              // Setup event handlers first
              setupImageEventHandlers(img);
              // Then load the proxy image
              await loadProxyImage(img, originalUrl);
            }
          }
        }
      },
      { 
        rootMargin: '50px', // Start loading images 50px before they come into view
        threshold: 0.1 
      }
    );

    // Observe all proxy images
    proxyImages.forEach(img => {
      imageObserver.observe(img);
    });

    // Cleanup
    return () => {
      imageObserver.disconnect();
      // Clean up any remaining event listeners
      proxyImages.forEach(img => {
        const imgElement = img as HTMLImageElement;
        imgElement.removeEventListener('load', () => {});
        imgElement.removeEventListener('error', () => {});
      });
    };
  }, [html]);

  // SECURITY FIX: Sanitize HTML before using dangerouslySetInnerHTML
  const sanitizedHtml = React.useMemo(() => {
    return DOMPurify.sanitize(html, {
      ALLOWED_TAGS: ['p', 'br', 'strong', 'b', 'em', 'i', 'u', 'a', 'img', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'ul', 'ol', 'li', 'blockquote', 'code', 'pre'],
      ALLOWED_ATTR: ['href', 'src', 'alt', 'title', 'target', 'rel'],
      FORBID_ATTR: ['onclick', 'onload', 'onerror', 'onmouseover', 'onmouseout', 'onfocus', 'onblur'],
      FORBID_TAGS: ['script', 'object', 'embed', 'form', 'input', 'textarea', 'button', 'select', 'option']
    });
  }, [html]);

  return (
    <div 
      ref={containerRef}
      className="smart-content-html"
      dangerouslySetInnerHTML={{ __html: sanitizedHtml }}
      style={{
        lineHeight: '1.7',
        color: 'var(--text-primary)'
      } as React.CSSProperties}
    />
  );
};

/**
 * Load image through POST API proxy endpoint
 * @param img - Image element to update
 * @param originalUrl - Original image URL to fetch
 */
async function loadProxyImage(img: HTMLImageElement, originalUrl: string): Promise<void> {
  try {
    // Show loading state
    img.style.opacity = '0.5';

    // Make POST request to our image proxy endpoint via /api prefix
    const response = await fetch('/api/backend/v1/images/fetch', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        url: originalUrl,
        options: {
          max_size: 5 * 1024 * 1024, // 5MB max
          timeout: 30 // 30 seconds timeout
        }
      })
    });

    if (!response.ok) {
      throw new Error(`Failed to fetch image: ${response.status} ${response.statusText}`);
    }

    // Get the binary image data
    const imageBlob = await response.blob();
    
    // Validate that we actually got image data
    if (!imageBlob.type.startsWith('image/')) {
      throw new Error(`Invalid content type: ${imageBlob.type}`);
    }
    
    const imageUrl = URL.createObjectURL(imageBlob);

    // Update the image source - this will trigger the event handlers set up earlier
    img.src = imageUrl;

    // Debug logging in development
    if (process.env.NODE_ENV === 'development') {
      console.debug('Successfully loaded proxy image:', {
        original: originalUrl,
        size: imageBlob.size,
        type: imageBlob.type
      });
    }

    // Set up cleanup for object URL (CSP compliant)
    const cleanupObjectUrl = () => {
      URL.revokeObjectURL(imageUrl);
    };
    
    // Use addEventListener instead of onload property (CSP compliant)
    img.addEventListener('load', cleanupObjectUrl, { once: true });
    img.addEventListener('error', cleanupObjectUrl, { once: true });

  } catch (error) {
    console.error('Failed to load proxy image:', originalUrl, error);
    
    // Trigger error handler by setting a non-existent src
    // This will cause the error event handler to fire, which will handle fallback
    img.src = 'data:image/gif;base64,invalid';
    
    // Store error details for debugging
    img.setAttribute('data-proxy-error', error instanceof Error ? error.message : 'Unknown error');
  }
}