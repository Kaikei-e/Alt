import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("$app/navigation", () => ({
	goto: vi.fn(),
}));

const mockGetLatestLetter = vi.fn();
const mockGetLetterByDate = vi.fn();
const mockGetLetterSources = vi.fn();

vi.mock("$lib/connect", () => ({
	createClientTransport: vi.fn(() => ({})),
	getLatestLetter: (...args: unknown[]) => mockGetLatestLetter(...args),
	getLetterByDate: (...args: unknown[]) => mockGetLetterByDate(...args),
	getLetterSources: (...args: unknown[]) => mockGetLetterSources(...args),
}));

import { goto } from "$app/navigation";
import { useMorningLetter } from "./useMorningLetter.svelte";

const fakeLetter = {
	id: "letter-001",
	targetDate: "2026-04-07",
	editionTimezone: "Asia/Tokyo",
	isDegraded: false,
	body: {
		lead: "Today's key developments...",
		sections: [{ key: "top3", title: "Top Stories", bullets: ["A", "B"] }],
	},
};

const fakeSources = [
	{ letterId: "letter-001", sectionKey: "top3", articleId: "art-1", sourceType: 1, position: 0 },
];

describe("useMorningLetter", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("uses initialLetter immediately without loading flash", () => {
		const ml = useMorningLetter(fakeLetter as never);

		expect(ml.letter).toEqual(fakeLetter);
		expect(ml.letterLoading).toBe(false);
	});

	it("starts with letterLoading=true when no initial data (undefined)", () => {
		const ml = useMorningLetter();

		expect(ml.letter).toBeNull();
		expect(ml.letterLoading).toBe(true);
	});

	it("starts with letterLoading=false when initial is null (NotFound)", () => {
		const ml = useMorningLetter(null);

		expect(ml.letter).toBeNull();
		expect(ml.letterLoading).toBe(false);
	});

	it("fetches latest letter when no initial and fetchLetter is called", async () => {
		mockGetLatestLetter.mockResolvedValue(fakeLetter);
		mockGetLetterSources.mockResolvedValue(fakeSources);

		const ml = useMorningLetter();
		await ml.fetchLetter();

		expect(ml.letter).toEqual(fakeLetter);
		expect(ml.letterLoading).toBe(false);
		expect(mockGetLatestLetter).toHaveBeenCalledTimes(1);
	});

	it("sets error on fetch failure", async () => {
		mockGetLatestLetter.mockRejectedValue(new Error("network error"));

		const ml = useMorningLetter();
		await ml.fetchLetter();

		expect(ml.letter).toBeNull();
		expect(ml.error).toBeInstanceOf(Error);
		expect(ml.letterLoading).toBe(false);
	});

	it("fetches letter by date", async () => {
		mockGetLetterByDate.mockResolvedValue(fakeLetter);
		mockGetLetterSources.mockResolvedValue([]);

		const ml = useMorningLetter();
		await ml.fetchByDate("2026-04-07");

		expect(ml.letter).toEqual(fakeLetter);
		expect(mockGetLetterByDate).toHaveBeenCalledWith(expect.anything(), "2026-04-07");
		expect(mockGetLatestLetter).not.toHaveBeenCalled();
	});

	it("handles null letter (not found)", async () => {
		mockGetLatestLetter.mockResolvedValue(null);

		const ml = useMorningLetter();
		await ml.fetchLetter();

		expect(ml.letter).toBeNull();
		expect(ml.letterLoading).toBe(false);
		expect(ml.error).toBeNull();
	});

	it("loads sources lazily after letter", async () => {
		mockGetLatestLetter.mockResolvedValue(fakeLetter);
		mockGetLetterSources.mockResolvedValue(fakeSources);

		const ml = useMorningLetter();
		await ml.fetchLetter();

		// Sources should be loaded after letter
		expect(ml.sources).toEqual(fakeSources);
		expect(mockGetLetterSources).toHaveBeenCalledWith(expect.anything(), "letter-001");
	});

	it("separates letterLoading and sourcesLoading", async () => {
		// Make sources slower than letter
		let resolveSourcesFn: (v: unknown) => void;
		const sourcesPromise = new Promise((resolve) => {
			resolveSourcesFn = resolve;
		});
		mockGetLatestLetter.mockResolvedValue(fakeLetter);
		mockGetLetterSources.mockReturnValue(sourcesPromise);

		const ml = useMorningLetter();
		await ml.fetchLetter();

		// Letter loaded, sources still loading
		expect(ml.letter).toEqual(fakeLetter);
		expect(ml.letterLoading).toBe(false);
		expect(ml.sourcesLoading).toBe(true);

		// Resolve sources
		resolveSourcesFn!(fakeSources);
		await sourcesPromise;
		// Wait a tick for state to update
		await new Promise((r) => setTimeout(r, 0));

		expect(ml.sources).toEqual(fakeSources);
		expect(ml.sourcesLoading).toBe(false);
	});

	it("retrySources keeps letter and only retries sources", async () => {
		mockGetLatestLetter.mockResolvedValue(fakeLetter);
		mockGetLetterSources
			.mockRejectedValueOnce(new Error("source error"))
			.mockResolvedValueOnce(fakeSources);

		const ml = useMorningLetter();
		await ml.fetchLetter();

		// First attempt failed sources
		expect(ml.sources).toEqual([]);

		// Retry sources only
		await ml.retrySources();

		expect(ml.letter).toEqual(fakeLetter); // letter unchanged
		expect(ml.sources).toEqual(fakeSources);
		expect(mockGetLatestLetter).toHaveBeenCalledTimes(1); // not re-fetched
		expect(mockGetLetterSources).toHaveBeenCalledTimes(2);
	});

	it("redirects to /login on Unauthenticated error", async () => {
		const unauthError = Object.assign(new Error("unauthenticated"), {
			code: 16,
			name: "ConnectError",
		});
		mockGetLatestLetter.mockRejectedValue(unauthError);

		const ml = useMorningLetter();
		await ml.fetchLetter();

		expect(goto).toHaveBeenCalledWith("/login");
	});
});
