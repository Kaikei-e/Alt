import type { ServerLoad } from "@sveltejs/kit";
import { getKnowledgeLoopForUser } from "$lib/server/knowledge-loop-api";
import type { KnowledgeLoopResult } from "$lib/connect/knowledge_loop";

export const load: ServerLoad = async ({ locals, url }) => {
	const backendToken = locals.backendToken;
	const lensModeId = url.searchParams.get("lens") ?? "default";

	if (!backendToken) {
		return {
			loop: null as KnowledgeLoopResult | null,
			error: "unauthenticated" as const,
			lensModeId,
		};
	}

	try {
		const loop = await getKnowledgeLoopForUser(backendToken, lensModeId, {
			foregroundLimit: 3,
		});
		return { loop, error: null, lensModeId };
	} catch (err) {
		return {
			loop: null as KnowledgeLoopResult | null,
			error: err instanceof Error ? err.message : "fetch_failed",
			lensModeId,
		};
	}
};
