/**
 * Connect-RPC wrapper for FeedService.ResolveOgImages.
 *
 * Feeds whose RSS carried no og:image are resolved when a reader brings the
 * card into view, rather than by a scheduled sweep of publisher pages.
 */

import type { Transport } from "@connectrpc/connect";
import type { ResolveOgImagesResponse } from "$lib/gen/alt/feeds/v2/feeds_pb";
import { createFeedClient } from "./client";

/**
 * Resolves og:image proxy URLs for feeds that arrived without one.
 *
 * Feeds the server could not resolve are absent from the returned map rather
 * than mapped to an empty string — the caller must be able to tell "we were
 * refused" from "we hold a blank image", because the first one must never be
 * asked again and the second is not a thing that exists.
 *
 * @param transport - The Connect transport to use
 * @param feedIds - UUIDs of the feeds now visible to the reader
 * @returns feed id → signed image-proxy URL, for the feeds that resolved
 */
export async function resolveOgImages(
	transport: Transport,
	feedIds: string[],
): Promise<Map<string, string>> {
	if (feedIds.length === 0) {
		return new Map();
	}

	const client = createFeedClient(transport);
	const response = (await client.resolveOgImages({
		feedIds,
	})) as ResolveOgImagesResponse;

	const resolved = new Map<string, string>();
	for (const image of response.images) {
		if (image.feedId && image.ogImageProxyUrl) {
			resolved.set(image.feedId, image.ogImageProxyUrl);
		}
	}
	return resolved;
}
