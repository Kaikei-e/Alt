/**
 * How long to wait before asking again for an og:image we failed to obtain.
 *
 * The ladder exists because `ResolveOgImages` fetches the publisher's page
 * inline, behind a per-host slot. When a batch cannot finish inside one RPC the
 * reader's request dies, but the feeds the server *did* get to are already in
 * the store — so the next ask is answered from there, with no origin request at
 * all, and the grid fills in a few cards at a time. Without the re-ask those
 * cards stay blank until the page is reloaded, even though the images exist.
 *
 * It is short and bounded on purpose. A thumbnail is not worth an open-ended
 * wait: three asks in total, and an unreachable backend lands the card on its
 * fallback rather than shimmering for the rest of the session.
 */

/** Asks in total, the first one included. */
export const MAX_RESOLVE_ATTEMPTS = 3;

/** The first retry window. Doubles per attempt. */
export const OG_RETRY_BASE_MS = 1_200;

/** No single wait may exceed this, whoever asked for it. */
export const OG_RETRY_CEILING_MS = 10_000;

/**
 * The wait before attempt `attempt + 1`, in milliseconds.
 *
 * Full jitter — `random(0, window)` rather than `window ± noise` — so that
 * every client whose request died in the same second does not come back in the
 * same second. This is the shape ADR-000982 settled on for the article-content
 * ladder, kept identical here so the two are read as one policy.
 *
 * A `Retry-After` the server named is a floor, not a suggestion: re-asking
 * inside a gate it has just closed spends a slot on a request it has already
 * said it will refuse. It is clamped to the same ceiling as everything else,
 * because an hour-long `Retry-After` is not a thing to hold a tile open for.
 */
export function ogImageRetryDelayMs(
	attempt: number,
	retryAfterMs: number | null,
	random: () => number = Math.random,
): number {
	const window = Math.min(
		OG_RETRY_CEILING_MS,
		OG_RETRY_BASE_MS * 2 ** Math.max(0, attempt),
	);
	const jittered = random() * window;
	const floor = Math.min(OG_RETRY_CEILING_MS, Math.max(0, retryAfterMs ?? 0));
	return Math.max(floor, jittered);
}
