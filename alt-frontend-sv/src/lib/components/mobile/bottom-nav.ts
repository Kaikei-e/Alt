import { NAV_TABS, type NavTab } from "$lib/config/navigation";

export { NAV_TABS };
export type { NavTab };

/**
 * Returns the index of the active bottom nav tab for a given pathname.
 *
 * Routing rules:
 * - /home → Home (0)
 * - /search and /feeds/search → Search (2) [search is one concept]
 * - /feeds/* (other) → Feeds (1)
 * - /augur/* → Augur (3)
 * - everything else surfaced via the Menu page → Menu (4)
 */
export function getActiveTabIndex(pathname: string): number {
	if (pathname === "/home" || pathname.startsWith("/home/")) return 0;
	if (
		pathname === "/search" ||
		pathname.startsWith("/search/") ||
		pathname === "/feeds/search" ||
		pathname.startsWith("/feeds/search/")
	) {
		return 2;
	}
	if (pathname === "/feeds" || pathname.startsWith("/feeds/")) return 1;
	if (pathname === "/augur" || pathname.startsWith("/augur/")) return 3;
	return 4;
}
