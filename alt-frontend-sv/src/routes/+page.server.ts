import { redirect, type ServerLoad } from "@sveltejs/kit";

/**
 * Knowledge Loop is the primary surface for authenticated users.
 *
 * - First-time visitors (no `alt_loop_welcomed` cookie) → `/loop/welcome`
 * - Returning users → `/loop` directly
 * - Unauthenticated visitors fall through to the landing page (`+page.svelte`)
 *
 * Server-side redirect avoids the flash of the landing UI for authenticated
 * sessions and keeps the entry point a single hop.
 */
export const load: ServerLoad = async ({ locals, cookies }) => {
	if (!locals.backendToken) {
		return {};
	}

	const welcomed = cookies.get("alt_loop_welcomed") === "true";
	if (welcomed) {
		throw redirect(303, "/loop");
	}
	throw redirect(303, "/loop/welcome");
};
