import { json, type RequestHandler } from "@sveltejs/kit";
import { deleteFeedLink, verifyCsrfToken } from "$lib/api";

export const DELETE: RequestHandler = async ({ request, params, cookies }) => {
	const cookieHeader = request.headers.get("cookie") || "";
	const id = params.id;

	if (!id) {
		return json({ error: "id is required" }, { status: 400 });
	}

	// V-004: CSRF validation for state-changing operations
	const providedCSRF = request.headers.get("X-CSRF-Token");

	if (!verifyCsrfToken(cookies, providedCSRF)) {
		return json({ error: "CSRF validation failed" }, { status: 403 });
	}

	try {
		await deleteFeedLink(cookieHeader, id);
		return json({ success: true });
	} catch (error) {
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Error in /api/v1/rss-feed-link/[id]:", {
			id,
			error: errorMessage,
		});
		return json({ error: "Failed to delete feed link" }, { status: 500 });
	}
};
