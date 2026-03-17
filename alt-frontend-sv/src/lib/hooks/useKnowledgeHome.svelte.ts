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
	KnowledgeHomeItemData,
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

	const fetchData = async (reset = false) => {
		try {
			loading = true;
			error = null;
			const transport = createClientTransport();
			const cursor = reset ? undefined : undefined;
			const result = await getKnowledgeHome(transport, cursor);
			items = result.items;
			digest = result.digest;
			hasMore = result.hasMore;
			degraded = result.degraded;
			nextCursor = result.nextCursor;
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

	const loadMore = async () => {
		if (!hasMore || loading) return;
		try {
			loading = true;
			const transport = createClientTransport();
			const result = await getKnowledgeHome(transport, nextCursor);
			items = [...items, ...result.items];
			hasMore = result.hasMore;
			nextCursor = result.nextCursor;
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
		get hasMore() {
			return hasMore;
		},
		fetchData,
		loadMore,
		trackSeen,
		trackAction,
		dismissItem,
	};
}
