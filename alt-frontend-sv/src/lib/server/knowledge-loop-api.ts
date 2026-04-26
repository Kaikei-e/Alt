/**
 * Server-side wrapper for the Knowledge Loop Connect-RPC service.
 *
 * Enforces the BFF invariant: the SvelteKit server routes are the only layer allowed to
 * talk to the backend; the browser never calls alt-backend directly. This wrapper takes
 * the backend token from `locals.backendToken` (hooks.server.ts) and emits the outbound
 * Connect-RPC call with the correct headers.
 */

import { createServerTransportWithToken } from "$lib/connect/transport-server";
import {
	getKnowledgeLoop as getKnowledgeLoopClient,
	transitionKnowledgeLoop as transitionKnowledgeLoopClient,
	type KnowledgeLoopResult,
} from "$lib/connect/knowledge_loop";
import { createAugurSessionFromLoopEntry as createAugurSessionFromLoopEntryClient } from "$lib/connect/augur";

export async function getKnowledgeLoopForUser(
	backendToken: string,
	lensModeId: string,
	opts: { foregroundLimit?: number; reducedMotion?: boolean } = {},
): Promise<KnowledgeLoopResult> {
	const transport = createServerTransportWithToken(backendToken);
	return getKnowledgeLoopClient(transport, lensModeId, opts);
}

export async function transitionKnowledgeLoopForUser(
	backendToken: string,
	args: {
		lensModeId: string;
		clientTransitionId: string;
		entryKey: string;
		fromStage: "observe" | "orient" | "decide" | "act";
		toStage: "observe" | "orient" | "decide" | "act";
		// `defer` is the soft dismiss / snooze trigger (canonical contract §8.2)
		// — the only trigger that allows fromStage===toStage.
		trigger: "user_tap" | "dwell" | "keyboard" | "programmatic" | "defer";
		observedProjectionRevision: number;
	},
): Promise<{ accepted: boolean; canonicalEntryKey?: string; message?: string }> {
	const transport = createServerTransportWithToken(backendToken);
	const resp = await transitionKnowledgeLoopClient(transport, args);
	return {
		accepted: resp.accepted,
		canonicalEntryKey: resp.canonicalEntryKey,
		message: resp.message,
	};
}

/**
 * Loop → Augur handshake. The BFF resolves the entry via sovereign first (so
 * Augur receives the canonical why_text + evidence_refs), then mints an Augur
 * conversation. Returns the new conversation id for client-side navigation.
 * See ADR-000836.
 */
export async function createAugurSessionFromLoopEntryForUser(
	backendToken: string,
	args: {
		lensModeId: string;
		clientHandshakeId: string;
		entryKey: string;
	},
): Promise<{ conversationId: string }> {
	const loop = await getKnowledgeLoopForUser(backendToken, args.lensModeId, {
		foregroundLimit: 12,
	});
	const entry = loop.foregroundEntries.find((e) => e.entryKey === args.entryKey);
	if (!entry) {
		const err = new Error("entry_not_found");
		(err as Error & { code: string }).code = "not_found";
		throw err;
	}
	const transport = createServerTransportWithToken(backendToken);
	return createAugurSessionFromLoopEntryClient(transport, {
		clientHandshakeId: args.clientHandshakeId,
		entryKey: args.entryKey,
		lensModeId: args.lensModeId,
		whyText: entry.whyPrimary.text,
		evidenceRefs: entry.whyPrimary.evidenceRefs.map((r) => ({
			refId: r.refId,
			label: r.label,
		})),
	});
}
