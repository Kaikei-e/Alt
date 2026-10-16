import type { RequestHandler } from "@sveltejs/kit";
import { getCSRFToken, issueCsrfCookie } from "#lib/api.js";

/**
 * GET /api/auth/csrf
 * Returns CSRF token for authenticated users, and mirrors it into an
 * httpOnly cookie so guarded routes can validate it via double-submit
 * comparison instead of re-fetching a fresh token from auth-hub.
 * V-004: CSRF protection support
 */
export const GET: RequestHandler = async ({ request, cookies, url }) => {
	const cookieHeader = request.headers.get("cookie") || "";

	const csrfToken = await getCSRFToken(cookieHeader);

	if (!csrfToken) {
		return Response.json({ error: "Not authenticated" }, { status: 401 });
	}

	const requestUrl = url ?? new URL(request.url);
	issueCsrfCookie(cookies, csrfToken, {
		secure: requestUrl.protocol === "https:",
	});

	return Response.json({ csrf_token: csrfToken });
};
