/**
 * Admin-only BFF for the Knowledge Loop reproject control plane.
 *
 *   GET  → returns the operator status snapshot (current WhyMappingVersion +
 *          projector checkpoint) so the admin UI can render "code is at
 *          v7; projector has caught up to event_seq N" without an extra
 *          round trip.
 *   POST → triggers the full TRUNCATE-and-rerun reproject (destructive).
 *
 * The runbook (docs/runbooks/knowledge-loop-reproject.md) is the
 * operator-facing source of truth; this file just lets an admin run the
 * procedure from the /admin/knowledge-home page without reaching for psql.
 *
 * Knowledge Loop reproject TRUNCATEs the three projection tables and resets
 * the projector checkpoint. The dedupe table is untouched (canonical
 * contract §3 invariant 8 — dedupe is ingest-side, not a projection).
 *
 * Idempotent: re-running after success is a no-op (TRUNCATE on empty tables
 * is fine; resetting the checkpoint to 0 when already 0 is fine).
 */

import { error, json, type RequestHandler } from "@sveltejs/kit";
import {
	fetchKnowledgeLoopReprojectStatus,
	triggerKnowledgeLoopReproject,
} from "$lib/server/sovereign-admin";
import { getUserRole } from "$lib/server/user-role";

export const GET: RequestHandler = async ({ locals }) => {
	if (getUserRole(locals.user) !== "admin") {
		throw error(403, "admin role required");
	}
	try {
		const status = await fetchKnowledgeLoopReprojectStatus();
		return json(status);
	} catch (e) {
		const message = e instanceof Error ? e.message : "unknown_error";
		throw error(502, message);
	}
};

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
