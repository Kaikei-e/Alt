import { describe, expect, it } from "vitest";
import type { ThemeSourceItem } from "#lib/connect/reading.js";
import {
	formatArticleCount,
	formatCalendarDate,
	formatCompactDate,
	formatSourceCount,
	formatThemeListMeta,
	formatThemePageMeta,
	formatThemeStatusLabel,
	formatThemeVersionRow,
	parseVersionParam,
	resolveSourceMarkers,
	sourceTitleOf,
	toThemeReaderItems,
} from "./themes-format.js";

describe("themes-format", () => {
	it("formats compact dates and returns empty string for null/invalid", () => {
		const formatted = formatCompactDate(new Date("2026-11-18T10:00:00.000Z"));
		expect(formatted).toMatch(/Nov\s+1[789],\s+2026/);
		expect(formatCompactDate(null)).toBe("");
		expect(formatCompactDate(undefined)).toBe("");
		expect(formatCompactDate(new Date("invalid"))).toBe("");
	});

	it("formats YYYY-MM-DD calendar dates without timezone shift and drops invalid dates", () => {
		expect(formatCalendarDate("2026-11-15")).toBe("Nov 15, 2026");
		// Dates at the start of a year or month would shift backward if parsed as UTC midnight in a negative-offset timezone.
		const origToLocaleDateString = Date.prototype.toLocaleDateString;
		const seenTimeZones: Array<string | undefined> = [];
		Date.prototype.toLocaleDateString = function (
			this: Date,
			locales?: Intl.LocalesArgument,
			options?: Intl.DateTimeFormatOptions,
		) {
			seenTimeZones.push(options?.timeZone);
			if (options?.timeZone) {
				return origToLocaleDateString.call(this, locales, options);
			}
			// Simulate a negative-offset timezone (UTC-8) when timeZone is omitted.
			return origToLocaleDateString.call(this, locales, {
				...options,
				timeZone: "America/Los_Angeles",
			});
		};
		try {
			expect(formatCalendarDate("2026-01-01")).toBe("Jan 1, 2026");
			expect(seenTimeZones).toContain("UTC");
		} finally {
			Date.prototype.toLocaleDateString = origToLocaleDateString;
		}

		expect(formatCalendarDate("")).toBe("");
		expect(formatCalendarDate(null)).toBe("");
		expect(formatCalendarDate(undefined)).toBe("");
		expect(formatCalendarDate("2026-02-30")).toBe("");
		expect(formatCalendarDate("2026-13-01")).toBe("");
		expect(formatCalendarDate("not-a-date")).toBe("");
	});

	it("pluralizes article and source counts", () => {
		expect(formatArticleCount(0)).toBe("0 articles");
		expect(formatArticleCount(1)).toBe("1 article");
		expect(formatArticleCount(4)).toBe("4 articles");

		expect(formatSourceCount(0)).toBe("0 sources");
		expect(formatSourceCount(1)).toBe("1 source");
		expect(formatSourceCount(3)).toBe("3 sources");
	});

	it("builds list meta line and status label for new, updated, and up-to-date themes", () => {
		const date = new Date("2026-11-18T10:00:00.000Z");
		const compact = formatCompactDate(date);

		expect(formatThemeListMeta(3, date)).toBe(
			`3 articles · updated ${compact}`,
		);
		expect(formatThemeListMeta(1, null)).toBe("1 article");

		expect(formatThemeStatusLabel(0, 2)).toBe("new");
		expect(formatThemeStatusLabel(1, 2)).toBe("updated since you read it");
		expect(formatThemeStatusLabel(2, 2)).toBeNull();
		expect(formatThemeStatusLabel(3, 2)).toBeNull();
	});

	it("builds theme page meta line and version row text", () => {
		const date = new Date("2026-11-18T10:00:00.000Z");
		const compact = formatCompactDate(date);

		expect(formatThemePageMeta(2, 2, date, 3)).toBe(
			`Version 2 of 2 · ${compact} · 3 sources`,
		);
		expect(formatThemeVersionRow(1, date, 2)).toBe(
			`Version 1 · ${compact} · 2 articles`,
		);
	});

	it("falls back to URL host when source title is empty", () => {
		expect(
			sourceTitleOf("Compiler Notes", "https://toolchain.example/notes"),
		).toBe("Compiler Notes");
		expect(sourceTitleOf("   ", "https://notes.example/build-cache")).toBe(
			"notes.example",
		);
		expect(sourceTitleOf("", "not-a-valid-url")).toBe("not-a-valid-url");
		expect(sourceTitleOf("", "")).toBe("Untitled source");
	});

	it("maps sourceArticleIds to 1-based source markers and omits missing ids", () => {
		const sources: ThemeSourceItem[] = [
			{
				articleId: "art-1",
				title: "First",
				url: "https://one.example/a",
			},
			{
				articleId: "art-2",
				title: "Second",
				url: "https://two.example/b",
			},
		];

		expect(
			resolveSourceMarkers(["art-2", "missing-id", "art-1", "art-2"], sources),
		).toEqual([
			{ number: 2, sourceIndex: 1, source: sources[1] },
			{ number: 1, sourceIndex: 0, source: sources[0] },
		]);
		expect(resolveSourceMarkers(["missing-only"], sources)).toEqual([]);
	});

	it("converts sources to ReaderArticleItem array with host fallback", () => {
		const sources: ThemeSourceItem[] = [
			{
				articleId: "art-1",
				title: "",
				url: "https://notes.example/article",
			},
		];

		expect(toThemeReaderItems(sources)).toEqual([
			{
				articleId: "art-1",
				url: "https://notes.example/article",
				title: "notes.example",
				author: "notes.example",
			},
		]);
	});

	it("parses positive integer version query parameter", () => {
		expect(parseVersionParam(null)).toBeUndefined();
		expect(parseVersionParam("")).toBeUndefined();
		expect(parseVersionParam("0")).toBeUndefined();
		expect(parseVersionParam("-2")).toBeUndefined();
		expect(parseVersionParam("abc")).toBeUndefined();
		expect(parseVersionParam("1")).toBe(1);
		expect(parseVersionParam("12")).toBe(12);
	});
});
