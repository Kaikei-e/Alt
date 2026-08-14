/**
 * Admin Observability Connect-RPC proxy
 *
 * Routes Admin Monitor calls through the BFF (alt-butterfly-facade) rather than
 * direct to alt-backend, honouring the project rule that the frontend never
 * talks to the backend directly. The BFF performs the admin role check and
 * injects the service token before forwarding upstream.
 *
 * Streaming (Watch) is supported: the body is piped through as-is so chunks
 * flush end-to-end (nginx has matching proxy_buffering off for this path).
 */

import type { RequestHandler } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";
import { isConnectMethodName } from "$lib/server/connect-path";

const BFF_URL = env.BFF_CONNECT_URL || "http://alt-butterfly-facade:9250";

const SERVICE_PREFIX = "/alt.admin_monitor.v1.AdminMonitorService/";

export const fallback: RequestHandler = async ({ request, params, locals }) => {
	const method = params.path || "";

	// The prefix above is the only thing scoping this proxy to the admin
	// service, and a prefix is just a string until the method appended to it is
	// known to be a single proto identifier. Without this check an authenticated
	// non-admin can send `..%2F..%2Fv1%2Faggregate` and have `fetch` normalise
	// its way onto any BFF endpoint with their token attached — the BFF's own
	// role check is the only thing left, which makes this a guard bypass rather
	// than mere defence in depth.
	if (!isConnectMethodName(method)) {
		console.warn(
			JSON.stringify({
				level: "warn",
				source: "admin-monitor-proxy",
				msg: "rejected malformed Connect method name",
				method,
			}),
		);
		return new Response(
			JSON.stringify({ code: "not_found", message: "Unknown endpoint" }),
			{ status: 404, headers: { "Content-Type": "application/json" } },
		);
	}

	const token = locals.backendToken;
	if (!token) {
		return new Response(
			JSON.stringify({
				code: "unauthenticated",
				message: "Authentication required",
			}),
			{ status: 401, headers: { "Content-Type": "application/json" } },
		);
	}

	const target = `${BFF_URL}${SERVICE_PREFIX}${method}`;

	const headers = new Headers(request.headers);
	headers.set("X-Alt-Backend-Token", token);
	headers.delete("cookie");
	headers.delete("host");
	// Node fetch handles decompression transparently.
	headers.delete("accept-encoding");

	try {
		const upstream = await fetch(target, {
			method: request.method,
			headers,
			body: request.body,
			// @ts-expect-error duplex is required for streaming request bodies
			duplex: "half",
			signal: request.signal,
		});

		const responseHeaders = new Headers();
		for (const [key, value] of upstream.headers.entries()) {
			if (
				!["content-encoding", "transfer-encoding", "content-length"].includes(
					key.toLowerCase(),
				)
			) {
				responseHeaders.set(key, value);
			}
		}
		// Defeat intermediate buffering even if the upstream forgot to set it.
		responseHeaders.set("X-Accel-Buffering", "no");
		responseHeaders.set("Cache-Control", "no-cache, no-transform");

		return new Response(upstream.body, {
			status: upstream.status,
			statusText: upstream.statusText,
			headers: responseHeaders,
		});
	} catch (error) {
		console.error("[admin-monitor-proxy] upstream error", error);
		return new Response(
			JSON.stringify({
				code: "internal",
				message: "Admin monitor proxy failed",
			}),
			{ status: 502, headers: { "Content-Type": "application/json" } },
		);
	}
};
