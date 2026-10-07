import type { RequestHandler } from "@sveltejs/kit";
import { verifyCsrfToken } from "#lib/api.js";
import {
	createSovereignSnapshot,
	fetchSovereignAdminSnapshot,
	runSovereignRetention,
} from "#lib/server/sovereign-admin.js";
import { getUserRole } from "#lib/server/user-role.js";

export const GET: RequestHandler = async ({ locals }) => {
	if (getUserRole(locals.user) !== "admin") {
		return Response.json({ error: "Admin access required." }, { status: 403 });
	}

	try {
		const snapshot = await fetchSovereignAdminSnapshot();
		return Response.json(snapshot, {
			headers: { "Cache-Control": "no-store" },
		});
	} catch (error) {
		console.error(
			"[api/admin/knowledge-home/sovereign] Failed to fetch snapshot:",
			error,
		);
		return Response.json(
			{ error: "Failed to load sovereign admin data." },
			{ status: 502 },
		);
	}
};

export const POST: RequestHandler = async ({ locals, request, cookies }) => {
	if (getUserRole(locals.user) !== "admin") {
		return Response.json({ error: "Admin access required." }, { status: 403 });
	}

	// V-004: CSRF validation for state-changing operations (reproject/backfill
	// swap-rollback and retention purge are destructive; must not rely solely
	// on SvelteKit's default checkOrigin).
	const providedCSRF = request.headers.get("X-CSRF-Token");
	if (!verifyCsrfToken(cookies, providedCSRF)) {
		return Response.json({ error: "CSRF validation failed" }, { status: 403 });
	}

	let body: unknown;
	try {
		body = await request.json();
	} catch {
		return Response.json({ error: "Invalid request body." }, { status: 400 });
	}

	try {
		if (
			typeof body === "object" &&
			body !== null &&
			"action" in body &&
			body.action === "create_snapshot"
		) {
			const snapshot = await createSovereignSnapshot();
			return Response.json(
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
			return Response.json(
				{ ok: true, result },
				{ headers: { "Cache-Control": "no-store" } },
			);
		}

		return Response.json({ error: "Invalid action." }, { status: 400 });
	} catch (error) {
		console.error("[api/admin/knowledge-home/sovereign] Action failed:", error);
		return Response.json(
			{ error: "Failed to run sovereign admin action." },
			{ status: 502 },
		);
	}
};
