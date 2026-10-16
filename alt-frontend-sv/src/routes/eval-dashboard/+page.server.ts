import { redirect } from "@sveltejs/kit";
import { getUserRole } from "#lib/server/user-role.js";

export const load = async ({ locals }: { locals: App.Locals }) => {
	if (!locals.session || !locals.user) {
		throw redirect(303, "/login?return_to=%2Feval-dashboard");
	}

	if (getUserRole(locals.user) !== "admin") {
		throw redirect(303, "/dashboard");
	}

	return {};
};
