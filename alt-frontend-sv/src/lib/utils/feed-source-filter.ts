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

/** Extract effective (registerable) domain, stripping subdomains like feeds., rss., www. */
export function getEffectiveDomain(url: string): string {
	try {
		const hostname = new URL(url).hostname;
		const parts = hostname.split(".");
		if (parts.length <= 2) return hostname;
		return parts.slice(-2).join(".");
	} catch {
		return url;
	}
}

/** Group sources by effective domain. */
export function groupSourcesByDomain<T extends { url: string }>(
	sources: T[],
): Map<string, T[]> {
	const map = new Map<string, T[]>();
	for (const source of sources) {
		const domain = getEffectiveDomain(source.url);
		const existing = map.get(domain);
		if (existing) {
			existing.push(source);
		} else {
			map.set(domain, [source]);
		}
	}
	return map;
}

/** Collect all feed_link_ids that belong to the same effective domain. */
export function collectFeedLinkIdsByDomain<
	T extends { id: string; url: string },
>(sources: T[], targetDomain: string): string[] {
	return sources
		.filter((s) => getEffectiveDomain(s.url) === targetDomain)
		.map((s) => s.id);
}
