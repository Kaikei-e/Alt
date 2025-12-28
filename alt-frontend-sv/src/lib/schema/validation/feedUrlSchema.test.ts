
import { describe, it, expect } from "vitest";
import * as v from "valibot";
import { feedUrlSchema } from "./feedUrlSchema";

describe("feedUrlSchema", () => {
  it("should validate a valid RSS URL", () => {
    const validUrl = "https://example.com/rss.xml";
    const result = v.safeParse(feedUrlSchema, { feed_url: validUrl });
    expect(result.success).toBe(true);
  });

  it("should validate a valid URL with trailing space", () => {
    const urlWithSpace = "https://example.com/rss.xml ";
    const result = v.safeParse(feedUrlSchema, { feed_url: urlWithSpace });
    expect(result.success).toBe(true);
  });

  it("should validate a valid URL with leading space", () => {
    const urlWithSpace = " https://example.com/rss.xml";
    const result = v.safeParse(feedUrlSchema, { feed_url: urlWithSpace });
    expect(result.success).toBe(true);
  });

  it("should fail for invalid URL", () => {
    const invalidUrl = "not-a-url";
    const result = v.safeParse(feedUrlSchema, { feed_url: invalidUrl });
    expect(result.success).toBe(false);
  });

  it("should fail for URL not looking like a feed", () => {
    const notFeedUrl = "https://example.com/some/random/page";
    const result = v.safeParse(feedUrlSchema, { feed_url: notFeedUrl });
    expect(result.success).toBe(false);
  })
});
