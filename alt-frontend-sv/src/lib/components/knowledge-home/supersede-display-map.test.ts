import { describe, expect, it } from "vitest";
import { resolveSupersede } from "./supersede-display-map";

describe("supersede-display-map (3-tier Alt-Paper accents)", () => {
	describe("update signals → accent-info (Fresh tier)", () => {
		const freshStates = [
			"summary_updated",
			"tags_updated",
			"multiple_updated",
			"both_updated",
		];

		for (const state of freshStates) {
			it(`${state} maps to accent-info`, () => {
				const result = resolveSupersede(state);
				expect(result.colorClass).toContain("accent-info");
			});
		}

		it("preserves display labels", () => {
			expect(resolveSupersede("summary_updated").label).toBe("Summary updated");
			expect(resolveSupersede("tags_updated").label).toBe("Tags updated");
			expect(resolveSupersede("multiple_updated").label).toBe("Updated");
		});
	});

	describe("reason changes → accent-muted (Contextual tier)", () => {
		it("reason_updated maps to accent-muted", () => {
			const result = resolveSupersede("reason_updated");
			expect(result.label).toBe("Reasons changed");
			expect(result.colorClass).toContain("accent-muted");
		});
	});

	describe("legacy badge tokens are removed", () => {
		const legacyTokens = [
			"badge-sky",
			"badge-indigo",
			"badge-amber",
			"badge-gray",
			"badge-blue",
			"badge-purple",
			"badge-teal",
		];

		const allKnownStates = [
			"summary_updated",
			"tags_updated",
			"reason_updated",
			"multiple_updated",
			"both_updated",
		];

		for (const state of allKnownStates) {
			it(`${state} does not reference any legacy badge-* token`, () => {
				const result = resolveSupersede(state);
				for (const token of legacyTokens) {
					expect(result.colorClass).not.toContain(token);
				}
			});
		}
	});

	describe("fallback", () => {
		it("unknown state falls back to accent-muted", () => {
			const result = resolveSupersede("unknown_state");
			expect(result.label).toBe("Updated");
			expect(result.colorClass).toContain("accent-muted");
		});
	});
});
