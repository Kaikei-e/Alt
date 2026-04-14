/**
 * Connect-RPC wrapper for FeedService.GetFeedTags.
 * Replaces GET /v1/feeds/:id/tags for the Tag Trail feature.
 */

import type { Transport } from "@connectrpc/connect";
import type { GetFeedTagsResponse } from "$lib/gen/alt/feeds/v2/feeds_pb";
import { createFeedClient } from "./client";

export interface FeedTag {
	id: string;
	name: string;
}

/**
 * Fetches tags attached to a single feed by ID.
 *
 * @param transport - The Connect transport to use
 * @param feedId - UUID of the feed
 * @param limit - Maximum tags to return (default 20)
 * @returns List of tags sorted by the server's ordering
 */
export async function getFeedTags(
	transport: Transport,
	feedId: string,
	limit = 20,
): Promise<FeedTag[]> {
	const client = createFeedClient(transport);
	const response = (await client.getFeedTags({
		feedId,
		limit,
	})) as GetFeedTagsResponse;

	return response.tags.map((tag) => ({
		id: tag.id,
		name: tag.name,
	}));
}
