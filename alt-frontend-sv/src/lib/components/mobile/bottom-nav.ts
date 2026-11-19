import { NAV_TABS, type NavTab } from "#lib/config/navigation.js";

export type { NavTab };
export { NAV_TABS };

/**
 * Immersive mode: swipe dispatch surfaces dismiss the persistent bottom nav
 * and rely on the Dispatch header's back link as the visible exit.
 */
export function isImmersiveRoute(pathname: string): boolean {
	return pathname === "/feeds/swipe" || pathname.startsWith("/feeds/swipe/");
}

/**
 * Returns the index of the active bottom nav tab for a given pathname.
 *
 * Routing rules:
 * - /today/* → Today (0)
 * - /feeds/swipe/* → Swipe (1)
 * - /feeds/visual-preview/* → Gallery (2)
 * - /search/* and /feeds/search/* → Search (3)
 * - everything else surfaced via the Menu page → Menu (4)
 */
export function getActiveTabIndex(pathname: string): number {
	if (pathname === "/today" || pathname.startsWith("/today/")) return 0;
	if (pathname === "/feeds/swipe" || pathname.startsWith("/feeds/swipe/")) {
		return 1;
	}
	if (
		pathname === "/feeds/visual-preview" ||
		pathname.startsWith("/feeds/visual-preview/")
	) {
		return 2;
	}
	if (
		pathname === "/search" ||
		pathname.startsWith("/search/") ||
		pathname === "/feeds/search" ||
		pathname.startsWith("/feeds/search/")
	) {
		return 3;
	}
	return 4;
}
