import { NAV_TABS, type NavTab } from "$lib/config/navigation";

export { NAV_TABS };
export type { NavTab };

/**
 * Returns the index of the active bottom nav tab for a given pathname.
 *
 * Routing rules:
 * - /home → Home (0)
 * - /loop/* → Loop (1) [primary cognitive-state surface]
 * - /search and /feeds/search → Search (2) [search is one concept]
 * - /augur/* → Augur (3)
 * - everything else surfaced via the Menu page (including /feeds raw inbox) → Menu (4)
 */
export function getActiveTabIndex(pathname: string): number {
	if (pathname === "/home" || pathname.startsWith("/home/")) return 0;
	if (pathname === "/loop" || pathname.startsWith("/loop/")) return 1;
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
