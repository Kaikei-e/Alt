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
		fetch: (input, init) =>
			fetch(input, {
				...init,
				credentials: "include",
			}),
	});

	return cachedTransport;
}
