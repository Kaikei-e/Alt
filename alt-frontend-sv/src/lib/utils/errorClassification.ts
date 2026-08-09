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
 * Classifies errors as transient (retryable) or permanent.
 * Used by Article/Summary retry logic across Desktop and Mobile components.
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
