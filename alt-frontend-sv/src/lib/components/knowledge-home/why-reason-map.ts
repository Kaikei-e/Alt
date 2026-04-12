/**
 * Maps WHY reason codes to display labels, icon names, and 3-tier accent classes.
 * Extracted for testability — used by WhySurfacedBadge.svelte.
 *
 * Alt-Paper 3-tier accent system:
 *   - Urgent      → --accent-emphasis  (oxblood ink)
 *   - Fresh       → --accent-info      (ink navy)
 *   - Contextual  → --accent-muted     (slate neutral)
 */

export interface WhyReasonDisplay {
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

const WHY_REASON_MAP: Record<string, WhyReasonDisplay> = {
	pulse_need_to_know: {
		label: "Need to Know",
		iconName: "Activity",
		colorClass: EMPHASIS,
	},
	new_unread: { label: "New", iconName: "Sparkles", colorClass: INFO },
	summary_completed: {
		label: "Summarized",
		iconName: "FileText",
		colorClass: INFO,
	},
	in_weekly_recap: {
		label: "In Recap",
		iconName: "CalendarRange",
		colorClass: MUTED,
	},
	tag_hotspot: { label: "Trending", iconName: "Tag", colorClass: MUTED },
	recent_interest_match: {
		label: "Interest",
		iconName: "Star",
		colorClass: MUTED,
	},
	related_to_recent_search: {
		label: "Search related",
		iconName: "Search",
		colorClass: MUTED,
	},
};

const FALLBACK: WhyReasonDisplay = {
	label: "Info",
	iconName: "Info",
	colorClass: MUTED,
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
