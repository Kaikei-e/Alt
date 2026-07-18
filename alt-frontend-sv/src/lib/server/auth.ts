import { env } from "$env/dynamic/private";
import { parseCsrfToken } from "$lib/schema/csrf";

const AUTH_HUB_URL = env.AUTH_HUB_INTERNAL_URL || "http://auth-hub:8888";
const AUTH_HUB_TIMEOUT_MS = 3000;

/**
 * Get CSRF token from auth-hub session
 * V-004: CSRF protection for state-changing operations
 */
export async function getCSRFToken(
	cookieHeader: string | null,
): Promise<string | null> {
	if (!cookieHeader) return null;

	try {
		const response = await fetch(`${AUTH_HUB_URL}/session`, {
			headers: { Cookie: cookieHeader },
			cache: "no-store",
			signal: AbortSignal.timeout(AUTH_HUB_TIMEOUT_MS),
		});

		if (!response.ok) return null;

		const data: unknown = await response.json();
		return parseCsrfToken(data);
	} catch {
		return null;
	}
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
