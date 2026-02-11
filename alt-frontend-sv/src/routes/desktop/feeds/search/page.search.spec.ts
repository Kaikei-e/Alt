import { describe, expect, it, vi, beforeEach } from "vitest";
import type { RenderFeed } from "$lib/schema/feed";
import type { FeedSearchResult } from "$lib/schema/search";
import { createRenderFeed } from "../../../../../tests/fixtures/feeds";

/**
 * Tests for desktop search page infinite scroll state management.
 * Follows the same pattern as page.logic.spec.ts - testing state logic
 * without browser rendering.
 */

// Create a batch of mock search results
function createSearchResult(
	startIndex: number,
	count: number,
	hasNextPage: boolean,
	nextCursor: number | null,
): FeedSearchResult {
	const results = Array.from({ length: count }, (_, i) => ({
		title: `Feed ${startIndex + i}`,
		description: `Description for feed ${startIndex + i}`,
		link: `https://example.com/feed-${startIndex + i}`,
		published: "2025-12-22T14:00:00Z",
		author: { name: "Test Author" },
	}));
	return {
		results,
		error: null,
		next_cursor: nextCursor,
		has_more: hasNextPage,
	};
}

// State manager that mirrors the page component's infinite scroll logic
class SearchPageState {
	feeds: RenderFeed[] = [];
	cursor: number | null = null;
	hasNextPage = false;
	isLoading = false;
	isFetchingNextPage = false;
	lastSearchedQuery = "";
	error: Error | null = null;

	// Modal navigation state
	selectedFeed: RenderFeed | null = null;
	currentIndex = -1;
	isModalOpen = false;

	private searchFn: (
		query: string,
		cursor?: number,
		limit?: number,
	) => Promise<FeedSearchResult>;

	constructor(
		searchFn: (
			query: string,
			cursor?: number,
			limit?: number,
		) => Promise<FeedSearchResult>,
	) {
		this.searchFn = searchFn;
	}

	async handleSearch(query: string): Promise<void> {
		if (!query.trim()) {
			this.feeds = [];
			this.error = null;
			this.lastSearchedQuery = "";
			this.cursor = null;
			this.hasNextPage = false;
			return;
		}

		try {
			this.isLoading = true;
			this.error = null;
			this.lastSearchedQuery = query.trim();

			const result = await this.searchFn(query.trim(), undefined, 20);

			if (result.error) {
				this.error = new Error(result.error);
				this.feeds = [];
				this.cursor = null;
				this.hasNextPage = false;
				return;
			}

			this.feeds = (result.results ?? []).map((item, i) =>
				createRenderFeed(`feed-${i}`, item.link),
			);
			this.cursor = result.next_cursor ?? null;
			this.hasNextPage = result.has_more ?? false;
		} catch (err) {
			this.error = err as Error;
			this.feeds = [];
			this.cursor = null;
			this.hasNextPage = false;
		} finally {
			this.isLoading = false;
		}
	}

	static readonly MAX_SEARCH_RESULTS = 200;

	async loadMore(): Promise<void> {
		if (this.isFetchingNextPage || !this.hasNextPage) return;
		if (this.feeds.length >= SearchPageState.MAX_SEARCH_RESULTS) {
			this.hasNextPage = false;
			return;
		}
		this.isFetchingNextPage = true;
		try {
			const result = await this.searchFn(
				this.lastSearchedQuery,
				this.cursor ?? undefined,
				20,
			);
			if (result.error) {
				this.hasNextPage = false;
				return;
			}
			const newFeeds = (result.results ?? []).map((item, i) =>
				createRenderFeed(`feed-${this.feeds.length + i}`, item.link),
			);
			if (newFeeds.length === 0) {
				this.hasNextPage = false;
				return;
			}
			this.feeds = [...this.feeds, ...newFeeds];
			this.cursor = result.next_cursor ?? null;
			this.hasNextPage = result.has_more ?? false;
		} finally {
			this.isFetchingNextPage = false;
		}
	}

	get hasPrevious(): boolean {
		return this.currentIndex > 0;
	}

	get hasNext(): boolean {
		return (
			(this.currentIndex >= 0 && this.currentIndex < this.feeds.length - 1) ||
			(this.currentIndex === this.feeds.length - 1 && this.hasNextPage)
		);
	}

	handleSelectFeed(feed: RenderFeed, index: number): void {
		this.selectedFeed = feed;
		this.currentIndex = index;
		this.isModalOpen = true;
	}

