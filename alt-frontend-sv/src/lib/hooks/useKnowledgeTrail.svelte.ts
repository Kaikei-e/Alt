import { createClientTransport } from "$lib/connect/transport-client";
import { getTrail, type FootprintData } from "$lib/connect/knowledge_trail";

/**
 * useKnowledgeTrail drives the Knowledge Trail spine. Pull-only by design: data
 * loads on an explicit fetchData()/loadMore()/refresh() call and never via an
 * $effect re-fetch. This is the direct lesson of PM-2026-039 (invalidateAll
 * storm) and PM-2026-045 (SSE silent failure) — the trail has no live channel.
 */
export function useKnowledgeTrail() {
	let footprints = $state<FootprintData[]>([]);
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
	};
}
