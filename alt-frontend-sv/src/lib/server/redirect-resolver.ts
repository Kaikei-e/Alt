// Redirect map: old path-based routes -> unified responsive routes
export const RESPONSIVE_REDIRECTS: Record<string, string> = {
	// Phase 1 (completed)
	"/desktop/feeds": "/feeds",
	"/mobile/feeds": "/feeds",
	// Batch A
	"/desktop/augur": "/augur",
	"/mobile/retrieve/ask-augur": "/augur",
	"/desktop/recap/morning-letter": "/recap/morning-letter",
	"/mobile/recap/morning-letter": "/recap/morning-letter",
	"/desktop/feeds/tag-trail": "/feeds/tag-trail",
	"/mobile/feeds/tag-trail": "/feeds/tag-trail",
	"/desktop/feeds/tag-verse": "/feeds/tag-verse",
	"/desktop/feeds/favorites": "/feeds/favorites",
	// Batch B
	"/desktop/feeds/search": "/feeds/search",
	"/mobile/feeds/search": "/feeds/search",
	"/desktop/feeds/viewed": "/feeds/viewed",
	"/mobile/feeds/viewed": "/feeds/viewed",
	// Batch C
	"/desktop/recap/evening-pulse": "/recap/evening-pulse",
	"/mobile/recap/evening-pulse": "/recap/evening-pulse",
	"/desktop/recap": "/recap",
	"/mobile/recap/3days": "/recap",
	"/mobile/recap/7days": "/recap?window=7",
	"/desktop/recap/job-status": "/recap/job-status",
	"/mobile/recap/job-status": "/recap/job-status",
	// Batch D
	"/desktop/settings/feeds": "/settings/feeds",
	"/mobile/feeds/manage": "/settings/feeds",
	"/desktop/stats": "/stats",
	"/mobile/feeds/stats": "/stats",
	// Batch E
	"/mobile/feeds/swipe": "/feeds/swipe",
	"/desktop": "/dashboard",
};

/**
 * Resolve a legacy path-based route to a unified responsive route,
 * preserving query parameters from the original URL.
 *
 * Returns null if no redirect is needed.
 */
export function resolveResponsiveRedirect(
	pathname: string,
	search: string,
): string | null {
	const target = RESPONSIVE_REDIRECTS[pathname];
	if (!target) return null;

	if (!search) return target;

	// If target already has query params, merge with &
	if (target.includes("?")) {
		return `${target}&${search.slice(1)}`;
	}

	return target + search;
}
