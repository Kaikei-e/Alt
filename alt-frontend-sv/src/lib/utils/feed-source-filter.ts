/**
 * Shared feed source filtering logic used by both desktop and mobile
 * exclude-source components.
 */

/** Filter sources by URL substring match, case-insensitive, max 10 results. */
export function filterSources<T extends { url: string }>(
	sources: T[],
	query: string,
): T[] {
	if (query.trim() === "") return [];
	return sources
		.filter((s) => s.url.toLowerCase().includes(query.toLowerCase()))
		.slice(0, 10);
}

/** Extract hostname from a URL string. Returns the input on parse failure. */
export function extractDomain(url: string): string {
	try {
		return new URL(url).hostname;
	} catch {
		return url;
	}
}
