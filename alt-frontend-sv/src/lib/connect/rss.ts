/**
 * RSSService client for Connect-RPC
 *
 * Provides type-safe methods to call RSSService endpoints.
 */

import { createClient } from "@connectrpc/connect";
import type { Client, Transport } from "@connectrpc/connect";
import {
	RSSService,
	type RegisterRSSFeedResponse,
	type ListRSSFeedLinksResponse,
	type DeleteRSSFeedLinkResponse,
	type RegisterFavoriteFeedResponse,
	type RSSFeedLink as ProtoRSSFeedLink,
} from "$lib/gen/alt/rss/v2/rss_pb";

/** Type-safe RSSService client */
type RSSClient = Client<typeof RSSService>;

// =============================================================================
// Types
// =============================================================================

/**
 * RSS feed link
 */
export interface RSSFeedLink {
	id: string;
	url: string;
}

/**
 * Register RSS feed response
 */
export interface RegisterRSSFeedResult {
	message: string;
}

/**
 * List RSS feed links response
 */
export interface ListRSSFeedLinksResult {
	links: RSSFeedLink[];
}

/**
 * Delete RSS feed link response
 */
export interface DeleteRSSFeedLinkResult {
	message: string;
}

/**
 * Register favorite feed response
 */
export interface RegisterFavoriteFeedResult {
	message: string;
}

// =============================================================================
// Client Factory
// =============================================================================

/**
 * Creates an RSSService client with the given transport.
 */
export function createRSSClient(transport: Transport): RSSClient {
	return createClient(RSSService, transport);
}

// =============================================================================
// API Functions
// =============================================================================

/**
 * Registers a new RSS feed link via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @param url - The RSS feed URL to register
 * @returns The registration result message
 */
export async function registerRSSFeed(
	transport: Transport,
	url: string,
): Promise<RegisterRSSFeedResult> {
	const client = createRSSClient(transport);
	const response = (await client.registerRSSFeed({
		url,
	})) as RegisterRSSFeedResponse;

	return {
		message: response.message,
	};
}

/**
 * Lists all registered RSS feed links via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @returns The list of registered feed links
 */
export async function listRSSFeedLinks(
	transport: Transport,
): Promise<ListRSSFeedLinksResult> {
	const client = createRSSClient(transport);
	const response = (await client.listRSSFeedLinks(
		{},
	)) as ListRSSFeedLinksResponse;

	return {
		links: response.links.map((link: ProtoRSSFeedLink) => ({
			id: link.id,
			url: link.url,
		})),
	};
}

/**
 * Deletes an RSS feed link via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @param id - The UUID of the feed link to delete
 * @returns The deletion result message
 */
export async function deleteRSSFeedLink(
	transport: Transport,
	id: string,
): Promise<DeleteRSSFeedLinkResult> {
	const client = createRSSClient(transport);
	const response = (await client.deleteRSSFeedLink({
		id,
	})) as DeleteRSSFeedLinkResponse;

	return {
		message: response.message,
	};
}

/**
 * Registers a feed as favorite via Connect-RPC.
 *
 * @param transport - The Connect transport to use
 * @param url - The feed URL to mark as favorite
 * @returns The registration result message
 */
export async function registerFavoriteFeed(
	transport: Transport,
	url: string,
): Promise<RegisterFavoriteFeedResult> {
	const client = createRSSClient(transport);
	const response = (await client.registerFavoriteFeed({
		url,
	})) as RegisterFavoriteFeedResponse;

	return {
		message: response.message,
	};
}
