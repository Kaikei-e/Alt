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
 * One answer from the server, kept as the two lists it arrived in.
 *
 * The server holds four answers about a feed and the wire says which one it
 * means by *membership*, not by a status field: resolved, refused (a bar of
 * zero), failed-for-now (a bar in the future), or never reached (in neither
 * list). Flattening those into one map keyed by presence would collapse the
 * last three into "absent", which is the bug this shape exists to prevent —
 * a card left blank for the session over a failure the server was willing to
 * be asked about again in five seconds.
 */
export interface OgImageResolution {
	/** feed id → signed image-proxy URL, for the feeds that resolved. */
	resolved: Map<string, string>;
	/**
	 * feed id → milliseconds before this feed may be asked about again, for
	 * the feeds the server considered and could not resolve. Zero is a settled
	 * "no" for the retention window, not a licence to ask straight away; a feed
	 * missing from this map entirely is the one the server never reached.
	 */
	unresolved: Map<string, number>;
}

/**
 * Resolves og:image proxy URLs for feeds that arrived without one.
 *
 * Feeds the server could not resolve are absent from `resolved` rather than
 * mapped to an empty string — the caller must be able to tell "we were
 * refused" from "we hold a blank image", because the first one must never be
 * asked again and the second is not a thing that exists.
 *
 * @param transport - The Connect transport to use
 * @param feedIds - UUIDs of the feeds now visible to the reader
 * @returns the two lists, with retry bars already in milliseconds
 */
export async function resolveOgImages(
	transport: Transport,
	feedIds: string[],
): Promise<OgImageResolution> {
	const resolved = new Map<string, string>();
	const unresolved = new Map<string, number>();

	if (feedIds.length === 0) {
		return { resolved, unresolved };
	}

	const client = createFeedClient(transport);
	const response = (await client.resolveOgImages({
		feedIds,
	})) as ResolveOgImagesResponse;

	for (const image of response.images) {
		if (image.feedId && image.ogImageProxyUrl) {
			resolved.set(image.feedId, image.ogImageProxyUrl);
		}
	}

	// `unresolved` is newer than `images`; a server that predates it sends no
	// field at all, and protobuf-es only guarantees the array on messages it
	// decoded itself. Guarding here keeps a hand-built response in a test
	// double from throwing where the real client would not.
	for (const entry of response.unresolved ?? []) {
		if (!entry.feedId) continue;
		// protobuf-es maps int64 to bigint. The conversion to a plain number of
		// milliseconds belongs here, at the edge: every consumer above compares
		// this against millisecond constants, and a bigint mixed into that
		// arithmetic is a TS2365 rather than a wrong answer. Clamped at zero
		// because a negative bar is not a shorter wait, it is a malformed one.
		unresolved.set(
			entry.feedId,
			Math.max(0, Number(entry.retryAfterSeconds ?? 0n) * 1000),
		);
	}

	return { resolved, unresolved };
}
