/**
 * POST /loop/act-outcome — BFF endpoint that closes the OODA loop in real
 * time by forwarding the FE's dwell-derived outcome signal to the
 * KnowledgeLoopService.EmitActOutcome RPC (ADR-000912).
 *
 * Body shape:
 * {
 *   entryKey: string,
 *   outcome: "engaged" | "deep_engagement" | "stale_save" | "accepted_change",
 *   clientOutcomeId: string, // UUIDv7
 *   occurredAtIso: string,   // ISO-8601, never wall-clock derived server-side
 *   dwellSeconds?: number,
 *   askTurns?: number,
 *   lensModeId?: string
 * }
 *
 * Responses:
 *   - 200 { accepted, deduplicated, eventSeq }   on success / idempotent retry
 *   - 400 { error }                              malformed body
 *   - 401 { error }                              no backend token
 *   - 5xx { error }                              upstream connect error
 *
 * Browser never talks to alt-backend directly — the route attaches
 * locals.backendToken before constructing the Connect-RPC transport.
 */

import { json, type RequestHandler } from "@sveltejs/kit";
import { emitActOutcomeForUser } from "$lib/server/knowledge-loop-api";
import { extractConnectCode } from "$lib/connect/error";

type Outcome = "engaged" | "deep_engagement" | "stale_save" | "accepted_change";

const ALLOWED_OUTCOMES: ReadonlySet<Outcome> = new Set([
	"engaged",
	"deep_engagement",
	"stale_save",
	"accepted_change",
]);

interface Body {
	entryKey?: unknown;
	outcome?: unknown;
	clientOutcomeId?: unknown;
	occurredAtIso?: unknown;
	dwellSeconds?: unknown;
	askTurns?: unknown;
	lensModeId?: unknown;
}

export const POST: RequestHandler = async ({ request, locals }) => {
	const token = locals.backendToken;
	if (!token) {
		return json({ error: "unauthenticated" }, { status: 401 });
	}

	let raw: Body;
	try {
		raw = (await request.json()) as Body;
	} catch {
		return json({ error: "invalid_body" }, { status: 400 });
	}

	const entryKey = typeof raw.entryKey === "string" ? raw.entryKey : "";
	const outcomeStr = typeof raw.outcome === "string" ? raw.outcome : "";
	const clientOutcomeId =
		typeof raw.clientOutcomeId === "string" ? raw.clientOutcomeId : "";
	const occurredAtIso =
		typeof raw.occurredAtIso === "string" ? raw.occurredAtIso : "";

	if (!entryKey || !clientOutcomeId || !occurredAtIso) {
		return json({ error: "invalid_body" }, { status: 400 });
	}
	if (!ALLOWED_OUTCOMES.has(outcomeStr as Outcome)) {
		return json({ error: "invalid_outcome" }, { status: 400 });
	}

	const occurredAt = new Date(occurredAtIso);
	if (Number.isNaN(occurredAt.getTime())) {
		return json({ error: "invalid_occurred_at" }, { status: 400 });
	}

	const dwellSeconds =
		typeof raw.dwellSeconds === "number" && raw.dwellSeconds >= 0
			? Math.floor(raw.dwellSeconds)
			: undefined;
	const askTurns =
		typeof raw.askTurns === "number" && raw.askTurns >= 0
			? Math.floor(raw.askTurns)
			: undefined;
	const lensModeId =
		typeof raw.lensModeId === "string" && raw.lensModeId.length > 0
			? raw.lensModeId
			: undefined;

	try {
		const res = await emitActOutcomeForUser(token, {
			entryKey,
			outcome: outcomeStr as Outcome,
			clientOutcomeId,
			occurredAt,
			dwellSeconds,
			askTurns,
			lensModeId,
		});
		return json(
			{
				accepted: res.accepted,
				deduplicated: res.deduplicated,
				eventSeq: Number(res.eventSeq),
			},
			{ status: 200 },
		);
	} catch (err) {
		const code = extractConnectCode(err);
		switch (code) {
			case "invalid_argument":
				return json({ error: "invalid_argument" }, { status: 400 });
			case "unauthenticated":
			case "permission_denied":
				return json({ error: code }, { status: 401 });
			case "deadline_exceeded":
				return json({ error: code }, { status: 504 });
			case "resource_exhausted":
				return json({ error: code }, { status: 429 });
			case "unavailable":
				return json({ error: "upstream_unavailable" }, { status: 502 });
			default:
				return json({ error: "upstream_internal" }, { status: 500 });
		}
	}
};
