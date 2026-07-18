import { goto } from "$app/navigation";
import {
	type BranchData,
	type BranchResolution,
	type EpisodeData,
	type FootprintData,
	getTrail,
	resolveBranch,
	searchTrail,
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
	let episodes = $state<EpisodeData[]>([]);
	let branches = $state<BranchData[]>([]);
	let loading = $state(false);
	let error = $state<Error | null>(null);
	let hasMore = $state(false);
	let nextCursor = $state("");
	let hasEverLoaded = $state(false);

	// Trail search (D25): the sole rediscovery instrument, pull-only. State is
	// kept separate from the base spine so clearing never needs a refetch.
	let searchActive = $state(false);
	let searchQuery = $state("");
	let searchEpisodes = $state<EpisodeData[]>([]);
	let matchedItemKeys = $state<string[]>([]);
	let searching = $state(false);
	let searchError = $state<Error | null>(null);

	async function fetchData(reset: boolean): Promise<void> {
		loading = true;
		error = null;
		try {
			const transport = createClientTransport();
			const result = await getTrail(transport, reset ? undefined : nextCursor);
			footprints = reset
				? result.footprints
				: [...footprints, ...result.footprints];
			// Episodes are the spine's default display unit (D24); cursor/limit page
			// over them the same way as the legacy flat footprints did.
			episodes = reset ? result.episodes : [...episodes, ...result.episodes];
			// Branches are a full snapshot of the user's open branches (not paged).
			branches = result.branches;
			nextCursor = result.nextCursor;
			hasMore = result.hasMore;
			hasEverLoaded = true;
		} catch (err) {
			error = err instanceof Error ? err : new Error(String(err));
			if (!hasEverLoaded) {
				footprints = [];
				episodes = [];
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

	// Pull-only (D25): fires only on an explicit call from a submit handler —
	// never from an $effect or a keystroke. An empty/whitespace query is a
	// no-op so the spine is never cleared by an accidental submit.
	async function search(query: string): Promise<void> {
		const trimmed = query.trim();
		if (!trimmed) return;
		searching = true;
		searchError = null;
		try {
			const transport = createClientTransport();
			const result = await searchTrail(transport, trimmed);
			searchQuery = trimmed;
			searchEpisodes = result.episodes;
			matchedItemKeys = result.matchedItemKeys;
			searchActive = true;
		} catch (err) {
			searchError = err instanceof Error ? err : new Error(String(err));
		} finally {
			searching = false;
		}
	}

	// Restores the normal spine from already-loaded state — no refetch needed.
	function clearSearch(): void {
		searchActive = false;
		searchQuery = "";
		searchEpisodes = [];
		matchedItemKeys = [];
		searchError = null;
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
		get episodes() {
			return episodes;
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
		get searchActive() {
			return searchActive;
		},
		get searchQuery() {
			return searchQuery;
		},
		get searchEpisodes() {
			return searchEpisodes;
		},
		get matchedItemKeys() {
			return matchedItemKeys;
		},
		get searching() {
			return searching;
		},
		get searchError() {
			return searchError;
		},
		fetchData,
		loadMore,
		refresh,
		resolveBranch: resolveBranchAction,
		search,
		clearSearch,
	};
}
