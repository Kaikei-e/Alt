import { describe, expect, it } from "vitest";
import { extractDomain, filterSources } from "$lib/utils/feed-source-filter";

/**
 * Tests for FeedSourceExcludeFilter component-specific behavior.
 * Core filterSources/extractDomain logic is tested in feed-source-filter.spec.ts.
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
		url: "https://feeds.theguardian.com/theguardian/rss",
		title: "",
		isSubscribed: true,
		createdAt: "2025-01-01T00:00:00Z",
	},
	{
		id: "uuid-2",
		url: "https://news.ycombinator.com/rss",
		title: "",
		isSubscribed: true,
		createdAt: "2025-01-02T00:00:00Z",
	},
	{
		id: "uuid-3",
		url: "https://rss.nytimes.com/services/xml/rss/nyt/World.xml",
		title: "",
		isSubscribed: true,
		createdAt: "2025-01-03T00:00:00Z",
	},
	{
		id: "uuid-4",
		url: "https://www.theguardian.com/world/rss",
		title: "",
		isSubscribed: true,
		createdAt: "2025-01-04T00:00:00Z",
	},
];

describe("FeedSourceExcludeFilter logic", () => {
	describe("filterSources (via shared utility)", () => {
		it("returns empty array for empty query", () => {
			expect(filterSources(mockSources, "")).toEqual([]);
			expect(filterSources(mockSources, "  ")).toEqual([]);
		});

		it("filters by URL substring case-insensitively", () => {
			const result = filterSources(mockSources, "guardian");
			expect(result).toHaveLength(2);
			expect(result[0]!.id).toBe("uuid-1");
			expect(result[1]!.id).toBe("uuid-4");
		});

		it("matches partial URL", () => {
			const result = filterSources(mockSources, "ycombinator");
			expect(result).toHaveLength(1);
			expect(result[0]!.id).toBe("uuid-2");
		});

		it("returns no results for non-matching query", () => {
			const result = filterSources(mockSources, "nonexistent");
			expect(result).toHaveLength(0);
		});

		it("limits results to 10", () => {
			const manySources = Array.from({ length: 20 }, (_, i) => ({
				id: `uuid-${i}`,
				url: `https://example${i}.com/rss`,
				title: "",
				isSubscribed: true,
				createdAt: "2025-01-01T00:00:00Z",
			}));
			const result = filterSources(manySources, "example");
			expect(result).toHaveLength(10);
		});

		it("is case-insensitive", () => {
			const result = filterSources(mockSources, "GUARDIAN");
			expect(result).toHaveLength(2);
		});
	});

	describe("extractDomain (via shared utility)", () => {
		it("extracts hostname from URL", () => {
			expect(
				extractDomain("https://feeds.theguardian.com/theguardian/rss"),
			).toBe("feeds.theguardian.com");
		});

		it("returns the input for invalid URLs", () => {
			expect(extractDomain("not-a-url")).toBe("not-a-url");
		});
	});

	describe("excluded source lookup", () => {
		it("finds excluded source by ID", () => {
			const excludedSourceId = "uuid-2";
			const excludedSource = mockSources.find((s) => s.id === excludedSourceId);
			expect(excludedSource).toBeDefined();
			expect(excludedSource?.url).toBe("https://news.ycombinator.com/rss");
		});

		it("returns undefined for non-existent ID", () => {
			const excludedSource = mockSources.find((s) => s.id === "non-existent");
			expect(excludedSource).toBeUndefined();
		});
	});
});
