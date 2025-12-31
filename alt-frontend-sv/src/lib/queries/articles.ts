/**
 * TanStack Query hooks for ArticleService Connect-RPC client
 *
 * Provides reactive query and mutation wrappers with:
 * - Content fetch with caching
 * - Archive mutation
 * - Cursor-based article listing
 */

import {
	createQuery,
	createMutation,
	createInfiniteQuery,
	useQueryClient,
} from "@tanstack/svelte-query";
import { createClientTransport } from "$lib/connect/transport.client";
import {
	fetchArticleContent,
	archiveArticle,
	fetchArticlesCursor,
	type FetchArticleContentResult,
	type ArchiveArticleResult,
	type ArticleCursorResponse,
	type ConnectArticleItem,
} from "$lib/connect/articles";
import { articleKeys } from "./keys";

// =============================================================================
// Article Content Query
// =============================================================================

/**
 * Query for fetching article content
 * Uses 30-minute cache since article content rarely changes
 */
export function createFetchArticleContentQuery(
	url: string,
	enabled: boolean = true,
) {
	const transport = createClientTransport();
	return createQuery(() => ({
		queryKey: articleKeys.content(url),
		queryFn: () => fetchArticleContent(transport, url),
		enabled: enabled && url.length > 0,
		staleTime: 1000 * 60 * 30, // 30 minutes
		gcTime: 1000 * 60 * 60, // 1 hour garbage collection
	}));
}

// =============================================================================
// Archive Article Mutation
// =============================================================================

/**
 * Mutation for archiving an article
 */
export function createArchiveArticleMutation() {
	const queryClient = useQueryClient();
	const transport = createClientTransport();

	return createMutation(() => ({
		mutationFn: ({ url, title }: { url: string; title?: string }) =>
			archiveArticle(transport, url, title),
		onSuccess: () => {
			// Invalidate article list to include newly archived article
			queryClient.invalidateQueries({ queryKey: articleKeys.list() });
		},
	}));
}

// =============================================================================
// Articles Cursor Query
// =============================================================================

/**
 * Infinite query for articles with cursor-based pagination
 */
export function createArticlesCursorQuery(limit: number = 20) {
	const transport = createClientTransport();
	return createInfiniteQuery(() => ({
		queryKey: articleKeys.list(),
		queryFn: ({ pageParam }: { pageParam: string | undefined }) =>
			fetchArticlesCursor(transport, pageParam, limit),
		getNextPageParam: (lastPage: ArticleCursorResponse) =>
			lastPage.hasMore ? lastPage.nextCursor ?? undefined : undefined,
		initialPageParam: undefined as string | undefined,
	}));
}

// =============================================================================
// Helper: Flatten pages to single array
// =============================================================================

/**
 * Helper to flatten infinite query pages into a single array
 */
export function flattenArticlePages(
	data: { pages: ArticleCursorResponse[] } | undefined,
): ConnectArticleItem[] {
	if (!data?.pages) return [];
	return data.pages.flatMap((page) => page.data);
}
