/**
 * TanStack Query hooks for RSSService Connect-RPC client
 *
 * Provides reactive query and mutation wrappers with:
 * - RSS feed link listing
 * - Register/delete mutations
 * - Favorite registration
 */

import {
	createMutation,
	createQuery,
	useQueryClient,
} from "@tanstack/svelte-query";
import {
	deleteRSSFeedLink,
	type ListRSSFeedLinksResult,
	listRSSFeedLinks,
	type RSSFeedLink,
	registerFavoriteFeed,
	registerRSSFeed,
} from "$lib/connect/rss";
import { createClientTransport } from "$lib/connect/transport-client";
import { feedKeys, rssKeys } from "./keys";

// =============================================================================
// RSS Feed Links Query
// =============================================================================

/**
 * Query for listing all RSS feed links
 */
export function createRSSLinksQuery() {
	const transport = createClientTransport();
	return createQuery(() => ({
		queryKey: rssKeys.links(),
		queryFn: () => listRSSFeedLinks(transport),
		staleTime: 1000 * 60 * 5, // 5 minutes
	}));
}

// =============================================================================
// Register RSS Feed Mutation
// =============================================================================

/**
 * Mutation for registering a new RSS feed
 */
export function createRegisterRSSMutation() {
	const queryClient = useQueryClient();
	const transport = createClientTransport();

	return createMutation(() => ({
		mutationFn: (url: string) => registerRSSFeed(transport, url),
		onSuccess: () => {
			// Invalidate RSS links list
			queryClient.invalidateQueries({ queryKey: rssKeys.links() });
			// Also invalidate feed stats since new feed was added
			queryClient.invalidateQueries({ queryKey: feedKeys.stats() });
		},
	}));
}

// =============================================================================
// Delete RSS Feed Mutation
// =============================================================================

interface DeleteContext {
	previousLinks?: ListRSSFeedLinksResult;
}

/**
 * Mutation for deleting an RSS feed link with optimistic update
 */
export function createDeleteRSSMutation() {
	const queryClient = useQueryClient();
	const transport = createClientTransport();

	return createMutation(() => ({
		mutationFn: (id: string) => deleteRSSFeedLink(transport, id),
		onMutate: async (id: string) => {
			// Cancel any outgoing refetches
			await queryClient.cancelQueries({ queryKey: rssKeys.links() });

			// Snapshot previous value
			const previousLinks = queryClient.getQueryData<ListRSSFeedLinksResult>(
				rssKeys.links(),
			);

			// Optimistically update: remove from list
			if (previousLinks) {
				queryClient.setQueryData(rssKeys.links(), {
					...previousLinks,
					links: previousLinks.links.filter(
						(link: RSSFeedLink) => link.id !== id,
					),
				});
			}

			return { previousLinks } as DeleteContext;
		},
		onError: (_err: Error, _id: string, context: DeleteContext | undefined) => {
			// Rollback on error
			if (context?.previousLinks) {
				queryClient.setQueryData(rssKeys.links(), context.previousLinks);
			}
		},
		onSettled: () => {
			// Refetch to ensure consistency
			queryClient.invalidateQueries({ queryKey: rssKeys.links() });
			queryClient.invalidateQueries({ queryKey: feedKeys.stats() });
		},
	}));
}

// =============================================================================
// Register Favorite Feed Mutation
// =============================================================================

/**
 * Mutation for registering a feed as favorite
 */
export function createRegisterFavoriteMutation() {
	const queryClient = useQueryClient();
	const transport = createClientTransport();

	return createMutation(() => ({
		mutationFn: (url: string) => registerFavoriteFeed(transport, url),
		onSuccess: () => {
			// Invalidate favorites list
			queryClient.invalidateQueries({ queryKey: feedKeys.favorites() });
		},
	}));
}
