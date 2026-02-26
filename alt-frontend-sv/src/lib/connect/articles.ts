/**
 * ArticleService client for Connect-RPC
 *
 * Provides type-safe methods to call ArticleService endpoints.
 */

import { createClient } from "@connectrpc/connect";
import type { Client, Transport } from "@connectrpc/connect";
import {
	ArticleService,
	ArticleTagEvent_EventType,
	type ArticleTagEvent,
	type FetchArticleContentResponse,
	type ArchiveArticleResponse,
	type FetchArticlesCursorResponse,
	type FetchArticleSummaryResponse,
	type ArticleSummaryItem as ProtoArticleSummaryItem,
	type FetchArticlesByTagResponse,
	type FetchArticleTagsResponse,
	type FetchRandomFeedResponse,
	type TagTrailArticleItem as ProtoTagTrailArticleItem,
	type ArticleTagItem as ProtoArticleTagItem,
} from "$lib/gen/alt/articles/v2/articles_pb";

/** Type-safe ArticleService client */
type ArticleClient = Client<typeof ArticleService>;

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
	ogImageUrl: string;
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
// Tag Trail Types (ADR-167, ADR-168, ADR-169)
// =============================================================================

/**
 * Tag Trail article item
 * Note: feedTitle is optional for compatibility with schema TagTrailArticle
 */
export interface TagTrailArticle {
	id: string;
	title: string;
	link: string;
	publishedAt: string;
	feedTitle?: string;
}

/**
 * Tag Trail tag item
 * Note: createdAt is optional for compatibility with schema TagTrailTag
 */
export interface TagTrailTag {
	id: string;
	name: string;
	createdAt?: string;
}

/**
 * Random feed response
 * ADR-173: Includes tags for the feed's latest article (generated on-the-fly if not in DB)
 */
export interface RandomFeed {
	id: string;
	url: string;
	title: string;
	description: string;
	tags: TagTrailTag[];
}

/**
 * Tag Trail articles response with pagination
 */
export interface TagTrailArticlesResponse {
	articles: TagTrailArticle[];
	nextCursor: string | null;
	hasMore: boolean;
}

// =============================================================================
// Client Factory
// =============================================================================

/**
 * Creates an ArticleService client with the given transport.
 */
