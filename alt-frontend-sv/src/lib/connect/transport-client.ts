/**
 * Client-side Connect-RPC Transport for alt-frontend-sv
 *
 * This module provides transport configuration for browser-side Connect-RPC calls.
 * It routes through the SvelteKit API proxy at /sv/api/v2.
 *
 * IMPORTANT: This module is safe to import in browser code.
 */

import { createConnectTransport } from "@connectrpc/connect-web";
import type { Transport } from "@connectrpc/connect";
import { base } from "$app/paths";

/**
 * Fetch priority a call site is asking the browser for.
 *
 * Mirrors the Fetch standard's `RequestInit.priority`. `"high"` is for a
 * request the reader is waiting on; `"low"` is for a lookahead or a warm,
 * which must yield the connection to anything the reader can see.
 */
export type AltFetchPriority = "high" | "low" | "auto";

/**
 * In-process sentinel header carrying a fetch priority from the call site down
 * to this module's custom `fetch`. Stripped here; it never goes on the wire.
 *
 * A header is the only channel that survives the trip. `CallOptions` in
 * `@connectrpc/connect` is exactly `{ timeoutMs, headers, signal, onHeader,
 * onTrailer, contextValues }` — there is no slot for a fetch hint — and
 * `@connectrpc/connect-web` builds the `RequestInit` itself
 * (`connect-transport.js`: `fetch(req.url, { ...fetchOptions, method, headers,
 * signal, body })`, where `fetchOptions` is a module-level `{ redirect:
 * "error" }`). Every caller-supplied key other than the four it names is
 * dropped before our `fetch` is reached, so `priority` cannot be passed as a
 * call option and has to arrive as a header we take back off.
 */
export const FETCH_PRIORITY_HEADER = "x-alt-fetch-priority";

/**
 * Call options fragment asking for a fetch priority.
 *
 * @example
 * client.fetchArticleContent({ url }, { headers: fetchPriorityHeaders("low") })
 */
export function fetchPriorityHeaders(
	priority: AltFetchPriority,
): Record<string, string> {
	return { [FETCH_PRIORITY_HEADER]: priority };
}

function isFetchPriority(value: string): value is AltFetchPriority {
	return value === "high" || value === "low" || value === "auto";
}

/**
 * Memoized answer to "does this engine implement `RequestInit.priority`?".
 * Null until the first request, so the probe runs in the environment that will
 * actually issue the fetch rather than at import time.
 */
let requestPrioritySupport: boolean | null = null;

/**
 * Whether the engine reads `priority` out of a `RequestInit`.
 *
 * WebIDL dictionary conversion reads every member the implementation knows
 * about, so a getter that fires is the presence test. `RequestInit.priority`
 * is Baseline (Chrome 101 / Firefox 132 / Safari 17.2) and older engines
 * simply ignore unknown init keys, so this is not a polyfill gate — it just
 * keeps us from putting a key on the object that nothing will read.
 */
function supportsRequestPriority(): boolean {
	if (requestPrioritySupport !== null) return requestPrioritySupport;

	requestPrioritySupport = false;
	try {
		if (typeof Request === "undefined") return requestPrioritySupport;

		let observed = false;
		new Request("https://alt.invalid/fetch-priority-probe", {
			get priority(): AltFetchPriority {
				observed = true;
				return "auto";
			},
		} as RequestInit);
		requestPrioritySupport = observed;
	} catch {
		requestPrioritySupport = false;
	}

	return requestPrioritySupport;
}

/**
 * Build the `RequestInit` the browser actually sees: credentials attached, the
 * priority sentinel consumed.
 *
 * Requests without the sentinel keep their original `headers` object
 * untouched — rebuilding `Headers` for every call would be pure allocation on
 * the common path.
 */
function withTransportInit(init: RequestInit | undefined): RequestInit {
	const next: RequestInit = { ...init, credentials: "include" };

	const headers = new Headers(init?.headers ?? undefined);
	const requested = headers.get(FETCH_PRIORITY_HEADER);
	if (requested === null) return next;

	// Unconditional, and before the support check: an engine that ignores
	// `priority` is exactly the one that would otherwise ship the sentinel to
	// the proxy, which forwards headers verbatim to alt-backend.
	headers.delete(FETCH_PRIORITY_HEADER);
	next.headers = headers;

	if (isFetchPriority(requested) && supportsRequestPriority()) {
		next.priority = requested;
	}

	return next;
}

/**
 * Cached client transport (singleton pattern for TTFT optimization).
 * Reusing the transport avoids HTTP connection setup overhead on each request.
 */
let cachedTransport: Transport | null = null;

/**
 * Creates or returns a cached client-side transport for Connect-RPC calls.
 * This transport routes through the SvelteKit API proxy at {base}/api/v2.
 *
 * Note: This is used in browser-side code where the proxy handles authentication.
 * The transport is cached (singleton) to avoid connection setup overhead.
 *
 * @returns A configured Connect transport for client-side use
 */
export function createClientTransport(): Transport {
	if (cachedTransport) {
		return cachedTransport;
	}

	cachedTransport = createConnectTransport({
		baseUrl: `${base}/api/v2`,
		// Credentials are handled by the proxy
		fetch: (input, init) => fetch(input, withTransportInit(init)),
	});

	return cachedTransport;
}
