import type { RequestHandler } from "@sveltejs/kit";
import { getBackendToken, verifyCsrfToken } from "#lib/api.js";
import { RECAP_WORKER_BASE_URL } from "$app/env/private";

const RECAP_WORKER_URL = RECAP_WORKER_BASE_URL || "http://recap-worker:9005";
const FETCH_TIMEOUT_MS = 10_000;

export const POST: RequestHandler = async ({ request, cookies }) => {
	const cookieHeader = request.headers.get("cookie") || "";

	// V-004: CSRF validation for state-changing operations
	const providedCSRF = request.headers.get("X-CSRF-Token");
	if (!verifyCsrfToken(cookies, providedCSRF)) {
		return Response.json({ error: "CSRF validation failed" }, { status: 403 });
	}

	try {
		const token = await getBackendToken(cookieHeader).catch((e) => {
			console.error("Error getting backend token:", e);
			return null;
		});

		const body = await request.json().catch(() => ({}));

		const headers: HeadersInit = {
			"Content-Type": "application/json",
		};

		if (token) {
			headers["X-Alt-Backend-Token"] = token;
		}

		const response = await fetch(
			`${RECAP_WORKER_URL}/v1/generate/topic-cards`,
			{
				method: "POST",
				headers,
				body: JSON.stringify(body),
				signal: AbortSignal.timeout(FETCH_TIMEOUT_MS),
			},
		);

		if (!response.ok) {
			let errorMsg = `Recap Worker API error: ${response.status}`;
			try {
				const errorData = await response.json();
				if (
					errorData &&
					typeof errorData === "object" &&
					"error" in errorData &&
					typeof errorData.error === "string"
				) {
					errorMsg = errorData.error;
				}
			} catch {
				// empty
			}
			console.error("Recap Worker API error:", {
				status: response.status,
				statusText: response.statusText,
				error: errorMsg,
			});
			return Response.json({ error: errorMsg }, { status: response.status });
		}

		const data = await response.json();
		return Response.json(data, { status: response.status });
	} catch (error) {
		console.error("Error in /api/v1/generate/topic-cards:", error);
		return Response.json({ error: "Internal server error" }, { status: 500 });
	}
};
