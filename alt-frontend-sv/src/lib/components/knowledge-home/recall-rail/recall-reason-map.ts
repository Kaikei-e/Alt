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
		colorClass: "text-amber-400 border-amber-400/30 bg-amber-400/10",
	},
	related_to_recent_search: {
		label: "Search related",
		iconName: "Search",
		colorClass: "text-blue-400 border-blue-400/30 bg-blue-400/10",
	},
	related_to_recent_augur_question: {
		label: "Augur related",
		iconName: "MessageSquare",
		colorClass: "text-purple-400 border-purple-400/30 bg-purple-400/10",
	},
	recap_context_unfinished: {
		label: "Recap unfinished",
		iconName: "BookOpen",
		colorClass: "text-teal-400 border-teal-400/30 bg-teal-400/10",
	},
	pulse_followup_needed: {
		label: "Pulse follow-up",
		iconName: "Activity",
		colorClass: "text-orange-400 border-orange-400/30 bg-orange-400/10",
	},
	tag_interest_overlap: {
		label: "Interest match",
		iconName: "Tag",
		colorClass: "text-green-400 border-green-400/30 bg-green-400/10",
	},
};

const FALLBACK: RecallReasonDisplay = {
	label: "Recall",
	iconName: "Bell",
	colorClass: "text-gray-400 border-gray-400/30 bg-gray-400/10",
};

export function resolveRecallReason(code: string): RecallReasonDisplay {
	return RECALL_REASON_MAP[code] ?? FALLBACK;
}
