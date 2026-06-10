import { redirect, type ServerLoad } from "@sveltejs/kit";

/**
 * Knowledge Home is the primary surface for authenticated users.
 *
 * - Authenticated users → `/home`
 * - Unauthenticated visitors fall through to the landing page (`+page.svelte`)
 *
 * Server-side redirect avoids the flash of the landing UI for authenticated
 * sessions and keeps the entry point a single hop.
 */
export const load: ServerLoad = async ({ locals }) => {
	if (!locals.backendToken) {
		return {};
	}

	throw redirect(303, "/home");
};
