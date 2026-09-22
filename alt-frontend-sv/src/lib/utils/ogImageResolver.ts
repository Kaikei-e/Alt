/**
 * Manages on-demand og:image resolution requests with bounded concurrency.
 *
 * Cards enter the viewport and request resolution independently as singletons.
 * Instead of an all-or-nothing batch await barrier where a slow publisher
 * scrape or politeness wait freezes all cards in the batch, requests are
 * dispatched independently via a work-conserving queue with a concurrency cap
 * (`maxConcurrent`, default 4). When any request completes, the next queued card
 * begins immediately.
 *
 * What is *not* remembered is an answer that says "not yet". Two different
 * things say it. One is a failure to ask at all: `ResolveOgImages` fetches the
 * publisher's page inline, behind a per-host slot, so the ordinary way a
 * successful resolution is missed is that the reader's RPC gave up before the
 * server finished — and the server finishes anyway, storing the image. Treating
 * that as the origin's answer is what pinned a card to "No preview" for the
 * rest of the session while the image sat in the store, one cheap re-ask away.
 * The other is the server answering that it tried and failed, and naming the
 * bar it wants respected before being asked again — a bar it would not have
 * sent if it thought the answer were final.
 *
 * So "the server did not give us a URL" is four answers, not one, and which one
 * it is decides whether the card folds or waits. `outcomeFor` below is where
 * that is read off the wire.
 */

import { Code, ConnectError } from "@connectrpc/connect";
import type { OgImageResolution } from "$lib/connect/feeds/ogImages";
import { FAILURE_SCOPE_HEADER } from "./errorClassification";
import { OG_RETRY_CEILING_MS } from "./ogImageRetry";
import { createRequestQueue } from "./requestQueue";
import { parseRetryAfter } from "./retryAfter";

/**
 * What one ask produced.
 *
 * The three cases are three different things to do next, which is why this is
 * not `string | null`. `resolved` and `absent` are settled and are remembered
 * for the session; `unavailable` is "ask again later" and is remembered by
 * nobody, so the caller may put the question again on its own schedule.
 */
export type OgImageOutcome =
	| { status: "resolved"; url: string }
	/** Settled: there is no image to be had here, now. Do not ask again. */
	| { status: "absent" }
	/**
	 * Not settled. Either we could not ask, or the server asked and failed and
	 * named a bar. `retryAfterMs` is that bar when there is one, and a floor
	 * rather than a schedule — see `ogImageRetryDelayMs`.
	 */
	| { status: "unavailable"; retryAfterMs: number | null };

export interface OgImageResolverOptions {
	/** Sends one singleton request. Returns the server's two lists for those feeds. */
	send: (feedIds: string[]) => Promise<OgImageResolution>;
	/** Maximum concurrent in-flight requests. Defaults to 4. */
	maxConcurrent?: number;
}

export interface OgImageResolver {
	/** Asks about this feed, bounded by the resolver's concurrency cap. */
	resolve: (feedId: string) => Promise<OgImageOutcome>;
}

const ABSENT: OgImageOutcome = { status: "absent" };

/**
 * Turn the server's answer about one feed into an outcome.
 *
 * This is the "we asked, and here is what came back" axis. `classifyFailure`
 * below is the other one — "we could not ask at all" — and the two are kept
 * apart deliberately: a transport failure says nothing about the publisher,
 * and a publisher's refusal says nothing about the transport.
 *
 * Four answers, distinguished by which list the feed is in:
 *
 *  - in `resolved` — the picture is there. Taken first, so a server that
 *    somehow emits a feed in both lists still gives the reader the image it
 *    managed to produce.
 *  - in `unresolved` with a bar of zero — the origin was asked and refused
 *    (robots.txt, or a page carrying no og:image). Settled for this retention
 *    window: remembered, and never asked again.
 *  - in `unresolved` with a bar inside the ceiling — the ask failed and may
 *    well succeed once the bar lifts. Not remembered, and the bar is handed to
 *    the caller's ladder as a floor.
 *  - in `unresolved` with a bar beyond the ceiling — treated as settled, see
 *    below.
 *  - in neither list — the server never reached this feed, see below.
 */
function outcomeFor(
	feedId: string,
	{ resolved, unresolved }: OgImageResolution,
): OgImageOutcome {
	const url = resolved.get(feedId);
	if (url) return { status: "resolved", url };

	const retryAfterMs = unresolved.get(feedId);

	if (retryAfterMs === undefined) {
		// In neither list: the server never reached this feed — its batch cap
		// trimmed it, no row exists, its page URL was unusable. Not remembered,
		// because no origin request was spent on it. Asking again costs only
		// our own server, and the publisher, who was never contacted, is owed
		// nothing by our silence.
		return { status: "unavailable", retryAfterMs: null };
	}

	if (retryAfterMs <= 0) return ABSENT;

	if (retryAfterMs > OG_RETRY_CEILING_MS) {
		// A bar longer than a browser tab can hold a tile open for. Shimmering
		// at a wait we have already decided not to honour would be a lie twice
		// over: to the reader, who is shown a picture arriving that this page
		// will never request, and to the publisher, whom we are not in fact
		// waiting for — no request is outstanding, and none is scheduled. The
		// card takes its fallback and folds; the server's own row survives and
		// is collected on a later page load, by a session whose clock has
		// actually passed the bar.
		return ABSENT;
	}

	return { status: "unavailable", retryAfterMs };
}

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
	const queue = createRequestQueue({ concurrency: options.maxConcurrent });

	// Feeds whose answer is settled, with that answer. An `absent` value is
	// "asked, and there is nothing to be had" — kept precisely so scrolling back
	// does not ask again. An `unavailable` never lands here, whether it came
	// from a transport failure or from the server naming a retry bar.
	const settled = new Map<string, OgImageOutcome>();
	const inFlight = new Map<string, Promise<OgImageOutcome>>();

	function resolve(feedId: string): Promise<OgImageOutcome> {
		if (!feedId) return Promise.resolve(ABSENT);

		const known = settled.get(feedId);
		if (known) return Promise.resolve(known);

		const pending = inFlight.get(feedId);
		if (pending) return pending;

		let settle!: (outcome: OgImageOutcome) => void;
		let fail!: (err: unknown) => void;
		const promise = new Promise<OgImageOutcome>((s, f) => {
			settle = s;
			fail = f;
		});

		// Register inFlight BEFORE queueing/dispatching to prevent stale registration
		// if send throws synchronously.
		inFlight.set(feedId, promise);

		queue
			.add(async () => {
				try {
					const answer = await options.send([feedId]);
					const outcome = outcomeFor(feedId, answer);
					// Only the server's settled answers are remembered.
					// `unavailable` is a bar that will lift, so recording it
					// would turn a five-second wait into a permanent blank.
					if (outcome.status !== "unavailable") {
						settled.set(feedId, outcome);
					}
					return outcome;
				} catch (err) {
					// A transport failure is ours, not the origin's answer. Only a
					// classification that says "this will answer the same way
					// forever" is remembered; anything else leaves the feed
					// unsettled so the caller's ladder may ask again.
					const outcome = classifyFailure(err);
					if (outcome.status === "absent") {
						settled.set(feedId, outcome);
					}
					return outcome;
				} finally {
					inFlight.delete(feedId);
				}
			})
			.then(settle, (err) => {
				inFlight.delete(feedId);
				fail(err);
			});

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
