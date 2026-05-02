/**
 * Parse a Retry-After header value (RFC 6585 / RFC 7231 section 7.1.3).
 *
 * Supports:
 * - delta-seconds (e.g. "120")
 * - HTTP-date (e.g. "Wed, 21 Oct 2026 07:28:00 GMT")
 *
 * Returns the wait in milliseconds, or null when the header is absent or
 * unparseable. Negative or NaN values are clamped to null so callers can fall
 * back to a default cooldown.
 */
export function parseRetryAfter(
	value: string | null | undefined,
): number | null {
	if (value === null || value === undefined) return null;
	const trimmed = value.trim();
	if (trimmed === "") return null;

	if (/^\d+$/.test(trimmed)) {
		const seconds = Number.parseInt(trimmed, 10);
		if (!Number.isFinite(seconds) || seconds < 0) return null;
		return seconds * 1000;
	}

	const dateMs = Date.parse(trimmed);
	if (Number.isNaN(dateMs)) return null;
	const delta = dateMs - Date.now();
	return delta > 0 ? delta : 0;
}
