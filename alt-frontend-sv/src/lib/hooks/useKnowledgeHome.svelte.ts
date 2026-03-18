/**
 * Headless composable for Knowledge Home data fetching and state management.
 * Follows the usePulse.svelte.ts Runes pattern.
 */
import { goto } from "$app/navigation";
import { ConnectError, Code } from "@connectrpc/connect";
import {
	createClientTransport,
	getKnowledgeHome,
	trackHomeItemsSeen,
	trackHomeAction,
} from "$lib/connect";
import type {
	FeatureFlagData,
	KnowledgeHomeItemData,
	ServiceQuality,
	TodayDigestData,
} from "$lib/connect/knowledge_home";

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

	const fetchData = async (reset = false, lensId?: string | null) => {
		try {
			loading = true;
			error = null;
			const transport = createClientTransport();
			const cursor = reset ? undefined : nextCursor || undefined;
			const result = await getKnowledgeHome(transport, cursor, 20, lensId ?? undefined);
			items = result.items;
			digest = result.digest;
			hasMore = result.hasMore;
			degraded = result.degraded;
			nextCursor = result.nextCursor;
			serviceQuality = result.serviceQuality;
			featureFlags = result.featureFlags;
		} catch (err) {
			if (err instanceof ConnectError) {
				if (err.code === Code.Unauthenticated) {
					goto("/login");
					return;
				}
			}
			error = err instanceof Error ? err : new Error("Unknown error");
			items = [];
		} finally {
			loading = false;
		}
	};

	const loadMore = async (lensId?: string | null) => {
		if (!hasMore || loading) return;
		try {
			loading = true;
			const transport = createClientTransport();
			const result = await getKnowledgeHome(transport, nextCursor, 20, lensId ?? undefined);
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
		trackAction("dismiss", itemKey);
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
		fetchData,
		loadMore,
		trackSeen,
		trackAction,
		dismissItem,
	};
}
