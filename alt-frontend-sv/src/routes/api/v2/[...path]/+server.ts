/**
 * Connect-RPC proxy route for /api/v2/[...path]
 *
 * This route forwards Connect-RPC requests from the browser to the backend.
 * Authentication is handled by hooks.server.ts which caches the token in locals.
 */

import type { RequestHandler } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";

const BACKEND_CONNECT_URL =
	env.BACKEND_CONNECT_URL || "http://alt-backend:9101";

/**
 * Fallback handler for all HTTP methods (GET, POST, etc.)
 * Connect-RPC primarily uses POST requests.
 *
 * Note: Backend token is cached in event.locals by hooks.server.ts (TTFT optimization).
 */
export const fallback: RequestHandler = async ({ request, params, locals }) => {
	console.log("[Connect-RPC Proxy] Request received:", {
		method: request.method,
		path: params.path,
		url: request.url,
	});

	// Use cached token from hooks.server.ts (request-scoped caching)
	const token = locals.backendToken;

	if (!token) {
		console.error(
			"[Connect-RPC Proxy] Authentication failed: no backend token",
		);
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
	console.log("[Connect-RPC Proxy] Forwarding to:", backendUrl);

	// Clone headers and add authentication
	const headers = new Headers(request.headers);
	headers.set("X-Alt-Backend-Token", token);
	// Remove cookie header as it's not needed for backend
	headers.delete("cookie");
	// Remove host header to avoid issues
	headers.delete("host");
	// Remove Accept-Encoding to prevent backend compression
	// Node.js fetch auto-decompresses but forwards Content-Encoding header,
	// causing browser to attempt double decompression
	// See: https://github.com/sveltejs/kit/issues/12197
	headers.delete("accept-encoding");

	try {
		// Forward the request to the backend
		const response = await fetch(backendUrl, {
			method: request.method,
			headers,
			body: request.body,
			// @ts-expect-error - duplex is needed for streaming request bodies
			duplex: "half",
		});

		console.log("[Connect-RPC Proxy] Backend response:", {
			status: response.status,
			statusText: response.statusText,
			contentType: response.headers.get("content-type"),
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
			// content-encoding must be excluded because Node.js fetch auto-decompresses
			// but the header remains, causing browser to attempt double decompression
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
		console.error("[Connect-RPC Proxy] Error:", error);
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
