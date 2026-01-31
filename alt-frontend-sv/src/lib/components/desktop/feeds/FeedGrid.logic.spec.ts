import { describe, expect, it } from "vitest";
import {
	renderFeedsFixture,
	createRenderFeed,
} from "../../../../../tests/fixtures/feeds";
import type { RenderFeed } from "$lib/schema/feed";

/**
 * These tests verify the pure logic of the FeedGrid API functions.
 * The actual Svelte component tests require browser mode, but we can verify
 * the core navigation and state logic here.
 */

// Helper functions that mirror the implementation logic
function calculateNextFeedUrl(
	feeds: RenderFeed[],
	removedUrl: string,
): { nextFeedUrl: string | null; totalCount: number } {
	// Find the index of the feed being removed
	const currentIndex = feeds.findIndex((f) => f.normalizedUrl === removedUrl);
	const wasLastItem = currentIndex === feeds.length - 1;

	if (currentIndex === -1) {
		return { nextFeedUrl: null, totalCount: feeds.length };
	}

	// After removal, the array will be shorter
	const remainingFeeds = feeds.filter((f) => f.normalizedUrl !== removedUrl);
	const totalCount = remainingFeeds.length;

	if (totalCount === 0) {
		return { nextFeedUrl: null, totalCount: 0 };
	}

	// If we removed the last item, return null to signal "close modal"
	// (Don't navigate to previous - this matches expected UX)
	if (wasLastItem) {
		return { nextFeedUrl: null, totalCount };
	}

	// Otherwise, return the item at the same index (which was the next item)
	return {
		nextFeedUrl: remainingFeeds[currentIndex].normalizedUrl,
		totalCount,
	};
}

function getFeedByUrl(feeds: RenderFeed[], url: string): RenderFeed | null {
	return feeds.find((f) => f.normalizedUrl === url) ?? null;
}

