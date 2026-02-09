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
	hasMore: boolean,
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
		has_more: hasMore,
	};
}

// State manager that mirrors the page component's infinite scroll logic
class SearchPageState {
	feeds: RenderFeed[] = [];
	cursor: number | null = null;
	hasMore = false;
	isLoading = false;
	isLoadingMore = false;
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
			this.hasMore = false;
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
				this.hasMore = false;
				return;
			}

			this.feeds = (result.results ?? []).map((item, i) =>
				createRenderFeed(`feed-${i}`, item.link),
			);
			this.cursor = result.next_cursor ?? null;
			this.hasMore = result.has_more ?? false;
		} catch (err) {
			this.error = err as Error;
			this.feeds = [];
			this.cursor = null;
			this.hasMore = false;
		} finally {
			this.isLoading = false;
		}
	}

	async loadMore(): Promise<void> {
		if (this.isLoadingMore || !this.hasMore) return;
		this.isLoadingMore = true;
		try {
			const result = await this.searchFn(
				this.lastSearchedQuery,
				this.cursor ?? undefined,
				20,
			);
			if (result.error) return;
			const newFeeds = (result.results ?? []).map((item, i) =>
				createRenderFeed(`feed-${this.feeds.length + i}`, item.link),
			);
			this.feeds = [...this.feeds, ...newFeeds];
			this.cursor = result.next_cursor ?? null;
			this.hasMore = result.has_more ?? false;
		} finally {
			this.isLoadingMore = false;
		}
	}

	get hasPrevious(): boolean {
		return this.currentIndex > 0;
	}

	get hasNext(): boolean {
		return (
			(this.currentIndex >= 0 && this.currentIndex < this.feeds.length - 1) ||
			(this.currentIndex === this.feeds.length - 1 && this.hasMore)
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
		} else if (this.hasMore && !this.isLoadingMore) {
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
			expect(state.hasMore).toBe(true);
			expect(state.lastSearchedQuery).toBe("test query");
		});

		it("sets hasMore=false when results exhausted", async () => {
			mockSearch.mockResolvedValue(createSearchResult(0, 5, false, null));

			await state.handleSearch("rare query");

			expect(state.feeds).toHaveLength(5);
			expect(state.cursor).toBeNull();
			expect(state.hasMore).toBe(false);
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
			expect(state.hasMore).toBe(false);
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
			expect(state.hasMore).toBe(false);
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
			expect(state.hasMore).toBe(true);
		});

		it("updates hasMore when results exhausted", async () => {
			mockSearch.mockResolvedValue(createSearchResult(20, 10, false, null));

			await state.loadMore();

			expect(state.feeds).toHaveLength(30);
			expect(state.hasMore).toBe(false);
			expect(state.cursor).toBeNull();
		});

		it("does not load when already loading", async () => {
			state.isLoadingMore = true;

			await state.loadMore();

			expect(mockSearch).not.toHaveBeenCalled();
		});

		it("does not load when hasMore is false", async () => {
			state.hasMore = false;

			await state.loadMore();

			expect(mockSearch).not.toHaveBeenCalled();
		});

		it("resets isLoadingMore after completion", async () => {
			mockSearch.mockResolvedValue(createSearchResult(20, 20, true, 40));

			await state.loadMore();

			expect(state.isLoadingMore).toBe(false);
		});

		it("resets isLoadingMore on API error", async () => {
			mockSearch.mockResolvedValue({
				results: [],
				error: "Server error",
				next_cursor: null,
				has_more: false,
			});

			await state.loadMore();

			// Should not append on error, but isLoadingMore should reset
			expect(state.isLoadingMore).toBe(false);
			expect(state.feeds).toHaveLength(20); // Original 20 unchanged
		});
	});

	describe("query change resets state", () => {
		it("resets cursor and hasMore on new search", async () => {
			// First search with pagination
			mockSearch.mockResolvedValueOnce(createSearchResult(0, 20, true, 20));
			await state.handleSearch("first query");
			expect(state.hasMore).toBe(true);
			expect(state.cursor).toBe(20);

			// New search should reset
			mockSearch.mockResolvedValueOnce(createSearchResult(0, 5, false, null));
			await state.handleSearch("second query");

			expect(state.feeds).toHaveLength(5);
			expect(state.cursor).toBeNull();
			expect(state.hasMore).toBe(false);
			expect(state.lastSearchedQuery).toBe("second query");
		});
	});

	describe("modal navigation with infinite scroll", () => {
		beforeEach(async () => {
			mockSearch.mockResolvedValueOnce(createSearchResult(0, 3, true, 3));
			await state.handleSearch("test");
			mockSearch.mockClear();
		});

		it("handleNext loads more when at last feed and hasMore", async () => {
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

		it("hasNext is true at last feed when hasMore is true", () => {
			state.handleSelectFeed(state.feeds[2], 2);

			expect(state.hasNext).toBe(true);
		});

		it("hasNext is false at last feed when hasMore is false", () => {
			state.hasMore = false;
			state.handleSelectFeed(state.feeds[2], 2);

			expect(state.hasNext).toBe(false);
		});
	});
});
