import { type RequestHandler, redirect } from "@sveltejs/kit";
import { ory } from "$lib/ory";
import { invalidateSessionCache } from "$lib/server/auth-middleware";

export const POST: RequestHandler = async ({ request, locals }) => {
	if (!locals.session) {
		throw redirect(303, "/login");
	}

	const cookieHeader = request.headers.get("cookie");
	// Bust the short-term session cache first so the cookie can no longer
	// authenticate via a stale cache hit even if the logout flow below fails.
	if (cookieHeader) {
		invalidateSessionCache(cookieHeader);
	}

	try {
		// Create logout flow
		const { data } = await ory.createBrowserLogoutFlow({
			cookie: cookieHeader || undefined,
		});

		// Redirect to logout URL
		throw redirect(303, data.logout_url);
	} catch (error) {
		// If redirect was thrown, rethrow it
		if (
			error &&
			typeof error === "object" &&
			"status" in error &&
			"location" in error
		) {
			throw error;
		}

		// Otherwise, redirect to login
		throw redirect(303, "/login");
	}
};
