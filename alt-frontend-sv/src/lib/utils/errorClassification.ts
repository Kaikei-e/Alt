import { Code, ConnectError } from "@connectrpc/connect";

const TRANSIENT_CODES: ReadonlySet<Code> = new Set([
	Code.Unavailable,
	Code.ResourceExhausted,
	Code.DeadlineExceeded,
]);

/**
 * User-facing message for a failed article-content fetch.
 *
 * The upstream message is deliberately dropped. connect-es formats a Connect
 * error as `[code] message`, where the message is whatever the BFF wrote — so
 * rendering it put "[unavailable] Service temporarily unavailable due to
 * circuit breaker" straight onto the reading surface.
 */
export function articleContentErrorMessage(err: unknown): string {
	return TRANSIENT_CODES.has(ConnectError.from(err).code)
		? "Source content is temporarily unavailable. Please try again shortly."
		: "Source content unavailable.";
}

/**
 * Header naming how far a failure reaches. alt-backend stamps it on errors it
 * has attributed to a third-party publisher; the BFF stamps it on the one
 * rejection that really is system-wide, its own open circuit breaker.
 */
export const FAILURE_SCOPE_HEADER = "X-Alt-Failure-Scope";

/**
 * Whether a failure reaches every host, so backing off one of them is futile.
 *
 * Only an explicit claim counts. `Code.Unavailable` is issued both by the BFF
 * for an open breaker — genuinely global — and by alt-backend for a single
 * publisher that never answered, which is most of them. Reading the code alone
 * as global let one dead link pause every host for the whole cooldown, and the
 * reader saw the summary fallback on every card without a request being sent.
 *
 * The ambiguous case resolves to false on purpose: pausing one host wrongly
 * costs one card, pausing all of them wrongly costs the session, and the
 * gateway's own breaker still bounds what a wrong guess can cost it.
 */
export function isGlobalFailureScope(metadata: Headers | undefined): boolean {
	return metadata?.get(FAILURE_SCOPE_HEADER) === "global";
}

/**
 * Codes that mean "later", not "never", for an article-content fetch.
 *
 * `Unavailable` is in here and deliberately NOT in `isTransientError` — see
 * the note on that function. The two predicates answer different questions.
 */
const RETRYABLE_CONTENT_CODES: ReadonlySet<Code> = new Set([
	Code.ResourceExhausted,
	Code.Unavailable,
	Code.DeadlineExceeded,
]);

/**
 * Codes that will fail the same way on the next attempt. Spelled out rather
 * than left to the default so that adding a code to the retryable set can
 * never silently promote one of these.
 */
const NEVER_RETRYABLE_CONTENT_CODES: ReadonlySet<Code> = new Set([
	Code.PermissionDenied,
	Code.NotFound,
	Code.InvalidArgument,
	Code.Unauthenticated,
]);

/**
 * Whether the background prefetch ladder may send this request again.
 *
 * Decided on `ConnectError.code` and the failure-scope metadata, never on the
 * message. Prose is the publisher's, or the gateway's, and changes without
 * notice: a 404 whose body mentions "429" is still permanent, and a real rate
 * limit that says nothing at all is still a rate limit.
 *
 * Codes in neither list resolve to false. `Code.Unknown` is what connect-es
 * produces for a fetch that never reached a gateway and `Code.Canceled` is the
 * reader navigating away; neither is a licence to re-send.
 *
 * The metadata branch keeps the retry off publishers. ADR-000963 §2 has
 * alt-backend stamp `X-Alt-Failure-Scope: host` only on failures it has
 * positively attributed to the third-party site, and deliberately leave its
 * own politeness gate unstamped. A stamped `ResourceExhausted` is therefore
 * the publisher itself answering 429 — re-sending into that is precisely the
 * storm ADR-000884 exists to prevent — while an unstamped one is Alt's own
 * host-slot queue, which frees on a schedule we control and never put a packet
 * on the wire.
 *
 * NOT a replacement for `isTransientError`, and not wired into its callers.
 * That predicate gates an immediate ~500ms component-level re-send, and
 * ADR-000959 explicitly considered and rejected adding `Code.Unavailable` to
 * it because two such call sites re-sending into an open circuit breaker was
 * the incident being fixed. This one gates a jittered, cooldown-gated,
 * budget-capped background retry inside `articlePrefetcher`, where the host
 * cooldown and the all-hosts pause still bound every attempt. The two are
 * allowed to disagree about `Unavailable`, and they do.
 */
export function isRetryableContentError(err: unknown): boolean {
	const connectErr = ConnectError.from(err);
	if (NEVER_RETRYABLE_CONTENT_CODES.has(connectErr.code)) return false;
	if (!RETRYABLE_CONTENT_CODES.has(connectErr.code)) return false;
	if (
		connectErr.code === Code.ResourceExhausted &&
		connectErr.metadata.get(FAILURE_SCOPE_HEADER) === "host"
	) {
		return false;
	}
	return true;
}

/**
 * Classifies errors as transient (retryable) or permanent.
 * Used by Article/Summary retry logic across Desktop and Mobile components.
 *
 * Message-based on purpose: its callers see plain `Error`s from paths that
 * never carried a Connect code. Leave `Code.Unavailable` out of its remit —
 * ADR-000959 rejected adding it, because the two components that act on it
 * re-send 500ms later and hammer the very circuit breaker that answered.
 * Background retries with a real budget belong to `isRetryableContentError`.
 */
export function isTransientError(err: unknown): boolean {
	if (!(err instanceof Error)) return false;
	const msg = err.message.toLowerCase();
	return (
		msg.includes("network") ||
		msg.includes("fetch") ||
		msg.includes("timeout") ||
		msg.includes("503") ||
		msg.includes("502") ||
		msg.includes("429")
	);
}
