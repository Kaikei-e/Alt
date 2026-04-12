/**
 * Maps supersede state codes to display properties.
 *
 * Alt-Paper 3-tier accents:
 *   - Update signals (summary/tags/both/multiple) → --accent-info (Fresh)
 *   - Reason changes                              → --accent-muted (Contextual)
 */

export interface SupersedeDisplay {
	label: string;
	iconName: string;
	colorClass: string;
}

const INFO =
	"text-[var(--accent-info-text)] border-[var(--accent-info-border)] bg-[var(--accent-info-bg)]";
const MUTED =
	"text-[var(--accent-muted-text)] border-[var(--accent-muted-border)] bg-[var(--accent-muted-bg)]";

const SUPERSEDE_MAP: Record<string, SupersedeDisplay> = {
	summary_updated: {
		label: "Summary updated",
		iconName: "FileText",
		colorClass: INFO,
	},
	tags_updated: {
		label: "Tags updated",
		iconName: "Tag",
		colorClass: INFO,
	},
	multiple_updated: {
		label: "Updated",
		iconName: "ArrowUpCircle",
		colorClass: INFO,
	},
	both_updated: {
		label: "Updated",
		iconName: "ArrowUpCircle",
		colorClass: INFO,
	},
	reason_updated: {
		label: "Reasons changed",
		iconName: "Info",
		colorClass: MUTED,
	},
};

const FALLBACK: SupersedeDisplay = {
	label: "Updated",
	iconName: "ArrowUpCircle",
	colorClass: MUTED,
};

export function resolveSupersede(state: string): SupersedeDisplay {
	return SUPERSEDE_MAP[state] ?? FALLBACK;
}
