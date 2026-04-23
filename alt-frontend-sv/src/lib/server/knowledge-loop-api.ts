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
		trigger: "user_tap" | "dwell" | "keyboard" | "programmatic";
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
