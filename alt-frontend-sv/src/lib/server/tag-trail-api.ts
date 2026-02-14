import { callBackendAPI } from "./backend-rest-client";

export interface RandomFeedResponse {
	feed: {
		id: string;
		url: string;
		title?: string;
		description?: string;
	} | null;
}

export interface ArticlesByTagResponse {
	articles: Array<{
		id: string;
		title: string;
		link: string;
		published_at: string;
		feed_title?: string;
	}>;
	next_cursor?: string;
	has_more: boolean;
}

export interface ArticleTagsResponse {
	article_id: string;
	tags: Array<{
		id: string;
		name: string;
		created_at: string;
	}>;
}

export interface FeedTagsResponse {
	feed_id: string;
	tags: Array<{
		id: string;
		name: string;
		created_at: string;
	}>;
	next_cursor?: string;
}

/**
 * Get a random subscription feed for Tag Trail (server-side)
 */
export async function getRandomSubscription(
	cookie: string | null,
): Promise<RandomFeedResponse> {
	return callBackendAPI<RandomFeedResponse>("/v1/rss-feed-link/random", cookie);
}

/**
 * Get articles by tag ID for Tag Trail (server-side)
 */
export async function getArticlesByTag(
	cookie: string | null,
	tagId: string,
	cursor?: string,
	limit = 20,
): Promise<ArticlesByTagResponse> {
	const params = new URLSearchParams({
		tag_id: tagId,
		limit: limit.toString(),
	});
	if (cursor) {
		params.append("cursor", cursor);
	}
	return callBackendAPI<ArticlesByTagResponse>(
		`/v1/articles/by-tag?${params.toString()}`,
		cookie,
	);
}

/**
 * Get tags for a specific article (server-side)
 */
export async function getArticleTags(
	cookie: string | null,
	articleId: string,
): Promise<ArticleTagsResponse> {
	return callBackendAPI<ArticleTagsResponse>(
		`/v1/articles/${articleId}/tags`,
		cookie,
	);
}

/**
 * Get tags for a specific feed by ID (server-side)
 * Used by Tag Trail to get tags for a random feed
 */
export async function getFeedTagsById(
	cookie: string | null,
	feedId: string,
	limit = 20,
): Promise<FeedTagsResponse> {
	return callBackendAPI<FeedTagsResponse>(
		`/v1/feeds/${feedId}/tags?limit=${limit}`,
		cookie,
	);
}
