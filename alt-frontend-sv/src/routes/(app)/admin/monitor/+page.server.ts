import { redirect } from "@sveltejs/kit";
import { getUserRole } from "$lib/server/user-role";

export const load = async ({ locals }) => {
	if (getUserRole(locals.user) !== "admin") {
		throw redirect(303, "/dashboard");
	}

	// The Connect-RPC stream is established client-side; the server-side load
	// only enforces the admin guard. SSR-prefetching Catalog + Snapshot here
	// would duplicate the work the hook does on mount and the marginal first-paint
	// gain does not outweigh the extra round-trip on cold-cache navigations.
	return {};
};