	handlePrevious(): void {
		if (this.currentIndex > 0) {
			this.selectedFeed = this.feeds[this.currentIndex - 1];
			this.currentIndex = this.currentIndex - 1;
		}
	}

	async handleNext(): Promise<void> {
		if (this.currentIndex >= 0 && this.currentIndex < this.feeds.length - 1) {
			this.selectedFeed = this.feeds[this.currentIndex + 1];
			this.currentIndex = this.currentIndex + 1;
		} else if (this.hasNextPage && !this.isFetchingNextPage) {
			await this.loadMore();
			if (this.currentIndex < this.feeds.length - 1) {
				this.selectedFeed = this.feeds[this.currentIndex + 1];
				this.currentIndex = this.currentIndex + 1;
			}
		}
	}
}

describe("Desktop Search Infinite Scroll State", () => {
	let mockSearch: ReturnType<
		typeof vi.fn<
			(
				query: string,
				cursor?: number,
				limit?: number,
			) => Promise<FeedSearchResult>
		>
	>;
	let state: SearchPageState;

	beforeEach(() => {
		mockSearch = vi.fn();
		state = new SearchPageState(mockSearch);
	});

	describe("initial search", () => {
		it("calls API with limit 20", async () => {
			mockSearch.mockResolvedValue(createSearchResult(0, 20, true, 20));

			await state.handleSearch("test query");

			expect(mockSearch).toHaveBeenCalledWith("test query", undefined, 20);
		});

		it("stores pagination state from response", async () => {
			mockSearch.mockResolvedValue(createSearchResult(0, 20, true, 20));

			await state.handleSearch("test query");

			expect(state.feeds).toHaveLength(20);
			expect(state.cursor).toBe(20);
			expect(state.hasNextPage).toBe(true);
			expect(state.lastSearchedQuery).toBe("test query");
		});

		it("sets hasNextPage=false when results exhausted", async () => {
			mockSearch.mockResolvedValue(createSearchResult(0, 5, false, null));

			await state.handleSearch("rare query");

			expect(state.feeds).toHaveLength(5);
			expect(state.cursor).toBeNull();
			expect(state.hasNextPage).toBe(false);
		});

		it("handles API error", async () => {
			mockSearch.mockResolvedValue({
				results: [],
				error: "Search failed",
				next_cursor: null,
				has_more: false,
			});

			await state.handleSearch("test");

			expect(state.error).toBeInstanceOf(Error);
			expect(state.error?.message).toBe("Search failed");
			expect(state.feeds).toHaveLength(0);
			expect(state.hasNextPage).toBe(false);
		});

		it("handles empty query by clearing state", async () => {
			// First do a real search
			mockSearch.mockResolvedValue(createSearchResult(0, 10, true, 10));
			await state.handleSearch("test");
			expect(state.feeds).toHaveLength(10);

			// Now clear with empty query
			await state.handleSearch("");

			expect(state.feeds).toHaveLength(0);
			expect(state.cursor).toBeNull();
			expect(state.hasNextPage).toBe(false);
			expect(state.lastSearchedQuery).toBe("");
		});
	});

	describe("loadMore", () => {
		beforeEach(async () => {
			mockSearch.mockResolvedValueOnce(createSearchResult(0, 20, true, 20));
			await state.handleSearch("test query");
			mockSearch.mockClear();
		});

		it("uses cursor for next batch", async () => {
			mockSearch.mockResolvedValue(createSearchResult(20, 20, true, 40));

			await state.loadMore();

			expect(mockSearch).toHaveBeenCalledWith("test query", 20, 20);
		});

		it("appends results to existing feeds", async () => {
			mockSearch.mockResolvedValue(createSearchResult(20, 20, true, 40));

			await state.loadMore();

			expect(state.feeds).toHaveLength(40);
			expect(state.cursor).toBe(40);
			expect(state.hasNextPage).toBe(true);
		});

		it("updates hasNextPage when results exhausted", async () => {
			mockSearch.mockResolvedValue(createSearchResult(20, 10, false, null));

			await state.loadMore();

			expect(state.feeds).toHaveLength(30);
			expect(state.hasNextPage).toBe(false);
			expect(state.cursor).toBeNull();
		});

		it("does not load when already loading", async () => {
			state.isFetchingNextPage = true;

			await state.loadMore();

			expect(mockSearch).not.toHaveBeenCalled();
		});

		it("does not load when hasNextPage is false", async () => {
			state.hasNextPage = false;

			await state.loadMore();

			expect(mockSearch).not.toHaveBeenCalled();
		});

		it("resets isFetchingNextPage after completion", async () => {
			mockSearch.mockResolvedValue(createSearchResult(20, 20, true, 40));

			await state.loadMore();

			expect(state.isFetchingNextPage).toBe(false);
		});

		it("resets isFetchingNextPage on API error", async () => {
			mockSearch.mockResolvedValue({
				results: [],
				error: "Server error",
				next_cursor: null,
				has_more: false,
			});

			await state.loadMore();

			// Should not append on error, but isFetchingNextPage should reset
			expect(state.isFetchingNextPage).toBe(false);
			expect(state.feeds).toHaveLength(20); // Original 20 unchanged
		});
	});

	describe("query change resets state", () => {
		it("resets cursor and hasNextPage on new search", async () => {
			// First search with pagination
			mockSearch.mockResolvedValueOnce(createSearchResult(0, 20, true, 20));
			await state.handleSearch("first query");
			expect(state.hasNextPage).toBe(true);
			expect(state.cursor).toBe(20);

			// New search should reset
			mockSearch.mockResolvedValueOnce(createSearchResult(0, 5, false, null));
			await state.handleSearch("second query");

			expect(state.feeds).toHaveLength(5);
			expect(state.cursor).toBeNull();
			expect(state.hasNextPage).toBe(false);
			expect(state.lastSearchedQuery).toBe("second query");
		});
	});

	describe("modal navigation with infinite scroll", () => {
		beforeEach(async () => {
			mockSearch.mockResolvedValueOnce(createSearchResult(0, 3, true, 3));
			await state.handleSearch("test");
			mockSearch.mockClear();
		});

		it("handleNext loads more when at last feed and hasNextPage", async () => {
			// Navigate to last feed
			state.handleSelectFeed(state.feeds[2], 2);
			expect(state.currentIndex).toBe(2);

			// Set up next batch
			mockSearch.mockResolvedValue(createSearchResult(3, 3, false, null));

			await state.handleNext();

			// Should have loaded more and navigated
			expect(mockSearch).toHaveBeenCalledWith("test", 3, 20);
			expect(state.feeds).toHaveLength(6);
			expect(state.currentIndex).toBe(3);
			expect(state.selectedFeed?.link).toBe("https://example.com/feed-3");
		});

		it("handleNext does not load more when not at last feed", async () => {
			state.handleSelectFeed(state.feeds[0], 0);

			await state.handleNext();

			expect(mockSearch).not.toHaveBeenCalled();
			expect(state.currentIndex).toBe(1);
		});

		it("hasNext is true at last feed when hasNextPage is true", () => {
			state.handleSelectFeed(state.feeds[2], 2);

			expect(state.hasNext).toBe(true);
		});

		it("hasNext is false at last feed when hasNextPage is false", () => {
			state.hasNextPage = false;
			state.handleSelectFeed(state.feeds[2], 2);

			expect(state.hasNext).toBe(false);
		});
	});

	describe("pagination safety guards", () => {
		it("stops pagination when feeds exceed MAX_SEARCH_RESULTS", async () => {
			// Simulate already having MAX_SEARCH_RESULTS feeds loaded
			mockSearch.mockResolvedValueOnce(createSearchResult(0, 20, true, 20));
			await state.handleSearch("test");
			mockSearch.mockClear();

			// Artificially inflate feeds to MAX_SEARCH_RESULTS
			const extraFeeds = Array.from({ length: 180 }, (_, i) =>
				createRenderFeed(`extra-${i}`, `https://example.com/extra-${i}`),
			);
			state.feeds = [...state.feeds, ...extraFeeds];
			state.hasNextPage = true;

			// loadMore should bail out without calling the API
			await state.loadMore();

			expect(mockSearch).not.toHaveBeenCalled();
			expect(state.hasNextPage).toBe(false);
		});

		it("stops pagination when API returns empty results with has_more true", async () => {
			mockSearch.mockResolvedValueOnce(createSearchResult(0, 20, true, 20));
			await state.handleSearch("test");
			mockSearch.mockClear();

			// API returns empty results but claims has_more=true (buggy backend)
			mockSearch.mockResolvedValue({
				results: [],
				error: null,
				next_cursor: 40,
				has_more: true,
			});

			await state.loadMore();

			expect(state.hasNextPage).toBe(false);
			expect(state.feeds).toHaveLength(20); // No new feeds appended
		});
	});
});
