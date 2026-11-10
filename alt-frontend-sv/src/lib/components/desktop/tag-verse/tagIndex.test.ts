import { describe, expect, it } from "vitest";
import type { TagCloudItem } from "#lib/connect/index.js";
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
});
