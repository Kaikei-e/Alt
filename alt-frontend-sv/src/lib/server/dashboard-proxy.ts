/**
 * Shared proxy for dashboard GET endpoints (`/api/v1/dashboard/*`).
 *
 * These routes all do the same thing: resolve a backend token, forward a
 * whitelisted subset of query params, and translate upstream errors into a
 * JSON error response. Centralizing it keeps the timeout/error-shape fix in
 * one place instead of N near-identical copies.
 */
import type { RequestEvent } from "@sveltejs/kit";
import { getBackendToken } from "#lib/api.js";
import { getUserRole } from "#lib/server/user-role.js";
import { BACKEND_CONNECT_URL, BACKEND_REST_URL } from "$app/env/private";

const FETCH_TIMEOUT_MS = 10_000;

export interface ProxyDashboardGetOptions {
	/** Query params to forward from the incoming request, if present. */
	allowedParams?: string[];
	/** Label used in error logs/responses (e.g. "Backend API error"). */
	errorLabel?: string;
	/** Minimal shape check on the parsed upstream body before forwarding it. */
	validateData?: (data: unknown) => boolean;
}

export async function proxyDashboardGet(
	baseUrl: string,
	path: string,
	event: Pick<RequestEvent, "request" | "url"> & {
		locals?: Partial<App.Locals>;
	},
	options: ProxyDashboardGetOptions = {},
): Promise<Response> {
	const allowedParams = options.allowedParams ?? [];
	const errorLabel = options.errorLabel ?? "Backend API error";
	const { request, url, locals } = event;
	if (locals?.user?.traits && getUserRole(locals.user) !== "admin") {
		return Response.json({ error: "Forbidden" }, { status: 403 });
	}
	const effectiveBaseUrl =
		baseUrl ||
		BACKEND_REST_URL ||
		BACKEND_CONNECT_URL ||
		"http://alt-butterfly-facade:9250";

	try {
		const cookieHeader = request.headers.get("cookie") || "";
		const token =
			locals?.backendToken ??
			(await getBackendToken(cookieHeader).catch((e) => {
				console.error("Error getting backend token:", e);
				return null;
			}));

		if (!token) {
			return Response.json({ error: "Unauthorized" }, { status: 401 });
		}

		const params = new URLSearchParams();
		for (const key of allowedParams) {
			const value = url.searchParams.get(key);
			if (value) {
				params.set(key, value);
			}
		}

		const queryString = params.toString();
		const backendEndpoint = `${effectiveBaseUrl}${path}${queryString ? `?${queryString}` : ""}`;

		const headers: HeadersInit = {
			"Content-Type": "application/json",
			"X-Alt-Backend-Token": token,
		};

		const response = await fetch(backendEndpoint, {
			headers,
			cache: "no-store",
			signal: AbortSignal.timeout(FETCH_TIMEOUT_MS),
		});

		if (!response.ok) {
			const errorText = await response.text().catch(() => "");
			console.error(errorLabel, {
				status: response.status,
				statusText: response.statusText,
				errorBody: errorText.substring(0, 200),
			});
			return Response.json(
				{ error: `${errorLabel}: ${response.status}` },
				{ status: response.status },
			);
		}

		const data = await response.json();
		if (options.validateData && !options.validateData(data)) {
			console.error(`${errorLabel}: unexpected response shape`, { path });
			return Response.json(
				{ error: `${errorLabel}: unexpected response shape` },
				{ status: 502 },
			);
		}
		return Response.json(data);
	} catch (error) {
		console.error(`Error in dashboard proxy for ${path}:`, error);
		return Response.json({ error: "Internal server error" }, { status: 500 });
	}
}
