import { describe, expect, it } from "vitest";
import * as v from "valibot";
import { feedUrlSchema } from "./feedUrlSchema";

describe("Feed URL Schema", () => {
  describe("feedUrlSchema", () => {
    it("should accept valid RSS feed URLs", () => {
      const validRssUrls = [
        { feed_url: "https://example.com/rss" },
        { feed_url: "https://example.com/rss.xml" },
        { feed_url: "https://example.com/feed.xml" },
        { feed_url: "https://example.com/atom.xml" },
        { feed_url: "https://feeds.feedburner.com/example" },
        { feed_url: "https://example.com/feeds/" },
        { feed_url: "https://example.com/feed" },
        { feed_url: "https://example.com/atom" },
      ];

      validRssUrls.forEach((feedUrl) => {
        const result = v.safeParse(feedUrlSchema, feedUrl);
        expect(result.success).toBe(true);
        if (result.success) {
          expect(result.output).toEqual(feedUrl);
        }
      });
    });

    it("should reject non-RSS URLs", () => {
      const nonRssUrls = [
        { feed_url: "https://example.com" },
        { feed_url: "https://example.com/page.html" },
        { feed_url: "https://example.com/about" },
        { feed_url: "https://example.com/blog" },
      ];

      nonRssUrls.forEach((feedUrl) => {
        const result = v.safeParse(feedUrlSchema, feedUrl);
        expect(result.success).toBe(false);
        if (!result.success) {
          expect(result.issues[0].message).toBe("URL does not appear to be a valid RSS or Atom feed");
        }
      });
    });

    it("should reject dangerous URLs", () => {
      const dangerousUrls = [
        { feed_url: "javascript:alert('XSS')" },
        { feed_url: "data:text/html,<script>alert('XSS')</script>" },
        { feed_url: "vbscript:alert('XSS')" },
        { feed_url: "file:///etc/passwd" },
      ];

      dangerousUrls.forEach((feedUrl) => {
        const result = v.safeParse(feedUrlSchema, feedUrl);
        expect(result.success).toBe(false);
        if (!result.success) {
          expect(result.issues[0].message).toBe("Invalid or unsafe URL");
        }
      });
    });

    it("should reject malformed URLs", () => {
      const malformedUrls = [
        { feed_url: "not-a-url" },
        { feed_url: "http://" },
        { feed_url: "" },
        { feed_url: "   " },
      ];

      malformedUrls.forEach((feedUrl) => {
        const result = v.safeParse(feedUrlSchema, feedUrl);
        expect(result.success).toBe(false);
        if (!result.success) {
          expect(result.issues[0].message).toBe("Invalid or unsafe URL");
        }
      });
    });

    it("should handle edge cases", () => {
      const edgeCases = [
        { feed_url: "https://example.com/rss?format=xml" },
        { feed_url: "https://example.com/feed.xml#latest" },
        { feed_url: "https://subdomain.example.com/feeds/all" },
      ];

      edgeCases.forEach((feedUrl) => {
        const result = v.safeParse(feedUrlSchema, feedUrl);
        expect(result.success).toBe(true);
      });
    });
  });
});