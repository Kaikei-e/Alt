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
import { isConnectMethodName } from "#lib/server/connect-path.js";
import { BFF_CONNECT_URL } from "$app/env/private";

const BFF_URL = BFF_CONNECT_URL || "http://alt-butterfly-facade:9250";

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

	const origin = request.headers.get("origin");
	if (origin) {
		try {
			const reqOrigin = new URL(request.url).origin;
			if (origin !== reqOrigin) {
				return new Response(
					JSON.stringify({
						code: "permission_denied",
						message: "Origin mismatch",
					}),
					{ status: 403, headers: { "Content-Type": "application/json" } },
				);
			}
		} catch {
			return new Response(
				JSON.stringify({
					code: "permission_denied",
					message: "Invalid origin",
				}),
				{ status: 403, headers: { "Content-Type": "application/json" } },
			);
		}
	}

	const contentLength = Number(request.headers.get("content-length") || "0");
	if (contentLength > 2 * 1024 * 1024) {
		return new Response(
			JSON.stringify({
				code: "resource_exhausted",
				message: "Request body too large",
			}),
			{ status: 413, headers: { "Content-Type": "application/json" } },
		);
	}

	const target = `${BFF_URL}${SERVICE_PREFIX}${method}`;

	const ALLOWED_FORWARD_HEADERS = [
		"content-type",
		"connect-protocol-version",
		"connect-timeout-ms",
		"connect-accept-encoding",
		"connect-content-encoding",
		"accept",
		"user-agent",
		"x-request-id",
	];
	const headers = new Headers();
	for (const name of ALLOWED_FORWARD_HEADERS) {
		const value = request.headers.get(name);
		if (value !== null) {
			headers.set(name, value);
		}
	}
	headers.set("X-Alt-Backend-Token", token);

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
