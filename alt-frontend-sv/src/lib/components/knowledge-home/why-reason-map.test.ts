import { describe, expect, it } from "vitest";
import { resolveWhyReason } from "./why-reason-map";

describe("why-reason-map (3-tier Alt-Paper accents)", () => {
	describe("Urgent tier → accent-emphasis", () => {
		it("pulse_need_to_know maps to accent-emphasis", () => {
			const result = resolveWhyReason("pulse_need_to_know");
			expect(result.label).toBe("Need to Know");
			expect(result.iconName).toBe("Activity");
			expect(result.colorClass).toContain("accent-emphasis");
		});
	});

	describe("Fresh tier → accent-info", () => {
		it("new_unread maps to accent-info", () => {
			const result = resolveWhyReason("new_unread");
			expect(result.label).toBe("New");
			expect(result.iconName).toBe("Sparkles");
			expect(result.colorClass).toContain("accent-info");
		});

		it("summary_completed maps to accent-info", () => {
			const result = resolveWhyReason("summary_completed");
			expect(result.label).toBe("Summarized");
			expect(result.iconName).toBe("FileText");
			expect(result.colorClass).toContain("accent-info");
		});
	});

	describe("Contextual tier → accent-muted", () => {
		it("in_weekly_recap maps to accent-muted", () => {
			const result = resolveWhyReason("in_weekly_recap");
			expect(result.label).toBe("In Recap");
			expect(result.colorClass).toContain("accent-muted");
		});

		it("tag_hotspot maps to accent-muted", () => {
			const result = resolveWhyReason("tag_hotspot");
			expect(result.label).toBe("Trending");
			expect(result.colorClass).toContain("accent-muted");
		});

		it("tag_hotspot with tag produces 'Trending: <tag>' label", () => {
			const result = resolveWhyReason("tag_hotspot", "rust");
			expect(result.label).toBe("Trending: rust");
			expect(result.colorClass).toContain("accent-muted");
		});

		it("recent_interest_match maps to accent-muted", () => {
			const result = resolveWhyReason("recent_interest_match");
			expect(result.label).toBe("Interest");
			expect(result.colorClass).toContain("accent-muted");
		});

		it("related_to_recent_search maps to accent-muted", () => {
			const result = resolveWhyReason("related_to_recent_search");
			expect(result.label).toBe("Search related");
			expect(result.colorClass).toContain("accent-muted");
		});
	});

	describe("legacy badge tokens are removed", () => {
		const legacyTokens = [
			"badge-blue",
			"badge-purple",
			"badge-green",
			"badge-orange",
			"badge-amber",
			"badge-teal",
			"badge-yellow",
			"badge-sky",
			"badge-indigo",
		];

		const allKnownCodes = [
			"new_unread",
			"in_weekly_recap",
			"tag_hotspot",
			"summary_completed",
			"pulse_need_to_know",
			"recent_interest_match",
			"related_to_recent_search",
		];

		for (const code of allKnownCodes) {
			it(`${code} does not reference any legacy badge-* token`, () => {
				const result = resolveWhyReason(code);
				for (const token of legacyTokens) {
					expect(result.colorClass).not.toContain(token);
				}
			});
		}
	});

	describe("fallback", () => {
		it("unknown code falls back to 'Info' with accent-muted", () => {
			const result = resolveWhyReason("unknown_code");
			expect(result.label).toBe("Info");
			expect(result.iconName).toBe("Info");
			expect(result.colorClass).toContain("accent-muted");
		});
	});
});
