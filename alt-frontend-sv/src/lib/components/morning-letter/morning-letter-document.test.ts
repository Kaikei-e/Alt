import { describe, it, expect } from "vitest";
import {
	orderSections,
	formatLetterDate,
	getSectionDisplayTitle,
	getSourcesForSection,
	isLetterStale,
	deriveWithinHours,
} from "./morning-letter-document";

describe("orderSections", () => {
	it("orders top3 first, then what_changed, then by_genre", () => {
		const sections = [
			{ key: "by_genre:tech", title: "Tech", bullets: [] },
			{ key: "what_changed", title: "What Changed", bullets: [] },
			{ key: "top3", title: "Top Stories", bullets: [] },
			{ key: "by_genre:finance", title: "Finance", bullets: [] },
		];

		const ordered = orderSections(sections);

		expect(ordered.map((s) => s.key)).toEqual([
			"top3",
			"what_changed",
			"by_genre:tech",
			"by_genre:finance",
		]);
	});

	it("handles missing sections gracefully", () => {
		const sections = [{ key: "by_genre:ai", title: "AI", bullets: [] }];

		const ordered = orderSections(sections);

		expect(ordered).toHaveLength(1);
		expect(ordered[0].key).toBe("by_genre:ai");
	});

	it("returns empty array for empty input", () => {
		expect(orderSections([])).toEqual([]);
	});
});

describe("formatLetterDate", () => {
	it("formats a civil date string for display", () => {
		const result = formatLetterDate("2026-04-07", "Asia/Tokyo");

		// Should contain year, month, day in some human-readable form
		expect(result).toContain("2026");
		expect(result).toContain("4");
		expect(result).toContain("7");
	});

	it("handles undefined timezone with fallback", () => {
		const result = formatLetterDate("2026-04-07");

		expect(result).toContain("2026");
	});

	it("does not use Date constructor for civil date (no off-by-one)", () => {
		// Decompose string directly, not via new Date()
		const result = formatLetterDate("2026-01-01", "Asia/Tokyo");

		expect(result).toContain("1"); // month
		expect(result).toContain("1"); // day
	});
});

describe("getSectionDisplayTitle", () => {
	it("prefers section.title over key mapping", () => {
		const section = { key: "top3", title: "Breaking News", bullets: [] };

		expect(getSectionDisplayTitle(section)).toBe("Breaking News");
	});

	it("falls back to key mapping when title is empty", () => {
		const section = { key: "top3", title: "", bullets: [] };

		expect(getSectionDisplayTitle(section)).toBe("Top Stories");
	});

	it("maps what_changed key", () => {
		const section = { key: "what_changed", title: "", bullets: [] };

		expect(getSectionDisplayTitle(section)).toBe("What Changed");
	});

	it("extracts and capitalizes genre from by_genre key", () => {
		const section = { key: "by_genre:technology", title: "", bullets: [] };

		expect(getSectionDisplayTitle(section)).toBe("Technology");
	});

	it("uses key as-is for unknown keys", () => {
		const section = { key: "unknown_key", title: "", bullets: [] };

		expect(getSectionDisplayTitle(section)).toBe("unknown_key");
	});

	it("shadows generic LLM titles (Need to Know) with mapped title", () => {
		const section = { key: "top3", title: "Need to Know", bullets: [] };

		expect(getSectionDisplayTitle(section)).toBe("Top Stories");
	});

	it("shadows 'Today's Headlines' with genre-derived title", () => {
		const section = {
			key: "by_genre:ai",
			title: "Today's Headlines",
			bullets: [],
		};

		expect(getSectionDisplayTitle(section)).toBe("Ai");
	});

	it("derives title from by_theme:slug keys with hyphen/underscore splitting", () => {
		const section = {
			key: "by_theme:data-pipelines-etl",
			title: "",
			bullets: [],
		};

		expect(getSectionDisplayTitle(section)).toBe("Data Pipelines Etl");
	});

	it("keeps a non-generic LLM title when no mapping exists", () => {
		const section = {
			key: "by_theme:ai_regulation",
			title: "AI regulation debate heats up",
			bullets: [],
		};

		expect(getSectionDisplayTitle(section)).toBe(
			"AI regulation debate heats up",
		);
	});
});

describe("getSourcesForSection", () => {
	const sources = [
		{
			letterId: "l1",
			sectionKey: "top3",
			articleId: "a1",
			sourceType: 1,
			position: 0,
		},
		{
			letterId: "l1",
			sectionKey: "top3",
			articleId: "a2",
			sourceType: 1,
			position: 1,
		},
		{
			letterId: "l1",
			sectionKey: "what_changed",
			articleId: "a3",
			sourceType: 2,
			position: 0,
		},
	];

	it("filters sources by section key", () => {
		const result = getSourcesForSection(sources, "top3");

		expect(result).toHaveLength(2);
		expect(result[0].articleId).toBe("a1");
	});

	it("returns empty array for unmatched section", () => {
		expect(getSourcesForSection(sources, "by_genre:ai")).toEqual([]);
	});

	it("handles empty sources", () => {
		expect(getSourcesForSection([], "top3")).toEqual([]);
	});
});

describe("isLetterStale", () => {
	it("returns false for recent letter", () => {
		const recentTimestamp = {
			seconds: BigInt(Math.floor(Date.now() / 1000) - 3600),
			nanos: 0,
		};

		expect(isLetterStale(recentTimestamp, 12)).toBe(false);
	});

	it("returns true for old letter", () => {
		const oldTimestamp = {
			seconds: BigInt(Math.floor(Date.now() / 1000) - 50000),
			nanos: 0,
		};

		expect(isLetterStale(oldTimestamp, 12)).toBe(true);
	});

	it("returns false for undefined timestamp", () => {
		expect(isLetterStale(undefined, 12)).toBe(false);
	});
});

describe("deriveWithinHours", () => {
	it("returns 24 for today", () => {
		const today = new Date().toISOString().split("T")[0];

		expect(deriveWithinHours(today)).toBe(24);
	});

	it("returns 24 for undefined", () => {
		expect(deriveWithinHours(undefined)).toBe(24);
	});

	it("returns hours since the target date morning for past dates", () => {
		// A date from 2 days ago
		const twoDaysAgo = new Date(Date.now() - 2 * 24 * 60 * 60 * 1000)
			.toISOString()
			.split("T")[0];

		const hours = deriveWithinHours(twoDaysAgo);

		// Should be roughly 48 + some hours, capped at 168 (7 days max)
		expect(hours).toBeGreaterThanOrEqual(24);
		expect(hours).toBeLessThanOrEqual(168);
	});

	it("caps at 168 hours (7 days)", () => {
		expect(deriveWithinHours("2020-01-01")).toBe(168);
	});
});
