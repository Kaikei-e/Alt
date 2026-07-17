import { NAV_TABS, type NavTab } from "$lib/config/navigation";

export type { NavTab };
export { NAV_TABS };

/**
 * Returns the index of the active bottom nav tab for a given pathname.
 *
 * Routing rules:
 * - /home → Home (0)
 * - /knowledge/trail/* → Trail (1) [the footprint spine]
 * - /search and /feeds/search → Search (2) [search is one concept]
 * - /augur/* → Augur (3)
 * - everything else surfaced via the Menu page (including /feeds raw inbox) → Menu (4)
 */
/**
 * Immersive mode (contexts/visual-preview.md): swipe dispatch surfaces
 * dismiss the persistent bottom nav and rely on the Dispatch header's
 * back link as the visible exit.
 */
export function isImmersiveRoute(pathname: string): boolean {
	return pathname === "/feeds/swipe" || pathname.startsWith("/feeds/swipe/");
}

export function getActiveTabIndex(pathname: string): number {
	if (pathname === "/home" || pathname.startsWith("/home/")) return 0;
	if (
		pathname === "/knowledge/trail" ||
		pathname.startsWith("/knowledge/trail/")
	)
		return 1;
	if (
		pathname === "/search" ||
		pathname.startsWith("/search/") ||
		pathname === "/feeds/search" ||
		pathname.startsWith("/feeds/search/")
	) {
		return 2;
	}
	if (pathname === "/augur" || pathname.startsWith("/augur/")) return 3;
	return 4;
}
