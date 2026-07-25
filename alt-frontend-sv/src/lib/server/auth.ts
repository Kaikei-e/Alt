import type { Cookies } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";
import { parseAuthHubCsrfToken } from "$lib/schema/csrf";

const AUTH_HUB_URL = env.AUTH_HUB_INTERNAL_URL || "http://auth-hub:8888";
const AUTH_HUB_TIMEOUT_MS = 3000;

// Name of the double-submit cookie mirroring the auth-hub-issued CSRF token.
const CSRF_COOKIE_NAME = "csrf_token";
// Matches auth-hub's DefaultCSRFTTL (auth-hub/internal/infrastructure/token/csrf.go).
const CSRF_COOKIE_MAX_AGE_S = 60 * 60;

/**
 * Get a fresh CSRF token for the session from auth-hub.
 * V-004: CSRF protection for state-changing operations
 */
export async function getCSRFToken(
	cookieHeader: string | null,
): Promise<string | null> {
	if (!cookieHeader) return null;

	try {
		const response = await fetch(`${AUTH_HUB_URL}/csrf`, {
			method: "POST",
			headers: { Cookie: cookieHeader },
			cache: "no-store",
			signal: AbortSignal.timeout(AUTH_HUB_TIMEOUT_MS),
		});

		if (!response.ok) return null;

		const data: unknown = await response.json();
		return parseAuthHubCsrfToken(data);
	} catch {
		return null;
	}
}

/**
 * Mirrors an auth-hub-issued CSRF token into an httpOnly cookie at issuance
 * time (double-submit cookie pattern). auth-hub's HMAC token embeds a fresh
 * unix-second timestamp on every call (auth-hub/internal/infrastructure/token/csrf.go),
 * so re-fetching a token from auth-hub at guard time can never match the
 * token the client obtained earlier — validation must compare against the
 * value captured here instead of calling auth-hub a second time.
 */
export function issueCsrfCookie(cookies: Cookies, token: string): void {
	cookies.set(CSRF_COOKIE_NAME, token, {
		httpOnly: true,
		sameSite: "strict",
		path: "/",
		maxAge: CSRF_COOKIE_MAX_AGE_S,
	});
}

/**
 * V-004: CSRF validation for state-changing operations, via double-submit
 * cookie comparison — no auth-hub round trip needed at guard time.
 */
export function verifyCsrfToken(
	cookies: Cookies,
	providedToken: string | null,
): boolean {
	const expected = cookies.get(CSRF_COOKIE_NAME);
	return !!expected && !!providedToken && expected === providedToken;
}

/**
 * auth-hubからバックエンドトークンを取得
 */
export async function getBackendToken(
	cookie: string | null,
): Promise<string | null> {
	if (!cookie) {
		console.warn("No cookie provided for backend token");
		return null;
	}

	try {
		const response = await fetch(`${AUTH_HUB_URL}/session`, {
			headers: {
				cookie: cookie,
			},
			cache: "no-store",
			signal: AbortSignal.timeout(AUTH_HUB_TIMEOUT_MS),
		});

		if (!response.ok) {
			console.warn(
				`Auth-hub session endpoint returned ${response.status}: ${response.statusText}`,
			);
			return null;
		}

		const token = response.headers.get("X-Alt-Backend-Token");
		if (!token) {
			console.warn("X-Alt-Backend-Token header not found in response");
		}
		return token;
	} catch (error) {
		const errorMessage = error instanceof Error ? error.message : String(error);
		const errorStack = error instanceof Error ? error.stack : undefined;
		console.error("Failed to get backend token:", {
			message: errorMessage,
			stack: errorStack,
			authHubUrl: AUTH_HUB_URL,
			hasCookie: !!cookie,
		});
		return null;
	}
}
