import { describe, it, expect } from "vitest";
import { escapeHtml, escapeForDisplay } from "../../../src/utils/htmlEscape";

describe("HTML Escape Utilities", () => {
  describe("escapeHtml", () => {
    it("should escape HTML special characters", () => {
      expect(escapeHtml('<script>alert("xss")</script>')).toBe(
        "&lt;script&gt;alert(&quot;xss&quot;)&lt;/script&gt;",
      );
    });

    it("should handle empty and null values", () => {
      expect(escapeHtml("")).toBe("");
      expect(escapeHtml(null)).toBe("");
      expect(escapeHtml(undefined)).toBe("");
    });

    it("should preserve safe characters", () => {
      expect(escapeHtml("Hello World 123")).toBe("Hello World 123");
    });

    it("should escape ampersands first to prevent double escaping", () => {
      expect(escapeHtml("&lt;script&gt;")).toBe("&amp;lt;script&amp;gt;");
    });

    it("should handle XSS attack vectors", () => {
      const xssVectors = [
        "<img src=x onerror=alert(1)>",
        "javascript:alert(1)",
        '"><script>alert(1)</script>',
        "';alert(1);//",
        "<svg onload=alert(1)>",
      ];

      xssVectors.forEach((vector) => {
        const escaped = escapeHtml(vector);
        // HTMLタグが適切にエスケープされていることを確認
        expect(escaped).not.toContain("<script>");
        expect(escaped).not.toContain("<img");
        expect(escaped).not.toContain("<svg");
        // 危険な文字が適切にエスケープされていることを確認
        expect(escaped).not.toContain("<");
        expect(escaped).not.toContain(">");
      });
    });
  });

  describe("escapeForDisplay", () => {
    it("should escape HTML for safe display", () => {
      const query = '<script>alert("search")</script>';
      expect(escapeForDisplay(query)).toBe(
        "&lt;script&gt;alert(&quot;search&quot;)&lt;/script&gt;",
      );
    });

    it("should handle search queries with special characters", () => {
      expect(escapeForDisplay("React & Vue")).toBe("React &amp; Vue");
      expect(escapeForDisplay("Price < 100")).toBe("Price &lt; 100");
      expect(escapeForDisplay("item > 50")).toBe("item &gt; 50");
    });

    it("should handle empty search query", () => {
      expect(escapeForDisplay("")).toBe("");
    });
  });
});
