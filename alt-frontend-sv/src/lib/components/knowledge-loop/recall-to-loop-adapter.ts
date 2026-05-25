/**
 * Adapter that maps a RecallCandidate (knowledge_home payload) to a
 * KnowledgeLoopEntryData-shaped object so the Loop Review plane can render
 * recall items with the same tile component as projector entries.
 *
 * ADR-000913 §D-9 / canonical contract §6.4: Review bucket absorbs the
 * recall rail. The adapter is a pure function — no time.Now(), no fetch.
 * Same inputs → same outputs, which keeps reproject semantics intact when
 * the UI flips Lens or re-derives the Review bucket.
 *
 * Grounding: Andy Matuschak's evergreen-note maintenance posits that
 * spaced re-engagement with prior writing approximates spaced repetition,
 * without an SRS schedule. We surface recall candidates in Review as
 * "the system thinks this is worth a second pass" — never as "your next
 * Anki card is due".
 */

import type {
	KnowledgeLoopEntryData,
	WhyPayloadData,
	ConfidenceLadderName,
	WhyKindName,
} from "$lib/connect/knowledge_loop";
import type {
	RecallCandidateData,
	RecallReasonData,
} from "$lib/connect/knowledge_home";

/**
 * Map a RecallReason.type code to the closest WhyKind so the Review tile
 * picks the right banner. Unknown codes fall back to RECALL — the
 * residual "you've encountered this" kind per canonical contract §11.
 */
export function mapRecallReasonToWhyKind(type: string): WhyKindName {
	switch (type) {
		case "opened_before_but_not_revisited":
		case "recap_context_unfinished":
			return "recall_why";
		case "related_to_recent_augur_question":
			return "unfinished_continue_why";
		case "related_to_recent_search":
		case "tag_interest_overlap":
		case "tag_interaction":
			return "topic_affinity_why";
		case "pulse_followup_needed":
			return "pattern_why";
		default:
			return "recall_why";
	}
}

/**
 * Derive a confidence ladder tier from recall_score. Thresholds match the
 * canonical contract §11 ladder: <0.25 SPECULATION, <0.5 PATTERN, <0.75
 * EVIDENCE, >=0.75 VERIFIED. The function is monotone so a higher score
 * never lands on a lower tier.
 */
export function confidenceLadderFromRecallScore(
	score: number,
): ConfidenceLadderName {
	if (!Number.isFinite(score) || score <= 0) {
		return "unspecified";
	}
	if (score < 0.25) return "speculation";
	if (score < 0.5) return "pattern";
	if (score < 0.75) return "evidence";
	return "verified";
}

/**
 * Convert a recall candidate into a Knowledge Loop entry data shape so
 * the existing LoopEntryTile / ReviewDock components can render it
 * without a dedicated recall component. The result is reproject-safe:
 * every field is a pure function of the candidate payload.
 */
export function adaptRecallToLoopEntry(
	candidate: RecallCandidateData,
): KnowledgeLoopEntryData {
	const primaryReason: RecallReasonData | undefined = candidate.reasons[0];
	const whyKind = mapRecallReasonToWhyKind(primaryReason?.type ?? "");
	const text =
		primaryReason?.description ?? "Recall candidate from your knowledge stream.";

	const why: WhyPayloadData = {
		kind: whyKind,
		text,
		confidenceLadder: confidenceLadderFromRecallScore(candidate.recallScore),
		evidenceRefs: [],
	};

	return {
		entryKey: candidate.itemKey,
		sourceItemKey: candidate.itemKey,
		proposedStage: "observe",
		surfaceBucket: "review",
		projectionRevision: 0,
		projectionSeqHiwater: 0,
		freshnessAt: candidate.firstEligibleAt,
		whyPrimary: why,
		dismissState: "active",
		renderDepthHint: 1,
		loopPriority: "reference",
		decisionOptions: [],
		actTargets: [],
	};
}
