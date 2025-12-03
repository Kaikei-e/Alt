import { callClientAPI } from "./core";

/**
 * Safe HTML string type (server-sanitized)
 */
export type SafeHtmlString = string;

/**
 * Article summary item from API
 */
export interface ArticleSummaryItem {
	article_url: string;
	title: string;
	author?: string;
	content: SafeHtmlString;
	content_type: string;
	published_at: string;
	fetched_at: string;
	source_id: string;
}

/**
 * Fetch article summary response
 */
export interface FetchArticleSummaryResponse {
	matched_articles: ArticleSummaryItem[];
	total_matched: number;
	requested_count: number;
}

/**
 * Feed content on-the-fly response
 */
export interface FeedContentOnTheFlyResponse {
	content: SafeHtmlString;
}

/**
 * Message response from API
 */
export interface MessageResponse {
	message: string;
}

/**
 * Summarize article response
 */
export interface SummarizeArticleResponse {
	success: boolean;
	summary: string;
	article_id: string;
	feed_url: string;
}

/**
 * Get article summary (クライアントサイド)
 */
export async function getArticleSummaryClient(
	feedUrl: string,
): Promise<FetchArticleSummaryResponse> {
	return callClientAPI<FetchArticleSummaryResponse>(
		"/v1/articles/summary",
		{
			method: "POST",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify({ feed_urls: [feedUrl] }),
		},
	);
}

/**
 * Get feed content on-the-fly (クライアントサイド)
 */
export async function getFeedContentOnTheFlyClient(
	feedUrl: string,
): Promise<FeedContentOnTheFlyResponse> {
	return callClientAPI<FeedContentOnTheFlyResponse>(
		"/v1/articles/content",
		{
			method: "POST",
			headers: {
				"Content-Type": "application/json",
			},
			body: JSON.stringify({ url: feedUrl }),
		},
	);
}

/**
 * Archive content (クライアントサイド)
 */
export async function archiveContentClient(
	feedUrl: string,
	title?: string,
): Promise<MessageResponse> {
	const payload: Record<string, unknown> = { feed_url: feedUrl };
	if (title?.trim()) {
		payload.title = title.trim();
	}

	return callClientAPI<MessageResponse>("/v1/articles/archive", {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
		},
		body: JSON.stringify(payload),
	});
}

/**
 * Summarize article (クライアントサイド)
 */
export async function summarizeArticleClient(
	feedUrl: string,
): Promise<SummarizeArticleResponse> {
	return callClientAPI<SummarizeArticleResponse>("/v1/feeds/summarize", {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
		},
		body: JSON.stringify({ feed_url: feedUrl }),
	});
}

/**
 * Register favorite feed (クライアントサイド)
 */
export async function registerFavoriteFeedClient(
	url: string,
): Promise<MessageResponse> {
	return callClientAPI<MessageResponse>("/v1/feeds/register/favorite", {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
		},
		body: JSON.stringify({ url }),
	});
}

