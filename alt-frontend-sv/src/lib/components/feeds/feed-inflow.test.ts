import { describe, expect, it } from "vitest";
import type { FeedInflowStatItem } from "#lib/connect/feeds.js";
import type { FeedLink } from "#lib/schema/feedLink.js";
import {
	formatDailyCap,
	formatInflowCount,
	formatReadRate,
	parseDailyCapInput,
	parseFeedUrlParts,
	resolveFeedDisplayName,
	sortFeedInflowStats,
} from "./feed-inflow";

function makeStat(
	overrides: Partial<FeedInflowStatItem> & { feedLinkId: string },
): FeedInflowStatItem {
	return {
		url: `https://example.com/${overrides.feedLinkId}.xml`,
		inflowCount: 0,
		readCount: 0,
		openedCountLegacy: 0,
		readRate: 0,
		...overrides,
	};
}

describe("feed-inflow helpers", () => {
	describe("resolveFeedDisplayName", () => {
		it("joins against feedLinks by feedLinkId and falls back to stat url", () => {
			const feedLinks = new Map<string, FeedLink>([
				[
					"id-1",
					{
						id: "id-1",
						url: "https://joined.example.com/rss",
						healthStatus: "healthy",
						consecutiveFailures: 0,
						lastFailureReason: "",
						isActive: true,
					},
				],
			]);

			expect(
				resolveFeedDisplayName(
					{ feedLinkId: "id-1", url: "https://stat.example.com/rss" },
					feedLinks,
				),
			).toBe("https://joined.example.com/rss");

			expect(
				resolveFeedDisplayName(
					{ feedLinkId: "id-2", url: "https://stat.example.com/rss" },
					feedLinks,
				),
			).toBe("https://stat.example.com/rss");
		});
	});

	describe("parseFeedUrlParts", () => {
		it("extracts hostname without leading www. and path + query", () => {
			expect(
				parseFeedUrlParts("https://www.example.com/blog/feed.xml?format=rss"),
			).toEqual({
				host: "example.com",
				pathAndQuery: "/blog/feed.xml?format=rss",
			});

			expect(parseFeedUrlParts("https://news.ycombinator.com/rss")).toEqual({
				host: "news.ycombinator.com",
				pathAndQuery: "/rss",
			});
		});

		it("falls back to raw string when URL cannot be parsed", () => {
			expect(parseFeedUrlParts("not a valid url")).toEqual({
				host: "not a valid url",
				pathAndQuery: "",
			});
		});
	});

	describe("formatters", () => {
		it("formats read rate as whole-number percentage and en-dash when inflow is 0", () => {
			expect(formatReadRate(0, 0)).toBe("–");
			expect(formatReadRate(100, 0)).toBe("0%");
			expect(formatReadRate(100, 0.051)).toBe("5%");
			expect(formatReadRate(100, 0.139)).toBe("14%");
			expect(formatReadRate(10, 1)).toBe("100%");
		});

		it("formats inflow counts with thousands separators", () => {
			expect(formatInflowCount(0)).toBe("0");
			expect(formatInflowCount(1234)).toBe("1,234");
		});

		it("formats daily cap as 'No cap' or 'N per day'", () => {
			expect(formatDailyCap(undefined)).toBe("No cap");
			expect(formatDailyCap(10)).toBe("10 per day");
		});
	});

	describe("parseDailyCapInput", () => {
		it("accepts integers from 1 to 500", () => {
			expect(parseDailyCapInput("1")).toEqual({ ok: true, value: 1 });
			expect(parseDailyCapInput(" 42 ")).toEqual({ ok: true, value: 42 });
			expect(parseDailyCapInput("500")).toEqual({ ok: true, value: 500 });
		});

		it("rejects empty, zero, negative, fractional, non-numeric, and >500 values", () => {
			for (const bad of ["", "   ", "0", "-5", "1.5", "501", "abc", "10px"]) {
				const res = parseDailyCapInput(bad);
				expect(res.ok).toBe(false);
			}
		});
	});

	describe("sortFeedInflowStats", () => {
		const stats: FeedInflowStatItem[] = [
			makeStat({
				feedLinkId: "zero",
				url: "https://alpha.example/feed.xml",
				inflowCount: 0,
				readCount: 0,
				readRate: 0,
			}),
			makeStat({
				feedLinkId: "high-low-rate",
				url: "https://gamma.example/feed.xml",
				inflowCount: 1000,
				readCount: 20,
				readRate: 0.02,
			}),
			makeStat({
				feedLinkId: "mid-high-rate",
				url: "https://beta.example/feed.xml",
				inflowCount: 200,
				readCount: 100,
				readRate: 0.5,
			}),
		];

		it("sorts by most inflow descending by default without mutating input", () => {
			const originalOrder = stats.map((s) => s.feedLinkId);
			const sorted = sortFeedInflowStats(stats, "most-inflow", (s) => s.url);
			expect(sorted.map((s) => s.feedLinkId)).toEqual([
				"high-low-rate",
				"mid-high-rate",
				"zero",
			]);
			expect(stats.map((s) => s.feedLinkId)).toEqual(originalOrder);
		});

		it("sorts by lowest read rate ascending and places zero-inflow feeds last", () => {
			const sorted = sortFeedInflowStats(
				stats,
				"lowest-read-rate",
				(s) => s.url,
			);
			expect(sorted.map((s) => s.feedLinkId)).toEqual([
				"high-low-rate",
				"mid-high-rate",
				"zero",
			]);
		});

		it("sorts by name ascending", () => {
			const sorted = sortFeedInflowStats(stats, "name", (s) => s.url);
			expect(sorted.map((s) => s.feedLinkId)).toEqual([
				"zero",
				"mid-high-rate",
				"high-low-rate",
			]);
		});
	});
});
