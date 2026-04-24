/**
 * POST /loop/transition — BFF endpoint for recording a Knowledge Loop state transition.
 *
 * The SvelteKit browser side never talks to alt-backend directly; it posts to this
 * route, which attaches locals.backendToken and calls the Connect-RPC client.
 *
 * Error mapping aligns with the Connect-RPC protocol (connectrpc.com/docs/protocol#error-codes):
 *   - missing token                                  → 401 unauthenticated
 *   - bad body / forbidden transition / non-UUIDv7   → 400 invalid_body
 *   - already_exists (sovereign replay)              → 200 { accepted: true, replay: true }
 *   - failed_precondition (stale projection)         → 409
 *   - invalid_argument                               → 400
 *   - unauthenticated / permission_denied            → 401
 *   - deadline_exceeded                              → 504
 *   - resource_exhausted                             → 429
 *   - unavailable                                    → 502 upstream_unavailable
 *   - internal                                       → 500 upstream_internal
 *   - canceled                                       → 499 canceled (client closed request)
 *   - unimplemented                                  → 501 unimplemented
 *   - not_found                                      → 404 not_found
 *   - aborted / out_of_range / data_loss / unknown   → 500 upstream_internal
 *   - bare Error (fetch TypeError, DNS, ECONNREFUSED without a Connect envelope)
 *                                                    → 502 upstream_unreachable (logged)
 */

import { json, type RequestHandler } from "@sveltejs/kit";
import { transitionKnowledgeLoopForUser } from "$lib/server/knowledge-loop-api";
import { canTransition } from "$lib/hooks/loop-transitions";
import { extractConnectCode } from "$lib/connect/error";
import type { LoopStageName } from "$lib/connect/knowledge_loop";

type Trigger = "user_tap" | "dwell" | "keyboard" | "programmatic";

interface TransitionBody {
	lensModeId: string;
	clientTransitionId: string;
	entryKey: string;
	fromStage: LoopStageName;
	toStage: LoopStageName;
	trigger: Trigger;
	observedProjectionRevision: number;
}

const STAGES: readonly LoopStageName[] = ["observe", "orient", "decide", "act"];
const TRIGGERS: readonly Trigger[] = [
	"user_tap",
	"dwell",
	"keyboard",
	"programmatic",
];

const UUIDV7_RE =
	/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

function parseBody(raw: unknown): TransitionBody | null {
	if (!raw || typeof raw !== "object") return null;
	const b = raw as Record<string, unknown>;
	if (
		typeof b.lensModeId !== "string" ||
		typeof b.clientTransitionId !== "string" ||
		typeof b.entryKey !== "string" ||
		typeof b.fromStage !== "string" ||
		typeof b.toStage !== "string" ||
		typeof b.trigger !== "string" ||
		typeof b.observedProjectionRevision !== "number"
	) {
		return null;
	}
	if (!UUIDV7_RE.test(b.clientTransitionId)) return null;
	if (!STAGES.includes(b.fromStage as LoopStageName)) return null;
	if (!STAGES.includes(b.toStage as LoopStageName)) return null;
	if (!TRIGGERS.includes(b.trigger as Trigger)) return null;
	if (!canTransition(b.fromStage as LoopStageName, b.toStage as LoopStageName))
		return null;
	if (!Number.isInteger(b.observedProjectionRevision)) return null;
	if (b.entryKey.length === 0 || b.entryKey.length > 128) return null;

	return {
		lensModeId: b.lensModeId,
		clientTransitionId: b.clientTransitionId,
		entryKey: b.entryKey,
		fromStage: b.fromStage as LoopStageName,
		toStage: b.toStage as LoopStageName,
		trigger: b.trigger as Trigger,
		observedProjectionRevision: b.observedProjectionRevision,
	};
}

export const POST: RequestHandler = async ({ request, locals }) => {
	const backendToken = locals.backendToken;
	if (!backendToken) {
		return json({ error: "unauthenticated" }, { status: 401 });
	}

	let raw: unknown;
	try {
		raw = await request.json();
	} catch {
		return json({ error: "invalid_json" }, { status: 400 });
	}

	const body = parseBody(raw);
	if (!body) {
		return json({ error: "invalid_body" }, { status: 400 });
	}

	try {
		const resp = await transitionKnowledgeLoopForUser(backendToken, body);
		return json({
			accepted: resp.accepted,
			canonicalEntryKey: resp.canonicalEntryKey,
			message: resp.message,
		});
	} catch (err) {
		const code = extractConnectCode(err);
		switch (code) {
			case "already_exists":
				return json({ accepted: true, replay: true });
			case "failed_precondition":
				return json({ error: "projection_stale" }, { status: 409 });
			case "invalid_argument":
				return json({ error: "invalid_argument" }, { status: 400 });
			case "unauthenticated":
			case "permission_denied":
				return json({ error: "unauthorized" }, { status: 401 });
			case "deadline_exceeded":
				return json({ error: "timeout" }, { status: 504 });
			case "resource_exhausted":
				return json({ error: "rate_limited" }, { status: 429 });
			case "unavailable":
				return json({ error: "upstream_unavailable" }, { status: 502 });
			case "internal":
				return json({ error: "upstream_internal" }, { status: 500 });
			case "canceled":
				return json({ error: "canceled" }, { status: 499 });
			case "unimplemented":
				return json({ error: "unimplemented" }, { status: 501 });
			case "not_found":
				return json({ error: "not_found" }, { status: 404 });
			case "aborted":
			case "out_of_range":
			case "data_loss":
			case "unknown":
				logUnknownError(err, code);
				return json({ error: "upstream_internal" }, { status: 500 });
			default:
				// bare Error (fetch TypeError, DNS, ECONNREFUSED without a Connect
				// envelope): surface as unreachable so the client can decide between
				// retrying with backoff (2xx-eventually) or failing. The log line is
				// the only diagnostic for this class, so emit it before returning.
				logUnknownError(err, code);
				return json({ error: "upstream_unreachable" }, { status: 502 });
		}
	}
};

function logUnknownError(err: unknown, code: string | undefined): void {
	const isError = err instanceof Error;
	console.error("loop.transition.unknown_error", {
		name: isError ? err.name : typeof err,
		message: isError ? err.message : String(err),
		code: code ?? null,
		cause: isError && err.cause !== undefined ? String(err.cause) : null,
	});
}
