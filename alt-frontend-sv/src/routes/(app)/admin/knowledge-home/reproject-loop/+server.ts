/**
 * POST /admin/knowledge-home/reproject-loop — admin-only BFF passthrough.
 *
 * Triggers a full Knowledge Loop reproject by calling knowledge-sovereign's
 * `POST /admin/knowledge-loop/reproject` endpoint. The runbook
 * (docs/runbooks/knowledge-loop-reproject.md) is the operator-facing source
 * of truth; this endpoint just lets an admin run the procedure from the
 * /admin/knowledge-home page without reaching for psql.
 *
 * Destructive — TRUNCATEs the three Knowledge Loop projection tables and
 * resets the projector checkpoint. The dedupe table is untouched (canonical
 * contract §3 invariant 8 — dedupe is ingest-side, not a projection).
 *
 * Idempotent: re-running after success is a no-op (TRUNCATE on empty tables
 * is fine; resetting the checkpoint to 0 when already 0 is fine).
 */

import { error, json, type RequestHandler } from "@sveltejs/kit";
import { triggerKnowledgeLoopReproject } from "$lib/server/sovereign-admin";
import { getUserRole } from "$lib/server/user-role";

export const POST: RequestHandler = async ({ locals }) => {
	if (getUserRole(locals.user) !== "admin") {
		throw error(403, "admin role required");
	}
	try {
		const result = await triggerKnowledgeLoopReproject();
		return json(result);
	} catch (e) {
		const message = e instanceof Error ? e.message : "unknown_error";
		throw error(502, message);
	}
};
