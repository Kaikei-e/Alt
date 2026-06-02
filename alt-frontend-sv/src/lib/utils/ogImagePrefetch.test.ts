import { describe, expect, it } from "vitest";
import type { RenderFeed } from "$lib/schema/feed";
import { selectOgImagePrefetchIds } from "./ogImagePrefetch";

/**
 * Pure-function tests for the visual-preview OG-image backfill selector.
 *
 * The bug this guards against: the prefetch effect used a count-based guard
 * (`visibleFeeds.length === prefetchedCount`). When marking a feed as read,
 * the grid removes one card and asynchronously appends a replacement, so the
 * visible count oscillates back to its previous value. A count-based guard
 * then skips the backfill and the replacement card stays a placeholder.
 *
 * Contract:
 *   - select feeds that HAVE an articleId AND LACK an ogImageProxyUrl
 *   - never select a feed whose articleId is already in `requested` (in-flight)
 *   - selection depends ONLY on content, never on the list length
 */

function makeFeed(overrides: Partial<RenderFeed> = {}): RenderFeed {
	return {
		id: overrides.id ?? "feed-1",
		title: "title",
		description: "desc",
		link: "https://example.com/a",
		published: "2026-06-02",
		publishedAtFormatted: "2h ago",
		mergedTagsLabel: "",
		normalizedUrl: "https://example.com/a",
		excerpt: "",
		...overrides,
	};
}

describe("selectOgImagePrefetchIds", () => {
	it("selects feeds that have an articleId but no ogImageProxyUrl", () => {
		const feeds = [
			makeFeed({ id: "1", articleId: "art-1" }),
			makeFeed({ id: "2", articleId: "art-2" }),
		];
		expect(selectOgImagePrefetchIds(feeds, new Set())).toEqual([
			"art-1",
			"art-2",
		]);
	});

	it("skips feeds that already have an ogImageProxyUrl", () => {
		const feeds = [
			makeFeed({ id: "1", articleId: "art-1", ogImageProxyUrl: "/proxy/1" }),
			makeFeed({ id: "2", articleId: "art-2" }),
		];
		expect(selectOgImagePrefetchIds(feeds, new Set())).toEqual(["art-2"]);
	});

	it("skips feeds without an articleId", () => {
		const feeds = [
			makeFeed({ id: "1" }),
			makeFeed({ id: "2", articleId: "art-2" }),
		];
		expect(selectOgImagePrefetchIds(feeds, new Set())).toEqual(["art-2"]);
	});

	it("skips articleIds already requested (idempotent, no double request)", () => {
		const feeds = [
			makeFeed({ id: "1", articleId: "art-1" }),
			makeFeed({ id: "2", articleId: "art-2" }),
		];
		const requested = new Set(["art-1"]);
		expect(selectOgImagePrefetchIds(feeds, requested)).toEqual(["art-2"]);
	});

	it("does not return duplicate ids within a single call", () => {
		const feeds = [
			makeFeed({ id: "1", articleId: "art-dup" }),
			makeFeed({ id: "2", articleId: "art-dup" }),
		];
		expect(selectOgImagePrefetchIds(feeds, new Set())).toEqual(["art-dup"]);
	});

	it("still selects a replacement feed when the visible count is unchanged (mark-as-read regression)", () => {
		// Initial grid: every feed already backfilled except art-1.
		const requested = new Set<string>();
		const initial = [
			makeFeed({ id: "1", articleId: "art-1" }),
			makeFeed({ id: "2", articleId: "art-2", ogImageProxyUrl: "/proxy/2" }),
		];
		const firstPass = selectOgImagePrefetchIds(initial, requested);
		expect(firstPass).toEqual(["art-1"]);
		for (const id of firstPass) requested.add(id);

		// Mark feed "1" as read -> removed; replacement "3" (no proxy) appended.
		// Visible count is unchanged (still 2), which previously skipped backfill.
		const afterMarkRead = [
			makeFeed({ id: "2", articleId: "art-2", ogImageProxyUrl: "/proxy/2" }),
			makeFeed({ id: "3", articleId: "art-3" }),
		];
		expect(selectOgImagePrefetchIds(afterMarkRead, requested)).toEqual([
			"art-3",
		]);
	});
});
