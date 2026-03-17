import { json, type RequestHandler } from "@sveltejs/kit";
import {
	fetchKnowledgeHomeAdminSnapshot,
	pauseKnowledgeHomeBackfill,
	resumeKnowledgeHomeBackfill,
	triggerKnowledgeHomeBackfill,
} from "$lib/server/knowledge-home-admin";
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

export const POST: RequestHandler = async ({ locals, request }) => {
	if (getUserRole(locals.user) !== "admin") {
		return json({ error: "Admin access required." }, { status: 403 });
	}

	if (!locals.backendToken) {
		return json({ error: "Failed to run admin action." }, { status: 401 });
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
			body.action === "trigger" &&
			"projectionVersion" in body &&
			typeof body.projectionVersion === "number"
		) {
			const job = await triggerKnowledgeHomeBackfill(
				locals.backendToken,
				body.projectionVersion,
			);
			return json({ ok: true, job }, { headers: { "Cache-Control": "no-store" } });
		}

		if (
			typeof body === "object" &&
			body !== null &&
			"action" in body &&
			body.action === "pause" &&
			"jobId" in body &&
			typeof body.jobId === "string"
		) {
			await pauseKnowledgeHomeBackfill(locals.backendToken, body.jobId);
			return json({ ok: true }, { headers: { "Cache-Control": "no-store" } });
		}

		if (
			typeof body === "object" &&
			body !== null &&
			"action" in body &&
			body.action === "resume" &&
			"jobId" in body &&
			typeof body.jobId === "string"
		) {
			await resumeKnowledgeHomeBackfill(locals.backendToken, body.jobId);
			return json({ ok: true }, { headers: { "Cache-Control": "no-store" } });
		}

		return json({ error: "Invalid admin action." }, { status: 400 });
	} catch (error) {
		console.error("[api/admin/knowledge-home] Failed to run admin action:", error);
		return json({ error: "Failed to run admin action." }, { status: 502 });
	}
};
