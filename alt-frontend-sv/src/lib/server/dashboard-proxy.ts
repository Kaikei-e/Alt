/**
 * Shared proxy for dashboard GET endpoints (`/api/v1/dashboard/*`).
 *
 * These routes all do the same thing: resolve a backend token, forward a
 * whitelisted subset of query params, and translate upstream errors into a
 * JSON error response. Centralizing it keeps the timeout/error-shape fix in
 * one place instead of N near-identical copies.
 */
import { json, type RequestEvent } from "@sveltejs/kit";
import { getBackendToken } from "$lib/api";

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
	{ request, url }: Pick<RequestEvent, "request" | "url">,
	options: ProxyDashboardGetOptions = {},
): Promise<Response> {
	const allowedParams = options.allowedParams ?? [];
	const errorLabel = options.errorLabel ?? "Backend API error";

	try {
		const cookieHeader = request.headers.get("cookie") || "";
		const token = await getBackendToken(cookieHeader).catch((e) => {
			console.error("Error getting backend token:", e);
			return null;
		});

		const params = new URLSearchParams();
		for (const key of allowedParams) {
			const value = url.searchParams.get(key);
			if (value) {
				params.set(key, value);
			}
		}

		const queryString = params.toString();
		const backendEndpoint = `${baseUrl}${path}${queryString ? `?${queryString}` : ""}`;

		const headers: HeadersInit = {
			"Content-Type": "application/json",
		};
		if (token) {
			headers["X-Alt-Backend-Token"] = token;
		}

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
			return json(
				{ error: `${errorLabel}: ${response.status}` },
				{ status: response.status },
			);
		}

		const data = await response.json();
		if (options.validateData && !options.validateData(data)) {
			console.error(`${errorLabel}: unexpected response shape`, { path });
			return json(
				{ error: `${errorLabel}: unexpected response shape` },
				{ status: 502 },
			);
		}
		return json(data);
	} catch (error) {
		console.error(`Error in dashboard proxy for ${path}:`, error);
		return json({ error: "Internal server error" }, { status: 500 });
	}
}
