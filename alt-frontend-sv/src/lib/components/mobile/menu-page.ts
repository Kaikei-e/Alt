import {
	Activity,
	BarChart3,
	Bird,
	BookOpen,
	Compass,
	Eye,
	GalleryHorizontalEnd,
	Heart,
	Link,
	MessagesSquare,
	Orbit,
	ScrollText,
	ShieldCheck,
} from "@lucide/svelte";
import type { IconProps } from "@lucide/svelte";
import type { Component } from "svelte";

type IconComponent = Component<IconProps>;

export interface MenuGridItem {
	label: string;
	href: string;
	icon: IconComponent;
	badge?: string;
	requiresAdmin?: boolean;
}

export interface MenuSection {
	title: string;
	items: MenuGridItem[];
}

export const MENU_SECTIONS: MenuSection[] = [
	{
		title: "Browse",
		items: [
			{ label: "Library", href: "/feeds", icon: BookOpen },
			{ label: "Favorites", href: "/feeds/favorites", icon: Heart },
			{ label: "Viewed", href: "/feeds/viewed", icon: Eye },
			{ label: "Explore", href: "/feeds/tag-trail", icon: Compass },
			{
				label: "Visual Preview",
				href: "/feeds/swipe/visual-preview",
				icon: GalleryHorizontalEnd,
			},
			{
				label: "Tag Verse",
				href: "/feeds/tag-verse",
				icon: Orbit,
				badge: "Desktop",
			},
		],
	},
	{
		title: "AI & Insights",
		items: [
			{ label: "Ask Augur", href: "/augur", icon: Bird },
			{ label: "Augur History", href: "/augur/history", icon: MessagesSquare },
			{ label: "Acolyte", href: "/acolyte", icon: ScrollText },
			{ label: "Statistics", href: "/stats", icon: BarChart3 },
			{ label: "Job Status", href: "/recap/job-status", icon: Activity },
		],
	},
	{
		title: "Settings",
		items: [{ label: "Manage Feeds", href: "/settings/feeds", icon: Link }],
	},
	{
		title: "Admin",
		items: [
			{
				label: "Admin",
				href: "/admin/knowledge-home",
				icon: ShieldCheck,
				requiresAdmin: true,
			},
		],
	},
];

export function getVisibleSections(isAdmin: boolean): MenuSection[] {
	return MENU_SECTIONS.map((section) => ({
		...section,
		items: section.items.filter((item) => !item.requiresAdmin || isAdmin),
	})).filter((section) => section.items.length > 0);
}
