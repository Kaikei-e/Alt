import { describe, it, expect } from "vitest";

import {
	adaptRecallToLoopEntry,
	confidenceLadderFromRecallScore,
	mapRecallReasonToWhyKind,
} from "./recall-to-loop-adapter";
import type { RecallCandidateData } from "$lib/connect/knowledge_home";

describe("recall-to-loop-adapter", () => {
	it("maps recall reasons[0].type to whyPrimary.kind via the canonical table", () => {
		const candidate: RecallCandidateData = {
			itemKey: "article:1",
			recallScore: 0.4,
			reasons: [
				{ type: "related_to_recent_augur_question", description: "Augur" },
			],
			firstEligibleAt: "2026-05-25T00:00:00Z",
			nextSuggestAt: "2026-05-25T00:00:00Z",
		};
		const entry = adaptRecallToLoopEntry(candidate);
		expect(entry.whyPrimary.kind).toBe("unfinished_continue_why");
		expect(entry.surfaceBucket).toBe("review");
	});

	it("derives confidence ladder from recall_score monotonically", () => {
		expect(confidenceLadderFromRecallScore(0)).toBe("unspecified");
		expect(confidenceLadderFromRecallScore(0.1)).toBe("speculation");
		expect(confidenceLadderFromRecallScore(0.3)).toBe("pattern");
		expect(confidenceLadderFromRecallScore(0.6)).toBe("evidence");
		expect(confidenceLadderFromRecallScore(0.9)).toBe("verified");
	});

	it("falls back to recall_why for unknown reason codes", () => {
		expect(mapRecallReasonToWhyKind("totally_made_up")).toBe("recall_why");
	});

	it("uses recall description as whyPrimary text", () => {
		const candidate: RecallCandidateData = {
			itemKey: "article:2",
			recallScore: 0.8,
			reasons: [
				{ type: "tag_interaction", description: 'You explored tag "AI"' },
			],
			firstEligibleAt: "2026-05-25T00:00:00Z",
			nextSuggestAt: "2026-05-25T00:00:00Z",
		};
		const entry = adaptRecallToLoopEntry(candidate);
		expect(entry.whyPrimary.text).toContain("AI");
		expect(entry.whyPrimary.kind).toBe("topic_affinity_why");
		expect(entry.whyPrimary.confidenceLadder).toBe("verified");
	});

	it("falls back when reasons array is empty", () => {
		const candidate: RecallCandidateData = {
			itemKey: "article:3",
			recallScore: 0.2,
			reasons: [],
			firstEligibleAt: "2026-05-25T00:00:00Z",
			nextSuggestAt: "2026-05-25T00:00:00Z",
		};
		const entry = adaptRecallToLoopEntry(candidate);
		expect(entry.whyPrimary.kind).toBe("recall_why");
		expect(entry.whyPrimary.text).not.toBe("");
	});
});
