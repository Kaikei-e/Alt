/**
 * Batches on-demand og:image resolution requests.
 *
 * Cards enter the viewport a few at a time, and each one asking on its own
 * would turn a scroll into a burst of RPCs. Requests raised within the same
 * short window are coalesced into one call, and every feed the server actually
 * answers about is asked at most once per session: a feed the server could not
 * resolve has already had its refusal recorded there, and asking again would
 * only cost the publisher another request.
 *
 * What is *not* remembered is a failure to ask. `ResolveOgImages` fetches the
 * publisher's page inline, behind a per-host slot, so the ordinary way a
 * successful resolution is missed is that the reader's RPC gave up before the
 * server finished — and the server finishes anyway, storing the image. Treating
 * that as the origin's answer is what pinned a card to "No preview" for the
 * rest of the session while the image sat in the store, one cheap re-ask away.
 */

import { Code, ConnectError } from "@connectrpc/connect";
import { FAILURE_SCOPE_HEADER } from "./errorClassification";
import { parseRetryAfter } from "./retryAfter";

/**
 * What one ask produced.
 *
 * The three cases are three different things to do next, which is why this is
 * not `string | null`. `resolved` and `absent` are the server's answer and are
 * remembered forever; `unavailable` is our own failure to obtain one and is
 * remembered by nobody, so the caller may ask again on its own schedule.
 */
export type OgImageOutcome =
	| { status: "resolved"; url: string }
	/** The server answered, and this feed has no image to give. Final. */
	| { status: "absent" }
	/** We never got an answer. Not the origin's verdict — ask again later. */
	| { status: "unavailable"; retryAfterMs: number | null };

export interface OgImageResolverOptions {
	/** Sends one batch. Returns feed id → proxy URL for the feeds that resolved. */
	send: (feedIds: string[]) => Promise<Map<string, string>>;
	/** How long to gather feeds before sending. */
	flushMs?: number;
	/** Server-side cap on one batch; must not exceed the RPC's own limit. */
	maxBatch?: number;
}

export interface OgImageResolver {
	/** Asks about this feed, coalesced with whatever else is being asked. */
	resolve: (feedId: string) => Promise<OgImageOutcome>;
}

interface Waiter {
	feedId: string;
	settle: (outcome: OgImageOutcome) => void;
}

const ABSENT: OgImageOutcome = { status: "absent" };

/**
 * Codes that will answer the same way however often they are re-sent, so the
 * ask is settled as the origin's own "no" and remembered.
 *
 * `Unimplemented` is the handler declaring the image proxy switched off, which
 * holds for the whole process; the rest are the request itself being wrong.
 */
const TERMINAL_CODES: ReadonlySet<Code> = new Set([
	Code.Unimplemented,
	Code.PermissionDenied,
	Code.Unauthenticated,
	Code.InvalidArgument,
	Code.NotFound,
]);

/**
 * Turn a failed ask into an outcome.
 *
 * Everything outside `TERMINAL_CODES` is `unavailable`, which is the opposite
 * default from `isRetryableContentError`'s allow-list, and deliberately so. That
 * predicate gates a ladder that re-contacts *publishers*, so an unclassifiable
 * failure there must not licence another origin request. This one gates a
 * re-ask of our own server, which — for the case that matters, a resolution
 * that completed after we stopped listening — is answered from the store with
 * no origin request at all. A feed the origin genuinely refused carries that
 * refusal in the store too, and `NeedsFetch()` keeps the re-ask away from the
 * publisher just the same. So the cost of guessing "retryable" wrongly here is
 * one more request to ourselves, not one more to somebody else.
 *
 * The one exception is the rate limit a publisher issued itself. ADR-000963 has
 * alt-backend stamp `X-Alt-Failure-Scope: host` only on failures it has
 * positively attributed to the third-party site; re-sending into that is the
 * storm ADR-000884 exists to prevent, so it settles as `absent`.
 *
 * Decided on the Connect code and metadata, never on the message: prose is the
 * publisher's or the gateway's and changes without notice.
 */
function classifyFailure(err: unknown): OgImageOutcome {
	const connectErr = ConnectError.from(err);

	if (TERMINAL_CODES.has(connectErr.code)) return ABSENT;
	if (
		connectErr.code === Code.ResourceExhausted &&
		connectErr.metadata.get(FAILURE_SCOPE_HEADER) === "host"
	) {
		return ABSENT;
	}

	return {
		status: "unavailable",
		retryAfterMs: parseRetryAfter(connectErr.metadata.get("retry-after")),
	};
}

export function createOgImageResolver(
	options: OgImageResolverOptions,
): OgImageResolver {
	const flushMs = options.flushMs ?? 40;
	const maxBatch = options.maxBatch ?? 10;

	// Feeds the server has answered about, with the answer. An `absent` value is
	// "asked, and the server had nothing" — kept precisely so scrolling back
	// does not ask again. An `unavailable` never lands here.
	const settled = new Map<string, OgImageOutcome>();
	const inFlight = new Map<string, Promise<OgImageOutcome>>();

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
					const url = resolved.get(waiter.feedId);
					const outcome: OgImageOutcome = url
						? { status: "resolved", url }
						: ABSENT;
					settled.set(waiter.feedId, outcome);
					waiter.settle(outcome);
				}
			} catch (err) {
				// A transport failure is ours, not the origin's answer. Only a
				// classification that says "this will answer the same way
				// forever" is remembered; anything else leaves the feed
				// unsettled so the caller's ladder may ask again.
				const outcome = classifyFailure(err);
				if (outcome.status === "absent") {
					for (const waiter of slice) settled.set(waiter.feedId, outcome);
				}
				for (const waiter of slice) waiter.settle(outcome);
			} finally {
				for (const waiter of slice) {
					inFlight.delete(waiter.feedId);
				}
			}
		}
	}

	function resolve(feedId: string): Promise<OgImageOutcome> {
		if (!feedId) return Promise.resolve(ABSENT);

		const known = settled.get(feedId);
		if (known) return Promise.resolve(known);

		const pending = inFlight.get(feedId);
		if (pending) return pending;

		const promise = new Promise<OgImageOutcome>((settle) => {
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
