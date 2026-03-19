/**
 * Headless composable for Knowledge Home data fetching and state management.
 * Follows the usePulse.svelte.ts Runes pattern.
 */

import { Code, ConnectError } from "@connectrpc/connect";
import { goto } from "$app/navigation";
import {
	createClientTransport,
	getKnowledgeHome,
	trackHomeAction,
	trackHomeItemsSeen,
} from "$lib/connect";
import type {
	FeatureFlagData,
	KnowledgeHomeItemData,
	RecallCandidateData,
	ServiceQuality,
	TodayDigestData,
} from "$lib/connect/knowledge_home";

type EmptyReason =
	| "no_data"
	| "ingest_pending"
	| "lens_strict"
	| "search_strict"
	| "degraded"
	| "hard_error";

export type PageState =
	| "initial_loading"
	| "ready"
	| "refreshing"
	| "degraded"
	| "fallback"
	| "hard_error";

export function useKnowledgeHome() {
	let items = $state<KnowledgeHomeItemData[]>([]);
	let digest = $state<TodayDigestData | null>(null);
	let loading = $state(false);
	let error = $state<Error | null>(null);
	let degraded = $state(false);
	let hasMore = $state(false);
	let nextCursor = $state("");
	let serviceQuality = $state<ServiceQuality>("full");
	let featureFlags = $state<FeatureFlagData[]>([]);
	let recallCandidates = $state<RecallCandidateData[]>([]);
	let hasEverLoaded = $state(false);
	let pageState = $state<PageState>("initial_loading");

	function computePageState(opts: {
		loading: boolean;
		hasEverLoaded: boolean;
		error: Error | null;
		serviceQuality: ServiceQuality;
	}): PageState {
		if (opts.error) return "hard_error";
		if (opts.loading && !opts.hasEverLoaded) return "initial_loading";
		if (opts.loading && opts.hasEverLoaded) return "refreshing";
		if (opts.serviceQuality === "fallback") return "fallback";
		if (opts.serviceQuality === "degraded") return "degraded";
		return "ready";
	}

	function computeEmptyReason(
		items: KnowledgeHomeItemData[],
		error: Error | null,
		hasEverLoaded: boolean,
		lensActive: boolean,
		digest: TodayDigestData | null,
	): EmptyReason | null {
		if (items.length > 0) return null;
		if (error) return "hard_error";
		if (!hasEverLoaded) return null;
		if (lensActive) return "lens_strict";
		if (digest && digest.unsummarizedArticles > 0) return "ingest_pending";
		return "no_data";
	}

	let activeLensId: string | null = null;

	const fetchData = async (reset = false, lensId?: string | null) => {
		try {
			loading = true;
			error = null;
			activeLensId = lensId ?? null;
			pageState = computePageState({
				loading: true,
				hasEverLoaded,
				error: null,
				serviceQuality,
			});
			const transport = createClientTransport();
			const cursor = reset ? undefined : nextCursor || undefined;
			const result = await getKnowledgeHome(
				transport,
				cursor,
				20,
				lensId ?? undefined,
			);
			items = result.items;
			digest = result.digest;
			hasMore = result.hasMore;
			degraded = result.degraded;
			nextCursor = result.nextCursor;
			serviceQuality = result.serviceQuality;
			featureFlags = result.featureFlags;
			recallCandidates = result.recallCandidates;
			hasEverLoaded = true;
			pageState = computePageState({
				loading: false,
				hasEverLoaded: true,
				error: null,
				serviceQuality: result.serviceQuality,
			});
		} catch (err) {
			if (err instanceof ConnectError) {
				if (err.code === Code.Unauthenticated) {
					goto("/login");
					return;
				}
			}
			error = err instanceof Error ? err : new Error("Unknown error");
			items = [];
			pageState = computePageState({
				loading: false,
				hasEverLoaded,
				error,
				serviceQuality,
			});
		} finally {
			loading = false;
		}
	};

	const loadMore = async (lensId?: string | null) => {
		if (!hasMore || loading) return;
		try {
			loading = true;
			const transport = createClientTransport();
			const result = await getKnowledgeHome(
				transport,
				nextCursor,
				20,
				lensId ?? undefined,
			);
			items = [...items, ...result.items];
			hasMore = result.hasMore;
			nextCursor = result.nextCursor;
			serviceQuality = result.serviceQuality;
		} catch (err) {
			if (err instanceof ConnectError && err.code === Code.Unauthenticated) {
				goto("/login");
				return;
			}
			error = err instanceof Error ? err : new Error("Unknown error");
		} finally {
			loading = false;
		}
	};

	const trackSeen = (itemKeys: string[], sessionId: string) => {
		if (itemKeys.length === 0) return;
		const transport = createClientTransport();
		trackHomeItemsSeen(transport, itemKeys, sessionId).catch(() => {
			// Fire-and-forget: silently ignore tracking errors
		});
	};

	const trackAction = (type: string, itemKey: string, metadata?: string) => {
		const transport = createClientTransport();
		trackHomeAction(transport, type, itemKey, metadata).catch(() => {
			// Fire-and-forget: silently ignore tracking errors
		});
	};

	const dismissItem = (itemKey: string) => {
		// Optimistic removal
		items = items.filter((item) => item.itemKey !== itemKey);
	};

	return {
		get items() {
			return items;
		},
		get digest() {
			return digest;
		},
		get loading() {
			return loading;
		},
		get error() {
			return error;
		},
		get degraded() {
			return degraded;
		},
		get serviceQuality() {
			return serviceQuality;
		},
		get hasMore() {
			return hasMore;
		},
		get featureFlags() {
			return featureFlags;
		},
		get recallCandidates() {
			return recallCandidates;
		},
		get pageState() {
			return pageState;
		},
		get emptyReason(): EmptyReason | null {
			return computeEmptyReason(
				items,
				error,
				hasEverLoaded,
				activeLensId !== null,
				digest,
			);
		},
		fetchData,
		loadMore,
		trackSeen,
		trackAction,
		dismissItem,
	};
}
