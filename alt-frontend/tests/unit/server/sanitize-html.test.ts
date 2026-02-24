/**
 * Server-only HTML sanitization tests
 * Tests for src/lib/server/sanitize-html.ts
 */

import { describe, expect, it } from "vitest";
import {
  createSafeHtml,
  decodeHtmlEntities,
  extractPlainText,
  type SafeHtmlString,
  sanitizeForArticle,
  sanitizeHtml,
  sanitizeToPlainText,
} from "../../../src/lib/server/sanitize-html";

describe("server-only sanitize-html", () => {
  describe("sanitizeForArticle", () => {
    it("should remove dangerous script tags", () => {
      const dangerous = '<p>Safe</p><script>alert("xss")</script>';
      const result = sanitizeForArticle(dangerous);

      expect(result).not.toContain("<script>");
      expect(result).not.toContain("alert");
      expect(result).toContain("<p>Safe</p>");
    });

    it("should preserve safe HTML tags", () => {
      const safe = "<p>Hello <strong>world</strong></p>";
      const result = sanitizeForArticle(safe);

      expect(result).toContain("<p>");
      expect(result).toContain("<strong>");
      expect(result).toContain("Hello");
      expect(result).toContain("world");
    });

    it("should add security attributes to links", () => {
      const withLink = '<a href="https://example.com">Link</a>';
      const result = sanitizeForArticle(withLink);

      expect(result).toContain('rel="noopener noreferrer nofollow ugc"');
      expect(result).toContain('href="https://example.com"');
    });

    it("should remove unsafe URL schemes from links", () => {
      const unsafeLink = '<a href="javascript:alert(1)">Click</a>';
      const result = sanitizeForArticle(unsafeLink);

      expect(result).not.toContain("javascript:");
      expect(result).not.toContain('href="javascript:');
    });

    it("should preserve images with safe attributes", () => {
      const withImage = '<img src="https://example.com/img.jpg" alt="Test">';
      const result = sanitizeForArticle(withImage);

      expect(result).toContain("<img");
      expect(result).toContain('src="https://example.com/img.jpg"');
      expect(result).toContain('alt="Test"');
    });

    it("should handle empty or null input", () => {
      expect(sanitizeForArticle("")).toBe("");
      expect(sanitizeForArticle(null as unknown as string)).toBe("");
      expect(sanitizeForArticle(undefined as unknown as string)).toBe("");
    });

    it("should return SafeHtmlString type", () => {
      const result = sanitizeForArticle("<p>Test</p>");
      // Type check: result should be assignable to SafeHtmlString
      const safe: SafeHtmlString = result;
      expect(typeof safe).toBe("string");
    });
  });

  describe("sanitizeHtml", () => {
    it("should sanitize basic HTML", () => {
      const html = "<div>Content</div>";
      const result = sanitizeHtml(html);

      expect(result).toContain("Content");
      expect(typeof result).toBe("string");
    });

    it("should remove dangerous content", () => {
      const dangerous = "<div><script>alert(1)</script></div>";
      const result = sanitizeHtml(dangerous);

      expect(result).not.toContain("<script>");
      expect(result).not.toContain("alert");
    });
  });

  describe("sanitizeToPlainText", () => {
    it("should remove all HTML tags", () => {
      const html = "<p>Hello <strong>world</strong></p>";
      const result = sanitizeToPlainText(html);

      expect(result).not.toContain("<p>");
      expect(result).not.toContain("<strong>");
      expect(result).toContain("Hello");
      expect(result).toContain("world");
    });

    it("should preserve text content", () => {
      const html = "<div>Text content</div>";
      const result = sanitizeToPlainText(html);

      expect(result).toBe("Text content");
    });

    it("should handle empty input", () => {
      expect(sanitizeToPlainText("")).toBe("");
      expect(sanitizeToPlainText(null as unknown as string)).toBe("");
    });
  });

  describe("decodeHtmlEntities", () => {
    it("should decode HTML entities", () => {
      expect(decodeHtmlEntities("&amp;")).toBe("&");
      expect(decodeHtmlEntities("&lt;")).toBe("<");
      expect(decodeHtmlEntities("&gt;")).toBe(">");
      expect(decodeHtmlEntities("&quot;")).toBe('"');
      expect(decodeHtmlEntities("&#39;")).toBe("'");
    });

    it("should handle complex entity sequences", () => {
      const encoded = "Hello &amp; world &lt;test&gt;";
      const result = decodeHtmlEntities(encoded);

      expect(result).toBe("Hello & world <test>");
    });

    it("should handle empty or invalid input", () => {
      expect(decodeHtmlEntities("")).toBe("");
      expect(decodeHtmlEntities(null as unknown as string)).toBe("");
    });
  });

  describe("extractPlainText", () => {
    it("should extract plain text from HTML", () => {
      const html = "<p>Hello <strong>world</strong></p>";
      const result = extractPlainText(html);

      expect(result).toBe("Hello world");
      expect(result).not.toContain("<");
      expect(result).not.toContain(">");
    });

    it("should decode entities in extracted text", () => {
      const html = "<p>Hello &amp; world</p>";
      const result = extractPlainText(html);

      expect(result).toBe("Hello & world");
    });

    it("should handle empty input", () => {
      expect(extractPlainText("")).toBe("");
    });
  });

  describe("createSafeHtml", () => {
    it("should create SafeHtmlString from string", () => {
      const safe = createSafeHtml("<p>Test</p>");
      const typed: SafeHtmlString = safe;

      expect(typed).toBe("<p>Test</p>");
      expect(typeof typed).toBe("string");
    });
  });
});
