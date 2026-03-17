import { redirect } from "@sveltejs/kit";
import { fetchKnowledgeHomeAdminSnapshot } from "$lib/server/knowledge-home-admin";
import { getUserRole } from "$lib/server/user-role";

export const load = async ({ locals }) => {
	if (getUserRole(locals.user) !== "admin") {
		throw redirect(303, "/dashboard");
	}

	if (!locals.backendToken) {
		return {
			adminData: {
				health: null,
				flags: null,
			},
			error: "Failed to load admin data.",
		};
	}

	try {
		return {
			adminData: await fetchKnowledgeHomeAdminSnapshot(locals.backendToken),
			error: null,
		};
	} catch (error) {
		console.error("[knowledge-home-admin] Failed to load admin snapshot:", error);
		return {
			adminData: {
				health: null,
				flags: null,
			},
			error: "Failed to load admin data.",
		};
	}
};
