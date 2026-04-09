import { createClientTransport } from "$lib/connect/transport.client";

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
	article_id: string;
	og_image_url: string;
	og_image_proxy_url: string;
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
 * Connect-RPCを使用
 */
export async function getArticleSummaryClient(
	feedUrl: string,
): Promise<FetchArticleSummaryResponse> {
	const transport = createClientTransport();
	const { fetchArticleSummary } = await import("$lib/connect/articles");
	const response = await fetchArticleSummary(transport, [feedUrl]);

	// Convert camelCase to snake_case for API compatibility
	return {
		matched_articles: response.matchedArticles.map((item) => ({
			article_url: feedUrl,
			title: item.title,
			author: item.author || undefined,
			content: item.content,
			content_type: "text/html",
			published_at: item.publishedAt,
			fetched_at: item.fetchedAt,
			source_id: item.sourceId,
		})),
		total_matched: response.totalMatched,
		requested_count: response.requestedCount,
	};
}

/**
 * Get feed content on-the-fly (クライアントサイド)
 * Connect-RPC を使用
 */
export async function getFeedContentOnTheFlyClient(
	feedUrl: string,
	options?: { signal?: AbortSignal; forceRefresh?: boolean },
): Promise<FeedContentOnTheFlyResponse> {
	const transport = createClientTransport();
	const { fetchArticleContent } = await import("$lib/connect/articles");
	const response = await fetchArticleContent(
		transport,
		feedUrl,
		options?.signal,
		options?.forceRefresh,
	);

	return {
		content: response.content,
		article_id: response.articleId,
		og_image_url: response.ogImageUrl,
		og_image_proxy_url: response.ogImageProxyUrl,
	};
}

/**
 * Archive content (クライアントサイド)
 * Connect-RPC を使用
 */
export async function archiveContentClient(
	feedUrl: string,
	title?: string,
): Promise<MessageResponse> {
	const transport = createClientTransport();
	const { archiveArticle } = await import("$lib/connect/articles");
	const response = await archiveArticle(transport, feedUrl, title?.trim());

	return {
		message: response.message,
	};
}

/**
 * Batch prefetch OGP image proxy URLs (クライアントサイド)
 * Connect-RPC を使用
 */
export async function batchPrefetchImagesClient(
	articleIds: string[],
): Promise<{ articleId: string; proxyUrl: string; isCached: boolean }[]> {
	const transport = createClientTransport();
	const { batchPrefetchImages } = await import("$lib/connect/articles");
	return batchPrefetchImages(transport, articleIds);
}

/**
 * Summarize article (クライアントサイド)
 * 内部的にConnect-RPC Server Streamingを使用し、全チャンクを収集してから返す
 */
export async function summarizeArticleClient(
	feedUrl: string,
): Promise<SummarizeArticleResponse> {
	const transport = createClientTransport();
	const { streamSummarize } = await import("$lib/connect/feeds");

	const result = await streamSummarize(transport, { feedUrl });

	return {
		success: true,
		summary: result.summary,
		article_id: result.articleId,
		feed_url: feedUrl,
	};
}

/**
 * Register favorite feed (クライアントサイド)
 * Connect-RPC を使用
 */
export async function registerFavoriteFeedClient(
	url: string,
): Promise<MessageResponse> {
	const transport = createClientTransport();
	const { registerFavoriteFeed } = await import("$lib/connect/rss");
	const response = await registerFavoriteFeed(transport, url);

	return {
		message: response.message,
	};
}

/**
 * Remove favorite feed (クライアントサイド)
 * Connect-RPC を使用
 */
export async function removeFavoriteFeedClient(
	url: string,
): Promise<MessageResponse> {
	const transport = createClientTransport();
	const { removeFavoriteFeed } = await import("$lib/connect/rss");
	const response = await removeFavoriteFeed(transport, url);

	return {
		message: response.message,
	};
}
