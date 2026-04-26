import type { ServerLoad } from "@sveltejs/kit";
import type { KnowledgeLoopResult } from "$lib/connect/knowledge_loop";
import { getKnowledgeLoopForUser } from "$lib/server/knowledge-loop-api";

export const load: ServerLoad = async ({ locals, url, depends }) => {
	// Register a coarse-grained dependency tag so the page can refetch *only*
	// this load via `invalidate("loop:data")` — without churning sibling +layout
	// loads or causing the wholesale `invalidateAll()` storm we saw in
	// 2026-04-26 nginx + alt-backend logs.
	depends("loop:data");

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
