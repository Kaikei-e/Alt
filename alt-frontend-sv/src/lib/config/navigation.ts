import {
	Activity,
	BarChart3,
	Bird,
	CalendarRange,
	Compass,
	Eye,
	Globe,
	Heart,
	Home,
	Infinity as InfinityIcon,
	Lightbulb,
	Link as LinkIcon,
	Menu,
	MessagesSquare,
	Moon,
	Newspaper,
	Orbit,
	Rss,
	ScrollText,
	Search,
	ShieldCheck,
	Sparkles,
	Tag,
	Workflow,
} from "@lucide/svelte";
import type { IconProps } from "@lucide/svelte";
import type { Component } from "svelte";

type IconComponent = Component<IconProps>;

export interface NavTab {
	label: string;
	href: string;
	icon: IconComponent;
}

export interface MobileMenuItem {
	label: string;
	href: string;
	icon: IconComponent;
	badge?: string;
	requiresAdmin?: boolean;
}

export interface MobileMenuSection {
	title: string;
	items: MobileMenuItem[];
}

/**
 * Persistent bottom navigation tabs for mobile.
 * Five top-level destinations following Material 3 / NN/g guidance.
 */
export const NAV_TABS: NavTab[] = [
	{ label: "Home", href: "/home", icon: Lightbulb },
	{ label: "Swipe", href: "/feeds/swipe/visual-preview", icon: Rss },
	{ label: "Search", href: "/search", icon: Search },
	{ label: "Augur", href: "/augur", icon: Bird },
	{ label: "Menu", href: "/menu", icon: Menu },
];

/**
 * Mobile Menu page sections.
 * Holds every secondary destination not represented as a primary tab.
 * Items here MUST NOT duplicate hrefs in NAV_TABS (enforced by tests).
 */
export const MOBILE_MENU_SECTIONS: MobileMenuSection[] = [
	{
		title: "Browse",
		items: [
			{ label: "Library", href: "/feeds", icon: Rss },
			{ label: "Swipe Mode", href: "/feeds/swipe", icon: InfinityIcon },
			{ label: "Favorites", href: "/feeds/favorites", icon: Heart },
			{ label: "Viewed", href: "/feeds/viewed", icon: Eye },
			{ label: "Tag Trail", href: "/feeds/tag-trail", icon: Compass },
			{
				label: "Tag Verse",
				href: "/feeds/tag-verse",
				icon: Orbit,
				badge: "Desktop",
			},
			{ label: "Articles by Tag", href: "/articles/by-tag", icon: Tag },
		],
	},
	{
		title: "Recap",
		items: [
			{ label: "3-Day Recap", href: "/recap", icon: CalendarRange },
			{
				label: "Morning Letter",
				href: "/recap/morning-letter",
				icon: Newspaper,
			},
			{ label: "Evening Pulse", href: "/recap/evening-pulse", icon: Moon },
			{ label: "Job Status", href: "/recap/job-status", icon: Activity },
		],
	},
	{
		title: "AI & Insights",
		items: [
			{
				label: "Augur History",
				href: "/augur/history",
				icon: MessagesSquare,
			},
			{ label: "Acolyte Reports", href: "/acolyte", icon: ScrollText },
			{ label: "Knowledge Loop", href: "/loop", icon: Workflow },
			{ label: "Statistics", href: "/stats", icon: BarChart3 },
			{ label: "Daily Pulse", href: "/dashboard", icon: Sparkles },
		],
	},
	{
		title: "Settings",
		items: [
			{ label: "Manage Feed Links", href: "/settings/feeds", icon: LinkIcon },
		],
	},
	{
		title: "Admin",
		items: [
			{
				label: "Knowledge Home Admin",
				href: "/admin/knowledge-home",
				icon: ShieldCheck,
				requiresAdmin: true,
			},
			{
				label: "Manage Domains",
				href: "/admin/scraping-domains",
				icon: Globe,
				requiresAdmin: true,
			},
		],
	},
];

export function getVisibleMobileMenuSections(
	isAdmin: boolean,
): MobileMenuSection[] {
	return MOBILE_MENU_SECTIONS.map((section) => ({
		...section,
		items: section.items.filter((item) => !item.requiresAdmin || isAdmin),
	})).filter((section) => section.items.length > 0);
}

// Re-export Home icon for callers that want it without depending on lucide directly.
export { Home };
