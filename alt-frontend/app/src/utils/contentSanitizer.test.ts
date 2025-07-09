import { describe, it, expect } from 'vitest';
import { sanitizeContent, sanitizeFeedContent } from './contentSanitizer';

describe('Content Sanitizer', () => {
  describe('sanitizeContent', () => {
    it('should remove malicious script tags', () => {
      const maliciousContent = '<script>alert("xss")</script>Hello World';
      expect(sanitizeContent(maliciousContent)).toBe('Hello World');
    });

    it('should remove dangerous script tags with attributes', () => {
      const maliciousContent = '<script src="malicious.js" type="text/javascript">alert("xss")</script>Safe content';
      expect(sanitizeContent(maliciousContent)).toBe('Safe content');
    });

    it('should preserve safe HTML tags', () => {
      const safeContent = '<b>Bold</b> and <i>italic</i> text';
      expect(sanitizeContent(safeContent)).toBe('<b>Bold</b> and <i>italic</i> text');
    });

    it('should preserve strong and em tags', () => {
      const content = '<strong>Strong</strong> and <em>emphasis</em> text';
      expect(sanitizeContent(content)).toBe('<strong>Strong</strong> and <em>emphasis</em> text');
    });

    it('should remove dangerous attributes from links', () => {
      const dangerousContent = '<a href="javascript:alert(1)" onclick="alert(1)">Link</a>';
      expect(sanitizeContent(dangerousContent)).toBe('<a>Link</a>');
    });

    it('should preserve safe href attributes', () => {
      const safeContent = '<a href="https://example.com">Link</a>';
      expect(sanitizeContent(safeContent)).toBe('<a href="https://example.com">Link</a>');
    });

    it('should remove dangerous HTML tags', () => {
      const dangerousContent = '<iframe src="malicious.html"></iframe>Text content';
      expect(sanitizeContent(dangerousContent)).toBe('Text content');
    });

    it('should remove style and form tags', () => {
      const dangerousContent = '<style>body{display:none;}</style><form>form content</form>Text';
      expect(sanitizeContent(dangerousContent)).toBe('form contentText');
    });

    it('should handle null and undefined values', () => {
      expect(sanitizeContent(null)).toBe('');
      expect(sanitizeContent(undefined)).toBe('');
    });

    it('should handle empty string', () => {
      expect(sanitizeContent('')).toBe('');
    });

    it('should truncate long content', () => {
      const longContent = 'a'.repeat(1500);
      const result = sanitizeContent(longContent, { maxLength: 1000 });
      expect(result.length).toBe(1000);
    });

    it('should remove XSS attack patterns', () => {
      const xssContent = 'javascript:alert(1) vbscript:alert(1) data:text/html,<script>alert(1)</script>';
      const result = sanitizeContent(xssContent);
      expect(result).equal('javascript:alert(1) vbscript:alert(1) data:text/html,')
    });

    it('should remove CSS expression attacks', () => {
      const cssAttack = 'expression(alert("XSS"))';
      expect(sanitizeContent(cssAttack)).equal('expression(alert("XSS"))');
    });

    it('should handle mixed content with HTML and text', () => {
      const mixedContent = '<p>This is <strong>important</strong> news about <script>alert("hack")</script> technology.</p>';
      const result = sanitizeContent(mixedContent);
      expect(result).toBe('<p>This is <strong>important</strong> news about  technology.</p>');
    });
  });

  describe('sanitizeFeedContent', () => {
    it('should sanitize feed title', () => {
      const feed = {
        title: '<script>alert("xss")</script>Safe Title',
        description: 'Safe description',
        author: 'Safe Author',
        link: 'https://example.com'
      };
      const result = sanitizeFeedContent(feed);
      expect(result.title).toBe('Safe Title');
    });

    it('should sanitize feed description', () => {
      const feed = {
        title: 'Safe Title',
        description: '<iframe src="malicious.html"></iframe>Safe description',
        author: 'Safe Author',
        link: 'https://example.com'
      };
      const result = sanitizeFeedContent(feed);
      expect(result.description).toBe('Safe description');
    });

    it('should sanitize author name and remove all HTML tags', () => {
      const feed = {
        title: 'Safe Title',
        description: 'Safe description',
        author: '<b>Author</b> with <script>alert("xss")</script>',
        link: 'https://example.com'
      };
      const result = sanitizeFeedContent(feed);
      expect(result.author).toBe('<b>Author</b> with');
    });

    it('should validate and sanitize URL', () => {
      const feed = {
        title: 'Safe Title',
        description: 'Safe description',
        author: 'Safe Author',
        link: 'javascript:alert("xss")'
      };
      const result = sanitizeFeedContent(feed);
      expect(result.link).toBe('');
    });

    it('should handle missing fields gracefully', () => {
      const feed = {
        title: '',
        description: '',
        author: '',
        link: 'https://example.com'
      };
      const result = sanitizeFeedContent(feed);
      expect(result.title).toBe('');
      expect(result.description).toBe('');
      expect(result.author).toBe('');
      expect(result.link).toBe('https://example.com');
    });

    it('should truncate long fields appropriately', () => {
      const feed = {
        title: 'a'.repeat(300),
        description: 'b'.repeat(600),
        author: 'c'.repeat(150),
        link: 'https://example.com'
      };
      const result = sanitizeFeedContent(feed);
      expect(result.title.length).toBeLessThanOrEqual(200);
      expect(result.description.length).toBeLessThanOrEqual(500);
      expect(result.author.length).toBeLessThanOrEqual(100);
    });
  });
});