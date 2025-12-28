import { json, type RequestHandler } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";
import { getBackendToken } from "$lib/api";

const BACKEND_BASE_URL = env.BACKEND_BASE_URL || "http://alt-backend:9000";

export const GET: RequestHandler = async ({ request }) => {
	const cookieHeader = request.headers.get("cookie") || "";
	const token = await getBackendToken(cookieHeader);

	// Build backend endpoint
	const backendEndpoint = `${BACKEND_BASE_URL}/v1/feeds/stats`;

	try {
		const headers: HeadersInit = {
			"Content-Type": "application/json",
		};

		if (token) {
			headers["X-Alt-Backend-Token"] = token;
		}

		console.log(`[API Proxy] Fetching stats from ${backendEndpoint}`);

		const response = await fetch(backendEndpoint, {
			method: "GET",
			headers,
			cache: "no-store",
		});

		if (!response.ok) {
			const errorText = await response.text().catch(() => "");
			console.error("Backend API error:", {
				status: response.status,
				statusText: response.statusText,
				errorBody: errorText.substring(0, 200),
			});
			return json(
				{ error: `Backend API error: ${response.status}` },
				{ status: response.status },
			);
		}

		// Forward specific headers if needed, mainly Cache-Control
		const responseHeaders: ResponseInit["headers"] = {};
		const cacheControl = response.headers.get("Cache-Control");
		if (cacheControl) {
			responseHeaders["Cache-Control"] = cacheControl;
		}

		const data = await response.json();
		return json(data, { headers: responseHeaders });
	} catch (error) {
		console.error("Error in /api/v1/feeds/stats:", error);
		return json({ error: "Internal server error" }, { status: 500 });
	}
};
