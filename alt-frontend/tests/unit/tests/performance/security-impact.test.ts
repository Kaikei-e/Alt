import { beforeAll, describe, expect, test } from "vitest";
import { validateUrl } from "@/schema/validation/urlValidation";
import { sanitizeContent } from "@/utils/contentSanitizer";
import { escapeHtml } from "@/utils/htmlEscape";

describe("Security Performance Tests - PROTECTED", () => {
  beforeAll(() => {
    // パフォーマンス測定の準備
    if (typeof performance === "undefined") {
      throw new Error("Performance API not available");
    }
  });

  describe("Content Sanitization Performance - PROTECTED", () => {
    test("content sanitization should be performant - PROTECTED", () => {
      const largeContent = "<p>Content</p>".repeat(1000);

      const startTime = performance.now();
      const sanitized = sanitizeContent(largeContent);
      const endTime = performance.now();

      const processingTime = endTime - startTime;

      // 1000要素の処理が500ms以内であることを確認
      expect(processingTime).toBeLessThan(500);
      expect(sanitized).toBeDefined();
      expect(sanitized).toContain("Content");
    });

    test("should handle large malicious content efficiently - PROTECTED", () => {
      const maliciousContent =
        '<script>alert("xss")</script>'.repeat(10) + "<p>Safe content</p>";

      const startTime = performance.now();
      const sanitized = sanitizeContent(maliciousContent);
      const endTime = performance.now();

      const processingTime = endTime - startTime;

      // 10個の悪意のあるスクリプトタグの処理が300ms以内であることを確認
      expect(processingTime).toBeLessThan(300);
      expect(sanitized).not.toContain("<script>");
      expect(sanitized).toContain("Safe content");
    });

    test("should handle deeply nested HTML efficiently - PROTECTED", () => {
      const deepNested =
        "<div>".repeat(100) +
        '<script>alert("xss")</script>' +
        "</div>".repeat(100);

      const startTime = performance.now();
      const sanitized = sanitizeContent(deepNested);
      const endTime = performance.now();

      const processingTime = endTime - startTime;

      // 深いネストの処理が200ms以内であることを確認
      expect(processingTime).toBeLessThan(200);
      expect(sanitized).not.toContain("<script>");
    });

    test("should handle mixed safe and malicious content - PROTECTED", () => {
      const mixedContent = [
        "<p>Safe paragraph</p>",
        '<script>alert("xss")</script>',
        "<b>Bold text</b>",
        "<i>Italic text</i>",
        "<strong>Strong text</strong>",
      ]
        .join("")
        .repeat(50);

      const startTime = performance.now();
      const sanitized = sanitizeContent(mixedContent);
      const endTime = performance.now();

      const processingTime = endTime - startTime;

      // 混在コンテンツの処理が500ms以内であることを確認
      expect(processingTime).toBeLessThan(500);
      expect(sanitized).toContain("Safe paragraph");
      expect(sanitized).toContain("Bold text");
      expect(sanitized).not.toContain("<script>");
    });
  });

  describe("HTML Escaping Performance - PROTECTED", () => {
    test("HTML escaping should be performant - PROTECTED", () => {
      const largeText =
        "Text with <script> and \"quotes\" and 'apostrophes'".repeat(1000);

      const startTime = performance.now();
      const escaped = escapeHtml(largeText);
      const endTime = performance.now();

      const processingTime = endTime - startTime;

      // 大量テキストの処理が150ms以内であることを確認 (CI環境対応)
      expect(processingTime).toBeLessThan(150);
      expect(escaped).toBeDefined();
      expect(escaped).toContain("&lt;script&gt;");
      expect(escaped).toContain("&quot;");
      expect(escaped).toContain("&#x27;");
    });

    test("should handle special characters efficiently - PROTECTED", () => {
      const specialChars = "<>&\"'&amp;".repeat(10000);

      const startTime = performance.now();
      const escaped = escapeHtml(specialChars);
      const endTime = performance.now();

      const processingTime = endTime - startTime;

      // 特殊文字の大量処理が100ms以内であることを確認 (CI環境対応)
      expect(processingTime).toBeLessThan(100);
      expect(escaped).toBeDefined();
      expect(escaped).toContain("&lt;");
      expect(escaped).toContain("&gt;");
      expect(escaped).toContain("&amp;");
    });
  });

  describe("URL Validation Performance - PROTECTED", () => {
    test("URL validation should be performant - PROTECTED", () => {
      const urls = [
        "https://example.com",
        "http://test.com",
        "javascript:alert(1)",
        "data:text/html,<script>alert(1)</script>",
        "https://very-long-domain-name.example.com/very/long/path/with/many/segments",
        "invalid-url",
        "ftp://example.com",
        "https://example.com/path?query=value&another=value2",
      ];

      const startTime = performance.now();

      // 大量のURL検証を実行
      for (let i = 0; i < 500; i++) {
        urls.forEach((url) => validateUrl(url));
      }

      const endTime = performance.now();
      const processingTime = endTime - startTime;

      // 4000回のURL検証が1000ms以内であることを確認
      expect(processingTime).toBeLessThan(1000);
    });

    test("should handle malicious URLs efficiently - PROTECTED", () => {
      const maliciousUrls = [
        "javascript:alert(1)",
        "data:text/html,<script>alert(1)</script>",
        "vbscript:alert(1)",
        "javascript:void(0)",
        "data:image/svg+xml;base64,PHN2ZyBvbmxvYWQ9YWxlcnQoMSk+PC9zdmc+",
        'javascript:eval(atob("YWxlcnQoMSk="))',
        "javascript:;alert(1)",
        "javascript://alert(1)",
        "javascript:/**/alert(1)",
      ];

      const startTime = performance.now();

      // 悪意のあるURLの大量検証
      for (let i = 0; i < 500; i++) {
        maliciousUrls.forEach((url) => validateUrl(url));
      }

      const endTime = performance.now();
      const processingTime = endTime - startTime;

      // 4500回の悪意のあるURL検証が600ms以内であることを確認
      expect(processingTime).toBeLessThan(600);
    });
  });

  describe("Memory Usage Tests - PROTECTED", () => {
    test("should not cause memory leaks during sanitization - PROTECTED", () => {
      const initialMemory = process.memoryUsage().heapUsed;

      // 大量のサニタイゼーション処理を実行
      for (let i = 0; i < 1000; i++) {
        const content =
          '<script>alert("xss")</script>'.repeat(10) + "Safe content";
        sanitizeContent(content);
      }

      // ガベージコレクションを促進
      if (global.gc) {
        global.gc();
      }

      const finalMemory = process.memoryUsage().heapUsed;
      const memoryIncrease = finalMemory - initialMemory;

      // メモリ使用量の増加が25MB以内であることを確認 (CI環境対応)
      expect(memoryIncrease).toBeLessThan(25 * 1024 * 1024); // 25MB
    });

    test("should handle large content without excessive memory usage - PROTECTED", () => {
      const initialMemory = process.memoryUsage().heapUsed;

      // 大きなコンテンツを処理
      const largeContent =
        "<p>".repeat(5000) + "Large content" + "</p>".repeat(5000);
      const result = sanitizeContent(largeContent);

      const finalMemory = process.memoryUsage().heapUsed;
      const memoryIncrease = finalMemory - initialMemory;

      // メモリ使用量の増加が50MB以内であることを確認
      expect(memoryIncrease).toBeLessThan(50 * 1024 * 1024); // 50MB
      expect(result).toBeDefined();
    });
  });

  describe("Concurrent Processing Tests - PROTECTED", () => {
    test("should handle concurrent sanitization requests - PROTECTED", async () => {
      const concurrentRequests = 20;
      const content =
        '<script>alert("xss")</script>'.repeat(3) + "<p>Safe content</p>";

      const startTime = performance.now();

      // 並行してサニタイゼーション処理を実行
      const promises = Array.from({ length: concurrentRequests }, () =>
        Promise.resolve(sanitizeContent(content)),
      );

      const results = await Promise.all(promises);

      const endTime = performance.now();
      const processingTime = endTime - startTime;

      // 20並行処理が500ms以内であることを確認
      expect(processingTime).toBeLessThan(500);
      expect(results.length).toBe(concurrentRequests);
      results.forEach((result) => {
        expect(result).not.toContain("<script>");
        expect(result).toContain("Safe content");
      });
    });

    test("should handle concurrent URL validation - PROTECTED", async () => {
      const concurrentRequests = 100;
      const urls = [
        "https://example.com",
        "javascript:alert(1)",
        "data:text/html,<script>alert(1)</script>",
        "https://test.com/path",
      ];

      const startTime = performance.now();

      // 並行してURL検証を実行
      const promises = Array.from({ length: concurrentRequests }, (_, i) =>
        Promise.resolve(validateUrl(urls[i % urls.length])),
      );

      const results = await Promise.all(promises);

      const endTime = performance.now();
      const processingTime = endTime - startTime;

      // 100並行URL検証が200ms以内であることを確認
      expect(processingTime).toBeLessThan(200);
      expect(results.length).toBe(concurrentRequests);
    });
  });

  describe("Edge Case Performance - PROTECTED", () => {
    test("should handle empty content efficiently - PROTECTED", () => {
      const startTime = performance.now();

      // 空のコンテンツを大量処理
      for (let i = 0; i < 10000; i++) {
        sanitizeContent("");
      }

      const endTime = performance.now();
      const processingTime = endTime - startTime;

      // 10000回の空文字処理が150ms以内であることを確認 (CI環境対応)
      expect(processingTime).toBeLessThan(150);
    });

    test("should handle null/undefined efficiently - PROTECTED", () => {
      const startTime = performance.now();

      // null/undefinedの大量処理
      for (let i = 0; i < 10000; i++) {
        sanitizeContent(null);
        sanitizeContent(undefined);
      }

      const endTime = performance.now();
      const processingTime = endTime - startTime;

      // 20000回のnull/undefined処理が100ms以内であることを確認 (CI環境対応)
      expect(processingTime).toBeLessThan(100);
    });
  });
});
