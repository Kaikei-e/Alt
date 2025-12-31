/**
 * TanStack Query hooks for FeedService Connect-RPC client
 *
 * Provides reactive query and mutation wrappers with:
 * - Infinite queries for paginated feeds
 * - Optimistic updates for mark as read
 * - Query invalidation patterns
 */

import {
	createQuery,
	createMutation,
	createInfiniteQuery,
	useQueryClient,
} from "@tanstack/svelte-query";
import { createClientTransport } from "$lib/connect/transport.client";
import {
	getUnreadFeeds,
	getReadFeeds,
	getFavoriteFeeds,
	searchFeeds,
	getFeedStats,
	getDetailedFeedStats,
	getUnreadCount,
	markAsRead,
	type ConnectFeedItem,
	type FeedCursorResponse,
	type FeedSearchResponse,
	type FeedStats,
	type DetailedFeedStats,
	type UnreadCount,
} from "$lib/connect/feeds";
import { feedKeys } from "./keys";

// =============================================================================
// Feed Stats Queries
// =============================================================================

/**
 * Query for basic feed statistics
 */
export function createFeedStatsQuery() {
	const transport = createClientTransport();
	return createQuery(() => ({
		queryKey: feedKeys.stats(),
		queryFn: () => getFeedStats(transport),
		staleTime: 1000 * 60 * 5, // 5 minutes
	}));
}

/**
 * Query for detailed feed statistics
 */
export function createDetailedFeedStatsQuery() {
	const transport = createClientTransport();
	return createQuery(() => ({
		queryKey: feedKeys.detailedStats(),
		queryFn: () => getDetailedFeedStats(transport),
		staleTime: 1000 * 60 * 5, // 5 minutes
	}));
}

/**
 * Query for unread count
 */
export function createUnreadCountQuery() {
	const transport = createClientTransport();
	return createQuery(() => ({
		queryKey: feedKeys.unreadCount(),
		queryFn: () => getUnreadCount(transport),
		staleTime: 1000 * 60, // 1 minute
	}));
}

// =============================================================================
// Feed List Infinite Queries
// =============================================================================

/**
 * Infinite query for unread feeds with cursor-based pagination
 */
export function createUnreadFeedsQuery(limit: number = 20) {
	const transport = createClientTransport();
	return createInfiniteQuery(() => ({
		queryKey: feedKeys.unread(),
		queryFn: ({ pageParam }: { pageParam: string | undefined }) =>
			getUnreadFeeds(transport, pageParam, limit),
		getNextPageParam: (lastPage: FeedCursorResponse) =>
			lastPage.hasMore ? lastPage.nextCursor ?? undefined : undefined,
		initialPageParam: undefined as string | undefined,
	}));
}

/**
 * Infinite query for read feeds with cursor-based pagination
 */
export function createReadFeedsQuery(limit: number = 32) {
	const transport = createClientTransport();
	return createInfiniteQuery(() => ({
		queryKey: feedKeys.read(),
		queryFn: ({ pageParam }: { pageParam: string | undefined }) =>
			getReadFeeds(transport, pageParam, limit),
		getNextPageParam: (lastPage: FeedCursorResponse) =>
			lastPage.hasMore ? lastPage.nextCursor ?? undefined : undefined,
		initialPageParam: undefined as string | undefined,
	}));
}

/**
 * Infinite query for favorite feeds with cursor-based pagination
 */
export function createFavoriteFeedsQuery(limit: number = 20) {
	const transport = createClientTransport();
	return createInfiniteQuery(() => ({
		queryKey: feedKeys.favorites(),
		queryFn: ({ pageParam }: { pageParam: string | undefined }) =>
			getFavoriteFeeds(transport, pageParam, limit),
		getNextPageParam: (lastPage: FeedCursorResponse) =>
			lastPage.hasMore ? lastPage.nextCursor ?? undefined : undefined,
		initialPageParam: undefined as string | undefined,
	}));
}

// =============================================================================
// Search Query
// =============================================================================

/**
 * Query for feed search with offset-based pagination
 */
export function createSearchFeedsQuery(
	query: string,
	limit: number = 20,
	enabled: boolean = true,
) {
	const transport = createClientTransport();
	return createInfiniteQuery(() => ({
		queryKey: feedKeys.search(query),
		queryFn: ({ pageParam }: { pageParam: number | undefined }) =>
			searchFeeds(transport, query, pageParam, limit),
		getNextPageParam: (lastPage: FeedSearchResponse) =>
			lastPage.hasMore ? lastPage.nextCursor ?? undefined : undefined,
		initialPageParam: undefined as number | undefined,
		enabled: enabled && query.length > 0,
	}));
}

// =============================================================================
// Mark As Read Mutation
// =============================================================================

interface MarkAsReadContext {
	previousUnread?: {
		pages: FeedCursorResponse[];
		pageParams: (string | undefined)[];
	};
}

/**
 * Mutation for marking a feed as read with optimistic updates
 */
export function createMarkAsReadMutation() {
	const queryClient = useQueryClient();
	const transport = createClientTransport();

	return createMutation(() => ({
		mutationFn: (feedUrl: string) => markAsRead(transport, feedUrl),
		onMutate: async (feedUrl: string) => {
			// Cancel any outgoing refetches
			await queryClient.cancelQueries({ queryKey: feedKeys.unread() });

			// Snapshot previous value
			const previousUnread = queryClient.getQueryData<{
				pages: FeedCursorResponse[];
				pageParams: (string | undefined)[];
			}>(feedKeys.unread());

			// Optimistically update: remove from unread list
			if (previousUnread) {
				queryClient.setQueryData(feedKeys.unread(), {
					...previousUnread,
					pages: previousUnread.pages.map((page) => ({
						...page,
						data: page.data.filter(
							(feed: ConnectFeedItem) => feed.link !== feedUrl,
						),
					})),
				});
			}

			return { previousUnread } as MarkAsReadContext;
		},
		onError: (
			_err: Error,
			_feedUrl: string,
			context: MarkAsReadContext | undefined,
		) => {
			// Rollback on error
			if (context?.previousUnread) {
				queryClient.setQueryData(feedKeys.unread(), context.previousUnread);
			}
		},
		onSettled: () => {
			// Invalidate related queries
			queryClient.invalidateQueries({ queryKey: feedKeys.stats() });
			queryClient.invalidateQueries({ queryKey: feedKeys.unreadCount() });
			// Add to read list
			queryClient.invalidateQueries({ queryKey: feedKeys.read() });
		},
	}));
}

// =============================================================================
// Helper: Flatten pages to single array
// =============================================================================

/**
 * Helper to flatten infinite query pages into a single array
 */
export function flattenFeedPages(
	data: { pages: FeedCursorResponse[] } | undefined,
): ConnectFeedItem[] {
	if (!data?.pages) return [];
	return data.pages.flatMap((page) => page.data);
}

/**
 * Helper to flatten search pages into a single array
 */
export function flattenSearchPages(
	data: { pages: FeedSearchResponse[] } | undefined,
): ConnectFeedItem[] {
	if (!data?.pages) return [];
	return data.pages.flatMap((page) => page.data);
}
