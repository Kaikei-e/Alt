/**
 * Connect-RPC proxy route for /api/v2/[...path]
 *
 * This route forwards Connect-RPC requests from the browser to the backend.
 * It handles authentication by getting the backend token from auth-hub.
 */

import type { RequestHandler } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";
import { getBackendToken } from "$lib/api";

const BACKEND_CONNECT_URL =
	env.BACKEND_CONNECT_URL || "http://alt-backend:9101";

/**
 * Fallback handler for all HTTP methods (GET, POST, etc.)
 * Connect-RPC primarily uses POST requests.
 */
export const fallback: RequestHandler = async ({ request, params, cookies }) => {
	const cookieHeader = request.headers.get("cookie");
	const token = await getBackendToken(cookieHeader);

	if (!token) {
		return new Response(
			JSON.stringify({
				code: "unauthenticated",
				message: "Authentication required",
			}),
			{
				status: 401,
				headers: {
					"Content-Type": "application/json",
				},
			},
		);
	}

	// Construct the backend URL
	const path = params.path || "";
	const backendUrl = `${BACKEND_CONNECT_URL}/${path}`;

	// Clone headers and add authentication
	const headers = new Headers(request.headers);
	headers.set("X-Alt-Backend-Token", token);
	// Remove cookie header as it's not needed for backend
	headers.delete("cookie");
	// Remove host header to avoid issues
	headers.delete("host");

	try {
		// Forward the request to the backend
		const response = await fetch(backendUrl, {
			method: request.method,
			headers,
			body: request.body,
			// @ts-expect-error - duplex is needed for streaming request bodies
			duplex: "half",
		});

		// Create response headers, preserving content-type for Connect-RPC
		const responseHeaders = new Headers();
		const contentType = response.headers.get("content-type");
		if (contentType) {
			responseHeaders.set("content-type", contentType);
		}

		// Copy other relevant headers
		for (const [key, value] of response.headers.entries()) {
			// Skip headers that shouldn't be forwarded
			if (
				!["content-encoding", "transfer-encoding", "content-length"].includes(
					key.toLowerCase(),
				)
			) {
				responseHeaders.set(key, value);
			}
		}

		return new Response(response.body, {
			status: response.status,
			statusText: response.statusText,
			headers: responseHeaders,
		});
	} catch (error) {
		console.error("Connect-RPC proxy error:", error);
		return new Response(
			JSON.stringify({
				code: "internal",
				message: "Failed to connect to backend",
			}),
			{
				status: 500,
				headers: {
					"Content-Type": "application/json",
				},
			},
		);
	}
};
