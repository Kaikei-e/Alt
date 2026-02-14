/**
 * Feed action functions via Connect-RPC
 * Mark as read, subscription management
 */

import type { Transport } from "@connectrpc/connect";
import type {
	MarkAsReadResponse,
	ListSubscriptionsResponse,
	SubscribeResponse,
	UnsubscribeResponse,
	FeedSource,
} from "$lib/gen/alt/feeds/v2/feeds_pb";
import { createFeedClient, normalizeUrl } from "./client";

// =============================================================================
// Mark As Read
// =============================================================================

/**
 * Result of marking a feed as read
 */
export interface MarkAsReadResult {
	message: string;
}

/**
 * Marks a feed/article as read via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @param articleUrl - The URL of the feed/article to mark as read
 * @returns The success message
 */
export async function markAsRead(
	transport: Transport,
	articleUrl: string,
): Promise<MarkAsReadResult> {
	// Normalize URL to remove trailing slash (zero-trust: ensure consistency)
	const normalizedUrl = normalizeUrl(articleUrl);

	const client = createFeedClient(transport);
	const response = (await client.markAsRead({
		articleUrl: normalizedUrl,
	})) as MarkAsReadResponse;

	return {
		message: response.message,
	};
}

// =============================================================================
// Subscription Management
// =============================================================================

/**
 * Feed source with subscription status
 */
export interface ConnectFeedSource {
	id: string;
	url: string;
	title: string;
	isSubscribed: boolean;
	createdAt: string;
}

/**
 * List all feed sources with subscription status for the current user.
 *
 * @param transport - The Connect transport to use
 * @returns All feed sources with subscription status
 */
export async function listSubscriptions(
	transport: Transport,
): Promise<ConnectFeedSource[]> {
	const client = createFeedClient(transport);
	const response = (await client.listSubscriptions(
		{},
	)) as ListSubscriptionsResponse;

	return response.sources.map((source: FeedSource) => ({
		id: source.id,
		url: source.url,
		title: source.title,
		isSubscribed: source.isSubscribed,
		createdAt: source.createdAt,
	}));
}

/**
 * Subscribe the current user to a feed source.
 *
 * @param transport - The Connect transport to use
 * @param feedLinkId - The feed link ID to subscribe to
 * @returns Success message
 */
export async function subscribe(
	transport: Transport,
	feedLinkId: string,
): Promise<string> {
	const client = createFeedClient(transport);
	const response = (await client.subscribe({
		feedLinkId,
	})) as SubscribeResponse;

	return response.message;
}

/**
 * Unsubscribe the current user from a feed source.
 *
 * @param transport - The Connect transport to use
 * @param feedLinkId - The feed link ID to unsubscribe from
 * @returns Success message
 */
export async function unsubscribe(
	transport: Transport,
	feedLinkId: string,
): Promise<string> {
	const client = createFeedClient(transport);
	const response = (await client.unsubscribe({
		feedLinkId,
	})) as UnsubscribeResponse;

	return response.message;
}
