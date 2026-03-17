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
		colorClass: "text-sky-400 border-sky-400/30 bg-sky-400/10",
	},
	tags_updated: {
		label: "Tags updated",
		iconName: "Tag",
		colorClass: "text-indigo-400 border-indigo-400/30 bg-indigo-400/10",
	},
	both_updated: {
		label: "Updated",
		iconName: "ArrowUpCircle",
		colorClass: "text-sky-400 border-sky-400/30 bg-sky-400/10",
	},
};

const FALLBACK: SupersedeDisplay = {
	label: "Updated",
	iconName: "ArrowUpCircle",
	colorClass: "text-gray-400 border-gray-400/30 bg-gray-400/10",
};

export function resolveSupersede(state: string): SupersedeDisplay {
	return SUPERSEDE_MAP[state] ?? FALLBACK;
}
