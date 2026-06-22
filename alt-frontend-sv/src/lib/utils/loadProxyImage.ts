/**
 * Status-aware loader for OG image proxy URLs.
 *
 * Fetches the proxied image through the per-host queue so it can read the HTTP
 * status: a transient rate-limit (429) or upstream blip (502/503/504) is retried
 * with backoff, while a permanent rejection (403/400/404) resolves to `absent`
 * immediately. This is what stops a transient failure from pinning the card to
 * the fallback gradient — the bug behind the mark-as-read regression.
 *
 * Loading via fetch() + an object URL (rather than a bare <img src>) is what
 * gives us the status; the response is still served from the proxy's immutable
 * HTTP cache on revisits, so no extra network cost is paid.
 */

import { imageLoadQueue } from "./imageLoadQueue";

export type ImageLoadResult =
	| { status: "loaded"; objectUrl: string }
	| { status: "absent" };

export interface LoadProxyImageDeps {
	fetch: typeof fetch;
	acquire: (proxyUrl: string) => Promise<() => void>;
	sleep: (ms: number) => Promise<void>;
	createObjectURL: (blob: Blob) => string;
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
				const blob = await res.blob();
				return { status: "loaded", objectUrl: deps.createObjectURL(blob) };
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
			createObjectURL: (blob) => URL.createObjectURL(blob),
		},
		signal,
	);
}
