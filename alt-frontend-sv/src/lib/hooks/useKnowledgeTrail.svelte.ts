import { goto } from "$app/navigation";
import {
	type BranchData,
	type BranchResolution,
	type FootprintData,
	getTrail,
	resolveBranch,
} from "$lib/connect/knowledge_trail";
import { createClientTransport } from "$lib/connect/transport-client";
import { uuidv7 } from "$lib/utils/uuidv7";

/**
 * useKnowledgeTrail drives the Knowledge Trail spine. Pull-only by design: data
 * loads on an explicit fetchData()/loadMore()/refresh() call and never via an
 * $effect re-fetch. This is the direct lesson of PM-2026-039 (invalidateAll
 * storm) and PM-2026-045 (SSE silent failure) — the trail has no live channel.
 */
export function useKnowledgeTrail() {
	let footprints = $state<FootprintData[]>([]);
	let branches = $state<BranchData[]>([]);
	let loading = $state(false);
	let error = $state<Error | null>(null);
	let hasMore = $state(false);
	let nextCursor = $state("");
	let hasEverLoaded = $state(false);

	async function fetchData(reset: boolean): Promise<void> {
		loading = true;
		error = null;
		try {
			const transport = createClientTransport();
			const result = await getTrail(transport, reset ? undefined : nextCursor);
			footprints = reset
				? result.footprints
				: [...footprints, ...result.footprints];
			// Branches are a full snapshot of the user's open branches (not paged).
			branches = result.branches;
			nextCursor = result.nextCursor;
			hasMore = result.hasMore;
			hasEverLoaded = true;
		} catch (err) {
			error = err instanceof Error ? err : new Error(String(err));
			if (!hasEverLoaded) {
				footprints = [];
			}
		} finally {
			loading = false;
		}
	}

	async function loadMore(): Promise<void> {
		if (loading || !hasMore) return;
		await fetchData(false);
	}

	async function refresh(): Promise<void> {
		await fetchData(true);
	}

	// resolveBranch is the single owner of the branch-resolution emit (one
	// component must not also emit — the PM-2026-045 tile-double-fire lesson).
	// It mints the idempotent UUIDv7 and records the resolution. Taking a branch
	// means walking it (D19): on `taken` the user is carried to the article,
	// with ?trail_proposal= as the sole gate for dwell measurement. Dismissals
	// re-fetch so the closure shows as a return-diff (the branch leaves the
	// open set).
	async function resolveBranchAction(
		branchKey: string,
		resolution: BranchResolution,
		targetItemKey?: string,
	): Promise<void> {
		try {
			const transport = createClientTransport();
			await resolveBranch(transport, branchKey, resolution, uuidv7());
		} catch (err) {
			error = err instanceof Error ? err : new Error(String(err));
			return;
		}
		const articleId =
			resolution === "taken" && targetItemKey?.startsWith("article:")
				? targetItemKey.slice(8)
				: null;
		if (articleId) {
			await goto(
				`/articles/${articleId}?trail_proposal=${encodeURIComponent(branchKey)}`,
			);
			return;
		}
		await fetchData(true);
	}

	return {
		get footprints() {
			return footprints;
		},
		get branches() {
			return branches;
		},
		get loading() {
			return loading;
		},
		get error() {
			return error;
		},
		get hasMore() {
			return hasMore;
		},
		get hasEverLoaded() {
			return hasEverLoaded;
		},
		fetchData,
		loadMore,
		refresh,
		resolveBranch: resolveBranchAction,
	};
}
