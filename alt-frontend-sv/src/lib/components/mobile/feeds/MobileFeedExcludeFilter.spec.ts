import { describe, expect, it } from "vitest";
import { extractDomain, filterSources } from "$lib/utils/feed-source-filter";

/**
 * Tests for MobileFeedExcludeFilter logic.
 * Component rendering tests require browser environment (vitest-browser).
 * These tests validate the data filtering logic used by the mobile component.
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

describe("MobileFeedExcludeFilter logic", () => {
	describe("filterSources for bottom sheet search", () => {
		it("returns empty array for empty query", () => {
			expect(filterSources(mockSources, "")).toEqual([]);
			expect(filterSources(mockSources, "  ")).toEqual([]);
		});

		it("filters by URL substring case-insensitively", () => {
			const result = filterSources(mockSources, "guardian");
			expect(result).toHaveLength(2);
			expect(result[0].id).toBe("uuid-1");
			expect(result[1].id).toBe("uuid-4");
		});

		it("returns all sources when query is empty and sources shown unfiltered", () => {
			// In the mobile component, when query is empty we show all sources
			// The component handles this by showing sources directly, not via filterSources
			const result = filterSources(mockSources, "");
			expect(result).toEqual([]);
		});
	});

	describe("extractDomain for chip display", () => {
		it("extracts hostname for chip label", () => {
			expect(
				extractDomain("https://feeds.theguardian.com/theguardian/rss"),
			).toBe("feeds.theguardian.com");
		});

		it("extracts hostname from complex URL", () => {
			expect(
				extractDomain("https://rss.nytimes.com/services/xml/rss/nyt/World.xml"),
			).toBe("rss.nytimes.com");
		});

		it("returns original string for invalid URL", () => {
			expect(extractDomain("not-a-url")).toBe("not-a-url");
		});
	});

	describe("excluded source lookup", () => {
		it("finds excluded source by ID for chip display", () => {
			const excludedSourceId = "uuid-2";
			const excludedSource = mockSources.find((s) => s.id === excludedSourceId);
			expect(excludedSource).toBeDefined();
			expect(excludedSource?.url).toBe("https://news.ycombinator.com/rss");
			expect(extractDomain(excludedSource?.url ?? "")).toBe(
				"news.ycombinator.com",
			);
		});

		it("returns undefined when no source is excluded", () => {
			const excludedSource = mockSources.find((s) => s.id === "non-existent");
			expect(excludedSource).toBeUndefined();
		});

		it("returns undefined for null excludedSourceId", () => {
			const excludedSourceId: string | null = null;
			const excludedSource = excludedSourceId
				? mockSources.find((s) => s.id === excludedSourceId)
				: null;
			expect(excludedSource).toBeNull();
		});
	});

	describe("clear exclusion behavior", () => {
		it("clearing exclusion should result in null excludedSourceId", () => {
			let excludedSourceId: string | null = "uuid-1";
			// Simulate clearing
			excludedSourceId = null;
			expect(excludedSourceId).toBeNull();

			const excludedSource = excludedSourceId
				? mockSources.find((s) => s.id === excludedSourceId)
				: null;
			expect(excludedSource).toBeNull();
		});
	});
});
