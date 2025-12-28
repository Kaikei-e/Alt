import { json, type RequestHandler } from "@sveltejs/kit";
import { getBackendToken } from "$lib/api";
import { env } from "$env/dynamic/private";

const BACKEND_BASE_URL = env.BACKEND_BASE_URL || "http://alt-backend:9000";

export const GET: RequestHandler = async ({ request, url }) => {
	try {
		const cookieHeader = request.headers.get("cookie") || "";
		const token = await getBackendToken(cookieHeader).catch((e) => {
			console.error("Error getting backend token:", e);
			return null;
		});

		const windowSeconds = url.searchParams.get("window");
		const limit = url.searchParams.get("limit");

		const params = new URLSearchParams();
		if (windowSeconds) {
			params.set("window", windowSeconds);
		}
		if (limit) {
			params.set("limit", limit);
		}

		const queryString = params.toString();
		const backendEndpoint = `${BACKEND_BASE_URL}/v1/dashboard/recap_jobs${
			queryString ? `?${queryString}` : ""
		}`;

		const headers: HeadersInit = {
			"Content-Type": "application/json",
		};

		if (token) {
			headers["X-Alt-Backend-Token"] = token;
		}

		const response = await fetch(backendEndpoint, {
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
		console.error("Error in /api/v1/dashboard/recap_jobs:", error);
		return json({ error: "Internal server error" }, { status: 500 });
	}
};
