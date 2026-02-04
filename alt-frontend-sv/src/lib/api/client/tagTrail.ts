import type {
	ArticlesByTagResponse,
	ArticleTagsResponse,
	FeedTagsResponse,
	RandomFeedResponse,
	TagTrailArticle,
	TagTrailTag,
} from "$lib/schema/tagTrail";
import { callClientAPI } from "./core";

/**
 * Get a random subscription feed for Tag Trail
 * Uses REST API: GET /v1/rss-feed-link/random
 */
export async function getRandomSubscriptionClient(): Promise<RandomFeedResponse> {
	return callClientAPI<RandomFeedResponse>("/feeds/random", {
		method: "GET",
	});
}

/**
 * Get articles by tag for Tag Trail
 * Uses REST API: GET /v1/articles/by-tag
 * @param tagName - Tag name to search across all feeds (preferred)
 * @param tagId - Tag ID for backward compatibility (fallback if tagName not provided)
 * @param cursor - Pagination cursor
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
	const params = new URLSearchParams({ limit: limit.toString() });
	// Prioritize tag_name for cross-feed discovery
	if (tagName) {
		params.append("tag_name", tagName);
	} else if (tagId) {
		params.append("tag_id", tagId);
	}
	if (cursor) {
		params.append("cursor", cursor);
	}

	const response = await callClientAPI<ArticlesByTagResponse>(
		`/articles/by-tag?${params.toString()}`,
		{ method: "GET" },
	);

	// Convert snake_case to camelCase
	return {
		articles: response.articles.map((article) => ({
			id: article.id,
			title: article.title,
			link: article.link,
			publishedAt: article.published_at,
			feedTitle: article.feed_title,
		})),
		nextCursor: response.next_cursor,
		hasMore: response.has_more,
	};
}

/**
 * Get tags for a specific article
 * Uses REST API: GET /v1/articles/{id}/tags
 */
export async function getArticleTagsClient(
	articleId: string,
): Promise<TagTrailTag[]> {
	const response = await callClientAPI<ArticleTagsResponse>(
		`/articles/${articleId}/tags`,
		{ method: "GET" },
	);

	return response.tags.map((tag) => ({
		id: tag.id,
		name: tag.name,
	}));
}

/**
 * Get tags for a specific feed by ID
 * Uses REST API: GET /v1/feeds/{id}/tags
 * Used by Tag Trail to get tags for a random feed
 */
export async function getFeedTagsByIdClient(
	feedId: string,
	limit = 20,
): Promise<TagTrailTag[]> {
	const response = await callClientAPI<FeedTagsResponse>(
		`/feeds/${feedId}/tags?limit=${limit}`,
		{ method: "GET" },
	);

	return response.tags.map((tag) => ({
		id: tag.id,
		name: tag.name,
	}));
}
