import { redirect, type RequestHandler } from "@sveltejs/kit";
import { ory } from "$lib/ory";

export const POST: RequestHandler = async ({ request, locals }) => {
	if (!locals.session) {
		throw redirect(303, "/login");
	}

	try {
		// Create logout flow
		const { data } = await ory.createBrowserLogoutFlow({
			cookie: request.headers.get("cookie") || undefined,
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
