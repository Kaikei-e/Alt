/**
 * Connect-RPC Transport module for alt-frontend-sv
 *
 * Provides transport configuration for server-side and client-side Connect-RPC calls.
 */

import { createConnectTransport } from "@connectrpc/connect-web";
import type { Transport } from "@connectrpc/connect";
import { env } from "$env/dynamic/private";
import { getBackendToken } from "$lib/api";

// Connect-RPC server URL (server-side internal URL)
const BACKEND_CONNECT_URL =
	env.BACKEND_CONNECT_URL || "http://alt-backend:9101";

/**
 * Creates a server-side transport for Connect-RPC calls.
 * This transport is used in server routes (+server.ts) to call the backend directly.
 *
 * @param cookie - The cookie header from the incoming request
 * @returns A configured Connect transport
 */
export async function createServerTransport(
	cookie: string | null,
): Promise<Transport> {
	const backendToken = await getBackendToken(cookie);

	return createConnectTransport({
		baseUrl: BACKEND_CONNECT_URL,
		interceptors: [
			(next) => async (req) => {
				if (backendToken) {
					req.header.set("X-Alt-Backend-Token", backendToken);
				}
				return next(req);
			},
		],
	});
}

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
