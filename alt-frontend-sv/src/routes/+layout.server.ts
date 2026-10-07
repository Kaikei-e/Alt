import { getUserRole } from "#lib/server/user-role.js";
import type { LayoutServerLoad } from "./$types";

export const load: LayoutServerLoad = async ({ locals }) => {
	return {
		user: locals.user,
		userRole: getUserRole(locals.user),
	};
};
