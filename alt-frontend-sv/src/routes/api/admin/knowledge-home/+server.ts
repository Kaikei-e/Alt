import { json, type RequestHandler } from "@sveltejs/kit";
import { fetchKnowledgeHomeAdminSnapshot } from "$lib/server/knowledge-home-admin";
import { getUserRole } from "$lib/server/user-role";

export const GET: RequestHandler = async ({ locals }) => {
	if (getUserRole(locals.user) !== "admin") {
		return json({ error: "Admin access required." }, { status: 403 });
	}

	if (!locals.backendToken) {
		return json({ error: "Failed to load admin data." }, { status: 401 });
	}

	try {
		const snapshot = await fetchKnowledgeHomeAdminSnapshot(locals.backendToken);
		return json(snapshot, { headers: { "Cache-Control": "no-store" } });
	} catch (error) {
		console.error("[api/admin/knowledge-home] Failed to refresh snapshot:", error);
		return json({ error: "Failed to load admin data." }, { status: 502 });
	}
};
