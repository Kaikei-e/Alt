import { json, type RequestHandler } from "@sveltejs/kit";
import { updateFeedReadStatus, verifyCsrfToken } from "$lib/api";

export const POST: RequestHandler = async ({ request, cookies }) => {
	const cookieHeader = request.headers.get("cookie") || "";

	// V-004: CSRF validation for state-changing operations
	const providedCSRF = request.headers.get("X-CSRF-Token");

	if (!verifyCsrfToken(cookies, providedCSRF)) {
		return json({ error: "CSRF validation failed" }, { status: 403 });
	}

	try {
		const body = await request.json();
		const feedUrl = body.feed_url;

		if (!feedUrl) {
			return json({ error: "feed_url is required" }, { status: 400 });
		}

		await updateFeedReadStatus(cookieHeader, feedUrl);

		return json({ success: true });
	} catch (error) {
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Error in /api/v1/feeds/read:", errorMessage);
		return json(
			{ error: "Failed to update feed read status" },
			{ status: 500 },
		);
	}
};
