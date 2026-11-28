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
	"/desktop/recap": "/recap",
	"/mobile/recap/3days": "/recap",
	"/desktop/recap/job-status": "/recap/job-status",
	"/mobile/recap/job-status": "/recap/job-status",
	// Batch D
	"/desktop/settings/feeds": "/settings/feeds",
	"/mobile/feeds/manage": "/settings/feeds",
	"/desktop/stats": "/stats",
	"/mobile/feeds/stats": "/stats",
	// Batch E
	"/feeds/swipe": "/feeds/swipe/visual-preview",
	"/mobile/feeds/swipe": "/feeds/swipe/visual-preview",
	"/desktop": "/dashboard",
	// ADR-001043: retired Knowledge Home & Knowledge Trail surfaces
	// ((app)/+layout.ts sets ssr = false, so hooks.server.ts must emit the HTTP 303)
	"/home": "/today",
	"/knowledge/trail": "/knowledge",
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
