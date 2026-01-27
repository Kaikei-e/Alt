/**
 * ArticleService client for Connect-RPC
 *
 * Provides type-safe methods to call ArticleService endpoints.
 */

import { createClient } from "@connectrpc/connect";
import type { Transport } from "@connectrpc/connect";
import { ArticleService } from "$lib/gen/alt/articles/v2/articles_pb";

// =============================================================================
// Types
// =============================================================================

/**
 * Article content response
 */
export interface FetchArticleContentResult {
	url: string;
	content: string;
	articleId: string;
}

/**
 * Archive article response
 */
export interface ArchiveArticleResult {
	message: string;
}

/**
 * Article item from Connect-RPC (converted from proto)
 */
export interface ConnectArticleItem {
	id: string;
	title: string;
	url: string;
	content: string;
	publishedAt: string;
	tags: string[];
}

/**
 * Cursor response for articles
 */
export interface ArticleCursorResponse {
	data: ConnectArticleItem[];
	nextCursor: string | null;
	hasMore: boolean;
}

/**
 * Article summary item
 */
export interface ArticleSummaryItem {
	title: string;
	content: string;
	author: string;
	publishedAt: string;
	fetchedAt: string;
	sourceId: string;
}

/**
 * Article summary response
 */
export interface FetchArticleSummaryResult {
	matchedArticles: ArticleSummaryItem[];
	totalMatched: number;
	requestedCount: number;
}

// =============================================================================
// Client Factory
// =============================================================================

/**
 * Creates an ArticleService client with the given transport.
 */
export function createArticleClient(transport: Transport) {
	return createClient(ArticleService, transport);
}

// =============================================================================
// API Functions
// =============================================================================

/**
 * Fetches and extracts compliant article content via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @param url - The article URL to fetch
 * @param signal - Optional AbortSignal to cancel the request
 * @returns The fetched article content
 */
export async function fetchArticleContent(
	transport: Transport,
	url: string,
	signal?: AbortSignal,
): Promise<FetchArticleContentResult> {
	const client = createArticleClient(transport);
	const response = await client.fetchArticleContent(
		{ url },
		signal ? { signal } : undefined,
	);

	return {
		url: response.url,
		content: response.content,
		articleId: response.articleId,
	};
}

/**
 * Archives an article for later reading via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @param feedUrl - The article URL to archive
 * @param title - Optional title override
 * @returns The archive result message
 */
export async function archiveArticle(
	transport: Transport,
	feedUrl: string,
	title?: string,
): Promise<ArchiveArticleResult> {
	const client = createArticleClient(transport);
	const response = await client.archiveArticle({
		feedUrl,
		title,
	});

	return {
		message: response.message,
	};
}

/**
 * Fetches articles with cursor-based pagination via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @param cursor - Optional cursor for pagination (RFC3339 timestamp)
 * @param limit - Maximum number of items to return (default: 20)
 * @returns Articles with pagination info
 */
export async function fetchArticlesCursor(
	transport: Transport,
	cursor?: string,
	limit: number = 20,
): Promise<ArticleCursorResponse> {
	const client = createArticleClient(transport);
	const response = await client.fetchArticlesCursor({
		cursor,
		limit,
	});

	return {
		data: response.data.map(convertProtoArticle),
		nextCursor: response.nextCursor ?? null,
		hasMore: response.hasMore,
	};
}

/**
 * Fetches article summaries for multiple URLs via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @param feedUrls - List of feed URLs to fetch summaries for (max 50)
 * @returns Article summaries with metadata
 */
export async function fetchArticleSummary(
	transport: Transport,
	feedUrls: string[],
): Promise<FetchArticleSummaryResult> {
	const client = createArticleClient(transport);
	const response = await client.fetchArticleSummary({ feedUrls });

	return {
		matchedArticles: response.matchedArticles.map((item) => ({
			title: item.title,
			content: item.content,
			author: item.author,
			publishedAt: item.publishedAt,
			fetchedAt: item.fetchedAt,
			sourceId: item.sourceId,
		})),
		totalMatched: response.totalMatched,
		requestedCount: response.requestedCount,
	};
}

// =============================================================================
// Helper Functions
// =============================================================================

/**
 * Convert proto ArticleItem to ConnectArticleItem.
 */
function convertProtoArticle(proto: {
	id: string;
	title: string;
	url: string;
	content: string;
	publishedAt: string;
	tags: string[];
}): ConnectArticleItem {
	return {
		id: proto.id,
		title: proto.title,
		url: proto.url,
		content: proto.content,
		publishedAt: proto.publishedAt,
		tags: proto.tags,
	};
}
