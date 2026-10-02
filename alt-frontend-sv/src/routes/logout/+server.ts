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
		try {
			await invalidateSessionCache(cookieHeader);
		} catch (error) {
			// A01 revised: Even if auth-hub cache invalidation fails, we must
			// continue to revoke the Kratos session. The cache bounds are 60s/5m.
			// Returning 500 here would trap the user with a permanent Kratos session.
			console.error("auth-hub session cache invalidation failed during logout", { error: error instanceof Error ? error.message : "Unknown error" });
		}
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

		console.error("kratos browser logout flow creation failed", { error: error instanceof Error ? error.message : "Unknown error" });
		return new Response("Internal Server Error: Failed to create logout flow", { status: 500 });
	}
};
