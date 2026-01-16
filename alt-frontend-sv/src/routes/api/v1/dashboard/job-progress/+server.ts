import { json, type RequestHandler } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";
import { getBackendToken } from "$lib/api";

const RECAP_WORKER_URL =
	env.RECAP_WORKER_BASE_URL || "http://recap-worker:9005";

export const GET: RequestHandler = async ({ request, url }) => {
	try {
		const cookieHeader = request.headers.get("cookie") || "";
		const token = await getBackendToken(cookieHeader).catch((e) => {
			console.error("Error getting backend token:", e);
			return null;
		});

		const userId = url.searchParams.get("user_id");
		const windowSeconds = url.searchParams.get("window");
		const limit = url.searchParams.get("limit");

		const params = new URLSearchParams();
		if (userId) {
			params.set("user_id", userId);
		}
		if (windowSeconds) {
			params.set("window", windowSeconds);
		}
		if (limit) {
			params.set("limit", limit);
		}

		const queryString = params.toString();
		const backendEndpoint = `${RECAP_WORKER_URL}/v1/dashboard/job-progress${
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
			console.error("Recap Worker API error:", {
				status: response.status,
				statusText: response.statusText,
				errorBody: errorText.substring(0, 200),
			});
			return json(
				{ error: `Recap Worker API error: ${response.status}` },
				{ status: response.status },
			);
		}

		const data = await response.json();
		return json(data);
	} catch (error) {
		console.error("Error in /api/v1/dashboard/job-progress:", error);
		return json({ error: "Internal server error" }, { status: 500 });
	}
};
