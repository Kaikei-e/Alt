import type { RequestHandler } from "@sveltejs/kit";
import { getBackendToken, verifyCsrfToken } from "#lib/api.js";
import { getUserRole } from "#lib/server/user-role.js";
import { RECAP_WORKER_BASE_URL } from "$app/env/private";

const RECAP_WORKER_URL = RECAP_WORKER_BASE_URL || "http://recap-worker:9005";
const FETCH_TIMEOUT_MS = 10_000;

export const POST: RequestHandler = async ({ request, cookies, locals }) => {
	if (!locals?.session || !locals?.backendToken) {
		return Response.json({ error: "Unauthorized" }, { status: 401 });
	}
	if (locals.user?.traits && getUserRole(locals.user) !== "admin") {
		return Response.json({ error: "Forbidden" }, { status: 403 });
	}

	const cookieHeader = request.headers.get("cookie") || "";

	// V-004: CSRF validation for state-changing operations
	const providedCSRF = request.headers.get("X-CSRF-Token");
	if (!verifyCsrfToken(cookies, providedCSRF)) {
		return Response.json({ error: "CSRF validation failed" }, { status: 403 });
	}

	try {
		const token =
			locals.backendToken ||
			(await getBackendToken(cookieHeader).catch((e) => {
				console.error("Error getting backend token:", e);
				return null;
			}));
		if (!token) {
			return Response.json({ error: "Unauthorized" }, { status: 401 });
		}

		const body = await request.json().catch(() => ({}));

		const headers: HeadersInit = {
			"Content-Type": "application/json",
			"X-Alt-Backend-Token": token,
		};

		const response = await fetch(
			`${RECAP_WORKER_URL}/v1/generate/recaps/7days`,
			{
				method: "POST",
				headers,
				body: JSON.stringify(body),
				signal: AbortSignal.timeout(FETCH_TIMEOUT_MS),
			},
		);

		if (!response.ok) {
			const errorText = await response.text().catch(() => "");
			console.error("Recap Worker API error:", {
				status: response.status,
				statusText: response.statusText,
				errorBody: errorText.substring(0, 200),
			});
			return Response.json(
				{ error: `Recap Worker API error: ${response.status}` },
				{ status: response.status },
			);
		}

		const data = await response.json();
		return Response.json(data);
	} catch (error) {
		console.error("Error in /api/v1/generate/recaps/7days:", error);
		return Response.json({ error: "Internal server error" }, { status: 500 });
	}
};
