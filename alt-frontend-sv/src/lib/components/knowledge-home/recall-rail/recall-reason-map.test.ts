import { describe, expect, it } from "vitest";
import { resolveRecallReason } from "./recall-reason-map";

describe("recall-reason-map", () => {
	describe("known reason types", () => {
		it("maps opened_before_but_not_revisited to 'Not revisited' with amber color", () => {
			const result = resolveRecallReason("opened_before_but_not_revisited");
			expect(result.label).toBe("Not revisited");
			expect(result.iconName).toBe("RotateCcw");
			expect(result.colorClass).toContain("amber");
		});

		it("maps related_to_recent_search to 'Search related' with blue color", () => {
			const result = resolveRecallReason("related_to_recent_search");
			expect(result.label).toBe("Search related");
			expect(result.iconName).toBe("Search");
			expect(result.colorClass).toContain("blue");
		});

		it("maps related_to_recent_augur_question to 'Augur related' with purple color", () => {
			const result = resolveRecallReason("related_to_recent_augur_question");
			expect(result.label).toBe("Augur related");
			expect(result.iconName).toBe("MessageSquare");
			expect(result.colorClass).toContain("purple");
		});

		it("maps recap_context_unfinished to 'Recap unfinished' with teal color", () => {
			const result = resolveRecallReason("recap_context_unfinished");
			expect(result.label).toBe("Recap unfinished");
			expect(result.iconName).toBe("BookOpen");
			expect(result.colorClass).toContain("teal");
		});

		it("maps pulse_followup_needed to 'Pulse follow-up' with orange color", () => {
			const result = resolveRecallReason("pulse_followup_needed");
			expect(result.label).toBe("Pulse follow-up");
			expect(result.iconName).toBe("Activity");
			expect(result.colorClass).toContain("orange");
		});

		it("maps tag_interest_overlap to 'Interest match' with green color", () => {
			const result = resolveRecallReason("tag_interest_overlap");
			expect(result.label).toBe("Interest match");
			expect(result.iconName).toBe("Tag");
			expect(result.colorClass).toContain("green");
		});

		it("maps tag_interaction to 'Tag explored' with teal color", () => {
			const result = resolveRecallReason("tag_interaction");
			expect(result.label).toBe("Tag explored");
			expect(result.iconName).toBe("Hash");
			expect(result.colorClass).toContain("teal");
		});
	});

	describe("fallback behavior", () => {
		it("returns 'Recall' fallback for unknown code without description", () => {
			const result = resolveRecallReason("unknown_code");
			expect(result.label).toBe("Recall");
			expect(result.iconName).toBe("Bell");
			expect(result.colorClass).toContain("gray");
		});

		it("returns 'Recall' fallback for empty string without description", () => {
			const result = resolveRecallReason("");
			expect(result.label).toBe("Recall");
		});

		it("uses description as label when code is unknown and description is provided", () => {
			const result = resolveRecallReason("unknown_code", "Opened 3 days ago, not revisited since");
			expect(result.label).toBe("Opened 3 days ago, not revisited since");
			expect(result.colorClass).toContain("gray");
		});

		it("ignores description when code is known", () => {
			const result = resolveRecallReason("related_to_recent_search", "some override text");
			expect(result.label).toBe("Search related");
		});
	});
});
