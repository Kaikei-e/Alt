import { json, type RequestHandler } from "@sveltejs/kit";
import {
	fetchKnowledgeHomeAdminSnapshot,
	pauseKnowledgeHomeBackfill,
	resumeKnowledgeHomeBackfill,
	triggerKnowledgeHomeBackfill,
	emitKnowledgeHomeArticleUrlBackfill,
	startKnowledgeHomeReproject,
	compareKnowledgeHomeReproject,
	swapKnowledgeHomeReproject,
	rollbackKnowledgeHomeReproject,
	runKnowledgeHomeAudit,
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
		console.error(
			"[api/admin/knowledge-home] Failed to refresh snapshot:",
			error,
		);
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
			return json(
				{ ok: true, job },
				{ headers: { "Cache-Control": "no-store" } },
			);
		}

		if (
			typeof body === "object" &&
			body !== null &&
			"action" in body &&
			body.action === "emit_article_url_backfill"
		) {
			const maxArticles =
				"maxArticles" in body && typeof body.maxArticles === "number"
					? body.maxArticles
					: 0;
			const dryRun = "dryRun" in body && body.dryRun === true;
			if (maxArticles < 0) {
				return json(
					{ error: "maxArticles must be non-negative." },
					{ status: 400 },
				);
			}
			const result = await emitKnowledgeHomeArticleUrlBackfill(
				locals.backendToken,
				maxArticles,
				dryRun,
			);
			return json(
				{ ok: true, result },
				{ headers: { "Cache-Control": "no-store" } },
			);
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

		if (
			typeof body === "object" &&
			body !== null &&
			"action" in body &&
			body.action === "start_reproject" &&
			"mode" in body &&
			typeof body.mode === "string" &&
			"fromVersion" in body &&
			typeof body.fromVersion === "string" &&
			"toVersion" in body &&
			typeof body.toVersion === "string"
		) {
			const rangeStart =
				"rangeStart" in body && typeof body.rangeStart === "string"
					? body.rangeStart
					: undefined;
			const rangeEnd =
				"rangeEnd" in body && typeof body.rangeEnd === "string"
					? body.rangeEnd
					: undefined;
			const run = await startKnowledgeHomeReproject(
				locals.backendToken,
				body.mode,
				body.fromVersion,
				body.toVersion,
				rangeStart,
				rangeEnd,
			);
			return json(
				{ ok: true, run },
				{ headers: { "Cache-Control": "no-store" } },
			);
		}

		if (
			typeof body === "object" &&
			body !== null &&
			"action" in body &&
			body.action === "compare_reproject" &&
			"reprojectRunId" in body &&
			typeof body.reprojectRunId === "string"
		) {
			const diff = await compareKnowledgeHomeReproject(
				locals.backendToken,
				body.reprojectRunId,
			);
			return json(
				{ ok: true, diff },
				{ headers: { "Cache-Control": "no-store" } },
			);
		}

		if (
			typeof body === "object" &&
			body !== null &&
			"action" in body &&
			body.action === "swap_reproject" &&
			"reprojectRunId" in body &&
			typeof body.reprojectRunId === "string"
		) {
			await swapKnowledgeHomeReproject(
				locals.backendToken,
				body.reprojectRunId,
			);
			return json({ ok: true }, { headers: { "Cache-Control": "no-store" } });
		}

		if (
			typeof body === "object" &&
			body !== null &&
			"action" in body &&
			body.action === "rollback_reproject" &&
			"reprojectRunId" in body &&
			typeof body.reprojectRunId === "string"
		) {
			await rollbackKnowledgeHomeReproject(
				locals.backendToken,
				body.reprojectRunId,
			);
			return json({ ok: true }, { headers: { "Cache-Control": "no-store" } });
		}

		if (
			typeof body === "object" &&
			body !== null &&
			"action" in body &&
			body.action === "run_audit" &&
			"projectionName" in body &&
			typeof body.projectionName === "string" &&
			"projectionVersion" in body &&
			typeof body.projectionVersion === "string" &&
			"sampleSize" in body &&
			typeof body.sampleSize === "number"
		) {
			const audit = await runKnowledgeHomeAudit(
				locals.backendToken,
				body.projectionName,
				body.projectionVersion,
				body.sampleSize,
			);
			return json(
				{ ok: true, audit },
				{ headers: { "Cache-Control": "no-store" } },
			);
		}

		return json({ error: "Invalid admin action." }, { status: 400 });
	} catch (error) {
		console.error(
			"[api/admin/knowledge-home] Failed to run admin action:",
			error,
		);
		return json({ error: "Failed to run admin action." }, { status: 502 });
	}
};
