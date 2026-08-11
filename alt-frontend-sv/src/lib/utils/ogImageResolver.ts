/**
 * Batches on-demand og:image resolution requests.
 *
 * Cards enter the viewport a few at a time, and each one asking on its own
 * would turn a scroll into a burst of RPCs. Requests raised within the same
 * short window are coalesced into one call, and every feed is asked about at
 * most once per session: a feed the server could not resolve has already had
 * its refusal recorded there, and asking again would only cost the publisher
 * another request.
 */

export interface OgImageResolverOptions {
	/** Sends one batch. Returns feed id → proxy URL for the feeds that resolved. */
	send: (feedIds: string[]) => Promise<Map<string, string>>;
	/** How long to gather feeds before sending. */
	flushMs?: number;
	/** Server-side cap on one batch; must not exceed the RPC's own limit. */
	maxBatch?: number;
}

export interface OgImageResolver {
	/** Resolves this feed's proxy URL, or null if it has none to give. */
	resolve: (feedId: string) => Promise<string | null>;
}

interface Waiter {
	feedId: string;
	settle: (url: string | null) => void;
}

export function createOgImageResolver(
	options: OgImageResolverOptions,
): OgImageResolver {
	const flushMs = options.flushMs ?? 40;
	const maxBatch = options.maxBatch ?? 10;

	// Feeds already asked about, with the answer. A null value is "asked, and
	// the server had nothing" — kept precisely so scrolling back does not ask
	// again.
	const settled = new Map<string, string | null>();
	const inFlight = new Map<string, Promise<string | null>>();

	let queue: Waiter[] = [];
	let timer: ReturnType<typeof setTimeout> | null = null;

	async function flush() {
		timer = null;
		const batch = queue;
		queue = [];

		for (let i = 0; i < batch.length; i += maxBatch) {
			const slice = batch.slice(i, i + maxBatch);
			const ids = slice.map((w) => w.feedId);

			try {
				const resolved = await options.send(ids);
				for (const waiter of slice) {
					const url = resolved.get(waiter.feedId) ?? null;
					settled.set(waiter.feedId, url);
					waiter.settle(url);
				}
			} catch {
				// A transport failure is ours, not the origin's answer. Leave
				// these feeds unsettled so a later viewport entry may try again
				// rather than turning our own outage into a permanent blank.
				for (const waiter of slice) {
					waiter.settle(null);
				}
			} finally {
				for (const waiter of slice) {
					inFlight.delete(waiter.feedId);
				}
			}
		}
	}

	function resolve(feedId: string): Promise<string | null> {
		if (!feedId) return Promise.resolve(null);

		if (settled.has(feedId)) {
			return Promise.resolve(settled.get(feedId) ?? null);
		}
		const pending = inFlight.get(feedId);
		if (pending) return pending;

		const promise = new Promise<string | null>((settle) => {
			queue.push({ feedId, settle });
		});
		inFlight.set(feedId, promise);

		if (timer === null) {
			timer = setTimeout(flush, flushMs);
		}
		return promise;
	}

	return { resolve };
}

/**
 * The resolver the feed surfaces share.
 *
 * One instance per session on purpose: the "already asked" memory is the thing
 * that keeps a reader scrolling up and down a grid from re-requesting the same
 * publisher pages, and a per-component resolver would forget between mounts.
 */
let shared: OgImageResolver | null = null;

export function ogImageResolver(): OgImageResolver {
	if (!shared) {
		shared = createOgImageResolver({
			send: async (feedIds) => {
				const { createClientTransport } = await import(
					"$lib/connect/transport-client"
				);
				const { resolveOgImages } = await import("$lib/connect/feeds/ogImages");
				return resolveOgImages(createClientTransport(), feedIds);
			},
		});
	}
	return shared;
}
