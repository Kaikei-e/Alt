/**
 * Tag Trail client — Connect-RPC only.
 *
 * All Tag Trail endpoints go through the SvelteKit /api/v2 proxy →
 * alt-butterfly-facade → alt-backend. The previous REST paths
 * (/feeds/random, /articles/by-tag, /articles/:id/tags, /feeds/:id/tags)
 * are no longer consumed from this file.
 */

import type {
	RandomFeedResponse,
	TagTrailArticle,
	TagTrailTag,
} from "$lib/schema/tagTrail";
import { createClientTransport } from "$lib/connect/transport.client";

/**
 * Get a random subscription feed for Tag Trail.
 * Uses RSSService.RandomSubscription.
 */
export async function getRandomSubscriptionClient(): Promise<RandomFeedResponse> {
	const transport = createClientTransport();
	const { randomSubscription } = await import("$lib/connect/rss");
	const feed = await randomSubscription(transport);
	if (!feed.id) {
		return { feed: null };
	}
	return {
		feed: {
			id: feed.id,
			url: feed.link,
			title: feed.title || undefined,
			description: feed.description || undefined,
		},
	};
}

/**
 * Get articles by tag for Tag Trail.
 * Uses ArticleService.FetchArticlesByTag.
 * @param tagName - Tag name to search across all feeds (preferred)
 * @param tagId - Tag ID for backward compatibility (fallback if tagName not provided)
 * @param cursor - Pagination cursor (RFC3339)
 * @param limit - Number of results per page
 */
export async function getArticlesByTagClient(
	tagName: string,
	tagId?: string,
	cursor?: string,
	limit = 20,
): Promise<{
	articles: TagTrailArticle[];
	nextCursor?: string;
	hasMore: boolean;
}> {
	const transport = createClientTransport();
	const { fetchArticlesByTag } = await import("$lib/connect/articles");
	const result = await fetchArticlesByTag(
		transport,
		tagName || undefined,
		tagId,
		cursor,
		limit,
	);
	return {
		articles: result.articles,
		nextCursor: result.nextCursor ?? undefined,
		hasMore: result.hasMore,
	};
}

/**
 * Get tags for a specific article.
 * Uses ArticleService.FetchArticleTags.
 */
export async function getArticleTagsClient(
	articleId: string,
): Promise<TagTrailTag[]> {
	const transport = createClientTransport();
	const { fetchArticleTags } = await import("$lib/connect/articles");
	const tags = await fetchArticleTags(transport, articleId);
	return tags.map((tag) => ({ id: tag.id, name: tag.name }));
}

/**
 * Get tags for a specific feed by ID.
 * Uses FeedService.GetFeedTags.
 * Used by Tag Trail to get tags for a random feed.
 */
export async function getFeedTagsByIdClient(
	feedId: string,
	limit = 20,
): Promise<TagTrailTag[]> {
	const transport = createClientTransport();
	const { getFeedTags } = await import("$lib/connect/feeds/tags");
	return getFeedTags(transport, feedId, limit);
}
