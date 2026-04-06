import { json, type RequestHandler } from "@sveltejs/kit";
import {
	fetchSovereignAdminSnapshot,
	createSovereignSnapshot,
	runSovereignRetention,
} from "$lib/server/sovereign-admin";
import { getUserRole } from "$lib/server/user-role";

export const GET: RequestHandler = async ({ locals }) => {
	if (getUserRole(locals.user) !== "admin") {
		return json({ error: "Admin access required." }, { status: 403 });
	}

	try {
		const snapshot = await fetchSovereignAdminSnapshot();
		return json(snapshot, { headers: { "Cache-Control": "no-store" } });
	} catch (error) {
		console.error(
			"[api/admin/knowledge-home/sovereign] Failed to fetch snapshot:",
			error,
		);
		return json(
			{ error: "Failed to load sovereign admin data." },
			{ status: 502 },
		);
	}
};

export const POST: RequestHandler = async ({ locals, request }) => {
	if (getUserRole(locals.user) !== "admin") {
		return json({ error: "Admin access required." }, { status: 403 });
	}

	let body: unknown;
	try {
		body = await request.json();
	} catch {
		return json({ error: "Invalid request body." }, { status: 400 });
	}

	try {
		if (
			typeof body === "object" &&
			body !== null &&
			"action" in body &&
			body.action === "create_snapshot"
		) {
			const snapshot = await createSovereignSnapshot();
			return json(
				{ ok: true, snapshot },
				{ headers: { "Cache-Control": "no-store" } },
			);
		}

		if (
			typeof body === "object" &&
			body !== null &&
			"action" in body &&
			body.action === "run_retention" &&
			"dry_run" in body &&
			typeof body.dry_run === "boolean"
		) {
			const result = await runSovereignRetention(body.dry_run);
			return json(
				{ ok: true, result },
				{ headers: { "Cache-Control": "no-store" } },
			);
		}

		return json({ error: "Invalid action." }, { status: 400 });
	} catch (error) {
		console.error("[api/admin/knowledge-home/sovereign] Action failed:", error);
		return json(
			{ error: "Failed to run sovereign admin action." },
			{ status: 502 },
		);
	}
};
