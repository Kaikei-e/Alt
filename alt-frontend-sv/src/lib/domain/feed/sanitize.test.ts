import { describe, expect, it } from "vitest";
import { sanitizeFeed } from "./sanitize";

function makeFeed(overrides: Record<string, unknown> = {}) {
	return {
		title: "",
		description: "",
		link: "https://example.com/article",
		published: "2025-01-01T00:00:00Z",
		created_at: "2025-01-01T00:00:00Z",
		...overrides,
	};
}

describe("sanitizeFeed id assignment", () => {
	it("uses article_id as id when present", () => {
		const feed = makeFeed({ article_id: "abc-123" });
		expect(sanitizeFeed(feed).id).toBe("abc-123");
	});

	it("falls back to link when article_id is missing", () => {
		const feed = makeFeed({ link: "https://example.com/article" });
		expect(sanitizeFeed(feed).id).toBe("https://example.com/article");
	});

	it("falls back to empty string when both are missing", () => {
		const feed = makeFeed({ article_id: undefined, link: "" });
		expect(sanitizeFeed(feed).id).toBe("");
	});
});

describe("sanitizeFeed feedId — the id ResolveOgImages matches", () => {
	// `id` and `feedId` name different tables, and the whole point of carrying
	// both is that a card can never reach for the wrong one. `id` is
	// articles.id or, failing that, the link URL — and a URL handed to
	// ResolveOgImages is a `$1::uuid[]` cast error, not a miss, which takes the
	// RPC to 5xx on an endpoint whose circuit breaker is shared with the rest
	// of externalContentEndpoints.
	it("carries feed_id through as feedId, distinct from id", () => {
		const feed = makeFeed({
			feed_id: "11111111-1111-4111-8111-111111111111",
			article_id: "22222222-2222-4222-8222-222222222222",
		});
		const result = sanitizeFeed(feed);
		expect(result.feedId).toBe("11111111-1111-4111-8111-111111111111");
		expect(result.id).toBe("22222222-2222-4222-8222-222222222222");
	});

	it("keeps feedId when the feed has no article at all", () => {
		// The common case for on-demand resolution: no article row, so `id`
		// degrades to the link URL while feeds.id is still perfectly good.
		const feed = makeFeed({
			feed_id: "11111111-1111-4111-8111-111111111111",
			article_id: undefined,
		});
		const result = sanitizeFeed(feed);
		expect(result.feedId).toBe("11111111-1111-4111-8111-111111111111");
		expect(result.id).toBe("https://example.com/article");
	});

	it("leaves feedId undefined rather than falling back to id", () => {
		// Search results come from Meilisearch hits and carry no feeds.id. The
		// card must then resolve nothing — never substitute `id`.
		const feed = makeFeed({ article_id: "abc-123" });
		expect(sanitizeFeed(feed).feedId).toBeUndefined();
	});
});

describe("sanitizeFeed XSS safety — description must NOT be used with {@html}", () => {
	it("entity-decoded description can contain raw angle brackets", () => {
		const feed = makeFeed({
			description: "&lt;img src=x onerror=&quot;alert(1)&quot;&gt;",
		});
		const result = sanitizeFeed(feed);
		// After entity decoding, description contains literal HTML characters.
		// This proves {@html feed.description} would execute XSS.
		// Components MUST use text interpolation {feed.description} instead.
		expect(result.description).toContain("<img");
	});

	it("entity-decoded description can contain script tags", () => {
		const feed = makeFeed({
			description: "&lt;script&gt;alert('XSS')&lt;/script&gt;",
		});
		const result = sanitizeFeed(feed);
		expect(result.description).toContain("<script>");
	});
});

describe("sanitizeFeed HTML entity decoding", () => {
	it("decodes &#39; in title", () => {
		const feed = makeFeed({ title: "Here&#39;s the news" });
		expect(sanitizeFeed(feed).title).toBe("Here's the news");
	});

	it("decodes &amp; in description", () => {
		const feed = makeFeed({ description: "A &amp; B" });
		expect(sanitizeFeed(feed).description).toBe("A & B");
	});

	it("decodes &quot; in title", () => {
		const feed = makeFeed({ title: "&quot;Hello&quot;" });
		expect(sanitizeFeed(feed).title).toBe('"Hello"');
	});

	it("decodes &#039; (with leading zero)", () => {
		const feed = makeFeed({ title: "It&#039;s fine" });
		expect(sanitizeFeed(feed).title).toBe("It's fine");
	});

	it("decodes &apos;", () => {
		const feed = makeFeed({ title: "It&apos;s fine" });
		expect(sanitizeFeed(feed).title).toBe("It's fine");
	});

	it("decodes &#x27;", () => {
		const feed = makeFeed({ title: "It&#x27;s fine" });
		expect(sanitizeFeed(feed).title).toBe("It's fine");
	});

	it("strips tags and decodes entities", () => {
		const feed = makeFeed({
			description: "<p>It&#39;s &amp; more</p>",
		});
		expect(sanitizeFeed(feed).description).toBe("It's & more");
	});
});
