// Legacy /sv/login route - redirect to /sv/auth/login
// This route is kept for backward compatibility with Kratos redirects
// that may still point to /sv/login before the service is restarted
import { redirect } from "@sveltejs/kit";
import type { PageServerLoad } from "./$types";

export const load: PageServerLoad = async ({ url }) => {
	// Redirect to the new unified auth/login route
	// Preserve flow and return_to parameters if present
	const flow = url.searchParams.get("flow");
	const returnTo = url.searchParams.get("return_to");

	let redirectUrl = "/sv/auth/login";
	const params = new URLSearchParams();
	if (flow) {
		params.set("flow", flow);
	}
	if (returnTo) {
		params.set("return_to", returnTo);
	}
	if (params.toString()) {
		redirectUrl += `?${params.toString()}`;
	}

	throw redirect(303, redirectUrl);
};
