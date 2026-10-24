import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@connectrpc/connect", () => {
	class ConnectError extends Error {
		code: number;
		constructor(message: string, code: number) {
			super(message);
			this.name = "ConnectError";
			this.code = code;
		}
	}
	return {
		ConnectError,
		Code: { AlreadyExists: 6, Internal: 13 },
	};
});

const { createFeedClientMock } = vi.hoisted(() => ({
	createFeedClientMock: vi.fn(),
}));

vi.mock("./client", () => ({
	createFeedClient: createFeedClientMock,
}));

import type { Transport } from "@connectrpc/connect";
import { Code, ConnectError } from "@connectrpc/connect";
import {
	streamFeedStats,
	streamSummarize,
	streamSummarizeWithAbort,
} from "./streaming";

async function* makeAsyncStream<T>(items: T[]): AsyncGenerator<T> {
	for (const item of items) {
		yield item;
	}
}

describe("connect/feeds/streaming", () => {
	let mockTransport: Transport;
	let mockClient: {
		streamFeedStats: ReturnType<typeof vi.fn>;
		streamSummarize: ReturnType<typeof vi.fn>;
	};

	beforeEach(() => {
		vi.clearAllMocks();
		mockTransport = {} as Transport;
		mockClient = {
			streamFeedStats: vi.fn(),
			streamSummarize: vi.fn(),
		};
		createFeedClientMock.mockReturnValue(mockClient);
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	describe("streamFeedStats", () => {
		it("streams stats and converts bigint/number fields to numbers", async () => {
			mockClient.streamFeedStats.mockReturnValue(
				makeAsyncStream([
					{
						feedAmount: BigInt(10),
						unsummarizedFeedAmount: BigInt(3),
						totalArticles: BigInt(100),
						metadata: { isHeartbeat: false, timestamp: BigInt(1700000000) },
					},
					{
						feedAmount: BigInt(10),
						unsummarizedFeedAmount: BigInt(2),
						totalArticles: BigInt(101),
						metadata: { isHeartbeat: true, timestamp: BigInt(1700000005) },
					},
				]),
			);

			const onData = vi.fn();
			const onError = vi.fn();

			await streamFeedStats(mockTransport, onData, onError);

			await vi.waitFor(() => {
				expect(onData).toHaveBeenCalledTimes(2);
			});

			expect(onData).toHaveBeenNthCalledWith(1, {
				feedAmount: 10,
				unsummarizedFeedAmount: 3,
				totalArticles: 100,
				isHeartbeat: false,
				timestamp: 1700000000,
			});
			expect(onData).toHaveBeenNthCalledWith(2, {
				feedAmount: 10,
				unsummarizedFeedAmount: 2,
				totalArticles: 101,
				isHeartbeat: true,
				timestamp: 1700000005,
			});
			expect(onError).not.toHaveBeenCalled();
		});

		it("suppresses error callback when stream is aborted", async () => {
			let rejectItem!: (err: Error) => void;
			mockClient.streamFeedStats.mockReturnValue({
				[Symbol.asyncIterator]() {
					return {
						next() {
							return new Promise((_, reject) => {
								rejectItem = reject;
							});
						},
					};
				},
			});

			const onData = vi.fn();
			const onError = vi.fn();

			const controller = await streamFeedStats(mockTransport, onData, onError);
			controller.abort();
			rejectItem(new Error("aborted stream"));

			await Promise.resolve();
			await Promise.resolve();

			expect(onError).not.toHaveBeenCalled();
		});
	});

	describe("streamSummarize", () => {
		it("throws when neither feedUrl nor articleId is provided", async () => {
			await expect(streamSummarize(mockTransport, {})).rejects.toThrow(
				"Either feedUrl or articleId is required",
			);
		});

		it("accumulates chunks and resolves with final summary", async () => {
			mockClient.streamSummarize.mockReturnValue(
				makeAsyncStream([
					{
						articleId: "art-1",
						chunk: "First part. ",
						isFinal: false,
						isCached: false,
					},
					{
						articleId: "art-1",
						chunk: "Second part.",
						isFinal: true,
						isCached: false,
						fullSummary: "First part. Second part.",
					},
				]),
			);

			const chunks: unknown[] = [];
			const result = await streamSummarize(
				mockTransport,
				{ articleId: "art-1" },
				(c) => chunks.push(c),
			);

			expect(chunks).toHaveLength(2);
			expect(result).toEqual({
				articleId: "art-1",
				summary: "First part. Second part.",
				wasCached: false,
			});
		});

		it("retries on 409 Conflict (Code.AlreadyExists) and succeeds", async () => {
			vi.useFakeTimers();

			let calls = 0;
			mockClient.streamSummarize.mockImplementation(() => {
				calls += 1;
				if (calls === 1) {
					return {
						[Symbol.asyncIterator]() {
							return {
								next() {
									return Promise.reject(
										new ConnectError("already processing", Code.AlreadyExists),
									);
								},
							};
						},
					};
				}
				return makeAsyncStream([
					{
						articleId: "art-1",
						chunk: "",
						isFinal: true,
						isCached: true,
						fullSummary: "Cached summary after retry",
					},
				]);
			});

			const onConflictRetry = vi.fn();
			const promise = streamSummarize(mockTransport, {
				feedUrl: "https://example.com/1",
				onConflictRetry,
			});

			await vi.advanceTimersByTimeAsync(3000);
			const result = await promise;

			expect(onConflictRetry).toHaveBeenCalledWith(1, 3);
			expect(result).toEqual({
				articleId: "art-1",
				summary: "Cached summary after retry",
				wasCached: true,
			});
		});
	});

	describe("streamSummarizeWithAbort", () => {
		it("reports validation error to onError when options are missing", () => {
			const onError = vi.fn();
			streamSummarizeWithAbort(mockTransport, {}, vi.fn(), vi.fn(), onError);
			expect(onError).toHaveBeenCalledTimes(1);
			expect((onError.mock.calls[0]?.[0] as Error).message).toBe(
				"Either feedUrl or articleId is required",
			);
		});

		it("streams chunks and calls onComplete", async () => {
			mockClient.streamSummarize.mockReturnValue(
				makeAsyncStream([
					{
						articleId: "art-2",
						chunk: "Summary text",
						isFinal: true,
						isCached: false,
						fullSummary: "Summary text",
					},
				]),
			);

			const onChunk = vi.fn();
			const onComplete = vi.fn();

			streamSummarizeWithAbort(
				mockTransport,
				{ articleId: "art-2" },
				onChunk,
				onComplete,
			);

			await vi.waitFor(() => {
				expect(onComplete).toHaveBeenCalledWith({
					articleId: "art-2",
					summary: "Summary text",
					wasCached: false,
				});
			});
			expect(onChunk).toHaveBeenCalledTimes(1);
		});
	});
});
