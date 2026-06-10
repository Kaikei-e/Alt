import { createClientTransport } from "$lib/connect/transport-client";
import {
	getTrail,
	resolveBranch,
	type FootprintData,
	type BranchData,
	type BranchResolution,
} from "$lib/connect/knowledge_trail";
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
	let activeTags = $state<string[]>([]);

	async function fetchData(reset: boolean): Promise<void> {
		loading = true;
		error = null;
		try {
			const transport = createClientTransport();
			const result = await getTrail(
				transport,
				reset ? undefined : nextCursor,
				20,
				activeTags,
			);
			footprints = reset ? result.footprints : [...footprints, ...result.footprints];
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
	// It mints the idempotent UUIDv7, records the resolution, then re-fetches so
	// the closure shows as a return-diff (dismissed branches leave the open set).
	async function resolveBranchAction(
		branchKey: string,
		resolution: BranchResolution,
	): Promise<void> {
		try {
			const transport = createClientTransport();
			await resolveBranch(transport, branchKey, resolution, uuidv7());
		} catch (err) {
			error = err instanceof Error ? err : new Error(String(err));
			return;
		}
		await fetchData(true);
	}

	// setLens applies (or clears with []) the theme lens and re-fetches from the
	// top. Pull-only: the re-fetch is an explicit user action, not an $effect.
	async function setLens(tags: string[]): Promise<void> {
		activeTags = tags;
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
		get activeTags() {
			return activeTags;
		},
		fetchData,
		loadMore,
		refresh,
		setLens,
		resolveBranch: resolveBranchAction,
	};
}
