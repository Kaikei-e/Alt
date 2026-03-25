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
		colorClass:
			"text-[var(--badge-blue-text)] border-[var(--badge-blue-border)] bg-[var(--badge-blue-bg)]",
	},
	in_weekly_recap: {
		label: "In Recap",
		iconName: "CalendarRange",
		colorClass:
			"text-[var(--badge-purple-text)] border-[var(--badge-purple-border)] bg-[var(--badge-purple-bg)]",
	},
	tag_hotspot: {
		label: "Trending",
		iconName: "Tag",
		colorClass:
			"text-[var(--badge-green-text)] border-[var(--badge-green-border)] bg-[var(--badge-green-bg)]",
	},
	summary_completed: {
		label: "Summarized",
		iconName: "FileText",
		colorClass:
			"text-[var(--badge-teal-text)] border-[var(--badge-teal-border)] bg-[var(--badge-teal-bg)]",
	},
	pulse_need_to_know: {
		label: "Need to Know",
		iconName: "Activity",
		colorClass:
			"text-[var(--badge-orange-text)] border-[var(--badge-orange-border)] bg-[var(--badge-orange-bg)]",
	},
	recent_interest_match: {
		label: "Interest",
		iconName: "Star",
		colorClass:
			"text-[var(--badge-yellow-text)] border-[var(--badge-yellow-border)] bg-[var(--badge-yellow-bg)]",
	},
	related_to_recent_search: {
		label: "Search related",
		iconName: "Search",
		colorClass:
			"text-[var(--badge-blue-text)] border-[var(--badge-blue-border)] bg-[var(--badge-blue-bg)]",
	},
};

const FALLBACK: WhyReasonDisplay = {
	label: "Info",
	iconName: "Info",
	colorClass:
		"text-[var(--badge-gray-text)] border-[var(--badge-gray-border)] bg-[var(--badge-gray-bg)]",
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
