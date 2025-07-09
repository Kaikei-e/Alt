import { describe, expect, it } from "vitest";
import { sanitizeUrl, addSecurityAttributes, isExternalLink } from "./linkSafety";

// Mock window.location for testing
Object.defineProperty(window, 'location', {
  value: {
    origin: 'https://example.com'
  },
  writable: true
});

describe("Link Safety Utilities", () => {
  describe("sanitizeUrl", () => {
    it("should return safe URLs unchanged", () => {
      const safeUrls = [
        "https://example.com",
        "http://example.com",
        "https://example.com/path",
      ];

      safeUrls.forEach((url) => {
        expect(sanitizeUrl(url)).toBe(url);
      });
    });

    it("should return # for dangerous URLs", () => {
      const dangerousUrls = [
        "javascript:alert('XSS')",
        "data:text/html,<script>alert('XSS')</script>",
        "vbscript:alert('XSS')",
        "file:///etc/passwd",
      ];

      dangerousUrls.forEach((url) => {
        expect(sanitizeUrl(url)).toBe("#");
      });
    });

    it("should return # for malformed URLs", () => {
      const malformedUrls = [
        "not-a-url",
        "http://",
        "",
        "   ",
      ];

      malformedUrls.forEach((url) => {
        expect(sanitizeUrl(url)).toBe("#");
      });
    });
  });

  describe("addSecurityAttributes", () => {
    it("should add security attributes for external links", () => {
      const externalUrl = "https://external.com";
      const result = addSecurityAttributes(externalUrl);

      expect(result).toEqual({
        href: externalUrl,
        rel: "noopener noreferrer",
        target: "_blank"
      });
    });

    it("should not add security attributes for internal links", () => {
      const internalUrl = "https://example.com/internal";
      const result = addSecurityAttributes(internalUrl);

      expect(result).toEqual({
        href: internalUrl
      });
    });

    it("should return safe fallback for dangerous URLs", () => {
      const dangerousUrl = "javascript:alert('XSS')";
      const result = addSecurityAttributes(dangerousUrl);

      expect(result).toEqual({ href: "#" });
    });
  });

  describe("isExternalLink", () => {
    it("should return true for external URLs", () => {
      const externalUrls = [
        "https://external.example.com",
        "http://another.example.com",
        "https://different.example.org/path",
      ];

      externalUrls.forEach((url) => {
        expect(isExternalLink(url)).toBe(true);
      });
    });

    it("should return false for internal URLs", () => {
      const internalUrls = [
        "https://example.com",
        "https://example.com/path",
        "https://example.com/path?query=value",
      ];

      internalUrls.forEach((url) => {
        expect(isExternalLink(url)).toBe(false);
      });
    });

    it("should return false for malformed URLs", () => {
      const malformedUrls = [
        "not-a-url",
        "http://",
        "",
      ];

      malformedUrls.forEach((url) => {
        expect(isExternalLink(url)).toBe(false);
      });
    });
  });
});