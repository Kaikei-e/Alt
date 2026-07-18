import { goto } from "$app/navigation";
import {
	type BranchData,
	type BranchResolution,
	getItemBranches,
	resolveBranch,
} from "$lib/connect/knowledge_trail";
import { createClientTransport } from "$lib/connect/transport-client";
import { uuidv7 } from "$lib/utils/uuidv7";

/**
 * useArticleEndBranches drives the article page's patch-exit branch surface
 * (D26): at most two system-proposed next steps anchored on the article the
 * user just finished reading. Pull-only — `load()` fires once from the
 * page's onMount, never an $effect re-fetch (the PM-2026-039/PM-2026-045
 * lesson the trail already encodes).
 */
export function useArticleEndBranches() {
	let branches = $state<BranchData[]>([]);
	let loading = $state(false);
	let error = $state<Error | null>(null);

	async function load(itemKey: string, limit = 2): Promise<void> {
		loading = true;
		error = null;
		try {
			const transport = createClientTransport();
			branches = await getItemBranches(transport, itemKey, limit);
		} catch (err) {
			error = err instanceof Error ? err : new Error(String(err));
			branches = [];
		} finally {
			loading = false;
		}
	}

	// Taking a branch walks it (D19): the user is carried to the target
	// article with ?trail_proposal= as the sole dwell-measurement gate — the
	// same contract the trail page's branch resolution has always used.
	// Dismissing (with or without a reason) drops it from the local view;
	// the article page shows a snapshot, not a live feed.
	async function resolve(
		branchKey: string,
		resolution: BranchResolution,
		targetItemKey?: string,
		dismissReason?: string,
	): Promise<void> {
		try {
			const transport = createClientTransport();
			await resolveBranch(
				transport,
				branchKey,
				resolution,
				uuidv7(),
				dismissReason,
			);
		} catch (err) {
			error = err instanceof Error ? err : new Error(String(err));
			return;
		}
		if (resolution === "taken" && targetItemKey?.startsWith("article:")) {
			await goto(
				`/articles/${targetItemKey.slice(8)}?trail_proposal=${encodeURIComponent(branchKey)}`,
			);
			return;
		}
		branches = branches.filter((b) => b.branchKey !== branchKey);
	}

	return {
		get branches() {
			return branches;
		},
		get loading() {
			return loading;
		},
		get error() {
			return error;
		},
		load,
		resolve,
	};
}
