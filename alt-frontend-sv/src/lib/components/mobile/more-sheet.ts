import {
	Bird,
	Compass,
	Settings,
	Link,
	BarChart3,
	Activity,
	Orbit,
	ShieldCheck,
} from "@lucide/svelte";
import type { Component } from "svelte";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type IconComponent = Component<any>;

export interface MoreSheetItem {
	label: string;
	href: string;
	icon: IconComponent;
	badge?: string;
	requiresAdmin?: boolean;
}

export const MORE_SHEET_ITEMS: MoreSheetItem[] = [
	{ label: "Ask Augur", href: "/augur", icon: Bird },
	{ label: "Explore", href: "/feeds/tag-trail", icon: Compass },
	{ label: "Settings", href: "/settings", icon: Settings },
	{ label: "Manage Feeds", href: "/settings/feeds", icon: Link },
	{ label: "Statistics", href: "/stats", icon: BarChart3 },
	{ label: "Job Status", href: "/recap/job-status", icon: Activity },
	{
		label: "Tag Verse",
		href: "/feeds/tag-verse",
		icon: Orbit,
		badge: "Desktop",
	},
	{
		label: "Admin",
		href: "/admin/knowledge-home",
		icon: ShieldCheck,
		requiresAdmin: true,
	},
];

export function getVisibleItems(isAdmin: boolean): MoreSheetItem[] {
	return MORE_SHEET_ITEMS.filter(
		(item) => !item.requiresAdmin || isAdmin,
	);
}
