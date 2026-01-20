/**
 * useFeedStats Hook Tests
 *
 * Tests for the unified feed stats hook that switches between SSE and Connect-RPC streaming.
 *
 * Note: These tests verify the hook's interface structure.
 * The switching logic is determined by environment variables which are
 * difficult to mock reliably in Vitest. The underlying implementations
 * (useSSEFeedsStats, useStreamingFeedStats) have their own tests.
 */
import { describe, expect, it, vi, beforeEach } from "vitest";

// Mock the underlying hooks to prevent actual connections
vi.mock("./useSSEFeedsStats.svelte", () => ({
	useSSEFeedsStats: vi.fn(() => ({
		feedAmount: 10,
		unsummarizedArticlesAmount: 5,
		totalArticlesAmount: 100,
		isConnected: true,
		retryCount: 0,
	})),
}));

vi.mock("./useStreamingFeedStats.svelte", () => ({
	useStreamingFeedStats: vi.fn(() => ({
		feedAmount: 20,
		unsummarizedArticlesAmount: 15,
		totalArticlesAmount: 200,
		isConnected: true,
		retryCount: 0,
	})),
}));

// Mock env to default false (SSE mode)
vi.mock("$env/dynamic/public", () => ({
	env: {
		PUBLIC_USE_CONNECT_STREAMING: "false",
	},
}));

describe("useFeedStats", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	describe("interface", () => {
		it("should return feedAmount property", async () => {
			const { useFeedStats } = await import("./useFeedStats.svelte");
			const result = useFeedStats();

			expect(result).toHaveProperty("feedAmount");
			expect(typeof result.feedAmount).toBe("number");
		});

		it("should return unsummarizedArticlesAmount property", async () => {
			const { useFeedStats } = await import("./useFeedStats.svelte");
			const result = useFeedStats();

			expect(result).toHaveProperty("unsummarizedArticlesAmount");
			expect(typeof result.unsummarizedArticlesAmount).toBe("number");
		});

		it("should return totalArticlesAmount property", async () => {
			const { useFeedStats } = await import("./useFeedStats.svelte");
			const result = useFeedStats();

			expect(result).toHaveProperty("totalArticlesAmount");
			expect(typeof result.totalArticlesAmount).toBe("number");
		});

		it("should return isConnected property", async () => {
			const { useFeedStats } = await import("./useFeedStats.svelte");
			const result = useFeedStats();

			expect(result).toHaveProperty("isConnected");
			expect(typeof result.isConnected).toBe("boolean");
		});

		it("should return retryCount property", async () => {
			const { useFeedStats } = await import("./useFeedStats.svelte");
			const result = useFeedStats();

			expect(result).toHaveProperty("retryCount");
			expect(typeof result.retryCount).toBe("number");
		});
	});

	describe("SSE mode (default)", () => {
		it("should call useSSEFeedsStats when PUBLIC_USE_CONNECT_STREAMING is false", async () => {
			const { useFeedStats } = await import("./useFeedStats.svelte");
			const { useSSEFeedsStats } = await import("./useSSEFeedsStats.svelte");

			useFeedStats();

			expect(useSSEFeedsStats).toHaveBeenCalled();
		});

		it("should return values from SSE implementation", async () => {
			const { useFeedStats } = await import("./useFeedStats.svelte");
			const result = useFeedStats();

			// Values come from the mocked useSSEFeedsStats
			expect(result.feedAmount).toBe(10);
			expect(result.unsummarizedArticlesAmount).toBe(5);
			expect(result.totalArticlesAmount).toBe(100);
		});
	});

	describe("interface consistency", () => {
		it("should return all required properties in the interface", async () => {
			const { useFeedStats } = await import("./useFeedStats.svelte");
			const result = useFeedStats();

			const expectedKeys = [
				"feedAmount",
				"unsummarizedArticlesAmount",
				"totalArticlesAmount",
				"isConnected",
				"retryCount",
			].sort();

			const actualKeys = Object.keys(result).sort();

			expect(actualKeys).toEqual(expectedKeys);
		});
	});
});
