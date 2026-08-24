import { describe, expect, it } from "vitest";
import type { FeedSearchResult, SearchFeedItem } from "$lib/schema/search";
import { transformFeedSearchResult } from "./transformFeedSearchResult";

/**
 * The mobile search path is the one reading surface that never ran its hits
 * through the feed sanitizer the desktop path uses, so a Meilisearch hit whose
 * `description` is a raw `content:encoded` blob reached the card verbatim and
 * rendered as literal markup: `<p><img src="https://…&amp;fit=crop…`.
 */
const RAW_HTML_HIT: SearchFeedItem = {
	title: "Samsung W26 &amp; the Galaxy Z Fold 7",
	description:
		'<p><img src="https://www.zdnet.com/a/img/resize/a380d377/dsc09454.jpg?auto=webp&amp;fit=crop&amp;width=1200" alt="dsc09454"></p>\n<p>Samsung has launched the W26.</p>',
	link: "https://www.zdnet.com/article/samsung-w26",
	published: "2026-02-28T10:00:00Z",
};

describe("transformFeedSearchResult", () => {
	it("unwraps a results envelope", () => {
		const envelope: FeedSearchResult = {
			results: [RAW_HTML_HIT],
			error: null,
		};
		expect(transformFeedSearchResult(envelope)).toHaveLength(1);
	});

	it("returns an empty array for an unexpected shape", () => {
		expect(
			transformFeedSearchResult({} as unknown as FeedSearchResult),
		).toEqual([]);
	});

	it("strips markup out of the description", () => {
		const [hit] = transformFeedSearchResult([RAW_HTML_HIT]);

		expect(hit?.description).not.toContain("<p>");
		expect(hit?.description).not.toContain("<img");
		expect(hit?.description).toContain("Samsung has launched the W26.");
	});

	it("decodes HTML entities left behind after stripping", () => {
		const [hit] = transformFeedSearchResult([RAW_HTML_HIT]);

		expect(hit?.title).toBe("Samsung W26 & the Galaxy Z Fold 7");
		expect(hit?.description).not.toContain("&amp;");
	});

	it("keeps the full description rather than truncating it", () => {
		// SearchFeedItem.description is contractually the untruncated text —
		// SearchResultItem does its own 200-char cut for READ MORE, and a
		// second cut here would silently shorten what READ MORE reveals.
		const long = `<p>${"word ".repeat(400)}</p>`;
		const [hit] = transformFeedSearchResult([
			{ ...RAW_HTML_HIT, description: long },
		]);

		expect(hit?.description.length).toBeGreaterThan(1000);
	});

	it("leaves a description that is pure markup as empty, not as markup", () => {
		const [hit] = transformFeedSearchResult([
			{
				...RAW_HTML_HIT,
				description: '<p><img src="https://example.com/a.jpg"></p>',
			},
		]);

		expect(hit?.description).toBe("");
	});

	it("preserves every non-text field untouched", () => {
		const [hit] = transformFeedSearchResult([RAW_HTML_HIT]);

		expect(hit?.link).toBe(RAW_HTML_HIT.link);
		expect(hit?.published).toBe(RAW_HTML_HIT.published);
	});
});
