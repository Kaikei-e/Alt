import {
	Home,
	Rss,
	Eye,
	Star,
	Search,
	CalendarRange,
	Newspaper,
	BirdIcon,
	ChartBar,
	Settings,
	LinkIcon,
	Plus,
	Activity,
	Sparkles,
	Shuffle,
	Compass,
	Infinity as InfinityIcon,
	Moon,
	Globe,
} from "@lucide/svelte";
import type { Component } from "svelte";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type IconComponent = Component<any>;

export interface NavigationItem {
	label: string;
	href: string;
	icon: IconComponent;
	description?: string;
}

export interface NavigationSection {
	label: string;
	category: string;
	icon: IconComponent;
	children: NavigationItem[];
}

export type NavigationEntry =
	| { type: "link"; item: NavigationItem & { category: string } }
	| { type: "section"; section: NavigationSection };

const svBasePath = "/sv";

/**
 * Sidebar navigation items for desktop.
 * Uses unified paths (no /desktop/ prefix).
 */
export const desktopNavigation: Array<
	| { label: string; href: string; icon: IconComponent; category: string }
	| NavigationSection
> = [
	{
		label: "Dashboard",
		href: `${svBasePath}/dashboard`,
		icon: Home,
		category: "main",
	},
	{
		label: "Feeds",
		category: "feeds",
		icon: Rss,
		children: [
			{
				label: "Unread Feeds",
				href: `${svBasePath}/feeds`,
				icon: Rss,
			},
			{
				label: "Read History",
				href: `${svBasePath}/feeds/viewed`,
				icon: Eye,
			},
			{
				label: "Favorites",
				href: `${svBasePath}/feeds/favorites`,
				icon: Star,
			},
			{
				label: "Search",
				href: `${svBasePath}/feeds/search`,
				icon: Search,
			},
		],
	},
	{
		label: "Recap",
		category: "recap",
		icon: CalendarRange,
		children: [
			{
				label: "3-Day Summary",
				href: `${svBasePath}/recap`,
				icon: CalendarRange,
			},
			{
				label: "Morning Letter",
				href: `${svBasePath}/recap/morning-letter`,
				icon: Newspaper,
			},
			{
				label: "Evening Pulse",
				href: `${svBasePath}/recap/evening-pulse`,
				icon: Sparkles,
			},
			{
				label: "Job Status",
				href: `${svBasePath}/recap/job-status`,
				icon: Activity,
			},
		],
	},
	{
		label: "Explore",
		category: "explore",
		icon: Compass,
		children: [
			{
				label: "Tag Trail",
				href: `${svBasePath}/feeds/tag-trail`,
				icon: Shuffle,
			},
		],
	},
	{
		label: "Ask Augur",
		href: `${svBasePath}/augur`,
		icon: BirdIcon,
		category: "main",
	},
	{
		label: "Settings",
		category: "settings",
		icon: Settings,
		children: [
			{
				label: "Manage Feed Links",
				href: `${svBasePath}/settings/feeds`,
				icon: LinkIcon,
			},
		],
	},
	{
		label: "Statistics",
		href: `${svBasePath}/stats`,
		icon: ChartBar,
		category: "main",
	},
];

/**
 * Mobile navigation items (used by FloatingMenu).
 * Uses unified paths where migrated, old paths where not yet migrated.
 */
export const mobileMenuItems = [
	{
		label: "View Feeds",
		href: `${svBasePath}/feeds`,
		category: "feeds",
		icon: Rss,
		description: "Browse all RSS feeds",
	},
	{
		label: "Swipe Mode",
		href: `${svBasePath}/feeds/swipe`,
		category: "feeds",
		icon: InfinityIcon,
		description: "Swipe through feeds",
	},
	{
		label: "Viewed Feeds",
		href: `${svBasePath}/feeds/viewed`,
		category: "feeds",
		icon: Eye,
		description: "Previously read feeds",
	},
	{
		label: "Favorite Feeds",
		href: `${svBasePath}/feeds/favorites`,
		category: "feeds",
		icon: Star,
		description: "Favorited articles",
	},
	{
		label: "Register Feed",
		href: `${svBasePath}/mobile/feeds/register`,
		category: "feeds",
		icon: Plus,
		description: "Add new RSS feed",
	},
	{
		label: "Manage Feeds Links",
		href: `${svBasePath}/settings/feeds`,
		category: "feeds",
		icon: LinkIcon,
		description: "Add or remove your registered RSS sources",
	},
	{
		label: "Search Feeds",
		href: `${svBasePath}/feeds/search`,
		category: "feeds",
		icon: Search,
		description: "Find specific feeds",
	},
	{
		label: "Tag Trail",
		href: `${svBasePath}/feeds/tag-trail`,
		category: "explore",
		icon: Shuffle,
		description: "Discover feeds by exploring tags",
	},
	{
		label: "Ask Augur",
		href: `${svBasePath}/augur`,
		category: "augur",
		icon: BirdIcon,
		description: "Chat with your knowledge base",
	},
	{
		label: "3-Day Recap",
		href: `${svBasePath}/recap`,
		category: "recap",
		icon: CalendarRange,
		description: "Review recent highlights",
	},
	{
		label: "Morning Letter",
		href: `${svBasePath}/recap/morning-letter`,
		category: "recap",
		icon: Newspaper,
		description: "Today's overnight updates",
	},
	{
		label: "Evening Pulse",
		href: `${svBasePath}/recap/evening-pulse`,
		category: "recap",
		icon: Moon,
		description: "Tonight's key highlights",
	},
	{
		label: "Job Status",
		href: `${svBasePath}/recap/job-status`,
		category: "recap",
		icon: Activity,
		description: "Monitor recap job progress",
	},
	{
		label: "View Articles",
		href: `${svBasePath}/mobile/articles/view`,
		category: "articles",
		icon: Newspaper,
		description: "Browse all articles",
	},
	{
		label: "Search Articles",
		href: `${svBasePath}/mobile/articles/search`,
		category: "articles",
		icon: Search,
		description: "Search through articles",
	},
	{
		label: "View Stats",
		href: `${svBasePath}/stats`,
		category: "other",
		icon: ChartBar,
		description: "Analytics & insights",
	},
	{
		label: "Home",
		href: `${svBasePath}/`,
		category: "other",
		icon: Home,
		description: "Return to dashboard",
	},
	{
		label: "Manage Domains",
		href: `${svBasePath}/admin/scraping-domains`,
		category: "other",
		icon: Globe,
		description: "Manage scraping domains",
	},
];

export const mobileCategories = [
	{
		title: "Feeds",
		items: mobileMenuItems.filter((i) => i.category === "feeds"),
		icon: Rss,
	},
	{
		title: "Explore",
		items: mobileMenuItems.filter((i) => i.category === "explore"),
		icon: Compass,
	},
	{
		title: "Recap",
		items: mobileMenuItems.filter((i) => i.category === "recap"),
		icon: CalendarRange,
	},
	{
		title: "Articles",
		items: mobileMenuItems.filter((i) => i.category === "articles"),
		icon: Newspaper,
	},
	{
		title: "Augur",
		items: mobileMenuItems.filter((i) => i.category === "augur"),
		icon: BirdIcon,
		description: "Chat with your knowledge base",
	},
	{
		title: "Other",
		items: mobileMenuItems.filter((i) => i.category === "other"),
		icon: Star,
	},
];
