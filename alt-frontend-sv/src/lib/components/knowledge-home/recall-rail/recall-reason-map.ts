/**
 * Maps recall reason codes to display labels, icons, and colors.
 * Follows why-reason-map.ts pattern.
 */

export interface RecallReasonDisplay {
	label: string;
	iconName: string;
	colorClass: string;
}

const RECALL_REASON_MAP: Record<string, RecallReasonDisplay> = {
	opened_before_but_not_revisited: {
		label: "Not revisited",
		iconName: "RotateCcw",
		colorClass:
			"text-[var(--badge-amber-text)] border-[var(--badge-amber-border)] bg-[var(--badge-amber-bg)]",
	},
	related_to_recent_search: {
		label: "Search related",
		iconName: "Search",
		colorClass:
			"text-[var(--badge-blue-text)] border-[var(--badge-blue-border)] bg-[var(--badge-blue-bg)]",
	},
	related_to_recent_augur_question: {
		label: "Augur related",
		iconName: "MessageSquare",
		colorClass:
			"text-[var(--badge-purple-text)] border-[var(--badge-purple-border)] bg-[var(--badge-purple-bg)]",
	},
	recap_context_unfinished: {
		label: "Recap unfinished",
		iconName: "BookOpen",
		colorClass:
			"text-[var(--badge-teal-text)] border-[var(--badge-teal-border)] bg-[var(--badge-teal-bg)]",
	},
	pulse_followup_needed: {
		label: "Pulse follow-up",
		iconName: "Activity",
		colorClass:
			"text-[var(--badge-orange-text)] border-[var(--badge-orange-border)] bg-[var(--badge-orange-bg)]",
	},
	tag_interest_overlap: {
		label: "Interest match",
		iconName: "Tag",
		colorClass:
			"text-[var(--badge-green-text)] border-[var(--badge-green-border)] bg-[var(--badge-green-bg)]",
	},
	tag_interaction: {
		label: "Tag explored",
		iconName: "Hash",
		colorClass:
			"text-[var(--badge-teal-text)] border-[var(--badge-teal-border)] bg-[var(--badge-teal-bg)]",
	},
};

const FALLBACK: RecallReasonDisplay = {
	label: "Recall",
	iconName: "Bell",
	colorClass:
		"text-[var(--badge-gray-text)] border-[var(--badge-gray-border)] bg-[var(--badge-gray-bg)]",
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
