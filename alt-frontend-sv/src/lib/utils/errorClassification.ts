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
