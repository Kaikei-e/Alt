/**
 * State hook for the Ask Augur history index.
 *
 * Wraps listAugurConversations/deleteAugurConversation with Svelte 5 Runes
 * so the /augur/history route can stay declarative.
 */

import {
	createClientTransport,
	listAugurConversations,
	deleteAugurConversation,
	type AugurConversationSummary,
} from "$lib/connect";

export interface UseAugurHistoryOptions {
	/** Page size for listConversations (default 20). */
	pageSize?: number;
	/** Pre-seed the list when SSR already fetched it. */
	initialConversations?: AugurConversationSummary[];
	/** Pre-seed the continuation token from SSR. */
	initialNextPageToken?: string;
}

export function useAugurHistory(options: UseAugurHistoryOptions = {}) {
	let conversations = $state<AugurConversationSummary[]>(
		options.initialConversations ?? [],
	);
	let nextPageToken = $state<string>(options.initialNextPageToken ?? "");
	let isLoading = $state(false);
	let errorMessage = $state<string>("");

	async function refresh() {
		isLoading = true;
		errorMessage = "";
		try {
			const result = await listAugurConversations(createClientTransport(), {
				pageSize: options.pageSize ?? 20,
			});
			conversations = result.conversations;
			nextPageToken = result.nextPageToken;
		} catch (err) {
			errorMessage = err instanceof Error ? err.message : "Failed to load";
		} finally {
			isLoading = false;
		}
	}

	async function loadMore() {
		if (!nextPageToken || isLoading) return;
		isLoading = true;
		try {
			const result = await listAugurConversations(createClientTransport(), {
				pageSize: options.pageSize ?? 20,
				pageToken: nextPageToken,
			});
			conversations = [...conversations, ...result.conversations];
			nextPageToken = result.nextPageToken;
		} catch (err) {
			errorMessage = err instanceof Error ? err.message : "Failed to load";
		} finally {
			isLoading = false;
		}
	}

	async function remove(id: string) {
		// optimistic remove; restore on failure
		const snapshot = conversations;
		conversations = conversations.filter((c) => c.id !== id);
		try {
			await deleteAugurConversation(createClientTransport(), id);
		} catch (err) {
			conversations = snapshot;
			errorMessage = err instanceof Error ? err.message : "Failed to delete";
			throw err;
		}
	}

	return {
		get conversations() {
			return conversations;
		},
		get isLoading() {
			return isLoading;
		},
		get errorMessage() {
			return errorMessage;
		},
		get hasMore() {
			return nextPageToken !== "";
		},
		refresh,
		loadMore,
		remove,
	};
}
