import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { destroyCallbacks, streamFeedStatsMock, createClientTransportMock } =
	vi.hoisted(() => ({
		destroyCallbacks: [] as Array<() => void>,
		streamFeedStatsMock: vi.fn(),
		createClientTransportMock: vi.fn(() => ({})),
	}));

vi.mock("svelte", async (importOriginal) => {
	const actual = await importOriginal<typeof import("svelte")>();
	return {
		...actual,
		onDestroy: (fn: () => void) => {
			destroyCallbacks.push(fn);
		},
		untrack: <T>(fn: () => T) => fn(),
	};
});

vi.mock("$app/env", () => ({
	browser: true,
}));

vi.mock("#lib/connect/feeds.js", () => ({
	streamFeedStats: streamFeedStatsMock,
}));

vi.mock("#lib/connect/transport-client.js", () => ({
	createClientTransport: createClientTransportMock,
}));

import { useStreamingFeedStats } from "./useStreamingFeedStats.svelte";

describe("useStreamingFeedStats", () => {
	beforeEach(() => {
		destroyCallbacks.length = 0;
		vi.clearAllMocks();
	});

	afterEach(() => {
		while (destroyCallbacks.length > 0) {
			const cb = destroyCallbacks.pop();
			cb?.();
		}
	});

	it("immediately aborts the resolved AbortController if subscriber unmounts while streamFeedStats is pending", async () => {
		let resolveStream!: (controller: AbortController) => void;
		const pendingPromise = new Promise<AbortController>((resolve) => {
			resolveStream = resolve;
		});
		streamFeedStatsMock.mockReturnValueOnce(pendingPromise);

		const state = useStreamingFeedStats();
		expect(streamFeedStatsMock).toHaveBeenCalledTimes(1);

		// Unmount before streamFeedStats resolves
		const cleanup = destroyCallbacks.pop();
		cleanup?.();

		const controller = new AbortController();
		const abortSpy = vi.spyOn(controller, "abort");
		resolveStream(controller);
		await Promise.resolve();
		await Promise.resolve();

		expect(abortSpy).toHaveBeenCalledTimes(1);
		expect(state.isConnected).toBe(false);
	});

	it("updates reactive stats on non-heartbeat data and ignores heartbeats for counts", async () => {
		let capturedOnData!: (stats: {
			feedAmount: number;
			unsummarizedFeedAmount: number;
			totalArticles: number;
			isHeartbeat: boolean;
			timestamp: number;
		}) => void;

		const controller = new AbortController();
		streamFeedStatsMock.mockImplementationOnce(
			async (_transport: unknown, onData: typeof capturedOnData) => {
				capturedOnData = onData;
				return controller;
			},
		);

		const state = useStreamingFeedStats();
		await vi.waitFor(() => {
			expect(state.isConnected).toBe(true);
		});

		capturedOnData({
			feedAmount: 15,
			unsummarizedFeedAmount: 4,
			totalArticles: 120,
			isHeartbeat: false,
			timestamp: 1000,
		});

		expect(state.feedAmount).toBe(15);
		expect(state.unsummarizedArticlesAmount).toBe(4);
		expect(state.totalArticlesAmount).toBe(120);

		// Heartbeat should not overwrite counts
		capturedOnData({
			feedAmount: 0,
			unsummarizedFeedAmount: 0,
			totalArticles: 0,
			isHeartbeat: true,
			timestamp: 1005,
		});

		expect(state.feedAmount).toBe(15);
		expect(state.unsummarizedArticlesAmount).toBe(4);
		expect(state.totalArticlesAmount).toBe(120);
	});

	it("shares a single stream across multiple subscribers and disconnects only when last unmounts", async () => {
		const controller = new AbortController();
		const abortSpy = vi.spyOn(controller, "abort");
		streamFeedStatsMock.mockResolvedValue(controller);

		useStreamingFeedStats();
		useStreamingFeedStats();

		await vi.waitFor(() => {
			expect(streamFeedStatsMock).toHaveBeenCalledTimes(1);
		});

		// Unmount first subscriber — stream must stay alive
		const firstCleanup = destroyCallbacks.pop();
		firstCleanup?.();
		expect(abortSpy).not.toHaveBeenCalled();

		// Unmount second subscriber — stream must abort
		const secondCleanup = destroyCallbacks.pop();
		secondCleanup?.();
		expect(abortSpy).toHaveBeenCalledTimes(1);
	});
});
