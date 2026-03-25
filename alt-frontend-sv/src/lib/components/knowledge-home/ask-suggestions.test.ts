import { describe, it, expect } from "vitest";
import { pickSuggestions, suggestionPool } from "./ask-suggestions";

describe("pickSuggestions", () => {
	it("returns exactly 3 suggestions", () => {
		const result = pickSuggestions(undefined);
		expect(result).toHaveLength(3);
	});

	it("returns strings from the pool", () => {
		const allQuestions = Object.values(suggestionPool).flat();
		const result = pickSuggestions(undefined);
		for (const s of result) {
			expect(allQuestions).toContain(s);
		}
	});

	it("returns different categories (no two from same category)", () => {
		// Run multiple times to check diversity
		for (let i = 0; i < 20; i++) {
			const result = pickSuggestions(undefined);
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

	it("prioritizes relevant categories for security tags", () => {
		const counts: Record<string, number> = {};
		for (let i = 0; i < 100; i++) {
			const result = pickSuggestions(["security"]);
			for (const q of result) {
				for (const [cat, questions] of Object.entries(suggestionPool)) {
					if (questions.includes(q)) {
						counts[cat] = (counts[cat] || 0) + 1;
					}
				}
			}
		}
		// critical should appear more often than average for security tags
		expect(counts["critical"]).toBeGreaterThan(20);
	});

	it("prioritizes relevant categories for AI tags", () => {
		const counts: Record<string, number> = {};
		for (let i = 0; i < 100; i++) {
			const result = pickSuggestions(["ai"]);
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
		const result = pickSuggestions([]);
		expect(result).toHaveLength(3);
	});

	it("handles unknown tags gracefully", () => {
		const result = pickSuggestions(["unknowntag123"]);
		expect(result).toHaveLength(3);
	});
});
