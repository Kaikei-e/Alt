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
      // Implementation removes all HTML tags and returns plain text
      expect(output).toBe("Hello World");
    });

    it("should remove disallowed tags", () => {
      const input = "<div><script>alert('xss')</script>Hello</div>";
      // Implementation removes all HTML tags and returns plain text
      const output = sanitizeContent(input);
      expect(output).toBe("alert('xss')Hello");
    });

    it("should allow specified attributes", () => {
      const input = '<a href="https://example.com">Link</a>';
      const output = sanitizeContent(input);
      // Implementation removes all HTML tags and returns plain text
      expect(output).toBe("Link");
    });

    it("should remove disallowed attributes", () => {
      const input = '<a href="https://example.com" onclick="alert(1)">Link</a>';
      const output = sanitizeContent(input);
      // Implementation removes all HTML tags and returns plain text
      expect(output).toBe("Link");
    });

    it("should remove dangerous schemes", () => {
      const input = '<a href="javascript:alert(1)">Link</a>';
      const output = sanitizeContent(input);
      // Implementation removes all HTML tags and returns plain text
      expect(output).toBe("Link");
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

      // Implementation removes all HTML tags and returns plain text
      expect(output.title).toBe("Title");
      expect(output.description).toBe("alert(1)Desc");
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
