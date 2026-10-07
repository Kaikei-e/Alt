import { redirect } from "@sveltejs/kit";
import { buildLegacySvRedirect } from "#lib/server/legacy-sv.js";
import type { PageServerLoad } from "./$types";

export const load: PageServerLoad = async ({ url }) => {
	throw redirect(303, buildLegacySvRedirect("/auth/login", url.searchParams));
};
