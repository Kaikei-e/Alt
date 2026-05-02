/**
 * Feed listing and search functions via Connect-RPC
 */

import type { Transport } from "@connectrpc/connect";
import type {
	GetUnreadFeedsResponse,
	GetAllFeedsResponse,
	GetReadFeedsResponse,
	GetFavoriteFeedsResponse,
	SearchFeedsResponse,
} from "$lib/gen/alt/feeds/v2/feeds_pb";
import {
	createFeedClient,
	convertProtoFeed,
	type FeedCursorResponse,
	type FeedSearchResponse,
} from "./client";

// Cap unary feed reads at 5s. The BFF derives its backend deadline from the
// Connect-Timeout-Ms header; without it the call hangs against a stale upstream
// during a backend rolling restart and surfaces as 502 to the user.
const UNARY_FEED_TIMEOUT_MS = 5000;

/**
 * Get unread feeds with cursor-based pagination via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @param cursor - Optional cursor for pagination (RFC3339 timestamp)
 * @param limit - Maximum number of items to return (default: 20)
 * @param view - Optional view mode ("swipe" for single-card response)
 * @returns Unread feeds with pagination info
 */
export async function getUnreadFeeds(
	transport: Transport,
	cursor?: string,
	limit: number = 20,
	view?: "swipe",
	excludeFeedLinkIds?: string[],
): Promise<FeedCursorResponse> {
	const client = createFeedClient(transport);
	const response = (await client.getUnreadFeeds(
		{
			cursor,
			limit,
			view,
			excludeFeedLinkIds: excludeFeedLinkIds ?? [],
		},
		{ timeoutMs: UNARY_FEED_TIMEOUT_MS },
	)) as GetUnreadFeedsResponse;

	return {
		data: response.data.map(convertProtoFeed),
		nextCursor: response.nextCursor ?? null,
		hasMore: response.hasMore,
	};
}

/**
 * Get all feeds (read + unread) with cursor-based pagination via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @param cursor - Optional cursor for pagination (RFC3339 timestamp)
 * @param limit - Maximum number of items to return (default: 20)
 * @returns All feeds with pagination info
 */
export async function getAllFeeds(
	transport: Transport,
	cursor?: string,
	limit: number = 20,
	excludeFeedLinkIds?: string[],
): Promise<FeedCursorResponse> {
	const client = createFeedClient(transport);
	const response = (await client.getAllFeeds(
		{
			cursor,
			limit,
			excludeFeedLinkIds: excludeFeedLinkIds ?? [],
		},
		{ timeoutMs: UNARY_FEED_TIMEOUT_MS },
	)) as GetAllFeedsResponse;

	return {
		data: response.data.map(convertProtoFeed),
		nextCursor: response.nextCursor ?? null,
		hasMore: response.hasMore,
	};
}

/**
 * Get read/viewed feeds with cursor-based pagination via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @param cursor - Optional cursor for pagination (RFC3339 timestamp)
 * @param limit - Maximum number of items to return (default: 32)
 * @returns Read feeds with pagination info
 */
export async function getReadFeeds(
	transport: Transport,
	cursor?: string,
	limit: number = 32,
): Promise<FeedCursorResponse> {
	const client = createFeedClient(transport);
	const response = (await client.getReadFeeds(
		{
			cursor,
			limit,
		},
		{ timeoutMs: UNARY_FEED_TIMEOUT_MS },
	)) as GetReadFeedsResponse;

	return {
		data: response.data.map(convertProtoFeed),
		nextCursor: response.nextCursor ?? null,
		hasMore: response.hasMore,
	};
}

/**
 * Get favorite feeds with cursor-based pagination via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @param cursor - Optional cursor for pagination (RFC3339 timestamp)
 * @param limit - Maximum number of items to return (default: 20)
 * @returns Favorite feeds with pagination info
 */
export async function getFavoriteFeeds(
	transport: Transport,
	cursor?: string,
	limit: number = 20,
): Promise<FeedCursorResponse> {
	const client = createFeedClient(transport);
	const response = (await client.getFavoriteFeeds(
		{
			cursor,
			limit,
		},
		{ timeoutMs: UNARY_FEED_TIMEOUT_MS },
	)) as GetFavoriteFeedsResponse;

	return {
		data: response.data.map(convertProtoFeed),
		nextCursor: response.nextCursor ?? null,
		hasMore: response.hasMore,
	};
}

/**
 * Search feeds with offset-based pagination via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @param query - Search query string
 * @param cursor - Optional offset cursor for pagination
 * @param limit - Maximum number of items to return (default: 20)
 * @returns Search results with pagination info
 */
export async function searchFeeds(
	transport: Transport,
	query: string,
	cursor?: number,
	limit: number = 20,
): Promise<FeedSearchResponse> {
	const client = createFeedClient(transport);
	const response = (await client.searchFeeds({
		query,
		cursor,
		limit,
	})) as SearchFeedsResponse;

	return {
		data: response.data.map(convertProtoFeed),
		nextCursor: response.nextCursor ?? null,
		hasMore: response.hasMore,
	};
}
