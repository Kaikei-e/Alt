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
