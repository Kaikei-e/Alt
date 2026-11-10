import { describe, expect, it } from "vitest";
import type { TagCloudItem } from "#lib/connect/index.js";
import * as tagIndexModule from "./tagIndex.js";
import {
	filterTagsForIndex,
	sortTagsForIndex,
	sumTagArticleCounts,
} from "./tagIndex.js";

const SAMPLE_TAGS: TagCloudItem[] = [
	{
		tagName: "svelte",
		articleCount: 12,
		positionX: 1,
		positionY: 2,
		positionZ: 3,
	},
	{
		tagName: "rust",
		articleCount: 45,
		positionX: -1,
		positionY: 0,
		positionZ: 2,
	},
	{
		tagName: "ai",
		articleCount: 45,
		positionX: 0,
		positionY: 1,
		positionZ: -1,
	},
	{
		tagName: "GoLang",
		articleCount: 20,
		positionX: 2,
		positionY: -2,
		positionZ: 0,
	},
];

describe("tagIndex pure helpers", () => {
	it("sorts tags by articleCount descending with tagName ascending tiebreak without mutating input", () => {
		const copy = [...SAMPLE_TAGS];
		const sorted = sortTagsForIndex(SAMPLE_TAGS);

		expect(SAMPLE_TAGS).toEqual(copy);
		expect(sorted.map((t) => t.tagName)).toEqual([
			"ai",
			"rust",
			"GoLang",
			"svelte",
		]);
	});

	it("filters tags by case-insensitive substring and preserves sort order", () => {
		expect(filterTagsForIndex(SAMPLE_TAGS, "").map((t) => t.tagName)).toEqual([
			"ai",
			"rust",
			"GoLang",
			"svelte",
		]);

		expect(
			filterTagsForIndex(SAMPLE_TAGS, "  gol ").map((t) => t.tagName),
		).toEqual(["GoLang"]);

		expect(filterTagsForIndex(SAMPLE_TAGS, "S").map((t) => t.tagName)).toEqual([
			"rust",
			"svelte",
		]);

		expect(filterTagsForIndex(SAMPLE_TAGS, "nonexistent")).toEqual([]);
	});

	it("sums total articleCount across all tags", () => {
		expect(sumTagArticleCounts(SAMPLE_TAGS)).toBe(122);
		expect(sumTagArticleCounts([])).toBe(0);
	});

	it("pluralizes tag and article counts", () => {
		const formatCount = (
			tagIndexModule as {
				formatCountLabel?: (count: number, singular: string) => string;
			}
		).formatCountLabel;
		expect(formatCount?.(1, "tag")).toBe("1 tag");
		expect(formatCount?.(0, "tag")).toBe("0 tags");
		expect(formatCount?.(2, "tag")).toBe("2 tags");
		expect(formatCount?.(1, "article")).toBe("1 article");
		expect(formatCount?.(45, "article")).toBe("45 articles");
	});
});
