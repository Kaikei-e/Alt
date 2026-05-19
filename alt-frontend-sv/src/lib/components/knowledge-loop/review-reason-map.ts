/**
 * Maps a Review-bucket entry's lifecycle to the reason label shown next to
 * the why-kind chip in ReviewDock. The Review bucket is a deliberate
 * re-evaluation queue: each row should make clear *why* the entry is here,
 * so stale evidence reads differently from a previously-dismissed v1
 * fallback or a user-acknowledged "Reviewed" item.
 *
 * Derivation, not persistence: review_reason is not a separate proto field.
 * It is derived deterministically from `dismissState` + `surfaceBucket` so
 * a reproject that produces the same projection row produces the same
 * label without any payload contract change. The read path already filters
 * `visibility_state='visible'`, so HIDDEN (archive outcome) entries never
 * reach this helper.
 *
 * Mapping (canonical contract §6.4 + plan/knowledge-loop-completion-03):
 *
 *   dismissState='active'    & bucket=REVIEW → Stale evidence
 *   dismissState='dismissed' & bucket=REVIEW → Previously dismissed
 *   dismissState='completed' & bucket=REVIEW → Reviewed
 *   dismissState='deferred'  & bucket=REVIEW → Deferred (not normally
 *                                              visible — DEFERRED maps to
 *                                              SNOOZED visibility — but
 *                                              kept here for completeness
 *                                              if a future read filter
 *                                              widens to include snoozed).
 *   anything outside Review                → undefined  (caller skips render)
 *
 * Labels use the Alt-Paper editorial register: short, lowercase except first
 * word, functional voice. They sit in the existing `.why-kind` register so
 * no new design token is introduced.
 */

import type {
	DismissStateName,
	SurfaceBucketName,
} from "$lib/connect/knowledge_loop";

export type ReviewReason =
	| "stale_evidence"
	| "previously_dismissed"
	| "reviewed"
	| "deferred";

export interface ReviewReasonDisplay {
	reason: ReviewReason;
	label: string;
	kicker: string;
	ariaText: string;
	/**
	 * Action-oriented guidance line per ADR-000907 §Δ8 / canonical contract §3.
	 * "What changed" alone is insufficient; the Review card must hint at "how
	 * to update understanding" so the bucket is a deliberate re-evaluation
	 * queue rather than a leftover surface. Deterministic template (no LLM).
	 */
	guidance: string;
}

const DISPLAY: Record<ReviewReason, ReviewReasonDisplay> = {
	stale_evidence: {
		reason: "stale_evidence",
		label: "Stale evidence",
		kicker: "STALE",
		ariaText: "Re-evaluation candidate: stale evidence",
		guidance: "Last seen a while ago. Worth a 30-second refresh?",
	},
	previously_dismissed: {
		reason: "previously_dismissed",
		label: "Previously dismissed",
		kicker: "DISMISSED",
		ariaText: "Re-evaluation candidate: previously dismissed",
		// Held for back-compat: ADR-000907 §Δ8 stopped routing dismiss to
		// Review, so this branch is dormant on the new projector path. If a
		// pre-cutover row is still rendered, surface the previous intent
		// without urging re-evaluation.
		guidance: "Dismissed earlier — nothing new since then.",
	},
	reviewed: {
		reason: "reviewed",
		label: "Reviewed",
		kicker: "REVIEWED",
		ariaText: "Acknowledged: reviewed",
		guidance: "You've already reviewed this. No follow-up needed.",
	},
	deferred: {
		reason: "deferred",
		label: "Deferred",
		kicker: "DEFERRED",
		ariaText: "Re-evaluation candidate: deferred",
		guidance: "Snoozed earlier — pick a moment to revisit.",
	},
};

/**
 * Returns the Review reason display for an entry, or `undefined` when the
 * caller should skip rendering a reason chip (entry is not in Review or its
 * lifecycle does not map to a known reason).
 */
export function resolveReviewReason(args: {
	dismissState: DismissStateName;
	surfaceBucket: SurfaceBucketName;
}): ReviewReasonDisplay | undefined {
	if (args.surfaceBucket !== "review") return undefined;
	switch (args.dismissState) {
		case "active":
			return DISPLAY.stale_evidence;
		case "dismissed":
			return DISPLAY.previously_dismissed;
		case "completed":
			return DISPLAY.reviewed;
		case "deferred":
			return DISPLAY.deferred;
		default:
			return undefined;
	}
}
