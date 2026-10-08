import type { RequestHandler } from "@sveltejs/kit";
import { getBackendToken } from "#lib/api.js";
import { BACKEND_CONNECT_URL } from "$app/env/private";

const BACKEND_URL = BACKEND_CONNECT_URL || "http://alt-butterfly-facade:9250";
const FETCH_TIMEOUT_MS = 10_000;

export const GET: RequestHandler = async ({ request, url }) => {
	const cookieHeader = request.headers.get("cookie") || "";
	const token = await getBackendToken(cookieHeader);

	const window = url.searchParams.get("window") || "24h";

	// Validate window parameter
	const validWindows = ["4h", "24h", "3d", "7d"];
	if (!validWindows.includes(window)) {
		return Response.json(
			{ error: "Invalid window parameter. Valid values: 4h, 24h, 3d, 7d" },
			{ status: 400 },
		);
	}

	const backendEndpoint = `${BACKEND_URL}/v1/feeds/stats/trends?window=${window}`;

	try {
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
			console.error("Backend API error:", {
				status: response.status,
				statusText: response.statusText,
				errorBody: errorText.substring(0, 200),
			});
			return Response.json(
				{ error: `Backend API error: ${response.status}` },
				{ status: response.status },
			);
		}

		const data = await response.json();
		return Response.json(data);
	} catch (error) {
		console.error("Error in /api/v1/feeds/stats/trends:", error);
		return Response.json({ error: "Internal server error" }, { status: 500 });
	}
};
