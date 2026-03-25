import { describe, expect, it } from "vitest";
import type { TagSuggestion } from "./TagCombobox.svelte";

/** Pure-logic tests for TagCombobox filtering and tag management.
 *  Component rendering tests are in TagCombobox.svelte.test.ts (requires VITEST_BROWSER=true). */

const suggestions: TagSuggestion[] = [
	{ name: "AI", count: 42 },
	{ name: "Rust", count: 15 },
	{ name: "Agents", count: 8 },
	{ name: "Go", count: 30 },
	{ name: "TypeScript", count: 20 },
];

function filterSuggestions(
	available: TagSuggestion[],
	selected: string[],
	query: string,
	maxItems = 20,
): TagSuggestion[] {
	const q = query.trim().toLowerCase();
	const selectedSet = new Set(selected);
	return available
		.filter((tag) => {
			if (selectedSet.has(tag.name)) return false;
			return q ? tag.name.toLowerCase().includes(q) : true;
		})
		.slice(0, maxItems);
}

function shouldShowCreateOption(
	query: string,
	selected: string[],
	available: TagSuggestion[],
): boolean {
	const q = query.trim();
	if (!q) return false;
	const lowerQ = q.toLowerCase();
	return (
		!selected.some((t) => t.toLowerCase() === lowerQ) &&
		!available.some((t) => t.name.toLowerCase() === lowerQ)
	);
}

describe("TagCombobox filter logic", () => {
	it("returns all suggestions when query is empty", () => {
		const result = filterSuggestions(suggestions, [], "");
		expect(result).toHaveLength(5);
	});

	it("filters suggestions by partial match (case-insensitive)", () => {
		const result = filterSuggestions(suggestions, [], "ru");
		expect(result).toHaveLength(1);
		expect(result[0]?.name).toBe("Rust");
	});

	it("excludes already selected tags", () => {
		const result = filterSuggestions(suggestions, ["AI"], "");
		expect(result).toHaveLength(4);
		expect(result.find((t) => t.name === "AI")).toBeUndefined();
	});

	it("limits results to maxItems", () => {
		const manyTags: TagSuggestion[] = Array.from({ length: 30 }, (_, i) => ({
			name: `Tag${i}`,
			count: 30 - i,
		}));
		const result = filterSuggestions(manyTags, [], "", 20);
		expect(result).toHaveLength(20);
	});

	it("combines query filter and selected exclusion", () => {
		const result = filterSuggestions(suggestions, ["AI"], "a");
		expect(result).toHaveLength(1);
		expect(result[0]?.name).toBe("Agents");
	});
});

describe("TagCombobox create option logic", () => {
	it("shows create when query doesn't match any existing tag", () => {
		expect(shouldShowCreateOption("NewTag", [], suggestions)).toBe(true);
	});

	it("hides create when query exactly matches an available tag", () => {
		expect(shouldShowCreateOption("AI", [], suggestions)).toBe(false);
	});

	it("hides create when query matches case-insensitively", () => {
		expect(shouldShowCreateOption("ai", [], suggestions)).toBe(false);
	});

	it("hides create when query is already selected", () => {
		expect(shouldShowCreateOption("NewTag", ["NewTag"], suggestions)).toBe(
			false,
		);
	});

	it("hides create when query is empty", () => {
		expect(shouldShowCreateOption("", [], suggestions)).toBe(false);
	});

	it("hides create when query is whitespace only", () => {
		expect(shouldShowCreateOption("   ", [], suggestions)).toBe(false);
	});
});
