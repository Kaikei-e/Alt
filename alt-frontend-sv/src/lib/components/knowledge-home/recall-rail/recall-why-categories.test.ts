import { describe, expect, it } from "vitest";
import { categorizeRecallReasons } from "./recall-why-categories";
import type { RecallReasonData } from "$lib/connect/knowledge_home";

describe("categorizeRecallReasons", () => {
	it("categorizes opened_before_but_not_revisited under 'Revisit'", () => {
		const reasons: RecallReasonData[] = [
			{
				type: "opened_before_but_not_revisited",
				description: "Opened 3 days ago, not revisited since",
			},
		];
		const groups = categorizeRecallReasons(reasons);
		const revisit = groups.find((g) => g.key === "revisit");
		expect(revisit).toBeDefined();
		expect(revisit?.label).toBe("Revisit");
		expect(revisit?.items).toHaveLength(1);
		expect(revisit?.items[0].displayLabel).toBe("Not revisited");
		expect(revisit?.items[0].reason.description).toBe(
			"Opened 3 days ago, not revisited since",
		);
	});

	it("categorizes search/augur/tag_interest under 'Connection'", () => {
		const reasons: RecallReasonData[] = [
			{ type: "related_to_recent_search", description: "Search" },
			{
				type: "related_to_recent_augur_question",
				description: "Augur",
			},
			{ type: "tag_interest_overlap", description: "Tags" },
		];
		const groups = categorizeRecallReasons(reasons);
		const connection = groups.find((g) => g.key === "connection");
		expect(connection).toBeDefined();
		expect(connection?.label).toBe("Connection");
		expect(connection?.items).toHaveLength(3);
	});

	it("categorizes tag_interaction under 'Connection'", () => {
		const reasons: RecallReasonData[] = [
			{ type: "tag_interaction", description: 'You explored tag "rust"' },
		];
		const groups = categorizeRecallReasons(reasons);
		const connection = groups.find((g) => g.key === "connection");
		expect(connection).toBeDefined();
		expect(connection?.items).toHaveLength(1);
		expect(connection?.items[0].displayLabel).toBe("Tag explored");
	});

	it("categorizes recap/pulse under 'Completion'", () => {
		const reasons: RecallReasonData[] = [
			{ type: "recap_context_unfinished", description: "Recap" },
			{ type: "pulse_followup_needed", description: "Pulse" },
		];
		const groups = categorizeRecallReasons(reasons);
		const completion = groups.find((g) => g.key === "completion");
		expect(completion).toBeDefined();
		expect(completion?.label).toBe("Completion");
		expect(completion?.items).toHaveLength(2);
	});

	it("returns empty array for empty reasons", () => {
		const groups = categorizeRecallReasons([]);
		expect(groups).toEqual([]);
	});

	it("puts unknown types under 'Other' and uses description as displayLabel", () => {
		const reasons: RecallReasonData[] = [
			{ type: "unknown_type", description: "Some custom reason" },
		];
		const groups = categorizeRecallReasons(reasons);
		const other = groups.find((g) => g.key === "other");
		expect(other).toBeDefined();
		expect(other?.items[0].displayLabel).toBe("Some custom reason");
	});

	it("handles mixed categories and preserves order", () => {
		const reasons: RecallReasonData[] = [
			{ type: "opened_before_but_not_revisited", description: "d1" },
			{ type: "related_to_recent_search", description: "d2" },
			{ type: "recap_context_unfinished", description: "d3" },
		];
		const groups = categorizeRecallReasons(reasons);
		expect(groups).toHaveLength(3);
		expect(groups.map((g) => g.key)).toEqual([
			"revisit",
			"connection",
			"completion",
		]);
	});
});
