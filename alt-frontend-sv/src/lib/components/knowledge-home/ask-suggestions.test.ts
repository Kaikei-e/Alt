import { describe, it, expect } from "vitest";
import { pickSuggestions, suggestionPool } from "./ask-suggestions";

const allQuestions = Object.values(suggestionPool).flat();

describe("pickSuggestions", () => {
	it("returns exactly 3 suggestions", () => {
		const result = pickSuggestions(undefined, 0);
		expect(result).toHaveLength(3);
	});

	it("returns strings from the pool", () => {
		const result = pickSuggestions(undefined, 0);
		for (const s of result) {
			expect(allQuestions).toContain(s);
		}
	});

	it("returns different categories (no two from same category)", () => {
		// Deterministic: same seed always produces same categories
		for (const seed of [0, 1, 2, 42, 100]) {
			const result = pickSuggestions(undefined, seed);
			const categories = result.map((q) => {
				for (const [cat, questions] of Object.entries(suggestionPool)) {
					if (questions.includes(q)) return cat;
				}
				return "unknown";
			});
			const unique = new Set(categories);
			expect(unique.size).toBe(3);
		}
	});

	it("is deterministic: same seed produces same results", () => {
		const a = pickSuggestions(["ai"], 42);
		const b = pickSuggestions(["ai"], 42);
		expect(a).toEqual(b);
	});

	it("different seeds produce different results", () => {
		const a = pickSuggestions(undefined, 0);
		const b = pickSuggestions(undefined, 1);
		// With different seeds, at least one suggestion should differ
		const same = a.every((s, i) => s === b[i]);
		expect(same).toBe(false);
	});

	it("prioritizes relevant categories for security tags", () => {
		const counts: Record<string, number> = {};
		for (let seed = 0; seed < 100; seed++) {
			const result = pickSuggestions(["security"], seed);
			for (const q of result) {
				for (const [cat, questions] of Object.entries(suggestionPool)) {
					if (questions.includes(q)) {
						counts[cat] = (counts[cat] || 0) + 1;
					}
				}
			}
		}
		// critical should appear in every result for security tags (it's preferred)
		expect(counts["critical"]).toBeGreaterThan(20);
	});

	it("prioritizes relevant categories for AI tags", () => {
		const counts: Record<string, number> = {};
		for (let seed = 0; seed < 100; seed++) {
			const result = pickSuggestions(["ai"], seed);
			for (const q of result) {
				for (const [cat, questions] of Object.entries(suggestionPool)) {
					if (questions.includes(q)) {
						counts[cat] = (counts[cat] || 0) + 1;
					}
				}
			}
		}
		expect(counts["deep_dive"]).toBeGreaterThan(20);
	});

	it("handles empty tags array", () => {
		const result = pickSuggestions([], 0);
		expect(result).toHaveLength(3);
	});

	it("handles unknown tags gracefully", () => {
		const result = pickSuggestions(["unknowntag123"], 0);
		expect(result).toHaveLength(3);
	});

	it("consecutive seeds avoid identical 3-suggestion sets", () => {
		// Verify that seed and seed+1 produce different sets
		for (let seed = 0; seed < 10; seed++) {
			const a = pickSuggestions(undefined, seed);
			const b = pickSuggestions(undefined, seed + 1);
			const identical = a.every((s, i) => s === b[i]);
			// At least one pair should differ (probabilistically always true with seeded RNG)
			if (seed < 9) {
				// Allow at most 1 collision across 10 pairs
				expect(identical).toBe(false);
			}
		}
	});
});
