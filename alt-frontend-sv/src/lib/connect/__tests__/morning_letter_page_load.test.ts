import { describe, it, expect, vi, beforeEach } from "vitest";

const mockGetLatestLetter = vi.fn();
const mockGetLetterByDate = vi.fn();

vi.mock("$lib/connect", () => ({
	createClientTransport: vi.fn(() => ({})),
	getLatestLetter: (...args: unknown[]) => mockGetLatestLetter(...args),
	getLetterByDate: (...args: unknown[]) => mockGetLetterByDate(...args),
}));

import { load } from "../../../routes/(app)/recap/morning-letter/+page";

const fakeLetter = {
	id: "letter-001",
	targetDate: "2026-04-07",
	editionTimezone: "Asia/Tokyo",
	isDegraded: false,
	body: { lead: "Today's news...", sections: [] },
};

function makeLoadArgs(searchParams?: Record<string, string>) {
	const params = new URLSearchParams(searchParams);
	return {
		url: new URL(`http://localhost/recap/morning-letter?${params.toString()}`),
	};
}

describe("+page.ts load", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("calls getLatestLetter when no date param", async () => {
		mockGetLatestLetter.mockResolvedValue(fakeLetter);

		const result = await load(makeLoadArgs() as never);

		expect(mockGetLatestLetter).toHaveBeenCalledTimes(1);
		expect(mockGetLetterByDate).not.toHaveBeenCalled();
		expect(result.letter).toEqual(fakeLetter);
		expect(result.requestedDate).toBeNull();
	});

	it("calls getLetterByDate when date param is provided", async () => {
		mockGetLetterByDate.mockResolvedValue(fakeLetter);

		const result = await load(makeLoadArgs({ date: "2026-04-07" }) as never);

		expect(mockGetLetterByDate).toHaveBeenCalledWith(
			expect.anything(),
			"2026-04-07",
		);
		expect(mockGetLatestLetter).not.toHaveBeenCalled();
		expect(result.letter).toEqual(fakeLetter);
		expect(result.requestedDate).toBe("2026-04-07");
	});

	it("returns null letter when not found", async () => {
		mockGetLatestLetter.mockResolvedValue(null);

		const result = await load(makeLoadArgs() as never);

		expect(result.letter).toBeNull();
		expect(result.error).toBeUndefined();
	});

	it("returns error flag on network failure", async () => {
		mockGetLatestLetter.mockRejectedValue(new Error("network error"));

		const result = await load(makeLoadArgs() as never);

		expect(result.letter).toBeNull();
		expect(result.error).toBe(true);
	});

	it("returns error flag when getLetterByDate fails", async () => {
		mockGetLetterByDate.mockRejectedValue(new Error("server error"));

		const result = await load(
			makeLoadArgs({ date: "2026-04-07" }) as never,
		);

		expect(result.letter).toBeNull();
		expect(result.requestedDate).toBe("2026-04-07");
		expect(result.error).toBe(true);
	});
});
