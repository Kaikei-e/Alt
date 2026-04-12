/**
 * Maps recall reason codes to display labels, icons, and 3-tier accent classes.
 * Mirrors the why-reason-map.ts Alt-Paper collapse:
 *   - Urgent      → --accent-emphasis
 *   - Fresh       → --accent-info
 *   - Contextual  → --accent-muted
 */

export interface RecallReasonDisplay {
	label: string;
	iconName: string;
	colorClass: string;
}

const EMPHASIS =
	"text-[var(--accent-emphasis-text)] border-[var(--accent-emphasis-border)] bg-[var(--accent-emphasis-bg)]";
const INFO =
	"text-[var(--accent-info-text)] border-[var(--accent-info-border)] bg-[var(--accent-info-bg)]";
const MUTED =
	"text-[var(--accent-muted-text)] border-[var(--accent-muted-border)] bg-[var(--accent-muted-bg)]";

const RECALL_REASON_MAP: Record<string, RecallReasonDisplay> = {
	pulse_followup_needed: {
		label: "Pulse follow-up",
		iconName: "Activity",
		colorClass: EMPHASIS,
	},
	related_to_recent_search: {
		label: "Search related",
		iconName: "Search",
		colorClass: INFO,
	},
	recap_context_unfinished: {
		label: "Recap unfinished",
		iconName: "BookOpen",
		colorClass: INFO,
	},
	opened_before_but_not_revisited: {
		label: "Not revisited",
		iconName: "RotateCcw",
		colorClass: MUTED,
	},
	related_to_recent_augur_question: {
		label: "Augur related",
		iconName: "MessageSquare",
		colorClass: MUTED,
	},
	tag_interest_overlap: {
		label: "Interest match",
		iconName: "Tag",
		colorClass: MUTED,
	},
	tag_interaction: {
		label: "Tag explored",
		iconName: "Hash",
		colorClass: MUTED,
	},
};

const FALLBACK: RecallReasonDisplay = {
	label: "Recall",
	iconName: "Bell",
	colorClass: MUTED,
};

/**
 * Resolves a recall reason code to its display properties.
 * When the code is unknown but a description is available, uses it as the label
 * instead of the generic "Recall" fallback — supporting Why-First principle.
 */
export function resolveRecallReason(
	code: string,
	description?: string,
): RecallReasonDisplay {
	const mapped = RECALL_REASON_MAP[code];
	if (mapped) return mapped;
	if (description) return { ...FALLBACK, label: description };
	return FALLBACK;
}
