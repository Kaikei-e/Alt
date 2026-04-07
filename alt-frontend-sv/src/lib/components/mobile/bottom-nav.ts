import {
	Home,
	Search,
	CalendarRange,
	Infinity as InfinityIcon,
	Menu,
} from "@lucide/svelte";
import type { Component } from "svelte";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type IconComponent = Component<any>;

export interface NavTab {
	label: string;
	href: string;
	icon: IconComponent;
}

export const NAV_TABS: NavTab[] = [
	{ label: "Home", href: "/home", icon: Home },
	{ label: "Swipe", href: "/feeds/swipe", icon: InfinityIcon },
	{ label: "Search", href: "/search", icon: Search },
	{ label: "Recap", href: "/recap", icon: CalendarRange },
	{ label: "Menu", href: "/menu", icon: Menu },
];

const HIDE_PATHS = ["/augur", "/feeds/search"];

export function shouldShowBottomNav(pathname: string): boolean {
	return !HIDE_PATHS.includes(pathname);
}

export function getActiveTabIndex(pathname: string): number {
	// Swipe must be checked before general /feeds paths
	if (pathname === "/feeds/swipe" || pathname.startsWith("/feeds/swipe/")) {
		return 1; // Swipe
	}
	for (let i = 0; i < NAV_TABS.length; i++) {
		const tab = NAV_TABS[i];
		if (tab.href === "/feeds/swipe") continue; // Already handled
		if (pathname === tab.href || pathname.startsWith(tab.href + "/")) {
			return i;
		}
	}
	return -1;
}
