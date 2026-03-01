/**
 * Server-side Connect-RPC Transport for alt-frontend-sv
 *
 * This module provides transport configuration for server-side Connect-RPC calls.
 * It's used in server routes (+server.ts) to call the backend directly.
 *
 * WARNING: This module imports $env/dynamic/private and MUST NOT be imported
 * in browser code. Use transport-client.ts for browser-side code instead.
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
 * Creates a server-side transport using a pre-fetched backend token.
 * Avoids redundant auth-hub /session calls when the token is already
 * available (e.g. from hooks.server.ts locals.backendToken).
 *
 * @param backendToken - The backend token from locals.backendToken
 * @returns A configured Connect transport
 */
export function createServerTransportWithToken(
	backendToken: string,
): Transport {
	return createConnectTransport({
		baseUrl: BACKEND_CONNECT_URL,
		interceptors: [
			(next) => async (req) => {
				req.header.set("X-Alt-Backend-Token", backendToken);
				return next(req);
			},
		],
	});
}
