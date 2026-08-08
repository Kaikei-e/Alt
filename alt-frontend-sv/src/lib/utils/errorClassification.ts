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
