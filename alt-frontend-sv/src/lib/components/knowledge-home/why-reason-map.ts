/**
 * Maps WHY reason codes to display labels, icon names, and color classes.
 * Extracted for testability — used by WhySurfacedBadge.svelte.
 */

export interface WhyReasonDisplay {
	label: string;
	iconName: string;
	colorClass: string;
}

const WHY_REASON_MAP: Record<string, WhyReasonDisplay> = {
	new_unread: {
		label: "New",
		iconName: "Sparkles",
		colorClass: "text-blue-400 border-blue-400/30 bg-blue-400/10",
	},
	in_weekly_recap: {
		label: "In Recap",
		iconName: "CalendarRange",
		colorClass: "text-purple-400 border-purple-400/30 bg-purple-400/10",
	},
	tag_hotspot: {
		label: "Trending",
		iconName: "Tag",
		colorClass: "text-green-400 border-green-400/30 bg-green-400/10",
	},
	summary_completed: {
		label: "Summarized",
		iconName: "FileText",
		colorClass: "text-teal-400 border-teal-400/30 bg-teal-400/10",
	},
	pulse_need_to_know: {
		label: "Need to Know",
		iconName: "Activity",
		colorClass: "text-orange-400 border-orange-400/30 bg-orange-400/10",
	},
	recent_interest_match: {
		label: "Interest",
		iconName: "Star",
		colorClass: "text-yellow-400 border-yellow-400/30 bg-yellow-400/10",
	},
	related_to_recent_search: {
		label: "Search related",
		iconName: "Search",
		colorClass: "text-blue-400 border-blue-400/30 bg-blue-400/10",
	},
};

const FALLBACK: WhyReasonDisplay = {
	label: "Info",
	iconName: "Info",
	colorClass: "text-gray-400 border-gray-400/30 bg-gray-400/10",
};

/**
 * Resolves a WHY reason code to its display properties.
 * For `tag_hotspot`, appends the tag name to the label.
 */
export function resolveWhyReason(code: string, tag?: string): WhyReasonDisplay {
	const base = WHY_REASON_MAP[code] ?? FALLBACK;
	if (code === "tag_hotspot" && tag) {
		return { ...base, label: `Trending: ${tag}` };
	}
	return base;
}
