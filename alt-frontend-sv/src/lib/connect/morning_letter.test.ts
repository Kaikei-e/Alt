import { describe, it, expect, vi, beforeEach } from "vitest";

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

vi.mock("$lib/gen/alt/morning_letter/v2/morning_letter_pb", () => ({
	MorningLetterService: {},
	MorningLetterReadService: {},
}));

import { createClient, ConnectError, Code } from "@connectrpc/connect";
import type { Transport } from "@connectrpc/connect";
import {
	getLatestLetter,
	getLetterByDate,
	getLetterSources,
} from "./morning_letter";

describe("morning_letter read service client", () => {
	let mockTransport: Transport;
	let mockReadClient: {
		getLatestLetter: ReturnType<typeof vi.fn>;
		getLetterByDate: ReturnType<typeof vi.fn>;
		getLetterSources: ReturnType<typeof vi.fn>;
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
		};
		(createClient as unknown as ReturnType<typeof vi.fn>).mockReturnValue(
			mockReadClient as never,
		);
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

			const result = await getLetterSources(
				mockTransport,
				"nonexistent",
			);

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
