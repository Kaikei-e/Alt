import type { LayoutServerLoad } from "./$types";
import { getUserRole } from "$lib/server/user-role";

export const load: LayoutServerLoad = async ({ locals }) => {
	return {
		user: locals.user,
		userRole: getUserRole(locals.user),
	};
};
