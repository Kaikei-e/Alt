/**
 * Maps supersede state codes to display properties.
 */

export interface SupersedeDisplay {
	label: string;
	iconName: string;
	colorClass: string;
}

const SUPERSEDE_MAP: Record<string, SupersedeDisplay> = {
	summary_updated: {
		label: "Summary updated",
		iconName: "FileText",
		colorClass:
			"text-[var(--badge-sky-text)] border-[var(--badge-sky-border)] bg-[var(--badge-sky-bg)]",
	},
	tags_updated: {
		label: "Tags updated",
		iconName: "Tag",
		colorClass:
			"text-[var(--badge-indigo-text)] border-[var(--badge-indigo-border)] bg-[var(--badge-indigo-bg)]",
	},
	reason_updated: {
		label: "Reasons changed",
		iconName: "Info",
		colorClass:
			"text-[var(--badge-amber-text)] border-[var(--badge-amber-border)] bg-[var(--badge-amber-bg)]",
	},
	multiple_updated: {
		label: "Updated",
		iconName: "ArrowUpCircle",
		colorClass:
			"text-[var(--badge-sky-text)] border-[var(--badge-sky-border)] bg-[var(--badge-sky-bg)]",
	},
	both_updated: {
		label: "Updated",
		iconName: "ArrowUpCircle",
		colorClass:
			"text-[var(--badge-sky-text)] border-[var(--badge-sky-border)] bg-[var(--badge-sky-bg)]",
	},
};

const FALLBACK: SupersedeDisplay = {
	label: "Updated",
	iconName: "ArrowUpCircle",
	colorClass:
		"text-[var(--badge-gray-text)] border-[var(--badge-gray-border)] bg-[var(--badge-gray-bg)]",
};

export function resolveSupersede(state: string): SupersedeDisplay {
	return SUPERSEDE_MAP[state] ?? FALLBACK;
}