describe("FeedGrid Logic", () => {
	describe("calculateNextFeedUrl", () => {
		it("returns next feed URL when middle item is removed", () => {
			const result = calculateNextFeedUrl(
				renderFeedsFixture,
				"https://example.com/feed-2", // index 1
			);

			// After removing feed-2, feed-3 is now at index 1
			expect(result.nextFeedUrl).toBe("https://example.com/feed-3");
			expect(result.totalCount).toBe(4);
		});

		it("returns next feed URL when first item is removed", () => {
			const result = calculateNextFeedUrl(
				renderFeedsFixture,
				"https://example.com/feed-1", // index 0
			);

			// After removing feed-1, feed-2 is now at index 0
			expect(result.nextFeedUrl).toBe("https://example.com/feed-2");
			expect(result.totalCount).toBe(4);
		});

		it("returns null when last item is removed (to close modal)", () => {
			const result = calculateNextFeedUrl(
				renderFeedsFixture,
				"https://example.com/feed-5", // index 4, last item
			);

			// After removing feed-5 (last item), return null to signal "close modal"
			expect(result.nextFeedUrl).toBeNull();
			expect(result.totalCount).toBe(4);
		});

		it("returns null when the only item is removed", () => {
			const singleFeed = [
				createRenderFeed("feed-1", "https://example.com/feed-1"),
			];

			const result = calculateNextFeedUrl(
				singleFeed,
				"https://example.com/feed-1",
			);

			expect(result.nextFeedUrl).toBeNull();
			expect(result.totalCount).toBe(0);
		});

		it("returns unchanged when URL not found", () => {
			const result = calculateNextFeedUrl(
				renderFeedsFixture,
				"https://example.com/non-existent",
			);

			expect(result.nextFeedUrl).toBeNull();
			expect(result.totalCount).toBe(5);
		});

		it("handles consecutive removals correctly", () => {
			let feeds = [...renderFeedsFixture];

			// Remove feed-2
			let result = calculateNextFeedUrl(feeds, "https://example.com/feed-2");
			expect(result.nextFeedUrl).toBe("https://example.com/feed-3");
			feeds = feeds.filter(
				(f) => f.normalizedUrl !== "https://example.com/feed-2",
			);

			// Remove feed-3 (now at index 1)
			result = calculateNextFeedUrl(feeds, "https://example.com/feed-3");
			expect(result.nextFeedUrl).toBe("https://example.com/feed-4");
			feeds = feeds.filter(
				(f) => f.normalizedUrl !== "https://example.com/feed-3",
			);

			// Remove feed-4 (now at index 1)
			result = calculateNextFeedUrl(feeds, "https://example.com/feed-4");
			expect(result.nextFeedUrl).toBe("https://example.com/feed-5");
			feeds = feeds.filter(
				(f) => f.normalizedUrl !== "https://example.com/feed-4",
			);

			// Remove feed-5 (now last, should return null to close modal)
			result = calculateNextFeedUrl(feeds, "https://example.com/feed-5");
			expect(result.nextFeedUrl).toBeNull(); // Close modal when removing last item
			expect(result.totalCount).toBe(1);
			feeds = feeds.filter(
				(f) => f.normalizedUrl !== "https://example.com/feed-5",
			);

			// Remove last remaining feed
			result = calculateNextFeedUrl(feeds, "https://example.com/feed-1");
			expect(result.nextFeedUrl).toBeNull();
			expect(result.totalCount).toBe(0);
		});
	});

	describe("getFeedByUrl", () => {
		it("returns the correct feed when found", () => {
			const feed = getFeedByUrl(
				renderFeedsFixture,
				"https://example.com/feed-3",
			);

			expect(feed).not.toBeNull();
			expect(feed?.id).toBe("feed-3");
		});

		it("returns null when feed is not found", () => {
			const feed = getFeedByUrl(
				renderFeedsFixture,
				"https://example.com/non-existent",
			);

			expect(feed).toBeNull();
		});

		it("returns null for removed feed", () => {
			const feeds = renderFeedsFixture.filter(
				(f) => f.normalizedUrl !== "https://example.com/feed-2",
			);

			const feed = getFeedByUrl(feeds, "https://example.com/feed-2");

			expect(feed).toBeNull();
		});
	});

	describe("URL-based state tracking (race condition prevention)", () => {
		it("maintains correct feed reference even when array mutates", () => {
			// Simulate: User viewing feed-3, then feed-2 gets removed
			const selectedFeedUrl = "https://example.com/feed-3";
			let feeds = [...renderFeedsFixture];

			// Remove feed-2 (not the selected one)
			feeds = feeds.filter(
				(f) => f.normalizedUrl !== "https://example.com/feed-2",
			);

			// URL-based lookup should still find feed-3
			const selectedFeed = getFeedByUrl(feeds, selectedFeedUrl);
			expect(selectedFeed).not.toBeNull();
			expect(selectedFeed?.id).toBe("feed-3");
		});

		it("URL-based tracking survives array reordering", () => {
			// Simulate array reordering due to async operations
			const selectedFeedUrl = "https://example.com/feed-3";
			const feeds = [...renderFeedsFixture].reverse();

			// Should still find the correct feed
			const selectedFeed = getFeedByUrl(feeds, selectedFeedUrl);
			expect(selectedFeed).not.toBeNull();
			expect(selectedFeed?.id).toBe("feed-3");
		});

		it("index-based tracking would fail on mutation (demonstrating the problem)", () => {
			// This test demonstrates why index-based tracking is problematic
			const selectedIndex = 2; // feed-3
			let feeds = [...renderFeedsFixture];

			// Remove feed-2 (index 1)
			feeds = feeds.filter(
				(f) => f.normalizedUrl !== "https://example.com/feed-2",
			);

			// Index-based lookup now returns wrong feed!
			const feedAtSameIndex = feeds[selectedIndex];
			// After removing index 1, index 2 is now feed-4, not feed-3
			expect(feedAtSameIndex?.id).toBe("feed-4"); // Wrong feed!
			expect(feedAtSameIndex?.id).not.toBe("feed-3");
		});
	});
});