export function createArticleClient(transport: Transport): ArticleClient {
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
	const response = (await client.fetchArticleContent(
		{ url },
		signal ? { signal } : undefined,
	)) as FetchArticleContentResponse;

	return {
		url: response.url,
		content: response.content,
		articleId: response.articleId,
		ogImageUrl: response.ogImageUrl,
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
	const response = (await client.archiveArticle({
		feedUrl,
		title,
	})) as ArchiveArticleResponse;

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
	const response = (await client.fetchArticlesCursor({
		cursor,
		limit,
	})) as FetchArticlesCursorResponse;

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
	const response = (await client.fetchArticleSummary({
		feedUrls,
	})) as FetchArticleSummaryResponse;

	return {
		matchedArticles: response.matchedArticles.map(
			(item: ProtoArticleSummaryItem) => ({
				title: item.title,
				content: item.content,
				author: item.author,
				publishedAt: item.publishedAt,
				fetchedAt: item.fetchedAt,
				sourceId: item.sourceId,
			}),
		),
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

// =============================================================================
// Tag Trail API Functions (ADR-167, ADR-168, ADR-169)
// =============================================================================

/**
 * Fetches articles by tag (ID or name) via Connect-RPC.
 * ADR-169: tag_name で横断検索、tag_id は後方互換性
 *
 * @param transport - The Connect transport to use
 * @param tagName - Tag name for cross-feed discovery (preferred)
 * @param tagId - Tag ID for backward compatibility
 * @param cursor - Optional cursor for pagination (RFC3339 timestamp)
 * @param limit - Maximum number of items to return (default: 20)
 * @returns Articles with pagination info
 */
export async function fetchArticlesByTag(
	transport: Transport,
	tagName?: string,
	tagId?: string,
	cursor?: string,
	limit = 20,
): Promise<TagTrailArticlesResponse> {
	const client = createArticleClient(transport);
	const response = (await client.fetchArticlesByTag({
		tagName,
		tagId,
		cursor,
		limit,
	})) as FetchArticlesByTagResponse;

	return {
		articles: response.articles.map(
			(item: ProtoTagTrailArticleItem): TagTrailArticle => ({
				id: item.id,
				title: item.title,
				link: item.link,
				publishedAt: item.publishedAt,
				feedTitle: item.feedTitle,
			}),
		),
		nextCursor: response.nextCursor ?? null,
		hasMore: response.hasMore,
	};
}

/**
 * Fetches tags for an article via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @param articleId - The article ID to fetch tags for
 * @returns Tags for the article
 */
export async function fetchArticleTags(
	transport: Transport,
	articleId: string,
): Promise<TagTrailTag[]> {
	const client = createArticleClient(transport);
	const response = (await client.fetchArticleTags({
		articleId,
	})) as FetchArticleTagsResponse;

	return response.tags.map(
		(item: ProtoArticleTagItem): TagTrailTag => ({
			id: item.id,
			name: item.name,
			createdAt: item.createdAt,
		}),
	);
}

/**
 * Fetches a random feed for Tag Trail via Connect-RPC.
 * ADR-173: Response includes tags for the feed's latest article.
 *
 * @param transport - The Connect transport to use
 * @returns A random feed with tags
 */
export async function fetchRandomFeed(
	transport: Transport,
): Promise<RandomFeed> {
	const client = createArticleClient(transport);
	const response = (await client.fetchRandomFeed(
		{},
	)) as FetchRandomFeedResponse;

	return {
		id: response.id,
		url: response.url,
		title: response.title,
		description: response.description,
		tags: (response.tags || []).map(
			(item: ProtoArticleTagItem): TagTrailTag => ({
				id: item.id,
				name: item.name,
				createdAt: item.createdAt,
			}),
		),
	};
}

// =============================================================================
// Streaming Tag Trail API Functions
// =============================================================================

/**
 * Event types for article tag streaming
 */
export type ArticleTagEventType =
	| "cached"
	| "generating"
	| "completed"
	| "error";

/**
 * Streaming tag event payload
 */
export interface StreamingArticleTagEvent {
	articleId: string;
	tags: TagTrailTag[];
	eventType: ArticleTagEventType;
	message?: string;
}

/**
 * Maps proto EventType to string literal for easier handling.
 */
function mapEventType(
	protoEventType: ArticleTagEvent_EventType,
): ArticleTagEventType {
	switch (protoEventType) {
		case ArticleTagEvent_EventType.CACHED:
			return "cached";
		case ArticleTagEvent_EventType.GENERATING:
			return "generating";
		case ArticleTagEvent_EventType.COMPLETED:
			return "completed";
		case ArticleTagEvent_EventType.ERROR:
			return "error";
		default:
			return "error";
	}
}

/**
 * Streams article tag updates via Connect-RPC Server Streaming.
 *
 * Returns cached tags immediately if available, otherwise streams
 * generation progress with heartbeats.
 *
 * @param transport - The Connect transport to use
 * @param articleId - The article ID to stream tags for
 * @param onEvent - Callback for each streaming event
 * @param onError - Optional error callback
 * @returns AbortController to cancel the stream
 *
 * @example
 * ```typescript
 * const abortController = streamArticleTags(
 *   transport,
 *   articleId,
 *   (event) => {
 *     if (event.eventType === 'cached' || event.eventType === 'completed') {
 *       setTags(event.tags);
 *     } else if (event.eventType === 'generating') {
 *       setIsGenerating(true);
 *     } else if (event.eventType === 'error') {
 *       setError(event.message);
 *     }
 *   },
 *   (error) => console.error('Stream error:', error),
 * );
 *
 * // On cleanup
 * onDestroy(() => abortController.abort());
 * ```
 */
export function streamArticleTags(
	transport: Transport,
	articleId: string,
	onEvent: (event: StreamingArticleTagEvent) => void,
	onError?: (error: Error) => void,
): AbortController {
	const client = createArticleClient(transport);
	const abortController = new AbortController();

	// Start streaming in background
	(async () => {
		try {
			const stream = client.streamArticleTags(
				{ articleId },
				{ signal: abortController.signal },
			);

			for await (const rawEvent of stream) {
				const event = rawEvent as ArticleTagEvent;
				const tags: TagTrailTag[] = event.tags.map(
					(item: ProtoArticleTagItem): TagTrailTag => ({
						id: item.id,
						name: item.name,
						createdAt: item.createdAt,
					}),
				);

				onEvent({
					articleId: event.articleId,
					tags,
					eventType: mapEventType(event.eventType),
					message: event.message,
				});
			}
		} catch (error) {
			// Check abort BEFORE logging to suppress navigation-related errors
			if (abortController.signal.aborted) {
				return;
			}
			if (onError && error instanceof Error) {
				onError(error);
			}
		}
	})();

	return abortController;
}
