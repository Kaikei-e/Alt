/**
 * POST /loop/ask — BFF endpoint for the Knowledge Loop → Ask Augur handshake.
 *
 * Flow (see ADR-000836):
 *   1. Client taps the Ask CTA on a /loop tile.
 *   2. Browser POSTs { lensModeId, clientHandshakeId, entryKey } to this route.
 *   3. BFF resolves the entry through sovereign (via getKnowledgeLoopForUser)
 *      and enriches the Augur request with the canonical why_text and
 *      evidence_refs the user saw on the tile.
 *   4. BFF calls alt-backend's proxy, which forwards to rag-orchestrator.
 *   5. Response carries the new conversation_id; the browser navigates to
 *      /augur/<conversation_id>.
 *
 * Error mapping:
 *   - missing token                                         → 401
 *   - non-UUIDv7 handshake / missing entry_key              → 400
 *   - entry not in user's foreground (stale or wrong lens)  → 404
 *   - anything else                                          → 502
 */

import { json, type RequestHandler } from "@sveltejs/kit";
import { createAugurSessionFromLoopEntryForUser } from "$lib/server/knowledge-loop-api";

interface AskBody {
	lensModeId: string;
	clientHandshakeId: string;
	entryKey: string;
}

const UUIDV7_RE =
	/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

function parseBody(raw: unknown): AskBody | null {
	if (!raw || typeof raw !== "object") return null;
	const b = raw as Record<string, unknown>;
	if (
		typeof b.lensModeId !== "string" ||
		typeof b.clientHandshakeId !== "string" ||
		typeof b.entryKey !== "string"
	) {
		return null;
	}
	if (!UUIDV7_RE.test(b.clientHandshakeId)) return null;
	if (b.entryKey.length === 0 || b.entryKey.length > 128) return null;
	if (b.lensModeId.length === 0 || b.lensModeId.length > 64) return null;
	return {
		lensModeId: b.lensModeId,
		clientHandshakeId: b.clientHandshakeId,
		entryKey: b.entryKey,
	};
}

function extractCode(err: unknown): string | undefined {
	if (err && typeof err === "object" && "code" in err) {
		const c = (err as { code: unknown }).code;
		return typeof c === "string" ? c.toLowerCase() : undefined;
	}
	return undefined;
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
		const result = await createAugurSessionFromLoopEntryForUser(
			backendToken,
			body,
		);
		return json({ conversationId: result.conversationId });
	} catch (err) {
		const code = extractCode(err);
		if (code === "not_found") {
			return json({ error: "entry_not_found" }, { status: 404 });
		}
		if (code === "invalid_argument") {
			return json({ error: "invalid_argument" }, { status: 400 });
		}
		if (code === "unauthenticated" || code === "permission_denied") {
			return json({ error: "unauthorized" }, { status: 401 });
		}
		return json({ error: "upstream_failure" }, { status: 502 });
	}
};
