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
