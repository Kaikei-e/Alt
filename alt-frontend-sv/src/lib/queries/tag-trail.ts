import { createQuery, createInfiniteQuery } from "@tanstack/svelte-query";
import { createClientTransport } from "$lib/connect/transport.client";
import {
	fetchArticlesByTag,
	fetchArticleTags,
	fetchRandomFeed,
	type TagTrailArticlesResponse,
} from "$lib/connect/articles";
import { tagTrailKeys } from "./keys";

export function createArticlesByTagQuery(
	tagName?: string,
	tagId?: string,
	limit = 20,
	enabled = true,
) {
	const transport = createClientTransport();
	return createInfiniteQuery(() => ({
		queryKey: tagTrailKeys.articlesByTag(tagName, tagId),
		queryFn: ({ pageParam }: { pageParam: string | undefined }) =>
			fetchArticlesByTag(transport, tagName, tagId, pageParam, limit),
		getNextPageParam: (lastPage: TagTrailArticlesResponse) =>
			lastPage.hasMore ? (lastPage.nextCursor ?? undefined) : undefined,
		initialPageParam: undefined as string | undefined,
		enabled: enabled && !!(tagName || tagId),
	}));
}

export function createArticleTagsQuery(articleId: string, enabled = true) {
	const transport = createClientTransport();
	return createQuery(() => ({
		queryKey: tagTrailKeys.articleTags(articleId),
		queryFn: () => fetchArticleTags(transport, articleId),
		enabled: enabled && articleId.length > 0,
		staleTime: 1000 * 60 * 15,
	}));
}

export function createRandomFeedQuery() {
	const transport = createClientTransport();
	return createQuery(() => ({
		queryKey: tagTrailKeys.randomFeed(),
		queryFn: () => fetchRandomFeed(transport),
		staleTime: 0,
	}));
}
