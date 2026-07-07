/**
 * Shared return_to sanitization for auth routes (login, register).
 *
 * Prevents open redirects: any return_to value that does not resolve to the
 * same origin as the trusted `origin` argument falls back to a safe path.
 * String-prefix checks (e.g. rejecting values that "start with http") are not
 * enough — `//evil.com` and backslash variants resolve to a different origin
 * once parsed, so we always parse with `URL` and compare the resulting
 * `origin`, never the raw string.
 */

const ABSOLUTE_URL_RE = /^https?:\/\//i;

export function isAbsoluteUrl(value: string): boolean {
	return ABSOLUTE_URL_RE.test(value);
}

export interface SanitizeReturnToOptions {
	/** Path to use when return_to is missing, unsafe, or loops back to the auth page. Defaults to "/feeds". */
	fallbackPath?: string;
	/** Exact path suffixes that indicate a redirect loop back to the auth flow itself (e.g. "/login"). */
	loopPaths?: string[];
}

/**
 * Sanitizes a return_to value against open redirects.
 * - Relative values are resolved against `origin`.
 * - Absolute values are only accepted when they share the same origin as
 *   `origin`; cross-origin values fall back to `fallbackPath`.
 * - The query string is stripped (avoids carrying stale params through the loop).
 * - Values ending in one of `loopPaths` fall back too (avoids bouncing back to
 *   the auth page that just redirected here).
 */
export function sanitizeReturnTo(
	returnTo: string | null | undefined,
	origin: string,
	options: SanitizeReturnToOptions = {},
): string {
	const fallbackPath = options.fallbackPath ?? "/feeds";
	const loopPaths = options.loopPaths ?? [];
	const originUrl = new URL(origin);
	const fallback = `${originUrl.origin}${fallbackPath}`;

	if (!returnTo) {
		return fallback;
	}

	let resolved: URL;
	try {
		resolved = isAbsoluteUrl(returnTo)
			? new URL(returnTo)
			: new URL(returnTo.startsWith("/") ? returnTo : `/${returnTo}`, originUrl);
	} catch {
		return fallback;
	}

	// Enforce same-origin: cross-origin absolute (and protocol-relative) URLs
	// are open-redirect vectors and must never be forwarded as-is.
	if (resolved.origin !== originUrl.origin) {
		return fallback;
	}

	const cleanUrl = `${resolved.origin}${resolved.pathname}`;

	if (loopPaths.some((path) => cleanUrl.endsWith(path))) {
		return fallback;
	}

	return cleanUrl;
}
