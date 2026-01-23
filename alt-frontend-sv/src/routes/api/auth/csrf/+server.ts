import { json, type RequestHandler } from "@sveltejs/kit";
import { getCSRFToken } from "$lib/api";

/**
 * GET /sv/api/auth/csrf
 * Returns CSRF token for authenticated users
 * V-004: CSRF protection support
 */
export const GET: RequestHandler = async ({ request }) => {
	const cookieHeader = request.headers.get("cookie") || "";

	const csrfToken = await getCSRFToken(cookieHeader);

	if (!csrfToken) {
		return json({ error: "Not authenticated" }, { status: 401 });
	}

	return json({ csrf_token: csrfToken });
};
