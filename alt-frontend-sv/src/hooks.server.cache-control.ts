/**
 * Stamp every HTML SSR response with a definitive
 * `Cache-Control: no-cache, must-revalidate` header.
 *
 * Vite's "Building for Production / Load Error Handling" docs
 * explicitly require this so the browser revalidates the HTML on every
 * navigation. Without it, a tab keeps a stale HTML referencing now-evicted
 * `_app/immutable/*` chunk hashes after a deploy → 404 → "Cannot Open
 * the Page" on iOS Safari.
 *
 * Only mutates the header on `text/html` responses. Connect-RPC JSON,
 * immutable JS chunks, and the image proxy keep their own caching policy.
 */

export function applyHtmlCacheControl(response: Response): void {
	const contentType = response.headers.get("content-type") ?? "";
	if (!contentType.startsWith("text/html")) return;
	try {
		response.headers.set("cache-control", "no-cache, must-revalidate");
	} catch {
		// Frozen headers (rare; some test environments) — soft fail rather
		// than 500 the request over an observability header.
	}
}

/**
 * Default every `/api/` JSON response to `Cache-Control: private, no-store`
 * unless the route already set its own Cache-Control header deliberately.
 *
 * Most `+server.ts` GET handlers return personal data (feed lists, dashboard
 * metrics, admin snapshots) with no Cache-Control header at all today, so
 * the app has no independent defense against a future CDN/proxy_cache
 * addition caching a response across users. See OWASP ASVS V14 (data
 * protection) — sensitive responses must be marked non-cacheable at the
 * application layer, not left to infra defaults.
 */
export function applyApiCacheControl(response: Response, pathname: string): void {
	if (!pathname.startsWith("/api/")) return;
	const contentType = response.headers.get("content-type") ?? "";
	if (!contentType.startsWith("application/json")) return;
	if (response.headers.has("cache-control")) return;
	try {
		response.headers.set("cache-control", "private, no-store");
	} catch {
		// Frozen headers (rare; some test environments) — soft fail rather
		// than 500 the request over an observability header.
	}
}
