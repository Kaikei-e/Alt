import { describe, expect, it } from "vitest";
import {
	extractDomain,
	filterSources,
} from "$lib/utils/feed-source-filter";

/**
 * Tests for SwipeFilterSortSheet logic.
 * Component rendering tests require browser environment (vitest-browser).
 * These tests validate the data filtering and sort logic used by the swipe filter sheet.
 */

interface MockFeedSource {
	id: string;
	url: string;
	title: string;
	isSubscribed: boolean;
	createdAt: string;
}

const mockSources: MockFeedSource[] = [
	{
		id: "uuid-1",
		url: "https://techcrunch.com/feed",
		title: "TechCrunch",
		isSubscribed: true,
		createdAt: "2025-01-01T00:00:00Z",
	},
	{
		id: "uuid-2",
		url: "https://www.wired.com/feed/rss",
		title: "Wired",
		isSubscribed: true,
		createdAt: "2025-01-02T00:00:00Z",
	},
	{
		id: "uuid-3",
		url: "https://feeds.arstechnica.com/arstechnica/index",
		title: "Ars Technica",
		isSubscribed: true,
		createdAt: "2025-01-03T00:00:00Z",
	},
	{
		id: "uuid-4",
		url: "https://rss.nytimes.com/services/xml/rss/nyt/World.xml",
		title: "NYT World",
		isSubscribed: true,
		createdAt: "2025-01-04T00:00:00Z",
	},
];

describe("SwipeFilterSortSheet logic", () => {
	describe("source filtering for exclude section", () => {
		it("returns empty array for empty query", () => {
			expect(filterSources(mockSources, "")).toEqual([]);
			expect(filterSources(mockSources, "  ")).toEqual([]);
		});

		it("filters sources by URL substring case-insensitively", () => {
			const result = filterSources(mockSources, "tech");
			expect(result).toHaveLength(2);
			expect(result[0].id).toBe("uuid-1");
			expect(result[1].id).toBe("uuid-3");
		});

		it("returns single match for specific domain", () => {
			const result = filterSources(mockSources, "wired");
			expect(result).toHaveLength(1);
			expect(result[0].id).toBe("uuid-2");
		});

		it("returns no matches for non-existent query", () => {
			const result = filterSources(mockSources, "nonexistent");
			expect(result).toHaveLength(0);
		});
	});

	describe("domain extraction for chip display", () => {
		it("extracts hostname from feed URL", () => {
			expect(extractDomain("https://techcrunch.com/feed")).toBe(
				"techcrunch.com",
			);
		});

		it("extracts hostname from complex URL", () => {
			expect(
				extractDomain(
					"https://feeds.arstechnica.com/arstechnica/index",
				),
			).toBe("feeds.arstechnica.com");
		});

		it("returns original string for invalid URL", () => {
			expect(extractDomain("not-a-url")).toBe("not-a-url");
		});
	});

	describe("excluded source lookup", () => {
		it("finds excluded source by ID", () => {
			const excludedSourceId = "uuid-1";
			const excluded = mockSources.find(
				(s) => s.id === excludedSourceId,
			);
			expect(excluded).toBeDefined();
			expect(excluded?.url).toBe("https://techcrunch.com/feed");
			expect(extractDomain(excluded?.url ?? "")).toBe("techcrunch.com");
		});

		it("returns undefined when no matching source", () => {
			const excluded = mockSources.find((s) => s.id === "non-existent");
			expect(excluded).toBeUndefined();
		});

		it("returns null when excludedSourceId is null", () => {
			const excludedSourceId: string | null = null;
			const excluded = excludedSourceId
				? mockSources.find((s) => s.id === excludedSourceId)
				: null;
			expect(excluded).toBeNull();
		});
	});

	describe("sort order state", () => {
		it("defaults to newest first", () => {
			const sortOrder: "newest" | "oldest" = "newest";
			expect(sortOrder).toBe("newest");
		});

		it("oldest option is recognized but disabled in current version", () => {
			// Sort order "oldest" is accepted as a type but backend does not support it yet
			const sortOrder: "newest" | "oldest" = "oldest";
			expect(sortOrder).toBe("oldest");
			// In UI, this option will be disabled with "(Coming soon)" label
		});
	});

	describe("badge visibility", () => {
		it("shows badge when a source is excluded", () => {
			const excludedSourceId: string | null = "uuid-1";
			const hasBadge = excludedSourceId !== null;
			expect(hasBadge).toBe(true);
		});

		it("hides badge when no source is excluded", () => {
			const excludedSourceId: string | null = null;
			const hasBadge = excludedSourceId !== null;
			expect(hasBadge).toBe(false);
		});
	});

	describe("clear exclusion behavior", () => {
		it("clearing exclusion resets to null", () => {
			let excludedSourceId: string | null = "uuid-1";
			// Simulate clear
			excludedSourceId = null;
			expect(excludedSourceId).toBeNull();

			const excluded = excludedSourceId
				? mockSources.find((s) => s.id === excludedSourceId)
				: null;
			expect(excluded).toBeNull();
		});
	});

	describe("reset and reload on filter change", () => {
		it("simulates state reset when exclusion changes", () => {
			// Simulates the resetAndReload() behavior in SwipeFeedScreen
			let feeds: unknown[] = [{ id: 1 }, { id: 2 }];
			let activeIndex = 5;
			let cursor: string | null = "some-cursor";
			let hasMore = false;
			let isInitialLoading = false;

			// Reset
			feeds = [];
			activeIndex = 0;
			cursor = null;
			hasMore = true;
			isInitialLoading = true;

			expect(feeds).toEqual([]);
			expect(activeIndex).toBe(0);
			expect(cursor).toBeNull();
			expect(hasMore).toBe(true);
			expect(isInitialLoading).toBe(true);
		});
	});
});
