/**
 * Status-aware loader for OG image proxy URLs.
 *
 * Fetches the proxied image through the per-host queue so it can read the HTTP
 * status: a transient rate-limit (429) or upstream blip (502/503/504) is retried
 * with backoff, while a permanent rejection (403/400/404) resolves to `absent`
 * immediately. This is what stops a transient failure from pinning the card to
 * the fallback gradient — the bug behind the mark-as-read regression.
 *
 * Probing with fetch() rather than a bare <img src> is what gives us the
 * status: an `<img>` reports both a 429 and a 403 as the same silent `onerror`,
 * so the distinction this loader exists to make is invisible from the element.
 * The probe yields a verdict and nothing else — the caller renders the proxy
 * URL itself. Those are not two downloads: the proxy answers
 * `Cache-Control: public, max-age=604800, immutable` under `Vary:
 * Accept-Encoding` (`alt-backend/app/orchestrator/rest/image_proxy_handlers.go`),
 * so the probe's response sits in the browser's HTTP cache under the same key
 * the `<img>` request uses.
 */

import { imageLoadQueue } from "./imageLoadQueue";

export type ImageLoadResult = { status: "loaded" } | { status: "absent" };

export interface LoadProxyImageDeps {
	fetch: typeof fetch;
	acquire: (proxyUrl: string) => Promise<() => void>;
	sleep: (ms: number) => Promise<void>;
	/** Injectable for deterministic jitter in tests. */
	random?: () => number;
}

const RETRYABLE_STATUS = new Set([408, 425, 429, 500, 502, 503, 504]);
const BACKOFFS_MS = [1500, 3000]; // 2 retries
const JITTER_MS = 400;

export async function loadProxyImage(
	proxyUrl: string | undefined | null,
	deps: LoadProxyImageDeps,
	signal?: AbortSignal,
): Promise<ImageLoadResult> {
	if (!proxyUrl) return { status: "absent" };

	const rand = deps.random ?? Math.random;
	const totalAttempts = BACKOFFS_MS.length + 1;

	for (let attempt = 0; attempt < totalAttempts; attempt++) {
		if (signal?.aborted) return { status: "absent" };

		const release = await deps.acquire(proxyUrl);
		try {
			const res = await deps.fetch(proxyUrl, { signal });
			if (res.ok) {
				// Read the body and drop it. The bytes are not wanted here — only
				// the status was — but the browser commits a response to its HTTP
				// cache only once the body has been consumed, and that cache entry
				// is what makes the <img src={proxyUrl}> the caller renders next a
				// cache hit instead of a second request to the proxy. Deleting
				// this line as dead code doubles the network cost per thumbnail.
				await res.blob();
				return { status: "loaded" };
			}
			// Permanent rejection (403 / 400 / 404 / ...) — no retry.
			if (!RETRYABLE_STATUS.has(res.status)) return { status: "absent" };
			// Retryable status: fall through to backoff.
		} catch {
			if (signal?.aborted) return { status: "absent" };
			// Network error: treat as retryable.
		} finally {
			release();
		}

		const backoff = BACKOFFS_MS[attempt];
		if (backoff === undefined) break; // retries exhausted
		await deps.sleep(backoff + Math.floor(rand() * JITTER_MS));
	}

	return { status: "absent" };
}

/** Default-wired loader used by components. */
export function loadProxyImageDefault(
	proxyUrl: string | undefined | null,
	signal?: AbortSignal,
): Promise<ImageLoadResult> {
	return loadProxyImage(
		proxyUrl,
		{
			fetch: (input, init) => globalThis.fetch(input, init),
			acquire: (url) => imageLoadQueue.acquire(url),
			sleep: (ms) => new Promise((r) => setTimeout(r, ms)),
		},
		signal,
	);
}
