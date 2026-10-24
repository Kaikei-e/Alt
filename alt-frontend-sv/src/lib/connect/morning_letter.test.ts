import { beforeEach, describe, expect, it, vi } from "vitest";

// Mock the connect module — class must be inlined because vi.mock is hoisted
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
		createClient: vi.fn(),
		ConnectError,
		Code: { NotFound: 5, Unauthenticated: 16, Internal: 13 },
	};
});

vi.mock("#lib/gen/alt/morning_letter/v2/morning_letter_pb.js", () => ({
	MorningLetterService: { typeName: "MorningLetterService" },
	MorningLetterReadService: { typeName: "MorningLetterReadService" },
}));

import type { Transport } from "@connectrpc/connect";
import { Code, ConnectError, createClient } from "@connectrpc/connect";
import {
	getLatestLetter,
	getLetterByDate,
	getLetterEnrichment,
	getLetterSources,
	regenerateLatestLetter,
	streamMorningLetterChat,
} from "./morning_letter";

async function* makeAsyncStream<T>(items: T[]): AsyncGenerator<T> {
	for (const item of items) {
		yield item;
	}
}

describe("morning_letter read service client", () => {
	let mockTransport: Transport;
	let mockReadClient: {
		getLatestLetter: ReturnType<typeof vi.fn>;
		getLetterByDate: ReturnType<typeof vi.fn>;
		getLetterSources: ReturnType<typeof vi.fn>;
		getLetterEnrichment: ReturnType<typeof vi.fn>;
		regenerateLatest: ReturnType<typeof vi.fn>;
	};
	let mockChatClient: {
		streamChat: ReturnType<typeof vi.fn>;
	};

	const fakeLetter = {
		id: "letter-001",
		targetDate: "2026-04-07",
		editionTimezone: "Asia/Tokyo",
		isDegraded: false,
		schemaVersion: 1,
		generationRevision: 1,
		model: "gemma4-e4b",
		createdAt: { seconds: BigInt(1744000000), nanos: 0 },
		etag: "rev-1",
		body: {
			lead: "Today's key developments...",
			sections: [
				{
					key: "top3",
					title: "Top Stories",
					bullets: ["Story A", "Story B"],
					genre: undefined,
				},
			],
			generatedAt: { seconds: BigInt(1744000000), nanos: 0 },
			sourceRecapWindowDays: 3,
		},
	};

	const fakeSources = [
		{
			letterId: "letter-001",
			sectionKey: "top3",
			articleId: "article-abc",
			sourceType: 1, // RECAP
			position: 0,
		},
	];

	beforeEach(() => {
		mockTransport = {} as Transport;
		mockReadClient = {
			getLatestLetter: vi.fn(),
			getLetterByDate: vi.fn(),
			getLetterSources: vi.fn(),
			getLetterEnrichment: vi.fn(),
			regenerateLatest: vi.fn(),
		};
		mockChatClient = {
			streamChat: vi.fn(),
		};
		(createClient as unknown as ReturnType<typeof vi.fn>).mockImplementation(
			(service: { typeName?: string }) => {
				if (service?.typeName === "MorningLetterService") {
					return mockChatClient as never;
				}
				return mockReadClient as never;
			},
		);
	});

	// =========================================================================
	// streamMorningLetterChat
	// =========================================================================

	describe("streamMorningLetterChat", () => {
		it("calls onComplete exactly once when delta events are followed by a done event", async () => {
			mockChatClient.streamChat.mockReturnValue(
				makeAsyncStream([
					{ payload: { case: "delta", value: "Hello " } },
					{ payload: { case: "delta", value: "world" } },
					{
						payload: {
							case: "meta",
							value: {
								citations: [
									{
										url: "https://example.com/a",
										title: "Article A",
										publishedAt: "2026-04-07T00:00:00Z",
									},
								],
								timeWindow: {
									since: "2026-04-06T00:00:00Z",
									until: "2026-04-07T00:00:00Z",
								},
								articlesScanned: 12,
							},
						},
					},
					{
						payload: {
							case: "done",
							value: {
								answer: "Hello world",
								citations: [],
							},
						},
					},
				]),
			);

			const onDelta = vi.fn();
			const onMeta = vi.fn();
			const onComplete = vi.fn();

			streamMorningLetterChat(
				mockTransport,
				{ messages: [{ role: "user", content: "What happened today?" }] },
				onDelta,
				onMeta,
				onComplete,
			);

			await vi.waitFor(() => {
				expect(onComplete).toHaveBeenCalledTimes(1);
			});

			expect(onDelta).toHaveBeenCalledTimes(2);
			expect(onMeta).toHaveBeenCalledWith({
				citations: [
					{
						url: "https://example.com/a",
						title: "Article A",
						publishedAt: "2026-04-07T00:00:00Z",
					},
				],
				timeWindow: {
					since: "2026-04-06T00:00:00Z",
					until: "2026-04-07T00:00:00Z",
				},
				articlesScanned: 12,
			});
			expect(onComplete).toHaveBeenCalledWith({
				answer: "Hello world",
				citations: [
					{
						url: "https://example.com/a",
						title: "Article A",
						publishedAt: "2026-04-07T00:00:00Z",
					},
				],
			});
		});

		it("calls onComplete once when stream ends without a done event", async () => {
			mockChatClient.streamChat.mockReturnValue(
				makeAsyncStream([
					{ payload: { case: "delta", value: "Partial " } },
					{ payload: { case: "delta", value: "answer" } },
				]),
			);

			const onComplete = vi.fn();

			streamMorningLetterChat(
				mockTransport,
				{ messages: [{ role: "user", content: "News?" }] },
				undefined,
				undefined,
				onComplete,
			);

			await vi.waitFor(() => {
				expect(onComplete).toHaveBeenCalledTimes(1);
			});

			expect(onComplete).toHaveBeenCalledWith({
				answer: "Partial answer",
				citations: [],
			});
		});

		it("does NOT call onComplete after fallbackCode and defaults empty fallbackCode to UNKNOWN", async () => {
			mockChatClient.streamChat.mockReturnValue(
				makeAsyncStream([
					{ payload: { case: "delta", value: "Some text before fallback" } },
					{ payload: { case: "fallbackCode", value: "" } },
				]),
			);

			const onComplete = vi.fn();
			const onFallback = vi.fn();

			streamMorningLetterChat(
				mockTransport,
				{ messages: [{ role: "user", content: "News?" }] },
				undefined,
				undefined,
				onComplete,
				onFallback,
			);

			await vi.waitFor(() => {
				expect(onFallback).toHaveBeenCalledTimes(1);
			});

			expect(onFallback).toHaveBeenCalledWith("UNKNOWN");
			expect(onComplete).not.toHaveBeenCalled();
		});

		it("does NOT call onComplete after errorMessage and defaults empty errorMessage", async () => {
			mockChatClient.streamChat.mockReturnValue(
				makeAsyncStream([
					{ payload: { case: "delta", value: "Some text before error" } },
					{ payload: { case: "errorMessage", value: "" } },
				]),
			);

			const onComplete = vi.fn();
			const onError = vi.fn();

			streamMorningLetterChat(
				mockTransport,
				{ messages: [{ role: "user", content: "News?" }] },
				undefined,
				undefined,
				onComplete,
				undefined,
				onError,
			);

			await vi.waitFor(() => {
				expect(onError).toHaveBeenCalledTimes(1);
			});

			expect(onError.mock.calls[0]?.[0]).toBeInstanceOf(Error);
			expect((onError.mock.calls[0]?.[0] as Error).message).toBe(
				"Unknown stream error",
			);
			expect(onComplete).not.toHaveBeenCalled();
		});
	});

	// =========================================================================
	// getLatestLetter
	// =========================================================================

	describe("getLatestLetter", () => {
		it("returns the letter document on success", async () => {
			mockReadClient.getLatestLetter.mockResolvedValue({
				letter: fakeLetter,
			});

			const result = await getLatestLetter(mockTransport);

			expect(result).toEqual(fakeLetter);
			expect(mockReadClient.getLatestLetter).toHaveBeenCalledWith({});
		});

		it("returns null when letter is not found (NotFound)", async () => {
			mockReadClient.getLatestLetter.mockRejectedValue(
				new ConnectError("not found", Code.NotFound),
			);

			const result = await getLatestLetter(mockTransport);

			expect(result).toBeNull();
		});

		it("rethrows Unauthenticated errors", async () => {
			mockReadClient.getLatestLetter.mockRejectedValue(
				new ConnectError("unauthenticated", Code.Unauthenticated),
			);

			await expect(getLatestLetter(mockTransport)).rejects.toThrow(
				ConnectError,
			);
		});

		it("rethrows other errors", async () => {
			mockReadClient.getLatestLetter.mockRejectedValue(
				new ConnectError("internal", Code.Internal),
			);

			await expect(getLatestLetter(mockTransport)).rejects.toThrow(
				ConnectError,
			);
		});

		it("returns null when response has no letter", async () => {
			mockReadClient.getLatestLetter.mockResolvedValue({
				letter: undefined,
			});

			const result = await getLatestLetter(mockTransport);

			expect(result).toBeNull();
		});
	});

	// =========================================================================
	// getLetterByDate
	// =========================================================================

	describe("getLetterByDate", () => {
		it("returns the letter for a specific date", async () => {
			mockReadClient.getLetterByDate.mockResolvedValue({
				letter: fakeLetter,
			});

			const result = await getLetterByDate(mockTransport, "2026-04-07");

			expect(result).toEqual(fakeLetter);
			expect(mockReadClient.getLetterByDate).toHaveBeenCalledWith({
				targetDate: "2026-04-07",
			});
		});

		it("returns null when date has no letter (NotFound)", async () => {
			mockReadClient.getLetterByDate.mockRejectedValue(
				new ConnectError("not found", Code.NotFound),
			);

			const result = await getLetterByDate(mockTransport, "2026-01-01");

			expect(result).toBeNull();
		});

		it("rethrows Unauthenticated errors", async () => {
			mockReadClient.getLetterByDate.mockRejectedValue(
				new ConnectError("unauthenticated", Code.Unauthenticated),
			);

			await expect(
				getLetterByDate(mockTransport, "2026-04-07"),
			).rejects.toThrow(ConnectError);
		});
	});

	// =========================================================================
	// regenerateLatestLetter
	// =========================================================================

	describe("regenerateLatestLetter", () => {
		it("passes editionTimezone and returns regenerated letter result", async () => {
			mockReadClient.regenerateLatest.mockResolvedValue({
				letter: fakeLetter,
				regenerated: true,
				retryAfterSeconds: 0,
			});

			const result = await regenerateLatestLetter(mockTransport, "Asia/Tokyo");

			expect(mockReadClient.regenerateLatest).toHaveBeenCalledWith({
				editionTimezone: "Asia/Tokyo",
			});
			expect(result).toEqual({
				letter: fakeLetter,
				regenerated: true,
				retryAfterSeconds: 0,
			});
		});

		it("returns null letter when response letter is undefined", async () => {
			mockReadClient.regenerateLatest.mockResolvedValue({
				letter: undefined,
				regenerated: false,
				retryAfterSeconds: 1800,
			});

			const result = await regenerateLatestLetter(mockTransport);

			expect(mockReadClient.regenerateLatest).toHaveBeenCalledWith({
				editionTimezone: undefined,
			});
			expect(result).toEqual({
				letter: null,
				regenerated: false,
				retryAfterSeconds: 1800,
			});
		});
	});

	// =========================================================================
	// getLetterEnrichment
	// =========================================================================

	describe("getLetterEnrichment", () => {
		it("returns enrichments on success", async () => {
			const fakeEnrichments = [
				{
					sectionKey: "top3",
					articleId: "article-abc",
					articleTitle: "Story A",
				},
			];
			mockReadClient.getLetterEnrichment.mockResolvedValue({
				enrichments: fakeEnrichments,
			});

			const result = await getLetterEnrichment(mockTransport, "letter-001");

			expect(mockReadClient.getLetterEnrichment).toHaveBeenCalledWith({
				letterId: "letter-001",
			});
			expect(result).toEqual(fakeEnrichments);
		});

		it("returns empty array when letter is not found (NotFound)", async () => {
			mockReadClient.getLetterEnrichment.mockRejectedValue(
				new ConnectError("not found", Code.NotFound),
			);

			const result = await getLetterEnrichment(mockTransport, "missing");

			expect(result).toEqual([]);
		});

		it("rethrows non-NotFound errors", async () => {
			mockReadClient.getLetterEnrichment.mockRejectedValue(
				new ConnectError("unauthenticated", Code.Unauthenticated),
			);

			await expect(
				getLetterEnrichment(mockTransport, "letter-001"),
			).rejects.toThrow(ConnectError);
		});
	});

	// =========================================================================
	// getLetterSources
	// =========================================================================

	describe("getLetterSources", () => {
		it("returns sources for a letter", async () => {
			mockReadClient.getLetterSources.mockResolvedValue({
				sources: fakeSources,
			});

			const result = await getLetterSources(mockTransport, "letter-001");

			expect(result).toEqual(fakeSources);
			expect(mockReadClient.getLetterSources).toHaveBeenCalledWith({
				letterId: "letter-001",
			});
		});

		it("returns empty array when no sources", async () => {
			mockReadClient.getLetterSources.mockResolvedValue({
				sources: [],
			});

			const result = await getLetterSources(mockTransport, "letter-001");

			expect(result).toEqual([]);
		});

		it("returns null on NotFound (letter does not exist)", async () => {
			mockReadClient.getLetterSources.mockRejectedValue(
				new ConnectError("not found", Code.NotFound),
			);

			const result = await getLetterSources(mockTransport, "nonexistent");

			expect(result).toBeNull();
		});

		it("rethrows Unauthenticated errors", async () => {
			mockReadClient.getLetterSources.mockRejectedValue(
				new ConnectError("unauthenticated", Code.Unauthenticated),
			);

			await expect(
				getLetterSources(mockTransport, "letter-001"),
			).rejects.toThrow(ConnectError);
		});
	});
});
