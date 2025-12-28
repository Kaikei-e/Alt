import { json, type RequestHandler } from "@sveltejs/kit";
import { getBackendToken } from "$lib/api";
import { env } from "$env/dynamic/private";

const BACKEND_BASE_URL = env.BACKEND_BASE_URL || "http://alt-backend:9000";

export const GET: RequestHandler = async ({ request }) => {
	const cookieHeader = request.headers.get("cookie") || "";
	const token = await getBackendToken(cookieHeader);

	// Build backend endpoint
	const backendEndpoint = `${BACKEND_BASE_URL}/v1/feeds/count/unreads`;

	try {
		const headers: HeadersInit = {
			"Content-Type": "application/json",
		};

		if (token) {
			headers["X-Alt-Backend-Token"] = token;
		}

		console.log(`[API Proxy] Fetching unread count from ${backendEndpoint}`);

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

		const data = await response.json();
		return json(data);
	} catch (error) {
		console.error("Error in /api/v1/feeds/count/unreads:", error);
		return json({ error: "Internal server error" }, { status: 500 });
	}
};
