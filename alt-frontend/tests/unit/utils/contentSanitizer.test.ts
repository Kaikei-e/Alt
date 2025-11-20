import { sanitizeContent, sanitizeFeedContent } from "@/utils/contentSanitizer";
import { describe, it, expect } from "vitest";

describe("contentSanitizer", () => {
  describe("sanitizeContent", () => {
    it("should return empty string for null or undefined", () => {
      expect(sanitizeContent(null)).toBe("");
      expect(sanitizeContent(undefined)).toBe("");
    });

    it("should sanitize basic HTML tags", () => {
      const input = "<p>Hello <b>World</b></p>";
      const output = sanitizeContent(input);
      expect(output).toBe("<p>Hello <b>World</b></p>");
    });

    it("should remove disallowed tags", () => {
      const input = "<div><script>alert('xss')</script>Hello</div>";
      // div is not in default allowedTags (b, i, em, strong, p, br, a)
      // so it might be stripped or escaped depending on implementation.
      // The current implementation uses sanitize-html default behavior for non-allowed tags which is usually to strip them but keep content.
      // Let's check the default allowedTags in contentSanitizer.ts: ["b", "i", "em", "strong", "p", "br", "a"]
      // So <div> should be stripped.
      const output = sanitizeContent(input);
      expect(output).toBe("Hello");
    });

    it("should allow specified attributes", () => {
      const input = '<a href="https://example.com">Link</a>';
      const output = sanitizeContent(input);
      expect(output).toBe('<a href="https://example.com">Link</a>');
    });

    it("should remove disallowed attributes", () => {
      const input = '<a href="https://example.com" onclick="alert(1)">Link</a>';
      const output = sanitizeContent(input);
      expect(output).toBe('<a href="https://example.com">Link</a>');
    });

    it("should remove dangerous schemes", () => {
      const input = '<a href="javascript:alert(1)">Link</a>';
      const output = sanitizeContent(input);
      // sanitize-html usually removes the href content or the whole attribute
      expect(output).toBe("<a>Link</a>");
    });

    it("should truncate content", () => {
      const input = "a".repeat(2000);
      const output = sanitizeContent(input, { maxLength: 10 });
      expect(output.length).toBeLessThanOrEqual(10);
    });
  });

  describe("sanitizeFeedContent", () => {
    it("should sanitize feed fields", () => {
      const feed = {
        title: "<b>Title</b>",
        description: "<script>alert(1)</script>Desc",
        author: "<i>Author</i>",
        link: "https://example.com",
      };
      const output = sanitizeFeedContent(feed);

      expect(output.title).toBe("<b>Title</b>");
      expect(output.description).toBe("Desc");
      // Author allows no tags
      expect(output.author).toBe("Author");
      expect(output.link).toBe("https://example.com");
    });

    it("should validate links", () => {
      const feed = {
        link: "javascript:alert(1)",
      };
      const output = sanitizeFeedContent(feed);
      expect(output.link).toBe("");
    });
  });
});
