import { describe, expect, it, vi, beforeEach } from "vitest";
import { renderFeedsFixture } from "../../../../tests/fixtures/feeds";
import type { RenderFeed } from "$lib/schema/feed";

/**
 * These tests verify the state management logic for the desktop feeds page.
 * They test the race condition fixes without needing browser rendering.
 */

// Types that match the new API
type RemoveFeedResult = {
	nextFeedUrl: string | null;
	totalCount: number;
};

type FeedGridApi = {
	removeFeedByUrl: (url: string) => RemoveFeedResult;
	getVisibleFeeds: () => RenderFeed[];
	getFeedByUrl: (url: string) => RenderFeed | null;
	fetchReplacementFeed: () => void;
};

// Simulated state management class that mirrors the page component logic
class PageStateManager {
	private feeds: RenderFeed[];
	private removedUrls: Set<string> = new Set();
	public selectedFeedUrl: string | null = null;
	public isModalOpen = false;
	public isProcessingMarkAsRead = false;

	constructor(initialFeeds: RenderFeed[]) {
		this.feeds = [...initialFeeds];
	}

	get visibleFeeds(): RenderFeed[] {
		return this.feeds.filter((f) => !this.removedUrls.has(f.normalizedUrl));
	}

	get selectedFeed(): RenderFeed | null {
		if (!this.selectedFeedUrl) return null;
		return this.visibleFeeds.find((f) => f.normalizedUrl === this.selectedFeedUrl) ?? null;
	}

	// Simulates the new FeedGrid API
	createApi(): FeedGridApi {
		return {
			removeFeedByUrl: (url: string): RemoveFeedResult => {
				const currentIndex = this.visibleFeeds.findIndex((f) => f.normalizedUrl === url);
				const wasLastItem = currentIndex === this.visibleFeeds.length - 1;
				this.removedUrls.add(url);
				const newFeeds = this.visibleFeeds;
				const totalCount = newFeeds.length;

				if (totalCount === 0) {
					return { nextFeedUrl: null, totalCount: 0 };
				}

				// If removed last item, return null to close modal
				if (wasLastItem) {
					return { nextFeedUrl: null, totalCount };
				}

				// Return item at same index (next item)
				return {
					nextFeedUrl: newFeeds[currentIndex].normalizedUrl,
					totalCount,
				};
			},
			getVisibleFeeds: () => this.visibleFeeds,
			getFeedByUrl: (url: string) => this.visibleFeeds.find((f) => f.normalizedUrl === url) ?? null,
			fetchReplacementFeed: vi.fn(),
		};
	}

	// New handleMarkAsRead implementation
	async handleMarkAsRead(feedUrl: string, api: FeedGridApi): Promise<void> {
		if (this.isProcessingMarkAsRead) return;

		this.isProcessingMarkAsRead = true;

		try {
			// Synchronously get navigation info BEFORE any async operations
			const { nextFeedUrl } = api.removeFeedByUrl(feedUrl);

			// Simulate API call (async)
			await new Promise((resolve) => setTimeout(resolve, 10));

			// Navigate based on pre-calculated info
			// nextFeedUrl is null when: no feeds left OR was viewing last feed
			if (nextFeedUrl === null) {
				this.isModalOpen = false;
				this.selectedFeedUrl = null;
			} else {
				this.selectedFeedUrl = nextFeedUrl;
			}

			// Fire-and-forget replacement fetch
			api.fetchReplacementFeed();
		} finally {
			this.isProcessingMarkAsRead = false;
		}
	}

	// Old problematic implementation for comparison
	async handleMarkAsReadOld(feedUrl: string): Promise<void> {
		const hadNext = (() => {
			const index = this.visibleFeeds.findIndex((f) => f.normalizedUrl === feedUrl);
			return index < this.visibleFeeds.length - 1;
		})();

		// Remove feed (this modifies the array)
		this.removedUrls.add(feedUrl);

		// Simulate async tick
		await new Promise((resolve) => setTimeout(resolve, 10));

		// Problem: Using index on mutated array
		const feeds = this.visibleFeeds;
		if (feeds.length === 0 || !hadNext) {
			this.isModalOpen = false;
			this.selectedFeedUrl = null;
		} else {
			// BUG: currentIndex is not tracked properly!
			// In the real component, this would use the old index on the new array
		}
	}

