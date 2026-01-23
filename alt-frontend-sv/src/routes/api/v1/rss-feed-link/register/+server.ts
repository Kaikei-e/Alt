import { json, type RequestHandler } from "@sveltejs/kit";
import { registerRssFeed, getCSRFToken } from "$lib/api";

export const POST: RequestHandler = async ({ request }) => {
	const cookieHeader = request.headers.get("cookie") || "";

	// V-004: CSRF validation for state-changing operations
	const expectedCSRF = await getCSRFToken(cookieHeader);
	const providedCSRF = request.headers.get("X-CSRF-Token");

	if (!expectedCSRF || expectedCSRF !== providedCSRF) {
		return json({ error: "CSRF validation failed" }, { status: 403 });
	}

	try {
		const body = await request.json().catch(() => null);
		const url = body?.url;

		if (!url || typeof url !== "string") {
			return json({ error: "url is required" }, { status: 400 });
		}

		await registerRssFeed(cookieHeader, url);

		return json({ success: true });
	} catch (error) {
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Error in /api/v1/rss-feed-link/register:", errorMessage);
		return json({ error: errorMessage }, { status: 500 });
	}
};
