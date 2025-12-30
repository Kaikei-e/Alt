/**
 * Connect-RPC Transport for client-side (browser) use only
 *
 * This module is safe to import in browser code.
 */

import { createConnectTransport } from "@connectrpc/connect-web";
import type { Transport } from "@connectrpc/connect";

/**
 * Creates a client-side transport for Connect-RPC calls.
 * This transport routes through the SvelteKit API proxy at /api/v2.
 *
 * Note: This is used in browser-side code where the proxy handles authentication.
 *
 * @returns A configured Connect transport for client-side use
 */
export function createClientTransport(): Transport {
	return createConnectTransport({
		baseUrl: "/api/v2",
		// Credentials are handled by the proxy
		fetch: (input, init) =>
			fetch(input, {
				...init,
				credentials: "include",
			}),
	});
}
