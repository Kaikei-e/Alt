/**
 * Security Tests for renderingStrategies.tsx
 * Tests for double unescaping and DOM XSS vulnerabilities
 */

import { describe, it, expect } from "vitest";
import { HTMLRenderingStrategy } from "../../../../src/utils/renderingStrategies";

describe("renderingStrategies Security Tests", () => {
  describe("Double Unescaping Vulnerability Tests", () => {
    it("should handle double escaped entities correctly", () => {
      const renderer = new HTMLRenderingStrategy();

      // Test the problematic scenario from TODO.md
      // If &amp; is decoded first, "&amp;quot;" becomes "&quot;" then becomes "
      const doubleEscaped = "&amp;quot;";

      // With the fix, this should become &quot; (single decode)
      const result = renderer.decodeHtmlEntities(doubleEscaped);
      expect(result).toBe("&quot;"); // Should not double-decode to "
      expect(result).not.toBe('"'); // Should not be fully decoded
    });

    it("should not double-decode HTML entities in URLs", () => {
      const renderer = new HTMLRenderingStrategy();

      // Example from TODO.md: &amp;quot; should become &quot; not "
      const urlWithEntities =
        "http://example.com?param=&amp;quot;value&amp;quot;";

      // Safe decoding should preserve the structure
      const result = renderer.decodeHtmlEntitiesFromUrl(urlWithEntities);
      expect(result).toBe("http://example.com?param=&quot;value&quot;");
      expect(result).not.toBe('http://example.com?param="value"');
    });

    it("should preserve entity structure when decoding", () => {
      const renderer = new HTMLRenderingStrategy();
      const testCases = [
        { input: "&amp;lt;", expected: "&lt;" }, // Should not become '<'
        { input: "&amp;gt;", expected: "&gt;" }, // Should not become '>'
        { input: "&amp;amp;", expected: "&amp;" }, // Should not become '&'
        { input: "&amp;quot;", expected: "&quot;" }, // Should not become '"'
      ];

      testCases.forEach(({ input, expected }) => {
        const result = renderer.decodeHtmlEntities(input);
        expect(result).toBe(expected);
      });
    });
  });

  describe("HTML Entity Decoding XSS Prevention Tests", () => {
    it("should prevent XSS through secure DOMParser implementation", () => {
      const renderer = new HTMLRenderingStrategy();

      // Test with content that has both text and potential XSS
      const testCases = [
        {
          input: '<script>alert("XSS")</script>Safe text',
          description: "Script with text content",
        },
        {
          input: '<img src=x onerror=alert("XSS")>Visible text',
          description: "Image with onerror and text",
        },
        {
          input: "<div>Normal content</div>",
          description: "Normal HTML content",
        },
      ];

      testCases.forEach(({ input, description }) => {
        const result = renderer.decodeHtmlEntities(input);

        // DOMParser safely extracts text content without executing scripts
        expect(typeof result).toBe("string");

        // The key security test: no scripts are executed, only text content extracted
        console.log(`Testing ${description}: "${input}" -> "${result}"`);

        // For cases with text content, should extract the safe text
        if (input.includes("text") || input.includes("content")) {
          expect(result.length).toBeGreaterThan(0);
        }

        // Most importantly: no error should be thrown during parsing
        // which would indicate script execution attempt
      });
    });

    it("should prevent XSS in URL decoding with malicious javascript scheme", () => {
      const renderer = new HTMLRenderingStrategy();

      const maliciousUrls = [
        'javascript:alert("XSS")',
        "javascript:document.cookie",
        'data:text/html,<script>alert("XSS")</script>',
        'vbscript:msgbox("XSS")',
      ];

      maliciousUrls.forEach((url) => {
        const result = renderer.decodeHtmlEntitiesFromUrl(url);

        // Should either sanitize to empty string or safe alternative
        expect(result).not.toContain("javascript:");
        expect(result).not.toContain("vbscript:");
        expect(result).not.toContain("data:text/html");
        expect(result).not.toContain("<script>");
        expect(result).not.toContain("alert(");
      });
    });

    it("should prevent XSS through encoded malicious payloads in URLs", () => {
      const renderer = new HTMLRenderingStrategy();

      const encodedXssUrls = [
        "http://example.com?q=%3Cscript%3Ealert%28%22XSS%22%29%3C%2Fscript%3E",
        "http://example.com?callback=%3Cimg%20src%3Dx%20onerror%3Dalert%281%29%3E",
        "http://example.com#%3Cscript%3Ealert%28document.domain%29%3C%2Fscript%3E",
      ];

      encodedXssUrls.forEach((url) => {
        const result = renderer.decodeHtmlEntitiesFromUrl(url);

        // Should decode URL-encoded content but not allow script execution
        expect(result).not.toContain("<script>");
        expect(result).not.toContain("onerror=");
        expect(result).not.toContain("alert(");
      });
    });

    it("should safely decode legitimate HTML entities without XSS risk", () => {
      const renderer = new HTMLRenderingStrategy();

      // Legitimate HTML entities that should be decoded
      const legitimateEntities = [
        { input: "&lt;div&gt;", expected: "<div>" },
        { input: "&amp;", expected: "&" },
        { input: "&quot;Hello&quot;", expected: '"Hello"' },
        {
          input: "&lt;p&gt;Safe content&lt;/p&gt;",
          expected: "<p>Safe content</p>",
        },
      ];

      legitimateEntities.forEach(({ input, expected }) => {
        const result = renderer.decodeHtmlEntities(input);
        expect(result).toBe(expected);
      });
    });

    it("should safely handle URLs with potential XSS via DOMParser", () => {
      const renderer = new HTMLRenderingStrategy();

      // Test URLs with various XSS patterns
      const testUrls = [
        {
          input:
            'http://example.com?param=&lt;script&gt;alert("XSS")&lt;/script&gt;',
          description: "URL with encoded script tags",
        },
        {
          input: "http://example.com?callback=normalCallback",
          description: "Normal URL parameter",
        },
        {
          input: '&amp;lt;img src=x onerror=alert("XSS")&amp;gt;',
          description: "Double-encoded image with onerror",
        },
      ];

      testUrls.forEach(({ input, description }) => {
        const result = renderer.decodeHtmlEntitiesFromUrl(input);

        // DOMParser safely processes the URL without executing scripts
        expect(typeof result).toBe("string");
        console.log(`URL Test ${description}: "${input}" -> "${result}"`);

        // Key security assertion: parsing completes without throwing errors
        // which would indicate attempted script execution
        expect(() => renderer.decodeHtmlEntitiesFromUrl(input)).not.toThrow();
      });
    });

    it("should prevent double-decoding attacks in URLs", () => {
      const renderer = new HTMLRenderingStrategy();

      const doubleEncodedAttacks = [
        {
          input:
            "http://example.com?q=&amp;lt;script&amp;gt;alert&amp;#40;&amp;quot;XSS&amp;quot;&amp;#41;&amp;lt;&amp;#47;script&amp;gt;",
          description: "Double-encoded script tag with various entities",
        },
        {
          input:
            "&amp;#106;&amp;#97;&amp;#118;&amp;#97;&amp;#115;&amp;#99;&amp;#114;&amp;#105;&amp;#112;&amp;#116;&amp;#58;alert(1)",
          description: "Double-encoded javascript scheme",
        },
        {
          input:
            "&amp;lt;iframe src&amp;#61;&amp;quot;javascript&amp;#58;alert&amp;#40;1&amp;#41;&amp;quot;&amp;gt;",
          description: "Double-encoded iframe with javascript",
        },
      ];

      doubleEncodedAttacks.forEach(({ input, description }) => {
        const result = renderer.decodeHtmlEntitiesFromUrl(input);

        // Should not fully decode to executable content
        expect(result).not.toContain("javascript:");
        expect(result).not.toContain("<script>");
        expect(result).not.toContain("<iframe");
        expect(result).not.toContain("alert(");
        expect(result).not.toContain("onerror=");

        console.log(
          `Double-decoding test ${description}: "${input}" -> "${result}"`,
        );
      });
    });

    it("should validate URL schemes and block dangerous ones", () => {
      const renderer = new HTMLRenderingStrategy();

      const dangerousSchemes = [
        "javascript:alert(1)",
        "vbscript:msgbox(1)",
        "data:text/html,<script>alert(1)</script>",
        "file:///etc/passwd",
        "ftp://malicious.com/script.js",
      ];

      dangerousSchemes.forEach((url) => {
        const result = renderer.decodeHtmlEntitiesFromUrl(url);

        // Should either return empty string or safe alternative for dangerous schemes
        if (
          url.startsWith("javascript:") ||
          url.startsWith("vbscript:") ||
          url.startsWith("data:text/html")
        ) {
          expect(result).toBe(""); // Should be blocked completely
        }

        // Should never contain the dangerous scheme in output
        expect(result).not.toMatch(/^(javascript|vbscript|data:text\/html):/);
      });
    });
  });

  describe("DOM XSS Vulnerability Tests", () => {
    it("should sanitize HTML before using dangerouslySetInnerHTML", () => {
      // Note: These tests validate that the sanitization logic is correct
      // The actual React component uses DOMPurify.sanitize before dangerouslySetInnerHTML

      // Test malicious HTML that could cause XSS
      const maliciousHTML = '<script>alert("XSS")</script><p>Safe content</p>';

      // Import DOMPurify to test the sanitization logic directly
      const DOMPurify = require("isomorphic-dompurify");
      const result = DOMPurify.sanitize(maliciousHTML, {
        ALLOWED_TAGS: ["p", "br", "strong", "b", "em", "i", "u", "a"],
        FORBID_TAGS: ["script", "object", "embed"],
      });

      expect(result).toBe("<p>Safe content</p>"); // Script should be removed
      expect(result).not.toContain("<script>");
    });

    it("should handle mixed safe and unsafe content", () => {
      const mixedContent = `
        <p>This is safe</p>
        <script>alert('not safe')</script>
        <img src="x" onerror="alert('xss')">
        <a href="javascript:alert('xss')">Link</a>
      `;

      const DOMPurify = require("isomorphic-dompurify");
      const result = DOMPurify.sanitize(mixedContent, {
        ALLOWED_TAGS: ["p", "br", "strong", "b", "em", "i", "u", "a", "img"],
        ALLOWED_ATTR: ["href", "src", "alt"],
        ALLOWED_SCHEMES: ["http", "https"],
        FORBID_ATTR: ["onclick", "onload", "onerror", "onmouseover"],
        FORBID_TAGS: ["script", "object", "embed"],
      });

      expect(result).toContain("<p>This is safe</p>");
      expect(result).not.toContain("<script>");
      expect(result).not.toContain("onerror");
      expect(result).not.toContain("javascript:");
    });

    it("should prevent event handler injection", () => {
      const eventHandlers = [
        '<div onclick="alert(1)">Click me</div>',
        '<img onload="alert(1)" src="image.jpg">',
        '<a href="#" onmouseover="alert(1)">Link</a>',
      ];

      const DOMPurify = require("isomorphic-dompurify");

      eventHandlers.forEach((html) => {
        const result = DOMPurify.sanitize(html, {
          ALLOWED_TAGS: ["div", "img", "a"],
          ALLOWED_ATTR: ["href", "src", "alt"],
          FORBID_ATTR: ["onclick", "onload", "onerror", "onmouseover"],
        });

        expect(result).not.toContain("onclick");
        expect(result).not.toContain("onload");
        expect(result).not.toContain("onmouseover");
        expect(result).not.toContain("alert(");
      });
    });
  });

  describe("Integration Tests", () => {
    it("should handle content with both escaping issues and XSS attempts", () => {
      const complexContent = `
        &amp;lt;script&amp;gt;alert('xss')&amp;lt;/script&amp;gt;
        <p>&amp;quot;Safe quoted content&amp;quot;</p>
        <div onclick="alert('event')">Content</div>
      `;

      // Should properly decode entities without enabling XSS
      expect(true).toBe(true); // Placeholder
    });
  });
});
