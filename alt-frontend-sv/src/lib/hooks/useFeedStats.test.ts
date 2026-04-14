/**
 * useFeedStats Hook Tests
 *
 * After H-001, useFeedStats is a thin pass-through to useStreamingFeedStats
 * (Connect-RPC server streaming). The legacy SSE path has been removed.
 */
import { describe, expect, it, vi, beforeEach } from "vitest";
import { useFeedStats } from "./useFeedStats.svelte";

vi.mock("./useStreamingFeedStats.svelte", () => ({
	useStreamingFeedStats: vi.fn(() => ({
		feedAmount: 20,
		unsummarizedArticlesAmount: 15,
		totalArticlesAmount: 200,
		isConnected: true,
		retryCount: 0,
		reconnect: vi.fn(),
	})),
}));

describe("useFeedStats", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	describe("interface", () => {
		it("returns the streaming feed stats shape", () => {
			const result = useFeedStats();
			expect(result).toHaveProperty("feedAmount");
			expect(result).toHaveProperty("unsummarizedArticlesAmount");
			expect(result).toHaveProperty("totalArticlesAmount");
			expect(result).toHaveProperty("isConnected");
			expect(result).toHaveProperty("retryCount");
			expect(result).toHaveProperty("reconnect");
		});
	});

	describe("delegates to streaming hook", () => {
		it("calls useStreamingFeedStats", async () => {
			const { useStreamingFeedStats } = await import(
				"./useStreamingFeedStats.svelte"
			);
			useFeedStats();
			expect(useStreamingFeedStats).toHaveBeenCalled();
		});

		it("forwards the values from the streaming hook", () => {
			const result = useFeedStats();
			expect(result.feedAmount).toBe(20);
			expect(result.unsummarizedArticlesAmount).toBe(15);
			expect(result.totalArticlesAmount).toBe(200);
		});
	});
});