	openModal(feedUrl: string) {
		this.selectedFeedUrl = feedUrl;
		this.isModalOpen = true;
	}
}

describe("Desktop Feeds Page State Management", () => {
	let state: PageStateManager;
	let api: FeedGridApi;

	beforeEach(() => {
		state = new PageStateManager(renderFeedsFixture);
		api = state.createApi();
	});

	describe("handleMarkAsRead", () => {
		it("navigates to correct next feed after marking as read", async () => {
			state.openModal("https://example.com/feed-2");
			expect(state.selectedFeed?.id).toBe("feed-2");

			await state.handleMarkAsRead("https://example.com/feed-2", api);

			// Should navigate to feed-3 (the next one)
			expect(state.selectedFeedUrl).toBe("https://example.com/feed-3");
			expect(state.selectedFeed?.id).toBe("feed-3");
		});

		it("closes modal when last feed is marked as read", async () => {
			state.openModal("https://example.com/feed-5");

			await state.handleMarkAsRead("https://example.com/feed-5", api);

			// Should close modal (not navigate to previous)
			expect(state.isModalOpen).toBe(false);
			expect(state.selectedFeedUrl).toBeNull();
		});

		it("closes modal when only remaining feed is marked as read", async () => {
			// Remove all but one feed
			state = new PageStateManager([renderFeedsFixture[0]]);
			api = state.createApi();
			state.openModal("https://example.com/feed-1");

			await state.handleMarkAsRead("https://example.com/feed-1", api);

			expect(state.isModalOpen).toBe(false);
			expect(state.selectedFeedUrl).toBeNull();
		});

		it("prevents duplicate clicks with processing flag", async () => {
			state.openModal("https://example.com/feed-2");

			// Start first call
			const firstCall = state.handleMarkAsRead("https://example.com/feed-2", api);

			// Second call should be ignored due to flag
			expect(state.isProcessingMarkAsRead).toBe(true);
			await state.handleMarkAsRead("https://example.com/feed-3", api); // Should be no-op

			await firstCall;

			// Only feed-2 should have been removed
			expect(state.visibleFeeds.length).toBe(4);
			expect(state.visibleFeeds.find((f) => f.id === "feed-2")).toBeUndefined();
			expect(state.visibleFeeds.find((f) => f.id === "feed-3")).toBeDefined();
		});

		it("resets processing flag after completion", async () => {
			state.openModal("https://example.com/feed-2");

			await state.handleMarkAsRead("https://example.com/feed-2", api);

			expect(state.isProcessingMarkAsRead).toBe(false);
		});
	});

	describe("URL-based tracking", () => {
		it("selectedFeed derives correctly from selectedFeedUrl", () => {
			state.selectedFeedUrl = "https://example.com/feed-3";

			expect(state.selectedFeed?.id).toBe("feed-3");
		});

		it("selectedFeed updates when feed is no longer visible", async () => {
			state.openModal("https://example.com/feed-2");
			expect(state.selectedFeed?.id).toBe("feed-2");

			// Remove the selected feed
			await state.handleMarkAsRead("https://example.com/feed-2", api);

			// selectedFeedUrl is updated to next feed
			expect(state.selectedFeedUrl).toBe("https://example.com/feed-3");
			expect(state.selectedFeed?.id).toBe("feed-3");
		});

		it("handles rapid consecutive mark-as-read operations", async () => {
			state.openModal("https://example.com/feed-1");

			// Process sequentially (processing flag prevents concurrent)
			await state.handleMarkAsRead("https://example.com/feed-1", api);
			expect(state.selectedFeedUrl).toBe("https://example.com/feed-2");

			// Update API to reflect new state
			api = state.createApi();
			await state.handleMarkAsRead("https://example.com/feed-2", api);
			expect(state.selectedFeedUrl).toBe("https://example.com/feed-3");

			api = state.createApi();
			await state.handleMarkAsRead("https://example.com/feed-3", api);
			expect(state.selectedFeedUrl).toBe("https://example.com/feed-4");

			// Verify we navigated through correctly
			expect(state.visibleFeeds.length).toBe(2);
		});
	});

	describe("fetchReplacementFeed", () => {
		it("is called after marking as read (fire-and-forget)", async () => {
			state.openModal("https://example.com/feed-2");

			await state.handleMarkAsRead("https://example.com/feed-2", api);

			expect(api.fetchReplacementFeed).toHaveBeenCalled();
		});
	});
});
