import { describe, expect, it } from "vitest";
import { resolveRecallReason } from "./recall-reason-map";

describe("recall-reason-map (3-tier Alt-Paper accents)", () => {
	describe("Urgent tier → accent-emphasis", () => {
		it("pulse_followup_needed maps to accent-emphasis", () => {
			const result = resolveRecallReason("pulse_followup_needed");
			expect(result.label).toBe("Pulse follow-up");
			expect(result.iconName).toBe("Activity");
			expect(result.colorClass).toContain("accent-emphasis");
		});
	});

	describe("Fresh tier → accent-info", () => {
		it("related_to_recent_search maps to accent-info", () => {
			const result = resolveRecallReason("related_to_recent_search");
			expect(result.label).toBe("Search related");
			expect(result.colorClass).toContain("accent-info");
		});

		it("recap_context_unfinished maps to accent-info", () => {
			const result = resolveRecallReason("recap_context_unfinished");
			expect(result.label).toBe("Recap unfinished");
			expect(result.colorClass).toContain("accent-info");
		});
	});

	describe("Contextual tier → accent-muted", () => {
		it("opened_before_but_not_revisited maps to accent-muted", () => {
			const result = resolveRecallReason("opened_before_but_not_revisited");
			expect(result.label).toBe("Not revisited");
			expect(result.colorClass).toContain("accent-muted");
		});

		it("related_to_recent_augur_question maps to accent-muted", () => {
			const result = resolveRecallReason("related_to_recent_augur_question");
			expect(result.label).toBe("Augur related");
			expect(result.colorClass).toContain("accent-muted");
		});

		it("tag_interest_overlap maps to accent-muted", () => {
			const result = resolveRecallReason("tag_interest_overlap");
			expect(result.label).toBe("Interest match");
			expect(result.colorClass).toContain("accent-muted");
		});

		it("tag_interaction maps to accent-muted", () => {
			const result = resolveRecallReason("tag_interaction");
			expect(result.label).toBe("Tag explored");
			expect(result.colorClass).toContain("accent-muted");
		});
	});

	describe("legacy badge tokens are removed", () => {
		const legacyTokens = [
			"badge-amber",
			"badge-blue",
			"badge-purple",
			"badge-teal",
			"badge-orange",
			"badge-green",
			"badge-yellow",
			"badge-sky",
			"badge-indigo",
		];

		const allKnownCodes = [
			"opened_before_but_not_revisited",
			"related_to_recent_search",
			"related_to_recent_augur_question",
			"recap_context_unfinished",
			"pulse_followup_needed",
			"tag_interest_overlap",
			"tag_interaction",
		];

		for (const code of allKnownCodes) {
			it(`${code} does not reference any legacy badge-* token`, () => {
				const result = resolveRecallReason(code);
				for (const token of legacyTokens) {
					expect(result.colorClass).not.toContain(token);
				}
			});
		}
	});

	describe("fallback behavior", () => {
		it("returns 'Recall' fallback with accent-muted for unknown code", () => {
			const result = resolveRecallReason("unknown_code");
			expect(result.label).toBe("Recall");
			expect(result.iconName).toBe("Bell");
			expect(result.colorClass).toContain("accent-muted");
		});

		it("uses description as label when code is unknown", () => {
			const result = resolveRecallReason(
				"unknown_code",
				"Opened 3 days ago, not revisited since",
			);
			expect(result.label).toBe("Opened 3 days ago, not revisited since");
			expect(result.colorClass).toContain("accent-muted");
		});

		it("ignores description when code is known", () => {
			const result = resolveRecallReason(
				"related_to_recent_search",
				"some override text",
			);
			expect(result.label).toBe("Search related");
		});
	});
});
