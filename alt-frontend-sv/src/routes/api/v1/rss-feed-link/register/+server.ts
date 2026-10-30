import type { RequestHandler } from "@sveltejs/kit";
import { registerRssFeed, verifyCsrfToken } from "#lib/api.js";
import {
	SSRFValidationError,
	validateUrlForSSRF,
} from "#lib/server/ssrf-validator.js";

export const POST: RequestHandler = async ({ request, cookies }) => {
	const cookieHeader = request.headers.get("cookie") || "";

	// V-004: CSRF validation for state-changing operations
	const providedCSRF = request.headers.get("X-CSRF-Token");

	if (!verifyCsrfToken(cookies, providedCSRF)) {
		return Response.json({ error: "CSRF validation failed" }, { status: 403 });
	}

	try {
		const body = await request.json().catch(() => null);
		const url = body?.url;

		if (!url || typeof url !== "string") {
			return Response.json({ error: "url is required" }, { status: 400 });
		}

		try {
			validateUrlForSSRF(url);
		} catch (err) {
			if (err instanceof SSRFValidationError) {
				return Response.json({ error: err.message }, { status: 400 });
			}
			throw err;
		}

		await registerRssFeed(cookieHeader, url);

		return Response.json({ success: true });
	} catch (error) {
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Error in /api/v1/rss-feed-link/register:", errorMessage);
		return Response.json(
			{ error: "Failed to register RSS feed" },
			{ status: 500 },
		);
	}
};
